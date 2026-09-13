package database

import (
	"errors"
	"log"

	"kikundibora/models"

	"github.com/shopspring/decimal"
	"gorm.io/gorm"
)

// EnsureGroupSetup guarantees exactly one Group row exists (this deployment
// serves a single group). Safe defaults: interval=monthly, due date and
// fixed amount unset until a proposal is approved. Idempotent — runs on
// every startup.
func EnsureGroupSetup() {
	var count int64
	DB.Model(&models.Group{}).Count(&count)
	if count == 0 {
		g := models.Group{
			Name:                  "Kikundi Bora",
			ContributionInterval:  models.IntervalMonthly,
		}
		if err := DB.Create(&g).Error; err != nil {
			log.Printf("ERROR: Failed to create default group: %v", err)
			return
		}
		log.Printf("Created default group %s (%s)", g.Name, g.ID)
	}
	EnsureFineSettingsForCurrentGroup()
	EnsureLoanSettingsForCurrentGroup()
}

// EnsureLoanSettingsForCurrentGroup creates a disabled (interest-free)
// LoanSettings row for the current group if none exists. Idempotent.
func EnsureLoanSettingsForCurrentGroup() {
	var g models.Group
	if err := DB.First(&g).Error; err != nil {
		return
	}
	if _, err := GetOrCreateLoanSettings(g.ID); err != nil {
		log.Printf("ERROR: Failed to ensure loan settings: %v", err)
	}
}

// GetOrCreateLoanSettings returns the group's LoanSettings, inserting
// interest-free defaults (30–365 days, flat) when missing. Self-heals when
// the table does not exist yet (same pattern as fine settings).
func GetOrCreateLoanSettings(groupID string) (*models.LoanSettings, error) {
	if !DB.Migrator().HasTable(&models.LoanSettings{}) {
		if err := DB.AutoMigrate(&models.LoanSettings{}); err != nil {
			return nil, err
		}
	}
	var s models.LoanSettings
	err := DB.Where("group_id = ?", groupID).First(&s).Error
	if err == nil {
		return &s, nil
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		if err2 := DB.AutoMigrate(&models.LoanSettings{}); err2 == nil {
			if retry := DB.Where("group_id = ?", groupID).First(&s).Error; retry == nil {
				return &s, nil
			} else if !errors.Is(retry, gorm.ErrRecordNotFound) {
				return nil, retry
			}
		} else {
			return nil, err
		}
	}
	s = models.LoanSettings{
		GroupID:             groupID,
		InterestEnabled:     false,
		DefaultInterestRate: decimal.Zero,
		InterestType:        models.LoanInterestFlat,
		MinTermDays:         14,
		MaxTermDays:         365,
	}
	// decimal import for the zero default.
	if err := DB.Create(&s).Error; err != nil {
		if err2 := DB.Where("group_id = ?", groupID).First(&s).Error; err2 != nil {
			return nil, err
		}
		return &s, nil
	}
	return &s, nil
}

// EnsureFineSettingsForCurrentGroup creates a disabled FineSettings row for
// the current group if none exists. Idempotent.
func EnsureFineSettingsForCurrentGroup() {
	var g models.Group
	if err := DB.First(&g).Error; err != nil {
		return
	}
	if _, err := GetOrCreateFineSettings(g.ID); err != nil {
		log.Printf("ERROR: Failed to ensure fine settings: %v", err)
	}
}

// GetOrCreateFineSettings returns the group's FineSettings, inserting
// disabled defaults when missing. Self-heals when the table does not exist
// yet (e.g. database created before the fines feature was added).
func GetOrCreateFineSettings(groupID string) (*models.FineSettings, error) {
	if !DB.Migrator().HasTable(&models.FineSettings{}) {
		if err := DB.AutoMigrate(&models.FineSettings{}, &models.Fine{}); err != nil {
			return nil, err
		}
	}
	var s models.FineSettings
	err := DB.Where("group_id = ?", groupID).First(&s).Error
	if err == nil {
		return &s, nil
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		// Missing relation (42P01) can still occur if migration was skipped;
		// attempt one auto-migrate + retry before giving up.
		if err2 := DB.AutoMigrate(&models.FineSettings{}, &models.Fine{}); err2 == nil {
			if retry := DB.Where("group_id = ?", groupID).First(&s).Error; retry == nil {
				return &s, nil
			} else if !errors.Is(retry, gorm.ErrRecordNotFound) {
				return nil, retry
			}
		} else {
			return nil, err
		}
	}
	s = models.FineSettings{
		GroupID:         groupID,
		Enabled:         false,
		FineType:        models.FineTypeFixed,
		GracePeriodDays: 0,
	}
	if err := DB.Create(&s).Error; err != nil {
		// Concurrent create — fetch the winner.
		if err2 := DB.Where("group_id = ?", groupID).First(&s).Error; err2 != nil {
			return nil, err
		}
		return &s, nil
	}
	return &s, nil
}

// GetCurrentLoanSettings returns the single deployment's loan settings,
// creating interest-free defaults when missing.
func GetCurrentLoanSettings() (*models.LoanSettings, error) {
	g, err := GetCurrentGroup()
	if err != nil {
		return nil, err
	}
	return GetOrCreateLoanSettings(g.ID)
}

// GetCurrentGroup returns the single group row, creating it if missing.
func GetCurrentGroup() (*models.Group, error) {
	EnsureGroupSetup()
	var g models.Group
	if err := DB.First(&g).Error; err != nil {
		return nil, err
	}
	return &g, nil
}

// IsCurrentGroup reports whether :id refers to this deployment's group.
// RBAC-M01: group-scoped endpoints must reject foreign IDs with 404 even
// if a second group ever appears in the table.
func IsCurrentGroup(id string) (bool, error) {
	g, err := GetCurrentGroup()
	if err != nil {
		return false, err
	}
	return g.ID == id, nil
}
