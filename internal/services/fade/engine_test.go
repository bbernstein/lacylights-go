package fade

import (
	"sync"
	"testing"
	"time"

	"github.com/bbernstein/lacylights-go/internal/services/dmx"
)

func createTestEngine() (*Engine, *dmx.Service) {
	dmxService := dmx.NewService(dmx.Config{Enabled: false})
	engine := NewEngine(dmxService)
	return engine, dmxService
}

func TestNewEngine(t *testing.T) {
	engine, dmxService := createTestEngine()

	if engine == nil {
		t.Fatal("NewEngine() returned nil")
	}

	if engine.dmxService != dmxService {
		t.Error("Engine should reference the provided DMX service")
	}

	if engine.updateRate != 25*time.Millisecond {
		t.Errorf("Default update rate = %v, want 25ms", engine.updateRate)
	}

	if len(engine.activeFades) != 0 {
		t.Error("New engine should have no active fades")
	}
}

func TestEngineStartStop(t *testing.T) {
	engine, _ := createTestEngine()

	// Initially not running
	if engine.IsRunning() {
		t.Error("Engine should not be running initially")
	}

	// Start engine
	engine.Start()
	time.Sleep(10 * time.Millisecond) // Give goroutine time to start

	if !engine.IsRunning() {
		t.Error("Engine should be running after Start()")
	}

	// Starting again should be a no-op
	engine.Start()
	if !engine.IsRunning() {
		t.Error("Engine should still be running after second Start()")
	}

	// Stop engine
	engine.Stop()
	time.Sleep(10 * time.Millisecond) // Give goroutine time to stop

	if engine.IsRunning() {
		t.Error("Engine should not be running after Stop()")
	}

	// Stopping again should be a no-op
	engine.Stop() // Should not panic
}

func TestFadeChannels_ImmediateFade(t *testing.T) {
	engine, dmxService := createTestEngine()
	engine.Start()
	defer engine.Stop()

	// Set initial values
	dmxService.SetChannelValue(1, 1, 0)
	dmxService.SetChannelValue(1, 2, 255)

	// Start fade (instant, 0 duration)
	targets := []ChannelTarget{
		{Universe: 1, Channel: 1, TargetValue: 255},
		{Universe: 1, Channel: 2, TargetValue: 0},
	}

	fadeID := engine.FadeChannels(targets, 0, "", EasingLinear, nil)

	if fadeID == "" {
		t.Error("FadeChannels should return a non-empty fade ID")
	}

	// Give engine time to process
	time.Sleep(50 * time.Millisecond)

	// Instant fades should complete immediately
	if dmxService.GetChannelValue(1, 1) != 255 {
		t.Errorf("Channel 1 should be 255 after instant fade, got %d", dmxService.GetChannelValue(1, 1))
	}
	if dmxService.GetChannelValue(1, 2) != 0 {
		t.Errorf("Channel 2 should be 0 after instant fade, got %d", dmxService.GetChannelValue(1, 2))
	}
}

func TestFadeChannels_WithDuration(t *testing.T) {
	engine, dmxService := createTestEngine()
	engine.Start()
	defer engine.Stop()

	// Set initial value
	dmxService.SetChannelValue(1, 1, 0)

	completed := make(chan bool, 1)
	targets := []ChannelTarget{
		{Universe: 1, Channel: 1, TargetValue: 200},
	}

	engine.FadeChannels(targets, 200*time.Millisecond, "test-fade", EasingLinear, func() {
		completed <- true
	})

	// Check mid-fade
	time.Sleep(100 * time.Millisecond)
	midValue := dmxService.GetChannelValue(1, 1)
	if midValue <= 0 || midValue >= 200 {
		t.Errorf("Mid-fade value should be between 0 and 200, got %d", midValue)
	}

	// Wait for completion
	select {
	case <-completed:
		// Expected
	case <-time.After(500 * time.Millisecond):
		t.Error("Fade should have completed within timeout")
	}

	// Final value
	finalValue := dmxService.GetChannelValue(1, 1)
	if finalValue != 200 {
		t.Errorf("Final value should be 200, got %d", finalValue)
	}
}

func TestFadeChannels_CustomID(t *testing.T) {
	engine, _ := createTestEngine()
	engine.Start()
	defer engine.Stop()

	targets := []ChannelTarget{
		{Universe: 1, Channel: 1, TargetValue: 255},
	}

	fadeID := engine.FadeChannels(targets, 100*time.Millisecond, "my-custom-id", EasingLinear, nil)

	if fadeID != "my-custom-id" {
		t.Errorf("FadeChannels should use custom ID, got %s", fadeID)
	}
}

func TestFadeChannels_ReplacesExistingFade(t *testing.T) {
	engine, dmxService := createTestEngine()
	engine.Start()
	defer engine.Stop()

	// Start first fade
	dmxService.SetChannelValue(1, 1, 0)
	targets1 := []ChannelTarget{
		{Universe: 1, Channel: 1, TargetValue: 100},
	}
	engine.FadeChannels(targets1, 500*time.Millisecond, "shared-id", EasingLinear, nil)

	// Wait a bit
	time.Sleep(50 * time.Millisecond)

	// Start second fade with same ID (should replace)
	targets2 := []ChannelTarget{
		{Universe: 1, Channel: 1, TargetValue: 200},
	}
	engine.FadeChannels(targets2, 200*time.Millisecond, "shared-id", EasingLinear, nil)

	// Wait for completion
	time.Sleep(300 * time.Millisecond)

	// Should end at 200, not 100
	finalValue := dmxService.GetChannelValue(1, 1)
	if finalValue != 200 {
		t.Errorf("Replaced fade should end at 200, got %d", finalValue)
	}
}

func TestFadeToScene(t *testing.T) {
	engine, dmxService := createTestEngine()
	engine.Start()
	defer engine.Stop()

	sceneChannels := []SceneChannel{
		{Universe: 1, Channel: 1, Value: 128},
		{Universe: 1, Channel: 2, Value: 64},
		{Universe: 2, Channel: 1, Value: 200},
	}

	fadeID := engine.FadeToScene(sceneChannels, 100*time.Millisecond, "scene-fade", "")

	if fadeID != "scene-fade" {
		t.Errorf("FadeToScene should return the provided fade ID, got %s", fadeID)
	}

	// Wait for completion
	time.Sleep(200 * time.Millisecond)

	if dmxService.GetChannelValue(1, 1) != 128 {
		t.Errorf("Channel 1,1 should be 128, got %d", dmxService.GetChannelValue(1, 1))
	}
	if dmxService.GetChannelValue(1, 2) != 64 {
		t.Errorf("Channel 1,2 should be 64, got %d", dmxService.GetChannelValue(1, 2))
	}
	if dmxService.GetChannelValue(2, 1) != 200 {
		t.Errorf("Channel 2,1 should be 200, got %d", dmxService.GetChannelValue(2, 1))
	}
}

func TestFadeToBlack(t *testing.T) {
	engine, dmxService := createTestEngine()
	engine.Start()
	defer engine.Stop()

	// Set some channels
	dmxService.SetChannelValue(1, 1, 255)
	dmxService.SetChannelValue(1, 100, 128)
	dmxService.SetChannelValue(2, 50, 200)

	// Fade to black
	fadeID := engine.FadeToBlack(100*time.Millisecond, EasingLinear)

	if fadeID != "fade-to-black" {
		t.Errorf("FadeToBlack should use 'fade-to-black' ID, got %s", fadeID)
	}

	// Wait for completion
	time.Sleep(200 * time.Millisecond)

	// All channels should be 0
	if dmxService.GetChannelValue(1, 1) != 0 {
		t.Errorf("Channel 1,1 should be 0, got %d", dmxService.GetChannelValue(1, 1))
	}
	if dmxService.GetChannelValue(1, 100) != 0 {
		t.Errorf("Channel 1,100 should be 0, got %d", dmxService.GetChannelValue(1, 100))
	}
	if dmxService.GetChannelValue(2, 50) != 0 {
		t.Errorf("Channel 2,50 should be 0, got %d", dmxService.GetChannelValue(2, 50))
	}
}

func TestCancelFade(t *testing.T) {
	engine, dmxService := createTestEngine()
	engine.Start()
	defer engine.Stop()

	dmxService.SetChannelValue(1, 1, 0)
	targets := []ChannelTarget{
		{Universe: 1, Channel: 1, TargetValue: 255},
	}

	fadeID := engine.FadeChannels(targets, 1*time.Second, "", EasingLinear, nil)

	// Wait a bit for fade to start
	time.Sleep(100 * time.Millisecond)

	// Cancel the fade
	engine.CancelFade(fadeID)

	// Get value at cancellation
	valueAtCancel := dmxService.GetChannelValue(1, 1)

	// Wait more
	time.Sleep(200 * time.Millisecond)

	// Value should not have changed significantly
	valueAfter := dmxService.GetChannelValue(1, 1)

	// Allow for some tolerance due to timing
	diff := int(valueAfter) - int(valueAtCancel)
	if diff < -10 || diff > 10 {
		t.Errorf("Value changed too much after cancel: before=%d, after=%d", valueAtCancel, valueAfter)
	}
}

func TestCancelAllFades(t *testing.T) {
	engine, dmxService := createTestEngine()
	engine.Start()
	defer engine.Stop()

	// Start multiple fades
	targets1 := []ChannelTarget{{Universe: 1, Channel: 1, TargetValue: 255}}
	targets2 := []ChannelTarget{{Universe: 1, Channel: 2, TargetValue: 255}}
	targets3 := []ChannelTarget{{Universe: 2, Channel: 1, TargetValue: 255}}

	engine.FadeChannels(targets1, 1*time.Second, "fade1", EasingLinear, nil)
	engine.FadeChannels(targets2, 1*time.Second, "fade2", EasingLinear, nil)
	engine.FadeChannels(targets3, 1*time.Second, "fade3", EasingLinear, nil)

	time.Sleep(50 * time.Millisecond)

	// Should have 3 active fades
	if engine.ActiveFadeCount() != 3 {
		t.Errorf("Expected 3 active fades, got %d", engine.ActiveFadeCount())
	}

	// Cancel all
	engine.CancelAllFades()

	// Should have 0 active fades
	if engine.ActiveFadeCount() != 0 {
		t.Errorf("Expected 0 active fades after CancelAllFades, got %d", engine.ActiveFadeCount())
	}

	// Capture values
	val1 := dmxService.GetChannelValue(1, 1)
	val2 := dmxService.GetChannelValue(1, 2)
	val3 := dmxService.GetChannelValue(2, 1)

	// Wait more
	time.Sleep(200 * time.Millisecond)

	// Values should not have changed
	if dmxService.GetChannelValue(1, 1) != val1 {
		t.Error("Channel 1,1 should not change after CancelAllFades")
	}
	if dmxService.GetChannelValue(1, 2) != val2 {
		t.Error("Channel 1,2 should not change after CancelAllFades")
	}
	if dmxService.GetChannelValue(2, 1) != val3 {
		t.Error("Channel 2,1 should not change after CancelAllFades")
	}
}

func TestActiveFadeCount(t *testing.T) {
	engine, _ := createTestEngine()
	engine.Start()
	defer engine.Stop()

	// Initially 0
	if engine.ActiveFadeCount() != 0 {
		t.Error("ActiveFadeCount should be 0 initially")
	}

	// Add two fades on DIFFERENT channels (fades on same channel get consolidated)
	targets1 := []ChannelTarget{{Universe: 1, Channel: 1, TargetValue: 255}}
	targets2 := []ChannelTarget{{Universe: 1, Channel: 2, TargetValue: 255}}
	engine.FadeChannels(targets1, 500*time.Millisecond, "fade1", EasingLinear, nil)
	engine.FadeChannels(targets2, 500*time.Millisecond, "fade2", EasingLinear, nil)

	time.Sleep(10 * time.Millisecond)

	if engine.ActiveFadeCount() != 2 {
		t.Errorf("ActiveFadeCount should be 2, got %d", engine.ActiveFadeCount())
	}
}

// TestFadeInterrupt verifies that starting a new fade on a channel that's already fading
// cancels the old fade for that channel, preventing flicker/fighting between fades.
func TestFadeInterrupt(t *testing.T) {
	engine, dmxService := createTestEngine()
	engine.Start()
	defer engine.Stop()

	// Start fade to 255 on channel 1 with a different ID
	dmxService.SetChannelValue(1, 1, 0)
	targets1 := []ChannelTarget{{Universe: 1, Channel: 1, TargetValue: 255}}
	engine.FadeChannels(targets1, 500*time.Millisecond, "scene-A", EasingLinear, nil)

	// Wait a bit so the first fade is in progress
	time.Sleep(50 * time.Millisecond)

	// Start a new fade to 100 on the SAME channel but with a DIFFERENT fade ID
	// This simulates clicking a different scene on the scene board
	targets2 := []ChannelTarget{{Universe: 1, Channel: 1, TargetValue: 100}}
	engine.FadeChannels(targets2, 500*time.Millisecond, "scene-B", EasingLinear, nil)

	// The first fade should be cancelled, only scene-B fade should be active
	if engine.ActiveFadeCount() != 1 {
		t.Errorf("After interrupt, should have 1 active fade, got %d", engine.ActiveFadeCount())
	}

	// Wait for fade to complete
	time.Sleep(600 * time.Millisecond)

	// Final value should be 100 (from scene-B), not 255 (from scene-A)
	finalValue := dmxService.GetChannelValue(1, 1)
	if finalValue != 100 {
		t.Errorf("Final value should be 100 (scene-B), got %d", finalValue)
	}
}

// TestFadePartialChannelOverlap verifies that when a new fade overlaps some channels
// of an existing fade, only the overlapping channels are removed from the old fade.
func TestFadePartialChannelOverlap(t *testing.T) {
	engine, dmxService := createTestEngine()
	engine.Start()
	defer engine.Stop()

	// Set initial values
	dmxService.SetChannelValue(1, 1, 0)
	dmxService.SetChannelValue(1, 2, 0)
	dmxService.SetChannelValue(1, 3, 0)

	// Start fade on channels 1, 2, 3 to value 255
	targets1 := []ChannelTarget{
		{Universe: 1, Channel: 1, TargetValue: 255},
		{Universe: 1, Channel: 2, TargetValue: 255},
		{Universe: 1, Channel: 3, TargetValue: 255},
	}
	engine.FadeChannels(targets1, 500*time.Millisecond, "fade-A", EasingLinear, nil)

	time.Sleep(50 * time.Millisecond)

	// Start new fade ONLY on channel 2 to value 100
	targets2 := []ChannelTarget{
		{Universe: 1, Channel: 2, TargetValue: 100},
	}
	engine.FadeChannels(targets2, 500*time.Millisecond, "fade-B", EasingLinear, nil)

	// Should still have 2 active fades (fade-A still controls channels 1 and 3)
	if engine.ActiveFadeCount() != 2 {
		t.Errorf("Should have 2 active fades, got %d", engine.ActiveFadeCount())
	}

	// Wait for both fades to complete
	time.Sleep(600 * time.Millisecond)

	// Channel 1 should be 255 (from fade-A, uninterrupted)
	// Channel 2 should be 100 (from fade-B, took over)
	// Channel 3 should be 255 (from fade-A, uninterrupted)
	if dmxService.GetChannelValue(1, 1) != 255 {
		t.Errorf("Channel 1 should be 255, got %d", dmxService.GetChannelValue(1, 1))
	}
	if dmxService.GetChannelValue(1, 2) != 100 {
		t.Errorf("Channel 2 should be 100, got %d", dmxService.GetChannelValue(1, 2))
	}
	if dmxService.GetChannelValue(1, 3) != 255 {
		t.Errorf("Channel 3 should be 255, got %d", dmxService.GetChannelValue(1, 3))
	}
}

func TestMidFadeTransition(t *testing.T) {
	engine, dmxService := createTestEngine()
	engine.Start()
	defer engine.Stop()

	// Start fade to 100
	dmxService.SetChannelValue(1, 1, 0)
	targets1 := []ChannelTarget{{Universe: 1, Channel: 1, TargetValue: 100}}
	engine.FadeChannels(targets1, 200*time.Millisecond, "fade", EasingLinear, nil)

	// Wait until mid-fade
	time.Sleep(100 * time.Millisecond)
	midValue := dmxService.GetChannelValue(1, 1)

	// Start new fade from current position to 200
	targets2 := []ChannelTarget{{Universe: 1, Channel: 1, TargetValue: 200}}
	engine.FadeChannels(targets2, 200*time.Millisecond, "fade", EasingLinear, nil)

	// Wait for completion
	time.Sleep(300 * time.Millisecond)

	finalValue := dmxService.GetChannelValue(1, 1)
	if finalValue != 200 {
		t.Errorf("Final value should be 200, got %d", finalValue)
	}

	// Mid-transition should have been smooth (value was somewhere between 0 and 100)
	if midValue <= 0 || midValue >= 100 {
		t.Errorf("Mid-transition value should be between 0 and 100, got %d", midValue)
	}
}

func TestCompletionCallback(t *testing.T) {
	engine, _ := createTestEngine()
	engine.Start()
	defer engine.Stop()

	var wg sync.WaitGroup
	wg.Add(1)

	callbackExecuted := false
	targets := []ChannelTarget{{Universe: 1, Channel: 1, TargetValue: 255}}

	engine.FadeChannels(targets, 50*time.Millisecond, "", EasingLinear, func() {
		callbackExecuted = true
		wg.Done()
	})

	// Wait for completion with timeout
	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		// Expected
	case <-time.After(500 * time.Millisecond):
		t.Fatal("Callback was not executed within timeout")
	}

	if !callbackExecuted {
		t.Error("Completion callback should have been executed")
	}
}

func TestDefaultEasingType(t *testing.T) {
	engine, dmxService := createTestEngine()
	engine.Start()
	defer engine.Stop()

	// FadeChannels with empty easing should use default
	dmxService.SetChannelValue(1, 1, 0)
	targets := []ChannelTarget{{Universe: 1, Channel: 1, TargetValue: 255}}

	engine.FadeChannels(targets, 50*time.Millisecond, "", "", nil)

	// Wait for completion
	time.Sleep(100 * time.Millisecond)

	if dmxService.GetChannelValue(1, 1) != 255 {
		t.Error("Fade with default easing should complete successfully")
	}
}

func TestClamp(t *testing.T) {
	tests := []struct {
		value, min, max, want int
	}{
		{50, 0, 100, 50},   // Within range
		{-10, 0, 100, 0},   // Below min
		{150, 0, 100, 100}, // Above max
		{0, 0, 100, 0},     // At min
		{100, 0, 100, 100}, // At max
	}

	for _, tt := range tests {
		got := clamp(tt.value, tt.min, tt.max)
		if got != tt.want {
			t.Errorf("clamp(%d, %d, %d) = %d, want %d", tt.value, tt.min, tt.max, got, tt.want)
		}
	}
}
