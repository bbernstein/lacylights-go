package resolvers

import (
	"context"

	"github.com/bbernstein/lacylights-go/internal/database/models"
)

// lookupDefinition returns the linked FixtureDefinition for a FixtureInstance,
// preferring a preloaded relation and falling back to a repo lookup.
//
// Older fixtures (notably those created via the Eos ASCII importer before
// denormalized fields were populated at creation) may have nil Manufacturer,
// Model, or Type and rely on this fallback for resolver output to be correct.
//
// Once a definition is loaded via the repo, it is memoized back onto the
// instance (`obj.Definition`) so subsequent field resolvers on the same
// instance reuse the same fetch within a single GraphQL request. This is
// safe under gqlgen's per-request execution model where a single
// `*FixtureInstance` is not resolved concurrently across goroutines.
//
// Returns (nil, nil) when no definition can be resolved (no preloaded
// relation and no DefinitionID, or the resolver/repo dependency is missing).
// Repository errors are propagated so callers can surface real DB problems
// rather than silently returning empty strings.
func (r *fixtureInstanceResolver) lookupDefinition(ctx context.Context, obj *models.FixtureInstance) (*models.FixtureDefinition, error) {
	if obj == nil {
		return nil, nil
	}
	if obj.Definition != nil {
		return obj.Definition, nil
	}
	if obj.DefinitionID == "" {
		return nil, nil
	}
	if r == nil || r.Resolver == nil || r.FixtureRepo == nil {
		return nil, nil
	}
	def, err := r.FixtureRepo.FindDefinitionByID(ctx, obj.DefinitionID)
	if err != nil {
		return nil, err
	}
	if def == nil {
		return nil, nil
	}
	obj.Definition = def
	return def, nil
}
