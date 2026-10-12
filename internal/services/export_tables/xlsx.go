package exporttables

import (
	"bytes"
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/xuri/excelize/v2"
)

// MaxSheetNameLength is Excel's limit on worksheet name length.
const MaxSheetNameLength = 31

// MaxCellLength is the most characters Excel stores in one cell. Longer
// strings make excelize reject the row, so they are truncated.
const MaxCellLength = 32767

// reservedSheetName is a worksheet name Excel reserves (case-insensitive).
const reservedSheetName = "history"

const (
	minColWidth = 8
	maxColWidth = 60
)

// invalidSheetNameChars may not appear in an Excel worksheet name.
const invalidSheetNameChars = `[]:*?/\`

// EncodeXLSX encodes sheets as an .xlsx workbook. Each sheet gets a bold,
// frozen header row and column widths sized to their content. Numeric
// values are stored as numbers, everything else as text (never as
// formulas). Sheet names must already be valid and unique (see
// UniqueSheetNames); EncodeXLSX applies UniqueSheetNames defensively.
func EncodeXLSX(sheets []Sheet) ([]byte, error) {
	if len(sheets) == 0 {
		return nil, fmt.Errorf("xlsx: at least one sheet is required")
	}
	sheets = UniqueSheetNames(sheets)

	f := excelize.NewFile()
	defer func() { _ = f.Close() }()

	bold, err := f.NewStyle(&excelize.Style{Font: &excelize.Font{Bold: true}})
	if err != nil {
		return nil, fmt.Errorf("xlsx: header style: %w", err)
	}

	for i, sh := range sheets {
		if i == 0 {
			if err := f.SetSheetName(f.GetSheetName(0), sh.Name); err != nil {
				return nil, fmt.Errorf("xlsx: rename sheet: %w", err)
			}
		} else if _, err := f.NewSheet(sh.Name); err != nil {
			return nil, fmt.Errorf("xlsx: new sheet %q: %w", sh.Name, err)
		}
		if err := writeSheet(f, sh, bold); err != nil {
			return nil, fmt.Errorf("xlsx: sheet %q: %w", sh.Name, err)
		}
	}
	f.SetActiveSheet(0)

	var buf bytes.Buffer
	if err := f.Write(&buf); err != nil {
		return nil, fmt.Errorf("xlsx: write: %w", err)
	}
	return buf.Bytes(), nil
}

func writeSheet(f *excelize.File, sh Sheet, headerStyle int) error {
	sw, err := f.NewStreamWriter(sh.Name)
	if err != nil {
		return err
	}
	for i, w := range columnWidths(sh) {
		if err := sw.SetColWidth(i+1, i+1, w); err != nil {
			return err
		}
	}
	if err := sw.SetPanes(&excelize.Panes{
		Freeze:      true,
		YSplit:      1,
		TopLeftCell: "A2",
		ActivePane:  "bottomLeft",
	}); err != nil {
		return err
	}

	header := make([]any, len(sh.Columns))
	for i, c := range sh.Columns {
		header[i] = excelize.Cell{StyleID: headerStyle, Value: c.Header}
	}
	if err := sw.SetRow("A1", header); err != nil {
		return err
	}
	for r, row := range sh.Rows {
		cell, err := excelize.CoordinatesToCellName(1, r+2)
		if err != nil {
			return err
		}
		values := make([]any, len(row))
		for i, v := range row {
			values[i] = xlsxValue(v)
		}
		if err := sw.SetRow(cell, values); err != nil {
			return err
		}
	}
	return sw.Flush()
}

// xlsxValue converts a cell value to what excelize should store. Numbers
// stay numeric; booleans become Yes/No text to match the CSV output; nil
// stays nil (an empty cell), as do NaN and ±Inf. Strings longer than
// MaxCellLength are truncated.
func xlsxValue(v any) any {
	switch x := finiteOrNil(v).(type) {
	case string:
		return truncateRunes(x, MaxCellLength)
	case nil, int, int8, int16, int32, int64, uint, uint8, uint16, uint32, uint64, float32, float64:
		return x
	default:
		return cellText(v)
	}
}

// columnWidths approximates a good width for each column from the header
// and the longest value, clamped to a sensible range.
func columnWidths(sh Sheet) []float64 {
	widths := make([]float64, len(sh.Columns))
	for i, c := range sh.Columns {
		widths[i] = float64(utf8.RuneCountInString(c.Header))
	}
	for _, row := range sh.Rows {
		for i, v := range row {
			if i >= len(widths) {
				break
			}
			if n := float64(utf8.RuneCountInString(cellText(v))); n > widths[i] {
				widths[i] = n
			}
		}
	}
	for i, w := range widths {
		w += 2 // padding
		if w < minColWidth {
			w = minColWidth
		}
		if w > maxColWidth {
			w = maxColWidth
		}
		widths[i] = w
	}
	return widths
}

// SanitizeSheetName makes name a valid Excel worksheet name: no
// `[]:*?/\` characters, no leading/trailing apostrophes, at most 31
// characters, not empty and not Excel's reserved name "History".
func SanitizeSheetName(name string) string {
	name = strings.Map(func(r rune) rune {
		if strings.ContainsRune(invalidSheetNameChars, r) || r < 0x20 {
			return ' '
		}
		return r
	}, name)
	name = strings.Trim(strings.TrimSpace(name), "'")
	// Truncation can expose an interior apostrophe or space at the end, so
	// trim again afterwards.
	name = strings.Trim(strings.TrimSpace(truncateRunes(name, MaxSheetNameLength)), "' ")
	if name == "" {
		return "Sheet"
	}
	if strings.EqualFold(name, reservedSheetName) {
		return name + " (1)"
	}
	return name
}

// UniqueSheetNames sanitises every sheet name and de-duplicates them
// (case-insensitively, as Excel does) by appending " (2)", " (3)", ...
// while staying within the 31-character limit. The input is not modified.
func UniqueSheetNames(sheets []Sheet) []Sheet {
	out := make([]Sheet, len(sheets))
	used := make(map[string]bool, len(sheets))
	for i, sh := range sheets {
		base := SanitizeSheetName(sh.Name)
		name := base
		for n := 2; used[strings.ToLower(name)]; n++ {
			suffix := fmt.Sprintf(" (%d)", n)
			name = strings.Trim(strings.TrimSpace(truncateRunes(base, MaxSheetNameLength-len(suffix))), "' ") + suffix
		}
		used[strings.ToLower(name)] = true
		sh.Name = name
		out[i] = sh
	}
	return out
}

func truncateRunes(s string, n int) string {
	if utf8.RuneCountInString(s) <= n {
		return s
	}
	r := []rune(s)
	return string(r[:n])
}
