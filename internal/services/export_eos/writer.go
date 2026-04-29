package exporteos

import "strings"

// asciiReplacer is built once and reused. strings.Replacer is safe for
// concurrent use, and a single instance avoids allocating a fresh one on
// every label emission.
var asciiReplacer = strings.NewReplacer("\r", " ", "\n", " ", "\t", " ")

// sanitizeASCII strips characters that would corrupt the EOS ASCII output:
// CR/LF would inject phantom directive lines, and tabs misalign the column
// layout some EOS consoles still rely on. Replaces each problem rune with a
// single space, preserving overall length-preserving behavior for any
// downstream offset assumptions.
func sanitizeASCII(s string) string {
	if !strings.ContainsAny(s, "\r\n\t") {
		return s
	}
	return asciiReplacer.Replace(s)
}

// WriterOptions configures the exporter.
type WriterOptions struct {
	Title         string
	FormatVersion string // default "3.20"
}

// writer is the in-memory buffer with helpers.
type writer struct {
	buf  strings.Builder
	opts WriterOptions
}

func newWriter(opts WriterOptions) *writer {
	if opts.FormatVersion == "" {
		opts.FormatVersion = "3.20"
	}
	return &writer{opts: opts}
}

func (w *writer) line(s string)  { w.buf.WriteString(s); w.buf.WriteByte('\n') }
func (w *writer) blank()         { w.buf.WriteByte('\n') }
func (w *writer) String() string { return w.buf.String() }

func (w *writer) commentBanner(s string) {
	w.line("!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!")
	w.line("! " + s)
}
