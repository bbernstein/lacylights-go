package testutil

import (
	"strings"
	"testing"

	"github.com/bbernstein/lacylights-go/internal/database/models"
)

// SetupTestDB must create the same ref_id unique indexes as production so
// that create paths which leave RefID empty fail in tests (go#86).
func TestSetupTestDB_CreatesRefIDIndexes(t *testing.T) {
	testDB, cleanup := SetupTestDB(t)
	defer cleanup()

	for _, idx := range []struct{ table, name string }{
		{"fixture_definitions", "idx_fixture_definitions_refid"},
		{"fixture_instances", "idx_fixture_instances_project_refid"},
		{"looks", "idx_looks_project_refid"},
		{"look_boards", "idx_look_boards_project_refid"},
	} {
		if !testDB.DB.Migrator().HasIndex(idx.table, idx.name) {
			t.Errorf("expected index %s on %s", idx.name, idx.table)
		}
	}

	// Two definitions with an empty RefID must collide on the unique index.
	if err := testDB.DB.Create(&models.FixtureDefinition{ID: "d1", Manufacturer: "A", Model: "One"}).Error; err != nil {
		t.Fatalf("first create failed: %v", err)
	}
	err := testDB.DB.Create(&models.FixtureDefinition{ID: "d2", Manufacturer: "B", Model: "Two"}).Error
	if err == nil || !strings.Contains(err.Error(), "UNIQUE constraint failed: fixture_definitions.ref_id") {
		t.Fatalf("expected unique ref_id violation for a second empty RefID, got %v", err)
	}
}
