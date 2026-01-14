package modulator

import (
	"fmt"
	"time"
)

// LookChannel matches the existing fade engine's LookChannel struct for compatibility.
type LookChannel struct {
	Universe     int
	Channel      int
	Value        int    // Target DMX value (0-255)
	FadeBehavior string // FADE, SNAP, SNAP_END - defaults to FADE
}

// FadeBehavior constants for backward compatibility with fade engine.
const (
	FadeBehaviorFade    = "FADE"     // Interpolate smoothly between values (default)
	FadeBehaviorSnap    = "SNAP"     // Jump to target value at start of transition
	FadeBehaviorSnapEnd = "SNAP_END" // Jump to target value at end of transition
)

// FadeToLook creates a crossfade effect to the specified look.
// This provides backward compatibility with the existing fade engine API.
func (e *Engine) FadeToLook(channels []LookChannel, duration time.Duration, fadeID string, easingType EasingType) string {
	e.mu.Lock()
	defer e.mu.Unlock()

	if fadeID == "" {
		fadeID = fmt.Sprintf("fade-%d", time.Now().UnixNano())
	}

	if easingType == "" {
		easingType = DefaultEasing
	}

	// Handle instant fades (duration <= 0) synchronously
	if duration <= 0 {
		for _, ch := range channels {
			e.dmxService.SetChannelValue(ch.Universe, ch.Channel, byte(ch.Value))
		}
		e.dmxService.TriggerChangeDetection()
		return fadeID
	}

	// Capture current state as from values
	fromValues := e.captureCurrentState(channels)

	// Build target values, handling fade behaviors
	toValues := make(map[ChannelKey]int)
	snapChannels := make(map[ChannelKey]bool)
	snapEndChannels := make(map[ChannelKey]bool)

	for _, ch := range channels {
		key := ChannelKey{Universe: ch.Universe, Channel: ch.Channel}
		toValues[key] = ch.Value

		switch ch.FadeBehavior {
		case FadeBehaviorSnap:
			snapChannels[key] = true
		case FadeBehaviorSnapEnd:
			snapEndChannels[key] = true
		}
	}

	// Apply SNAP channels immediately
	for key := range snapChannels {
		e.dmxService.SetChannelValue(key.Universe, key.Channel, byte(toValues[key]))
		// Update from value so crossfade doesn't try to fade it
		fromValues[key] = toValues[key]
	}

	// Remove any existing crossfades
	e.removeEffectsByType(EffectTypeCrossfade)

	// Create crossfade effect
	effect := CreateCrossfadeEffect(fromValues, toValues, duration, easingType)
	effect.ID = fadeID

	active := &ActiveEffect{
		Effect:       effect,
		StartTime:    time.Now(),
		Intensity:    100,
		FromValues:   fromValues,
		ToValues:     toValues,
		Duration:     duration,
		cachedValues: make(map[ChannelKey]int),
	}

	// Store snap-end channels for processing at completion
	// We'll handle these in processEffect when the crossfade completes

	e.activeEffects = append(e.activeEffects, active)
	e.sortEffects()

	e.dmxService.ForceImmediateTransmission()

	return fadeID
}

// FadeToBlack creates a crossfade to all zeros.
func (e *Engine) FadeToBlack(duration time.Duration, easingType EasingType) string {
	e.mu.Lock()
	defer e.mu.Unlock()

	if easingType == "" {
		easingType = DefaultEasing
	}

	// Capture current state
	fromValues := e.captureAllChannelState()

	// Handle instant fade
	if duration <= 0 {
		for key := range fromValues {
			e.dmxService.SetChannelValue(key.Universe, key.Channel, 0)
		}
		e.dmxService.TriggerChangeDetection()
		return "fade-to-black"
	}

	// Target is all zeros
	toValues := make(map[ChannelKey]int)
	for key := range fromValues {
		toValues[key] = 0
	}

	// Remove any existing crossfades
	e.removeEffectsByType(EffectTypeCrossfade)

	effect := CreateCrossfadeEffect(fromValues, toValues, duration, easingType)
	effect.ID = "fade-to-black"

	active := &ActiveEffect{
		Effect:       effect,
		StartTime:    time.Now(),
		Intensity:    100,
		FromValues:   fromValues,
		ToValues:     toValues,
		Duration:     duration,
		cachedValues: make(map[ChannelKey]int),
	}

	e.activeEffects = append(e.activeEffects, active)
	e.sortEffects()

	e.dmxService.ForceImmediateTransmission()

	return "fade-to-black"
}

// CancelFade removes a specific crossfade by ID.
func (e *Engine) CancelFade(fadeID string) {
	e.mu.Lock()
	defer e.mu.Unlock()

	e.removeEffectByID(fadeID)
}

// CancelAllFades removes all crossfade effects.
func (e *Engine) CancelAllFades() {
	e.mu.Lock()
	defer e.mu.Unlock()

	e.removeEffectsByType(EffectTypeCrossfade)
}

// ActiveFadeCount returns the number of active crossfade effects.
func (e *Engine) ActiveFadeCount() int {
	e.mu.RLock()
	defer e.mu.RUnlock()

	count := 0
	for _, active := range e.activeEffects {
		if active.Effect.EffectType == EffectTypeCrossfade {
			count++
		}
	}
	return count
}

// captureCurrentState captures the current DMX state for specified channels.
func (e *Engine) captureCurrentState(channels []LookChannel) map[ChannelKey]int {
	values := make(map[ChannelKey]int)
	for _, ch := range channels {
		key := ChannelKey{Universe: ch.Universe, Channel: ch.Channel}
		values[key] = int(e.dmxService.GetChannelValue(ch.Universe, ch.Channel))
	}
	return values
}

// captureAllChannelState captures the current state of all non-zero channels.
func (e *Engine) captureAllChannelState() map[ChannelKey]int {
	values := make(map[ChannelKey]int)
	allUniverses := e.dmxService.GetAllUniverses()

	for universe, channels := range allUniverses {
		for channel, value := range channels {
			if value > 0 {
				key := ChannelKey{Universe: universe, Channel: channel + 1} // Convert to 1-indexed
				values[key] = int(value)
			}
		}
	}
	return values
}

// removeEffectsByType removes all effects of the specified type.
func (e *Engine) removeEffectsByType(effectType EffectType) {
	newEffects := make([]*ActiveEffect, 0, len(e.activeEffects))
	for _, active := range e.activeEffects {
		if active.Effect.EffectType != effectType {
			newEffects = append(newEffects, active)
		}
	}
	e.activeEffects = newEffects
}

// removeEffectByID removes an effect by its ID.
func (e *Engine) removeEffectByID(effectID string) {
	newEffects := make([]*ActiveEffect, 0, len(e.activeEffects))
	for _, active := range e.activeEffects {
		if active.Effect.ID != effectID {
			newEffects = append(newEffects, active)
		}
	}
	e.activeEffects = newEffects
}
