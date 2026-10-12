package exporttables

import (
	"strings"
	"testing"
	"time"
)

func TestSanitizeFilenamePart(t *testing.T) {
	cases := map[string]string{
		"My Show":                  "My-Show",
		"  spaced  out  ":          "spaced-out",
		"../../etc/passwd":         "etc-passwd",
		"a..b":                     "a.b",
		`Bad:/\*?"<>|chars`:        "Bad-chars",
		"Café Théâtre":             "Café-Théâtre",
		"under_score-dash":         "under_score-dash",
		"---":                      "",
		"":                         "",
		"Act 1 / Scene 2 (final)!": "Act-1-Scene-2-final",
	}
	for in, want := range cases {
		if got := SanitizeFilenamePart(in); got != want {
			t.Errorf("SanitizeFilenamePart(%q) = %q, want %q", in, got, want)
		}
	}
	long := SanitizeFilenamePart(strings.Repeat("abc ", 40))
	if n := len([]rune(long)); n > maxFilenamePartLength {
		t.Errorf("part length = %d, want <= %d", n, maxFilenamePartLength)
	}
	if strings.HasSuffix(long, "-") {
		t.Errorf("truncated part ends with separator: %q", long)
	}
}

func TestFilename(t *testing.T) {
	d := time.Date(2026, 10, 12, 23, 0, 0, 0, time.UTC)
	cases := []struct {
		project, table, item, ext, want string
	}{
		{"My Show", "fixtures", "", ExtCSV, "My-Show-fixtures-2026-10-12.csv"},
		{"My Show", "look-details", "Warm / Cool", ExtXLSX, "My-Show-look-details-Warm-Cool-2026-10-12.xlsx"},
		{"", "fixture-library", "", ExtCSV, "fixture-library-2026-10-12.csv"},
		{"///", "", "", ExtCSV, "export-2026-10-12.csv"},
	}
	for _, c := range cases {
		if got := Filename(c.project, c.table, c.item, d, c.ext); got != c.want {
			t.Errorf("Filename(%q,%q,%q) = %q, want %q", c.project, c.table, c.item, got, c.want)
		}
	}
}
