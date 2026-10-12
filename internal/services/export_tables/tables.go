package exporttables

import (
	"context"
	"encoding/json"
	"sort"
	"strings"
	"time"

	"github.com/bbernstein/lacylights-go/internal/database/models"
)

// registry returns every exportable table in display order (the order of
// the GraphQL DataExportTable enum).
func registry() []*Table {
	return []*Table{
		fixtureTypesTable(),
		fixtureLibraryTable(),
		fixturesTable(),
		looksTable(),
		lookDetailsTable(),
		effectsTable(),
		lookBoardEntriesTable(),
		cueListsTable(),
		cueListDetailsTable(),
	}
}

// --- FIXTURES ---------------------------------------------------------------

func fixturesTable() *Table {
	return &Table{
		ID:    TableFixtures,
		Label: "Fixtures",
		Slug:  "fixtures",
		Scope: ScopeProject,
		Columns: []Column{
			{"name", "Name"},
			{"manufacturer", "Manufacturer"},
			{"model", "Model"},
			{"mode", "Mode"},
			{"type", "Type"},
			{"universe", "Universe"},
			{"startChannel", "Start channel"},
			{"endChannel", "End channel"},
			{"channelCount", "Channel count"},
			{"tags", "Tags"},
			{"groups", "Groups"},
			{"description", "Description"},
		},
		build: buildFixtures,
	}
}

func buildFixtures(ctx context.Context, s *Service, projectID, _ string) (*TableData, error) {
	fixtures, err := s.repos.Fixtures.FindByProjectID(ctx, projectID)
	if err != nil {
		return nil, err
	}
	defs, err := s.definitionsFor(ctx, fixtures)
	if err != nil {
		return nil, err
	}
	channels, err := s.repos.Fixtures.GetInstanceChannelsByFixtureIDs(ctx, fixtureIDs(fixtures))
	if err != nil {
		return nil, err
	}
	groups, err := s.fixtureGroupNames(ctx, projectID)
	if err != nil {
		return nil, err
	}

	rows := make([][]any, 0, len(fixtures))
	for i := range fixtures {
		f := &fixtures[i]
		def := defs[f.DefinitionID]
		count := fixtureChannelCount(f, channels[f.ID])
		var end any
		if count > 0 {
			end = f.StartChannel + count - 1
		}
		rows = append(rows, []any{
			f.Name,
			fixtureManufacturer(f, def),
			fixtureModel(f, def),
			strOrNil(f.ModeName),
			fixtureType(f, def),
			f.Universe,
			f.StartChannel,
			end,
			count,
			joinList(parseTags(f.Tags)),
			joinList(groups[f.ID]),
			strOrNil(f.Description),
		})
	}
	return &TableData{Columns: fixturesTable().Columns, Rows: rows}, nil
}

// --- LOOKS ------------------------------------------------------------------

func looksTable() *Table {
	return &Table{
		ID:    TableLooks,
		Label: "Looks",
		Slug:  "looks",
		Scope: ScopeProject,
		Columns: []Column{
			{"name", "Name"},
			{"description", "Description"},
			{"fixtureCount", "Fixture count"},
			{"createdAt", "Created (UTC)"},
			{"updatedAt", "Updated (UTC)"},
		},
		build: buildLooks,
	}
}

func buildLooks(ctx context.Context, s *Service, projectID, _ string) (*TableData, error) {
	looks, err := s.repos.Looks.FindByProjectID(ctx, projectID)
	if err != nil {
		return nil, err
	}
	sort.SliceStable(looks, func(i, j int) bool { return lessName(looks[i].Name, looks[j].Name) })
	ids := make([]string, len(looks))
	for i, l := range looks {
		ids[i] = l.ID
	}
	counts, err := s.repos.Looks.CountFixturesByLookIDs(ctx, ids)
	if err != nil {
		return nil, err
	}
	rows := make([][]any, 0, len(looks))
	for _, l := range looks {
		rows = append(rows, []any{
			l.Name,
			strOrNil(l.Description),
			counts[l.ID],
			timestamp(l.CreatedAt),
			timestamp(l.UpdatedAt),
		})
	}
	return &TableData{Columns: looksTable().Columns, Rows: rows}, nil
}

// --- CUE_LISTS --------------------------------------------------------------

func cueListsTable() *Table {
	return &Table{
		ID:    TableCueLists,
		Label: "Cue Lists",
		Slug:  "cue-lists",
		Scope: ScopeProject,
		Columns: []Column{
			{"name", "Name"},
			{"description", "Description"},
			{"cueCount", "Cue count"},
			{"loop", "Loop"},
			{"createdAt", "Created (UTC)"},
			{"updatedAt", "Updated (UTC)"},
		},
		build: buildCueLists,
	}
}

func buildCueLists(ctx context.Context, s *Service, projectID, _ string) (*TableData, error) {
	lists, err := s.repos.CueLists.FindByProjectID(ctx, projectID)
	if err != nil {
		return nil, err
	}
	sort.SliceStable(lists, func(i, j int) bool { return lessName(lists[i].Name, lists[j].Name) })
	ids := make([]string, len(lists))
	for i, cl := range lists {
		ids[i] = cl.ID
	}
	counts, err := s.repos.CueLists.CountCuesByCueListIDs(ctx, ids)
	if err != nil {
		return nil, err
	}
	rows := make([][]any, 0, len(lists))
	for _, cl := range lists {
		rows = append(rows, []any{
			cl.Name,
			strOrNil(cl.Description),
			counts[cl.ID],
			cl.Loop,
			timestamp(cl.CreatedAt),
			timestamp(cl.UpdatedAt),
		})
	}
	return &TableData{Columns: cueListsTable().Columns, Rows: rows}, nil
}

// --- shared helpers ---------------------------------------------------------

// definitionsFor loads the fixture definitions referenced by fixtures in
// one query, keyed by definition ID. Missing definitions are simply absent.
func (s *Service) definitionsFor(ctx context.Context, fixtures []models.FixtureInstance) (map[string]*models.FixtureDefinition, error) {
	seen := make(map[string]bool)
	var ids []string
	for i := range fixtures {
		id := fixtures[i].DefinitionID
		if id == "" || seen[id] {
			continue
		}
		seen[id] = true
		ids = append(ids, id)
	}
	return s.repos.Fixtures.FindDefinitionsByIDs(ctx, ids)
}

// fixtureGroupNames maps fixture ID to the names of the fixture groups it
// belongs to, in group order.
func (s *Service) fixtureGroupNames(ctx context.Context, projectID string) (map[string][]string, error) {
	out := make(map[string][]string)
	if s.repos.FixtureGroups == nil {
		return out, nil
	}
	groups, err := s.repos.FixtureGroups.FindByProjectID(ctx, projectID)
	if err != nil {
		return nil, err
	}
	ids := make([]string, len(groups))
	for i, g := range groups {
		ids[i] = g.ID
	}
	members, err := s.repos.FixtureGroups.GetMembersByGroupIDs(ctx, ids)
	if err != nil {
		return nil, err
	}
	for _, g := range groups {
		for _, m := range members[g.ID] {
			out[m.FixtureID] = append(out[m.FixtureID], g.Name)
		}
	}
	return out, nil
}

func fixtureIDs(fixtures []models.FixtureInstance) []string {
	ids := make([]string, len(fixtures))
	for i := range fixtures {
		ids[i] = fixtures[i].ID
	}
	return ids
}

// fixtureChannelCount prefers the denormalised channel count and falls back
// to the number of instance channels.
func fixtureChannelCount(f *models.FixtureInstance, channels []models.InstanceChannel) int {
	if f.ChannelCount != nil && *f.ChannelCount > 0 {
		return *f.ChannelCount
	}
	return len(channels)
}

func fixtureManufacturer(f *models.FixtureInstance, def *models.FixtureDefinition) string {
	if f.Manufacturer != nil && *f.Manufacturer != "" {
		return *f.Manufacturer
	}
	if def != nil {
		return def.Manufacturer
	}
	return ""
}

func fixtureModel(f *models.FixtureInstance, def *models.FixtureDefinition) string {
	if f.Model != nil && *f.Model != "" {
		return *f.Model
	}
	if def != nil {
		return def.Model
	}
	return ""
}

func fixtureType(f *models.FixtureInstance, def *models.FixtureDefinition) string {
	if f.Type != nil && *f.Type != "" {
		return *f.Type
	}
	if def != nil {
		return def.Type
	}
	return ""
}

func parseTags(raw *string) []string {
	if raw == nil || *raw == "" {
		return nil
	}
	var tags []string
	if err := json.Unmarshal([]byte(*raw), &tags); err != nil {
		return nil
	}
	return tags
}

// joinList joins list values with "; " (commas are common inside names).
// Returns nil (a blank cell) for an empty list.
func joinList(items []string) any {
	if len(items) == 0 {
		return nil
	}
	return strings.Join(items, "; ")
}

// lessName orders names case-insensitively, falling back to a byte
// comparison so the order is total and deterministic.
func lessName(a, b string) bool {
	la, lb := strings.ToLower(a), strings.ToLower(b)
	if la != lb {
		return la < lb
	}
	return a < b
}

func strOrNil(p *string) any {
	if p == nil {
		return nil
	}
	return *p
}

// timestamp formats a time as "YYYY-MM-DD HH:MM:SS" in UTC, which
// spreadsheet apps recognise as a date-time.
func timestamp(t time.Time) any {
	if t.IsZero() {
		return nil
	}
	return t.UTC().Format("2006-01-02 15:04:05")
}
