package migrations

import (
	"testing"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func openTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	return db
}

func TestBackfillRefIDs_PopulatesEmptyRefIDs(t *testing.T) {
	db := openTestDB(t)
	if err := db.Exec(`CREATE TABLE fixture_instances (id TEXT PRIMARY KEY, project_id TEXT, ref_id TEXT)`).Error; err != nil {
		t.Fatalf("create table: %v", err)
	}
	if err := db.Exec(`INSERT INTO fixture_instances (id, project_id, ref_id) VALUES ('a','p1',''),('b','p1',NULL),('c','p1','custom')`).Error; err != nil {
		t.Fatalf("seed: %v", err)
	}

	if err := BackfillRefIDs(db); err != nil {
		t.Fatalf("backfill: %v", err)
	}

	var rows []struct {
		ID, RefID string
	}
	if err := db.Raw(`SELECT id, ref_id FROM fixture_instances ORDER BY id`).Scan(&rows).Error; err != nil {
		t.Fatalf("scan: %v", err)
	}
	want := map[string]string{"a": "a", "b": "b", "c": "custom"}
	for _, r := range rows {
		if got := r.RefID; got != want[r.ID] {
			t.Errorf("row %s: ref_id = %q, want %q", r.ID, got, want[r.ID])
		}
	}
}

func TestBackfillRefIDs_SkipsMissingTablesAndColumns(t *testing.T) {
	db := openTestDB(t)
	// No tables exist — backfill should be a no-op (no error).
	if err := BackfillRefIDs(db); err != nil {
		t.Fatalf("backfill on empty schema: %v", err)
	}
}

func TestCreateRefIDIndexes_Idempotent(t *testing.T) {
	db := openTestDB(t)
	for _, table := range refIDTables {
		// fixture_groups has an additional eos_number column referenced by
		// the partial unique index created in CreateRefIDIndexes.
		schema := `(id TEXT PRIMARY KEY, project_id TEXT, ref_id TEXT)`
		if table == "fixture_groups" {
			schema = `(id TEXT PRIMARY KEY, project_id TEXT, ref_id TEXT, eos_number INTEGER)`
		}
		if err := db.Exec(`CREATE TABLE ` + table + ` ` + schema).Error; err != nil {
			t.Fatalf("create %s: %v", table, err)
		}
	}
	if err := CreateRefIDIndexes(db); err != nil {
		t.Fatalf("create indexes: %v", err)
	}
	// Run twice — IF NOT EXISTS should make this a no-op.
	if err := CreateRefIDIndexes(db); err != nil {
		t.Fatalf("create indexes second run: %v", err)
	}
}

func TestCreateRefIDIndexes_EnforcesEosNumberPartialUniqueness(t *testing.T) {
	db := openTestDB(t)
	// Create all ref_id tables — CreateRefIDIndexes builds indexes for all of them.
	for _, table := range refIDTables {
		schema := `(id TEXT PRIMARY KEY, project_id TEXT, ref_id TEXT)`
		if table == "fixture_groups" {
			schema = `(id TEXT PRIMARY KEY, project_id TEXT, ref_id TEXT, eos_number INTEGER)`
		}
		if err := db.Exec(`CREATE TABLE ` + table + ` ` + schema).Error; err != nil {
			t.Fatalf("create %s: %v", table, err)
		}
	}
	if err := CreateRefIDIndexes(db); err != nil {
		t.Fatalf("create indexes: %v", err)
	}

	// Two groups in the same project with EosNumber=1 → second insert must fail.
	if err := db.Exec(`INSERT INTO fixture_groups (id, project_id, ref_id, eos_number) VALUES ('a','p1','a',1)`).Error; err != nil {
		t.Fatalf("first: %v", err)
	}
	if err := db.Exec(`INSERT INTO fixture_groups (id, project_id, ref_id, eos_number) VALUES ('b','p1','b',1)`).Error; err == nil {
		t.Errorf("expected unique-constraint failure on duplicate EosNumber")
	}
	// Two groups with NULL EosNumber must succeed (partial index ignores NULLs).
	if err := db.Exec(`INSERT INTO fixture_groups (id, project_id, ref_id, eos_number) VALUES ('c','p1','c',NULL)`).Error; err != nil {
		t.Fatalf("third: %v", err)
	}
	if err := db.Exec(`INSERT INTO fixture_groups (id, project_id, ref_id, eos_number) VALUES ('d','p1','d',NULL)`).Error; err != nil {
		t.Fatalf("fourth: %v", err)
	}

	// Same EosNumber in DIFFERENT projects must succeed — the partial
	// unique index is scoped on (project_id, eos_number), not global.
	// This is the multi-tenant invariant.
	if err := db.Exec(`INSERT INTO fixture_groups (id, project_id, ref_id, eos_number) VALUES ('e','p2','e',1)`).Error; err != nil {
		t.Errorf("same eos_number in different project should be allowed; got: %v", err)
	}
}

// TestCreateRefIDIndexes_SkipsTableMissingRefIDColumn guards the defensive
// HasColumn check: if a legacy DB still lacks the ref_id column on one of
// the ref_id tables (e.g. AutoMigrate was somehow skipped), index creation
// must skip rather than fail with a cryptic "no such column" SQL error.
func TestCreateRefIDIndexes_SkipsTableMissingRefIDColumn(t *testing.T) {
	db := openTestDB(t)
	// Create all tables, but omit ref_id from fixture_instances.
	for _, table := range refIDTables {
		schema := `(id TEXT PRIMARY KEY, project_id TEXT, ref_id TEXT)`
		switch table {
		case "fixture_instances":
			schema = `(id TEXT PRIMARY KEY, project_id TEXT)` // legacy schema, no ref_id
		case "fixture_definitions":
			// fixture_definitions has no project_id in the real model.
			schema = `(id TEXT PRIMARY KEY, ref_id TEXT)`
		case "fixture_groups":
			schema = `(id TEXT PRIMARY KEY, project_id TEXT, ref_id TEXT, eos_number INTEGER)`
		}
		if err := db.Exec(`CREATE TABLE ` + table + ` ` + schema).Error; err != nil {
			t.Fatalf("create %s: %v", table, err)
		}
	}
	if err := CreateRefIDIndexes(db); err != nil {
		t.Fatalf("expected skip on missing ref_id column, got error: %v", err)
	}
	// The fixture_instances index must not exist.
	var n int
	if err := db.Raw(`SELECT count(*) FROM sqlite_master WHERE type='index' AND name='idx_fixture_instances_project_refid'`).Scan(&n).Error; err != nil {
		t.Fatalf("query sqlite_master: %v", err)
	}
	if n != 0 {
		t.Errorf("expected fixture_instances index to be skipped, found %d", n)
	}
	// Indexes on tables that DO have ref_id must still be created.
	if err := db.Raw(`SELECT count(*) FROM sqlite_master WHERE type='index' AND name='idx_looks_project_refid'`).Scan(&n).Error; err != nil {
		t.Fatalf("query sqlite_master: %v", err)
	}
	if n != 1 {
		t.Errorf("expected looks index to be created, found %d", n)
	}
}
