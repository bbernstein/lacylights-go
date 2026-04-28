package importeos

import (
	"os"
	"path/filepath"
	"testing"
)

func parseFixture(t *testing.T, name string) *Show {
	t.Helper()
	f, err := os.Open(filepath.Join("testdata", name))
	if err != nil {
		t.Fatalf("open fixture: %v", err)
	}
	defer func() { _ = f.Close() }()
	show, _, err := Parse(f)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	return show
}

func TestParse_MinimalPatch(t *testing.T) {
	show := parseFixture(t, "minimal_patch.asc")

	if show.Ident != "3:0" {
		t.Errorf("ident: got %q, want %q", show.Ident, "3:0")
	}
	if show.Manufacturer != "ETC" {
		t.Errorf("manufacturer: got %q, want %q", show.Manufacturer, "ETC")
	}
	if show.Console != "Eos" {
		t.Errorf("console: got %q", show.Console)
	}
	if show.Format != "3.20" {
		t.Errorf("format: got %q", show.Format)
	}
	if show.Title != "Minimal Patch Test" {
		t.Errorf("title: got %q", show.Title)
	}
	if len(show.ParamTypes) != 1 || show.ParamTypes[0].ID != 1 || show.ParamTypes[0].LongName != "Intens" {
		t.Errorf("param types: got %+v", show.ParamTypes)
	}
	if len(show.Personalities) != 1 || show.Personalities[0].ID != 9001 ||
		show.Personalities[0].Manuf != "Generic" || show.Personalities[0].Model != "Dimmer" {
		t.Errorf("personalities: got %+v", show.Personalities)
	}
	if len(show.Patch) != 2 {
		t.Fatalf("patch entries: got %d, want 2", len(show.Patch))
	}
	if show.Patch[0].Channel != 1 || show.Patch[0].AddressRaw != "1" || show.Patch[0].PersonalityID != 9001 ||
		show.Patch[0].Label != "Cyc R" {
		t.Errorf("patch[0]: got %+v", show.Patch[0])
	}
	if show.Patch[1].Label != "Cyc L" {
		t.Errorf("patch[1] label: got %q", show.Patch[1].Label)
	}
}

func TestParse_BasicCues(t *testing.T) {
	show := parseFixture(t, "basic_cues.asc")

	if len(show.CueLists) < 1 {
		t.Fatalf("cue lists: got %d, want >=1", len(show.CueLists))
	}
	cl := show.CueLists[0]
	if cl.Number != 1 || cl.Label != "Main" {
		t.Errorf("cue list: got number=%d label=%q", cl.Number, cl.Label)
	}
	if len(cl.Cues) < 2 {
		t.Fatalf("cues: got %d, want >=2", len(cl.Cues))
	}

	c0 := cl.Cues[0]
	if c0.Number != "1" || c0.Label != "Open" || c0.UpFade != 5 || c0.DownFade != 5 {
		t.Errorf("cue 0: got %+v", c0)
	}
	if len(c0.ChanMoves) != 2 {
		t.Fatalf("cue 0 chan moves: got %d", len(c0.ChanMoves))
	}
	if c0.ChanMoves[0].Channel != 1 || c0.ChanMoves[0].Value != 0xFF {
		t.Errorf("cue 0 chan move 0: got %+v", c0.ChanMoves[0])
	}
	if c0.ChanMoves[1].Channel != 2 || c0.ChanMoves[1].Value != 0x80 {
		t.Errorf("cue 0 chan move 1: got %+v", c0.ChanMoves[1])
	}

	c1 := cl.Cues[1]
	if c1.Number != "2" || c1.Follow == nil || *c1.Follow != 2 {
		t.Errorf("cue 1 follow: got %+v", c1.Follow)
	}
}

func TestParse_CuePartsAndQualified(t *testing.T) {
	show := parseFixture(t, "basic_cues.asc")
	if len(show.CueLists) != 2 {
		t.Fatalf("cue lists: got %d, want 2", len(show.CueLists))
	}
	var found *Cue
	for i := range show.CueLists[0].Cues {
		if show.CueLists[0].Cues[i].Number == "5" && show.CueLists[0].Cues[i].Part == 2 {
			found = &show.CueLists[0].Cues[i]
		}
	}
	if found == nil {
		t.Fatalf("expected cue 5 part 2 in list 1, got %+v", show.CueLists[0].Cues)
	}
	if found.Label != "Part2" {
		t.Errorf("part 2 label: got %q", found.Label)
	}

	if show.CueLists[1].Number != 2 || len(show.CueLists[1].Cues) != 1 {
		t.Fatalf("list 2 cues: got %+v", show.CueLists[1])
	}
	if show.CueLists[1].Cues[0].Number != "1.5" || show.CueLists[1].Cues[0].Label != "Backup A" {
		t.Errorf("list 2 cue 0: got %+v", show.CueLists[1].Cues[0])
	}
}

func TestParse_Palettes(t *testing.T) {
	show := parseFixture(t, "palettes_color_focus.asc")
	if len(show.ColorPalettes) != 2 {
		t.Fatalf("color palettes: got %d, want 2", len(show.ColorPalettes))
	}
	cp1 := show.ColorPalettes[0]
	if cp1.Number != "1" || cp1.Label != "Red" {
		t.Errorf("color palette 1: got %+v", cp1)
	}
	if len(cp1.ParamMoves) != 1 || cp1.ParamMoves[0].Channel != 1 ||
		len(cp1.ParamMoves[0].Values) != 3 {
		t.Errorf("color palette 1 params: got %+v", cp1.ParamMoves)
	}
	if len(show.FocusPalettes) != 1 || show.FocusPalettes[0].Label != "Center" {
		t.Errorf("focus palettes: got %+v", show.FocusPalettes)
	}
}

func TestParse_PresetsAndGroups(t *testing.T) {
	show := parseFixture(t, "presets_groups.asc")
	if len(show.Presets) != 1 || show.Presets[0].Label != "Half Up" {
		t.Fatalf("presets: got %+v", show.Presets)
	}
	if len(show.Presets[0].ChanMoves) != 3 {
		t.Errorf("preset chan moves: got %d, want 3", len(show.Presets[0].ChanMoves))
	}
	if len(show.Groups) != 2 {
		t.Fatalf("groups: got %d, want 2", len(show.Groups))
	}
	g1 := show.Groups[0]
	if g1.Label != "Front Wash" || len(g1.Channels) != 2 || g1.Channels[0] != 1 || g1.Channels[1] != 2 {
		t.Errorf("group 1: got %+v", g1)
	}
	g2 := show.Groups[1]
	if len(g2.Channels) != 3 || g2.Channels[0] != 1 || g2.Channels[2] != 3 {
		t.Errorf("group 2 thru: got %+v", g2)
	}
}
