package handlers

import (
	"time"

	"kikundibora/database"
	"kikundibora/models"
	"kikundibora/services"

	"github.com/gofiber/fiber/v2"
	"github.com/shopspring/decimal"
	"gorm.io/gorm"
)

type LoanPortfolioHandler struct{}

func NewLoanPortfolioHandler() *LoanPortfolioHandler {
	return &LoanPortfolioHandler{}
}

// Portfolio returns every disbursed loan in the group with aggregate stats
// (total disbursed / repaid / outstanding / overdue, counts by status) and
// the individual loan rows. Filters: status, member_id, from/to (disbursed
// date range). Access: mwenyekiti, katibu, mweka hazina (+ admin) via route
// guard.
// GET /api/v1/loans/portfolio
func (h *LoanPortfolioHandler) Portfolio(c *fiber.Ctx) error {
	q := database.DB.Model(&models.Loan{}).
		Where("disbursed_at IS NOT NULL").
		Where("status IN ?", []models.LoanStatus{models.LoanOutstanding, models.LoanClosed})

	if status := c.Query("status"); status != "" {
		q = q.Where("status = ?", status)
	}
	if memberID := c.Query("member_id"); memberID != "" {
		q = q.Where("member_id = ?", memberID)
	}
	if from := c.Query("from"); from != "" {
		q = q.Where("disbursed_at >= ?", from)
	}
	if to := c.Query("to"); to != "" {
		q = q.Where("disbursed_at <= ?", to+" 23:59:59")
	}

	var loans []models.Loan
	if err := q.
		Preload("Member", func(db *gorm.DB) *gorm.DB {
			return db.Select("id, member_no, full_name")
		}).
		Order("disbursed_at DESC").
		Find(&loans).Error; err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"message": "Imeshindikana kupata portfolio"})
	}

	sum := services.BuildLoanPortfolio(loans, time.Now())

	// Schedule-based overdue override: a loan WITH a generated schedule is
	// overdue only when a scheduled installment is past due and unpaid —
	// never off the bare due_date timer. Legacy loans without schedule rows
	// keep the due_date fallback computed inside BuildLoanPortfolio.
	today := time.Now().Truncate(24 * time.Hour)
	ids := make([]string, 0, len(loans))
	for _, l := range loans {
		ids = append(ids, l.ID)
	}
	overdueIDs := map[string]struct{}{}
	scheduledIDs := map[string]struct{}{}
	if len(ids) > 0 {
		var insts []models.LoanInstallment
		database.DB.Where("loan_id IN ?", ids).Find(&insts)
		for _, in := range insts {
			scheduledIDs[in.LoanID] = struct{}{}
			if in.DueDate.Before(today) && in.PaidAmount.LessThan(in.TotalAmount) {
				overdueIDs[in.LoanID] = struct{}{}
			}
		}
	}
	if len(scheduledIDs) > 0 {
		// Recompute the overdue aggregates from the corrected flags.
		sum.TotalOverdue = decimal.Zero
		sum.CountOverdue = 0
		for i := range sum.Loans {
			if _, hasSched := scheduledIDs[sum.Loans[i].ID]; hasSched {
				_, od := overdueIDs[sum.Loans[i].ID]
				sum.Loans[i].IsOverdue = od && sum.Loans[i].Status == string(models.LoanOutstanding)
			}
			if sum.Loans[i].IsOverdue {
				sum.CountOverdue++
				sum.TotalOverdue = sum.TotalOverdue.Add(sum.Loans[i].Outstanding)
			}
		}
	}
	return c.JSON(fiber.Map{"data": sum})
}
