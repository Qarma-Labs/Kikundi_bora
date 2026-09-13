package main

import (
	"context"
	"flag"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"kikundibora/config"
	"kikundibora/database"
	"kikundibora/handlers"
	"kikundibora/ledger"
	"kikundibora/middleware"
	"kikundibora/models"
	"kikundibora/services"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
)

func main() {
	migrateFlag := flag.Bool("migrate", false, "Run database migration and seed, then exit")
	seedFlag := flag.Bool("seed", false, "Run seed only (no table drop), then exit")
	replayLedgerFlag := flag.String("replay-ledger", "", "Rebuild ledger projections from the event log: '' skip, 'group' this group's scope, 'all' every group")
	backfillLedgerFlag := flag.Bool("backfill-ledger", false, "Post one opening-balance ledger transaction per member for pre-existing PAID contributions (fresh ledger only), then continue")
	serveFlag := flag.Bool("serve", true, "Start the HTTP server (set -serve=false with -replay-ledger for CLI-style runs)")
	flag.Parse()

	config.Load()
	database.Connect()
	services.InitSentry()
	services.InitEmail()
	services.InitSMS()

	// Auto-migrate on every boot (idempotent — GORM never drops tables).
	// This self-heals existing databases when new models are added
	// (e.g. fine_settings/fines); previously migration only ran with
	// -migrate on empty DBs, so old DBs hit "relation does not exist".
	database.AutoMigrate()

	// Ensure leadership positions and group defaults exist on every startup (idempotent)
	database.EnsureLeadershipSetup()
	database.EnsureGroupSetup()

	// Background scheduler: contribution due-date notifications
	services.StartScheduler()

	if *migrateFlag {
		database.Seed()
		log.Println("Migration complete. Exiting.")
		os.Exit(0)
	}

	if *seedFlag {
		database.Seed()
		log.Println("Seed complete. Exiting.")
		os.Exit(0)
	}

	// ---- Ledger core (event-sourced, append-only) -------------------------
	lg, ledgerGroupID := ledgerInit()
	services.SetAutoLedger(lg, *ledgerGroupID)

	if *backfillLedgerFlag {
		posted, err := services.BackfillLedgerFromHistory("system-backfill")
		if err != nil {
			log.Printf("Ledger backfill skipped: %v", err)
		} else {
			log.Printf("Ledger backfill posted %d opening-balance transactions", posted)
		}
		if !*serveFlag {
			database.ClosePgx()
			return
		}
	}

	// Replay / rebuild-projections operation (spec §4, §6 op 7).
	//   ./server -replay-ledger=group -serve=false
	//   ./server -replay-ledger=all -serve=false
	if *replayLedgerFlag != "" {
		ctx := context.Background()
		var scope *uuid.UUID
		switch *replayLedgerFlag {
		case "group":
			scope = ledgerGroupID
		case "all":
			// nil scope = whole store
		default:
			log.Fatalf("FATAL: unknown -replay-ledger scope %q (use 'group' or 'all')", *replayLedgerFlag)
		}
		start := time.Now()
		if err := lg.RebuildProjections(ctx, scope); err != nil {
			log.Fatalf("FATAL: ledger replay failed: %v", err)
		}
		log.Printf("Ledger projections rebuilt in %s", time.Since(start))
		if !*serveFlag {
			database.ClosePgx()
			return
		}
		log.Println("Continuing to serve.")
	}

	app := fiber.New(fiber.Config{
		AppName:      "Kikundi API v1.0",
		ErrorHandler: errorHandler,
	})

	app.Use(middleware.SetupCORS())
	app.Use(middleware.SecurityHeaders())
	app.Use(middleware.GlobalRateLimiter())
	app.Use(middleware.Sentry())
	app.Use(middleware.RequestLogger())

	app.Get("/health", func(c *fiber.Ctx) error {
		return c.JSON(fiber.Map{"status": "ok", "service": "kikundi-api"})
	})

	api := app.Group("/api/v1")

	authHandler := handlers.NewAuthHandler()
	groupSettingsHandler := handlers.NewGroupSettingsHandler()
	memberHandler := handlers.NewMemberHandler()
	contribHandler := handlers.NewContributionHandler()
	loanHandler := handlers.NewLoanHandler()
	repayHandler := handlers.NewRepaymentHandler()
	dashHandler := handlers.NewDashboardHandler()
	notifHandler := handlers.NewNotificationHandler()
	auditHandler := handlers.NewAuditHandler()
	committeeHandler := handlers.NewLoanCommitteeHandler()
	welfareHandler := handlers.NewWelfareHandler()
	userMgmtHandler := handlers.NewUserManagementHandler()
	adminHandler := handlers.NewAdminHandler()
	pendingActionHandler := handlers.NewPendingActionHandler()
	uploadHandler := handlers.NewUploadHandler(config.AppConfig.PublicBaseURL)
	backupHandler := handlers.NewBackupHandler()
	reportHandler := handlers.NewReportHandler()
	leadershipHandler := handlers.NewLeadershipHandler()
	memberContribHandler := handlers.NewMemberContributionHandler()
	announcementHandler := handlers.NewAnnouncementHandler()
	importHandler := handlers.NewImportHandler()
	ledgerHandler := handlers.NewLedgerHandler(lg, *ledgerGroupID)
	dissolutionHandler := handlers.NewDissolutionHandler()

	auth := api.Group("/auth")
	auth.Post("/login", authHandler.Login)
	auth.Post("/verify-otp", authHandler.VerifyOTP)

	// Serve uploaded files (authenticated only)
	uploads := app.Group("/uploads")
	uploads.Use(middleware.AuthRequired)
	// AUTH-02: JWTs arrive via ?token= for <img> tags — never cache them.
	uploads.Use(func(c *fiber.Ctx) error {
		c.Set("Cache-Control", "private, no-store")
		return c.Next()
	})
	uploads.Static("/", "./uploads")

	protected := api.Group("")
	protected.Use(middleware.AuthRequired)

	protected.Get("/me", authHandler.Me)
	protected.Put("/me", authHandler.UpdateProfile)
	protected.Post("/auth/change-password", authHandler.ChangePassword)
	protected.Post("/auth/logout", authHandler.Logout)
	protected.Post("/auth/first-login-setup", authHandler.FirstLoginSetup)
	protected.Post("/auth/refresh", authHandler.RefreshToken)

	// File upload routes
	protected.Post("/upload/avatar", uploadHandler.UploadAvatar)
	protected.Post("/upload/doc", uploadHandler.UploadDoc)

	protected.Get("/dashboard", dashHandler.Summary)

	// Group contribution settings (single-group deployment).
	// Chair proposes; only secretary approval applies changes.
	groups := protected.Group("/groups")
	groups.Get("/current", groupSettingsHandler.GetCurrent)
	// Group profile metadata (name/location/founded year): direct edit by
	// Mwenyekiti + Msimamizi. Separate from the contribution-settings
	// proposal flow (chair propose + secretary approve).
	groups.Patch("/:id/profile", middleware.RequireRoles(models.RoleChair, models.RoleAdmin), groupSettingsHandler.UpdateProfile)
	// Role-scoped group dashboard summaries (leadership + admin only)
	groups.Get("/:id/dashboard-summary",
		middleware.RequireLeadership(models.LeadershipChair, models.LeadershipTreasurer, models.LeadershipSecretary),
		dashHandler.GroupSummary)
	groups.Get("/:id/dashboard-summary/katibu",
		middleware.RequireLeadership(models.LeadershipSecretary),
		dashHandler.GroupSummaryKatibu)
	groups.Get("/:id/dashboard-summary/mweka-hazina",
		middleware.RequireLeadership(models.LeadershipTreasurer),
		dashHandler.GroupSummaryMwekaHazina)
	settings := groups.Group("/:id/contribution-settings")
	settings.Get("/", groupSettingsHandler.GetSettings)
	settings.Post("/propose", middleware.RequireRoles(models.RoleChair), groupSettingsHandler.Propose)
	settings.Post("/approve", middleware.RequireRoles(models.RoleSecretary), groupSettingsHandler.Approve)
	settings.Post("/reject", middleware.RequireRoles(models.RoleSecretary), groupSettingsHandler.Reject)

	// Group loan settings (riba + muda). Same proposal pattern: chair
	// proposes, secretary approves/rejects. Reads are open to all members
	// (the application form needs min/max + rate).
	loanSettingsHandler := handlers.NewLoanSettingsHandler()
	loanSettings := groups.Group("/:id/loan-settings")
	loanSettings.Get("/", loanSettingsHandler.Get)
	loanSettings.Post("/propose", middleware.RequireRoles(models.RoleChair), loanSettingsHandler.Propose)
	loanSettings.Post("/approve", middleware.RequireRoles(models.RoleSecretary), loanSettingsHandler.Approve)
	loanSettings.Post("/reject", middleware.RequireRoles(models.RoleSecretary), loanSettingsHandler.Reject)

	// Onboarding: wizard resume status + completion flag (chair), and the
	// member app-tour "seen" flag. Steps reuse the endpoints above.
	onboardingHandler := handlers.NewOnboardingHandler()
	groups.Get("/:id/onboarding-status", onboardingHandler.Status)
	groups.Patch("/:id/onboarding-complete", middleware.RequireRoles(models.RoleChair), onboardingHandler.Complete)

	// Member obligations (arrears + current + fines combined).
	// Member reads own; leadership reads all (self-or-leadership).
	memberOblig := protected.Group("/members/:id/obligations")
	memberOblig.Get("/summary", middleware.RequireSelfOrLeadership(func(c *fiber.Ctx) (string, string) {
		return c.Params("id"), ""
	}), handlers.ObligationsMemberSummary)

	// Group obligations aggregate + treasurer collection queue.
	leadership3 := middleware.RequireRoles(models.RoleChair, models.RoleSecretary, models.RoleTreasurer)
	groups.Get("/:id/obligations/summary", leadership3, handlers.ObligationsGroupSummary)
	groups.Get("/:id/collection-queue", middleware.RequireRoles(models.RoleTreasurer), handlers.CollectionQueue)

	// Notification settings (SMS channel): chair/admin only.
	chairAdmin := middleware.RequireRoles(models.RoleChair, models.RoleAdmin)
	groups.Get("/:id/notification-settings", chairAdmin, handlers.GetNotificationSettings)
	groups.Put("/:id/notification-settings", chairAdmin, handlers.UpdateNotificationSettings)
	// Pending self-service repayments: treasurer's approval queue.
	groups.Get("/:id/repayments", middleware.RequireRoles(models.RoleTreasurer), repayHandler.PendingQueue)

	// Fine offence types: chair proposes, secretary approves.
	offences := groups.Group("/:id/fine-offence-types")
	offences.Get("/", leadership3, handlers.ListOffenceTypes)
	offences.Post("/", middleware.RequireRoles(models.RoleChair), handlers.CreateOffenceType)
	offences.Patch("/:typeId", middleware.RequireRoles(models.RoleChair), handlers.UpdateOffenceType)
	offences.Post("/:typeId/approve", middleware.RequireRoles(models.RoleSecretary), handlers.ApproveOffenceType)
	offences.Post("/:typeId/deactivate", middleware.RequireRoles(models.RoleChair, models.RoleSecretary), handlers.DeactivateOffenceType)

	// Fines: list (members own-only), collect (treasurer), waivers (chair propose + secretary decide).
	fines := protected.Group("/fines")
	fines.Get("/", handlers.ListFines)
	fines.Post("/:id/collect", middleware.RequireRoles(models.RoleTreasurer), handlers.CollectFine)
	fines.Patch("/:id/collect", middleware.RequireRoles(models.RoleTreasurer), handlers.CollectFine)
	fines.Post("/:id/waive-propose", middleware.RequireRoles(models.RoleChair), handlers.ProposeFineWaiver)
	fines.Patch("/:id/waive", middleware.RequireRoles(models.RoleChair), handlers.ProposeFineWaiver)
	fines.Post("/:id/waive-approve", middleware.RequireRoles(models.RoleSecretary), handlers.ApproveFineWaiver)
	fines.Post("/:id/waive-reject", middleware.RequireRoles(models.RoleSecretary), handlers.RejectFineWaiver)

	// Meetings: chair/secretary manage, secretary marks attendance + triggers fines.
	meetings := groups.Group("/:id/meetings")
	meetings.Get("/", leadership3, handlers.ListMeetings)
	meetings.Post("/", middleware.RequireRoles(models.RoleChair, models.RoleSecretary), handlers.CreateMeeting)
	mtg := protected.Group("/meetings/:id")
	mtg.Get("/attendance", leadership3, handlers.GetAttendance)
	mtg.Put("/attendance", middleware.RequireRoles(models.RoleSecretary), handlers.SetAttendance)
	mtg.Post("/trigger-fines", middleware.RequireRoles(models.RoleSecretary), handlers.TriggerMeetingFines)

	// Payment methods (LipaNamba / bank accounts) — members read, chair/treasurer manage
	paymentMethodHandler := handlers.NewPaymentMethodHandler()
	portfolioHandler := handlers.NewLoanPortfolioHandler()
	pms := groups.Group("/:id/payment-methods")
	pms.Get("/", paymentMethodHandler.List)
	pms.Post("/", middleware.RequireRoles(models.RoleChair, models.RoleTreasurer), paymentMethodHandler.Create)
	pms.Post("/:pmId/approve", middleware.RequireRoles(models.RoleChair), paymentMethodHandler.Approve)
	pms.Patch("/:pmId", middleware.RequireRoles(models.RoleChair, models.RoleTreasurer), paymentMethodHandler.Update)
	pms.Delete("/:pmId", middleware.RequireRoles(models.RoleChair, models.RoleTreasurer), paymentMethodHandler.Delete)
	pms.Post("/:pmId/restore", middleware.RequireRoles(models.RoleChair, models.RoleTreasurer), paymentMethodHandler.RestorePaymentMethod)

	// User Management routes (Mwenyekiti creates, Katibu approves)
	users := protected.Group("/users")
	users.Post("/create", middleware.RequireRoles(models.RoleChair), userMgmtHandler.CreateUser)
	users.Get("/pending", middleware.RequireRoles(models.RoleSecretary), userMgmtHandler.ListPending)
	users.Get("/", middleware.RequireRoles(models.RoleChair, models.RoleSecretary), userMgmtHandler.ListUsers)
	users.Post("/:id/approve", middleware.RequireRoles(models.RoleSecretary), userMgmtHandler.ApproveUser)
	users.Post("/:id/reject", middleware.RequireRoles(models.RoleSecretary), userMgmtHandler.RejectUser)
	// Chair may reset non-admin user passwords (temp returned once); not full admin powers
	users.Post("/:id/reset-password", middleware.RequireRoles(models.RoleChair), userMgmtHandler.ResetUserPassword)
	// Roles a user holds (self / leadership / admin — used for the role-switch toggle)
	users.Get("/:id/roles", middleware.RequireSelfOrLeadership(func(c *fiber.Ctx) (string, string) {
		return "", c.Params("id")
	}), dashHandler.UserRoles)

	members := protected.Group("/members")
	members.Get("/", middleware.RequireLeadership(models.LeadershipChair, models.LeadershipTreasurer, models.LeadershipSecretary), memberHandler.List)
	members.Get("/:id", middleware.RequireLeadership(models.LeadershipChair, models.LeadershipTreasurer, models.LeadershipSecretary), memberHandler.Get)
	// Role-scoped dashboard summaries (self / leadership / admin — see handler)
	members.Get("/:id/dashboard-summary", middleware.RequireSelfOrLeadership(func(c *fiber.Ctx) (string, string) {
		return c.Params("id"), ""
	}), dashHandler.MemberSummary)
	members.Post("/", middleware.RequireRoles(models.RoleChair, models.RoleSecretary, models.RoleTreasurer), memberHandler.Create)
	members.Post("/:id/create-login", middleware.RequireRoles(models.RoleChair), memberHandler.CreateLogin)
	members.Patch("/:id/approve", middleware.RequireRoles(models.RoleSecretary), memberHandler.ApproveMember)
	members.Patch("/:id/reject", middleware.RequireRoles(models.RoleSecretary), memberHandler.RejectMember)
	members.Post("/:id/toggle-active", middleware.RequireRoles(models.RoleSecretary), memberHandler.ToggleActive)
	members.Put("/:id", middleware.RequireRoles(models.RoleChair, models.RoleSecretary), memberHandler.Update)
	members.Delete("/:id", middleware.RequireRoles(models.RoleChair), memberHandler.Delete)
	members.Post("/:id/restore", middleware.RequireRoles(models.RoleChair), memberHandler.RestoreMember)
	members.Patch("/:id/onboarding-seen", onboardingHandler.MarkOnboardingSeen)

	contribs := protected.Group("/contributions")
	contribs.Get("/", contribHandler.List)
	contribs.Post("/", middleware.RequirePosition(models.PositionTreasurer), contribHandler.Create)
	contribs.Put("/:id", middleware.RequirePosition(models.PositionTreasurer), contribHandler.Edit)
	contribs.Get("/monthly-report", middleware.RequireLeadership(models.LeadershipChair, models.LeadershipTreasurer, models.LeadershipSecretary), contribHandler.MonthlyReport)

	loans := protected.Group("/loans")
	loans.Get("/", loanHandler.List)
	loans.Get("/portfolio", middleware.RequireRoles(models.RoleChair, models.RoleSecretary, models.RoleTreasurer), portfolioHandler.Portfolio)
	loans.Get("/outstanding-report", middleware.RequireLeadership(models.LeadershipChair, models.LeadershipTreasurer, models.LeadershipSecretary), loanHandler.OutstandingReport)
	loans.Get("/:id/schedule", loanHandler.Schedule)
	loans.Get("/:id", loanHandler.Get)
	loans.Post("/apply", loanHandler.Apply)
	// BUG-2 fix: the ONLY approval path is the sequential chain
	// (Hazina → Katibu → Bodi → Mwenyekiti) via /uongozi/mikopo/:id/approve.
	// The legacy direct-approve/reject bypass (which let chair/treasurer skip
	// katibu + bodi entirely) has been removed. Rejection inside the chain
	// happens via committee review (POST /loan-committee/loans/:id/review).
	loans.Post("/:id/disburse", middleware.RequirePosition(models.PositionTreasurer), loanHandler.Disburse)
	// BUG-5: borrower acknowledges receiving the disbursed loan.
	loans.Patch("/:id/confirm-received", loanHandler.ConfirmReceived)

	// Member self-service repayment (mirrors Weka Mchango): any authenticated
	// member submits against their OWN disbursed loan (ownership enforced in
	// handler); starts PENDING, moves money only on treasurer approval.
	loans.Post("/:id/repayments", repayHandler.Submit)

	// Loan offset (overdue debt paid from member savings): three-role check —
	// mwenyekiti proposes, katibu approves/rejects, mweka-hazina executes.
	// A plain mwanachama hits 403 on every one of these (RequireRoles).
	offsetHandler := handlers.NewLoanOffsetHandler()
	leadership3off := middleware.RequireRoles(models.RoleChair, models.RoleSecretary, models.RoleTreasurer)
	loans.Get("/:id/offset-preview", leadership3off, offsetHandler.Preview)
	loans.Post("/:id/offset-propose", middleware.RequireRoles(models.RoleChair), offsetHandler.Propose)
	offsets := protected.Group("/loan-offsets")
	offsets.Get("/", leadership3off, offsetHandler.List)
	offsets.Post("/:id/approve", middleware.RequireRoles(models.RoleSecretary), offsetHandler.Approve)
	offsets.Post("/:id/reject", middleware.RequireRoles(models.RoleSecretary), offsetHandler.Reject)
	offsets.Post("/:id/execute", middleware.RequireRoles(models.RoleTreasurer), offsetHandler.Execute)

	repayments := protected.Group("/repayments")
	repayments.Get("/", repayHandler.List)
	repayments.Post("/", middleware.RequirePosition(models.PositionTreasurer), repayHandler.Record)
	// Self-service lifecycle (treasurer ONLY — same RBAC as contribution
	// approval and fine collection: not mwenyekiti/katibu's action).
	repayments.Patch("/:id/approve", middleware.RequireRoles(models.RoleTreasurer), repayHandler.Approve)
	repayments.Patch("/:id/reject", middleware.RequireRoles(models.RoleTreasurer), repayHandler.Reject)

	notifs := protected.Group("/notifications")
	notifs.Get("/", notifHandler.List)
	notifs.Post("/read", notifHandler.MarkRead)
	notifs.Post("/read-all", notifHandler.MarkAllRead)

	protected.Get("/audit-logs", middleware.RequireRoles(models.RoleChair, models.RoleAdmin), auditHandler.List)
	protected.Get("/audit-logs/login-activity", middleware.RequireRoles(models.RoleChair, models.RoleAdmin), auditHandler.LoginActivity)
	protected.Get("/audit-logs/failed-logins", middleware.RequireRoles(models.RoleChair, models.RoleAdmin), auditHandler.FailedLogins)
	protected.Get("/audit-logs/summary", middleware.RequireRoles(models.RoleChair, models.RoleAdmin), auditHandler.AuditSummary)

	// Loan committee check endpoint (for frontend gating)
	protected.Get("/loan-committee/check", committeeHandler.IsCommitteeMember)

	// Loan committee routes
	committee := protected.Group("/loan-committee")
	committee.Use(middleware.RequireLoanCommitteeMember())

	committee.Get("/members", committeeHandler.ListMembers)
	// BUG-3 fix: appointing members to the loan board is MWENYEKITI's action
	// only — katibu was incorrectly allowed before.
	committee.Post("/members", middleware.RequireRoles(models.RoleChair), committeeHandler.AppointMember)
	committee.Delete("/members/:id", middleware.RequireRoles(models.RoleChair), committeeHandler.RemoveMember)
	committee.Post("/members/:id/restore", middleware.RequireRoles(models.RoleChair), committeeHandler.RestoreCommitteeMember)
	committee.Get("/loans", committeeHandler.ListLoans)
	committee.Get("/loans/:id", committeeHandler.GetLoan)
	committee.Post("/loans/:id/review", committeeHandler.SubmitReview)
	committee.Get("/dashboard", committeeHandler.GetDashboard)
	committee.Get("/history", committeeHandler.GetHistory)
	committee.Get("/report", committeeHandler.GetReport)
	committee.Get("/pending-count", committeeHandler.GetPendingLoansCount)

	// Welfare routes (Mfuko wa Kijamii)
	welfare := protected.Group("/welfare")

	// Dashboard — all authenticated users
	welfare.Get("/dashboard", welfareHandler.Dashboard)

	// Events members can contribute to (approved, member-funded)
	welfare.Get("/contribute-events", welfareHandler.ListContributeEvents)
	welfare.Get("/events/:id/my-obligation", welfareHandler.MyObligation)

	// Events — all can view, treasurer creates, chair OR secretary approves
	welfare.Get("/events", welfareHandler.ListEvents)
	welfare.Get("/events/:id", welfareHandler.GetEvent)
	welfare.Post("/events", middleware.RequireRoles(models.RoleTreasurer), welfareHandler.CreateEvent)
	welfare.Post("/events/:id/approve", middleware.RequireRoles(models.RoleChair, models.RoleSecretary), welfareHandler.ApproveEvent)
	welfare.Post("/events/:id/reject", middleware.RequireRoles(models.RoleChair, models.RoleSecretary), welfareHandler.RejectEvent)

	// Contributions — leadership see all (receipting/records); members must
	// use /my-contributions. Guard closes IDOR RBAC-H01 (any role=member
	// previously could enumerate all welfare obligations).
	welfare.Get("/contributions", middleware.RequireRoles(models.RoleTreasurer, models.RoleChair, models.RoleSecretary), welfareHandler.ListContributions)
	welfare.Get("/my-contributions", welfareHandler.MyContributions)
	welfare.Post("/events/:id/contributions/:memberId/pay", middleware.RequireRoles(models.RoleTreasurer), welfareHandler.RecordPayment)
	welfare.Post("/events/:id/contributions/:memberId/waive", middleware.RequireRoles(models.RoleTreasurer), welfareHandler.WaiveContribution)
	welfare.Post("/contributions/:id/approve", middleware.RequireRoles(models.RoleTreasurer), welfareHandler.ApproveContribution)
	welfare.Post("/contributions/:id/reject", middleware.RequireRoles(models.RoleTreasurer), welfareHandler.RejectContribution)
	welfare.Delete("/contributions/:id", middleware.RequireRoles(models.RoleTreasurer), welfareHandler.RemoveContribution)
	welfare.Post("/contributions/:id/restore", middleware.RequireRoles(models.RoleTreasurer), welfareHandler.RestoreContribution)

	// Disbursement — treasurer disburses after event is approved and fully funded
	welfare.Post("/events/:id/disburse", middleware.RequireRoles(models.RoleTreasurer), welfareHandler.DisburseEvent)
	welfare.Post("/events/:id/confirm-receipt", welfareHandler.ConfirmReceipt)

	// Pending Actions routes (Chairperson approves)
	pending := protected.Group("/pending-actions")
	pending.Get("/", middleware.RequirePosition(models.PositionChairperson), pendingActionHandler.List)
	pending.Get("/:id", middleware.RequirePosition(models.PositionChairperson), pendingActionHandler.Get)
	pending.Post("/:id/approve", middleware.RequirePosition(models.PositionChairperson), pendingActionHandler.Approve)
	pending.Post("/:id/reject", middleware.RequirePosition(models.PositionChairperson), pendingActionHandler.Reject)

	// Admin routes (Super Admin only). NOTE: the guard is attached
	// per-route (not via admin.Use) because the /admin/ledger subgroup
	// below carries its own wider guards — a blanket prefix middleware
	// here would 403 treasurer/chair/secretary on every ledger read.
	admin := protected.Group("/admin")
	adminOnly := middleware.RequireRoles(models.RoleAdmin)
	admin.Get("/users", adminOnly, adminHandler.ListAllUsers)
	admin.Get("/logs", adminOnly, adminHandler.GetAdminLogs)
	admin.Post("/users/:id/override", adminOnly, adminHandler.OverrideUser)
	admin.Post("/users/:id/reset-password", adminOnly, adminHandler.ResetUserPassword)
	admin.Post("/auth/reset-password", adminOnly, authHandler.ResetPassword) // Admin-only password reset
	admin.Get("/health", adminOnly, adminHandler.GetSystemHealth)

	// Admin Backup routes
	admin.Post("/backup/generate", adminOnly, backupHandler.GenerateBackup)
	admin.Get("/backup/history", adminOnly, backupHandler.GetBackupHistory)
	admin.Get("/backup/settings", adminOnly, backupHandler.GetBackupSettings)
	admin.Post("/backup/settings", adminOnly, backupHandler.SaveBackupSettings)
	admin.Get("/backup/download/:id", adminOnly, backupHandler.DownloadBackup)

	// Ledger / accounting core routes — event-sourced, append-only; every
	// write attributes to the session user. Reads (balance/statement/
	// trial-balance) are view-only for chair + secretary; writes stay
	// treasurer/admin-only so the books can't be altered by viewers.
	ledgerRoutes := protected.Group("/admin/ledger")
	readLedger := middleware.RequireRoles(models.RoleChair, models.RoleSecretary, models.RoleTreasurer)
	writeLedger := middleware.RequireRoles(models.RoleTreasurer)
	ledgerRoutes.Post("/accounts", writeLedger, ledgerHandler.OpenAccount)
	ledgerRoutes.Post("/transactions", writeLedger, ledgerHandler.RecordTransaction)
	ledgerRoutes.Post("/transactions/:id/reverse", writeLedger, ledgerHandler.ReverseTransaction)
	ledgerRoutes.Get("/balance", readLedger, ledgerHandler.GetBalance)
	ledgerRoutes.Get("/statement", readLedger, ledgerHandler.GetStatement)
	ledgerRoutes.Get("/trial-balance", readLedger, ledgerHandler.GetTrialBalance)
	ledgerRoutes.Get("/transactions/:id", readLedger, ledgerHandler.GetTransactionDetail)
	ledgerRoutes.Post("/replay", middleware.RequireRoles(models.RoleAdmin), ledgerHandler.Replay)

	// Reports routes (Chair only)
	reports := protected.Group("/reports")
	reports.Use(middleware.RequireRoles(models.RoleChair))
	reports.Get("/wanachama", reportHandler.MembersReport)
	reports.Get("/michango", reportHandler.ContributionsReport)
	reports.Get("/mikopo", reportHandler.LoansReport)
	reports.Get("/mapato", reportHandler.IncomeExpenseReport)
	reports.Get("/muhtasari", reportHandler.SummaryReport)

	// Member Contribution routes (Phase 4)
	// "Pokea Michango" (all-contributions listing/receipting) is a TREASURY
	// function — mwenyekiti (chair) no longer has access (katibu keeps
	// records view). Chair's MFUKO verification lives on /michango/:id/confirm
	// (type-gated) and /michango-inayosubiri.
	michango := protected.Group("/michango")
	michango.Post("/", memberContribHandler.Submit)
	michango.Get("/mine", memberContribHandler.MyContributions)
	michango.Get("/members-summary", middleware.RequireLeadership(models.LeadershipChair, models.LeadershipTreasurer, models.LeadershipSecretary), memberContribHandler.MembersSummary)
	michango.Get("/pending", middleware.RequireLeadership(models.LeadershipChair, models.LeadershipTreasurer, models.LeadershipSecretary), memberContribHandler.PendingContributions)
	michango.Get("/", middleware.RequireRoles(models.RoleTreasurer, models.RoleSecretary), memberContribHandler.AllContributions)
	michango.Post("/:id/confirm", middleware.RequireRoles(models.RoleChair, models.RoleTreasurer), memberContribHandler.Confirm)
	michango.Post("/:id/reject", middleware.RequireRoles(models.RoleChair, models.RoleTreasurer), memberContribHandler.Reject)

	// Leadership routes (dual plane — members with leadership roles).
	// NOTE: do NOT attach RequireLeadership via uongozi.Use() — Fiber matches
	// Use-middleware by path prefix, so it would also run on
	// /uongozi/mikopo/* below (registered on `protected` on purpose) and
	// block appointed bodi members (role=member, no leadership position).
	// The guard is attached per-route instead.
	uongozi := protected.Group("/uongozi")
	leadershipOnly := middleware.RequireLeadership(models.LeadershipChair, models.LeadershipTreasurer, models.LeadershipSecretary)

	uongozi.Get("/dashboard", leadershipOnly, leadershipHandler.Dashboard)
	uongozi.Get("/quick-stats", leadershipOnly, leadershipHandler.QuickStats)
	uongozi.Post("/announcements", leadershipOnly, announcementHandler.Broadcast)
	uongozi.Get("/ripoti", leadershipOnly, leadershipHandler.Reports)
	uongozi.Get("/wanachama", leadershipOnly, memberHandler.List)

	// BUG-2 fix: the loan-approval chain routes live OUTSIDE the uongozi
	// leadership group (its RequireLeadership middleware would block appointed
	// bodi members, whose users.role is plain member). They are instead
	// guarded by RequireLoanCommitteeMember — leadership roles OR appointed
	// bodi members — matching the handler's sequential-stage switch.
	protected.Get("/uongozi/mikopo/pending", middleware.RequireLoanCommitteeMember(), leadershipHandler.PendingLoans)
	protected.Post("/uongozi/mikopo/:id/approve", middleware.RequireLoanCommitteeMember(), leadershipHandler.ApproveLoan)

	// Dissolution routes
	dissolution := protected.Group("/dissolution-proposals")
	dissolution.Post("/:id/vote", dissolutionHandler.Vote)
	dissolution.Get("/:id", dissolutionHandler.Get)
	dissolution.Get("/:id/payouts", dissolutionHandler.ListPayouts)
	dissolution.Post("/:id/execute", middleware.RequireRoles(models.RoleChair, models.RoleSecretary), dissolutionHandler.Execute)
	protected.Get("/groups/:id/dissolution-proposals", dissolutionHandler.ListByGroup)
	protected.Post("/groups/:id/dissolution-proposals", middleware.RequireRoles(models.RoleChair, models.RoleSecretary), dissolutionHandler.Propose)
	protected.Patch("/dissolution-payouts/:id/mark-paid", middleware.RequireRoles(models.RoleTreasurer), dissolutionHandler.MarkPaid)
	protected.Get("/dissolution-payouts/me", dissolutionHandler.MyPayouts)

	// Import routes (leadership only — for historical data from books)
	importRoutes := protected.Group("/import")
	importRoutes.Use(middleware.RequireRoles(models.RoleChair, models.RoleTreasurer))
	importRoutes.Post("/contributions", importHandler.ImportContributions)
	importRoutes.Post("/loans", importHandler.ImportLoans)

	// Graceful shutdown
	go func() {
		sigCh := make(chan os.Signal, 1)
		signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
		<-sigCh
		log.Println("Shutting down gracefully...")
		services.FlushSentry()
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := app.ShutdownWithContext(ctx); err != nil {
			log.Fatalf("Shutdown error: %v", err)
		}
		log.Println("Server stopped.")
	}()

	log.Println("Server starting on :" + config.AppConfig.Port)
	if err := app.Listen(":" + config.AppConfig.Port); err != nil {
		log.Fatalf("Server error: %v", err)
	}
}

func errorHandler(c *fiber.Ctx, err error) error {
	code := fiber.StatusInternalServerError
	if e, ok := err.(*fiber.Error); ok {
		code = e.Code
	}
	log.Printf("ERROR [%d]: %v", code, err)
	if code >= 500 && services.Enabled {
		services.CaptureError(err, c.Method(), c.Path())
	}
	message := "Hitilafu ya mfumo"
	if code < 500 {
		message = err.Error()
	}
	return c.Status(code).JSON(fiber.Map{"message": message})
}

// ledgerInit brings up the raw-SQL pool, the ledger schema and this
// deployment's group scope, returning a ready-to-use Ledger. Fatal on failure.
func ledgerInit() (*ledger.Ledger, *uuid.UUID) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	pool, err := database.ConnectPgx(ctx)
	if err != nil {
		log.Fatalf("FATAL: ledger pool: %v", err)
	}
	if err := ledger.Migrate(ctx, pool); err != nil {
		log.Fatalf("FATAL: ledger migrate: %v", err)
	}
	groupName := os.Getenv("LEDGER_GROUP_NAME")
	if groupName == "" {
		groupName = "kikundi-main"
	}
	gid, err := ledger.CreateGroup(ctx, pool, groupName, ledger.CurrencyTZS)
	if err != nil {
		log.Fatalf("FATAL: ensure ledger group %q: %v", groupName, err)
	}
	lg, err := ledger.New(pool)
	if err != nil {
		log.Fatalf("FATAL: ledger init: %v", err)
	}
	return lg, &gid
}
