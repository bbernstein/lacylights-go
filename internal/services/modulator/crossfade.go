package modulator

import (
	"time"

	"github.com/google/uuid"
)

// CreateCrossfadeEffect creates a crossfade effect from current state to target.
func CreateCrossfadeEffect(
	fromValues map[ChannelKey]int,
	toValues map[ChannelKey]int,
	duration time.Duration,
	easingType EasingType,
) *Effect {
	if easingType == "" {
		easingType = DefaultEasing
	}

	return &Effect{
		ID:              uuid.New().String(),
		Name:            "Crossfade",
		EffectType:      EffectTypeCrossfade,
		Priority:        PriorityCueDefault,
		CompositionMode: ComposeModeOverride,
		OnCueChange:     TransitionSnapOff, // Crossfades are replaced by new crossfades
		EasingType:      easingType,
	}
}

// CreateCrossfadeActive creates an active crossfade effect with the given values.
func CreateCrossfadeActive(
	fromValues map[ChannelKey]int,
	toValues map[ChannelKey]int,
	duration time.Duration,
	easingType EasingType,
) *ActiveEffect {
	effect := CreateCrossfadeEffect(fromValues, toValues, duration, easingType)

	return &ActiveEffect{
		Effect:       effect,
		StartTime:    time.Now(),
		Intensity:    100,
		FromValues:   fromValues,
		ToValues:     toValues,
		Duration:     duration,
		cachedValues: make(map[ChannelKey]int),
	}
}

// CalculateCrossfadeValues returns current interpolated values for a crossfade effect.
func CalculateCrossfadeValues(active *ActiveEffect) map[ChannelKey]int {
	if active.Effect.EffectType != EffectTypeCrossfade {
		return nil
	}

	// Calculate progress
	progress := active.GetProgress()

	// Apply easing to progress
	easedProgress := ApplyEasing(progress, active.Effect.EasingType)

	values := make(map[ChannelKey]int)

	// Interpolate each channel in the target values
	for key, toValue := range active.ToValues {
		fromValue := 0
		if v, ok := active.FromValues[key]; ok {
			fromValue = v
		}
		interpolated := Interpolate(float64(fromValue), float64(toValue), easedProgress, EasingLinear)
		values[key] = int(interpolated)
	}

	// Handle channels in from but not in to (fade to 0)
	for key, fromValue := range active.FromValues {
		if _, inTo := active.ToValues[key]; !inTo {
			interpolated := Interpolate(float64(fromValue), 0, easedProgress, EasingLinear)
			values[key] = int(interpolated)
		}
	}

	// Apply intensity scaling
	intensity := active.GetCurrentIntensity() / 100.0
	if intensity < 1.0 {
		for key, value := range values {
			values[key] = int(float64(value) * intensity)
		}
	}

	return values
}

// IsCrossfadeComplete returns true if the crossfade has finished.
func IsCrossfadeComplete(active *ActiveEffect) bool {
	if active.Effect.EffectType != EffectTypeCrossfade {
		return false
	}
	return time.Since(active.StartTime) >= active.Duration
}

// GetCrossfadeRemainingTime returns the remaining time for a crossfade.
func GetCrossfadeRemainingTime(active *ActiveEffect) time.Duration {
	if active.Effect.EffectType != EffectTypeCrossfade {
		return 0
	}
	elapsed := time.Since(active.StartTime)
	if elapsed >= active.Duration {
		return 0
	}
	return active.Duration - elapsed
}

// MergeCrossfadeValues merges crossfade values into an existing channel state map.
// Uses the effect's composition mode to determine how values combine.
func MergeCrossfadeValues(
	active *ActiveEffect,
	existing map[ChannelKey]ChannelState,
) {
	values := CalculateCrossfadeValues(active)
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
