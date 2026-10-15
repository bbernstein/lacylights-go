package resolvers

import "github.com/bbernstein/lacylights-go/internal/graphql/generated"

// toFixtureSources converts stored source names to the GraphQL enum.
func toFixtureSources(order []string) []generated.FixtureSource {
	out := make([]generated.FixtureSource, len(order))
	for i, s := range order {
		out[i] = generated.FixtureSource(s)
	}
	return out
}
