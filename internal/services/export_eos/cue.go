package exporteos

import "fmt"

// CueListOut configures one cue list emission.
type CueListOut struct {
	Number int
	Label  string
	Cues   []CueOut
}

// CueOut configures one cue emission.
type CueOut struct {
	Number      string
	Part        int
	Label       string
	UnicodeText *string
	UpFade      float64
	DownFade    float64
	Follow      *float64
	Hang        *float64
	Block       bool
	IntBlock    bool
	ChanMoves   []ChanMoveOut
	ParamMoves  []ParamMoveOut
}

func (w *writer) writeCueList(cl CueListOut) {
	w.line(fmt.Sprintf("$CueList %d", cl.Number))
	if cl.Label != "" {
		w.line("   Text " + cl.Label)
	}
	w.blank()
	for _, c := range cl.Cues {
		w.writeCue(cl.Number, c)
		w.blank()
	}
}

func (w *writer) writeCue(listNum int, c CueOut) {
	num := c.Number
	if c.Part > 0 {
		num = fmt.Sprintf("%s/%d", c.Number, c.Part)
	}
	w.line(fmt.Sprintf("Cue %s %d", num, listNum))
	if c.Label != "" {
		w.line("   Text " + c.Label)
	}
	if c.UnicodeText != nil {
		w.line("   $$UText " + encodeUText(*c.UnicodeText))
	}
	if c.UpFade > 0 {
		w.line(fmt.Sprintf("   Up %g", c.UpFade))
		w.line(fmt.Sprintf("   $$TimeUp %g 0 1 1", c.UpFade))
	}
	if c.DownFade > 0 {
		w.line(fmt.Sprintf("   Down %g", c.DownFade))
		w.line(fmt.Sprintf("   $$TimeDown %g 0 1 1", c.DownFade))
	}
	if c.Follow != nil {
		w.line(fmt.Sprintf("   $$Follow %g", *c.Follow))
	}
	if c.Hang != nil {
		w.line(fmt.Sprintf("   $$Hang %g", *c.Hang))
	}
	if c.Block {
		w.line("   $$Block")
	}
	if c.IntBlock {
		w.line("   $$IntBlock")
	}
	if len(c.ChanMoves) > 0 {
		w.line("   $$ChanMove " + formatChanMoves(c.ChanMoves))
	}
	for _, pm := range c.ParamMoves {
		w.line("   $$Param " + formatParamMove(pm))
	}
}
