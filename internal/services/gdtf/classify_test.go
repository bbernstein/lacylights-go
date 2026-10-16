package gdtf

import "testing"

func TestClassifyAttribute(t *testing.T) {
	tests := []struct {
		attr, feature, want string
	}{
		{"Dimmer", "", typeIntensity},
		{"dimmer", "", typeIntensity}, // case-insensitive
		{"HSB_Brightness", "", typeIntensity},
		{"Pan", "", typePan},
		{"Tilt", "", typeTilt},
		{"ColorAdd_R", "", typeRed},
		{"ColorRGB_Green", "", typeGreen},
		{"ColorAdd_B", "", typeBlue},
		{"ColorAdd_W", "", typeWhite},
		{"ColorAdd_WW", "", typeWarmWhite},
		{"ColorAdd_CW", "", typeColdWhite},
		{"ColorAdd_RY", "", typeAmber},
		{"ColorAdd_GY", "", typeLime},
		{"ColorAdd_UV", "", typeUV},
		{"ColorSub_C", "", typeCyan},
		{"ColorAdd_M", "", typeMagenta},
		{"ColorRGB_Yellow", "", typeYellow},
		{"CTO", "", typeColorTemperature},
		{"CTC", "", typeColorTemperature},
		{"CTB", "", typeColorTemperature},
		{"HSB_Hue", "", typeHue},
		{"HSB_Saturation", "", typeSaturation},
		{"Color1", "", typeColorWheel},
		{"Color2WheelIndex", "", typeColorWheel},
		{"Gobo1", "", typeGobo},
		{"Gobo2WheelIndex", "", typeGobo},
		{"Gobo1SelectSpin", "", typeGobo},
		{"Gobo1SelectShake", "", typeGobo},
		{"Gobo2Pos", "", typeEffect},
		{"Gobo1PosRotate", "", typeEffect},
		{"Gobo1WheelSpin", "", typeEffect},
		{"Prism1", "", typeEffect},
		{"Prism1PosRotate", "", typeEffect},
		{"Effects1", "", typeEffect},
		{"Effects1Adjust1", "", typeEffect},
		{"Effects1Rate", "", typeSpeed},
		{"ColorMacro1Rate", "", typeSpeed},
		{"PanRotate", "", typeEffect},
		{"Zoom", "", typeZoom},
		{"ZoomModeSpot", "", typeZoom},
		{"Focus1", "", typeFocus},
		{"Iris", "", typeIris},
		{"Shutter1", "", typeStrobe},
		{"Shutter1Strobe", "", typeStrobe},
		{"Shutter2StrobePulse", "", typeStrobe},
		{"StrobeRate", "", typeStrobe},
		{"PositionMSpeed", "", typeSpeed},
		{"IntensityMSpeed", "", typeSpeed},
		{"Control1", "", typeMacro},
		{"DimmerMode", "Control.Control", typeMacro},
		{"PanMode", "", typeMacro},
		{"CTOReset", "", typeMacro},
		{"FixtureGlobalReset", "", typeMacro},
		{"LampControl", "", typeMacro},
		{"Function", "", typeMacro},
		{"ColorMacro1", "", typeMacro},
		{"ColorAdd_GC", "", typeOther},
		{"ColorSub_R", "", typeOther},
		{"Tint", "", typeOther},
		{"CIE_X", "", typeOther},
		{"Frost1", "", typeOther},
		{"Fan1", "", typeOther},
		{"Fog1", "", typeOther},
		{"Haze1", "", typeOther},
		{"Dummy", "", typeOther},
		{"NoFeature", "", typeOther},
		// Unknown (custom) attributes fall back on Feature.
		{"MyCustomIntensity", "Dimmer.Dimmer", typeIntensity},
		{"VendorSpecial", "Control.Control", typeMacro},
		{"VendorSpecial", "control.control", typeMacro},
		{"VendorSpecial", "Beam.Beam", typeOther},
		{"VendorSpecial", "", typeOther},
		// A "*Mode" attribute outside the control group is not a macro.
		{"VendorBeamMode", "Beam.Beam", typeOther},
	}
	for _, tt := range tests {
		t.Run(tt.attr+"/"+tt.feature, func(t *testing.T) {
			if got := classifyAttribute(tt.attr, tt.feature); got != tt.want {
				t.Errorf("classifyAttribute(%q, %q) = %s, want %s", tt.attr, tt.feature, got, tt.want)
			}
		})
	}
}

func TestNormalizeAttribute(t *testing.T) {
	tests := map[string]string{
		"Gobo2Pos":        "gobo(n)pos",
		"Effects1Adjust2": "effects(n)adjust(n)",
		"Gobo(n)Pos":      "gobo(n)pos",
		"ColorAdd_R":      "coloradd_r",
		" Pan ":           "pan",
	}
	for in, want := range tests {
		if got := normalizeAttribute(in); got != want {
			t.Errorf("normalizeAttribute(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestFixtureTypeFor(t *testing.T) {
	tests := []struct {
		name  string
		types []string
		want  string
	}{
		{"pan and tilt", []string{typePan, typeTilt, typeIntensity, typeRed}, "MOVING_HEAD"},
		{"pan only is not a mover", []string{typePan, typeIntensity}, "OTHER"},
		{"rgb", []string{typeRed, typeGreen, typeBlue}, "LED_PAR"},
		{"hue/sat", []string{typeIntensity, typeHue, typeSaturation}, "LED_PAR"},
		{"dimmer", []string{typeIntensity}, "DIMMER"},
		{"dimmer with strobe and control", []string{typeIntensity, typeStrobe, typeMacro}, "DIMMER"},
		{"strobe only", []string{typeStrobe}, "STROBE"},
		{"strobe dominant", []string{typeIntensity, typeStrobe, typeStrobe}, "STROBE"},
		{"colour temperature is not colour mixing", []string{typeIntensity, typeColorTemperature}, "OTHER"},
		{"scroller", []string{typeIntensity, typeColorWheel, typeSpeed}, "OTHER"},
		{"empty", nil, "OTHER"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := fixtureTypeFor(tt.types); got != tt.want {
				t.Errorf("fixtureTypeFor(%v) = %s, want %s", tt.types, got, tt.want)
			}
		})
	}
}
