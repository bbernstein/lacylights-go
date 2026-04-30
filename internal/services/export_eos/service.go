// Package exporteos writes LacyLights project state to ETC Eos ASCII format.
package exporteos

import (
	"context"
	"fmt"

	"github.com/bbernstein/lacylights-go/internal/database/repositories"
	importeos "github.com/bbernstein/lacylights-go/internal/services/import_eos"
)

// Result is returned from a successful export.
type Result struct {
	ProjectName    string
	Content        string
	FilenameSuffix string
	// Warnings collects non-fatal issues encountered during export, e.g.
	// fixtures skipped because they have no DMX address (UNPATCHED_INSTANCE).
	// More codes may be added as the writer learns to flag lossy palette
	// conversions, dropped fixture-group references, etc.
	Warnings []importeos.Warning
}

// Service handles Eos ASCII export operations.
type Service struct {
	projectRepo      *repositories.ProjectRepository
	fixtureRepo      *repositories.FixtureRepository
	fixtureGroupRepo *repositories.FixtureGroupRepository
	lookRepo         *repositories.LookRepository
	cueListRepo      *repositories.CueListRepository
	cueRepo          *repositories.CueRepository
	lookBoardRepo    *repositories.LookBoardRepository
}

// NewServiceWithDeps wires the service to repositories.
func NewServiceWithDeps(
	p *repositories.ProjectRepository,
	f *repositories.FixtureRepository,
	fg *repositories.FixtureGroupRepository,
	l *repositories.LookRepository,
	cl *repositories.CueListRepository,
	c *repositories.CueRepository,
	lb *repositories.LookBoardRepository,
) *Service {
	return &Service{
		projectRepo:      p,
		fixtureRepo:      f,
		fixtureGroupRepo: fg,
		lookRepo:         l,
		cueListRepo:      cl,
		cueRepo:          c,
		lookBoardRepo:    lb,
	}
}

// validateDependencies ensures all required repositories are wired.
func (s *Service) validateDependencies() error {
	if s.projectRepo == nil {
		return fmt.Errorf("export_eos: service not wired with project repository")
	}
	if s.fixtureRepo == nil {
		return fmt.Errorf("export_eos: service not wired with fixture repository")
	}
	if s.fixtureGroupRepo == nil {
		return fmt.Errorf("export_eos: service not wired with fixture group repository")
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

// Export renders the project's state as Eos ASCII and returns the content via
// Result. Callers that need to stream to a writer can simply Write the
// returned content; the entire showfile is held in memory regardless because
// the deterministic-emit guarantee requires the writer to fully assemble the
// document before flushing. Theatre-scale projects fit comfortably (the OTBPA
// reference fixture is ~500 KB); the practical upper bound is well under the
// 50 MiB import cap declared in the GraphQL resolver.
func (s *Service) Export(ctx context.Context, projectID string) (*Result, error) {
	if err := s.validateDependencies(); err != nil {
		return nil, err
	}

	collector := &importeos.Collector{}
	bundle, err := s.loadBundle(ctx, projectID, collector)
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

	return &Result{
		ProjectName:    bundle.ProjectName,
		Content:        wr.String(),
		FilenameSuffix: ".asc",
		Warnings:       collector.All(),
	}, nil
}
