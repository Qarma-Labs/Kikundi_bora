-- BASELINE_PROD.sql — one-off MANUAL baseline for the production database.
-- Run EXACTLY ONCE via deploy SSH access, with explicit human sign-off,
-- BEFORE the versioned registry is relied upon. Never automated.
--
-- What it does: records that every schema object existing up to 2026-09-13
-- (all GORM AutoMigrate tables + idempotent fixes in database/migrate.go)
-- is already accounted for, so the registry starts counting from here.
-- It re-runs NOTHING — purely a ledger insert.
--
-- Verify afterwards: SELECT * FROM schema_migrations;  → exactly 1 row.

INSERT INTO schema_migrations (version, applied_at, description)
VALUES ('20260913_baseline', NOW(), 'Baseline — all pre-existing GORM AutoMigrate schema accounted for')
ON CONFLICT (version) DO NOTHING;
