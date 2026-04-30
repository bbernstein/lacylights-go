package exporteos

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/bbernstein/lacylights-go/internal/database/models"
	"github.com/bbernstein/lacylights-go/internal/graphql/generated"
	importeos "github.com/bbernstein/lacylights-go/internal/services/import_eos"
)

// loadBundle reads a project from the repositories and translates it to the
// writer's input shapes. The returned bundle is suitable for direct emission
// in a fixed section order (see Service.Export). The collector receives any
// non-fatal warnings produced while assembling the bundle (e.g. fixtures
// skipped because they are unpatched).
func (s *Service) loadBundle(ctx context.Context, projectID string, collector *importeos.Collector) (*bundle, error) {
	proj, err := s.projectRepo.FindByID(ctx, projectID)
	if err != nil {
		return nil, fmt.Errorf("export_eos: project: %w", err)
	}
	if proj == nil {
		return nil, fmt.Errorf("export_eos: project %s not found", projectID)
	}

	instances, err := s.fixtureRepo.FindByProjectID(ctx, projectID)
	if err != nil {
		return nil, fmt.Errorf("export_eos: fixtures: %w", err)
	}
	// Sort instances by EOS channel (ProjectOrder when present, then by name).
	sort.SliceStable(instances, func(i, j int) bool {
		ai, bi := eosChannelFor(&instances[i]), eosChannelFor(&instances[j])
		if ai != bi {
			return ai < bi
		}
		return instances[i].Name < instances[j].Name
	})

	// Group instances by definition; load definition channels exactly once.
	defChannels := map[string][]models.ChannelDefinition{}
	defModels := map[string]*models.FixtureDefinition{}
	for i := range instances {
		def := instances[i].DefinitionID
		if _, ok := defChannels[def]; ok {
			continue
		}
		channels, err := s.fixtureRepo.GetDefinitionChannels(ctx, def)
		if err != nil {
			return nil, fmt.Errorf("export_eos: def channels: %w", err)
		}
		defChannels[def] = channels
		dm, err := s.fixtureRepo.FindDefinitionByID(ctx, def)
		if err != nil {
			return nil, fmt.Errorf("export_eos: def: %w", err)
		}
		defModels[def] = dm
	}

	personalityIDs, personalities := buildPersonalities(defModels, defChannels, collector)

	patch := buildPatch(instances, personalityIDs, collector)

	paramTypes := buildParamTable(collectChannelTypes(defChannels))

	// Cue lists (and palette synthesis from named lists).
	cueLists, err := s.cueListRepo.FindByProjectID(ctx, projectID)
	if err != nil {
		return nil, fmt.Errorf("export_eos: cue lists: %w", err)
	}
	sort.SliceStable(cueLists, func(i, j int) bool { return cueLists[i].Name < cueLists[j].Name })

	instanceStates, eosChannelByInstance := buildInstanceStates(instances, defChannels)

	palettes, regularLists, err := s.buildPaletteAndCueLists(ctx, cueLists, instanceStates, eosChannelByInstance, collector)
	if err != nil {
		return nil, err
	}

	// Sidecar: look boards + synthesized definitions.
	sidecar, err := s.buildSidecar(ctx, projectID, defModels, defChannels)
	if err != nil {
		return nil, err
	}

	groupsOut, err := s.loadGroups(ctx, projectID, eosChannelByInstance, collector)
	if err != nil {
		return nil, err
	}

	return &bundle{
		ProjectName:   proj.Name,
		ParamTypes:    paramTypes,
		Personalities: personalities,
		Patch:         patch,
		Palettes:      palettes,
		CueLists:      regularLists,
		Groups:        groupsOut,
		Sidecar:       sidecar,
	}, nil
}

// loadGroups returns the project's FixtureGroups as writer-shape rows,
// resolving members to EOS channel numbers. Empty groups (no resolvable
// members) are skipped with WarnExportEmptyGroupSkipped. Groups missing
// an EosNumber are auto-assigned the next available number and the
// updated number is persisted so subsequent exports stay stable.
func (s *Service) loadGroups(
	ctx context.Context,
	projectID string,
	eosByInstance map[string]int,
	collector *importeos.Collector,
) ([]GroupOut, error) {
	groups, err := s.fixtureGroupRepo.FindByProjectID(ctx, projectID)
	if err != nil {
		return nil, fmt.Errorf("export_eos: groups: %w", err)
	}
	// Sort: assigned EosNumber first (ascending), then nil-EosNumber
	// (alphabetic), then ID for tie-break. This makes auto-assignment
	// deterministic.
	sort.SliceStable(groups, func(i, j int) bool {
		ai, bi := groups[i].EosNumber, groups[j].EosNumber
		switch {
		case ai != nil && bi != nil:
			if *ai != *bi {
				return *ai < *bi
			}
		case ai != nil:
			return true
		case bi != nil:
			return false
		}
		if groups[i].Name != groups[j].Name {
			return groups[i].Name < groups[j].Name
		}
		return groups[i].ID < groups[j].ID
	})

	// Auto-assign EosNumbers. NOTE: this mutates DB state during a
	// nominally read-only export — intentional, since the alternative
	// (renumber on every export) breaks round-trip stability.
	maxAssigned := 0
	for _, g := range groups {
		if g.EosNumber != nil && *g.EosNumber > maxAssigned {
			maxAssigned = *g.EosNumber
		}
	}
	for i := range groups {
		if groups[i].EosNumber != nil {
			continue
		}
		maxAssigned++
		next := maxAssigned
		groups[i].EosNumber = &next
		if err := s.fixtureGroupRepo.Update(ctx, &groups[i]); err != nil {
			return nil, fmt.Errorf("export_eos: persist auto-assigned eos_number: %w", err)
		}
	}

	out := make([]GroupOut, 0, len(groups))
	for _, g := range groups {
		members, err := s.fixtureGroupRepo.GetMembers(ctx, g.ID)
		if err != nil {
			return nil, fmt.Errorf("export_eos: group members: %w", err)
		}
		channels := make([]int, 0, len(members))
		for _, m := range members {
			ch, ok := eosByInstance[m.FixtureID]
			if !ok {
				continue // unpatched fixture — silently exclude
			}
			channels = append(channels, ch)
		}
		if len(channels) == 0 {
			if collector != nil {
				collector.Add(importeos.WarnExportEmptyGroupSkipped, importeos.SeverityInfo,
					fmt.Sprintf("group %s has no patched members; skipped on export", g.Name),
					map[string]string{"groupId": g.ID, "groupName": g.Name})
			}
			continue
		}
		number := strconv.Itoa(*g.EosNumber)
		out = append(out, GroupOut{
			Number:   number,
			Label:    g.Name,
			Channels: channels,
		})
	}
	return out, nil
}

// eosChannelFor returns the EOS channel number for a fixture instance. We use
// ProjectOrder if present (importer sets it from EOS channel), otherwise fall
// back to start address.
func eosChannelFor(fi *models.FixtureInstance) int {
	if fi.ProjectOrder != nil {
		return *fi.ProjectOrder
	}
	return fi.StartChannel
}

// buildPersonalities assigns a stable personality ID to every distinct
// FixtureDefinition referenced by the project and emits the writer-shape rows.
// Definitions that have been deleted (FindDefinitionByID returned nil) are
// emitted as a placeholder "Unknown" personality and reported through the
// collector so the operator can see that the export is missing data.
func buildPersonalities(defs map[string]*models.FixtureDefinition,
	defChannels map[string][]models.ChannelDefinition,
	collector *importeos.Collector,
) (map[string]int, []PersonalityIn) {
	defIDs := make([]string, 0, len(defs))
	for id := range defs {
		defIDs = append(defIDs, id)
	}
	sort.Strings(defIDs)

	idMap := make(map[string]int, len(defIDs))
	out := make([]PersonalityIn, 0, len(defIDs))
	// 90001 is the base for LacyLights-synthesized personality IDs. EOS
	// library personality IDs in observed real-world showfiles top out in
	// the mid-five-digit range (OTBPA's largest is 23759), so 90001 sits
	// comfortably above that band — IDs below 90001 stay free for any
	// imported library personality plus hand-written test fixtures.
	const persIDBase = 90001
	for i, defID := range defIDs {
		persID := persIDBase + i
		idMap[defID] = persID
		dm := defs[defID]
		channels := append([]models.ChannelDefinition(nil), defChannels[defID]...)
		sort.Slice(channels, func(a, b int) bool { return channels[a].Offset < channels[b].Offset })
		persChans := make([]PersonalityChannelIn, 0, len(channels))
		for _, ch := range channels {
			meta := paramTypeMetaFor(generated.ChannelType(ch.Type))
			persChans = append(persChans, PersonalityChannelIn{
				ParamID:   meta.paramID,
				Size:      1,
				Offset:    ch.Offset + 1, // EOS PersChan offsets are 1-based
				HomeValue: ch.DefaultValue,
				Snap:      ch.FadeBehavior == "SNAP",
			})
		}
		footprint := len(channels)
		// FindDefinitionByID can legitimately return (nil, nil) when the
		// referenced definition has been deleted; emit a synthetic
		// "Unknown" personality rather than dereferencing nil. The
		// resulting EOS file still re-imports cleanly because the
		// channel-only fingerprint matches.
		manuf, model := "Unknown", "Unknown"
		if dm != nil {
			manuf = dm.Manufacturer
			model = dm.Model
		} else if collector != nil {
			collector.Add(importeos.WarnSynthesizedFixture, importeos.SeverityInfo,
				fmt.Sprintf("fixture definition %s missing; emitting placeholder personality", defID),
				map[string]string{"definitionId": defID, "personalityId": strconv.Itoa(persID)})
		}
		out = append(out, PersonalityIn{
			ID:        persID,
			Manuf:     manuf,
			Model:     model,
			Footprint: footprint,
			Channels:  persChans,
		})
	}
	return idMap, out
}

// buildPatch renders one $Patch line per fixture instance.
func buildPatch(instances []models.FixtureInstance, persIDs map[string]int, collector *importeos.Collector) []PatchEntryOut {
	out := make([]PatchEntryOut, 0, len(instances))
	for i := range instances {
		fi := &instances[i]
		// Fixtures with StartChannel <= 0 have no DMX address. Emitting "0"
		// would round-trip as ErrAddressUnpatched, silently dropping the
		// fixture on re-import — skip them at export time instead. The same
		// applies to a non-positive Universe (which would otherwise produce
		// a malformed dotted address). (LacyLights does not currently allow
		// patching at address 0 / universe 0, but a defensive guard avoids
		// data loss if either invariant ever slips.)
		if fi.StartChannel <= 0 || fi.Universe <= 0 || fi.StartChannel > 512 {
			if collector != nil {
				collector.Add(importeos.WarnUnpatchedInstance, importeos.SeverityInfo,
					fmt.Sprintf("fixture %q has no valid DMX address (universe=%d address=%d); skipped on export", fi.Name, fi.Universe, fi.StartChannel),
					map[string]string{"fixtureId": fi.ID})
			}
			continue
		}
		out = append(out, PatchEntryOut{
			Channel:       eosChannelFor(fi),
			Address:       formatEOSAddress(fi.Universe, fi.StartChannel),
			PersonalityID: persIDs[fi.DefinitionID],
			Label:         fi.Name,
		})
	}
	return out
}

// formatEOSAddress emits dotted form for universe > 1 and flat form for 1.
// Callers (buildPatch) pre-filter address <= 0 and universe <= 0.
func formatEOSAddress(universe, address int) string {
	if universe <= 1 {
		return strconv.Itoa(address)
	}
	return fmt.Sprintf("%d.%d", universe, address)
}

// collectChannelTypes returns the set of LacyLights ChannelTypes referenced by
// any channel definition in scope.
func collectChannelTypes(defChannels map[string][]models.ChannelDefinition) map[generated.ChannelType]struct{} {
	types := map[generated.ChannelType]struct{}{}
	for _, channels := range defChannels {
		for _, ch := range channels {
			types[generated.ChannelType(ch.Type)] = struct{}{}
		}
	}
	if len(types) == 0 {
		// Always emit at least the Intensity row to match how every Eos file begins.
		types[generated.ChannelTypeIntensity] = struct{}{}
	}
	return types
}

// instanceState ties an instance to its channel definitions for value lookup.
type instanceState struct {
	instance *models.FixtureInstance
	channels []models.ChannelDefinition
}

// buildInstanceStates returns lookup maps from instance ID and EOS channel.
// Unpatched instances are excluded so values referencing them in a Look don't
// render as a phantom $$ChanMove referencing a channel that doesn't appear
// in the $Patch section — that would corrupt the cue section on re-import.
// The exclusion criteria mirror buildPatch exactly so the two sets stay in
// sync.
func buildInstanceStates(
	instances []models.FixtureInstance,
	defChannels map[string][]models.ChannelDefinition,
) (map[string]*instanceState, map[string]int) {
	states := make(map[string]*instanceState, len(instances))
	eosByInstance := make(map[string]int, len(instances))
	for i := range instances {
		fi := &instances[i]
		if fi.StartChannel <= 0 || fi.Universe <= 0 || fi.StartChannel > 512 {
			continue
		}
		states[fi.ID] = &instanceState{instance: fi, channels: defChannels[fi.DefinitionID]}
		eosByInstance[fi.ID] = eosChannelFor(fi)
	}
	return states, eosByInstance
}

// paletteListNames maps the synthetic cue-list name the importer creates for
// each EOS palette/preset category to the corresponding writer "Kind". The
// keys reference exported constants in import_eos so a rename of those
// constants surfaces here as a build error rather than silent palette
// disappearance.
var paletteListNames = map[string]string{
	importeos.PaletteListNameColor:     "ColorPalette",
	importeos.PaletteListNameBeam:      "BeamPalette",
	importeos.PaletteListNameFocus:     "FocusPalette",
	importeos.PaletteListNameIntensity: "IntensPalette",
	importeos.PaletteListNamePreset:    "Preset",
}

func (s *Service) buildPaletteAndCueLists(
	ctx context.Context,
	cueLists []models.CueList,
	states map[string]*instanceState,
	eosByInstance map[string]int,
	collector *importeos.Collector,
) ([]PaletteOut, []CueListOut, error) {
	// TODO: batch the per-cue GetFixtureValues calls when project scale
	// warrants — currently O(cues) round-trips per export. Acceptable for
	// typical theatre showfiles; consider GetFixtureValuesByLookIDs.
	var palettes []PaletteOut
	var regular []CueListOut
	regularNumber := 1
	for _, cl := range cueLists {
		kind, isPalette := paletteListNames[cl.Name]
		cues, err := s.cueRepo.FindByCueListID(ctx, cl.ID)
		if err != nil {
			return nil, nil, fmt.Errorf("export_eos: cues for %s: %w", cl.ID, err)
		}
		sort.SliceStable(cues, func(i, j int) bool { return cues[i].CueNumber < cues[j].CueNumber })

		if isPalette {
			for _, cue := range cues {
				values, err := s.lookRepo.GetFixtureValues(ctx, cue.LookID)
				if err != nil {
					return nil, nil, fmt.Errorf("export_eos: palette values: %w", err)
				}
				chanMoves, paramMoves := renderMoves(values, states, eosByInstance, collector)
				palettes = append(palettes, PaletteOut{
					Kind:       kind,
					Number:     formatNumber(cue.CueNumber),
					Label:      cue.Name,
					ChanMoves:  chanMoves,
					ParamMoves: paramMoves,
				})
			}
			continue
		}

		out := CueListOut{
			Number: regularNumber,
			Label:  cl.Name,
		}
		regularNumber++
		for _, cue := range cues {
			values, err := s.lookRepo.GetFixtureValues(ctx, cue.LookID)
			if err != nil {
				return nil, nil, fmt.Errorf("export_eos: cue values: %w", err)
			}
			chanMoves, paramMoves := renderMoves(values, states, eosByInstance, collector)
			c := CueOut{
				Number:     formatNumber(cue.CueNumber),
				Label:      cueLabelFor(cue),
				UpFade:     cue.FadeInTime,
				DownFade:   cue.FadeOutTime,
				Follow:     cue.FollowTime,
				ChanMoves:  chanMoves,
				ParamMoves: paramMoves,
			}
			out.Cues = append(out.Cues, c)
		}
		regular = append(regular, out)
	}
	return palettes, regular, nil
}

// formatNumber renders a cue number trimming trailing zeros.
func formatNumber(n float64) string {
	s := strconv.FormatFloat(n, 'f', -1, 64)
	if !strings.Contains(s, ".") {
		return s
	}
	s = strings.TrimRight(s, "0")
	s = strings.TrimRight(s, ".")
	if s == "" {
		s = "0"
	}
	return s
}

// cueLabelFor strips the synthetic "Cue X - " prefix the importer adds, so the
// label round-trips cleanly. Falls back to the full name otherwise.
//
// Edge case: a user who deliberately names their cue exactly "Cue 3" (with no
// label) will see their name stripped on round-trip. The importer can't
// distinguish "user typed Cue 3" from "imported with no label", so this is
// inherent to the current data model. Adding a separate user-set-label
// field on models.Cue would resolve it; out of scope for Phase 1.
func cueLabelFor(cue models.Cue) string {
	prefix := fmt.Sprintf("Cue %s", formatNumber(cue.CueNumber))
	if rest, ok := strings.CutPrefix(cue.Name, prefix+" - "); ok {
		return rest
	}
	if cue.Name == prefix {
		return ""
	}
	return cue.Name
}

// renderMoves materializes a Look's fixture values into chan/param move lists.
// Channel-type INTENSITY values become $$ChanMove; all others become $$Param.
// Malformed FixtureValue.Channels JSON is reported as a LOOK_VALUES_INVALID
// warning and the offending row is skipped; the alternative (failing the
// whole export) would be more disruptive than dropping a single look's data.
func renderMoves(values []models.FixtureValue, states map[string]*instanceState, eosByInstance map[string]int, collector *importeos.Collector) ([]ChanMoveOut, []ParamMoveOut) {
	var chanMoves []ChanMoveOut
	type paramAcc struct {
		channel int
		values  []ParamValueOut
	}
	paramByChannel := map[int]*paramAcc{}

	// Stable iteration order: by EOS channel.
	type valueRow struct {
		eosChannel int
		fixtureID  string
		channels   []models.ChannelValue
	}
	rows := make([]valueRow, 0, len(values))
	for _, fv := range values {
		st := states[fv.FixtureID]
		if st == nil {
			continue
		}
		var chanList []models.ChannelValue
		if err := json.Unmarshal([]byte(fv.Channels), &chanList); err != nil {
			if collector != nil {
				collector.Add(importeos.WarnLookValuesInvalid, importeos.SeverityWarn,
					fmt.Sprintf("invalid FixtureValue.Channels JSON for fixture %s; skipping", fv.FixtureID),
					map[string]string{"fixtureId": fv.FixtureID, "lookId": fv.LookID, "err": err.Error()})
			}
			continue
		}
		if len(chanList) == 0 {
			continue
		}
		rows = append(rows, valueRow{
			eosChannel: eosByInstance[fv.FixtureID],
			fixtureID:  fv.FixtureID,
			channels:   chanList,
		})
	}
	sort.SliceStable(rows, func(i, j int) bool { return rows[i].eosChannel < rows[j].eosChannel })

	for _, row := range rows {
		st := states[row.fixtureID]
		// offset → channel def
		byOffset := map[int]models.ChannelDefinition{}
		for _, cd := range st.channels {
			byOffset[cd.Offset] = cd
		}
		for _, cv := range row.channels {
			cd, ok := byOffset[cv.Offset]
			if !ok {
				continue
			}
			ct := generated.ChannelType(cd.Type)
			if ct == generated.ChannelTypeIntensity {
				chanMoves = append(chanMoves, ChanMoveOut{Channel: row.eosChannel, Value: cv.Value})
				continue
			}
			meta := paramTypeMetaFor(ct)
			pa, ok := paramByChannel[row.eosChannel]
			if !ok {
				pa = &paramAcc{channel: row.eosChannel}
				paramByChannel[row.eosChannel] = pa
			}
			pa.values = append(pa.values, ParamValueOut{ParamID: meta.paramID, Value: cv.Value})
		}
	}

	channelKeys := make([]int, 0, len(paramByChannel))
	for k := range paramByChannel {
		channelKeys = append(channelKeys, k)
	}
	sort.Ints(channelKeys)
	paramMoves := make([]ParamMoveOut, 0, len(channelKeys))
	for _, k := range channelKeys {
		pa := paramByChannel[k]
		sort.SliceStable(pa.values, func(i, j int) bool { return pa.values[i].ParamID < pa.values[j].ParamID })
		paramMoves = append(paramMoves, ParamMoveOut{Channel: pa.channel, Values: pa.values})
	}
	return chanMoves, paramMoves
}

// buildSidecar collects round-trip metadata: look boards (with deterministic
// stable refIds) and synthesized definitions (defs that aren't built-in).
func (s *Service) buildSidecar(
	ctx context.Context,
	projectID string,
	defs map[string]*models.FixtureDefinition,
	defChannels map[string][]models.ChannelDefinition,
) (SidecarOut, error) {
	out := SidecarOut{Version: 1}

	boards, err := s.lookBoardRepo.FindByProjectID(ctx, projectID)
	if err != nil {
		return SidecarOut{}, fmt.Errorf("export_eos: look boards: %w", err)
	}
	sort.SliceStable(boards, func(i, j int) bool { return boards[i].ID < boards[j].ID })
	// TODO: batch the per-board GetButtons calls when project scale warrants
	// — currently O(boards) round-trips per export. Acceptable for typical
	// shows (low single-digit board counts).
	for _, b := range boards {
		buttons, err := s.lookBoardRepo.GetButtons(ctx, b.ID)
		if err != nil {
			return SidecarOut{}, fmt.Errorf("export_eos: look board buttons: %w", err)
		}
		sort.SliceStable(buttons, func(i, j int) bool { return buttons[i].ID < buttons[j].ID })
		var sb []importeos.SidecarLookBoardButton
		for _, btn := range buttons {
			color := ""
			if btn.Color != nil {
				color = *btn.Color
			}
			look, err := s.lookRepo.FindByID(ctx, btn.LookID)
			if err != nil {
				return SidecarOut{}, fmt.Errorf("export_eos: look for button: %w", err)
			}
			if look == nil {
				continue // dangling button reference; skip
			}
			sb = append(sb, importeos.SidecarLookBoardButton{
				LookRefID: look.RefID,
				X:         btn.LayoutX,
				Y:         btn.LayoutY,
				Color:     color,
			})
		}
		out.LookBoards = append(out.LookBoards, importeos.SidecarLookBoard{
			RefID:   b.RefID,
			Name:    b.Name,
			Buttons: sb,
		})
	}

	// fade_behavior: any FixtureInstance with non-default channel fade
	// behaviors gets a record. Sorted by RefID for determinism.
	instances, err := s.fixtureRepo.FindByProjectID(ctx, projectID)
	if err != nil {
		return SidecarOut{}, fmt.Errorf("export_eos: instances for fade_behavior: %w", err)
	}
	sort.SliceStable(instances, func(i, j int) bool { return instances[i].RefID < instances[j].RefID })
	for i := range instances {
		fi := &instances[i]
		channels, err := s.fixtureRepo.GetInstanceChannels(ctx, fi.ID)
		if err != nil {
			return SidecarOut{}, fmt.Errorf("export_eos: instance channels: %w", err)
		}
		var nonDefault []importeos.SidecarFadeBehaviorChannel
		for _, c := range channels {
			if c.FadeBehavior != "" && c.FadeBehavior != "FADE" {
				nonDefault = append(nonDefault, importeos.SidecarFadeBehaviorChannel{
					Offset:   c.Offset,
					Behavior: c.FadeBehavior,
				})
			}
		}
		if len(nonDefault) == 0 {
			continue
		}
		sort.Slice(nonDefault, func(a, b int) bool { return nonDefault[a].Offset < nonDefault[b].Offset })
		out.FadeBehaviors = append(out.FadeBehaviors, importeos.SidecarFadeBehavior{
			InstanceRefID: fi.RefID,
			Channels:      nonDefault,
		})
	}

	// Synthesized definitions: those not flagged IsBuiltIn.
	defIDs := make([]string, 0, len(defs))
	for id := range defs {
		defIDs = append(defIDs, id)
	}
	sort.Strings(defIDs)
	for _, id := range defIDs {
		dm := defs[id]
		if dm == nil || dm.IsBuiltIn {
			continue
		}
		out.SynthDefs = append(out.SynthDefs, importeos.SidecarSynthDef{
			DefRefID:           dm.RefID,
			Manufacturer:       dm.Manufacturer,
			Model:              dm.Model,
			ChannelFingerprint: channelFingerprint(defChannels[id]),
		})
	}

	return out, nil
}

// channelFingerprint produces a stable comma-separated list of EOS paramIDs in
// channel-offset order. Mirrors the importer's fingerprint format.
//
// TODO (Task 14c): when the sidecar is actually re-applied on import, add a
// round-trip test that imports a synthesized definition, exports it, and
// verifies the recomputed fingerprint matches what the importer wrote into
// the sidecar — currently this alignment is only enforced by convention.
func channelFingerprint(channels []models.ChannelDefinition) string {
	sorted := append([]models.ChannelDefinition(nil), channels...)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].Offset < sorted[j].Offset })
	parts := make([]string, len(sorted))
	for i, ch := range sorted {
		meta := paramTypeMetaFor(generated.ChannelType(ch.Type))
		parts[i] = strconv.Itoa(meta.paramID)
	}
	return strings.Join(parts, ",")
}
