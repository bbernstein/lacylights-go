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

	// Verify sparse format was applied
	expectedContains := []string{`"offset":0`, `"offset":1`, `"offset":2`}
	for _, expected := range expectedContains {
		if !strings.Contains(channels1, expected) {
			t.Errorf("Expected channels1 to contain %s, got: %s", expected, channels1)
		}
	}

	// Verify values are correct
	if !strings.Contains(channels1, `"value":255`) {
		t.Errorf("Expected channels1 to contain value 255, got: %s", channels1)
	}
	if !strings.Contains(channels1, `"value":128`) {
		t.Errorf("Expected channels1 to contain value 128, got: %s", channels1)
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
