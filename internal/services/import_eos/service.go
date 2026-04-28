package importeos

import (
	"context"
	"errors"
	"io"
)

// ErrNotImplemented is returned by methods that have not been implemented yet.
// It will be replaced as later tasks add functionality.
var ErrNotImplemented = errors.New("import_eos: not implemented")

// Result is returned from a successful import.
type Result struct {
	ProjectID                string
	FixtureDefinitionsCount  int
	FixtureInstancesCount    int
	LooksCount               int
	CueListsCount            int
	CuesCount                int
	GroupsCount              int
	Warnings                 []Warning
	SynthesizedDefinitionIDs []string
}

// Options configures an import run.
type Options struct {
	TargetProjectID *string
	NewProjectName  *string
	GroupID         *string
}

// Service handles Eos ASCII import operations.
type Service struct {
	// Repos and dependencies will be injected in Task 12 when the mapper lands.
}

// NewService constructs an empty Service. Wiring is added in Task 12.
func NewService() *Service {
	return &Service{}
}

// Import reads an Eos ASCII showfile from r and applies it to the database.
// Stub: returns ErrNotImplemented until later tasks land the parser and mapper.
func (s *Service) Import(ctx context.Context, r io.Reader, opts Options) (*Result, error) {
	return nil, ErrNotImplemented
}
