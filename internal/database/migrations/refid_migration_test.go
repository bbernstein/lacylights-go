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
		if err := db.Exec(`CREATE TABLE ` + table + ` (id TEXT PRIMARY KEY, project_id TEXT, ref_id TEXT)`).Error; err != nil {
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
