package modulator

import (
	"math"
	"testing"
)

func TestCompositionOverride(t *testing.T) {
	tests := []struct {
		name        string
		existing    int
		effect      int
		hasExisting bool
		expected    int
	}{
		{"replace existing", 100, 200, true, 200},
		{"override to black", 255, 0, true, 0},
		{"override from black", 0, 128, true, 128},
		{"no existing value", 0, 150, false, 150},
		{"clamp to max", 100, 300, true, 255},
		{"clamp to min", 100, -50, true, 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ApplyComposition(tt.existing, tt.effect, ComposeModeOverride, tt.hasExisting)
			if result != tt.expected {
				t.Errorf("ApplyComposition(%d, %d, OVERRIDE, %v) = %d, expected %d",
					tt.existing, tt.effect, tt.hasExisting, result, tt.expected)
			}
		})
	}
}

func TestCompositionAdditive(t *testing.T) {
	tests := []struct {
		name        string
		existing    int
		effect      int
		hasExisting bool
		expected    int
	}{
		{"add to existing", 100, 50, true, 150},
		{"clamp at max", 200, 100, true, 255},
		{"add from zero", 0, 50, true, 50},
		{"no existing value", 0, 75, false, 75},
		{"add zero", 100, 0, true, 100},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ApplyComposition(tt.existing, tt.effect, ComposeModeAdditive, tt.hasExisting)
			if result != tt.expected {
				t.Errorf("ApplyComposition(%d, %d, ADDITIVE, %v) = %d, expected %d",
					tt.existing, tt.effect, tt.hasExisting, result, tt.expected)
			}
		})
	}
}

func TestCompositionMultiply(t *testing.T) {
	tests := []struct {
		name        string
		existing    int
		effect      int  // Treated as 0-255 = 0.0-1.0 multiplier
		hasExisting bool
		expected    int
		tolerance   int
	}{
		{"full multiply (255 = 1.0)", 200, 255, true, 200, 1},
		{"half multiply (127 ≈ 0.5)", 200, 127, true, 99, 2},
		{"zero multiply", 200, 0, true, 0, 0},
		{"quarter multiply", 255, 64, true, 64, 2},
		{"no existing value", 0, 128, false, 128, 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ApplyComposition(tt.existing, tt.effect, ComposeModeMultiply, tt.hasExisting)
			diff := result - tt.expected
			if diff < 0 {
				diff = -diff
			}
			if diff > tt.tolerance {
				t.Errorf("ApplyComposition(%d, %d, MULTIPLY, %v) = %d, expected %d (±%d)",
					tt.existing, tt.effect, tt.hasExisting, result, tt.expected, tt.tolerance)
			}
		})
	}
}

func TestCompositionNoExistingValue(t *testing.T) {
	// When no existing value, effect becomes the value regardless of mode
	modes := []CompositionMode{ComposeModeOverride, ComposeModeAdditive, ComposeModeMultiply}
	effectValue := 128

	for _, mode := range modes {
		result := ApplyComposition(0, effectValue, mode, false)
		if result != effectValue {
			t.Errorf("ApplyComposition with no existing (mode=%s) = %d, expected %d",
				mode, result, effectValue)
		}
	}
}

func TestCompositionUnknownMode(t *testing.T) {
	// Unknown mode should default to override
	result := ApplyComposition(100, 200, "UNKNOWN", true)
	if result != 200 {
		t.Errorf("Unknown mode should default to override, got %d", result)
	}
}

func TestApplyCompositionFloat(t *testing.T) {
	tests := []struct {
		name        string
		existing    float64
		effect      float64
		mode        CompositionMode
		hasExisting bool
		expected    float64
	}{
		{"override float", 100.5, 200.7, ComposeModeOverride, true, 200.7},
		{"additive float", 100.0, 50.5, ComposeModeAdditive, true, 150.5},
		{"multiply float", 200.0, 127.5, ComposeModeMultiply, true, 100.0},
		{"no existing", 0.0, 150.5, ComposeModeOverride, false, 150.5},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ApplyCompositionFloat(tt.existing, tt.effect, tt.mode, tt.hasExisting)
			if math.Abs(result-tt.expected) > 0.1 {
				t.Errorf("ApplyCompositionFloat(%f, %f, %s, %v) = %f, expected %f",
					tt.existing, tt.effect, tt.mode, tt.hasExisting, result, tt.expected)
			}
		})
	}
}

func TestCompositionModeString(t *testing.T) {
	tests := []struct {
		mode     CompositionMode
		expected string
	}{
		{ComposeModeOverride, "OVERRIDE"},
		{ComposeModeAdditive, "ADDITIVE"},
		{ComposeModeMultiply, "MULTIPLY"},
	}

	for _, tt := range tests {
		if tt.mode.String() != tt.expected {
			t.Errorf("CompositionMode.String() = %q, expected %q", tt.mode.String(), tt.expected)
		}
	}
}

func TestParseCompositionMode(t *testing.T) {
	tests := []struct {
		input    string
		expected CompositionMode
	}{
		{"OVERRIDE", ComposeModeOverride},
		{"ADDITIVE", ComposeModeAdditive},
		{"MULTIPLY", ComposeModeMultiply},
		{"unknown", ComposeModeOverride}, // Defaults to OVERRIDE
		{"", ComposeModeOverride},
	}

	for _, tt := range tests {
		result := ParseCompositionMode(tt.input)
		if result != tt.expected {
			t.Errorf("ParseCompositionMode(%q) = %v, expected %v", tt.input, result, tt.expected)
		}
	}
}

func TestCompositionModeIsValid(t *testing.T) {
	validModes := []CompositionMode{ComposeModeOverride, ComposeModeAdditive, ComposeModeMultiply}
	for _, mode := range validModes {
		if !mode.IsValid() {
			t.Errorf("CompositionMode %q should be valid", mode)
		}
	}

	invalidModes := []CompositionMode{"INVALID", "", "override"}
	for _, mode := range invalidModes {
		if mode.IsValid() {
			t.Errorf("CompositionMode %q should be invalid", mode)
		}
	}
}
