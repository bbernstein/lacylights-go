package modulator

// PriorityBand represents the major priority category of an effect.
// Higher bands always override lower bands for the same channel.
type PriorityBand int

const (
	// PriorityBandBase is the lowest priority: static look values, default state.
	PriorityBandBase PriorityBand = 0

	// PriorityBandUser is for user-activated effects: chase, pulse, color cycle.
	PriorityBandUser PriorityBand = 1

	// PriorityBandCue is for cue transitions: crossfades between looks.
	PriorityBandCue PriorityBand = 2

	// PriorityBandSystem is the highest priority: blackout, grand master, emergency.
	PriorityBandSystem PriorityBand = 3
)

// String returns the string representation of the priority band.
func (p PriorityBand) String() string {
	switch p {
	case PriorityBandBase:
		return "BASE"
	case PriorityBandUser:
		return "USER"
	case PriorityBandCue:
		return "CUE"
	case PriorityBandSystem:
		return "SYSTEM"
	default:
		return "UNKNOWN"
	}
}

// ParsePriorityBand parses a string into a PriorityBand.
func ParsePriorityBand(s string) PriorityBand {
	switch s {
	case "BASE":
		return PriorityBandBase
	case "USER":
		return PriorityBandUser
	case "CUE":
		return PriorityBandCue
	case "SYSTEM":
		return PriorityBandSystem
	default:
		return PriorityBandBase
	}
}

// EffectPriority represents the complete priority of an effect,
// consisting of a band and a sub-priority within that band.
type EffectPriority struct {
	Band        PriorityBand
	SubPriority int // 0-100, higher values have higher priority within the band
}

// NewEffectPriority creates a new EffectPriority with the given band and sub-priority.
// SubPriority is clamped to 0-100.
func NewEffectPriority(band PriorityBand, subPriority int) EffectPriority {
	if subPriority < 0 {
		subPriority = 0
	}
	if subPriority > 100 {
		subPriority = 100
	}
	return EffectPriority{
		Band:        band,
		SubPriority: subPriority,
	}
}

// Compare compares this priority with another.
// Returns:
//
//	-1 if this priority is lower than other
//	 0 if priorities are equal
//	 1 if this priority is higher than other
func (p EffectPriority) Compare(other EffectPriority) int {
	// First compare bands
	if p.Band < other.Band {
		return -1
	}
	if p.Band > other.Band {
		return 1
	}

	// Bands are equal, compare sub-priority
	if p.SubPriority < other.SubPriority {
		return -1
	}
	if p.SubPriority > other.SubPriority {
		return 1
	}

	return 0
}

// Less returns true if this priority is lower than other.
// Used for sorting effects by priority (lowest first).
func (p EffectPriority) Less(other EffectPriority) bool {
	return p.Compare(other) < 0
}

// Equal returns true if this priority equals other.
func (p EffectPriority) Equal(other EffectPriority) bool {
	return p.Compare(other) == 0
}

// Greater returns true if this priority is higher than other.
func (p EffectPriority) Greater(other EffectPriority) bool {
	return p.Compare(other) > 0
}

// Default priorities for common use cases
var (
	// PriorityUserDefault is the default priority for user-activated effects.
	PriorityUserDefault = NewEffectPriority(PriorityBandUser, 50)

	// PriorityCueDefault is the default priority for cue crossfades.
	PriorityCueDefault = NewEffectPriority(PriorityBandCue, 50)

	// PriorityBlackout is the priority for system blackout (highest).
	PriorityBlackout = NewEffectPriority(PriorityBandSystem, 100)

	// PriorityGrandMaster is the priority for grand master.
	PriorityGrandMaster = NewEffectPriority(PriorityBandSystem, 50)
)
