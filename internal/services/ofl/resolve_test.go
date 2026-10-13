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

// Channel-level defaultValue is honoured and split across coarse/fine
// bytes at the channel's resolution (OFL: percentages use Math.floor).
func TestBuildImportPlan_DefaultValueResolution(t *testing.T) {
	js := `{"name":"X","categories":["Moving Head"],"availableChannels":{
		"Pan": {"fineChannelAliases":["Pan fine"],"defaultValue":"50%","capability":{"type":"Pan","angleStart":"0deg","angleEnd":"540deg"}},
		"Iris": {"fineChannelAliases":["Iris fine"],"dmxValueResolution":"8bit","defaultValue":132,
			"capabilities":[{"dmxRange":[0,131],"type":"Iris"},{"dmxRange":[132,255],"type":"Iris"}]},
		"Tilt": {"fineChannelAliases":["Tilt fine"],"capability":{"dmxRange":[0,65535],"type":"Tilt"}},
		"Dimmer": {"defaultValue":255,"capability":{"type":"Intensity"}},
		"Raw": {"defaultValue":"25%"}
	},"modes":[{"name":"m","channels":["Pan","Pan fine","Iris","Iris fine","Tilt","Tilt fine","Dimmer","Raw"]}]}`
	f, err := parseOFLFixture(js)
	if err != nil {
		t.Fatal(err)
	}
	plan, err := buildImportPlan(f)
	if err != nil {
		t.Fatal(err)
	}
	got := map[string][3]int{}
	for _, idx := range plan.Modes[0].channelIdx {
		c := plan.Channels[idx]
		got[c.Name] = [3]int{c.DefaultValue, c.MinValue, c.MaxValue}
	}
	want := map[string][3]int{
		"Pan":       {127, 0, 255}, // floor(0.5 * 65535) = 32767 = 127/255
		"Pan fine":  {255, 0, 255},
		"Iris":      {132, 0, 131}, // 8-bit 132 on a 16-bit channel: 0x8484 (OFL scaleDmxValue)
		"Iris fine": {132, 0, 255},
		"Tilt":      {127, 0, 255}, // no default: centre of [0, 65535], reduced to coarse
		"Tilt fine": {0, 0, 255},
		"Dimmer":    {255, 0, 255},
		"Raw":       {63, 0, 255}, // floor(0.25 * 255)
	}
	for name, w := range want {
		if got[name] != w {
			t.Errorf("%s default/min/max = %v, want %v", name, got[name], w)
		}
	}
}

// A percentage defaultValue selects the switching capability it falls in.
func TestBuildImportPlan_SwitchingPercentDefaultSelectsCapability(t *testing.T) {
	js := `{"name":"X","categories":["Dimmer"],"availableChannels":{
		"Mode": {"defaultValue": "90%", "capabilities": [
			{"dmxRange":[0,127],"type":"Generic","switchChannels":{"Alias":"Speed"}},
			{"dmxRange":[128,255],"type":"Generic","switchChannels":{"Alias":"Sens"}}
		]},
		"Speed": {"capability": {"type": "Speed"}},
		"Sens": {"capability": {"type": "Intensity"}}
	},"modes":[{"name":"m","channels":["Alias"]}]}`
	f, _ := parseOFLFixture(js)
	plan, err := buildImportPlan(f)
	if err != nil {
		t.Fatal(err)
	}
	if c := plan.Channels[plan.Modes[0].channelIdx[0]]; c.Type != "INTENSITY" {
		t.Errorf("alias type = %s, want INTENSITY (target Sens)", c.Type)
	}
}

func TestParseDMXValue(t *testing.T) {
	tests := []struct {
		raw   string
		bytes int
		want  int
		ok    bool
	}{
		{``, 1, 0, false},
		{`128`, 1, 128, true},
		{`999`, 1, 255, true},
		{`-5`, 1, 0, true},
		{`"100%"`, 2, 65535, true},
		{`"0%"`, 1, 0, true},
		{`"abc%"`, 1, 0, false},
		{`"fast"`, 1, 0, false},
		{`true`, 1, 0, false},
	}
	for _, tt := range tests {
		got, ok := parseDMXValue(json.RawMessage(tt.raw), tt.bytes)
		if got != tt.want || ok != tt.ok {
			t.Errorf("parseDMXValue(%s, %d) = %d, %v; want %d, %v", tt.raw, tt.bytes, got, ok, tt.want, tt.ok)
		}
	}
}

func TestResolution(t *testing.T) {
	r := resolutionOf(OFLChannel{FineChannelAliases: []string{"a"}, DMXValueResolution: "24bit"})
	if r.channelBytes != 2 || r.valueBytes != 2 {
		t.Errorf("resolution clamped = %+v", r)
	}
	r = resolutionOf(OFLChannel{FineChannelAliases: []string{"a", "b"}, DMXValueResolution: "16bit"})
	if r.valueBytes != 2 || r.toFull(0x1234) != 0x123434 || r.byteAt(0x123456, 2) != 0x56 || r.byteAt(1, 5) != 0 {
		t.Errorf("24-bit channel with 16-bit values: %+v", r)
	}
	// A channel without capability still honours an explicit default.
	if d := channelDefFromOFL("x", OFLChannel{DefaultValue: json.RawMessage(`7`)}); d.DefaultValue != 7 || d.Type != "OTHER" {
		t.Errorf("no-capability default = %+v", d)
	}
}

// A real channel named like the null placeholder is not confused with it.
func TestBuildImportPlan_NullPlaceholderNameCollision(t *testing.T) {
	js := `{"name":"X","categories":["Dimmer"],"availableChannels":{"No function":{"capability":{"type":"Intensity"}}},
		"modes":[{"name":"m","channels":["No function",null]}]}`
	f, _ := parseOFLFixture(js)
	plan, err := buildImportPlan(f)
	if err != nil {
		t.Fatal(err)
	}
	m := plan.Modes[0]
	if m.channelIdx[0] == m.channelIdx[1] {
		t.Fatal("null slot resolved to the real 'No function' channel")
	}
	if plan.Channels[m.channelIdx[0]].Type != "INTENSITY" || plan.Channels[m.channelIdx[1]].Type != "OTHER" {
		t.Errorf("types = %s, %s", plan.Channels[m.channelIdx[0]].Type, plan.Channels[m.channelIdx[1]].Type)
	}
}

func TestBuildImportPlan_RejectsHugeMatrix(t *testing.T) {
	js := `{"name":"X","categories":["Matrix"],"matrix":{"pixelCount":[100000,100000,100000]},
		"templateChannels":{"D $pixelKey":{}},"modes":[{"name":"m","channels":["D 1"]}]}`
	f, _ := parseOFLFixture(js)
	if _, err := buildImportPlan(f); !errors.Is(err, ErrUnsupportedModeChannel) {
		t.Errorf("err = %v, want ErrUnsupportedModeChannel", err)
	}
}

func TestNaturalLess_LongDigitRuns(t *testing.T) {
	a := "pixel 99999999999999999999999"
	b := "pixel 100000000000000000000000"
	if !naturalLess(a, b) || naturalLess(b, a) {
		t.Error("long digit runs must compare numerically")
	}
	if !naturalLess("x007", "x8") {
		t.Error("leading zeros should be ignored")
	}
	if !naturalLess("x12", "x13") {
		t.Error("equal-length digit runs compare lexically")
	}
}

func TestCoarseRange(t *testing.T) {
	r := resolution{channelBytes: 2, valueBytes: 2}
	tests := []struct{ start, end, wantStart, wantEnd int }{
		{1, 65535, 1, 255},       // non-zero dropped byte rounds the start up
		{0, 65535, 0, 255},       // zero dropped byte keeps the start
		{256, 511, 1, 1},         // exact byte boundary
		{257, 300, 1, 1},         // rounding up would pass the end: keep floor
		{32768, 65535, 128, 255}, // upper half
	}
	for _, tt := range tests {
		s, e := r.coarseRange(tt.start, tt.end)
		if s != tt.wantStart || e != tt.wantEnd {
			t.Errorf("coarseRange(%d, %d) = %d, %d; want %d, %d", tt.start, tt.end, s, e, tt.wantStart, tt.wantEnd)
		}
	}
	if s, e := (resolution{channelBytes: 1, valueBytes: 1}).coarseRange(10, 20); s != 10 || e != 20 {
		t.Errorf("8-bit range changed: %d, %d", s, e)
	}
}

func TestBuildImportPlan_ModeChannelBudget(t *testing.T) {
	// 600 pixels x 1 template channel is more than one universe.
	big := `{"name":"X","categories":["Matrix"],"matrix":{"pixelCount":[600,1,1]},
		"templateChannels":{"D $pixelKey":{}},
		"modes":[{"name":"m","channels":[{"insert":"matrixChannels","repeatFor":"eachPixelABC","channelOrder":"perPixel","templateChannels":["D $pixelKey"]}]}]}`
	f, _ := parseOFLFixture(big)
	if _, err := buildImportPlan(f); !errors.Is(err, ErrUnsupportedModeChannel) {
		t.Errorf("oversized insert: err = %v, want ErrUnsupportedModeChannel", err)
	}

	// One pixel with 513 template entries is rejected as well.
	tmpls := make([]string, 513)
	for i := range tmpls {
		tmpls[i] = `"D $pixelKey"`
	}
	wide := `{"name":"X","categories":["Matrix"],"templateChannels":{"D $pixelKey":{}},
		"modes":[{"name":"m","channels":[{"insert":"matrixChannels","repeatFor":["1"],"channelOrder":"perPixel","templateChannels":[` + strings.Join(tmpls, ",") + `]}]}]}`
	f, _ = parseOFLFixture(wide)
	if _, err := buildImportPlan(f); !errors.Is(err, ErrUnsupportedModeChannel) {
		t.Errorf("wide insert: err = %v, want ErrUnsupportedModeChannel", err)
	}

	// A plain channel list longer than 512 entries is rejected too.
	entries := make([]string, 513)
	for i := range entries {
		entries[i] = "null"
	}
	long := `{"name":"X","categories":["Dimmer"],"availableChannels":{"D":{}},
		"modes":[{"name":"m","channels":[` + strings.Join(entries, ",") + `]}]}`
	f, _ = parseOFLFixture(long)
	if _, err := buildImportPlan(f); !errors.Is(err, ErrUnsupportedModeChannel) {
		t.Errorf("long mode: err = %v, want ErrUnsupportedModeChannel", err)
	}

	// Exactly 512 is fine.
	f, _ = parseOFLFixture(`{"name":"X","categories":["Dimmer"],"availableChannels":{"D":{}},
		"modes":[{"name":"m","channels":[` + strings.Join(entries[:512], ",") + `]}]}`)
	if _, err := buildImportPlan(f); err != nil {
		t.Errorf("512-channel mode rejected: %v", err)
	}
}

func TestOFLMatrix_PixelKeysABCDeterministicTies(t *testing.T) {
	one, zeroOne := "1", "01"
	for i := 0; i < 20; i++ {
		m := &OFLMatrix{PixelKeys: [][][]*string{{{&one, &zeroOne}}}}
		if got := fmt.Sprint(m.pixelKeysABC()); got != "[01 1]" {
			t.Fatalf("iteration %d: pixelKeysABC = %s, want [01 1]", i, got)
		}
	}
}

// When several template patterns match, the one with the longest literal
// part wins: "Red 1" is the "Red $pixelKey" template, not "$pixelKey".
func TestBuildImportPlan_TemplateLongestLiteralWins(t *testing.T) {
	js := `{"name":"X","categories":["Pixel Bar"],
		"templateChannels":{
			"$pixelKey":{"capability":{"type":"Intensity"}},
			"Red $pixelKey":{"capability":{"type":"ColorIntensity","color":"Red"}}
		},
		"modes":[{"name":"m","channels":["Red 1","Solo"]}]}`
	f, _ := parseOFLFixture(js)
	plan, err := buildImportPlan(f)
	if err != nil {
		t.Fatal(err)
	}
	m := plan.Modes[0]
	if got := plan.Channels[m.channelIdx[0]].Type; got != "RED" {
		t.Errorf("Red 1 type = %s, want RED", got)
	}
	if got := plan.Channels[m.channelIdx[1]].Type; got != "INTENSITY" {
		t.Errorf("Solo type = %s, want INTENSITY", got)
	}
}

func TestValidPixelCount(t *testing.T) {
	for _, tt := range []struct {
		c    [3]int
		want bool
	}{
		{[3]int{16, 1, 1}, true},
		{[3]int{256, 256, 1}, true},
		{[3]int{256, 256, 2}, false},
		{[3]int{-1, 1, 1}, false},
		{[3]int{1 << 30, 1 << 30, 1}, false},
		{[3]int{65536, 65536, 1}, false}, // wraps to 0 in 32-bit int if multiplied first
		{[3]int{65536, 1, 1}, true},
		{[3]int{0, 5, 5}, true},
	} {
		if got := validPixelCount(tt.c); got != tt.want {
			t.Errorf("validPixelCount(%v) = %v, want %v", tt.c, got, tt.want)
		}
	}
}
