package export

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/bbernstein/lacylights-go/internal/database/models"
	"github.com/bbernstein/lacylights-go/internal/database/repositories"
	"github.com/glebarez/sqlite"
	"github.com/lucsky/cuid"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func TestExportedProject_ToJSON(t *testing.T) {
	desc := "Test project description"
	project := &ExportedProject{
		Version: "1.0",
		Project: &ExportProjectInfo{
			OriginalID:  "proj-123",
			Name:        "Test Project",
			Description: &desc,
		},
		FixtureDefinitions: []ExportedFixtureDefinition{
			{
				RefID:        "def-1",
				Manufacturer: "TestMfg",
				Model:        "TestModel",
				Type:         "DIMMER",
				IsBuiltIn:    false,
				Channels: []ExportedChannelDefinition{
					{
						Name:         "Intensity",
						Type:         "INTENSITY",
						Offset:       0,
						MinValue:     0,
						MaxValue:     255,
						DefaultValue: 0,
					},
				},
			},
		},
		FixtureInstances: []ExportedFixtureInstance{
			{
				RefID:           "fix-1",
				Name:            "Fixture 1",
				DefinitionRefID: "def-1",
				Universe:        1,
				StartChannel:    1,
				Tags:            []string{"front", "stage"},
			},
		},
		Scenes: []ExportedScene{
			{
				RefID: "scene-1",
				Name:  "Opening",
				FixtureValues: []ExportedFixtureValue{
					{
						FixtureRefID: "fix-1",
						Channels: []ExportedChannelValue{
							{Offset: 0, Value: 255},
						},
					},
				},
			},
		},
		CueLists: []ExportedCueList{
			{
				RefID: "cuelist-1",
				Name:  "Main",
				Loop:  true,
				Cues: []ExportedCue{
					{
						OriginalID:  "cue-1",
						Name:        "Cue 1",
						CueNumber:   1.0,
						SceneRefID:  "scene-1",
						FadeInTime:  2.0,
						FadeOutTime: 1.0,
					},
				},
			},
		},
	}

	jsonStr, err := project.ToJSON()
	if err != nil {
		t.Fatalf("ToJSON() error: %v", err)
	}

	// Verify it's valid JSON
	var parsed map[string]interface{}
	if err := json.Unmarshal([]byte(jsonStr), &parsed); err != nil {
		t.Fatalf("ToJSON() produced invalid JSON: %v", err)
	}

	// Check key fields are present
	if parsed["version"] != "1.0" {
		t.Errorf("Expected version '1.0', got '%v'", parsed["version"])
	}
	projInfo := parsed["project"].(map[string]interface{})
	if projInfo["originalId"] != "proj-123" {
		t.Errorf("Expected project.originalId 'proj-123', got '%v'", projInfo["originalId"])
	}
	if projInfo["name"] != "Test Project" {
		t.Errorf("Expected project.name 'Test Project', got '%v'", projInfo["name"])
	}
}

func TestExportedProject_ToJSON_Empty(t *testing.T) {
	project := &ExportedProject{
		Version: "1.0",
		Project: &ExportProjectInfo{
			OriginalID: "empty-proj",
			Name:       "Empty Project",
		},
	}

	jsonStr, err := project.ToJSON()
	if err != nil {
		t.Fatalf("ToJSON() error: %v", err)
	}

	// Verify it's valid JSON
	var parsed ExportedProject
	if err := json.Unmarshal([]byte(jsonStr), &parsed); err != nil {
		t.Fatalf("ToJSON() produced invalid JSON: %v", err)
	}

	if parsed.GetProjectName() != "Empty Project" {
		t.Errorf("Expected 'Empty Project', got '%s'", parsed.GetProjectName())
	}
}

func TestParseExportedProject(t *testing.T) {
	// Test parsing the Node.js export format
	jsonStr := `{
		"version": "1.0",
		"metadata": {
			"exportedAt": "2025-01-01T00:00:00Z",
			"lacyLightsVersion": "1.0.0"
		},
		"project": {
			"originalId": "test-id",
			"name": "Parsed Project",
			"description": "A test description"
		},
		"fixtureDefinitions": [
			{
				"refId": "def-1",
				"manufacturer": "ACME",
				"model": "Par64",
				"type": "PAR",
				"isBuiltIn": false,
				"channels": [
					{
						"refId": "ch-1",
						"name": "Dimmer",
						"type": "INTENSITY",
						"offset": 0,
						"minValue": 0,
						"maxValue": 255,
						"defaultValue": 0
					}
				]
			}
		],
		"fixtureInstances": [
			{
				"refId": "inst-1",
				"originalId": "orig-inst-1",
				"name": "Front Wash",
				"definitionRefId": "def-1",
				"universe": 1,
				"startChannel": 1,
				"tags": ["front", "wash"]
			}
		],
		"scenes": [
			{
				"refId": "scene-1",
				"originalId": "orig-scene-1",
				"name": "Full",
				"fixtureValues": [
					{
						"fixtureRefId": "inst-1",
						"channels": [{"offset": 0, "value": 255}]
					}
				]
			}
		],
		"cueLists": [
			{
				"refId": "cl-1",
				"originalId": "orig-cl-1",
				"name": "Main",
				"loop": true,
				"cues": [
					{
						"originalId": "orig-cue-1",
						"name": "Blackout",
						"cueNumber": 0,
						"sceneRefId": "scene-1",
						"fadeInTime": 1.5,
						"fadeOutTime": 0.5
					}
				]
			}
		]
	}`

	project, err := ParseExportedProject(jsonStr)
	if err != nil {
		t.Fatalf("ParseExportedProject() error: %v", err)
	}

	if project.Version != "1.0" {
		t.Errorf("Expected version '1.0', got '%s'", project.Version)
	}
	if project.GetProjectName() != "Parsed Project" {
		t.Errorf("Expected projectName 'Parsed Project', got '%s'", project.GetProjectName())
	}
	if project.GetProjectDescription() == nil || *project.GetProjectDescription() != "A test description" {
		t.Errorf("Expected description 'A test description', got '%v'", project.GetProjectDescription())
	}

	// Check fixture definitions
	if len(project.FixtureDefinitions) != 1 {
		t.Fatalf("Expected 1 fixture definition, got %d", len(project.FixtureDefinitions))
	}
	if project.FixtureDefinitions[0].Manufacturer != "ACME" {
		t.Errorf("Expected manufacturer 'ACME', got '%s'", project.FixtureDefinitions[0].Manufacturer)
	}
	if len(project.FixtureDefinitions[0].Channels) != 1 {
		t.Errorf("Expected 1 channel, got %d", len(project.FixtureDefinitions[0].Channels))
	}

	// Check fixture instances
	if len(project.FixtureInstances) != 1 {
		t.Fatalf("Expected 1 fixture instance, got %d", len(project.FixtureInstances))
	}
	if project.FixtureInstances[0].Name != "Front Wash" {
		t.Errorf("Expected name 'Front Wash', got '%s'", project.FixtureInstances[0].Name)
	}
	if len(project.FixtureInstances[0].Tags) != 2 {
		t.Errorf("Expected 2 tags, got %d", len(project.FixtureInstances[0].Tags))
	}

	// Check scenes
	if len(project.Scenes) != 1 {
		t.Fatalf("Expected 1 scene, got %d", len(project.Scenes))
	}
	if project.Scenes[0].Name != "Full" {
		t.Errorf("Expected scene name 'Full', got '%s'", project.Scenes[0].Name)
	}

	// Check cue lists
	if len(project.CueLists) != 1 {
		t.Fatalf("Expected 1 cue list, got %d", len(project.CueLists))
	}
	if !project.CueLists[0].Loop {
		t.Error("Expected loop to be true")
	}
	if len(project.CueLists[0].Cues) != 1 {
		t.Errorf("Expected 1 cue, got %d", len(project.CueLists[0].Cues))
	}
}

func TestParseExportedProject_InvalidJSON(t *testing.T) {
	_, err := ParseExportedProject("not valid json")
	if err == nil {
		t.Error("Expected error for invalid JSON, got nil")
	}
}

func TestParseExportedProject_MinimalValid(t *testing.T) {
	jsonStr := `{"version": "1.0", "project": {"originalId": "min", "name": "Minimal"}}`

	project, err := ParseExportedProject(jsonStr)
	if err != nil {
		t.Fatalf("ParseExportedProject() error: %v", err)
	}

	if project.Version != "1.0" {
		t.Errorf("Expected version '1.0', got '%s'", project.Version)
	}
	if project.GetProjectName() != "Minimal" {
		t.Errorf("Expected projectName 'Minimal', got '%s'", project.GetProjectName())
	}
}

func TestRoundTrip_ToJSON_ParseExportedProject(t *testing.T) {
	desc := "Round trip test"
	followTime := 1.5
	easingType := "EASE_IN_OUT"
	notes := "Test notes"
	sceneOrder := 0

	original := &ExportedProject{
		Version: "1.0",
		Project: &ExportProjectInfo{
			OriginalID:  "round-trip",
			Name:        "Round Trip Test",
			Description: &desc,
		},
		FixtureDefinitions: []ExportedFixtureDefinition{
			{
				RefID:        "def-1",
				Manufacturer: "Test",
				Model:        "Model",
				Type:         "LED",
				IsBuiltIn:    true,
				Channels: []ExportedChannelDefinition{
					{Name: "Red", Type: "COLOR", Offset: 0, MinValue: 0, MaxValue: 255, DefaultValue: 0},
					{Name: "Green", Type: "COLOR", Offset: 1, MinValue: 0, MaxValue: 255, DefaultValue: 0},
					{Name: "Blue", Type: "COLOR", Offset: 2, MinValue: 0, MaxValue: 255, DefaultValue: 0},
				},
			},
		},
		FixtureInstances: []ExportedFixtureInstance{
			{
				RefID:           "inst-1",
				Name:            "Instance 1",
				Description:     &desc,
				DefinitionRefID: "def-1",
				Universe:        2,
				StartChannel:    100,
				Tags:            []string{"a", "b", "c"},
			},
		},
		Scenes: []ExportedScene{
			{
				RefID:       "scene-1",
				Name:        "Test Scene",
				Description: &desc,
				FixtureValues: []ExportedFixtureValue{
					{
						FixtureRefID: "inst-1",
						Channels: []ExportedChannelValue{
							{Offset: 0, Value: 255},
							{Offset: 1, Value: 128},
							{Offset: 2, Value: 64},
						},
						SceneOrder: &sceneOrder,
					},
				},
			},
		},
		CueLists: []ExportedCueList{
			{
				RefID:       "cl-1",
				Name:        "Test Cue List",
				Description: &desc,
				Loop:        false,
				Cues: []ExportedCue{
					{
						OriginalID:  "cue-1",
						Name:        "Test Cue",
						CueNumber:   1.5,
						SceneRefID:  "scene-1",
						FadeInTime:  2.5,
						FadeOutTime: 1.25,
						FollowTime:  &followTime,
						EasingType:  &easingType,
						Notes:       &notes,
					},
				},
			},
		},
	}

	// Convert to JSON
	jsonStr, err := original.ToJSON()
	if err != nil {
		t.Fatalf("ToJSON() error: %v", err)
	}

	// Parse back
	parsed, err := ParseExportedProject(jsonStr)
	if err != nil {
		t.Fatalf("ParseExportedProject() error: %v", err)
	}

	// Verify key fields match
	if parsed.Version != original.Version {
		t.Errorf("Version mismatch: got '%s', want '%s'", parsed.Version, original.Version)
	}
	if parsed.GetProjectName() != original.GetProjectName() {
		t.Errorf("ProjectName mismatch: got '%s', want '%s'", parsed.GetProjectName(), original.GetProjectName())
	}
	if *parsed.GetProjectDescription() != *original.GetProjectDescription() {
		t.Errorf("ProjectDescription mismatch")
	}

	// Check fixture definitions
	if len(parsed.FixtureDefinitions) != len(original.FixtureDefinitions) {
		t.Fatalf("FixtureDefinitions count mismatch")
	}
	if len(parsed.FixtureDefinitions[0].Channels) != 3 {
		t.Errorf("Expected 3 channels, got %d", len(parsed.FixtureDefinitions[0].Channels))
	}

	// Check fixture instances
	if len(parsed.FixtureInstances) != 1 {
		t.Fatalf("FixtureInstances count mismatch")
	}
	if len(parsed.FixtureInstances[0].Tags) != 3 {
		t.Errorf("Expected 3 tags, got %d", len(parsed.FixtureInstances[0].Tags))
	}

	// Check scenes
	if len(parsed.Scenes) != 1 {
		t.Fatalf("Scenes count mismatch")
	}
	if len(parsed.Scenes[0].FixtureValues) != 1 {
		t.Fatalf("FixtureValues count mismatch")
	}
	if len(parsed.Scenes[0].FixtureValues[0].Channels) != 3 {
		t.Errorf("Expected 3 channel values, got %d", len(parsed.Scenes[0].FixtureValues[0].Channels))
	}

	// Check cue lists
	if len(parsed.CueLists) != 1 {
		t.Fatalf("CueLists count mismatch")
	}
	if len(parsed.CueLists[0].Cues) != 1 {
		t.Fatalf("Cues count mismatch")
	}
	if parsed.CueLists[0].Cues[0].CueNumber != 1.5 {
		t.Errorf("CueNumber mismatch: got %f, want 1.5", parsed.CueLists[0].Cues[0].CueNumber)
	}
	if parsed.CueLists[0].Cues[0].FollowTime == nil || *parsed.CueLists[0].Cues[0].FollowTime != 1.5 {
		t.Error("FollowTime mismatch")
	}
}

func TestExportStats(t *testing.T) {
	stats := ExportStats{
		FixtureDefinitionsCount: 5,
		FixtureInstancesCount:   10,
		ScenesCount:             8,
		CueListsCount:           2,
		CuesCount:               20,
	}

	if stats.FixtureDefinitionsCount != 5 {
		t.Errorf("Expected 5, got %d", stats.FixtureDefinitionsCount)
	}
	if stats.FixtureInstancesCount != 10 {
		t.Errorf("Expected 10, got %d", stats.FixtureInstancesCount)
	}
	if stats.ScenesCount != 8 {
		t.Errorf("Expected 8, got %d", stats.ScenesCount)
	}
	if stats.CueListsCount != 2 {
		t.Errorf("Expected 2, got %d", stats.CueListsCount)
	}
	if stats.CuesCount != 20 {
		t.Errorf("Expected 20, got %d", stats.CuesCount)
	}
}

func TestExportedProject_ToJSON_Formatting(t *testing.T) {
	project := &ExportedProject{
		Version: "1.0",
		Project: &ExportProjectInfo{
			OriginalID: "format-test",
			Name:       "Format Test",
		},
	}

	jsonStr, err := project.ToJSON()
	if err != nil {
		t.Fatalf("ToJSON() error: %v", err)
	}

	// Should be pretty-printed (indented)
	if !strings.Contains(jsonStr, "\n") {
		t.Error("Expected pretty-printed JSON with newlines")
	}
	if !strings.Contains(jsonStr, "  ") {
		t.Error("Expected pretty-printed JSON with indentation")
	}
}

func TestGetProjectName_NilProject(t *testing.T) {
	project := &ExportedProject{
		Version: "1.0",
		Project: nil,
	}

	if project.GetProjectName() != "" {
		t.Errorf("Expected empty string for nil project, got '%s'", project.GetProjectName())
	}
}

func TestGetProjectDescription_NilProject(t *testing.T) {
	project := &ExportedProject{
		Version: "1.0",
		Project: nil,
	}

	if project.GetProjectDescription() != nil {
		t.Errorf("Expected nil for nil project description, got '%v'", project.GetProjectDescription())
	}
}

func TestNewService(t *testing.T) {
	// Test that NewService creates a service with nil repos
	service := NewService(nil, nil, nil, nil, nil)

	if service == nil {
		t.Error("Expected NewService to return non-nil service")
	}
}

func TestService_Structure(t *testing.T) {
	// Verify Service struct has expected fields
	service := &Service{}
	_ = service.projectRepo
	_ = service.fixtureRepo
	_ = service.sceneRepo
	_ = service.cueListRepo
	_ = service.cueRepo
}

func TestExportMetadata(t *testing.T) {
	desc := "Test export"
	metadata := &ExportMetadata{
		ExportedAt:        "2025-01-01T00:00:00Z",
		LacyLightsVersion: "1.0.0",
		Description:       &desc,
	}

	if metadata.ExportedAt != "2025-01-01T00:00:00Z" {
		t.Errorf("Expected ExportedAt '2025-01-01T00:00:00Z', got '%s'", metadata.ExportedAt)
	}
	if metadata.LacyLightsVersion != "1.0.0" {
		t.Errorf("Expected LacyLightsVersion '1.0.0', got '%s'", metadata.LacyLightsVersion)
	}
	if metadata.Description == nil || *metadata.Description != "Test export" {
		t.Errorf("Expected Description 'Test export', got '%v'", metadata.Description)
	}
}

func TestExportedFixtureMode(t *testing.T) {
	shortName := "RGB"
	mode := ExportedFixtureMode{
		RefID:        "mode-1",
		Name:         "RGB Mode",
		ShortName:    &shortName,
		ChannelCount: 3,
		ModeChannels: []ExportedModeChannel{
			{ChannelRefID: "ch-1", Offset: 0},
			{ChannelRefID: "ch-2", Offset: 1},
			{ChannelRefID: "ch-3", Offset: 2},
		},
	}

	if mode.RefID != "mode-1" {
		t.Errorf("Expected RefID 'mode-1', got '%s'", mode.RefID)
	}
	if mode.Name != "RGB Mode" {
		t.Errorf("Expected Name 'RGB Mode', got '%s'", mode.Name)
	}
	if mode.ShortName == nil || *mode.ShortName != "RGB" {
		t.Error("Expected ShortName 'RGB'")
	}
	if mode.ChannelCount != 3 {
		t.Errorf("Expected ChannelCount 3, got %d", mode.ChannelCount)
	}
	if len(mode.ModeChannels) != 3 {
		t.Errorf("Expected 3 ModeChannels, got %d", len(mode.ModeChannels))
	}
}

func TestExportedInstanceChannel(t *testing.T) {
	ch := ExportedInstanceChannel{
		Name:         "Intensity",
		Type:         "INTENSITY",
		Offset:       0,
		MinValue:     0,
		MaxValue:     255,
		DefaultValue: 0,
	}

	if ch.Name != "Intensity" {
		t.Errorf("Expected Name 'Intensity', got '%s'", ch.Name)
	}
	if ch.Type != "INTENSITY" {
		t.Errorf("Expected Type 'INTENSITY', got '%s'", ch.Type)
	}
	if ch.Offset != 0 {
		t.Errorf("Expected Offset 0, got %d", ch.Offset)
	}
	if ch.MinValue != 0 {
		t.Errorf("Expected MinValue 0, got %d", ch.MinValue)
	}
	if ch.MaxValue != 255 {
		t.Errorf("Expected MaxValue 255, got %d", ch.MaxValue)
	}
	if ch.DefaultValue != 0 {
		t.Errorf("Expected DefaultValue 0, got %d", ch.DefaultValue)
	}
}

func TestExportedProject_AllFields(t *testing.T) {
	desc := "Full test"
	followTime := 1.5
	easingType := "LINEAR"
	notes := "Test notes"
	sceneOrder := 1
	channelCount := 4
	projectOrder := 2
	shortName := "4CH"

	project := &ExportedProject{
		Version: "1.0",
		Metadata: &ExportMetadata{
			ExportedAt:        "2025-01-01T00:00:00Z",
			LacyLightsVersion: "1.0.0",
			Description:       &desc,
		},
		Project: &ExportProjectInfo{
			OriginalID:  "proj-1",
			Name:        "Full Test",
			Description: &desc,
			CreatedAt:   "2025-01-01T00:00:00Z",
			UpdatedAt:   "2025-01-02T00:00:00Z",
		},
		FixtureDefinitions: []ExportedFixtureDefinition{
			{
				RefID:        "def-1",
				Manufacturer: "TestMfg",
				Model:        "TestModel",
				Type:         "LED",
				IsBuiltIn:    false,
				Modes: []ExportedFixtureMode{
					{
						RefID:        "mode-1",
						Name:         "4 Channel",
						ShortName:    &shortName,
						ChannelCount: 4,
						ModeChannels: []ExportedModeChannel{
							{ChannelRefID: "ch-1", Offset: 0},
						},
					},
				},
				Channels: []ExportedChannelDefinition{
					{RefID: "ch-1", Name: "Red", Type: "COLOR", Offset: 0, MinValue: 0, MaxValue: 255, DefaultValue: 0},
				},
			},
		},
		FixtureInstances: []ExportedFixtureInstance{
			{
				RefID:           "inst-1",
				OriginalID:      "orig-inst-1",
				Name:            "LED 1",
				Description:     &desc,
				DefinitionRefID: "def-1",
				ModeName:        &shortName,
				ChannelCount:    &channelCount,
				Universe:        1,
				StartChannel:    1,
				Tags:            []string{"tag1", "tag2"},
				ProjectOrder:    &projectOrder,
				InstanceChannels: []ExportedInstanceChannel{
					{Name: "Red", Type: "COLOR", Offset: 0, MinValue: 0, MaxValue: 255, DefaultValue: 0},
				},
				CreatedAt: "2025-01-01T00:00:00Z",
				UpdatedAt: "2025-01-02T00:00:00Z",
			},
		},
		Scenes: []ExportedScene{
			{
				RefID:       "scene-1",
				OriginalID:  "orig-scene-1",
				Name:        "Test Scene",
				Description: &desc,
				FixtureValues: []ExportedFixtureValue{
					{
						FixtureRefID: "inst-1",
						Channels: []ExportedChannelValue{
							{Offset: 0, Value: 255},
							{Offset: 1, Value: 0},
							{Offset: 2, Value: 0},
							{Offset: 3, Value: 0},
						},
						SceneOrder: &sceneOrder,
					},
				},
				CreatedAt: "2025-01-01T00:00:00Z",
				UpdatedAt: "2025-01-02T00:00:00Z",
			},
		},
		CueLists: []ExportedCueList{
			{
				RefID:       "cl-1",
				OriginalID:  "orig-cl-1",
				Name:        "Test Cue List",
				Description: &desc,
				Loop:        true,
				Cues: []ExportedCue{
					{
						OriginalID:  "cue-1",
						Name:        "Cue 1",
						CueNumber:   1.0,
						SceneRefID:  "scene-1",
						FadeInTime:  2.0,
						FadeOutTime: 1.0,
						FollowTime:  &followTime,
						EasingType:  &easingType,
						Notes:       &notes,
						CreatedAt:   "2025-01-01T00:00:00Z",
						UpdatedAt:   "2025-01-02T00:00:00Z",
					},
				},
				CreatedAt: "2025-01-01T00:00:00Z",
				UpdatedAt: "2025-01-02T00:00:00Z",
			},
		},
	}

	// Test JSON round-trip for all fields
	jsonStr, err := project.ToJSON()
	if err != nil {
		t.Fatalf("ToJSON() error: %v", err)
	}

	parsed, err := ParseExportedProject(jsonStr)
	if err != nil {
		t.Fatalf("ParseExportedProject() error: %v", err)
	}

	// Verify metadata
	if parsed.Metadata == nil {
		t.Fatal("Expected Metadata to be present")
	}
	if parsed.Metadata.LacyLightsVersion != "1.0.0" {
		t.Errorf("Expected LacyLightsVersion '1.0.0', got '%s'", parsed.Metadata.LacyLightsVersion)
	}

	// Verify fixture definition modes
	if len(parsed.FixtureDefinitions) != 1 {
		t.Fatal("Expected 1 fixture definition")
	}
	if len(parsed.FixtureDefinitions[0].Modes) != 1 {
		t.Errorf("Expected 1 mode, got %d", len(parsed.FixtureDefinitions[0].Modes))
	}

	// Verify fixture instance channels
	if len(parsed.FixtureInstances) != 1 {
		t.Fatal("Expected 1 fixture instance")
	}
	if len(parsed.FixtureInstances[0].InstanceChannels) != 1 {
		t.Errorf("Expected 1 instance channel, got %d", len(parsed.FixtureInstances[0].InstanceChannels))
	}
	if parsed.FixtureInstances[0].ModeName == nil || *parsed.FixtureInstances[0].ModeName != "4CH" {
		t.Error("Expected ModeName '4CH'")
	}

	// Verify cue follow time, easing, notes
	if len(parsed.CueLists) != 1 || len(parsed.CueLists[0].Cues) != 1 {
		t.Fatal("Expected 1 cue list with 1 cue")
	}
	cue := parsed.CueLists[0].Cues[0]
	if cue.FollowTime == nil || *cue.FollowTime != 1.5 {
		t.Error("Expected FollowTime 1.5")
	}
	if cue.EasingType == nil || *cue.EasingType != "LINEAR" {
		t.Error("Expected EasingType 'LINEAR'")
	}
	if cue.Notes == nil || *cue.Notes != "Test notes" {
		t.Error("Expected Notes 'Test notes'")
	}
}

func TestExportStats_AllFields(t *testing.T) {
	stats := &ExportStats{
		FixtureDefinitionsCount: 5,
		FixtureInstancesCount:   10,
		ScenesCount:             3,
		CueListsCount:           2,
		CuesCount:               15,
	}

	if stats.FixtureDefinitionsCount != 5 {
		t.Errorf("Expected 5, got %d", stats.FixtureDefinitionsCount)
	}
	if stats.FixtureInstancesCount != 10 {
		t.Errorf("Expected 10, got %d", stats.FixtureInstancesCount)
	}
	if stats.ScenesCount != 3 {
		t.Errorf("Expected 3, got %d", stats.ScenesCount)
	}
	if stats.CueListsCount != 2 {
		t.Errorf("Expected 2, got %d", stats.CueListsCount)
	}
	if stats.CuesCount != 15 {
		t.Errorf("Expected 15, got %d", stats.CuesCount)
	}
}

func TestExportStats_ZeroValues(t *testing.T) {
	stats := &ExportStats{}

	if stats.FixtureDefinitionsCount != 0 {
		t.Errorf("Expected 0, got %d", stats.FixtureDefinitionsCount)
	}
	if stats.FixtureInstancesCount != 0 {
		t.Errorf("Expected 0, got %d", stats.FixtureInstancesCount)
	}
	if stats.ScenesCount != 0 {
		t.Errorf("Expected 0, got %d", stats.ScenesCount)
	}
	if stats.CueListsCount != 0 {
		t.Errorf("Expected 0, got %d", stats.CueListsCount)
	}
	if stats.CuesCount != 0 {
		t.Errorf("Expected 0, got %d", stats.CuesCount)
	}
}

func TestExportProjectInfo_AllFields(t *testing.T) {
	desc := "Project description"
	info := &ExportProjectInfo{
		OriginalID:  "proj-123",
		Name:        "Test Project",
		Description: &desc,
		CreatedAt:   "2025-01-01T00:00:00Z",
		UpdatedAt:   "2025-01-02T00:00:00Z",
	}

	if info.OriginalID != "proj-123" {
		t.Errorf("Expected 'proj-123', got '%s'", info.OriginalID)
	}
	if info.Name != "Test Project" {
		t.Errorf("Expected 'Test Project', got '%s'", info.Name)
	}
	if info.Description == nil || *info.Description != "Project description" {
		t.Error("Expected description")
	}
	if info.CreatedAt != "2025-01-01T00:00:00Z" {
		t.Errorf("Expected '2025-01-01T00:00:00Z', got '%s'", info.CreatedAt)
	}
	if info.UpdatedAt != "2025-01-02T00:00:00Z" {
		t.Errorf("Expected '2025-01-02T00:00:00Z', got '%s'", info.UpdatedAt)
	}
}

func TestExportedFixtureDefinition_AllFields(t *testing.T) {
	def := ExportedFixtureDefinition{
		RefID:        "def-1",
		Manufacturer: "TestMfg",
		Model:        "TestModel",
		Type:         "LED",
		IsBuiltIn:    true,
		Modes: []ExportedFixtureMode{
			{RefID: "mode-1", Name: "3 Channel", ChannelCount: 3},
		},
		Channels: []ExportedChannelDefinition{
			{Name: "Red", Type: "COLOR", Offset: 0, MinValue: 0, MaxValue: 255, DefaultValue: 0},
		},
	}

	if def.RefID != "def-1" {
		t.Errorf("Expected 'def-1', got '%s'", def.RefID)
	}
	if def.Manufacturer != "TestMfg" {
		t.Errorf("Expected 'TestMfg', got '%s'", def.Manufacturer)
	}
	if !def.IsBuiltIn {
		t.Error("Expected IsBuiltIn to be true")
	}
	if len(def.Modes) != 1 {
		t.Errorf("Expected 1 mode, got %d", len(def.Modes))
	}
	if len(def.Channels) != 1 {
		t.Errorf("Expected 1 channel, got %d", len(def.Channels))
	}
}

func TestExportedFixtureInstance_AllFields(t *testing.T) {
	desc := "Test fixture"
	modeName := "RGB"
	channelCount := 3
	projectOrder := 1

	inst := ExportedFixtureInstance{
		RefID:           "inst-1",
		OriginalID:      "orig-inst-1",
		Name:            "LED 1",
		Description:     &desc,
		DefinitionRefID: "def-1",
		ModeName:        &modeName,
		ChannelCount:    &channelCount,
		Universe:        1,
		StartChannel:    1,
		Tags:            []string{"front", "wash"},
		ProjectOrder:    &projectOrder,
		InstanceChannels: []ExportedInstanceChannel{
			{Name: "Red", Type: "COLOR", Offset: 0, MinValue: 0, MaxValue: 255, DefaultValue: 0},
		},
		CreatedAt: "2025-01-01T00:00:00Z",
		UpdatedAt: "2025-01-02T00:00:00Z",
	}

	if inst.RefID != "inst-1" {
		t.Errorf("Expected 'inst-1', got '%s'", inst.RefID)
	}
	if inst.Universe != 1 {
		t.Errorf("Expected 1, got %d", inst.Universe)
	}
	if len(inst.Tags) != 2 {
		t.Errorf("Expected 2 tags, got %d", len(inst.Tags))
	}
	if inst.ModeName == nil || *inst.ModeName != "RGB" {
		t.Error("Expected ModeName 'RGB'")
	}
	if inst.ChannelCount == nil || *inst.ChannelCount != 3 {
		t.Error("Expected ChannelCount 3")
	}
}

func TestExportedScene_AllFields(t *testing.T) {
	desc := "Test scene"
	sceneOrder := 1

	scene := ExportedScene{
		RefID:       "scene-1",
		OriginalID:  "orig-scene-1",
		Name:        "Opening",
		Description: &desc,
		FixtureValues: []ExportedFixtureValue{
			{
				FixtureRefID: "inst-1",
				Channels: []ExportedChannelValue{
					{Offset: 0, Value: 255},
					{Offset: 1, Value: 128},
					{Offset: 2, Value: 64},
				},
				SceneOrder: &sceneOrder,
			},
		},
		CreatedAt: "2025-01-01T00:00:00Z",
		UpdatedAt: "2025-01-02T00:00:00Z",
	}

	if scene.RefID != "scene-1" {
		t.Errorf("Expected 'scene-1', got '%s'", scene.RefID)
	}
	if scene.Name != "Opening" {
		t.Errorf("Expected 'Opening', got '%s'", scene.Name)
	}
	if len(scene.FixtureValues) != 1 {
		t.Errorf("Expected 1 fixture value, got %d", len(scene.FixtureValues))
	}
	if len(scene.FixtureValues[0].Channels) != 3 {
		t.Errorf("Expected 3 channel values, got %d", len(scene.FixtureValues[0].Channels))
	}
}

func TestExportedCueList_AllFields(t *testing.T) {
	desc := "Test cue list"
	followTime := 2.0
	easingType := "LINEAR"
	notes := "Test notes"

	cueList := ExportedCueList{
		RefID:       "cl-1",
		OriginalID:  "orig-cl-1",
		Name:        "Main Show",
		Description: &desc,
		Loop:        true,
		Cues: []ExportedCue{
			{
				OriginalID:  "cue-1",
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
		CreatedAt: "2025-01-01T00:00:00Z",
		UpdatedAt: "2025-01-02T00:00:00Z",
	}

	if cueList.RefID != "cl-1" {
		t.Errorf("Expected 'cl-1', got '%s'", cueList.RefID)
	}
	if !cueList.Loop {
		t.Error("Expected Loop to be true")
	}
	if len(cueList.Cues) != 1 {
		t.Errorf("Expected 1 cue, got %d", len(cueList.Cues))
	}
}

func TestExportedModeChannel_Fields(t *testing.T) {
	mc := ExportedModeChannel{
		ChannelRefID: "ch-1",
		Offset:       0,
	}

	if mc.ChannelRefID != "ch-1" {
		t.Errorf("Expected 'ch-1', got '%s'", mc.ChannelRefID)
	}
	if mc.Offset != 0 {
		t.Errorf("Expected 0, got %d", mc.Offset)
	}
}

func TestExportedChannelDefinition_Fields(t *testing.T) {
	ch := ExportedChannelDefinition{
		RefID:        "ch-1",
		Name:         "Intensity",
		Type:         "INTENSITY",
		Offset:       0,
		MinValue:     0,
		MaxValue:     255,
		DefaultValue: 128,
	}

	if ch.RefID != "ch-1" {
		t.Errorf("Expected 'ch-1', got '%s'", ch.RefID)
	}
	if ch.Name != "Intensity" {
		t.Errorf("Expected 'Intensity', got '%s'", ch.Name)
	}
	if ch.DefaultValue != 128 {
		t.Errorf("Expected 128, got %d", ch.DefaultValue)
	}
}

func TestExportedFixtureValue_Fields(t *testing.T) {
	sceneOrder := 2
	fv := ExportedFixtureValue{
		FixtureRefID: "inst-1",
		Channels: []ExportedChannelValue{
			{Offset: 0, Value: 255},
			{Offset: 1, Value: 128},
			{Offset: 2, Value: 64},
			{Offset: 3, Value: 32},
		},
		SceneOrder: &sceneOrder,
	}

	if fv.FixtureRefID != "inst-1" {
		t.Errorf("Expected 'inst-1', got '%s'", fv.FixtureRefID)
	}
	if len(fv.Channels) != 4 {
		t.Errorf("Expected 4 channel values, got %d", len(fv.Channels))
	}
	if fv.SceneOrder == nil || *fv.SceneOrder != 2 {
		t.Error("Expected SceneOrder 2")
	}
}

func TestExportedCue_AllOptionalFields(t *testing.T) {
	// Test cue without optional fields
	cue := ExportedCue{
		Name:        "Cue 1",
		CueNumber:   1.0,
		SceneRefID:  "scene-1",
		FadeInTime:  2.0,
		FadeOutTime: 1.0,
	}

	if cue.FollowTime != nil {
		t.Error("Expected nil FollowTime")
	}
	if cue.EasingType != nil {
		t.Error("Expected nil EasingType")
	}
	if cue.Notes != nil {
		t.Error("Expected nil Notes")
	}
}

func TestExportedCue_WithOptionalFields(t *testing.T) {
	followTime := 2.5
	easingType := "EASE_IN_OUT"
	notes := "Important cue"

	cue := ExportedCue{
		OriginalID:  "cue-123",
		Name:        "Cue 1",
		CueNumber:   1.5,
		SceneRefID:  "scene-1",
		FadeInTime:  2.0,
		FadeOutTime: 1.0,
		FollowTime:  &followTime,
		EasingType:  &easingType,
		Notes:       &notes,
		CreatedAt:   "2025-01-01T00:00:00Z",
		UpdatedAt:   "2025-01-02T00:00:00Z",
	}

	if cue.OriginalID != "cue-123" {
		t.Errorf("Expected 'cue-123', got '%s'", cue.OriginalID)
	}
	if cue.CueNumber != 1.5 {
		t.Errorf("Expected 1.5, got %f", cue.CueNumber)
	}
	if cue.FollowTime == nil || *cue.FollowTime != 2.5 {
		t.Error("Expected FollowTime 2.5")
	}
	if cue.EasingType == nil || *cue.EasingType != "EASE_IN_OUT" {
		t.Error("Expected EasingType 'EASE_IN_OUT'")
	}
	if cue.Notes == nil || *cue.Notes != "Important cue" {
		t.Error("Expected Notes 'Important cue'")
	}
}

func TestExportedProject_EmptyArrays(t *testing.T) {
	project := &ExportedProject{
		Version: "1.0",
		Project: &ExportProjectInfo{
			OriginalID: "proj-1",
			Name:       "Empty Project",
		},
		FixtureDefinitions: []ExportedFixtureDefinition{},
		FixtureInstances:   []ExportedFixtureInstance{},
		Scenes:             []ExportedScene{},
		CueLists:           []ExportedCueList{},
	}

	jsonStr, err := project.ToJSON()
	if err != nil {
		t.Fatalf("ToJSON() error: %v", err)
	}

	parsed, err := ParseExportedProject(jsonStr)
	if err != nil {
		t.Fatalf("ParseExportedProject() error: %v", err)
	}

	if len(parsed.FixtureDefinitions) != 0 {
		t.Errorf("Expected 0 fixture definitions, got %d", len(parsed.FixtureDefinitions))
	}
	if len(parsed.FixtureInstances) != 0 {
		t.Errorf("Expected 0 fixture instances, got %d", len(parsed.FixtureInstances))
	}
	if len(parsed.Scenes) != 0 {
		t.Errorf("Expected 0 scenes, got %d", len(parsed.Scenes))
	}
	if len(parsed.CueLists) != 0 {
		t.Errorf("Expected 0 cue lists, got %d", len(parsed.CueLists))
	}
}

func TestExportedProject_NilArrays(t *testing.T) {
	project := &ExportedProject{
		Version: "1.0",
		Project: &ExportProjectInfo{
			OriginalID: "proj-1",
			Name:       "Project with nil arrays",
		},
		// Leave arrays nil
	}

	jsonStr, err := project.ToJSON()
	if err != nil {
		t.Fatalf("ToJSON() error: %v", err)
	}

	parsed, err := ParseExportedProject(jsonStr)
	if err != nil {
		t.Fatalf("ParseExportedProject() error: %v", err)
	}

	// nil arrays should serialize as null in JSON
	if len(parsed.FixtureDefinitions) > 0 {
		t.Errorf("Expected nil or empty fixture definitions")
	}
}

func TestExportedFixtureDefinition_MultipleChannels(t *testing.T) {
	def := ExportedFixtureDefinition{
		RefID:        "def-1",
		Manufacturer: "TestMfg",
		Model:        "RGBW LED",
		Type:         "LED",
		IsBuiltIn:    false,
		Channels: []ExportedChannelDefinition{
			{Name: "Red", Type: "COLOR", Offset: 0, MinValue: 0, MaxValue: 255, DefaultValue: 0},
			{Name: "Green", Type: "COLOR", Offset: 1, MinValue: 0, MaxValue: 255, DefaultValue: 0},
			{Name: "Blue", Type: "COLOR", Offset: 2, MinValue: 0, MaxValue: 255, DefaultValue: 0},
			{Name: "White", Type: "COLOR", Offset: 3, MinValue: 0, MaxValue: 255, DefaultValue: 0},
			{Name: "Intensity", Type: "INTENSITY", Offset: 4, MinValue: 0, MaxValue: 255, DefaultValue: 255},
		},
	}

	if len(def.Channels) != 5 {
		t.Errorf("Expected 5 channels, got %d", len(def.Channels))
	}

	// Verify channel order
	expectedNames := []string{"Red", "Green", "Blue", "White", "Intensity"}
	for i, ch := range def.Channels {
		if ch.Name != expectedNames[i] {
			t.Errorf("Channel %d: expected '%s', got '%s'", i, expectedNames[i], ch.Name)
		}
		if ch.Offset != i {
			t.Errorf("Channel %d: expected offset %d, got %d", i, i, ch.Offset)
		}
	}
}

func TestExportedFixtureDefinition_WithModes(t *testing.T) {
	shortName1 := "3CH"
	shortName2 := "4CH"

	def := ExportedFixtureDefinition{
		RefID:        "def-1",
		Manufacturer: "TestMfg",
		Model:        "Multi-Mode LED",
		Type:         "LED",
		IsBuiltIn:    false,
		Modes: []ExportedFixtureMode{
			{
				RefID:        "mode-1",
				Name:         "3 Channel",
				ShortName:    &shortName1,
				ChannelCount: 3,
				ModeChannels: []ExportedModeChannel{
					{ChannelRefID: "ch-r", Offset: 0},
					{ChannelRefID: "ch-g", Offset: 1},
					{ChannelRefID: "ch-b", Offset: 2},
				},
			},
			{
				RefID:        "mode-2",
				Name:         "4 Channel",
				ShortName:    &shortName2,
				ChannelCount: 4,
				ModeChannels: []ExportedModeChannel{
					{ChannelRefID: "ch-r", Offset: 0},
					{ChannelRefID: "ch-g", Offset: 1},
					{ChannelRefID: "ch-b", Offset: 2},
					{ChannelRefID: "ch-i", Offset: 3},
				},
			},
		},
		Channels: []ExportedChannelDefinition{
			{RefID: "ch-r", Name: "Red", Type: "COLOR", Offset: 0, MinValue: 0, MaxValue: 255, DefaultValue: 0},
			{RefID: "ch-g", Name: "Green", Type: "COLOR", Offset: 1, MinValue: 0, MaxValue: 255, DefaultValue: 0},
			{RefID: "ch-b", Name: "Blue", Type: "COLOR", Offset: 2, MinValue: 0, MaxValue: 255, DefaultValue: 0},
			{RefID: "ch-i", Name: "Intensity", Type: "INTENSITY", Offset: 3, MinValue: 0, MaxValue: 255, DefaultValue: 255},
		},
	}

	if len(def.Modes) != 2 {
		t.Errorf("Expected 2 modes, got %d", len(def.Modes))
	}

	if def.Modes[0].ChannelCount != 3 {
		t.Errorf("Expected mode 0 channel count 3, got %d", def.Modes[0].ChannelCount)
	}

	if def.Modes[1].ChannelCount != 4 {
		t.Errorf("Expected mode 1 channel count 4, got %d", def.Modes[1].ChannelCount)
	}
}

func TestExportedFixtureDefinition_ModesRoundTrip(t *testing.T) {
	// Test that modes with channel references survive JSON round-trip
	shortName := "4CH"
	desc := "Test project"

	project := &ExportedProject{
		Version: "1.0",
		Project: &ExportProjectInfo{
			OriginalID:  "proj-1",
			Name:        "Mode Test Project",
			Description: &desc,
		},
		FixtureDefinitions: []ExportedFixtureDefinition{
			{
				RefID:        "def-1",
				Manufacturer: "Chauvet DJ",
				Model:        "SlimPar Pro RGBA",
				Type:         "LED_PAR",
				IsBuiltIn:    false,
				Channels: []ExportedChannelDefinition{
					{RefID: "ch-r", Name: "Red", Type: "COLOR", Offset: 0, MinValue: 0, MaxValue: 255, DefaultValue: 0},
					{RefID: "ch-g", Name: "Green", Type: "COLOR", Offset: 1, MinValue: 0, MaxValue: 255, DefaultValue: 0},
					{RefID: "ch-b", Name: "Blue", Type: "COLOR", Offset: 2, MinValue: 0, MaxValue: 255, DefaultValue: 0},
					{RefID: "ch-a", Name: "Amber", Type: "COLOR", Offset: 3, MinValue: 0, MaxValue: 255, DefaultValue: 0},
					{RefID: "ch-i", Name: "Intensity", Type: "INTENSITY", Offset: 4, MinValue: 0, MaxValue: 255, DefaultValue: 0},
					{RefID: "ch-s", Name: "Strobe", Type: "STROBE", Offset: 5, MinValue: 0, MaxValue: 255, DefaultValue: 0},
					{RefID: "ch-m", Name: "Mode", Type: "CONTROL", Offset: 6, MinValue: 0, MaxValue: 255, DefaultValue: 0},
				},
				Modes: []ExportedFixtureMode{
					{
						RefID:        "mode-4ch",
						Name:         "4-channel",
						ShortName:    &shortName,
						ChannelCount: 4,
						ModeChannels: []ExportedModeChannel{
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
						ModeChannels: []ExportedModeChannel{
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

	// Convert to JSON
	jsonStr, err := project.ToJSON()
	if err != nil {
		t.Fatalf("ToJSON() error: %v", err)
	}

	// Parse back
	parsed, err := ParseExportedProject(jsonStr)
	if err != nil {
		t.Fatalf("ParseExportedProject() error: %v", err)
	}

	// Verify fixture definition
	if len(parsed.FixtureDefinitions) != 1 {
		t.Fatalf("Expected 1 fixture definition, got %d", len(parsed.FixtureDefinitions))
	}

	def := parsed.FixtureDefinitions[0]
	if def.Manufacturer != "Chauvet DJ" {
		t.Errorf("Expected manufacturer 'Chauvet DJ', got '%s'", def.Manufacturer)
	}
	if def.Model != "SlimPar Pro RGBA" {
		t.Errorf("Expected model 'SlimPar Pro RGBA', got '%s'", def.Model)
	}

	// Verify channels have RefID preserved
	if len(def.Channels) != 7 {
		t.Fatalf("Expected 7 channels, got %d", len(def.Channels))
	}
	for _, ch := range def.Channels {
		if ch.RefID == "" {
			t.Errorf("Channel '%s' missing RefID", ch.Name)
		}
	}

	// Verify modes
	if len(def.Modes) != 2 {
		t.Fatalf("Expected 2 modes, got %d", len(def.Modes))
	}

	// Check 4-channel mode
	mode4ch := def.Modes[0]
	if mode4ch.Name != "4-channel" {
		t.Errorf("Expected mode name '4-channel', got '%s'", mode4ch.Name)
	}
	if mode4ch.ChannelCount != 4 {
		t.Errorf("Expected channel count 4, got %d", mode4ch.ChannelCount)
	}
	if len(mode4ch.ModeChannels) != 4 {
		t.Fatalf("Expected 4 mode channels, got %d", len(mode4ch.ModeChannels))
	}

	// Verify mode channels reference correct channel RefIDs
	expectedRefs := []string{"ch-r", "ch-g", "ch-b", "ch-a"}
	for i, mc := range mode4ch.ModeChannels {
		if mc.ChannelRefID != expectedRefs[i] {
			t.Errorf("Mode channel %d: expected RefID '%s', got '%s'", i, expectedRefs[i], mc.ChannelRefID)
		}
		if mc.Offset != i {
			t.Errorf("Mode channel %d: expected offset %d, got %d", i, i, mc.Offset)
		}
	}

	// Check 5-channel mode
	mode5ch := def.Modes[1]
	if mode5ch.Name != "5-channel" {
		t.Errorf("Expected mode name '5-channel', got '%s'", mode5ch.Name)
	}
	if mode5ch.ChannelCount != 5 {
		t.Errorf("Expected channel count 5, got %d", mode5ch.ChannelCount)
	}
	if len(mode5ch.ModeChannels) != 5 {
		t.Fatalf("Expected 5 mode channels, got %d", len(mode5ch.ModeChannels))
	}

	// Verify first channel of 5-channel mode is Intensity (ch-i)
	if mode5ch.ModeChannels[0].ChannelRefID != "ch-i" {
		t.Errorf("Expected first channel of 5-channel mode to be 'ch-i', got '%s'", mode5ch.ModeChannels[0].ChannelRefID)
	}
}

func TestExportedFixtureInstance_WithDescription(t *testing.T) {
	desc := "Front wash fixture"
	modeName := "RGB"
	channelCount := 3
	projectOrder := 5

	inst := ExportedFixtureInstance{
		RefID:           "inst-1",
		OriginalID:      "orig-1",
		Name:            "Front Wash 1",
		Description:     &desc,
		DefinitionRefID: "def-1",
		ModeName:        &modeName,
		ChannelCount:    &channelCount,
		Universe:        1,
		StartChannel:    1,
		Tags:            []string{"front", "wash", "rgb"},
		ProjectOrder:    &projectOrder,
		CreatedAt:       "2025-01-01T00:00:00Z",
		UpdatedAt:       "2025-01-02T00:00:00Z",
	}

	if inst.Description == nil || *inst.Description != "Front wash fixture" {
		t.Error("Expected description 'Front wash fixture'")
	}
	if inst.ModeName == nil || *inst.ModeName != "RGB" {
		t.Error("Expected ModeName 'RGB'")
	}
	if len(inst.Tags) != 3 {
		t.Errorf("Expected 3 tags, got %d", len(inst.Tags))
	}
	if inst.ProjectOrder == nil || *inst.ProjectOrder != 5 {
		t.Error("Expected ProjectOrder 5")
	}
}

func TestExportedScene_WithMultipleFixtureValues(t *testing.T) {
	desc := "Test scene"
	sceneOrder1 := 0
	sceneOrder2 := 1
	sceneOrder3 := 2

	scene := ExportedScene{
		RefID:       "scene-1",
		OriginalID:  "orig-scene-1",
		Name:        "Multi-Fixture Scene",
		Description: &desc,
		FixtureValues: []ExportedFixtureValue{
			{
				FixtureRefID: "inst-1",
				Channels: []ExportedChannelValue{
					{Offset: 0, Value: 255},
					{Offset: 1, Value: 0},
					{Offset: 2, Value: 0},
				},
				SceneOrder: &sceneOrder1,
			},
			{
				FixtureRefID: "inst-2",
				Channels: []ExportedChannelValue{
					{Offset: 0, Value: 0},
					{Offset: 1, Value: 255},
					{Offset: 2, Value: 0},
				},
				SceneOrder: &sceneOrder2,
			},
			{
				FixtureRefID: "inst-3",
				Channels: []ExportedChannelValue{
					{Offset: 0, Value: 0},
					{Offset: 1, Value: 0},
					{Offset: 2, Value: 255},
				},
				SceneOrder: &sceneOrder3,
			},
		},
		CreatedAt: "2025-01-01T00:00:00Z",
		UpdatedAt: "2025-01-02T00:00:00Z",
	}

	if len(scene.FixtureValues) != 3 {
		t.Errorf("Expected 3 fixture values, got %d", len(scene.FixtureValues))
	}

	// Verify scene orders
	for i, fv := range scene.FixtureValues {
		if fv.SceneOrder == nil || *fv.SceneOrder != i {
			t.Errorf("Fixture value %d: expected scene order %d", i, i)
		}
	}
}

func TestExportedCueList_WithMultipleCues(t *testing.T) {
	desc := "Main show cue list"
	followTime1 := 0.5
	followTime2 := 1.0
	easingType := "EASE_IN"
	notes := "Transition cue"

	cueList := ExportedCueList{
		RefID:       "cl-1",
		OriginalID:  "orig-cl-1",
		Name:        "Main Show",
		Description: &desc,
		Loop:        false,
		Cues: []ExportedCue{
			{
				OriginalID:  "cue-1",
				Name:        "Blackout",
				CueNumber:   0,
				SceneRefID:  "scene-blackout",
				FadeInTime:  0,
				FadeOutTime: 0,
			},
			{
				OriginalID:  "cue-2",
				Name:        "Opening",
				CueNumber:   1,
				SceneRefID:  "scene-opening",
				FadeInTime:  2.0,
				FadeOutTime: 1.0,
				FollowTime:  &followTime1,
			},
			{
				OriginalID:  "cue-3",
				Name:        "Transition",
				CueNumber:   2,
				SceneRefID:  "scene-transition",
				FadeInTime:  1.5,
				FadeOutTime: 0.5,
				FollowTime:  &followTime2,
				EasingType:  &easingType,
				Notes:       &notes,
			},
		},
		CreatedAt: "2025-01-01T00:00:00Z",
		UpdatedAt: "2025-01-02T00:00:00Z",
	}

	if len(cueList.Cues) != 3 {
		t.Errorf("Expected 3 cues, got %d", len(cueList.Cues))
	}

	// Verify cue numbers are sequential
	for i, cue := range cueList.Cues {
		if cue.CueNumber != float64(i) {
			t.Errorf("Cue %d: expected cue number %d, got %f", i, i, cue.CueNumber)
		}
	}
}

func TestParseExportedProject_EmptyObject(t *testing.T) {
	project, err := ParseExportedProject("{}")
	if err != nil {
		t.Fatalf("ParseExportedProject() error: %v", err)
	}

	if project.Version != "" {
		t.Errorf("Expected empty version, got '%s'", project.Version)
	}
	if project.Project != nil {
		t.Error("Expected nil project")
	}
}

func TestParseExportedProject_VersionOnly(t *testing.T) {
	project, err := ParseExportedProject(`{"version": "2.0"}`)
	if err != nil {
		t.Fatalf("ParseExportedProject() error: %v", err)
	}

	if project.Version != "2.0" {
		t.Errorf("Expected version '2.0', got '%s'", project.Version)
	}
}

func TestExportStats_Increment(t *testing.T) {
	stats := &ExportStats{}

	stats.FixtureDefinitionsCount++
	stats.FixtureInstancesCount += 5
	stats.ScenesCount = 10
	stats.CueListsCount++
	stats.CuesCount = 25

	if stats.FixtureDefinitionsCount != 1 {
		t.Errorf("Expected 1, got %d", stats.FixtureDefinitionsCount)
	}
	if stats.FixtureInstancesCount != 5 {
		t.Errorf("Expected 5, got %d", stats.FixtureInstancesCount)
	}
	if stats.ScenesCount != 10 {
		t.Errorf("Expected 10, got %d", stats.ScenesCount)
	}
	if stats.CueListsCount != 1 {
		t.Errorf("Expected 1, got %d", stats.CueListsCount)
	}
	if stats.CuesCount != 25 {
		t.Errorf("Expected 25, got %d", stats.CuesCount)
	}
}

func TestService_NilRepos(t *testing.T) {
	service := NewService(nil, nil, nil, nil, nil)

	if service == nil {
		t.Fatal("Expected non-nil service")
	}
	if service.projectRepo != nil {
		t.Error("Expected nil projectRepo")
	}
	if service.fixtureRepo != nil {
		t.Error("Expected nil fixtureRepo")
	}
	if service.sceneRepo != nil {
		t.Error("Expected nil sceneRepo")
	}
	if service.cueListRepo != nil {
		t.Error("Expected nil cueListRepo")
	}
	if service.cueRepo != nil {
		t.Error("Expected nil cueRepo")
	}
}

func TestExportedInstanceChannel_AllTypes(t *testing.T) {
	channelTypes := []struct {
		name     string
		chanType string
	}{
		{"Intensity", "INTENSITY"},
		{"Red", "COLOR"},
		{"Pan", "POSITION"},
		{"Strobe", "STROBE"},
		{"Gobo", "GOBO"},
		{"Effect", "EFFECT"},
	}

	for _, ct := range channelTypes {
		ch := ExportedInstanceChannel{
			Name:         ct.name,
			Type:         ct.chanType,
			Offset:       0,
			MinValue:     0,
			MaxValue:     255,
			DefaultValue: 0,
		}

		if ch.Name != ct.name {
			t.Errorf("Expected name '%s', got '%s'", ct.name, ch.Name)
		}
		if ch.Type != ct.chanType {
			t.Errorf("Expected type '%s', got '%s'", ct.chanType, ch.Type)
		}
	}
}

func TestExportMetadata_WithNilDescription(t *testing.T) {
	metadata := &ExportMetadata{
		ExportedAt:        "2025-01-01T00:00:00Z",
		LacyLightsVersion: "1.0.0",
		Description:       nil,
	}

	if metadata.Description != nil {
		t.Error("Expected nil description")
	}
}

func TestExportProjectInfo_WithNilDescription(t *testing.T) {
	info := &ExportProjectInfo{
		OriginalID:  "proj-1",
		Name:        "No Description Project",
		Description: nil,
	}

	if info.Description != nil {
		t.Error("Expected nil description")
	}
}

func TestExportedFixtureInstance_WithNilOptionals(t *testing.T) {
	inst := ExportedFixtureInstance{
		RefID:           "inst-1",
		Name:            "Basic Instance",
		DefinitionRefID: "def-1",
		Universe:        1,
		StartChannel:    1,
	}

	if inst.Description != nil {
		t.Error("Expected nil Description")
	}
	if inst.ModeName != nil {
		t.Error("Expected nil ModeName")
	}
	if inst.ChannelCount != nil {
		t.Error("Expected nil ChannelCount")
	}
	if inst.ProjectOrder != nil {
		t.Error("Expected nil ProjectOrder")
	}
	if len(inst.Tags) != 0 {
		t.Error("Expected empty Tags")
	}
	if len(inst.InstanceChannels) != 0 {
		t.Error("Expected empty InstanceChannels")
	}
}

// Integration tests with in-memory database
// These tests require database setup and test actual export functionality

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

func TestExportProject_Integration_EmptyProject(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	projectRepo := repositories.NewProjectRepository(db)
	fixtureRepo := repositories.NewFixtureRepository(db)
	sceneRepo := repositories.NewSceneRepository(db)
	cueListRepo := repositories.NewCueListRepository(db)
	cueRepo := repositories.NewCueRepository(db)

	service := NewService(projectRepo, fixtureRepo, sceneRepo, cueListRepo, cueRepo)
	ctx := context.Background()

	// Create project
	project := &models.Project{ID: cuid.New(), Name: "Test Export Project"}
	_ = projectRepo.Create(ctx, project)

	// Export with all flags
	exported, stats, err := service.ExportProject(ctx, project.ID, true, true, true)
	if err != nil {
		t.Fatalf("ExportProject failed: %v", err)
	}
	if exported == nil {
		t.Fatal("Expected exported project")
	}
	if stats == nil {
		t.Fatal("Expected export stats")
	}

	if exported.GetProjectName() != "Test Export Project" {
		t.Errorf("Expected project name 'Test Export Project', got '%s'", exported.GetProjectName())
	}
	if stats.FixtureDefinitionsCount != 0 {
		t.Errorf("Expected 0 fixture definitions, got %d", stats.FixtureDefinitionsCount)
	}
}

func TestExportProject_Integration_NotFound(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	projectRepo := repositories.NewProjectRepository(db)
	fixtureRepo := repositories.NewFixtureRepository(db)
	sceneRepo := repositories.NewSceneRepository(db)
	cueListRepo := repositories.NewCueListRepository(db)
	cueRepo := repositories.NewCueRepository(db)

	service := NewService(projectRepo, fixtureRepo, sceneRepo, cueListRepo, cueRepo)
	ctx := context.Background()

	// Export non-existent project
	exported, stats, err := service.ExportProject(ctx, "non-existent-id", true, true, true)
	if err != nil {
		t.Fatalf("ExportProject should not error for non-existent project: %v", err)
	}
	if exported != nil {
		t.Error("Expected nil exported for non-existent project")
	}
	if stats != nil {
		t.Error("Expected nil stats for non-existent project")
	}
}

func TestExportProject_Integration_WithFixtures(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	projectRepo := repositories.NewProjectRepository(db)
	fixtureRepo := repositories.NewFixtureRepository(db)
	sceneRepo := repositories.NewSceneRepository(db)
	cueListRepo := repositories.NewCueListRepository(db)
	cueRepo := repositories.NewCueRepository(db)

	service := NewService(projectRepo, fixtureRepo, sceneRepo, cueListRepo, cueRepo)
	ctx := context.Background()

	// Create project
	project := &models.Project{ID: cuid.New(), Name: "Fixture Export Test"}
	_ = projectRepo.Create(ctx, project)

	// Create fixture definition with channels
	def := &models.FixtureDefinition{
		ID:           cuid.New(),
		Manufacturer: "TestMfg",
		Model:        "TestModel",
		Type:         "LED_PAR",
	}
	_ = fixtureRepo.CreateDefinition(ctx, def)

	ch1 := &models.ChannelDefinition{ID: cuid.New(), Name: "Red", Type: "COLOR", Offset: 0, DefinitionID: def.ID}
	ch2 := &models.ChannelDefinition{ID: cuid.New(), Name: "Green", Type: "COLOR", Offset: 1, DefinitionID: def.ID}
	ch3 := &models.ChannelDefinition{ID: cuid.New(), Name: "Blue", Type: "COLOR", Offset: 2, DefinitionID: def.ID}
	db.Create(ch1)
	db.Create(ch2)
	db.Create(ch3)

	// Create mode for definition
	mode := &models.FixtureMode{ID: cuid.New(), Name: "3 Channel", ChannelCount: 3, DefinitionID: def.ID}
	_ = fixtureRepo.CreateMode(ctx, mode)

	modeChannels := []models.ModeChannel{
		{ModeID: mode.ID, ChannelID: ch1.ID, Offset: 0},
		{ModeID: mode.ID, ChannelID: ch2.ID, Offset: 1},
		{ModeID: mode.ID, ChannelID: ch3.ID, Offset: 2},
	}
	_ = fixtureRepo.CreateModeChannels(ctx, modeChannels)

	// Create fixture instance
	modeName := "3 Channel"
	channelCount := 3
	tags := "front,wash"
	fixture := &models.FixtureInstance{
		ID:           cuid.New(),
		Name:         "LED 1",
		ProjectID:    project.ID,
		DefinitionID: def.ID,
		ModeName:     &modeName,
		ChannelCount: &channelCount,
		Universe:     1,
		StartChannel: 1,
		Tags:         &tags,
	}
	_ = fixtureRepo.Create(ctx, fixture)

	// Create instance channels
	ic1 := &models.InstanceChannel{FixtureID: fixture.ID, Name: "Red", Type: "COLOR", Offset: 0}
	ic2 := &models.InstanceChannel{FixtureID: fixture.ID, Name: "Green", Type: "COLOR", Offset: 1}
	ic3 := &models.InstanceChannel{FixtureID: fixture.ID, Name: "Blue", Type: "COLOR", Offset: 2}
	db.Create(ic1)
	db.Create(ic2)
	db.Create(ic3)

	// Export with fixtures
	exported, stats, err := service.ExportProject(ctx, project.ID, true, false, false)
	if err != nil {
		t.Fatalf("ExportProject failed: %v", err)
	}

	if stats.FixtureDefinitionsCount != 1 {
		t.Errorf("Expected 1 fixture definition, got %d", stats.FixtureDefinitionsCount)
	}
	if stats.FixtureInstancesCount != 1 {
		t.Errorf("Expected 1 fixture instance, got %d", stats.FixtureInstancesCount)
	}

	// Verify definition export
	if len(exported.FixtureDefinitions) != 1 {
		t.Fatalf("Expected 1 fixture definition in export, got %d", len(exported.FixtureDefinitions))
	}
	expDef := exported.FixtureDefinitions[0]
	if expDef.Manufacturer != "TestMfg" {
		t.Errorf("Expected manufacturer 'TestMfg', got '%s'", expDef.Manufacturer)
	}
	if len(expDef.Channels) != 3 {
		t.Errorf("Expected 3 channels, got %d", len(expDef.Channels))
	}
	if len(expDef.Modes) != 1 {
		t.Errorf("Expected 1 mode, got %d", len(expDef.Modes))
	}

	// Verify mode export
	expMode := expDef.Modes[0]
	if expMode.Name != "3 Channel" {
		t.Errorf("Expected mode name '3 Channel', got '%s'", expMode.Name)
	}
	if len(expMode.ModeChannels) != 3 {
		t.Errorf("Expected 3 mode channels, got %d", len(expMode.ModeChannels))
	}

	// Verify fixture instance export
	if len(exported.FixtureInstances) != 1 {
		t.Fatalf("Expected 1 fixture instance, got %d", len(exported.FixtureInstances))
	}
	expInst := exported.FixtureInstances[0]
	if expInst.Name != "LED 1" {
		t.Errorf("Expected instance name 'LED 1', got '%s'", expInst.Name)
	}
	if expInst.ModeName == nil || *expInst.ModeName != "3 Channel" {
		t.Error("Expected ModeName '3 Channel'")
	}
}

func TestExportProject_Integration_WithScenes(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	projectRepo := repositories.NewProjectRepository(db)
	fixtureRepo := repositories.NewFixtureRepository(db)
	sceneRepo := repositories.NewSceneRepository(db)
	cueListRepo := repositories.NewCueListRepository(db)
	cueRepo := repositories.NewCueRepository(db)

	service := NewService(projectRepo, fixtureRepo, sceneRepo, cueListRepo, cueRepo)
	ctx := context.Background()

	// Create project
	project := &models.Project{ID: cuid.New(), Name: "Scene Export Test"}
	_ = projectRepo.Create(ctx, project)

	// Create definition and fixture
	def := &models.FixtureDefinition{ID: cuid.New(), Manufacturer: "T", Model: "M", Type: "t"}
	_ = fixtureRepo.CreateDefinition(ctx, def)
	fixture := &models.FixtureInstance{
		ID:           cuid.New(),
		Name:         "F1",
		ProjectID:    project.ID,
		DefinitionID: def.ID,
		Universe:     1,
		StartChannel: 1,
	}
	_ = fixtureRepo.Create(ctx, fixture)

	// Create scene with fixture values
	sceneDesc := "Test scene description"
	scene := &models.Scene{
		ID:          cuid.New(),
		Name:        "Test Scene",
		Description: &sceneDesc,
		ProjectID:   project.ID,
	}
	_ = sceneRepo.Create(ctx, scene)

	sceneOrder := 0
	fv := &models.FixtureValue{
		SceneID:    scene.ID,
		FixtureID:  fixture.ID,
		Channels:   `[{"offset":0,"value":255},{"offset":1,"value":128}]`,
		SceneOrder: &sceneOrder,
	}
	_ = sceneRepo.CreateFixtureValue(ctx, fv)

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
	expScene := exported.Scenes[0]
	if expScene.Name != "Test Scene" {
		t.Errorf("Expected scene name 'Test Scene', got '%s'", expScene.Name)
	}
	if len(expScene.FixtureValues) != 1 {
		t.Errorf("Expected 1 fixture value, got %d", len(expScene.FixtureValues))
	}
}

func TestExportProject_Integration_WithCueLists(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	projectRepo := repositories.NewProjectRepository(db)
	fixtureRepo := repositories.NewFixtureRepository(db)
	sceneRepo := repositories.NewSceneRepository(db)
	cueListRepo := repositories.NewCueListRepository(db)
	cueRepo := repositories.NewCueRepository(db)

	service := NewService(projectRepo, fixtureRepo, sceneRepo, cueListRepo, cueRepo)
	ctx := context.Background()

	// Create project
	project := &models.Project{ID: cuid.New(), Name: "CueList Export Test"}
	_ = projectRepo.Create(ctx, project)

	// Create scene
	scene := &models.Scene{ID: cuid.New(), Name: "Scene 1", ProjectID: project.ID}
	_ = sceneRepo.Create(ctx, scene)

	// Create cue list
	cueListDesc := "Main show"
	cueList := &models.CueList{
		ID:          cuid.New(),
		Name:        "Main CueList",
		Description: &cueListDesc,
		ProjectID:   project.ID,
		Loop:        true,
	}
	_ = cueListRepo.Create(ctx, cueList)

	// Create cue
	followTime := 2.0
	easingType := "LINEAR"
	notes := "Opening cue"
	cue := &models.Cue{
		ID:          cuid.New(),
		Name:        "Cue 1",
		CueNumber:   1.0,
		CueListID:   cueList.ID,
		SceneID:     scene.ID,
		FadeInTime:  2.0,
		FadeOutTime: 1.0,
		FollowTime:  &followTime,
		EasingType:  &easingType,
		Notes:       &notes,
	}
	_ = cueRepo.Create(ctx, cue)

	// Export with cue lists
	exported, stats, err := service.ExportProject(ctx, project.ID, false, true, true)
	if err != nil {
		t.Fatalf("ExportProject failed: %v", err)
	}

	if stats.CueListsCount != 1 {
		t.Errorf("Expected 1 cue list, got %d", stats.CueListsCount)
	}
	if stats.CuesCount != 1 {
		t.Errorf("Expected 1 cue, got %d", stats.CuesCount)
	}

	if len(exported.CueLists) != 1 {
		t.Fatalf("Expected 1 cue list in export, got %d", len(exported.CueLists))
	}
	expCL := exported.CueLists[0]
	if expCL.Name != "Main CueList" {
		t.Errorf("Expected cue list name 'Main CueList', got '%s'", expCL.Name)
	}
	if !expCL.Loop {
		t.Error("Expected loop to be true")
	}
	if len(expCL.Cues) != 1 {
		t.Fatalf("Expected 1 cue, got %d", len(expCL.Cues))
	}

	expCue := expCL.Cues[0]
	if expCue.Name != "Cue 1" {
		t.Errorf("Expected cue name 'Cue 1', got '%s'", expCue.Name)
	}
	if expCue.FollowTime == nil || *expCue.FollowTime != 2.0 {
		t.Error("Expected FollowTime 2.0")
	}
	if expCue.EasingType == nil || *expCue.EasingType != "LINEAR" {
		t.Error("Expected EasingType 'LINEAR'")
	}
	if expCue.Notes == nil || *expCue.Notes != "Opening cue" {
		t.Error("Expected Notes 'Opening cue'")
	}
}

func TestExportProject_Integration_SelectiveExport(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	projectRepo := repositories.NewProjectRepository(db)
	fixtureRepo := repositories.NewFixtureRepository(db)
	sceneRepo := repositories.NewSceneRepository(db)
	cueListRepo := repositories.NewCueListRepository(db)
	cueRepo := repositories.NewCueRepository(db)

	service := NewService(projectRepo, fixtureRepo, sceneRepo, cueListRepo, cueRepo)
	ctx := context.Background()

	// Create project with fixtures, scenes, cue lists
	project := &models.Project{ID: cuid.New(), Name: "Selective Export Test"}
	_ = projectRepo.Create(ctx, project)

	def := &models.FixtureDefinition{ID: cuid.New(), Manufacturer: "T", Model: "M", Type: "t"}
	_ = fixtureRepo.CreateDefinition(ctx, def)
	fixture := &models.FixtureInstance{ID: cuid.New(), Name: "F1", ProjectID: project.ID, DefinitionID: def.ID, Universe: 1, StartChannel: 1}
	_ = fixtureRepo.Create(ctx, fixture)

	scene := &models.Scene{ID: cuid.New(), Name: "S1", ProjectID: project.ID}
	_ = sceneRepo.Create(ctx, scene)

	cueList := &models.CueList{ID: cuid.New(), Name: "CL1", ProjectID: project.ID}
	_ = cueListRepo.Create(ctx, cueList)

	// Export fixtures only
	exported, stats, err := service.ExportProject(ctx, project.ID, true, false, false)
	if err != nil {
		t.Fatalf("ExportProject failed: %v", err)
	}
	if stats.FixtureInstancesCount != 1 {
		t.Errorf("Expected 1 fixture, got %d", stats.FixtureInstancesCount)
	}
	if stats.ScenesCount != 0 {
		t.Errorf("Expected 0 scenes, got %d", stats.ScenesCount)
	}
	if stats.CueListsCount != 0 {
		t.Errorf("Expected 0 cue lists, got %d", stats.CueListsCount)
	}
	if len(exported.Scenes) != 0 {
		t.Errorf("Expected no scenes in export, got %d", len(exported.Scenes))
	}
	if len(exported.CueLists) != 0 {
		t.Errorf("Expected no cue lists in export, got %d", len(exported.CueLists))
	}
}
