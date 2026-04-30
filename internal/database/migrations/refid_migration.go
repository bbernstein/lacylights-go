// Package migrations contains one-shot, idempotent schema migrations
// invoked from the server's startup AutoMigrate flow.
package migrations

import (
	"fmt"
	"log"

	"gorm.io/gorm"
)

// refIDTables enumerates every table that received a ref_id column in
// task 14c. Order doesn't matter — each statement is independent.
var refIDTables = []string{
	"fixture_instances",
	"fixture_definitions",
	"looks",
	"look_boards",
	"fixture_groups",
}

// BackfillRefIDs copies id → ref_id for every row whose ref_id is empty.
// Idempotent: safe to run on every server start.
func BackfillRefIDs(db *gorm.DB) error {
	for _, table := range refIDTables {
		if !db.Migrator().HasTable(table) {
			continue
		}
		if !db.Migrator().HasColumn(table, "ref_id") {
			continue
		}
		stmt := fmt.Sprintf(
			`UPDATE %s SET ref_id = id WHERE ref_id IS NULL OR ref_id = ''`,
			table,
		)
		if err := db.Exec(stmt).Error; err != nil {
			return fmt.Errorf("backfill ref_id for %s: %w", table, err)
		}
	}
	return nil
}

// CreateRefIDIndexes creates the unique indexes that enforce ref_id
// uniqueness. Run after BackfillRefIDs so existing rows can satisfy the
// constraint. Uses CREATE UNIQUE INDEX IF NOT EXISTS for idempotency.
//
// Each index is silently skipped (rather than failing) when its required
// column is absent. The missing column should have been added by
// AutoMigrate earlier in startup; if it wasn't, the problem will surface
// when the column is first used rather than here with a cryptic SQL
// error. The pre-flight check matches the equivalent guard in
// BackfillRefIDs and keeps both helpers safe on partially-migrated DBs.
func CreateRefIDIndexes(db *gorm.DB) error {
	stmts := []struct {
		name      string
		table     string
		reqCols   []string
		sql       string
	}{
		{
			"idx_fixture_instances_project_refid",
			"fixture_instances",
			[]string{"project_id", "ref_id"},
			`CREATE UNIQUE INDEX IF NOT EXISTS idx_fixture_instances_project_refid
				ON fixture_instances(project_id, ref_id)`,
		},
		{
			"idx_fixture_definitions_refid",
			"fixture_definitions",
			[]string{"ref_id"},
			`CREATE UNIQUE INDEX IF NOT EXISTS idx_fixture_definitions_refid
				ON fixture_definitions(ref_id)`,
		},
		{
			"idx_looks_project_refid",
			"looks",
			[]string{"project_id", "ref_id"},
			`CREATE UNIQUE INDEX IF NOT EXISTS idx_looks_project_refid
				ON looks(project_id, ref_id)`,
		},
		{
			"idx_look_boards_project_refid",
			"look_boards",
			[]string{"project_id", "ref_id"},
			`CREATE UNIQUE INDEX IF NOT EXISTS idx_look_boards_project_refid
				ON look_boards(project_id, ref_id)`,
		},
		{
			"idx_fixture_groups_project_refid",
			"fixture_groups",
			[]string{"project_id", "ref_id"},
			`CREATE UNIQUE INDEX IF NOT EXISTS idx_fixture_groups_project_refid
				ON fixture_groups(project_id, ref_id)`,
		},
		{
			"idx_fixture_groups_project_eos_number",
			"fixture_groups",
			[]string{"project_id", "eos_number"},
			`CREATE UNIQUE INDEX IF NOT EXISTS idx_fixture_groups_project_eos_number
				ON fixture_groups(project_id, eos_number)
				WHERE eos_number IS NOT NULL`,
		},
	}
	for _, s := range stmts {
		if !db.Migrator().HasTable(s.table) {
			continue
		}
		var missing []string
		for _, c := range s.reqCols {
			if !db.Migrator().HasColumn(s.table, c) {
				missing = append(missing, c)
			}
		}
		if len(missing) > 0 {
			// Log so a partially-migrated DB doesn't fail silently — the
			// next operator can see which column AutoMigrate didn't add.
			log.Printf("migrations: skipping index %s on %s: missing column(s) %v",
				s.name, s.table, missing)
			continue
		}
		if err := db.Exec(s.sql).Error; err != nil {
			return fmt.Errorf("create index %s: %w", s.name, err)
		}
	}
	return nil
}
