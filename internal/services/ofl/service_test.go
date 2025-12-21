package ofl

import (
	"context"
	"testing"

	"github.com/bbernstein/lacylights-go/internal/database/models"
	"github.com/bbernstein/lacylights-go/internal/services/testutil"
)

func setupTestDB(t *testing.T) (*Service, func()) {
	testDB, cleanup := testutil.SetupTestDB(t)

	service := NewService(testDB.DB, testDB.FixtureRepo)

	return service, cleanup
}

// Minimal valid OFL fixture JSON
const minimalOFLFixture = `{
	"name": "Test Fixture",
	"categories": ["Color Changer"],
	"availableChannels": {
		"Dimmer": {
			"capability": {
				"type": "Intensity"
			}
		}
	},
	"modes": [
		{
			"name": "1 Channel",
			"channels": ["Dimmer"]
		}
	]
}`

// Complex fixture with multiple channel types
const complexOFLFixture = `{
	"name": "Complex LED PAR",
	"categories": ["Color Changer", "PAR"],
	"availableChannels": {
		"Dimmer": {
			"capability": {
				"type": "Intensity",
				"dmxRange": [0, 255]
			},
			"fineChannelAliases": ["Dimmer fine"]
		},
		"Red": {
			"capability": {
				"type": "ColorIntensity",
				"color": "Red"
			}
		},
		"Green": {
			"capability": {
				"type": "ColorIntensity",
				"color": "Green"
			}
		},
		"Blue": {
			"capability": {
				"type": "ColorIntensity",
				"color": "Blue"
			}
		},
		"Strobe": {
			"capabilities": [
				{"type": "NoFunction", "dmxRange": [0, 9]},
				{"type": "ShutterStrobe", "dmxRange": [10, 255]}
			]
		},
		"Color Macros": {
			"capabilities": [
				{"type": "NoFunction", "dmxRange": [0, 9]},
				{"type": "ColorPreset", "dmxRange": [10, 50]},
				{"type": "ColorPreset", "dmxRange": [51, 100]},
				{"type": "Effect", "dmxRange": [101, 255]}
			]
		},
		"Pan": {
			"capability": {
				"type": "Pan",
				"dmxRange": [0, 255]
			}
		},
		"Gobo": {
			"capabilities": [
				{"type": "Gobo", "dmxRange": [0, 20]},
				{"type": "Gobo", "dmxRange": [21, 40]},
				{"type": "Gobo", "dmxRange": [41, 60]},
				{"type": "Gobo", "dmxRange": [61, 80]}
			]
		}
	},
	"modes": [
		{
			"name": "Standard",
			"shortName": "STD",
			"channels": ["Dimmer", "Dimmer fine", "Red", "Green", "Blue", "Strobe", "Color Macros", "Pan", "Gobo"]
		},
		{
			"name": "Simple",
			"channels": ["Dimmer", "Red", "Green", "Blue"]
		}
	]
}`

func TestImportFixture_MinimalValid(t *testing.T) {
	service, cleanup := setupTestDB(t)
	defer cleanup()

	ctx := context.Background()
	result, err := service.ImportFixture(ctx, "Test Manufacturer", minimalOFLFixture, false)
	if err != nil {
		t.Fatalf("ImportFixture failed: %v", err)
	}

	if result.Manufacturer != "Test Manufacturer" {
		t.Errorf("Manufacturer = %q, want %q", result.Manufacturer, "Test Manufacturer")
	}
	if result.Model != "Test Fixture" {
		t.Errorf("Model = %q, want %q", result.Model, "Test Fixture")
	}
	if result.Type != "LED_PAR" {
		t.Errorf("Type = %q, want %q (Color Changer should map to LED_PAR)", result.Type, "LED_PAR")
	}
	if len(result.Channels) != 1 {
		t.Errorf("Channel count = %d, want 1", len(result.Channels))
	}
	if len(result.Modes) != 1 {
		t.Errorf("Mode count = %d, want 1", len(result.Modes))
	}
}

func TestImportFixture_ComplexFixture(t *testing.T) {
	service, cleanup := setupTestDB(t)
	defer cleanup()

	ctx := context.Background()
	result, err := service.ImportFixture(ctx, "Chauvet", complexOFLFixture, false)
	if err != nil {
		t.Fatalf("ImportFixture failed: %v", err)
	}

	// Verify basic info
	if result.Model != "Complex LED PAR" {
		t.Errorf("Model = %q, want %q", result.Model, "Complex LED PAR")
	}

	// Count channels (including fine channel alias)
	// Dimmer, Dimmer fine, Red, Green, Blue, Strobe, Color Macros, Pan, Gobo = 9
	if len(result.Channels) != 9 {
		t.Errorf("Channel count = %d, want 9", len(result.Channels))
	}

	// Verify modes
	if len(result.Modes) != 2 {
		t.Errorf("Mode count = %d, want 2", len(result.Modes))
	}
}

func TestImportFixture_FadeBehaviorAutoDetection(t *testing.T) {
	service, cleanup := setupTestDB(t)
	defer cleanup()

	ctx := context.Background()
	result, err := service.ImportFixture(ctx, "Chauvet", complexOFLFixture, false)
	if err != nil {
		t.Fatalf("ImportFixture failed: %v", err)
	}

	// Create a map of channel names to channels for easier testing
	channelMap := make(map[string]*models.ChannelDefinition)
	for i := range result.Channels {
		channelMap[result.Channels[i].Name] = &result.Channels[i]
	}

	// Test FADE channels (should interpolate smoothly)
	fadeChannels := []string{"Dimmer", "Dimmer fine", "Red", "Green", "Blue", "Pan"}
	for _, name := range fadeChannels {
		ch, ok := channelMap[name]
		if !ok {
			t.Errorf("Channel %q not found", name)
			continue
		}
		if ch.FadeBehavior != "FADE" {
			t.Errorf("Channel %q FadeBehavior = %q, want FADE", name, ch.FadeBehavior)
		}
	}

	// Test SNAP channels (should jump instantly)
	snapChannels := []string{"Gobo", "Color Macros"}
	for _, name := range snapChannels {
		ch, ok := channelMap[name]
		if !ok {
			t.Errorf("Channel %q not found", name)
			continue
		}
		if ch.FadeBehavior != "SNAP" {
			t.Errorf("Channel %q FadeBehavior = %q, want SNAP (discrete/wheel channel)", name, ch.FadeBehavior)
		}
	}

	// Test Strobe - it has multiple capabilities so should be SNAP
	strobeChannel := channelMap["Strobe"]
	if strobeChannel == nil {
		t.Error("Strobe channel not found")
	} else if strobeChannel.FadeBehavior != "SNAP" {
		t.Errorf("Strobe FadeBehavior = %q, want SNAP (discrete)", strobeChannel.FadeBehavior)
	}
}

func TestImportFixture_IsDiscreteAutoDetection(t *testing.T) {
	service, cleanup := setupTestDB(t)
	defer cleanup()

	ctx := context.Background()
	result, err := service.ImportFixture(ctx, "Chauvet", complexOFLFixture, false)
	if err != nil {
		t.Fatalf("ImportFixture failed: %v", err)
	}

	channelMap := make(map[string]*models.ChannelDefinition)
	for i := range result.Channels {
		channelMap[result.Channels[i].Name] = &result.Channels[i]
	}

	// Continuous channels (single capability) should NOT be discrete
	continuousChannels := []string{"Dimmer", "Red", "Green", "Blue", "Pan"}
	for _, name := range continuousChannels {
		ch := channelMap[name]
		if ch == nil {
			t.Errorf("Channel %q not found", name)
			continue
		}
		if ch.IsDiscrete {
			t.Errorf("Channel %q IsDiscrete = true, want false (single capability)", name)
		}
	}

	// Discrete channels (multiple capabilities) should be discrete
	discreteChannels := []string{"Strobe", "Color Macros", "Gobo"}
	for _, name := range discreteChannels {
		ch := channelMap[name]
		if ch == nil {
			t.Errorf("Channel %q not found", name)
			continue
		}
		if !ch.IsDiscrete {
			t.Errorf("Channel %q IsDiscrete = false, want true (multiple capabilities)", name)
		}
	}
}

func TestImportFixture_ChannelTypes(t *testing.T) {
	service, cleanup := setupTestDB(t)
	defer cleanup()

	ctx := context.Background()
	result, err := service.ImportFixture(ctx, "Test", complexOFLFixture, false)
	if err != nil {
		t.Fatalf("ImportFixture failed: %v", err)
	}

	channelMap := make(map[string]*models.ChannelDefinition)
	for i := range result.Channels {
		channelMap[result.Channels[i].Name] = &result.Channels[i]
	}

	// Test channel type mappings
	expectedTypes := map[string]string{
		"Dimmer":       "INTENSITY",
		"Red":          "RED",
		"Green":        "GREEN",
		"Blue":         "BLUE",
		"Pan":          "PAN",
		"Gobo":         "GOBO",
		"Strobe":       "OTHER", // First capability is NoFunction
		"Color Macros": "OTHER", // First capability is NoFunction
	}

	for name, expectedType := range expectedTypes {
		ch := channelMap[name]
		if ch == nil {
			t.Errorf("Channel %q not found", name)
			continue
		}
		if ch.Type != expectedType {
			t.Errorf("Channel %q Type = %q, want %q", name, ch.Type, expectedType)
		}
	}
}

func TestImportFixture_InvalidJSON(t *testing.T) {
	service, cleanup := setupTestDB(t)
	defer cleanup()

	ctx := context.Background()
	_, err := service.ImportFixture(ctx, "Test", "not json", false)
	if err == nil {
		t.Error("Expected error for invalid JSON, got nil")
	}
}

func TestImportFixture_MissingName(t *testing.T) {
	service, cleanup := setupTestDB(t)
	defer cleanup()

	json := `{
		"categories": ["Color Changer"],
		"availableChannels": {"Dimmer": {"capability": {"type": "Intensity"}}},
		"modes": [{"name": "1ch", "channels": ["Dimmer"]}]
	}`

	ctx := context.Background()
	_, err := service.ImportFixture(ctx, "Test", json, false)
	if err == nil {
		t.Error("Expected error for missing name, got nil")
	}
}

func TestImportFixture_MissingCategories(t *testing.T) {
	service, cleanup := setupTestDB(t)
	defer cleanup()

	json := `{
		"name": "Test",
		"categories": [],
		"availableChannels": {"Dimmer": {"capability": {"type": "Intensity"}}},
		"modes": [{"name": "1ch", "channels": ["Dimmer"]}]
	}`

	ctx := context.Background()
	_, err := service.ImportFixture(ctx, "Test", json, false)
	if err == nil {
		t.Error("Expected error for empty categories, got nil")
	}
}

func TestImportFixture_DuplicateDetection(t *testing.T) {
	service, cleanup := setupTestDB(t)
	defer cleanup()

	ctx := context.Background()

	// First import should succeed
	_, err := service.ImportFixture(ctx, "Test", minimalOFLFixture, false)
	if err != nil {
		t.Fatalf("First import failed: %v", err)
	}

	// Second import (same fixture) should fail with FIXTURE_EXISTS error
	_, err = service.ImportFixture(ctx, "Test", minimalOFLFixture, false)
	if err == nil {
		t.Error("Expected FIXTURE_EXISTS error, got nil")
	}
	if err != nil && !containsString(err.Error(), "FIXTURE_EXISTS") {
		t.Errorf("Expected FIXTURE_EXISTS error, got: %v", err)
	}
}

func TestImportFixture_ReplaceExisting(t *testing.T) {
	service, cleanup := setupTestDB(t)
	defer cleanup()

	ctx := context.Background()

	// First import
	_, err := service.ImportFixture(ctx, "Test", minimalOFLFixture, false)
	if err != nil {
		t.Fatalf("First import failed: %v", err)
	}

	// Second import with replace=true should succeed
	result, err := service.ImportFixture(ctx, "Test", minimalOFLFixture, true)
	if err != nil {
		t.Fatalf("Replace import failed: %v", err)
	}

	if result.Model != "Test Fixture" {
		t.Errorf("Model = %q, want %q", result.Model, "Test Fixture")
	}
}

func TestMapFadeBehavior(t *testing.T) {
	tests := []struct {
		channelType string
		isDiscrete  bool
		want        string
	}{
		// Continuous channels should FADE
		{"INTENSITY", false, "FADE"},
		{"RED", false, "FADE"},
		{"GREEN", false, "FADE"},
		{"BLUE", false, "FADE"},
		{"WHITE", false, "FADE"},
		{"AMBER", false, "FADE"},
		{"UV", false, "FADE"},
		{"CYAN", false, "FADE"},
		{"MAGENTA", false, "FADE"},
		{"YELLOW", false, "FADE"},
		{"LIME", false, "FADE"},
		{"INDIGO", false, "FADE"},
		{"COLD_WHITE", false, "FADE"},
		{"WARM_WHITE", false, "FADE"},
		{"PAN", false, "FADE"},
		{"TILT", false, "FADE"},
		{"ZOOM", false, "FADE"},
		{"FOCUS", false, "FADE"},
		{"IRIS", false, "FADE"},
		{"EFFECT", false, "FADE"},
		{"OTHER", false, "FADE"},

		// Discrete channels should SNAP (regardless of type)
		{"INTENSITY", true, "SNAP"},
		{"PAN", true, "SNAP"},

		// Wheel/selector channels should SNAP
		{"GOBO", false, "SNAP"},
		{"COLOR_WHEEL", false, "SNAP"},
		{"MACRO", false, "SNAP"},
		{"STROBE", false, "SNAP"},
	}

	for _, tt := range tests {
		got := mapFadeBehavior(tt.channelType, tt.isDiscrete)
		if got != tt.want {
			t.Errorf("mapFadeBehavior(%q, %v) = %q, want %q", tt.channelType, tt.isDiscrete, got, tt.want)
		}
	}
}

func TestMapChannelType(t *testing.T) {
	tests := []struct {
		capability OFLCapability
		want       string
	}{
		{OFLCapability{Type: "Intensity"}, "INTENSITY"},
		{OFLCapability{Type: "ColorIntensity", Color: "Red"}, "RED"},
		{OFLCapability{Type: "ColorIntensity", Color: "green"}, "GREEN"},
		{OFLCapability{Type: "ColorIntensity", Color: "BLUE"}, "BLUE"},
		{OFLCapability{Type: "ColorIntensity", Color: "White"}, "WHITE"},
		{OFLCapability{Type: "ColorIntensity", Color: "Amber"}, "AMBER"},
		{OFLCapability{Type: "ColorIntensity", Color: "UV"}, "UV"},
		{OFLCapability{Type: "ColorIntensity", Color: "Cyan"}, "CYAN"},
		{OFLCapability{Type: "ColorIntensity", Color: "Magenta"}, "MAGENTA"},
		{OFLCapability{Type: "ColorIntensity", Color: "Yellow"}, "YELLOW"},
		{OFLCapability{Type: "ColorIntensity", Color: "Lime"}, "LIME"},
		{OFLCapability{Type: "ColorIntensity", Color: "Indigo"}, "INDIGO"},
		{OFLCapability{Type: "ColorIntensity", Color: "Cold White"}, "COLD_WHITE"},
		{OFLCapability{Type: "ColorIntensity", Color: "Warm White"}, "WARM_WHITE"},
		{OFLCapability{Type: "Pan"}, "PAN"},
		{OFLCapability{Type: "Tilt"}, "TILT"},
		{OFLCapability{Type: "Zoom"}, "ZOOM"},
		{OFLCapability{Type: "Focus"}, "FOCUS"},
		{OFLCapability{Type: "Iris"}, "IRIS"},
		{OFLCapability{Type: "Gobo"}, "GOBO"},
		{OFLCapability{Type: "WheelSlot"}, "GOBO"},
		{OFLCapability{Type: "ColorWheel"}, "COLOR_WHEEL"},
		{OFLCapability{Type: "ColorPreset"}, "COLOR_WHEEL"},
		{OFLCapability{Type: "Effect"}, "EFFECT"},
		{OFLCapability{Type: "EffectSpeed"}, "EFFECT"},
		{OFLCapability{Type: "Speed"}, "EFFECT"},
		{OFLCapability{Type: "Rotation"}, "EFFECT"},
		{OFLCapability{Type: "ShutterStrobe"}, "STROBE"},
		{OFLCapability{Type: "Maintenance"}, "MACRO"},
		{OFLCapability{Type: "NoFunction"}, "OTHER"},
		{OFLCapability{Type: "SomethingUnknown"}, "OTHER"},
	}

	for _, tt := range tests {
		got := mapChannelType(&tt.capability)
		if got != tt.want {
			t.Errorf("mapChannelType(%+v) = %q, want %q", tt.capability, got, tt.want)
		}
	}
}

func TestMapFixtureType(t *testing.T) {
	tests := []struct {
		categories []string
		want       string
	}{
		{[]string{"Moving Head"}, "MOVING_HEAD"},
		{[]string{"Scanner"}, "MOVING_HEAD"},
		{[]string{"Strobe"}, "STROBE"},
		{[]string{"Blinder"}, "STROBE"},
		{[]string{"Dimmer"}, "DIMMER"},
		{[]string{"Color Changer"}, "LED_PAR"},
		{[]string{"PAR"}, "LED_PAR"},
		{[]string{"Wash"}, "LED_PAR"},
		{[]string{"Unknown Category"}, "OTHER"},
		{[]string{"Par", "Color Changer"}, "LED_PAR"},
	}

	for _, tt := range tests {
		got := mapFixtureType(tt.categories)
		if got != tt.want {
			t.Errorf("mapFixtureType(%v) = %q, want %q", tt.categories, got, tt.want)
		}
	}
}

func containsString(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsString(s[1:], substr) || s[:len(substr)] == substr)
}
