package importqlc

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"unicode"

	"github.com/bbernstein/lacylights-go/internal/database/models"
	"github.com/bbernstein/lacylights-go/internal/database/repositories"
)

// QLC+ synthesizes these models itself instead of loading a .qxf
// definition (Fixture::loadXML: KXMLFixtureGeneric / KXMLFixtureRGBPanel).
const (
	qlcGenericModel  = "Generic"
	qlcRGBPanelModel = "RGBPanel"
)

// resolvedFixture is the definition, mode and channel layout chosen for one
// QLC+ fixture.
type resolvedFixture struct {
	def      *models.FixtureDefinition
	modeName *string
	channels []models.InstanceChannel
}

// definitionResolver maps QLC+ fixtures (manufacturer, model, mode, channel
// count) onto LacyLights fixture definitions. It prefers an existing
// definition with a mode of the right size and otherwise creates a
// definition so the patch and every scene value survive the import.
type definitionResolver struct {
	repo *repositories.FixtureRepository

	cache   map[string]*resolvedFixture
	allDefs []models.FixtureDefinition
	loaded  bool

	created  int
	warnings []string
}

func newDefinitionResolver(repo *repositories.FixtureRepository) *definitionResolver {
	return &definitionResolver{repo: repo, cache: map[string]*resolvedFixture{}}
}

func (r *definitionResolver) resolve(ctx context.Context, fx Fixture, channels int) (*resolvedFixture, error) {
	key := strings.Join([]string{fx.Manufacturer, fx.Model, fx.Mode, strconv.Itoa(channels)}, "\x00")
	if res, ok := r.cache[key]; ok {
		return res, nil
	}
	res, err := r.resolveUncached(ctx, fx, channels)
	if err != nil {
		return nil, err
	}
	r.cache[key] = res
	return res, nil
}

func (r *definitionResolver) resolveUncached(ctx context.Context, fx Fixture, channels int) (*resolvedFixture, error) {
	qlcSynthesized := fx.Model == qlcGenericModel || fx.Model == qlcRGBPanelModel

	// 1. An existing library/user definition with a mode of the right size.
	if !qlcSynthesized {
		res, err := r.findLibraryMatch(ctx, fx, channels)
		if err != nil || res != nil {
			return res, err
		}
	}

	// 2. A definition created by a previous QLC+ import, or a new one.
	names := candidateModelNames(fx, channels)
	for _, name := range names {
		def, err := r.repo.FindDefinitionByManufacturerModel(ctx, fx.Manufacturer, name)
		if err != nil {
			return nil, err
		}
		if def == nil {
			return r.synthesize(ctx, fx, name, channels)
		}
		res, err := r.compatible(ctx, def, fx.Mode, channels)
		if err != nil || res != nil {
			return res, err
		}
	}
	return r.synthesize(ctx, fx, names[len(names)-1], channels)
}

// candidateModelNames lists the model names under which a definition for fx
// is looked up and, failing that, created (first free name). QLC+ builds
// generic dimmers and RGB panels from the channel count, so those get one
// size-specific name, e.g. "Dimmer (6 ch)" or "RGB Panel (GRB, 6 ch)".
func candidateModelNames(fx Fixture, channels int) []string {
	switch fx.Model {
	case qlcGenericModel:
		return []string{fmt.Sprintf("Dimmer (%d ch)", channels)}
	case qlcRGBPanelModel:
		return []string{fmt.Sprintf("RGB Panel (%s, %d ch)", fx.Mode, channels)}
	}
	return []string{
		fx.Model,
		fmt.Sprintf("%s (%s)", fx.Model, fx.Mode),
		fmt.Sprintf("%s (%s, %d ch)", fx.Model, fx.Mode, channels),
	}
}

// findLibraryMatch looks for an existing definition by exact name, then by
// normalized name (case, spacing and punctuation ignored), then by an
// unambiguous manufacturer alias (QLC+ "Chauvet" vs library "Chauvet DJ").
func (r *definitionResolver) findLibraryMatch(ctx context.Context, fx Fixture, channels int) (*resolvedFixture, error) {
	def, err := r.repo.FindDefinitionByManufacturerModel(ctx, fx.Manufacturer, fx.Model)
	if err != nil {
		return nil, err
	}
	if def != nil {
		if res, err := r.compatible(ctx, def, fx.Mode, channels); err != nil || res != nil {
			return res, err
		}
	}

	if err := r.loadAllDefinitions(ctx); err != nil {
		return nil, err
	}
	wantMfr, wantModel := normalize(fx.Manufacturer), normalize(fx.Model)
	if wantModel == "" {
		return nil, nil
	}

	var aliasMatches []*resolvedFixture
	for i := range r.allDefs {
		cand := &r.allDefs[i]
		if def != nil && cand.ID == def.ID {
			continue
		}
		if normalize(cand.Model) != wantModel {
			continue
		}
		candMfr := normalize(cand.Manufacturer)
		exact := candMfr == wantMfr
		alias := !exact && wantMfr != "" && (strings.HasPrefix(candMfr, wantMfr) || strings.HasPrefix(wantMfr, candMfr))
		if !exact && !alias {
			continue
		}
		res, err := r.compatible(ctx, cand, fx.Mode, channels)
		if err != nil {
			return nil, err
		}
		if res == nil {
			continue
		}
		if exact {
			return res, nil
		}
		aliasMatches = append(aliasMatches, res)
	}
	if len(aliasMatches) == 1 {
		m := aliasMatches[0]
		r.warnings = append(r.warnings, fmt.Sprintf(
			"QLC+ fixture %q %q matched to LacyLights fixture %q %q (%s mode); verify the channel layout",
			fx.Manufacturer, fx.Model, m.def.Manufacturer, m.def.Model, deref(m.modeName)))
		return m, nil
	}
	return nil, nil
}

func (r *definitionResolver) loadAllDefinitions(ctx context.Context) error {
	if r.loaded {
		return nil
	}
	defs, err := r.repo.FindAllDefinitions(ctx)
	if err != nil {
		return err
	}
	r.allDefs = defs
	r.loaded = true
	return nil
}

// compatible returns the definition's channel layout for a mode with the
// given channel count, preferring a mode whose name matches modeName. It
// returns nil when the definition has no such mode.
func (r *definitionResolver) compatible(ctx context.Context, def *models.FixtureDefinition, modeName string, channels int) (*resolvedFixture, error) {
	modes, err := r.repo.GetDefinitionModes(ctx, def.ID)
	if err != nil {
		return nil, err
	}
	defChannels, err := r.repo.GetDefinitionChannels(ctx, def.ID)
	if err != nil {
		return nil, err
	}

	if len(modes) == 0 {
		// Definitions without modes (e.g. hand-made ones) use every channel.
		if len(defChannels) != channels {
			return nil, nil
		}
		return &resolvedFixture{def: def, channels: instanceChannelsFromDefinition(defChannels)}, nil
	}

	var chosen *models.FixtureMode
	for i := range modes {
		if modes[i].ChannelCount == channels && normalize(modes[i].Name) == normalize(modeName) {
			chosen = &modes[i]
			break
		}
	}
	if chosen == nil {
		for i := range modes {
			if modes[i].ChannelCount == channels {
				chosen = &modes[i]
				break
			}
		}
	}
	if chosen == nil {
		return nil, nil
	}

	modeChannels, err := r.repo.GetModeChannels(ctx, chosen.ID)
	if err != nil {
		return nil, err
	}
	byID := make(map[string]models.ChannelDefinition, len(defChannels))
	for _, ch := range defChannels {
		byID[ch.ID] = ch
	}
	out := make([]models.InstanceChannel, 0, len(modeChannels))
	for _, mc := range modeChannels {
		ch, ok := byID[mc.ChannelID]
		if !ok {
			continue
		}
		ic := instanceChannelsFromDefinition([]models.ChannelDefinition{ch})[0]
		ic.Offset = mc.Offset
		out = append(out, ic)
	}
	if len(out) != channels {
		// Broken mode (dangling channel references); don't trust it.
		return nil, nil
	}
	name := chosen.Name
	return &resolvedFixture{def: def, modeName: &name, channels: out}, nil
}

// synthesize creates a definition (with a single mode) for a QLC+ fixture
// that has no LacyLights equivalent.
func (r *definitionResolver) synthesize(ctx context.Context, fx Fixture, model string, channels int) (*resolvedFixture, error) {
	defType, chans, exact := synthesizedLayout(fx, channels)
	def := &models.FixtureDefinition{
		Manufacturer: fx.Manufacturer,
		Model:        model,
		Type:         defType,
		IsBuiltIn:    false,
	}
	if err := r.repo.CreateDefinitionWithChannels(ctx, def, chans); err != nil {
		return nil, fmt.Errorf("create fixture definition %s %s: %w", def.Manufacturer, def.Model, err)
	}
	modeName := fx.Mode
	if modeName == "" {
		modeName = fmt.Sprintf("%d Channel", channels)
	}
	mode := &models.FixtureMode{Name: modeName, ChannelCount: channels, DefinitionID: def.ID}
	if err := r.repo.CreateMode(ctx, mode); err != nil {
		return nil, fmt.Errorf("create fixture mode for %s %s: %w", def.Manufacturer, def.Model, err)
	}
	modeChannels := make([]models.ModeChannel, len(chans))
	for i, ch := range chans {
		modeChannels[i] = models.ModeChannel{ModeID: mode.ID, ChannelID: ch.ID, Offset: ch.Offset}
	}
	if err := r.repo.CreateModeChannels(ctx, modeChannels); err != nil {
		return nil, fmt.Errorf("create fixture mode channels for %s %s: %w", def.Manufacturer, def.Model, err)
	}
	r.created++
	r.allDefs = append(r.allDefs, *def)

	if !exact {
		r.warnings = append(r.warnings, fmt.Sprintf(
			"No LacyLights fixture definition matches QLC+ fixture %q %q (%s, %d channels); created definition %q %q. Check its channel types",
			fx.Manufacturer, fx.Model, modeName, channels, def.Manufacturer, def.Model))
	}
	return &resolvedFixture{def: def, modeName: &modeName, channels: instanceChannelsFromDefinition(chans)}, nil
}

// synthesizedLayout returns the definition type and channels for a QLC+
// fixture without a library match. exact reports whether the layout is
// known (QLC+ generic dimmers and RGB panels, which QLC+ builds itself)
// rather than guessed.
func synthesizedLayout(fx Fixture, channels int) (defType string, chans []models.ChannelDefinition, exact bool) {
	mk := func(i int, name, typ string) models.ChannelDefinition {
		return models.ChannelDefinition{Name: name, Type: typ, Offset: i, MinValue: 0, MaxValue: 255, FadeBehavior: "FADE"}
	}
	switch fx.Model {
	case qlcGenericModel:
		for i := 0; i < channels; i++ {
			chans = append(chans, mk(i, fmt.Sprintf("Dimmer %d", i+1), "INTENSITY"))
		}
		return "DIMMER", chans, true
	case qlcRGBPanelModel:
		return "OTHER", rgbPanelChannels(fx.Mode, channels, mk), true
	}

	if colors := colorLettersFromMode(fx.Mode, channels); colors != nil {
		for i, c := range colors {
			chans = append(chans, mk(i, c.name, c.typ))
		}
		return "LED_PAR", chans, false
	}
	for i := 0; i < channels; i++ {
		chans = append(chans, mk(i, fmt.Sprintf("Channel %d", i+1), "OTHER"))
	}
	return "OTHER", chans, false
}

type colorChannel struct{ name, typ string }

var colorLetters = map[rune]colorChannel{
	'R': {"Red", "RED"},
	'G': {"Green", "GREEN"},
	'B': {"Blue", "BLUE"},
	'W': {"White", "WHITE"},
	'A': {"Amber", "AMBER"},
	'D': {"Dimmer", "INTENSITY"},
	'C': {"Cyan", "CYAN"},
	'M': {"Magenta", "MAGENTA"},
	'Y': {"Yellow", "YELLOW"},
}

// colorLettersFromMode infers channel types from an all-caps colour token in
// the mode name whose length equals the channel count, e.g. "RGB" in
// "3-channel (RGB)" or "DRGBW".
func colorLettersFromMode(mode string, channels int) []colorChannel {
	tokens := strings.FieldsFunc(mode, func(r rune) bool { return !unicode.IsLetter(r) })
	for _, tok := range tokens {
		// Single letters ("Mode A") are too ambiguous to be colour names.
		if len(tok) < 2 || len(tok) != channels {
			continue
		}
		out := make([]colorChannel, 0, len(tok))
		for _, ch := range tok {
			c, ok := colorLetters[ch]
			if !ok {
				out = nil
				break
			}
			out = append(out, c)
		}
		if out != nil {
			return out
		}
	}
	return nil
}

// rgbPanelChannels mirrors Fixture::genericRGBPanelDef: per pixel, each
// component in mode order, each followed by its fine channel in 16-bit mode.
func rgbPanelChannels(mode string, channels int, mk func(int, string, string) models.ChannelDefinition) []models.ChannelDefinition {
	fields := strings.Fields(mode)
	order := "RGB"
	is16bit := len(fields) == 2 && fields[1] == "16bit"
	if len(fields) > 0 {
		switch fields[0] {
		case "BGR", "BRG", "GBR", "GRB", "RBG", "RGBW":
			order = fields[0]
		}
	}
	var out []models.ChannelDefinition
	for pixel := 1; len(out) < channels; pixel++ {
		for _, comp := range order {
			if len(out) >= channels {
				break
			}
			c := colorLetters[comp]
			out = append(out, mk(len(out), fmt.Sprintf("%s %d", c.name, pixel), c.typ))
			if is16bit && len(out) < channels {
				out = append(out, mk(len(out), fmt.Sprintf("%s Fine %d", c.name, pixel), "OTHER"))
			}
		}
	}
	return out
}

func instanceChannelsFromDefinition(chs []models.ChannelDefinition) []models.InstanceChannel {
	out := make([]models.InstanceChannel, len(chs))
	for i, ch := range chs {
		fb := ch.FadeBehavior
		if fb == "" {
			fb = "FADE"
		}
		out[i] = models.InstanceChannel{
			Offset:       ch.Offset,
			Name:         ch.Name,
			Type:         ch.Type,
			MinValue:     ch.MinValue,
			MaxValue:     ch.MaxValue,
			DefaultValue: ch.DefaultValue,
			FadeBehavior: fb,
			IsDiscrete:   ch.IsDiscrete,
		}
	}
	return out
}

// normalize lower-cases s and drops everything but letters and digits.
func normalize(s string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(s) {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			b.WriteRune(r)
		}
	}
	return b.String()
}

func deref(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}
