package importeos

import (
	"context"
	"testing"

	"github.com/bbernstein/lacylights-go/internal/database/models"
	"github.com/lucsky/cuid"
)

func TestMatcher_SynthesizedDefinitionIsSynthesizedSource(t *testing.T) {
	m := NewMatcher(&fakeFixtureRepo{}, NewParamTable(nil))
	res, _, err := m.Match(context.Background(), Personality{
		Manuf: "Acme", Model: "Thing", Channels: []PersChannel{{ParamID: 1, Offset: 1}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.SynthesizedDef == nil || res.SynthesizedDef.Source != models.FixtureSourceSynthesized {
		t.Fatalf("synthesized def source = %+v", res.SynthesizedDef)
	}
}

// seedDimmer creates a one-channel INTENSITY definition from source src.
func seedDimmer(t *testing.T, d *testDeps, src string, chType string) string {
	t.Helper()
	ctx := context.Background()
	def := &models.FixtureDefinition{Manufacturer: "Generic", Model: "Dimmer", Type: "DIMMER", Source: src}
	if err := d.fixtureRepo.CreateDefinition(ctx, def); err != nil {
		t.Fatal(err)
	}
	ch := models.ChannelDefinition{ID: cuid.New(), Name: "Intensity", Type: chType, Offset: 0, MaxValue: 255, FadeBehavior: "FADE", DefinitionID: def.ID}
	if err := d.db.Create(&ch).Error; err != nil {
		t.Fatal(err)
	}
	return def.ID
}

func TestRepoAdapter_PrefersHigherPrecedenceSource(t *testing.T) {
	d := newTestDeps(t)
	ctx := context.Background()
	synthID := seedDimmer(t, d, models.FixtureSourceSynthesized, "INTENSITY")
	oflID := seedDimmer(t, d, models.FixtureSourceOFL, "INTENSITY")

	got, err := newRepoAdapter(d.fixtureRepo).FindMatchingDefinition(ctx, "Generic", "Dimmer", []int{1})
	if err != nil {
		t.Fatal(err)
	}
	if got == nil || got.ID != oflID {
		t.Fatalf("matched %+v, want OFL %s (not synthesized %s)", got, oflID, synthID)
	}
}

func TestRepoAdapter_SkipsCandidatesWhoseChannelsDiffer(t *testing.T) {
	d := newTestDeps(t)
	ctx := context.Background()
	seedDimmer(t, d, models.FixtureSourceUser, "RED") // higher precedence, wrong layout
	oflID := seedDimmer(t, d, models.FixtureSourceOFL, "INTENSITY")

	got, err := newRepoAdapter(d.fixtureRepo).FindMatchingDefinition(ctx, "Generic", "Dimmer", []int{1})
	if err != nil {
		t.Fatal(err)
	}
	if got == nil || got.ID != oflID {
		t.Fatalf("matched %+v, want OFL %s", got, oflID)
	}
}
