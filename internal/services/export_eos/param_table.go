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
// ParamIDs match the conventions used by paramIDForChannelType in the importer
// where defined; otherwise a stable ID > 1000 is assigned per channel type to
// keep export output deterministic and avoid collisions with EOS-default IDs.
var channelTypeMetaTable = map[generated.ChannelType]channelTypeMeta{
	generated.ChannelTypeIntensity:  {paramID: 1, category: 1, longName: "Intens"},
	generated.ChannelTypePan:        {paramID: 2, category: 2, longName: "Pan"},
	generated.ChannelTypeTilt:       {paramID: 3, category: 2, longName: "Tilt"},
	generated.ChannelTypeRed:        {paramID: 12, category: 3, longName: "Red"},
	generated.ChannelTypeGreen:      {paramID: 13, category: 3, longName: "Green"},
	generated.ChannelTypeBlue:       {paramID: 14, category: 3, longName: "Blue"},
	generated.ChannelTypeAmber:      {paramID: 48, category: 3, longName: "Amber"},
	generated.ChannelTypeWhite:      {paramID: 51, category: 3, longName: "White"},
	generated.ChannelTypeColdWhite:  {paramID: 52, category: 3, longName: "Cool_White"},
	generated.ChannelTypeWarmWhite:  {paramID: 53, category: 3, longName: "Warm_White"},
	generated.ChannelTypeCyan:       {paramID: 15, category: 3, longName: "Cyan"},
	generated.ChannelTypeMagenta:    {paramID: 16, category: 3, longName: "Magenta"},
	generated.ChannelTypeYellow:     {paramID: 17, category: 3, longName: "Yellow"},
	generated.ChannelTypeUv:         {paramID: 49, category: 3, longName: "UV"},
	generated.ChannelTypeLime:       {paramID: 50, category: 3, longName: "Lime"},
	generated.ChannelTypeIndigo:     {paramID: 54, category: 3, longName: "Indigo"},
	generated.ChannelTypeZoom:       {paramID: 79, category: 4, longName: "Zoom"},
	generated.ChannelTypeFocus:      {paramID: 78, category: 4, longName: "Focus"},
	generated.ChannelTypeIris:       {paramID: 77, category: 4, longName: "Iris"},
	generated.ChannelTypeGobo:       {paramID: 80, category: 5, longName: "Gobo"},
	generated.ChannelTypeStrobe:     {paramID: 204, category: 1, longName: "Shutter_Strobe"},
	generated.ChannelTypeColorWheel: {paramID: 81, category: 3, longName: "Color_Wheel"},
	generated.ChannelTypeEffect:     {paramID: 82, category: 6, longName: "Effect"},
	generated.ChannelTypeMacro:      {paramID: 83, category: 6, longName: "Macro"},
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
