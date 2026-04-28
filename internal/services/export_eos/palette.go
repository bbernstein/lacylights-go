package exporteos

import (
	"fmt"
	"strings"
	"unicode/utf16"
)

// PaletteOut configures one palette emission.
type PaletteOut struct {
	Kind        string // "ColorPalette" | "BeamPalette" | "FocusPalette" | "IntensPalette" | "Preset"
	Number      string
	Label       string
	UnicodeText *string
	ChanMoves   []ChanMoveOut
	ParamMoves  []ParamMoveOut
}

// ChanMoveOut is a single intensity move.
type ChanMoveOut struct {
	Channel int
	Value   int // 0..255
}

// ParamMoveOut is one channel's attribute moves.
type ParamMoveOut struct {
	Channel int
	Values  []ParamValueOut
}

// ParamValueOut is one paramID/value pair.
type ParamValueOut struct {
	ParamID int
	Value   int
}

func (w *writer) writePalette(p PaletteOut) {
	w.line(fmt.Sprintf("$%s %s", p.Kind, p.Number))
	if p.Label != "" {
		w.line("   Text " + p.Label)
	}
	if p.UnicodeText != nil {
		w.line("   $$UText " + encodeUText(*p.UnicodeText))
	}
	if len(p.ChanMoves) > 0 {
		w.line("   $$ChanMove " + formatChanMoves(p.ChanMoves))
	}
	for _, pm := range p.ParamMoves {
		w.line("   $$Param " + formatParamMove(pm))
	}
}

func formatChanMoves(moves []ChanMoveOut) string {
	parts := make([]string, len(moves))
	for i, m := range moves {
		parts[i] = fmt.Sprintf("%d@H%02X", m.Channel, m.Value&0xFF)
	}
	return strings.Join(parts, " ")
}

func formatParamMove(pm ParamMoveOut) string {
	parts := make([]string, 0, 1+len(pm.Values))
	parts = append(parts, fmt.Sprintf("%d", pm.Channel))
	for _, v := range pm.Values {
		parts = append(parts, fmt.Sprintf("%d@%d", v.ParamID, v.Value))
	}
	return strings.Join(parts, " ")
}

// encodeUText returns hex-encoded UTF-16-LE for s.
func encodeUText(s string) string {
	u16 := utf16.Encode([]rune(s))
	parts := make([]string, len(u16))
	for i, c := range u16 {
		parts[i] = fmt.Sprintf("%02X%02X", byte(c&0xFF), byte((c>>8)&0xFF))
	}
	return strings.Join(parts, " ")
}
