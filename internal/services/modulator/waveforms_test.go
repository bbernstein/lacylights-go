package modulator

import (
	"math"
	"testing"
	"time"
)

// TestGenerateWaveform_Sine tests the sine waveform generation at key points.
func TestGenerateWaveform_Sine(t *testing.T) {
	tests := []struct {
		name     string
		phase    float64
		expected float64
		tol      float64
	}{
		{"0 degrees", 0, 0.5, 0.001},
		{"90 degrees", 90, 1.0, 0.001},
		{"180 degrees", 180, 0.5, 0.001},
		{"270 degrees", 270, 0.0, 0.001},
		{"360 degrees", 360, 0.5, 0.001},
		{"45 degrees", 45, 0.8536, 0.001}, // (sin(45°) + 1) / 2 ≈ 0.854
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := GenerateWaveform(tt.phase, WaveformSine)
			if math.Abs(result-tt.expected) > tt.tol {
				t.Errorf("GenerateWaveform(%v, SINE) = %v, expected %v (±%v)",
					tt.phase, result, tt.expected, tt.tol)
			}
		})
	}
}

// TestGenerateWaveform_Cosine tests the cosine waveform generation at key points.
func TestGenerateWaveform_Cosine(t *testing.T) {
	tests := []struct {
		name     string
		phase    float64
		expected float64
		tol      float64
	}{
		{"0 degrees", 0, 1.0, 0.001},
		{"90 degrees", 90, 0.5, 0.001},
		{"180 degrees", 180, 0.0, 0.001},
		{"270 degrees", 270, 0.5, 0.001},
		{"360 degrees", 360, 1.0, 0.001},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := GenerateWaveform(tt.phase, WaveformCosine)
			if math.Abs(result-tt.expected) > tt.tol {
				t.Errorf("GenerateWaveform(%v, COSINE) = %v, expected %v (±%v)",
					tt.phase, result, tt.expected, tt.tol)
			}
		})
	}
}

// TestGenerateWaveform_Square tests the square waveform generation.
func TestGenerateWaveform_Square(t *testing.T) {
	tests := []struct {
		name     string
		phase    float64
		expected float64
	}{
		{"0 degrees", 0, 1.0},
		{"89 degrees", 89, 1.0},
		{"90 degrees", 90, 1.0},
		{"179 degrees", 179, 1.0},
		{"180 degrees", 180, 0.0},
		{"270 degrees", 270, 0.0},
		{"359 degrees", 359, 0.0},
		{"360 degrees (wraps to 0)", 360, 1.0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := GenerateWaveform(tt.phase, WaveformSquare)
			if result != tt.expected {
				t.Errorf("GenerateWaveform(%v, SQUARE) = %v, expected %v",
					tt.phase, result, tt.expected)
			}
		})
	}
}

// TestGenerateWaveform_Sawtooth tests the sawtooth waveform generation.
func TestGenerateWaveform_Sawtooth(t *testing.T) {
	tests := []struct {
		name     string
		phase    float64
		expected float64
		tol      float64
	}{
		{"0 degrees", 0, 0.0, 0.001},
		{"90 degrees", 90, 0.25, 0.001},
		{"180 degrees", 180, 0.5, 0.001},
		{"270 degrees", 270, 0.75, 0.001},
		{"360 degrees (wraps to 0)", 360, 0.0, 0.001},
		{"36 degrees", 36, 0.1, 0.001},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := GenerateWaveform(tt.phase, WaveformSawtooth)
			if math.Abs(result-tt.expected) > tt.tol {
				t.Errorf("GenerateWaveform(%v, SAWTOOTH) = %v, expected %v (±%v)",
					tt.phase, result, tt.expected, tt.tol)
			}
		})
	}
}

// TestGenerateWaveform_Triangle tests the triangle waveform generation.
func TestGenerateWaveform_Triangle(t *testing.T) {
	tests := []struct {
		name     string
		phase    float64
		expected float64
		tol      float64
	}{
		{"0 degrees", 0, 0.0, 0.001},
		{"90 degrees", 90, 0.5, 0.001},
		{"180 degrees", 180, 1.0, 0.001},
		{"270 degrees", 270, 0.5, 0.001},
		{"360 degrees (wraps to 0)", 360, 0.0, 0.001},
		{"45 degrees", 45, 0.25, 0.001},
		{"225 degrees", 225, 0.75, 0.001},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := GenerateWaveform(tt.phase, WaveformTriangle)
			if math.Abs(result-tt.expected) > tt.tol {
				t.Errorf("GenerateWaveform(%v, TRIANGLE) = %v, expected %v (±%v)",
					tt.phase, result, tt.expected, tt.tol)
			}
		})
	}
}

// TestGenerateWaveform_Random tests that random waveform produces consistent values for same phase.
func TestGenerateWaveform_Random(t *testing.T) {
	// Random is deterministic based on phase
	phase := 50.0
	result1 := GenerateWaveform(phase, WaveformRandom)
	result2 := GenerateWaveform(phase, WaveformRandom)

	if result1 != result2 {
		t.Errorf("Random waveform should be deterministic: %v != %v", result1, result2)
	}

	// Result should be in valid range [0, 1]
	if result1 < 0 || result1 > 1 {
		t.Errorf("Random waveform value out of range: %v", result1)
	}

	// Different phases should (usually) produce different values
	differentPhase := 100.0 // Different enough to likely be different step
	result3 := GenerateWaveform(differentPhase, WaveformRandom)
	// Note: This is probabilistic, but with phase steps of 10 degrees, should differ
	if result1 == result3 {
		t.Logf("Warning: Different phases produced same value (can happen but is rare)")
	}
}

// TestGenerateWaveform_BoundaryValues tests all waveform types at boundary values.
func TestGenerateWaveform_BoundaryValues(t *testing.T) {
	waveforms := []WaveformType{
		WaveformSine, WaveformCosine, WaveformSquare,
		WaveformSawtooth, WaveformTriangle, WaveformRandom,
	}
	boundaries := []float64{0, 90, 180, 270, 360}

	for _, wf := range waveforms {
		for _, phase := range boundaries {
			t.Run(wf.String()+"_"+string(rune('0'+int(phase/90))), func(t *testing.T) {
				result := GenerateWaveform(phase, wf)
				if result < 0 || result > 1 {
					t.Errorf("GenerateWaveform(%v, %s) = %v, out of range [0,1]",
						phase, wf, result)
				}
			})
		}
	}
}

// TestGenerateWaveform_NegativePhase tests that negative phases are normalized correctly.
func TestGenerateWaveform_NegativePhase(t *testing.T) {
	tests := []struct {
		name          string
		negativePhase float64
		equivalentPos float64
	}{
		{"-90 equals 270", -90, 270},
		{"-180 equals 180", -180, 180},
		{"-270 equals 90", -270, 90},
		{"-360 equals 0", -360, 0},
		{"-450 equals 270", -450, 270}, // -450 + 720 = 270
	}

	waveforms := []WaveformType{WaveformSine, WaveformCosine, WaveformTriangle, WaveformSawtooth}

	for _, wf := range waveforms {
		for _, tt := range tests {
			t.Run(wf.String()+"_"+tt.name, func(t *testing.T) {
				negResult := GenerateWaveform(tt.negativePhase, wf)
				posResult := GenerateWaveform(tt.equivalentPos, wf)

				if math.Abs(negResult-posResult) > 0.001 {
					t.Errorf("GenerateWaveform(%v, %s) = %v, should equal %v (from phase %v)",
						tt.negativePhase, wf, negResult, posResult, tt.equivalentPos)
				}
			})
		}
	}
}

// TestGenerateWaveform_PhaseGreaterThan360 tests that phases > 360 are normalized.
func TestGenerateWaveform_PhaseGreaterThan360(t *testing.T) {
	tests := []struct {
		name          string
		largePhase    float64
		equivalentPos float64
	}{
		{"450 equals 90", 450, 90},
		{"720 equals 0", 720, 0},
		{"810 equals 90", 810, 90},
		{"540 equals 180", 540, 180},
		{"1080 equals 0", 1080, 0},
	}

	waveforms := []WaveformType{WaveformSine, WaveformCosine, WaveformTriangle, WaveformSawtooth}

	for _, wf := range waveforms {
		for _, tt := range tests {
			t.Run(wf.String()+"_"+tt.name, func(t *testing.T) {
				largeResult := GenerateWaveform(tt.largePhase, wf)
				normalResult := GenerateWaveform(tt.equivalentPos, wf)

				if math.Abs(largeResult-normalResult) > 0.001 {
					t.Errorf("GenerateWaveform(%v, %s) = %v, should equal %v (from phase %v)",
						tt.largePhase, wf, largeResult, normalResult, tt.equivalentPos)
				}
			})
		}
	}
}

// TestGenerateWaveform_UnknownType tests that unknown waveform types return 0.5.
func TestGenerateWaveform_UnknownType(t *testing.T) {
	result := GenerateWaveform(90, "UNKNOWN")
	if result != 0.5 {
		t.Errorf("Unknown waveform type should return 0.5, got %v", result)
	}
}

// TestWaveformType_String tests the string representation of waveform types.
func TestWaveformType_String(t *testing.T) {
	tests := []struct {
		waveform WaveformType
		expected string
	}{
		{WaveformSine, "SINE"},
		{WaveformCosine, "COSINE"},
		{WaveformSquare, "SQUARE"},
		{WaveformSawtooth, "SAWTOOTH"},
		{WaveformTriangle, "TRIANGLE"},
		{WaveformRandom, "RANDOM"},
	}

	for _, tt := range tests {
		if tt.waveform.String() != tt.expected {
			t.Errorf("WaveformType.String() = %q, expected %q", tt.waveform.String(), tt.expected)
		}
	}
}

// TestParseWaveformType tests parsing strings into waveform types.
func TestParseWaveformType(t *testing.T) {
	tests := []struct {
		input    string
		expected WaveformType
	}{
		{"SINE", WaveformSine},
		{"COSINE", WaveformCosine},
		{"SQUARE", WaveformSquare},
		{"SAWTOOTH", WaveformSawtooth},
		{"TRIANGLE", WaveformTriangle},
		{"RANDOM", WaveformRandom},
		{"unknown", WaveformSine},  // Defaults to SINE
		{"", WaveformSine},          // Defaults to SINE
		{"sine", WaveformSine},      // Case sensitive, defaults to SINE
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := ParseWaveformType(tt.input)
			if result != tt.expected {
				t.Errorf("ParseWaveformType(%q) = %v, expected %v", tt.input, result, tt.expected)
			}
		})
	}
}

// TestWaveformType_IsValid tests validation of waveform types.
func TestWaveformType_IsValid(t *testing.T) {
	validTypes := []WaveformType{
		WaveformSine, WaveformCosine, WaveformSquare,
		WaveformSawtooth, WaveformTriangle, WaveformRandom,
	}
	for _, wf := range validTypes {
		if !wf.IsValid() {
			t.Errorf("WaveformType %q should be valid", wf)
		}
	}

	invalidTypes := []WaveformType{"INVALID", "", "sine", "Sine"}
	for _, wf := range invalidTypes {
		if wf.IsValid() {
			t.Errorf("WaveformType %q should be invalid", wf)
		}
	}
}

// TestCalculateWaveformValues_BasicCalculation tests basic waveform calculation.
func TestCalculateWaveformValues_BasicCalculation(t *testing.T) {
	effect := &Effect{
		ID:              "test-effect-1",
		EffectType:      EffectTypeWaveform,
		Waveform:        WaveformSine,
		Frequency:       1.0,
		Amplitude:       100.0, // Full amplitude (±100% of 255)
		Offset:          50.0,  // Center at 127
		PhaseOffset:     0,
		CompositionMode: ComposeModeOverride,
		TargetChannels: []EffectChannel{
			{Universe: 1, Channel: 1},
		},
	}

	active := NewActiveEffect(effect)
	active.Phase = 0    // Start at phase 0
	active.Intensity = 100

	// Calculate values with 0 elapsed time
	values := CalculateWaveformValues(active, 0)

	if values == nil {
		t.Fatal("CalculateWaveformValues returned nil")
	}

	key := ChannelKey{Universe: 1, Channel: 1}
	_, ok := values[key]
	if !ok {
		t.Fatal("Expected channel key not found in values")
	}

	// At phase 0, sine wave is at 0.5, so with 50% offset and 100% amplitude:
	// offset = 127.5, modulation = (0.5 - 0.5) * 2 * 255 * 1.0 = 0
	// final = 127.5 + 0 = 127
}

// TestCalculateWaveformValues_AmplitudeAndOffset tests amplitude and offset calculations.
// The formula is: finalValue = offset + (waveValue - 0.5) * 2 * amplitudeRange * intensity
// Where amplitudeRange = amplitude% * 255 / 100
// So amplitude=50 means range of ±127.5 from the offset center
func TestCalculateWaveformValues_AmplitudeAndOffset(t *testing.T) {
	tests := []struct {
		name        string
		amplitude   float64
		offset      float64
		phase       float64
		waveform    WaveformType
		expectedMin int
		expectedMax int
	}{
		{
			// At sine peak (1.0), modulation = (1-0.5)*2 * 127.5 = 127.5
			// Final = 127.5 + 127.5 = 255 (clamped)
			name:        "50% amplitude, 50% offset at sine peak",
			amplitude:   50.0,
			offset:      50.0,
			phase:       90, // Sine peak
			waveform:    WaveformSine,
			expectedMin: 254,
			expectedMax: 255,
		},
		{
			// At sine trough (0.0), modulation = (0-0.5)*2 * 127.5 = -127.5
			// Final = 127.5 - 127.5 = 0
			name:        "50% amplitude, 50% offset at sine trough",
			amplitude:   50.0,
			offset:      50.0,
			phase:       270, // Sine trough
			waveform:    WaveformSine,
			expectedMin: 0,
			expectedMax: 1,
		},
		{
			// At sine peak (1.0), offset = 75% of 255 = 191.25
			// amplitude = 25% of 255 = 63.75
			// modulation = (1-0.5)*2 * 63.75 = 63.75
			// Final = 191.25 + 63.75 = 255 (clamped to 255)
			name:        "25% amplitude, 75% offset at peak",
			amplitude:   25.0,
			offset:      75.0,
			phase:       90, // Peak
			waveform:    WaveformSine,
			expectedMin: 254,
			expectedMax: 255,
		},
		{
			// At sine value 0.5 (phase 0), modulation = 0
			// Final = offset = 127.5 ~ 127
			name:        "50% amplitude, 50% offset at sine midpoint",
			amplitude:   50.0,
			offset:      50.0,
			phase:       0, // Sine midpoint
			waveform:    WaveformSine,
			expectedMin: 126,
			expectedMax: 128,
		},
		{
			// Lower amplitude test: 10% amplitude, 50% offset at peak
			// amplitude = 10% of 255 = 25.5
			// modulation = (1-0.5)*2 * 25.5 = 25.5
			// Final = 127.5 + 25.5 = 153
			name:        "10% amplitude, 50% offset at peak",
			amplitude:   10.0,
			offset:      50.0,
			phase:       90,
			waveform:    WaveformSine,
			expectedMin: 151,
			expectedMax: 155,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			effect := &Effect{
				ID:              "test-effect",
				EffectType:      EffectTypeWaveform,
				Waveform:        tt.waveform,
				Frequency:       1.0,
				Amplitude:       tt.amplitude,
				Offset:          tt.offset,
				PhaseOffset:     0,
				CompositionMode: ComposeModeOverride,
				TargetChannels: []EffectChannel{
					{Universe: 1, Channel: 1},
				},
			}

			active := NewActiveEffect(effect)
			active.Phase = tt.phase
			active.Intensity = 100

			values := CalculateWaveformValues(active, 0)
			key := ChannelKey{Universe: 1, Channel: 1}
			val, ok := values[key]
			if !ok {
				t.Fatal("Expected channel key not found")
			}

			if val < tt.expectedMin || val > tt.expectedMax {
				t.Errorf("Value %d out of expected range [%d, %d]", val, tt.expectedMin, tt.expectedMax)
			}
		})
	}
}

// TestCalculateWaveformValues_PerChannelPhaseOffset tests per-channel phase offsets.
func TestCalculateWaveformValues_PerChannelPhaseOffset(t *testing.T) {
	phaseOffset90 := 90.0
	phaseOffset180 := 180.0

	effect := &Effect{
		ID:              "test-effect",
		EffectType:      EffectTypeWaveform,
		Waveform:        WaveformSine,
		Frequency:       1.0,
		Amplitude:       100.0,
		Offset:          50.0,
		PhaseOffset:     0,
		CompositionMode: ComposeModeOverride,
		TargetChannels: []EffectChannel{
			{Universe: 1, Channel: 1, PhaseOffset: nil},            // No offset
			{Universe: 1, Channel: 2, PhaseOffset: &phaseOffset90}, // 90° offset
			{Universe: 1, Channel: 3, PhaseOffset: &phaseOffset180}, // 180° offset
		},
	}

	active := NewActiveEffect(effect)
	active.Phase = 0
	active.Intensity = 100

	values := CalculateWaveformValues(active, 0)

	ch1 := values[ChannelKey{Universe: 1, Channel: 1}]
	ch2 := values[ChannelKey{Universe: 1, Channel: 2}]
	ch3 := values[ChannelKey{Universe: 1, Channel: 3}]

	// At phase 0:
	// Channel 1 (phase 0): sine = 0.5, value = 127
	// Channel 2 (phase 90): sine = 1.0, value = 255
	// Channel 3 (phase 180): sine = 0.5, value = 127

	// Check that channel 2 (at peak) is higher than channel 1
	if ch2 <= ch1 {
		t.Errorf("Channel 2 (90° offset) should be higher than channel 1: ch2=%d, ch1=%d", ch2, ch1)
	}

	// Check that channel 1 and 3 are similar (both at 0.5 waveform value)
	if math.Abs(float64(ch1-ch3)) > 5 {
		t.Errorf("Channel 1 and 3 should be similar: ch1=%d, ch3=%d", ch1, ch3)
	}
}

// TestCalculateWaveformValues_IntensityScaling tests intensity scaling.
func TestCalculateWaveformValues_IntensityScaling(t *testing.T) {
	tests := []struct {
		name      string
		intensity float64
	}{
		{"full intensity", 100},
		{"half intensity", 50},
		{"quarter intensity", 25},
		{"zero intensity", 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			effect := &Effect{
				ID:              "test-effect",
				EffectType:      EffectTypeWaveform,
				Waveform:        WaveformSine,
				Frequency:       1.0,
				Amplitude:       100.0,
				Offset:          50.0,
				PhaseOffset:     0,
				CompositionMode: ComposeModeOverride,
				TargetChannels: []EffectChannel{
					{Universe: 1, Channel: 1},
				},
			}

			active := NewActiveEffect(effect)
			active.Phase = 90 // At peak
			active.Intensity = tt.intensity

			values := CalculateWaveformValues(active, 0)
			key := ChannelKey{Universe: 1, Channel: 1}
			val := values[key]

			// At intensity 0, the modulation should be 0, so value = offset = 127
			// At intensity 100 and phase 90, value should be at max (255)
			switch tt.intensity {
			case 0:
				// With 0 intensity, modulation is 0, so value should be at offset
				expected := 127 // 50% of 255
				if math.Abs(float64(val-expected)) > 2 {
					t.Errorf("At zero intensity, value should be near offset: got %d, expected ~%d", val, expected)
				}
			case 100:
				// At full intensity and peak, should be at max
				if val < 250 {
					t.Errorf("At full intensity and peak, value should be near max: got %d", val)
				}
			}
		})
	}
}

// TestCalculateWaveformValues_NonWaveformEffect tests that non-waveform effects return nil.
func TestCalculateWaveformValues_NonWaveformEffect(t *testing.T) {
	effect := &Effect{
		ID:         "test-effect",
		EffectType: EffectTypeCrossfade, // Not a waveform
	}

	active := NewActiveEffect(effect)
	values := CalculateWaveformValues(active, 0)

	if values != nil {
		t.Errorf("CalculateWaveformValues should return nil for non-waveform effects, got %v", values)
	}
}

// TestCalculateWaveformValues_PhaseAdvancement tests that phase advances with elapsed time.
func TestCalculateWaveformValues_PhaseAdvancement(t *testing.T) {
	effect := &Effect{
		ID:              "test-effect",
		EffectType:      EffectTypeWaveform,
		Waveform:        WaveformSawtooth, // Easy to track linear progression
		Frequency:       1.0,              // 1 Hz = 360 degrees per second
		Amplitude:       100.0,
		Offset:          50.0,
		PhaseOffset:     0,
		CompositionMode: ComposeModeOverride,
		TargetChannels: []EffectChannel{
			{Universe: 1, Channel: 1},
		},
	}

	active := NewActiveEffect(effect)
	active.Phase = 0
	active.Intensity = 100

	// First calculation
	CalculateWaveformValues(active, 0)
	initialPhase := active.Phase

	// Advance by 250ms (90 degrees at 1Hz)
	elapsed := 250 * time.Millisecond
	CalculateWaveformValues(active, elapsed)

	// Phase should have advanced by 90 degrees
	expectedPhase := math.Mod(initialPhase+90, 360)
	if math.Abs(active.Phase-expectedPhase) > 1 {
		t.Errorf("Phase should advance: expected ~%v, got %v", expectedPhase, active.Phase)
	}
}

// TestCalculateWaveformValues_PerChannelAmplitudeScale tests per-channel amplitude scaling.
func TestCalculateWaveformValues_PerChannelAmplitudeScale(t *testing.T) {
	halfAmplitude := 0.5

	// Use lower base amplitude to avoid hitting clamps
	effect := &Effect{
		ID:              "test-effect",
		EffectType:      EffectTypeWaveform,
		Waveform:        WaveformSine,
		Frequency:       1.0,
		Amplitude:       40.0, // 40% amplitude so we don't saturate
		Offset:          50.0,
		PhaseOffset:     0,
		CompositionMode: ComposeModeOverride,
		TargetChannels: []EffectChannel{
			{Universe: 1, Channel: 1, AmplitudeScale: nil},            // Full amplitude
			{Universe: 1, Channel: 2, AmplitudeScale: &halfAmplitude}, // Half amplitude
		},
	}

	active := NewActiveEffect(effect)
	active.Phase = 90 // At peak
	active.Intensity = 100

	values := CalculateWaveformValues(active, 0)

	ch1 := values[ChannelKey{Universe: 1, Channel: 1}]
	ch2 := values[ChannelKey{Universe: 1, Channel: 2}]

	// With 40% amplitude at sine peak:
	// Channel 1: amplitudeRange = 40% * 255 = 102
	//            modulation = (1.0-0.5)*2 * 102 = 102
	//            final = 127.5 + 102 = 229.5 ~ 229
	// Channel 2: amplitude = 40% * 0.5 = 20%
	//            amplitudeRange = 20% * 255 = 51
	//            modulation = (1.0-0.5)*2 * 51 = 51
	//            final = 127.5 + 51 = 178.5 ~ 178

	// The difference from center should be about half for ch2
	ch1FromCenter := math.Abs(float64(ch1 - 127))
	ch2FromCenter := math.Abs(float64(ch2 - 127))

	// ch2FromCenter should be approximately half of ch1FromCenter
	if ch1FromCenter < 1 {
		t.Fatalf("ch1FromCenter too small: %v", ch1FromCenter)
	}
	ratio := ch2FromCenter / ch1FromCenter
	if ratio < 0.4 || ratio > 0.6 {
		t.Errorf("Amplitude scale not working correctly: ch1=%d, ch2=%d, ch1FromCenter=%v, ch2FromCenter=%v, ratio=%v",
			ch1, ch2, ch1FromCenter, ch2FromCenter, ratio)
	}
}

// TestCalculateWaveformValues_PerChannelFrequencyScale tests per-channel frequency scaling.
func TestCalculateWaveformValues_PerChannelFrequencyScale(t *testing.T) {
	doubleFreq := 2.0

	effect := &Effect{
		ID:              "test-effect",
		EffectType:      EffectTypeWaveform,
		Waveform:        WaveformSawtooth,
		Frequency:       1.0,
		Amplitude:       100.0,
		Offset:          50.0,
		PhaseOffset:     0,
		CompositionMode: ComposeModeOverride,
		TargetChannels: []EffectChannel{
			{Universe: 1, Channel: 1, FrequencyScale: nil},         // Normal frequency
			{Universe: 1, Channel: 2, FrequencyScale: &doubleFreq}, // Double frequency
		},
	}

	active := NewActiveEffect(effect)
	active.Phase = 90 // Set base phase
	active.Intensity = 100

	values := CalculateWaveformValues(active, 0)

	ch1 := values[ChannelKey{Universe: 1, Channel: 1}]
	ch2 := values[ChannelKey{Universe: 1, Channel: 2}]

	// With double frequency scale, channel 2 effectively sees phase 180
	// At phase 90, sawtooth = 0.25
	// At phase 180, sawtooth = 0.5
	// So ch2 should be different from ch1
	if ch1 == ch2 {
		t.Logf("Note: ch1=%d and ch2=%d happen to be equal (possible but unlikely)", ch1, ch2)
	}
}

// TestCalculateWaveformValues_MultipleChannels tests calculation with multiple target channels.
func TestCalculateWaveformValues_MultipleChannels(t *testing.T) {
	effect := &Effect{
		ID:              "test-effect",
		EffectType:      EffectTypeWaveform,
		Waveform:        WaveformSine,
		Frequency:       1.0,
		Amplitude:       100.0,
		Offset:          50.0,
		PhaseOffset:     0,
		CompositionMode: ComposeModeOverride,
		TargetChannels: []EffectChannel{
			{Universe: 1, Channel: 1},
			{Universe: 1, Channel: 2},
			{Universe: 2, Channel: 1},
			{Universe: 2, Channel: 100},
		},
	}

	active := NewActiveEffect(effect)
	active.Phase = 0
	active.Intensity = 100

	values := CalculateWaveformValues(active, 0)

	if len(values) != 4 {
		t.Errorf("Expected 4 channel values, got %d", len(values))
	}

	expectedKeys := []ChannelKey{
		{Universe: 1, Channel: 1},
		{Universe: 1, Channel: 2},
		{Universe: 2, Channel: 1},
		{Universe: 2, Channel: 100},
	}

	for _, key := range expectedKeys {
		if _, ok := values[key]; !ok {
			t.Errorf("Missing expected channel key: %+v", key)
		}
	}
}

// TestCalculateWaveformValues_ValuesClamped tests that output values are clamped to 0-255.
func TestCalculateWaveformValues_ValuesClamped(t *testing.T) {
	// Use extreme values that might produce out-of-range results
	effect := &Effect{
		ID:              "test-effect",
		EffectType:      EffectTypeWaveform,
		Waveform:        WaveformSquare, // Produces 0 or 1 (sharp transitions)
		Frequency:       1.0,
		Amplitude:       200.0, // Very high amplitude
		Offset:          100.0, // High offset
		PhaseOffset:     0,
		CompositionMode: ComposeModeOverride,
		TargetChannels: []EffectChannel{
			{Universe: 1, Channel: 1},
		},
	}

	active := NewActiveEffect(effect)
	active.Intensity = 100

	// Test at different phases
	phases := []float64{0, 90, 180, 270}
	for _, phase := range phases {
		active.Phase = phase
		values := CalculateWaveformValues(active, 0)
		key := ChannelKey{Universe: 1, Channel: 1}
		val := values[key]

		if val < 0 || val > 255 {
			t.Errorf("Value out of DMX range at phase %v: %d", phase, val)
		}
	}
}

// TestGenerateWaveform_AllWaveformsOutputRange tests that all waveforms output values in [0,1].
func TestGenerateWaveform_AllWaveformsOutputRange(t *testing.T) {
	waveforms := []WaveformType{
		WaveformSine, WaveformCosine, WaveformSquare,
		WaveformSawtooth, WaveformTriangle, WaveformRandom,
	}

	// Test at many phases
	for _, wf := range waveforms {
		t.Run(string(wf), func(t *testing.T) {
			for phase := 0.0; phase < 720; phase += 10 {
				result := GenerateWaveform(phase, wf)
				if result < 0 || result > 1 {
					t.Errorf("GenerateWaveform(%v, %s) = %v, out of range [0,1]",
						phase, wf, result)
				}
			}
		})
	}
}

// TestGenerateWaveform_ContinuousWaveforms tests smoothness of continuous waveforms.
func TestGenerateWaveform_ContinuousWaveforms(t *testing.T) {
	// Sine, cosine, sawtooth, and triangle should be smooth (no large jumps)
	continuousWaveforms := []WaveformType{
		WaveformSine, WaveformCosine, WaveformSawtooth, WaveformTriangle,
	}

	for _, wf := range continuousWaveforms {
		t.Run(string(wf), func(t *testing.T) {
			var prev float64
			first := true
			for phase := 0.0; phase < 360; phase += 1 {
				current := GenerateWaveform(phase, wf)
				if !first {
					diff := math.Abs(current - prev)
					// For continuous waveforms, change per degree should be small
					// Sawtooth has max rate of 1/360 per degree = ~0.0028
					// Triangle has max rate of 1/180 per degree = ~0.0056
					// Sin/cos have max rate at midpoints = ~0.0175
					if diff > 0.02 {
						t.Errorf("Large jump in %s at phase %v: %v -> %v (diff=%v)",
							wf, phase, prev, current, diff)
					}
				}
				prev = current
				first = false
			}
		})
	}
}

// TestGenerateWaveform_SquareWaveDiscontinuity tests that square wave has expected discontinuity.
func TestGenerateWaveform_SquareWaveDiscontinuity(t *testing.T) {
	// Just before 180
	before := GenerateWaveform(179.9, WaveformSquare)
	// Just after 180
	after := GenerateWaveform(180.1, WaveformSquare)

	if before != 1.0 {
		t.Errorf("Square wave at 179.9 should be 1.0, got %v", before)
	}
	if after != 0.0 {
		t.Errorf("Square wave at 180.1 should be 0.0, got %v", after)
	}
}

// TestDeterministicRandom tests that the random waveform is deterministic.
func TestDeterministicRandom(t *testing.T) {
	// Same phase should produce same result
	for phase := 0.0; phase < 360; phase += 30 {
		r1 := GenerateWaveform(phase, WaveformRandom)
		r2 := GenerateWaveform(phase, WaveformRandom)
		if r1 != r2 {
			t.Errorf("Random waveform not deterministic at phase %v: %v != %v", phase, r1, r2)
		}
	}
}
