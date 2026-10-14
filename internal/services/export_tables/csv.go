package exporttables

import (
	"bytes"
	"encoding/csv"
	"fmt"
	"io"
	"math"
	"strconv"
	"strings"
)

// utf8BOM makes Excel open the CSV as UTF-8 instead of the system code page.
const utf8BOM = "\uFEFF"

// formulaPrefixes are the leading characters that make spreadsheet apps
// interpret a cell as a formula (CSV/formula injection): OWASP's
// = + - @ TAB CR, plus LF, which some importers also treat as a line
// break before a formula.
const formulaPrefixes = "=+-@\t\r\n"

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
			record = append(record, normalizeNewlines(csvCell(v)))
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
	return encodeCSV(cols, rows, math.MaxInt)
}

// encodeCSV is EncodeCSV with the output capped at maxBytes (ErrTooLarge).
func encodeCSV(cols []Column, rows [][]any, maxBytes int) ([]byte, error) {
	buf := &cappedBuffer{max: maxBytes}
	if err := WriteCSV(buf, cols, rows); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// cappedBuffer is a bytes.Buffer that fails with ErrTooLarge once a write
// would take it past max bytes, so an oversized export stops encoding
// early instead of being built in full and rejected afterwards.
type cappedBuffer struct {
	bytes.Buffer
	max int
}

func (b *cappedBuffer) Write(p []byte) (int, error) {
	if len(p) > b.max-b.Len() {
		return 0, fmt.Errorf("%w: file exceeds this server's export limit of %d bytes", ErrTooLarge, b.max)
	}
	return b.Buffer.Write(p)
}

// csvCell formats a cell value. Only string values are formula-escaped.
// Non-finite floats (NaN, ±Inf) are written as blank cells.
func csvCell(v any) string {
	v = finiteOrNil(v)
	if s, ok := formatNumber(v); ok {
		return s
	}
	return escapeFormula(cellText(v))
}

// normalizeNewlines turns CRLF and lone CR line breaks into LF. With
// UseCRLF, encoding/csv drops every '\r' inside a field and writes each
// '\n' as CRLF, so a lone CR would otherwise be silently deleted.
func normalizeNewlines(s string) string {
	if !strings.ContainsRune(s, '\r') {
		return s
	}
	return strings.ReplaceAll(strings.ReplaceAll(s, "\r\n", "\n"), "\r", "\n")
}

// escapeFormula neutralises a string that a spreadsheet would treat as a
// formula by prefixing it with a single quote (the OWASP mitigation).
//
// This changes the data: a legitimate value such as "-5 dB" or "+1" is
// written as "'-5 dB" / "'+1", so CSV exports are not a lossless round-trip
// format. XLSX output never needs this because strings are stored as text
// cells, never as formulas.
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

// finiteOrNil returns nil for NaN and ±Inf floats (which are not
// meaningful spreadsheet values and, as "+Inf", would bypass formula
// escaping) and v unchanged otherwise.
func finiteOrNil(v any) any {
	var f float64
	switch n := v.(type) {
	case float64:
		f = n
	case float32:
		f = float64(n)
	default:
		return v
	}
	if math.IsNaN(f) || math.IsInf(f, 0) {
		return nil
	}
	return v
}

// formatNumber renders numeric cell values. ok is false for non-numbers.
func formatNumber(v any) (string, bool) {
	switch n := v.(type) {
	case int:
		return strconv.Itoa(n), true
	case int8:
		return strconv.FormatInt(int64(n), 10), true
	case int16:
		return strconv.FormatInt(int64(n), 10), true
	case int32:
		return strconv.FormatInt(int64(n), 10), true
	case int64:
		return strconv.FormatInt(n, 10), true
	case uint:
		return strconv.FormatUint(uint64(n), 10), true
	case uint8:
		return strconv.FormatUint(uint64(n), 10), true
	case uint16:
		return strconv.FormatUint(uint64(n), 10), true
	case uint32:
		return strconv.FormatUint(uint64(n), 10), true
	case uint64:
		return strconv.FormatUint(n, 10), true
	case float32:
		return strconv.FormatFloat(float64(n), 'f', -1, 32), true
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
