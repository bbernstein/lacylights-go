package modulator

import (
	"math"
	"testing"
	"time"
)

// --- EffectType Tests ---

func TestParseEffectType(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected EffectType
	}{
		{"parse WAVEFORM", "WAVEFORM", EffectTypeWaveform},
		{"parse CROSSFADE", "CROSSFADE", EffectTypeCrossfade},
		{"parse STATIC", "STATIC", EffectTypeStatic},
		{"parse MASTER", "MASTER", EffectTypeMaster},
		{"parse unknown defaults to WAVEFORM", "UNKNOWN", EffectTypeWaveform},
		{"parse empty defaults to WAVEFORM", "", EffectTypeWaveform},
		{"parse lowercase defaults to WAVEFORM", "waveform", EffectTypeWaveform},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ParseEffectType(tt.input)
			if result != tt.expected {
				t.Errorf("ParseEffectType(%q) = %v, expected %v", tt.input, result, tt.expected)
			}
		})
	}
}

func TestEffectTypeString(t *testing.T) {
	tests := []struct {
		effectType EffectType
		expected   string
	}{
		{EffectTypeWaveform, "WAVEFORM"},
		{EffectTypeCrossfade, "CROSSFADE"},
		{EffectTypeStatic, "STATIC"},
		{EffectTypeMaster, "MASTER"},
	}

	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			result := tt.effectType.String()
			if result != tt.expected {
				t.Errorf("EffectType.String() = %q, expected %q", result, tt.expected)
			}
		})
	}
}

func TestEffectTypeIsValid(t *testing.T) {
	validTypes := []EffectType{
		EffectTypeWaveform,
		EffectTypeCrossfade,
		EffectTypeStatic,
		EffectTypeMaster,
	}
	for _, et := range validTypes {
		t.Run("valid_"+string(et), func(t *testing.T) {
			if !et.IsValid() {
				t.Errorf("EffectType %q should be valid", et)
			}
		})
	}

	invalidTypes := []EffectType{"INVALID", "", "waveform", "unknown"}
	for _, et := range invalidTypes {
		t.Run("invalid_"+string(et), func(t *testing.T) {
			if et.IsValid() {
				t.Errorf("EffectType %q should be invalid", et)
			}
		})
	}
}

// --- NewEffect Tests ---

func TestNewEffect(t *testing.T) {
	tests := []struct {
		name       string
		effectType EffectType
		priority   EffectPriority
	}{
		{"waveform with base priority", EffectTypeWaveform, NewEffectPriority(PriorityBandBase, 50)},
		{"crossfade with cue priority", EffectTypeCrossfade, PriorityCueDefault},
		{"static with user priority", EffectTypeStatic, PriorityUserDefault},
		{"master with system priority", EffectTypeMaster, PriorityGrandMaster},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			effect := NewEffect(tt.effectType, tt.priority)

			// Verify ID is generated
			if effect.ID == "" {
				t.Error("NewEffect should generate a non-empty ID")
			}

			// Verify effect type and priority
			if effect.EffectType != tt.effectType {
				t.Errorf("EffectType = %v, expected %v", effect.EffectType, tt.effectType)
			}
			if !effect.Priority.Equal(tt.priority) {
				t.Errorf("Priority = %v, expected %v", effect.Priority, tt.priority)
			}

			// Verify defaults
			if effect.CompositionMode != ComposeModeOverride {
				t.Errorf("CompositionMode = %v, expected %v", effect.CompositionMode, ComposeModeOverride)
			}
			if effect.OnCueChange != TransitionFadeOut {
				t.Errorf("OnCueChange = %v, expected %v", effect.OnCueChange, TransitionFadeOut)
			}
			if effect.EasingType != DefaultEasing {
				t.Errorf("EasingType = %v, expected %v", effect.EasingType, DefaultEasing)
			}
			if effect.Frequency != 1.0 {
				t.Errorf("Frequency = %v, expected 1.0", effect.Frequency)
			}
			if effect.Amplitude != 100.0 {
				t.Errorf("Amplitude = %v, expected 100.0", effect.Amplitude)
			}
			if effect.Offset != 50.0 {
				t.Errorf("Offset = %v, expected 50.0", effect.Offset)
			}
			if effect.MasterValue != 1.0 {
				t.Errorf("MasterValue = %v, expected 1.0", effect.MasterValue)
			}
		})
	}
}

func TestNewEffectGeneratesUniqueIDs(t *testing.T) {
	basePriority := NewEffectPriority(PriorityBandBase, 50)
	effect1 := NewEffect(EffectTypeWaveform, basePriority)
	effect2 := NewEffect(EffectTypeWaveform, basePriority)

	if effect1.ID == effect2.ID {
		t.Error("NewEffect should generate unique IDs for each effect")
	}
}

// --- Effect.Clone Tests ---

func TestEffectClone(t *testing.T) {
	// Create an effect with all fields populated
	fadeDuration := 2.5
	phaseOffset := 45.0
	ampScale := 0.8
	freqScale := 1.5

	original := &Effect{
		ID:              "test-id",
		Name:            "Test Effect",
		Description:     "A test effect",
		ProjectID:       "project-123",
		EffectType:      EffectTypeCrossfade,
		Priority:        PriorityCueDefault,
		CompositionMode: ComposeModeAdditive,
		OnCueChange:     TransitionPersist,
		FadeDuration:    &fadeDuration,
		EasingType:      EasingInOutCubic,
		Waveform:        WaveformSine,
		Frequency:       2.0,
		Amplitude:       75.0,
		Offset:          25.0,
		PhaseOffset:     90.0,
		MasterValue:     0.5,
		TargetChannels: []EffectChannel{
			{Universe: 1, Channel: 10, PhaseOffset: &phaseOffset, AmplitudeScale: &ampScale, FrequencyScale: &freqScale},
			{Universe: 1, Channel: 11},
			{Universe: 2, Channel: 5, PhaseOffset: &phaseOffset},
		},
	}

	clone := original.Clone()

	// Verify all fields are copied
	if clone.ID != original.ID {
		t.Errorf("Clone.ID = %v, expected %v", clone.ID, original.ID)
	}
	if clone.Name != original.Name {
		t.Errorf("Clone.Name = %v, expected %v", clone.Name, original.Name)
	}
	if clone.Description != original.Description {
		t.Errorf("Clone.Description = %v, expected %v", clone.Description, original.Description)
	}
	if clone.ProjectID != original.ProjectID {
		t.Errorf("Clone.ProjectID = %v, expected %v", clone.ProjectID, original.ProjectID)
	}
	if clone.EffectType != original.EffectType {
		t.Errorf("Clone.EffectType = %v, expected %v", clone.EffectType, original.EffectType)
	}
	if clone.Priority != original.Priority {
		t.Errorf("Clone.Priority = %v, expected %v", clone.Priority, original.Priority)
	}
	if clone.CompositionMode != original.CompositionMode {
		t.Errorf("Clone.CompositionMode = %v, expected %v", clone.CompositionMode, original.CompositionMode)
	}
	if clone.OnCueChange != original.OnCueChange {
		t.Errorf("Clone.OnCueChange = %v, expected %v", clone.OnCueChange, original.OnCueChange)
	}
	if clone.EasingType != original.EasingType {
		t.Errorf("Clone.EasingType = %v, expected %v", clone.EasingType, original.EasingType)
	}
	if clone.Waveform != original.Waveform {
		t.Errorf("Clone.Waveform = %v, expected %v", clone.Waveform, original.Waveform)
	}
	if clone.Frequency != original.Frequency {
		t.Errorf("Clone.Frequency = %v, expected %v", clone.Frequency, original.Frequency)
	}
	if clone.Amplitude != original.Amplitude {
		t.Errorf("Clone.Amplitude = %v, expected %v", clone.Amplitude, original.Amplitude)
	}
	if clone.Offset != original.Offset {
		t.Errorf("Clone.Offset = %v, expected %v", clone.Offset, original.Offset)
	}
	if clone.PhaseOffset != original.PhaseOffset {
		t.Errorf("Clone.PhaseOffset = %v, expected %v", clone.PhaseOffset, original.PhaseOffset)
	}
	if clone.MasterValue != original.MasterValue {
		t.Errorf("Clone.MasterValue = %v, expected %v", clone.MasterValue, original.MasterValue)
	}

	// Verify FadeDuration pointer is deep copied
	if clone.FadeDuration == nil {
		t.Error("Clone.FadeDuration should not be nil")
	} else if clone.FadeDuration == original.FadeDuration {
		t.Error("Clone.FadeDuration should be a different pointer")
	} else if *clone.FadeDuration != *original.FadeDuration {
		t.Errorf("Clone.FadeDuration value = %v, expected %v", *clone.FadeDuration, *original.FadeDuration)
	}

	// Verify TargetChannels are deep copied
	if len(clone.TargetChannels) != len(original.TargetChannels) {
		t.Errorf("Clone.TargetChannels length = %d, expected %d", len(clone.TargetChannels), len(original.TargetChannels))
	}

	// Verify first channel with all pointers
	if clone.TargetChannels[0].Universe != 1 || clone.TargetChannels[0].Channel != 10 {
		t.Error("Clone.TargetChannels[0] basic fields mismatch")
	}
	if clone.TargetChannels[0].PhaseOffset == original.TargetChannels[0].PhaseOffset {
		t.Error("Clone.TargetChannels[0].PhaseOffset should be a different pointer")
	}
	if *clone.TargetChannels[0].PhaseOffset != phaseOffset {
		t.Errorf("Clone.TargetChannels[0].PhaseOffset value = %v, expected %v", *clone.TargetChannels[0].PhaseOffset, phaseOffset)
	}
	if clone.TargetChannels[0].AmplitudeScale == original.TargetChannels[0].AmplitudeScale {
		t.Error("Clone.TargetChannels[0].AmplitudeScale should be a different pointer")
	}
	if clone.TargetChannels[0].FrequencyScale == original.TargetChannels[0].FrequencyScale {
		t.Error("Clone.TargetChannels[0].FrequencyScale should be a different pointer")
	}

	// Verify second channel without pointers
	if clone.TargetChannels[1].PhaseOffset != nil {
		t.Error("Clone.TargetChannels[1].PhaseOffset should be nil")
	}
}

func TestEffectCloneNil(t *testing.T) {
	var effect *Effect
	clone := effect.Clone()

	if clone != nil {
		t.Error("Clone of nil effect should return nil")
	}
}

func TestEffectCloneModifyingCloneDoesNotAffectOriginal(t *testing.T) {
	fadeDuration := 2.5
	phaseOffset := 45.0

	original := &Effect{
		ID:           "test-id",
		Name:         "Test Effect",
		FadeDuration: &fadeDuration,
		TargetChannels: []EffectChannel{
			{Universe: 1, Channel: 10, PhaseOffset: &phaseOffset},
		},
	}

	clone := original.Clone()

	// Modify the clone
	clone.ID = "modified-id"
	clone.Name = "Modified Effect"
	*clone.FadeDuration = 5.0
	*clone.TargetChannels[0].PhaseOffset = 90.0
	clone.TargetChannels[0].Universe = 2

	// Verify original is unchanged
	if original.ID != "test-id" {
		t.Errorf("Original.ID was modified: %v", original.ID)
	}
	if original.Name != "Test Effect" {
		t.Errorf("Original.Name was modified: %v", original.Name)
	}
	if *original.FadeDuration != 2.5 {
		t.Errorf("Original.FadeDuration was modified: %v", *original.FadeDuration)
	}
	if *original.TargetChannels[0].PhaseOffset != 45.0 {
		t.Errorf("Original.TargetChannels[0].PhaseOffset was modified: %v", *original.TargetChannels[0].PhaseOffset)
	}
	if original.TargetChannels[0].Universe != 1 {
		t.Errorf("Original.TargetChannels[0].Universe was modified: %v", original.TargetChannels[0].Universe)
	}
}

func TestEffectCloneWithEmptyTargetChannels(t *testing.T) {
	original := &Effect{
		ID:             "test-id",
		TargetChannels: []EffectChannel{},
	}

	clone := original.Clone()

	if clone.TargetChannels == nil {
		t.Error("Clone.TargetChannels should not be nil for empty slice")
	}
	if len(clone.TargetChannels) != 0 {
		t.Error("Clone.TargetChannels should be empty")
	}
}

func TestEffectCloneWithNilFadeDuration(t *testing.T) {
	original := &Effect{
		ID:           "test-id",
		FadeDuration: nil,
	}

	clone := original.Clone()

	if clone.FadeDuration != nil {
		t.Error("Clone.FadeDuration should be nil when original is nil")
	}
}

// --- NewActiveEffect Tests ---

func TestNewActiveEffect(t *testing.T) {
	effect := &Effect{
		ID:          "effect-123",
		PhaseOffset: 45.0,
	}

	before := time.Now()
	activeEffect := NewActiveEffect(effect)
	after := time.Now()

	// Verify Effect reference
	if activeEffect.Effect != effect {
		t.Error("ActiveEffect.Effect should reference the original effect")
	}

	// Verify StartTime is set correctly
	if activeEffect.StartTime.Before(before) || activeEffect.StartTime.After(after) {
		t.Error("ActiveEffect.StartTime should be set to approximately now")
	}

	// Verify default values
	if activeEffect.Intensity != 100 {
		t.Errorf("ActiveEffect.Intensity = %v, expected 100", activeEffect.Intensity)
	}

	// Verify Phase is set from effect's PhaseOffset
	if activeEffect.Phase != 45.0 {
		t.Errorf("ActiveEffect.Phase = %v, expected 45.0", activeEffect.Phase)
	}

	// Verify maps are initialized
	if activeEffect.FromValues == nil {
		t.Error("ActiveEffect.FromValues should be initialized")
	}
	if activeEffect.ToValues == nil {
		t.Error("ActiveEffect.ToValues should be initialized")
	}
	if activeEffect.cachedValues == nil {
		t.Error("ActiveEffect.cachedValues should be initialized")
	}

	// Verify IntensityFade is nil
	if activeEffect.IntensityFade != nil {
		t.Error("ActiveEffect.IntensityFade should be nil initially")
	}
}

// --- ActiveEffect.IsComplete Tests ---

func TestActiveEffectIsComplete(t *testing.T) {
	tests := []struct {
		name     string
		setup    func() *ActiveEffect
		expected bool
	}{
		{
			name: "waveform effect with full intensity is not complete",
			setup: func() *ActiveEffect {
				effect := &Effect{EffectType: EffectTypeWaveform}
				ae := NewActiveEffect(effect)
				ae.Intensity = 100
				return ae
			},
			expected: false,
		},
		{
			name: "effect with zero intensity and no fade is complete",
			setup: func() *ActiveEffect {
				effect := &Effect{EffectType: EffectTypeWaveform}
				ae := NewActiveEffect(effect)
				ae.Intensity = 0
				ae.IntensityFade = nil
				return ae
			},
			expected: true,
		},
		{
			name: "effect with completed fade to zero is complete",
			setup: func() *ActiveEffect {
				effect := &Effect{EffectType: EffectTypeWaveform}
				ae := NewActiveEffect(effect)
				ae.IntensityFade = &FadeTransition{
					StartValue: 100,
					EndValue:   0,
					Duration:   time.Second,
					StartTime:  time.Now().Add(-2 * time.Second), // Already completed
					EasingType: EasingLinear,
				}
				return ae
			},
			expected: true,
		},
		{
			name: "effect with completed fade to non-zero is not complete",
			setup: func() *ActiveEffect {
				effect := &Effect{EffectType: EffectTypeWaveform}
				ae := NewActiveEffect(effect)
				ae.IntensityFade = &FadeTransition{
					StartValue: 100,
					EndValue:   50,
					Duration:   time.Second,
					StartTime:  time.Now().Add(-2 * time.Second), // Already completed
					EasingType: EasingLinear,
				}
				return ae
			},
			expected: false,
		},
		{
			name: "effect with ongoing fade to zero is not complete",
			setup: func() *ActiveEffect {
				effect := &Effect{EffectType: EffectTypeWaveform}
				ae := NewActiveEffect(effect)
				ae.IntensityFade = &FadeTransition{
					StartValue: 100,
					EndValue:   0,
					Duration:   10 * time.Second,
					StartTime:  time.Now(), // Just started
					EasingType: EasingLinear,
				}
				return ae
			},
			expected: false,
		},
		{
			name: "crossfade effect with duration is complete after duration",
			setup: func() *ActiveEffect {
				effect := &Effect{EffectType: EffectTypeCrossfade}
				ae := NewActiveEffect(effect)
				ae.Duration = time.Second
				ae.StartTime = time.Now().Add(-2 * time.Second) // Past duration
				return ae
			},
			expected: true,
		},
		{
			name: "crossfade effect with duration is not complete before duration",
			setup: func() *ActiveEffect {
				effect := &Effect{EffectType: EffectTypeCrossfade}
				ae := NewActiveEffect(effect)
				ae.Duration = 10 * time.Second
				ae.StartTime = time.Now() // Just started
				return ae
			},
			expected: false,
		},
		{
			name: "crossfade effect with zero duration is not complete by duration",
			setup: func() *ActiveEffect {
				effect := &Effect{EffectType: EffectTypeCrossfade}
				ae := NewActiveEffect(effect)
				ae.Duration = 0
				return ae
			},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			activeEffect := tt.setup()
			result := activeEffect.IsComplete()
			if result != tt.expected {
				t.Errorf("IsComplete() = %v, expected %v", result, tt.expected)
			}
		})
	}
}

// --- ActiveEffect.GetProgress Tests ---

func TestActiveEffectGetProgress(t *testing.T) {
	tests := []struct {
		name       string
		effectType EffectType
		duration   time.Duration
		elapsed    time.Duration
		expected   float64
		tolerance  float64
	}{
		{
			name:       "crossfade at start",
			effectType: EffectTypeCrossfade,
			duration:   time.Second,
			elapsed:    0,
			expected:   0.0,
			tolerance:  0.05,
		},
		{
			name:       "crossfade at half",
			effectType: EffectTypeCrossfade,
			duration:   time.Second,
			elapsed:    500 * time.Millisecond,
			expected:   0.5,
			tolerance:  0.05,
		},
		{
			name:       "crossfade at end",
			effectType: EffectTypeCrossfade,
			duration:   time.Second,
			elapsed:    time.Second,
			expected:   1.0,
			tolerance:  0.05,
		},
		{
			name:       "crossfade past end should clamp to 1",
			effectType: EffectTypeCrossfade,
			duration:   time.Second,
			elapsed:    2 * time.Second,
			expected:   1.0,
			tolerance:  0.01,
		},
		{
			name:       "crossfade with zero duration returns 1",
			effectType: EffectTypeCrossfade,
			duration:   0,
			elapsed:    0,
			expected:   1.0,
			tolerance:  0.01,
		},
		{
			name:       "waveform always returns 1",
			effectType: EffectTypeWaveform,
			duration:   time.Second,
			elapsed:    500 * time.Millisecond,
			expected:   1.0,
			tolerance:  0.01,
		},
		{
			name:       "static always returns 1",
			effectType: EffectTypeStatic,
			duration:   time.Second,
			elapsed:    500 * time.Millisecond,
			expected:   1.0,
			tolerance:  0.01,
		},
		{
			name:       "master always returns 1",
			effectType: EffectTypeMaster,
			duration:   time.Second,
			elapsed:    500 * time.Millisecond,
			expected:   1.0,
			tolerance:  0.01,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			effect := &Effect{EffectType: tt.effectType}
			activeEffect := NewActiveEffect(effect)
			activeEffect.Duration = tt.duration
			activeEffect.StartTime = time.Now().Add(-tt.elapsed)

			result := activeEffect.GetProgress()
			if math.Abs(result-tt.expected) > tt.tolerance {
				t.Errorf("GetProgress() = %f, expected %f (tolerance: %f)", result, tt.expected, tt.tolerance)
			}
		})
	}
}

// --- ActiveEffect.GetCurrentIntensity Tests ---

func TestActiveEffectGetCurrentIntensity(t *testing.T) {
	tests := []struct {
		name      string
		setup     func() *ActiveEffect
		expected  float64
		tolerance float64
	}{
		{
			name: "no fade returns base intensity",
			setup: func() *ActiveEffect {
				effect := &Effect{EffectType: EffectTypeWaveform}
				ae := NewActiveEffect(effect)
				ae.Intensity = 75
				ae.IntensityFade = nil
				return ae
			},
			expected:  75,
			tolerance: 0.01,
		},
		{
			name: "with fade at start",
			setup: func() *ActiveEffect {
				effect := &Effect{EffectType: EffectTypeWaveform}
				ae := NewActiveEffect(effect)
				ae.Intensity = 100
				ae.IntensityFade = &FadeTransition{
					StartValue: 100,
					EndValue:   0,
					Duration:   time.Second,
					StartTime:  time.Now(),
					EasingType: EasingLinear,
				}
				return ae
			},
			expected:  100,
			tolerance: 5,
		},
		{
			name: "with fade at midpoint",
			setup: func() *ActiveEffect {
				effect := &Effect{EffectType: EffectTypeWaveform}
				ae := NewActiveEffect(effect)
				ae.Intensity = 100
				ae.IntensityFade = &FadeTransition{
					StartValue: 100,
					EndValue:   0,
					Duration:   time.Second,
					StartTime:  time.Now().Add(-500 * time.Millisecond),
					EasingType: EasingLinear,
				}
				return ae
			},
			expected:  50,
			tolerance: 5,
		},
		{
			name: "with fade completed",
			setup: func() *ActiveEffect {
				effect := &Effect{EffectType: EffectTypeWaveform}
				ae := NewActiveEffect(effect)
				ae.Intensity = 100
				ae.IntensityFade = &FadeTransition{
					StartValue: 100,
					EndValue:   25,
					Duration:   time.Second,
					StartTime:  time.Now().Add(-2 * time.Second),
					EasingType: EasingLinear,
				}
				return ae
			},
			expected:  25,
			tolerance: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			activeEffect := tt.setup()
			result := activeEffect.GetCurrentIntensity()
			if math.Abs(result-tt.expected) > tt.tolerance {
				t.Errorf("GetCurrentIntensity() = %f, expected %f (tolerance: %f)", result, tt.expected, tt.tolerance)
			}
		})
	}
}

// --- ActiveEffect.FadeIntensityTo Tests ---

func TestActiveEffectFadeIntensityTo(t *testing.T) {
	tests := []struct {
		name           string
		initialIntensity float64
		targetIntensity  float64
		duration       time.Duration
		easing         EasingType
	}{
		{
			name:           "fade from 100 to 0",
			initialIntensity: 100,
			targetIntensity:  0,
			duration:       3 * time.Second,
			easing:         EasingLinear,
		},
		{
			name:           "fade from 0 to 100",
			initialIntensity: 0,
			targetIntensity:  100,
			duration:       2 * time.Second,
			easing:         EasingInOutSine,
		},
		{
			name:           "fade from 50 to 75",
			initialIntensity: 50,
			targetIntensity:  75,
			duration:       time.Second,
			easing:         EasingInOutCubic,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			effect := &Effect{EffectType: EffectTypeWaveform}
			activeEffect := NewActiveEffect(effect)
			activeEffect.Intensity = tt.initialIntensity

			before := time.Now()
			activeEffect.FadeIntensityTo(tt.targetIntensity, tt.duration, tt.easing)
			after := time.Now()

			// Verify IntensityFade is created
			if activeEffect.IntensityFade == nil {
				t.Fatal("IntensityFade should be created")
			}

			// Verify fade parameters
			if activeEffect.IntensityFade.StartValue != tt.initialIntensity {
				t.Errorf("IntensityFade.StartValue = %v, expected %v", activeEffect.IntensityFade.StartValue, tt.initialIntensity)
			}
			if activeEffect.IntensityFade.EndValue != tt.targetIntensity {
				t.Errorf("IntensityFade.EndValue = %v, expected %v", activeEffect.IntensityFade.EndValue, tt.targetIntensity)
			}
			if activeEffect.IntensityFade.Duration != tt.duration {
				t.Errorf("IntensityFade.Duration = %v, expected %v", activeEffect.IntensityFade.Duration, tt.duration)
			}
			if activeEffect.IntensityFade.EasingType != tt.easing {
				t.Errorf("IntensityFade.EasingType = %v, expected %v", activeEffect.IntensityFade.EasingType, tt.easing)
			}

			// Verify StartTime is set correctly
			if activeEffect.IntensityFade.StartTime.Before(before) || activeEffect.IntensityFade.StartTime.After(after) {
				t.Error("IntensityFade.StartTime should be set to approximately now")
			}
		})
	}
}

// --- ActiveEffect.UpdateIntensityFromFade Tests ---

func TestActiveEffectUpdateIntensityFromFade(t *testing.T) {
	t.Run("no fade does nothing", func(t *testing.T) {
		effect := &Effect{EffectType: EffectTypeWaveform}
		activeEffect := NewActiveEffect(effect)
		activeEffect.Intensity = 75
		activeEffect.IntensityFade = nil

		activeEffect.UpdateIntensityFromFade()

		if activeEffect.Intensity != 75 {
			t.Errorf("Intensity should remain 75, got %v", activeEffect.Intensity)
		}
		if activeEffect.IntensityFade != nil {
			t.Error("IntensityFade should remain nil")
		}
	})

	t.Run("ongoing fade updates intensity but keeps fade", func(t *testing.T) {
		effect := &Effect{EffectType: EffectTypeWaveform}
		activeEffect := NewActiveEffect(effect)
		activeEffect.Intensity = 100
		activeEffect.IntensityFade = &FadeTransition{
			StartValue: 100,
			EndValue:   0,
			Duration:   10 * time.Second, // Long duration so it's not complete
			StartTime:  time.Now(),
			EasingType: EasingLinear,
		}

		activeEffect.UpdateIntensityFromFade()

		// Intensity should be updated (close to 100 since we just started)
		if activeEffect.Intensity < 90 || activeEffect.Intensity > 100 {
			t.Errorf("Intensity should be close to 100, got %v", activeEffect.Intensity)
		}
		// IntensityFade should still exist
		if activeEffect.IntensityFade == nil {
			t.Error("IntensityFade should still exist during ongoing fade")
		}
	})

	t.Run("completed fade updates intensity and clears fade", func(t *testing.T) {
		effect := &Effect{EffectType: EffectTypeWaveform}
		activeEffect := NewActiveEffect(effect)
		activeEffect.Intensity = 100
		activeEffect.IntensityFade = &FadeTransition{
			StartValue: 100,
			EndValue:   25,
			Duration:   time.Second,
			StartTime:  time.Now().Add(-2 * time.Second), // Already completed
			EasingType: EasingLinear,
		}

		activeEffect.UpdateIntensityFromFade()

		// Intensity should be at end value
		if activeEffect.Intensity != 25 {
			t.Errorf("Intensity should be 25, got %v", activeEffect.Intensity)
		}
		// IntensityFade should be cleared
		if activeEffect.IntensityFade != nil {
			t.Error("IntensityFade should be nil after fade completes")
		}
	})
}

// --- ActiveEffect Cached Values Tests ---

func TestActiveEffectCachedValues(t *testing.T) {
	effect := &Effect{EffectType: EffectTypeWaveform}
	activeEffect := NewActiveEffect(effect)

	// Verify initially empty
	cached := activeEffect.GetCachedValues()
	if len(cached) != 0 {
		t.Error("Cached values should be empty initially")
	}

	// Set cached values
	values := map[ChannelKey]int{
		{Universe: 1, Channel: 10}: 128,
		{Universe: 1, Channel: 11}: 255,
		{Universe: 2, Channel: 5}:  64,
	}
	activeEffect.SetCachedValues(values)

	// Retrieve cached values
	retrieved := activeEffect.GetCachedValues()
	if len(retrieved) != 3 {
		t.Errorf("Expected 3 cached values, got %d", len(retrieved))
	}
	if retrieved[ChannelKey{Universe: 1, Channel: 10}] != 128 {
		t.Error("Cached value mismatch for universe 1, channel 10")
	}
	if retrieved[ChannelKey{Universe: 1, Channel: 11}] != 255 {
		t.Error("Cached value mismatch for universe 1, channel 11")
	}
	if retrieved[ChannelKey{Universe: 2, Channel: 5}] != 64 {
		t.Error("Cached value mismatch for universe 2, channel 5")
	}
}

// --- EffectChannel Tests ---

func TestEffectChannel(t *testing.T) {
	phaseOffset := 45.0
	ampScale := 0.8
	freqScale := 1.5

	channel := EffectChannel{
		Universe:       1,
		Channel:        10,
		PhaseOffset:    &phaseOffset,
		AmplitudeScale: &ampScale,
		FrequencyScale: &freqScale,
	}

	if channel.Universe != 1 {
		t.Errorf("Universe = %d, expected 1", channel.Universe)
	}
	if channel.Channel != 10 {
		t.Errorf("Channel = %d, expected 10", channel.Channel)
	}
	if *channel.PhaseOffset != 45.0 {
		t.Errorf("PhaseOffset = %v, expected 45.0", *channel.PhaseOffset)
	}
	if *channel.AmplitudeScale != 0.8 {
		t.Errorf("AmplitudeScale = %v, expected 0.8", *channel.AmplitudeScale)
	}
	if *channel.FrequencyScale != 1.5 {
		t.Errorf("FrequencyScale = %v, expected 1.5", *channel.FrequencyScale)
	}
}

func TestEffectChannelWithNilPointers(t *testing.T) {
	channel := EffectChannel{
		Universe: 1,
		Channel:  10,
	}

	if channel.PhaseOffset != nil {
		t.Error("PhaseOffset should be nil")
	}
	if channel.AmplitudeScale != nil {
		t.Error("AmplitudeScale should be nil")
	}
	if channel.FrequencyScale != nil {
		t.Error("FrequencyScale should be nil")
	}
}
