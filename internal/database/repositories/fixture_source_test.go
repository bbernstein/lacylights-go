package repositories

import (
	"context"
	"reflect"
	"testing"
	"time"

	"github.com/bbernstein/lacylights-go/internal/database/models"
)

func TestParseFixtureSourcePrecedence(t *testing.T) {
	def := models.DefaultFixtureSourcePrecedence
	cases := []struct {
		name    string
		value   string
		want    []string
		wantErr bool
	}{
		{"empty uses default", "", def, false},
		{"full order", `["OFL","GDTF","USER","BUILTIN","EOS","SYNTHESIZED"]`,
			[]string{"OFL", "GDTF", "USER", "BUILTIN", "EOS", "SYNTHESIZED"}, false},
		{"partial order appends the rest in default order", `["OFL"]`,
			[]string{"OFL", "USER", "GDTF", "BUILTIN", "EOS", "SYNTHESIZED"}, false},
		{"unknown source", `["OFL","NOPE"]`, nil, true},
		{"duplicate source", `["OFL","OFL"]`, nil, true},
		{"not json", `OFL,GDTF`, nil, true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := ParseFixtureSourcePrecedence(c.value)
			if (err != nil) != c.wantErr {
				t.Fatalf("err = %v, wantErr %v", err, c.wantErr)
			}
			if !c.wantErr && !reflect.DeepEqual(got, c.want) {
				t.Errorf("got %v, want %v", got, c.want)
			}
		})
	}
}

func TestFixtureSourcePrecedence_LoadAndSave(t *testing.T) {
	tdb, cleanup := setupTestDB(t)
	defer cleanup()
	ctx := context.Background()
	repo := NewFixtureRepository(tdb.DB)

	got, err := repo.FixtureSourcePrecedence(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, models.DefaultFixtureSourcePrecedence) {
		t.Errorf("default precedence = %v", got)
	}

	saved, err := repo.SetFixtureSourcePrecedence(ctx, []string{"OFL", "USER"})
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"OFL", "USER", "GDTF", "BUILTIN", "EOS", "SYNTHESIZED"}
	if !reflect.DeepEqual(saved, want) {
		t.Errorf("saved = %v, want %v", saved, want)
	}
	got, err = repo.FixtureSourcePrecedence(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("reloaded = %v, want %v", got, want)
	}

	if _, err := repo.SetFixtureSourcePrecedence(ctx, []string{"BOGUS"}); err == nil {
		t.Error("expected error for unknown source")
	}
}

func TestFixtureSourcePrecedence_CorruptSettingFallsBackToDefault(t *testing.T) {
	tdb, cleanup := setupTestDB(t)
	defer cleanup()
	ctx := context.Background()
	if _, err := NewSettingRepository(tdb.DB).Upsert(ctx, FixtureSourcePrecedenceSettingKey, "not json"); err != nil {
		t.Fatal(err)
	}
	got, err := NewFixtureRepository(tdb.DB).FixtureSourcePrecedence(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, models.DefaultFixtureSourcePrecedence) {
		t.Errorf("got %v, want default", got)
	}
}

func TestCreateDefinition_DefaultsSourceToUser(t *testing.T) {
	tdb, cleanup := setupTestDB(t)
	defer cleanup()
	ctx := context.Background()
	repo := NewFixtureRepository(tdb.DB)
	def := &models.FixtureDefinition{Manufacturer: "M", Model: "X", Type: "OTHER"}
	if err := repo.CreateDefinition(ctx, def); err != nil {
		t.Fatal(err)
	}
	if def.Source != models.FixtureSourceUser {
		t.Errorf("in-memory Source = %q, want USER", def.Source)
	}
	found, err := repo.FindDefinitionByID(ctx, def.ID)
	if err != nil || found == nil {
		t.Fatalf("find: %v %v", found, err)
	}
	if found.Source != models.FixtureSourceUser {
		t.Errorf("stored Source = %q, want USER", found.Source)
	}
}

func seedSourceDefs(t *testing.T, repo *FixtureRepository, sources ...string) map[string]string {
	t.Helper()
	ids := map[string]string{}
	for _, src := range sources {
		def := &models.FixtureDefinition{Manufacturer: "Acme", Model: "Par", Type: "LED_PAR", Source: src}
		if err := repo.CreateDefinition(context.Background(), def); err != nil {
			t.Fatal(err)
		}
		ids[src] = def.ID
	}
	return ids
}

func TestFindDefinitionsByManufacturerModel_OrdersByPrecedence(t *testing.T) {
	tdb, cleanup := setupTestDB(t)
	defer cleanup()
	ctx := context.Background()
	repo := NewFixtureRepository(tdb.DB)
	ids := seedSourceDefs(t, repo, models.FixtureSourceSynthesized, models.FixtureSourceOFL, models.FixtureSourceGDTF)

	got, err := repo.FindDefinitionsByManufacturerModel(ctx, "Acme", "Par")
	if err != nil {
		t.Fatal(err)
	}
	var order []string
	for _, d := range got {
		order = append(order, d.Source)
	}
	if want := []string{"GDTF", "OFL", "SYNTHESIZED"}; !reflect.DeepEqual(order, want) {
		t.Errorf("order = %v, want %v", order, want)
	}

	if _, err := repo.SetFixtureSourcePrecedence(ctx, []string{"SYNTHESIZED", "OFL"}); err != nil {
		t.Fatal(err)
	}
	got, err = repo.FindDefinitionsByManufacturerModel(ctx, "Acme", "Par")
	if err != nil {
		t.Fatal(err)
	}
	if got[0].ID != ids[models.FixtureSourceSynthesized] || got[1].ID != ids[models.FixtureSourceOFL] {
		t.Errorf("custom order not applied: %v, %v", got[0].Source, got[1].Source)
	}

	none, err := repo.FindDefinitionsByManufacturerModel(ctx, "Acme", "Nope")
	if err != nil || len(none) != 0 {
		t.Errorf("expected no results, got %v %v", none, err)
	}
}

func TestFindDefinitionBySourceManufacturerModel(t *testing.T) {
	tdb, cleanup := setupTestDB(t)
	defer cleanup()
	ctx := context.Background()
	repo := NewFixtureRepository(tdb.DB)
	ids := seedSourceDefs(t, repo, models.FixtureSourceUser, models.FixtureSourceOFL)

	got, err := repo.FindDefinitionBySourceManufacturerModel(ctx, models.FixtureSourceOFL, "Acme", "Par")
	if err != nil || got == nil || got.ID != ids[models.FixtureSourceOFL] {
		t.Fatalf("OFL lookup = %v, %v", got, err)
	}
	got, err = repo.FindDefinitionBySourceManufacturerModel(ctx, models.FixtureSourceGDTF, "Acme", "Par")
	if err != nil || got != nil {
		t.Errorf("GDTF lookup should be nil, got %v %v", got, err)
	}
}

func TestFindDefinitionBySourceID(t *testing.T) {
	tdb, cleanup := setupTestDB(t)
	defer cleanup()
	ctx := context.Background()
	repo := NewFixtureRepository(tdb.DB)
	sid := "8F54E11C-4C91-11E9-80BC-F1DFE217E634"
	def := &models.FixtureDefinition{Manufacturer: "Acme", Model: "Spot", Type: "MOVING_HEAD",
		Source: models.FixtureSourceGDTF, SourceID: &sid}
	if err := repo.CreateDefinition(ctx, def); err != nil {
		t.Fatal(err)
	}
	got, err := repo.FindDefinitionBySourceID(ctx, models.FixtureSourceGDTF, sid)
	if err != nil || got == nil || got.ID != def.ID {
		t.Fatalf("lookup = %v, %v", got, err)
	}
	got, err = repo.FindDefinitionBySourceID(ctx, models.FixtureSourceOFL, sid)
	if err != nil || got != nil {
		t.Errorf("wrong-source lookup should be nil, got %v %v", got, err)
	}
}

func TestSortDefinitionsByPrecedence_UnknownSourceLast(t *testing.T) {
	defs := []models.FixtureDefinition{
		{ID: "a", Source: "WEIRD"},
		{ID: "b", Source: models.FixtureSourceOFL},
		{ID: "c", Source: models.FixtureSourceUser},
	}
	SortDefinitionsByPrecedence(defs, models.DefaultFixtureSourcePrecedence)
	if defs[0].ID != "c" || defs[1].ID != "b" || defs[2].ID != "a" {
		t.Errorf("order = %s %s %s", defs[0].ID, defs[1].ID, defs[2].ID)
	}
}

func TestFindDefinitionsByManufacturerModel_TiesOrderByID(t *testing.T) {
	tdb, cleanup := setupTestDB(t)
	defer cleanup()
	ctx := context.Background()
	repo := NewFixtureRepository(tdb.DB)
	same := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	for _, id := range []string{"b-def", "a-def", "c-def"} {
		def := &models.FixtureDefinition{ID: id, Manufacturer: "Acme", Model: "Tie", Type: "LED_PAR", CreatedAt: same}
		if err := repo.CreateDefinition(ctx, def); err != nil {
			t.Fatal(err)
		}
	}
	got, err := repo.FindDefinitionsByManufacturerModel(ctx, "Acme", "Tie")
	if err != nil {
		t.Fatal(err)
	}
	var ids []string
	for _, d := range got {
		ids = append(ids, d.ID)
	}
	if want := []string{"a-def", "b-def", "c-def"}; !reflect.DeepEqual(ids, want) {
		t.Errorf("order = %v, want %v", ids, want)
	}
}
