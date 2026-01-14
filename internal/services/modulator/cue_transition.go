package modulator

import (
	"time"
)

// CueTransitionRequest encapsulates parameters for a cue transition.
type CueTransitionRequest struct {
	FromCueID    *string           // Current cue (nil if starting fresh)
	ToCueID      *string           // Target cue (nil for fade to black)
	ToLookValues map[ChannelKey]int // Pre-computed target values
	FadeTime     float64           // Transition duration in seconds
	EasingType   EasingType
}

// ExecuteCueTransition executes a cue transition with proper effect lifecycle management.
// It handles transition behaviors for existing effects based on their OnCueChange settings.
func (e *Engine) ExecuteCueTransition(req CueTransitionRequest) error {
	e.mu.Lock()
	defer e.mu.Unlock()

	// Validate and set defaults
	if req.EasingType == "" {
		req.EasingType = DefaultEasing
	}

	fadeTime := time.Duration(req.FadeTime * float64(time.Second))

	// Step 1: Capture current DMX state before any changes
	fromValues := e.captureAllChannelState()

	// Step 2: Apply transition behaviors to existing USER band effects
	e.applyTransitionBehaviorsLocked(fadeTime, req.EasingType)

	// Step 3: Remove any existing crossfade effects (they get replaced by new one)
	e.removeEffectsByType(EffectTypeCrossfade)

	// Step 4: Determine target values
	toValues := req.ToLookValues
	if toValues == nil {
		toValues = make(map[ChannelKey]int)
	}

	// If transitioning to black (no ToCueID and empty values), fade all channels to 0
	if req.ToCueID == nil && len(toValues) == 0 {
		for key := range fromValues {
			toValues[key] = 0
		}
	}

	// Step 5: Create crossfade effect if there's a fade time
	if fadeTime > 0 {
		effect := CreateCrossfadeEffect(fromValues, toValues, fadeTime, req.EasingType)

		active := &ActiveEffect{
			Effect:       effect,
			StartTime:    time.Now(),
			Intensity:    100,
			FromValues:   fromValues,
			ToValues:     toValues,
			Duration:     fadeTime,
			cachedValues: make(map[ChannelKey]int),
		}

		e.activeEffects = append(e.activeEffects, active)
		e.sortEffects()
	} else {
		// Instant transition - apply values directly
		for key, value := range toValues {
			e.dmxService.SetChannelValue(key.Universe, key.Channel, byte(value))
		}
		// Fade to black - clear channels not in toValues
		for key := range fromValues {
			if _, inTo := toValues[key]; !inTo {
				e.dmxService.SetChannelValue(key.Universe, key.Channel, 0)
			}
		}
		e.dmxService.TriggerChangeDetection()
	}

	// Force immediate transmission to start the transition
	if fadeTime > 0 {
		e.dmxService.ForceImmediateTransmission()
	}

	return nil
}

// ApplyTransitionBehaviors applies OnCueChange behaviors to all active effects.
// This is the public version that acquires the lock.
func (e *Engine) ApplyTransitionBehaviors(fadeTime float64, easing EasingType) {
	e.mu.Lock()
	defer e.mu.Unlock()

	duration := time.Duration(fadeTime * float64(time.Second))
	e.applyTransitionBehaviorsLocked(duration, easing)
}

// applyTransitionBehaviorsLocked applies transition behaviors without acquiring lock.
// This processes effects based on their OnCueChange setting.
func (e *Engine) applyTransitionBehaviorsLocked(fadeTime time.Duration, easing EasingType) {
	// Track effects to remove immediately
	effectsToRemove := make(map[string]bool)

	for _, active := range e.activeEffects {
		// Only process USER band effects during cue transitions
		// CUE band effects (crossfades) are handled separately
		// SYSTEM band effects (blackout, grand master) persist through cue changes
		if active.Effect.Priority.Band != PriorityBandUser {
			continue
		}

		switch active.Effect.OnCueChange {
		case TransitionFadeOut:
			// Fade intensity to 0 over fadeTime (or effect's FadeDuration)
			duration := fadeTime
			if active.Effect.FadeDuration != nil {
				duration = time.Duration(*active.Effect.FadeDuration * float64(time.Second))
			}
			if duration > 0 {
				active.FadeIntensityTo(0, duration, easing)
			} else {
				// Instant fade out
				active.Intensity = 0
			}

		case TransitionPersist:
			// Do nothing - effect continues unchanged

		case TransitionSnapOff:
			// Immediately set intensity to 0 and mark for cleanup
			active.Intensity = 0
			effectsToRemove[active.Effect.ID] = true

		case TransitionCrossfadeParams:
			// This would require new parameter values from the incoming cue
			// For now, we just start a parameter fade to current values (no change)
			// Full implementation would require passing new effect parameters
			// For the basic case, we treat it like persist
		}
	}

	// Remove effects marked for immediate removal
	if len(effectsToRemove) > 0 {
		newEffects := make([]*ActiveEffect, 0, len(e.activeEffects))
		for _, active := range e.activeEffects {
			if !effectsToRemove[active.Effect.ID] {
				newEffects = append(newEffects, active)
			}
		}
		e.activeEffects = newEffects
	}
}

// GetEffectsByBand returns all active effects in a specific priority band.
func (e *Engine) GetEffectsByBand(band PriorityBand) []*ActiveEffect {
	e.mu.RLock()
	defer e.mu.RUnlock()

	result := make([]*ActiveEffect, 0)
	for _, active := range e.activeEffects {
		if active.Effect.Priority.Band == band {
			result = append(result, active)
		}
	}
	return result
}

// GetEffectsByTransitionBehavior returns all active effects with a specific transition behavior.
func (e *Engine) GetEffectsByTransitionBehavior(behavior TransitionBehavior) []*ActiveEffect {
	e.mu.RLock()
	defer e.mu.RUnlock()

	result := make([]*ActiveEffect, 0)
	for _, active := range e.activeEffects {
		if active.Effect.OnCueChange == behavior {
			result = append(result, active)
		}
	}
	return result
}

// CleanupCompletedEffects removes effects that have reached 0 intensity or completed.
// This is automatically called in processModulation but can be called manually.
func (e *Engine) CleanupCompletedEffects() int {
	e.mu.Lock()
	defer e.mu.Unlock()

	originalCount := len(e.activeEffects)
	newEffects := make([]*ActiveEffect, 0, len(e.activeEffects))

	for _, active := range e.activeEffects {
		if !active.IsComplete() {
			newEffects = append(newEffects, active)
		}
	}

	e.activeEffects = newEffects
	return originalCount - len(e.activeEffects)
}

// CrossfadeEffectParams starts a parameter crossfade for an existing effect.
// This implements the CROSSFADE_PARAMS transition behavior.
func (e *Engine) CrossfadeEffectParams(effectID string, newParams EffectParamUpdate, duration time.Duration, easing EasingType) bool {
	e.mu.Lock()
	defer e.mu.Unlock()

	// Find the effect
	var target *ActiveEffect
	for _, active := range e.activeEffects {
		if active.Effect.ID == effectID {
			target = active
			break
		}
	}

	if target == nil {
		return false
	}

	// Apply parameter updates based on what's provided
	if newParams.Intensity != nil {
		target.FadeIntensityTo(*newParams.Intensity, duration, easing)
	}

	// For waveform effects, we could also crossfade frequency, amplitude, etc.
	// This would require additional FadeTransition fields on ActiveEffect
	// For now, we just handle intensity fading

	return true
}

// EffectParamUpdate represents parameter updates for CROSSFADE_PARAMS behavior.
type EffectParamUpdate struct {
	Intensity *float64 // Target intensity (0-100)
	Frequency *float64 // Target frequency for waveforms
	Amplitude *float64 // Target amplitude for waveforms
	Offset    *float64 // Target offset for waveforms
}

// HasActiveTransition returns true if there's an active cue transition (crossfade) in progress.
func (e *Engine) HasActiveTransition() bool {
	e.mu.RLock()
	defer e.mu.RUnlock()

	for _, active := range e.activeEffects {
		if active.Effect.EffectType == EffectTypeCrossfade && !active.IsComplete() {
			return true
		}
	}
	return false
}

// GetTransitionProgress returns the progress (0-1) of the current cue transition.
// Returns 1.0 if no transition is active.
func (e *Engine) GetTransitionProgress() float64 {
	e.mu.RLock()
	defer e.mu.RUnlock()

	for _, active := range e.activeEffects {
		if active.Effect.EffectType == EffectTypeCrossfade {
			return active.GetProgress()
		}
	}
	return 1.0
}
