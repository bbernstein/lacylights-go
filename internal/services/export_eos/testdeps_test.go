package exporteos

import (
	"testing"

	"github.com/bbernstein/lacylights-go/internal/database/migrations"
	"github.com/bbernstein/lacylights-go/internal/database/models"
	"github.com/bbernstein/lacylights-go/internal/database/repositories"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

type testDeps struct {
	db               *gorm.DB
	projectRepo      *repositories.ProjectRepository
	fixtureRepo      *repositories.FixtureRepository
	fixtureGroupRepo *repositories.FixtureGroupRepository
	lookRepo         *repositories.LookRepository
	cueListRepo      *repositories.CueListRepository
	cueRepo          *repositories.CueRepository
	lookBoardRepo    *repositories.LookBoardRepository
}

func newTestDeps(t *testing.T) *testDeps {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := db.AutoMigrate(
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
		&models.Setting{},
		&models.LookBoard{},
		&models.LookBoardButton{},
		&models.FixtureGroup{},
		&models.FixtureGroupMember{},
		&models.UserGroup{},
		&models.UserGroupMember{},
	); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	// Apply RefID unique indexes so tests catch constraint violations the
	// same way production does.
	if err := migrations.CreateRefIDIndexes(db); err != nil {
		t.Fatalf("create ref_id indexes: %v", err)
	}
	return &testDeps{
		db:               db,
		projectRepo:      repositories.NewProjectRepository(db),
		fixtureRepo:      repositories.NewFixtureRepository(db),
		fixtureGroupRepo: repositories.NewFixtureGroupRepository(db),
		lookRepo:         repositories.NewLookRepository(db),
		cueListRepo:      repositories.NewCueListRepository(db),
		cueRepo:          repositories.NewCueRepository(db),
		lookBoardRepo:    repositories.NewLookBoardRepository(db),
	}
}

func (d *testDeps) close() {
	if sqlDB, err := d.db.DB(); err == nil {
		_ = sqlDB.Close()
	}
}
