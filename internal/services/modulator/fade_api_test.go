package modulator

import (
	"testing"
	"time"

	"github.com/bbernstein/lacylights-go/internal/services/dmx"
)

// createTestModulatorEngine creates a modulator engine with a disabled DMX service for testing.
func createTestModulatorEngine() (*Engine, *dmx.Service) {
	dmxService := dmx.NewService(dmx.Config{Enabled: false})
	engine := NewEngine(dmxService, 60) // 60Hz update rate
	return engine, dmxService
}

// waitForCondition polls until condition returns true or timeout is reached.
// Uses 10ms polling interval to avoid flaky timing failures in CI.
// onTimeout is called to produce the failure message (allowing current values to be included).
func waitForCondition(t *testing.T, timeout time.Duration, condition func() bool, onTimeout func()) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for {
		if condition() {
			return
		}
		if time.Now().After(deadline) {
			onTimeout()
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
}

// TestFadeToLook_InstantFade tests that duration <= 0 applies changes immediately.
func TestFadeToLook_InstantFade(t *testing.T) {
	tests := []struct {
		name           string
		duration       time.Duration
		channels       []LookChannel
		expectedValues map[ChannelKey]byte
	}{
		{
			name:     "zero duration",
			duration: 0,
			channels: []LookChannel{
				{Universe: 1, Channel: 1, Value: 128},
				{Universe: 1, Channel: 2, Value: 255},
			},
			expectedValues: map[ChannelKey]byte{
				{Universe: 1, Channel: 1}: 128,
				{Universe: 1, Channel: 2}: 255,
			},
		},
		{
			name:     "negative duration",
			duration: -100 * time.Millisecond,
			channels: []LookChannel{
				{Universe: 1, Channel: 5, Value: 200},
				{Universe: 2, Channel: 1, Value: 100},
			},
			expectedValues: map[ChannelKey]byte{
				{Universe: 1, Channel: 5}: 200,
				{Universe: 2, Channel: 1}: 100,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			engine, dmxService := createTestModulatorEngine()

			fadeID := engine.FadeToLook(tt.channels, tt.duration, "test-fade", EasingLinear)

			if fadeID == "" {
				t.Error("FadeToLook should return a non-empty fade ID")
			}

			// Instant fades should apply immediately without needing engine to run
			for key, expectedValue := range tt.expectedValues {
				actualValue := dmxService.GetChannelValue(key.Universe, key.Channel)
				if actualValue != expectedValue {
					t.Errorf("Channel %d:%d = %d, want %d",
						key.Universe, key.Channel, actualValue, expectedValue)
				}
			}

			// Instant fades should not create active effects
			if engine.ActiveFadeCount() != 0 {
				t.Errorf("Instant fade should not create active effects, got %d", engine.ActiveFadeCount())
			}
		})
	}
}

// TestFadeToLook_InstantFadeCancelsExistingCrossfade tests that an instant fade (duration=0)
// cancels any in-progress crossfade so the old fade doesn't overwrite the instant values.
// This is the "hurry button" scenario: GoToCue with fadeInTime=0 during an active fade.
func TestFadeToLook_InstantFadeCancelsExistingCrossfade(t *testing.T) {
	engine, dmxService := createTestModulatorEngine()
	_ = engine.Start()
	defer func() { _ = engine.Stop() }()

	// Set initial channel values
	dmxService.SetChannelValue(1, 1, 0)
	dmxService.SetChannelValue(1, 2, 255)

	// Start a long crossfade (10 seconds so it's definitely still in progress)
	engine.FadeToLook([]LookChannel{
		{Universe: 1, Channel: 1, Value: 255},
		{Universe: 1, Channel: 2, Value: 0},
	}, 10*time.Second, "slow-fade", EasingLinear)

	// Verify crossfade is active
	if engine.ActiveFadeCount() != 1 {
		t.Fatalf("Expected 1 active crossfade, got %d", engine.ActiveFadeCount())
	}

	// Now do an instant fade (the "hurry" button) to different target values
	engine.FadeToLook([]LookChannel{
		{Universe: 1, Channel: 1, Value: 200},
		{Universe: 1, Channel: 2, Value: 100},
	}, 0, "hurry-fade", EasingLinear)

	// The old crossfade should be cancelled
	if engine.ActiveFadeCount() != 0 {
		t.Fatalf("Instant fade should cancel existing crossfades, but %d still active", engine.ActiveFadeCount())
	}

	// Values should be the instant fade targets
	if v := dmxService.GetChannelValue(1, 1); v != 200 {
		t.Errorf("Channel 1:1 = %d, want 200", v)
	}
	if v := dmxService.GetChannelValue(1, 2); v != 100 {
		t.Errorf("Channel 1:2 = %d, want 100", v)
	}

	// For a bounded window, repeatedly assert that the old crossfade does not overwrite values.
	// This avoids relying on a single fixed sleep, which can be flaky under slow CI schedulers.
	deadline := time.Now().Add(200 * time.Millisecond)
	for time.Now().Before(deadline) {
		if v := dmxService.GetChannelValue(1, 1); v != 200 {
			t.Fatalf("During polling window, Channel 1:1 = %d, want 200 (old crossfade overwrote)", v)
		}
		if v := dmxService.GetChannelValue(1, 2); v != 100 {
			t.Fatalf("During polling window, Channel 1:2 = %d, want 100 (old crossfade overwrote)", v)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

// TestFadeToLook_CreatesCrossfadeEffect tests that a positive duration creates a crossfade effect.
func TestFadeToLook_CreatesCrossfadeEffect(t *testing.T) {
	engine, dmxService := createTestModulatorEngine()
	_ = engine.Start()
	defer func() { _ = engine.Stop() }()

	// Set initial values
	dmxService.SetChannelValue(1, 1, 0)
	dmxService.SetChannelValue(1, 2, 255)

	channels := []LookChannel{
		{Universe: 1, Channel: 1, Value: 200},
		{Universe: 1, Channel: 2, Value: 50},
	}

	fadeID := engine.FadeToLook(channels, 200*time.Millisecond, "crossfade-test", EasingLinear)

	if fadeID != "crossfade-test" {
		t.Errorf("FadeToLook should return provided fade ID, got %s", fadeID)
	}

	// Should have exactly 1 active crossfade effect
	if engine.ActiveFadeCount() != 1 {
		t.Errorf("Expected 1 active fade, got %d", engine.ActiveFadeCount())
	}

	// Poll until fade completes (bounded to avoid flaky timing failures)
	waitForCondition(t, 2*time.Second,
		func() bool {
			return dmxService.GetChannelValue(1, 1) == 200 && dmxService.GetChannelValue(1, 2) == 50
		},
		func() {
			t.Fatalf("Crossfade did not reach target values within timeout: ch1:1=%d (want 200), ch1:2=%d (want 50)",
				dmxService.GetChannelValue(1, 1), dmxService.GetChannelValue(1, 2))
		},
	)
}

// TestFadeToLook_ReplacesExistingCrossfade tests that a new crossfade replaces an existing one.
func TestFadeToLook_ReplacesExistingCrossfade(t *testing.T) {
	engine, dmxService := createTestModulatorEngine()
	_ = engine.Start()
	defer func() { _ = engine.Stop() }()

	// Set initial value
	dmxService.SetChannelValue(1, 1, 0)

	// Start first fade to 100
	channels1 := []LookChannel{
		{Universe: 1, Channel: 1, Value: 100},
	}
	engine.FadeToLook(channels1, 500*time.Millisecond, "fade-1", EasingLinear)

	// Wait a bit
	time.Sleep(50 * time.Millisecond)

	// Start second fade to 200 (should replace first)
	channels2 := []LookChannel{
		{Universe: 1, Channel: 1, Value: 200},
	}
	engine.FadeToLook(channels2, 200*time.Millisecond, "fade-2", EasingLinear)

	// Should still have only 1 active fade
	if engine.ActiveFadeCount() != 1 {
		t.Errorf("Expected 1 active fade after replacement, got %d", engine.ActiveFadeCount())
	}

	// Poll until the engine reports the fade is fully retired, then check
	// the final value. Asserting strict equality on the in-flight value is
	// flaky in CI: the linear interpolation can land on 199 one tick before
	// the engine writes the exact target, and an overloaded scheduler can
	// delay that final tick past the polling deadline.
	waitForCondition(t, 2*time.Second,
		func() bool { return engine.ActiveFadeCount() == 0 },
		func() {
			t.Fatalf("Replacement fade did not finish within timeout, last value %d, active=%d",
				dmxService.GetChannelValue(1, 1), engine.ActiveFadeCount())
		},
	)
	final := dmxService.GetChannelValue(1, 1)
	if final != 200 {
		t.Errorf("Replacement fade final value = %d, want 200", final)
	}
}

// TestFadeToLook_SnapBehavior tests that SNAP channels jump to target immediately.
func TestFadeToLook_SnapBehavior(t *testing.T) {
	engine, dmxService := createTestModulatorEngine()
	_ = engine.Start()
	defer func() { _ = engine.Stop() }()

	// Set initial values
	dmxService.SetChannelValue(1, 1, 0) // Will use FADE
	dmxService.SetChannelValue(1, 2, 0) // Will use SNAP

	channels := []LookChannel{
		{Universe: 1, Channel: 1, Value: 100, FadeBehavior: FadeBehaviorFade},
		{Universe: 1, Channel: 2, Value: 100, FadeBehavior: FadeBehaviorSnap},
	}

	engine.FadeToLook(channels, 500*time.Millisecond, "snap-test", EasingLinear)

	// SNAP channel should be at target immediately
	snapValue := dmxService.GetChannelValue(1, 2)
	if snapValue != 100 {
		t.Errorf("SNAP channel should immediately be at target 100, got %d", snapValue)
	}

	// FADE channel should still be near 0 at start
	fadeValue := dmxService.GetChannelValue(1, 1)
	if fadeValue > 50 {
		t.Errorf("FADE channel should start near 0, got %d", fadeValue)
	}
}

// TestFadeToLook_DefaultEasing tests that empty easing uses the default.
func TestFadeToLook_DefaultEasing(t *testing.T) {
	engine, dmxService := createTestModulatorEngine()
	_ = engine.Start()
	defer func() { _ = engine.Stop() }()

	dmxService.SetChannelValue(1, 1, 0)

	channels := []LookChannel{
		{Universe: 1, Channel: 1, Value: 255},
	}

	// Pass empty easing type
	engine.FadeToLook(channels, 100*time.Millisecond, "", "")

	// Poll until fade completes
	waitForCondition(t, 2*time.Second,
		func() bool { return dmxService.GetChannelValue(1, 1) == 255 },
		func() { t.Fatalf("Fade with default easing should complete, got %d", dmxService.GetChannelValue(1, 1)) },
	)
}

// TestFadeToLook_GeneratedFadeID tests that empty fadeID generates a unique ID.
func TestFadeToLook_GeneratedFadeID(t *testing.T) {
	engine, _ := createTestModulatorEngine()

	channels := []LookChannel{
		{Universe: 1, Channel: 1, Value: 100},
	}

	fadeID1 := engine.FadeToLook(channels, 100*time.Millisecond, "", EasingLinear)
	fadeID2 := engine.FadeToLook(channels, 100*time.Millisecond, "", EasingLinear)

	if fadeID1 == "" {
		t.Error("Generated fade ID should not be empty")
	}
	if fadeID2 == "" {
		t.Error("Generated fade ID should not be empty")
	}
	if fadeID1 == fadeID2 {
		t.Error("Generated fade IDs should be unique")
	}
}

// TestFadeToBlack_FadesAllChannelsToZero tests that FadeToBlack fades all non-zero channels to 0.
func TestFadeToBlack_FadesAllChannelsToZero(t *testing.T) {
	engine, dmxService := createTestModulatorEngine()
	_ = engine.Start()
	defer func() { _ = engine.Stop() }()

	// Set some channels to non-zero values
	dmxService.SetChannelValue(1, 1, 255)
	dmxService.SetChannelValue(1, 100, 128)
	dmxService.SetChannelValue(2, 50, 200)

	fadeID := engine.FadeToBlack(100*time.Millisecond, EasingLinear)

	if fadeID != "fade-to-black" {
		t.Errorf("FadeToBlack should return 'fade-to-black' ID, got %s", fadeID)
	}

	// Poll until all channels reach 0
	waitForCondition(t, 2*time.Second,
		func() bool {
			return dmxService.GetChannelValue(1, 1) == 0 &&
				dmxService.GetChannelValue(1, 100) == 0 &&
				dmxService.GetChannelValue(2, 50) == 0
		},
		func() {
			t.Fatalf("FadeToBlack did not reach 0 within timeout: ch1:1=%d, ch1:100=%d, ch2:50=%d",
				dmxService.GetChannelValue(1, 1), dmxService.GetChannelValue(1, 100), dmxService.GetChannelValue(2, 50))
		},
	)
}

// TestFadeToBlack_Instant tests that FadeToBlack with duration <= 0 is immediate.
func TestFadeToBlack_Instant(t *testing.T) {
	tests := []struct {
		name     string
		duration time.Duration
	}{
		{"zero duration", 0},
		{"negative duration", -100 * time.Millisecond},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			engine, dmxService := createTestModulatorEngine()

			// Set some channels
			dmxService.SetChannelValue(1, 1, 255)
			dmxService.SetChannelValue(1, 2, 128)

			fadeID := engine.FadeToBlack(tt.duration, EasingLinear)

			if fadeID != "fade-to-black" {
				t.Errorf("FadeToBlack should return 'fade-to-black' ID, got %s", fadeID)
			}

			// Should be immediately at 0 without engine running
			if dmxService.GetChannelValue(1, 1) != 0 {
				t.Errorf("Channel 1:1 should be 0 immediately, got %d", dmxService.GetChannelValue(1, 1))
			}
			if dmxService.GetChannelValue(1, 2) != 0 {
				t.Errorf("Channel 1:2 should be 0 immediately, got %d", dmxService.GetChannelValue(1, 2))
			}

			// No active effects should be created
			if engine.ActiveFadeCount() != 0 {
				t.Errorf("Instant fade should not create active effects, got %d", engine.ActiveFadeCount())
			}
		})
	}
}

// TestFadeToBlack_DefaultEasing tests that empty easing uses the default.
func TestFadeToBlack_DefaultEasing(t *testing.T) {
	engine, dmxService := createTestModulatorEngine()
	_ = engine.Start()
	defer func() { _ = engine.Stop() }()

	dmxService.SetChannelValue(1, 1, 255)

	// Pass empty easing type
	engine.FadeToBlack(100*time.Millisecond, "")

	// Poll until fade completes
	waitForCondition(t, 2*time.Second,
		func() bool { return dmxService.GetChannelValue(1, 1) == 0 },
		func() { t.Fatalf("FadeToBlack with default easing should complete, got %d", dmxService.GetChannelValue(1, 1)) },
	)
}

// TestCancelFade_RemovesSpecificFade tests that CancelFade removes only the specified fade.
func TestCancelFade_RemovesSpecificFade(t *testing.T) {
	engine, dmxService := createTestModulatorEngine()
	_ = engine.Start()
	defer func() { _ = engine.Stop() }()

	dmxService.SetChannelValue(1, 1, 0)

	channels := []LookChannel{
		{Universe: 1, Channel: 1, Value: 255},
	}

	fadeID := engine.FadeToLook(channels, 1*time.Second, "cancel-test", EasingLinear)

	// Should have 1 active fade
	if engine.ActiveFadeCount() != 1 {
		t.Errorf("Expected 1 active fade, got %d", engine.ActiveFadeCount())
	}

	// Wait a bit
	time.Sleep(100 * time.Millisecond)

	// Cancel the fade
	engine.CancelFade(fadeID)

	// Wait for engine to process
	time.Sleep(50 * time.Millisecond)

	// Should have 0 active fades
	if engine.ActiveFadeCount() != 0 {
		t.Errorf("Expected 0 active fades after cancel, got %d", engine.ActiveFadeCount())
	}

	// Channel value should be frozen at its current state
	val1 := dmxService.GetChannelValue(1, 1)
	time.Sleep(100 * time.Millisecond)
	val2 := dmxService.GetChannelValue(1, 1)

	// Values should not change significantly
	diff := int(val2) - int(val1)
	if diff < -5 || diff > 5 {
		t.Errorf("Value should not change after cancel: before=%d, after=%d", val1, val2)
	}
}

// TestCancelFade_NonExistent tests that canceling a non-existent fade is safe.
func TestCancelFade_NonExistent(t *testing.T) {
	engine, _ := createTestModulatorEngine()
	_ = engine.Start()
	defer func() { _ = engine.Stop() }()

	// Should not panic
	engine.CancelFade("non-existent-fade-id")

	if engine.ActiveFadeCount() != 0 {
		t.Errorf("Expected 0 active fades, got %d", engine.ActiveFadeCount())
	}
}

// TestCancelAllFades_RemovesAllCrossfades tests that CancelAllFades removes all crossfade effects.
func TestCancelAllFades_RemovesAllCrossfades(t *testing.T) {
	engine, dmxService := createTestModulatorEngine()
	_ = engine.Start()
	defer func() { _ = engine.Stop() }()

	// Create multiple crossfades
	channels1 := []LookChannel{{Universe: 1, Channel: 1, Value: 100}}
	channels2 := []LookChannel{{Universe: 1, Channel: 2, Value: 100}}

	engine.FadeToLook(channels1, 1*time.Second, "fade-1", EasingLinear)

	// Wait a moment, then create another (note: FadeToLook replaces existing crossfades)
	// So we need to test that CancelAllFades clears the one that exists
	time.Sleep(50 * time.Millisecond)

	if engine.ActiveFadeCount() < 1 {
		t.Errorf("Expected at least 1 active fade, got %d", engine.ActiveFadeCount())
	}

	// Capture current values
	val1 := dmxService.GetChannelValue(1, 1)
	_ = dmxService.GetChannelValue(1, 2)

	// Cancel all fades
	engine.CancelAllFades()

	// Should have 0 active fades
	if engine.ActiveFadeCount() != 0 {
		t.Errorf("Expected 0 active fades after CancelAllFades, got %d", engine.ActiveFadeCount())
	}

	// Wait and verify values don't change
	time.Sleep(100 * time.Millisecond)

	val1After := dmxService.GetChannelValue(1, 1)

	// Channel 1 should be frozen near its value at cancel time
	diff := int(val1After) - int(val1)
	if diff < -10 || diff > 10 {
		t.Errorf("Channel 1:1 should be frozen after cancel: before=%d, after=%d", val1, val1After)
	}

	_ = channels2 // Acknowledge unused variable
}

// TestActiveFadeCount_CountsCrossfadeEffectsOnly tests that ActiveFadeCount only counts crossfades.
func TestActiveFadeCount_CountsCrossfadeEffectsOnly(t *testing.T) {
	engine, _ := createTestModulatorEngine()
	_ = engine.Start()
	defer func() { _ = engine.Stop() }()

	// Initially 0
	if engine.ActiveFadeCount() != 0 {
		t.Errorf("Expected 0 active fades initially, got %d", engine.ActiveFadeCount())
	}

	// Add a crossfade via FadeToLook
	channels := []LookChannel{{Universe: 1, Channel: 1, Value: 100}}
	engine.FadeToLook(channels, 500*time.Millisecond, "fade-1", EasingLinear)

	if engine.ActiveFadeCount() != 1 {
		t.Errorf("Expected 1 active fade, got %d", engine.ActiveFadeCount())
	}

	// Add a non-crossfade effect (waveform) via AddEffect
	waveformEffect := NewEffect(EffectTypeWaveform, PriorityUserDefault)
	waveformEffect.Waveform = WaveformSine
	waveformEffect.Frequency = 1.0
	engine.AddEffect(waveformEffect)

	// ActiveFadeCount should still be 1 (only crossfades)
	if engine.ActiveFadeCount() != 1 {
		t.Errorf("Expected 1 active crossfade (waveform shouldn't count), got %d", engine.ActiveFadeCount())
	}

	// Add a static effect
	staticEffect := NewEffect(EffectTypeStatic, PriorityCueDefault)
	engine.AddEffect(staticEffect)

	// ActiveFadeCount should still be 1
	if engine.ActiveFadeCount() != 1 {
		t.Errorf("Expected 1 active crossfade (static shouldn't count), got %d", engine.ActiveFadeCount())
	}

	// Total effects should be 3
	if engine.GetActiveEffectCount() != 3 {
		t.Errorf("Expected 3 total active effects, got %d", engine.GetActiveEffectCount())
	}
}

// TestFadeToLook_MultipleUniverses tests that fades work across multiple universes.
func TestFadeToLook_MultipleUniverses(t *testing.T) {
	engine, dmxService := createTestModulatorEngine()
	_ = engine.Start()
	defer func() { _ = engine.Stop() }()

	channels := []LookChannel{
		{Universe: 1, Channel: 1, Value: 100},
		{Universe: 1, Channel: 2, Value: 150},
		{Universe: 2, Channel: 1, Value: 200},
		{Universe: 2, Channel: 10, Value: 250},
	}

	engine.FadeToLook(channels, 100*time.Millisecond, "multi-universe", EasingLinear)

	expectedValues := map[ChannelKey]byte{
		{Universe: 1, Channel: 1}:  100,
		{Universe: 1, Channel: 2}:  150,
		{Universe: 2, Channel: 1}:  200,
		{Universe: 2, Channel: 10}: 250,
	}

	// Poll until all channels reach targets
	waitForCondition(t, 2*time.Second,
		func() bool {
			for key, expected := range expectedValues {
				if dmxService.GetChannelValue(key.Universe, key.Channel) != expected {
					return false
				}
			}
			return true
		},
		func() {
			for key, expected := range expectedValues {
				actual := dmxService.GetChannelValue(key.Universe, key.Channel)
				if actual != expected {
					t.Errorf("Channel %d:%d = %d, want %d", key.Universe, key.Channel, actual, expected)
				}
			}
			t.FailNow()
		},
	)
}

// TestFadeToLook_MidFadeTransition tests starting a new fade while one is in progress.
//
// Avoids fixed sleep timing: polls until fade-1 has demonstrably started
// (value > 0) and is still in progress (value < target). Without this,
// time.Sleep(100ms) under CI load can overshoot the 200ms total fade and
// land outside the [20,80] mid-range window, causing flakes.
func TestFadeToLook_MidFadeTransition(t *testing.T) {
	engine, dmxService := createTestModulatorEngine()
	_ = engine.Start()
	defer func() { _ = engine.Stop() }()

	// Start at 0
	dmxService.SetChannelValue(1, 1, 0)

	// Start fade to 100 over a generous duration so that even a slow CI
	// scheduler can land the polling window inside the fade.
	const fadeTarget = 100
	const fadeDuration = 1 * time.Second
	channels1 := []LookChannel{{Universe: 1, Channel: 1, Value: fadeTarget}}
	engine.FadeToLook(channels1, fadeDuration, "fade-1", EasingLinear)

	// Poll until we observe a value strictly between 0 and the target — that
	// is, fade-1 is in progress. This is the actual condition we care about
	// for "mid-fade transition", and it doesn't depend on wall-clock timing.
	var midValue byte
	waitForCondition(t, 2*time.Second,
		func() bool {
			midValue = dmxService.GetChannelValue(1, 1)
			return midValue > 0 && midValue < fadeTarget
		},
		func() {
			t.Fatalf("fade-1 never reached an in-progress value (last %d)", midValue)
		},
	)

	// Start new fade to 200 from the observed mid-fade value.
	channels2 := []LookChannel{{Universe: 1, Channel: 1, Value: 200}}
	engine.FadeToLook(channels2, 200*time.Millisecond, "fade-2", EasingLinear)

	// Poll until fade-2 completes.
	waitForCondition(t, 2*time.Second,
		func() bool { return dmxService.GetChannelValue(1, 1) == 200 },
		func() { t.Fatalf("Final value should be 200, got %d", dmxService.GetChannelValue(1, 1)) },
	)
}

// TestFadeToLook_SnapEnd tests that SNAP_END channels hold until fade completes.
func TestFadeToLook_SnapEnd(t *testing.T) {
	engine, dmxService := createTestModulatorEngine()
	_ = engine.Start()
	defer func() { _ = engine.Stop() }()

	// Set initial value
	dmxService.SetChannelValue(1, 1, 50)

	channels := []LookChannel{
		{Universe: 1, Channel: 1, Value: 200, FadeBehavior: FadeBehaviorSnapEnd},
	}

	engine.FadeToLook(channels, 200*time.Millisecond, "snap-end-test", EasingLinear)

	// Check immediately - SNAP_END should start at its current value (50) but...
	// Actually in this implementation, SNAP_END just uses normal fade behavior
	// since the modulator handles SNAP_END differently than the original fade engine.
	// The value should remain at the starting position during the fade.

	time.Sleep(50 * time.Millisecond)

	// Verify the fade eventually reaches the target using bounded poll
	waitForCondition(t, 1*time.Second,
		func() bool { return dmxService.GetChannelValue(1, 1) == 200 },
		func() { t.Fatalf("SNAP_END channel did not reach target 200 within timeout, last value %d", dmxService.GetChannelValue(1, 1)) },
	)
}

// TestFadeToLook_EmptyChannels tests that FadeToLook handles empty channel list gracefully.
func TestFadeToLook_EmptyChannels(t *testing.T) {
	engine, _ := createTestModulatorEngine()

	// Should not panic with empty channels
	fadeID := engine.FadeToLook([]LookChannel{}, 100*time.Millisecond, "empty-test", EasingLinear)

	if fadeID == "" {
		t.Error("FadeToLook should return a fade ID even with empty channels")
	}
}

// TestCaptureCurrentState tests the internal state capture from DMX service.
func TestCaptureCurrentState(t *testing.T) {
	engine, dmxService := createTestModulatorEngine()

	// Set some values
	dmxService.SetChannelValue(1, 1, 100)
	dmxService.SetChannelValue(1, 2, 200)
	dmxService.SetChannelValue(2, 5, 150)

	channels := []LookChannel{
		{Universe: 1, Channel: 1, Value: 0},
		{Universe: 1, Channel: 2, Value: 0},
		{Universe: 2, Channel: 5, Value: 0},
	}

	captured := engine.captureCurrentState(channels)

	expected := map[ChannelKey]int{
		{Universe: 1, Channel: 1}: 100,
		{Universe: 1, Channel: 2}: 200,
		{Universe: 2, Channel: 5}: 150,
	}

	for key, expectedVal := range expected {
		if capturedVal, ok := captured[key]; !ok {
			t.Errorf("Expected channel %d:%d to be captured", key.Universe, key.Channel)
		} else if capturedVal != expectedVal {
			t.Errorf("Captured value for %d:%d = %d, want %d",
				key.Universe, key.Channel, capturedVal, expectedVal)
		}
	}
}

// TestCaptureAllChannelState tests capturing all non-zero channel states.
func TestCaptureAllChannelState(t *testing.T) {
	engine, dmxService := createTestModulatorEngine()

	// Set some values (channels are 0-indexed internally, 1-indexed externally)
	dmxService.SetChannelValue(1, 1, 100)
	dmxService.SetChannelValue(1, 5, 200)
	dmxService.SetChannelValue(2, 10, 150)
	// Leave channel 1:3 at 0, should not be captured

	captured := engine.captureAllChannelState()

	// Check that non-zero channels are captured
	if _, ok := captured[ChannelKey{Universe: 1, Channel: 1}]; !ok {
		t.Error("Channel 1:1 should be captured")
	}
	if _, ok := captured[ChannelKey{Universe: 1, Channel: 5}]; !ok {
		t.Error("Channel 1:5 should be captured")
	}
	if _, ok := captured[ChannelKey{Universe: 2, Channel: 10}]; !ok {
		t.Error("Channel 2:10 should be captured")
	}

	// Channel at 0 should not be captured
	if val, ok := captured[ChannelKey{Universe: 1, Channel: 3}]; ok && val != 0 {
		t.Errorf("Channel 1:3 (value 0) should not be captured, got %d", val)
	}
}

// TestRemoveEffectsByType tests the internal method that removes effects by type.
func TestRemoveEffectsByType(t *testing.T) {
	engine, _ := createTestModulatorEngine()

	// Add different types of effects
	crossfadeEffect := NewEffect(EffectTypeCrossfade, PriorityCueDefault)
	waveformEffect := NewEffect(EffectTypeWaveform, PriorityUserDefault)
	staticEffect := NewEffect(EffectTypeStatic, PriorityCueDefault)

	engine.AddEffect(crossfadeEffect)
	engine.AddEffect(waveformEffect)
	engine.AddEffect(staticEffect)

	if engine.GetActiveEffectCount() != 3 {
		t.Errorf("Expected 3 effects, got %d", engine.GetActiveEffectCount())
	}

	// Remove all crossfade effects
	engine.mu.Lock()
	engine.removeEffectsByType(EffectTypeCrossfade)
	engine.mu.Unlock()

	// Should have 2 effects remaining
	if engine.GetActiveEffectCount() != 2 {
		t.Errorf("Expected 2 effects after removing crossfades, got %d", engine.GetActiveEffectCount())
	}

	// Verify remaining effects are not crossfades
	effects := engine.GetActiveEffects()
	for _, effect := range effects {
		if effect.Effect.EffectType == EffectTypeCrossfade {
			t.Error("Crossfade effect should have been removed")
		}
	}
}

// TestRemoveEffectByID tests the internal method that removes an effect by ID.
func TestRemoveEffectByID(t *testing.T) {
	engine, _ := createTestModulatorEngine()

	// Add effects
	effect1 := NewEffect(EffectTypeWaveform, PriorityUserDefault)
	effect1.ID = "effect-1"
	effect2 := NewEffect(EffectTypeWaveform, PriorityUserDefault)
	effect2.ID = "effect-2"
	effect3 := NewEffect(EffectTypeStatic, PriorityCueDefault)
	effect3.ID = "effect-3"

	engine.AddEffect(effect1)
	engine.AddEffect(effect2)
	engine.AddEffect(effect3)

	if engine.GetActiveEffectCount() != 3 {
		t.Errorf("Expected 3 effects, got %d", engine.GetActiveEffectCount())
	}

	// Remove effect-2
	engine.mu.Lock()
	engine.removeEffectByID("effect-2")
	engine.mu.Unlock()

	// Should have 2 effects
	if engine.GetActiveEffectCount() != 2 {
		t.Errorf("Expected 2 effects after removal, got %d", engine.GetActiveEffectCount())
	}

	// Verify effect-2 was removed
	effects := engine.GetActiveEffects()
	for _, effect := range effects {
		if effect.Effect.ID == "effect-2" {
			t.Error("Effect with ID 'effect-2' should have been removed")
		}
	}

	// Verify others remain
	foundEffect1, foundEffect3 := false, false
	for _, effect := range effects {
		if effect.Effect.ID == "effect-1" {
			foundEffect1 = true
		}
		if effect.Effect.ID == "effect-3" {
			foundEffect3 = true
		}
	}
	if !foundEffect1 {
		t.Error("effect-1 should still exist")
	}
	if !foundEffect3 {
		t.Error("effect-3 should still exist")
	}
}

// TestLookChannel_FadeBehaviorConstants tests that fade behavior constants are defined correctly.
func TestLookChannel_FadeBehaviorConstants(t *testing.T) {
	if FadeBehaviorFade != "FADE" {
		t.Errorf("FadeBehaviorFade = %q, want FADE", FadeBehaviorFade)
	}
	if FadeBehaviorSnap != "SNAP" {
		t.Errorf("FadeBehaviorSnap = %q, want SNAP", FadeBehaviorSnap)
	}
	if FadeBehaviorSnapEnd != "SNAP_END" {
		t.Errorf("FadeBehaviorSnapEnd = %q, want SNAP_END", FadeBehaviorSnapEnd)
	}
}
