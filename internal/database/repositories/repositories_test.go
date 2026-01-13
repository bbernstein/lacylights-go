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
		&models.Look{},
		&models.FixtureValue{},
		&models.CueList{},
		&models.Cue{},
		&models.Setting{},
		&models.LookBoard{},
		&models.LookBoardButton{},
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

	// Test CountLooks (should be 0)
	count, err = repo.CountLooks(ctx, project.ID)
	if err != nil {
		t.Fatalf("CountLooks failed: %v", err)
	}
	if count != 0 {
		t.Errorf("Expected 0 looks, got %d", count)
	}

	// Create a look
	look := &models.Look{
		ID:        cuid.New(),
		Name:      "Test Look",
		ProjectID: project.ID,
	}
	testDB.DB.Create(look)

	count, err = repo.CountLooks(ctx, project.ID)
	if err != nil {
		t.Fatalf("CountLooks failed: %v", err)
	}
	if count != 1 {
		t.Errorf("Expected 1 look, got %d", count)
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

// TestLookRepository_CRUD tests basic CRUD operations on the LookRepository.
func TestLookRepository_CRUD(t *testing.T) {
	testDB, cleanup := setupTestDB(t)
	defer cleanup()

	repo := NewLookRepository(testDB.DB)
	ctx := context.Background()

	// Create a project first
	project := &models.Project{ID: cuid.New(), Name: "Test Project"}
	testDB.DB.Create(project)

	// Test Create
	look := &models.Look{
		Name:      "Test Look " + cuid.Slug(),
		ProjectID: project.ID,
	}
	err := repo.Create(ctx, look)
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}
	if look.ID == "" {
		t.Error("Expected look ID to be set after Create")
	}

	// Test FindByID
	found, err := repo.FindByID(ctx, look.ID)
	if err != nil {
		t.Fatalf("FindByID failed: %v", err)
	}
	if found == nil {
		t.Fatal("Expected to find look")
	}
	if found.Name != look.Name {
		t.Errorf("Name mismatch: got %s, want %s", found.Name, look.Name)
	}

	// Test FindByProjectID
	looks, err := repo.FindByProjectID(ctx, project.ID)
	if err != nil {
		t.Fatalf("FindByProjectID failed: %v", err)
	}
	if len(looks) == 0 {
		t.Error("Expected at least one look")
	}

	// Test Update
	look.Name = "Updated Look Name"
	err = repo.Update(ctx, look)
	if err != nil {
		t.Fatalf("Update failed: %v", err)
	}
	found, _ = repo.FindByID(ctx, look.ID)
	if found.Name != "Updated Look Name" {
		t.Errorf("Update didn't persist: got %s", found.Name)
	}

	// Test Delete
	err = repo.Delete(ctx, look.ID)
	if err != nil {
		t.Fatalf("Delete failed: %v", err)
	}
	found, err = repo.FindByID(ctx, look.ID)
	if err != nil {
		t.Fatalf("FindByID after delete failed: %v", err)
	}
	if found != nil {
		t.Error("Expected look to be deleted")
	}
}

// TestLookRepository_FindByID_NotFound tests FindByID with non-existent ID.
func TestLookRepository_FindByID_NotFound(t *testing.T) {
	testDB, cleanup := setupTestDB(t)
	defer cleanup()

	repo := NewLookRepository(testDB.DB)
	ctx := context.Background()

	found, err := repo.FindByID(ctx, "nonexistent-id")
	if err != nil {
		t.Fatalf("FindByID failed: %v", err)
	}
	if found != nil {
		t.Error("Expected nil for non-existent look")
	}
}

// TestLookRepository_FixtureValueOperations tests fixture value operations.
func TestLookRepository_FixtureValueOperations(t *testing.T) {
	testDB, cleanup := setupTestDB(t)
	defer cleanup()

	repo := NewLookRepository(testDB.DB)
	ctx := context.Background()

	// Create project and look
	project := &models.Project{ID: cuid.New(), Name: "Test Project"}
	testDB.DB.Create(project)
	look := &models.Look{ID: cuid.New(), Name: "Test Look", ProjectID: project.ID}
	testDB.DB.Create(look)

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
	count, err := repo.CountFixtures(ctx, look.ID)
	if err != nil {
		t.Fatalf("CountFixtures failed: %v", err)
	}
	if count != 0 {
		t.Errorf("Expected 0 fixtures, got %d", count)
	}

	// Test CreateFixtureValue
	fv := &models.FixtureValue{
		LookID:    look.ID,
		FixtureID: fixture.ID,
		Channels:  `[{"offset":0,"value":255},{"offset":1,"value":128},{"offset":2,"value":64}]`,
	}
	err = repo.CreateFixtureValue(ctx, fv)
	if err != nil {
		t.Fatalf("CreateFixtureValue failed: %v", err)
	}

	count, _ = repo.CountFixtures(ctx, look.ID)
	if count != 1 {
		t.Errorf("Expected 1 fixture, got %d", count)
	}

	// Test GetFixtureValues
	values, err := repo.GetFixtureValues(ctx, look.ID)
	if err != nil {
		t.Fatalf("GetFixtureValues failed: %v", err)
	}
	if len(values) != 1 {
		t.Errorf("Expected 1 value, got %d", len(values))
	}

	// Test GetFixtureValue
	found, err := repo.GetFixtureValue(ctx, look.ID, fixture.ID)
	if err != nil {
		t.Fatalf("GetFixtureValue failed: %v", err)
	}
	if found == nil {
		t.Fatal("Expected to find fixture value")
	}
	if found.Channels != `[{"offset":0,"value":255},{"offset":1,"value":128},{"offset":2,"value":64}]` {
		t.Errorf("Channels mismatch: got %s", found.Channels)
	}

	// Test UpdateFixtureValue
	found.Channels = `[{"offset":0,"value":0},{"offset":1,"value":0},{"offset":2,"value":0}]`
	err = repo.UpdateFixtureValue(ctx, found)
	if err != nil {
		t.Fatalf("UpdateFixtureValue failed: %v", err)
	}
	updated, _ := repo.GetFixtureValue(ctx, look.ID, fixture.ID)
	if updated.Channels != `[{"offset":0,"value":0},{"offset":1,"value":0},{"offset":2,"value":0}]` {
		t.Errorf("Update didn't persist: got %s", updated.Channels)
	}

	// Test DeleteFixtureValue
	err = repo.DeleteFixtureValue(ctx, look.ID, fixture.ID)
	if err != nil {
		t.Fatalf("DeleteFixtureValue failed: %v", err)
	}
	found, _ = repo.GetFixtureValue(ctx, look.ID, fixture.ID)
	if found != nil {
		t.Error("Expected fixture value to be deleted")
	}
}

// TestLookRepository_CreateWithFixtureValues tests creating look with values.
func TestLookRepository_CreateWithFixtureValues(t *testing.T) {
	testDB, cleanup := setupTestDB(t)
	defer cleanup()

	repo := NewLookRepository(testDB.DB)
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

	// Create look with fixture values
	look := &models.Look{Name: "Look with values", ProjectID: project.ID}
	values := []models.FixtureValue{
		{FixtureID: fixture.ID, Channels: `[{"offset":0,"value":255}]`},
	}

	err := repo.CreateWithFixtureValues(ctx, look, values)
	if err != nil {
		t.Fatalf("CreateWithFixtureValues failed: %v", err)
	}

	if look.ID == "" {
		t.Error("Expected look ID to be set")
	}

	// Verify fixture values were created
	fvs, _ := repo.GetFixtureValues(ctx, look.ID)
	if len(fvs) != 1 {
		t.Errorf("Expected 1 fixture value, got %d", len(fvs))
	}
}

// TestLookRepository_CreateFixtureValues tests bulk create.
func TestLookRepository_CreateFixtureValues(t *testing.T) {
	testDB, cleanup := setupTestDB(t)
	defer cleanup()

	repo := NewLookRepository(testDB.DB)
	ctx := context.Background()

	// Create project, look, fixtures
	project := &models.Project{ID: cuid.New(), Name: "Test Project"}
	testDB.DB.Create(project)
	look := &models.Look{ID: cuid.New(), Name: "Test Look", ProjectID: project.ID}
	testDB.DB.Create(look)
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
		{LookID: look.ID, FixtureID: fixtures[0].ID, Channels: `[{"offset":0,"value":1}]`},
		{LookID: look.ID, FixtureID: fixtures[1].ID, Channels: `[{"offset":0,"value":2}]`},
		{LookID: look.ID, FixtureID: fixtures[2].ID, Channels: `[{"offset":0,"value":3}]`},
	}
	err := repo.CreateFixtureValues(ctx, values)
	if err != nil {
		t.Fatalf("CreateFixtureValues failed: %v", err)
	}

	fvs, _ := repo.GetFixtureValues(ctx, look.ID)
	if len(fvs) != 3 {
		t.Errorf("Expected 3 fixture values, got %d", len(fvs))
	}

	// Test empty values
	err = repo.CreateFixtureValues(ctx, []models.FixtureValue{})
	if err != nil {
		t.Errorf("CreateFixtureValues with empty slice failed: %v", err)
	}

	// Test DeleteFixtureValues
	err = repo.DeleteFixtureValues(ctx, look.ID)
	if err != nil {
		t.Fatalf("DeleteFixtureValues failed: %v", err)
	}
	fvs, _ = repo.GetFixtureValues(ctx, look.ID)
	if len(fvs) != 0 {
		t.Errorf("Expected 0 fixture values after delete, got %d", len(fvs))
	}
}

// TestNewLookRepository tests the constructor.
func TestNewLookRepository(t *testing.T) {
	testDB, cleanup := setupTestDB(t)
	defer cleanup()

	repo := NewLookRepository(testDB.DB)
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

	// Create project, look, cue list
	project := &models.Project{ID: cuid.New(), Name: "Test Project"}
	testDB.DB.Create(project)
	look := &models.Look{ID: cuid.New(), Name: "Test Look", ProjectID: project.ID}
	testDB.DB.Create(look)
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
		LookID:    look.ID,
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

	// Create project, look, cue list
	project := &models.Project{ID: cuid.New(), Name: "Test Project"}
	testDB.DB.Create(project)
	look := &models.Look{ID: cuid.New(), Name: "Test Look", ProjectID: project.ID}
	testDB.DB.Create(look)
	cueList := &models.CueList{ID: cuid.New(), Name: "Test CL", ProjectID: project.ID}
	testDB.DB.Create(cueList)

	// Test Create
	cue := &models.Cue{
		Name:      "Test Cue",
		CueNumber: 1.0,
		CueListID: cueList.ID,
		LookID:    look.ID,
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

	// Create project, look, cue list, cues
	project := &models.Project{ID: cuid.New(), Name: "Test Project"}
	testDB.DB.Create(project)
	look := &models.Look{ID: cuid.New(), Name: "Test Look", ProjectID: project.ID}
	testDB.DB.Create(look)
	cueList := &models.CueList{ID: cuid.New(), Name: "Test CL", ProjectID: project.ID}
	testDB.DB.Create(cueList)

	for i := 0; i < 3; i++ {
		cue := &models.Cue{
			ID:        cuid.New(),
			Name:      "Cue",
			CueNumber: float64(i + 1),
			CueListID: cueList.ID,
			LookID:    look.ID,
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

// TestCueRepository_FindCueListIDsByLookID tests finding cue lists that use a given look.
func TestCueRepository_FindCueListIDsByLookID(t *testing.T) {
	testDB, cleanup := setupTestDB(t)
	defer cleanup()

	repo := NewCueRepository(testDB.DB)
	ctx := context.Background()

	// Create project
	project := &models.Project{ID: cuid.New(), Name: "Test Project"}
	testDB.DB.Create(project)

	// Create two looks
	look1 := &models.Look{ID: cuid.New(), Name: "Look 1", ProjectID: project.ID}
	look2 := &models.Look{ID: cuid.New(), Name: "Look 2", ProjectID: project.ID}
	testDB.DB.Create(look1)
	testDB.DB.Create(look2)

	// Create three cue lists
	cueList1 := &models.CueList{ID: cuid.New(), Name: "Cue List 1", ProjectID: project.ID}
	cueList2 := &models.CueList{ID: cuid.New(), Name: "Cue List 2", ProjectID: project.ID}
	cueList3 := &models.CueList{ID: cuid.New(), Name: "Cue List 3", ProjectID: project.ID}
	testDB.DB.Create(cueList1)
	testDB.DB.Create(cueList2)
	testDB.DB.Create(cueList3)

	// Create cues: look1 is used in cueList1 and cueList2, look2 only in cueList3
	cue1 := &models.Cue{ID: cuid.New(), Name: "Cue 1", CueNumber: 1.0, CueListID: cueList1.ID, LookID: look1.ID}
	cue2 := &models.Cue{ID: cuid.New(), Name: "Cue 2", CueNumber: 2.0, CueListID: cueList1.ID, LookID: look1.ID} // look1 used twice in same cue list
	cue3 := &models.Cue{ID: cuid.New(), Name: "Cue 3", CueNumber: 1.0, CueListID: cueList2.ID, LookID: look1.ID}
	cue4 := &models.Cue{ID: cuid.New(), Name: "Cue 4", CueNumber: 1.0, CueListID: cueList3.ID, LookID: look2.ID}
	testDB.DB.Create(cue1)
	testDB.DB.Create(cue2)
	testDB.DB.Create(cue3)
	testDB.DB.Create(cue4)

	// Test: Find cue lists using look1 - should return cueList1 and cueList2 (distinct)
	cueListIDs, err := repo.FindCueListIDsByLookID(ctx, look1.ID)
	if err != nil {
		t.Fatalf("FindCueListIDsByLookID failed: %v", err)
	}
	if len(cueListIDs) != 2 {
		t.Errorf("Expected 2 cue list IDs for look1, got %d", len(cueListIDs))
	}
	// Check that both cueList1 and cueList2 are in the result
	foundCueList1, foundCueList2 := false, false
	for _, id := range cueListIDs {
		if id == cueList1.ID {
			foundCueList1 = true
		}
		if id == cueList2.ID {
			foundCueList2 = true
		}
	}
	if !foundCueList1 {
		t.Error("Expected cueList1.ID in results for look1")
	}
	if !foundCueList2 {
		t.Error("Expected cueList2.ID in results for look1")
	}

	// Test: Find cue lists using look2 - should return only cueList3
	cueListIDs, err = repo.FindCueListIDsByLookID(ctx, look2.ID)
	if err != nil {
		t.Fatalf("FindCueListIDsByLookID failed: %v", err)
	}
	if len(cueListIDs) != 1 {
		t.Errorf("Expected 1 cue list ID for look2, got %d", len(cueListIDs))
	}
	if len(cueListIDs) > 0 && cueListIDs[0] != cueList3.ID {
		t.Errorf("Expected cueList3.ID for look2, got %s", cueListIDs[0])
	}

	// Test: Find cue lists for non-existent look - should return empty slice
	nonExistentLookID := cuid.New()
	cueListIDs, err = repo.FindCueListIDsByLookID(ctx, nonExistentLookID)
	if err != nil {
		t.Fatalf("FindCueListIDsByLookID failed for non-existent look: %v", err)
	}
	if len(cueListIDs) != 0 {
		t.Errorf("Expected 0 cue list IDs for non-existent look, got %d", len(cueListIDs))
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

// TestFixtureRepository_CreateWithChannels_PresetIDs tests creating fixture with preset IDs.
func TestFixtureRepository_CreateWithChannels_PresetIDs(t *testing.T) {
	testDB, cleanup := setupTestDB(t)
	defer cleanup()

	repo := NewFixtureRepository(testDB.DB)
	ctx := context.Background()

	// Create project, definition
	project := &models.Project{ID: cuid.New(), Name: "P"}
	testDB.DB.Create(project)
	def := &models.FixtureDefinition{ID: cuid.New(), Manufacturer: "T", Model: "M", Type: "t"}
	testDB.DB.Create(def)

	// Create fixture with preset IDs
	fixtureID := cuid.New()
	channel1ID := cuid.New()
	channel2ID := cuid.New()
	fixture := &models.FixtureInstance{
		ID:           fixtureID, // Pre-set ID
		Name:         "Fixture with preset IDs",
		ProjectID:    project.ID,
		DefinitionID: def.ID,
		Universe:     1,
		StartChannel: 1,
	}
	channels := []models.InstanceChannel{
		{ID: channel1ID, Offset: 0, Name: "Dimmer", Type: "intensity"}, // Pre-set ID
		{ID: channel2ID, Offset: 1, Name: "Red", Type: "color"},        // Pre-set ID
	}

	err := repo.CreateWithChannels(ctx, fixture, channels)
	if err != nil {
		t.Fatalf("CreateWithChannels failed: %v", err)
	}

	// Verify preset IDs were used
	if fixture.ID != fixtureID {
		t.Errorf("Expected fixture ID %s, got %s", fixtureID, fixture.ID)
	}

	instanceChannels, _ := repo.GetInstanceChannels(ctx, fixture.ID)
	if len(instanceChannels) != 2 {
		t.Errorf("Expected 2 channels, got %d", len(instanceChannels))
	}
}

// TestFixtureRepository_CreateWithChannels_NoChannels tests creating fixture with no channels.
func TestFixtureRepository_CreateWithChannels_NoChannels(t *testing.T) {
	testDB, cleanup := setupTestDB(t)
	defer cleanup()

	repo := NewFixtureRepository(testDB.DB)
	ctx := context.Background()

	// Create project, definition
	project := &models.Project{ID: cuid.New(), Name: "P"}
	testDB.DB.Create(project)
	def := &models.FixtureDefinition{ID: cuid.New(), Manufacturer: "T", Model: "M", Type: "t"}
	testDB.DB.Create(def)

	// Create fixture with empty channels
	fixture := &models.FixtureInstance{
		Name:         "Fixture with no channels",
		ProjectID:    project.ID,
		DefinitionID: def.ID,
		Universe:     1,
		StartChannel: 1,
	}

	err := repo.CreateWithChannels(ctx, fixture, nil)
	if err != nil {
		t.Fatalf("CreateWithChannels with no channels failed: %v", err)
	}

	if fixture.ID == "" {
		t.Error("Expected fixture ID to be set")
	}

	// Verify no channels were created
	instanceChannels, _ := repo.GetInstanceChannels(ctx, fixture.ID)
	if len(instanceChannels) != 0 {
		t.Errorf("Expected 0 channels, got %d", len(instanceChannels))
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

// TestFixtureRepository_CreateDefinitionWithChannels_NoChannels tests creating definition with no channels.
func TestFixtureRepository_CreateDefinitionWithChannels_NoChannels(t *testing.T) {
	testDB, cleanup := setupTestDB(t)
	defer cleanup()

	repo := NewFixtureRepository(testDB.DB)
	ctx := context.Background()

	// Create definition with no channels
	def := &models.FixtureDefinition{
		Manufacturer: "Test",
		Model:        "NoChannels",
		Type:         "dimmer",
	}

	err := repo.CreateDefinitionWithChannels(ctx, def, nil)
	if err != nil {
		t.Fatalf("CreateDefinitionWithChannels with no channels failed: %v", err)
	}

	if def.ID == "" {
		t.Error("Expected definition ID to be set")
	}

	// Verify no channels were created
	defChannels, _ := repo.GetDefinitionChannels(ctx, def.ID)
	if len(defChannels) != 0 {
		t.Errorf("Expected 0 channels, got %d", len(defChannels))
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

// TestFixtureRepository_GetDefinitionModes tests getting modes for a definition.
func TestFixtureRepository_GetDefinitionModes(t *testing.T) {
	testDB, cleanup := setupTestDB(t)
	defer cleanup()

	repo := NewFixtureRepository(testDB.DB)
	ctx := context.Background()

	// Create definition
	def := &models.FixtureDefinition{ID: cuid.New(), Manufacturer: "T", Model: "M", Type: "t"}
	testDB.DB.Create(def)

	// Test GetDefinitionModes with no modes (should return empty)
	modes, err := repo.GetDefinitionModes(ctx, def.ID)
	if err != nil {
		t.Fatalf("GetDefinitionModes failed: %v", err)
	}
	if len(modes) != 0 {
		t.Errorf("Expected 0 modes, got %d", len(modes))
	}

	// Create modes
	mode1 := &models.FixtureMode{ID: cuid.New(), Name: "3 Channel", ChannelCount: 3, DefinitionID: def.ID}
	mode2 := &models.FixtureMode{ID: cuid.New(), Name: "5 Channel", ChannelCount: 5, DefinitionID: def.ID}
	testDB.DB.Create(mode1)
	testDB.DB.Create(mode2)

	// Test GetDefinitionModes with modes
	modes, err = repo.GetDefinitionModes(ctx, def.ID)
	if err != nil {
		t.Fatalf("GetDefinitionModes failed: %v", err)
	}
	if len(modes) != 2 {
		t.Errorf("Expected 2 modes, got %d", len(modes))
	}

	// Verify modes are sorted by name
	if modes[0].Name != "3 Channel" {
		t.Errorf("Expected first mode name '3 Channel', got '%s'", modes[0].Name)
	}
	if modes[1].Name != "5 Channel" {
		t.Errorf("Expected second mode name '5 Channel', got '%s'", modes[1].Name)
	}
}

// TestFixtureRepository_CreateMode tests creating a mode.
func TestFixtureRepository_CreateMode(t *testing.T) {
	testDB, cleanup := setupTestDB(t)
	defer cleanup()

	repo := NewFixtureRepository(testDB.DB)
	ctx := context.Background()

	// Create definition
	def := &models.FixtureDefinition{ID: cuid.New(), Manufacturer: "T", Model: "M", Type: "t"}
	testDB.DB.Create(def)

	// Test CreateMode without ID (should auto-generate)
	mode := &models.FixtureMode{
		Name:         "Standard",
		ChannelCount: 4,
		DefinitionID: def.ID,
	}
	err := repo.CreateMode(ctx, mode)
	if err != nil {
		t.Fatalf("CreateMode failed: %v", err)
	}
	if mode.ID == "" {
		t.Error("Expected mode ID to be auto-generated")
	}

	// Test CreateMode with ID
	customID := cuid.New()
	mode2 := &models.FixtureMode{
		ID:           customID,
		Name:         "Extended",
		ChannelCount: 8,
		DefinitionID: def.ID,
	}
	err = repo.CreateMode(ctx, mode2)
	if err != nil {
		t.Fatalf("CreateMode with ID failed: %v", err)
	}
	if mode2.ID != customID {
		t.Errorf("Expected mode ID '%s', got '%s'", customID, mode2.ID)
	}

	// Verify modes were created
	modes, _ := repo.GetDefinitionModes(ctx, def.ID)
	if len(modes) != 2 {
		t.Errorf("Expected 2 modes, got %d", len(modes))
	}
}

// TestFixtureRepository_CreateModeChannels tests creating mode channels.
func TestFixtureRepository_CreateModeChannels(t *testing.T) {
	testDB, cleanup := setupTestDB(t)
	defer cleanup()

	repo := NewFixtureRepository(testDB.DB)
	ctx := context.Background()

	// Create definition, channels, and mode
	def := &models.FixtureDefinition{ID: cuid.New(), Manufacturer: "T", Model: "M", Type: "t"}
	testDB.DB.Create(def)

	ch1 := &models.ChannelDefinition{ID: cuid.New(), Name: "Red", Type: "color", Offset: 0, DefinitionID: def.ID}
	ch2 := &models.ChannelDefinition{ID: cuid.New(), Name: "Green", Type: "color", Offset: 1, DefinitionID: def.ID}
	ch3 := &models.ChannelDefinition{ID: cuid.New(), Name: "Blue", Type: "color", Offset: 2, DefinitionID: def.ID}
	testDB.DB.Create(ch1)
	testDB.DB.Create(ch2)
	testDB.DB.Create(ch3)

	mode := &models.FixtureMode{ID: cuid.New(), Name: "RGB", ChannelCount: 3, DefinitionID: def.ID}
	testDB.DB.Create(mode)

	// Test CreateModeChannels with empty slice (should be no-op)
	err := repo.CreateModeChannels(ctx, []models.ModeChannel{})
	if err != nil {
		t.Errorf("CreateModeChannels with empty slice failed: %v", err)
	}

	// Test CreateModeChannels without IDs (should auto-generate)
	modeChannels := []models.ModeChannel{
		{ModeID: mode.ID, ChannelID: ch1.ID, Offset: 0},
		{ModeID: mode.ID, ChannelID: ch2.ID, Offset: 1},
		{ModeID: mode.ID, ChannelID: ch3.ID, Offset: 2},
	}
	err = repo.CreateModeChannels(ctx, modeChannels)
	if err != nil {
		t.Fatalf("CreateModeChannels failed: %v", err)
	}

	// Verify mode channels were created with IDs
	for i, mc := range modeChannels {
		if mc.ID == "" {
			t.Errorf("Mode channel %d: expected ID to be auto-generated", i)
		}
	}

	// Verify via GetModeChannels
	retrieved, err := repo.GetModeChannels(ctx, mode.ID)
	if err != nil {
		t.Fatalf("GetModeChannels failed: %v", err)
	}
	if len(retrieved) != 3 {
		t.Errorf("Expected 3 mode channels, got %d", len(retrieved))
	}
}

// TestFixtureRepository_DeleteDefinitionModes tests deleting modes for a definition.
func TestFixtureRepository_DeleteDefinitionModes(t *testing.T) {
	testDB, cleanup := setupTestDB(t)
	defer cleanup()

	repo := NewFixtureRepository(testDB.DB)
	ctx := context.Background()

	// Create definition
	def := &models.FixtureDefinition{ID: cuid.New(), Manufacturer: "T", Model: "M", Type: "t"}
	testDB.DB.Create(def)

	// Create channels
	ch1 := &models.ChannelDefinition{ID: cuid.New(), Name: "Red", Type: "color", Offset: 0, DefinitionID: def.ID}
	testDB.DB.Create(ch1)

	// Create modes
	mode1 := &models.FixtureMode{ID: cuid.New(), Name: "Mode 1", ChannelCount: 1, DefinitionID: def.ID}
	mode2 := &models.FixtureMode{ID: cuid.New(), Name: "Mode 2", ChannelCount: 1, DefinitionID: def.ID}
	testDB.DB.Create(mode1)
	testDB.DB.Create(mode2)

	// Create mode channels
	mc1 := &models.ModeChannel{ID: cuid.New(), ModeID: mode1.ID, ChannelID: ch1.ID, Offset: 0}
	mc2 := &models.ModeChannel{ID: cuid.New(), ModeID: mode2.ID, ChannelID: ch1.ID, Offset: 0}
	testDB.DB.Create(mc1)
	testDB.DB.Create(mc2)

	// Verify modes exist
	modes, _ := repo.GetDefinitionModes(ctx, def.ID)
	if len(modes) != 2 {
		t.Fatalf("Expected 2 modes before delete, got %d", len(modes))
	}

	// Delete modes
	err := repo.DeleteDefinitionModes(ctx, def.ID)
	if err != nil {
		t.Fatalf("DeleteDefinitionModes failed: %v", err)
	}

	// Verify modes are deleted
	modes, _ = repo.GetDefinitionModes(ctx, def.ID)
	if len(modes) != 0 {
		t.Errorf("Expected 0 modes after delete, got %d", len(modes))
	}

	// Verify mode channels are also deleted
	var modeChannelCount int64
	testDB.DB.Model(&models.ModeChannel{}).Count(&modeChannelCount)
	if modeChannelCount != 0 {
		t.Errorf("Expected 0 mode channels after delete, got %d", modeChannelCount)
	}
}

// TestFixtureRepository_DeleteDefinitionModes_NoModes tests delete with no modes.
func TestFixtureRepository_DeleteDefinitionModes_NoModes(t *testing.T) {
	testDB, cleanup := setupTestDB(t)
	defer cleanup()

	repo := NewFixtureRepository(testDB.DB)
	ctx := context.Background()

	// Create definition without modes
	def := &models.FixtureDefinition{ID: cuid.New(), Manufacturer: "T", Model: "M", Type: "t"}
	testDB.DB.Create(def)

	// Delete modes (should not error even with no modes)
	err := repo.DeleteDefinitionModes(ctx, def.ID)
	if err != nil {
		t.Fatalf("DeleteDefinitionModes failed: %v", err)
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

// TestNewLookBoardRepository tests the constructor.
func TestNewLookBoardRepository(t *testing.T) {
	testDB, cleanup := setupTestDB(t)
	defer cleanup()

	repo := NewLookBoardRepository(testDB.DB)
	if repo == nil {
		t.Fatal("Expected non-nil repository")
	}
	if repo.db != testDB.DB {
		t.Error("Expected db to be set")
	}
}

// TestLookBoardRepository_CRUD tests basic CRUD operations.
func TestLookBoardRepository_CRUD(t *testing.T) {
	testDB, cleanup := setupTestDB(t)
	defer cleanup()

	repo := NewLookBoardRepository(testDB.DB)
	ctx := context.Background()

	// Create project
	project := &models.Project{ID: cuid.New(), Name: "Test Project"}
	testDB.DB.Create(project)

	// Test Create without ID (should auto-generate)
	board := &models.LookBoard{
		Name:      "Test Board " + cuid.Slug(),
		ProjectID: project.ID,
	}
	err := repo.Create(ctx, board)
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}
	if board.ID == "" {
		t.Error("Expected board ID to be auto-generated")
	}

	// Test FindByID
	found, err := repo.FindByID(ctx, board.ID)
	if err != nil {
		t.Fatalf("FindByID failed: %v", err)
	}
	if found == nil {
		t.Fatal("Expected to find board")
	}
	if found.Name != board.Name {
		t.Errorf("Name mismatch: got %s, want %s", found.Name, board.Name)
	}

	// Test FindByProjectID
	boards, err := repo.FindByProjectID(ctx, project.ID)
	if err != nil {
		t.Fatalf("FindByProjectID failed: %v", err)
	}
	if len(boards) == 0 {
		t.Error("Expected at least one board")
	}

	// Test Update
	board.Name = "Updated Board Name"
	err = repo.Update(ctx, board)
	if err != nil {
		t.Fatalf("Update failed: %v", err)
	}
	found, _ = repo.FindByID(ctx, board.ID)
	if found.Name != "Updated Board Name" {
		t.Errorf("Update didn't persist: got %s", found.Name)
	}

	// Test Delete
	err = repo.Delete(ctx, board.ID)
	if err != nil {
		t.Fatalf("Delete failed: %v", err)
	}
	found, err = repo.FindByID(ctx, board.ID)
	if err != nil {
		t.Fatalf("FindByID after delete failed: %v", err)
	}
	if found != nil {
		t.Error("Expected board to be deleted")
	}
}

// TestLookBoardRepository_Create_WithID tests Create with pre-set ID.
func TestLookBoardRepository_Create_WithID(t *testing.T) {
	testDB, cleanup := setupTestDB(t)
	defer cleanup()

	repo := NewLookBoardRepository(testDB.DB)
	ctx := context.Background()

	// Create project
	project := &models.Project{ID: cuid.New(), Name: "Test Project"}
	testDB.DB.Create(project)

	customID := cuid.New()
	board := &models.LookBoard{
		ID:        customID,
		Name:      "Board with custom ID",
		ProjectID: project.ID,
	}
	err := repo.Create(ctx, board)
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}
	if board.ID != customID {
		t.Errorf("ID changed: got %s, want %s", board.ID, customID)
	}
}

// TestLookBoardRepository_FindByID_NotFound tests FindByID with non-existent ID.
func TestLookBoardRepository_FindByID_NotFound(t *testing.T) {
	testDB, cleanup := setupTestDB(t)
	defer cleanup()

	repo := NewLookBoardRepository(testDB.DB)
	ctx := context.Background()

	found, err := repo.FindByID(ctx, "nonexistent-id")
	if err != nil {
		t.Fatalf("FindByID failed: %v", err)
	}
	if found != nil {
		t.Error("Expected nil for non-existent board")
	}
}

// TestLookBoardRepository_FindByProjectID_EmptyResult tests FindByProjectID with no boards.
func TestLookBoardRepository_FindByProjectID_EmptyResult(t *testing.T) {
	testDB, cleanup := setupTestDB(t)
	defer cleanup()

	repo := NewLookBoardRepository(testDB.DB)
	ctx := context.Background()

	// Create project without boards
	project := &models.Project{ID: cuid.New(), Name: "Empty Project"}
	testDB.DB.Create(project)

	boards, err := repo.FindByProjectID(ctx, project.ID)
	if err != nil {
		t.Fatalf("FindByProjectID failed: %v", err)
	}
	if len(boards) != 0 {
		t.Errorf("Expected 0 boards, got %d", len(boards))
	}
}

// TestLookBoardRepository_ButtonOperations tests button-related operations.
func TestLookBoardRepository_ButtonOperations(t *testing.T) {
	testDB, cleanup := setupTestDB(t)
	defer cleanup()

	repo := NewLookBoardRepository(testDB.DB)
	ctx := context.Background()

	// Create project and look
	project := &models.Project{ID: cuid.New(), Name: "Test Project"}
	testDB.DB.Create(project)
	look := &models.Look{ID: cuid.New(), Name: "Test Look", ProjectID: project.ID}
	testDB.DB.Create(look)

	// Create board
	board := &models.LookBoard{ID: cuid.New(), Name: "Test Board", ProjectID: project.ID}
	testDB.DB.Create(board)

	// Test GetButtons with no buttons
	buttons, err := repo.GetButtons(ctx, board.ID)
	if err != nil {
		t.Fatalf("GetButtons failed: %v", err)
	}
	if len(buttons) != 0 {
		t.Errorf("Expected 0 buttons, got %d", len(buttons))
	}

	// Test CreateButton without ID (should auto-generate)
	button := &models.LookBoardButton{
		LookBoardID: board.ID,
		LookID:      look.ID,
		LayoutX:      100,
		LayoutY:      200,
	}
	err = repo.CreateButton(ctx, button)
	if err != nil {
		t.Fatalf("CreateButton failed: %v", err)
	}
	if button.ID == "" {
		t.Error("Expected button ID to be auto-generated")
	}

	// Test GetButtons with one button
	buttons, err = repo.GetButtons(ctx, board.ID)
	if err != nil {
		t.Fatalf("GetButtons failed: %v", err)
	}
	if len(buttons) != 1 {
		t.Errorf("Expected 1 button, got %d", len(buttons))
	}
	if buttons[0].LayoutX != 100 || buttons[0].LayoutY != 200 {
		t.Errorf("Button position mismatch: got (%d, %d)", buttons[0].LayoutX, buttons[0].LayoutY)
	}

	// Test DeleteButtons
	err = repo.DeleteButtons(ctx, board.ID)
	if err != nil {
		t.Fatalf("DeleteButtons failed: %v", err)
	}
	buttons, _ = repo.GetButtons(ctx, board.ID)
	if len(buttons) != 0 {
		t.Errorf("Expected 0 buttons after delete, got %d", len(buttons))
	}
}

// TestLookBoardRepository_CreateButton_WithID tests CreateButton with pre-set ID.
func TestLookBoardRepository_CreateButton_WithID(t *testing.T) {
	testDB, cleanup := setupTestDB(t)
	defer cleanup()

	repo := NewLookBoardRepository(testDB.DB)
	ctx := context.Background()

	// Create project, look, and board
	project := &models.Project{ID: cuid.New(), Name: "Test Project"}
	testDB.DB.Create(project)
	look := &models.Look{ID: cuid.New(), Name: "Test Look", ProjectID: project.ID}
	testDB.DB.Create(look)
	board := &models.LookBoard{ID: cuid.New(), Name: "Test Board", ProjectID: project.ID}
	testDB.DB.Create(board)

	customID := cuid.New()
	button := &models.LookBoardButton{
		ID:           customID,
		LookBoardID: board.ID,
		LookID:      look.ID,
		LayoutX:      50,
		LayoutY:      50,
	}
	err := repo.CreateButton(ctx, button)
	if err != nil {
		t.Fatalf("CreateButton failed: %v", err)
	}
	if button.ID != customID {
		t.Errorf("ID changed: got %s, want %s", button.ID, customID)
	}
}

// TestLookBoardRepository_CreateButtons tests bulk button creation.
func TestLookBoardRepository_CreateButtons(t *testing.T) {
	testDB, cleanup := setupTestDB(t)
	defer cleanup()

	repo := NewLookBoardRepository(testDB.DB)
	ctx := context.Background()

	// Create project, looks, and board
	project := &models.Project{ID: cuid.New(), Name: "Test Project"}
	testDB.DB.Create(project)
	look1 := &models.Look{ID: cuid.New(), Name: "Look 1", ProjectID: project.ID}
	look2 := &models.Look{ID: cuid.New(), Name: "Look 2", ProjectID: project.ID}
	look3 := &models.Look{ID: cuid.New(), Name: "Look 3", ProjectID: project.ID}
	testDB.DB.Create(look1)
	testDB.DB.Create(look2)
	testDB.DB.Create(look3)
	board := &models.LookBoard{ID: cuid.New(), Name: "Test Board", ProjectID: project.ID}
	testDB.DB.Create(board)

	// Test CreateButtons with empty slice (should be no-op)
	err := repo.CreateButtons(ctx, []models.LookBoardButton{})
	if err != nil {
		t.Errorf("CreateButtons with empty slice failed: %v", err)
	}

	// Test CreateButtons without IDs (should auto-generate)
	buttons := []models.LookBoardButton{
		{LookBoardID: board.ID, LookID: look1.ID, LayoutX: 0, LayoutY: 0},
		{LookBoardID: board.ID, LookID: look2.ID, LayoutX: 100, LayoutY: 0},
		{LookBoardID: board.ID, LookID: look3.ID, LayoutX: 200, LayoutY: 0},
	}
	err = repo.CreateButtons(ctx, buttons)
	if err != nil {
		t.Fatalf("CreateButtons failed: %v", err)
	}

	// Verify IDs were auto-generated
	for i, btn := range buttons {
		if btn.ID == "" {
			t.Errorf("Button %d: expected ID to be auto-generated", i)
		}
	}

	// Verify buttons were created
	retrievedButtons, _ := repo.GetButtons(ctx, board.ID)
	if len(retrievedButtons) != 3 {
		t.Errorf("Expected 3 buttons, got %d", len(retrievedButtons))
	}
}

// TestLookBoardRepository_CreateButtons_WithIDs tests bulk creation with pre-set IDs.
func TestLookBoardRepository_CreateButtons_WithIDs(t *testing.T) {
	testDB, cleanup := setupTestDB(t)
	defer cleanup()

	repo := NewLookBoardRepository(testDB.DB)
	ctx := context.Background()

	// Create project, look, and board
	project := &models.Project{ID: cuid.New(), Name: "Test Project"}
	testDB.DB.Create(project)
	look := &models.Look{ID: cuid.New(), Name: "Test Look", ProjectID: project.ID}
	testDB.DB.Create(look)
	board := &models.LookBoard{ID: cuid.New(), Name: "Test Board", ProjectID: project.ID}
	testDB.DB.Create(board)

	// Test CreateButtons with pre-set IDs
	id1 := cuid.New()
	id2 := cuid.New()
	buttons := []models.LookBoardButton{
		{ID: id1, LookBoardID: board.ID, LookID: look.ID, LayoutX: 0, LayoutY: 0},
		{ID: id2, LookBoardID: board.ID, LookID: look.ID, LayoutX: 100, LayoutY: 0},
	}
	err := repo.CreateButtons(ctx, buttons)
	if err != nil {
		t.Fatalf("CreateButtons failed: %v", err)
	}

	// Verify pre-set IDs were preserved
	if buttons[0].ID != id1 {
		t.Errorf("Button 0: expected ID %s, got %s", id1, buttons[0].ID)
	}
	if buttons[1].ID != id2 {
		t.Errorf("Button 1: expected ID %s, got %s", id2, buttons[1].ID)
	}
}

// TestLookBoardRepository_CreateWithButtons tests transactional board creation with buttons.
func TestLookBoardRepository_CreateWithButtons(t *testing.T) {
	testDB, cleanup := setupTestDB(t)
	defer cleanup()

	repo := NewLookBoardRepository(testDB.DB)
	ctx := context.Background()

	// Create project and looks
	project := &models.Project{ID: cuid.New(), Name: "Test Project"}
	testDB.DB.Create(project)
	look1 := &models.Look{ID: cuid.New(), Name: "Look 1", ProjectID: project.ID}
	look2 := &models.Look{ID: cuid.New(), Name: "Look 2", ProjectID: project.ID}
	testDB.DB.Create(look1)
	testDB.DB.Create(look2)

	// Test CreateWithButtons without IDs (should auto-generate)
	board := &models.LookBoard{
		Name:      "Board with buttons",
		ProjectID: project.ID,
	}
	buttons := []models.LookBoardButton{
		{LookID: look1.ID, LayoutX: 0, LayoutY: 0},
		{LookID: look2.ID, LayoutX: 100, LayoutY: 0},
	}

	err := repo.CreateWithButtons(ctx, board, buttons)
	if err != nil {
		t.Fatalf("CreateWithButtons failed: %v", err)
	}

	// Verify board ID was auto-generated
	if board.ID == "" {
		t.Error("Expected board ID to be auto-generated")
	}

	// Verify button IDs were auto-generated and LookBoardID was set
	for i, btn := range buttons {
		if btn.ID == "" {
			t.Errorf("Button %d: expected ID to be auto-generated", i)
		}
		if btn.LookBoardID != board.ID {
			t.Errorf("Button %d: expected LookBoardID %s, got %s", i, board.ID, btn.LookBoardID)
		}
	}

	// Verify board was created
	found, _ := repo.FindByID(ctx, board.ID)
	if found == nil {
		t.Fatal("Expected to find board after CreateWithButtons")
	}

	// Verify buttons were created
	retrievedButtons, _ := repo.GetButtons(ctx, board.ID)
	if len(retrievedButtons) != 2 {
		t.Errorf("Expected 2 buttons, got %d", len(retrievedButtons))
	}
}

// TestLookBoardRepository_CreateWithButtons_PresetIDs tests transactional creation with preset IDs.
func TestLookBoardRepository_CreateWithButtons_PresetIDs(t *testing.T) {
	testDB, cleanup := setupTestDB(t)
	defer cleanup()

	repo := NewLookBoardRepository(testDB.DB)
	ctx := context.Background()

	// Create project and look
	project := &models.Project{ID: cuid.New(), Name: "Test Project"}
	testDB.DB.Create(project)
	look := &models.Look{ID: cuid.New(), Name: "Test Look", ProjectID: project.ID}
	testDB.DB.Create(look)

	// Test CreateWithButtons with pre-set IDs
	boardID := cuid.New()
	buttonID := cuid.New()
	board := &models.LookBoard{
		ID:        boardID,
		Name:      "Board with preset IDs",
		ProjectID: project.ID,
	}
	buttons := []models.LookBoardButton{
		{ID: buttonID, LookID: look.ID, LayoutX: 50, LayoutY: 50},
	}

	err := repo.CreateWithButtons(ctx, board, buttons)
	if err != nil {
		t.Fatalf("CreateWithButtons failed: %v", err)
	}

	// Verify preset IDs were preserved
	if board.ID != boardID {
		t.Errorf("Board ID changed: got %s, want %s", board.ID, boardID)
	}
	if buttons[0].ID != buttonID {
		t.Errorf("Button ID changed: got %s, want %s", buttons[0].ID, buttonID)
	}
}

// TestLookBoardRepository_CreateWithButtons_NoButtons tests transactional creation with no buttons.
func TestLookBoardRepository_CreateWithButtons_NoButtons(t *testing.T) {
	testDB, cleanup := setupTestDB(t)
	defer cleanup()

	repo := NewLookBoardRepository(testDB.DB)
	ctx := context.Background()

	// Create project
	project := &models.Project{ID: cuid.New(), Name: "Test Project"}
	testDB.DB.Create(project)

	// Test CreateWithButtons with empty buttons slice
	board := &models.LookBoard{
		Name:      "Board with no buttons",
		ProjectID: project.ID,
	}

	err := repo.CreateWithButtons(ctx, board, nil)
	if err != nil {
		t.Fatalf("CreateWithButtons with no buttons failed: %v", err)
	}

	if board.ID == "" {
		t.Error("Expected board ID to be auto-generated")
	}

	// Verify board was created
	found, _ := repo.FindByID(ctx, board.ID)
	if found == nil {
		t.Fatal("Expected to find board after CreateWithButtons")
	}

	// Verify no buttons were created
	buttons, _ := repo.GetButtons(ctx, board.ID)
	if len(buttons) != 0 {
		t.Errorf("Expected 0 buttons, got %d", len(buttons))
	}
}

// TestLookBoardRepository_GetButtons_Ordering tests that buttons are ordered by position.
func TestLookBoardRepository_GetButtons_Ordering(t *testing.T) {
	testDB, cleanup := setupTestDB(t)
	defer cleanup()

	repo := NewLookBoardRepository(testDB.DB)
	ctx := context.Background()

	// Create project, look, and board
	project := &models.Project{ID: cuid.New(), Name: "Test Project"}
	testDB.DB.Create(project)
	look := &models.Look{ID: cuid.New(), Name: "Test Look", ProjectID: project.ID}
	testDB.DB.Create(look)
	board := &models.LookBoard{ID: cuid.New(), Name: "Test Board", ProjectID: project.ID}
	testDB.DB.Create(board)

	// Create buttons in non-ordered way
	// Buttons should be ordered by Y first, then X
	buttons := []models.LookBoardButton{
		{ID: cuid.New(), LookBoardID: board.ID, LookID: look.ID, LayoutX: 200, LayoutY: 100}, // 3rd (row 1, col 2)
		{ID: cuid.New(), LookBoardID: board.ID, LookID: look.ID, LayoutX: 100, LayoutY: 0},   // 2nd (row 0, col 1)
		{ID: cuid.New(), LookBoardID: board.ID, LookID: look.ID, LayoutX: 0, LayoutY: 0},     // 1st (row 0, col 0)
		{ID: cuid.New(), LookBoardID: board.ID, LookID: look.ID, LayoutX: 0, LayoutY: 100},   // 4th (row 1, col 0)
	}
	for _, btn := range buttons {
		testDB.DB.Create(&btn)
	}

	// Get buttons and verify ordering
	retrieved, err := repo.GetButtons(ctx, board.ID)
	if err != nil {
		t.Fatalf("GetButtons failed: %v", err)
	}
	if len(retrieved) != 4 {
		t.Fatalf("Expected 4 buttons, got %d", len(retrieved))
	}

	// Expected order: (0,0), (100,0), (0,100), (200,100)
	expectedOrder := []struct{ x, y int }{
		{0, 0},
		{100, 0},
		{0, 100},
		{200, 100},
	}
	for i, expected := range expectedOrder {
		if retrieved[i].LayoutX != expected.x || retrieved[i].LayoutY != expected.y {
			t.Errorf("Button %d: expected position (%d, %d), got (%d, %d)",
				i, expected.x, expected.y, retrieved[i].LayoutX, retrieved[i].LayoutY)
		}
	}
}

// TestLookBoardRepository_Delete_CascadesButtons tests that Delete removes associated buttons.
func TestLookBoardRepository_Delete_CascadesButtons(t *testing.T) {
	testDB, cleanup := setupTestDB(t)
	defer cleanup()

	repo := NewLookBoardRepository(testDB.DB)
	ctx := context.Background()

	// Create project and look
	project := &models.Project{ID: cuid.New(), Name: "Test Project"}
	testDB.DB.Create(project)
	look := &models.Look{ID: cuid.New(), Name: "Test Look", ProjectID: project.ID}
	testDB.DB.Create(look)

	// Create board with buttons
	board := &models.LookBoard{ID: cuid.New(), Name: "Test Board", ProjectID: project.ID}
	buttons := []models.LookBoardButton{
		{LookID: look.ID, LayoutX: 0, LayoutY: 0},
		{LookID: look.ID, LayoutX: 100, LayoutY: 0},
		{LookID: look.ID, LayoutX: 0, LayoutY: 100},
	}
	err := repo.CreateWithButtons(ctx, board, buttons)
	if err != nil {
		t.Fatalf("CreateWithButtons failed: %v", err)
	}

	// Verify buttons exist
	retrievedButtons, _ := repo.GetButtons(ctx, board.ID)
	if len(retrievedButtons) != 3 {
		t.Fatalf("Expected 3 buttons before delete, got %d", len(retrievedButtons))
	}

	// Count all buttons in DB before delete
	var buttonCountBefore int64
	testDB.DB.Model(&models.LookBoardButton{}).Count(&buttonCountBefore)
	if buttonCountBefore != 3 {
		t.Fatalf("Expected 3 buttons in DB before delete, got %d", buttonCountBefore)
	}

	// Delete the board
	err = repo.Delete(ctx, board.ID)
	if err != nil {
		t.Fatalf("Delete failed: %v", err)
	}

	// Verify board is deleted
	found, _ := repo.FindByID(ctx, board.ID)
	if found != nil {
		t.Error("Expected board to be deleted")
	}

	// Verify buttons are also deleted (cascade delete)
	var buttonCountAfter int64
	testDB.DB.Model(&models.LookBoardButton{}).Count(&buttonCountAfter)
	if buttonCountAfter != 0 {
		t.Errorf("Expected 0 buttons in DB after cascade delete, got %d", buttonCountAfter)
	}
}

func TestLookBoardRepository_FindButtonByID(t *testing.T) {
	testDB, cleanup := setupTestDB(t)
	defer cleanup()
	ctx := context.Background()
	repo := NewLookBoardRepository(testDB.DB)

	// Create project and look
	project := &models.Project{ID: cuid.New(), Name: "Test Project"}
	testDB.DB.Create(project)
	look := &models.Look{ID: cuid.New(), Name: "Test Look", ProjectID: project.ID}
	testDB.DB.Create(look)

	// Create board and button
	board := &models.LookBoard{ID: cuid.New(), Name: "Test Board", ProjectID: project.ID}
	_ = repo.Create(ctx, board)
	button := &models.LookBoardButton{LookBoardID: board.ID, LookID: look.ID, LayoutX: 100, LayoutY: 200}
	_ = repo.CreateButton(ctx, button)

	// Find by ID
	found, err := repo.FindButtonByID(ctx, button.ID)
	if err != nil {
		t.Fatalf("FindButtonByID failed: %v", err)
	}
	if found == nil {
		t.Fatal("Expected button to be found")
	}
	if found.LayoutX != 100 || found.LayoutY != 200 {
		t.Errorf("Button position mismatch: got (%d, %d)", found.LayoutX, found.LayoutY)
	}

	// Find non-existent button
	notFound, err := repo.FindButtonByID(ctx, "non-existent-id")
	if err != nil {
		t.Fatalf("FindButtonByID for non-existent should not error: %v", err)
	}
	if notFound != nil {
		t.Error("Expected nil for non-existent button")
	}
}

func TestLookBoardRepository_UpdateButton(t *testing.T) {
	testDB, cleanup := setupTestDB(t)
	defer cleanup()
	ctx := context.Background()
	repo := NewLookBoardRepository(testDB.DB)

	// Create project and look
	project := &models.Project{ID: cuid.New(), Name: "Test Project"}
	testDB.DB.Create(project)
	look := &models.Look{ID: cuid.New(), Name: "Test Look", ProjectID: project.ID}
	testDB.DB.Create(look)

	// Create board and button
	board := &models.LookBoard{ID: cuid.New(), Name: "Test Board", ProjectID: project.ID}
	_ = repo.Create(ctx, board)
	button := &models.LookBoardButton{LookBoardID: board.ID, LookID: look.ID, LayoutX: 100, LayoutY: 200}
	_ = repo.CreateButton(ctx, button)

	// Update button
	button.LayoutX = 300
	button.LayoutY = 400
	color := "#FF0000"
	button.Color = &color
	err := repo.UpdateButton(ctx, button)
	if err != nil {
		t.Fatalf("UpdateButton failed: %v", err)
	}

	// Verify update
	found, _ := repo.FindButtonByID(ctx, button.ID)
	if found.LayoutX != 300 || found.LayoutY != 400 {
		t.Errorf("Expected position (300, 400), got (%d, %d)", found.LayoutX, found.LayoutY)
	}
	if found.Color == nil || *found.Color != "#FF0000" {
		t.Errorf("Expected color #FF0000, got %v", found.Color)
	}
}

func TestLookBoardRepository_DeleteButton(t *testing.T) {
	testDB, cleanup := setupTestDB(t)
	defer cleanup()
	ctx := context.Background()
	repo := NewLookBoardRepository(testDB.DB)

	// Create project and look
	project := &models.Project{ID: cuid.New(), Name: "Test Project"}
	testDB.DB.Create(project)
	look := &models.Look{ID: cuid.New(), Name: "Test Look", ProjectID: project.ID}
	testDB.DB.Create(look)

	// Create board and button
	board := &models.LookBoard{ID: cuid.New(), Name: "Test Board", ProjectID: project.ID}
	_ = repo.Create(ctx, board)
	button := &models.LookBoardButton{LookBoardID: board.ID, LookID: look.ID, LayoutX: 100, LayoutY: 200}
	_ = repo.CreateButton(ctx, button)

	// Verify button exists
	found, _ := repo.FindButtonByID(ctx, button.ID)
	if found == nil {
		t.Fatal("Button should exist before delete")
	}

	// Delete button
	err := repo.DeleteButton(ctx, button.ID)
	if err != nil {
		t.Fatalf("DeleteButton failed: %v", err)
	}

	// Verify button is deleted
	deleted, _ := repo.FindButtonByID(ctx, button.ID)
	if deleted != nil {
		t.Error("Button should be deleted")
	}
}
