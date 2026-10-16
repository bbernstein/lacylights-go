package gdtf

import (
	"errors"
	"strings"
	"testing"
)

func TestCheckVersion(t *testing.T) {
	tests := []struct {
		version  string
		wantErr  bool
		wantWarn string
	}{
		{version: "1.2"},
		{version: " 1.2 "},
		{version: "1.1", wantWarn: "best-effort"},
		{version: "1.3", wantWarn: "newer than the supported version"},
		{version: "2.0", wantWarn: "newer than the supported version"},
		{version: "1.0", wantErr: true},
		{version: "0.9", wantErr: true},
		{version: "", wantErr: true},
		{version: "1", wantErr: true},
		{version: "a.b", wantErr: true},
		{version: "1.-1", wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.version, func(t *testing.T) {
			warns, err := checkVersion(tt.version)
			if (err != nil) != tt.wantErr {
				t.Fatalf("err = %v, wantErr %v", err, tt.wantErr)
			}
			if err != nil && !errors.Is(err, ErrUnsupportedVersion) {
				t.Errorf("err %v is not ErrUnsupportedVersion", err)
			}
			if tt.wantWarn == "" && len(warns) > 0 {
				t.Errorf("unexpected warnings %v", warns)
			}
			if tt.wantWarn != "" && !hasWarning(warns, tt.wantWarn) {
				t.Errorf("warnings %v do not mention %q", warns, tt.wantWarn)
			}
		})
	}
}

func TestParseDescription_Errors(t *testing.T) {
	valid := inlineXML("1.2", "6b9b8c2e-1111-4111-8111-000000000020", dmxChannel("1", "Dimmer", "", ""))
	tests := []struct {
		name string
		xml  string
		want string
	}{
		{"malformed", "<GDTF DataVersion=\"1.2\"><FixtureType>", "XML"},
		{"not GDTF", "<Other/>", "XML"},
		{"1.0 rejected", strings.Replace(valid, `DataVersion="1.2"`, `DataVersion="1.0"`, 1), "GDTF 1.0 is not supported"},
		{"no FixtureTypeID", strings.Replace(valid, `FixtureTypeID="6b9b8c2e-1111-4111-8111-000000000020"`, ``, 1), "no FixtureTypeID"},
		{"no name", strings.Replace(strings.Replace(valid, `Name="Inline" `, ``, 1), `LongName="Inline Test" `, ``, 1), "no Name"},
		{"no manufacturer", strings.Replace(valid, `Manufacturer="Synthetic Lighting Co"`, ``, 1), "no Manufacturer"},
		{"no modes", strings.Replace(valid, `<DMXMode Name="Mode" Geometry="Body"><DMXChannels>`+dmxChannel("1", "Dimmer", "", "")+`</DMXChannels></DMXMode>`, ``, 1), "no DMX modes"},
		{"unsupported encoding", strings.Replace(valid, `encoding="UTF-8"`, `encoding="UTF-16"`, 1), "unsupported XML encoding"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, _, err := parseDescription(tt.xml)
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Errorf("err = %v, want it to mention %q", err, tt.want)
			}
		})
	}
}

func TestParseDescription_BOMAndASCII(t *testing.T) {
	valid := inlineXML("1.2", "6b9b8c2e-1111-4111-8111-000000000021", dmxChannel("1", "Dimmer", "", ""))
	if _, _, err := parseDescription("\xef\xbb\xbf" + valid); err != nil {
		t.Errorf("BOM: %v", err)
	}
	if _, _, err := parseDescription(strings.Replace(valid, `encoding="UTF-8"`, `encoding="us-ascii"`, 1)); err != nil {
		t.Errorf("us-ascii: %v", err)
	}
}

func TestParseDescription_GeometryKinds(t *testing.T) {
	doc, _, err := parseDescription(readTestdata(t, "pixelbar.xml"))
	if err != nil {
		t.Fatal(err)
	}
	g := doc.FixtureType.Geometries.Children
	if len(g) != 2 || g[0].Name != "Bar" || len(g[0].Children) != 4 {
		t.Fatalf("geometries = %+v", g)
	}
	ref := g[0].Children[0]
	if !ref.isReference() || ref.Geometry != "Pixel" || len(ref.Children) != 1 || !ref.Children[0].isBreak() || ref.Children[0].DMXOffset != "2" {
		t.Errorf("reference = %+v", ref)
	}
}
