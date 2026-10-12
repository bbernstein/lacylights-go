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
	// ErrTooManyRows is returned when an export exceeds MaxExportRows.
	ErrTooManyRows = errors.New("export is too large")
)

// MaxExportRows is the default cap on the number of data rows in one export (a table, or
// the total across a workbook's sheets). Exports are built in memory and
// returned inline as base64, so the cap bounds server memory and response
// size. It is well above any realistic show (and the full OFL library) and
// below Excel's 1,048,576-row sheet limit.
const MaxExportRows = 250_000

// Repos are the repositories a Service reads from.
type Repos struct {
	Fixtures      *repositories.FixtureRepository
	FixtureGroups *repositories.FixtureGroupRepository
	Looks         *repositories.LookRepository
	CueLists      *repositories.CueListRepository
	LookBoards    *repositories.LookBoardRepository
	Effects       *repositories.EffectRepository
}

// Service builds export tables.
type Service struct {
	repos  Repos
	tables []*Table
	byID   map[TableID]*Table
	// Now returns the current time (server local time; it supplies the
	// date in generated filenames). Overridable in tests.
	Now func() time.Time
	// MaxRows caps the rows in one export; defaults to MaxExportRows.
	MaxRows int
}

// NewService creates a Service with every registered table.
func NewService(repos Repos) *Service {
	s := &Service{repos: repos, Now: time.Now, MaxRows: MaxExportRows}
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
	data, err := t.build(ctx, s, projectID, scopeID)
	if err != nil {
		return nil, err
	}
	if len(data.Rows) > s.MaxRows {
		return nil, fmt.Errorf("%w: %s has %d rows", ErrTooManyRows, id, len(data.Rows))
	}
	return data, nil
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
// project. When ids is empty WorkbookTables is used. Tables that need a
// scope ID, and global tables such as FIXTURE_LIBRARY, cannot be part of a
// workbook. Sheet names are made valid and unique. The returned row count
// is the total number of data rows across the primary sheets.
func (s *Service) BuildWorkbook(ctx context.Context, projectID string, ids []TableID) ([]Sheet, int, error) {
	if projectID == "" {
		return nil, 0, ErrProjectRequired
	}
	if len(ids) == 0 {
		ids = s.WorkbookTables()
	}
	var sheets []Sheet
	total := 0
	seen := make(map[TableID]bool, len(ids))
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
		data, err := t.build(ctx, s, projectID, "")
		if err != nil {
			return nil, 0, fmt.Errorf("build %s: %w", id, err)
		}
		total += len(data.Rows)
		if total > s.MaxRows {
			return nil, 0, fmt.Errorf("%w: workbook has more than %d rows", ErrTooManyRows, s.MaxRows)
		}
		sheets = append(sheets, data.Sheets(t.Label)...)
	}
	return UniqueSheetNames(sheets), total, nil
}
