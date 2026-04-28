package importeos

import (
	"context"
	"errors"
	"fmt"
	"io"

	"github.com/bbernstein/lacylights-go/internal/database/repositories"
)

// ErrNotImplemented is returned by methods that have not been implemented yet.
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
	projectRepo   *repositories.ProjectRepository
	fixtureRepo   *repositories.FixtureRepository
	lookRepo      *repositories.LookRepository
	cueListRepo   *repositories.CueListRepository
	cueRepo       *repositories.CueRepository
	lookBoardRepo *repositories.LookBoardRepository
}

// NewService constructs an empty Service. Use NewServiceWithDeps in production.
func NewService() *Service {
	return &Service{}
}

// NewServiceWithDeps wires the service to repositories.
func NewServiceWithDeps(
	p *repositories.ProjectRepository,
	f *repositories.FixtureRepository,
	l *repositories.LookRepository,
	cl *repositories.CueListRepository,
	c *repositories.CueRepository,
	lb *repositories.LookBoardRepository,
) *Service {
	return &Service{p, f, l, cl, c, lb}
}

// validateDependencies ensures all required repositories are wired.
func (s *Service) validateDependencies() error {
	if s.projectRepo == nil {
		return fmt.Errorf("import_eos: service not wired with project repository")
	}
	if s.fixtureRepo == nil {
		return fmt.Errorf("import_eos: service not wired with fixture repository")
	}
	if s.lookRepo == nil {
		return fmt.Errorf("import_eos: service not wired with look repository")
	}
	if s.cueListRepo == nil {
		return fmt.Errorf("import_eos: service not wired with cue list repository")
	}
	if s.cueRepo == nil {
		return fmt.Errorf("import_eos: service not wired with cue repository")
	}
	if s.lookBoardRepo == nil {
		return fmt.Errorf("import_eos: service not wired with look board repository")
	}
	return nil
}

// Import reads an Eos ASCII showfile from r and applies it to the database.
func (s *Service) Import(ctx context.Context, r io.Reader, opts Options) (*Result, error) {
	if err := s.validateDependencies(); err != nil {
		return nil, err
	}
	show, parseWarn, err := Parse(r)
	if err != nil {
		return nil, err
	}
	collector := parseWarn
	sidecar := ReadSidecar(show.SidecarLines, collector)
	mapper := NewMapper(s.projectRepo, s.fixtureRepo, s.lookRepo, s.cueListRepo, s.cueRepo, s.lookBoardRepo)
	return mapper.Apply(ctx, show, sidecar, opts, collector)
}
