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
