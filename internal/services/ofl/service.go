package ofl

import (
	"context"
	"encoding/json"
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
	var oflFixture OFLFixture
	if err := json.Unmarshal([]byte(oflFixtureJSON), &oflFixture); err != nil {
		return nil, fmt.Errorf("invalid JSON: %w", err)
	}

	// Validate required fields
	if err := validateOFLFixture(&oflFixture); err != nil {
		return nil, err
	}

	model := oflFixture.Name

	// Check if fixture already exists
	existing, err := s.fixtureRepo.FindDefinitionByManufacturerModel(ctx, manufacturer, model)
	if err != nil && err != gorm.ErrRecordNotFound {
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

		// Process channels with fade behavior auto-detection
		channelDefs := processChannels(oflFixture.AvailableChannels)

		// Create fixture definition
		fixtureID := cuid.New()
		definition := &models.FixtureDefinition{
			ID:           fixtureID,
			Manufacturer: manufacturer,
			Model:        model,
			Type:         fixtureType,
			IsBuiltIn:    false,
		}

		if err := tx.Create(definition).Error; err != nil {
			return fmt.Errorf("failed to create fixture definition: %w", err)
		}

		// Create channel definitions
		var channels []models.ChannelDefinition
		for _, ch := range channelDefs {
			channels = append(channels, models.ChannelDefinition{
				ID:           cuid.New(),
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
			if err := tx.Create(&channels).Error; err != nil {
				return fmt.Errorf("failed to create channels: %w", err)
			}
		}

		// Build channel name to ID map
		channelNameToID := make(map[string]string)
		for _, ch := range channels {
			channelNameToID[ch.Name] = ch.ID
		}

		// Create modes
		for _, oflMode := range oflFixture.Modes {
			modeID := cuid.New()
			mode := &models.FixtureMode{
				ID:           modeID,
				Name:         oflMode.Name,
				ShortName:    stringPtr(oflMode.ShortName),
				ChannelCount: len(oflMode.Channels),
				DefinitionID: fixtureID,
			}

			if err := tx.Create(mode).Error; err != nil {
				return fmt.Errorf("failed to create mode %s: %w", oflMode.Name, err)
			}

			// Create mode channels
			for offset, channelName := range oflMode.Channels {
				// Handle switched channels (e.g., "Dimmer fine / Step Duration")
				primaryChannelName := channelName
				if strings.Contains(channelName, " / ") {
					primaryChannelName = strings.Split(channelName, " / ")[0]
				}

				channelID, ok := channelNameToID[primaryChannelName]
				if !ok {
					return fmt.Errorf("channel %q (primary: %q) in mode %q not found in availableChannels",
						channelName, primaryChannelName, oflMode.Name)
				}

				modeChannel := &models.ModeChannel{
					ID:        cuid.New(),
					ModeID:    modeID,
					ChannelID: channelID,
					Offset:    offset,
				}

				if err := tx.Create(modeChannel).Error; err != nil {
					return fmt.Errorf("failed to create mode channel: %w", err)
				}
			}
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

// validateOFLFixture validates required OFL fixture fields
func validateOFLFixture(fixture *OFLFixture) error {
	if fixture.Name == "" {
		return fmt.Errorf("OFL fixture must have a \"name\" field")
	}

	if len(fixture.Categories) == 0 {
		return fmt.Errorf("OFL fixture must have a \"categories\" array with at least one category")
	}

	if len(fixture.AvailableChannels) == 0 {
		return fmt.Errorf("OFL fixture must have \"availableChannels\" with at least one channel")
	}

	if len(fixture.Modes) == 0 {
		return fmt.Errorf("OFL fixture must have a \"modes\" array with at least one mode")
	}

	return nil
}

// processChannels converts OFL channels to our format with fade behavior auto-detection
func processChannels(availableChannels map[string]OFLChannel) []ChannelDefinition {
	var channels []ChannelDefinition
	offset := 0

	for channelName, channelData := range availableChannels {
		// Get the first capability to determine channel type and values
		var capability *OFLCapability
		if channelData.Capability != nil {
			capability = channelData.Capability
		} else if len(channelData.Capabilities) > 0 {
			capability = &channelData.Capabilities[0]
		}

		// Determine if this is a discrete channel (has multiple capabilities/ranges)
		isDiscrete := len(channelData.Capabilities) > 1

		if capability == nil {
			// Default channel if no capability info
			channels = append(channels, ChannelDefinition{
				Name:         channelName,
				Type:         "OTHER",
				Offset:       offset,
				MinValue:     0,
				MaxValue:     255,
				DefaultValue: 0,
				FadeBehavior: "FADE", // Default to FADE
				IsDiscrete:   false,
			})
			offset++
			continue
		}

		channelType := mapChannelType(capability)
		minVal, maxVal := getMinMaxValues(capability)
		defaultVal := getDefaultValue(capability)
		fadeBehavior := mapFadeBehavior(channelType, isDiscrete)

		// Add the main channel
		channels = append(channels, ChannelDefinition{
			Name:         channelName,
			Type:         channelType,
			Offset:       offset,
			MinValue:     minVal,
			MaxValue:     maxVal,
			DefaultValue: defaultVal,
			FadeBehavior: fadeBehavior,
			IsDiscrete:   isDiscrete,
		})
		offset++

		// Add fine channel aliases if they exist
		for _, fineAlias := range channelData.FineChannelAliases {
			channels = append(channels, ChannelDefinition{
				Name:         fineAlias,
				Type:         channelType, // Same type as parent channel
				Offset:       offset,
				MinValue:     0,
				MaxValue:     255,
				DefaultValue: 0,
				FadeBehavior: fadeBehavior, // Same behavior as parent
				IsDiscrete:   false,        // Fine channels are always continuous
			})
			offset++
		}
	}

	return channels
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
	case "EffectSpeed":
		return "EFFECT"
	case "Speed":
		return "EFFECT"
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
	// PAN, TILT, ZOOM, FOCUS, IRIS, EFFECT, OTHER
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

// stringPtr returns a pointer to a string, or nil if empty
func stringPtr(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}
