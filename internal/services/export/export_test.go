package export

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestExportedProject_ToJSON(t *testing.T) {
	desc := "Test project description"
	project := &ExportedProject{
		Version:            "1.0",
		ProjectID:          "proj-123",
		ProjectName:        "Test Project",
		ProjectDescription: &desc,
		FixtureDefinitions: []ExportedFixtureDefinition{
			{
				ID:           "def-1",
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
				ID:           "fix-1",
				Name:         "Fixture 1",
				DefinitionID: "def-1",
				Universe:     1,
				StartChannel: 1,
				Tags:         []string{"front", "stage"},
			},
		},
		Scenes: []ExportedScene{
			{
				ID:   "scene-1",
				Name: "Opening",
				FixtureValues: []ExportedFixtureValue{
					{
						FixtureID:     "fix-1",
						ChannelValues: []int{255},
					},
				},
			},
		},
		CueLists: []ExportedCueList{
			{
				ID:   "cuelist-1",
				Name: "Main",
				Loop: true,
				Cues: []ExportedCue{
					{
						ID:          "cue-1",
						Name:        "Cue 1",
						CueNumber:   1.0,
						SceneID:     "scene-1",
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
	if parsed["projectId"] != "proj-123" {
		t.Errorf("Expected projectId 'proj-123', got '%v'", parsed["projectId"])
	}
	if parsed["projectName"] != "Test Project" {
		t.Errorf("Expected projectName 'Test Project', got '%v'", parsed["projectName"])
	}
}

func TestExportedProject_ToJSON_Empty(t *testing.T) {
	project := &ExportedProject{
		Version:     "1.0",
		ProjectID:   "empty-proj",
		ProjectName: "Empty Project",
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

	if parsed.ProjectName != "Empty Project" {
		t.Errorf("Expected 'Empty Project', got '%s'", parsed.ProjectName)
	}
}

func TestParseExportedProject(t *testing.T) {
	jsonStr := `{
		"version": "1.0",
		"projectId": "test-id",
		"projectName": "Parsed Project",
		"projectDescription": "A test description",
		"fixtureDefinitions": [
			{
				"id": "def-1",
				"manufacturer": "ACME",
				"model": "Par64",
				"type": "PAR",
				"isBuiltIn": false,
				"channels": [
					{
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
				"id": "inst-1",
				"name": "Front Wash",
				"definitionId": "def-1",
				"universe": 1,
				"startChannel": 1,
				"tags": ["front", "wash"]
			}
		],
		"scenes": [
			{
				"id": "scene-1",
				"name": "Full",
				"fixtureValues": [
					{
						"fixtureId": "inst-1",
						"channelValues": [255]
					}
				]
			}
		],
		"cueLists": [
			{
				"id": "cl-1",
				"name": "Main",
				"loop": true,
				"cues": [
					{
						"id": "cue-1",
						"name": "Blackout",
						"cueNumber": 0,
						"sceneId": "scene-1",
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
	if project.ProjectID != "test-id" {
		t.Errorf("Expected projectId 'test-id', got '%s'", project.ProjectID)
	}
	if project.ProjectName != "Parsed Project" {
		t.Errorf("Expected projectName 'Parsed Project', got '%s'", project.ProjectName)
	}
	if project.ProjectDescription == nil || *project.ProjectDescription != "A test description" {
		t.Errorf("Expected description 'A test description', got '%v'", project.ProjectDescription)
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
	jsonStr := `{"version": "1.0", "projectId": "min", "projectName": "Minimal"}`

	project, err := ParseExportedProject(jsonStr)
	if err != nil {
		t.Fatalf("ParseExportedProject() error: %v", err)
	}

	if project.Version != "1.0" {
		t.Errorf("Expected version '1.0', got '%s'", project.Version)
	}
	if project.ProjectID != "min" {
		t.Errorf("Expected projectId 'min', got '%s'", project.ProjectID)
	}
}

func TestRoundTrip_ToJSON_ParseExportedProject(t *testing.T) {
	desc := "Round trip test"
	followTime := 1.5
	easingType := "EASE_IN_OUT"
	notes := "Test notes"
	sceneOrder := 0

	original := &ExportedProject{
		Version:            "1.0",
		ProjectID:          "round-trip",
		ProjectName:        "Round Trip Test",
		ProjectDescription: &desc,
		FixtureDefinitions: []ExportedFixtureDefinition{
			{
				ID:           "def-1",
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
				ID:           "inst-1",
				Name:         "Instance 1",
				Description:  &desc,
				DefinitionID: "def-1",
				Universe:     2,
				StartChannel: 100,
				Tags:         []string{"a", "b", "c"},
			},
		},
		Scenes: []ExportedScene{
			{
				ID:          "scene-1",
				Name:        "Test Scene",
				Description: &desc,
				FixtureValues: []ExportedFixtureValue{
					{FixtureID: "inst-1", ChannelValues: []int{255, 128, 64}, SceneOrder: &sceneOrder},
				},
			},
		},
		CueLists: []ExportedCueList{
			{
				ID:          "cl-1",
				Name:        "Test Cue List",
				Description: &desc,
				Loop:        false,
				Cues: []ExportedCue{
					{
						ID:          "cue-1",
						Name:        "Test Cue",
						CueNumber:   1.5,
						SceneID:     "scene-1",
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
	if parsed.ProjectID != original.ProjectID {
		t.Errorf("ProjectID mismatch: got '%s', want '%s'", parsed.ProjectID, original.ProjectID)
	}
	if parsed.ProjectName != original.ProjectName {
		t.Errorf("ProjectName mismatch: got '%s', want '%s'", parsed.ProjectName, original.ProjectName)
	}
	if *parsed.ProjectDescription != *original.ProjectDescription {
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
		Version:     "1.0",
		ProjectID:   "format-test",
		ProjectName: "Format Test",
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
