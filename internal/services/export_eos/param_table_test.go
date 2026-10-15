package exporteos

import (
	"testing"

	"github.com/bbernstein/lacylights-go/internal/graphql/generated"
)

func TestParamTypeMetaFor_HueSaturation(t *testing.T) {
	// IDs and categories verified against real Eos exports (docs/*.asc).
	cases := []struct {
		ct   generated.ChannelType
		want channelTypeMeta
	}{
		{generated.ChannelTypeHue, channelTypeMeta{paramID: 7, category: 3, longName: "Hue"}},
		{generated.ChannelTypeSaturation, channelTypeMeta{paramID: 8, category: 3, longName: "Saturation"}},
	}
	for _, c := range cases {
		if got := paramTypeMetaFor(c.ct); got != c.want {
			t.Errorf("paramTypeMetaFor(%s) = %+v, want %+v", c.ct, got, c.want)
		}
	}
}

func TestParamTypeMetaFor_UnverifiedTypesUseOtherRow(t *testing.T) {
	// Eos param IDs for colour temperature and speed have not been verified
	// against a real export yet, so they keep the generic "Other" row.
	other := channelTypeMetaTable[generated.ChannelTypeOther]
	for _, ct := range []generated.ChannelType{generated.ChannelTypeColorTemperature, generated.ChannelTypeSpeed} {
		if got := paramTypeMetaFor(ct); got != other {
			t.Errorf("paramTypeMetaFor(%s) = %+v, want Other row %+v", ct, got, other)
		}
	}
}

func TestBuildParamTable_IncludesHueSaturation(t *testing.T) {
	rows := buildParamTable(map[generated.ChannelType]struct{}{
		generated.ChannelTypeSaturation: {},
		generated.ChannelTypeIntensity:  {},
		generated.ChannelTypeHue:        {},
	})
	want := []ParamTypeOut{
		{ID: 1, Category: 1, LongName: "Intens"},
		{ID: 7, Category: 3, LongName: "Hue"},
		{ID: 8, Category: 3, LongName: "Saturation"},
	}
	if len(rows) != len(want) {
		t.Fatalf("rows = %+v, want %+v", rows, want)
	}
	for i := range want {
		if rows[i] != want[i] {
			t.Errorf("row %d = %+v, want %+v", i, rows[i], want[i])
		}
	}
}
