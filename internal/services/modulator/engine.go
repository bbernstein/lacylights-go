package modulator

import (
	"sync"
	"time"

	"github.com/bbernstein/lacylights-go/internal/services/dmx"
)

// DefaultUpdateRateHz is the default update rate for the modulation engine.
const DefaultUpdateRateHz = 60

// Engine is the unified modulator engine that processes all DMX modulation.
// It replaces the fade engine with a priority-based effect system.
type Engine struct {
	mu sync.RWMutex

	dmxService *dmx.Service

	// All active effects, sorted by priority (lowest first)
	activeEffects []*ActiveEffect

	// Control
	stopChan chan struct{}
	doneChan chan struct{}
	running  bool

	// Configuration
	updateRate time.Duration // How often to update (default ~16.67ms = 60Hz)

	// Timing
	lastUpdate time.Time

	// Callbacks
	onEffectComplete func(effectID string)
	onStateChange    func()
}

// NewEngine creates a new modulator engine with the specified update rate.
// If updateRateHz is <= 0, it defaults to 60Hz.
func NewEngine(dmxService *dmx.Service, updateRateHz int) *Engine {
	if updateRateHz <= 0 {
		updateRateHz = DefaultUpdateRateHz
	}
	updateRate := time.Second / time.Duration(updateRateHz)

	return &Engine{
		dmxService:    dmxService,
		activeEffects: make([]*ActiveEffect, 0),
		updateRate:    updateRate,
	}
}

// SetOnEffectComplete sets a callback that is called when an effect completes.
func (e *Engine) SetOnEffectComplete(callback func(effectID string)) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.onEffectComplete = callback
}

// SetOnStateChange sets a callback that is called when the modulation state changes.
func (e *Engine) SetOnStateChange(callback func()) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.onStateChange = callback
}

// IsRunning returns true if the engine's update loop is running.
func (e *Engine) IsRunning() bool {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.running
}

// GetUpdateRateHz returns the current update rate in Hz.
func (e *Engine) GetUpdateRateHz() int {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return int(time.Second / e.updateRate)
}

// SetUpdateRate changes the update rate.
// If the engine is running, it will be stopped and restarted.
func (e *Engine) SetUpdateRate(rateHz int) error {
	if rateHz <= 0 {
		rateHz = DefaultUpdateRateHz
	}

	e.mu.Lock()
	wasRunning := e.running
	e.mu.Unlock()

	if wasRunning {
		_ = e.Stop() // Ignoring error as we're about to restart
	}

	e.mu.Lock()
	e.updateRate = time.Second / time.Duration(rateHz)
	e.mu.Unlock()

	if wasRunning {
		return e.Start()
	}
	return nil
}

// GetActiveEffectCount returns the number of active effects.
func (e *Engine) GetActiveEffectCount() int {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return len(e.activeEffects)
}

// Start starts the engine's update loop.
// Returns an error if the engine is already running.
func (e *Engine) Start() error {
	e.mu.Lock()
	if e.running {
		e.mu.Unlock()
		return nil // Already running, not an error
	}
	e.running = true
	e.stopChan = make(chan struct{})
	e.doneChan = make(chan struct{})
	e.lastUpdate = time.Now()
	e.mu.Unlock()

	go e.updateLoop()
	return nil
}

// Stop stops the engine's update loop.
// Blocks until the update loop has exited.
func (e *Engine) Stop() error {
	e.mu.Lock()
	if !e.running {
		e.mu.Unlock()
		return nil
	}
	e.running = false
	close(e.stopChan)
	doneChan := e.doneChan
	e.mu.Unlock()

	// Wait for the update loop to exit
	<-doneChan
	return nil
}

// updateLoop runs the main modulation update loop.
// This is the core 60Hz loop that processes all effects.
func (e *Engine) updateLoop() {
	ticker := time.NewTicker(e.updateRate)
	defer ticker.Stop()

	for {
		select {
		case <-e.stopChan:
			close(e.doneChan)
			return
		case <-ticker.C:
			e.processModulation()
		}
	}
}

// processModulation processes all active effects and updates DMX channels.
// This is called once per update tick (60Hz by default).
func (e *Engine) processModulation() {
	e.mu.Lock()
	defer e.mu.Unlock()

	// Skip if no DMX service
	if e.dmxService == nil {
		return
	}

	now := time.Now()
	elapsed := now.Sub(e.lastUpdate)
	e.lastUpdate = now

	// Track if any effects were cleaned up for callback
	var completedEffects []string

	// Initialize channel values
	channelValues := make(map[ChannelKey]ChannelState)

	// Process effects in priority order (lowest to highest)
	// Effects are already sorted by priority
	for _, active := range e.activeEffects {
		e.processEffect(active, elapsed, channelValues)
	}

	// Write final values to DMX
	for key, state := range channelValues {
		e.dmxService.SetChannelValue(key.Universe, key.Channel, byte(state.Value))
	}

	// Trigger DMX transmission if any channels were updated
	if len(channelValues) > 0 {
		e.dmxService.TriggerChangeDetection()
	}

	// Clean up completed effects
	newActiveEffects := make([]*ActiveEffect, 0, len(e.activeEffects))
	for _, active := range e.activeEffects {
		if active.IsComplete() {
			completedEffects = append(completedEffects, active.Effect.ID)
		} else {
			newActiveEffects = append(newActiveEffects, active)
		}
	}
	e.activeEffects = newActiveEffects

	// Fire callbacks for completed effects
	if len(completedEffects) > 0 && e.onEffectComplete != nil {
		for _, effectID := range completedEffects {
			go e.onEffectComplete(effectID)
		}
	}

	// Fire state change callback if any effects were processed
	if len(channelValues) > 0 && e.onStateChange != nil {
		go e.onStateChange()
	}
}

// processEffect calculates the effect's contribution to channel values.
func (e *Engine) processEffect(active *ActiveEffect, elapsed time.Duration, channels map[ChannelKey]ChannelState) {
	// Update intensity if fading
	active.UpdateIntensityFromFade()

	// Skip if intensity is zero
	if active.Intensity == 0 {
		return
	}

	// Calculate effect values based on type
	switch active.Effect.EffectType {
	case EffectTypeCrossfade:
		MergeCrossfadeValues(active, channels)

	case EffectTypeWaveform:
		MergeWaveformValues(active, elapsed, channels)

	case EffectTypeStatic:
		e.processStaticEffect(active, channels)

	case EffectTypeMaster:
		e.processMasterEffect(active, channels)
	}
}

// processStaticEffect processes a static value effect.
func (e *Engine) processStaticEffect(active *ActiveEffect, channels map[ChannelKey]ChannelState) {
	// Static effects set fixed values for their target channels
	intensity := active.GetCurrentIntensity() / 100.0

	for _, target := range active.Effect.TargetChannels {
		key := ChannelKey{Universe: target.Universe, Channel: target.Channel}

		// For static effects, we'd need a target value stored somewhere
		// For now, handle special cases like blackout
		if active.Effect.ID == "system-blackout" {
			// Blackout sets all active channels to 0
			existingState, hasExisting := channels[key]
			if hasExisting {
				// Scale existing value toward 0 based on intensity
				newValue := int(float64(existingState.Value) * (1 - intensity))
				channels[key] = ChannelState{
					Value:           newValue,
					OwnerEffectID:   active.Effect.ID,
					CompositionMode: ComposeModeOverride,
				}
			}
		}
	}

	// Special handling for blackout - affects all channels
	if active.Effect.ID == "system-blackout" {
		for key, state := range channels {
			newValue := int(float64(state.Value) * (1 - intensity))
			channels[key] = ChannelState{
				Value:           newValue,
				OwnerEffectID:   active.Effect.ID,
				CompositionMode: ComposeModeOverride,
			}
		}
	}
}

// processMasterEffect processes a master fader effect (e.g., grand master).
func (e *Engine) processMasterEffect(active *ActiveEffect, channels map[ChannelKey]ChannelState) {
	// Master effects multiply existing values
	masterValue := active.Effect.MasterValue
	intensity := active.GetCurrentIntensity() / 100.0

	// Effective multiplier is interpolated based on intensity
	// At 0% intensity, multiplier is 1.0 (no effect)
	// At 100% intensity, multiplier is masterValue
	effectiveMultiplier := 1.0 + (masterValue-1.0)*intensity

	for key, state := range channels {
		newValue := int(float64(state.Value) * effectiveMultiplier)
		newValue = clamp(newValue, 0, 255)
		channels[key] = ChannelState{
			Value:           newValue,
			OwnerEffectID:   active.Effect.ID,
			CompositionMode: ComposeModeMultiply,
		}
	}
}

// sortEffects sorts the active effects by priority (lowest first).
func (e *Engine) sortEffects() {
	// Simple insertion sort - effects list is usually small
	for i := 1; i < len(e.activeEffects); i++ {
		for j := i; j > 0; j-- {
			if e.activeEffects[j].Effect.Priority.Less(e.activeEffects[j-1].Effect.Priority) {
				e.activeEffects[j], e.activeEffects[j-1] = e.activeEffects[j-1], e.activeEffects[j]
			} else {
				break
			}
		}
	}
}

// AddEffect adds an effect to the engine.
func (e *Engine) AddEffect(effect *Effect) *ActiveEffect {
	e.mu.Lock()
	defer e.mu.Unlock()

	active := NewActiveEffect(effect)
	e.activeEffects = append(e.activeEffects, active)
	e.sortEffects()

	return active
}

// RemoveEffect removes an effect from the engine by ID.
func (e *Engine) RemoveEffect(effectID string) {
	e.mu.Lock()
	defer e.mu.Unlock()

	e.removeEffectByID(effectID)
}

// GetActiveEffects returns a copy of all active effects.
func (e *Engine) GetActiveEffects() []*ActiveEffect {
	e.mu.RLock()
	defer e.mu.RUnlock()

	result := make([]*ActiveEffect, len(e.activeEffects))
	copy(result, e.activeEffects)
	return result
}

// ClearAllEffects removes all active effects.
func (e *Engine) ClearAllEffects() {
	e.mu.Lock()
	defer e.mu.Unlock()

	e.activeEffects = make([]*ActiveEffect, 0)
}
