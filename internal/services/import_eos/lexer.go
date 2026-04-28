package importeos

import (
	"bufio"
	"io"
	"strings"
)

// LineKind classifies a single physical line of an Eos ASCII file.
type LineKind int

const (
	// LineBlank is an empty line or whitespace-only line.
	LineBlank LineKind = iota
	// LineComment is a line starting with `!`.
	LineComment
	// LineDirective is a directive line (any non-blank, non-comment line).
	// Directive names start with `$` (top-level), `$$` (sub-directive), or
	// are bare identifiers like `Cue`, `Up`, `Down`, `Text`, etc.
	LineDirective
	// LineSidecar is a "$$ LACYLIGHTS:..." sidecar comment line.
	LineSidecar
)

// Line is a single tokenized line.
type Line struct {
	Lineno    int // 1-based source line number
	Kind      LineKind
	Indent    int      // count of leading spaces
	Directive string   // first whitespace-separated token (e.g. "$Personality", "Cue", "$$Manuf")
	Fields    []string // remaining whitespace-separated tokens
	Raw       string   // verbatim line content (without trailing newline)
}

// tokenize reads r as Eos ASCII and returns the line tokens.
// It accepts LF or CRLF line endings.
func tokenize(r io.Reader) ([]Line, error) {
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)
	var out []Line
	lineno := 0
	for scanner.Scan() {
		lineno++
		raw := strings.TrimRight(scanner.Text(), "\r")
		trimmed := strings.TrimSpace(raw)
		if trimmed == "" {
			out = append(out, Line{Lineno: lineno, Kind: LineBlank, Raw: raw})
			continue
		}
		if strings.HasPrefix(trimmed, "!") {
			out = append(out, Line{Lineno: lineno, Kind: LineComment, Raw: raw})
			continue
		}
		if strings.HasPrefix(trimmed, "$$ LACYLIGHTS:") {
			out = append(out, Line{Lineno: lineno, Kind: LineSidecar, Raw: raw})
			continue
		}
		indent := 0
		for indent < len(raw) && raw[indent] == ' ' {
			indent++
		}
		fields := strings.Fields(trimmed)
		out = append(out, Line{
			Lineno:    lineno,
			Kind:      LineDirective,
			Indent:    indent,
			Directive: fields[0],
			Fields:    fields[1:],
			Raw:       raw,
		})
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	return out, nil
}
