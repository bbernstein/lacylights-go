package export

import (
	"encoding/json"
	"strings"
	"testing"
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
						FixtureRefID:  "fix-1",
						ChannelValues: []int{255},
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
						"channelValues": [255]
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
					{FixtureRefID: "inst-1", ChannelValues: []int{255, 128, 64}, SceneOrder: &sceneOrder},
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
	if len(parsed.Scenes[0].FixtureValues[0].ChannelValues) != 3 {
		t.Errorf("Expected 3 channel values, got %d", len(parsed.Scenes[0].FixtureValues[0].ChannelValues))
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
					{FixtureRefID: "inst-1", ChannelValues: []int{255, 0, 0, 0}, SceneOrder: &sceneOrder},
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
			{FixtureRefID: "inst-1", ChannelValues: []int{255, 128, 64}, SceneOrder: &sceneOrder},
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
	if len(scene.FixtureValues[0].ChannelValues) != 3 {
		t.Errorf("Expected 3 channel values, got %d", len(scene.FixtureValues[0].ChannelValues))
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
		FixtureRefID:  "inst-1",
		ChannelValues: []int{255, 128, 64, 32},
		SceneOrder:    &sceneOrder,
	}

	if fv.FixtureRefID != "inst-1" {
		t.Errorf("Expected 'inst-1', got '%s'", fv.FixtureRefID)
	}
	if len(fv.ChannelValues) != 4 {
		t.Errorf("Expected 4 channel values, got %d", len(fv.ChannelValues))
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
			{FixtureRefID: "inst-1", ChannelValues: []int{255, 0, 0}, SceneOrder: &sceneOrder1},
			{FixtureRefID: "inst-2", ChannelValues: []int{0, 255, 0}, SceneOrder: &sceneOrder2},
			{FixtureRefID: "inst-3", ChannelValues: []int{0, 0, 255}, SceneOrder: &sceneOrder3},
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
