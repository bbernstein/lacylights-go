package modulator

import (
	"math"
	"testing"
	"time"
)

func TestFadeTransitionCurrentValue(t *testing.T) {
	tests := []struct {
		name       string
		startValue float64
		endValue   float64
		duration   time.Duration
		elapsed    time.Duration
		easingType EasingType
		expected   float64
		tolerance  float64
	}{
		{
			name:       "at start",
			startValue: 100,
			endValue:   0,
			duration:   time.Second,
			elapsed:    0,
			easingType: EasingLinear,
			expected:   100,
			tolerance:  1,
		},
		{
			name:       "at end",
			startValue: 100,
			endValue:   0,
			duration:   time.Second,
			elapsed:    time.Second,
			easingType: EasingLinear,
			expected:   0,
			tolerance:  1,
		},
		{
			name:       "half way linear",
			startValue: 0,
			endValue:   100,
			duration:   time.Second,
			elapsed:    500 * time.Millisecond,
			easingType: EasingLinear,
			expected:   50,
			tolerance:  2,
		},
		{
			name:       "reverse fade",
			startValue: 255,
			endValue:   0,
			duration:   time.Second,
			elapsed:    500 * time.Millisecond,
			easingType: EasingLinear,
			expected:   127.5,
			tolerance:  2,
		},
		{
			name:       "zero duration",
			startValue: 100,
			endValue:   200,
			duration:   0,
			elapsed:    0,
			easingType: EasingLinear,
			expected:   200, // Should immediately be at end value
			tolerance:  1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fade := &FadeTransition{
				StartValue: tt.startValue,
				EndValue:   tt.endValue,
				Duration:   tt.duration,
				StartTime:  time.Now().Add(-tt.elapsed),
				EasingType: tt.easingType,
			}

			result := fade.CurrentValue()
			if math.Abs(result-tt.expected) > tt.tolerance {
				t.Errorf("CurrentValue() = %f, expected %f (±%f)", result, tt.expected, tt.tolerance)
			}
		})
	}
}

func TestFadeTransitionIsComplete(t *testing.T) {
	tests := []struct {
		name     string
		duration time.Duration
		elapsed  time.Duration
		expected bool
	}{
		{
			name:     "not started",
			duration: time.Second,
			elapsed:  0,
			expected: false,
		},
		{
			name:     "half way",
			duration: time.Second,
			elapsed:  500 * time.Millisecond,
			expected: false,
		},
		{
			name:     "exactly complete",
			duration: time.Second,
			elapsed:  time.Second,
			expected: true,
		},
		{
			name:     "past complete",
			duration: time.Second,
			elapsed:  2 * time.Second,
			expected: true,
		},
		{
			name:     "zero duration",
			duration: 0,
			elapsed:  0,
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fade := &FadeTransition{
				StartValue: 0,
				EndValue:   100,
				Duration:   tt.duration,
				StartTime:  time.Now().Add(-tt.elapsed),
				EasingType: EasingLinear,
			}

			result := fade.IsComplete()
			if result != tt.expected {
				t.Errorf("IsComplete() = %v, expected %v", result, tt.expected)
			}
		})
	}
}

func TestFadeTransitionProgress(t *testing.T) {
	tests := []struct {
		name     string
		duration time.Duration
		elapsed  time.Duration
		expected float64
	}{
		{"at start", time.Second, 0, 0},
		{"half way", time.Second, 500 * time.Millisecond, 0.5},
		{"at end", time.Second, time.Second, 1.0},
		{"past end", time.Second, 2 * time.Second, 1.0},
		{"zero duration", 0, 0, 1.0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fade := &FadeTransition{
				StartValue: 0,
				EndValue:   100,
				Duration:   tt.duration,
				StartTime:  time.Now().Add(-tt.elapsed),
				EasingType: EasingLinear,
			}

			result := fade.Progress()
			if math.Abs(result-tt.expected) > 0.01 {
				t.Errorf("Progress() = %f, expected %f", result, tt.expected)
			}
		})
	}
}

func TestClamp(t *testing.T) {
	tests := []struct {
		value    int
		min      int
		max      int
		expected int
	}{
		{50, 0, 100, 50},    // In range
		{-10, 0, 100, 0},    // Below min
		{150, 0, 100, 100},  // Above max
		{0, 0, 100, 0},      // At min
		{100, 0, 100, 100},  // At max
		{255, 0, 255, 255},  // DMX max
	}

	for _, tt := range tests {
		result := clamp(tt.value, tt.min, tt.max)
		if result != tt.expected {
			t.Errorf("clamp(%d, %d, %d) = %d, expected %d",
				tt.value, tt.min, tt.max, result, tt.expected)
		}
	}
}

func TestClampFloat(t *testing.T) {
	tests := []struct {
		value    float64
		min      float64
		max      float64
		expected float64
	}{
		{50.5, 0, 100, 50.5},
		{-10.5, 0, 100, 0},
		{150.5, 0, 100, 100},
		{0.0, 0, 100, 0.0},
		{255.0, 0, 255, 255.0},
	}

	for _, tt := range tests {
		result := clampFloat(tt.value, tt.min, tt.max)
		if math.Abs(result-tt.expected) > 0.001 {
			t.Errorf("clampFloat(%f, %f, %f) = %f, expected %f",
				tt.value, tt.min, tt.max, result, tt.expected)
		}
	}
}

func TestLerp(t *testing.T) {
	tests := []struct {
		start    float64
		end      float64
		t        float64
		expected float64
	}{
		{0, 100, 0, 0},
		{0, 100, 1, 100},
		{0, 100, 0.5, 50},
		{100, 0, 0.5, 50},
		{50, 150, 0.5, 100},
	}

	for _, tt := range tests {
		result := lerp(tt.start, tt.end, tt.t)
		if math.Abs(result-tt.expected) > 0.001 {
			t.Errorf("lerp(%f, %f, %f) = %f, expected %f",
				tt.start, tt.end, tt.t, result, tt.expected)
		}
	}
}

func TestChannelKey(t *testing.T) {
	// Test that ChannelKey can be used as a map key
	key1 := ChannelKey{Universe: 1, Channel: 10}
	key2 := ChannelKey{Universe: 1, Channel: 10}
	key3 := ChannelKey{Universe: 2, Channel: 10}

	m := make(map[ChannelKey]int)
	m[key1] = 100

	// Same key should retrieve the value
	if val, ok := m[key2]; !ok || val != 100 {
		t.Error("ChannelKey should work as map key with equal values")
	}

	// Different key should not find value
	if _, ok := m[key3]; ok {
		t.Error("Different ChannelKey should not match")
	}
}

func TestChannelState(t *testing.T) {
	state := ChannelState{
		Value:           128,
		OwnerEffectID:   "effect-123",
		CompositionMode: ComposeModeOverride,
	}

	if state.Value != 128 {
		t.Errorf("Value = %d, expected 128", state.Value)
	}
	if state.OwnerEffectID != "effect-123" {
		t.Errorf("OwnerEffectID = %s, expected effect-123", state.OwnerEffectID)
	}
	if state.CompositionMode != ComposeModeOverride {
		t.Errorf("CompositionMode = %s, expected OVERRIDE", state.CompositionMode)
	}
}
