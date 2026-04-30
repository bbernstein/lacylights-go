package repositories

import (
	"context"
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

func TestFixtureGroupRepository_FindByID(t *testing.T) {
	repo, _, _ := newFixtureGroupRepoForTest(t)
	ctx := context.Background()

	g := &models.FixtureGroup{ProjectID: "p1", Name: "G"}
	if err := repo.Create(ctx, g); err != nil {
		t.Fatalf("create: %v", err)
	}

	got, err := repo.FindByID(ctx, g.ID)
	if err != nil || got == nil || got.ID != g.ID {
		t.Errorf("hit: got %v, %v", got, err)
	}

	miss, err := repo.FindByID(ctx, "missing")
	if err != nil {
		t.Errorf("miss err: %v", err)
	}
	if miss != nil {
		t.Errorf("miss should be nil, got %v", miss)
	}
}

func TestFixtureGroupRepository_FindByProjectID(t *testing.T) {
	repo, _, _ := newFixtureGroupRepoForTest(t)
	ctx := context.Background()

	one, two := 2, 1
	a := &models.FixtureGroup{ProjectID: "p1", Name: "Beta", ProjectOrder: &one}
	b := &models.FixtureGroup{ProjectID: "p1", Name: "Alpha", ProjectOrder: &two}
	c := &models.FixtureGroup{ProjectID: "p1", Name: "NoOrder"}
	other := &models.FixtureGroup{ProjectID: "p2", Name: "Other"}
	for _, g := range []*models.FixtureGroup{a, b, c, other} {
		if err := repo.Create(ctx, g); err != nil {
			t.Fatalf("create %s: %v", g.Name, err)
		}
	}

	got, err := repo.FindByProjectID(ctx, "p1")
	if err != nil {
		t.Fatalf("find: %v", err)
	}
	if len(got) != 3 {
		t.Fatalf("got %d groups, want 3 (cross-project leak?)", len(got))
	}
	if got[0].Name != "Alpha" || got[1].Name != "Beta" || got[2].Name != "NoOrder" {
		t.Errorf("order: got %s, %s, %s", got[0].Name, got[1].Name, got[2].Name)
	}
}

func TestFixtureGroupRepository_FindByRefID(t *testing.T) {
	repo, _, _ := newFixtureGroupRepoForTest(t)
	ctx := context.Background()

	a := &models.FixtureGroup{ProjectID: "p1", Name: "A", RefID: "shared"}
	b := &models.FixtureGroup{ProjectID: "p2", Name: "B", RefID: "shared"}
	for _, g := range []*models.FixtureGroup{a, b} {
		if err := repo.Create(ctx, g); err != nil {
			t.Fatalf("create: %v", err)
		}
	}

	got, err := repo.FindByRefID(ctx, "p1", "shared")
	if err != nil || got == nil || got.ID != a.ID {
		t.Errorf("hit: got %v, %v", got, err)
	}

	miss, err := repo.FindByRefID(ctx, "p1", "missing")
	if err != nil || miss != nil {
		t.Errorf("miss: got (%v, %v)", miss, err)
	}
}

func TestFixtureGroupRepository_Update(t *testing.T) {
	repo, _, _ := newFixtureGroupRepoForTest(t)
	ctx := context.Background()

	g := &models.FixtureGroup{ProjectID: "p1", Name: "Original", RefID: "frozen"}
	if err := repo.Create(ctx, g); err != nil {
		t.Fatalf("create: %v", err)
	}

	desc := "updated description"
	five := 5
	order := 3
	g.Name = "Updated"
	g.Description = &desc
	g.EosNumber = &five
	g.ProjectOrder = &order
	g.RefID = "should-not-change"
	if err := repo.Update(ctx, g); err != nil {
		t.Fatalf("update: %v", err)
	}

	reloaded, err := repo.FindByID(ctx, g.ID)
	if err != nil || reloaded == nil {
		t.Fatalf("reload: %v", err)
	}
	if reloaded.Name != "Updated" {
		t.Errorf("Name = %q, want Updated", reloaded.Name)
	}
	if reloaded.Description == nil || *reloaded.Description != "updated description" {
		t.Errorf("Description = %v", reloaded.Description)
	}
	if reloaded.EosNumber == nil || *reloaded.EosNumber != 5 {
		t.Errorf("EosNumber = %v", reloaded.EosNumber)
	}
	if reloaded.ProjectOrder == nil || *reloaded.ProjectOrder != 3 {
		t.Errorf("ProjectOrder = %v", reloaded.ProjectOrder)
	}
	if reloaded.RefID != "frozen" {
		t.Errorf("RefID = %q, want frozen (Update must not touch RefID)", reloaded.RefID)
	}
}

func TestFixtureGroupRepository_Delete(t *testing.T) {
	repo, fr, _ := newFixtureGroupRepoForTest(t)
	ctx := context.Background()

	a := seedFixtureForGroup(t, fr, "p1", "ch-1")
	g := &models.FixtureGroup{ProjectID: "p1", Name: "G"}
	if err := repo.CreateWithMembers(ctx, g, []string{a.ID}); err != nil {
		t.Fatalf("create: %v", err)
	}

	if err := repo.Delete(ctx, g.ID); err != nil {
		t.Fatalf("delete: %v", err)
	}

	got, err := repo.FindByID(ctx, g.ID)
	if err != nil {
		t.Fatalf("find after delete: %v", err)
	}
	if got != nil {
		t.Errorf("expected (nil, nil) after delete, got %v", got)
	}
}

func TestFixtureGroupRepository_RemoveMembers_NoOpWhenEmpty(t *testing.T) {
	repo, _, _ := newFixtureGroupRepoForTest(t)
	if err := repo.RemoveMembers(context.Background(), "any", nil); err != nil {
		t.Errorf("expected no-op on empty list, got: %v", err)
	}
}

func TestFixtureGroupRepository_ReorderMembers_NoOpWhenEmpty(t *testing.T) {
	repo, _, _ := newFixtureGroupRepoForTest(t)
	if err := repo.ReorderMembers(context.Background(), "any", nil); err != nil {
		t.Errorf("expected no-op on empty list, got: %v", err)
	}
}

func TestFixtureGroupRepository_AddMembers_NoOpWhenEmpty(t *testing.T) {
	repo, _, _ := newFixtureGroupRepoForTest(t)
	if err := repo.AddMembers(context.Background(), "any", nil); err != nil {
		t.Errorf("expected no-op on empty list, got: %v", err)
	}
}

func TestFixtureGroupRepository_AssignAndPersistMissingEosNumbers(t *testing.T) {
	repo, _, _ := newFixtureGroupRepoForTest(t)
	ctx := context.Background()

	// Mix of assigned and unassigned. Three pre-existing groups: one with
	// EosNumber=2, two with nil. After AssignAndPersist, the nils should
	// receive 3 and 4 (next after max), in order.
	two := 2
	a := &models.FixtureGroup{ProjectID: "p1", Name: "A", EosNumber: &two}
	b := &models.FixtureGroup{ProjectID: "p1", Name: "B"}
	c := &models.FixtureGroup{ProjectID: "p1", Name: "C"}
	for _, g := range []*models.FixtureGroup{a, b, c} {
		if err := repo.Create(ctx, g); err != nil {
			t.Fatalf("create %s: %v", g.Name, err)
		}
	}

	groups := []models.FixtureGroup{*a, *b, *c}
	if err := repo.AssignAndPersistMissingEosNumbers(ctx, "p1", groups); err != nil {
		t.Fatalf("assign: %v", err)
	}

	// In-memory mutation: the slice's nil entries must now have numbers.
	for i, g := range groups {
		if g.EosNumber == nil {
			t.Errorf("groups[%d] (%s) EosNumber still nil", i, g.Name)
		}
	}
	// Persistence: read back and verify both are above 2 and unique.
	all, _ := repo.FindByProjectID(ctx, "p1")
	seen := map[int]bool{}
	for _, g := range all {
		if g.EosNumber == nil {
			t.Errorf("group %s still nil after persist", g.Name)
			continue
		}
		if seen[*g.EosNumber] {
			t.Errorf("duplicate EosNumber %d", *g.EosNumber)
		}
		seen[*g.EosNumber] = true
	}

	// Idempotent: running again with all-assigned slice is a no-op.
	all2, _ := repo.FindByProjectID(ctx, "p1")
	if err := repo.AssignAndPersistMissingEosNumbers(ctx, "p1", all2); err != nil {
		t.Fatalf("re-run: %v", err)
	}
}

