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
// Model, Type, ModeName, or ChannelCount and rely on this fallback for
// resolver output to be correct.
func (r *fixtureInstanceResolver) lookupDefinition(ctx context.Context, obj *models.FixtureInstance) *models.FixtureDefinition {
	if obj == nil {
		return nil
	}
	if obj.Definition != nil {
		return obj.Definition
	}
	if obj.DefinitionID == "" {
		return nil
	}
	def, err := r.FixtureRepo.FindDefinitionByID(ctx, obj.DefinitionID)
	if err != nil || def == nil {
		return nil
	}
	obj.Definition = def
	return def
}
