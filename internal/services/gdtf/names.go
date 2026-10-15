package gdtf

import (
	"strings"
	"unicode"
)

// friendlyAttributeNames gives readable channel names for standard GDTF
// attributes whose names or Pretty values are terse ("ColorAdd_RY", "Dim").
// Keys are lower case.
var friendlyAttributeNames = map[string]string{
	"dimmer":         "Dimmer",
	"pan":            "Pan",
	"tilt":           "Tilt",
	"coloradd_r":     "Red",
	"coloradd_g":     "Green",
	"coloradd_b":     "Blue",
	"coloradd_c":     "Cyan",
	"coloradd_m":     "Magenta",
	"coloradd_y":     "Yellow",
	"coloradd_ry":    "Amber",
	"coloradd_gy":    "Lime",
	"coloradd_gc":    "Green-Cyan",
	"coloradd_bc":    "Blue-Cyan",
	"coloradd_bm":    "Blue-Magenta",
	"coloradd_rm":    "Red-Magenta",
	"coloradd_w":     "White",
	"coloradd_ww":    "Warm White",
	"coloradd_cw":    "Cool White",
	"coloradd_uv":    "UV",
	"colorsub_r":     "Red (subtractive)",
	"colorsub_g":     "Green (subtractive)",
	"colorsub_b":     "Blue (subtractive)",
	"colorsub_c":     "Cyan",
	"colorsub_m":     "Magenta",
	"colorsub_y":     "Yellow",
	"colorrgb_red":   "Red",
	"colorrgb_green": "Green",
	"colorrgb_blue":  "Blue",
	"hsb_hue":        "Hue",
	"hsb_saturation": "Saturation",
	"hsb_brightness": "Brightness",
	"hsb_quality":    "HSB Quality",
	"cie_x":          "CIE X",
	"cie_y":          "CIE Y",
	"cie_brightness": "CIE Brightness",
	"nofeature":      placeholderName,
}

// channelDisplayName picks a readable channel name for an attribute.
// A descriptive Pretty value is used as-is ("Gobo Rotate", "Shutter"); short
// console labels such as "Dim", "P" or "Sh1" are not. Then standard
// attributes get a friendly name, and anything else is split into words
// ("Shutter1Strobe" → "Shutter 1 Strobe").
func channelDisplayName(attr string, pretty string) string {
	if p := strings.TrimSpace(pretty); isDescriptivePretty(p) {
		return p
	}
	attr = strings.TrimSpace(attr)
	if attr == "" {
		if p := strings.TrimSpace(pretty); p != "" {
			return p
		}
		return placeholderName
	}
	if f, ok := friendlyAttributeNames[strings.ToLower(attr)]; ok {
		return f
	}
	return humanizeAttribute(attr)
}

// humanizeAttribute splits a GDTF attribute name into words: underscores
// become spaces, and a space is inserted at lower→upper and letter↔digit
// boundaries. A trailing "MSpeed" (GDTF's "speed of movement") reads as
// "Speed".
func humanizeAttribute(attr string) string {
	if strings.HasSuffix(attr, "MSpeed") && len(attr) > len("MSpeed") {
		attr = strings.TrimSuffix(attr, "MSpeed") + "Speed"
	}
	runes := []rune(strings.ReplaceAll(attr, "_", " "))
	var b strings.Builder
	for i, r := range runes {
		if i > 0 {
			prev := runes[i-1]
			boundary := (unicode.IsLower(prev) && unicode.IsUpper(r)) ||
				(unicode.IsLetter(prev) && unicode.IsDigit(r)) ||
				(unicode.IsDigit(prev) && unicode.IsLetter(r))
			if boundary && prev != ' ' {
				b.WriteRune(' ')
			}
		}
		b.WriteRune(r)
	}
	return strings.Join(strings.Fields(b.String()), " ")
}

// isDescriptivePretty reports whether a GDTF Pretty value reads as a name
// rather than a console abbreviation: it contains a space, or it is a word
// of at least four letters that is not all upper case.
func isDescriptivePretty(p string) bool {
	if strings.Contains(p, " ") {
		return true
	}
	if len([]rune(p)) < 4 || strings.ToUpper(p) == p {
		return false
	}
	for _, r := range p {
		if !unicode.IsLetter(r) {
			return false
		}
	}
	return true
}
