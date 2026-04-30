package repositories

import (
	"context"
	"errors"
	"testing"

	"github.com/bbernstein/lacylights-go/internal/database/migrations"
	"github.com/bbernstein/lacylights-go/internal/database/models"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func newFixtureGroupRepoForTest(t *testing.T) (*FixtureGroupRepository, *FixtureRepository, *gorm.DB) {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	if err := db.AutoMigrate(
		&models.Project{},
		&models.FixtureDefinition{},
		&models.ChannelDefinition{},
		&models.FixtureInstance{},
		&models.InstanceChannel{},
		&models.Look{},
		&models.LookBoard{},
		&models.FixtureGroup{},
		&models.FixtureGroupMember{},
	); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	if err := migrations.CreateRefIDIndexes(db); err != nil {
		t.Fatalf("indexes: %v", err)
	}
	return NewFixtureGroupRepository(db), NewFixtureRepository(db), db
}

func seedFixtureForGroup(t *testing.T, fr *FixtureRepository, projectID, refID string) *models.FixtureInstance {
	t.Helper()
	def := &models.FixtureDefinition{Manufacturer: "Generic", Model: "Dimmer", Type: "DIMMER"}
	if err := fr.CreateDefinition(context.Background(), def); err != nil {
		t.Fatalf("create def: %v", err)
	}
	fi := &models.FixtureInstance{ProjectID: projectID, DefinitionID: def.ID, Name: refID, RefID: refID}
	if err := fr.CreateWithChannels(context.Background(), fi, nil); err != nil {
		t.Fatalf("create fi: %v", err)
	}
	return fi
}

func TestFixtureGroupRepository_Create_DefaultsRefIDAndAssignsID(t *testing.T) {
	repo, _, _ := newFixtureGroupRepoForTest(t)
	g := &models.FixtureGroup{ProjectID: "p1", Name: "Movers"}
	if err := repo.Create(context.Background(), g); err != nil {
		t.Fatalf("create: %v", err)
	}
	if g.ID == "" {
		t.Fatalf("expected ID assigned")
	}
	if g.RefID != g.ID {
		t.Errorf("RefID = %q, want %q", g.RefID, g.ID)
	}
}

func TestFixtureGroupRepository_CreateWithMembers_AssignsOrder(t *testing.T) {
	repo, fr, _ := newFixtureGroupRepoForTest(t)
	ctx := context.Background()

	a := seedFixtureForGroup(t, fr, "p1", "ch-1")
	b := seedFixtureForGroup(t, fr, "p1", "ch-2")
	c := seedFixtureForGroup(t, fr, "p1", "ch-3")

	g := &models.FixtureGroup{ProjectID: "p1", Name: "All"}
	if err := repo.CreateWithMembers(ctx, g, []string{a.ID, c.ID, b.ID}); err != nil {
		t.Fatalf("create: %v", err)
	}

	ms, err := repo.GetMembers(ctx, g.ID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if len(ms) != 3 {
		t.Fatalf("got %d, want 3", len(ms))
	}
	if ms[0].FixtureID != a.ID || ms[0].OrderIndex != 0 {
		t.Errorf("ms[0] = %v", ms[0])
	}
	if ms[1].FixtureID != c.ID || ms[1].OrderIndex != 1 {
		t.Errorf("ms[1] = %v", ms[1])
	}
	if ms[2].FixtureID != b.ID || ms[2].OrderIndex != 2 {
		t.Errorf("ms[2] = %v", ms[2])
	}
}

func TestFixtureGroupRepository_SetMembers_Replaces(t *testing.T) {
	repo, fr, _ := newFixtureGroupRepoForTest(t)
	ctx := context.Background()
	a := seedFixtureForGroup(t, fr, "p1", "ch-1")
	b := seedFixtureForGroup(t, fr, "p1", "ch-2")
	c := seedFixtureForGroup(t, fr, "p1", "ch-3")

	g := &models.FixtureGroup{ProjectID: "p1", Name: "G"}
	if err := repo.CreateWithMembers(ctx, g, []string{a.ID, b.ID}); err != nil {
		t.Fatalf("create: %v", err)
	}
	if err := repo.SetMembers(ctx, g.ID, []string{c.ID}); err != nil {
		t.Fatalf("set: %v", err)
	}
	ms, _ := repo.GetMembers(ctx, g.ID)
	if len(ms) != 1 || ms[0].FixtureID != c.ID {
		t.Errorf("after set: %v", ms)
	}
}

func TestFixtureGroupRepository_AddMembers_SkipsDuplicates(t *testing.T) {
	repo, fr, _ := newFixtureGroupRepoForTest(t)
	ctx := context.Background()
	a := seedFixtureForGroup(t, fr, "p1", "ch-1")
	b := seedFixtureForGroup(t, fr, "p1", "ch-2")

	g := &models.FixtureGroup{ProjectID: "p1", Name: "G"}
	_ = repo.CreateWithMembers(ctx, g, []string{a.ID})
	if err := repo.AddMembers(ctx, g.ID, []string{a.ID, b.ID}); err != nil {
		t.Fatalf("add: %v", err)
	}
	ms, _ := repo.GetMembers(ctx, g.ID)
	if len(ms) != 2 {
		t.Errorf("want 2 members, got %d", len(ms))
	}
}

func TestFixtureGroupRepository_RemoveMembers(t *testing.T) {
	repo, fr, _ := newFixtureGroupRepoForTest(t)
	ctx := context.Background()
	a := seedFixtureForGroup(t, fr, "p1", "ch-1")
	b := seedFixtureForGroup(t, fr, "p1", "ch-2")
	g := &models.FixtureGroup{ProjectID: "p1", Name: "G"}
	_ = repo.CreateWithMembers(ctx, g, []string{a.ID, b.ID})

	if err := repo.RemoveMembers(ctx, g.ID, []string{a.ID, "missing"}); err != nil {
		t.Fatalf("remove: %v", err)
	}
	ms, _ := repo.GetMembers(ctx, g.ID)
	if len(ms) != 1 || ms[0].FixtureID != b.ID {
		t.Errorf("after remove: %v", ms)
	}
}

func TestFixtureGroupRepository_ReorderMembers(t *testing.T) {
	repo, fr, _ := newFixtureGroupRepoForTest(t)
	ctx := context.Background()
	a := seedFixtureForGroup(t, fr, "p1", "ch-1")
	b := seedFixtureForGroup(t, fr, "p1", "ch-2")
	c := seedFixtureForGroup(t, fr, "p1", "ch-3")
	g := &models.FixtureGroup{ProjectID: "p1", Name: "G"}
	_ = repo.CreateWithMembers(ctx, g, []string{a.ID, b.ID, c.ID})

	if err := repo.ReorderMembers(ctx, g.ID, []string{c.ID, a.ID, b.ID}); err != nil {
		t.Fatalf("reorder: %v", err)
	}
	ms, _ := repo.GetMembers(ctx, g.ID)
	want := []string{c.ID, a.ID, b.ID}
	for i, id := range want {
		if ms[i].FixtureID != id {
			t.Errorf("ms[%d] = %s, want %s", i, ms[i].FixtureID, id)
		}
	}
}

func TestFixtureGroupRepository_AssignNextEosNumber(t *testing.T) {
	repo, _, _ := newFixtureGroupRepoForTest(t)
	ctx := context.Background()

	n, err := repo.AssignNextEosNumber(ctx, "p1")
	if err != nil {
		t.Fatalf("first: %v", err)
	}
	if n != 1 {
		t.Errorf("first = %d, want 1", n)
	}

	one := 1
	if err := repo.Create(ctx, &models.FixtureGroup{ProjectID: "p1", Name: "A", EosNumber: &one}); err != nil {
		t.Fatalf("seed: %v", err)
	}
	five := 5
	if err := repo.Create(ctx, &models.FixtureGroup{ProjectID: "p1", Name: "B", EosNumber: &five}); err != nil {
		t.Fatalf("seed: %v", err)
	}

	n, _ = repo.AssignNextEosNumber(ctx, "p1")
	if n != 6 {
		t.Errorf("after seeding 1+5 = %d, want 6", n)
	}
}

func TestFixtureGroupRepository_FindByEosNumber(t *testing.T) {
	repo, _, _ := newFixtureGroupRepoForTest(t)
	ctx := context.Background()
	three := 3
	g := &models.FixtureGroup{ProjectID: "p1", Name: "G", EosNumber: &three}
	if err := repo.Create(ctx, g); err != nil {
		t.Fatalf("create: %v", err)
	}
	got, err := repo.FindByEosNumber(ctx, "p1", 3)
	if err != nil || got == nil || got.ID != g.ID {
		t.Errorf("got %v, %v", got, err)
	}
	miss, _ := repo.FindByEosNumber(ctx, "p1", 99)
	if miss != nil {
		t.Errorf("miss should be nil")
	}
}

func TestFixtureGroupRepository_CascadeDeleteOnFixture(t *testing.T) {
	repo, fr, _ := newFixtureGroupRepoForTest(t)
	ctx := context.Background()
	a := seedFixtureForGroup(t, fr, "p1", "ch-1")
	g := &models.FixtureGroup{ProjectID: "p1", Name: "G"}
	_ = repo.CreateWithMembers(ctx, g, []string{a.ID})

	if err := fr.Delete(ctx, a.ID); err != nil {
		t.Fatalf("delete fixture: %v", err)
	}

	// FixtureRepository.Delete now wraps deletion in a transaction that
	// also removes group memberships — verify membership is gone.
	ms, _ := repo.GetMembers(ctx, g.ID)
	if len(ms) != 0 {
		t.Errorf("expected cascade delete; got %d members", len(ms))
	}
}

// (Suppress unused import warning if errors isn't otherwise used)
var _ = errors.New
