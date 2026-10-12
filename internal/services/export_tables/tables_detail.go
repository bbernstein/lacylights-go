package exporttables

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strconv"

	"github.com/bbernstein/lacylights-go/internal/database/models"
)

// Fixture definition sources shown in the "Source" column.
const (
	sourceOFL     = "OFL"
	sourceBuiltIn = "Built-in"
	sourceCustom  = "Custom"
)

// --- FIXTURE_TYPES / FIXTURE_LIBRARY ---------------------------------------

var fixtureTypeColumns = []Column{
	{"manufacturer", "Manufacturer"},
	{"model", "Model"},
	{"mode", "Mode"},
	{"channelCount", "Channel count"},
	{"type", "Type"},
	{"source", "Source"},
}

func fixtureTypesTable() *Table {
	cols := append(append([]Column{}, fixtureTypeColumns...), Column{"fixtureCount", "Fixtures in project using it"})
	return &Table{
		ID:       TableFixtureTypes,
		Label:    "Fixture Types",
		Slug:     "fixture-types",
		Scope:    ScopeProject,
		Columns:  cols,
		build:    buildFixtureTypes,
		estimate: countInProject(&models.FixtureInstance{}),
	}
}

func fixtureLibraryTable() *Table {
	return &Table{
		ID:       TableFixtureLibrary,
		Label:    "Fixture Library",
		Slug:     "fixture-library",
		Scope:    ScopeNone,
		Columns:  append([]Column{}, fixtureTypeColumns...),
		build:    buildFixtureLibrary,
		estimate: countLibrary,
	}
}

// buildFixtureTypes lists one row per (definition, mode) used by fixtures
// in the project, with the number of fixtures using it.
func buildFixtureTypes(ctx context.Context, s *Service, projectID, _ string) (*TableData, error) {
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

	type key struct{ defID, mode string }
	type usage struct {
		manufacturer, model, mode, typ, source string
		channelCount, fixtures                 int
	}
	byKey := make(map[key]*usage)
	var order []key
	for i := range fixtures {
		f := &fixtures[i]
		mode := ""
		if f.ModeName != nil {
			mode = *f.ModeName
		}
		k := key{f.DefinitionID, mode}
		u, ok := byKey[k]
		if !ok {
			def := defs[f.DefinitionID]
			u = &usage{
				manufacturer: fixtureManufacturer(f, def),
				model:        fixtureModel(f, def),
				mode:         mode,
				typ:          fixtureType(f, def),
				source:       definitionSource(def),
				channelCount: fixtureChannelCount(f, channels[f.ID]),
			}
			byKey[k] = u
			order = append(order, k)
		}
		u.fixtures++
	}

	usages := make([]*usage, 0, len(order))
	for _, k := range order {
		usages = append(usages, byKey[k])
	}
	sort.SliceStable(usages, func(i, j int) bool {
		a, b := usages[i], usages[j]
		if a.manufacturer != b.manufacturer {
			return lessName(a.manufacturer, b.manufacturer)
		}
		if a.model != b.model {
			return lessName(a.model, b.model)
		}
		return lessName(a.mode, b.mode)
	})

	rows := make([][]any, 0, len(usages))
	for _, u := range usages {
		rows = append(rows, []any{
			u.manufacturer, u.model, blankIfEmpty(u.mode), u.channelCount, u.typ, blankIfEmpty(u.source), u.fixtures,
		})
	}
	return &TableData{Columns: fixtureTypesTable().Columns, Rows: rows}, nil
}

// buildFixtureLibrary lists every fixture definition, one row per mode
// (or a single row with a blank mode for definitions without modes).
func buildFixtureLibrary(ctx context.Context, s *Service, _, _ string) (*TableData, error) {
	defs, err := s.repos.Fixtures.FindAllDefinitions(ctx)
	if err != nil {
		return nil, err
	}
	modes, err := s.repos.Fixtures.FindAllModes(ctx)
	if err != nil {
		return nil, err
	}
	modesByDef := make(map[string][]models.FixtureMode)
	for _, m := range modes {
		modesByDef[m.DefinitionID] = append(modesByDef[m.DefinitionID], m)
	}
	channelCounts, err := s.repos.Fixtures.CountChannelsByDefinition(ctx)
	if err != nil {
		return nil, err
	}

	rows := make([][]any, 0, len(defs))
	for i := range defs {
		d := &defs[i]
		src := definitionSource(d)
		ms := modesByDef[d.ID]
		if len(ms) == 0 {
			rows = append(rows, []any{d.Manufacturer, d.Model, nil, channelCounts[d.ID], d.Type, src})
			continue
		}
		for _, m := range ms {
			rows = append(rows, []any{d.Manufacturer, d.Model, m.Name, m.ChannelCount, d.Type, src})
		}
	}
	return &TableData{Columns: fixtureLibraryTable().Columns, Rows: rows}, nil
}

// definitionSource classifies a definition as OFL-imported, built-in or
// user-created. Returns "" for a missing definition.
func definitionSource(def *models.FixtureDefinition) string {
	switch {
	case def == nil:
		return ""
	case def.OFLSourceHash != nil:
		return sourceOFL
	case def.IsBuiltIn:
		return sourceBuiltIn
	default:
		return sourceCustom
	}
}

// --- LOOK_DETAILS ----------------------------------------------------------

var lookDetailBaseColumns = []Column{
	{"fixture", "Fixture"},
	{"manufacturer", "Manufacturer"},
	{"model", "Model"},
	{"mode", "Mode"},
	{"universe", "Universe"},
	{"startAddress", "Start address"},
}

// ChannelNamesSheet is the name of LOOK_DETAILS' second XLSX sheet.
const ChannelNamesSheet = "Channel names"

func lookDetailsTable() *Table {
	return &Table{
		ID:             TableLookDetails,
		Label:          "Look Details",
		Slug:           "look-details",
		Scope:          ScopeLook,
		Columns:        append([]Column{}, lookDetailBaseColumns...),
		DynamicColumns: "Ch 1 … Ch N",
		build:          buildLookDetails,
		estimate:       countInScope(&models.FixtureValue{}, "look_id"),
	}
}

// buildLookDetails produces the wide look format: one row per fixture,
// then "Ch 1" … "Ch N" where N is the largest channel count among the
// look's fixtures. Channels a fixture does not have, or that the look does
// not set, are blank; a stored 0 is written as 0. Stored values beyond a
// fixture's channel count are ignored. Fixtures from other projects are
// skipped. An extra "Channel names" sheet has the same grid with each
// channel's name.
func buildLookDetails(ctx context.Context, s *Service, projectID, scopeID string) (*TableData, error) {
	look, err := s.repos.Looks.FindByID(ctx, scopeID)
	if err != nil {
		return nil, err
	}
	if look == nil || look.ProjectID != projectID {
		return nil, fmt.Errorf("%w: look %s", ErrScopeNotFound, scopeID)
	}
	values, err := s.repos.Looks.GetFixtureValues(ctx, look.ID)
	if err != nil {
		return nil, err
	}
	ids := make([]string, len(values))
	for i, v := range values {
		ids[i] = v.FixtureID
	}
	fixtures, err := s.repos.Fixtures.FindByIDs(ctx, ids)
	if err != nil {
		return nil, err
	}
	// Only export fixtures that belong to the look's project. Look writes do
	// not validate fixture ownership, so a look could reference a fixture
	// from another project; exporting it would disclose that project's data.
	owned := fixtures[:0]
	for i := range fixtures {
		if fixtures[i].ProjectID == projectID {
			owned = append(owned, fixtures[i])
		}
	}
	fixtures = owned
	byID := make(map[string]*models.FixtureInstance, len(fixtures))
	for i := range fixtures {
		byID[fixtures[i].ID] = &fixtures[i]
	}
	defs, err := s.definitionsFor(ctx, fixtures)
	if err != nil {
		return nil, err
	}
	channels, err := s.repos.Fixtures.GetInstanceChannelsByFixtureIDs(ctx, ids)
	if err != nil {
		return nil, err
	}

	type entry struct {
		order    *int
		fixture  *models.FixtureInstance
		values   map[int]int
		names    map[int]string
		channels int
	}
	entries := make([]entry, 0, len(values))
	maxChannels := 0
	for _, fv := range values {
		f := byID[fv.FixtureID]
		if f == nil {
			continue // dangling fixture value
		}
		var cvs []models.ChannelValue
		if fv.Channels != "" {
			if err := json.Unmarshal([]byte(fv.Channels), &cvs); err != nil {
				return nil, fmt.Errorf("look %s fixture %s: invalid channel values: %w", look.ID, f.ID, err)
			}
		}
		e := entry{
			order:    fv.LookOrder,
			fixture:  f,
			values:   make(map[int]int, len(cvs)),
			names:    make(map[int]string),
			channels: fixtureChannelCount(f, channels[f.ID]),
		}
		for _, cv := range cvs {
			// Ignore stored values for channels the fixture does not have.
			// When the channel count is unknown (0), keep every stored value
			// so the data is not silently lost.
			if cv.Offset < 0 || (e.channels > 0 && cv.Offset >= e.channels) {
				continue
			}
			e.values[cv.Offset] = cv.Value
			if cv.Offset+1 > e.channels {
				e.channels = cv.Offset + 1
			}
		}
		for _, ch := range channels[f.ID] {
			e.names[ch.Offset] = ch.Name
		}
		if e.channels > maxChannels {
			maxChannels = e.channels
		}
		entries = append(entries, e)
	}

	sort.SliceStable(entries, func(i, j int) bool {
		a, b := entries[i], entries[j]
		if (a.order != nil) != (b.order != nil) {
			return a.order != nil
		}
		if a.order != nil && *a.order != *b.order {
			return *a.order < *b.order
		}
		if a.fixture.Universe != b.fixture.Universe {
			return a.fixture.Universe < b.fixture.Universe
		}
		if a.fixture.StartChannel != b.fixture.StartChannel {
			return a.fixture.StartChannel < b.fixture.StartChannel
		}
		return lessName(a.fixture.Name, b.fixture.Name)
	})

	cols := append([]Column{}, lookDetailBaseColumns...)
	for i := 1; i <= maxChannels; i++ {
		n := strconv.Itoa(i)
		cols = append(cols, Column{"ch" + n, "Ch " + n})
	}

	rows := make([][]any, 0, len(entries))
	nameRows := make([][]any, 0, len(entries))
	for _, e := range entries {
		f := e.fixture
		def := defs[f.DefinitionID]
		base := []any{f.Name, fixtureManufacturer(f, def), fixtureModel(f, def), strOrNil(f.ModeName), f.Universe, f.StartChannel}
		row := append(make([]any, 0, len(cols)), base...)
		nameRow := append(make([]any, 0, len(cols)), base...)
		for off := 0; off < maxChannels; off++ {
			if v, ok := e.values[off]; ok {
				row = append(row, v)
			} else {
				row = append(row, nil)
			}
			if name, ok := e.names[off]; ok && off < e.channels {
				nameRow = append(nameRow, name)
			} else {
				nameRow = append(nameRow, nil)
			}
		}
		rows = append(rows, row)
		nameRows = append(nameRows, nameRow)
	}

	return &TableData{
		Columns:  cols,
		Rows:     rows,
		ItemName: look.Name,
		Extra:    []Sheet{{Name: ChannelNamesSheet, Columns: cols, Rows: nameRows}},
	}, nil
}

// --- EFFECTS ---------------------------------------------------------------

func effectsTable() *Table {
	return &Table{
		ID:    TableEffects,
		Label: "Effects",
		Slug:  "effects",
		Scope: ScopeProject,
		Columns: []Column{
			{"name", "Name"},
			{"description", "Description"},
			{"type", "Type"},
			{"waveform", "Waveform"},
			{"frequency", "Frequency (Hz)"},
			{"amplitude", "Amplitude (%)"},
			{"offset", "Offset (%)"},
			{"phaseOffset", "Phase offset (deg)"},
			{"easing", "Easing"},
			{"priorityBand", "Priority band"},
			{"prioritySub", "Priority"},
			{"compositionMode", "Composition"},
			{"onCueChange", "On cue change"},
			{"fadeDuration", "Fade duration (s)"},
			{"masterValue", "Master value"},
			{"fixtureCount", "Fixture count"},
		},
		build:    buildEffects,
		estimate: countInProject(&models.Effect{}),
	}
}

func buildEffects(ctx context.Context, s *Service, projectID, _ string) (*TableData, error) {
	effects, err := s.repos.Effects.FindByProjectID(ctx, projectID)
	if err != nil {
		return nil, err
	}
	sort.SliceStable(effects, func(i, j int) bool { return lessName(effects[i].Name, effects[j].Name) })
	ids := make([]string, len(effects))
	for i, e := range effects {
		ids[i] = e.ID
	}
	counts, err := s.repos.Effects.CountFixturesByEffectIDs(ctx, ids)
	if err != nil {
		return nil, err
	}
	rows := make([][]any, 0, len(effects))
	for _, e := range effects {
		rows = append(rows, []any{
			e.Name,
			strOrNil(e.Description),
			e.EffectType,
			strOrNil(e.Waveform),
			e.Frequency,
			e.Amplitude,
			e.Offset,
			e.PhaseOffset,
			strOrNil(e.EasingType),
			e.PriorityBand,
			e.PrioritySub,
			e.CompositionMode,
			e.OnCueChange,
			floatOrNil(e.FadeDuration),
			floatOrNil(e.MasterValue),
			counts[e.ID],
		})
	}
	return &TableData{Columns: effectsTable().Columns, Rows: rows}, nil
}

// --- LOOK_BOARD_ENTRIES ----------------------------------------------------

func lookBoardEntriesTable() *Table {
	return &Table{
		ID:            TableLookBoardEntries,
		Label:         "Look Board Entries",
		Slug:          "look-board-entries",
		Scope:         ScopeLookBoard,
		ScopeOptional: true,
		Columns: []Column{
			{"board", "Board"},
			{"label", "Button label"},
			{"look", "Look"},
			{"x", "X"},
			{"y", "Y"},
			{"width", "Width"},
			{"height", "Height"},
			{"color", "Color"},
			{"fadeTime", "Board fade time (s)"},
		},
		build:    buildLookBoardEntries,
		estimate: countBoardButtons,
	}
}

// buildLookBoardEntries lists the buttons of one board (scopeID) or of
// every board in the project (no scopeID), boards sorted by name and
// buttons top-to-bottom, left-to-right.
func buildLookBoardEntries(ctx context.Context, s *Service, projectID, scopeID string) (*TableData, error) {
	var boards []models.LookBoard
	item := ""
	if scopeID != "" {
		b, err := s.repos.LookBoards.FindByID(ctx, scopeID)
		if err != nil {
			return nil, err
		}
		if b == nil || b.ProjectID != projectID {
			return nil, fmt.Errorf("%w: look board %s", ErrScopeNotFound, scopeID)
		}
		boards = []models.LookBoard{*b}
		item = b.Name
	} else {
		var err error
		boards, err = s.repos.LookBoards.FindByProjectID(ctx, projectID)
		if err != nil {
			return nil, err
		}
		sort.SliceStable(boards, func(i, j int) bool { return lessName(boards[i].Name, boards[j].Name) })
	}

	lookNames, err := s.lookNames(ctx, projectID)
	if err != nil {
		return nil, err
	}

	boardIDs := make([]string, len(boards))
	for i, b := range boards {
		boardIDs[i] = b.ID
	}
	buttonsByBoard, err := s.repos.LookBoards.GetButtonsByBoardIDs(ctx, boardIDs)
	if err != nil {
		return nil, err
	}

	var rows [][]any
	for _, b := range boards {
		for _, btn := range buttonsByBoard[b.ID] {
			rows = append(rows, []any{
				b.Name,
				strOrNil(btn.Label),
				blankIfEmpty(lookNames[btn.LookID]),
				btn.LayoutX,
				btn.LayoutY,
				intOrNil(btn.Width),
				intOrNil(btn.Height),
				strOrNil(btn.Color),
				b.DefaultFadeTime,
			})
		}
	}
	return &TableData{Columns: lookBoardEntriesTable().Columns, Rows: rows, ItemName: item}, nil
}

// --- CUE_LIST_DETAILS ------------------------------------------------------

func cueListDetailsTable() *Table {
	return &Table{
		ID:    TableCueListDetails,
		Label: "Cue List Details",
		Slug:  "cue-list-details",
		Scope: ScopeCueList,
		Columns: []Column{
			{"cueNumber", "Cue #"},
			{"name", "Name"},
			{"look", "Look"},
			{"fadeIn", "Fade in (s)"},
			{"fadeOut", "Fade out (s)"},
			{"followTime", "Follow time (s)"},
			{"easing", "Easing"},
			{"skip", "Skip"},
			{"notes", "Notes"},
		},
		build:    buildCueListDetails,
		estimate: countInScope(&models.Cue{}, "cue_list_id"),
	}
}

func buildCueListDetails(ctx context.Context, s *Service, projectID, scopeID string) (*TableData, error) {
	cl, err := s.repos.CueLists.FindByID(ctx, scopeID)
	if err != nil {
		return nil, err
	}
	if cl == nil || cl.ProjectID != projectID {
		return nil, fmt.Errorf("%w: cue list %s", ErrScopeNotFound, scopeID)
	}
	cues, err := s.repos.CueLists.GetCues(ctx, cl.ID)
	if err != nil {
		return nil, err
	}
	lookNames, err := s.lookNames(ctx, projectID)
	if err != nil {
		return nil, err
	}
	rows := make([][]any, 0, len(cues))
	for _, c := range cues {
		rows = append(rows, []any{
			c.CueNumber,
			c.Name,
			blankIfEmpty(lookNames[c.LookID]),
			c.FadeInTime,
			c.FadeOutTime,
			floatOrNil(c.FollowTime),
			strOrNil(c.EasingType),
			c.Skip,
			strOrNil(c.Notes),
		})
	}
	return &TableData{Columns: cueListDetailsTable().Columns, Rows: rows, ItemName: cl.Name}, nil
}

// --- helpers ---------------------------------------------------------------

// lookNames maps look ID to name for every look in the project.
func (s *Service) lookNames(ctx context.Context, projectID string) (map[string]string, error) {
	looks, err := s.repos.Looks.FindByProjectID(ctx, projectID)
	if err != nil {
		return nil, err
	}
	out := make(map[string]string, len(looks))
	for _, l := range looks {
		out[l.ID] = l.Name
	}
	return out, nil
}

func floatOrNil(p *float64) any {
	if p == nil {
		return nil
	}
	return *p
}

func intOrNil(p *int) any {
	if p == nil {
		return nil
	}
	return *p
}

func blankIfEmpty(s string) any {
	if s == "" {
		return nil
	}
	return s
}
