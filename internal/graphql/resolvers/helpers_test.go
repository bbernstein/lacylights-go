package resolvers

import (
	"context"
	"strings"
	"testing"

	"github.com/bbernstein/lacylights-go/internal/database/models"
)

func TestValidateCanvasDimension_ValidValues(t *testing.T) {
	tests := []struct {
		name      string
		value     int
		fieldName string
	}{
		{"minimum value", MinCanvasSize, "testField"},
		{"maximum value", MaxCanvasSize, "testField"},
		{"default value (2000)", 2000, "layoutCanvasWidth"},
		{"mid-range value", 5000, "layoutCanvasHeight"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateCanvasDimension(tt.value, tt.fieldName)
			if err != nil {
				t.Errorf("validateCanvasDimension(%d, %q) returned unexpected error: %v", tt.value, tt.fieldName, err)
			}
		})
	}
}

func TestValidateCanvasDimension_TooSmall(t *testing.T) {
	tests := []struct {
		name      string
		value     int
		fieldName string
	}{
		{"zero", 0, "layoutCanvasWidth"},
		{"negative", -100, "layoutCanvasHeight"},
		{"just below minimum", MinCanvasSize - 1, "testField"},
		{"very small", 10, "testField"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateCanvasDimension(tt.value, tt.fieldName)
			if err == nil {
				t.Errorf("validateCanvasDimension(%d, %q) should return error for value below minimum", tt.value, tt.fieldName)
			}
		})
	}
}

func TestValidateCanvasDimension_TooLarge(t *testing.T) {
	tests := []struct {
		name      string
		value     int
		fieldName string
	}{
		{"just above maximum", MaxCanvasSize + 1, "layoutCanvasWidth"},
		{"very large", 1000000, "layoutCanvasHeight"},
		{"extreme", 999999999, "testField"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateCanvasDimension(tt.value, tt.fieldName)
			if err == nil {
				t.Errorf("validateCanvasDimension(%d, %q) should return error for value above maximum", tt.value, tt.fieldName)
			}
		})
	}
}

func TestValidateCanvasDimension_ErrorMessage(t *testing.T) {
	// Test that error messages contain the field name and value
	err := validateCanvasDimension(50, "layoutCanvasWidth")
	if err == nil {
		t.Fatal("Expected error for value 50")
	}
	errMsg := err.Error()
	if !strings.Contains(errMsg, "layoutCanvasWidth") {
		t.Errorf("Error message should contain field name, got: %s", errMsg)
	}
	if !strings.Contains(errMsg, "50") {
		t.Errorf("Error message should contain the value, got: %s", errMsg)
	}

	// Test too large error message
	err = validateCanvasDimension(200000, "layoutCanvasHeight")
	if err == nil {
		t.Fatal("Expected error for value 200000")
	}
	errMsg = err.Error()
	if !strings.Contains(errMsg, "layoutCanvasHeight") {
		t.Errorf("Error message should contain field name, got: %s", errMsg)
	}
	if !strings.Contains(errMsg, "200000") {
		t.Errorf("Error message should contain the value, got: %s", errMsg)
	}
}

func TestIsNormalizedCoordinate(t *testing.T) {
	tests := []struct {
		name     string
		value    *float64
		expected bool
	}{
		{"nil value", nil, false},
		{"zero (edge case - not normalized)", floatPtr(0.0), false},
		{"one (edge case - not normalized)", floatPtr(1.0), false},
		{"mid-range normalized", floatPtr(0.5), true},
		{"low normalized", floatPtr(0.1), true},
		{"high normalized", floatPtr(0.9), true},
		{"pixel value", floatPtr(500.0), false},
		{"negative", floatPtr(-0.5), false},
		{"greater than one", floatPtr(1.5), false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isNormalizedCoordinate(tt.value)
			if result != tt.expected {
				if tt.value != nil {
					t.Errorf("isNormalizedCoordinate(%f) = %v, want %v", *tt.value, result, tt.expected)
				} else {
					t.Errorf("isNormalizedCoordinate(nil) = %v, want %v", result, tt.expected)
				}
			}
		})
	}
}

func TestConvertNormalizedToPixels(t *testing.T) {
	t.Run("converts normalized coordinates", func(t *testing.T) {
		fixture := &models.FixtureInstance{
			LayoutX: floatPtr(0.5),
			LayoutY: floatPtr(0.25),
		}

		converted := convertNormalizedToPixels(fixture, 2000, 2000)

		if !converted {
			t.Error("Expected conversion to occur")
		}
		if *fixture.LayoutX != 1000.0 {
			t.Errorf("LayoutX = %f, want 1000.0", *fixture.LayoutX)
		}
		if *fixture.LayoutY != 500.0 {
			t.Errorf("LayoutY = %f, want 500.0", *fixture.LayoutY)
		}
	})

	t.Run("skips pixel coordinates", func(t *testing.T) {
		fixture := &models.FixtureInstance{
			LayoutX: floatPtr(500.0),
			LayoutY: floatPtr(300.0),
		}

		converted := convertNormalizedToPixels(fixture, 2000, 2000)

		if converted {
			t.Error("Expected no conversion for pixel coordinates")
		}
		if *fixture.LayoutX != 500.0 {
			t.Errorf("LayoutX should be unchanged, got %f", *fixture.LayoutX)
		}
	})

	t.Run("skips nil fixture", func(t *testing.T) {
		converted := convertNormalizedToPixels(nil, 2000, 2000)
		if converted {
			t.Error("Expected no conversion for nil fixture")
		}
	})

	t.Run("skips nil coordinates", func(t *testing.T) {
		fixture := &models.FixtureInstance{
			LayoutX: nil,
			LayoutY: nil,
		}

		converted := convertNormalizedToPixels(fixture, 2000, 2000)

		if converted {
			t.Error("Expected no conversion for nil coordinates")
		}
	})

	t.Run("uses default canvas size for invalid dimensions", func(t *testing.T) {
		fixture := &models.FixtureInstance{
			LayoutX: floatPtr(0.5),
			LayoutY: floatPtr(0.5),
		}

		converted := convertNormalizedToPixels(fixture, 0, 0)

		if !converted {
			t.Error("Expected conversion to occur")
		}
		// Should use DefaultCanvasSize (2000)
		if *fixture.LayoutX != 1000.0 {
			t.Errorf("LayoutX = %f, want 1000.0 (using default canvas)", *fixture.LayoutX)
		}
	})

	t.Run("uses custom canvas dimensions", func(t *testing.T) {
		fixture := &models.FixtureInstance{
			LayoutX: floatPtr(0.5),
			LayoutY: floatPtr(0.5),
		}

		converted := convertNormalizedToPixels(fixture, 4000, 3000)

		if !converted {
			t.Error("Expected conversion to occur")
		}
		if *fixture.LayoutX != 2000.0 {
			t.Errorf("LayoutX = %f, want 2000.0", *fixture.LayoutX)
		}
		if *fixture.LayoutY != 1500.0 {
			t.Errorf("LayoutY = %f, want 1500.0", *fixture.LayoutY)
		}
	})

	t.Run("skips when only X is normalized", func(t *testing.T) {
		fixture := &models.FixtureInstance{
			LayoutX: floatPtr(0.5),
			LayoutY: floatPtr(500.0), // pixel value
		}

		converted := convertNormalizedToPixels(fixture, 2000, 2000)

		if converted {
			t.Error("Expected no conversion when only X is normalized")
		}
	})
}

// Helper function to create float64 pointer
func floatPtr(f float64) *float64 {
	return &f
}

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

// Test helpers for reapplyActiveLookIfNeeded - these use the testSetup from dmx_integration_test.go
// Note: These tests require the test infrastructure from dmx_integration_test.go to be available

func TestReapplyActiveLookIfNeeded_LookNotActive(t *testing.T) {
	_, resolver, cleanup := testSetup(t)
	defer cleanup()

	ctx := context.Background()

	// When no look is active, the function should return nil immediately
	err := resolver.reapplyActiveLookIfNeeded(ctx, "non-existent-look-id")
	if err != nil {
		t.Errorf("Expected nil error when look is not active, got: %v", err)
	}
}

func TestReapplyActiveLookIfNeeded_ActiveLookDifferentID(t *testing.T) {
	_, resolver, cleanup := testSetup(t)
	defer cleanup()

	ctx := context.Background()

	// Set an active look ID
	resolver.DMXService.SetActiveLook("look-123")

	// Try to re-apply a different look - should return nil without error
	err := resolver.reapplyActiveLookIfNeeded(ctx, "different-look-456")
	if err != nil {
		t.Errorf("Expected nil error when look ID doesn't match active look, got: %v", err)
	}
}

func TestReapplyActiveLookIfNeeded_ActiveLookNotInDB(t *testing.T) {
	_, resolver, cleanup := testSetup(t)
	defer cleanup()

	ctx := context.Background()

	// Set an active look ID that doesn't exist in the database
	resolver.DMXService.SetActiveLook("non-existent-look")

	// Try to re-apply - should return error because look doesn't exist
	err := resolver.reapplyActiveLookIfNeeded(ctx, "non-existent-look")
	if err == nil {
		t.Error("Expected error when active look doesn't exist in database, got nil")
	}
}

func TestReapplyActiveLookIfNeeded_ActiveLookReapplied(t *testing.T) {
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

	// Create a look with fixture values
	look := &models.Look{
		ID:        "test-look-1",
		Name:      "Test Look",
		ProjectID: project.ID,
	}
	if err := resolver.LookRepo.Create(ctx, look); err != nil {
		t.Fatalf("Failed to create look: %v", err)
	}

	// Create fixture values for the look
	fixtureValue := &models.FixtureValue{
		LookID:    look.ID,
		FixtureID: fixture.ID,
		Channels:  `[{"offset":0,"value":200},{"offset":1,"value":100}]`,
	}
	if err := resolver.LookRepo.CreateFixtureValue(ctx, fixtureValue); err != nil {
		t.Fatalf("Failed to create fixture value: %v", err)
	}

	// Set this look as active
	resolver.DMXService.SetActiveLook(look.ID)

	// Re-apply the look
	err := resolver.reapplyActiveLookIfNeeded(ctx, look.ID)
	if err != nil {
		t.Fatalf("Failed to re-apply active look: %v", err)
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

func TestReapplyActiveLookIfNeeded_InvalidChannelJSON(t *testing.T) {
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

	// Create a look with invalid JSON in fixture values
	look := &models.Look{
		ID:        "test-look-2",
		Name:      "Test Look 2",
		ProjectID: project.ID,
	}
	if err := resolver.LookRepo.Create(ctx, look); err != nil {
		t.Fatalf("Failed to create look: %v", err)
	}

	// Create fixture values with invalid JSON
	fixtureValue := &models.FixtureValue{
		LookID:    look.ID,
		FixtureID: fixture.ID,
		Channels:  `invalid json`,
	}
	if err := resolver.LookRepo.CreateFixtureValue(ctx, fixtureValue); err != nil {
		t.Fatalf("Failed to create fixture value: %v", err)
	}

	// Set this look as active
	resolver.DMXService.SetActiveLook(look.ID)

	// Re-apply should succeed (gracefully handle invalid JSON with logging)
	err := resolver.reapplyActiveLookIfNeeded(ctx, look.ID)
	if err != nil {
		t.Errorf("Expected function to succeed despite invalid JSON (with warning logged), got: %v", err)
	}
}
