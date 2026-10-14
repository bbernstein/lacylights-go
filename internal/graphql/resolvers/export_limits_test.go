package resolvers

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/bbernstein/lacylights-go/internal/graphql/generated"
	exporttables "github.com/bbernstein/lacylights-go/internal/services/export_tables"
)

// restoreProjectExportBound resets the process-wide project export bound
// after a test changes it.
func restoreProjectExportBound(t *testing.T) {
	t.Helper()
	t.Cleanup(func() { setMaxConcurrentProjectExports(0) })
}

// TestExportLimits_DefaultUnlimited checks that, with no EXPORT_MAX_*
// configuration, nothing is capped: every export path succeeds and the
// services report 0 (no limit).
func TestExportLimits_DefaultUnlimited(t *testing.T) {
	_, r, cleanup := testSetup(t)
	defer cleanup()
	restoreProjectExportBound(t)
	s := seedDataExport(t, r, nil, nil)
	q := r.Query().(*queryResolver)
	ctx := context.Background()

	require.Zero(t, r.DataExportService.MaxRows)
	require.Zero(t, r.DataExportService.MaxCells)
	require.Zero(t, r.DataExportService.MaxBytes)
	require.Zero(t, r.projectExportMaxBytes)
	require.Equal(t, MaxConcurrentProjectExports, maxConcurrentProjectExports())

	// Applying the zero-value config (all env vars unset) keeps it so.
	r.SetExportLimits(ExportLimits{})
	require.Zero(t, r.DataExportService.MaxRows)
	require.Zero(t, r.DataExportService.MaxCells)
	require.Zero(t, r.DataExportService.MaxBytes)
	require.Zero(t, r.projectExportMaxBytes)
	require.Equal(t, MaxConcurrentProjectExports, maxConcurrentProjectExports())

	_, err := q.ExportDataTable(ctx, &s.projectID, generated.DataExportTableFixtures, generated.DataExportFormatCSV, nil)
	require.NoError(t, err)
	_, err = q.ExportDataTable(ctx, &s.projectID, generated.DataExportTableLookDetails, generated.DataExportFormatXlsx, &s.lookID)
	require.NoError(t, err)
	_, err = q.ExportDataWorkbook(ctx, s.projectID, nil)
	require.NoError(t, err)
	_, err = q.ProjectExport(ctx, s.projectID, nil)
	require.NoError(t, err)
	_, err = q.ProjectExportEos(ctx, s.projectID)
	require.NoError(t, err)
}

// TestSetExportLimits_WiresDataExportService checks that configured row,
// cell and byte limits reach the data export service and are enforced
// with ErrTooManyRows / ErrTooLarge and a message naming the limit.
func TestSetExportLimits_WiresDataExportService(t *testing.T) {
	_, r, cleanup := testSetup(t)
	defer cleanup()
	restoreProjectExportBound(t)
	s := seedDataExport(t, r, nil, nil)
	q := r.Query().(*queryResolver)
	ctx := context.Background()

	r.SetExportLimits(ExportLimits{MaxRows: 1})
	require.Equal(t, 1, r.DataExportService.MaxRows)
	_, err := q.ExportDataWorkbook(ctx, s.projectID, nil)
	require.ErrorIs(t, err, exporttables.ErrTooManyRows)
	require.ErrorContains(t, err, "export limit of 1 rows")

	r.SetExportLimits(ExportLimits{MaxCells: 1})
	require.Equal(t, 1, r.DataExportService.MaxCells)
	_, err = q.ExportDataTable(ctx, &s.projectID, generated.DataExportTableLookDetails, generated.DataExportFormatCSV, &s.lookID)
	require.ErrorIs(t, err, exporttables.ErrTooManyRows)
	require.ErrorContains(t, err, "export limit of 1 cells")

	r.SetExportLimits(ExportLimits{MaxBytes: 10})
	require.Equal(t, 10, r.DataExportService.MaxBytes)
	_, err = q.ExportDataTable(ctx, &s.projectID, generated.DataExportTableFixtures, generated.DataExportFormatCSV, nil)
	require.ErrorIs(t, err, exporttables.ErrTooLarge)
	require.ErrorContains(t, err, "export limit of 10 bytes")

	// Negative values (never produced by config, but possible from code)
	// mean no limit.
	r.SetExportLimits(ExportLimits{MaxRows: -1, MaxCells: -1, MaxBytes: -1})
	require.Zero(t, r.DataExportService.MaxRows)
	require.Zero(t, r.DataExportService.MaxCells)
	require.Zero(t, r.DataExportService.MaxBytes)
	require.Zero(t, r.projectExportMaxBytes)
	_, err = q.ExportDataWorkbook(ctx, s.projectID, nil)
	require.NoError(t, err)
}

// TestSetExportLimits_ProjectExportBytes checks that EXPORT_MAX_BYTES also
// caps whole-project JSON and Eos ASCII exports, and that an export exactly
// at the limit is allowed.
func TestSetExportLimits_ProjectExportBytes(t *testing.T) {
	_, r, cleanup := testSetup(t)
	defer cleanup()
	restoreProjectExportBound(t)
	s := seedDataExport(t, r, nil, nil)
	q := r.Query().(*queryResolver)
	m := r.Mutation().(*mutationResolver)
	ctx := context.Background()

	full, err := q.ProjectExport(ctx, s.projectID, nil)
	require.NoError(t, err)
	fullEos, err := q.ProjectExportEos(ctx, s.projectID)
	require.NoError(t, err)

	r.SetExportLimits(ExportLimits{MaxBytes: 10})
	_, err = q.ProjectExport(ctx, s.projectID, nil)
	require.ErrorIs(t, err, exporttables.ErrTooLarge)
	require.ErrorContains(t, err, "project JSON export is")
	require.ErrorContains(t, err, "export limit of 10 bytes")
	// The deprecated mutation shares the implementation.
	_, err = m.ExportProject(ctx, s.projectID, nil)
	require.ErrorIs(t, err, exporttables.ErrTooLarge)

	_, err = q.ProjectExportEos(ctx, s.projectID)
	require.ErrorIs(t, err, exporttables.ErrTooLarge)
	require.ErrorContains(t, err, "project Eos ASCII export is")

	// Exactly at the limit is fine.
	r.SetExportLimits(ExportLimits{MaxBytes: len(full.JSONContent)})
	res, err := q.ProjectExport(ctx, s.projectID, nil)
	require.NoError(t, err)
	require.Equal(t, full.JSONContent, res.JSONContent)
	r.SetExportLimits(ExportLimits{MaxBytes: len(fullEos.ASCIIContent)})
	_, err = q.ProjectExportEos(ctx, s.projectID)
	require.NoError(t, err)
}

// TestSetExportLimits_ConcurrentProjectExports checks that
// EXPORT_MAX_CONCURRENT_PROJECT_EXPORTS resizes the shared project export
// bound, and that 0 restores the built-in default.
func TestSetExportLimits_ConcurrentProjectExports(t *testing.T) {
	_, r, cleanup := testSetup(t)
	defer cleanup()
	restoreProjectExportBound(t)
	s := seedDataExport(t, r, nil, nil)

	r.SetExportLimits(ExportLimits{MaxConcurrentProjectExports: 1})
	require.Equal(t, 1, maxConcurrentProjectExports())
	release, err := acquireProjectExportSlot(context.Background())
	require.NoError(t, err)
	short, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	_, err = r.exportProjectJSON(short, s.projectID, nil)
	require.ErrorIs(t, err, context.DeadlineExceeded)
	release()
	_, err = r.exportProjectJSON(context.Background(), s.projectID, nil)
	require.NoError(t, err)

	r.SetExportLimits(ExportLimits{MaxConcurrentProjectExports: 3})
	require.Equal(t, 3, maxConcurrentProjectExports())

	r.SetExportLimits(ExportLimits{})
	require.Equal(t, MaxConcurrentProjectExports, maxConcurrentProjectExports())
}

// A release obtained before the bound is reconfigured returns its slot to
// the channel it came from, not the new one.
func TestProjectExportSlot_ReleaseAfterResize(t *testing.T) {
	restoreProjectExportBound(t)
	setMaxConcurrentProjectExports(1)
	release, err := acquireProjectExportSlot(context.Background())
	require.NoError(t, err)
	setMaxConcurrentProjectExports(1)
	release() // must not block or touch the new channel
	r2, err := acquireProjectExportSlot(context.Background())
	require.NoError(t, err)
	r2()
}

// SetExportLimits works on a Resolver built without a data export service:
// the project export limits still apply.
func TestSetExportLimits_NoDataExportService(t *testing.T) {
	restoreProjectExportBound(t)
	r := &Resolver{}
	r.SetExportLimits(ExportLimits{MaxRows: 1, MaxCells: 2, MaxBytes: 3, MaxConcurrentProjectExports: 1})
	require.Nil(t, r.DataExportService)
	require.Equal(t, 3, r.projectExportMaxBytes)
	require.Equal(t, 1, maxConcurrentProjectExports())
	require.ErrorIs(t, r.checkProjectExportSize("JSON", 4), exporttables.ErrTooLarge)
	require.NoError(t, r.checkProjectExportSize("JSON", 3))
}
