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

func TestParamTable_ColorModelAndSpeedTypes(t *testing.T) {
	cases := []struct {
		name     string
		category int
		want     generated.ChannelType
	}{
		// IDs 7 and 8 match real Eos exports (docs/*.asc).
		{"Hue", 3, generated.ChannelTypeHue},
		{"Saturation", 3, generated.ChannelTypeSaturation},
		{"Color_Temperature", 3, generated.ChannelTypeColorTemperature},
		{"Colour_Temperature", 3, generated.ChannelTypeColorTemperature},
		{"Color_Temp", 3, generated.ChannelTypeColorTemperature},
		{"CTC", 3, generated.ChannelTypeColorTemperature},
		{"CCT", 3, generated.ChannelTypeColorTemperature},
		{"Speed", 6, generated.ChannelTypeSpeed},
		{"Effect_Speed", 6, generated.ChannelTypeSpeed},
		{"Position_MSpeed", 2, generated.ChannelTypeSpeed},
		{"MSpeed", 2, generated.ChannelTypeSpeed},
		// Index/speed combo channels are not pure speed channels.
		{"Gobo_Index/Speed", 5, generated.ChannelTypeOther},
		// "speed" must be a whole trailing word.
		{"Speed_Mode", 5, generated.ChannelTypeOther},
		{"Gamspeed", 5, generated.ChannelTypeOther},
		{"Wheelspeed", 5, generated.ChannelTypeOther},
	}
	for _, c := range cases {
		if got := mapParamTypeName(c.name, c.category); got != c.want {
			t.Errorf("mapParamTypeName(%q, %d) = %v, want %v", c.name, c.category, got, c.want)
		}
	}
}

func TestChannelNameFor_ColorModelAndSpeedTypes(t *testing.T) {
	tbl := NewParamTable([]ParamType{
		{ID: 7, Category: 3, LongName: "Hue"},
		{ID: 8, Category: 3, LongName: "Saturation"},
		{ID: 500, Category: 3, LongName: "Color_Temperature"},
		{ID: 501, Category: 6, LongName: "Speed"},
	})
	cases := map[int]string{7: "Hue", 8: "Saturation", 500: "Color Temperature", 501: "Speed"}
	for id, want := range cases {
		if got := channelNameFor(id, tbl); got != want {
			t.Errorf("channelNameFor(%d) = %q, want %q", id, got, want)
		}
	}
}

func TestParamIDForChannelType_HueSaturation(t *testing.T) {
	if got := paramIDForChannelType(string(generated.ChannelTypeHue)); got != 7 {
		t.Errorf("HUE paramID = %d, want 7", got)
	}
	if got := paramIDForChannelType(string(generated.ChannelTypeSaturation)); got != 8 {
		t.Errorf("SATURATION paramID = %d, want 8", got)
	}
}
