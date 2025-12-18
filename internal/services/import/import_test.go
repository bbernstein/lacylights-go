package importservice

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/bbernstein/lacylights-go/internal/database/models"
	"github.com/bbernstein/lacylights-go/internal/database/repositories"
	"github.com/bbernstein/lacylights-go/internal/services/export"
	"github.com/glebarez/sqlite"
	"github.com/lucsky/cuid"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
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

func TestImportProject_ComplexExport(t *testing.T) {
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
	// Verify complex structure elements are preserved
	if len(parsed.FixtureDefinitions) != 1 {
		t.Errorf("Expected 1 fixture definition, got %d", len(parsed.FixtureDefinitions))
	}
	if len(parsed.FixtureInstances) != 1 {
		t.Errorf("Expected 1 fixture instance, got %d", len(parsed.FixtureInstances))
	}
	if len(parsed.Scenes) != 1 {
		t.Errorf("Expected 1 scene, got %d", len(parsed.Scenes))
	}
	if len(parsed.CueLists) != 1 {
		t.Errorf("Expected 1 cue list, got %d", len(parsed.CueLists))
	}
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

func TestImportProject_SceneOrderJSONParsing(t *testing.T) {
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

	// Verify JSON was created correctly with scene order
	parsed, err := export.ParseExportedProject(jsonStr)
	if err != nil {
		t.Fatalf("Failed to parse JSON: %v", err)
	}
	if len(parsed.Scenes) != 1 {
		t.Fatalf("Expected 1 scene, got %d", len(parsed.Scenes))
	}
	if len(parsed.Scenes[0].FixtureValues) != 1 {
		t.Fatalf("Expected 1 fixture value, got %d", len(parsed.Scenes[0].FixtureValues))
	}
	if parsed.Scenes[0].FixtureValues[0].SceneOrder == nil || *parsed.Scenes[0].FixtureValues[0].SceneOrder != 5 {
		t.Error("Expected scene order to be preserved")
	}
}

func TestImportProject_NotesJSONParsing(t *testing.T) {
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
}

func TestImportProject_WithModes(t *testing.T) {
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
}

func TestImportProject_ModesChannelNameFallbackJSONParsing(t *testing.T) {
	// Test that channels without RefID use name as fallback
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
}

func TestImportProject_WithModeNameAndChannelCount(t *testing.T) {
	// Test that fixture instances with ModeName and ChannelCount are properly preserved
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
}

func TestImportProject_FixtureInstanceNoModeNameJSONParsing(t *testing.T) {
	// Test that fixture instances without ModeName work correctly (nil stays nil)
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
}

func TestImportProject_LegacyChannelValuesFormat(t *testing.T) {
	// Test that the legacy channelValues array format is parsed correctly
	// This format was used before the sparse channels format was introduced
	legacyJSON := `{
		"version": "1.0.0",
		"project": {
			"originalId": "proj-1",
			"name": "Legacy Format Project"
		},
		"fixtureDefinitions": [],
		"fixtureInstances": [],
		"scenes": [
			{
				"refId": "scene-1",
				"name": "Test Scene",
				"fixtureValues": [
					{
						"fixtureRefId": "fixture-1",
						"channelValues": [255, 128, 64, 0]
					},
					{
						"fixtureRefId": "fixture-2",
						"channelValues": [100, 200, 150]
					}
				]
			}
		],
		"cueLists": []
	}`

	// Parse the legacy JSON
	parsed, err := export.ParseExportedProject(legacyJSON)
	if err != nil {
		t.Fatalf("Failed to parse legacy JSON: %v", err)
	}

	// Verify project name
	if parsed.Project.Name != "Legacy Format Project" {
		t.Errorf("Expected project name 'Legacy Format Project', got '%s'", parsed.Project.Name)
	}

	// Verify scene
	if len(parsed.Scenes) != 1 {
		t.Fatalf("Expected 1 scene, got %d", len(parsed.Scenes))
	}

	scene := parsed.Scenes[0]
	if len(scene.FixtureValues) != 2 {
		t.Fatalf("Expected 2 fixture values, got %d", len(scene.FixtureValues))
	}

	// Verify first fixture value has legacy channelValues (not sparse channels)
	fv1 := scene.FixtureValues[0]
	if fv1.FixtureRefID != "fixture-1" {
		t.Errorf("Expected fixture ref 'fixture-1', got '%s'", fv1.FixtureRefID)
	}
	// The sparse Channels should be empty
	if len(fv1.Channels) != 0 {
		t.Errorf("Expected empty Channels (sparse format), got %d items", len(fv1.Channels))
	}
	// The legacy ChannelValues should be populated
	if len(fv1.ChannelValues) != 4 {
		t.Fatalf("Expected 4 channel values, got %d", len(fv1.ChannelValues))
	}
	expectedValues := []int{255, 128, 64, 0}
	for i, expected := range expectedValues {
		if fv1.ChannelValues[i] != expected {
			t.Errorf("ChannelValues[%d]: expected %d, got %d", i, expected, fv1.ChannelValues[i])
		}
	}

	// Verify second fixture value
	fv2 := scene.FixtureValues[1]
	if len(fv2.ChannelValues) != 3 {
		t.Fatalf("Expected 3 channel values for fixture-2, got %d", len(fv2.ChannelValues))
	}
}

func TestExportedFixtureValue_ChannelValuesParsing(t *testing.T) {
	// Test that both formats can coexist and are parsed correctly
	testCases := []struct {
		name                  string
		json                  string
		expectedChannelsLen   int
		expectedChannelValues []int
	}{
		{
			name: "sparse_channels_format",
			json: `{
				"fixtureRefId": "fix-1",
				"channels": [
					{"offset": 0, "value": 255},
					{"offset": 2, "value": 128}
				]
			}`,
			expectedChannelsLen:   2,
			expectedChannelValues: nil,
		},
		{
			name: "legacy_channelValues_format",
			json: `{
				"fixtureRefId": "fix-1",
				"channelValues": [255, 128, 64]
			}`,
			expectedChannelsLen:   0,
			expectedChannelValues: []int{255, 128, 64},
		},
		{
			name: "both_formats_sparse_takes_precedence",
			json: `{
				"fixtureRefId": "fix-1",
				"channels": [{"offset": 0, "value": 100}],
				"channelValues": [255, 128]
			}`,
			expectedChannelsLen:   1,
			expectedChannelValues: []int{255, 128},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			var fv export.ExportedFixtureValue
			if err := json.Unmarshal([]byte(tc.json), &fv); err != nil {
				t.Fatalf("Failed to unmarshal: %v", err)
			}

			if len(fv.Channels) != tc.expectedChannelsLen {
				t.Errorf("Expected %d channels, got %d", tc.expectedChannelsLen, len(fv.Channels))
			}

			if tc.expectedChannelValues == nil {
				if len(fv.ChannelValues) != 0 {
					t.Errorf("Expected no channel values, got %d", len(fv.ChannelValues))
				}
			} else {
				if len(fv.ChannelValues) != len(tc.expectedChannelValues) {
					t.Fatalf("Expected %d channel values, got %d", len(tc.expectedChannelValues), len(fv.ChannelValues))
				}
				for i, expected := range tc.expectedChannelValues {
					if fv.ChannelValues[i] != expected {
						t.Errorf("ChannelValues[%d]: expected %d, got %d", i, expected, fv.ChannelValues[i])
					}
				}
			}
		})
	}
}

// Integration tests with in-memory database
// These tests require database setup and test actual import functionality

func setupTestDB(t *testing.T) (*gorm.DB, func()) {
	t.Helper()

	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		t.Fatalf("Failed to open in-memory database: %v", err)
	}

	err = db.AutoMigrate(
		&models.Project{},
		&models.FixtureDefinition{},
		&models.ChannelDefinition{},
		&models.FixtureMode{},
		&models.ModeChannel{},
		&models.FixtureInstance{},
		&models.InstanceChannel{},
		&models.Scene{},
		&models.FixtureValue{},
		&models.CueList{},
		&models.Cue{},
		&models.SceneBoard{},
		&models.SceneBoardButton{},
	)
	if err != nil {
		t.Fatalf("Failed to migrate database: %v", err)
	}

	cleanup := func() {
		sqlDB, err := db.DB()
		if err == nil {
			_ = sqlDB.Close()
		}
	}

	return db, cleanup
}

func TestImportProject_Integration_EmptyProject(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	projectRepo := repositories.NewProjectRepository(db)
	fixtureRepo := repositories.NewFixtureRepository(db)
	sceneRepo := repositories.NewSceneRepository(db)
	cueListRepo := repositories.NewCueListRepository(db)
	cueRepo := repositories.NewCueRepository(db)

	service := NewService(projectRepo, fixtureRepo, sceneRepo, cueListRepo, cueRepo)
	ctx := context.Background()

	exported := &export.ExportedProject{
		Version: "1.0",
		Project: &export.ExportProjectInfo{
			OriginalID: "proj-1",
			Name:       "Empty Project",
		},
	}

	jsonStr, err := exported.ToJSON()
	if err != nil {
		t.Fatalf("Failed to create JSON: %v", err)
	}

	projectID, stats, warnings, err := service.ImportProject(ctx, jsonStr, ImportOptions{
		Mode: ImportModeCreate,
	})
	if err != nil {
		t.Fatalf("ImportProject failed: %v", err)
	}

	if projectID == "" {
		t.Error("Expected project ID")
	}
	if stats == nil {
		t.Fatal("Expected stats")
	}
	if stats.FixtureDefinitionsCreated != 0 {
		t.Errorf("Expected 0 fixture definitions, got %d", stats.FixtureDefinitionsCreated)
	}
	if len(warnings) != 0 {
		t.Errorf("Expected no warnings, got %d", len(warnings))
	}

	// Verify project was created
	project, _ := projectRepo.FindByID(ctx, projectID)
	if project == nil {
		t.Fatal("Expected project to be created")
	}
	if project.Name != "Empty Project" {
		t.Errorf("Expected name 'Empty Project', got '%s'", project.Name)
	}
}

func TestImportProject_Integration_WithCustomName(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	projectRepo := repositories.NewProjectRepository(db)
	fixtureRepo := repositories.NewFixtureRepository(db)
	sceneRepo := repositories.NewSceneRepository(db)
	cueListRepo := repositories.NewCueListRepository(db)
	cueRepo := repositories.NewCueRepository(db)

	service := NewService(projectRepo, fixtureRepo, sceneRepo, cueListRepo, cueRepo)
	ctx := context.Background()

	exported := &export.ExportedProject{
		Version: "1.0",
		Project: &export.ExportProjectInfo{
			OriginalID: "proj-1",
			Name:       "Original Name",
		},
	}

	jsonStr, _ := exported.ToJSON()

	customName := "Custom Import Name"
	projectID, _, _, err := service.ImportProject(ctx, jsonStr, ImportOptions{
		Mode:        ImportModeCreate,
		ProjectName: &customName,
	})
	if err != nil {
		t.Fatalf("ImportProject failed: %v", err)
	}

	project, _ := projectRepo.FindByID(ctx, projectID)
	if project.Name != "Custom Import Name" {
		t.Errorf("Expected name 'Custom Import Name', got '%s'", project.Name)
	}
}

func TestImportProject_Integration_WithFixtureDefinition(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	projectRepo := repositories.NewProjectRepository(db)
	fixtureRepo := repositories.NewFixtureRepository(db)
	sceneRepo := repositories.NewSceneRepository(db)
	cueListRepo := repositories.NewCueListRepository(db)
	cueRepo := repositories.NewCueRepository(db)

	service := NewService(projectRepo, fixtureRepo, sceneRepo, cueListRepo, cueRepo)
	ctx := context.Background()

	shortName := "3CH"
	exported := &export.ExportedProject{
		Version: "1.0",
		Project: &export.ExportProjectInfo{
			OriginalID: "proj-1",
			Name:       "Fixture Definition Test",
		},
		FixtureDefinitions: []export.ExportedFixtureDefinition{
			{
				RefID:        "def-1",
				Manufacturer: "TestMfg",
				Model:        "TestModel",
				Type:         "LED_PAR",
				IsBuiltIn:    false,
				Channels: []export.ExportedChannelDefinition{
					{RefID: "ch-r", Name: "Red", Type: "COLOR", Offset: 0, MinValue: 0, MaxValue: 255, DefaultValue: 0},
					{RefID: "ch-g", Name: "Green", Type: "COLOR", Offset: 1, MinValue: 0, MaxValue: 255, DefaultValue: 0},
					{RefID: "ch-b", Name: "Blue", Type: "COLOR", Offset: 2, MinValue: 0, MaxValue: 255, DefaultValue: 0},
				},
				Modes: []export.ExportedFixtureMode{
					{
						RefID:        "mode-3ch",
						Name:         "3-channel",
						ShortName:    &shortName,
						ChannelCount: 3,
						ModeChannels: []export.ExportedModeChannel{
							{ChannelRefID: "ch-r", Offset: 0},
							{ChannelRefID: "ch-g", Offset: 1},
							{ChannelRefID: "ch-b", Offset: 2},
						},
					},
				},
			},
		},
	}

	jsonStr, _ := exported.ToJSON()

	projectID, stats, _, err := service.ImportProject(ctx, jsonStr, ImportOptions{
		Mode: ImportModeCreate,
	})
	if err != nil {
		t.Fatalf("ImportProject failed: %v", err)
	}

	if stats.FixtureDefinitionsCreated != 1 {
		t.Errorf("Expected 1 fixture definition, got %d", stats.FixtureDefinitionsCreated)
	}

	// Verify definition was created with channels
	def, _ := fixtureRepo.FindDefinitionByManufacturerModel(ctx, "TestMfg", "TestModel")
	if def == nil {
		t.Fatal("Expected definition to be created")
	}

	channels, _ := fixtureRepo.GetDefinitionChannels(ctx, def.ID)
	if len(channels) != 3 {
		t.Errorf("Expected 3 channels, got %d", len(channels))
	}

	// Verify modes were created
	modes, _ := fixtureRepo.GetDefinitionModes(ctx, def.ID)
	if len(modes) != 1 {
		t.Errorf("Expected 1 mode, got %d", len(modes))
	}
	if len(modes) > 0 && modes[0].Name != "3-channel" {
		t.Errorf("Expected mode name '3-channel', got '%s'", modes[0].Name)
	}

	// Verify mode channels
	if len(modes) > 0 {
		modeChannels, _ := fixtureRepo.GetModeChannels(ctx, modes[0].ID)
		if len(modeChannels) != 3 {
			t.Errorf("Expected 3 mode channels, got %d", len(modeChannels))
		}
	}

	// Verify project ID is returned
	if projectID == "" {
		t.Error("Expected project ID")
	}
}

func TestImportProject_Integration_WithFixtureInstances(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	projectRepo := repositories.NewProjectRepository(db)
	fixtureRepo := repositories.NewFixtureRepository(db)
	sceneRepo := repositories.NewSceneRepository(db)
	cueListRepo := repositories.NewCueListRepository(db)
	cueRepo := repositories.NewCueRepository(db)

	service := NewService(projectRepo, fixtureRepo, sceneRepo, cueListRepo, cueRepo)
	ctx := context.Background()

	modeName := "3-channel"
	channelCount := 3
	exported := &export.ExportedProject{
		Version: "1.0",
		Project: &export.ExportProjectInfo{
			OriginalID: "proj-1",
			Name:       "Fixture Instance Test",
		},
		FixtureDefinitions: []export.ExportedFixtureDefinition{
			{
				RefID:        "def-1",
				Manufacturer: "TestMfg",
				Model:        "TestModel",
				Type:         "LED_PAR",
				Channels: []export.ExportedChannelDefinition{
					{RefID: "ch-r", Name: "Red", Type: "COLOR", Offset: 0},
					{RefID: "ch-g", Name: "Green", Type: "COLOR", Offset: 1},
					{RefID: "ch-b", Name: "Blue", Type: "COLOR", Offset: 2},
				},
				Modes: []export.ExportedFixtureMode{
					{
						RefID:        "mode-3ch",
						Name:         "3-channel",
						ChannelCount: 3,
						ModeChannels: []export.ExportedModeChannel{
							{ChannelRefID: "ch-r", Offset: 0},
							{ChannelRefID: "ch-g", Offset: 1},
							{ChannelRefID: "ch-b", Offset: 2},
						},
					},
				},
			},
		},
		FixtureInstances: []export.ExportedFixtureInstance{
			{
				RefID:           "inst-1",
				Name:            "LED 1",
				DefinitionRefID: "def-1",
				ModeName:        &modeName,
				ChannelCount:    &channelCount,
				Universe:        1,
				StartChannel:    1,
				Tags:            []string{"front", "wash"},
			},
			{
				RefID:           "inst-2",
				Name:            "LED 2",
				DefinitionRefID: "def-1",
				ModeName:        &modeName,
				ChannelCount:    &channelCount,
				Universe:        1,
				StartChannel:    4,
				Tags:            []string{"back"},
			},
		},
	}

	jsonStr, _ := exported.ToJSON()

	projectID, stats, _, err := service.ImportProject(ctx, jsonStr, ImportOptions{
		Mode: ImportModeCreate,
	})
	if err != nil {
		t.Fatalf("ImportProject failed: %v", err)
	}

	if stats.FixtureInstancesCreated != 2 {
		t.Errorf("Expected 2 fixture instances, got %d", stats.FixtureInstancesCreated)
	}

	// Verify instances were created
	fixtures, _ := fixtureRepo.FindByProjectID(ctx, projectID)
	if len(fixtures) != 2 {
		t.Errorf("Expected 2 fixtures, got %d", len(fixtures))
	}

	// Verify ModeName was preserved
	for _, f := range fixtures {
		if f.ModeName == nil || *f.ModeName != "3-channel" {
			t.Errorf("Expected ModeName '3-channel', got %v", f.ModeName)
		}
	}
}

func TestImportProject_Integration_WithScenes(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	projectRepo := repositories.NewProjectRepository(db)
	fixtureRepo := repositories.NewFixtureRepository(db)
	sceneRepo := repositories.NewSceneRepository(db)
	cueListRepo := repositories.NewCueListRepository(db)
	cueRepo := repositories.NewCueRepository(db)

	service := NewService(projectRepo, fixtureRepo, sceneRepo, cueListRepo, cueRepo)
	ctx := context.Background()

	sceneOrder := 0
	exported := &export.ExportedProject{
		Version: "1.0",
		Project: &export.ExportProjectInfo{
			OriginalID: "proj-1",
			Name:       "Scene Test",
		},
		FixtureDefinitions: []export.ExportedFixtureDefinition{
			{
				RefID:        "def-1",
				Manufacturer: "T",
				Model:        "M",
				Type:         "LED",
				Channels: []export.ExportedChannelDefinition{
					{Name: "Dimmer", Type: "INTENSITY", Offset: 0},
				},
			},
		},
		FixtureInstances: []export.ExportedFixtureInstance{
			{
				RefID:           "inst-1",
				Name:            "F1",
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
						SceneOrder: &sceneOrder,
					},
				},
			},
		},
	}

	jsonStr, _ := exported.ToJSON()

	projectID, stats, _, err := service.ImportProject(ctx, jsonStr, ImportOptions{
		Mode: ImportModeCreate,
	})
	if err != nil {
		t.Fatalf("ImportProject failed: %v", err)
	}

	if stats.ScenesCreated != 1 {
		t.Errorf("Expected 1 scene, got %d", stats.ScenesCreated)
	}

	// Verify scene was created
	scenes, _ := sceneRepo.FindByProjectID(ctx, projectID)
	if len(scenes) != 1 {
		t.Errorf("Expected 1 scene, got %d", len(scenes))
	}
	if len(scenes) > 0 && scenes[0].Name != "Test Scene" {
		t.Errorf("Expected scene name 'Test Scene', got '%s'", scenes[0].Name)
	}

	// Verify fixture values
	if len(scenes) > 0 {
		fvs, _ := sceneRepo.GetFixtureValues(ctx, scenes[0].ID)
		if len(fvs) != 1 {
			t.Errorf("Expected 1 fixture value, got %d", len(fvs))
		}
	}
}

func TestImportProject_Integration_WithCueLists(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	projectRepo := repositories.NewProjectRepository(db)
	fixtureRepo := repositories.NewFixtureRepository(db)
	sceneRepo := repositories.NewSceneRepository(db)
	cueListRepo := repositories.NewCueListRepository(db)
	cueRepo := repositories.NewCueRepository(db)

	service := NewService(projectRepo, fixtureRepo, sceneRepo, cueListRepo, cueRepo)
	ctx := context.Background()

	followTime := 2.0
	easingType := "LINEAR"
	notes := "Opening"

	exported := &export.ExportedProject{
		Version: "1.0",
		Project: &export.ExportProjectInfo{
			OriginalID: "proj-1",
			Name:       "CueList Test",
		},
		Scenes: []export.ExportedScene{
			{
				RefID: "scene-1",
				Name:  "Scene 1",
			},
		},
		CueLists: []export.ExportedCueList{
			{
				RefID: "cl-1",
				Name:  "Main CueList",
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
						Notes:       &notes,
					},
				},
			},
		},
	}

	jsonStr, _ := exported.ToJSON()

	projectID, stats, _, err := service.ImportProject(ctx, jsonStr, ImportOptions{
		Mode: ImportModeCreate,
	})
	if err != nil {
		t.Fatalf("ImportProject failed: %v", err)
	}

	if stats.CueListsCreated != 1 {
		t.Errorf("Expected 1 cue list, got %d", stats.CueListsCreated)
	}
	if stats.CuesCreated != 1 {
		t.Errorf("Expected 1 cue, got %d", stats.CuesCreated)
	}

	// Verify cue list was created
	cueLists, _ := cueListRepo.FindByProjectID(ctx, projectID)
	if len(cueLists) != 1 {
		t.Errorf("Expected 1 cue list, got %d", len(cueLists))
	}
	if len(cueLists) > 0 && !cueLists[0].Loop {
		t.Error("Expected loop to be true")
	}

	// Verify cue was created
	if len(cueLists) > 0 {
		cues, _ := cueRepo.FindByCueListID(ctx, cueLists[0].ID)
		if len(cues) != 1 {
			t.Errorf("Expected 1 cue, got %d", len(cues))
		}
	}
}

func TestImportProject_Integration_ExistingDefinitionModeImport(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	projectRepo := repositories.NewProjectRepository(db)
	fixtureRepo := repositories.NewFixtureRepository(db)
	sceneRepo := repositories.NewSceneRepository(db)
	cueListRepo := repositories.NewCueListRepository(db)
	cueRepo := repositories.NewCueRepository(db)

	service := NewService(projectRepo, fixtureRepo, sceneRepo, cueListRepo, cueRepo)
	ctx := context.Background()

	// Pre-create a fixture definition WITHOUT modes
	def := &models.FixtureDefinition{
		ID:           cuid.New(),
		Manufacturer: "TestMfg",
		Model:        "TestModel",
		Type:         "LED_PAR",
	}
	_ = fixtureRepo.CreateDefinition(ctx, def)

	// Create channels for it
	ch1 := &models.ChannelDefinition{ID: cuid.New(), Name: "Red", Type: "COLOR", Offset: 0, DefinitionID: def.ID}
	ch2 := &models.ChannelDefinition{ID: cuid.New(), Name: "Green", Type: "COLOR", Offset: 1, DefinitionID: def.ID}
	ch3 := &models.ChannelDefinition{ID: cuid.New(), Name: "Blue", Type: "COLOR", Offset: 2, DefinitionID: def.ID}
	db.Create(ch1)
	db.Create(ch2)
	db.Create(ch3)

	// Now import a project that references this definition with modes
	exported := &export.ExportedProject{
		Version: "1.0",
		Project: &export.ExportProjectInfo{
			OriginalID: "proj-1",
			Name:       "Mode Merge Test",
		},
		FixtureDefinitions: []export.ExportedFixtureDefinition{
			{
				RefID:        "def-1",
				Manufacturer: "TestMfg",
				Model:        "TestModel",
				Type:         "LED_PAR",
				IsBuiltIn:    false,
				Channels: []export.ExportedChannelDefinition{
					{RefID: "ch-r", Name: "Red", Type: "COLOR", Offset: 0},
					{RefID: "ch-g", Name: "Green", Type: "COLOR", Offset: 1},
					{RefID: "ch-b", Name: "Blue", Type: "COLOR", Offset: 2},
				},
				Modes: []export.ExportedFixtureMode{
					{
						RefID:        "mode-3ch",
						Name:         "3-channel",
						ChannelCount: 3,
						ModeChannels: []export.ExportedModeChannel{
							{ChannelRefID: "ch-r", Offset: 0},
							{ChannelRefID: "ch-g", Offset: 1},
							{ChannelRefID: "ch-b", Offset: 2},
						},
					},
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
	}

	jsonStr, _ := exported.ToJSON()

	// Use SKIP strategy to use the existing definition
	_, stats, _, err := service.ImportProject(ctx, jsonStr, ImportOptions{
		Mode:                    ImportModeCreate,
		FixtureConflictStrategy: FixtureConflictSkip,
	})
	if err != nil {
		t.Fatalf("ImportProject failed: %v", err)
	}

	// Definition should have been skipped (0 created) but modes should have been merged
	if stats.FixtureDefinitionsCreated != 0 {
		t.Errorf("Expected 0 fixture definitions created (skipped), got %d", stats.FixtureDefinitionsCreated)
	}

	// Verify modes were merged into existing definition
	modes, _ := fixtureRepo.GetDefinitionModes(ctx, def.ID)
	if len(modes) != 1 {
		t.Errorf("Expected 1 mode to be merged, got %d", len(modes))
	}
	if len(modes) > 0 && modes[0].Name != "3-channel" {
		t.Errorf("Expected mode name '3-channel', got '%s'", modes[0].Name)
	}
}

func TestImportProject_Integration_LegacyChannelValues(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	projectRepo := repositories.NewProjectRepository(db)
	fixtureRepo := repositories.NewFixtureRepository(db)
	sceneRepo := repositories.NewSceneRepository(db)
	cueListRepo := repositories.NewCueListRepository(db)
	cueRepo := repositories.NewCueRepository(db)

	service := NewService(projectRepo, fixtureRepo, sceneRepo, cueListRepo, cueRepo)
	ctx := context.Background()

	// Use raw JSON with legacy channelValues format
	jsonStr := `{
		"version": "1.0",
		"project": {
			"originalId": "proj-1",
			"name": "Legacy Format Test"
		},
		"fixtureDefinitions": [
			{
				"refId": "def-1",
				"manufacturer": "TestMfg",
				"model": "TestModel",
				"type": "LED_PAR",
				"channels": [
					{"name": "Red", "type": "COLOR", "offset": 0},
					{"name": "Green", "type": "COLOR", "offset": 1},
					{"name": "Blue", "type": "COLOR", "offset": 2}
				]
			}
		],
		"fixtureInstances": [
			{
				"refId": "inst-1",
				"name": "LED 1",
				"definitionRefId": "def-1",
				"universe": 1,
				"startChannel": 1
			}
		],
		"scenes": [
			{
				"refId": "scene-1",
				"name": "Legacy Scene",
				"fixtureValues": [
					{
						"fixtureRefId": "inst-1",
						"channelValues": [255, 128, 64]
					}
				]
			}
		]
	}`

	projectID, stats, _, err := service.ImportProject(ctx, jsonStr, ImportOptions{
		Mode: ImportModeCreate,
	})
	if err != nil {
		t.Fatalf("ImportProject failed: %v", err)
	}

	if stats.ScenesCreated != 1 {
		t.Errorf("Expected 1 scene, got %d", stats.ScenesCreated)
	}

	// Verify scene fixture values were converted from legacy format
	scenes, _ := sceneRepo.FindByProjectID(ctx, projectID)
	if len(scenes) != 1 {
		t.Fatalf("Expected 1 scene, got %d", len(scenes))
	}

	fvs, _ := sceneRepo.GetFixtureValues(ctx, scenes[0].ID)
	if len(fvs) != 1 {
		t.Fatalf("Expected 1 fixture value, got %d", len(fvs))
	}

	// Verify the channels were converted from array to sparse format
	// The Channels field in the database should contain the sparse JSON format
	if fvs[0].Channels == "" {
		t.Error("Expected Channels to be populated")
	}
}

func TestImportProject_Integration_ReplaceStrategy(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	projectRepo := repositories.NewProjectRepository(db)
	fixtureRepo := repositories.NewFixtureRepository(db)
	sceneRepo := repositories.NewSceneRepository(db)
	cueListRepo := repositories.NewCueListRepository(db)
	cueRepo := repositories.NewCueRepository(db)

	service := NewService(projectRepo, fixtureRepo, sceneRepo, cueListRepo, cueRepo)
	ctx := context.Background()

	// Pre-create a fixture definition
	def := &models.FixtureDefinition{
		ID:           cuid.New(),
		Manufacturer: "ExistingMfg",
		Model:        "ExistingModel",
		Type:         "LED_PAR",
	}
	_ = fixtureRepo.CreateDefinition(ctx, def)

	// Create channels for existing definition
	ch1 := &models.ChannelDefinition{ID: cuid.New(), Name: "Dimmer", Type: "INTENSITY", Offset: 0, DefinitionID: def.ID}
	db.Create(ch1)

	// Now import with REPLACE strategy
	exported := &export.ExportedProject{
		Version: "1.0",
		Project: &export.ExportProjectInfo{
			OriginalID: "proj-1",
			Name:       "Replace Strategy Test",
		},
		FixtureDefinitions: []export.ExportedFixtureDefinition{
			{
				RefID:        "def-1",
				Manufacturer: "ExistingMfg",
				Model:        "ExistingModel",
				Type:         "LED_PAR",
				Channels: []export.ExportedChannelDefinition{
					{RefID: "ch-d", Name: "Dimmer", Type: "INTENSITY", Offset: 0},
				},
				Modes: []export.ExportedFixtureMode{
					{
						RefID:        "mode-1ch",
						Name:         "1-channel",
						ChannelCount: 1,
						ModeChannels: []export.ExportedModeChannel{
							{ChannelRefID: "ch-d", Offset: 0},
						},
					},
				},
			},
		},
		FixtureInstances: []export.ExportedFixtureInstance{
			{
				RefID:           "inst-1",
				Name:            "Fixture 1",
				DefinitionRefID: "def-1",
				Universe:        1,
				StartChannel:    1,
			},
		},
	}

	jsonStr, _ := exported.ToJSON()

	_, stats, _, err := service.ImportProject(ctx, jsonStr, ImportOptions{
		Mode:                    ImportModeCreate,
		FixtureConflictStrategy: FixtureConflictReplace,
	})
	if err != nil {
		t.Fatalf("ImportProject failed: %v", err)
	}

	// Definition should not be created (replaced with existing)
	if stats.FixtureDefinitionsCreated != 0 {
		t.Errorf("Expected 0 fixture definitions (replace uses existing), got %d", stats.FixtureDefinitionsCreated)
	}

	// Modes should have been merged into existing definition
	modes, _ := fixtureRepo.GetDefinitionModes(ctx, def.ID)
	if len(modes) != 1 {
		t.Errorf("Expected 1 mode to be merged, got %d", len(modes))
	}
}

func TestImportProject_Integration_RenameStrategy(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	projectRepo := repositories.NewProjectRepository(db)
	fixtureRepo := repositories.NewFixtureRepository(db)
	sceneRepo := repositories.NewSceneRepository(db)
	cueListRepo := repositories.NewCueListRepository(db)
	cueRepo := repositories.NewCueRepository(db)

	service := NewService(projectRepo, fixtureRepo, sceneRepo, cueListRepo, cueRepo)
	ctx := context.Background()

	// Pre-create a fixture definition
	def := &models.FixtureDefinition{
		ID:           cuid.New(),
		Manufacturer: "RenameMfg",
		Model:        "RenameModel",
		Type:         "LED_PAR",
	}
	_ = fixtureRepo.CreateDefinition(ctx, def)

	// Now import with RENAME strategy - should create new definition
	exported := &export.ExportedProject{
		Version: "1.0",
		Project: &export.ExportProjectInfo{
			OriginalID: "proj-1",
			Name:       "Rename Strategy Test",
		},
		FixtureDefinitions: []export.ExportedFixtureDefinition{
			{
				RefID:        "def-1",
				Manufacturer: "RenameMfg",
				Model:        "RenameModel",
				Type:         "LED_PAR",
				Channels: []export.ExportedChannelDefinition{
					{RefID: "ch-d", Name: "Dimmer", Type: "INTENSITY", Offset: 0},
				},
			},
		},
		FixtureInstances: []export.ExportedFixtureInstance{
			{
				RefID:           "inst-1",
				Name:            "Fixture 1",
				DefinitionRefID: "def-1",
				Universe:        1,
				StartChannel:    1,
			},
		},
	}

	jsonStr, _ := exported.ToJSON()

	_, stats, _, err := service.ImportProject(ctx, jsonStr, ImportOptions{
		Mode:                    ImportModeCreate,
		FixtureConflictStrategy: FixtureConflictRename,
	})
	if err != nil {
		t.Fatalf("ImportProject failed: %v", err)
	}

	// With RENAME, a new definition should be created
	if stats.FixtureDefinitionsCreated != 1 {
		t.Errorf("Expected 1 fixture definition (rename creates new), got %d", stats.FixtureDefinitionsCreated)
	}
}

func TestImportProject_Integration_UnknownSceneInCue(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	projectRepo := repositories.NewProjectRepository(db)
	fixtureRepo := repositories.NewFixtureRepository(db)
	sceneRepo := repositories.NewSceneRepository(db)
	cueListRepo := repositories.NewCueListRepository(db)
	cueRepo := repositories.NewCueRepository(db)

	service := NewService(projectRepo, fixtureRepo, sceneRepo, cueListRepo, cueRepo)
	ctx := context.Background()

	// Import project with cue referencing non-existent scene
	exported := &export.ExportedProject{
		Version: "1.0",
		Project: &export.ExportProjectInfo{
			OriginalID: "proj-1",
			Name:       "Unknown Scene Test",
		},
		CueLists: []export.ExportedCueList{
			{
				RefID: "cl-1",
				Name:  "Test CueList",
				Cues: []export.ExportedCue{
					{
						Name:        "Cue with unknown scene",
						CueNumber:   1.0,
						SceneRefID:  "nonexistent-scene",
						FadeInTime:  2.0,
						FadeOutTime: 1.0,
					},
				},
			},
		},
	}

	jsonStr, _ := exported.ToJSON()

	_, stats, warnings, err := service.ImportProject(ctx, jsonStr, ImportOptions{
		Mode: ImportModeCreate,
	})
	if err != nil {
		t.Fatalf("ImportProject failed: %v", err)
	}

	// Cue list should be created but cue should be skipped with warning
	if stats.CueListsCreated != 1 {
		t.Errorf("Expected 1 cue list, got %d", stats.CueListsCreated)
	}
	if stats.CuesCreated != 0 {
		t.Errorf("Expected 0 cues (skipped due to unknown scene), got %d", stats.CuesCreated)
	}

	// Should have a warning about unknown scene
	found := false
	for _, w := range warnings {
		if w == "Skipping cue with unknown scene in cue list: Test CueList" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("Expected warning about unknown scene, got: %v", warnings)
	}
}

func TestImportProject_Integration_UnknownFixtureDefinition(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	projectRepo := repositories.NewProjectRepository(db)
	fixtureRepo := repositories.NewFixtureRepository(db)
	sceneRepo := repositories.NewSceneRepository(db)
	cueListRepo := repositories.NewCueListRepository(db)
	cueRepo := repositories.NewCueRepository(db)

	service := NewService(projectRepo, fixtureRepo, sceneRepo, cueListRepo, cueRepo)
	ctx := context.Background()

	// Import fixture instance referencing unknown definition
	exported := &export.ExportedProject{
		Version: "1.0",
		Project: &export.ExportProjectInfo{
			OriginalID: "proj-1",
			Name:       "Unknown Definition Test",
		},
		FixtureInstances: []export.ExportedFixtureInstance{
			{
				RefID:           "inst-1",
				Name:            "Orphan Fixture",
				DefinitionRefID: "nonexistent-def",
				Universe:        1,
				StartChannel:    1,
			},
		},
	}

	jsonStr, _ := exported.ToJSON()

	_, stats, warnings, err := service.ImportProject(ctx, jsonStr, ImportOptions{
		Mode: ImportModeCreate,
	})
	if err != nil {
		t.Fatalf("ImportProject failed: %v", err)
	}

	// Fixture should be skipped with warning
	if stats.FixtureInstancesCreated != 0 {
		t.Errorf("Expected 0 fixtures (unknown definition), got %d", stats.FixtureInstancesCreated)
	}

	// Should have a warning
	found := false
	for _, w := range warnings {
		if w == "Skipping fixture instance with unknown definition: Orphan Fixture" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("Expected warning about unknown definition, got: %v", warnings)
	}
}

func TestImportProject_Integration_UnknownFixtureInScene(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	projectRepo := repositories.NewProjectRepository(db)
	fixtureRepo := repositories.NewFixtureRepository(db)
	sceneRepo := repositories.NewSceneRepository(db)
	cueListRepo := repositories.NewCueListRepository(db)
	cueRepo := repositories.NewCueRepository(db)

	service := NewService(projectRepo, fixtureRepo, sceneRepo, cueListRepo, cueRepo)
	ctx := context.Background()

	// Import scene with fixture value referencing unknown fixture
	exported := &export.ExportedProject{
		Version: "1.0",
		Project: &export.ExportProjectInfo{
			OriginalID: "proj-1",
			Name:       "Unknown Fixture in Scene Test",
		},
		Scenes: []export.ExportedScene{
			{
				RefID: "scene-1",
				Name:  "Scene with unknown fixture",
				FixtureValues: []export.ExportedFixtureValue{
					{
						FixtureRefID: "nonexistent-fixture",
						Channels: []export.ExportedChannelValue{
							{Offset: 0, Value: 255},
						},
					},
				},
			},
		},
	}

	jsonStr, _ := exported.ToJSON()

	_, stats, warnings, err := service.ImportProject(ctx, jsonStr, ImportOptions{
		Mode: ImportModeCreate,
	})
	if err != nil {
		t.Fatalf("ImportProject failed: %v", err)
	}

	// Scene should be created but fixture value should be skipped
	if stats.ScenesCreated != 1 {
		t.Errorf("Expected 1 scene, got %d", stats.ScenesCreated)
	}

	// Should have a warning about unknown fixture
	found := false
	for _, w := range warnings {
		if w == "Skipping fixture value with unknown fixture 'nonexistent-fixture' in scene 'Scene with unknown fixture'" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("Expected warning about unknown fixture, got: %v", warnings)
	}
}

func TestImportProject_Integration_BuiltInDefinitionWithModes(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	projectRepo := repositories.NewProjectRepository(db)
	fixtureRepo := repositories.NewFixtureRepository(db)
	sceneRepo := repositories.NewSceneRepository(db)
	cueListRepo := repositories.NewCueListRepository(db)
	cueRepo := repositories.NewCueRepository(db)

	service := NewService(projectRepo, fixtureRepo, sceneRepo, cueListRepo, cueRepo)
	ctx := context.Background()

	// Pre-create a "built-in" fixture definition
	def := &models.FixtureDefinition{
		ID:           cuid.New(),
		Manufacturer: "BuiltInMfg",
		Model:        "BuiltInModel",
		Type:         "LED_PAR",
		IsBuiltIn:    true,
	}
	_ = fixtureRepo.CreateDefinition(ctx, def)

	// Create channels for it
	ch1 := &models.ChannelDefinition{ID: cuid.New(), Name: "Red", Type: "COLOR", Offset: 0, DefinitionID: def.ID}
	ch2 := &models.ChannelDefinition{ID: cuid.New(), Name: "Green", Type: "COLOR", Offset: 1, DefinitionID: def.ID}
	db.Create(ch1)
	db.Create(ch2)

	// Import project with built-in definition and modes
	exported := &export.ExportedProject{
		Version: "1.0",
		Project: &export.ExportProjectInfo{
			OriginalID: "proj-1",
			Name:       "BuiltIn Mode Test",
		},
		FixtureDefinitions: []export.ExportedFixtureDefinition{
			{
				RefID:        "def-1",
				Manufacturer: "BuiltInMfg",
				Model:        "BuiltInModel",
				Type:         "LED_PAR",
				IsBuiltIn:    true,
				Channels: []export.ExportedChannelDefinition{
					{RefID: "ch-r", Name: "Red", Type: "COLOR", Offset: 0},
					{RefID: "ch-g", Name: "Green", Type: "COLOR", Offset: 1},
				},
				Modes: []export.ExportedFixtureMode{
					{
						RefID:        "mode-2ch",
						Name:         "2-channel",
						ChannelCount: 2,
						ModeChannels: []export.ExportedModeChannel{
							{ChannelRefID: "ch-r", Offset: 0},
							{ChannelRefID: "ch-g", Offset: 1},
						},
					},
				},
			},
		},
		FixtureInstances: []export.ExportedFixtureInstance{
			{
				RefID:           "inst-1",
				Name:            "BuiltIn Fixture",
				DefinitionRefID: "def-1",
				Universe:        1,
				StartChannel:    1,
			},
		},
	}

	jsonStr, _ := exported.ToJSON()

	// Import with ImportBuiltInFixtures=false - should use existing built-in definition
	_, stats, _, err := service.ImportProject(ctx, jsonStr, ImportOptions{
		Mode:                  ImportModeCreate,
		ImportBuiltInFixtures: false,
	})
	if err != nil {
		t.Fatalf("ImportProject failed: %v", err)
	}

	// No new definition should be created
	if stats.FixtureDefinitionsCreated != 0 {
		t.Errorf("Expected 0 definitions (using built-in), got %d", stats.FixtureDefinitionsCreated)
	}

	// Modes should have been merged into existing built-in definition
	modes, _ := fixtureRepo.GetDefinitionModes(ctx, def.ID)
	if len(modes) != 1 {
		t.Errorf("Expected 1 mode merged, got %d", len(modes))
	}
}

func TestImportProject_Integration_FixtureInstanceWithoutChannels(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	projectRepo := repositories.NewProjectRepository(db)
	fixtureRepo := repositories.NewFixtureRepository(db)
	sceneRepo := repositories.NewSceneRepository(db)
	cueListRepo := repositories.NewCueListRepository(db)
	cueRepo := repositories.NewCueRepository(db)

	service := NewService(projectRepo, fixtureRepo, sceneRepo, cueListRepo, cueRepo)
	ctx := context.Background()

	// Import fixture instance without explicit instance channels
	// This should copy channels from definition
	exported := &export.ExportedProject{
		Version: "1.0",
		Project: &export.ExportProjectInfo{
			OriginalID: "proj-1",
			Name:       "No Instance Channels Test",
		},
		FixtureDefinitions: []export.ExportedFixtureDefinition{
			{
				RefID:        "def-1",
				Manufacturer: "TestMfg",
				Model:        "TestModel",
				Type:         "LED_PAR",
				Channels: []export.ExportedChannelDefinition{
					{Name: "Red", Type: "COLOR", Offset: 0},
					{Name: "Green", Type: "COLOR", Offset: 1},
					{Name: "Blue", Type: "COLOR", Offset: 2},
				},
			},
		},
		FixtureInstances: []export.ExportedFixtureInstance{
			{
				RefID:           "inst-1",
				Name:            "Fixture without channels",
				DefinitionRefID: "def-1",
				Universe:        1,
				StartChannel:    1,
				// No InstanceChannels specified
				// No ChannelCount specified
			},
		},
	}

	jsonStr, _ := exported.ToJSON()

	projectID, stats, _, err := service.ImportProject(ctx, jsonStr, ImportOptions{
		Mode: ImportModeCreate,
	})
	if err != nil {
		t.Fatalf("ImportProject failed: %v", err)
	}

	if stats.FixtureInstancesCreated != 1 {
		t.Errorf("Expected 1 fixture instance, got %d", stats.FixtureInstancesCreated)
	}

	// Verify instance was created with channels from definition
	fixtures, _ := fixtureRepo.FindByProjectID(ctx, projectID)
	if len(fixtures) != 1 {
		t.Fatalf("Expected 1 fixture, got %d", len(fixtures))
	}

	// Channel count should be set from definition channels
	if fixtures[0].ChannelCount == nil || *fixtures[0].ChannelCount != 3 {
		t.Errorf("Expected ChannelCount 3, got %v", fixtures[0].ChannelCount)
	}

	// Verify instance channels were created
	instanceChannels, _ := fixtureRepo.GetInstanceChannels(ctx, fixtures[0].ID)
	if len(instanceChannels) != 3 {
		t.Errorf("Expected 3 instance channels (from definition), got %d", len(instanceChannels))
	}
}

func TestImportProject_Integration_MergeWithExistingProject(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	projectRepo := repositories.NewProjectRepository(db)
	fixtureRepo := repositories.NewFixtureRepository(db)
	sceneRepo := repositories.NewSceneRepository(db)
	cueListRepo := repositories.NewCueListRepository(db)
	cueRepo := repositories.NewCueRepository(db)

	service := NewService(projectRepo, fixtureRepo, sceneRepo, cueListRepo, cueRepo)
	ctx := context.Background()

	// Create existing project
	existingProject := &models.Project{
		ID:   cuid.New(),
		Name: "Existing Project",
	}
	_ = projectRepo.Create(ctx, existingProject)

	// Import into existing project with MERGE mode
	exported := &export.ExportedProject{
		Version: "1.0",
		Project: &export.ExportProjectInfo{
			OriginalID: "proj-1",
			Name:       "Imported Project",
		},
		FixtureDefinitions: []export.ExportedFixtureDefinition{
			{
				RefID:        "def-1",
				Manufacturer: "MergeMfg",
				Model:        "MergeModel",
				Type:         "LED",
				Channels: []export.ExportedChannelDefinition{
					{Name: "Dimmer", Type: "INTENSITY", Offset: 0},
				},
			},
		},
		FixtureInstances: []export.ExportedFixtureInstance{
			{
				RefID:           "inst-1",
				Name:            "Merged Fixture",
				DefinitionRefID: "def-1",
				Universe:        1,
				StartChannel:    1,
			},
		},
	}

	jsonStr, _ := exported.ToJSON()

	projectID, stats, _, err := service.ImportProject(ctx, jsonStr, ImportOptions{
		Mode:            ImportModeMerge,
		TargetProjectID: &existingProject.ID,
	})
	if err != nil {
		t.Fatalf("ImportProject failed: %v", err)
	}

	// Should use the existing project ID
	if projectID != existingProject.ID {
		t.Errorf("Expected project ID %s, got %s", existingProject.ID, projectID)
	}

	if stats.FixtureDefinitionsCreated != 1 {
		t.Errorf("Expected 1 definition created, got %d", stats.FixtureDefinitionsCreated)
	}
	if stats.FixtureInstancesCreated != 1 {
		t.Errorf("Expected 1 instance created, got %d", stats.FixtureInstancesCreated)
	}
}

func TestImportProject_Integration_MergeProjectNotFound(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	projectRepo := repositories.NewProjectRepository(db)
	fixtureRepo := repositories.NewFixtureRepository(db)
	sceneRepo := repositories.NewSceneRepository(db)
	cueListRepo := repositories.NewCueListRepository(db)
	cueRepo := repositories.NewCueRepository(db)

	service := NewService(projectRepo, fixtureRepo, sceneRepo, cueListRepo, cueRepo)
	ctx := context.Background()

	exported := &export.ExportedProject{
		Version: "1.0",
		Project: &export.ExportProjectInfo{
			OriginalID: "proj-1",
			Name:       "Test Project",
		},
	}

	jsonStr, _ := exported.ToJSON()

	nonexistentID := "nonexistent-project-id"
	projectID, stats, _, err := service.ImportProject(ctx, jsonStr, ImportOptions{
		Mode:            ImportModeMerge,
		TargetProjectID: &nonexistentID,
	})
	if err != nil {
		t.Fatalf("ImportProject should not error on project not found: %v", err)
	}

	// Should return empty since project not found
	if projectID != "" {
		t.Errorf("Expected empty project ID when target not found, got %s", projectID)
	}
	if stats != nil {
		t.Error("Expected nil stats when target not found")
	}
}

func TestImportProject_Integration_ModeChannelUnknownReference(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	projectRepo := repositories.NewProjectRepository(db)
	fixtureRepo := repositories.NewFixtureRepository(db)
	sceneRepo := repositories.NewSceneRepository(db)
	cueListRepo := repositories.NewCueListRepository(db)
	cueRepo := repositories.NewCueRepository(db)

	service := NewService(projectRepo, fixtureRepo, sceneRepo, cueListRepo, cueRepo)
	ctx := context.Background()

	// Import definition with mode channel referencing non-existent channel
	exported := &export.ExportedProject{
		Version: "1.0",
		Project: &export.ExportProjectInfo{
			OriginalID: "proj-1",
			Name:       "Bad Mode Channel Test",
		},
		FixtureDefinitions: []export.ExportedFixtureDefinition{
			{
				RefID:        "def-1",
				Manufacturer: "TestMfg",
				Model:        "TestModel",
				Type:         "LED_PAR",
				Channels: []export.ExportedChannelDefinition{
					{RefID: "ch-r", Name: "Red", Type: "COLOR", Offset: 0},
				},
				Modes: []export.ExportedFixtureMode{
					{
						RefID:        "mode-bad",
						Name:         "Bad Mode",
						ChannelCount: 2,
						ModeChannels: []export.ExportedModeChannel{
							{ChannelRefID: "ch-r", Offset: 0},
							{ChannelRefID: "ch-nonexistent", Offset: 1}, // Unknown channel
						},
					},
				},
			},
		},
	}

	jsonStr, _ := exported.ToJSON()

	_, stats, warnings, err := service.ImportProject(ctx, jsonStr, ImportOptions{
		Mode: ImportModeCreate,
	})
	if err != nil {
		t.Fatalf("ImportProject failed: %v", err)
	}

	if stats.FixtureDefinitionsCreated != 1 {
		t.Errorf("Expected 1 definition, got %d", stats.FixtureDefinitionsCreated)
	}

	// Should have a warning about unknown channel reference
	found := false
	for _, w := range warnings {
		if w == "Mode channel references unknown channel: ch-nonexistent" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("Expected warning about unknown channel, got: %v", warnings)
	}
}

func TestNewServiceWithSceneBoards(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	projectRepo := repositories.NewProjectRepository(db)
	fixtureRepo := repositories.NewFixtureRepository(db)
	sceneRepo := repositories.NewSceneRepository(db)
	cueListRepo := repositories.NewCueListRepository(db)
	cueRepo := repositories.NewCueRepository(db)
	sceneBoardRepo := repositories.NewSceneBoardRepository(db)

	// Test with scene board repo
	service := NewServiceWithSceneBoards(projectRepo, fixtureRepo, sceneRepo, cueListRepo, cueRepo, sceneBoardRepo)
	if service == nil {
		t.Fatal("Expected service to be created")
	}
	if service.sceneBoardRepo == nil {
		t.Error("Expected sceneBoardRepo to be set")
	}

	// Test without scene board repo (original constructor)
	serviceOrig := NewService(projectRepo, fixtureRepo, sceneRepo, cueListRepo, cueRepo)
	if serviceOrig == nil {
		t.Fatal("Expected original service to be created")
	}
	if serviceOrig.sceneBoardRepo != nil {
		t.Error("Expected sceneBoardRepo to be nil for original constructor")
	}
}

func TestImportProject_Integration_WithSceneBoards(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	projectRepo := repositories.NewProjectRepository(db)
	fixtureRepo := repositories.NewFixtureRepository(db)
	sceneRepo := repositories.NewSceneRepository(db)
	cueListRepo := repositories.NewCueListRepository(db)
	cueRepo := repositories.NewCueRepository(db)
	sceneBoardRepo := repositories.NewSceneBoardRepository(db)

	service := NewServiceWithSceneBoards(projectRepo, fixtureRepo, sceneRepo, cueListRepo, cueRepo, sceneBoardRepo)
	ctx := context.Background()

	description := "Main board for lighting control"
	gridSize := 8
	exported := &export.ExportedProject{
		Version: "1.0",
		Project: &export.ExportProjectInfo{
			OriginalID: "proj-1",
			Name:       "Scene Board Test",
		},
		Scenes: []export.ExportedScene{
			{
				RefID: "scene-1",
				Name:  "Scene One",
			},
			{
				RefID: "scene-2",
				Name:  "Scene Two",
			},
		},
		SceneBoards: []export.ExportedSceneBoard{
			{
				RefID:           "board-1",
				Name:            "Main Board",
				Description:     &description,
				DefaultFadeTime: 2.5,
				GridSize:        &gridSize,
				CanvasWidth:     2000,
				CanvasHeight:    2000,
				Buttons: []export.ExportedSceneBoardButton{
					{
						SceneRefID: "scene-1",
						LayoutX:    100,
						LayoutY:    200,
						Width:      intPtr(150),
						Height:     intPtr(100),
						Color:      strPtr("#FF0000"),
						Label:      strPtr("Scene 1"),
					},
					{
						SceneRefID: "scene-2",
						LayoutX:    300,
						LayoutY:    200,
						Width:      intPtr(150),
						Height:     intPtr(100),
						Color:      strPtr("#00FF00"),
						Label:      strPtr("Scene 2"),
					},
				},
			},
		},
	}

	jsonStr, err := exported.ToJSON()
	if err != nil {
		t.Fatalf("Failed to create JSON: %v", err)
	}

	projectID, stats, _, err := service.ImportProject(ctx, jsonStr, ImportOptions{
		Mode: ImportModeCreate,
	})
	if err != nil {
		t.Fatalf("ImportProject failed: %v", err)
	}

	// Verify stats
	if stats.SceneBoardsCreated != 1 {
		t.Errorf("Expected 1 scene board created, got %d", stats.SceneBoardsCreated)
	}

	// Verify scene boards were created
	boards, err := sceneBoardRepo.FindByProjectID(ctx, projectID)
	if err != nil {
		t.Fatalf("Failed to find scene boards: %v", err)
	}
	if len(boards) != 1 {
		t.Fatalf("Expected 1 scene board, got %d", len(boards))
	}

	board := boards[0]
	if board.Name != "Main Board" {
		t.Errorf("Expected board name 'Main Board', got '%s'", board.Name)
	}
	if board.Description == nil || *board.Description != description {
		t.Errorf("Expected description '%s', got %v", description, board.Description)
	}
	if board.DefaultFadeTime != 2.5 {
		t.Errorf("Expected defaultFadeTime 2.5, got %f", board.DefaultFadeTime)
	}
	if board.GridSize == nil || *board.GridSize != 8 {
		t.Errorf("Expected gridSize 8, got %v", board.GridSize)
	}
	if board.CanvasWidth != 2000 {
		t.Errorf("Expected canvasWidth 2000, got %d", board.CanvasWidth)
	}
	if board.CanvasHeight != 2000 {
		t.Errorf("Expected canvasHeight 2000, got %d", board.CanvasHeight)
	}

	// Verify buttons were created
	buttons, err := sceneBoardRepo.GetButtons(ctx, board.ID)
	if err != nil {
		t.Fatalf("Failed to get buttons: %v", err)
	}
	if len(buttons) != 2 {
		t.Fatalf("Expected 2 buttons, got %d", len(buttons))
	}

	// Check first button
	btn1 := buttons[0]
	if btn1.LayoutX != 100 || btn1.LayoutY != 200 {
		t.Errorf("Button 1 position: expected (100, 200), got (%d, %d)", btn1.LayoutX, btn1.LayoutY)
	}
	if btn1.Width == nil || *btn1.Width != 150 {
		t.Errorf("Button 1 width: expected 150, got %v", btn1.Width)
	}
	if btn1.Height == nil || *btn1.Height != 100 {
		t.Errorf("Button 1 height: expected 100, got %v", btn1.Height)
	}
	if btn1.Color == nil || *btn1.Color != "#FF0000" {
		t.Errorf("Button 1 color: expected #FF0000, got %v", btn1.Color)
	}
}

func TestImportProject_Integration_WithLayoutFields(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	projectRepo := repositories.NewProjectRepository(db)
	fixtureRepo := repositories.NewFixtureRepository(db)
	sceneRepo := repositories.NewSceneRepository(db)
	cueListRepo := repositories.NewCueListRepository(db)
	cueRepo := repositories.NewCueRepository(db)

	service := NewService(projectRepo, fixtureRepo, sceneRepo, cueListRepo, cueRepo)
	ctx := context.Background()

	layoutX := 0.25
	layoutY := 0.75
	layoutRotation := 45.0
	projectOrder := 2
	exported := &export.ExportedProject{
		Version: "1.0",
		Project: &export.ExportProjectInfo{
			OriginalID: "proj-1",
			Name:       "Layout Fields Test",
		},
		FixtureDefinitions: []export.ExportedFixtureDefinition{
			{
				RefID:        "def-1",
				Manufacturer: "TestMfg",
				Model:        "TestModel",
				Type:         "LED_PAR",
				Channels: []export.ExportedChannelDefinition{
					{RefID: "ch-d", Name: "Dimmer", Type: "INTENSITY", Offset: 0},
				},
			},
		},
		FixtureInstances: []export.ExportedFixtureInstance{
			{
				RefID:           "inst-1",
				Name:            "Fixture With Layout",
				DefinitionRefID: "def-1",
				Universe:        1,
				StartChannel:    1,
				LayoutX:         &layoutX,
				LayoutY:         &layoutY,
				LayoutRotation:  &layoutRotation,
				ProjectOrder:    &projectOrder,
			},
		},
	}

	jsonStr, err := exported.ToJSON()
	if err != nil {
		t.Fatalf("Failed to create JSON: %v", err)
	}

	projectID, stats, _, err := service.ImportProject(ctx, jsonStr, ImportOptions{
		Mode: ImportModeCreate,
	})
	if err != nil {
		t.Fatalf("ImportProject failed: %v", err)
	}

	if stats.FixtureInstancesCreated != 1 {
		t.Errorf("Expected 1 fixture instance, got %d", stats.FixtureInstancesCreated)
	}

	// Verify fixture was created with layout fields
	fixtures, _ := fixtureRepo.FindByProjectID(ctx, projectID)
	if len(fixtures) != 1 {
		t.Fatalf("Expected 1 fixture, got %d", len(fixtures))
	}

	f := fixtures[0]
	if f.LayoutX == nil || *f.LayoutX != 0.25 {
		t.Errorf("Expected LayoutX 0.25, got %v", f.LayoutX)
	}
	if f.LayoutY == nil || *f.LayoutY != 0.75 {
		t.Errorf("Expected LayoutY 0.75, got %v", f.LayoutY)
	}
	if f.LayoutRotation == nil || *f.LayoutRotation != 45.0 {
		t.Errorf("Expected LayoutRotation 45.0, got %v", f.LayoutRotation)
	}
	if f.ProjectOrder == nil || *f.ProjectOrder != 2 {
		t.Errorf("Expected ProjectOrder 2, got %v", f.ProjectOrder)
	}
}

func TestImportProject_Integration_WithFadeBehavior(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	projectRepo := repositories.NewProjectRepository(db)
	fixtureRepo := repositories.NewFixtureRepository(db)
	sceneRepo := repositories.NewSceneRepository(db)
	cueListRepo := repositories.NewCueListRepository(db)
	cueRepo := repositories.NewCueRepository(db)

	service := NewService(projectRepo, fixtureRepo, sceneRepo, cueListRepo, cueRepo)
	ctx := context.Background()

	exported := &export.ExportedProject{
		Version: "1.0",
		Project: &export.ExportProjectInfo{
			OriginalID: "proj-1",
			Name:       "Fade Behavior Test",
		},
		FixtureDefinitions: []export.ExportedFixtureDefinition{
			{
				RefID:        "def-1",
				Manufacturer: "TestMfg",
				Model:        "TestModel",
				Type:         "LED_PAR",
				Channels: []export.ExportedChannelDefinition{
					{RefID: "ch-d", Name: "Dimmer", Type: "INTENSITY", Offset: 0, FadeBehavior: "FADE", IsDiscrete: false},
					{RefID: "ch-g", Name: "Gobo", Type: "GOBO", Offset: 1, FadeBehavior: "SNAP", IsDiscrete: true},
					{RefID: "ch-s", Name: "Strobe", Type: "STROBE", Offset: 2, FadeBehavior: "SNAP_END", IsDiscrete: false},
				},
			},
		},
		FixtureInstances: []export.ExportedFixtureInstance{
			{
				RefID:           "inst-1",
				Name:            "Fixture With Fade Behavior",
				DefinitionRefID: "def-1",
				Universe:        1,
				StartChannel:    1,
				InstanceChannels: []export.ExportedInstanceChannel{
					{Name: "Dimmer", Type: "INTENSITY", Offset: 0, FadeBehavior: "FADE", IsDiscrete: false},
					{Name: "Gobo", Type: "GOBO", Offset: 1, FadeBehavior: "SNAP", IsDiscrete: true},
					{Name: "Strobe", Type: "STROBE", Offset: 2, FadeBehavior: "SNAP_END", IsDiscrete: false},
				},
			},
		},
	}

	jsonStr, err := exported.ToJSON()
	if err != nil {
		t.Fatalf("Failed to create JSON: %v", err)
	}

	projectID, stats, _, err := service.ImportProject(ctx, jsonStr, ImportOptions{
		Mode: ImportModeCreate,
	})
	if err != nil {
		t.Fatalf("ImportProject failed: %v", err)
	}

	if stats.FixtureDefinitionsCreated != 1 {
		t.Errorf("Expected 1 fixture definition, got %d", stats.FixtureDefinitionsCreated)
	}

	// Verify definition channels have fade behavior
	def, _ := fixtureRepo.FindDefinitionByManufacturerModel(ctx, "TestMfg", "TestModel")
	if def == nil {
		t.Fatal("Expected definition to be created")
	}

	channels, _ := fixtureRepo.GetDefinitionChannels(ctx, def.ID)
	if len(channels) != 3 {
		t.Fatalf("Expected 3 channels, got %d", len(channels))
	}

	// Check dimmer channel (FADE)
	dimmer := findChannelByName(channels, "Dimmer")
	if dimmer == nil {
		t.Fatal("Expected Dimmer channel")
	}
	if dimmer.FadeBehavior != "FADE" {
		t.Errorf("Expected Dimmer FadeBehavior 'FADE', got '%s'", dimmer.FadeBehavior)
	}
	if dimmer.IsDiscrete {
		t.Error("Expected Dimmer IsDiscrete to be false")
	}

	// Check gobo channel (SNAP)
	gobo := findChannelByName(channels, "Gobo")
	if gobo == nil {
		t.Fatal("Expected Gobo channel")
	}
	if gobo.FadeBehavior != "SNAP" {
		t.Errorf("Expected Gobo FadeBehavior 'SNAP', got '%s'", gobo.FadeBehavior)
	}
	if !gobo.IsDiscrete {
		t.Error("Expected Gobo IsDiscrete to be true")
	}

	// Check strobe channel (SNAP_END)
	strobe := findChannelByName(channels, "Strobe")
	if strobe == nil {
		t.Fatal("Expected Strobe channel")
	}
	if strobe.FadeBehavior != "SNAP_END" {
		t.Errorf("Expected Strobe FadeBehavior 'SNAP_END', got '%s'", strobe.FadeBehavior)
	}

	// Verify instance channels have fade behavior
	fixtures, _ := fixtureRepo.FindByProjectID(ctx, projectID)
	if len(fixtures) != 1 {
		t.Fatalf("Expected 1 fixture, got %d", len(fixtures))
	}

	instanceChannels, _ := fixtureRepo.GetInstanceChannels(ctx, fixtures[0].ID)
	if len(instanceChannels) != 3 {
		t.Fatalf("Expected 3 instance channels, got %d", len(instanceChannels))
	}

	// Check instance dimmer
	instDimmer := findInstanceChannelByName(instanceChannels, "Dimmer")
	if instDimmer == nil {
		t.Fatal("Expected instance Dimmer channel")
	}
	if instDimmer.FadeBehavior != "FADE" {
		t.Errorf("Expected instance Dimmer FadeBehavior 'FADE', got '%s'", instDimmer.FadeBehavior)
	}

	// Check instance gobo
	instGobo := findInstanceChannelByName(instanceChannels, "Gobo")
	if instGobo == nil {
		t.Fatal("Expected instance Gobo channel")
	}
	if instGobo.FadeBehavior != "SNAP" {
		t.Errorf("Expected instance Gobo FadeBehavior 'SNAP', got '%s'", instGobo.FadeBehavior)
	}
}

func findChannelByName(channels []models.ChannelDefinition, name string) *models.ChannelDefinition {
	for i := range channels {
		if channels[i].Name == name {
			return &channels[i]
		}
	}
	return nil
}

func findInstanceChannelByName(channels []models.InstanceChannel, name string) *models.InstanceChannel {
	for i := range channels {
		if channels[i].Name == name {
			return &channels[i]
		}
	}
	return nil
}

// Helper functions for creating pointers
func intPtr(i int) *int {
	return &i
}

func strPtr(s string) *string {
	return &s
}

func TestImportProject_Integration_FadeBehaviorDefaultsToFade(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	projectRepo := repositories.NewProjectRepository(db)
	fixtureRepo := repositories.NewFixtureRepository(db)
	sceneRepo := repositories.NewSceneRepository(db)
	cueListRepo := repositories.NewCueListRepository(db)
	cueRepo := repositories.NewCueRepository(db)

	service := NewService(projectRepo, fixtureRepo, sceneRepo, cueListRepo, cueRepo)
	ctx := context.Background()

	// Import definition without fade behavior specified (legacy data)
	exported := &export.ExportedProject{
		Version: "1.0",
		Project: &export.ExportProjectInfo{
			OriginalID: "proj-1",
			Name:       "Legacy No Fade Behavior Test",
		},
		FixtureDefinitions: []export.ExportedFixtureDefinition{
			{
				RefID:        "def-1",
				Manufacturer: "TestMfg",
				Model:        "TestModel",
				Type:         "LED_PAR",
				Channels: []export.ExportedChannelDefinition{
					{RefID: "ch-d", Name: "Dimmer", Type: "INTENSITY", Offset: 0},
					// No FadeBehavior or IsDiscrete specified
				},
			},
		},
	}

	jsonStr, err := exported.ToJSON()
	if err != nil {
		t.Fatalf("Failed to create JSON: %v", err)
	}

	_, _, _, err = service.ImportProject(ctx, jsonStr, ImportOptions{
		Mode: ImportModeCreate,
	})
	if err != nil {
		t.Fatalf("ImportProject failed: %v", err)
	}

	// Verify definition channels default to FADE
	def, _ := fixtureRepo.FindDefinitionByManufacturerModel(ctx, "TestMfg", "TestModel")
	if def == nil {
		t.Fatal("Expected definition to be created")
	}

	channels, _ := fixtureRepo.GetDefinitionChannels(ctx, def.ID)
	if len(channels) != 1 {
		t.Fatalf("Expected 1 channel, got %d", len(channels))
	}

	// Should default to FADE
	if channels[0].FadeBehavior != "FADE" {
		t.Errorf("Expected default FadeBehavior 'FADE', got '%s'", channels[0].FadeBehavior)
	}
	if channels[0].IsDiscrete {
		t.Error("Expected default IsDiscrete to be false")
	}
}

func TestImportProject_Integration_SceneBoardsWithoutRepo(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	projectRepo := repositories.NewProjectRepository(db)
	fixtureRepo := repositories.NewFixtureRepository(db)
	sceneRepo := repositories.NewSceneRepository(db)
	cueListRepo := repositories.NewCueListRepository(db)
	cueRepo := repositories.NewCueRepository(db)

	// Use service WITHOUT scene board repo
	service := NewService(projectRepo, fixtureRepo, sceneRepo, cueListRepo, cueRepo)
	ctx := context.Background()

	// Import data with scene boards, but service doesn't have scene board repo
	exported := &export.ExportedProject{
		Version: "1.0",
		Project: &export.ExportProjectInfo{
			OriginalID: "proj-1",
			Name:       "Scene Board Test Without Repo",
		},
		SceneBoards: []export.ExportedSceneBoard{
			{
				RefID:           "board-1",
				Name:            "Test Board",
				DefaultFadeTime: 2.0,
				CanvasWidth:     2000,
				CanvasHeight:    2000,
			},
		},
	}

	jsonStr, err := exported.ToJSON()
	if err != nil {
		t.Fatalf("Failed to create JSON: %v", err)
	}

	_, stats, _, err := service.ImportProject(ctx, jsonStr, ImportOptions{
		Mode: ImportModeCreate,
	})
	if err != nil {
		t.Fatalf("ImportProject failed: %v", err)
	}

	// Should not have created any scene boards since repo is nil
	if stats.SceneBoardsCreated != 0 {
		t.Errorf("Expected 0 scene boards created (no repo), got %d", stats.SceneBoardsCreated)
	}
}

func TestImportProject_Integration_SceneBoardButtonWithUnknownScene(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	projectRepo := repositories.NewProjectRepository(db)
	fixtureRepo := repositories.NewFixtureRepository(db)
	sceneRepo := repositories.NewSceneRepository(db)
	cueListRepo := repositories.NewCueListRepository(db)
	cueRepo := repositories.NewCueRepository(db)
	sceneBoardRepo := repositories.NewSceneBoardRepository(db)

	service := NewServiceWithSceneBoards(projectRepo, fixtureRepo, sceneRepo, cueListRepo, cueRepo, sceneBoardRepo)
	ctx := context.Background()

	// Import scene board with button referencing unknown scene
	exported := &export.ExportedProject{
		Version: "1.0",
		Project: &export.ExportProjectInfo{
			OriginalID: "proj-1",
			Name:       "Unknown Scene Button Test",
		},
		Scenes: []export.ExportedScene{
			{
				RefID: "scene-1",
				Name:  "Known Scene",
			},
		},
		SceneBoards: []export.ExportedSceneBoard{
			{
				RefID:           "board-1",
				Name:            "Test Board",
				DefaultFadeTime: 2.0,
				CanvasWidth:     2000,
				CanvasHeight:    2000,
				Buttons: []export.ExportedSceneBoardButton{
					{
						SceneRefID: "scene-1", // Known scene
						LayoutX:    100,
						LayoutY:    100,
						Width:      intPtr(100),
						Height:     intPtr(100),
						Color:      strPtr("#FF0000"),
					},
					{
						SceneRefID: "unknown-scene", // Unknown scene
						LayoutX:    200,
						LayoutY:    100,
						Width:      intPtr(100),
						Height:     intPtr(100),
						Color:      strPtr("#00FF00"),
					},
				},
			},
		},
	}

	jsonStr, err := exported.ToJSON()
	if err != nil {
		t.Fatalf("Failed to create JSON: %v", err)
	}

	projectID, stats, _, err := service.ImportProject(ctx, jsonStr, ImportOptions{
		Mode: ImportModeCreate,
	})
	if err != nil {
		t.Fatalf("ImportProject failed: %v", err)
	}

	if stats.SceneBoardsCreated != 1 {
		t.Errorf("Expected 1 scene board created, got %d", stats.SceneBoardsCreated)
	}

	// Board should be created with only the button referencing known scene
	boards, _ := sceneBoardRepo.FindByProjectID(ctx, projectID)
	if len(boards) != 1 {
		t.Fatalf("Expected 1 board, got %d", len(boards))
	}

	buttons, _ := sceneBoardRepo.GetButtons(ctx, boards[0].ID)
	// Only the button with known scene should be created
	if len(buttons) != 1 {
		t.Errorf("Expected 1 button (unknown scene button skipped), got %d", len(buttons))
	}
}

func TestImportProject_ModeRefID_Integration(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	ctx := context.Background()
	projectRepo := repositories.NewProjectRepository(db)
	fixtureRepo := repositories.NewFixtureRepository(db)
	sceneRepo := repositories.NewSceneRepository(db)
	cueListRepo := repositories.NewCueListRepository(db)
	cueRepo := repositories.NewCueRepository(db)

	service := NewService(projectRepo, fixtureRepo, sceneRepo, cueListRepo, cueRepo)

	// Create export data with fixtures using different modes
	// This simulates an exported project where fixtures reference modes by refId
	exported := &export.ExportedProject{
		Version: "1.0",
		Project: &export.ExportProjectInfo{
			OriginalID: "test-project",
			Name:       "Mode RefID Import Test",
		},
		FixtureDefinitions: []export.ExportedFixtureDefinition{
			{
				RefID:        "def-1",
				Manufacturer: "Chauvet DJ",
				Model:        "SlimPAR Pro RGBA",
				Type:         "LED_PAR",
				IsBuiltIn:    false,
				Modes: []export.ExportedFixtureMode{
					{
						RefID:        "mode-10ch",
						Name:         "10-channel",
						ChannelCount: 10,
						ModeChannels: []export.ExportedModeChannel{
							{ChannelRefID: "ch-dimmer", Offset: 0},
							{ChannelRefID: "ch-red", Offset: 1},
							{ChannelRefID: "ch-green", Offset: 2},
							{ChannelRefID: "ch-blue", Offset: 3},
							{ChannelRefID: "ch-amber", Offset: 4},
							{ChannelRefID: "ch-strobe", Offset: 5},
							{ChannelRefID: "ch-speed", Offset: 6},
							{ChannelRefID: "ch-program", Offset: 7},
							{ChannelRefID: "ch-dimmer-curve", Offset: 8},
							{ChannelRefID: "ch-auto", Offset: 9},
						},
					},
					{
						RefID:        "mode-4ch",
						Name:         "4-channel",
						ChannelCount: 4,
						ModeChannels: []export.ExportedModeChannel{
							{ChannelRefID: "ch-red", Offset: 0},
							{ChannelRefID: "ch-green", Offset: 1},
							{ChannelRefID: "ch-blue", Offset: 2},
							{ChannelRefID: "ch-amber", Offset: 3},
						},
					},
					{
						RefID:        "mode-5ch",
						Name:         "5-channel",
						ChannelCount: 5,
						ModeChannels: []export.ExportedModeChannel{
							{ChannelRefID: "ch-dimmer", Offset: 0},
							{ChannelRefID: "ch-red", Offset: 1},
							{ChannelRefID: "ch-green", Offset: 2},
							{ChannelRefID: "ch-blue", Offset: 3},
							{ChannelRefID: "ch-amber", Offset: 4},
						},
					},
				},
				Channels: []export.ExportedChannelDefinition{
					{RefID: "ch-dimmer", Name: "Dimmer", Type: "INTENSITY", Offset: 0, MinValue: 0, MaxValue: 255, DefaultValue: 0},
					{RefID: "ch-red", Name: "Red", Type: "RED", Offset: 1, MinValue: 0, MaxValue: 255, DefaultValue: 0},
					{RefID: "ch-green", Name: "Green", Type: "GREEN", Offset: 2, MinValue: 0, MaxValue: 255, DefaultValue: 0},
					{RefID: "ch-blue", Name: "Blue", Type: "BLUE", Offset: 3, MinValue: 0, MaxValue: 255, DefaultValue: 0},
					{RefID: "ch-amber", Name: "Amber", Type: "AMBER", Offset: 4, MinValue: 0, MaxValue: 255, DefaultValue: 0},
					{RefID: "ch-strobe", Name: "Strobe", Type: "STROBE", Offset: 5, MinValue: 0, MaxValue: 255, DefaultValue: 0},
					{RefID: "ch-speed", Name: "Speed", Type: "SPEED", Offset: 6, MinValue: 0, MaxValue: 255, DefaultValue: 128},
					{RefID: "ch-program", Name: "Program", Type: "EFFECT", Offset: 7, MinValue: 0, MaxValue: 255, DefaultValue: 0},
					{RefID: "ch-dimmer-curve", Name: "Dimmer Curve", Type: "MAINTENANCE", Offset: 8, MinValue: 0, MaxValue: 255, DefaultValue: 0},
					{RefID: "ch-auto", Name: "Auto", Type: "EFFECT", Offset: 9, MinValue: 0, MaxValue: 255, DefaultValue: 0},
				},
			},
		},
		FixtureInstances: []export.ExportedFixtureInstance{
			{
				RefID:           "fixture-1",
				OriginalID:      "fixture-1",
				Name:            "PAR 1",
				DefinitionRefID: "def-1",
				ModeName:        strPtr("4-channel"),
				ModeRefID:       strPtr("mode-4ch"), // This should be used to set the correct mode
				ChannelCount:    intPtr(4),
				Universe:        1,
				StartChannel:    1,
			},
			{
				RefID:           "fixture-2",
				OriginalID:      "fixture-2",
				Name:            "PAR 2",
				DefinitionRefID: "def-1",
				ModeName:        strPtr("10-channel"),
				ModeRefID:       strPtr("mode-10ch"), // This should be used to set the correct mode
				ChannelCount:    intPtr(10),
				Universe:        1,
				StartChannel:    5,
			},
			{
				RefID:           "fixture-3",
				OriginalID:      "fixture-3",
				Name:            "PAR 3",
				DefinitionRefID: "def-1",
				ModeName:        strPtr("5-channel"),
				ModeRefID:       strPtr("mode-5ch"), // This should be used to set the correct mode
				ChannelCount:    intPtr(5),
				Universe:        1,
				StartChannel:    15,
			},
		},
	}

	// Convert to JSON
	jsonContent, err := exported.ToJSON()
	if err != nil {
		t.Fatalf("Failed to convert export to JSON: %v", err)
	}

	// Import the project
	projectID, stats, warnings, err := service.ImportProject(ctx, jsonContent, ImportOptions{
		Mode:                    ImportModeCreate,
		ImportBuiltInFixtures:   false,
		FixtureConflictStrategy: FixtureConflictSkip,
	})

	if err != nil {
		t.Fatalf("ImportProject failed: %v", err)
	}

	if len(warnings) > 0 {
		t.Logf("Import warnings: %v", warnings)
	}

	// Verify import stats
	if stats.FixtureDefinitionsCreated != 1 {
		t.Errorf("Expected 1 fixture definition created, got %d", stats.FixtureDefinitionsCreated)
	}
	if stats.FixtureInstancesCreated != 3 {
		t.Errorf("Expected 3 fixture instances created, got %d", stats.FixtureInstancesCreated)
	}

	// Get all fixtures from the imported project
	fixtures, err := fixtureRepo.FindByProjectID(ctx, projectID)
	if err != nil {
		t.Fatalf("Failed to get fixtures: %v", err)
	}

	if len(fixtures) != 3 {
		t.Fatalf("Expected 3 fixtures, got %d", len(fixtures))
	}

	// Build a map by name for easier verification
	fixturesByName := make(map[string]*models.FixtureInstance)
	for i, f := range fixtures {
		fixturesByName[f.Name] = &fixtures[i]
	}

	// Verify PAR 1 (should have 4-channel mode)
	par1 := fixturesByName["PAR 1"]
	if par1 == nil {
		t.Fatal("PAR 1 not found")
	}
	if par1.ModeName == nil || *par1.ModeName != "4-channel" {
		t.Errorf("PAR 1: Expected ModeName '4-channel', got %v", par1.ModeName)
	}
	if par1.ChannelCount == nil || *par1.ChannelCount != 4 {
		t.Errorf("PAR 1: Expected ChannelCount 4, got %v", par1.ChannelCount)
	}

	// Verify PAR 1 instance channels
	par1Channels, err := fixtureRepo.GetInstanceChannels(ctx, par1.ID)
	if err != nil {
		t.Fatalf("Failed to get PAR 1 instance channels: %v", err)
	}
	if len(par1Channels) != 4 {
		t.Errorf("PAR 1: Expected 4 instance channels, got %d", len(par1Channels))
	}
	// Verify the 4-channel mode uses Red, Green, Blue, Amber (no Dimmer)
	expectedPar1Types := []string{"RED", "GREEN", "BLUE", "AMBER"}
	for i, ch := range par1Channels {
		if ch.Type != expectedPar1Types[i] {
			t.Errorf("PAR 1 channel %d: Expected type %s, got %s", i, expectedPar1Types[i], ch.Type)
		}
		if ch.Offset != i {
			t.Errorf("PAR 1 channel %d: Expected offset %d, got %d", i, i, ch.Offset)
		}
	}

	// Verify PAR 2 (should have 10-channel mode)
	par2 := fixturesByName["PAR 2"]
	if par2 == nil {
		t.Fatal("PAR 2 not found")
	}
	if par2.ModeName == nil || *par2.ModeName != "10-channel" {
		t.Errorf("PAR 2: Expected ModeName '10-channel', got %v", par2.ModeName)
	}
	if par2.ChannelCount == nil || *par2.ChannelCount != 10 {
		t.Errorf("PAR 2: Expected ChannelCount 10, got %v", par2.ChannelCount)
	}

	// Verify PAR 2 instance channels
	par2Channels, err := fixtureRepo.GetInstanceChannels(ctx, par2.ID)
	if err != nil {
		t.Fatalf("Failed to get PAR 2 instance channels: %v", err)
	}
	if len(par2Channels) != 10 {
		t.Errorf("PAR 2: Expected 10 instance channels, got %d", len(par2Channels))
	}
	// Verify the 10-channel mode uses all channels in order
	expectedPar2Types := []string{"INTENSITY", "RED", "GREEN", "BLUE", "AMBER", "STROBE", "SPEED", "EFFECT", "MAINTENANCE", "EFFECT"}
	for i, ch := range par2Channels {
		if ch.Type != expectedPar2Types[i] {
			t.Errorf("PAR 2 channel %d: Expected type %s, got %s", i, expectedPar2Types[i], ch.Type)
		}
		if ch.Offset != i {
			t.Errorf("PAR 2 channel %d: Expected offset %d, got %d", i, i, ch.Offset)
		}
	}

	// Verify PAR 3 (should have 5-channel mode)
	par3 := fixturesByName["PAR 3"]
	if par3 == nil {
		t.Fatal("PAR 3 not found")
	}
	if par3.ModeName == nil || *par3.ModeName != "5-channel" {
		t.Errorf("PAR 3: Expected ModeName '5-channel', got %v", par3.ModeName)
	}
	if par3.ChannelCount == nil || *par3.ChannelCount != 5 {
		t.Errorf("PAR 3: Expected ChannelCount 5, got %v", par3.ChannelCount)
	}

	// Verify PAR 3 instance channels
	par3Channels, err := fixtureRepo.GetInstanceChannels(ctx, par3.ID)
	if err != nil {
		t.Fatalf("Failed to get PAR 3 instance channels: %v", err)
	}
	if len(par3Channels) != 5 {
		t.Errorf("PAR 3: Expected 5 instance channels, got %d", len(par3Channels))
	}
	// Verify the 5-channel mode uses Dimmer, Red, Green, Blue, Amber
	expectedPar3Types := []string{"INTENSITY", "RED", "GREEN", "BLUE", "AMBER"}
	for i, ch := range par3Channels {
		if ch.Type != expectedPar3Types[i] {
			t.Errorf("PAR 3 channel %d: Expected type %s, got %s", i, expectedPar3Types[i], ch.Type)
		}
		if ch.Offset != i {
			t.Errorf("PAR 3 channel %d: Expected offset %d, got %d", i, i, ch.Offset)
		}
	}

	// Verify that modes were created correctly
	def := fixturesByName["PAR 1"]
	if def == nil {
		t.Fatal("Fixture definition not found")
	}

	modes, err := fixtureRepo.GetDefinitionModes(ctx, def.DefinitionID)
	if err != nil {
		t.Fatalf("Failed to get modes: %v", err)
	}

	if len(modes) != 3 {
		t.Errorf("Expected 3 modes, got %d", len(modes))
	}

	// Verify that each mode has the correct name
	modeNames := make(map[string]bool)
	for _, mode := range modes {
		modeNames[mode.Name] = true
	}

	if !modeNames["4-channel"] {
		t.Error("Expected mode '4-channel' to exist")
	}
	if !modeNames["5-channel"] {
		t.Error("Expected mode '5-channel' to exist")
	}
	if !modeNames["10-channel"] {
		t.Error("Expected mode '10-channel' to exist")
	}
}

func TestImportProject_ModeRefID_ExistingDefinition_Integration(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	ctx := context.Background()
	projectRepo := repositories.NewProjectRepository(db)
	fixtureRepo := repositories.NewFixtureRepository(db)
	sceneRepo := repositories.NewSceneRepository(db)
	cueListRepo := repositories.NewCueListRepository(db)
	cueRepo := repositories.NewCueRepository(db)

	service := NewService(projectRepo, fixtureRepo, sceneRepo, cueListRepo, cueRepo)

	// Create an existing fixture definition with one mode
	def := &models.FixtureDefinition{
		Manufacturer: "Chauvet DJ",
		Model:        "SlimPAR Pro RGBA",
		Type:         "LED_PAR",
		IsBuiltIn:    false,
	}
	_ = fixtureRepo.CreateDefinition(ctx, def)

	// Create channels (include Dimmer for 5-channel mode)
	ch0 := &models.ChannelDefinition{ID: cuid.New(), Name: "Dimmer", Type: "INTENSITY", Offset: 0, DefinitionID: def.ID}
	ch1 := &models.ChannelDefinition{ID: cuid.New(), Name: "Red", Type: "RED", Offset: 1, DefinitionID: def.ID}
	ch2 := &models.ChannelDefinition{ID: cuid.New(), Name: "Green", Type: "GREEN", Offset: 2, DefinitionID: def.ID}
	ch3 := &models.ChannelDefinition{ID: cuid.New(), Name: "Blue", Type: "BLUE", Offset: 3, DefinitionID: def.ID}
	ch4 := &models.ChannelDefinition{ID: cuid.New(), Name: "Amber", Type: "AMBER", Offset: 4, DefinitionID: def.ID}
	_ = fixtureRepo.CreateChannelDefinition(ctx, ch0)
	_ = fixtureRepo.CreateChannelDefinition(ctx, ch1)
	_ = fixtureRepo.CreateChannelDefinition(ctx, ch2)
	_ = fixtureRepo.CreateChannelDefinition(ctx, ch3)
	_ = fixtureRepo.CreateChannelDefinition(ctx, ch4)

	// Create an existing mode (4-channel) - no Dimmer, just colors
	existingMode := &models.FixtureMode{
		Name:         "4-channel",
		ChannelCount: 4,
		DefinitionID: def.ID,
	}
	_ = fixtureRepo.CreateMode(ctx, existingMode)

	// Create mode channels for the existing 4-channel mode
	mc1 := &models.ModeChannel{ID: cuid.New(), ModeID: existingMode.ID, ChannelID: ch1.ID, Offset: 0}
	mc2 := &models.ModeChannel{ID: cuid.New(), ModeID: existingMode.ID, ChannelID: ch2.ID, Offset: 1}
	mc3 := &models.ModeChannel{ID: cuid.New(), ModeID: existingMode.ID, ChannelID: ch3.ID, Offset: 2}
	mc4 := &models.ModeChannel{ID: cuid.New(), ModeID: existingMode.ID, ChannelID: ch4.ID, Offset: 3}
	db.Create(mc1)
	db.Create(mc2)
	db.Create(mc3)
	db.Create(mc4)

	// Import a project that has the same definition with additional modes
	exported := &export.ExportedProject{
		Version: "1.0",
		Project: &export.ExportProjectInfo{
			OriginalID: "test-project",
			Name:       "Mode RefID Existing Definition Test",
		},
		FixtureDefinitions: []export.ExportedFixtureDefinition{
			{
				RefID:        "def-1",
				Manufacturer: "Chauvet DJ",
				Model:        "SlimPAR Pro RGBA",
				Type:         "LED_PAR",
				IsBuiltIn:    false,
				Modes: []export.ExportedFixtureMode{
					{
						RefID:        "mode-4ch",
						Name:         "4-channel",
						ChannelCount: 4,
						ModeChannels: []export.ExportedModeChannel{
							{ChannelRefID: "ch-red", Offset: 0},
							{ChannelRefID: "ch-green", Offset: 1},
							{ChannelRefID: "ch-blue", Offset: 2},
							{ChannelRefID: "ch-amber", Offset: 3},
						},
					},
					{
						RefID:        "mode-5ch",
						Name:         "5-channel",
						ChannelCount: 5,
						ModeChannels: []export.ExportedModeChannel{
							{ChannelRefID: "ch-dimmer", Offset: 0},
							{ChannelRefID: "ch-red", Offset: 1},
							{ChannelRefID: "ch-green", Offset: 2},
							{ChannelRefID: "ch-blue", Offset: 3},
							{ChannelRefID: "ch-amber", Offset: 4},
						},
					},
				},
				Channels: []export.ExportedChannelDefinition{
					{RefID: "ch-dimmer", Name: "Dimmer", Type: "INTENSITY", Offset: 0, MinValue: 0, MaxValue: 255, DefaultValue: 0},
					{RefID: "ch-red", Name: "Red", Type: "RED", Offset: 1, MinValue: 0, MaxValue: 255, DefaultValue: 0},
					{RefID: "ch-green", Name: "Green", Type: "GREEN", Offset: 2, MinValue: 0, MaxValue: 255, DefaultValue: 0},
					{RefID: "ch-blue", Name: "Blue", Type: "BLUE", Offset: 3, MinValue: 0, MaxValue: 255, DefaultValue: 0},
					{RefID: "ch-amber", Name: "Amber", Type: "AMBER", Offset: 4, MinValue: 0, MaxValue: 255, DefaultValue: 0},
				},
			},
		},
		FixtureInstances: []export.ExportedFixtureInstance{
			{
				RefID:           "fixture-1",
				OriginalID:      "fixture-1",
				Name:            "PAR 1",
				DefinitionRefID: "def-1",
				ModeName:        strPtr("4-channel"),
				ModeRefID:       strPtr("mode-4ch"), // Should map to existing mode
				Universe:        1,
				StartChannel:    1,
			},
			{
				RefID:           "fixture-2",
				OriginalID:      "fixture-2",
				Name:            "PAR 2",
				DefinitionRefID: "def-1",
				ModeName:        strPtr("5-channel"),
				ModeRefID:       strPtr("mode-5ch"), // Should map to newly created mode
				Universe:        1,
				StartChannel:    5,
			},
		},
	}

	jsonContent, err := exported.ToJSON()
	if err != nil {
		t.Fatalf("Failed to convert export to JSON: %v", err)
	}

	// Import the project
	projectID, stats, warnings, err := service.ImportProject(ctx, jsonContent, ImportOptions{
		Mode:                    ImportModeCreate,
		ImportBuiltInFixtures:   false,
		FixtureConflictStrategy: FixtureConflictSkip,
	})

	if err != nil {
		t.Fatalf("ImportProject failed: %v", err)
	}

	if len(warnings) > 0 {
		t.Logf("Import warnings: %v", warnings)
	}

	// Verify that no new definition was created (existing one was reused)
	if stats.FixtureDefinitionsCreated != 0 {
		t.Errorf("Expected 0 fixture definitions created (should reuse existing), got %d", stats.FixtureDefinitionsCreated)
	}

	// Get all fixtures
	fixtures, err := fixtureRepo.FindByProjectID(ctx, projectID)
	if err != nil {
		t.Fatalf("Failed to get fixtures: %v", err)
	}

	if len(fixtures) != 2 {
		t.Fatalf("Expected 2 fixtures, got %d", len(fixtures))
	}

	// Build map by name
	fixturesByName := make(map[string]*models.FixtureInstance)
	for i, f := range fixtures {
		fixturesByName[f.Name] = &fixtures[i]
	}

	// Verify PAR 1 uses existing 4-channel mode
	par1 := fixturesByName["PAR 1"]
	if par1 == nil {
		t.Fatal("PAR 1 not found")
	}
	if par1.ModeName == nil || *par1.ModeName != "4-channel" {
		t.Errorf("PAR 1: Expected ModeName '4-channel', got %v", par1.ModeName)
	}

	// Verify PAR 1 instance channels
	par1Channels, err := fixtureRepo.GetInstanceChannels(ctx, par1.ID)
	if err != nil {
		t.Fatalf("Failed to get PAR 1 instance channels: %v", err)
	}
	if len(par1Channels) != 4 {
		t.Errorf("PAR 1: Expected 4 instance channels, got %d", len(par1Channels))
	}
	// Verify the 4-channel mode uses Red, Green, Blue, Amber (no Dimmer)
	expectedPar1Types := []string{"RED", "GREEN", "BLUE", "AMBER"}
	for i, ch := range par1Channels {
		if ch.Type != expectedPar1Types[i] {
			t.Errorf("PAR 1 channel %d: Expected type %s, got %s", i, expectedPar1Types[i], ch.Type)
		}
	}

	// Verify PAR 2 uses new 10-channel mode
	par2 := fixturesByName["PAR 2"]
	if par2 == nil {
		t.Fatal("PAR 2 not found")
	}
	if par2.ModeName == nil || *par2.ModeName != "5-channel" {
		t.Errorf("PAR 2: Expected ModeName '5-channel', got %v", par2.ModeName)
	}

	// Verify PAR 2 instance channels
	par2Channels, err := fixtureRepo.GetInstanceChannels(ctx, par2.ID)
	if err != nil {
		t.Fatalf("Failed to get PAR 2 instance channels: %v", err)
	}
	if len(par2Channels) != 5 {
		t.Errorf("PAR 2: Expected 5 instance channels, got %d", len(par2Channels))
	}
	// Verify the 5-channel mode uses Dimmer + colors
	expectedPar2Types := []string{"INTENSITY", "RED", "GREEN", "BLUE", "AMBER"}
	for i, ch := range par2Channels {
		if ch.Type != expectedPar2Types[i] {
			t.Errorf("PAR 2 channel %d: Expected type %s, got %s", i, expectedPar2Types[i], ch.Type)
		}
	}

	// Verify that modes were created correctly (should have 2 modes total: existing + new)
	modes, err := fixtureRepo.GetDefinitionModes(ctx, def.ID)
	if err != nil {
		t.Fatalf("Failed to get modes: %v", err)
	}

	if len(modes) != 2 {
		t.Errorf("Expected 2 modes (1 existing + 1 new), got %d", len(modes))
	}

	modeNames := make(map[string]bool)
	for _, mode := range modes {
		modeNames[mode.Name] = true
	}

	if !modeNames["4-channel"] {
		t.Error("Expected mode '4-channel' to exist")
	}
	if !modeNames["5-channel"] {
		t.Error("Expected mode '5-channel' to exist")
	}
}
