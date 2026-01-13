package repositories

import (
	"context"

	"github.com/bbernstein/lacylights-go/internal/database/models"
	"github.com/lucsky/cuid"
	"gorm.io/gorm"
)

// LookRepository handles scene data access.
type LookRepository struct {
	db *gorm.DB
}

// NewLookRepository creates a new LookRepository.
func NewLookRepository(db *gorm.DB) *LookRepository {
	return &LookRepository{db: db}
}

// FindByProjectID returns all looks in a project.
func (r *LookRepository) FindByProjectID(ctx context.Context, projectID string) ([]models.Look, error) {
	var looks []models.Look
	result := r.db.WithContext(ctx).
		Where("project_id = ?", projectID).
		Order("created_at DESC").
		Find(&looks)
	return looks, result.Error
}

// FindByID returns a look by ID.
func (r *LookRepository) FindByID(ctx context.Context, id string) (*models.Look, error) {
	var look models.Look
	result := r.db.WithContext(ctx).First(&look, "id = ?", id)
	if result.Error != nil {
		if result.Error == gorm.ErrRecordNotFound {
			return nil, nil
		}
		return nil, result.Error
	}
	return &look, nil
}

// Create creates a new look.
func (r *LookRepository) Create(ctx context.Context, look *models.Look) error {
	if look.ID == "" {
		look.ID = cuid.New()
	}
	return r.db.WithContext(ctx).Create(look).Error
}

// Update updates an existing look.
func (r *LookRepository) Update(ctx context.Context, look *models.Look) error {
	return r.db.WithContext(ctx).Save(look).Error
}

// Delete deletes a look by ID.
func (r *LookRepository) Delete(ctx context.Context, id string) error {
	return r.db.WithContext(ctx).Delete(&models.Look{}, "id = ?", id).Error
}

// GetFixtureValues returns all fixture values for a look.
func (r *LookRepository) GetFixtureValues(ctx context.Context, lookID string) ([]models.FixtureValue, error) {
	var values []models.FixtureValue
	result := r.db.WithContext(ctx).
		Where("look_id = ?", lookID).
		Order("look_order ASC").
		Find(&values)
	return values, result.Error
}

// CountFixtures returns the number of fixtures in a look.
func (r *LookRepository) CountFixtures(ctx context.Context, lookID string) (int64, error) {
	var count int64
	result := r.db.WithContext(ctx).
		Model(&models.FixtureValue{}).
		Where("look_id = ?", lookID).
		Count(&count)
	return count, result.Error
}

// CreateWithFixtureValues creates a look with its fixture values in a transaction.
func (r *LookRepository) CreateWithFixtureValues(ctx context.Context, look *models.Look, fixtureValues []models.FixtureValue) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if look.ID == "" {
			look.ID = cuid.New()
		}
		if err := tx.Create(look).Error; err != nil {
			return err
		}

		if len(fixtureValues) > 0 {
			for i := range fixtureValues {
				if fixtureValues[i].ID == "" {
					fixtureValues[i].ID = cuid.New()
				}
				fixtureValues[i].LookID = look.ID
			}
			if err := tx.Create(&fixtureValues).Error; err != nil {
				return err
			}
		}
		return nil
	})
}

// DeleteFixtureValues deletes all fixture values for a look.
func (r *LookRepository) DeleteFixtureValues(ctx context.Context, lookID string) error {
	return r.db.WithContext(ctx).Delete(&models.FixtureValue{}, "look_id = ?", lookID).Error
}

// CreateFixtureValue creates a fixture value.
func (r *LookRepository) CreateFixtureValue(ctx context.Context, value *models.FixtureValue) error {
	if value.ID == "" {
		value.ID = cuid.New()
	}
	return r.db.WithContext(ctx).Create(value).Error
}

// CreateFixtureValues creates multiple fixture values.
func (r *LookRepository) CreateFixtureValues(ctx context.Context, values []models.FixtureValue) error {
	if len(values) == 0 {
		return nil
	}
	for i := range values {
		if values[i].ID == "" {
			values[i].ID = cuid.New()
		}
	}
	return r.db.WithContext(ctx).Create(&values).Error
}

// DeleteFixtureValue deletes a single fixture value by fixture ID and look ID.
func (r *LookRepository) DeleteFixtureValue(ctx context.Context, lookID, fixtureID string) error {
	return r.db.WithContext(ctx).Delete(&models.FixtureValue{}, "look_id = ? AND fixture_id = ?", lookID, fixtureID).Error
}

// GetFixtureValue returns a specific fixture value by look and fixture ID.
func (r *LookRepository) GetFixtureValue(ctx context.Context, lookID, fixtureID string) (*models.FixtureValue, error) {
	var value models.FixtureValue
	result := r.db.WithContext(ctx).Where("look_id = ? AND fixture_id = ?", lookID, fixtureID).First(&value)
	if result.Error != nil {
		if result.Error == gorm.ErrRecordNotFound {
			return nil, nil
		}
		return nil, result.Error
	}
	return &value, nil
}

// UpdateFixtureValue updates a fixture value.
func (r *LookRepository) UpdateFixtureValue(ctx context.Context, value *models.FixtureValue) error {
	return r.db.WithContext(ctx).Save(value).Error
}
