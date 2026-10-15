package ofl

import (
	"context"
	"fmt"
	"strings"

	"github.com/bbernstein/lacylights-go/internal/database/models"
	"github.com/bbernstein/lacylights-go/internal/database/repositories"
	"github.com/lucsky/cuid"
	"gorm.io/gorm"
)

// Service handles OFL fixture import operations
type Service struct {
	db          *gorm.DB
	fixtureRepo *repositories.FixtureRepository
}

// NewService creates a new OFL import service
func NewService(db *gorm.DB, fixtureRepo *repositories.FixtureRepository) *Service {
	return &Service{
		db:          db,
		fixtureRepo: fixtureRepo,
	}
}

// ImportFixture imports a fixture from OFL JSON format
func (s *Service) ImportFixture(ctx context.Context, manufacturer, oflFixtureJSON string, replace bool) (*models.FixtureDefinition, error) {
	// Parse the OFL JSON
	oflFixture, err := parseOFLFixture(oflFixtureJSON)
	if err != nil {
		return nil, err
	}

	// Validate and resolve every mode channel before touching the database
	plan, err := buildImportPlan(oflFixture)
	if err != nil {
		return nil, err
	}

	model := oflFixture.Name

	// Check if fixture already exists
	existing, err := s.fixtureRepo.FindDefinitionByManufacturerModel(ctx, manufacturer, model)
	if err != nil {
		return nil, fmt.Errorf("error checking existing fixture: %w", err)
	}

	if existing != nil && !replace {
		// Count instances using this definition
		instanceCount, _ := s.fixtureRepo.CountInstancesByDefinitionID(ctx, existing.ID)
		return nil, fmt.Errorf("FIXTURE_EXISTS:%s %s:%d", manufacturer, model, instanceCount)
	}

	// Process in transaction
	var result *models.FixtureDefinition
	err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// Delete existing fixture if replacing
		if existing != nil && replace {
			if err := tx.Delete(existing).Error; err != nil {
				return fmt.Errorf("failed to delete existing fixture: %w", err)
			}
		}

		// Map fixture type from categories
		fixtureType := mapFixtureType(oflFixture.Categories)

		// Create fixture definition
		fixtureID := cuid.New()
		definition := &models.FixtureDefinition{
			ID:           fixtureID,
			RefID:        refIDForImport(existing, fixtureID),
			Manufacturer: manufacturer,
			Model:        model,
			Type:         fixtureType,
			IsBuiltIn:    false,
		}

		if err := tx.Create(definition).Error; err != nil {
			return fmt.Errorf("failed to create fixture definition: %w", err)
		}

		if err := writeImportPlan(tx, fixtureID, plan); err != nil {
			return err
		}

		// Load the complete result with relations
		if err := tx.Preload("Channels").Preload("Modes").First(definition, "id = ?", fixtureID).Error; err != nil {
			return fmt.Errorf("failed to load created fixture: %w", err)
		}

		result = definition
		return nil
	})

	if err != nil {
		return nil, err
	}

	return result, nil
}

// mapChannelType maps OFL capability type to our ChannelType enum
func mapChannelType(capability *OFLCapability) string {
	capType := capability.Type

	switch capType {
	case "Intensity":
		return "INTENSITY"

	case "ColorIntensity":
		if capability.Color != "" {
			switch strings.ToLower(capability.Color) {
			case "red":
				return "RED"
			case "green":
				return "GREEN"
			case "blue":
				return "BLUE"
			case "white":
				return "WHITE"
			case "amber":
				return "AMBER"
			case "uv":
				return "UV"
			case "cyan":
				return "CYAN"
			case "magenta":
				return "MAGENTA"
			case "yellow":
				return "YELLOW"
			case "lime":
				return "LIME"
			case "indigo":
				return "INDIGO"
			case "cold white":
				return "COLD_WHITE"
			case "warm white":
				return "WARM_WHITE"
			}
		}
		return "OTHER"

	case "Pan":
		return "PAN"
	case "Tilt":
		return "TILT"
	case "Zoom":
		return "ZOOM"
	case "Focus":
		return "FOCUS"
	case "Iris":
		return "IRIS"
	case "Gobo":
		return "GOBO"
	case "WheelSlot":
		return "GOBO" // WheelSlot is typically gobo or color wheel
	case "ColorWheel":
		return "COLOR_WHEEL"
	case "Effect":
		return "EFFECT"
	case "ShutterStrobe":
		return "STROBE"
	case "Maintenance":
		return "MACRO"
	case "ColorPreset":
		return "COLOR_WHEEL"
	case "ColorTemperature":
		return "COLOR_TEMPERATURE"
	case "EffectSpeed", "Speed", "PanTiltSpeed":
		return "SPEED"
	case "Rotation":
		return "EFFECT"
	case "NoFunction":
		return "OTHER"
	default:
		return "OTHER"
	}
}

// mapFadeBehavior determines the appropriate fade behavior based on channel type
// This is the core of Phase 3: auto-detecting which channels should snap vs fade
func mapFadeBehavior(channelType string, isDiscrete bool) string {
	// Discrete channels (multiple DMX ranges) should always snap
	if isDiscrete {
		return "SNAP"
	}

	// Channel types that should SNAP (instant change)
	// These are channels where intermediate values don't make sense
	snapTypes := map[string]bool{
		"GOBO":        true, // Gobo wheel positions
		"COLOR_WHEEL": true, // Color wheel positions
		"MACRO":       true, // Program/macro selection
		"STROBE":      true, // Strobe effects (often have distinct modes)
	}

	if snapTypes[channelType] {
		return "SNAP"
	}

	// All other channels should FADE smoothly
	// This includes: INTENSITY, RED, GREEN, BLUE, WHITE, AMBER, UV,
	// COLOR_TEMPERATURE, HUE, SATURATION, PAN, TILT, ZOOM, FOCUS, IRIS,
	// EFFECT, SPEED, OTHER
	return "FADE"
}

// mapFixtureType maps OFL categories to our FixtureType enum
func mapFixtureType(categories []string) string {
	for _, category := range categories {
		catLower := strings.ToLower(category)

		if strings.Contains(catLower, "moving head") || strings.Contains(catLower, "scanner") {
			return "MOVING_HEAD"
		}
		if strings.Contains(catLower, "strobe") || strings.Contains(catLower, "blinder") {
			return "STROBE"
		}
		if strings.Contains(catLower, "dimmer") {
			return "DIMMER"
		}
		if strings.Contains(catLower, "color changer") || strings.Contains(catLower, "par") || strings.Contains(catLower, "wash") {
			return "LED_PAR"
		}
	}

	return "OTHER"
}

// getMinMaxValues extracts DMX range from OFL capability
func getMinMaxValues(capability *OFLCapability) (int, int) {
	if capability.DMXRange != nil {
		return capability.DMXRange[0], capability.DMXRange[1]
	}
	return 0, 255
}

// getDefaultValue determines the default DMX value for a capability
func getDefaultValue(capability *OFLCapability) int {
	// For intensity channels, default to 0 (off)
	if capability.Type == "Intensity" || capability.Type == "ColorIntensity" {
		return 0
	}

	// For position channels (pan/tilt), default to center
	if capability.Type == "Pan" || capability.Type == "Tilt" {
		if capability.DMXRange != nil {
			return (capability.DMXRange[0] + capability.DMXRange[1]) / 2
		}
		return 127 // Center of 0-255
	}

	// For other channels, use the minimum value if available
	if capability.DMXRange != nil {
		return capability.DMXRange[0]
	}

	return 0
}

// refIDForImport returns the RefID for a newly created OFL definition.
// fixture_definitions.ref_id is globally unique, so it must never be empty.
// When replacing an existing definition (deleted earlier in the same
// transaction) its RefID is preserved so round-trip references stay stable;
// otherwise it defaults to the new ID, matching the repository convention.
func refIDForImport(existing *models.FixtureDefinition, newID string) string {
	if existing != nil && existing.RefID != "" {
		return existing.RefID
	}
	return newID
}

// stringPtr returns a pointer to a string, or nil if empty
func stringPtr(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

// ImportFixtureWithHash imports a fixture from OFL JSON format with hash tracking for change detection
func (s *Service) ImportFixtureWithHash(ctx context.Context, manufacturer, oflFixtureJSON, hash, version string, update bool) (*models.FixtureDefinition, error) {
	// Parse the OFL JSON
	oflFixture, err := parseOFLFixture(oflFixtureJSON)
	if err != nil {
		return nil, err
	}

	// Validate and resolve every mode channel before touching the database
	plan, err := buildImportPlan(oflFixture)
	if err != nil {
		return nil, err
	}

	model := oflFixture.Name

	// Check if fixture already exists
	existing, err := s.fixtureRepo.FindDefinitionByManufacturerModel(ctx, manufacturer, model)
	if err != nil {
		return nil, fmt.Errorf("error checking existing fixture: %w", err)
	}

	if existing != nil && !update {
		return nil, fmt.Errorf("FIXTURE_EXISTS:%s %s:0", manufacturer, model)
	}

	// Process in transaction
	var result *models.FixtureDefinition
	err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// Delete existing fixture if updating
		if existing != nil && update {
			// First delete related mode channels
			var modeIDs []string
			if err := tx.Table("fixture_modes").Where("definition_id = ?", existing.ID).Pluck("id", &modeIDs).Error; err != nil {
				return fmt.Errorf("failed to load existing modes: %w", err)
			}
			if len(modeIDs) > 0 {
				if err := tx.Where("mode_id IN ?", modeIDs).Delete(&models.ModeChannel{}).Error; err != nil {
					return fmt.Errorf("failed to delete existing mode channels: %w", err)
				}
			}
			// Delete modes
			if err := tx.Where("definition_id = ?", existing.ID).Delete(&models.FixtureMode{}).Error; err != nil {
				return fmt.Errorf("failed to delete existing modes: %w", err)
			}
			// Delete channels
			if err := tx.Where("definition_id = ?", existing.ID).Delete(&models.ChannelDefinition{}).Error; err != nil {
				return fmt.Errorf("failed to delete existing channels: %w", err)
			}
			// Delete definition
			if err := tx.Delete(existing).Error; err != nil {
				return fmt.Errorf("failed to delete existing fixture: %w", err)
			}
		}

		// Map fixture type from categories
		fixtureType := mapFixtureType(oflFixture.Categories)

		// Create fixture definition with hash and version
		fixtureID := cuid.New()
		definition := &models.FixtureDefinition{
			ID:            fixtureID,
			RefID:         refIDForImport(existing, fixtureID),
			Manufacturer:  manufacturer,
			Model:         model,
			Type:          fixtureType,
			IsBuiltIn:     false,
			OFLSourceHash: &hash,
			OFLVersion:    &version,
		}

		if err := tx.Create(definition).Error; err != nil {
			return fmt.Errorf("failed to create fixture definition: %w", err)
		}

		if err := writeImportPlan(tx, fixtureID, plan); err != nil {
			return err
		}

		// Load the complete result with relations
		if err := tx.Preload("Channels").Preload("Modes").First(definition, "id = ?", fixtureID).Error; err != nil {
			return fmt.Errorf("failed to load created fixture: %w", err)
		}

		result = definition
		return nil
	})

	if err != nil {
		return nil, err
	}

	return result, nil
}

// writeImportPlan creates the channel definitions, modes and mode channels
// of a resolved plan for the definition fixtureID. Each mode gets one mode
// channel per DMX offset, in the order the OFL mode lists them.
func writeImportPlan(tx *gorm.DB, fixtureID string, plan *importPlan) error {
	channelIDs := make([]string, len(plan.Channels))
	channels := make([]models.ChannelDefinition, 0, len(plan.Channels))
	for i, ch := range plan.Channels {
		channelIDs[i] = cuid.New()
		channels = append(channels, models.ChannelDefinition{
			ID:           channelIDs[i],
			Name:         ch.Name,
			Type:         ch.Type,
			Offset:       ch.Offset,
			MinValue:     ch.MinValue,
			MaxValue:     ch.MaxValue,
			DefaultValue: ch.DefaultValue,
			FadeBehavior: ch.FadeBehavior,
			IsDiscrete:   ch.IsDiscrete,
			DefinitionID: fixtureID,
		})
	}
	if len(channels) > 0 {
		if err := tx.CreateInBatches(&channels, 100).Error; err != nil {
			return fmt.Errorf("failed to create channels: %w", err)
		}
	}

	for _, pm := range plan.Modes {
		modeID := cuid.New()
		mode := &models.FixtureMode{
			ID:           modeID,
			Name:         pm.Name,
			ShortName:    stringPtr(pm.ShortName),
			ChannelCount: len(pm.channelIdx),
			DefinitionID: fixtureID,
		}
		if err := tx.Create(mode).Error; err != nil {
			return fmt.Errorf("failed to create mode %s: %w", pm.Name, err)
		}
		if len(pm.channelIdx) == 0 {
			continue
		}
		modeChannels := make([]models.ModeChannel, 0, len(pm.channelIdx))
		for offset, idx := range pm.channelIdx {
			modeChannels = append(modeChannels, models.ModeChannel{
				ID:        cuid.New(),
				ModeID:    modeID,
				ChannelID: channelIDs[idx],
				Offset:    offset,
			})
		}
		if err := tx.CreateInBatches(&modeChannels, 100).Error; err != nil {
			return fmt.Errorf("failed to create mode channels for mode %s: %w", pm.Name, err)
		}
	}
	return nil
}
