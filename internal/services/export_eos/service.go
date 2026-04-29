// Package exporteos writes LacyLights project state to ETC Eos ASCII format.
package exporteos

import (
	"context"
	"fmt"
	"io"

	"github.com/bbernstein/lacylights-go/internal/database/repositories"
	importeos "github.com/bbernstein/lacylights-go/internal/services/import_eos"
)

// Result is returned from a successful export.
type Result struct {
	Content        string
	FilenameSuffix string
	Warnings       []importeos.Warning
}

// Service handles Eos ASCII export operations.
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
		return fmt.Errorf("export_eos: service not wired with project repository")
	}
	if s.fixtureRepo == nil {
		return fmt.Errorf("export_eos: service not wired with fixture repository")
	}
	if s.lookRepo == nil {
		return fmt.Errorf("export_eos: service not wired with look repository")
	}
	if s.cueListRepo == nil {
		return fmt.Errorf("export_eos: service not wired with cue list repository")
	}
	if s.cueRepo == nil {
		return fmt.Errorf("export_eos: service not wired with cue repository")
	}
	if s.lookBoardRepo == nil {
		return fmt.Errorf("export_eos: service not wired with look board repository")
	}
	return nil
}

// Export writes the project's state as Eos ASCII to w.
func (s *Service) Export(ctx context.Context, projectID string, w io.Writer) (*Result, error) {
	if err := s.validateDependencies(); err != nil {
		return nil, err
	}

	collector := &importeos.Collector{}
	bundle, err := s.loadBundle(ctx, projectID)
	if err != nil {
		return nil, err
	}

	wr := newWriter(WriterOptions{Title: bundle.ProjectName})
	wr.writeHeader()
	wr.commentBanner("Fixture Parameters")
	wr.writeParamTable(bundle.ParamTypes)
	wr.commentBanner("Fixture Personalities")
	for _, p := range bundle.Personalities {
		wr.writePersonality(p)
	}
	wr.commentBanner("Fixture Patch")
	wr.writePatch(bundle.Patch)
	if len(bundle.Palettes) > 0 {
		wr.commentBanner("Palettes")
		for _, pal := range bundle.Palettes {
			wr.writePalette(pal)
		}
	}
	if len(bundle.CueLists) > 0 {
		wr.commentBanner("Cues")
		for _, cl := range bundle.CueLists {
			wr.writeCueList(cl)
		}
	}
	if len(bundle.Groups) > 0 {
		wr.commentBanner("Groups")
		for _, g := range bundle.Groups {
			wr.writeGroup(g)
		}
	}
	if err := wr.writeSidecar(bundle.Sidecar); err != nil {
		return nil, err
	}

	out := wr.String()
	if _, err := w.Write([]byte(out)); err != nil {
		return nil, err
	}
	return &Result{
		Content:        out,
		FilenameSuffix: ".asc",
		Warnings:       collector.All(),
	}, nil
}
