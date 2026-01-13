package resolvers

import (
	"testing"
	"time"

	"github.com/99designs/gqlgen/client"

	"github.com/bbernstein/lacylights-go/internal/database/models"
)

// TestSparseChannels_CreateLook tests creating a look with sparse channel format
// instead of the full channel array. Only specified channels are stored.
func TestSparseChannels_CreateLook(t *testing.T) {
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

	// Create look with SPARSE channels format
	// Only setting Red=255, Blue=128, Dimmer=200 (offsets 0, 2, 4)
	var sceneResp struct {
		CreateLook struct {
			ID          string `json:"id"`
			Name        string `json:"name"`
			Description string `json:"description"`
		} `json:"createLook"`
	}
	err = c.Post(`mutation($projectId: ID!, $fixtureId: ID!) {
		createLook(input: {
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
		t.Fatalf("CreateLook mutation with sparse channels failed: %v", err)
	}

	if sceneResp.CreateLook.ID == "" {
		t.Error("Expected scene ID to be set")
	}
	if sceneResp.CreateLook.Name != "Sparse Channel Scene" {
		t.Errorf("Expected name 'Sparse Channel Scene', got '%s'", sceneResp.CreateLook.Name)
	}
}

// TestSparseChannels_QueryLook tests that querying a look returns sparse channels
func TestSparseChannels_QueryLook(t *testing.T) {
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

	// Create a look (in the new format, this will use sparse storage)
	scene := &models.Look{
		ID:        "test-scene-sparse-query",
		Name:      "Test Scene",
		ProjectID: project.ID,
	}
	resolver.db.Create(scene)

	// Create fixture values with sparse channels (offset 1=100, offset 3=200)
	// Using new sparse format - only store non-zero channels
	fixtureValue := &models.FixtureValue{
		ID:        "fv-sparse-query",
		LookID:   scene.ID,
		FixtureID: fixture.ID,
		// New sparse format: only specify non-zero channels
		Channels: `[{"offset":1,"value":100},{"offset":3,"value":200}]`,
	}
	resolver.db.Create(fixtureValue)

	// Query the look and expect sparse channels back
	var readResp struct {
		Look struct {
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
		} `json:"look"`
	}
	err := c.Post(`query($id: ID!) {
		look(id: $id) {
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
		t.Fatalf("Look query failed: %v", err)
	}

	if readResp.Look.ID != scene.ID {
		t.Errorf("Expected scene ID %s, got %s", scene.ID, readResp.Look.ID)
	}

	// Verify sparse channel format in response
	if len(readResp.Look.FixtureValues) != 1 {
		t.Fatalf("Expected 1 fixture value, got %d", len(readResp.Look.FixtureValues))
	}

	channels := readResp.Look.FixtureValues[0].Channels
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

// TestSparseChannels_UpdateLook tests updating a look with sparse channels
func TestSparseChannels_UpdateLook(t *testing.T) {
	c, resolver, cleanup := testSetup(t)
	defer cleanup()

	// Create project, fixture, and initial look
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

	scene := &models.Look{
		ID:        "test-scene-sparse-update",
		Name:      "Original Scene",
		ProjectID: project.ID,
	}
	resolver.db.Create(scene)

	// Initial values: only offset 0 = 255 (sparse format)
	fixtureValue := &models.FixtureValue{
		ID:        "fv-sparse-update",
		LookID:   scene.ID,
		FixtureID: fixture.ID,
		Channels:  `[{"offset":0,"value":255}]`,
	}
	resolver.db.Create(fixtureValue)

	// Update look with sparse channels - change offset 0 to 128, add offset 2 to 64
	var updateResp struct {
		UpdateLook struct {
			ID   string `json:"id"`
			Name string `json:"name"`
		} `json:"updateLook"`
	}
	err := c.Post(`mutation($id: ID!, $fixtureId: ID!) {
		updateLook(id: $id, input: {
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
		t.Fatalf("UpdateLook mutation with sparse channels failed: %v", err)
	}

	if updateResp.UpdateLook.Name != "Updated Scene" {
		t.Errorf("Expected name 'Updated Scene', got '%s'", updateResp.UpdateLook.Name)
	}
}

// TestSparseChannels_AddFixturesToLook tests adding fixtures with sparse channels
func TestSparseChannels_AddFixturesToLook(t *testing.T) {
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

	// Create look with only fixture1
	scene := &models.Look{
		ID:        "test-scene-sparse-add",
		Name:      "Test Scene",
		ProjectID: project.ID,
	}
	resolver.db.Create(scene)

	fixtureValue := &models.FixtureValue{
		ID:        "fv-1-sparse-add",
		LookID:   scene.ID,
		FixtureID: fixture1.ID,
		Channels:  `[{"offset":0,"value":255}]`,
	}
	resolver.db.Create(fixtureValue)

	// Add fixture2 to look with sparse channels
	var addResp struct {
		AddFixturesToLook struct {
			ID string `json:"id"`
		} `json:"addFixturesToLook"`
	}
	err := c.Post(`mutation($lookId: ID!, $fixtureId: ID!) {
		addFixturesToLook(
			lookId: $lookId
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
		client.Var("lookId", scene.ID),
		client.Var("fixtureId", fixture2.ID))

	if err != nil {
		t.Fatalf("AddFixturesToLook mutation with sparse channels failed: %v", err)
	}

	if addResp.AddFixturesToLook.ID != scene.ID {
		t.Errorf("Expected scene ID %s, got %s", scene.ID, addResp.AddFixturesToLook.ID)
	}
}

// TestSparseChannels_UpdateLookPartial tests partial look updates with sparse channels
func TestSparseChannels_UpdateLookPartial(t *testing.T) {
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

	// Create look with initial values
	scene := &models.Look{
		ID:        "test-scene-sparse-partial",
		Name:      "Original Name",
		ProjectID: project.ID,
	}
	resolver.db.Create(scene)

	// Initial sparse values: offset 0=255, offset 2=128
	fixtureValue := &models.FixtureValue{
		ID:        "fv-sparse-partial",
		LookID:   scene.ID,
		FixtureID: fixture.ID,
		Channels:  `[{"offset":0,"value":255},{"offset":2,"value":128}]`,
	}
	resolver.db.Create(fixtureValue)

	// Partial update - only change offset 1, preserve others
	var updateResp struct {
		UpdateLookPartial struct {
			ID   string `json:"id"`
			Name string `json:"name"`
		} `json:"updateLookPartial"`
	}
	err := c.Post(`mutation($lookId: ID!, $fixtureId: ID!) {
		updateLookPartial(
			lookId: $lookId
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
		client.Var("lookId", scene.ID),
		client.Var("fixtureId", fixture.ID))

	if err != nil {
		t.Fatalf("UpdateLookPartial mutation with sparse channels failed: %v", err)
	}

	if updateResp.UpdateLookPartial.Name != "Updated Name" {
		t.Errorf("Expected name 'Updated Name', got '%s'", updateResp.UpdateLookPartial.Name)
	}
}

// TestSparseChannels_LookActivation_OnlyAffectsSpecifiedChannels tests that
// when activating a look with sparse channels, only the specified channels
// are modified on the DMX output. Other channels should remain at their
// previous values.
func TestSparseChannels_LookActivation_OnlyAffectsSpecifiedChannels(t *testing.T) {
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

	// Create look with sparse channels - only set channels at offsets 1, 3, 5
	// (DMX channels 11, 13, 15)
	scene := &models.Look{
		ID:        "test-scene-sparse-dmx",
		Name:      "Sparse DMX Scene",
		ProjectID: project.ID,
	}
	resolver.db.Create(scene)

	// Sparse values: offset 1=100, offset 3=150, offset 5=200
	// Using new sparse format - only store the channels we're setting
	fixtureValue := &models.FixtureValue{
		ID:        "fv-sparse-dmx",
		LookID:   scene.ID,
		FixtureID: fixture.ID,
		// New sparse format: only specify channels we want to set
		Channels: `[{"offset":1,"value":100},{"offset":3,"value":150},{"offset":5,"value":200}]`,
	}
	resolver.db.Create(fixtureValue)

	// Activate the look
	var resp struct {
		SetLookLive bool `json:"setLookLive"`
	}
	err := c.Post(`mutation($lookId: ID!) {
		setLookLive(lookId: $lookId)
	}`, &resp, client.Var("lookId", scene.ID))

	if err != nil {
		t.Fatalf("SetLookLive mutation failed: %v", err)
	}

	if !resp.SetLookLive {
		t.Error("Expected setLookLive to return true")
	}

	// Wait for look to be applied (no fade, should be instant)
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
// in a look with an empty channels array, meaning the fixture is part of
// the look but no channels are controlled (useful for look templates or
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

	// Create look with empty channels array
	var sceneResp struct {
		CreateLook struct {
			ID   string `json:"id"`
			Name string `json:"name"`
		} `json:"createLook"`
	}
	err := c.Post(`mutation($projectId: ID!, $fixtureId: ID!) {
		createLook(input: {
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
		t.Fatalf("CreateLook mutation with empty channels array failed: %v", err)
	}

	if sceneResp.CreateLook.ID == "" {
		t.Error("Expected scene ID to be set")
	}

	// Query the look back
	var queryResp struct {
		Look struct {
			FixtureValues []struct {
				Fixture struct {
					ID string `json:"id"`
				} `json:"fixture"`
				Channels []struct {
					Offset int `json:"offset"`
					Value  int `json:"value"`
				} `json:"channels"`
			} `json:"fixtureValues"`
		} `json:"look"`
	}
	err = c.Post(`query($id: ID!) {
		look(id: $id) {
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
	}`, &queryResp, client.Var("id", sceneResp.CreateLook.ID))

	if err != nil {
		t.Fatalf("Look query failed: %v", err)
	}

	// Should have one fixture value with empty channels array
	if len(queryResp.Look.FixtureValues) != 1 {
		t.Fatalf("Expected 1 fixture value, got %d", len(queryResp.Look.FixtureValues))
	}

	if queryResp.Look.FixtureValues[0].Fixture.ID != fixture.ID {
		t.Errorf("Expected fixture ID %s, got %s", fixture.ID, queryResp.Look.FixtureValues[0].Fixture.ID)
	}

	if len(queryResp.Look.FixtureValues[0].Channels) != 0 {
		t.Errorf("Expected 0 channels, got %d", len(queryResp.Look.FixtureValues[0].Channels))
	}

	// Activate the look - should not modify any DMX values
	resolver.DMXService.SetChannelValue(1, 1, 100)
	resolver.DMXService.SetChannelValue(1, 2, 150)

	var activateResp struct {
		SetLookLive bool `json:"setLookLive"`
	}
	err = c.Post(`mutation($lookId: ID!) {
		setLookLive(lookId: $lookId)
	}`, &activateResp, client.Var("lookId", sceneResp.CreateLook.ID))

	if err != nil {
		t.Fatalf("SetLookLive mutation failed: %v", err)
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

// TestSparseChannels_BulkUpdateLooksPartial tests bulk partial look updates with sparse channels
func TestSparseChannels_BulkUpdateLooksPartial(t *testing.T) {
	c, resolver, cleanup := testSetup(t)
	defer cleanup()

	// Create project and fixture
	project := &models.Project{
		ID:   "test-project-bulk-partial",
		Name: "Test Project",
	}
	resolver.db.Create(project)

	fixtureDef := &models.FixtureDefinition{
		ID:           "test-fixture-def-bulk-partial",
		Manufacturer: "Test",
		Model:        "TestPar",
		Type:         "LED_PAR",
	}
	resolver.db.Create(fixtureDef)

	fixture := &models.FixtureInstance{
		ID:           "test-fixture-bulk-partial",
		Name:         "Test Par",
		ProjectID:    project.ID,
		DefinitionID: fixtureDef.ID,
		Universe:     1,
		StartChannel: 1,
	}
	resolver.db.Create(fixture)

	// Create two looks with initial values
	scene1 := &models.Look{
		ID:        "test-scene-bulk-partial-1",
		Name:      "Scene 1",
		ProjectID: project.ID,
	}
	scene2 := &models.Look{
		ID:        "test-scene-bulk-partial-2",
		Name:      "Scene 2",
		ProjectID: project.ID,
	}
	resolver.db.Create(scene1)
	resolver.db.Create(scene2)

	// Initial fixture values for both looks: offset 0=255, offset 1=128
	fixtureValue1 := &models.FixtureValue{
		ID:        "fv-bulk-partial-1",
		LookID:   scene1.ID,
		FixtureID: fixture.ID,
		Channels:  `[{"offset":0,"value":255},{"offset":1,"value":128}]`,
	}
	fixtureValue2 := &models.FixtureValue{
		ID:        "fv-bulk-partial-2",
		LookID:   scene2.ID,
		FixtureID: fixture.ID,
		Channels:  `[{"offset":0,"value":255},{"offset":1,"value":128}]`,
	}
	resolver.db.Create(fixtureValue1)
	resolver.db.Create(fixtureValue2)

	// Bulk update - change offset 1 value to 64 in both looks
	var updateResp struct {
		BulkUpdateLooksPartial []struct {
			ID   string `json:"id"`
			Name string `json:"name"`
		} `json:"bulkUpdateLooksPartial"`
	}
	err := c.Post(`mutation($look1Id: ID!, $look2Id: ID!, $fixtureId: ID!) {
		bulkUpdateLooksPartial(input: {
			looks: [
				{
					lookId: $look1Id
					name: "Scene 1 Updated"
					fixtureValues: [
						{
							fixtureId: $fixtureId
							channels: [
								{ offset: 1, value: 64 }
							]
						}
					]
				}
				{
					lookId: $look2Id
					name: "Scene 2 Updated"
					fixtureValues: [
						{
							fixtureId: $fixtureId
							channels: [
								{ offset: 1, value: 32 }
							]
						}
					]
				}
			]
		}) {
			id
			name
		}
	}`, &updateResp,
		client.Var("look1Id", scene1.ID),
		client.Var("look2Id", scene2.ID),
		client.Var("fixtureId", fixture.ID))

	if err != nil {
		t.Fatalf("BulkUpdateLooksPartial mutation failed: %v", err)
	}

	// Verify both looks were updated
	if len(updateResp.BulkUpdateLooksPartial) != 2 {
		t.Fatalf("Expected 2 looks updated, got %d", len(updateResp.BulkUpdateLooksPartial))
	}

	if updateResp.BulkUpdateLooksPartial[0].Name != "Scene 1 Updated" {
		t.Errorf("Expected 'Scene 1 Updated', got '%s'", updateResp.BulkUpdateLooksPartial[0].Name)
	}
	if updateResp.BulkUpdateLooksPartial[1].Name != "Scene 2 Updated" {
		t.Errorf("Expected 'Scene 2 Updated', got '%s'", updateResp.BulkUpdateLooksPartial[1].Name)
	}

	// Verify fixture values were updated (channels array is replaced, not merged at channel level)
	// mergeFixtures=true means: keep fixtures not mentioned, update fixtures that are mentioned
	// When updating a fixture, the channels array provided replaces the old one
	var updatedFV1 models.FixtureValue
	resolver.db.First(&updatedFV1, "look_id = ? AND fixture_id = ?", scene1.ID, fixture.ID)

	// Scene 1: channels should be the new value [offset 1 = 64]
	expectedChannels1 := `[{"offset":1,"value":64}]`
	if !sparseChannelsEqual(updatedFV1.Channels, expectedChannels1) {
		t.Errorf("Scene 1 channels should be %s, got %s", expectedChannels1, updatedFV1.Channels)
	}

	var updatedFV2 models.FixtureValue
	resolver.db.First(&updatedFV2, "look_id = ? AND fixture_id = ?", scene2.ID, fixture.ID)

	// Scene 2: channels should be the new value [offset 1 = 32]
	expectedChannels2 := `[{"offset":1,"value":32}]`
	if !sparseChannelsEqual(updatedFV2.Channels, expectedChannels2) {
		t.Errorf("Scene 2 channels should be %s, got %s", expectedChannels2, updatedFV2.Channels)
	}
}

// TestSparseChannels_BulkUpdateLooksPartial_MergeFixturesFalse tests bulk partial look updates with replace mode
func TestSparseChannels_BulkUpdateLooksPartial_MergeFixturesFalse(t *testing.T) {
	c, resolver, cleanup := testSetup(t)
	defer cleanup()

	// Create project and fixtures
	project := &models.Project{
		ID:   "test-project-bulk-replace",
		Name: "Test Project",
	}
	resolver.db.Create(project)

	fixtureDef := &models.FixtureDefinition{
		ID:           "test-fixture-def-bulk-replace",
		Manufacturer: "Test",
		Model:        "TestPar",
		Type:         "LED_PAR",
	}
	resolver.db.Create(fixtureDef)

	fixture1 := &models.FixtureInstance{
		ID:           "test-fixture-bulk-replace-1",
		Name:         "Test Par 1",
		ProjectID:    project.ID,
		DefinitionID: fixtureDef.ID,
		Universe:     1,
		StartChannel: 1,
	}
	fixture2 := &models.FixtureInstance{
		ID:           "test-fixture-bulk-replace-2",
		Name:         "Test Par 2",
		ProjectID:    project.ID,
		DefinitionID: fixtureDef.ID,
		Universe:     1,
		StartChannel: 10,
	}
	resolver.db.Create(fixture1)
	resolver.db.Create(fixture2)

	// Create look with two fixtures
	scene := &models.Look{
		ID:        "test-scene-bulk-replace",
		Name:      "Original Scene",
		ProjectID: project.ID,
	}
	resolver.db.Create(scene)

	// Both fixtures have values
	fv1 := &models.FixtureValue{
		ID:        "fv-bulk-replace-1",
		LookID:   scene.ID,
		FixtureID: fixture1.ID,
		Channels:  `[{"offset":0,"value":255}]`,
	}
	fv2 := &models.FixtureValue{
		ID:        "fv-bulk-replace-2",
		LookID:   scene.ID,
		FixtureID: fixture2.ID,
		Channels:  `[{"offset":0,"value":128}]`,
	}
	resolver.db.Create(fv1)
	resolver.db.Create(fv2)

	// Bulk update with mergeFixtures=false - should replace all fixtures with only fixture1
	var updateResp struct {
		BulkUpdateLooksPartial []struct {
			ID   string `json:"id"`
			Name string `json:"name"`
		} `json:"bulkUpdateLooksPartial"`
	}
	err := c.Post(`mutation($lookId: ID!, $fixtureId: ID!) {
		bulkUpdateLooksPartial(input: {
			looks: [
				{
					lookId: $lookId
					fixtureValues: [
						{
							fixtureId: $fixtureId
							channels: [
								{ offset: 0, value: 100 }
							]
						}
					]
					mergeFixtures: false
				}
			]
		}) {
			id
			name
		}
	}`, &updateResp,
		client.Var("lookId", scene.ID),
		client.Var("fixtureId", fixture1.ID))

	if err != nil {
		t.Fatalf("BulkUpdateLooksPartial with mergeFixtures=false failed: %v", err)
	}

	// Verify fixture2 was removed (mergeFixtures=false deletes all and replaces)
	var count int64
	resolver.db.Model(&models.FixtureValue{}).Where("look_id = ?", scene.ID).Count(&count)
	if count != 1 {
		t.Errorf("Expected 1 fixture value (fixture2 should be removed), got %d", count)
	}

	// Verify fixture1 has the new value
	var updatedFV models.FixtureValue
	resolver.db.First(&updatedFV, "look_id = ? AND fixture_id = ?", scene.ID, fixture1.ID)

	expectedChannels := `[{"offset":0,"value":100}]`
	if !sparseChannelsEqual(updatedFV.Channels, expectedChannels) {
		t.Errorf("Fixture 1 channels should be %s, got %s", expectedChannels, updatedFV.Channels)
	}
}
