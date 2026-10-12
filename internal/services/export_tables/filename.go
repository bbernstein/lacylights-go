package exporttables

import (
	"strings"
	"time"
	"unicode"
)

// maxFilenamePartLength bounds each sanitised filename component (in runes).
const maxFilenamePartLength = 60

// File extensions and MIME types for the supported formats.
const (
	ExtCSV   = "csv"
	ExtXLSX  = "xlsx"
	MimeCSV  = "text/csv"
	MimeXLSX = "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet"
)

// SanitizeFilenamePart reduces s to letters, digits, '.', '_' and '-'.
// Runs of other characters (spaces, slashes, quotes, ...) collapse to a
// single '-', leading/trailing separators are trimmed and the result is
// limited to 60 runes. It may return "".
func SanitizeFilenamePart(s string) string {
	var b strings.Builder
	lastDash := false
	for _, r := range s {
		switch {
		case unicode.IsLetter(r) || unicode.IsDigit(r) || r == '_':
			b.WriteRune(r)
			lastDash = false
		case r == '.' || r == '-':
			// Keep dots/dashes but never two separators in a row, which
			// also prevents ".." path segments.
			if !lastDash {
				b.WriteRune(r)
				lastDash = true
			}
		default:
			if !lastDash {
				b.WriteRune('-')
				lastDash = true
			}
		}
	}
	out := strings.Trim(b.String(), "-.")
	out = truncateRunes(out, maxFilenamePartLength)
	return strings.Trim(out, "-.")
}

// Filename builds `<project>-<table>[-<item>]-YYYY-MM-DD.<ext>`. Each part
// is sanitised; empty parts (e.g. no project for the global fixture
// library, or no item for project-wide tables) are omitted.
func Filename(projectName, table, item string, date time.Time, ext string) string {
	var parts []string
	for _, p := range []string{projectName, table, item} {
		if s := SanitizeFilenamePart(p); s != "" {
			parts = append(parts, s)
		}
	}
	if len(parts) == 0 {
		parts = append(parts, "export")
	}
	parts = append(parts, date.Format("2006-01-02"))
	return strings.Join(parts, "-") + "." + ext
}
