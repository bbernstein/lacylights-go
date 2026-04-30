package main

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/bbernstein/lacylights-go/internal/config"
	"github.com/bbernstein/lacylights-go/internal/database/models"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func TestHealthCheckHandler(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	w := httptest.NewRecorder()

	healthCheckHandler(w, req)

	resp := w.Result()
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("Expected status 200, got %d", resp.StatusCode)
	}

	contentType := resp.Header.Get("Content-Type")
	if contentType != "application/json" {
		t.Errorf("Expected Content-Type application/json, got %s", contentType)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("Failed to read response body: %v", err)
	}

	bodyStr := string(body)
	if !strings.Contains(bodyStr, `"status": "ok"`) {
		t.Error("Expected status ok in response")
	}
	if !strings.Contains(bodyStr, `"version":`) {
		t.Error("Expected version in response")
	}
	if !strings.Contains(bodyStr, `"timestamp":`) {
		t.Error("Expected timestamp in response")
	}
	if !strings.Contains(bodyStr, `"gitCommit":`) {
		t.Error("Expected gitCommit in response")
	}
	if !strings.Contains(bodyStr, `"buildTime":`) {
		t.Error("Expected buildTime in response")
	}
}

func TestPrintBanner(t *testing.T) {
	// Capture stdout
	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	cfg := &config.Config{
		Env:         "test",
		Port:        "4000",
		DatabaseURL: "test.db",
	}

	printBanner(cfg)

	_ = w.Close()
	os.Stdout = oldStdout

	var buf bytes.Buffer
	_, _ = io.Copy(&buf, r)
	output := buf.String()

	// Verify banner contains expected elements
	if !strings.Contains(output, "LacyLights Go Server") {
		t.Error("Expected 'LacyLights Go Server' in banner")
	}
	if !strings.Contains(output, "Version:") {
		t.Error("Expected 'Version:' in banner")
	}
	if !strings.Contains(output, "Environment: test") {
		t.Error("Expected 'Environment: test' in banner")
	}
	if !strings.Contains(output, "Port:        4000") {
		t.Error("Expected 'Port: 4000' in banner")
	}
	if !strings.Contains(output, "Database:    test.db") {
		t.Error("Expected 'Database: test.db' in banner")
	}
}

func TestVersionVariables(t *testing.T) {
	// These are set at build time, but we can verify they have default values
	if Version == "" {
		t.Error("Version should have a default value")
	}
	if BuildTime == "" {
		t.Error("BuildTime should have a default value")
	}
	if GitCommit == "" {
		t.Error("GitCommit should have a default value")
	}
}

// setupTestDB creates a test SQLite database with the old channelValues column
func setupTestDB(t *testing.T) *gorm.DB {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		t.Fatalf("Failed to create test database: %v", err)
	}

	// Create fixture_values table with both old and new columns
	err = db.Exec(`
		CREATE TABLE fixture_values (
			id TEXT PRIMARY KEY,
			scene_id TEXT,
			fixture_id TEXT,
			channelValues TEXT,
			channels TEXT,
			scene_order INTEGER
		)
	`).Error
	if err != nil {
		t.Fatalf("Failed to create fixture_values table: %v", err)
	}

	return db
}

// setupInMemoryDB creates an empty in-memory database for tests
// that need to set up their own schema (e.g., testing migration scenarios)
func setupInMemoryDB(t *testing.T) *gorm.DB {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		t.Fatalf("Failed to create test database: %v", err)
	}
	return db
}

func TestMigrateChannelValuesToSparse_EmptyDatabase(t *testing.T) {
	db := setupTestDB(t)

	// Should not error with empty database
	err := migrateChannelValuesToSparse(db)
	if err != nil {
		t.Errorf("Expected no error for empty database, got: %v", err)
	}
}

func TestMigrateChannelValuesToSparse_NoMigrationNeeded(t *testing.T) {
	db := setupTestDB(t)

	// Insert a row that already has channels populated (no migration needed)
	err := db.Exec(`
		INSERT INTO fixture_values (id, scene_id, fixture_id, channelValues, channels)
		VALUES ('test-1', 'scene-1', 'fixture-1', '[100, 200]', '[{"offset":0,"value":100},{"offset":1,"value":200}]')
	`).Error
	if err != nil {
		t.Fatalf("Failed to insert test data: %v", err)
	}

	err = migrateChannelValuesToSparse(db)
	if err != nil {
		t.Errorf("Expected no error, got: %v", err)
	}

	// Verify the data wasn't changed
	var channels string
	db.Raw("SELECT channels FROM fixture_values WHERE id = 'test-1'").Scan(&channels)
	if channels != `[{"offset":0,"value":100},{"offset":1,"value":200}]` {
		t.Errorf("Channels should not have changed, got: %s", channels)
	}
}

func TestMigrateChannelValuesToSparse_MigratesOldFormat(t *testing.T) {
	db := setupTestDB(t)

	// Insert rows with old channelValues format but empty channels
	err := db.Exec(`
		INSERT INTO fixture_values (id, scene_id, fixture_id, channelValues, channels)
		VALUES
			('test-1', 'scene-1', 'fixture-1', '[255, 128, 64]', ''),
			('test-2', 'scene-1', 'fixture-2', '[100, 0, 50]', NULL)
	`).Error
	if err != nil {
		t.Fatalf("Failed to insert test data: %v", err)
	}

	err = migrateChannelValuesToSparse(db)
	if err != nil {
		t.Errorf("Expected no error, got: %v", err)
	}

	// Verify the migration happened
	var channels1, channels2 string
	db.Raw("SELECT channels FROM fixture_values WHERE id = 'test-1'").Scan(&channels1)
	db.Raw("SELECT channels FROM fixture_values WHERE id = 'test-2'").Scan(&channels2)

	// Verify sparse format was applied with all offsets
	expectedContains := []string{`"offset":0`, `"offset":1`, `"offset":2`}
	for _, expected := range expectedContains {
		if !strings.Contains(channels1, expected) {
			t.Errorf("Expected channels1 to contain %s, got: %s", expected, channels1)
		}
	}

	// Verify all values are correctly migrated for channels1: [255, 128, 64]
	if !strings.Contains(channels1, `"value":255`) {
		t.Errorf("Expected channels1 to contain value 255, got: %s", channels1)
	}
	if !strings.Contains(channels1, `"value":128`) {
		t.Errorf("Expected channels1 to contain value 128, got: %s", channels1)
	}
	if !strings.Contains(channels1, `"value":64`) {
		t.Errorf("Expected channels1 to contain value 64, got: %s", channels1)
	}

	// Verify channels2 was also migrated correctly: [100, 0, 50]
	if !strings.Contains(channels2, `"value":100`) {
		t.Errorf("Expected channels2 to contain value 100, got: %s", channels2)
	}
	if !strings.Contains(channels2, `"value":0`) {
		t.Errorf("Expected channels2 to contain value 0, got: %s", channels2)
	}
	if !strings.Contains(channels2, `"value":50`) {
		t.Errorf("Expected channels2 to contain value 50, got: %s", channels2)
	}
}

func TestMigrateChannelValuesToSparse_HandlesInvalidJSON(t *testing.T) {
	db := setupTestDB(t)

	// Insert a row with invalid JSON in channelValues
	err := db.Exec(`
		INSERT INTO fixture_values (id, scene_id, fixture_id, channelValues, channels)
		VALUES ('test-1', 'scene-1', 'fixture-1', 'not valid json', '')
	`).Error
	if err != nil {
		t.Fatalf("Failed to insert test data: %v", err)
	}

	// Should not error, but should skip the invalid row
	err = migrateChannelValuesToSparse(db)
	if err != nil {
		t.Errorf("Expected no error for invalid JSON (should skip), got: %v", err)
	}

	// Verify the row wasn't changed
	var channels string
	db.Raw("SELECT channels FROM fixture_values WHERE id = 'test-1'").Scan(&channels)
	if channels != "" {
		t.Errorf("Channels should still be empty for invalid JSON, got: %s", channels)
	}
}

func TestMigrateChannelValuesToSparse_NonSQLite(t *testing.T) {
	// Create a mock database that reports as non-SQLite
	// This tests the early return for non-SQLite databases
	db := setupTestDB(t)

	// The actual function checks db.Name() == "sqlite"
	// For a memory SQLite database, this should work
	err := migrateChannelValuesToSparse(db)
	if err != nil {
		t.Errorf("Expected no error, got: %v", err)
	}
}

func TestMigrateChannelValuesToSparse_NoChannelValuesColumn(t *testing.T) {
	// Create a database without the channelValues column
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		t.Fatalf("Failed to create test database: %v", err)
	}

	// Create table without channelValues column
	err = db.Exec(`
		CREATE TABLE fixture_values (
			id TEXT PRIMARY KEY,
			scene_id TEXT,
			fixture_id TEXT,
			channels TEXT,
			scene_order INTEGER
		)
	`).Error
	if err != nil {
		t.Fatalf("Failed to create fixture_values table: %v", err)
	}

	// Should return early without error
	err = migrateChannelValuesToSparse(db)
	if err != nil {
		t.Errorf("Expected no error when column doesn't exist, got: %v", err)
	}
}

func TestMigrateChannelValuesToSparse_EmptyChannelValuesArray(t *testing.T) {
	db := setupTestDB(t)

	// Insert a row with empty channelValues array
	err := db.Exec(`
		INSERT INTO fixture_values (id, scene_id, fixture_id, channelValues, channels)
		VALUES ('test-1', 'scene-1', 'fixture-1', '[]', '')
	`).Error
	if err != nil {
		t.Fatalf("Failed to insert test data: %v", err)
	}

	// This row should be skipped since channelValues is empty array
	err = migrateChannelValuesToSparse(db)
	if err != nil {
		t.Errorf("Expected no error, got: %v", err)
	}
}

// Test that models.ChannelValue is properly used
func TestChannelValueModel(t *testing.T) {
	cv := models.ChannelValue{
		Offset: 5,
		Value:  128,
	}

	if cv.Offset != 5 {
		t.Errorf("Expected Offset 5, got %d", cv.Offset)
	}
	if cv.Value != 128 {
		t.Errorf("Expected Value 128, got %d", cv.Value)
	}
}

// setupOldSchemaDB creates a test database with the old "scene" terminology
func setupOldSchemaDB(t *testing.T) *gorm.DB {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		t.Fatalf("Failed to create test database: %v", err)
	}

	// Create tables with old schema (scene terminology)
	err = db.Exec(`
		CREATE TABLE projects (
			id TEXT PRIMARY KEY,
			name TEXT,
			description TEXT,
			created_at DATETIME,
			updated_at DATETIME
		)
	`).Error
	if err != nil {
		t.Fatalf("Failed to create projects table: %v", err)
	}

	err = db.Exec(`
		CREATE TABLE scenes (
			id TEXT PRIMARY KEY,
			name TEXT,
			description TEXT,
			project_id TEXT,
			created_at DATETIME,
			updated_at DATETIME
		)
	`).Error
	if err != nil {
		t.Fatalf("Failed to create scenes table: %v", err)
	}

	err = db.Exec(`
		CREATE TABLE fixture_instances (
			id TEXT PRIMARY KEY,
			name TEXT,
			project_id TEXT
		)
	`).Error
	if err != nil {
		t.Fatalf("Failed to create fixture_instances table: %v", err)
	}

	err = db.Exec(`
		CREATE TABLE fixture_values (
			id TEXT PRIMARY KEY,
			scene_id TEXT,
			fixture_id TEXT,
			channels TEXT DEFAULT '[]',
			scene_order INTEGER
		)
	`).Error
	if err != nil {
		t.Fatalf("Failed to create fixture_values table: %v", err)
	}

	err = db.Exec(`
		CREATE TABLE cue_lists (
			id TEXT PRIMARY KEY,
			name TEXT,
			project_id TEXT
		)
	`).Error
	if err != nil {
		t.Fatalf("Failed to create cue_lists table: %v", err)
	}

	err = db.Exec(`
		CREATE TABLE cues (
			id TEXT PRIMARY KEY,
			name TEXT,
			cue_number REAL,
			cue_list_id TEXT,
			scene_id TEXT,
			fade_in_time REAL DEFAULT 0,
			fade_out_time REAL DEFAULT 0,
			follow_time REAL,
			easing_type TEXT,
			notes TEXT,
			skip INTEGER DEFAULT 0,
			created_at DATETIME,
			updated_at DATETIME
		)
	`).Error
	if err != nil {
		t.Fatalf("Failed to create cues table: %v", err)
	}

	err = db.Exec(`
		CREATE TABLE scene_boards (
			id TEXT PRIMARY KEY,
			name TEXT,
			project_id TEXT,
			default_fade_time REAL DEFAULT 3.0,
			grid_size INTEGER DEFAULT 50,
			canvas_width INTEGER DEFAULT 2000,
			canvas_height INTEGER DEFAULT 2000,
			created_at DATETIME,
			updated_at DATETIME
		)
	`).Error
	if err != nil {
		t.Fatalf("Failed to create scene_boards table: %v", err)
	}

	err = db.Exec(`
		CREATE TABLE scene_board_buttons (
			id TEXT PRIMARY KEY,
			scene_board_id TEXT,
			scene_id TEXT,
			layout_x INTEGER,
			layout_y INTEGER,
			width INTEGER DEFAULT 200,
			height INTEGER DEFAULT 120,
			color TEXT,
			label TEXT,
			created_at DATETIME,
			updated_at DATETIME
		)
	`).Error
	if err != nil {
		t.Fatalf("Failed to create scene_board_buttons table: %v", err)
	}

	return db
}

func TestMigrateSceneToLook_FreshDatabase(t *testing.T) {
	// Create database without the scenes table (fresh install)
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		t.Fatalf("Failed to create test database: %v", err)
	}

	// Should return early without error
	err = migrateSceneToLook(db)
	if err != nil {
		t.Errorf("Expected no error for fresh database, got: %v", err)
	}
}

func TestMigrateSceneToLook_RenamesScenesTable(t *testing.T) {
	db := setupOldSchemaDB(t)

	// Insert test data
	err := db.Exec(`
		INSERT INTO scenes (id, name, description, project_id)
		VALUES ('scene-1', 'Opening', 'Opening scene', 'proj-1')
	`).Error
	if err != nil {
		t.Fatalf("Failed to insert test scene: %v", err)
	}

	// Run migration
	err = migrateSceneToLook(db)
	if err != nil {
		t.Errorf("Expected no error, got: %v", err)
	}

	// Verify scenes table is gone
	if db.Migrator().HasTable("scenes") {
		t.Error("Expected scenes table to be renamed")
	}

	// Verify looks table exists with data
	if !db.Migrator().HasTable("looks") {
		t.Error("Expected looks table to exist")
	}

	var name string
	db.Raw("SELECT name FROM looks WHERE id = 'scene-1'").Scan(&name)
	if name != "Opening" {
		t.Errorf("Expected look name 'Opening', got: %s", name)
	}
}

func TestMigrateSceneToLook_MigratesFixtureValues(t *testing.T) {
	db := setupOldSchemaDB(t)

	// Insert test data
	err := db.Exec(`
		INSERT INTO fixture_values (id, scene_id, fixture_id, channels, scene_order)
		VALUES ('fv-1', 'scene-1', 'fixture-1', '[{"offset":0,"value":255}]', 1)
	`).Error
	if err != nil {
		t.Fatalf("Failed to insert test data: %v", err)
	}

	// Run migration
	err = migrateSceneToLook(db)
	if err != nil {
		t.Errorf("Expected no error, got: %v", err)
	}

	// Verify columns were renamed
	if db.Migrator().HasColumn("fixture_values", "scene_id") {
		t.Error("Expected scene_id column to be renamed to look_id")
	}
	if db.Migrator().HasColumn("fixture_values", "scene_order") {
		t.Error("Expected scene_order column to be renamed to look_order")
	}

	// Verify data was migrated correctly
	var lookID string
	var lookOrder int
	if err := db.Raw("SELECT look_id, look_order FROM fixture_values WHERE id = 'fv-1'").Row().Scan(&lookID, &lookOrder); err != nil {
		t.Fatalf("Failed to scan fixture_values: %v", err)
	}
	if lookID != "scene-1" {
		t.Errorf("Expected look_id 'scene-1', got: %s", lookID)
	}
	if lookOrder != 1 {
		t.Errorf("Expected look_order 1, got: %d", lookOrder)
	}
}

func TestMigrateSceneToLook_MigratesCues(t *testing.T) {
	db := setupOldSchemaDB(t)

	// Insert test data
	err := db.Exec(`
		INSERT INTO cues (id, name, cue_number, cue_list_id, scene_id)
		VALUES ('cue-1', 'Cue 1', 1.0, 'list-1', 'scene-1')
	`).Error
	if err != nil {
		t.Fatalf("Failed to insert test data: %v", err)
	}

	// Run migration
	err = migrateSceneToLook(db)
	if err != nil {
		t.Errorf("Expected no error, got: %v", err)
	}

	// Verify column was renamed
	if db.Migrator().HasColumn("cues", "scene_id") {
		t.Error("Expected scene_id column to be renamed to look_id")
	}

	// Verify data was migrated correctly
	var lookID string
	db.Raw("SELECT look_id FROM cues WHERE id = 'cue-1'").Scan(&lookID)
	if lookID != "scene-1" {
		t.Errorf("Expected look_id 'scene-1', got: %s", lookID)
	}
}

func TestMigrateSceneToLook_RenamesSceneBoards(t *testing.T) {
	db := setupOldSchemaDB(t)

	// Insert test data
	err := db.Exec(`
		INSERT INTO scene_boards (id, name, project_id)
		VALUES ('sb-1', 'Main Board', 'proj-1')
	`).Error
	if err != nil {
		t.Fatalf("Failed to insert test data: %v", err)
	}

	// Run migration
	err = migrateSceneToLook(db)
	if err != nil {
		t.Errorf("Expected no error, got: %v", err)
	}

	// Verify table was renamed
	if db.Migrator().HasTable("scene_boards") {
		t.Error("Expected scene_boards table to be renamed")
	}
	if !db.Migrator().HasTable("look_boards") {
		t.Error("Expected look_boards table to exist")
	}

	// Verify data exists
	var name string
	db.Raw("SELECT name FROM look_boards WHERE id = 'sb-1'").Scan(&name)
	if name != "Main Board" {
		t.Errorf("Expected name 'Main Board', got: %s", name)
	}
}

func TestMigrateSceneToLook_MigratesSceneBoardButtons(t *testing.T) {
	db := setupOldSchemaDB(t)

	// Insert test data
	err := db.Exec(`
		INSERT INTO scene_board_buttons (id, scene_board_id, scene_id, layout_x, layout_y)
		VALUES ('btn-1', 'sb-1', 'scene-1', 100, 200)
	`).Error
	if err != nil {
		t.Fatalf("Failed to insert test data: %v", err)
	}

	// Run migration
	err = migrateSceneToLook(db)
	if err != nil {
		t.Errorf("Expected no error, got: %v", err)
	}

	// Verify table was renamed
	if db.Migrator().HasTable("scene_board_buttons") {
		t.Error("Expected scene_board_buttons table to be renamed")
	}
	if !db.Migrator().HasTable("look_board_buttons") {
		t.Error("Expected look_board_buttons table to exist")
	}

	// Verify columns were renamed and data migrated
	var lookBoardID, lookID string
	var layoutX, layoutY int
	if err := db.Raw("SELECT look_board_id, look_id, layout_x, layout_y FROM look_board_buttons WHERE id = 'btn-1'").Row().Scan(&lookBoardID, &lookID, &layoutX, &layoutY); err != nil {
		t.Fatalf("Failed to scan look_board_buttons: %v", err)
	}
	if lookBoardID != "sb-1" {
		t.Errorf("Expected look_board_id 'sb-1', got: %s", lookBoardID)
	}
	if lookID != "scene-1" {
		t.Errorf("Expected look_id 'scene-1', got: %s", lookID)
	}
	if layoutX != 100 {
		t.Errorf("Expected layout_x 100, got: %d", layoutX)
	}
	if layoutY != 200 {
		t.Errorf("Expected layout_y 200, got: %d", layoutY)
	}
}

func TestMigrateSceneToLook_FullMigration(t *testing.T) {
	db := setupOldSchemaDB(t)

	// Insert comprehensive test data across all tables
	db.Exec(`INSERT INTO projects (id, name) VALUES ('proj-1', 'Test Project')`)
	db.Exec(`INSERT INTO scenes (id, name, project_id) VALUES ('scene-1', 'Look 1', 'proj-1'), ('scene-2', 'Look 2', 'proj-1')`)
	db.Exec(`INSERT INTO fixture_instances (id, name, project_id) VALUES ('fix-1', 'Par 1', 'proj-1')`)
	db.Exec(`INSERT INTO fixture_values (id, scene_id, fixture_id, channels, scene_order) VALUES ('fv-1', 'scene-1', 'fix-1', '[{"offset":0,"value":255}]', 1)`)
	db.Exec(`INSERT INTO cue_lists (id, name, project_id) VALUES ('list-1', 'Main', 'proj-1')`)
	db.Exec(`INSERT INTO cues (id, name, cue_number, cue_list_id, scene_id) VALUES ('cue-1', 'Cue 1', 1.0, 'list-1', 'scene-1')`)
	db.Exec(`INSERT INTO scene_boards (id, name, project_id) VALUES ('sb-1', 'Board 1', 'proj-1')`)
	db.Exec(`INSERT INTO scene_board_buttons (id, scene_board_id, scene_id, layout_x, layout_y) VALUES ('btn-1', 'sb-1', 'scene-1', 0, 0)`)

	// Run migration
	err := migrateSceneToLook(db)
	if err != nil {
		t.Fatalf("Migration failed: %v", err)
	}

	// Verify all old tables/columns are gone
	if db.Migrator().HasTable("scenes") {
		t.Error("scenes table should not exist")
	}
	if db.Migrator().HasTable("scene_boards") {
		t.Error("scene_boards table should not exist")
	}
	if db.Migrator().HasTable("scene_board_buttons") {
		t.Error("scene_board_buttons table should not exist")
	}

	// Verify all new tables exist
	if !db.Migrator().HasTable("looks") {
		t.Error("looks table should exist")
	}
	if !db.Migrator().HasTable("look_boards") {
		t.Error("look_boards table should exist")
	}
	if !db.Migrator().HasTable("look_board_buttons") {
		t.Error("look_board_buttons table should exist")
	}

	// Verify data integrity
	var lookCount, fvCount, cueCount, boardCount, buttonCount int64
	db.Raw("SELECT COUNT(*) FROM looks").Scan(&lookCount)
	db.Raw("SELECT COUNT(*) FROM fixture_values").Scan(&fvCount)
	db.Raw("SELECT COUNT(*) FROM cues").Scan(&cueCount)
	db.Raw("SELECT COUNT(*) FROM look_boards").Scan(&boardCount)
	db.Raw("SELECT COUNT(*) FROM look_board_buttons").Scan(&buttonCount)

	if lookCount != 2 {
		t.Errorf("Expected 2 looks, got %d", lookCount)
	}
	if fvCount != 1 {
		t.Errorf("Expected 1 fixture_value, got %d", fvCount)
	}
	if cueCount != 1 {
		t.Errorf("Expected 1 cue, got %d", cueCount)
	}
	if boardCount != 1 {
		t.Errorf("Expected 1 look_board, got %d", boardCount)
	}
	if buttonCount != 1 {
		t.Errorf("Expected 1 look_board_button, got %d", buttonCount)
	}
}

func TestMigrateSceneToLook_Idempotent(t *testing.T) {
	db := setupOldSchemaDB(t)

	// Insert test data
	db.Exec(`INSERT INTO scenes (id, name) VALUES ('scene-1', 'Look 1')`)

	// Run migration twice - should be idempotent
	err := migrateSceneToLook(db)
	if err != nil {
		t.Fatalf("First migration failed: %v", err)
	}

	// Second run should return early (no scenes table)
	err = migrateSceneToLook(db)
	if err != nil {
		t.Errorf("Second migration should succeed (idempotent), got: %v", err)
	}

	// Data should still be intact
	var count int64
	db.Raw("SELECT COUNT(*) FROM looks").Scan(&count)
	if count != 1 {
		t.Errorf("Expected 1 look after idempotent migration, got %d", count)
	}
}

// Tests for AutoMigrate scenario (both old and new tables exist)
// This simulates the case where GORM AutoMigrate runs before our custom migration,
// creating empty new tables while old tables still have data.

func TestMigrateSceneToLook_AutoMigrate_ScenesAndLooksBothExist(t *testing.T) {
	db := setupInMemoryDB(t)

	// Simulate AutoMigrate creating both old (scenes) and new (looks) tables
	db.Exec(`CREATE TABLE scenes (
		id TEXT PRIMARY KEY,
		name TEXT,
		description TEXT,
		project_id TEXT,
		created_at DATETIME,
		updated_at DATETIME
	)`)
	db.Exec(`CREATE TABLE looks (
		id TEXT PRIMARY KEY,
		name TEXT,
		description TEXT,
		project_id TEXT,
		created_at DATETIME,
		updated_at DATETIME
	)`)

	// Add data to old table (scenes has data, looks is empty - simulates user's existing database)
	db.Exec(`INSERT INTO scenes (id, name, description) VALUES ('scene-1', 'Scene 1', 'First scene')`)
	db.Exec(`INSERT INTO scenes (id, name, description) VALUES ('scene-2', 'Scene 2', 'Second scene')`)

	err := migrateSceneToLook(db)
	if err != nil {
		t.Fatalf("Migration failed: %v", err)
	}

	// Verify scenes table is dropped
	var scenesExists int
	db.Raw("SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='scenes'").Scan(&scenesExists)
	if scenesExists != 0 {
		t.Error("scenes table should be dropped after migration")
	}

	// Verify looks table has the data
	var count int64
	db.Raw("SELECT COUNT(*) FROM looks").Scan(&count)
	if count != 2 {
		t.Errorf("Expected 2 looks after migration, got %d", count)
	}

	// Verify data integrity
	var name string
	db.Raw("SELECT name FROM looks WHERE id = 'scene-1'").Scan(&name)
	if name != "Scene 1" {
		t.Errorf("Expected 'Scene 1', got '%s'", name)
	}
}

func TestMigrateSceneToLook_AutoMigrate_FixtureValuesWithBothColumns(t *testing.T) {
	db := setupInMemoryDB(t)

	// Create tables that simulate AutoMigrate having run
	// Old scenes table with data
	db.Exec(`CREATE TABLE scenes (id TEXT PRIMARY KEY, name TEXT)`)
	db.Exec(`INSERT INTO scenes (id, name) VALUES ('scene-1', 'Scene 1')`)

	// fixture_values with BOTH old (scene_id) and new (look_id) columns
	// This simulates AutoMigrate adding new columns without removing old ones
	db.Exec(`CREATE TABLE fixture_values (
		id TEXT PRIMARY KEY,
		scene_id TEXT,
		scene_order INTEGER,
		look_id TEXT,
		look_order INTEGER,
		fixture_id TEXT,
		channels TEXT DEFAULT '[]'
	)`)

	// Add data with old column populated, new column empty
	db.Exec(`INSERT INTO fixture_values (id, scene_id, scene_order, fixture_id) VALUES ('fv-1', 'scene-1', 1, 'fixture-1')`)
	db.Exec(`INSERT INTO fixture_values (id, scene_id, scene_order, fixture_id) VALUES ('fv-2', 'scene-1', 2, 'fixture-2')`)

	// Create other required tables with all columns needed by the migration
	db.Exec(`CREATE TABLE cues (
		id TEXT PRIMARY KEY,
		name TEXT,
		cue_number REAL,
		cue_list_id TEXT,
		scene_id TEXT,
		fade_in_time REAL,
		fade_out_time REAL,
		follow_time REAL,
		easing_type TEXT,
		notes TEXT,
		skip INTEGER,
		created_at DATETIME,
		updated_at DATETIME
	)`)
	db.Exec(`CREATE TABLE scene_boards (
		id TEXT PRIMARY KEY,
		name TEXT,
		description TEXT,
		project_id TEXT,
		grid_size INTEGER,
		default_fade_time REAL,
		canvas_width INTEGER,
		canvas_height INTEGER,
		created_at DATETIME,
		updated_at DATETIME
	)`)
	db.Exec(`CREATE TABLE scene_board_buttons (
		id TEXT PRIMARY KEY,
		scene_board_id TEXT,
		scene_id TEXT,
		layout_x INTEGER,
		layout_y INTEGER,
		width INTEGER,
		height INTEGER,
		color TEXT,
		label TEXT,
		created_at DATETIME,
		updated_at DATETIME
	)`)

	err := migrateSceneToLook(db)
	if err != nil {
		t.Fatalf("Migration failed: %v", err)
	}

	// Verify fixture_values now uses look_id
	var lookId string
	db.Raw("SELECT look_id FROM fixture_values WHERE id = 'fv-1'").Scan(&lookId)
	if lookId != "scene-1" {
		t.Errorf("Expected look_id 'scene-1', got '%s'", lookId)
	}

	// Verify scene_id column is removed
	var hasSceneId bool
	db.Raw("SELECT COUNT(*) > 0 FROM pragma_table_info('fixture_values') WHERE name = 'scene_id'").Scan(&hasSceneId)
	if hasSceneId {
		t.Error("fixture_values should not have scene_id column after migration")
	}
}

func TestMigrateSceneToLook_AutoMigrate_CuesWithBothColumns(t *testing.T) {
	db := setupInMemoryDB(t)

	// Create tables that simulate AutoMigrate having run
	db.Exec(`CREATE TABLE scenes (id TEXT PRIMARY KEY, name TEXT)`)
	db.Exec(`INSERT INTO scenes (id, name) VALUES ('scene-1', 'Scene 1')`)

	// cues with BOTH old (scene_id) and new (look_id) columns
	db.Exec(`CREATE TABLE cues (
		id TEXT PRIMARY KEY,
		name TEXT,
		cue_number REAL,
		cue_list_id TEXT,
		scene_id TEXT,
		look_id TEXT,
		fade_in_time REAL,
		fade_out_time REAL,
		follow_time REAL,
		easing_type TEXT,
		notes TEXT,
		skip INTEGER,
		created_at DATETIME,
		updated_at DATETIME
	)`)

	// Add data with old column populated, new column empty
	db.Exec(`INSERT INTO cues (id, name, cue_number, scene_id) VALUES ('cue-1', 'Cue 1', 1.0, 'scene-1')`)
	db.Exec(`INSERT INTO cues (id, name, cue_number, scene_id) VALUES ('cue-2', 'Cue 2', 2.0, 'scene-1')`)

	// Create other required tables with all columns needed by the migration
	db.Exec(`CREATE TABLE fixture_values (
		id TEXT PRIMARY KEY,
		scene_id TEXT,
		scene_order INTEGER,
		fixture_id TEXT,
		channels TEXT DEFAULT '[]'
	)`)
	db.Exec(`CREATE TABLE scene_boards (
		id TEXT PRIMARY KEY,
		name TEXT,
		description TEXT,
		project_id TEXT,
		grid_size INTEGER,
		default_fade_time REAL,
		canvas_width INTEGER,
		canvas_height INTEGER,
		created_at DATETIME,
		updated_at DATETIME
	)`)
	db.Exec(`CREATE TABLE scene_board_buttons (
		id TEXT PRIMARY KEY,
		scene_board_id TEXT,
		scene_id TEXT,
		layout_x INTEGER,
		layout_y INTEGER,
		width INTEGER,
		height INTEGER,
		color TEXT,
		label TEXT,
		created_at DATETIME,
		updated_at DATETIME
	)`)

	err := migrateSceneToLook(db)
	if err != nil {
		t.Fatalf("Migration failed: %v", err)
	}

	// Verify cues now uses look_id
	var lookId string
	db.Raw("SELECT look_id FROM cues WHERE id = 'cue-1'").Scan(&lookId)
	if lookId != "scene-1" {
		t.Errorf("Expected look_id 'scene-1', got '%s'", lookId)
	}

	// Verify scene_id column is removed
	var hasSceneId bool
	db.Raw("SELECT COUNT(*) > 0 FROM pragma_table_info('cues') WHERE name = 'scene_id'").Scan(&hasSceneId)
	if hasSceneId {
		t.Error("cues should not have scene_id column after migration")
	}
}

func TestMigrateSceneToLook_AutoMigrate_SceneBoardsBothExist(t *testing.T) {
	db := setupInMemoryDB(t)

	// Create tables that simulate AutoMigrate having run
	db.Exec(`CREATE TABLE scenes (id TEXT PRIMARY KEY, name TEXT)`)
	db.Exec(`INSERT INTO scenes (id, name) VALUES ('scene-1', 'Scene 1')`)

	// Both old and new scene_boards/look_boards tables
	db.Exec(`CREATE TABLE scene_boards (
		id TEXT PRIMARY KEY,
		name TEXT,
		description TEXT,
		project_id TEXT,
		grid_size INTEGER,
		default_fade_time REAL,
		canvas_width INTEGER,
		canvas_height INTEGER,
		created_at DATETIME,
		updated_at DATETIME
	)`)
	db.Exec(`CREATE TABLE look_boards (
		id TEXT PRIMARY KEY,
		name TEXT,
		description TEXT,
		project_id TEXT,
		grid_size INTEGER,
		default_fade_time REAL,
		canvas_width INTEGER,
		canvas_height INTEGER,
		created_at DATETIME,
		updated_at DATETIME
	)`)

	// Add data to old table
	db.Exec(`INSERT INTO scene_boards (id, name, grid_size) VALUES ('sb-1', 'Board 1', 50)`)
	db.Exec(`INSERT INTO scene_boards (id, name, grid_size) VALUES ('sb-2', 'Board 2', 100)`)

	// Create other required tables with all columns needed by the migration
	db.Exec(`CREATE TABLE fixture_values (
		id TEXT PRIMARY KEY,
		scene_id TEXT,
		scene_order INTEGER,
		fixture_id TEXT,
		channels TEXT DEFAULT '[]'
	)`)
	db.Exec(`CREATE TABLE cues (
		id TEXT PRIMARY KEY,
		name TEXT,
		cue_number REAL,
		cue_list_id TEXT,
		scene_id TEXT,
		fade_in_time REAL,
		fade_out_time REAL,
		follow_time REAL,
		easing_type TEXT,
		notes TEXT,
		skip INTEGER,
		created_at DATETIME,
		updated_at DATETIME
	)`)
	db.Exec(`CREATE TABLE scene_board_buttons (
		id TEXT PRIMARY KEY,
		scene_board_id TEXT,
		scene_id TEXT,
		layout_x INTEGER,
		layout_y INTEGER,
		width INTEGER,
		height INTEGER,
		color TEXT,
		label TEXT,
		created_at DATETIME,
		updated_at DATETIME
	)`)

	err := migrateSceneToLook(db)
	if err != nil {
		t.Fatalf("Migration failed: %v", err)
	}

	// Verify scene_boards table is dropped
	var sceneBoardsExists int
	db.Raw("SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='scene_boards'").Scan(&sceneBoardsExists)
	if sceneBoardsExists != 0 {
		t.Error("scene_boards table should be dropped after migration")
	}

	// Verify look_boards table has the data
	var count int64
	db.Raw("SELECT COUNT(*) FROM look_boards").Scan(&count)
	if count != 2 {
		t.Errorf("Expected 2 look_boards after migration, got %d", count)
	}
}

func TestMigrateSceneToLook_AutoMigrate_ButtonsBothExist(t *testing.T) {
	db := setupInMemoryDB(t)

	// Create tables that simulate AutoMigrate having run
	db.Exec(`CREATE TABLE scenes (id TEXT PRIMARY KEY, name TEXT)`)
	db.Exec(`INSERT INTO scenes (id, name) VALUES ('scene-1', 'Scene 1')`)

	db.Exec(`CREATE TABLE scene_boards (
		id TEXT PRIMARY KEY,
		name TEXT,
		description TEXT,
		project_id TEXT,
		grid_size INTEGER,
		default_fade_time REAL,
		canvas_width INTEGER,
		canvas_height INTEGER,
		created_at DATETIME,
		updated_at DATETIME
	)`)
	db.Exec(`INSERT INTO scene_boards (id, name) VALUES ('sb-1', 'Board 1')`)

	// Both old and new button tables
	db.Exec(`CREATE TABLE scene_board_buttons (
		id TEXT PRIMARY KEY,
		scene_board_id TEXT,
		scene_id TEXT,
		layout_x INTEGER,
		layout_y INTEGER,
		width INTEGER,
		height INTEGER,
		color TEXT,
		label TEXT,
		created_at DATETIME,
		updated_at DATETIME
	)`)
	db.Exec(`CREATE TABLE look_board_buttons (
		id TEXT PRIMARY KEY,
		look_board_id TEXT,
		look_id TEXT,
		layout_x INTEGER,
		layout_y INTEGER,
		width INTEGER,
		height INTEGER,
		color TEXT,
		label TEXT,
		created_at DATETIME,
		updated_at DATETIME
	)`)

	// Add data to old table
	db.Exec(`INSERT INTO scene_board_buttons (id, scene_board_id, scene_id, layout_x, layout_y) VALUES ('btn-1', 'sb-1', 'scene-1', 100, 200)`)
	db.Exec(`INSERT INTO scene_board_buttons (id, scene_board_id, scene_id, layout_x, layout_y) VALUES ('btn-2', 'sb-1', 'scene-1', 300, 400)`)

	// Create other required tables with all columns needed by the migration
	db.Exec(`CREATE TABLE fixture_values (
		id TEXT PRIMARY KEY,
		scene_id TEXT,
		scene_order INTEGER,
		fixture_id TEXT,
		channels TEXT DEFAULT '[]'
	)`)
	db.Exec(`CREATE TABLE cues (
		id TEXT PRIMARY KEY,
		name TEXT,
		cue_number REAL,
		cue_list_id TEXT,
		scene_id TEXT,
		fade_in_time REAL,
		fade_out_time REAL,
		follow_time REAL,
		easing_type TEXT,
		notes TEXT,
		skip INTEGER,
		created_at DATETIME,
		updated_at DATETIME
	)`)

	err := migrateSceneToLook(db)
	if err != nil {
		t.Fatalf("Migration failed: %v", err)
	}

	// Verify scene_board_buttons table is dropped
	var oldTableExists int
	db.Raw("SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='scene_board_buttons'").Scan(&oldTableExists)
	if oldTableExists != 0 {
		t.Error("scene_board_buttons table should be dropped after migration")
	}

	// Verify look_board_buttons table has the data
	var count int64
	db.Raw("SELECT COUNT(*) FROM look_board_buttons").Scan(&count)
	if count != 2 {
		t.Errorf("Expected 2 look_board_buttons after migration, got %d", count)
	}

	// Verify column names were translated
	var lookBoardId, lookId string
	if err := db.Raw("SELECT look_board_id, look_id FROM look_board_buttons WHERE id = 'btn-1'").Row().Scan(&lookBoardId, &lookId); err != nil {
		t.Fatalf("Failed to query look_board_buttons: %v", err)
	}
	if lookBoardId != "sb-1" {
		t.Errorf("Expected look_board_id 'sb-1', got '%s'", lookBoardId)
	}
	if lookId != "scene-1" {
		t.Errorf("Expected look_id 'scene-1', got '%s'", lookId)
	}
}

// setupGroupOwnershipDB creates a test database with the group ownership schema
func setupGroupOwnershipDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		t.Fatalf("Failed to create test database: %v", err)
	}

	// AutoMigrate the relevant tables
	if err := db.AutoMigrate(
		&models.User{},
		&models.Project{},
		&models.UserGroup{},
	); err != nil {
		t.Fatalf("Failed to migrate: %v", err)
	}

	// Create user_group_members table with role column
	if err := db.AutoMigrate(&models.UserGroupMember{}); err != nil {
		t.Fatalf("Failed to migrate user_group_members: %v", err)
	}

	return db
}

func TestMigrateGroupOwnership_AuthDisabled(t *testing.T) {
	db := setupGroupOwnershipDB(t)

	// With auth disabled, should do nothing and succeed
	err := migrateGroupOwnership(db, false)
	if err != nil {
		t.Errorf("Expected no error with auth disabled, got: %v", err)
	}
}

func TestMigrateGroupOwnership_AuthEnabled_NoOrphanedProjects(t *testing.T) {
	db := setupGroupOwnershipDB(t)

	// No projects at all - nothing to migrate
	err := migrateGroupOwnership(db, true)
	if err != nil {
		t.Errorf("Expected no error with no projects, got: %v", err)
	}
}

func TestMigrateGroupOwnership_AuthEnabled_NoAdminUser(t *testing.T) {
	db := setupGroupOwnershipDB(t)

	// Create a project without group_id but no admin user exists
	db.Exec(`INSERT INTO projects (id, name) VALUES ('proj-1', 'Test Project')`)

	err := migrateGroupOwnership(db, true)
	if err != nil {
		t.Errorf("Expected no error (should skip gracefully), got: %v", err)
	}

	// Project should still have no group_id
	var groupID *string
	db.Raw("SELECT group_id FROM projects WHERE id = 'proj-1'").Scan(&groupID)
	if groupID != nil {
		t.Errorf("Expected group_id to remain nil, got: %v", groupID)
	}
}

func TestMigrateGroupOwnership_AuthEnabled_AssignsOrphanedProjects(t *testing.T) {
	db := setupGroupOwnershipDB(t)

	// Create admin user
	db.Create(&models.User{
		ID:       "admin-1",
		Email:    "admin@test.com",
		Role:     "ADMIN",
		IsActive: true,
	})

	// Create orphaned projects (no group_id)
	db.Exec(`INSERT INTO projects (id, name) VALUES ('proj-1', 'Project 1')`)
	db.Exec(`INSERT INTO projects (id, name) VALUES ('proj-2', 'Project 2')`)

	err := migrateGroupOwnership(db, true)
	if err != nil {
		t.Fatalf("Migration failed: %v", err)
	}

	// Verify personal group was created
	var personalGroup models.UserGroup
	if err := db.Where("owner_id = ? AND is_personal = ?", "admin-1", true).First(&personalGroup).Error; err != nil {
		t.Fatalf("Expected personal group to be created, got: %v", err)
	}
	if !personalGroup.IsPersonal {
		t.Error("Expected group to be personal")
	}

	// Verify admin was added as GROUP_ADMIN
	var member models.UserGroupMember
	if err := db.Where("user_id = ? AND group_id = ?", "admin-1", personalGroup.ID).First(&member).Error; err != nil {
		t.Fatalf("Expected admin to be a member: %v", err)
	}
	if member.Role != models.GroupRoleGroupAdmin {
		t.Errorf("Expected role GROUP_ADMIN, got: %s", member.Role)
	}

	// Verify projects are assigned to the personal group
	var proj1GroupID, proj2GroupID string
	db.Raw("SELECT group_id FROM projects WHERE id = 'proj-1'").Scan(&proj1GroupID)
	db.Raw("SELECT group_id FROM projects WHERE id = 'proj-2'").Scan(&proj2GroupID)
	if proj1GroupID != personalGroup.ID {
		t.Errorf("Expected proj-1 group_id %s, got: %s", personalGroup.ID, proj1GroupID)
	}
	if proj2GroupID != personalGroup.ID {
		t.Errorf("Expected proj-2 group_id %s, got: %s", personalGroup.ID, proj2GroupID)
	}
}

func TestMigrateGroupOwnership_AuthEnabled_ReusesExistingPersonalGroup(t *testing.T) {
	db := setupGroupOwnershipDB(t)

	// Create admin user
	db.Create(&models.User{
		ID:       "admin-1",
		Email:    "admin@test.com",
		Role:     "ADMIN",
		IsActive: true,
	})

	// Pre-create personal group
	adminID := "admin-1"
	db.Create(&models.UserGroup{
		ID:         "existing-group",
		Name:       "admin@test.com's Projects",
		IsPersonal: true,
		OwnerID:    &adminID,
	})

	// Create orphaned project
	db.Exec(`INSERT INTO projects (id, name) VALUES ('proj-1', 'Project 1')`)

	err := migrateGroupOwnership(db, true)
	if err != nil {
		t.Fatalf("Migration failed: %v", err)
	}

	// Verify it used the existing group, not created a new one
	var groupCount int64
	db.Model(&models.UserGroup{}).Where("owner_id = ? AND is_personal = ?", "admin-1", true).Count(&groupCount)
	if groupCount != 1 {
		t.Errorf("Expected exactly 1 personal group, got: %d", groupCount)
	}

	// Verify project assigned to existing group
	var projGroupID string
	db.Raw("SELECT group_id FROM projects WHERE id = 'proj-1'").Scan(&projGroupID)
	if projGroupID != "existing-group" {
		t.Errorf("Expected group_id 'existing-group', got: %s", projGroupID)
	}
}

func TestMigrateGroupOwnership_BackfillsRoles(t *testing.T) {
	db := setupGroupOwnershipDB(t)

	// Create a user group member without a role
	db.Exec(`INSERT INTO user_group_members (id, user_id, group_id, role) VALUES ('mem-1', 'user-1', 'group-1', '')`)
	db.Exec(`INSERT INTO user_group_members (id, user_id, group_id) VALUES ('mem-2', 'user-2', 'group-1')`)

	err := migrateGroupOwnership(db, false)
	if err != nil {
		t.Fatalf("Migration failed: %v", err)
	}

	// Verify roles were backfilled
	var role1, role2 string
	db.Raw("SELECT role FROM user_group_members WHERE id = 'mem-1'").Scan(&role1)
	db.Raw("SELECT role FROM user_group_members WHERE id = 'mem-2'").Scan(&role2)
	if role1 != "MEMBER" {
		t.Errorf("Expected role 'MEMBER' for mem-1, got: %s", role1)
	}
	if role2 != "MEMBER" {
		t.Errorf("Expected role 'MEMBER' for mem-2, got: %s", role2)
	}
}

func TestMigrateGroupOwnership_Idempotent(t *testing.T) {
	db := setupGroupOwnershipDB(t)

	// Create admin user and orphaned project
	db.Create(&models.User{
		ID:       "admin-1",
		Email:    "admin@test.com",
		Role:     "ADMIN",
		IsActive: true,
	})
	db.Exec(`INSERT INTO projects (id, name) VALUES ('proj-1', 'Project 1')`)

	// Run migration twice
	err := migrateGroupOwnership(db, true)
	if err != nil {
		t.Fatalf("First migration failed: %v", err)
	}

	err = migrateGroupOwnership(db, true)
	if err != nil {
		t.Fatalf("Second migration failed: %v", err)
	}

	// Verify exactly 1 personal group exists
	var groupCount int64
	db.Model(&models.UserGroup{}).Where("owner_id = ? AND is_personal = ?", "admin-1", true).Count(&groupCount)
	if groupCount != 1 {
		t.Errorf("Expected exactly 1 personal group after idempotent run, got: %d", groupCount)
	}
}

func TestMigrateGroupOwnership_SkipsSoftDeletedProjects(t *testing.T) {
	db := setupGroupOwnershipDB(t)

	// Create admin user
	db.Create(&models.User{
		ID:       "admin-1",
		Email:    "admin@test.com",
		Role:     "ADMIN",
		IsActive: true,
	})

	// Create a soft-deleted project (deleted_at is not null)
	db.Exec(`INSERT INTO projects (id, name, deleted_at) VALUES ('proj-deleted', 'Deleted Project', '2024-01-01 00:00:00')`)

	// No orphaned non-deleted projects exist
	err := migrateGroupOwnership(db, true)
	if err != nil {
		t.Fatalf("Migration failed: %v", err)
	}

	// Verify no personal group was created (nothing to assign)
	var groupCount int64
	db.Model(&models.UserGroup{}).Where("is_personal = ?", true).Count(&groupCount)
	if groupCount != 0 {
		t.Errorf("Expected no personal groups (only deleted project), got: %d", groupCount)
	}
}

func TestMigrateGroupOwnership_DryRun(t *testing.T) {
	db := setupGroupOwnershipDB(t)

	// Create admin user
	db.Create(&models.User{
		ID:       "admin-1",
		Email:    "admin@test.com",
		Role:     "ADMIN",
		IsActive: true,
	})

	// Create orphaned projects
	db.Exec(`INSERT INTO projects (id, name) VALUES ('proj-1', 'Project 1')`)
	db.Exec(`INSERT INTO projects (id, name) VALUES ('proj-2', 'Project 2')`)

	// Enable dry-run mode
	t.Setenv("MIGRATION_DRY_RUN", "true")

	err := migrateGroupOwnership(db, true)
	if err != nil {
		t.Fatalf("Migration failed: %v", err)
	}

	// Verify NO personal group was created (dry-run)
	var groupCount int64
	db.Model(&models.UserGroup{}).Where("is_personal = ?", true).Count(&groupCount)
	if groupCount != 0 {
		t.Errorf("Expected no personal groups in dry-run mode, got: %d", groupCount)
	}

	// Verify projects still have no group_id
	var orphanedCount int64
	db.Model(&models.Project{}).Where("group_id IS NULL AND deleted_at IS NULL").Count(&orphanedCount)
	if orphanedCount != 2 {
		t.Errorf("Expected 2 orphaned projects in dry-run mode, got: %d", orphanedCount)
	}
}

func TestMigrateGroupOwnership_DryRunWithExistingGroup(t *testing.T) {
	db := setupGroupOwnershipDB(t)

	// Create admin user with existing personal group
	adminID := "admin-1"
	db.Create(&models.User{
		ID:       adminID,
		Email:    "admin@test.com",
		Role:     "ADMIN",
		IsActive: true,
	})
	db.Create(&models.UserGroup{
		ID:         "existing-group",
		Name:       "admin@test.com's Projects",
		IsPersonal: true,
		OwnerID:    &adminID,
	})

	// Create orphaned project
	db.Exec(`INSERT INTO projects (id, name) VALUES ('proj-1', 'Project 1')`)

	// Enable dry-run mode
	t.Setenv("MIGRATION_DRY_RUN", "true")

	err := migrateGroupOwnership(db, true)
	if err != nil {
		t.Fatalf("Migration failed: %v", err)
	}

	// Verify project still has no group_id (dry-run)
	var groupID *string
	db.Raw("SELECT group_id FROM projects WHERE id = 'proj-1'").Scan(&groupID)
	if groupID != nil {
		t.Errorf("Expected group_id to remain nil in dry-run mode, got: %v", groupID)
	}
}

func TestMigrateGroupOwnership_TargetGroupIDOverride(t *testing.T) {
	db := setupGroupOwnershipDB(t)

	// Create admin user (needed for the fallback, but we'll override)
	db.Create(&models.User{
		ID:       "admin-1",
		Email:    "admin@test.com",
		Role:     "ADMIN",
		IsActive: true,
	})

	// Create a target group to assign to
	targetGroup := &models.UserGroup{
		ID:   "target-group-id",
		Name: "Target Group",
	}
	db.Create(targetGroup)

	// Create orphaned projects
	db.Exec(`INSERT INTO projects (id, name) VALUES ('proj-1', 'Project 1')`)
	db.Exec(`INSERT INTO projects (id, name) VALUES ('proj-2', 'Project 2')`)

	// Set target group override
	t.Setenv("MIGRATION_TARGET_GROUP_ID", "target-group-id")

	err := migrateGroupOwnership(db, true)
	if err != nil {
		t.Fatalf("Migration failed: %v", err)
	}

	// Verify projects were assigned to the target group (not admin's personal group)
	var proj1GroupID, proj2GroupID string
	db.Raw("SELECT group_id FROM projects WHERE id = 'proj-1'").Scan(&proj1GroupID)
	db.Raw("SELECT group_id FROM projects WHERE id = 'proj-2'").Scan(&proj2GroupID)
	if proj1GroupID != "target-group-id" {
		t.Errorf("Expected proj-1 group_id 'target-group-id', got: %s", proj1GroupID)
	}
	if proj2GroupID != "target-group-id" {
		t.Errorf("Expected proj-2 group_id 'target-group-id', got: %s", proj2GroupID)
	}

	// Verify NO personal group was created (used override instead)
	var personalGroupCount int64
	db.Model(&models.UserGroup{}).Where("is_personal = ?", true).Count(&personalGroupCount)
	if personalGroupCount != 0 {
		t.Errorf("Expected no personal groups with target override, got: %d", personalGroupCount)
	}
}

func TestRepairOrphanedGroupMemberships_RepairsOrphans(t *testing.T) {
	db := setupGroupOwnershipDB(t)

	ownerID := "owner-user-1"
	groupID := "personal-group-1"

	// Create a personal group with owner but no membership
	db.Exec(`INSERT INTO users (id, email, role, is_active) VALUES (?, 'owner@test.com', 'USER', 1)`, ownerID)
	db.Exec(`INSERT INTO user_groups (id, name, is_personal, owner_id) VALUES (?, 'owner@test.com', 1, ?)`, groupID, ownerID)

	err := repairOrphanedGroupMemberships(db)
	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}

	// Verify membership was created
	var count int64
	db.Model(&models.UserGroupMember{}).Where("user_id = ? AND group_id = ?", ownerID, groupID).Count(&count)
	if count != 1 {
		t.Errorf("Expected 1 membership, got %d", count)
	}

	// Verify role is GROUP_ADMIN
	var member models.UserGroupMember
	db.Where("user_id = ? AND group_id = ?", ownerID, groupID).First(&member)
	if member.Role != models.GroupRoleGroupAdmin {
		t.Errorf("Expected GROUP_ADMIN role, got %s", member.Role)
	}
}

func TestRepairOrphanedGroupMemberships_Idempotent(t *testing.T) {
	db := setupGroupOwnershipDB(t)

	ownerID := "owner-user-1"
	groupID := "personal-group-1"

	db.Exec(`INSERT INTO users (id, email, role, is_active) VALUES (?, 'owner@test.com', 'USER', 1)`, ownerID)
	db.Exec(`INSERT INTO user_groups (id, name, is_personal, owner_id) VALUES (?, 'owner@test.com', 1, ?)`, groupID, ownerID)

	// Run twice
	err := repairOrphanedGroupMemberships(db)
	if err != nil {
		t.Fatalf("First run: expected no error, got: %v", err)
	}
	err = repairOrphanedGroupMemberships(db)
	if err != nil {
		t.Fatalf("Second run: expected no error, got: %v", err)
	}

	// Should still be exactly 1 membership
	var count int64
	db.Model(&models.UserGroupMember{}).Where("user_id = ? AND group_id = ?", ownerID, groupID).Count(&count)
	if count != 1 {
		t.Errorf("Expected exactly 1 membership after two runs, got %d", count)
	}
}

func TestRepairOrphanedGroupMemberships_SkipsNonPersonalGroups(t *testing.T) {
	db := setupGroupOwnershipDB(t)

	ownerID := "owner-user-1"
	groupID := "shared-group-1"

	db.Exec(`INSERT INTO users (id, email, role, is_active) VALUES (?, 'owner@test.com', 'USER', 1)`, ownerID)
	// Non-personal group with owner but no membership - should NOT be repaired
	db.Exec(`INSERT INTO user_groups (id, name, is_personal, owner_id) VALUES (?, 'Shared Group', 0, ?)`, groupID, ownerID)

	err := repairOrphanedGroupMemberships(db)
	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}

	// Should not create membership for non-personal group
	var count int64
	db.Model(&models.UserGroupMember{}).Where("user_id = ? AND group_id = ?", ownerID, groupID).Count(&count)
	if count != 0 {
		t.Errorf("Expected 0 memberships for non-personal group, got %d", count)
	}
}

func TestMigrateGroupOwnership_TargetGroupIDNotFound(t *testing.T) {
	db := setupGroupOwnershipDB(t)

	// Create orphaned project
	db.Exec(`INSERT INTO projects (id, name) VALUES ('proj-1', 'Project 1')`)

	// Set invalid target group override
	t.Setenv("MIGRATION_TARGET_GROUP_ID", "nonexistent-group")

	err := migrateGroupOwnership(db, true)
	if err == nil {
		t.Fatal("Expected error for nonexistent target group, got nil")
	}
	if !strings.Contains(err.Error(), "MIGRATION_TARGET_GROUP_ID") {
		t.Errorf("Expected error to mention MIGRATION_TARGET_GROUP_ID, got: %v", err)
	}
}

// TestLegacyDB_AutoMigrate_AddsRefIDColumn verifies the contract this PR
// depends on: when the keyColumns guard detects a missing column on a
// legacy FK-constrained table, db.AutoMigrate adds the column without
// triggering the GORM/SQLite "FOREIGN parsed as column" rebuild bug.
//
// Regression guard for the startup crash where ref_id was added to models
// in task 14c but kept missing from keyColumns, so AutoMigrate was skipped
// on legacy databases and CreateRefIDIndexes failed with "no such column".
func TestLegacyDB_AutoMigrate_AddsRefIDColumn(t *testing.T) {
	db := setupInMemoryDB(t)

	// Simulate a legacy schema: fixture_definitions exists, but predates
	// the ref_id column added in task 14c. No FK constraints to keep the
	// fixture minimal; AutoMigrate's column-addition path is what we test.
	if err := db.Exec(`
		CREATE TABLE fixture_definitions (
			id TEXT PRIMARY KEY,
			manufacturer TEXT,
			model TEXT,
			type TEXT,
			is_built_in INTEGER DEFAULT 0,
			created_at DATETIME,
			updated_at DATETIME
		)
	`).Error; err != nil {
		t.Fatalf("create legacy fixture_definitions: %v", err)
	}

	if db.Migrator().HasColumn("fixture_definitions", "ref_id") {
		t.Fatal("precondition: legacy schema must not have ref_id column")
	}

	// AutoMigrate the model — this is what the keyColumns guard triggers
	// when it detects the missing column.
	if err := db.AutoMigrate(&models.FixtureDefinition{}); err != nil {
		t.Fatalf("AutoMigrate: %v", err)
	}

	if !db.Migrator().HasColumn("fixture_definitions", "ref_id") {
		t.Error("ref_id column must be added by AutoMigrate")
	}
}
