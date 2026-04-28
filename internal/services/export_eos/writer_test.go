package exporteos

import (
	"strings"
	"testing"
)

func TestWriter_HeaderSection(t *testing.T) {
	w := newWriter(WriterOptions{Title: "My Show", FormatVersion: "3.20"})
	w.writeHeader()
	got := w.String()
	for _, want := range []string{
		"Ident 3:0\n",
		"Manufacturer ETC\n",
		"Console Eos\n",
		"$$Format 3.20\n",
		"$$Title My Show\n",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in header:\n%s", want, got)
		}
	}
}

func TestWriter_RoundtripsPaletteAndCue(t *testing.T) {
	w := newWriter(WriterOptions{Title: "X"})
	w.writePalette(PaletteOut{
		Kind: "ColorPalette", Number: "1", Label: "Red",
		ParamMoves: []ParamMoveOut{{Channel: 1, Values: []ParamValueOut{{ParamID: 12, Value: 255}}}},
	})
	w.writeCue(1, CueOut{
		Number: "1", Label: "Open", UpFade: 5, DownFade: 5,
		ChanMoves: []ChanMoveOut{{Channel: 1, Value: 0xFF}},
	})
	got := w.String()
	for _, want := range []string{
		"$ColorPalette 1\n",
		"   Text Red\n",
		"   $$Param 1 12@255\n",
		"Cue 1 1\n",
		"   Up 5\n",
		"   $$ChanMove 1@HFF\n",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in:\n%s", want, got)
		}
	}
}

func TestWriter_GroupAndUText(t *testing.T) {
	w := newWriter(WriterOptions{Title: "X"})
	cafe := "Café"
	w.writeGroup(GroupOut{
		Number: "1", Label: "Front Wash", UnicodeText: &cafe,
		Channels: []int{1, 2, 3},
	})
	got := w.String()
	for _, want := range []string{
		"$Group 1\n",
		"   Text Front Wash\n",
		"   $$UText 4300 6100 6600 E900\n",
		"   1\n",
		"   2\n",
		"   3\n",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in:\n%s", want, got)
		}
	}
}

func TestWriter_CueListWrapsCues(t *testing.T) {
	w := newWriter(WriterOptions{Title: "X"})
	w.writeCueList(CueListOut{
		Number: 1, Label: "Main",
		Cues: []CueOut{
			{Number: "1", Label: "Open", UpFade: 5},
		},
	})
	got := w.String()
	for _, want := range []string{
		"$CueList 1\n",
		"   Text Main\n",
		"Cue 1 1\n",
		"   Text Open\n",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in:\n%s", want, got)
		}
	}
}

func TestWriter_SidecarSection(t *testing.T) {
	w := newWriter(WriterOptions{Title: "X"})
	if err := w.writeSidecar(SidecarOut{
		Version:    1,
		LookBoards: nil,
	}); err != nil {
		t.Fatal(err)
	}
	got := w.String()
	if !strings.Contains(got, "$$ LACYLIGHTS:version 1\n") {
		t.Errorf("missing sidecar version line:\n%s", got)
	}
	if !strings.Contains(got, "! LacyLights round-trip metadata") {
		t.Errorf("missing sidecar comment banner:\n%s", got)
	}
}

func TestWriter_PatchAndPersonality(t *testing.T) {
	w := newWriter(WriterOptions{Title: "T"})
	w.writePersonality(PersonalityIn{
		ID: 9001, Manuf: "Generic", Model: "Dimmer", Footprint: 1,
		Channels: []PersonalityChannelIn{{ParamID: 1, Size: 1, Offset: 1}},
	})
	w.writePatch([]PatchEntryOut{
		{Channel: 1, Address: "1", PersonalityID: 9001, Label: "Front"},
		{Channel: 2, Address: "2", PersonalityID: 9001},
	})
	got := w.String()
	for _, want := range []string{
		"$Personality 9001\n",
		"   $$Manuf Generic\n",
		"   $$PersChan",
		"$Patch 1 1 9001\n",
		"   Text Front\n",
		"$Patch 2 2 9001\n",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in:\n%s", want, got)
		}
	}
}
