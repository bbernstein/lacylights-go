package importservice

import (
	"context"
	"testing"

	"github.com/bbernstein/lacylights-go/internal/services/export"
)

func TestImportModeConstants(t *testing.T) {
	// Test that constants have expected values
	tests := []struct {
		mode     ImportMode
		expected string
	}{
		{ImportModeCreate, "CREATE"},
		{ImportModeMerge, "MERGE"},
		{ImportModeReplace, "REPLACE"},
	}

	for _, tt := range tests {
		if string(tt.mode) != tt.expected {
			t.Errorf("Expected %s, got %s", tt.expected, string(tt.mode))
		}
	}
}

func TestFixtureConflictStrategyConstants(t *testing.T) {
	// Test that constants have expected values
	tests := []struct {
		strategy FixtureConflictStrategy
		expected string
	}{
		{FixtureConflictSkip, "SKIP"},
		{FixtureConflictReplace, "REPLACE"},
		{FixtureConflictRename, "RENAME"},
	}

	for _, tt := range tests {
		if string(tt.strategy) != tt.expected {
			t.Errorf("Expected %s, got %s", tt.expected, string(tt.strategy))
		}
	}
}

func TestImportStats(t *testing.T) {
	stats := &ImportStats{
		FixtureDefinitionsCreated: 5,
		FixtureInstancesCreated:   10,
		ScenesCreated:             8,
		CueListsCreated:           2,
		CuesCreated:               15,
	}

	if stats.FixtureDefinitionsCreated != 5 {
		t.Errorf("Expected FixtureDefinitionsCreated 5, got %d", stats.FixtureDefinitionsCreated)
	}
	if stats.FixtureInstancesCreated != 10 {
		t.Errorf("Expected FixtureInstancesCreated 10, got %d", stats.FixtureInstancesCreated)
	}
	if stats.ScenesCreated != 8 {
		t.Errorf("Expected ScenesCreated 8, got %d", stats.ScenesCreated)
	}
	if stats.CueListsCreated != 2 {
		t.Errorf("Expected CueListsCreated 2, got %d", stats.CueListsCreated)
	}
	if stats.CuesCreated != 15 {
		t.Errorf("Expected CuesCreated 15, got %d", stats.CuesCreated)
	}
}

func TestImportOptions(t *testing.T) {
	projectName := "Test Project"
	targetProjectID := "proj-123"

	options := ImportOptions{
		Mode:                    ImportModeCreate,
		TargetProjectID:         &targetProjectID,
		ProjectName:             &projectName,
		FixtureConflictStrategy: FixtureConflictSkip,
		ImportBuiltInFixtures:   true,
	}

	if options.Mode != ImportModeCreate {
		t.Errorf("Expected Mode ImportModeCreate, got %s", options.Mode)
	}
	if options.TargetProjectID == nil || *options.TargetProjectID != targetProjectID {
		t.Error("Expected TargetProjectID to be set")
	}
	if options.ProjectName == nil || *options.ProjectName != projectName {
		t.Error("Expected ProjectName to be set")
	}
	if options.FixtureConflictStrategy != FixtureConflictSkip {
		t.Errorf("Expected FixtureConflictStrategy FixtureConflictSkip, got %s", options.FixtureConflictStrategy)
	}
	if !options.ImportBuiltInFixtures {
		t.Error("Expected ImportBuiltInFixtures to be true")
	}
}

func TestImportOptions_Defaults(t *testing.T) {
	// Test zero value options
	options := ImportOptions{}

	if options.Mode != "" {
		t.Errorf("Expected empty Mode, got %s", options.Mode)
	}
	if options.TargetProjectID != nil {
		t.Error("Expected nil TargetProjectID")
	}
	if options.ProjectName != nil {
		t.Error("Expected nil ProjectName")
	}
	if options.FixtureConflictStrategy != "" {
		t.Errorf("Expected empty FixtureConflictStrategy, got %s", options.FixtureConflictStrategy)
	}
	if options.ImportBuiltInFixtures {
		t.Error("Expected ImportBuiltInFixtures to be false by default")
	}
}

func TestNewService(t *testing.T) {
	// Test that NewService creates a service with nil repos
	// (since we can't easily create real repos without a database)
	service := NewService(nil, nil, nil, nil, nil)

	if service == nil {
		t.Fatal("Expected NewService to return non-nil service")
	}
	if service.projectRepo != nil {
		t.Error("Expected projectRepo to be nil")
	}
	if service.fixtureRepo != nil {
		t.Error("Expected fixtureRepo to be nil")
	}
	if service.sceneRepo != nil {
		t.Error("Expected sceneRepo to be nil")
	}
	if service.cueListRepo != nil {
		t.Error("Expected cueListRepo to be nil")
	}
	if service.cueRepo != nil {
		t.Error("Expected cueRepo to be nil")
	}
}

func TestService_Structure(t *testing.T) {
	// Test that Service struct has expected fields
	service := &Service{}

	// These should compile - verifying struct has expected field types
	_ = service.projectRepo
	_ = service.fixtureRepo
	_ = service.sceneRepo
	_ = service.cueListRepo
	_ = service.cueRepo
}

func TestImportStats_ZeroValues(t *testing.T) {
	stats := &ImportStats{}

	if stats.FixtureDefinitionsCreated != 0 {
		t.Errorf("Expected 0, got %d", stats.FixtureDefinitionsCreated)
	}
	if stats.FixtureInstancesCreated != 0 {
		t.Errorf("Expected 0, got %d", stats.FixtureInstancesCreated)
	}
	if stats.ScenesCreated != 0 {
		t.Errorf("Expected 0, got %d", stats.ScenesCreated)
	}
	if stats.CueListsCreated != 0 {
		t.Errorf("Expected 0, got %d", stats.CueListsCreated)
	}
	if stats.CuesCreated != 0 {
		t.Errorf("Expected 0, got %d", stats.CuesCreated)
	}
}

func TestImportMode_TypeConversion(t *testing.T) {
	// Test type conversion
	mode := ImportMode("CUSTOM")
	if string(mode) != "CUSTOM" {
		t.Errorf("Expected CUSTOM, got %s", string(mode))
	}
}

func TestFixtureConflictStrategy_TypeConversion(t *testing.T) {
	// Test type conversion
	strategy := FixtureConflictStrategy("CUSTOM")
	if string(strategy) != "CUSTOM" {
		t.Errorf("Expected CUSTOM, got %s", string(strategy))
	}
}

func TestImportProject_InvalidJSON(t *testing.T) {
	service := NewService(nil, nil, nil, nil, nil)

	// Test with invalid JSON - should fail at parsing stage
	_, _, _, err := service.ImportProject(context.Background(), "invalid json", ImportOptions{})
	if err == nil {
		t.Error("Expected error for invalid JSON")
	}
}

func TestImportProject_EmptyJSON(t *testing.T) {
	service := NewService(nil, nil, nil, nil, nil)

	// Test with empty JSON object - should parse but panic on nil repo
	defer func() {
		if r := recover(); r == nil {
			t.Error("Expected panic due to nil repository")
		}
	}()

	_, _, _, _ = service.ImportProject(context.Background(), "{}", ImportOptions{
		Mode: ImportModeCreate,
	})
}

func TestImportProject_ValidJSONWithNilRepos(t *testing.T) {
	service := NewService(nil, nil, nil, nil, nil)

	// Create a valid export JSON
	exported := &export.ExportedProject{
		Version: "1.0",
		Project: &export.ExportProjectInfo{
			OriginalID: "proj-1",
			Name:       "Test Project",
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

	// This will parse successfully but panic on project creation due to nil repo
	defer func() {
		if r := recover(); r == nil {
			t.Error("Expected panic due to nil repository")
		}
	}()

	_, _, _, _ = service.ImportProject(context.Background(), jsonStr, ImportOptions{
		Mode: ImportModeCreate,
	})
}

func TestImportProject_MergeWithNilTargetProject(t *testing.T) {
	service := NewService(nil, nil, nil, nil, nil)

	exported := &export.ExportedProject{
		Version: "1.0",
		Project: &export.ExportProjectInfo{
			OriginalID: "proj-1",
			Name:       "Test Project",
		},
	}

	jsonStr, err := exported.ToJSON()
	if err != nil {
		t.Fatalf("Failed to create JSON: %v", err)
	}

	// Merge mode without target project ID should return empty without error
	projectID, stats, warnings, err := service.ImportProject(context.Background(), jsonStr, ImportOptions{
		Mode: ImportModeMerge,
	})
	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}
	if projectID != "" {
		t.Errorf("Expected empty projectID, got %s", projectID)
	}
	if stats != nil {
		t.Error("Expected nil stats")
	}
	if warnings != nil {
		t.Error("Expected nil warnings")
	}
}

func TestImportProject_ReplaceWithNilTargetProject(t *testing.T) {
	service := NewService(nil, nil, nil, nil, nil)

	exported := &export.ExportedProject{
		Version: "1.0",
		Project: &export.ExportProjectInfo{
			OriginalID: "proj-1",
			Name:       "Test Project",
		},
	}

	jsonStr, err := exported.ToJSON()
	if err != nil {
		t.Fatalf("Failed to create JSON: %v", err)
	}

	// Replace mode without target project ID should return empty without error
	projectID, stats, warnings, err := service.ImportProject(context.Background(), jsonStr, ImportOptions{
		Mode: ImportModeReplace,
	})
	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}
	if projectID != "" {
		t.Errorf("Expected empty projectID, got %s", projectID)
	}
	if stats != nil {
		t.Error("Expected nil stats")
	}
	if warnings != nil {
		t.Error("Expected nil warnings")
	}
}

func TestImportProject_MergeWithTargetProjectButNilRepo(t *testing.T) {
	service := NewService(nil, nil, nil, nil, nil)

	exported := &export.ExportedProject{
		Version: "1.0",
		Project: &export.ExportProjectInfo{
			OriginalID: "proj-1",
			Name:       "Test Project",
		},
	}

	jsonStr, err := exported.ToJSON()
	if err != nil {
		t.Fatalf("Failed to create JSON: %v", err)
	}

	targetID := "target-project-id"
	// Merge mode with target project ID but nil repo should panic
	defer func() {
		if r := recover(); r == nil {
			t.Error("Expected panic due to nil repository")
		}
	}()

	_, _, _, _ = service.ImportProject(context.Background(), jsonStr, ImportOptions{
		Mode:            ImportModeMerge,
		TargetProjectID: &targetID,
	})
}

func TestImportOptions_AllModes(t *testing.T) {
	modes := []ImportMode{ImportModeCreate, ImportModeMerge, ImportModeReplace}

	for _, mode := range modes {
		options := ImportOptions{Mode: mode}
		if options.Mode != mode {
			t.Errorf("Expected mode %s, got %s", mode, options.Mode)
		}
	}
}

func TestImportOptions_AllStrategies(t *testing.T) {
	strategies := []FixtureConflictStrategy{FixtureConflictSkip, FixtureConflictReplace, FixtureConflictRename}

	for _, strategy := range strategies {
		options := ImportOptions{FixtureConflictStrategy: strategy}
		if options.FixtureConflictStrategy != strategy {
			t.Errorf("Expected strategy %s, got %s", strategy, options.FixtureConflictStrategy)
		}
	}
}

func TestImportStats_Increment(t *testing.T) {
	stats := &ImportStats{}

	stats.FixtureDefinitionsCreated++
	stats.FixtureInstancesCreated += 5
	stats.ScenesCreated = 3
	stats.CueListsCreated = 2
	stats.CuesCreated = 10

	if stats.FixtureDefinitionsCreated != 1 {
		t.Errorf("Expected 1, got %d", stats.FixtureDefinitionsCreated)
	}
	if stats.FixtureInstancesCreated != 5 {
		t.Errorf("Expected 5, got %d", stats.FixtureInstancesCreated)
	}
	if stats.ScenesCreated != 3 {
		t.Errorf("Expected 3, got %d", stats.ScenesCreated)
	}
	if stats.CueListsCreated != 2 {
		t.Errorf("Expected 2, got %d", stats.CueListsCreated)
	}
	if stats.CuesCreated != 10 {
		t.Errorf("Expected 10, got %d", stats.CuesCreated)
	}
}

func TestImportOptions_WithProjectName(t *testing.T) {
	projectName := "Custom Project Name"
	options := ImportOptions{
		Mode:        ImportModeCreate,
		ProjectName: &projectName,
	}

	if options.ProjectName == nil {
		t.Error("Expected ProjectName to be set")
	}
	if *options.ProjectName != projectName {
		t.Errorf("Expected '%s', got '%s'", projectName, *options.ProjectName)
	}
}

func TestImportOptions_WithTargetProjectID(t *testing.T) {
	targetID := "target-proj-123"
	options := ImportOptions{
		Mode:            ImportModeMerge,
		TargetProjectID: &targetID,
	}

	if options.TargetProjectID == nil {
		t.Error("Expected TargetProjectID to be set")
	}
	if *options.TargetProjectID != targetID {
		t.Errorf("Expected '%s', got '%s'", targetID, *options.TargetProjectID)
	}
}

func TestImportProject_WithCustomProjectName(t *testing.T) {
	service := NewService(nil, nil, nil, nil, nil)

	exported := &export.ExportedProject{
		Version: "1.0",
		Project: &export.ExportProjectInfo{
			OriginalID: "proj-1",
			Name:       "Original Name",
		},
	}

	jsonStr, err := exported.ToJSON()
	if err != nil {
		t.Fatalf("Failed to create JSON: %v", err)
	}

	customName := "Custom Project Name"
	// Will panic due to nil repo, but we're testing that the path is exercised
	defer func() {
		if r := recover(); r == nil {
			t.Error("Expected panic due to nil repository")
		}
	}()

	_, _, _, _ = service.ImportProject(context.Background(), jsonStr, ImportOptions{
		Mode:        ImportModeCreate,
		ProjectName: &customName,
	})
}

func TestImportProject_ReplaceWithTargetProject(t *testing.T) {
	service := NewService(nil, nil, nil, nil, nil)

	exported := &export.ExportedProject{
		Version: "1.0",
		Project: &export.ExportProjectInfo{
			OriginalID: "proj-1",
			Name:       "Test Project",
		},
	}

	jsonStr, err := exported.ToJSON()
	if err != nil {
		t.Fatalf("Failed to create JSON: %v", err)
	}

	targetID := "target-project-id"
	// Replace mode with target project ID but nil repo should panic
	defer func() {
		if r := recover(); r == nil {
			t.Error("Expected panic due to nil repository")
		}
	}()

	_, _, _, _ = service.ImportProject(context.Background(), jsonStr, ImportOptions{
		Mode:            ImportModeReplace,
		TargetProjectID: &targetID,
	})
}

func TestImportProject_WithFixtureDefinitions(t *testing.T) {
	service := NewService(nil, nil, nil, nil, nil)

	exported := &export.ExportedProject{
		Version: "1.0",
		Project: &export.ExportProjectInfo{
			OriginalID: "proj-1",
			Name:       "Test Project",
		},
		FixtureDefinitions: []export.ExportedFixtureDefinition{
			{
				RefID:        "def-1",
				Manufacturer: "TestMfg",
				Model:        "TestModel",
				Type:         "LED",
				IsBuiltIn:    false,
				Channels: []export.ExportedChannelDefinition{
					{Name: "Red", Type: "COLOR", Offset: 0, MinValue: 0, MaxValue: 255, DefaultValue: 0},
				},
			},
		},
	}

	jsonStr, err := exported.ToJSON()
	if err != nil {
		t.Fatalf("Failed to create JSON: %v", err)
	}

	// Will panic due to nil repo
	defer func() {
		if r := recover(); r == nil {
			t.Error("Expected panic due to nil repository")
		}
	}()

	_, _, _, _ = service.ImportProject(context.Background(), jsonStr, ImportOptions{
		Mode: ImportModeCreate,
	})
}

func TestImportProject_ComplexExport(t *testing.T) {
	service := NewService(nil, nil, nil, nil, nil)

	followTime := 2.0
	easingType := "LINEAR"
	desc := "Test description"

	exported := &export.ExportedProject{
		Version: "1.0",
		Metadata: &export.ExportMetadata{
			ExportedAt:        "2025-01-01T00:00:00Z",
			LacyLightsVersion: "1.0.0",
		},
		Project: &export.ExportProjectInfo{
			OriginalID:  "proj-1",
			Name:        "Complex Project",
			Description: &desc,
		},
		FixtureDefinitions: []export.ExportedFixtureDefinition{
			{
				RefID:        "def-1",
				Manufacturer: "TestMfg",
				Model:        "TestModel",
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
				Tags:            []string{"front", "wash"},
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
				Name:  "Main Cue List",
				Loop:  true,
				Cues: []export.ExportedCue{
					{
						Name:        "Cue 1",
						CueNumber:   1.0,
						SceneRefID:  "scene-1",
						FadeInTime:  2.0,
						FadeOutTime: 1.0,
						FollowTime:  &followTime,
						EasingType:  &easingType,
					},
				},
			},
		},
	}

	jsonStr, err := exported.ToJSON()
	if err != nil {
		t.Fatalf("Failed to create JSON: %v", err)
	}

	// Verify JSON was created and is parseable
	parsed, err := export.ParseExportedProject(jsonStr)
	if err != nil {
		t.Fatalf("Failed to parse generated JSON: %v", err)
	}
	if parsed.Project.Name != "Complex Project" {
		t.Errorf("Expected 'Complex Project', got '%s'", parsed.Project.Name)
	}

	// Will panic due to nil repo
	defer func() {
		if r := recover(); r == nil {
			t.Error("Expected panic due to nil repository")
		}
	}()

	_, _, _, _ = service.ImportProject(context.Background(), jsonStr, ImportOptions{
		Mode:                    ImportModeCreate,
		FixtureConflictStrategy: FixtureConflictSkip,
		ImportBuiltInFixtures:   true,
	})
}

func TestImportProject_WithBuiltInFixtures(t *testing.T) {
	service := NewService(nil, nil, nil, nil, nil)

	exported := &export.ExportedProject{
		Version: "1.0",
		Project: &export.ExportProjectInfo{
			OriginalID: "proj-1",
			Name:       "Test Project",
		},
		FixtureDefinitions: []export.ExportedFixtureDefinition{
			{
				RefID:        "def-1",
				Manufacturer: "Generic",
				Model:        "Dimmer",
				Type:         "DIMMER",
				IsBuiltIn:    true, // Built-in fixture
				Channels: []export.ExportedChannelDefinition{
					{Name: "Intensity", Type: "INTENSITY", Offset: 0, MinValue: 0, MaxValue: 255, DefaultValue: 0},
				},
			},
		},
	}

	jsonStr, err := exported.ToJSON()
	if err != nil {
		t.Fatalf("Failed to create JSON: %v", err)
	}

	// Will panic due to nil repo
	defer func() {
		if r := recover(); r == nil {
			t.Error("Expected panic due to nil repository")
		}
	}()

	// Import with ImportBuiltInFixtures=false should try to find existing built-in
	_, _, _, _ = service.ImportProject(context.Background(), jsonStr, ImportOptions{
		Mode:                  ImportModeCreate,
		ImportBuiltInFixtures: false,
	})
}

func TestImportProject_WithInstanceChannels(t *testing.T) {
	service := NewService(nil, nil, nil, nil, nil)

	exported := &export.ExportedProject{
		Version: "1.0",
		Project: &export.ExportProjectInfo{
			OriginalID: "proj-1",
			Name:       "Test Project",
		},
		FixtureDefinitions: []export.ExportedFixtureDefinition{
			{
				RefID:        "def-1",
				Manufacturer: "TestMfg",
				Model:        "TestModel",
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
				Name:            "LED with Instance Channels",
				DefinitionRefID: "def-1",
				Universe:        1,
				StartChannel:    1,
				InstanceChannels: []export.ExportedInstanceChannel{
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

	// Will panic due to nil repo
	defer func() {
		if r := recover(); r == nil {
			t.Error("Expected panic due to nil repository")
		}
	}()

	_, _, _, _ = service.ImportProject(context.Background(), jsonStr, ImportOptions{
		Mode: ImportModeCreate,
	})
}

func TestImportProject_MalformedJSON(t *testing.T) {
	service := NewService(nil, nil, nil, nil, nil)

	testCases := []struct {
		name string
		json string
	}{
		{"empty string", ""},
		{"not json", "this is not json"},
		{"incomplete json", `{"version": "1.0"`},
		{"invalid unicode", `{"version": "\uinvalid"}`},
		{"array instead of object", `["item1", "item2"]`},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			_, _, _, err := service.ImportProject(context.Background(), tc.json, ImportOptions{})
			if err == nil {
				t.Errorf("Expected error for %s, got nil", tc.name)
			}
		})
	}
}

func TestImportOptions_CombinedFields(t *testing.T) {
	projectName := "Custom Name"
	targetID := "target-123"

	options := ImportOptions{
		Mode:                    ImportModeReplace,
		TargetProjectID:         &targetID,
		ProjectName:             &projectName,
		FixtureConflictStrategy: FixtureConflictRename,
		ImportBuiltInFixtures:   true,
	}

	if options.Mode != ImportModeReplace {
		t.Errorf("Expected ImportModeReplace, got %s", options.Mode)
	}
	if *options.TargetProjectID != targetID {
		t.Errorf("Expected '%s', got '%s'", targetID, *options.TargetProjectID)
	}
	if *options.ProjectName != projectName {
		t.Errorf("Expected '%s', got '%s'", projectName, *options.ProjectName)
	}
	if options.FixtureConflictStrategy != FixtureConflictRename {
		t.Errorf("Expected FixtureConflictRename, got %s", options.FixtureConflictStrategy)
	}
	if !options.ImportBuiltInFixtures {
		t.Error("Expected ImportBuiltInFixtures to be true")
	}
}

func TestImportStats_AllFieldsSet(t *testing.T) {
	stats := &ImportStats{
		FixtureDefinitionsCreated: 10,
		FixtureInstancesCreated:   25,
		ScenesCreated:             15,
		CueListsCreated:           5,
		CuesCreated:               50,
	}

	total := stats.FixtureDefinitionsCreated +
		stats.FixtureInstancesCreated +
		stats.ScenesCreated +
		stats.CueListsCreated +
		stats.CuesCreated

	if total != 105 {
		t.Errorf("Expected total of 105, got %d", total)
	}
}

func TestImportProject_WithSceneOrder(t *testing.T) {
	service := NewService(nil, nil, nil, nil, nil)

	sceneOrder := 5
	exported := &export.ExportedProject{
		Version: "1.0",
		Project: &export.ExportProjectInfo{
			OriginalID: "proj-1",
			Name:       "Test Project",
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
							{Offset: 1, Value: 128},
							{Offset: 2, Value: 64},
						},
						SceneOrder: &sceneOrder,
					},
				},
			},
		},
	}

	jsonStr, err := exported.ToJSON()
	if err != nil {
		t.Fatalf("Failed to create JSON: %v", err)
	}

	// Will panic due to nil repo
	defer func() {
		if r := recover(); r == nil {
			t.Error("Expected panic due to nil repository")
		}
	}()

	_, _, _, _ = service.ImportProject(context.Background(), jsonStr, ImportOptions{
		Mode: ImportModeCreate,
	})
}

func TestImportProject_WithNotes(t *testing.T) {
	service := NewService(nil, nil, nil, nil, nil)

	notes := "Important cue notes"
	followTime := 3.0
	easingType := "EASE_OUT"

	exported := &export.ExportedProject{
		Version: "1.0",
		Project: &export.ExportProjectInfo{
			OriginalID: "proj-1",
			Name:       "Test Project",
		},
		CueLists: []export.ExportedCueList{
			{
				RefID: "cl-1",
				Name:  "Test Cue List",
				Loop:  false,
				Cues: []export.ExportedCue{
					{
						Name:        "Cue with Notes",
						CueNumber:   1.5,
						SceneRefID:  "scene-1",
						FadeInTime:  2.0,
						FadeOutTime: 1.0,
						FollowTime:  &followTime,
						EasingType:  &easingType,
						Notes:       &notes,
					},
				},
			},
		},
	}

	jsonStr, err := exported.ToJSON()
	if err != nil {
		t.Fatalf("Failed to create JSON: %v", err)
	}

	// Verify JSON was created correctly
	parsed, err := export.ParseExportedProject(jsonStr)
	if err != nil {
		t.Fatalf("Failed to parse JSON: %v", err)
	}
	if len(parsed.CueLists) != 1 {
		t.Fatalf("Expected 1 cue list, got %d", len(parsed.CueLists))
	}
	if len(parsed.CueLists[0].Cues) != 1 {
		t.Fatalf("Expected 1 cue, got %d", len(parsed.CueLists[0].Cues))
	}
	if parsed.CueLists[0].Cues[0].Notes == nil || *parsed.CueLists[0].Cues[0].Notes != notes {
		t.Error("Expected notes to be preserved")
	}

	// Will panic due to nil repo
	defer func() {
		if r := recover(); r == nil {
			t.Error("Expected panic due to nil repository")
		}
	}()

	_, _, _, _ = service.ImportProject(context.Background(), jsonStr, ImportOptions{
		Mode: ImportModeCreate,
	})
}

func TestImportProject_WithModes(t *testing.T) {
	service := NewService(nil, nil, nil, nil, nil)

	shortName := "4CH"
	exported := &export.ExportedProject{
		Version: "1.0",
		Project: &export.ExportProjectInfo{
			OriginalID: "proj-1",
			Name:       "Modes Test Project",
		},
		FixtureDefinitions: []export.ExportedFixtureDefinition{
			{
				RefID:        "def-1",
				Manufacturer: "Chauvet DJ",
				Model:        "SlimPar Pro RGBA",
				Type:         "LED_PAR",
				IsBuiltIn:    false,
				Channels: []export.ExportedChannelDefinition{
					{RefID: "ch-r", Name: "Red", Type: "COLOR", Offset: 0, MinValue: 0, MaxValue: 255, DefaultValue: 0},
					{RefID: "ch-g", Name: "Green", Type: "COLOR", Offset: 1, MinValue: 0, MaxValue: 255, DefaultValue: 0},
					{RefID: "ch-b", Name: "Blue", Type: "COLOR", Offset: 2, MinValue: 0, MaxValue: 255, DefaultValue: 0},
					{RefID: "ch-a", Name: "Amber", Type: "COLOR", Offset: 3, MinValue: 0, MaxValue: 255, DefaultValue: 0},
					{RefID: "ch-i", Name: "Intensity", Type: "INTENSITY", Offset: 4, MinValue: 0, MaxValue: 255, DefaultValue: 0},
				},
				Modes: []export.ExportedFixtureMode{
					{
						RefID:        "mode-4ch",
						Name:         "4-channel",
						ShortName:    &shortName,
						ChannelCount: 4,
						ModeChannels: []export.ExportedModeChannel{
							{ChannelRefID: "ch-r", Offset: 0},
							{ChannelRefID: "ch-g", Offset: 1},
							{ChannelRefID: "ch-b", Offset: 2},
							{ChannelRefID: "ch-a", Offset: 3},
						},
					},
					{
						RefID:        "mode-5ch",
						Name:         "5-channel",
						ChannelCount: 5,
						ModeChannels: []export.ExportedModeChannel{
							{ChannelRefID: "ch-i", Offset: 0},
							{ChannelRefID: "ch-r", Offset: 1},
							{ChannelRefID: "ch-g", Offset: 2},
							{ChannelRefID: "ch-b", Offset: 3},
							{ChannelRefID: "ch-a", Offset: 4},
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

	// Verify JSON structure has modes
	parsed, err := export.ParseExportedProject(jsonStr)
	if err != nil {
		t.Fatalf("Failed to parse JSON: %v", err)
	}

	// Verify fixture definition has modes
	if len(parsed.FixtureDefinitions) != 1 {
		t.Fatalf("Expected 1 fixture definition, got %d", len(parsed.FixtureDefinitions))
	}

	def := parsed.FixtureDefinitions[0]
	if len(def.Modes) != 2 {
		t.Fatalf("Expected 2 modes, got %d", len(def.Modes))
	}

	// Verify mode channel references
	mode4ch := def.Modes[0]
	if mode4ch.Name != "4-channel" {
		t.Errorf("Expected mode name '4-channel', got '%s'", mode4ch.Name)
	}
	if len(mode4ch.ModeChannels) != 4 {
		t.Fatalf("Expected 4 mode channels, got %d", len(mode4ch.ModeChannels))
	}

	// Verify channel RefID references are preserved
	expectedRefs := []string{"ch-r", "ch-g", "ch-b", "ch-a"}
	for i, mc := range mode4ch.ModeChannels {
		if mc.ChannelRefID != expectedRefs[i] {
			t.Errorf("Mode channel %d: expected RefID '%s', got '%s'", i, expectedRefs[i], mc.ChannelRefID)
		}
	}

	// Verify 5-channel mode starts with intensity
	mode5ch := def.Modes[1]
	if mode5ch.ModeChannels[0].ChannelRefID != "ch-i" {
		t.Errorf("Expected 5-channel mode first channel to be 'ch-i', got '%s'", mode5ch.ModeChannels[0].ChannelRefID)
	}

	// Will panic due to nil repo when trying to import
	defer func() {
		if r := recover(); r == nil {
			t.Error("Expected panic due to nil repository")
		}
	}()

	_, _, _, _ = service.ImportProject(context.Background(), jsonStr, ImportOptions{
		Mode: ImportModeCreate,
	})
}

func TestImportProject_WithModes_ChannelNameFallback(t *testing.T) {
	// Test that import works when channels have no RefID (uses name as fallback)
	service := NewService(nil, nil, nil, nil, nil)

	exported := &export.ExportedProject{
		Version: "1.0",
		Project: &export.ExportProjectInfo{
			OriginalID: "proj-1",
			Name:       "Fallback Test Project",
		},
		FixtureDefinitions: []export.ExportedFixtureDefinition{
			{
				RefID:        "def-1",
				Manufacturer: "Generic",
				Model:        "RGB LED",
				Type:         "LED",
				IsBuiltIn:    false,
				// Channels without RefID - should use name as fallback
				Channels: []export.ExportedChannelDefinition{
					{Name: "Red", Type: "COLOR", Offset: 0, MinValue: 0, MaxValue: 255, DefaultValue: 0},
					{Name: "Green", Type: "COLOR", Offset: 1, MinValue: 0, MaxValue: 255, DefaultValue: 0},
					{Name: "Blue", Type: "COLOR", Offset: 2, MinValue: 0, MaxValue: 255, DefaultValue: 0},
				},
				Modes: []export.ExportedFixtureMode{
					{
						RefID:        "mode-rgb",
						Name:         "RGB",
						ChannelCount: 3,
						// Mode channels reference channel by name (fallback case)
						ModeChannels: []export.ExportedModeChannel{
							{ChannelRefID: "Red", Offset: 0},
							{ChannelRefID: "Green", Offset: 1},
							{ChannelRefID: "Blue", Offset: 2},
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

	// Verify JSON parses correctly
	parsed, err := export.ParseExportedProject(jsonStr)
	if err != nil {
		t.Fatalf("Failed to parse JSON: %v", err)
	}

	// Verify channels have no RefID
	def := parsed.FixtureDefinitions[0]
	for _, ch := range def.Channels {
		if ch.RefID != "" {
			t.Errorf("Expected empty RefID for channel '%s', got '%s'", ch.Name, ch.RefID)
		}
	}

	// Verify mode channel references use channel names
	if def.Modes[0].ModeChannels[0].ChannelRefID != "Red" {
		t.Errorf("Expected mode channel RefID 'Red', got '%s'", def.Modes[0].ModeChannels[0].ChannelRefID)
	}

	// Will panic due to nil repo
	defer func() {
		if r := recover(); r == nil {
			t.Error("Expected panic due to nil repository")
		}
	}()

	_, _, _, _ = service.ImportProject(context.Background(), jsonStr, ImportOptions{
		Mode: ImportModeCreate,
	})
}

func TestImportProject_WithModeNameAndChannelCount(t *testing.T) {
	// Test that fixture instances with ModeName and ChannelCount are properly preserved
	service := NewService(nil, nil, nil, nil, nil)

	modeName := "4-channel"
	channelCount := 4
	shortName := "4CH"

	exported := &export.ExportedProject{
		Version: "1.0",
		Project: &export.ExportProjectInfo{
			OriginalID: "proj-1",
			Name:       "ModeName Test Project",
		},
		FixtureDefinitions: []export.ExportedFixtureDefinition{
			{
				RefID:        "def-1",
				Manufacturer: "Chauvet DJ",
				Model:        "SlimPar Pro RGBA",
				Type:         "LED_PAR",
				IsBuiltIn:    false,
				Channels: []export.ExportedChannelDefinition{
					{RefID: "ch-r", Name: "Red", Type: "COLOR", Offset: 0, MinValue: 0, MaxValue: 255, DefaultValue: 0},
					{RefID: "ch-g", Name: "Green", Type: "COLOR", Offset: 1, MinValue: 0, MaxValue: 255, DefaultValue: 0},
					{RefID: "ch-b", Name: "Blue", Type: "COLOR", Offset: 2, MinValue: 0, MaxValue: 255, DefaultValue: 0},
					{RefID: "ch-a", Name: "Amber", Type: "COLOR", Offset: 3, MinValue: 0, MaxValue: 255, DefaultValue: 0},
					{RefID: "ch-i", Name: "Intensity", Type: "INTENSITY", Offset: 4, MinValue: 0, MaxValue: 255, DefaultValue: 0},
				},
				Modes: []export.ExportedFixtureMode{
					{
						RefID:        "mode-4ch",
						Name:         "4-channel",
						ShortName:    &shortName,
						ChannelCount: 4,
						ModeChannels: []export.ExportedModeChannel{
							{ChannelRefID: "ch-r", Offset: 0},
							{ChannelRefID: "ch-g", Offset: 1},
							{ChannelRefID: "ch-b", Offset: 2},
							{ChannelRefID: "ch-a", Offset: 3},
						},
					},
					{
						RefID:        "mode-5ch",
						Name:         "5-channel",
						ChannelCount: 5,
						ModeChannels: []export.ExportedModeChannel{
							{ChannelRefID: "ch-i", Offset: 0},
							{ChannelRefID: "ch-r", Offset: 1},
							{ChannelRefID: "ch-g", Offset: 2},
							{ChannelRefID: "ch-b", Offset: 3},
							{ChannelRefID: "ch-a", Offset: 4},
						},
					},
				},
			},
		},
		FixtureInstances: []export.ExportedFixtureInstance{
			{
				RefID:           "inst-1",
				Name:            "LED Par 1",
				DefinitionRefID: "def-1",
				ModeName:        &modeName,
				ChannelCount:    &channelCount,
				Universe:        1,
				StartChannel:    1,
				Tags:            []string{"front", "wash"},
			},
			{
				RefID:           "inst-2",
				Name:            "LED Par 2",
				DefinitionRefID: "def-1",
				ModeName:        &modeName,
				ChannelCount:    &channelCount,
				Universe:        1,
				StartChannel:    5,
				Tags:            []string{"front", "wash"},
			},
		},
	}

	jsonStr, err := exported.ToJSON()
	if err != nil {
		t.Fatalf("Failed to create JSON: %v", err)
	}

	// Verify JSON parses correctly and preserves ModeName and ChannelCount
	parsed, err := export.ParseExportedProject(jsonStr)
	if err != nil {
		t.Fatalf("Failed to parse JSON: %v", err)
	}

	// Verify fixture instances count
	if len(parsed.FixtureInstances) != 2 {
		t.Fatalf("Expected 2 fixture instances, got %d", len(parsed.FixtureInstances))
	}

	// Verify first fixture instance has ModeName and ChannelCount
	inst1 := parsed.FixtureInstances[0]
	if inst1.ModeName == nil {
		t.Fatal("Expected ModeName to be set on fixture instance 1")
	}
	if *inst1.ModeName != "4-channel" {
		t.Errorf("Expected ModeName '4-channel', got '%s'", *inst1.ModeName)
	}
	if inst1.ChannelCount == nil {
		t.Fatal("Expected ChannelCount to be set on fixture instance 1")
	}
	if *inst1.ChannelCount != 4 {
		t.Errorf("Expected ChannelCount 4, got %d", *inst1.ChannelCount)
	}

	// Verify second fixture instance has ModeName and ChannelCount
	inst2 := parsed.FixtureInstances[1]
	if inst2.ModeName == nil {
		t.Fatal("Expected ModeName to be set on fixture instance 2")
	}
	if *inst2.ModeName != "4-channel" {
		t.Errorf("Expected ModeName '4-channel', got '%s'", *inst2.ModeName)
	}
	if inst2.ChannelCount == nil {
		t.Fatal("Expected ChannelCount to be set on fixture instance 2")
	}
	if *inst2.ChannelCount != 4 {
		t.Errorf("Expected ChannelCount 4, got %d", *inst2.ChannelCount)
	}

	// Will panic due to nil repo when trying to import
	defer func() {
		if r := recover(); r == nil {
			t.Error("Expected panic due to nil repository")
		}
	}()

	_, _, _, _ = service.ImportProject(context.Background(), jsonStr, ImportOptions{
		Mode: ImportModeCreate,
	})
}

func TestImportProject_FixtureInstance_NoModeName(t *testing.T) {
	// Test that fixture instances without ModeName work correctly (nil stays nil)
	service := NewService(nil, nil, nil, nil, nil)

	exported := &export.ExportedProject{
		Version: "1.0",
		Project: &export.ExportProjectInfo{
			OriginalID: "proj-1",
			Name:       "No Mode Test Project",
		},
		FixtureDefinitions: []export.ExportedFixtureDefinition{
			{
				RefID:        "def-1",
				Manufacturer: "Generic",
				Model:        "Dimmer",
				Type:         "DIMMER",
				IsBuiltIn:    false,
				Channels: []export.ExportedChannelDefinition{
					{Name: "Intensity", Type: "INTENSITY", Offset: 0, MinValue: 0, MaxValue: 255, DefaultValue: 0},
				},
			},
		},
		FixtureInstances: []export.ExportedFixtureInstance{
			{
				RefID:           "inst-1",
				Name:            "Dimmer 1",
				DefinitionRefID: "def-1",
				// ModeName is nil - no mode set
				// ChannelCount is nil
				Universe:     1,
				StartChannel: 1,
			},
		},
	}

	jsonStr, err := exported.ToJSON()
	if err != nil {
		t.Fatalf("Failed to create JSON: %v", err)
	}

	// Verify JSON parses correctly
	parsed, err := export.ParseExportedProject(jsonStr)
	if err != nil {
		t.Fatalf("Failed to parse JSON: %v", err)
	}

	// Verify fixture instance has nil ModeName and ChannelCount
	inst := parsed.FixtureInstances[0]
	if inst.ModeName != nil {
		t.Errorf("Expected ModeName to be nil, got '%s'", *inst.ModeName)
	}
	if inst.ChannelCount != nil {
		t.Errorf("Expected ChannelCount to be nil, got %d", *inst.ChannelCount)
	}

	// Will panic due to nil repo
	defer func() {
		if r := recover(); r == nil {
			t.Error("Expected panic due to nil repository")
		}
	}()

	_, _, _, _ = service.ImportProject(context.Background(), jsonStr, ImportOptions{
		Mode: ImportModeCreate,
	})
}
