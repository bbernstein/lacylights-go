package exporttables

import (
	"bytes"
	"context"
	"encoding/csv"
	"errors"
	"math"
	"strings"
	"testing"

	"github.com/bbernstein/lacylights-go/internal/database/models"
	"github.com/bbernstein/lacylights-go/internal/database/repositories"
)

// A look in this project may reference a fixture from another project
// (look writes do not validate ownership); the export must skip it.
func TestLookDetailsTable_SkipsFixturesFromOtherProjects(t *testing.T) {
	s := newSeededService(t)
	ctx := context.Background()

	foreign := &models.FixtureInstance{
		Name: "Secret", ProjectID: s.otherProjectID, DefinitionID: s.defCustom,
		ChannelCount: ptr(8), Universe: 9, StartChannel: 100,
	}
	if err := s.db.FixtureRepo.CreateWithChannels(ctx, foreign, nil); err != nil {
		t.Fatal(err)
	}
	look := &models.Look{Name: "Mixed", ProjectID: s.projectID}
	if err := s.db.LookRepo.CreateWithFixtureValues(ctx, look, []models.FixtureValue{
		{FixtureID: s.washA, Channels: `[{"offset":0,"value":1}]`},
		{FixtureID: foreign.ID, Channels: `[{"offset":7,"value":42}]`},
	}); err != nil {
		t.Fatal(err)
	}

	data, err := s.svc.Build(ctx, TableLookDetails, s.projectID, look.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(data.Rows) != 1 || data.Rows[0][0] != "Wash 1" {
		t.Fatalf("rows = %v", data.Rows)
	}
	// The foreign fixture's 8 channels must not widen the grid either.
	if len(data.Columns) != len(lookDetailBaseColumns)+3 {
		t.Errorf("columns = %v", headers(data.Columns))
	}
}

// When a fixture's channel count is unknown, stored values are kept rather
// than silently dropped.
func TestLookDetailsTable_UnknownChannelCountKeepsValues(t *testing.T) {
	s := newSeededService(t)
	ctx := context.Background()

	bare := &models.FixtureInstance{Name: "Bare", ProjectID: s.projectID, DefinitionID: s.defCustom, Universe: 3, StartChannel: 1}
	if err := s.db.FixtureRepo.CreateWithChannels(ctx, bare, nil); err != nil {
		t.Fatal(err)
	}
	look := &models.Look{Name: "Bare look", ProjectID: s.projectID}
	if err := s.db.LookRepo.CreateWithFixtureValues(ctx, look, []models.FixtureValue{
		{FixtureID: bare.ID, Channels: `[{"offset":1,"value":7}]`},
	}); err != nil {
		t.Fatal(err)
	}
	data, err := s.svc.Build(ctx, TableLookDetails, s.projectID, look.ID)
	if err != nil {
		t.Fatal(err)
	}
	base := len(lookDetailBaseColumns)
	if len(data.Columns) != base+2 || data.Rows[0][base] != nil || data.Rows[0][base+1] != 7 {
		t.Errorf("columns = %v rows = %v", headers(data.Columns), data.Rows)
	}
}

func TestBuild_RowCap(t *testing.T) {
	s := newSeededService(t)
	ctx := context.Background()
	s.svc.MaxRows = 2 // the seeded project has 3 fixtures

	if _, err := s.svc.Build(ctx, TableFixtures, s.projectID, ""); !errors.Is(err, ErrTooManyRows) {
		t.Errorf("table err = %v", err)
	}
	if _, _, err := s.svc.BuildWorkbook(ctx, s.projectID, nil); !errors.Is(err, ErrTooManyRows) {
		t.Errorf("workbook err = %v", err)
	}
	if NewService(Repos{}).MaxRows != 0 {
		t.Error("rows must be unlimited by default")
	}
}

func TestBatchedCounts(t *testing.T) {
	s := newSeededService(t)
	ctx := context.Background()

	looks, err := s.svc.Build(ctx, TableLooks, s.projectID, "")
	if err != nil {
		t.Fatal(err)
	}
	got := map[any]any{}
	for _, r := range looks.Rows {
		got[r[0]] = r[2]
	}
	// An empty look has no fixture_values rows, so its count must be 0
	// (not missing).
	if got["Warm"] != 2 || got["Empty"] != 0 {
		t.Errorf("look fixture counts = %v", got)
	}

	lists, err := s.svc.Build(ctx, TableCueLists, s.projectID, "")
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range lists.Rows {
		if _, ok := r[2].(int); !ok {
			t.Errorf("cue count for %v is %T, want int", r[0], r[2])
		}
	}
}

func TestSanitizeSheetName_TrailingApostropheAfterTruncation(t *testing.T) {
	// Rune 31 is an interior apostrophe: truncation would leave it last.
	in := strings.Repeat("a", 30) + "'s sheet"
	got := SanitizeSheetName(in)
	if strings.HasSuffix(got, "'") || got != strings.Repeat("a", 30) {
		t.Errorf("got %q", got)
	}
	// The same applies to de-duplicated names.
	out := UniqueSheetNames([]Sheet{{Name: in}, {Name: in}})
	for _, sh := range out {
		if strings.Contains(sh.Name, "' (") || strings.HasSuffix(sh.Name, "'") {
			t.Errorf("unique name %q", sh.Name)
		}
	}
}

func TestNumericCellTypes(t *testing.T) {
	cases := []struct {
		v    any
		want string
	}{
		{int8(-8), "-8"}, {int16(16), "16"}, {int32(-32), "-32"}, {int64(64), "64"},
		{uint(1), "1"}, {uint8(8), "8"}, {uint16(16), "16"}, {uint32(32), "32"}, {uint64(64), "64"},
		{float32(1.5), "1.5"}, {2.25, "2.25"},
	}
	for _, c := range cases {
		got, ok := formatNumber(c.v)
		if !ok || got != c.want {
			t.Errorf("formatNumber(%T %v) = %q, %v", c.v, c.v, got, ok)
		}
		// Negative numbers are not formula-escaped in CSV.
		if csvCell(c.v) != c.want {
			t.Errorf("csvCell(%T) = %q", c.v, csvCell(c.v))
		}
		// XLSX keeps them numeric (not text).
		if _, isString := xlsxValue(c.v).(string); isString {
			t.Errorf("xlsxValue(%T) became text", c.v)
		}
	}
}

// Every table's COUNT-based estimate must be an upper bound on the rows
// actually built, so the pre-build cap never rejects an export that fits.
func TestEstimateRows_UpperBound(t *testing.T) {
	s := newSeededService(t)
	ctx := context.Background()
	scopes := map[TableID]string{
		TableLookDetails:      s.lookWarm,
		TableCueListDetails:   s.cueList,
		TableLookBoardEntries: "",
	}
	for _, tbl := range s.svc.Tables() {
		if tbl.estimate == nil {
			t.Errorf("%s has no estimate", tbl.ID)
			continue
		}
		pid := s.projectID
		if tbl.Scope == ScopeNone {
			pid = ""
		}
		scope := scopes[tbl.ID]
		data, err := s.svc.Build(ctx, tbl.ID, pid, scope)
		if err != nil {
			t.Fatalf("%s: %v", tbl.ID, err)
		}
		n, err := s.svc.estimateRows(ctx, tbl, pid, scope)
		if err != nil {
			t.Fatal(err)
		}
		if n < len(data.Rows) || (len(data.Rows) > 0 && n == 0) {
			t.Errorf("%s estimate %d < rows %d", tbl.ID, n, len(data.Rows))
		}
	}
	// One board only.
	n, err := s.svc.estimateRows(ctx, lookBoardEntriesTable(), s.projectID, s.boardMain)
	if err != nil || n == 0 {
		t.Errorf("board estimate = %d, %v", n, err)
	}
}

// The cap is enforced from COUNT queries before any rows are loaded.
func TestBuild_RowCapCheckedBeforeBuilding(t *testing.T) {
	s := newSeededService(t)
	ctx := context.Background()
	s.svc.MaxRows = 1
	built := false
	tbl := &Table{
		ID: "PROBE", Scope: ScopeProject,
		build: func(context.Context, *Service, string, string) (*TableData, error) {
			built = true
			return &TableData{}, nil
		},
		estimate: func(context.Context, *repositories.RowCounter, string, string) (int, error) { return 2, nil },
	}
	s.svc.byID[tbl.ID] = tbl
	if _, err := s.svc.Build(ctx, tbl.ID, s.projectID, ""); !errors.Is(err, ErrTooManyRows) {
		t.Errorf("err = %v", err)
	}
	if _, _, err := s.svc.BuildWorkbook(ctx, s.projectID, []TableID{tbl.ID}); !errors.Is(err, ErrTooManyRows) {
		t.Errorf("workbook err = %v", err)
	}
	if built {
		t.Error("table was built despite exceeding the cap")
	}

	// Estimate errors are reported; with no counter, only the post-build
	// check applies.
	tbl.estimate = func(context.Context, *repositories.RowCounter, string, string) (int, error) {
		return 0, errors.New("boom")
	}
	if _, err := s.svc.Build(ctx, tbl.ID, s.projectID, ""); err == nil || !strings.Contains(err.Error(), "boom") {
		t.Errorf("estimate err = %v", err)
	}
	if _, _, err := s.svc.BuildWorkbook(ctx, s.projectID, []TableID{tbl.ID}); err == nil {
		t.Error("expected workbook estimate error")
	}
	s.svc.repos.Counts = nil
	if n, err := s.svc.estimateRows(ctx, tbl, s.projectID, ""); n != 0 || err != nil {
		t.Errorf("no counter = %d, %v", n, err)
	}
}

func TestBuildWorkbook_ExplicitEmptyListRejected(t *testing.T) {
	s := newSeededService(t)
	ctx := context.Background()
	if _, _, err := s.svc.BuildWorkbook(ctx, s.projectID, []TableID{}); !errors.Is(err, ErrNoTables) {
		t.Errorf("empty list err = %v", err)
	}
	sheets, _, err := s.svc.BuildWorkbook(ctx, s.projectID, nil)
	if err != nil || len(sheets) != len(s.svc.WorkbookTables()) {
		t.Errorf("nil list = %d sheets, %v", len(sheets), err)
	}
}

func TestAcquire_BoundsConcurrency(t *testing.T) {
	svc := NewService(Repos{})
	ctx := context.Background()
	var releases []func()
	for i := 0; i < MaxConcurrentExports; i++ {
		r, err := svc.Acquire(ctx)
		if err != nil {
			t.Fatal(err)
		}
		releases = append(releases, r)
	}
	// All slots taken: a request whose context ends gives up.
	cctx, cancel := context.WithCancel(ctx)
	cancel()
	if _, err := svc.Acquire(cctx); !errors.Is(err, context.Canceled) {
		t.Errorf("err = %v", err)
	}
	// Releasing a slot lets the next export in.
	releases[0]()
	r, err := svc.Acquire(ctx)
	if err != nil {
		t.Fatal(err)
	}
	r()
	for _, rel := range releases[1:] {
		rel()
	}
}

// Every builder must emit exactly one value per column in every sheet.
func TestAllTables_RowWidthsMatchColumns(t *testing.T) {
	s := newSeededService(t)
	ctx := context.Background()
	scopes := map[TableID]string{TableLookDetails: s.lookWarm, TableCueListDetails: s.cueList}
	for _, tbl := range s.svc.Tables() {
		pid := s.projectID
		if tbl.Scope == ScopeNone {
			pid = ""
		}
		data, err := s.svc.Build(ctx, tbl.ID, pid, scopes[tbl.ID])
		if err != nil {
			t.Fatalf("%s: %v", tbl.ID, err)
		}
		for _, sh := range data.Sheets(tbl.Label) {
			for i, row := range sh.Rows {
				if len(row) != len(sh.Columns) {
					t.Errorf("%s/%s row %d has %d cells, want %d", tbl.ID, sh.Name, i, len(row), len(sh.Columns))
				}
			}
		}
	}
}

func TestXLSX_LongCellsAndReservedSheetName(t *testing.T) {
	long := strings.Repeat("x", MaxCellLength+10)
	if got, _ := xlsxValue(long).(string); len(got) != MaxCellLength {
		t.Errorf("long cell length = %d", len(got))
	}
	// A whole sheet with an over-long cell still encodes.
	if _, err := EncodeXLSX([]Sheet{{Name: "History", Columns: []Column{{"a", "A"}}, Rows: [][]any{{long}}}}); err != nil {
		t.Errorf("encode: %v", err)
	}
	for _, in := range []string{"History", "history", " HISTORY "} {
		if got := SanitizeSheetName(in); strings.EqualFold(got, "history") {
			t.Errorf("SanitizeSheetName(%q) = %q", in, got)
		}
	}
}

// encoding/csv with UseCRLF drops a bare CR inside a field; WriteCSV must
// keep it as a line break instead of silently joining the text.
func TestWriteCSV_PreservesCarriageReturns(t *testing.T) {
	cols := []Column{{"a", "Notes"}}
	out, err := EncodeCSV(cols, [][]any{{"before\rafter"}, {"one\r\ntwo"}, {"\r=1+1"}})
	if err != nil {
		t.Fatal(err)
	}
	recs, err := csv.NewReader(bytes.NewReader(out[len(utf8BOM):])).ReadAll()
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"Notes", "before\nafter", "one\ntwo", "'\n=1+1"}
	for i, w := range want {
		if recs[i][0] != w {
			t.Errorf("record %d = %q, want %q", i, recs[i][0], w)
		}
	}
}

func TestLookDetailsTable_CellBudget(t *testing.T) {
	s := newSeededService(t)
	ctx := context.Background()
	// The Warm look has 2 fixtures × (6 base + 3 channels) × 2 sheets = 36 cells.
	s.svc.MaxCells = 35
	if _, err := s.svc.Build(ctx, TableLookDetails, s.projectID, s.lookWarm); !errors.Is(err, ErrTooManyRows) {
		t.Errorf("err = %v", err)
	}
	s.svc.MaxCells = 36
	if _, err := s.svc.Build(ctx, TableLookDetails, s.projectID, s.lookWarm); err != nil {
		t.Errorf("at budget: %v", err)
	}
	if NewService(Repos{}).MaxCells != 0 {
		t.Error("cells must be unlimited by default")
	}
}

// Estimates for FIXTURE_TYPES, FIXTURE_LIBRARY and LOOK_DETAILS are exact,
// so a cap equal to the real row count never rejects the export.
func TestEstimateRows_ExactForCollapsedAndFilteredTables(t *testing.T) {
	s := newSeededService(t)
	ctx := context.Background()

	// A dangling and a cross-project fixture in a look are not counted.
	foreign := &models.FixtureInstance{Name: "F", ProjectID: s.otherProjectID, DefinitionID: s.defCustom}
	if err := s.db.FixtureRepo.CreateWithChannels(ctx, foreign, nil); err != nil {
		t.Fatal(err)
	}
	look := &models.Look{Name: "Refs", ProjectID: s.projectID}
	if err := s.db.LookRepo.CreateWithFixtureValues(ctx, look, []models.FixtureValue{
		{FixtureID: s.washA, Channels: `[]`},
		{FixtureID: foreign.ID, Channels: `[]`},
		{FixtureID: "gone", Channels: `[]`},
	}); err != nil {
		t.Fatal(err)
	}

	cases := []struct {
		id    TableID
		pid   string
		scope string
	}{
		{TableFixtureTypes, s.projectID, ""},
		{TableFixtureLibrary, "", ""},
		{TableLookDetails, s.projectID, look.ID},
		{TableLookDetails, s.projectID, s.lookWarm},
	}
	for _, c := range cases {
		tbl, _ := s.svc.Lookup(c.id)
		data, err := s.svc.Build(ctx, c.id, c.pid, c.scope)
		if err != nil {
			t.Fatal(err)
		}
		n, err := s.svc.estimateRows(ctx, tbl, c.pid, c.scope)
		if err != nil {
			t.Fatal(err)
		}
		if n != len(data.Rows) {
			t.Errorf("%s estimate = %d, rows = %d", c.id, n, len(data.Rows))
		}
		// A cap equal to the real row count is accepted.
		s.svc.MaxRows = len(data.Rows)
		if _, err := s.svc.Build(ctx, c.id, c.pid, c.scope); err != nil {
			t.Errorf("%s at cap: %v", c.id, err)
		}
		s.svc.MaxRows = 0
	}
}

// A foreign scope ID gets ErrScopeNotFound even when its table would be
// over the cap: the error must not reveal the foreign item's row count.
func TestBuild_ForeignScopeCheckedBeforeSizing(t *testing.T) {
	s := newSeededService(t)
	ctx := context.Background()
	s.svc.MaxRows = 1
	cases := map[TableID]string{
		TableLookDetails:      s.otherLook,
		TableCueListDetails:   s.otherCueList,
		TableLookBoardEntries: s.otherBoard,
	}
	for id, scope := range cases {
		_, err := s.svc.Build(ctx, id, s.projectID, scope)
		if !errors.Is(err, ErrScopeNotFound) {
			t.Errorf("%s err = %v", id, err)
		}
		if _, err := s.svc.Build(ctx, id, s.projectID, "missing"); !errors.Is(err, ErrScopeNotFound) {
			t.Errorf("%s missing err = %v", id, err)
		}
	}
}

func TestIDsNeedingChannelFallback(t *testing.T) {
	fixtures := []models.FixtureInstance{
		{ID: "a", ChannelCount: ptr(3)},
		{ID: "b"},
		{ID: "c", ChannelCount: ptr(0)},
	}
	got := idsNeedingChannelFallback(fixtures)
	if strings.Join(got, ",") != "b,c" {
		t.Errorf("got %v", got)
	}
}

func TestGridFits(t *testing.T) {
	cases := []struct {
		rows, base, channels, max int
		want                      bool
	}{
		{0, 6, 1 << 62, 10, true},
		{2, 6, 3, 36, true},
		{2, 6, 3, 35, false},
		{1, 6, math.MaxInt - 3, 5_000_000, false}, // would overflow if multiplied
		{math.MaxInt / 2, 6, 10, 5_000_000, false},
		{1, 6, -1, 100, false},
		{1, 6, 1, 0, false},
	}
	for _, c := range cases {
		if got := gridFits(c.rows, c.base, c.channels, c.max); got != c.want {
			t.Errorf("gridFits(%d, %d, %d, %d) = %v", c.rows, c.base, c.channels, c.max, got)
		}
	}
}

// Look names come only from looks referenced by the exported rows and in
// the project.
func TestLookNames_OnlyReferencedProjectLooks(t *testing.T) {
	s := newSeededService(t)
	ctx := context.Background()
	names, err := s.svc.lookNames(ctx, s.projectID, []string{s.lookWarm, s.otherLook, "missing"})
	if err != nil {
		t.Fatal(err)
	}
	if len(names) != 1 || names[s.lookWarm] != "Warm" {
		t.Errorf("names = %v", names)
	}
}

// An already-cancelled request never takes a free slot (select would
// otherwise pick randomly between the ready cases).
func TestAcquire_CancelledContextWithFreeSlot(t *testing.T) {
	svc := NewService(Repos{})
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	for i := 0; i < 50; i++ {
		if rel, err := svc.Acquire(ctx); err == nil {
			rel()
			t.Fatal("acquired a slot with a cancelled context")
		}
	}
}

func TestEncode_ByteCap(t *testing.T) {
	svc := NewService(Repos{})
	if svc.MaxBytes != 0 {
		t.Error("bytes must be unlimited by default")
	}
	cols := []Column{{"a", "Name"}}
	rows := [][]any{{strings.Repeat("x", 100)}}
	svc.MaxBytes = 50
	if _, err := svc.EncodeCSV(cols, rows); !errors.Is(err, ErrTooLarge) {
		t.Errorf("csv err = %v", err)
	}
	if _, err := svc.EncodeXLSX([]Sheet{{Name: "S", Columns: cols, Rows: rows}}); !errors.Is(err, ErrTooLarge) {
		t.Errorf("xlsx err = %v", err)
	}
	svc.MaxBytes = 0 // unlimited
	if _, err := svc.EncodeCSV(cols, rows); err != nil {
		t.Errorf("csv under cap: %v", err)
	}
	if _, err := svc.EncodeXLSX([]Sheet{{Name: "S", Columns: cols, Rows: rows}}); err != nil {
		t.Errorf("xlsx under cap: %v", err)
	}
	// Exactly at the cap is allowed.
	full, _ := EncodeCSV(cols, rows)
	svc.MaxBytes = len(full)
	if _, err := svc.EncodeCSV(cols, rows); err != nil {
		t.Errorf("at cap: %v", err)
	}
}

// With no limits set (the default), exports never run sizing queries, so
// the common path costs nothing extra.
func TestBuild_NoLimitsSkipsSizing(t *testing.T) {
	s := newSeededService(t)
	ctx := context.Background()
	tbl := &Table{
		ID: "PROBE2", Scope: ScopeProject,
		build: func(context.Context, *Service, string, string) (*TableData, error) {
			return &TableData{Rows: [][]any{{1}, {2}, {3}}}, nil
		},
		estimate: func(context.Context, *repositories.RowCounter, string, string) (int, error) {
			t.Error("estimate must not run when MaxRows is unset")
			return 0, nil
		},
	}
	s.svc.byID[tbl.ID] = tbl
	if _, err := s.svc.Build(ctx, tbl.ID, s.projectID, ""); err != nil {
		t.Fatal(err)
	}
	if _, _, err := s.svc.BuildWorkbook(ctx, s.projectID, []TableID{tbl.ID}); err != nil {
		t.Fatal(err)
	}
}

// Channel offsets can have gaps: the export uses the highest offset for
// the end address and the Look Details grid.
func TestSparseChannelOffsets(t *testing.T) {
	s := newSeededService(t)
	ctx := context.Background()
	sparse := &models.FixtureInstance{
		Name: "Sparse", ProjectID: s.projectID, DefinitionID: s.defCustom,
		ChannelCount: ptr(2), Universe: 5, StartChannel: 10,
	}
	if err := s.db.FixtureRepo.CreateWithChannels(ctx, sparse, []models.InstanceChannel{
		{Offset: 0, Name: "Dim", Type: "INTENSITY"},
		{Offset: 5, Name: "Strobe", Type: "STROBE"},
	}); err != nil {
		t.Fatal(err)
	}

	data, err := s.svc.Build(ctx, TableFixtures, s.projectID, "")
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range data.Rows {
		if r[0] == "Sparse" {
			if r[7] != 15 || r[8] != 2 { // end = 10 + 5, count stays 2
				t.Errorf("end = %v, count = %v", r[7], r[8])
			}
		}
	}

	look := &models.Look{Name: "Sparse look", ProjectID: s.projectID}
	if err := s.db.LookRepo.CreateWithFixtureValues(ctx, look, []models.FixtureValue{
		{FixtureID: sparse.ID, Channels: `[{"offset":0,"value":10},{"offset":3,"value":77},{"offset":5,"value":200}]`},
	}); err != nil {
		t.Fatal(err)
	}
	ld, err := s.svc.Build(ctx, TableLookDetails, s.projectID, look.ID)
	if err != nil {
		t.Fatal(err)
	}
	base := len(lookDetailBaseColumns)
	if len(ld.Columns) != base+6 || ld.Rows[0][base+5] != 200 {
		t.Errorf("cols = %v row = %v", headers(ld.Columns), ld.Rows[0])
	}
	if ld.Rows[0][base+3] != nil {
		t.Errorf("value in the channel gap must be blank, got %v", ld.Rows[0][base+3])
	}
	if ld.Extra[0].Rows[0][base+5] != "Strobe" {
		t.Errorf("names row = %v", ld.Extra[0].Rows[0])
	}
}

func TestEncodeXLSX_ColumnLimit(t *testing.T) {
	cols := make([]Column, MaxXLSXColumns+1)
	for i := range cols {
		cols[i] = Column{"c", "C"}
	}
	if _, err := EncodeXLSX([]Sheet{{Name: "Wide", Columns: cols}}); !errors.Is(err, ErrTooLarge) {
		t.Errorf("err = %v", err)
	}
	// CSV has no column limit.
	if _, err := EncodeCSV(cols, nil); err != nil {
		t.Errorf("csv: %v", err)
	}
}
