package resolvers

import (
	"context"
	"testing"

	"github.com/bbernstein/lacylights-go/internal/database/models"
)

func strPtr(s string) *string { return &s }

func TestFixtureInstanceResolver_Manufacturer_FallsBackToDefinition(t *testing.T) {
	r := &fixtureInstanceResolver{Resolver: &Resolver{}}
	ctx := context.Background()

	t.Run("uses denormalized field when set", func(t *testing.T) {
		obj := &models.FixtureInstance{
			Manufacturer: strPtr("Chauvet"),
			Definition:   &models.FixtureDefinition{Manufacturer: "Other"},
		}
		got, err := r.Manufacturer(ctx, obj)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got != "Chauvet" {
			t.Errorf("got %q, want %q", got, "Chauvet")
		}
	})

	t.Run("falls back to preloaded Definition when nil", func(t *testing.T) {
		obj := &models.FixtureInstance{
			Definition: &models.FixtureDefinition{Manufacturer: "Chauvet"},
		}
		got, err := r.Manufacturer(ctx, obj)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got != "Chauvet" {
			t.Errorf("got %q, want %q", got, "Chauvet")
		}
	})

	t.Run("returns empty when both nil and no definition resolvable", func(t *testing.T) {
		// No DefinitionID set, no Definition preloaded; lookup should bail.
		obj := &models.FixtureInstance{}
		got, err := r.Manufacturer(ctx, obj)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got != "" {
			t.Errorf("got %q, want empty string", got)
		}
	})
}

func TestFixtureInstanceResolver_Model_FallsBackToDefinition(t *testing.T) {
	r := &fixtureInstanceResolver{Resolver: &Resolver{}}
	ctx := context.Background()

	t.Run("uses denormalized field when set", func(t *testing.T) {
		obj := &models.FixtureInstance{
			Model:      strPtr("SlimPAR Q12"),
			Definition: &models.FixtureDefinition{Model: "Other"},
		}
		got, err := r.Model(ctx, obj)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got != "SlimPAR Q12" {
			t.Errorf("got %q, want %q", got, "SlimPAR Q12")
		}
	})

	t.Run("falls back to preloaded Definition when nil", func(t *testing.T) {
		obj := &models.FixtureInstance{
			Definition: &models.FixtureDefinition{Model: "SlimPAR Q12"},
		}
		got, err := r.Model(ctx, obj)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got != "SlimPAR Q12" {
			t.Errorf("got %q, want %q", got, "SlimPAR Q12")
		}
	})
}
