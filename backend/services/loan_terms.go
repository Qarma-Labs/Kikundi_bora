package services

import (
	"time"

	"kikundibora/models"

	"github.com/shopspring/decimal"
)

// Loan-terms math — pure functions (no DB) so interest, snapshots and the
// repayment schedule are unit-testable in isolation.
//
// Rate convention: DefaultInterestRate / ApplicableInterestRate is a MONTHLY
// percentage (e.g. 5.00 = 5% per month), documented on the model. Schedule
// frequency is monthly-ish: n = max(1, round(termDays/30)) installments
// spaced termDays/n days apart, so every due date falls ON or BEFORE the
// term end and the last installment lands exactly on it.

// MonthsForTerm converts a term in days to a monthly installment count.
func MonthsForTerm(termDays int) int {
	if termDays <= 0 {
		return 1
	}
	n := (termDays + 15) / 30 // round(termDays/30), min 1
	if n < 1 {
		n = 1
	}
	return n
}

// CalcLoanInterest returns the total interest for principal over termDays at
// a monthly percentage rate. When enabled=false it structurally returns zero
// WITHOUT touching the rate — interest-free is a mode, not rate=0.
func CalcLoanInterest(principal decimal.Decimal, monthlyRatePct decimal.Decimal, termDays int, interestType string, enabled bool) decimal.Decimal {
	if !enabled {
		return decimal.Zero
	}
	if principal.LessThanOrEqual(decimal.Zero) || termDays <= 0 {
		return decimal.Zero
	}
	if monthlyRatePct.LessThanOrEqual(decimal.Zero) {
		return decimal.Zero
	}
	n := MonthsForTerm(termDays)
	r := monthlyRatePct.Div(decimal.NewFromInt(100))

	switch interestType {
	case models.LoanInterestReducing:
		// Amortized equal installments at monthly rate r over n periods:
		// total = n*payment − principal.
		if r.IsZero() {
			return decimal.Zero
		}
		one := decimal.NewFromInt(1)
		pow := one.Add(r).Pow(decimal.NewFromInt(int64(n)))
		payment := principal.Mul(r).Mul(pow).Div(pow.Sub(one))
		total := payment.Mul(decimal.NewFromInt(int64(n)))
		interest := total.Sub(principal)
		if interest.LessThan(decimal.Zero) {
			return decimal.Zero
		}
		return interest.Round(2)
	default: // flat
		return principal.Mul(r).Mul(decimal.NewFromInt(int64(n))).Round(2)
	}
}

// ScheduledInstallment is one generated row (amounts only; the handler
// persists LoanInstallment records from these).
type ScheduledInstallment struct {
	Number    int
	DueDate   time.Time
	Principal decimal.Decimal
	Interest  decimal.Decimal
	Total     decimal.Decimal
}

// BuildLoanSchedule generates n monthly-ish installments for principal over
// termDays starting at disbursement. All due dates fall within
// [disbursement+1d, disbursement+termDays]; the last is exactly the term end.
// Rounding dust (cents) is pushed into the LAST installment so the column
// sums equal principal / interest / total exactly.
func BuildLoanSchedule(principal decimal.Decimal, monthlyRatePct decimal.Decimal, termDays int, interestType string, enabled bool, disbursedAt time.Time) []ScheduledInstallment {
	n := MonthsForTerm(termDays)
	day := disbursedAt.Truncate(24 * time.Hour)
	if day.IsZero() {
		day = time.Now().Truncate(24 * time.Hour)
	}

	totalInterest := CalcLoanInterest(principal, monthlyRatePct, termDays, interestType, enabled)

	// Per-installment splits.
	principalParts := splitEvenly(principal, n)
	interestParts := splitEvenly(totalInterest, n)

	// Reducing-balance: interest front-loaded per amortization; recompute the
	// exact per-period interest so the schedule mirrors the quoted total.
	if enabled && interestType == models.LoanInterestReducing && n > 1 && totalInterest.GreaterThan(decimal.Zero) {
		r := monthlyRatePct.Div(decimal.NewFromInt(100))
		remaining := principal
		acc := decimal.Zero
		for i := 0; i < n; i++ {
			ip := remaining.Mul(r).Round(2)
			if i == n-1 {
				ip = totalInterest.Sub(acc)
			}
			interestParts[i] = ip
			acc = acc.Add(ip)
			pp := principalParts[i]
			if pp.GreaterThan(remaining) {
				pp = remaining
				principalParts[i] = pp
			}
			remaining = remaining.Sub(pp)
		}
	}

	out := make([]ScheduledInstallment, 0, n)
	for i := 0; i < n; i++ {
		var due time.Time
		if i == n-1 {
			due = day.AddDate(0, 0, termDays) // exactly the term end
		} else {
			// Even spacing across the term: termDays/n per step.
			due = day.Add(time.Duration(float64(termDays) / float64(n) * float64(i+1) * float64(24*time.Hour))).Truncate(24 * time.Hour)
			termEnd := day.AddDate(0, 0, termDays)
			if !due.Before(termEnd) {
				due = termEnd.AddDate(0, 0, -(n - 1 - i))
			}
		}
		out = append(out, ScheduledInstallment{
			Number:    i + 1,
			DueDate:   due,
			Principal: principalParts[i],
			Interest:  interestParts[i],
			Total:     principalParts[i].Add(interestParts[i]),
		})
	}
	return out
}

// splitEvenly divides amount into n parts (2dp) with dust in the last part.
func splitEvenly(amount decimal.Decimal, n int) []decimal.Decimal {
	parts := make([]decimal.Decimal, n)
	if n <= 0 {
		return parts
	}
	if amount.IsZero() {
		return parts
	}
	each := amount.Div(decimal.NewFromInt(int64(n))).Round(2)
	acc := decimal.Zero
	for i := 0; i < n; i++ {
		if i == n-1 {
			parts[i] = amount.Sub(acc)
		} else {
			parts[i] = each
			acc = acc.Add(each)
		}
	}
	return parts
}

// ScheduleTotal sums installment totals (== principal + interest by construction).
func ScheduleTotal(sched []ScheduledInstallment) decimal.Decimal {
	t := decimal.Zero
	for _, s := range sched {
		t = t.Add(s.Total)
	}
	return t
}
