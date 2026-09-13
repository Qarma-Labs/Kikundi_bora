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
)

type GroupSettingsHandler struct{}

func NewGroupSettingsHandler() *GroupSettingsHandler {
	return &GroupSettingsHandler{}
}

func loadPendingProposal(groupID string) *models.GroupSettingProposal {
	return loadPendingProposalOfKind(groupID, "")
}

// loadPendingProposalOfKind returns the latest PENDING proposal of a given
// kind ("" = any kind, legacy behavior). One PENDING proposal may exist PER
// KIND — e.g. a pending contribution proposal must not block a loan-settings
// proposal. Legacy rows with an empty kind count as contribution proposals.
func loadPendingProposalOfKind(groupID, kind string) *models.GroupSettingProposal {
	q := database.DB.
		Preload("Proposer", func(db *gorm.DB) *gorm.DB { return db.Select("id, name, role") }).
		Where("group_id = ? AND status = ?", groupID, models.ProposalPending)
	if kind != "" {
		if kind == models.ProposalKindContribution {
			q = q.Where("(proposal_kind = ? OR proposal_kind = '' OR proposal_kind IS NULL)", kind)
		} else {
			q = q.Where("proposal_kind = ?", kind)
		}
	}
	var p models.GroupSettingProposal
	if err := q.Order("created_at DESC").First(&p).Error; err != nil {
		return nil
	}
	return &p
}

func pendingProposalOfKind(groupID, kind string) *models.GroupSettingProposal {
	p := loadPendingProposalOfKind(groupID, kind)
	if p == nil {
		return nil
	}
	if p.ProposalKind == "" {
		p.ProposalKind = models.ProposalKindContribution
	}
	if p.ProposalKind != kind {
		return nil
	}
	return p
}

// GET /api/v1/groups/current — convenience for the frontend (single-group
// deployment): resolves the group id, then returns the settings payload.
func (h *GroupSettingsHandler) GetCurrent(c *fiber.Ctx) error {
	g, err := database.GetCurrentGroup()
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"message": "Imeshindikana kupata kikundi"})
	}
	return h.settingsResponse(c, g)
}

// GET /api/v1/groups/:id/contribution-settings — any authenticated user
// (members need read-only settings for the submission form and banners).
func (h *GroupSettingsHandler) GetSettings(c *fiber.Ctx) error {
	if ok, err := database.IsCurrentGroup(c.Params("id")); err != nil || !ok {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"message": "Kikundi hakijapatikana"})
	}
	var g models.Group
	if err := database.DB.First(&g, "id = ?", c.Params("id")).Error; err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"message": "Kikundi hakijapatikana"})
	}
	return h.settingsResponse(c, &g)
}

func (h *GroupSettingsHandler) settingsResponse(c *fiber.Ctx, g *models.Group) error {
	nextDue := ""
	if g.ContributionDueDate != nil && g.ContributionInterval != "" {
		if due, ok := services.NextContributionDueDate(g.ContributionInterval, *g.ContributionDueDate, time.Now()); ok {
			nextDue = due.Format("2006-01-02")
		}
	}

	// Per-member current-cycle contribution status: has the requesting user
	// (if they hold a member row) already submitted their AKIBA contribution
	// for the currently open cycle?
	var myContribution *myCycleStatus
	if userID := middleware.GetUserID(c); userID != "" {
		var member models.Member
		if err := database.DB.Where("user_id = ? AND deleted_at IS NULL", userID).
			First(&member).Error; err == nil {
			myContribution = h.cycleStatusForMember(g, member.ID, time.Now())
		}
	}

	return c.JSON(fiber.Map{
		"data":             g,
		"pending_proposal": pendingProposalOfKind(g.ID, models.ProposalKindContribution),
		"next_due_date":    nextDue,
		"my_contribution":  myContribution,
	})
}

type myCycleStatus struct {
	Status        string `json:"status"` // "none" | "pending" | "confirmed"
	PeriodDueDate string `json:"period_due_date,omitempty"`
}

// cycleStatusForMember checks BOTH contribution stores inside the current
// cycle window (prevDue, due]:
//   - member_contributions: self-submitted (Weka Mchango), created_at
//     CONFIRMED → "confirmed"; only PENDING_VERIFICATION → "pending"
//   - contributions: treasurer-recorded (Pokea Mchango), PAID paid_at → "confirmed"
func (h *GroupSettingsHandler) cycleStatusForMember(g *models.Group, memberID string, now time.Time) *myCycleStatus {
	start, end, ok := services.ContributionCycleWindow(g, now)
	if !ok {
		return nil
	}
	dueStr := end.Format("2006-01-02")

	var mcConfirmed, legacyConfirmed int64
	database.DB.Model(&models.MemberContribution{}).
		Where("member_id = ? AND contribution_type = ? AND status = ? AND created_at > ? AND created_at <= ?",
			memberID, models.ContributionAkiba, models.ContributionConfirmed, start, end).
		Count(&mcConfirmed)
	database.DB.Model(&models.Contribution{}).
		Where("member_id = ? AND status = ? AND paid_at > ? AND paid_at <= ?",
			memberID, "PAID", start, end).
		Count(&legacyConfirmed)
	confirmed := mcConfirmed + legacyConfirmed

	if confirmed > 0 {
		return &myCycleStatus{Status: "confirmed", PeriodDueDate: dueStr}
	}

	var pending int64
	database.DB.Model(&models.MemberContribution{}).
		Where("member_id = ? AND contribution_type = ? AND status = ? AND created_at > ? AND created_at <= ?",
			memberID, models.ContributionAkiba, models.ContributionPending, start, end).
		Count(&pending)
	if pending > 0 {
		return &myCycleStatus{Status: "pending", PeriodDueDate: dueStr}
	}

	return &myCycleStatus{Status: "none", PeriodDueDate: dueStr}
}

type proposeRequest struct {
	ContributionInterval    string           `json:"contribution_interval"`
	ContributionDueDate     string           `json:"contribution_due_date"`
	FixedContributionAmount *decimal.Decimal `json:"fixed_contribution_amount"`
}

// profileUpdateRequest patches group profile metadata (name / location /
// founded year). Direct edit — deliberately OUTSIDE the contribution
// proposal flow (chair-propose + secretary-approve), which governs ONLY
// contribution interval/due-date/amount.
type profileUpdateRequest struct {
	Name        *string `json:"name"`
	Location    *string `json:"location"`
	FoundedYear *int    `json:"founded_year"`
}

// PATCH /api/v1/groups/:id/profile — Mwenyekiti + Msimamizi only.
// Msimamizi (system admin) manages platform/group metadata but stays out
// of group financial operations (see contribution-settings guards).
func (h *GroupSettingsHandler) UpdateProfile(c *fiber.Ctx) error {
	if ok, err := database.IsCurrentGroup(c.Params("id")); err != nil || !ok {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"message": "Kikundi hakijapatikana"})
	}
	var g models.Group
	if err := database.DB.First(&g, "id = ?", c.Params("id")).Error; err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"message": "Kikundi hakijapatikana"})
	}

	var req profileUpdateRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"message": "Data si sahihi"})
	}
	if req.Name == nil && req.Location == nil && req.FoundedYear == nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"message": "Hakuna mabadiliko yaliyotumwa"})
	}

	if req.Name != nil {
		name := strings.TrimSpace(*req.Name)
		if len(name) < 2 || len(name) > 150 {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"message": "Jina la kikundi lazima liwe na herufi 2–150"})
		}
		g.Name = name
	}
	if req.Location != nil {
		loc := strings.TrimSpace(*req.Location)
		if len(loc) > 150 {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"message": "Mahali pafupi sana (max herufi 150)"})
		}
		if loc == "" {
			g.Location = nil
		} else {
			g.Location = &loc
		}
	}
	if req.FoundedYear != nil {
		if *req.FoundedYear < 1900 || *req.FoundedYear > time.Now().Year() {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"message": "Mwaka ulioanzishwa si sahihi"})
		}
		g.FoundedYear = req.FoundedYear
	}

	if err := database.DB.Save(&g).Error; err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"message": "Imeshindikana kuhifadhi taarifa za kikundi"})
	}

	userID := middleware.GetUserID(c)
	services.LogAudit(c, &userID, models.AuditUpdate, "groups", &g.ID, nil, map[string]interface{}{
		"name": g.Name, "location": g.Location, "founded_year": g.FoundedYear,
	})

	return c.JSON(fiber.Map{"message": "Taarifa za kikundi zimehifadhiwa.", "data": g})
}

// POST /api/v1/groups/:id/contribution-settings/propose — Mwenyekiti only.
func (h *GroupSettingsHandler) Propose(c *fiber.Ctx) error {
	var req proposeRequest
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

	if err := services.ValidateProposalSpec(req.ContributionInterval, req.ContributionDueDate, req.FixedContributionAmount); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"message": err.Error()})
	}

	// One PENDING contribution proposal at a time (other kinds are independent).
	if existing := loadPendingProposalOfKind(g.ID, models.ProposalKindContribution); existing != nil {
		return c.Status(fiber.StatusConflict).JSON(fiber.Map{
			"message": "Kuna pendekezo lililopo bado halijajibiwa. Katibu lazima alipe Wakati wa kwanza kabla ya pendekezo jipya.",
			"data":    existing,
		})
	}

	userID := middleware.GetUserID(c)
	proposal := models.GroupSettingProposal{
		GroupID:                 g.ID,
		ProposalKind:            models.ProposalKindContribution,
		ContributionInterval:    models.ContributionInterval(req.ContributionInterval),
		ContributionDueDate:     &req.ContributionDueDate,
		FixedContributionAmount: req.FixedContributionAmount,
		Status:                  models.ProposalPending,
		ProposedBy:              userID,
	}
	if err := database.DB.Create(&proposal).Error; err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"message": "Imeshindikana kutuma pendekezo"})
	}

	services.LogAudit(c, &userID, models.AuditCreate, "group_setting_proposals", &proposal.ID, nil, map[string]interface{}{
		"group_id": g.ID, "interval": req.ContributionInterval, "due_date": req.ContributionDueDate, "amount": req.FixedContributionAmount,
	})

	services.NotifyRole(models.RoleSecretary, models.NotifSystem,
		"Pendekezo la Mipangilio ya Michango",
		"Mwenyekiti amependekeza kipindi cha michango ("+req.ContributionInterval+"). Subiri uthibitisho wako.",
		"",
	)

	return c.Status(fiber.StatusCreated).JSON(fiber.Map{
		"message": "Pendekezo limetumwa kwa Katibu kwa idhini.",
		"data":    proposal,
	})
}

// POST /api/v1/groups/:id/contribution-settings/approve — Katibu only.
// Applies the pending proposal to the group settings.
func (h *GroupSettingsHandler) Approve(c *fiber.Ctx) error {
	if ok, err := database.IsCurrentGroup(c.Params("id")); err != nil || !ok {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"message": "Kikundi hakijapatikana"})
	}
	var g models.Group
	if err := database.DB.First(&g, "id = ?", c.Params("id")).Error; err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"message": "Kikundi hakijapatikana"})
	}

	proposal := loadPendingProposalOfKind(g.ID, models.ProposalKindContribution)
	if proposal == nil {
		// Legacy fallback: fines proposals (never created via API anymore)
		// were approved through this same endpoint.
		proposal = loadPendingProposalOfKind(g.ID, models.ProposalKindFines)
	}
	if proposal == nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"message": "Hakuna pendekezo lililosubiri"})
	}

	userID := middleware.GetUserID(c)
	now := time.Now()

	if proposal.ProposalKind == models.ProposalKindFines {
		if err := applyFineProposal(g, proposal); err != nil {
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"message": "Imeshindikana kuhifadhi mipangilio ya faini"})
		}
	} else {
		// Apply to group — the ONLY path that mutates group contribution settings
		g.ContributionInterval = proposal.ContributionInterval
		g.ContributionDueDate = proposal.ContributionDueDate
		g.FixedContributionAmount = proposal.FixedContributionAmount
		if err := database.DB.Save(&g).Error; err != nil {
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"message": "Imeshindikana kuhifadhi mipangilio"})
		}
	}

	proposal.Status = models.ProposalApproved
	proposal.ApprovedBy = &userID
	proposal.ReviewedAt = &now
	if err := database.DB.Save(proposal).Error; err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"message": "Imeshindikana kuhifadhi pendekezo"})
	}

	services.LogAudit(c, &userID, models.AuditApprove, "group_setting_proposals", &proposal.ID,
		map[string]interface{}{"status": models.ProposalPending},
		map[string]interface{}{"status": models.ProposalApproved, "interval": g.ContributionInterval, "due_date": g.ContributionDueDate, "amount": g.FixedContributionAmount},
	)

	if proposal.ProposalKind == models.ProposalKindFines {
		services.NotifyRole(models.RoleChair, models.NotifSystem,
			"Mipangilio ya Faini Imeidhinishwa",
			"Katibu ameidhinisha mipangilio mpya ya faini. Sasa inatumika.",
			"",
		)
		settings, _ := database.GetOrCreateFineSettings(g.ID)
		return c.JSON(fiber.Map{"message": "Mipangilio ya faini imeidhinishwa na sasa inatumika.", "data": settings})
	}

	services.NotifyRole(models.RoleChair, models.NotifSystem,
		"Mipangilio ya Michango Imeidhinishwa",
		"Katibu ameidhinisha mipangilio mpya ya michango. Sasa inatumika.",
		"",
	)

	return c.JSON(fiber.Map{"message": "Mipangilio imeidhinishwa na sasa inatumika.", "data": g})
}

// POST /api/v1/groups/:id/contribution-settings/reject — Katibu only.
func (h *GroupSettingsHandler) Reject(c *fiber.Ctx) error {
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

	proposal := loadPendingProposalOfKind(g.ID, models.ProposalKindContribution)
	if proposal == nil {
		// Legacy fallback: fines proposals (never created via API anymore)
		// were approved through this same endpoint.
		proposal = loadPendingProposalOfKind(g.ID, models.ProposalKindFines)
	}
	if proposal == nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"message": "Hakuna pendekezo lililosubiri"})
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
		map[string]interface{}{"status": models.ProposalRejected, "reason": req.Reason},
	)

	services.NotifyRole(models.RoleChair, models.NotifSystem,
		"Pendekezo la Mipangilio Limekataliwa",
		"Katibu amekatalia pendekezo la mipangilio ya michango. Sababu: "+req.Reason,
		"",
	)

	return c.JSON(fiber.Map{"message": "Pendekezo limekataliwa.", "data": proposal})
}
