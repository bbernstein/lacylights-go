// Package integration contains integration tests for the LacyLights system.
package integration

import (
	"context"
	"testing"

	"github.com/bbernstein/lacylights-go/internal/database/models"
	"github.com/bbernstein/lacylights-go/internal/database/repositories"
	"github.com/bbernstein/lacylights-go/internal/services/export"
	importservice "github.com/bbernstein/lacylights-go/internal/services/import"
	"github.com/glebarez/sqlite"
	"github.com/lucsky/cuid"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

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
		&models.Scene{},
		&models.FixtureValue{},
		&models.CueList{},
		&models.Cue{},
		&models.SceneBoard{},
		&models.SceneBoardButton{},
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

// TestExportImportRoundTrip_Integration tests that data survives a full export -> import cycle
func TestExportImportRoundTrip_Integration(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	projectRepo := repositories.NewProjectRepository(db)
	fixtureRepo := repositories.NewFixtureRepository(db)
	sceneRepo := repositories.NewSceneRepository(db)
	cueListRepo := repositories.NewCueListRepository(db)
	cueRepo := repositories.NewCueRepository(db)
	sceneBoardRepo := repositories.NewSceneBoardRepository(db)

	exportService := export.NewServiceWithSceneBoards(projectRepo, fixtureRepo, sceneRepo, cueListRepo, cueRepo, sceneBoardRepo)
	importService := importservice.NewServiceWithSceneBoards(projectRepo, fixtureRepo, sceneRepo, cueListRepo, cueRepo, sceneBoardRepo)
	ctx := context.Background()

	// === SETUP: Create source project with all data types ===

	// 1. Create project
	sourceProject := &models.Project{
		ID:   cuid.New(),
		Name: "Source Project",
	}
	if err := projectRepo.Create(ctx, sourceProject); err != nil {
		t.Fatalf("Failed to create project: %v", err)
	}

	// 2. Create fixture definition with fade behavior
	def := &models.FixtureDefinition{
		ID:           cuid.New(),
		Manufacturer: "TestMfg",
		Model:        "TestModel",
		Type:         "LED_PAR",
		IsBuiltIn:    false,
	}
	if err := fixtureRepo.CreateDefinition(ctx, def); err != nil {
		t.Fatalf("Failed to create definition: %v", err)
	}

	// Create channels with different fade behaviors
	channels := []models.ChannelDefinition{
		{ID: cuid.New(), Name: "Dimmer", Type: "INTENSITY", Offset: 0, DefinitionID: def.ID, FadeBehavior: "FADE", IsDiscrete: false},
		{ID: cuid.New(), Name: "Gobo", Type: "GOBO", Offset: 1, DefinitionID: def.ID, FadeBehavior: "SNAP", IsDiscrete: true},
		{ID: cuid.New(), Name: "Red", Type: "COLOR", Offset: 2, DefinitionID: def.ID, FadeBehavior: "FADE", IsDiscrete: false},
	}
	for _, ch := range channels {
		db.Create(&ch)
	}

	// 3. Create fixture instance with layout fields
	layoutX := 0.35
	layoutY := 0.65
	layoutRotation := 90.0
	projectOrder := 1
	channelCount := 3
	tags := "front,wash"
	fixture := &models.FixtureInstance{
		ID:             cuid.New(),
		Name:           "Test Fixture",
		ProjectID:      sourceProject.ID,
		DefinitionID:   def.ID,
		Universe:       1,
		StartChannel:   1,
		ChannelCount:   &channelCount,
		Tags:           &tags,
		LayoutX:        &layoutX,
		LayoutY:        &layoutY,
		LayoutRotation: &layoutRotation,
		ProjectOrder:   &projectOrder,
	}
	if err := fixtureRepo.Create(ctx, fixture); err != nil {
		t.Fatalf("Failed to create fixture: %v", err)
	}

	// Create instance channels with fade behavior
	instChannels := []models.InstanceChannel{
		{ID: cuid.New(), Name: "Dimmer", Type: "INTENSITY", Offset: 0, FixtureID: fixture.ID, FadeBehavior: "FADE", IsDiscrete: false},
		{ID: cuid.New(), Name: "Gobo", Type: "GOBO", Offset: 1, FixtureID: fixture.ID, FadeBehavior: "SNAP", IsDiscrete: true},
		{ID: cuid.New(), Name: "Red", Type: "COLOR", Offset: 2, FixtureID: fixture.ID, FadeBehavior: "FADE", IsDiscrete: false},
	}
	for _, ic := range instChannels {
		db.Create(&ic)
	}

	// 4. Create scene
	scene := &models.Scene{
		ID:        cuid.New(),
		Name:      "Test Scene",
		ProjectID: sourceProject.ID,
	}
	if err := sceneRepo.Create(ctx, scene); err != nil {
		t.Fatalf("Failed to create scene: %v", err)
	}

	// Create fixture value in scene
	fv := &models.FixtureValue{
		ID:        cuid.New(),
		SceneID:   scene.ID,
		FixtureID: fixture.ID,
		Channels:  `[{"offset":0,"value":255},{"offset":2,"value":128}]`,
	}
	if err := sceneRepo.CreateFixtureValue(ctx, fv); err != nil {
		t.Fatalf("Failed to create fixture value: %v", err)
	}

	// 5. Create scene board
	desc := "Main control board"
	gridSize := 10
	board := &models.SceneBoard{
		ID:              cuid.New(),
		ProjectID:       sourceProject.ID,
		Name:            "Main Board",
		Description:     &desc,
		DefaultFadeTime: 2.5,
		GridSize:        &gridSize,
		CanvasWidth:     2000,
		CanvasHeight:    2000,
	}
	db.Create(board)

	// Create button on scene board
	buttonWidth := 150
	buttonHeight := 100
	buttonColor := "#FF5500"
	buttonLabel := "Scene 1"
	button := &models.SceneBoardButton{
		ID:           cuid.New(),
		SceneBoardID: board.ID,
		SceneID:      scene.ID,
		LayoutX:      100,
		LayoutY:      200,
		Width:        &buttonWidth,
		Height:       &buttonHeight,
		Color:        &buttonColor,
		Label:        &buttonLabel,
	}
	db.Create(button)

	// 6. Create cue list and cue
	cueList := &models.CueList{
		ID:        cuid.New(),
		Name:      "Main Cue List",
		ProjectID: sourceProject.ID,
		Loop:      true,
	}
	if err := cueListRepo.Create(ctx, cueList); err != nil {
		t.Fatalf("Failed to create cue list: %v", err)
	}

	cue := &models.Cue{
		ID:          cuid.New(),
		Name:        "Cue 1",
		CueNumber:   1.0,
		CueListID:   cueList.ID,
		SceneID:     scene.ID,
		FadeInTime:  3.0,
		FadeOutTime: 2.0,
	}
	if err := cueRepo.Create(ctx, cue); err != nil {
		t.Fatalf("Failed to create cue: %v", err)
	}

	// === EXPORT ===
	exported, exportStats, err := exportService.ExportProject(ctx, sourceProject.ID, true, true, true)
	if err != nil {
		t.Fatalf("Export failed: %v", err)
	}

	// Verify export stats
	if exportStats.FixtureDefinitionsCount != 1 {
		t.Errorf("Export: Expected 1 definition, got %d", exportStats.FixtureDefinitionsCount)
	}
	if exportStats.SceneBoardsCount != 1 {
		t.Errorf("Export: Expected 1 scene board, got %d", exportStats.SceneBoardsCount)
	}

	// Convert to JSON (simulating file save/load)
	jsonStr, err := exported.ToJSON()
	if err != nil {
		t.Fatalf("ToJSON failed: %v", err)
	}

	// === IMPORT ===
	importedProjectID, importStats, warnings, err := importService.ImportProject(ctx, jsonStr, importservice.ImportOptions{
		Mode:                    importservice.ImportModeCreate,
		FixtureConflictStrategy: importservice.FixtureConflictSkip, // Use existing definition
	})
	if err != nil {
		t.Fatalf("Import failed: %v", err)
	}

	if len(warnings) > 0 {
		t.Logf("Import warnings: %v", warnings)
	}

	// Verify import stats
	if importStats.ScenesCreated != 1 {
		t.Errorf("Import: Expected 1 scene, got %d", importStats.ScenesCreated)
	}
	if importStats.SceneBoardsCreated != 1 {
		t.Errorf("Import: Expected 1 scene board, got %d", importStats.SceneBoardsCreated)
	}

	// === VERIFY IMPORTED DATA ===

	// 1. Verify imported project
	importedProject, err := projectRepo.FindByID(ctx, importedProjectID)
	if err != nil || importedProject == nil {
		t.Fatalf("Failed to find imported project: %v", err)
	}
	if importedProject.Name != "Source Project" {
		t.Errorf("Expected project name 'Source Project', got '%s'", importedProject.Name)
	}

	// 2. Verify imported fixture with layout fields
	importedFixtures, _ := fixtureRepo.FindByProjectID(ctx, importedProjectID)
	if len(importedFixtures) != 1 {
		t.Fatalf("Expected 1 imported fixture, got %d", len(importedFixtures))
	}

	importedFixture := importedFixtures[0]
	if importedFixture.Name != "Test Fixture" {
		t.Errorf("Expected fixture name 'Test Fixture', got '%s'", importedFixture.Name)
	}
	if importedFixture.LayoutX == nil || *importedFixture.LayoutX != 0.35 {
		t.Errorf("Expected LayoutX 0.35, got %v", importedFixture.LayoutX)
	}
	if importedFixture.LayoutY == nil || *importedFixture.LayoutY != 0.65 {
		t.Errorf("Expected LayoutY 0.65, got %v", importedFixture.LayoutY)
	}
	if importedFixture.LayoutRotation == nil || *importedFixture.LayoutRotation != 90.0 {
		t.Errorf("Expected LayoutRotation 90.0, got %v", importedFixture.LayoutRotation)
	}
	if importedFixture.ProjectOrder == nil || *importedFixture.ProjectOrder != 1 {
		t.Errorf("Expected ProjectOrder 1, got %v", importedFixture.ProjectOrder)
	}

	// 3. Verify imported instance channels with fade behavior
	importedInstChannels, _ := fixtureRepo.GetInstanceChannels(ctx, importedFixture.ID)
	if len(importedInstChannels) < 1 {
		t.Fatalf("Expected at least 1 imported instance channel, got %d", len(importedInstChannels))
	}

	// Find the Gobo channel and verify SNAP behavior
	for _, ic := range importedInstChannels {
		if ic.Name == "Gobo" {
			if ic.FadeBehavior != "SNAP" {
				t.Errorf("Expected Gobo FadeBehavior 'SNAP', got '%s'", ic.FadeBehavior)
			}
			if !ic.IsDiscrete {
				t.Error("Expected Gobo IsDiscrete to be true")
			}
		}
	}

	// 4. Verify imported scene
	importedScenes, _ := sceneRepo.FindByProjectID(ctx, importedProjectID)
	if len(importedScenes) != 1 {
		t.Fatalf("Expected 1 imported scene, got %d", len(importedScenes))
	}
	if importedScenes[0].Name != "Test Scene" {
		t.Errorf("Expected scene name 'Test Scene', got '%s'", importedScenes[0].Name)
	}

	// 5. Verify imported scene board
	importedBoards, err := sceneBoardRepo.FindByProjectID(ctx, importedProjectID)
	if err != nil {
		t.Fatalf("Failed to get imported scene boards: %v", err)
	}
	if len(importedBoards) != 1 {
		t.Fatalf("Expected 1 imported scene board, got %d", len(importedBoards))
	}

	importedBoard := importedBoards[0]
	if importedBoard.Name != "Main Board" {
		t.Errorf("Expected board name 'Main Board', got '%s'", importedBoard.Name)
	}
	if importedBoard.DefaultFadeTime != 2.5 {
		t.Errorf("Expected DefaultFadeTime 2.5, got %f", importedBoard.DefaultFadeTime)
	}
	if importedBoard.GridSize == nil || *importedBoard.GridSize != 10 {
		t.Errorf("Expected GridSize 10, got %v", importedBoard.GridSize)
	}
	if importedBoard.CanvasWidth != 2000 || importedBoard.CanvasHeight != 2000 {
		t.Errorf("Expected canvas 2000x2000, got %dx%d", importedBoard.CanvasWidth, importedBoard.CanvasHeight)
	}

	// Verify imported button
	importedButtons, _ := sceneBoardRepo.GetButtons(ctx, importedBoard.ID)
	if len(importedButtons) != 1 {
		t.Fatalf("Expected 1 imported button, got %d", len(importedButtons))
	}

	importedButton := importedButtons[0]
	if importedButton.LayoutX != 100 || importedButton.LayoutY != 200 {
		t.Errorf("Expected button position (100, 200), got (%d, %d)", importedButton.LayoutX, importedButton.LayoutY)
	}
	if importedButton.Width == nil || *importedButton.Width != 150 {
		t.Errorf("Expected button width 150, got %v", importedButton.Width)
	}
	if importedButton.Color == nil || *importedButton.Color != "#FF5500" {
		t.Errorf("Expected button color '#FF5500', got %v", importedButton.Color)
	}

	// 6. Verify imported cue list and cue
	importedCueLists, _ := cueListRepo.FindByProjectID(ctx, importedProjectID)
	if len(importedCueLists) != 1 {
		t.Fatalf("Expected 1 imported cue list, got %d", len(importedCueLists))
	}

	importedCueList := importedCueLists[0]
	if importedCueList.Name != "Main Cue List" {
		t.Errorf("Expected cue list name 'Main Cue List', got '%s'", importedCueList.Name)
	}
	if !importedCueList.Loop {
		t.Error("Expected cue list Loop to be true")
	}

	importedCues, _ := cueRepo.FindByCueListID(ctx, importedCueList.ID)
	if len(importedCues) != 1 {
		t.Fatalf("Expected 1 imported cue, got %d", len(importedCues))
	}

	importedCue := importedCues[0]
	if importedCue.Name != "Cue 1" {
		t.Errorf("Expected cue name 'Cue 1', got '%s'", importedCue.Name)
	}
	if importedCue.FadeInTime != 3.0 {
		t.Errorf("Expected FadeInTime 3.0, got %f", importedCue.FadeInTime)
	}
}

// TestExportImportRoundTrip_FadeBehavior tests export/import round-trip for fade behavior
func TestExportImportRoundTrip_FadeBehavior(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	projectRepo := repositories.NewProjectRepository(db)
	fixtureRepo := repositories.NewFixtureRepository(db)
	sceneRepo := repositories.NewSceneRepository(db)
	cueListRepo := repositories.NewCueListRepository(db)
	cueRepo := repositories.NewCueRepository(db)

	exportService := export.NewService(projectRepo, fixtureRepo, sceneRepo, cueListRepo, cueRepo)
	importService := importservice.NewService(projectRepo, fixtureRepo, sceneRepo, cueListRepo, cueRepo)
	ctx := context.Background()

	// Create project with fixture definition containing different fade behaviors
	sourceProject := &models.Project{
		ID:   cuid.New(),
		Name: "Fade Behavior Test Project",
	}
	if err := projectRepo.Create(ctx, sourceProject); err != nil {
		t.Fatalf("Failed to create project: %v", err)
	}

	def := &models.FixtureDefinition{
		ID:           cuid.New(),
		Manufacturer: "FadeMfg",
		Model:        "FadeModel",
		Type:         "MOVING_HEAD",
		IsBuiltIn:    false,
	}
	if err := fixtureRepo.CreateDefinition(ctx, def); err != nil {
		t.Fatalf("Failed to create definition: %v", err)
	}

	// Create channels with all three fade behaviors
	channels := []models.ChannelDefinition{
		{ID: cuid.New(), Name: "Intensity", Type: "INTENSITY", Offset: 0, DefinitionID: def.ID, FadeBehavior: "FADE", IsDiscrete: false},
		{ID: cuid.New(), Name: "Gobo Wheel", Type: "GOBO", Offset: 1, DefinitionID: def.ID, FadeBehavior: "SNAP", IsDiscrete: true},
		{ID: cuid.New(), Name: "Strobe", Type: "STROBE", Offset: 2, DefinitionID: def.ID, FadeBehavior: "SNAP_END", IsDiscrete: false},
		{ID: cuid.New(), Name: "Pan", Type: "PAN", Offset: 3, DefinitionID: def.ID, FadeBehavior: "FADE", IsDiscrete: false},
	}
	for _, ch := range channels {
		db.Create(&ch)
	}

	channelCount := 4
	fixture := &models.FixtureInstance{
		ID:           cuid.New(),
		Name:         "Mover 1",
		ProjectID:    sourceProject.ID,
		DefinitionID: def.ID,
		Universe:     1,
		StartChannel: 1,
		ChannelCount: &channelCount,
	}
	if err := fixtureRepo.Create(ctx, fixture); err != nil {
		t.Fatalf("Failed to create fixture: %v", err)
	}

	// Create instance channels
	for _, ch := range channels {
		ic := models.InstanceChannel{
			ID:           cuid.New(),
			Name:         ch.Name,
			Type:         ch.Type,
			Offset:       ch.Offset,
			FixtureID:    fixture.ID,
			FadeBehavior: ch.FadeBehavior,
			IsDiscrete:   ch.IsDiscrete,
		}
		db.Create(&ic)
	}

	// Export
	exported, _, err := exportService.ExportProject(ctx, sourceProject.ID, true, false, false)
	if err != nil {
		t.Fatalf("Export failed: %v", err)
	}

	jsonStr, _ := exported.ToJSON()

	// Import
	importedProjectID, _, _, err := importService.ImportProject(ctx, jsonStr, importservice.ImportOptions{
		Mode:                    importservice.ImportModeCreate,
		FixtureConflictStrategy: importservice.FixtureConflictRename, // Create new definition
	})
	if err != nil {
		t.Fatalf("Import failed: %v", err)
	}

	// Verify imported fixture instance channels have correct fade behaviors
	importedFixtures, _ := fixtureRepo.FindByProjectID(ctx, importedProjectID)
	if len(importedFixtures) != 1 {
		t.Fatalf("Expected 1 fixture, got %d", len(importedFixtures))
	}

	importedChannels, _ := fixtureRepo.GetInstanceChannels(ctx, importedFixtures[0].ID)
	if len(importedChannels) != 4 {
		t.Fatalf("Expected 4 channels, got %d", len(importedChannels))
	}

	fadeBehaviorMap := map[string]string{
		"Intensity":  "FADE",
		"Gobo Wheel": "SNAP",
		"Strobe":     "SNAP_END",
		"Pan":        "FADE",
	}
	discreteMap := map[string]bool{
		"Intensity":  false,
		"Gobo Wheel": true,
		"Strobe":     false,
		"Pan":        false,
	}

	for _, ic := range importedChannels {
		expectedBehavior := fadeBehaviorMap[ic.Name]
		expectedDiscrete := discreteMap[ic.Name]

		if ic.FadeBehavior != expectedBehavior {
			t.Errorf("Channel '%s': expected FadeBehavior '%s', got '%s'", ic.Name, expectedBehavior, ic.FadeBehavior)
		}
		if ic.IsDiscrete != expectedDiscrete {
			t.Errorf("Channel '%s': expected IsDiscrete %v, got %v", ic.Name, expectedDiscrete, ic.IsDiscrete)
		}
	}
}
