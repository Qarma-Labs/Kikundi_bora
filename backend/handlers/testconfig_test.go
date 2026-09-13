package handlers

import (
	"kikundibora/config"
	"kikundibora/database"
	"kikundibora/models"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/joho/godotenv"
	"github.com/shopspring/decimal"
)

// requireTestDB is the safety guard that keeps destructive test cleanups off
// real databases. cleanAndSeed / scopedCleanAndSeed / cleanObligationTables
// DELETE production-shaped data, so they refuse to run unless the connected
// database name ends in _test (override with DB_NAME for CI).
func requireTestDB(t *testing.T) {
	t.Helper()
	name := ""
	if config.AppConfig != nil {
		name = config.AppConfig.DBName
	}
	if !strings.HasSuffix(name, "_test") {
		t.Fatalf("REFUSING to wipe non-test database %q — run tests with DB_NAME=kikundi_test", name)
	}
}

// cleanAllTables performs a full, FK-ordered cleanup of every table the
// handlers touch, so tests always start from a known state regardless of what
// previous runs left behind. A partial delete list lets FK-referenced rows
// survive, silently aborts later DELETEs and skips reseeding.
func cleanAllTables() {
	for _, stmt := range []string{
		"DELETE FROM loan_reviews",
		"DELETE FROM repayments",
		"DELETE FROM loan_installments",
		"DELETE FROM loans",
		"DELETE FROM loan_settings",
		"DELETE FROM contribution_edits",
		"DELETE FROM contributions",
		"DELETE FROM member_contributions",
		"DELETE FROM user_approvals",
		"DELETE FROM user_sessions",
		"DELETE FROM failed_logins",
		"DELETE FROM loan_committee_members",
		"DELETE FROM leadership_positions",
		"DELETE FROM user_positions",
		"DELETE FROM notifications",
		"DELETE FROM audit_logs",
		"DELETE FROM admin_logs",
		"DELETE FROM pending_actions",
		"DELETE FROM welfare_contributions",
		"DELETE FROM welfare_events",
		"DELETE FROM group_setting_proposals",
		"DELETE FROM members",
		"DELETE FROM users",
	} {
		database.DB.Exec(stmt)
	}
}

// fundTreasury inserts a PAID contribution directly in the DB so loan
// affordability checks (treasury must cover the loan) pass in tests.
func fundTreasury(amount int64) {
	var chair models.User
	if err := database.DB.Where("role = ? AND deleted_at IS NULL", models.RoleChair).
		First(&chair).Error; err != nil {
		return
	}
	var member models.Member
	if err := database.DB.Where("deleted_at IS NULL AND is_active = TRUE").
		Order("member_no").First(&member).Error; err != nil {
		return
	}
	now := time.Now()
	database.DB.Create(&models.Contribution{
		MemberID:      member.ID,
		RecordedBy:    chair.ID,
		Amount:        decimal.NewFromInt(amount),
		Month:         time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, now.Location()),
		PaidAt:        now,
		PaymentMethod: "CASH",
		Status:        "PAID",
		ConfirmedBy:   &chair.ID,
	})
}

// withLoginRateLimitEnabled temporarily clears DISABLE_LOGIN_RATE_LIMIT
// (a local-testing convenience read from .env at request time) so rate-limit
// tests behave deterministically. Returns a restore func.
func withLoginRateLimitEnabled() func() {
	prev := os.Getenv("DISABLE_LOGIN_RATE_LIMIT")
	os.Setenv("DISABLE_LOGIN_RATE_LIMIT", "")
	return func() {
		val := prev
		os.Setenv("DISABLE_LOGIN_RATE_LIMIT", val)
	}
}

// testConfig builds AppConfig from the environment (loading .env when present)
// so tests run against whatever database the developer or CI provides,
// instead of hardcoded local credentials.
func testConfig() *config.Config {
	// Load .env from the package dir (go test cwd) or the project root.
	_ = godotenv.Load()
	_ = godotenv.Load("../.env")
	get := func(key, fallback string) string {
		if v := os.Getenv(key); v != "" {
			return v
		}
		return fallback
	}
	return &config.Config{
		DBHost:      get("DB_HOST", "127.0.0.1"),
		DBPort:      get("DB_PORT", "5432"),
		DBUser:      get("DB_USER", "postgres"),
		DBPassword:  get("DB_PASSWORD", ""),
		// Tests NEVER default to the dev database: backend/.env sets
		// DB_NAME=kikundi_db, so tests use a dedicated TEST_DB_NAME
		// (default kikundi_test) and requireTestDB refuses anything
		// else. This is what keeps `go test` from ever wiping live data.
		DBName:      get("TEST_DB_NAME", "kikundi_test"),
		DBSSLMode:   get("DB_SSLMODE", "disable"),
		JWTSecret:   get("JWT_SECRET", "test-secret-key-at-least-32-characters!!"),
		Port:        "0",
		CORSOrigins: "http://localhost:3000",
		Environment: "test",
	}
}
