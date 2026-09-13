package models

import (
	"gorm.io/gorm"
	"time"

	"github.com/shopspring/decimal"
)

// Loan interest modes.
const (
	LoanInterestFlat     = "flat"     // riba ya moja kwa moja: principal × rate × miezi
	LoanInterestReducing = "reducing" // salio linalopungua: riba ya mwezi kwenye deni lililobaki
)

// LoanSettings is the approved, group-scoped loan policy. Mutated ONLY when
// a GroupSettingProposal of kind "loan" is approved by the Katibu — the same
// propose → katibu-approve queue used for contribution and fine settings.
//
// Interest-free lending (interest_enabled=false) is a FIRST-CLASS mode, not
// a "rate=0" edge case: when disabled, the entire loan flow must structurally
// skip interest logic — no rate is snapshotted, no interest is calculated,
// and no interest field is shown or charged anywhere.
type LoanSettings struct {
	ID      string `gorm:"primaryKey;type:uuid;default:gen_random_uuid()" json:"id"`
	GroupID string `gorm:"type:uuid;not null;uniqueIndex" json:"group_id"`

	InterestEnabled bool `gorm:"not null;default:false" json:"interest_enabled"`
	// DefaultInterestRate is a MONTHLY percentage (e.g. 5.00 = 5% per month).
	// Only meaningful when InterestEnabled is true.
	DefaultInterestRate decimal.Decimal `gorm:"type:decimal(7,2);not null;default:0" json:"default_interest_rate"`
	// InterestType is "flat" or "reducing" (see constants above).
	InterestType string `gorm:"type:varchar(20);not null;default:'flat'" json:"interest_type"`

	// Allowed loan term window, in days. Applications outside [min, max]
	// are rejected at submission.
	MinTermDays int `gorm:"not null;default:14" json:"min_term_days"`
	MaxTermDays int `gorm:"not null;default:365" json:"max_term_days"`

	UpdatedBy *string `gorm:"type:uuid" json:"updated_by,omitempty"`

	DeletedAt gorm.DeletedAt `gorm:"index" json:"deleted_at,omitempty"`
	DeletedBy *string        `gorm:"type:uuid" json:"deleted_by,omitempty"`
	CreatedAt time.Time      `gorm:"autoCreateTime" json:"created_at"`
	UpdatedAt time.Time      `gorm:"autoUpdateTime" json:"updated_at"`

	Group *Group `gorm:"foreignKey:GroupID" json:"group,omitempty"`
}

// IsValidLoanInterestType reports whether t is a supported interest mode.
func IsValidLoanInterestType(t string) bool {
	return t == LoanInterestFlat || t == LoanInterestReducing
}
