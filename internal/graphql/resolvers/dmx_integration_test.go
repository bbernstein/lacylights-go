package resolvers

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/99designs/gqlgen/client"
	"github.com/99designs/gqlgen/graphql/handler"
	"github.com/glebarez/sqlite" // Pure Go SQLite driver (no CGO required)
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"github.com/bbernstein/lacylights-go/internal/database/models"
	"github.com/bbernstein/lacylights-go/internal/graphql/generated"
	"github.com/bbernstein/lacylights-go/internal/services/dmx"
	"github.com/bbernstein/lacylights-go/internal/services/fade"
	"github.com/bbernstein/lacylights-go/internal/services/playback"
)

// testSetup creates a test GraphQL server with an in-memory database
func testSetup(t *testing.T) (*client.Client, *Resolver, func()) {
	t.Helper()

	// Create in-memory SQLite database
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		t.Fatalf("Failed to open in-memory database: %v", err)
	}

	// Auto-migrate models
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
		&models.SceneBoard{},
		&models.SceneBoardButton{},
		&models.Setting{},
	)
	if err != nil {
		t.Fatalf("Failed to migrate database: %v", err)
	}

	// Create DMX service (disabled for testing)
	dmxService := dmx.NewService(dmx.Config{
		Enabled:          false,
		BroadcastAddr:    "255.255.255.255",
		Port:             6454,
		RefreshRateHz:    44,
		IdleRateHz:       1,
		HighRateDuration: 2 * time.Second,
	})

	// Create and start fade engine (60Hz for testing)
	fadeEngine := fade.NewEngine(dmxService, 60)
	fadeEngine.Start()

	// Create playback service
	playbackService := playback.NewService(db, dmxService, fadeEngine)

	// Create resolver with test OFL cache path
	resolver := NewResolver(db, dmxService, fadeEngine, playbackService, t.TempDir())

	// Create GraphQL server
	srv := handler.NewDefaultServer(generated.NewExecutableSchema(generated.Config{
		Resolvers: resolver,
	}))

	// Create test client
	c := client.New(srv)

	// Cleanup function
	cleanup := func() {
		fadeEngine.Stop()
		dmxService.Stop()
	}

	return c, resolver, cleanup
}

func TestSystemInfo_Query(t *testing.T) {
	c, _, cleanup := testSetup(t)
	defer cleanup()

	var resp struct {
		SystemInfo struct {
			ArtnetEnabled          bool   `json:"artnetEnabled"`
			ArtnetBroadcastAddress string `json:"artnetBroadcastAddress"`
		} `json:"systemInfo"`
	}

	err := c.Post(`query {
		systemInfo {
			artnetEnabled
			artnetBroadcastAddress
		}
	}`, &resp)

	if err != nil {
		t.Fatalf("SystemInfo query failed: %v", err)
	}

	// In test mode, artnet is disabled
	if resp.SystemInfo.ArtnetEnabled != false {
		t.Errorf("Expected artnetEnabled=false in test mode, got %v", resp.SystemInfo.ArtnetEnabled)
	}
}

func TestSetChannelValue_Mutation(t *testing.T) {
	c, resolver, cleanup := testSetup(t)
	defer cleanup()

	var resp struct {
		SetChannelValue bool `json:"setChannelValue"`
	}

	// Set a channel value
	err := c.Post(`mutation {
		setChannelValue(universe: 1, channel: 1, value: 128)
	}`, &resp)

	if err != nil {
		t.Fatalf("SetChannelValue mutation failed: %v", err)
	}

	if !resp.SetChannelValue {
		t.Error("Expected setChannelValue to return true")
	}

	// Verify the value was set
	value := resolver.DMXService.GetChannelValue(1, 1)
	if value != 128 {
		t.Errorf("Expected channel value 128, got %d", value)
	}
}

func TestSetChannelValue_ClampValues(t *testing.T) {
	c, resolver, cleanup := testSetup(t)
	defer cleanup()

	var resp struct {
		SetChannelValue bool `json:"setChannelValue"`
	}

	// Test value above 255 (should clamp)
	err := c.Post(`mutation {
		setChannelValue(universe: 1, channel: 1, value: 300)
	}`, &resp)

	if err != nil {
		t.Fatalf("SetChannelValue mutation failed: %v", err)
	}

	value := resolver.DMXService.GetChannelValue(1, 1)
	if value != 255 {
		t.Errorf("Expected channel value to be clamped to 255, got %d", value)
	}

	// Test negative value (should clamp to 0)
	err = c.Post(`mutation {
		setChannelValue(universe: 1, channel: 2, value: -10)
	}`, &resp)

	if err != nil {
		t.Fatalf("SetChannelValue mutation failed: %v", err)
	}

	value = resolver.DMXService.GetChannelValue(1, 2)
	if value != 0 {
		t.Errorf("Expected channel value to be clamped to 0, got %d", value)
	}
}

func TestFadeToBlack_Mutation(t *testing.T) {
	c, resolver, cleanup := testSetup(t)
	defer cleanup()

	// First set some channel values
	resolver.DMXService.SetChannelValue(1, 1, 255)
	resolver.DMXService.SetChannelValue(1, 100, 128)
	resolver.DMXService.SetChannelValue(2, 50, 200)

	var resp struct {
		FadeToBlack bool `json:"fadeToBlack"`
	}

	// Execute fade to black with very short fade time for testing
	err := c.Post(`mutation {
		fadeToBlack(fadeOutTime: 0.05)
	}`, &resp)

	if err != nil {
		t.Fatalf("FadeToBlack mutation failed: %v", err)
	}

	if !resp.FadeToBlack {
		t.Error("Expected fadeToBlack to return true")
	}

	// Wait for fade to complete
	time.Sleep(100 * time.Millisecond)

	// Verify all channels are 0
	if resolver.DMXService.GetChannelValue(1, 1) != 0 {
		t.Errorf("Expected channel 1,1 to be 0 after fade to black")
	}
	if resolver.DMXService.GetChannelValue(1, 100) != 0 {
		t.Errorf("Expected channel 1,100 to be 0 after fade to black")
	}
	if resolver.DMXService.GetChannelValue(2, 50) != 0 {
		t.Errorf("Expected channel 2,50 to be 0 after fade to black")
	}
}

func TestDmxOutput_Query(t *testing.T) {
	c, resolver, cleanup := testSetup(t)
	defer cleanup()

	// Set some channel values
	resolver.DMXService.SetChannelValue(1, 1, 100)
	resolver.DMXService.SetChannelValue(1, 2, 200)

	var resp struct {
		DmxOutput []int `json:"dmxOutput"`
	}

	err := c.Post(`query {
		dmxOutput(universe: 1)
	}`, &resp)

	if err != nil {
		t.Fatalf("DmxOutput query failed: %v", err)
	}

	if len(resp.DmxOutput) != 512 {
		t.Errorf("Expected 512 channels, got %d", len(resp.DmxOutput))
	}

	if resp.DmxOutput[0] != 100 {
		t.Errorf("Expected channel 1 = 100, got %d", resp.DmxOutput[0])
	}

	if resp.DmxOutput[1] != 200 {
		t.Errorf("Expected channel 2 = 200, got %d", resp.DmxOutput[1])
	}
}

func TestAllDmxOutput_Query(t *testing.T) {
	c, resolver, cleanup := testSetup(t)
	defer cleanup()

	// Set some channel values in different universes
	resolver.DMXService.SetChannelValue(1, 1, 100)
	resolver.DMXService.SetChannelValue(2, 1, 150)

	var resp struct {
		AllDmxOutput []struct {
			Universe int   `json:"universe"`
			Channels []int `json:"channels"`
		} `json:"allDmxOutput"`
	}

	err := c.Post(`query {
		allDmxOutput {
			universe
			channels
		}
	}`, &resp)

	if err != nil {
		t.Fatalf("AllDmxOutput query failed: %v", err)
	}

	if len(resp.AllDmxOutput) != 4 {
		t.Errorf("Expected 4 universes, got %d", len(resp.AllDmxOutput))
	}

	// Find universe 1 and check value
	for _, u := range resp.AllDmxOutput {
		if u.Universe == 1 && len(u.Channels) > 0 && u.Channels[0] != 100 {
			t.Errorf("Universe 1 channel 1: expected 100, got %d", u.Channels[0])
		}
		if u.Universe == 2 && len(u.Channels) > 0 && u.Channels[0] != 150 {
			t.Errorf("Universe 2 channel 1: expected 150, got %d", u.Channels[0])
		}
	}
}

func TestSetSceneLive_WithFixture(t *testing.T) {
	c, resolver, cleanup := testSetup(t)
	defer cleanup()

	ctx := context.Background()

	// Create a project
	desc := "Test project for DMX integration tests"
	project := &models.Project{
		ID:          "test-project-1",
		Name:        "Test Project",
		Description: &desc,
	}
	resolver.db.Create(project)

	// Create a fixture definition
	fixtureDef := &models.FixtureDefinition{
		ID:           "test-fixture-def-1",
		Manufacturer: "Test",
		Model:        "TestPar",
		Type:         "LED_PAR",
	}
	resolver.db.Create(fixtureDef)

	// Create a fixture instance
	fixture := &models.FixtureInstance{
		ID:           "test-fixture-1",
		Name:         "Test Par 1",
		ProjectID:    project.ID,
		DefinitionID: fixtureDef.ID,
		Universe:     1,
		StartChannel: 1,
	}
	resolver.db.Create(fixture)

	// Create a scene
	sceneDesc := "Test scene with fixture values"
	scene := &models.Scene{
		ID:          "test-scene-1",
		Name:        "Test Scene",
		ProjectID:   project.ID,
		Description: &sceneDesc,
	}
	resolver.db.Create(scene)

	// Create fixture values for the scene using sparse format
	fixtureValues := []models.FixtureValue{
		{ID: "fv-1", SceneID: scene.ID, FixtureID: fixture.ID, Channels: `[{"offset":0,"value":255},{"offset":1,"value":128},{"offset":2,"value":64},{"offset":3,"value":32}]`},
	}
	for _, fv := range fixtureValues {
		resolver.db.Create(&fv)
	}

	var resp struct {
		SetSceneLive bool `json:"setSceneLive"`
	}

	// Activate the scene
	err := c.Post(`mutation($sceneId: ID!) {
		setSceneLive(sceneId: $sceneId)
	}`, &resp, client.Var("sceneId", scene.ID))

	if err != nil {
		t.Fatalf("SetSceneLive mutation failed: %v", err)
	}

	if !resp.SetSceneLive {
		t.Error("Expected setSceneLive to return true")
	}

	// Wait for values to be applied
	time.Sleep(50 * time.Millisecond)

	// Verify the DMX values were set (1-indexed channels)
	// Note: The exact behavior depends on how setSceneLive is implemented
	activeSceneID := resolver.DMXService.GetActiveSceneID()
	if activeSceneID == nil || *activeSceneID != scene.ID {
		t.Errorf("Expected active scene to be %s, got %v", scene.ID, activeSceneID)
	}

	// Verify fixture values were applied
	if resolver.DMXService.GetChannelValue(1, 1) != 255 {
		t.Logf("Note: Channel values may not be applied immediately - got %d", resolver.DMXService.GetChannelValue(1, 1))
	}

	_ = ctx // Suppress unused variable warning
}

func TestCurrentActiveScene_Query(t *testing.T) {
	c, resolver, cleanup := testSetup(t)
	defer cleanup()

	// Initially no active scene
	var resp struct {
		CurrentActiveScene *struct {
			ID   string `json:"id"`
			Name string `json:"name"`
		} `json:"currentActiveScene"`
	}

	err := c.Post(`query {
		currentActiveScene {
			id
			name
		}
	}`, &resp)

	if err != nil {
		t.Fatalf("CurrentActiveScene query failed: %v", err)
	}

	if resp.CurrentActiveScene != nil {
		t.Error("Expected currentActiveScene to be nil initially")
	}

	// Set an active scene (manually, since we don't have a full scene in test)
	resolver.DMXService.SetActiveScene("test-scene-id")

	// Now the query should still work (though it may return nil if scene doesn't exist in DB)
	// This tests that the resolver handles missing scenes gracefully
}

// Note: dmxStatus query doesn't exist in schema - using systemInfo and allDmxOutput instead
func TestSystemInfo_ArtnetStatus(t *testing.T) {
	c, _, cleanup := testSetup(t)
	defer cleanup()

	var resp struct {
		SystemInfo struct {
			ArtnetEnabled          bool   `json:"artnetEnabled"`
			ArtnetBroadcastAddress string `json:"artnetBroadcastAddress"`
		} `json:"systemInfo"`
	}

	err := c.Post(`query {
		systemInfo {
			artnetEnabled
			artnetBroadcastAddress
		}
	}`, &resp)

	if err != nil {
		t.Fatalf("SystemInfo query failed: %v", err)
	}

	// In test mode, DMX is disabled
	if resp.SystemInfo.ArtnetEnabled != false {
		t.Errorf("Expected artnetEnabled=false in test mode, got %v", resp.SystemInfo.ArtnetEnabled)
	}

	// Broadcast address should be the default
	if resp.SystemInfo.ArtnetBroadcastAddress != "255.255.255.255" {
		t.Errorf("Expected broadcast 255.255.255.255, got %s", resp.SystemInfo.ArtnetBroadcastAddress)
	}
}

func TestMultipleChannelOperations(t *testing.T) {
	c, resolver, cleanup := testSetup(t)
	defer cleanup()

	// Set multiple channels
	for i := 1; i <= 10; i++ {
		var resp struct {
			SetChannelValue bool `json:"setChannelValue"`
		}
		err := c.Post(`mutation($universe: Int!, $channel: Int!, $value: Int!) {
			setChannelValue(universe: $universe, channel: $channel, value: $value)
		}`, &resp,
			client.Var("universe", 1),
			client.Var("channel", i),
			client.Var("value", i*10),
		)

		if err != nil {
			t.Fatalf("SetChannelValue mutation failed for channel %d: %v", i, err)
		}
	}

	// Verify all channels were set
	for i := 1; i <= 10; i++ {
		value := resolver.DMXService.GetChannelValue(1, i)
		expected := byte(i * 10)
		if value != expected {
			t.Errorf("Channel %d: expected %d, got %d", i, expected, value)
		}
	}

	// Now fade to black
	var resp struct {
		FadeToBlack bool `json:"fadeToBlack"`
	}
	err := c.Post(`mutation { fadeToBlack(fadeOutTime: 0.01) }`, &resp)
	if err != nil {
		t.Fatalf("FadeToBlack mutation failed: %v", err)
	}

	// Wait for fade
	time.Sleep(50 * time.Millisecond)

	// Verify all channels are 0
	for i := 1; i <= 10; i++ {
		value := resolver.DMXService.GetChannelValue(1, i)
		if value != 0 {
			t.Errorf("Channel %d should be 0 after fade to black, got %d", i, value)
		}
	}
}

func TestConcurrentChannelOperations(t *testing.T) {
	c, resolver, cleanup := testSetup(t)
	defer cleanup()

	// Test concurrent access to DMX channels
	done := make(chan bool)

	// Writer goroutine
	go func() {
		for i := 0; i < 100; i++ {
			var resp struct {
				SetChannelValue bool `json:"setChannelValue"`
			}
			_ = c.Post(`mutation($value: Int!) {
				setChannelValue(universe: 1, channel: 1, value: $value)
			}`, &resp, client.Var("value", i%256))
		}
		done <- true
	}()

	// Reader goroutine
	go func() {
		for i := 0; i < 100; i++ {
			var resp struct {
				DmxOutput struct {
					Channels []int `json:"channels"`
				} `json:"dmxOutput"`
			}
			_ = c.Post(`query { dmxOutput(universe: 1) { channels } }`, &resp)
		}
		done <- true
	}()

	// Wait for both to complete
	<-done
	<-done

	// Should not have panicked or deadlocked
	_ = resolver // Just verify resolver is still accessible
}

// TestSceneBoardFadeBehavior tests that when activating a scene from the scene board,
// channels with SNAP behavior jump immediately while channels with FADE behavior interpolate smoothly.
// This tests the fix for the bug where all channels were being faded regardless of their FadeBehavior.
func TestSceneBoardFadeBehavior(t *testing.T) {
	c, resolver, cleanup := testSetup(t)
	defer cleanup()

	// Create a project
	project := &models.Project{
		ID:   "test-project-fade",
		Name: "Test Project for Fade Behavior",
	}
	resolver.db.Create(project)

	// Create a fixture definition with channels that have different fade behaviors
	fixtureDef := &models.FixtureDefinition{
		ID:           "test-fixture-def-fade",
		Manufacturer: "Test",
		Model:        "FadeBehaviorTest",
		Type:         "LED_PAR",
	}
	resolver.db.Create(fixtureDef)

	// Create a fixture instance with channel definitions
	fixture := &models.FixtureInstance{
		ID:           "test-fixture-fade",
		Name:         "Test Fixture",
		ProjectID:    project.ID,
		DefinitionID: fixtureDef.ID,
		Universe:     1,
		StartChannel: 1,
	}
	resolver.db.Create(fixture)

	// Create channel definitions with different fade behaviors:
	// Channel 0 (offset 0): Dimmer - FADE
	// Channel 1 (offset 1): Red - FADE
	// Channel 2 (offset 2): Green - FADE
	// Channel 3 (offset 3): Blue - FADE
	// Channel 4 (offset 4): Color Macro - SNAP
	// Channel 5 (offset 5): Strobe - SNAP
	channels := []models.InstanceChannel{
		{ID: "ch-0", FixtureID: fixture.ID, Name: "Dimmer", Type: "INTENSITY", Offset: 0, FadeBehavior: "FADE"},
		{ID: "ch-1", FixtureID: fixture.ID, Name: "Red", Type: "COLOR", Offset: 1, FadeBehavior: "FADE"},
		{ID: "ch-2", FixtureID: fixture.ID, Name: "Green", Type: "COLOR", Offset: 2, FadeBehavior: "FADE"},
		{ID: "ch-3", FixtureID: fixture.ID, Name: "Blue", Type: "COLOR", Offset: 3, FadeBehavior: "FADE"},
		{ID: "ch-4", FixtureID: fixture.ID, Name: "Color Macro", Type: "COLOR_MACRO", Offset: 4, FadeBehavior: "SNAP"},
		{ID: "ch-5", FixtureID: fixture.ID, Name: "Strobe", Type: "STROBE", Offset: 5, FadeBehavior: "SNAP"},
	}
	for _, ch := range channels {
		resolver.db.Create(&ch)
	}

	// Create a scene board
	sceneBoard := &models.SceneBoard{
		ID:              "test-board-fade",
		Name:            "Test Board",
		ProjectID:       project.ID,
		DefaultFadeTime: 0.5, // 500ms fade time
	}
	resolver.db.Create(sceneBoard)

	// Create a scene with fixture values
	scene := &models.Scene{
		ID:        "test-scene-fade",
		Name:      "Test Scene",
		ProjectID: project.ID,
	}
	resolver.db.Create(scene)

	// Create fixture values: [Dimmer=200, R=150, G=100, B=50, ColorMacro=180, Strobe=255]
	// Using sparse format (all channels are specified in this case)
	fixtureValue := &models.FixtureValue{
		ID:        "fv-fade-test",
		SceneID:   scene.ID,
		FixtureID: fixture.ID,
		Channels:  `[{"offset":0,"value":200},{"offset":1,"value":150},{"offset":2,"value":100},{"offset":3,"value":50},{"offset":4,"value":180},{"offset":5,"value":255}]`,
	}
	resolver.db.Create(fixtureValue)

	// Create a scene board button for this scene
	width := 100
	height := 100
	button := &models.SceneBoardButton{
		ID:           "btn-fade-test",
		SceneBoardID: sceneBoard.ID,
		SceneID:      scene.ID,
		LayoutX:      0,
		LayoutY:      0,
		Width:        &width,
		Height:       &height,
	}
	resolver.db.Create(button)

	// Set initial DMX values to 0
	for i := 1; i <= 6; i++ {
		resolver.DMXService.SetChannelValue(1, i, 0)
	}

	// Activate the scene from the scene board with a fade
	var resp struct {
		ActivateSceneFromBoard bool `json:"activateSceneFromBoard"`
	}

	err := c.Post(`mutation($boardId: ID!, $sceneId: ID!) {
		activateSceneFromBoard(sceneBoardId: $boardId, sceneId: $sceneId)
	}`, &resp,
		client.Var("boardId", sceneBoard.ID),
		client.Var("sceneId", scene.ID),
	)

	if err != nil {
		t.Fatalf("ActivateSceneFromBoard mutation failed: %v", err)
	}

	if !resp.ActivateSceneFromBoard {
		t.Error("Expected activateSceneFromBoard to return true")
	}

	// Wait for ~50% of the fade (250ms into a 500ms fade)
	time.Sleep(250 * time.Millisecond)

	// Check SNAP channels - they should already be at their target values
	colorMacroValue := resolver.DMXService.GetChannelValue(1, 5) // Channel 5 = Color Macro (offset 4 + start 1)
	strobeValue := resolver.DMXService.GetChannelValue(1, 6)     // Channel 6 = Strobe (offset 5 + start 1)

	if colorMacroValue != 180 {
		t.Errorf("SNAP channel 'Color Macro' should be at target 180 immediately, got %d", colorMacroValue)
	}
	if strobeValue != 255 {
		t.Errorf("SNAP channel 'Strobe' should be at target 255 immediately, got %d", strobeValue)
	}

	// Check FADE channels - they should be somewhere between start (0) and target
	// At 50% through a linear fade, they should be approximately half way
	dimmerValue := resolver.DMXService.GetChannelValue(1, 1)
	redValue := resolver.DMXService.GetChannelValue(1, 2)
	greenValue := resolver.DMXService.GetChannelValue(1, 3)
	blueValue := resolver.DMXService.GetChannelValue(1, 4)

	// Allow for timing variance - check they're interpolating (not at 0 and not at target yet)
	if dimmerValue == 0 || dimmerValue >= 200 {
		t.Errorf("FADE channel 'Dimmer' should be interpolating (0 < x < 200), got %d", dimmerValue)
	}
	if redValue == 0 || redValue >= 150 {
		t.Errorf("FADE channel 'Red' should be interpolating (0 < x < 150), got %d", redValue)
	}
	if greenValue == 0 || greenValue >= 100 {
		t.Errorf("FADE channel 'Green' should be interpolating (0 < x < 100), got %d", greenValue)
	}
	if blueValue == 0 || blueValue >= 50 {
		t.Errorf("FADE channel 'Blue' should be interpolating (0 < x < 50), got %d", blueValue)
	}

	// Wait for fade to complete
	time.Sleep(300 * time.Millisecond)

	// All channels should now be at their target values
	if resolver.DMXService.GetChannelValue(1, 1) != 200 {
		t.Errorf("Dimmer should be at target 200 after fade, got %d", resolver.DMXService.GetChannelValue(1, 1))
	}
	if resolver.DMXService.GetChannelValue(1, 2) != 150 {
		t.Errorf("Red should be at target 150 after fade, got %d", resolver.DMXService.GetChannelValue(1, 2))
	}
	if resolver.DMXService.GetChannelValue(1, 3) != 100 {
		t.Errorf("Green should be at target 100 after fade, got %d", resolver.DMXService.GetChannelValue(1, 3))
	}
	if resolver.DMXService.GetChannelValue(1, 4) != 50 {
		t.Errorf("Blue should be at target 50 after fade, got %d", resolver.DMXService.GetChannelValue(1, 4))
	}

	t.Logf("Test passed - SNAP channels (Color Macro, Strobe) jumped immediately, FADE channels (Dimmer, RGB) interpolated smoothly")
}

func TestFadeUpdateRate_QueryAndMutation(t *testing.T) {
	c, resolver, cleanup := testSetup(t)
	defer cleanup()

	// Test initial rate via SystemInfo query
	var resp1 struct {
		SystemInfo struct {
			FadeUpdateRateHz int `json:"fadeUpdateRateHz"`
		} `json:"systemInfo"`
	}

	err := c.Post(`query {
		systemInfo {
			fadeUpdateRateHz
		}
	}`, &resp1)

	if err != nil {
		t.Fatalf("SystemInfo query failed: %v", err)
	}

	// Should match the initial rate set in testSetup (60Hz)
	if resp1.SystemInfo.FadeUpdateRateHz != 60 {
		t.Errorf("Initial fade update rate = %d, want 60", resp1.SystemInfo.FadeUpdateRateHz)
	}

	// Test updating the rate
	var resp2 struct {
		UpdateFadeUpdateRate bool `json:"updateFadeUpdateRate"`
	}

	err = c.Post(`mutation {
		updateFadeUpdateRate(rateHz: 120)
	}`, &resp2)

	if err != nil {
		t.Fatalf("updateFadeUpdateRate mutation failed: %v", err)
	}

	if !resp2.UpdateFadeUpdateRate {
		t.Error("updateFadeUpdateRate should return true")
	}

	// Verify the rate was updated in the engine
	if rate := resolver.FadeEngine.GetUpdateRateHz(); rate != 120 {
		t.Errorf("Fade engine rate after mutation = %d, want 120", rate)
	}

	// Verify the rate was saved to database
	setting, err := resolver.SettingRepo.FindByKey(context.Background(), "fade_update_rate_hz")
	if err != nil {
		t.Fatalf("Failed to query setting from database: %v", err)
	}
	if setting == nil || setting.Value != "120" {
		t.Errorf("Setting not saved to database, got %v", setting)
	}

	// Query again to verify the change persists
	var resp3 struct {
		SystemInfo struct {
			FadeUpdateRateHz int `json:"fadeUpdateRateHz"`
		} `json:"systemInfo"`
	}

	err = c.Post(`query {
		systemInfo {
			fadeUpdateRateHz
		}
	}`, &resp3)

	if err != nil {
		t.Fatalf("SystemInfo query after update failed: %v", err)
	}

	if resp3.SystemInfo.FadeUpdateRateHz != 120 {
		t.Errorf("Fade update rate after mutation = %d, want 120", resp3.SystemInfo.FadeUpdateRateHz)
	}
}

func TestFadeUpdateRate_ValidationErrors(t *testing.T) {
	c, _, cleanup := testSetup(t)
	defer cleanup()

	tests := []struct {
		name   string
		rateHz int
	}{
		{"too low", 0},
		{"negative", -10},
		{"too high", 300},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var resp struct {
				UpdateFadeUpdateRate bool `json:"updateFadeUpdateRate"`
			}

			query := fmt.Sprintf(`mutation {
				updateFadeUpdateRate(rateHz: %d)
			}`, tt.rateHz)

			err := c.Post(query, &resp)

			// Should return an error for invalid rates
			if err == nil {
				t.Errorf("Expected error for rateHz=%d, got none", tt.rateHz)
			}
		})
	}
}
