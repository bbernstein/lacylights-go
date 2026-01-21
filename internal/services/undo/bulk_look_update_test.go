package undo

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/bbernstein/lacylights-go/internal/database/models"
	"github.com/lucsky/cuid"
)

func TestService_CaptureBulkLookState(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	ctx := context.Background()
	service := createTestService(t, db)
	project := createTestProject(t, db)

	// Create looks with fixture values
	look1 := &models.Look{
		ID:        cuid.New(),
		Name:      "Look 1",
		ProjectID: project.ID,
	}
	db.Create(look1)

	look2 := &models.Look{
		ID:        cuid.New(),
		Name:      "Look 2",
		ProjectID: project.ID,
	}
	db.Create(look2)

	// Create fixture definition and instance
	def := &models.FixtureDefinition{
		ID:           cuid.New(),
		Manufacturer: "Test",
		Model:        "TestModel",
		Type:         "LED_PAR",
	}
	db.Create(def)

	fixture := &models.FixtureInstance{
		ID:           cuid.New(),
		Name:         "Fixture 1",
		ProjectID:    project.ID,
		DefinitionID: def.ID,
		Universe:     1,
		StartChannel: 1,
	}
	db.Create(fixture)

	// Add fixture values to looks
	fv1 := &models.FixtureValue{
		ID:        cuid.New(),
		LookID:    look1.ID,
		FixtureID: fixture.ID,
		Channels:  `{"0":255,"1":128,"2":64}`,
	}
	db.Create(fv1)

	fv2 := &models.FixtureValue{
		ID:        cuid.New(),
		LookID:    look2.ID,
		FixtureID: fixture.ID,
		Channels:  `{"0":100,"1":50,"2":25}`,
	}
	db.Create(fv2)

	// Capture bulk look state
	snapshot, err := service.CaptureBulkLookState(ctx, []string{look1.ID, look2.ID})
	if err != nil {
		t.Fatalf("CaptureBulkLookState failed: %v", err)
	}

	if snapshot == nil {
		t.Fatal("Expected non-nil snapshot")
	}

	if len(snapshot.Looks) != 2 {
		t.Fatalf("Expected 2 looks, got %d", len(snapshot.Looks))
	}

	// Verify look1 data
	var look1Snapshot *LookSnapshot
	for i := range snapshot.Looks {
		if snapshot.Looks[i].Look != nil && snapshot.Looks[i].Look.ID == look1.ID {
			look1Snapshot = &snapshot.Looks[i]
			break
		}
	}
	if look1Snapshot == nil {
		t.Fatal("Look 1 not found in snapshot")
	}
	if look1Snapshot.Look.Name != "Look 1" {
		t.Errorf("Expected Look 1 name, got %s", look1Snapshot.Look.Name)
	}
	if len(look1Snapshot.FixtureValues) != 1 {
		t.Errorf("Expected 1 fixture value for look 1, got %d", len(look1Snapshot.FixtureValues))
	}
}

func TestService_CaptureBulkLookState_Empty(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	ctx := context.Background()
	service := createTestService(t, db)

	// Capture with empty list
	snapshot, err := service.CaptureBulkLookState(ctx, []string{})
	if err != nil {
		t.Fatalf("CaptureBulkLookState failed: %v", err)
	}

	if snapshot == nil {
		t.Fatal("Expected non-nil snapshot")
	}
	if len(snapshot.Looks) != 0 {
		t.Errorf("Expected 0 looks, got %d", len(snapshot.Looks))
	}
}

func TestService_CaptureBulkLookState_NotFound(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	ctx := context.Background()
	service := createTestService(t, db)

	// Capture with non-existent look
	snapshot, err := service.CaptureBulkLookState(ctx, []string{"nonexistent-id"})
	if err != nil {
		t.Fatalf("CaptureBulkLookState failed: %v", err)
	}

	// Should return empty since non-existent looks are skipped
	if len(snapshot.Looks) != 0 {
		t.Errorf("Expected 0 looks for non-existent ID, got %d", len(snapshot.Looks))
	}
}

func TestService_ApplyBulkLookUpdateSnapshot(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	ctx := context.Background()
	service := createTestService(t, db)
	project := createTestProject(t, db)

	// Create fixture definition and instance
	def := &models.FixtureDefinition{
		ID:           cuid.New(),
		Manufacturer: "Test",
		Model:        "TestModel",
		Type:         "LED_PAR",
	}
	db.Create(def)

	fixture := &models.FixtureInstance{
		ID:           cuid.New(),
		Name:         "Fixture 1",
		ProjectID:    project.ID,
		DefinitionID: def.ID,
		Universe:     1,
		StartChannel: 1,
	}
	db.Create(fixture)

	// Create looks with initial fixture values
	look1 := &models.Look{
		ID:        cuid.New(),
		Name:      "Look 1",
		ProjectID: project.ID,
	}
	db.Create(look1)

	look2 := &models.Look{
		ID:        cuid.New(),
		Name:      "Look 2",
		ProjectID: project.ID,
	}
	db.Create(look2)

	fv1 := &models.FixtureValue{
		ID:        cuid.New(),
		LookID:    look1.ID,
		FixtureID: fixture.ID,
		Channels:  `{"0":100}`,
	}
	db.Create(fv1)

	fv2 := &models.FixtureValue{
		ID:        cuid.New(),
		LookID:    look2.ID,
		FixtureID: fixture.ID,
		Channels:  `{"0":100}`,
	}
	db.Create(fv2)

	// Capture before state
	beforeSnapshot, _ := service.CaptureBulkLookState(ctx, []string{look1.ID, look2.ID})

	// Update fixture values
	fv1.Channels = `{"0":255}`
	fv2.Channels = `{"0":200}`
	db.Save(fv1)
	db.Save(fv2)

	// Capture after state
	afterSnapshot, _ := service.CaptureBulkLookState(ctx, []string{look1.ID, look2.ID})

	// Record operation
	err := service.RecordOperation(ctx, project.ID, OperationTypeBulk, EntityTypeBulkLookUpdate,
		"", "Update fixtures in 2 looks", beforeSnapshot, afterSnapshot, []string{look1.ID, look2.ID})
	if err != nil {
		t.Fatalf("RecordOperation failed: %v", err)
	}

	// Undo - should restore original fixture values
	_, err = service.Undo(ctx, project.ID)
	if err != nil {
		t.Fatalf("Undo failed: %v", err)
	}

	// Verify fixture values restored
	var restoredFv1, restoredFv2 models.FixtureValue
	db.First(&restoredFv1, "look_id = ? AND fixture_id = ?", look1.ID, fixture.ID)
	db.First(&restoredFv2, "look_id = ? AND fixture_id = ?", look2.ID, fixture.ID)

	if restoredFv1.Channels != `{"0":100}` {
		t.Errorf("Expected look1 fixture value to be restored, got %s", restoredFv1.Channels)
	}
	if restoredFv2.Channels != `{"0":100}` {
		t.Errorf("Expected look2 fixture value to be restored, got %s", restoredFv2.Channels)
	}

	// Redo - should re-apply the updates
	_, err = service.Redo(ctx, project.ID)
	if err != nil {
		t.Fatalf("Redo failed: %v", err)
	}

	// Verify fixture values updated again
	db.First(&restoredFv1, "look_id = ? AND fixture_id = ?", look1.ID, fixture.ID)
	db.First(&restoredFv2, "look_id = ? AND fixture_id = ?", look2.ID, fixture.ID)

	if restoredFv1.Channels != `{"0":255}` {
		t.Errorf("Expected look1 fixture value to be updated after redo, got %s", restoredFv1.Channels)
	}
	if restoredFv2.Channels != `{"0":200}` {
		t.Errorf("Expected look2 fixture value to be updated after redo, got %s", restoredFv2.Channels)
	}
}

func TestService_ApplyBulkLookUpdateSnapshot_EmptySnapshot(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	ctx := context.Background()
	service := createTestService(t, db)

	// Apply empty snapshot - should not error
	entityID, err := service.applyBulkLookUpdateSnapshot(ctx, "")
	if err != nil {
		t.Fatalf("Expected no error for empty snapshot, got: %v", err)
	}
	if entityID != "" {
		t.Errorf("Expected empty entityID, got: %s", entityID)
	}
}

func TestService_ApplyBulkLookUpdateSnapshot_InvalidJSON(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	ctx := context.Background()
	service := createTestService(t, db)

	// Apply invalid JSON
	_, err := service.applyBulkLookUpdateSnapshot(ctx, "invalid json")
	if err == nil {
		t.Error("Expected error for invalid JSON")
	}
}

func TestService_ApplyBulkLookUpdateSnapshot_WithNilLook(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	ctx := context.Background()
	service := createTestService(t, db)

	// Create snapshot with nil look (should be skipped)
	snapshot := BulkLookUpdateSnapshot{
		Looks: []LookSnapshot{
			{Look: nil, FixtureValues: nil},
		},
	}
	snapshotJSON, _ := json.Marshal(snapshot)

	// Apply snapshot - should succeed and skip nil looks
	entityID, err := service.applyBulkLookUpdateSnapshot(ctx, string(snapshotJSON))
	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}
	if entityID != "" {
		t.Errorf("Expected empty entityID for skipped looks, got: %s", entityID)
	}
}
