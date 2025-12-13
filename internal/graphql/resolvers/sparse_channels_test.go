package resolvers

import (
	"testing"
	"time"

	"github.com/99designs/gqlgen/client"

	"github.com/bbernstein/lacylights-go/internal/database/models"
)

// TestSparseChannels_CreateScene tests creating a scene with sparse channel format
// instead of the full channel array. Only specified channels are stored.
func TestSparseChannels_CreateScene(t *testing.T) {
	c, _, cleanup := testSetup(t)
	defer cleanup()

	// Create project, definition, and fixture instance with 6 channels
	var projectResp struct {
		CreateProject struct {
			ID string `json:"id"`
		} `json:"createProject"`
	}
	err := c.Post(`mutation { createProject(input: { name: "Test Project" }) { id } }`, &projectResp)
	if err != nil {
		t.Fatalf("CreateProject mutation failed: %v", err)
	}

	var defResp struct {
		CreateFixtureDefinition struct {
			ID string `json:"id"`
		} `json:"createFixtureDefinition"`
	}
	err = c.Post(`mutation {
		createFixtureDefinition(input: {
			manufacturer: "Test"
			model: "TestPar6"
			type: LED_PAR
			channels: [
				{ name: "Red", type: RED, offset: 0, minValue: 0, maxValue: 255, defaultValue: 0 }
				{ name: "Green", type: GREEN, offset: 1, minValue: 0, maxValue: 255, defaultValue: 0 }
				{ name: "Blue", type: BLUE, offset: 2, minValue: 0, maxValue: 255, defaultValue: 0 }
				{ name: "White", type: WHITE, offset: 3, minValue: 0, maxValue: 255, defaultValue: 0 }
				{ name: "Dimmer", type: INTENSITY, offset: 4, minValue: 0, maxValue: 255, defaultValue: 0 }
				{ name: "Strobe", type: STROBE, offset: 5, minValue: 0, maxValue: 255, defaultValue: 0 }
			]
		}) {
			id
		}
	}`, &defResp)
	if err != nil {
		t.Fatalf("CreateFixtureDefinition mutation failed: %v", err)
	}

	var instanceResp struct {
		CreateFixtureInstance struct {
			ID string `json:"id"`
		} `json:"createFixtureInstance"`
	}
	err = c.Post(`mutation($projectId: ID!, $defId: ID!) {
		createFixtureInstance(input: {
			name: "Test Par 6ch"
			projectId: $projectId
			definitionId: $defId
			universe: 1
			startChannel: 1
		}) {
			id
		}
	}`, &instanceResp,
		client.Var("projectId", projectResp.CreateProject.ID),
		client.Var("defId", defResp.CreateFixtureDefinition.ID))
	if err != nil {
		t.Fatalf("CreateFixtureInstance mutation failed: %v", err)
	}

	// Create scene with SPARSE channels format
	// Only setting Red=255, Blue=128, Dimmer=200 (offsets 0, 2, 4)
	var sceneResp struct {
		CreateScene struct {
			ID          string `json:"id"`
			Name        string `json:"name"`
			Description string `json:"description"`
		} `json:"createScene"`
	}
	err = c.Post(`mutation($projectId: ID!, $fixtureId: ID!) {
		createScene(input: {
			name: "Sparse Channel Scene"
			description: "Only Red, Blue, and Dimmer set"
			projectId: $projectId
			fixtureValues: [
				{
					fixtureId: $fixtureId
					channels: [
						{ offset: 0, value: 255 }
						{ offset: 2, value: 128 }
						{ offset: 4, value: 200 }
					]
				}
			]
		}) {
			id
			name
			description
		}
	}`, &sceneResp,
		client.Var("projectId", projectResp.CreateProject.ID),
		client.Var("fixtureId", instanceResp.CreateFixtureInstance.ID))

	if err != nil {
		t.Fatalf("CreateScene mutation with sparse channels failed: %v", err)
	}

	if sceneResp.CreateScene.ID == "" {
		t.Error("Expected scene ID to be set")
	}
	if sceneResp.CreateScene.Name != "Sparse Channel Scene" {
		t.Errorf("Expected name 'Sparse Channel Scene', got '%s'", sceneResp.CreateScene.Name)
	}
}

// TestSparseChannels_QueryScene tests that querying a scene returns sparse channels
func TestSparseChannels_QueryScene(t *testing.T) {
	c, resolver, cleanup := testSetup(t)
	defer cleanup()

	// Create project, definition, and fixture with 4 channels
	project := &models.Project{
		ID:   "test-project-sparse-query",
		Name: "Test Project",
	}
	resolver.db.Create(project)

	fixtureDef := &models.FixtureDefinition{
		ID:           "test-fixture-def-sparse-query",
		Manufacturer: "Test",
		Model:        "TestPar4",
		Type:         "LED_PAR",
	}
	resolver.db.Create(fixtureDef)

	fixture := &models.FixtureInstance{
		ID:           "test-fixture-sparse-query",
		Name:         "Test Par 4ch",
		ProjectID:    project.ID,
		DefinitionID: fixtureDef.ID,
		Universe:     1,
		StartChannel: 10,
	}
	resolver.db.Create(fixture)

	// Create a scene (in the new format, this will use sparse storage)
	scene := &models.Scene{
		ID:        "test-scene-sparse-query",
		Name:      "Test Scene",
		ProjectID: project.ID,
	}
	resolver.db.Create(scene)

	// Create fixture values with sparse channels (offset 1=100, offset 3=200)
	// Using new sparse format - only store non-zero channels
	fixtureValue := &models.FixtureValue{
		ID:        "fv-sparse-query",
		SceneID:   scene.ID,
		FixtureID: fixture.ID,
		// New sparse format: only specify non-zero channels
		Channels: `[{"offset":1,"value":100},{"offset":3,"value":200}]`,
	}
	resolver.db.Create(fixtureValue)

	// Query the scene and expect sparse channels back
	var readResp struct {
		Scene struct {
			ID            string `json:"id"`
			Name          string `json:"name"`
			FixtureValues []struct {
				Fixture struct {
					ID string `json:"id"`
				} `json:"fixture"`
				Channels []struct {
					Offset int `json:"offset"`
					Value  int `json:"value"`
				} `json:"channels"`
			} `json:"fixtureValues"`
		} `json:"scene"`
	}
	err := c.Post(`query($id: ID!) {
		scene(id: $id) {
			id
			name
			fixtureValues {
				fixture {
					id
				}
				channels {
					offset
					value
				}
			}
		}
	}`, &readResp, client.Var("id", scene.ID))

	if err != nil {
		t.Fatalf("Scene query failed: %v", err)
	}

	if readResp.Scene.ID != scene.ID {
		t.Errorf("Expected scene ID %s, got %s", scene.ID, readResp.Scene.ID)
	}

	// Verify sparse channel format in response
	if len(readResp.Scene.FixtureValues) != 1 {
		t.Fatalf("Expected 1 fixture value, got %d", len(readResp.Scene.FixtureValues))
	}

	channels := readResp.Scene.FixtureValues[0].Channels
	if len(channels) != 2 {
		t.Errorf("Expected 2 sparse channels (only non-zero values), got %d", len(channels))
	}

	// Verify channel values
	expectedChannels := map[int]int{
		1: 100,
		3: 200,
	}

	for _, ch := range channels {
		expectedValue, exists := expectedChannels[ch.Offset]
		if !exists {
			t.Errorf("Unexpected channel offset %d in sparse response", ch.Offset)
		}
		if ch.Value != expectedValue {
			t.Errorf("Channel offset %d: expected value %d, got %d", ch.Offset, expectedValue, ch.Value)
		}
	}
}

// TestSparseChannels_UpdateScene tests updating a scene with sparse channels
func TestSparseChannels_UpdateScene(t *testing.T) {
	c, resolver, cleanup := testSetup(t)
	defer cleanup()

	// Create project, fixture, and initial scene
	project := &models.Project{
		ID:   "test-project-sparse-update",
		Name: "Test Project",
	}
	resolver.db.Create(project)

	fixtureDef := &models.FixtureDefinition{
		ID:           "test-fixture-def-sparse-update",
		Manufacturer: "Test",
		Model:        "TestPar3",
		Type:         "LED_PAR",
	}
	resolver.db.Create(fixtureDef)

	fixture := &models.FixtureInstance{
		ID:           "test-fixture-sparse-update",
		Name:         "Test Par 3ch",
		ProjectID:    project.ID,
		DefinitionID: fixtureDef.ID,
		Universe:     1,
		StartChannel: 1,
	}
	resolver.db.Create(fixture)

	scene := &models.Scene{
		ID:        "test-scene-sparse-update",
		Name:      "Original Scene",
		ProjectID: project.ID,
	}
	resolver.db.Create(scene)

	// Initial values: only offset 0 = 255 (sparse format)
	fixtureValue := &models.FixtureValue{
		ID:        "fv-sparse-update",
		SceneID:   scene.ID,
		FixtureID: fixture.ID,
		Channels:  `[{"offset":0,"value":255}]`,
	}
	resolver.db.Create(fixtureValue)

	// Update scene with sparse channels - change offset 0 to 128, add offset 2 to 64
	var updateResp struct {
		UpdateScene struct {
			ID   string `json:"id"`
			Name string `json:"name"`
		} `json:"updateScene"`
	}
	err := c.Post(`mutation($id: ID!, $fixtureId: ID!) {
		updateScene(id: $id, input: {
			name: "Updated Scene"
			fixtureValues: [
				{
					fixtureId: $fixtureId
					channels: [
						{ offset: 0, value: 128 }
						{ offset: 2, value: 64 }
					]
				}
			]
		}) {
			id
			name
		}
	}`, &updateResp,
		client.Var("id", scene.ID),
		client.Var("fixtureId", fixture.ID))

	if err != nil {
		t.Fatalf("UpdateScene mutation with sparse channels failed: %v", err)
	}

	if updateResp.UpdateScene.Name != "Updated Scene" {
		t.Errorf("Expected name 'Updated Scene', got '%s'", updateResp.UpdateScene.Name)
	}
}

// TestSparseChannels_AddFixturesToScene tests adding fixtures with sparse channels
func TestSparseChannels_AddFixturesToScene(t *testing.T) {
	c, resolver, cleanup := testSetup(t)
	defer cleanup()

	// Create project with two fixtures
	project := &models.Project{
		ID:   "test-project-sparse-add",
		Name: "Test Project",
	}
	resolver.db.Create(project)

	fixtureDef := &models.FixtureDefinition{
		ID:           "test-fixture-def-sparse-add",
		Manufacturer: "Test",
		Model:        "TestPar",
		Type:         "LED_PAR",
	}
	resolver.db.Create(fixtureDef)

	fixture1 := &models.FixtureInstance{
		ID:           "test-fixture-1-sparse-add",
		Name:         "Test Par 1",
		ProjectID:    project.ID,
		DefinitionID: fixtureDef.ID,
		Universe:     1,
		StartChannel: 1,
	}
	resolver.db.Create(fixture1)

	fixture2 := &models.FixtureInstance{
		ID:           "test-fixture-2-sparse-add",
		Name:         "Test Par 2",
		ProjectID:    project.ID,
		DefinitionID: fixtureDef.ID,
		Universe:     1,
		StartChannel: 10,
	}
	resolver.db.Create(fixture2)

	// Create scene with only fixture1
	scene := &models.Scene{
		ID:        "test-scene-sparse-add",
		Name:      "Test Scene",
		ProjectID: project.ID,
	}
	resolver.db.Create(scene)

	fixtureValue := &models.FixtureValue{
		ID:        "fv-1-sparse-add",
		SceneID:   scene.ID,
		FixtureID: fixture1.ID,
		Channels:  `[{"offset":0,"value":255}]`,
	}
	resolver.db.Create(fixtureValue)

	// Add fixture2 to scene with sparse channels
	var addResp struct {
		AddFixturesToScene struct {
			ID string `json:"id"`
		} `json:"addFixturesToScene"`
	}
	err := c.Post(`mutation($sceneId: ID!, $fixtureId: ID!) {
		addFixturesToScene(
			sceneId: $sceneId
			fixtureValues: [
				{
					fixtureId: $fixtureId
					channels: [
						{ offset: 1, value: 200 }
						{ offset: 2, value: 100 }
					]
				}
			]
		) {
			id
		}
	}`, &addResp,
		client.Var("sceneId", scene.ID),
		client.Var("fixtureId", fixture2.ID))

	if err != nil {
		t.Fatalf("AddFixturesToScene mutation with sparse channels failed: %v", err)
	}

	if addResp.AddFixturesToScene.ID != scene.ID {
		t.Errorf("Expected scene ID %s, got %s", scene.ID, addResp.AddFixturesToScene.ID)
	}
}

// TestSparseChannels_UpdateScenePartial tests partial scene updates with sparse channels
func TestSparseChannels_UpdateScenePartial(t *testing.T) {
	c, resolver, cleanup := testSetup(t)
	defer cleanup()

	// Create project and fixture
	project := &models.Project{
		ID:   "test-project-sparse-partial",
		Name: "Test Project",
	}
	resolver.db.Create(project)

	fixtureDef := &models.FixtureDefinition{
		ID:           "test-fixture-def-sparse-partial",
		Manufacturer: "Test",
		Model:        "TestPar",
		Type:         "LED_PAR",
	}
	resolver.db.Create(fixtureDef)

	fixture := &models.FixtureInstance{
		ID:           "test-fixture-sparse-partial",
		Name:         "Test Par",
		ProjectID:    project.ID,
		DefinitionID: fixtureDef.ID,
		Universe:     1,
		StartChannel: 1,
	}
	resolver.db.Create(fixture)

	// Create scene with initial values
	scene := &models.Scene{
		ID:        "test-scene-sparse-partial",
		Name:      "Original Name",
		ProjectID: project.ID,
	}
	resolver.db.Create(scene)

	// Initial sparse values: offset 0=255, offset 2=128
	fixtureValue := &models.FixtureValue{
		ID:        "fv-sparse-partial",
		SceneID:   scene.ID,
		FixtureID: fixture.ID,
		Channels:  `[{"offset":0,"value":255},{"offset":2,"value":128}]`,
	}
	resolver.db.Create(fixtureValue)

	// Partial update - only change offset 1, preserve others
	var updateResp struct {
		UpdateScenePartial struct {
			ID   string `json:"id"`
			Name string `json:"name"`
		} `json:"updateScenePartial"`
	}
	err := c.Post(`mutation($sceneId: ID!, $fixtureId: ID!) {
		updateScenePartial(
			sceneId: $sceneId
			name: "Updated Name"
			fixtureValues: [
				{
					fixtureId: $fixtureId
					channels: [
						{ offset: 1, value: 64 }
					]
				}
			]
			mergeFixtures: true
		) {
			id
			name
		}
	}`, &updateResp,
		client.Var("sceneId", scene.ID),
		client.Var("fixtureId", fixture.ID))

	if err != nil {
		t.Fatalf("UpdateScenePartial mutation with sparse channels failed: %v", err)
	}

	if updateResp.UpdateScenePartial.Name != "Updated Name" {
		t.Errorf("Expected name 'Updated Name', got '%s'", updateResp.UpdateScenePartial.Name)
	}
}

// TestSparseChannels_SceneActivation_OnlyAffectsSpecifiedChannels tests that
// when activating a scene with sparse channels, only the specified channels
// are modified on the DMX output. Other channels should remain at their
// previous values.
func TestSparseChannels_SceneActivation_OnlyAffectsSpecifiedChannels(t *testing.T) {
	c, resolver, cleanup := testSetup(t)
	defer cleanup()

	// Create project and fixture with 6 channels
	project := &models.Project{
		ID:   "test-project-sparse-dmx",
		Name: "Test Project",
	}
	resolver.db.Create(project)

	fixtureDef := &models.FixtureDefinition{
		ID:           "test-fixture-def-sparse-dmx",
		Manufacturer: "Test",
		Model:        "TestPar6",
		Type:         "LED_PAR",
	}
	resolver.db.Create(fixtureDef)

	fixture := &models.FixtureInstance{
		ID:           "test-fixture-sparse-dmx",
		Name:         "Test Par 6ch",
		ProjectID:    project.ID,
		DefinitionID: fixtureDef.ID,
		Universe:     1,
		StartChannel: 10, // Channels 10-15
	}
	resolver.db.Create(fixture)

	// Set initial DMX values for all channels
	for i := 10; i <= 15; i++ {
		resolver.DMXService.SetChannelValue(1, i, 50) // All start at 50
	}

	// Create scene with sparse channels - only set channels at offsets 1, 3, 5
	// (DMX channels 11, 13, 15)
	scene := &models.Scene{
		ID:        "test-scene-sparse-dmx",
		Name:      "Sparse DMX Scene",
		ProjectID: project.ID,
	}
	resolver.db.Create(scene)

	// Sparse values: offset 1=100, offset 3=150, offset 5=200
	// Using new sparse format - only store the channels we're setting
	fixtureValue := &models.FixtureValue{
		ID:        "fv-sparse-dmx",
		SceneID:   scene.ID,
		FixtureID: fixture.ID,
		// New sparse format: only specify channels we want to set
		Channels: `[{"offset":1,"value":100},{"offset":3,"value":150},{"offset":5,"value":200}]`,
	}
	resolver.db.Create(fixtureValue)

	// Activate the scene
	var resp struct {
		SetSceneLive bool `json:"setSceneLive"`
	}
	err := c.Post(`mutation($sceneId: ID!) {
		setSceneLive(sceneId: $sceneId)
	}`, &resp, client.Var("sceneId", scene.ID))

	if err != nil {
		t.Fatalf("SetSceneLive mutation failed: %v", err)
	}

	if !resp.SetSceneLive {
		t.Error("Expected setSceneLive to return true")
	}

	// Wait for scene to be applied (no fade, should be instant)
	time.Sleep(50 * time.Millisecond)

	// Verify DMX values:
	// - Channels NOT in sparse map should remain at 50 (offsets 0, 2, 4 = DMX 10, 12, 14)
	// - Channels IN sparse map should be updated (offsets 1, 3, 5 = DMX 11, 13, 15)

	// In the NEW implementation with sparse channels:
	// Channel 10 (offset 0): should remain 50 (not specified)
	// Channel 11 (offset 1): should be 100 (specified)
	// Channel 12 (offset 2): should remain 50 (not specified)
	// Channel 13 (offset 3): should be 150 (specified)
	// Channel 14 (offset 4): should remain 50 (not specified)
	// Channel 15 (offset 5): should be 200 (specified)

	// NOTE: This test will FAIL with current implementation because it sets
	// ALL channels (including 0 values). With sparse channels, only specified
	// channels should be modified.

	ch10 := resolver.DMXService.GetChannelValue(1, 10)
	ch11 := resolver.DMXService.GetChannelValue(1, 11)
	ch12 := resolver.DMXService.GetChannelValue(1, 12)
	ch13 := resolver.DMXService.GetChannelValue(1, 13)
	ch14 := resolver.DMXService.GetChannelValue(1, 14)
	ch15 := resolver.DMXService.GetChannelValue(1, 15)

	// Channels NOT in sparse map should remain unchanged at 50
	if ch10 != 50 {
		t.Errorf("Channel 10 (offset 0, not in sparse map): expected 50, got %d", ch10)
	}
	if ch12 != 50 {
		t.Errorf("Channel 12 (offset 2, not in sparse map): expected 50, got %d", ch12)
	}
	if ch14 != 50 {
		t.Errorf("Channel 14 (offset 4, not in sparse map): expected 50, got %d", ch14)
	}

	// Channels IN sparse map should be updated
	if ch11 != 100 {
		t.Errorf("Channel 11 (offset 1, in sparse map): expected 100, got %d", ch11)
	}
	if ch13 != 150 {
		t.Errorf("Channel 13 (offset 3, in sparse map): expected 150, got %d", ch13)
	}
	if ch15 != 200 {
		t.Errorf("Channel 15 (offset 5, in sparse map): expected 200, got %d", ch15)
	}
}

// TestSparseChannels_EmptyChannelsArray tests that a fixture can be included
// in a scene with an empty channels array, meaning the fixture is part of
// the scene but no channels are controlled (useful for scene templates or
// organizational purposes)
func TestSparseChannels_EmptyChannelsArray(t *testing.T) {
	c, resolver, cleanup := testSetup(t)
	defer cleanup()

	// Create project and fixture
	project := &models.Project{
		ID:   "test-project-sparse-empty",
		Name: "Test Project",
	}
	resolver.db.Create(project)

	fixtureDef := &models.FixtureDefinition{
		ID:           "test-fixture-def-sparse-empty",
		Manufacturer: "Test",
		Model:        "TestPar",
		Type:         "LED_PAR",
	}
	resolver.db.Create(fixtureDef)

	fixture := &models.FixtureInstance{
		ID:           "test-fixture-sparse-empty",
		Name:         "Test Par",
		ProjectID:    project.ID,
		DefinitionID: fixtureDef.ID,
		Universe:     1,
		StartChannel: 1,
	}
	resolver.db.Create(fixture)

	// Create scene with empty channels array
	var sceneResp struct {
		CreateScene struct {
			ID   string `json:"id"`
			Name string `json:"name"`
		} `json:"createScene"`
	}
	err := c.Post(`mutation($projectId: ID!, $fixtureId: ID!) {
		createScene(input: {
			name: "Empty Channels Scene"
			projectId: $projectId
			fixtureValues: [
				{
					fixtureId: $fixtureId
					channels: []
				}
			]
		}) {
			id
			name
		}
	}`, &sceneResp,
		client.Var("projectId", project.ID),
		client.Var("fixtureId", fixture.ID))

	if err != nil {
		t.Fatalf("CreateScene mutation with empty channels array failed: %v", err)
	}

	if sceneResp.CreateScene.ID == "" {
		t.Error("Expected scene ID to be set")
	}

	// Query the scene back
	var queryResp struct {
		Scene struct {
			FixtureValues []struct {
				Fixture struct {
					ID string `json:"id"`
				} `json:"fixture"`
				Channels []struct {
					Offset int `json:"offset"`
					Value  int `json:"value"`
				} `json:"channels"`
			} `json:"fixtureValues"`
		} `json:"scene"`
	}
	err = c.Post(`query($id: ID!) {
		scene(id: $id) {
			fixtureValues {
				fixture {
					id
				}
				channels {
					offset
					value
				}
			}
		}
	}`, &queryResp, client.Var("id", sceneResp.CreateScene.ID))

	if err != nil {
		t.Fatalf("Scene query failed: %v", err)
	}

	// Should have one fixture value with empty channels array
	if len(queryResp.Scene.FixtureValues) != 1 {
		t.Fatalf("Expected 1 fixture value, got %d", len(queryResp.Scene.FixtureValues))
	}

	if queryResp.Scene.FixtureValues[0].Fixture.ID != fixture.ID {
		t.Errorf("Expected fixture ID %s, got %s", fixture.ID, queryResp.Scene.FixtureValues[0].Fixture.ID)
	}

	if len(queryResp.Scene.FixtureValues[0].Channels) != 0 {
		t.Errorf("Expected 0 channels, got %d", len(queryResp.Scene.FixtureValues[0].Channels))
	}

	// Activate the scene - should not modify any DMX values
	resolver.DMXService.SetChannelValue(1, 1, 100)
	resolver.DMXService.SetChannelValue(1, 2, 150)

	var activateResp struct {
		SetSceneLive bool `json:"setSceneLive"`
	}
	err = c.Post(`mutation($sceneId: ID!) {
		setSceneLive(sceneId: $sceneId)
	}`, &activateResp, client.Var("sceneId", sceneResp.CreateScene.ID))

	if err != nil {
		t.Fatalf("SetSceneLive mutation failed: %v", err)
	}

	time.Sleep(50 * time.Millisecond)

	// DMX values should remain unchanged
	if resolver.DMXService.GetChannelValue(1, 1) != 100 {
		t.Errorf("Channel 1 should remain 100, got %d", resolver.DMXService.GetChannelValue(1, 1))
	}
	if resolver.DMXService.GetChannelValue(1, 2) != 150 {
		t.Errorf("Channel 2 should remain 150, got %d", resolver.DMXService.GetChannelValue(1, 2))
	}
}
