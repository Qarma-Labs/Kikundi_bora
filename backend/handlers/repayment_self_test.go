package handlers

import (
	"encoding/json"
	"testing"
	"time"

	"kikundibora/database"
	"kikundibora/models"

	"github.com/gofiber/fiber/v2"
	"github.com/shopspring/decimal"
)

// Self-service repayment suite (mirrors Weka Mchango): submit → PENDING
// (nothing moves) → treasurer approve (balance drops, schedule fills,
// zero closes) or reject with reason. Only mweka hazina may decide.

// disburseLoanTerms applies (30d), chain-approves and disburses; returns id.
func disburseLoanTerms(t *testing.T, app *fiber.App, a loanTermsActors, amount float64) string {
	loanID, _ := applyLoanTerms(t, app, a.borrower, a.memberID, amount, 30)
	appointAsha(t, app, a.chair)
	chainApproveAll(t, app, a, loanID)
	code, d := hPost(t, app, "/api/v1/loans/"+loanID+"/disburse", nil, a.treasurer)
	if code != 200 {
		t.Fatalf("disburse: %d %s", code, d)
	}
	return loanID
}

func submitRepayment(t *testing.T, app *fiber.App, token, loanID string, amount float64) (int, []byte) {
	return hPost(t, app, "/api/v1/loans/"+loanID+"/repayments", map[string]interface{}{
		"amount": amount, "proof_message": "Tigo Pesa MP123",
	}, token)
}

// 1. Own loan only: borrower submits (201 PENDING), stranger gets 403.
func TestSelfRepayOwnership(t *testing.T) {
	app := fullTestApp()
	cleanAndSeed(t)
	a := setupLoanTermsActors(t, app)
	fundTreasury(1000000)
	loanID := disburseLoanTerms(t, app, a, 100000)

	code, d := submitRepayment(t, app, a.borrower, loanID, 10000)
	if code != 201 {
		t.Fatalf("own submit: %d %s", code, d)
	}
	var r struct {
		Data models.Repayment `json:"data"`
	}
	json.Unmarshal(d, &r)
	if r.Data.Status != models.RepaymentPending {
		t.Errorf("new submission must be PENDING, got %s", r.Data.Status)
	}

	// Another member's loan → 403.
	stranger := hLogin(t, app, "asha@kikundi.tz", "demo123")
	code, d = submitRepayment(t, app, stranger, loanID, 10000)
	if code != 403 {
		t.Errorf("foreign submit: want 403, got %d %s", code, d)
	}
}

// 2. PENDING changes nothing: balance + schedule untouched until approval.
func TestSelfRepayPendingMovesNothing(t *testing.T) {
	app := fullTestApp()
	cleanAndSeed(t)
	a := setupLoanTermsActors(t, app)
	fundTreasury(1000000)
	loanID := disburseLoanTerms(t, app, a, 100000)
	before := loanByID(t, loanID)

	code, _ := submitRepayment(t, app, a.borrower, loanID, 20000)
	if code != 201 {
		t.Fatalf("submit: %d", code)
	}
	after := loanByID(t, loanID)
	if !after.BalanceRemaining.Equal(*before.BalanceRemaining) {
		t.Errorf("PENDING must not move balance: %s → %s",
			before.BalanceRemaining, after.BalanceRemaining)
	}
	var paid int64
	database.DB.Model(&models.LoanInstallment{}).
		Where("loan_id = ? AND paid_amount > 0", loanID).Count(&paid)
	if paid != 0 {
		t.Errorf("PENDING must not touch schedule, %d rows paid", paid)
	}
}

// 3. Approve reduces balance + fills oldest installment first.
func TestSelfRepayApproveAllocates(t *testing.T) {
	app := fullTestApp()
	cleanAndSeed(t)
	a := setupLoanTermsActors(t, app)
	fundTreasury(1000000)
	loanID := disburseLoanTerms(t, app, a, 90000)

	code, d := submitRepayment(t, app, a.borrower, loanID, 30000)
	if code != 201 {
		t.Fatalf("submit: %d %s", code, d)
	}
	var r struct {
		Data models.Repayment `json:"data"`
	}
	json.Unmarshal(d, &r)

	gid := getCurrentGroupID(t, app, a.treasurer)
	code, qd := hGet(t, app, "/api/v1/groups/"+gid+"/repayments?status=PENDING", a.treasurer)
	if code != 200 {
		t.Fatalf("queue: %d %s", code, qd)
	}
	var q struct {
		Data  []models.Repayment `json:"data"`
		Total int                `json:"total"`
	}
	json.Unmarshal(qd, &q)
	if q.Total != 1 {
		t.Fatalf("queue must list the pending repayment, got %d", q.Total)
	}

	code, ad := hPatch(t, app, "/api/v1/repayments/"+r.Data.ID+"/approve", a.treasurer, nil)
	if code != 200 {
		t.Fatalf("approve: %d %s", code, ad)
	}
	loan := loanByID(t, loanID)
	if !loan.BalanceRemaining.Equal(decimal.NewFromInt(60000)) {
		t.Errorf("balance must drop to 60000, got %s", loan.BalanceRemaining)
	}
	// Oldest-first: the 30000 lands on installment #1 (30d term = single
	// 90000 row) as a partial payment — status stays PENDING until covered.
	var first models.LoanInstallment
	database.DB.Where("loan_id = ? AND number = 1", loanID).First(&first)
	if !first.PaidAmount.Equal(decimal.NewFromInt(30000)) {
		t.Errorf("oldest installment must absorb 30000 first, got paid %s/%s",
			first.PaidAmount, first.TotalAmount)
	}
	if first.Status != models.InstallmentPending {
		t.Errorf("partially-paid installment must stay PENDING, got %s", first.Status)
	}
	// Cover the rest → row flips PAID.
	code, d = submitRepayment(t, app, a.borrower, loanID, 60000)
	if code != 201 {
		t.Fatalf("second submit: %d %s", code, d)
	}
	var r2 struct {
		Data models.Repayment `json:"data"`
	}
	json.Unmarshal(d, &r2)
	code, _ = hPatch(t, app, "/api/v1/repayments/"+r2.Data.ID+"/approve", a.treasurer, nil)
	if code != 200 {
		t.Fatalf("second approve: %d", code)
	}
	database.DB.Where("loan_id = ? AND number = 1", loanID).First(&first)
	if first.Status != models.InstallmentPaid {
		t.Errorf("covered installment must be PAID, got %s", first.Status)
	}
}

// 4. Reaching zero closes the loan (same rule as offsets/direct).
func TestSelfRepayApproveClosesAtZero(t *testing.T) {
	app := fullTestApp()
	cleanAndSeed(t)
	a := setupLoanTermsActors(t, app)
	fundTreasury(1000000)
	loanID := disburseLoanTerms(t, app, a, 40000)

	code, d := submitRepayment(t, app, a.borrower, loanID, 40000)
	if code != 201 {
		t.Fatalf("submit: %d %s", code, d)
	}
	var r struct {
		Data models.Repayment `json:"data"`
	}
	json.Unmarshal(d, &r)
	code, ad := hPatch(t, app, "/api/v1/repayments/"+r.Data.ID+"/approve", a.treasurer, nil)
	if code != 200 {
		t.Fatalf("approve: %d %s", code, ad)
	}
	loan := loanByID(t, loanID)
	if loan.Status != models.LoanClosed || !loan.BalanceRemaining.IsZero() {
		t.Errorf("loan must be CLOSED at zero, got %s / %s", loan.Status, loan.BalanceRemaining)
	}
}

// 5. Only hazina decides: chair/secretary get 403 on approve AND reject.
func TestSelfRepayTreasurerOnly(t *testing.T) {
	app := fullTestApp()
	cleanAndSeed(t)
	a := setupLoanTermsActors(t, app)
	fundTreasury(1000000)
	loanID := disburseLoanTerms(t, app, a, 50000)

	code, d := submitRepayment(t, app, a.borrower, loanID, 5000)
	if code != 201 {
		t.Fatalf("submit: %d %s", code, d)
	}
	var r struct {
		Data models.Repayment `json:"data"`
	}
	json.Unmarshal(d, &r)

	for _, tc := range []struct {
		name, tok, path string
	}{{"chair-approve", a.chair, "/api/v1/repayments/" + r.Data.ID + "/approve"},
		{"secretary-approve", a.secretary, "/api/v1/repayments/" + r.Data.ID + "/approve"},
		{"chair-reject", a.chair, "/api/v1/repayments/" + r.Data.ID + "/reject"},
		{"secretary-reject", a.secretary, "/api/v1/repayments/" + r.Data.ID + "/reject"}} {
		code, _ := hPatch(t, app, tc.path, tc.tok, map[string]string{"reason": "x"})
		if code != 403 {
			t.Errorf("%s: want 403, got %d", tc.name, code)
		}
	}
	// Still pending afterwards.
	var still models.Repayment
	database.DB.First(&still, "id = ?", r.Data.ID)
	if still.Status != models.RepaymentPending {
		t.Errorf("failed unauthorized attempts must leave PENDING, got %s", still.Status)
	}
}

// 6. Reject needs a reason; with one it voids without moving money.
func TestSelfRepayReject(t *testing.T) {
	app := fullTestApp()
	cleanAndSeed(t)
	a := setupLoanTermsActors(t, app)
	fundTreasury(1000000)
	loanID := disburseLoanTerms(t, app, a, 50000)
	before := loanByID(t, loanID)

	code, d := submitRepayment(t, app, a.borrower, loanID, 5000)
	if code != 201 {
		t.Fatalf("submit: %d %s", code, d)
	}
	var r struct {
		Data models.Repayment `json:"data"`
	}
	json.Unmarshal(d, &r)

	code, _ = hPatch(t, app, "/api/v1/repayments/"+r.Data.ID+"/reject", a.treasurer, nil)
	if code != 400 {
		t.Errorf("reject without reason: want 400, got %d", code)
	}
	code, rd := hPatch(t, app, "/api/v1/repayments/"+r.Data.ID+"/reject", a.treasurer, map[string]string{"reason": "Picha haionekani"})
	if code != 200 {
		t.Fatalf("reject: %d %s", code, rd)
	}
	after := loanByID(t, loanID)
	if !after.BalanceRemaining.Equal(*before.BalanceRemaining) {
		t.Errorf("REJECTED must not move balance")
	}
	_ = time.Now
}
