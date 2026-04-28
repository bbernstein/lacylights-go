package importeos

import (
	"testing"

	"github.com/bbernstein/lacylights-go/internal/graphql/generated"
)

func TestParamTable_KnownTypes(t *testing.T) {
	pts := []ParamType{
		{ID: 1, Category: 1, LongName: "Intens"},
		{ID: 2, Category: 2, LongName: "Pan"},
		{ID: 12, Category: 3, LongName: "Red"},
		{ID: 13, Category: 3, LongName: "Green"},
		{ID: 14, Category: 3, LongName: "Blue"},
		{ID: 48, Category: 3, LongName: "Amber"},
		{ID: 79, Category: 5, LongName: "Zoom"},
		{ID: 204, Category: 5, LongName: "Shutter_Strobe"},
		{ID: 9999, Category: 7, LongName: "Custom"},
	}
	tbl := NewParamTable(pts)
	cases := []struct {
		id   int
		want generated.ChannelType
	}{
		{1, generated.ChannelTypeIntensity},
		{2, generated.ChannelTypePan},
		{12, generated.ChannelTypeRed},
		{13, generated.ChannelTypeGreen},
		{14, generated.ChannelTypeBlue},
		{48, generated.ChannelTypeAmber},
		{79, generated.ChannelTypeZoom},
		{204, generated.ChannelTypeStrobe},
		{9999, generated.ChannelTypeOther},
	}
	for _, c := range cases {
		got := tbl.ChannelType(c.id)
		if got != c.want {
			t.Errorf("paramID %d: got %v, want %v", c.id, got, c.want)
		}
	}
}
