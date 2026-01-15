package repositories

import (
	"context"
	"errors"

	"github.com/bbernstein/lacylights-go/internal/database/models"
	"github.com/lucsky/cuid"
	"gorm.io/gorm"
)

// EffectRepository handles effect data access.
type EffectRepository struct {
	db *gorm.DB
}

// NewEffectRepository creates a new EffectRepository.
func NewEffectRepository(db *gorm.DB) *EffectRepository {
	return &EffectRepository{db: db}
}

// ----------------------
// Basic CRUD Operations
// ----------------------

// Create creates a new effect.
func (r *EffectRepository) Create(ctx context.Context, effect *models.Effect) error {
	if effect.ID == "" {
		effect.ID = cuid.New()
	}
	return r.db.WithContext(ctx).Create(effect).Error
}

// Update updates an existing effect.
func (r *EffectRepository) Update(ctx context.Context, effect *models.Effect) error {
	return r.db.WithContext(ctx).Save(effect).Error
}

// Delete deletes an effect by ID.
// This cascades to delete associated EffectFixtures and EffectChannels.
func (r *EffectRepository) Delete(ctx context.Context, id string) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// First, get all EffectFixture IDs for this effect
		var effectFixtureIDs []string
		if err := tx.Model(&models.EffectFixture{}).
			Where("effect_id = ?", id).
			Pluck("id", &effectFixtureIDs).Error; err != nil {
			return err
		}

		// Delete associated EffectChannels
		if len(effectFixtureIDs) > 0 {
			if err := tx.Where("effect_fixture_id IN ?", effectFixtureIDs).
				Delete(&models.EffectChannel{}).Error; err != nil {
				return err
			}
		}

		// Delete EffectFixtures
		if err := tx.Where("effect_id = ?", id).
			Delete(&models.EffectFixture{}).Error; err != nil {
			return err
		}

		// Delete CueEffects that reference this effect
		if err := tx.Where("effect_id = ?", id).
			Delete(&models.CueEffect{}).Error; err != nil {
			return err
		}

		// Finally, delete the effect itself
		return tx.Delete(&models.Effect{}, "id = ?", id).Error
	})
}

// FindByID returns an effect by ID.
func (r *EffectRepository) FindByID(ctx context.Context, id string) (*models.Effect, error) {
	var effect models.Effect
	result := r.db.WithContext(ctx).First(&effect, "id = ?", id)
	if result.Error != nil {
		if errors.Is(result.Error, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, result.Error
	}
	return &effect, nil
}

// FindByProjectID returns all effects in a project.
func (r *EffectRepository) FindByProjectID(ctx context.Context, projectID string) ([]*models.Effect, error) {
	var effects []*models.Effect
	result := r.db.WithContext(ctx).
		Where("project_id = ?", projectID).
		Order("created_at DESC").
		Find(&effects)
	return effects, result.Error
}

// ---------------------------------
// Relationship Loading Methods
// ---------------------------------

// FindWithFixtures returns an effect with its fixtures and channels preloaded.
func (r *EffectRepository) FindWithFixtures(ctx context.Context, id string) (*models.Effect, error) {
	var effect models.Effect
	result := r.db.WithContext(ctx).
		Preload("Fixtures").
		Preload("Fixtures.Channels").
		Preload("Fixtures.Fixture").
		First(&effect, "id = ?", id)
	if result.Error != nil {
		if errors.Is(result.Error, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, result.Error
	}
	return &effect, nil
}

// GetEffectFixtures returns all fixtures for an effect.
func (r *EffectRepository) GetEffectFixtures(ctx context.Context, effectID string) ([]models.EffectFixture, error) {
	var fixtures []models.EffectFixture
	result := r.db.WithContext(ctx).
		Preload("Channels").
		Where("effect_id = ?", effectID).
		Order("effect_order ASC, created_at ASC").
		Find(&fixtures)
	return fixtures, result.Error
}

// AddFixtureToEffect adds a fixture to an effect with optional phase offset and amplitude scale.
func (r *EffectRepository) AddFixtureToEffect(ctx context.Context, effectID, fixtureID string, phaseOffset, amplitudeScale *float64) error {
	effectFixture := &models.EffectFixture{
		ID:             cuid.New(),
		EffectID:       effectID,
		FixtureID:      fixtureID,
		PhaseOffset:    phaseOffset,
		AmplitudeScale: amplitudeScale,
	}
	return r.db.WithContext(ctx).Create(effectFixture).Error
}

// RemoveFixtureFromEffect removes a fixture from an effect.
// This also deletes associated EffectChannels.
func (r *EffectRepository) RemoveFixtureFromEffect(ctx context.Context, effectID, fixtureID string) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// Find the EffectFixture
		var effectFixture models.EffectFixture
		if err := tx.Where("effect_id = ? AND fixture_id = ?", effectID, fixtureID).
			First(&effectFixture).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return nil // Not found, nothing to delete
			}
			return err
		}

		// Delete associated EffectChannels
		if err := tx.Where("effect_fixture_id = ?", effectFixture.ID).
			Delete(&models.EffectChannel{}).Error; err != nil {
			return err
		}

		// Delete the EffectFixture
		return tx.Delete(&effectFixture).Error
	})
}

// GetEffectFixture returns a specific effect-fixture relationship.
func (r *EffectRepository) GetEffectFixture(ctx context.Context, effectID, fixtureID string) (*models.EffectFixture, error) {
	var effectFixture models.EffectFixture
	result := r.db.WithContext(ctx).
		Preload("Channels").
		Where("effect_id = ? AND fixture_id = ?", effectID, fixtureID).
		First(&effectFixture)
	if result.Error != nil {
		if errors.Is(result.Error, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, result.Error
	}
	return &effectFixture, nil
}

// UpdateEffectFixture updates an existing effect-fixture relationship.
func (r *EffectRepository) UpdateEffectFixture(ctx context.Context, effectFixture *models.EffectFixture) error {
	return r.db.WithContext(ctx).Save(effectFixture).Error
}

// ---------------------------------
// EffectChannel Methods
// ---------------------------------

// AddChannelToEffectFixture adds a channel override to an effect fixture.
func (r *EffectRepository) AddChannelToEffectFixture(ctx context.Context, effectFixtureID string, channelOffset *int, channelType *string) error {
	effectChannel := &models.EffectChannel{
		ID:              cuid.New(),
		EffectFixtureID: effectFixtureID,
		ChannelOffset:   channelOffset,
		ChannelType:     channelType,
	}
	return r.db.WithContext(ctx).Create(effectChannel).Error
}

// AddChannelToEffectFixtureWithScales adds a channel override with amplitude and frequency scales.
func (r *EffectRepository) AddChannelToEffectFixtureWithScales(ctx context.Context, effectFixtureID string, channelOffset *int, channelType *string, amplitudeScale, frequencyScale *float64) error {
	effectChannel := &models.EffectChannel{
		ID:              cuid.New(),
		EffectFixtureID: effectFixtureID,
		ChannelOffset:   channelOffset,
		ChannelType:     channelType,
		AmplitudeScale:  amplitudeScale,
		FrequencyScale:  frequencyScale,
	}
	return r.db.WithContext(ctx).Create(effectChannel).Error
}

// GetEffectChannels returns all channels for an effect fixture.
func (r *EffectRepository) GetEffectChannels(ctx context.Context, effectFixtureID string) ([]models.EffectChannel, error) {
	var channels []models.EffectChannel
	result := r.db.WithContext(ctx).
		Where("effect_fixture_id = ?", effectFixtureID).
		Order("created_at ASC").
		Find(&channels)
	return channels, result.Error
}

// DeleteEffectChannel deletes an effect channel by ID.
func (r *EffectRepository) DeleteEffectChannel(ctx context.Context, channelID string) error {
	return r.db.WithContext(ctx).Delete(&models.EffectChannel{}, "id = ?", channelID).Error
}

// FindEffectChannelByID returns an effect channel by its ID.
func (r *EffectRepository) FindEffectChannelByID(ctx context.Context, id string) (*models.EffectChannel, error) {
	var channel models.EffectChannel
	result := r.db.WithContext(ctx).First(&channel, "id = ?", id)
	if result.Error != nil {
		if errors.Is(result.Error, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, result.Error
	}
	return &channel, nil
}

// UpdateEffectChannel updates an existing effect channel.
func (r *EffectRepository) UpdateEffectChannel(ctx context.Context, channel *models.EffectChannel) error {
	return r.db.WithContext(ctx).Save(channel).Error
}

// ---------------------------------
// CueEffect Methods
// ---------------------------------

// AddEffectToCue adds an effect to a cue with specified intensity and speed.
func (r *EffectRepository) AddEffectToCue(ctx context.Context, cueID, effectID string, intensity, speed float64) error {
	cueEffect := &models.CueEffect{
		ID:        cuid.New(),
		CueID:     cueID,
		EffectID:  effectID,
		Intensity: intensity,
		Speed:     speed,
	}
	return r.db.WithContext(ctx).Create(cueEffect).Error
}

// RemoveEffectFromCue removes an effect from a cue.
func (r *EffectRepository) RemoveEffectFromCue(ctx context.Context, cueID, effectID string) error {
	return r.db.WithContext(ctx).
		Where("cue_id = ? AND effect_id = ?", cueID, effectID).
		Delete(&models.CueEffect{}).Error
}

// FindEffectsByCueID returns all effects attached to a cue.
func (r *EffectRepository) FindEffectsByCueID(ctx context.Context, cueID string) ([]*models.CueEffect, error) {
	var cueEffects []*models.CueEffect
	result := r.db.WithContext(ctx).
		Preload("Effect").
		Where("cue_id = ?", cueID).
		Order("created_at ASC").
		Find(&cueEffects)
	return cueEffects, result.Error
}

// UpdateCueEffect updates an existing cue-effect relationship.
func (r *EffectRepository) UpdateCueEffect(ctx context.Context, cueEffect *models.CueEffect) error {
	return r.db.WithContext(ctx).Save(cueEffect).Error
}

// GetCueEffect returns a specific cue-effect relationship.
func (r *EffectRepository) GetCueEffect(ctx context.Context, cueID, effectID string) (*models.CueEffect, error) {
	var cueEffect models.CueEffect
	result := r.db.WithContext(ctx).
		Where("cue_id = ? AND effect_id = ?", cueID, effectID).
		First(&cueEffect)
	if result.Error != nil {
		if errors.Is(result.Error, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, result.Error
	}
	return &cueEffect, nil
}

// FindCueEffectByID returns a cue effect by its ID.
func (r *EffectRepository) FindCueEffectByID(ctx context.Context, id string) (*models.CueEffect, error) {
	var cueEffect models.CueEffect
	result := r.db.WithContext(ctx).
		Preload("Effect").
		First(&cueEffect, "id = ?", id)
	if result.Error != nil {
		if errors.Is(result.Error, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, result.Error
	}
	return &cueEffect, nil
}

// ---------------------------------
// Count Methods
// ---------------------------------

// CountFixtures returns the number of fixtures in an effect.
func (r *EffectRepository) CountFixtures(ctx context.Context, effectID string) (int64, error) {
	var count int64
	result := r.db.WithContext(ctx).
		Model(&models.EffectFixture{}).
		Where("effect_id = ?", effectID).
		Count(&count)
	return count, result.Error
}

// CountEffectsByCueID returns the number of effects attached to a cue.
func (r *EffectRepository) CountEffectsByCueID(ctx context.Context, cueID string) (int64, error) {
	var count int64
	result := r.db.WithContext(ctx).
		Model(&models.CueEffect{}).
		Where("cue_id = ?", cueID).
		Count(&count)
	return count, result.Error
}

// ---------------------------------
// Bulk Operations
// ---------------------------------

// CreateWithFixtures creates an effect with its fixtures in a transaction.
func (r *EffectRepository) CreateWithFixtures(ctx context.Context, effect *models.Effect, fixtures []models.EffectFixture) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if effect.ID == "" {
			effect.ID = cuid.New()
		}
		if err := tx.Create(effect).Error; err != nil {
			return err
		}

		if len(fixtures) > 0 {
			for i := range fixtures {
				if fixtures[i].ID == "" {
					fixtures[i].ID = cuid.New()
				}
				fixtures[i].EffectID = effect.ID
			}
			if err := tx.Create(&fixtures).Error; err != nil {
				return err
			}
		}
		return nil
	})
}

// DeleteEffectFixtures deletes all fixtures for an effect.
func (r *EffectRepository) DeleteEffectFixtures(ctx context.Context, effectID string) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// First, get all EffectFixture IDs
		var effectFixtureIDs []string
		if err := tx.Model(&models.EffectFixture{}).
			Where("effect_id = ?", effectID).
			Pluck("id", &effectFixtureIDs).Error; err != nil {
			return err
		}

		// Delete associated EffectChannels
		if len(effectFixtureIDs) > 0 {
			if err := tx.Where("effect_fixture_id IN ?", effectFixtureIDs).
				Delete(&models.EffectChannel{}).Error; err != nil {
				return err
			}
		}

		// Delete EffectFixtures
		return tx.Where("effect_id = ?", effectID).
			Delete(&models.EffectFixture{}).Error
	})
}

// FindEffectFixtureByID returns an effect fixture by its ID.
func (r *EffectRepository) FindEffectFixtureByID(ctx context.Context, id string) (*models.EffectFixture, error) {
	var effectFixture models.EffectFixture
	result := r.db.WithContext(ctx).
		Preload("Channels").
		Preload("Fixture").
		First(&effectFixture, "id = ?", id)
	if result.Error != nil {
		if errors.Is(result.Error, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, result.Error
	}
	return &effectFixture, nil
}
