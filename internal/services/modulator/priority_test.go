package modulator

import (
	"testing"
)

func TestPriorityBandOrdering(t *testing.T) {
	// Verify band ordering: BASE < USER < CUE < SYSTEM
	tests := []struct {
		name   string
		lower  PriorityBand
		higher PriorityBand
	}{
		{"BASE < USER", PriorityBandBase, PriorityBandUser},
		{"USER < CUE", PriorityBandUser, PriorityBandCue},
		{"CUE < SYSTEM", PriorityBandCue, PriorityBandSystem},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p1 := EffectPriority{Band: tt.lower, SubPriority: 50}
			p2 := EffectPriority{Band: tt.higher, SubPriority: 50}

			if !p1.Less(p2) {
				t.Errorf("%s: expected p1.Less(p2) to be true", tt.name)
			}
			if p2.Less(p1) {
				t.Errorf("%s: expected p2.Less(p1) to be false", tt.name)
			}
		})
	}
}

func TestSubPriorityWithinBand(t *testing.T) {
	// Verify sub-priority ordering within same band
	p1 := EffectPriority{Band: PriorityBandUser, SubPriority: 30}
	p2 := EffectPriority{Band: PriorityBandUser, SubPriority: 70}

	if !p1.Less(p2) {
		t.Error("expected p1.Less(p2) to be true (30 < 70)")
	}
	if p2.Less(p1) {
		t.Error("expected p2.Less(p1) to be false (70 > 30)")
	}
}

func TestSubPriorityDoesNotOverrideBand(t *testing.T) {
	// High sub-priority in lower band still loses to low sub-priority in higher band
	userHigh := EffectPriority{Band: PriorityBandUser, SubPriority: 100}
	cueLow := EffectPriority{Band: PriorityBandCue, SubPriority: 0}

	if !userHigh.Less(cueLow) {
		t.Error("USER band with sub-priority 100 should be less than CUE band with sub-priority 0")
	}
}

func TestPriorityEquality(t *testing.T) {
	p1 := EffectPriority{Band: PriorityBandUser, SubPriority: 50}
	p2 := EffectPriority{Band: PriorityBandUser, SubPriority: 50}

	if p1.Compare(p2) != 0 {
		t.Error("expected Compare to return 0 for equal priorities")
	}
	if p1.Less(p2) {
		t.Error("expected Less to return false for equal priorities")
	}
	if p2.Less(p1) {
		t.Error("expected Less to return false for equal priorities")
	}
	if !p1.Equal(p2) {
		t.Error("expected Equal to return true for equal priorities")
	}
}

func TestPriorityCompare(t *testing.T) {
	tests := []struct {
		name     string
		p1       EffectPriority
		p2       EffectPriority
		expected int
	}{
		{
			"same priority",
			EffectPriority{PriorityBandUser, 50},
			EffectPriority{PriorityBandUser, 50},
			0,
		},
		{
			"lower band",
			EffectPriority{PriorityBandUser, 50},
			EffectPriority{PriorityBandCue, 50},
			-1,
		},
		{
			"higher band",
			EffectPriority{PriorityBandCue, 50},
			EffectPriority{PriorityBandUser, 50},
			1,
		},
		{
			"same band lower sub",
			EffectPriority{PriorityBandUser, 30},
			EffectPriority{PriorityBandUser, 70},
			-1,
		},
		{
			"same band higher sub",
			EffectPriority{PriorityBandUser, 70},
			EffectPriority{PriorityBandUser, 30},
			1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.p1.Compare(tt.p2)
			if result != tt.expected {
				t.Errorf("Compare returned %d, expected %d", result, tt.expected)
			}
		})
	}
}

func TestNewEffectPriority(t *testing.T) {
	// Test clamping of sub-priority
	tests := []struct {
		name           string
		band           PriorityBand
		subPriority    int
		expectedSub    int
	}{
		{"normal value", PriorityBandUser, 50, 50},
		{"negative clamped", PriorityBandUser, -10, 0},
		{"over 100 clamped", PriorityBandUser, 150, 100},
		{"zero", PriorityBandUser, 0, 0},
		{"max", PriorityBandUser, 100, 100},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := NewEffectPriority(tt.band, tt.subPriority)
			if p.SubPriority != tt.expectedSub {
				t.Errorf("SubPriority = %d, expected %d", p.SubPriority, tt.expectedSub)
			}
			if p.Band != tt.band {
				t.Errorf("Band = %v, expected %v", p.Band, tt.band)
			}
		})
	}
}

func TestPriorityBandString(t *testing.T) {
	tests := []struct {
		band     PriorityBand
		expected string
	}{
		{PriorityBandBase, "BASE"},
		{PriorityBandUser, "USER"},
		{PriorityBandCue, "CUE"},
		{PriorityBandSystem, "SYSTEM"},
		{PriorityBand(99), "UNKNOWN"},
	}

	for _, tt := range tests {
		if tt.band.String() != tt.expected {
			t.Errorf("PriorityBand(%d).String() = %q, expected %q", tt.band, tt.band.String(), tt.expected)
		}
	}
}

func TestParsePriorityBand(t *testing.T) {
	tests := []struct {
		input    string
		expected PriorityBand
	}{
		{"BASE", PriorityBandBase},
		{"USER", PriorityBandUser},
		{"CUE", PriorityBandCue},
		{"SYSTEM", PriorityBandSystem},
		{"unknown", PriorityBandBase}, // Defaults to BASE
		{"", PriorityBandBase},
	}

	for _, tt := range tests {
		result := ParsePriorityBand(tt.input)
		if result != tt.expected {
			t.Errorf("ParsePriorityBand(%q) = %v, expected %v", tt.input, result, tt.expected)
		}
	}
}

func TestPriorityGreater(t *testing.T) {
	p1 := EffectPriority{Band: PriorityBandCue, SubPriority: 50}
	p2 := EffectPriority{Band: PriorityBandUser, SubPriority: 50}

	if !p1.Greater(p2) {
		t.Error("expected CUE to be greater than USER")
	}
	if p2.Greater(p1) {
		t.Error("expected USER to not be greater than CUE")
	}
}

func TestDefaultPriorities(t *testing.T) {
	// Verify default priorities are set correctly
	if PriorityUserDefault.Band != PriorityBandUser || PriorityUserDefault.SubPriority != 50 {
		t.Errorf("PriorityUserDefault incorrect: %+v", PriorityUserDefault)
	}
	if PriorityCueDefault.Band != PriorityBandCue || PriorityCueDefault.SubPriority != 50 {
		t.Errorf("PriorityCueDefault incorrect: %+v", PriorityCueDefault)
	}
	if PriorityBlackout.Band != PriorityBandSystem || PriorityBlackout.SubPriority != 100 {
		t.Errorf("PriorityBlackout incorrect: %+v", PriorityBlackout)
	}
	if PriorityGrandMaster.Band != PriorityBandSystem || PriorityGrandMaster.SubPriority != 50 {
		t.Errorf("PriorityGrandMaster incorrect: %+v", PriorityGrandMaster)
	}

	// Verify blackout has higher priority than grand master
	if !PriorityBlackout.Greater(PriorityGrandMaster) {
		t.Error("Blackout should have higher priority than Grand Master")
	}
}
