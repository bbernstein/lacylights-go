package undo

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"

	"github.com/bbernstein/lacylights-go/internal/database/models"
	"github.com/bbernstein/lacylights-go/internal/database/repositories"
	"github.com/bbernstein/lacylights-go/internal/services/pubsub"
	"github.com/glebarez/sqlite"
	"github.com/lucsky/cuid"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// intPtr returns a pointer to the given int value.
func intPtr(i int) *int {
	return &i
}

// setupTestDB creates an in-memory SQLite database for testing.
func setupTestDB(t *testing.T) (*gorm.DB, func()) {
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
		&models.Effect{},
		&models.EffectFixture{},
		&models.EffectChannel{},
		&models.CueEffect{},
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

// createTestService creates an undo service with all dependencies.
func createTestService(t *testing.T, db *gorm.DB) *Service {
	t.Helper()

	opRepo := repositories.NewOperationRepository(db)
	lookRepo := repositories.NewLookRepository(db)
	fixtureRepo := repositories.NewFixtureRepository(db)
	cueRepo := repositories.NewCueRepository(db)
	cueListRepo := repositories.NewCueListRepository(db)
	lookBoardRepo := repositories.NewLookBoardRepository(db)
	effectRepo := repositories.NewEffectRepository(db)
	projectRepo := repositories.NewProjectRepository(db)
	ps := pubsub.New()

	return NewService(
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
}

// createTestProject creates a project for testing.
func createTestProject(t *testing.T, db *gorm.DB) *models.Project {
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

// createTestLook creates a look for testing.
func createTestLook(t *testing.T, db *gorm.DB, projectID string) *models.Look {
	t.Helper()

	look := &models.Look{
		ID:        cuid.New(),
		Name:      "Test Look " + cuid.Slug(),
		ProjectID: projectID,
	}
	if err := db.Create(look).Error; err != nil {
		t.Fatalf("Failed to create look: %v", err)
	}
	return look
}

// createTestFixtureInstance creates a fixture instance for testing.
func createTestFixtureInstance(t *testing.T, db *gorm.DB, projectID string) *models.FixtureInstance {
	t.Helper()

	// Create fixture definition first
	def := &models.FixtureDefinition{
		ID:           cuid.New(),
		Manufacturer: "Test",
		Model:        "TestModel",
		Type:         "LED_PAR",
	}
	if err := db.Create(def).Error; err != nil {
		t.Fatalf("Failed to create fixture definition: %v", err)
	}

	fixture := &models.FixtureInstance{
		ID:           cuid.New(),
		Name:         "Test Fixture " + cuid.Slug(),
		ProjectID:    projectID,
		DefinitionID: def.ID,
		Universe:     1,
		StartChannel: 1,
	}
	if err := db.Create(fixture).Error; err != nil {
		t.Fatalf("Failed to create fixture instance: %v", err)
	}
	return fixture
}

// createTestCueList creates a cue list for testing.
func createTestCueList(t *testing.T, db *gorm.DB, projectID string) *models.CueList {
	t.Helper()

	cueList := &models.CueList{
		ID:        cuid.New(),
		Name:      "Test Cue List " + cuid.Slug(),
		ProjectID: projectID,
	}
	if err := db.Create(cueList).Error; err != nil {
		t.Fatalf("Failed to create cue list: %v", err)
	}
	return cueList
}

// createTestCue creates a cue for testing.
func createTestCue(t *testing.T, db *gorm.DB, cueListID, lookID string) *models.Cue {
	t.Helper()

	cue := &models.Cue{
		ID:         cuid.New(),
		Name:       "Test Cue " + cuid.Slug(),
		CueListID:  cueListID,
		LookID:     lookID,
		CueNumber:  1.0,
		FadeInTime: 3.0,
	}
	if err := db.Create(cue).Error; err != nil {
		t.Fatalf("Failed to create cue: %v", err)
	}
	return cue
}

func TestNewService(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	service := createTestService(t, db)
	if service == nil {
		t.Fatal("Expected non-nil service")
	}
}

func TestService_SetPlaybackController(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	service := createTestService(t, db)

	// Initially nil
	if service.playbackController != nil {
		t.Error("Expected nil playbackController initially")
	}

	// Create a mock controller
	mockController := &mockPlaybackController{}
	service.SetPlaybackController(mockController)

	if service.playbackController != mockController {
		t.Error("Expected playbackController to be set")
	}
}

// mockPlaybackController implements PlaybackController for testing.
type mockPlaybackController struct {
	goToCueCalled bool
	stopCalled    bool
	lastCueListID string
	lastCueNumber float64
}

func (m *mockPlaybackController) GoToCueNumber(ctx context.Context, cueListID string, cueNumber float64, fadeInTimeOverride *float64) error {
	m.goToCueCalled = true
	m.lastCueListID = cueListID
	m.lastCueNumber = cueNumber
	return nil
}

func (m *mockPlaybackController) StopCueList(cueListID string) {
	m.stopCalled = true
	m.lastCueListID = cueListID
}

func TestService_RecordOperation(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	service := createTestService(t, db)
	ctx := context.Background()
	project := createTestProject(t, db)

	// Record a look creation
	snapshot := LookSnapshot{
		Look: &models.Look{
			ID:        cuid.New(),
			Name:      "Test Look",
			ProjectID: project.ID,
		},
	}

	err := service.RecordOperation(
		ctx,
		project.ID,
		OperationTypeCreate,
		EntityTypeLook,
		snapshot.Look.ID,
		"Created test look",
		nil,
		snapshot,
		nil,
	)
	if err != nil {
		t.Fatalf("RecordOperation failed: %v", err)
	}

	// Verify operation was recorded
	status, err := service.GetStatus(ctx, project.ID)
	if err != nil {
		t.Fatalf("GetStatus failed: %v", err)
	}
	if !status.CanUndo {
		t.Error("Expected CanUndo=true after recording operation")
	}
	if status.TotalOperations != 1 {
		t.Errorf("Expected TotalOperations=1, got %d", status.TotalOperations)
	}
}

func TestService_RecordOperationWithRelatedIDs(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	service := createTestService(t, db)
	ctx := context.Background()
	project := createTestProject(t, db)

	relatedIDs := []string{"id1", "id2", "id3"}
	err := service.RecordOperation(
		ctx,
		project.ID,
		OperationTypeBulk,
		EntityTypeLook,
		"bulk-op",
		"Bulk created looks",
		nil,
		struct{}{},
		relatedIDs,
	)
	if err != nil {
		t.Fatalf("RecordOperation with related IDs failed: %v", err)
	}
}

func TestService_Undo_NothingToUndo(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	service := createTestService(t, db)
	ctx := context.Background()
	project := createTestProject(t, db)

	result, err := service.Undo(ctx, project.ID)
	if err != nil {
		t.Fatalf("Undo failed: %v", err)
	}
	if result.Success {
		t.Error("Expected Success=false when nothing to undo")
	}
	if result.Message != "Nothing to undo" {
		t.Errorf("Expected message 'Nothing to undo', got '%s'", result.Message)
	}
}

func TestService_Redo_NothingToRedo(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	service := createTestService(t, db)
	ctx := context.Background()
	project := createTestProject(t, db)

	result, err := service.Redo(ctx, project.ID)
	if err != nil {
		t.Fatalf("Redo failed: %v", err)
	}
	if result.Success {
		t.Error("Expected Success=false when nothing to redo")
	}
	if result.Message != "Nothing to redo" {
		t.Errorf("Expected message 'Nothing to redo', got '%s'", result.Message)
	}
}

func TestService_UndoRedo_LookCreate(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	service := createTestService(t, db)
	ctx := context.Background()
	project := createTestProject(t, db)

	// Create a look
	look := createTestLook(t, db, project.ID)

	// Record the creation
	newSnapshot := LookSnapshot{
		Look: look,
	}
	err := service.RecordOperation(
		ctx,
		project.ID,
		OperationTypeCreate,
		EntityTypeLook,
		look.ID,
		"Created look",
		nil, // No previous state for create
		newSnapshot,
		nil,
	)
	if err != nil {
		t.Fatalf("RecordOperation failed: %v", err)
	}

	// Verify look exists
	var foundLook models.Look
	result := db.First(&foundLook, "id = ?", look.ID)
	if result.Error != nil {
		t.Fatalf("Look should exist: %v", result.Error)
	}

	// Undo the creation (should delete the look)
	undoResult, err := service.Undo(ctx, project.ID)
	if err != nil {
		t.Fatalf("Undo failed: %v", err)
	}
	if !undoResult.Success {
		t.Errorf("Expected undo success, got: %s", undoResult.Message)
	}

	// Verify look was deleted
	result = db.First(&foundLook, "id = ?", look.ID)
	if result.Error == nil {
		t.Error("Look should have been deleted after undo")
	}

	// Redo the creation (should recreate the look)
	redoResult, err := service.Redo(ctx, project.ID)
	if err != nil {
		t.Fatalf("Redo failed: %v", err)
	}
	if !redoResult.Success {
		t.Errorf("Expected redo success, got: %s", redoResult.Message)
	}

	// Verify look was recreated
	result = db.First(&foundLook, "id = ?", look.ID)
	if result.Error != nil {
		t.Errorf("Look should exist after redo: %v", result.Error)
	}
}

func TestService_UndoRedo_LookUpdate(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	service := createTestService(t, db)
	ctx := context.Background()
	project := createTestProject(t, db)

	// Create a look
	look := createTestLook(t, db, project.ID)
	originalName := look.Name

	// Capture state before update
	prevSnapshot := LookSnapshot{
		Look: &models.Look{
			ID:        look.ID,
			Name:      look.Name,
			ProjectID: look.ProjectID,
		},
	}

	// Update the look
	newName := "Updated Name"
	look.Name = newName
	if err := db.Save(look).Error; err != nil {
		t.Fatalf("Failed to update look: %v", err)
	}

	// Capture state after update
	newSnapshot := LookSnapshot{
		Look: &models.Look{
			ID:        look.ID,
			Name:      newName,
			ProjectID: look.ProjectID,
		},
	}

	// Record the update
	err := service.RecordOperation(
		ctx,
		project.ID,
		OperationTypeUpdate,
		EntityTypeLook,
		look.ID,
		"Updated look",
		prevSnapshot,
		newSnapshot,
		nil,
	)
	if err != nil {
		t.Fatalf("RecordOperation failed: %v", err)
	}

	// Undo should restore original name
	_, err = service.Undo(ctx, project.ID)
	if err != nil {
		t.Fatalf("Undo failed: %v", err)
	}

	var foundLook models.Look
	db.First(&foundLook, "id = ?", look.ID)
	if foundLook.Name != originalName {
		t.Errorf("Expected name '%s' after undo, got '%s'", originalName, foundLook.Name)
	}

	// Redo should restore updated name
	_, err = service.Redo(ctx, project.ID)
	if err != nil {
		t.Fatalf("Redo failed: %v", err)
	}

	db.First(&foundLook, "id = ?", look.ID)
	if foundLook.Name != newName {
		t.Errorf("Expected name '%s' after redo, got '%s'", newName, foundLook.Name)
	}
}

func TestService_UndoRedo_LookDelete(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	service := createTestService(t, db)
	ctx := context.Background()
	project := createTestProject(t, db)

	// Create a look
	look := createTestLook(t, db, project.ID)

	// Capture state before deletion
	prevSnapshot := LookSnapshot{
		Look: look,
	}

	// Delete the look
	if err := db.Delete(look).Error; err != nil {
		t.Fatalf("Failed to delete look: %v", err)
	}

	// Record the deletion
	err := service.RecordOperation(
		ctx,
		project.ID,
		OperationTypeDelete,
		EntityTypeLook,
		look.ID,
		"Deleted look",
		prevSnapshot,
		nil, // No new state for delete
		nil,
	)
	if err != nil {
		t.Fatalf("RecordOperation failed: %v", err)
	}

	// Verify look was deleted
	var foundLook models.Look
	result := db.First(&foundLook, "id = ?", look.ID)
	if result.Error == nil {
		t.Error("Look should be deleted")
	}

	// Undo should recreate the look
	undoResult, err := service.Undo(ctx, project.ID)
	if err != nil {
		t.Fatalf("Undo failed: %v", err)
	}
	if !undoResult.Success {
		t.Errorf("Expected undo success, got: %s", undoResult.Message)
	}

	result = db.First(&foundLook, "id = ?", look.ID)
	if result.Error != nil {
		t.Errorf("Look should be restored after undo: %v", result.Error)
	}
}

func TestService_UndoRedo_FixtureInstance(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	service := createTestService(t, db)
	ctx := context.Background()
	project := createTestProject(t, db)

	// Create a fixture
	fixture := createTestFixtureInstance(t, db, project.ID)

	// Capture state
	newSnapshot := FixtureInstanceSnapshot{
		Fixture: fixture,
	}

	// Record the creation
	err := service.RecordOperation(
		ctx,
		project.ID,
		OperationTypeCreate,
		EntityTypeFixtureInstance,
		fixture.ID,
		"Created fixture",
		nil,
		newSnapshot,
		nil,
	)
	if err != nil {
		t.Fatalf("RecordOperation failed: %v", err)
	}

	// Undo should delete the fixture
	undoResult, err := service.Undo(ctx, project.ID)
	if err != nil {
		t.Fatalf("Undo failed: %v", err)
	}
	if !undoResult.Success {
		t.Errorf("Expected undo success, got: %s", undoResult.Message)
	}

	var foundFixture models.FixtureInstance
	result := db.First(&foundFixture, "id = ?", fixture.ID)
	if result.Error == nil {
		t.Error("Fixture should be deleted after undo")
	}

	// Redo should recreate the fixture
	redoResult, err := service.Redo(ctx, project.ID)
	if err != nil {
		t.Fatalf("Redo failed: %v", err)
	}
	if !redoResult.Success {
		t.Errorf("Expected redo success, got: %s", redoResult.Message)
	}

	result = db.First(&foundFixture, "id = ?", fixture.ID)
	if result.Error != nil {
		t.Errorf("Fixture should exist after redo: %v", result.Error)
	}
}

func TestService_UndoRedo_Cue(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	service := createTestService(t, db)
	ctx := context.Background()
	project := createTestProject(t, db)

	// Create prerequisite entities
	look := createTestLook(t, db, project.ID)
	cueList := createTestCueList(t, db, project.ID)
	cue := createTestCue(t, db, cueList.ID, look.ID)

	// Capture state
	newSnapshot := CueSnapshot{
		Cue: cue,
	}

	// Record the creation
	err := service.RecordOperation(
		ctx,
		project.ID,
		OperationTypeCreate,
		EntityTypeCue,
		cue.ID,
		"Created cue",
		nil,
		newSnapshot,
		nil,
	)
	if err != nil {
		t.Fatalf("RecordOperation failed: %v", err)
	}

	// Undo should delete the cue
	undoResult, err := service.Undo(ctx, project.ID)
	if err != nil {
		t.Fatalf("Undo failed: %v", err)
	}
	if !undoResult.Success {
		t.Errorf("Expected undo success, got: %s", undoResult.Message)
	}

	var foundCue models.Cue
	result := db.First(&foundCue, "id = ?", cue.ID)
	if result.Error == nil {
		t.Error("Cue should be deleted after undo")
	}

	// Redo should recreate the cue
	redoResult, err := service.Redo(ctx, project.ID)
	if err != nil {
		t.Fatalf("Redo failed: %v", err)
	}
	if !redoResult.Success {
		t.Errorf("Expected redo success, got: %s", redoResult.Message)
	}

	result = db.First(&foundCue, "id = ?", cue.ID)
	if result.Error != nil {
		t.Errorf("Cue should exist after redo: %v", result.Error)
	}
}

func TestService_UndoRedo_CueList(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	service := createTestService(t, db)
	ctx := context.Background()
	project := createTestProject(t, db)

	// Create a cue list
	cueList := createTestCueList(t, db, project.ID)

	// Capture state
	newSnapshot := CueListSnapshot{
		CueList: cueList,
	}

	// Record the creation
	err := service.RecordOperation(
		ctx,
		project.ID,
		OperationTypeCreate,
		EntityTypeCueList,
		cueList.ID,
		"Created cue list",
		nil,
		newSnapshot,
		nil,
	)
	if err != nil {
		t.Fatalf("RecordOperation failed: %v", err)
	}

	// Undo should delete the cue list
	undoResult, err := service.Undo(ctx, project.ID)
	if err != nil {
		t.Fatalf("Undo failed: %v", err)
	}
	if !undoResult.Success {
		t.Errorf("Expected undo success, got: %s", undoResult.Message)
	}

	var foundCueList models.CueList
	result := db.First(&foundCueList, "id = ?", cueList.ID)
	if result.Error == nil {
		t.Error("CueList should be deleted after undo")
	}

	// Redo should recreate the cue list
	redoResult, err := service.Redo(ctx, project.ID)
	if err != nil {
		t.Fatalf("Redo failed: %v", err)
	}
	if !redoResult.Success {
		t.Errorf("Expected redo success, got: %s", redoResult.Message)
	}

	result = db.First(&foundCueList, "id = ?", cueList.ID)
	if result.Error != nil {
		t.Errorf("CueList should exist after redo: %v", result.Error)
	}
}

func TestService_JumpToOperation(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	service := createTestService(t, db)
	ctx := context.Background()
	project := createTestProject(t, db)

	// Create several looks and record operations
	var operationIDs []string
	for i := 0; i < 5; i++ {
		look := createTestLook(t, db, project.ID)
		snapshot := LookSnapshot{Look: look}

		// Get the operation repository to capture the operation ID
		opRepo := repositories.NewOperationRepository(db)
		op := &models.Operation{
			ProjectID:     project.ID,
			OperationType: "CREATE",
			EntityType:    "Look",
			EntityID:      look.ID,
			Description:   "Created look",
		}
		prevJSON, _ := repositories.MarshalState(nil)
		newJSON, _ := repositories.MarshalState(snapshot)
		op.PreviousState = prevJSON
		op.NewState = newJSON

		err := opRepo.RecordOperation(ctx, op)
		if err != nil {
			t.Fatalf("RecordOperation %d failed: %v", i, err)
		}
		operationIDs = append(operationIDs, op.ID)
	}

	// Jump to operation 2 (should undo operations 5, 4, 3)
	result, err := service.JumpToOperation(ctx, project.ID, operationIDs[1])
	if err != nil {
		t.Fatalf("JumpToOperation failed: %v", err)
	}
	if !result.Success {
		t.Errorf("Expected success, got: %s", result.Message)
	}

	// Verify current sequence
	status, _ := service.GetStatus(ctx, project.ID)
	if status.CurrentSequence != 2 {
		t.Errorf("Expected CurrentSequence=2 after jump, got %d", status.CurrentSequence)
	}

	// Jump forward to operation 4
	result, err = service.JumpToOperation(ctx, project.ID, operationIDs[3])
	if err != nil {
		t.Fatalf("JumpToOperation forward failed: %v", err)
	}
	if !result.Success {
		t.Errorf("Expected success, got: %s", result.Message)
	}

	status, _ = service.GetStatus(ctx, project.ID)
	if status.CurrentSequence != 4 {
		t.Errorf("Expected CurrentSequence=4 after jump, got %d", status.CurrentSequence)
	}
}

func TestService_JumpToOperation_NotFound(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	service := createTestService(t, db)
	ctx := context.Background()
	project := createTestProject(t, db)

	result, err := service.JumpToOperation(ctx, project.ID, "nonexistent")
	if err != nil {
		t.Fatalf("JumpToOperation failed: %v", err)
	}
	if result.Success {
		t.Error("Expected failure for non-existent operation")
	}
	if result.Message != "Operation not found" {
		t.Errorf("Expected 'Operation not found', got '%s'", result.Message)
	}
}

func TestService_JumpToOperation_WrongProject(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	service := createTestService(t, db)
	ctx := context.Background()
	project1 := createTestProject(t, db)
	project2 := createTestProject(t, db)

	// Create an operation in project1
	look := createTestLook(t, db, project1.ID)
	snapshot := LookSnapshot{Look: look}

	opRepo := repositories.NewOperationRepository(db)
	op := &models.Operation{
		ProjectID:     project1.ID,
		OperationType: "CREATE",
		EntityType:    "Look",
		EntityID:      look.ID,
		Description:   "Created look",
	}
	newJSON, _ := repositories.MarshalState(snapshot)
	op.NewState = newJSON
	_ = opRepo.RecordOperation(ctx, op)

	// Try to jump to it from project2
	result, err := service.JumpToOperation(ctx, project2.ID, op.ID)
	if err != nil {
		t.Fatalf("JumpToOperation failed: %v", err)
	}
	if result.Success {
		t.Error("Expected failure for wrong project")
	}
	if result.Message != "Operation does not belong to this project" {
		t.Errorf("Unexpected message: '%s'", result.Message)
	}
}

func TestService_GetStatus(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	service := createTestService(t, db)
	ctx := context.Background()
	project := createTestProject(t, db)

	// Initially empty
	status, err := service.GetStatus(ctx, project.ID)
	if err != nil {
		t.Fatalf("GetStatus failed: %v", err)
	}
	if status.CanUndo || status.CanRedo {
		t.Error("Expected CanUndo=false and CanRedo=false initially")
	}
}

func TestService_GetOperationHistory(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	service := createTestService(t, db)
	ctx := context.Background()
	project := createTestProject(t, db)

	// Record some operations
	for i := 0; i < 3; i++ {
		look := createTestLook(t, db, project.ID)
		snapshot := LookSnapshot{Look: look}
		_ = service.RecordOperation(
			ctx,
			project.ID,
			OperationTypeCreate,
			EntityTypeLook,
			look.ID,
			"Created look",
			nil,
			snapshot,
			nil,
		)
	}

	// Get history
	ops, total, err := service.GetOperationHistory(ctx, project.ID, 1, 10)
	if err != nil {
		t.Fatalf("GetOperationHistory failed: %v", err)
	}
	if total != 3 {
		t.Errorf("Expected total=3, got %d", total)
	}
	if len(ops) != 3 {
		t.Errorf("Expected 3 operations, got %d", len(ops))
	}
}

func TestService_ClearHistory(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	service := createTestService(t, db)
	ctx := context.Background()
	project := createTestProject(t, db)

	// Record an operation
	look := createTestLook(t, db, project.ID)
	snapshot := LookSnapshot{Look: look}
	_ = service.RecordOperation(
		ctx,
		project.ID,
		OperationTypeCreate,
		EntityTypeLook,
		look.ID,
		"Created look",
		nil,
		snapshot,
		nil,
	)

	// Clear history
	err := service.ClearHistory(ctx, project.ID)
	if err != nil {
		t.Fatalf("ClearHistory failed: %v", err)
	}

	// Verify history is empty
	_, total, _ := service.GetOperationHistory(ctx, project.ID, 1, 10)
	if total != 0 {
		t.Errorf("Expected 0 operations after clear, got %d", total)
	}
}

func TestService_CaptureLookState(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	service := createTestService(t, db)
	ctx := context.Background()
	project := createTestProject(t, db)

	look := createTestLook(t, db, project.ID)

	snapshot, err := service.CaptureLookState(ctx, look.ID)
	if err != nil {
		t.Fatalf("CaptureLookState failed: %v", err)
	}
	if snapshot == nil {
		t.Fatal("Expected non-nil snapshot")
	}
	if snapshot.Look == nil {
		t.Fatal("Expected non-nil Look in snapshot")
	}
	if snapshot.Look.ID != look.ID {
		t.Errorf("Expected Look ID=%s, got %s", look.ID, snapshot.Look.ID)
	}

	// Test with non-existent look
	snapshot, err = service.CaptureLookState(ctx, "nonexistent")
	if err != nil {
		t.Fatalf("CaptureLookState for non-existent failed: %v", err)
	}
	if snapshot != nil {
		t.Error("Expected nil snapshot for non-existent look")
	}
}

func TestService_CaptureFixtureState(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	service := createTestService(t, db)
	ctx := context.Background()
	project := createTestProject(t, db)

	fixture := createTestFixtureInstance(t, db, project.ID)

	snapshot, err := service.CaptureFixtureState(ctx, fixture.ID)
	if err != nil {
		t.Fatalf("CaptureFixtureState failed: %v", err)
	}
	if snapshot == nil {
		t.Fatal("Expected non-nil snapshot")
	}
	if snapshot.Fixture == nil {
		t.Fatal("Expected non-nil Fixture in snapshot")
	}
	if snapshot.Fixture.ID != fixture.ID {
		t.Errorf("Expected Fixture ID=%s, got %s", fixture.ID, snapshot.Fixture.ID)
	}
}

func TestService_CaptureCueState(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	service := createTestService(t, db)
	ctx := context.Background()
	project := createTestProject(t, db)

	look := createTestLook(t, db, project.ID)
	cueList := createTestCueList(t, db, project.ID)
	cue := createTestCue(t, db, cueList.ID, look.ID)

	snapshot, err := service.CaptureCueState(ctx, cue.ID)
	if err != nil {
		t.Fatalf("CaptureCueState failed: %v", err)
	}
	if snapshot == nil {
		t.Fatal("Expected non-nil snapshot")
	}
	if snapshot.Cue == nil {
		t.Fatal("Expected non-nil Cue in snapshot")
	}
	if snapshot.Cue.ID != cue.ID {
		t.Errorf("Expected Cue ID=%s, got %s", cue.ID, snapshot.Cue.ID)
	}
}

func TestService_CaptureCueListState(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	service := createTestService(t, db)
	ctx := context.Background()
	project := createTestProject(t, db)

	cueList := createTestCueList(t, db, project.ID)

	snapshot, err := service.CaptureCueListState(ctx, cueList.ID)
	if err != nil {
		t.Fatalf("CaptureCueListState failed: %v", err)
	}
	if snapshot == nil {
		t.Fatal("Expected non-nil snapshot")
	}
	if snapshot.CueList == nil {
		t.Fatal("Expected non-nil CueList in snapshot")
	}
	if snapshot.CueList.ID != cueList.ID {
		t.Errorf("Expected CueList ID=%s, got %s", cueList.ID, snapshot.CueList.ID)
	}
}

func TestService_DeleteEntityForUndo(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	service := createTestService(t, db)
	ctx := context.Background()
	project := createTestProject(t, db)

	// Test deleting Look
	look := createTestLook(t, db, project.ID)
	err := service.DeleteEntityForUndo(ctx, EntityTypeLook, look.ID)
	if err != nil {
		t.Fatalf("DeleteEntityForUndo Look failed: %v", err)
	}

	// Test deleting FixtureInstance
	fixture := createTestFixtureInstance(t, db, project.ID)
	err = service.DeleteEntityForUndo(ctx, EntityTypeFixtureInstance, fixture.ID)
	if err != nil {
		t.Fatalf("DeleteEntityForUndo Fixture failed: %v", err)
	}

	// Test unsupported entity type
	err = service.DeleteEntityForUndo(ctx, "UnsupportedType", "someid")
	if err == nil {
		t.Error("Expected error for unsupported entity type")
	}
}

func TestService_ApplySnapshot_UnsupportedType(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	service := createTestService(t, db)
	ctx := context.Background()
	project := createTestProject(t, db)

	// Record an operation with unsupported entity type
	opRepo := repositories.NewOperationRepository(db)
	op := &models.Operation{
		ProjectID:     project.ID,
		OperationType: "CREATE",
		EntityType:    "UnsupportedEntity",
		EntityID:      "someid",
		Description:   "Test unsupported",
		NewState:      "{}",
	}
	_ = opRepo.RecordOperation(ctx, op)

	// Try to undo - should fail
	_, err := service.Undo(ctx, project.ID)
	if err == nil {
		t.Error("Expected error for unsupported entity type")
	}
}

func TestService_ApplyCuePlaybackSnapshot(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	service := createTestService(t, db)
	ctx := context.Background()
	project := createTestProject(t, db)

	// Test without playback controller - should fail
	snapshot := CuePlaybackSnapshot{
		CueListID: "test-cue-list",
		IsPlaying: true,
	}
	snapshotJSON, _ := json.Marshal(snapshot)

	opRepo := repositories.NewOperationRepository(db)
	op := &models.Operation{
		ProjectID:     project.ID,
		OperationType: "UPDATE",
		EntityType:    string(EntityTypeCuePlayback),
		EntityID:      "playback",
		Description:   "Playback change",
		NewState:      string(snapshotJSON),
	}
	_ = opRepo.RecordOperation(ctx, op)

	_, err := service.Undo(ctx, project.ID)
	if err == nil {
		t.Error("Expected error when playback controller not set")
	}

	// Clear history and test with playback controller
	_ = service.ClearHistory(ctx, project.ID)

	mockController := &mockPlaybackController{}
	service.SetPlaybackController(mockController)

	// Record operation for playing a cue - use a stopped previous state
	stopSnapshot := CuePlaybackSnapshot{
		CueListID: "test-cue-list",
		IsPlaying: false,
	}
	stopJSON, _ := json.Marshal(stopSnapshot)

	cueNumber := 1.0
	playSnapshot := CuePlaybackSnapshot{
		CueListID: "test-cue-list",
		IsPlaying: true,
		CueNumber: &cueNumber,
	}
	playJSON, _ := json.Marshal(playSnapshot)

	op2 := &models.Operation{
		ProjectID:     project.ID,
		OperationType: "UPDATE",
		EntityType:    string(EntityTypeCuePlayback),
		EntityID:      "playback2",
		Description:   "Playing cue",
		PreviousState: string(stopJSON),
		NewState:      string(playJSON),
	}
	_ = opRepo.RecordOperation(ctx, op2)

	// Undo and then Redo should call playback controller
	_, err = service.Undo(ctx, project.ID)
	if err != nil {
		t.Fatalf("Undo failed: %v", err)
	}

	result, err := service.Redo(ctx, project.ID)
	if err != nil {
		t.Fatalf("Redo failed: %v", err)
	}
	if !result.Success {
		t.Errorf("Expected success, got: %s", result.Message)
	}
	if !mockController.goToCueCalled {
		t.Error("Expected GoToCueNumber to be called")
	}

	// Test stop playback - record a stop operation
	op3 := &models.Operation{
		ProjectID:     project.ID,
		OperationType: "UPDATE",
		EntityType:    string(EntityTypeCuePlayback),
		EntityID:      "playback3",
		Description:   "Stopped playback",
		PreviousState: string(playJSON),
		NewState:      string(stopJSON),
	}
	_ = opRepo.RecordOperation(ctx, op3)

	// Undo then Redo should call stop
	_, _ = service.Undo(ctx, project.ID)
	mockController.stopCalled = false
	_, err = service.Redo(ctx, project.ID)
	if err != nil {
		t.Fatalf("Redo stop failed: %v", err)
	}
	if !mockController.stopCalled {
		t.Error("Expected StopCueList to be called")
	}
}

// createTestLookBoard creates a look board for testing.
func createTestLookBoard(t *testing.T, db *gorm.DB, projectID string) *models.LookBoard {
	t.Helper()

	lookBoard := &models.LookBoard{
		ID:              cuid.New(),
		Name:            "Test Look Board " + cuid.Slug(),
		ProjectID:       projectID,
		CanvasWidth:     2000,
		CanvasHeight:    2000,
		GridSize:        intPtr(50),
		DefaultFadeTime: 3.0,
	}
	if err := db.Create(lookBoard).Error; err != nil {
		t.Fatalf("Failed to create look board: %v", err)
	}
	return lookBoard
}

// createTestLookBoardButton creates a look board button for testing.
func createTestLookBoardButton(t *testing.T, db *gorm.DB, lookBoardID, lookID string) *models.LookBoardButton {
	t.Helper()

	button := &models.LookBoardButton{
		ID:          cuid.New(),
		LookBoardID: lookBoardID,
		LookID:      lookID,
		LayoutX:     100,
		LayoutY:     100,
		Width:       intPtr(200),
		Height:      intPtr(120),
	}
	if err := db.Create(button).Error; err != nil {
		t.Fatalf("Failed to create look board button: %v", err)
	}
	return button
}

func TestService_UndoRedo_LookBoard(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	service := createTestService(t, db)
	ctx := context.Background()
	project := createTestProject(t, db)

	// Create a look board
	lookBoard := createTestLookBoard(t, db, project.ID)

	// Capture state
	newSnapshot := LookBoardSnapshot{
		LookBoard: lookBoard,
	}

	// Record the creation
	err := service.RecordOperation(
		ctx,
		project.ID,
		OperationTypeCreate,
		EntityTypeLookBoard,
		lookBoard.ID,
		"Created look board",
		nil,
		newSnapshot,
		nil,
	)
	if err != nil {
		t.Fatalf("RecordOperation failed: %v", err)
	}

	// Undo should delete the look board
	undoResult, err := service.Undo(ctx, project.ID)
	if err != nil {
		t.Fatalf("Undo failed: %v", err)
	}
	if !undoResult.Success {
		t.Errorf("Expected undo success, got: %s", undoResult.Message)
	}

	var foundBoard models.LookBoard
	result := db.First(&foundBoard, "id = ?", lookBoard.ID)
	if result.Error == nil {
		t.Error("LookBoard should be deleted after undo")
	}

	// Redo should recreate the look board
	redoResult, err := service.Redo(ctx, project.ID)
	if err != nil {
		t.Fatalf("Redo failed: %v", err)
	}
	if !redoResult.Success {
		t.Errorf("Expected redo success, got: %s", redoResult.Message)
	}

	result = db.First(&foundBoard, "id = ?", lookBoard.ID)
	if result.Error != nil {
		t.Errorf("LookBoard should exist after redo: %v", result.Error)
	}
}

func TestService_UndoRedo_LookBoardWithButtons(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	service := createTestService(t, db)
	ctx := context.Background()
	project := createTestProject(t, db)

	// Create a look for the button
	look := createTestLook(t, db, project.ID)

	// Create a look board with buttons
	lookBoard := createTestLookBoard(t, db, project.ID)
	button := createTestLookBoardButton(t, db, lookBoard.ID, look.ID)

	// Capture state before deletion (for delete operation)
	prevSnapshot := LookBoardSnapshot{
		LookBoard: lookBoard,
		Buttons:   []models.LookBoardButton{*button},
	}

	// Delete the look board
	db.Delete(&models.LookBoardButton{}, "look_board_id = ?", lookBoard.ID)
	if err := db.Delete(lookBoard).Error; err != nil {
		t.Fatalf("Failed to delete look board: %v", err)
	}

	// Record the deletion
	err := service.RecordOperation(
		ctx,
		project.ID,
		OperationTypeDelete,
		EntityTypeLookBoard,
		lookBoard.ID,
		"Deleted look board with buttons",
		prevSnapshot,
		nil,
		nil,
	)
	if err != nil {
		t.Fatalf("RecordOperation failed: %v", err)
	}

	// Undo should recreate the look board with buttons
	undoResult, err := service.Undo(ctx, project.ID)
	if err != nil {
		t.Fatalf("Undo failed: %v", err)
	}
	if !undoResult.Success {
		t.Errorf("Expected undo success, got: %s", undoResult.Message)
	}

	// Verify look board exists
	var foundBoard models.LookBoard
	result := db.First(&foundBoard, "id = ?", lookBoard.ID)
	if result.Error != nil {
		t.Errorf("LookBoard should exist after undo: %v", result.Error)
	}

	// Verify button was recreated
	var foundButton models.LookBoardButton
	result = db.First(&foundButton, "id = ?", button.ID)
	if result.Error != nil {
		t.Errorf("LookBoardButton should exist after undo: %v", result.Error)
	}
}

func TestService_UndoRedo_LookBoardDelete(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	service := createTestService(t, db)
	ctx := context.Background()
	project := createTestProject(t, db)

	// Create a look board
	lookBoard := createTestLookBoard(t, db, project.ID)

	// Capture state before deletion
	prevSnapshot := LookBoardSnapshot{
		LookBoard: lookBoard,
	}

	// Delete the look board
	if err := db.Delete(lookBoard).Error; err != nil {
		t.Fatalf("Failed to delete look board: %v", err)
	}

	// Record the deletion
	err := service.RecordOperation(
		ctx,
		project.ID,
		OperationTypeDelete,
		EntityTypeLookBoard,
		lookBoard.ID,
		"Deleted look board",
		prevSnapshot,
		nil,
		nil,
	)
	if err != nil {
		t.Fatalf("RecordOperation failed: %v", err)
	}

	// Undo should recreate the look board
	undoResult, err := service.Undo(ctx, project.ID)
	if err != nil {
		t.Fatalf("Undo failed: %v", err)
	}
	if !undoResult.Success {
		t.Errorf("Expected undo success, got: %s", undoResult.Message)
	}

	var foundBoard models.LookBoard
	result := db.First(&foundBoard, "id = ?", lookBoard.ID)
	if result.Error != nil {
		t.Errorf("LookBoard should be restored after undo: %v", result.Error)
	}
}

func TestService_UndoRedo_LookBoardUpdate(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	service := createTestService(t, db)
	ctx := context.Background()
	project := createTestProject(t, db)

	// Create a look board
	lookBoard := createTestLookBoard(t, db, project.ID)
	originalName := lookBoard.Name

	// Capture state before update
	prevSnapshot := LookBoardSnapshot{
		LookBoard: &models.LookBoard{
			ID:              lookBoard.ID,
			Name:            lookBoard.Name,
			ProjectID:       lookBoard.ProjectID,
			CanvasWidth:     lookBoard.CanvasWidth,
			CanvasHeight:    lookBoard.CanvasHeight,
			GridSize:        lookBoard.GridSize,
			DefaultFadeTime: lookBoard.DefaultFadeTime,
		},
	}

	// Update the look board
	newName := "Updated Board Name"
	lookBoard.Name = newName
	if err := db.Save(lookBoard).Error; err != nil {
		t.Fatalf("Failed to update look board: %v", err)
	}

	// Capture state after update
	newSnapshot := LookBoardSnapshot{
		LookBoard: &models.LookBoard{
			ID:              lookBoard.ID,
			Name:            newName,
			ProjectID:       lookBoard.ProjectID,
			CanvasWidth:     lookBoard.CanvasWidth,
			CanvasHeight:    lookBoard.CanvasHeight,
			GridSize:        lookBoard.GridSize,
			DefaultFadeTime: lookBoard.DefaultFadeTime,
		},
	}

	// Record the update
	err := service.RecordOperation(
		ctx,
		project.ID,
		OperationTypeUpdate,
		EntityTypeLookBoard,
		lookBoard.ID,
		"Updated look board",
		prevSnapshot,
		newSnapshot,
		nil,
	)
	if err != nil {
		t.Fatalf("RecordOperation failed: %v", err)
	}

	// Undo should restore original name
	_, err = service.Undo(ctx, project.ID)
	if err != nil {
		t.Fatalf("Undo failed: %v", err)
	}

	var foundBoard models.LookBoard
	db.First(&foundBoard, "id = ?", lookBoard.ID)
	if foundBoard.Name != originalName {
		t.Errorf("Expected name '%s' after undo, got '%s'", originalName, foundBoard.Name)
	}
}

func TestService_CaptureLookBoardState(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	service := createTestService(t, db)
	ctx := context.Background()
	project := createTestProject(t, db)

	lookBoard := createTestLookBoard(t, db, project.ID)

	snapshot, err := service.CaptureLookBoardState(ctx, lookBoard.ID)
	if err != nil {
		t.Fatalf("CaptureLookBoardState failed: %v", err)
	}
	if snapshot == nil {
		t.Fatal("Expected non-nil snapshot")
	}
	if snapshot.LookBoard == nil {
		t.Fatal("Expected non-nil LookBoard in snapshot")
	}
	if snapshot.LookBoard.ID != lookBoard.ID {
		t.Errorf("Expected LookBoard ID=%s, got %s", lookBoard.ID, snapshot.LookBoard.ID)
	}

	// Test with non-existent look board
	snapshot, err = service.CaptureLookBoardState(ctx, "nonexistent")
	if err != nil {
		t.Fatalf("CaptureLookBoardState for non-existent failed: %v", err)
	}
	if snapshot != nil {
		t.Error("Expected nil snapshot for non-existent look board")
	}
}

func TestService_CaptureLookBoardState_WithButtons(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	service := createTestService(t, db)
	ctx := context.Background()
	project := createTestProject(t, db)

	look := createTestLook(t, db, project.ID)
	lookBoard := createTestLookBoard(t, db, project.ID)
	_ = createTestLookBoardButton(t, db, lookBoard.ID, look.ID)

	snapshot, err := service.CaptureLookBoardState(ctx, lookBoard.ID)
	if err != nil {
		t.Fatalf("CaptureLookBoardState failed: %v", err)
	}
	if snapshot == nil {
		t.Fatal("Expected non-nil snapshot")
	}
	if len(snapshot.Buttons) != 1 {
		t.Errorf("Expected 1 button, got %d", len(snapshot.Buttons))
	}
}

func TestService_UndoRedo_LookWithFixtureValues(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	service := createTestService(t, db)
	ctx := context.Background()
	project := createTestProject(t, db)

	// Create a fixture
	fixture := createTestFixtureInstance(t, db, project.ID)

	// Create a look
	look := createTestLook(t, db, project.ID)

	// Add fixture values
	fixtureValue := models.FixtureValue{
		ID:        cuid.New(),
		LookID:    look.ID,
		FixtureID: fixture.ID,
		LookOrder: intPtr(1),
	}
	if err := db.Create(&fixtureValue).Error; err != nil {
		t.Fatalf("Failed to create fixture value: %v", err)
	}

	// Capture state with fixture values
	newSnapshot := LookSnapshot{
		Look:          look,
		FixtureValues: []models.FixtureValue{fixtureValue},
	}

	// Record the creation
	err := service.RecordOperation(
		ctx,
		project.ID,
		OperationTypeCreate,
		EntityTypeLook,
		look.ID,
		"Created look with fixtures",
		nil,
		newSnapshot,
		nil,
	)
	if err != nil {
		t.Fatalf("RecordOperation failed: %v", err)
	}

	// Undo should delete the look and fixture values
	undoResult, err := service.Undo(ctx, project.ID)
	if err != nil {
		t.Fatalf("Undo failed: %v", err)
	}
	if !undoResult.Success {
		t.Errorf("Expected undo success, got: %s", undoResult.Message)
	}

	// Verify look was deleted
	var foundLook models.Look
	result := db.First(&foundLook, "id = ?", look.ID)
	if result.Error == nil {
		t.Error("Look should be deleted after undo")
	}

	// Redo should recreate the look with fixture values
	redoResult, err := service.Redo(ctx, project.ID)
	if err != nil {
		t.Fatalf("Redo failed: %v", err)
	}
	if !redoResult.Success {
		t.Errorf("Expected redo success, got: %s", redoResult.Message)
	}

	// Verify look exists
	result = db.First(&foundLook, "id = ?", look.ID)
	if result.Error != nil {
		t.Errorf("Look should exist after redo: %v", result.Error)
	}

	// Verify fixture values exist
	var foundFV models.FixtureValue
	result = db.First(&foundFV, "look_id = ?", look.ID)
	if result.Error != nil {
		t.Errorf("FixtureValue should exist after redo: %v", result.Error)
	}
}

func TestService_UndoRedo_LookUpdateWithFixtureValues(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	service := createTestService(t, db)
	ctx := context.Background()
	project := createTestProject(t, db)

	// Create a fixture
	fixture := createTestFixtureInstance(t, db, project.ID)

	// Create a look
	look := createTestLook(t, db, project.ID)

	// Capture previous state (no fixture values)
	prevSnapshot := LookSnapshot{
		Look:          look,
		FixtureValues: []models.FixtureValue{},
	}

	// Add fixture values
	fixtureValue := models.FixtureValue{
		ID:        cuid.New(),
		LookID:    look.ID,
		FixtureID: fixture.ID,
		LookOrder: intPtr(1),
	}
	if err := db.Create(&fixtureValue).Error; err != nil {
		t.Fatalf("Failed to create fixture value: %v", err)
	}

	// Capture new state with fixture values
	newSnapshot := LookSnapshot{
		Look:          look,
		FixtureValues: []models.FixtureValue{fixtureValue},
	}

	// Record the update
	err := service.RecordOperation(
		ctx,
		project.ID,
		OperationTypeUpdate,
		EntityTypeLook,
		look.ID,
		"Updated look with fixtures",
		prevSnapshot,
		newSnapshot,
		nil,
	)
	if err != nil {
		t.Fatalf("RecordOperation failed: %v", err)
	}

	// Undo should remove fixture values
	_, err = service.Undo(ctx, project.ID)
	if err != nil {
		t.Fatalf("Undo failed: %v", err)
	}

	// Verify no fixture values
	var count int64
	db.Model(&models.FixtureValue{}).Where("look_id = ?", look.ID).Count(&count)
	if count != 0 {
		t.Errorf("Expected 0 fixture values after undo, got %d", count)
	}

	// Redo should restore fixture values
	_, err = service.Redo(ctx, project.ID)
	if err != nil {
		t.Fatalf("Redo failed: %v", err)
	}

	db.Model(&models.FixtureValue{}).Where("look_id = ?", look.ID).Count(&count)
	if count != 1 {
		t.Errorf("Expected 1 fixture value after redo, got %d", count)
	}
}

func TestService_UndoRedo_CueListWithCues(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	service := createTestService(t, db)
	ctx := context.Background()
	project := createTestProject(t, db)

	// Create a look for the cue
	look := createTestLook(t, db, project.ID)

	// Create a cue list with cues
	cueList := createTestCueList(t, db, project.ID)
	cue := createTestCue(t, db, cueList.ID, look.ID)

	// Capture state before deletion
	prevSnapshot := CueListSnapshot{
		CueList: cueList,
		Cues:    []models.Cue{*cue},
	}

	// Delete the cue list and cues
	db.Delete(&models.Cue{}, "cue_list_id = ?", cueList.ID)
	if err := db.Delete(cueList).Error; err != nil {
		t.Fatalf("Failed to delete cue list: %v", err)
	}

	// Record the deletion
	err := service.RecordOperation(
		ctx,
		project.ID,
		OperationTypeDelete,
		EntityTypeCueList,
		cueList.ID,
		"Deleted cue list with cues",
		prevSnapshot,
		nil,
		nil,
	)
	if err != nil {
		t.Fatalf("RecordOperation failed: %v", err)
	}

	// Undo should recreate the cue list with cues
	undoResult, err := service.Undo(ctx, project.ID)
	if err != nil {
		t.Fatalf("Undo failed: %v", err)
	}
	if !undoResult.Success {
		t.Errorf("Expected undo success, got: %s", undoResult.Message)
	}

	// Verify cue list exists
	var foundCueList models.CueList
	result := db.First(&foundCueList, "id = ?", cueList.ID)
	if result.Error != nil {
		t.Errorf("CueList should exist after undo: %v", result.Error)
	}

	// Verify cue was recreated
	var foundCue models.Cue
	result = db.First(&foundCue, "id = ?", cue.ID)
	if result.Error != nil {
		t.Errorf("Cue should exist after undo: %v", result.Error)
	}
}

func TestService_UndoRedo_FixtureInstanceWithChannels(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	service := createTestService(t, db)
	ctx := context.Background()
	project := createTestProject(t, db)

	// Create a fixture with channels
	fixture := createTestFixtureInstance(t, db, project.ID)

	// Add instance channel
	channel := models.InstanceChannel{
		ID:           cuid.New(),
		FixtureID:    fixture.ID,
		Offset:       0,
		Name:         "Dimmer",
		Type:         "INTENSITY",
		DefaultValue: 128,
	}
	if err := db.Create(&channel).Error; err != nil {
		t.Fatalf("Failed to create instance channel: %v", err)
	}

	// Capture state before deletion
	prevSnapshot := FixtureInstanceSnapshot{
		Fixture:  fixture,
		Channels: []models.InstanceChannel{channel},
	}

	// Delete the fixture
	db.Delete(&models.InstanceChannel{}, "fixture_id = ?", fixture.ID)
	if err := db.Delete(fixture).Error; err != nil {
		t.Fatalf("Failed to delete fixture: %v", err)
	}

	// Record the deletion
	err := service.RecordOperation(
		ctx,
		project.ID,
		OperationTypeDelete,
		EntityTypeFixtureInstance,
		fixture.ID,
		"Deleted fixture with channels",
		prevSnapshot,
		nil,
		nil,
	)
	if err != nil {
		t.Fatalf("RecordOperation failed: %v", err)
	}

	// Undo should recreate the fixture with channels
	undoResult, err := service.Undo(ctx, project.ID)
	if err != nil {
		t.Fatalf("Undo failed: %v", err)
	}
	if !undoResult.Success {
		t.Errorf("Expected undo success, got: %s", undoResult.Message)
	}

	// Verify fixture exists
	var foundFixture models.FixtureInstance
	result := db.First(&foundFixture, "id = ?", fixture.ID)
	if result.Error != nil {
		t.Errorf("Fixture should exist after undo: %v", result.Error)
	}
}

func TestService_DeleteEntityForUndo_AllTypes(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	service := createTestService(t, db)
	ctx := context.Background()
	project := createTestProject(t, db)

	// Test deleting CueList
	cueList := createTestCueList(t, db, project.ID)
	err := service.DeleteEntityForUndo(ctx, EntityTypeCueList, cueList.ID)
	if err != nil {
		t.Fatalf("DeleteEntityForUndo CueList failed: %v", err)
	}

	// Test deleting Cue
	look := createTestLook(t, db, project.ID)
	cueList2 := createTestCueList(t, db, project.ID)
	cue := createTestCue(t, db, cueList2.ID, look.ID)
	err = service.DeleteEntityForUndo(ctx, EntityTypeCue, cue.ID)
	if err != nil {
		t.Fatalf("DeleteEntityForUndo Cue failed: %v", err)
	}

	// Test deleting LookBoard
	lookBoard := createTestLookBoard(t, db, project.ID)
	err = service.DeleteEntityForUndo(ctx, EntityTypeLookBoard, lookBoard.ID)
	if err != nil {
		t.Fatalf("DeleteEntityForUndo LookBoard failed: %v", err)
	}
}

func TestService_ApplySnapshot_InvalidJSON(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	service := createTestService(t, db)
	ctx := context.Background()
	project := createTestProject(t, db)

	opRepo := repositories.NewOperationRepository(db)

	// Test with invalid JSON for Look DELETE operation
	// When undoing a DELETE, the service parses PreviousState to recreate the entity
	op := &models.Operation{
		ProjectID:     project.ID,
		OperationType: "DELETE",
		EntityType:    string(EntityTypeLook),
		EntityID:      "someid",
		Description:   "Test invalid JSON",
		PreviousState: "invalid json {",
	}
	_ = opRepo.RecordOperation(ctx, op)

	// Undo should fail due to invalid JSON when trying to restore from PreviousState
	_, err := service.Undo(ctx, project.ID)
	if err == nil {
		t.Error("Expected error for invalid JSON")
	}
}

func TestService_RecordOperation_MarshalError(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	service := createTestService(t, db)
	ctx := context.Background()
	project := createTestProject(t, db)

	// Create a value that can't be marshaled (channel)
	ch := make(chan int)

	err := service.RecordOperation(
		ctx,
		project.ID,
		OperationTypeCreate,
		EntityTypeLook,
		"someid",
		"Test marshal error",
		ch, // Can't marshal channels
		nil,
		nil,
	)
	if err == nil {
		t.Error("Expected error for unmarshalable value")
	}
}

func TestService_Redo_ApplyError(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	service := createTestService(t, db)
	ctx := context.Background()
	project := createTestProject(t, db)

	opRepo := repositories.NewOperationRepository(db)

	// Record an operation with invalid new state
	op := &models.Operation{
		ProjectID:     project.ID,
		OperationType: "CREATE",
		EntityType:    string(EntityTypeLook),
		EntityID:      "someid",
		Description:   "Test redo error",
		PreviousState: "",
		NewState:      "invalid json",
	}
	_ = opRepo.RecordOperation(ctx, op)

	// Undo first
	_, _ = service.Undo(ctx, project.ID)

	// Redo should fail
	result, err := service.Redo(ctx, project.ID)
	if err == nil {
		t.Error("Expected error for redo with invalid JSON")
	}
	if result != nil && result.Success {
		t.Error("Expected failure result")
	}
}

func TestService_UndoRedo_FixtureUpdate(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	service := createTestService(t, db)
	ctx := context.Background()
	project := createTestProject(t, db)

	// Create a fixture
	fixture := createTestFixtureInstance(t, db, project.ID)
	originalName := fixture.Name

	// Make a copy of the fixture for the previous state
	fixtureCopy := *fixture
	prevSnapshot := FixtureInstanceSnapshot{
		Fixture: &fixtureCopy,
	}

	// Update the fixture
	newName := "Updated Fixture Name"
	fixture.Name = newName
	db.Save(fixture)

	// Capture new state (need a copy because we'll verify state after redo)
	fixtureUpdated := *fixture
	newSnapshot := FixtureInstanceSnapshot{
		Fixture: &fixtureUpdated,
	}

	// Record the update
	err := service.RecordOperation(
		ctx,
		project.ID,
		OperationTypeUpdate,
		EntityTypeFixtureInstance,
		fixture.ID,
		"Updated fixture name",
		prevSnapshot,
		newSnapshot,
		nil,
	)
	if err != nil {
		t.Fatalf("RecordOperation failed: %v", err)
	}

	// Undo should restore original name
	_, err = service.Undo(ctx, project.ID)
	if err != nil {
		t.Fatalf("Undo failed: %v", err)
	}

	var foundFixture models.FixtureInstance
	db.First(&foundFixture, "id = ?", fixture.ID)
	if foundFixture.Name != originalName {
		t.Errorf("Expected name %q after undo, got %q", originalName, foundFixture.Name)
	}

	// Redo should restore new name
	_, err = service.Redo(ctx, project.ID)
	if err != nil {
		t.Fatalf("Redo failed: %v", err)
	}

	db.First(&foundFixture, "id = ?", fixture.ID)
	if foundFixture.Name != newName {
		t.Errorf("Expected name %q after redo, got %q", newName, foundFixture.Name)
	}
}

func TestService_UndoRedo_CueUpdate(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	service := createTestService(t, db)
	ctx := context.Background()
	project := createTestProject(t, db)

	// Create a look and cue list for the cue
	look := createTestLook(t, db, project.ID)
	cueList := createTestCueList(t, db, project.ID)
	cue := createTestCue(t, db, cueList.ID, look.ID)

	originalName := cue.Name

	// Make a copy for the previous state
	cueCopy := *cue
	prevSnapshot := CueSnapshot{
		Cue: &cueCopy,
	}

	// Update the cue
	newName := "Updated Cue Name"
	cue.Name = newName
	db.Save(cue)

	// Capture new state
	cueUpdated := *cue
	newSnapshot := CueSnapshot{
		Cue: &cueUpdated,
	}

	// Record the update
	err := service.RecordOperation(
		ctx,
		project.ID,
		OperationTypeUpdate,
		EntityTypeCue,
		cue.ID,
		"Updated cue name",
		prevSnapshot,
		newSnapshot,
		nil,
	)
	if err != nil {
		t.Fatalf("RecordOperation failed: %v", err)
	}

	// Undo should restore original name
	_, err = service.Undo(ctx, project.ID)
	if err != nil {
		t.Fatalf("Undo failed: %v", err)
	}

	var foundCue models.Cue
	db.First(&foundCue, "id = ?", cue.ID)
	if foundCue.Name != originalName {
		t.Errorf("Expected name %q after undo, got %q", originalName, foundCue.Name)
	}
}

func TestService_UndoRedo_CueListUpdate(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	service := createTestService(t, db)
	ctx := context.Background()
	project := createTestProject(t, db)

	// Create a cue list
	cueList := createTestCueList(t, db, project.ID)
	originalName := cueList.Name

	// Make a copy for the previous state
	cueListCopy := *cueList
	prevSnapshot := CueListSnapshot{
		CueList: &cueListCopy,
	}

	// Update the cue list
	newName := "Updated CueList Name"
	cueList.Name = newName
	db.Save(cueList)

	// Capture new state
	cueListUpdated := *cueList
	newSnapshot := CueListSnapshot{
		CueList: &cueListUpdated,
	}

	// Record the update
	err := service.RecordOperation(
		ctx,
		project.ID,
		OperationTypeUpdate,
		EntityTypeCueList,
		cueList.ID,
		"Updated cue list name",
		prevSnapshot,
		newSnapshot,
		nil,
	)
	if err != nil {
		t.Fatalf("RecordOperation failed: %v", err)
	}

	// Undo should restore original name
	_, err = service.Undo(ctx, project.ID)
	if err != nil {
		t.Fatalf("Undo failed: %v", err)
	}

	var foundCueList models.CueList
	db.First(&foundCueList, "id = ?", cueList.ID)
	if foundCueList.Name != originalName {
		t.Errorf("Expected name %q after undo, got %q", originalName, foundCueList.Name)
	}
}

func TestService_ApplyLookSnapshot_NilLook(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	service := createTestService(t, db)
	ctx := context.Background()
	project := createTestProject(t, db)

	opRepo := repositories.NewOperationRepository(db)

	// Create a look that we'll "delete" via snapshot with nil Look
	look := createTestLook(t, db, project.ID)

	// Create an operation with a snapshot that has a nil Look field
	// This simulates an UPDATE operation where the new state has nil Look
	snapshotWithNilLook := `{"Look": null}`
	op := &models.Operation{
		ProjectID:     project.ID,
		OperationType: "UPDATE",
		EntityType:    string(EntityTypeLook),
		EntityID:      look.ID,
		Description:   "Test nil look snapshot",
		NewState:      snapshotWithNilLook,
	}
	_ = opRepo.RecordOperation(ctx, op)

	// Undo should handle the nil Look by deleting the entity
	_, err := service.Undo(ctx, project.ID)
	if err != nil {
		t.Fatalf("Undo failed: %v", err)
	}

	// Look should be deleted
	var foundLook models.Look
	result := db.First(&foundLook, "id = ?", look.ID)
	if result.Error == nil {
		t.Error("Look should be deleted when snapshot has nil Look")
	}
}

func TestService_ApplyFixtureSnapshot_NilFixture(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	service := createTestService(t, db)
	ctx := context.Background()
	project := createTestProject(t, db)

	opRepo := repositories.NewOperationRepository(db)

	// Create a fixture
	fixture := createTestFixtureInstance(t, db, project.ID)

	// Create an operation with a snapshot that has a nil Fixture field
	snapshotWithNilFixture := `{"Fixture": null}`
	op := &models.Operation{
		ProjectID:     project.ID,
		OperationType: "UPDATE",
		EntityType:    string(EntityTypeFixtureInstance),
		EntityID:      fixture.ID,
		Description:   "Test nil fixture snapshot",
		NewState:      snapshotWithNilFixture,
	}
	_ = opRepo.RecordOperation(ctx, op)

	// Undo should handle the nil Fixture by deleting the entity
	_, err := service.Undo(ctx, project.ID)
	if err != nil {
		t.Fatalf("Undo failed: %v", err)
	}

	// Fixture should be deleted
	var foundFixture models.FixtureInstance
	result := db.First(&foundFixture, "id = ?", fixture.ID)
	if result.Error == nil {
		t.Error("Fixture should be deleted when snapshot has nil Fixture")
	}
}

func TestService_ApplyCueSnapshot_NilCue(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	service := createTestService(t, db)
	ctx := context.Background()
	project := createTestProject(t, db)

	opRepo := repositories.NewOperationRepository(db)

	// Create a cue
	look := createTestLook(t, db, project.ID)
	cueList := createTestCueList(t, db, project.ID)
	cue := createTestCue(t, db, cueList.ID, look.ID)

	// Create an operation with a snapshot that has a nil Cue field
	snapshotWithNilCue := `{"Cue": null}`
	op := &models.Operation{
		ProjectID:     project.ID,
		OperationType: "UPDATE",
		EntityType:    string(EntityTypeCue),
		EntityID:      cue.ID,
		Description:   "Test nil cue snapshot",
		NewState:      snapshotWithNilCue,
	}
	_ = opRepo.RecordOperation(ctx, op)

	// Undo should handle the nil Cue by deleting the entity
	_, err := service.Undo(ctx, project.ID)
	if err != nil {
		t.Fatalf("Undo failed: %v", err)
	}

	// Cue should be deleted
	var foundCue models.Cue
	result := db.First(&foundCue, "id = ?", cue.ID)
	if result.Error == nil {
		t.Error("Cue should be deleted when snapshot has nil Cue")
	}
}

func TestService_ApplyCueListSnapshot_NilCueList(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	service := createTestService(t, db)
	ctx := context.Background()
	project := createTestProject(t, db)

	opRepo := repositories.NewOperationRepository(db)

	// Create a cue list
	cueList := createTestCueList(t, db, project.ID)

	// Create an operation with a snapshot that has a nil CueList field
	snapshotWithNilCueList := `{"CueList": null}`
	op := &models.Operation{
		ProjectID:     project.ID,
		OperationType: "UPDATE",
		EntityType:    string(EntityTypeCueList),
		EntityID:      cueList.ID,
		Description:   "Test nil cue list snapshot",
		NewState:      snapshotWithNilCueList,
	}
	_ = opRepo.RecordOperation(ctx, op)

	// Undo should handle the nil CueList by deleting the entity
	_, err := service.Undo(ctx, project.ID)
	if err != nil {
		t.Fatalf("Undo failed: %v", err)
	}

	// CueList should be deleted
	var foundCueList models.CueList
	result := db.First(&foundCueList, "id = ?", cueList.ID)
	if result.Error == nil {
		t.Error("CueList should be deleted when snapshot has nil CueList")
	}
}

func TestService_ApplyLookBoardSnapshot_NilLookBoard(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	service := createTestService(t, db)
	ctx := context.Background()
	project := createTestProject(t, db)

	opRepo := repositories.NewOperationRepository(db)

	// Create a look board
	lookBoard := createTestLookBoard(t, db, project.ID)

	// Create an operation with a snapshot that has a nil LookBoard field
	snapshotWithNilLookBoard := `{"LookBoard": null}`
	op := &models.Operation{
		ProjectID:     project.ID,
		OperationType: "UPDATE",
		EntityType:    string(EntityTypeLookBoard),
		EntityID:      lookBoard.ID,
		Description:   "Test nil look board snapshot",
		NewState:      snapshotWithNilLookBoard,
	}
	_ = opRepo.RecordOperation(ctx, op)

	// Undo should handle the nil LookBoard by deleting the entity
	_, err := service.Undo(ctx, project.ID)
	if err != nil {
		t.Fatalf("Undo failed: %v", err)
	}

	// LookBoard should be deleted
	var foundLookBoard models.LookBoard
	result := db.First(&foundLookBoard, "id = ?", lookBoard.ID)
	if result.Error == nil {
		t.Error("LookBoard should be deleted when snapshot has nil LookBoard")
	}
}

func TestService_UndoRedo_FixtureInstanceCreate(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	service := createTestService(t, db)
	ctx := context.Background()
	project := createTestProject(t, db)

	// Create a fixture
	fixture := createTestFixtureInstance(t, db, project.ID)

	// Capture state
	newSnapshot := FixtureInstanceSnapshot{
		Fixture: fixture,
	}

	// Record the creation
	err := service.RecordOperation(
		ctx,
		project.ID,
		OperationTypeCreate,
		EntityTypeFixtureInstance,
		fixture.ID,
		"Created fixture",
		nil,
		newSnapshot,
		nil,
	)
	if err != nil {
		t.Fatalf("RecordOperation failed: %v", err)
	}

	// Undo should delete the fixture
	_, err = service.Undo(ctx, project.ID)
	if err != nil {
		t.Fatalf("Undo failed: %v", err)
	}

	var foundFixture models.FixtureInstance
	result := db.First(&foundFixture, "id = ?", fixture.ID)
	if result.Error == nil {
		t.Error("Fixture should be deleted after undo of create")
	}

	// Redo should recreate the fixture
	_, err = service.Redo(ctx, project.ID)
	if err != nil {
		t.Fatalf("Redo failed: %v", err)
	}

	result = db.First(&foundFixture, "id = ?", fixture.ID)
	if result.Error != nil {
		t.Errorf("Fixture should exist after redo: %v", result.Error)
	}
}

func TestService_UndoRedo_CueCreate(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	service := createTestService(t, db)
	ctx := context.Background()
	project := createTestProject(t, db)

	// Create a look and cue list for the cue
	look := createTestLook(t, db, project.ID)
	cueList := createTestCueList(t, db, project.ID)
	cue := createTestCue(t, db, cueList.ID, look.ID)

	// Capture state
	newSnapshot := CueSnapshot{
		Cue: cue,
	}

	// Record the creation
	err := service.RecordOperation(
		ctx,
		project.ID,
		OperationTypeCreate,
		EntityTypeCue,
		cue.ID,
		"Created cue",
		nil,
		newSnapshot,
		nil,
	)
	if err != nil {
		t.Fatalf("RecordOperation failed: %v", err)
	}

	// Undo should delete the cue
	_, err = service.Undo(ctx, project.ID)
	if err != nil {
		t.Fatalf("Undo failed: %v", err)
	}

	var foundCue models.Cue
	result := db.First(&foundCue, "id = ?", cue.ID)
	if result.Error == nil {
		t.Error("Cue should be deleted after undo of create")
	}

	// Redo should recreate the cue
	_, err = service.Redo(ctx, project.ID)
	if err != nil {
		t.Fatalf("Redo failed: %v", err)
	}

	result = db.First(&foundCue, "id = ?", cue.ID)
	if result.Error != nil {
		t.Errorf("Cue should exist after redo: %v", result.Error)
	}
}

func TestService_UndoRedo_CueListCreate(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	service := createTestService(t, db)
	ctx := context.Background()
	project := createTestProject(t, db)

	// Create a cue list
	cueList := createTestCueList(t, db, project.ID)

	// Capture state
	newSnapshot := CueListSnapshot{
		CueList: cueList,
	}

	// Record the creation
	err := service.RecordOperation(
		ctx,
		project.ID,
		OperationTypeCreate,
		EntityTypeCueList,
		cueList.ID,
		"Created cue list",
		nil,
		newSnapshot,
		nil,
	)
	if err != nil {
		t.Fatalf("RecordOperation failed: %v", err)
	}

	// Undo should delete the cue list
	_, err = service.Undo(ctx, project.ID)
	if err != nil {
		t.Fatalf("Undo failed: %v", err)
	}

	var foundCueList models.CueList
	result := db.First(&foundCueList, "id = ?", cueList.ID)
	if result.Error == nil {
		t.Error("CueList should be deleted after undo of create")
	}

	// Redo should recreate the cue list
	_, err = service.Redo(ctx, project.ID)
	if err != nil {
		t.Fatalf("Redo failed: %v", err)
	}

	result = db.First(&foundCueList, "id = ?", cueList.ID)
	if result.Error != nil {
		t.Errorf("CueList should exist after redo: %v", result.Error)
	}
}

func TestService_UndoRedo_LookBoardCreate(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	service := createTestService(t, db)
	ctx := context.Background()
	project := createTestProject(t, db)

	// Create a look board
	lookBoard := createTestLookBoard(t, db, project.ID)

	// Capture state
	newSnapshot := LookBoardSnapshot{
		LookBoard: lookBoard,
	}

	// Record the creation
	err := service.RecordOperation(
		ctx,
		project.ID,
		OperationTypeCreate,
		EntityTypeLookBoard,
		lookBoard.ID,
		"Created look board",
		nil,
		newSnapshot,
		nil,
	)
	if err != nil {
		t.Fatalf("RecordOperation failed: %v", err)
	}

	// Undo should delete the look board
	_, err = service.Undo(ctx, project.ID)
	if err != nil {
		t.Fatalf("Undo failed: %v", err)
	}

	var foundLookBoard models.LookBoard
	result := db.First(&foundLookBoard, "id = ?", lookBoard.ID)
	if result.Error == nil {
		t.Error("LookBoard should be deleted after undo of create")
	}

	// Redo should recreate the look board
	_, err = service.Redo(ctx, project.ID)
	if err != nil {
		t.Fatalf("Redo failed: %v", err)
	}

	result = db.First(&foundLookBoard, "id = ?", lookBoard.ID)
	if result.Error != nil {
		t.Errorf("LookBoard should exist after redo: %v", result.Error)
	}
}

// TestService_MultipleUndoRedo_Cues tests undo/redo through multiple cue operations.
func TestService_MultipleUndoRedo_Cues(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	service := createTestService(t, db)
	ctx := context.Background()
	project := createTestProject(t, db)
	look := createTestLook(t, db, project.ID)

	// Create a cue list
	cueList := createTestCueList(t, db, project.ID)

	// Create 3 cues with different numbers
	cue1 := createTestCue(t, db, cueList.ID, look.ID)
	cue1.CueNumber = 1.0
	cue1.Name = "Opening"
	db.Save(cue1)

	cue2 := createTestCue(t, db, cueList.ID, look.ID)
	cue2.CueNumber = 2.0
	cue2.Name = "Scene 1"
	db.Save(cue2)

	cue3 := createTestCue(t, db, cueList.ID, look.ID)
	cue3.CueNumber = 3.0
	cue3.Name = "Scene 2"
	db.Save(cue3)

	// Record 3 create operations
	for _, cue := range []*models.Cue{cue1, cue2, cue3} {
		newSnapshot := CueSnapshot{Cue: cue}
		err := service.RecordOperation(
			ctx,
			project.ID,
			OperationTypeCreate,
			EntityTypeCue,
			cue.ID,
			fmt.Sprintf("Create cue '%s' (#%.1f)", cue.Name, cue.CueNumber),
			nil,
			newSnapshot,
			nil,
		)
		if err != nil {
			t.Fatalf("RecordOperation failed for %s: %v", cue.Name, err)
		}
	}

	// Verify status shows 3 operations
	status, err := service.GetStatus(ctx, project.ID)
	if err != nil {
		t.Fatalf("GetStatus failed: %v", err)
	}
	if status.TotalOperations != 3 {
		t.Errorf("Expected 3 operations, got %d", status.TotalOperations)
	}
	if !status.CanUndo {
		t.Error("Should be able to undo")
	}
	if status.CanRedo {
		t.Error("Should not be able to redo before undoing")
	}

	// Undo all 3 operations
	for i := 3; i > 0; i-- {
		result, err := service.Undo(ctx, project.ID)
		if err != nil {
			t.Fatalf("Undo %d failed: %v", i, err)
		}
		if !result.Success {
			t.Errorf("Undo %d should succeed", i)
		}
	}

	// Verify all cues are deleted
	var count int64
	db.Model(&models.Cue{}).Where("cue_list_id = ?", cueList.ID).Count(&count)
	if count != 0 {
		t.Errorf("Expected 0 cues after undo all, got %d", count)
	}

	// Verify can redo but not undo
	status, _ = service.GetStatus(ctx, project.ID)
	if status.CanUndo {
		t.Error("Should not be able to undo after undoing all")
	}
	if !status.CanRedo {
		t.Error("Should be able to redo after undoing all")
	}

	// Redo all 3 operations
	for i := 1; i <= 3; i++ {
		result, err := service.Redo(ctx, project.ID)
		if err != nil {
			t.Fatalf("Redo %d failed: %v", i, err)
		}
		if !result.Success {
			t.Errorf("Redo %d should succeed", i)
		}
	}

	// Verify all cues are back
	db.Model(&models.Cue{}).Where("cue_list_id = ?", cueList.ID).Count(&count)
	if count != 3 {
		t.Errorf("Expected 3 cues after redo all, got %d", count)
	}
}

// TestService_OperationHistory_Descriptions tests that operation history shows descriptive information.
func TestService_OperationHistory_Descriptions(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	service := createTestService(t, db)
	ctx := context.Background()
	project := createTestProject(t, db)
	look := createTestLook(t, db, project.ID)

	// Create a cue list
	cueList := createTestCueList(t, db, project.ID)

	// Create a cue
	cue := createTestCue(t, db, cueList.ID, look.ID)
	cue.Name = "Blackout"
	cue.CueNumber = 5.5
	db.Save(cue)

	// Record the creation with a descriptive message
	newSnapshot := CueSnapshot{Cue: cue}
	err := service.RecordOperation(
		ctx,
		project.ID,
		OperationTypeCreate,
		EntityTypeCue,
		cue.ID,
		"Create cue 'Blackout' (#5.5)",
		nil,
		newSnapshot,
		nil,
	)
	if err != nil {
		t.Fatalf("RecordOperation failed: %v", err)
	}

	// Capture prev state
	prevSnapshot := CueSnapshot{Cue: cue}

	// Update the cue
	cue.Name = "Final Blackout"
	cue.FadeInTime = 3.0
	db.Save(cue)

	newSnapshot = CueSnapshot{Cue: cue}
	err = service.RecordOperation(
		ctx,
		project.ID,
		OperationTypeUpdate,
		EntityTypeCue,
		cue.ID,
		"Update cue 'Final Blackout'",
		prevSnapshot,
		newSnapshot,
		nil,
	)
	if err != nil {
		t.Fatalf("RecordOperation update failed: %v", err)
	}

	// Get operation history
	ops, total, err := service.GetOperationHistory(ctx, project.ID, 1, 10)
	if err != nil {
		t.Fatalf("GetOperationHistory failed: %v", err)
	}

	if total != 2 {
		t.Errorf("Expected 2 operations, got %d", total)
	}

	// Operations are returned in descending sequence order (newest first)
	if len(ops) < 2 {
		t.Fatalf("Expected at least 2 operations, got %d", len(ops))
	}

	// First operation should be update (most recent)
	if ops[0].Description != "Update cue 'Final Blackout'" {
		t.Errorf("Expected update description, got: %s", ops[0].Description)
	}
	if ops[0].OperationType != string(OperationTypeUpdate) {
		t.Errorf("Expected UPDATE operation type, got: %s", ops[0].OperationType)
	}
	if ops[0].EntityType != string(EntityTypeCue) {
		t.Errorf("Expected Cue entity type, got: %s", ops[0].EntityType)
	}

	// Second operation should be create (older)
	if ops[1].Description != "Create cue 'Blackout' (#5.5)" {
		t.Errorf("Expected create description, got: %s", ops[1].Description)
	}
	if ops[1].OperationType != string(OperationTypeCreate) {
		t.Errorf("Expected CREATE operation type, got: %s", ops[1].OperationType)
	}
}

// TestService_UndoRedo_CueListWithReorder tests undo/redo of cue list reordering.
func TestService_UndoRedo_CueListWithReorder(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	service := createTestService(t, db)
	ctx := context.Background()
	project := createTestProject(t, db)
	look := createTestLook(t, db, project.ID)

	// Create a cue list with 3 cues
	cueList := createTestCueList(t, db, project.ID)

	cue1 := createTestCue(t, db, cueList.ID, look.ID)
	cue1.CueNumber = 1.0
	db.Save(cue1)

	cue2 := createTestCue(t, db, cueList.ID, look.ID)
	cue2.CueNumber = 2.0
	db.Save(cue2)

	cue3 := createTestCue(t, db, cueList.ID, look.ID)
	cue3.CueNumber = 3.0
	db.Save(cue3)

	// Capture previous state (with original order)
	prevSnapshot, err := service.CaptureCueListState(ctx, cueList.ID)
	if err != nil {
		t.Fatalf("CaptureCueListState failed: %v", err)
	}

	// Reorder cues: 3, 1, 2 -> new numbers: 1, 2, 3
	cue3.CueNumber = 1.0
	cue1.CueNumber = 2.0
	cue2.CueNumber = 3.0
	db.Save(cue3)
	db.Save(cue1)
	db.Save(cue2)

	// Capture new state (with new order)
	newSnapshot, err := service.CaptureCueListState(ctx, cueList.ID)
	if err != nil {
		t.Fatalf("CaptureCueListState failed: %v", err)
	}

	// Record the reorder operation
	err = service.RecordOperation(
		ctx,
		project.ID,
		OperationTypeUpdate,
		EntityTypeCueList,
		cueList.ID,
		"Reorder 3 cues in 'Test CueList'",
		prevSnapshot,
		newSnapshot,
		[]string{cue1.ID, cue2.ID, cue3.ID},
	)
	if err != nil {
		t.Fatalf("RecordOperation failed: %v", err)
	}

	// Verify current order: cue3=1, cue1=2, cue2=3
	var foundCue3 models.Cue
	db.First(&foundCue3, "id = ?", cue3.ID)
	if foundCue3.CueNumber != 1.0 {
		t.Errorf("Expected cue3 to be #1, got #%.1f", foundCue3.CueNumber)
	}

	// Undo the reorder
	result, err := service.Undo(ctx, project.ID)
	if err != nil {
		t.Fatalf("Undo failed: %v", err)
	}
	if !result.Success {
		t.Errorf("Undo should succeed: %s", result.Message)
	}

	// Verify original order is restored
	db.First(&foundCue3, "id = ?", cue3.ID)
	if foundCue3.CueNumber != 3.0 {
		t.Errorf("Expected cue3 to be #3 after undo, got #%.1f", foundCue3.CueNumber)
	}

	var foundCue1 models.Cue
	db.First(&foundCue1, "id = ?", cue1.ID)
	if foundCue1.CueNumber != 1.0 {
		t.Errorf("Expected cue1 to be #1 after undo, got #%.1f", foundCue1.CueNumber)
	}

	// Redo the reorder
	result, err = service.Redo(ctx, project.ID)
	if err != nil {
		t.Fatalf("Redo failed: %v", err)
	}
	if !result.Success {
		t.Errorf("Redo should succeed: %s", result.Message)
	}

	// Verify reorder is re-applied
	db.First(&foundCue3, "id = ?", cue3.ID)
	if foundCue3.CueNumber != 1.0 {
		t.Errorf("Expected cue3 to be #1 after redo, got #%.1f", foundCue3.CueNumber)
	}
}

// TestService_UndoRedo_ToggleCueSkip tests undo/redo of cue skip toggle.
func TestService_UndoRedo_ToggleCueSkip(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	service := createTestService(t, db)
	ctx := context.Background()
	project := createTestProject(t, db)
	look := createTestLook(t, db, project.ID)

	// Create a cue list and cue
	cueList := createTestCueList(t, db, project.ID)
	cue := createTestCue(t, db, cueList.ID, look.ID)
	cue.Name = "Transition"
	cue.Skip = false
	db.Save(cue)

	// Capture previous state
	prevSnapshot, _ := service.CaptureCueState(ctx, cue.ID)

	// Toggle skip to true
	cue.Skip = true
	db.Save(cue)

	// Capture new state
	newSnapshot, _ := service.CaptureCueState(ctx, cue.ID)

	// Record the skip toggle
	err := service.RecordOperation(
		ctx,
		project.ID,
		OperationTypeUpdate,
		EntityTypeCue,
		cue.ID,
		"Skip cue 'Transition'",
		prevSnapshot,
		newSnapshot,
		nil,
	)
	if err != nil {
		t.Fatalf("RecordOperation failed: %v", err)
	}

	// Verify skip is true
	var foundCue models.Cue
	db.First(&foundCue, "id = ?", cue.ID)
	if !foundCue.Skip {
		t.Error("Expected cue to be skipped")
	}

	// Undo the skip
	result, err := service.Undo(ctx, project.ID)
	if err != nil {
		t.Fatalf("Undo failed: %v", err)
	}
	if !result.Success {
		t.Errorf("Undo should succeed: %s", result.Message)
	}

	// Verify skip is false
	db.First(&foundCue, "id = ?", cue.ID)
	if foundCue.Skip {
		t.Error("Expected cue to not be skipped after undo")
	}

	// Redo the skip
	_, err = service.Redo(ctx, project.ID)
	if err != nil {
		t.Fatalf("Redo failed: %v", err)
	}

	// Verify skip is true again
	db.First(&foundCue, "id = ?", cue.ID)
	if !foundCue.Skip {
		t.Error("Expected cue to be skipped after redo")
	}
}

// TestService_UndoRedo_CueDelete tests undo/redo of cue deletion.
func TestService_UndoRedo_CueDelete(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	service := createTestService(t, db)
	ctx := context.Background()
	project := createTestProject(t, db)
	look := createTestLook(t, db, project.ID)

	// Create a cue list and cue
	cueList := createTestCueList(t, db, project.ID)
	cue := createTestCue(t, db, cueList.ID, look.ID)
	cue.Name = "Important Cue"
	cue.CueNumber = 7.5
	cue.FadeInTime = 5.0
	cue.FadeOutTime = 2.0
	db.Save(cue)

	cueID := cue.ID

	// Capture state before deletion
	prevSnapshot, _ := service.CaptureCueState(ctx, cue.ID)

	// Delete the cue
	db.Delete(cue)

	// Record the deletion
	err := service.RecordOperation(
		ctx,
		project.ID,
		OperationTypeDelete,
		EntityTypeCue,
		cueID,
		"Delete cue 'Important Cue'",
		prevSnapshot,
		nil,
		nil,
	)
	if err != nil {
		t.Fatalf("RecordOperation failed: %v", err)
	}

	// Verify cue is deleted
	var foundCue models.Cue
	result := db.First(&foundCue, "id = ?", cueID)
	if result.Error == nil {
		t.Error("Cue should be deleted")
	}

	// Undo the deletion
	undoResult, err := service.Undo(ctx, project.ID)
	if err != nil {
		t.Fatalf("Undo failed: %v", err)
	}
	if !undoResult.Success {
		t.Errorf("Undo should succeed: %s", undoResult.Message)
	}

	// Verify cue is restored with all properties
	result = db.First(&foundCue, "id = ?", cueID)
	if result.Error != nil {
		t.Fatalf("Cue should exist after undo: %v", result.Error)
	}
	if foundCue.Name != "Important Cue" {
		t.Errorf("Expected name 'Important Cue', got '%s'", foundCue.Name)
	}
	if foundCue.CueNumber != 7.5 {
		t.Errorf("Expected cue number 7.5, got %.1f", foundCue.CueNumber)
	}
	if foundCue.FadeInTime != 5.0 {
		t.Errorf("Expected fade in time 5.0, got %.1f", foundCue.FadeInTime)
	}

	// Redo the deletion
	redoResult, err := service.Redo(ctx, project.ID)
	if err != nil {
		t.Fatalf("Redo failed: %v", err)
	}
	if !redoResult.Success {
		t.Errorf("Redo should succeed: %s", redoResult.Message)
	}

	// Verify cue is deleted again
	result = db.First(&foundCue, "id = ?", cueID)
	if result.Error == nil {
		t.Error("Cue should be deleted after redo")
	}
}

// =============================================================================
// Edge Case and Error Path Tests
// =============================================================================

// TestService_ApplyLookSnapshot_InvalidJSON tests error handling for invalid JSON.
func TestService_ApplyLookSnapshot_InvalidJSON(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	service := createTestService(t, db)
	ctx := context.Background()

	// Call applyLookSnapshot with invalid JSON
	_, err := service.applyLookSnapshot(ctx, "not valid json", OperationTypeUpdate, "some-id")
	if err == nil {
		t.Error("Expected error for invalid JSON, got nil")
	}
	if err != nil && !contains(err.Error(), "failed to unmarshal look snapshot") {
		t.Errorf("Expected unmarshal error, got: %v", err)
	}
}

// TestService_ApplyFixtureSnapshot_InvalidJSON tests error handling for invalid JSON.
func TestService_ApplyFixtureSnapshot_InvalidJSON(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	service := createTestService(t, db)
	ctx := context.Background()

	// Call applyFixtureSnapshot with invalid JSON
	_, err := service.applyFixtureSnapshot(ctx, "not valid json", OperationTypeUpdate, "some-id")
	if err == nil {
		t.Error("Expected error for invalid JSON, got nil")
	}
	if err != nil && !contains(err.Error(), "failed to unmarshal fixture snapshot") {
		t.Errorf("Expected unmarshal error, got: %v", err)
	}
}

// TestService_ApplyCueSnapshot_InvalidJSON tests error handling for invalid JSON.
func TestService_ApplyCueSnapshot_InvalidJSON(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	service := createTestService(t, db)
	ctx := context.Background()

	// Call applyCueSnapshot with invalid JSON
	_, err := service.applyCueSnapshot(ctx, "not valid json", OperationTypeUpdate, "some-id")
	if err == nil {
		t.Error("Expected error for invalid JSON, got nil")
	}
	if err != nil && !contains(err.Error(), "failed to unmarshal cue snapshot") {
		t.Errorf("Expected unmarshal error, got: %v", err)
	}
}

// TestService_ApplyCueListSnapshot_InvalidJSON tests error handling for invalid JSON.
func TestService_ApplyCueListSnapshot_InvalidJSON(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	service := createTestService(t, db)
	ctx := context.Background()

	// Call applyCueListSnapshot with invalid JSON
	_, err := service.applyCueListSnapshot(ctx, "not valid json", OperationTypeUpdate, "some-id")
	if err == nil {
		t.Error("Expected error for invalid JSON, got nil")
	}
	if err != nil && !contains(err.Error(), "failed to unmarshal cue list snapshot") {
		t.Errorf("Expected unmarshal error, got: %v", err)
	}
}

// TestService_ApplyLookBoardSnapshot_InvalidJSON tests error handling for invalid JSON.
func TestService_ApplyLookBoardSnapshot_InvalidJSON(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	service := createTestService(t, db)
	ctx := context.Background()

	// Call applyLookBoardSnapshot with invalid JSON
	_, err := service.applyLookBoardSnapshot(ctx, "not valid json", OperationTypeUpdate, "some-id")
	if err == nil {
		t.Error("Expected error for invalid JSON, got nil")
	}
	if err != nil && !contains(err.Error(), "failed to unmarshal look board snapshot") {
		t.Errorf("Expected unmarshal error, got: %v", err)
	}
}

// TestService_ApplyCuePlaybackSnapshot_NoController tests error handling when playback controller is nil.
func TestService_ApplyCuePlaybackSnapshot_NoController(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	service := createTestService(t, db)
	ctx := context.Background()

	// Call applyCuePlaybackSnapshot without a playback controller set
	_, err := service.applyCuePlaybackSnapshot(ctx, `{"cueListId":"test","isPlaying":true}`)
	if err == nil {
		t.Error("Expected error when playback controller not set, got nil")
	}
	if err != nil && !contains(err.Error(), "playback controller not set") {
		t.Errorf("Expected 'playback controller not set' error, got: %v", err)
	}
}

// TestService_ApplyLookSnapshot_EmptySnapshotWithEntityID tests deleting when undoing CREATE.
func TestService_ApplyLookSnapshot_EmptySnapshotWithEntityID(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	service := createTestService(t, db)
	ctx := context.Background()
	project := createTestProject(t, db)
	look := createTestLook(t, db, project.ID)

	// Apply empty snapshot (should delete the look)
	resultID, err := service.applyLookSnapshot(ctx, "", OperationTypeCreate, look.ID)
	if err != nil {
		t.Fatalf("applyLookSnapshot with empty snapshot failed: %v", err)
	}
	if resultID != look.ID {
		t.Errorf("Expected result ID %s, got %s", look.ID, resultID)
	}

	// Verify look is deleted
	var count int64
	db.Model(&models.Look{}).Where("id = ?", look.ID).Count(&count)
	if count != 0 {
		t.Error("Look should be deleted after applying empty snapshot")
	}
}

// TestService_ApplyFixtureSnapshot_EmptySnapshotWithEntityID tests deleting when undoing CREATE.
func TestService_ApplyFixtureSnapshot_EmptySnapshotWithEntityID(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	service := createTestService(t, db)
	ctx := context.Background()
	project := createTestProject(t, db)
	fixture := createTestFixtureInstance(t, db, project.ID)

	// Apply empty snapshot (should delete the fixture)
	resultID, err := service.applyFixtureSnapshot(ctx, "", OperationTypeCreate, fixture.ID)
	if err != nil {
		t.Fatalf("applyFixtureSnapshot with empty snapshot failed: %v", err)
	}
	if resultID != fixture.ID {
		t.Errorf("Expected result ID %s, got %s", fixture.ID, resultID)
	}

	// Verify fixture is deleted
	var count int64
	db.Model(&models.FixtureInstance{}).Where("id = ?", fixture.ID).Count(&count)
	if count != 0 {
		t.Error("Fixture should be deleted after applying empty snapshot")
	}
}

// TestService_ApplyCueSnapshot_EmptySnapshotWithEntityID tests deleting when undoing CREATE.
func TestService_ApplyCueSnapshot_EmptySnapshotWithEntityID(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	service := createTestService(t, db)
	ctx := context.Background()
	project := createTestProject(t, db)
	look := createTestLook(t, db, project.ID)
	cueList := createTestCueList(t, db, project.ID)
	cue := createTestCue(t, db, cueList.ID, look.ID)

	// Apply empty snapshot (should delete the cue)
	resultID, err := service.applyCueSnapshot(ctx, "", OperationTypeCreate, cue.ID)
	if err != nil {
		t.Fatalf("applyCueSnapshot with empty snapshot failed: %v", err)
	}
	if resultID != cue.ID {
		t.Errorf("Expected result ID %s, got %s", cue.ID, resultID)
	}

	// Verify cue is deleted
	var count int64
	db.Model(&models.Cue{}).Where("id = ?", cue.ID).Count(&count)
	if count != 0 {
		t.Error("Cue should be deleted after applying empty snapshot")
	}
}

// TestService_ApplyCueListSnapshot_EmptySnapshotWithEntityID tests deleting when undoing CREATE.
func TestService_ApplyCueListSnapshot_EmptySnapshotWithEntityID(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	service := createTestService(t, db)
	ctx := context.Background()
	project := createTestProject(t, db)
	cueList := createTestCueList(t, db, project.ID)

	// Apply empty snapshot (should delete the cue list)
	resultID, err := service.applyCueListSnapshot(ctx, "", OperationTypeCreate, cueList.ID)
	if err != nil {
		t.Fatalf("applyCueListSnapshot with empty snapshot failed: %v", err)
	}
	if resultID != cueList.ID {
		t.Errorf("Expected result ID %s, got %s", cueList.ID, resultID)
	}

	// Verify cue list is deleted
	var count int64
	db.Model(&models.CueList{}).Where("id = ?", cueList.ID).Count(&count)
	if count != 0 {
		t.Error("CueList should be deleted after applying empty snapshot")
	}
}

// TestService_ApplyLookBoardSnapshot_EmptySnapshotWithEntityID tests deleting when undoing CREATE.
func TestService_ApplyLookBoardSnapshot_EmptySnapshotWithEntityID(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	service := createTestService(t, db)
	ctx := context.Background()
	project := createTestProject(t, db)

	// Create a look board
	lookBoard := &models.LookBoard{
		ID:        cuid.New(),
		ProjectID: project.ID,
		Name:      "Test Board",
	}
	if err := db.Create(lookBoard).Error; err != nil {
		t.Fatalf("Failed to create look board: %v", err)
	}

	// Apply empty snapshot (should delete the look board)
	resultID, err := service.applyLookBoardSnapshot(ctx, "", OperationTypeCreate, lookBoard.ID)
	if err != nil {
		t.Fatalf("applyLookBoardSnapshot with empty snapshot failed: %v", err)
	}
	if resultID != lookBoard.ID {
		t.Errorf("Expected result ID %s, got %s", lookBoard.ID, resultID)
	}

	// Verify look board is deleted
	var count int64
	db.Model(&models.LookBoard{}).Where("id = ?", lookBoard.ID).Count(&count)
	if count != 0 {
		t.Error("LookBoard should be deleted after applying empty snapshot")
	}
}

// TestService_PublishStatusUpdate_NilPubSub tests that nil pubsub doesn't panic.
func TestService_PublishStatusUpdate_NilPubSub(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	// Create service with nil pubsub
	opRepo := repositories.NewOperationRepository(db)
	lookRepo := repositories.NewLookRepository(db)
	fixtureRepo := repositories.NewFixtureRepository(db)
	cueRepo := repositories.NewCueRepository(db)
	cueListRepo := repositories.NewCueListRepository(db)
	lookBoardRepo := repositories.NewLookBoardRepository(db)
	effectRepo := repositories.NewEffectRepository(db)
	projectRepo := repositories.NewProjectRepository(db)

	service := NewService(
		opRepo, lookRepo, fixtureRepo, cueRepo, cueListRepo, lookBoardRepo, effectRepo, projectRepo, nil,
	)

	ctx := context.Background()
	project := createTestProject(t, db)

	// Should not panic with nil pubsub
	service.publishStatusUpdate(ctx, project.ID)
}

// TestService_CaptureLookState_WithFixtureValues tests capturing look state with fixture values.
func TestService_CaptureLookState_WithFixtureValues(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	service := createTestService(t, db)
	ctx := context.Background()
	project := createTestProject(t, db)
	fixture := createTestFixtureInstance(t, db, project.ID)
	look := createTestLook(t, db, project.ID)

	// Add fixture values to the look
	fixtureValue := &models.FixtureValue{
		ID:        cuid.New(),
		LookID:    look.ID,
		FixtureID: fixture.ID,
	}
	if err := db.Create(fixtureValue).Error; err != nil {
		t.Fatalf("Failed to create fixture value: %v", err)
	}

	// Capture state
	snapshot, err := service.CaptureLookState(ctx, look.ID)
	if err != nil {
		t.Fatalf("CaptureLookState failed: %v", err)
	}
	if snapshot == nil {
		t.Fatal("Expected non-nil snapshot")
	}
	if snapshot.Look == nil {
		t.Error("Expected non-nil Look in snapshot")
	}
	if len(snapshot.FixtureValues) != 1 {
		t.Errorf("Expected 1 fixture value, got %d", len(snapshot.FixtureValues))
	}
}

// TestService_CaptureLookState_NotFound tests capturing non-existent look.
func TestService_CaptureLookState_NotFound(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	service := createTestService(t, db)
	ctx := context.Background()

	// Capture state for non-existent look
	snapshot, err := service.CaptureLookState(ctx, "non-existent-id")
	if err != nil {
		t.Fatalf("CaptureLookState should not error for missing look: %v", err)
	}
	if snapshot != nil {
		t.Error("Expected nil snapshot for non-existent look")
	}
}

// TestService_CaptureFixtureState_NotFound tests capturing non-existent fixture.
func TestService_CaptureFixtureState_NotFound(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	service := createTestService(t, db)
	ctx := context.Background()

	// Capture state for non-existent fixture
	snapshot, err := service.CaptureFixtureState(ctx, "non-existent-id")
	if err != nil {
		t.Fatalf("CaptureFixtureState should not error for missing fixture: %v", err)
	}
	if snapshot != nil {
		t.Error("Expected nil snapshot for non-existent fixture")
	}
}

// TestService_CaptureCueState_NotFound tests capturing non-existent cue.
func TestService_CaptureCueState_NotFound(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	service := createTestService(t, db)
	ctx := context.Background()

	// Capture state for non-existent cue
	snapshot, err := service.CaptureCueState(ctx, "non-existent-id")
	if err != nil {
		t.Fatalf("CaptureCueState should not error for missing cue: %v", err)
	}
	if snapshot != nil {
		t.Error("Expected nil snapshot for non-existent cue")
	}
}

// TestService_CaptureCueListState_NotFound tests capturing non-existent cue list.
func TestService_CaptureCueListState_NotFound(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	service := createTestService(t, db)
	ctx := context.Background()

	// Capture state for non-existent cue list
	snapshot, err := service.CaptureCueListState(ctx, "non-existent-id")
	if err != nil {
		t.Fatalf("CaptureCueListState should not error for missing cue list: %v", err)
	}
	if snapshot != nil {
		t.Error("Expected nil snapshot for non-existent cue list")
	}
}

// TestService_CaptureLookBoardState_NotFound tests capturing non-existent look board.
func TestService_CaptureLookBoardState_NotFound(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	service := createTestService(t, db)
	ctx := context.Background()

	// Capture state for non-existent look board
	snapshot, err := service.CaptureLookBoardState(ctx, "non-existent-id")
	if err != nil {
		t.Fatalf("CaptureLookBoardState should not error for missing look board: %v", err)
	}
	if snapshot != nil {
		t.Error("Expected nil snapshot for non-existent look board")
	}
}

// TestService_ApplyLookSnapshot_RecreateDeleted tests recreating a deleted look.
func TestService_ApplyLookSnapshot_RecreateDeleted(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	service := createTestService(t, db)
	ctx := context.Background()
	project := createTestProject(t, db)

	// Create a look and capture its state
	look := createTestLook(t, db, project.ID)
	lookID := look.ID
	snapshot, err := service.CaptureLookState(ctx, lookID)
	if err != nil {
		t.Fatalf("CaptureLookState failed: %v", err)
	}

	// Delete the look
	if err := db.Delete(look).Error; err != nil {
		t.Fatalf("Failed to delete look: %v", err)
	}

	// Verify look is deleted
	var count int64
	db.Model(&models.Look{}).Where("id = ?", lookID).Count(&count)
	if count != 0 {
		t.Fatal("Look should be deleted")
	}

	// Apply snapshot to recreate
	snapshotJSON, _ := json.Marshal(snapshot)
	resultID, err := service.applyLookSnapshot(ctx, string(snapshotJSON), OperationTypeDelete, lookID)
	if err != nil {
		t.Fatalf("applyLookSnapshot to recreate failed: %v", err)
	}
	if resultID != lookID {
		t.Errorf("Expected result ID %s, got %s", lookID, resultID)
	}

	// Verify look is recreated
	db.Model(&models.Look{}).Where("id = ?", lookID).Count(&count)
	if count != 1 {
		t.Error("Look should be recreated")
	}
}

// TestService_ApplyFixtureSnapshot_RecreateDeleted tests recreating a deleted fixture.
func TestService_ApplyFixtureSnapshot_RecreateDeleted(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	service := createTestService(t, db)
	ctx := context.Background()
	project := createTestProject(t, db)

	// Create a fixture and capture its state
	fixture := createTestFixtureInstance(t, db, project.ID)
	fixtureID := fixture.ID
	snapshot, err := service.CaptureFixtureState(ctx, fixtureID)
	if err != nil {
		t.Fatalf("CaptureFixtureState failed: %v", err)
	}

	// Delete the fixture
	if err := db.Delete(fixture).Error; err != nil {
		t.Fatalf("Failed to delete fixture: %v", err)
	}

	// Verify fixture is deleted
	var count int64
	db.Model(&models.FixtureInstance{}).Where("id = ?", fixtureID).Count(&count)
	if count != 0 {
		t.Fatal("Fixture should be deleted")
	}

	// Apply snapshot to recreate
	snapshotJSON, _ := json.Marshal(snapshot)
	resultID, err := service.applyFixtureSnapshot(ctx, string(snapshotJSON), OperationTypeDelete, fixtureID)
	if err != nil {
		t.Fatalf("applyFixtureSnapshot to recreate failed: %v", err)
	}
	if resultID != fixtureID {
		t.Errorf("Expected result ID %s, got %s", fixtureID, resultID)
	}

	// Verify fixture is recreated
	db.Model(&models.FixtureInstance{}).Where("id = ?", fixtureID).Count(&count)
	if count != 1 {
		t.Error("Fixture should be recreated")
	}
}

// TestService_ClearHistory_NoOperations tests clearing history when none exist.
func TestService_ClearHistory_NoOperations(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	service := createTestService(t, db)
	ctx := context.Background()
	project := createTestProject(t, db)

	// Clear history when none exist
	err := service.ClearHistory(ctx, project.ID)
	if err != nil {
		t.Errorf("ClearHistory should not error with no operations: %v", err)
	}
}

// TestService_RecordOperation_NilSnapshots tests recording with nil snapshots.
func TestService_RecordOperation_NilSnapshots(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	service := createTestService(t, db)
	ctx := context.Background()
	project := createTestProject(t, db)

	// Record operation with nil previous snapshot (CREATE)
	err := service.RecordOperation(
		ctx,
		project.ID,
		OperationTypeCreate,
		EntityTypeLook,
		"test-id",
		"Test operation",
		nil,
		LookSnapshot{Look: &models.Look{ID: "test-id", ProjectID: project.ID, Name: "Test"}},
		nil,
	)
	if err != nil {
		t.Errorf("RecordOperation with nil prev snapshot failed: %v", err)
	}

	// Verify operation was recorded
	status, _ := service.GetStatus(ctx, project.ID)
	if status.TotalOperations != 1 {
		t.Errorf("Expected 1 operation, got %d", status.TotalOperations)
	}
}

// TestService_Undo_NoOperations tests undo when no operations exist.
func TestService_Undo_NoOperations(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	service := createTestService(t, db)
	ctx := context.Background()
	project := createTestProject(t, db)

	// Try to undo with no operations
	result, err := service.Undo(ctx, project.ID)
	if err != nil {
		t.Fatalf("Undo should not error with no operations: %v", err)
	}
	if result.Success {
		t.Error("Undo should not succeed with no operations")
	}
	if !contains(result.Message, "Nothing to undo") {
		t.Errorf("Expected 'Nothing to undo' message, got: %s", result.Message)
	}
}

// TestService_Redo_NoOperations tests redo when no operations exist to redo.
func TestService_Redo_NoOperations(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	service := createTestService(t, db)
	ctx := context.Background()
	project := createTestProject(t, db)

	// Try to redo with no operations
	result, err := service.Redo(ctx, project.ID)
	if err != nil {
		t.Fatalf("Redo should not error with no operations: %v", err)
	}
	if result.Success {
		t.Error("Redo should not succeed with no operations to redo")
	}
	if !contains(result.Message, "Nothing to redo") {
		t.Errorf("Expected 'Nothing to redo' message, got: %s", result.Message)
	}
}

// Helper function to check if a string contains a substring.
func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsHelper(s, substr))
}

func containsHelper(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

// createTestEffect creates an effect for testing.
func createTestEffect(t *testing.T, db *gorm.DB, projectID string) *models.Effect {
	t.Helper()

	effect := &models.Effect{
		ID:          cuid.New(),
		Name:        "Test Effect " + cuid.Slug(),
		ProjectID:   projectID,
		EffectType:  "SINE",
		Frequency:   1.0,
		OnCueChange: "maintain",
	}
	if err := db.Create(effect).Error; err != nil {
		t.Fatalf("Failed to create effect: %v", err)
	}
	return effect
}

// createTestEffectFixture creates an effect fixture for testing.
func createTestEffectFixture(t *testing.T, db *gorm.DB, effectID, fixtureID string) *models.EffectFixture {
	t.Helper()

	phaseOffset := 0.0
	amplitudeScale := 1.0
	ef := &models.EffectFixture{
		ID:             cuid.New(),
		EffectID:       effectID,
		FixtureID:      fixtureID,
		PhaseOffset:    &phaseOffset,
		AmplitudeScale: &amplitudeScale,
	}
	if err := db.Create(ef).Error; err != nil {
		t.Fatalf("Failed to create effect fixture: %v", err)
	}
	return ef
}

// createTestEffectChannel creates an effect channel for testing.
func createTestEffectChannel(t *testing.T, db *gorm.DB, effectFixtureID string) *models.EffectChannel {
	t.Helper()

	channelOffset := 0
	channelType := "INTENSITY"
	amplitudeScale := 1.0
	frequencyScale := 1.0
	ec := &models.EffectChannel{
		ID:              cuid.New(),
		EffectFixtureID: effectFixtureID,
		ChannelOffset:   &channelOffset,
		ChannelType:     &channelType,
		AmplitudeScale:  &amplitudeScale,
		FrequencyScale:  &frequencyScale,
	}
	if err := db.Create(ec).Error; err != nil {
		t.Fatalf("Failed to create effect channel: %v", err)
	}
	return ec
}

// createTestCueEffect creates a cue effect for testing.
func createTestCueEffect(t *testing.T, db *gorm.DB, cueID, effectID string) *models.CueEffect {
	t.Helper()

	ce := &models.CueEffect{
		ID:        cuid.New(),
		CueID:     cueID,
		EffectID:  effectID,
		Intensity: 100.0,
		Speed:     1.0,
	}
	if err := db.Create(ce).Error; err != nil {
		t.Fatalf("Failed to create cue effect: %v", err)
	}
	return ce
}

// TestService_CaptureEffectState tests capturing effect state.
func TestService_CaptureEffectState(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	service := createTestService(t, db)
	ctx := context.Background()
	project := createTestProject(t, db)
	effect := createTestEffect(t, db, project.ID)

	// Create fixture instance for the effect
	fixture := createTestFixtureInstance(t, db, project.ID)
	ef := createTestEffectFixture(t, db, effect.ID, fixture.ID)
	_ = createTestEffectChannel(t, db, ef.ID)

	snapshot, err := service.CaptureEffectState(ctx, effect.ID)
	if err != nil {
		t.Fatalf("Failed to capture effect state: %v", err)
	}
	if snapshot == nil {
		t.Fatal("Expected non-nil snapshot")
	}
	if snapshot.Effect == nil {
		t.Fatal("Expected non-nil Effect in snapshot")
	}
	if snapshot.Effect.ID != effect.ID {
		t.Errorf("Expected effect ID %s, got %s", effect.ID, snapshot.Effect.ID)
	}
	if len(snapshot.EffectFixtures) != 1 {
		t.Errorf("Expected 1 effect fixture, got %d", len(snapshot.EffectFixtures))
	}
	if len(snapshot.EffectChannels) != 1 {
		t.Errorf("Expected 1 effect channel, got %d", len(snapshot.EffectChannels))
	}
}

// TestService_CaptureEffectState_NotFound tests capturing state for non-existent effect.
func TestService_CaptureEffectState_NotFound(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	service := createTestService(t, db)
	ctx := context.Background()

	snapshot, err := service.CaptureEffectState(ctx, "non-existent-id")
	if err != nil {
		t.Fatalf("Error should be nil for non-existent effect: %v", err)
	}
	if snapshot != nil {
		t.Error("Expected nil snapshot for non-existent effect")
	}
}

// TestService_UndoRedo_Effect tests undo/redo for effect operations.
func TestService_UndoRedo_Effect(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	service := createTestService(t, db)
	ctx := context.Background()
	project := createTestProject(t, db)

	// Create effect
	effect := createTestEffect(t, db, project.ID)

	// Capture state after creation
	newState, err := service.CaptureEffectState(ctx, effect.ID)
	if err != nil {
		t.Fatalf("Failed to capture effect state: %v", err)
	}

	// Record create operation
	err = service.RecordOperation(ctx, project.ID, OperationTypeCreate, EntityTypeEffect, effect.ID,
		fmt.Sprintf("Create effect '%s'", effect.Name), nil, newState, nil)
	if err != nil {
		t.Fatalf("Failed to record operation: %v", err)
	}

	// Verify effect exists
	var effectCheck models.Effect
	if err := db.First(&effectCheck, "id = ?", effect.ID).Error; err != nil {
		t.Fatalf("Effect should exist: %v", err)
	}

	// Undo - should delete the effect
	result, err := service.Undo(ctx, project.ID)
	if err != nil {
		t.Fatalf("Undo failed: %v", err)
	}
	if !result.Success {
		t.Errorf("Expected undo to succeed: %s", result.Message)
	}

	// Verify effect is deleted
	if err := db.First(&effectCheck, "id = ?", effect.ID).Error; err == nil {
		t.Error("Effect should be deleted after undo")
	}

	// Redo - should recreate the effect
	result, err = service.Redo(ctx, project.ID)
	if err != nil {
		t.Fatalf("Redo failed: %v", err)
	}
	if !result.Success {
		t.Errorf("Expected redo to succeed: %s", result.Message)
	}

	// Verify effect exists again
	if err := db.First(&effectCheck, "id = ?", effect.ID).Error; err != nil {
		t.Fatalf("Effect should exist after redo: %v", err)
	}
}

// TestService_UndoRedo_EffectWithFixtures tests undo/redo for effects with fixtures.
func TestService_UndoRedo_EffectWithFixtures(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	service := createTestService(t, db)
	ctx := context.Background()
	project := createTestProject(t, db)

	// Create effect with fixtures and channels
	effect := createTestEffect(t, db, project.ID)
	fixture := createTestFixtureInstance(t, db, project.ID)
	ef := createTestEffectFixture(t, db, effect.ID, fixture.ID)
	ec := createTestEffectChannel(t, db, ef.ID)

	// Capture state after creation
	newState, err := service.CaptureEffectState(ctx, effect.ID)
	if err != nil {
		t.Fatalf("Failed to capture effect state: %v", err)
	}
	if len(newState.EffectFixtures) != 1 {
		t.Errorf("Expected 1 effect fixture in snapshot, got %d", len(newState.EffectFixtures))
	}
	if len(newState.EffectChannels) != 1 {
		t.Errorf("Expected 1 effect channel in snapshot, got %d", len(newState.EffectChannels))
	}

	// Record create operation
	err = service.RecordOperation(ctx, project.ID, OperationTypeCreate, EntityTypeEffect, effect.ID,
		fmt.Sprintf("Create effect '%s'", effect.Name), nil, newState, nil)
	if err != nil {
		t.Fatalf("Failed to record operation: %v", err)
	}

	// Undo - should delete the effect and its fixtures/channels
	result, err := service.Undo(ctx, project.ID)
	if err != nil {
		t.Fatalf("Undo failed: %v", err)
	}
	if !result.Success {
		t.Errorf("Expected undo to succeed: %s", result.Message)
	}

	// Verify effect fixture and channel are deleted
	var efCheck models.EffectFixture
	if err := db.First(&efCheck, "id = ?", ef.ID).Error; err == nil {
		t.Error("Effect fixture should be deleted after undo")
	}
	var ecCheck models.EffectChannel
	if err := db.First(&ecCheck, "id = ?", ec.ID).Error; err == nil {
		t.Error("Effect channel should be deleted after undo")
	}

	// Redo - should recreate the effect with fixtures/channels
	result, err = service.Redo(ctx, project.ID)
	if err != nil {
		t.Fatalf("Redo failed: %v", err)
	}
	if !result.Success {
		t.Errorf("Expected redo to succeed: %s", result.Message)
	}

	// Verify effect exists with fixtures
	var effectFixtures []models.EffectFixture
	if err := db.Find(&effectFixtures, "effect_id = ?", effect.ID).Error; err != nil {
		t.Fatalf("Failed to query effect fixtures: %v", err)
	}
	if len(effectFixtures) != 1 {
		t.Errorf("Expected 1 effect fixture after redo, got %d", len(effectFixtures))
	}
}

// TestService_CaptureEffectState_WithMultipleFixtures tests capturing complex effect state.
func TestService_CaptureEffectState_WithMultipleFixtures(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	service := createTestService(t, db)
	ctx := context.Background()
	project := createTestProject(t, db)

	// Create effect with multiple fixtures
	effect := createTestEffect(t, db, project.ID)
	fixture1 := createTestFixtureInstance(t, db, project.ID)
	fixture2 := createTestFixtureInstance(t, db, project.ID)
	ef1 := createTestEffectFixture(t, db, effect.ID, fixture1.ID)
	ef2 := createTestEffectFixture(t, db, effect.ID, fixture2.ID)
	createTestEffectChannel(t, db, ef1.ID)
	createTestEffectChannel(t, db, ef2.ID)

	snapshot, err := service.CaptureEffectState(ctx, effect.ID)
	if err != nil {
		t.Fatalf("Failed to capture effect state: %v", err)
	}
	if len(snapshot.EffectFixtures) != 2 {
		t.Errorf("Expected 2 effect fixtures, got %d", len(snapshot.EffectFixtures))
	}
	if len(snapshot.EffectChannels) != 2 {
		t.Errorf("Expected 2 effect channels, got %d", len(snapshot.EffectChannels))
	}
}

// TestService_ApplyEffectSnapshot_InvalidJSON tests applying invalid effect snapshot.
func TestService_ApplyEffectSnapshot_InvalidJSON(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	service := createTestService(t, db)
	ctx := context.Background()

	_, err := service.applyEffectSnapshot(ctx, "invalid json", OperationTypeCreate, "test-id")
	if err == nil {
		t.Error("Expected error for invalid JSON")
	}
}

// TestService_ApplyEffectSnapshot_NilEffect tests applying snapshot with nil effect.
func TestService_ApplyEffectSnapshot_NilEffect(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	service := createTestService(t, db)
	ctx := context.Background()
	project := createTestProject(t, db)
	effect := createTestEffect(t, db, project.ID)

	// Create snapshot with nil effect
	snapshot := &EffectSnapshot{
		Effect:         nil,
		EffectFixtures: nil,
		EffectChannels: nil,
	}
	snapshotJSON, _ := json.Marshal(snapshot)

	// Apply should delete the entity
	id, err := service.applyEffectSnapshot(ctx, string(snapshotJSON), OperationTypeUpdate, effect.ID)
	if err != nil {
		t.Fatalf("applyEffectSnapshot failed: %v", err)
	}
	if id != effect.ID {
		t.Errorf("Expected entity ID %s, got %s", effect.ID, id)
	}

	// Verify effect is deleted
	var effectCheck models.Effect
	if err := db.First(&effectCheck, "id = ?", effect.ID).Error; err == nil {
		t.Error("Effect should be deleted when applying nil effect snapshot")
	}
}

// TestService_CaptureCueEffectState tests capturing cue effect state.
func TestService_CaptureCueEffectState(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	service := createTestService(t, db)
	ctx := context.Background()
	project := createTestProject(t, db)
	look := createTestLook(t, db, project.ID)
	cueList := createTestCueList(t, db, project.ID)
	cue := createTestCue(t, db, cueList.ID, look.ID)
	effect := createTestEffect(t, db, project.ID)
	ce := createTestCueEffect(t, db, cue.ID, effect.ID)

	snapshot, err := service.CaptureCueEffectState(ctx, cue.ID, effect.ID)
	if err != nil {
		t.Fatalf("Failed to capture cue effect state: %v", err)
	}
	if snapshot == nil {
		t.Fatal("Expected non-nil snapshot")
	}
	if snapshot.CueEffect == nil {
		t.Fatal("Expected non-nil CueEffect in snapshot")
	}
	if snapshot.CueEffect.ID != ce.ID {
		t.Errorf("Expected cue effect ID %s, got %s", ce.ID, snapshot.CueEffect.ID)
	}
	if snapshot.CueID != cue.ID {
		t.Errorf("Expected cue ID %s, got %s", cue.ID, snapshot.CueID)
	}
	if snapshot.EffectID != effect.ID {
		t.Errorf("Expected effect ID %s, got %s", effect.ID, snapshot.EffectID)
	}
	if snapshot.ProjectID != project.ID {
		t.Errorf("Expected project ID %s, got %s", project.ID, snapshot.ProjectID)
	}
}

// TestService_CaptureCueEffectState_NotFound tests capturing state for non-existent cue effect.
func TestService_CaptureCueEffectState_NotFound(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	service := createTestService(t, db)
	ctx := context.Background()

	snapshot, err := service.CaptureCueEffectState(ctx, "non-existent-cue", "non-existent-effect")
	if err != nil {
		t.Fatalf("Error should be nil for non-existent cue effect: %v", err)
	}
	if snapshot != nil {
		t.Error("Expected nil snapshot for non-existent cue effect")
	}
}

// TestService_UndoRedo_CueEffect tests undo/redo for cue effect operations.
func TestService_UndoRedo_CueEffect(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	service := createTestService(t, db)
	ctx := context.Background()
	project := createTestProject(t, db)
	look := createTestLook(t, db, project.ID)
	cueList := createTestCueList(t, db, project.ID)
	cue := createTestCue(t, db, cueList.ID, look.ID)
	effect := createTestEffect(t, db, project.ID)

	// Create cue effect
	ce := createTestCueEffect(t, db, cue.ID, effect.ID)
	t.Logf("Created cue effect: ID=%s, CueID=%s, EffectID=%s", ce.ID, cue.ID, effect.ID)

	// Capture state after creation
	newState, err := service.CaptureCueEffectState(ctx, cue.ID, effect.ID)
	if err != nil {
		t.Fatalf("Failed to capture cue effect state: %v", err)
	}
	newStateJSON, _ := json.Marshal(newState)
	t.Logf("NewState JSON: %s", string(newStateJSON))

	// Record create operation
	entityID := fmt.Sprintf("%s:%s", cue.ID, effect.ID)
	t.Logf("EntityID: %s", entityID)
	err = service.RecordOperation(ctx, project.ID, OperationTypeCreate, EntityTypeCueEffect, entityID,
		fmt.Sprintf("Add effect '%s' to cue", effect.Name), nil, newState, nil)
	if err != nil {
		t.Fatalf("Failed to record operation: %v", err)
	}

	// Verify cue effect exists
	var ceCheck models.CueEffect
	if err := db.First(&ceCheck, "id = ?", ce.ID).Error; err != nil {
		t.Fatalf("Cue effect should exist: %v", err)
	}

	// Undo - should delete the cue effect
	t.Log("Calling Undo...")
	result, err := service.Undo(ctx, project.ID)
	if err != nil {
		t.Fatalf("Undo failed: %v", err)
	}
	t.Logf("Undo result: Success=%v, Message=%s", result.Success, result.Message)
	if !result.Success {
		t.Errorf("Expected undo to succeed: %s", result.Message)
	}

	// Verify cue effect is deleted
	if err := db.First(&ceCheck, "cue_id = ? AND effect_id = ?", cue.ID, effect.ID).Error; err == nil {
		t.Error("Cue effect should be deleted after undo")
	} else {
		t.Log("Cue effect deleted successfully")
	}

	// Redo - should recreate the cue effect
	t.Log("Calling Redo...")
	result, err = service.Redo(ctx, project.ID)
	if err != nil {
		t.Fatalf("Redo failed: %v", err)
	}
	t.Logf("Redo result: Success=%v, Message=%s, EntityID=%s", result.Success, result.Message, result.RestoredEntityID)
	if !result.Success {
		t.Errorf("Expected redo to succeed: %s", result.Message)
	}

	// Verify cue effect exists again - use a fresh variable to avoid GORM caching issues
	var recreatedCE models.CueEffect
	if err := db.Where("cue_id = ? AND effect_id = ?", cue.ID, effect.ID).First(&recreatedCE).Error; err != nil {
		t.Fatalf("Cue effect should exist after redo: %v", err)
	}
	// The ID may be different after recreation, but CueID and EffectID should match
	if recreatedCE.CueID != cue.ID || recreatedCE.EffectID != effect.ID {
		t.Errorf("Recreated cue effect has wrong references: got CueID=%s, EffectID=%s; want CueID=%s, EffectID=%s",
			recreatedCE.CueID, recreatedCE.EffectID, cue.ID, effect.ID)
	}
}

// TestService_ApplyCueEffectSnapshot_InvalidJSON tests applying invalid cue effect snapshot.
func TestService_ApplyCueEffectSnapshot_InvalidJSON(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	service := createTestService(t, db)
	ctx := context.Background()

	_, err := service.applyCueEffectSnapshot(ctx, "invalid json", OperationTypeCreate, "test:id")
	if err == nil {
		t.Error("Expected error for invalid JSON")
	}
}

// TestService_ApplyCueEffectSnapshot_EmptySnapshot tests applying empty cue effect snapshot.
func TestService_ApplyCueEffectSnapshot_EmptySnapshot(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	service := createTestService(t, db)
	ctx := context.Background()

	id, err := service.applyCueEffectSnapshot(ctx, "", OperationTypeCreate, "cue1:effect1")
	if err != nil {
		t.Fatalf("Expected no error for empty snapshot: %v", err)
	}
	// With entityID, should return the entityID even for empty snapshot
	if id != "cue1:effect1" {
		t.Errorf("Expected 'cue1:effect1' for empty snapshot with entityID, got %s", id)
	}
}

// TestService_ApplyCueEffectSnapshot_Recreate tests recreating a cue effect via snapshot.
func TestService_ApplyCueEffectSnapshot_Recreate(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	service := createTestService(t, db)
	ctx := context.Background()
	project := createTestProject(t, db)
	look := createTestLook(t, db, project.ID)
	cueList := createTestCueList(t, db, project.ID)
	cue := createTestCue(t, db, cueList.ID, look.ID)
	effect := createTestEffect(t, db, project.ID)

	// Create cue effect
	ce := createTestCueEffect(t, db, cue.ID, effect.ID)
	t.Logf("Created cue effect: ID=%s, CueID=%s, EffectID=%s", ce.ID, ce.CueID, ce.EffectID)

	// Capture state
	snapshot, err := service.CaptureCueEffectState(ctx, cue.ID, effect.ID)
	if err != nil {
		t.Fatalf("Failed to capture state: %v", err)
	}
	t.Logf("Captured snapshot: CueID=%s, EffectID=%s", snapshot.CueID, snapshot.EffectID)

	// Marshal to JSON
	snapshotJSON, err := json.Marshal(snapshot)
	if err != nil {
		t.Fatalf("Failed to marshal snapshot: %v", err)
	}
	t.Logf("Snapshot JSON: %s", string(snapshotJSON))

	// Delete the cue effect directly
	if err := db.Delete(&models.CueEffect{}, "id = ?", ce.ID).Error; err != nil {
		t.Fatalf("Failed to delete cue effect: %v", err)
	}

	// Verify it's deleted
	var check models.CueEffect
	if err := db.First(&check, "cue_id = ? AND effect_id = ?", cue.ID, effect.ID).Error; err == nil {
		t.Fatal("Cue effect should be deleted")
	}

	// Apply snapshot to recreate
	entityID := fmt.Sprintf("%s:%s", cue.ID, effect.ID)
	id, err := service.applyCueEffectSnapshot(ctx, string(snapshotJSON), OperationTypeCreate, entityID)
	if err != nil {
		t.Fatalf("applyCueEffectSnapshot failed: %v", err)
	}
	t.Logf("Result ID: %s", id)

	// Verify it's recreated
	if err := db.First(&check, "cue_id = ? AND effect_id = ?", cue.ID, effect.ID).Error; err != nil {
		t.Fatalf("Cue effect should be recreated: %v", err)
	}
	t.Logf("Recreated cue effect: ID=%s", check.ID)
}

// TestService_DeleteEntityForUndo_Effect tests deleting effect for undo.
func TestService_DeleteEntityForUndo_Effect(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	service := createTestService(t, db)
	ctx := context.Background()
	project := createTestProject(t, db)
	effect := createTestEffect(t, db, project.ID)

	// Delete effect
	err := service.DeleteEntityForUndo(ctx, EntityTypeEffect, effect.ID)
	if err != nil {
		t.Fatalf("DeleteEntityForUndo failed: %v", err)
	}

	// Verify effect is deleted
	var effectCheck models.Effect
	if err := db.First(&effectCheck, "id = ?", effect.ID).Error; err == nil {
		t.Error("Effect should be deleted")
	}
}

// TestService_DeleteEntityForUndo_CueEffect tests that CueEffect delete returns error.
func TestService_DeleteEntityForUndo_CueEffect(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	service := createTestService(t, db)
	ctx := context.Background()

	// CueEffect should return an error since it uses composite key
	err := service.DeleteEntityForUndo(ctx, EntityTypeCueEffect, "test-id")
	if err == nil {
		t.Error("Expected error for CueEffect DeleteEntityForUndo")
	}
}

// TestService_UndoRedo_CueEffectUpdate tests updating an existing cue effect.
func TestService_UndoRedo_CueEffectUpdate(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	service := createTestService(t, db)
	ctx := context.Background()

	// Create project and cue list
	project := &models.Project{ID: cuid.New(), Name: "Test Project"}
	db.Create(project)

	cueList := &models.CueList{ID: cuid.New(), Name: "Test Cue List", ProjectID: project.ID}
	db.Create(cueList)

	// Create a look and a cue
	look := &models.Look{ID: cuid.New(), Name: "Test Look", ProjectID: project.ID}
	db.Create(look)

	cue := &models.Cue{ID: cuid.New(), Name: "Test Cue", CueNumber: 1, CueListID: cueList.ID, LookID: look.ID}
	db.Create(cue)

	// Create an effect
	effect := &models.Effect{
		ID:          cuid.New(),
		Name:        "Test Effect",
		EffectType:  "WAVEFORM",
		ProjectID:   project.ID,
		Frequency:   1.0,
		OnCueChange: "maintain",
	}
	db.Create(effect)

	// Add effect to cue with initial values
	effectRepo := repositories.NewEffectRepository(db)
	err := effectRepo.AddEffectToCue(ctx, cue.ID, effect.ID, 0.5, 0.5)
	if err != nil {
		t.Fatalf("AddEffectToCue failed: %v", err)
	}

	// Capture before state
	beforeState, err := service.CaptureCueEffectState(ctx, cue.ID, effect.ID)
	if err != nil {
		t.Fatalf("CaptureCueEffectState failed: %v", err)
	}

	// Update the cue effect
	cueEffect, err := effectRepo.GetCueEffect(ctx, cue.ID, effect.ID)
	if err != nil {
		t.Fatalf("GetCueEffect failed: %v", err)
	}
	cueEffect.Intensity = 0.8
	cueEffect.Speed = 0.8
	err = effectRepo.UpdateCueEffect(ctx, cueEffect)
	if err != nil {
		t.Fatalf("UpdateCueEffect failed: %v", err)
	}

	// Capture after state
	afterState, err := service.CaptureCueEffectState(ctx, cue.ID, effect.ID)
	if err != nil {
		t.Fatalf("CaptureCueEffectState failed: %v", err)
	}

	// Record the update operation
	entityID := cue.ID + ":" + effect.ID
	err = service.RecordOperation(ctx, project.ID, OperationTypeUpdate, EntityTypeCueEffect, entityID,
		"Update cue effect", beforeState, afterState, nil)
	if err != nil {
		t.Fatalf("RecordOperation failed: %v", err)
	}

	// Undo - should restore original intensity and speed
	_, err = service.Undo(ctx, project.ID)
	if err != nil {
		t.Fatalf("Undo failed: %v", err)
	}

	// Verify cue effect was restored to original values
	restoredCueEffect, err := effectRepo.GetCueEffect(ctx, cue.ID, effect.ID)
	if err != nil {
		t.Fatalf("GetCueEffect after undo failed: %v", err)
	}
	if restoredCueEffect.Intensity != 0.5 {
		t.Errorf("Expected intensity 0.5, got %f", restoredCueEffect.Intensity)
	}
	if restoredCueEffect.Speed != 0.5 {
		t.Errorf("Expected speed 0.5, got %f", restoredCueEffect.Speed)
	}

	// Redo - should restore updated values
	_, err = service.Redo(ctx, project.ID)
	if err != nil {
		t.Fatalf("Redo failed: %v", err)
	}

	// Verify cue effect was restored to updated values
	redoCueEffect, err := effectRepo.GetCueEffect(ctx, cue.ID, effect.ID)
	if err != nil {
		t.Fatalf("GetCueEffect after redo failed: %v", err)
	}
	if redoCueEffect.Intensity != 0.8 {
		t.Errorf("Expected intensity 0.8, got %f", redoCueEffect.Intensity)
	}
	if redoCueEffect.Speed != 0.8 {
		t.Errorf("Expected speed 0.8, got %f", redoCueEffect.Speed)
	}
}

// TestService_UndoRedo_CueEffectWithOnCueChange tests recreating a cue effect with OnCueChange.
func TestService_UndoRedo_CueEffectWithOnCueChange(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	service := createTestService(t, db)
	ctx := context.Background()

	// Create project and cue list
	project := &models.Project{ID: cuid.New(), Name: "Test Project"}
	db.Create(project)

	cueList := &models.CueList{ID: cuid.New(), Name: "Test Cue List", ProjectID: project.ID}
	db.Create(cueList)

	// Create a look and a cue
	look := &models.Look{ID: cuid.New(), Name: "Test Look", ProjectID: project.ID}
	db.Create(look)

	cue := &models.Cue{ID: cuid.New(), Name: "Test Cue", CueNumber: 1, CueListID: cueList.ID, LookID: look.ID}
	db.Create(cue)

	// Create an effect
	effect := &models.Effect{
		ID:          cuid.New(),
		Name:        "Test Effect",
		EffectType:  "WAVEFORM",
		ProjectID:   project.ID,
		Frequency:   1.0,
		OnCueChange: "maintain",
	}
	db.Create(effect)

	// Add effect to cue with OnCueChange
	effectRepo := repositories.NewEffectRepository(db)
	err := effectRepo.AddEffectToCue(ctx, cue.ID, effect.ID, 0.5, 0.5)
	if err != nil {
		t.Fatalf("AddEffectToCue failed: %v", err)
	}

	// Set OnCueChange
	cueEffect, err := effectRepo.GetCueEffect(ctx, cue.ID, effect.ID)
	if err != nil {
		t.Fatalf("GetCueEffect failed: %v", err)
	}
	onCueChange := "start"
	cueEffect.OnCueChange = &onCueChange
	err = effectRepo.UpdateCueEffect(ctx, cueEffect)
	if err != nil {
		t.Fatalf("UpdateCueEffect failed: %v", err)
	}

	// Capture before state (with OnCueChange set)
	beforeState, err := service.CaptureCueEffectState(ctx, cue.ID, effect.ID)
	if err != nil {
		t.Fatalf("CaptureCueEffectState failed: %v", err)
	}

	// Delete the cue effect
	err = effectRepo.RemoveEffectFromCue(ctx, cue.ID, effect.ID)
	if err != nil {
		t.Fatalf("RemoveEffectFromCue failed: %v", err)
	}

	// Record the delete operation
	entityID := cue.ID + ":" + effect.ID
	err = service.RecordOperation(ctx, project.ID, OperationTypeDelete, EntityTypeCueEffect, entityID,
		"Delete cue effect", beforeState, nil, nil)
	if err != nil {
		t.Fatalf("RecordOperation failed: %v", err)
	}

	// Verify cue effect is deleted
	deletedCueEffect, err := effectRepo.GetCueEffect(ctx, cue.ID, effect.ID)
	if err != nil {
		t.Fatalf("GetCueEffect failed: %v", err)
	}
	if deletedCueEffect != nil {
		t.Error("Expected cue effect to be deleted")
	}

	// Undo - should recreate the cue effect with OnCueChange
	_, err = service.Undo(ctx, project.ID)
	if err != nil {
		t.Fatalf("Undo failed: %v", err)
	}

	// Verify cue effect was recreated with OnCueChange
	restoredCueEffect, err := effectRepo.GetCueEffect(ctx, cue.ID, effect.ID)
	if err != nil {
		t.Fatalf("GetCueEffect after undo failed: %v", err)
	}
	if restoredCueEffect == nil {
		t.Fatal("Expected cue effect to be recreated")
	}
	if restoredCueEffect.OnCueChange == nil || *restoredCueEffect.OnCueChange != "start" {
		t.Error("Expected OnCueChange to be 'start' after undo")
	}
}

// TestService_ApplyCueEffectSnapshot_NilSnapshotWithEntityID tests applyCueEffectSnapshot with nil snapshot and entityID.
func TestService_ApplyCueEffectSnapshot_NilSnapshotWithEntityID(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	service := createTestService(t, db)
	ctx := context.Background()

	// Create project and cue list
	project := &models.Project{ID: cuid.New(), Name: "Test Project"}
	db.Create(project)

	cueList := &models.CueList{ID: cuid.New(), Name: "Test Cue List", ProjectID: project.ID}
	db.Create(cueList)

	// Create a look and a cue
	look := &models.Look{ID: cuid.New(), Name: "Test Look", ProjectID: project.ID}
	db.Create(look)

	cue := &models.Cue{ID: cuid.New(), Name: "Test Cue", CueNumber: 1, CueListID: cueList.ID, LookID: look.ID}
	db.Create(cue)

	// Create an effect
	effect := &models.Effect{
		ID:          cuid.New(),
		Name:        "Test Effect",
		EffectType:  "WAVEFORM",
		ProjectID:   project.ID,
		Frequency:   1.0,
		OnCueChange: "maintain",
	}
	db.Create(effect)

	// Add effect to cue
	effectRepo := repositories.NewEffectRepository(db)
	err := effectRepo.AddEffectToCue(ctx, cue.ID, effect.ID, 0.5, 0.5)
	if err != nil {
		t.Fatalf("AddEffectToCue failed: %v", err)
	}

	// Capture state before deletion
	beforeState, err := service.CaptureCueEffectState(ctx, cue.ID, effect.ID)
	if err != nil {
		t.Fatalf("CaptureCueEffectState failed: %v", err)
	}

	// Record a CREATE operation with the captured state as afterState
	entityID := cue.ID + ":" + effect.ID
	err = service.RecordOperation(ctx, project.ID, OperationTypeCreate, EntityTypeCueEffect, entityID,
		"Create cue effect", nil, beforeState, nil)
	if err != nil {
		t.Fatalf("RecordOperation failed: %v", err)
	}

	// Undo the CREATE - this should delete the cue effect
	// This exercises the empty snapshot path with a valid entityID
	_, err = service.Undo(ctx, project.ID)
	if err != nil {
		t.Fatalf("Undo failed: %v", err)
	}

	// Verify cue effect was removed
	removedCueEffect, err := effectRepo.GetCueEffect(ctx, cue.ID, effect.ID)
	if err != nil {
		t.Fatalf("GetCueEffect failed: %v", err)
	}
	if removedCueEffect != nil {
		t.Error("Expected cue effect to be removed after undoing create")
	}
}

// TestService_ApplyCueEffectSnapshot_SnapshotWithNilCueEffect tests the nil CueEffect in snapshot path.
func TestService_ApplyCueEffectSnapshot_SnapshotWithNilCueEffect(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	service := createTestService(t, db)
	ctx := context.Background()

	// Create project and cue list
	project := &models.Project{ID: cuid.New(), Name: "Test Project"}
	db.Create(project)

	cueList := &models.CueList{ID: cuid.New(), Name: "Test Cue List", ProjectID: project.ID}
	db.Create(cueList)

	// Create a look and a cue
	look := &models.Look{ID: cuid.New(), Name: "Test Look", ProjectID: project.ID}
	db.Create(look)

	cue := &models.Cue{ID: cuid.New(), Name: "Test Cue", CueNumber: 1, CueListID: cueList.ID, LookID: look.ID}
	db.Create(cue)

	// Create an effect
	effect := &models.Effect{
		ID:          cuid.New(),
		Name:        "Test Effect",
		EffectType:  "WAVEFORM",
		ProjectID:   project.ID,
		Frequency:   1.0,
		OnCueChange: "maintain",
	}
	db.Create(effect)

	// Add effect to cue
	effectRepo := repositories.NewEffectRepository(db)
	err := effectRepo.AddEffectToCue(ctx, cue.ID, effect.ID, 0.5, 0.5)
	if err != nil {
		t.Fatalf("AddEffectToCue failed: %v", err)
	}

	// Create a snapshot with CueID and EffectID but nil CueEffect
	// This simulates the "snapshot has no cue effect" path
	snapshot := &CueEffectSnapshot{
		CueID:     cue.ID,
		EffectID:  effect.ID,
		CueEffect: nil, // nil CueEffect means remove the relationship
	}

	// Record operation with this special snapshot as beforeState
	entityID := cue.ID + ":" + effect.ID
	afterState, _ := service.CaptureCueEffectState(ctx, cue.ID, effect.ID)
	err = service.RecordOperation(ctx, project.ID, OperationTypeUpdate, EntityTypeCueEffect, entityID,
		"Update cue effect", snapshot, afterState, nil)
	if err != nil {
		t.Fatalf("RecordOperation failed: %v", err)
	}

	// Undo - this will apply the snapshot with nil CueEffect, removing the relationship
	_, err = service.Undo(ctx, project.ID)
	if err != nil {
		t.Fatalf("Undo failed: %v", err)
	}

	// Verify cue effect was removed due to nil CueEffect in snapshot
	removedCueEffect, err := effectRepo.GetCueEffect(ctx, cue.ID, effect.ID)
	if err != nil {
		t.Fatalf("GetCueEffect failed: %v", err)
	}
	if removedCueEffect != nil {
		t.Error("Expected cue effect to be removed when restoring snapshot with nil CueEffect")
	}
}
