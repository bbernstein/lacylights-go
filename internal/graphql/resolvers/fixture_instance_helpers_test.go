package resolvers

import (
	"context"
	"testing"

	"github.com/bbernstein/lacylights-go/internal/database/models"
	"github.com/bbernstein/lacylights-go/internal/database/repositories"
	"github.com/bbernstein/lacylights-go/internal/graphql/generated"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func strPtr(s string) *string { return &s }

// newResolverWithFixtureRepo wires a *fixtureInstanceResolver against a real
// in-memory SQLite-backed FixtureRepository so tests can exercise the
// repo-lookup fallback path in lookupDefinition.
func newResolverWithFixtureRepo(t *testing.T) (*fixtureInstanceResolver, *gorm.DB) {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		t.Fatalf("failed to open in-memory db: %v", err)
	}
	if err := db.AutoMigrate(
		&models.FixtureDefinition{},
		&models.ChannelDefinition{},
		&models.FixtureMode{},
		&models.ModeChannel{},
		&models.FixtureInstance{},
		&models.InstanceChannel{},
	); err != nil {
		t.Fatalf("failed to migrate db: %v", err)
	}
	r := &fixtureInstanceResolver{Resolver: &Resolver{
		FixtureRepo: repositories.NewFixtureRepository(db),
	}}
	return r, db
}

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

	t.Run("returns empty when FixtureRepo nil and Definition not preloaded", func(t *testing.T) {
		// DefinitionID set but no FixtureRepo wired — must not panic.
		obj := &models.FixtureInstance{DefinitionID: "def-1"}
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

func TestFixtureInstanceResolver_Type_FallsBackToDefinition(t *testing.T) {
	r := &fixtureInstanceResolver{Resolver: &Resolver{}}
	ctx := context.Background()

	t.Run("uses denormalized type when set", func(t *testing.T) {
		typeStr := string(generated.FixtureTypeLedPar)
		obj := &models.FixtureInstance{
			Type:       &typeStr,
			Definition: &models.FixtureDefinition{Type: string(generated.FixtureTypeMovingHead)},
		}
		got, err := r.Type(ctx, obj)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got != generated.FixtureTypeLedPar {
			t.Errorf("got %v, want %v", got, generated.FixtureTypeLedPar)
		}
	})

	t.Run("falls back to preloaded Definition when nil", func(t *testing.T) {
		obj := &models.FixtureInstance{
			Definition: &models.FixtureDefinition{Type: string(generated.FixtureTypeMovingHead)},
		}
		got, err := r.Type(ctx, obj)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got != generated.FixtureTypeMovingHead {
			t.Errorf("got %v, want %v", got, generated.FixtureTypeMovingHead)
		}
	})

	t.Run("returns OTHER default when no type and no definition", func(t *testing.T) {
		obj := &models.FixtureInstance{}
		got, err := r.Type(ctx, obj)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got != generated.FixtureTypeOther {
			t.Errorf("got %v, want %v", got, generated.FixtureTypeOther)
		}
	})
}

// TestFixtureInstanceResolver_RepoLookupFallback exercises the path where
// Definition is not preloaded but DefinitionID is set, so lookupDefinition
// must call FixtureRepo.FindDefinitionByID. It also asserts that the result
// is memoized back onto obj.Definition for subsequent calls.
func TestFixtureInstanceResolver_RepoLookupFallback(t *testing.T) {
	r, db := newResolverWithFixtureRepo(t)
	ctx := context.Background()

	def := &models.FixtureDefinition{
		ID:           "def-repo-lookup",
		Manufacturer: "ETC",
		Model:        "Source Four",
		Type:         string(generated.FixtureTypeLedPar),
	}
	if err := db.Create(def).Error; err != nil {
		t.Fatalf("failed to seed definition: %v", err)
	}

	t.Run("Manufacturer falls back via repo", func(t *testing.T) {
		obj := &models.FixtureInstance{DefinitionID: def.ID}
		got, err := r.Manufacturer(ctx, obj)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got != "ETC" {
			t.Errorf("got %q, want %q", got, "ETC")
		}
		if obj.Definition == nil || obj.Definition.ID != def.ID {
			t.Errorf("expected obj.Definition to be memoized, got %+v", obj.Definition)
		}
	})

	t.Run("Model falls back via repo", func(t *testing.T) {
		obj := &models.FixtureInstance{DefinitionID: def.ID}
		got, err := r.Model(ctx, obj)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got != "Source Four" {
			t.Errorf("got %q, want %q", got, "Source Four")
		}
	})

	t.Run("Type falls back via repo", func(t *testing.T) {
		obj := &models.FixtureInstance{DefinitionID: def.ID}
		got, err := r.Type(ctx, obj)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got != generated.FixtureTypeLedPar {
			t.Errorf("got %v, want %v", got, generated.FixtureTypeLedPar)
		}
	})

	t.Run("returns empty when DefinitionID not in DB", func(t *testing.T) {
		obj := &models.FixtureInstance{DefinitionID: "does-not-exist"}
		// FindDefinitionByID returns (nil, nil) for missing rows, so the
		// resolver should return an empty string with no error.
		got, err := r.Manufacturer(ctx, obj)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got != "" {
			t.Errorf("got %q, want empty string", got)
		}
	})
}
