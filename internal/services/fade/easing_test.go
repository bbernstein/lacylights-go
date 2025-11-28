package fade

import (
	"math"
	"testing"
)

func TestApplyEasing_Linear(t *testing.T) {
	tests := []struct {
		progress float64
		want     float64
	}{
		{0.0, 0.0},
		{0.25, 0.25},
		{0.5, 0.5},
		{0.75, 0.75},
		{1.0, 1.0},
	}

	for _, tt := range tests {
		got := ApplyEasing(tt.progress, EasingLinear)
		if math.Abs(got-tt.want) > 0.0001 {
			t.Errorf("ApplyEasing(%v, LINEAR) = %v, want %v", tt.progress, got, tt.want)
		}
	}
}

func TestApplyEasing_InOutSine(t *testing.T) {
	// InOutSine should be 0 at 0, 0.5 at 0.5, and 1 at 1
	tests := []struct {
		progress float64
		want     float64
	}{
		{0.0, 0.0},
		{0.5, 0.5},
		{1.0, 1.0},
	}

	for _, tt := range tests {
		got := ApplyEasing(tt.progress, EasingInOutSine)
		if math.Abs(got-tt.want) > 0.0001 {
			t.Errorf("ApplyEasing(%v, EASE_IN_OUT_SINE) = %v, want %v", tt.progress, got, tt.want)
		}
	}

	// Check that it's slower at start and end
	early := ApplyEasing(0.1, EasingInOutSine)
	linear := 0.1
	if early >= linear {
		t.Errorf("EASE_IN_OUT_SINE at 0.1 should be less than linear, got %v", early)
	}
}

func TestApplyEasing_InOutCubic(t *testing.T) {
	// InOutCubic should be 0 at 0, 0.5 at 0.5, and 1 at 1
	tests := []struct {
		progress float64
		want     float64
	}{
		{0.0, 0.0},
		{0.5, 0.5},
		{1.0, 1.0},
	}

	for _, tt := range tests {
		got := ApplyEasing(tt.progress, EasingInOutCubic)
		if math.Abs(got-tt.want) > 0.0001 {
			t.Errorf("ApplyEasing(%v, EASE_IN_OUT_CUBIC) = %v, want %v", tt.progress, got, tt.want)
		}
	}
}

func TestApplyEasing_OutExponential(t *testing.T) {
	// OutExponential should be 0 at 0 and 1 at 1
	tests := []struct {
		progress float64
		want     float64
	}{
		{0.0, 0.0},
		{1.0, 1.0},
	}

	for _, tt := range tests {
		got := ApplyEasing(tt.progress, EasingOutExponential)
		if math.Abs(got-tt.want) > 0.0001 {
			t.Errorf("ApplyEasing(%v, EASE_OUT_EXPONENTIAL) = %v, want %v", tt.progress, got, tt.want)
		}
	}

	// Exponential out should be fast at start
	early := ApplyEasing(0.1, EasingOutExponential)
	linear := 0.1
	if early <= linear {
		t.Errorf("EASE_OUT_EXPONENTIAL at 0.1 should be greater than linear, got %v", early)
	}
}

func TestApplyEasing_SCurve(t *testing.T) {
	// S-curve should be near 0 at 0, 0.5 at 0.5, and near 1 at 1
	start := ApplyEasing(0.0, EasingSCurve)
	mid := ApplyEasing(0.5, EasingSCurve)
	end := ApplyEasing(1.0, EasingSCurve)

	if start >= 0.01 {
		t.Errorf("S_CURVE at 0 should be near 0, got %v", start)
	}
	if math.Abs(mid-0.5) > 0.001 {
		t.Errorf("S_CURVE at 0.5 should be 0.5, got %v", mid)
	}
	if end <= 0.99 {
		t.Errorf("S_CURVE at 1 should be near 1, got %v", end)
	}
}

func TestInterpolate(t *testing.T) {
	tests := []struct {
		name       string
		start      float64
		end        float64
		progress   float64
		easingType EasingType
		want       float64
	}{
		{"linear 0 to 255 at 50%", 0, 255, 0.5, EasingLinear, 127.5},
		{"linear 0 to 255 at 0%", 0, 255, 0.0, EasingLinear, 0},
		{"linear 0 to 255 at 100%", 0, 255, 1.0, EasingLinear, 255},
		{"reverse linear 255 to 0 at 50%", 255, 0, 0.5, EasingLinear, 127.5},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Interpolate(tt.start, tt.end, tt.progress, tt.easingType)
			if math.Abs(got-tt.want) > 0.001 {
				t.Errorf("Interpolate(%v, %v, %v, %v) = %v, want %v",
					tt.start, tt.end, tt.progress, tt.easingType, got, tt.want)
			}
		})
	}
}

func TestInterpolate_DefaultEasing(t *testing.T) {
	// Empty easing type should use default (InOutSine)
	withEmpty := Interpolate(0, 255, 0.5, "")
	withSine := Interpolate(0, 255, 0.5, EasingInOutSine)

	if math.Abs(withEmpty-withSine) > 0.001 {
		t.Errorf("Empty easing type should default to EASE_IN_OUT_SINE, got %v vs %v", withEmpty, withSine)
	}
}

func TestApplyEasing_UnknownType(t *testing.T) {
	// Unknown easing type should return linear (progress unchanged)
	got := ApplyEasing(0.5, "UNKNOWN")
	if got != 0.5 {
		t.Errorf("Unknown easing type should return progress unchanged, got %v", got)
	}
}
