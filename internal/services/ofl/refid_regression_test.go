package ofl

import (
	"context"
	"strings"
	"testing"

	"github.com/bbernstein/lacylights-go/internal/database/migrations"
)

// setupTestDBWithRefIDIndexes mirrors production startup, which creates the
// unique ref_id indexes (including the global idx_fixture_definitions_refid)
// after AutoMigrate. testutil.SetupTestDB does not create them, which is how
// the empty-RefID bug in the OFL import paths went unnoticed.
func setupTestDBWithRefIDIndexes(t *testing.T) (*Service, func()) {
	t.Helper()
	service, cleanup := setupTestDB(t)
	if err := migrations.CreateRefIDIndexes(service.db); err != nil {
		cleanup()
		t.Fatalf("CreateRefIDIndexes: %v", err)
	}
	return service, cleanup
}

func renamedFixture(name string) string {
	return strings.Replace(minimalOFLFixture, `"Test Fixture"`, `"`+name+`"`, 1)
}

// Regression: on a fresh database every OFL import after the first failed
// with "UNIQUE constraint failed: fixture_definitions.ref_id" because the
// definition was created with an empty RefID.
func TestImportFixture_MultipleDefinitionsGetUniqueRefIDs(t *testing.T) {
	service, cleanup := setupTestDBWithRefIDIndexes(t)
	defer cleanup()
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

func TestImportFixture_ReplaceAssignsRefID(t *testing.T) {
	service, cleanup := setupTestDBWithRefIDIndexes(t)
	defer cleanup()
	ctx := context.Background()

	if _, err := service.ImportFixture(ctx, "Generic", renamedFixture("Other"), false); err != nil {
		t.Fatalf("seed import failed: %v", err)
	}
	if _, err := service.ImportFixture(ctx, "Generic", minimalOFLFixture, false); err != nil {
		t.Fatalf("first import failed: %v", err)
	}
	def, err := service.ImportFixture(ctx, "Generic", minimalOFLFixture, true)
	if err != nil {
		t.Fatalf("replace import failed: %v", err)
	}
	if def.RefID == "" || def.RefID != def.ID {
		t.Errorf("RefID = %q, want ID %q", def.RefID, def.ID)
	}
}

func TestImportFixtureWithHash_MultipleDefinitionsGetUniqueRefIDs(t *testing.T) {
	service, cleanup := setupTestDBWithRefIDIndexes(t)
	defer cleanup()
	ctx := context.Background()

	seen := map[string]bool{}
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
	}

	// Update path (update=true) re-creates the definition and must also set RefID.
	def, err := service.ImportFixtureWithHash(ctx, "Generic", renamedFixture("Fixture A"), "hash-new", "1.0.1", true)
	if err != nil {
		t.Fatalf("ImportFixtureWithHash update failed: %v", err)
	}
	if def.RefID == "" || def.RefID != def.ID {
		t.Errorf("update RefID = %q, want ID %q", def.RefID, def.ID)
	}
}
