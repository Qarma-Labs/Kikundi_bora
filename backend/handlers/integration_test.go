package handlers

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http/httptest"
	"testing"
	"time"

	"kikundibora/config"
	"kikundibora/database"
	"kikundibora/middleware"
	"kikundibora/models"
	"kikundibora/services"

	"github.com/gofiber/fiber/v2"
	"github.com/shopspring/decimal"
	"golang.org/x/crypto/bcrypt"
)

func fullTestApp() *fiber.App {
	config.AppConfig = testConfig()
	database.Connect()
	database.AutoMigrate()
	services.InitEmail()

	app := fiber.New(fiber.Config{AppName: "Kikundi API Test"})
	app.Use(middleware.SetupCORS())

	api := app.Group("/api/v1")

	authHandler := NewAuthHandler()
	memberHandler := NewMemberHandler()
	contribHandler := NewContributionHandler()
	loanHandler := NewLoanHandler()
	repayHandler := NewRepaymentHandler()
	committeeHandler := NewLoanCommitteeHandler()
	userMgmtHandler := NewUserManagementHandler()
	loanSettingsHandler := NewLoanSettingsHandler()
	offsetHandler := NewLoanOffsetHandler()

	api.Post("/auth/login", authHandler.Login)

	protected := api.Group("")
	protected.Use(middleware.AuthRequired)
	protected.Get("/me", authHandler.Me)

	members := protected.Group("/members")
	members.Get("/", memberHandler.List)
	members.Post("/", middleware.RequireRoles(models.RoleChair, models.RoleSecretary, models.RoleTreasurer), memberHandler.Create)

	contribs := protected.Group("/contributions")
	contribs.Post("/", middleware.RequirePosition(models.PositionTreasurer), contribHandler.Create)

	loans := protected.Group("/loans")
	loans.Post("/apply", loanHandler.Apply)
	loans.Post("/:id/disburse", middleware.RequirePosition(models.PositionTreasurer), loanHandler.Disburse)
	loans.Patch("/:id/confirm-received", loanHandler.ConfirmReceived)
	loans.Get("/:id/offset-preview", middleware.RequireRoles(models.RoleChair, models.RoleSecretary, models.RoleTreasurer), offsetHandler.Preview)
	loans.Post("/:id/repayments", repayHandler.Submit)

	// Sequential loan-approval chain (BUG-2 fix) — outside the leadership
	// group so appointed bodi members can act on their turn.
	leadershipHandler := NewLeadershipHandler()
	protected.Get("/uongozi/mikopo/pending", middleware.RequireLoanCommitteeMember(), leadershipHandler.PendingLoans)
	protected.Post("/uongozi/mikopo/:id/approve", middleware.RequireLoanCommitteeMember(), leadershipHandler.ApproveLoan)

	welfareHandler := NewWelfareHandler()
	welfare := protected.Group("/welfare")
	welfare.Post("/events/:id/confirm-receipt", welfareHandler.ConfirmReceipt)

	committee := protected.Group("/loan-committee")
	committee.Use(middleware.RequireLoanCommitteeMember())
	committee.Post("/loans/:id/review", committeeHandler.SubmitReview)
	committee.Post("/members", middleware.RequireRoles(models.RoleChair), committeeHandler.AppointMember)

	repayments := protected.Group("/repayments")
	// NOTE: direct recording removed in production — submit → approve only.
	repayments.Patch("/:id/approve", middleware.RequireRoles(models.RoleTreasurer), repayHandler.Approve)
	repayments.Patch("/:id/reject", middleware.RequireRoles(models.RoleTreasurer), repayHandler.Reject)

	groups := protected.Group("/groups")
	groups.Get("/:id/repayments", middleware.RequireRoles(models.RoleTreasurer), repayHandler.PendingQueue)

	users := protected.Group("/users")
	users.Post("/create", middleware.RequireRoles(models.RoleChair), userMgmtHandler.CreateUser)

	loanSettings := groups.Group("/:id/loan-settings")
	loanSettings.Get("/", loanSettingsHandler.Get)
	loanSettings.Post("/propose", middleware.RequireRoles(models.RoleChair), loanSettingsHandler.Propose)
	loanSettings.Post("/approve", middleware.RequireRoles(models.RoleSecretary), loanSettingsHandler.Approve)
	loanSettings.Post("/reject", middleware.RequireRoles(models.RoleSecretary), loanSettingsHandler.Reject)

	return app
}

func hLogin(t *testing.T, app *fiber.App, email, pass string) string {
	t.Helper()
	body := map[string]string{"email": email, "password": pass}
	b, _ := json.Marshal(body)
	req := httptest.NewRequest("POST", "/api/v1/auth/login", bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	resp, _ := app.Test(req)
	data, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("login %s: %d %s", email, resp.StatusCode, data)
	}
	var r struct{ Token string }
	json.Unmarshal(data, &r)
	return r.Token
}

func hPost(t *testing.T, app *fiber.App, path string, body interface{}, token string) (int, []byte) {
	t.Helper()
	var r io.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		r = bytes.NewReader(b)
	}
	req := httptest.NewRequest("POST", path, r)
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, _ := app.Test(req)
	data, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	return resp.StatusCode, data
}

func hGet(t *testing.T, app *fiber.App, path string, token string) (int, []byte) {
	t.Helper()
	req := httptest.NewRequest("GET", path, nil)
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, _ := app.Test(req)
	data, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	return resp.StatusCode, data
}

func hExtract(t *testing.T, data []byte, key string) string {
	t.Helper()
	var m map[string]interface{}
	json.Unmarshal(data, &m)
	n := m[key].(map[string]interface{})
	return n["id"].(string)
}

func cleanAndSeed(t *testing.T) {
	t.Helper()
	requireTestDB(t)
	// Full dependency-ordered cleanup (FK-safe). A partial delete list lets
	// FK-referenced rows survive, silently aborts later DELETEs and skips the
	// reseed — leaving tests running against stale data.
	database.DB.Exec("UPDATE groups SET status='active' WHERE status='dissolved'")
	for _, stmt := range []string{
		"DELETE FROM loan_offset_transactions",
		"DELETE FROM loan_reviews",
		"DELETE FROM repayments",
		"DELETE FROM loan_installments",
		"DELETE FROM loans",
		"DELETE FROM loan_settings",
		"DELETE FROM contribution_edits",
		"DELETE FROM contributions",
		"DELETE FROM member_contributions",
		"DELETE FROM user_approvals",
		"DELETE FROM user_sessions",
		"DELETE FROM failed_logins",
		"DELETE FROM loan_committee_members",
		"DELETE FROM leadership_positions",
		"DELETE FROM user_positions",
		"DELETE FROM notifications",
		"DELETE FROM notification_sms_prefs",
		"DELETE FROM otp_challenges",
		"DELETE FROM audit_logs",
		"DELETE FROM admin_logs",
		"DELETE FROM pending_actions",
		"DELETE FROM welfare_contributions",
		"DELETE FROM welfare_events",
		"DELETE FROM meeting_attendances",
		"DELETE FROM meetings",
		"DELETE FROM fines",
		"DELETE FROM contribution_cycles",
		"DELETE FROM dissolution_payouts",
		"DELETE FROM dissolution_votes",
		"DELETE FROM group_dissolution_proposals",
		"DELETE FROM fine_offence_types",
		"DELETE FROM group_setting_proposals",
		"DELETE FROM members",
		"DELETE FROM users",
	} {
		database.DB.Exec(stmt)
	}
	var c int64
	database.DB.Model(&models.User{}).Count(&c)
	if c == 0 {
		database.Seed()
	}
}

// ---- P2: Full HTTP Loan Lifecycle ----

func TestLoanLifecycleHTTP(t *testing.T) {
	app := fullTestApp()
	cleanAndSeed(t)

	chair := hLogin(t, app, "juma@kikundi.tz", "demo123")
	treasurer := hLogin(t, app, "fatuma@kikundi.tz", "demo123")
	secretary := hLogin(t, app, "rashidi@kikundi.tz", "demo123")

	// Get member — the long-standing seed member (joined 2024) carries
	// ample arrears so allocation order is exercised deterministically.
	var seedMember models.Member
	database.DB.Where("deleted_at IS NULL").Order("member_no ASC").First(&seedMember)
	memberID := seedMember.ID

	// Step 1: Apply
	fundTreasury(500000) // loan approval requires treasury coverage
	code, d := hPost(t, app, "/api/v1/loans/apply", map[string]interface{}{
		"member_id": memberID, "amount": 200000.0, "purpose": "Kilimo", "due_date": "2026-12-31",
	}, chair)
	if code != 201 {
		t.Fatalf("apply: %d", code)
	}
	loanID := hExtract(t, d, "data")
	t.Logf("Loan: %s", loanID)

	// Appoint bodi FIRST so unanimous needs 4 (3 leaders + asha).
	code, _ = hPost(t, app, "/api/v1/loan-committee/members", map[string]interface{}{
		"user_id": hGetUserID(t, "asha@kikundi.tz"),
	}, chair)
	bodiTok := hLogin(t, app, "asha@kikundi.tz", "demo123")

	// Step 2: Committee review — 3 leaders approve (SYNC mirrors to chain
	// slots; BODI slot still missing → stays PENDING, not APPROVED).
	review := map[string]interface{}{"decision": "APPROVE"}
	for _, tok := range []string{chair, treasurer, secretary} {
		code, _ = hPost(t, app, "/api/v1/loan-committee/loans/"+loanID+"/review", review, tok)
		if code != 200 {
			t.Fatalf("committee review: %d", code)
		}
	}

	// Step 3: Parallel chain — bodi signs the last slot → 4/4 APPROVED.
	code, d = hPost(t, app, "/api/v1/uongozi/mikopo/"+loanID+"/approve", map[string]interface{}{}, bodiTok)
	if code != 200 {
		t.Fatalf("chain bodi approve: %d %s", code, d)
	}

	// Verify APPROVED
	var loan models.Loan
	database.DB.First(&loan, "id = ?", loanID)
	if loan.Status != models.LoanApproved {
		t.Fatalf("not approved: %s", loan.Status)
	}

	// Step 4: Disburse
	code, _ = hPost(t, app, fmt.Sprintf("/api/v1/loans/%s/disburse", loanID), nil, treasurer)
	if code != 200 {
		t.Fatalf("disburse: %d", code)
	}

	// Step 5: Repay via self-service (submit as borrower -> treasurer
	// approves -> CLOSED). Link the seed member to asha's login first.
	var ashaUser models.User
	database.DB.Where("email = ?", "asha@kikundi.tz").First(&ashaUser)
	database.DB.Model(&models.Member{}).Where("id = ?", memberID).Update("user_id", ashaUser.ID)
	borrowerTok := hLogin(t, app, "asha@kikundi.tz", "demo123")
	code, d = hPost(t, app, "/api/v1/loans/"+loanID+"/repayments", map[string]interface{}{
		"amount": 200000.0, "proof_message": "CASH handed to treasurer",
	}, borrowerTok)
	if code != 201 {
		t.Fatalf("repay submit: %d %s", code, d)
	}
	var sub struct {
		Data models.Repayment `json:"data"`
	}
	json.Unmarshal(d, &sub)
	code, d = hPatch(t, app, "/api/v1/repayments/"+sub.Data.ID+"/approve", treasurer, nil)
	if code != 200 {
		t.Fatalf("repay approve: %d %s", code, d)
	}
	var closed models.Loan
	database.DB.First(&closed, "id = ?", loanID)
	if closed.Status != models.LoanClosed {
		t.Error("loan not closed")
	}
	t.Log("Full HTTP loan lifecycle: PENDING -> APPROVED -> OUTSTANDING -> CLOSED")
}

// ---- P3: Negative Scenario Tests ----

func TestNegativeScenarios(t *testing.T) {
	app := fullTestApp()
	cleanAndSeed(t)

	chair := hLogin(t, app, "juma@kikundi.tz", "demo123")
	treasurer := hLogin(t, app, "fatuma@kikundi.tz", "demo123")
	secretary := hLogin(t, app, "rashidi@kikundi.tz", "demo123")

	// Apply loan
	_, d := hGet(t, app, "/api/v1/members", chair)
	var ml struct {
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	json.Unmarshal(d, &ml)
	memberID := ml.Data[0].ID

	fundTreasury(500000) // loan approval requires treasury coverage
	code, d := hPost(t, app, "/api/v1/loans/apply", map[string]interface{}{
		"member_id": memberID, "amount": 50000.0, "purpose": "Test", "due_date": "2026-12-31",
	}, chair)
	if code != 201 {
		t.Fatalf("apply: %d", code)
	}
	loanID := hExtract(t, d, "data")

	// NEGATIVE 1: Disburse before approval
	code, d = hPost(t, app, fmt.Sprintf("/api/v1/loans/%s/disburse", loanID), nil, treasurer)
	if code == 200 {
		t.Error("disburse BEFORE approval should fail")
	} else {
		t.Logf("disburse-before-approve: %d (expected non-200)", code)
	}

	// Approve: committee review satisfies the bodi stage, then the
	// sequential chain (Hazina → Katibu → Mwenyekiti) finalizes
	review := map[string]interface{}{"decision": "APPROVE"}
	for _, tok := range []string{chair, treasurer, secretary} {
		hPost(t, app, "/api/v1/loan-committee/loans/"+loanID+"/review", review, tok)
	}
	for _, tok := range []string{treasurer, secretary, chair} {
		hPost(t, app, "/api/v1/uongozi/mikopo/"+loanID+"/approve", map[string]interface{}{}, tok)
	}

	// Disburse
	hPost(t, app, fmt.Sprintf("/api/v1/loans/%s/disburse", loanID), nil, treasurer)

	// NEGATIVE 2: Disburse again
	code, d = hPost(t, app, fmt.Sprintf("/api/v1/loans/%s/disburse", loanID), nil, treasurer)
	if code == 200 {
		t.Error("double disburse should fail")
	} else {
		t.Logf("double-disburse: %d (expected non-200)", code)
	}

	// NEGATIVE 3: Over-repay via self-service submit (amount > balance).
	// Link the loan's member to asha's login so submission is authorized.
	var negUser models.User
	database.DB.Where("email = ?", "asha@kikundi.tz").First(&negUser)
	database.DB.Model(&models.Member{}).Where("id = ?", memberID).Update("user_id", negUser.ID)
	negBorrower := hLogin(t, app, "asha@kikundi.tz", "demo123")
	code, d = hPost(t, app, "/api/v1/loans/"+loanID+"/repayments", map[string]interface{}{
		"amount": 100000.0, "proof_message": "overpay attempt",
	}, negBorrower)
	if code == 201 {
		t.Error("over-repay should fail (amount > balance)")
	} else {
		t.Logf("over-repay: %d (expected non-201)", code)
	}

	// NEGATIVE 4: Chair cannot approve (committee-only now)
	code, d = hPost(t, app, "/api/v1/users/create", map[string]interface{}{
		"full_name": "Bad Role", "phone": "0719999999", "role": "admin",
	}, chair)
	if code == 201 {
		t.Error("chair should NOT be able to create admin user")
	} else {
		t.Logf("chair-create-admin: %d (expected non-201)", code)
	}

	// NEGATIVE 5: Member cannot access chair-only API
	memberTok := hLogin(t, app, "asha@kikundi.tz", "demo123")
	code, _ = hPost(t, app, "/api/v1/users/create", map[string]interface{}{
		"full_name": "Hacker", "phone": "0718888888", "role": "member",
	}, memberTok)
	if code == 201 {
		t.Error("member should NOT be able to create users")
	} else {
		t.Logf("member-create-user: %d (expected non-201)", code)
	}
}

// ---- P6: Contribution Test ----

func TestContributionFlow(t *testing.T) {
	app := fullTestApp()
	cleanAndSeed(t)
	cleanObligationTables(t)
	// Hermetic group schedule: fixed 10000/monthly due on the 5th, so the
	// seeded members (joined 2024) carry ample arrears for allocation.
	var grp models.Group
	database.DB.First(&grp)
	amt10000 := decimal.NewFromInt(10000)
	due05 := "05"
	grp.FixedContributionAmount = &amt10000
	grp.ContributionDueDate = &due05
	grp.ContributionInterval = models.IntervalMonthly
	database.DB.Save(&grp)

	_ = hLogin(t, app, "juma@kikundi.tz", "demo123")
	treasurer := hLogin(t, app, "fatuma@kikundi.tz", "demo123")

	// Long-standing seed member (joined 2024) carries ample arrears, so
	// allocation order is exercised deterministically (list order is
	// unstable and may return a recently-backfilled member with no arrears).
	var seedMemberCF models.Member
	database.DB.Where("deleted_at IS NULL").Order("member_no ASC").First(&seedMemberCF)
	memberID := seedMemberCF.ID

	// Record a contribution (allocated across obligations; receipt returned)
	code, d := hPost(t, app, "/api/v1/contributions", map[string]interface{}{
		"member_id":      memberID,
		"amount":         30000.0,
		"month":          "2026-07",
		"paid_at":        "2026-07-11",
		"payment_method": "CASH",
	}, treasurer)
	if code != 201 {
		t.Fatalf("contribution failed: %d %s", code, d)
	}
	var receipt struct {
		Data services.AllocationReceipt `json:"data"`
	}
	json.Unmarshal(d, &receipt)
	if receipt.Data.Applied.String() != "30000" {
		t.Errorf("applied = %s, want 30000", receipt.Data.Applied)
	}
	t.Logf("Contribution allocated: %+v", receipt.Data.Lines)

	// Verify reconciled rows in DB (allocation creates/merges month rows)
	var total string
	database.DB.Raw("SELECT COALESCE(SUM(amount),0)::text FROM contributions WHERE member_id = ?", memberID).Scan(&total)
	totalDec, _ := decimal.NewFromString(total)
	if !totalDec.Equal(decimal.NewFromInt(30000)) {
		t.Errorf("contributions total = %s, want 30000", total)
	}

	// Second payment for the same month no longer conflicts — it flows to
	// the next outstanding obligation (allocation order).
	code, d = hPost(t, app, "/api/v1/contributions", map[string]interface{}{
		"member_id":      memberID,
		"amount":         5000.0,
		"month":          "2026-07",
		"paid_at":        "2026-07-12",
		"payment_method": "CASH",
	}, treasurer)
	if code != 201 {
		t.Errorf("second payment should allocate, got: %d %s", code, d)
	} else {
		t.Logf("second payment allocated: %d", code)
	}
}

// ---- P7: Rate Limiting Test ----

func TestLoginRateLimiting(t *testing.T) {
	app := fullTestApp()
	cleanAndSeed(t)
	// Rate limiting must actually engage: the local .env disables it for
	// dev convenience, and the login handler auto-bypasses when config
	// Environment == "test" (so suites never lock themselves out).
	// Both bypasses are lifted for the duration of this test only.
	defer withLoginRateLimitEnabled()()
	prevEnv := config.AppConfig.Environment
	config.AppConfig.Environment = "development"
	defer func() { config.AppConfig.Environment = prevEnv }()

	body := map[string]string{"email": "nonexistent@test.com", "password": "wrong"}
	b, _ := json.Marshal(body)

	for i := 1; i <= 6; i++ {
		req := httptest.NewRequest("POST", "/api/v1/auth/login", bytes.NewReader(b))
		req.Header.Set("Content-Type", "application/json")
		resp, _ := app.Test(req)
		data, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		t.Logf("Attempt %d: %d", i, resp.StatusCode)
		if i <= 5 && resp.StatusCode != 401 {
			t.Errorf("attempt %d: expected 401, got %d", i, resp.StatusCode)
		}
		if i == 6 && resp.StatusCode != 429 {
			t.Errorf("attempt %d: expected 429 (rate limited), got %d", i, resp.StatusCode)
		} else if i == 6 {
			var r struct {
				Message string `json:"message"`
			}
			json.Unmarshal(data, &r)
			t.Logf("Rate limit message: %s", r.Message)
		}
	}
}

// ---- P8: Committee Review Flow ----

func TestCommitteeReviewFlow(t *testing.T) {
	app := fullTestApp()
	cleanAndSeed(t)

	chair := hLogin(t, app, "juma@kikundi.tz", "demo123")
	treasurer := hLogin(t, app, "fatuma@kikundi.tz", "demo123")
	secretary := hLogin(t, app, "rashidi@kikundi.tz", "demo123")
	member := hLogin(t, app, "asha@kikundi.tz", "demo123")

	// Appoint member to committee
	code, _ := hPost(t, app, "/api/v1/loan-committee/members", map[string]interface{}{
		"user_id": hGetUserID(t, "asha@kikundi.tz"),
	}, chair)
	t.Logf("Appoint member: %d", code)

	// Neutral borrower outside the committee so self-review guard never blocks
	// unanimous vote (borrower must not review own loan).
	neemaEmail := "neema2@kikundi.tz"
	neemaHash, _ := bcrypt.GenerateFromPassword([]byte("demo123"), bcrypt.MinCost)
	neemaUser := models.User{
		Name: "Neema Borrower", Email: &neemaEmail, Phone: "0718888888",
		Password: string(neemaHash), Role: models.RoleMember,
		Status: models.UserStatusActive, IsActive: true,
	}
	if err := database.DB.Create(&neemaUser).Error; err != nil {
		t.Fatalf("create neutral user: %v", err)
	}
	neemaMember := models.Member{
		MemberNo: "KKK-NEUTRAL-02", FullName: "Neema Borrower", Phone: "0718888888",
		UserID: &neemaUser.ID, IsActive: true, ApprovalStatus: "approved",
		JoinedAt: time.Now(), RegisteredBy: neemaUser.ID,
	}
	if err := database.DB.Create(&neemaMember).Error; err != nil {
		t.Fatalf("create neutral member: %v", err)
	}
	borrower := hLogin(t, app, neemaEmail, "demo123")
	fundTreasury(500000) // apply requires treasury coverage
	code, d := hPost(t, app, "/api/v1/loans/apply", map[string]interface{}{
		"member_id": neemaMember.ID, "amount": 100000.0, "purpose": "Test", "due_date": "2026-12-31",
	}, borrower)
	loanID := hExtract(t, d, "data")

	// Try review by non-committee member (asha is committee now though — use a different non-member)
	// Actually asha IS now committee, so normal member can't review. Skip this sub-test.

	// All 4 committee members (chair, treasurer, secretary, asha) approve —
	// unanimous committee decision satisfies the BODI stage only.
	review := map[string]interface{}{"decision": "APPROVE"}
	for _, tok := range []string{chair, treasurer, secretary, member} {
		code, _ = hPost(t, app, "/api/v1/loan-committee/loans/"+loanID+"/review", review, tok)
		t.Logf("  review: %d", code)
	}

	// SYNC: unanimous committee by all 4 leaders mirrors to all chain slots,
	// so parallel rule finalizes APPROVED immediately (no separate chain
	// round needed). Bodi slot must be satisfied too.
	var loan models.Loan
	database.DB.First(&loan, "id = ?", loanID)
	if loan.Status != models.LoanApproved {
		t.Errorf("expected APPROVED after unanimous committee (sync finalizes 4/4), got %s", loan.Status)
	}
	if loan.BodiApprovedAt == nil {
		t.Errorf("expected bodi_approved_at set by unanimous committee")
	}
	if loan.HazinaApprovedAt == nil || loan.KatibuApprovedAt == nil || loan.MwenyekitiApprovedAt == nil {
		t.Errorf("expected all chain slots mirrored from committee reviews")
	}
	t.Logf("Committee review sync → APPROVED: %s", loan.Status)
}

func hGetUserID(t *testing.T, email string) string {
	t.Helper()
	var u models.User
	if err := database.DB.Where("email = ?", email).First(&u).Error; err != nil {
		t.Fatalf("user %s not found", email)
	}
	return u.ID
}

func hGetCommitteeCount() int64 {
	var posCount, apptCount int64
	database.DB.Model(&models.UserPosition{}).
		Where("position_type IN ? AND is_active = TRUE",
			[]models.PositionType{models.PositionChairperson, models.PositionSecretary, models.PositionTreasurer}).
		Count(&posCount)
	database.DB.Model(&models.LoanCommitteeMember{}).Where("is_active = TRUE").Count(&apptCount)
	return posCount + apptCount
}
