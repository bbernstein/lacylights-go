package modulator

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

	// Check that it's slower at start
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

func TestApplyEasing_Bezier(t *testing.T) {
	// Bezier should be 0 at 0 and near 1 at 1
	start := ApplyEasing(0.0, EasingBezier)
	end := ApplyEasing(1.0, EasingBezier)

	if math.Abs(start) > 0.0001 {
		t.Errorf("BEZIER at 0 should be 0, got %v", start)
	}
	if math.Abs(end-1.0) > 0.0001 {
		t.Errorf("BEZIER at 1 should be 1, got %v", end)
	}
}

func TestApplyEasing_Boundaries(t *testing.T) {
	easings := []EasingType{
		EasingLinear, EasingInOutCubic, EasingInOutSine,
		EasingOutExponential, EasingSCurve,
	}

	for _, easing := range easings {
		t.Run(string(easing), func(t *testing.T) {
			// At progress 0, output should be 0 (or very close)
			atZero := ApplyEasing(0, easing)
			if atZero > 0.01 {
				t.Errorf("ApplyEasing(0, %s) = %v, expected near 0", easing, atZero)
			}

			// At progress 1, output should be 1 (or very close)
			atOne := ApplyEasing(1, easing)
			if atOne < 0.99 {
				t.Errorf("ApplyEasing(1, %s) = %v, expected near 1", easing, atOne)
			}
		})
	}
}

func TestApplyEasing_Monotonicity(t *testing.T) {
	// Easing functions should be monotonically increasing
	easings := []EasingType{
		EasingLinear, EasingInOutCubic, EasingInOutSine,
		EasingOutExponential, EasingSCurve,
	}

	for _, easing := range easings {
		t.Run(string(easing), func(t *testing.T) {
			prev := 0.0
			for p := 0.0; p <= 1.0; p += 0.1 {
				current := ApplyEasing(p, easing)
				if current < prev-0.0001 { // Allow small tolerance
					t.Errorf("easing %s should be monotonic: at progress %.1f got %v, prev was %v",
						easing, p, current, prev)
				}
				prev = current
			}
		})
	}
}

func TestApplyEasing_ClampInput(t *testing.T) {
	// Progress outside 0-1 should be clamped
	tests := []struct {
		progress float64
		expected float64
	}{
		{-0.5, 0.0},
		{1.5, 1.0},
		{-10, 0.0},
		{10, 1.0},
	}

	for _, tt := range tests {
		result := ApplyEasing(tt.progress, EasingLinear)
		if math.Abs(result-tt.expected) > 0.0001 {
			t.Errorf("ApplyEasing(%v, LINEAR) = %v, expected %v (clamped)",
				tt.progress, result, tt.expected)
		}
	}
}

func TestApplyEasing_UnknownType(t *testing.T) {
	// Unknown easing type should return linear (progress unchanged)
	got := ApplyEasing(0.5, "UNKNOWN")
	if got != 0.5 {
		t.Errorf("Unknown easing type should return progress unchanged, got %v", got)
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
		{"non-zero start", 50, 200, 0.5, EasingLinear, 125},
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

func TestParseEasingType(t *testing.T) {
	tests := []struct {
		input    string
		expected EasingType
	}{
		{"LINEAR", EasingLinear},
		{"EASE_IN_OUT_CUBIC", EasingInOutCubic},
		{"EASE_IN_OUT_SINE", EasingInOutSine},
		{"EASE_OUT_EXPONENTIAL", EasingOutExponential},
		{"BEZIER", EasingBezier},
		{"S_CURVE", EasingSCurve},
		{"unknown", DefaultEasing}, // Defaults
		{"", DefaultEasing},
	}

	for _, tt := range tests {
		result := ParseEasingType(tt.input)
		if result != tt.expected {
			t.Errorf("ParseEasingType(%q) = %v, expected %v", tt.input, result, tt.expected)
		}
	}
}

func TestEasingTypeString(t *testing.T) {
	tests := []struct {
		easing   EasingType
		expected string
	}{
		{EasingLinear, "LINEAR"},
		{EasingInOutCubic, "EASE_IN_OUT_CUBIC"},
		{EasingInOutSine, "EASE_IN_OUT_SINE"},
	}

	for _, tt := range tests {
		if tt.easing.String() != tt.expected {
			t.Errorf("EasingType.String() = %q, expected %q", tt.easing.String(), tt.expected)
		}
	}
}

func TestEasingTypeIsValid(t *testing.T) {
	validTypes := []EasingType{
		EasingLinear, EasingInOutCubic, EasingInOutSine,
		EasingOutExponential, EasingBezier, EasingSCurve,
	}
	for _, e := range validTypes {
		if !e.IsValid() {
			t.Errorf("EasingType %q should be valid", e)
		}
	}

	invalidTypes := []EasingType{"INVALID", "", "linear"}
	for _, e := range invalidTypes {
		if e.IsValid() {
			t.Errorf("EasingType %q should be invalid", e)
		}
	}
}
