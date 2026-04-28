// Package exporteos writes LacyLights project state to ETC Eos ASCII format.
package exporteos

import (
	"context"
	"errors"
	"io"

	importeos "github.com/bbernstein/lacylights-go/internal/services/import_eos"
)

// ErrNotImplemented is returned until later tasks land the writer.
var ErrNotImplemented = errors.New("export_eos: not implemented")

// Result is returned from a successful export.
type Result struct {
	Content        string
	FilenameSuffix string
	Warnings       []importeos.Warning
}

// Service handles Eos ASCII export operations.
type Service struct{}

// NewService constructs an empty Service. Wiring is added later.
func NewService() *Service {
	return &Service{}
}

// Export writes the project's state as Eos ASCII to w.
// Stub: returns ErrNotImplemented until later tasks land the writer.
func (s *Service) Export(ctx context.Context, projectID string, w io.Writer) (*Result, error) {
	return nil, ErrNotImplemented
}
