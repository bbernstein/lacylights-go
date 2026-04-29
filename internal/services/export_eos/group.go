package exporteos

import "fmt"

// GroupOut configures one group emission.
type GroupOut struct {
	Number      string
	Label       string
	UnicodeText *string
	Channels    []int
}

func (w *writer) writeGroup(g GroupOut) {
	w.line(fmt.Sprintf("$Group %s", g.Number))
	if g.Label != "" {
		w.line("   Text " + sanitizeASCII(g.Label))
	}
	if g.UnicodeText != nil {
		w.line("   $$UText " + encodeUText(*g.UnicodeText))
	}
	for _, ch := range g.Channels {
		w.line(fmt.Sprintf("   %d", ch))
	}
}
