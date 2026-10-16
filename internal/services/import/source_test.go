package importservice

import (
	"context"
	"testing"

	"github.com/bbernstein/lacylights-go/internal/database/models"
	"github.com/bbernstein/lacylights-go/internal/database/repositories"
	"github.com/bbernstein/lacylights-go/internal/services/export"
)

func newSourceTestService(t *testing.T) (*Service, *repositories.FixtureRepository, func()) {
	t.Helper()
	db, cleanup := setupTestDB(t)
	fixtureRepo := repositories.NewFixtureRepository(db)
	svc := NewService(repositories.NewProjectRepository(db), fixtureRepo, repositories.NewLookRepository(db),
		repositories.NewCueListRepository(db), repositories.NewCueRepository(db))
	return svc, fixtureRepo, cleanup
}

func exportWithDef(def export.ExportedFixtureDefinition) string {
	def.RefID = "def-x"
	def.Manufacturer = "TestMfg"
	def.Model = "TestModel"
	def.Type = "DIMMER"
	def.Channels = []export.ExportedChannelDefinition{{RefID: "ch-1", Name: "Dimmer", Type: "INTENSITY", MaxValue: 255}}
	p := &export.ExportedProject{
		Version:            "1.0",
		Project:            &export.ExportProjectInfo{OriginalID: "p", Name: "Source Import"},
		FixtureDefinitions: []export.ExportedFixtureDefinition{def},
	}
	js, _ := p.ToJSON()
	return js
}

func TestImportProject_CreatesDefinitionWithExportedSource(t *testing.T) {
	ctx := context.Background()
	sid, ver := "GUID-1", "1.2"
	cases := []struct {
		name string
		def  export.ExportedFixtureDefinition
		want string
	}{
		{"gdtf", export.ExportedFixtureDefinition{Source: models.FixtureSourceGDTF, SourceID: &sid, SourceVersion: &ver}, models.FixtureSourceGDTF},
		{"missing source", export.ExportedFixtureDefinition{}, models.FixtureSourceUser},
		{"unknown source", export.ExportedFixtureDefinition{Source: "BOGUS"}, models.FixtureSourceUser},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			svc, repo, cleanup := newSourceTestService(t)
			defer cleanup()
			if _, _, _, err := svc.ImportProject(ctx, exportWithDef(c.def), ImportOptions{Mode: ImportModeCreate}); err != nil {
				t.Fatal(err)
			}
			defs, err := repo.FindDefinitionsByManufacturerModel(ctx, "TestMfg", "TestModel")
			if err != nil || len(defs) != 1 {
				t.Fatalf("defs = %v, %v", defs, err)
			}
			if defs[0].Source != c.want {
				t.Errorf("source = %q, want %q", defs[0].Source, c.want)
			}
			if c.want == models.FixtureSourceGDTF && (defs[0].SourceID == nil || *defs[0].SourceID != sid) {
				t.Errorf("sourceId = %v", defs[0].SourceID)
			}
		})
	}
}

func TestImportProject_ReusesExistingDefinitionFromSameSource(t *testing.T) {
	ctx := context.Background()
	svc, repo, cleanup := newSourceTestService(t)
	defer cleanup()
	ids := map[string]string{}
	for _, src := range []string{models.FixtureSourceUser, models.FixtureSourceOFL} {
		d := &models.FixtureDefinition{Manufacturer: "TestMfg", Model: "TestModel", Type: "DIMMER", Source: src}
		if err := repo.CreateDefinitionWithChannels(ctx, d, []models.ChannelDefinition{{Name: "Dimmer", Type: "INTENSITY", MaxValue: 255}}); err != nil {
			t.Fatal(err)
		}
		ids[src] = d.ID
	}

	countDefs := func() int {
		defs, _ := repo.FindDefinitionsByManufacturerModel(ctx, "TestMfg", "TestModel")
		return len(defs)
	}

	// Exported OFL definition maps onto the existing OFL definition, not the
	// higher-precedence USER one.
	if _, _, _, err := svc.ImportProject(ctx, exportWithDef(export.ExportedFixtureDefinition{Source: models.FixtureSourceOFL}),
		ImportOptions{Mode: ImportModeCreate, FixtureConflictStrategy: FixtureConflictSkip}); err != nil {
		t.Fatal(err)
	}
	if n := countDefs(); n != 2 {
		t.Errorf("definitions after OFL import = %d, want 2 (reused)", n)
	}

	// No source in the export: reuse follows precedence (USER first).
	got, err := svc.findExistingDefinition(ctx, export.ExportedFixtureDefinition{Manufacturer: "TestMfg", Model: "TestModel"}, nil)
	if err != nil || got == nil || got.ID != ids[models.FixtureSourceUser] {
		t.Errorf("no-source match = %v, %v; want USER %s", got, err, ids[models.FixtureSourceUser])
	}
	got, err = svc.findExistingDefinition(ctx, export.ExportedFixtureDefinition{Manufacturer: "TestMfg", Model: "TestModel", Source: models.FixtureSourceOFL}, nil)
	if err != nil || got == nil || got.ID != ids[models.FixtureSourceOFL] {
		t.Errorf("OFL match = %v, %v; want OFL %s", got, err, ids[models.FixtureSourceOFL])
	}
	got, err = svc.findExistingDefinition(ctx, export.ExportedFixtureDefinition{Manufacturer: "TestMfg", Model: "TestModel", Source: models.FixtureSourceGDTF}, nil)
	if err != nil || got == nil || got.ID != ids[models.FixtureSourceUser] {
		t.Errorf("GDTF (absent) should fall back to precedence: %v, %v", got, err)
	}
}

func TestImportProject_KeepsSameNameDefinitionsFromDifferentSources(t *testing.T) {
	// An archive holding same-name USER and OFL definitions, imported into an
	// empty library with SKIP, must create both and keep each instance on its
	// own definition, in either export order.
	for _, order := range [][]string{
		{models.FixtureSourceUser, models.FixtureSourceOFL},
		{models.FixtureSourceOFL, models.FixtureSourceUser},
	} {
		t.Run(order[0]+"_first", func(t *testing.T) {
			ctx := context.Background()
			svc, repo, cleanup := newSourceTestService(t)
			defer cleanup()

			p := &export.ExportedProject{
				Version: "1.0",
				Project: &export.ExportProjectInfo{OriginalID: "p", Name: "Two Sources"},
			}
			channelsBySource := map[string][]export.ExportedChannelDefinition{
				models.FixtureSourceUser: {{RefID: "u-1", Name: "Dimmer", Type: "INTENSITY", MaxValue: 255}},
				models.FixtureSourceOFL: {
					{RefID: "o-1", Name: "Dimmer", Type: "INTENSITY", MaxValue: 255},
					{RefID: "o-2", Name: "Strobe", Type: "STROBE", MaxValue: 255},
				},
			}
			for i, src := range order {
				refID := "def-" + src
				p.FixtureDefinitions = append(p.FixtureDefinitions, export.ExportedFixtureDefinition{
					RefID: refID, Manufacturer: "TestMfg", Model: "TestModel", Type: "DIMMER",
					Source: src, Channels: channelsBySource[src],
				})
				p.FixtureInstances = append(p.FixtureInstances, export.ExportedFixtureInstance{
					RefID: "fx-" + src, Name: "Fixture " + src, DefinitionRefID: refID,
					Universe: 1, StartChannel: 1 + i*10,
				})
			}
			js, err := p.ToJSON()
			if err != nil {
				t.Fatal(err)
			}

			projectID, _, _, err := svc.ImportProject(ctx, js, ImportOptions{Mode: ImportModeCreate, FixtureConflictStrategy: FixtureConflictSkip})
			if err != nil {
				t.Fatal(err)
			}
			defs, err := repo.FindDefinitionsByManufacturerModel(ctx, "TestMfg", "TestModel")
			if err != nil || len(defs) != 2 {
				t.Fatalf("definitions = %d (%v), want 2", len(defs), err)
			}
			sourceByDefID := map[string]string{}
			for _, d := range defs {
				sourceByDefID[d.ID] = d.Source
			}
			instances, err := repo.FindByProjectID(ctx, projectID)
			if err != nil || len(instances) != 2 {
				t.Fatalf("instances = %d (%v), want 2", len(instances), err)
			}
			for _, fi := range instances {
				want := models.FixtureSourceUser
				if fi.Name == "Fixture "+models.FixtureSourceOFL {
					want = models.FixtureSourceOFL
				}
				if got := sourceByDefID[fi.DefinitionID]; got != want {
					t.Errorf("%s uses a %s definition, want %s", fi.Name, got, want)
				}
			}
		})
	}
}

func TestFindExistingDefinition_MatchesSourceIDAcrossRenames(t *testing.T) {
	ctx := context.Background()
	svc, repo, cleanup := newSourceTestService(t)
	defer cleanup()
	sid := "FIXTURE-TYPE-ID"
	d := &models.FixtureDefinition{Manufacturer: "NewMfg", Model: "NewName", Type: "MOVING_HEAD",
		Source: models.FixtureSourceGDTF, SourceID: &sid}
	if err := repo.CreateDefinition(ctx, d); err != nil {
		t.Fatal(err)
	}
	got, err := svc.findExistingDefinition(ctx, export.ExportedFixtureDefinition{
		Manufacturer: "OldMfg", Model: "OldName", Source: models.FixtureSourceGDTF, SourceID: &sid,
	}, nil)
	if err != nil || got == nil || got.ID != d.ID {
		t.Fatalf("source ID lookup = %v, %v; want %s", got, err, d.ID)
	}

	// A definition created earlier in the same import for another source is
	// not used as a cross-source fallback; a preexisting one is.
	other := &models.FixtureDefinition{Manufacturer: "M", Model: "X", Type: "DIMMER", Source: models.FixtureSourceUser}
	if err := repo.CreateDefinition(ctx, other); err != nil {
		t.Fatal(err)
	}
	ofl := export.ExportedFixtureDefinition{Manufacturer: "M", Model: "X", Source: models.FixtureSourceOFL}
	got, err = svc.findExistingDefinition(ctx, ofl, map[string]string{other.ID: models.FixtureSourceUser})
	if err != nil || got != nil {
		t.Errorf("created-this-import USER def should be skipped, got %v, %v", got, err)
	}
	got, err = svc.findExistingDefinition(ctx, ofl, nil)
	if err != nil || got == nil || got.ID != other.ID {
		t.Errorf("preexisting USER def should be the fallback, got %v, %v", got, err)
	}
}
