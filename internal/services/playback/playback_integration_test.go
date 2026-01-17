package playback

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/bbernstein/lacylights-go/internal/database/models"
	"github.com/bbernstein/lacylights-go/internal/services/dmx"
	"github.com/bbernstein/lacylights-go/internal/services/modulator"
	"github.com/bbernstein/lacylights-go/internal/services/testutil"
	"github.com/lucsky/cuid"
)

// setupPlaybackTest creates a test database and playback service.
func setupPlaybackTest(t *testing.T) (*testutil.TestDB, *Service, func()) {
	t.Helper()

	testDB, cleanupDB := testutil.SetupTestDB(t)

	// Create DMX service with Art-Net disabled for testing
	dmxCfg := dmx.DefaultConfig()
	dmxCfg.Enabled = false
	dmxService := dmx.NewService(dmxCfg)

	// Create fade engine (60Hz for testing)
	fadeEngine := modulator.NewEngine(dmxService, 60)
	_ = fadeEngine.Start()

	// Create playback service
	playbackService := NewService(testDB.DB, dmxService, fadeEngine)

	cleanup := func() {
		playbackService.Cleanup()
		_ = fadeEngine.Stop()
		dmxService.Stop()
		cleanupDB()
	}

	return testDB, playbackService, cleanup
}

// createTestProject creates a project for testing.
func createTestProject(t *testing.T, testDB *testutil.TestDB) *models.Project {
	t.Helper()

	project := &models.Project{
		ID:   cuid.New(),
		Name: testutil.UniqueProjectName("playback-test"),
	}
	if err := testDB.DB.Create(project).Error; err != nil {
		t.Fatalf("Failed to create test project: %v", err)
	}
	return project
}

// createTestFixtureWithLook creates a fixture definition, instance, look, and fixture values.
func createTestFixtureWithLook(t *testing.T, testDB *testutil.TestDB, project *models.Project) (*models.FixtureInstance, *models.Look) {
	t.Helper()

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

	// Create fixture mode
	fixtureMode := &models.FixtureMode{
		ID:           cuid.New(),
		DefinitionID: fixtureDef.ID,
		Name:         "Standard",
		ChannelCount: 4,
	}
	if err := testDB.DB.Create(fixtureMode).Error; err != nil {
		t.Fatalf("Failed to create fixture mode: %v", err)
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

	// Create look
	look := &models.Look{
		ID:        cuid.New(),
		ProjectID: project.ID,
		Name:      "Test Look",
	}
	if err := testDB.DB.Create(look).Error; err != nil {
		t.Fatalf("Failed to create look: %v", err)
	}

	// Create fixture value for look
	fixtureValue := &models.FixtureValue{
		ID:        cuid.New(),
		LookID:    look.ID,
		FixtureID: fixture.ID,
		Channels:  `[{"offset":0,"value":255},{"offset":1,"value":128},{"offset":2,"value":64},{"offset":3,"value":32}]`,
	}
	if err := testDB.DB.Create(fixtureValue).Error; err != nil {
		t.Fatalf("Failed to create fixture value: %v", err)
	}

	return fixture, look
}

// createTestCueList creates a cue list with cues.
func createTestCueList(t *testing.T, testDB *testutil.TestDB, project *models.Project, looks []*models.Look, loop bool) *models.CueList {
	t.Helper()

	cueList := &models.CueList{
		ID:        cuid.New(),
		ProjectID: project.ID,
		Name:      "Test Cue List",
		Loop:      loop,
	}
	if err := testDB.DB.Create(cueList).Error; err != nil {
		t.Fatalf("Failed to create cue list: %v", err)
	}

	for i, look := range looks {
		cue := &models.Cue{
			ID:          cuid.New(),
			CueListID:   cueList.ID,
			LookID:      look.ID,
			Name:        look.Name,
			CueNumber:   float64(i + 1),
			FadeInTime:  0.1, // Short fade times for testing
			FadeOutTime: 0.05,
		}
		if err := testDB.DB.Create(cue).Error; err != nil {
			t.Fatalf("Failed to create cue: %v", err)
		}
	}

	return cueList
}

// TestStartCueList_Integration tests starting a cue list with database.
func TestStartCueList_Integration(t *testing.T) {
	testDB, service, cleanup := setupPlaybackTest(t)
	defer cleanup()

	ctx := context.Background()
	project := createTestProject(t, testDB)
	_, look := createTestFixtureWithLook(t, testDB, project)
	cueList := createTestCueList(t, testDB, project, []*models.Look{look}, false)

	// Start cue list
	err := service.StartCueList(ctx, cueList.ID, nil, nil)
	if err != nil {
		t.Fatalf("Failed to start cue list: %v", err)
	}

	// Check state
	state := service.GetPlaybackState(cueList.ID)
	if state == nil {
		t.Fatal("Expected state to exist")
	}
	if !state.IsPlaying {
		t.Error("Expected IsPlaying to be true")
	}
	if !state.IsFading {
		t.Error("Expected IsFading to be true")
	}
	if state.CurrentCueIndex == nil || *state.CurrentCueIndex != 0 {
		t.Errorf("Expected CurrentCueIndex to be 0, got %v", state.CurrentCueIndex)
	}
}

// TestStartCueList_EmptyCueList tests that empty cue list returns error.
func TestStartCueList_EmptyCueList(t *testing.T) {
	testDB, service, cleanup := setupPlaybackTest(t)
	defer cleanup()

	ctx := context.Background()
	project := createTestProject(t, testDB)

	// Create empty cue list
	cueList := &models.CueList{
		ID:        cuid.New(),
		ProjectID: project.ID,
		Name:      "Empty Cue List",
	}
	if err := testDB.DB.Create(cueList).Error; err != nil {
		t.Fatalf("Failed to create cue list: %v", err)
	}

	// Try to start
	err := service.StartCueList(ctx, cueList.ID, nil, nil)
	if err == nil {
		t.Error("Expected error for empty cue list")
	}
}

// TestStartCueList_NonExistent tests that non-existent cue list returns error.
func TestStartCueList_NonExistent(t *testing.T) {
	_, service, cleanup := setupPlaybackTest(t)
	defer cleanup()

	ctx := context.Background()

	err := service.StartCueList(ctx, "nonexistent-id", nil, nil)
	if err == nil {
		t.Error("Expected error for non-existent cue list")
	}
}

// TestStartCueList_FromSpecificCue tests starting from a specific cue number.
func TestStartCueList_FromSpecificCue(t *testing.T) {
	testDB, service, cleanup := setupPlaybackTest(t)
	defer cleanup()

	ctx := context.Background()
	project := createTestProject(t, testDB)

	// Create two looks and cues
	_, look1 := createTestFixtureWithLook(t, testDB, project)
	look2 := &models.Look{
		ID:        cuid.New(),
		ProjectID: project.ID,
		Name:      "Look 2",
	}
	if err := testDB.DB.Create(look2).Error; err != nil {
		t.Fatalf("Failed to create look 2: %v", err)
	}

	cueList := createTestCueList(t, testDB, project, []*models.Look{look1, look2}, false)

	// Start from cue number 2
	startCue := 2.0
	err := service.StartCueList(ctx, cueList.ID, &startCue, nil)
	if err != nil {
		t.Fatalf("Failed to start cue list: %v", err)
	}

	// Verify we're at cue index 1 (second cue)
	state := service.GetPlaybackState(cueList.ID)
	if state.CurrentCueIndex == nil || *state.CurrentCueIndex != 1 {
		t.Errorf("Expected CurrentCueIndex to be 1, got %v", state.CurrentCueIndex)
	}
}

// TestNextCue_Integration tests navigating to next cue.
func TestNextCue_Integration(t *testing.T) {
	testDB, service, cleanup := setupPlaybackTest(t)
	defer cleanup()

	ctx := context.Background()
	project := createTestProject(t, testDB)

	// Create two looks
	_, look1 := createTestFixtureWithLook(t, testDB, project)
	look2 := &models.Look{
		ID:        cuid.New(),
		ProjectID: project.ID,
		Name:      "Look 2",
	}
	if err := testDB.DB.Create(look2).Error; err != nil {
		t.Fatalf("Failed to create look 2: %v", err)
	}

	cueList := createTestCueList(t, testDB, project, []*models.Look{look1, look2}, false)

	// Start cue list
	err := service.StartCueList(ctx, cueList.ID, nil, nil)
	if err != nil {
		t.Fatalf("Failed to start cue list: %v", err)
	}

	// Go to next
	err = service.NextCue(ctx, cueList.ID, nil)
	if err != nil {
		t.Fatalf("Failed to go to next cue: %v", err)
	}

	// Verify we're at cue index 1
	state := service.GetPlaybackState(cueList.ID)
	if state.CurrentCueIndex == nil || *state.CurrentCueIndex != 1 {
		t.Errorf("Expected CurrentCueIndex to be 1, got %v", state.CurrentCueIndex)
	}
}

// TestNextCue_AtEnd_NoLoop tests next cue at end without looping.
func TestNextCue_AtEnd_NoLoop(t *testing.T) {
	testDB, service, cleanup := setupPlaybackTest(t)
	defer cleanup()

	ctx := context.Background()
	project := createTestProject(t, testDB)
	_, look := createTestFixtureWithLook(t, testDB, project)
	cueList := createTestCueList(t, testDB, project, []*models.Look{look}, false)

	// Start cue list
	err := service.StartCueList(ctx, cueList.ID, nil, nil)
	if err != nil {
		t.Fatalf("Failed to start cue list: %v", err)
	}

	// Try to go to next (should fail - at end)
	err = service.NextCue(ctx, cueList.ID, nil)
	if err == nil {
		t.Error("Expected error when at end of non-looping cue list")
	}
}

// TestNextCue_AtEnd_WithLoop tests next cue at end with looping.
func TestNextCue_AtEnd_WithLoop(t *testing.T) {
	testDB, service, cleanup := setupPlaybackTest(t)
	defer cleanup()

	ctx := context.Background()
	project := createTestProject(t, testDB)
	_, look := createTestFixtureWithLook(t, testDB, project)
	cueList := createTestCueList(t, testDB, project, []*models.Look{look}, true) // Loop enabled

	// Start cue list
	err := service.StartCueList(ctx, cueList.ID, nil, nil)
	if err != nil {
		t.Fatalf("Failed to start cue list: %v", err)
	}

	// Go to next - should loop to 0
	err = service.NextCue(ctx, cueList.ID, nil)
	if err != nil {
		t.Fatalf("Failed to go to next cue with looping: %v", err)
	}

	state := service.GetPlaybackState(cueList.ID)
	if state.CurrentCueIndex == nil || *state.CurrentCueIndex != 0 {
		t.Errorf("Expected CurrentCueIndex to loop back to 0, got %v", state.CurrentCueIndex)
	}
}

// TestPreviousCue_Integration tests navigating to previous cue.
func TestPreviousCue_Integration(t *testing.T) {
	testDB, service, cleanup := setupPlaybackTest(t)
	defer cleanup()

	ctx := context.Background()
	project := createTestProject(t, testDB)

	// Create two looks
	_, look1 := createTestFixtureWithLook(t, testDB, project)
	look2 := &models.Look{
		ID:        cuid.New(),
		ProjectID: project.ID,
		Name:      "Look 2",
	}
	if err := testDB.DB.Create(look2).Error; err != nil {
		t.Fatalf("Failed to create look 2: %v", err)
	}

	cueList := createTestCueList(t, testDB, project, []*models.Look{look1, look2}, false)

	// Start from cue 2
	startCue := 2.0
	err := service.StartCueList(ctx, cueList.ID, &startCue, nil)
	if err != nil {
		t.Fatalf("Failed to start cue list: %v", err)
	}

	// Go to previous
	err = service.PreviousCue(ctx, cueList.ID, nil)
	if err != nil {
		t.Fatalf("Failed to go to previous cue: %v", err)
	}

	// Verify we're at cue index 0
	state := service.GetPlaybackState(cueList.ID)
	if state.CurrentCueIndex == nil || *state.CurrentCueIndex != 0 {
		t.Errorf("Expected CurrentCueIndex to be 0, got %v", state.CurrentCueIndex)
	}
}

// TestPreviousCue_AtStart_NoLoop tests previous cue at start without looping.
func TestPreviousCue_AtStart_NoLoop(t *testing.T) {
	testDB, service, cleanup := setupPlaybackTest(t)
	defer cleanup()

	ctx := context.Background()
	project := createTestProject(t, testDB)
	_, look := createTestFixtureWithLook(t, testDB, project)
	cueList := createTestCueList(t, testDB, project, []*models.Look{look}, false)

	// Start cue list
	err := service.StartCueList(ctx, cueList.ID, nil, nil)
	if err != nil {
		t.Fatalf("Failed to start cue list: %v", err)
	}

	// Try to go to previous (should fail - at start)
	err = service.PreviousCue(ctx, cueList.ID, nil)
	if err == nil {
		t.Error("Expected error when at start of non-looping cue list")
	}
}

// TestPreviousCue_AtStart_WithLoop tests previous cue at start with looping.
func TestPreviousCue_AtStart_WithLoop(t *testing.T) {
	testDB, service, cleanup := setupPlaybackTest(t)
	defer cleanup()

	ctx := context.Background()
	project := createTestProject(t, testDB)

	// Create two looks
	_, look1 := createTestFixtureWithLook(t, testDB, project)
	look2 := &models.Look{
		ID:        cuid.New(),
		ProjectID: project.ID,
		Name:      "Look 2",
	}
	if err := testDB.DB.Create(look2).Error; err != nil {
		t.Fatalf("Failed to create look 2: %v", err)
	}

	cueList := createTestCueList(t, testDB, project, []*models.Look{look1, look2}, true) // Loop enabled

	// Start at cue 1
	err := service.StartCueList(ctx, cueList.ID, nil, nil)
	if err != nil {
		t.Fatalf("Failed to start cue list: %v", err)
	}

	// Go to previous - should loop to last
	err = service.PreviousCue(ctx, cueList.ID, nil)
	if err != nil {
		t.Fatalf("Failed to go to previous cue with looping: %v", err)
	}

	state := service.GetPlaybackState(cueList.ID)
	if state.CurrentCueIndex == nil || *state.CurrentCueIndex != 1 {
		t.Errorf("Expected CurrentCueIndex to loop to last (1), got %v", state.CurrentCueIndex)
	}
}

// TestJumpToCue_Integration tests jumping to specific cue index.
func TestJumpToCue_Integration(t *testing.T) {
	testDB, service, cleanup := setupPlaybackTest(t)
	defer cleanup()

	ctx := context.Background()
	project := createTestProject(t, testDB)

	// Create three looks
	_, look1 := createTestFixtureWithLook(t, testDB, project)
	look2 := &models.Look{ID: cuid.New(), ProjectID: project.ID, Name: "Look 2"}
	look3 := &models.Look{ID: cuid.New(), ProjectID: project.ID, Name: "Look 3"}
	testDB.DB.Create(look2)
	testDB.DB.Create(look3)

	cueList := createTestCueList(t, testDB, project, []*models.Look{look1, look2, look3}, false)

	// Jump to cue index 2
	err := service.JumpToCue(ctx, cueList.ID, 2, nil)
	if err != nil {
		t.Fatalf("Failed to jump to cue: %v", err)
	}

	state := service.GetPlaybackState(cueList.ID)
	if state.CurrentCueIndex == nil || *state.CurrentCueIndex != 2 {
		t.Errorf("Expected CurrentCueIndex to be 2, got %v", state.CurrentCueIndex)
	}
}

// TestJumpToCue_InvalidIndex tests jumping to invalid cue index.
func TestJumpToCue_InvalidIndex(t *testing.T) {
	testDB, service, cleanup := setupPlaybackTest(t)
	defer cleanup()

	ctx := context.Background()
	project := createTestProject(t, testDB)
	_, look := createTestFixtureWithLook(t, testDB, project)
	cueList := createTestCueList(t, testDB, project, []*models.Look{look}, false)

	// Try invalid indices
	err := service.JumpToCue(ctx, cueList.ID, -1, nil)
	if err == nil {
		t.Error("Expected error for negative index")
	}

	err = service.JumpToCue(ctx, cueList.ID, 10, nil)
	if err == nil {
		t.Error("Expected error for out of bounds index")
	}
}

// TestGoToCueNumber_Integration tests jumping by cue number.
func TestGoToCueNumber_Integration(t *testing.T) {
	testDB, service, cleanup := setupPlaybackTest(t)
	defer cleanup()

	ctx := context.Background()
	project := createTestProject(t, testDB)

	// Create two looks
	_, look1 := createTestFixtureWithLook(t, testDB, project)
	look2 := &models.Look{ID: cuid.New(), ProjectID: project.ID, Name: "Look 2"}
	testDB.DB.Create(look2)

	cueList := createTestCueList(t, testDB, project, []*models.Look{look1, look2}, false)

	// Go to cue number 2.0
	err := service.GoToCueNumber(ctx, cueList.ID, 2.0, nil)
	if err != nil {
		t.Fatalf("Failed to go to cue number: %v", err)
	}

	state := service.GetPlaybackState(cueList.ID)
	if state.CurrentCue == nil || state.CurrentCue.CueNumber != 2.0 {
		t.Errorf("Expected cue number 2.0, got %v", state.CurrentCue)
	}
}

// TestGoToCueNumber_NotFound tests going to non-existent cue number.
func TestGoToCueNumber_NotFound(t *testing.T) {
	testDB, service, cleanup := setupPlaybackTest(t)
	defer cleanup()

	ctx := context.Background()
	project := createTestProject(t, testDB)
	_, look := createTestFixtureWithLook(t, testDB, project)
	cueList := createTestCueList(t, testDB, project, []*models.Look{look}, false)

	err := service.GoToCueNumber(ctx, cueList.ID, 99.0, nil)
	if err == nil {
		t.Error("Expected error for non-existent cue number")
	}
}

// TestGoToCueName_Integration tests jumping by cue name.
func TestGoToCueName_Integration(t *testing.T) {
	testDB, service, cleanup := setupPlaybackTest(t)
	defer cleanup()

	ctx := context.Background()
	project := createTestProject(t, testDB)
	_, look := createTestFixtureWithLook(t, testDB, project)
	cueList := createTestCueList(t, testDB, project, []*models.Look{look}, false)

	// The cue name is the look name "Test Look"
	err := service.GoToCueName(ctx, cueList.ID, "Test Look", nil)
	if err != nil {
		t.Fatalf("Failed to go to cue name: %v", err)
	}

	state := service.GetPlaybackState(cueList.ID)
	if state.CurrentCue == nil || state.CurrentCue.Name != "Test Look" {
		t.Errorf("Expected cue name 'Test Look', got %v", state.CurrentCue)
	}
}

// TestGoToCueName_NotFound tests going to non-existent cue name.
func TestGoToCueName_NotFound(t *testing.T) {
	testDB, service, cleanup := setupPlaybackTest(t)
	defer cleanup()

	ctx := context.Background()
	project := createTestProject(t, testDB)
	_, look := createTestFixtureWithLook(t, testDB, project)
	cueList := createTestCueList(t, testDB, project, []*models.Look{look}, false)

	err := service.GoToCueName(ctx, cueList.ID, "Nonexistent", nil)
	if err == nil {
		t.Error("Expected error for non-existent cue name")
	}
}

// TestExecuteCueDmx_Integration tests executing a cue's DMX output.
func TestExecuteCueDmx_Integration(t *testing.T) {
	testDB, service, cleanup := setupPlaybackTest(t)
	defer cleanup()

	ctx := context.Background()
	project := createTestProject(t, testDB)
	_, look := createTestFixtureWithLook(t, testDB, project)
	cueList := createTestCueList(t, testDB, project, []*models.Look{look}, false)

	// Get the cue ID
	var cue models.Cue
	testDB.DB.First(&cue, "cue_list_id = ?", cueList.ID)

	// Execute DMX
	err := service.ExecuteCueDmx(ctx, cue.ID, nil)
	if err != nil {
		t.Fatalf("Failed to execute cue DMX: %v", err)
	}
}

// TestExecuteCueDmx_NonExistentCue tests executing non-existent cue.
func TestExecuteCueDmx_NonExistentCue(t *testing.T) {
	_, service, cleanup := setupPlaybackTest(t)
	defer cleanup()

	ctx := context.Background()

	err := service.ExecuteCueDmx(ctx, "nonexistent", nil)
	if err == nil {
		t.Error("Expected error for non-existent cue")
	}
}

// TestExecuteCueDmx_WithFadeOverride tests fade time override.
func TestExecuteCueDmx_WithFadeOverride(t *testing.T) {
	testDB, service, cleanup := setupPlaybackTest(t)
	defer cleanup()

	ctx := context.Background()
	project := createTestProject(t, testDB)
	_, look := createTestFixtureWithLook(t, testDB, project)
	cueList := createTestCueList(t, testDB, project, []*models.Look{look}, false)

	var cue models.Cue
	testDB.DB.First(&cue, "cue_list_id = ?", cueList.ID)

	// Execute with fade override
	fadeTime := 5.0
	err := service.ExecuteCueDmx(ctx, cue.ID, &fadeTime)
	if err != nil {
		t.Fatalf("Failed to execute cue DMX with override: %v", err)
	}
}

// TestStopCueList_Integration tests stopping cue list playback.
func TestStopCueList_Integration(t *testing.T) {
	testDB, service, cleanup := setupPlaybackTest(t)
	defer cleanup()

	ctx := context.Background()
	project := createTestProject(t, testDB)
	_, look := createTestFixtureWithLook(t, testDB, project)
	cueList := createTestCueList(t, testDB, project, []*models.Look{look}, false)

	// Start cue list
	err := service.StartCueList(ctx, cueList.ID, nil, nil)
	if err != nil {
		t.Fatalf("Failed to start cue list: %v", err)
	}

	// Verify playing
	state := service.GetPlaybackState(cueList.ID)
	if !state.IsPlaying {
		t.Error("Expected IsPlaying to be true")
	}

	// Stop
	service.StopCueList(cueList.ID)

	// Verify stopped
	state = service.GetPlaybackState(cueList.ID)
	if state.IsPlaying {
		t.Error("Expected IsPlaying to be false after stop")
	}
	if state.IsFading {
		t.Error("Expected IsFading to be false after stop")
	}
}

// TestMultipleCueListsPlayback tests multiple cue lists playing concurrently.
func TestMultipleCueListsPlayback(t *testing.T) {
	testDB, service, cleanup := setupPlaybackTest(t)
	defer cleanup()

	ctx := context.Background()
	project := createTestProject(t, testDB)
	_, look := createTestFixtureWithLook(t, testDB, project)

	// Create two cue lists
	cueList1 := createTestCueList(t, testDB, project, []*models.Look{look}, false)

	look2 := &models.Look{ID: cuid.New(), ProjectID: project.ID, Name: "Look 2"}
	testDB.DB.Create(look2)
	cueList2 := &models.CueList{ID: cuid.New(), ProjectID: project.ID, Name: "CL2"}
	testDB.DB.Create(cueList2)
	cue2 := &models.Cue{
		ID:         cuid.New(),
		CueListID:  cueList2.ID,
		LookID:     look2.ID,
		Name:       "Look 2",
		CueNumber:  1.0,
		FadeInTime: 0.1,
	}
	testDB.DB.Create(cue2)

	// Start cue list 1
	err := service.StartCueList(ctx, cueList1.ID, nil, nil)
	if err != nil {
		t.Fatalf("Failed to start cue list 1: %v", err)
	}

	// Cue list 1 should be playing
	state1 := service.GetPlaybackState(cueList1.ID)
	if !state1.IsPlaying {
		t.Error("Expected cue list 1 to be playing")
	}

	// Start cue list 2 - this should stop cue list 1
	err = service.StartCueList(ctx, cueList2.ID, nil, nil)
	if err != nil {
		t.Fatalf("Failed to start cue list 2: %v", err)
	}

	// Cue list 2 should be playing, cue list 1 should be stopped
	state1 = service.GetPlaybackState(cueList1.ID)
	state2 := service.GetPlaybackState(cueList2.ID)

	if state1.IsPlaying {
		t.Error("Expected cue list 1 to be stopped when cue list 2 started")
	}
	if !state2.IsPlaying {
		t.Error("Expected cue list 2 to be playing")
	}

	// Stop all
	service.StopAllCueLists()

	state1 = service.GetPlaybackState(cueList1.ID)
	state2 = service.GetPlaybackState(cueList2.ID)

	if state1.IsPlaying {
		t.Error("Expected cue list 1 to be stopped")
	}
	if state2.IsPlaying {
		t.Error("Expected cue list 2 to be stopped")
	}
}

// TestFadeProgressTracking tests that fade progress is tracked correctly.
func TestFadeProgressTracking(t *testing.T) {
	testDB, service, cleanup := setupPlaybackTest(t)
	defer cleanup()

	ctx := context.Background()
	project := createTestProject(t, testDB)
	_, look := createTestFixtureWithLook(t, testDB, project)

	// Create cue list with longer fade time
	cueList := &models.CueList{ID: cuid.New(), ProjectID: project.ID, Name: "Test CL"}
	testDB.DB.Create(cueList)

	cue := &models.Cue{
		ID:         cuid.New(),
		CueListID:  cueList.ID,
		LookID:     look.ID,
		Name:       "Test Cue",
		CueNumber:  1.0,
		FadeInTime: 0.5, // 500ms fade
	}
	testDB.DB.Create(cue)

	// Start
	err := service.StartCueList(ctx, cueList.ID, nil, nil)
	if err != nil {
		t.Fatalf("Failed to start cue list: %v", err)
	}

	// Wait a bit for progress to update
	time.Sleep(200 * time.Millisecond)

	// Check progress is between 0 and 100
	state := service.GetPlaybackState(cueList.ID)
	if state.FadeProgress <= 0 || state.FadeProgress > 100 {
		t.Errorf("Expected FadeProgress between 0 and 100, got %f", state.FadeProgress)
	}

	// Wait for fade to complete
	time.Sleep(400 * time.Millisecond)

	state = service.GetPlaybackState(cueList.ID)
	if state.FadeProgress != 100.0 {
		t.Errorf("Expected FadeProgress to be 100 after fade, got %f", state.FadeProgress)
	}
}

// TestUpdateCallback_Integration tests that update callbacks are triggered.
func TestUpdateCallback_Integration(t *testing.T) {
	testDB, service, cleanup := setupPlaybackTest(t)
	defer cleanup()

	ctx := context.Background()
	project := createTestProject(t, testDB)
	_, look := createTestFixtureWithLook(t, testDB, project)
	cueList := createTestCueList(t, testDB, project, []*models.Look{look}, false)

	// Set up callback with thread-safe access
	var mu sync.Mutex
	callbackCount := 0
	service.SetUpdateCallback(func(status *CueListPlaybackStatus) {
		mu.Lock()
		callbackCount++
		mu.Unlock()
	})

	// Start cue list
	err := service.StartCueList(ctx, cueList.ID, nil, nil)
	if err != nil {
		t.Fatalf("Failed to start cue list: %v", err)
	}

	// Wait for callbacks
	time.Sleep(150 * time.Millisecond)

	mu.Lock()
	count := callbackCount
	mu.Unlock()

	if count == 0 {
		t.Error("Expected callbacks to be triggered")
	}
}

// TestExecuteCueDmx_CueWithNoLook tests executing a cue without a look.
func TestExecuteCueDmx_CueWithNoLook(t *testing.T) {
	testDB, service, cleanup := setupPlaybackTest(t)
	defer cleanup()

	ctx := context.Background()
	project := createTestProject(t, testDB)

	// Create cue list
	cueList := &models.CueList{ID: cuid.New(), ProjectID: project.ID, Name: "Test CL"}
	testDB.DB.Create(cueList)

	// Create cue without a valid look
	cue := &models.Cue{
		ID:         cuid.New(),
		CueListID:  cueList.ID,
		LookID:     "nonexistent-look",
		Name:       "Test Cue",
		CueNumber:  1.0,
		FadeInTime: 0.1,
	}
	testDB.DB.Create(cue)

	// Execute should fail because look doesn't exist (cue.Look will be nil after preload)
	err := service.ExecuteCueDmx(ctx, cue.ID, nil)
	if err == nil {
		t.Error("Expected error for cue with no look")
	}
}

// TestGetFormattedStatus_Integration tests formatted status with actual playback.
func TestGetFormattedStatus_Integration(t *testing.T) {
	testDB, service, cleanup := setupPlaybackTest(t)
	defer cleanup()

	ctx := context.Background()
	project := createTestProject(t, testDB)
	_, look := createTestFixtureWithLook(t, testDB, project)
	cueList := createTestCueList(t, testDB, project, []*models.Look{look}, false)

	// Before starting - should show not playing
	status := service.GetFormattedStatus(cueList.ID)
	if status.IsPlaying {
		t.Error("Expected IsPlaying to be false before starting")
	}

	// Start
	err := service.StartCueList(ctx, cueList.ID, nil, nil)
	if err != nil {
		t.Fatalf("Failed to start cue list: %v", err)
	}

	// Check formatted status
	status = service.GetFormattedStatus(cueList.ID)
	if !status.IsPlaying {
		t.Error("Expected IsPlaying to be true")
	}
	if status.CueListID != cueList.ID {
		t.Errorf("Expected CueListID %s, got %s", cueList.ID, status.CueListID)
	}
	if status.CurrentCue == nil {
		t.Error("Expected CurrentCue to be set")
	}
	if status.LastUpdated == "" {
		t.Error("Expected LastUpdated to be set")
	}
}

// TestStartCueList_WithFadeOverride tests starting with fade time override.
func TestStartCueList_WithFadeOverride(t *testing.T) {
	testDB, service, cleanup := setupPlaybackTest(t)
	defer cleanup()

	ctx := context.Background()
	project := createTestProject(t, testDB)
	_, look := createTestFixtureWithLook(t, testDB, project)
	cueList := createTestCueList(t, testDB, project, []*models.Look{look}, false)

	// Start with custom fade time
	fadeOverride := 0.2
	err := service.StartCueList(ctx, cueList.ID, nil, &fadeOverride)
	if err != nil {
		t.Fatalf("Failed to start cue list with fade override: %v", err)
	}

	// Check that the cue has the override fade time
	state := service.GetPlaybackState(cueList.ID)
	if state.CurrentCue == nil {
		t.Fatal("Expected CurrentCue to be set")
	}
	if state.CurrentCue.FadeInTime != fadeOverride {
		t.Errorf("Expected FadeInTime %f, got %f", fadeOverride, state.CurrentCue.FadeInTime)
	}
}

// TestNextCue_WithFadeOverride tests next cue with fade override.
func TestNextCue_WithFadeOverride(t *testing.T) {
	testDB, service, cleanup := setupPlaybackTest(t)
	defer cleanup()

	ctx := context.Background()
	project := createTestProject(t, testDB)
	_, look1 := createTestFixtureWithLook(t, testDB, project)
	look2 := &models.Look{ID: cuid.New(), ProjectID: project.ID, Name: "Look 2"}
	testDB.DB.Create(look2)
	cueList := createTestCueList(t, testDB, project, []*models.Look{look1, look2}, false)

	// Start
	err := service.StartCueList(ctx, cueList.ID, nil, nil)
	if err != nil {
		t.Fatalf("Failed to start cue list: %v", err)
	}

	// Next with override
	fadeOverride := 0.3
	err = service.NextCue(ctx, cueList.ID, &fadeOverride)
	if err != nil {
		t.Fatalf("Failed to go to next cue with fade override: %v", err)
	}

	state := service.GetPlaybackState(cueList.ID)
	if state.CurrentCueIndex == nil || *state.CurrentCueIndex != 1 {
		t.Errorf("Expected to be at cue index 1, got %v", state.CurrentCueIndex)
	}
}

// TestGoToCueNumber_WithFadeOverride tests go to cue number with fade override.
func TestGoToCueNumber_WithFadeOverride(t *testing.T) {
	testDB, service, cleanup := setupPlaybackTest(t)
	defer cleanup()

	ctx := context.Background()
	project := createTestProject(t, testDB)
	_, look := createTestFixtureWithLook(t, testDB, project)
	cueList := createTestCueList(t, testDB, project, []*models.Look{look}, false)

	fadeOverride := 0.5
	err := service.GoToCueNumber(ctx, cueList.ID, 1.0, &fadeOverride)
	if err != nil {
		t.Fatalf("Failed to go to cue number with fade override: %v", err)
	}

	state := service.GetPlaybackState(cueList.ID)
	if !state.IsPlaying {
		t.Error("Expected IsPlaying to be true")
	}
}

// TestExecuteCueDmx_WithEasingType tests cue with custom easing type.
func TestExecuteCueDmx_WithEasingType(t *testing.T) {
	testDB, service, cleanup := setupPlaybackTest(t)
	defer cleanup()

	ctx := context.Background()
	project := createTestProject(t, testDB)
	_, look := createTestFixtureWithLook(t, testDB, project)

	// Create cue list and cue with easing type
	cueList := &models.CueList{ID: cuid.New(), ProjectID: project.ID, Name: "Test CL"}
	testDB.DB.Create(cueList)

	easingType := "linear"
	cue := &models.Cue{
		ID:         cuid.New(),
		CueListID:  cueList.ID,
		LookID:     look.ID,
		Name:       "Test Cue",
		CueNumber:  1.0,
		FadeInTime: 0.1,
		EasingType: &easingType,
	}
	testDB.DB.Create(cue)

	// Execute DMX - should use the easing type
	err := service.ExecuteCueDmx(ctx, cue.ID, nil)
	if err != nil {
		t.Fatalf("Failed to execute cue DMX with easing: %v", err)
	}
}

// TestExecuteCueDmx_InvalidChannelsJSON tests handling of invalid channels JSON.
func TestExecuteCueDmx_InvalidChannelsJSON(t *testing.T) {
	testDB, service, cleanup := setupPlaybackTest(t)
	defer cleanup()

	ctx := context.Background()
	project := createTestProject(t, testDB)

	// Create fixture definition
	fixtureDef := &models.FixtureDefinition{
		ID:           cuid.New(),
		Manufacturer: "Test",
		Model:        testutil.UniqueFixtureName("test-invalid-json"),
		Type:         "dimmer",
	}
	testDB.DB.Create(fixtureDef)

	// Create fixture instance
	fixture := &models.FixtureInstance{
		ID:           cuid.New(),
		ProjectID:    project.ID,
		DefinitionID: fixtureDef.ID,
		Name:         testutil.UniqueFixtureName("fixture-invalid"),
		Universe:     1,
		StartChannel: 1,
	}
	testDB.DB.Create(fixture)

	// Create look
	look := &models.Look{
		ID:        cuid.New(),
		ProjectID: project.ID,
		Name:      "Look with invalid JSON",
	}
	testDB.DB.Create(look)

	// Create fixture value with invalid channels JSON
	fixtureValue := &models.FixtureValue{
		ID:        cuid.New(),
		LookID:    look.ID,
		FixtureID: fixture.ID,
		Channels:  `not-valid-json`,
	}
	testDB.DB.Create(fixtureValue)

	// Create cue list and cue
	cueList := &models.CueList{ID: cuid.New(), ProjectID: project.ID, Name: "Test CL"}
	testDB.DB.Create(cueList)

	cue := &models.Cue{
		ID:         cuid.New(),
		CueListID:  cueList.ID,
		LookID:     look.ID,
		Name:       "Test Cue",
		CueNumber:  1.0,
		FadeInTime: 0.1,
	}
	testDB.DB.Create(cue)

	// Execute DMX - should not error, but should skip the invalid fixture value
	err := service.ExecuteCueDmx(ctx, cue.ID, nil)
	if err != nil {
		t.Fatalf("ExecuteCueDmx should not fail for invalid channels JSON: %v", err)
	}
}

// TestExecuteCueDmx_DMXChannelOutOfBounds tests handling of out-of-bounds DMX channels.
func TestExecuteCueDmx_DMXChannelOutOfBounds(t *testing.T) {
	testDB, service, cleanup := setupPlaybackTest(t)
	defer cleanup()

	ctx := context.Background()
	project := createTestProject(t, testDB)

	// Create fixture definition
	fixtureDef := &models.FixtureDefinition{
		ID:           cuid.New(),
		Manufacturer: "Test",
		Model:        testutil.UniqueFixtureName("test-oob-channel"),
		Type:         "dimmer",
	}
	testDB.DB.Create(fixtureDef)

	// Create fixture instance with high start channel that will go out of bounds
	fixture := &models.FixtureInstance{
		ID:           cuid.New(),
		ProjectID:    project.ID,
		DefinitionID: fixtureDef.ID,
		Name:         testutil.UniqueFixtureName("fixture-oob"),
		Universe:     1,
		StartChannel: 510, // This will cause channels with offset > 2 to exceed 512
	}
	testDB.DB.Create(fixture)

	// Create look
	look := &models.Look{
		ID:        cuid.New(),
		ProjectID: project.ID,
		Name:      "Look with OOB channels",
	}
	testDB.DB.Create(look)

	// Create fixture value with channels that will be out of bounds
	// StartChannel 510 + offset 5 = 515 > 512 (out of bounds)
	fixtureValue := &models.FixtureValue{
		ID:        cuid.New(),
		LookID:    look.ID,
		FixtureID: fixture.ID,
		Channels:  `[{"offset":0,"value":255},{"offset":5,"value":128}]`, // offset 5 will be OOB
	}
	testDB.DB.Create(fixtureValue)

	// Create cue list and cue
	cueList := &models.CueList{ID: cuid.New(), ProjectID: project.ID, Name: "Test CL"}
	testDB.DB.Create(cueList)

	cue := &models.Cue{
		ID:         cuid.New(),
		CueListID:  cueList.ID,
		LookID:     look.ID,
		Name:       "Test Cue",
		CueNumber:  1.0,
		FadeInTime: 0.1,
	}
	testDB.DB.Create(cue)

	// Execute DMX - should not error, but should skip out-of-bounds channels
	err := service.ExecuteCueDmx(ctx, cue.ID, nil)
	if err != nil {
		t.Fatalf("ExecuteCueDmx should not fail for out-of-bounds channels: %v", err)
	}
}

// TestStartCueList_StopsPreviousCueList tests that starting a second cue list stops the first one.
func TestStartCueList_StopsPreviousCueList(t *testing.T) {
	testDB, service, cleanup := setupPlaybackTest(t)
	defer cleanup()

	ctx := context.Background()
	project := createTestProject(t, testDB)

	// Create two looks
	_, look1 := createTestFixtureWithLook(t, testDB, project)
	look2 := &models.Look{
		ID:        cuid.New(),
		ProjectID: project.ID,
		Name:      "Look 2",
	}
	if err := testDB.DB.Create(look2).Error; err != nil {
		t.Fatalf("Failed to create look 2: %v", err)
	}

	// Create first cue list
	cueList1 := createTestCueList(t, testDB, project, []*models.Look{look1}, false)

	// Create second cue list
	cueList2 := &models.CueList{
		ID:        cuid.New(),
		ProjectID: project.ID,
		Name:      "Test Cue List 2",
	}
	if err := testDB.DB.Create(cueList2).Error; err != nil {
		t.Fatalf("Failed to create cue list 2: %v", err)
	}
	cue2 := &models.Cue{
		ID:          cuid.New(),
		CueListID:   cueList2.ID,
		LookID:      look2.ID,
		Name:        look2.Name,
		CueNumber:   1.0,
		FadeInTime:  0.1,
		FadeOutTime: 0.05,
	}
	if err := testDB.DB.Create(cue2).Error; err != nil {
		t.Fatalf("Failed to create cue 2: %v", err)
	}

	// Start first cue list
	err := service.StartCueList(ctx, cueList1.ID, nil, nil)
	if err != nil {
		t.Fatalf("Failed to start cue list 1: %v", err)
	}

	// Verify first cue list is playing
	state1 := service.GetPlaybackState(cueList1.ID)
	if state1 == nil {
		t.Fatal("Expected state for cue list 1 to exist")
	}
	if !state1.IsPlaying {
		t.Error("Expected cue list 1 to be playing")
	}

	// Start second cue list
	err = service.StartCueList(ctx, cueList2.ID, nil, nil)
	if err != nil {
		t.Fatalf("Failed to start cue list 2: %v", err)
	}

	// Verify second cue list is playing
	state2 := service.GetPlaybackState(cueList2.ID)
	if state2 == nil {
		t.Fatal("Expected state for cue list 2 to exist")
	}
	if !state2.IsPlaying {
		t.Error("Expected cue list 2 to be playing")
	}

	// Verify first cue list has been stopped
	state1After := service.GetPlaybackState(cueList1.ID)
	if state1After == nil {
		t.Fatal("Expected state for cue list 1 to still exist")
	}
	if state1After.IsPlaying {
		t.Error("Expected cue list 1 to be stopped after starting cue list 2")
	}
	if state1After.IsFading {
		t.Error("Expected cue list 1 to not be fading after starting cue list 2")
	}
}

// TestStartCueList_CrossProjectPlayback tests that cue lists from different projects can play simultaneously.
func TestStartCueList_CrossProjectPlayback(t *testing.T) {
	testDB, service, cleanup := setupPlaybackTest(t)
	defer cleanup()

	ctx := context.Background()

	// Create first project with a cue list
	project1 := createTestProject(t, testDB)
	_, look1 := createTestFixtureWithLook(t, testDB, project1)
	cueList1 := createTestCueList(t, testDB, project1, []*models.Look{look1}, false)

	// Create second project with a cue list
	project2 := &models.Project{
		ID:   cuid.New(),
		Name: testutil.UniqueProjectName("playback-test-2"),
	}
	if err := testDB.DB.Create(project2).Error; err != nil {
		t.Fatalf("Failed to create project 2: %v", err)
	}

	look2 := &models.Look{
		ID:        cuid.New(),
		ProjectID: project2.ID,
		Name:      "Look 2",
	}
	if err := testDB.DB.Create(look2).Error; err != nil {
		t.Fatalf("Failed to create look 2: %v", err)
	}

	cueList2 := &models.CueList{
		ID:        cuid.New(),
		ProjectID: project2.ID,
		Name:      "Test Cue List 2",
	}
	if err := testDB.DB.Create(cueList2).Error; err != nil {
		t.Fatalf("Failed to create cue list 2: %v", err)
	}
	cue2 := &models.Cue{
		ID:          cuid.New(),
		CueListID:   cueList2.ID,
		LookID:      look2.ID,
		Name:        look2.Name,
		CueNumber:   1.0,
		FadeInTime:  0.1,
		FadeOutTime: 0.05,
	}
	if err := testDB.DB.Create(cue2).Error; err != nil {
		t.Fatalf("Failed to create cue 2: %v", err)
	}

	// Start cue list from project 1
	err := service.StartCueList(ctx, cueList1.ID, nil, nil)
	if err != nil {
		t.Fatalf("Failed to start cue list 1: %v", err)
	}

	// Verify cue list 1 is playing
	state1 := service.GetPlaybackState(cueList1.ID)
	if state1 == nil {
		t.Fatal("Expected state for cue list 1 to exist")
	}
	if !state1.IsPlaying {
		t.Error("Expected cue list 1 to be playing")
	}

	// Start cue list from project 2 - this should NOT stop cue list 1 (different project)
	err = service.StartCueList(ctx, cueList2.ID, nil, nil)
	if err != nil {
		t.Fatalf("Failed to start cue list 2: %v", err)
	}

	// Verify cue list 2 is playing
	state2 := service.GetPlaybackState(cueList2.ID)
	if state2 == nil {
		t.Fatal("Expected state for cue list 2 to exist")
	}
	if !state2.IsPlaying {
		t.Error("Expected cue list 2 to be playing")
	}

	// Verify cue list 1 is STILL playing (different project, should not be stopped)
	state1After := service.GetPlaybackState(cueList1.ID)
	if state1After == nil {
		t.Fatal("Expected state for cue list 1 to still exist")
	}
	if !state1After.IsPlaying {
		t.Error("Expected cue list 1 to still be playing (different project)")
	}
}
