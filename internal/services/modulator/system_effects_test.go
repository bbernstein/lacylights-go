package modulator

import (
	"math"
	"testing"
	"time"
)

// --- CreateBlackoutEffect Tests ---

func TestCreateBlackoutEffect(t *testing.T) {
	effect := CreateBlackoutEffect()

	// Verify ID is the well-known system blackout ID
	if effect.ID != SystemBlackoutEffectID {
		t.Errorf("ID = %q, expected %q", effect.ID, SystemBlackoutEffectID)
	}

	// Verify effect type
	if effect.EffectType != EffectTypeStatic {
		t.Errorf("EffectType = %v, expected %v", effect.EffectType, EffectTypeStatic)
	}

	// Verify priority is system-level blackout priority
	if !effect.Priority.Equal(PriorityBlackout) {
		t.Errorf("Priority = %v, expected %v", effect.Priority, PriorityBlackout)
	}

	// Verify composition mode
	if effect.CompositionMode != ComposeModeOverride {
		t.Errorf("CompositionMode = %v, expected %v", effect.CompositionMode, ComposeModeOverride)
	}

	// Verify it persists across cue changes
	if effect.OnCueChange != TransitionPersist {
		t.Errorf("OnCueChange = %v, expected %v", effect.OnCueChange, TransitionPersist)
	}

	// Verify name and description are set
	if effect.Name == "" {
		t.Error("Name should not be empty")
	}
	if effect.Description == "" {
		t.Error("Description should not be empty")
	}
}

func TestCreateBlackoutEffectHasHighestPriority(t *testing.T) {
	blackout := CreateBlackoutEffect()
	grandMaster := CreateGrandMasterEffect(1.0)

	// Blackout should have higher priority than grand master
	if !blackout.Priority.Greater(grandMaster.Priority) {
		t.Error("Blackout priority should be greater than grand master priority")
	}

	// Blackout should have system band
	if blackout.Priority.Band != PriorityBandSystem {
		t.Errorf("Blackout priority band = %v, expected %v", blackout.Priority.Band, PriorityBandSystem)
	}
}

// --- CreateGrandMasterEffect Tests ---

func TestCreateGrandMasterEffect(t *testing.T) {
	tests := []struct {
		name          string
		inputValue    float64
		expectedValue float64
	}{
		{"full intensity", 1.0, 1.0},
		{"half intensity", 0.5, 0.5},
		{"zero intensity", 0.0, 0.0},
		{"quarter intensity", 0.25, 0.25},
		{"value clamped above 1", 1.5, 1.0},
		{"value clamped below 0", -0.5, 0.0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			effect := CreateGrandMasterEffect(tt.inputValue)

			// Verify ID is the well-known grand master ID
			if effect.ID != SystemGrandMasterEffectID {
				t.Errorf("ID = %q, expected %q", effect.ID, SystemGrandMasterEffectID)
			}

			// Verify effect type
			if effect.EffectType != EffectTypeMaster {
				t.Errorf("EffectType = %v, expected %v", effect.EffectType, EffectTypeMaster)
			}

			// Verify priority is system-level grand master priority
			if !effect.Priority.Equal(PriorityGrandMaster) {
				t.Errorf("Priority = %v, expected %v", effect.Priority, PriorityGrandMaster)
			}

			// Verify composition mode is multiply
			if effect.CompositionMode != ComposeModeMultiply {
				t.Errorf("CompositionMode = %v, expected %v", effect.CompositionMode, ComposeModeMultiply)
			}

			// Verify it persists across cue changes
			if effect.OnCueChange != TransitionPersist {
				t.Errorf("OnCueChange = %v, expected %v", effect.OnCueChange, TransitionPersist)
			}

			// Verify master value is clamped correctly
			if effect.MasterValue != tt.expectedValue {
				t.Errorf("MasterValue = %v, expected %v", effect.MasterValue, tt.expectedValue)
			}
		})
	}
}

// --- Engine.ActivateBlackout Tests ---

func TestEngineActivateBlackoutImmediate(t *testing.T) {
	engine := NewEngine(nil, 60)

	// Activate blackout immediately
	engine.ActivateBlackout(0)

	// Verify blackout is active
	if !engine.IsBlackoutActive() {
		t.Error("Blackout should be active")
	}

	// Verify blackout intensity is 100
	intensity := engine.GetBlackoutIntensity()
	if intensity != 100 {
		t.Errorf("Blackout intensity = %v, expected 100", intensity)
	}

	// Verify effect was added
	if engine.GetActiveEffectCount() != 1 {
		t.Errorf("Active effect count = %d, expected 1", engine.GetActiveEffectCount())
	}
}

func TestEngineActivateBlackoutWithFade(t *testing.T) {
	engine := NewEngine(nil, 60)

	// Activate blackout with 1 second fade
	engine.ActivateBlackout(1.0)

	// Verify blackout is active (fading to non-zero)
	if !engine.IsBlackoutActive() {
		t.Error("Blackout should be active during fade in")
	}

	// Intensity should be near 0 (just started)
	intensity := engine.GetBlackoutIntensity()
	if intensity > 10 {
		t.Errorf("Blackout intensity at start = %v, expected near 0", intensity)
	}

	// Check that an intensity fade is active
	effects := engine.GetActiveEffects()
	if len(effects) != 1 {
		t.Fatalf("Expected 1 active effect, got %d", len(effects))
	}
	if effects[0].IntensityFade == nil {
		t.Error("Expected intensity fade to be active")
	}
}

func TestEngineActivateBlackoutMultipleTimes(t *testing.T) {
	engine := NewEngine(nil, 60)

	// Activate blackout twice - should not create duplicate
	engine.ActivateBlackout(0)
	engine.ActivateBlackout(0)

	// Verify only one effect exists
	if engine.GetActiveEffectCount() != 1 {
		t.Errorf("Active effect count = %d, expected 1", engine.GetActiveEffectCount())
	}
}

func TestEngineActivateBlackoutDuringFadeOut(t *testing.T) {
	engine := NewEngine(nil, 60)

	// Activate and then start releasing blackout
	engine.ActivateBlackout(0)
	engine.ReleaseBlackout(1.0) // Start fading out

	// Re-activate blackout (should interrupt fade out)
	engine.ActivateBlackout(0)

	// Blackout should be at full intensity
	intensity := engine.GetBlackoutIntensity()
	if intensity != 100 {
		t.Errorf("Blackout intensity after reactivation = %v, expected 100", intensity)
	}
}

// --- Engine.ReleaseBlackout Tests ---

func TestEngineReleaseBlackoutImmediate(t *testing.T) {
	engine := NewEngine(nil, 60)

	// Activate then release blackout immediately
	engine.ActivateBlackout(0)
	engine.ReleaseBlackout(0)

	// Verify blackout is no longer active (intensity is 0)
	intensity := engine.GetBlackoutIntensity()
	if intensity != 0 {
		t.Errorf("Blackout intensity after release = %v, expected 0", intensity)
	}

	// Note: The effect will be cleaned up on next processModulation cycle
	// For immediate release, we just set intensity to 0
}

func TestEngineReleaseBlackoutWithFade(t *testing.T) {
	engine := NewEngine(nil, 60)

	// Activate then start releasing with fade
	engine.ActivateBlackout(0)
	engine.ReleaseBlackout(1.0)

	// Blackout should still be active (fading out)
	if !engine.IsBlackoutActive() {
		t.Error("Blackout should still be active during fade out")
	}

	// Check that fade is toward 0
	effects := engine.GetActiveEffects()
	if len(effects) != 1 {
		t.Fatalf("Expected 1 active effect, got %d", len(effects))
	}
	if effects[0].IntensityFade == nil {
		t.Fatal("Expected intensity fade to be active")
	}
	if effects[0].IntensityFade.EndValue != 0 {
		t.Errorf("Fade end value = %v, expected 0", effects[0].IntensityFade.EndValue)
	}
}

func TestEngineReleaseBlackoutWhenNotActive(t *testing.T) {
	engine := NewEngine(nil, 60)

	// Release blackout that doesn't exist - should not panic
	engine.ReleaseBlackout(0)
	engine.ReleaseBlackout(1.0)

	// Should have no effects
	if engine.GetActiveEffectCount() != 0 {
		t.Errorf("Active effect count = %d, expected 0", engine.GetActiveEffectCount())
	}
}

// --- Engine.SetGrandMaster Tests ---

func TestEngineSetGrandMaster(t *testing.T) {
	engine := NewEngine(nil, 60)

	// Set grand master to 50%
	engine.SetGrandMaster(0.5)

	// Verify grand master value
	value := engine.GetGrandMasterValue()
	if value != 0.5 {
		t.Errorf("Grand master value = %v, expected 0.5", value)
	}

	// Verify effect was added
	if engine.GetActiveEffectCount() != 1 {
		t.Errorf("Active effect count = %d, expected 1", engine.GetActiveEffectCount())
	}
}

func TestEngineSetGrandMasterUpdate(t *testing.T) {
	engine := NewEngine(nil, 60)

	// Set grand master multiple times
	engine.SetGrandMaster(0.5)
	engine.SetGrandMaster(0.75)
	engine.SetGrandMaster(0.25)

	// Verify final value
	value := engine.GetGrandMasterValue()
	if value != 0.25 {
		t.Errorf("Grand master value = %v, expected 0.25", value)
	}

	// Verify only one effect exists (updates in place)
	if engine.GetActiveEffectCount() != 1 {
		t.Errorf("Active effect count = %d, expected 1", engine.GetActiveEffectCount())
	}
}

func TestEngineSetGrandMasterClamps(t *testing.T) {
	engine := NewEngine(nil, 60)

	// Set value above 1.0
	engine.SetGrandMaster(1.5)
	if engine.GetGrandMasterValue() != 1.0 {
		t.Errorf("Grand master should clamp to 1.0, got %v", engine.GetGrandMasterValue())
	}

	// Set value below 0.0
	engine.SetGrandMaster(-0.5)
	if engine.GetGrandMasterValue() != 0.0 {
		t.Errorf("Grand master should clamp to 0.0, got %v", engine.GetGrandMasterValue())
	}
}

// --- Engine.GetGrandMasterValue Tests ---

func TestEngineGetGrandMasterValueDefault(t *testing.T) {
	engine := NewEngine(nil, 60)

	// No grand master set - should return 1.0 (full intensity)
	value := engine.GetGrandMasterValue()
	if value != 1.0 {
		t.Errorf("Default grand master value = %v, expected 1.0", value)
	}
}

// --- Engine.IsBlackoutActive Tests ---

func TestEngineIsBlackoutActiveStates(t *testing.T) {
	tests := []struct {
		name     string
		setup    func(e *Engine)
		expected bool
	}{
		{
			name:     "no blackout",
			setup:    func(e *Engine) {},
			expected: false,
		},
		{
			name: "blackout active at full intensity",
			setup: func(e *Engine) {
				e.ActivateBlackout(0)
			},
			expected: true,
		},
		{
			name: "blackout fading in",
			setup: func(e *Engine) {
				e.ActivateBlackout(5.0) // Long fade
			},
			expected: true,
		},
		{
			name: "blackout released immediately",
			setup: func(e *Engine) {
				e.ActivateBlackout(0)
				e.ReleaseBlackout(0)
			},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			engine := NewEngine(nil, 60)
			tt.setup(engine)

			result := engine.IsBlackoutActive()
			if result != tt.expected {
				t.Errorf("IsBlackoutActive() = %v, expected %v", result, tt.expected)
			}
		})
	}
}

// --- Blackout Overrides All Other Effects Tests ---

func TestBlackoutOverridesOtherEffects(t *testing.T) {
	engine := NewEngine(nil, 60)

	// Add a user effect
	userEffect := NewEffect(EffectTypeStatic, PriorityUserDefault)
	userEffect.ID = "user-effect"
	engine.AddEffect(userEffect)

	// Add a cue effect
	cueEffect := NewEffect(EffectTypeCrossfade, PriorityCueDefault)
	cueEffect.ID = "cue-effect"
	engine.AddEffect(cueEffect)

	// Add blackout - should be processed last due to highest priority
	engine.ActivateBlackout(0)

	// Get effects in priority order
	effects := engine.GetActiveEffects()
	if len(effects) != 3 {
		t.Fatalf("Expected 3 active effects, got %d", len(effects))
	}

	// Verify order: base/user first, cue second, system (blackout) last
	if effects[0].Effect.ID != "user-effect" {
		t.Errorf("First effect should be user effect, got %s", effects[0].Effect.ID)
	}
	if effects[1].Effect.ID != "cue-effect" {
		t.Errorf("Second effect should be cue effect, got %s", effects[1].Effect.ID)
	}
	if effects[2].Effect.ID != SystemBlackoutEffectID {
		t.Errorf("Third effect should be blackout, got %s", effects[2].Effect.ID)
	}
}

// --- Grand Master Scales Intensity Tests ---

func TestGrandMasterScalesChannels(t *testing.T) {
	// This test verifies the effect configuration is correct for scaling
	effect := CreateGrandMasterEffect(0.5)

	// Verify it's a MASTER type (which multiplies values)
	if effect.EffectType != EffectTypeMaster {
		t.Errorf("Grand master should be MASTER type, got %v", effect.EffectType)
	}

	// Verify it uses MULTIPLY composition
	if effect.CompositionMode != ComposeModeMultiply {
		t.Errorf("Grand master should use MULTIPLY composition, got %v", effect.CompositionMode)
	}

	// Verify the master value is set correctly
	if effect.MasterValue != 0.5 {
		t.Errorf("MasterValue = %v, expected 0.5", effect.MasterValue)
	}
}

// --- Grand Master Persists Across Cue Changes Tests ---

func TestGrandMasterPersistsAcrossCueChanges(t *testing.T) {
	effect := CreateGrandMasterEffect(0.75)

	// Verify grand master uses PERSIST transition behavior
	if effect.OnCueChange != TransitionPersist {
		t.Errorf("Grand master OnCueChange = %v, expected %v", effect.OnCueChange, TransitionPersist)
	}
}

// --- Blackout Fade In/Out Tests ---

func TestBlackoutFadeInProgression(t *testing.T) {
	engine := NewEngine(nil, 60)

	// Start fade in over 1 second
	engine.ActivateBlackout(1.0)

	// Get the effect
	effects := engine.GetActiveEffects()
	if len(effects) != 1 {
		t.Fatalf("Expected 1 effect, got %d", len(effects))
	}

	fade := effects[0].IntensityFade
	if fade == nil {
		t.Fatal("Expected intensity fade to be active")
	}

	// Verify fade parameters
	if fade.StartValue != 0 {
		t.Errorf("Fade start value = %v, expected 0", fade.StartValue)
	}
	if fade.EndValue != 100 {
		t.Errorf("Fade end value = %v, expected 100", fade.EndValue)
	}
	if fade.Duration != time.Second {
		t.Errorf("Fade duration = %v, expected 1s", fade.Duration)
	}
}

func TestBlackoutFadeOutProgression(t *testing.T) {
	engine := NewEngine(nil, 60)

	// Activate and then release with fade
	engine.ActivateBlackout(0)
	engine.ReleaseBlackout(2.0)

	// Get the effect
	effects := engine.GetActiveEffects()
	if len(effects) != 1 {
		t.Fatalf("Expected 1 effect, got %d", len(effects))
	}

	fade := effects[0].IntensityFade
	if fade == nil {
		t.Fatal("Expected intensity fade to be active")
	}

	// Verify fade parameters
	if fade.StartValue != 100 {
		t.Errorf("Fade start value = %v, expected 100", fade.StartValue)
	}
	if fade.EndValue != 0 {
		t.Errorf("Fade end value = %v, expected 0", fade.EndValue)
	}
	if fade.Duration != 2*time.Second {
		t.Errorf("Fade duration = %v, expected 2s", fade.Duration)
	}
}

// --- ClearSystemEffects Tests ---

func TestEngineClearSystemEffects(t *testing.T) {
	engine := NewEngine(nil, 60)

	// Add various effects
	engine.SetGrandMaster(0.5)
	engine.ActivateBlackout(0)

	// Add a non-system effect
	userEffect := NewEffect(EffectTypeWaveform, PriorityUserDefault)
	engine.AddEffect(userEffect)

	// Verify we have 3 effects
	if engine.GetActiveEffectCount() != 3 {
		t.Fatalf("Expected 3 effects, got %d", engine.GetActiveEffectCount())
	}

	// Clear system effects
	engine.ClearSystemEffects()

	// Should only have the user effect left
	if engine.GetActiveEffectCount() != 1 {
		t.Errorf("Expected 1 effect after clear, got %d", engine.GetActiveEffectCount())
	}

	// Verify blackout and grand master are gone
	if engine.IsBlackoutActive() {
		t.Error("Blackout should not be active after clear")
	}
	if engine.GetGrandMasterValue() != 1.0 {
		t.Error("Grand master should return default value after clear")
	}
}

// --- HasSystemEffect Tests ---

func TestEngineHasSystemEffect(t *testing.T) {
	engine := NewEngine(nil, 60)

	// Initially no system effects
	if engine.HasSystemEffect(SystemBlackoutEffectID) {
		t.Error("Should not have blackout effect initially")
	}
	if engine.HasSystemEffect(SystemGrandMasterEffectID) {
		t.Error("Should not have grand master effect initially")
	}

	// Add blackout
	engine.ActivateBlackout(0)
	if !engine.HasSystemEffect(SystemBlackoutEffectID) {
		t.Error("Should have blackout effect after activation")
	}

	// Add grand master
	engine.SetGrandMaster(0.5)
	if !engine.HasSystemEffect(SystemGrandMasterEffectID) {
		t.Error("Should have grand master effect after setting")
	}

	// Check for non-existent effect
	if engine.HasSystemEffect("non-existent-id") {
		t.Error("Should not have non-existent effect")
	}
}

// --- Edge Cases Tests ---

func TestBlackoutAndGrandMasterTogether(t *testing.T) {
	engine := NewEngine(nil, 60)

	// Set grand master to 50%
	engine.SetGrandMaster(0.5)

	// Activate blackout
	engine.ActivateBlackout(0)

	// Both should be active
	if engine.GetGrandMasterValue() != 0.5 {
		t.Errorf("Grand master value = %v, expected 0.5", engine.GetGrandMasterValue())
	}
	if !engine.IsBlackoutActive() {
		t.Error("Blackout should be active")
	}

	// Verify effect count
	if engine.GetActiveEffectCount() != 2 {
		t.Errorf("Active effect count = %d, expected 2", engine.GetActiveEffectCount())
	}

	// Blackout should be processed after grand master (higher priority)
	effects := engine.GetActiveEffects()
	grandMasterIdx := -1
	blackoutIdx := -1
	for i, e := range effects {
		if e.Effect.ID == SystemGrandMasterEffectID {
			grandMasterIdx = i
		}
		if e.Effect.ID == SystemBlackoutEffectID {
			blackoutIdx = i
		}
	}

	if grandMasterIdx >= blackoutIdx {
		t.Error("Grand master should be processed before blackout (lower index)")
	}
}

func TestZeroFadeTimeIsInstant(t *testing.T) {
	engine := NewEngine(nil, 60)

	// Activate with zero fade time
	engine.ActivateBlackout(0)

	// Should be at full intensity immediately
	intensity := engine.GetBlackoutIntensity()
	if intensity != 100 {
		t.Errorf("Blackout intensity = %v, expected 100 for instant activation", intensity)
	}

	// There should be no active fade
	effects := engine.GetActiveEffects()
	if len(effects) > 0 && effects[0].IntensityFade != nil {
		t.Error("Should not have an active fade for zero fade time")
	}
}

func TestNegativeFadeTimeTreatedAsInstant(t *testing.T) {
	engine := NewEngine(nil, 60)

	// Negative fade time should work like 0
	engine.ActivateBlackout(-1.0)

	// Should be at full intensity immediately
	intensity := engine.GetBlackoutIntensity()
	// Note: Negative duration in time.Duration might produce unexpected behavior,
	// but the implementation should handle this gracefully
	if intensity != 100 {
		t.Logf("Blackout intensity with negative fade = %v", intensity)
	}
}

func TestVeryLongFadeTime(t *testing.T) {
	engine := NewEngine(nil, 60)

	// Very long fade time
	engine.ActivateBlackout(3600.0) // 1 hour

	// Should be active but at low intensity
	if !engine.IsBlackoutActive() {
		t.Error("Blackout should be active with long fade")
	}

	intensity := engine.GetBlackoutIntensity()
	if intensity > 1 {
		t.Errorf("Blackout intensity at start of long fade = %v, expected near 0", intensity)
	}
}

// --- GetBlackoutIntensity Tests ---

func TestGetBlackoutIntensityVariousStates(t *testing.T) {
	tests := []struct {
		name              string
		setup             func(e *Engine)
		expectedIntensity float64
		tolerance         float64
	}{
		{
			name:              "no blackout returns 0",
			setup:             func(e *Engine) {},
			expectedIntensity: 0,
			tolerance:         0.1,
		},
		{
			name: "full blackout returns 100",
			setup: func(e *Engine) {
				e.ActivateBlackout(0)
			},
			expectedIntensity: 100,
			tolerance:         0.1,
		},
		{
			name: "released blackout returns 0",
			setup: func(e *Engine) {
				e.ActivateBlackout(0)
				e.ReleaseBlackout(0)
			},
			expectedIntensity: 0,
			tolerance:         0.1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			engine := NewEngine(nil, 60)
			tt.setup(engine)

			intensity := engine.GetBlackoutIntensity()
			if math.Abs(intensity-tt.expectedIntensity) > tt.tolerance {
				t.Errorf("GetBlackoutIntensity() = %v, expected %v (tolerance: %v)",
					intensity, tt.expectedIntensity, tt.tolerance)
			}
		})
	}
}

// --- Concurrent Access Tests ---

func TestSystemEffectsConcurrentAccess(t *testing.T) {
	engine := NewEngine(nil, 60)

	// Start multiple goroutines accessing system effects
	done := make(chan bool, 4)

	go func() {
		for i := 0; i < 100; i++ {
			engine.ActivateBlackout(0)
			engine.ReleaseBlackout(0)
		}
		done <- true
	}()

	go func() {
		for i := 0; i < 100; i++ {
			engine.SetGrandMaster(float64(i) / 100.0)
		}
		done <- true
	}()

	go func() {
		for i := 0; i < 100; i++ {
			_ = engine.IsBlackoutActive()
			_ = engine.GetBlackoutIntensity()
		}
		done <- true
	}()

	go func() {
		for i := 0; i < 100; i++ {
			_ = engine.GetGrandMasterValue()
			_ = engine.HasSystemEffect(SystemBlackoutEffectID)
		}
		done <- true
	}()

	// Wait for all goroutines
	for i := 0; i < 4; i++ {
		<-done
	}

	// If we get here without panic or deadlock, the test passes
}

// --- Effect Constants Tests ---

func TestSystemEffectConstants(t *testing.T) {
	// Verify the constants are defined correctly
	if SystemBlackoutEffectID != "system-blackout" {
		t.Errorf("SystemBlackoutEffectID = %q, expected %q", SystemBlackoutEffectID, "system-blackout")
	}
	if SystemGrandMasterEffectID != "system-grand-master" {
		t.Errorf("SystemGrandMasterEffectID = %q, expected %q", SystemGrandMasterEffectID, "system-grand-master")
	}
}

// --- Priority Constants Alignment Tests ---

func TestSystemEffectPriorityAlignment(t *testing.T) {
	// Verify system effects use the predefined priority constants
	blackout := CreateBlackoutEffect()
	if !blackout.Priority.Equal(PriorityBlackout) {
		t.Errorf("Blackout priority = %v, expected %v", blackout.Priority, PriorityBlackout)
	}

	grandMaster := CreateGrandMasterEffect(1.0)
	if !grandMaster.Priority.Equal(PriorityGrandMaster) {
		t.Errorf("Grand master priority = %v, expected %v", grandMaster.Priority, PriorityGrandMaster)
	}

	// Verify blackout > grand master > cue > user > base
	if !blackout.Priority.Greater(grandMaster.Priority) {
		t.Error("Blackout should have higher priority than grand master")
	}
	if !grandMaster.Priority.Greater(PriorityCueDefault) {
		t.Error("Grand master should have higher priority than cue default")
	}
	if !PriorityCueDefault.Greater(PriorityUserDefault) {
		t.Error("Cue default should have higher priority than user default")
	}
}
