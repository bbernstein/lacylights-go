package undo

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/bbernstein/lacylights-go/internal/database/models"
	"github.com/bbernstein/lacylights-go/internal/database/repositories"
	"github.com/bbernstein/lacylights-go/internal/services/pubsub"
	"github.com/glebarez/sqlite"
	"github.com/lucsky/cuid"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

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
