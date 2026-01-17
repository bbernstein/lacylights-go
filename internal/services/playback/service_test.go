package playback

import (
	"context"
	"testing"
	"time"

	"github.com/bbernstein/lacylights-go/internal/database/models"
	"github.com/bbernstein/lacylights-go/internal/services/modulator"
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
	projectID := "test-project"
	cueListName := "Test Cue List"
	cueCount := 1
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
	service.StartCue(cueListID, projectID, cueListName, cueCount, cueIndex, cue)

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
	projectID := "test-project"
	cueListName := "Test Cue List"
	cueCount := 1
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
	service.StartCue(cueListID, projectID, cueListName, cueCount, cueIndex, cue)

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
	projectID := "test-project"
	cueListName := "Test Cue List"
	cueCount := 1
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
	service.StartCue(cueListID, projectID, cueListName, cueCount, cueIndex, cue)

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
	projectID := "test-project"
	cueListName := "Test Cue List"
	cueCount := 1
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
	service.StartCue(cueListID, projectID, cueListName, cueCount, cueIndex, cue)

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

func TestPauseCueList(t *testing.T) {
	service := &Service{
		states:              make(map[string]*PlaybackState),
		fadeProgressTickers: make(map[string]*time.Ticker),
		followTimers:        make(map[string]*time.Timer),
		fadeCompleteTimers:  make(map[string]*time.Timer),
	}

	// Set up a playing state
	cueIndex := 5
	service.states["test-cue-list"] = &PlaybackState{
		CueListID:       "test-cue-list",
		CurrentCueIndex: &cueIndex,
		IsPlaying:       true,
		IsPaused:        false,
		IsFading:        true,
		FadeProgress:    50.0,
		LastUpdated:     time.Now(),
	}

	// Create timers to test cleanup
	ticker := time.NewTicker(100 * time.Millisecond)
	service.fadeProgressTickers["test-cue-list"] = ticker
	timer := time.NewTimer(10 * time.Second)
	service.followTimers["test-cue-list"] = timer
	fadeTimer := time.NewTimer(10 * time.Second)
	service.fadeCompleteTimers["test-cue-list"] = fadeTimer

	// Pause the cue list
	service.PauseCueList("test-cue-list")

	// Verify state was updated
	state := service.GetPlaybackState("test-cue-list")
	if state.IsPlaying {
		t.Error("Expected IsPlaying to be false after pause")
	}
	if !state.IsPaused {
		t.Error("Expected IsPaused to be true after pause")
	}
	if state.IsFading {
		t.Error("Expected IsFading to be false after pause")
	}
	if state.FadeProgress != 0 {
		t.Errorf("Expected FadeProgress 0 after pause, got %f", state.FadeProgress)
	}

	// Verify CurrentCueIndex is preserved
	if state.CurrentCueIndex == nil || *state.CurrentCueIndex != 5 {
		t.Errorf("Expected CurrentCueIndex 5 to be preserved, got %v", state.CurrentCueIndex)
	}

	// Verify timers were cleaned up
	if _, exists := service.fadeProgressTickers["test-cue-list"]; exists {
		t.Error("Expected fade progress ticker to be removed after pause")
	}
	if _, exists := service.followTimers["test-cue-list"]; exists {
		t.Error("Expected follow timer to be removed after pause")
	}
	if _, exists := service.fadeCompleteTimers["test-cue-list"]; exists {
		t.Error("Expected fade complete timer to be removed after pause")
	}
}

func TestPauseCueList_NotPlaying(t *testing.T) {
	service := &Service{
		states:              make(map[string]*PlaybackState),
		fadeProgressTickers: make(map[string]*time.Ticker),
		followTimers:        make(map[string]*time.Timer),
		fadeCompleteTimers:  make(map[string]*time.Timer),
	}

	// Set up a stopped state
	cueIndex := 3
	service.states["test-cue-list"] = &PlaybackState{
		CueListID:       "test-cue-list",
		CurrentCueIndex: &cueIndex,
		IsPlaying:       false,
		IsPaused:        false,
		LastUpdated:     time.Now(),
	}

	// Pause should do nothing since not playing
	service.PauseCueList("test-cue-list")

	// Verify state unchanged
	state := service.GetPlaybackState("test-cue-list")
	if state.IsPlaying {
		t.Error("Expected IsPlaying to remain false")
	}
	if state.IsPaused {
		t.Error("Expected IsPaused to remain false since was not playing")
	}
}

func TestPauseCueList_NonExistent(t *testing.T) {
	service := &Service{
		states:              make(map[string]*PlaybackState),
		fadeProgressTickers: make(map[string]*time.Ticker),
		followTimers:        make(map[string]*time.Timer),
		fadeCompleteTimers:  make(map[string]*time.Timer),
	}

	// Pause on non-existent should not panic
	service.PauseCueList("non-existent")

	// State should still be nil
	state := service.GetPlaybackState("non-existent")
	if state != nil {
		t.Error("Expected nil state for non-existent cue list")
	}
}

func TestPausePlayingCueLists(t *testing.T) {
	service := &Service{
		states:              make(map[string]*PlaybackState),
		fadeProgressTickers: make(map[string]*time.Ticker),
		followTimers:        make(map[string]*time.Timer),
		fadeCompleteTimers:  make(map[string]*time.Timer),
	}

	// Set up multiple states: some playing, some not
	cueIndex := 0
	service.states["playing-1"] = &PlaybackState{
		CueListID:       "playing-1",
		CurrentCueIndex: &cueIndex,
		IsPlaying:       true,
		IsPaused:        false,
		LastUpdated:     time.Now(),
	}
	service.states["playing-2"] = &PlaybackState{
		CueListID:       "playing-2",
		CurrentCueIndex: &cueIndex,
		IsPlaying:       true,
		IsPaused:        false,
		LastUpdated:     time.Now(),
	}
	service.states["stopped-1"] = &PlaybackState{
		CueListID:       "stopped-1",
		CurrentCueIndex: &cueIndex,
		IsPlaying:       false,
		IsPaused:        false,
		LastUpdated:     time.Now(),
	}

	// Pause all playing cue lists
	service.PausePlayingCueLists()

	// Verify playing cue lists are paused
	state1 := service.GetPlaybackState("playing-1")
	if state1.IsPlaying || !state1.IsPaused {
		t.Error("Expected playing-1 to be paused")
	}
	state2 := service.GetPlaybackState("playing-2")
	if state2.IsPlaying || !state2.IsPaused {
		t.Error("Expected playing-2 to be paused")
	}

	// Verify stopped cue list is unchanged
	stateStopped := service.GetPlaybackState("stopped-1")
	if stateStopped.IsPaused {
		t.Error("Expected stopped-1 to remain not paused")
	}
}

func TestStopCueList_ClearsPausedState(t *testing.T) {
	service := &Service{
		states:              make(map[string]*PlaybackState),
		fadeProgressTickers: make(map[string]*time.Ticker),
		followTimers:        make(map[string]*time.Timer),
		fadeCompleteTimers:  make(map[string]*time.Timer),
	}

	// Set up a paused state
	cueIndex := 3
	service.states["test-cue-list"] = &PlaybackState{
		CueListID:       "test-cue-list",
		CurrentCueIndex: &cueIndex,
		IsPlaying:       false,
		IsPaused:        true,
		LastUpdated:     time.Now(),
	}

	// Stop the cue list
	service.StopCueList("test-cue-list")

	// Verify both playing and paused are false
	state := service.GetPlaybackState("test-cue-list")
	if state.IsPlaying {
		t.Error("Expected IsPlaying to be false after stop")
	}
	if state.IsPaused {
		t.Error("Expected IsPaused to be false after stop")
	}
}

func TestGetFormattedStatus_PausedState(t *testing.T) {
	service := &Service{
		states:              make(map[string]*PlaybackState),
		fadeProgressTickers: make(map[string]*time.Ticker),
		followTimers:        make(map[string]*time.Timer),
		fadeCompleteTimers:  make(map[string]*time.Timer),
	}

	// Set up a paused state
	cueIndex := 2
	service.states["test-cue-list"] = &PlaybackState{
		CueListID:       "test-cue-list",
		CurrentCueIndex: &cueIndex,
		IsPlaying:       false,
		IsPaused:        true,
		IsFading:        false,
		LastUpdated:     time.Now(),
	}

	status := service.GetFormattedStatus("test-cue-list")

	if status.IsPlaying {
		t.Error("Expected IsPlaying to be false in formatted status")
	}
	if !status.IsPaused {
		t.Error("Expected IsPaused to be true in formatted status")
	}
	if status.CurrentCueIndex == nil || *status.CurrentCueIndex != 2 {
		t.Errorf("Expected CurrentCueIndex 2, got %v", status.CurrentCueIndex)
	}
}

func TestPlaybackState_IsPausedField(t *testing.T) {
	now := time.Now()
	cueIndex := 0
	state := &PlaybackState{
		CueListID:       "cue-list-1",
		CurrentCueIndex: &cueIndex,
		IsPlaying:       false,
		IsPaused:        true,
		IsFading:        false,
		FadeProgress:    0,
		StartTime:       &now,
		LastUpdated:     now,
	}

	if state.IsPlaying {
		t.Error("Expected IsPlaying to be false")
	}
	if !state.IsPaused {
		t.Error("Expected IsPaused to be true")
	}
}

func TestCueListPlaybackStatus_IsPausedField(t *testing.T) {
	cueIndex := 1
	status := &CueListPlaybackStatus{
		CueListID:       "cue-list-1",
		CurrentCueIndex: &cueIndex,
		IsPlaying:       false,
		IsPaused:        true,
		IsFading:        false,
		FadeProgress:    0,
		LastUpdated:     "2025-11-26T10:00:00Z",
	}

	if status.IsPlaying {
		t.Error("Expected IsPlaying to be false")
	}
	if !status.IsPaused {
		t.Error("Expected IsPaused to be true")
	}
}

func TestGlobalPlaybackStatus_IsPausedField(t *testing.T) {
	cueListID := "cue-list-1"
	cueListName := "Test List"
	status := &GlobalPlaybackStatus{
		IsPlaying:   false,
		IsPaused:    true,
		IsFading:    false,
		CueListID:   &cueListID,
		CueListName: &cueListName,
		LastUpdated: "2025-11-26T10:00:00Z",
	}

	if status.IsPlaying {
		t.Error("Expected IsPlaying to be false")
	}
	if !status.IsPaused {
		t.Error("Expected IsPaused to be true")
	}
}

func TestResumeCueList_NotFound(t *testing.T) {
	service := &Service{
		states:              make(map[string]*PlaybackState),
		fadeProgressTickers: make(map[string]*time.Ticker),
		followTimers:        make(map[string]*time.Timer),
		fadeCompleteTimers:  make(map[string]*time.Timer),
	}

	ctx := context.Background()
	err := service.ResumeCueList(ctx, "non-existent-cue-list")

	if err == nil {
		t.Error("Expected error for non-existent cue list")
	}
	if err.Error() != "cue list not found" {
		t.Errorf("Expected 'cue list not found' error, got: %v", err)
	}
}

func TestResumeCueList_NotPaused(t *testing.T) {
	service := &Service{
		states:              make(map[string]*PlaybackState),
		fadeProgressTickers: make(map[string]*time.Ticker),
		followTimers:        make(map[string]*time.Timer),
		fadeCompleteTimers:  make(map[string]*time.Timer),
	}

	// Set up a playing (not paused) state
	cueIndex := 0
	service.states["test-cue-list"] = &PlaybackState{
		CueListID:       "test-cue-list",
		CurrentCueIndex: &cueIndex,
		IsPlaying:       true,
		IsPaused:        false,
		LastUpdated:     time.Now(),
	}

	ctx := context.Background()
	err := service.ResumeCueList(ctx, "test-cue-list")

	if err == nil {
		t.Error("Expected error for non-paused cue list")
	}
	if err.Error() != "cue list is not paused" {
		t.Errorf("Expected 'cue list is not paused' error, got: %v", err)
	}
}

func TestResumeCueList_NilCurrentCueIndex(t *testing.T) {
	service := &Service{
		states:              make(map[string]*PlaybackState),
		fadeProgressTickers: make(map[string]*time.Ticker),
		followTimers:        make(map[string]*time.Timer),
		fadeCompleteTimers:  make(map[string]*time.Timer),
	}

	// Set up a paused state without a current cue index
	service.states["test-cue-list"] = &PlaybackState{
		CueListID:       "test-cue-list",
		CurrentCueIndex: nil, // No cue index
		IsPlaying:       false,
		IsPaused:        true,
		LastUpdated:     time.Now(),
	}

	ctx := context.Background()
	err := service.ResumeCueList(ctx, "test-cue-list")

	if err == nil {
		t.Error("Expected error for nil current cue index")
	}
	if err.Error() != "no current cue to resume" {
		t.Errorf("Expected 'no current cue to resume' error, got: %v", err)
	}
}

// Tests for findNextNonSkippedCue helper function

func TestFindNextNonSkippedCue_EmptyCues(t *testing.T) {
	cues := []models.Cue{}
	result := findNextNonSkippedCue(cues, 0, false)
	if result != -1 {
		t.Errorf("Expected -1 for empty cues, got %d", result)
	}
}

func TestFindNextNonSkippedCue_AllSkipped(t *testing.T) {
	cues := []models.Cue{
		{ID: "1", Skip: true},
		{ID: "2", Skip: true},
		{ID: "3", Skip: true},
	}

	// Without loop
	result := findNextNonSkippedCue(cues, 0, false)
	if result != -1 {
		t.Errorf("Expected -1 when all cues are skipped (no loop), got %d", result)
	}

	// With loop
	result = findNextNonSkippedCue(cues, 0, true)
	if result != -1 {
		t.Errorf("Expected -1 when all cues are skipped (loop), got %d", result)
	}
}

func TestFindNextNonSkippedCue_NoneSkipped(t *testing.T) {
	cues := []models.Cue{
		{ID: "1", Skip: false},
		{ID: "2", Skip: false},
		{ID: "3", Skip: false},
	}

	result := findNextNonSkippedCue(cues, 0, false)
	if result != 0 {
		t.Errorf("Expected 0 when first cue is not skipped, got %d", result)
	}

	result = findNextNonSkippedCue(cues, 1, false)
	if result != 1 {
		t.Errorf("Expected 1 when starting from index 1, got %d", result)
	}
}

func TestFindNextNonSkippedCue_SkipFirst(t *testing.T) {
	cues := []models.Cue{
		{ID: "1", Skip: true},
		{ID: "2", Skip: false},
		{ID: "3", Skip: false},
	}

	result := findNextNonSkippedCue(cues, 0, false)
	if result != 1 {
		t.Errorf("Expected 1 when first cue is skipped, got %d", result)
	}
}

func TestFindNextNonSkippedCue_SkipMiddle(t *testing.T) {
	cues := []models.Cue{
		{ID: "1", Skip: false},
		{ID: "2", Skip: true},
		{ID: "3", Skip: false},
	}

	result := findNextNonSkippedCue(cues, 1, false)
	if result != 2 {
		t.Errorf("Expected 2 when middle cue is skipped, got %d", result)
	}
}

func TestFindNextNonSkippedCue_SkipEnd_NoLoop(t *testing.T) {
	cues := []models.Cue{
		{ID: "1", Skip: false},
		{ID: "2", Skip: false},
		{ID: "3", Skip: true},
	}

	// Starting from index 2 (skipped), no non-skipped cues after
	result := findNextNonSkippedCue(cues, 2, false)
	if result != -1 {
		t.Errorf("Expected -1 when last cue is skipped (no loop), got %d", result)
	}
}

func TestFindNextNonSkippedCue_SkipEnd_Loop(t *testing.T) {
	cues := []models.Cue{
		{ID: "1", Skip: false},
		{ID: "2", Skip: false},
		{ID: "3", Skip: true},
	}

	// Starting from index 2 (skipped), should loop to find cue at index 0
	result := findNextNonSkippedCue(cues, 2, true)
	if result != 0 {
		t.Errorf("Expected 0 when last cue is skipped (with loop), got %d", result)
	}
}

func TestFindNextNonSkippedCue_WrapAround(t *testing.T) {
	cues := []models.Cue{
		{ID: "1", Skip: false},
		{ID: "2", Skip: true},
		{ID: "3", Skip: true},
	}

	// Starting from index 1, need to wrap around to find cue at 0
	result := findNextNonSkippedCue(cues, 1, true)
	if result != 0 {
		t.Errorf("Expected 0 when wrap around is needed (with loop), got %d", result)
	}

	// Without loop, should return -1
	result = findNextNonSkippedCue(cues, 1, false)
	if result != -1 {
		t.Errorf("Expected -1 when wrap around is needed (no loop), got %d", result)
	}
}

func TestFindNextNonSkippedCue_SingleCue(t *testing.T) {
	// Single non-skipped cue
	cues := []models.Cue{{ID: "1", Skip: false}}
	result := findNextNonSkippedCue(cues, 0, false)
	if result != 0 {
		t.Errorf("Expected 0 for single non-skipped cue, got %d", result)
	}

	// Single skipped cue
	cues = []models.Cue{{ID: "1", Skip: true}}
	result = findNextNonSkippedCue(cues, 0, false)
	if result != -1 {
		t.Errorf("Expected -1 for single skipped cue, got %d", result)
	}
}

// Tests for findPreviousNonSkippedCue helper function

func TestFindPreviousNonSkippedCue_EmptyCues(t *testing.T) {
	cues := []models.Cue{}
	result := findPreviousNonSkippedCue(cues, 0, false)
	if result != -1 {
		t.Errorf("Expected -1 for empty cues, got %d", result)
	}
}

func TestFindPreviousNonSkippedCue_AllSkipped(t *testing.T) {
	cues := []models.Cue{
		{ID: "1", Skip: true},
		{ID: "2", Skip: true},
		{ID: "3", Skip: true},
	}

	// Without loop
	result := findPreviousNonSkippedCue(cues, 2, false)
	if result != -1 {
		t.Errorf("Expected -1 when all cues are skipped (no loop), got %d", result)
	}

	// With loop
	result = findPreviousNonSkippedCue(cues, 2, true)
	if result != -1 {
		t.Errorf("Expected -1 when all cues are skipped (loop), got %d", result)
	}
}

func TestFindPreviousNonSkippedCue_NoneSkipped(t *testing.T) {
	cues := []models.Cue{
		{ID: "1", Skip: false},
		{ID: "2", Skip: false},
		{ID: "3", Skip: false},
	}

	result := findPreviousNonSkippedCue(cues, 2, false)
	if result != 2 {
		t.Errorf("Expected 2 when last cue is not skipped, got %d", result)
	}

	result = findPreviousNonSkippedCue(cues, 1, false)
	if result != 1 {
		t.Errorf("Expected 1 when starting from index 1, got %d", result)
	}
}

func TestFindPreviousNonSkippedCue_SkipLast(t *testing.T) {
	cues := []models.Cue{
		{ID: "1", Skip: false},
		{ID: "2", Skip: false},
		{ID: "3", Skip: true},
	}

	result := findPreviousNonSkippedCue(cues, 2, false)
	if result != 1 {
		t.Errorf("Expected 1 when last cue is skipped, got %d", result)
	}
}

func TestFindPreviousNonSkippedCue_SkipMiddle(t *testing.T) {
	cues := []models.Cue{
		{ID: "1", Skip: false},
		{ID: "2", Skip: true},
		{ID: "3", Skip: false},
	}

	result := findPreviousNonSkippedCue(cues, 1, false)
	if result != 0 {
		t.Errorf("Expected 0 when middle cue is skipped, got %d", result)
	}
}

func TestFindPreviousNonSkippedCue_SkipFirst_NoLoop(t *testing.T) {
	cues := []models.Cue{
		{ID: "1", Skip: true},
		{ID: "2", Skip: false},
		{ID: "3", Skip: false},
	}

	// Starting from index 0 (skipped), no non-skipped cues before
	result := findPreviousNonSkippedCue(cues, 0, false)
	if result != -1 {
		t.Errorf("Expected -1 when first cue is skipped (no loop), got %d", result)
	}
}

func TestFindPreviousNonSkippedCue_SkipFirst_Loop(t *testing.T) {
	cues := []models.Cue{
		{ID: "1", Skip: true},
		{ID: "2", Skip: false},
		{ID: "3", Skip: false},
	}

	// Starting from index 0 (skipped), should loop to find cue at index 2
	result := findPreviousNonSkippedCue(cues, 0, true)
	if result != 2 {
		t.Errorf("Expected 2 when first cue is skipped (with loop), got %d", result)
	}
}

func TestFindPreviousNonSkippedCue_WrapAround(t *testing.T) {
	cues := []models.Cue{
		{ID: "1", Skip: true},
		{ID: "2", Skip: true},
		{ID: "3", Skip: false},
	}

	// Starting from index 1, need to wrap around to find cue at 2
	result := findPreviousNonSkippedCue(cues, 1, true)
	if result != 2 {
		t.Errorf("Expected 2 when wrap around is needed (with loop), got %d", result)
	}

	// Without loop, should return -1
	result = findPreviousNonSkippedCue(cues, 1, false)
	if result != -1 {
		t.Errorf("Expected -1 when wrap around is needed (no loop), got %d", result)
	}
}

func TestFindPreviousNonSkippedCue_SingleCue(t *testing.T) {
	// Single non-skipped cue
	cues := []models.Cue{{ID: "1", Skip: false}}
	result := findPreviousNonSkippedCue(cues, 0, false)
	if result != 0 {
		t.Errorf("Expected 0 for single non-skipped cue, got %d", result)
	}

	// Single skipped cue
	cues = []models.Cue{{ID: "1", Skip: true}}
	result = findPreviousNonSkippedCue(cues, 0, false)
	if result != -1 {
		t.Errorf("Expected -1 for single skipped cue, got %d", result)
	}
}

// Tests for effect conversion and parsing functions

func TestParseEffectPriority(t *testing.T) {
	tests := []struct {
		name         string
		band         string
		sub          int
		wantBand     modulator.PriorityBand
		wantSubPrior int
	}{
		{"BASE band", "BASE", 10, modulator.PriorityBandBase, 10},
		{"USER band", "USER", 20, modulator.PriorityBandUser, 20},
		{"CUE band", "CUE", 30, modulator.PriorityBandCue, 30},
		{"SYSTEM band", "SYSTEM", 40, modulator.PriorityBandSystem, 40},
		{"Unknown band defaults to USER", "UNKNOWN", 50, modulator.PriorityBandUser, 50},
		{"Empty band defaults to USER", "", 0, modulator.PriorityBandUser, 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parseEffectPriority(tt.band, tt.sub)
			if got.Band != tt.wantBand {
				t.Errorf("parseEffectPriority(%q, %d).Band = %v, want %v", tt.band, tt.sub, got.Band, tt.wantBand)
			}
			if got.SubPriority != tt.wantSubPrior {
				t.Errorf("parseEffectPriority(%q, %d).SubPriority = %d, want %d", tt.band, tt.sub, got.SubPriority, tt.wantSubPrior)
			}
		})
	}
}

func TestParseCompositionMode(t *testing.T) {
	tests := []struct {
		name string
		mode string
		want string
	}{
		{"OVERRIDE mode", "OVERRIDE", "OVERRIDE"},
		{"ADDITIVE mode", "ADDITIVE", "ADDITIVE"},
		{"MULTIPLY mode", "MULTIPLY", "MULTIPLY"},
		{"Unknown mode defaults to OVERRIDE", "UNKNOWN", "OVERRIDE"},
		{"Empty mode defaults to OVERRIDE", "", "OVERRIDE"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parseCompositionMode(tt.mode)
			if got.String() != tt.want {
				t.Errorf("parseCompositionMode(%q) = %s, want %s", tt.mode, got.String(), tt.want)
			}
		})
	}
}

func TestParseTransitionBehavior(t *testing.T) {
	tests := []struct {
		name     string
		behavior string
		want     string
	}{
		{"FADE_OUT behavior", "FADE_OUT", "FADE_OUT"},
		{"PERSIST behavior", "PERSIST", "PERSIST"},
		{"SNAP_OFF behavior", "SNAP_OFF", "SNAP_OFF"},
		{"CROSSFADE_PARAMS behavior", "CROSSFADE_PARAMS", "CROSSFADE_PARAMS"},
		{"Unknown behavior defaults to FADE_OUT", "UNKNOWN", "FADE_OUT"},
		{"Empty behavior defaults to FADE_OUT", "", "FADE_OUT"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parseTransitionBehavior(tt.behavior)
			if got.String() != tt.want {
				t.Errorf("parseTransitionBehavior(%q) = %s, want %s", tt.behavior, got.String(), tt.want)
			}
		})
	}
}

func TestSetPreviewService(t *testing.T) {
	service := &Service{
		states:              make(map[string]*PlaybackState),
		fadeProgressTickers: make(map[string]*time.Ticker),
		followTimers:        make(map[string]*time.Timer),
		fadeCompleteTimers:  make(map[string]*time.Timer),
	}

	// Initially nil
	if service.previewService != nil {
		t.Error("Expected previewService to be nil initially")
	}

	// Set a nil preview service (valid use case for disabling)
	service.SetPreviewService(nil)
	if service.previewService != nil {
		t.Error("Expected previewService to remain nil after setting nil")
	}
}

func TestConvertToModulatorEffect_BasicFields(t *testing.T) {
	service := &Service{
		states:              make(map[string]*PlaybackState),
		fadeProgressTickers: make(map[string]*time.Ticker),
		followTimers:        make(map[string]*time.Timer),
		fadeCompleteTimers:  make(map[string]*time.Timer),
	}

	// Create a basic effect
	dbEffect := &models.Effect{
		ID:              "effect-1",
		Name:            "Test Effect",
		ProjectID:       "project-1",
		EffectType:      "WAVEFORM",
		PriorityBand:    "USER",
		PrioritySub:     10,
		CompositionMode: "ADDITIVE",
		OnCueChange:     "FADE_OUT",
		Frequency:       2.0,
		Amplitude:       50.0,
		Offset:          25.0,
		PhaseOffset:     0.5,
	}

	result := service.convertToModulatorEffect(dbEffect)

	if result.ID != "effect-1" {
		t.Errorf("Expected ID 'effect-1', got %s", result.ID)
	}
	if result.Name != "Test Effect" {
		t.Errorf("Expected Name 'Test Effect', got %s", result.Name)
	}
	if result.ProjectID != "project-1" {
		t.Errorf("Expected ProjectID 'project-1', got %s", result.ProjectID)
	}
	if result.Frequency != 2.0 {
		t.Errorf("Expected Frequency 2.0, got %f", result.Frequency)
	}
	if result.Amplitude != 50.0 {
		t.Errorf("Expected Amplitude 50.0, got %f", result.Amplitude)
	}
	if result.Offset != 25.0 {
		t.Errorf("Expected Offset 25.0, got %f", result.Offset)
	}
	if result.PhaseOffset != 0.5 {
		t.Errorf("Expected PhaseOffset 0.5, got %f", result.PhaseOffset)
	}
}

func TestConvertToModulatorEffect_OptionalFields(t *testing.T) {
	service := &Service{
		states:              make(map[string]*PlaybackState),
		fadeProgressTickers: make(map[string]*time.Ticker),
		followTimers:        make(map[string]*time.Timer),
		fadeCompleteTimers:  make(map[string]*time.Timer),
	}

	description := "A test effect"
	fadeDuration := 3.0
	waveform := "SINE"
	masterValue := 80.0

	dbEffect := &models.Effect{
		ID:              "effect-2",
		Name:            "Effect with optional fields",
		ProjectID:       "project-1",
		EffectType:      "WAVEFORM",
		PriorityBand:    "CUE",
		PrioritySub:     50,
		CompositionMode: "OVERRIDE",
		OnCueChange:     "PERSIST",
		Frequency:       1.0,
		Amplitude:       100.0,
		Offset:          0.0,
		PhaseOffset:     0.0,
		Description:     &description,
		FadeDuration:    &fadeDuration,
		Waveform:        &waveform,
		MasterValue:     &masterValue,
	}

	result := service.convertToModulatorEffect(dbEffect)

	if result.Description != "A test effect" {
		t.Errorf("Expected Description 'A test effect', got %s", result.Description)
	}
	if result.FadeDuration == nil || *result.FadeDuration != 3.0 {
		t.Errorf("Expected FadeDuration 3.0, got %v", result.FadeDuration)
	}
	if result.MasterValue != 80.0 {
		t.Errorf("Expected MasterValue 80.0, got %f", result.MasterValue)
	}
}

func TestConvertToModulatorEffect_WithFixtures(t *testing.T) {
	service := &Service{
		states:              make(map[string]*PlaybackState),
		fadeProgressTickers: make(map[string]*time.Ticker),
		followTimers:        make(map[string]*time.Timer),
		fadeCompleteTimers:  make(map[string]*time.Timer),
	}

	channelOffset := 0
	amplitudeScale := 0.5
	frequencyScale := 2.0
	fixturePhaseOffset := 0.25

	dbEffect := &models.Effect{
		ID:              "effect-3",
		Name:            "Effect with fixtures",
		ProjectID:       "project-1",
		EffectType:      "WAVEFORM",
		PriorityBand:    "USER",
		PrioritySub:     0,
		CompositionMode: "ADDITIVE",
		OnCueChange:     "FADE_OUT",
		Frequency:       1.0,
		Amplitude:       100.0,
		Offset:          50.0,
		PhaseOffset:     0.0,
		Fixtures: []models.EffectFixture{
			{
				ID:             "ef-1",
				EffectID:       "effect-3",
				FixtureID:      "fixture-1",
				PhaseOffset:    &fixturePhaseOffset,
				AmplitudeScale: &amplitudeScale,
				Fixture: &models.FixtureInstance{
					ID:           "fixture-1",
					Universe:     1,
					StartChannel: 10,
				},
				Channels: []models.EffectChannel{
					{
						ID:              "ec-1",
						EffectFixtureID: "ef-1",
						ChannelOffset:   &channelOffset,
						AmplitudeScale:  nil, // Will use fixture-level scale
						FrequencyScale:  &frequencyScale,
					},
				},
			},
		},
	}

	result := service.convertToModulatorEffect(dbEffect)

	if len(result.TargetChannels) != 1 {
		t.Errorf("Expected 1 target channel, got %d", len(result.TargetChannels))
	}

	if len(result.TargetChannels) > 0 {
		tc := result.TargetChannels[0]
		if tc.Universe != 1 {
			t.Errorf("Expected Universe 1, got %d", tc.Universe)
		}
		if tc.Channel != 10 {
			t.Errorf("Expected Channel 10, got %d", tc.Channel)
		}
		if tc.PhaseOffset == nil || *tc.PhaseOffset != 0.25 {
			t.Errorf("Expected PhaseOffset 0.25, got %v", tc.PhaseOffset)
		}
		if tc.AmplitudeScale == nil || *tc.AmplitudeScale != 0.5 {
			t.Errorf("Expected AmplitudeScale 0.5, got %v", tc.AmplitudeScale)
		}
		if tc.FrequencyScale == nil || *tc.FrequencyScale != 2.0 {
			t.Errorf("Expected FrequencyScale 2.0, got %v", tc.FrequencyScale)
		}
	}
}

func TestConvertToModulatorEffect_ChannelMinMaxValues(t *testing.T) {
	service := &Service{
		states:              make(map[string]*PlaybackState),
		fadeProgressTickers: make(map[string]*time.Ticker),
		followTimers:        make(map[string]*time.Timer),
		fadeCompleteTimers:  make(map[string]*time.Timer),
	}

	channelOffset := 0
	minValue := 50.0
	maxValue := 200.0

	dbEffect := &models.Effect{
		ID:              "effect-4",
		Name:            "Effect with min/max",
		ProjectID:       "project-1",
		EffectType:      "WAVEFORM",
		PriorityBand:    "USER",
		PrioritySub:     0,
		CompositionMode: "OVERRIDE",
		OnCueChange:     "FADE_OUT",
		Frequency:       1.0,
		Amplitude:       100.0,
		Offset:          50.0,
		PhaseOffset:     0.0,
		Fixtures: []models.EffectFixture{
			{
				ID:        "ef-1",
				EffectID:  "effect-4",
				FixtureID: "fixture-1",
				Fixture: &models.FixtureInstance{
					ID:           "fixture-1",
					Universe:     1,
					StartChannel: 1,
				},
				Channels: []models.EffectChannel{
					{
						ID:              "ec-1",
						EffectFixtureID: "ef-1",
						ChannelOffset:   &channelOffset,
						MinValue:        &minValue,
						MaxValue:        &maxValue,
					},
				},
			},
		},
	}

	result := service.convertToModulatorEffect(dbEffect)

	if len(result.TargetChannels) != 1 {
		t.Fatalf("Expected 1 target channel, got %d", len(result.TargetChannels))
	}

	tc := result.TargetChannels[0]

	// When minValue and maxValue are set, offset and amplitude should be calculated
	expectedOffset := (minValue + maxValue) / 2    // 125
	expectedAmplitude := (maxValue - minValue) / 2 // 75

	if tc.Offset == nil || *tc.Offset != expectedOffset {
		t.Errorf("Expected Offset %f, got %v", expectedOffset, tc.Offset)
	}
	if tc.Amplitude == nil || *tc.Amplitude != expectedAmplitude {
		t.Errorf("Expected Amplitude %f, got %v", expectedAmplitude, tc.Amplitude)
	}
	// AmplitudeScale should be nil when min/max are used
	if tc.AmplitudeScale != nil {
		t.Errorf("Expected AmplitudeScale nil when min/max used, got %v", tc.AmplitudeScale)
	}
}

func TestConvertToModulatorEffect_ChannelAmplitudeScalePrecedence(t *testing.T) {
	service := &Service{
		states:              make(map[string]*PlaybackState),
		fadeProgressTickers: make(map[string]*time.Ticker),
		followTimers:        make(map[string]*time.Timer),
		fadeCompleteTimers:  make(map[string]*time.Timer),
	}

	channelOffset := 0
	fixtureAmplitudeScale := 0.5
	channelAmplitudeScale := 0.8 // Should take precedence

	dbEffect := &models.Effect{
		ID:              "effect-5",
		Name:            "Effect with amplitude scales",
		ProjectID:       "project-1",
		EffectType:      "WAVEFORM",
		PriorityBand:    "USER",
		PrioritySub:     0,
		CompositionMode: "OVERRIDE",
		OnCueChange:     "FADE_OUT",
		Frequency:       1.0,
		Amplitude:       100.0,
		Offset:          50.0,
		PhaseOffset:     0.0,
		Fixtures: []models.EffectFixture{
			{
				ID:             "ef-1",
				EffectID:       "effect-5",
				FixtureID:      "fixture-1",
				AmplitudeScale: &fixtureAmplitudeScale,
				Fixture: &models.FixtureInstance{
					ID:           "fixture-1",
					Universe:     1,
					StartChannel: 1,
				},
				Channels: []models.EffectChannel{
					{
						ID:              "ec-1",
						EffectFixtureID: "ef-1",
						ChannelOffset:   &channelOffset,
						AmplitudeScale:  &channelAmplitudeScale, // Takes precedence
					},
				},
			},
		},
	}

	result := service.convertToModulatorEffect(dbEffect)

	if len(result.TargetChannels) != 1 {
		t.Fatalf("Expected 1 target channel, got %d", len(result.TargetChannels))
	}

	tc := result.TargetChannels[0]

	// Channel-level amplitude scale should take precedence over fixture-level
	if tc.AmplitudeScale == nil || *tc.AmplitudeScale != 0.8 {
		t.Errorf("Expected channel AmplitudeScale 0.8 to take precedence, got %v", tc.AmplitudeScale)
	}
}

func TestConvertToModulatorEffect_NilFixture(t *testing.T) {
	service := &Service{
		states:              make(map[string]*PlaybackState),
		fadeProgressTickers: make(map[string]*time.Ticker),
		followTimers:        make(map[string]*time.Timer),
		fadeCompleteTimers:  make(map[string]*time.Timer),
	}

	// Effect with fixture entry but nil Fixture pointer
	dbEffect := &models.Effect{
		ID:              "effect-6",
		Name:            "Effect with nil fixture",
		ProjectID:       "project-1",
		EffectType:      "WAVEFORM",
		PriorityBand:    "USER",
		PrioritySub:     0,
		CompositionMode: "OVERRIDE",
		OnCueChange:     "FADE_OUT",
		Frequency:       1.0,
		Amplitude:       100.0,
		Offset:          50.0,
		PhaseOffset:     0.0,
		Fixtures: []models.EffectFixture{
			{
				ID:        "ef-1",
				EffectID:  "effect-6",
				FixtureID: "fixture-1",
				Fixture:   nil, // Nil fixture should be skipped
			},
		},
	}

	result := service.convertToModulatorEffect(dbEffect)

	// Should have no target channels since fixture is nil
	if len(result.TargetChannels) != 0 {
		t.Errorf("Expected 0 target channels when fixture is nil, got %d", len(result.TargetChannels))
	}
}
