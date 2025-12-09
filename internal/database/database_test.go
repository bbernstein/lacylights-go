package database

import (
	"os"
	"path/filepath"
	"testing"
)

func TestConnect_InMemory(t *testing.T) {
	// Reset global DB
	DB = nil

	cfg := Config{
		URL:         ":memory:",
		MaxIdleConn: 1,
		MaxOpenConn: 1,
		Debug:       false,
	}

	db, err := Connect(cfg)
	if err != nil {
		t.Fatalf("Connect failed: %v", err)
	}
	if db == nil {
		t.Fatal("Expected non-nil db")
	}
	if DB == nil {
		t.Error("Expected global DB to be set")
	}

	// Verify we can query
	var result int
	if err := db.Raw("SELECT 1").Scan(&result).Error; err != nil {
		t.Errorf("Failed to query database: %v", err)
	}
	if result != 1 {
		t.Errorf("Expected 1, got %d", result)
	}

	// Cleanup
	err = Close()
	if err != nil {
		t.Errorf("Close failed: %v", err)
	}
}

func TestConnect_WithFilePrefix(t *testing.T) {
	// Reset global DB
	DB = nil

	// Create temp directory
	tmpDir, err := os.MkdirTemp("", "lacylights-db-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer func() { _ = os.RemoveAll(tmpDir) }()

	dbPath := filepath.Join(tmpDir, "test.db")
	cfg := Config{
		URL:         "file:" + dbPath,
		MaxIdleConn: 1,
		MaxOpenConn: 1,
		Debug:       false,
	}

	db, err := Connect(cfg)
	if err != nil {
		t.Fatalf("Connect failed: %v", err)
	}
	if db == nil {
		t.Fatal("Expected non-nil db")
	}

	// Verify file was created
	if _, err := os.Stat(dbPath); os.IsNotExist(err) {
		t.Error("Expected database file to be created")
	}

	// Cleanup
	_ = Close()
}

func TestConnect_CreatesDirectory(t *testing.T) {
	// Reset global DB
	DB = nil

	// Create temp directory and add a nested path
	tmpDir, err := os.MkdirTemp("", "lacylights-db-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer func() { _ = os.RemoveAll(tmpDir) }()

	nestedPath := filepath.Join(tmpDir, "nested", "dir", "test.db")
	cfg := Config{
		URL:         nestedPath,
		MaxIdleConn: 1,
		MaxOpenConn: 1,
		Debug:       false,
	}

	db, err := Connect(cfg)
	if err != nil {
		t.Fatalf("Connect failed: %v", err)
	}
	if db == nil {
		t.Fatal("Expected non-nil db")
	}

	// Verify nested directory was created
	nestedDir := filepath.Dir(nestedPath)
	if _, err := os.Stat(nestedDir); os.IsNotExist(err) {
		t.Error("Expected nested directory to be created")
	}

	// Cleanup
	_ = Close()
}

func TestConnect_DebugMode(t *testing.T) {
	// Reset global DB
	DB = nil

	cfg := Config{
		URL:         ":memory:",
		MaxIdleConn: 1,
		MaxOpenConn: 1,
		Debug:       true, // Enable debug logging
	}

	db, err := Connect(cfg)
	if err != nil {
		t.Fatalf("Connect failed: %v", err)
	}
	if db == nil {
		t.Fatal("Expected non-nil db")
	}

	// Cleanup
	_ = Close()
}

func TestClose_NilDB(t *testing.T) {
	// Reset global DB
	DB = nil

	// Close should not error when DB is nil
	err := Close()
	if err != nil {
		t.Errorf("Close with nil DB should not error: %v", err)
	}
}

func TestClose_AfterConnect(t *testing.T) {
	// Reset global DB
	DB = nil

	cfg := Config{
		URL:         ":memory:",
		MaxIdleConn: 1,
		MaxOpenConn: 1,
		Debug:       false,
	}

	_, err := Connect(cfg)
	if err != nil {
		t.Fatalf("Connect failed: %v", err)
	}

	err = Close()
	if err != nil {
		t.Errorf("Close failed: %v", err)
	}
}

func TestConfig_Fields(t *testing.T) {
	cfg := Config{
		URL:         "test.db",
		MaxIdleConn: 5,
		MaxOpenConn: 10,
		Debug:       true,
	}

	if cfg.URL != "test.db" {
		t.Errorf("URL mismatch: got %s", cfg.URL)
	}
	if cfg.MaxIdleConn != 5 {
		t.Errorf("MaxIdleConn mismatch: got %d", cfg.MaxIdleConn)
	}
	if cfg.MaxOpenConn != 10 {
		t.Errorf("MaxOpenConn mismatch: got %d", cfg.MaxOpenConn)
	}
	if !cfg.Debug {
		t.Error("Debug should be true")
	}
}
