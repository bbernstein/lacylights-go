package resolvers

import (
	"testing"

	"github.com/99designs/gqlgen/client"
)

// =============================================================================
// Project CRUD Tests
// =============================================================================

func TestProject_Create(t *testing.T) {
	c, _, cleanup := testSetup(t)
	defer cleanup()

	var resp struct {
		CreateProject struct {
			ID          string  `json:"id"`
			Name        string  `json:"name"`
			Description *string `json:"description"`
		} `json:"createProject"`
	}

	err := c.Post(`mutation {
		createProject(input: { name: "Test Project", description: "A test project" }) {
			id
			name
			description
		}
	}`, &resp)

	if err != nil {
		t.Fatalf("CreateProject mutation failed: %v", err)
	}

	if resp.CreateProject.ID == "" {
		t.Error("Expected project ID to be set")
	}
	if resp.CreateProject.Name != "Test Project" {
		t.Errorf("Expected name 'Test Project', got '%s'", resp.CreateProject.Name)
	}
	if resp.CreateProject.Description == nil || *resp.CreateProject.Description != "A test project" {
		t.Error("Expected description to be 'A test project'")
	}
}

func TestProject_Read(t *testing.T) {
	c, _, cleanup := testSetup(t)
	defer cleanup()

	// First create a project
	var createResp struct {
		CreateProject struct {
			ID string `json:"id"`
		} `json:"createProject"`
	}
	err := c.Post(`mutation {
		createProject(input: { name: "Test Project" }) {
			id
		}
	}`, &createResp)
	if err != nil {
		t.Fatalf("CreateProject failed: %v", err)
	}

	// Then read it
	var readResp struct {
		Project struct {
			ID   string `json:"id"`
			Name string `json:"name"`
		} `json:"project"`
	}
	err = c.Post(`query($id: ID!) {
		project(id: $id) {
			id
			name
		}
	}`, &readResp, client.Var("id", createResp.CreateProject.ID))

	if err != nil {
		t.Fatalf("Project query failed: %v", err)
	}

	if readResp.Project.ID != createResp.CreateProject.ID {
		t.Errorf("Expected project ID %s, got %s", createResp.CreateProject.ID, readResp.Project.ID)
	}
}

func TestProject_Update(t *testing.T) {
	c, _, cleanup := testSetup(t)
	defer cleanup()

	// Create a project
	var createResp struct {
		CreateProject struct {
			ID string `json:"id"`
		} `json:"createProject"`
	}
	err := c.Post(`mutation {
		createProject(input: { name: "Original Name" }) {
			id
		}
	}`, &createResp)
	if err != nil {
		t.Fatalf("CreateProject failed: %v", err)
	}

	// Update it
	var updateResp struct {
		UpdateProject struct {
			ID   string `json:"id"`
			Name string `json:"name"`
		} `json:"updateProject"`
	}
	err = c.Post(`mutation($id: ID!) {
		updateProject(id: $id, input: { name: "Updated Name" }) {
			id
			name
		}
	}`, &updateResp, client.Var("id", createResp.CreateProject.ID))

	if err != nil {
		t.Fatalf("UpdateProject mutation failed: %v", err)
	}

	if updateResp.UpdateProject.Name != "Updated Name" {
		t.Errorf("Expected name 'Updated Name', got '%s'", updateResp.UpdateProject.Name)
	}
}

func TestProject_Delete(t *testing.T) {
	c, _, cleanup := testSetup(t)
	defer cleanup()

	// Create a project
	var createResp struct {
		CreateProject struct {
			ID string `json:"id"`
		} `json:"createProject"`
	}
	err := c.Post(`mutation {
		createProject(input: { name: "To Delete" }) {
			id
		}
	}`, &createResp)
	if err != nil {
		t.Fatalf("CreateProject failed: %v", err)
	}

	// Delete it
	var deleteResp struct {
		DeleteProject bool `json:"deleteProject"`
	}
	err = c.Post(`mutation($id: ID!) {
		deleteProject(id: $id)
	}`, &deleteResp, client.Var("id", createResp.CreateProject.ID))

	if err != nil {
		t.Fatalf("DeleteProject mutation failed: %v", err)
	}

	if !deleteResp.DeleteProject {
		t.Error("Expected deleteProject to return true")
	}

	// Verify it's gone
	var readResp struct {
		Project *struct {
			ID string `json:"id"`
		} `json:"project"`
	}
	err = c.Post(`query($id: ID!) {
		project(id: $id) {
			id
		}
	}`, &readResp, client.Var("id", createResp.CreateProject.ID))

	if err == nil && readResp.Project != nil {
		t.Error("Expected project to be deleted")
	}
}

func TestProject_List(t *testing.T) {
	c, _, cleanup := testSetup(t)
	defer cleanup()

	// Create multiple projects
	for i := 0; i < 3; i++ {
		var resp struct {
			CreateProject struct {
				ID string `json:"id"`
			} `json:"createProject"`
		}
		err := c.Post(`mutation($name: String!) {
			createProject(input: { name: $name }) {
				id
			}
		}`, &resp, client.Var("name", "Project "+string(rune('A'+i))))
		if err != nil {
			t.Fatalf("CreateProject failed: %v", err)
		}
	}

	// List projects
	var listResp struct {
		Projects []struct {
			ID   string `json:"id"`
			Name string `json:"name"`
		} `json:"projects"`
	}
	err := c.Post(`query {
		projects {
			id
			name
		}
	}`, &listResp)

	if err != nil {
		t.Fatalf("Projects query failed: %v", err)
	}

	if len(listResp.Projects) != 3 {
		t.Errorf("Expected 3 projects, got %d", len(listResp.Projects))
	}
}

// =============================================================================
// Fixture Definition CRUD Tests
// =============================================================================

func TestFixtureDefinition_Create(t *testing.T) {
	c, _, cleanup := testSetup(t)
	defer cleanup()

	var resp struct {
		CreateFixtureDefinition struct {
			ID           string `json:"id"`
			Manufacturer string `json:"manufacturer"`
			Model        string `json:"model"`
			Type         string `json:"type"`
		} `json:"createFixtureDefinition"`
	}

	err := c.Post(`mutation {
		createFixtureDefinition(input: {
			manufacturer: "Chauvet DJ"
			model: "SlimPAR Pro H USB"
			type: LED_PAR
			channels: [
				{ name: "Red", type: RED, offset: 0, minValue: 0, maxValue: 255, defaultValue: 0 }
				{ name: "Green", type: GREEN, offset: 1, minValue: 0, maxValue: 255, defaultValue: 0 }
				{ name: "Blue", type: BLUE, offset: 2, minValue: 0, maxValue: 255, defaultValue: 0 }
			]
		}) {
			id
			manufacturer
			model
			type
		}
	}`, &resp)

	if err != nil {
		t.Fatalf("CreateFixtureDefinition mutation failed: %v", err)
	}

	if resp.CreateFixtureDefinition.ID == "" {
		t.Error("Expected fixture definition ID to be set")
	}
	if resp.CreateFixtureDefinition.Manufacturer != "Chauvet DJ" {
		t.Errorf("Expected manufacturer 'Chauvet DJ', got '%s'", resp.CreateFixtureDefinition.Manufacturer)
	}
}

func TestFixtureDefinition_List(t *testing.T) {
	c, _, cleanup := testSetup(t)
	defer cleanup()

	// Create a definition
	var createResp struct {
		CreateFixtureDefinition struct {
			ID string `json:"id"`
		} `json:"createFixtureDefinition"`
	}
	err := c.Post(`mutation {
		createFixtureDefinition(input: {
			manufacturer: "Test"
			model: "TestPar"
			type: LED_PAR
			channels: [
				{ name: "Dimmer", type: INTENSITY, offset: 0, minValue: 0, maxValue: 255, defaultValue: 0 }
			]
		}) {
			id
		}
	}`, &createResp)
	if err != nil {
		t.Fatalf("CreateFixtureDefinition failed: %v", err)
	}

	// List definitions
	var listResp struct {
		FixtureDefinitions []struct {
			ID           string `json:"id"`
			Manufacturer string `json:"manufacturer"`
			Model        string `json:"model"`
		} `json:"fixtureDefinitions"`
	}
	err = c.Post(`query {
		fixtureDefinitions {
			id
			manufacturer
			model
		}
	}`, &listResp)

	if err != nil {
		t.Fatalf("FixtureDefinitions query failed: %v", err)
	}

	if len(listResp.FixtureDefinitions) < 1 {
		t.Error("Expected at least 1 fixture definition")
	}
}

func TestFixtureDefinition_FilterByManufacturer(t *testing.T) {
	c, _, cleanup := testSetup(t)
	defer cleanup()

	// Create definitions with different manufacturers
	manufacturers := []string{"Chauvet DJ", "Martin", "Robe"}
	for _, mfr := range manufacturers {
		var resp struct {
			CreateFixtureDefinition struct {
				ID string `json:"id"`
			} `json:"createFixtureDefinition"`
		}
		err := c.Post(`mutation($mfr: String!) {
			createFixtureDefinition(input: {
				manufacturer: $mfr
				model: "TestPar"
				type: LED_PAR
				channels: [
					{ name: "Dimmer", type: INTENSITY, offset: 0, minValue: 0, maxValue: 255, defaultValue: 0 }
				]
			}) {
				id
			}
		}`, &resp, client.Var("mfr", mfr))
		if err != nil {
			t.Fatalf("CreateFixtureDefinition failed: %v", err)
		}
	}

	// Filter by manufacturer (partial match)
	var filterResp struct {
		FixtureDefinitions []struct {
			ID           string `json:"id"`
			Manufacturer string `json:"manufacturer"`
		} `json:"fixtureDefinitions"`
	}
	err := c.Post(`query {
		fixtureDefinitions(filter: { manufacturer: "chau" }) {
			id
			manufacturer
		}
	}`, &filterResp)

	if err != nil {
		t.Fatalf("FixtureDefinitions filter query failed: %v", err)
	}

	if len(filterResp.FixtureDefinitions) != 1 {
		t.Errorf("Expected 1 Chauvet fixture, got %d", len(filterResp.FixtureDefinitions))
	}
	if len(filterResp.FixtureDefinitions) > 0 && filterResp.FixtureDefinitions[0].Manufacturer != "Chauvet DJ" {
		t.Errorf("Expected manufacturer 'Chauvet DJ', got '%s'", filterResp.FixtureDefinitions[0].Manufacturer)
	}
}

// =============================================================================
// Fixture Instance CRUD Tests
// =============================================================================

func TestFixtureInstance_Create(t *testing.T) {
	c, _, cleanup := testSetup(t)
	defer cleanup()

	// First create a project and fixture definition
	var projectResp struct {
		CreateProject struct {
			ID string `json:"id"`
		} `json:"createProject"`
	}
	err := c.Post(`mutation {
		createProject(input: { name: "Test Project" }) {
			id
		}
	}`, &projectResp)
	if err != nil {
		t.Fatalf("CreateProject failed: %v", err)
	}

	var defResp struct {
		CreateFixtureDefinition struct {
			ID string `json:"id"`
		} `json:"createFixtureDefinition"`
	}
	err = c.Post(`mutation {
		createFixtureDefinition(input: {
			manufacturer: "Test"
			model: "TestPar"
			type: LED_PAR
			channels: [
				{ name: "Red", type: RED, offset: 0, minValue: 0, maxValue: 255, defaultValue: 0 }
				{ name: "Green", type: GREEN, offset: 1, minValue: 0, maxValue: 255, defaultValue: 0 }
				{ name: "Blue", type: BLUE, offset: 2, minValue: 0, maxValue: 255, defaultValue: 0 }
			]
		}) {
			id
		}
	}`, &defResp)
	if err != nil {
		t.Fatalf("CreateFixtureDefinition failed: %v", err)
	}

	// Create fixture instance
	var instanceResp struct {
		CreateFixtureInstance struct {
			ID           string `json:"id"`
			Name         string `json:"name"`
			Universe     int    `json:"universe"`
			StartChannel int    `json:"startChannel"`
		} `json:"createFixtureInstance"`
	}
	err = c.Post(`mutation($projectId: ID!, $defId: ID!) {
		createFixtureInstance(input: {
			name: "Front Par 1"
			projectId: $projectId
			definitionId: $defId
			universe: 1
			startChannel: 1
			tags: ["front", "par"]
		}) {
			id
			name
			universe
			startChannel
		}
	}`, &instanceResp,
		client.Var("projectId", projectResp.CreateProject.ID),
		client.Var("defId", defResp.CreateFixtureDefinition.ID))

	if err != nil {
		t.Fatalf("CreateFixtureInstance mutation failed: %v", err)
	}

	if instanceResp.CreateFixtureInstance.ID == "" {
		t.Error("Expected fixture instance ID to be set")
	}
	if instanceResp.CreateFixtureInstance.Name != "Front Par 1" {
		t.Errorf("Expected name 'Front Par 1', got '%s'", instanceResp.CreateFixtureInstance.Name)
	}
	if instanceResp.CreateFixtureInstance.Universe != 1 {
		t.Errorf("Expected universe 1, got %d", instanceResp.CreateFixtureInstance.Universe)
	}
}

func TestFixtureInstance_Update(t *testing.T) {
	c, _, cleanup := testSetup(t)
	defer cleanup()

	// Create project, definition, and instance
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
			model: "TestPar"
			type: LED_PAR
			channels: [
				{ name: "Dimmer", type: INTENSITY, offset: 0, minValue: 0, maxValue: 255, defaultValue: 0 }
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
			name: "Original Name"
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

	// Update the fixture instance
	var updateResp struct {
		UpdateFixtureInstance struct {
			ID           string `json:"id"`
			Name         string `json:"name"`
			StartChannel int    `json:"startChannel"`
		} `json:"updateFixtureInstance"`
	}
	err = c.Post(`mutation($id: ID!) {
		updateFixtureInstance(id: $id, input: {
			name: "Updated Name"
			startChannel: 10
		}) {
			id
			name
			startChannel
		}
	}`, &updateResp, client.Var("id", instanceResp.CreateFixtureInstance.ID))

	if err != nil {
		t.Fatalf("UpdateFixtureInstance mutation failed: %v", err)
	}

	if updateResp.UpdateFixtureInstance.Name != "Updated Name" {
		t.Errorf("Expected name 'Updated Name', got '%s'", updateResp.UpdateFixtureInstance.Name)
	}
	if updateResp.UpdateFixtureInstance.StartChannel != 10 {
		t.Errorf("Expected startChannel 10, got %d", updateResp.UpdateFixtureInstance.StartChannel)
	}
}

func TestFixtureInstance_Delete(t *testing.T) {
	c, _, cleanup := testSetup(t)
	defer cleanup()

	// Create project, definition, and instance
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
			model: "TestPar"
			type: LED_PAR
			channels: [
				{ name: "Dimmer", type: INTENSITY, offset: 0, minValue: 0, maxValue: 255, defaultValue: 0 }
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
			name: "To Delete"
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

	// Delete it
	var deleteResp struct {
		DeleteFixtureInstance bool `json:"deleteFixtureInstance"`
	}
	err = c.Post(`mutation($id: ID!) {
		deleteFixtureInstance(id: $id)
	}`, &deleteResp, client.Var("id", instanceResp.CreateFixtureInstance.ID))

	if err != nil {
		t.Fatalf("DeleteFixtureInstance mutation failed: %v", err)
	}

	if !deleteResp.DeleteFixtureInstance {
		t.Error("Expected deleteFixtureInstance to return true")
	}
}

// =============================================================================
// Scene CRUD Tests
// =============================================================================

func TestScene_Create(t *testing.T) {
	c, _, cleanup := testSetup(t)
	defer cleanup()

	// Create project, definition, and fixture instance
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
			model: "TestPar"
			type: LED_PAR
			channels: [
				{ name: "Red", type: RED, offset: 0, minValue: 0, maxValue: 255, defaultValue: 0 }
				{ name: "Green", type: GREEN, offset: 1, minValue: 0, maxValue: 255, defaultValue: 0 }
				{ name: "Blue", type: BLUE, offset: 2, minValue: 0, maxValue: 255, defaultValue: 0 }
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
			name: "Test Par"
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

	// Create scene with fixture values
	var sceneResp struct {
		CreateScene struct {
			ID          string `json:"id"`
			Name        string `json:"name"`
			Description string `json:"description"`
		} `json:"createScene"`
	}
	err = c.Post(`mutation($projectId: ID!, $fixtureId: ID!) {
		createScene(input: {
			name: "Red Scene"
			description: "All fixtures red"
			projectId: $projectId
			fixtureValues: [
				{ fixtureId: $fixtureId, channelValues: [255, 0, 0] }
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
		t.Fatalf("CreateScene mutation failed: %v", err)
	}

	if sceneResp.CreateScene.ID == "" {
		t.Error("Expected scene ID to be set")
	}
	if sceneResp.CreateScene.Name != "Red Scene" {
		t.Errorf("Expected name 'Red Scene', got '%s'", sceneResp.CreateScene.Name)
	}
}

func TestScene_Read(t *testing.T) {
	c, _, cleanup := testSetup(t)
	defer cleanup()

	// Create project and scene (no fixtures needed for basic scene)
	var projectResp struct {
		CreateProject struct {
			ID string `json:"id"`
		} `json:"createProject"`
	}
	_ = c.Post(`mutation { createProject(input: { name: "Test Project" }) { id } }`, &projectResp)

	var createResp struct {
		CreateScene struct {
			ID string `json:"id"`
		} `json:"createScene"`
	}
	_ = c.Post(`mutation($projectId: ID!) {
		createScene(input: {
			name: "Test Scene"
			projectId: $projectId
			fixtureValues: []
		}) {
			id
		}
	}`, &createResp, client.Var("projectId", projectResp.CreateProject.ID))

	// Read the scene
	var readResp struct {
		Scene struct {
			ID   string `json:"id"`
			Name string `json:"name"`
		} `json:"scene"`
	}
	err := c.Post(`query($id: ID!) {
		scene(id: $id) {
			id
			name
		}
	}`, &readResp, client.Var("id", createResp.CreateScene.ID))

	if err != nil {
		t.Fatalf("Scene query failed: %v", err)
	}

	if readResp.Scene.ID != createResp.CreateScene.ID {
		t.Errorf("Expected scene ID %s, got %s", createResp.CreateScene.ID, readResp.Scene.ID)
	}
}

func TestScene_Update(t *testing.T) {
	c, _, cleanup := testSetup(t)
	defer cleanup()

	// Create project and scene
	var projectResp struct {
		CreateProject struct {
			ID string `json:"id"`
		} `json:"createProject"`
	}
	_ = c.Post(`mutation { createProject(input: { name: "Test Project" }) { id } }`, &projectResp)

	var createResp struct {
		CreateScene struct {
			ID string `json:"id"`
		} `json:"createScene"`
	}
	_ = c.Post(`mutation($projectId: ID!) {
		createScene(input: {
			name: "Original Name"
			projectId: $projectId
			fixtureValues: []
		}) {
			id
		}
	}`, &createResp, client.Var("projectId", projectResp.CreateProject.ID))

	// Update the scene
	var updateResp struct {
		UpdateScene struct {
			ID   string `json:"id"`
			Name string `json:"name"`
		} `json:"updateScene"`
	}
	err := c.Post(`mutation($id: ID!) {
		updateScene(id: $id, input: { name: "Updated Name" }) {
			id
			name
		}
	}`, &updateResp, client.Var("id", createResp.CreateScene.ID))

	if err != nil {
		t.Fatalf("UpdateScene mutation failed: %v", err)
	}

	if updateResp.UpdateScene.Name != "Updated Name" {
		t.Errorf("Expected name 'Updated Name', got '%s'", updateResp.UpdateScene.Name)
	}
}

func TestScene_Delete(t *testing.T) {
	c, _, cleanup := testSetup(t)
	defer cleanup()

	// Create project and scene
	var projectResp struct {
		CreateProject struct {
			ID string `json:"id"`
		} `json:"createProject"`
	}
	_ = c.Post(`mutation { createProject(input: { name: "Test Project" }) { id } }`, &projectResp)

	var createResp struct {
		CreateScene struct {
			ID string `json:"id"`
		} `json:"createScene"`
	}
	_ = c.Post(`mutation($projectId: ID!) {
		createScene(input: {
			name: "To Delete"
			projectId: $projectId
			fixtureValues: []
		}) {
			id
		}
	}`, &createResp, client.Var("projectId", projectResp.CreateProject.ID))

	// Delete the scene
	var deleteResp struct {
		DeleteScene bool `json:"deleteScene"`
	}
	err := c.Post(`mutation($id: ID!) {
		deleteScene(id: $id)
	}`, &deleteResp, client.Var("id", createResp.CreateScene.ID))

	if err != nil {
		t.Fatalf("DeleteScene mutation failed: %v", err)
	}

	if !deleteResp.DeleteScene {
		t.Error("Expected deleteScene to return true")
	}
}

// =============================================================================
// Cue List CRUD Tests
// =============================================================================

func TestCueList_Create(t *testing.T) {
	c, _, cleanup := testSetup(t)
	defer cleanup()

	// Create project
	var projectResp struct {
		CreateProject struct {
			ID string `json:"id"`
		} `json:"createProject"`
	}
	_ = c.Post(`mutation { createProject(input: { name: "Test Project" }) { id } }`, &projectResp)

	// Create cue list
	var cueListResp struct {
		CreateCueList struct {
			ID          string `json:"id"`
			Name        string `json:"name"`
			Description string `json:"description"`
			Loop        bool   `json:"loop"`
		} `json:"createCueList"`
	}
	err := c.Post(`mutation($projectId: ID!) {
		createCueList(input: {
			name: "Main Show"
			description: "Main show cue list"
			projectId: $projectId
			loop: false
		}) {
			id
			name
			description
			loop
		}
	}`, &cueListResp, client.Var("projectId", projectResp.CreateProject.ID))

	if err != nil {
		t.Fatalf("CreateCueList mutation failed: %v", err)
	}

	if cueListResp.CreateCueList.ID == "" {
		t.Error("Expected cue list ID to be set")
	}
	if cueListResp.CreateCueList.Name != "Main Show" {
		t.Errorf("Expected name 'Main Show', got '%s'", cueListResp.CreateCueList.Name)
	}
}

func TestCueList_Update(t *testing.T) {
	c, _, cleanup := testSetup(t)
	defer cleanup()

	// Create project and cue list
	var projectResp struct {
		CreateProject struct {
			ID string `json:"id"`
		} `json:"createProject"`
	}
	_ = c.Post(`mutation { createProject(input: { name: "Test Project" }) { id } }`, &projectResp)

	var createResp struct {
		CreateCueList struct {
			ID string `json:"id"`
		} `json:"createCueList"`
	}
	_ = c.Post(`mutation($projectId: ID!) {
		createCueList(input: {
			name: "Original Name"
			projectId: $projectId
		}) {
			id
		}
	}`, &createResp, client.Var("projectId", projectResp.CreateProject.ID))

	// Update cue list - note: updateCueList uses CreateCueListInput which requires projectId
	var updateResp struct {
		UpdateCueList struct {
			ID   string `json:"id"`
			Name string `json:"name"`
			Loop bool   `json:"loop"`
		} `json:"updateCueList"`
	}
	err := c.Post(`mutation($id: ID!, $projectId: ID!) {
		updateCueList(id: $id, input: { name: "Updated Name", projectId: $projectId, loop: true }) {
			id
			name
			loop
		}
	}`, &updateResp,
		client.Var("id", createResp.CreateCueList.ID),
		client.Var("projectId", projectResp.CreateProject.ID))

	if err != nil {
		t.Fatalf("UpdateCueList mutation failed: %v", err)
	}

	if updateResp.UpdateCueList.Name != "Updated Name" {
		t.Errorf("Expected name 'Updated Name', got '%s'", updateResp.UpdateCueList.Name)
	}
	if !updateResp.UpdateCueList.Loop {
		t.Error("Expected loop to be true")
	}
}

func TestCueList_Delete(t *testing.T) {
	c, _, cleanup := testSetup(t)
	defer cleanup()

	// Create project and cue list
	var projectResp struct {
		CreateProject struct {
			ID string `json:"id"`
		} `json:"createProject"`
	}
	_ = c.Post(`mutation { createProject(input: { name: "Test Project" }) { id } }`, &projectResp)

	var createResp struct {
		CreateCueList struct {
			ID string `json:"id"`
		} `json:"createCueList"`
	}
	_ = c.Post(`mutation($projectId: ID!) {
		createCueList(input: {
			name: "To Delete"
			projectId: $projectId
		}) {
			id
		}
	}`, &createResp, client.Var("projectId", projectResp.CreateProject.ID))

	// Delete cue list
	var deleteResp struct {
		DeleteCueList bool `json:"deleteCueList"`
	}
	err := c.Post(`mutation($id: ID!) {
		deleteCueList(id: $id)
	}`, &deleteResp, client.Var("id", createResp.CreateCueList.ID))

	if err != nil {
		t.Fatalf("DeleteCueList mutation failed: %v", err)
	}

	if !deleteResp.DeleteCueList {
		t.Error("Expected deleteCueList to return true")
	}
}

// =============================================================================
// Cue CRUD Tests
// =============================================================================

func TestCue_Create(t *testing.T) {
	c, _, cleanup := testSetup(t)
	defer cleanup()

	// Create project, scene, and cue list
	var projectResp struct {
		CreateProject struct {
			ID string `json:"id"`
		} `json:"createProject"`
	}
	_ = c.Post(`mutation { createProject(input: { name: "Test Project" }) { id } }`, &projectResp)

	var sceneResp struct {
		CreateScene struct {
			ID string `json:"id"`
		} `json:"createScene"`
	}
	_ = c.Post(`mutation($projectId: ID!) {
		createScene(input: {
			name: "Test Scene"
			projectId: $projectId
			fixtureValues: []
		}) {
			id
		}
	}`, &sceneResp, client.Var("projectId", projectResp.CreateProject.ID))

	var cueListResp struct {
		CreateCueList struct {
			ID string `json:"id"`
		} `json:"createCueList"`
	}
	_ = c.Post(`mutation($projectId: ID!) {
		createCueList(input: {
			name: "Test Cue List"
			projectId: $projectId
		}) {
			id
		}
	}`, &cueListResp, client.Var("projectId", projectResp.CreateProject.ID))

	// Create cue
	var cueResp struct {
		CreateCue struct {
			ID          string  `json:"id"`
			Name        string  `json:"name"`
			CueNumber   float64 `json:"cueNumber"`
			FadeInTime  float64 `json:"fadeInTime"`
			FadeOutTime float64 `json:"fadeOutTime"`
		} `json:"createCue"`
	}
	err := c.Post(`mutation($cueListId: ID!, $sceneId: ID!) {
		createCue(input: {
			name: "Cue 1"
			cueNumber: 1.0
			cueListId: $cueListId
			sceneId: $sceneId
			fadeInTime: 3.0
			fadeOutTime: 2.0
		}) {
			id
			name
			cueNumber
			fadeInTime
			fadeOutTime
		}
	}`, &cueResp,
		client.Var("cueListId", cueListResp.CreateCueList.ID),
		client.Var("sceneId", sceneResp.CreateScene.ID))

	if err != nil {
		t.Fatalf("CreateCue mutation failed: %v", err)
	}

	if cueResp.CreateCue.ID == "" {
		t.Error("Expected cue ID to be set")
	}
	if cueResp.CreateCue.Name != "Cue 1" {
		t.Errorf("Expected name 'Cue 1', got '%s'", cueResp.CreateCue.Name)
	}
	if cueResp.CreateCue.CueNumber != 1.0 {
		t.Errorf("Expected cueNumber 1.0, got %f", cueResp.CreateCue.CueNumber)
	}
}

func TestCue_Update(t *testing.T) {
	c, _, cleanup := testSetup(t)
	defer cleanup()

	// Create project, scene, cue list, and cue
	var projectResp struct {
		CreateProject struct {
			ID string `json:"id"`
		} `json:"createProject"`
	}
	_ = c.Post(`mutation { createProject(input: { name: "Test Project" }) { id } }`, &projectResp)

	var sceneResp struct {
		CreateScene struct {
			ID string `json:"id"`
		} `json:"createScene"`
	}
	_ = c.Post(`mutation($projectId: ID!) {
		createScene(input: {
			name: "Test Scene"
			projectId: $projectId
			fixtureValues: []
		}) {
			id
		}
	}`, &sceneResp, client.Var("projectId", projectResp.CreateProject.ID))

	var cueListResp struct {
		CreateCueList struct {
			ID string `json:"id"`
		} `json:"createCueList"`
	}
	_ = c.Post(`mutation($projectId: ID!) {
		createCueList(input: {
			name: "Test Cue List"
			projectId: $projectId
		}) {
			id
		}
	}`, &cueListResp, client.Var("projectId", projectResp.CreateProject.ID))

	var createResp struct {
		CreateCue struct {
			ID string `json:"id"`
		} `json:"createCue"`
	}
	_ = c.Post(`mutation($cueListId: ID!, $sceneId: ID!) {
		createCue(input: {
			name: "Original Name"
			cueNumber: 1.0
			cueListId: $cueListId
			sceneId: $sceneId
			fadeInTime: 3.0
			fadeOutTime: 2.0
		}) {
			id
		}
	}`, &createResp,
		client.Var("cueListId", cueListResp.CreateCueList.ID),
		client.Var("sceneId", sceneResp.CreateScene.ID))

	// Update the cue - note: updateCue uses CreateCueInput which requires all fields
	var updateResp struct {
		UpdateCue struct {
			ID         string  `json:"id"`
			Name       string  `json:"name"`
			FadeInTime float64 `json:"fadeInTime"`
		} `json:"updateCue"`
	}
	err := c.Post(`mutation($id: ID!, $cueListId: ID!, $sceneId: ID!) {
		updateCue(id: $id, input: {
			name: "Updated Name",
			cueNumber: 1.0,
			cueListId: $cueListId,
			sceneId: $sceneId,
			fadeInTime: 5.0,
			fadeOutTime: 2.0
		}) {
			id
			name
			fadeInTime
		}
	}`, &updateResp,
		client.Var("id", createResp.CreateCue.ID),
		client.Var("cueListId", cueListResp.CreateCueList.ID),
		client.Var("sceneId", sceneResp.CreateScene.ID))

	if err != nil {
		t.Fatalf("UpdateCue mutation failed: %v", err)
	}

	if updateResp.UpdateCue.Name != "Updated Name" {
		t.Errorf("Expected name 'Updated Name', got '%s'", updateResp.UpdateCue.Name)
	}
	if updateResp.UpdateCue.FadeInTime != 5.0 {
		t.Errorf("Expected fadeInTime 5.0, got %f", updateResp.UpdateCue.FadeInTime)
	}
}

func TestCue_Delete(t *testing.T) {
	c, _, cleanup := testSetup(t)
	defer cleanup()

	// Create project, scene, cue list, and cue
	var projectResp struct {
		CreateProject struct {
			ID string `json:"id"`
		} `json:"createProject"`
	}
	_ = c.Post(`mutation { createProject(input: { name: "Test Project" }) { id } }`, &projectResp)

	var sceneResp struct {
		CreateScene struct {
			ID string `json:"id"`
		} `json:"createScene"`
	}
	_ = c.Post(`mutation($projectId: ID!) {
		createScene(input: {
			name: "Test Scene"
			projectId: $projectId
			fixtureValues: []
		}) {
			id
		}
	}`, &sceneResp, client.Var("projectId", projectResp.CreateProject.ID))

	var cueListResp struct {
		CreateCueList struct {
			ID string `json:"id"`
		} `json:"createCueList"`
	}
	_ = c.Post(`mutation($projectId: ID!) {
		createCueList(input: {
			name: "Test Cue List"
			projectId: $projectId
		}) {
			id
		}
	}`, &cueListResp, client.Var("projectId", projectResp.CreateProject.ID))

	var createResp struct {
		CreateCue struct {
			ID string `json:"id"`
		} `json:"createCue"`
	}
	_ = c.Post(`mutation($cueListId: ID!, $sceneId: ID!) {
		createCue(input: {
			name: "To Delete"
			cueNumber: 1.0
			cueListId: $cueListId
			sceneId: $sceneId
			fadeInTime: 3.0
			fadeOutTime: 2.0
		}) {
			id
		}
	}`, &createResp,
		client.Var("cueListId", cueListResp.CreateCueList.ID),
		client.Var("sceneId", sceneResp.CreateScene.ID))

	// Delete the cue
	var deleteResp struct {
		DeleteCue bool `json:"deleteCue"`
	}
	err := c.Post(`mutation($id: ID!) {
		deleteCue(id: $id)
	}`, &deleteResp, client.Var("id", createResp.CreateCue.ID))

	if err != nil {
		t.Fatalf("DeleteCue mutation failed: %v", err)
	}

	if !deleteResp.DeleteCue {
		t.Error("Expected deleteCue to return true")
	}
}

// =============================================================================
// Scene Board CRUD Tests
// =============================================================================

func TestSceneBoard_Create(t *testing.T) {
	c, _, cleanup := testSetup(t)
	defer cleanup()

	// Create project
	var projectResp struct {
		CreateProject struct {
			ID string `json:"id"`
		} `json:"createProject"`
	}
	_ = c.Post(`mutation { createProject(input: { name: "Test Project" }) { id } }`, &projectResp)

	// Create scene board
	var boardResp struct {
		CreateSceneBoard struct {
			ID              string  `json:"id"`
			Name            string  `json:"name"`
			DefaultFadeTime float64 `json:"defaultFadeTime"`
		} `json:"createSceneBoard"`
	}
	err := c.Post(`mutation($projectId: ID!) {
		createSceneBoard(input: {
			name: "Main Board"
			description: "Main scene board"
			projectId: $projectId
			defaultFadeTime: 2.5
		}) {
			id
			name
			defaultFadeTime
		}
	}`, &boardResp, client.Var("projectId", projectResp.CreateProject.ID))

	if err != nil {
		t.Fatalf("CreateSceneBoard mutation failed: %v", err)
	}

	if boardResp.CreateSceneBoard.ID == "" {
		t.Error("Expected scene board ID to be set")
	}
	if boardResp.CreateSceneBoard.Name != "Main Board" {
		t.Errorf("Expected name 'Main Board', got '%s'", boardResp.CreateSceneBoard.Name)
	}
}

func TestSceneBoard_Update(t *testing.T) {
	c, _, cleanup := testSetup(t)
	defer cleanup()

	// Create project and scene board
	var projectResp struct {
		CreateProject struct {
			ID string `json:"id"`
		} `json:"createProject"`
	}
	_ = c.Post(`mutation { createProject(input: { name: "Test Project" }) { id } }`, &projectResp)

	var createResp struct {
		CreateSceneBoard struct {
			ID string `json:"id"`
		} `json:"createSceneBoard"`
	}
	_ = c.Post(`mutation($projectId: ID!) {
		createSceneBoard(input: {
			name: "Original Name"
			projectId: $projectId
		}) {
			id
		}
	}`, &createResp, client.Var("projectId", projectResp.CreateProject.ID))

	// Update scene board
	var updateResp struct {
		UpdateSceneBoard struct {
			ID              string  `json:"id"`
			Name            string  `json:"name"`
			DefaultFadeTime float64 `json:"defaultFadeTime"`
		} `json:"updateSceneBoard"`
	}
	err := c.Post(`mutation($id: ID!) {
		updateSceneBoard(id: $id, input: { name: "Updated Name", defaultFadeTime: 5.0 }) {
			id
			name
			defaultFadeTime
		}
	}`, &updateResp, client.Var("id", createResp.CreateSceneBoard.ID))

	if err != nil {
		t.Fatalf("UpdateSceneBoard mutation failed: %v", err)
	}

	if updateResp.UpdateSceneBoard.Name != "Updated Name" {
		t.Errorf("Expected name 'Updated Name', got '%s'", updateResp.UpdateSceneBoard.Name)
	}
}

func TestSceneBoard_Delete(t *testing.T) {
	c, _, cleanup := testSetup(t)
	defer cleanup()

	// Create project and scene board
	var projectResp struct {
		CreateProject struct {
			ID string `json:"id"`
		} `json:"createProject"`
	}
	_ = c.Post(`mutation { createProject(input: { name: "Test Project" }) { id } }`, &projectResp)

	var createResp struct {
		CreateSceneBoard struct {
			ID string `json:"id"`
		} `json:"createSceneBoard"`
	}
	_ = c.Post(`mutation($projectId: ID!) {
		createSceneBoard(input: {
			name: "To Delete"
			projectId: $projectId
		}) {
			id
		}
	}`, &createResp, client.Var("projectId", projectResp.CreateProject.ID))

	// Delete scene board
	var deleteResp struct {
		DeleteSceneBoard bool `json:"deleteSceneBoard"`
	}
	err := c.Post(`mutation($id: ID!) {
		deleteSceneBoard(id: $id)
	}`, &deleteResp, client.Var("id", createResp.CreateSceneBoard.ID))

	if err != nil {
		t.Fatalf("DeleteSceneBoard mutation failed: %v", err)
	}

	if !deleteResp.DeleteSceneBoard {
		t.Error("Expected deleteSceneBoard to return true")
	}
}

// =============================================================================
// Scene Board Button CRUD Tests
// =============================================================================

func TestSceneBoardButton_Create(t *testing.T) {
	c, _, cleanup := testSetup(t)
	defer cleanup()

	// Create project, scene, and scene board
	var projectResp struct {
		CreateProject struct {
			ID string `json:"id"`
		} `json:"createProject"`
	}
	_ = c.Post(`mutation { createProject(input: { name: "Test Project" }) { id } }`, &projectResp)

	var sceneResp struct {
		CreateScene struct {
			ID string `json:"id"`
		} `json:"createScene"`
	}
	_ = c.Post(`mutation($projectId: ID!) {
		createScene(input: {
			name: "Test Scene"
			projectId: $projectId
			fixtureValues: []
		}) {
			id
		}
	}`, &sceneResp, client.Var("projectId", projectResp.CreateProject.ID))

	var boardResp struct {
		CreateSceneBoard struct {
			ID string `json:"id"`
		} `json:"createSceneBoard"`
	}
	_ = c.Post(`mutation($projectId: ID!) {
		createSceneBoard(input: {
			name: "Test Board"
			projectId: $projectId
		}) {
			id
		}
	}`, &boardResp, client.Var("projectId", projectResp.CreateProject.ID))

	// Create button using addSceneToBoard mutation
	var buttonResp struct {
		AddSceneToBoard struct {
			ID      string `json:"id"`
			LayoutX int    `json:"layoutX"`
			LayoutY int    `json:"layoutY"`
			Label   string `json:"label"`
		} `json:"addSceneToBoard"`
	}
	err := c.Post(`mutation($boardId: ID!, $sceneId: ID!) {
		addSceneToBoard(input: {
			sceneBoardId: $boardId
			sceneId: $sceneId
			layoutX: 100
			layoutY: 200
			width: 150
			height: 100
			color: "#FF0000"
			label: "Red Scene"
		}) {
			id
			layoutX
			layoutY
			label
		}
	}`, &buttonResp,
		client.Var("boardId", boardResp.CreateSceneBoard.ID),
		client.Var("sceneId", sceneResp.CreateScene.ID))

	if err != nil {
		t.Fatalf("AddSceneToBoard mutation failed: %v", err)
	}

	if buttonResp.AddSceneToBoard.ID == "" {
		t.Error("Expected button ID to be set")
	}
	if buttonResp.AddSceneToBoard.LayoutX != 100 {
		t.Errorf("Expected layoutX 100, got %d", buttonResp.AddSceneToBoard.LayoutX)
	}
}

func TestSceneBoardButton_Delete(t *testing.T) {
	c, _, cleanup := testSetup(t)
	defer cleanup()

	// Create project, scene, scene board, and button
	var projectResp struct {
		CreateProject struct {
			ID string `json:"id"`
		} `json:"createProject"`
	}
	_ = c.Post(`mutation { createProject(input: { name: "Test Project" }) { id } }`, &projectResp)

	var sceneResp struct {
		CreateScene struct {
			ID string `json:"id"`
		} `json:"createScene"`
	}
	_ = c.Post(`mutation($projectId: ID!) {
		createScene(input: {
			name: "Test Scene"
			projectId: $projectId
			fixtureValues: []
		}) {
			id
		}
	}`, &sceneResp, client.Var("projectId", projectResp.CreateProject.ID))

	var boardResp struct {
		CreateSceneBoard struct {
			ID string `json:"id"`
		} `json:"createSceneBoard"`
	}
	_ = c.Post(`mutation($projectId: ID!) {
		createSceneBoard(input: {
			name: "Test Board"
			projectId: $projectId
		}) {
			id
		}
	}`, &boardResp, client.Var("projectId", projectResp.CreateProject.ID))

	// Create button using addSceneToBoard
	var buttonResp struct {
		AddSceneToBoard struct {
			ID string `json:"id"`
		} `json:"addSceneToBoard"`
	}
	_ = c.Post(`mutation($boardId: ID!, $sceneId: ID!) {
		addSceneToBoard(input: {
			sceneBoardId: $boardId
			sceneId: $sceneId
			layoutX: 0
			layoutY: 0
		}) {
			id
		}
	}`, &buttonResp,
		client.Var("boardId", boardResp.CreateSceneBoard.ID),
		client.Var("sceneId", sceneResp.CreateScene.ID))

	// Delete button using removeSceneFromBoard
	var deleteResp struct {
		RemoveSceneFromBoard bool `json:"removeSceneFromBoard"`
	}
	err := c.Post(`mutation($buttonId: ID!) {
		removeSceneFromBoard(buttonId: $buttonId)
	}`, &deleteResp, client.Var("buttonId", buttonResp.AddSceneToBoard.ID))

	if err != nil {
		t.Fatalf("RemoveSceneFromBoard mutation failed: %v", err)
	}

	if !deleteResp.RemoveSceneFromBoard {
		t.Error("Expected removeSceneFromBoard to return true")
	}
}
