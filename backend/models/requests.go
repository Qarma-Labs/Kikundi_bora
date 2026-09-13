package models

import (
	"time"

	"github.com/shopspring/decimal"
)

type LoginRequest struct {
	Email    string `json:"email" validate:"required"`
	Password string `json:"password" validate:"required"`
}

type RegisterRequest struct {
	Name     string `json:"name" validate:"required,min=2"`
	Email    string `json:"email" validate:"omitempty,email"`
	Phone    string `json:"phone" validate:"required"`
	Password string `json:"password" validate:"required,min=8"`
	Role     Role   `json:"role" validate:"required,oneof=chair treasurer secretary member"`
}

type AuthResponse struct {
	Token              string `json:"token"`
	User               *User  `json:"user"`
	ExpiresAt          string `json:"expires_at"`
	FirstLoginRequired bool   `json:"first_login_required,omitempty"`
}

// MeResponse extends User with member context and leadership roles for dual-plane UI.
type MeResponse struct {
	*User
	MemberID   *string  `json:"member_id,omitempty"`
	MemberCode *string  `json:"member_code,omitempty"`
	Leadership []string `json:"leadership"`
}

type UpdateProfileRequest struct {
	Name      *string `json:"name" validate:"omitempty,min=2,max=100"`
	Phone     *string `json:"phone" validate:"omitempty,min=9,max=15"`
	AvatarURL *string `json:"avatar_url" validate:"omitempty,url,max=500"`
	Bio       *string `json:"bio" validate:"omitempty,max=500"`
}

type ResetPasswordRequest struct {
	Email       string `json:"email" validate:"required,email"`
	NewPassword string `json:"new_password" validate:"required,min=8"`
}

type CreateMemberRequest struct {
	FullName       string  `json:"full_name" validate:"required,min=2"`
	Phone          string  `json:"phone" validate:"required"`
	Address        *string `json:"address"`
	Gender         *string `json:"gender" validate:"omitempty,oneof=MME MKE"`
	Occupation     *string `json:"occupation"`
	Email          *string `json:"email" validate:"omitempty,email"`
	NextOfKinName  *string `json:"next_of_kin_name"`
	NextOfKinPhone *string `json:"next_of_kin_phone"`
	PhotoURL       *string `json:"photo_url"`
	JoinedAt       string  `json:"joined_at" validate:"required"`
	// Selective arrears backdating (mwenyekiti only, main michango cycles
	// only — never social funds). When true, arrears rows are generated
	// from BackdateFromCycle up to now using the group's fixed amount.
	BackdateArrears   *bool   `json:"backdate_arrears"`
	BackdateFromCycle *string `json:"backdate_from_cycle"`
}

type ApproveMemberRequest struct{}

type RejectMemberRequest struct {
	Reason string `json:"reason" validate:"required,min=3"`
}

type UpdateMemberRequest struct {
	FullName *string `json:"full_name" validate:"omitempty,min=2,max=150"`
	Phone    *string `json:"phone" validate:"omitempty,min=9,max=15"`
	Address  *string `json:"address" validate:"omitempty,max=500"`
	IsActive *bool   `json:"is_active"`
}

type CreateContributionRequest struct {
	MemberID        string          `json:"member_id" validate:"required"`
	Amount          decimal.Decimal `json:"amount" validate:"required,gt=0"`
	Month           string          `json:"month" validate:"required"`
	PaidAt          string          `json:"paid_at" validate:"required"`
	PaymentMethod   string          `json:"payment_method" validate:"required,oneof=CASH BANK MOBILE_MONEY"`
	ReferenceNumber string          `json:"reference_number"`
	ReceiptURL      string          `json:"receipt_url"`
	Notes           *string         `json:"notes"`
}

type EditContributionRequest struct {
	NewAmount decimal.Decimal `json:"new_amount" validate:"required,gt=0"`
	Reason    string          `json:"reason" validate:"required"`
}

type ApplyLoanRequest struct {
	MemberID string          `json:"member_id" validate:"required"`
	Amount   decimal.Decimal `json:"amount" validate:"required,gt=0"`
	Purpose  *string         `json:"purpose"`
	DueDate  string          `json:"due_date"`
	// TermDays is the requested loan duration in days. When provided it is
	// validated against the group's loan-settings window and DueDate is
	// derived from it. When omitted, the term is derived from DueDate
	// (legacy clients) and validated the same way.
	TermDays *int `json:"term_days" validate:"omitempty,gt=0"`
}

type ApproveLoanRequest struct {
	ApprovedAmount decimal.Decimal `json:"approved_amount" validate:"required,gt=0"`
}

type RejectLoanRequest struct {
	Reason string `json:"reason" validate:"required"`
}

type DashboardSummary struct {
	TotalActiveMembers          int64           `json:"total_active_members"`
	TotalContributions          decimal.Decimal `json:"total_contributions"`
	TotalLoansIssued            decimal.Decimal `json:"total_loans_issued"`
	TotalRepayments             decimal.Decimal `json:"total_repayments"`
	TotalOutstanding            decimal.Decimal `json:"total_outstanding_balance"`
	CountOutstandingLoans       int64           `json:"count_outstanding_loans"`
	CountPendingLoans           int64           `json:"count_pending_loans"`
	MembersPaidThisMonth        int64           `json:"members_paid_this_month"`
	MembersDefaulted            int64           `json:"members_defaulted_this_month"`
	TotalContributionsThisMonth decimal.Decimal `json:"total_contributions_this_month"`
	TotalRepaymentsThisMonth    decimal.Decimal `json:"total_repayments_this_month"`
}

type MessageResponse struct {
	Message string      `json:"message"`
	Data    interface{} `json:"data,omitempty"`
}

type MonthlyContributionRow struct {
	MemberNo   string          `json:"member_no"`
	FullName   string          `json:"full_name"`
	Phone      string          `json:"phone"`
	AmountPaid decimal.Decimal `json:"amount_paid"`
	PaidAt     *string         `json:"paid_at,omitempty"`
	Status     string          `json:"status"`
	Notes      *string         `json:"notes,omitempty"`
}

type OutstandingLoanRow struct {
	MemberNo         string          `json:"member_no"`
	FullName         string          `json:"full_name"`
	Phone            string          `json:"phone"`
	LoanID           string          `json:"loan_id"`
	ApprovedAmount   decimal.Decimal `json:"approved_amount"`
	BalanceRemaining decimal.Decimal `json:"balance_remaining"`
	AmountPaidSoFar  decimal.Decimal `json:"amount_paid_so_far"`
	DueDate          string          `json:"due_date"`
	Urgency          string          `json:"urgency"`
	DaysRemaining    int             `json:"days_remaining"`
}

type PaginationQuery struct {
	Page  int `query:"page"`
	Limit int `query:"limit"`
}

func (p *PaginationQuery) GetOffset() int {
	if p.Page <= 0 {
		p.Page = 1
	}
	if p.Limit <= 0 {
		p.Limit = 20
	}
	if p.Limit > 500 {
		p.Limit = 500
	}
	return (p.Page - 1) * p.Limit
}

type MemberNoResponse struct {
	MemberID string `json:"member_id"`
	MemberNo string `json:"member_no"`
}

type RepaymentResponse struct {
	RepaymentID  string          `json:"repayment_id"`
	BalanceAfter decimal.Decimal `json:"balance_after"`
	LoanClosed   bool            `json:"loan_closed"`
}

type NotificationReadRequest struct {
	IDs []string `json:"ids"`
}

// Loan Committee request types

type AppointCommitteeMemberRequest struct {
	UserID string `json:"user_id" validate:"required"`
}

type SubmitLoanReviewRequest struct {
	Decision string  `json:"decision" validate:"required,oneof=APPROVE REJECT"`
	Comments *string `json:"comments"`
}

// Loan Committee response types

type LoanCommitteeMemberResponse struct {
	ID          string  `json:"id"`
	UserID      string  `json:"user_id"`
	UserName    string  `json:"user_name"`
	UserEmail   string  `json:"user_email"`
	UserRole    string  `json:"user_role"`
	AppointedBy *string `json:"appointed_by,omitempty"`
	AppointedAt string  `json:"appointed_at"`
	IsActive    bool    `json:"is_active"`
}

type LoanReviewResponse struct {
	ID           string  `json:"id"`
	LoanID       string  `json:"loan_id"`
	ReviewerID   string  `json:"reviewer_id"`
	ReviewerName string  `json:"reviewer_name"`
	Decision     string  `json:"decision"`
	Comments     *string `json:"comments,omitempty"`
	ReviewedAt   *string `json:"reviewed_at,omitempty"`
}

type LoanCommitteeDashboard struct {
	PendingReviews   int64 `json:"pending_reviews"`
	LoansUnderReview int64 `json:"loans_under_review"`
	ApprovedLoans    int64 `json:"approved_loans"`
	RejectedLoans    int64 `json:"rejected_loans"`
	MyReviews        int64 `json:"my_reviews"`
	CommitteeMembers int64 `json:"committee_members"`
}

type LoanCommitteeHistoryRow struct {
	LoanID        string          `json:"loan_id"`
	ApplicantName string          `json:"applicant_name"`
	MemberNo      string          `json:"member_no"`
	Amount        decimal.Decimal `json:"amount"`
	Status        string          `json:"status"`
	ReviewedBy    string          `json:"reviewed_by"`
	Decision      string          `json:"decision"`
	Comments      *string         `json:"comments,omitempty"`
	ReviewedAt    string          `json:"reviewed_at"`
}

type CommitteeActivityReport struct {
	TotalReviews         int64                        `json:"total_reviews"`
	ApprovalRate         float64                      `json:"approval_rate"`
	RejectionRate        float64                      `json:"rejection_rate"`
	ReviewsByMember      []CommitteeMemberReviewCount `json:"reviews_by_member"`
	CommitteeComposition []CommitteeCompositionEntry  `json:"committee_composition"`
	ReviewHistory        []LoanCommitteeHistoryRow    `json:"review_history"`
}

type CommitteeMemberReviewCount struct {
	UserID     string `json:"user_id"`
	UserName   string `json:"user_name"`
	Reviews    int64  `json:"reviews"`
	Approvals  int64  `json:"approvals"`
	Rejections int64  `json:"rejections"`
}

type CommitteeCompositionEntry struct {
	UserID      string `json:"user_id"`
	UserName    string `json:"user_name"`
	Role        string `json:"role"`
	AppointedAt string `json:"appointed_at"`
	Type        string `json:"type"` // "automatic" or "appointed"
}

type DateRange struct {
	From time.Time `query:"from"`
	To   time.Time `query:"to"`
}

// Welfare module request types

type CreateWelfareEventRequest struct {
	MemberID        string          `json:"member_id" validate:"required"`
	EventType       string          `json:"event_type" validate:"required,oneof=MSIBA HARUSI DHARURA MATIBABU KUZALIWA ELIMU"`
	Description     string          `json:"description" validate:"required"`
	AmountRequested decimal.Decimal `json:"amount_requested" validate:"required,gt=0"`
	FundingSource   string          `json:"funding_source" validate:"required,oneof=TREASURY MEMBER_CONTRIBUTION BOTH"`
	TreasuryAmount  decimal.Decimal `json:"treasury_amount"`
	MemberAmount    decimal.Decimal `json:"member_amount"`
}

type ApproveWelfareEventRequest struct {
	ApprovedAmount decimal.Decimal `json:"approved_amount" validate:"required,gt=0"`
}

type RejectWelfareEventRequest struct {
	Reason string `json:"reason" validate:"required"`
}

type RecordWelfarePaymentRequest struct {
	Amount decimal.Decimal `json:"amount" validate:"required,gt=0"`
}

// User Management request types

type CreateUserRequest struct {
	FullName string `json:"full_name" validate:"required,min=2"`
	Phone    string `json:"phone" validate:"required"`
	Role     Role   `json:"role"`
}

type ApproveUserRequest struct {
	Remarks string `json:"remarks"`
}

type RejectUserRequest struct {
	Remarks string `json:"remarks"`
}

type FirstLoginSetupRequest struct {
	NewPassword     string `json:"new_password" validate:"required,min=8"`
	ConfirmPassword string `json:"confirm_password" validate:"required"`
}

// Admin request types

type AdminOverrideRequest struct {
	Action string `json:"action" validate:"required,oneof=activate deactivate suspend"`
	Reason string `json:"reason"`
}

type AdminResetPasswordRequest struct {
	NewPassword string `json:"new_password"`
}

// User Management response types

type UserManagementDashboard struct {
	PendingUsers   int64 `json:"pending_users"`
	ActiveUsers    int64 `json:"active_users"`
	RejectedUsers  int64 `json:"rejected_users"`
	SuspendedUsers int64 `json:"suspended_users"`
	TotalUsers     int64 `json:"total_users"`
}

// Welfare module dashboard types

type WelfareDashboard struct {
	TotalEvents            int64           `json:"total_events"`
	PendingApproval        int64           `json:"pending_approval"`
	ActiveEvents           int64           `json:"active_events"`
	CompletedEvents        int64           `json:"completed_events"`
	RejectedEvents         int64           `json:"rejected_events"`
	TotalCollected         decimal.Decimal `json:"total_collected"`
	TotalFromTreasury      decimal.Decimal `json:"total_from_treasury"`
	MyPendingContributions int64           `json:"my_pending_contributions"`
	MyPaidContributions    int64           `json:"my_paid_contributions"`
}

type WelfareEventSummary struct {
	ID              string          `json:"id"`
	EventType       string          `json:"event_type"`
	Description     string          `json:"description"`
	MemberName      string          `json:"member_name"`
	MemberNo        string          `json:"member_no"`
	AmountRequested decimal.Decimal `json:"amount_requested"`
	AmountApproved  decimal.Decimal `json:"amount_approved"`
	FundingSource   string          `json:"funding_source"`
	Status          string          `json:"status"`
	CreatedAt       string          `json:"created_at"`
}

type WelfareContributionSummary struct {
	ID         string          `json:"id"`
	EventID    string          `json:"event_id"`
	EventType  string          `json:"event_type"`
	EventDesc  string          `json:"event_desc"`
	MemberID   string          `json:"member_id"`
	MemberName string          `json:"member_name"`
	MemberNo   string          `json:"member_no"`
	Amount     decimal.Decimal `json:"amount"`
	Status     string          `json:"status"`
	PaidAt     *string         `json:"paid_at,omitempty"`
}
