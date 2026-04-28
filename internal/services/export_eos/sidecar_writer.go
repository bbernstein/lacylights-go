package exporteos

import (
	"encoding/json"
	"fmt"

	importeos "github.com/bbernstein/lacylights-go/internal/services/import_eos"
)

// SidecarOut is the data we serialize at end-of-file.
type SidecarOut struct {
	Version       int
	LookBoards    []importeos.SidecarLookBoard
	FadeBehaviors []importeos.SidecarFadeBehavior
	SynthDefs     []importeos.SidecarSynthDef
}

func (w *writer) writeSidecar(sc SidecarOut) error {
	w.commentBanner("LacyLights round-trip metadata (ignored by ETC consoles)")
	w.line(fmt.Sprintf("$$ LACYLIGHTS:version %d", sc.Version))
	for _, lb := range sc.LookBoards {
		b, err := json.Marshal(lb)
		if err != nil {
			return err
		}
		w.line("$$ LACYLIGHTS:look_board " + string(b))
	}
	for _, fb := range sc.FadeBehaviors {
		b, err := json.Marshal(fb)
		if err != nil {
			return err
		}
		w.line("$$ LACYLIGHTS:fade_behavior " + string(b))
	}
	for _, sd := range sc.SynthDefs {
		b, err := json.Marshal(sd)
		if err != nil {
			return err
		}
		w.line("$$ LACYLIGHTS:synth_def " + string(b))
	}
	return nil
}
