package resolvers

import (
	"context"
	"testing"

	"github.com/99designs/gqlgen/graphql"

	"github.com/bbernstein/lacylights-go/internal/database/models"
	"github.com/bbernstein/lacylights-go/internal/graphql/generated"
)

func seedFixtureForGroupResolver(t *testing.T, r *Resolver, projectID, name string) *models.FixtureInstance {
	t.Helper()
	def := &models.FixtureDefinition{Manufacturer: "Generic", Model: "Dimmer-" + name, Type: "DIMMER"}
	if err := r.FixtureRepo.CreateDefinition(context.Background(), def); err != nil {
		t.Fatalf("def: %v", err)
	}
	fi := &models.FixtureInstance{ProjectID: projectID, DefinitionID: def.ID, Name: name}
	if err := r.FixtureRepo.CreateWithChannels(context.Background(), fi, nil); err != nil {
		t.Fatalf("fi: %v", err)
	}
	return fi
}

func TestResolver_CreateAndQueryFixtureGroup(t *testing.T) {
	_, r, cleanup := testSetup(t)
	defer cleanup()
	ctx := context.Background()

	proj := &models.Project{Name: "P"}
	if err := r.ProjectRepo.Create(ctx, proj); err != nil {
		t.Fatalf("project: %v", err)
	}

	a := seedFixtureForGroupResolver(t, r, proj.ID, "A")
	b := seedFixtureForGroupResolver(t, r, proj.ID, "B")

	in := generated.CreateFixtureGroupInput{
		ProjectID:  proj.ID,
		Name:       "Movers",
		FixtureIds: []string{a.ID, b.ID},
	}
	g, err := r.createFixtureGroup(ctx, in)
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if g.Name != "Movers" {
		t.Errorf("name = %q", g.Name)
	}

	q, err := r.fixtureGroupQuery(ctx, g.ID)
	if err != nil || q == nil {
		t.Fatalf("query: %v %v", q, err)
	}

	// fixtureGroupsQuery should return our newly-created group
	groups, err := r.fixtureGroupsQuery(ctx, proj.ID)
	if err != nil {
		t.Fatalf("fixtureGroupsQuery: %v", err)
	}
	if len(groups) != 1 {
		t.Fatalf("expected 1 group, got %d", len(groups))
	}

	// Update name
	newName := "Movers Renamed"
	updated, err := r.updateFixtureGroup(ctx, generated.UpdateFixtureGroupInput{
		ID:   g.ID,
		Name: graphql.OmittableOf(&newName),
	})
	if err != nil {
		t.Fatalf("update: %v", err)
	}
	if updated.Name != newName {
		t.Errorf("name after update = %q, want %q", updated.Name, newName)
	}

	// Delete
	ok, err := r.deleteFixtureGroup(ctx, g.ID)
	if err != nil || !ok {
		t.Fatalf("delete: ok=%v err=%v", ok, err)
	}

	// Verify gone
	q2, err := r.fixtureGroupQuery(ctx, g.ID)
	if err != nil {
		t.Fatalf("post-delete query: %v", err)
	}
	if q2 != nil {
		t.Errorf("expected nil after delete, got %+v", q2)
	}
}

func TestResolver_AddRemoveReorderFixturesInGroup(t *testing.T) {
	_, r, cleanup := testSetup(t)
	defer cleanup()
	ctx := context.Background()

	proj := &models.Project{Name: "P"}
	if err := r.ProjectRepo.Create(ctx, proj); err != nil {
		t.Fatalf("project: %v", err)
	}
	a := seedFixtureForGroupResolver(t, r, proj.ID, "A")
	b := seedFixtureForGroupResolver(t, r, proj.ID, "B")
	c := seedFixtureForGroupResolver(t, r, proj.ID, "C")

	g, err := r.createFixtureGroup(ctx, generated.CreateFixtureGroupInput{
		ProjectID: proj.ID, Name: "G", FixtureIds: []string{a.ID},
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if _, err := r.addFixturesToGroup(ctx, g.ID, []string{b.ID, c.ID}); err != nil {
		t.Fatalf("add: %v", err)
	}
	if _, err := r.reorderFixturesInGroup(ctx, generated.ReorderFixturesInGroupInput{
		GroupID: g.ID, FixtureIds: []string{c.ID, a.ID, b.ID},
	}); err != nil {
		t.Fatalf("reorder: %v", err)
	}
	// Verify via the child resolver: get fixtures sorted by OrderIndex.
	fixtures, err := r.fixturesForGroup(ctx, g.ID)
	if err != nil {
		t.Fatalf("fixtures: %v", err)
	}
	if len(fixtures) != 3 {
		t.Fatalf("expected 3 fixtures, got %d", len(fixtures))
	}
	if fixtures[0].ID != c.ID {
		t.Errorf("fixtures[0] = %s, want %s (c)", fixtures[0].ID, c.ID)
	}
	if fixtures[1].ID != a.ID {
		t.Errorf("fixtures[1] = %s, want %s (a)", fixtures[1].ID, a.ID)
	}
	if fixtures[2].ID != b.ID {
		t.Errorf("fixtures[2] = %s, want %s (b)", fixtures[2].ID, b.ID)
	}

	// Remove one fixture and verify
	if _, err := r.removeFixturesFromGroup(ctx, g.ID, []string{a.ID}); err != nil {
		t.Fatalf("remove: %v", err)
	}
	fixtures, err = r.fixturesForGroup(ctx, g.ID)
	if err != nil {
		t.Fatalf("fixtures (after remove): %v", err)
	}
	if len(fixtures) != 2 {
		t.Fatalf("expected 2 fixtures after remove, got %d", len(fixtures))
	}
}

func TestResolver_AssertFixturesInProject_CrossProjectRejection(t *testing.T) {
	_, r, cleanup := testSetup(t)
	defer cleanup()
	ctx := context.Background()

	p1 := &models.Project{Name: "p1"}
	p2 := &models.Project{Name: "p2"}
	if err := r.ProjectRepo.Create(ctx, p1); err != nil {
		t.Fatalf("p1: %v", err)
	}
	if err := r.ProjectRepo.Create(ctx, p2); err != nil {
		t.Fatalf("p2: %v", err)
	}
	a := seedFixtureForGroupResolver(t, r, p1.ID, "A")
	b := seedFixtureForGroupResolver(t, r, p2.ID, "B")

	_, err := r.createFixtureGroup(ctx, generated.CreateFixtureGroupInput{
		ProjectID: p1.ID, Name: "G", FixtureIds: []string{a.ID, b.ID},
	})
	if err == nil {
		t.Errorf("expected cross-project rejection")
	}
}
