package preview

import (
	"testing"
)

func TestRandomString(t *testing.T) {
	// Test that it returns string of correct length
	lengths := []int{1, 5, 9, 10, 20}
	for _, length := range lengths {
		result := randomString(length)
		if len(result) != length {
			t.Errorf("randomString(%d) returned string of length %d", length, len(result))
		}
	}

	// Test that it only contains valid characters
	const validChars = "abcdefghijklmnopqrstuvwxyz0123456789"
	result := randomString(100)
	for _, c := range result {
		found := false
		for _, valid := range validChars {
			if c == valid {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("randomString returned invalid character: %c", c)
		}
	}

	// Test that zero length returns empty string
	result = randomString(0)
	if result != "" {
		t.Errorf("randomString(0) should return empty string, got '%s'", result)
	}
}

func TestNewService(t *testing.T) {
	service := NewService(nil, nil, nil)
	if service == nil {
		t.Fatal("NewService() returned nil")
	}
	if service.sessions == nil {
		t.Error("sessions map should be initialized")
	}
	if service.sessionTimers == nil {
		t.Error("sessionTimers map should be initialized")
	}
}

func TestGetSession_NonExistent(t *testing.T) {
	service := NewService(nil, nil, nil)
	session := service.GetSession("non-existent-id")
	if session != nil {
		t.Error("GetSession() should return nil for non-existent session")
	}
}

func TestGetProjectSession_NonExistent(t *testing.T) {
	service := NewService(nil, nil, nil)
	session := service.GetProjectSession("non-existent-project")
	if session != nil {
		t.Error("GetProjectSession() should return nil for non-existent project")
	}
}

func TestGetDMXOutput_NonExistent(t *testing.T) {
	service := NewService(nil, nil, nil)
	output := service.GetDMXOutput("non-existent-session")
	if output != nil {
		t.Error("GetDMXOutput() should return nil for non-existent session")
	}
}

func TestSetSessionUpdateCallback(t *testing.T) {
	service := NewService(nil, nil, nil)

	callback := func(session *Session, dmxOutput []DMXOutput) {
		// Callback function for testing
	}

	service.SetSessionUpdateCallback(callback)

	// Verify callback is set (indirectly through the test)
	if service.onSessionUpdate == nil {
		t.Error("SetSessionUpdateCallback did not set the callback")
	}
}

func TestSession_Structure(t *testing.T) {
	userID := "user-123"
	session := &Session{
		ID:               "session-123",
		ProjectID:        "project-456",
		UserID:           &userID,
		IsActive:         true,
		ChannelOverrides: make(map[string]int),
	}

	if session.ID != "session-123" {
		t.Errorf("Expected ID 'session-123', got '%s'", session.ID)
	}
	if session.ProjectID != "project-456" {
		t.Errorf("Expected ProjectID 'project-456', got '%s'", session.ProjectID)
	}
	if session.UserID == nil || *session.UserID != "user-123" {
		t.Error("UserID mismatch")
	}
	if !session.IsActive {
		t.Error("Expected IsActive to be true")
	}
}

func TestDMXOutput_Structure(t *testing.T) {
	output := DMXOutput{
		Universe: 1,
		Channels: []int{255, 128, 64, 0},
	}

	if output.Universe != 1 {
		t.Errorf("Expected Universe 1, got %d", output.Universe)
	}
	if len(output.Channels) != 4 {
		t.Errorf("Expected 4 channels, got %d", len(output.Channels))
	}
	if output.Channels[0] != 255 {
		t.Errorf("Expected channel 0 to be 255, got %d", output.Channels[0])
	}
}

func TestService_GetUniverseChannelsLocked_NoSession(t *testing.T) {
	service := NewService(nil, nil, nil)

	// With no session, should return 512 zeros
	channels := service.getUniverseChannelsLocked(1, "")
	if len(channels) != 512 {
		t.Errorf("Expected 512 channels, got %d", len(channels))
	}

	// Verify all zeros
	for i, v := range channels {
		if v != 0 {
			t.Errorf("Expected channel %d to be 0, got %d", i, v)
			break
		}
	}
}

func TestService_GetCurrentDMXOutputLocked_NoSession(t *testing.T) {
	service := NewService(nil, nil, nil)

	output := service.getCurrentDMXOutputLocked("non-existent")
	if output != nil {
		t.Error("Expected nil output for non-existent session")
	}
}

func TestService_SessionWithOverrides(t *testing.T) {
	service := NewService(nil, nil, nil)

	// Manually create a session for testing
	session := &Session{
		ID:               "test-session",
		ProjectID:        "test-project",
		IsActive:         true,
		ChannelOverrides: make(map[string]int),
	}

	// Add some channel overrides
	session.ChannelOverrides["1:10"] = 255
	session.ChannelOverrides["1:20"] = 128
	session.ChannelOverrides["2:5"] = 64

	service.sessions = map[string]*Session{
		"test-session": session,
	}

	// Test getting the session
	retrieved := service.GetSession("test-session")
	if retrieved == nil {
		t.Fatal("GetSession() returned nil for existing session")
	}
	if retrieved.ID != "test-session" {
		t.Errorf("Expected ID 'test-session', got '%s'", retrieved.ID)
	}

	// Test getting by project
	byProject := service.GetProjectSession("test-project")
	if byProject == nil {
		t.Fatal("GetProjectSession() returned nil for existing project")
	}
	if byProject.ProjectID != "test-project" {
		t.Errorf("Expected ProjectID 'test-project', got '%s'", byProject.ProjectID)
	}

	// Test getting DMX output
	output := service.GetDMXOutput("test-session")
	if output == nil {
		t.Fatal("GetDMXOutput() returned nil")
	}
	// Should have universes 1 and 2 (from the overrides)
	if len(output) != 2 {
		t.Errorf("Expected 2 universes in output, got %d", len(output))
	}
}

func TestService_InactiveSessionNotReturnedByProjectSession(t *testing.T) {
	service := NewService(nil, nil, nil)

	// Create an inactive session
	session := &Session{
		ID:               "inactive-session",
		ProjectID:        "test-project",
		IsActive:         false, // Inactive
		ChannelOverrides: make(map[string]int),
	}

	service.sessions = map[string]*Session{
		"inactive-session": session,
	}

	// GetProjectSession should return nil for inactive sessions
	result := service.GetProjectSession("test-project")
	if result != nil {
		t.Error("GetProjectSession() should return nil for inactive session")
	}
}

func TestService_GetUniverseChannelsLocked_WithOverrides(t *testing.T) {
	service := NewService(nil, nil, nil)

	// Create a session with overrides
	session := &Session{
		ID:               "test-session",
		ProjectID:        "test-project",
		IsActive:         true,
		ChannelOverrides: map[string]int{
			"1:1":   255, // Channel 1
			"1:100": 128, // Channel 100
			"1:512": 64,  // Channel 512
		},
	}
	service.sessions["test-session"] = session

	channels := service.getUniverseChannelsLocked(1, "test-session")

	if len(channels) != 512 {
		t.Fatalf("Expected 512 channels, got %d", len(channels))
	}

	// Check overrides are applied (0-indexed in result, 1-indexed in override keys)
	if channels[0] != 255 {
		t.Errorf("Expected channel 1 (index 0) to be 255, got %d", channels[0])
	}
	if channels[99] != 128 {
		t.Errorf("Expected channel 100 (index 99) to be 128, got %d", channels[99])
	}
	if channels[511] != 64 {
		t.Errorf("Expected channel 512 (index 511) to be 64, got %d", channels[511])
	}

	// Other channels should be 0
	if channels[50] != 0 {
		t.Errorf("Expected channel 51 (index 50) to be 0, got %d", channels[50])
	}
}

func TestService_CancelExistingProjectSessionsLocked_NoSessions(t *testing.T) {
	service := NewService(nil, nil, nil)

	// Should not panic with no sessions
	service.cancelExistingProjectSessionsLocked("non-existent-project")
}
