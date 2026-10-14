package modulator

import (
	"testing"
	"time"
)

// startManualCrossfade starts a FadeToLook crossfade on an engine whose update
// loop is NOT running, so the test can drive ticks deterministically via
// processModulationAt. It rewinds the effect's StartTime far into the past so
// that, by wall-clock time, the crossfade is already finished. Any code path
// that consults the wall clock (rather than the tick timestamp) to decide
// completion will therefore misbehave, which is exactly the race these tests
// guard against.
func startManualCrossfade(t *testing.T, channels []LookChannel, duration time.Duration) (*Engine, func(int) byte, time.Time) {
	t.Helper()
	engine, dmxService := createTestModulatorEngine()

	engine.FadeToLook(channels, duration, "manual-crossfade", EasingInOutSine)

	engine.mu.Lock()
	if len(engine.activeEffects) != 1 {
		engine.mu.Unlock()
		t.Fatalf("expected exactly 1 active crossfade, got %d", len(engine.activeEffects))
	}
	start := time.Now().Add(-10 * duration)
	engine.activeEffects[0].StartTime = start
	engine.mu.Unlock()

	get := func(ch int) byte { return dmxService.GetChannelValue(1, ch) }
	return engine, get, start
}

func activeEffectCount(e *Engine) int {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return len(e.activeEffects)
}

// TestProcessModulation_FinalFrameUsesTickTimestamp is a deterministic
// regression test for the TestLookBoardFadeBehavior "199 vs 200" flake.
//
// processModulation used to compute channel values with one time.Now() (inside
// GetProgress) and then decide whether to remove the crossfade with another
// time.Now() (inside IsComplete). If the fade's end fell between those two
// reads, the last frame written was computed at progress < 1 (e.g. 199 for a
// target of 200 - values are truncated) and the effect was then removed, so the
// channel was left one step short of its target forever. A SNAP_END channel was
// hit even harder: it would be left at its *start* value.
//
// Values and completion must both be evaluated against the same tick timestamp
// so the final frame written before removal is always at progress == 1.
func TestProcessModulation_FinalFrameUsesTickTimestamp(t *testing.T) {
	duration := 500 * time.Millisecond
	engine, get, start := startManualCrossfade(t, []LookChannel{
		{Universe: 1, Channel: 1, Value: 200, FadeBehavior: FadeBehaviorFade},
		{Universe: 1, Channel: 2, Value: 255, FadeBehavior: FadeBehaviorSnapEnd},
	}, duration)

	// A tick stamped just before the end of the fade. The wall clock is long
	// past the end, but this tick's frame was computed at progress < 1, so the
	// crossfade must NOT be removed yet.
	engine.processModulationAt(start.Add(duration - time.Millisecond))

	if v := get(1); v >= 200 {
		t.Fatalf("FADE channel just before fade end should be short of target 200, got %d", v)
	}
	if v := get(2); v != 0 {
		t.Fatalf("SNAP_END channel just before fade end should still hold 0, got %d", v)
	}
	if n := activeEffectCount(engine); n != 1 {
		t.Fatalf("crossfade removed after a tick computed at progress < 1 (active effects = %d); "+
			"channels would be stranded short of their targets", n)
	}

	// The tick at the end of the fade writes the exact targets and only then
	// retires the crossfade.
	engine.processModulationAt(start.Add(duration))

	if v := get(1); v != 200 {
		t.Errorf("FADE channel should be exactly at target 200 after final tick, got %d", v)
	}
	if v := get(2); v != 255 {
		t.Errorf("SNAP_END channel should snap to target 255 on final tick, got %d", v)
	}
	if n := activeEffectCount(engine); n != 0 {
		t.Errorf("crossfade should be removed after its final (progress == 1) tick, active effects = %d", n)
	}
}

// TestProcessModulation_MidFadeTickUsesTickTimestamp verifies that crossfade
// values are computed from the tick timestamp rather than the wall clock.
func TestProcessModulation_MidFadeTickUsesTickTimestamp(t *testing.T) {
	duration := time.Second
	engine, get, start := startManualCrossfade(t, []LookChannel{
		{Universe: 1, Channel: 1, Value: 200, FadeBehavior: FadeBehaviorFade},
		{Universe: 1, Channel: 2, Value: 255, FadeBehavior: FadeBehaviorSnapEnd},
	}, duration)

	// Exactly halfway: ease-in-out-sine(0.5) == 0.5, so FADE = 100.
	engine.processModulationAt(start.Add(duration / 2))

	if v := get(1); v < 99 || v > 101 {
		t.Errorf("FADE channel at the fade midpoint should be ~100, got %d", v)
	}
	if v := get(2); v != 0 {
		t.Errorf("SNAP_END channel at the fade midpoint should hold 0, got %d", v)
	}
	if n := activeEffectCount(engine); n != 1 {
		t.Errorf("crossfade should still be active at its midpoint, active effects = %d", n)
	}
}

// TestActiveEffect_ProgressAndCompletionAt covers the timestamp-based helpers
// used by the tick loop.
func TestActiveEffect_ProgressAndCompletionAt(t *testing.T) {
	start := time.Now()
	active := &ActiveEffect{
		Effect:    &Effect{EffectType: EffectTypeCrossfade},
		StartTime: start,
		Intensity: 100,
		Duration:  time.Second,
	}

	cases := []struct {
		offset       time.Duration
		wantProgress float64
		wantComplete bool
	}{
		{-time.Second, 0, false}, // clamped below
		{0, 0, false},
		{250 * time.Millisecond, 0.25, false},
		{time.Second - time.Nanosecond, 1 - 1e-9, false},
		{time.Second, 1, true},
		{2 * time.Second, 1, true}, // clamped above
	}
	for _, tc := range cases {
		now := start.Add(tc.offset)
		if got := active.progressAt(now); got < tc.wantProgress-1e-9 || got > tc.wantProgress+1e-9 {
			t.Errorf("progressAt(+%v) = %v, want %v", tc.offset, got, tc.wantProgress)
		}
		if got := active.isCompleteAt(now); got != tc.wantComplete {
			t.Errorf("isCompleteAt(+%v) = %v, want %v", tc.offset, got, tc.wantComplete)
		}
	}

	// Non-crossfade effects report full progress and never complete by time.
	static := &ActiveEffect{Effect: &Effect{EffectType: EffectTypeStatic}, Intensity: 100, StartTime: start}
	if got := static.progressAt(start); got != 1 {
		t.Errorf("static progressAt = %v, want 1", got)
	}
	if static.isCompleteAt(start.Add(time.Hour)) {
		t.Error("static effect with intensity should never complete by time")
	}
}
