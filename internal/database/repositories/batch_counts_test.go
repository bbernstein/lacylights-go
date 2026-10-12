package repositories

import (
	"context"
	"fmt"
	"testing"

	"github.com/bbernstein/lacylights-go/internal/database/models"
)

func TestBatchedCountsAndLookups(t *testing.T) {
	tdb, cleanup := setupTestDB(t)
	defer cleanup()
	db := tdb.DB
	ctx := context.Background()
	must := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
	}

	// fixture_values: look a has 2, look b has 1, look c none.
	must(db.Create(&[]models.FixtureValue{
		{ID: "fv1", LookID: "a", FixtureID: "f1"},
		{ID: "fv2", LookID: "a", FixtureID: "f2"},
		{ID: "fv3", LookID: "b", FixtureID: "f1"},
		{ID: "fv4", LookID: "other", FixtureID: "f1"},
	}).Error)
	looks, err := NewLookRepository(db).CountFixturesByLookIDs(ctx, []string{"a", "b", "c"})
	must(err)
	if looks["a"] != 2 || looks["b"] != 1 || looks["c"] != 0 || len(looks) != 2 {
		t.Errorf("look counts = %v", looks)
	}

	must(db.Create(&[]models.Cue{
		{ID: "c1", CueListID: "l1"}, {ID: "c2", CueListID: "l1"}, {ID: "c3", CueListID: "l2"},
	}).Error)
	cues, err := NewCueListRepository(db).CountCuesByCueListIDs(ctx, []string{"l1", "l2"})
	must(err)
	if cues["l1"] != 2 || cues["l2"] != 1 {
		t.Errorf("cue counts = %v", cues)
	}

	must(db.AutoMigrate(&models.EffectFixture{}))
	must(db.Create(&[]models.EffectFixture{
		{ID: "e1", EffectID: "fx1", FixtureID: "f1"}, {ID: "e2", EffectID: "fx1", FixtureID: "f2"},
	}).Error)
	effects, err := NewEffectRepository(db).CountFixturesByEffectIDs(ctx, []string{"fx1", "fx2"})
	must(err)
	if effects["fx1"] != 2 || effects["fx2"] != 0 {
		t.Errorf("effect counts = %v", effects)
	}

	must(db.Create(&[]models.FixtureDefinition{
		{ID: "d1", Manufacturer: "A", Model: "One"}, {ID: "d2", Manufacturer: "B", Model: "Two"},
	}).Error)
	defs, err := NewFixtureRepository(db).FindDefinitionsByIDs(ctx, []string{"d1", "missing"})
	must(err)
	if len(defs) != 1 || defs["d1"] == nil || defs["d1"].Model != "One" {
		t.Errorf("defs = %v", defs)
	}

	must(db.Create(&[]models.LookBoardButton{
		{ID: "b1", LookBoardID: "board1", LayoutX: 5, LayoutY: 1},
		{ID: "b2", LookBoardID: "board1", LayoutX: 1, LayoutY: 1},
		{ID: "b3", LookBoardID: "board1", LayoutX: 0, LayoutY: 0},
		{ID: "b4", LookBoardID: "board2"},
	}).Error)
	buttons, err := NewLookBoardRepository(db).GetButtonsByBoardIDs(ctx, []string{"board1"})
	must(err)
	b1 := buttons["board1"]
	if len(buttons) != 1 || len(b1) != 3 || b1[0].ID != "b3" || b1[1].ID != "b2" || b1[2].ID != "b1" {
		t.Errorf("buttons = %+v", buttons)
	}
}

func TestBatchedCounts_EmptyInput(t *testing.T) {
	tdb, cleanup := setupTestDB(t)
	defer cleanup()
	ctx := context.Background()
	db := tdb.DB

	if m, err := NewLookRepository(db).CountFixturesByLookIDs(ctx, nil); err != nil || len(m) != 0 {
		t.Errorf("looks = %v, %v", m, err)
	}
	if m, err := NewFixtureRepository(db).FindDefinitionsByIDs(ctx, nil); err != nil || len(m) != 0 {
		t.Errorf("defs = %v, %v", m, err)
	}
	if m, err := NewLookBoardRepository(db).GetButtonsByBoardIDs(ctx, nil); err != nil || len(m) != 0 {
		t.Errorf("buttons = %v, %v", m, err)
	}
}

func TestBatchedCounts_DBError(t *testing.T) {
	tdb, cleanup := setupTestDB(t)
	ctx := context.Background()
	db := tdb.DB
	cleanup() // closed database: every query fails

	if _, err := NewLookRepository(db).CountFixturesByLookIDs(ctx, []string{"a"}); err == nil {
		t.Error("expected count error")
	}
	if _, err := NewFixtureRepository(db).FindDefinitionsByIDs(ctx, []string{"a"}); err == nil {
		t.Error("expected definitions error")
	}
	if _, err := NewLookBoardRepository(db).GetButtonsByBoardIDs(ctx, []string{"a"}); err == nil {
		t.Error("expected buttons error")
	}
}

func TestChunkIDs(t *testing.T) {
	if got := chunkIDs(nil); len(got) != 0 {
		t.Errorf("nil = %v", got)
	}
	ids := make([]string, 2*maxInVars+1)
	for i := range ids {
		ids[i] = fmt.Sprintf("id%d", i)
	}
	got := chunkIDs(ids)
	if len(got) != 3 || len(got[0]) != maxInVars || len(got[2]) != 1 {
		t.Errorf("chunks = %d", len(got))
	}
}

// Lookups with more IDs than one IN clause takes are split into chunks and
// still return every match.
func TestBatchedLookups_ManyIDs(t *testing.T) {
	tdb, cleanup := setupTestDB(t)
	defer cleanup()
	db := tdb.DB
	ctx := context.Background()

	n := 2*maxInVars + 3
	defs := make([]models.FixtureDefinition, n)
	values := make([]models.FixtureValue, n)
	ids := make([]string, n)
	for i := range defs {
		ids[i] = fmt.Sprintf("id%04d", i)
		defs[i] = models.FixtureDefinition{ID: ids[i], Manufacturer: "M", Model: ids[i]}
		values[i] = models.FixtureValue{ID: "v" + ids[i], LookID: ids[i], FixtureID: "f"}
	}
	if err := db.CreateInBatches(&defs, 200).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.CreateInBatches(&values, 200).Error; err != nil {
		t.Fatal(err)
	}
	got, err := NewFixtureRepository(db).FindDefinitionsByIDs(ctx, ids)
	if err != nil || len(got) != n {
		t.Errorf("definitions = %d, %v", len(got), err)
	}
	counts, err := NewLookRepository(db).CountFixturesByLookIDs(ctx, ids)
	if err != nil || len(counts) != n {
		t.Errorf("counts = %d, %v", len(counts), err)
	}
}

func TestRowCounter(t *testing.T) {
	tdb, cleanup := setupTestDB(t)
	defer cleanup()
	db := tdb.DB
	ctx := context.Background()
	c := NewRowCounter(db)

	if err := db.Create(&[]models.LookBoard{{ID: "b1", ProjectID: "p"}, {ID: "b2", ProjectID: "p"}, {ID: "b3", ProjectID: "q"}}).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&[]models.LookBoardButton{
		{ID: "x1", LookBoardID: "b1"}, {ID: "x2", LookBoardID: "b1"}, {ID: "x3", LookBoardID: "b2"}, {ID: "x4", LookBoardID: "b3"},
	}).Error; err != nil {
		t.Fatal(err)
	}
	if n, err := c.CountWhere(ctx, &models.LookBoardButton{}, "look_board_id", "b1"); err != nil || n != 2 {
		t.Errorf("CountWhere = %d, %v", n, err)
	}
	if n, err := c.CountAll(ctx, &models.LookBoard{}); err != nil || n != 3 {
		t.Errorf("CountAll = %d, %v", n, err)
	}
	if n, err := c.CountProjectLookBoardButtons(ctx, "p"); err != nil || n != 3 {
		t.Errorf("project buttons = %d, %v", n, err)
	}
}

func TestRowCounter_ExactExportCounts(t *testing.T) {
	tdb, cleanup := setupTestDB(t)
	defer cleanup()
	db := tdb.DB
	ctx := context.Background()
	c := NewRowCounter(db)
	mode := func(s string) *string { return &s }

	// Fixture types: (d1, A) ×2, (d1, B), (d2, nil) → 3 distinct; other project ignored.
	if err := db.Create(&[]models.FixtureInstance{
		{ID: "i1", ProjectID: "p", DefinitionID: "d1", ModeName: mode("A")},
		{ID: "i2", ProjectID: "p", DefinitionID: "d1", ModeName: mode("A")},
		{ID: "i3", ProjectID: "p", DefinitionID: "d1", ModeName: mode("B")},
		{ID: "i4", ProjectID: "p", DefinitionID: "d2"},
		{ID: "i5", ProjectID: "q", DefinitionID: "d3"},
	}).Error; err != nil {
		t.Fatal(err)
	}
	if n, err := c.CountProjectFixtureTypes(ctx, "p"); err != nil || n != 3 {
		t.Errorf("fixture types = %d, %v", n, err)
	}

	// Library: d1 has 2 modes, d2 has none → 3 rows.
	if err := db.Create(&[]models.FixtureDefinition{{ID: "d1"}, {ID: "d2"}}).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&[]models.FixtureMode{{ID: "m1", DefinitionID: "d1"}, {ID: "m2", DefinitionID: "d1"}}).Error; err != nil {
		t.Fatal(err)
	}
	if n, err := c.CountLibraryRows(ctx); err != nil || n != 3 {
		t.Errorf("library rows = %d, %v", n, err)
	}

	// Look fixtures: i1 (p), i5 (q) and a dangling reference → 1 in project p.
	if err := db.Create(&[]models.FixtureValue{
		{ID: "v1", LookID: "L", FixtureID: "i1"}, {ID: "v2", LookID: "L", FixtureID: "i5"}, {ID: "v3", LookID: "L", FixtureID: "gone"},
	}).Error; err != nil {
		t.Fatal(err)
	}
	if n, err := c.CountLookFixturesInProject(ctx, "L", "p"); err != nil || n != 1 {
		t.Errorf("look fixtures = %d, %v", n, err)
	}
}

func TestOrphanModesAndProjectFixtureValues(t *testing.T) {
	tdb, cleanup := setupTestDB(t)
	defer cleanup()
	db := tdb.DB
	ctx := context.Background()

	// An orphaned mode (its definition was deleted) is not a library row.
	if err := db.Create(&models.FixtureDefinition{ID: "d1"}).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&[]models.FixtureMode{{ID: "m1", DefinitionID: "d1"}, {ID: "m2", DefinitionID: "deleted"}}).Error; err != nil {
		t.Fatal(err)
	}
	if n, err := NewRowCounter(db).CountLibraryRows(ctx); err != nil || n != 1 {
		t.Errorf("library rows = %d, %v", n, err)
	}
	// The builder's mode query skips the same orphans, so the work matches
	// the estimate.
	modes, err := NewFixtureRepository(db).FindAllModes(ctx)
	if err != nil || len(modes) != 1 || modes[0].ID != "m1" {
		t.Errorf("modes = %+v, %v", modes, err)
	}

	// Only fixture values whose fixture is in the project are loaded, in
	// look order.
	if err := db.Create(&[]models.FixtureInstance{{ID: "a", ProjectID: "p"}, {ID: "b", ProjectID: "p"}, {ID: "x", ProjectID: "q"}}).Error; err != nil {
		t.Fatal(err)
	}
	one, two := 1, 2
	if err := db.Create(&[]models.FixtureValue{
		{ID: "v1", LookID: "L", FixtureID: "b", LookOrder: &two},
		{ID: "v2", LookID: "L", FixtureID: "a", LookOrder: &one},
		{ID: "v3", LookID: "L", FixtureID: "x"},
		{ID: "v4", LookID: "L", FixtureID: "gone"},
	}).Error; err != nil {
		t.Fatal(err)
	}
	vals, err := NewLookRepository(db).GetFixtureValuesInProject(ctx, "L", "p")
	if err != nil || len(vals) != 2 || vals[0].FixtureID != "a" || vals[1].FixtureID != "b" {
		t.Errorf("values = %+v, %v", vals, err)
	}
}

// An ID repeated across chunk boundaries must not duplicate results.
func TestChunkIDs_DeduplicatesAcrossChunks(t *testing.T) {
	ids := make([]string, 0, maxInVars+2)
	ids = append(ids, "dup")
	for i := 0; i < maxInVars; i++ {
		ids = append(ids, fmt.Sprintf("x%d", i))
	}
	ids = append(ids, "dup")
	chunks := chunkIDs(ids)
	total := 0
	for _, c := range chunks {
		total += len(c)
	}
	if total != maxInVars+1 {
		t.Errorf("total ids = %d, want %d", total, maxInVars+1)
	}

	tdb, cleanup := setupTestDB(t)
	defer cleanup()
	if err := tdb.DB.Create(&[]models.InstanceChannel{{ID: "c1", FixtureID: "dup", Offset: 0}, {ID: "c2", FixtureID: "dup", Offset: 1}}).Error; err != nil {
		t.Fatal(err)
	}
	got, err := NewFixtureRepository(tdb.DB).GetInstanceChannelsByFixtureIDs(context.Background(), ids)
	if err != nil || len(got["dup"]) != 2 {
		t.Errorf("channels = %v, %v", got["dup"], err)
	}
}
