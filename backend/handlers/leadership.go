package handlers

import (
	"strings"
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

type LeadershipHandler struct{}

var treasurySvc = services.NewTreasuryService()

func NewLeadershipHandler() *LeadershipHandler {
	return &LeadershipHandler{}
}

// QuickStats returns quick statistics for the "Takwimu za Haraka" dashboard section.
// GET /api/v1/uongozi/quick-stats
func (h *LeadershipHandler) QuickStats(c *fiber.Ctx) error {
	// Get total members count (approved only — pending must not inflate totals)
	var totalMembers int64
	database.DB.Model(&models.Member{}).
		Where("deleted_at IS NULL AND approval_status = 'approved'").
		Count(&totalMembers)

	// Get contributions this month from NEW MemberContribution table (Phase 4)
	var contributionsThisMonth float64
	startOfMonth := time.Date(time.Now().Year(), time.Now().Month(), 1, 0, 0, 0, 0, time.UTC)
	database.DB.Model(&models.MemberContribution{}).
		Where("status = ? AND created_at >= ?", models.ContributionConfirmed, startOfMonth).
		Select("COALESCE(SUM(amount), 0)").
		Scan(&contributionsThisMonth)

	// Also check OLD Contribution table for legacy data
	var legacyContributionsThisMonth float64
	database.DB.Model(&models.Contribution{}).
		Where("status = 'PAID' AND paid_at >= ?", startOfMonth).
		Select("COALESCE(SUM(amount), 0)").
		Scan(&legacyContributionsThisMonth)
	contributionsThisMonth += legacyContributionsThisMonth

	// Get pending contributions count
	var pendingContributions int64
	database.DB.Model(&models.MemberContribution{}).
		Where("status = ?", models.ContributionPending).
		Count(&pendingContributions)

	// Get outstanding loans
	var outstandingLoans int64
	database.DB.Model(&models.Loan{}).
		Where("status = ?", models.LoanOutstanding).
		Count(&outstandingLoans)

	// Get pending loans
	var pendingLoans int64
	database.DB.Model(&models.Loan{}).
		Where("status = ?", models.LoanPending).
		Count(&pendingLoans)

	// Get treasury balance
	treasury, err := treasurySvc.CalculateHazinaBalance()
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"message": "Imeshindikana kupata hesabu ya hazina",
		})
	}

	return c.JSON(fiber.Map{
		"total_members":          totalMembers,
		"contributions_month":    contributionsThisMonth,
		"pending_contributions":  pendingContributions,
		"outstanding_loans":      outstandingLoans,
		"pending_loans":          pendingLoans,
		"treasury_balance":       treasury.AvailableBalance,
		"total_contributions":    treasury.TotalContributions,
		"total_repayments":       treasury.TotalRepayments,
		"total_disbursed":        treasury.TotalDisbursed,
	})
}

// PendingLoans returns loans in the parallel approval set
// (Hazina + Katibu + Bodi + Mwenyekiti — kila mtu aidhinisha kwa muda
// wake, mkopo hautoki mpaka WOTE waidhinishe):
//   - awaiting_role: first still-missing stage (compat display only)
//   - my_turn: whether the CALLER's own stage is still pending
//
// Supports ?my_turn=true for the role-scoped "loans awaiting MY action" view.
// GET /api/v1/uongozi/mikopo/pending
// NOTE: must include UNDER_REVIEW — committee review flips PENDING→
// UNDER_REVIEW on first vote, and Hazina/Katibu chain stages must still
// see + act on those loans. Filtering PENDING only made the queue go
// empty for Mweka Hazina ("sioni mkopo uliombwa").
func (h *LeadershipHandler) PendingLoans(c *fiber.Ctx) error {
	var loans []models.Loan
	if err := database.DB.
		Where("status IN ?", []models.LoanStatus{models.LoanPending, models.LoanUnderReview}).
		Preload("Member", func(db *gorm.DB) *gorm.DB {
			return db.Select("id, member_no, full_name, phone")
		}).
		Order("applied_at DESC"). // NOTE: loans has no created_at column
		Find(&loans).Error; err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"message": "Imeshindikana kupata mikopo",
		})
	}

	role := middleware.GetUserRole(c)
	userID := middleware.GetUserID(c)
	myStage := loanStageForRole(role)
	// Appointed bodi members have role=member: resolve their stage too.
	if myStage == "bodi" || (role == models.RoleMember && isAppointedBodi(userID)) {
		myStage = "bodi"
	}
	myTurnOnly := c.Query("my_turn") == "true"

	out := make([]map[string]interface{}, 0, len(loans))
	for _, loan := range loans {
		stage := loanAwaitingStage(&loan)
		mine := myStage != "" && loanStagePending(&loan, myStage)
		if myTurnOnly && !mine {
			continue
		}
		out = append(out, map[string]interface{}{
			"id":            loan.ID,
			"member_id":     loan.MemberID,
			"amount":        loan.Amount,
			"purpose":       loan.Purpose,
			"due_date":      loan.DueDate,
			"status":        loan.Status,
			"applied_at":    loan.AppliedAt,
			"member":        loan.Member,
			"awaiting_role": stage,
			"my_turn":       mine,
			// Term + interest snapshot so every approval stage can decide
			// on total cost, not just principal.
			"term_days":                loan.TermDays,
			"interest_enabled":         loan.InterestEnabled,
			"interest_type":            loan.InterestType,
			"applicable_interest_rate": loan.ApplicableInterestRate,
			"interest_amount":          loan.InterestAmount,
			"total_repayment":          loan.TotalRepayment,
			// Approval trail timestamps (the pipeline chips in the UI)
			"hazina_approved_at":      loan.HazinaApprovedAt,
			"katibu_approved_at":      loan.KatibuApprovedAt,
			"bodi_approved_at":        loan.BodiApprovedAt,
			"mwenyekiti_approved_at":  loan.MwenyekitiApprovedAt,
		})
	}

	return c.JSON(fiber.Map{
		"data":  out,
		"total": len(out),
	})
}

// loanAwaitingStage returns the first still-missing stage (display compat).
func loanAwaitingStage(l *models.Loan) string {
	switch {
	case l.HazinaApprovedAt == nil:
		return "hazina"
	case l.KatibuApprovedAt == nil:
		return "katibu"
	case l.BodiApprovedAt == nil:
		return "bodi"
	default:
		return "mwenyekiti"
	}
}

// loanStagePending reports whether the given parallel stage is still missing.
func loanStagePending(l *models.Loan, stage string) bool {
	switch stage {
	case "hazina":
		return l.HazinaApprovedAt == nil
	case "katibu":
		return l.KatibuApprovedAt == nil
	case "bodi":
		return l.BodiApprovedAt == nil
	case "mwenyekiti":
		return l.MwenyekitiApprovedAt == nil
	default:
		return false
	}
}

// loanAllStagesDone reports whether Hazina+Katibu+Bodi+Mwenyekiti all signed.
func loanAllStagesDone(l *models.Loan) bool {
	return l.HazinaApprovedAt != nil && l.KatibuApprovedAt != nil &&
		l.BodiApprovedAt != nil && l.MwenyekitiApprovedAt != nil
}

// loanPendingStages lists still-missing stages for messages/notifications.
func loanPendingStages(l *models.Loan) []string {
	var out []string
	for _, s := range []string{"hazina", "katibu", "bodi", "mwenyekiti"} {
		if loanStagePending(l, s) {
			out = append(out, s)
		}
	}
	return out
}

func isAppointedBodi(userID string) bool {
	if userID == "" {
		return false
	}
	var n int64
	database.DB.Model(&models.LoanCommitteeMember{}).
		Where("user_id = ? AND is_active = TRUE", userID).
		Count(&n)
	return n > 0
}

// loanStageForRole maps a caller's role to its sequential stage
// ("bodi" covers appointed committee members, whose users.role is member).
func loanStageForRole(role models.Role) string {
	switch role {
	case models.RoleTreasurer:
		return "hazina"
	case models.RoleSecretary:
		return "katibu"
	case models.RoleChair:
		return "mwenyekiti"
	case models.RoleMember:
		return "bodi"
	default:
		return "" // admin has no stage in the chain
	}
}

// notifyLoanStage pings the role that is now on turn in the sequential chain.
func notifyLoanStage(stage string, loan *models.Loan) {
	msg := "Mkopo wa TZS " + formatMoney(loan.Amount) + " unasubiri idhini yako (Hatua: " + stage + ")."
	switch stage {
	case "hazina":
		services.NotifyRole(models.RoleTreasurer, models.NotifLoanUnderReview, "Mkopo: Zamu ya Hazina", msg, "")
	case "katibu":
		services.NotifyRole(models.RoleSecretary, models.NotifLoanUnderReview, "Mkopo: Zamu ya Katibu", msg, "")
	case "bodi":
		var appointed []models.LoanCommitteeMember
		database.DB.Where("is_active = TRUE").Find(&appointed)
		for _, m := range appointed {
			services.NotifyUser(m.UserID, models.NotifLoanUnderReview, "Mkopo: Zamu ya Bodi ya Mikopo", msg)
		}
	case "mwenyekiti":
		services.NotifyRole(models.RoleChair, models.NotifLoanUnderReview, "Mkopo: Zamu ya Mwenyekiti", msg, "")
	}
}

// notifyLoanApplicantUser notifies the loan's applicant (their member's login,
// falling back to the registrar like the committee flow does).
func notifyLoanApplicantUser(loan *models.Loan, ntype models.NotificationType, title, msg string) {
	var member models.Member
	if err := database.DB.First(&member, "id = ?", loan.MemberID).Error; err != nil {
		return
	}
	target := ""
	if member.UserID != nil {
		target = *member.UserID
	}
	if target == "" {
		target = member.RegisteredBy
	}
	if target != "" {
		services.NotifyUser(target, ntype, title, msg)
	}
}

// ApproveLoan handles sequential loan approval: Hazina → Katibu → Bodi → Mwenyekiti.
// POST /api/v1/uongozi/mikopo/:id/approve
func (h *LeadershipHandler) ApproveLoan(c *fiber.Ctx) error {
	id := c.Params("id")
	userID := middleware.GetUserID(c)
	role := middleware.GetUserRole(c)

	var req struct {
		ApprovedAmount decimal.Decimal `json:"approved_amount"`
	}
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"message": "Data si sahihi"})
	}

	tx := database.DB.Begin()

	var loan models.Loan
	if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&loan, "id = ?", id).Error; err != nil {
		tx.Rollback()
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"message": "Mkopo haujapatikana"})
	}

	if loan.Status != models.LoanPending && loan.Status != models.LoanUnderReview {
		tx.Rollback()
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"message": "Mkopo huu hauwezi kuidhinishwa. Hali yake: " + string(loan.Status)})
	}

	// NOTE: borrower MAY sign at own stage (e.g. Mwenyekiti akiomba kama
	// mwanachama afanye sign-off). Parallel rule: kila mtu aidhinisha kwa
	// muda wake, mkopo hautoki (APPROVED) mpaka WOTE waidhinishe.

	now := time.Now()

	// Parallel approvals: no order gate — each role signs own stage anytime.
	// Loan finalizes (APPROVED) only when ALL four stages are done.
	switch role {
	case models.RoleTreasurer:
		if loan.HazinaApprovedAt != nil {
			tx.Rollback()
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"message": "Hazina tayari ameidhinisha mkopo huu"})
		}
		loan.HazinaApprovedBy = &userID
		loan.HazinaApprovedAt = &now

	case models.RoleSecretary:
		if loan.KatibuApprovedAt != nil {
			tx.Rollback()
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"message": "Katibu tayari ameidhinisha mkopo huu"})
		}
		loan.KatibuApprovedBy = &userID
		loan.KatibuApprovedAt = &now

	case models.RoleChair:
		if loan.MwenyekitiApprovedAt != nil {
			tx.Rollback()
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"message": "Mwenyekiti tayari ameidhinisha mkopo huu"})
		}
		loan.MwenyekitiApprovedBy = &userID
		loan.MwenyekitiApprovedAt = &now
		// Chair's amount counts as proposed figure even before others finish.
		if req.ApprovedAmount.GreaterThan(decimal.Zero) {
			amt := req.ApprovedAmount
			loan.ApprovedAmount = &amt
		}

	default:
		// Bodi member (appointed committee member) — parallel, anytime.
		var isCommittee bool
		database.DB.Model(&models.LoanCommitteeMember{}).
			Where("user_id = ? AND is_active = TRUE", userID).
			Select("1").Scan(&isCommittee)
		if !isCommittee {
			tx.Rollback()
			return c.Status(fiber.StatusForbidden).JSON(fiber.Map{"message": "Huna ruhusa ya kuidhinisha mkopo"})
		}
		if loan.BodiApprovedAt != nil {
			tx.Rollback()
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"message": "Bodi tayari imeidhinisha mkopo huu"})
		}
		loan.BodiApprovedBy = &userID
		loan.BodiApprovedAt = &now
	}

	// SYNC: chain signature mirrors to committee reviews so the ukaguzi
	// page counts it (uongozi ↔ ukaguzi agree). Upsert APPROVE; never
	// overwrite an existing REJECT (rejection already finalized the loan,
	// and this handler only runs on PENDING/UNDER_REVIEW anyway).
	var cr models.LoanReview
	if err := tx.Where("loan_id = ? AND reviewer_id = ?", loan.ID, userID).First(&cr).Error; err != nil {
		tx.Create(&models.LoanReview{
			LoanID: loan.ID, ReviewerID: userID,
			Decision: models.ReviewApprove, ReviewedAt: &now,
		})
	} else if cr.Decision == models.ReviewPending {
		cr.Decision = models.ReviewApprove
		cr.ReviewedAt = &now
		tx.Save(&cr)
	}

	// Finalize only when ALL four have signed.
	if loanAllStagesDone(&loan) {
		loan.Status = models.LoanApproved
		if loan.ApprovedAmount == nil {
			loan.ApprovedAmount = &loan.Amount
		}
		if req.ApprovedAmount.GreaterThan(decimal.Zero) {
			amt := req.ApprovedAmount
			loan.ApprovedAmount = &amt
		}
		// Recompute interest on the APPROVED principal using the loan's
		// snapshotted mode/rate (never the live group settings). Interest-free
		// loans stay structurally at zero.
		if loan.InterestEnabled {
			loan.InterestAmount = services.CalcLoanInterest(
				*loan.ApprovedAmount, loan.ApplicableInterestRate, loan.TermDays, loan.InterestType, true)
		} else {
			loan.InterestAmount = decimal.Zero
		}
		loan.TotalRepayment = loan.ApprovedAmount.Add(loan.InterestAmount)
		loan.ReviewedBy = &userID
		loan.ReviewedAt = &now
	}

	if err := tx.Save(&loan).Error; err != nil {
		tx.Rollback()
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"message": "Imeshindikana kuidhinisha"})
	}

	if err := tx.Commit().Error; err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"message": "Imeshindikana kuidhinisha"})
	}

	services.LogAudit(c, &userID, models.AuditLoanReview, "loans", &loan.ID, nil, map[string]interface{}{
		"status": string(loan.Status), "role": string(role),
	})

	// Parallel: ping every still-pending stage; finalize only when none remain.
	stageMsg := ""
	if loan.Status == models.LoanApproved {
		stageMsg = "Mkopo umekamilika idhini zote. Mweka Hazina atautolea fedha."
		// All four signed: tell the borrower + treasury (disbursement).
		notifyLoanApplicantUser(&loan, models.NotifLoanApproved, "Mkopo Umeidhinishwa",
			"Mkopo wako wa TZS "+formatMoney(loan.Amount)+" umekamilika idhini zote. Utatolewa fedha na Mweka Hazina.")
		services.NotifyRole(models.RoleTreasurer, models.NotifLoanApproved, "Mkopo Umeidhinishwa — Toa Fedha",
			"Mkopo wa TZS "+formatMoney(loan.Amount)+" umekamilika idhini zote. Toa fedha (Toa Mkopo).", "")
	} else {
		pending := loanPendingStages(&loan)
		stageMsg = "Imehifadhiwa. Bado idhini za: " + strings.Join(pending, ", ") + "."
		for _, s := range pending {
			notifyLoanStage(s, &loan)
		}
	}

	return c.JSON(fiber.Map{"message": stageMsg, "data": loan})
}

// Reports returns leadership reports (delegates to report handler).
// GET /api/v1/uongozi/ripoti
func (h *LeadershipHandler) Reports(c *fiber.Ctx) error {
	reportType := c.Query("type", "summary")

	reportHandler := NewReportHandler()

	switch reportType {
	case "wanachama":
		return reportHandler.MembersReport(c)
	case "michango":
		return reportHandler.ContributionsReport(c)
	case "mikopo":
		return reportHandler.LoansReport(c)
	case "mapato":
		return reportHandler.IncomeExpenseReport(c)
	case "summary":
		return reportHandler.SummaryReport(c)
	default:
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Aina ya ripoti si sahihi",
		})
	}
}

// Dashboard returns leadership-specific dashboard data.
// GET /api/v1/uongozi/dashboard
func (h *LeadershipHandler) Dashboard(c *fiber.Ctx) error {
	userID := middleware.GetUserID(c)

	// Find member linked to user
	var member models.Member
	if err := database.DB.Where("user_id = ? AND deleted_at IS NULL", userID).
		First(&member).Error; err != nil {
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{
			"message": "Lazima uwe mwanachama wa kikundi",
		})
	}

	// Get leadership roles
	var positions []models.LeadershipPosition
	database.DB.Where("member_id = ? AND is_current = TRUE", member.ID).
		Find(&positions)

	var roles []string
	for _, p := range positions {
		roles = append(roles, string(p.Role))
	}

	// Get pending loans count
	var pendingLoans int64
	database.DB.Model(&models.Loan{}).
		Where("status = ?", models.LoanPending).
		Count(&pendingLoans)

	// Get total members count (approved only — pending must not inflate totals)
	var totalMembers int64
	database.DB.Model(&models.Member{}).
		Where("deleted_at IS NULL AND approval_status = 'approved'").
		Count(&totalMembers)

	// Get outstanding loans count
	var outstandingLoans int64
	database.DB.Model(&models.Loan{}).
		Where("status = ?", models.LoanOutstanding).
		Count(&outstandingLoans)

	return c.JSON(fiber.Map{
		"member_id":       member.ID,
		"member_code":     member.MemberNo,
		"leadership":      roles,
		"pending_loans":   pendingLoans,
		"total_members":   totalMembers,
		"outstanding_loans": outstandingLoans,
	})
}
