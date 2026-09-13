package handlers

import (
	"time"

	"kikundibora/database"
	"kikundibora/middleware"
	"kikundibora/models"
	"kikundibora/services"

	"github.com/gofiber/fiber/v2"
	"github.com/shopspring/decimal"
)

type LoanSettingsHandler struct{}

func NewLoanSettingsHandler() *LoanSettingsHandler { return &LoanSettingsHandler{} }

type loanSettingsProposeRequest struct {
	InterestEnabled     *bool            `json:"interest_enabled"`
	DefaultInterestRate *decimal.Decimal `json:"default_interest_rate"`
	InterestType        *string          `json:"interest_type"`
	MinTermDays         *int             `json:"min_term_days"`
	MaxTermDays         *int             `json:"max_term_days"`
}

func validateLoanSettingsProposal(req loanSettingsProposeRequest) (bool, decimal.Decimal, string, int, int, string) {
	enabled := false
	if req.InterestEnabled != nil {
		enabled = *req.InterestEnabled
	}
	rate := decimal.Zero
	if req.DefaultInterestRate != nil {
		rate = *req.DefaultInterestRate
	}
	itype := models.LoanInterestFlat
	if req.InterestType != nil && *req.InterestType != "" {
		itype = *req.InterestType
	}
	minDays, maxDays := 14, 365
	if req.MinTermDays != nil {
		minDays = *req.MinTermDays
	}
	if req.MaxTermDays != nil {
		maxDays = *req.MaxTermDays
	}

	if !models.IsValidLoanInterestType(itype) {
		return false, decimal.Zero, "", 0, 0, "Aina ya riba si sahihi (flat au reducing)"
	}
	if minDays < 1 || maxDays < 1 {
		return false, decimal.Zero, "", 0, 0, "Muda wa mkopo lazima uwe siku 1 au zaidi"
	}
	if minDays > maxDays {
		return false, decimal.Zero, "", 0, 0, "Muda wa chini hauwezi kuzidi muda wa juu"
	}
	if maxDays > 3650 {
		return false, decimal.Zero, "", 0, 0, "Muda wa juu umezidi kikomo (siku 3650)"
	}
	if enabled {
		if rate.LessThanOrEqual(decimal.Zero) {
			return false, decimal.Zero, "", 0, 0, "Riba ikiwezeshwa, kiwango lazima kiwe zaidi ya sifuri"
		}
		if rate.GreaterThan(decimal.NewFromInt(100)) {
			return false, decimal.Zero, "", 0, 0, "Kiwango cha riba kimezidi 100% kwa mwezi"
		}
	} else {
		// Interest-free is structural: rate/type are neutralized at apply
		// time regardless of what is proposed alongside.
		rate = decimal.Zero
		itype = models.LoanInterestFlat
	}
	return enabled, rate, itype, minDays, maxDays, ""
}

// GET /api/v1/groups/:id/loan-settings — any authenticated user (applicants
// need read-only settings for the term/rate form).
func (h *LoanSettingsHandler) Get(c *fiber.Ctx) error {
	if ok, err := database.IsCurrentGroup(c.Params("id")); err != nil || !ok {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"message": "Kikundi hakijapatikana"})
	}
	var g models.Group
	if err := database.DB.First(&g, "id = ?", c.Params("id")).Error; err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"message": "Kikundi hakijapatikana"})
	}
	settings, err := database.GetOrCreateLoanSettings(g.ID)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"message": "Imeshindikana kupata mipangilio ya mikopo"})
	}
	return c.JSON(fiber.Map{
		"data":             settings,
		"pending_proposal": pendingProposalOfKind(g.ID, models.ProposalKindLoan),
	})
}

// POST /api/v1/groups/:id/loan-settings/propose — Mwenyekiti only.
func (h *LoanSettingsHandler) Propose(c *fiber.Ctx) error {
	var req loanSettingsProposeRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"message": "Data si sahihi"})
	}
	if ok, err := database.IsCurrentGroup(c.Params("id")); err != nil || !ok {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"message": "Kikundi hakijapatikana"})
	}
	var g models.Group
	if err := database.DB.First(&g, "id = ?", c.Params("id")).Error; err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"message": "Kikundi hakijapatikana"})
	}

	enabled, rate, itype, minDays, maxDays, msg := validateLoanSettingsProposal(req)
	if msg != "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"message": msg})
	}

	// One PENDING proposal per group across ALL kinds.
	if existing := loadPendingProposal(g.ID); existing != nil {
		return c.Status(fiber.StatusConflict).JSON(fiber.Map{
			"message": "Kuna pendekezo lililopo bado halijajibiwa. Katibu lazima alijibu kwanza kabla ya pendekezo jipya.",
			"data":    existing,
		})
	}

	userID := middleware.GetUserID(c)
	proposal := models.GroupSettingProposal{
		GroupID:                   g.ID,
		ProposalKind:              models.ProposalKindLoan,
		Status:                    models.ProposalPending,
		ProposedBy:                userID,
		LoanInterestEnabled:       &enabled,
		LoanDefaultInterestRate:   &rate,
		LoanInterestType:          &itype,
		LoanMinTermDays:           &minDays,
		LoanMaxTermDays:           &maxDays,
		ContributionInterval:      g.ContributionInterval,
		ContributionDueDate:       g.ContributionDueDate,
		FixedContributionAmount:   g.FixedContributionAmount,
	}
	if err := database.DB.Create(&proposal).Error; err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"message": "Imeshindikana kutuma pendekezo"})
	}

	services.LogAudit(c, &userID, models.AuditCreate, "group_setting_proposals", &proposal.ID, nil, map[string]interface{}{
		"group_id": g.ID, "kind": "loan", "interest_enabled": enabled,
		"rate": rate, "type": itype, "min_days": minDays, "max_days": maxDays,
	})
	services.NotifyRole(models.RoleSecretary, models.NotifSystem,
		"Pendekezo la Mipangilio ya Mikopo",
		"Mwenyekiti amependekeza mipangilio ya mikopo (riba/muda). Subiri uthibitisho wako.", "")

	return c.Status(fiber.StatusCreated).JSON(fiber.Map{
		"message": "Pendekezo limetumwa kwa Katibu kwa idhini.",
		"data":    proposal,
	})
}

// POST /api/v1/groups/:id/loan-settings/approve — Katibu only.
func (h *LoanSettingsHandler) Approve(c *fiber.Ctx) error {
	if ok, err := database.IsCurrentGroup(c.Params("id")); err != nil || !ok {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"message": "Kikundi hakijapatikana"})
	}
	var g models.Group
	if err := database.DB.First(&g, "id = ?", c.Params("id")).Error; err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"message": "Kikundi hakijapatikana"})
	}
	proposal := pendingProposalOfKind(g.ID, models.ProposalKindLoan)
	if proposal == nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"message": "Hakuna pendekezo la mikopo lililosubiri"})
	}
	if proposal.LoanInterestEnabled == nil || proposal.LoanMinTermDays == nil || proposal.LoanMaxTermDays == nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"message": "Pendekezo hili halina taarifa za mikopo"})
	}

	settings, err := database.GetOrCreateLoanSettings(g.ID)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"message": "Imeshindikana kupata mipangilio"})
	}
	userID := middleware.GetUserID(c)
	now := time.Now()

	settings.InterestEnabled = *proposal.LoanInterestEnabled
	if proposal.LoanDefaultInterestRate != nil {
		settings.DefaultInterestRate = *proposal.LoanDefaultInterestRate
	}
	if proposal.LoanInterestType != nil && *proposal.LoanInterestType != "" {
		settings.InterestType = *proposal.LoanInterestType
	}
	settings.MinTermDays = *proposal.LoanMinTermDays
	settings.MaxTermDays = *proposal.LoanMaxTermDays
	settings.UpdatedBy = &userID
	if err := database.DB.Save(settings).Error; err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"message": "Imeshindikana kuhifadhi mipangilio"})
	}

	proposal.Status = models.ProposalApproved
	proposal.ApprovedBy = &userID
	proposal.ReviewedAt = &now
	if err := database.DB.Save(proposal).Error; err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"message": "Imeshindikana kuhifadhi pendekezo"})
	}

	services.LogAudit(c, &userID, models.AuditApprove, "group_setting_proposals", &proposal.ID,
		map[string]interface{}{"status": models.ProposalPending},
		map[string]interface{}{"status": models.ProposalApproved, "kind": "loan",
			"interest_enabled": settings.InterestEnabled, "rate": settings.DefaultInterestRate,
			"type": settings.InterestType, "min_days": settings.MinTermDays, "max_days": settings.MaxTermDays})
	services.NotifyRole(models.RoleChair, models.NotifSystem,
		"Mipangilio ya Mikopo Imeidhinishwa",
		"Katibu ameidhinisha mipangilio mpya ya mikopo. Sasa inatumika.", "")

	return c.JSON(fiber.Map{"message": "Mipangilio ya mikopo imeidhinishwa na sasa inatumika.", "data": settings})
}

// POST /api/v1/groups/:id/loan-settings/reject — Katibu only.
func (h *LoanSettingsHandler) Reject(c *fiber.Ctx) error {
	var req struct {
		Reason string `json:"reason"`
	}
	if err := c.BodyParser(&req); err != nil || req.Reason == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"message": "Sababu ya kukataa inahitajika"})
	}
	if ok, err := database.IsCurrentGroup(c.Params("id")); err != nil || !ok {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"message": "Kikundi hakijapatikana"})
	}
	var g models.Group
	if err := database.DB.First(&g, "id = ?", c.Params("id")).Error; err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"message": "Kikundi hakijapatikana"})
	}
	proposal := pendingProposalOfKind(g.ID, models.ProposalKindLoan)
	if proposal == nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"message": "Hakuna pendekezo la mikopo lililosubiri"})
	}

	userID := middleware.GetUserID(c)
	now := time.Now()
	proposal.Status = models.ProposalRejected
	proposal.ApprovedBy = &userID
	proposal.RejectionReason = &req.Reason
	proposal.ReviewedAt = &now
	if err := database.DB.Save(proposal).Error; err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"message": "Imeshindikana kuhifadhi pendekezo"})
	}
	services.LogAudit(c, &userID, models.AuditReject, "group_setting_proposals", &proposal.ID,
		map[string]interface{}{"status": models.ProposalPending},
		map[string]interface{}{"status": models.ProposalRejected, "reason": req.Reason})
	services.NotifyRole(models.RoleChair, models.NotifSystem,
		"Pendekezo la Mikopo Limekataliwa",
		"Katibu amekatalia pendekezo la mipangilio ya mikopo. Sababu: "+req.Reason, "")

	return c.JSON(fiber.Map{"message": "Pendekezo limekataliwa.", "data": proposal})
}
