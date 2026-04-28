package exporteos

import "strings"

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
