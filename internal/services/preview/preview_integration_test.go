package preview

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/bbernstein/lacylights-go/internal/database/models"
	"github.com/bbernstein/lacylights-go/internal/services/dmx"
	"github.com/bbernstein/lacylights-go/internal/services/testutil"
	"github.com/lucsky/cuid"
)

// setupPreviewTest creates a test database and preview service.
func setupPreviewTest(t *testing.T) (*testutil.TestDB, *Service, func()) {
	t.Helper()

	testDB, cleanupDB := testutil.SetupTestDB(t)

	// Create DMX service with Art-Net disabled for testing
	dmxService := dmx.NewService(dmx.Config{
		Enabled:          false,
		BroadcastAddr:    "255.255.255.255",
		Port:             6454,
		RefreshRateHz:    44,
		IdleRateHz:       1,
		HighRateDuration: 2 * time.Second,
	})

	// Create preview service
	previewService := NewService(testDB.FixtureRepo, testDB.SceneRepo, dmxService)

	cleanup := func() {
		dmxService.Stop()
		cleanupDB()
	}

	return testDB, previewService, cleanup
}

// createTestProjectWithFixture creates a project and fixture for testing.
func createTestProjectWithFixture(t *testing.T, testDB *testutil.TestDB) (*models.Project, *models.FixtureInstance) {
	t.Helper()

	project := &models.Project{
		ID:   cuid.New(),
		Name: testutil.UniqueProjectName("preview-test"),
	}
	if err := testDB.DB.Create(project).Error; err != nil {
		t.Fatalf("Failed to create test project: %v", err)
	}

	// Create fixture definition
	fixtureDef := &models.FixtureDefinition{
		ID:           cuid.New(),
		Manufacturer: "Test",
		Model:        testutil.UniqueFixtureName("par-can"),
		Type:         "dimmer",
	}
	if err := testDB.DB.Create(fixtureDef).Error; err != nil {
		t.Fatalf("Failed to create fixture definition: %v", err)
	}

	// Create fixture instance
	fixture := &models.FixtureInstance{
		ID:           cuid.New(),
		ProjectID:    project.ID,
		DefinitionID: fixtureDef.ID,
		Name:         testutil.UniqueFixtureName("fixture"),
		Universe:     1,
		StartChannel: 1,
	}
	if err := testDB.DB.Create(fixture).Error; err != nil {
		t.Fatalf("Failed to create fixture instance: %v", err)
	}

	return project, fixture
}

// TestStartSession_Integration tests starting a preview session.
func TestStartSession_Integration(t *testing.T) {
	testDB, service, cleanup := setupPreviewTest(t)
	defer cleanup()

	ctx := context.Background()
	project, _ := createTestProjectWithFixture(t, testDB)

	// Start session
	session, err := service.StartSession(ctx, project.ID, nil)
	if err != nil {
		t.Fatalf("Failed to start session: %v", err)
	}

	if session == nil {
		t.Fatal("Expected session to be created")
	}
	if session.ProjectID != project.ID {
		t.Errorf("Expected ProjectID %s, got %s", project.ID, session.ProjectID)
	}
	if !session.IsActive {
		t.Error("Expected session to be active")
	}
	if len(session.ChannelOverrides) != 0 {
		t.Error("Expected empty channel overrides initially")
	}
}

// TestStartSession_CancelsExisting tests that starting a new session cancels existing ones.
func TestStartSession_CancelsExisting(t *testing.T) {
	testDB, service, cleanup := setupPreviewTest(t)
	defer cleanup()

	ctx := context.Background()
	project, _ := createTestProjectWithFixture(t, testDB)

	// Start first session
	session1, err := service.StartSession(ctx, project.ID, nil)
	if err != nil {
		t.Fatalf("Failed to start first session: %v", err)
	}
	session1ID := session1.ID

	// Start second session
	session2, err := service.StartSession(ctx, project.ID, nil)
	if err != nil {
		t.Fatalf("Failed to start second session: %v", err)
	}

	// First session should be cancelled
	oldSession := service.GetSession(session1ID)
	if oldSession != nil {
		t.Error("Expected first session to be cancelled")
	}

	// Second session should be active
	if !session2.IsActive {
		t.Error("Expected second session to be active")
	}
}

// TestUpdateChannelValue_Integration tests updating channel values.
func TestUpdateChannelValue_Integration(t *testing.T) {
	testDB, service, cleanup := setupPreviewTest(t)
	defer cleanup()

	ctx := context.Background()
	project, fixture := createTestProjectWithFixture(t, testDB)

	// Start session
	session, err := service.StartSession(ctx, project.ID, nil)
	if err != nil {
		t.Fatalf("Failed to start session: %v", err)
	}

	// Update channel value
	success, err := service.UpdateChannelValue(ctx, session.ID, fixture.ID, 0, 128)
	if err != nil {
		t.Fatalf("Failed to update channel: %v", err)
	}
	if !success {
		t.Error("Expected update to succeed")
	}

	// Check that the override is stored
	session = service.GetSession(session.ID)
	channelKey := "1:1" // Universe 1, Channel 1
	if val, exists := session.ChannelOverrides[channelKey]; !exists || val != 128 {
		t.Errorf("Expected channel override 128, got %d (exists: %v)", val, exists)
	}
}

// TestUpdateChannelValue_ValueClamping tests that values are clamped to 0-255.
func TestUpdateChannelValue_ValueClamping(t *testing.T) {
	testDB, service, cleanup := setupPreviewTest(t)
	defer cleanup()

	ctx := context.Background()
	project, fixture := createTestProjectWithFixture(t, testDB)

	session, _ := service.StartSession(ctx, project.ID, nil)

	// Test negative value
	_, _ = service.UpdateChannelValue(ctx, session.ID, fixture.ID, 0, -50)
	session = service.GetSession(session.ID)
	if val := session.ChannelOverrides["1:1"]; val != 0 {
		t.Errorf("Expected negative value clamped to 0, got %d", val)
	}

	// Test value over 255
	_, _ = service.UpdateChannelValue(ctx, session.ID, fixture.ID, 0, 500)
	session = service.GetSession(session.ID)
	if val := session.ChannelOverrides["1:1"]; val != 255 {
		t.Errorf("Expected value clamped to 255, got %d", val)
	}
}

// TestUpdateChannelValue_NonExistentSession tests updating non-existent session.
func TestUpdateChannelValue_NonExistentSession(t *testing.T) {
	testDB, service, cleanup := setupPreviewTest(t)
	defer cleanup()

	ctx := context.Background()
	_, fixture := createTestProjectWithFixture(t, testDB)

	success, err := service.UpdateChannelValue(ctx, "nonexistent-session", fixture.ID, 0, 128)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	if success {
		t.Error("Expected update to fail for non-existent session")
	}
}

// TestUpdateChannelValue_NonExistentFixture tests updating with non-existent fixture.
func TestUpdateChannelValue_NonExistentFixture(t *testing.T) {
	testDB, service, cleanup := setupPreviewTest(t)
	defer cleanup()

	ctx := context.Background()
	project, _ := createTestProjectWithFixture(t, testDB)

	session, _ := service.StartSession(ctx, project.ID, nil)

	success, err := service.UpdateChannelValue(ctx, session.ID, "nonexistent-fixture", 0, 128)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	if success {
		t.Error("Expected update to fail for non-existent fixture")
	}
}

// TestCancelSession_Integration tests cancelling a session.
func TestCancelSession_Integration(t *testing.T) {
	testDB, service, cleanup := setupPreviewTest(t)
	defer cleanup()

	ctx := context.Background()
	project, _ := createTestProjectWithFixture(t, testDB)

	session, _ := service.StartSession(ctx, project.ID, nil)
	sessionID := session.ID

	// Cancel session
	success, err := service.CancelSession(ctx, sessionID)
	if err != nil {
		t.Fatalf("Failed to cancel session: %v", err)
	}
	if !success {
		t.Error("Expected cancel to succeed")
	}

	// Session should be gone
	if service.GetSession(sessionID) != nil {
		t.Error("Expected session to be removed after cancel")
	}
}

// TestCancelSession_NonExistent tests cancelling non-existent session.
func TestCancelSession_NonExistent(t *testing.T) {
	_, service, cleanup := setupPreviewTest(t)
	defer cleanup()

	ctx := context.Background()

	success, err := service.CancelSession(ctx, "nonexistent")
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	if success {
		t.Error("Expected cancel to return false for non-existent session")
	}
}

// TestCommitSession_Integration tests committing a session.
func TestCommitSession_Integration(t *testing.T) {
	testDB, service, cleanup := setupPreviewTest(t)
	defer cleanup()

	ctx := context.Background()
	project, _ := createTestProjectWithFixture(t, testDB)

	session, _ := service.StartSession(ctx, project.ID, nil)
	sessionID := session.ID

	// Commit session
	success, err := service.CommitSession(ctx, sessionID)
	if err != nil {
		t.Fatalf("Failed to commit session: %v", err)
	}
	if !success {
		t.Error("Expected commit to succeed")
	}

	// Session should be removed
	if service.GetSession(sessionID) != nil {
		t.Error("Expected session to be removed after commit")
	}
}

// TestInitializeWithScene_Integration tests initializing session with scene values.
func TestInitializeWithScene_Integration(t *testing.T) {
	testDB, service, cleanup := setupPreviewTest(t)
	defer cleanup()

	ctx := context.Background()
	project, fixture := createTestProjectWithFixture(t, testDB)

	// Create scene with fixture values
	scene := &models.Scene{
		ID:        cuid.New(),
		ProjectID: project.ID,
		Name:      "Test Scene",
	}
	testDB.DB.Create(scene)

	fixtureValue := &models.FixtureValue{
		ID:            cuid.New(),
		SceneID:       scene.ID,
		FixtureID:     fixture.ID,
		ChannelValues: "[255, 128, 64, 32]",
	}
	testDB.DB.Create(fixtureValue)

	// Start session
	session, _ := service.StartSession(ctx, project.ID, nil)

	// Initialize with scene
	success, err := service.InitializeWithScene(ctx, session.ID, scene.ID)
	if err != nil {
		t.Fatalf("Failed to initialize with scene: %v", err)
	}
	if !success {
		t.Error("Expected initialize to succeed")
	}

	// Check channel overrides
	session = service.GetSession(session.ID)
	if val := session.ChannelOverrides["1:1"]; val != 255 {
		t.Errorf("Expected channel 1:1 to be 255, got %d", val)
	}
	if val := session.ChannelOverrides["1:2"]; val != 128 {
		t.Errorf("Expected channel 1:2 to be 128, got %d", val)
	}
	if val := session.ChannelOverrides["1:3"]; val != 64 {
		t.Errorf("Expected channel 1:3 to be 64, got %d", val)
	}
	if val := session.ChannelOverrides["1:4"]; val != 32 {
		t.Errorf("Expected channel 1:4 to be 32, got %d", val)
	}
}

// TestInitializeWithScene_NonExistentSession tests initializing non-existent session.
func TestInitializeWithScene_NonExistentSession(t *testing.T) {
	testDB, service, cleanup := setupPreviewTest(t)
	defer cleanup()

	ctx := context.Background()
	project, _ := createTestProjectWithFixture(t, testDB)

	scene := &models.Scene{
		ID:        cuid.New(),
		ProjectID: project.ID,
		Name:      "Test Scene",
	}
	testDB.DB.Create(scene)

	success, err := service.InitializeWithScene(ctx, "nonexistent", scene.ID)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	if success {
		t.Error("Expected initialize to fail for non-existent session")
	}
}

// TestInitializeWithScene_NonExistentScene tests initializing with non-existent scene.
func TestInitializeWithScene_NonExistentScene(t *testing.T) {
	testDB, service, cleanup := setupPreviewTest(t)
	defer cleanup()

	ctx := context.Background()
	project, _ := createTestProjectWithFixture(t, testDB)

	session, _ := service.StartSession(ctx, project.ID, nil)

	success, err := service.InitializeWithScene(ctx, session.ID, "nonexistent-scene")
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	if success {
		t.Error("Expected initialize to fail for non-existent scene")
	}
}

// TestGetSession_Integration tests getting a session by ID.
func TestGetSession_Integration(t *testing.T) {
	testDB, service, cleanup := setupPreviewTest(t)
	defer cleanup()

	ctx := context.Background()
	project, _ := createTestProjectWithFixture(t, testDB)

	// Create session
	session, _ := service.StartSession(ctx, project.ID, nil)
	sessionID := session.ID

	// Get session
	retrieved := service.GetSession(sessionID)
	if retrieved == nil {
		t.Fatal("Expected to get session")
	}
	if retrieved.ID != sessionID {
		t.Errorf("Expected session ID %s, got %s", sessionID, retrieved.ID)
	}

	// Non-existent session
	if service.GetSession("nonexistent") != nil {
		t.Error("Expected nil for non-existent session")
	}
}

// TestGetProjectSession_Integration tests getting active session for project.
func TestGetProjectSession_Integration(t *testing.T) {
	testDB, service, cleanup := setupPreviewTest(t)
	defer cleanup()

	ctx := context.Background()
	project, _ := createTestProjectWithFixture(t, testDB)

	// No session initially
	if service.GetProjectSession(project.ID) != nil {
		t.Error("Expected no session initially")
	}

	// Start session
	session, _ := service.StartSession(ctx, project.ID, nil)

	// Should find session
	found := service.GetProjectSession(project.ID)
	if found == nil {
		t.Fatal("Expected to find session for project")
	}
	if found.ID != session.ID {
		t.Errorf("Expected session ID %s, got %s", session.ID, found.ID)
	}

	// Cancel session
	_, _ = service.CancelSession(ctx, session.ID)

	// Should not find session
	if service.GetProjectSession(project.ID) != nil {
		t.Error("Expected no session after cancel")
	}
}

// TestGetDMXOutput_Integration tests getting DMX output for a session.
func TestGetDMXOutput_Integration(t *testing.T) {
	testDB, service, cleanup := setupPreviewTest(t)
	defer cleanup()

	ctx := context.Background()
	project, fixture := createTestProjectWithFixture(t, testDB)

	session, _ := service.StartSession(ctx, project.ID, nil)

	// No output initially
	output := service.GetDMXOutput(session.ID)
	if len(output) != 0 {
		t.Errorf("Expected no DMX output initially, got %d universes", len(output))
	}

	// Add channel override
	_, _ = service.UpdateChannelValue(ctx, session.ID, fixture.ID, 0, 200)

	// Should have output now
	output = service.GetDMXOutput(session.ID)
	if len(output) == 0 {
		t.Fatal("Expected DMX output after update")
	}
	if output[0].Universe != 1 {
		t.Errorf("Expected universe 1, got %d", output[0].Universe)
	}
	if len(output[0].Channels) != 512 {
		t.Errorf("Expected 512 channels, got %d", len(output[0].Channels))
	}
	if output[0].Channels[0] != 200 {
		t.Errorf("Expected channel 0 value 200, got %d", output[0].Channels[0])
	}
}

// TestGetDMXOutput_NonExistentSession tests DMX output for non-existent session.
func TestGetDMXOutput_NonExistentSession(t *testing.T) {
	_, service, cleanup := setupPreviewTest(t)
	defer cleanup()

	output := service.GetDMXOutput("nonexistent")
	if output != nil {
		t.Error("Expected nil output for non-existent session")
	}
}

// TestSessionUpdateCallback tests that update callbacks are triggered.
func TestSessionUpdateCallback_Integration(t *testing.T) {
	testDB, service, cleanup := setupPreviewTest(t)
	defer cleanup()

	ctx := context.Background()
	project, fixture := createTestProjectWithFixture(t, testDB)

	// Set up callback
	callbackCount := 0
	var lastSession *Session
	service.SetSessionUpdateCallback(func(session *Session, dmxOutput []DMXOutput) {
		callbackCount++
		lastSession = session
	})

	// Start session should trigger callback
	session, _ := service.StartSession(ctx, project.ID, nil)
	time.Sleep(10 * time.Millisecond) // Let goroutine run

	if callbackCount == 0 {
		t.Error("Expected callback on start")
	}
	initialCount := callbackCount

	// Update channel should trigger callback
	_, _ = service.UpdateChannelValue(ctx, session.ID, fixture.ID, 0, 128)
	time.Sleep(10 * time.Millisecond)

	if callbackCount <= initialCount {
		t.Error("Expected callback on update")
	}

	// Cancel should trigger callback
	_, _ = service.CancelSession(ctx, session.ID)
	time.Sleep(10 * time.Millisecond)

	if lastSession == nil || lastSession.IsActive {
		t.Error("Expected callback with inactive session on cancel")
	}
}

// TestMultipleChannelUpdates tests updating multiple channels.
func TestMultipleChannelUpdates(t *testing.T) {
	testDB, service, cleanup := setupPreviewTest(t)
	defer cleanup()

	ctx := context.Background()
	project, fixture := createTestProjectWithFixture(t, testDB)

	session, _ := service.StartSession(ctx, project.ID, nil)

	// Update multiple channels
	for i := 0; i < 4; i++ {
		_, _ = service.UpdateChannelValue(ctx, session.ID, fixture.ID, i, (i+1)*50)
	}

	// Check all overrides
	session = service.GetSession(session.ID)
	expected := []int{50, 100, 150, 200}
	for i, exp := range expected {
		channelKey := fmt.Sprintf("1:%d", i+1)
		if val := session.ChannelOverrides[channelKey]; val != exp {
			t.Errorf("Channel %s: expected %d, got %d", channelKey, exp, val)
		}
	}
}

// TestSessionWithUserID tests session with user ID.
func TestSessionWithUserID(t *testing.T) {
	testDB, service, cleanup := setupPreviewTest(t)
	defer cleanup()

	ctx := context.Background()
	project, _ := createTestProjectWithFixture(t, testDB)

	userID := "user-123"
	session, err := service.StartSession(ctx, project.ID, &userID)
	if err != nil {
		t.Fatalf("Failed to start session: %v", err)
	}

	if session.UserID == nil || *session.UserID != userID {
		t.Errorf("Expected UserID %s, got %v", userID, session.UserID)
	}
}

// TestMultipleUniverses tests handling of multiple universes.
func TestMultipleUniverses(t *testing.T) {
	testDB, service, cleanup := setupPreviewTest(t)
	defer cleanup()

	ctx := context.Background()
	project, _ := createTestProjectWithFixture(t, testDB)

	// Create fixture in universe 2
	fixtureDef := &models.FixtureDefinition{
		ID:           cuid.New(),
		Manufacturer: "Test",
		Model:        testutil.UniqueFixtureName("fixture2"),
		Type:         "dimmer",
	}
	testDB.DB.Create(fixtureDef)

	fixture2 := &models.FixtureInstance{
		ID:           cuid.New(),
		ProjectID:    project.ID,
		DefinitionID: fixtureDef.ID,
		Name:         testutil.UniqueFixtureName("fixture2"),
		Universe:     2,
		StartChannel: 10,
	}
	testDB.DB.Create(fixture2)

	session, _ := service.StartSession(ctx, project.ID, nil)

	// Update channel in universe 2
	_, _ = service.UpdateChannelValue(ctx, session.ID, fixture2.ID, 0, 175)

	// Check DMX output includes universe 2
	output := service.GetDMXOutput(session.ID)
	found := false
	for _, o := range output {
		if o.Universe == 2 {
			found = true
			if o.Channels[9] != 175 { // Channel 10 is index 9
				t.Errorf("Expected channel 10 value 175, got %d", o.Channels[9])
			}
		}
	}
	if !found {
		t.Error("Expected universe 2 in output")
	}
}

// TestInitializeWithScene_MultipleFixtures tests initializing with scene that has multiple fixtures.
func TestInitializeWithScene_MultipleFixtures(t *testing.T) {
	testDB, service, cleanup := setupPreviewTest(t)
	defer cleanup()

	ctx := context.Background()
	project, fixture1 := createTestProjectWithFixture(t, testDB)

	// Create second fixture
	fixtureDef := &models.FixtureDefinition{
		ID:           cuid.New(),
		Manufacturer: "Test",
		Model:        testutil.UniqueFixtureName("fixture2"),
		Type:         "dimmer",
	}
	testDB.DB.Create(fixtureDef)

	fixture2 := &models.FixtureInstance{
		ID:           cuid.New(),
		ProjectID:    project.ID,
		DefinitionID: fixtureDef.ID,
		Name:         testutil.UniqueFixtureName("fixture2"),
		Universe:     1,
		StartChannel: 10,
	}
	testDB.DB.Create(fixture2)

	// Create scene with both fixtures
	scene := &models.Scene{
		ID:        cuid.New(),
		ProjectID: project.ID,
		Name:      "Test Scene",
	}
	testDB.DB.Create(scene)

	fixtureValue1 := &models.FixtureValue{
		ID:            cuid.New(),
		SceneID:       scene.ID,
		FixtureID:     fixture1.ID,
		ChannelValues: "[100, 100, 100]",
	}
	fixtureValue2 := &models.FixtureValue{
		ID:            cuid.New(),
		SceneID:       scene.ID,
		FixtureID:     fixture2.ID,
		ChannelValues: "[200, 200, 200]",
	}
	testDB.DB.Create(fixtureValue1)
	testDB.DB.Create(fixtureValue2)

	// Start session and initialize
	session, _ := service.StartSession(ctx, project.ID, nil)
	_, _ = service.InitializeWithScene(ctx, session.ID, scene.ID)

	// Check both fixtures' values
	session = service.GetSession(session.ID)
	if val := session.ChannelOverrides["1:1"]; val != 100 {
		t.Errorf("Fixture1 channel 1: expected 100, got %d", val)
	}
	if val := session.ChannelOverrides["1:10"]; val != 200 {
		t.Errorf("Fixture2 channel 10: expected 200, got %d", val)
	}
}

// Helper is unused, fmt is imported at top
