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

// testDB holds the test database.
type testDB struct {
	DB *gorm.DB
}

// setupTestDB creates an in-memory SQLite database for testing repositories.
func setupTestDB(t *testing.T) (*testDB, func()) {
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
		&models.Scene{},
		&models.FixtureValue{},
		&models.CueList{},
		&models.Cue{},
		&models.Setting{},
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

	return &testDB{DB: db}, cleanup
}

// TestProjectRepository_CRUD tests basic CRUD operations on the ProjectRepository.
func TestProjectRepository_CRUD(t *testing.T) {
	testDB, cleanup := setupTestDB(t)
	defer cleanup()

	repo := NewProjectRepository(testDB.DB)
	ctx := context.Background()

	// Test Create
	project := &models.Project{
		Name: "Test Project " + cuid.Slug(),
	}
	err := repo.Create(ctx, project)
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}
	if project.ID == "" {
		t.Error("Expected project ID to be set after Create")
	}

	// Test FindByID
	found, err := repo.FindByID(ctx, project.ID)
	if err != nil {
		t.Fatalf("FindByID failed: %v", err)
	}
	if found == nil {
		t.Fatal("Expected to find project")
	}
	if found.Name != project.Name {
		t.Errorf("Name mismatch: got %s, want %s", found.Name, project.Name)
	}

	// Test FindAll
	projects, err := repo.FindAll(ctx)
	if err != nil {
		t.Fatalf("FindAll failed: %v", err)
	}
	if len(projects) == 0 {
		t.Error("Expected at least one project")
	}

	// Test Update
	project.Name = "Updated Project Name"
	err = repo.Update(ctx, project)
	if err != nil {
		t.Fatalf("Update failed: %v", err)
	}
	found, _ = repo.FindByID(ctx, project.ID)
	if found.Name != "Updated Project Name" {
		t.Errorf("Update didn't persist: got %s", found.Name)
	}

	// Test Delete
	err = repo.Delete(ctx, project.ID)
	if err != nil {
		t.Fatalf("Delete failed: %v", err)
	}
	found, err = repo.FindByID(ctx, project.ID)
	if err != nil {
		t.Fatalf("FindByID after delete failed: %v", err)
	}
	if found != nil {
		t.Error("Expected project to be deleted")
	}
}

// TestProjectRepository_FindByID_NotFound tests FindByID with non-existent ID.
func TestProjectRepository_FindByID_NotFound(t *testing.T) {
	testDB, cleanup := setupTestDB(t)
	defer cleanup()

	repo := NewProjectRepository(testDB.DB)
	ctx := context.Background()

	found, err := repo.FindByID(ctx, "nonexistent-id")
	if err != nil {
		t.Fatalf("FindByID failed: %v", err)
	}
	if found != nil {
		t.Error("Expected nil for non-existent project")
	}
}

// TestProjectRepository_Create_WithID tests Create with pre-set ID.
func TestProjectRepository_Create_WithID(t *testing.T) {
	testDB, cleanup := setupTestDB(t)
	defer cleanup()

	repo := NewProjectRepository(testDB.DB)
	ctx := context.Background()

	customID := cuid.New()
	project := &models.Project{
		ID:   customID,
		Name: "Project with custom ID",
	}
	err := repo.Create(ctx, project)
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}
	if project.ID != customID {
		t.Errorf("ID changed: got %s, want %s", project.ID, customID)
	}
}

// TestProjectRepository_CountMethods tests the count methods.
func TestProjectRepository_CountMethods(t *testing.T) {
	testDB, cleanup := setupTestDB(t)
	defer cleanup()

	repo := NewProjectRepository(testDB.DB)
	ctx := context.Background()

	// Create a project
	project := &models.Project{ID: cuid.New(), Name: "Count Test Project"}
	testDB.DB.Create(project)

	// Test CountFixtures (should be 0)
	count, err := repo.CountFixtures(ctx, project.ID)
	if err != nil {
		t.Fatalf("CountFixtures failed: %v", err)
	}
	if count != 0 {
		t.Errorf("Expected 0 fixtures, got %d", count)
	}

	// Create a fixture definition and fixture
	fixtureDef := &models.FixtureDefinition{
		ID:           cuid.New(),
		Manufacturer: "Test",
		Model:        "Test Model",
		Type:         "test",
	}
	testDB.DB.Create(fixtureDef)

	fixture := &models.FixtureInstance{
		ID:           cuid.New(),
		Name:         "Test Fixture",
		ProjectID:    project.ID,
		DefinitionID: fixtureDef.ID,
		Universe:     1,
		StartChannel: 1,
	}
	testDB.DB.Create(fixture)

	count, err = repo.CountFixtures(ctx, project.ID)
	if err != nil {
		t.Fatalf("CountFixtures failed: %v", err)
	}
	if count != 1 {
		t.Errorf("Expected 1 fixture, got %d", count)
	}

	// Test CountScenes (should be 0)
	count, err = repo.CountScenes(ctx, project.ID)
	if err != nil {
		t.Fatalf("CountScenes failed: %v", err)
	}
	if count != 0 {
		t.Errorf("Expected 0 scenes, got %d", count)
	}

	// Create a scene
	scene := &models.Scene{
		ID:        cuid.New(),
		Name:      "Test Scene",
		ProjectID: project.ID,
	}
	testDB.DB.Create(scene)

	count, err = repo.CountScenes(ctx, project.ID)
	if err != nil {
		t.Fatalf("CountScenes failed: %v", err)
	}
	if count != 1 {
		t.Errorf("Expected 1 scene, got %d", count)
	}

	// Test CountCueLists (should be 0)
	count, err = repo.CountCueLists(ctx, project.ID)
	if err != nil {
		t.Fatalf("CountCueLists failed: %v", err)
	}
	if count != 0 {
		t.Errorf("Expected 0 cue lists, got %d", count)
	}

	// Create a cue list
	cueList := &models.CueList{
		ID:        cuid.New(),
		Name:      "Test Cue List",
		ProjectID: project.ID,
	}
	testDB.DB.Create(cueList)

	count, err = repo.CountCueLists(ctx, project.ID)
	if err != nil {
		t.Fatalf("CountCueLists failed: %v", err)
	}
	if count != 1 {
		t.Errorf("Expected 1 cue list, got %d", count)
	}
}

// TestSettingRepository_CRUD tests basic CRUD operations on the SettingRepository.
func TestSettingRepository_CRUD(t *testing.T) {
	testDB, cleanup := setupTestDB(t)
	defer cleanup()

	repo := NewSettingRepository(testDB.DB)
	ctx := context.Background()

	testKey := "test_key_" + cuid.Slug()

	// Test FindByKey (not found)
	found, err := repo.FindByKey(ctx, testKey)
	if err != nil {
		t.Fatalf("FindByKey failed: %v", err)
	}
	if found != nil {
		t.Error("Expected nil for non-existent setting")
	}

	// Test Upsert (create)
	setting, err := repo.Upsert(ctx, testKey, "test_value")
	if err != nil {
		t.Fatalf("Upsert (create) failed: %v", err)
	}
	if setting.ID == "" {
		t.Error("Expected setting ID to be set")
	}
	if setting.Key != testKey {
		t.Errorf("Key mismatch: got %s, want %s", setting.Key, testKey)
	}
	if setting.Value != "test_value" {
		t.Errorf("Value mismatch: got %s, want test_value", setting.Value)
	}

	// Test Upsert (update)
	updated, err := repo.Upsert(ctx, testKey, "updated_value")
	if err != nil {
		t.Fatalf("Upsert (update) failed: %v", err)
	}
	if updated.ID != setting.ID {
		t.Error("Expected same ID after update")
	}
	if updated.Value != "updated_value" {
		t.Errorf("Value mismatch after update: got %s", updated.Value)
	}

	// Test FindByKey (found)
	found, err = repo.FindByKey(ctx, testKey)
	if err != nil {
		t.Fatalf("FindByKey failed: %v", err)
	}
	if found == nil {
		t.Fatal("Expected to find setting")
	}
	if found.Value != "updated_value" {
		t.Errorf("Value mismatch: got %s", found.Value)
	}

	// Test FindAll
	settings, err := repo.FindAll(ctx)
	if err != nil {
		t.Fatalf("FindAll failed: %v", err)
	}
	if len(settings) == 0 {
		t.Error("Expected at least one setting")
	}

	// Test Delete
	err = repo.Delete(ctx, testKey)
	if err != nil {
		t.Fatalf("Delete failed: %v", err)
	}
	found, _ = repo.FindByKey(ctx, testKey)
	if found != nil {
		t.Error("Expected setting to be deleted")
	}
}

// TestNewProjectRepository tests the constructor.
func TestNewProjectRepository(t *testing.T) {
	testDB, cleanup := setupTestDB(t)
	defer cleanup()

	repo := NewProjectRepository(testDB.DB)
	if repo == nil {
		t.Fatal("Expected non-nil repository")
	}
	if repo.db != testDB.DB {
		t.Error("Expected db to be set")
	}
}

// TestNewSettingRepository tests the constructor.
func TestNewSettingRepository(t *testing.T) {
	testDB, cleanup := setupTestDB(t)
	defer cleanup()

	repo := NewSettingRepository(testDB.DB)
	if repo == nil {
		t.Fatal("Expected non-nil repository")
	}
	if repo.db != testDB.DB {
		t.Error("Expected db to be set")
	}
}

// TestSceneRepository_CRUD tests basic CRUD operations on the SceneRepository.
func TestSceneRepository_CRUD(t *testing.T) {
	testDB, cleanup := setupTestDB(t)
	defer cleanup()

	repo := NewSceneRepository(testDB.DB)
	ctx := context.Background()

	// Create a project first
	project := &models.Project{ID: cuid.New(), Name: "Test Project"}
	testDB.DB.Create(project)

	// Test Create
	scene := &models.Scene{
		Name:      "Test Scene " + cuid.Slug(),
		ProjectID: project.ID,
	}
	err := repo.Create(ctx, scene)
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}
	if scene.ID == "" {
		t.Error("Expected scene ID to be set after Create")
	}

	// Test FindByID
	found, err := repo.FindByID(ctx, scene.ID)
	if err != nil {
		t.Fatalf("FindByID failed: %v", err)
	}
	if found == nil {
		t.Fatal("Expected to find scene")
	}
	if found.Name != scene.Name {
		t.Errorf("Name mismatch: got %s, want %s", found.Name, scene.Name)
	}

	// Test FindByProjectID
	scenes, err := repo.FindByProjectID(ctx, project.ID)
	if err != nil {
		t.Fatalf("FindByProjectID failed: %v", err)
	}
	if len(scenes) == 0 {
		t.Error("Expected at least one scene")
	}

	// Test Update
	scene.Name = "Updated Scene Name"
	err = repo.Update(ctx, scene)
	if err != nil {
		t.Fatalf("Update failed: %v", err)
	}
	found, _ = repo.FindByID(ctx, scene.ID)
	if found.Name != "Updated Scene Name" {
		t.Errorf("Update didn't persist: got %s", found.Name)
	}

	// Test Delete
	err = repo.Delete(ctx, scene.ID)
	if err != nil {
		t.Fatalf("Delete failed: %v", err)
	}
	found, err = repo.FindByID(ctx, scene.ID)
	if err != nil {
		t.Fatalf("FindByID after delete failed: %v", err)
	}
	if found != nil {
		t.Error("Expected scene to be deleted")
	}
}

// TestSceneRepository_FindByID_NotFound tests FindByID with non-existent ID.
func TestSceneRepository_FindByID_NotFound(t *testing.T) {
	testDB, cleanup := setupTestDB(t)
	defer cleanup()

	repo := NewSceneRepository(testDB.DB)
	ctx := context.Background()

	found, err := repo.FindByID(ctx, "nonexistent-id")
	if err != nil {
		t.Fatalf("FindByID failed: %v", err)
	}
	if found != nil {
		t.Error("Expected nil for non-existent scene")
	}
}

// TestSceneRepository_FixtureValueOperations tests fixture value operations.
func TestSceneRepository_FixtureValueOperations(t *testing.T) {
	testDB, cleanup := setupTestDB(t)
	defer cleanup()

	repo := NewSceneRepository(testDB.DB)
	ctx := context.Background()

	// Create project and scene
	project := &models.Project{ID: cuid.New(), Name: "Test Project"}
	testDB.DB.Create(project)
	scene := &models.Scene{ID: cuid.New(), Name: "Test Scene", ProjectID: project.ID}
	testDB.DB.Create(scene)

	// Create fixture definition and fixture
	fixtureDef := &models.FixtureDefinition{
		ID:           cuid.New(),
		Manufacturer: "Test",
		Model:        "Test Model",
		Type:         "test",
	}
	testDB.DB.Create(fixtureDef)
	fixture := &models.FixtureInstance{
		ID:           cuid.New(),
		Name:         "Test Fixture",
		ProjectID:    project.ID,
		DefinitionID: fixtureDef.ID,
		Universe:     1,
		StartChannel: 1,
	}
	testDB.DB.Create(fixture)

	// Test CountFixtures (should be 0)
	count, err := repo.CountFixtures(ctx, scene.ID)
	if err != nil {
		t.Fatalf("CountFixtures failed: %v", err)
	}
	if count != 0 {
		t.Errorf("Expected 0 fixtures, got %d", count)
	}

	// Test CreateFixtureValue
	fv := &models.FixtureValue{
		SceneID:       scene.ID,
		FixtureID:     fixture.ID,
		ChannelValues: "[255, 128, 64]",
	}
	err = repo.CreateFixtureValue(ctx, fv)
	if err != nil {
		t.Fatalf("CreateFixtureValue failed: %v", err)
	}

	count, _ = repo.CountFixtures(ctx, scene.ID)
	if count != 1 {
		t.Errorf("Expected 1 fixture, got %d", count)
	}

	// Test GetFixtureValues
	values, err := repo.GetFixtureValues(ctx, scene.ID)
	if err != nil {
		t.Fatalf("GetFixtureValues failed: %v", err)
	}
	if len(values) != 1 {
		t.Errorf("Expected 1 value, got %d", len(values))
	}

	// Test GetFixtureValue
	found, err := repo.GetFixtureValue(ctx, scene.ID, fixture.ID)
	if err != nil {
		t.Fatalf("GetFixtureValue failed: %v", err)
	}
	if found == nil {
		t.Fatal("Expected to find fixture value")
	}
	if found.ChannelValues != "[255, 128, 64]" {
		t.Errorf("ChannelValues mismatch: got %s", found.ChannelValues)
	}

	// Test UpdateFixtureValue
	found.ChannelValues = "[0, 0, 0]"
	err = repo.UpdateFixtureValue(ctx, found)
	if err != nil {
		t.Fatalf("UpdateFixtureValue failed: %v", err)
	}
	updated, _ := repo.GetFixtureValue(ctx, scene.ID, fixture.ID)
	if updated.ChannelValues != "[0, 0, 0]" {
		t.Errorf("Update didn't persist: got %s", updated.ChannelValues)
	}

	// Test DeleteFixtureValue
	err = repo.DeleteFixtureValue(ctx, scene.ID, fixture.ID)
	if err != nil {
		t.Fatalf("DeleteFixtureValue failed: %v", err)
	}
	found, _ = repo.GetFixtureValue(ctx, scene.ID, fixture.ID)
	if found != nil {
		t.Error("Expected fixture value to be deleted")
	}
}

// TestSceneRepository_CreateWithFixtureValues tests creating scene with values.
func TestSceneRepository_CreateWithFixtureValues(t *testing.T) {
	testDB, cleanup := setupTestDB(t)
	defer cleanup()

	repo := NewSceneRepository(testDB.DB)
	ctx := context.Background()

	// Create project and fixture
	project := &models.Project{ID: cuid.New(), Name: "Test Project"}
	testDB.DB.Create(project)
	fixtureDef := &models.FixtureDefinition{ID: cuid.New(), Manufacturer: "Test", Model: "M", Type: "t"}
	testDB.DB.Create(fixtureDef)
	fixture := &models.FixtureInstance{
		ID:           cuid.New(),
		Name:         "F1",
		ProjectID:    project.ID,
		DefinitionID: fixtureDef.ID,
		Universe:     1,
		StartChannel: 1,
	}
	testDB.DB.Create(fixture)

	// Create scene with fixture values
	scene := &models.Scene{Name: "Scene with values", ProjectID: project.ID}
	values := []models.FixtureValue{
		{FixtureID: fixture.ID, ChannelValues: "[255]"},
	}

	err := repo.CreateWithFixtureValues(ctx, scene, values)
	if err != nil {
		t.Fatalf("CreateWithFixtureValues failed: %v", err)
	}

	if scene.ID == "" {
		t.Error("Expected scene ID to be set")
	}

	// Verify fixture values were created
	fvs, _ := repo.GetFixtureValues(ctx, scene.ID)
	if len(fvs) != 1 {
		t.Errorf("Expected 1 fixture value, got %d", len(fvs))
	}
}

// TestSceneRepository_CreateFixtureValues tests bulk create.
func TestSceneRepository_CreateFixtureValues(t *testing.T) {
	testDB, cleanup := setupTestDB(t)
	defer cleanup()

	repo := NewSceneRepository(testDB.DB)
	ctx := context.Background()

	// Create project, scene, fixtures
	project := &models.Project{ID: cuid.New(), Name: "Test Project"}
	testDB.DB.Create(project)
	scene := &models.Scene{ID: cuid.New(), Name: "Test Scene", ProjectID: project.ID}
	testDB.DB.Create(scene)
	fixtureDef := &models.FixtureDefinition{ID: cuid.New(), Manufacturer: "T", Model: "M", Type: "t"}
	testDB.DB.Create(fixtureDef)

	fixtures := make([]*models.FixtureInstance, 3)
	for i := range fixtures {
		fixtures[i] = &models.FixtureInstance{
			ID:           cuid.New(),
			Name:         "F" + string(rune('1'+i)),
			ProjectID:    project.ID,
			DefinitionID: fixtureDef.ID,
			Universe:     1,
			StartChannel: i*10 + 1,
		}
		testDB.DB.Create(fixtures[i])
	}

	// Test CreateFixtureValues
	values := []models.FixtureValue{
		{SceneID: scene.ID, FixtureID: fixtures[0].ID, ChannelValues: "[1]"},
		{SceneID: scene.ID, FixtureID: fixtures[1].ID, ChannelValues: "[2]"},
		{SceneID: scene.ID, FixtureID: fixtures[2].ID, ChannelValues: "[3]"},
	}
	err := repo.CreateFixtureValues(ctx, values)
	if err != nil {
		t.Fatalf("CreateFixtureValues failed: %v", err)
	}

	fvs, _ := repo.GetFixtureValues(ctx, scene.ID)
	if len(fvs) != 3 {
		t.Errorf("Expected 3 fixture values, got %d", len(fvs))
	}

	// Test empty values
	err = repo.CreateFixtureValues(ctx, []models.FixtureValue{})
	if err != nil {
		t.Errorf("CreateFixtureValues with empty slice failed: %v", err)
	}

	// Test DeleteFixtureValues
	err = repo.DeleteFixtureValues(ctx, scene.ID)
	if err != nil {
		t.Fatalf("DeleteFixtureValues failed: %v", err)
	}
	fvs, _ = repo.GetFixtureValues(ctx, scene.ID)
	if len(fvs) != 0 {
		t.Errorf("Expected 0 fixture values after delete, got %d", len(fvs))
	}
}

// TestNewSceneRepository tests the constructor.
func TestNewSceneRepository(t *testing.T) {
	testDB, cleanup := setupTestDB(t)
	defer cleanup()

	repo := NewSceneRepository(testDB.DB)
	if repo == nil {
		t.Error("Expected non-nil repository")
	}
}

// TestCueListRepository_CRUD tests basic CRUD operations.
func TestCueListRepository_CRUD(t *testing.T) {
	testDB, cleanup := setupTestDB(t)
	defer cleanup()

	repo := NewCueListRepository(testDB.DB)
	ctx := context.Background()

	// Create project
	project := &models.Project{ID: cuid.New(), Name: "Test Project"}
	testDB.DB.Create(project)

	// Test Create
	cueList := &models.CueList{
		Name:      "Test Cue List " + cuid.Slug(),
		ProjectID: project.ID,
	}
	err := repo.Create(ctx, cueList)
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}
	if cueList.ID == "" {
		t.Error("Expected cue list ID to be set")
	}

	// Test FindByID
	found, err := repo.FindByID(ctx, cueList.ID)
	if err != nil {
		t.Fatalf("FindByID failed: %v", err)
	}
	if found == nil {
		t.Fatal("Expected to find cue list")
	}

	// Test FindByProjectID
	lists, err := repo.FindByProjectID(ctx, project.ID)
	if err != nil {
		t.Fatalf("FindByProjectID failed: %v", err)
	}
	if len(lists) == 0 {
		t.Error("Expected at least one cue list")
	}

	// Test Update
	cueList.Name = "Updated Name"
	err = repo.Update(ctx, cueList)
	if err != nil {
		t.Fatalf("Update failed: %v", err)
	}

	// Test Delete
	err = repo.Delete(ctx, cueList.ID)
	if err != nil {
		t.Fatalf("Delete failed: %v", err)
	}
	found, _ = repo.FindByID(ctx, cueList.ID)
	if found != nil {
		t.Error("Expected cue list to be deleted")
	}
}

// TestCueListRepository_FindByID_NotFound tests FindByID with non-existent ID.
func TestCueListRepository_FindByID_NotFound(t *testing.T) {
	testDB, cleanup := setupTestDB(t)
	defer cleanup()

	repo := NewCueListRepository(testDB.DB)
	ctx := context.Background()

	found, err := repo.FindByID(ctx, "nonexistent-id")
	if err != nil {
		t.Fatalf("FindByID failed: %v", err)
	}
	if found != nil {
		t.Error("Expected nil for non-existent cue list")
	}
}

// TestCueListRepository_CueOperations tests cue-related operations.
func TestCueListRepository_CueOperations(t *testing.T) {
	testDB, cleanup := setupTestDB(t)
	defer cleanup()

	repo := NewCueListRepository(testDB.DB)
	ctx := context.Background()

	// Create project, scene, cue list
	project := &models.Project{ID: cuid.New(), Name: "Test Project"}
	testDB.DB.Create(project)
	scene := &models.Scene{ID: cuid.New(), Name: "Test Scene", ProjectID: project.ID}
	testDB.DB.Create(scene)
	cueList := &models.CueList{ID: cuid.New(), Name: "Test CL", ProjectID: project.ID}
	testDB.DB.Create(cueList)

	// Test CountCues (should be 0)
	count, err := repo.CountCues(ctx, cueList.ID)
	if err != nil {
		t.Fatalf("CountCues failed: %v", err)
	}
	if count != 0 {
		t.Errorf("Expected 0 cues, got %d", count)
	}

	// Create cues
	cue := &models.Cue{
		ID:        cuid.New(),
		Name:      "Cue 1",
		CueNumber: 1.0,
		CueListID: cueList.ID,
		SceneID:   scene.ID,
	}
	testDB.DB.Create(cue)

	count, _ = repo.CountCues(ctx, cueList.ID)
	if count != 1 {
		t.Errorf("Expected 1 cue, got %d", count)
	}

	// Test GetCues
	cues, err := repo.GetCues(ctx, cueList.ID)
	if err != nil {
		t.Fatalf("GetCues failed: %v", err)
	}
	if len(cues) != 1 {
		t.Errorf("Expected 1 cue, got %d", len(cues))
	}
}

// TestNewCueListRepository tests the constructor.
func TestNewCueListRepository(t *testing.T) {
	testDB, cleanup := setupTestDB(t)
	defer cleanup()

	repo := NewCueListRepository(testDB.DB)
	if repo == nil {
		t.Error("Expected non-nil repository")
	}
}

// TestCueRepository_CRUD tests basic CRUD operations.
func TestCueRepository_CRUD(t *testing.T) {
	testDB, cleanup := setupTestDB(t)
	defer cleanup()

	repo := NewCueRepository(testDB.DB)
	ctx := context.Background()

	// Create project, scene, cue list
	project := &models.Project{ID: cuid.New(), Name: "Test Project"}
	testDB.DB.Create(project)
	scene := &models.Scene{ID: cuid.New(), Name: "Test Scene", ProjectID: project.ID}
	testDB.DB.Create(scene)
	cueList := &models.CueList{ID: cuid.New(), Name: "Test CL", ProjectID: project.ID}
	testDB.DB.Create(cueList)

	// Test Create
	cue := &models.Cue{
		Name:      "Test Cue",
		CueNumber: 1.0,
		CueListID: cueList.ID,
		SceneID:   scene.ID,
	}
	err := repo.Create(ctx, cue)
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}
	if cue.ID == "" {
		t.Error("Expected cue ID to be set")
	}

	// Test FindByID
	found, err := repo.FindByID(ctx, cue.ID)
	if err != nil {
		t.Fatalf("FindByID failed: %v", err)
	}
	if found == nil {
		t.Fatal("Expected to find cue")
	}

	// Test FindByCueListID
	cues, err := repo.FindByCueListID(ctx, cueList.ID)
	if err != nil {
		t.Fatalf("FindByCueListID failed: %v", err)
	}
	if len(cues) == 0 {
		t.Error("Expected at least one cue")
	}

	// Test Update
	cue.Name = "Updated Cue"
	err = repo.Update(ctx, cue)
	if err != nil {
		t.Fatalf("Update failed: %v", err)
	}

	// Test Delete
	err = repo.Delete(ctx, cue.ID)
	if err != nil {
		t.Fatalf("Delete failed: %v", err)
	}
	found, _ = repo.FindByID(ctx, cue.ID)
	if found != nil {
		t.Error("Expected cue to be deleted")
	}
}

// TestCueRepository_FindByID_NotFound tests FindByID with non-existent ID.
func TestCueRepository_FindByID_NotFound(t *testing.T) {
	testDB, cleanup := setupTestDB(t)
	defer cleanup()

	repo := NewCueRepository(testDB.DB)
	ctx := context.Background()

	found, err := repo.FindByID(ctx, "nonexistent-id")
	if err != nil {
		t.Fatalf("FindByID failed: %v", err)
	}
	if found != nil {
		t.Error("Expected nil for non-existent cue")
	}
}

// TestCueRepository_DeleteByCueListID tests bulk delete.
func TestCueRepository_DeleteByCueListID(t *testing.T) {
	testDB, cleanup := setupTestDB(t)
	defer cleanup()

	repo := NewCueRepository(testDB.DB)
	ctx := context.Background()

	// Create project, scene, cue list, cues
	project := &models.Project{ID: cuid.New(), Name: "Test Project"}
	testDB.DB.Create(project)
	scene := &models.Scene{ID: cuid.New(), Name: "Test Scene", ProjectID: project.ID}
	testDB.DB.Create(scene)
	cueList := &models.CueList{ID: cuid.New(), Name: "Test CL", ProjectID: project.ID}
	testDB.DB.Create(cueList)

	for i := 0; i < 3; i++ {
		cue := &models.Cue{
			ID:        cuid.New(),
			Name:      "Cue",
			CueNumber: float64(i + 1),
			CueListID: cueList.ID,
			SceneID:   scene.ID,
		}
		testDB.DB.Create(cue)
	}

	cues, _ := repo.FindByCueListID(ctx, cueList.ID)
	if len(cues) != 3 {
		t.Fatalf("Expected 3 cues, got %d", len(cues))
	}

	err := repo.DeleteByCueListID(ctx, cueList.ID)
	if err != nil {
		t.Fatalf("DeleteByCueListID failed: %v", err)
	}

	cues, _ = repo.FindByCueListID(ctx, cueList.ID)
	if len(cues) != 0 {
		t.Errorf("Expected 0 cues after delete, got %d", len(cues))
	}
}

// TestNewCueRepository tests the constructor.
func TestNewCueRepository(t *testing.T) {
	testDB, cleanup := setupTestDB(t)
	defer cleanup()

	repo := NewCueRepository(testDB.DB)
	if repo == nil {
		t.Error("Expected non-nil repository")
	}
}

// TestFixtureRepository_CRUD tests basic CRUD operations.
func TestFixtureRepository_CRUD(t *testing.T) {
	testDB, cleanup := setupTestDB(t)
	defer cleanup()

	repo := NewFixtureRepository(testDB.DB)
	ctx := context.Background()

	// Create project and definition
	project := &models.Project{ID: cuid.New(), Name: "Test Project"}
	testDB.DB.Create(project)
	fixtureDef := &models.FixtureDefinition{
		ID:           cuid.New(),
		Manufacturer: "Test",
		Model:        "Test Model",
		Type:         "test",
	}
	testDB.DB.Create(fixtureDef)

	// Test Create
	fixture := &models.FixtureInstance{
		Name:         "Test Fixture " + cuid.Slug(),
		ProjectID:    project.ID,
		DefinitionID: fixtureDef.ID,
		Universe:     1,
		StartChannel: 1,
	}
	err := repo.Create(ctx, fixture)
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}
	if fixture.ID == "" {
		t.Error("Expected fixture ID to be set")
	}

	// Test FindByID
	found, err := repo.FindByID(ctx, fixture.ID)
	if err != nil {
		t.Fatalf("FindByID failed: %v", err)
	}
	if found == nil {
		t.Fatal("Expected to find fixture")
	}

	// Test FindByProjectID
	fixtures, err := repo.FindByProjectID(ctx, project.ID)
	if err != nil {
		t.Fatalf("FindByProjectID failed: %v", err)
	}
	if len(fixtures) == 0 {
		t.Error("Expected at least one fixture")
	}

	// Test Update
	fixture.Name = "Updated Fixture"
	err = repo.Update(ctx, fixture)
	if err != nil {
		t.Fatalf("Update failed: %v", err)
	}

	// Test Delete
	err = repo.Delete(ctx, fixture.ID)
	if err != nil {
		t.Fatalf("Delete failed: %v", err)
	}
	found, _ = repo.FindByID(ctx, fixture.ID)
	if found != nil {
		t.Error("Expected fixture to be deleted")
	}
}

// TestFixtureRepository_FindByID_NotFound tests FindByID with non-existent ID.
func TestFixtureRepository_FindByID_NotFound(t *testing.T) {
	testDB, cleanup := setupTestDB(t)
	defer cleanup()

	repo := NewFixtureRepository(testDB.DB)
	ctx := context.Background()

	found, err := repo.FindByID(ctx, "nonexistent-id")
	if err != nil {
		t.Fatalf("FindByID failed: %v", err)
	}
	if found != nil {
		t.Error("Expected nil for non-existent fixture")
	}
}

// TestFixtureRepository_DefinitionOperations tests definition operations.
func TestFixtureRepository_DefinitionOperations(t *testing.T) {
	testDB, cleanup := setupTestDB(t)
	defer cleanup()

	repo := NewFixtureRepository(testDB.DB)
	ctx := context.Background()

	// Test CreateDefinition
	def := &models.FixtureDefinition{
		Manufacturer: "Test Mfg",
		Model:        "Test Model " + cuid.Slug(),
		Type:         "dimmer",
	}
	err := repo.CreateDefinition(ctx, def)
	if err != nil {
		t.Fatalf("CreateDefinition failed: %v", err)
	}
	if def.ID == "" {
		t.Error("Expected definition ID to be set")
	}

	// Test FindDefinitionByID
	found, err := repo.FindDefinitionByID(ctx, def.ID)
	if err != nil {
		t.Fatalf("FindDefinitionByID failed: %v", err)
	}
	if found == nil {
		t.Fatal("Expected to find definition")
	}

	// Test FindDefinitionByManufacturerModel
	found, err = repo.FindDefinitionByManufacturerModel(ctx, def.Manufacturer, def.Model)
	if err != nil {
		t.Fatalf("FindDefinitionByManufacturerModel failed: %v", err)
	}
	if found == nil {
		t.Fatal("Expected to find definition by manufacturer/model")
	}

	// Test FindAllDefinitions
	defs, err := repo.FindAllDefinitions(ctx)
	if err != nil {
		t.Fatalf("FindAllDefinitions failed: %v", err)
	}
	if len(defs) == 0 {
		t.Error("Expected at least one definition")
	}

	// Test UpdateDefinition
	def.Type = "moving_head"
	err = repo.UpdateDefinition(ctx, def)
	if err != nil {
		t.Fatalf("UpdateDefinition failed: %v", err)
	}

	// Test DeleteDefinition
	err = repo.DeleteDefinition(ctx, def.ID)
	if err != nil {
		t.Fatalf("DeleteDefinition failed: %v", err)
	}
	found, _ = repo.FindDefinitionByID(ctx, def.ID)
	if found != nil {
		t.Error("Expected definition to be deleted")
	}
}

// TestFixtureRepository_FindDefinitionByID_NotFound tests FindDefinitionByID with non-existent ID.
func TestFixtureRepository_FindDefinitionByID_NotFound(t *testing.T) {
	testDB, cleanup := setupTestDB(t)
	defer cleanup()

	repo := NewFixtureRepository(testDB.DB)
	ctx := context.Background()

	found, err := repo.FindDefinitionByID(ctx, "nonexistent-id")
	if err != nil {
		t.Fatalf("FindDefinitionByID failed: %v", err)
	}
	if found != nil {
		t.Error("Expected nil for non-existent definition")
	}
}

// TestFixtureRepository_FindDefinitionByManufacturerModel_NotFound tests not found case.
func TestFixtureRepository_FindDefinitionByManufacturerModel_NotFound(t *testing.T) {
	testDB, cleanup := setupTestDB(t)
	defer cleanup()

	repo := NewFixtureRepository(testDB.DB)
	ctx := context.Background()

	found, err := repo.FindDefinitionByManufacturerModel(ctx, "NoSuchMfg", "NoSuchModel")
	if err != nil {
		t.Fatalf("FindDefinitionByManufacturerModel failed: %v", err)
	}
	if found != nil {
		t.Error("Expected nil for non-existent definition")
	}
}

// TestFixtureRepository_ChannelOperations tests channel operations.
func TestFixtureRepository_ChannelOperations(t *testing.T) {
	testDB, cleanup := setupTestDB(t)
	defer cleanup()

	repo := NewFixtureRepository(testDB.DB)
	ctx := context.Background()

	// Create definition
	def := &models.FixtureDefinition{
		ID:           cuid.New(),
		Manufacturer: "Test",
		Model:        "M",
		Type:         "t",
	}
	testDB.DB.Create(def)

	// Test CreateChannelDefinition
	channelDef := &models.ChannelDefinition{
		Name:         "Dimmer",
		Type:         "intensity",
		Offset:       0,
		DefinitionID: def.ID,
	}
	err := repo.CreateChannelDefinition(ctx, channelDef)
	if err != nil {
		t.Fatalf("CreateChannelDefinition failed: %v", err)
	}

	// Test GetDefinitionChannels
	channels, err := repo.GetDefinitionChannels(ctx, def.ID)
	if err != nil {
		t.Fatalf("GetDefinitionChannels failed: %v", err)
	}
	if len(channels) != 1 {
		t.Errorf("Expected 1 channel, got %d", len(channels))
	}

	// Test GetChannelDefinitionByID
	found, err := repo.GetChannelDefinitionByID(ctx, channelDef.ID)
	if err != nil {
		t.Fatalf("GetChannelDefinitionByID failed: %v", err)
	}
	if found == nil {
		t.Fatal("Expected to find channel definition")
	}

	// Test CreateChannelDefinitions
	newChannels := []models.ChannelDefinition{
		{Name: "Red", Type: "color", Offset: 1, DefinitionID: def.ID},
		{Name: "Green", Type: "color", Offset: 2, DefinitionID: def.ID},
	}
	err = repo.CreateChannelDefinitions(ctx, newChannels)
	if err != nil {
		t.Fatalf("CreateChannelDefinitions failed: %v", err)
	}

	channels, _ = repo.GetDefinitionChannels(ctx, def.ID)
	if len(channels) != 3 {
		t.Errorf("Expected 3 channels, got %d", len(channels))
	}

	// Test empty channels
	err = repo.CreateChannelDefinitions(ctx, []models.ChannelDefinition{})
	if err != nil {
		t.Errorf("CreateChannelDefinitions with empty slice failed: %v", err)
	}

	// Test DeleteChannelDefinitions
	err = repo.DeleteChannelDefinitions(ctx, def.ID)
	if err != nil {
		t.Fatalf("DeleteChannelDefinitions failed: %v", err)
	}
	channels, _ = repo.GetDefinitionChannels(ctx, def.ID)
	if len(channels) != 0 {
		t.Errorf("Expected 0 channels after delete, got %d", len(channels))
	}
}

// TestFixtureRepository_GetChannelDefinitionByID_NotFound tests not found case.
func TestFixtureRepository_GetChannelDefinitionByID_NotFound(t *testing.T) {
	testDB, cleanup := setupTestDB(t)
	defer cleanup()

	repo := NewFixtureRepository(testDB.DB)
	ctx := context.Background()

	found, err := repo.GetChannelDefinitionByID(ctx, "nonexistent-id")
	if err != nil {
		t.Fatalf("GetChannelDefinitionByID failed: %v", err)
	}
	if found != nil {
		t.Error("Expected nil for non-existent channel definition")
	}
}

// TestFixtureRepository_InstanceChannelOperations tests instance channel operations.
func TestFixtureRepository_InstanceChannelOperations(t *testing.T) {
	testDB, cleanup := setupTestDB(t)
	defer cleanup()

	repo := NewFixtureRepository(testDB.DB)
	ctx := context.Background()

	// Create project, definition, fixture
	project := &models.Project{ID: cuid.New(), Name: "P"}
	testDB.DB.Create(project)
	def := &models.FixtureDefinition{ID: cuid.New(), Manufacturer: "T", Model: "M", Type: "t"}
	testDB.DB.Create(def)
	fixture := &models.FixtureInstance{
		ID:           cuid.New(),
		Name:         "F",
		ProjectID:    project.ID,
		DefinitionID: def.ID,
		Universe:     1,
		StartChannel: 1,
	}
	testDB.DB.Create(fixture)

	// Test CreateInstanceChannels
	channels := []models.InstanceChannel{
		{FixtureID: fixture.ID, Offset: 0, Name: "Dimmer", Type: "intensity"},
		{FixtureID: fixture.ID, Offset: 1, Name: "Red", Type: "color"},
	}
	err := repo.CreateInstanceChannels(ctx, channels)
	if err != nil {
		t.Fatalf("CreateInstanceChannels failed: %v", err)
	}

	// Test GetInstanceChannels
	instanceChannels, err := repo.GetInstanceChannels(ctx, fixture.ID)
	if err != nil {
		t.Fatalf("GetInstanceChannels failed: %v", err)
	}
	if len(instanceChannels) != 2 {
		t.Errorf("Expected 2 channels, got %d", len(instanceChannels))
	}

	// Test empty channels
	err = repo.CreateInstanceChannels(ctx, []models.InstanceChannel{})
	if err != nil {
		t.Errorf("CreateInstanceChannels with empty slice failed: %v", err)
	}

	// Test DeleteInstanceChannels
	err = repo.DeleteInstanceChannels(ctx, fixture.ID)
	if err != nil {
		t.Fatalf("DeleteInstanceChannels failed: %v", err)
	}
	instanceChannels, _ = repo.GetInstanceChannels(ctx, fixture.ID)
	if len(instanceChannels) != 0 {
		t.Errorf("Expected 0 channels after delete, got %d", len(instanceChannels))
	}
}

// TestFixtureRepository_CreateWithChannels tests transactional creation.
func TestFixtureRepository_CreateWithChannels(t *testing.T) {
	testDB, cleanup := setupTestDB(t)
	defer cleanup()

	repo := NewFixtureRepository(testDB.DB)
	ctx := context.Background()

	// Create project, definition
	project := &models.Project{ID: cuid.New(), Name: "P"}
	testDB.DB.Create(project)
	def := &models.FixtureDefinition{ID: cuid.New(), Manufacturer: "T", Model: "M", Type: "t"}
	testDB.DB.Create(def)

	// Create fixture with channels
	fixture := &models.FixtureInstance{
		Name:         "Fixture with channels",
		ProjectID:    project.ID,
		DefinitionID: def.ID,
		Universe:     1,
		StartChannel: 1,
	}
	channels := []models.InstanceChannel{
		{Offset: 0, Name: "Dimmer", Type: "intensity"},
		{Offset: 1, Name: "Red", Type: "color"},
	}

	err := repo.CreateWithChannels(ctx, fixture, channels)
	if err != nil {
		t.Fatalf("CreateWithChannels failed: %v", err)
	}

	if fixture.ID == "" {
		t.Error("Expected fixture ID to be set")
	}

	// Verify channels were created
	instanceChannels, _ := repo.GetInstanceChannels(ctx, fixture.ID)
	if len(instanceChannels) != 2 {
		t.Errorf("Expected 2 channels, got %d", len(instanceChannels))
	}
}

// TestFixtureRepository_CreateDefinitionWithChannels tests transactional definition creation.
func TestFixtureRepository_CreateDefinitionWithChannels(t *testing.T) {
	testDB, cleanup := setupTestDB(t)
	defer cleanup()

	repo := NewFixtureRepository(testDB.DB)
	ctx := context.Background()

	// Create definition with channels
	def := &models.FixtureDefinition{
		Manufacturer: "Test",
		Model:        "Model with channels",
		Type:         "led",
	}
	channels := []models.ChannelDefinition{
		{Name: "Dimmer", Type: "intensity", Offset: 0},
		{Name: "Red", Type: "color", Offset: 1},
		{Name: "Green", Type: "color", Offset: 2},
	}

	err := repo.CreateDefinitionWithChannels(ctx, def, channels)
	if err != nil {
		t.Fatalf("CreateDefinitionWithChannels failed: %v", err)
	}

	if def.ID == "" {
		t.Error("Expected definition ID to be set")
	}

	// Verify channels were created
	defChannels, _ := repo.GetDefinitionChannels(ctx, def.ID)
	if len(defChannels) != 3 {
		t.Errorf("Expected 3 channels, got %d", len(defChannels))
	}
}

// TestFixtureRepository_ModeOperations tests mode-related operations.
func TestFixtureRepository_ModeOperations(t *testing.T) {
	testDB, cleanup := setupTestDB(t)
	defer cleanup()

	repo := NewFixtureRepository(testDB.DB)
	ctx := context.Background()

	// Create definition and mode
	def := &models.FixtureDefinition{ID: cuid.New(), Manufacturer: "T", Model: "M", Type: "t"}
	testDB.DB.Create(def)
	mode := &models.FixtureMode{
		ID:           cuid.New(),
		Name:         "Standard",
		ChannelCount: 3,
		DefinitionID: def.ID,
	}
	testDB.DB.Create(mode)

	// Test FindModeByID
	found, err := repo.FindModeByID(ctx, mode.ID)
	if err != nil {
		t.Fatalf("FindModeByID failed: %v", err)
	}
	if found == nil {
		t.Fatal("Expected to find mode")
	}
	if found.Name != "Standard" {
		t.Errorf("Name mismatch: got %s", found.Name)
	}

	// Test FindModeByID not found
	found, err = repo.FindModeByID(ctx, "nonexistent-id")
	if err != nil {
		t.Fatalf("FindModeByID failed: %v", err)
	}
	if found != nil {
		t.Error("Expected nil for non-existent mode")
	}

	// Create channel definition and mode channel
	channelDef := &models.ChannelDefinition{
		ID:           cuid.New(),
		Name:         "Dimmer",
		Type:         "intensity",
		Offset:       0,
		DefinitionID: def.ID,
	}
	testDB.DB.Create(channelDef)
	modeChannel := &models.ModeChannel{
		ID:        cuid.New(),
		ModeID:    mode.ID,
		ChannelID: channelDef.ID,
		Offset:    0,
	}
	testDB.DB.Create(modeChannel)

	// Test GetModeChannels
	modeChannels, err := repo.GetModeChannels(ctx, mode.ID)
	if err != nil {
		t.Fatalf("GetModeChannels failed: %v", err)
	}
	if len(modeChannels) != 1 {
		t.Errorf("Expected 1 mode channel, got %d", len(modeChannels))
	}
}

// TestFixtureRepository_CountInstancesByDefinitionID tests count operation.
func TestFixtureRepository_CountInstancesByDefinitionID(t *testing.T) {
	testDB, cleanup := setupTestDB(t)
	defer cleanup()

	repo := NewFixtureRepository(testDB.DB)
	ctx := context.Background()

	// Create project and definition
	project := &models.Project{ID: cuid.New(), Name: "P"}
	testDB.DB.Create(project)
	def := &models.FixtureDefinition{ID: cuid.New(), Manufacturer: "T", Model: "M", Type: "t"}
	testDB.DB.Create(def)

	// Test count (should be 0)
	count, err := repo.CountInstancesByDefinitionID(ctx, def.ID)
	if err != nil {
		t.Fatalf("CountInstancesByDefinitionID failed: %v", err)
	}
	if count != 0 {
		t.Errorf("Expected 0 instances, got %d", count)
	}

	// Create fixtures
	for i := 0; i < 3; i++ {
		fixture := &models.FixtureInstance{
			ID:           cuid.New(),
			Name:         "F",
			ProjectID:    project.ID,
			DefinitionID: def.ID,
			Universe:     1,
			StartChannel: i*10 + 1,
		}
		testDB.DB.Create(fixture)
	}

	count, _ = repo.CountInstancesByDefinitionID(ctx, def.ID)
	if count != 3 {
		t.Errorf("Expected 3 instances, got %d", count)
	}
}

// TestNewFixtureRepository tests the constructor.
func TestNewFixtureRepository(t *testing.T) {
	testDB, cleanup := setupTestDB(t)
	defer cleanup()

	repo := NewFixtureRepository(testDB.DB)
	if repo == nil {
		t.Error("Expected non-nil repository")
	}
}

// TestFixtureRepository_CountDefinitions tests the definition count operation.
func TestFixtureRepository_CountDefinitions(t *testing.T) {
	testDB, cleanup := setupTestDB(t)
	defer cleanup()

	repo := NewFixtureRepository(testDB.DB)
	ctx := context.Background()

	// Test count (should be 0)
	count, err := repo.CountDefinitions(ctx)
	if err != nil {
		t.Fatalf("CountDefinitions failed: %v", err)
	}
	if count != 0 {
		t.Errorf("Expected 0 definitions, got %d", count)
	}

	// Create definitions
	for i := 0; i < 3; i++ {
		def := &models.FixtureDefinition{
			ID:           cuid.New(),
			Manufacturer: "Test",
			Model:        "Model" + string(rune('A'+i)),
			Type:         "led",
		}
		testDB.DB.Create(def)
	}

	count, err = repo.CountDefinitions(ctx)
	if err != nil {
		t.Fatalf("CountDefinitions failed: %v", err)
	}
	if count != 3 {
		t.Errorf("Expected 3 definitions, got %d", count)
	}
}
