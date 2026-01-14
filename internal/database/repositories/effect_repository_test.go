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

// setupEffectTestDB creates an in-memory SQLite database for testing effect repository.
func setupEffectTestDB(t *testing.T) (*gorm.DB, func()) {
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
		&models.Effect{},
		&models.EffectFixture{},
		&models.EffectChannel{},
		&models.CueEffect{},
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

// createTestProjectAndFixtures creates a project with fixtures for testing.
func createTestProjectAndFixtures(t *testing.T, db *gorm.DB) (*models.Project, []*models.FixtureInstance) {
	t.Helper()

	project := &models.Project{
		ID:   cuid.New(),
		Name: "Test Project " + cuid.Slug(),
	}
	if err := db.Create(project).Error; err != nil {
		t.Fatalf("Failed to create project: %v", err)
	}

	fixtureDef := &models.FixtureDefinition{
		ID:           cuid.New(),
		Manufacturer: "Test",
		Model:        "TestModel",
		Type:         "LED_PAR",
	}
	if err := db.Create(fixtureDef).Error; err != nil {
		t.Fatalf("Failed to create fixture definition: %v", err)
	}

	fixtures := make([]*models.FixtureInstance, 3)
	for i := range fixtures {
		fixtures[i] = &models.FixtureInstance{
			ID:           cuid.New(),
			Name:         "Fixture " + string(rune('A'+i)),
			ProjectID:    project.ID,
			DefinitionID: fixtureDef.ID,
			Universe:     1,
			StartChannel: i*10 + 1,
		}
		if err := db.Create(fixtures[i]).Error; err != nil {
			t.Fatalf("Failed to create fixture: %v", err)
		}
	}

	return project, fixtures
}

// TestNewEffectRepository tests the constructor.
func TestNewEffectRepository(t *testing.T) {
	db, cleanup := setupEffectTestDB(t)
	defer cleanup()

	repo := NewEffectRepository(db)
	if repo == nil {
		t.Fatal("Expected non-nil repository")
	}
	if repo.db != db {
		t.Error("Expected db to be set")
	}
}

// TestEffectRepository_CRUD tests basic CRUD operations on the EffectRepository.
func TestEffectRepository_CRUD(t *testing.T) {
	db, cleanup := setupEffectTestDB(t)
	defer cleanup()

	repo := NewEffectRepository(db)
	ctx := context.Background()

	project, _ := createTestProjectAndFixtures(t, db)

	// Test Create
	effect := &models.Effect{
		Name:        "Test Effect " + cuid.Slug(),
		ProjectID:   project.ID,
		EffectType:  "WAVEFORM",
		Waveform:    strPtr("SINE"),
		Frequency:   1.0,
		Amplitude:   100.0,
		Offset:      50.0,
		PhaseOffset: 0.0,
	}
	err := repo.Create(ctx, effect)
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}
	if effect.ID == "" {
		t.Error("Expected effect ID to be set after Create")
	}

	// Test FindByID
	found, err := repo.FindByID(ctx, effect.ID)
	if err != nil {
		t.Fatalf("FindByID failed: %v", err)
	}
	if found == nil {
		t.Fatal("Expected to find effect")
	}
	if found.Name != effect.Name {
		t.Errorf("Name mismatch: got %s, want %s", found.Name, effect.Name)
	}

	// Test FindByProjectID
	effects, err := repo.FindByProjectID(ctx, project.ID)
	if err != nil {
		t.Fatalf("FindByProjectID failed: %v", err)
	}
	if len(effects) == 0 {
		t.Error("Expected at least one effect")
	}

	// Test Update
	effect.Name = "Updated Effect Name"
	err = repo.Update(ctx, effect)
	if err != nil {
		t.Fatalf("Update failed: %v", err)
	}
	found, _ = repo.FindByID(ctx, effect.ID)
	if found.Name != "Updated Effect Name" {
		t.Errorf("Update didn't persist: got %s", found.Name)
	}

	// Test Delete
	err = repo.Delete(ctx, effect.ID)
	if err != nil {
		t.Fatalf("Delete failed: %v", err)
	}
	found, err = repo.FindByID(ctx, effect.ID)
	if err != nil {
		t.Fatalf("FindByID after delete failed: %v", err)
	}
	if found != nil {
		t.Error("Expected effect to be deleted")
	}
}

// TestEffectRepository_FindByID_NotFound tests FindByID with non-existent ID.
func TestEffectRepository_FindByID_NotFound(t *testing.T) {
	db, cleanup := setupEffectTestDB(t)
	defer cleanup()

	repo := NewEffectRepository(db)
	ctx := context.Background()

	found, err := repo.FindByID(ctx, "nonexistent-id")
	if err != nil {
		t.Fatalf("FindByID failed: %v", err)
	}
	if found != nil {
		t.Error("Expected nil for non-existent effect")
	}
}

// TestEffectRepository_Create_WithID tests Create with pre-set ID.
func TestEffectRepository_Create_WithID(t *testing.T) {
	db, cleanup := setupEffectTestDB(t)
	defer cleanup()

	repo := NewEffectRepository(db)
	ctx := context.Background()

	project, _ := createTestProjectAndFixtures(t, db)

	customID := cuid.New()
	effect := &models.Effect{
		ID:         customID,
		Name:       "Effect with custom ID",
		ProjectID:  project.ID,
		EffectType: "WAVEFORM",
	}
	err := repo.Create(ctx, effect)
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}
	if effect.ID != customID {
		t.Errorf("ID changed: got %s, want %s", effect.ID, customID)
	}
}

// TestEffectRepository_FixtureOperations tests fixture-related operations.
func TestEffectRepository_FixtureOperations(t *testing.T) {
	db, cleanup := setupEffectTestDB(t)
	defer cleanup()

	repo := NewEffectRepository(db)
	ctx := context.Background()

	project, fixtures := createTestProjectAndFixtures(t, db)

	// Create an effect
	effect := &models.Effect{
		ID:         cuid.New(),
		Name:       "Effect for Fixtures",
		ProjectID:  project.ID,
		EffectType: "WAVEFORM",
	}
	db.Create(effect)

	// Test CountFixtures (should be 0)
	count, err := repo.CountFixtures(ctx, effect.ID)
	if err != nil {
		t.Fatalf("CountFixtures failed: %v", err)
	}
	if count != 0 {
		t.Errorf("Expected 0 fixtures, got %d", count)
	}

	// Test AddFixtureToEffect
	phaseOffset := 90.0
	amplitudeScale := 0.5
	err = repo.AddFixtureToEffect(ctx, effect.ID, fixtures[0].ID, &phaseOffset, &amplitudeScale)
	if err != nil {
		t.Fatalf("AddFixtureToEffect failed: %v", err)
	}

	count, _ = repo.CountFixtures(ctx, effect.ID)
	if count != 1 {
		t.Errorf("Expected 1 fixture, got %d", count)
	}

	// Test GetEffectFixture
	effectFixture, err := repo.GetEffectFixture(ctx, effect.ID, fixtures[0].ID)
	if err != nil {
		t.Fatalf("GetEffectFixture failed: %v", err)
	}
	if effectFixture == nil {
		t.Fatal("Expected to find effect fixture")
	}
	if effectFixture.PhaseOffset == nil || *effectFixture.PhaseOffset != phaseOffset {
		t.Errorf("PhaseOffset mismatch: got %v, want %f", effectFixture.PhaseOffset, phaseOffset)
	}
	if effectFixture.AmplitudeScale == nil || *effectFixture.AmplitudeScale != amplitudeScale {
		t.Errorf("AmplitudeScale mismatch: got %v, want %f", effectFixture.AmplitudeScale, amplitudeScale)
	}

	// Test GetEffectFixtures
	effectFixtures, err := repo.GetEffectFixtures(ctx, effect.ID)
	if err != nil {
		t.Fatalf("GetEffectFixtures failed: %v", err)
	}
	if len(effectFixtures) != 1 {
		t.Errorf("Expected 1 effect fixture, got %d", len(effectFixtures))
	}

	// Add more fixtures
	err = repo.AddFixtureToEffect(ctx, effect.ID, fixtures[1].ID, nil, nil)
	if err != nil {
		t.Fatalf("AddFixtureToEffect (second) failed: %v", err)
	}

	count, _ = repo.CountFixtures(ctx, effect.ID)
	if count != 2 {
		t.Errorf("Expected 2 fixtures, got %d", count)
	}

	// Test RemoveFixtureFromEffect
	err = repo.RemoveFixtureFromEffect(ctx, effect.ID, fixtures[0].ID)
	if err != nil {
		t.Fatalf("RemoveFixtureFromEffect failed: %v", err)
	}

	count, _ = repo.CountFixtures(ctx, effect.ID)
	if count != 1 {
		t.Errorf("Expected 1 fixture after removal, got %d", count)
	}

	// Verify the right fixture was removed
	effectFixture, _ = repo.GetEffectFixture(ctx, effect.ID, fixtures[0].ID)
	if effectFixture != nil {
		t.Error("Expected fixture to be removed")
	}

	// Test RemoveFixtureFromEffect for non-existent fixture (should not error)
	err = repo.RemoveFixtureFromEffect(ctx, effect.ID, "nonexistent-fixture")
	if err != nil {
		t.Errorf("RemoveFixtureFromEffect for non-existent should not error: %v", err)
	}
}

// TestEffectRepository_UpdateEffectFixture tests updating effect fixture properties.
func TestEffectRepository_UpdateEffectFixture(t *testing.T) {
	db, cleanup := setupEffectTestDB(t)
	defer cleanup()

	repo := NewEffectRepository(db)
	ctx := context.Background()

	project, fixtures := createTestProjectAndFixtures(t, db)

	// Create an effect with a fixture
	effect := &models.Effect{
		ID:         cuid.New(),
		Name:       "Effect for Update",
		ProjectID:  project.ID,
		EffectType: "WAVEFORM",
	}
	db.Create(effect)

	err := repo.AddFixtureToEffect(ctx, effect.ID, fixtures[0].ID, nil, nil)
	if err != nil {
		t.Fatalf("AddFixtureToEffect failed: %v", err)
	}

	// Get and update the effect fixture
	effectFixture, _ := repo.GetEffectFixture(ctx, effect.ID, fixtures[0].ID)
	newPhaseOffset := 180.0
	newAmplitude := 0.75
	effectFixture.PhaseOffset = &newPhaseOffset
	effectFixture.AmplitudeScale = &newAmplitude

	err = repo.UpdateEffectFixture(ctx, effectFixture)
	if err != nil {
		t.Fatalf("UpdateEffectFixture failed: %v", err)
	}

	// Verify update
	updated, _ := repo.GetEffectFixture(ctx, effect.ID, fixtures[0].ID)
	if updated.PhaseOffset == nil || *updated.PhaseOffset != newPhaseOffset {
		t.Errorf("PhaseOffset not updated: got %v, want %f", updated.PhaseOffset, newPhaseOffset)
	}
}

// TestEffectRepository_ChannelOperations tests channel-related operations.
func TestEffectRepository_ChannelOperations(t *testing.T) {
	db, cleanup := setupEffectTestDB(t)
	defer cleanup()

	repo := NewEffectRepository(db)
	ctx := context.Background()

	project, fixtures := createTestProjectAndFixtures(t, db)

	// Create an effect with a fixture
	effect := &models.Effect{
		ID:         cuid.New(),
		Name:       "Effect for Channels",
		ProjectID:  project.ID,
		EffectType: "WAVEFORM",
	}
	db.Create(effect)

	err := repo.AddFixtureToEffect(ctx, effect.ID, fixtures[0].ID, nil, nil)
	if err != nil {
		t.Fatalf("AddFixtureToEffect failed: %v", err)
	}

	effectFixture, _ := repo.GetEffectFixture(ctx, effect.ID, fixtures[0].ID)

	// Test AddChannelToEffectFixture
	channelOffset := 0
	channelType := "INTENSITY"
	err = repo.AddChannelToEffectFixture(ctx, effectFixture.ID, &channelOffset, &channelType)
	if err != nil {
		t.Fatalf("AddChannelToEffectFixture failed: %v", err)
	}

	// Test GetEffectChannels
	channels, err := repo.GetEffectChannels(ctx, effectFixture.ID)
	if err != nil {
		t.Fatalf("GetEffectChannels failed: %v", err)
	}
	if len(channels) != 1 {
		t.Errorf("Expected 1 channel, got %d", len(channels))
	}
	if channels[0].ChannelOffset == nil || *channels[0].ChannelOffset != channelOffset {
		t.Errorf("ChannelOffset mismatch: got %v, want %d", channels[0].ChannelOffset, channelOffset)
	}

	// Test AddChannelToEffectFixtureWithScales
	channelOffset2 := 1
	channelType2 := "COLOR"
	amplitudeScale := 0.8
	frequencyScale := 2.0
	err = repo.AddChannelToEffectFixtureWithScales(ctx, effectFixture.ID, &channelOffset2, &channelType2, &amplitudeScale, &frequencyScale)
	if err != nil {
		t.Fatalf("AddChannelToEffectFixtureWithScales failed: %v", err)
	}

	channels, _ = repo.GetEffectChannels(ctx, effectFixture.ID)
	if len(channels) != 2 {
		t.Errorf("Expected 2 channels, got %d", len(channels))
	}

	// Test UpdateEffectChannel
	channels[0].AmplitudeScale = &amplitudeScale
	err = repo.UpdateEffectChannel(ctx, &channels[0])
	if err != nil {
		t.Fatalf("UpdateEffectChannel failed: %v", err)
	}

	// Test DeleteEffectChannel
	err = repo.DeleteEffectChannel(ctx, channels[0].ID)
	if err != nil {
		t.Fatalf("DeleteEffectChannel failed: %v", err)
	}

	channels, _ = repo.GetEffectChannels(ctx, effectFixture.ID)
	if len(channels) != 1 {
		t.Errorf("Expected 1 channel after delete, got %d", len(channels))
	}
}

// TestEffectRepository_FindWithFixtures tests loading effect with fixtures preloaded.
func TestEffectRepository_FindWithFixtures(t *testing.T) {
	db, cleanup := setupEffectTestDB(t)
	defer cleanup()

	repo := NewEffectRepository(db)
	ctx := context.Background()

	project, fixtures := createTestProjectAndFixtures(t, db)

	// Create an effect with fixtures and channels
	effect := &models.Effect{
		ID:         cuid.New(),
		Name:       "Effect with Fixtures",
		ProjectID:  project.ID,
		EffectType: "WAVEFORM",
	}
	db.Create(effect)

	err := repo.AddFixtureToEffect(ctx, effect.ID, fixtures[0].ID, nil, nil)
	if err != nil {
		t.Fatalf("AddFixtureToEffect failed: %v", err)
	}

	effectFixture, _ := repo.GetEffectFixture(ctx, effect.ID, fixtures[0].ID)
	channelOffset := 0
	channelType := "INTENSITY"
	err = repo.AddChannelToEffectFixture(ctx, effectFixture.ID, &channelOffset, &channelType)
	if err != nil {
		t.Fatalf("AddChannelToEffectFixture failed: %v", err)
	}

	// Test FindWithFixtures
	loaded, err := repo.FindWithFixtures(ctx, effect.ID)
	if err != nil {
		t.Fatalf("FindWithFixtures failed: %v", err)
	}
	if loaded == nil {
		t.Fatal("Expected to find effect with fixtures")
	}
	if len(loaded.Fixtures) != 1 {
		t.Errorf("Expected 1 fixture, got %d", len(loaded.Fixtures))
	}
	if len(loaded.Fixtures[0].Channels) != 1 {
		t.Errorf("Expected 1 channel, got %d", len(loaded.Fixtures[0].Channels))
	}
}

// TestEffectRepository_FindWithFixtures_NotFound tests FindWithFixtures with non-existent ID.
func TestEffectRepository_FindWithFixtures_NotFound(t *testing.T) {
	db, cleanup := setupEffectTestDB(t)
	defer cleanup()

	repo := NewEffectRepository(db)
	ctx := context.Background()

	found, err := repo.FindWithFixtures(ctx, "nonexistent-id")
	if err != nil {
		t.Fatalf("FindWithFixtures failed: %v", err)
	}
	if found != nil {
		t.Error("Expected nil for non-existent effect")
	}
}

// TestEffectRepository_CueEffectOperations tests cue-effect operations.
func TestEffectRepository_CueEffectOperations(t *testing.T) {
	db, cleanup := setupEffectTestDB(t)
	defer cleanup()

	repo := NewEffectRepository(db)
	ctx := context.Background()

	project, _ := createTestProjectAndFixtures(t, db)

	// Create a look
	look := &models.Look{
		ID:        cuid.New(),
		Name:      "Test Look",
		ProjectID: project.ID,
	}
	db.Create(look)

	// Create a cue list and cue
	cueList := &models.CueList{
		ID:        cuid.New(),
		Name:      "Test Cue List",
		ProjectID: project.ID,
	}
	db.Create(cueList)

	cue := &models.Cue{
		ID:        cuid.New(),
		Name:      "Test Cue",
		CueNumber: 1.0,
		CueListID: cueList.ID,
		LookID:    look.ID,
	}
	db.Create(cue)

	// Create effects
	effect1 := &models.Effect{
		ID:         cuid.New(),
		Name:       "Effect 1",
		ProjectID:  project.ID,
		EffectType: "WAVEFORM",
	}
	effect2 := &models.Effect{
		ID:         cuid.New(),
		Name:       "Effect 2",
		ProjectID:  project.ID,
		EffectType: "WAVEFORM",
	}
	db.Create(effect1)
	db.Create(effect2)

	// Test CountEffectsByCueID (should be 0)
	count, err := repo.CountEffectsByCueID(ctx, cue.ID)
	if err != nil {
		t.Fatalf("CountEffectsByCueID failed: %v", err)
	}
	if count != 0 {
		t.Errorf("Expected 0 effects, got %d", count)
	}

	// Test AddEffectToCue
	err = repo.AddEffectToCue(ctx, cue.ID, effect1.ID, 100.0, 1.0)
	if err != nil {
		t.Fatalf("AddEffectToCue failed: %v", err)
	}

	count, _ = repo.CountEffectsByCueID(ctx, cue.ID)
	if count != 1 {
		t.Errorf("Expected 1 effect, got %d", count)
	}

	// Test GetCueEffect
	cueEffect, err := repo.GetCueEffect(ctx, cue.ID, effect1.ID)
	if err != nil {
		t.Fatalf("GetCueEffect failed: %v", err)
	}
	if cueEffect == nil {
		t.Fatal("Expected to find cue effect")
	}
	if cueEffect.Intensity != 100.0 {
		t.Errorf("Intensity mismatch: got %f, want 100.0", cueEffect.Intensity)
	}
	if cueEffect.Speed != 1.0 {
		t.Errorf("Speed mismatch: got %f, want 1.0", cueEffect.Speed)
	}

	// Test FindEffectsByCueID
	cueEffects, err := repo.FindEffectsByCueID(ctx, cue.ID)
	if err != nil {
		t.Fatalf("FindEffectsByCueID failed: %v", err)
	}
	if len(cueEffects) != 1 {
		t.Errorf("Expected 1 cue effect, got %d", len(cueEffects))
	}

	// Add another effect
	err = repo.AddEffectToCue(ctx, cue.ID, effect2.ID, 50.0, 2.0)
	if err != nil {
		t.Fatalf("AddEffectToCue (second) failed: %v", err)
	}

	count, _ = repo.CountEffectsByCueID(ctx, cue.ID)
	if count != 2 {
		t.Errorf("Expected 2 effects, got %d", count)
	}

	// Test UpdateCueEffect
	cueEffect.Intensity = 75.0
	cueEffect.Speed = 1.5
	err = repo.UpdateCueEffect(ctx, cueEffect)
	if err != nil {
		t.Fatalf("UpdateCueEffect failed: %v", err)
	}

	updated, _ := repo.GetCueEffect(ctx, cue.ID, effect1.ID)
	if updated.Intensity != 75.0 {
		t.Errorf("Intensity not updated: got %f, want 75.0", updated.Intensity)
	}

	// Test FindCueEffectByID
	foundByID, err := repo.FindCueEffectByID(ctx, cueEffect.ID)
	if err != nil {
		t.Fatalf("FindCueEffectByID failed: %v", err)
	}
	if foundByID == nil {
		t.Fatal("Expected to find cue effect by ID")
	}
	if foundByID.Effect == nil {
		t.Error("Expected Effect to be preloaded")
	}

	// Test RemoveEffectFromCue
	err = repo.RemoveEffectFromCue(ctx, cue.ID, effect1.ID)
	if err != nil {
		t.Fatalf("RemoveEffectFromCue failed: %v", err)
	}

	count, _ = repo.CountEffectsByCueID(ctx, cue.ID)
	if count != 1 {
		t.Errorf("Expected 1 effect after removal, got %d", count)
	}

	cueEffect, _ = repo.GetCueEffect(ctx, cue.ID, effect1.ID)
	if cueEffect != nil {
		t.Error("Expected cue effect to be removed")
	}
}

// TestEffectRepository_GetCueEffect_NotFound tests GetCueEffect with non-existent IDs.
func TestEffectRepository_GetCueEffect_NotFound(t *testing.T) {
	db, cleanup := setupEffectTestDB(t)
	defer cleanup()

	repo := NewEffectRepository(db)
	ctx := context.Background()

	found, err := repo.GetCueEffect(ctx, "nonexistent-cue", "nonexistent-effect")
	if err != nil {
		t.Fatalf("GetCueEffect failed: %v", err)
	}
	if found != nil {
		t.Error("Expected nil for non-existent cue effect")
	}
}

// TestEffectRepository_CreateWithFixtures tests creating effect with fixtures in transaction.
func TestEffectRepository_CreateWithFixtures(t *testing.T) {
	db, cleanup := setupEffectTestDB(t)
	defer cleanup()

	repo := NewEffectRepository(db)
	ctx := context.Background()

	project, fixtures := createTestProjectAndFixtures(t, db)

	// Create effect with fixtures
	effect := &models.Effect{
		Name:       "Effect with Fixtures",
		ProjectID:  project.ID,
		EffectType: "WAVEFORM",
	}
	phaseOffset := 45.0
	effectFixtures := []models.EffectFixture{
		{FixtureID: fixtures[0].ID, PhaseOffset: &phaseOffset},
		{FixtureID: fixtures[1].ID},
	}

	err := repo.CreateWithFixtures(ctx, effect, effectFixtures)
	if err != nil {
		t.Fatalf("CreateWithFixtures failed: %v", err)
	}

	if effect.ID == "" {
		t.Error("Expected effect ID to be set")
	}

	// Verify fixtures were created
	count, _ := repo.CountFixtures(ctx, effect.ID)
	if count != 2 {
		t.Errorf("Expected 2 fixtures, got %d", count)
	}

	// Verify fixture IDs were set
	loaded, _ := repo.FindWithFixtures(ctx, effect.ID)
	if len(loaded.Fixtures) != 2 {
		t.Errorf("Expected 2 fixtures loaded, got %d", len(loaded.Fixtures))
	}
	for _, f := range loaded.Fixtures {
		if f.ID == "" {
			t.Error("Expected fixture ID to be set")
		}
		if f.EffectID != effect.ID {
			t.Errorf("EffectID mismatch: got %s, want %s", f.EffectID, effect.ID)
		}
	}
}

// TestEffectRepository_CreateWithFixtures_EmptyFixtures tests creating effect with no fixtures.
func TestEffectRepository_CreateWithFixtures_EmptyFixtures(t *testing.T) {
	db, cleanup := setupEffectTestDB(t)
	defer cleanup()

	repo := NewEffectRepository(db)
	ctx := context.Background()

	project, _ := createTestProjectAndFixtures(t, db)

	effect := &models.Effect{
		Name:       "Effect without Fixtures",
		ProjectID:  project.ID,
		EffectType: "WAVEFORM",
	}

	err := repo.CreateWithFixtures(ctx, effect, []models.EffectFixture{})
	if err != nil {
		t.Fatalf("CreateWithFixtures with empty fixtures failed: %v", err)
	}

	if effect.ID == "" {
		t.Error("Expected effect ID to be set")
	}

	count, _ := repo.CountFixtures(ctx, effect.ID)
	if count != 0 {
		t.Errorf("Expected 0 fixtures, got %d", count)
	}
}

// TestEffectRepository_DeleteEffectFixtures tests deleting all fixtures from an effect.
func TestEffectRepository_DeleteEffectFixtures(t *testing.T) {
	db, cleanup := setupEffectTestDB(t)
	defer cleanup()

	repo := NewEffectRepository(db)
	ctx := context.Background()

	project, fixtures := createTestProjectAndFixtures(t, db)

	// Create effect with fixtures and channels
	effect := &models.Effect{
		ID:         cuid.New(),
		Name:       "Effect for Delete",
		ProjectID:  project.ID,
		EffectType: "WAVEFORM",
	}
	db.Create(effect)

	err := repo.AddFixtureToEffect(ctx, effect.ID, fixtures[0].ID, nil, nil)
	if err != nil {
		t.Fatalf("AddFixtureToEffect failed: %v", err)
	}

	effectFixture, _ := repo.GetEffectFixture(ctx, effect.ID, fixtures[0].ID)
	channelOffset := 0
	channelType := "INTENSITY"
	err = repo.AddChannelToEffectFixture(ctx, effectFixture.ID, &channelOffset, &channelType)
	if err != nil {
		t.Fatalf("AddChannelToEffectFixture failed: %v", err)
	}

	// Verify setup
	count, _ := repo.CountFixtures(ctx, effect.ID)
	if count != 1 {
		t.Fatalf("Expected 1 fixture before delete, got %d", count)
	}

	// Test DeleteEffectFixtures
	err = repo.DeleteEffectFixtures(ctx, effect.ID)
	if err != nil {
		t.Fatalf("DeleteEffectFixtures failed: %v", err)
	}

	count, _ = repo.CountFixtures(ctx, effect.ID)
	if count != 0 {
		t.Errorf("Expected 0 fixtures after delete, got %d", count)
	}

	// Verify channels were also deleted
	var channelCount int64
	db.Model(&models.EffectChannel{}).
		Where("effect_fixture_id = ?", effectFixture.ID).
		Count(&channelCount)
	if channelCount != 0 {
		t.Errorf("Expected 0 channels after delete, got %d", channelCount)
	}
}

// TestEffectRepository_Delete_Cascade tests that deleting an effect cascades properly.
func TestEffectRepository_Delete_Cascade(t *testing.T) {
	db, cleanup := setupEffectTestDB(t)
	defer cleanup()

	repo := NewEffectRepository(db)
	ctx := context.Background()

	project, fixtures := createTestProjectAndFixtures(t, db)

	// Create a look, cue list, and cue
	look := &models.Look{ID: cuid.New(), Name: "Test Look", ProjectID: project.ID}
	db.Create(look)
	cueList := &models.CueList{ID: cuid.New(), Name: "Test CL", ProjectID: project.ID}
	db.Create(cueList)
	cue := &models.Cue{ID: cuid.New(), Name: "Cue 1", CueNumber: 1.0, CueListID: cueList.ID, LookID: look.ID}
	db.Create(cue)

	// Create effect with fixtures, channels, and cue effect
	effect := &models.Effect{
		ID:         cuid.New(),
		Name:       "Effect for Cascade Delete",
		ProjectID:  project.ID,
		EffectType: "WAVEFORM",
	}
	db.Create(effect)

	err := repo.AddFixtureToEffect(ctx, effect.ID, fixtures[0].ID, nil, nil)
	if err != nil {
		t.Fatalf("AddFixtureToEffect failed: %v", err)
	}

	effectFixture, _ := repo.GetEffectFixture(ctx, effect.ID, fixtures[0].ID)
	channelOffset := 0
	channelType := "INTENSITY"
	err = repo.AddChannelToEffectFixture(ctx, effectFixture.ID, &channelOffset, &channelType)
	if err != nil {
		t.Fatalf("AddChannelToEffectFixture failed: %v", err)
	}

	err = repo.AddEffectToCue(ctx, cue.ID, effect.ID, 100.0, 1.0)
	if err != nil {
		t.Fatalf("AddEffectToCue failed: %v", err)
	}

	// Verify setup
	var fixtureCount, channelCount, cueEffectCount int64
	db.Model(&models.EffectFixture{}).Where("effect_id = ?", effect.ID).Count(&fixtureCount)
	db.Model(&models.EffectChannel{}).Where("effect_fixture_id = ?", effectFixture.ID).Count(&channelCount)
	db.Model(&models.CueEffect{}).Where("effect_id = ?", effect.ID).Count(&cueEffectCount)

	if fixtureCount != 1 || channelCount != 1 || cueEffectCount != 1 {
		t.Fatalf("Setup verification failed: fixtures=%d, channels=%d, cueEffects=%d",
			fixtureCount, channelCount, cueEffectCount)
	}

	// Delete the effect
	err = repo.Delete(ctx, effect.ID)
	if err != nil {
		t.Fatalf("Delete failed: %v", err)
	}

	// Verify cascade
	db.Model(&models.EffectFixture{}).Where("effect_id = ?", effect.ID).Count(&fixtureCount)
	db.Model(&models.EffectChannel{}).Where("effect_fixture_id = ?", effectFixture.ID).Count(&channelCount)
	db.Model(&models.CueEffect{}).Where("effect_id = ?", effect.ID).Count(&cueEffectCount)

	if fixtureCount != 0 {
		t.Errorf("Expected 0 fixtures after delete, got %d", fixtureCount)
	}
	if channelCount != 0 {
		t.Errorf("Expected 0 channels after delete, got %d", channelCount)
	}
	if cueEffectCount != 0 {
		t.Errorf("Expected 0 cue effects after delete, got %d", cueEffectCount)
	}
}

// TestEffectRepository_FindEffectFixtureByID tests finding effect fixture by ID.
func TestEffectRepository_FindEffectFixtureByID(t *testing.T) {
	db, cleanup := setupEffectTestDB(t)
	defer cleanup()

	repo := NewEffectRepository(db)
	ctx := context.Background()

	project, fixtures := createTestProjectAndFixtures(t, db)

	// Create effect with fixture
	effect := &models.Effect{
		ID:         cuid.New(),
		Name:       "Effect for FindByID",
		ProjectID:  project.ID,
		EffectType: "WAVEFORM",
	}
	db.Create(effect)

	err := repo.AddFixtureToEffect(ctx, effect.ID, fixtures[0].ID, nil, nil)
	if err != nil {
		t.Fatalf("AddFixtureToEffect failed: %v", err)
	}

	effectFixture, _ := repo.GetEffectFixture(ctx, effect.ID, fixtures[0].ID)

	// Test FindEffectFixtureByID
	found, err := repo.FindEffectFixtureByID(ctx, effectFixture.ID)
	if err != nil {
		t.Fatalf("FindEffectFixtureByID failed: %v", err)
	}
	if found == nil {
		t.Fatal("Expected to find effect fixture")
	}
	if found.ID != effectFixture.ID {
		t.Errorf("ID mismatch: got %s, want %s", found.ID, effectFixture.ID)
	}
	if found.Fixture == nil {
		t.Error("Expected Fixture to be preloaded")
	}

	// Test not found
	notFound, err := repo.FindEffectFixtureByID(ctx, "nonexistent-id")
	if err != nil {
		t.Fatalf("FindEffectFixtureByID for non-existent failed: %v", err)
	}
	if notFound != nil {
		t.Error("Expected nil for non-existent effect fixture")
	}
}

// TestEffectRepository_FindCueEffectByID_NotFound tests FindCueEffectByID with non-existent ID.
func TestEffectRepository_FindCueEffectByID_NotFound(t *testing.T) {
	db, cleanup := setupEffectTestDB(t)
	defer cleanup()

	repo := NewEffectRepository(db)
	ctx := context.Background()

	found, err := repo.FindCueEffectByID(ctx, "nonexistent-id")
	if err != nil {
		t.Fatalf("FindCueEffectByID failed: %v", err)
	}
	if found != nil {
		t.Error("Expected nil for non-existent cue effect")
	}
}

// Helper function to create a string pointer
func strPtr(s string) *string {
	return &s
}
