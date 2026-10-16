package exporttables

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/xuri/excelize/v2"

	"github.com/bbernstein/lacylights-go/internal/database/models"
)

func TestFixtureTypesTable(t *testing.T) {
	s := newSeededService(t)
	data, err := s.svc.Build(context.Background(), TableFixtureTypes, s.projectID, "")
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if got := headerString(data.Columns); got != "Manufacturer|Model|Mode|Channel count|Type|Source|Fixtures in project using it" {
		t.Errorf("headers = %q", got)
	}
	// Only definitions used in the project (not "Acme Builtin Spot"),
	// sorted by manufacturer.
	want := [][]any{
		{"Chauvet", "SlimPAR 56", "3ch", 3, "LED_PAR", "OFL", 2},
		{"Generic", "Dimmer", "1ch", 1, "DIMMER", "Custom", 1},
	}
	if len(data.Rows) != len(want) {
		t.Fatalf("rows = %v", data.Rows)
	}
	for i := range want {
		for j := range want[i] {
			if data.Rows[i][j] != want[i][j] {
				t.Errorf("row %d %s = %#v, want %#v", i, data.Columns[j].Header, data.Rows[i][j], want[i][j])
			}
		}
	}
}

func TestFixtureTypesTable_DistinctModesAndMissingDefinition(t *testing.T) {
	s := newSeededService(t)
	ctx := context.Background()
	// Same definition, different mode -> separate row; fixture without a
	// mode -> blank mode; fixture whose definition is gone -> blank source.
	must := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
	}
	must(s.db.FixtureRepo.Create(ctx, &models.FixtureInstance{
		Name: "Wash 7", ProjectID: s.projectID, DefinitionID: s.defOFL, ModeName: ptr("7ch"), ChannelCount: ptr(7), Universe: 3, StartChannel: 1,
	}))
	must(s.db.FixtureRepo.Create(ctx, &models.FixtureInstance{
		Name: "Ghost", ProjectID: s.projectID, DefinitionID: "missing-def", Manufacturer: ptr("Zed"), Model: ptr("Ghost"), Universe: 4, StartChannel: 1,
	}))
	data, err := s.svc.Build(ctx, TableFixtureTypes, s.projectID, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(data.Rows) != 4 {
		t.Fatalf("rows = %v", data.Rows)
	}
	if data.Rows[1][2] != "7ch" || data.Rows[1][3] != 7 || data.Rows[1][6] != 1 {
		t.Errorf("7ch row = %v", data.Rows[1])
	}
	ghost := data.Rows[3]
	if ghost[0] != "Zed" || ghost[2] != nil || ghost[5] != nil || ghost[3] != 0 {
		t.Errorf("ghost row = %v", ghost)
	}
}

func TestFixtureLibraryTable(t *testing.T) {
	s := newSeededService(t)
	// Library is global: no project needed.
	data, err := s.svc.Build(context.Background(), TableFixtureLibrary, "", "")
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if got := headerString(data.Columns); got != "Manufacturer|Model|Mode|Channel count|Type|Source" {
		t.Errorf("headers = %q", got)
	}
	want := [][]any{
		{"Acme", "Builtin Spot", nil, 2, "MOVING_HEAD", "Built-in"},
		{"Chauvet", "SlimPAR 56", "3ch", 3, "LED_PAR", "OFL"},
		{"Chauvet", "SlimPAR 56", "7ch", 7, "LED_PAR", "OFL"},
		{"Generic", "Dimmer", nil, 1, "DIMMER", "Custom"},
	}
	if len(data.Rows) != len(want) {
		t.Fatalf("rows = %v", data.Rows)
	}
	for i := range want {
		for j := range want[i] {
			if data.Rows[i][j] != want[i][j] {
				t.Errorf("row %d %s = %#v, want %#v", i, data.Columns[j].Header, data.Rows[i][j], want[i][j])
			}
		}
	}
	// A project ID is accepted (and ignored) too.
	if _, err := s.svc.Build(context.Background(), TableFixtureLibrary, s.projectID, ""); err != nil {
		t.Errorf("with project: %v", err)
	}
}

func TestDefinitionSource(t *testing.T) {
	if definitionSource(nil) != "" {
		t.Error("nil def")
	}
	hash := "h"
	cases := []struct {
		def  models.FixtureDefinition
		want string
	}{
		{models.FixtureDefinition{Source: models.FixtureSourceGDTF}, "GDTF"},
		{models.FixtureDefinition{Source: models.FixtureSourceOFL}, "OFL"},
		{models.FixtureDefinition{Source: models.FixtureSourceBuiltIn}, "Built-in"},
		{models.FixtureDefinition{Source: models.FixtureSourceSynthesized}, "Synthesized"},
		{models.FixtureDefinition{Source: models.FixtureSourceEOS}, "Eos"},
		{models.FixtureDefinition{Source: models.FixtureSourceUser}, "Custom"},
		// Rows not yet backfilled still classify by the legacy fields.
		{models.FixtureDefinition{Source: models.FixtureSourceUser, OFLSourceHash: &hash}, "OFL"},
		{models.FixtureDefinition{Source: models.FixtureSourceUser, IsBuiltIn: true}, "Built-in"},
	}
	for _, c := range cases {
		def := c.def
		if got := definitionSource(&def); got != c.want {
			t.Errorf("definitionSource(%+v) = %q, want %q", c.def.Source, got, c.want)
		}
	}
}

func TestLookDetailsTable(t *testing.T) {
	s := newSeededService(t)
	ctx := context.Background()
	data, err := s.svc.Build(ctx, TableLookDetails, s.projectID, s.lookWarm)
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	// N = 3 (Wash 1 has 3 channels, Dimmer 1 has 1).
	if got := headerString(data.Columns); got != "Fixture|Manufacturer|Model|Mode|Universe|Start address|Ch 1|Ch 2|Ch 3" {
		t.Errorf("headers = %q", got)
	}
	if data.ItemName != "Warm" {
		t.Errorf("item = %q", data.ItemName)
	}
	want := [][]any{
		// Explicit 0 stays 0; channel 3 not stored in the look -> blank.
		{"Wash 1", "Chauvet", "SlimPAR 56", "3ch", 1, 1, 255, 0, nil},
		// Dimmer has 1 channel: Ch 2/Ch 3 are blank, not 0.
		{"Dimmer 1", "Generic", "Dimmer", "1ch", 1, 10, 128, nil, nil},
	}
	if len(data.Rows) != len(want) {
		t.Fatalf("rows = %v", data.Rows)
	}
	for i := range want {
		for j := range want[i] {
			if data.Rows[i][j] != want[i][j] {
				t.Errorf("row %d %s = %#v, want %#v", i, data.Columns[j].Header, data.Rows[i][j], want[i][j])
			}
		}
	}

	if len(data.Extra) != 1 || data.Extra[0].Name != ChannelNamesSheet {
		t.Fatalf("extra sheets = %+v", data.Extra)
	}
	names := data.Extra[0].Rows
	if names[0][6] != "Red" || names[0][7] != "Green" || names[0][8] != "Blue" {
		t.Errorf("wash names = %v", names[0])
	}
	if names[1][6] != "Intensity" || names[1][7] != nil || names[1][8] != nil {
		t.Errorf("dimmer names = %v", names[1])
	}

	// CSV has only the values; blank and zero are distinguishable.
	csvOut, err := EncodeCSV(data.Columns, data.Rows)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(csvOut), "Wash 1,Chauvet,SlimPAR 56,3ch,1,1,255,0,\r\n") {
		t.Errorf("csv = %q", csvOut)
	}
	if !strings.Contains(string(csvOut), "Dimmer 1,Generic,Dimmer,1ch,1,10,128,,\r\n") {
		t.Errorf("csv = %q", csvOut)
	}

	// XLSX has both sheets.
	xl, err := EncodeXLSX(data.Sheets("Look Details"))
	if err != nil {
		t.Fatal(err)
	}
	f, err := excelize.OpenReader(bytes.NewReader(xl))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = f.Close() }()
	if got := strings.Join(f.GetSheetList(), "|"); got != "Look Details|Channel names" {
		t.Errorf("sheets = %q", got)
	}
	if v, _ := f.GetCellValue("Look Details", "H2"); v != "0" {
		t.Errorf("H2 = %q, want 0", v)
	}
	if v, _ := f.GetCellValue("Look Details", "H3"); v != "" {
		t.Errorf("H3 = %q, want blank", v)
	}
	if v, _ := f.GetCellValue(ChannelNamesSheet, "G3"); v != "Intensity" {
		t.Errorf("names G3 = %q", v)
	}
}

func TestLookDetailsTable_EmptyLookAndEdgeCases(t *testing.T) {
	s := newSeededService(t)
	ctx := context.Background()
	data, err := s.svc.Build(ctx, TableLookDetails, s.projectID, s.lookEmpty)
	if err != nil {
		t.Fatal(err)
	}
	if len(data.Rows) != 0 || len(data.Columns) != len(lookDetailBaseColumns) {
		t.Errorf("empty look = %d rows, %d cols", len(data.Rows), len(data.Columns))
	}

	// A value beyond the fixture's channel count is ignored (N stays the
	// largest real channel count); a negative offset and a dangling
	// fixture value are skipped; unordered values sort after ordered ones
	// by address.
	look := &models.Look{Name: "Odd", ProjectID: s.projectID}
	if err := s.db.LookRepo.CreateWithFixtureValues(ctx, look, []models.FixtureValue{
		{FixtureID: s.washB, Channels: `[{"offset":4,"value":9},{"offset":-1,"value":1}]`},
		{FixtureID: s.dimmer, Channels: ``},
		{FixtureID: "gone", Channels: `[]`},
		{FixtureID: s.washA, Channels: `[]`, LookOrder: ptr(5)},
	}); err != nil {
		t.Fatal(err)
	}
	data, err = s.svc.Build(ctx, TableLookDetails, s.projectID, look.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(data.Columns) != len(lookDetailBaseColumns)+3 {
		t.Errorf("columns = %v", headers(data.Columns))
	}
	if len(data.Rows) != 3 {
		t.Fatalf("rows = %v", data.Rows)
	}
	order := []any{data.Rows[0][0], data.Rows[1][0], data.Rows[2][0]}
	if order[0] != "Wash 1" || order[1] != "Dimmer 1" || order[2] != "Wash 2" {
		t.Errorf("order = %v", order)
	}
	for _, v := range data.Rows[2][len(lookDetailBaseColumns):] {
		if v != nil {
			t.Errorf("wash 2 should have no values, got %v", data.Rows[2])
		}
	}

	// Invalid JSON is reported.
	bad := &models.Look{Name: "Bad", ProjectID: s.projectID}
	if err := s.db.LookRepo.CreateWithFixtureValues(ctx, bad, []models.FixtureValue{
		{FixtureID: s.washA, Channels: `{not json`},
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.svc.Build(ctx, TableLookDetails, s.projectID, bad.ID); err == nil {
		t.Error("expected invalid JSON error")
	}
}

func TestEffectsTable(t *testing.T) {
	s := newSeededService(t)
	data, err := s.svc.Build(context.Background(), TableEffects, s.projectID, "")
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	wantHeaders := "Name|Description|Type|Waveform|Frequency (Hz)|Amplitude (%)|Offset (%)|Phase offset (deg)|Easing|Priority band|Priority|Composition|On cue change|Fade duration (s)|Master value|Fixture count"
	if got := headerString(data.Columns); got != wantHeaders {
		t.Errorf("headers = %q", got)
	}
	if len(data.Rows) != 1 {
		t.Fatalf("rows = %v", data.Rows)
	}
	want := []any{"Pulse", nil, "WAVEFORM", "SINE", 2.0, 50.0, 50.0, 90.0, nil, "USER", 50, "OVERRIDE", "FADE_OUT", nil, nil, 2}
	for i, v := range want {
		if data.Rows[0][i] != v {
			t.Errorf("%s = %#v, want %#v", data.Columns[i].Header, data.Rows[0][i], v)
		}
	}
}

func TestLookBoardEntriesTable(t *testing.T) {
	s := newSeededService(t)
	ctx := context.Background()

	all, err := s.svc.Build(ctx, TableLookBoardEntries, s.projectID, "")
	if err != nil {
		t.Fatalf("Build all: %v", err)
	}
	if got := headerString(all.Columns); got != "Board|Button label|Look|X|Y|Width|Height|Color|Board fade time (s)" {
		t.Errorf("headers = %q", got)
	}
	if len(all.Rows) != 3 || all.ItemName != "" {
		t.Fatalf("all rows = %v item %q", all.Rows, all.ItemName)
	}
	// Boards sorted by name: Alt first.
	if all.Rows[0][0] != "Alt" || all.Rows[1][0] != "Main" {
		t.Errorf("board order = %v, %v", all.Rows[0][0], all.Rows[1][0])
	}

	one, err := s.svc.Build(ctx, TableLookBoardEntries, s.projectID, s.boardMain)
	if err != nil {
		t.Fatalf("Build one: %v", err)
	}
	if one.ItemName != "Main" || len(one.Rows) != 2 {
		t.Fatalf("one = %q %v", one.ItemName, one.Rows)
	}
	want := [][]any{
		{"Main", "Warm wash", "Warm", 0, 0, 200, 120, "#ff0000", 2.5},
		{"Main", nil, "Empty", 220, 0, 200, 120, nil, 2.5},
	}
	for i := range want {
		for j := range want[i] {
			if one.Rows[i][j] != want[i][j] {
				t.Errorf("row %d %s = %#v, want %#v", i, one.Columns[j].Header, one.Rows[i][j], want[i][j])
			}
		}
	}
}

func TestCueListDetailsTable(t *testing.T) {
	s := newSeededService(t)
	data, err := s.svc.Build(context.Background(), TableCueListDetails, s.projectID, s.cueList)
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if got := headerString(data.Columns); got != "Cue #|Name|Look|Fade in (s)|Fade out (s)|Follow time (s)|Easing|Skip|Notes" {
		t.Errorf("headers = %q", got)
	}
	if data.ItemName != "Act 1" {
		t.Errorf("item = %q", data.ItemName)
	}
	want := [][]any{
		{1.0, "Open", "Warm", 3.0, 2.0, nil, "EASE_IN_OUT", false, "=SUM(A1)"},
		{2.5, "Blackout", "Empty", 0.0, 5.0, 1.5, nil, true, nil},
	}
	if len(data.Rows) != len(want) {
		t.Fatalf("rows = %v", data.Rows)
	}
	for i := range want {
		for j := range want[i] {
			if data.Rows[i][j] != want[i][j] {
				t.Errorf("row %d %s = %#v, want %#v", i, data.Columns[j].Header, data.Rows[i][j], want[i][j])
			}
		}
	}
	// The formula-looking note is neutralised in CSV.
	out, err := EncodeCSV(data.Columns, data.Rows)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(out), ",'=SUM(A1)\r\n") || !strings.Contains(string(out), "2.5,Blackout,Empty,0,5,1.5,,Yes,\r\n") {
		t.Errorf("csv = %q", out)
	}
}

func TestScopedTables_ScopeValidation(t *testing.T) {
	s := newSeededService(t)
	ctx := context.Background()
	cases := []struct {
		table TableID
		other string
	}{
		{TableLookDetails, s.otherLook},
		{TableCueListDetails, s.otherCueList},
		{TableLookBoardEntries, s.otherBoard},
	}
	for _, c := range cases {
		// Scope item from another project.
		if _, err := s.svc.Build(ctx, c.table, s.projectID, c.other); !errors.Is(err, ErrScopeNotFound) {
			t.Errorf("%s other-project scope err = %v", c.table, err)
		}
		// Non-existent scope item.
		if _, err := s.svc.Build(ctx, c.table, s.projectID, "missing"); !errors.Is(err, ErrScopeNotFound) {
			t.Errorf("%s missing scope err = %v", c.table, err)
		}
	}
	for _, id := range []TableID{TableLookDetails, TableCueListDetails} {
		if _, err := s.svc.Build(ctx, id, s.projectID, ""); !errors.Is(err, ErrScopeRequired) {
			t.Errorf("%s without scope err = %v", id, err)
		}
	}
	if _, err := s.svc.Build(ctx, TableFixtureLibrary, "", s.lookWarm); !errors.Is(err, ErrScopeNotAllowed) {
		t.Errorf("library with scope err = %v", err)
	}
}

func TestBuildWorkbook_AllTables(t *testing.T) {
	s := newSeededService(t)
	ctx := context.Background()
	ids := s.svc.WorkbookTables()
	want := []TableID{TableFixtureTypes, TableFixtures, TableLooks, TableEffects, TableLookBoardEntries, TableCueLists}
	if len(ids) != len(want) {
		t.Fatalf("workbook tables = %v", ids)
	}
	for i := range want {
		if ids[i] != want[i] {
			t.Errorf("workbook table %d = %s, want %s", i, ids[i], want[i])
		}
	}
	sheets, total, err := s.svc.BuildWorkbook(ctx, s.projectID, nil)
	if err != nil {
		t.Fatal(err)
	}
	// 2 fixture types + 3 fixtures + 2 looks + 1 effect + 3 buttons + 2 cue lists.
	if total != 13 {
		t.Errorf("total rows = %d, want 13", total)
	}
	xl, err := EncodeXLSX(sheets)
	if err != nil {
		t.Fatal(err)
	}
	f, err := excelize.OpenReader(bytes.NewReader(xl))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = f.Close() }()
	if got := strings.Join(f.GetSheetList(), "|"); got != "Fixture Types|Fixtures|Looks|Effects|Look Board Entries|Cue Lists" {
		t.Errorf("sheets = %q", got)
	}

	// Workbooks hold project tables only: neither the global library nor
	// scoped tables may be requested.
	if _, _, err := s.svc.BuildWorkbook(ctx, s.projectID, []TableID{TableFixtureLibrary}); !errors.Is(err, ErrNotWorkbookTable) {
		t.Errorf("library in workbook err = %v", err)
	}
	if _, _, err := s.svc.BuildWorkbook(ctx, s.projectID, []TableID{TableLookDetails}); !errors.Is(err, ErrScopeRequired) {
		t.Errorf("look details in workbook err = %v", err)
	}
}
