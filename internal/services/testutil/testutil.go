// Package testutil provides shared test utilities for integration tests.
package testutil

import (
	"testing"

	"github.com/glebarez/sqlite"
	"github.com/lucsky/cuid"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"github.com/bbernstein/lacylights-go/internal/database/models"
	"github.com/bbernstein/lacylights-go/internal/database/repositories"
)

// TestDB holds the test database and repositories.
type TestDB struct {
	DB          *gorm.DB
	ProjectRepo *repositories.ProjectRepository
	FixtureRepo *repositories.FixtureRepository
	SceneRepo   *repositories.SceneRepository
	CueListRepo *repositories.CueListRepository
	CueRepo     *repositories.CueRepository
}

// SetupTestDB creates an in-memory SQLite database for testing.
// It returns a TestDB with all repositories initialized and a cleanup function.
func SetupTestDB(t *testing.T) (*TestDB, func()) {
	t.Helper()

	// Create in-memory SQLite database
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		t.Fatalf("Failed to open in-memory database: %v", err)
	}

	// Auto-migrate all models
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

	// Create repositories
	testDB := &TestDB{
		DB:          db,
		ProjectRepo: repositories.NewProjectRepository(db),
		FixtureRepo: repositories.NewFixtureRepository(db),
		SceneRepo:   repositories.NewSceneRepository(db),
		CueListRepo: repositories.NewCueListRepository(db),
		CueRepo:     repositories.NewCueRepository(db),
	}

	// Cleanup function - close the database connection
	cleanup := func() {
		sqlDB, err := db.DB()
		if err == nil {
			_ = sqlDB.Close()
		}
	}

	return testDB, cleanup
}

// UniqueProjectName generates a unique project name for testing.
// This ensures tests don't conflict with each other.
func UniqueProjectName(prefix string) string {
	return prefix + "-" + cuid.New()[:8]
}

// UniqueFixtureName generates a unique fixture name for testing.
func UniqueFixtureName(prefix string) string {
	return prefix + "-" + cuid.New()[:8]
}
