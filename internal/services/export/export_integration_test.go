package export

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/bbernstein/lacylights-go/internal/database/models"
	"github.com/bbernstein/lacylights-go/internal/services/testutil"
	"github.com/lucsky/cuid"
)

func TestExportProject_EmptyProject(t *testing.T) {
	testDB, cleanup := testutil.SetupTestDB(t)
	defer cleanup()

	service := NewService(
		testDB.ProjectRepo,
		testDB.FixtureRepo,
		testDB.SceneRepo,
		testDB.CueListRepo,
		testDB.CueRepo,
	)

	ctx := context.Background()

	// Create an empty project
	project := &models.Project{
		Name: testutil.UniqueProjectName("TestExport"),
	}
	if err := testDB.ProjectRepo.Create(ctx, project); err != nil {
		t.Fatalf("Failed to create project: %v", err)
	}

	// Export the project
	exported, stats, err := service.ExportProject(ctx, project.ID, true, true, true)
	if err != nil {
		t.Fatalf("ExportProject failed: %v", err)
	}
	if exported == nil {
		t.Fatal("Expected non-nil exported project")
	}
	if stats == nil {
		t.Fatal("Expected non-nil stats")
	}

	// Verify export structure
	if exported.Version != "1.0" {
		t.Errorf("Expected version '1.0', got '%s'", exported.Version)
	}
	if exported.Project == nil {
		t.Fatal("Expected non-nil project info")
	}
	if exported.Project.Name != project.Name {
		t.Errorf("Expected project name '%s', got '%s'", project.Name, exported.Project.Name)
	}
	if exported.Project.OriginalID != project.ID {
		t.Errorf("Expected original ID '%s', got '%s'", project.ID, exported.Project.OriginalID)
	}

	// Verify stats for empty project
	if stats.FixtureDefinitionsCount != 0 {
		t.Errorf("Expected 0 fixture definitions, got %d", stats.FixtureDefinitionsCount)
	}
	if stats.FixtureInstancesCount != 0 {
		t.Errorf("Expected 0 fixture instances, got %d", stats.FixtureInstancesCount)
	}
	if stats.ScenesCount != 0 {
		t.Errorf("Expected 0 scenes, got %d", stats.ScenesCount)
	}
	if stats.CueListsCount != 0 {
		t.Errorf("Expected 0 cue lists, got %d", stats.CueListsCount)
	}
	if stats.CuesCount != 0 {
		t.Errorf("Expected 0 cues, got %d", stats.CuesCount)
	}
}

func TestExportProject_ProjectNotFound(t *testing.T) {
	testDB, cleanup := testutil.SetupTestDB(t)
	defer cleanup()

	service := NewService(
		testDB.ProjectRepo,
		testDB.FixtureRepo,
		testDB.SceneRepo,
		testDB.CueListRepo,
		testDB.CueRepo,
	)

	ctx := context.Background()

	// Try to export non-existent project
	exported, stats, err := service.ExportProject(ctx, "non-existent-id", true, true, true)
	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}
	if exported != nil {
		t.Error("Expected nil exported project for non-existent ID")
	}
	if stats != nil {
		t.Error("Expected nil stats for non-existent ID")
	}
}

func TestExportProject_WithFixtures(t *testing.T) {
	testDB, cleanup := testutil.SetupTestDB(t)
	defer cleanup()

	service := NewService(
		testDB.ProjectRepo,
		testDB.FixtureRepo,
		testDB.SceneRepo,
		testDB.CueListRepo,
		testDB.CueRepo,
	)

	ctx := context.Background()

	// Create project
	project := &models.Project{
		Name: testutil.UniqueProjectName("TestExportFixtures"),
	}
	if err := testDB.ProjectRepo.Create(ctx, project); err != nil {
		t.Fatalf("Failed to create project: %v", err)
	}

	// Create fixture definition with channels
	def := &models.FixtureDefinition{
		Manufacturer: "TestMfg",
		Model:        testutil.UniqueFixtureName("ExportModel"),
		Type:         "LED",
		IsBuiltIn:    false,
	}
	channels := []models.ChannelDefinition{
		{Name: "Red", Type: "COLOR", Offset: 0, MinValue: 0, MaxValue: 255, DefaultValue: 0},
		{Name: "Green", Type: "COLOR", Offset: 1, MinValue: 0, MaxValue: 255, DefaultValue: 0},
		{Name: "Blue", Type: "COLOR", Offset: 2, MinValue: 0, MaxValue: 255, DefaultValue: 0},
	}
	if err := testDB.FixtureRepo.CreateDefinitionWithChannels(ctx, def, channels); err != nil {
		t.Fatalf("Failed to create fixture definition: %v", err)
	}

	// Create fixture instances (each needs its own channel slice for unique IDs)
	channelCount := 3
	fixture1 := &models.FixtureInstance{
		Name:         "LED 1",
		DefinitionID: def.ID,
		ProjectID:    project.ID,
		Universe:     1,
		StartChannel: 1,
		ChannelCount: &channelCount,
		Manufacturer: &def.Manufacturer,
		Model:        &def.Model,
		Type:         &def.Type,
	}
	instanceChannels1 := []models.InstanceChannel{
		{Name: "Red", Type: "COLOR", Offset: 0, MinValue: 0, MaxValue: 255, DefaultValue: 0},
		{Name: "Green", Type: "COLOR", Offset: 1, MinValue: 0, MaxValue: 255, DefaultValue: 0},
		{Name: "Blue", Type: "COLOR", Offset: 2, MinValue: 0, MaxValue: 255, DefaultValue: 0},
	}
	if err := testDB.FixtureRepo.CreateWithChannels(ctx, fixture1, instanceChannels1); err != nil {
		t.Fatalf("Failed to create fixture 1: %v", err)
	}

	fixture2 := &models.FixtureInstance{
		Name:         "LED 2",
		DefinitionID: def.ID,
		ProjectID:    project.ID,
		Universe:     1,
		StartChannel: 4,
		ChannelCount: &channelCount,
		Manufacturer: &def.Manufacturer,
		Model:        &def.Model,
		Type:         &def.Type,
	}
	instanceChannels2 := []models.InstanceChannel{
		{Name: "Red", Type: "COLOR", Offset: 0, MinValue: 0, MaxValue: 255, DefaultValue: 0},
		{Name: "Green", Type: "COLOR", Offset: 1, MinValue: 0, MaxValue: 255, DefaultValue: 0},
		{Name: "Blue", Type: "COLOR", Offset: 2, MinValue: 0, MaxValue: 255, DefaultValue: 0},
	}
	if err := testDB.FixtureRepo.CreateWithChannels(ctx, fixture2, instanceChannels2); err != nil {
		t.Fatalf("Failed to create fixture 2: %v", err)
	}

	// Export the project with fixtures
	exported, stats, err := service.ExportProject(ctx, project.ID, true, false, false)
	if err != nil {
		t.Fatalf("ExportProject failed: %v", err)
	}

	// Verify stats
	if stats.FixtureDefinitionsCount != 1 {
		t.Errorf("Expected 1 fixture definition, got %d", stats.FixtureDefinitionsCount)
	}
	if stats.FixtureInstancesCount != 2 {
		t.Errorf("Expected 2 fixture instances, got %d", stats.FixtureInstancesCount)
	}

	// Verify exported fixture definitions
	if len(exported.FixtureDefinitions) != 1 {
		t.Fatalf("Expected 1 fixture definition in export, got %d", len(exported.FixtureDefinitions))
	}
	if exported.FixtureDefinitions[0].Manufacturer != "TestMfg" {
		t.Errorf("Expected manufacturer 'TestMfg', got '%s'", exported.FixtureDefinitions[0].Manufacturer)
	}
	if len(exported.FixtureDefinitions[0].Channels) != 3 {
		t.Errorf("Expected 3 channels, got %d", len(exported.FixtureDefinitions[0].Channels))
	}

	// Verify exported fixture instances
	if len(exported.FixtureInstances) != 2 {
		t.Fatalf("Expected 2 fixture instances in export, got %d", len(exported.FixtureInstances))
	}
}

func TestExportProject_WithScenes(t *testing.T) {
	testDB, cleanup := testutil.SetupTestDB(t)
	defer cleanup()

	service := NewService(
		testDB.ProjectRepo,
		testDB.FixtureRepo,
		testDB.SceneRepo,
		testDB.CueListRepo,
		testDB.CueRepo,
	)

	ctx := context.Background()

	// Create project
	project := &models.Project{
		Name: testutil.UniqueProjectName("TestExportScenes"),
	}
	if err := testDB.ProjectRepo.Create(ctx, project); err != nil {
		t.Fatalf("Failed to create project: %v", err)
	}

	// Create fixture definition and instance
	def := &models.FixtureDefinition{
		Manufacturer: "TestMfg",
		Model:        testutil.UniqueFixtureName("SceneModel"),
		Type:         "LED",
		IsBuiltIn:    false,
	}
	channels := []models.ChannelDefinition{
		{Name: "Intensity", Type: "INTENSITY", Offset: 0, MinValue: 0, MaxValue: 255, DefaultValue: 0},
	}
	if err := testDB.FixtureRepo.CreateDefinitionWithChannels(ctx, def, channels); err != nil {
		t.Fatalf("Failed to create fixture definition: %v", err)
	}

	channelCount := 1
	fixture := &models.FixtureInstance{
		Name:         "Dimmer 1",
		DefinitionID: def.ID,
		ProjectID:    project.ID,
		Universe:     1,
		StartChannel: 1,
		ChannelCount: &channelCount,
		Manufacturer: &def.Manufacturer,
		Model:        &def.Model,
		Type:         &def.Type,
	}
	instanceChannels := []models.InstanceChannel{
		{Name: "Intensity", Type: "INTENSITY", Offset: 0, MinValue: 0, MaxValue: 255, DefaultValue: 0},
	}
	if err := testDB.FixtureRepo.CreateWithChannels(ctx, fixture, instanceChannels); err != nil {
		t.Fatalf("Failed to create fixture: %v", err)
	}

	// Create scene with fixture values
	channelData, _ := json.Marshal([]map[string]int{{"offset": 0, "value": 255}})
	scene := &models.Scene{
		Name:      "Full On",
		ProjectID: project.ID,
	}
	fixtureValues := []models.FixtureValue{
		{
			ID:       cuid.New(),
			FixtureID: fixture.ID,
			Channels:  string(channelData),
		},
	}
	if err := testDB.SceneRepo.CreateWithFixtureValues(ctx, scene, fixtureValues); err != nil {
		t.Fatalf("Failed to create scene: %v", err)
	}

	// Export with scenes
	exported, stats, err := service.ExportProject(ctx, project.ID, true, true, false)
	if err != nil {
		t.Fatalf("ExportProject failed: %v", err)
	}

	if stats.ScenesCount != 1 {
		t.Errorf("Expected 1 scene, got %d", stats.ScenesCount)
	}
	if len(exported.Scenes) != 1 {
		t.Fatalf("Expected 1 scene in export, got %d", len(exported.Scenes))
	}
	if exported.Scenes[0].Name != "Full On" {
		t.Errorf("Expected scene name 'Full On', got '%s'", exported.Scenes[0].Name)
	}
	if len(exported.Scenes[0].FixtureValues) != 1 {
		t.Errorf("Expected 1 fixture value, got %d", len(exported.Scenes[0].FixtureValues))
	}
}

func TestExportProject_WithCueLists(t *testing.T) {
	testDB, cleanup := testutil.SetupTestDB(t)
	defer cleanup()

	service := NewService(
		testDB.ProjectRepo,
		testDB.FixtureRepo,
		testDB.SceneRepo,
		testDB.CueListRepo,
		testDB.CueRepo,
	)

	ctx := context.Background()

	// Create project
	project := &models.Project{
		Name: testutil.UniqueProjectName("TestExportCueLists"),
	}
	if err := testDB.ProjectRepo.Create(ctx, project); err != nil {
		t.Fatalf("Failed to create project: %v", err)
	}

	// Create fixture and scene
	def := &models.FixtureDefinition{
		Manufacturer: "TestMfg",
		Model:        testutil.UniqueFixtureName("CueListModel"),
		Type:         "LED",
		IsBuiltIn:    false,
	}
	channels := []models.ChannelDefinition{
		{Name: "Intensity", Type: "INTENSITY", Offset: 0, MinValue: 0, MaxValue: 255, DefaultValue: 0},
	}
	if err := testDB.FixtureRepo.CreateDefinitionWithChannels(ctx, def, channels); err != nil {
		t.Fatalf("Failed to create fixture definition: %v", err)
	}

	channelCount := 1
	fixture := &models.FixtureInstance{
		Name:         "Dimmer",
		DefinitionID: def.ID,
		ProjectID:    project.ID,
		Universe:     1,
		StartChannel: 1,
		ChannelCount: &channelCount,
		Manufacturer: &def.Manufacturer,
		Model:        &def.Model,
		Type:         &def.Type,
	}
	instanceChannels := []models.InstanceChannel{
		{Name: "Intensity", Type: "INTENSITY", Offset: 0, MinValue: 0, MaxValue: 255, DefaultValue: 0},
	}
	if err := testDB.FixtureRepo.CreateWithChannels(ctx, fixture, instanceChannels); err != nil {
		t.Fatalf("Failed to create fixture: %v", err)
	}

	channelData, _ := json.Marshal([]map[string]int{{"offset": 0, "value": 255}})
	scene := &models.Scene{
		Name:      "Full",
		ProjectID: project.ID,
	}
	fixtureValues := []models.FixtureValue{
		{
			ID:       cuid.New(),
			FixtureID: fixture.ID,
			Channels:  string(channelData),
		},
	}
	if err := testDB.SceneRepo.CreateWithFixtureValues(ctx, scene, fixtureValues); err != nil {
		t.Fatalf("Failed to create scene: %v", err)
	}

	// Create cue list with cues
	cueList := &models.CueList{
		Name:      "Main Show",
		ProjectID: project.ID,
		Loop:      true,
	}
	if err := testDB.CueListRepo.Create(ctx, cueList); err != nil {
		t.Fatalf("Failed to create cue list: %v", err)
	}

	cue1 := &models.Cue{
		Name:        "Cue 1",
		CueNumber:   1.0,
		CueListID:   cueList.ID,
		SceneID:     scene.ID,
		FadeInTime:  2.0,
		FadeOutTime: 1.0,
	}
	if err := testDB.CueRepo.Create(ctx, cue1); err != nil {
		t.Fatalf("Failed to create cue 1: %v", err)
	}

	cue2 := &models.Cue{
		Name:        "Cue 2",
		CueNumber:   2.0,
		CueListID:   cueList.ID,
		SceneID:     scene.ID,
		FadeInTime:  1.0,
		FadeOutTime: 0.5,
	}
	if err := testDB.CueRepo.Create(ctx, cue2); err != nil {
		t.Fatalf("Failed to create cue 2: %v", err)
	}

	// Export with cue lists
	exported, stats, err := service.ExportProject(ctx, project.ID, true, true, true)
	if err != nil {
		t.Fatalf("ExportProject failed: %v", err)
	}

	if stats.CueListsCount != 1 {
		t.Errorf("Expected 1 cue list, got %d", stats.CueListsCount)
	}
	if stats.CuesCount != 2 {
		t.Errorf("Expected 2 cues, got %d", stats.CuesCount)
	}
	if len(exported.CueLists) != 1 {
		t.Fatalf("Expected 1 cue list in export, got %d", len(exported.CueLists))
	}
	if exported.CueLists[0].Name != "Main Show" {
		t.Errorf("Expected cue list name 'Main Show', got '%s'", exported.CueLists[0].Name)
	}
	if !exported.CueLists[0].Loop {
		t.Error("Expected cue list loop to be true")
	}
	if len(exported.CueLists[0].Cues) != 2 {
		t.Errorf("Expected 2 cues in cue list, got %d", len(exported.CueLists[0].Cues))
	}
}

func TestExportProject_SelectiveExport(t *testing.T) {
	testDB, cleanup := testutil.SetupTestDB(t)
	defer cleanup()

	service := NewService(
		testDB.ProjectRepo,
		testDB.FixtureRepo,
		testDB.SceneRepo,
		testDB.CueListRepo,
		testDB.CueRepo,
	)

	ctx := context.Background()

	// Create a complete project
	project := &models.Project{
		Name: testutil.UniqueProjectName("TestSelectiveExport"),
	}
	if err := testDB.ProjectRepo.Create(ctx, project); err != nil {
		t.Fatalf("Failed to create project: %v", err)
	}

	// Create fixture
	def := &models.FixtureDefinition{
		Manufacturer: "TestMfg",
		Model:        testutil.UniqueFixtureName("SelectiveModel"),
		Type:         "LED",
		IsBuiltIn:    false,
	}
	channels := []models.ChannelDefinition{
		{Name: "Intensity", Type: "INTENSITY", Offset: 0, MinValue: 0, MaxValue: 255, DefaultValue: 0},
	}
	if err := testDB.FixtureRepo.CreateDefinitionWithChannels(ctx, def, channels); err != nil {
		t.Fatalf("Failed to create fixture definition: %v", err)
	}

	channelCount := 1
	fixture := &models.FixtureInstance{
		Name:         "Dimmer",
		DefinitionID: def.ID,
		ProjectID:    project.ID,
		Universe:     1,
		StartChannel: 1,
		ChannelCount: &channelCount,
		Manufacturer: &def.Manufacturer,
		Model:        &def.Model,
		Type:         &def.Type,
	}
	instanceChannels := []models.InstanceChannel{
		{Name: "Intensity", Type: "INTENSITY", Offset: 0, MinValue: 0, MaxValue: 255, DefaultValue: 0},
	}
	if err := testDB.FixtureRepo.CreateWithChannels(ctx, fixture, instanceChannels); err != nil {
		t.Fatalf("Failed to create fixture: %v", err)
	}

	// Create scene
	channelData, _ := json.Marshal([]map[string]int{{"offset": 0, "value": 255}})
	scene := &models.Scene{
		Name:      "Test Scene",
		ProjectID: project.ID,
	}
	fixtureValues := []models.FixtureValue{
		{
			ID:       cuid.New(),
			FixtureID: fixture.ID,
			Channels:  string(channelData),
		},
	}
	if err := testDB.SceneRepo.CreateWithFixtureValues(ctx, scene, fixtureValues); err != nil {
		t.Fatalf("Failed to create scene: %v", err)
	}

	// Create cue list
	cueList := &models.CueList{
		Name:      "Test Cue List",
		ProjectID: project.ID,
	}
	if err := testDB.CueListRepo.Create(ctx, cueList); err != nil {
		t.Fatalf("Failed to create cue list: %v", err)
	}

	// Test: Export only fixtures (no scenes, no cue lists)
	_, stats, err := service.ExportProject(ctx, project.ID, true, false, false)
	if err != nil {
		t.Fatalf("ExportProject failed: %v", err)
	}

	if stats.FixtureInstancesCount != 1 {
		t.Errorf("Expected 1 fixture instance, got %d", stats.FixtureInstancesCount)
	}
	if stats.ScenesCount != 0 {
		t.Errorf("Expected 0 scenes (not requested), got %d", stats.ScenesCount)
	}
	if stats.CueListsCount != 0 {
		t.Errorf("Expected 0 cue lists (not requested), got %d", stats.CueListsCount)
	}

	// Test: Export only scenes (no fixtures, no cue lists)
	var exported *ExportedProject
	exported, stats, err = service.ExportProject(ctx, project.ID, false, true, false)
	if err != nil {
		t.Fatalf("ExportProject failed: %v", err)
	}

	if stats.FixtureInstancesCount != 0 {
		t.Errorf("Expected 0 fixture instances (not requested), got %d", stats.FixtureInstancesCount)
	}
	if stats.ScenesCount != 1 {
		t.Errorf("Expected 1 scene, got %d", stats.ScenesCount)
	}
	if stats.CueListsCount != 0 {
		t.Errorf("Expected 0 cue lists (not requested), got %d", stats.CueListsCount)
	}

	// Verify arrays are empty/nil for non-requested items
	if len(exported.FixtureDefinitions) != 0 {
		t.Errorf("Expected 0 fixture definitions, got %d", len(exported.FixtureDefinitions))
	}
	if len(exported.FixtureInstances) != 0 {
		t.Errorf("Expected 0 fixture instances, got %d", len(exported.FixtureInstances))
	}
}

func TestExportProject_ToJSON_RoundTrip(t *testing.T) {
	testDB, cleanup := testutil.SetupTestDB(t)
	defer cleanup()

	service := NewService(
		testDB.ProjectRepo,
		testDB.FixtureRepo,
		testDB.SceneRepo,
		testDB.CueListRepo,
		testDB.CueRepo,
	)

	ctx := context.Background()

	// Create a complete project
	desc := "Round trip test project"
	project := &models.Project{
		Name:        testutil.UniqueProjectName("TestRoundTrip"),
		Description: &desc,
	}
	if err := testDB.ProjectRepo.Create(ctx, project); err != nil {
		t.Fatalf("Failed to create project: %v", err)
	}

	// Create fixture
	def := &models.FixtureDefinition{
		Manufacturer: "TestMfg",
		Model:        testutil.UniqueFixtureName("RoundTripModel"),
		Type:         "LED",
		IsBuiltIn:    false,
	}
	defChannels := []models.ChannelDefinition{
		{Name: "Red", Type: "COLOR", Offset: 0, MinValue: 0, MaxValue: 255, DefaultValue: 0},
		{Name: "Green", Type: "COLOR", Offset: 1, MinValue: 0, MaxValue: 255, DefaultValue: 0},
		{Name: "Blue", Type: "COLOR", Offset: 2, MinValue: 0, MaxValue: 255, DefaultValue: 0},
	}
	if err := testDB.FixtureRepo.CreateDefinitionWithChannels(ctx, def, defChannels); err != nil {
		t.Fatalf("Failed to create fixture definition: %v", err)
	}

	channelCount := 3
	fixture := &models.FixtureInstance{
		Name:         "RGB LED",
		DefinitionID: def.ID,
		ProjectID:    project.ID,
		Universe:     1,
		StartChannel: 1,
		ChannelCount: &channelCount,
		Manufacturer: &def.Manufacturer,
		Model:        &def.Model,
		Type:         &def.Type,
	}
	instanceChannels := []models.InstanceChannel{
		{Name: "Red", Type: "COLOR", Offset: 0, MinValue: 0, MaxValue: 255, DefaultValue: 0},
		{Name: "Green", Type: "COLOR", Offset: 1, MinValue: 0, MaxValue: 255, DefaultValue: 0},
		{Name: "Blue", Type: "COLOR", Offset: 2, MinValue: 0, MaxValue: 255, DefaultValue: 0},
	}
	if err := testDB.FixtureRepo.CreateWithChannels(ctx, fixture, instanceChannels); err != nil {
		t.Fatalf("Failed to create fixture: %v", err)
	}

	// Export the project
	exported, _, err := service.ExportProject(ctx, project.ID, true, true, true)
	if err != nil {
		t.Fatalf("ExportProject failed: %v", err)
	}

	// Convert to JSON
	jsonStr, err := exported.ToJSON()
	if err != nil {
		t.Fatalf("ToJSON failed: %v", err)
	}

	// Parse it back
	parsed, err := ParseExportedProject(jsonStr)
	if err != nil {
		t.Fatalf("ParseExportedProject failed: %v", err)
	}

	// Verify key fields survived the round trip
	if parsed.Version != exported.Version {
		t.Errorf("Version mismatch: '%s' vs '%s'", parsed.Version, exported.Version)
	}
	if parsed.Project.Name != exported.Project.Name {
		t.Errorf("Project name mismatch: '%s' vs '%s'", parsed.Project.Name, exported.Project.Name)
	}
	if len(parsed.FixtureDefinitions) != len(exported.FixtureDefinitions) {
		t.Errorf("Fixture definitions count mismatch: %d vs %d", len(parsed.FixtureDefinitions), len(exported.FixtureDefinitions))
	}
	if len(parsed.FixtureInstances) != len(exported.FixtureInstances) {
		t.Errorf("Fixture instances count mismatch: %d vs %d", len(parsed.FixtureInstances), len(exported.FixtureInstances))
	}
}

func TestExportProject_WithTags(t *testing.T) {
	testDB, cleanup := testutil.SetupTestDB(t)
	defer cleanup()

	service := NewService(
		testDB.ProjectRepo,
		testDB.FixtureRepo,
		testDB.SceneRepo,
		testDB.CueListRepo,
		testDB.CueRepo,
	)

	ctx := context.Background()

	// Create project
	project := &models.Project{
		Name: testutil.UniqueProjectName("TestExportTags"),
	}
	if err := testDB.ProjectRepo.Create(ctx, project); err != nil {
		t.Fatalf("Failed to create project: %v", err)
	}

	// Create fixture definition
	def := &models.FixtureDefinition{
		Manufacturer: "TestMfg",
		Model:        testutil.UniqueFixtureName("TagsModel"),
		Type:         "LED",
		IsBuiltIn:    false,
	}
	channels := []models.ChannelDefinition{
		{Name: "Intensity", Type: "INTENSITY", Offset: 0, MinValue: 0, MaxValue: 255, DefaultValue: 0},
	}
	if err := testDB.FixtureRepo.CreateDefinitionWithChannels(ctx, def, channels); err != nil {
		t.Fatalf("Failed to create fixture definition: %v", err)
	}

	// Create fixture with tags
	tags, _ := json.Marshal([]string{"front", "wash", "rgb"})
	tagsStr := string(tags)
	channelCount := 1
	fixture := &models.FixtureInstance{
		Name:         "Tagged Fixture",
		DefinitionID: def.ID,
		ProjectID:    project.ID,
		Universe:     1,
		StartChannel: 1,
		ChannelCount: &channelCount,
		Tags:         &tagsStr,
		Manufacturer: &def.Manufacturer,
		Model:        &def.Model,
		Type:         &def.Type,
	}
	instanceChannels := []models.InstanceChannel{
		{Name: "Intensity", Type: "INTENSITY", Offset: 0, MinValue: 0, MaxValue: 255, DefaultValue: 0},
	}
	if err := testDB.FixtureRepo.CreateWithChannels(ctx, fixture, instanceChannels); err != nil {
		t.Fatalf("Failed to create fixture: %v", err)
	}

	// Export
	exported, _, err := service.ExportProject(ctx, project.ID, true, false, false)
	if err != nil {
		t.Fatalf("ExportProject failed: %v", err)
	}

	if len(exported.FixtureInstances) != 1 {
		t.Fatalf("Expected 1 fixture instance, got %d", len(exported.FixtureInstances))
	}
	if len(exported.FixtureInstances[0].Tags) != 3 {
		t.Errorf("Expected 3 tags, got %d", len(exported.FixtureInstances[0].Tags))
	}
}

// TestExportProject_InvalidTags tests that invalid tags JSON is handled gracefully
func TestExportProject_InvalidTags(t *testing.T) {
	testDB, cleanup := testutil.SetupTestDB(t)
	defer cleanup()

	service := NewService(
		testDB.ProjectRepo,
		testDB.FixtureRepo,
		testDB.SceneRepo,
		testDB.CueListRepo,
		testDB.CueRepo,
	)

	ctx := context.Background()

	// Create a project
	project := &models.Project{
		Name: testutil.UniqueProjectName("TestExportInvalidTags"),
	}
	if err := testDB.ProjectRepo.Create(ctx, project); err != nil {
		t.Fatalf("Failed to create project: %v", err)
	}

	// Create fixture definition
	def := &models.FixtureDefinition{
		Manufacturer: "Test",
		Model:        "InvalidTagsTest",
		Type:         "DIMMER",
	}
	if err := testDB.FixtureRepo.CreateDefinitionWithChannels(ctx, def, []models.ChannelDefinition{
		{Name: "Intensity", Type: "INTENSITY", Offset: 0, MinValue: 0, MaxValue: 255, DefaultValue: 0},
	}); err != nil {
		t.Fatalf("Failed to create fixture definition: %v", err)
	}

	// Create fixture with INVALID tags JSON (not a valid JSON array)
	invalidTagsStr := "not-valid-json"
	channelCount := 1
	fixture := &models.FixtureInstance{
		Name:         "Fixture with invalid tags",
		DefinitionID: def.ID,
		ProjectID:    project.ID,
		Universe:     1,
		StartChannel: 1,
		ChannelCount: &channelCount,
		Tags:         &invalidTagsStr,
		Manufacturer: &def.Manufacturer,
		Model:        &def.Model,
		Type:         &def.Type,
	}
	instanceChannels := []models.InstanceChannel{
		{Name: "Intensity", Type: "INTENSITY", Offset: 0, MinValue: 0, MaxValue: 255, DefaultValue: 0},
	}
	if err := testDB.FixtureRepo.CreateWithChannels(ctx, fixture, instanceChannels); err != nil {
		t.Fatalf("Failed to create fixture: %v", err)
	}

	// Export should succeed but with empty tags (error is logged, not returned)
	exported, _, err := service.ExportProject(ctx, project.ID, true, false, false)
	if err != nil {
		t.Fatalf("ExportProject failed: %v", err)
	}

	if len(exported.FixtureInstances) != 1 {
		t.Fatalf("Expected 1 fixture instance, got %d", len(exported.FixtureInstances))
	}
	// Tags should be empty due to invalid JSON
	if len(exported.FixtureInstances[0].Tags) != 0 {
		t.Errorf("Expected 0 tags (invalid JSON), got %d", len(exported.FixtureInstances[0].Tags))
	}
}

// TestExportProject_InvalidChannels tests that invalid channels JSON is handled gracefully
func TestExportProject_InvalidChannels(t *testing.T) {
	testDB, cleanup := testutil.SetupTestDB(t)
	defer cleanup()

	service := NewService(
		testDB.ProjectRepo,
		testDB.FixtureRepo,
		testDB.SceneRepo,
		testDB.CueListRepo,
		testDB.CueRepo,
	)

	ctx := context.Background()

	// Create a project
	project := &models.Project{
		Name: testutil.UniqueProjectName("TestExportInvalidChannels"),
	}
	if err := testDB.ProjectRepo.Create(ctx, project); err != nil {
		t.Fatalf("Failed to create project: %v", err)
	}

	// Create fixture definition
	def := &models.FixtureDefinition{
		Manufacturer: "Test",
		Model:        "InvalidChannelsTest",
		Type:         "DIMMER",
	}
	if err := testDB.FixtureRepo.CreateDefinitionWithChannels(ctx, def, []models.ChannelDefinition{
		{Name: "Intensity", Type: "INTENSITY", Offset: 0, MinValue: 0, MaxValue: 255, DefaultValue: 0},
	}); err != nil {
		t.Fatalf("Failed to create fixture definition: %v", err)
	}

	// Create fixture
	channelCount := 1
	fixture := &models.FixtureInstance{
		Name:         "Fixture",
		DefinitionID: def.ID,
		ProjectID:    project.ID,
		Universe:     1,
		StartChannel: 1,
		ChannelCount: &channelCount,
		Manufacturer: &def.Manufacturer,
		Model:        &def.Model,
		Type:         &def.Type,
	}
	instanceChannels := []models.InstanceChannel{
		{Name: "Intensity", Type: "INTENSITY", Offset: 0, MinValue: 0, MaxValue: 255, DefaultValue: 0},
	}
	if err := testDB.FixtureRepo.CreateWithChannels(ctx, fixture, instanceChannels); err != nil {
		t.Fatalf("Failed to create fixture: %v", err)
	}

	// Create scene
	scene := &models.Scene{
		Name:      "Test Scene",
		ProjectID: project.ID,
	}
	testDB.DB.Create(scene)

	// Create fixture value with INVALID channels JSON
	fixtureValue := &models.FixtureValue{
		ID:        cuid.New(),
		SceneID:   scene.ID,
		FixtureID: fixture.ID,
		Channels:  "not-valid-json",
	}
	testDB.DB.Create(fixtureValue)

	// Export should succeed but skip the invalid fixture value
	exported, stats, err := service.ExportProject(ctx, project.ID, false, true, false)
	if err != nil {
		t.Fatalf("ExportProject failed: %v", err)
	}

	if stats.ScenesCount != 1 {
		t.Errorf("Expected 1 scene, got %d", stats.ScenesCount)
	}
	if len(exported.Scenes) != 1 {
		t.Fatalf("Expected 1 scene, got %d", len(exported.Scenes))
	}
	// Fixture value should be skipped due to invalid JSON
	if len(exported.Scenes[0].FixtureValues) != 0 {
		t.Errorf("Expected 0 fixture values (invalid JSON should be skipped), got %d", len(exported.Scenes[0].FixtureValues))
	}
}

// TestExportProject_EmptyChannels tests that empty channels array is exported correctly
func TestExportProject_EmptyChannels(t *testing.T) {
	testDB, cleanup := testutil.SetupTestDB(t)
	defer cleanup()

	service := NewService(
		testDB.ProjectRepo,
		testDB.FixtureRepo,
		testDB.SceneRepo,
		testDB.CueListRepo,
		testDB.CueRepo,
	)

	ctx := context.Background()

	// Create a project
	project := &models.Project{
		Name: testutil.UniqueProjectName("TestExportEmptyChannels"),
	}
	if err := testDB.ProjectRepo.Create(ctx, project); err != nil {
		t.Fatalf("Failed to create project: %v", err)
	}

	// Create fixture definition
	def := &models.FixtureDefinition{
		Manufacturer: "Test",
		Model:        "EmptyChannelsTest",
		Type:         "DIMMER",
	}
	if err := testDB.FixtureRepo.CreateDefinitionWithChannels(ctx, def, []models.ChannelDefinition{
		{Name: "Intensity", Type: "INTENSITY", Offset: 0, MinValue: 0, MaxValue: 255, DefaultValue: 0},
	}); err != nil {
		t.Fatalf("Failed to create fixture definition: %v", err)
	}

	// Create fixture
	channelCount := 1
	fixture := &models.FixtureInstance{
		Name:         "Fixture",
		DefinitionID: def.ID,
		ProjectID:    project.ID,
		Universe:     1,
		StartChannel: 1,
		ChannelCount: &channelCount,
		Manufacturer: &def.Manufacturer,
		Model:        &def.Model,
		Type:         &def.Type,
	}
	instanceChannels := []models.InstanceChannel{
		{Name: "Intensity", Type: "INTENSITY", Offset: 0, MinValue: 0, MaxValue: 255, DefaultValue: 0},
	}
	if err := testDB.FixtureRepo.CreateWithChannels(ctx, fixture, instanceChannels); err != nil {
		t.Fatalf("Failed to create fixture: %v", err)
	}

	// Create scene
	scene := &models.Scene{
		Name:      "Test Scene",
		ProjectID: project.ID,
	}
	testDB.DB.Create(scene)

	// Create fixture value with empty channels array
	fixtureValue := &models.FixtureValue{
		ID:        cuid.New(),
		SceneID:   scene.ID,
		FixtureID: fixture.ID,
		Channels:  "[]",
	}
	testDB.DB.Create(fixtureValue)

	// Export should succeed with empty channels
	exported, stats, err := service.ExportProject(ctx, project.ID, false, true, false)
	if err != nil {
		t.Fatalf("ExportProject failed: %v", err)
	}

	if stats.ScenesCount != 1 {
		t.Errorf("Expected 1 scene, got %d", stats.ScenesCount)
	}
	if len(exported.Scenes) != 1 {
		t.Fatalf("Expected 1 scene, got %d", len(exported.Scenes))
	}
	// Fixture value should be present with empty channels
	if len(exported.Scenes[0].FixtureValues) != 1 {
		t.Errorf("Expected 1 fixture value, got %d", len(exported.Scenes[0].FixtureValues))
	}
	if len(exported.Scenes[0].FixtureValues[0].Channels) != 0 {
		t.Errorf("Expected 0 channels, got %d", len(exported.Scenes[0].FixtureValues[0].Channels))
	}
}

// TestExportProject_SparseChannels tests that sparse channels are exported correctly
func TestExportProject_SparseChannels(t *testing.T) {
	testDB, cleanup := testutil.SetupTestDB(t)
	defer cleanup()

	service := NewService(
		testDB.ProjectRepo,
		testDB.FixtureRepo,
		testDB.SceneRepo,
		testDB.CueListRepo,
		testDB.CueRepo,
	)

	ctx := context.Background()

	// Create a project
	project := &models.Project{
		Name: testutil.UniqueProjectName("TestExportSparseChannels"),
	}
	if err := testDB.ProjectRepo.Create(ctx, project); err != nil {
		t.Fatalf("Failed to create project: %v", err)
	}

	// Create fixture definition
	def := &models.FixtureDefinition{
		Manufacturer: "Test",
		Model:        "SparseChannelsTest",
		Type:         "LED_PAR",
	}
	if err := testDB.FixtureRepo.CreateDefinitionWithChannels(ctx, def, []models.ChannelDefinition{
		{Name: "Dimmer", Type: "INTENSITY", Offset: 0, MinValue: 0, MaxValue: 255, DefaultValue: 0},
		{Name: "Red", Type: "COLOR", Offset: 1, MinValue: 0, MaxValue: 255, DefaultValue: 0},
		{Name: "Green", Type: "COLOR", Offset: 2, MinValue: 0, MaxValue: 255, DefaultValue: 0},
		{Name: "Blue", Type: "COLOR", Offset: 3, MinValue: 0, MaxValue: 255, DefaultValue: 0},
	}); err != nil {
		t.Fatalf("Failed to create fixture definition: %v", err)
	}

	// Create fixture
	channelCount := 4
	fixture := &models.FixtureInstance{
		Name:         "LED PAR",
		DefinitionID: def.ID,
		ProjectID:    project.ID,
		Universe:     1,
		StartChannel: 1,
		ChannelCount: &channelCount,
		Manufacturer: &def.Manufacturer,
		Model:        &def.Model,
		Type:         &def.Type,
	}
	instanceChannels := []models.InstanceChannel{
		{Name: "Dimmer", Type: "INTENSITY", Offset: 0, MinValue: 0, MaxValue: 255, DefaultValue: 0},
		{Name: "Red", Type: "COLOR", Offset: 1, MinValue: 0, MaxValue: 255, DefaultValue: 0},
		{Name: "Green", Type: "COLOR", Offset: 2, MinValue: 0, MaxValue: 255, DefaultValue: 0},
		{Name: "Blue", Type: "COLOR", Offset: 3, MinValue: 0, MaxValue: 255, DefaultValue: 0},
	}
	if err := testDB.FixtureRepo.CreateWithChannels(ctx, fixture, instanceChannels); err != nil {
		t.Fatalf("Failed to create fixture: %v", err)
	}

	// Create scene
	scene := &models.Scene{
		Name:      "Red Only Scene",
		ProjectID: project.ID,
	}
	testDB.DB.Create(scene)

	// Create fixture value with sparse channels (only dimmer and red)
	channelData, _ := json.Marshal([]models.ChannelValue{
		{Offset: 0, Value: 255}, // Dimmer at full
		{Offset: 1, Value: 255}, // Red at full
		// Green and Blue intentionally omitted
	})
	fixtureValue := &models.FixtureValue{
		ID:        cuid.New(),
		SceneID:   scene.ID,
		FixtureID: fixture.ID,
		Channels:  string(channelData),
	}
	testDB.DB.Create(fixtureValue)

	// Export
	exported, stats, err := service.ExportProject(ctx, project.ID, false, true, false)
	if err != nil {
		t.Fatalf("ExportProject failed: %v", err)
	}

	if stats.ScenesCount != 1 {
		t.Errorf("Expected 1 scene, got %d", stats.ScenesCount)
	}
	if len(exported.Scenes) != 1 {
		t.Fatalf("Expected 1 scene, got %d", len(exported.Scenes))
	}
	if len(exported.Scenes[0].FixtureValues) != 1 {
		t.Fatalf("Expected 1 fixture value, got %d", len(exported.Scenes[0].FixtureValues))
	}

	// Verify sparse channels were exported correctly
	channels := exported.Scenes[0].FixtureValues[0].Channels
	if len(channels) != 2 {
		t.Errorf("Expected 2 channels (sparse), got %d", len(channels))
	}
	// Verify channel values
	if channels[0].Offset != 0 || channels[0].Value != 255 {
		t.Errorf("Expected channel 0: {0, 255}, got: {%d, %d}", channels[0].Offset, channels[0].Value)
	}
	if channels[1].Offset != 1 || channels[1].Value != 255 {
		t.Errorf("Expected channel 1: {1, 255}, got: {%d, %d}", channels[1].Offset, channels[1].Value)
	}
}
