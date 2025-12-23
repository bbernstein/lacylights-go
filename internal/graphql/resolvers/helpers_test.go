package resolvers

import (
	"context"
	"testing"

	"github.com/bbernstein/lacylights-go/internal/database/models"
)

func TestIntPtr(t *testing.T) {
	tests := []struct {
		input    int
		expected int
	}{
		{0, 0},
		{1, 1},
		{-1, -1},
		{100, 100},
		{255, 255},
		{-100, -100},
	}

	for _, tt := range tests {
		result := intPtr(tt.input)
		if result == nil {
			t.Errorf("intPtr(%d) returned nil", tt.input)
			continue
		}
		if *result != tt.expected {
			t.Errorf("intPtr(%d) = %d, want %d", tt.input, *result, tt.expected)
		}
	}
}

func TestIntPtr_IndependentPointers(t *testing.T) {
	// Test that each call returns an independent pointer
	ptr1 := intPtr(10)
	ptr2 := intPtr(10)

	if ptr1 == ptr2 {
		t.Error("intPtr should return independent pointers")
	}

	*ptr1 = 20
	if *ptr2 != 10 {
		t.Error("Modifying one pointer should not affect the other")
	}
}

func TestIntPtr_ZeroValue(t *testing.T) {
	ptr := intPtr(0)
	if ptr == nil {
		t.Fatal("intPtr(0) should not return nil")
	}
	if *ptr != 0 {
		t.Errorf("intPtr(0) = %d, want 0", *ptr)
	}
}

func TestIntPtr_MaxInt(t *testing.T) {
	maxInt := int(^uint(0) >> 1)
	ptr := intPtr(maxInt)
	if ptr == nil {
		t.Fatal("intPtr(maxInt) should not return nil")
	}
	if *ptr != maxInt {
		t.Errorf("intPtr(maxInt) = %d, want %d", *ptr, maxInt)
	}
}

func TestIntPtr_MinInt(t *testing.T) {
	minInt := -int(^uint(0)>>1) - 1
	ptr := intPtr(minInt)
	if ptr == nil {
		t.Fatal("intPtr(minInt) should not return nil")
	}
	if *ptr != minInt {
		t.Errorf("intPtr(minInt) = %d, want %d", *ptr, minInt)
	}
}

func TestIntPtr_ConsecutiveCalls(t *testing.T) {
	// Test rapid consecutive calls
	results := make([]*int, 100)
	for i := 0; i < 100; i++ {
		results[i] = intPtr(i)
	}

	// Verify all values are correct
	for i := 0; i < 100; i++ {
		if results[i] == nil {
			t.Fatalf("intPtr(%d) returned nil", i)
		}
		if *results[i] != i {
			t.Errorf("intPtr(%d) = %d, want %d", i, *results[i], i)
		}
	}

	// Verify all pointers are unique
	ptrSet := make(map[*int]bool)
	for _, ptr := range results {
		if ptrSet[ptr] {
			t.Error("intPtr returned duplicate pointer")
		}
		ptrSet[ptr] = true
	}
}

func TestIntPtr_NegativeValues(t *testing.T) {
	tests := []int{-1, -10, -100, -1000, -32768, -65536}

	for _, val := range tests {
		ptr := intPtr(val)
		if ptr == nil {
			t.Fatalf("intPtr(%d) returned nil", val)
		}
		if *ptr != val {
			t.Errorf("intPtr(%d) = %d, want %d", val, *ptr, val)
		}
	}
}

func TestIntPtr_CommonDMXValues(t *testing.T) {
	// Test common DMX channel values
	dmxValues := []int{0, 1, 127, 128, 254, 255}

	for _, val := range dmxValues {
		ptr := intPtr(val)
		if ptr == nil {
			t.Fatalf("intPtr(%d) returned nil", val)
		}
		if *ptr != val {
			t.Errorf("intPtr(%d) = %d, want %d", val, *ptr, val)
		}
	}
}

func TestIntPtr_UniverseNumbers(t *testing.T) {
	// Test common universe numbers
	universes := []int{0, 1, 2, 3, 4, 15, 16, 31, 32, 63}

	for _, val := range universes {
		ptr := intPtr(val)
		if ptr == nil {
			t.Fatalf("intPtr(%d) returned nil", val)
		}
		if *ptr != val {
			t.Errorf("intPtr(%d) = %d, want %d", val, *ptr, val)
		}
	}
}

func TestIntPtr_ChannelOffsets(t *testing.T) {
	// Test channel offsets (0-511 for DMX)
	offsets := []int{0, 1, 255, 256, 510, 511}

	for _, val := range offsets {
		ptr := intPtr(val)
		if ptr == nil {
			t.Fatalf("intPtr(%d) returned nil", val)
		}
		if *ptr != val {
			t.Errorf("intPtr(%d) = %d, want %d", val, *ptr, val)
		}
	}
}

func TestSparseChannelsEqual_IdenticalJSON(t *testing.T) {
	json1 := `[{"offset":0,"value":255},{"offset":1,"value":128}]`
	json2 := `[{"offset":0,"value":255},{"offset":1,"value":128}]`

	if !sparseChannelsEqual(json1, json2) {
		t.Error("sparseChannelsEqual should return true for identical JSON")
	}
}

func TestSparseChannelsEqual_DifferentOrder(t *testing.T) {
	// Same values but different order in the JSON array
	json1 := `[{"offset":0,"value":255},{"offset":1,"value":128}]`
	json2 := `[{"offset":1,"value":128},{"offset":0,"value":255}]`

	if !sparseChannelsEqual(json1, json2) {
		t.Error("sparseChannelsEqual should return true for same values in different order")
	}
}

func TestSparseChannelsEqual_DifferentValues(t *testing.T) {
	json1 := `[{"offset":0,"value":255},{"offset":1,"value":128}]`
	json2 := `[{"offset":0,"value":255},{"offset":1,"value":64}]`

	if sparseChannelsEqual(json1, json2) {
		t.Error("sparseChannelsEqual should return false for different values")
	}
}

func TestSparseChannelsEqual_DifferentOffsets(t *testing.T) {
	json1 := `[{"offset":0,"value":255},{"offset":1,"value":128}]`
	json2 := `[{"offset":0,"value":255},{"offset":2,"value":128}]`

	if sparseChannelsEqual(json1, json2) {
		t.Error("sparseChannelsEqual should return false for different offsets")
	}
}

func TestSparseChannelsEqual_DifferentLength(t *testing.T) {
	json1 := `[{"offset":0,"value":255},{"offset":1,"value":128}]`
	json2 := `[{"offset":0,"value":255}]`

	if sparseChannelsEqual(json1, json2) {
		t.Error("sparseChannelsEqual should return false for different number of channels")
	}
}

func TestSparseChannelsEqual_EmptyArrays(t *testing.T) {
	json1 := `[]`
	json2 := `[]`

	if !sparseChannelsEqual(json1, json2) {
		t.Error("sparseChannelsEqual should return true for two empty arrays")
	}
}

func TestSparseChannelsEqual_EmptyAndNonEmpty(t *testing.T) {
	json1 := `[]`
	json2 := `[{"offset":0,"value":255}]`

	if sparseChannelsEqual(json1, json2) {
		t.Error("sparseChannelsEqual should return false for empty vs non-empty")
	}
}

func TestSparseChannelsEqual_InvalidJSON(t *testing.T) {
	validJSON := `[{"offset":0,"value":255}]`
	invalidJSON := `not valid json`

	// Invalid JSON should return false (safer behavior)
	if sparseChannelsEqual(validJSON, invalidJSON) {
		t.Error("sparseChannelsEqual should return false for valid vs invalid JSON")
	}

	// Two invalid JSONs should return false (invalid data is never "equal")
	if sparseChannelsEqual(invalidJSON, invalidJSON) {
		t.Error("sparseChannelsEqual should return false for two invalid JSONs")
	}
}

func TestSparseChannelsEqual_WhitespaceDifference(t *testing.T) {
	// Same values with different whitespace
	json1 := `[{"offset":0,"value":255},{"offset":1,"value":128}]`
	json2 := `[{"offset": 0, "value": 255}, {"offset": 1, "value": 128}]`

	if !sparseChannelsEqual(json1, json2) {
		t.Error("sparseChannelsEqual should return true regardless of whitespace")
	}
}

func TestSparseChannelsEqual_ManyChannels(t *testing.T) {
	// Test with more channels, different order
	json1 := `[{"offset":0,"value":255},{"offset":5,"value":128},{"offset":10,"value":64},{"offset":15,"value":32}]`
	json2 := `[{"offset":15,"value":32},{"offset":0,"value":255},{"offset":10,"value":64},{"offset":5,"value":128}]`

	if !sparseChannelsEqual(json1, json2) {
		t.Error("sparseChannelsEqual should return true for same channels in different order")
	}
}

func TestSparseChannelsEqual_DuplicateOffsets(t *testing.T) {
	// JSON with duplicate offsets should return false
	validJSON := `[{"offset":0,"value":255},{"offset":1,"value":128}]`
	duplicateJSON := `[{"offset":0,"value":100},{"offset":0,"value":200}]`

	// Duplicate in second arg
	if sparseChannelsEqual(validJSON, duplicateJSON) {
		t.Error("sparseChannelsEqual should return false when channels2 has duplicate offsets")
	}

	// Duplicate in first arg
	if sparseChannelsEqual(duplicateJSON, validJSON) {
		t.Error("sparseChannelsEqual should return false when channels1 has duplicate offsets")
	}

	// Both have same duplicates - should still return false
	if sparseChannelsEqual(duplicateJSON, duplicateJSON) {
		t.Error("sparseChannelsEqual should return false when both have duplicate offsets")
	}
}

// Test helpers for reapplyActiveSceneIfNeeded - these use the testSetup from dmx_integration_test.go
// Note: These tests require the test infrastructure from dmx_integration_test.go to be available

func TestReapplyActiveSceneIfNeeded_SceneNotActive(t *testing.T) {
	_, resolver, cleanup := testSetup(t)
	defer cleanup()

	ctx := context.Background()

	// When no scene is active, the function should return nil immediately
	err := resolver.reapplyActiveSceneIfNeeded(ctx, "non-existent-scene-id")
	if err != nil {
		t.Errorf("Expected nil error when scene is not active, got: %v", err)
	}
}

func TestReapplyActiveSceneIfNeeded_ActiveSceneDifferentID(t *testing.T) {
	_, resolver, cleanup := testSetup(t)
	defer cleanup()

	ctx := context.Background()

	// Set an active scene ID
	resolver.DMXService.SetActiveScene("scene-123")

	// Try to re-apply a different scene - should return nil without error
	err := resolver.reapplyActiveSceneIfNeeded(ctx, "different-scene-456")
	if err != nil {
		t.Errorf("Expected nil error when scene ID doesn't match active scene, got: %v", err)
	}
}

func TestReapplyActiveSceneIfNeeded_ActiveSceneNotInDB(t *testing.T) {
	_, resolver, cleanup := testSetup(t)
	defer cleanup()

	ctx := context.Background()

	// Set an active scene ID that doesn't exist in the database
	resolver.DMXService.SetActiveScene("non-existent-scene")

	// Try to re-apply - should return error because scene doesn't exist
	err := resolver.reapplyActiveSceneIfNeeded(ctx, "non-existent-scene")
	if err == nil {
		t.Error("Expected error when active scene doesn't exist in database, got nil")
	}
}

func TestReapplyActiveSceneIfNeeded_ActiveSceneReapplied(t *testing.T) {
	_, resolver, cleanup := testSetup(t)
	defer cleanup()

	ctx := context.Background()

	// Create a project
	project := &models.Project{
		ID:   "test-project-1",
		Name: "Test Project",
	}
	if err := resolver.ProjectRepo.Create(ctx, project); err != nil {
		t.Fatalf("Failed to create project: %v", err)
	}

	// Create a fixture definition
	def := &models.FixtureDefinition{
		ID:           "test-def-1",
		Manufacturer: "Test",
		Model:        "TestFixture",
		Type:         "LED_PAR",
	}
	if err := resolver.FixtureRepo.CreateDefinition(ctx, def); err != nil {
		t.Fatalf("Failed to create fixture definition: %v", err)
	}

	// Create a fixture instance
	fixture := &models.FixtureInstance{
		ID:           "test-fixture-1",
		Name:         "Test Fixture",
		ProjectID:    project.ID,
		DefinitionID: def.ID,
		Universe:     1,
		StartChannel: 1,
	}
	if err := resolver.FixtureRepo.Create(ctx, fixture); err != nil {
		t.Fatalf("Failed to create fixture instance: %v", err)
	}

	// Create a scene with fixture values
	scene := &models.Scene{
		ID:        "test-scene-1",
		Name:      "Test Scene",
		ProjectID: project.ID,
	}
	if err := resolver.SceneRepo.Create(ctx, scene); err != nil {
		t.Fatalf("Failed to create scene: %v", err)
	}

	// Create fixture values for the scene
	fixtureValue := &models.FixtureValue{
		SceneID:   scene.ID,
		FixtureID: fixture.ID,
		Channels:  `[{"offset":0,"value":200},{"offset":1,"value":100}]`,
	}
	if err := resolver.SceneRepo.CreateFixtureValue(ctx, fixtureValue); err != nil {
		t.Fatalf("Failed to create fixture value: %v", err)
	}

	// Set this scene as active
	resolver.DMXService.SetActiveScene(scene.ID)

	// Re-apply the scene
	err := resolver.reapplyActiveSceneIfNeeded(ctx, scene.ID)
	if err != nil {
		t.Fatalf("Failed to re-apply active scene: %v", err)
	}

	// Verify the DMX values were set
	// Channel 1 (offset 0 + startChannel 1) should be 200
	value1 := resolver.DMXService.GetChannelValue(1, 1)
	if value1 != 200 {
		t.Errorf("Expected channel 1 to be 200, got %d", value1)
	}

	// Channel 2 (offset 1 + startChannel 1) should be 100
	value2 := resolver.DMXService.GetChannelValue(1, 2)
	if value2 != 100 {
		t.Errorf("Expected channel 2 to be 100, got %d", value2)
	}
}

func TestReapplyActiveSceneIfNeeded_InvalidChannelJSON(t *testing.T) {
	_, resolver, cleanup := testSetup(t)
	defer cleanup()

	ctx := context.Background()

	// Create a project
	project := &models.Project{
		ID:   "test-project-2",
		Name: "Test Project 2",
	}
	if err := resolver.ProjectRepo.Create(ctx, project); err != nil {
		t.Fatalf("Failed to create project: %v", err)
	}

	// Create a fixture definition
	def := &models.FixtureDefinition{
		ID:           "test-def-2",
		Manufacturer: "Test",
		Model:        "TestFixture2",
		Type:         "LED_PAR",
	}
	if err := resolver.FixtureRepo.CreateDefinition(ctx, def); err != nil {
		t.Fatalf("Failed to create fixture definition: %v", err)
	}

	// Create a fixture instance
	fixture := &models.FixtureInstance{
		ID:           "test-fixture-2",
		Name:         "Test Fixture 2",
		ProjectID:    project.ID,
		DefinitionID: def.ID,
		Universe:     1,
		StartChannel: 10,
	}
	if err := resolver.FixtureRepo.Create(ctx, fixture); err != nil {
		t.Fatalf("Failed to create fixture instance: %v", err)
	}

	// Create a scene with invalid JSON in fixture values
	scene := &models.Scene{
		ID:        "test-scene-2",
		Name:      "Test Scene 2",
		ProjectID: project.ID,
	}
	if err := resolver.SceneRepo.Create(ctx, scene); err != nil {
		t.Fatalf("Failed to create scene: %v", err)
	}

	// Create fixture values with invalid JSON
	fixtureValue := &models.FixtureValue{
		SceneID:   scene.ID,
		FixtureID: fixture.ID,
		Channels:  `invalid json`,
	}
	if err := resolver.SceneRepo.CreateFixtureValue(ctx, fixtureValue); err != nil {
		t.Fatalf("Failed to create fixture value: %v", err)
	}

	// Set this scene as active
	resolver.DMXService.SetActiveScene(scene.ID)

	// Re-apply should succeed (gracefully handle invalid JSON with logging)
	err := resolver.reapplyActiveSceneIfNeeded(ctx, scene.ID)
	if err != nil {
		t.Errorf("Expected function to succeed despite invalid JSON (with warning logged), got: %v", err)
	}
}
