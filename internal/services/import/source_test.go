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
	got, err := svc.findExistingDefinition(ctx, export.ExportedFixtureDefinition{Manufacturer: "TestMfg", Model: "TestModel"})
	if err != nil || got == nil || got.ID != ids[models.FixtureSourceUser] {
		t.Errorf("no-source match = %v, %v; want USER %s", got, err, ids[models.FixtureSourceUser])
	}
	got, err = svc.findExistingDefinition(ctx, export.ExportedFixtureDefinition{Manufacturer: "TestMfg", Model: "TestModel", Source: models.FixtureSourceOFL})
	if err != nil || got == nil || got.ID != ids[models.FixtureSourceOFL] {
		t.Errorf("OFL match = %v, %v; want OFL %s", got, err, ids[models.FixtureSourceOFL])
	}
	got, err = svc.findExistingDefinition(ctx, export.ExportedFixtureDefinition{Manufacturer: "TestMfg", Model: "TestModel", Source: models.FixtureSourceGDTF})
	if err != nil || got == nil || got.ID != ids[models.FixtureSourceUser] {
		t.Errorf("GDTF (absent) should fall back to precedence: %v, %v", got, err)
	}
}
