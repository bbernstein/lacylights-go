// Package modulator provides a unified modulation engine for DMX channel control.
// It replaces the fade engine with a priority-based effect system where everything
// (crossfades, waveforms, static looks, system controls) is treated as an effect.
package modulator

import (
	"time"
)

// ChannelKey uniquely identifies a DMX channel across universes.
type ChannelKey struct {
	Universe int
	Channel  int
}

// ChannelState tracks the current value of a channel and its source.
type ChannelState struct {
	Value           int
	OwnerEffectID   string
	CompositionMode CompositionMode
}

// FadeTransition represents an ongoing intensity or parameter fade.
type FadeTransition struct {
	StartValue  float64
	EndValue    float64
	Duration    time.Duration
	StartTime   time.Time
	EasingType  EasingType
}

// CurrentValue calculates the current interpolated value of the fade.
func (f *FadeTransition) CurrentValue() float64 {
	if f.Duration <= 0 {
		return f.EndValue
	}

	elapsed := time.Since(f.StartTime)
	progress := elapsed.Seconds() / f.Duration.Seconds()

	// Clamp progress to 0-1
	if progress < 0 {
		progress = 0
	}
	if progress > 1 {
		progress = 1
	}

	easedProgress := ApplyEasing(progress, f.EasingType)
	return f.StartValue + (f.EndValue-f.StartValue)*easedProgress
}

// IsComplete returns true if the fade has finished.
func (f *FadeTransition) IsComplete() bool {
	if f.Duration <= 0 {
		return true
	}
	return time.Since(f.StartTime) >= f.Duration
}

// Progress returns the current progress of the fade (0-1).
func (f *FadeTransition) Progress() float64 {
	if f.Duration <= 0 {
		return 1
	}
	progress := time.Since(f.StartTime).Seconds() / f.Duration.Seconds()
	if progress < 0 {
		return 0
	}
	if progress > 1 {
		return 1
	}
	return progress
}

// clamp restricts a value to the range [min, max].
func clamp(value, min, max int) int {
	if value < min {
		return min
	}
	if value > max {
		return max
	}
	return value
}

// clampFloat restricts a float value to the range [min, max].
func clampFloat(value, min, max float64) float64 {
	if value < min {
		return min
	}
	if value > max {
		return max
	}
	return value
}

// lerp performs linear interpolation between two values.
func lerp(start, end, t float64) float64 {
	return start + (end-start)*t
}
