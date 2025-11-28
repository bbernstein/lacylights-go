package repositories

import (
	"context"

	"github.com/bbernstein/lacylights-go/internal/database/models"
	"github.com/lucsky/cuid"
	"gorm.io/gorm"
)

// SettingRepository handles setting data access.
type SettingRepository struct {
	db *gorm.DB
}

// NewSettingRepository creates a new SettingRepository.
func NewSettingRepository(db *gorm.DB) *SettingRepository {
	return &SettingRepository{db: db}
}

// FindAll returns all settings.
func (r *SettingRepository) FindAll(ctx context.Context) ([]models.Setting, error) {
	var settings []models.Setting
	result := r.db.WithContext(ctx).
		Order("key ASC").
		Find(&settings)
	return settings, result.Error
}

// FindByKey returns a setting by key.
func (r *SettingRepository) FindByKey(ctx context.Context, key string) (*models.Setting, error) {
	var setting models.Setting
	result := r.db.WithContext(ctx).First(&setting, "key = ?", key)
	if result.Error != nil {
		if result.Error == gorm.ErrRecordNotFound {
			return nil, nil
		}
		return nil, result.Error
	}
	return &setting, nil
}

// Upsert creates or updates a setting by key.
func (r *SettingRepository) Upsert(ctx context.Context, key, value string) (*models.Setting, error) {
	var setting models.Setting

	// Try to find existing setting
	result := r.db.WithContext(ctx).First(&setting, "key = ?", key)

	if result.Error == gorm.ErrRecordNotFound {
		// Create new setting
		setting = models.Setting{
			ID:    cuid.New(),
			Key:   key,
			Value: value,
		}
		if err := r.db.WithContext(ctx).Create(&setting).Error; err != nil {
			return nil, err
		}
		return &setting, nil
	} else if result.Error != nil {
		return nil, result.Error
	}

	// Update existing setting
	setting.Value = value
	if err := r.db.WithContext(ctx).Save(&setting).Error; err != nil {
		return nil, err
	}

	return &setting, nil
}

// Delete deletes a setting by key.
func (r *SettingRepository) Delete(ctx context.Context, key string) error {
	return r.db.WithContext(ctx).Delete(&models.Setting{}, "key = ?", key).Error
}
