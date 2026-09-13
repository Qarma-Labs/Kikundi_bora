package migrations

import (
	"fmt"
	"log"
	"time"

	"kikundibora/database"

	"gorm.io/gorm"
)

// SchemaMigration is one row of the schema_migrations ledger: proof that a
// versioned migration ran to completion. The table itself is bootstrapped via
// GORM AutoMigrate (same tool as everything else — no new dependency).
type SchemaMigration struct {
	Version     string    `gorm:"primaryKey;type:varchar(100)" json:"version"`
	AppliedAt   time.Time `gorm:"not null" json:"applied_at"`
	Description string    `gorm:"type:text" json:"description"`
}

// Migration is one versioned schema change. Up holds the GORM AutoMigrate
// calls / raw idempotent Exec statements for that change. A nil Up means
// "manual bookkeeping step" (used exactly once: the v1 baseline, which a
// human inserts directly against production — the runner never creates it).
// Down is optional and only expected where reversal is trivial.
type Migration struct {
	Version     string
	Description string
	Up          func(db *gorm.DB) error
	Down        func(db *gorm.DB) error // optional, may be nil
}

// Migrations is the ordered registry. NEW schema changes are appended here
// as dated entries — never silently folded into AutoMigrate. Pre-existing
// schema stays under AutoMigrate, covered by the v1 baseline row.
var Migrations = []Migration{
	{
		Version:     "20260913_baseline",
		Description: "Baseline — all pre-existing GORM AutoMigrate schema accounted for",
		Up:          nil, // manual one-off INSERT (see README + BASELINE_PROD.sql)
	},
	{
		Version:     "20260913_loan_perf_indexes",
		Description: "Lookup indexes for loan installments, repayments status and loan due dates",
		Up:          upLoanPerfIndexes,
		Down:        nil, // indexes are safe to leave; drop manually if ever needed
	},
}

func upLoanPerfIndexes(db *gorm.DB) error {
	stmts := []string{
		`CREATE INDEX IF NOT EXISTS idx_loan_installments_loan_number ON loan_installments (loan_id, number)`,
		`CREATE INDEX IF NOT EXISTS idx_repayments_status ON repayments (status)`,
		`CREATE INDEX IF NOT EXISTS idx_loans_status_due ON loans (status, due_date)`,
	}
	for _, s := range stmts {
		if err := db.Exec(s).Error; err != nil {
			return fmt.Errorf("index stmt failed: %w", err)
		}
	}
	return nil
}

// ensureLedgerTable bootstraps the tracking table using the same GORM
// mechanism as the rest of the schema.
func ensureLedgerTable() error {
	return database.DB.AutoMigrate(&SchemaMigration{})
}

func isApplied(version string) (bool, error) {
	var n int64
	if err := database.DB.Model(&SchemaMigration{}).
		Where("version = ?", version).Count(&n).Error; err != nil {
		return false, err
	}
	return n > 0, nil
}

func recordApplied(m Migration) error {
	return database.DB.Create(&SchemaMigration{
		Version:     m.Version,
		AppliedAt:   time.Now(),
		Description: m.Description,
	}).Error
}

// Run applies every not-yet-applied migration in registry order, printing
// [APPLIED] / [SKIP] per entry. A failed Up is never recorded and stops the
// whole run loudly (callers exit non-zero — matches deploy `set -e`).
// The nil-Up baseline is NEVER auto-recorded: if its row is absent the
// runner prints the manual-step instruction and continues with later
// entries (which are self-contained and idempotent).
func Run() error {
	if err := ensureLedgerTable(); err != nil {
		return fmt.Errorf("schema_migrations bootstrap: %w", err)
	}
	for _, m := range Migrations {
		done, err := isApplied(m.Version)
		if err != nil {
			return fmt.Errorf("ledger read %s: %w", m.Version, err)
		}
		if done {
			log.Printf("MIGRATE: [SKIP] %s already applied — %s", m.Version, m.Description)
			continue
		}
		if m.Up == nil {
			log.Printf("MIGRATE: [!!] baseline %s NOT RECORDED — all pre-existing schema is assumed applied; record it manually when this matters (one-off, see backend/migrations/README.md + BASELINE_PROD.sql). Continuing.", m.Version)
			continue
		}
		log.Printf("MIGRATE: [RUN] %s — %s", m.Version, m.Description)
		if err := m.Up(database.DB); err != nil {
			return fmt.Errorf("MIGRATE: [FAIL] %s: %w", m.Version, err)
		}
		if err := recordApplied(m); err != nil {
			return fmt.Errorf("MIGRATE: [FAIL] %s ledger write: %w", m.Version, err)
		}
		log.Printf("MIGRATE: [APPLIED] %s — %s", m.Version, m.Description)
	}
	return nil
}

// AppliedVersions lists the ledger contents (newest last) for operators.
func AppliedVersions() ([]SchemaMigration, error) {
	if err := ensureLedgerTable(); err != nil {
		return nil, err
	}
	var rows []SchemaMigration
	if err := database.DB.Order("applied_at ASC").Find(&rows).Error; err != nil {
		return nil, err
	}
	return rows, nil
}
