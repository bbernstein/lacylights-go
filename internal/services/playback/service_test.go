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
	}

	status := service.GetFormattedStatus("nonexistent-cue-list")

	if status.CueListID != "nonexistent-cue-list" {
		t.Errorf("Expected CueListID 'nonexistent-cue-list', got %s", status.CueListID)
	}
	if status.IsPlaying {
		t.Error("Expected IsPlaying to be false for nonexistent cue list")
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
