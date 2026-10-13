package ofl

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"
)

func TestParseOFLFixture_RecordsKeyOrder(t *testing.T) {
	f, err := parseOFLFixture(`{
		"name": "X", "categories": ["Dimmer"],
		"availableChannels": {"Zeta": {}, "Alpha": {}, "Mid": {}},
		"matrix": {"pixelCount": [2,1,1], "pixelGroups": {"B": "all", "A": ["1"]}},
		"modes": [{"name": "m", "channels": ["Zeta"]}]
	}`)
	if err != nil {
		t.Fatal(err)
	}
	if got := fmt.Sprint(f.availableChannelOrder); got != "[Zeta Alpha Mid]" {
		t.Errorf("availableChannelOrder = %s", got)
	}
	if got := fmt.Sprint(f.Matrix.pixelGroupKeys()); got != "[B A]" {
		t.Errorf("pixelGroupKeys = %s", got)
	}
}

func TestParseOFLFixture_InvalidJSON(t *testing.T) {
	if _, err := parseOFLFixture("not json"); err == nil || !strings.HasPrefix(err.Error(), "invalid JSON") {
		t.Errorf("err = %v", err)
	}
}

func TestOrderedObjectKeys_NonObject(t *testing.T) {
	for _, raw := range []string{"", "[1,2]", "null", "{"} {
		if got := orderedObjectKeys(json.RawMessage(raw)); len(got) != 0 {
			t.Errorf("orderedObjectKeys(%q) = %v", raw, got)
		}
	}
}

// Definition channels follow the JSON order of availableChannels, with fine
// aliases right after their coarse channel.
func TestImportFixture_DefinitionChannelsFollowJSONOrder(t *testing.T) {
	service, cleanup := setupTestDB(t)
	defer cleanup()
	def, err := service.ImportFixture(context.Background(), "Test", `{
		"name": "Ordered", "categories": ["Dimmer"],
		"availableChannels": {
			"Tilt": {"fineChannelAliases": ["Tilt fine"], "capability": {"type": "Tilt"}},
			"Dimmer": {"capability": {"type": "Intensity"}},
			"Blue": {"capability": {"type": "ColorIntensity", "color": "Blue"}}
		},
		"modes": [{"name": "m", "channels": ["Dimmer"]}]
	}`, false)
	if err != nil {
		t.Fatal(err)
	}
	byOffset := map[int]string{}
	for _, ch := range def.Channels {
		byOffset[ch.Offset] = ch.Name
	}
	want := []string{"Tilt", "Tilt fine", "Dimmer", "Blue"}
	for i, name := range want {
		if byOffset[i] != name {
			t.Errorf("offset %d = %q, want %q (all: %v)", i, byOffset[i], name, byOffset)
		}
	}
}

func TestBuildImportPlan_Errors(t *testing.T) {
	tests := []struct {
		name    string
		json    string
		wantErr error
	}{
		{
			name:    "unknown channel",
			json:    `{"name":"X","categories":["Dimmer"],"availableChannels":{"Dimmer":{}},"modes":[{"name":"m","channels":["Nope"]}]}`,
			wantErr: ErrUnknownModeChannel,
		},
		{
			name:    "unknown insert",
			json:    `{"name":"X","categories":["Dimmer"],"availableChannels":{"Dimmer":{}},"modes":[{"name":"m","channels":[{"insert":"somethingElse"}]}]}`,
			wantErr: ErrUnsupportedModeChannel,
		},
		{
			name:    "unknown channelOrder",
			json:    `{"name":"X","categories":["Dimmer"],"matrix":{"pixelCount":[2,1,1]},"templateChannels":{"D $pixelKey":{}},"modes":[{"name":"m","channels":[{"insert":"matrixChannels","repeatFor":"eachPixelABC","channelOrder":"diagonal","templateChannels":["D $pixelKey"]}]}]}`,
			wantErr: ErrUnsupportedModeChannel,
		},
		{
			name:    "repeatFor without matrix",
			json:    `{"name":"X","categories":["Dimmer"],"templateChannels":{"D $pixelKey":{}},"modes":[{"name":"m","channels":[{"insert":"matrixChannels","repeatFor":"eachPixelABC","channelOrder":"perPixel","templateChannels":["D $pixelKey"]}]}]}`,
			wantErr: ErrUnsupportedModeChannel,
		},
		{
			name:    "invalid repeatFor",
			json:    `{"name":"X","categories":["Dimmer"],"matrix":{"pixelCount":[2,1,1]},"templateChannels":{"D $pixelKey":{}},"modes":[{"name":"m","channels":[{"insert":"matrixChannels","repeatFor":"eachPixelXXY","channelOrder":"perPixel","templateChannels":["D $pixelKey"]}]}]}`,
			wantErr: ErrUnsupportedModeChannel,
		},
		{
			name:    "numeric mode entry",
			json:    `{"name":"X","categories":["Dimmer"],"availableChannels":{"Dimmer":{}},"modes":[{"name":"m","channels":[42]}]}`,
			wantErr: ErrUnsupportedModeChannel,
		},
		{
			name:    "no channels",
			json:    `{"name":"X","categories":["Dimmer"],"availableChannels":{},"modes":[{"name":"m","channels":[]}]}`,
			wantErr: ErrNoChannels,
		},
		{
			name:    "redirect",
			json:    `{"name":"X","redirectTo":"a/b","reason":"FixtureRenamed"}`,
			wantErr: ErrFixtureRedirect,
		},
		{
			name:    "switching alias cycle",
			json:    `{"name":"X","categories":["Dimmer"],"availableChannels":{"T":{"capabilities":[{"dmxRange":[0,255],"type":"Generic","switchChannels":{"A":"B","B":"A"}}]}},"modes":[{"name":"m","channels":["A"]}]}`,
			wantErr: ErrUnknownModeChannel,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f, err := parseOFLFixture(tt.json)
			if err != nil {
				t.Fatal(err)
			}
			_, err = buildImportPlan(f)
			if !errors.Is(err, tt.wantErr) {
				t.Errorf("err = %v, want %v", err, tt.wantErr)
			}
		})
	}
}

func planLayout(t *testing.T, js, mode string) []string {
	t.Helper()
	f, err := parseOFLFixture(js)
	if err != nil {
		t.Fatal(err)
	}
	plan, err := buildImportPlan(f)
	if err != nil {
		t.Fatal(err)
	}
	for _, m := range plan.Modes {
		if m.Name == mode {
			out := make([]string, len(m.channelIdx))
			for i, idx := range m.channelIdx {
				out[i] = plan.Channels[idx].Name
			}
			return out
		}
	}
	t.Fatalf("mode %q not in plan", mode)
	return nil
}

func TestBuildImportPlan_SwitchingDefaults(t *testing.T) {
	// The default value 200 selects the second capability, so "Speed /
	// Sens" defaults to "Sens"; "Other" switches to nothing by default.
	js := `{"name":"X","categories":["Dimmer"],"availableChannels":{
		"Mode": {"defaultValue": 200, "capabilities": [
			{"dmxRange":[0,127],"type":"Generic","switchChannels":{"Speed / Sens":"Speed","Other":"Speed"}},
			{"dmxRange":[128,255],"type":"Generic","switchChannels":{"Speed / Sens":"Sens","Other":null}}
		]},
		"Speed": {"capability": {"type": "Speed"}},
		"Sens": {"capability": {"type": "Intensity"}}
	},"modes":[{"name":"m","channels":["Mode","Speed / Sens","Other"]}]}`
	f, err := parseOFLFixture(js)
	if err != nil {
		t.Fatal(err)
	}
	plan, err := buildImportPlan(f)
	if err != nil {
		t.Fatal(err)
	}
	m := plan.Modes[0]
	sw := plan.Channels[m.channelIdx[1]]
	if sw.Name != "Speed / Sens" || sw.Type != "INTENSITY" {
		t.Errorf("switching channel = %+v, want INTENSITY named %q", sw, "Speed / Sens")
	}
	other := plan.Channels[m.channelIdx[2]]
	if other.Name != "Other" || other.Type != "OTHER" {
		t.Errorf("null-target switching channel = %+v", other)
	}
}

func TestBuildImportPlan_SwitchingWithPercentDefault(t *testing.T) {
	// A non-numeric defaultValue falls back to the first capability.
	js := `{"name":"X","categories":["Dimmer"],"availableChannels":{
		"Mode": {"defaultValue": "50%", "capability": {"type":"Generic","switchChannels":{"Alias":"Speed"}}},
		"Speed": {"capability": {"type": "Speed"}}
	},"modes":[{"name":"m","channels":["Alias"]}]}`
	if got := planLayout(t, js, "m"); fmt.Sprint(got) != "[Alias]" {
		t.Errorf("layout = %v", got)
	}
}

func TestBuildImportPlan_LegacySlashFallback(t *testing.T) {
	// Not valid OFL, but the importer has always accepted "A / B" when "A"
	// is a channel; keep doing so.
	js := `{"name":"X","categories":["Dimmer"],"availableChannels":{"Dimmer":{}},"modes":[{"name":"m","channels":["Dimmer / Something"]}]}`
	if got := planLayout(t, js, "m"); fmt.Sprint(got) != "[Dimmer]" {
		t.Errorf("layout = %v", got)
	}
}

func TestBuildImportPlan_PerChannelOrderAndNullTemplate(t *testing.T) {
	js := `{"name":"X","categories":["Pixel Bar"],"matrix":{"pixelCount":[3,1,1]},
		"templateChannels":{"R $pixelKey":{"capability":{"type":"ColorIntensity","color":"Red"}},"G $pixelKey":{"capability":{"type":"ColorIntensity","color":"Green"}}},
		"modes":[{"name":"m","channels":[{"insert":"matrixChannels","repeatFor":"eachPixelABC","channelOrder":"perChannel","templateChannels":["R $pixelKey",null,"G $pixelKey"]}]}]}`
	got := planLayout(t, js, "m")
	want := "[R 1 R 2 R 3 No function No function No function G 1 G 2 G 3]"
	if fmt.Sprint(got) != want {
		t.Errorf("layout = %v, want %s", got, want)
	}
}

func TestBuildImportPlan_AvailableChannelOverridesTemplate(t *testing.T) {
	js := `{"name":"X","categories":["Pixel Bar"],"matrix":{"pixelCount":[2,1,1]},
		"availableChannels":{"Speed 1":{"capability":{"type":"Intensity"}}},
		"templateChannels":{"Speed $pixelKey":{"capability":{"type":"Speed"}}},
		"modes":[{"name":"m","channels":["Speed 1","Speed 2"]}]}`
	f, _ := parseOFLFixture(js)
	plan, err := buildImportPlan(f)
	if err != nil {
		t.Fatal(err)
	}
	m := plan.Modes[0]
	if plan.Channels[m.channelIdx[0]].Type != "INTENSITY" || plan.Channels[m.channelIdx[1]].Type != "EFFECT" {
		t.Errorf("types = %s, %s", plan.Channels[m.channelIdx[0]].Type, plan.Channels[m.channelIdx[1]].Type)
	}
}

func TestBuildImportPlan_TemplateRejectsUnknownPixel(t *testing.T) {
	js := `{"name":"X","categories":["Pixel Bar"],"matrix":{"pixelCount":[2,1,1]},
		"templateChannels":{"Red $pixelKey":{}},
		"modes":[{"name":"m","channels":["Red 9"]}]}`
	f, _ := parseOFLFixture(js)
	if _, err := buildImportPlan(f); !errors.Is(err, ErrUnknownModeChannel) {
		t.Errorf("err = %v, want ErrUnknownModeChannel", err)
	}
}

func TestOFLMatrix_PixelKeyOrders(t *testing.T) {
	// 2D 3x2 matrix: keys are "(x, y)".
	m := &OFLMatrix{PixelCount: []int{3, 2, 1}}
	if got := fmt.Sprint(m.pixelKeysByOrder("XYZ")); got != "[(1, 1) (2, 1) (3, 1) (1, 2) (2, 2) (3, 2)]" {
		t.Errorf("XYZ = %s", got)
	}
	if got := fmt.Sprint(m.pixelKeysByOrder("YXZ")); got != "[(1, 1) (1, 2) (2, 1) (2, 2) (3, 1) (3, 2)]" {
		t.Errorf("YXZ = %s", got)
	}
	// 3D keys.
	m3 := &OFLMatrix{PixelCount: []int{2, 1, 2}}
	if got := fmt.Sprint(m3.pixelKeysABC()); got != "[(1, 1) (1, 2) (2, 1) (2, 2)]" {
		t.Errorf("2D xz ABC = %s", got)
	}
	m3d := &OFLMatrix{PixelCount: []int{2, 2, 2}}
	if got := len(m3d.pixelKeysABC()); got != 8 {
		t.Errorf("3D count = %d", got)
	}
	if got := m3d.pixelKeysByOrder("XYZ")[1]; got != "(2, 1, 1)" {
		t.Errorf("3D XYZ[1] = %s", got)
	}
	// 1D numeric keys sort naturally.
	m1 := &OFLMatrix{PixelCount: []int{12, 1, 1}}
	if got := m1.pixelKeysABC(); got[1] != "2" || got[11] != "12" {
		t.Errorf("1D ABC = %v", got)
	}
	// Single pixel.
	if got := fmt.Sprint((&OFLMatrix{PixelCount: []int{1, 1, 1}}).pixelKeysABC()); got != "[1]" {
		t.Errorf("single pixel = %s", got)
	}
	// pixelKeys with holes; pixelCount derived from the structure.
	a, b, c := "A", "B", "C"
	mk := &OFLMatrix{PixelKeys: [][][]*string{{{&b, nil, &a}, {nil, &c}}}}
	if got := mk.pixelCount(); got != [3]int{3, 2, 1} {
		t.Errorf("derived pixelCount = %v", got)
	}
	if got := fmt.Sprint(mk.pixelKeysByOrder("XYZ")); got != "[B A C]" {
		t.Errorf("pixelKeys XYZ = %s", got)
	}
	// Groups without recorded JSON order fall back to sorted keys.
	mg := &OFLMatrix{PixelGroups: map[string]json.RawMessage{"b": nil, "a": nil}}
	if got := fmt.Sprint(mg.pixelGroupKeys()); got != "[a b]" {
		t.Errorf("group keys = %s", got)
	}
}

func TestNaturalLess(t *testing.T) {
	tests := []struct {
		a, b string
		want bool
	}{
		{"2", "10", true},
		{"10", "2", false},
		{"LED 2", "LED 10", true},
		{"a", "B", true},
		{"abc", "abcd", true},
		{"abcd", "abc", false},
		{"x1", "x1", false},
	}
	for _, tt := range tests {
		if got := naturalLess(tt.a, tt.b); got != tt.want {
			t.Errorf("naturalLess(%q, %q) = %v, want %v", tt.a, tt.b, got, tt.want)
		}
	}
}

func TestRepeatForKeys_ExplicitList(t *testing.T) {
	got, err := repeatForKeys(json.RawMessage(`["b","a"]`), nil)
	if err != nil || fmt.Sprint(got) != "[b a]" {
		t.Errorf("got %v, %v", got, err)
	}
	if _, err := repeatForKeys(json.RawMessage(`42`), nil); err == nil {
		t.Error("expected error for numeric repeatFor")
	}
	if _, err := repeatForKeys(json.RawMessage(`"bogus"`), &OFLMatrix{PixelCount: []int{1, 1, 1}}); err == nil {
		t.Error("expected error for unknown repeatFor")
	}
}
