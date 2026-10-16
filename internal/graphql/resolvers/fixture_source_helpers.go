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

// toFixtureSource converts a stored source to the GraphQL enum. An empty or
// unknown value (for example written by a newer server) reads as USER rather
// than failing the whole query when the enum is marshalled.
func toFixtureSource(s string) generated.FixtureSource {
	if src := generated.FixtureSource(s); src.IsValid() {
		return src
	}
	return generated.FixtureSourceUser
}
