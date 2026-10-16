package gdtf

import (
	"fmt"
	"reflect"
	"strings"
	"testing"
)

func TestBuildPlan_RGBWPar(t *testing.T) {
	p := mustPlan(t, readTestdata(t, "rgbw_par.xml"))
	if p.Manufacturer != "Synthetic Lighting Co" || p.Model != "Test Par RGBW" {
		t.Errorf("manufacturer/model = %q / %q", p.Manufacturer, p.Model)
	}
	if p.SourceID != "5A1C0B7E-0000-4000-8000-000000000001" {
		t.Errorf("SourceID = %q, want upper-cased FixtureTypeID", p.SourceID)
	}
	if p.SourceVersion != "1.2 (2026-02-03T11:30:00 Added 6ch mode)" {
		t.Errorf("SourceVersion = %q", p.SourceVersion)
	}
	if p.Type != "LED_PAR" {
		t.Errorf("Type = %s, want LED_PAR", p.Type)
	}
	if got := p.modeNames(); !reflect.DeepEqual(got, []string{"4ch", "6ch"}) {
		t.Fatalf("modes = %v", got)
	}

	four := p.channels(p.mode(t, "4ch"))
	if got := channelNames(four); !reflect.DeepEqual(got, []string{"Red", "Green", "Blue", "White"}) {
		t.Errorf("4ch channels = %v", got)
	}
	wantTypes := []string{typeRed, typeGreen, typeBlue, typeWhite}
	for i, ch := range four {
		if ch.Type != wantTypes[i] || ch.FadeBehavior != "FADE" || ch.IsDiscrete || ch.DefaultValue != 0 {
			t.Errorf("4ch[%d] = %+v", i, ch)
		}
	}

	six := p.channels(p.mode(t, "6ch"))
	if got := channelNames(six); !reflect.DeepEqual(got, []string{"Dimmer", "Red", "Green", "Blue", "White", "Shutter 1"}) {
		t.Errorf("6ch channels = %v", got)
	}
	shutter := six[5]
	// InitialFunction "Open" has no Default and there is no legacy
	// DMXChannel@Default, so its DMXFrom (10) is the default.
	if shutter.Type != typeStrobe || shutter.DefaultValue != 10 || !shutter.IsDiscrete || shutter.FadeBehavior != "SNAP" {
		t.Errorf("shutter = %+v", shutter)
	}
	// Red/Green/Blue/White are shared between the modes.
	if len(p.Channels) != 6 {
		t.Errorf("deduplicated channel count = %d, want 6 (%v)", len(p.Channels), channelNames(p.Channels))
	}
}

func TestBuildPlan_Mover16Bit(t *testing.T) {
	p := mustPlan(t, readTestdata(t, "mover16.xml"))
	if p.Model != "TestSpot" {
		t.Errorf("Model = %q, want Name when LongName is empty", p.Model)
	}
	if p.Type != "MOVING_HEAD" {
		t.Errorf("Type = %s, want MOVING_HEAD", p.Type)
	}
	if p.SourceVersion != "1.2" {
		t.Errorf("SourceVersion = %q, want 1.2 without revisions", p.SourceVersion)
	}
	std := p.channels(p.mode(t, "Standard 16bit"))
	type want struct {
		name, typ string
		def       int
		fade      string
		discrete  bool
	}
	wants := []want{
		{"Pan", typePan, 128, "FADE", false},      // 128/1 mirrored to 16 bit = 0x8080
		{"Pan Fine", typePan, 128, "FADE", false}, //
		{"Tilt", typeTilt, 128, "FADE", false},    // 128/1s shifted to 16 bit = 0x8000
		{"Tilt Fine", typeTilt, 0, "FADE", false}, //
		{"PT Speed", typeSpeed, 0, "FADE", false}, // PositionMSpeed
		{"Color Wheel", typeColorWheel, 0, "SNAP", true},
		{"Gobo", typeGobo, 0, "SNAP", true},
		{"Gobo Rotate", typeEffect, 128, "FADE", false},
		{"Shutter", typeStrobe, 40, "SNAP", true}, // InitialFunction Open's Default
		{"Dimmer", typeIntensity, 0, "FADE", false},
		{"Dimmer Fine", typeIntensity, 0, "FADE", false},
	}
	if len(std) != len(wants) {
		t.Fatalf("Standard 16bit has %d channels (%v), want %d", len(std), channelNames(std), len(wants))
	}
	for i, w := range wants {
		got := std[i]
		if got.Name != w.name || got.Type != w.typ || got.DefaultValue != w.def || got.FadeBehavior != w.fade || got.IsDiscrete != w.discrete {
			t.Errorf("channel %d = %+v, want %+v", i+1, got, w)
		}
	}
	basic := p.channels(p.mode(t, "Basic 8bit"))
	if got := channelNames(basic); !reflect.DeepEqual(got, []string{"Pan", "Tilt", "Dimmer"}) {
		t.Errorf("Basic 8bit = %v", got)
	}
	// The 8-bit Pan/Tilt/Dim equal the 16-bit coarse channels and are shared.
	if len(p.Channels) != 11 {
		t.Errorf("deduplicated channel count = %d, want 11", len(p.Channels))
	}
}

func TestBuildPlan_PixelBarGeometryReferences(t *testing.T) {
	p := mustPlan(t, readTestdata(t, "pixelbar.xml"))
	m := p.mode(t, "13ch")
	got := channelNames(p.channels(m))
	want := []string{"Dimmer"}
	for _, px := range []string{"Pixel 1", "Pixel 2", "Pixel 3", "Pixel 4"} {
		want = append(want, px+" Red", px+" Green", px+" Blue")
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("13ch channels =\n%v\nwant\n%v", got, want)
	}
	if p.channels(m)[0].DefaultValue != 255 {
		t.Errorf("master dimmer default = %d, want 255", p.channels(m)[0].DefaultValue)
	}
	if p.Type != "LED_PAR" {
		t.Errorf("Type = %s", p.Type)
	}
}

func TestBuildPlan_MultiBreakSplitsModes(t *testing.T) {
	p := mustPlan(t, readTestdata(t, "multibreak.xml"))
	if got := p.modeNames(); !reflect.DeepEqual(got, []string{"Dimmer + Scroller (Break 1)", "Dimmer + Scroller (Break 2)"}) {
		t.Fatalf("modes = %v", got)
	}
	if got := channelNames(p.channels(p.Modes[0])); !reflect.DeepEqual(got, []string{"Dimmer"}) {
		t.Errorf("break 1 = %v", got)
	}
	b2 := p.channels(p.Modes[1])
	if got := channelNames(b2); !reflect.DeepEqual(got, []string{"Scroller", "Scroll Speed"}) {
		t.Errorf("break 2 = %v", got)
	}
	if b2[0].Type != typeColorWheel || b2[1].Type != typeSpeed {
		t.Errorf("break 2 types = %s, %s", b2[0].Type, b2[1].Type)
	}
	if !hasWarning(p.Warnings, "one mode per break") {
		t.Errorf("expected a multi-break warning, got %v", p.Warnings)
	}
}

func TestBuildPlan_OverwriteBreakUsesLastBreak(t *testing.T) {
	p := mustPlan(t, readTestdata(t, "overwrite.xml"))
	if got := p.modeNames(); !reflect.DeepEqual(got, []string{"Cells (Break 1)", "Cells (Break 2)", "Cells (Break 3)"}) {
		t.Fatalf("modes = %v", got)
	}
	// Break 1: the shutter of each cell at Offset + DMXOffset - 1, with a gap.
	if got := channelNames(p.channels(p.Modes[0])); !reflect.DeepEqual(got, []string{"Cell A Shutter", placeholderName, "Cell B Shutter"}) {
		t.Errorf("break 1 = %v", got)
	}
	// Overwrite: Cell A's last Break is break 2 offset 1, Cell B's is break 3 offset 5.
	if got := channelNames(p.channels(p.Modes[1])); !reflect.DeepEqual(got, []string{"Dimmer"}) {
		t.Errorf("break 2 = %v", got)
	}
	b3 := channelNames(p.channels(p.Modes[2]))
	if len(b3) != 5 || b3[4] != "Dimmer" || b3[0] != placeholderName {
		t.Errorf("break 3 = %v, want 4 placeholders then Dimmer", b3)
	}
}

func TestBuildPlan_VirtualChannelsAndGaps(t *testing.T) {
	channels := dmxChannel("1", "Dimmer", "", `<ChannelFunction Name="Dimmer" Attribute="Dimmer" DMXFrom="0/1"/>`) +
		dmxChannel("None", "ColorAdd_R", "", `<ChannelFunction Name="Red" Attribute="ColorAdd_R" DMXFrom="0/1"/>`) +
		dmxChannel("-", "ColorAdd_G", "", `<ChannelFunction Name="Green" Attribute="ColorAdd_G" DMXFrom="0/1"/>`) +
		dmxChannel("4", "NoFeature", "", `<ChannelFunction Name="NoFeature" Attribute="NoFeature" DMXFrom="0/1"/>`) +
		dmxChannel("3", "Pan", "", `<ChannelFunction Name="Pan" Attribute="Pan" DMXFrom="0/1"/>`)
	p := mustPlan(t, inlineXML("1.2", "6b9b8c2e-1111-4111-8111-000000000010", channels))
	got := channelNames(p.channels(p.Modes[0]))
	want := []string{"Dimmer", placeholderName, "Pan", placeholderName}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("channels = %v, want %v", got, want)
	}
	for _, ch := range p.Channels {
		if ch.Name == "Red" || ch.Name == "Green" {
			t.Errorf("virtual channel %q was imported", ch.Name)
		}
	}
	// The gap and the NoFeature filler share one placeholder definition.
	m := p.Modes[0]
	if m.channelIdx[1] != m.channelIdx[3] {
		t.Errorf("placeholders not shared: %v", m.channelIdx)
	}
	if ph := p.Channels[m.channelIdx[1]]; ph.Type != typeOther || ph.DefaultValue != 0 {
		t.Errorf("placeholder = %+v", ph)
	}
}

func TestBuildPlan_Defaults(t *testing.T) {
	tests := []struct {
		name     string
		version  string
		channel  string
		wantVals []int
	}{
		{
			name:     "InitialFunction selects the second function",
			version:  "1.2",
			channel:  dmxChannel("1", "Dimmer", `InitialFunction="Beam_Dimmer.Dimmer.Full"`, `<ChannelFunction Name="Low" Attribute="Dimmer" DMXFrom="0/1" Default="5/1"/><ChannelFunction Name="Full" Attribute="Dimmer" DMXFrom="128/1" Default="200/1"/>`),
			wantVals: []int{200},
		},
		{
			name:     "default InitialFunction is the first function",
			version:  "1.2",
			channel:  dmxChannel("1", "Dimmer", "", `<ChannelFunction Name="Low" Attribute="Dimmer" DMXFrom="0/1" Default="5/1"/><ChannelFunction Name="Full" Attribute="Dimmer" DMXFrom="128/1" Default="200/1"/>`),
			wantVals: []int{5},
		},
		{
			name:     "unknown InitialFunction falls back to the first function",
			version:  "1.2",
			channel:  dmxChannel("1", "Dimmer", `InitialFunction="Beam_Dimmer.Dimmer.Missing"`, `<ChannelFunction Name="Low" Attribute="Dimmer" DMXFrom="0/1" Default="7/1"/>`),
			wantVals: []int{7},
		},
		{
			name:     "InitialFunction by default function name",
			version:  "1.2",
			channel:  dmxChannel("1", "Dimmer", `InitialFunction="Beam_Dimmer.Dimmer.Dimmer 2"`, `<ChannelFunction Attribute="Dimmer" DMXFrom="0/1" Default="1/1"/><ChannelFunction Attribute="Dimmer" DMXFrom="100/1" Default="150/1"/>`),
			wantVals: []int{150},
		},
		{
			name:     "legacy DMXChannel@Default (GDTF 1.1)",
			version:  "1.1",
			channel:  dmxChannel("1", "Dimmer", `Default="77/1"`, `<ChannelFunction Name="Dimmer" Attribute="Dimmer" DMXFrom="10/1"/>`),
			wantVals: []int{77},
		},
		{
			name:     "DMXFrom when no Default is given",
			version:  "1.2",
			channel:  dmxChannel("1", "Dimmer", "", `<ChannelFunction Name="Dimmer" Attribute="Dimmer" DMXFrom="10/1"/>`),
			wantVals: []int{10},
		},
		{
			name:     "16-bit mirrored default to coarse/fine",
			version:  "1.2",
			channel:  dmxChannel("1,2", "Pan", "", `<ChannelFunction Name="Pan" Attribute="Pan" DMXFrom="0/2" Default="255/1"/>`),
			wantVals: []int{255, 255},
		},
		{
			name:     "16-bit shifted default to coarse/fine",
			version:  "1.2",
			channel:  dmxChannel("1,2", "Pan", "", `<ChannelFunction Name="Pan" Attribute="Pan" DMXFrom="0/2" Default="255/1s"/>`),
			wantVals: []int{255, 0},
		},
		{
			name:     "16-bit native default",
			version:  "1.2",
			channel:  dmxChannel("1,2", "Pan", "", `<ChannelFunction Name="Pan" Attribute="Pan" DMXFrom="0/2" Default="4660/2"/>`),
			wantVals: []int{0x12, 0x34},
		},
		{
			name:     "24-bit default names a second fine channel",
			version:  "1.2",
			channel:  dmxChannel("1,2,3", "Pan", "", `<ChannelFunction Name="Pan" Attribute="Pan" DMXFrom="0/3" Default="128/1"/>`),
			wantVals: []int{128, 128, 128},
		},
		{
			name:     "no channel functions",
			version:  "1.2",
			channel:  dmxChannel("1", "Dimmer", "", ``),
			wantVals: []int{0},
		},
		{
			name:     "invalid Default is ignored",
			version:  "1.2",
			channel:  dmxChannel("1", "Dimmer", "", `<ChannelFunction Name="Dimmer" Attribute="Dimmer" DMXFrom="9/1" Default="oops"/>`),
			wantVals: []int{9},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := mustPlan(t, inlineXML(tt.version, "6b9b8c2e-1111-4111-8111-000000000011", tt.channel))
			chs := p.channels(p.Modes[0])
			if len(chs) != len(tt.wantVals) {
				t.Fatalf("got %d channels (%v), want %d", len(chs), channelNames(chs), len(tt.wantVals))
			}
			for i, w := range tt.wantVals {
				if chs[i].DefaultValue != w {
					t.Errorf("byte %d default = %d, want %d", i, chs[i].DefaultValue, w)
				}
			}
			if len(chs) == 3 && chs[2].Name != "Pan Fine 2" {
				t.Errorf("third byte name = %q, want Pan Fine 2", chs[2].Name)
			}
		})
	}
}

func TestBuildPlan_SnapAndDiscrete(t *testing.T) {
	tests := []struct {
		name         string
		channel      string
		wantFade     string
		wantDiscrete bool
	}{
		{"single function", dmxChannel("1", "Dimmer", "", `<ChannelFunction Name="D" Attribute="Dimmer" DMXFrom="0/1"/>`), "FADE", false},
		{"multiple functions", dmxChannel("1", "Dimmer", "", `<ChannelFunction Name="A" Attribute="Dimmer" DMXFrom="0/1"/><ChannelFunction Name="B" Attribute="Dimmer" DMXFrom="128/1"/>`), "SNAP", true},
		{"ModeMaster alternatives", dmxChannel("1", "Dimmer", "", `<ChannelFunction Name="A" Attribute="Dimmer" DMXFrom="0/1" ModeMaster="Beam_Pan" ModeFrom="0/1" ModeTo="127/1"/><ChannelFunction Name="B" Attribute="Dimmer" DMXFrom="0/1" ModeMaster="Beam_Pan" ModeFrom="128/1" ModeTo="255/1"/>`), "FADE", false},
		{"ModeMaster group with two functions", dmxChannel("1", "Dimmer", "", `<ChannelFunction Name="A" Attribute="Dimmer" DMXFrom="0/1" ModeMaster="Beam_Pan" ModeFrom="0/1" ModeTo="127/1"/><ChannelFunction Name="B" Attribute="Dimmer" DMXFrom="100/1" ModeMaster="Beam_Pan" ModeFrom="0/1" ModeTo="127/1"/>`), "SNAP", true},
		{"ungated function plus a ModeMaster group", dmxChannel("1", "Dimmer", "", `<ChannelFunction Name="U" Attribute="Dimmer" DMXFrom="0/1"/><ChannelFunction Name="A" Attribute="Dimmer" DMXFrom="0/1" ModeMaster="Beam_Pan" ModeFrom="0/1" ModeTo="127/1"/><ChannelFunction Name="B" Attribute="Dimmer" DMXFrom="100/1" ModeMaster="Beam_Pan" ModeFrom="0/1" ModeTo="127/1"/>`), "SNAP", true},
		{"wheel slot sets", dmxChannel("1", "Dimmer", "", `<ChannelFunction Name="A" Attribute="Dimmer" DMXFrom="0/1"><ChannelSet Name="x" DMXFrom="0/1" WheelSlotIndex="1"/></ChannelFunction>`), "SNAP", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := mustPlan(t, inlineXML("1.2", "6b9b8c2e-1111-4111-8111-000000000012", tt.channel))
			ch := p.channels(p.Modes[0])[0]
			if ch.FadeBehavior != tt.wantFade || ch.IsDiscrete != tt.wantDiscrete {
				t.Errorf("fade=%s discrete=%v, want %s %v", ch.FadeBehavior, ch.IsDiscrete, tt.wantFade, tt.wantDiscrete)
			}
		})
	}

	snap := strings.Replace(dmxChannel("1", "Dimmer", "", `<ChannelFunction Name="D" Attribute="Dimmer" DMXFrom="0/1"/>`),
		`<LogicalChannel Attribute="Dimmer">`, `<LogicalChannel Attribute="Dimmer" Snap="Yes">`, 1)
	for _, value := range []string{"Yes", "On", "No", "Off"} {
		xml := inlineXML("1.2", "6b9b8c2e-1111-4111-8111-000000000013", strings.Replace(snap, `Snap="Yes"`, `Snap="`+value+`"`, 1))
		ch := mustPlan(t, xml).Channels[0]
		want := "FADE"
		if value == "Yes" || value == "On" {
			want = "SNAP"
		}
		if ch.FadeBehavior != want {
			t.Errorf("Snap=%s: fade = %s, want %s", value, ch.FadeBehavior, want)
		}
	}
}

func TestBuildPlan_Warnings(t *testing.T) {
	tests := []struct {
		name     string
		channels string
		want     string
	}{
		{"overlapping addresses", dmxChannel("1", "Dimmer", "", "") + dmxChannel("1", "Pan", "", ""), "used by more than one channel"},
		{"invalid offset", dmxChannel("x", "Dimmer", "", "") + dmxChannel("1", "Pan", "", ""), "invalid offset"},
		{"too many offset bytes", dmxChannel("1,2,3,4,5", "Dimmer", "", "") + dmxChannel("1", "Pan", "", ""), "more than 4 bytes"},
		{"beyond one universe", dmxChannel("600", "Dimmer", "", "") + dmxChannel("1", "Pan", "", ""), "beyond address 512"},
		{"invalid break", strings.Replace(dmxChannel("1", "Dimmer", "", ""), `DMXBreak="1"`, `DMXBreak="zero"`, 1), "invalid DMXBreak"},
		{"overwrite outside a reference", strings.Replace(dmxChannel("1", "Dimmer", "", ""), `DMXBreak="1"`, `DMXBreak="Overwrite"`, 1), "outside a geometry reference"},
		{"geometry outside the tree", strings.Replace(dmxChannel("1", "Dimmer", "", ""), `Geometry="Beam"`, `Geometry="Elsewhere"`, 1), "outside the mode geometry"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := mustPlan(t, inlineXML("1.2", "6b9b8c2e-1111-4111-8111-000000000014", tt.channels))
			if !hasWarning(p.Warnings, tt.want) {
				t.Errorf("warnings %v do not mention %q", p.Warnings, tt.want)
			}
		})
	}
}

func TestBuildPlan_Errors(t *testing.T) {
	allVirtual := inlineXML("1.2", "6b9b8c2e-1111-4111-8111-000000000015", dmxChannel("None", "Dimmer", "", ""))
	doc, w, err := parseDescription(allVirtual)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := buildPlan(doc, w); err == nil || !strings.Contains(err.Error(), "no DMX mode has patchable channels") {
		t.Errorf("all-virtual fixture: err = %v", err)
	}

	badRoot := strings.Replace(inlineXML("1.2", "6b9b8c2e-1111-4111-8111-000000000016", dmxChannel("1", "Dimmer", "", "")),
		`<Geometries><Geometry Name="Body"><Beam Name="Beam"/></Geometry></Geometries>`,
		`<Geometries><Geometry Name="A"/><Geometry Name="B"/></Geometries>`, 1)
	doc, w, err = parseDescription(badRoot)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := buildPlan(doc, w); err == nil || !strings.Contains(err.Error(), "not a top-level geometry") {
		t.Errorf("unknown mode geometry: err = %v", err)
	}
}

func TestBuildPlan_SingleGeometryFallbackAndUnnamedMode(t *testing.T) {
	x := inlineXML("1.2", "6b9b8c2e-1111-4111-8111-000000000017", dmxChannel("1", "Dimmer", "", ""))
	x = strings.Replace(x, `<DMXMode Name="Mode" Geometry="Body">`, `<DMXMode Name="" Geometry="Missing">`, 1)
	p := mustPlan(t, x)
	if p.Modes[0].Name != "Mode 1" {
		t.Errorf("mode name = %q, want Mode 1", p.Modes[0].Name)
	}
	if !hasWarning(p.Warnings, `using "Body"`) {
		t.Errorf("warnings = %v", p.Warnings)
	}
}

func TestBuildPlan_UniqueNames(t *testing.T) {
	// Two modes with the same name and two channels with the same Pretty
	// name on the same geometry.
	x := inlineXML("1.2", "6b9b8c2e-1111-4111-8111-000000000018",
		dmxChannel("1", "Dimmer", "", "")+dmxChannel("2", "Dimmer", "", ""))
	x = strings.Replace(x, `</DMXModes>`, `<DMXMode Name="Mode" Geometry="Body"><DMXChannels>`+dmxChannel("1", "Pan", "", "")+`</DMXChannels></DMXMode></DMXModes>`, 1)
	p := mustPlan(t, x)
	if got := p.modeNames(); !reflect.DeepEqual(got, []string{"Mode", "Mode (2)"}) {
		t.Errorf("modes = %v", got)
	}
	if got := channelNames(p.channels(p.Modes[0])); !reflect.DeepEqual(got, []string{"Beam Dimmer", "Beam Dimmer 2"}) {
		t.Errorf("channels = %v", got)
	}
}

func TestSourceVersion(t *testing.T) {
	doc := &xmlGDTF{DataVersion: "1.2"}
	if got := sourceVersion(doc); got != "1.2" {
		t.Errorf("no revisions: %q", got)
	}
	doc.FixtureType.Revisions = []xmlRevision{{Date: "2020-01-01"}, {Text: "Only text"}}
	if got := sourceVersion(doc); got != "1.2 (Only text)" {
		t.Errorf("text only: %q", got)
	}
	doc.FixtureType.Revisions = []xmlRevision{{}}
	if got := sourceVersion(doc); got != "1.2" {
		t.Errorf("empty revision: %q", got)
	}
}

func TestParseDMXAddress(t *testing.T) {
	tests := map[string]int{"1": 1, "17": 17, "2.5": 517, " 3 ": 3}
	for in, want := range tests {
		if got, ok := parseDMXAddress(in); !ok || got != want {
			t.Errorf("parseDMXAddress(%q) = %d, %v", in, got, ok)
		}
	}
	for _, bad := range []string{"", "x", "0", "1.x", "0.1", "1.513", "99999999.1", "9223372036854775807.1"} {
		if _, ok := parseDMXAddress(bad); ok {
			t.Errorf("parseDMXAddress(%q) should fail", bad)
		}
	}
}

func TestDisplayName(t *testing.T) {
	b := &planBuilder{attrs: map[string]*xmlAttribute{
		"dimmer":  {Name: "Dimmer", Pretty: "Dim"},
		"pan":     {Name: "Pan"},
		"unnamed": {},
	}}
	tests := map[string]string{
		"Dimmer":      "Dimmer",       // terse Pretty "Dim" ignored
		"DIMMER":      "Dimmer",       // case-insensitive lookup
		"Pan":         "Pan",          // no Pretty: attribute name
		"VendorThing": "Vendor Thing", // not defined: the link, humanized
		"unnamed":     "unnamed",      // defined without names
		"":            placeholderName,
	}
	for attr, want := range tests {
		if got := b.displayName(attr); got != want {
			t.Errorf("displayName(%q) = %q, want %q", attr, got, want)
		}
	}
}

func TestBuildPlan_ChannelWithoutLogicalChannel(t *testing.T) {
	x := inlineXML("1.2", "6b9b8c2e-1111-4111-8111-000000000019",
		`<DMXChannel DMXBreak="1" Offset="1" Geometry="Beam"/>`+dmxChannel("2", "Dimmer", "", ""))
	p := mustPlan(t, x)
	if got := channelNames(p.channels(p.Modes[0])); !reflect.DeepEqual(got, []string{placeholderName, "Dimmer"}) {
		t.Errorf("channels = %v", got)
	}
}

func TestBuildPlan_ReferenceToUnknownGeometry(t *testing.T) {
	x := strings.Replace(readTestdata(t, "pixelbar.xml"),
		`<GeometryReference Name="Pixel 4" Geometry="Pixel">`, `<GeometryReference Name="Pixel 4" Geometry="Nope">`, 1)
	p := mustPlan(t, x)
	if !hasWarning(p.Warnings, `points at unknown geometry "Nope"`) {
		t.Errorf("warnings = %v", p.Warnings)
	}
	if n := len(p.Modes[0].channelIdx); n != 10 {
		t.Errorf("13ch has %d channels without Pixel 4, want 10", n)
	}
}

func TestBuildPlan_ReferenceWithoutMatchingBreak(t *testing.T) {
	x := strings.Replace(readTestdata(t, "pixelbar.xml"),
		`<GeometryReference Name="Pixel 1" Geometry="Pixel"><Break DMXBreak="1" DMXOffset="2"/></GeometryReference>`,
		`<GeometryReference Name="Pixel 1" Geometry="Pixel"><Break DMXBreak="2" DMXOffset="40"/></GeometryReference>`, 1)
	p := mustPlan(t, x)
	if !hasWarning(p.Warnings, `"Pixel 1" has no Break for DMX break 1`) {
		t.Errorf("warnings = %v", p.Warnings)
	}
}

func TestBuildPlan_HugeOffsetsDoNotOverflow(t *testing.T) {
	// Offset + DMXOffset would wrap to a negative address on 64-bit if added
	// before the bounds check.
	for _, tc := range []struct{ offset, dmxOffset string }{
		{"9223372036854775807", "2"},
		{"2", "9223372036854775807"},
	} {
		x := strings.Replace(readTestdata(t, "pixelbar.xml"),
			`<GeometryReference Name="Pixel 1" Geometry="Pixel"><Break DMXBreak="1" DMXOffset="2"/>`,
			`<GeometryReference Name="Pixel 1" Geometry="Pixel"><Break DMXBreak="1" DMXOffset="`+tc.dmxOffset+`"/>`, 1)
		x = strings.Replace(x, `<DMXChannel DMXBreak="1" Offset="1" Geometry="Pixel">`,
			`<DMXChannel DMXBreak="1" Offset="`+tc.offset+`" Geometry="Pixel">`, 1)
		p := mustPlan(t, x)
		if !hasWarning(p.Warnings, "beyond address 512") {
			t.Errorf("offset %s + DMXOffset %s: warnings = %v", tc.offset, tc.dmxOffset, p.Warnings)
		}
		for _, m := range p.Modes {
			if n := len(m.channelIdx); n > maxBreakChannels {
				t.Errorf("mode %s has %d channels", m.Name, n)
			}
		}
	}
}

func TestBuildPlan_ExpansionBudget(t *testing.T) {
	var refs strings.Builder
	for i := range maxModeInstances + 1 {
		fmt.Fprintf(&refs, `<GeometryReference Name="P%d" Geometry="Pixel"><Break DMXBreak="1" DMXOffset="1"/></GeometryReference>`, i)
	}
	x := strings.Replace(readTestdata(t, "pixelbar.xml"),
		`<GeometryReference Name="Pixel 1" Geometry="Pixel"><Break DMXBreak="1" DMXOffset="2"/></GeometryReference>`,
		refs.String(), 1)
	doc, w, err := parseDescription(x)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := buildPlan(doc, w); err == nil || !strings.Contains(err.Error(), "expands to more than") {
		t.Errorf("err = %v, want expansion budget error", err)
	}
}
