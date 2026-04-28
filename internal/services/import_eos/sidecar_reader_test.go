package importeos

import "testing"

func TestReadSidecar_Roundtrip(t *testing.T) {
	show := parseFixture(t, "sidecar_roundtrip.asc")
	if len(show.SidecarLines) != 4 {
		t.Fatalf("sidecar lines: got %d, want 4", len(show.SidecarLines))
	}
	warn := &Collector{}
	sc := ReadSidecar(show.SidecarLines, warn)
	if sc.Version != 1 {
		t.Errorf("version: got %d", sc.Version)
	}
	if len(sc.LookBoards) != 1 || sc.LookBoards[0].RefID != "board-a" ||
		len(sc.LookBoards[0].Buttons) != 1 ||
		sc.LookBoards[0].Buttons[0].LookRefID != "L1" {
		t.Errorf("look board: got %+v", sc.LookBoards)
	}
	if len(sc.FadeBehaviors) != 1 || sc.FadeBehaviors[0].InstanceRefID != "fix-1" ||
		sc.FadeBehaviors[0].Channels[0].Behavior != "SNAP_END" {
		t.Errorf("fade behavior: got %+v", sc.FadeBehaviors)
	}
	if len(sc.SynthDefs) != 1 || sc.SynthDefs[0].DefRefID != "def-1" {
		t.Errorf("synth def: got %+v", sc.SynthDefs)
	}
	if len(warn.All()) != 0 {
		t.Errorf("unexpected warnings: %+v", warn.All())
	}
}

func TestReadSidecar_BadJSON(t *testing.T) {
	warn := &Collector{}
	sc := ReadSidecar([]string{`$$ LACYLIGHTS:look_board {invalid json}`}, warn)
	if len(sc.LookBoards) != 0 {
		t.Error("expected no look boards from bad JSON")
	}
	if len(warn.All()) != 1 || warn.All()[0].Code != WarnSidecarInvalid {
		t.Errorf("expected SIDECAR_INVALID warning, got %+v", warn.All())
	}
}
