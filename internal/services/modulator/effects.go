package modulator

import (
	"time"

	"github.com/google/uuid"
)

// EffectType represents the type of effect, determining its calculation behavior.
type EffectType string

const (
	// EffectTypeWaveform is an LFO-based continuous modulation using waveforms.
	EffectTypeWaveform EffectType = "WAVEFORM"

	// EffectTypeCrossfade interpolates between channel states over time.
	EffectTypeCrossfade EffectType = "CROSSFADE"

	// EffectTypeStatic sets channels to fixed values without modulation.
	EffectTypeStatic EffectType = "STATIC"

	// EffectTypeMaster is a multiplier effect for intensity scaling (grand master).
	EffectTypeMaster EffectType = "MASTER"
)

// String returns the string representation of the effect type.
func (e EffectType) String() string {
	return string(e)
}

// ParseEffectType parses a string into an EffectType.
func ParseEffectType(s string) EffectType {
	switch s {
	case "WAVEFORM":
		return EffectTypeWaveform
	case "CROSSFADE":
		return EffectTypeCrossfade
	case "STATIC":
		return EffectTypeStatic
	case "MASTER":
		return EffectTypeMaster
	default:
		return EffectTypeWaveform
	}
}

// IsValid returns true if the effect type is a known valid type.
func (e EffectType) IsValid() bool {
	switch e {
	case EffectTypeWaveform, EffectTypeCrossfade, EffectTypeStatic, EffectTypeMaster:
		return true
	default:
		return false
	}
}

// EffectChannel represents channel-specific parameters for an effect.
type EffectChannel struct {
	Universe int
	Channel  int

	// Per-channel overrides
	PhaseOffset    *float64 // Phase offset in degrees (for waveforms)
	AmplitudeScale *float64 // Amplitude multiplier
	FrequencyScale *float64 // Frequency multiplier
}

// Effect represents an effect definition.
// Effects can be waveform-based (LFO), crossfades, static values, or master faders.
type Effect struct {
	ID          string
	Name        string
	Description string
	ProjectID   string

	// Type and behavior
	EffectType      EffectType
	Priority        EffectPriority
	CompositionMode CompositionMode
	OnCueChange     TransitionBehavior
	FadeDuration    *float64 // Override cue fadeInTime if set

	// Easing (for CROSSFADE type)
	EasingType EasingType

	// Waveform parameters (for WAVEFORM type)
	Waveform    WaveformType
	Frequency   float64 // Hz
	Amplitude   float64 // 0-100%
	Offset      float64 // 0-100% baseline
	PhaseOffset float64 // 0-360 degrees

	// For MASTER type
	MasterValue float64 // 0.0-1.0 multiplier

	// Channel targets
	TargetChannels []EffectChannel
}

// NewEffect creates a new effect with the specified parameters.
func NewEffect(effectType EffectType, priority EffectPriority) *Effect {
	return &Effect{
		ID:              uuid.New().String(),
		EffectType:      effectType,
		Priority:        priority,
		CompositionMode: ComposeModeOverride,
		OnCueChange:     TransitionFadeOut,
		EasingType:      DefaultEasing,
		Frequency:       1.0,
		Amplitude:       100.0,
		Offset:          50.0,
		MasterValue:     1.0,
	}
}

// Clone creates a deep copy of the effect.
func (e *Effect) Clone() *Effect {
	if e == nil {
		return nil
	}
	clone := *e
	if e.FadeDuration != nil {
		duration := *e.FadeDuration
		clone.FadeDuration = &duration
	}
	// Clone target channels
	if len(e.TargetChannels) > 0 {
		clone.TargetChannels = make([]EffectChannel, len(e.TargetChannels))
		for i, tc := range e.TargetChannels {
			clone.TargetChannels[i] = tc
			if tc.PhaseOffset != nil {
				v := *tc.PhaseOffset
				clone.TargetChannels[i].PhaseOffset = &v
			}
			if tc.AmplitudeScale != nil {
				v := *tc.AmplitudeScale
				clone.TargetChannels[i].AmplitudeScale = &v
			}
			if tc.FrequencyScale != nil {
				v := *tc.FrequencyScale
				clone.TargetChannels[i].FrequencyScale = &v
			}
		}
	}
	return &clone
}

// ActiveEffect represents a running effect instance.
type ActiveEffect struct {
	Effect    *Effect
	StartTime time.Time

	// Runtime state
	Intensity float64 // Current intensity (0-100)
	Phase     float64 // Current phase for waveforms (0-360)

	// For crossfades
	FromValues map[ChannelKey]int
	ToValues   map[ChannelKey]int
	Duration   time.Duration

	// Intensity fading
	IntensityFade *FadeTransition

	// Cached channel values from last calculation
	cachedValues map[ChannelKey]int
}

// NewActiveEffect creates a new active effect instance.
func NewActiveEffect(effect *Effect) *ActiveEffect {
	return &ActiveEffect{
		Effect:       effect,
		StartTime:    time.Now(),
		Intensity:    100,
		Phase:        effect.PhaseOffset,
		FromValues:   make(map[ChannelKey]int),
		ToValues:     make(map[ChannelKey]int),
		cachedValues: make(map[ChannelKey]int),
	}
}

// IsComplete returns true if the effect has finished and should be removed.
func (a *ActiveEffect) IsComplete() bool {
	// If intensity is fading to 0 and complete, effect is done
	if a.IntensityFade != nil && a.IntensityFade.IsComplete() && a.IntensityFade.EndValue == 0 {
		return true
	}

	// If intensity is 0 (after fade completed), effect is done
	if a.IntensityFade == nil && a.Intensity == 0 {
		return true
	}

	// Crossfade effects complete when their duration is done
	if a.Effect.EffectType == EffectTypeCrossfade && a.Duration > 0 {
		return time.Since(a.StartTime) >= a.Duration
	}

	return false
}

// GetProgress returns the progress of the effect (0-1).
// For crossfades, this is based on duration. For others, it's always 1.
func (a *ActiveEffect) GetProgress() float64 {
	if a.Effect.EffectType == EffectTypeCrossfade && a.Duration > 0 {
		progress := time.Since(a.StartTime).Seconds() / a.Duration.Seconds()
		return clampFloat(progress, 0, 1)
	}
	return 1
}

// GetCurrentIntensity returns the current intensity, accounting for any active fade.
func (a *ActiveEffect) GetCurrentIntensity() float64 {
	if a.IntensityFade != nil {
		return a.IntensityFade.CurrentValue()
	}
	return a.Intensity
}

// UpdateIntensityFromFade updates the intensity if there's an active fade.
func (a *ActiveEffect) UpdateIntensityFromFade() {
	if a.IntensityFade != nil {
		a.Intensity = a.IntensityFade.CurrentValue()
		if a.IntensityFade.IsComplete() {
			a.IntensityFade = nil
		}
	}
}

// FadeIntensityTo starts a fade of the effect's intensity.
func (a *ActiveEffect) FadeIntensityTo(target float64, duration time.Duration, easing EasingType) {
	a.IntensityFade = &FadeTransition{
		StartValue: a.Intensity,
		EndValue:   target,
		Duration:   duration,
		StartTime:  time.Now(),
		EasingType: easing,
	}
}

// SetCachedValues stores calculated channel values for later use.
func (a *ActiveEffect) SetCachedValues(values map[ChannelKey]int) {
	a.cachedValues = values
}

// GetCachedValues returns the cached channel values.
func (a *ActiveEffect) GetCachedValues() map[ChannelKey]int {
	return a.cachedValues
}
