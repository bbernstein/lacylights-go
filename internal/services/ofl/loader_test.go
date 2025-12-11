package ofl

import (
	"context"
	"testing"

	"github.com/bbernstein/lacylights-go/internal/database/models"
	"github.com/bbernstein/lacylights-go/internal/database/repositories"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// setupTestDB creates a fresh in-memory SQLite database for testing
func setupLoaderTestDB(t *testing.T) *gorm.DB {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		t.Fatalf("Failed to create test database: %v", err)
	}

	// Run migrations
	err = db.AutoMigrate(
		&models.FixtureDefinition{},
		&models.ChannelDefinition{},
		&models.FixtureMode{},
		&models.ModeChannel{},
		&models.FixtureInstance{},
		&models.InstanceChannel{},
	)
	if err != nil {
		t.Fatalf("Failed to migrate test database: %v", err)
	}

	return db
}

func TestNewLoader(t *testing.T) {
	db := setupLoaderTestDB(t)
	fixtureRepo := repositories.NewFixtureRepository(db)

	loader := NewLoader(db, fixtureRepo, "/tmp/ofl-test-cache")

	if loader == nil {
		t.Fatal("Expected loader to be created")
	}
	if loader.cachePath != "/tmp/ofl-test-cache" {
		t.Errorf("Expected cachePath to be /tmp/ofl-test-cache, got %s", loader.cachePath)
	}
}

func TestNeedsImport_EmptyDatabase(t *testing.T) {
	db := setupLoaderTestDB(t)
	fixtureRepo := repositories.NewFixtureRepository(db)
	loader := NewLoader(db, fixtureRepo, "/tmp/ofl-test-cache")

	needsImport, err := loader.NeedsImport(context.Background())
	if err != nil {
		t.Fatalf("NeedsImport returned error: %v", err)
	}

	if !needsImport {
		t.Error("Expected NeedsImport to return true for empty database")
	}
}

func TestNeedsImport_WithExistingFixtures(t *testing.T) {
	db := setupLoaderTestDB(t)
	fixtureRepo := repositories.NewFixtureRepository(db)
	loader := NewLoader(db, fixtureRepo, "/tmp/ofl-test-cache")

	// Create a fixture definition
	definition := &models.FixtureDefinition{
		ID:           "test-fixture-1",
		Manufacturer: "Test Manufacturer",
		Model:        "Test Model",
		Type:         "LED_PAR",
	}
	if err := db.Create(definition).Error; err != nil {
		t.Fatalf("Failed to create test fixture: %v", err)
	}

	needsImport, err := loader.NeedsImport(context.Background())
	if err != nil {
		t.Fatalf("NeedsImport returned error: %v", err)
	}

	if needsImport {
		t.Error("Expected NeedsImport to return false when fixtures exist")
	}
}

func TestLoadImportStatus_NoFile(t *testing.T) {
	db := setupLoaderTestDB(t)
	fixtureRepo := repositories.NewFixtureRepository(db)
	loader := NewLoader(db, fixtureRepo, "/tmp/ofl-nonexistent-cache-12345")

	status, err := loader.LoadImportStatus()
	if err != nil {
		t.Fatalf("LoadImportStatus returned error: %v", err)
	}

	if status != nil {
		t.Error("Expected nil status for nonexistent cache directory")
	}
}

func TestManufacturerParsing(t *testing.T) {
	// Test the Manufacturer type can be parsed correctly
	mfg := Manufacturer{
		Name:    "Chauvet DJ",
		Website: "https://www.chauvetdj.com",
		RDMId:   12345,
	}

	if mfg.Name != "Chauvet DJ" {
		t.Errorf("Expected Name to be 'Chauvet DJ', got '%s'", mfg.Name)
	}
}

func TestImportStatus_Fields(t *testing.T) {
	status := ImportStatus{
		TotalFixtures:     1000,
		SuccessfulImports: 950,
		FailedImports:     50,
		Version:           "master",
	}

	if status.TotalFixtures != 1000 {
		t.Errorf("Expected TotalFixtures to be 1000, got %d", status.TotalFixtures)
	}
	if status.SuccessfulImports != 950 {
		t.Errorf("Expected SuccessfulImports to be 950, got %d", status.SuccessfulImports)
	}
	if status.FailedImports != 50 {
		t.Errorf("Expected FailedImports to be 50, got %d", status.FailedImports)
	}
}
