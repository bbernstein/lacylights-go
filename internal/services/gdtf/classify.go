package gdtf

import (
	"regexp"
	"strings"
)

// LacyLights channel types (the ChannelType GraphQL enum).
const (
	typeIntensity        = "INTENSITY"
	typeRed              = "RED"
	typeGreen            = "GREEN"
	typeBlue             = "BLUE"
	typeWhite            = "WHITE"
	typeAmber            = "AMBER"
	typeUV               = "UV"
	typeCyan             = "CYAN"
	typeMagenta          = "MAGENTA"
	typeYellow           = "YELLOW"
	typeLime             = "LIME"
	typeColdWhite        = "COLD_WHITE"
	typeWarmWhite        = "WARM_WHITE"
	typeColorTemperature = "COLOR_TEMPERATURE"
	typeHue              = "HUE"
	typeSaturation       = "SATURATION"
	typePan              = "PAN"
	typeTilt             = "TILT"
	typeZoom             = "ZOOM"
	typeFocus            = "FOCUS"
	typeIris             = "IRIS"
	typeGobo             = "GOBO"
	typeColorWheel       = "COLOR_WHEEL"
	typeEffect           = "EFFECT"
	typeSpeed            = "SPEED"
	typeStrobe           = "STROBE"
	typeMacro            = "MACRO"
	typeOther            = "OTHER"
)

// exactTypes maps normalized attribute names (lower case, numbers replaced
// by "(n)") to channel types.
var exactTypes = map[string]string{
	"dimmer":         typeIntensity,
	"hsb_brightness": typeIntensity,

	"pan":  typePan,
	"tilt": typeTilt,

	"coloradd_r":     typeRed,
	"colorrgb_red":   typeRed,
	"coloradd_g":     typeGreen,
	"colorrgb_green": typeGreen,
	"coloradd_b":     typeBlue,
	"colorrgb_blue":  typeBlue,
	"coloradd_w":     typeWhite,
	"coloradd_ww":    typeWarmWhite,
	"coloradd_cw":    typeColdWhite,
	"coloradd_ry":    typeAmber, // GDTF has no ColorAdd_A: amber is red-yellow
	"coloradd_gy":    typeLime,  // lime is green-yellow
	"coloradd_uv":    typeUV,

	"coloradd_c":       typeCyan,
	"colorsub_c":       typeCyan,
	"colorrgb_cyan":    typeCyan,
	"coloradd_m":       typeMagenta,
	"colorsub_m":       typeMagenta,
	"colorrgb_magenta": typeMagenta,
	"coloradd_y":       typeYellow,
	"colorsub_y":       typeYellow,
	"colorrgb_yellow":  typeYellow,

	"cto": typeColorTemperature,
	"ctc": typeColorTemperature,
	"ctb": typeColorTemperature,

	"hsb_hue":        typeHue,
	"hsb_saturation": typeSaturation,

	"color(n)":           typeColorWheel,
	"color(n)wheelindex": typeColorWheel,

	"gobo(n)":              typeGobo,
	"gobo(n)wheelindex":    typeGobo,
	"gobo(n)selectspin":    typeGobo,
	"gobo(n)selectshake":   typeGobo,
	"gobo(n)selecteffects": typeGobo,

	"zoom":           typeZoom,
	"zoommodespot":   typeZoom,
	"zoommodebeam":   typeZoom,
	"focus(n)":       typeFocus,
	"iris":           typeIris,
	"effects(n)rate": typeSpeed,

	"colormacro(n)rate": typeSpeed,
	"colormacro(n)":     typeMacro,
	"control(n)":        typeMacro,
	"lampcontrol":       typeMacro,
	"function":          typeMacro,

	"coloradd_gc": typeOther,
	"coloradd_bc": typeOther,
	"coloradd_bm": typeOther,
	"coloradd_rm": typeOther,
	"colorsub_r":  typeOther,
	"colorsub_g":  typeOther,
	"colorsub_b":  typeOther,
	"tint":        typeOther,
	"dummy":       typeOther,
	"nofeature":   typeOther,
}

// affixRule matches a normalized attribute by prefix and/or suffix.
type affixRule struct {
	prefix, suffix string
	typ            string
}

// suffixRules run before prefixRules, so "CTOReset" is a reset (MACRO) and
// "IntensityMSpeed" a speed, not a colour temperature or an intensity.
var suffixRules = []affixRule{
	{suffix: "mspeed", typ: typeSpeed},
	{suffix: "reset", typ: typeMacro},
}

var prefixRules = []affixRule{
	{prefix: "shutter(n)", typ: typeStrobe},
	{prefix: "strobe", typ: typeStrobe},
	{prefix: "prism(n)", typ: typeEffect},
	{prefix: "effects(n)", typ: typeEffect},
	{prefix: "gobo(n)pos", typ: typeEffect},
	{suffix: "rotate", typ: typeEffect},
	{suffix: "spin", typ: typeEffect}, // gobo/colour select spins are matched exactly above
	{prefix: "cie_", typ: typeOther},
	{prefix: "frost", typ: typeOther},
	{prefix: "fan", typ: typeOther},
	{prefix: "fog", typ: typeOther},
	{prefix: "haze", typ: typeOther},
}

// featureTypes is the fallback for attributes the tables do not know
// (including custom attributes), keyed by lower-case "Group.Feature".
var featureTypes = map[string]string{
	"dimmer.dimmer":   typeIntensity,
	"control.control": typeMacro,
}

var digitsRe = regexp.MustCompile(`[0-9]+`)

// normalizeAttribute lower-cases an attribute name and replaces its
// numbers with "(n)", matching the spec's naming ("Gobo2Pos" ->
// "gobo(n)pos").
func normalizeAttribute(name string) string {
	n := strings.ToLower(strings.TrimSpace(name))
	n = strings.ReplaceAll(n, "(n)", "0")
	n = strings.ReplaceAll(n, "(m)", "0")
	return digitsRe.ReplaceAllString(n, "(n)")
}

// classifyAttribute maps a GDTF attribute to a LacyLights channel type.
// feature is the attribute's Feature ("Group.Feature"), used when the
// name is not recognised.
func classifyAttribute(name, feature string) string {
	n := normalizeAttribute(name)
	if t, ok := exactTypes[n]; ok {
		return t
	}
	for _, r := range suffixRules {
		if r.matches(n) {
			return r.typ
		}
	}
	// "*Mode" attributes are control settings (DimmerMode, PanMode, ...).
	// Without a Feature, assume the control group as the spec's do.
	group, _, _ := strings.Cut(strings.ToLower(strings.TrimSpace(feature)), ".")
	if strings.HasSuffix(n, "mode") && (group == "" || group == "control") {
		return typeMacro
	}
	for _, r := range prefixRules {
		if r.matches(n) {
			return r.typ
		}
	}
	if t, ok := featureTypes[strings.ToLower(strings.TrimSpace(feature))]; ok {
		return t
	}
	return typeOther
}

func (r affixRule) matches(n string) bool {
	if r.prefix != "" && !strings.HasPrefix(n, r.prefix) {
		return false
	}
	if r.suffix != "" && !strings.HasSuffix(n, r.suffix) {
		return false
	}
	return true
}

// colourTypes are colour-mixing channel types (a fixture with any of them
// is an LED_PAR unless it moves).
var colourTypes = map[string]bool{
	typeRed: true, typeGreen: true, typeBlue: true, typeWhite: true,
	typeAmber: true, typeUV: true, typeCyan: true, typeMagenta: true,
	typeYellow: true, typeLime: true, typeColdWhite: true, typeWarmWhite: true,
	"INDIGO": true, typeHue: true, typeSaturation: true,
}

// fixtureTypeFor derives FixtureDefinition.Type from the (coarse, real)
// channel types of all modes:
//   - Pan and Tilt: MOVING_HEAD
//   - any colour mixing: LED_PAR
//   - only intensity, plus no more strobe than intensity channels and any
//     control channels: DIMMER
//   - strobe-dominant (at least as many strobe as intensity channels, and
//     at least half of all channels): STROBE
//   - otherwise OTHER
func fixtureTypeFor(types []string) string {
	count := map[string]int{}
	for _, t := range types {
		count[t]++
	}
	if count[typePan] > 0 && count[typeTilt] > 0 {
		return "MOVING_HEAD"
	}
	for t := range colourTypes {
		if count[t] > 0 {
			return "LED_PAR"
		}
	}
	n := len(types)
	i, s, m := count[typeIntensity], count[typeStrobe], count[typeMacro]
	if i > 0 && i+s+m == n && s <= i {
		return "DIMMER"
	}
	if s > 0 && s >= i && 2*s >= n {
		return "STROBE"
	}
	return "OTHER"
}
