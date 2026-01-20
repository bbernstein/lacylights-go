package repositories

import (
	"context"
	"testing"

	"github.com/bbernstein/lacylights-go/internal/database/models"
	"github.com/glebarez/sqlite"
	"github.com/lucsky/cuid"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// setupOperationTestDB creates an in-memory SQLite database for testing operation repository.
func setupOperationTestDB(t *testing.T) (*gorm.DB, func()) {
	t.Helper()

	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		t.Fatalf("Failed to open in-memory database: %v", err)
	}

	err = db.AutoMigrate(
		&models.Project{},
		&models.Operation{},
		&models.OperationPointer{},
	)
	if err != nil {
		t.Fatalf("Failed to migrate database: %v", err)
	}

	cleanup := func() {
		sqlDB, err := db.DB()
		if err == nil {
			_ = sqlDB.Close()
		}
	}

	return db, cleanup
}

// createOperationTestProject creates a project for testing.
func createOperationTestProject(t *testing.T, db *gorm.DB) *models.Project {
	t.Helper()

	project := &models.Project{
		ID:   cuid.New(),
		Name: "Test Project " + cuid.Slug(),
	}
	if err := db.Create(project).Error; err != nil {
		t.Fatalf("Failed to create project: %v", err)
	}
	return project
}

func TestNewOperationRepository(t *testing.T) {
	db, cleanup := setupOperationTestDB(t)
	defer cleanup()

	repo := NewOperationRepository(db)
	if repo == nil {
		t.Fatal("Expected non-nil repository")
	}
	if repo.db != db {
		t.Error("Expected db to be set")
	}
	if repo.maxHistoryPerProject != DefaultMaxHistoryPerProject {
		t.Errorf("Expected maxHistoryPerProject=%d, got %d", DefaultMaxHistoryPerProject, repo.maxHistoryPerProject)
	}
}

func TestNewOperationRepositoryWithLimit(t *testing.T) {
	db, cleanup := setupOperationTestDB(t)
	defer cleanup()

	customLimit := 50
	repo := NewOperationRepositoryWithLimit(db, customLimit)
	if repo == nil {
		t.Fatal("Expected non-nil repository")
	}
	if repo.maxHistoryPerProject != customLimit {
		t.Errorf("Expected maxHistoryPerProject=%d, got %d", customLimit, repo.maxHistoryPerProject)
	}
}

func TestOperationRepository_RecordOperation(t *testing.T) {
	db, cleanup := setupOperationTestDB(t)
	defer cleanup()

	repo := NewOperationRepository(db)
	ctx := context.Background()
	project := createOperationTestProject(t, db)

	op := &models.Operation{
		ProjectID:     project.ID,
		OperationType: "CREATE",
		EntityType:    "Look",
		EntityID:      cuid.New(),
		Description:   "Created test look",
		PreviousState: "",
		NewState:      `{"look":{"id":"test"}}`,
	}

	err := repo.RecordOperation(ctx, op)
	if err != nil {
		t.Fatalf("RecordOperation failed: %v", err)
	}

	if op.ID == "" {
		t.Error("Expected operation ID to be set")
	}
	if op.Sequence != 1 {
		t.Errorf("Expected sequence=1, got %d", op.Sequence)
	}

	// Record another operation
	op2 := &models.Operation{
		ProjectID:     project.ID,
		OperationType: "UPDATE",
		EntityType:    "Look",
		EntityID:      op.EntityID,
		Description:   "Updated test look",
		PreviousState: `{"look":{"id":"test"}}`,
		NewState:      `{"look":{"id":"test","name":"updated"}}`,
	}

	err = repo.RecordOperation(ctx, op2)
	if err != nil {
		t.Fatalf("RecordOperation second failed: %v", err)
	}

	if op2.Sequence != 2 {
		t.Errorf("Expected sequence=2, got %d", op2.Sequence)
	}
}

func TestOperationRepository_RecordOperationWithRelatedIDs(t *testing.T) {
	db, cleanup := setupOperationTestDB(t)
	defer cleanup()

	repo := NewOperationRepository(db)
	ctx := context.Background()
	project := createOperationTestProject(t, db)

	relatedIDs := `["id1","id2","id3"]`
	op := &models.Operation{
		ProjectID:     project.ID,
		OperationType: "BULK",
		EntityType:    "Look",
		EntityID:      cuid.New(),
		Description:   "Bulk created looks",
		PreviousState: "",
		NewState:      `{}`,
		RelatedIDs:    &relatedIDs,
	}

	err := repo.RecordOperation(ctx, op)
	if err != nil {
		t.Fatalf("RecordOperation with related IDs failed: %v", err)
	}

	// Verify the operation was saved with related IDs
	found, err := repo.GetOperationByID(ctx, op.ID)
	if err != nil {
		t.Fatalf("GetOperationByID failed: %v", err)
	}
	if found.RelatedIDs == nil || *found.RelatedIDs != relatedIDs {
		t.Errorf("Expected related IDs to be saved")
	}
}

func TestOperationRepository_GetOperationAtSequence(t *testing.T) {
	db, cleanup := setupOperationTestDB(t)
	defer cleanup()

	repo := NewOperationRepository(db)
	ctx := context.Background()
	project := createOperationTestProject(t, db)

	// Record an operation
	op := &models.Operation{
		ProjectID:     project.ID,
		OperationType: "CREATE",
		EntityType:    "Look",
		EntityID:      cuid.New(),
		Description:   "Test operation",
	}
	err := repo.RecordOperation(ctx, op)
	if err != nil {
		t.Fatalf("RecordOperation failed: %v", err)
	}

	// Get operation at sequence 1
	found, err := repo.GetOperationAtSequence(ctx, project.ID, 1)
	if err != nil {
		t.Fatalf("GetOperationAtSequence failed: %v", err)
	}
	if found == nil {
		t.Fatal("Expected to find operation")
	}
	if found.ID != op.ID {
		t.Errorf("Expected operation ID=%s, got %s", op.ID, found.ID)
	}

	// Get operation at non-existent sequence
	notFound, err := repo.GetOperationAtSequence(ctx, project.ID, 999)
	if err != nil {
		t.Fatalf("GetOperationAtSequence for non-existent failed: %v", err)
	}
	if notFound != nil {
		t.Error("Expected nil for non-existent sequence")
	}
}

func TestOperationRepository_GetOperationByID(t *testing.T) {
	db, cleanup := setupOperationTestDB(t)
	defer cleanup()

	repo := NewOperationRepository(db)
	ctx := context.Background()
	project := createOperationTestProject(t, db)

	// Record an operation
	op := &models.Operation{
		ProjectID:     project.ID,
		OperationType: "CREATE",
		EntityType:    "Look",
		EntityID:      cuid.New(),
		Description:   "Test operation",
	}
	err := repo.RecordOperation(ctx, op)
	if err != nil {
		t.Fatalf("RecordOperation failed: %v", err)
	}

	// Get by ID
	found, err := repo.GetOperationByID(ctx, op.ID)
	if err != nil {
		t.Fatalf("GetOperationByID failed: %v", err)
	}
	if found == nil {
		t.Fatal("Expected to find operation")
	}
	if found.Description != op.Description {
		t.Errorf("Description mismatch: got %s, want %s", found.Description, op.Description)
	}

	// Get non-existent
	notFound, err := repo.GetOperationByID(ctx, "nonexistent")
	if err != nil {
		t.Fatalf("GetOperationByID for non-existent failed: %v", err)
	}
	if notFound != nil {
		t.Error("Expected nil for non-existent ID")
	}
}

func TestOperationRepository_GetOperationsByProject(t *testing.T) {
	db, cleanup := setupOperationTestDB(t)
	defer cleanup()

	repo := NewOperationRepository(db)
	ctx := context.Background()
	project := createOperationTestProject(t, db)

	// Record multiple operations
	for i := 0; i < 5; i++ {
		op := &models.Operation{
			ProjectID:     project.ID,
			OperationType: "CREATE",
			EntityType:    "Look",
			EntityID:      cuid.New(),
			Description:   "Test operation",
		}
		err := repo.RecordOperation(ctx, op)
		if err != nil {
			t.Fatalf("RecordOperation %d failed: %v", i, err)
		}
	}

	// Get all operations
	ops, total, err := repo.GetOperationsByProject(ctx, project.ID, 1, 10)
	if err != nil {
		t.Fatalf("GetOperationsByProject failed: %v", err)
	}
	if total != 5 {
		t.Errorf("Expected total=5, got %d", total)
	}
	if len(ops) != 5 {
		t.Errorf("Expected 5 operations, got %d", len(ops))
	}

	// Verify ordering (most recent first)
	if ops[0].Sequence != 5 {
		t.Errorf("Expected first operation sequence=5, got %d", ops[0].Sequence)
	}

	// Test pagination
	opsPage1, _, err := repo.GetOperationsByProject(ctx, project.ID, 1, 2)
	if err != nil {
		t.Fatalf("GetOperationsByProject page 1 failed: %v", err)
	}
	if len(opsPage1) != 2 {
		t.Errorf("Expected 2 operations on page 1, got %d", len(opsPage1))
	}

	opsPage2, _, err := repo.GetOperationsByProject(ctx, project.ID, 2, 2)
	if err != nil {
		t.Fatalf("GetOperationsByProject page 2 failed: %v", err)
	}
	if len(opsPage2) != 2 {
		t.Errorf("Expected 2 operations on page 2, got %d", len(opsPage2))
	}
}

func TestOperationRepository_GetPointer(t *testing.T) {
	db, cleanup := setupOperationTestDB(t)
	defer cleanup()

	repo := NewOperationRepository(db)
	ctx := context.Background()
	project := createOperationTestProject(t, db)

	// No pointer yet
	pointer, err := repo.GetPointer(ctx, project.ID)
	if err != nil {
		t.Fatalf("GetPointer failed: %v", err)
	}
	if pointer != nil {
		t.Error("Expected nil pointer before any operations")
	}

	// Record an operation (creates pointer)
	op := &models.Operation{
		ProjectID:     project.ID,
		OperationType: "CREATE",
		EntityType:    "Look",
		EntityID:      cuid.New(),
		Description:   "Test operation",
	}
	err = repo.RecordOperation(ctx, op)
	if err != nil {
		t.Fatalf("RecordOperation failed: %v", err)
	}

	// Now pointer should exist
	pointer, err = repo.GetPointer(ctx, project.ID)
	if err != nil {
		t.Fatalf("GetPointer after record failed: %v", err)
	}
	if pointer == nil {
		t.Fatal("Expected non-nil pointer after operation")
	}
	if pointer.CurrentSequence != 1 {
		t.Errorf("Expected CurrentSequence=1, got %d", pointer.CurrentSequence)
	}
	if pointer.MaxSequence != 1 {
		t.Errorf("Expected MaxSequence=1, got %d", pointer.MaxSequence)
	}
}

func TestOperationRepository_UpdatePointer(t *testing.T) {
	db, cleanup := setupOperationTestDB(t)
	defer cleanup()

	repo := NewOperationRepository(db)
	ctx := context.Background()
	project := createOperationTestProject(t, db)

	// Record operations first
	for i := 0; i < 3; i++ {
		op := &models.Operation{
			ProjectID:     project.ID,
			OperationType: "CREATE",
			EntityType:    "Look",
			EntityID:      cuid.New(),
			Description:   "Test operation",
		}
		err := repo.RecordOperation(ctx, op)
		if err != nil {
			t.Fatalf("RecordOperation %d failed: %v", i, err)
		}
	}

	// Update pointer to sequence 1
	err := repo.UpdatePointer(ctx, project.ID, 1)
	if err != nil {
		t.Fatalf("UpdatePointer failed: %v", err)
	}

	pointer, err := repo.GetPointer(ctx, project.ID)
	if err != nil {
		t.Fatalf("GetPointer failed: %v", err)
	}
	if pointer.CurrentSequence != 1 {
		t.Errorf("Expected CurrentSequence=1 after update, got %d", pointer.CurrentSequence)
	}
}

func TestOperationRepository_TruncateAfterSequence(t *testing.T) {
	db, cleanup := setupOperationTestDB(t)
	defer cleanup()

	repo := NewOperationRepository(db)
	ctx := context.Background()
	project := createOperationTestProject(t, db)

	// Record multiple operations
	for i := 0; i < 5; i++ {
		op := &models.Operation{
			ProjectID:     project.ID,
			OperationType: "CREATE",
			EntityType:    "Look",
			EntityID:      cuid.New(),
			Description:   "Test operation",
		}
		err := repo.RecordOperation(ctx, op)
		if err != nil {
			t.Fatalf("RecordOperation %d failed: %v", i, err)
		}
	}

	// Truncate after sequence 2
	err := repo.TruncateAfterSequence(ctx, project.ID, 2)
	if err != nil {
		t.Fatalf("TruncateAfterSequence failed: %v", err)
	}

	// Verify only 2 operations remain
	ops, total, err := repo.GetOperationsByProject(ctx, project.ID, 1, 10)
	if err != nil {
		t.Fatalf("GetOperationsByProject failed: %v", err)
	}
	if total != 2 {
		t.Errorf("Expected 2 operations after truncate, got %d", total)
	}
	if len(ops) != 2 {
		t.Errorf("Expected 2 operations, got %d", len(ops))
	}
}

func TestOperationRepository_ClearHistory(t *testing.T) {
	db, cleanup := setupOperationTestDB(t)
	defer cleanup()

	repo := NewOperationRepository(db)
	ctx := context.Background()
	project := createOperationTestProject(t, db)

	// Record operations
	for i := 0; i < 3; i++ {
		op := &models.Operation{
			ProjectID:     project.ID,
			OperationType: "CREATE",
			EntityType:    "Look",
			EntityID:      cuid.New(),
			Description:   "Test operation",
		}
		err := repo.RecordOperation(ctx, op)
		if err != nil {
			t.Fatalf("RecordOperation %d failed: %v", i, err)
		}
	}

	// Clear history
	err := repo.ClearHistory(ctx, project.ID)
	if err != nil {
		t.Fatalf("ClearHistory failed: %v", err)
	}

	// Verify history is empty
	ops, total, err := repo.GetOperationsByProject(ctx, project.ID, 1, 10)
	if err != nil {
		t.Fatalf("GetOperationsByProject failed: %v", err)
	}
	if total != 0 {
		t.Errorf("Expected 0 operations after clear, got %d", total)
	}
	if len(ops) != 0 {
		t.Errorf("Expected empty operations slice, got %d", len(ops))
	}

	// Verify pointer is gone
	pointer, err := repo.GetPointer(ctx, project.ID)
	if err != nil {
		t.Fatalf("GetPointer failed: %v", err)
	}
	if pointer != nil {
		t.Error("Expected nil pointer after clear")
	}
}

func TestOperationRepository_DeleteByProjectID(t *testing.T) {
	db, cleanup := setupOperationTestDB(t)
	defer cleanup()

	repo := NewOperationRepository(db)
	ctx := context.Background()
	project := createOperationTestProject(t, db)

	// Record operations
	op := &models.Operation{
		ProjectID:     project.ID,
		OperationType: "CREATE",
		EntityType:    "Look",
		EntityID:      cuid.New(),
		Description:   "Test operation",
	}
	err := repo.RecordOperation(ctx, op)
	if err != nil {
		t.Fatalf("RecordOperation failed: %v", err)
	}

	// Delete by project ID
	err = repo.DeleteByProjectID(ctx, project.ID)
	if err != nil {
		t.Fatalf("DeleteByProjectID failed: %v", err)
	}

	// Verify history is empty
	_, total, err := repo.GetOperationsByProject(ctx, project.ID, 1, 10)
	if err != nil {
		t.Fatalf("GetOperationsByProject failed: %v", err)
	}
	if total != 0 {
		t.Errorf("Expected 0 operations after delete, got %d", total)
	}
}

func TestOperationRepository_Undo(t *testing.T) {
	db, cleanup := setupOperationTestDB(t)
	defer cleanup()

	repo := NewOperationRepository(db)
	ctx := context.Background()
	project := createOperationTestProject(t, db)

	// Undo with no history
	result, err := repo.Undo(ctx, project.ID)
	if err != nil {
		t.Fatalf("Undo with no history failed: %v", err)
	}
	if result != nil {
		t.Error("Expected nil result for undo with no history")
	}

	// Record operations
	op1 := &models.Operation{
		ProjectID:     project.ID,
		OperationType: "CREATE",
		EntityType:    "Look",
		EntityID:      cuid.New(),
		Description:   "Created look 1",
	}
	err = repo.RecordOperation(ctx, op1)
	if err != nil {
		t.Fatalf("RecordOperation 1 failed: %v", err)
	}

	op2 := &models.Operation{
		ProjectID:     project.ID,
		OperationType: "CREATE",
		EntityType:    "Look",
		EntityID:      cuid.New(),
		Description:   "Created look 2",
	}
	err = repo.RecordOperation(ctx, op2)
	if err != nil {
		t.Fatalf("RecordOperation 2 failed: %v", err)
	}

	// Undo should return op2 (most recent)
	result, err = repo.Undo(ctx, project.ID)
	if err != nil {
		t.Fatalf("Undo failed: %v", err)
	}
	if result == nil {
		t.Fatal("Expected non-nil result for undo")
	}
	if result.ID != op2.ID {
		t.Errorf("Expected to undo op2 (ID=%s), got ID=%s", op2.ID, result.ID)
	}

	// Verify pointer moved
	pointer, _ := repo.GetPointer(ctx, project.ID)
	if pointer.CurrentSequence != 1 {
		t.Errorf("Expected CurrentSequence=1 after undo, got %d", pointer.CurrentSequence)
	}

	// Undo again should return op1
	result, err = repo.Undo(ctx, project.ID)
	if err != nil {
		t.Fatalf("Undo second failed: %v", err)
	}
	if result == nil {
		t.Fatal("Expected non-nil result for second undo")
	}
	if result.ID != op1.ID {
		t.Errorf("Expected to undo op1 (ID=%s), got ID=%s", op1.ID, result.ID)
	}

	// Undo with nothing left should return nil
	result, err = repo.Undo(ctx, project.ID)
	if err != nil {
		t.Fatalf("Undo with nothing left failed: %v", err)
	}
	if result != nil {
		t.Error("Expected nil result when nothing left to undo")
	}
}

func TestOperationRepository_Redo(t *testing.T) {
	db, cleanup := setupOperationTestDB(t)
	defer cleanup()

	repo := NewOperationRepository(db)
	ctx := context.Background()
	project := createOperationTestProject(t, db)

	// Redo with no history
	result, err := repo.Redo(ctx, project.ID)
	if err != nil {
		t.Fatalf("Redo with no history failed: %v", err)
	}
	if result != nil {
		t.Error("Expected nil result for redo with no history")
	}

	// Record operations
	op1 := &models.Operation{
		ProjectID:     project.ID,
		OperationType: "CREATE",
		EntityType:    "Look",
		EntityID:      cuid.New(),
		Description:   "Created look 1",
	}
	err = repo.RecordOperation(ctx, op1)
	if err != nil {
		t.Fatalf("RecordOperation 1 failed: %v", err)
	}

	op2 := &models.Operation{
		ProjectID:     project.ID,
		OperationType: "CREATE",
		EntityType:    "Look",
		EntityID:      cuid.New(),
		Description:   "Created look 2",
	}
	err = repo.RecordOperation(ctx, op2)
	if err != nil {
		t.Fatalf("RecordOperation 2 failed: %v", err)
	}

	// Redo with nothing to redo (at max sequence)
	result, err = repo.Redo(ctx, project.ID)
	if err != nil {
		t.Fatalf("Redo at max sequence failed: %v", err)
	}
	if result != nil {
		t.Error("Expected nil result for redo at max sequence")
	}

	// Undo both operations
	_, _ = repo.Undo(ctx, project.ID)
	_, _ = repo.Undo(ctx, project.ID)

	// Redo should return op1
	result, err = repo.Redo(ctx, project.ID)
	if err != nil {
		t.Fatalf("Redo failed: %v", err)
	}
	if result == nil {
		t.Fatal("Expected non-nil result for redo")
	}
	if result.ID != op1.ID {
		t.Errorf("Expected to redo op1 (ID=%s), got ID=%s", op1.ID, result.ID)
	}

	// Redo again should return op2
	result, err = repo.Redo(ctx, project.ID)
	if err != nil {
		t.Fatalf("Redo second failed: %v", err)
	}
	if result == nil {
		t.Fatal("Expected non-nil result for second redo")
	}
	if result.ID != op2.ID {
		t.Errorf("Expected to redo op2 (ID=%s), got ID=%s", op2.ID, result.ID)
	}
}

func TestOperationRepository_GetUndoRedoStatus(t *testing.T) {
	db, cleanup := setupOperationTestDB(t)
	defer cleanup()

	repo := NewOperationRepository(db)
	ctx := context.Background()
	project := createOperationTestProject(t, db)

	// Status with no history
	status, err := repo.GetUndoRedoStatus(ctx, project.ID)
	if err != nil {
		t.Fatalf("GetUndoRedoStatus failed: %v", err)
	}
	if status.CanUndo {
		t.Error("Expected CanUndo=false with no history")
	}
	if status.CanRedo {
		t.Error("Expected CanRedo=false with no history")
	}

	// Record operations
	op1 := &models.Operation{
		ProjectID:     project.ID,
		OperationType: "CREATE",
		EntityType:    "Look",
		EntityID:      cuid.New(),
		Description:   "Created look 1",
	}
	err = repo.RecordOperation(ctx, op1)
	if err != nil {
		t.Fatalf("RecordOperation 1 failed: %v", err)
	}

	op2 := &models.Operation{
		ProjectID:     project.ID,
		OperationType: "UPDATE",
		EntityType:    "Look",
		EntityID:      op1.EntityID,
		Description:   "Updated look",
	}
	err = repo.RecordOperation(ctx, op2)
	if err != nil {
		t.Fatalf("RecordOperation 2 failed: %v", err)
	}

	// Status after recording
	status, err = repo.GetUndoRedoStatus(ctx, project.ID)
	if err != nil {
		t.Fatalf("GetUndoRedoStatus failed: %v", err)
	}
	if !status.CanUndo {
		t.Error("Expected CanUndo=true after recording operations")
	}
	if status.CanRedo {
		t.Error("Expected CanRedo=false at max sequence")
	}
	if status.UndoDescription != "Updated look" {
		t.Errorf("Expected UndoDescription='Updated look', got '%s'", status.UndoDescription)
	}
	if status.TotalOperations != 2 {
		t.Errorf("Expected TotalOperations=2, got %d", status.TotalOperations)
	}

	// Undo one operation
	_, _ = repo.Undo(ctx, project.ID)

	// Status after undo
	status, err = repo.GetUndoRedoStatus(ctx, project.ID)
	if err != nil {
		t.Fatalf("GetUndoRedoStatus after undo failed: %v", err)
	}
	if !status.CanUndo {
		t.Error("Expected CanUndo=true (still have op1 to undo)")
	}
	if !status.CanRedo {
		t.Error("Expected CanRedo=true after undo")
	}
	if status.UndoDescription != "Created look 1" {
		t.Errorf("Expected UndoDescription='Created look 1', got '%s'", status.UndoDescription)
	}
	if status.RedoDescription != "Updated look" {
		t.Errorf("Expected RedoDescription='Updated look', got '%s'", status.RedoDescription)
	}
}

func TestOperationRepository_ForkTimeline(t *testing.T) {
	db, cleanup := setupOperationTestDB(t)
	defer cleanup()

	repo := NewOperationRepository(db)
	ctx := context.Background()
	project := createOperationTestProject(t, db)

	// Record 3 operations
	for i := 1; i <= 3; i++ {
		op := &models.Operation{
			ProjectID:     project.ID,
			OperationType: "CREATE",
			EntityType:    "Look",
			EntityID:      cuid.New(),
			Description:   "Operation",
		}
		err := repo.RecordOperation(ctx, op)
		if err != nil {
			t.Fatalf("RecordOperation %d failed: %v", i, err)
		}
	}

	// Undo twice (pointer at sequence 1)
	_, _ = repo.Undo(ctx, project.ID)
	_, _ = repo.Undo(ctx, project.ID)

	// Record a new operation (should fork timeline)
	opNew := &models.Operation{
		ProjectID:     project.ID,
		OperationType: "CREATE",
		EntityType:    "Cue",
		EntityID:      cuid.New(),
		Description:   "New operation",
	}
	err := repo.RecordOperation(ctx, opNew)
	if err != nil {
		t.Fatalf("RecordOperation new failed: %v", err)
	}

	// Should now have only 2 operations (1 original + 1 new)
	ops, total, err := repo.GetOperationsByProject(ctx, project.ID, 1, 10)
	if err != nil {
		t.Fatalf("GetOperationsByProject failed: %v", err)
	}
	if total != 2 {
		t.Errorf("Expected 2 operations after fork, got %d", total)
	}
	if len(ops) != 2 {
		t.Errorf("Expected 2 operations, got %d", len(ops))
	}

	// Verify new operation has sequence 2
	if opNew.Sequence != 2 {
		t.Errorf("Expected new operation sequence=2, got %d", opNew.Sequence)
	}
}

func TestOperationRepository_PruneOldOperations(t *testing.T) {
	db, cleanup := setupOperationTestDB(t)
	defer cleanup()

	// Create repo with small limit
	limit := 5
	repo := NewOperationRepositoryWithLimit(db, limit)
	ctx := context.Background()
	project := createOperationTestProject(t, db)

	// Record more operations than the limit
	for i := 1; i <= 10; i++ {
		op := &models.Operation{
			ProjectID:     project.ID,
			OperationType: "CREATE",
			EntityType:    "Look",
			EntityID:      cuid.New(),
			Description:   "Operation",
		}
		err := repo.RecordOperation(ctx, op)
		if err != nil {
			t.Fatalf("RecordOperation %d failed: %v", i, err)
		}
	}

	// Should have only `limit` operations
	ops, total, err := repo.GetOperationsByProject(ctx, project.ID, 1, 20)
	if err != nil {
		t.Fatalf("GetOperationsByProject failed: %v", err)
	}
	if total != int64(limit) {
		t.Errorf("Expected %d operations after pruning, got %d", limit, total)
	}
	if len(ops) != limit {
		t.Errorf("Expected %d operations, got %d", limit, len(ops))
	}

	// Verify we kept the most recent operations (sequences 6-10)
	if ops[0].Sequence != 10 {
		t.Errorf("Expected most recent operation sequence=10, got %d", ops[0].Sequence)
	}
	if ops[len(ops)-1].Sequence != 6 {
		t.Errorf("Expected oldest kept operation sequence=6, got %d", ops[len(ops)-1].Sequence)
	}
}

func TestMarshalState(t *testing.T) {
	// Test with nil
	result, err := MarshalState(nil)
	if err != nil {
		t.Fatalf("MarshalState(nil) failed: %v", err)
	}
	if result != "" {
		t.Errorf("Expected empty string for nil, got '%s'", result)
	}

	// Test with struct
	type testStruct struct {
		Name  string `json:"name"`
		Value int    `json:"value"`
	}
	data := testStruct{Name: "test", Value: 42}
	result, err = MarshalState(data)
	if err != nil {
		t.Fatalf("MarshalState failed: %v", err)
	}
	expected := `{"name":"test","value":42}`
	if result != expected {
		t.Errorf("Expected '%s', got '%s'", expected, result)
	}
}

func TestUnmarshalState(t *testing.T) {
	// Test with empty string
	var result struct{}
	err := UnmarshalState("", &result)
	if err != nil {
		t.Fatalf("UnmarshalState('') failed: %v", err)
	}

	// Test with valid JSON
	type testStruct struct {
		Name  string `json:"name"`
		Value int    `json:"value"`
	}
	var data testStruct
	err = UnmarshalState(`{"name":"test","value":42}`, &data)
	if err != nil {
		t.Fatalf("UnmarshalState failed: %v", err)
	}
	if data.Name != "test" {
		t.Errorf("Expected Name='test', got '%s'", data.Name)
	}
	if data.Value != 42 {
		t.Errorf("Expected Value=42, got %d", data.Value)
	}
}

func TestOperationRepository_GetOperationForUndo(t *testing.T) {
	db, cleanup := setupOperationTestDB(t)
	defer cleanup()

	repo := NewOperationRepository(db)
	ctx := context.Background()
	project := createOperationTestProject(t, db)

	// No operation to undo
	op, err := repo.GetOperationForUndo(ctx, project.ID)
	if err != nil {
		t.Fatalf("GetOperationForUndo with no history failed: %v", err)
	}
	if op != nil {
		t.Error("Expected nil operation when nothing to undo")
	}

	// Record operations
	op1 := &models.Operation{
		ProjectID:     project.ID,
		OperationType: "CREATE",
		EntityType:    "Look",
		EntityID:      cuid.New(),
		Description:   "Created look 1",
	}
	if err := repo.RecordOperation(ctx, op1); err != nil {
		t.Fatalf("RecordOperation 1 failed: %v", err)
	}

	op2 := &models.Operation{
		ProjectID:     project.ID,
		OperationType: "UPDATE",
		EntityType:    "Look",
		EntityID:      op1.EntityID,
		Description:   "Updated look",
	}
	if err := repo.RecordOperation(ctx, op2); err != nil {
		t.Fatalf("RecordOperation 2 failed: %v", err)
	}

	// Get operation for undo - should return op2 WITHOUT modifying pointer
	op, err = repo.GetOperationForUndo(ctx, project.ID)
	if err != nil {
		t.Fatalf("GetOperationForUndo failed: %v", err)
	}
	if op == nil {
		t.Fatal("Expected non-nil operation for undo")
	}
	if op.ID != op2.ID {
		t.Errorf("Expected op2 (ID=%s), got ID=%s", op2.ID, op.ID)
	}

	// Verify pointer was NOT modified
	pointer, _ := repo.GetPointer(ctx, project.ID)
	if pointer.CurrentSequence != 2 {
		t.Errorf("Pointer should still be at sequence 2, got %d", pointer.CurrentSequence)
	}

	// Call again - should still return op2 (pointer unchanged)
	op, err = repo.GetOperationForUndo(ctx, project.ID)
	if err != nil {
		t.Fatalf("Second GetOperationForUndo failed: %v", err)
	}
	if op.ID != op2.ID {
		t.Errorf("Second call should also return op2 (ID=%s), got ID=%s", op2.ID, op.ID)
	}
}

func TestOperationRepository_GetOperationForRedo(t *testing.T) {
	db, cleanup := setupOperationTestDB(t)
	defer cleanup()

	repo := NewOperationRepository(db)
	ctx := context.Background()
	project := createOperationTestProject(t, db)

	// No operation to redo
	op, err := repo.GetOperationForRedo(ctx, project.ID)
	if err != nil {
		t.Fatalf("GetOperationForRedo with no history failed: %v", err)
	}
	if op != nil {
		t.Error("Expected nil operation when nothing to redo")
	}

	// Record operations
	op1 := &models.Operation{
		ProjectID:     project.ID,
		OperationType: "CREATE",
		EntityType:    "Look",
		EntityID:      cuid.New(),
		Description:   "Created look 1",
	}
	if err := repo.RecordOperation(ctx, op1); err != nil {
		t.Fatalf("RecordOperation 1 failed: %v", err)
	}

	op2 := &models.Operation{
		ProjectID:     project.ID,
		OperationType: "UPDATE",
		EntityType:    "Look",
		EntityID:      op1.EntityID,
		Description:   "Updated look",
	}
	if err := repo.RecordOperation(ctx, op2); err != nil {
		t.Fatalf("RecordOperation 2 failed: %v", err)
	}

	// Nothing to redo when at max sequence
	op, err = repo.GetOperationForRedo(ctx, project.ID)
	if err != nil {
		t.Fatalf("GetOperationForRedo at max failed: %v", err)
	}
	if op != nil {
		t.Error("Expected nil operation when at max sequence")
	}

	// Undo to create redo possibility (using deprecated Undo to set up state)
	_, _ = repo.Undo(ctx, project.ID)
	_, _ = repo.Undo(ctx, project.ID)

	// Get operation for redo - should return op1 WITHOUT modifying pointer
	op, err = repo.GetOperationForRedo(ctx, project.ID)
	if err != nil {
		t.Fatalf("GetOperationForRedo failed: %v", err)
	}
	if op == nil {
		t.Fatal("Expected non-nil operation for redo")
	}
	if op.ID != op1.ID {
		t.Errorf("Expected op1 (ID=%s), got ID=%s", op1.ID, op.ID)
	}

	// Verify pointer was NOT modified
	pointer, _ := repo.GetPointer(ctx, project.ID)
	if pointer.CurrentSequence != 0 {
		t.Errorf("Pointer should still be at sequence 0, got %d", pointer.CurrentSequence)
	}
}

func TestOperationRepository_ConfirmUndo(t *testing.T) {
	db, cleanup := setupOperationTestDB(t)
	defer cleanup()

	repo := NewOperationRepository(db)
	ctx := context.Background()
	project := createOperationTestProject(t, db)

	// Record operations
	for i := 1; i <= 3; i++ {
		op := &models.Operation{
			ProjectID:     project.ID,
			OperationType: "CREATE",
			EntityType:    "Look",
			EntityID:      cuid.New(),
			Description:   "Test operation",
		}
		if err := repo.RecordOperation(ctx, op); err != nil {
			t.Fatalf("RecordOperation %d failed: %v", i, err)
		}
	}

	// Pointer at sequence 3
	pointer, _ := repo.GetPointer(ctx, project.ID)
	if pointer.CurrentSequence != 3 {
		t.Fatalf("Expected CurrentSequence=3, got %d", pointer.CurrentSequence)
	}

	// ConfirmUndo decrements pointer
	if err := repo.ConfirmUndo(ctx, project.ID); err != nil {
		t.Fatalf("ConfirmUndo failed: %v", err)
	}

	pointer, _ = repo.GetPointer(ctx, project.ID)
	if pointer.CurrentSequence != 2 {
		t.Errorf("Expected CurrentSequence=2 after ConfirmUndo, got %d", pointer.CurrentSequence)
	}

	// ConfirmUndo again
	if err := repo.ConfirmUndo(ctx, project.ID); err != nil {
		t.Fatalf("Second ConfirmUndo failed: %v", err)
	}

	pointer, _ = repo.GetPointer(ctx, project.ID)
	if pointer.CurrentSequence != 1 {
		t.Errorf("Expected CurrentSequence=1 after second ConfirmUndo, got %d", pointer.CurrentSequence)
	}

	// ConfirmUndo at sequence 1 - should go to 0
	if err := repo.ConfirmUndo(ctx, project.ID); err != nil {
		t.Fatalf("Third ConfirmUndo failed: %v", err)
	}

	pointer, _ = repo.GetPointer(ctx, project.ID)
	if pointer.CurrentSequence != 0 {
		t.Errorf("Expected CurrentSequence=0 after third ConfirmUndo, got %d", pointer.CurrentSequence)
	}

	// ConfirmUndo at sequence 0 - should stay at 0
	if err := repo.ConfirmUndo(ctx, project.ID); err != nil {
		t.Fatalf("Fourth ConfirmUndo failed: %v", err)
	}

	pointer, _ = repo.GetPointer(ctx, project.ID)
	if pointer.CurrentSequence != 0 {
		t.Errorf("Expected CurrentSequence=0 (unchanged), got %d", pointer.CurrentSequence)
	}
}

func TestOperationRepository_ConfirmRedo(t *testing.T) {
	db, cleanup := setupOperationTestDB(t)
	defer cleanup()

	repo := NewOperationRepository(db)
	ctx := context.Background()
	project := createOperationTestProject(t, db)

	// Record operations
	for i := 1; i <= 3; i++ {
		op := &models.Operation{
			ProjectID:     project.ID,
			OperationType: "CREATE",
			EntityType:    "Look",
			EntityID:      cuid.New(),
			Description:   "Test operation",
		}
		if err := repo.RecordOperation(ctx, op); err != nil {
			t.Fatalf("RecordOperation %d failed: %v", i, err)
		}
	}

	// Undo all operations (using deprecated Undo to set up state)
	_, _ = repo.Undo(ctx, project.ID)
	_, _ = repo.Undo(ctx, project.ID)
	_, _ = repo.Undo(ctx, project.ID)

	// Pointer at sequence 0
	pointer, _ := repo.GetPointer(ctx, project.ID)
	if pointer.CurrentSequence != 0 {
		t.Fatalf("Expected CurrentSequence=0, got %d", pointer.CurrentSequence)
	}

	// ConfirmRedo increments pointer
	if err := repo.ConfirmRedo(ctx, project.ID); err != nil {
		t.Fatalf("ConfirmRedo failed: %v", err)
	}

	pointer, _ = repo.GetPointer(ctx, project.ID)
	if pointer.CurrentSequence != 1 {
		t.Errorf("Expected CurrentSequence=1 after ConfirmRedo, got %d", pointer.CurrentSequence)
	}

	// ConfirmRedo again
	if err := repo.ConfirmRedo(ctx, project.ID); err != nil {
		t.Fatalf("Second ConfirmRedo failed: %v", err)
	}

	pointer, _ = repo.GetPointer(ctx, project.ID)
	if pointer.CurrentSequence != 2 {
		t.Errorf("Expected CurrentSequence=2 after second ConfirmRedo, got %d", pointer.CurrentSequence)
	}

	// ConfirmRedo to max
	if err := repo.ConfirmRedo(ctx, project.ID); err != nil {
		t.Fatalf("Third ConfirmRedo failed: %v", err)
	}

	pointer, _ = repo.GetPointer(ctx, project.ID)
	if pointer.CurrentSequence != 3 {
		t.Errorf("Expected CurrentSequence=3 after third ConfirmRedo, got %d", pointer.CurrentSequence)
	}

	// ConfirmRedo at max sequence - should stay at 3
	if err := repo.ConfirmRedo(ctx, project.ID); err != nil {
		t.Fatalf("Fourth ConfirmRedo failed: %v", err)
	}

	pointer, _ = repo.GetPointer(ctx, project.ID)
	if pointer.CurrentSequence != 3 {
		t.Errorf("Expected CurrentSequence=3 (unchanged), got %d", pointer.CurrentSequence)
	}
}

func TestOperationRepository_AtomicUndoRedo_PointerNotModifiedOnReadOnly(t *testing.T) {
	// This test verifies that the new atomic methods don't modify the pointer
	// until explicitly confirmed - the key fix for transaction atomicity
	db, cleanup := setupOperationTestDB(t)
	defer cleanup()

	repo := NewOperationRepository(db)
	ctx := context.Background()
	project := createOperationTestProject(t, db)

	// Record operations
	for i := 1; i <= 3; i++ {
		op := &models.Operation{
			ProjectID:     project.ID,
			OperationType: "CREATE",
			EntityType:    "Look",
			EntityID:      cuid.New(),
			Description:   "Test operation",
		}
		if err := repo.RecordOperation(ctx, op); err != nil {
			t.Fatalf("RecordOperation %d failed: %v", i, err)
		}
	}

	initialPointer, _ := repo.GetPointer(ctx, project.ID)
	initialSeq := initialPointer.CurrentSequence

	// Call GetOperationForUndo multiple times - pointer should NOT change
	for i := 0; i < 5; i++ {
		_, _ = repo.GetOperationForUndo(ctx, project.ID)
		pointer, _ := repo.GetPointer(ctx, project.ID)
		if pointer.CurrentSequence != initialSeq {
			t.Errorf("GetOperationForUndo modified pointer (iteration %d): expected %d, got %d",
				i, initialSeq, pointer.CurrentSequence)
		}
	}

	// Undo to create redo possibility (using deprecated Undo)
	_, _ = repo.Undo(ctx, project.ID)
	_, _ = repo.Undo(ctx, project.ID)

	newPointer, _ := repo.GetPointer(ctx, project.ID)
	newSeq := newPointer.CurrentSequence

	// Call GetOperationForRedo multiple times - pointer should NOT change
	for i := 0; i < 5; i++ {
		_, _ = repo.GetOperationForRedo(ctx, project.ID)
		pointer, _ := repo.GetPointer(ctx, project.ID)
		if pointer.CurrentSequence != newSeq {
			t.Errorf("GetOperationForRedo modified pointer (iteration %d): expected %d, got %d",
				i, newSeq, pointer.CurrentSequence)
		}
	}
}
