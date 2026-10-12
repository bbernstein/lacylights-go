package exporttables

import (
	"context"
	"errors"
	"strings"
	"testing"
)

func headerString(cols []Column) string {
	return strings.Join(headers(cols), "|")
}

func TestFixturesTable(t *testing.T) {
	s := newSeededService(t)
	data, err := s.svc.Build(context.Background(), TableFixtures, s.projectID, "")
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	want := "Name|Manufacturer|Model|Mode|Type|Universe|Start channel|End channel|Channel count|Tags|Groups|Description"
	if got := headerString(data.Columns); got != want {
		t.Errorf("headers = %q", got)
	}
	if len(data.Rows) != 3 {
		t.Fatalf("rows = %d, want 3", len(data.Rows))
	}
	// Ordered by universe, start channel.
	if data.Rows[0][0] != "Wash 1" || data.Rows[1][0] != "Dimmer 1" || data.Rows[2][0] != "Wash 2" {
		t.Errorf("order = %v, %v, %v", data.Rows[0][0], data.Rows[1][0], data.Rows[2][0])
	}

	wash := data.Rows[0]
	wantWash := []any{"Wash 1", "Chauvet", "SlimPAR 56", "3ch", "LED_PAR", 1, 1, 3, 3, "front; wash", "All; Front", "Stage left"}
	for i, v := range wantWash {
		if wash[i] != v {
			t.Errorf("wash[%s] = %#v, want %#v", data.Columns[i].Header, wash[i], v)
		}
	}
	// Dimmer: manufacturer/model/type from definition, channel count from
	// instance channels, no tags/groups/description.
	dim := data.Rows[1]
	wantDim := []any{"Dimmer 1", "Generic", "Dimmer", "1ch", "DIMMER", 1, 10, 10, 1, nil, nil, nil}
	for i, v := range wantDim {
		if dim[i] != v {
			t.Errorf("dimmer[%s] = %#v, want %#v", data.Columns[i].Header, dim[i], v)
		}
	}
	if got := data.Rows[2][colIndex(t, data, "Groups")]; got != "Front" {
		t.Errorf("Wash 2 groups = %v", got)
	}
}

func TestLooksTable(t *testing.T) {
	s := newSeededService(t)
	data, err := s.svc.Build(context.Background(), TableLooks, s.projectID, "")
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if got := headerString(data.Columns); got != "Name|Description|Fixture count|Created (UTC)|Updated (UTC)" {
		t.Errorf("headers = %q", got)
	}
	if len(data.Rows) != 2 {
		t.Fatalf("rows = %d, want 2 (other project's look excluded)", len(data.Rows))
	}
	// Sorted by name.
	if data.Rows[0][0] != "Empty" || data.Rows[1][0] != "Warm" {
		t.Errorf("order = %v, %v", data.Rows[0][0], data.Rows[1][0])
	}
	warm := data.Rows[1]
	if warm[1] != "Warm wash" || warm[2] != 2 {
		t.Errorf("warm = %v", warm)
	}
	if ts, ok := warm[3].(string); !ok || len(ts) != len("2006-01-02 15:04:05") {
		t.Errorf("created = %#v", warm[3])
	}
	if data.Rows[0][1] != nil || data.Rows[0][2] != 0 {
		t.Errorf("empty = %v", data.Rows[0])
	}
}

func TestCueListsTable(t *testing.T) {
	s := newSeededService(t)
	data, err := s.svc.Build(context.Background(), TableCueLists, s.projectID, "")
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if got := headerString(data.Columns); got != "Name|Description|Cue count|Loop|Created (UTC)|Updated (UTC)" {
		t.Errorf("headers = %q", got)
	}
	if len(data.Rows) != 2 {
		t.Fatalf("rows = %d, want 2", len(data.Rows))
	}
	act := rowBy(t, data, "Name", "Act 1")
	if act[1] != "First act" || act[2] != 2 || act[3] != false {
		t.Errorf("act 1 = %v", act)
	}
	pre := rowBy(t, data, "Name", "Preshow")
	if pre[2] != 0 || pre[3] != true {
		t.Errorf("preshow = %v", pre)
	}
}

func TestBuild_Validation(t *testing.T) {
	s := newSeededService(t)
	ctx := context.Background()

	if _, err := s.svc.Build(ctx, "NOPE", s.projectID, ""); !errors.Is(err, ErrUnknownTable) {
		t.Errorf("unknown table err = %v", err)
	}
	if _, err := s.svc.Build(ctx, TableFixtures, "", ""); !errors.Is(err, ErrProjectRequired) {
		t.Errorf("missing project err = %v", err)
	}
	if _, err := s.svc.Build(ctx, TableFixtures, s.projectID, s.lookWarm); !errors.Is(err, ErrScopeNotAllowed) {
		t.Errorf("scope on project table err = %v", err)
	}
}

func TestRegistryMetadata(t *testing.T) {
	s := newSeededService(t)
	seen := map[TableID]bool{}
	for _, tbl := range s.svc.Tables() {
		if seen[tbl.ID] {
			t.Errorf("duplicate table %s", tbl.ID)
		}
		seen[tbl.ID] = true
		if tbl.Label == "" || tbl.Slug == "" || len(tbl.Columns) == 0 || tbl.build == nil {
			t.Errorf("incomplete table %+v", tbl)
		}
		if SanitizeFilenamePart(tbl.Slug) != tbl.Slug {
			t.Errorf("slug %q is not filename-safe", tbl.Slug)
		}
		if got, ok := s.svc.Lookup(tbl.ID); !ok || got != tbl {
			t.Errorf("Lookup(%s) failed", tbl.ID)
		}
		if len(tbl.Headers()) != len(tbl.Columns) {
			t.Errorf("Headers() length mismatch for %s", tbl.ID)
		}
	}
	if _, ok := s.svc.Lookup("NOPE"); ok {
		t.Error("Lookup(NOPE) should fail")
	}
}

func TestScopeHelpers(t *testing.T) {
	cases := []struct {
		tbl                         Table
		requires, accepts, needProj bool
	}{
		{Table{Scope: ScopeNone}, false, false, false},
		{Table{Scope: ScopeProject}, false, false, true},
		{Table{Scope: ScopeLook}, true, true, true},
		{Table{Scope: ScopeCueList}, true, true, true},
		{Table{Scope: ScopeLookBoard, ScopeOptional: true}, false, true, true},
	}
	for _, c := range cases {
		if c.tbl.RequiresScopeID() != c.requires || c.tbl.AcceptsScopeID() != c.accepts || c.tbl.RequiresProject() != c.needProj {
			t.Errorf("scope helpers wrong for %+v", c.tbl)
		}
	}
}

func TestTableDataSheets(t *testing.T) {
	d := &TableData{
		Columns: []Column{{"a", "A"}},
		Rows:    [][]any{{1}},
		Extra:   []Sheet{{Name: "Extra"}},
	}
	sh := d.Sheets("Main")
	if len(sh) != 2 || sh[0].Name != "Main" || sh[1].Name != "Extra" || len(sh[0].Rows) != 1 {
		t.Errorf("sheets = %+v", sh)
	}
}

func TestBuildWorkbook(t *testing.T) {
	s := newSeededService(t)
	ctx := context.Background()

	sheets, total, err := s.svc.BuildWorkbook(ctx, s.projectID, nil)
	if err != nil {
		t.Fatalf("BuildWorkbook: %v", err)
	}
	ids := s.svc.WorkbookTables()
	if len(sheets) != len(ids) {
		t.Fatalf("sheets = %d, want %d", len(sheets), len(ids))
	}
	sum := 0
	for _, sh := range sheets {
		sum += len(sh.Rows)
	}
	if total != sum {
		t.Errorf("total = %d, want %d", total, sum)
	}

	// Explicit list, duplicates collapsed.
	sheets, total, err = s.svc.BuildWorkbook(ctx, s.projectID, []TableID{TableLooks, TableLooks, TableCueLists})
	if err != nil {
		t.Fatalf("BuildWorkbook explicit: %v", err)
	}
	if len(sheets) != 2 || sheets[0].Name != "Looks" || sheets[1].Name != "Cue Lists" || total != 4 {
		t.Errorf("explicit workbook = %d sheets, total %d", len(sheets), total)
	}

	if _, _, err := s.svc.BuildWorkbook(ctx, "", nil); !errors.Is(err, ErrProjectRequired) {
		t.Errorf("no project err = %v", err)
	}
	if _, _, err := s.svc.BuildWorkbook(ctx, s.projectID, []TableID{"NOPE"}); !errors.Is(err, ErrUnknownTable) {
		t.Errorf("unknown err = %v", err)
	}
}
