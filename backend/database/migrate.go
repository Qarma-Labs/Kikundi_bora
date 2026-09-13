package database

import (
	"log"

	"kikundibora/models"
)

func AutoMigrate() {
	// Enable pgcrypto extension for gen_random_uuid()
	log.Println("Migration: enabling pgcrypto extension...")
	DB.Exec("CREATE EXTENSION IF NOT EXISTS pgcrypto")

	// First pass: create all tables without foreign key constraints (safe — won't drop existing).
	log.Println("Migration: creating/migrating tables (pass 1 — structure only)...")
	DB.Config.DisableForeignKeyConstraintWhenMigrating = true
	err := DB.AutoMigrate(
		// Core auth / user tables
		&models.User{},
		&models.UserSession{},
		&models.FailedLogin{},
		&models.UserApproval{},
		&models.AdminLog{},
		&models.UserPosition{},
		&models.LeadershipPosition{},
		&models.MemberContribution{},
		&models.PendingAction{},
		&models.Group{},
		&models.GroupSettingProposal{},
		&models.LoanSettings{},
		&models.LoanInstallment{},
		&models.FineSettings{},
		&models.Fine{},
		&models.FineOffenceType{},
		&models.ContributionCycle{},
		&models.Meeting{},
		&models.MeetingAttendance{},
		&models.PaymentMethod{},

		// Member & financial tables
		&models.Member{},
		&models.Contribution{},
		&models.ContributionEdit{},
		&models.Loan{},
		&models.Repayment{},

		// Loan committee tables
		&models.LoanCommitteeMember{},
		&models.LoanReview{},
		&models.LoanOffsetTransaction{},

		// Welfare (Mfuko wa Kijamii) tables
		&models.WelfareEvent{},
		&models.WelfareContribution{},

		// System tables
		&models.AuditLog{},
		&models.Notification{},
		&models.NotificationSMSPref{},
		&models.OTPChallenge{},

		// Dissolution tables
		&models.GroupDissolutionProposal{},
		&models.DissolutionVote{},
		&models.DissolutionPayout{},

		// Backup tables
		&models.BackupHistory{},
		&models.BackupSettings{},
	)
	if err != nil {
		panic("Failed to auto-migrate (pass 1): " + err.Error())
	}
	DB.Config.DisableForeignKeyConstraintWhenMigrating = false

	// Second pass: add foreign key constraints.
	log.Println("Migration: adding foreign key constraints (pass 2)...")
	addFKConstraints()

	// Fines idempotency, take 2: the old single unique index on
	// (group, member, cycle) predates offence types and would wrongly block
	// multiple offences per cycle. Replace with partial uniques.
	DB.Exec(`DROP INDEX IF EXISTS idx_fines_group_member_cycle`)
	DB.Exec(`CREATE UNIQUE INDEX IF NOT EXISTS idx_fines_cycle_occurrence
		ON fines (group_id, member_id, offence_type_id, contribution_cycle_label)
		WHERE contribution_cycle_label <> ''`)
	DB.Exec(`CREATE UNIQUE INDEX IF NOT EXISTS idx_fines_event_occurrence
		ON fines (group_id, member_id, offence_type_id, occurrence_date)
		WHERE contribution_cycle_label = ''`)
	DB.Exec(`CREATE UNIQUE INDEX IF NOT EXISTS idx_contribution_cycles_member_cycle
		ON contribution_cycles (group_id, member_id, cycle_label)`)
	DB.Exec(`CREATE UNIQUE INDEX IF NOT EXISTS idx_dissolution_votes_proposal_member
		ON dissolution_votes (proposal_id, member_id) WHERE deleted_at IS NULL`)

	// Backfill: payment methods created before the approval workflow
	// carry an empty status — treat them as approved (they were live).
	DB.Model(&models.PaymentMethod{}).
		Where("status = '' OR status IS NULL").
		Update("status", models.PaymentMethodApproved)

	// Data-integrity fix (Sept 2026): users.must_change_password was created
	// with DB DEFAULT true while seed code passes false. GORM omits
	// zero-valued fields that carry a `default:` tag, so every seeded user
	// silently stored true and was forced through "set your password" after
	// each reseed. The model tag is now default:false; fix the live default
	// too (AutoMigrate never alters existing column defaults).
	DB.Exec(`ALTER TABLE users ALTER COLUMN must_change_password SET DEFAULT FALSE`)

	log.Println("Migration complete.")
}

// addFKConstraints creates FK constraints that GORM may skip when
// DisableForeignKeyConstraintWhenMigrating is used.
func addFKConstraints() {
	type fkDef struct {
		table      string
		constraint string
		column     string
		refTable   string
		refColumn  string
		onDelete   string
	}

	fks := []fkDef{
		// UserSession → User
		{"user_sessions", "fk_user_sessions_user", "user_id", "users", "id", "CASCADE"},
		// UserApproval → User (two FKs)
		{"user_approvals", "fk_user_approvals_user", "user_id", "users", "id", "RESTRICT"},
		{"user_approvals", "fk_user_approvals_approver", "approved_by", "users", "id", "RESTRICT"},
		// AdminLog → User (two FKs)
		{"admin_logs", "fk_admin_logs_admin", "admin_id", "users", "id", "RESTRICT"},
		{"admin_logs", "fk_admin_logs_target_user", "target_user_id", "users", "id", "SET NULL"},
		// UserPosition → User
		{"user_positions", "fk_user_positions_user", "user_id", "users", "id", "RESTRICT"},
		// PendingAction → User (two FKs)
		{"pending_actions", "fk_pending_actions_requester", "requested_by", "users", "id", "RESTRICT"},
		{"pending_actions", "fk_pending_actions_approver", "approved_by", "users", "id", "SET NULL"},
		// Member → User (three FKs — approved_by points at the katibu's
		// USER row, never at members)
		{"members", "fk_members_registered_by", "registered_by", "users", "id", "RESTRICT"},
		{"members", "fk_members_user", "user_id", "users", "id", "SET NULL"},
		{"members", "fk_members_approver", "approved_by", "users", "id", "SET NULL"},
		// Contribution → Member, User
		{"contributions", "fk_contributions_member", "member_id", "members", "id", "RESTRICT"},
		{"contributions", "fk_contributions_recorder", "recorded_by", "users", "id", "RESTRICT"},
		{"contributions", "fk_contributions_confirmer", "confirmed_by", "users", "id", "SET NULL"},
		// ContributionEdit → Contribution, User
		{"contribution_edits", "fk_contribution_edits_contribution", "contribution_id", "contributions", "id", "RESTRICT"},
		{"contribution_edits", "fk_contribution_edits_editor", "edited_by", "users", "id", "RESTRICT"},
		// Loan → Member, User
		{"loans", "fk_loans_member", "member_id", "members", "id", "RESTRICT"},
		{"loans", "fk_loans_reviewer", "reviewed_by", "users", "id", "SET NULL"},
		{"loans", "fk_loans_disburser", "disbursed_by", "users", "id", "SET NULL"},
		// LoanInstallment → Loan
		{"loan_installments", "fk_loan_installments_loan", "loan_id", "loans", "id", "CASCADE"},
		// LoanSettings → Group
		{"loan_settings", "fk_loan_settings_group", "group_id", "groups", "id", "CASCADE"},
		// Repayment → Loan, Member, User
		{"repayments", "fk_repayments_loan", "loan_id", "loans", "id", "RESTRICT"},
		{"repayments", "fk_repayments_member", "member_id", "members", "id", "RESTRICT"},
		{"repayments", "fk_repayments_recorder", "recorded_by", "users", "id", "RESTRICT"},
		// LoanCommitteeMember → User (two FKs)
		{"loan_committee_members", "fk_loan_committee_members_user", "user_id", "users", "id", "RESTRICT"},
		{"loan_committee_members", "fk_loan_committee_members_appointer", "appointed_by", "users", "id", "RESTRICT"},
		// LoanReview → Loan, User
		{"loan_reviews", "fk_loan_reviews_loan", "loan_id", "loans", "id", "RESTRICT"},
		{"loan_reviews", "fk_loan_reviews_reviewer", "reviewer_id", "users", "id", "RESTRICT"},
		// LoanOffsetTransaction → Loan, Member, Users
		{"loan_offset_transactions", "fk_loan_offsets_loan", "loan_id", "loans", "id", "RESTRICT"},
		{"loan_offset_transactions", "fk_loan_offsets_member", "member_id", "members", "id", "RESTRICT"},
		{"loan_offset_transactions", "fk_loan_offsets_proposer", "proposed_by", "users", "id", "RESTRICT"},
		{"loan_offset_transactions", "fk_loan_offsets_approver", "approved_by", "users", "id", "SET NULL"},
		{"loan_offset_transactions", "fk_loan_offsets_executor", "executed_by", "users", "id", "SET NULL"},
		// WelfareEvent → Member, User
		{"welfare_events", "fk_welfare_events_member", "member_id", "members", "id", "RESTRICT"},
		{"welfare_events", "fk_welfare_events_creator", "created_by", "users", "id", "RESTRICT"},
		{"welfare_events", "fk_welfare_events_approver", "approved_by", "users", "id", "SET NULL"},
		{"welfare_events", "fk_welfare_events_rejector", "rejected_by", "users", "id", "SET NULL"},
		// WelfareContribution → WelfareEvent, Member, User
		{"welfare_contributions", "fk_welfare_contributions_event", "event_id", "welfare_events", "id", "RESTRICT"},
		{"welfare_contributions", "fk_welfare_contributions_member", "member_id", "members", "id", "RESTRICT"},
		{"welfare_contributions", "fk_welfare_contributions_recorder", "recorded_by", "users", "id", "SET NULL"},
		// AuditLog → User
		{"audit_logs", "fk_audit_logs_user", "user_id", "users", "id", "SET NULL"},
		// Notification → User
		{"notifications", "fk_notifications_user", "user_id", "users", "id", "CASCADE"},
		// LeadershipPosition → Member
		{"leadership_positions", "fk_leadership_positions_member", "member_id", "members", "id", "CASCADE"},
		// FineSettings → Group
		{"fine_settings", "fk_fine_settings_group", "group_id", "groups", "id", "CASCADE"},
		// FineOffenceType → Group
		{"fine_offence_types", "fk_offence_types_group", "group_id", "groups", "id", "CASCADE"},
		// ContributionCycle → Group, Member
		{"contribution_cycles", "fk_cycles_group", "group_id", "groups", "id", "CASCADE"},
		{"contribution_cycles", "fk_cycles_member", "member_id", "members", "id", "CASCADE"},
		// Meeting → Group
		{"meetings", "fk_meetings_group", "group_id", "groups", "id", "CASCADE"},
		// MeetingAttendance → Meeting, Member
		{"meeting_attendances", "fk_attendance_meeting", "meeting_id", "meetings", "id", "CASCADE"},
		{"meeting_attendances", "fk_attendance_member", "member_id", "members", "id", "CASCADE"},
		// Fine → Group, Member, User
		{"fines", "fk_fines_group", "group_id", "groups", "id", "CASCADE"},
		{"fines", "fk_fines_member", "member_id", "members", "id", "RESTRICT"},
		{"fines", "fk_fines_collector", "collected_by", "users", "id", "SET NULL"},
		{"fines", "fk_fines_waiver", "waived_by", "users", "id", "SET NULL"},
		{"fines", "fk_fines_offence", "offence_type_id", "fine_offence_types", "id", "RESTRICT"},
	}

	for _, fk := range fks {
		// Verify-and-heal: a constraint with the right name but the WRONG
		// definition (e.g. fk_members_approver pointing at members(id)
		// instead of users(id), which made katibu approval impossible with
		// an FK violation on every attempt) is dropped and recreated
		// correctly. Unmanaged constraints are never touched.
		if def, found := ExistingFKDef(fk.table, fk.constraint); found {
			if def.RefTable == fk.refTable && def.Column == fk.column &&
				def.RefColumn == fk.refColumn && def.OnDelete == fk.onDelete {
				continue // already correct
			}
			log.Printf("Migration: updating FK %s.%s (%s) — recreating with correct definition",
				fk.table, fk.constraint, def.Describe())
			if err := DB.Exec(`ALTER TABLE "` + fk.table + `" DROP CONSTRAINT "` + fk.constraint + `"`).Error; err != nil {
				log.Printf("FK %s.%s drop failed: %v", fk.table, fk.constraint, err)
				continue
			}
		}

		sql := `ALTER TABLE "` + fk.table + `" ADD CONSTRAINT "` + fk.constraint + `"` +
			` FOREIGN KEY ("` + fk.column + `")` +
			` REFERENCES "` + fk.refTable + `" ("` + fk.refColumn + `")` +
			` ON DELETE ` + fk.onDelete

		if err := DB.Exec(sql).Error; err != nil {
			log.Printf("FK %s.%s: %v", fk.table, fk.constraint, err)
		}
	}
}

// FkDefActual is the live definition of a single-column FK constraint.
type FkDefActual struct {
	RefTable  string
	Column    string
	RefColumn string
	OnDelete  string // CASCADE | RESTRICT | SET NULL | NO ACTION | SET DEFAULT
}

func (d FkDefActual) Describe() string {
	return "FOREIGN KEY (" + d.Column + ") REFERENCES " + d.RefTable + "(" + d.RefColumn + ") ON DELETE " + d.OnDelete
}

// ExistingFKDef returns the live definition of a named FK on a table.
// found=false when no such constraint exists. Only single-column FKs are
// compared (every entry in our managed list is single-column).
func ExistingFKDef(table, constraint string) (def FkDefActual, found bool) {
	var row struct {
		RefTable  string
		Column    string
		RefColumn string
		DelType   string
	}
	err := DB.Raw(`
		SELECT c.confrelid::regclass::text AS ref_table,
		       a.attname AS column,
		       af.attname AS ref_column,
		       c.confdeltype AS del_type
		  FROM pg_constraint c
		  JOIN pg_attribute a  ON a.attrelid = c.conrelid AND a.attnum = c.conkey[1]
		  JOIN pg_attribute af ON af.attrelid = c.confrelid AND af.attnum = c.confkey[1]
		 WHERE c.conname = ? AND c.contype = 'f' AND c.conrelid = ?::regclass
		   AND array_length(c.conkey, 1) = 1`,
		constraint, table).Scan(&row).Error
	if err != nil || row.RefTable == "" {
		return FkDefActual{}, false
	}
	delMap := map[string]string{"c": "CASCADE", "r": "RESTRICT", "n": "SET NULL", "a": "NO ACTION", "d": "SET DEFAULT"}
	onDelete, ok := delMap[row.DelType]
	if !ok {
		onDelete = row.DelType
	}
	return FkDefActual{RefTable: row.RefTable, Column: row.Column, RefColumn: row.RefColumn, OnDelete: onDelete}, true
}
