package exporttables

import (
	"bytes"
	"encoding/csv"
	"errors"
	"strings"
	"testing"

	"github.com/xuri/excelize/v2"
)

func TestWriteCSV_BOMQuotingAndNewlines(t *testing.T) {
	cols := []Column{{"a", "Name"}, {"b", "Notes"}, {"c", "Value"}}
	rows := [][]any{
		{"Wash, front", "line1\nline2", 5},
		{`He said "hi"`, nil, 1.25},
		{"plain", true, int64(-3)},
	}
	out, err := EncodeCSV(cols, rows)
	if err != nil {
		t.Fatalf("EncodeCSV: %v", err)
	}
	if !bytes.HasPrefix(out, []byte("\xEF\xBB\xBF")) {
		t.Fatalf("missing UTF-8 BOM: %q", out[:3])
	}
	body := string(out[3:])
	if !strings.Contains(body, "\"Wash, front\"") {
		t.Errorf("comma field not quoted: %q", body)
	}
	if !strings.Contains(body, "\"He said \"\"hi\"\"\"") {
		t.Errorf("quote field not escaped: %q", body)
	}
	if !strings.HasSuffix(body, "\r\n") {
		t.Errorf("expected CRLF record terminator")
	}

	recs, err := csv.NewReader(strings.NewReader(body)).ReadAll()
	if err != nil {
		t.Fatalf("re-read CSV: %v", err)
	}
	want := [][]string{
		{"Name", "Notes", "Value"},
		{"Wash, front", "line1\r\nline2", "5"},
		{`He said "hi"`, "", "1.25"},
		{"plain", "Yes", "-3"},
	}
	if len(recs) != len(want) {
		t.Fatalf("records = %d, want %d", len(recs), len(want))
	}
	for i := range want {
		for j := range want[i] {
			// encoding/csv normalises \r\n inside quoted fields to \n.
			w := strings.ReplaceAll(want[i][j], "\r\n", "\n")
			if recs[i][j] != w {
				t.Errorf("rec[%d][%d] = %q, want %q", i, j, recs[i][j], w)
			}
		}
	}
}

func TestWriteCSV_FormulaInjection(t *testing.T) {
	cols := []Column{{"v", "Value"}}
	cases := []struct {
		in   any
		want string
	}{
		{"=SUM(A1:A2)", "'=SUM(A1:A2)"},
		{"+1", "'+1"},
		{"-1+2", "'-1+2"},
		{"@cmd", "'@cmd"},
		{"\tTab", "'\tTab"},
		{"\rCR", "'\rCR"},
		{"safe = text", "safe = text"},
		{"", ""},
		// Genuine numbers are never escaped.
		{-5, "-5"},
		{int64(-7), "-7"},
		{-2.5, "-2.5"},
		{false, "No"},
	}
	for _, c := range cases {
		got := csvCell(c.in)
		if got != c.want {
			t.Errorf("csvCell(%q) = %q, want %q", c.in, got, c.want)
		}
	}

	// Headers are escaped too.
	out, err := EncodeCSV([]Column{{"x", "=bad"}}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(out), "'=bad") {
		t.Errorf("header not escaped: %q", out)
	}
	_ = cols
}

type failWriter struct{ after int }

func (f *failWriter) Write(p []byte) (int, error) {
	if f.after <= 0 {
		return 0, errors.New("boom")
	}
	f.after--
	return len(p), nil
}

func TestWriteCSV_WriterErrors(t *testing.T) {
	cols := []Column{{"a", "A"}}
	if err := WriteCSV(&failWriter{after: 0}, cols, nil); err == nil {
		t.Error("expected BOM write error")
	}
	if err := WriteCSV(&failWriter{after: 1}, cols, [][]any{{"x"}}); err == nil {
		t.Error("expected flush error")
	}
}

func TestCellText(t *testing.T) {
	if got := cellText(struct{ A int }{1}); got != "{1}" {
		t.Errorf("fallback = %q", got)
	}
	if got := cellText(3); got != "3" {
		t.Errorf("int = %q", got)
	}
}

func TestEncodeXLSX_RoundTrip(t *testing.T) {
	sheets := []Sheet{
		{
			Name:    "Fixtures",
			Columns: []Column{{"n", "Name"}, {"u", "Universe"}, {"f", "Fade"}, {"b", "Loop"}, {"x", "Blank"}},
			Rows: [][]any{
				{"=not a formula", 1, 2.5, true, nil},
				{"Wash, 2", -5, 0.0, false, "x"},
			},
		},
		{
			Name:    "Fixtures", // duplicate -> de-duplicated
			Columns: []Column{{"a", "A"}},
			Rows:    [][]any{{"only"}},
		},
		{
			Name:    "A very long sheet name that exceeds Excel's limit: yes/no?",
			Columns: []Column{{"a", "A"}},
		},
	}
	out, err := EncodeXLSX(sheets)
	if err != nil {
		t.Fatalf("EncodeXLSX: %v", err)
	}
	f, err := excelize.OpenReader(bytes.NewReader(out))
	if err != nil {
		t.Fatalf("open xlsx: %v", err)
	}
	defer func() { _ = f.Close() }()

	names := f.GetSheetList()
	if len(names) != 3 {
		t.Fatalf("sheets = %v", names)
	}
	if names[0] != "Fixtures" || names[1] != "Fixtures (2)" {
		t.Errorf("sheet names = %v", names)
	}
	if n := len([]rune(names[2])); n > MaxSheetNameLength {
		t.Errorf("sheet name too long (%d): %q", n, names[2])
	}
	if strings.ContainsAny(names[2], invalidSheetNameChars) {
		t.Errorf("sheet name has invalid chars: %q", names[2])
	}

	rows, err := f.GetRows("Fixtures")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(rows[0], "|") != "Name|Universe|Fade|Loop|Blank" {
		t.Errorf("header = %v", rows[0])
	}
	if rows[1][0] != "=not a formula" || rows[1][1] != "1" || rows[1][2] != "2.5" || rows[1][3] != "Yes" {
		t.Errorf("row1 = %v", rows[1])
	}
	if rows[2][1] != "-5" || rows[2][2] != "0" || rows[2][3] != "No" || rows[2][4] != "x" {
		t.Errorf("row2 = %v", rows[2])
	}

	// Numbers are typed as numbers, strings as strings, and nothing is a formula.
	typ, _ := f.GetCellType("Fixtures", "B2")
	if typ != excelize.CellTypeNumber && typ != excelize.CellTypeUnset {
		t.Errorf("B2 type = %v, want number", typ)
	}
	if v, _ := f.GetCellValue("Fixtures", "B3"); v != "-5" {
		t.Errorf("B3 = %q", v)
	}
	if typ, _ := f.GetCellType("Fixtures", "A2"); typ == excelize.CellTypeNumber {
		t.Errorf("A2 should be text")
	}
	if formula, _ := f.GetCellFormula("Fixtures", "A2"); formula != "" {
		t.Errorf("A2 formula = %q, want none", formula)
	}
	if v, _ := f.GetCellValue("Fixtures", "E2"); v != "" {
		t.Errorf("nil cell = %q, want blank", v)
	}

	// Bold header and frozen first row.
	styleID, _ := f.GetCellStyle("Fixtures", "A1")
	style, err := f.GetStyle(styleID)
	if err != nil || style.Font == nil || !style.Font.Bold {
		t.Errorf("header not bold: %+v %v", style, err)
	}
	panes, err := f.GetPanes("Fixtures")
	if err != nil || !panes.Freeze || panes.YSplit != 1 {
		t.Errorf("panes = %+v %v", panes, err)
	}
	if w, _ := f.GetColWidth("Fixtures", "A"); w < minColWidth || w > maxColWidth {
		t.Errorf("col width = %v", w)
	}
}

func TestEncodeXLSX_RequiresSheet(t *testing.T) {
	if _, err := EncodeXLSX(nil); err == nil {
		t.Error("expected error for no sheets")
	}
}

func TestColumnWidths(t *testing.T) {
	sh := Sheet{
		Columns: []Column{{"a", "A"}, {"b", "Header"}, {"c", "C"}},
		Rows:    [][]any{{"x", "short", strings.Repeat("y", 200)}, {"x", "y", "z", "extra ignored"}},
	}
	w := columnWidths(sh)
	if w[0] != minColWidth {
		t.Errorf("w0 = %v, want min", w[0])
	}
	if w[1] != float64(len("Header")+2) {
		t.Errorf("w1 = %v", w[1])
	}
	if w[2] != maxColWidth {
		t.Errorf("w2 = %v, want max", w[2])
	}
}

func TestSanitizeAndUniqueSheetNames(t *testing.T) {
	cases := map[string]string{
		"Cue Lists":             "Cue Lists",
		"a/b\\c?d*e[f]g:":       "a b c d e f g",
		"'quoted'":              "quoted",
		"":                      "Sheet",
		"[]":                    "Sheet",
		strings.Repeat("x", 40): strings.Repeat("x", 31),
	}
	for in, want := range cases {
		if got := SanitizeSheetName(in); got != want {
			t.Errorf("SanitizeSheetName(%q) = %q, want %q", in, got, want)
		}
	}

	long := strings.Repeat("L", 31)
	in := []Sheet{{Name: "Looks"}, {Name: "looks"}, {Name: "LOOKS"}, {Name: long}, {Name: long}}
	out := UniqueSheetNames(in)
	want := []string{"Looks", "looks (2)", "LOOKS (3)", long, strings.Repeat("L", 27) + " (2)"}
	for i := range want {
		if out[i].Name != want[i] {
			t.Errorf("name[%d] = %q, want %q", i, out[i].Name, want[i])
		}
		if len([]rune(out[i].Name)) > MaxSheetNameLength {
			t.Errorf("name[%d] too long", i)
		}
	}
	if in[1].Name != "looks" {
		t.Error("input must not be modified")
	}
}
