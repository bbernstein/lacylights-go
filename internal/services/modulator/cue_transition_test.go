package modulator

import (
	"testing"
	"time"

	"github.com/bbernstein/lacylights-go/internal/services/dmx"
)

// createTestEngine creates a modulator engine with a disabled DMX service for testing.
func createTestEngine() (*Engine, *dmx.Service) {
	dmxService := dmx.NewService(dmx.Config{Enabled: false})
	engine := NewEngine(dmxService, 60) // 60Hz update rate
	return engine, dmxService
}

func TestCueTransitionRequest_Defaults(t *testing.T) {
	req := CueTransitionRequest{
		ToLookValues: map[ChannelKey]int{{Universe: 1, Channel: 1}: 255},
		FadeTime:     3.0,
	}

	if req.FromCueID != nil {
		t.Error("FromCueID should be nil by default")
	}
	if req.ToCueID != nil {
		t.Error("ToCueID should be nil by default")
	}
	if req.EasingType != "" {
		t.Error("EasingType should be empty by default (to use default)")
	}
}

func TestExecuteCueTransition_BasicTransition(t *testing.T) {
	engine, dmxService := createTestEngine()

	// Pre-set some channel values
	dmxService.SetChannelValue(1, 1, 100)
	dmxService.SetChannelValue(1, 2, 150)

	req := CueTransitionRequest{
		ToLookValues: map[ChannelKey]int{
			{Universe: 1, Channel: 1}: 200,
			{Universe: 1, Channel: 2}: 50,
		},
		FadeTime:   1.0,
		EasingType: EasingLinear,
	}

	err := engine.ExecuteCueTransition(req)
	if err != nil {
		t.Fatalf("ExecuteCueTransition returned error: %v", err)
	}

	// Should have created a crossfade effect
	effects := engine.GetActiveEffects()
	if len(effects) != 1 {
		t.Fatalf("Expected 1 active effect, got %d", len(effects))
	}

	active := effects[0]
	if active.Effect.EffectType != EffectTypeCrossfade {
		t.Errorf("Expected CROSSFADE effect, got %s", active.Effect.EffectType)
	}

	// Verify from values were captured
	if active.FromValues[ChannelKey{Universe: 1, Channel: 1}] != 100 {
		t.Errorf("FromValues[1,1] = %d, expected 100", active.FromValues[ChannelKey{Universe: 1, Channel: 1}])
	}
	if active.FromValues[ChannelKey{Universe: 1, Channel: 2}] != 150 {
		t.Errorf("FromValues[1,2] = %d, expected 150", active.FromValues[ChannelKey{Universe: 1, Channel: 2}])
	}

	// Verify to values
	if active.ToValues[ChannelKey{Universe: 1, Channel: 1}] != 200 {
		t.Errorf("ToValues[1,1] = %d, expected 200", active.ToValues[ChannelKey{Universe: 1, Channel: 1}])
	}
	if active.ToValues[ChannelKey{Universe: 1, Channel: 2}] != 50 {
		t.Errorf("ToValues[1,2] = %d, expected 50", active.ToValues[ChannelKey{Universe: 1, Channel: 2}])
	}
}

func TestExecuteCueTransition_InstantTransition(t *testing.T) {
	engine, dmxService := createTestEngine()
	dmxService.SetChannelValue(1, 1, 100)

	req := CueTransitionRequest{
		ToLookValues: map[ChannelKey]int{
			{Universe: 1, Channel: 1}: 200,
		},
		FadeTime: 0, // Instant
	}

	err := engine.ExecuteCueTransition(req)
	if err != nil {
		t.Fatalf("ExecuteCueTransition returned error: %v", err)
	}

	// Should NOT have created a crossfade effect
	effects := engine.GetActiveEffects()
	if len(effects) != 0 {
		t.Errorf("Expected 0 active effects for instant transition, got %d", len(effects))
	}

	// Value should be applied directly
	if dmxService.GetChannelValue(1, 1) != 200 {
		t.Errorf("Channel value = %d, expected 200", dmxService.GetChannelValue(1, 1))
	}
}

func TestExecuteCueTransition_FadeToBlack(t *testing.T) {
	engine, dmxService := createTestEngine()
	dmxService.SetChannelValue(1, 1, 200)
	dmxService.SetChannelValue(1, 2, 150)

	req := CueTransitionRequest{
		ToCueID:      nil,                    // No target cue
		ToLookValues: nil,                    // Empty values = fade to black
		FadeTime:     2.0,
	}

	err := engine.ExecuteCueTransition(req)
	if err != nil {
		t.Fatalf("ExecuteCueTransition returned error: %v", err)
	}

	// Should have created a crossfade effect
	effects := engine.GetActiveEffects()
	if len(effects) != 1 {
		t.Fatalf("Expected 1 active effect, got %d", len(effects))
	}

	active := effects[0]

	// To values should be 0 for all channels that were on
	if active.ToValues[ChannelKey{Universe: 1, Channel: 1}] != 0 {
		t.Errorf("ToValues[1,1] = %d, expected 0 for fade to black", active.ToValues[ChannelKey{Universe: 1, Channel: 1}])
	}
	if active.ToValues[ChannelKey{Universe: 1, Channel: 2}] != 0 {
		t.Errorf("ToValues[1,2] = %d, expected 0 for fade to black", active.ToValues[ChannelKey{Universe: 1, Channel: 2}])
	}
}

func TestExecuteCueTransition_DefaultEasing(t *testing.T) {
	engine, _ := createTestEngine()

	req := CueTransitionRequest{
		ToLookValues: map[ChannelKey]int{{Universe: 1, Channel: 1}: 255},
		FadeTime:     1.0,
		EasingType:   "", // Empty - should use default
	}

	err := engine.ExecuteCueTransition(req)
	if err != nil {
		t.Fatalf("ExecuteCueTransition returned error: %v", err)
	}

	effects := engine.GetActiveEffects()
	if len(effects) != 1 {
		t.Fatalf("Expected 1 effect, got %d", len(effects))
	}

	if effects[0].Effect.EasingType != DefaultEasing {
		t.Errorf("EasingType = %s, expected %s", effects[0].Effect.EasingType, DefaultEasing)
	}
}

func TestExecuteCueTransition_ReplacesExistingCrossfade(t *testing.T) {
	engine, _ := createTestEngine()

	// Create first transition
	req1 := CueTransitionRequest{
		ToLookValues: map[ChannelKey]int{{Universe: 1, Channel: 1}: 100},
		FadeTime:     5.0,
	}
	_ = engine.ExecuteCueTransition(req1)

	// Verify first crossfade exists
	if engine.GetActiveEffectCount() != 1 {
		t.Fatalf("Expected 1 effect after first transition")
	}

	// Create second transition (should replace first)
	req2 := CueTransitionRequest{
		ToLookValues: map[ChannelKey]int{{Universe: 1, Channel: 1}: 200},
		FadeTime:     3.0,
	}
	_ = engine.ExecuteCueTransition(req2)

	// Should still have only 1 crossfade (the new one)
	effects := engine.GetActiveEffects()
	if len(effects) != 1 {
		t.Fatalf("Expected 1 effect after second transition, got %d", len(effects))
	}

	// New crossfade should have the new target
	if effects[0].ToValues[ChannelKey{Universe: 1, Channel: 1}] != 200 {
		t.Errorf("Expected new crossfade target 200, got %d", effects[0].ToValues[ChannelKey{Universe: 1, Channel: 1}])
	}

	// Duration should be from new request
	if effects[0].Duration != 3*time.Second {
		t.Errorf("Expected duration 3s, got %v", effects[0].Duration)
	}
}

func TestApplyTransitionBehaviors_FadeOut(t *testing.T) {
	engine, _ := createTestEngine()

	// Add a USER band effect with FADE_OUT behavior
	effect := NewEffect(EffectTypeWaveform, PriorityUserDefault)
	effect.OnCueChange = TransitionFadeOut
	active := engine.AddEffect(effect)

	// Verify initial intensity
	if active.Intensity != 100 {
		t.Fatalf("Initial intensity = %f, expected 100", active.Intensity)
	}

	// Apply transition behaviors
	engine.ApplyTransitionBehaviors(2.0, EasingLinear)

	// Effect should now have an intensity fade
	effects := engine.GetActiveEffects()
	if len(effects) != 1 {
		t.Fatalf("Expected 1 effect, got %d", len(effects))
	}

	if effects[0].IntensityFade == nil {
		t.Error("Expected IntensityFade to be set")
	} else {
		if effects[0].IntensityFade.EndValue != 0 {
			t.Errorf("IntensityFade.EndValue = %f, expected 0", effects[0].IntensityFade.EndValue)
		}
		if effects[0].IntensityFade.Duration != 2*time.Second {
			t.Errorf("IntensityFade.Duration = %v, expected 2s", effects[0].IntensityFade.Duration)
		}
	}
}

func TestApplyTransitionBehaviors_FadeOut_CustomDuration(t *testing.T) {
	engine, _ := createTestEngine()

	// Add effect with custom FadeDuration
	effect := NewEffect(EffectTypeWaveform, PriorityUserDefault)
	effect.OnCueChange = TransitionFadeOut
	customDuration := 5.0
	effect.FadeDuration = &customDuration
	engine.AddEffect(effect)

	// Apply transition with 2s fadeTime
	engine.ApplyTransitionBehaviors(2.0, EasingLinear)

	effects := engine.GetActiveEffects()
	if effects[0].IntensityFade == nil {
		t.Fatal("Expected IntensityFade to be set")
	}

	// Should use custom duration instead of provided fadeTime
	if effects[0].IntensityFade.Duration != 5*time.Second {
		t.Errorf("IntensityFade.Duration = %v, expected 5s (custom)", effects[0].IntensityFade.Duration)
	}
}

func TestApplyTransitionBehaviors_Persist(t *testing.T) {
	engine, _ := createTestEngine()

	// Add a USER band effect with PERSIST behavior
	effect := NewEffect(EffectTypeWaveform, PriorityUserDefault)
	effect.OnCueChange = TransitionPersist
	active := engine.AddEffect(effect)

	// Apply transition behaviors
	engine.ApplyTransitionBehaviors(2.0, EasingLinear)

	// Effect should be unchanged
	effects := engine.GetActiveEffects()
	if len(effects) != 1 {
		t.Fatalf("Expected 1 effect to persist, got %d", len(effects))
	}

	if effects[0].Intensity != 100 {
		t.Errorf("Intensity = %f, expected 100 (unchanged)", effects[0].Intensity)
	}

	if effects[0].IntensityFade != nil {
		t.Error("IntensityFade should be nil for PERSIST behavior")
	}

	if effects[0].Effect.ID != active.Effect.ID {
		t.Error("Effect ID changed unexpectedly")
	}
}

func TestApplyTransitionBehaviors_SnapOff(t *testing.T) {
	engine, _ := createTestEngine()

	// Add a USER band effect with SNAP_OFF behavior
	effect := NewEffect(EffectTypeWaveform, PriorityUserDefault)
	effect.OnCueChange = TransitionSnapOff
	engine.AddEffect(effect)

	// Verify effect exists
	if engine.GetActiveEffectCount() != 1 {
		t.Fatalf("Expected 1 effect initially, got %d", engine.GetActiveEffectCount())
	}

	// Apply transition behaviors
	engine.ApplyTransitionBehaviors(2.0, EasingLinear)

	// Effect should be immediately removed
	if engine.GetActiveEffectCount() != 0 {
		t.Errorf("Expected 0 effects after SNAP_OFF, got %d", engine.GetActiveEffectCount())
	}
}

func TestApplyTransitionBehaviors_CrossfadeParams(t *testing.T) {
	engine, _ := createTestEngine()

	// Add a USER band effect with CROSSFADE_PARAMS behavior
	effect := NewEffect(EffectTypeWaveform, PriorityUserDefault)
	effect.OnCueChange = TransitionCrossfadeParams
	engine.AddEffect(effect)

	// Apply transition behaviors
	engine.ApplyTransitionBehaviors(2.0, EasingLinear)

	// Effect should persist (CROSSFADE_PARAMS without new params acts like PERSIST)
	effects := engine.GetActiveEffects()
	if len(effects) != 1 {
		t.Errorf("Expected 1 effect to persist, got %d", len(effects))
	}
}

func TestApplyTransitionBehaviors_SystemBandUnaffected(t *testing.T) {
	engine, _ := createTestEngine()

	// Add a SYSTEM band effect (should not be affected by cue transitions)
	effect := NewEffect(EffectTypeStatic, PriorityBlackout)
	effect.OnCueChange = TransitionFadeOut // Even with FADE_OUT, SYSTEM should not fade
	engine.AddEffect(effect)

	// Apply transition behaviors
	engine.ApplyTransitionBehaviors(2.0, EasingLinear)

	// SYSTEM effect should be unchanged
	effects := engine.GetActiveEffects()
	if len(effects) != 1 {
		t.Fatalf("Expected 1 effect (SYSTEM), got %d", len(effects))
	}

	if effects[0].IntensityFade != nil {
		t.Error("SYSTEM band effect should not have IntensityFade")
	}
}

func TestApplyTransitionBehaviors_CueBandUnaffected(t *testing.T) {
	engine, _ := createTestEngine()

	// Add a CUE band effect (crossfades are handled separately)
	effect := NewEffect(EffectTypeCrossfade, PriorityCueDefault)
	effect.OnCueChange = TransitionSnapOff // Should be ignored for CUE band
	engine.AddEffect(effect)

	// Apply transition behaviors
	engine.ApplyTransitionBehaviors(2.0, EasingLinear)

	// CUE effect should be unchanged by this method
	// (crossfades are removed by ExecuteCueTransition, not by ApplyTransitionBehaviors)
	effects := engine.GetActiveEffects()
	if len(effects) != 1 {
		t.Errorf("Expected 1 effect (CUE), got %d", len(effects))
	}
}

func TestApplyTransitionBehaviors_MultipleBehaviors(t *testing.T) {
	engine, _ := createTestEngine()

	// Add effects with different behaviors
	fadeOutEffect := NewEffect(EffectTypeWaveform, PriorityUserDefault)
	fadeOutEffect.OnCueChange = TransitionFadeOut
	fadeOutEffect.Name = "FadeOut"
	engine.AddEffect(fadeOutEffect)

	persistEffect := NewEffect(EffectTypeWaveform, PriorityUserDefault)
	persistEffect.OnCueChange = TransitionPersist
	persistEffect.Name = "Persist"
	engine.AddEffect(persistEffect)

	snapEffect := NewEffect(EffectTypeWaveform, PriorityUserDefault)
	snapEffect.OnCueChange = TransitionSnapOff
	snapEffect.Name = "SnapOff"
	engine.AddEffect(snapEffect)

	// Verify all 3 exist
	if engine.GetActiveEffectCount() != 3 {
		t.Fatalf("Expected 3 effects initially, got %d", engine.GetActiveEffectCount())
	}

	// Apply transition behaviors
	engine.ApplyTransitionBehaviors(2.0, EasingLinear)

	// Should have 2 effects remaining (SnapOff removed)
	effects := engine.GetActiveEffects()
	if len(effects) != 2 {
		t.Errorf("Expected 2 effects remaining, got %d", len(effects))
	}

	// Check each remaining effect
	hasFadeOut := false
	hasPersist := false
	for _, active := range effects {
		switch active.Effect.Name {
		case "FadeOut":
			hasFadeOut = true
			if active.IntensityFade == nil {
				t.Error("FadeOut effect should have IntensityFade set")
			}
		case "Persist":
			hasPersist = true
			if active.IntensityFade != nil {
				t.Error("Persist effect should not have IntensityFade")
			}
		case "SnapOff":
			t.Error("SnapOff effect should have been removed")
		}
	}

	if !hasFadeOut {
		t.Error("FadeOut effect missing")
	}
	if !hasPersist {
		t.Error("Persist effect missing")
	}
}

func TestGetEffectsByBand(t *testing.T) {
	engine, _ := createTestEngine()

	// Add effects to different bands
	engine.AddEffect(NewEffect(EffectTypeWaveform, PriorityUserDefault))
	engine.AddEffect(NewEffect(EffectTypeWaveform, PriorityUserDefault))
	engine.AddEffect(NewEffect(EffectTypeCrossfade, PriorityCueDefault))
	engine.AddEffect(NewEffect(EffectTypeStatic, PriorityBlackout))

	// Get USER band effects
	userEffects := engine.GetEffectsByBand(PriorityBandUser)
	if len(userEffects) != 2 {
		t.Errorf("Expected 2 USER effects, got %d", len(userEffects))
	}

	// Get CUE band effects
	cueEffects := engine.GetEffectsByBand(PriorityBandCue)
	if len(cueEffects) != 1 {
		t.Errorf("Expected 1 CUE effect, got %d", len(cueEffects))
	}

	// Get SYSTEM band effects
	systemEffects := engine.GetEffectsByBand(PriorityBandSystem)
	if len(systemEffects) != 1 {
		t.Errorf("Expected 1 SYSTEM effect, got %d", len(systemEffects))
	}

	// Get BASE band effects (should be empty)
	baseEffects := engine.GetEffectsByBand(PriorityBandBase)
	if len(baseEffects) != 0 {
		t.Errorf("Expected 0 BASE effects, got %d", len(baseEffects))
	}
}

func TestGetEffectsByTransitionBehavior(t *testing.T) {
	engine, _ := createTestEngine()

	// Add effects with different behaviors
	e1 := NewEffect(EffectTypeWaveform, PriorityUserDefault)
	e1.OnCueChange = TransitionFadeOut
	engine.AddEffect(e1)

	e2 := NewEffect(EffectTypeWaveform, PriorityUserDefault)
	e2.OnCueChange = TransitionFadeOut
	engine.AddEffect(e2)

	e3 := NewEffect(EffectTypeWaveform, PriorityUserDefault)
	e3.OnCueChange = TransitionPersist
	engine.AddEffect(e3)

	e4 := NewEffect(EffectTypeWaveform, PriorityUserDefault)
	e4.OnCueChange = TransitionSnapOff
	engine.AddEffect(e4)

	// Get FADE_OUT effects
	fadeOutEffects := engine.GetEffectsByTransitionBehavior(TransitionFadeOut)
	if len(fadeOutEffects) != 2 {
		t.Errorf("Expected 2 FADE_OUT effects, got %d", len(fadeOutEffects))
	}

	// Get PERSIST effects
	persistEffects := engine.GetEffectsByTransitionBehavior(TransitionPersist)
	if len(persistEffects) != 1 {
		t.Errorf("Expected 1 PERSIST effect, got %d", len(persistEffects))
	}

	// Get SNAP_OFF effects
	snapEffects := engine.GetEffectsByTransitionBehavior(TransitionSnapOff)
	if len(snapEffects) != 1 {
		t.Errorf("Expected 1 SNAP_OFF effect, got %d", len(snapEffects))
	}
}

func TestHasActiveTransition(t *testing.T) {
	engine, _ := createTestEngine()

	// Initially no transition
	if engine.HasActiveTransition() {
		t.Error("HasActiveTransition should return false initially")
	}

	// Start a transition
	req := CueTransitionRequest{
		ToLookValues: map[ChannelKey]int{{Universe: 1, Channel: 1}: 255},
		FadeTime:     5.0,
	}
	_ = engine.ExecuteCueTransition(req)

	if !engine.HasActiveTransition() {
		t.Error("HasActiveTransition should return true during transition")
	}
}

func TestGetTransitionProgress(t *testing.T) {
	engine, _ := createTestEngine()

	// No transition - should return 1.0
	progress := engine.GetTransitionProgress()
	if progress != 1.0 {
		t.Errorf("Expected progress 1.0 with no transition, got %f", progress)
	}

	// Start a transition
	req := CueTransitionRequest{
		ToLookValues: map[ChannelKey]int{{Universe: 1, Channel: 1}: 255},
		FadeTime:     1.0,
	}
	_ = engine.ExecuteCueTransition(req)

	// Progress should be close to 0 immediately after starting
	progress = engine.GetTransitionProgress()
	if progress > 0.1 {
		t.Errorf("Expected progress near 0 at start, got %f", progress)
	}
}

func TestCleanupCompletedEffects(t *testing.T) {
	engine, _ := createTestEngine()

	// Add an effect and set its intensity to 0
	effect := NewEffect(EffectTypeWaveform, PriorityUserDefault)
	active := engine.AddEffect(effect)
	active.Intensity = 0 // Mark as complete

	if engine.GetActiveEffectCount() != 1 {
		t.Fatalf("Expected 1 effect, got %d", engine.GetActiveEffectCount())
	}

	// Cleanup
	removed := engine.CleanupCompletedEffects()

	if removed != 1 {
		t.Errorf("Expected 1 effect removed, got %d", removed)
	}

	if engine.GetActiveEffectCount() != 0 {
		t.Errorf("Expected 0 effects after cleanup, got %d", engine.GetActiveEffectCount())
	}
}

func TestCrossfadeEffectParams(t *testing.T) {
	engine, _ := createTestEngine()

	// Add an effect
	effect := NewEffect(EffectTypeWaveform, PriorityUserDefault)
	engine.AddEffect(effect)

	// Crossfade its intensity
	newIntensity := 50.0
	update := EffectParamUpdate{
		Intensity: &newIntensity,
	}

	success := engine.CrossfadeEffectParams(effect.ID, update, 2*time.Second, EasingLinear)
	if !success {
		t.Error("CrossfadeEffectParams should return true for existing effect")
	}

	// Verify intensity fade was set
	effects := engine.GetActiveEffects()
	if effects[0].IntensityFade == nil {
		t.Error("IntensityFade should be set")
	} else {
		if effects[0].IntensityFade.EndValue != 50 {
			t.Errorf("IntensityFade.EndValue = %f, expected 50", effects[0].IntensityFade.EndValue)
		}
	}
}

func TestCrossfadeEffectParams_NonExistent(t *testing.T) {
	engine, _ := createTestEngine()

	update := EffectParamUpdate{
		Intensity: new(float64),
	}

	success := engine.CrossfadeEffectParams("non-existent-id", update, time.Second, EasingLinear)
	if success {
		t.Error("CrossfadeEffectParams should return false for non-existent effect")
	}
}

func TestExecuteCueTransition_NoFromCue(t *testing.T) {
	engine, _ := createTestEngine()
	// Start with no channel values (fresh start)

	toCueID := "cue-1"
	req := CueTransitionRequest{
		FromCueID: nil, // Starting fresh
		ToCueID:   &toCueID,
		ToLookValues: map[ChannelKey]int{
			{Universe: 1, Channel: 1}: 255,
		},
		FadeTime: 1.0,
	}

	err := engine.ExecuteCueTransition(req)
	if err != nil {
		t.Fatalf("ExecuteCueTransition returned error: %v", err)
	}

	// Should create crossfade from 0 (no existing channels)
	effects := engine.GetActiveEffects()
	if len(effects) != 1 {
		t.Fatalf("Expected 1 effect, got %d", len(effects))
	}

	// FromValues should be empty since no channels had values
	if len(effects[0].FromValues) != 0 {
		t.Errorf("Expected empty FromValues, got %d values", len(effects[0].FromValues))
	}
}

func TestExecuteCueTransition_NoToCue(t *testing.T) {
	engine, dmxService := createTestEngine()
	dmxService.SetChannelValue(1, 1, 200)
	dmxService.SetChannelValue(1, 2, 150)

	fromCueID := "cue-1"
	req := CueTransitionRequest{
		FromCueID:    &fromCueID,
		ToCueID:      nil,                    // Going to black
		ToLookValues: map[ChannelKey]int{}, // Empty = fade to black
		FadeTime:     2.0,
	}

	err := engine.ExecuteCueTransition(req)
	if err != nil {
		t.Fatalf("ExecuteCueTransition returned error: %v", err)
	}

	// All existing channels should fade to 0
	effects := engine.GetActiveEffects()
	if len(effects) != 1 {
		t.Fatalf("Expected 1 effect, got %d", len(effects))
	}

	for key := range effects[0].FromValues {
		if effects[0].ToValues[key] != 0 {
			t.Errorf("ToValues[%+v] = %d, expected 0 for fade to black", key, effects[0].ToValues[key])
		}
	}
}

func TestExecuteCueTransition_EmptyLooks(t *testing.T) {
	engine, _ := createTestEngine()

	// Both from and to have no channel values
	req := CueTransitionRequest{
		ToLookValues: map[ChannelKey]int{},
		FadeTime:     1.0,
	}

	err := engine.ExecuteCueTransition(req)
	if err != nil {
		t.Fatalf("ExecuteCueTransition returned error: %v", err)
	}

	// Should still create a crossfade (even if it does nothing)
	effects := engine.GetActiveEffects()
	if len(effects) != 1 {
		t.Fatalf("Expected 1 effect, got %d", len(effects))
	}
}

func TestApplyTransitionBehaviors_ZeroFadeTime(t *testing.T) {
	engine, _ := createTestEngine()

	// Add effect with FADE_OUT behavior
	effect := NewEffect(EffectTypeWaveform, PriorityUserDefault)
	effect.OnCueChange = TransitionFadeOut
	engine.AddEffect(effect)

	// Apply with 0 fade time (instant)
	engine.ApplyTransitionBehaviors(0, EasingLinear)

	effects := engine.GetActiveEffects()
	if len(effects) != 1 {
		t.Fatalf("Expected 1 effect, got %d", len(effects))
	}

	// Intensity should be set to 0 immediately (no fade)
	if effects[0].Intensity != 0 {
		t.Errorf("Intensity = %f, expected 0 for instant fade", effects[0].Intensity)
	}
}

func TestEffectParamUpdate(t *testing.T) {
	// Test that EffectParamUpdate struct works correctly
	intensity := 50.0
	frequency := 2.0

	update := EffectParamUpdate{
		Intensity: &intensity,
		Frequency: &frequency,
	}

	if update.Intensity == nil || *update.Intensity != 50.0 {
		t.Error("Intensity not set correctly")
	}
	if update.Frequency == nil || *update.Frequency != 2.0 {
		t.Error("Frequency not set correctly")
	}
	if update.Amplitude != nil {
		t.Error("Amplitude should be nil")
	}
	if update.Offset != nil {
		t.Error("Offset should be nil")
	}
}

func TestExecuteCueTransition_WithUserEffects(t *testing.T) {
	engine, dmxService := createTestEngine()
	dmxService.SetChannelValue(1, 1, 100)

	// Add USER effect with FADE_OUT behavior
	userEffect := NewEffect(EffectTypeWaveform, PriorityUserDefault)
	userEffect.OnCueChange = TransitionFadeOut
	userEffect.Name = "Test Waveform"
	engine.AddEffect(userEffect)

	// Verify USER effect exists
	if len(engine.GetEffectsByBand(PriorityBandUser)) != 1 {
		t.Fatal("Expected 1 USER effect before transition")
	}

	// Execute cue transition
	req := CueTransitionRequest{
		ToLookValues: map[ChannelKey]int{{Universe: 1, Channel: 1}: 200},
		FadeTime:     2.0,
	}
	_ = engine.ExecuteCueTransition(req)

	// Should now have 2 effects: 1 USER (fading out) + 1 CUE (crossfade)
	effects := engine.GetActiveEffects()
	if len(effects) != 2 {
		t.Errorf("Expected 2 effects after transition, got %d", len(effects))
	}

	// USER effect should have intensity fade set
	userEffects := engine.GetEffectsByBand(PriorityBandUser)
	if len(userEffects) != 1 {
		t.Fatal("Expected 1 USER effect still present")
	}
	if userEffects[0].IntensityFade == nil {
		t.Error("USER effect should have IntensityFade set during transition")
	}

	// CUE effect should be the crossfade
	cueEffects := engine.GetEffectsByBand(PriorityBandCue)
	if len(cueEffects) != 1 {
		t.Fatal("Expected 1 CUE effect (crossfade)")
	}
	if cueEffects[0].Effect.EffectType != EffectTypeCrossfade {
		t.Error("CUE effect should be CROSSFADE type")
	}
}

func TestExecuteCueTransition_WithPersistingUserEffects(t *testing.T) {
	engine, dmxService := createTestEngine()
	dmxService.SetChannelValue(1, 1, 100)

	// Add USER effect with PERSIST behavior (e.g., ambient effect)
	persistEffect := NewEffect(EffectTypeWaveform, PriorityUserDefault)
	persistEffect.OnCueChange = TransitionPersist
	persistEffect.Name = "Ambient"
	engine.AddEffect(persistEffect)

	// Execute cue transition
	req := CueTransitionRequest{
		ToLookValues: map[ChannelKey]int{{Universe: 1, Channel: 1}: 200},
		FadeTime:     2.0,
	}
	_ = engine.ExecuteCueTransition(req)

	// USER effect should persist without intensity fade
	userEffects := engine.GetEffectsByBand(PriorityBandUser)
	if len(userEffects) != 1 {
		t.Fatal("Expected 1 USER effect still present")
	}
	if userEffects[0].IntensityFade != nil {
		t.Error("PERSIST effect should not have IntensityFade")
	}
	if userEffects[0].Intensity != 100 {
		t.Errorf("PERSIST effect intensity should be unchanged, got %f", userEffects[0].Intensity)
	}
}
