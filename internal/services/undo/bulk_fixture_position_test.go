package undo

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/bbernstein/lacylights-go/internal/database/models"
	"github.com/lucsky/cuid"
)

// floatPtr returns a pointer to the given float64 value.
func floatPtr(f float64) *float64 {
	return &f
}

func TestService_CaptureFixturePositions(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	ctx := context.Background()
	service := createTestService(t, db)
	project := createTestProject(t, db)

	// Create fixture definition
	def := &models.FixtureDefinition{
		ID:           cuid.New(),
		Manufacturer: "Test",
		Model:        "TestModel",
		Type:         "LED_PAR",
	}
	db.Create(def)

	// Create fixtures with layout positions
	fixture1 := &models.FixtureInstance{
		ID:             cuid.New(),
		Name:           "Fixture 1",
		ProjectID:      project.ID,
		DefinitionID:   def.ID,
		Universe:       1,
		StartChannel:   1,
		LayoutX:        floatPtr(100.0),
		LayoutY:        floatPtr(200.0),
		LayoutRotation: floatPtr(45.0),
	}
	db.Create(fixture1)

	fixture2 := &models.FixtureInstance{
		ID:             cuid.New(),
		Name:           "Fixture 2",
		ProjectID:      project.ID,
		DefinitionID:   def.ID,
		Universe:       1,
		StartChannel:   5,
		LayoutX:        floatPtr(300.0),
		LayoutY:        floatPtr(400.0),
		LayoutRotation: nil,
	}
	db.Create(fixture2)

	// Capture positions
	snapshot, err := service.CaptureFixturePositions(ctx, []string{fixture1.ID, fixture2.ID})
	if err != nil {
		t.Fatalf("CaptureFixturePositions failed: %v", err)
	}

	if snapshot == nil {
		t.Fatal("Expected non-nil snapshot")
	}

	if len(snapshot.Positions) != 2 {
		t.Fatalf("Expected 2 positions, got %d", len(snapshot.Positions))
	}

	// Verify fixture1 position
	var pos1 *FixturePositionEntry
	for i := range snapshot.Positions {
		if snapshot.Positions[i].FixtureID == fixture1.ID {
			pos1 = &snapshot.Positions[i]
			break
		}
	}
	if pos1 == nil {
		t.Fatal("Fixture 1 not found in snapshot")
	}
	if pos1.LayoutX == nil || *pos1.LayoutX != 100.0 {
		t.Errorf("Expected LayoutX 100.0, got %v", pos1.LayoutX)
	}
	if pos1.LayoutY == nil || *pos1.LayoutY != 200.0 {
		t.Errorf("Expected LayoutY 200.0, got %v", pos1.LayoutY)
	}
	if pos1.LayoutRotation == nil || *pos1.LayoutRotation != 45.0 {
		t.Errorf("Expected LayoutRotation 45.0, got %v", pos1.LayoutRotation)
	}
}

func TestService_CaptureFixturePositions_NotFound(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	ctx := context.Background()
	service := createTestService(t, db)

	// Capture positions with non-existent fixture
	snapshot, err := service.CaptureFixturePositions(ctx, []string{"nonexistent-id"})
	if err != nil {
		t.Fatalf("CaptureFixturePositions failed: %v", err)
	}

	// Should return empty positions (non-existent fixtures are skipped)
	if len(snapshot.Positions) != 0 {
		t.Errorf("Expected 0 positions for non-existent fixture, got %d", len(snapshot.Positions))
	}
}

func TestService_CaptureFixturePositions_Empty(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	ctx := context.Background()
	service := createTestService(t, db)

	// Capture positions with empty list
	snapshot, err := service.CaptureFixturePositions(ctx, []string{})
	if err != nil {
		t.Fatalf("CaptureFixturePositions failed: %v", err)
	}

	if snapshot == nil {
		t.Fatal("Expected non-nil snapshot")
	}
	if len(snapshot.Positions) != 0 {
		t.Errorf("Expected 0 positions, got %d", len(snapshot.Positions))
	}
}

func TestService_ApplyBulkFixturePositionSnapshot(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	ctx := context.Background()
	service := createTestService(t, db)
	project := createTestProject(t, db)

	// Create fixture definition
	def := &models.FixtureDefinition{
		ID:           cuid.New(),
		Manufacturer: "Test",
		Model:        "TestModel",
		Type:         "LED_PAR",
	}
	db.Create(def)

	// Create fixtures with initial positions
	fixture1 := &models.FixtureInstance{
		ID:             cuid.New(),
		Name:           "Fixture 1",
		ProjectID:      project.ID,
		DefinitionID:   def.ID,
		Universe:       1,
		StartChannel:   1,
		LayoutX:        floatPtr(100.0),
		LayoutY:        floatPtr(200.0),
		LayoutRotation: floatPtr(45.0),
	}
	db.Create(fixture1)

	fixture2 := &models.FixtureInstance{
		ID:             cuid.New(),
		Name:           "Fixture 2",
		ProjectID:      project.ID,
		DefinitionID:   def.ID,
		Universe:       1,
		StartChannel:   5,
		LayoutX:        floatPtr(300.0),
		LayoutY:        floatPtr(400.0),
		LayoutRotation: nil,
	}
	db.Create(fixture2)

	// Capture original positions
	beforeSnapshot, _ := service.CaptureFixturePositions(ctx, []string{fixture1.ID, fixture2.ID})

	// Update positions
	fixture1.LayoutX = floatPtr(500.0)
	fixture1.LayoutY = floatPtr(600.0)
	fixture2.LayoutX = floatPtr(700.0)
	fixture2.LayoutY = floatPtr(800.0)
	db.Save(fixture1)
	db.Save(fixture2)

	// Record operation
	afterSnapshot, _ := service.CaptureFixturePositions(ctx, []string{fixture1.ID, fixture2.ID})
	relatedIDs := []string{fixture1.ID, fixture2.ID}
	err := service.RecordOperation(ctx, project.ID, OperationTypeBulk, EntityTypeBulkFixturePosition,
		"bulk-position-update", "Update fixture positions", beforeSnapshot, afterSnapshot, relatedIDs)
	if err != nil {
		t.Fatalf("RecordOperation failed: %v", err)
	}

	// Undo - should restore original positions
	_, err = service.Undo(ctx, project.ID)
	if err != nil {
		t.Fatalf("Undo failed: %v", err)
	}

	// Verify positions restored
	var restored1, restored2 models.FixtureInstance
	db.First(&restored1, "id = ?", fixture1.ID)
	db.First(&restored2, "id = ?", fixture2.ID)

	if restored1.LayoutX == nil || *restored1.LayoutX != 100.0 {
		t.Errorf("Expected fixture1 LayoutX 100.0, got %v", restored1.LayoutX)
	}
	if restored1.LayoutY == nil || *restored1.LayoutY != 200.0 {
		t.Errorf("Expected fixture1 LayoutY 200.0, got %v", restored1.LayoutY)
	}
	if restored2.LayoutX == nil || *restored2.LayoutX != 300.0 {
		t.Errorf("Expected fixture2 LayoutX 300.0, got %v", restored2.LayoutX)
	}
	if restored2.LayoutY == nil || *restored2.LayoutY != 400.0 {
		t.Errorf("Expected fixture2 LayoutY 400.0, got %v", restored2.LayoutY)
	}

	// Redo - should re-apply the updated positions
	_, err = service.Redo(ctx, project.ID)
	if err != nil {
		t.Fatalf("Redo failed: %v", err)
	}

	// Verify positions updated again
	db.First(&restored1, "id = ?", fixture1.ID)
	db.First(&restored2, "id = ?", fixture2.ID)

	if restored1.LayoutX == nil || *restored1.LayoutX != 500.0 {
		t.Errorf("Expected fixture1 LayoutX 500.0 after redo, got %v", restored1.LayoutX)
	}
	if restored2.LayoutX == nil || *restored2.LayoutX != 700.0 {
		t.Errorf("Expected fixture2 LayoutX 700.0 after redo, got %v", restored2.LayoutX)
	}
}

func TestService_ApplyBulkFixturePositionSnapshot_EmptySnapshot(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	ctx := context.Background()
	service := createTestService(t, db)

	// Apply empty snapshot - should not error
	entityID, err := service.applyBulkFixturePositionSnapshot(ctx, "")
	if err != nil {
		t.Fatalf("Expected no error for empty snapshot, got: %v", err)
	}
	if entityID != "" {
		t.Errorf("Expected empty entityID, got: %s", entityID)
	}
}

func TestService_ApplyBulkFixturePositionSnapshot_InvalidJSON(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	ctx := context.Background()
	service := createTestService(t, db)

	// Apply invalid JSON
	_, err := service.applyBulkFixturePositionSnapshot(ctx, "invalid json")
	if err == nil {
		t.Error("Expected error for invalid JSON")
	}
}

func TestService_ApplyBulkFixturePositionSnapshot_MissingFixture(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	ctx := context.Background()
	service := createTestService(t, db)

	// Create snapshot with non-existent fixture
	snapshot := BulkFixturePositionSnapshot{
		Positions: []FixturePositionEntry{
			{
				FixtureID:   "nonexistent-id",
				FixtureName: "Missing Fixture",
				LayoutX:     floatPtr(100.0),
				LayoutY:     floatPtr(200.0),
			},
		},
	}
	snapshotJSON, _ := json.Marshal(snapshot)

	// Apply snapshot - should not error (missing fixtures are logged and skipped)
	entityID, err := service.applyBulkFixturePositionSnapshot(ctx, string(snapshotJSON))
	if err != nil {
		t.Fatalf("Expected no error for missing fixture, got: %v", err)
	}
	if entityID != "" {
		t.Errorf("Expected empty entityID, got: %s", entityID)
	}
}
