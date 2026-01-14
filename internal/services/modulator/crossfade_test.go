package modulator

import (
	"math"
	"testing"
	"time"
)

func TestCreateCrossfadeEffect(t *testing.T) {
	tests := []struct {
		name       string
		fromValues map[ChannelKey]int
		toValues   map[ChannelKey]int
		duration   time.Duration
		easingType EasingType
	}{
		{
			name:       "basic crossfade",
			fromValues: map[ChannelKey]int{{Universe: 1, Channel: 1}: 100},
			toValues:   map[ChannelKey]int{{Universe: 1, Channel: 1}: 200},
			duration:   time.Second,
			easingType: EasingLinear,
		},
		{
			name:       "with empty easing (should default)",
			fromValues: map[ChannelKey]int{{Universe: 1, Channel: 1}: 0},
			toValues:   map[ChannelKey]int{{Universe: 1, Channel: 1}: 255},
			duration:   2 * time.Second,
			easingType: "",
		},
		{
			name:       "with multiple channels",
			fromValues: map[ChannelKey]int{{Universe: 1, Channel: 1}: 50, {Universe: 1, Channel: 2}: 100},
			toValues:   map[ChannelKey]int{{Universe: 1, Channel: 1}: 150, {Universe: 1, Channel: 2}: 200},
			duration:   500 * time.Millisecond,
			easingType: EasingInOutCubic,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			effect := CreateCrossfadeEffect(tt.fromValues, tt.toValues, tt.duration, tt.easingType)

			// Verify effect is created with correct properties
			if effect == nil {
				t.Fatal("CreateCrossfadeEffect returned nil")
			}

			if effect.EffectType != EffectTypeCrossfade {
				t.Errorf("EffectType = %v, expected %v", effect.EffectType, EffectTypeCrossfade)
			}

			if effect.Name != "Crossfade" {
				t.Errorf("Name = %q, expected %q", effect.Name, "Crossfade")
			}

			if effect.CompositionMode != ComposeModeOverride {
				t.Errorf("CompositionMode = %v, expected %v", effect.CompositionMode, ComposeModeOverride)
			}

			if effect.OnCueChange != TransitionSnapOff {
				t.Errorf("OnCueChange = %v, expected %v", effect.OnCueChange, TransitionSnapOff)
			}

			if effect.Priority != PriorityCueDefault {
				t.Errorf("Priority = %v, expected %v", effect.Priority, PriorityCueDefault)
			}

			// ID should be generated (non-empty)
			if effect.ID == "" {
				t.Error("Effect ID should be generated")
			}

			// Easing should be set correctly (or default if empty)
			expectedEasing := tt.easingType
			if expectedEasing == "" {
				expectedEasing = DefaultEasing
			}
			if effect.EasingType != expectedEasing {
				t.Errorf("EasingType = %v, expected %v", effect.EasingType, expectedEasing)
			}
		})
	}
}

func TestCreateCrossfadeActive(t *testing.T) {
	tests := []struct {
		name       string
		fromValues map[ChannelKey]int
		toValues   map[ChannelKey]int
		duration   time.Duration
		easingType EasingType
	}{
		{
			name:       "basic active crossfade",
			fromValues: map[ChannelKey]int{{Universe: 1, Channel: 1}: 100, {Universe: 1, Channel: 2}: 150},
			toValues:   map[ChannelKey]int{{Universe: 1, Channel: 1}: 200, {Universe: 1, Channel: 2}: 50},
			duration:   time.Second,
			easingType: EasingLinear,
		},
		{
			name:       "empty from values",
			fromValues: map[ChannelKey]int{},
			toValues:   map[ChannelKey]int{{Universe: 1, Channel: 1}: 255},
			duration:   2 * time.Second,
			easingType: EasingInOutSine,
		},
		{
			name:       "empty to values",
			fromValues: map[ChannelKey]int{{Universe: 1, Channel: 1}: 255},
			toValues:   map[ChannelKey]int{},
			duration:   500 * time.Millisecond,
			easingType: EasingLinear,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			before := time.Now()
			active := CreateCrossfadeActive(tt.fromValues, tt.toValues, tt.duration, tt.easingType)
			after := time.Now()

			if active == nil {
				t.Fatal("CreateCrossfadeActive returned nil")
			}

			// Verify underlying effect
			if active.Effect == nil {
				t.Fatal("ActiveEffect.Effect is nil")
			}

			if active.Effect.EffectType != EffectTypeCrossfade {
				t.Errorf("Effect.EffectType = %v, expected %v", active.Effect.EffectType, EffectTypeCrossfade)
			}

			// Verify from values are stored
			if len(active.FromValues) != len(tt.fromValues) {
				t.Errorf("FromValues length = %d, expected %d", len(active.FromValues), len(tt.fromValues))
			}
			for key, expectedValue := range tt.fromValues {
				if value, ok := active.FromValues[key]; !ok {
					t.Errorf("FromValues missing key %+v", key)
				} else if value != expectedValue {
					t.Errorf("FromValues[%+v] = %d, expected %d", key, value, expectedValue)
				}
			}

			// Verify to values are stored
			if len(active.ToValues) != len(tt.toValues) {
				t.Errorf("ToValues length = %d, expected %d", len(active.ToValues), len(tt.toValues))
			}
			for key, expectedValue := range tt.toValues {
				if value, ok := active.ToValues[key]; !ok {
					t.Errorf("ToValues missing key %+v", key)
				} else if value != expectedValue {
					t.Errorf("ToValues[%+v] = %d, expected %d", key, value, expectedValue)
				}
			}

			// Verify duration
			if active.Duration != tt.duration {
				t.Errorf("Duration = %v, expected %v", active.Duration, tt.duration)
			}

			// Verify intensity is 100
			if active.Intensity != 100 {
				t.Errorf("Intensity = %v, expected 100", active.Intensity)
			}

			// Verify start time is reasonable
			if active.StartTime.Before(before) || active.StartTime.After(after) {
				t.Errorf("StartTime %v is not between %v and %v", active.StartTime, before, after)
			}
		})
	}
}

func TestCalculateCrossfadeValues_Progress(t *testing.T) {
	tests := []struct {
		name           string
		fromValues     map[ChannelKey]int
		toValues       map[ChannelKey]int
		duration       time.Duration
		elapsed        time.Duration
		expectedValues map[ChannelKey]int
		tolerance      int
	}{
		{
			name:           "0% progress",
			fromValues:     map[ChannelKey]int{{Universe: 1, Channel: 1}: 0},
			toValues:       map[ChannelKey]int{{Universe: 1, Channel: 1}: 255},
			duration:       time.Second,
			elapsed:        0,
			expectedValues: map[ChannelKey]int{{Universe: 1, Channel: 1}: 0},
			tolerance:      2,
		},
		{
			name:           "50% progress",
			fromValues:     map[ChannelKey]int{{Universe: 1, Channel: 1}: 0},
			toValues:       map[ChannelKey]int{{Universe: 1, Channel: 1}: 254},
			duration:       time.Second,
			elapsed:        500 * time.Millisecond,
			expectedValues: map[ChannelKey]int{{Universe: 1, Channel: 1}: 127},
			tolerance:      2,
		},
		{
			name:           "100% progress",
			fromValues:     map[ChannelKey]int{{Universe: 1, Channel: 1}: 0},
			toValues:       map[ChannelKey]int{{Universe: 1, Channel: 1}: 255},
			duration:       time.Second,
			elapsed:        time.Second,
			expectedValues: map[ChannelKey]int{{Universe: 1, Channel: 1}: 255},
			tolerance:      1,
		},
		{
			name:           "past 100% progress",
			fromValues:     map[ChannelKey]int{{Universe: 1, Channel: 1}: 100},
			toValues:       map[ChannelKey]int{{Universe: 1, Channel: 1}: 200},
			duration:       time.Second,
			elapsed:        2 * time.Second,
			expectedValues: map[ChannelKey]int{{Universe: 1, Channel: 1}: 200},
			tolerance:      1,
		},
		{
			name:           "reverse fade 50%",
			fromValues:     map[ChannelKey]int{{Universe: 1, Channel: 1}: 200},
			toValues:       map[ChannelKey]int{{Universe: 1, Channel: 1}: 100},
			duration:       time.Second,
			elapsed:        500 * time.Millisecond,
			expectedValues: map[ChannelKey]int{{Universe: 1, Channel: 1}: 150},
			tolerance:      2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			active := &ActiveEffect{
				Effect: &Effect{
					EffectType: EffectTypeCrossfade,
					EasingType: EasingLinear,
				},
				StartTime:  time.Now().Add(-tt.elapsed),
				Intensity:  100,
				FromValues: tt.fromValues,
				ToValues:   tt.toValues,
				Duration:   tt.duration,
			}

			values := CalculateCrossfadeValues(active)

			if values == nil {
				t.Fatal("CalculateCrossfadeValues returned nil")
			}

			for key, expected := range tt.expectedValues {
				actual, ok := values[key]
				if !ok {
					t.Errorf("Missing key %+v in result", key)
					continue
				}
				diff := abs(actual - expected)
				if diff > tt.tolerance {
					t.Errorf("values[%+v] = %d, expected %d (+/-%d)", key, actual, expected, tt.tolerance)
				}
			}
		})
	}
}

func TestCalculateCrossfadeValues_ChannelsOnlyInFrom(t *testing.T) {
	// Channels only in 'from' should fade to 0
	fromValues := map[ChannelKey]int{
		{Universe: 1, Channel: 1}: 200,
		{Universe: 1, Channel: 2}: 100, // Only in from
	}
	toValues := map[ChannelKey]int{
		{Universe: 1, Channel: 1}: 200, // In both
		// Channel 2 is missing - should fade to 0
	}

	tests := []struct {
		name     string
		elapsed  time.Duration
		expected map[ChannelKey]int
	}{
		{
			name:    "0% progress - from value preserved",
			elapsed: 0,
			expected: map[ChannelKey]int{
				{Universe: 1, Channel: 1}: 200,
				{Universe: 1, Channel: 2}: 100, // Still at from value
			},
		},
		{
			name:    "50% progress - halfway to 0",
			elapsed: 500 * time.Millisecond,
			expected: map[ChannelKey]int{
				{Universe: 1, Channel: 1}: 200,
				{Universe: 1, Channel: 2}: 50, // Halfway to 0
			},
		},
		{
			name:    "100% progress - faded to 0",
			elapsed: time.Second,
			expected: map[ChannelKey]int{
				{Universe: 1, Channel: 1}: 200,
				{Universe: 1, Channel: 2}: 0, // Fully faded to 0
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			active := &ActiveEffect{
				Effect: &Effect{
					EffectType: EffectTypeCrossfade,
					EasingType: EasingLinear,
				},
				StartTime:  time.Now().Add(-tt.elapsed),
				Intensity:  100,
				FromValues: fromValues,
				ToValues:   toValues,
				Duration:   time.Second,
			}

			values := CalculateCrossfadeValues(active)

			for key, expected := range tt.expected {
				actual, ok := values[key]
				if !ok {
					t.Errorf("Missing key %+v in result", key)
					continue
				}
				diff := abs(actual - expected)
				if diff > 2 {
					t.Errorf("values[%+v] = %d, expected %d", key, actual, expected)
				}
			}
		})
	}
}

func TestCalculateCrossfadeValues_ChannelsOnlyInTo(t *testing.T) {
	// Channels only in 'to' should fade from 0
	fromValues := map[ChannelKey]int{
		{Universe: 1, Channel: 1}: 100, // In both
		// Channel 2 is missing - should start at 0
	}
	toValues := map[ChannelKey]int{
		{Universe: 1, Channel: 1}: 100,
		{Universe: 1, Channel: 2}: 200, // Only in to
	}

	tests := []struct {
		name     string
		elapsed  time.Duration
		expected map[ChannelKey]int
	}{
		{
			name:    "0% progress - starts at 0",
			elapsed: 0,
			expected: map[ChannelKey]int{
				{Universe: 1, Channel: 1}: 100,
				{Universe: 1, Channel: 2}: 0, // Starts at 0
			},
		},
		{
			name:    "50% progress - halfway to target",
			elapsed: 500 * time.Millisecond,
			expected: map[ChannelKey]int{
				{Universe: 1, Channel: 1}: 100,
				{Universe: 1, Channel: 2}: 100, // Halfway from 0 to 200
			},
		},
		{
			name:    "100% progress - at target value",
			elapsed: time.Second,
			expected: map[ChannelKey]int{
				{Universe: 1, Channel: 1}: 100,
				{Universe: 1, Channel: 2}: 200, // At target
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			active := &ActiveEffect{
				Effect: &Effect{
					EffectType: EffectTypeCrossfade,
					EasingType: EasingLinear,
				},
				StartTime:  time.Now().Add(-tt.elapsed),
				Intensity:  100,
				FromValues: fromValues,
				ToValues:   toValues,
				Duration:   time.Second,
			}

			values := CalculateCrossfadeValues(active)

			for key, expected := range tt.expected {
				actual, ok := values[key]
				if !ok {
					t.Errorf("Missing key %+v in result", key)
					continue
				}
				diff := abs(actual - expected)
				if diff > 2 {
					t.Errorf("values[%+v] = %d, expected %d", key, actual, expected)
				}
			}
		})
	}
}

func TestCalculateCrossfadeValues_OverlappingChannels(t *testing.T) {
	// Test with overlapping channels - all channels appear in both from and to
	fromValues := map[ChannelKey]int{
		{Universe: 1, Channel: 1}: 0,
		{Universe: 1, Channel: 2}: 100,
		{Universe: 2, Channel: 1}: 255,
	}
	toValues := map[ChannelKey]int{
		{Universe: 1, Channel: 1}: 255,
		{Universe: 1, Channel: 2}: 200,
		{Universe: 2, Channel: 1}: 0,
	}

	tests := []struct {
		name     string
		elapsed  time.Duration
		expected map[ChannelKey]int
	}{
		{
			name:    "0% progress",
			elapsed: 0,
			expected: map[ChannelKey]int{
				{Universe: 1, Channel: 1}: 0,
				{Universe: 1, Channel: 2}: 100,
				{Universe: 2, Channel: 1}: 255,
			},
		},
		{
			name:    "50% progress",
			elapsed: 500 * time.Millisecond,
			expected: map[ChannelKey]int{
				{Universe: 1, Channel: 1}: 127, // 0 -> 255
				{Universe: 1, Channel: 2}: 150, // 100 -> 200
				{Universe: 2, Channel: 1}: 127, // 255 -> 0
			},
		},
		{
			name:    "100% progress",
			elapsed: time.Second,
			expected: map[ChannelKey]int{
				{Universe: 1, Channel: 1}: 255,
				{Universe: 1, Channel: 2}: 200,
				{Universe: 2, Channel: 1}: 0,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			active := &ActiveEffect{
				Effect: &Effect{
					EffectType: EffectTypeCrossfade,
					EasingType: EasingLinear,
				},
				StartTime:  time.Now().Add(-tt.elapsed),
				Intensity:  100,
				FromValues: fromValues,
				ToValues:   toValues,
				Duration:   time.Second,
			}

			values := CalculateCrossfadeValues(active)

			if len(values) != 3 {
				t.Errorf("Expected 3 channel values, got %d", len(values))
			}

			for key, expected := range tt.expected {
				actual, ok := values[key]
				if !ok {
					t.Errorf("Missing key %+v in result", key)
					continue
				}
				diff := abs(actual - expected)
				if diff > 2 {
					t.Errorf("values[%+v] = %d, expected %d", key, actual, expected)
				}
			}
		})
	}
}

func TestCalculateCrossfadeValues_NonCrossfadeType(t *testing.T) {
	// Should return nil for non-crossfade effects
	active := &ActiveEffect{
		Effect: &Effect{
			EffectType: EffectTypeWaveform, // Not crossfade
			EasingType: EasingLinear,
		},
		StartTime:  time.Now(),
		Intensity:  100,
		FromValues: map[ChannelKey]int{{Universe: 1, Channel: 1}: 0},
		ToValues:   map[ChannelKey]int{{Universe: 1, Channel: 1}: 255},
		Duration:   time.Second,
	}

	values := CalculateCrossfadeValues(active)

	if values != nil {
		t.Errorf("CalculateCrossfadeValues should return nil for non-crossfade effect, got %v", values)
	}
}

func TestCalculateCrossfadeValues_IntensityScaling(t *testing.T) {
	fromValues := map[ChannelKey]int{
		{Universe: 1, Channel: 1}: 0,
	}
	toValues := map[ChannelKey]int{
		{Universe: 1, Channel: 1}: 200,
	}

	tests := []struct {
		name      string
		intensity float64
		expected  int
	}{
		{"100% intensity", 100, 200},
		{"50% intensity", 50, 100},
		{"0% intensity", 0, 0},
		{"25% intensity", 25, 50},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			active := &ActiveEffect{
				Effect: &Effect{
					EffectType: EffectTypeCrossfade,
					EasingType: EasingLinear,
				},
				StartTime:  time.Now().Add(-time.Second), // 100% progress
				Intensity:  tt.intensity,
				FromValues: fromValues,
				ToValues:   toValues,
				Duration:   time.Second,
			}

			values := CalculateCrossfadeValues(active)

			key := ChannelKey{Universe: 1, Channel: 1}
			actual := values[key]
			diff := abs(actual - tt.expected)
			if diff > 2 {
				t.Errorf("At %v%% intensity, value = %d, expected %d", tt.intensity, actual, tt.expected)
			}
		})
	}
}

func TestIsCrossfadeComplete(t *testing.T) {
	tests := []struct {
		name     string
		elapsed  time.Duration
		duration time.Duration
		expected bool
	}{
		{
			name:     "not started",
			elapsed:  0,
			duration: time.Second,
			expected: false,
		},
		{
			name:     "halfway",
			elapsed:  500 * time.Millisecond,
			duration: time.Second,
			expected: false,
		},
		{
			name:     "just before complete",
			elapsed:  999 * time.Millisecond,
			duration: time.Second,
			expected: false,
		},
		{
			name:     "exactly complete",
			elapsed:  time.Second,
			duration: time.Second,
			expected: true,
		},
		{
			name:     "past complete",
			elapsed:  2 * time.Second,
			duration: time.Second,
			expected: true,
		},
		{
			name:     "zero duration - immediately complete",
			elapsed:  0,
			duration: 0,
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			active := &ActiveEffect{
				Effect: &Effect{
					EffectType: EffectTypeCrossfade,
				},
				StartTime:  time.Now().Add(-tt.elapsed),
				Duration:   tt.duration,
				FromValues: map[ChannelKey]int{},
				ToValues:   map[ChannelKey]int{},
			}

			result := IsCrossfadeComplete(active)

			if result != tt.expected {
				t.Errorf("IsCrossfadeComplete() = %v, expected %v", result, tt.expected)
			}
		})
	}
}

func TestIsCrossfadeComplete_NonCrossfadeType(t *testing.T) {
	// Should return false for non-crossfade effects
	effectTypes := []EffectType{EffectTypeWaveform, EffectTypeStatic, EffectTypeMaster}

	for _, effectType := range effectTypes {
		t.Run(string(effectType), func(t *testing.T) {
			active := &ActiveEffect{
				Effect: &Effect{
					EffectType: effectType,
				},
				StartTime: time.Now().Add(-2 * time.Second), // Past any duration
				Duration:  time.Second,
			}

			result := IsCrossfadeComplete(active)

			if result != false {
				t.Errorf("IsCrossfadeComplete() for %s = %v, expected false", effectType, result)
			}
		})
	}
}

func TestGetCrossfadeRemainingTime(t *testing.T) {
	tests := []struct {
		name     string
		duration time.Duration
		elapsed  time.Duration
		expected time.Duration
	}{
		{
			name:     "not started",
			duration: time.Second,
			elapsed:  0,
			expected: time.Second,
		},
		{
			name:     "halfway",
			duration: time.Second,
			elapsed:  500 * time.Millisecond,
			expected: 500 * time.Millisecond,
		},
		{
			name:     "nearly complete",
			duration: time.Second,
			elapsed:  900 * time.Millisecond,
			expected: 100 * time.Millisecond,
		},
		{
			name:     "exactly complete",
			duration: time.Second,
			elapsed:  time.Second,
			expected: 0,
		},
		{
			name:     "past complete",
			duration: time.Second,
			elapsed:  2 * time.Second,
			expected: 0,
		},
		{
			name:     "2 second fade at 25%",
			duration: 2 * time.Second,
			elapsed:  500 * time.Millisecond,
			expected: 1500 * time.Millisecond,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			active := &ActiveEffect{
				Effect: &Effect{
					EffectType: EffectTypeCrossfade,
				},
				StartTime: time.Now().Add(-tt.elapsed),
				Duration:  tt.duration,
			}

			result := GetCrossfadeRemainingTime(active)

			// Allow small tolerance for timing
			diff := result - tt.expected
			if diff < 0 {
				diff = -diff
			}
			// Allow up to 50ms tolerance for test execution time
			tolerance := 50 * time.Millisecond
			if diff > tolerance {
				t.Errorf("GetCrossfadeRemainingTime() = %v, expected %v (+/-%v)", result, tt.expected, tolerance)
			}
		})
	}
}

func TestGetCrossfadeRemainingTime_NonCrossfadeType(t *testing.T) {
	// Should return 0 for non-crossfade effects
	effectTypes := []EffectType{EffectTypeWaveform, EffectTypeStatic, EffectTypeMaster}

	for _, effectType := range effectTypes {
		t.Run(string(effectType), func(t *testing.T) {
			active := &ActiveEffect{
				Effect: &Effect{
					EffectType: effectType,
				},
				StartTime: time.Now(),
				Duration:  time.Second,
			}

			result := GetCrossfadeRemainingTime(active)

			if result != 0 {
				t.Errorf("GetCrossfadeRemainingTime() for %s = %v, expected 0", effectType, result)
			}
		})
	}
}

func TestMergeCrossfadeValues(t *testing.T) {
	tests := []struct {
		name             string
		fromValues       map[ChannelKey]int
		toValues         map[ChannelKey]int
		existingState    map[ChannelKey]ChannelState
		expectedState    map[ChannelKey]ChannelState
		checkOwnerEffect bool
	}{
		{
			name:       "merge into empty state",
			fromValues: map[ChannelKey]int{{Universe: 1, Channel: 1}: 0},
			toValues:   map[ChannelKey]int{{Universe: 1, Channel: 1}: 200},
			existingState: map[ChannelKey]ChannelState{},
			expectedState: map[ChannelKey]ChannelState{
				{Universe: 1, Channel: 1}: {
					Value:           200,
					CompositionMode: ComposeModeOverride,
				},
			},
			checkOwnerEffect: true,
		},
		{
			name:       "override existing value",
			fromValues: map[ChannelKey]int{{Universe: 1, Channel: 1}: 100},
			toValues:   map[ChannelKey]int{{Universe: 1, Channel: 1}: 200},
			existingState: map[ChannelKey]ChannelState{
				{Universe: 1, Channel: 1}: {
					Value:           50,
					OwnerEffectID:   "old-effect",
					CompositionMode: ComposeModeOverride,
				},
			},
			expectedState: map[ChannelKey]ChannelState{
				{Universe: 1, Channel: 1}: {
					Value:           200,
					CompositionMode: ComposeModeOverride,
				},
			},
			checkOwnerEffect: true,
		},
		{
			name:       "merge multiple channels",
			fromValues: map[ChannelKey]int{{Universe: 1, Channel: 1}: 0, {Universe: 1, Channel: 2}: 0},
			toValues:   map[ChannelKey]int{{Universe: 1, Channel: 1}: 100, {Universe: 1, Channel: 2}: 200},
			existingState: map[ChannelKey]ChannelState{
				{Universe: 1, Channel: 3}: {
					Value:           150,
					OwnerEffectID:   "other-effect",
					CompositionMode: ComposeModeOverride,
				},
			},
			expectedState: map[ChannelKey]ChannelState{
				{Universe: 1, Channel: 1}: {
					Value:           100,
					CompositionMode: ComposeModeOverride,
				},
				{Universe: 1, Channel: 2}: {
					Value:           200,
					CompositionMode: ComposeModeOverride,
				},
				{Universe: 1, Channel: 3}: {
					Value:           150,
					OwnerEffectID:   "other-effect",
					CompositionMode: ComposeModeOverride,
				},
			},
			checkOwnerEffect: false, // Don't check owner for existing channel 3
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			active := &ActiveEffect{
				Effect: &Effect{
					ID:              "test-crossfade-effect",
					EffectType:      EffectTypeCrossfade,
					EasingType:      EasingLinear,
					CompositionMode: ComposeModeOverride,
				},
				StartTime:  time.Now().Add(-time.Second), // 100% progress
				Intensity:  100,
				FromValues: tt.fromValues,
				ToValues:   tt.toValues,
				Duration:   time.Second,
			}

			// Make a copy of existing state
			existing := make(map[ChannelKey]ChannelState)
			for k, v := range tt.existingState {
				existing[k] = v
			}

			MergeCrossfadeValues(active, existing)

			// Verify expected state
			for key, expectedState := range tt.expectedState {
				actualState, ok := existing[key]
				if !ok {
					t.Errorf("Missing key %+v in result", key)
					continue
				}

				diff := abs(actualState.Value - expectedState.Value)
				if diff > 2 {
					t.Errorf("State[%+v].Value = %d, expected %d", key, actualState.Value, expectedState.Value)
				}

				if actualState.CompositionMode != expectedState.CompositionMode {
					t.Errorf("State[%+v].CompositionMode = %v, expected %v",
						key, actualState.CompositionMode, expectedState.CompositionMode)
				}

				// Check owner effect ID for crossfade-modified channels
				if tt.checkOwnerEffect && actualState.OwnerEffectID != active.Effect.ID {
					t.Errorf("State[%+v].OwnerEffectID = %v, expected %v",
						key, actualState.OwnerEffectID, active.Effect.ID)
				}
			}
		})
	}
}

func TestMergeCrossfadeValues_NonCrossfadeType(t *testing.T) {
	// Should not modify existing state for non-crossfade effects
	active := &ActiveEffect{
		Effect: &Effect{
			EffectType: EffectTypeWaveform, // Not crossfade
		},
		FromValues: map[ChannelKey]int{{Universe: 1, Channel: 1}: 100},
		ToValues:   map[ChannelKey]int{{Universe: 1, Channel: 1}: 200},
	}

	existing := map[ChannelKey]ChannelState{
		{Universe: 1, Channel: 1}: {Value: 50},
	}

	MergeCrossfadeValues(active, existing)

	// State should be unchanged
	if existing[ChannelKey{Universe: 1, Channel: 1}].Value != 50 {
		t.Error("MergeCrossfadeValues should not modify state for non-crossfade effects")
	}
}

func TestMergeCrossfadeValues_WithAdditive(t *testing.T) {
	// Test with additive composition mode
	active := &ActiveEffect{
		Effect: &Effect{
			ID:              "additive-crossfade",
			EffectType:      EffectTypeCrossfade,
			EasingType:      EasingLinear,
			CompositionMode: ComposeModeAdditive,
		},
		StartTime:  time.Now().Add(-time.Second), // 100% progress
		Intensity:  100,
		FromValues: map[ChannelKey]int{{Universe: 1, Channel: 1}: 0},
		ToValues:   map[ChannelKey]int{{Universe: 1, Channel: 1}: 50},
		Duration:   time.Second,
	}

	existing := map[ChannelKey]ChannelState{
		{Universe: 1, Channel: 1}: {Value: 100},
	}

	MergeCrossfadeValues(active, existing)

	// With additive, 100 + 50 = 150
	expected := 150
	actual := existing[ChannelKey{Universe: 1, Channel: 1}].Value
	diff := abs(actual - expected)
	if diff > 2 {
		t.Errorf("Additive merge: value = %d, expected %d", actual, expected)
	}
}

func TestCalculateCrossfadeValues_WithEasing(t *testing.T) {
	// Test that easing is applied correctly
	fromValues := map[ChannelKey]int{
		{Universe: 1, Channel: 1}: 0,
	}
	toValues := map[ChannelKey]int{
		{Universe: 1, Channel: 1}: 100,
	}

	// At 50% progress with different easings
	elapsed := 500 * time.Millisecond
	duration := time.Second

	// Linear should be exactly 50
	activeLinear := &ActiveEffect{
		Effect:     &Effect{EffectType: EffectTypeCrossfade, EasingType: EasingLinear},
		StartTime:  time.Now().Add(-elapsed),
		Intensity:  100,
		FromValues: fromValues,
		ToValues:   toValues,
		Duration:   duration,
	}
	valuesLinear := CalculateCrossfadeValues(activeLinear)
	linearValue := valuesLinear[ChannelKey{Universe: 1, Channel: 1}]

	// InOutSine at 50% should also be 50 (symmetric)
	activeInOutSine := &ActiveEffect{
		Effect:     &Effect{EffectType: EffectTypeCrossfade, EasingType: EasingInOutSine},
		StartTime:  time.Now().Add(-elapsed),
		Intensity:  100,
		FromValues: fromValues,
		ToValues:   toValues,
		Duration:   duration,
	}
	valuesInOutSine := CalculateCrossfadeValues(activeInOutSine)
	sineValue := valuesInOutSine[ChannelKey{Universe: 1, Channel: 1}]

	// At 50%, both should be close to 50
	if math.Abs(float64(linearValue)-50) > 2 {
		t.Errorf("Linear at 50%% = %d, expected ~50", linearValue)
	}
	if math.Abs(float64(sineValue)-50) > 2 {
		t.Errorf("InOutSine at 50%% = %d, expected ~50", sineValue)
	}

	// Test at 25% where easings diverge more
	elapsed25 := 250 * time.Millisecond
	activeLinear25 := &ActiveEffect{
		Effect:     &Effect{EffectType: EffectTypeCrossfade, EasingType: EasingLinear},
		StartTime:  time.Now().Add(-elapsed25),
		Intensity:  100,
		FromValues: fromValues,
		ToValues:   toValues,
		Duration:   duration,
	}
	activeInOutSine25 := &ActiveEffect{
		Effect:     &Effect{EffectType: EffectTypeCrossfade, EasingType: EasingInOutSine},
		StartTime:  time.Now().Add(-elapsed25),
		Intensity:  100,
		FromValues: fromValues,
		ToValues:   toValues,
		Duration:   duration,
	}

	linear25 := CalculateCrossfadeValues(activeLinear25)[ChannelKey{Universe: 1, Channel: 1}]
	sine25 := CalculateCrossfadeValues(activeInOutSine25)[ChannelKey{Universe: 1, Channel: 1}]

	// Linear at 25% should be ~25
	if math.Abs(float64(linear25)-25) > 2 {
		t.Errorf("Linear at 25%% = %d, expected ~25", linear25)
	}

	// InOutSine at 25% should be less than linear (slower start)
	if sine25 >= linear25 {
		t.Errorf("InOutSine at 25%% (%d) should be less than linear (%d)", sine25, linear25)
	}
}

// Helper function for absolute value of int
func abs(x int) int {
	if x < 0 {
		return -x
	}
	return x
}
