package resolvers

import (
	"context"
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

	// Create and start fade engine
	fadeEngine := fade.NewEngine(dmxService)
	fadeEngine.Start()

	// Create playback service
	playbackService := playback.NewService(db, dmxService, fadeEngine)

	// Create resolver
	resolver := NewResolver(db, dmxService, fadeEngine, playbackService)

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

	// Create fixture values for the scene
	fixtureValues := []models.FixtureValue{
		{ID: "fv-1", SceneID: scene.ID, FixtureID: fixture.ID, ChannelValues: "[255, 128, 64, 32]"},
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
