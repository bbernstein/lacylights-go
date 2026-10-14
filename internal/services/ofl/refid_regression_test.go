package ofl

import (
	"context"
	"strings"
	"testing"

	"github.com/bbernstein/lacylights-go/internal/database/models"
)

// setupRefIDTestService returns an OFL service backed by testutil.SetupTestDB,
// which creates the production ref_id unique indexes (including the global
// idx_fixture_definitions_refid). Before go#86 the shared setup skipped them,
// which is how the empty-RefID bug in the OFL import paths went unnoticed.
func setupRefIDTestService(t *testing.T) *Service {
	t.Helper()
	service, cleanup := setupTestDB(t)
	t.Cleanup(cleanup)
	return service
}

func renamedFixture(name string) string {
	return strings.Replace(minimalOFLFixture, `"Test Fixture"`, `"`+name+`"`, 1)
}

// Regression: on a fresh database every OFL import after the first failed
// with "UNIQUE constraint failed: fixture_definitions.ref_id" because the
// definition was created with an empty RefID.
func TestImportFixture_MultipleDefinitionsGetUniqueRefIDs(t *testing.T) {
	service := setupRefIDTestService(t)
	ctx := context.Background()

	seen := map[string]bool{}
	for _, name := range []string{"Fixture A", "Fixture B", "Fixture C"} {
		def, err := service.ImportFixture(ctx, "Generic", renamedFixture(name), false)
		if err != nil {
			t.Fatalf("ImportFixture(%q) failed: %v", name, err)
		}
		if def.RefID == "" {
			t.Fatalf("ImportFixture(%q) returned empty RefID", name)
		}
		if def.RefID != def.ID {
			t.Errorf("ImportFixture(%q) RefID = %q, want ID %q", name, def.RefID, def.ID)
		}
		if seen[def.RefID] {
			t.Errorf("duplicate RefID %q", def.RefID)
		}
		seen[def.RefID] = true
	}
}

func TestImportFixture_ReplacePreservesRefID(t *testing.T) {
	service := setupRefIDTestService(t)
	ctx := context.Background()

	// Seed an unrelated definition first so that, without the fix, the
	// next create (with an empty RefID) collides on the global ref_id index.
	if _, err := service.ImportFixture(ctx, "Generic", renamedFixture("Other"), false); err != nil {
		t.Fatalf("seed import failed: %v", err)
	}
	first, err := service.ImportFixture(ctx, "Generic", minimalOFLFixture, false)
	if err != nil {
		t.Fatalf("first import failed: %v", err)
	}
	def, err := service.ImportFixture(ctx, "Generic", minimalOFLFixture, true)
	if err != nil {
		t.Fatalf("replace import failed: %v", err)
	}
	if def.ID == first.ID {
		t.Fatalf("replace should create a new definition ID")
	}
	// RefID is preserved across replace so round-trip references stay stable.
	if def.RefID != first.RefID {
		t.Errorf("replace RefID = %q, want preserved %q", def.RefID, first.RefID)
	}
}

func TestImportFixtureWithHash_MultipleDefinitionsGetUniqueRefIDs(t *testing.T) {
	service := setupRefIDTestService(t)
	ctx := context.Background()

	seen := map[string]bool{}
	refByName := map[string]string{}
	for _, name := range []string{"Fixture A", "Fixture B", "Fixture C"} {
		def, err := service.ImportFixtureWithHash(ctx, "Generic", renamedFixture(name), "hash-"+name, "1.0.0", false)
		if err != nil {
			t.Fatalf("ImportFixtureWithHash(%q) failed: %v", name, err)
		}
		if def.RefID == "" || def.RefID != def.ID {
			t.Errorf("ImportFixtureWithHash(%q) RefID = %q, want ID %q", name, def.RefID, def.ID)
		}
		if seen[def.RefID] {
			t.Errorf("duplicate RefID %q", def.RefID)
		}
		seen[def.RefID] = true
		refByName[name] = def.RefID
	}

	// Update path (update=true) re-creates the definition and must preserve RefID.
	def, err := service.ImportFixtureWithHash(ctx, "Generic", renamedFixture("Fixture A"), "hash-new", "1.0.1", true)
	if err != nil {
		t.Fatalf("ImportFixtureWithHash update failed: %v", err)
	}
	if def.RefID != refByName["Fixture A"] {
		t.Errorf("update RefID = %q, want preserved %q", def.RefID, refByName["Fixture A"])
	}
}

func TestRefIDForImport(t *testing.T) {
	if got := refIDForImport(nil, "new"); got != "new" {
		t.Errorf("nil existing: got %q, want %q", got, "new")
	}
	if got := refIDForImport(&models.FixtureDefinition{ID: "old"}, "new"); got != "new" {
		t.Errorf("existing with empty RefID: got %q, want %q", got, "new")
	}
	if got := refIDForImport(&models.FixtureDefinition{ID: "old", RefID: "keep"}, "new"); got != "keep" {
		t.Errorf("existing with RefID: got %q, want %q", got, "keep")
	}
}
