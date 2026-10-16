package gdtf

import "testing"

func TestChannelDisplayName(t *testing.T) {
	tests := []struct {
		attr, pretty, want string
	}{
		{"Dimmer", "Dim", "Dimmer"},    // terse Pretty ignored
		{"Pan", "P", "Pan"},            // terse Pretty ignored
		{"ColorAdd_RY", "RY", "Amber"}, // friendly name
		{"colorADD_gy", "", "Lime"},    // case-insensitive
		{"HSB_Hue", "", "Hue"},
		{"Gobo1PosRotate", "Gobo Rotate", "Gobo Rotate"}, // descriptive Pretty kept
		{"Shutter1Strobe", "Sh1Str", "Shutter 1 Strobe"},
		{"Color1", "Color1", "Color 1"},
		{"PositionMSpeed", "PTMS", "Position Speed"},
		{"FixtureGlobalReset", "", "Fixture Global Reset"},
		{"CTO", "CTO", "CTO"},
		{"Shutter1", "Shutter", "Shutter"}, // one descriptive word kept
		{"Zoom", "ZOOM", "Zoom"},           // all-caps Pretty ignored
		{"Gobo1", "Go", "Gobo 1"},
		{"XYZ_X", "", "XYZ X"},
		{"VendorThing", "", "Vendor Thing"},
		{"", "Custom", "Custom"},
		{"", "", placeholderName},
		{"NoFeature", "", placeholderName},
	}
	for _, tt := range tests {
		if got := channelDisplayName(tt.attr, tt.pretty); got != tt.want {
			t.Errorf("channelDisplayName(%q, %q) = %q, want %q", tt.attr, tt.pretty, got, tt.want)
		}
	}
}

func TestHumanizeAttribute(t *testing.T) {
	tests := map[string]string{
		"Gobo2WheelSpin":  "Gobo 2 Wheel Spin",
		"MSpeed":          "MSpeed",
		"ColorMacro1Rate": "Color Macro 1 Rate",
		"Effects1Adjust2": "Effects 1 Adjust 2",
		"Focus_Distance":  "Focus Distance",
		"LampControl":     "Lamp Control",
	}
	for in, want := range tests {
		if got := humanizeAttribute(in); got != want {
			t.Errorf("humanizeAttribute(%q) = %q, want %q", in, got, want)
		}
	}
}
