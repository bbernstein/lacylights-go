package importeos

import (
	"context"
	"fmt"
	"slices"
	"strconv"
	"strings"

	"github.com/bbernstein/lacylights-go/internal/database/models"
	"github.com/bbernstein/lacylights-go/internal/database/repositories"
	"github.com/bbernstein/lacylights-go/internal/graphql/generated"
)

// FakeDef is the minimal fixture-definition shape used by the matcher's repo
// interface. The production adapter wraps *repositories.FixtureRepository to
// satisfy the same interface in Task 14.
type FakeDef struct {
	ID              string
	Manufacturer    string
	Model           string
	ChannelParamIDs []int
}

// MatcherRepo is the subset of FixtureRepository the matcher needs.
type MatcherRepo interface {
	FindMatchingDefinition(ctx context.Context, mfg, model string, paramIDs []int) (*FakeDef, error)
}

// MatchResult is the matcher's verdict for one personality.
type MatchResult struct {
	ExistingDefinitionID string
	SynthesizedDef       *models.FixtureDefinition
	SynthesizedChannels  []models.ChannelDefinition
	ChannelFingerprint   string
}

// Matcher resolves Eos personalities to LacyLights fixture definitions.
type Matcher struct {
	repo  MatcherRepo
	table *ParamTable
}

// NewMatcher constructs a matcher.
func NewMatcher(repo MatcherRepo, table *ParamTable) *Matcher {
	return &Matcher{repo: repo, table: table}
}

// Match attempts to find an existing definition; otherwise synthesizes one.
func (m *Matcher) Match(ctx context.Context, pers Personality) (MatchResult, []Warning, error) {
	paramIDs := make([]int, len(pers.Channels))
	for i, c := range pers.Channels {
		paramIDs[i] = c.ParamID
	}
	res := MatchResult{ChannelFingerprint: fingerprint(paramIDs)}

	if pers.Manuf != "" && pers.Model != "" && m.repo != nil {
		existing, err := m.repo.FindMatchingDefinition(ctx, pers.Manuf, pers.Model, paramIDs)
		if err != nil {
			return res, nil, err
		}
		if existing != nil {
			res.ExistingDefinitionID = existing.ID
			return res, nil, nil
		}
	}

	def, channels := m.synthesize(pers)
	res.SynthesizedDef = def
	res.SynthesizedChannels = channels
	return res, []Warning{{
		Code:     WarnSynthesizedFixture,
		Severity: SeverityInfo,
		Message:  fmt.Sprintf("synthesized fixture definition for %s %s", pers.Manuf, pers.Model),
		Context: map[string]string{
			"manufacturer": pers.Manuf,
			"model":        pers.Model,
			"channels":     res.ChannelFingerprint,
		},
	}}, nil
}

func (m *Matcher) synthesize(pers Personality) (*models.FixtureDefinition, []models.ChannelDefinition) {
	def := &models.FixtureDefinition{
		Manufacturer: pers.Manuf,
		Model:        pers.Model,
		Type:         "OTHER",
		IsBuiltIn:    false,
	}
	channels := make([]models.ChannelDefinition, len(pers.Channels))
	for i, pc := range pers.Channels {
		// Eos `$$PersChan` offsets are 1-based DMX offsets; LacyLights treats
		// channel offsets as 0-based (playback computes StartChannel + Offset).
		// Convert here so synthesized fixtures play back on the correct DMX
		// addresses. Clamp to >= 0 to defensively handle malformed input
		// (Offset == 0 in Eos is invalid; treat as 0-based 0).
		offset := pc.Offset - 1
		if offset < 0 {
			offset = 0
		}
		channels[i] = models.ChannelDefinition{
			Offset:       offset,
			Name:         channelNameFor(pc.ParamID, m.table),
			Type:         string(m.table.ChannelType(pc.ParamID)),
			MinValue:     0,
			MaxValue:     255,
			DefaultValue: pc.HomeValue,
			FadeBehavior: snapToFadeBehavior(pc.Flags),
		}
	}
	return def, channels
}

func channelNameFor(paramID int, table *ParamTable) string {
	switch table.ChannelType(paramID) {
	case generated.ChannelTypeIntensity:
		return "Intensity"
	case generated.ChannelTypePan:
		return "Pan"
	case generated.ChannelTypeTilt:
		return "Tilt"
	case generated.ChannelTypeRed:
		return "Red"
	case generated.ChannelTypeGreen:
		return "Green"
	case generated.ChannelTypeBlue:
		return "Blue"
	case generated.ChannelTypeAmber:
		return "Amber"
	case generated.ChannelTypeWhite:
		return "White"
	case generated.ChannelTypeColdWhite:
		return "Cool White"
	case generated.ChannelTypeWarmWhite:
		return "Warm White"
	case generated.ChannelTypeZoom:
		return "Zoom"
	case generated.ChannelTypeFocus:
		return "Focus"
	case generated.ChannelTypeIris:
		return "Iris"
	case generated.ChannelTypeGobo:
		return "Gobo"
	case generated.ChannelTypeStrobe:
		return "Strobe"
	}
	return "Channel " + strconv.Itoa(paramID)
}

func snapToFadeBehavior(flags string) string {
	if strings.Contains(flags, "S") {
		return "SNAP"
	}
	return "FADE"
}

func fingerprint(paramIDs []int) string {
	parts := make([]string, len(paramIDs))
	for i, id := range paramIDs {
		parts[i] = strconv.Itoa(id)
	}
	return strings.Join(parts, ",")
}

// repoAdapter wraps a *repositories.FixtureRepository to satisfy MatcherRepo.
type repoAdapter struct {
	repo *repositories.FixtureRepository
}

func newRepoAdapter(repo *repositories.FixtureRepository) MatcherRepo {
	return &repoAdapter{repo: repo}
}

func (a *repoAdapter) FindMatchingDefinition(ctx context.Context, mfg, model string, paramIDs []int) (*FakeDef, error) {
	if a.repo == nil {
		return nil, nil
	}
	def, err := a.repo.FindDefinitionByManufacturerModel(ctx, mfg, model)
	if err != nil || def == nil {
		return nil, err
	}
	channels, err := a.repo.GetDefinitionChannels(ctx, def.ID)
	if err != nil {
		return nil, err
	}
	ids := channelDefsToParamIDs(channels)
	if !slices.Equal(ids, paramIDs) {
		return nil, nil
	}
	return &FakeDef{
		ID:              def.ID,
		Manufacturer:    def.Manufacturer,
		Model:           def.Model,
		ChannelParamIDs: ids,
	}, nil
}

// channelDefsToParamIDs derives a representative paramID slice from existing
// channel definitions by mapping ChannelType strings back. Used for fingerprint
// equality comparison only.
func channelDefsToParamIDs(channels []models.ChannelDefinition) []int {
	out := make([]int, len(channels))
	for i, ch := range channels {
		out[i] = paramIDForChannelType(ch.Type)
	}
	return out
}

func paramIDForChannelType(t string) int {
	switch generated.ChannelType(t) {
	case generated.ChannelTypeIntensity:
		return 1
	case generated.ChannelTypePan:
		return 2
	case generated.ChannelTypeTilt:
		return 3
	case generated.ChannelTypeRed:
		return 12
	case generated.ChannelTypeGreen:
		return 13
	case generated.ChannelTypeBlue:
		return 14
	case generated.ChannelTypeAmber:
		return 48
	case generated.ChannelTypeWhite:
		return 51
	case generated.ChannelTypeColdWhite:
		return 52
	case generated.ChannelTypeWarmWhite:
		return 53
	case generated.ChannelTypeZoom:
		return 79
	case generated.ChannelTypeStrobe:
		return 204
	}
	return 0
}
