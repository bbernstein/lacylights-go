package repositories

import (
	"context"

	"github.com/bbernstein/lacylights-go/internal/database/models"
	"github.com/lucsky/cuid"
	"gorm.io/gorm"
)

// FixtureRepository handles fixture data access.
type FixtureRepository struct {
	db *gorm.DB
}

// NewFixtureRepository creates a new FixtureRepository.
func NewFixtureRepository(db *gorm.DB) *FixtureRepository {
	return &FixtureRepository{db: db}
}

// FindByProjectID returns all fixtures in a project.
func (r *FixtureRepository) FindByProjectID(ctx context.Context, projectID string) ([]models.FixtureInstance, error) {
	var fixtures []models.FixtureInstance
	result := r.db.WithContext(ctx).
		Where("project_id = ?", projectID).
		Order("universe ASC, start_channel ASC").
		Find(&fixtures)
	return fixtures, result.Error
}

// FindByID returns a fixture by ID.
func (r *FixtureRepository) FindByID(ctx context.Context, id string) (*models.FixtureInstance, error) {
	var fixture models.FixtureInstance
	result := r.db.WithContext(ctx).First(&fixture, "id = ?", id)
	if result.Error != nil {
		if result.Error == gorm.ErrRecordNotFound {
			return nil, nil
		}
		return nil, result.Error
	}
	return &fixture, nil
}

// Create creates a new fixture instance.
func (r *FixtureRepository) Create(ctx context.Context, fixture *models.FixtureInstance) error {
	if fixture.ID == "" {
		fixture.ID = cuid.New()
	}
	return r.db.WithContext(ctx).Create(fixture).Error
}

// Update updates an existing fixture instance.
func (r *FixtureRepository) Update(ctx context.Context, fixture *models.FixtureInstance) error {
	return r.db.WithContext(ctx).Save(fixture).Error
}

// Delete deletes a fixture instance by ID.
func (r *FixtureRepository) Delete(ctx context.Context, id string) error {
	return r.db.WithContext(ctx).Delete(&models.FixtureInstance{}, "id = ?", id).Error
}

// FindDefinitionByID returns a fixture definition by ID.
func (r *FixtureRepository) FindDefinitionByID(ctx context.Context, id string) (*models.FixtureDefinition, error) {
	var def models.FixtureDefinition
	result := r.db.WithContext(ctx).First(&def, "id = ?", id)
	if result.Error != nil {
		if result.Error == gorm.ErrRecordNotFound {
			return nil, nil
		}
		return nil, result.Error
	}
	return &def, nil
}

// FindDefinitionByManufacturerModel returns a fixture definition by manufacturer and model.
func (r *FixtureRepository) FindDefinitionByManufacturerModel(ctx context.Context, manufacturer, model string) (*models.FixtureDefinition, error) {
	var def models.FixtureDefinition
	result := r.db.WithContext(ctx).
		Where("manufacturer = ? AND model = ?", manufacturer, model).
		First(&def)
	if result.Error != nil {
		if result.Error == gorm.ErrRecordNotFound {
			return nil, nil
		}
		return nil, result.Error
	}
	return &def, nil
}

// FindAllDefinitions returns all fixture definitions.
func (r *FixtureRepository) FindAllDefinitions(ctx context.Context) ([]models.FixtureDefinition, error) {
	var defs []models.FixtureDefinition
	result := r.db.WithContext(ctx).
		Order("manufacturer ASC, model ASC").
		Find(&defs)
	return defs, result.Error
}

// GetInstanceChannels returns all channels for a fixture instance.
func (r *FixtureRepository) GetInstanceChannels(ctx context.Context, fixtureID string) ([]models.InstanceChannel, error) {
	var channels []models.InstanceChannel
	result := r.db.WithContext(ctx).
		Where("fixture_id = ?", fixtureID).
		Order("offset ASC").
		Find(&channels)
	return channels, result.Error
}

// GetDefinitionChannels returns all channels for a fixture definition.
func (r *FixtureRepository) GetDefinitionChannels(ctx context.Context, definitionID string) ([]models.ChannelDefinition, error) {
	var channels []models.ChannelDefinition
	result := r.db.WithContext(ctx).
		Where("definition_id = ?", definitionID).
		Order("offset ASC").
		Find(&channels)
	return channels, result.Error
}

// FindModeByID returns a fixture mode by ID.
func (r *FixtureRepository) FindModeByID(ctx context.Context, id string) (*models.FixtureMode, error) {
	var mode models.FixtureMode
	result := r.db.WithContext(ctx).First(&mode, "id = ?", id)
	if result.Error != nil {
		if result.Error == gorm.ErrRecordNotFound {
			return nil, nil
		}
		return nil, result.Error
	}
	return &mode, nil
}

// GetModeChannels returns all mode channels for a mode with their channel definitions.
func (r *FixtureRepository) GetModeChannels(ctx context.Context, modeID string) ([]models.ModeChannel, error) {
	var modeChannels []models.ModeChannel
	result := r.db.WithContext(ctx).
		Where("mode_id = ?", modeID).
		Order("offset ASC").
		Find(&modeChannels)
	return modeChannels, result.Error
}

// GetChannelDefinitionByID returns a channel definition by ID.
func (r *FixtureRepository) GetChannelDefinitionByID(ctx context.Context, id string) (*models.ChannelDefinition, error) {
	var channel models.ChannelDefinition
	result := r.db.WithContext(ctx).First(&channel, "id = ?", id)
	if result.Error != nil {
		if result.Error == gorm.ErrRecordNotFound {
			return nil, nil
		}
		return nil, result.Error
	}
	return &channel, nil
}

// CreateInstanceChannels creates instance channels for a fixture.
func (r *FixtureRepository) CreateInstanceChannels(ctx context.Context, channels []models.InstanceChannel) error {
	if len(channels) == 0 {
		return nil
	}
	for i := range channels {
		if channels[i].ID == "" {
			channels[i].ID = cuid.New()
		}
	}
	return r.db.WithContext(ctx).Create(&channels).Error
}

// DeleteInstanceChannels deletes all instance channels for a fixture.
func (r *FixtureRepository) DeleteInstanceChannels(ctx context.Context, fixtureID string) error {
	return r.db.WithContext(ctx).Delete(&models.InstanceChannel{}, "fixture_id = ?", fixtureID).Error
}

// CreateWithChannels creates a fixture instance with its channels in a transaction.
func (r *FixtureRepository) CreateWithChannels(ctx context.Context, fixture *models.FixtureInstance, channels []models.InstanceChannel) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if fixture.ID == "" {
			fixture.ID = cuid.New()
		}
		if err := tx.Create(fixture).Error; err != nil {
			return err
		}

		if len(channels) > 0 {
			for i := range channels {
				if channels[i].ID == "" {
					channels[i].ID = cuid.New()
				}
				channels[i].FixtureID = fixture.ID
			}
			if err := tx.Create(&channels).Error; err != nil {
				return err
			}
		}
		return nil
	})
}

// CreateDefinition creates a new fixture definition.
func (r *FixtureRepository) CreateDefinition(ctx context.Context, definition *models.FixtureDefinition) error {
	if definition.ID == "" {
		definition.ID = cuid.New()
	}
	return r.db.WithContext(ctx).Create(definition).Error
}

// UpdateDefinition updates an existing fixture definition.
func (r *FixtureRepository) UpdateDefinition(ctx context.Context, definition *models.FixtureDefinition) error {
	return r.db.WithContext(ctx).Save(definition).Error
}

// DeleteDefinition deletes a fixture definition by ID.
func (r *FixtureRepository) DeleteDefinition(ctx context.Context, id string) error {
	return r.db.WithContext(ctx).Delete(&models.FixtureDefinition{}, "id = ?", id).Error
}

// CreateChannelDefinition creates a new channel definition.
func (r *FixtureRepository) CreateChannelDefinition(ctx context.Context, channel *models.ChannelDefinition) error {
	if channel.ID == "" {
		channel.ID = cuid.New()
	}
	return r.db.WithContext(ctx).Create(channel).Error
}

// CreateChannelDefinitions creates multiple channel definitions.
func (r *FixtureRepository) CreateChannelDefinitions(ctx context.Context, channels []models.ChannelDefinition) error {
	if len(channels) == 0 {
		return nil
	}
	for i := range channels {
		if channels[i].ID == "" {
			channels[i].ID = cuid.New()
		}
	}
	return r.db.WithContext(ctx).Create(&channels).Error
}

// DeleteChannelDefinitions deletes all channel definitions for a fixture definition.
func (r *FixtureRepository) DeleteChannelDefinitions(ctx context.Context, definitionID string) error {
	return r.db.WithContext(ctx).Delete(&models.ChannelDefinition{}, "definition_id = ?", definitionID).Error
}

// CreateDefinitionWithChannels creates a fixture definition with its channels in a transaction.
func (r *FixtureRepository) CreateDefinitionWithChannels(ctx context.Context, definition *models.FixtureDefinition, channels []models.ChannelDefinition) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if definition.ID == "" {
			definition.ID = cuid.New()
		}
		if err := tx.Create(definition).Error; err != nil {
			return err
		}

		if len(channels) > 0 {
			for i := range channels {
				if channels[i].ID == "" {
					channels[i].ID = cuid.New()
				}
				channels[i].DefinitionID = definition.ID
			}
			if err := tx.Create(&channels).Error; err != nil {
				return err
			}
		}
		return nil
	})
}

// CountInstancesByDefinitionID returns the count of fixture instances using a definition.
func (r *FixtureRepository) CountInstancesByDefinitionID(ctx context.Context, definitionID string) (int64, error) {
	var count int64
	result := r.db.WithContext(ctx).
		Model(&models.FixtureInstance{}).
		Where("definition_id = ?", definitionID).
		Count(&count)
	return count, result.Error
}

// CountDefinitions returns the total count of fixture definitions in the database.
func (r *FixtureRepository) CountDefinitions(ctx context.Context) (int64, error) {
	var count int64
	result := r.db.WithContext(ctx).
		Model(&models.FixtureDefinition{}).
		Count(&count)
	return count, result.Error
}

// GetDefinitionModes returns all modes for a fixture definition.
func (r *FixtureRepository) GetDefinitionModes(ctx context.Context, definitionID string) ([]models.FixtureMode, error) {
	var modes []models.FixtureMode
	result := r.db.WithContext(ctx).
		Where("definition_id = ?", definitionID).
		Order("name ASC").
		Find(&modes)
	return modes, result.Error
}

// CreateMode creates a new fixture mode.
func (r *FixtureRepository) CreateMode(ctx context.Context, mode *models.FixtureMode) error {
	if mode.ID == "" {
		mode.ID = cuid.New()
	}
	return r.db.WithContext(ctx).Create(mode).Error
}

// CreateModeChannels creates multiple mode channels.
func (r *FixtureRepository) CreateModeChannels(ctx context.Context, modeChannels []models.ModeChannel) error {
	if len(modeChannels) == 0 {
		return nil
	}
	for i := range modeChannels {
		if modeChannels[i].ID == "" {
			modeChannels[i].ID = cuid.New()
		}
	}
	return r.db.WithContext(ctx).Create(&modeChannels).Error
}

// DeleteDefinitionModes deletes all modes for a fixture definition.
func (r *FixtureRepository) DeleteDefinitionModes(ctx context.Context, definitionID string) error {
	// First get all mode IDs
	var modeIDs []string
	if err := r.db.WithContext(ctx).
		Model(&models.FixtureMode{}).
		Where("definition_id = ?", definitionID).
		Pluck("id", &modeIDs).Error; err != nil {
		return err
	}

	// Delete mode channels for these modes
	if len(modeIDs) > 0 {
		if err := r.db.WithContext(ctx).
			Delete(&models.ModeChannel{}, "mode_id IN ?", modeIDs).Error; err != nil {
			return err
		}
	}

	// Delete the modes
	return r.db.WithContext(ctx).Delete(&models.FixtureMode{}, "definition_id = ?", definitionID).Error
}
