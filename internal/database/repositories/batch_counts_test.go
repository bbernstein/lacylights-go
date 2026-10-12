package repositories

import (
	"context"
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
