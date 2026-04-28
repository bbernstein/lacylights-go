package importeos

// Snapshot is the fully resolved per-channel state for one cue.
type Snapshot struct {
	CueNumber     string
	CuePart       int
	ChannelLevels map[int]int         // channel → 0..255 (intensity)
	ParamLevels   map[int]map[int]int // channel → paramID → 0..255
	UpFade        float64
	UpDelay       float64
	DownFade      float64
	DownDelay     float64
	Follow        *float64
	Hang          *float64
	Block         bool
	IntBlock      bool
	Label         string
	UnicodeText   *string
}

// Tracker walks a cue list applying tracking semantics.
type Tracker struct{}

// NewTracker returns a fresh tracker.
func NewTracker() *Tracker { return &Tracker{} }

// ResolveCueList resolves each cue against cumulative state.
// Palette references in $$Param values are resolved against the supplied
// palette tables (currently unused since EOS exports inline values; reserved
// for future palette-ref syntax).
func (t *Tracker) ResolveCueList(
	cues []Cue,
	colorPalettes, beamPalettes, focusPalettes, intensPalettes, presets []Palette,
) []Snapshot {
	_ = colorPalettes
	_ = beamPalettes
	_ = focusPalettes
	_ = intensPalettes
	_ = presets
	chanState := make(map[int]int)
	paramState := make(map[int]map[int]int)
	out := make([]Snapshot, 0, len(cues))
	for _, cue := range cues {
		if cue.Block {
			chanState = make(map[int]int)
			paramState = make(map[int]map[int]int)
		} else if cue.IntBlock {
			chanState = make(map[int]int)
		}
		for _, m := range cue.ChanMoves {
			chanState[m.Channel] = m.Value
		}
		for _, pm := range cue.ParamMoves {
			ps, ok := paramState[pm.Channel]
			if !ok {
				ps = make(map[int]int)
				paramState[pm.Channel] = ps
			}
			for _, v := range pm.Values {
				ps[v.ParamID] = v.Value
			}
		}
		sn := Snapshot{
			CueNumber:     cue.Number,
			CuePart:       cue.Part,
			ChannelLevels: copyIntMap(chanState),
			ParamLevels:   copyParamMap(paramState),
			UpFade:        cue.UpFade,
			UpDelay:       cue.UpDelay,
			DownFade:      cue.DownFade,
			DownDelay:     cue.DownDelay,
			Follow:        cue.Follow,
			Hang:          cue.Hang,
			Block:         cue.Block,
			IntBlock:      cue.IntBlock,
			Label:         cue.Label,
			UnicodeText:   cue.UnicodeText,
		}
		out = append(out, sn)
	}
	return out
}

func copyIntMap(m map[int]int) map[int]int {
	out := make(map[int]int, len(m))
	for k, v := range m {
		out[k] = v
	}
	return out
}

func copyParamMap(m map[int]map[int]int) map[int]map[int]int {
	out := make(map[int]map[int]int, len(m))
	for ch, params := range m {
		out[ch] = copyIntMap(params)
	}
	return out
}
