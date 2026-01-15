// Package fade provides fade engine functionality for smooth DMX transitions.
// Easing functions are delegated to the modulator package for shared use.
package fade

import (
	"github.com/bbernstein/lacylights-go/internal/services/modulator"
)

// EasingType represents the type of easing function to use for fades.
// This is an alias to the modulator package's EasingType for backward compatibility.
type EasingType = modulator.EasingType

// Easing type constants - re-exported from modulator for backward compatibility.
const (
	// EasingLinear provides constant rate of change.
	EasingLinear EasingType = modulator.EasingLinear
	// EasingInOutCubic provides smooth acceleration and deceleration.
	EasingInOutCubic EasingType = modulator.EasingInOutCubic
	// EasingInOutSine provides gentle sine wave easing.
	EasingInOutSine EasingType = modulator.EasingInOutSine
	// EasingOutExponential provides sharp start, smooth end.
	EasingOutExponential EasingType = modulator.EasingOutExponential
	// EasingBezier provides bezier curve easing.
	EasingBezier EasingType = modulator.EasingBezier
	// EasingSCurve provides sigmoid function easing.
	EasingSCurve EasingType = modulator.EasingSCurve
)

// ApplyEasing applies an easing function to a progress value (0-1).
// Delegates to the modulator package.
func ApplyEasing(progress float64, easingType EasingType) float64 {
	return modulator.ApplyEasing(progress, easingType)
}

// Interpolate calculates an interpolated value between start and end.
// Delegates to the modulator package.
func Interpolate(start, end, progress float64, easingType EasingType) float64 {
	return modulator.Interpolate(start, end, progress, easingType)
}
