package modulator

// TransitionBehavior determines what happens to an effect when a cue change occurs.
type TransitionBehavior string

const (
	// TransitionFadeOut means the effect fades intensity to 0 over the cue's fadeInTime.
	// This is the default behavior for most effects.
	TransitionFadeOut TransitionBehavior = "FADE_OUT"

	// TransitionPersist means the effect continues unchanged until explicitly stopped.
	// Useful for ambient effects and blackout.
	TransitionPersist TransitionBehavior = "PERSIST"

	// TransitionSnapOff means the effect stops immediately when cue changes.
	// Useful for strobe effects and precise timing.
	TransitionSnapOff TransitionBehavior = "SNAP_OFF"

	// TransitionCrossfadeParams means the effect parameters smoothly transition to new values.
	// Used when the same effect continues but with different intensity/speed.
	TransitionCrossfadeParams TransitionBehavior = "CROSSFADE_PARAMS"
)

// String returns the string representation of the transition behavior.
func (t TransitionBehavior) String() string {
	return string(t)
}

// ParseTransitionBehavior parses a string into a TransitionBehavior.
func ParseTransitionBehavior(s string) TransitionBehavior {
	switch s {
	case "FADE_OUT":
		return TransitionFadeOut
	case "PERSIST":
		return TransitionPersist
	case "SNAP_OFF":
		return TransitionSnapOff
	case "CROSSFADE_PARAMS":
		return TransitionCrossfadeParams
	default:
		return TransitionFadeOut
	}
}

// IsValid returns true if the transition behavior is a known valid behavior.
func (t TransitionBehavior) IsValid() bool {
	switch t {
	case TransitionFadeOut, TransitionPersist, TransitionSnapOff, TransitionCrossfadeParams:
		return true
	default:
		return false
	}
}

// RequiresFade returns true if this transition behavior involves a fade animation.
func (t TransitionBehavior) RequiresFade() bool {
	switch t {
	case TransitionFadeOut, TransitionCrossfadeParams:
		return true
	default:
		return false
	}
}

// IsImmediate returns true if this transition behavior takes effect immediately.
func (t TransitionBehavior) IsImmediate() bool {
	switch t {
	case TransitionSnapOff:
		return true
	default:
		return false
	}
}
