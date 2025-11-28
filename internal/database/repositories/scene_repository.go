package repositories

import (
	"context"

	"github.com/bbernstein/lacylights-go/internal/database/models"
	"github.com/lucsky/cuid"
	"gorm.io/gorm"
)

// SceneRepository handles scene data access.
type SceneRepository struct {
	db *gorm.DB
}

// NewSceneRepository creates a new SceneRepository.
func NewSceneRepository(db *gorm.DB) *SceneRepository {
	return &SceneRepository{db: db}
}

// FindByProjectID returns all scenes in a project.
func (r *SceneRepository) FindByProjectID(ctx context.Context, projectID string) ([]models.Scene, error) {
	var scenes []models.Scene
	result := r.db.WithContext(ctx).
		Where("project_id = ?", projectID).
		Order("created_at DESC").
		Find(&scenes)
	return scenes, result.Error
}

// FindByID returns a scene by ID.
func (r *SceneRepository) FindByID(ctx context.Context, id string) (*models.Scene, error) {
	var scene models.Scene
	result := r.db.WithContext(ctx).First(&scene, "id = ?", id)
	if result.Error != nil {
		if result.Error == gorm.ErrRecordNotFound {
			return nil, nil
		}
		return nil, result.Error
	}
	return &scene, nil
}

// Create creates a new scene.
func (r *SceneRepository) Create(ctx context.Context, scene *models.Scene) error {
	if scene.ID == "" {
		scene.ID = cuid.New()
	}
	return r.db.WithContext(ctx).Create(scene).Error
}

// Update updates an existing scene.
func (r *SceneRepository) Update(ctx context.Context, scene *models.Scene) error {
	return r.db.WithContext(ctx).Save(scene).Error
}

// Delete deletes a scene by ID.
func (r *SceneRepository) Delete(ctx context.Context, id string) error {
	return r.db.WithContext(ctx).Delete(&models.Scene{}, "id = ?", id).Error
}

// GetFixtureValues returns all fixture values for a scene.
func (r *SceneRepository) GetFixtureValues(ctx context.Context, sceneID string) ([]models.FixtureValue, error) {
	var values []models.FixtureValue
	result := r.db.WithContext(ctx).
		Where("scene_id = ?", sceneID).
		Order("scene_order ASC").
		Find(&values)
	return values, result.Error
}

// CountFixtures returns the number of fixtures in a scene.
func (r *SceneRepository) CountFixtures(ctx context.Context, sceneID string) (int64, error) {
	var count int64
	result := r.db.WithContext(ctx).
		Model(&models.FixtureValue{}).
		Where("scene_id = ?", sceneID).
		Count(&count)
	return count, result.Error
}

// CreateWithFixtureValues creates a scene with its fixture values in a transaction.
func (r *SceneRepository) CreateWithFixtureValues(ctx context.Context, scene *models.Scene, fixtureValues []models.FixtureValue) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if scene.ID == "" {
			scene.ID = cuid.New()
		}
		if err := tx.Create(scene).Error; err != nil {
			return err
		}

		if len(fixtureValues) > 0 {
			for i := range fixtureValues {
				if fixtureValues[i].ID == "" {
					fixtureValues[i].ID = cuid.New()
				}
				fixtureValues[i].SceneID = scene.ID
			}
			if err := tx.Create(&fixtureValues).Error; err != nil {
				return err
			}
		}
		return nil
	})
}

// DeleteFixtureValues deletes all fixture values for a scene.
func (r *SceneRepository) DeleteFixtureValues(ctx context.Context, sceneID string) error {
	return r.db.WithContext(ctx).Delete(&models.FixtureValue{}, "scene_id = ?", sceneID).Error
}

// CreateFixtureValue creates a fixture value.
func (r *SceneRepository) CreateFixtureValue(ctx context.Context, value *models.FixtureValue) error {
	if value.ID == "" {
		value.ID = cuid.New()
	}
	return r.db.WithContext(ctx).Create(value).Error
}

// CreateFixtureValues creates multiple fixture values.
func (r *SceneRepository) CreateFixtureValues(ctx context.Context, values []models.FixtureValue) error {
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

// DeleteFixtureValue deletes a single fixture value by fixture ID and scene ID.
func (r *SceneRepository) DeleteFixtureValue(ctx context.Context, sceneID, fixtureID string) error {
	return r.db.WithContext(ctx).Delete(&models.FixtureValue{}, "scene_id = ? AND fixture_id = ?", sceneID, fixtureID).Error
}

// GetFixtureValue returns a specific fixture value by scene and fixture ID.
func (r *SceneRepository) GetFixtureValue(ctx context.Context, sceneID, fixtureID string) (*models.FixtureValue, error) {
	var value models.FixtureValue
	result := r.db.WithContext(ctx).Where("scene_id = ? AND fixture_id = ?", sceneID, fixtureID).First(&value)
	if result.Error != nil {
		if result.Error == gorm.ErrRecordNotFound {
			return nil, nil
		}
		return nil, result.Error
	}
	return &value, nil
}

// UpdateFixtureValue updates a fixture value.
func (r *SceneRepository) UpdateFixtureValue(ctx context.Context, value *models.FixtureValue) error {
	return r.db.WithContext(ctx).Save(value).Error
}
