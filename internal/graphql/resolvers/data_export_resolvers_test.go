package resolvers

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/csv"
	"errors"
	"regexp"
	"testing"
	"time"

	"github.com/99designs/gqlgen/client"
	"github.com/lucsky/cuid"
	"github.com/stretchr/testify/require"
	"github.com/xuri/excelize/v2"

	"github.com/bbernstein/lacylights-go/internal/database/models"
	"github.com/bbernstein/lacylights-go/internal/graphql/generated"
	exporttables "github.com/bbernstein/lacylights-go/internal/services/export_tables"
)

// exportSeed identifies data created by seedDataExport.
type exportSeed struct {
	projectID string
	lookID    string
	cueListID string
	boardID   string

	otherProjectID string
	otherLookID    string
	otherCueListID string
	otherBoardID   string
}

// seedDataExport creates a small project (optionally owned by groupID) and
// a second project used for cross-project scope checks.
func seedDataExport(t *testing.T, r *Resolver, groupID, otherGroupID *string) exportSeed {
	t.Helper()
	ctx := context.Background()
	require.NoError(t, r.db.AutoMigrate(&models.Effect{}, &models.EffectFixture{}, &models.EffectChannel{}, &models.CueEffect{}))

	var s exportSeed
	proj := &models.Project{Name: "Export Show", GroupID: groupID}
	require.NoError(t, r.ProjectRepo.Create(ctx, proj))
	s.projectID = proj.ID

	def := &models.FixtureDefinition{Manufacturer: "Generic", Model: "RGB", Type: "LED_PAR"}
	require.NoError(t, r.FixtureRepo.CreateDefinitionWithChannels(ctx, def, []models.ChannelDefinition{
		{Name: "Red", Type: "RED", Offset: 0},
		{Name: "Green", Type: "GREEN", Offset: 1},
		{Name: "Blue", Type: "BLUE", Offset: 2},
	}))
	count := 3
	mode := "3ch"
	fx := &models.FixtureInstance{
		Name: "Par 1", ProjectID: proj.ID, DefinitionID: def.ID,
		ModeName: &mode, ChannelCount: &count, Universe: 1, StartChannel: 1,
	}
	require.NoError(t, r.FixtureRepo.CreateWithChannels(ctx, fx, []models.InstanceChannel{
		{Offset: 0, Name: "Red", Type: "RED"},
		{Offset: 1, Name: "Green", Type: "GREEN"},
		{Offset: 2, Name: "Blue", Type: "BLUE"},
	}))

	look := &models.Look{Name: "Blue Wash", ProjectID: proj.ID}
	require.NoError(t, r.LookRepo.CreateWithFixtureValues(ctx, look, []models.FixtureValue{
		{FixtureID: fx.ID, Channels: `[{"offset":0,"value":0},{"offset":2,"value":255}]`},
	}))
	s.lookID = look.ID

	cl := &models.CueList{Name: "Main", ProjectID: proj.ID}
	require.NoError(t, r.CueListRepo.Create(ctx, cl))
	require.NoError(t, r.CueRepo.Create(ctx, &models.Cue{Name: "Go", CueNumber: 1, CueListID: cl.ID, LookID: look.ID, FadeInTime: 2}))
	s.cueListID = cl.ID

	board := &models.LookBoard{Name: "Board", ProjectID: proj.ID, DefaultFadeTime: 3}
	require.NoError(t, r.LookBoardRepo.CreateWithButtons(ctx, board, []models.LookBoardButton{{LookID: look.ID}}))
	s.boardID = board.ID

	other := &models.Project{Name: "Other", GroupID: otherGroupID}
	require.NoError(t, r.ProjectRepo.Create(ctx, other))
	s.otherProjectID = other.ID
	ol := &models.Look{Name: "Other look", ProjectID: other.ID}
	require.NoError(t, r.LookRepo.Create(ctx, ol))
	s.otherLookID = ol.ID
	ocl := &models.CueList{Name: "Other list", ProjectID: other.ID}
	require.NoError(t, r.CueListRepo.Create(ctx, ocl))
	s.otherCueListID = ocl.ID
	ob := &models.LookBoard{Name: "Other board", ProjectID: other.ID}
	require.NoError(t, r.LookBoardRepo.Create(ctx, ob))
	s.otherBoardID = ob.ID

	r.DataExportService.Now = func() time.Time { return time.Date(2026, 10, 12, 12, 0, 0, 0, time.UTC) }
	return s
}

type dataExportResp struct {
	Filename      string
	MimeType      string
	ContentBase64 string
	RowCount      int
}

func decodeCSV(t *testing.T, b64 string) [][]string {
	t.Helper()
	raw, err := base64.StdEncoding.DecodeString(b64)
	require.NoError(t, err)
	require.True(t, bytes.HasPrefix(raw, []byte("\xEF\xBB\xBF")), "CSV must start with a UTF-8 BOM")
	recs, err := csv.NewReader(bytes.NewReader(raw[3:])).ReadAll()
	require.NoError(t, err)
	return recs
}

func openXLSX(t *testing.T, b64 string) *excelize.File {
	t.Helper()
	raw, err := base64.StdEncoding.DecodeString(b64)
	require.NoError(t, err)
	f, err := excelize.OpenReader(bytes.NewReader(raw))
	require.NoError(t, err)
	t.Cleanup(func() { _ = f.Close() })
	return f
}

func TestDataExportTables_Query(t *testing.T) {
	c, r, cleanup := testSetup(t)
	defer cleanup()

	var resp struct {
		DataExportTables []struct {
			Table   string
			Label   string
			Scope   string
			Columns []string
		}
	}
	require.NoError(t, c.Post(`query { dataExportTables { table label scope columns } }`, &resp))
	require.Len(t, resp.DataExportTables, len(r.DataExportService.Tables()))
	byID := map[string]int{}
	for i, info := range resp.DataExportTables {
		byID[info.Table] = i
		require.NotEmpty(t, info.Label)
		require.NotEmpty(t, info.Columns)
	}
	fx := resp.DataExportTables[byID["FIXTURES"]]
	require.Equal(t, "project", fx.Scope)
	require.Equal(t, "Name", fx.Columns[0])
	ld := resp.DataExportTables[byID["LOOK_DETAILS"]]
	require.Equal(t, "look", ld.Scope)
	require.Equal(t, "Ch 1 … Ch N", ld.Columns[len(ld.Columns)-1])
	require.Equal(t, "none", resp.DataExportTables[byID["FIXTURE_LIBRARY"]].Scope)
	require.Equal(t, "lookBoard", resp.DataExportTables[byID["LOOK_BOARD_ENTRIES"]].Scope)
	require.Len(t, resp.DataExportTables, 9)
}

var filenamePattern = regexp.MustCompile(`^[\p{L}\p{N}._-]+-\d{4}-\d{2}-\d{2}\.(csv|xlsx)$`)

func TestExportDataTable_CSVAndXLSX(t *testing.T) {
	c, r, cleanup := testSetup(t)
	defer cleanup()
	s := seedDataExport(t, r, nil, nil)

	var csvResp struct{ ExportDataTable dataExportResp }
	require.NoError(t, c.Post(`query($p: ID, $t: DataExportTable!, $f: DataExportFormat!) {
		exportDataTable(projectId: $p, table: $t, format: $f) { filename mimeType contentBase64 rowCount }
	}`, &csvResp, client.Var("p", s.projectID), client.Var("t", "FIXTURES"), client.Var("f", "CSV")))
	res := csvResp.ExportDataTable
	require.Equal(t, "Export-Show-fixtures-2026-10-12.csv", res.Filename)
	require.Regexp(t, filenamePattern, res.Filename)
	require.Equal(t, exporttables.MimeCSV, res.MimeType)
	require.Equal(t, 1, res.RowCount)
	recs := decodeCSV(t, res.ContentBase64)
	require.Len(t, recs, 2)
	require.Equal(t, "Name", recs[0][0])
	require.Equal(t, "Par 1", recs[1][0])

	var xResp struct{ ExportDataTable dataExportResp }
	require.NoError(t, c.Post(`query($p: ID, $t: DataExportTable!, $f: DataExportFormat!) {
		exportDataTable(projectId: $p, table: $t, format: $f) { filename mimeType contentBase64 rowCount }
	}`, &xResp, client.Var("p", s.projectID), client.Var("t", "LOOKS"), client.Var("f", "XLSX")))
	require.Equal(t, "Export-Show-looks-2026-10-12.xlsx", xResp.ExportDataTable.Filename)
	require.Equal(t, exporttables.MimeXLSX, xResp.ExportDataTable.MimeType)
	f := openXLSX(t, xResp.ExportDataTable.ContentBase64)
	require.Equal(t, []string{"Looks"}, f.GetSheetList())
	rows, err := f.GetRows("Looks")
	require.NoError(t, err)
	require.Len(t, rows, 2)
	require.Equal(t, "Blue Wash", rows[1][0])
}

func TestExportDataWorkbook_Query(t *testing.T) {
	c, r, cleanup := testSetup(t)
	defer cleanup()
	s := seedDataExport(t, r, nil, nil)

	var resp struct{ ExportDataWorkbook dataExportResp }
	require.NoError(t, c.Post(`query($p: ID!) {
		exportDataWorkbook(projectId: $p) { filename mimeType contentBase64 rowCount }
	}`, &resp, client.Var("p", s.projectID)))
	res := resp.ExportDataWorkbook
	require.Equal(t, "Export-Show-data-2026-10-12.xlsx", res.Filename)
	require.Equal(t, exporttables.MimeXLSX, res.MimeType)
	f := openXLSX(t, res.ContentBase64)
	require.Len(t, f.GetSheetList(), len(r.DataExportService.WorkbookTables()))
	require.Positive(t, res.RowCount)

	var subset struct{ ExportDataWorkbook dataExportResp }
	require.NoError(t, c.Post(`query($p: ID!, $t: [DataExportTable!]) {
		exportDataWorkbook(projectId: $p, tables: $t) { contentBase64 rowCount }
	}`, &subset, client.Var("p", s.projectID), client.Var("t", []string{"FIXTURES", "CUE_LISTS"})))
	f = openXLSX(t, subset.ExportDataWorkbook.ContentBase64)
	require.Equal(t, []string{"Fixtures", "Cue Lists"}, f.GetSheetList())
	require.Equal(t, 2, subset.ExportDataWorkbook.RowCount)
}

func TestExportDataTable_Validation(t *testing.T) {
	_, r, cleanup := testSetup(t)
	defer cleanup()
	s := seedDataExport(t, r, nil, nil)
	q := r.Query().(*queryResolver)
	ctx := context.Background()

	// Null projectId is only allowed for FIXTURE_LIBRARY.
	_, err := q.ExportDataTable(ctx, nil, generated.DataExportTableFixtures, generated.DataExportFormatCSV, nil)
	require.ErrorContains(t, err, "projectId is required")

	// Unknown project.
	missing := "does-not-exist"
	_, err = q.ExportDataTable(ctx, &missing, generated.DataExportTableFixtures, generated.DataExportFormatCSV, nil)
	require.ErrorContains(t, err, "project not found")
	_, err = q.ExportDataWorkbook(ctx, missing, nil)
	require.ErrorContains(t, err, "project not found")

	// scopeId on a project-level table is rejected.
	_, err = q.ExportDataTable(ctx, &s.projectID, generated.DataExportTableFixtures, generated.DataExportFormatCSV, &s.lookID)
	require.True(t, errors.Is(err, exporttables.ErrScopeNotAllowed), "err = %v", err)

	// Unknown enum value / format.
	_, err = q.ExportDataTable(ctx, &s.projectID, generated.DataExportTable("NOPE"), generated.DataExportFormatCSV, nil)
	require.ErrorContains(t, err, "not supported")
	_, err = q.ExportDataTable(ctx, &s.projectID, generated.DataExportTableFixtures, generated.DataExportFormat("PDF"), nil)
	require.ErrorContains(t, err, "unsupported export format")

	// Workbook with an unknown table.
	_, err = q.ExportDataWorkbook(ctx, s.projectID, []generated.DataExportTable{"NOPE"})
	require.Error(t, err)
}

func TestDataExport_ServiceUnavailable(t *testing.T) {
	_, r, cleanup := testSetup(t)
	defer cleanup()
	r.DataExportService = nil
	q := r.Query().(*queryResolver)
	ctx := context.Background()
	pid := "p"

	_, err := q.DataExportTables(ctx)
	require.ErrorContains(t, err, "not available")
	_, err = q.ExportDataTable(ctx, &pid, generated.DataExportTableFixtures, generated.DataExportFormatCSV, nil)
	require.ErrorContains(t, err, "not available")
	_, err = q.ExportDataWorkbook(ctx, pid, nil)
	require.ErrorContains(t, err, "not available")
}

func TestExportData_AccessControl(t *testing.T) {
	r, cleanup := setupGroupAdminTestResolver(t, true)
	defer cleanup()

	groupID := cuid.New()
	otherGroupID := cuid.New()
	s := seedDataExport(t, r, &groupID, &otherGroupID)
	q := r.Query().(*queryResolver)

	outsider := createMemberContext("outsider", "out@test.com", otherGroupID)
	member := createMemberContext("member", "member@test.com", groupID)

	// Outsider is rejected for table and workbook exports.
	_, err := q.ExportDataTable(outsider, &s.projectID, generated.DataExportTableFixtures, generated.DataExportFormatCSV, nil)
	require.ErrorIs(t, err, ErrNotAuthorized)
	_, err = q.ExportDataWorkbook(outsider, s.projectID, nil)
	require.ErrorIs(t, err, ErrNotAuthorized)

	// Unauthenticated callers are rejected.
	_, err = q.ExportDataTable(context.Background(), &s.projectID, generated.DataExportTableFixtures, generated.DataExportFormatCSV, nil)
	require.ErrorIs(t, err, ErrNotAuthenticated)

	// Members of the owning group can export.
	res, err := q.ExportDataTable(member, &s.projectID, generated.DataExportTableFixtures, generated.DataExportFormatCSV, nil)
	require.NoError(t, err)
	require.Equal(t, 1, res.RowCount)
	wb, err := q.ExportDataWorkbook(member, s.projectID, []generated.DataExportTable{generated.DataExportTableLooks})
	require.NoError(t, err)
	require.Equal(t, 1, wb.RowCount)

	// Admins can export any project.
	_, err = q.ExportDataTable(createAdminContext(), &s.otherProjectID, generated.DataExportTableLooks, generated.DataExportFormatXlsx, nil)
	require.NoError(t, err)
}

func TestRequireAuthenticated(t *testing.T) {
	_, r, cleanup := testSetup(t)
	defer cleanup()
	require.NoError(t, r.requireAuthenticated(context.Background()), "auth disabled is a no-op")

	ra, cleanupA := setupGroupAdminTestResolver(t, true)
	defer cleanupA()
	require.ErrorIs(t, ra.requireAuthenticated(context.Background()), ErrNotAuthenticated)
	require.NoError(t, ra.requireAuthenticated(createMemberContext("u", "u@test.com", "g")))
	require.Equal(t, "", derefString(nil))
}

func TestExportDataTable_ScopedTables(t *testing.T) {
	c, r, cleanup := testSetup(t)
	defer cleanup()
	s := seedDataExport(t, r, nil, nil)

	var resp struct{ ExportDataTable dataExportResp }
	require.NoError(t, c.Post(`query($p: ID, $s: ID) {
		exportDataTable(projectId: $p, table: LOOK_DETAILS, format: XLSX, scopeId: $s) { filename contentBase64 rowCount }
	}`, &resp, client.Var("p", s.projectID), client.Var("s", s.lookID)))
	require.Equal(t, "Export-Show-look-details-Blue-Wash-2026-10-12.xlsx", resp.ExportDataTable.Filename)
	require.Equal(t, 1, resp.ExportDataTable.RowCount)
	f := openXLSX(t, resp.ExportDataTable.ContentBase64)
	require.Equal(t, []string{"Look Details", "Channel names"}, f.GetSheetList())

	var cl struct{ ExportDataTable dataExportResp }
	require.NoError(t, c.Post(`query($p: ID, $s: ID) {
		exportDataTable(projectId: $p, table: CUE_LIST_DETAILS, format: CSV, scopeId: $s) { filename contentBase64 rowCount }
	}`, &cl, client.Var("p", s.projectID), client.Var("s", s.cueListID)))
	require.Equal(t, "Export-Show-cue-list-details-Main-2026-10-12.csv", cl.ExportDataTable.Filename)
	recs := decodeCSV(t, cl.ExportDataTable.ContentBase64)
	require.Equal(t, []string{"1", "Go", "Blue Wash", "2", "0", "", "", "No", ""}, recs[1])

	// Board scope is optional.
	var lb struct{ ExportDataTable dataExportResp }
	require.NoError(t, c.Post(`query($p: ID) {
		exportDataTable(projectId: $p, table: LOOK_BOARD_ENTRIES, format: CSV) { filename rowCount }
	}`, &lb, client.Var("p", s.projectID)))
	require.Equal(t, "Export-Show-look-board-entries-2026-10-12.csv", lb.ExportDataTable.Filename)
	require.Equal(t, 1, lb.ExportDataTable.RowCount)

	q := r.Query().(*queryResolver)
	ctx := context.Background()
	// scopeId belonging to another project is rejected for every scoped table.
	for table, other := range map[generated.DataExportTable]string{
		generated.DataExportTableLookDetails:      s.otherLookID,
		generated.DataExportTableCueListDetails:   s.otherCueListID,
		generated.DataExportTableLookBoardEntries: s.otherBoardID,
	} {
		_, err := q.ExportDataTable(ctx, &s.projectID, table, generated.DataExportFormatCSV, &other)
		require.ErrorIs(t, err, exporttables.ErrScopeNotFound, "table %s", table)
	}
	// Scoped tables need a scopeId and a projectId.
	_, err := q.ExportDataTable(ctx, &s.projectID, generated.DataExportTableLookDetails, generated.DataExportFormatCSV, nil)
	require.ErrorIs(t, err, exporttables.ErrScopeRequired)
	_, err = q.ExportDataTable(ctx, nil, generated.DataExportTableLookDetails, generated.DataExportFormatCSV, &s.lookID)
	require.ErrorContains(t, err, "projectId is required")
	// Workbooks cannot include scoped tables.
	_, err = q.ExportDataWorkbook(ctx, s.projectID, []generated.DataExportTable{generated.DataExportTableCueListDetails})
	require.ErrorIs(t, err, exporttables.ErrScopeRequired)
}

func TestExportDataTable_FixtureLibrary(t *testing.T) {
	c, r, cleanup := testSetup(t)
	defer cleanup()
	s := seedDataExport(t, r, nil, nil)

	var resp struct{ ExportDataTable dataExportResp }
	require.NoError(t, c.Post(`query {
		exportDataTable(table: FIXTURE_LIBRARY, format: CSV) { filename rowCount contentBase64 }
	}`, &resp))
	require.Equal(t, "fixture-library-2026-10-12.csv", resp.ExportDataTable.Filename)
	require.Equal(t, 1, resp.ExportDataTable.RowCount)
	recs := decodeCSV(t, resp.ExportDataTable.ContentBase64)
	require.Equal(t, []string{"Manufacturer", "Model", "Mode", "Channel count", "Type", "Source"}, recs[0])

	// With a projectId the access check still runs, but the library
	// filename is not project-specific.
	q := r.Query().(*queryResolver)
	res, err := q.ExportDataTable(context.Background(), &s.projectID, generated.DataExportTableFixtureLibrary, generated.DataExportFormatCSV, nil)
	require.NoError(t, err)
	require.Equal(t, "fixture-library-2026-10-12.csv", res.Filename)
}

func TestExportDataTable_FixtureLibraryAuth(t *testing.T) {
	r, cleanup := setupGroupAdminTestResolver(t, true)
	defer cleanup()
	groupID := cuid.New()
	otherGroupID := cuid.New()
	s := seedDataExport(t, r, &groupID, &otherGroupID)
	q := r.Query().(*queryResolver)

	_, err := q.ExportDataTable(context.Background(), nil, generated.DataExportTableFixtureLibrary, generated.DataExportFormatCSV, nil)
	require.ErrorIs(t, err, ErrNotAuthenticated)

	member := createMemberContext("member", "m@test.com", groupID)
	res, err := q.ExportDataTable(member, nil, generated.DataExportTableFixtureLibrary, generated.DataExportFormatXlsx, nil)
	require.NoError(t, err)
	require.Equal(t, 1, res.RowCount)

	// Passing a project the caller cannot access is still rejected.
	_, err = q.ExportDataTable(member, &s.otherProjectID, generated.DataExportTableFixtureLibrary, generated.DataExportFormatCSV, nil)
	require.ErrorIs(t, err, ErrNotAuthorized)

	// An outsider cannot use their own project to reach another
	// project's scoped data either.
	outsider := createMemberContext("outsider", "o@test.com", otherGroupID)
	_, err = q.ExportDataTable(outsider, &s.otherProjectID, generated.DataExportTableLookDetails, generated.DataExportFormatCSV, &s.lookID)
	require.ErrorIs(t, err, exporttables.ErrScopeNotFound)
	_, err = q.ExportDataTable(outsider, &s.projectID, generated.DataExportTableLookDetails, generated.DataExportFormatCSV, &s.lookID)
	require.ErrorIs(t, err, ErrNotAuthorized)
}
