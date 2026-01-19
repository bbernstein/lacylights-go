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
