package importeos

import (
	"strings"

	"github.com/bbernstein/lacylights-go/internal/graphql/generated"
)

// ParamTable maps EOS parameter IDs to LacyLights ChannelType values.
type ParamTable struct {
	byID map[int]generated.ChannelType
}

// NewParamTable builds a lookup from a parsed $ParamType list.
// Unknown IDs map to generated.ChannelTypeOther.
func NewParamTable(pts []ParamType) *ParamTable {
	t := &ParamTable{byID: make(map[int]generated.ChannelType, len(pts))}
	for _, pt := range pts {
		t.byID[pt.ID] = mapParamTypeName(pt.LongName, pt.Category)
	}
	return t
}

// ChannelType returns the LacyLights ChannelType for an EOS parameter ID.
func (t *ParamTable) ChannelType(id int) generated.ChannelType {
	if t == nil {
		return generated.ChannelTypeOther
	}
	if v, ok := t.byID[id]; ok {
		return v
	}
	return generated.ChannelTypeOther
}

// mapParamTypeName classifies by name first (most reliable), category second.
func mapParamTypeName(name string, category int) generated.ChannelType {
	switch strings.ToLower(name) {
	case "intens", "intensity":
		return generated.ChannelTypeIntensity
	case "pan":
		return generated.ChannelTypePan
	case "tilt":
		return generated.ChannelTypeTilt
	case "red":
		return generated.ChannelTypeRed
	case "green":
		return generated.ChannelTypeGreen
	case "blue":
		return generated.ChannelTypeBlue
	case "amber":
		return generated.ChannelTypeAmber
	case "white":
		return generated.ChannelTypeWhite
	case "cool_white", "cold_white":
		return generated.ChannelTypeColdWhite
	case "warm_white":
		return generated.ChannelTypeWarmWhite
	case "cyan":
		return generated.ChannelTypeCyan
	case "magenta":
		return generated.ChannelTypeMagenta
	case "yellow":
		return generated.ChannelTypeYellow
	case "uv":
		return generated.ChannelTypeUv
	case "lime":
		return generated.ChannelTypeLime
	case "indigo":
		return generated.ChannelTypeIndigo
	case "zoom":
		return generated.ChannelTypeZoom
	case "focus":
		return generated.ChannelTypeFocus
	case "iris":
		return generated.ChannelTypeIris
	case "gobo":
		return generated.ChannelTypeGobo
	case "shutter_strobe", "strobe":
		return generated.ChannelTypeStrobe
	}
	switch category {
	case 1:
		return generated.ChannelTypeIntensity
	}
	return generated.ChannelTypeOther
}
