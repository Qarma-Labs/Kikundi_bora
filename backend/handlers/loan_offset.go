package handlers

import (
	"time"

	"kikundibora/database"
	"kikundibora/middleware"
	"kikundibora/models"
	"kikundibora/services"

	"github.com/gofiber/fiber/v2"
	"github.com/shopspring/decimal"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type LoanOffsetHandler struct{}

func NewLoanOffsetHandler() *LoanOffsetHandler { return &LoanOffsetHandler{} }

// loanOutstandingOf floors the remaining balance at zero (never negative).
func loanOutstandingOf(loan *models.Loan) decimal.Decimal {
	if loan.BalanceRemaining == nil {
		return decimal.Zero
	}
	if loan.BalanceRemaining.LessThan(decimal.Zero) {
		return decimal.Zero
	}
	return *loan.BalanceRemaining
}

// isOffsetEligible reuses the portfolio overdue rule: OUTSTANDING with a
// scheduled installment past due and unpaid (schedule-based, never a bare
// timer). Loans disbursed before schedules existed fall back to due_date.
func isOffsetEligible(loan *models.Loan, today time.Time) bool {
	if loan.Status != models.LoanOutstanding {
		return false
	}
	var n int64
	database.DB.Model(&models.LoanInstallment{}).
		Where("loan_id = ?", loan.ID).Count(&n)
	if n > 0 {
		return loanScheduleOverdue(loan.ID, today)
	}
	return dateOnlyOf(loan.DueDate).Before(dateOnlyOf(today))
}

// memberSavingsGross sums confirmed AKIBA savings from both stores
// (treasurer-recorded PAID + CONFIRMED self-submissions).
func memberSavingsGross(memberID string) (decimal.Decimal, error) {
	total, _, err := sumContributionsBothStores(memberID, true)
	return total, err
}

// memberOffsetsApplied sums already-executed offsets for a member.
func memberOffsetsApplied(memberID string) decimal.Decimal {
	var total decimal.Decimal
	database.DB.Model(&models.LoanOffsetTransaction{}).
		Where("member_id = ? AND status = ?", memberID, models.LoanOffsetExecuted).
		Select("COALESCE(SUM(amount), 0)").Scan(&total)
	return total
}

// memberAvailableSavings returns gross confirmed savings, total offsets
// applied, and the net available balance (floored at zero).
func memberAvailableSavings(memberID string) (gross, applied, available decimal.Decimal, err error) {
	gross, err = memberSavingsGross(memberID)
	if err != nil {
		return decimal.Zero, decimal.Zero, decimal.Zero, err
	}
	applied = memberOffsetsApplied(memberID)
	available = gross.Sub(applied)
	if available.LessThan(decimal.Zero) {
		available = decimal.Zero
	}
	return gross, applied, available, nil
}

// offsetCapOf caps at MIN(outstanding, available) — never more than the
// member saved, never driving the loan negative.
func offsetCapOf(outstanding, available decimal.Decimal) decimal.Decimal {
	if outstanding.LessThan(available) {
		return outstanding
	}
	return available
}

func dateOnlyOf(t time.Time) time.Time {
	y, m, d := t.Date()
	return time.Date(y, m, d, 0, 0, 0, 0, time.UTC)
}

type offsetPreview struct {
	Eligible          bool                    `json:"eligible"`
	Reason            string                  `json:"reason,omitempty"`
	Outstanding       decimal.Decimal         `json:"outstanding"`
	GrossSavings      decimal.Decimal         `json:"gross_savings"`
	OffsetsApplied    decimal.Decimal         `json:"offsets_applied"`
	AvailableSavings  decimal.Decimal         `json:"available_savings"`
	OffsetAmount      decimal.Decimal         `json:"offset_amount"`
	ExistingProposal  *models.LoanOffsetTransaction `json:"existing_proposal,omitempty"`
	// Term + interest context: Outstanding is ALWAYS interest-inclusive
	// (balance_remaining tracks principal + snapshotted interest). For
	// interest-free loans the interest fields are verifiably zero.
	TermDays         int             `json:"term_days"`
	InterestEnabled  bool            `json:"interest_enabled"`
	InterestRate     decimal.Decimal `json:"interest_rate"`
	InterestAmount   decimal.Decimal `json:"interest_amount"`
	TotalRepayment   decimal.Decimal `json:"total_repayment"`
	Principal        decimal.Decimal `json:"principal"`
}

// buildPreview loads the loan + member balances and computes the capped
// offset. It never writes.
func buildPreview(loanID string) (*models.Loan, *offsetPreview, int, string) {
	var loan models.Loan
	if err := database.DB.First(&loan, "id = ?", loanID).Error; err != nil {
		return nil, nil, fiber.StatusNotFound, "Mkopo haujapatikana"
	}
	p := &offsetPreview{Eligible: true, Outstanding: loanOutstandingOf(&loan)}
	p.TermDays = loan.TermDays
	p.InterestEnabled = loan.InterestEnabled
	p.InterestRate = loan.ApplicableInterestRate
	p.InterestAmount = loan.InterestAmount
	p.TotalRepayment = loan.TotalRepayment
	p.Principal = loan.Amount
	if loan.ApprovedAmount != nil {
		p.Principal = *loan.ApprovedAmount
	}
	if loan.Status != models.LoanOutstanding {
		p.Eligible = false
		p.Reason = "Mkopo huu haupo wazi (hali: " + string(loan.Status) + ")"
		return &loan, p, 0, ""
	}
	if !isOffsetEligible(&loan, time.Now()) {
		p.Eligible = false
		p.Reason = "Mkopo haujachelewa — offset inaruhusiwa kwa mikopo iliyochelewa tu"
		return &loan, p, 0, ""
	}
	var member models.Member
	if err := database.DB.Where("id = ? AND deleted_at IS NULL", loan.MemberID).First(&member).Error; err != nil {
		p.Eligible = false
		p.Reason = "Mwanachama hajapatikana"
		return &loan, p, 0, ""
	}
	if member.ApprovalStatus != models.MemberApprovalApproved || !member.IsActive {
		p.Eligible = false
		p.Reason = "Mwanachama hajaidhinishwa"
		return &loan, p, 0, ""
	}
	gross, applied, available, err := memberAvailableSavings(loan.MemberID)
	if err != nil {
		return nil, nil, fiber.StatusInternalServerError, "Imeshindikana kupata akiba ya mwanachama"
	}
	p.GrossSavings = gross
	p.OffsetsApplied = applied
	p.AvailableSavings = available
	p.OffsetAmount = offsetCapOf(p.Outstanding, available)
	if p.OffsetAmount.LessThanOrEqual(decimal.Zero) {
		p.Eligible = false
		p.Reason = "Mwanachama hana akiba inayopatikana ya kukata"
		return &loan, p, 0, ""
	}
	var existing models.LoanOffsetTransaction
	if err := database.DB.Where("loan_id = ? AND status IN ?", loanID,
		[]string{models.LoanOffsetProposed, models.LoanOffsetApproved}).
		Order("created_at DESC").First(&existing).Error; err == nil {
		p.ExistingProposal = &existing
	}
	return &loan, p, 0, ""
}

// Preview returns the capped offset numbers for an overdue loan.
// GET /api/v1/loans/:id/offset-preview (chair/secretary/treasurer)
func (h *LoanOffsetHandler) Preview(c *fiber.Ctx) error {
	_, p, code, msg := buildPreview(c.Params("id"))
	if code != 0 {
		return c.Status(code).JSON(fiber.Map{"message": msg})
	}
	return c.JSON(fiber.Map{"data": p})
}

// Propose creates a PROPOSED offset (mwenyekiti only). All money figures
// are computed server-side — the client may only supply a reason.
// POST /api/v1/loans/:id/offset-propose
func (h *LoanOffsetHandler) Propose(c *fiber.Ctx) error {
	loan, p, code, msg := buildPreview(c.Params("id"))
	if code != 0 {
		return c.Status(code).JSON(fiber.Map{"message": msg})
	}
	if !p.Eligible {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"message": p.Reason})
	}
	if p.ExistingProposal != nil {
		return c.Status(fiber.StatusConflict).JSON(fiber.Map{"message": "Tayari kuna pendekezo linalosubiri kwa mkopo huu", "data": p.ExistingProposal})
	}
	var req struct {
		Reason *string `json:"reason"`
	}
	_ = c.BodyParser(&req) // reason optional; figures never come from client

	userID := middleware.GetUserID(c)
	tx := models.LoanOffsetTransaction{
		LoanID:            loan.ID,
		MemberID:          loan.MemberID,
		ProposedAmount:    p.OffsetAmount,
		OutstandingBefore: p.Outstanding,
		SavingsBefore:     p.AvailableSavings,
		Status:            models.LoanOffsetProposed,
		ProposedBy:        userID,
		Reason:            req.Reason,
	}
	if err := database.DB.Create(&tx).Error; err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"message": "Imeshindikana kutengeneza pendekezo"})
	}
	services.LogAudit(c, &userID, models.AuditCreate, "loan_offset_transactions", &tx.ID, nil, map[string]interface{}{
		"loan_id": loan.ID, "member_id": loan.MemberID,
		"outstanding": p.Outstanding, "available_savings": p.AvailableSavings,
		"proposed_amount": p.OffsetAmount,
	})
	return c.Status(fiber.StatusCreated).JSON(fiber.Map{"message": "Pendekezo la offset limetumwa kwa Katibu", "data": tx})
}

// List returns offset transactions (leadership view).
// GET /api/v1/loan-offsets?status=&loan_id=&member_id=
func (h *LoanOffsetHandler) List(c *fiber.Ctx) error {
	var pq models.PaginationQuery
	if err := c.QueryParser(&pq); err != nil {
		pq = models.PaginationQuery{Page: 1, Limit: 20}
	}
	query := database.DB.
		Preload("Member", func(db *gorm.DB) *gorm.DB {
			return db.Select("id, member_no, full_name")
		})
	if s := c.Query("status"); s != "" {
		query = query.Where("loan_offset_transactions.status = ?", s)
	}
	if l := c.Query("loan_id"); l != "" {
		query = query.Where("loan_id = ?", l)
	}
	if m := c.Query("member_id"); m != "" {
		query = query.Where("member_id = ?", m)
	}
	var total int64
	query.Model(&models.LoanOffsetTransaction{}).Count(&total)
	var rows []models.LoanOffsetTransaction
	query.Order("created_at DESC").Offset(pq.GetOffset()).Limit(pq.Limit).Find(&rows)
	return c.JSON(fiber.Map{"data": rows, "total": total, "page": pq.Page, "limit": pq.Limit})
}

// Approve moves PROPOSED → APPROVED (katibu only; approver from session).
// POST /api/v1/loan-offsets/:id/approve
func (h *LoanOffsetHandler) Approve(c *fiber.Ctx) error {
	var tx models.LoanOffsetTransaction
	if err := database.DB.First(&tx, "id = ?", c.Params("id")).Error; err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"message": "Pendekezo halijapatikana"})
	}
	if tx.Status != models.LoanOffsetProposed {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"message": "Pendekezo haliwezi kuidhinishwa (hali: " + tx.Status + ")"})
	}
	userID := middleware.GetUserID(c)
	now := time.Now()
	tx.Status = models.LoanOffsetApproved
	tx.ApprovedBy = &userID
	tx.ApprovedAt = &now
	if err := database.DB.Save(&tx).Error; err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"message": "Imeshindikana kuidhinisha"})
	}
	services.LogAudit(c, &userID, models.AuditApprove, "loan_offset_transactions", &tx.ID,
		map[string]interface{}{"status": models.LoanOffsetProposed},
		map[string]interface{}{"status": models.LoanOffsetApproved})
	return c.JSON(fiber.Map{"message": "Offset imeidhinishwa — inasubiri kutekelezwa na Hazina", "data": tx})
}

// Reject moves PROPOSED → REJECTED (katibu only).
// POST /api/v1/loan-offsets/:id/reject
func (h *LoanOffsetHandler) Reject(c *fiber.Ctx) error {
	var tx models.LoanOffsetTransaction
	if err := database.DB.First(&tx, "id = ?", c.Params("id")).Error; err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"message": "Pendekezo halijapatikana"})
	}
	if tx.Status != models.LoanOffsetProposed {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"message": "Pendekezo haliwezi kukataliwa (hali: " + tx.Status + ")"})
	}
	var req struct {
		Reason *string `json:"reason"`
	}
	_ = c.BodyParser(&req)
	userID := middleware.GetUserID(c)
	now := time.Now()
	tx.Status = models.LoanOffsetRejected
	tx.ApprovedBy = &userID
	tx.ApprovedAt = &now
	tx.Reason = req.Reason
	if err := database.DB.Save(&tx).Error; err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"message": "Imeshindikana kukataa"})
	}
	services.LogAudit(c, &userID, models.AuditReject, "loan_offset_transactions", &tx.ID,
		map[string]interface{}{"status": models.LoanOffsetProposed},
		map[string]interface{}{"status": models.LoanOffsetRejected})
	return c.JSON(fiber.Map{"message": "Pendekezo limekataliwa", "data": tx})
}

// Execute applies an APPROVED offset (mweka-hazina only; executor from
// session). The cap is recomputed inside the locked transaction from live
// balances — never from client input, never negative, never over either
// side. Remainder (if any) stays as outstanding debt: nothing is forgiven.
// POST /api/v1/loan-offsets/:id/execute
func (h *LoanOffsetHandler) Execute(c *fiber.Ctx) error {
	userID := middleware.GetUserID(c)

	tx := database.DB.Begin()
	defer tx.Rollback()

	var off models.LoanOffsetTransaction
	if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&off, "id = ?", c.Params("id")).Error; err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"message": "Pendekezo halijapatikana"})
	}
	if off.Status != models.LoanOffsetApproved {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"message": "Offset lazima iidhinishwe kwanza (hali: " + off.Status + ")"})
	}
	var loan models.Loan
	if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&loan, "id = ?", off.LoanID).Error; err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"message": "Mkopo haujapatikana"})
	}
	if loan.Status != models.LoanOutstanding {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"message": "Mkopo haupo wazi tena"})
	}

	outstanding := loanOutstandingOf(&loan)
	if outstanding.LessThanOrEqual(decimal.Zero) {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"message": "Mkopo hauna salio"})
	}
	// Live cap: gross savings minus offsets already executed (this row is
	// still APPROVED so it is not double-counted).
	var gross decimal.Decimal
	tx.Model(&models.Contribution{}).Where("member_id = ? AND status = ?", loan.MemberID, "PAID").
		Select("COALESCE(SUM(amount), 0)").Scan(&gross)
	var mcTotal decimal.Decimal
	tx.Model(&models.MemberContribution{}).
		Where("member_id = ? AND status = ? AND contribution_type = ?",
			loan.MemberID, models.ContributionConfirmed, models.ContributionAkiba).
		Select("COALESCE(SUM(amount), 0)").Scan(&mcTotal)
	gross = gross.Add(mcTotal)
	var applied decimal.Decimal
	tx.Model(&models.LoanOffsetTransaction{}).
		Where("member_id = ? AND status = ?", loan.MemberID, models.LoanOffsetExecuted).
		Select("COALESCE(SUM(amount), 0)").Scan(&applied)
	available := gross.Sub(applied)
	if available.LessThan(decimal.Zero) {
		available = decimal.Zero
	}
	amount := offsetCapOf(outstanding, available)
	if amount.LessThanOrEqual(decimal.Zero) {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"message": "Mwanachama hana akiba inayopatikana ya kukata"})
	}

	newBalance := outstanding.Sub(amount)
	newStatus := models.LoanOutstanding
	loanClosed := false
	if newBalance.LessThan(decimal.NewFromFloat(0.001)) {
		newBalance = decimal.Zero
		newStatus = models.LoanClosed
		loanClosed = true
	}

	now := time.Now()
	off.Amount = amount
	off.Status = models.LoanOffsetExecuted
	off.ExecutedBy = &userID
	off.ExecutedAt = &now
	if err := tx.Save(&off).Error; err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"message": "Imeshindikana kutekeleza"})
	}
	// Mirror the offset into the repayment schedule (oldest-due first), so
	// the schedule, balance and offset stay consistent.
	var installments []models.LoanInstallment
	tx.Where("loan_id = ? AND status = ?", loan.ID, models.InstallmentPending).
		Order("number ASC").Find(&installments)
	remaining := amount
	for i := range installments {
		if remaining.LessThanOrEqual(decimal.Zero) {
			break
		}
		inst := &installments[i]
		owed := inst.TotalAmount.Sub(inst.PaidAmount)
		if owed.LessThanOrEqual(decimal.Zero) {
			continue
		}
		pay := owed
		if remaining.LessThan(owed) {
			pay = remaining
		}
		inst.PaidAmount = inst.PaidAmount.Add(pay)
		remaining = remaining.Sub(pay)
		if inst.PaidAmount.GreaterThanOrEqual(inst.TotalAmount.Sub(decimal.NewFromFloat(0.001))) {
			inst.Status = models.InstallmentPaid
		}
		if err := tx.Save(inst).Error; err != nil {
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"message": "Imeshindikana kusasisha ratiba"})
		}
	}
	loan.BalanceRemaining = &newBalance
	loan.Status = newStatus
	if err := tx.Save(&loan).Error; err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"message": "Imeshindikana kusasisha mkopo"})
	}
	if err := tx.Commit().Error; err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"message": "Imeshindikana kutekeleza"})
	}

	services.LogAudit(c, &userID, models.AuditUpdate, "loan_offset_transactions", &off.ID,
		map[string]interface{}{"status": models.LoanOffsetApproved},
		map[string]interface{}{"status": models.LoanOffsetExecuted, "amount": amount, "loan_balance_after": newBalance, "loan_closed": loanClosed},
	)

	// Notify the member plainly: savings were used on the overdue loan.
	var member models.Member
	if err := database.DB.First(&member, "id = ?", loan.MemberID).Error; err == nil {
		notifUserID := ""
		if member.UserID != nil {
			notifUserID = *member.UserID
		}
		if notifUserID == "" {
			notifUserID = member.RegisteredBy
		}
		if notifUserID != "" {
			msg := "Akiba yako ya TZS " + amount.StringFixed(0) + " imetumika kulipa mkopo uliochelewa."
			if loanClosed {
				msg += " Mkopo umefungwa kikamilifu."
			} else {
				msg += " Salio lililobaki: TZS " + newBalance.StringFixed(0) + "."
			}
			services.NotifyUser(notifUserID, models.NotifRepayment, "Akiba Imetumika Kulipa Mkopo", msg)
		}
	}

	respMsg := "Offset imetekelezwa. Salio lililobaki: TZS " + newBalance.StringFixed(0)
	if loanClosed {
		respMsg = "Offset imetekelezwa. Mkopo umefungwa kikamilifu!"
	}
	return c.JSON(fiber.Map{"message": respMsg, "data": off})
}
