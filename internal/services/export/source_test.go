package export

import (
	"context"
	"testing"

	"github.com/bbernstein/lacylights-go/internal/database/models"
	"github.com/bbernstein/lacylights-go/internal/services/testutil"
)

func TestExportProject_IncludesDefinitionSource(t *testing.T) {
	testDB, cleanup := testutil.SetupTestDB(t)
	defer cleanup()
	ctx := context.Background()
	service := NewService(testDB.ProjectRepo, testDB.FixtureRepo, testDB.LookRepo, testDB.CueListRepo, testDB.CueRepo)

	project := &models.Project{Name: testutil.UniqueProjectName("SourceExport")}
	if err := testDB.ProjectRepo.Create(ctx, project); err != nil {
		t.Fatal(err)
	}
	sid, ver := "8F54E11C-4C91-11E9-80BC-F1DFE217E634", "1.2"
	def := &models.FixtureDefinition{Manufacturer: "Acme", Model: testutil.UniqueFixtureName("Spot"), Type: "MOVING_HEAD",
		Source: models.FixtureSourceGDTF, SourceID: &sid, SourceVersion: &ver}
	if err := testDB.FixtureRepo.CreateDefinitionWithChannels(ctx, def, []models.ChannelDefinition{
		{Name: "Dimmer", Type: "INTENSITY", Offset: 0, MaxValue: 255},
	}); err != nil {
		t.Fatal(err)
	}
	count := 1
	fi := &models.FixtureInstance{Name: "Spot 1", DefinitionID: def.ID, ProjectID: project.ID, Universe: 1, StartChannel: 1,
		ChannelCount: &count, Manufacturer: &def.Manufacturer, Model: &def.Model, Type: &def.Type}
	if err := testDB.FixtureRepo.CreateWithChannels(ctx, fi, []models.InstanceChannel{
		{Name: "Dimmer", Type: "INTENSITY", Offset: 0, MaxValue: 255},
	}); err != nil {
		t.Fatal(err)
	}

	exported, _, err := service.ExportProject(ctx, project.ID, true, false, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(exported.FixtureDefinitions) != 1 {
		t.Fatalf("definitions = %d", len(exported.FixtureDefinitions))
	}
	got := exported.FixtureDefinitions[0]
	if got.Source != models.FixtureSourceGDTF || got.SourceID == nil || *got.SourceID != sid || got.SourceVersion == nil || *got.SourceVersion != ver {
		t.Errorf("exported source = %q %v %v", got.Source, got.SourceID, got.SourceVersion)
	}
}
