package fade

import (
	"fmt"
	"math"
	"sync"
	"time"

	"github.com/bbernstein/lacylights-go/internal/services/dmx"
)

// ChannelTarget represents a target value for a channel.
type ChannelTarget struct {
	Universe    int
	Channel     int
	TargetValue int
}

// SceneChannel represents a channel value in a scene.
type SceneChannel struct {
	Universe int
	Channel  int
	Value    int
}

// channelFade represents a fade operation on a single channel.
type channelFade struct {
	universe   int
	channel    int
	startValue float64
	endValue   float64
}

// activeFade represents an active fade operation.
type activeFade struct {
	id         string
	channels   []channelFade
	startTime  time.Time
	duration   time.Duration
	easingType EasingType
	onComplete func()
}

// Engine manages DMX channel fades with easing support.
type Engine struct {
	mu sync.RWMutex

	dmxService *dmx.Service
	activeFades map[string]*activeFade

	// Track interpolated values for smooth mid-fade transitions
	interpolatedValues map[string]float64 // key: "universe-channel"

	// Control
	stopChan chan struct{}
	running  bool

	// Configuration
	updateRate time.Duration // How often to update fades (default 25ms = 40Hz)
}

// NewEngine creates a new fade engine.
func NewEngine(dmxService *dmx.Service) *Engine {
	return &Engine{
		dmxService:         dmxService,
		activeFades:        make(map[string]*activeFade),
		interpolatedValues: make(map[string]float64),
		stopChan:           make(chan struct{}),
		updateRate:         25 * time.Millisecond, // 40Hz
	}
}

// Start starts the fade engine's update loop.
func (e *Engine) Start() {
	e.mu.Lock()
	if e.running {
		e.mu.Unlock()
		return
	}
	e.running = true
	e.mu.Unlock()

	go e.updateLoop()
}

// Stop stops the fade engine.
func (e *Engine) Stop() {
	e.mu.Lock()
	if !e.running {
		e.mu.Unlock()
		return
	}
	e.running = false
	close(e.stopChan)
	e.mu.Unlock()
}

// updateLoop runs the fade update loop.
func (e *Engine) updateLoop() {
	ticker := time.NewTicker(e.updateRate)
	defer ticker.Stop()

	for {
		select {
		case <-e.stopChan:
			return
		case <-ticker.C:
			e.processFades()
		}
	}
}

// processFades updates all active fades.
func (e *Engine) processFades() {
	e.mu.Lock()
	defer e.mu.Unlock()

	now := time.Now()
	var completedFades []string
	var callbacks []func()

	for id, fade := range e.activeFades {
		elapsed := now.Sub(fade.startTime)
		progress := float64(elapsed) / float64(fade.duration)

		if progress >= 1 {
			// Fade complete - set final values
			for _, ch := range fade.channels {
				channelKey := fmt.Sprintf("%d-%d", ch.universe, ch.channel)
				e.interpolatedValues[channelKey] = ch.endValue
				e.dmxService.SetChannelValue(ch.universe, ch.channel, byte(ch.endValue))
			}

			completedFades = append(completedFades, id)
			if fade.onComplete != nil {
				callbacks = append(callbacks, fade.onComplete)
			}
		} else {
			// Interpolate values
			for _, ch := range fade.channels {
				currentValue := Interpolate(ch.startValue, ch.endValue, progress, fade.easingType)
				roundedValue := math.Round(currentValue)
				clampedValue := clamp(int(roundedValue), 0, 255)

				channelKey := fmt.Sprintf("%d-%d", ch.universe, ch.channel)
				e.interpolatedValues[channelKey] = currentValue
				e.dmxService.SetChannelValue(ch.universe, ch.channel, byte(clampedValue))
			}
		}
	}

	// Remove completed fades
	for _, id := range completedFades {
		delete(e.activeFades, id)
	}

	// Execute callbacks outside of lock
	go func() {
		for _, cb := range callbacks {
			cb()
		}
	}()
}

// FadeChannels starts a fade operation on multiple channels.
func (e *Engine) FadeChannels(targets []ChannelTarget, duration time.Duration, fadeID string, easingType EasingType, onComplete func()) string {
	e.mu.Lock()
	defer e.mu.Unlock()

	if fadeID == "" {
		fadeID = fmt.Sprintf("fade-%d-%d", time.Now().UnixNano(), len(e.activeFades))
	}

	if easingType == "" {
		easingType = EasingInOutSine
	}

	// Build a set of channels that this new fade will control
	newChannelSet := make(map[string]bool)
	for _, target := range targets {
		channelKey := fmt.Sprintf("%d-%d", target.Universe, target.Channel)
		newChannelSet[channelKey] = true
	}

	// Cancel conflicting channels from ALL existing fades (not just same ID)
	// This prevents multiple fades from fighting over the same channels
	var fadesToRemove []string
	for existingID, existingFade := range e.activeFades {
		// Filter out channels that conflict with our new fade
		var remainingChannels []channelFade
		for _, ch := range existingFade.channels {
			channelKey := fmt.Sprintf("%d-%d", ch.universe, ch.channel)
			if !newChannelSet[channelKey] {
				// This channel is NOT being taken over, keep it
				remainingChannels = append(remainingChannels, ch)
			}
			// If the channel IS being taken over, we don't add it to remaining
			// but we keep the interpolated value for smooth transition
		}

		if len(remainingChannels) == 0 {
			// This fade has no channels left, mark it for removal
			fadesToRemove = append(fadesToRemove, existingID)
		} else if len(remainingChannels) < len(existingFade.channels) {
			// Some channels were removed, update the fade
			existingFade.channels = remainingChannels
		}
	}

	// Remove fades that have no channels left
	for _, id := range fadesToRemove {
		delete(e.activeFades, id)
	}

	// Build channel fades
	startTime := time.Now()
	var channels []channelFade

	for _, target := range targets {
		channelKey := fmt.Sprintf("%d-%d", target.Universe, target.Channel)

		// Get current value - prefer interpolated value if available
		var startValue float64
		if interpolated, ok := e.interpolatedValues[channelKey]; ok {
			startValue = interpolated
		} else {
			startValue = float64(e.dmxService.GetChannelValue(target.Universe, target.Channel))
		}

		channels = append(channels, channelFade{
			universe:   target.Universe,
			channel:    target.Channel,
			startValue: startValue,
			endValue:   float64(target.TargetValue),
		})
	}

	e.activeFades[fadeID] = &activeFade{
		id:         fadeID,
		channels:   channels,
		startTime:  startTime,
		duration:   duration,
		easingType: easingType,
		onComplete: onComplete,
	}

	return fadeID
}

// FadeToScene fades to a scene's channel values.
func (e *Engine) FadeToScene(sceneChannels []SceneChannel, fadeInTime time.Duration, fadeID string, easingType EasingType) string {
	targets := make([]ChannelTarget, len(sceneChannels))
	for i, ch := range sceneChannels {
		targets[i] = ChannelTarget{
			Universe:    ch.Universe,
			Channel:     ch.Channel,
			TargetValue: ch.Value,
		}
	}

	if easingType == "" {
		easingType = EasingInOutSine
	}

	return e.FadeChannels(targets, fadeInTime, fadeID, easingType, nil)
}

// FadeToBlack fades all active channels to zero.
func (e *Engine) FadeToBlack(fadeOutTime time.Duration, easingType EasingType) string {
	e.mu.RLock()
	allUniverses := e.dmxService.GetAllUniverses()
	e.mu.RUnlock()

	var targets []ChannelTarget
	for universe, channels := range allUniverses {
		for channel, value := range channels {
			if value > 0 {
				targets = append(targets, ChannelTarget{
					Universe:    universe,
					Channel:     channel + 1, // Convert to 1-indexed
					TargetValue: 0,
				})
			}
		}
	}

	if easingType == "" {
		easingType = EasingInOutSine
	}

	return e.FadeChannels(targets, fadeOutTime, "fade-to-black", easingType, nil)
}

// CancelFade cancels an active fade by ID.
func (e *Engine) CancelFade(fadeID string) {
	e.mu.Lock()
	defer e.mu.Unlock()

	if fade, ok := e.activeFades[fadeID]; ok {
		// Clean up interpolated values for this fade's channels
		for _, ch := range fade.channels {
			channelKey := fmt.Sprintf("%d-%d", ch.universe, ch.channel)
			delete(e.interpolatedValues, channelKey)
		}
		delete(e.activeFades, fadeID)
	}
}

// CancelAllFades cancels all active fades.
func (e *Engine) CancelAllFades() {
	e.mu.Lock()
	defer e.mu.Unlock()

	e.interpolatedValues = make(map[string]float64)
	e.activeFades = make(map[string]*activeFade)
}

// IsRunning returns whether the engine is running.
func (e *Engine) IsRunning() bool {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.running
}

// ActiveFadeCount returns the number of active fades.
func (e *Engine) ActiveFadeCount() int {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return len(e.activeFades)
}

// clamp clamps an integer to a range.
func clamp(value, min, max int) int {
	if value < min {
		return min
	}
	if value > max {
		return max
	}
	return value
}
