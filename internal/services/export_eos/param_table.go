package exporteos

import (
	"fmt"
	"sort"

	"github.com/bbernstein/lacylights-go/internal/graphql/generated"
)

// ParamTypeOut describes one row of the $ParamType table.
type ParamTypeOut struct {
	ID       int
	Category int
	LongName string
}

func (w *writer) writeParamTable(rows []ParamTypeOut) {
	for _, r := range rows {
		w.line(fmt.Sprintf("$ParamType %d %d %s", r.ID, r.Category, r.LongName))
	}
	w.blank()
}

// channelTypeMeta is the EOS metadata associated with a LacyLights channel type.
type channelTypeMeta struct {
	paramID  int
	category int
	longName string
}

// channelTypeMetaTable is the inverse of import_eos.mapParamTypeName: given a
// LacyLights ChannelType, return the EOS ($ParamType) row to emit.
//
// ParamIDs and categories track the convention seen in real-world EOS exports
// (see testdata/golden_otbpa.asc). EOS category numbers in use here:
//
//	1 = Intensity
//	2 = Position (Pan/Tilt)
//	3 = Color
//	5 = Form/Beam (Zoom, Iris, Focus, Gobo, Strobe — anything beam-shape)
//	6 = Effect
//	7 = Other/Reset
//
// Round-trip is preserved because the importer matches by name (long-name) for
// known channel types, not by ID.
var channelTypeMetaTable = map[generated.ChannelType]channelTypeMeta{
	generated.ChannelTypeIntensity:  {paramID: 1, category: 1, longName: "Intens"},
	generated.ChannelTypePan:        {paramID: 2, category: 2, longName: "Pan"},
	generated.ChannelTypeTilt:       {paramID: 3, category: 2, longName: "Tilt"},
	generated.ChannelTypeCyan:       {paramID: 9, category: 3, longName: "Cyan"},
	generated.ChannelTypeMagenta:    {paramID: 10, category: 3, longName: "Magenta"},
	generated.ChannelTypeYellow:     {paramID: 11, category: 3, longName: "Yellow"},
	generated.ChannelTypeRed:        {paramID: 12, category: 3, longName: "Red"},
	generated.ChannelTypeGreen:      {paramID: 13, category: 3, longName: "Green"},
	generated.ChannelTypeBlue:       {paramID: 14, category: 3, longName: "Blue"},
	generated.ChannelTypeAmber:      {paramID: 48, category: 3, longName: "Amber"},
	generated.ChannelTypeUv:         {paramID: 49, category: 3, longName: "UV"},
	generated.ChannelTypeLime:       {paramID: 50, category: 3, longName: "Lime"},
	generated.ChannelTypeWhite:      {paramID: 51, category: 3, longName: "White"},
	generated.ChannelTypeColdWhite:  {paramID: 52, category: 3, longName: "Cool_White"},
	generated.ChannelTypeWarmWhite:  {paramID: 53, category: 3, longName: "Warm_White"},
	generated.ChannelTypeIndigo:     {paramID: 54, category: 3, longName: "Indigo"},
	generated.ChannelTypeIris:       {paramID: 77, category: 5, longName: "Iris"},
	generated.ChannelTypeFocus:      {paramID: 78, category: 5, longName: "Focus"},
	generated.ChannelTypeZoom:       {paramID: 79, category: 5, longName: "Zoom"},
	generated.ChannelTypeGobo:       {paramID: 80, category: 5, longName: "Gobo"},
	generated.ChannelTypeColorWheel: {paramID: 81, category: 3, longName: "Color_Wheel"},
	generated.ChannelTypeEffect:     {paramID: 82, category: 6, longName: "Effect"},
	generated.ChannelTypeMacro:      {paramID: 83, category: 6, longName: "Macro"},
	generated.ChannelTypeStrobe:     {paramID: 204, category: 5, longName: "Shutter_Strobe"},
	generated.ChannelTypeOther:      {paramID: 999, category: 7, longName: "Other"},
}

// paramTypeMetaFor returns the $ParamType row metadata for a LacyLights
// ChannelType. Unknown types map to a stable "Other" row.
func paramTypeMetaFor(ct generated.ChannelType) channelTypeMeta {
	if m, ok := channelTypeMetaTable[ct]; ok {
		return m
	}
	return channelTypeMetaTable[generated.ChannelTypeOther]
}

// buildParamTable returns a deterministically sorted []ParamTypeOut covering
// every channel type referenced by the supplied set.
func buildParamTable(types map[generated.ChannelType]struct{}) []ParamTypeOut {
	out := make([]ParamTypeOut, 0, len(types))
	for ct := range types {
		m := paramTypeMetaFor(ct)
		out = append(out, ParamTypeOut{ID: m.paramID, Category: m.category, LongName: m.longName})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}
