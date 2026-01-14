package modulator

import (
	"math"
)

// EasingType represents the type of easing function to use for fades and transitions.
type EasingType string

const (
	// EasingLinear provides constant rate of change.
	EasingLinear EasingType = "LINEAR"

	// EasingInOutCubic provides smooth acceleration and deceleration.
	EasingInOutCubic EasingType = "EASE_IN_OUT_CUBIC"

	// EasingInOutSine provides gentle sine wave easing.
	// This is the default easing for crossfades.
	EasingInOutSine EasingType = "EASE_IN_OUT_SINE"

	// EasingOutExponential provides sharp start, smooth end.
	EasingOutExponential EasingType = "EASE_OUT_EXPONENTIAL"

	// EasingBezier provides bezier curve easing.
	EasingBezier EasingType = "BEZIER"

	// EasingSCurve provides sigmoid function easing.
	EasingSCurve EasingType = "S_CURVE"
)

// DefaultEasing is the default easing type used when none is specified.
const DefaultEasing = EasingInOutSine

// String returns the string representation of the easing type.
func (e EasingType) String() string {
	return string(e)
}

// ParseEasingType parses a string into an EasingType.
func ParseEasingType(s string) EasingType {
	switch s {
	case "LINEAR":
		return EasingLinear
	case "EASE_IN_OUT_CUBIC":
		return EasingInOutCubic
	case "EASE_IN_OUT_SINE":
		return EasingInOutSine
	case "EASE_OUT_EXPONENTIAL":
		return EasingOutExponential
	case "BEZIER":
		return EasingBezier
	case "S_CURVE":
		return EasingSCurve
	default:
		return DefaultEasing
	}
}

// IsValid returns true if the easing type is a known valid type.
func (e EasingType) IsValid() bool {
	switch e {
	case EasingLinear, EasingInOutCubic, EasingInOutSine,
		EasingOutExponential, EasingBezier, EasingSCurve:
		return true
	default:
		return false
	}
}

// ApplyEasing applies an easing function to a progress value (0-1).
// Returns a value in the range 0-1 that represents the eased progress.
func ApplyEasing(progress float64, easingType EasingType) float64 {
	// Clamp progress to 0-1
	if progress < 0 {
		progress = 0
	}
	if progress > 1 {
		progress = 1
	}

	switch easingType {
	case EasingLinear:
		return progress

	case EasingInOutCubic:
		if progress < 0.5 {
			return 4 * progress * progress * progress
		}
		temp := -2*progress + 2
		return 1 - temp*temp*temp/2

	case EasingInOutSine:
		return -(math.Cos(math.Pi*progress) - 1) / 2

	case EasingOutExponential:
		if progress == 1 {
			return 1
		}
		return 1 - math.Pow(2, -10*progress)

	case EasingBezier:
		// Standard ease-in-out bezier curve (0.42, 0, 0.58, 1)
		return cubicBezier(0.42, 0, 0.58, 1, progress)

	case EasingSCurve:
		// Sigmoid function normalized to 0-1 range
		k := 10.0 // Steepness factor
		return 1 / (1 + math.Exp(-k*(progress-0.5)))

	default:
		// Unknown easing type defaults to linear
		return progress
	}
}

// cubicBezier calculates the y value for a cubic bezier curve.
func cubicBezier(p1x, p1y, p2x, p2y, t float64) float64 {
	// Simplified cubic bezier implementation
	// For a more complete implementation, we'd use Newton-Raphson method
	_ = p1x // Control points x values not used in simplified implementation
	_ = p2x

	cy := 3 * p1y
	by := 3*(p2y-p1y) - cy
	ay := 1 - cy - by

	tSquared := t * t
	tCubed := tSquared * t

	return ay*tCubed + by*tSquared + cy*t
}

// Interpolate calculates an interpolated value between start and end.
func Interpolate(start, end, progress float64, easingType EasingType) float64 {
	if easingType == "" {
		easingType = DefaultEasing
	}
	easedProgress := ApplyEasing(progress, easingType)
	return start + (end-start)*easedProgress
}
