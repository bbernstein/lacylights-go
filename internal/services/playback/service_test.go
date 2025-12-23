package playback

import (
	"testing"
	"time"
)

func TestCueForPlayback(t *testing.T) {
	followTime := 2.0
	cue := &CueForPlayback{
		ID:          "cue-1",
		Name:        "Test Cue",
		CueNumber:   1.0,
		FadeInTime:  3.0,
		FadeOutTime: 2.0,
		FollowTime:  &followTime,
	}

	if cue.ID != "cue-1" {
		t.Errorf("Expected ID 'cue-1', got %s", cue.ID)
	}
	if cue.Name != "Test Cue" {
		t.Errorf("Expected Name 'Test Cue', got %s", cue.Name)
	}
	if cue.CueNumber != 1.0 {
		t.Errorf("Expected CueNumber 1.0, got %f", cue.CueNumber)
	}
	if cue.FadeInTime != 3.0 {
		t.Errorf("Expected FadeInTime 3.0, got %f", cue.FadeInTime)
	}
	if cue.FadeOutTime != 2.0 {
		t.Errorf("Expected FadeOutTime 2.0, got %f", cue.FadeOutTime)
	}
	if cue.FollowTime == nil || *cue.FollowTime != 2.0 {
		t.Errorf("Expected FollowTime 2.0, got %v", cue.FollowTime)
	}
}

func TestPlaybackState(t *testing.T) {
	now := time.Now()
	cueIndex := 0
	state := &PlaybackState{
		CueListID:       "cue-list-1",
		CurrentCueIndex: &cueIndex,
		IsPlaying:       true,
		IsFading:        true,
		CurrentCue: &CueForPlayback{
			ID:          "cue-1",
			Name:        "Opening",
			CueNumber:   1.0,
			FadeInTime:  3.0,
			FadeOutTime: 2.0,
		},
		FadeProgress: 50.0,
		StartTime:    &now,
		LastUpdated:  now,
	}

	if state.CueListID != "cue-list-1" {
		t.Errorf("Expected CueListID 'cue-list-1', got %s", state.CueListID)
	}
	if !state.IsPlaying {
		t.Error("Expected IsPlaying to be true")
	}
	if !state.IsFading {
		t.Error("Expected IsFading to be true")
	}
	if state.CurrentCueIndex == nil || *state.CurrentCueIndex != 0 {
		t.Errorf("Expected CurrentCueIndex 0, got %v", state.CurrentCueIndex)
	}
	if state.FadeProgress != 50.0 {
		t.Errorf("Expected FadeProgress 50.0, got %f", state.FadeProgress)
	}
}

func TestCueListPlaybackStatus(t *testing.T) {
	cueIndex := 1
	status := &CueListPlaybackStatus{
		CueListID:       "cue-list-1",
		CurrentCueIndex: &cueIndex,
		IsPlaying:       true,
		IsFading:        true,
		CurrentCue: &CueForPlayback{
			ID:          "cue-2",
			Name:        "Scene Two",
			CueNumber:   2.0,
			FadeInTime:  2.0,
			FadeOutTime: 1.0,
		},
		FadeProgress: 75.5,
		LastUpdated:  "2025-11-26T10:00:00Z",
	}

	if status.CueListID != "cue-list-1" {
		t.Errorf("Expected CueListID 'cue-list-1', got %s", status.CueListID)
	}
	if !status.IsPlaying {
		t.Error("Expected IsPlaying to be true")
	}
	if !status.IsFading {
		t.Error("Expected IsFading to be true")
	}
	if status.CurrentCueIndex == nil || *status.CurrentCueIndex != 1 {
		t.Errorf("Expected CurrentCueIndex 1, got %v", status.CurrentCueIndex)
	}
	if status.FadeProgress != 75.5 {
		t.Errorf("Expected FadeProgress 75.5, got %f", status.FadeProgress)
	}
	if status.CurrentCue == nil || status.CurrentCue.Name != "Scene Two" {
		t.Error("Expected CurrentCue to have Name 'Scene Two'")
	}
}

func TestGetFormattedStatus_NilState(t *testing.T) {
	// Create service without any database (just testing the nil state case)
	service := &Service{
		states:              make(map[string]*PlaybackState),
		fadeProgressTickers: make(map[string]*time.Ticker),
		followTimers:        make(map[string]*time.Timer),
		fadeCompleteTimers:  make(map[string]*time.Timer),
	}

	status := service.GetFormattedStatus("nonexistent-cue-list")

	if status.CueListID != "nonexistent-cue-list" {
		t.Errorf("Expected CueListID 'nonexistent-cue-list', got %s", status.CueListID)
	}
	if status.IsPlaying {
		t.Error("Expected IsPlaying to be false for nonexistent cue list")
	}
	if status.IsFading {
		t.Error("Expected IsFading to be false for nonexistent cue list")
	}
	if status.CurrentCueIndex != nil {
		t.Error("Expected CurrentCueIndex to be nil")
	}
	if status.FadeProgress != 0 {
		t.Errorf("Expected FadeProgress 0, got %f", status.FadeProgress)
	}
}

func TestGetPlaybackState_NilState(t *testing.T) {
	service := &Service{
		states:              make(map[string]*PlaybackState),
		fadeProgressTickers: make(map[string]*time.Ticker),
		followTimers:        make(map[string]*time.Timer),
		fadeCompleteTimers:  make(map[string]*time.Timer),
	}

	state := service.GetPlaybackState("nonexistent-cue-list")
	if state != nil {
		t.Error("Expected nil state for nonexistent cue list")
	}
}

func TestSetUpdateCallback(t *testing.T) {
	service := &Service{
		states:              make(map[string]*PlaybackState),
		fadeProgressTickers: make(map[string]*time.Ticker),
		followTimers:        make(map[string]*time.Timer),
		fadeCompleteTimers:  make(map[string]*time.Timer),
	}

	callbackCalled := false
	service.SetUpdateCallback(func(status *CueListPlaybackStatus) {
		callbackCalled = true
	})

	// Trigger an emit (this will call the callback)
	service.emitUpdate("test-cue-list")

	if !callbackCalled {
		t.Error("Expected callback to be called")
	}
}

func TestStopCueList(t *testing.T) {
	service := &Service{
		states:              make(map[string]*PlaybackState),
		fadeProgressTickers: make(map[string]*time.Ticker),
		followTimers:        make(map[string]*time.Timer),
		fadeCompleteTimers:  make(map[string]*time.Timer),
	}

	// Set up a playing state
	cueIndex := 0
	service.states["test-cue-list"] = &PlaybackState{
		CueListID:       "test-cue-list",
		CurrentCueIndex: &cueIndex,
		IsPlaying:       true,
		FadeProgress:    50.0,
		LastUpdated:     time.Now(),
	}

	// Create a ticker and timer to test cleanup
	ticker := time.NewTicker(100 * time.Millisecond)
	service.fadeProgressTickers["test-cue-list"] = ticker

	timer := time.NewTimer(10 * time.Second)
	service.followTimers["test-cue-list"] = timer

	// Stop the cue list
	service.StopCueList("test-cue-list")

	// Verify state was updated
	state := service.GetPlaybackState("test-cue-list")
	if state.IsPlaying {
		t.Error("Expected IsPlaying to be false after stop")
	}
	if state.FadeProgress != 0 {
		t.Errorf("Expected FadeProgress 0 after stop, got %f", state.FadeProgress)
	}

	// Verify ticker and timer were cleaned up
	if _, exists := service.fadeProgressTickers["test-cue-list"]; exists {
		t.Error("Expected fade progress ticker to be removed")
	}
	if _, exists := service.followTimers["test-cue-list"]; exists {
		t.Error("Expected follow timer to be removed")
	}
}

func TestCleanup(t *testing.T) {
	service := &Service{
		states:              make(map[string]*PlaybackState),
		fadeProgressTickers: make(map[string]*time.Ticker),
		followTimers:        make(map[string]*time.Timer),
		fadeCompleteTimers:  make(map[string]*time.Timer),
	}

	// Add some test data
	cueIndex := 0
	service.states["test-1"] = &PlaybackState{
		CueListID:       "test-1",
		CurrentCueIndex: &cueIndex,
		IsPlaying:       true,
	}
	service.states["test-2"] = &PlaybackState{
		CueListID:       "test-2",
		CurrentCueIndex: &cueIndex,
		IsPlaying:       true,
	}

	service.fadeProgressTickers["test-1"] = time.NewTicker(100 * time.Millisecond)
	service.followTimers["test-1"] = time.NewTimer(10 * time.Second)

	// Cleanup
	service.Cleanup()

	// Verify everything is cleared
	if len(service.states) != 0 {
		t.Errorf("Expected 0 states after cleanup, got %d", len(service.states))
	}
	if len(service.fadeProgressTickers) != 0 {
		t.Errorf("Expected 0 tickers after cleanup, got %d", len(service.fadeProgressTickers))
	}
	if len(service.followTimers) != 0 {
		t.Errorf("Expected 0 timers after cleanup, got %d", len(service.followTimers))
	}
}

func TestStopAllCueLists(t *testing.T) {
	service := &Service{
		states:              make(map[string]*PlaybackState),
		fadeProgressTickers: make(map[string]*time.Ticker),
		followTimers:        make(map[string]*time.Timer),
		fadeCompleteTimers:  make(map[string]*time.Timer),
	}

	// Set up multiple playing states
	cueIndex := 0
	service.states["cue-list-1"] = &PlaybackState{
		CueListID:       "cue-list-1",
		CurrentCueIndex: &cueIndex,
		IsPlaying:       true,
		FadeProgress:    30.0,
		LastUpdated:     time.Now(),
	}
	service.states["cue-list-2"] = &PlaybackState{
		CueListID:       "cue-list-2",
		CurrentCueIndex: &cueIndex,
		IsPlaying:       true,
		FadeProgress:    60.0,
		LastUpdated:     time.Now(),
	}

	// Stop all
	service.StopAllCueLists()

	// Verify both are stopped
	for _, id := range []string{"cue-list-1", "cue-list-2"} {
		state := service.GetPlaybackState(id)
		if state.IsPlaying {
			t.Errorf("Expected %s IsPlaying to be false", id)
		}
		if state.FadeProgress != 0 {
			t.Errorf("Expected %s FadeProgress 0, got %f", id, state.FadeProgress)
		}
	}
}

// TestIsFadingTransitions tests the isFading state transitions during cue playback.
func TestIsFadingTransitions(t *testing.T) {
	service := &Service{
		states:              make(map[string]*PlaybackState),
		fadeProgressTickers: make(map[string]*time.Ticker),
		followTimers:        make(map[string]*time.Timer),
		fadeCompleteTimers:  make(map[string]*time.Timer),
	}

	cueListID := "test-cue-list"
	cueIndex := 0
	cue := &CueForPlayback{
		ID:          "cue-1",
		Name:        "Test Cue",
		CueNumber:   1.0,
		FadeInTime:  0.2, // 200ms for quick test
		FadeOutTime: 0.1,
		FollowTime:  nil,
	}

	// Start the cue
	service.StartCue(cueListID, cueIndex, cue)

	// Immediately check: both should be true at start
	state := service.GetPlaybackState(cueListID)
	if !state.IsPlaying {
		t.Error("Expected IsPlaying to be true at start")
	}
	if !state.IsFading {
		t.Error("Expected IsFading to be true at start")
	}

	// Wait for fade to complete (200ms + buffer)
	time.Sleep(300 * time.Millisecond)

	// After fade completes: IsPlaying should still be true, IsFading should be false
	state = service.GetPlaybackState(cueListID)
	if !state.IsPlaying {
		t.Error("Expected IsPlaying to be true after fade completes (scene still active)")
	}
	if state.IsFading {
		t.Error("Expected IsFading to be false after fade completes")
	}
}

// TestIsPlayingStaysAfterFade tests that isPlaying stays true after fade completes.
func TestIsPlayingStaysAfterFade(t *testing.T) {
	service := &Service{
		states:              make(map[string]*PlaybackState),
		fadeProgressTickers: make(map[string]*time.Ticker),
		followTimers:        make(map[string]*time.Timer),
		fadeCompleteTimers:  make(map[string]*time.Timer),
	}

	cueListID := "test-cue-list"
	cueIndex := 0
	cue := &CueForPlayback{
		ID:          "cue-1",
		Name:        "Test Cue",
		CueNumber:   1.0,
		FadeInTime:  0.15, // 150ms
		FadeOutTime: 0.1,
		FollowTime:  nil,
	}

	// Start the cue
	service.StartCue(cueListID, cueIndex, cue)

	// Check state during fade (within first 150ms)
	time.Sleep(50 * time.Millisecond)
	state := service.GetPlaybackState(cueListID)
	if !state.IsPlaying {
		t.Error("Expected IsPlaying true during fade")
	}
	if !state.IsFading {
		t.Error("Expected IsFading true during fade")
	}

	// Wait for fade to complete
	time.Sleep(200 * time.Millisecond)

	// Check state after fade: scene should still be active (IsPlaying=true) but not fading
	state = service.GetPlaybackState(cueListID)
	if !state.IsPlaying {
		t.Error("Expected IsPlaying to remain true after fade completes")
	}
	if state.IsFading {
		t.Error("Expected IsFading to be false after fade completes")
	}

	// Verify FadeProgress is at 100%
	if state.FadeProgress != 100.0 {
		t.Errorf("Expected FadeProgress to be 100.0, got %f", state.FadeProgress)
	}
}

// TestStopCueListSetsIsPlayingAndIsFadingToFalse tests that StopCueList sets both flags to false.
func TestStopCueListSetsIsPlayingAndIsFadingToFalse(t *testing.T) {
	service := &Service{
		states:              make(map[string]*PlaybackState),
		fadeProgressTickers: make(map[string]*time.Ticker),
		followTimers:        make(map[string]*time.Timer),
		fadeCompleteTimers:  make(map[string]*time.Timer),
	}

	cueListID := "test-cue-list"
	cueIndex := 0

	// Set up a playing and fading state
	service.states[cueListID] = &PlaybackState{
		CueListID:       cueListID,
		CurrentCueIndex: &cueIndex,
		IsPlaying:       true,
		IsFading:        true,
		FadeProgress:    50.0,
		LastUpdated:     time.Now(),
	}

	// Create a ticker and timer to test cleanup
	ticker := time.NewTicker(100 * time.Millisecond)
	service.fadeProgressTickers[cueListID] = ticker

	timer := time.NewTimer(10 * time.Second)
	service.followTimers[cueListID] = timer

	// Stop the cue list
	service.StopCueList(cueListID)

	// Verify both IsPlaying and IsFading are false
	state := service.GetPlaybackState(cueListID)
	if state.IsPlaying {
		t.Error("Expected IsPlaying to be false after stop")
	}
	if state.IsFading {
		t.Error("Expected IsFading to be false after stop")
	}
	if state.FadeProgress != 0 {
		t.Errorf("Expected FadeProgress 0 after stop, got %f", state.FadeProgress)
	}
}

// TestStopCueListCleansFadeCompleteTimer tests that StopCueList properly stops the fade completion timer.
func TestStopCueListCleansFadeCompleteTimer(t *testing.T) {
	service := &Service{
		states:              make(map[string]*PlaybackState),
		fadeProgressTickers: make(map[string]*time.Ticker),
		followTimers:        make(map[string]*time.Timer),
		fadeCompleteTimers:  make(map[string]*time.Timer),
	}

	cueListID := "test-cue-list"
	cueIndex := 0
	cue := &CueForPlayback{
		ID:          "cue-1",
		Name:        "Test Cue",
		CueNumber:   1.0,
		FadeInTime:  1.0, // 1 second fade
		FadeOutTime: 0.5,
		FollowTime:  nil,
	}

	// Start the cue - this should create a fade completion timer
	service.StartCue(cueListID, cueIndex, cue)

	// Verify the fade completion timer was created
	if _, exists := service.fadeCompleteTimers[cueListID]; !exists {
		t.Error("Expected fade completion timer to be created")
	}

	// Stop the cue list before fade completes
	service.StopCueList(cueListID)

	// Verify the fade completion timer was cleaned up
	if _, exists := service.fadeCompleteTimers[cueListID]; exists {
		t.Error("Expected fade completion timer to be removed after StopCueList")
	}

	// Verify state shows not fading
	state := service.GetPlaybackState(cueListID)
	if state.IsFading {
		t.Error("Expected IsFading to be false after stop")
	}
	if state.IsPlaying {
		t.Error("Expected IsPlaying to be false after stop")
	}
}

// TestFadeCompleteTimerDoesNotFireAfterStop tests that stopped timers don't incorrectly modify state.
func TestFadeCompleteTimerDoesNotFireAfterStop(t *testing.T) {
	service := &Service{
		states:              make(map[string]*PlaybackState),
		fadeProgressTickers: make(map[string]*time.Ticker),
		followTimers:        make(map[string]*time.Timer),
		fadeCompleteTimers:  make(map[string]*time.Timer),
	}

	cueListID := "test-cue-list"
	cueIndex := 0
	cue := &CueForPlayback{
		ID:          "cue-1",
		Name:        "Test Cue",
		CueNumber:   1.0,
		FadeInTime:  0.1, // 100ms fade
		FadeOutTime: 0.05,
		FollowTime:  nil,
	}

	// Start a cue
	service.StartCue(cueListID, cueIndex, cue)

	// Immediately stop it (before fade completes)
	service.StopCueList(cueListID)

	// Verify IsFading is false after stop
	state := service.GetPlaybackState(cueListID)
	if state.IsFading {
		t.Error("Expected IsFading to be false immediately after stop")
	}

	// Wait longer than the fade time to ensure the timer doesn't fire and change state
	time.Sleep(200 * time.Millisecond)

	// IsFading should still be false (the stopped timer shouldn't have changed it)
	state = service.GetPlaybackState(cueListID)
	if state.IsFading {
		t.Error("Expected IsFading to remain false after timer would have fired")
	}
	if state.IsPlaying {
		t.Error("Expected IsPlaying to remain false after timer would have fired")
	}
}

// TestGetFormattedStatusIncludesIsFading tests that formatted status includes isFading field.
func TestGetFormattedStatusIncludesIsFading(t *testing.T) {
	service := &Service{
		states:              make(map[string]*PlaybackState),
		fadeProgressTickers: make(map[string]*time.Ticker),
		followTimers:        make(map[string]*time.Timer),
		fadeCompleteTimers:  make(map[string]*time.Timer),
	}

	cueListID := "test-cue-list"
	cueIndex := 0

	// Set up a state with IsFading true
	service.states[cueListID] = &PlaybackState{
		CueListID:       cueListID,
		CurrentCueIndex: &cueIndex,
		IsPlaying:       true,
		IsFading:        true,
		FadeProgress:    25.0,
		LastUpdated:     time.Now(),
	}

	// Get formatted status
	status := service.GetFormattedStatus(cueListID)

	// Verify IsFading is included
	if !status.IsFading {
		t.Error("Expected IsFading to be true in formatted status")
	}
	if !status.IsPlaying {
		t.Error("Expected IsPlaying to be true in formatted status")
	}

	// Now set IsFading to false and verify
	service.states[cueListID].IsFading = false

	status = service.GetFormattedStatus(cueListID)
	if status.IsFading {
		t.Error("Expected IsFading to be false in formatted status")
	}
	if !status.IsPlaying {
		t.Error("Expected IsPlaying to still be true in formatted status")
	}
}

// TestPlaybackStateStructHasIsFading verifies PlaybackState has IsFading field.
func TestPlaybackStateStructHasIsFading(t *testing.T) {
	now := time.Now()
	cueIndex := 0
	state := &PlaybackState{
		CueListID:       "cue-list-1",
		CurrentCueIndex: &cueIndex,
		IsPlaying:       true,
		IsFading:        true,
		CurrentCue: &CueForPlayback{
			ID:          "cue-1",
			Name:        "Test",
			CueNumber:   1.0,
			FadeInTime:  3.0,
			FadeOutTime: 2.0,
		},
		FadeProgress: 50.0,
		StartTime:    &now,
		LastUpdated:  now,
	}

	if !state.IsFading {
		t.Error("Expected IsFading to be true")
	}

	state.IsFading = false
	if state.IsFading {
		t.Error("Expected IsFading to be false after setting to false")
	}
}

// TestCueListPlaybackStatusStructHasIsFading verifies CueListPlaybackStatus has IsFading field.
func TestCueListPlaybackStatusStructHasIsFading(t *testing.T) {
	cueIndex := 1
	status := &CueListPlaybackStatus{
		CueListID:       "cue-list-1",
		CurrentCueIndex: &cueIndex,
		IsPlaying:       true,
		IsFading:        true,
		CurrentCue: &CueForPlayback{
			ID:          "cue-2",
			Name:        "Scene Two",
			CueNumber:   2.0,
			FadeInTime:  2.0,
			FadeOutTime: 1.0,
		},
		FadeProgress: 75.5,
		LastUpdated:  "2025-11-26T10:00:00Z",
	}

	if !status.IsFading {
		t.Error("Expected IsFading to be true")
	}

	status.IsFading = false
	if status.IsFading {
		t.Error("Expected IsFading to be false after setting to false")
	}
}

func TestGlobalPlaybackStatus(t *testing.T) {
	cueIndex := 1
	cueCount := 5
	cueListID := "cue-list-1"
	cueListName := "Main Show"
	cueName := "Opening"

	status := &GlobalPlaybackStatus{
		IsPlaying:       true,
		IsFading:        true,
		CueListID:       &cueListID,
		CueListName:     &cueListName,
		CurrentCueIndex: &cueIndex,
		CueCount:        &cueCount,
		CurrentCueName:  &cueName,
		FadeProgress:    50.0,
		LastUpdated:     "2025-12-23T10:00:00Z",
	}

	if !status.IsPlaying {
		t.Error("Expected IsPlaying to be true")
	}
	if !status.IsFading {
		t.Error("Expected IsFading to be true")
	}
	if status.CueListID == nil || *status.CueListID != "cue-list-1" {
		t.Error("Expected CueListID to be cue-list-1")
	}
	if status.CueListName == nil || *status.CueListName != "Main Show" {
		t.Error("Expected CueListName to be Main Show")
	}
	if status.CurrentCueIndex == nil || *status.CurrentCueIndex != 1 {
		t.Error("Expected CurrentCueIndex to be 1")
	}
	if status.CueCount == nil || *status.CueCount != 5 {
		t.Error("Expected CueCount to be 5")
	}
	if status.CurrentCueName == nil || *status.CurrentCueName != "Opening" {
		t.Error("Expected CurrentCueName to be Opening")
	}
	if status.FadeProgress != 50.0 {
		t.Errorf("Expected FadeProgress 50.0, got %f", status.FadeProgress)
	}
}

func TestGlobalPlaybackStatus_NotPlaying(t *testing.T) {
	status := &GlobalPlaybackStatus{
		IsPlaying:   false,
		IsFading:    false,
		LastUpdated: "2025-12-23T10:00:00Z",
	}

	if status.IsPlaying {
		t.Error("Expected IsPlaying to be false")
	}
	if status.IsFading {
		t.Error("Expected IsFading to be false")
	}
	if status.CueListID != nil {
		t.Error("Expected CueListID to be nil when not playing")
	}
	if status.CueListName != nil {
		t.Error("Expected CueListName to be nil when not playing")
	}
	if status.CurrentCueIndex != nil {
		t.Error("Expected CurrentCueIndex to be nil when not playing")
	}
}

func TestSetGlobalUpdateCallback(t *testing.T) {
	service := &Service{
		states:              make(map[string]*PlaybackState),
		fadeProgressTickers: make(map[string]*time.Ticker),
		followTimers:        make(map[string]*time.Timer),
		fadeCompleteTimers:  make(map[string]*time.Timer),
	}

	callbackCalled := false
	service.SetGlobalUpdateCallback(func(status *GlobalPlaybackStatus) {
		callbackCalled = true
	})

	// Trigger an emit (this will call both callbacks)
	service.emitUpdate("test-cue-list")

	if !callbackCalled {
		t.Error("Expected global callback to be called")
	}
}

func TestGetPlaybackState_WithAllFields(t *testing.T) {
	service := &Service{
		states:              make(map[string]*PlaybackState),
		fadeProgressTickers: make(map[string]*time.Ticker),
		followTimers:        make(map[string]*time.Timer),
		fadeCompleteTimers:  make(map[string]*time.Timer),
	}

	now := time.Now()
	cueIndex := 2
	followTime := 3.0
	cue := &CueForPlayback{
		ID:          "cue-3",
		Name:        "Act 2 Opening",
		CueNumber:   3.0,
		FadeInTime:  5.0,
		FadeOutTime: 3.0,
		FollowTime:  &followTime,
	}

	service.states["test-cue-list"] = &PlaybackState{
		CueListID:       "test-cue-list",
		CurrentCueIndex: &cueIndex,
		IsPlaying:       true,
		IsFading:        true,
		CurrentCue:      cue,
		FadeProgress:    75.0,
		StartTime:       &now,
		LastUpdated:     now,
	}

	state := service.GetPlaybackState("test-cue-list")

	// Verify all fields are returned correctly
	if state == nil {
		t.Fatal("Expected non-nil state")
	}
	if state.CueListID != "test-cue-list" {
		t.Errorf("Expected CueListID 'test-cue-list', got %s", state.CueListID)
	}
	if state.CurrentCueIndex == nil || *state.CurrentCueIndex != 2 {
		t.Errorf("Expected CurrentCueIndex 2, got %v", state.CurrentCueIndex)
	}
	if !state.IsPlaying {
		t.Error("Expected IsPlaying to be true")
	}
	if !state.IsFading {
		t.Error("Expected IsFading to be true")
	}
	if state.CurrentCue == nil {
		t.Fatal("Expected non-nil CurrentCue")
	}
	if state.CurrentCue.Name != "Act 2 Opening" {
		t.Errorf("Expected CurrentCue.Name 'Act 2 Opening', got %s", state.CurrentCue.Name)
	}
	if state.FadeProgress != 75.0 {
		t.Errorf("Expected FadeProgress 75.0, got %f", state.FadeProgress)
	}
	if state.StartTime == nil {
		t.Error("Expected non-nil StartTime")
	}

	// Verify it's a copy (modifying returned state doesn't affect original)
	*state.CurrentCueIndex = 99
	originalState := service.states["test-cue-list"]
	if *originalState.CurrentCueIndex != 2 {
		t.Error("GetPlaybackState should return a copy, not the original")
	}
}

func TestGetFormattedStatus_WithState(t *testing.T) {
	service := &Service{
		states:              make(map[string]*PlaybackState),
		fadeProgressTickers: make(map[string]*time.Ticker),
		followTimers:        make(map[string]*time.Timer),
		fadeCompleteTimers:  make(map[string]*time.Timer),
	}

	now := time.Now()
	cueIndex := 1
	cue := &CueForPlayback{
		ID:          "cue-1",
		Name:        "Scene 1",
		CueNumber:   1.0,
		FadeInTime:  3.0,
		FadeOutTime: 2.0,
	}

	service.states["test-cue-list"] = &PlaybackState{
		CueListID:       "test-cue-list",
		CurrentCueIndex: &cueIndex,
		IsPlaying:       true,
		IsFading:        false,
		CurrentCue:      cue,
		FadeProgress:    100.0,
		LastUpdated:     now,
	}

	status := service.GetFormattedStatus("test-cue-list")

	if status.CueListID != "test-cue-list" {
		t.Errorf("Expected CueListID 'test-cue-list', got %s", status.CueListID)
	}
	if !status.IsPlaying {
		t.Error("Expected IsPlaying to be true")
	}
	if status.IsFading {
		t.Error("Expected IsFading to be false")
	}
	if status.CurrentCueIndex == nil || *status.CurrentCueIndex != 1 {
		t.Errorf("Expected CurrentCueIndex 1, got %v", status.CurrentCueIndex)
	}
	if status.CurrentCue == nil || status.CurrentCue.Name != "Scene 1" {
		t.Error("Expected CurrentCue with Name 'Scene 1'")
	}
	if status.FadeProgress != 100.0 {
		t.Errorf("Expected FadeProgress 100.0, got %f", status.FadeProgress)
	}
	// Check that LastUpdated is a valid RFC3339 timestamp
	_, err := time.Parse(time.RFC3339, status.LastUpdated)
	if err != nil {
		t.Errorf("Expected valid RFC3339 timestamp, got parse error: %v", err)
	}
}

func TestEmitUpdate_CallsBothCallbacks(t *testing.T) {
	service := &Service{
		states:              make(map[string]*PlaybackState),
		fadeProgressTickers: make(map[string]*time.Ticker),
		followTimers:        make(map[string]*time.Timer),
		fadeCompleteTimers:  make(map[string]*time.Timer),
	}

	localCallbackCalled := false
	globalCallbackCalled := false
	var localStatus *CueListPlaybackStatus
	var globalStatus *GlobalPlaybackStatus

	service.SetUpdateCallback(func(status *CueListPlaybackStatus) {
		localCallbackCalled = true
		localStatus = status
	})

	service.SetGlobalUpdateCallback(func(status *GlobalPlaybackStatus) {
		globalCallbackCalled = true
		globalStatus = status
	})

	// Set up a playing state
	cueIndex := 0
	service.states["test-cue-list"] = &PlaybackState{
		CueListID:       "test-cue-list",
		CurrentCueIndex: &cueIndex,
		IsPlaying:       true,
		IsFading:        true,
		FadeProgress:    50.0,
		LastUpdated:     time.Now(),
	}

	// Trigger emit
	service.emitUpdate("test-cue-list")

	// Verify both callbacks were called
	if !localCallbackCalled {
		t.Error("Expected local callback to be called")
	}
	if !globalCallbackCalled {
		t.Error("Expected global callback to be called")
	}

	// Verify local status
	if localStatus == nil || localStatus.CueListID != "test-cue-list" {
		t.Error("Local status should contain correct cue list ID")
	}

	// Verify global status shows playing
	if globalStatus == nil || !globalStatus.IsPlaying {
		t.Error("Global status should show isPlaying true")
	}
}

func TestEmitUpdate_GlobalStatusNotPlaying(t *testing.T) {
	service := &Service{
		states:              make(map[string]*PlaybackState),
		fadeProgressTickers: make(map[string]*time.Ticker),
		followTimers:        make(map[string]*time.Timer),
		fadeCompleteTimers:  make(map[string]*time.Timer),
	}

	var globalStatus *GlobalPlaybackStatus

	service.SetGlobalUpdateCallback(func(status *GlobalPlaybackStatus) {
		globalStatus = status
	})

	// Set up a non-playing state
	cueIndex := 0
	service.states["test-cue-list"] = &PlaybackState{
		CueListID:       "test-cue-list",
		CurrentCueIndex: &cueIndex,
		IsPlaying:       false,
		IsFading:        false,
		FadeProgress:    0,
		LastUpdated:     time.Now(),
	}

	// Trigger emit
	service.emitUpdate("test-cue-list")

	// Verify global status shows not playing
	if globalStatus == nil {
		t.Fatal("Expected global callback to be called")
	}
	if globalStatus.IsPlaying {
		t.Error("Expected IsPlaying to be false")
	}
	if globalStatus.IsFading {
		t.Error("Expected IsFading to be false")
	}
	if globalStatus.CueListID != nil {
		t.Error("Expected CueListID to be nil when not playing")
	}
}

func TestGetPlaybackState_NilCurrentCue(t *testing.T) {
	service := &Service{
		states:              make(map[string]*PlaybackState),
		fadeProgressTickers: make(map[string]*time.Ticker),
		followTimers:        make(map[string]*time.Timer),
		fadeCompleteTimers:  make(map[string]*time.Timer),
	}

	// State without CurrentCue
	service.states["test-cue-list"] = &PlaybackState{
		CueListID:       "test-cue-list",
		CurrentCueIndex: nil,
		IsPlaying:       false,
		IsFading:        false,
		CurrentCue:      nil,
		FadeProgress:    0,
		StartTime:       nil,
		LastUpdated:     time.Now(),
	}

	state := service.GetPlaybackState("test-cue-list")

	if state == nil {
		t.Fatal("Expected non-nil state")
	}
	if state.CurrentCue != nil {
		t.Error("Expected CurrentCue to be nil")
	}
	if state.CurrentCueIndex != nil {
		t.Error("Expected CurrentCueIndex to be nil")
	}
	if state.StartTime != nil {
		t.Error("Expected StartTime to be nil")
	}
}

func TestStopCueList_NonExistent(t *testing.T) {
	service := &Service{
		states:              make(map[string]*PlaybackState),
		fadeProgressTickers: make(map[string]*time.Ticker),
		followTimers:        make(map[string]*time.Timer),
		fadeCompleteTimers:  make(map[string]*time.Timer),
	}

	// Calling StopCueList on a non-existent cue list should not panic
	service.StopCueList("non-existent-cue-list")

	// State should still be nil/empty
	state := service.GetPlaybackState("non-existent-cue-list")
	if state != nil {
		t.Error("Expected nil state for non-existent cue list after stop")
	}
}
