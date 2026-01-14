package modulator

import (
	"math"
	"math/rand"
	"time"
)

// WaveformType represents the type of waveform for LFO effects.
type WaveformType string

const (
	// WaveformSine produces a smooth sine wave oscillation.
	WaveformSine WaveformType = "SINE"

	// WaveformCosine produces a cosine wave (90° phase-shifted sine).
	WaveformCosine WaveformType = "COSINE"

	// WaveformSquare produces a square wave (on/off).
	WaveformSquare WaveformType = "SQUARE"

	// WaveformSawtooth produces a sawtooth wave (linear ramp up, instant reset).
	WaveformSawtooth WaveformType = "SAWTOOTH"

	// WaveformTriangle produces a triangle wave (linear up, linear down).
	WaveformTriangle WaveformType = "TRIANGLE"

	// WaveformRandom produces random values (deterministic based on phase).
	WaveformRandom WaveformType = "RANDOM"
)

// String returns the string representation of the waveform type.
func (w WaveformType) String() string {
	return string(w)
}

// ParseWaveformType parses a string into a WaveformType.
func ParseWaveformType(s string) WaveformType {
	switch s {
	case "SINE":
		return WaveformSine
	case "COSINE":
		return WaveformCosine
	case "SQUARE":
		return WaveformSquare
	case "SAWTOOTH":
		return WaveformSawtooth
	case "TRIANGLE":
		return WaveformTriangle
	case "RANDOM":
		return WaveformRandom
	default:
		return WaveformSine
	}
}

// IsValid returns true if the waveform type is a known valid type.
func (w WaveformType) IsValid() bool {
	switch w {
	case WaveformSine, WaveformCosine, WaveformSquare, WaveformSawtooth, WaveformTriangle, WaveformRandom:
		return true
	default:
		return false
	}
}

// GenerateWaveform returns a value 0-1 for the given phase (0-360 degrees).
func GenerateWaveform(phase float64, waveform WaveformType) float64 {
	// Normalize phase to 0-360
	phase = math.Mod(phase, 360)
	if phase < 0 {
		phase += 360
	}

	radians := phase * math.Pi / 180

	switch waveform {
	case WaveformSine:
		// Sine: 0° = 0.5, 90° = 1.0, 180° = 0.5, 270° = 0.0
		return (math.Sin(radians) + 1) / 2

	case WaveformCosine:
		// Cosine: 0° = 1.0, 90° = 0.5, 180° = 0.0, 270° = 0.5
		return (math.Cos(radians) + 1) / 2

	case WaveformSquare:
		// Square: 0-180° = 1.0, 180-360° = 0.0
		if phase < 180 {
			return 1
		}
		return 0

	case WaveformSawtooth:
		// Sawtooth: Linear 0-1 over 0-360°
		return phase / 360

	case WaveformTriangle:
		// Triangle: Linear up 0-180°, linear down 180-360°
		if phase < 180 {
			return phase / 180
		}
		return 2 - (phase / 180)

	case WaveformRandom:
		// Random: Deterministic random based on phase
		// Use phase as seed for reproducible results
		return deterministicRandom(phase)

	default:
		return 0.5
	}
}

// deterministicRandom generates a deterministic random value based on phase.
// The same phase always produces the same value.
func deterministicRandom(phase float64) float64 {
	// Use phase as seed - discretize to steps for stable values
	step := int64(phase / 10) // Change value every 10 degrees
	r := rand.New(rand.NewSource(step))
	return r.Float64()
}

// CalculateWaveformValues returns modulated channel values for a waveform effect.
func CalculateWaveformValues(active *ActiveEffect, elapsed time.Duration) map[ChannelKey]int {
	if active.Effect.EffectType != EffectTypeWaveform {
		return nil
	}

	effect := active.Effect

	// Update phase based on frequency and elapsed time
	degreesPerSecond := effect.Frequency * 360
	active.Phase += degreesPerSecond * elapsed.Seconds()
	active.Phase = math.Mod(active.Phase, 360)
	if active.Phase < 0 {
		active.Phase += 360
	}

	values := make(map[ChannelKey]int)

	// Calculate value for each target channel
	for _, target := range effect.TargetChannels {
		// Calculate phase for this channel (including per-channel offset)
		channelPhase := active.Phase + effect.PhaseOffset
		if target.PhaseOffset != nil {
			channelPhase += *target.PhaseOffset
		}

		// Apply frequency scale to phase (higher scale = faster oscillation for this channel)
		if target.FrequencyScale != nil && *target.FrequencyScale != 1.0 {
			channelPhase *= *target.FrequencyScale
		}

		// Generate waveform value (0-1)
		waveValue := GenerateWaveform(channelPhase, effect.Waveform)

		// Get amplitude (scale if per-channel override)
		amplitude := effect.Amplitude
		if target.AmplitudeScale != nil {
			amplitude *= *target.AmplitudeScale
		}

		// Apply amplitude and offset
		// amplitude = modulation range (e.g., 50 means ±50% of 255)
		// offset = center point (e.g., 50% means center at 127)
		amplitudeRange := amplitude / 100.0 * 255.0
		offsetValue := effect.Offset / 100.0 * 255.0

		// Scale by intensity
		intensity := active.GetCurrentIntensity() / 100.0

		// Calculate final value
		// waveValue is 0-1, convert to -1 to +1, then scale by amplitude
		modulation := (waveValue - 0.5) * 2 * amplitudeRange * intensity
		finalValue := int(offsetValue + modulation)
		finalValue = clamp(finalValue, 0, 255)

		key := ChannelKey{Universe: target.Universe, Channel: target.Channel}
		values[key] = finalValue
	}

	return values
}

// MergeWaveformValues merges waveform values into an existing channel state map.
func MergeWaveformValues(
	active *ActiveEffect,
	elapsed time.Duration,
	existing map[ChannelKey]ChannelState,
) {
	values := CalculateWaveformValues(active, elapsed)
	if values == nil {
		return
	}

	for key, value := range values {
		existingState, hasExisting := existing[key]

		composedValue := ApplyComposition(
			existingState.Value,
			value,
			active.Effect.CompositionMode,
			hasExisting,
		)

		existing[key] = ChannelState{
			Value:           composedValue,
			OwnerEffectID:   active.Effect.ID,
			CompositionMode: active.Effect.CompositionMode,
		}
	}
}
