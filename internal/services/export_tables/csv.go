package exporttables

import (
	"bytes"
	"encoding/csv"
	"fmt"
	"io"
	"strconv"
)

// utf8BOM makes Excel open the CSV as UTF-8 instead of the system code page.
const utf8BOM = "\uFEFF"

// formulaPrefixes are the leading characters that make spreadsheet apps
// interpret a cell as a formula (CSV/formula injection, OWASP).
const formulaPrefixes = "=+-@\t\r"

// WriteCSV writes a header row and data rows as RFC 4180 CSV with a UTF-8
// BOM. String cells that would be interpreted as formulas are prefixed with
// a single quote; numeric cells (including negative numbers) are written
// as-is.
func WriteCSV(w io.Writer, cols []Column, rows [][]any) error {
	if _, err := io.WriteString(w, utf8BOM); err != nil {
		return err
	}
	cw := csv.NewWriter(w)
	cw.UseCRLF = true

	record := make([]string, len(cols))
	for i, c := range cols {
		record[i] = escapeFormula(c.Header)
	}
	if err := cw.Write(record); err != nil {
		return err
	}
	for _, row := range rows {
		record = record[:0]
		for _, v := range row {
			record = append(record, csvCell(v))
		}
		if err := cw.Write(record); err != nil {
			return err
		}
	}
	cw.Flush()
	return cw.Error()
}

// EncodeCSV returns the CSV encoding of a table's primary sheet.
func EncodeCSV(cols []Column, rows [][]any) ([]byte, error) {
	var buf bytes.Buffer
	if err := WriteCSV(&buf, cols, rows); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// csvCell formats a cell value. Only string values are formula-escaped.
func csvCell(v any) string {
	if s, ok := formatNumber(v); ok {
		return s
	}
	return escapeFormula(cellText(v))
}

// escapeFormula neutralises a string that a spreadsheet would treat as a
// formula by prefixing it with a single quote.
func escapeFormula(s string) string {
	if s == "" {
		return s
	}
	for i := 0; i < len(formulaPrefixes); i++ {
		if s[0] == formulaPrefixes[i] {
			return "'" + s
		}
	}
	return s
}

// formatNumber renders numeric cell values. ok is false for non-numbers.
func formatNumber(v any) (string, bool) {
	switch n := v.(type) {
	case int:
		return strconv.Itoa(n), true
	case int64:
		return strconv.FormatInt(n, 10), true
	case float64:
		return strconv.FormatFloat(n, 'f', -1, 64), true
	default:
		return "", false
	}
}

// cellText renders a non-numeric cell value as text.
func cellText(v any) string {
	switch x := v.(type) {
	case nil:
		return ""
	case string:
		return x
	case bool:
		if x {
			return "Yes"
		}
		return "No"
	default:
		if s, ok := formatNumber(v); ok {
			return s
		}
		return fmt.Sprint(v)
	}
}
