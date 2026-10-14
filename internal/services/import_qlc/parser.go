// Package importqlc imports QLC+ workspace (.qxw) files into LacyLights
// projects.
//
// The workspace format is the XML written by QLC+'s Doc::saveXML. Only the
// parts that map onto LacyLights concepts are read:
//
//   - <Engine><Fixture>   -> fixture instances (patch)
//   - Scene functions     -> looks
//   - Chaser functions    -> cue lists whose cues reference scene looks
//   - Sequence functions  -> cue lists with one look per step
//
// Everything else (EFX, RGB matrices, collections, shows, scripts, audio,
// video, the virtual console, input/output mapping) is skipped and reported
// as an import warning.
package importqlc

import (
	"bytes"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"math"
	"sort"
	"strconv"
	"strings"
)

// Function types as written in <Function Type="...">.
const (
	TypeScene    = "Scene"
	TypeChaser   = "Chaser"
	TypeSequence = "Sequence"
)

// Chaser speed modes as written in <SpeedModes FadeIn="..." ...>.
const (
	SpeedModeDefault = "Default"
	SpeedModeCommon  = "Common"
	SpeedModePerStep = "PerStep"
)

// InfiniteSpeed is QLC+'s Function::infiniteSpeed() ((uint)-2): a hold that
// waits for a manual "next".
const InfiniteSpeed uint32 = math.MaxUint32 - 1

// DefaultSpeed is QLC+'s Function::defaultSpeed() ((uint)-1).
const DefaultSpeed uint32 = math.MaxUint32

// Workspace is the subset of a QLC+ workspace that the importer uses.
type Workspace struct {
	CreatorVersion string
	Fixtures       []Fixture
	Functions      []Function
}

// Fixture is one patched QLC+ fixture. Universe and Address are 0-based, as
// stored by QLC+.
type Fixture struct {
	ID           string
	Manufacturer string
	Model        string
	Mode         string
	Name         string
	Universe     int
	Address      int
	Channels     int
	// ExcludeFade lists 0-based channel indices that QLC+ snaps instead of
	// fading.
	ExcludeFade []int
}

// ChannelValue is one channel/value pair of a scene or sequence step.
type ChannelValue struct {
	Channel int
	Value   int
}

// FixtureValues holds the channel values of one fixture in a scene or step.
type FixtureValues struct {
	FixtureID string
	Channels  []ChannelValue
}

// Speed is a function's <Speed> element, in milliseconds.
type Speed struct {
	FadeIn   uint32
	FadeOut  uint32
	Duration uint32
}

// SpeedModes is a chaser/sequence <SpeedModes> element.
type SpeedModes struct {
	FadeIn   string
	FadeOut  string
	Duration string
}

// Step is one chaser or sequence step. Times are in milliseconds.
type Step struct {
	Number     int
	FunctionID string
	FadeIn     uint32
	Hold       uint32
	FadeOut    uint32
	Note       string
	// HasValues is true for sequence steps, which carry their own values
	// instead of referencing a function.
	HasValues bool
	Values    []FixtureValues
}

// Function is a QLC+ function. Only the fields relevant to its Type are set.
type Function struct {
	ID           string
	Type         string
	Name         string
	Speed        Speed
	Values       []FixtureValues
	Direction    string
	RunOrder     string
	SpeedModes   SpeedModes
	BoundSceneID string
	Steps        []Step
}

// --- raw XML shapes -------------------------------------------------------

type xmlWorkspace struct {
	XMLName xml.Name `xml:"Workspace"`
	Creator struct {
		Version string `xml:"Version"`
	} `xml:"Creator"`
	Engine struct {
		Fixtures  []xmlFixture  `xml:"Fixture"`
		Functions []xmlFunction `xml:"Function"`
	} `xml:"Engine"`
}

type xmlFixture struct {
	Manufacturer string `xml:"Manufacturer"`
	Model        string `xml:"Model"`
	Mode         string `xml:"Mode"`
	ID           string `xml:"ID"`
	Name         string `xml:"Name"`
	Universe     string `xml:"Universe"`
	Address      string `xml:"Address"`
	Channels     string `xml:"Channels"`
	ExcludeFade  string `xml:"ExcludeFade"`
}

type xmlSpeed struct {
	FadeIn   string `xml:"FadeIn,attr"`
	FadeOut  string `xml:"FadeOut,attr"`
	Duration string `xml:"Duration,attr"`
}

type xmlFixtureVal struct {
	ID   string `xml:"ID,attr"`
	Text string `xml:",chardata"`
}

// xmlLegacyValue is the pre-FixtureVal scene format:
// <Value Fixture="0" Channel="1">255</Value>.
type xmlLegacyValue struct {
	Fixture string `xml:"Fixture,attr"`
	Channel string `xml:"Channel,attr"`
	Text    string `xml:",chardata"`
}

type xmlStep struct {
	Number   string  `xml:"Number,attr"`
	FadeIn   string  `xml:"FadeIn,attr"`
	Hold     *string `xml:"Hold,attr"`
	FadeOut  string  `xml:"FadeOut,attr"`
	Duration *string `xml:"Duration,attr"`
	Note     string  `xml:"Note,attr"`
	Values   *string `xml:"Values,attr"`
	Text     string  `xml:",chardata"`
}

type xmlFunction struct {
	ID           string           `xml:"ID,attr"`
	Type         string           `xml:"Type,attr"`
	Name         string           `xml:"Name,attr"`
	BoundScene   string           `xml:"BoundScene,attr"`
	Speed        *xmlSpeed        `xml:"Speed"`
	FixtureVals  []xmlFixtureVal  `xml:"FixtureVal"`
	LegacyValues []xmlLegacyValue `xml:"Value"`
	Direction    string           `xml:"Direction"`
	RunOrder     string           `xml:"RunOrder"`
	SpeedModes   *xmlSpeed        `xml:"SpeedModes"`
	Steps        []xmlStep        `xml:"Step"`
}

// Parse parses QLC+ workspace XML.
func Parse(content string) (*Workspace, error) {
	content = strings.TrimPrefix(content, "\ufeff")
	if strings.TrimSpace(content) == "" {
		return nil, errors.New("invalid QLC+ workspace: file is empty")
	}

	dec := xml.NewDecoder(bytes.NewReader([]byte(content)))
	// The content arrives as a (UTF-8) GraphQL string: the browser already
	// decoded the file, so whatever charset the declaration names, the
	// bytes are UTF-8 now. Pass them through unchanged.
	dec.CharsetReader = func(_ string, input io.Reader) (io.Reader, error) { return input, nil }

	var raw xmlWorkspace
	if err := dec.Decode(&raw); err != nil {
		var unexpected xml.UnmarshalError
		if errors.As(err, &unexpected) || strings.Contains(err.Error(), "expected element type") {
			return nil, fmt.Errorf("invalid QLC+ workspace: expected <Workspace> root element: %w", err)
		}
		return nil, fmt.Errorf("invalid QLC+ workspace: %w", err)
	}

	ws := &Workspace{CreatorVersion: strings.TrimSpace(raw.Creator.Version)}
	for _, f := range raw.Engine.Fixtures {
		ws.Fixtures = append(ws.Fixtures, convertFixture(f))
	}
	for _, f := range raw.Engine.Functions {
		ws.Functions = append(ws.Functions, convertFunction(f))
	}
	return ws, nil
}

func convertFixture(f xmlFixture) Fixture {
	fx := Fixture{
		ID:           strings.TrimSpace(f.ID),
		Manufacturer: strings.TrimSpace(f.Manufacturer),
		Model:        strings.TrimSpace(f.Model),
		Mode:         strings.TrimSpace(f.Mode),
		Name:         strings.TrimSpace(f.Name),
		Universe:     int(parseUint(f.Universe)),
		Address:      int(parseUint(f.Address)),
		Channels:     int(parseUint(f.Channels)),
	}
	if strings.TrimSpace(f.ExcludeFade) != "" {
		for _, s := range strings.Split(f.ExcludeFade, ",") {
			if n, err := strconv.Atoi(strings.TrimSpace(s)); err == nil && n >= 0 {
				fx.ExcludeFade = append(fx.ExcludeFade, n)
			}
		}
	}
	return fx
}

func convertFunction(f xmlFunction) Function {
	fn := Function{
		ID:           strings.TrimSpace(f.ID),
		Type:         strings.TrimSpace(f.Type),
		Name:         strings.TrimSpace(f.Name),
		Direction:    strings.TrimSpace(f.Direction),
		RunOrder:     strings.TrimSpace(f.RunOrder),
		BoundSceneID: strings.TrimSpace(f.BoundScene),
		SpeedModes:   SpeedModes{FadeIn: SpeedModeDefault, FadeOut: SpeedModeDefault, Duration: SpeedModeDefault},
	}
	if f.Speed != nil {
		fn.Speed = Speed{
			FadeIn:   parseUint(f.Speed.FadeIn),
			FadeOut:  parseUint(f.Speed.FadeOut),
			Duration: parseUint(f.Speed.Duration),
		}
	}
	if f.SpeedModes != nil {
		fn.SpeedModes = SpeedModes{
			FadeIn:   normalizeSpeedMode(f.SpeedModes.FadeIn),
			FadeOut:  normalizeSpeedMode(f.SpeedModes.FadeOut),
			Duration: normalizeSpeedMode(f.SpeedModes.Duration),
		}
	}

	for _, fv := range f.FixtureVals {
		fn.Values = append(fn.Values, FixtureValues{
			FixtureID: strings.TrimSpace(fv.ID),
			Channels:  parseChannelPairs(fv.Text),
		})
	}
	for _, lv := range f.LegacyValues {
		fn.Values = mergeValue(fn.Values, strings.TrimSpace(lv.Fixture), ChannelValue{
			Channel: int(parseUint(lv.Channel)),
			Value:   int(parseUint(lv.Text)),
		})
	}

	for i, s := range f.Steps {
		fn.Steps = append(fn.Steps, convertStep(s, i))
	}
	sort.SliceStable(fn.Steps, func(a, b int) bool { return fn.Steps[a].Number < fn.Steps[b].Number })
	return fn
}

// convertStep mirrors ChaserStep::loadXML: Hold wins when present; otherwise
// a legacy Duration is converted with hold = duration - fadeIn.
func convertStep(s xmlStep, index int) Step {
	st := Step{
		Number:  index,
		FadeIn:  parseUint(s.FadeIn),
		FadeOut: parseUint(s.FadeOut),
		Note:    s.Note,
	}
	if n, err := strconv.Atoi(strings.TrimSpace(s.Number)); err == nil {
		st.Number = n
	}
	switch {
	case s.Hold != nil:
		st.Hold = parseUint(*s.Hold)
	case s.Duration != nil:
		d := parseUint(*s.Duration)
		switch {
		case d >= InfiniteSpeed:
			st.Hold = d
		case d > st.FadeIn:
			st.Hold = d - st.FadeIn
		default:
			st.Hold = 0
		}
	}
	if s.Values != nil {
		st.HasValues = true
		st.Values = parseSequenceValues(s.Text)
	} else {
		st.FunctionID = strings.TrimSpace(s.Text)
	}
	return st
}

func normalizeSpeedMode(s string) string {
	switch strings.TrimSpace(s) {
	case SpeedModeCommon:
		return SpeedModeCommon
	case SpeedModePerStep:
		return SpeedModePerStep
	default:
		return SpeedModeDefault
	}
}

// parseChannelPairs parses "ch,val,ch,val,...". A dangling channel without a
// value is dropped, as QLC+ does.
func parseChannelPairs(s string) []ChannelValue {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil
	}
	parts := strings.Split(s, ",")
	out := make([]ChannelValue, 0, len(parts)/2)
	for i := 0; i+1 < len(parts); i += 2 {
		out = append(out, ChannelValue{
			Channel: int(parseUint(parts[i])),
			Value:   int(parseUint(parts[i+1])),
		})
	}
	return out
}

// parseSequenceValues parses sequence step text:
// "fixtureID:ch,val,ch,val:fixtureID:ch,val...".
func parseSequenceValues(s string) []FixtureValues {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil
	}
	chunks := strings.Split(s, ":")
	var out []FixtureValues
	for i := 0; i+1 < len(chunks); i += 2 {
		out = append(out, FixtureValues{
			FixtureID: strings.TrimSpace(chunks[i]),
			Channels:  parseChannelPairs(chunks[i+1]),
		})
	}
	return out
}

func mergeValue(values []FixtureValues, fixtureID string, cv ChannelValue) []FixtureValues {
	for i := range values {
		if values[i].FixtureID == fixtureID {
			values[i].Channels = append(values[i].Channels, cv)
			return values
		}
	}
	return append(values, FixtureValues{FixtureID: fixtureID, Channels: []ChannelValue{cv}})
}

// parseUint parses like QString::toUInt: surrounding whitespace is ignored
// and malformed or negative input yields 0.
func parseUint(s string) uint32 {
	n, err := strconv.ParseUint(strings.TrimSpace(s), 10, 32)
	if err != nil {
		return 0
	}
	return uint32(n)
}
