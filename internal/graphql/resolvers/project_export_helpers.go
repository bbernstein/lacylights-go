package resolvers

import (
	"context"
	"fmt"

	"github.com/bbernstein/lacylights-go/internal/graphql/generated"
)

// Shared implementations of the project export operations. Each is exposed
// twice: as the read-only Query fields (projectExport, projectExportQLC,
// projectExportEos) and as the deprecated Mutation fields (exportProject,
// exportProjectToQLC, exportProjectToEos) kept for older clients.

// exportProjectJSON exports a project as LacyLights JSON.
func (r *Resolver) exportProjectJSON(ctx context.Context, projectID string, options *generated.ExportOptionsInput) (*generated.ExportResult, error) {
	if err := r.ensureProjectAccess(ctx, projectID); err != nil {
		return nil, err
	}
	// Get project first to get name
	project, err := r.ProjectRepo.FindByID(ctx, projectID)
	if err != nil {
		return nil, err
	}
	if project == nil {
		return nil, fmt.Errorf("project not found: %s", projectID)
	}

	// Parse options
	includeFixtures := true
	includeLooks := true
	includeCueLists := true

	if options != nil {
		if options.IncludeFixtures.IsSet() && options.IncludeFixtures.Value() != nil {
			includeFixtures = *options.IncludeFixtures.Value()
		}
		if options.IncludeLooks.IsSet() && options.IncludeLooks.Value() != nil {
			includeLooks = *options.IncludeLooks.Value()
		}
		if options.IncludeCueLists.IsSet() && options.IncludeCueLists.Value() != nil {
			includeCueLists = *options.IncludeCueLists.Value()
		}
	}

	// Export project
	exported, stats, err := r.ExportService.ExportProject(ctx, projectID, includeFixtures, includeLooks, includeCueLists)
	if err != nil {
		return nil, err
	}
	if exported == nil {
		return nil, fmt.Errorf("project not found: %s", projectID)
	}

	// Convert to JSON
	jsonContent, err := exported.ToJSON()
	if err != nil {
		return nil, err
	}

	return &generated.ExportResult{
		ProjectID:   projectID,
		ProjectName: project.Name,
		JSONContent: jsonContent,
		Stats: generated.ExportStats{
			FixtureDefinitionsCount: stats.FixtureDefinitionsCount,
			FixtureInstancesCount:   stats.FixtureInstancesCount,
			LooksCount:              stats.LooksCount,
			CueListsCount:           stats.CueListsCount,
			CuesCount:               stats.CuesCount,
			LookBoardsCount:         stats.LookBoardsCount,
		},
	}, nil
}

// exportProjectQLC exports a project as a QLC+ workspace. QLC+ export is
// not available on this platform; access is still checked first so the
// error does not leak project existence to unauthorized callers.
// TODO: wire fixtureMappings through once a QLC+ exporter exists.
func (r *Resolver) exportProjectQLC(ctx context.Context, projectID string, _ []*generated.FixtureMappingInput) (*generated.QLCExportResult, error) {
	if err := r.ensureProjectAccess(ctx, projectID); err != nil {
		return nil, err
	}
	return nil, fmt.Errorf("QLC+ export not available on this platform")
}

// exportProjectEos exports a project as ETC Eos ASCII.
func (r *Resolver) exportProjectEos(ctx context.Context, projectID string) (*generated.EosExportResult, error) {
	// Check access first, matching exportProjectQLC, so unauthorized callers
	// learn nothing about the server's capabilities.
	if err := r.ensureProjectAccess(ctx, projectID); err != nil {
		return nil, err
	}
	if r.EosExportService == nil {
		return nil, fmt.Errorf("eos export not available on this platform")
	}
	res, err := r.EosExportService.Export(ctx, projectID)
	if err != nil {
		return nil, err
	}
	return &generated.EosExportResult{
		ProjectID:      projectID,
		ProjectName:    res.ProjectName,
		ASCIIContent:   res.Content,
		FilenameSuffix: res.FilenameSuffix,
		Warnings:       toEosWarnings(res.Warnings),
	}, nil
}
