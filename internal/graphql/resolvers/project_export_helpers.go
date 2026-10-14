package resolvers

import (
	"context"
	"fmt"
	"sync/atomic"

	"github.com/bbernstein/lacylights-go/internal/graphql/generated"
	exporttables "github.com/bbernstein/lacylights-go/internal/services/export_tables"
)

// Shared implementations of the project export operations. Each is exposed
// twice: as the read-only Query fields (projectExport, projectExportQLC,
// projectExportEos) and as the deprecated Mutation fields (exportProject,
// exportProjectToQLC, exportProjectToEos) kept for older clients.

// MaxConcurrentProjectExports is how many whole-project exports (JSON or
// Eos) may run at once. As Queries, aliased root fields are resolved
// concurrently by gqlgen (Mutation root fields are serialized), so without
// a bound one request could start many full-project scans in parallel.
// This bounds concurrency only; it is not a size limit.
const MaxConcurrentProjectExports = 2

// projectExportSlots is shared by every Resolver in the process, so the
// bound holds across requests as well as across aliases in one request.
// It is an atomic pointer so the bound can be configured at startup
// (setMaxConcurrentProjectExports); an export releases into the same
// channel it acquired from.
var projectExportSlots atomic.Pointer[chan struct{}]

func init() {
	setMaxConcurrentProjectExports(0)
}

// setMaxConcurrentProjectExports sets how many project exports may run at
// once. n <= 0 restores the default, MaxConcurrentProjectExports. Call it at
// startup, before exports run; exports already holding a slot keep it.
func setMaxConcurrentProjectExports(n int) {
	if n <= 0 {
		n = MaxConcurrentProjectExports
	}
	slots := make(chan struct{}, n)
	projectExportSlots.Store(&slots)
}

// maxConcurrentProjectExports reports the current project export bound.
func maxConcurrentProjectExports() int {
	return cap(*projectExportSlots.Load())
}

// acquireProjectExportSlot waits for a project export slot. It returns
// ctx.Err() if the context ends first. Call the returned release function
// when the export is finished.
func acquireProjectExportSlot(ctx context.Context) (func(), error) {
	// select picks randomly among ready cases, so check cancellation
	// first: an already-cancelled request must not take a free slot.
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	slots := *projectExportSlots.Load()
	select {
	case slots <- struct{}{}:
		return func() { <-slots }, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

// ExportLimits are the opt-in export limits read from EXPORT_MAX_* at
// startup. Every field defaults to 0, meaning no limit (or, for
// MaxConcurrentProjectExports, the built-in default): LacyLights is mainly
// used in-house and large projects must always export. A restricted
// deployment such as a public demo sets them.
type ExportLimits struct {
	// MaxRows caps the data rows in one CSV/XLSX data export.
	MaxRows int
	// MaxCells caps the cells in a dynamic-width LOOK_DETAILS grid.
	MaxCells int
	// MaxBytes caps the encoded size of any export: CSV/XLSX data exports
	// and whole-project JSON and Eos ASCII exports.
	MaxBytes int
	// MaxConcurrentProjectExports bounds concurrent project exports.
	MaxConcurrentProjectExports int
}

// SetExportLimits applies export limits. Call it once at startup, before
// the server handles requests: the limits are plain fields read by request
// handlers without synchronization. The concurrency bound is process-wide
// (projectExportSlots), so calling this on any Resolver changes it for all
// of them; the size limits apply to this Resolver only.
func (r *Resolver) SetExportLimits(l ExportLimits) {
	if r.DataExportService != nil {
		r.DataExportService.MaxRows = max(l.MaxRows, 0)
		r.DataExportService.MaxCells = max(l.MaxCells, 0)
		r.DataExportService.MaxBytes = max(l.MaxBytes, 0)
	}
	r.projectExportMaxBytes = max(l.MaxBytes, 0)
	setMaxConcurrentProjectExports(l.MaxConcurrentProjectExports)
}

// checkProjectExportSize enforces the opt-in byte limit on a whole-project
// export. kind names the format in the error ("JSON", "Eos ASCII").
func (r *Resolver) checkProjectExportSize(kind string, size int) error {
	if r.projectExportMaxBytes > 0 && size > r.projectExportMaxBytes {
		return fmt.Errorf("%w: project %s export is %d bytes, over this server's export limit of %d bytes",
			exporttables.ErrTooLarge, kind, size, r.projectExportMaxBytes)
	}
	return nil
}

// exportProjectJSON exports a project as LacyLights JSON.
func (r *Resolver) exportProjectJSON(ctx context.Context, projectID string, options *generated.ExportOptionsInput) (*generated.ExportResult, error) {
	if err := r.ensureProjectAccess(ctx, projectID); err != nil {
		return nil, err
	}
	release, err := acquireProjectExportSlot(ctx)
	if err != nil {
		return nil, err
	}
	defer release()
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
	if err := r.checkProjectExportSize("JSON", len(jsonContent)); err != nil {
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
	release, err := acquireProjectExportSlot(ctx)
	if err != nil {
		return nil, err
	}
	defer release()
	res, err := r.EosExportService.Export(ctx, projectID)
	if err != nil {
		return nil, err
	}
	if err := r.checkProjectExportSize("Eos ASCII", len(res.Content)); err != nil {
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
