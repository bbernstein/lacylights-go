package exporteos

import "fmt"

// PatchEntryOut is one $Patch line.
type PatchEntryOut struct {
	Channel       int
	Address       string // already normalized to EOS form
	PersonalityID int
	Label         string
}

func (w *writer) writePatch(entries []PatchEntryOut) {
	for _, e := range entries {
		w.line(fmt.Sprintf("$Patch %d %s %d", e.Channel, e.Address, e.PersonalityID))
		if e.Label != "" {
			w.line("   Text " + sanitizeASCII(e.Label))
		}
	}
	w.blank()
}
