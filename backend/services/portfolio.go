package services

import (
	"time"

	"kikundibora/models"

	"github.com/shopspring/decimal"
)

// Loan portfolio aggregation — pure logic so totals are unit-testable.
// Per-loan term/interest come from the loan's snapshot columns; "repaid" is
// derived (total owed − remaining balance) and repayment history lives in
// the repayments table. Schedule-based overdue is applied by the handler
// (which has DB access); the due_date check here is the legacy fallback for
// loans disbursed before schedules existed.

// PortfolioLoan is one disbursed loan in the leadership portfolio view.
type PortfolioLoan struct {
	ID               string          `json:"id"`
	MemberID         string          `json:"member_id"`
	MemberNo         string          `json:"member_no"`
	FullName         string          `json:"full_name"`
	Principal        decimal.Decimal `json:"principal"`          // approved (or requested) amount disbursed
	AmountRepaid     decimal.Decimal `json:"amount_repaid"`      // total owed − remaining
	Outstanding      decimal.Decimal `json:"outstanding"`        // balance_remaining (interest-inclusive)
	Status           string          `json:"status"`             // OUTSTANDING | CLOSED
	IsOverdue        bool            `json:"is_overdue"`         // OUTSTANDING with a scheduled installment past due, unpaid
	DisbursedAt      string          `json:"disbursed_at,omitempty"`
	DueDate          string          `json:"due_date"`
	TermDays         int             `json:"term_days"`
	InterestEnabled  bool            `json:"interest_enabled"`
	InterestType     string          `json:"interest_type,omitempty"`
	InterestRate     decimal.Decimal `json:"interest_rate"`      // monthly % snapshot (0 when interest-free)
	InterestAmount   decimal.Decimal `json:"interest_amount"`
	TotalRepayment   decimal.Decimal `json:"total_repayment"`    // principal + interest
}

// LoanPortfolioSummary is the aggregate view over a set of disbursed loans.
type LoanPortfolioSummary struct {
	TotalDisbursed  decimal.Decimal  `json:"total_disbursed"`
	TotalRepaid     decimal.Decimal  `json:"total_repaid"`
	TotalOutstanding decimal.Decimal `json:"total_outstanding"`
	TotalOverdue    decimal.Decimal  `json:"total_overdue"`
	CountOutstanding int64           `json:"count_outstanding"`
	CountClosed     int64            `json:"count_closed"`
	CountOverdue    int64            `json:"count_overdue"`
	StatusCounts    map[string]int64 `json:"status_counts"`
	Loans           []PortfolioLoan  `json:"loans"`
}

// BuildLoanPortfolio aggregates disbursed loans (status OUTSTANDING or
// CLOSED with disbursed_at set) into summary stats + per-loan rows.
// Anything else (PENDING/APPROVED/REJECTED, never disbursed) is ignored.
func BuildLoanPortfolio(loans []models.Loan, today time.Time) LoanPortfolioSummary {
	sum := LoanPortfolioSummary{
		TotalDisbursed:   decimal.Zero,
		TotalRepaid:      decimal.Zero,
		TotalOutstanding: decimal.Zero,
		TotalOverdue:     decimal.Zero,
		StatusCounts:     map[string]int64{},
		Loans:            []PortfolioLoan{},
	}
	today = dateOf(today)

	for _, l := range loans {
		// Only actually-disbursed loans belong in the portfolio
		if l.DisbursedAt == nil ||
			(l.Status != models.LoanOutstanding && l.Status != models.LoanClosed) {
			continue
		}

		principal := l.Amount
		if l.ApprovedAmount != nil {
			principal = *l.ApprovedAmount
		}
		outstanding := decimal.Zero
		if l.BalanceRemaining != nil {
			outstanding = *l.BalanceRemaining
		}
		if outstanding.LessThan(decimal.Zero) {
			outstanding = decimal.Zero
		}
		// Total owed tracks the snapshot (principal + interest); legacy rows
		// predate the snapshot columns, so fall back to principal-only.
		totalOwed := l.TotalRepayment
		if totalOwed.LessThanOrEqual(decimal.Zero) {
			totalOwed = principal
		}
		repaid := totalOwed.Sub(outstanding)
		if repaid.LessThan(decimal.Zero) {
			repaid = decimal.Zero
		}

		isOverdue := l.Status == models.LoanOutstanding && l.DueDate.Before(today)

		sum.TotalDisbursed = sum.TotalDisbursed.Add(principal)
		sum.TotalRepaid = sum.TotalRepaid.Add(repaid)
		sum.TotalOutstanding = sum.TotalOutstanding.Add(outstanding)
		sum.StatusCounts[string(l.Status)]++

		switch l.Status {
		case models.LoanOutstanding:
			sum.CountOutstanding++
			if isOverdue {
				sum.CountOverdue++
				sum.TotalOverdue = sum.TotalOverdue.Add(outstanding)
			}
		case models.LoanClosed:
			sum.CountClosed++
		}

		item := PortfolioLoan{
			ID:              l.ID,
			MemberID:        l.MemberID,
			Principal:       principal,
			AmountRepaid:    repaid,
			Outstanding:     outstanding,
			Status:          string(l.Status),
			IsOverdue:       isOverdue,
			DueDate:         l.DueDate.Format("2006-01-02"),
			TermDays:        l.TermDays,
			InterestEnabled: l.InterestEnabled,
			InterestType:    l.InterestType,
			InterestRate:    l.ApplicableInterestRate,
			InterestAmount:  l.InterestAmount,
			TotalRepayment:  totalOwed,
		}
		if l.DisbursedAt != nil {
			item.DisbursedAt = l.DisbursedAt.Format("2006-01-02")
		}
		if l.Member != nil {
			item.MemberNo = l.Member.MemberNo
			item.FullName = l.Member.FullName
		}
		sum.Loans = append(sum.Loans, item)
	}
	return sum
}
