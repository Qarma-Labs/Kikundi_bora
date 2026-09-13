package models

import (
	"gorm.io/gorm"
	"time"

	"github.com/shopspring/decimal"
)

type LoanStatus string

const (
	LoanPending     LoanStatus = "PENDING"
	LoanUnderReview LoanStatus = "UNDER_REVIEW"
	LoanApproved    LoanStatus = "APPROVED"
	LoanOutstanding LoanStatus = "OUTSTANDING"
	LoanRejected    LoanStatus = "REJECTED"
	LoanClosed      LoanStatus = "CLOSED"
)

type Loan struct {
	ID               string           `gorm:"primaryKey;type:uuid;default:gen_random_uuid()" json:"id"`
	MemberID         string           `gorm:"type:uuid;not null;index" json:"member_id"`
	ReviewedBy       *string          `gorm:"type:uuid" json:"reviewed_by,omitempty"`
	Amount           decimal.Decimal  `gorm:"type:decimal(15,2);not null" json:"amount"`
	ApprovedAmount   *decimal.Decimal `gorm:"type:decimal(15,2)" json:"approved_amount,omitempty"`
	BalanceRemaining *decimal.Decimal `gorm:"type:decimal(15,2)" json:"balance_remaining,omitempty"`

	// ---- Loan term + interest snapshot (muda + riba) --------------------
	// TermDays is the approved loan duration in days, anchored at
	// disbursement: the repayment schedule fits entirely within it and
	// DueDate == disbursement date + TermDays once disbursed.
	TermDays int `gorm:"not null;default:0" json:"term_days"`
	// InterestEnabled is snapshotted from the group's loan_settings at
	// application time. When false the loan is STRUCTURALLY interest-free:
	// no rate is stored, no interest calculated, none charged — verifiably
	// absent, not defaulted to zero.
	InterestEnabled bool `gorm:"not null;default:false" json:"interest_enabled"`
	// InterestType is "flat" or "reducing" (models.LoanInterest*), snapshotted
	// alongside the rate. Empty when InterestEnabled is false.
	InterestType string `gorm:"type:varchar(20);not null;default:''" json:"interest_type,omitempty"`
	// ApplicableInterestRate is the MONTHLY percentage snapshotted from group
	// settings at application time. Later group changes never alter it.
	// Always zero when InterestEnabled is false.
	ApplicableInterestRate decimal.Decimal `gorm:"type:decimal(7,2);not null;default:0" json:"applicable_interest_rate"`
	// InterestAmount is the total interest over the term (0 when interest-free).
	InterestAmount decimal.Decimal `gorm:"type:decimal(15,2);not null;default:0" json:"interest_amount"`
	// TotalRepayment = principal (approved) + InterestAmount. BalanceRemaining
	// tracks THIS total once disbursed, so offsets/repayments are always
	// interest-inclusive for interest-bearing loans.
	TotalRepayment  decimal.Decimal `gorm:"type:decimal(15,2);not null;default:0" json:"total_repayment"`
	Purpose         *string         `gorm:"type:text" json:"purpose,omitempty"`
	DueDate         time.Time       `gorm:"type:date;not null;index" json:"due_date"`
	Status          LoanStatus      `gorm:"type:varchar(20);not null;default:'PENDING';index" json:"status"`
	RejectionReason *string         `gorm:"type:text" json:"rejection_reason,omitempty"`
	AppliedAt       time.Time       `gorm:"autoCreateTime" json:"applied_at"`
	ReviewedAt      *time.Time      `json:"reviewed_at,omitempty"`
	DisbursedBy     *string         `gorm:"type:uuid" json:"disbursed_by,omitempty"`
	DisbursedAt     *time.Time      `json:"disbursed_at,omitempty"`
	DeletedAt       gorm.DeletedAt  `gorm:"index" json:"deleted_at,omitempty"`
	DeletedBy       *string         `gorm:"type:uuid" json:"deleted_by,omitempty"`
	UpdatedAt       time.Time       `gorm:"autoUpdateTime" json:"updated_at"`

	// Sequential approval — Hazina → Katibu → Bodi Member → Mwenyekiti (in order)
	HazinaApprovedBy     *string    `gorm:"type:uuid" json:"hazina_approved_by,omitempty"`
	HazinaApprovedAt     *time.Time `json:"hazina_approved_at,omitempty"`
	KatibuApprovedBy     *string    `gorm:"type:uuid" json:"katibu_approved_by,omitempty"`
	KatibuApprovedAt     *time.Time `json:"katibu_approved_at,omitempty"`
	BodiApprovedBy       *string    `gorm:"type:uuid" json:"bodi_approved_by,omitempty"`
	BodiApprovedAt       *time.Time `json:"bodi_approved_at,omitempty"`
	MwenyekitiApprovedBy *string    `gorm:"type:uuid" json:"mwenyekiti_approved_by,omitempty"`
	MwenyekitiApprovedAt *time.Time `json:"mwenyekiti_approved_at,omitempty"`

	// Borrower receipt confirmation — distinct from disbursement: OUTSTANDING
	// means hazina paid it out; BorrowerConfirmedAt means the borrower
	// acknowledges actually receiving the funds. Set via
	// PATCH /api/v1/loans/:id/confirm-received (borrower only).
	BorrowerConfirmedAt *time.Time `json:"borrower_confirmed_at,omitempty"`

	Member    *Member `gorm:"foreignKey:MemberID" json:"member,omitempty"`
	Reviewer  *User   `gorm:"foreignKey:ReviewedBy" json:"reviewer,omitempty"`
	Disburser *User   `gorm:"foreignKey:DisbursedBy" json:"disburser,omitempty"`
}

// Installment status.
const (
	InstallmentPending = "PENDING"
	InstallmentPaid    = "PAID"
)

// LoanInstallment is one row of a disbursed loan's auto-generated repayment
// schedule. The whole schedule fits within the loan's TermDays measured from
// disbursement: installment i is due on disbursement + i*interval, the last
// exactly on the term end. Interest-free loans carry zero interest component;
// the sum of TotalAmount across rows equals the loan's TotalRepayment.
type LoanInstallment struct {
	ID      string    `gorm:"primaryKey;type:uuid;default:gen_random_uuid()" json:"id"`
	LoanID  string    `gorm:"type:uuid;not null;index" json:"loan_id"`
	Number  int       `gorm:"not null" json:"number"` // 1..N in due order
	DueDate time.Time `gorm:"type:date;not null;index" json:"due_date"`

	PrincipalAmount decimal.Decimal `gorm:"type:decimal(15,2);not null" json:"principal_amount"`
	InterestAmount  decimal.Decimal `gorm:"type:decimal(15,2);not null;default:0" json:"interest_amount"`
	TotalAmount     decimal.Decimal `gorm:"type:decimal(15,2);not null" json:"total_amount"`

	// PaidAmount tracks allocation from recorded repayments (oldest-due first).
	PaidAmount decimal.Decimal `gorm:"type:decimal(15,2);not null;default:0" json:"paid_amount"`
	Status     string          `gorm:"type:varchar(20);not null;default:'PENDING';index" json:"status"`

	CreatedAt time.Time `gorm:"autoCreateTime" json:"created_at"`
	UpdatedAt time.Time `gorm:"autoUpdateTime" json:"updated_at"`

	Loan *Loan `gorm:"foreignKey:LoanID" json:"loan,omitempty"`
}

// Self-service repayment lifecycle (mirrors MemberContribution).
const (
	RepaymentPending   = "PENDING"
	RepaymentConfirmed = "CONFIRMED"
	RepaymentRejected  = "REJECTED"
)

type Repayment struct {
	ID              string          `gorm:"primaryKey;type:uuid;default:gen_random_uuid()" json:"id"`
	LoanID          string          `gorm:"type:uuid;not null;index" json:"loan_id"`
	MemberID        string          `gorm:"type:uuid;not null;index" json:"member_id"`
	RecordedBy      string          `gorm:"type:uuid;not null" json:"recorded_by"`
	Amount          decimal.Decimal `gorm:"type:decimal(15,2);not null" json:"amount"`
	BalanceAfter    decimal.Decimal `gorm:"type:decimal(15,2);not null" json:"balance_after"`
	PaidAt          time.Time       `gorm:"type:date;not null" json:"paid_at"`
	PaymentMethod   string          `gorm:"type:varchar(20);not null;default:'CASH'" json:"payment_method"`
	ReferenceNumber string          `gorm:"type:varchar(100)" json:"reference_number,omitempty"`
	ReceiptURL      string          `gorm:"type:varchar(500)" json:"receipt_url,omitempty"`
	Notes           *string         `gorm:"type:text" json:"notes,omitempty"`
	// Self-service lifecycle (mirrors MemberContribution): member submissions
	// start PENDING and touch nothing until the treasurer approves. Direct
	// treasurer recordings are CONFIRMED immediately (legacy behavior).
	Status        string         `gorm:"type:varchar(20);not null;default:'CONFIRMED';index" json:"status"`
	SubmittedBy   *string        `gorm:"type:uuid" json:"submitted_by,omitempty"`
	SubmittedAt   *time.Time     `json:"submitted_at,omitempty"`
	ProofImageURL string         `gorm:"type:varchar(500)" json:"proof_image_url,omitempty"`
	ProofMessage  *string        `gorm:"type:text" json:"proof_message,omitempty"`
	ReviewedBy    *string        `gorm:"type:uuid" json:"reviewed_by,omitempty"`
	ReviewedAt    *time.Time     `json:"reviewed_at,omitempty"`
	ReviewReason  *string        `gorm:"type:text" json:"review_reason,omitempty"`
	DeletedAt     gorm.DeletedAt `gorm:"index" json:"deleted_at,omitempty"`
	DeletedBy     *string        `gorm:"type:uuid" json:"deleted_by,omitempty"`
	CreatedAt     time.Time      `gorm:"autoCreateTime" json:"created_at"`

	Loan     *Loan   `gorm:"foreignKey:LoanID" json:"loan,omitempty"`
	Member   *Member `gorm:"foreignKey:MemberID" json:"member,omitempty"`
	Recorder *User   `gorm:"foreignKey:RecordedBy" json:"recorder,omitempty"`
}
