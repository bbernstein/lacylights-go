package resolvers

import (
	"context"
	"fmt"

	"github.com/bbernstein/lacylights-go/internal/database/models"
	"github.com/bbernstein/lacylights-go/internal/graphql/generated"
)

// fixtureGroupQuery returns the group by ID, gated on project access.
func (r *Resolver) fixtureGroupQuery(ctx context.Context, id string) (*models.FixtureGroup, error) {
	g, err := r.FixtureGroupRepo.FindByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if g == nil {
		return nil, nil
	}
	if err := r.ensureProjectAccess(ctx, g.ProjectID); err != nil {
		return nil, err
	}
	return g, nil
}

// fixtureGroupsQuery returns all groups in a project.
func (r *Resolver) fixtureGroupsQuery(ctx context.Context, projectID string) ([]*models.FixtureGroup, error) {
	if err := r.ensureProjectAccess(ctx, projectID); err != nil {
		return nil, err
	}
	gs, err := r.FixtureGroupRepo.FindByProjectID(ctx, projectID)
	if err != nil {
		return nil, err
	}
	out := make([]*models.FixtureGroup, len(gs))
	for i := range gs {
		out[i] = &gs[i]
	}
	return out, nil
}

func (r *Resolver) createFixtureGroup(ctx context.Context, input generated.CreateFixtureGroupInput) (*models.FixtureGroup, error) {
	if err := r.ensureProjectAccess(ctx, input.ProjectID); err != nil {
		return nil, err
	}
	// Dedupe fixtureIds while preserving caller-supplied order so the
	// junction-table composite PK doesn't reject the second insert.
	fixtureIDs := dedupePreserveOrder(input.FixtureIds)
	if err := r.assertFixturesInProject(ctx, input.ProjectID, fixtureIDs); err != nil {
		return nil, err
	}

	var description *string
	if input.Description.IsSet() {
		description = input.Description.Value()
	}
	var eosNumber *int
	if input.EosNumber.IsSet() {
		eosNumber = input.EosNumber.Value()
	}

	g := &models.FixtureGroup{
		ProjectID:   input.ProjectID,
		Name:        input.Name,
		Description: description,
		EosNumber:   eosNumber,
	}
	if err := r.FixtureGroupRepo.CreateWithMembers(ctx, g, fixtureIDs); err != nil {
		return nil, err
	}
	return g, nil
}

func (r *Resolver) updateFixtureGroup(ctx context.Context, input generated.UpdateFixtureGroupInput) (*models.FixtureGroup, error) {
	g, err := r.FixtureGroupRepo.FindByID(ctx, input.ID)
	if err != nil {
		return nil, err
	}
	if g == nil {
		return nil, fmt.Errorf("fixture group %s not found", input.ID)
	}
	if err := r.ensureProjectAccess(ctx, g.ProjectID); err != nil {
		return nil, err
	}
	if input.Name.IsSet() && input.Name.Value() != nil {
		g.Name = *input.Name.Value()
	}
	if input.Description.IsSet() {
		g.Description = input.Description.Value()
	}
	if input.EosNumber.IsSet() {
		g.EosNumber = input.EosNumber.Value()
	}
	if input.ProjectOrder.IsSet() {
		g.ProjectOrder = input.ProjectOrder.Value()
	}
	if err := r.FixtureGroupRepo.Update(ctx, g); err != nil {
		return nil, err
	}
	// Re-fetch to return a fresh UpdatedAt (GORM's partial-map Updates does
	// not populate the autoUpdateTime field back onto the in-memory struct).
	return r.FixtureGroupRepo.FindByID(ctx, g.ID)
}

func (r *Resolver) deleteFixtureGroup(ctx context.Context, id string) (bool, error) {
	g, err := r.FixtureGroupRepo.FindByID(ctx, id)
	if err != nil {
		return false, err
	}
	if g == nil {
		return false, nil
	}
	if err := r.ensureProjectAccess(ctx, g.ProjectID); err != nil {
		return false, err
	}
	if err := r.FixtureGroupRepo.Delete(ctx, id); err != nil {
		return false, err
	}
	return true, nil
}

// addFixturesToGroup, removeFixturesFromGroup, and reorderFixturesInGroup
// return the group struct loaded BEFORE the membership mutation. This is
// safe because the GraphQL `fixtures` field is resolved lazily by
// fixturesForGroup, which re-reads from the DB and reflects the post-
// mutation state. Don't try to populate `Members` here; gqlgen never
// reads it — it always invokes the field resolver.
func (r *Resolver) addFixturesToGroup(ctx context.Context, groupID string, fixtureIDs []string) (*models.FixtureGroup, error) {
	g, err := r.requireGroup(ctx, groupID)
	if err != nil {
		return nil, err
	}
	deduped := dedupePreserveOrder(fixtureIDs)
	if err := r.assertFixturesInProject(ctx, g.ProjectID, deduped); err != nil {
		return nil, err
	}
	if err := r.FixtureGroupRepo.AddMembers(ctx, groupID, deduped); err != nil {
		return nil, err
	}
	return g, nil
}

func (r *Resolver) removeFixturesFromGroup(ctx context.Context, groupID string, fixtureIDs []string) (*models.FixtureGroup, error) {
	g, err := r.requireGroup(ctx, groupID)
	if err != nil {
		return nil, err
	}
	if err := r.FixtureGroupRepo.RemoveMembers(ctx, groupID, fixtureIDs); err != nil {
		return nil, err
	}
	return g, nil
}

func (r *Resolver) reorderFixturesInGroup(ctx context.Context, input generated.ReorderFixturesInGroupInput) (*models.FixtureGroup, error) {
	g, err := r.requireGroup(ctx, input.GroupID)
	if err != nil {
		return nil, err
	}
	if err := r.FixtureGroupRepo.ReorderMembers(ctx, input.GroupID, input.FixtureIds); err != nil {
		return nil, err
	}
	return g, nil
}

// requireGroup loads the group and gates project access.
func (r *Resolver) requireGroup(ctx context.Context, id string) (*models.FixtureGroup, error) {
	g, err := r.FixtureGroupRepo.FindByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if g == nil {
		return nil, fmt.Errorf("fixture group %s not found", id)
	}
	if err := r.ensureProjectAccess(ctx, g.ProjectID); err != nil {
		return nil, err
	}
	return g, nil
}

// assertFixturesInProject ensures every supplied fixtureID belongs to the
// expected project. Single batch query rather than N+1.
func (r *Resolver) assertFixturesInProject(ctx context.Context, projectID string, fixtureIDs []string) error {
	if len(fixtureIDs) == 0 {
		return nil
	}
	fixtures, err := r.FixtureRepo.FindByIDs(ctx, fixtureIDs)
	if err != nil {
		return err
	}
	byID := make(map[string]string, len(fixtures))
	for i := range fixtures {
		byID[fixtures[i].ID] = fixtures[i].ProjectID
	}
	for _, id := range fixtureIDs {
		pid, ok := byID[id]
		if !ok {
			return fmt.Errorf("fixture %s not found", id)
		}
		if pid != projectID {
			// Don't leak the other project's ID: a multi-tenant caller
			// shouldn't learn that fixture X belongs to a project they
			// can't access.
			return fmt.Errorf("fixture %s does not belong to the specified project", id)
		}
	}
	return nil
}

// Bulk variants — straightforward fanout. Atomic semantics not promised.
func (r *Resolver) bulkCreateFixtureGroups(ctx context.Context, inputs []*generated.CreateFixtureGroupInput) ([]*models.FixtureGroup, error) {
	out := make([]*models.FixtureGroup, 0, len(inputs))
	for _, in := range inputs {
		g, err := r.createFixtureGroup(ctx, *in)
		if err != nil {
			return nil, err
		}
		out = append(out, g)
	}
	return out, nil
}

func (r *Resolver) bulkUpdateFixtureGroups(ctx context.Context, inputs []*generated.UpdateFixtureGroupInput) ([]*models.FixtureGroup, error) {
	out := make([]*models.FixtureGroup, 0, len(inputs))
	for _, in := range inputs {
		g, err := r.updateFixtureGroup(ctx, *in)
		if err != nil {
			return nil, err
		}
		out = append(out, g)
	}
	return out, nil
}

func (r *Resolver) bulkDeleteFixtureGroups(ctx context.Context, ids []string) (int, error) {
	count := 0
	for _, id := range ids {
		ok, err := r.deleteFixtureGroup(ctx, id)
		if err != nil {
			return count, err
		}
		if ok {
			count++
		}
	}
	return count, nil
}

// fixturesForGroup loads members ordered by OrderIndex and resolves to
// FixtureInstance models via a single batch query, preserving member order.
func (r *Resolver) fixturesForGroup(ctx context.Context, groupID string) ([]*models.FixtureInstance, error) {
	members, err := r.FixtureGroupRepo.GetMembers(ctx, groupID)
	if err != nil {
		return nil, err
	}
	if len(members) == 0 {
		return []*models.FixtureInstance{}, nil
	}
	ids := make([]string, len(members))
	for i, m := range members {
		ids[i] = m.FixtureID
	}
	fixtures, err := r.FixtureRepo.FindByIDs(ctx, ids)
	if err != nil {
		return nil, err
	}
	byID := make(map[string]*models.FixtureInstance, len(fixtures))
	for i := range fixtures {
		byID[fixtures[i].ID] = &fixtures[i]
	}
	out := make([]*models.FixtureInstance, 0, len(members))
	for _, m := range members {
		if fi, ok := byID[m.FixtureID]; ok {
			out = append(out, fi)
		}
	}
	return out, nil
}

// dedupePreserveOrder returns a copy of ids with duplicates removed, keeping
// the first occurrence's position. Stable on empty/nil input.
func dedupePreserveOrder(ids []string) []string {
	if len(ids) == 0 {
		return ids
	}
	seen := make(map[string]bool, len(ids))
	out := make([]string, 0, len(ids))
	for _, id := range ids {
		if seen[id] {
			continue
		}
		seen[id] = true
		out = append(out, id)
	}
	return out
}
