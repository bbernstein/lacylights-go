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
					{FixtureRefID: "inst-1", ChannelValues: []int{255}},
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
						FixtureRefID:  "inst-1",
						ChannelValues: []int{255, 128, 64},
						SceneOrder:    &sceneOrder,
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
