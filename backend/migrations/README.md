# Versioned schema migrations (GORM-ledger extension)

This package adds **tracking** on top of the existing GORM AutoMigrate
system — it does not replace it. The 60+ existing model tables and the
idempotent fixes in `database/migrate.go` stay exactly as they are, covered
by the v1 baseline row.

## How it works

- `schema_migrations(version, applied_at, description)` is the ledger. The
  table bootstraps itself via GORM AutoMigrate — no new dependency.
- `Migrations` (in `migrations.go`) is the ordered registry. Each entry has
  a dated `Version`, a `Description`, and an `Up` func holding whatever GORM
  AutoMigrate calls or raw idempotent `DB.Exec` statements that change needs.
- The runner (`Run`, invoked by the existing `-migrate` flag) reads the
  ledger, runs only NOT-yet-applied entries in order, and inserts a ledger
  row per completed entry. Output per entry:
  - `MIGRATE: [SKIP] <version> already applied`
  - `MIGRATE: [RUN] <version>` → `MIGRATE: [APPLIED] <version>`
  - `MIGRATE: [FAIL] <version>: <err>` — never recorded, whole run stops
    (non-zero exit; deploy `set -e` halts before containers restart).

## Adding a new migration (the discipline)

1. Append to `Migrations` — never edit/reorder existing entries:
   ```go
   {Version: "20260920_my_change", Description: "What it does", Up: upMyChange},
   ```
2. Write `Up` with **idempotent** statements only (`CREATE ... IF NOT EXISTS`,
   guarded backfills). Assume it may run twice.
3. `Down` is optional — add one only where reversal is trivial. The
   supported rollback path is **restore from the pre-migration backup**
   taken by the deploy pipeline moments before migrating (deliberate
   choice: GORM-based automatic downs for 60+ tables are not worth building).
4. Test locally: `./server -migrate` twice — first run `[APPLIED]`, second
   run `[SKIP]`.

## One-off production baseline (MANUAL, once, with sign-off)

The `20260913_baseline` entry has `Up: nil` — the runner **never** creates
its row. Against the CURRENT production database, run exactly once:

```sql
-- BASELINE_PROD.sql (also saved next to this README)
INSERT INTO schema_migrations (version, applied_at, description)
VALUES ('20260913_baseline', NOW(), 'Baseline — all pre-existing GORM AutoMigrate schema accounted for')
ON CONFLICT (version) DO NOTHING;
```

Confirm with `SELECT * FROM schema_migrations;` (exactly 1 row) before
considering Step 5.1 done. Fresh databases simply log the `[!!]` reminder
and continue — later entries are self-contained.

## What stays under AutoMigrate

All pre-existing models/tables and the idempotent Execs in
`database/migrate.go`. Do NOT retroactively convert them into registry
entries; only NEW changes from here forward use this registry.
