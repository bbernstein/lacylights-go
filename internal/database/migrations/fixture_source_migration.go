package migrations

import (
	"fmt"

	"gorm.io/gorm"
)

// BackfillFixtureSources classifies fixture definitions that predate the
// source column. Adding the column gives every existing row the default
// 'USER'; this moves OFL imports, built-ins and Eos-synthesized definitions
// to their real source. Only rows still marked 'USER' are touched, so it is
// idempotent and safe to run on every server start.
func BackfillFixtureSources(db *gorm.DB) error {
	const table = "fixture_definitions"
	m := db.Migrator()
	if !m.HasTable(table) {
		return nil
	}
	for _, col := range []string{"source", "source_hash", "source_version", "ofl_source_hash", "ofl_version", "is_built_in", "ref_id"} {
		if !m.HasColumn(table, col) {
			return nil
		}
	}
	stmts := []string{
		// OFL imports carry an OFL source hash.
		`UPDATE fixture_definitions
		    SET source = 'OFL',
		        source_hash = COALESCE(source_hash, ofl_source_hash),
		        source_version = COALESCE(source_version, ofl_version)
		  WHERE source = 'USER' AND ofl_source_hash IS NOT NULL`,
		`UPDATE fixture_definitions SET source = 'BUILTIN'
		  WHERE source = 'USER' AND is_built_in`,
		// Eos ASCII import synthesizes definitions with "def-<mfr>-<model>" RefIDs.
		`UPDATE fixture_definitions SET source = 'SYNTHESIZED'
		  WHERE source = 'USER' AND ref_id LIKE 'def-%'`,
	}
	for _, stmt := range stmts {
		if err := db.Exec(stmt).Error; err != nil {
			return fmt.Errorf("backfill fixture sources: %w", err)
		}
	}
	return nil
}
