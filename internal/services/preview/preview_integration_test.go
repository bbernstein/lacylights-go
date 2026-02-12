package preview

import (
	"context"
	"fmt"
	"strings"
	"sync"
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
	dmxCfg := dmx.DefaultConfig()
	dmxCfg.Enabled = false
	dmxService := dmx.NewService(dmxCfg)

	// Create preview service
	previewService := NewService(testDB.FixtureRepo, testDB.LookRepo, dmxService)

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

// TestUpdateChannelValues_BulkUpdate tests bulk channel updates
func TestUpdateChannelValues_BulkUpdate(t *testing.T) {
	testDB, service, cleanup := setupPreviewTest(t)
	defer cleanup()

	ctx := context.Background()
	project, fixture := createTestProjectWithFixture(t, testDB)

	// Start a session
	session, err := service.StartSession(ctx, project.ID, nil)
	if err != nil {
		t.Fatalf("Failed to start session: %v", err)
	}

	// Prepare bulk updates for RGB channels
	updates := []ChannelUpdate{
		{FixtureID: fixture.ID, ChannelIndex: 0, Value: 255}, // Red
		{FixtureID: fixture.ID, ChannelIndex: 1, Value: 128}, // Green
		{FixtureID: fixture.ID, ChannelIndex: 2, Value: 64},  // Blue
	}

	// Execute bulk update
	success, err := service.UpdateChannelValues(ctx, session.ID, updates)
	if err != nil {
		t.Fatalf("UpdateChannelValues failed: %v", err)
	}
	if !success {
		t.Error("UpdateChannelValues should return true")
	}

	// Verify all channels were updated
	retrievedSession := service.GetSession(session.ID)
	if retrievedSession == nil {
		t.Fatal("Session not found after bulk update")
	}

	// Check each channel value
	expectedChannels := map[string]int{
		"1:1": 255, // Universe 1, Channel 1 (Red)
		"1:2": 128, // Universe 1, Channel 2 (Green)
		"1:3": 64,  // Universe 1, Channel 3 (Blue)
	}

	for key, expectedValue := range expectedChannels {
		if actualValue, exists := retrievedSession.ChannelOverrides[key]; !exists {
			t.Errorf("Channel %s not found in overrides", key)
		} else if actualValue != expectedValue {
			t.Errorf("Channel %s: expected %d, got %d", key, expectedValue, actualValue)
		}
	}
}

// TestUpdateChannelValues_ValueClamping tests value clamping in bulk updates
func TestUpdateChannelValues_ValueClamping(t *testing.T) {
	testDB, service, cleanup := setupPreviewTest(t)
	defer cleanup()

	ctx := context.Background()
	project, fixture := createTestProjectWithFixture(t, testDB)

	session, err := service.StartSession(ctx, project.ID, nil)
	if err != nil {
		t.Fatalf("Failed to start session: %v", err)
	}

	// Test values outside valid range
	updates := []ChannelUpdate{
		{FixtureID: fixture.ID, ChannelIndex: 0, Value: -50},  // Should clamp to 0
		{FixtureID: fixture.ID, ChannelIndex: 1, Value: 300},  // Should clamp to 255
		{FixtureID: fixture.ID, ChannelIndex: 2, Value: 128},  // Valid value
	}

	success, err := service.UpdateChannelValues(ctx, session.ID, updates)
	if err != nil {
		t.Fatalf("UpdateChannelValues failed: %v", err)
	}
	if !success {
		t.Error("UpdateChannelValues should return true")
	}

	// Verify clamping
	retrievedSession := service.GetSession(session.ID)
	if val, exists := retrievedSession.ChannelOverrides["1:1"]; !exists || val != 0 {
		t.Errorf("Expected channel 1:1 to be clamped to 0, got %d", val)
	}
	if val, exists := retrievedSession.ChannelOverrides["1:2"]; !exists || val != 255 {
		t.Errorf("Expected channel 1:2 to be clamped to 255, got %d", val)
	}
	if val, exists := retrievedSession.ChannelOverrides["1:3"]; !exists || val != 128 {
		t.Errorf("Expected channel 1:3 to be 128, got %d", val)
	}
}

// TestUpdateChannelValues_EmptyUpdates tests bulk update with empty slice
func TestUpdateChannelValues_EmptyUpdates(t *testing.T) {
	testDB, service, cleanup := setupPreviewTest(t)
	defer cleanup()

	ctx := context.Background()
	project, _ := createTestProjectWithFixture(t, testDB)

	session, err := service.StartSession(ctx, project.ID, nil)
	if err != nil {
		t.Fatalf("Failed to start session: %v", err)
	}

	// Test with empty updates
	updates := []ChannelUpdate{}
	success, err := service.UpdateChannelValues(ctx, session.ID, updates)
	if err != nil {
		t.Fatalf("UpdateChannelValues failed: %v", err)
	}
	if !success {
		t.Error("UpdateChannelValues should return true even with empty updates")
	}
}

// TestUpdateChannelValues_NonExistentSession tests bulk update on non-existent session
func TestUpdateChannelValues_NonExistentSession(t *testing.T) {
	testDB, service, cleanup := setupPreviewTest(t)
	defer cleanup()

	ctx := context.Background()
	_, fixture := createTestProjectWithFixture(t, testDB)

	updates := []ChannelUpdate{
		{FixtureID: fixture.ID, ChannelIndex: 0, Value: 255},
	}

	success, err := service.UpdateChannelValues(ctx, "non-existent-session", updates)
	if err != nil {
		t.Fatalf("UpdateChannelValues failed: %v", err)
	}
	if success {
		t.Error("UpdateChannelValues should return false for non-existent session")
	}
}

// TestUpdateChannelValues_SessionTimeout tests that bulk update resets session timeout
func TestUpdateChannelValues_SessionTimeout(t *testing.T) {
	testDB, service, cleanup := setupPreviewTest(t)
	defer cleanup()

	ctx := context.Background()
	project, fixture := createTestProjectWithFixture(t, testDB)

	session, err := service.StartSession(ctx, project.ID, nil)
	if err != nil {
		t.Fatalf("Failed to start session: %v", err)
	}

	initialTime := session.CreatedAt

	// Wait a bit
	time.Sleep(10 * time.Millisecond)

	// Perform bulk update
	updates := []ChannelUpdate{
		{FixtureID: fixture.ID, ChannelIndex: 0, Value: 255},
	}

	_, err = service.UpdateChannelValues(ctx, session.ID, updates)
	if err != nil {
		t.Fatalf("UpdateChannelValues failed: %v", err)
	}

	// Verify session timestamp was updated
	retrievedSession := service.GetSession(session.ID)
	if !retrievedSession.CreatedAt.After(initialTime) {
		t.Error("Session timestamp should be updated after bulk update")
	}
}

// TestUpdateChannelValues_NonExistentFixture tests bulk update with non-existent fixture
func TestUpdateChannelValues_NonExistentFixture(t *testing.T) {
	testDB, service, cleanup := setupPreviewTest(t)
	defer cleanup()

	ctx := context.Background()
	project, _ := createTestProjectWithFixture(t, testDB)

	session, err := service.StartSession(ctx, project.ID, nil)
	if err != nil {
		t.Fatalf("Failed to start session: %v", err)
	}

	updates := []ChannelUpdate{
		{FixtureID: "non-existent-fixture-id", ChannelIndex: 0, Value: 255},
	}

	success, err := service.UpdateChannelValues(ctx, session.ID, updates)
	if err == nil {
		t.Error("UpdateChannelValues should return error for non-existent fixture")
	}
	if success {
		t.Error("UpdateChannelValues should return false for non-existent fixture")
	}
	if err != nil && !strings.Contains(err.Error(), "fixture not found") {
		t.Errorf("Expected 'fixture not found' error, got: %v", err)
	}
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

// TestInitializeWithLook_Integration tests initializing session with look values.
func TestInitializeWithLook_Integration(t *testing.T) {
	testDB, service, cleanup := setupPreviewTest(t)
	defer cleanup()

	ctx := context.Background()
	project, fixture := createTestProjectWithFixture(t, testDB)

	// Create look with fixture values
	look := &models.Look{
		ID:        cuid.New(),
		ProjectID: project.ID,
		Name:      "Test Look",
	}
	testDB.DB.Create(look)

	fixtureValue := &models.FixtureValue{
		ID:        cuid.New(),
		LookID:    look.ID,
		FixtureID: fixture.ID,
		Channels:  `[{"offset":0,"value":255},{"offset":1,"value":128},{"offset":2,"value":64},{"offset":3,"value":32}]`,
	}
	testDB.DB.Create(fixtureValue)

	// Start session
	session, _ := service.StartSession(ctx, project.ID, nil)

	// Initialize with look
	success, err := service.InitializeWithLook(ctx, session.ID, look.ID)
	if err != nil {
		t.Fatalf("Failed to initialize with look: %v", err)
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

// TestInitializeWithLook_NonExistentSession tests initializing non-existent session.
func TestInitializeWithLook_NonExistentSession(t *testing.T) {
	testDB, service, cleanup := setupPreviewTest(t)
	defer cleanup()

	ctx := context.Background()
	project, _ := createTestProjectWithFixture(t, testDB)

	look := &models.Look{
		ID:        cuid.New(),
		ProjectID: project.ID,
		Name:      "Test Look",
	}
	testDB.DB.Create(look)

	success, err := service.InitializeWithLook(ctx, "nonexistent", look.ID)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	if success {
		t.Error("Expected initialize to fail for non-existent session")
	}
}

// TestInitializeWithLook_NonExistentLook tests initializing with non-existent look.
func TestInitializeWithLook_NonExistentLook(t *testing.T) {
	testDB, service, cleanup := setupPreviewTest(t)
	defer cleanup()

	ctx := context.Background()
	project, _ := createTestProjectWithFixture(t, testDB)

	session, _ := service.StartSession(ctx, project.ID, nil)

	success, err := service.InitializeWithLook(ctx, session.ID, "nonexistent-look")
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	if success {
		t.Error("Expected initialize to fail for non-existent look")
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

	// Set up callback with thread-safe access
	var mu sync.Mutex
	callbackCount := 0
	var lastSession *Session
	service.SetSessionUpdateCallback(func(session *Session, dmxOutput []DMXOutput) {
		mu.Lock()
		callbackCount++
		lastSession = session
		mu.Unlock()
	})

	// Start session should trigger callback
	session, _ := service.StartSession(ctx, project.ID, nil)
	time.Sleep(10 * time.Millisecond) // Let goroutine run

	mu.Lock()
	count := callbackCount
	mu.Unlock()
	if count == 0 {
		t.Error("Expected callback on start")
	}

	mu.Lock()
	initialCount := callbackCount
	mu.Unlock()

	// Update channel should trigger callback
	_, _ = service.UpdateChannelValue(ctx, session.ID, fixture.ID, 0, 128)
	time.Sleep(10 * time.Millisecond)

	mu.Lock()
	count = callbackCount
	mu.Unlock()
	if count <= initialCount {
		t.Error("Expected callback on update")
	}

	// Cancel should trigger callback
	_, _ = service.CancelSession(ctx, session.ID)
	time.Sleep(10 * time.Millisecond)

	mu.Lock()
	lastSess := lastSession
	mu.Unlock()
	if lastSess == nil || lastSess.IsActive {
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

// TestInitializeWithLook_MultipleFixtures tests initializing with look that has multiple fixtures.
func TestInitializeWithLook_MultipleFixtures(t *testing.T) {
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

	// Create look with both fixtures
	look := &models.Look{
		ID:        cuid.New(),
		ProjectID: project.ID,
		Name:      "Test Look",
	}
	testDB.DB.Create(look)

	fixtureValue1 := &models.FixtureValue{
		ID:        cuid.New(),
		LookID:    look.ID,
		FixtureID: fixture1.ID,
		Channels:  `[{"offset":0,"value":100},{"offset":1,"value":100},{"offset":2,"value":100}]`,
	}
	fixtureValue2 := &models.FixtureValue{
		ID:        cuid.New(),
		LookID:    look.ID,
		FixtureID: fixture2.ID,
		Channels:  `[{"offset":0,"value":200},{"offset":1,"value":200},{"offset":2,"value":200}]`,
	}
	testDB.DB.Create(fixtureValue1)
	testDB.DB.Create(fixtureValue2)

	// Start session and initialize
	session, _ := service.StartSession(ctx, project.ID, nil)
	_, _ = service.InitializeWithLook(ctx, session.ID, look.ID)

	// Check both fixtures' values
	session = service.GetSession(session.ID)
	if val := session.ChannelOverrides["1:1"]; val != 100 {
		t.Errorf("Fixture1 channel 1: expected 100, got %d", val)
	}
	if val := session.ChannelOverrides["1:10"]; val != 200 {
		t.Errorf("Fixture2 channel 10: expected 200, got %d", val)
	}
}

// TestCancelAllProjectSessions_Integration tests cancelling all sessions for a project.
func TestCancelAllProjectSessions_Integration(t *testing.T) {
	testDB, service, cleanup := setupPreviewTest(t)
	defer cleanup()

	ctx := context.Background()
	project, fixture := createTestProjectWithFixture(t, testDB)

	// Create multiple sessions by manually adding them (simulating multiple tabs/windows)
	// Note: In practice, StartSession cancels existing sessions, so we manually add them
	session1 := &Session{
		ID:               "session-1",
		ProjectID:        project.ID,
		IsActive:         true,
		ChannelOverrides: make(map[string]int),
	}
	session2 := &Session{
		ID:               "session-2",
		ProjectID:        project.ID,
		IsActive:         true,
		ChannelOverrides: make(map[string]int),
	}

	// Add channel overrides to both sessions
	session1.ChannelOverrides["1:1"] = 100
	session1.ChannelOverrides["1:2"] = 150
	session2.ChannelOverrides["1:3"] = 200

	service.mu.Lock()
	service.sessions["session-1"] = session1
	service.sessions["session-2"] = session2
	service.mu.Unlock()

	// Set up a callback to track notifications (set AFTER adding sessions)
	var mu sync.Mutex
	var callbackCount int
	var cancelledSessions []*Session
	var wg sync.WaitGroup
	wg.Add(2) // Expecting exactly 2 cancellation callbacks

	service.SetSessionUpdateCallback(func(session *Session, dmxOutput []DMXOutput) {
		mu.Lock()
		callbackCount++
		cancelledSessions = append(cancelledSessions, session)
		mu.Unlock()
		wg.Done()
	})

	// Cancel all sessions for the project
	service.CancelAllProjectSessions(ctx, project.ID)

	// Wait for async callbacks
	wg.Wait()

	// Verify all sessions were cancelled
	if service.GetSession("session-1") != nil {
		t.Error("Expected session-1 to be cancelled")
	}
	if service.GetSession("session-2") != nil {
		t.Error("Expected session-2 to be cancelled")
	}

	// Verify callbacks were triggered
	mu.Lock()
	if callbackCount != 2 {
		t.Errorf("Expected 2 callbacks, got %d", callbackCount)
	}
	for _, sess := range cancelledSessions {
		if sess.IsActive {
			t.Errorf("Expected session %s to be inactive in callback", sess.ID)
		}
	}
	mu.Unlock()

	// Verify no active sessions for project
	if service.GetProjectSession(project.ID) != nil {
		t.Error("Expected no active sessions for project")
	}

	// Clear callback for next part of test
	service.SetSessionUpdateCallback(nil)

	// Test with actual session that has set DMX overrides through the service
	session, err := service.StartSession(ctx, project.ID, nil)
	if err != nil {
		t.Fatalf("Failed to start session: %v", err)
	}

	// Update channel values through the service (this sets DMX overrides)
	_, _ = service.UpdateChannelValue(ctx, session.ID, fixture.ID, 0, 255)
	_, _ = service.UpdateChannelValue(ctx, session.ID, fixture.ID, 1, 128)

	// Verify the overrides are set
	session = service.GetSession(session.ID)
	if len(session.ChannelOverrides) == 0 {
		t.Error("Expected channel overrides to be set")
	}

	// Cancel all sessions
	service.CancelAllProjectSessions(ctx, project.ID)

	// Verify session is gone
	if service.GetSession(session.ID) != nil {
		t.Error("Expected session to be cancelled after CancelAllProjectSessions")
	}
}

// TestCancelAllProjectSessions_DoesNotAffectOtherProjects tests that cancelling
// sessions for one project doesn't affect sessions for other projects.
func TestCancelAllProjectSessions_DoesNotAffectOtherProjects(t *testing.T) {
	testDB, service, cleanup := setupPreviewTest(t)
	defer cleanup()

	ctx := context.Background()

	// Create two projects
	project1 := &models.Project{
		ID:   cuid.New(),
		Name: testutil.UniqueProjectName("project1"),
	}
	project2 := &models.Project{
		ID:   cuid.New(),
		Name: testutil.UniqueProjectName("project2"),
	}
	testDB.DB.Create(project1)
	testDB.DB.Create(project2)

	// Create sessions for both projects
	session1, _ := service.StartSession(ctx, project1.ID, nil)
	session2, _ := service.StartSession(ctx, project2.ID, nil)

	session1ID := session1.ID
	session2ID := session2.ID

	// Cancel sessions for project1 only
	service.CancelAllProjectSessions(ctx, project1.ID)

	// Project1 session should be gone
	if service.GetSession(session1ID) != nil {
		t.Error("Expected project1 session to be cancelled")
	}

	// Project2 session should still exist
	if service.GetSession(session2ID) == nil {
		t.Error("Expected project2 session to still exist")
	}
}

// Helper is unused, fmt is imported at top
