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
