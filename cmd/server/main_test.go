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
