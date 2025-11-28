// Package fade provides fade engine functionality for smooth DMX transitions.
package fade

import (
	"math"
)

// EasingType represents the type of easing function to use for fades.
type EasingType string

const (
	// EasingLinear provides constant rate of change.
	EasingLinear EasingType = "LINEAR"
	// EasingInOutCubic provides smooth acceleration and deceleration.
	EasingInOutCubic EasingType = "EASE_IN_OUT_CUBIC"
	// EasingInOutSine provides gentle sine wave easing.
	EasingInOutSine EasingType = "EASE_IN_OUT_SINE"
	// EasingOutExponential provides sharp start, smooth end.
	EasingOutExponential EasingType = "EASE_OUT_EXPONENTIAL"
	// EasingBezier provides bezier curve easing.
	EasingBezier EasingType = "BEZIER"
	// EasingSCurve provides sigmoid function easing.
	EasingSCurve EasingType = "S_CURVE"
)

// ApplyEasing applies an easing function to a progress value (0-1).
func ApplyEasing(progress float64, easingType EasingType) float64 {
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
		easingType = EasingInOutSine // Default easing
	}
	easedProgress := ApplyEasing(progress, easingType)
	return start + (end-start)*easedProgress
}
