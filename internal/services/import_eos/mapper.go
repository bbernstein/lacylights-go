package importeos

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strconv"

	"github.com/bbernstein/lacylights-go/internal/database/models"
	"github.com/bbernstein/lacylights-go/internal/database/repositories"
	"github.com/bbernstein/lacylights-go/internal/graphql/generated"
)

// Cue list names used by the importer to file synthesized palette/preset
// looks. Exported so the exporter can recognize the same lists when
// translating LacyLights state back to Eos ASCII; keeping the strings in one
// place avoids silent drift between the two halves of the round-trip.
const (
	PaletteListNameColor     = "Color Palettes"
	PaletteListNameBeam      = "Beam Palettes"
	PaletteListNameFocus     = "Focus Palettes"
	PaletteListNameIntensity = "Intensity Palettes"
	PaletteListNamePreset    = "Presets"
)

// Mapper applies a parsed Show + Sidecar to LacyLights repos.
type Mapper struct {
	projectRepo   *repositories.ProjectRepository
	fixtureRepo   *repositories.FixtureRepository
	lookRepo      *repositories.LookRepository
	cueListRepo   *repositories.CueListRepository
	cueRepo       *repositories.CueRepository
	lookBoardRepo *repositories.LookBoardRepository
}

// NewMapper constructs a Mapper.
func NewMapper(p *repositories.ProjectRepository, f *repositories.FixtureRepository,
	l *repositories.LookRepository, cl *repositories.CueListRepository,
	c *repositories.CueRepository, lb *repositories.LookBoardRepository) *Mapper {
	return &Mapper{p, f, l, cl, c, lb}
}

// Apply maps the parsed show into the database.
func (m *Mapper) Apply(ctx context.Context, show *Show, sidecar Sidecar, opts Options, warn *Collector) (*Result, error) {
	// Initialize SynthesizedDefinitionIDs as a non-nil empty slice so
	// callers (including the GraphQL resolver, which marshals it to a
	// non-null list) never see a nil. Without this, append() leaves the
	// field nil when no definitions were synthesized.
	res := &Result{SynthesizedDefinitionIDs: []string{}}

	projectID, err := m.resolveProject(ctx, show, opts)
	if err != nil {
		return nil, fmt.Errorf("resolve project: %w", err)
	}
	res.ProjectID = projectID

	table := NewParamTable(show.ParamTypes)

	matcher := NewMatcher(newRepoAdapter(m.fixtureRepo), table)
	persToDefID := make(map[int]string, len(show.Personalities))
	persToChannels := make(map[int][]models.ChannelDefinition, len(show.Personalities))
	for _, pers := range show.Personalities {
		mr, mWarns, err := matcher.Match(ctx, pers)
		if err != nil {
			return nil, fmt.Errorf("match personality %d: %w", pers.ID, err)
		}
		for _, w := range mWarns {
			warn.Add(w.Code, w.Severity, w.Message, w.Context)
		}
		var defID string
		var channels []models.ChannelDefinition
		if mr.ExistingDefinitionID != "" {
			defID = mr.ExistingDefinitionID
			channels, err = m.fixtureRepo.GetDefinitionChannels(ctx, defID)
			if err != nil {
				return nil, fmt.Errorf("load existing def channels: %w", err)
			}
		} else {
			if err := m.fixtureRepo.CreateDefinitionWithChannels(ctx, mr.SynthesizedDef, mr.SynthesizedChannels); err != nil {
				return nil, fmt.Errorf("create synth def: %w", err)
			}
			defID = mr.SynthesizedDef.ID
			channels = mr.SynthesizedChannels
			res.SynthesizedDefinitionIDs = append(res.SynthesizedDefinitionIDs, defID)
		}
		persToDefID[pers.ID] = defID
		persToChannels[pers.ID] = channels
		res.FixtureDefinitionsCount++
	}

	eosChannelToInstanceID := make(map[int]string, len(show.Patch))
	type addrKey struct{ u, a int }
	seenAddr := make(map[addrKey]int, len(show.Patch))
	for _, pe := range show.Patch {
		defID, ok := persToDefID[pe.PersonalityID]
		if !ok {
			return nil, fmt.Errorf("$Patch %d references undeclared personality %d", pe.Channel, pe.PersonalityID)
		}
		universe, address, err := NormalizeAddress(pe.AddressRaw)
		if err != nil {
			if errors.Is(err, ErrAddressUnpatched) {
				warn.Add(WarnUnpatchedChannel, SeverityInfo,
					fmt.Sprintf("channel %d has no DMX address (skipping)", pe.Channel),
					map[string]string{"channel": strconv.Itoa(pe.Channel)})
				continue
			}
			return nil, err
		}
		key := addrKey{u: universe, a: address}
		if prevCh, dup := seenAddr[key]; dup {
			// EOS supports multi-patching (two logical channels at the
			// same DMX address). We treat the second occurrence as a
			// soft conflict: warn so the operator can review, but
			// continue importing rather than aborting the whole file.
			warn.Add(WarnAddressConflict, SeverityWarn,
				fmt.Sprintf("address conflict at universe %d address %d: channels %d and %d",
					universe, address, prevCh, pe.Channel),
				map[string]string{
					"universe": strconv.Itoa(universe),
					"address":  strconv.Itoa(address),
					"prevCh":   strconv.Itoa(prevCh),
					"newCh":    strconv.Itoa(pe.Channel),
				})
		}
		seenAddr[key] = pe.Channel
		label := pe.Label
		if pe.UnicodeText != nil {
			label = *pe.UnicodeText
		}
		if label == "" {
			label = fmt.Sprintf("Channel %d", pe.Channel)
		}
		order := pe.Channel
		instance := &models.FixtureInstance{
			ProjectID:    projectID,
			DefinitionID: defID,
			Name:         label,
			Universe:     universe,
			StartChannel: address,
			ProjectOrder: &order,
		}
		instanceChannels := createInstanceChannelsFor(persToChannels[pe.PersonalityID])
		if err := m.fixtureRepo.CreateWithChannels(ctx, instance, instanceChannels); err != nil {
			return nil, fmt.Errorf("create instance: %w", err)
		}
		eosChannelToInstanceID[pe.Channel] = instance.ID
		res.FixtureInstancesCount++
	}

	if err := m.applyPalettes(ctx, projectID, show, eosChannelToInstanceID, persToChannels, table, &res.LooksCount, &res.CueListsCount); err != nil {
		return nil, err
	}
	if err := m.applyCues(ctx, projectID, show, eosChannelToInstanceID, persToChannels, table, &res.LooksCount, &res.CueListsCount, &res.CuesCount); err != nil {
		return nil, err
	}
	if err := m.applyGroups(ctx, projectID, show, &res.GroupsCount, warn); err != nil {
		return nil, err
	}
	if err := m.applySidecar(ctx, projectID, sidecar, warn); err != nil {
		return nil, err
	}

	res.Warnings = warn.All()
	return res, nil
}

func (m *Mapper) resolveProject(ctx context.Context, show *Show, opts Options) (string, error) {
	if opts.TargetProjectID != nil {
		// Verify the target still exists; the request may have been
		// queued before the project was deleted. Without this guard we
		// would create fixtures with an orphaned ProjectID FK.
		existing, err := m.projectRepo.FindByID(ctx, *opts.TargetProjectID)
		if err != nil {
			return "", fmt.Errorf("load target project: %w", err)
		}
		if existing == nil {
			return "", fmt.Errorf("target project %s not found", *opts.TargetProjectID)
		}
		return *opts.TargetProjectID, nil
	}
	name := show.Title
	if opts.NewProjectName != nil {
		name = *opts.NewProjectName
	}
	if name == "" {
		name = "Imported Eos Show"
	}
	p := &models.Project{Name: name}
	if opts.GroupID != nil {
		p.GroupID = opts.GroupID
	}
	if err := m.projectRepo.Create(ctx, p); err != nil {
		return "", err
	}
	return p.ID, nil
}

// createInstanceChannelsFor copies definition channels to instance channels.
func createInstanceChannelsFor(defChannels []models.ChannelDefinition) []models.InstanceChannel {
	out := make([]models.InstanceChannel, 0, len(defChannels))
	for _, ch := range defChannels {
		out = append(out, models.InstanceChannel{
			Offset:       ch.Offset,
			Name:         ch.Name,
			Type:         ch.Type,
			MinValue:     ch.MinValue,
			MaxValue:     ch.MaxValue,
			DefaultValue: ch.DefaultValue,
			FadeBehavior: ch.FadeBehavior,
			IsDiscrete:   ch.IsDiscrete,
		})
	}
	return out
}

// instanceState holds the loaded fixture instance + its definition channels (for offset/type lookup).
type instanceState struct {
	instance *models.FixtureInstance
	defChans []models.ChannelDefinition
}

// loadInstanceStates returns a map keyed by EOS channel number.
func (m *Mapper) loadInstanceStates(ctx context.Context, projectID string,
	eosChannelToInstanceID map[int]string,
	persToChannels map[int][]models.ChannelDefinition,
	patch []PatchEntry,
) (map[int]*instanceState, error) {
	out := make(map[int]*instanceState, len(eosChannelToInstanceID))
	persByChannel := make(map[int]int, len(patch))
	for _, pe := range patch {
		persByChannel[pe.Channel] = pe.PersonalityID
	}
	for eosCh, instID := range eosChannelToInstanceID {
		fi, err := m.fixtureRepo.FindByID(ctx, instID)
		if err != nil {
			return nil, err
		}
		if fi == nil {
			continue
		}
		out[eosCh] = &instanceState{
			instance: fi,
			defChans: persToChannels[persByChannel[eosCh]],
		}
	}
	return out, nil
}

// findOffsetForType returns the channel offset within an instance whose type matches ct.
// Returns -1 if not found.
func findOffsetForType(defChans []models.ChannelDefinition, ct generated.ChannelType) int {
	want := string(ct)
	for _, c := range defChans {
		if c.Type == want {
			return c.Offset
		}
	}
	return -1
}

// buildLookValues converts EOS chan moves and param moves to FixtureValues
// for a Look. The output is sorted by FixtureID, and each fixture's channel
// list is sorted by Offset, so two imports of the same showfile produce
// byte-identical FixtureValue.Channels JSON. Without these sorts, Go's map
// randomisation would produce different DB rows on each run, breaking
// round-trip diff tests and surprising clients that compare snapshots.
//
// Returns an error if any per-instance channel slice fails to marshal —
// unreachable today (ChannelValue is two ints) but propagated as a value
// rather than a panic so callers (including the GraphQL resolver) get a
// structured error instead of a server crash.
func buildLookValues(
	chanLevels map[int]int,
	paramLevels map[int]map[int]int,
	states map[int]*instanceState,
	table *ParamTable,
) ([]models.FixtureValue, error) {
	type fvAcc struct {
		fixtureID string
		channels  []models.ChannelValue
	}
	byInstance := map[string]*fvAcc{}
	add := func(instID string, cv models.ChannelValue) {
		acc, ok := byInstance[instID]
		if !ok {
			acc = &fvAcc{fixtureID: instID}
			byInstance[instID] = acc
		}
		acc.channels = append(acc.channels, cv)
	}

	// Walk the input maps in deterministic key order so duplicate
	// (instance, offset) collisions resolve identically across runs.
	chanKeys := make([]int, 0, len(chanLevels))
	for k := range chanLevels {
		chanKeys = append(chanKeys, k)
	}
	sort.Ints(chanKeys)
	for _, eosCh := range chanKeys {
		st := states[eosCh]
		if st == nil {
			continue
		}
		offset := findOffsetForType(st.defChans, generated.ChannelTypeIntensity)
		if offset < 0 {
			continue
		}
		add(st.instance.ID, models.ChannelValue{Offset: offset, Value: chanLevels[eosCh]})
	}

	paramKeys := make([]int, 0, len(paramLevels))
	for k := range paramLevels {
		paramKeys = append(paramKeys, k)
	}
	sort.Ints(paramKeys)
	for _, eosCh := range paramKeys {
		st := states[eosCh]
		if st == nil {
			continue
		}
		params := paramLevels[eosCh]
		innerKeys := make([]int, 0, len(params))
		for k := range params {
			innerKeys = append(innerKeys, k)
		}
		sort.Ints(innerKeys)
		for _, paramID := range innerKeys {
			ct := table.ChannelType(paramID)
			offset := findOffsetForType(st.defChans, ct)
			if offset < 0 {
				continue
			}
			add(st.instance.ID, models.ChannelValue{Offset: offset, Value: params[paramID]})
		}
	}

	instIDs := make([]string, 0, len(byInstance))
	for id := range byInstance {
		instIDs = append(instIDs, id)
	}
	sort.Strings(instIDs)
	out := make([]models.FixtureValue, 0, len(byInstance))
	for _, id := range instIDs {
		acc := byInstance[id]
		sort.Slice(acc.channels, func(i, j int) bool { return acc.channels[i].Offset < acc.channels[j].Offset })
		channelsJSON, err := json.Marshal(acc.channels)
		if err != nil {
			return nil, fmt.Errorf("import_eos: marshal ChannelValue for fixture %s: %w", acc.fixtureID, err)
		}
		out = append(out, models.FixtureValue{
			FixtureID: acc.fixtureID,
			Channels:  string(channelsJSON),
		})
	}
	return out, nil
}

func (m *Mapper) applyPalettes(ctx context.Context, projectID string, show *Show,
	eosChannelToInstanceID map[int]string,
	persToChannels map[int][]models.ChannelDefinition,
	table *ParamTable,
	looksCount, cueListsCount *int,
) error {
	states, err := m.loadInstanceStates(ctx, projectID, eosChannelToInstanceID, persToChannels, show.Patch)
	if err != nil {
		return err
	}
	groups := []struct {
		name     string
		palettes []Palette
	}{
		{PaletteListNameColor, show.ColorPalettes},
		{PaletteListNameBeam, show.BeamPalettes},
		{PaletteListNameFocus, show.FocusPalettes},
		{PaletteListNameIntensity, show.IntensPalettes},
		{PaletteListNamePreset, show.Presets},
	}
	for _, g := range groups {
		if len(g.palettes) == 0 {
			continue
		}
		cl := &models.CueList{ProjectID: projectID, Name: g.name}
		if err := m.cueListRepo.Create(ctx, cl); err != nil {
			return fmt.Errorf("create palette cue list %s: %w", g.name, err)
		}
		*cueListsCount++
		for i, pal := range g.palettes {
			label := pal.Label
			if pal.UnicodeText != nil {
				label = *pal.UnicodeText
			}
			look := &models.Look{
				ProjectID: projectID,
				Name:      fmt.Sprintf("%s %s - %s", g.name, pal.Number, label),
			}
			chanLevels := map[int]int{}
			for _, mv := range pal.ChanMoves {
				chanLevels[mv.Channel] = mv.Value
			}
			paramLevels := map[int]map[int]int{}
			for _, pm := range pal.ParamMoves {
				ps, ok := paramLevels[pm.Channel]
				if !ok {
					ps = map[int]int{}
					paramLevels[pm.Channel] = ps
				}
				for _, v := range pm.Values {
					ps[v.ParamID] = v.Value
				}
			}
			values, err := buildLookValues(chanLevels, paramLevels, states, table)
			if err != nil {
				return err
			}
			if err := m.lookRepo.CreateWithFixtureValues(ctx, look, values); err != nil {
				return fmt.Errorf("create palette look: %w", err)
			}
			*looksCount++
			cueNum, _ := strconv.ParseFloat(pal.Number, 64)
			if cueNum == 0 {
				cueNum = float64(i + 1)
			}
			cue := &models.Cue{
				CueListID: cl.ID,
				LookID:    look.ID,
				Name:      look.Name,
				CueNumber: cueNum,
			}
			if err := m.cueRepo.Create(ctx, cue); err != nil {
				return err
			}
		}
	}
	return nil
}

func (m *Mapper) applyCues(ctx context.Context, projectID string, show *Show,
	eosChannelToInstanceID map[int]string,
	persToChannels map[int][]models.ChannelDefinition,
	table *ParamTable,
	looksCount, cueListsCount, cuesCount *int,
) error {
	states, err := m.loadInstanceStates(ctx, projectID, eosChannelToInstanceID, persToChannels, show.Patch)
	if err != nil {
		return err
	}
	tracker := NewTracker()
	for _, cl := range show.CueLists {
		listName := cl.Label
		if listName == "" {
			listName = fmt.Sprintf("Cue List %d", cl.Number)
		}
		dbList := &models.CueList{ProjectID: projectID, Name: listName}
		if err := m.cueListRepo.Create(ctx, dbList); err != nil {
			return err
		}
		*cueListsCount++

		snaps := tracker.ResolveCueList(cl.Cues, show.ColorPalettes, show.BeamPalettes,
			show.FocusPalettes, show.IntensPalettes, show.Presets)
		for _, snap := range snaps {
			cueLabel := snap.Label
			if snap.UnicodeText != nil {
				cueLabel = *snap.UnicodeText
			}
			cueName := fmt.Sprintf("Cue %s", snap.CueNumber)
			if snap.CuePart > 0 {
				cueName = fmt.Sprintf("Cue %s Part %d", snap.CueNumber, snap.CuePart)
			}
			if cueLabel != "" {
				cueName = cueName + " - " + cueLabel
			}
			look := &models.Look{ProjectID: projectID, Name: cueName}
			values, err := buildLookValues(snap.ChannelLevels, snap.ParamLevels, states, table)
			if err != nil {
				return err
			}
			if err := m.lookRepo.CreateWithFixtureValues(ctx, look, values); err != nil {
				return err
			}
			*looksCount++

			cueNum, _ := strconv.ParseFloat(snap.CueNumber, 64)
			cue := &models.Cue{
				CueListID:   dbList.ID,
				LookID:      look.ID,
				Name:        cueName,
				CueNumber:   cueNum,
				FadeInTime:  snap.UpFade,
				FadeOutTime: snap.DownFade,
				FollowTime:  snap.Follow,
			}
			if err := m.cueRepo.Create(ctx, cue); err != nil {
				return err
			}
			*cuesCount++
		}
	}
	return nil
}

func (m *Mapper) applyGroups(_ context.Context, _ string, show *Show, _ *int, warn *Collector) error {
	// Group persistence is part of Task 14c. Held back until then — but
	// surface a warning when the file actually contained groups, so a
	// caller doesn't silently lose that data.
	if len(show.Groups) > 0 {
		warn.Add(WarnGroupsSkipped, SeverityInfo,
			fmt.Sprintf("dropped %d $Group block(s) — Task 14c will land FixtureGroup persistence",
				len(show.Groups)),
			map[string]string{"count": strconv.Itoa(len(show.Groups))})
	}
	return nil
}

func (m *Mapper) applySidecar(_ context.Context, _ string, sc Sidecar, warn *Collector) error {
	// Sidecar application is part of Task 14c. As with applyGroups, warn
	// rather than silently swallow the parsed sidecar contents.
	if len(sc.LookBoards) > 0 || len(sc.FadeBehaviors) > 0 || len(sc.SynthDefs) > 0 {
		warn.Add(WarnSidecarUnresolved, SeverityInfo,
			"sidecar metadata parsed but not applied (Task 14c)",
			map[string]string{
				"lookBoards":    strconv.Itoa(len(sc.LookBoards)),
				"fadeBehaviors": strconv.Itoa(len(sc.FadeBehaviors)),
				"synthDefs":     strconv.Itoa(len(sc.SynthDefs)),
			})
	}
	return nil
}
