package resolvers

import (
	"fmt"
	"strings"
	"testing"

	"github.com/99designs/gqlgen/client"

	"github.com/bbernstein/lacylights-go/internal/services/version"
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

func TestProject_CreateWithCanvasDimensions(t *testing.T) {
	c, _, cleanup := testSetup(t)
	defer cleanup()

	// Test with default canvas dimensions
	var defaultResp struct {
		CreateProject struct {
			ID                 string `json:"id"`
			Name               string `json:"name"`
			LayoutCanvasWidth  int    `json:"layoutCanvasWidth"`
			LayoutCanvasHeight int    `json:"layoutCanvasHeight"`
		} `json:"createProject"`
	}

	err := c.Post(`mutation {
		createProject(input: { name: "Default Canvas Project" }) {
			id
			name
			layoutCanvasWidth
			layoutCanvasHeight
		}
	}`, &defaultResp)

	if err != nil {
		t.Fatalf("CreateProject mutation failed: %v", err)
	}

	if defaultResp.CreateProject.LayoutCanvasWidth != 2000 {
		t.Errorf("Expected default layoutCanvasWidth 2000, got %d", defaultResp.CreateProject.LayoutCanvasWidth)
	}
	if defaultResp.CreateProject.LayoutCanvasHeight != 2000 {
		t.Errorf("Expected default layoutCanvasHeight 2000, got %d", defaultResp.CreateProject.LayoutCanvasHeight)
	}

	// Test with custom canvas dimensions
	var customResp struct {
		CreateProject struct {
			ID                 string `json:"id"`
			Name               string `json:"name"`
			LayoutCanvasWidth  int    `json:"layoutCanvasWidth"`
			LayoutCanvasHeight int    `json:"layoutCanvasHeight"`
		} `json:"createProject"`
	}

	err = c.Post(`mutation {
		createProject(input: { name: "Custom Canvas Project", layoutCanvasWidth: 4000, layoutCanvasHeight: 3000 }) {
			id
			name
			layoutCanvasWidth
			layoutCanvasHeight
		}
	}`, &customResp)

	if err != nil {
		t.Fatalf("CreateProject with custom canvas failed: %v", err)
	}

	if customResp.CreateProject.LayoutCanvasWidth != 4000 {
		t.Errorf("Expected layoutCanvasWidth 4000, got %d", customResp.CreateProject.LayoutCanvasWidth)
	}
	if customResp.CreateProject.LayoutCanvasHeight != 3000 {
		t.Errorf("Expected layoutCanvasHeight 3000, got %d", customResp.CreateProject.LayoutCanvasHeight)
	}
}

func TestProject_CreateWithInvalidCanvasDimensions(t *testing.T) {
	c, _, cleanup := testSetup(t)
	defer cleanup()

	testCases := []struct {
		name          string
		width         int
		height        int
		expectedError string
	}{
		{"too small width", 50, 2000, "layoutCanvasWidth must be at least 100"},
		{"too small height", 2000, 50, "layoutCanvasHeight must be at least 100"},
		{"zero width", 0, 2000, "layoutCanvasWidth must be at least 100"},
		{"negative width", -100, 2000, "layoutCanvasWidth must be at least 100"},
		{"too large width", 200000, 2000, "layoutCanvasWidth must be at most 100000"},
		{"too large height", 2000, 200000, "layoutCanvasHeight must be at most 100000"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			var resp struct {
				CreateProject struct {
					ID string `json:"id"`
				} `json:"createProject"`
			}

			query := fmt.Sprintf(`mutation {
				createProject(input: { name: "Test Project", layoutCanvasWidth: %d, layoutCanvasHeight: %d }) {
					id
				}
			}`, tc.width, tc.height)

			err := c.Post(query, &resp)

			if err == nil {
				t.Errorf("Expected error for %s, got nil", tc.name)
			} else if !strings.Contains(err.Error(), tc.expectedError) {
				t.Errorf("Expected error containing '%s', got: %v", tc.expectedError, err)
			}
		})
	}
}

func TestProject_UpdateCanvasDimensions(t *testing.T) {
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
		t.Fatalf("Failed to create project: %v", err)
	}

	projectID := createResp.CreateProject.ID

	// Update canvas dimensions
	var updateResp struct {
		UpdateProject struct {
			ID                 string `json:"id"`
			LayoutCanvasWidth  int    `json:"layoutCanvasWidth"`
			LayoutCanvasHeight int    `json:"layoutCanvasHeight"`
		} `json:"updateProject"`
	}

	query := fmt.Sprintf(`mutation {
		updateProject(id: "%s", input: { name: "Test Project", layoutCanvasWidth: 5000, layoutCanvasHeight: 4000 }) {
			id
			layoutCanvasWidth
			layoutCanvasHeight
		}
	}`, projectID)

	err = c.Post(query, &updateResp)

	if err != nil {
		t.Fatalf("Failed to update project: %v", err)
	}

	if updateResp.UpdateProject.LayoutCanvasWidth != 5000 {
		t.Errorf("Expected layoutCanvasWidth 5000, got %d", updateResp.UpdateProject.LayoutCanvasWidth)
	}
	if updateResp.UpdateProject.LayoutCanvasHeight != 4000 {
		t.Errorf("Expected layoutCanvasHeight 4000, got %d", updateResp.UpdateProject.LayoutCanvasHeight)
	}
}

func TestProject_UpdateWithInvalidCanvasDimensions(t *testing.T) {
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
		t.Fatalf("Failed to create project: %v", err)
	}

	projectID := createResp.CreateProject.ID

	// Try to update with invalid dimensions
	var updateResp struct {
		UpdateProject struct {
			ID string `json:"id"`
		} `json:"updateProject"`
	}

	query := fmt.Sprintf(`mutation {
		updateProject(id: "%s", input: { name: "Test Project", layoutCanvasWidth: 50 }) {
			id
		}
	}`, projectID)

	err = c.Post(query, &updateResp)

	if err == nil {
		t.Error("Expected error for invalid canvas width, got nil")
	} else if !strings.Contains(err.Error(), "layoutCanvasWidth must be at least 100") {
		t.Errorf("Expected validation error, got: %v", err)
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

func TestProject_BulkUpdateCanvasDimensions(t *testing.T) {
	c, _, cleanup := testSetup(t)
	defer cleanup()

	// Create two projects
	var createResp1, createResp2 struct {
		CreateProject struct {
			ID string `json:"id"`
		} `json:"createProject"`
	}

	err := c.Post(`mutation {
		createProject(input: { name: "Project 1" }) {
			id
		}
	}`, &createResp1)
	if err != nil {
		t.Fatalf("Failed to create project 1: %v", err)
	}

	err = c.Post(`mutation {
		createProject(input: { name: "Project 2" }) {
			id
		}
	}`, &createResp2)
	if err != nil {
		t.Fatalf("Failed to create project 2: %v", err)
	}

	// Bulk update canvas dimensions
	var updateResp struct {
		BulkUpdateProjects []struct {
			ID                 string `json:"id"`
			LayoutCanvasWidth  int    `json:"layoutCanvasWidth"`
			LayoutCanvasHeight int    `json:"layoutCanvasHeight"`
		} `json:"bulkUpdateProjects"`
	}

	query := fmt.Sprintf(`mutation {
		bulkUpdateProjects(input: {
			projects: [
				{ projectId: "%s", layoutCanvasWidth: 3000, layoutCanvasHeight: 2500 }
				{ projectId: "%s", layoutCanvasWidth: 4000, layoutCanvasHeight: 3500 }
			]
		}) {
			id
			layoutCanvasWidth
			layoutCanvasHeight
		}
	}`, createResp1.CreateProject.ID, createResp2.CreateProject.ID)

	err = c.Post(query, &updateResp)

	if err != nil {
		t.Fatalf("BulkUpdateProjects failed: %v", err)
	}

	if len(updateResp.BulkUpdateProjects) != 2 {
		t.Fatalf("Expected 2 projects, got %d", len(updateResp.BulkUpdateProjects))
	}

	// Check first project
	found1 := false
	for _, p := range updateResp.BulkUpdateProjects {
		if p.ID == createResp1.CreateProject.ID {
			found1 = true
			if p.LayoutCanvasWidth != 3000 {
				t.Errorf("Project 1: expected layoutCanvasWidth 3000, got %d", p.LayoutCanvasWidth)
			}
			if p.LayoutCanvasHeight != 2500 {
				t.Errorf("Project 1: expected layoutCanvasHeight 2500, got %d", p.LayoutCanvasHeight)
			}
		}
	}
	if !found1 {
		t.Error("Project 1 not found in bulk update response")
	}
}

func TestProject_BulkUpdateWithInvalidCanvasDimensions(t *testing.T) {
	c, _, cleanup := testSetup(t)
	defer cleanup()

	// Create a project
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
		t.Fatalf("Failed to create project: %v", err)
	}

	// Try bulk update with invalid dimensions
	var updateResp struct {
		BulkUpdateProjects []struct {
			ID string `json:"id"`
		} `json:"bulkUpdateProjects"`
	}

	query := fmt.Sprintf(`mutation {
		bulkUpdateProjects(input: {
			projects: [
				{ projectId: "%s", layoutCanvasWidth: 50 }
			]
		}) {
			id
		}
	}`, createResp.CreateProject.ID)

	err = c.Post(query, &updateResp)

	if err == nil {
		t.Error("Expected error for invalid canvas width, got nil")
	} else if !strings.Contains(err.Error(), "layoutCanvasWidth must be at least 100") {
		t.Errorf("Expected validation error, got: %v", err)
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

func TestFixtureInstance_DeleteCascadesFixtureValues(t *testing.T) {
	c, _, cleanup := testSetup(t)
	defer cleanup()

	// Create project
	var projectResp struct {
		CreateProject struct {
			ID string `json:"id"`
		} `json:"createProject"`
	}
	err := c.Post(`mutation { createProject(input: { name: "Cascade Test Project" }) { id } }`, &projectResp)
	if err != nil {
		t.Fatalf("CreateProject mutation failed: %v", err)
	}
	projectID := projectResp.CreateProject.ID

	// Create fixture definition
	var defResp struct {
		CreateFixtureDefinition struct {
			ID string `json:"id"`
		} `json:"createFixtureDefinition"`
	}
	err = c.Post(`mutation {
		createFixtureDefinition(input: {
			manufacturer: "Test"
			model: "CascadeTest"
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
	defID := defResp.CreateFixtureDefinition.ID

	// Create fixture instance
	var instanceResp struct {
		CreateFixtureInstance struct {
			ID string `json:"id"`
		} `json:"createFixtureInstance"`
	}
	err = c.Post(`mutation($projectId: ID!, $defId: ID!) {
		createFixtureInstance(input: {
			name: "Fixture With Values"
			projectId: $projectId
			definitionId: $defId
			universe: 1
			startChannel: 1
		}) {
			id
		}
	}`, &instanceResp,
		client.Var("projectId", projectID),
		client.Var("defId", defID))
	if err != nil {
		t.Fatalf("CreateFixtureInstance mutation failed: %v", err)
	}
	fixtureID := instanceResp.CreateFixtureInstance.ID

	// Create a look with fixture values for this fixture
	var lookResp struct {
		CreateLook struct {
			ID            string `json:"id"`
			FixtureValues []struct {
				Fixture struct {
					ID string `json:"id"`
				} `json:"fixture"`
			} `json:"fixtureValues"`
		} `json:"createLook"`
	}
	err = c.Post(`mutation($projectId: ID!, $fixtureId: ID!) {
		createLook(input: {
			name: "Look With Fixture Values"
			projectId: $projectId
			fixtureValues: [
				{ fixtureId: $fixtureId, channels: [{ offset: 0, value: 255 }] }
			]
		}) {
			id
			fixtureValues {
				fixture { id }
			}
		}
	}`, &lookResp,
		client.Var("projectId", projectID),
		client.Var("fixtureId", fixtureID))
	if err != nil {
		t.Fatalf("CreateLook mutation failed: %v", err)
	}
	lookID := lookResp.CreateLook.ID

	// Verify the look has fixture values
	if len(lookResp.CreateLook.FixtureValues) != 1 {
		t.Fatalf("Expected 1 fixture value in look, got %d", len(lookResp.CreateLook.FixtureValues))
	}

	// Delete the fixture instance
	var deleteResp struct {
		DeleteFixtureInstance bool `json:"deleteFixtureInstance"`
	}
	err = c.Post(`mutation($id: ID!) {
		deleteFixtureInstance(id: $id)
	}`, &deleteResp, client.Var("id", fixtureID))
	if err != nil {
		t.Fatalf("DeleteFixtureInstance mutation failed: %v", err)
	}
	if !deleteResp.DeleteFixtureInstance {
		t.Error("Expected deleteFixtureInstance to return true")
	}

	// Fetch the look again and verify fixture values are gone
	var lookQueryResp struct {
		Look struct {
			ID            string `json:"id"`
			FixtureValues []struct {
				Fixture struct {
					ID string `json:"id"`
				} `json:"fixture"`
			} `json:"fixtureValues"`
		} `json:"look"`
	}
	err = c.Post(`query($id: ID!) {
		look(id: $id) {
			id
			fixtureValues {
				fixture { id }
			}
		}
	}`, &lookQueryResp, client.Var("id", lookID))
	if err != nil {
		t.Fatalf("Look query failed: %v", err)
	}

	// The look should now have 0 fixture values (the fixture was deleted and cascaded)
	if len(lookQueryResp.Look.FixtureValues) != 0 {
		t.Errorf("Expected 0 fixture values after fixture deletion, got %d", len(lookQueryResp.Look.FixtureValues))
	}
}

// =============================================================================
// Look CRUD Tests
// =============================================================================

func TestLook_Create(t *testing.T) {
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
		CreateLook struct {
			ID          string `json:"id"`
			Name        string `json:"name"`
			Description string `json:"description"`
		} `json:"createLook"`
	}
	err = c.Post(`mutation($projectId: ID!, $fixtureId: ID!) {
		createLook(input: {
			name: "Red Scene"
			description: "All fixtures red"
			projectId: $projectId
			fixtureValues: [
				{
					fixtureId: $fixtureId,
					channels: [
						{ offset: 0, value: 255 },
						{ offset: 1, value: 0 },
						{ offset: 2, value: 0 }
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
		t.Fatalf("CreateLook mutation failed: %v", err)
	}

	if sceneResp.CreateLook.ID == "" {
		t.Error("Expected scene ID to be set")
	}
	if sceneResp.CreateLook.Name != "Red Scene" {
		t.Errorf("Expected name 'Red Scene', got '%s'", sceneResp.CreateLook.Name)
	}
}

func TestLook_Read(t *testing.T) {
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
		CreateLook struct {
			ID string `json:"id"`
		} `json:"createLook"`
	}
	_ = c.Post(`mutation($projectId: ID!) {
		createLook(input: {
			name: "Test Scene"
			projectId: $projectId
			fixtureValues: []
		}) {
			id
		}
	}`, &createResp, client.Var("projectId", projectResp.CreateProject.ID))

	// Read the scene
	var readResp struct {
		Look struct {
			ID   string `json:"id"`
			Name string `json:"name"`
		} `json:"look"`
	}
	err := c.Post(`query($id: ID!) {
		look(id: $id) {
			id
			name
		}
	}`, &readResp, client.Var("id", createResp.CreateLook.ID))

	if err != nil {
		t.Fatalf("Look query failed: %v", err)
	}

	if readResp.Look.ID != createResp.CreateLook.ID {
		t.Errorf("Expected scene ID %s, got %s", createResp.CreateLook.ID, readResp.Look.ID)
	}
}

func TestLook_Update(t *testing.T) {
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
		CreateLook struct {
			ID string `json:"id"`
		} `json:"createLook"`
	}
	_ = c.Post(`mutation($projectId: ID!) {
		createLook(input: {
			name: "Original Name"
			projectId: $projectId
			fixtureValues: []
		}) {
			id
		}
	}`, &createResp, client.Var("projectId", projectResp.CreateProject.ID))

	// Update the scene
	var updateResp struct {
		UpdateLook struct {
			ID   string `json:"id"`
			Name string `json:"name"`
		} `json:"updateLook"`
	}
	err := c.Post(`mutation($id: ID!) {
		updateLook(id: $id, input: { name: "Updated Name" }) {
			id
			name
		}
	}`, &updateResp, client.Var("id", createResp.CreateLook.ID))

	if err != nil {
		t.Fatalf("UpdateLook mutation failed: %v", err)
	}

	if updateResp.UpdateLook.Name != "Updated Name" {
		t.Errorf("Expected name 'Updated Name', got '%s'", updateResp.UpdateLook.Name)
	}
}

func TestLook_Delete(t *testing.T) {
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
		CreateLook struct {
			ID string `json:"id"`
		} `json:"createLook"`
	}
	_ = c.Post(`mutation($projectId: ID!) {
		createLook(input: {
			name: "To Delete"
			projectId: $projectId
			fixtureValues: []
		}) {
			id
		}
	}`, &createResp, client.Var("projectId", projectResp.CreateProject.ID))

	// Delete the scene
	var deleteResp struct {
		DeleteLook bool `json:"deleteLook"`
	}
	err := c.Post(`mutation($id: ID!) {
		deleteLook(id: $id)
	}`, &deleteResp, client.Var("id", createResp.CreateLook.ID))

	if err != nil {
		t.Fatalf("DeleteLook mutation failed: %v", err)
	}

	if !deleteResp.DeleteLook {
		t.Error("Expected deleteLook to return true")
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
		CreateLook struct {
			ID string `json:"id"`
		} `json:"createLook"`
	}
	_ = c.Post(`mutation($projectId: ID!) {
		createLook(input: {
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
	err := c.Post(`mutation($cueListId: ID!, $lookId: ID!) {
		createCue(input: {
			name: "Cue 1"
			cueNumber: 1.0
			cueListId: $cueListId
			lookId: $lookId
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
		client.Var("lookId", sceneResp.CreateLook.ID))

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
		CreateLook struct {
			ID string `json:"id"`
		} `json:"createLook"`
	}
	_ = c.Post(`mutation($projectId: ID!) {
		createLook(input: {
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
	_ = c.Post(`mutation($cueListId: ID!, $lookId: ID!) {
		createCue(input: {
			name: "Original Name"
			cueNumber: 1.0
			cueListId: $cueListId
			lookId: $lookId
			fadeInTime: 3.0
			fadeOutTime: 2.0
		}) {
			id
		}
	}`, &createResp,
		client.Var("cueListId", cueListResp.CreateCueList.ID),
		client.Var("lookId", sceneResp.CreateLook.ID))

	// Update the cue - note: updateCue uses CreateCueInput which requires all fields
	var updateResp struct {
		UpdateCue struct {
			ID         string  `json:"id"`
			Name       string  `json:"name"`
			FadeInTime float64 `json:"fadeInTime"`
		} `json:"updateCue"`
	}
	err := c.Post(`mutation($id: ID!, $cueListId: ID!, $lookId: ID!) {
		updateCue(id: $id, input: {
			name: "Updated Name",
			cueNumber: 1.0,
			cueListId: $cueListId,
			lookId: $lookId,
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
		client.Var("lookId", sceneResp.CreateLook.ID))

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
		CreateLook struct {
			ID string `json:"id"`
		} `json:"createLook"`
	}
	_ = c.Post(`mutation($projectId: ID!) {
		createLook(input: {
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
	_ = c.Post(`mutation($cueListId: ID!, $lookId: ID!) {
		createCue(input: {
			name: "To Delete"
			cueNumber: 1.0
			cueListId: $cueListId
			lookId: $lookId
			fadeInTime: 3.0
			fadeOutTime: 2.0
		}) {
			id
		}
	}`, &createResp,
		client.Var("cueListId", cueListResp.CreateCueList.ID),
		client.Var("lookId", sceneResp.CreateLook.ID))

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
// Look Board CRUD Tests
// =============================================================================

func TestLookBoard_Create(t *testing.T) {
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
		CreateLookBoard struct {
			ID              string  `json:"id"`
			Name            string  `json:"name"`
			DefaultFadeTime float64 `json:"defaultFadeTime"`
		} `json:"createLookBoard"`
	}
	err := c.Post(`mutation($projectId: ID!) {
		createLookBoard(input: {
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
		t.Fatalf("CreateLookBoard mutation failed: %v", err)
	}

	if boardResp.CreateLookBoard.ID == "" {
		t.Error("Expected scene board ID to be set")
	}
	if boardResp.CreateLookBoard.Name != "Main Board" {
		t.Errorf("Expected name 'Main Board', got '%s'", boardResp.CreateLookBoard.Name)
	}
}

func TestLookBoard_Update(t *testing.T) {
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
		CreateLookBoard struct {
			ID string `json:"id"`
		} `json:"createLookBoard"`
	}
	_ = c.Post(`mutation($projectId: ID!) {
		createLookBoard(input: {
			name: "Original Name"
			projectId: $projectId
		}) {
			id
		}
	}`, &createResp, client.Var("projectId", projectResp.CreateProject.ID))

	// Update scene board
	var updateResp struct {
		UpdateLookBoard struct {
			ID              string  `json:"id"`
			Name            string  `json:"name"`
			DefaultFadeTime float64 `json:"defaultFadeTime"`
		} `json:"updateLookBoard"`
	}
	err := c.Post(`mutation($id: ID!) {
		updateLookBoard(id: $id, input: { name: "Updated Name", defaultFadeTime: 5.0 }) {
			id
			name
			defaultFadeTime
		}
	}`, &updateResp, client.Var("id", createResp.CreateLookBoard.ID))

	if err != nil {
		t.Fatalf("UpdateLookBoard mutation failed: %v", err)
	}

	if updateResp.UpdateLookBoard.Name != "Updated Name" {
		t.Errorf("Expected name 'Updated Name', got '%s'", updateResp.UpdateLookBoard.Name)
	}
}

func TestLookBoard_Delete(t *testing.T) {
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
		CreateLookBoard struct {
			ID string `json:"id"`
		} `json:"createLookBoard"`
	}
	_ = c.Post(`mutation($projectId: ID!) {
		createLookBoard(input: {
			name: "To Delete"
			projectId: $projectId
		}) {
			id
		}
	}`, &createResp, client.Var("projectId", projectResp.CreateProject.ID))

	// Delete scene board
	var deleteResp struct {
		DeleteLookBoard bool `json:"deleteLookBoard"`
	}
	err := c.Post(`mutation($id: ID!) {
		deleteLookBoard(id: $id)
	}`, &deleteResp, client.Var("id", createResp.CreateLookBoard.ID))

	if err != nil {
		t.Fatalf("DeleteLookBoard mutation failed: %v", err)
	}

	if !deleteResp.DeleteLookBoard {
		t.Error("Expected deleteLookBoard to return true")
	}
}

// =============================================================================
// Look Board Button CRUD Tests
// =============================================================================

func TestLookBoardButton_Create(t *testing.T) {
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
		CreateLook struct {
			ID string `json:"id"`
		} `json:"createLook"`
	}
	_ = c.Post(`mutation($projectId: ID!) {
		createLook(input: {
			name: "Test Scene"
			projectId: $projectId
			fixtureValues: []
		}) {
			id
		}
	}`, &sceneResp, client.Var("projectId", projectResp.CreateProject.ID))

	var boardResp struct {
		CreateLookBoard struct {
			ID string `json:"id"`
		} `json:"createLookBoard"`
	}
	_ = c.Post(`mutation($projectId: ID!) {
		createLookBoard(input: {
			name: "Test Board"
			projectId: $projectId
		}) {
			id
		}
	}`, &boardResp, client.Var("projectId", projectResp.CreateProject.ID))

	// Create button using addLookToBoard mutation
	var buttonResp struct {
		AddLookToBoard struct {
			ID      string `json:"id"`
			LayoutX int    `json:"layoutX"`
			LayoutY int    `json:"layoutY"`
			Label   string `json:"label"`
		} `json:"addLookToBoard"`
	}
	err := c.Post(`mutation($boardId: ID!, $lookId: ID!) {
		addLookToBoard(input: {
			lookBoardId: $boardId
			lookId: $lookId
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
		client.Var("boardId", boardResp.CreateLookBoard.ID),
		client.Var("lookId", sceneResp.CreateLook.ID))

	if err != nil {
		t.Fatalf("AddLookToBoard mutation failed: %v", err)
	}

	if buttonResp.AddLookToBoard.ID == "" {
		t.Error("Expected button ID to be set")
	}
	if buttonResp.AddLookToBoard.LayoutX != 100 {
		t.Errorf("Expected layoutX 100, got %d", buttonResp.AddLookToBoard.LayoutX)
	}
}

func TestLookBoardButton_Delete(t *testing.T) {
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
		CreateLook struct {
			ID string `json:"id"`
		} `json:"createLook"`
	}
	_ = c.Post(`mutation($projectId: ID!) {
		createLook(input: {
			name: "Test Scene"
			projectId: $projectId
			fixtureValues: []
		}) {
			id
		}
	}`, &sceneResp, client.Var("projectId", projectResp.CreateProject.ID))

	var boardResp struct {
		CreateLookBoard struct {
			ID string `json:"id"`
		} `json:"createLookBoard"`
	}
	_ = c.Post(`mutation($projectId: ID!) {
		createLookBoard(input: {
			name: "Test Board"
			projectId: $projectId
		}) {
			id
		}
	}`, &boardResp, client.Var("projectId", projectResp.CreateProject.ID))

	// Create button using addLookToBoard
	var buttonResp struct {
		AddLookToBoard struct {
			ID string `json:"id"`
		} `json:"addLookToBoard"`
	}
	_ = c.Post(`mutation($boardId: ID!, $lookId: ID!) {
		addLookToBoard(input: {
			lookBoardId: $boardId
			lookId: $lookId
			layoutX: 0
			layoutY: 0
		}) {
			id
		}
	}`, &buttonResp,
		client.Var("boardId", boardResp.CreateLookBoard.ID),
		client.Var("lookId", sceneResp.CreateLook.ID))

	// Delete button using removeLookFromBoard
	var deleteResp struct {
		RemoveLookFromBoard bool `json:"removeLookFromBoard"`
	}
	err := c.Post(`mutation($buttonId: ID!) {
		removeLookFromBoard(buttonId: $buttonId)
	}`, &deleteResp, client.Var("buttonId", buttonResp.AddLookToBoard.ID))

	if err != nil {
		t.Fatalf("RemoveLookFromBoard mutation failed: %v", err)
	}

	if !deleteResp.RemoveLookFromBoard {
		t.Error("Expected removeLookFromBoard to return true")
	}
}

// =============================================================================
// Bulk Operations Tests
// =============================================================================

func TestBulkCreateProjects(t *testing.T) {
	c, _, cleanup := testSetup(t)
	defer cleanup()

	var resp struct {
		BulkCreateProjects []struct {
			ID   string `json:"id"`
			Name string `json:"name"`
		} `json:"bulkCreateProjects"`
	}

	err := c.Post(`mutation {
		bulkCreateProjects(input: {
			projects: [
				{ name: "Project 1" },
				{ name: "Project 2" },
				{ name: "Project 3" }
			]
		}) {
			id
			name
		}
	}`, &resp)

	if err != nil {
		t.Fatalf("BulkCreateProjects mutation failed: %v", err)
	}

	if len(resp.BulkCreateProjects) != 3 {
		t.Errorf("Expected 3 projects, got %d", len(resp.BulkCreateProjects))
	}

	names := map[string]bool{}
	for _, p := range resp.BulkCreateProjects {
		names[p.Name] = true
	}
	if !names["Project 1"] || !names["Project 2"] || !names["Project 3"] {
		t.Error("Expected all projects to be created with correct names")
	}
}

func TestBulkDeleteProjects(t *testing.T) {
	c, _, cleanup := testSetup(t)
	defer cleanup()

	// Create projects first
	var createResp struct {
		BulkCreateProjects []struct {
			ID string `json:"id"`
		} `json:"bulkCreateProjects"`
	}
	err := c.Post(`mutation {
		bulkCreateProjects(input: {
			projects: [
				{ name: "To Delete 1" },
				{ name: "To Delete 2" }
			]
		}) {
			id
		}
	}`, &createResp)
	if err != nil {
		t.Fatalf("BulkCreateProjects failed: %v", err)
	}

	ids := make([]string, len(createResp.BulkCreateProjects))
	for i, p := range createResp.BulkCreateProjects {
		ids[i] = p.ID
	}

	// Delete them
	var deleteResp struct {
		BulkDeleteProjects struct {
			DeletedCount int      `json:"deletedCount"`
			DeletedIds   []string `json:"deletedIds"`
		} `json:"bulkDeleteProjects"`
	}
	err = c.Post(`mutation($ids: [ID!]!) {
		bulkDeleteProjects(projectIds: $ids) {
			deletedCount
			deletedIds
		}
	}`, &deleteResp, client.Var("ids", ids))

	if err != nil {
		t.Fatalf("BulkDeleteProjects mutation failed: %v", err)
	}

	if deleteResp.BulkDeleteProjects.DeletedCount != 2 {
		t.Errorf("Expected deletedCount 2, got %d", deleteResp.BulkDeleteProjects.DeletedCount)
	}
}

func TestBulkCreateLooks(t *testing.T) {
	c, _, cleanup := testSetup(t)
	defer cleanup()

	// Create a project first
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

	var resp struct {
		BulkCreateLooks []struct {
			ID   string `json:"id"`
			Name string `json:"name"`
		} `json:"bulkCreateLooks"`
	}

	err = c.Post(`mutation($projectId: ID!) {
		bulkCreateLooks(input: {
			looks: [
				{ name: "Scene 1", projectId: $projectId, fixtureValues: [] },
				{ name: "Scene 2", projectId: $projectId, fixtureValues: [] }
			]
		}) {
			id
			name
		}
	}`, &resp, client.Var("projectId", projectResp.CreateProject.ID))

	if err != nil {
		t.Fatalf("BulkCreateLooks mutation failed: %v", err)
	}

	if len(resp.BulkCreateLooks) != 2 {
		t.Errorf("Expected 2 scenes, got %d", len(resp.BulkCreateLooks))
	}
}

func TestBulkDeleteLooks(t *testing.T) {
	c, _, cleanup := testSetup(t)
	defer cleanup()

	// Create a project first
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

	// Create scenes
	var createResp struct {
		BulkCreateLooks []struct {
			ID string `json:"id"`
		} `json:"bulkCreateLooks"`
	}
	err = c.Post(`mutation($projectId: ID!) {
		bulkCreateLooks(input: {
			looks: [
				{ name: "To Delete 1", projectId: $projectId, fixtureValues: [] },
				{ name: "To Delete 2", projectId: $projectId, fixtureValues: [] }
			]
		}) {
			id
		}
	}`, &createResp, client.Var("projectId", projectResp.CreateProject.ID))
	if err != nil {
		t.Fatalf("BulkCreateLooks failed: %v", err)
	}

	ids := make([]string, len(createResp.BulkCreateLooks))
	for i, s := range createResp.BulkCreateLooks {
		ids[i] = s.ID
	}

	// Delete them
	var deleteResp struct {
		BulkDeleteLooks struct {
			DeletedCount int      `json:"deletedCount"`
			DeletedIds   []string `json:"deletedIds"`
		} `json:"bulkDeleteLooks"`
	}
	err = c.Post(`mutation($ids: [ID!]!) {
		bulkDeleteLooks(lookIds: $ids) {
			deletedCount
			deletedIds
		}
	}`, &deleteResp, client.Var("ids", ids))

	if err != nil {
		t.Fatalf("BulkDeleteLooks mutation failed: %v", err)
	}

	if deleteResp.BulkDeleteLooks.DeletedCount != 2 {
		t.Errorf("Expected deletedCount 2, got %d", deleteResp.BulkDeleteLooks.DeletedCount)
	}
}

func TestProjectsByIds(t *testing.T) {
	c, _, cleanup := testSetup(t)
	defer cleanup()

	// Create projects
	var createResp struct {
		BulkCreateProjects []struct {
			ID   string `json:"id"`
			Name string `json:"name"`
		} `json:"bulkCreateProjects"`
	}
	err := c.Post(`mutation {
		bulkCreateProjects(input: {
			projects: [
				{ name: "Project A" },
				{ name: "Project B" },
				{ name: "Project C" }
			]
		}) {
			id
			name
		}
	}`, &createResp)
	if err != nil {
		t.Fatalf("BulkCreateProjects failed: %v", err)
	}

	// Query by IDs (just first two)
	ids := []string{createResp.BulkCreateProjects[0].ID, createResp.BulkCreateProjects[1].ID}

	var queryResp struct {
		ProjectsByIds []struct {
			ID   string `json:"id"`
			Name string `json:"name"`
		} `json:"projectsByIds"`
	}
	err = c.Post(`query($ids: [ID!]!) {
		projectsByIds(ids: $ids) {
			id
			name
		}
	}`, &queryResp, client.Var("ids", ids))

	if err != nil {
		t.Fatalf("ProjectsByIds query failed: %v", err)
	}

	if len(queryResp.ProjectsByIds) != 2 {
		t.Errorf("Expected 2 projects, got %d", len(queryResp.ProjectsByIds))
	}
}

func TestLooksByIds(t *testing.T) {
	c, _, cleanup := testSetup(t)
	defer cleanup()

	// Create a project first
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

	// Create scenes
	var createResp struct {
		BulkCreateLooks []struct {
			ID   string `json:"id"`
			Name string `json:"name"`
		} `json:"bulkCreateLooks"`
	}
	err = c.Post(`mutation($projectId: ID!) {
		bulkCreateLooks(input: {
			looks: [
				{ name: "Scene A", projectId: $projectId, fixtureValues: [] },
				{ name: "Scene B", projectId: $projectId, fixtureValues: [] }
			]
		}) {
			id
			name
		}
	}`, &createResp, client.Var("projectId", projectResp.CreateProject.ID))
	if err != nil {
		t.Fatalf("BulkCreateLooks failed: %v", err)
	}

	ids := make([]string, len(createResp.BulkCreateLooks))
	for i, s := range createResp.BulkCreateLooks {
		ids[i] = s.ID
	}

	var queryResp struct {
		LooksByIds []struct {
			ID   string `json:"id"`
			Name string `json:"name"`
		} `json:"looksByIds"`
	}
	err = c.Post(`query($ids: [ID!]!) {
		looksByIds(ids: $ids) {
			id
			name
		}
	}`, &queryResp, client.Var("ids", ids))

	if err != nil {
		t.Fatalf("LooksByIds query failed: %v", err)
	}

	if len(queryResp.LooksByIds) != 2 {
		t.Errorf("Expected 2 scenes, got %d", len(queryResp.LooksByIds))
	}
}

// =============================================================================
// BuildInfo Query Tests
// =============================================================================

func TestBuildInfo_Query(t *testing.T) {
	c, _, cleanup := testSetup(t)
	defer cleanup()

	// Reset build info for testing and set known values
	version.ResetBuildInfoForTesting()
	version.SetBuildInfo("v1.2.3", "abc123def", "2025-01-15T10:30:00Z")

	var resp struct {
		BuildInfo struct {
			Version   string `json:"version"`
			GitCommit string `json:"gitCommit"`
			BuildTime string `json:"buildTime"`
		} `json:"buildInfo"`
	}

	err := c.Post(`query {
		buildInfo {
			version
			gitCommit
			buildTime
		}
	}`, &resp)

	if err != nil {
		t.Fatalf("BuildInfo query failed: %v", err)
	}

	if resp.BuildInfo.Version != "v1.2.3" {
		t.Errorf("Expected version 'v1.2.3', got %q", resp.BuildInfo.Version)
	}
	if resp.BuildInfo.GitCommit != "abc123def" {
		t.Errorf("Expected gitCommit 'abc123def', got %q", resp.BuildInfo.GitCommit)
	}
	if resp.BuildInfo.BuildTime != "2025-01-15T10:30:00Z" {
		t.Errorf("Expected buildTime '2025-01-15T10:30:00Z', got %q", resp.BuildInfo.BuildTime)
	}
}

func TestBuildInfo_DefaultValues(t *testing.T) {
	c, _, cleanup := testSetup(t)
	defer cleanup()

	// Reset build info to defaults
	version.ResetBuildInfoForTesting()

	var resp struct {
		BuildInfo struct {
			Version   string `json:"version"`
			GitCommit string `json:"gitCommit"`
			BuildTime string `json:"buildTime"`
		} `json:"buildInfo"`
	}

	err := c.Post(`query {
		buildInfo {
			version
			gitCommit
			buildTime
		}
	}`, &resp)

	if err != nil {
		t.Fatalf("BuildInfo query failed: %v", err)
	}

	// Should return default values when not set via ldflags
	if resp.BuildInfo.Version != "0.1.0" {
		t.Errorf("Expected default version '0.1.0', got %q", resp.BuildInfo.Version)
	}
	if resp.BuildInfo.GitCommit != "unknown" {
		t.Errorf("Expected default gitCommit 'unknown', got %q", resp.BuildInfo.GitCommit)
	}
	if resp.BuildInfo.BuildTime != "unknown" {
		t.Errorf("Expected default buildTime 'unknown', got %q", resp.BuildInfo.BuildTime)
	}
}
