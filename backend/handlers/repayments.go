package handlers

import (
	"fmt"
	"log"
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

type RepaymentHandler struct{}

func NewRepaymentHandler() *RepaymentHandler {
	return &RepaymentHandler{}
}

func (h *RepaymentHandler) List(c *fiber.Ctx) error {
	var pq models.PaginationQuery
	if err := c.QueryParser(&pq); err != nil {
		pq = models.PaginationQuery{Page: 1, Limit: 20}
	}

	loanID := c.Query("loan_id")
	memberID := c.Query("member_id")
	role := middleware.GetUserRole(c)
	userID := middleware.GetUserID(c)

	query := database.DB.
		Preload("Member", func(db *gorm.DB) *gorm.DB {
			return db.Select("id, member_no, full_name, phone")
		}).
		Preload("Recorder", func(db *gorm.DB) *gorm.DB {
			return db.Select("id, name, role")
		})

	// Members only see their own repayments
	if role == models.RoleMember {
		var ownMember models.Member
		if err := database.DB.Where("user_id = ? AND deleted_at IS NULL", userID).First(&ownMember).Error; err != nil {
			return c.JSON(fiber.Map{"data": []models.Repayment{}, "total": 0, "page": pq.Page, "limit": pq.Limit})
		}
		query = query.Where("member_id = ?", ownMember.ID)
	} else if memberID != "" {
		query = query.Where("member_id = ?", memberID)
	}
	if loanID != "" {
		query = query.Where("loan_id = ?", loanID)
	}
	if st := c.Query("status"); st != "" {
		query = query.Where("repayments.status = ?", st)
	}

	var total int64
	query.Model(&models.Repayment{}).Count(&total)

	var repayments []models.Repayment
	query.Offset(pq.GetOffset()).Limit(pq.Limit).Order("paid_at DESC").Find(&repayments)

	return c.JSON(fiber.Map{
		"data":  repayments,
		"total": total,
		"page":  pq.Page,
		"limit": pq.Limit,
	})
}

func (h *RepaymentHandler) Record(c *fiber.Ctx) error {
	var req models.RecordRepaymentRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"message": "Data si sahihi"})
	}

	if err := validate.Struct(req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"message": formatValidationErrors(err)})
	}

	if req.Amount.LessThanOrEqual(decimal.Zero) {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"message": "Kiasi cha malipo lazima kiwe zaidi ya sifuri"})
	}

	paidAt, err := time.Parse("2006-01-02", req.PaidAt)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"message": "Tarehe ya malipo si sahihi"})
	}

	userID := middleware.GetUserID(c)

	tx := database.DB.Begin()

	var loan models.Loan
	if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&loan, "id = ?", req.LoanID).Error; err != nil {
		tx.Rollback()
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"message": "Mkopo haujapatikana"})
	}

	if loan.Status != models.LoanOutstanding {
		tx.Rollback()
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"message": fmt.Sprintf("Mkopo huu hauhitaji malipo. Hali yake ni: %s", loan.Status)})
	}

	if loan.BalanceRemaining == nil {
		tx.Rollback()
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"message": "Salio la mkopo halijapatikana"})
	}

	balance := *loan.BalanceRemaining
	if req.Amount.GreaterThan(balance) {
		tx.Rollback()
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"message": fmt.Sprintf("Kiasi kimezidi salio. Salio ni TZS %s", balance.StringFixed(2))})
	}

	newBalance := balance.Sub(req.Amount)
	newStatus := models.LoanOutstanding
	if newBalance.LessThan(decimal.NewFromFloat(0.001)) {
		newStatus = models.LoanClosed
	}

	// Allocate the payment across schedule installments, oldest-due first.
	// Loans disbursed before schedules existed simply have no rows — the
	// balance math above still holds.
	var installments []models.LoanInstallment
	tx.Where("loan_id = ? AND status = ?", loan.ID, models.InstallmentPending).
		Order("number ASC").Find(&installments)
	remaining := req.Amount
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
			tx.Rollback()
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"message": "Imeshindikana kusasisha ratiba ya marejesho"})
		}
	}

	// Late flag (not a rejection): payments past the term end are accepted
	// but explicitly flagged — restructuring/extension is a follow-up action.
	late := dateOnlyOf(paidAt).After(dateOnlyOf(loan.DueDate))

	repayment := models.Repayment{
		LoanID:          req.LoanID,
		MemberID:        loan.MemberID,
		RecordedBy:      userID,
		Amount:          req.Amount,
		BalanceAfter:    newBalance,
		PaidAt:          paidAt,
		PaymentMethod:   req.PaymentMethod,
		ReferenceNumber: req.ReferenceNumber,
		ReceiptURL:      req.ReceiptURL,
		Notes:           req.Notes,
		Status:          models.RepaymentConfirmed,
	}

	if err := tx.Create(&repayment).Error; err != nil {
		tx.Rollback()
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"message": "Imeshindikana kurekodi malipo"})
	}

	loan.BalanceRemaining = &newBalance
	loan.Status = newStatus
	if err := tx.Save(&loan).Error; err != nil {
		tx.Rollback()
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"message": "Imeshindikana kusasisha mkopo"})
	}

	if err := tx.Commit().Error; err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"message": "Imeshindikana kurekodi malipo"})
	}

	// Audit and notifications only after successful commit
	services.LogAudit(c, &userID, models.AuditUpdate, "repayments", &repayment.ID,
		map[string]interface{}{"balance_before": balance},
		map[string]interface{}{"amount_paid": req.Amount, "balance_after": newBalance, "loan_status": string(newStatus), "payment_method": req.PaymentMethod},
	)

	// Auto-post into the double-entry ledger (best-effort).
	var payMember models.Member
	if err := database.DB.First(&payMember, "id = ?", loan.MemberID).Error; err == nil {
		if err := services.PostRepayment(payMember.MemberNo, req.Amount, paidAt, userID,
			fmt.Sprintf("Marejesho %s mkopo %s", payMember.MemberNo, loan.ID)); err != nil {
			log.Printf("WARN: ledger auto-post repayment %s: %v", repayment.ID, err)
		}
	}

	if newStatus == models.LoanClosed {
		services.NotifyRole(models.RoleChair, models.NotifRepayment, "Mkopo Umefungwa", "Hongera! Mkopo umelipwa kikamilifu.", "")
		services.NotifyRole(models.RoleTreasurer, models.NotifRepayment, "Mkopo Umefungwa", "Hongera! Mkopo umelipwa kikamilifu.", "")
	}

	loanClosed := newStatus == models.LoanClosed
	msg := fmt.Sprintf("Malipo yamerekodiwa. Salio lililobaki: TZS %s", newBalance.StringFixed(2))
	if loanClosed {
		msg = "Malipo yamerekodiwa. Mkopo umefungwa kikamilifu!"
	}
	if late && !loanClosed {
		msg += " (Malipo yamechelewa — nje ya muda wa mkopo.)"
	}

	return c.Status(fiber.StatusCreated).JSON(fiber.Map{
		"message": msg,
		"data": models.RepaymentResponse{
			RepaymentID:  repayment.ID,
			BalanceAfter: newBalance,
			LoanClosed:   loanClosed,
		},
		"late": late,
	})
}

// Submit lets a member pay toward their OWN disbursed loan (self-service,
// mirrors Weka Mchango). The record starts PENDING and touches NOTHING — no
// balance, schedule or ledger movement — until the treasurer approves.
// POST /api/v1/loans/:id/repayments
func (h *RepaymentHandler) Submit(c *fiber.Ctx) error {
	if IsGroupDissolved() {
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{"message": "Kikundi kimevunjwa — marejesho yamefungwa"})
	}
	loanID := c.Params("id")
	var req struct {
		Amount        decimal.Decimal `json:"amount" validate:"required,gt=0"`
		PaidAt        string          `json:"paid_at"`
		PaymentMethod string          `json:"payment_method" validate:"omitempty,oneof=CASH BANK MOBILE_MONEY"`
		ProofImageURL string          `json:"proof_image_url" validate:"omitempty,url,max=500"`
		ProofMessage  *string         `json:"proof_message" validate:"omitempty,max=1000"`
		Notes         *string         `json:"notes" validate:"omitempty,max=1000"`
	}
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"message": "Data si sahihi"})
	}
	if err := validate.Struct(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"message": formatValidationErrors(err)})
	}
	if req.Amount.LessThanOrEqual(decimal.Zero) {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"message": "Kiasi cha malipo lazima kiwe zaidi ya sifuri"})
	}
	// Proof required — exactly like contribution submission (image OR message).
	if req.ProofImageURL == "" && (req.ProofMessage == nil || *req.ProofMessage == "") {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"message": "Lazima uweke picha ya uthibitisho au ujumbe wa muamala"})
	}

	paidAt := time.Now()
	if req.PaidAt != "" {
		var err error
		paidAt, err = time.Parse("2006-01-02", req.PaidAt)
		if err != nil {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"message": "Tarehe ya malipo si sahihi"})
		}
	}
	method := req.PaymentMethod
	if method == "" {
		method = "CASH"
	}

	userID := middleware.GetUserID(c)

	var loan models.Loan
	if err := database.DB.First(&loan, "id = ?", loanID).Error; err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"message": "Mkopo haujapatikana"})
	}
	if loan.Status != models.LoanOutstanding {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": fmt.Sprintf("Mkopo huu haupokei marejesho kwa sasa. Hali yake ni: %s", loan.Status),
		})
	}
	// Ownership: own loan only (same pattern as loan apply/confirm).
	var me models.Member
	if err := database.DB.Where("user_id = ? AND deleted_at IS NULL", userID).First(&me).Error; err != nil || me.ID != loan.MemberID {
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{"message": "Unaweza kufanya marejesho kwa mkopo wako tu"})
	}
	if loan.BalanceRemaining == nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"message": "Salio la mkopo halijapatikana"})
	}
	if req.Amount.GreaterThan(*loan.BalanceRemaining) {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": fmt.Sprintf("Kiasi kimezidi salio. Salio ni TZS %s", loan.BalanceRemaining.StringFixed(2)),
		})
	}

	now := time.Now()
	repayment := models.Repayment{
		LoanID:        loan.ID,
		MemberID:      loan.MemberID,
		RecordedBy:    userID,
		Amount:        req.Amount,
		BalanceAfter:  *loan.BalanceRemaining, // unchanged until approval
		PaidAt:        paidAt,
		PaymentMethod: method,
		Notes:         req.Notes,
		Status:        models.RepaymentPending,
		SubmittedBy:   &userID,
		SubmittedAt:   &now,
		ProofImageURL: req.ProofImageURL,
		ProofMessage:  req.ProofMessage,
	}
	if err := database.DB.Create(&repayment).Error; err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"message": "Imeshindikana kuwasilisha malipo"})
	}

	services.LogAudit(c, &userID, models.AuditCreate, "repayments", &repayment.ID, nil, map[string]interface{}{
		"loan_id": loan.ID, "amount": req.Amount, "status": "PENDING",
	})
	services.NotifyRole(models.RoleTreasurer, models.NotifRepayment, "Marejesho Yanasubiri Uthibitisho",
		fmt.Sprintf("Mwanachama amewasilisha marejesho ya TZS %s. Subiri uthibitisho wako.", req.Amount.StringFixed(0)), "")

	return c.Status(fiber.StatusCreated).JSON(fiber.Map{
		"message": "Malipo yamewasilishwa. Inasubiri uthibitisho wa Mweka Hazina.",
		"data":    repayment,
	})
}

// PendingQueue lists PENDING repayments group-wide (treasurer's approval
// queue — mirrors michango/pending).
// GET /api/v1/groups/:id/repayments?status=pending
func (h *RepaymentHandler) PendingQueue(c *fiber.Ctx) error {
	if ok, err := database.IsCurrentGroup(c.Params("id")); err != nil || !ok {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"message": "Kikundi hakijapatikana"})
	}
	status := c.Query("status")
	if status == "" {
		status = models.RepaymentPending
	}
	var rows []models.Repayment
	database.DB.
		Preload("Member", func(db *gorm.DB) *gorm.DB {
			return db.Select("id, member_no, full_name, phone")
		}).
		Preload("Loan", func(db *gorm.DB) *gorm.DB {
			return db.Select("id, amount, approved_amount, balance_remaining, status, due_date")
		}).
		Where("repayments.status = ?", status).
		Order("repayments.created_at ASC").
		Find(&rows)
	return c.JSON(fiber.Map{"data": rows, "total": len(rows)})
}

// Approve confirms a member-submitted repayment (TREASURER ONLY). Only here
// does money move: balance drops, schedule fills oldest-first, zero closes
// the loan (same completion rule as offsets and direct recordings).
// PATCH /api/v1/repayments/:id/approve
func (h *RepaymentHandler) Approve(c *fiber.Ctx) error {
	id := c.Params("id")
	userID := middleware.GetUserID(c)

	tx := database.DB.Begin()
	defer tx.Rollback()

	var repayment models.Repayment
	if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&repayment, "id = ?", id).Error; err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"message": "Malipo hayajapatikana"})
	}
	if repayment.Status != models.RepaymentPending {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Malipo haya hayawezi kuidhinishwa (hali: " + repayment.Status + ")",
		})
	}

	var loan models.Loan
	if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&loan, "id = ?", repayment.LoanID).Error; err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"message": "Mkopo haujapatikana"})
	}
	if loan.Status != models.LoanOutstanding {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": fmt.Sprintf("Mkopo huu hauhitaji malipo. Hali yake ni: %s", loan.Status),
		})
	}
	if loan.BalanceRemaining == nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"message": "Salio la mkopo halijapatikana"})
	}

	balance := *loan.BalanceRemaining
	amount := repayment.Amount
	if amount.GreaterThan(balance) {
		amount = balance // cap: never drive negative (balance shrank meanwhile)
	}
	newBalance := balance.Sub(amount)
	newStatus := models.LoanOutstanding
	loanClosed := false
	if newBalance.LessThan(decimal.NewFromFloat(0.001)) {
		newBalance = decimal.Zero
		newStatus = models.LoanClosed
		loanClosed = true
	}

	// Oldest-due-first schedule allocation (same order as direct recordings).
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

	now := time.Now()
	repayment.Status = models.RepaymentConfirmed
	repayment.Amount = amount
	repayment.BalanceAfter = newBalance
	repayment.ReviewedBy = &userID
	repayment.ReviewedAt = &now
	if err := tx.Save(&repayment).Error; err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"message": "Imeshindikana kuidhinisha"})
	}
	loan.BalanceRemaining = &newBalance
	loan.Status = newStatus
	if err := tx.Save(&loan).Error; err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"message": "Imeshindikana kusasisha mkopo"})
	}
	if err := tx.Commit().Error; err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"message": "Imeshindikana kuidhinisha"})
	}

	services.LogAudit(c, &userID, models.AuditApprove, "repayments", &repayment.ID,
		map[string]interface{}{"status": models.RepaymentPending},
		map[string]interface{}{"status": models.RepaymentConfirmed, "amount": amount,
			"balance_after": newBalance, "loan_closed": loanClosed})
	var notifUserID string
	var member models.Member
	if err := database.DB.First(&member, "id = ?", loan.MemberID).Error; err == nil {
		if member.UserID != nil {
			notifUserID = *member.UserID
		}
		if notifUserID == "" {
			notifUserID = member.RegisteredBy
		}
		if err := services.PostRepayment(member.MemberNo, amount, repayment.PaidAt, userID,
			fmt.Sprintf("Marejesho %s mkopo %s", member.MemberNo, loan.ID)); err != nil {
			log.Printf("WARN: ledger auto-post approved repayment %s: %v", repayment.ID, err)
		}
	}
	if notifUserID != "" {
		msg := fmt.Sprintf("Marejesho yako ya TZS %s yamethibitishwa. Salio: TZS %s.", amount.StringFixed(0), newBalance.StringFixed(0))
		if loanClosed {
			msg = "Hongera! Mkopo wako umelipwa kikamilifu."
		}
		services.NotifyUser(notifUserID, models.NotifRepayment, "Marejesho Yamethibitishwa", msg)
	}

	respMsg := fmt.Sprintf("Marejesho yamethibitishwa. Salio: TZS %s", newBalance.StringFixed(2))
	if loanClosed {
		respMsg = "Marejesho yamethibitishwa. Mkopo umefungwa kikamilifu!"
	}
	return c.JSON(fiber.Map{"message": respMsg, "data": repayment})
}

// Reject declines a member-submitted repayment (TREASURER ONLY, reason
// required — same pattern as contribution rejections).
// PATCH /api/v1/repayments/:id/reject
func (h *RepaymentHandler) Reject(c *fiber.Ctx) error {
	id := c.Params("id")
	var req struct {
		Reason string `json:"reason"`
	}
	if err := c.BodyParser(&req); err != nil || req.Reason == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"message": "Sababu ya kukataa inahitajika"})
	}
	userID := middleware.GetUserID(c)

	var repayment models.Repayment
	if err := database.DB.First(&repayment, "id = ?", id).Error; err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"message": "Malipo hayajapatikana"})
	}
	if repayment.Status != models.RepaymentPending {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Malipo haya hayawezi kukataliwa (hali: " + repayment.Status + ")",
		})
	}

	now := time.Now()
	repayment.Status = models.RepaymentRejected
	repayment.ReviewedBy = &userID
	repayment.ReviewedAt = &now
	repayment.ReviewReason = &req.Reason
	if err := database.DB.Save(&repayment).Error; err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"message": "Imeshindikana kukataa"})
	}

	services.LogAudit(c, &userID, models.AuditReject, "repayments", &repayment.ID,
		map[string]interface{}{"status": models.RepaymentPending},
		map[string]interface{}{"status": models.RepaymentRejected, "reason": req.Reason})
	var member models.Member
	if err := database.DB.First(&member, "id = ?", repayment.MemberID).Error; err == nil && member.UserID != nil {
		services.NotifyUser(*member.UserID, models.NotifRepayment, "Marejesho Yamekataliwa",
			fmt.Sprintf("Marejesho yako ya TZS %s yamekataliwa. Sababu: %s", repayment.Amount.StringFixed(0), req.Reason))
	}

	return c.JSON(fiber.Map{"message": "Marejesho yamekataliwa", "data": repayment})
}
