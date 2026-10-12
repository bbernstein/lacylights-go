package resolvers

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"

	"github.com/bbernstein/lacylights-go/internal/database/models"
	"github.com/bbernstein/lacylights-go/internal/graphql/generated"
	"github.com/bbernstein/lacylights-go/internal/middleware"
	exporttables "github.com/bbernstein/lacylights-go/internal/services/export_tables"
)

// workbookFilenameSlug is the <table> part of a workbook export filename.
const workbookFilenameSlug = "data"

// dataExportTables lists the registered export tables for the UI picker.
func (r *Resolver) dataExportTables() ([]*generated.DataExportTableInfo, error) {
	if err := r.requireDataExport(); err != nil {
		return nil, err
	}
	tables := r.DataExportService.Tables()
	out := make([]*generated.DataExportTableInfo, 0, len(tables))
	for _, t := range tables {
		out = append(out, &generated.DataExportTableInfo{
			Table:   generated.DataExportTable(t.ID),
			Label:   t.Label,
			Scope:   string(t.Scope),
			Columns: t.InfoHeaders(),
		})
	}
	return out, nil
}

// exportDataTable exports a single table as CSV or XLSX.
//
// Access rules:
//   - projectId is required for every table except FIXTURE_LIBRARY.
//   - When a projectId is given the caller must have access to it
//     (ensureProjectAccess) and the project must exist.
//   - FIXTURE_LIBRARY without a projectId only requires an authenticated
//     caller when auth is enabled (the library is global, like the
//     fixtureDefinitions query).
//   - scopeId must belong to projectId (enforced by the export service).
func (r *Resolver) exportDataTable(ctx context.Context, projectID *string, table generated.DataExportTable, format generated.DataExportFormat, scopeID *string) (*generated.DataExportResult, error) {
	if err := r.requireDataExport(); err != nil {
		return nil, err
	}
	t, ok := r.DataExportService.Lookup(exporttables.TableID(table))
	if !ok {
		return nil, fmt.Errorf("export table %s is not supported", table)
	}

	pid := derefString(projectID)
	sid := derefString(scopeID)

	projectName := ""
	if pid == "" {
		if t.RequiresProject() {
			return nil, fmt.Errorf("projectId is required for table %s", table)
		}
		if err := r.requireAuthenticated(ctx); err != nil {
			return nil, err
		}
	} else {
		project, err := r.loadAccessibleProject(ctx, pid)
		if err != nil {
			return nil, err
		}
		projectName = project.Name
	}

	release, err := r.DataExportService.Acquire(ctx)
	if err != nil {
		return nil, err
	}
	defer release()

	buildCtx := ctx
	if format == generated.DataExportFormatCSV {
		// CSV encodes only the primary sheet; skip XLSX-only extra sheets.
		buildCtx = exporttables.WithoutExtraSheets(ctx)
	}
	data, err := r.DataExportService.Build(buildCtx, t.ID, pid, sid)
	if err != nil {
		return nil, err
	}
	// Don't encode for a client that has gone away; free the slot now.
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	if t.Scope == exporttables.ScopeNone {
		// Global tables (the fixture library) are not project content, so
		// the filename does not carry a project name.
		projectName = ""
	}

	var (
		content  []byte
		ext      string
		mimeType string
	)
	switch format {
	case generated.DataExportFormatCSV:
		content, err = r.DataExportService.EncodeCSV(data.Columns, data.Rows)
		ext, mimeType = exporttables.ExtCSV, exporttables.MimeCSV
	case generated.DataExportFormatXlsx:
		content, err = r.DataExportService.EncodeXLSX(data.Sheets(t.Label))
		ext, mimeType = exporttables.ExtXLSX, exporttables.MimeXLSX
	default:
		return nil, fmt.Errorf("unsupported export format: %s", format)
	}
	if err != nil {
		return nil, err
	}

	return &generated.DataExportResult{
		Filename:      exporttables.Filename(projectName, t.Slug, data.ItemName, r.DataExportService.Now(), ext),
		MimeType:      mimeType,
		ContentBase64: base64.StdEncoding.EncodeToString(content),
		RowCount:      len(data.Rows),
	}, nil
}

// exportDataWorkbook exports several project-level tables into one XLSX
// workbook, one sheet per table.
func (r *Resolver) exportDataWorkbook(ctx context.Context, projectID string, tables []generated.DataExportTable) (*generated.DataExportResult, error) {
	if err := r.requireDataExport(); err != nil {
		return nil, err
	}
	project, err := r.loadAccessibleProject(ctx, projectID)
	if err != nil {
		return nil, err
	}

	// Keep nil (tables omitted → default set) distinct from an explicit
	// empty list, which BuildWorkbook rejects.
	var ids []exporttables.TableID
	if tables != nil {
		ids = make([]exporttables.TableID, len(tables))
		for i, t := range tables {
			ids[i] = exporttables.TableID(t)
		}
	}
	release, err := r.DataExportService.Acquire(ctx)
	if err != nil {
		return nil, err
	}
	defer release()
	sheets, rowCount, err := r.DataExportService.BuildWorkbook(ctx, projectID, ids)
	if err != nil {
		return nil, err
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	content, err := r.DataExportService.EncodeXLSX(sheets)
	if err != nil {
		return nil, err
	}
	return &generated.DataExportResult{
		Filename:      exporttables.Filename(project.Name, workbookFilenameSlug, "", r.DataExportService.Now(), exporttables.ExtXLSX),
		MimeType:      exporttables.MimeXLSX,
		ContentBase64: base64.StdEncoding.EncodeToString(content),
		RowCount:      rowCount,
	}, nil
}

// errDataExportUnavailable is returned when the server was built without
// a data export service.
var errDataExportUnavailable = errors.New("data export not available on this platform")

func (r *Resolver) requireDataExport() error {
	if r.DataExportService == nil {
		return errDataExportUnavailable
	}
	return nil
}

// loadAccessibleProject checks that the caller can access the project
// (ensureProjectAccess) and that it exists.
func (r *Resolver) loadAccessibleProject(ctx context.Context, projectID string) (*models.Project, error) {
	if err := r.ensureProjectAccess(ctx, projectID); err != nil {
		return nil, err
	}
	project, err := r.ProjectRepo.FindByID(ctx, projectID)
	if err != nil {
		return nil, err
	}
	if project == nil {
		return nil, fmt.Errorf("project not found: %s", projectID)
	}
	return project, nil
}

// requireAuthenticated returns ErrNotAuthenticated when auth is enabled and
// the request carries no authenticated identity. It is a no-op when auth
// is disabled.
func (r *Resolver) requireAuthenticated(ctx context.Context) error {
	if r.AuthService == nil || !r.AuthService.IsEnabled() {
		return nil
	}
	if !middleware.IsAuthenticated(ctx) {
		return ErrNotAuthenticated
	}
	return nil
}

func derefString(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}
