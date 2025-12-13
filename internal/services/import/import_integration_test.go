package importservice

import (
	"context"
	"testing"

	"github.com/bbernstein/lacylights-go/internal/services/export"
	"github.com/bbernstein/lacylights-go/internal/services/testutil"
)

func TestImportProject_CreateMode_EmptyProject(t *testing.T) {
	testDB, cleanup := testutil.SetupTestDB(t)
	defer cleanup()

	service := NewService(
		testDB.ProjectRepo,
		testDB.FixtureRepo,
		testDB.SceneRepo,
		testDB.CueListRepo,
		testDB.CueRepo,
	)

	projectName := testutil.UniqueProjectName("TestImport")
	exported := &export.ExportedProject{
		Version: "1.0",
		Project: &export.ExportProjectInfo{
			OriginalID: "orig-proj-1",
			Name:       projectName,
		},
		FixtureDefinitions: []export.ExportedFixtureDefinition{},
		FixtureInstances:   []export.ExportedFixtureInstance{},
		Scenes:             []export.ExportedScene{},
		CueLists:           []export.ExportedCueList{},
	}

	jsonStr, err := exported.ToJSON()
	if err != nil {
		t.Fatalf("Failed to create JSON: %v", err)
	}

	ctx := context.Background()
	projectID, stats, warnings, err := service.ImportProject(ctx, jsonStr, ImportOptions{
		Mode: ImportModeCreate,
	})

	if err != nil {
		t.Fatalf("ImportProject failed: %v", err)
	}
	if projectID == "" {
		t.Error("Expected non-empty project ID")
	}
	if stats == nil {
		t.Fatal("Expected non-nil stats")
	}
	if stats.FixtureDefinitionsCreated != 0 {
		t.Errorf("Expected 0 fixture definitions, got %d", stats.FixtureDefinitionsCreated)
	}
	if len(warnings) != 0 {
		t.Errorf("Expected no warnings, got %v", warnings)
	}

	// Verify project was created in database
	project, err := testDB.ProjectRepo.FindByID(ctx, projectID)
	if err != nil {
		t.Fatalf("Failed to find project: %v", err)
	}
	if project == nil {
		t.Fatal("Project not found in database")
	}
	if project.Name != projectName {
		t.Errorf("Expected project name '%s', got '%s'", projectName, project.Name)
	}
}

func TestImportProject_CreateMode_WithCustomProjectName(t *testing.T) {
	testDB, cleanup := testutil.SetupTestDB(t)
	defer cleanup()

	service := NewService(
		testDB.ProjectRepo,
		testDB.FixtureRepo,
		testDB.SceneRepo,
		testDB.CueListRepo,
		testDB.CueRepo,
	)

	originalName := testutil.UniqueProjectName("Original")
	customName := testutil.UniqueProjectName("Custom")

	exported := &export.ExportedProject{
		Version: "1.0",
		Project: &export.ExportProjectInfo{
			OriginalID: "orig-proj-1",
			Name:       originalName,
		},
	}

	jsonStr, err := exported.ToJSON()
	if err != nil {
		t.Fatalf("Failed to create JSON: %v", err)
	}

	ctx := context.Background()
	projectID, _, _, err := service.ImportProject(ctx, jsonStr, ImportOptions{
		Mode:        ImportModeCreate,
		ProjectName: &customName,
	})

	if err != nil {
		t.Fatalf("ImportProject failed: %v", err)
	}

	// Verify project was created with custom name
	project, err := testDB.ProjectRepo.FindByID(ctx, projectID)
	if err != nil {
		t.Fatalf("Failed to find project: %v", err)
	}
	if project.Name != customName {
		t.Errorf("Expected custom name '%s', got '%s'", customName, project.Name)
	}
}

func TestImportProject_CreateMode_WithFixtureDefinition(t *testing.T) {
	testDB, cleanup := testutil.SetupTestDB(t)
	defer cleanup()

	service := NewService(
		testDB.ProjectRepo,
		testDB.FixtureRepo,
		testDB.SceneRepo,
		testDB.CueListRepo,
		testDB.CueRepo,
	)

	projectName := testutil.UniqueProjectName("TestImportFixtures")
	exported := &export.ExportedProject{
		Version: "1.0",
		Project: &export.ExportProjectInfo{
			OriginalID: "orig-proj-1",
			Name:       projectName,
		},
		FixtureDefinitions: []export.ExportedFixtureDefinition{
			{
				RefID:        "def-1",
				Manufacturer: "TestMfg",
				Model:        testutil.UniqueFixtureName("TestModel"),
				Type:         "LED",
				IsBuiltIn:    false,
				Channels: []export.ExportedChannelDefinition{
					{Name: "Red", Type: "COLOR", Offset: 0, MinValue: 0, MaxValue: 255, DefaultValue: 0},
					{Name: "Green", Type: "COLOR", Offset: 1, MinValue: 0, MaxValue: 255, DefaultValue: 0},
					{Name: "Blue", Type: "COLOR", Offset: 2, MinValue: 0, MaxValue: 255, DefaultValue: 0},
				},
			},
		},
	}

	jsonStr, err := exported.ToJSON()
	if err != nil {
		t.Fatalf("Failed to create JSON: %v", err)
	}

	ctx := context.Background()
	projectID, stats, _, err := service.ImportProject(ctx, jsonStr, ImportOptions{
		Mode: ImportModeCreate,
	})

	if err != nil {
		t.Fatalf("ImportProject failed: %v", err)
	}
	if projectID == "" {
		t.Error("Expected non-empty project ID")
	}
	if stats.FixtureDefinitionsCreated != 1 {
		t.Errorf("Expected 1 fixture definition created, got %d", stats.FixtureDefinitionsCreated)
	}
}

func TestImportProject_CreateMode_WithFixtureInstance(t *testing.T) {
	testDB, cleanup := testutil.SetupTestDB(t)
	defer cleanup()

	service := NewService(
		testDB.ProjectRepo,
		testDB.FixtureRepo,
		testDB.SceneRepo,
		testDB.CueListRepo,
		testDB.CueRepo,
	)

	projectName := testutil.UniqueProjectName("TestImportInstances")
	modelName := testutil.UniqueFixtureName("Model")

	exported := &export.ExportedProject{
		Version: "1.0",
		Project: &export.ExportProjectInfo{
			OriginalID: "orig-proj-1",
			Name:       projectName,
		},
		FixtureDefinitions: []export.ExportedFixtureDefinition{
			{
				RefID:        "def-1",
				Manufacturer: "TestMfg",
				Model:        modelName,
				Type:         "LED",
				IsBuiltIn:    false,
				Channels: []export.ExportedChannelDefinition{
					{Name: "Intensity", Type: "INTENSITY", Offset: 0, MinValue: 0, MaxValue: 255, DefaultValue: 0},
				},
			},
		},
		FixtureInstances: []export.ExportedFixtureInstance{
			{
				RefID:           "inst-1",
				Name:            "LED 1",
				DefinitionRefID: "def-1",
				Universe:        1,
				StartChannel:    1,
			},
			{
				RefID:           "inst-2",
				Name:            "LED 2",
				DefinitionRefID: "def-1",
				Universe:        1,
				StartChannel:    2,
			},
		},
	}

	jsonStr, err := exported.ToJSON()
	if err != nil {
		t.Fatalf("Failed to create JSON: %v", err)
	}

	ctx := context.Background()
	projectID, stats, _, err := service.ImportProject(ctx, jsonStr, ImportOptions{
		Mode: ImportModeCreate,
	})

	if err != nil {
		t.Fatalf("ImportProject failed: %v", err)
	}
	if stats.FixtureDefinitionsCreated != 1 {
		t.Errorf("Expected 1 fixture definition, got %d", stats.FixtureDefinitionsCreated)
	}
	if stats.FixtureInstancesCreated != 2 {
		t.Errorf("Expected 2 fixture instances, got %d", stats.FixtureInstancesCreated)
	}

	// Verify fixtures were created in database
	fixtures, err := testDB.FixtureRepo.FindByProjectID(ctx, projectID)
	if err != nil {
		t.Fatalf("Failed to find fixtures: %v", err)
	}
	if len(fixtures) != 2 {
		t.Errorf("Expected 2 fixtures in database, got %d", len(fixtures))
	}
}

func TestImportProject_CreateMode_WithScene(t *testing.T) {
	testDB, cleanup := testutil.SetupTestDB(t)
	defer cleanup()

	service := NewService(
		testDB.ProjectRepo,
		testDB.FixtureRepo,
		testDB.SceneRepo,
		testDB.CueListRepo,
		testDB.CueRepo,
	)

	projectName := testutil.UniqueProjectName("TestImportScenes")
	modelName := testutil.UniqueFixtureName("Model")

	exported := &export.ExportedProject{
		Version: "1.0",
		Project: &export.ExportProjectInfo{
			OriginalID: "orig-proj-1",
			Name:       projectName,
		},
		FixtureDefinitions: []export.ExportedFixtureDefinition{
			{
				RefID:        "def-1",
				Manufacturer: "TestMfg",
				Model:        modelName,
				Type:         "LED",
				IsBuiltIn:    false,
				Channels: []export.ExportedChannelDefinition{
					{Name: "Intensity", Type: "INTENSITY", Offset: 0, MinValue: 0, MaxValue: 255, DefaultValue: 0},
				},
			},
		},
		FixtureInstances: []export.ExportedFixtureInstance{
			{
				RefID:           "inst-1",
				Name:            "LED 1",
				DefinitionRefID: "def-1",
				Universe:        1,
				StartChannel:    1,
			},
		},
		Scenes: []export.ExportedScene{
			{
				RefID: "scene-1",
				Name:  "Test Scene",
				FixtureValues: []export.ExportedFixtureValue{
					{
						FixtureRefID: "inst-1",
						Channels: []export.ExportedChannelValue{
							{Offset: 0, Value: 255},
						},
					},
				},
			},
		},
	}

	jsonStr, err := exported.ToJSON()
	if err != nil {
		t.Fatalf("Failed to create JSON: %v", err)
	}

	ctx := context.Background()
	projectID, stats, _, err := service.ImportProject(ctx, jsonStr, ImportOptions{
		Mode: ImportModeCreate,
	})

	if err != nil {
		t.Fatalf("ImportProject failed: %v", err)
	}
	if stats.ScenesCreated != 1 {
		t.Errorf("Expected 1 scene, got %d", stats.ScenesCreated)
	}

	// Verify scene was created in database
	scenes, err := testDB.SceneRepo.FindByProjectID(ctx, projectID)
	if err != nil {
		t.Fatalf("Failed to find scenes: %v", err)
	}
	if len(scenes) != 1 {
		t.Errorf("Expected 1 scene in database, got %d", len(scenes))
	}
}

func TestImportProject_CreateMode_WithCueList(t *testing.T) {
	testDB, cleanup := testutil.SetupTestDB(t)
	defer cleanup()

	service := NewService(
		testDB.ProjectRepo,
		testDB.FixtureRepo,
		testDB.SceneRepo,
		testDB.CueListRepo,
		testDB.CueRepo,
	)

	projectName := testutil.UniqueProjectName("TestImportCueLists")
	modelName := testutil.UniqueFixtureName("Model")

	exported := &export.ExportedProject{
		Version: "1.0",
		Project: &export.ExportProjectInfo{
			OriginalID: "orig-proj-1",
			Name:       projectName,
		},
		FixtureDefinitions: []export.ExportedFixtureDefinition{
			{
				RefID:        "def-1",
				Manufacturer: "TestMfg",
				Model:        modelName,
				Type:         "LED",
				IsBuiltIn:    false,
				Channels: []export.ExportedChannelDefinition{
					{Name: "Intensity", Type: "INTENSITY", Offset: 0, MinValue: 0, MaxValue: 255, DefaultValue: 0},
				},
			},
		},
		FixtureInstances: []export.ExportedFixtureInstance{
			{
				RefID:           "inst-1",
				Name:            "LED 1",
				DefinitionRefID: "def-1",
				Universe:        1,
				StartChannel:    1,
			},
		},
		Scenes: []export.ExportedScene{
			{
				RefID: "scene-1",
				Name:  "Test Scene",
				FixtureValues: []export.ExportedFixtureValue{
					{
						FixtureRefID: "inst-1",
						Channels: []export.ExportedChannelValue{
							{Offset: 0, Value: 255},
						},
					},
				},
			},
		},
		CueLists: []export.ExportedCueList{
			{
				RefID: "cl-1",
				Name:  "Test Cue List",
				Loop:  true,
				Cues: []export.ExportedCue{
					{
						Name:        "Cue 1",
						CueNumber:   1.0,
						SceneRefID:  "scene-1",
						FadeInTime:  2.0,
						FadeOutTime: 1.0,
					},
					{
						Name:        "Cue 2",
						CueNumber:   2.0,
						SceneRefID:  "scene-1",
						FadeInTime:  1.0,
						FadeOutTime: 0.5,
					},
				},
			},
		},
	}

	jsonStr, err := exported.ToJSON()
	if err != nil {
		t.Fatalf("Failed to create JSON: %v", err)
	}

	ctx := context.Background()
	projectID, stats, _, err := service.ImportProject(ctx, jsonStr, ImportOptions{
		Mode: ImportModeCreate,
	})

	if err != nil {
		t.Fatalf("ImportProject failed: %v", err)
	}
	if stats.CueListsCreated != 1 {
		t.Errorf("Expected 1 cue list, got %d", stats.CueListsCreated)
	}
	if stats.CuesCreated != 2 {
		t.Errorf("Expected 2 cues, got %d", stats.CuesCreated)
	}

	// Verify cue list was created in database
	cueLists, err := testDB.CueListRepo.FindByProjectID(ctx, projectID)
	if err != nil {
		t.Fatalf("Failed to find cue lists: %v", err)
	}
	if len(cueLists) != 1 {
		t.Errorf("Expected 1 cue list in database, got %d", len(cueLists))
	}
}

func TestImportProject_CreateMode_CompleteProject(t *testing.T) {
	testDB, cleanup := testutil.SetupTestDB(t)
	defer cleanup()

	service := NewService(
		testDB.ProjectRepo,
		testDB.FixtureRepo,
		testDB.SceneRepo,
		testDB.CueListRepo,
		testDB.CueRepo,
	)

	projectName := testutil.UniqueProjectName("TestCompleteImport")
	modelName := testutil.UniqueFixtureName("CompleteModel")
	desc := "A complete test project"

	exported := &export.ExportedProject{
		Version: "1.0",
		Metadata: &export.ExportMetadata{
			ExportedAt:        "2025-01-01T00:00:00Z",
			LacyLightsVersion: "1.0.0",
		},
		Project: &export.ExportProjectInfo{
			OriginalID:  "orig-proj-1",
			Name:        projectName,
			Description: &desc,
		},
		FixtureDefinitions: []export.ExportedFixtureDefinition{
			{
				RefID:        "def-1",
				Manufacturer: "TestMfg",
				Model:        modelName,
				Type:         "LED",
				IsBuiltIn:    false,
				Channels: []export.ExportedChannelDefinition{
					{Name: "Red", Type: "COLOR", Offset: 0, MinValue: 0, MaxValue: 255, DefaultValue: 0},
					{Name: "Green", Type: "COLOR", Offset: 1, MinValue: 0, MaxValue: 255, DefaultValue: 0},
					{Name: "Blue", Type: "COLOR", Offset: 2, MinValue: 0, MaxValue: 255, DefaultValue: 0},
				},
			},
		},
		FixtureInstances: []export.ExportedFixtureInstance{
			{
				RefID:           "inst-1",
				Name:            "RGB LED 1",
				DefinitionRefID: "def-1",
				Universe:        1,
				StartChannel:    1,
				Tags:            []string{"front", "wash"},
			},
			{
				RefID:           "inst-2",
				Name:            "RGB LED 2",
				DefinitionRefID: "def-1",
				Universe:        1,
				StartChannel:    4,
				Tags:            []string{"back"},
			},
		},
		Scenes: []export.ExportedScene{
			{
				RefID: "scene-1",
				Name:  "Full Red",
				FixtureValues: []export.ExportedFixtureValue{
					{
						FixtureRefID: "inst-1",
						Channels: []export.ExportedChannelValue{
							{Offset: 0, Value: 255},
							{Offset: 1, Value: 0},
							{Offset: 2, Value: 0},
						},
					},
					{
						FixtureRefID: "inst-2",
						Channels: []export.ExportedChannelValue{
							{Offset: 0, Value: 255},
							{Offset: 1, Value: 0},
							{Offset: 2, Value: 0},
						},
					},
				},
			},
			{
				RefID: "scene-2",
				Name:  "Full Green",
				FixtureValues: []export.ExportedFixtureValue{
					{
						FixtureRefID: "inst-1",
						Channels: []export.ExportedChannelValue{
							{Offset: 0, Value: 0},
							{Offset: 1, Value: 255},
							{Offset: 2, Value: 0},
						},
					},
					{
						FixtureRefID: "inst-2",
						Channels: []export.ExportedChannelValue{
							{Offset: 0, Value: 0},
							{Offset: 1, Value: 255},
							{Offset: 2, Value: 0},
						},
					},
				},
			},
		},
		CueLists: []export.ExportedCueList{
			{
				RefID: "cl-1",
				Name:  "Color Chase",
				Loop:  true,
				Cues: []export.ExportedCue{
					{Name: "Red", CueNumber: 1.0, SceneRefID: "scene-1", FadeInTime: 1.0, FadeOutTime: 0.5},
					{Name: "Green", CueNumber: 2.0, SceneRefID: "scene-2", FadeInTime: 1.0, FadeOutTime: 0.5},
				},
			},
		},
	}

	jsonStr, err := exported.ToJSON()
	if err != nil {
		t.Fatalf("Failed to create JSON: %v", err)
	}

	ctx := context.Background()
	projectID, stats, warnings, err := service.ImportProject(ctx, jsonStr, ImportOptions{
		Mode: ImportModeCreate,
	})

	if err != nil {
		t.Fatalf("ImportProject failed: %v", err)
	}
	if len(warnings) != 0 {
		t.Logf("Warnings: %v", warnings)
	}

	// Verify all stats
	if stats.FixtureDefinitionsCreated != 1 {
		t.Errorf("Expected 1 fixture definition, got %d", stats.FixtureDefinitionsCreated)
	}
	if stats.FixtureInstancesCreated != 2 {
		t.Errorf("Expected 2 fixture instances, got %d", stats.FixtureInstancesCreated)
	}
	if stats.ScenesCreated != 2 {
		t.Errorf("Expected 2 scenes, got %d", stats.ScenesCreated)
	}
	if stats.CueListsCreated != 1 {
		t.Errorf("Expected 1 cue list, got %d", stats.CueListsCreated)
	}
	if stats.CuesCreated != 2 {
		t.Errorf("Expected 2 cues, got %d", stats.CuesCreated)
	}

	// Verify project description was preserved
	project, err := testDB.ProjectRepo.FindByID(ctx, projectID)
	if err != nil {
		t.Fatalf("Failed to find project: %v", err)
	}
	if project.Description == nil || *project.Description != desc {
		t.Error("Project description was not preserved")
	}
}

func TestImportProject_MergeMode_NoTargetProject(t *testing.T) {
	testDB, cleanup := testutil.SetupTestDB(t)
	defer cleanup()

	service := NewService(
		testDB.ProjectRepo,
		testDB.FixtureRepo,
		testDB.SceneRepo,
		testDB.CueListRepo,
		testDB.CueRepo,
	)

	exported := &export.ExportedProject{
		Version: "1.0",
		Project: &export.ExportProjectInfo{
			OriginalID: "orig-proj-1",
			Name:       "Test Project",
		},
	}

	jsonStr, err := exported.ToJSON()
	if err != nil {
		t.Fatalf("Failed to create JSON: %v", err)
	}

	ctx := context.Background()
	projectID, stats, warnings, err := service.ImportProject(ctx, jsonStr, ImportOptions{
		Mode: ImportModeMerge,
		// No TargetProjectID specified
	})

	// Should return empty without error
	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}
	if projectID != "" {
		t.Errorf("Expected empty project ID, got: %s", projectID)
	}
	if stats != nil {
		t.Error("Expected nil stats")
	}
	if warnings != nil {
		t.Error("Expected nil warnings")
	}
}

func TestImportProject_MergeMode_ProjectNotFound(t *testing.T) {
	testDB, cleanup := testutil.SetupTestDB(t)
	defer cleanup()

	service := NewService(
		testDB.ProjectRepo,
		testDB.FixtureRepo,
		testDB.SceneRepo,
		testDB.CueListRepo,
		testDB.CueRepo,
	)

	exported := &export.ExportedProject{
		Version: "1.0",
		Project: &export.ExportProjectInfo{
			OriginalID: "orig-proj-1",
			Name:       "Test Project",
		},
	}

	jsonStr, err := exported.ToJSON()
	if err != nil {
		t.Fatalf("Failed to create JSON: %v", err)
	}

	nonExistentID := "non-existent-project-id"
	ctx := context.Background()
	projectID, stats, warnings, err := service.ImportProject(ctx, jsonStr, ImportOptions{
		Mode:            ImportModeMerge,
		TargetProjectID: &nonExistentID,
	})

	// Should return empty without error (project not found)
	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}
	if projectID != "" {
		t.Errorf("Expected empty project ID, got: %s", projectID)
	}
	if stats != nil {
		t.Error("Expected nil stats")
	}
	if warnings != nil {
		t.Error("Expected nil warnings")
	}
}

func TestImportProject_FixtureConflict_Skip(t *testing.T) {
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
	projectName := testutil.UniqueProjectName("TestConflict")
	manufacturer := "ConflictMfg"
	model := testutil.UniqueFixtureName("ConflictModel")

	// First import - creates the fixture definition
	exported1 := &export.ExportedProject{
		Version: "1.0",
		Project: &export.ExportProjectInfo{
			OriginalID: "orig-proj-1",
			Name:       projectName,
		},
		FixtureDefinitions: []export.ExportedFixtureDefinition{
			{
				RefID:        "def-1",
				Manufacturer: manufacturer,
				Model:        model,
				Type:         "LED",
				IsBuiltIn:    false,
				Channels: []export.ExportedChannelDefinition{
					{Name: "Intensity", Type: "INTENSITY", Offset: 0, MinValue: 0, MaxValue: 255, DefaultValue: 0},
				},
			},
		},
	}

	jsonStr1, _ := exported1.ToJSON()
	_, stats1, _, err := service.ImportProject(ctx, jsonStr1, ImportOptions{
		Mode: ImportModeCreate,
	})
	if err != nil {
		t.Fatalf("First import failed: %v", err)
	}
	if stats1.FixtureDefinitionsCreated != 1 {
		t.Errorf("Expected 1 fixture definition created in first import, got %d", stats1.FixtureDefinitionsCreated)
	}

	// Second import with same manufacturer/model - should skip
	exported2 := &export.ExportedProject{
		Version: "1.0",
		Project: &export.ExportProjectInfo{
			OriginalID: "orig-proj-2",
			Name:       testutil.UniqueProjectName("TestConflict2"),
		},
		FixtureDefinitions: []export.ExportedFixtureDefinition{
			{
				RefID:        "def-1",
				Manufacturer: manufacturer,
				Model:        model, // Same manufacturer/model
				Type:         "LED",
				IsBuiltIn:    false,
				Channels: []export.ExportedChannelDefinition{
					{Name: "Intensity", Type: "INTENSITY", Offset: 0, MinValue: 0, MaxValue: 255, DefaultValue: 0},
				},
			},
		},
	}

	jsonStr2, _ := exported2.ToJSON()
	_, stats2, warnings, err := service.ImportProject(ctx, jsonStr2, ImportOptions{
		Mode:                    ImportModeCreate,
		FixtureConflictStrategy: FixtureConflictSkip,
	})
	if err != nil {
		t.Fatalf("Second import failed: %v", err)
	}
	if stats2.FixtureDefinitionsCreated != 0 {
		t.Errorf("Expected 0 fixture definitions created (skipped), got %d", stats2.FixtureDefinitionsCreated)
	}
	if len(warnings) == 0 {
		t.Error("Expected warning about skipped fixture definition")
	}
}

func TestImportProject_UnknownFixtureReference_Warning(t *testing.T) {
	testDB, cleanup := testutil.SetupTestDB(t)
	defer cleanup()

	service := NewService(
		testDB.ProjectRepo,
		testDB.FixtureRepo,
		testDB.SceneRepo,
		testDB.CueListRepo,
		testDB.CueRepo,
	)

	projectName := testutil.UniqueProjectName("TestUnknownRef")

	// Create export with fixture instance referencing non-existent definition
	exported := &export.ExportedProject{
		Version: "1.0",
		Project: &export.ExportProjectInfo{
			OriginalID: "orig-proj-1",
			Name:       projectName,
		},
		FixtureDefinitions: []export.ExportedFixtureDefinition{}, // No definitions
		FixtureInstances: []export.ExportedFixtureInstance{
			{
				RefID:           "inst-1",
				Name:            "Orphan Fixture",
				DefinitionRefID: "non-existent-def", // Unknown reference
				Universe:        1,
				StartChannel:    1,
			},
		},
	}

	jsonStr, err := exported.ToJSON()
	if err != nil {
		t.Fatalf("Failed to create JSON: %v", err)
	}

	ctx := context.Background()
	_, stats, warnings, err := service.ImportProject(ctx, jsonStr, ImportOptions{
		Mode: ImportModeCreate,
	})

	if err != nil {
		t.Fatalf("ImportProject failed: %v", err)
	}
	if stats.FixtureInstancesCreated != 0 {
		t.Errorf("Expected 0 fixture instances (skipped), got %d", stats.FixtureInstancesCreated)
	}
	if len(warnings) == 0 {
		t.Error("Expected warning about unknown fixture definition reference")
	}
}

func TestImportProject_Integration_WithInstanceChannels(t *testing.T) {
	testDB, cleanup := testutil.SetupTestDB(t)
	defer cleanup()

	service := NewService(
		testDB.ProjectRepo,
		testDB.FixtureRepo,
		testDB.SceneRepo,
		testDB.CueListRepo,
		testDB.CueRepo,
	)

	projectName := testutil.UniqueProjectName("TestInstanceChannels")
	modelName := testutil.UniqueFixtureName("InstanceChModel")

	exported := &export.ExportedProject{
		Version: "1.0",
		Project: &export.ExportProjectInfo{
			OriginalID: "orig-proj-1",
			Name:       projectName,
		},
		FixtureDefinitions: []export.ExportedFixtureDefinition{
			{
				RefID:        "def-1",
				Manufacturer: "TestMfg",
				Model:        modelName,
				Type:         "LED",
				IsBuiltIn:    false,
				Channels: []export.ExportedChannelDefinition{
					{Name: "Red", Type: "COLOR", Offset: 0, MinValue: 0, MaxValue: 255, DefaultValue: 0},
					{Name: "Green", Type: "COLOR", Offset: 1, MinValue: 0, MaxValue: 255, DefaultValue: 0},
					{Name: "Blue", Type: "COLOR", Offset: 2, MinValue: 0, MaxValue: 255, DefaultValue: 0},
				},
			},
		},
		FixtureInstances: []export.ExportedFixtureInstance{
			{
				RefID:           "inst-1",
				Name:            "Custom Channel LED",
				DefinitionRefID: "def-1",
				Universe:        1,
				StartChannel:    1,
				InstanceChannels: []export.ExportedInstanceChannel{
					{Name: "Red", Type: "COLOR", Offset: 0, MinValue: 0, MaxValue: 200, DefaultValue: 50},
					{Name: "Green", Type: "COLOR", Offset: 1, MinValue: 0, MaxValue: 200, DefaultValue: 50},
					{Name: "Blue", Type: "COLOR", Offset: 2, MinValue: 0, MaxValue: 200, DefaultValue: 50},
				},
			},
		},
	}

	jsonStr, err := exported.ToJSON()
	if err != nil {
		t.Fatalf("Failed to create JSON: %v", err)
	}

	ctx := context.Background()
	projectID, stats, _, err := service.ImportProject(ctx, jsonStr, ImportOptions{
		Mode: ImportModeCreate,
	})

	if err != nil {
		t.Fatalf("ImportProject failed: %v", err)
	}
	if stats.FixtureInstancesCreated != 1 {
		t.Errorf("Expected 1 fixture instance, got %d", stats.FixtureInstancesCreated)
	}

	// Verify fixture was created
	fixtures, err := testDB.FixtureRepo.FindByProjectID(ctx, projectID)
	if err != nil {
		t.Fatalf("Failed to find fixtures: %v", err)
	}
	if len(fixtures) != 1 {
		t.Errorf("Expected 1 fixture, got %d", len(fixtures))
	}
}
