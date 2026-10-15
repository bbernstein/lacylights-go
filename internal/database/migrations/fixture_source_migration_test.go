package migrations

import "testing"

const fixtureDefsDDL = `CREATE TABLE fixture_definitions (
	id TEXT PRIMARY KEY, ref_id TEXT, is_built_in NUMERIC DEFAULT false,
	ofl_source_hash TEXT, ofl_version TEXT,
	source TEXT NOT NULL DEFAULT 'USER', source_id TEXT, source_version TEXT, source_hash TEXT)`

func TestBackfillFixtureSources_ClassifiesLegacyRows(t *testing.T) {
	db := openTestDB(t)
	if err := db.Exec(fixtureDefsDDL).Error; err != nil {
		t.Fatalf("create table: %v", err)
	}
	seed := `INSERT INTO fixture_definitions (id, ref_id, is_built_in, ofl_source_hash, ofl_version, source) VALUES
		('ofl', 'ofl', false, 'h1', 'v1', 'USER'),
		('builtin', 'builtin', true, NULL, NULL, 'USER'),
		('synth', 'def-Chauvet-Par', false, NULL, NULL, 'USER'),
		('user', 'cabc', false, NULL, NULL, 'USER'),
		('gdtf', 'gdtf', false, NULL, NULL, 'GDTF')`
	if err := db.Exec(seed).Error; err != nil {
		t.Fatalf("seed: %v", err)
	}

	for i := 0; i < 2; i++ { // idempotent
		if err := BackfillFixtureSources(db); err != nil {
			t.Fatalf("backfill run %d: %v", i, err)
		}
	}

	var rows []struct {
		ID, Source                string
		SourceHash, SourceVersion *string
	}
	if err := db.Raw(`SELECT id, source, source_hash, source_version FROM fixture_definitions`).Scan(&rows).Error; err != nil {
		t.Fatalf("scan: %v", err)
	}
	want := map[string]string{"ofl": "OFL", "builtin": "BUILTIN", "synth": "SYNTHESIZED", "user": "USER", "gdtf": "GDTF"}
	for _, r := range rows {
		if r.Source != want[r.ID] {
			t.Errorf("%s: source = %q, want %q", r.ID, r.Source, want[r.ID])
		}
		if r.ID == "ofl" && (r.SourceHash == nil || *r.SourceHash != "h1" || r.SourceVersion == nil || *r.SourceVersion != "v1") {
			t.Errorf("ofl: hash/version not copied: %v %v", r.SourceHash, r.SourceVersion)
		}
	}
}

func TestBackfillFixtureSources_SkipsMissingTableOrColumn(t *testing.T) {
	db := openTestDB(t)
	if err := BackfillFixtureSources(db); err != nil {
		t.Fatalf("no table: %v", err)
	}
	if err := db.Exec(`CREATE TABLE fixture_definitions (id TEXT PRIMARY KEY)`).Error; err != nil {
		t.Fatal(err)
	}
	if err := BackfillFixtureSources(db); err != nil {
		t.Fatalf("no source column: %v", err)
	}
}
