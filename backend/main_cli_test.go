package main

// Regression test for the deploy-pipeline hang: the `-migrate` one-off mode
// must run migrations + seed and then EXIT (code 0, within seconds) — it
// must never fall through into scheduler/server startup.
//
// Background: `docker compose run --rm backend /app/kikundi-api -migrate`
// hung until the 20-minute CI timeout even though migration output printed
// in ~2s. Two defenses now guard this:
//  1. main.go handles ALL one-off CLI flags BEFORE scheduler/ledger/server
//     code, so "Scheduler started" / "Server starting" structurally cannot appear.
//  2. deploy.yml pins restart:no via docker-compose.migrate.yml for one-offs.
//
// This test enforces defense #1 against the TEST database only
// (kikundi_test — never kikundi_db): it builds the real binary, runs it
// with -migrate, and fails on timeout, non-zero exit, or forbidden lines.
//
// NOTE: needs a reachable PostgreSQL with a kikundi_test database
// (same requirement as the handlers suite).

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func buildMigrateBinary(t *testing.T) string {
	t.Helper()
	bin := filepath.Join(t.TempDir(), "kikundi-api-test")
	build := exec.Command("go", "build", "-o", bin, ".")
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build test binary: %v\n%s", err, out)
	}
	return bin
}

func migrateTestEnv() []string {
	get := func(k, fb string) string {
		if v := os.Getenv(k); v != "" {
			return v
		}
		return fb
	}
	return append(os.Environ(),
		"DB_HOST="+get("TEST_DB_HOST", "127.0.0.1"),
		"DB_PORT="+get("TEST_DB_PORT", "5432"),
		"DB_USER="+get("TEST_DB_USER", "postgres"),
		"DB_PASSWORD="+get("TEST_DB_PASSWORD", ""),
		"DB_NAME="+get("TEST_DB_NAME", "kikundi_test"),
		"DB_SSLMODE="+get("TEST_DB_SSLMODE", "disable"),
		"JWT_SECRET="+get("TEST_JWT_SECRET", "test-secret-key-at-least-32-characters!!"),
		"ENVIRONMENT=test",
		"PORT=18081",
	)
}

func runBinaryOnce(t *testing.T, bin string, args ...string) string {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, bin, args...)
	cmd.Env = migrateTestEnv()
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out
	err := cmd.Run()
	if ctx.Err() == context.DeadlineExceeded {
		t.Fatalf("-migrate did not exit within 90s (deploy-hang regression):\n%s", out.String())
	}
	if err != nil {
		t.Fatalf("-migrate exited non-zero: %v\n%s", err, out.String())
	}
	return out.String()
}

func TestMigrateFlagExitsPromptly(t *testing.T) {
	bin := buildMigrateBinary(t)

	first := runBinaryOnce(t, bin, "-migrate")
	if !strings.Contains(first, "Migration complete") {
		t.Errorf("first run must print 'Migration complete', got:\n%s", first)
	}
	for _, forbidden := range []string{"Scheduler started", "Server starting"} {
		if strings.Contains(first, forbidden) {
			t.Errorf("one-off mode must never print %q, got:\n%s", forbidden, first)
		}
	}

	// Second run proves the idempotent SKIP path exits just as cleanly.
	second := runBinaryOnce(t, bin, "-migrate")
	if !strings.Contains(second, "Migration complete") {
		t.Errorf("second run must also complete, got:\n%s", second)
	}
	for _, forbidden := range []string{"Scheduler started", "Server starting"} {
		if strings.Contains(second, forbidden) {
			t.Errorf("second run must never print %q, got:\n%s", forbidden, second)
		}
	}
}
