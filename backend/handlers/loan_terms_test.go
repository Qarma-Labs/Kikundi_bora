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

// Loan-terms suite: group loan-settings (propose/approve), term + interest
// snapshot at application, schedule confined to the term, schedule-based
// overdue, and interest-aware offset. Interest-free groups must never see or
// be charged interest anywhere in the flow.

type loanTermsActors struct {
	chair     string
	treasurer string
	secretary string
	borrower  string
	memberID  string
}

func setupLoanTermsActors(t *testing.T, app *fiber.App) loanTermsActors {
	chair := hLogin(t, app, "juma@kikundi.tz", "demo123")
	treasurer := hLogin(t, app, "fatuma@kikundi.tz", "demo123")
	secretary := hLogin(t, app, "rashidi@kikundi.tz", "demo123")

	// Neutral borrower outside the approval chain.
	email := "muda@kikundi.tz"
	hash, _ := hashTestPassword("demo123")
	u := models.User{
		Name: "Muda Neutral", Email: &email, Phone: "0717777777",
		Password: string(hash), Role: models.RoleMember,
		Status: models.UserStatusActive, IsActive: true,
	}
	if err := database.DB.Create(&u).Error; err != nil {
		t.Fatalf("create borrower user: %v", err)
	}
	m := models.Member{
		MemberNo: "M-TERMS-01", FullName: "Muda Neutral", Phone: "0717777777",
		UserID: &u.ID, IsActive: true, ApprovalStatus: "approved",
		JoinedAt: time.Now(), RegisteredBy: u.ID,
	}
	if err := database.DB.Create(&m).Error; err != nil {
		t.Fatalf("create borrower member: %v", err)
	}
	borrower := hLogin(t, app, email, "demo123")
	return loanTermsActors{chair: chair, treasurer: treasurer, secretary: secretary, borrower: borrower, memberID: m.ID}
}

func getCurrentGroupID(t *testing.T, app *fiber.App, token string) string {
	// fullTestApp does not mount /groups/current — resolve via the database.
	g, err := database.GetCurrentGroup()
	if err != nil {
		t.Fatalf("current group: %v", err)
	}
	return g.ID
}

func proposeLoanSettings(t *testing.T, app *fiber.App, gid, chair string, body map[string]interface{}) {
	code, d := hPost(t, app, "/api/v1/groups/"+gid+"/loan-settings/propose", body, chair)
	if code != 201 {
		t.Fatalf("propose loan-settings: %d %s", code, d)
	}
}

func approveLoanSettings(t *testing.T, app *fiber.App, gid, secretary string) {
	code, d := hPost(t, app, "/api/v1/groups/"+gid+"/loan-settings/approve", map[string]interface{}{}, secretary)
	if code != 200 {
		t.Fatalf("approve loan-settings: %d %s", code, d)
	}
}

// applyLoanTerms applies for a loan with an explicit term; returns the loan id + raw payload.
func applyLoanTerms(t *testing.T, app *fiber.App, token, memberID string, amount float64, termDays int) (string, []byte) {
	code, d := hPost(t, app, "/api/v1/loans/apply", map[string]interface{}{
		"member_id": memberID, "amount": amount, "purpose": "MudaTest", "term_days": termDays,
	}, token)
	if code != 201 {
		t.Fatalf("apply: %d %s", code, d)
	}
	return hExtract(t, d, "data"), d
}

// chainApproveAll signs every parallel slot (treasurer, secretary, bodi,
// chair). The borrower is neutral so no self-sign is involved.
func chainApproveAll(t *testing.T, app *fiber.App, a loanTermsActors, loanID string) {
	bodiTok := hLogin(t, app, "asha@kikundi.tz", "demo123")
	for _, tc := range []struct {
		name string
		tok  string
	}{{"treasurer", a.treasurer}, {"secretary", a.secretary}, {"bodi", bodiTok}, {"chair", a.chair}} {
		code, d := hPost(t, app, "/api/v1/uongozi/mikopo/"+loanID+"/approve", map[string]interface{}{}, tc.tok)
		if code != 200 {
			t.Fatalf("chain %s approve: %d %s", tc.name, code, d)
		}
	}
}

// appointAsha puts asha on the loan board (needed for the bodi slot).
func appointAsha(t *testing.T, app *fiber.App, chair string) {
	code, _ := hPost(t, app, "/api/v1/loan-committee/members", map[string]interface{}{
		"user_id": hGetUserID(t, "asha@kikundi.tz"),
	}, chair)
	if code != 200 && code != 201 && code != 409 {
		t.Fatalf("appoint asha: %d", code)
	}
}

func loanByID(t *testing.T, id string) models.Loan {
	var loan models.Loan
	if err := database.DB.First(&loan, "id = ?", id).Error; err != nil {
		t.Fatalf("load loan: %v", err)
	}
	return loan
}

// 1. Interest-free group: no rate accepted/shown, schedule has zero interest,
// total == principal exactly.
func TestLoanTermsInterestFree(t *testing.T) {
	app := fullTestApp()
	cleanAndSeed(t)
	a := setupLoanTermsActors(t, app)
	fundTreasury(1000000)

	// Default settings are interest-free (enabled=false).
	gid := getCurrentGroupID(t, app, a.chair)
	code, d := hGet(t, app, "/api/v1/groups/"+gid+"/loan-settings", a.borrower)
	if code != 200 {
		t.Fatalf("get loan-settings: %d %s", code, d)
	}
	var s struct {
		Data struct {
			InterestEnabled bool `json:"interest_enabled"`
		} `json:"data"`
	}
	json.Unmarshal(d, &s)
	if s.Data.InterestEnabled {
		t.Fatal("default group must be interest-free")
	}

	loanID, _ := applyLoanTerms(t, app, a.borrower, a.memberID, 120000, 90)
	loan := loanByID(t, loanID)
	if loan.InterestEnabled {
		t.Error("interest-free loan must snapshot interest_enabled=false")
	}
	if !loan.ApplicableInterestRate.IsZero() || !loan.InterestAmount.IsZero() {
		t.Errorf("interest-free loan must carry zero rate/amount, got %s / %s",
			loan.ApplicableInterestRate, loan.InterestAmount)
	}
	if !loan.TotalRepayment.Equal(loan.Amount) {
		t.Errorf("interest-free total must equal principal: %s vs %s", loan.TotalRepayment, loan.Amount)
	}

	appointAsha(t, app, a.chair)
	chainApproveAll(t, app, a, loanID)
	code, d = hPost(t, app, "/api/v1/loans/"+loanID+"/disburse", nil, a.treasurer)
	if code != 200 {
		t.Fatalf("disburse: %d %s", code, d)
	}
	var insts []models.LoanInstallment
	database.DB.Where("loan_id = ?", loanID).Order("number ASC").Find(&insts)
	if len(insts) == 0 {
		t.Fatal("disbursement must generate a schedule")
	}
	sum := decimal.Zero
	intSum := decimal.Zero
	for _, in := range insts {
		sum = sum.Add(in.TotalAmount)
		intSum = intSum.Add(in.InterestAmount)
	}
	if !intSum.IsZero() {
		t.Errorf("interest-free schedule must have zero interest component, got %s", intSum)
	}
	if !sum.Equal(loan.TotalRepayment) {
		t.Errorf("schedule total %s must equal loan total %s", sum, loan.TotalRepayment)
	}
	// Confined to the 90-day term.
	loan = loanByID(t, loanID)
	for _, in := range insts {
		if in.DueDate.After(loan.DueDate) {
			t.Errorf("installment %d due %s exceeds term end %s", in.Number, in.DueDate, loan.DueDate)
		}
	}
}

// 2. Snapshot: rate at application time sticks; later group changes (5%→10%)
// do not alter existing loans, new loans take the new rate.
func TestLoanTermsRateSnapshot(t *testing.T) {
	app := fullTestApp()
	cleanAndSeed(t)
	a := setupLoanTermsActors(t, app)
	fundTreasury(2000000)
	gid := getCurrentGroupID(t, app, a.chair)

	proposeLoanSettings(t, app, gid, a.chair, map[string]interface{}{
		"interest_enabled": true, "default_interest_rate": 5.0,
		"interest_type": "flat", "min_term_days": 30, "max_term_days": 365,
	})
	approveLoanSettings(t, app, gid, a.secretary)

	loan1, _ := applyLoanTerms(t, app, a.borrower, a.memberID, 100000, 60)
	l1 := loanByID(t, loan1)
	if !l1.InterestEnabled || !l1.ApplicableInterestRate.Equal(decimal.NewFromInt(5)) {
		t.Fatalf("expected 5%% snapshot, got enabled=%v rate=%s", l1.InterestEnabled, l1.ApplicableInterestRate)
	}
	// flat 5%/month × 2 months × 100000 = 10000.
	if !l1.InterestAmount.Equal(decimal.NewFromInt(10000)) {
		t.Errorf("expected interest 10000, got %s", l1.InterestAmount)
	}

	// Change group to 10%.
	proposeLoanSettings(t, app, gid, a.chair, map[string]interface{}{
		"interest_enabled": true, "default_interest_rate": 10.0,
		"interest_type": "flat", "min_term_days": 30, "max_term_days": 365,
	})
	approveLoanSettings(t, app, gid, a.secretary)

	l1again := loanByID(t, loan1)
	if !l1again.ApplicableInterestRate.Equal(decimal.NewFromInt(5)) {
		t.Errorf("existing loan rate must stay 5%%, got %s", l1again.ApplicableInterestRate)
	}
	// Second application needs a different borrower (one active loan each).
	email2 := "muda2@kikundi.tz"
	hash2, _ := hashTestPassword("demo123")
	u2 := models.User{
		Name: "Muda Pili", Email: &email2, Phone: "0717777778",
		Password: string(hash2), Role: models.RoleMember,
		Status: models.UserStatusActive, IsActive: true,
	}
	if err := database.DB.Create(&u2).Error; err != nil {
		t.Fatalf("create borrower2 user: %v", err)
	}
	m2 := models.Member{
		MemberNo: "M-TERMS-02", FullName: "Muda Pili", Phone: "0717777778",
		UserID: &u2.ID, IsActive: true, ApprovalStatus: "approved",
		JoinedAt: time.Now(), RegisteredBy: u2.ID,
	}
	if err := database.DB.Create(&m2).Error; err != nil {
		t.Fatalf("create borrower2 member: %v", err)
	}
	borrower2 := hLogin(t, app, email2, "demo123")
	loan2, _ := applyLoanTerms(t, app, borrower2, m2.ID, 50000, 30)
	l2 := loanByID(t, loan2)
	if !l2.ApplicableInterestRate.Equal(decimal.NewFromInt(10)) {
		t.Errorf("new loan must take 10%%, got %s", l2.ApplicableInterestRate)
	}
}

// 3. Term outside [min, max] is rejected.
func TestLoanTermsRangeRejected(t *testing.T) {
	app := fullTestApp()
	cleanAndSeed(t)
	a := setupLoanTermsActors(t, app)
	fundTreasury(1000000)

	for _, term := range []int{10, 400} {
		code, d := hPost(t, app, "/api/v1/loans/apply", map[string]interface{}{
			"member_id": a.memberID, "amount": 50000.0, "term_days": term,
		}, a.borrower)
		if code != 400 {
			t.Errorf("term %d: want 400, got %d %s", term, code, d)
		}
	}
}

// 4. Schedule duration never exceeds the term (incl. uneven terms).
func TestLoanTermsScheduleConfined(t *testing.T) {
	app := fullTestApp()
	cleanAndSeed(t)
	a := setupLoanTermsActors(t, app)
	fundTreasury(1000000)
	gid := getCurrentGroupID(t, app, a.chair)
	proposeLoanSettings(t, app, gid, a.chair, map[string]interface{}{
		"interest_enabled": true, "default_interest_rate": 5.0,
		"interest_type": "reducing", "min_term_days": 7, "max_term_days": 365,
	})
	approveLoanSettings(t, app, gid, a.secretary)

	loanID, _ := applyLoanTerms(t, app, a.borrower, a.memberID, 90000, 100)
	appointAsha(t, app, a.chair)
	chainApproveAll(t, app, a, loanID)
	code, d := hPost(t, app, "/api/v1/loans/"+loanID+"/disburse", nil, a.treasurer)
	if code != 200 {
		t.Fatalf("disburse: %d %s", code, d)
	}
	loan := loanByID(t, loanID)
	var insts []models.LoanInstallment
	database.DB.Where("loan_id = ?", loanID).Order("number ASC").Find(&insts)
	if len(insts) == 0 {
		t.Fatal("no schedule generated")
	}
	for _, in := range insts {
		if in.DueDate.After(loan.DueDate) {
			t.Errorf("installment %d due %s exceeds term end %s", in.Number, in.DueDate, loan.DueDate)
		}
	}
	last := insts[len(insts)-1]
	if last.DueDate.Format("2006-01-02") != loan.DueDate.Format("2006-01-02") {
		t.Errorf("last installment %s must land exactly on term end %s", last.DueDate, loan.DueDate)
	}
	sum := decimal.Zero
	for _, in := range insts {
		sum = sum.Add(in.TotalAmount)
	}
	if !sum.Equal(loan.TotalRepayment) {
		t.Errorf("schedule total %s must equal %s", sum, loan.TotalRepayment)
	}
}

// 5. Overdue keys off schedule due dates.
func TestLoanTermsScheduleOverdue(t *testing.T) {
	app := fullTestApp()
	cleanAndSeed(t)
	a := setupLoanTermsActors(t, app)
	fundTreasury(1000000)

	loanID, _ := applyLoanTerms(t, app, a.borrower, a.memberID, 60000, 90)
	appointAsha(t, app, a.chair)
	chainApproveAll(t, app, a, loanID)
	code, _ := hPost(t, app, "/api/v1/loans/"+loanID+"/disburse", nil, a.treasurer)
	if code != 200 {
		t.Fatalf("disburse: %d", code)
	}
	// Fresh schedule: nothing past due yet.
	if loanScheduleOverdue(loanID, time.Now()) {
		t.Error("fresh schedule must not be overdue")
	}
	// Age the first installment past due, unpaid.
	past := time.Now().AddDate(0, 0, -5).Truncate(24 * time.Hour)
	database.DB.Model(&models.LoanInstallment{}).
		Where("loan_id = ? AND number = 1", loanID).Update("due_date", past)
	if !loanScheduleOverdue(loanID, time.Now()) {
		t.Error("unpaid past-due installment must flag overdue")
	}
	// Pay it off → no longer overdue.
	database.DB.Exec(`UPDATE loan_installments SET paid_amount = total_amount, status = 'PAID'
		WHERE loan_id = ? AND number = 1`, loanID)
	if loanScheduleOverdue(loanID, time.Now()) {
		t.Error("fully-paid past-due installment must not flag overdue")
	}
}

// 6. Offset reflects the interest-inclusive outstanding balance.
func TestLoanTermsOffsetInterestAware(t *testing.T) {
	app := fullTestApp()
	cleanAndSeed(t)
	a := setupLoanTermsActors(t, app)
	fundTreasury(1000000)
	gid := getCurrentGroupID(t, app, a.chair)
	proposeLoanSettings(t, app, gid, a.chair, map[string]interface{}{
		"interest_enabled": true, "default_interest_rate": 10.0,
		"interest_type": "flat", "min_term_days": 30, "max_term_days": 365,
	})
	approveLoanSettings(t, app, gid, a.secretary)

	// Borrower needs savings: record a treasurer contribution for them.
	var borrowerMember models.Member
	database.DB.Where("id = ?", a.memberID).First(&borrowerMember)
	code, d := hPost(t, app, "/api/v1/contributions", map[string]interface{}{
		"member_id": a.memberID, "amount": 200000.0, "month": time.Now().Format("2006-01"),
		"paid_at": time.Now().Format("2006-01-02"), "payment_method": "CASH",
	}, a.treasurer)
	if code != 201 && code != 200 {
		t.Fatalf("fund borrower savings: %d %s", code, d)
	}

	loanID, _ := applyLoanTerms(t, app, a.borrower, a.memberID, 100000, 30)
	appointAsha(t, app, a.chair)
	chainApproveAll(t, app, a, loanID)
	code, _ = hPost(t, app, "/api/v1/loans/"+loanID+"/disburse", nil, a.treasurer)
	if code != 200 {
		t.Fatalf("disburse: %d", code)
	}
	loan := loanByID(t, loanID)
	// flat 10% × 1 month × 100000 = 10000 → total 110000.
	if !loan.TotalRepayment.Equal(decimal.NewFromInt(110000)) {
		t.Fatalf("expected total 110000, got %s", loan.TotalRepayment)
	}
	// Force overdue via the schedule, then preview the offset.
	past := time.Now().AddDate(0, 0, -5).Truncate(24 * time.Hour)
	database.DB.Model(&models.LoanInstallment{}).
		Where("loan_id = ?", loanID).Update("due_date", past)
	code, d = hGet(t, app, "/api/v1/loans/"+loanID+"/offset-preview", a.treasurer)
	if code != 200 {
		t.Fatalf("offset-preview: %d %s", code, d)
	}
	var p struct {
		Data struct {
			Eligible        bool            `json:"eligible"`
			Outstanding     decimal.Decimal `json:"outstanding"`
			InterestEnabled bool            `json:"interest_enabled"`
			InterestAmount  decimal.Decimal `json:"interest_amount"`
			TotalRepayment  decimal.Decimal `json:"total_repayment"`
		} `json:"data"`
	}
	json.Unmarshal(d, &p)
	if !p.Data.Eligible {
		t.Fatalf("expected eligible offset: %s", d)
	}
	if !p.Data.Outstanding.Equal(decimal.NewFromInt(110000)) {
		t.Errorf("offset outstanding must be interest-inclusive 110000, got %s", p.Data.Outstanding)
	}
	if !p.Data.InterestEnabled || !p.Data.InterestAmount.Equal(decimal.NewFromInt(10000)) {
		t.Errorf("preview must expose interest mode/amount, got %+v", p.Data)
	}
	_ = borrowerMember
}
