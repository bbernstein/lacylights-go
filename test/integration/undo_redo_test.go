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
		&models.CueEffect{},
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
func createUndoTestService(db *gorm.DB) (*undo.Service, *repositories.ProjectRepository, *repositories.CueRepository, *repositories.CueListRepository, *repositories.LookRepository, *repositories.FixtureRepository, *repositories.LookBoardRepository, *repositories.EffectRepository) {
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

	return undoService, projectRepo, cueRepo, cueListRepo, lookRepo, fixtureRepo, lookBoardRepo, effectRepo
}

// TestUndoRedoCueWorkflow_Integration tests a complete cue create/update/delete/undo/redo workflow
func TestUndoRedoCueWorkflow_Integration(t *testing.T) {
	db, cleanup := setupUndoTestDB(t)
	defer cleanup()

	undoService, projectRepo, cueRepo, cueListRepo, lookRepo, _, _, _ := createUndoTestService(db)

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

	undoService, projectRepo, cueRepo, cueListRepo, lookRepo, _, _, _ := createUndoTestService(db)

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

	undoService, projectRepo, cueRepo, cueListRepo, lookRepo, _, _, _ := createUndoTestService(db)

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

	undoService, projectRepo, cueRepo, cueListRepo, lookRepo, _, _, _ := createUndoTestService(db)

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

	undoService, projectRepo, cueRepo, cueListRepo, lookRepo, _, _, _ := createUndoTestService(db)

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

	undoService, projectRepo, cueRepo, cueListRepo, lookRepo, _, _, _ := createUndoTestService(db)

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

	undoService, projectRepo, _, _, _, fixtureRepo, _, _ := createUndoTestService(db)

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

	undoService, projectRepo, _, _, lookRepo, fixtureRepo, _, _ := createUndoTestService(db)

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

	undoService, projectRepo, _, cueListRepo, _, _, _, _ := createUndoTestService(db)

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

	undoService, projectRepo, cueRepo, cueListRepo, lookRepo, _, _, _ := createUndoTestService(db)

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

	undoService, projectRepo, cueRepo, cueListRepo, lookRepo, _, _, _ := createUndoTestService(db)

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

	undoService, projectRepo, cueRepo, cueListRepo, lookRepo, _, _, _ := createUndoTestService(db)

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

// Helper functions for pointer values in tests
func intPtr(i int) *int       { return &i }
func strPtr(s string) *string { return &s }

// TestUndoRedoLookBoardWorkflow_Integration tests look board create/update/delete/undo/redo
func TestUndoRedoLookBoardWorkflow_Integration(t *testing.T) {
	db, cleanup := setupUndoTestDB(t)
	defer cleanup()

	undoService, projectRepo, _, _, lookRepo, _, lookBoardRepo, _ := createUndoTestService(db)

	ctx := context.Background()

	// Setup: Create project and look
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

	// Step 1: Create a look board
	board := &models.LookBoard{
		ID:              cuid.New(),
		ProjectID:       project.ID,
		Name:            "Main Board",
		DefaultFadeTime: 3.0,
		GridSize:        intPtr(50),
		CanvasWidth:     2000,
		CanvasHeight:    2000,
	}
	if err := lookBoardRepo.Create(ctx, board); err != nil {
		t.Fatalf("Failed to create look board: %v", err)
	}

	// Record create operation
	newState, err := undoService.CaptureLookBoardState(ctx, board.ID)
	if err != nil {
		t.Fatalf("Failed to capture look board state: %v", err)
	}
	if err := undoService.RecordOperation(ctx, project.ID, undo.OperationTypeCreate, undo.EntityTypeLookBoard, board.ID,
		"Create look board 'Main Board'", nil, newState, nil); err != nil {
		t.Fatalf("Failed to record create operation: %v", err)
	}

	// Step 2: Update the look board
	prevState, err := undoService.CaptureLookBoardState(ctx, board.ID)
	if err != nil {
		t.Fatalf("Failed to capture previous state: %v", err)
	}

	board.Name = "Primary Control Board"
	board.DefaultFadeTime = 5.0
	if err := lookBoardRepo.Update(ctx, board); err != nil {
		t.Fatalf("Failed to update look board: %v", err)
	}

	newState, err = undoService.CaptureLookBoardState(ctx, board.ID)
	if err != nil {
		t.Fatalf("Failed to capture new state: %v", err)
	}
	if err := undoService.RecordOperation(ctx, project.ID, undo.OperationTypeUpdate, undo.EntityTypeLookBoard, board.ID,
		"Update look board 'Primary Control Board'", prevState, newState, nil); err != nil {
		t.Fatalf("Failed to record update operation: %v", err)
	}

	// Verify current state
	currentBoard, err := lookBoardRepo.FindByID(ctx, board.ID)
	if err != nil {
		t.Fatalf("Failed to find look board: %v", err)
	}
	if currentBoard.Name != "Primary Control Board" {
		t.Errorf("Expected board name 'Primary Control Board', got '%s'", currentBoard.Name)
	}
	if currentBoard.DefaultFadeTime != 5.0 {
		t.Errorf("Expected default fade time 5.0, got %f", currentBoard.DefaultFadeTime)
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
	currentBoard, err = lookBoardRepo.FindByID(ctx, board.ID)
	if err != nil {
		t.Fatalf("Failed to find look board after undo: %v", err)
	}
	if currentBoard.Name != "Main Board" {
		t.Errorf("Expected board name 'Main Board' after undo, got '%s'", currentBoard.Name)
	}
	if currentBoard.DefaultFadeTime != 3.0 {
		t.Errorf("Expected default fade time 3.0 after undo, got %f", currentBoard.DefaultFadeTime)
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
	currentBoard, err = lookBoardRepo.FindByID(ctx, board.ID)
	if err != nil {
		t.Fatalf("Failed to find look board after redo: %v", err)
	}
	if currentBoard.Name != "Primary Control Board" {
		t.Errorf("Expected board name 'Primary Control Board' after redo, got '%s'", currentBoard.Name)
	}

	// Step 5: Undo both operations (delete the board)
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

	// Verify board was deleted
	currentBoard, err = lookBoardRepo.FindByID(ctx, board.ID)
	if err == nil && currentBoard != nil {
		t.Errorf("Expected board to be deleted after undo create, but it still exists")
	}

	// Step 6: Redo should recreate the board
	result, err = undoService.Redo(ctx, project.ID)
	if err != nil {
		t.Fatalf("Redo create failed: %v", err)
	}
	if !result.Success {
		t.Fatalf("Redo create was not successful: %s", result.Message)
	}

	// Verify board was recreated
	currentBoard, err = lookBoardRepo.FindByID(ctx, board.ID)
	if err != nil {
		t.Fatalf("Failed to find look board after redo create: %v", err)
	}
	if currentBoard.Name != "Main Board" {
		t.Errorf("Expected board name 'Main Board' after redo create, got '%s'", currentBoard.Name)
	}
}

// TestUndoRedoLookBoardWithButtons_Integration tests look board with buttons undo/redo
func TestUndoRedoLookBoardWithButtons_Integration(t *testing.T) {
	db, cleanup := setupUndoTestDB(t)
	defer cleanup()

	undoService, projectRepo, _, _, lookRepo, _, lookBoardRepo, _ := createUndoTestService(db)

	ctx := context.Background()

	// Setup: Create project, look, and board
	project := &models.Project{ID: cuid.New(), Name: "Test Project"}
	_ = projectRepo.Create(ctx, project)

	look := &models.Look{ID: cuid.New(), ProjectID: project.ID, Name: "Test Look"}
	_ = lookRepo.Create(ctx, look)

	board := &models.LookBoard{
		ID:              cuid.New(),
		ProjectID:       project.ID,
		Name:            "Button Board",
		DefaultFadeTime: 3.0,
		GridSize:        intPtr(50),
		CanvasWidth:     2000,
		CanvasHeight:    2000,
	}
	_ = lookBoardRepo.Create(ctx, board)

	// Capture state before adding button
	prevState, _ := undoService.CaptureLookBoardState(ctx, board.ID)

	// Add a button to the board
	button := &models.LookBoardButton{
		ID:          cuid.New(),
		LookBoardID: board.ID,
		LookID:      look.ID,
		LayoutX:     100,
		LayoutY:     100,
		Width:       intPtr(200),
		Height:      intPtr(120),
		Label:       strPtr("Look 1"),
		Color:       strPtr("#FF0000"),
	}
	if err := lookBoardRepo.CreateButton(ctx, button); err != nil {
		t.Fatalf("Failed to create button: %v", err)
	}

	// Capture state after adding button
	newState, _ := undoService.CaptureLookBoardState(ctx, board.ID)
	_ = undoService.RecordOperation(ctx, project.ID, undo.OperationTypeUpdate, undo.EntityTypeLookBoard, board.ID,
		"Add button 'Look 1' to board", prevState, newState, nil)

	// Verify button was added
	buttons, _ := lookBoardRepo.GetButtons(ctx, board.ID)
	if len(buttons) != 1 {
		t.Errorf("Expected 1 button, got %d", len(buttons))
	}

	// Capture state before removing button
	prevState, _ = undoService.CaptureLookBoardState(ctx, board.ID)

	// Remove the button
	if err := lookBoardRepo.DeleteButton(ctx, button.ID); err != nil {
		t.Fatalf("Failed to delete button: %v", err)
	}

	// Capture state after removing button
	newState, _ = undoService.CaptureLookBoardState(ctx, board.ID)
	_ = undoService.RecordOperation(ctx, project.ID, undo.OperationTypeUpdate, undo.EntityTypeLookBoard, board.ID,
		"Remove button 'Look 1' from board", prevState, newState, nil)

	// Verify button was removed
	buttons, _ = lookBoardRepo.GetButtons(ctx, board.ID)
	if len(buttons) != 0 {
		t.Errorf("Expected 0 buttons after removal, got %d", len(buttons))
	}

	// Undo: Should restore the button
	result, err := undoService.Undo(ctx, project.ID)
	if err != nil {
		t.Fatalf("Undo failed: %v", err)
	}
	if !result.Success {
		t.Fatalf("Undo was not successful: %s", result.Message)
	}

	// Verify button was restored
	buttons, _ = lookBoardRepo.GetButtons(ctx, board.ID)
	if len(buttons) != 1 {
		t.Errorf("Expected 1 button after undo, got %d", len(buttons))
	}
	if len(buttons) > 0 {
		if buttons[0].Label == nil || *buttons[0].Label != "Look 1" {
			t.Errorf("Expected button label 'Look 1', got '%v'", buttons[0].Label)
		}
		if buttons[0].LayoutX != 100 || buttons[0].LayoutY != 100 {
			t.Errorf("Expected button position (100, 100), got (%d, %d)", buttons[0].LayoutX, buttons[0].LayoutY)
		}
	}

	// Undo again: Should remove the button (back to empty board)
	result, err = undoService.Undo(ctx, project.ID)
	if err != nil {
		t.Fatalf("Second undo failed: %v", err)
	}
	if !result.Success {
		t.Fatalf("Second undo was not successful: %s", result.Message)
	}

	// Verify button was removed again
	buttons, _ = lookBoardRepo.GetButtons(ctx, board.ID)
	if len(buttons) != 0 {
		t.Errorf("Expected 0 buttons after second undo, got %d", len(buttons))
	}

	// Redo: Should add button back
	result, err = undoService.Redo(ctx, project.ID)
	if err != nil {
		t.Fatalf("Redo failed: %v", err)
	}
	if !result.Success {
		t.Fatalf("Redo was not successful: %s", result.Message)
	}

	// Verify button was added again
	buttons, _ = lookBoardRepo.GetButtons(ctx, board.ID)
	if len(buttons) != 1 {
		t.Errorf("Expected 1 button after redo, got %d", len(buttons))
	}
}

// TestUndoRedoLookBoardButtonPositions_Integration tests button position updates undo/redo
func TestUndoRedoLookBoardButtonPositions_Integration(t *testing.T) {
	db, cleanup := setupUndoTestDB(t)
	defer cleanup()

	undoService, projectRepo, _, _, lookRepo, _, lookBoardRepo, _ := createUndoTestService(db)

	ctx := context.Background()

	// Setup: Create project, look, board with buttons
	project := &models.Project{ID: cuid.New(), Name: "Test Project"}
	_ = projectRepo.Create(ctx, project)

	look1 := &models.Look{ID: cuid.New(), ProjectID: project.ID, Name: "Look 1"}
	look2 := &models.Look{ID: cuid.New(), ProjectID: project.ID, Name: "Look 2"}
	_ = lookRepo.Create(ctx, look1)
	_ = lookRepo.Create(ctx, look2)

	board := &models.LookBoard{
		ID:              cuid.New(),
		ProjectID:       project.ID,
		Name:            "Position Test Board",
		DefaultFadeTime: 3.0,
		GridSize:        intPtr(50),
		CanvasWidth:     2000,
		CanvasHeight:    2000,
	}
	_ = lookBoardRepo.Create(ctx, board)

	// Create two buttons
	button1 := &models.LookBoardButton{
		ID:          cuid.New(),
		LookBoardID: board.ID,
		LookID:      look1.ID,
		LayoutX:     100,
		LayoutY:     100,
		Width:       intPtr(200),
		Height:      intPtr(120),
		Label:       strPtr("Button 1"),
	}
	button2 := &models.LookBoardButton{
		ID:          cuid.New(),
		LookBoardID: board.ID,
		LookID:      look2.ID,
		LayoutX:     400,
		LayoutY:     100,
		Width:       intPtr(200),
		Height:      intPtr(120),
		Label:       strPtr("Button 2"),
	}
	_ = lookBoardRepo.CreateButton(ctx, button1)
	_ = lookBoardRepo.CreateButton(ctx, button2)

	// Capture state before position update
	prevState, _ := undoService.CaptureLookBoardState(ctx, board.ID)

	// Update button positions (simulating drag)
	button1.LayoutX = 200
	button1.LayoutY = 300
	button2.LayoutX = 500
	button2.LayoutY = 300
	_ = lookBoardRepo.UpdateButton(ctx, button1)
	_ = lookBoardRepo.UpdateButton(ctx, button2)

	// Capture state after position update
	newState, _ := undoService.CaptureLookBoardState(ctx, board.ID)
	_ = undoService.RecordOperation(ctx, project.ID, undo.OperationTypeUpdate, undo.EntityTypeLookBoard, board.ID,
		"Update button positions", prevState, newState, nil)

	// Verify new positions
	buttons, _ := lookBoardRepo.GetButtons(ctx, board.ID)
	buttonMap := make(map[string]models.LookBoardButton)
	for _, b := range buttons {
		if b.Label != nil {
			buttonMap[*b.Label] = b
		}
	}
	if buttonMap["Button 1"].LayoutX != 200 || buttonMap["Button 1"].LayoutY != 300 {
		t.Errorf("Expected Button 1 at (200, 300), got (%d, %d)",
			buttonMap["Button 1"].LayoutX, buttonMap["Button 1"].LayoutY)
	}
	if buttonMap["Button 2"].LayoutX != 500 || buttonMap["Button 2"].LayoutY != 300 {
		t.Errorf("Expected Button 2 at (500, 300), got (%d, %d)",
			buttonMap["Button 2"].LayoutX, buttonMap["Button 2"].LayoutY)
	}

	// Undo: Should restore original positions
	result, err := undoService.Undo(ctx, project.ID)
	if err != nil {
		t.Fatalf("Undo failed: %v", err)
	}
	if !result.Success {
		t.Fatalf("Undo was not successful: %s", result.Message)
	}

	// Verify original positions restored
	buttons, _ = lookBoardRepo.GetButtons(ctx, board.ID)
	buttonMap = make(map[string]models.LookBoardButton)
	for _, b := range buttons {
		if b.Label != nil {
			buttonMap[*b.Label] = b
		}
	}
	if buttonMap["Button 1"].LayoutX != 100 || buttonMap["Button 1"].LayoutY != 100 {
		t.Errorf("Expected Button 1 at (100, 100) after undo, got (%d, %d)",
			buttonMap["Button 1"].LayoutX, buttonMap["Button 1"].LayoutY)
	}
	if buttonMap["Button 2"].LayoutX != 400 || buttonMap["Button 2"].LayoutY != 100 {
		t.Errorf("Expected Button 2 at (400, 100) after undo, got (%d, %d)",
			buttonMap["Button 2"].LayoutX, buttonMap["Button 2"].LayoutY)
	}

	// Redo: Should apply new positions again
	result, err = undoService.Redo(ctx, project.ID)
	if err != nil {
		t.Fatalf("Redo failed: %v", err)
	}
	if !result.Success {
		t.Fatalf("Redo was not successful: %s", result.Message)
	}

	// Verify new positions again
	buttons, _ = lookBoardRepo.GetButtons(ctx, board.ID)
	buttonMap = make(map[string]models.LookBoardButton)
	for _, b := range buttons {
		if b.Label != nil {
			buttonMap[*b.Label] = b
		}
	}
	if buttonMap["Button 1"].LayoutX != 200 || buttonMap["Button 1"].LayoutY != 300 {
		t.Errorf("Expected Button 1 at (200, 300) after redo, got (%d, %d)",
			buttonMap["Button 1"].LayoutX, buttonMap["Button 1"].LayoutY)
	}
	if buttonMap["Button 2"].LayoutX != 500 || buttonMap["Button 2"].LayoutY != 300 {
		t.Errorf("Expected Button 2 at (500, 300) after redo, got (%d, %d)",
			buttonMap["Button 2"].LayoutX, buttonMap["Button 2"].LayoutY)
	}
}

// TestUndoRedoLookBoardDelete_Integration tests deleting a look board with buttons
func TestUndoRedoLookBoardDelete_Integration(t *testing.T) {
	db, cleanup := setupUndoTestDB(t)
	defer cleanup()

	undoService, projectRepo, _, _, lookRepo, _, lookBoardRepo, _ := createUndoTestService(db)

	ctx := context.Background()

	// Setup: Create project, look, board with buttons
	project := &models.Project{ID: cuid.New(), Name: "Test Project"}
	_ = projectRepo.Create(ctx, project)

	look := &models.Look{ID: cuid.New(), ProjectID: project.ID, Name: "Test Look"}
	_ = lookRepo.Create(ctx, look)

	board := &models.LookBoard{
		ID:              cuid.New(),
		ProjectID:       project.ID,
		Name:            "Board To Delete",
		DefaultFadeTime: 3.0,
		GridSize:        intPtr(50),
		CanvasWidth:     2000,
		CanvasHeight:    2000,
	}
	_ = lookBoardRepo.Create(ctx, board)

	// Add buttons
	button1 := &models.LookBoardButton{
		ID:          cuid.New(),
		LookBoardID: board.ID,
		LookID:      look.ID,
		LayoutX:     100,
		LayoutY:     100,
		Width:       intPtr(200),
		Height:      intPtr(120),
		Label:       strPtr("Button 1"),
		Color:       strPtr("#FF0000"),
	}
	button2 := &models.LookBoardButton{
		ID:          cuid.New(),
		LookBoardID: board.ID,
		LookID:      look.ID,
		LayoutX:     400,
		LayoutY:     100,
		Width:       intPtr(200),
		Height:      intPtr(120),
		Label:       strPtr("Button 2"),
		Color:       strPtr("#00FF00"),
	}
	_ = lookBoardRepo.CreateButton(ctx, button1)
	_ = lookBoardRepo.CreateButton(ctx, button2)

	// Capture state before delete
	prevState, err := undoService.CaptureLookBoardState(ctx, board.ID)
	if err != nil {
		t.Fatalf("Failed to capture look board state: %v", err)
	}

	// Delete the board (buttons are deleted by cascade in repository)
	if err := lookBoardRepo.Delete(ctx, board.ID); err != nil {
		t.Fatalf("Failed to delete look board: %v", err)
	}

	// Record delete operation
	_ = undoService.RecordOperation(ctx, project.ID, undo.OperationTypeDelete, undo.EntityTypeLookBoard, board.ID,
		"Delete look board 'Board To Delete'", prevState, nil, nil)

	// Verify board and buttons are deleted
	deletedBoard, _ := lookBoardRepo.FindByID(ctx, board.ID)
	if deletedBoard != nil {
		t.Error("Expected board to be deleted")
	}
	buttons, _ := lookBoardRepo.GetButtons(ctx, board.ID)
	if len(buttons) != 0 {
		t.Errorf("Expected 0 buttons after delete, got %d", len(buttons))
	}

	// Undo: Should restore board and buttons
	result, err := undoService.Undo(ctx, project.ID)
	if err != nil {
		t.Fatalf("Undo failed: %v", err)
	}
	if !result.Success {
		t.Fatalf("Undo was not successful: %s", result.Message)
	}

	// Verify board was restored
	restoredBoard, err := lookBoardRepo.FindByID(ctx, board.ID)
	if err != nil {
		t.Fatalf("Failed to find restored board: %v", err)
	}
	if restoredBoard.Name != "Board To Delete" {
		t.Errorf("Expected board name 'Board To Delete', got '%s'", restoredBoard.Name)
	}

	// Verify buttons were restored
	buttons, _ = lookBoardRepo.GetButtons(ctx, board.ID)
	if len(buttons) != 2 {
		t.Errorf("Expected 2 buttons after undo, got %d", len(buttons))
	}
	buttonLabels := make(map[string]bool)
	for _, b := range buttons {
		if b.Label != nil {
			buttonLabels[*b.Label] = true
		}
	}
	if !buttonLabels["Button 1"] || !buttonLabels["Button 2"] {
		t.Error("Expected both buttons to be restored")
	}

	// Redo: Should delete board and buttons again
	result, err = undoService.Redo(ctx, project.ID)
	if err != nil {
		t.Fatalf("Redo failed: %v", err)
	}
	if !result.Success {
		t.Fatalf("Redo was not successful: %s", result.Message)
	}

	// Verify board is deleted again
	deletedBoard, _ = lookBoardRepo.FindByID(ctx, board.ID)
	if deletedBoard != nil {
		t.Error("Expected board to be deleted after redo")
	}
}

// TestUndoRedoEffectWorkflow_Integration tests effect create/update/delete/undo/redo
func TestUndoRedoEffectWorkflow_Integration(t *testing.T) {
	db, cleanup := setupUndoTestDB(t)
	defer cleanup()

	undoService, projectRepo, _, _, _, _, _, effectRepo := createUndoTestService(db)

	ctx := context.Background()

	// Setup: Create project
	project := &models.Project{ID: cuid.New(), Name: "Test Project"}
	if err := projectRepo.Create(ctx, project); err != nil {
		t.Fatalf("Failed to create project: %v", err)
	}

	// Step 1: Create an effect
	effect := &models.Effect{
		ID:         cuid.New(),
		ProjectID:  project.ID,
		Name:       "Pulse Effect",
		EffectType: "WAVEFORM",
		Frequency:  1.0,
	}
	if err := effectRepo.Create(ctx, effect); err != nil {
		t.Fatalf("Failed to create effect: %v", err)
	}

	// Record create operation
	newState, err := undoService.CaptureEffectState(ctx, effect.ID)
	if err != nil {
		t.Fatalf("Failed to capture effect state: %v", err)
	}
	if err := undoService.RecordOperation(ctx, project.ID, undo.OperationTypeCreate, undo.EntityTypeEffect, effect.ID,
		"Create effect 'Pulse Effect'", nil, newState, nil); err != nil {
		t.Fatalf("Failed to record create operation: %v", err)
	}

	// Step 2: Update the effect
	prevState, err := undoService.CaptureEffectState(ctx, effect.ID)
	if err != nil {
		t.Fatalf("Failed to capture previous state: %v", err)
	}

	effect.Name = "Fast Pulse"
	effect.Frequency = 2.0
	if err := effectRepo.Update(ctx, effect); err != nil {
		t.Fatalf("Failed to update effect: %v", err)
	}

	newState, err = undoService.CaptureEffectState(ctx, effect.ID)
	if err != nil {
		t.Fatalf("Failed to capture new state: %v", err)
	}
	if err := undoService.RecordOperation(ctx, project.ID, undo.OperationTypeUpdate, undo.EntityTypeEffect, effect.ID,
		"Update effect 'Fast Pulse'", prevState, newState, nil); err != nil {
		t.Fatalf("Failed to record update operation: %v", err)
	}

	// Verify current state
	currentEffect, err := effectRepo.FindByID(ctx, effect.ID)
	if err != nil {
		t.Fatalf("Failed to find effect: %v", err)
	}
	if currentEffect.Name != "Fast Pulse" {
		t.Errorf("Expected effect name 'Fast Pulse', got '%s'", currentEffect.Name)
	}
	if currentEffect.Frequency != 2.0 {
		t.Errorf("Expected frequency 2.0, got %f", currentEffect.Frequency)
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
	currentEffect, err = effectRepo.FindByID(ctx, effect.ID)
	if err != nil {
		t.Fatalf("Failed to find effect after undo: %v", err)
	}
	if currentEffect.Name != "Pulse Effect" {
		t.Errorf("Expected effect name 'Pulse Effect' after undo, got '%s'", currentEffect.Name)
	}
	if currentEffect.Frequency != 1.0 {
		t.Errorf("Expected frequency 1.0 after undo, got %f", currentEffect.Frequency)
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
	currentEffect, err = effectRepo.FindByID(ctx, effect.ID)
	if err != nil {
		t.Fatalf("Failed to find effect after redo: %v", err)
	}
	if currentEffect.Name != "Fast Pulse" {
		t.Errorf("Expected effect name 'Fast Pulse' after redo, got '%s'", currentEffect.Name)
	}

	// Step 5: Undo both operations (delete the effect)
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

	// Verify effect was deleted
	currentEffect, err = effectRepo.FindByID(ctx, effect.ID)
	if err == nil && currentEffect != nil {
		t.Errorf("Expected effect to be deleted after undo create, but it still exists")
	}

	// Step 6: Redo should recreate the effect
	result, err = undoService.Redo(ctx, project.ID)
	if err != nil {
		t.Fatalf("Redo create failed: %v", err)
	}
	if !result.Success {
		t.Fatalf("Redo create was not successful: %s", result.Message)
	}

	// Verify effect was recreated
	currentEffect, err = effectRepo.FindByID(ctx, effect.ID)
	if err != nil {
		t.Fatalf("Failed to find effect after redo create: %v", err)
	}
	if currentEffect.Name != "Pulse Effect" {
		t.Errorf("Expected effect name 'Pulse Effect' after redo create, got '%s'", currentEffect.Name)
	}
}

// TestUndoRedoEffectWithFixtures_Integration tests effect with fixtures undo/redo
func TestUndoRedoEffectWithFixtures_Integration(t *testing.T) {
	db, cleanup := setupUndoTestDB(t)
	defer cleanup()

	undoService, projectRepo, _, _, _, fixtureRepo, _, effectRepo := createUndoTestService(db)

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

	// Create an effect with fixtures
	effect := &models.Effect{
		ID:         cuid.New(),
		ProjectID:  project.ID,
		Name:       "Fixture Effect",
		EffectType: "WAVEFORM",
		Frequency:  1.0,
	}
	if err := effectRepo.Create(ctx, effect); err != nil {
		t.Fatalf("Failed to create effect: %v", err)
	}

	// Add a fixture to the effect
	phaseOffset := 0.0
	amplitudeScale := 1.0
	if err := effectRepo.AddFixtureToEffect(ctx, effect.ID, fixture.ID, &phaseOffset, &amplitudeScale); err != nil {
		t.Fatalf("Failed to add fixture to effect: %v", err)
	}

	// Get the effect fixture ID to add a channel
	effectFixtures, err := effectRepo.GetEffectFixtures(ctx, effect.ID)
	if err != nil || len(effectFixtures) == 0 {
		t.Fatalf("Failed to get effect fixtures: %v", err)
	}
	effectFixtureID := effectFixtures[0].ID

	// Add a channel to the fixture
	channelOffset := 0
	channelType := "dimmer"
	channelAmplitude := 1.0
	channelFrequency := 1.0
	if err := effectRepo.AddChannelToEffectFixtureWithScales(ctx, effectFixtureID, &channelOffset, &channelType, &channelAmplitude, &channelFrequency); err != nil {
		t.Fatalf("Failed to add channel to fixture: %v", err)
	}

	// Record state after adding fixtures and channels
	newState, err := undoService.CaptureEffectState(ctx, effect.ID)
	if err != nil {
		t.Fatalf("Failed to capture effect state: %v", err)
	}
	if err := undoService.RecordOperation(ctx, project.ID, undo.OperationTypeCreate, undo.EntityTypeEffect, effect.ID,
		"Create effect 'Fixture Effect' with fixtures", nil, newState, nil); err != nil {
		t.Fatalf("Failed to record create operation: %v", err)
	}

	// Verify effect has fixtures
	fixtures, err := effectRepo.GetEffectFixtures(ctx, effect.ID)
	if err != nil {
		t.Fatalf("Failed to get effect fixtures: %v", err)
	}
	if len(fixtures) != 1 {
		t.Fatalf("Expected 1 fixture, got %d", len(fixtures))
	}

	// Capture state before delete
	prevState, err := undoService.CaptureEffectState(ctx, effect.ID)
	if err != nil {
		t.Fatalf("Failed to capture state before delete: %v", err)
	}

	// Delete effect (cascade deletes fixtures and channels)
	if err := effectRepo.Delete(ctx, effect.ID); err != nil {
		t.Fatalf("Failed to delete effect: %v", err)
	}

	// Record delete operation
	if err := undoService.RecordOperation(ctx, project.ID, undo.OperationTypeDelete, undo.EntityTypeEffect, effect.ID,
		"Delete effect 'Fixture Effect'", prevState, nil, nil); err != nil {
		t.Fatalf("Failed to record delete operation: %v", err)
	}

	// Verify effect is deleted
	deletedEffect, err := effectRepo.FindByID(ctx, effect.ID)
	if err == nil && deletedEffect != nil {
		t.Error("Expected effect to be deleted")
	}

	// Undo should restore effect with fixtures
	result, err := undoService.Undo(ctx, project.ID)
	if err != nil {
		t.Fatalf("Undo failed: %v", err)
	}
	if !result.Success {
		t.Fatalf("Undo was not successful: %s", result.Message)
	}

	// Verify effect was restored
	restoredEffect, err := effectRepo.FindByID(ctx, effect.ID)
	if err != nil {
		t.Fatalf("Failed to find restored effect: %v", err)
	}
	if restoredEffect.Name != "Fixture Effect" {
		t.Errorf("Expected effect name 'Fixture Effect', got '%s'", restoredEffect.Name)
	}

	// Verify fixtures were restored
	restoredFixtures, err := effectRepo.GetEffectFixtures(ctx, effect.ID)
	if err != nil {
		t.Fatalf("Failed to get restored fixtures: %v", err)
	}
	if len(restoredFixtures) != 1 {
		t.Errorf("Expected 1 fixture after undo, got %d", len(restoredFixtures))
	}

	// Verify channels were restored
	if len(restoredFixtures) > 0 {
		channels, err := effectRepo.GetEffectChannels(ctx, restoredFixtures[0].ID)
		if err != nil {
			t.Fatalf("Failed to get restored channels: %v", err)
		}
		if len(channels) != 1 {
			t.Errorf("Expected 1 channel after undo, got %d", len(channels))
		}
	}
}

// TestUndoRedoCueEffectWorkflow_Integration tests cue effect add/remove/undo/redo
func TestUndoRedoCueEffectWorkflow_Integration(t *testing.T) {
	db, cleanup := setupUndoTestDB(t)
	defer cleanup()

	undoService, projectRepo, cueRepo, cueListRepo, lookRepo, _, _, effectRepo := createUndoTestService(db)

	ctx := context.Background()

	// Setup: Create project, look, cue list, cue, and effect
	project := &models.Project{ID: cuid.New(), Name: "Test Project"}
	if err := projectRepo.Create(ctx, project); err != nil {
		t.Fatalf("Failed to create project: %v", err)
	}

	look := &models.Look{ID: cuid.New(), ProjectID: project.ID, Name: "Test Look"}
	if err := lookRepo.Create(ctx, look); err != nil {
		t.Fatalf("Failed to create look: %v", err)
	}

	cueList := &models.CueList{ID: cuid.New(), ProjectID: project.ID, Name: "Test Cue List"}
	if err := cueListRepo.Create(ctx, cueList); err != nil {
		t.Fatalf("Failed to create cue list: %v", err)
	}

	cue := &models.Cue{
		ID:         cuid.New(),
		CueListID:  cueList.ID,
		LookID:     look.ID,
		Name:       "Test Cue",
		CueNumber:  1.0,
		FadeInTime: 3.0,
	}
	if err := cueRepo.Create(ctx, cue); err != nil {
		t.Fatalf("Failed to create cue: %v", err)
	}

	effect := &models.Effect{
		ID:         cuid.New(),
		ProjectID:  project.ID,
		Name:       "Test Effect",
		EffectType: "WAVEFORM",
		Frequency:  1.0,
	}
	if err := effectRepo.Create(ctx, effect); err != nil {
		t.Fatalf("Failed to create effect: %v", err)
	}

	// Step 1: Add effect to cue
	intensity := 100.0
	speed := 1.0
	if err := effectRepo.AddEffectToCue(ctx, cue.ID, effect.ID, intensity, speed); err != nil {
		t.Fatalf("Failed to add effect to cue: %v", err)
	}

	// Record add operation
	newState, err := undoService.CaptureCueEffectState(ctx, cue.ID, effect.ID)
	if err != nil {
		t.Fatalf("Failed to capture cue effect state: %v", err)
	}
	entityID := cue.ID + ":" + effect.ID
	if err := undoService.RecordOperation(ctx, project.ID, undo.OperationTypeCreate, undo.EntityTypeCueEffect, entityID,
		"Add effect 'Test Effect' to cue", nil, newState, nil); err != nil {
		t.Fatalf("Failed to record add operation: %v", err)
	}

	// Verify cue effect exists
	cueEffect, err := effectRepo.GetCueEffect(ctx, cue.ID, effect.ID)
	if err != nil {
		t.Fatalf("Failed to get cue effect: %v", err)
	}
	if cueEffect.Intensity != 100.0 {
		t.Errorf("Expected intensity 100.0, got %f", cueEffect.Intensity)
	}

	// Step 2: Undo add - should remove cue effect
	result, err := undoService.Undo(ctx, project.ID)
	if err != nil {
		t.Fatalf("Undo failed: %v", err)
	}
	if !result.Success {
		t.Fatalf("Undo was not successful: %s", result.Message)
	}

	// Verify cue effect was removed
	removedEffect, err := effectRepo.GetCueEffect(ctx, cue.ID, effect.ID)
	if err == nil && removedEffect != nil {
		t.Error("Expected cue effect to be removed after undo")
	}

	// Step 3: Redo add - should restore cue effect
	result, err = undoService.Redo(ctx, project.ID)
	if err != nil {
		t.Fatalf("Redo failed: %v", err)
	}
	if !result.Success {
		t.Fatalf("Redo was not successful: %s", result.Message)
	}

	// Verify cue effect was restored
	restoredEffect, err := effectRepo.GetCueEffect(ctx, cue.ID, effect.ID)
	if err != nil {
		t.Fatalf("Failed to get restored cue effect: %v", err)
	}
	if restoredEffect.Intensity != 100.0 {
		t.Errorf("Expected intensity 100.0 after redo, got %f", restoredEffect.Intensity)
	}

	// Step 4: Capture state before remove
	prevState, err := undoService.CaptureCueEffectState(ctx, cue.ID, effect.ID)
	if err != nil {
		t.Fatalf("Failed to capture state before remove: %v", err)
	}

	// Remove the effect from cue
	if err := effectRepo.RemoveEffectFromCue(ctx, cue.ID, effect.ID); err != nil {
		t.Fatalf("Failed to remove effect from cue: %v", err)
	}

	// Record remove operation
	if err := undoService.RecordOperation(ctx, project.ID, undo.OperationTypeDelete, undo.EntityTypeCueEffect, entityID,
		"Remove effect 'Test Effect' from cue", prevState, nil, nil); err != nil {
		t.Fatalf("Failed to record remove operation: %v", err)
	}

	// Verify cue effect was removed
	removedEffect, err = effectRepo.GetCueEffect(ctx, cue.ID, effect.ID)
	if err == nil && removedEffect != nil {
		t.Error("Expected cue effect to be removed")
	}

	// Step 5: Undo remove - should restore cue effect
	result, err = undoService.Undo(ctx, project.ID)
	if err != nil {
		t.Fatalf("Undo remove failed: %v", err)
	}
	if !result.Success {
		t.Fatalf("Undo remove was not successful: %s", result.Message)
	}

	// Verify cue effect was restored
	restoredEffect, err = effectRepo.GetCueEffect(ctx, cue.ID, effect.ID)
	if err != nil {
		t.Fatalf("Failed to get restored cue effect after undo remove: %v", err)
	}
	if restoredEffect.Intensity != 100.0 {
		t.Errorf("Expected intensity 100.0 after undo remove, got %f", restoredEffect.Intensity)
	}
}

// TestUndoRedoBulkFixtureUpdate_Integration tests undo/redo for bulk fixture updates
func TestUndoRedoBulkFixtureUpdate_Integration(t *testing.T) {
	db, cleanup := setupUndoTestDB(t)
	defer cleanup()

	undoService, projectRepo, _, _, _, fixtureRepo, _, _ := createUndoTestService(db)

	ctx := context.Background()

	// Setup: Create project and fixture definition
	project := &models.Project{ID: cuid.New(), Name: "Test Project"}
	if err := projectRepo.Create(ctx, project); err != nil {
		t.Fatalf("Failed to create project: %v", err)
	}

	// Create fixture definition
	definition := &models.FixtureDefinition{
		ID:           cuid.New(),
		Manufacturer: "Test",
		Model:        "Fixture",
		Type:         "LED_PAR",
	}
	if err := db.Create(definition).Error; err != nil {
		t.Fatalf("Failed to create fixture definition: %v", err)
	}

	// Create two fixtures
	fixture1 := &models.FixtureInstance{
		ID:           cuid.New(),
		Name:         "Fixture 1",
		ProjectID:    project.ID,
		DefinitionID: definition.ID,
		Universe:     1,
		StartChannel: 1,
	}
	fixture2 := &models.FixtureInstance{
		ID:           cuid.New(),
		Name:         "Fixture 2",
		ProjectID:    project.ID,
		DefinitionID: definition.ID,
		Universe:     1,
		StartChannel: 10,
	}
	if err := fixtureRepo.Create(ctx, fixture1); err != nil {
		t.Fatalf("Failed to create fixture1: %v", err)
	}
	if err := fixtureRepo.Create(ctx, fixture2); err != nil {
		t.Fatalf("Failed to create fixture2: %v", err)
	}

	// Step 1: Simulate bulk update by recording individual operations
	// Capture previous state for fixture 1
	prevState1, err := undoService.CaptureFixtureState(ctx, fixture1.ID)
	if err != nil {
		t.Fatalf("Failed to capture fixture1 previous state: %v", err)
	}

	// Update fixture 1
	fixture1.Name = "Updated Fixture 1"
	if err := fixtureRepo.Update(ctx, fixture1); err != nil {
		t.Fatalf("Failed to update fixture1: %v", err)
	}

	// Capture new state and record operation
	newState1, err := undoService.CaptureFixtureState(ctx, fixture1.ID)
	if err != nil {
		t.Fatalf("Failed to capture fixture1 new state: %v", err)
	}
	if err := undoService.RecordOperation(ctx, project.ID, undo.OperationTypeUpdate, undo.EntityTypeFixtureInstance, fixture1.ID,
		"Update fixture 'Fixture 1'", prevState1, newState1, nil); err != nil {
		t.Fatalf("Failed to record fixture1 update: %v", err)
	}

	// Capture previous state for fixture 2
	prevState2, err := undoService.CaptureFixtureState(ctx, fixture2.ID)
	if err != nil {
		t.Fatalf("Failed to capture fixture2 previous state: %v", err)
	}

	// Update fixture 2
	fixture2.Name = "Updated Fixture 2"
	if err := fixtureRepo.Update(ctx, fixture2); err != nil {
		t.Fatalf("Failed to update fixture2: %v", err)
	}

	// Capture new state and record operation
	newState2, err := undoService.CaptureFixtureState(ctx, fixture2.ID)
	if err != nil {
		t.Fatalf("Failed to capture fixture2 new state: %v", err)
	}
	if err := undoService.RecordOperation(ctx, project.ID, undo.OperationTypeUpdate, undo.EntityTypeFixtureInstance, fixture2.ID,
		"Update fixture 'Fixture 2'", prevState2, newState2, nil); err != nil {
		t.Fatalf("Failed to record fixture2 update: %v", err)
	}

	// Step 2: Undo fixture 2 update (most recent)
	result, err := undoService.Undo(ctx, project.ID)
	if err != nil {
		t.Fatalf("Undo fixture2 failed: %v", err)
	}
	if !result.Success {
		t.Fatalf("Undo fixture2 was not successful: %s", result.Message)
	}

	// Verify fixture 2 was reverted
	f2, _ := fixtureRepo.FindByID(ctx, fixture2.ID)
	if f2.Name != "Fixture 2" {
		t.Errorf("Expected fixture2 name 'Fixture 2' after undo, got '%s'", f2.Name)
	}

	// Step 3: Undo fixture 1 update
	result, err = undoService.Undo(ctx, project.ID)
	if err != nil {
		t.Fatalf("Undo fixture1 failed: %v", err)
	}
	if !result.Success {
		t.Fatalf("Undo fixture1 was not successful: %s", result.Message)
	}

	// Verify fixture 1 was reverted
	f1, _ := fixtureRepo.FindByID(ctx, fixture1.ID)
	if f1.Name != "Fixture 1" {
		t.Errorf("Expected fixture1 name 'Fixture 1' after undo, got '%s'", f1.Name)
	}

	// Step 4: Redo both updates
	_, err = undoService.Redo(ctx, project.ID)
	if err != nil {
		t.Fatalf("Redo fixture1 failed: %v", err)
	}
	f1, _ = fixtureRepo.FindByID(ctx, fixture1.ID)
	if f1.Name != "Updated Fixture 1" {
		t.Errorf("Expected fixture1 name 'Updated Fixture 1' after redo, got '%s'", f1.Name)
	}

	_, err = undoService.Redo(ctx, project.ID)
	if err != nil {
		t.Fatalf("Redo fixture2 failed: %v", err)
	}
	f2, _ = fixtureRepo.FindByID(ctx, fixture2.ID)
	if f2.Name != "Updated Fixture 2" {
		t.Errorf("Expected fixture2 name 'Updated Fixture 2' after redo, got '%s'", f2.Name)
	}
}

// TestUndoRedoBulkLookDelete_Integration tests undo/redo for bulk look deletes
func TestUndoRedoBulkLookDelete_Integration(t *testing.T) {
	db, cleanup := setupUndoTestDB(t)
	defer cleanup()

	undoService, projectRepo, _, _, lookRepo, _, _, _ := createUndoTestService(db)

	ctx := context.Background()

	// Setup: Create project
	project := &models.Project{ID: cuid.New(), Name: "Test Project"}
	if err := projectRepo.Create(ctx, project); err != nil {
		t.Fatalf("Failed to create project: %v", err)
	}

	// Create two looks
	look1 := &models.Look{
		ID:        cuid.New(),
		Name:      "Look 1",
		ProjectID: project.ID,
	}
	look2 := &models.Look{
		ID:        cuid.New(),
		Name:      "Look 2",
		ProjectID: project.ID,
	}
	if err := lookRepo.Create(ctx, look1); err != nil {
		t.Fatalf("Failed to create look1: %v", err)
	}
	if err := lookRepo.Create(ctx, look2); err != nil {
		t.Fatalf("Failed to create look2: %v", err)
	}

	// Step 1: Simulate bulk delete by recording individual delete operations
	// Capture previous state for look 1
	prevState1, err := undoService.CaptureLookState(ctx, look1.ID)
	if err != nil {
		t.Fatalf("Failed to capture look1 previous state: %v", err)
	}

	// Delete look 1
	if err := lookRepo.Delete(ctx, look1.ID); err != nil {
		t.Fatalf("Failed to delete look1: %v", err)
	}

	// Record delete operation
	if err := undoService.RecordOperation(ctx, project.ID, undo.OperationTypeDelete, undo.EntityTypeLook, look1.ID,
		"Delete look 'Look 1'", prevState1, nil, nil); err != nil {
		t.Fatalf("Failed to record look1 delete: %v", err)
	}

	// Capture previous state for look 2
	prevState2, err := undoService.CaptureLookState(ctx, look2.ID)
	if err != nil {
		t.Fatalf("Failed to capture look2 previous state: %v", err)
	}

	// Delete look 2
	if err := lookRepo.Delete(ctx, look2.ID); err != nil {
		t.Fatalf("Failed to delete look2: %v", err)
	}

	// Record delete operation
	if err := undoService.RecordOperation(ctx, project.ID, undo.OperationTypeDelete, undo.EntityTypeLook, look2.ID,
		"Delete look 'Look 2'", prevState2, nil, nil); err != nil {
		t.Fatalf("Failed to record look2 delete: %v", err)
	}

	// Verify both looks are deleted
	l1, _ := lookRepo.FindByID(ctx, look1.ID)
	if l1 != nil {
		t.Error("Expected look1 to be deleted")
	}
	l2, _ := lookRepo.FindByID(ctx, look2.ID)
	if l2 != nil {
		t.Error("Expected look2 to be deleted")
	}

	// Step 2: Undo look 2 delete (most recent)
	result, err := undoService.Undo(ctx, project.ID)
	if err != nil {
		t.Fatalf("Undo look2 delete failed: %v", err)
	}
	if !result.Success {
		t.Fatalf("Undo look2 delete was not successful: %s", result.Message)
	}

	// Verify look 2 was restored
	l2, _ = lookRepo.FindByID(ctx, look2.ID)
	if l2 == nil {
		t.Fatal("Expected look2 to be restored")
	}
	if l2.Name != "Look 2" {
		t.Errorf("Expected look2 name 'Look 2' after undo, got '%s'", l2.Name)
	}

	// Step 3: Undo look 1 delete
	result, err = undoService.Undo(ctx, project.ID)
	if err != nil {
		t.Fatalf("Undo look1 delete failed: %v", err)
	}
	if !result.Success {
		t.Fatalf("Undo look1 delete was not successful: %s", result.Message)
	}

	// Verify look 1 was restored
	l1, _ = lookRepo.FindByID(ctx, look1.ID)
	if l1 == nil {
		t.Fatal("Expected look1 to be restored")
	}
	if l1.Name != "Look 1" {
		t.Errorf("Expected look1 name 'Look 1' after undo, got '%s'", l1.Name)
	}

	// Step 4: Redo both deletes
	_, err = undoService.Redo(ctx, project.ID)
	if err != nil {
		t.Fatalf("Redo look1 delete failed: %v", err)
	}
	l1, _ = lookRepo.FindByID(ctx, look1.ID)
	if l1 != nil {
		t.Error("Expected look1 to be deleted after redo")
	}

	_, err = undoService.Redo(ctx, project.ID)
	if err != nil {
		t.Fatalf("Redo look2 delete failed: %v", err)
	}
	l2, _ = lookRepo.FindByID(ctx, look2.ID)
	if l2 != nil {
		t.Error("Expected look2 to be deleted after redo")
	}
}

// TestUndoRedoBulkCueListUpdate_Integration tests undo/redo for bulk cue list updates
func TestUndoRedoBulkCueListUpdate_Integration(t *testing.T) {
	db, cleanup := setupUndoTestDB(t)
	defer cleanup()

	undoService, projectRepo, _, cueListRepo, _, _, _, _ := createUndoTestService(db)

	ctx := context.Background()

	// Setup: Create project
	project := &models.Project{ID: cuid.New(), Name: "Test Project"}
	if err := projectRepo.Create(ctx, project); err != nil {
		t.Fatalf("Failed to create project: %v", err)
	}

	// Create two cue lists
	cueList1 := &models.CueList{
		ID:        cuid.New(),
		Name:      "Cue List 1",
		ProjectID: project.ID,
		Loop:      false,
	}
	cueList2 := &models.CueList{
		ID:        cuid.New(),
		Name:      "Cue List 2",
		ProjectID: project.ID,
		Loop:      false,
	}
	if err := cueListRepo.Create(ctx, cueList1); err != nil {
		t.Fatalf("Failed to create cueList1: %v", err)
	}
	if err := cueListRepo.Create(ctx, cueList2); err != nil {
		t.Fatalf("Failed to create cueList2: %v", err)
	}

	// Step 1: Simulate bulk update by recording individual operations
	// Capture previous state for cue list 1
	prevState1, err := undoService.CaptureCueListState(ctx, cueList1.ID)
	if err != nil {
		t.Fatalf("Failed to capture cueList1 previous state: %v", err)
	}

	// Update cue list 1
	cueList1.Loop = true
	if err := cueListRepo.Update(ctx, cueList1); err != nil {
		t.Fatalf("Failed to update cueList1: %v", err)
	}

	// Capture new state and record operation
	newState1, err := undoService.CaptureCueListState(ctx, cueList1.ID)
	if err != nil {
		t.Fatalf("Failed to capture cueList1 new state: %v", err)
	}
	if err := undoService.RecordOperation(ctx, project.ID, undo.OperationTypeUpdate, undo.EntityTypeCueList, cueList1.ID,
		"Update cue list 'Cue List 1'", prevState1, newState1, nil); err != nil {
		t.Fatalf("Failed to record cueList1 update: %v", err)
	}

	// Capture previous state for cue list 2
	prevState2, err := undoService.CaptureCueListState(ctx, cueList2.ID)
	if err != nil {
		t.Fatalf("Failed to capture cueList2 previous state: %v", err)
	}

	// Update cue list 2
	cueList2.Loop = true
	if err := cueListRepo.Update(ctx, cueList2); err != nil {
		t.Fatalf("Failed to update cueList2: %v", err)
	}

	// Capture new state and record operation
	newState2, err := undoService.CaptureCueListState(ctx, cueList2.ID)
	if err != nil {
		t.Fatalf("Failed to capture cueList2 new state: %v", err)
	}
	if err := undoService.RecordOperation(ctx, project.ID, undo.OperationTypeUpdate, undo.EntityTypeCueList, cueList2.ID,
		"Update cue list 'Cue List 2'", prevState2, newState2, nil); err != nil {
		t.Fatalf("Failed to record cueList2 update: %v", err)
	}

	// Step 2: Undo cue list 2 update (most recent)
	result, err := undoService.Undo(ctx, project.ID)
	if err != nil {
		t.Fatalf("Undo cueList2 failed: %v", err)
	}
	if !result.Success {
		t.Fatalf("Undo cueList2 was not successful: %s", result.Message)
	}

	// Verify cue list 2 was reverted
	cl2, _ := cueListRepo.FindByID(ctx, cueList2.ID)
	if cl2.Loop != false {
		t.Errorf("Expected cueList2 loop false after undo, got %v", cl2.Loop)
	}

	// Step 3: Undo cue list 1 update
	result, err = undoService.Undo(ctx, project.ID)
	if err != nil {
		t.Fatalf("Undo cueList1 failed: %v", err)
	}
	if !result.Success {
		t.Fatalf("Undo cueList1 was not successful: %s", result.Message)
	}

	// Verify cue list 1 was reverted
	cl1, _ := cueListRepo.FindByID(ctx, cueList1.ID)
	if cl1.Loop != false {
		t.Errorf("Expected cueList1 loop false after undo, got %v", cl1.Loop)
	}

	// Step 4: Redo both updates
	_, err = undoService.Redo(ctx, project.ID)
	if err != nil {
		t.Fatalf("Redo cueList1 failed: %v", err)
	}
	cl1, _ = cueListRepo.FindByID(ctx, cueList1.ID)
	if cl1.Loop != true {
		t.Errorf("Expected cueList1 loop true after redo, got %v", cl1.Loop)
	}

	_, err = undoService.Redo(ctx, project.ID)
	if err != nil {
		t.Fatalf("Redo cueList2 failed: %v", err)
	}
	cl2, _ = cueListRepo.FindByID(ctx, cueList2.ID)
	if cl2.Loop != true {
		t.Errorf("Expected cueList2 loop true after redo, got %v", cl2.Loop)
	}
}

// TestUndoRedoBulkCueUpdate_Integration tests undo/redo for bulk cue updates
func TestUndoRedoBulkCueUpdate_Integration(t *testing.T) {
	db, cleanup := setupUndoTestDB(t)
	defer cleanup()

	undoService, projectRepo, cueRepo, cueListRepo, lookRepo, _, _, _ := createUndoTestService(db)

	ctx := context.Background()

	// Setup: Create project, look, and cue list
	project := &models.Project{ID: cuid.New(), Name: "Test Project"}
	if err := projectRepo.Create(ctx, project); err != nil {
		t.Fatalf("Failed to create project: %v", err)
	}

	look := &models.Look{
		ID:        cuid.New(),
		Name:      "Test Look",
		ProjectID: project.ID,
	}
	if err := lookRepo.Create(ctx, look); err != nil {
		t.Fatalf("Failed to create look: %v", err)
	}

	cueList := &models.CueList{
		ID:        cuid.New(),
		Name:      "Test Cue List",
		ProjectID: project.ID,
	}
	if err := cueListRepo.Create(ctx, cueList); err != nil {
		t.Fatalf("Failed to create cue list: %v", err)
	}

	// Create two cues
	cue1 := &models.Cue{
		ID:         cuid.New(),
		Name:       "Cue 1",
		CueNumber:  1.0,
		CueListID:  cueList.ID,
		LookID:     look.ID,
		FadeInTime: 3.0,
	}
	cue2 := &models.Cue{
		ID:         cuid.New(),
		Name:       "Cue 2",
		CueNumber:  2.0,
		CueListID:  cueList.ID,
		LookID:     look.ID,
		FadeInTime: 3.0,
	}
	if err := cueRepo.Create(ctx, cue1); err != nil {
		t.Fatalf("Failed to create cue1: %v", err)
	}
	if err := cueRepo.Create(ctx, cue2); err != nil {
		t.Fatalf("Failed to create cue2: %v", err)
	}

	// Step 1: Simulate bulk update (same fade time applied to multiple cues)
	// Capture previous state for cue 1
	prevState1, err := undoService.CaptureCueState(ctx, cue1.ID)
	if err != nil {
		t.Fatalf("Failed to capture cue1 previous state: %v", err)
	}

	// Update cue 1
	cue1.FadeInTime = 5.0
	if err := cueRepo.Update(ctx, cue1); err != nil {
		t.Fatalf("Failed to update cue1: %v", err)
	}

	// Capture new state and record operation
	newState1, err := undoService.CaptureCueState(ctx, cue1.ID)
	if err != nil {
		t.Fatalf("Failed to capture cue1 new state: %v", err)
	}
	if err := undoService.RecordOperation(ctx, project.ID, undo.OperationTypeUpdate, undo.EntityTypeCue, cue1.ID,
		"Update cue 'Cue 1'", prevState1, newState1, nil); err != nil {
		t.Fatalf("Failed to record cue1 update: %v", err)
	}

	// Capture previous state for cue 2
	prevState2, err := undoService.CaptureCueState(ctx, cue2.ID)
	if err != nil {
		t.Fatalf("Failed to capture cue2 previous state: %v", err)
	}

	// Update cue 2
	cue2.FadeInTime = 5.0
	if err := cueRepo.Update(ctx, cue2); err != nil {
		t.Fatalf("Failed to update cue2: %v", err)
	}

	// Capture new state and record operation
	newState2, err := undoService.CaptureCueState(ctx, cue2.ID)
	if err != nil {
		t.Fatalf("Failed to capture cue2 new state: %v", err)
	}
	if err := undoService.RecordOperation(ctx, project.ID, undo.OperationTypeUpdate, undo.EntityTypeCue, cue2.ID,
		"Update cue 'Cue 2'", prevState2, newState2, nil); err != nil {
		t.Fatalf("Failed to record cue2 update: %v", err)
	}

	// Step 2: Undo cue 2 update (most recent)
	result, err := undoService.Undo(ctx, project.ID)
	if err != nil {
		t.Fatalf("Undo cue2 failed: %v", err)
	}
	if !result.Success {
		t.Fatalf("Undo cue2 was not successful: %s", result.Message)
	}

	// Verify cue 2 was reverted
	c2, _ := cueRepo.FindByID(ctx, cue2.ID)
	if c2.FadeInTime != 3.0 {
		t.Errorf("Expected cue2 fadeInTime 3.0 after undo, got %f", c2.FadeInTime)
	}

	// Step 3: Undo cue 1 update
	result, err = undoService.Undo(ctx, project.ID)
	if err != nil {
		t.Fatalf("Undo cue1 failed: %v", err)
	}
	if !result.Success {
		t.Fatalf("Undo cue1 was not successful: %s", result.Message)
	}

	// Verify cue 1 was reverted
	c1, _ := cueRepo.FindByID(ctx, cue1.ID)
	if c1.FadeInTime != 3.0 {
		t.Errorf("Expected cue1 fadeInTime 3.0 after undo, got %f", c1.FadeInTime)
	}

	// Step 4: Redo both updates
	_, err = undoService.Redo(ctx, project.ID)
	if err != nil {
		t.Fatalf("Redo cue1 failed: %v", err)
	}
	c1, _ = cueRepo.FindByID(ctx, cue1.ID)
	if c1.FadeInTime != 5.0 {
		t.Errorf("Expected cue1 fadeInTime 5.0 after redo, got %f", c1.FadeInTime)
	}

	_, err = undoService.Redo(ctx, project.ID)
	if err != nil {
		t.Fatalf("Redo cue2 failed: %v", err)
	}
	c2, _ = cueRepo.FindByID(ctx, cue2.ID)
	if c2.FadeInTime != 5.0 {
		t.Errorf("Expected cue2 fadeInTime 5.0 after redo, got %f", c2.FadeInTime)
	}
}

// TestUndoRedoCloneLook_Integration tests undo/redo for cloning a look
func TestUndoRedoCloneLook_Integration(t *testing.T) {
	db, cleanup := setupUndoTestDB(t)
	defer cleanup()

	undoService, projectRepo, _, _, lookRepo, fixtureRepo, _, _ := createUndoTestService(db)

	ctx := context.Background()

	// Setup: Create project and fixture
	project := &models.Project{ID: cuid.New(), Name: "Clone Look Test"}
	if err := projectRepo.Create(ctx, project); err != nil {
		t.Fatalf("Failed to create project: %v", err)
	}

	fixtureDef := &models.FixtureDefinition{
		ID:           cuid.New(),
		Manufacturer: "Test",
		Model:        "Model1",
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

	// Create original look
	originalLook := &models.Look{
		ID:        cuid.New(),
		ProjectID: project.ID,
		Name:      "Original Look",
	}
	fixtureValues := []models.FixtureValue{
		{
			ID:        cuid.New(),
			LookID:    originalLook.ID,
			FixtureID: fixture.ID,
			Channels:  `[{"offset":0,"value":255}]`,
		},
	}
	if err := lookRepo.CreateWithFixtureValues(ctx, originalLook, fixtureValues); err != nil {
		t.Fatalf("Failed to create original look: %v", err)
	}

	// Clone the look
	clonedLook := &models.Look{
		ID:        cuid.New(),
		ProjectID: project.ID,
		Name:      "Cloned Look",
	}
	clonedValues := []models.FixtureValue{
		{
			ID:        cuid.New(),
			LookID:    clonedLook.ID,
			FixtureID: fixture.ID,
			Channels:  `[{"offset":0,"value":255}]`,
		},
	}
	if err := lookRepo.CreateWithFixtureValues(ctx, clonedLook, clonedValues); err != nil {
		t.Fatalf("Failed to create cloned look: %v", err)
	}

	// Record clone operation as CREATE
	newState, err := undoService.CaptureLookState(ctx, clonedLook.ID)
	if err != nil {
		t.Fatalf("Failed to capture cloned look state: %v", err)
	}
	if err := undoService.RecordOperation(ctx, project.ID, undo.OperationTypeCreate, undo.EntityTypeLook, clonedLook.ID,
		"Clone look to 'Cloned Look'", nil, newState, nil); err != nil {
		t.Fatalf("Failed to record clone operation: %v", err)
	}

	// Verify cloned look exists
	look, err := lookRepo.FindByID(ctx, clonedLook.ID)
	if err != nil || look == nil {
		t.Fatalf("Cloned look should exist")
	}

	// Undo should delete the cloned look
	result, err := undoService.Undo(ctx, project.ID)
	if err != nil {
		t.Fatalf("Undo failed: %v", err)
	}
	if !result.Success {
		t.Fatalf("Undo was not successful: %s", result.Message)
	}

	// Verify cloned look was deleted
	look, err = lookRepo.FindByID(ctx, clonedLook.ID)
	if err == nil && look != nil {
		t.Errorf("Expected cloned look to be deleted after undo")
	}

	// Original look should still exist
	look, err = lookRepo.FindByID(ctx, originalLook.ID)
	if err != nil || look == nil {
		t.Errorf("Original look should still exist after undo")
	}

	// Redo should recreate the cloned look
	result, err = undoService.Redo(ctx, project.ID)
	if err != nil {
		t.Fatalf("Redo failed: %v", err)
	}
	if !result.Success {
		t.Fatalf("Redo was not successful: %s", result.Message)
	}

	look, err = lookRepo.FindByID(ctx, clonedLook.ID)
	if err != nil || look == nil {
		t.Errorf("Cloned look should exist after redo")
	}
}

// TestUndoRedoAddRemoveFixturesFromLook_Integration tests undo/redo for adding/removing fixtures from a look
func TestUndoRedoAddRemoveFixturesFromLook_Integration(t *testing.T) {
	db, cleanup := setupUndoTestDB(t)
	defer cleanup()

	undoService, projectRepo, _, _, lookRepo, fixtureRepo, _, _ := createUndoTestService(db)

	ctx := context.Background()

	// Setup
	project := &models.Project{ID: cuid.New(), Name: "Add/Remove Fixtures Test"}
	if err := projectRepo.Create(ctx, project); err != nil {
		t.Fatalf("Failed to create project: %v", err)
	}

	fixtureDef := &models.FixtureDefinition{
		ID:           cuid.New(),
		Manufacturer: "Test",
		Model:        "Model1",
		Type:         "LED_PAR",
	}
	if err := db.Create(fixtureDef).Error; err != nil {
		t.Fatalf("Failed to create fixture definition: %v", err)
	}

	fixture1 := &models.FixtureInstance{
		ID:           cuid.New(),
		ProjectID:    project.ID,
		DefinitionID: fixtureDef.ID,
		Name:         "Fixture 1",
		Universe:     1,
		StartChannel: 1,
	}
	fixture2 := &models.FixtureInstance{
		ID:           cuid.New(),
		ProjectID:    project.ID,
		DefinitionID: fixtureDef.ID,
		Name:         "Fixture 2",
		Universe:     1,
		StartChannel: 10,
	}
	if err := fixtureRepo.Create(ctx, fixture1); err != nil {
		t.Fatalf("Failed to create fixture1: %v", err)
	}
	if err := fixtureRepo.Create(ctx, fixture2); err != nil {
		t.Fatalf("Failed to create fixture2: %v", err)
	}

	// Create look with one fixture
	look := &models.Look{
		ID:        cuid.New(),
		ProjectID: project.ID,
		Name:      "Test Look",
	}
	initialValues := []models.FixtureValue{
		{
			ID:        cuid.New(),
			LookID:    look.ID,
			FixtureID: fixture1.ID,
			Channels:  `[{"offset":0,"value":100}]`,
		},
	}
	if err := lookRepo.CreateWithFixtureValues(ctx, look, initialValues); err != nil {
		t.Fatalf("Failed to create look: %v", err)
	}

	// Capture state before adding fixture
	prevState, _ := undoService.CaptureLookState(ctx, look.ID)

	// Add second fixture to look
	newFixtureValue := &models.FixtureValue{
		ID:        cuid.New(),
		LookID:    look.ID,
		FixtureID: fixture2.ID,
		Channels:  `[{"offset":0,"value":200}]`,
	}
	if err := lookRepo.CreateFixtureValue(ctx, newFixtureValue); err != nil {
		t.Fatalf("Failed to add fixture to look: %v", err)
	}

	// Record add operation
	newState, _ := undoService.CaptureLookState(ctx, look.ID)
	if err := undoService.RecordOperation(ctx, project.ID, undo.OperationTypeUpdate, undo.EntityTypeLook, look.ID,
		"Add fixtures to look 'Test Look'", prevState, newState, nil); err != nil {
		t.Fatalf("Failed to record operation: %v", err)
	}

	// Verify two fixtures in look
	values, err := lookRepo.GetFixtureValues(ctx, look.ID)
	if err != nil {
		t.Fatalf("Failed to get fixture values: %v", err)
	}
	if len(values) != 2 {
		t.Errorf("Expected 2 fixture values, got %d", len(values))
	}

	// Undo should remove the added fixture
	result, err := undoService.Undo(ctx, project.ID)
	if err != nil {
		t.Fatalf("Undo failed: %v", err)
	}
	if !result.Success {
		t.Fatalf("Undo was not successful: %s", result.Message)
	}

	values, err = lookRepo.GetFixtureValues(ctx, look.ID)
	if err != nil {
		t.Fatalf("Failed to get fixture values after undo: %v", err)
	}
	if len(values) != 1 {
		t.Errorf("Expected 1 fixture value after undo, got %d", len(values))
	}

	// Redo should re-add the fixture
	result, err = undoService.Redo(ctx, project.ID)
	if err != nil {
		t.Fatalf("Redo failed: %v", err)
	}
	if !result.Success {
		t.Fatalf("Redo was not successful: %s", result.Message)
	}

	values, err = lookRepo.GetFixtureValues(ctx, look.ID)
	if err != nil {
		t.Fatalf("Failed to get fixture values after redo: %v", err)
	}
	if len(values) != 2 {
		t.Errorf("Expected 2 fixture values after redo, got %d", len(values))
	}
}

// TestUndoRedoUpdateLookPartial_Integration tests undo/redo for partial look updates
func TestUndoRedoUpdateLookPartial_Integration(t *testing.T) {
	db, cleanup := setupUndoTestDB(t)
	defer cleanup()

	undoService, projectRepo, _, _, lookRepo, fixtureRepo, _, _ := createUndoTestService(db)

	ctx := context.Background()

	// Setup
	project := &models.Project{ID: cuid.New(), Name: "Update Look Partial Test"}
	if err := projectRepo.Create(ctx, project); err != nil {
		t.Fatalf("Failed to create project: %v", err)
	}

	fixtureDef := &models.FixtureDefinition{
		ID:           cuid.New(),
		Manufacturer: "Test",
		Model:        "Model1",
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

	// Create look
	look := &models.Look{
		ID:        cuid.New(),
		ProjectID: project.ID,
		Name:      "Original Name",
	}
	fixtureValues := []models.FixtureValue{
		{
			ID:        cuid.New(),
			LookID:    look.ID,
			FixtureID: fixture.ID,
			Channels:  `[{"offset":0,"value":50}]`,
		},
	}
	if err := lookRepo.CreateWithFixtureValues(ctx, look, fixtureValues); err != nil {
		t.Fatalf("Failed to create look: %v", err)
	}

	// Capture previous state
	prevState, _ := undoService.CaptureLookState(ctx, look.ID)

	// Update look name and fixture values
	look.Name = "Updated Name"
	if err := lookRepo.Update(ctx, look); err != nil {
		t.Fatalf("Failed to update look: %v", err)
	}
	if err := lookRepo.DeleteFixtureValues(ctx, look.ID); err != nil {
		t.Fatalf("Failed to delete fixture values: %v", err)
	}
	newValues := []models.FixtureValue{
		{
			ID:        cuid.New(),
			LookID:    look.ID,
			FixtureID: fixture.ID,
			Channels:  `[{"offset":0,"value":200}]`,
		},
	}
	if err := lookRepo.CreateFixtureValues(ctx, newValues); err != nil {
		t.Fatalf("Failed to create new fixture values: %v", err)
	}

	// Record update operation
	newState, _ := undoService.CaptureLookState(ctx, look.ID)
	if err := undoService.RecordOperation(ctx, project.ID, undo.OperationTypeUpdate, undo.EntityTypeLook, look.ID,
		"Update look 'Original Name'", prevState, newState, nil); err != nil {
		t.Fatalf("Failed to record operation: %v", err)
	}

	// Verify current state
	currentLook, _ := lookRepo.FindByID(ctx, look.ID)
	if currentLook.Name != "Updated Name" {
		t.Errorf("Expected 'Updated Name', got '%s'", currentLook.Name)
	}

	// Undo
	result, err := undoService.Undo(ctx, project.ID)
	if err != nil || !result.Success {
		t.Fatalf("Undo failed")
	}

	// Verify original state restored
	currentLook, _ = lookRepo.FindByID(ctx, look.ID)
	if currentLook.Name != "Original Name" {
		t.Errorf("Expected 'Original Name' after undo, got '%s'", currentLook.Name)
	}

	values, _ := lookRepo.GetFixtureValues(ctx, look.ID)
	if len(values) != 1 || values[0].Channels != `[{"offset":0,"value":50}]` {
		t.Errorf("Expected original fixture values after undo")
	}

	// Redo
	result, err = undoService.Redo(ctx, project.ID)
	if err != nil || !result.Success {
		t.Fatalf("Redo failed")
	}

	currentLook, _ = lookRepo.FindByID(ctx, look.ID)
	if currentLook.Name != "Updated Name" {
		t.Errorf("Expected 'Updated Name' after redo, got '%s'", currentLook.Name)
	}
}

// TestUndoRedoReorderLookFixtures_Integration tests undo/redo for reordering fixtures in a look
func TestUndoRedoReorderLookFixtures_Integration(t *testing.T) {
	db, cleanup := setupUndoTestDB(t)
	defer cleanup()

	undoService, projectRepo, _, _, lookRepo, fixtureRepo, _, _ := createUndoTestService(db)

	ctx := context.Background()

	// Setup
	project := &models.Project{ID: cuid.New(), Name: "Reorder Look Fixtures Test"}
	if err := projectRepo.Create(ctx, project); err != nil {
		t.Fatalf("Failed to create project: %v", err)
	}

	fixtureDef := &models.FixtureDefinition{
		ID:           cuid.New(),
		Manufacturer: "Test",
		Model:        "Model1",
		Type:         "LED_PAR",
	}
	if err := db.Create(fixtureDef).Error; err != nil {
		t.Fatalf("Failed to create fixture definition: %v", err)
	}

	fixture1 := &models.FixtureInstance{
		ID:           cuid.New(),
		ProjectID:    project.ID,
		DefinitionID: fixtureDef.ID,
		Name:         "Fixture 1",
		Universe:     1,
		StartChannel: 1,
	}
	fixture2 := &models.FixtureInstance{
		ID:           cuid.New(),
		ProjectID:    project.ID,
		DefinitionID: fixtureDef.ID,
		Name:         "Fixture 2",
		Universe:     1,
		StartChannel: 10,
	}
	if err := fixtureRepo.Create(ctx, fixture1); err != nil {
		t.Fatalf("Failed to create fixture1: %v", err)
	}
	if err := fixtureRepo.Create(ctx, fixture2); err != nil {
		t.Fatalf("Failed to create fixture2: %v", err)
	}

	// Create look with both fixtures
	look := &models.Look{
		ID:        cuid.New(),
		ProjectID: project.ID,
		Name:      "Test Look",
	}
	order1, order2 := 0, 1
	fixtureValues := []models.FixtureValue{
		{
			ID:        cuid.New(),
			LookID:    look.ID,
			FixtureID: fixture1.ID,
			Channels:  `[{"offset":0,"value":100}]`,
			LookOrder: &order1,
		},
		{
			ID:        cuid.New(),
			LookID:    look.ID,
			FixtureID: fixture2.ID,
			Channels:  `[{"offset":0,"value":200}]`,
			LookOrder: &order2,
		},
	}
	if err := lookRepo.CreateWithFixtureValues(ctx, look, fixtureValues); err != nil {
		t.Fatalf("Failed to create look: %v", err)
	}

	// Capture previous state
	prevState, _ := undoService.CaptureLookState(ctx, look.ID)

	// Reorder fixtures (swap orders)
	values, _ := lookRepo.GetFixtureValues(ctx, look.ID)
	for i := range values {
		switch values[i].FixtureID {
		case fixture1.ID:
			newOrder := 1
			values[i].LookOrder = &newOrder
			if err := lookRepo.UpdateFixtureValue(ctx, &values[i]); err != nil {
				t.Fatalf("Failed to update fixture1 value: %v", err)
			}
		case fixture2.ID:
			newOrder := 0
			values[i].LookOrder = &newOrder
			if err := lookRepo.UpdateFixtureValue(ctx, &values[i]); err != nil {
				t.Fatalf("Failed to update fixture2 value: %v", err)
			}
		}
	}

	// Record reorder operation
	newState, _ := undoService.CaptureLookState(ctx, look.ID)
	if err := undoService.RecordOperation(ctx, project.ID, undo.OperationTypeUpdate, undo.EntityTypeLook, look.ID,
		"Reorder fixtures in look 'Test Look'", prevState, newState, nil); err != nil {
		t.Fatalf("Failed to record operation: %v", err)
	}

	// Verify new order
	values, _ = lookRepo.GetFixtureValues(ctx, look.ID)
	for _, v := range values {
		if v.FixtureID == fixture1.ID && (v.LookOrder == nil || *v.LookOrder != 1) {
			t.Errorf("Expected fixture1 order to be 1")
		}
		if v.FixtureID == fixture2.ID && (v.LookOrder == nil || *v.LookOrder != 0) {
			t.Errorf("Expected fixture2 order to be 0")
		}
	}

	// Undo
	result, err := undoService.Undo(ctx, project.ID)
	if err != nil || !result.Success {
		t.Fatalf("Undo failed")
	}

	// Verify original order restored
	values, _ = lookRepo.GetFixtureValues(ctx, look.ID)
	for _, v := range values {
		if v.FixtureID == fixture1.ID && (v.LookOrder == nil || *v.LookOrder != 0) {
			t.Errorf("Expected fixture1 order to be 0 after undo")
		}
		if v.FixtureID == fixture2.ID && (v.LookOrder == nil || *v.LookOrder != 1) {
			t.Errorf("Expected fixture2 order to be 1 after undo")
		}
	}
}

// TestUndoRedoUpdateFixturePositions_Integration tests undo/redo for updating fixture positions
func TestUndoRedoUpdateFixturePositions_Integration(t *testing.T) {
	db, cleanup := setupUndoTestDB(t)
	defer cleanup()

	undoService, projectRepo, _, _, _, fixtureRepo, _, _ := createUndoTestService(db)

	ctx := context.Background()

	// Setup
	project := &models.Project{ID: cuid.New(), Name: "Fixture Positions Test"}
	if err := projectRepo.Create(ctx, project); err != nil {
		t.Fatalf("Failed to create project: %v", err)
	}

	fixtureDef := &models.FixtureDefinition{
		ID:           cuid.New(),
		Manufacturer: "Test",
		Model:        "Model1",
		Type:         "LED_PAR",
	}
	if err := db.Create(fixtureDef).Error; err != nil {
		t.Fatalf("Failed to create fixture definition: %v", err)
	}

	originalX, originalY := 0.1, 0.2
	fixture := &models.FixtureInstance{
		ID:           cuid.New(),
		ProjectID:    project.ID,
		DefinitionID: fixtureDef.ID,
		Name:         "Test Fixture",
		Universe:     1,
		StartChannel: 1,
		LayoutX:      &originalX,
		LayoutY:      &originalY,
	}
	if err := fixtureRepo.Create(ctx, fixture); err != nil {
		t.Fatalf("Failed to create fixture: %v", err)
	}

	// Capture previous state
	prevState, _ := undoService.CaptureFixtureState(ctx, fixture.ID)

	// Update position
	newX, newY := 0.5, 0.6
	fixture.LayoutX = &newX
	fixture.LayoutY = &newY
	if err := fixtureRepo.Update(ctx, fixture); err != nil {
		t.Fatalf("Failed to update fixture: %v", err)
	}

	// Record position update
	newState, _ := undoService.CaptureFixtureState(ctx, fixture.ID)
	if err := undoService.RecordOperation(ctx, project.ID, undo.OperationTypeUpdate, undo.EntityTypeFixtureInstance, fixture.ID,
		"Update position for fixture 'Test Fixture'", prevState, newState, nil); err != nil {
		t.Fatalf("Failed to record operation: %v", err)
	}

	// Verify new position
	currentFixture, _ := fixtureRepo.FindByID(ctx, fixture.ID)
	if *currentFixture.LayoutX != 0.5 || *currentFixture.LayoutY != 0.6 {
		t.Errorf("Expected position (0.5, 0.6), got (%f, %f)", *currentFixture.LayoutX, *currentFixture.LayoutY)
	}

	// Undo
	result, err := undoService.Undo(ctx, project.ID)
	if err != nil || !result.Success {
		t.Fatalf("Undo failed")
	}

	// Verify original position restored
	currentFixture, _ = fixtureRepo.FindByID(ctx, fixture.ID)
	if *currentFixture.LayoutX != 0.1 || *currentFixture.LayoutY != 0.2 {
		t.Errorf("Expected position (0.1, 0.2) after undo, got (%f, %f)", *currentFixture.LayoutX, *currentFixture.LayoutY)
	}

	// Redo
	result, err = undoService.Redo(ctx, project.ID)
	if err != nil || !result.Success {
		t.Fatalf("Redo failed")
	}

	currentFixture, _ = fixtureRepo.FindByID(ctx, fixture.ID)
	if *currentFixture.LayoutX != 0.5 || *currentFixture.LayoutY != 0.6 {
		t.Errorf("Expected position (0.5, 0.6) after redo, got (%f, %f)", *currentFixture.LayoutX, *currentFixture.LayoutY)
	}
}

// TestUndoRedoChannelFadeBehavior_Integration tests undo/redo for updating channel fade behavior
func TestUndoRedoChannelFadeBehavior_Integration(t *testing.T) {
	db, cleanup := setupUndoTestDB(t)
	defer cleanup()

	undoService, projectRepo, _, _, _, fixtureRepo, _, _ := createUndoTestService(db)

	ctx := context.Background()

	// Setup
	project := &models.Project{ID: cuid.New(), Name: "Channel Fade Test"}
	if err := projectRepo.Create(ctx, project); err != nil {
		t.Fatalf("Failed to create project: %v", err)
	}

	fixtureDef := &models.FixtureDefinition{
		ID:           cuid.New(),
		Manufacturer: "Test",
		Model:        "Model1",
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

	// Create instance channel
	channel := &models.InstanceChannel{
		ID:           cuid.New(),
		FixtureID:    fixture.ID,
		Offset:       0,
		Name:         "Dimmer",
		Type:         "INTENSITY",
		FadeBehavior: "FADE",
	}
	if err := db.Create(channel).Error; err != nil {
		t.Fatalf("Failed to create channel: %v", err)
	}

	// Capture previous state
	prevState, _ := undoService.CaptureFixtureState(ctx, fixture.ID)

	// Update fade behavior
	channel.FadeBehavior = "SNAP"
	if err := db.Save(channel).Error; err != nil {
		t.Fatalf("Failed to update channel: %v", err)
	}

	// Record update
	newState, _ := undoService.CaptureFixtureState(ctx, fixture.ID)
	if err := undoService.RecordOperation(ctx, project.ID, undo.OperationTypeUpdate, undo.EntityTypeFixtureInstance, fixture.ID,
		"Update fade behavior for channel 'Dimmer'", prevState, newState, nil); err != nil {
		t.Fatalf("Failed to record operation: %v", err)
	}

	// Verify new fade behavior
	var updatedChannel models.InstanceChannel
	if err := db.First(&updatedChannel, "id = ?", channel.ID).Error; err != nil {
		t.Fatalf("Failed to find channel: %v", err)
	}
	if updatedChannel.FadeBehavior != "SNAP" {
		t.Errorf("Expected SNAP, got %s", updatedChannel.FadeBehavior)
	}

	// Undo
	result, err := undoService.Undo(ctx, project.ID)
	if err != nil || !result.Success {
		t.Fatalf("Undo failed: %v", err)
	}

	// Verify original fade behavior restored
	if err := db.First(&updatedChannel, "id = ?", channel.ID).Error; err != nil {
		t.Fatalf("Failed to find channel after undo: %v", err)
	}
	if updatedChannel.FadeBehavior != "FADE" {
		t.Errorf("Expected FADE after undo, got %s", updatedChannel.FadeBehavior)
	}

	// Redo
	result, err = undoService.Redo(ctx, project.ID)
	if err != nil || !result.Success {
		t.Fatalf("Redo failed: %v", err)
	}

	if err := db.First(&updatedChannel, "id = ?", channel.ID).Error; err != nil {
		t.Fatalf("Failed to find channel after redo: %v", err)
	}
	if updatedChannel.FadeBehavior != "SNAP" {
		t.Errorf("Expected SNAP after redo, got %s", updatedChannel.FadeBehavior)
	}
}

// TestUndoRedoReorderProjectFixtures_Integration tests undo/redo for reordering fixtures in a project
func TestUndoRedoReorderProjectFixtures_Integration(t *testing.T) {
	db, cleanup := setupUndoTestDB(t)
	defer cleanup()

	undoService, projectRepo, _, _, _, fixtureRepo, _, _ := createUndoTestService(db)

	ctx := context.Background()

	// Setup
	project := &models.Project{ID: cuid.New(), Name: "Reorder Project Fixtures Test"}
	if err := projectRepo.Create(ctx, project); err != nil {
		t.Fatalf("Failed to create project: %v", err)
	}

	fixtureDef := &models.FixtureDefinition{
		ID:           cuid.New(),
		Manufacturer: "Test",
		Model:        "Model1",
		Type:         "LED_PAR",
	}
	if err := db.Create(fixtureDef).Error; err != nil {
		t.Fatalf("Failed to create fixture definition: %v", err)
	}

	order1, order2 := 0, 1
	fixture1 := &models.FixtureInstance{
		ID:           cuid.New(),
		ProjectID:    project.ID,
		DefinitionID: fixtureDef.ID,
		Name:         "Fixture 1",
		Universe:     1,
		StartChannel: 1,
		ProjectOrder: &order1,
	}
	fixture2 := &models.FixtureInstance{
		ID:           cuid.New(),
		ProjectID:    project.ID,
		DefinitionID: fixtureDef.ID,
		Name:         "Fixture 2",
		Universe:     1,
		StartChannel: 10,
		ProjectOrder: &order2,
	}
	if err := fixtureRepo.Create(ctx, fixture1); err != nil {
		t.Fatalf("Failed to create fixture1: %v", err)
	}
	if err := fixtureRepo.Create(ctx, fixture2); err != nil {
		t.Fatalf("Failed to create fixture2: %v", err)
	}

	// Capture previous state for fixture1
	prevState1, _ := undoService.CaptureFixtureState(ctx, fixture1.ID)

	// Reorder fixture1 (change order from 0 to 1)
	newOrder := 1
	fixture1.ProjectOrder = &newOrder
	if err := fixtureRepo.Update(ctx, fixture1); err != nil {
		t.Fatalf("Failed to update fixture1: %v", err)
	}

	// Record reorder operation
	newState1, _ := undoService.CaptureFixtureState(ctx, fixture1.ID)
	if err := undoService.RecordOperation(ctx, project.ID, undo.OperationTypeUpdate, undo.EntityTypeFixtureInstance, fixture1.ID,
		"Reorder fixture 'Fixture 1'", prevState1, newState1, nil); err != nil {
		t.Fatalf("Failed to record operation: %v", err)
	}

	// Verify new order
	currentFixture1, _ := fixtureRepo.FindByID(ctx, fixture1.ID)
	if *currentFixture1.ProjectOrder != 1 {
		t.Errorf("Expected fixture1 order 1, got %d", *currentFixture1.ProjectOrder)
	}

	// Undo
	result, err := undoService.Undo(ctx, project.ID)
	if err != nil || !result.Success {
		t.Fatalf("Undo failed")
	}

	// Verify original order restored
	currentFixture1, _ = fixtureRepo.FindByID(ctx, fixture1.ID)
	if *currentFixture1.ProjectOrder != 0 {
		t.Errorf("Expected fixture1 order 0 after undo, got %d", *currentFixture1.ProjectOrder)
	}

	// Redo
	result, err = undoService.Redo(ctx, project.ID)
	if err != nil || !result.Success {
		t.Fatalf("Redo failed")
	}

	currentFixture1, _ = fixtureRepo.FindByID(ctx, fixture1.ID)
	if *currentFixture1.ProjectOrder != 1 {
		t.Errorf("Expected fixture1 order 1 after redo, got %d", *currentFixture1.ProjectOrder)
	}
}

// TestProjectSoftDelete_Integration tests soft delete functionality for projects.
func TestProjectSoftDelete_Integration(t *testing.T) {
	db, cleanup := setupUndoTestDB(t)
	defer cleanup()
	ctx := context.Background()

	projectRepo := repositories.NewProjectRepository(db)

	// Create a project
	project := &models.Project{
		ID:   cuid.New(),
		Name: "Test Project",
	}
	if err := projectRepo.Create(ctx, project); err != nil {
		t.Fatalf("Failed to create project: %v", err)
	}

	// Verify project is in FindAll
	projects, err := projectRepo.FindAll(ctx)
	if err != nil {
		t.Fatalf("FindAll failed: %v", err)
	}
	if len(projects) != 1 {
		t.Errorf("Expected 1 project, got %d", len(projects))
	}

	// Soft delete the project
	if err := projectRepo.Delete(ctx, project.ID); err != nil {
		t.Fatalf("Delete failed: %v", err)
	}

	// Verify project is NOT in FindAll
	projects, err = projectRepo.FindAll(ctx)
	if err != nil {
		t.Fatalf("FindAll failed: %v", err)
	}
	if len(projects) != 0 {
		t.Errorf("Expected 0 projects after soft delete, got %d", len(projects))
	}

	// Verify project IS in FindAllDeleted
	deletedProjects, err := projectRepo.FindAllDeleted(ctx)
	if err != nil {
		t.Fatalf("FindAllDeleted failed: %v", err)
	}
	if len(deletedProjects) != 1 {
		t.Errorf("Expected 1 deleted project, got %d", len(deletedProjects))
	}
	if deletedProjects[0].DeletedAt == nil {
		t.Error("Expected DeletedAt to be set")
	}

	// Verify FindByID returns nil for deleted project
	found, err := projectRepo.FindByID(ctx, project.ID)
	if err != nil {
		t.Fatalf("FindByID failed: %v", err)
	}
	if found != nil {
		t.Error("Expected FindByID to return nil for deleted project")
	}

	// Verify FindByIDIncludingDeleted returns the project
	found, err = projectRepo.FindByIDIncludingDeleted(ctx, project.ID)
	if err != nil {
		t.Fatalf("FindByIDIncludingDeleted failed: %v", err)
	}
	if found == nil {
		t.Error("Expected FindByIDIncludingDeleted to return the deleted project")
	}
}

// TestProjectRestore_Integration tests restoring a soft-deleted project.
func TestProjectRestore_Integration(t *testing.T) {
	db, cleanup := setupUndoTestDB(t)
	defer cleanup()
	ctx := context.Background()

	projectRepo := repositories.NewProjectRepository(db)

	// Create and soft delete a project
	project := &models.Project{
		ID:   cuid.New(),
		Name: "Test Project",
	}
	if err := projectRepo.Create(ctx, project); err != nil {
		t.Fatalf("Failed to create project: %v", err)
	}
	if err := projectRepo.Delete(ctx, project.ID); err != nil {
		t.Fatalf("Delete failed: %v", err)
	}

	// Verify project is deleted
	found, err := projectRepo.FindByID(ctx, project.ID)
	if err != nil {
		t.Fatalf("FindByID failed: %v", err)
	}
	if found != nil {
		t.Error("Expected project to be soft-deleted")
	}

	// Restore the project
	if err := projectRepo.Restore(ctx, project.ID); err != nil {
		t.Fatalf("Restore failed: %v", err)
	}

	// Verify project is back in FindAll
	projects, err := projectRepo.FindAll(ctx)
	if err != nil {
		t.Fatalf("FindAll failed: %v", err)
	}
	if len(projects) != 1 {
		t.Errorf("Expected 1 project after restore, got %d", len(projects))
	}

	// Verify FindByID returns the project
	found, err = projectRepo.FindByID(ctx, project.ID)
	if err != nil {
		t.Fatalf("FindByID failed: %v", err)
	}
	if found == nil {
		t.Fatal("Expected FindByID to return the restored project")
	}
	if found.DeletedAt != nil {
		t.Error("Expected DeletedAt to be nil after restore")
	}

	// Verify project is NOT in FindAllDeleted
	deletedProjects, err := projectRepo.FindAllDeleted(ctx)
	if err != nil {
		t.Fatalf("FindAllDeleted failed: %v", err)
	}
	if len(deletedProjects) != 0 {
		t.Errorf("Expected 0 deleted projects after restore, got %d", len(deletedProjects))
	}
}

// TestProjectPermanentDelete_Integration tests permanently deleting a soft-deleted project.
func TestProjectPermanentDelete_Integration(t *testing.T) {
	db, cleanup := setupUndoTestDB(t)
	defer cleanup()
	ctx := context.Background()

	projectRepo := repositories.NewProjectRepository(db)

	// Create and soft delete a project
	project := &models.Project{
		ID:   cuid.New(),
		Name: "Test Project",
	}
	if err := projectRepo.Create(ctx, project); err != nil {
		t.Fatalf("Failed to create project: %v", err)
	}
	if err := projectRepo.Delete(ctx, project.ID); err != nil {
		t.Fatalf("Delete failed: %v", err)
	}

	// Permanently delete the project
	if err := projectRepo.PermanentDelete(ctx, project.ID); err != nil {
		t.Fatalf("PermanentDelete failed: %v", err)
	}

	// Verify project is NOT in FindAllDeleted
	deletedProjects, err := projectRepo.FindAllDeleted(ctx)
	if err != nil {
		t.Fatalf("FindAllDeleted failed: %v", err)
	}
	if len(deletedProjects) != 0 {
		t.Errorf("Expected 0 deleted projects after permanent delete, got %d", len(deletedProjects))
	}

	// Verify project is NOT in FindByIDIncludingDeleted
	found, err := projectRepo.FindByIDIncludingDeleted(ctx, project.ID)
	if err != nil {
		t.Fatalf("FindByIDIncludingDeleted failed: %v", err)
	}
	if found != nil {
		t.Error("Expected project to be permanently deleted")
	}
}

// TestPubSubFixtureDataChanged_Integration tests that fixture data changes are published via pubsub
func TestPubSubFixtureDataChanged_Integration(t *testing.T) {
	db, cleanup := setupUndoTestDB(t)
	defer cleanup()

	ctx := context.Background()

	// Create repositories and services
	projectRepo := repositories.NewProjectRepository(db)
	fixtureRepo := repositories.NewFixtureRepository(db)
	opRepo := repositories.NewOperationRepository(db)
	lookRepo := repositories.NewLookRepository(db)
	cueRepo := repositories.NewCueRepository(db)
	cueListRepo := repositories.NewCueListRepository(db)
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

	// Setup: Create project and fixture
	project := &models.Project{ID: cuid.New(), Name: "PubSub Test Project"}
	if err := projectRepo.Create(ctx, project); err != nil {
		t.Fatalf("Failed to create project: %v", err)
	}

	fixtureDef := &models.FixtureDefinition{
		ID:           cuid.New(),
		Manufacturer: "Test",
		Model:        "PubSubModel",
		Type:         "LED_PAR",
	}
	if err := db.Create(fixtureDef).Error; err != nil {
		t.Fatalf("Failed to create fixture definition: %v", err)
	}

	originalX, originalY := 100.0, 200.0
	fixture1 := &models.FixtureInstance{
		ID:           cuid.New(),
		ProjectID:    project.ID,
		DefinitionID: fixtureDef.ID,
		Name:         "Fixture 1",
		Universe:     1,
		StartChannel: 1,
		LayoutX:      &originalX,
		LayoutY:      &originalY,
	}
	if err := fixtureRepo.Create(ctx, fixture1); err != nil {
		t.Fatalf("Failed to create fixture: %v", err)
	}

	// Note: Pubsub publishing happens at the resolver level, not the service level.
	// This test verifies the service correctly captures and restores fixture positions
	// and returns the operation with RelatedIDs. The resolver uses RelatedIDs to publish
	// pubsub events. Resolver-level pubsub tests are in graphql_test.go.
	// We don't need a subscriber here since the service doesn't publish directly.
	_ = ps // ps is available but unused at service level

	// Capture previous state for bulk position update
	fixtureIDs := []string{fixture1.ID}
	prevState, err := undoService.CaptureFixturePositions(ctx, fixtureIDs)
	if err != nil {
		t.Fatalf("Failed to capture previous state: %v", err)
	}

	// Update fixture position
	newX, newY := 500.0, 600.0
	fixture1.LayoutX = &newX
	fixture1.LayoutY = &newY
	if err := fixtureRepo.Update(ctx, fixture1); err != nil {
		t.Fatalf("Failed to update fixture: %v", err)
	}

	// Record bulk position update operation
	newState, err := undoService.CaptureFixturePositions(ctx, fixtureIDs)
	if err != nil {
		t.Fatalf("Failed to capture new state: %v", err)
	}
	if err := undoService.RecordOperation(ctx, project.ID, undo.OperationTypeBulk, undo.EntityTypeBulkFixturePosition, "",
		"Update positions for 1 fixture", prevState, newState, fixtureIDs); err != nil {
		t.Fatalf("Failed to record operation: %v", err)
	}

	// Undo the operation - this should publish fixture data changed
	result, err := undoService.Undo(ctx, project.ID)
	if err != nil || !result.Success {
		t.Fatalf("Undo failed: %v", err)
	}

	// Verify the undo restored the original position
	currentFixture, _ := fixtureRepo.FindByID(ctx, fixture1.ID)
	if *currentFixture.LayoutX != originalX || *currentFixture.LayoutY != originalY {
		t.Errorf("Expected position (%f, %f) after undo, got (%f, %f)",
			originalX, originalY, *currentFixture.LayoutX, *currentFixture.LayoutY)
	}

	// Verify operation was recorded correctly
	if result.Operation == nil {
		t.Error("Expected operation to be returned")
	} else if result.Operation.EntityType != "BulkFixturePosition" {
		t.Errorf("Expected EntityType BulkFixturePosition, got %s", result.Operation.EntityType)
	}

	// Check that related IDs were stored
	if result.Operation.RelatedIDs == nil {
		t.Error("Expected RelatedIDs to be set for bulk operation")
	}
}

// TestPubSubBulkFixturePositionRestore_Integration tests that bulk fixture position undo/redo preserves all fixture IDs
func TestPubSubBulkFixturePositionRestore_Integration(t *testing.T) {
	db, cleanup := setupUndoTestDB(t)
	defer cleanup()

	ctx := context.Background()

	undoService, projectRepo, _, _, _, fixtureRepo, _, _ := createUndoTestService(db)

	// Setup: Create project and multiple fixtures
	project := &models.Project{ID: cuid.New(), Name: "Bulk Position Test"}
	if err := projectRepo.Create(ctx, project); err != nil {
		t.Fatalf("Failed to create project: %v", err)
	}

	fixtureDef := &models.FixtureDefinition{
		ID:           cuid.New(),
		Manufacturer: "Test",
		Model:        "BulkModel",
		Type:         "LED_PAR",
	}
	if err := db.Create(fixtureDef).Error; err != nil {
		t.Fatalf("Failed to create fixture definition: %v", err)
	}

	// Create 3 fixtures with initial positions
	fixtures := make([]*models.FixtureInstance, 3)
	originalPositions := []struct{ x, y float64 }{
		{100, 100},
		{200, 200},
		{300, 300},
	}

	for i := range fixtures {
		x, y := originalPositions[i].x, originalPositions[i].y
		fixtures[i] = &models.FixtureInstance{
			ID:           cuid.New(),
			ProjectID:    project.ID,
			DefinitionID: fixtureDef.ID,
			Name:         "Fixture " + string(rune('A'+i)),
			Universe:     1,
			StartChannel: 1 + i*10,
			LayoutX:      &x,
			LayoutY:      &y,
		}
		if err := fixtureRepo.Create(ctx, fixtures[i]); err != nil {
			t.Fatalf("Failed to create fixture %d: %v", i, err)
		}
	}

	// Capture previous positions
	fixtureIDs := []string{fixtures[0].ID, fixtures[1].ID, fixtures[2].ID}
	prevState, _ := undoService.CaptureFixturePositions(ctx, fixtureIDs)

	// Update all positions
	newPositions := []struct{ x, y float64 }{
		{500, 500},
		{600, 600},
		{700, 700},
	}
	for i, f := range fixtures {
		x, y := newPositions[i].x, newPositions[i].y
		f.LayoutX = &x
		f.LayoutY = &y
		if err := fixtureRepo.Update(ctx, f); err != nil {
			t.Fatalf("Failed to update fixture %d: %v", i, err)
		}
	}

	// Record bulk operation
	newState, _ := undoService.CaptureFixturePositions(ctx, fixtureIDs)
	if err := undoService.RecordOperation(ctx, project.ID, undo.OperationTypeBulk, undo.EntityTypeBulkFixturePosition, "",
		"Update positions for 3 fixtures", prevState, newState, fixtureIDs); err != nil {
		t.Fatalf("Failed to record operation: %v", err)
	}

	// Verify new positions
	for i, f := range fixtures {
		current, _ := fixtureRepo.FindByID(ctx, f.ID)
		if *current.LayoutX != newPositions[i].x || *current.LayoutY != newPositions[i].y {
			t.Errorf("Fixture %d: expected (%f, %f), got (%f, %f)",
				i, newPositions[i].x, newPositions[i].y, *current.LayoutX, *current.LayoutY)
		}
	}

	// Undo - all fixtures should restore to original positions
	result, err := undoService.Undo(ctx, project.ID)
	if err != nil || !result.Success {
		t.Fatalf("Undo failed: %v", err)
	}

	// Verify all original positions restored
	for i, f := range fixtures {
		current, _ := fixtureRepo.FindByID(ctx, f.ID)
		if *current.LayoutX != originalPositions[i].x || *current.LayoutY != originalPositions[i].y {
			t.Errorf("After undo, Fixture %d: expected (%f, %f), got (%f, %f)",
				i, originalPositions[i].x, originalPositions[i].y, *current.LayoutX, *current.LayoutY)
		}
	}

	// Redo - all fixtures should go back to new positions
	result, err = undoService.Redo(ctx, project.ID)
	if err != nil || !result.Success {
		t.Fatalf("Redo failed: %v", err)
	}

	// Verify new positions restored after redo
	for i, f := range fixtures {
		current, _ := fixtureRepo.FindByID(ctx, f.ID)
		if *current.LayoutX != newPositions[i].x || *current.LayoutY != newPositions[i].y {
			t.Errorf("After redo, Fixture %d: expected (%f, %f), got (%f, %f)",
				i, newPositions[i].x, newPositions[i].y, *current.LayoutX, *current.LayoutY)
		}
	}
}
