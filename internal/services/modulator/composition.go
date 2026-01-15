package modulator

// CompositionMode determines how an effect combines with lower-priority effects.
type CompositionMode string

const (
	// ComposeModeOverride means the effect completely replaces lower priority values.
	// This is the default mode for most effects including crossfades and blackout.
	ComposeModeOverride CompositionMode = "OVERRIDE"

	// ComposeModeAdditive means the effect adds its value to the existing value.
	// Useful for chase pulses and flashes layered on a base look.
	// Final value is clamped to 0-255.
	ComposeModeAdditive CompositionMode = "ADDITIVE"

	// ComposeModeMultiply means the effect scales the existing value.
	// The effect value is treated as a multiplier: 255 = 1.0, 0 = 0.0.
	// Used for grand master and dimmer curves.
	ComposeModeMultiply CompositionMode = "MULTIPLY"

	// ComposeModeModulate means the effect modulates the existing value.
	// The effect value is centered at 128: 128 = no change, 255 = +127, 0 = -128.
	// Perfect for LFO waveforms that oscillate around the underlying value.
	ComposeModeModulate CompositionMode = "MODULATE"
)

// String returns the string representation of the composition mode.
func (c CompositionMode) String() string {
	return string(c)
}

// ParseCompositionMode parses a string into a CompositionMode.
func ParseCompositionMode(s string) CompositionMode {
	switch s {
	case "OVERRIDE":
		return ComposeModeOverride
	case "ADDITIVE":
		return ComposeModeAdditive
	case "MULTIPLY":
		return ComposeModeMultiply
	case "MODULATE":
		return ComposeModeModulate
	default:
		return ComposeModeOverride
	}
}

// IsValid returns true if the composition mode is a known valid mode.
func (c CompositionMode) IsValid() bool {
	switch c {
	case ComposeModeOverride, ComposeModeAdditive, ComposeModeMultiply, ComposeModeModulate:
		return true
	default:
		return false
	}
}

// ApplyComposition applies the composition mode to combine an existing value
// with an effect value.
//
// Parameters:
//   - existing: the current channel value (from lower priority effects)
//   - effectValue: the value from this effect
//   - mode: how to combine the values
//   - hasExisting: true if there was a previous value for this channel
//
// Returns the composed value, clamped to 0-255.
func ApplyComposition(existing, effectValue int, mode CompositionMode, hasExisting bool) int {
	// If there's no existing value, the effect value becomes the value
	// regardless of composition mode
	if !hasExisting {
		return clamp(effectValue, 0, 255)
	}

	switch mode {
	case ComposeModeOverride:
		// Higher priority completely replaces lower priority
		return clamp(effectValue, 0, 255)

	case ComposeModeAdditive:
		// Add effect modulation to existing value
		// Negative values would subtract (if supported)
		return clamp(existing+effectValue, 0, 255)

	case ComposeModeMultiply:
		// Effect value is a multiplier: 255 = 1.0, 0 = 0.0
		multiplier := float64(effectValue) / 255.0
		return clamp(int(float64(existing)*multiplier), 0, 255)

	case ComposeModeModulate:
		// Effect value centered at 128: 128 = no change, 255 = +127, 0 = -128
		// This is ideal for LFO waveforms that modulate underlying values
		modulation := effectValue - 128
		return clamp(existing+modulation, 0, 255)

	default:
		// Unknown mode defaults to override
		return clamp(effectValue, 0, 255)
	}
}

// ApplyCompositionFloat is like ApplyComposition but works with float values.
// This is useful for intermediate calculations before final DMX output.
func ApplyCompositionFloat(existing, effectValue float64, mode CompositionMode, hasExisting bool) float64 {
	if !hasExisting {
		return clampFloat(effectValue, 0, 255)
	}

	switch mode {
	case ComposeModeOverride:
		return clampFloat(effectValue, 0, 255)

	case ComposeModeAdditive:
		return clampFloat(existing+effectValue, 0, 255)

	case ComposeModeMultiply:
		multiplier := effectValue / 255.0
		return clampFloat(existing*multiplier, 0, 255)

	case ComposeModeModulate:
		// Effect value centered at 128: 128 = no change, 255 = +127, 0 = -128
		modulation := effectValue - 128
		return clampFloat(existing+modulation, 0, 255)

	default:
		return clampFloat(effectValue, 0, 255)
	}
}
