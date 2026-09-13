package services

import (
	"testing"
	"time"

	"github.com/shopspring/decimal"
)

func TestCalcLoanInterestFreeIsStructural(t *testing.T) {
	got := CalcLoanInterest(decimal.NewFromInt(100000), decimal.NewFromInt(99), 365, "flat", false)
	if !got.IsZero() {
		t.Fatalf("disabled mode must return zero without touching the rate, got %s", got)
	}
}

func TestCalcLoanInterestFlat(t *testing.T) {
	// 100000 × 5%/month × 2 months (60 days) = 10000.
	got := CalcLoanInterest(decimal.NewFromInt(100000), decimal.NewFromInt(5), 60, "flat", true)
	if !got.Equal(decimal.NewFromInt(10000)) {
		t.Fatalf("flat: want 10000, got %s", got)
	}
}

func TestCalcLoanInterestReducingLessThanFlat(t *testing.T) {
	flat := CalcLoanInterest(decimal.NewFromInt(120000), decimal.NewFromInt(5), 90, "flat", true)
	red := CalcLoanInterest(decimal.NewFromInt(120000), decimal.NewFromInt(5), 90, "reducing", true)
	if red.LessThanOrEqual(decimal.Zero) || red.GreaterThanOrEqual(flat) {
		t.Fatalf("reducing (%s) must be positive and below flat (%s)", red, flat)
	}
}

func TestBuildLoanScheduleConfinedAndSumming(t *testing.T) {
	day := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	for _, term := range []int{30, 90, 100, 365} {
		sched := BuildLoanSchedule(decimal.NewFromInt(100000), decimal.NewFromInt(5), term, "flat", true, day)
		if len(sched) == 0 {
			t.Fatalf("term %d: empty schedule", term)
		}
		end := day.AddDate(0, 0, term)
		for _, s := range sched {
			if s.DueDate.After(end) {
				t.Fatalf("term %d: installment %d due %s past term end %s", term, s.Number, s.DueDate, end)
			}
		}
		if got := sched[len(sched)-1].DueDate.Format("2006-01-02"); got != end.Format("2006-01-02") {
			t.Fatalf("term %d: last due %s must equal term end %s", term, got, end.Format("2006-01-02"))
		}
		total := ScheduleTotal(sched)
		want := decimal.NewFromInt(100000).Add(CalcLoanInterest(decimal.NewFromInt(100000), decimal.NewFromInt(5), term, "flat", true))
		if !total.Equal(want) {
			t.Fatalf("term %d: schedule total %s must equal %s", term, total, want)
		}
	}
}

func TestBuildLoanScheduleFreeHasNoInterest(t *testing.T) {
	day := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	sched := BuildLoanSchedule(decimal.NewFromInt(50000), decimal.NewFromInt(10), 90, "flat", false, day)
	for _, s := range sched {
		if !s.Interest.IsZero() {
			t.Fatalf("interest-free schedule must carry zero interest, got %s", s.Interest)
		}
	}
	if total := ScheduleTotal(sched); !total.Equal(decimal.NewFromInt(50000)) {
		t.Fatalf("interest-free total must equal principal, got %s", total)
	}
}
