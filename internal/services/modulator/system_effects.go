package modulator

import (
	"time"
)

// System effect IDs are well-known constants for identification
const (
	// SystemBlackoutEffectID is the well-known ID for the blackout effect
	SystemBlackoutEffectID = "system-blackout"

	// SystemGrandMasterEffectID is the well-known ID for the grand master effect
	SystemGrandMasterEffectID = "system-grand-master"
)

// CreateBlackoutEffect creates a STATIC effect at SYSTEM priority that zeros all channels.
// When active at full intensity, it overrides all other effects and sets all channels to 0.
// The blackout effect uses PERSIST behavior so it doesn't fade out on cue changes.
func CreateBlackoutEffect() *Effect {
	effect := &Effect{
		ID:              SystemBlackoutEffectID,
		Name:            "System Blackout",
		Description:     "System-level blackout effect that zeros all channels",
		EffectType:      EffectTypeStatic,
		Priority:        PriorityBlackout,
		CompositionMode: ComposeModeOverride,
		OnCueChange:     TransitionPersist, // Blackout persists until explicitly released
		EasingType:      EasingLinear,
		TargetChannels:  []EffectChannel{}, // Empty means all channels
	}
	return effect
}

// CreateGrandMasterEffect creates a MASTER effect that scales all intensity channels.
// The value parameter controls the multiplier: 0.0 = all dark, 1.0 = full intensity.
// Grand master uses PERSIST behavior so it persists across cue changes.
func CreateGrandMasterEffect(value float64) *Effect {
	// Clamp value to valid range
	value = clampFloat(value, 0.0, 1.0)

	effect := &Effect{
		ID:              SystemGrandMasterEffectID,
		Name:            "Grand Master",
		Description:     "System-level grand master fader that scales all intensity channels",
		EffectType:      EffectTypeMaster,
		Priority:        PriorityGrandMaster,
		CompositionMode: ComposeModeMultiply,
		OnCueChange:     TransitionPersist, // Grand master persists across cue changes
		EasingType:      EasingLinear,
		MasterValue:     value,
		TargetChannels:  []EffectChannel{}, // Empty means all channels
	}
	return effect
}

// ActivateBlackout enables the system blackout effect with an optional fade time.
// If fadeTime is 0, the blackout is immediate. Otherwise, it fades in over the specified duration.
func (e *Engine) ActivateBlackout(fadeTime float64) {
	e.mu.Lock()
	defer e.mu.Unlock()

	// Check if blackout is already active
	for _, active := range e.activeEffects {
		if active.Effect.ID == SystemBlackoutEffectID {
			// Already active, just ensure it's fading to full intensity
			if fadeTime > 0 {
				active.FadeIntensityTo(100, time.Duration(fadeTime*float64(time.Second)), EasingLinear)
			} else {
				active.Intensity = 100
				active.IntensityFade = nil
			}
			return
		}
	}

	// Create and add the blackout effect
	effect := CreateBlackoutEffect()
	active := NewActiveEffect(effect)

	if fadeTime > 0 {
		// Start at 0 intensity and fade to 100
		active.Intensity = 0
		active.FadeIntensityTo(100, time.Duration(fadeTime*float64(time.Second)), EasingLinear)
	} else {
		// Immediate blackout
		active.Intensity = 100
	}

	e.activeEffects = append(e.activeEffects, active)
	e.sortEffects()
}

// ReleaseBlackout disables the system blackout effect with an optional fade time.
// If fadeTime is 0, the blackout is released immediately. Otherwise, it fades out over the specified duration.
func (e *Engine) ReleaseBlackout(fadeTime float64) {
	e.mu.Lock()
	defer e.mu.Unlock()

	// Find the blackout effect
	for _, active := range e.activeEffects {
		if active.Effect.ID == SystemBlackoutEffectID {
			if fadeTime > 0 {
				// Fade intensity to 0, effect will be removed when complete
				active.FadeIntensityTo(0, time.Duration(fadeTime*float64(time.Second)), EasingLinear)
			} else {
				// Immediate release - set intensity to 0 so it will be cleaned up
				active.Intensity = 0
				active.IntensityFade = nil
			}
			return
		}
	}
	// Not found - nothing to do
}

// SetGrandMaster sets the grand master level (0.0-1.0).
// If the grand master effect doesn't exist, it creates one.
// The change is immediate (no fade).
func (e *Engine) SetGrandMaster(value float64) {
	e.mu.Lock()
	defer e.mu.Unlock()

	// Clamp value to valid range
	value = clampFloat(value, 0.0, 1.0)

	// Find existing grand master effect
	for _, active := range e.activeEffects {
		if active.Effect.ID == SystemGrandMasterEffectID {
			// Update the master value
			active.Effect.MasterValue = value
			return
		}
	}

	// Create and add the grand master effect
	effect := CreateGrandMasterEffect(value)
	active := NewActiveEffect(effect)
	active.Intensity = 100 // Grand master is always at full effect intensity

	e.activeEffects = append(e.activeEffects, active)
	e.sortEffects()
}

// GetGrandMasterValue returns the current grand master level (0.0-1.0).
// Returns 1.0 if no grand master effect is active.
func (e *Engine) GetGrandMasterValue() float64 {
	e.mu.RLock()
	defer e.mu.RUnlock()

	// Find the grand master effect
	for _, active := range e.activeEffects {
		if active.Effect.ID == SystemGrandMasterEffectID {
			return active.Effect.MasterValue
		}
	}

	// No grand master effect - return default (full intensity)
	return 1.0
}

// IsBlackoutActive returns true if the blackout effect is currently active.
// An active blackout is one that exists and has non-zero intensity (or is fading to non-zero).
func (e *Engine) IsBlackoutActive() bool {
	e.mu.RLock()
	defer e.mu.RUnlock()

	for _, active := range e.activeEffects {
		if active.Effect.ID == SystemBlackoutEffectID {
			// Consider it active if intensity > 0 or if it's fading to non-zero
			if active.Intensity > 0 {
				return true
			}
			if active.IntensityFade != nil && active.IntensityFade.EndValue > 0 {
				return true
			}
		}
	}
	return false
}

// GetBlackoutIntensity returns the current blackout intensity (0-100).
// Returns 0 if blackout is not active.
func (e *Engine) GetBlackoutIntensity() float64 {
	e.mu.RLock()
	defer e.mu.RUnlock()

	for _, active := range e.activeEffects {
		if active.Effect.ID == SystemBlackoutEffectID {
			return active.GetCurrentIntensity()
		}
	}
	return 0
}

// ClearSystemEffects removes all system-level effects (blackout and grand master).
// This is useful for resetting the system to a clean state.
func (e *Engine) ClearSystemEffects() {
	e.mu.Lock()
	defer e.mu.Unlock()

	newEffects := make([]*ActiveEffect, 0, len(e.activeEffects))
	for _, active := range e.activeEffects {
		if active.Effect.Priority.Band != PriorityBandSystem {
			newEffects = append(newEffects, active)
		}
	}
	e.activeEffects = newEffects
}

// HasSystemEffect returns true if a system effect with the given ID is active.
func (e *Engine) HasSystemEffect(effectID string) bool {
	e.mu.RLock()
	defer e.mu.RUnlock()

	for _, active := range e.activeEffects {
		if active.Effect.ID == effectID {
			return true
		}
	}
	return false
}
