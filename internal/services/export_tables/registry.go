// Package exporttables turns LacyLights data lists (fixtures, looks, cue
// lists, ...) into spreadsheet-friendly tables and encodes them as CSV or
// XLSX files.
//
// Each exportable list is described by a Table in a registry: an ID, a
// human-readable label, the scope it is exported in (whole project, a single
// look, a single cue list, ...) and the column headers. Row data is produced
// on demand by Service.Build, which also verifies that the requested scope
// item belongs to the requested project.
package exporttables

import (
	"context"
	"errors"
	"fmt"
	"math"
	"time"

	"github.com/bbernstein/lacylights-go/internal/database/repositories"
)

// TableID identifies an exportable table. Values match the GraphQL
// DataExportTable enum.
type TableID string

// Table identifiers.
const (
	TableFixtureTypes     TableID = "FIXTURE_TYPES"
	TableFixtureLibrary   TableID = "FIXTURE_LIBRARY"
	TableFixtures         TableID = "FIXTURES"
	TableLooks            TableID = "LOOKS"
	TableLookDetails      TableID = "LOOK_DETAILS"
	TableEffects          TableID = "EFFECTS"
	TableLookBoardEntries TableID = "LOOK_BOARD_ENTRIES"
	TableCueLists         TableID = "CUE_LISTS"
	TableCueListDetails   TableID = "CUE_LIST_DETAILS"
)

// Scope describes what a table's rows are drawn from.
type Scope string

// Scopes. The string values are exposed through GraphQL
// (DataExportTableInfo.scope).
const (
	// ScopeNone tables are global and need neither a project nor a scope ID.
	ScopeNone Scope = "none"
	// ScopeProject tables cover a whole project.
	ScopeProject Scope = "project"
	// ScopeLook tables cover a single look (scopeId = look ID).
	ScopeLook Scope = "look"
	// ScopeCueList tables cover a single cue list (scopeId = cue list ID).
	ScopeCueList Scope = "cueList"
	// ScopeLookBoard tables cover a single look board (scopeId = board ID).
	ScopeLookBoard Scope = "lookBoard"
)

// Column is a single column in an exported table.
type Column struct {
	Key    string
	Header string
}

// Sheet is a named grid of values ready to be encoded.
type Sheet struct {
	Name    string
	Columns []Column
	Rows    [][]any
}

// TableData is the result of building a table.
type TableData struct {
	// Columns are the final columns. For most tables these equal the
	// registry columns; LOOK_DETAILS appends a dynamic "Ch 1 … Ch N" run.
	Columns []Column
	// Rows hold cell values: string, int, float64, bool or nil (blank).
	Rows [][]any
	// ItemName is the display name of the scope item (look, cue list or
	// look board), used in the generated filename. Empty for
	// project-wide and global tables.
	ItemName string
	// Extra are additional sheets that only XLSX output includes (for
	// example LOOK_DETAILS' "Channel names" sheet).
	Extra []Sheet
}

// Sheets returns the primary sheet (named name) followed by any extra sheets.
func (d *TableData) Sheets(name string) []Sheet {
	out := make([]Sheet, 0, 1+len(d.Extra))
	out = append(out, Sheet{Name: name, Columns: d.Columns, Rows: d.Rows})
	out = append(out, d.Extra...)
	return out
}

// Table describes one exportable list.
type Table struct {
	ID    TableID
	Label string
	// Slug is the filename-safe table name used in generated filenames.
	Slug  string
	Scope Scope
	// ScopeOptional is true when the table accepts a scope ID but can also
	// be exported for the whole project without one (LOOK_BOARD_ENTRIES).
	ScopeOptional bool
	// Columns are the static column definitions. LOOK_DETAILS adds a
	// variable number of channel columns at build time.
	Columns []Column
	// DynamicColumns, when set, describes columns appended at build time
	// (e.g. "Ch 1 … Ch N"). It is shown by InfoHeaders.
	DynamicColumns string

	build func(ctx context.Context, s *Service, projectID, scopeID string) (*TableData, error)
	// estimate returns an upper bound on the table's row count using
	// COUNT queries only. Used to enforce MaxRows before building.
	estimate func(ctx context.Context, c *repositories.RowCounter, projectID, scopeID string) (int, error)
}

// RequiresScopeID reports whether a scope ID must be supplied.
func (t *Table) RequiresScopeID() bool {
	switch t.Scope {
	case ScopeLook, ScopeCueList, ScopeLookBoard:
		return !t.ScopeOptional
	default:
		return false
	}
}

// AcceptsScopeID reports whether a scope ID may be supplied.
func (t *Table) AcceptsScopeID() bool {
	switch t.Scope {
	case ScopeLook, ScopeCueList, ScopeLookBoard:
		return true
	default:
		return false
	}
}

// RequiresProject reports whether a project ID must be supplied.
func (t *Table) RequiresProject() bool {
	return t.Scope != ScopeNone
}

// Headers returns the column headers.
func (t *Table) Headers() []string {
	return headers(t.Columns)
}

// InfoHeaders returns the column headers for display in a table picker:
// the static headers followed by the DynamicColumns description, if any.
func (t *Table) InfoHeaders() []string {
	h := t.Headers()
	if t.DynamicColumns != "" {
		h = append(h, t.DynamicColumns)
	}
	return h
}

func headers(cols []Column) []string {
	out := make([]string, len(cols))
	for i, c := range cols {
		out[i] = c.Header
	}
	return out
}

// Errors returned by Build and BuildWorkbook.
var (
	// ErrUnknownTable is returned for an unregistered table ID.
	ErrUnknownTable = errors.New("unknown export table")
	// ErrScopeRequired is returned when a scoped table is requested
	// without a scope ID.
	ErrScopeRequired = errors.New("scopeId is required for this table")
	// ErrScopeNotAllowed is returned when a scope ID is given for a table
	// that does not take one.
	ErrScopeNotAllowed = errors.New("scopeId is not supported for this table")
	// ErrScopeNotFound is returned when the scope item does not exist or
	// belongs to a different project.
	ErrScopeNotFound = errors.New("scope item not found in project")
	// ErrProjectRequired is returned when a project-bound table is
	// requested without a project ID.
	ErrProjectRequired = errors.New("projectId is required for this table")
	// ErrNotWorkbookTable is returned when a global (non-project) table is
	// requested in a workbook; workbooks only contain project tables.
	ErrNotWorkbookTable = errors.New("table is not a project table and cannot be included in a workbook")
	// ErrNoTables is returned when a workbook is requested with an
	// explicitly empty table list (omit the list to get the defaults).
	ErrNoTables = errors.New("at least one table is required; omit tables to export the default set")
	// ErrTooLarge is returned when an encoded export exceeds MaxBytes.
	ErrTooLarge = errors.New("export file is too large")
	// ErrTooManyRows is returned when an export exceeds MaxRows or MaxCells.
	ErrTooManyRows = errors.New("export is too large")
)

// Export size limits are opt-in. LacyLights is mainly used in-house by
// lighting designers and techs, who must be able to export large projects,
// so by default nothing is capped and no sizing queries run. A restricted
// deployment (for example a public demo) can set Service.MaxRows,
// MaxCells and MaxBytes (from the EXPORT_MAX_ROWS, EXPORT_MAX_CELLS and
// EXPORT_MAX_BYTES env vars); the constants below are suggested values.

// MaxExportRows is a suggested cap on the data rows in one export (a
// table, or the total across a workbook's sheets).
const MaxExportRows = 100_000

// MaxExportBytes is a suggested cap on an encoded export file's size
// (base64 in the GraphQL response adds about a third).
const MaxExportBytes = 20 << 20 // 20 MiB

// EncodeCSV encodes a table as CSV. When MaxBytes is set, it fails with
// ErrTooLarge as soon as the output would exceed it.
func (s *Service) EncodeCSV(cols []Column, rows [][]any) ([]byte, error) {
	return encodeCSV(cols, rows, byteLimit(s.MaxBytes))
}

// EncodeXLSX encodes sheets as XLSX. When MaxBytes is set, it fails with
// ErrTooLarge when the output would exceed it.
func (s *Service) EncodeXLSX(sheets []Sheet) ([]byte, error) {
	return encodeXLSX(sheets, byteLimit(s.MaxBytes))
}

// byteLimit maps an unset (<= 0) limit to "unlimited".
func byteLimit(n int) int {
	if n <= 0 {
		return math.MaxInt
	}
	return n
}

// MaxExportCells is a suggested cap on the cells of a dynamic-width table
// (LOOK_DETAILS), whose width grows with fixture channel counts.
const MaxExportCells = 5_000_000

// Repos are the repositories a Service reads from.
type Repos struct {
	Fixtures      *repositories.FixtureRepository
	FixtureGroups *repositories.FixtureGroupRepository
	Looks         *repositories.LookRepository
	CueLists      *repositories.CueListRepository
	LookBoards    *repositories.LookBoardRepository
	Effects       *repositories.EffectRepository
	// Counts sizes an export with COUNT queries before its data is loaded,
	// so over-limit exports are rejected without materialising them. Only
	// used when MaxRows is set; when nil the row cap is only checked after
	// building.
	Counts *repositories.RowCounter
}

// Service builds export tables.
type Service struct {
	repos  Repos
	tables []*Table
	byID   map[TableID]*Table
	// Now returns the current time (server local time; it supplies the
	// date in generated filenames). Overridable in tests.
	Now func() time.Time
	// MaxRows caps the rows in one export. 0 (the default) means no limit
	// and skips the pre-build COUNT queries.
	MaxRows int
	// MaxBytes caps an encoded export's size. 0 (the default) means no
	// limit.
	MaxBytes int
	// MaxCells caps the cells in a dynamic-width grid (LOOK_DETAILS,
	// counting both of its sheets). 0 (the default) means no limit.
	MaxCells int
	// slots bounds how many exports build and encode at once.
	slots chan struct{}
}

// MaxConcurrentExports is how many exports may build and encode at the
// same time. gqlgen resolves aliased root fields concurrently, so without
// a bound one request could start many large exports in parallel.
const MaxConcurrentExports = 2

// Acquire waits for an export slot. Call the returned release function
// when the export (including encoding) is finished. It returns ctx.Err()
// if the context ends first.
func (s *Service) Acquire(ctx context.Context) (func(), error) {
	// select picks randomly among ready cases, so check cancellation
	// first: an already-cancelled request must not take a free slot.
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	select {
	case s.slots <- struct{}{}:
		return func() { <-s.slots }, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

// NewService creates a Service with every registered table.
func NewService(repos Repos) *Service {
	s := &Service{repos: repos, Now: time.Now, slots: make(chan struct{}, MaxConcurrentExports)}
	s.tables = registry()
	s.byID = make(map[TableID]*Table, len(s.tables))
	for _, t := range s.tables {
		s.byID[t.ID] = t
	}
	return s
}

// Tables returns every registered table in display order.
func (s *Service) Tables() []*Table {
	return s.tables
}

// Lookup returns the table with the given ID.
func (s *Service) Lookup(id TableID) (*Table, bool) {
	t, ok := s.byID[id]
	return t, ok
}

// Build validates the request and produces the table's data.
// projectID may be empty only for ScopeNone tables.
func (s *Service) Build(ctx context.Context, id TableID, projectID, scopeID string) (*TableData, error) {
	t, ok := s.byID[id]
	if !ok {
		return nil, fmt.Errorf("%w: %s", ErrUnknownTable, id)
	}
	if t.RequiresProject() && projectID == "" {
		return nil, fmt.Errorf("%w: %s", ErrProjectRequired, id)
	}
	if scopeID == "" && t.RequiresScopeID() {
		return nil, fmt.Errorf("%w: %s", ErrScopeRequired, id)
	}
	if scopeID != "" && !t.AcceptsScopeID() {
		return nil, fmt.Errorf("%w: %s", ErrScopeNotAllowed, id)
	}
	// Check scope ownership before sizing, so a foreign scope ID gets
	// ErrScopeNotFound rather than an error revealing its row count.
	if err := s.checkScope(ctx, t, projectID, scopeID); err != nil {
		return nil, err
	}
	if s.MaxRows > 0 {
		n, err := s.estimateRows(ctx, t, projectID, scopeID)
		if err != nil {
			return nil, err
		}
		if n > s.MaxRows {
			return nil, fmt.Errorf("%w: %s has about %d rows, over this server's export limit of %d rows", ErrTooManyRows, id, n, s.MaxRows)
		}
	}
	data, err := t.build(ctx, s, projectID, scopeID)
	if err != nil {
		return nil, err
	}
	if s.MaxRows > 0 && len(data.Rows) > s.MaxRows {
		return nil, fmt.Errorf("%w: %s has %d rows, over this server's export limit of %d rows", ErrTooManyRows, id, len(data.Rows), s.MaxRows)
	}
	return data, nil
}

// checkScope verifies that scopeID (when given) names an item of t's
// scope type in the project. Builders re-check this when loading the item.
func (s *Service) checkScope(ctx context.Context, t *Table, projectID, scopeID string) error {
	if scopeID == "" {
		return nil
	}
	var owner string
	switch t.Scope {
	case ScopeLook:
		l, err := s.repos.Looks.FindByID(ctx, scopeID)
		if err != nil {
			return err
		}
		if l != nil {
			owner = l.ProjectID
		}
	case ScopeCueList:
		cl, err := s.repos.CueLists.FindByID(ctx, scopeID)
		if err != nil {
			return err
		}
		if cl != nil {
			owner = cl.ProjectID
		}
	case ScopeLookBoard:
		b, err := s.repos.LookBoards.FindByID(ctx, scopeID)
		if err != nil {
			return err
		}
		if b != nil {
			owner = b.ProjectID
		}
	default:
		return nil
	}
	if owner == "" || owner != projectID {
		return fmt.Errorf("%w: %s", ErrScopeNotFound, scopeID)
	}
	return nil
}

type extraSheetsKey struct{}

// WithoutExtraSheets returns a context telling builders to skip sheets that
// only XLSX output includes (such as LOOK_DETAILS' "Channel names"). Use it
// for CSV exports, which encode just the primary sheet.
func WithoutExtraSheets(ctx context.Context) context.Context {
	return context.WithValue(ctx, extraSheetsKey{}, false)
}

// wantExtraSheets reports whether builders should produce extra sheets
// (the default).
func wantExtraSheets(ctx context.Context) bool {
	v, ok := ctx.Value(extraSheetsKey{}).(bool)
	return !ok || v
}

// estimateRows returns an upper bound on t's row count, or 0 when no
// counter or estimate is available (the post-build check still applies).
func (s *Service) estimateRows(ctx context.Context, t *Table, projectID, scopeID string) (int, error) {
	if s.repos.Counts == nil || t.estimate == nil {
		return 0, nil
	}
	n, err := t.estimate(ctx, s.repos.Counts, projectID, scopeID)
	if err != nil {
		return 0, fmt.Errorf("size %s: %w", t.ID, err)
	}
	return n, nil
}

// WorkbookTables returns the tables included in a workbook export when the
// caller does not choose: every table that can be built for a project
// without a scope ID, except the (potentially huge) global fixture library.
func (s *Service) WorkbookTables() []TableID {
	var out []TableID
	for _, t := range s.tables {
		if t.Scope == ScopeNone || t.RequiresScopeID() {
			continue
		}
		out = append(out, t.ID)
	}
	return out
}

// BuildWorkbook builds one sheet per table (plus any extra sheets) for a
// project. When ids is nil WorkbookTables is used; an empty, non-nil ids
// is rejected with ErrNoTables. Tables that need a
// scope ID, and global tables such as FIXTURE_LIBRARY, cannot be part of a
// workbook. Sheet names are made valid and unique. The returned row count
// is the total number of data rows across the primary sheets.
func (s *Service) BuildWorkbook(ctx context.Context, projectID string, ids []TableID) ([]Sheet, int, error) {
	if projectID == "" {
		return nil, 0, ErrProjectRequired
	}
	if ids == nil {
		ids = s.WorkbookTables()
	} else if len(ids) == 0 {
		return nil, 0, ErrNoTables
	}

	// Validate every table and size the whole workbook before building any
	// sheet.
	var tables []*Table
	seen := make(map[TableID]bool, len(ids))
	estimated := 0
	for _, id := range ids {
		if seen[id] {
			continue
		}
		seen[id] = true
		t, ok := s.byID[id]
		if !ok {
			return nil, 0, fmt.Errorf("%w: %s", ErrUnknownTable, id)
		}
		if t.RequiresScopeID() {
			return nil, 0, fmt.Errorf("%w: %s cannot be included in a workbook", ErrScopeRequired, id)
		}
		if t.Scope == ScopeNone {
			return nil, 0, fmt.Errorf("%w: %s", ErrNotWorkbookTable, id)
		}
		if s.MaxRows > 0 {
			n, err := s.estimateRows(ctx, t, projectID, "")
			if err != nil {
				return nil, 0, err
			}
			estimated += n
			if estimated > s.MaxRows {
				return nil, 0, fmt.Errorf("%w: workbook has about %d rows, over this server's export limit of %d rows", ErrTooManyRows, estimated, s.MaxRows)
			}
		}
		tables = append(tables, t)
	}

	var sheets []Sheet
	total := 0
	for _, t := range tables {
		id := t.ID
		data, err := t.build(ctx, s, projectID, "")
		if err != nil {
			return nil, 0, fmt.Errorf("build %s: %w", id, err)
		}
		total += len(data.Rows)
		if s.MaxRows > 0 && total > s.MaxRows {
			return nil, 0, fmt.Errorf("%w: workbook has more than %d rows, this server's export limit", ErrTooManyRows, s.MaxRows)
		}
		sheets = append(sheets, data.Sheets(t.Label)...)
	}
	return UniqueSheetNames(sheets), total, nil
}
