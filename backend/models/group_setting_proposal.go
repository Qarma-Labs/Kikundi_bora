package models

import (
	"gorm.io/gorm"
	"time"

	"github.com/shopspring/decimal"
)

// Proposal status constants
const (
	ProposalPending  = "PENDING"
	ProposalApproved = "APPROVED"
	ProposalRejected = "REJECTED"
)

// ProposalKind distinguishes which settings a GroupSettingProposal mutates.
// The same propose → katibu-approve queue is reused (one PENDING per group).
const (
	ProposalKindContribution = "contribution"
	ProposalKindFines        = "fines"
	ProposalKindLoan         = "loan"
)

// GroupSettingProposal is a change to group contribution OR fine settings
// proposed by the Mwenyekiti (chair). It only takes effect when the Katibu
// (secretary) approves it. Only one PENDING proposal may exist per group.
type GroupSettingProposal struct {
	ID                      string               `gorm:"primaryKey;type:uuid;default:gen_random_uuid()" json:"id"`
	GroupID                 string               `gorm:"type:uuid;not null;index" json:"group_id"`
	ProposalKind            string               `gorm:"type:varchar(20);not null;default:'contribution';index" json:"proposal_kind"`
	ContributionInterval    ContributionInterval `gorm:"type:varchar(20);not null" json:"contribution_interval"`
	ContributionDueDate     *string              `gorm:"type:varchar(10)" json:"contribution_due_date,omitempty"`
	FixedContributionAmount *decimal.Decimal     `gorm:"type:decimal(15,2)" json:"fixed_contribution_amount,omitempty"`

	// Fine-settings payload (kind = fines). Nil on contribution proposals.
	FinesEnabled    *bool            `json:"fines_enabled,omitempty"`
	FineType        *string          `gorm:"type:varchar(20)" json:"fine_type,omitempty"`
	FineAmount      *decimal.Decimal `gorm:"type:decimal(15,2)" json:"fine_amount,omitempty"`
	FinePercentage  *decimal.Decimal `gorm:"type:decimal(7,2)" json:"fine_percentage,omitempty"`
	GracePeriodDays *int             `json:"grace_period_days,omitempty"`

	// Loan-settings payload (kind = loan). Nil on other proposal kinds.
	LoanInterestEnabled       *bool            `json:"loan_interest_enabled,omitempty"`
	LoanDefaultInterestRate   *decimal.Decimal `gorm:"type:decimal(7,2)" json:"loan_default_interest_rate,omitempty"`
	LoanInterestType          *string          `gorm:"type:varchar(20)" json:"loan_interest_type,omitempty"`
	LoanMinTermDays           *int             `json:"loan_min_term_days,omitempty"`
	LoanMaxTermDays           *int             `json:"loan_max_term_days,omitempty"`

	Status          string  `gorm:"type:varchar(20);not null;default:'PENDING';index" json:"status"`
	ProposedBy      string  `gorm:"type:uuid;not null" json:"proposed_by"`
	ApprovedBy      *string `gorm:"type:uuid" json:"approved_by,omitempty"`
	RejectionReason *string `gorm:"type:text" json:"rejection_reason,omitempty"`

	DeletedAt gorm.DeletedAt `gorm:"index" json:"deleted_at,omitempty"`

	DeletedBy *string `gorm:"type:uuid" json:"deleted_by,omitempty"`

	CreatedAt  time.Time  `gorm:"autoCreateTime" json:"created_at"`
	UpdatedAt  time.Time  `gorm:"autoUpdateTime" json:"updated_at"`
	ReviewedAt *time.Time `json:"reviewed_at,omitempty"`

	Group    *Group `gorm:"foreignKey:GroupID" json:"group,omitempty"`
	Proposer *User  `gorm:"foreignKey:ProposedBy" json:"proposer,omitempty"`
	Approver *User  `gorm:"foreignKey:ApprovedBy" json:"approver,omitempty"`
}
