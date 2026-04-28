package importeos

import (
	"context"
	"slices"
	"testing"
)

type fakeFixtureRepo struct {
	defs []*FakeDef
}

func (f *fakeFixtureRepo) FindMatchingDefinition(_ context.Context, mfg, model string, paramIDs []int) (*FakeDef, error) {
	for _, d := range f.defs {
		if d.Manufacturer == mfg && d.Model == model && slices.Equal(d.ChannelParamIDs, paramIDs) {
			return d, nil
		}
	}
	return nil, nil
}

func TestMatcher_MatchesExisting(t *testing.T) {
	repo := &fakeFixtureRepo{defs: []*FakeDef{
		{ID: "def-1", Manufacturer: "Generic", Model: "Dimmer", ChannelParamIDs: []int{1}},
	}}
	m := NewMatcher(repo, NewParamTable(nil))
	pers := Personality{Manuf: "Generic", Model: "Dimmer", Channels: []PersChannel{{ParamID: 1}}}
	res, _, err := m.Match(context.Background(), pers)
	if err != nil {
		t.Fatal(err)
	}
	if res.ExistingDefinitionID != "def-1" || res.SynthesizedDef != nil {
		t.Errorf("expected match, got %+v", res)
	}
}

func TestMatcher_Synthesizes(t *testing.T) {
	repo := &fakeFixtureRepo{}
	m := NewMatcher(repo, NewParamTable([]ParamType{
		{ID: 1, LongName: "Intens"},
		{ID: 12, LongName: "Red"},
		{ID: 13, LongName: "Green"},
		{ID: 14, LongName: "Blue"},
	}))
	pers := Personality{
		Manuf: "Acme", Model: "QuadLED",
		Channels: []PersChannel{{ParamID: 1}, {ParamID: 12}, {ParamID: 13}, {ParamID: 14}},
	}
	res, warns, err := m.Match(context.Background(), pers)
	if err != nil {
		t.Fatal(err)
	}
	if res.SynthesizedDef == nil {
		t.Fatal("expected synthesized def")
	}
	if res.SynthesizedDef.Manufacturer != "Acme" || res.SynthesizedDef.Model != "QuadLED" {
		t.Errorf("synth name: got %s/%s", res.SynthesizedDef.Manufacturer, res.SynthesizedDef.Model)
	}
	if len(res.SynthesizedChannels) != 4 {
		t.Errorf("channels: got %d", len(res.SynthesizedChannels))
	}
	if len(warns) != 1 || warns[0].Code != WarnSynthesizedFixture {
		t.Errorf("warnings: got %+v", warns)
	}
}

func TestMatcher_SynthesizeNormalizesOffsetsToZeroBased(t *testing.T) {
	// Eos `$$PersChan` offsets are 1-based DMX offsets; LacyLights treats
	// them as 0-based (playback computes StartChannel + Offset). Synthesizing
	// must subtract one so imported fixtures play on the correct addresses.
	m := NewMatcher(&fakeFixtureRepo{}, NewParamTable([]ParamType{
		{ID: 1, LongName: "Intens"},
		{ID: 12, LongName: "Red"},
	}))
	pers := Personality{
		Manuf: "Acme", Model: "Dimmer",
		Channels: []PersChannel{
			{ParamID: 1, Offset: 1},  // Eos channel 1 -> LL offset 0
			{ParamID: 12, Offset: 3}, // Eos channel 3 -> LL offset 2
		},
	}
	res, _, err := m.Match(context.Background(), pers)
	if err != nil {
		t.Fatal(err)
	}
	if got := res.SynthesizedChannels[0].Offset; got != 0 {
		t.Errorf("first channel offset: got %d, want 0", got)
	}
	if got := res.SynthesizedChannels[1].Offset; got != 2 {
		t.Errorf("second channel offset: got %d, want 2", got)
	}
}

func TestMatcher_SynthesizeClampsZeroOffset(t *testing.T) {
	// Defensive: malformed Eos input may have Offset == 0 (invalid). Clamp to
	// 0 rather than producing a negative offset.
	m := NewMatcher(&fakeFixtureRepo{}, NewParamTable([]ParamType{
		{ID: 1, LongName: "Intens"},
	}))
	pers := Personality{
		Manuf: "Acme", Model: "Bad",
		Channels: []PersChannel{{ParamID: 1, Offset: 0}},
	}
	res, _, err := m.Match(context.Background(), pers)
	if err != nil {
		t.Fatal(err)
	}
	if got := res.SynthesizedChannels[0].Offset; got != 0 {
		t.Errorf("clamped offset: got %d, want 0", got)
	}
}
