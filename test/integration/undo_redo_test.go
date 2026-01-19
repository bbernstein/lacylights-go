// Package integration contains integration tests for the LacyLights system.
package integration

import (
	"context"
	"testing"

	"github.com/bbernstein/lacylights-go/internal/database/models"
	"github.com/bbernstein/lacylights-go/internal/database/repositories"
	"github.com/bbernstein/lacylights-go/internal/services/pubsub"
	"github.com/bbernstein/lacylights-go/internal/services/undo"
	"github.com/glebarez/sqlite"
	"github.com/lucsky/cuid"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func setupUndoTestDB(t *testing.T) (*gorm.DB, func()) {
	t.Helper()

	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		t.Fatalf("Failed to open in-memory database: %v", err)
	}

	err = db.AutoMigrate(
		&models.Project{},
		&models.FixtureDefinition{},
		&models.ChannelDefinition{},
		&models.FixtureMode{},
		&models.ModeChannel{},
		&models.FixtureInstance{},
		&models.InstanceChannel{},
		&models.Look{},
		&models.FixtureValue{},
		&models.CueList{},
		&models.Cue{},
		&models.LookBoard{},
		&models.LookBoardButton{},
		&models.Operation{},
		&models.OperationPointer{},
		&models.Effect{},
		&models.EffectFixture{},
		&models.EffectChannel{},
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

// createUndoTestService creates an undo service with all required dependencies for testing
func createUndoTestService(db *gorm.DB) (*undo.Service, *repositories.ProjectRepository, *repositories.CueRepository, *repositories.CueListRepository, *repositories.LookRepository, *repositories.FixtureRepository) {
	projectRepo := repositories.NewProjectRepository(db)
	cueRepo := repositories.NewCueRepository(db)
	cueListRepo := repositories.NewCueListRepository(db)
	lookRepo := repositories.NewLookRepository(db)
	fixtureRepo := repositories.NewFixtureRepository(db)
	opRepo := repositories.NewOperationRepository(db)
	lookBoardRepo := repositories.NewLookBoardRepository(db)
	effectRepo := repositories.NewEffectRepository(db)
	ps := pubsub.New()

	undoService := undo.NewService(
		opRepo,
		lookRepo,
		fixtureRepo,
		cueRepo,
		cueListRepo,
		lookBoardRepo,
		effectRepo,
		projectRepo,
		ps,
	)

	return undoService, projectRepo, cueRepo, cueListRepo, lookRepo, fixtureRepo
}

// TestUndoRedoCueWorkflow_Integration tests a complete cue create/update/delete/undo/redo workflow
func TestUndoRedoCueWorkflow_Integration(t *testing.T) {
	db, cleanup := setupUndoTestDB(t)
	defer cleanup()

	undoService, projectRepo, cueRepo, cueListRepo, lookRepo, _ := createUndoTestService(db)

	ctx := context.Background()

	// Setup: Create project, look, and cue list
	project := &models.Project{ID: cuid.New(), Name: "Test Project"}
	if err := projectRepo.Create(ctx, project); err != nil {
		t.Fatalf("Failed to create project: %v", err)
	}

	look := &models.Look{
		ID:        cuid.New(),
		ProjectID: project.ID,
		Name:      "Test Look",
	}
	if err := lookRepo.Create(ctx, look); err != nil {
		t.Fatalf("Failed to create look: %v", err)
	}

	cueList := &models.CueList{
		ID:        cuid.New(),
		ProjectID: project.ID,
		Name:      "Test Cue List",
	}
	if err := cueListRepo.Create(ctx, cueList); err != nil {
		t.Fatalf("Failed to create cue list: %v", err)
	}

	// Step 1: Create a cue
	cue := &models.Cue{
		ID:         cuid.New(),
		CueListID:  cueList.ID,
		LookID:     look.ID,
		Name:       "Opening",
		CueNumber:  1.0,
		FadeInTime: 3.0,
	}
	if err := cueRepo.Create(ctx, cue); err != nil {
		t.Fatalf("Failed to create cue: %v", err)
	}

	// Record create operation
	newState, err := undoService.CaptureCueState(ctx, cue.ID)
	if err != nil {
		t.Fatalf("Failed to capture cue state: %v", err)
	}
	if err := undoService.RecordOperation(ctx, project.ID, undo.OperationTypeCreate, undo.EntityTypeCue, cue.ID,
		"Create cue 'Opening' (#1.0)", nil, newState, nil); err != nil {
		t.Fatalf("Failed to record create operation: %v", err)
	}

	// Step 2: Update the cue
	prevState, err := undoService.CaptureCueState(ctx, cue.ID)
	if err != nil {
		t.Fatalf("Failed to capture previous state: %v", err)
	}

	cue.Name = "Grand Opening"
	cue.FadeInTime = 5.0
	if err := cueRepo.Update(ctx, cue); err != nil {
		t.Fatalf("Failed to update cue: %v", err)
	}

	newState, err = undoService.CaptureCueState(ctx, cue.ID)
	if err != nil {
		t.Fatalf("Failed to capture new state: %v", err)
	}
	if err := undoService.RecordOperation(ctx, project.ID, undo.OperationTypeUpdate, undo.EntityTypeCue, cue.ID,
		"Update cue 'Grand Opening'", prevState, newState, nil); err != nil {
		t.Fatalf("Failed to record update operation: %v", err)
	}

	// Verify current state
	currentCue, err := cueRepo.FindByID(ctx, cue.ID)
	if err != nil {
		t.Fatalf("Failed to find cue: %v", err)
	}
	if currentCue.Name != "Grand Opening" {
		t.Errorf("Expected cue name 'Grand Opening', got '%s'", currentCue.Name)
	}
	if currentCue.FadeInTime != 5.0 {
		t.Errorf("Expected fade time 5.0, got %f", currentCue.FadeInTime)
	}

	// Step 3: Undo the update - should revert to "Opening"
	result, err := undoService.Undo(ctx, project.ID)
	if err != nil {
		t.Fatalf("Undo failed: %v", err)
	}
	if !result.Success {
		t.Fatalf("Undo was not successful: %s", result.Message)
	}

	// Verify undo result
	currentCue, err = cueRepo.FindByID(ctx, cue.ID)
	if err != nil {
		t.Fatalf("Failed to find cue after undo: %v", err)
	}
	if currentCue.Name != "Opening" {
		t.Errorf("Expected cue name 'Opening' after undo, got '%s'", currentCue.Name)
	}
	if currentCue.FadeInTime != 3.0 {
		t.Errorf("Expected fade time 3.0 after undo, got %f", currentCue.FadeInTime)
	}

	// Step 4: Redo the update - should return to "Grand Opening"
	result, err = undoService.Redo(ctx, project.ID)
	if err != nil {
		t.Fatalf("Redo failed: %v", err)
	}
	if !result.Success {
		t.Fatalf("Redo was not successful: %s", result.Message)
	}

	// Verify redo result
	currentCue, err = cueRepo.FindByID(ctx, cue.ID)
	if err != nil {
		t.Fatalf("Failed to find cue after redo: %v", err)
	}
	if currentCue.Name != "Grand Opening" {
		t.Errorf("Expected cue name 'Grand Opening' after redo, got '%s'", currentCue.Name)
	}
	if currentCue.FadeInTime != 5.0 {
		t.Errorf("Expected fade time 5.0 after redo, got %f", currentCue.FadeInTime)
	}

	// Step 5: Undo again, then undo the create
	_, err = undoService.Undo(ctx, project.ID)
	if err != nil {
		t.Fatalf("Second undo failed: %v", err)
	}

	result, err = undoService.Undo(ctx, project.ID)
	if err != nil {
		t.Fatalf("Undo create failed: %v", err)
	}
	if !result.Success {
		t.Fatalf("Undo create was not successful: %s", result.Message)
	}

	// Verify cue was deleted
	currentCue, err = cueRepo.FindByID(ctx, cue.ID)
	if err == nil && currentCue != nil {
		t.Errorf("Expected cue to be deleted after undo create, but it still exists")
	}

	// Step 6: Redo should recreate the cue
	result, err = undoService.Redo(ctx, project.ID)
	if err != nil {
		t.Fatalf("Redo create failed: %v", err)
	}
	if !result.Success {
		t.Fatalf("Redo create was not successful: %s", result.Message)
	}

	// Verify cue was recreated
	currentCue, err = cueRepo.FindByID(ctx, cue.ID)
	if err != nil {
		t.Fatalf("Failed to find cue after redo create: %v", err)
	}
	if currentCue.Name != "Opening" {
		t.Errorf("Expected cue name 'Opening' after redo create, got '%s'", currentCue.Name)
	}
}

// TestUndoRedoCueListWorkflow_Integration tests cue list with cues undo/redo
func TestUndoRedoCueListWorkflow_Integration(t *testing.T) {
	db, cleanup := setupUndoTestDB(t)
	defer cleanup()

	undoService, projectRepo, cueRepo, cueListRepo, lookRepo, _ := createUndoTestService(db)

	ctx := context.Background()

	// Setup
	project := &models.Project{ID: cuid.New(), Name: "Test Project"}
	if err := projectRepo.Create(ctx, project); err != nil {
		t.Fatalf("Failed to create project: %v", err)
	}

	look := &models.Look{ID: cuid.New(), ProjectID: project.ID, Name: "Test Look"}
	if err := lookRepo.Create(ctx, look); err != nil {
		t.Fatalf("Failed to create look: %v", err)
	}

	// Create cue list with cues
	cueList := &models.CueList{ID: cuid.New(), ProjectID: project.ID, Name: "Act 1"}
	if err := cueListRepo.Create(ctx, cueList); err != nil {
		t.Fatalf("Failed to create cue list: %v", err)
	}

	cue1 := &models.Cue{ID: cuid.New(), CueListID: cueList.ID, LookID: look.ID, Name: "Scene 1", CueNumber: 1.0}
	cue2 := &models.Cue{ID: cuid.New(), CueListID: cueList.ID, LookID: look.ID, Name: "Scene 2", CueNumber: 2.0}
	cue3 := &models.Cue{ID: cuid.New(), CueListID: cueList.ID, LookID: look.ID, Name: "Scene 3", CueNumber: 3.0}

	for _, c := range []*models.Cue{cue1, cue2, cue3} {
		if err := cueRepo.Create(ctx, c); err != nil {
			t.Fatalf("Failed to create cue: %v", err)
		}
	}

	// Capture state before deletion
	prevState, err := undoService.CaptureCueListState(ctx, cueList.ID)
	if err != nil {
		t.Fatalf("Failed to capture cue list state: %v", err)
	}

	// Delete cues first (mimics production resolver behavior)
	for _, c := range []*models.Cue{cue1, cue2, cue3} {
		if err := cueRepo.Delete(ctx, c.ID); err != nil {
			t.Fatalf("Failed to delete cue: %v", err)
		}
	}

	// Delete cue list
	if err := cueListRepo.Delete(ctx, cueList.ID); err != nil {
		t.Fatalf("Failed to delete cue list: %v", err)
	}

	// Record delete operation
	if err := undoService.RecordOperation(ctx, project.ID, undo.OperationTypeDelete, undo.EntityTypeCueList, cueList.ID,
		"Delete cue list 'Act 1' with 3 cues", prevState, nil, nil); err != nil {
		t.Fatalf("Failed to record delete operation: %v", err)
	}

	// Verify cue list and cues are deleted
	deletedCueList, _ := cueListRepo.FindByID(ctx, cueList.ID)
	if deletedCueList != nil {
		t.Errorf("Expected cue list to be deleted")
	}

	// Undo should restore cue list and all cues
	result, err := undoService.Undo(ctx, project.ID)
	if err != nil {
		t.Fatalf("Undo failed: %v", err)
	}
	if !result.Success {
		t.Fatalf("Undo was not successful: %s", result.Message)
	}

	// Verify cue list is restored
	restoredCueList, err := cueListRepo.FindByID(ctx, cueList.ID)
	if err != nil {
		t.Fatalf("Failed to find restored cue list: %v", err)
	}
	if restoredCueList.Name != "Act 1" {
		t.Errorf("Expected cue list name 'Act 1', got '%s'", restoredCueList.Name)
	}

	// Verify all cues are restored
	cues, err := cueRepo.FindByCueListID(ctx, cueList.ID)
	if err != nil {
		t.Fatalf("Failed to find cues: %v", err)
	}
	if len(cues) != 3 {
		t.Errorf("Expected 3 cues after undo, got %d", len(cues))
	}
}

// TestUndoRedoMultipleOperations_Integration tests undo/redo through multiple operations
func TestUndoRedoMultipleOperations_Integration(t *testing.T) {
	db, cleanup := setupUndoTestDB(t)
	defer cleanup()

	undoService, projectRepo, cueRepo, cueListRepo, lookRepo, _ := createUndoTestService(db)

	ctx := context.Background()

	// Setup
	project := &models.Project{ID: cuid.New(), Name: "Test Project"}
	if err := projectRepo.Create(ctx, project); err != nil {
		t.Fatalf("Failed to create project: %v", err)
	}

	look := &models.Look{ID: cuid.New(), ProjectID: project.ID, Name: "Test Look"}
	if err := lookRepo.Create(ctx, look); err != nil {
		t.Fatalf("Failed to create look: %v", err)
	}

	cueList := &models.CueList{ID: cuid.New(), ProjectID: project.ID, Name: "Main"}
	if err := cueListRepo.Create(ctx, cueList); err != nil {
		t.Fatalf("Failed to create cue list: %v", err)
	}

	// Create 5 cues sequentially, recording each operation
	cueIDs := make([]string, 5)
	for i := 0; i < 5; i++ {
		cue := &models.Cue{
			ID:         cuid.New(),
			CueListID:  cueList.ID,
			LookID:     look.ID,
			Name:       string(rune('A' + i)),
			CueNumber:  float64(i + 1),
			FadeInTime: float64(i + 1),
		}
		if err := cueRepo.Create(ctx, cue); err != nil {
			t.Fatalf("Failed to create cue %d: %v", i, err)
		}
		cueIDs[i] = cue.ID

		newState, err := undoService.CaptureCueState(ctx, cue.ID)
		if err != nil {
			t.Fatalf("Failed to capture cue state: %v", err)
		}
		if err := undoService.RecordOperation(ctx, project.ID, undo.OperationTypeCreate, undo.EntityTypeCue, cue.ID,
			"Create cue "+cue.Name, nil, newState, nil); err != nil {
			t.Fatalf("Failed to record operation: %v", err)
		}
	}

	// Verify operation history
	ops, total, err := undoService.GetOperationHistory(ctx, project.ID, 1, 10)
	if err != nil {
		t.Fatalf("Failed to get operation history: %v", err)
	}
	if total != 5 {
		t.Errorf("Expected 5 operations, got %d", total)
	}
	// Operations should be in newest-first order (E, D, C, B, A)
	if len(ops) >= 5 {
		if ops[0].Description != "Create cue E" {
			t.Errorf("Expected newest operation to be 'Create cue E', got '%s'", ops[0].Description)
		}
		if ops[4].Description != "Create cue A" {
			t.Errorf("Expected oldest operation to be 'Create cue A', got '%s'", ops[4].Description)
		}
	}

	// Undo 3 operations
	for i := 0; i < 3; i++ {
		_, err := undoService.Undo(ctx, project.ID)
		if err != nil {
			t.Fatalf("Undo %d failed: %v", i+1, err)
		}
	}

	// Verify only 2 cues remain (A and B)
	cues, err := cueRepo.FindByCueListID(ctx, cueList.ID)
	if err != nil {
		t.Fatalf("Failed to find cues: %v", err)
	}
	if len(cues) != 2 {
		t.Errorf("Expected 2 cues after 3 undos, got %d", len(cues))
	}

	// Redo 2 operations
	for i := 0; i < 2; i++ {
		_, err := undoService.Redo(ctx, project.ID)
		if err != nil {
			t.Fatalf("Redo %d failed: %v", i+1, err)
		}
	}

	// Verify 4 cues now exist (A, B, C, D)
	cues, err = cueRepo.FindByCueListID(ctx, cueList.ID)
	if err != nil {
		t.Fatalf("Failed to find cues after redo: %v", err)
	}
	if len(cues) != 4 {
		t.Errorf("Expected 4 cues after 2 redos, got %d", len(cues))
	}

	// Get status - should show can undo (4 ops) and can redo (1 op)
	status, err := undoService.GetStatus(ctx, project.ID)
	if err != nil {
		t.Fatalf("Failed to get status: %v", err)
	}
	if !status.CanUndo {
		t.Errorf("Expected CanUndo to be true")
	}
	if !status.CanRedo {
		t.Errorf("Expected CanRedo to be true")
	}
}

// TestUndoRedoReorderCues_Integration tests that reordering cues can be undone
func TestUndoRedoReorderCues_Integration(t *testing.T) {
	db, cleanup := setupUndoTestDB(t)
	defer cleanup()

	undoService, projectRepo, cueRepo, cueListRepo, lookRepo, _ := createUndoTestService(db)

	ctx := context.Background()

	// Setup
	project := &models.Project{ID: cuid.New(), Name: "Test Project"}
	_ = projectRepo.Create(ctx, project)

	look := &models.Look{ID: cuid.New(), ProjectID: project.ID, Name: "Test Look"}
	_ = lookRepo.Create(ctx, look)

	cueList := &models.CueList{ID: cuid.New(), ProjectID: project.ID, Name: "Main"}
	_ = cueListRepo.Create(ctx, cueList)

	// Create cues in order 1, 2, 3
	cues := []*models.Cue{
		{ID: cuid.New(), CueListID: cueList.ID, LookID: look.ID, Name: "First", CueNumber: 1.0},
		{ID: cuid.New(), CueListID: cueList.ID, LookID: look.ID, Name: "Second", CueNumber: 2.0},
		{ID: cuid.New(), CueListID: cueList.ID, LookID: look.ID, Name: "Third", CueNumber: 3.0},
	}
	for _, c := range cues {
		_ = cueRepo.Create(ctx, c)
	}

	// Capture state before reorder
	prevState, _ := undoService.CaptureCueListState(ctx, cueList.ID)

	// Reorder cues to 3, 1, 2
	cues[0].CueNumber = 2.0 // First -> 2
	cues[1].CueNumber = 3.0 // Second -> 3
	cues[2].CueNumber = 1.0 // Third -> 1
	for _, c := range cues {
		_ = cueRepo.Update(ctx, c)
	}

	// Capture state after reorder
	newState, _ := undoService.CaptureCueListState(ctx, cueList.ID)

	// Record reorder operation
	relatedIDs := []string{cues[0].ID, cues[1].ID, cues[2].ID}
	_ = undoService.RecordOperation(ctx, project.ID, undo.OperationTypeUpdate, undo.EntityTypeCueList, cueList.ID,
		"Reorder 3 cues in 'Main'", prevState, newState, relatedIDs)

	// Verify reordered state
	reorderedCues, _ := cueRepo.FindByCueListID(ctx, cueList.ID)
	cueMap := make(map[string]float64)
	for _, c := range reorderedCues {
		cueMap[c.Name] = c.CueNumber
	}
	if cueMap["First"] != 2.0 || cueMap["Second"] != 3.0 || cueMap["Third"] != 1.0 {
		t.Errorf("Reorder didn't work correctly: First=%f, Second=%f, Third=%f",
			cueMap["First"], cueMap["Second"], cueMap["Third"])
	}

	// Undo reorder
	result, err := undoService.Undo(ctx, project.ID)
	if err != nil {
		t.Fatalf("Undo reorder failed: %v", err)
	}
	if !result.Success {
		t.Fatalf("Undo reorder was not successful: %s", result.Message)
	}

	// Verify original order is restored
	restoredCues, _ := cueRepo.FindByCueListID(ctx, cueList.ID)
	cueMap = make(map[string]float64)
	for _, c := range restoredCues {
		cueMap[c.Name] = c.CueNumber
	}
	if cueMap["First"] != 1.0 || cueMap["Second"] != 2.0 || cueMap["Third"] != 3.0 {
		t.Errorf("Undo reorder didn't restore: First=%f, Second=%f, Third=%f",
			cueMap["First"], cueMap["Second"], cueMap["Third"])
	}

	// Redo reorder
	_, err = undoService.Redo(ctx, project.ID)
	if err != nil {
		t.Fatalf("Redo reorder failed: %v", err)
	}

	// Verify reorder is reapplied
	redoCues, _ := cueRepo.FindByCueListID(ctx, cueList.ID)
	cueMap = make(map[string]float64)
	for _, c := range redoCues {
		cueMap[c.Name] = c.CueNumber
	}
	if cueMap["First"] != 2.0 || cueMap["Second"] != 3.0 || cueMap["Third"] != 1.0 {
		t.Errorf("Redo reorder didn't work: First=%f, Second=%f, Third=%f",
			cueMap["First"], cueMap["Second"], cueMap["Third"])
	}
}

// TestUndoRedoDescriptiveHistory_Integration tests that operation history has useful descriptions
func TestUndoRedoDescriptiveHistory_Integration(t *testing.T) {
	db, cleanup := setupUndoTestDB(t)
	defer cleanup()

	undoService, projectRepo, cueRepo, cueListRepo, lookRepo, _ := createUndoTestService(db)

	ctx := context.Background()

	// Setup
	project := &models.Project{ID: cuid.New(), Name: "Test Project"}
	_ = projectRepo.Create(ctx, project)

	look := &models.Look{ID: cuid.New(), ProjectID: project.ID, Name: "Warm Wash"}
	_ = lookRepo.Create(ctx, look)

	cueList := &models.CueList{ID: cuid.New(), ProjectID: project.ID, Name: "Act 2 - Scene 4"}
	_ = cueListRepo.Create(ctx, cueList)

	cue := &models.Cue{
		ID:         cuid.New(),
		CueListID:  cueList.ID,
		LookID:     look.ID,
		Name:       "Romeo enters",
		CueNumber:  24.5,
		FadeInTime: 4.0,
	}
	_ = cueRepo.Create(ctx, cue)

	// Record with descriptive messages
	newState, _ := undoService.CaptureCueState(ctx, cue.ID)
	_ = undoService.RecordOperation(ctx, project.ID, undo.OperationTypeCreate, undo.EntityTypeCue, cue.ID,
		"Create cue 'Romeo enters' (#24.5)", nil, newState, nil)

	// Update
	prevState, _ := undoService.CaptureCueState(ctx, cue.ID)
	cue.Name = "Romeo enters slowly"
	cue.FadeInTime = 8.0
	_ = cueRepo.Update(ctx, cue)
	newState, _ = undoService.CaptureCueState(ctx, cue.ID)
	_ = undoService.RecordOperation(ctx, project.ID, undo.OperationTypeUpdate, undo.EntityTypeCue, cue.ID,
		"Update cue 'Romeo enters slowly' (fade 4.0s → 8.0s)", prevState, newState, nil)

	// Toggle skip
	prevState, _ = undoService.CaptureCueState(ctx, cue.ID)
	cue.Skip = true
	_ = cueRepo.Update(ctx, cue)
	newState, _ = undoService.CaptureCueState(ctx, cue.ID)
	_ = undoService.RecordOperation(ctx, project.ID, undo.OperationTypeUpdate, undo.EntityTypeCue, cue.ID,
		"Skip cue 'Romeo enters slowly'", prevState, newState, nil)

	// Get history
	ops, total, _ := undoService.GetOperationHistory(ctx, project.ID, 1, 10)
	if total != 3 {
		t.Errorf("Expected 3 operations, got %d", total)
	}

	// Verify descriptions (newest first)
	expectedDescs := []string{
		"Skip cue 'Romeo enters slowly'",
		"Update cue 'Romeo enters slowly' (fade 4.0s → 8.0s)",
		"Create cue 'Romeo enters' (#24.5)",
	}
	for i, expected := range expectedDescs {
		if i < len(ops) && ops[i].Description != expected {
			t.Errorf("Operation %d: expected '%s', got '%s'", i, expected, ops[i].Description)
		}
	}

	// Verify undo/redo descriptions in status
	status, _ := undoService.GetStatus(ctx, project.ID)
	if status.UndoDescription != "Skip cue 'Romeo enters slowly'" {
		t.Errorf("Expected undo description 'Skip cue...', got '%s'", status.UndoDescription)
	}

	// Undo once
	_, _ = undoService.Undo(ctx, project.ID)

	// Check status shows the next undo and redo descriptions
	status, _ = undoService.GetStatus(ctx, project.ID)
	if status.UndoDescription != "Update cue 'Romeo enters slowly' (fade 4.0s → 8.0s)" {
		t.Errorf("Expected undo description for update, got '%s'", status.UndoDescription)
	}
	if status.RedoDescription != "Skip cue 'Romeo enters slowly'" {
		t.Errorf("Expected redo description for skip, got '%s'", status.RedoDescription)
	}
}

// TestJumpToOperation_Integration tests jumping to a specific point in history
func TestJumpToOperation_Integration(t *testing.T) {
	db, cleanup := setupUndoTestDB(t)
	defer cleanup()

	undoService, projectRepo, cueRepo, cueListRepo, lookRepo, _ := createUndoTestService(db)

	ctx := context.Background()

	// Setup
	project := &models.Project{ID: cuid.New(), Name: "Test Project"}
	_ = projectRepo.Create(ctx, project)

	look := &models.Look{ID: cuid.New(), ProjectID: project.ID, Name: "Test Look"}
	_ = lookRepo.Create(ctx, look)

	cueList := &models.CueList{ID: cuid.New(), ProjectID: project.ID, Name: "Main"}
	_ = cueListRepo.Create(ctx, cueList)

	// Create 4 cues
	var opIDs []string
	for i := 1; i <= 4; i++ {
		cue := &models.Cue{
			ID:        cuid.New(),
			CueListID: cueList.ID,
			LookID:    look.ID,
			Name:      string(rune('A' + i - 1)),
			CueNumber: float64(i),
		}
		_ = cueRepo.Create(ctx, cue)

		newState, _ := undoService.CaptureCueState(ctx, cue.ID)
		_ = undoService.RecordOperation(ctx, project.ID, undo.OperationTypeCreate, undo.EntityTypeCue, cue.ID,
			"Create cue "+cue.Name, nil, newState, nil)
	}

	// Get operation history to find the second operation's ID
	ops, _, _ := undoService.GetOperationHistory(ctx, project.ID, 1, 10)
	// ops[0] = D (newest), ops[1] = C, ops[2] = B, ops[3] = A (oldest)
	// We want to jump to after B was created (ops[2])
	for _, op := range ops {
		opIDs = append(opIDs, op.ID)
	}
	// The second operation ID (B)
	targetOpID := opIDs[2]

	// Jump to that operation
	result, err := undoService.JumpToOperation(ctx, project.ID, targetOpID)
	if err != nil {
		t.Fatalf("JumpToOperation failed: %v", err)
	}
	if !result.Success {
		t.Fatalf("JumpToOperation was not successful: %s", result.Message)
	}

	// Verify state - should have cues A and B, but not C or D
	cues, _ := cueRepo.FindByCueListID(ctx, cueList.ID)
	if len(cues) != 2 {
		t.Errorf("Expected 2 cues after jump, got %d", len(cues))
	}
	cueNames := make(map[string]bool)
	for _, c := range cues {
		cueNames[c.Name] = true
	}
	if !cueNames["A"] || !cueNames["B"] {
		t.Errorf("Expected cues A and B after jump")
	}
	if cueNames["C"] || cueNames["D"] {
		t.Errorf("Expected cues C and D to be deleted after jump")
	}
}

// TestUndoRedoFixtureInstanceWorkflow_Integration tests fixture instance create/update/delete/undo/redo
func TestUndoRedoFixtureInstanceWorkflow_Integration(t *testing.T) {
	db, cleanup := setupUndoTestDB(t)
	defer cleanup()

	undoService, projectRepo, _, _, _, fixtureRepo := createUndoTestService(db)

	ctx := context.Background()

	// Setup: Create project and fixture definition
	project := &models.Project{ID: cuid.New(), Name: "Test Project"}
	if err := projectRepo.Create(ctx, project); err != nil {
		t.Fatalf("Failed to create project: %v", err)
	}

	// Create a fixture definition (required for fixture instances)
	fixtureDef := &models.FixtureDefinition{
		ID:           cuid.New(),
		Manufacturer: "Test Manufacturer",
		Model:        "Test Model",
		Type:         "LED_PAR",
	}
	if err := db.Create(fixtureDef).Error; err != nil {
		t.Fatalf("Failed to create fixture definition: %v", err)
	}

	// Step 1: Create a fixture instance
	fixture := &models.FixtureInstance{
		ID:           cuid.New(),
		ProjectID:    project.ID,
		DefinitionID: fixtureDef.ID,
		Name:         "Front Wash 1",
		Universe:     1,
		StartChannel: 1,
	}
	if err := fixtureRepo.Create(ctx, fixture); err != nil {
		t.Fatalf("Failed to create fixture: %v", err)
	}

	// Record create operation
	newState, err := undoService.CaptureFixtureState(ctx, fixture.ID)
	if err != nil {
		t.Fatalf("Failed to capture fixture state: %v", err)
	}
	if err := undoService.RecordOperation(ctx, project.ID, undo.OperationTypeCreate, undo.EntityTypeFixtureInstance, fixture.ID,
		"Create fixture 'Front Wash 1'", nil, newState, nil); err != nil {
		t.Fatalf("Failed to record create operation: %v", err)
	}

	// Step 2: Update the fixture
	prevState, err := undoService.CaptureFixtureState(ctx, fixture.ID)
	if err != nil {
		t.Fatalf("Failed to capture previous state: %v", err)
	}

	fixture.Name = "Front Wash Left"
	fixture.StartChannel = 10
	if err := fixtureRepo.Update(ctx, fixture); err != nil {
		t.Fatalf("Failed to update fixture: %v", err)
	}

	newState, err = undoService.CaptureFixtureState(ctx, fixture.ID)
	if err != nil {
		t.Fatalf("Failed to capture new state: %v", err)
	}
	if err := undoService.RecordOperation(ctx, project.ID, undo.OperationTypeUpdate, undo.EntityTypeFixtureInstance, fixture.ID,
		"Update fixture 'Front Wash Left'", prevState, newState, nil); err != nil {
		t.Fatalf("Failed to record update operation: %v", err)
	}

	// Verify current state
	currentFixture, err := fixtureRepo.FindByID(ctx, fixture.ID)
	if err != nil {
		t.Fatalf("Failed to find fixture: %v", err)
	}
	if currentFixture.Name != "Front Wash Left" {
		t.Errorf("Expected fixture name 'Front Wash Left', got '%s'", currentFixture.Name)
	}
	if currentFixture.StartChannel != 10 {
		t.Errorf("Expected start channel 10, got %d", currentFixture.StartChannel)
	}

	// Step 3: Undo the update
	result, err := undoService.Undo(ctx, project.ID)
	if err != nil {
		t.Fatalf("Undo failed: %v", err)
	}
	if !result.Success {
		t.Fatalf("Undo was not successful: %s", result.Message)
	}

	// Verify undo result
	currentFixture, err = fixtureRepo.FindByID(ctx, fixture.ID)
	if err != nil {
		t.Fatalf("Failed to find fixture after undo: %v", err)
	}
	if currentFixture.Name != "Front Wash 1" {
		t.Errorf("Expected fixture name 'Front Wash 1' after undo, got '%s'", currentFixture.Name)
	}
	if currentFixture.StartChannel != 1 {
		t.Errorf("Expected start channel 1 after undo, got %d", currentFixture.StartChannel)
	}

	// Step 4: Redo the update
	result, err = undoService.Redo(ctx, project.ID)
	if err != nil {
		t.Fatalf("Redo failed: %v", err)
	}
	if !result.Success {
		t.Fatalf("Redo was not successful: %s", result.Message)
	}

	// Verify redo result
	currentFixture, err = fixtureRepo.FindByID(ctx, fixture.ID)
	if err != nil {
		t.Fatalf("Failed to find fixture after redo: %v", err)
	}
	if currentFixture.Name != "Front Wash Left" {
		t.Errorf("Expected fixture name 'Front Wash Left' after redo, got '%s'", currentFixture.Name)
	}

	// Step 5: Undo both operations (delete the fixture)
	_, err = undoService.Undo(ctx, project.ID)
	if err != nil {
		t.Fatalf("Second undo failed: %v", err)
	}

	result, err = undoService.Undo(ctx, project.ID)
	if err != nil {
		t.Fatalf("Undo create failed: %v", err)
	}
	if !result.Success {
		t.Fatalf("Undo create was not successful: %s", result.Message)
	}

	// Verify fixture was deleted
	currentFixture, err = fixtureRepo.FindByID(ctx, fixture.ID)
	if err == nil && currentFixture != nil {
		t.Errorf("Expected fixture to be deleted after undo create, but it still exists")
	}

	// Step 6: Redo should recreate the fixture
	result, err = undoService.Redo(ctx, project.ID)
	if err != nil {
		t.Fatalf("Redo create failed: %v", err)
	}
	if !result.Success {
		t.Fatalf("Redo create was not successful: %s", result.Message)
	}

	// Verify fixture was recreated
	currentFixture, err = fixtureRepo.FindByID(ctx, fixture.ID)
	if err != nil {
		t.Fatalf("Failed to find fixture after redo create: %v", err)
	}
	if currentFixture.Name != "Front Wash 1" {
		t.Errorf("Expected fixture name 'Front Wash 1' after redo create, got '%s'", currentFixture.Name)
	}
}

// TestUndoRedoLookWorkflow_Integration tests look create/update/delete/undo/redo with fixture values
func TestUndoRedoLookWorkflow_Integration(t *testing.T) {
	db, cleanup := setupUndoTestDB(t)
	defer cleanup()

	undoService, projectRepo, _, _, lookRepo, fixtureRepo := createUndoTestService(db)

	ctx := context.Background()

	// Setup: Create project and fixture
	project := &models.Project{ID: cuid.New(), Name: "Test Project"}
	if err := projectRepo.Create(ctx, project); err != nil {
		t.Fatalf("Failed to create project: %v", err)
	}

	fixtureDef := &models.FixtureDefinition{
		ID:           cuid.New(),
		Manufacturer: "Test Manufacturer",
		Model:        "Test Model",
		Type:         "LED_PAR",
	}
	if err := db.Create(fixtureDef).Error; err != nil {
		t.Fatalf("Failed to create fixture definition: %v", err)
	}

	fixture := &models.FixtureInstance{
		ID:           cuid.New(),
		ProjectID:    project.ID,
		DefinitionID: fixtureDef.ID,
		Name:         "Test Fixture",
		Universe:     1,
		StartChannel: 1,
	}
	if err := fixtureRepo.Create(ctx, fixture); err != nil {
		t.Fatalf("Failed to create fixture: %v", err)
	}

	// Step 1: Create a look with fixture values
	look := &models.Look{
		ID:        cuid.New(),
		ProjectID: project.ID,
		Name:      "Warm Wash",
	}
	fixtureValues := []models.FixtureValue{
		{
			ID:        cuid.New(),
			LookID:    look.ID,
			FixtureID: fixture.ID,
			Channels:  `[{"offset":0,"value":255},{"offset":1,"value":128}]`,
		},
	}
	if err := lookRepo.CreateWithFixtureValues(ctx, look, fixtureValues); err != nil {
		t.Fatalf("Failed to create look: %v", err)
	}

	// Record create operation
	newState, err := undoService.CaptureLookState(ctx, look.ID)
	if err != nil {
		t.Fatalf("Failed to capture look state: %v", err)
	}
	if err := undoService.RecordOperation(ctx, project.ID, undo.OperationTypeCreate, undo.EntityTypeLook, look.ID,
		"Create look 'Warm Wash'", nil, newState, nil); err != nil {
		t.Fatalf("Failed to record create operation: %v", err)
	}

	// Step 2: Update the look
	prevState, err := undoService.CaptureLookState(ctx, look.ID)
	if err != nil {
		t.Fatalf("Failed to capture previous state: %v", err)
	}

	look.Name = "Cool Wash"
	if err := lookRepo.Update(ctx, look); err != nil {
		t.Fatalf("Failed to update look: %v", err)
	}
	// Update fixture values
	if err := lookRepo.DeleteFixtureValues(ctx, look.ID); err != nil {
		t.Fatalf("Failed to delete fixture values: %v", err)
	}
	newFixtureValues := []models.FixtureValue{
		{
			ID:        cuid.New(),
			LookID:    look.ID,
			FixtureID: fixture.ID,
			Channels:  `[{"offset":0,"value":100},{"offset":1,"value":200}]`,
		},
	}
	if err := lookRepo.CreateFixtureValues(ctx, newFixtureValues); err != nil {
		t.Fatalf("Failed to create new fixture values: %v", err)
	}

	newState, err = undoService.CaptureLookState(ctx, look.ID)
	if err != nil {
		t.Fatalf("Failed to capture new state: %v", err)
	}
	if err := undoService.RecordOperation(ctx, project.ID, undo.OperationTypeUpdate, undo.EntityTypeLook, look.ID,
		"Update look 'Cool Wash'", prevState, newState, nil); err != nil {
		t.Fatalf("Failed to record update operation: %v", err)
	}

	// Verify current state
	currentLook, err := lookRepo.FindByID(ctx, look.ID)
	if err != nil {
		t.Fatalf("Failed to find look: %v", err)
	}
	if currentLook.Name != "Cool Wash" {
		t.Errorf("Expected look name 'Cool Wash', got '%s'", currentLook.Name)
	}

	// Step 3: Undo the update
	result, err := undoService.Undo(ctx, project.ID)
	if err != nil {
		t.Fatalf("Undo failed: %v", err)
	}
	if !result.Success {
		t.Fatalf("Undo was not successful: %s", result.Message)
	}

	// Verify look was restored
	currentLook, err = lookRepo.FindByID(ctx, look.ID)
	if err != nil {
		t.Fatalf("Failed to find look after undo: %v", err)
	}
	if currentLook.Name != "Warm Wash" {
		t.Errorf("Expected look name 'Warm Wash' after undo, got '%s'", currentLook.Name)
	}

	// Verify fixture values were restored
	restoredValues, err := lookRepo.GetFixtureValues(ctx, look.ID)
	if err != nil {
		t.Fatalf("Failed to get fixture values: %v", err)
	}
	if len(restoredValues) != 1 {
		t.Errorf("Expected 1 fixture value after undo, got %d", len(restoredValues))
	}
	if restoredValues[0].Channels != `[{"offset":0,"value":255},{"offset":1,"value":128}]` {
		t.Errorf("Expected original fixture values after undo")
	}

	// Step 4: Undo the create (delete the look)
	result, err = undoService.Undo(ctx, project.ID)
	if err != nil {
		t.Fatalf("Undo create failed: %v", err)
	}
	if !result.Success {
		t.Fatalf("Undo create was not successful: %s", result.Message)
	}

	// Verify look was deleted
	currentLook, err = lookRepo.FindByID(ctx, look.ID)
	if err == nil && currentLook != nil {
		t.Errorf("Expected look to be deleted after undo create")
	}

	// Step 5: Redo should recreate the look with fixture values
	result, err = undoService.Redo(ctx, project.ID)
	if err != nil {
		t.Fatalf("Redo create failed: %v", err)
	}
	if !result.Success {
		t.Fatalf("Redo create was not successful: %s", result.Message)
	}

	// Verify look and fixture values were restored
	currentLook, err = lookRepo.FindByID(ctx, look.ID)
	if err != nil {
		t.Fatalf("Failed to find look after redo: %v", err)
	}
	if currentLook.Name != "Warm Wash" {
		t.Errorf("Expected look name 'Warm Wash' after redo, got '%s'", currentLook.Name)
	}

	restoredValues, err = lookRepo.GetFixtureValues(ctx, look.ID)
	if err != nil {
		t.Fatalf("Failed to get fixture values after redo: %v", err)
	}
	if len(restoredValues) != 1 {
		t.Errorf("Expected 1 fixture value after redo, got %d", len(restoredValues))
	}
}

// TestUndoRedoCueListUpdate_Integration tests cue list update undo/redo
func TestUndoRedoCueListUpdate_Integration(t *testing.T) {
	db, cleanup := setupUndoTestDB(t)
	defer cleanup()

	undoService, projectRepo, _, cueListRepo, _, _ := createUndoTestService(db)

	ctx := context.Background()

	// Setup
	project := &models.Project{ID: cuid.New(), Name: "Test Project"}
	_ = projectRepo.Create(ctx, project)

	desc := "First act"
	cueList := &models.CueList{
		ID:          cuid.New(),
		ProjectID:   project.ID,
		Name:        "Act 1",
		Description: &desc,
		Loop:        false,
	}
	if err := cueListRepo.Create(ctx, cueList); err != nil {
		t.Fatalf("Failed to create cue list: %v", err)
	}

	// Capture state before update
	prevState, _ := undoService.CaptureCueListState(ctx, cueList.ID)

	// Update cue list
	newDesc := "First act with changes"
	cueList.Name = "Act 1 - Revised"
	cueList.Description = &newDesc
	cueList.Loop = true
	if err := cueListRepo.Update(ctx, cueList); err != nil {
		t.Fatalf("Failed to update cue list: %v", err)
	}

	newState, _ := undoService.CaptureCueListState(ctx, cueList.ID)
	_ = undoService.RecordOperation(ctx, project.ID, undo.OperationTypeUpdate, undo.EntityTypeCueList, cueList.ID,
		"Update cue list 'Act 1 - Revised'", prevState, newState, nil)

	// Verify update
	updated, _ := cueListRepo.FindByID(ctx, cueList.ID)
	if updated.Name != "Act 1 - Revised" {
		t.Errorf("Expected 'Act 1 - Revised', got '%s'", updated.Name)
	}
	if !updated.Loop {
		t.Error("Expected Loop to be true")
	}

	// Undo
	result, err := undoService.Undo(ctx, project.ID)
	if err != nil || !result.Success {
		t.Fatalf("Undo failed: %v, %s", err, result.Message)
	}

	// Verify undo
	restored, _ := cueListRepo.FindByID(ctx, cueList.ID)
	if restored.Name != "Act 1" {
		t.Errorf("Expected 'Act 1' after undo, got '%s'", restored.Name)
	}
	if restored.Loop {
		t.Error("Expected Loop to be false after undo")
	}

	// Redo
	result, err = undoService.Redo(ctx, project.ID)
	if err != nil || !result.Success {
		t.Fatalf("Redo failed: %v, %s", err, result.Message)
	}

	// Verify redo
	redone, _ := cueListRepo.FindByID(ctx, cueList.ID)
	if redone.Name != "Act 1 - Revised" {
		t.Errorf("Expected 'Act 1 - Revised' after redo, got '%s'", redone.Name)
	}
}

// TestUndoRedoCueDelete_Integration tests direct cue deletion undo/redo
func TestUndoRedoCueDelete_Integration(t *testing.T) {
	db, cleanup := setupUndoTestDB(t)
	defer cleanup()

	undoService, projectRepo, cueRepo, cueListRepo, lookRepo, _ := createUndoTestService(db)

	ctx := context.Background()

	// Setup
	project := &models.Project{ID: cuid.New(), Name: "Test Project"}
	_ = projectRepo.Create(ctx, project)

	look := &models.Look{ID: cuid.New(), ProjectID: project.ID, Name: "Test Look"}
	_ = lookRepo.Create(ctx, look)

	cueList := &models.CueList{ID: cuid.New(), ProjectID: project.ID, Name: "Main"}
	_ = cueListRepo.Create(ctx, cueList)

	notes := "End of scene"
	cue := &models.Cue{
		ID:         cuid.New(),
		CueListID:  cueList.ID,
		LookID:     look.ID,
		Name:       "Blackout",
		CueNumber:  10.0,
		FadeInTime: 2.0,
		Notes:      &notes,
	}
	if err := cueRepo.Create(ctx, cue); err != nil {
		t.Fatalf("Failed to create cue: %v", err)
	}

	// Capture state before delete
	prevState, _ := undoService.CaptureCueState(ctx, cue.ID)

	// Delete cue
	if err := cueRepo.Delete(ctx, cue.ID); err != nil {
		t.Fatalf("Failed to delete cue: %v", err)
	}

	// Record delete operation
	_ = undoService.RecordOperation(ctx, project.ID, undo.OperationTypeDelete, undo.EntityTypeCue, cue.ID,
		"Delete cue 'Blackout'", prevState, nil, nil)

	// Verify cue is deleted
	deleted, _ := cueRepo.FindByID(ctx, cue.ID)
	if deleted != nil {
		t.Error("Expected cue to be deleted")
	}

	// Undo should restore the cue
	result, err := undoService.Undo(ctx, project.ID)
	if err != nil || !result.Success {
		t.Fatalf("Undo failed: %v, %s", err, result.Message)
	}

	// Verify cue is restored with all properties
	restored, err := cueRepo.FindByID(ctx, cue.ID)
	if err != nil {
		t.Fatalf("Failed to find restored cue: %v", err)
	}
	if restored.Name != "Blackout" {
		t.Errorf("Expected name 'Blackout', got '%s'", restored.Name)
	}
	if restored.CueNumber != 10.0 {
		t.Errorf("Expected cue number 10.0, got %f", restored.CueNumber)
	}
	if restored.FadeInTime != 2.0 {
		t.Errorf("Expected fade time 2.0, got %f", restored.FadeInTime)
	}
	if restored.Notes == nil || *restored.Notes != "End of scene" {
		t.Errorf("Expected notes 'End of scene', got '%v'", restored.Notes)
	}

	// Redo should delete the cue again
	result, err = undoService.Redo(ctx, project.ID)
	if err != nil || !result.Success {
		t.Fatalf("Redo failed: %v, %s", err, result.Message)
	}

	// Verify cue is deleted again
	deleted, _ = cueRepo.FindByID(ctx, cue.ID)
	if deleted != nil {
		t.Error("Expected cue to be deleted after redo")
	}
}

// TestClearHistory_Integration tests clearing operation history
func TestClearHistory_Integration(t *testing.T) {
	db, cleanup := setupUndoTestDB(t)
	defer cleanup()

	undoService, projectRepo, cueRepo, cueListRepo, lookRepo, _ := createUndoTestService(db)

	ctx := context.Background()

	// Setup
	project := &models.Project{ID: cuid.New(), Name: "Test Project"}
	_ = projectRepo.Create(ctx, project)

	look := &models.Look{ID: cuid.New(), ProjectID: project.ID, Name: "Test Look"}
	_ = lookRepo.Create(ctx, look)

	cueList := &models.CueList{ID: cuid.New(), ProjectID: project.ID, Name: "Main"}
	_ = cueListRepo.Create(ctx, cueList)

	// Create multiple cues and record operations
	for i := 0; i < 5; i++ {
		cue := &models.Cue{
			ID:        cuid.New(),
			CueListID: cueList.ID,
			LookID:    look.ID,
			Name:      string(rune('A' + i)),
			CueNumber: float64(i + 1),
		}
		_ = cueRepo.Create(ctx, cue)

		newState, _ := undoService.CaptureCueState(ctx, cue.ID)
		_ = undoService.RecordOperation(ctx, project.ID, undo.OperationTypeCreate, undo.EntityTypeCue, cue.ID,
			"Create cue "+cue.Name, nil, newState, nil)
	}

	// Verify we have operations
	ops, total, _ := undoService.GetOperationHistory(ctx, project.ID, 1, 10)
	if total != 5 {
		t.Errorf("Expected 5 operations before clear, got %d", total)
	}
	if len(ops) != 5 {
		t.Errorf("Expected 5 operations in list, got %d", len(ops))
	}

	// Verify undo is available
	status, _ := undoService.GetStatus(ctx, project.ID)
	if !status.CanUndo {
		t.Error("Expected CanUndo to be true before clear")
	}

	// Clear history
	if err := undoService.ClearHistory(ctx, project.ID); err != nil {
		t.Fatalf("ClearHistory failed: %v", err)
	}

	// Verify history is cleared
	ops, total, _ = undoService.GetOperationHistory(ctx, project.ID, 1, 10)
	if total != 0 {
		t.Errorf("Expected 0 operations after clear, got %d", total)
	}
	if len(ops) != 0 {
		t.Errorf("Expected empty operations list, got %d", len(ops))
	}

	// Verify undo is not available
	status, _ = undoService.GetStatus(ctx, project.ID)
	if status.CanUndo {
		t.Error("Expected CanUndo to be false after clear")
	}
	if status.CanRedo {
		t.Error("Expected CanRedo to be false after clear")
	}
}

// mockPlaybackController implements undo.PlaybackController for testing
type mockPlaybackController struct {
	goToCalls []struct {
		cueListID string
		cueNumber float64
		fadeTime  *float64
	}
	stopCalls []string
}

func (m *mockPlaybackController) GoToCueNumber(ctx context.Context, cueListID string, cueNumber float64, fadeInTimeOverride *float64) error {
	m.goToCalls = append(m.goToCalls, struct {
		cueListID string
		cueNumber float64
		fadeTime  *float64
	}{cueListID, cueNumber, fadeInTimeOverride})
	return nil
}

func (m *mockPlaybackController) StopCueList(cueListID string) {
	m.stopCalls = append(m.stopCalls, cueListID)
}

// TestUndoRedoCuePlayback_Integration tests cue playback undo/redo
func TestUndoRedoCuePlayback_Integration(t *testing.T) {
	db, cleanup := setupUndoTestDB(t)
	defer cleanup()

	undoService, projectRepo, cueRepo, cueListRepo, lookRepo, _ := createUndoTestService(db)

	// Set up mock playback controller
	mockPlayback := &mockPlaybackController{}
	undoService.SetPlaybackController(mockPlayback)

	ctx := context.Background()

	// Setup
	project := &models.Project{ID: cuid.New(), Name: "Test Project"}
	_ = projectRepo.Create(ctx, project)

	look := &models.Look{ID: cuid.New(), ProjectID: project.ID, Name: "Test Look"}
	_ = lookRepo.Create(ctx, look)

	cueList := &models.CueList{ID: cuid.New(), ProjectID: project.ID, Name: "Main Show"}
	_ = cueListRepo.Create(ctx, cueList)

	cue1 := &models.Cue{ID: cuid.New(), CueListID: cueList.ID, LookID: look.ID, Name: "Opening", CueNumber: 1.0, FadeInTime: 3.0}
	cue2 := &models.Cue{ID: cuid.New(), CueListID: cueList.ID, LookID: look.ID, Name: "Scene 1", CueNumber: 2.0, FadeInTime: 2.0}
	_ = cueRepo.Create(ctx, cue1)
	_ = cueRepo.Create(ctx, cue2)

	// Simulate: Playback was stopped, then started on cue 1
	fadeTime := 3.0
	prevSnapshot := &undo.CuePlaybackSnapshot{
		CueListID: cueList.ID,
		ProjectID: project.ID,
		IsPlaying: false,
	}
	newSnapshot := &undo.CuePlaybackSnapshot{
		CueListID:   cueList.ID,
		ProjectID:   project.ID,
		CueID:       &cue1.ID,
		CueNumber:   &cue1.CueNumber,
		CueName:     &cue1.Name,
		FadeInTime:  &fadeTime,
		IsPlaying:   true,
		CueListName: cueList.Name,
	}

	_ = undoService.RecordOperation(ctx, project.ID, undo.OperationTypeUpdate, undo.EntityTypeCuePlayback, cueList.ID,
		"Start cue list 'Main Show'", prevSnapshot, newSnapshot, nil)

	// Simulate: Advance to cue 2
	cue2FadeTime := 2.0
	prevSnapshot2 := newSnapshot
	newSnapshot2 := &undo.CuePlaybackSnapshot{
		CueListID:   cueList.ID,
		ProjectID:   project.ID,
		CueID:       &cue2.ID,
		CueNumber:   &cue2.CueNumber,
		CueName:     &cue2.Name,
		FadeInTime:  &cue2FadeTime,
		IsPlaying:   true,
		CueListName: cueList.Name,
	}

	_ = undoService.RecordOperation(ctx, project.ID, undo.OperationTypeUpdate, undo.EntityTypeCuePlayback, cueList.ID,
		"Next cue to 'Scene 1'", prevSnapshot2, newSnapshot2, nil)

	// Undo: Should go back to cue 1
	result, err := undoService.Undo(ctx, project.ID)
	if err != nil {
		t.Fatalf("Undo failed: %v", err)
	}
	if !result.Success {
		t.Fatalf("Undo was not successful: %s", result.Message)
	}

	// Verify mock was called to go to cue 1
	if len(mockPlayback.goToCalls) != 1 {
		t.Errorf("Expected 1 GoToCueNumber call, got %d", len(mockPlayback.goToCalls))
	} else {
		call := mockPlayback.goToCalls[0]
		if call.cueListID != cueList.ID {
			t.Errorf("Expected cue list ID %s, got %s", cueList.ID, call.cueListID)
		}
		if call.cueNumber != 1.0 {
			t.Errorf("Expected cue number 1.0, got %f", call.cueNumber)
		}
	}

	// Undo again: Should stop playback
	mockPlayback.goToCalls = nil // Reset
	result, err = undoService.Undo(ctx, project.ID)
	if err != nil {
		t.Fatalf("Second undo failed: %v", err)
	}
	if !result.Success {
		t.Fatalf("Second undo was not successful: %s", result.Message)
	}

	// Verify mock was called to stop
	if len(mockPlayback.stopCalls) != 1 {
		t.Errorf("Expected 1 StopCueList call, got %d", len(mockPlayback.stopCalls))
	} else if mockPlayback.stopCalls[0] != cueList.ID {
		t.Errorf("Expected stop for cue list ID %s, got %s", cueList.ID, mockPlayback.stopCalls[0])
	}

	// Redo: Should restart playback at cue 1
	mockPlayback.stopCalls = nil
	mockPlayback.goToCalls = nil
	result, err = undoService.Redo(ctx, project.ID)
	if err != nil {
		t.Fatalf("Redo failed: %v", err)
	}
	if !result.Success {
		t.Fatalf("Redo was not successful: %s", result.Message)
	}

	// Verify mock was called to go to cue 1
	if len(mockPlayback.goToCalls) != 1 {
		t.Errorf("Expected 1 GoToCueNumber call on redo, got %d", len(mockPlayback.goToCalls))
	}
}
