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

	personalityIDs, personalities := buildPersonalities(defModels, defChannels)

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

	return &bundle{
		ProjectName:   proj.Name,
		ParamTypes:    paramTypes,
		Personalities: personalities,
		Patch:         patch,
		Palettes:      palettes,
		CueLists:      regularLists,
		Groups:        nil, // FixtureGroup model not yet present (Task 14c)
		Sidecar:       sidecar,
	}, nil
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
func buildPersonalities(defs map[string]*models.FixtureDefinition,
	defChannels map[string][]models.ChannelDefinition,
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
	// the mid-five-digit range (OTBPA's largest is 23759); starting at
	// 90001 sits comfortably above that band, avoiding accidental
	// collisions when a user imports a real showfile and then re-exports
	// it. Lower five-digit values stay free for hand-written test fixtures.
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
		// fixture on re-import — skip them at export time instead. (LacyLights
		// does not currently allow patching at address 0, but a defensive
		// guard avoids data loss if that invariant ever slips.)
		if fi.StartChannel <= 0 {
			if collector != nil {
				collector.Add(importeos.WarnUnpatchedInstance, importeos.SeverityInfo,
					fmt.Sprintf("fixture %q has no DMX address; skipped on export", fi.Name),
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

// formatEOSAddress emits the dotted form ("1.512") for non-trivial universes,
// and a flat form for universe <= 1 (matches what the parser accepts).
// Callers must pre-filter out address <= 0; emitting "0" here would re-parse
// as ErrAddressUnpatched on round-trip. Universe 0 collapses to the flat
// form for the same reason — there is no real "universe 0" in EOS, but a
// fixture stored that way can still produce a valid flat address.
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
// Unpatched instances (StartChannel <= 0) are excluded so that values
// referencing them in a Look don't render as a phantom $$ChanMove on
// channel 0 — channel 0 is invalid in EOS and would corrupt the cue
// section on re-import.
func buildInstanceStates(
	instances []models.FixtureInstance,
	defChannels map[string][]models.ChannelDefinition,
) (map[string]*instanceState, map[string]int) {
	states := make(map[string]*instanceState, len(instances))
	eosByInstance := make(map[string]int, len(instances))
	for i := range instances {
		fi := &instances[i]
		if fi.StartChannel <= 0 {
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
			sb = append(sb, importeos.SidecarLookBoardButton{
				LookRefID: btn.LookID,
				X:         btn.LayoutX,
				Y:         btn.LayoutY,
				Color:     color,
			})
		}
		out.LookBoards = append(out.LookBoards, importeos.SidecarLookBoard{
			RefID:   b.ID,
			Name:    b.Name,
			Buttons: sb,
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
			DefRefID:           id,
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
