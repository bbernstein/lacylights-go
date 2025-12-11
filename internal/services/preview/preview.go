// Package preview provides preview session management.
package preview

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/bbernstein/lacylights-go/internal/database/repositories"
	"github.com/bbernstein/lacylights-go/internal/services/dmx"
)

// Session represents an active preview session.
type Session struct {
	ID               string
	ProjectID        string
	UserID           *string
	IsActive         bool
	CreatedAt        time.Time
	ChannelOverrides map[string]int // Key: "universe:channel", Value: 0-255
}

// DMXOutput represents DMX output for a universe.
type DMXOutput struct {
	Universe int
	Channels []int
}

// Service handles preview session operations.
type Service struct {
	mu              sync.RWMutex
	sessions        map[string]*Session
	sessionTimeout  time.Duration
	sessionTimers   map[string]*time.Timer
	fixtureRepo     *repositories.FixtureRepository
	sceneRepo       *repositories.SceneRepository
	dmxService      *dmx.Service
	onSessionUpdate func(session *Session, dmxOutput []DMXOutput) // Callback for subscription updates
}

// NewService creates a new preview service.
func NewService(
	fixtureRepo *repositories.FixtureRepository,
	sceneRepo *repositories.SceneRepository,
	dmxService *dmx.Service,
) *Service {
	return &Service{
		sessions:       make(map[string]*Session),
		sessionTimeout: 30 * time.Minute,
		sessionTimers:  make(map[string]*time.Timer),
		fixtureRepo:    fixtureRepo,
		sceneRepo:      sceneRepo,
		dmxService:     dmxService,
	}
}

// SetSessionUpdateCallback sets the callback for session updates (for subscriptions).
func (s *Service) SetSessionUpdateCallback(callback func(session *Session, dmxOutput []DMXOutput)) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.onSessionUpdate = callback
}

// StartSession starts a new preview session for a project.
func (s *Service) StartSession(ctx context.Context, projectID string, userID *string) (*Session, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Cancel any existing sessions for this project
	s.cancelExistingProjectSessionsLocked(projectID)

	// Generate a unique session ID
	sessionID := fmt.Sprintf("preview_%d_%s", time.Now().UnixNano(), randomString(9))

	session := &Session{
		ID:               sessionID,
		ProjectID:        projectID,
		UserID:           userID,
		IsActive:         true,
		CreatedAt:        time.Now(),
		ChannelOverrides: make(map[string]int),
	}

	s.sessions[sessionID] = session

	// Set auto-cleanup timeout
	timer := time.AfterFunc(s.sessionTimeout, func() {
		_, _ = s.CancelSession(ctx, sessionID)
	})
	s.sessionTimers[sessionID] = timer

	// Notify subscribers
	if s.onSessionUpdate != nil {
		dmxOutput := s.getCurrentDMXOutputLocked(sessionID)
		go s.onSessionUpdate(session, dmxOutput)
	}

	return session, nil
}

// UpdateChannelValue updates a channel value in a preview session.
func (s *Service) UpdateChannelValue(ctx context.Context, sessionID string, fixtureID string, channelIndex int, value int) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	session, exists := s.sessions[sessionID]
	if !exists || !session.IsActive {
		return false, nil
	}

	// Get fixture information to calculate universe and channel
	fixture, err := s.fixtureRepo.FindByID(ctx, fixtureID)
	if err != nil {
		return false, err
	}
	if fixture == nil {
		return false, nil
	}

	absoluteChannel := fixture.StartChannel + channelIndex
	channelKey := fmt.Sprintf("%d:%d", fixture.Universe, absoluteChannel)

	// Clamp value to valid range
	if value < 0 {
		value = 0
	}
	if value > 255 {
		value = 255
	}

	// Update the channel override in session state
	session.ChannelOverrides[channelKey] = value

	// Apply to live DMX output immediately via channel override.
	// Preview overrides take precedence over scene playback, allowing
	// real-time channel adjustments to be visible on the physical fixtures.
	if s.dmxService != nil {
		s.dmxService.SetChannelOverride(fixture.Universe, absoluteChannel, byte(value))
	}

	// Reset session timeout
	if timer, exists := s.sessionTimers[sessionID]; exists {
		timer.Reset(s.sessionTimeout)
	}
	session.CreatedAt = time.Now()

	// Notify subscribers
	if s.onSessionUpdate != nil {
		dmxOutput := s.getCurrentDMXOutputLocked(sessionID)
		go s.onSessionUpdate(session, dmxOutput)
	}

	return true, nil
}

// CommitSession commits a preview session (keeps changes, cleans up session).
func (s *Service) CommitSession(ctx context.Context, sessionID string) (bool, error) {
	// The preview changes are already live in DMX output
	// Just clean up the session
	return s.CancelSession(ctx, sessionID)
}

// CancelSession cancels a preview session and removes its overrides.
func (s *Service) CancelSession(ctx context.Context, sessionID string) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	session, exists := s.sessions[sessionID]
	if !exists {
		return false, nil
	}

	// Clear timeout timer
	if timer, exists := s.sessionTimers[sessionID]; exists {
		timer.Stop()
		delete(s.sessionTimers, sessionID)
	}

	// Remove channel overrides from DMX output
	for channelKey := range session.ChannelOverrides {
		var universe, channel int
		_, _ = fmt.Sscanf(channelKey, "%d:%d", &universe, &channel)
		if s.dmxService != nil {
			s.dmxService.ClearChannelOverride(universe, channel)
		}
	}

	// Mark session as inactive and remove
	session.IsActive = false
	delete(s.sessions, sessionID)

	// Notify subscribers
	if s.onSessionUpdate != nil {
		go s.onSessionUpdate(session, []DMXOutput{})
	}

	return true, nil
}

// InitializeWithScene initializes a preview session with values from a scene.
func (s *Service) InitializeWithScene(ctx context.Context, sessionID string, sceneID string) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	session, exists := s.sessions[sessionID]
	if !exists || !session.IsActive {
		return false, nil
	}

	// Get scene with fixture values
	scene, err := s.sceneRepo.FindByID(ctx, sceneID)
	if err != nil {
		return false, err
	}
	if scene == nil {
		return false, nil
	}

	fixtureValues, err := s.sceneRepo.GetFixtureValues(ctx, sceneID)
	if err != nil {
		return false, err
	}

	// Apply all fixture values from the scene
	for _, fv := range fixtureValues {
		fixture, err := s.fixtureRepo.FindByID(ctx, fv.FixtureID)
		if err != nil {
			continue
		}
		if fixture == nil {
			continue
		}

		// Parse channel values from JSON
		channelValues := parseChannelValues(fv.ChannelValues)

		for channelIndex, value := range channelValues {
			absoluteChannel := fixture.StartChannel + channelIndex
			channelKey := fmt.Sprintf("%d:%d", fixture.Universe, absoluteChannel)

			// Clamp value
			if value < 0 {
				value = 0
			}
			if value > 255 {
				value = 255
			}

			session.ChannelOverrides[channelKey] = value

			// Apply to live DMX output via override (preview takes precedence)
			if s.dmxService != nil {
				s.dmxService.SetChannelOverride(fixture.Universe, absoluteChannel, byte(value))
			}
		}
	}

	// Notify subscribers
	if s.onSessionUpdate != nil {
		dmxOutput := s.getCurrentDMXOutputLocked(sessionID)
		go s.onSessionUpdate(session, dmxOutput)
	}

	return true, nil
}

// GetSession returns a session by ID.
func (s *Service) GetSession(sessionID string) *Session {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.sessions[sessionID]
}

// GetProjectSession returns the active session for a project.
func (s *Service) GetProjectSession(projectID string) *Session {
	s.mu.RLock()
	defer s.mu.RUnlock()

	for _, session := range s.sessions {
		if session.ProjectID == projectID && session.IsActive {
			return session
		}
	}
	return nil
}

// GetDMXOutput returns the current DMX output for a session.
func (s *Service) GetDMXOutput(sessionID string) []DMXOutput {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.getCurrentDMXOutputLocked(sessionID)
}

// cancelExistingProjectSessionsLocked cancels all sessions for a project.
// Must be called with lock held.
func (s *Service) cancelExistingProjectSessionsLocked(projectID string) {
	for sessionID, session := range s.sessions {
		if session.ProjectID == projectID && session.IsActive {
			// Clear timeout timer
			if timer, exists := s.sessionTimers[sessionID]; exists {
				timer.Stop()
				delete(s.sessionTimers, sessionID)
			}

			// Remove channel overrides
			for channelKey := range session.ChannelOverrides {
				var universe, channel int
				_, _ = fmt.Sscanf(channelKey, "%d:%d", &universe, &channel)
				if s.dmxService != nil {
					s.dmxService.ClearChannelOverride(universe, channel)
				}
			}

			session.IsActive = false
			delete(s.sessions, sessionID)
		}
	}
}

// getCurrentDMXOutputLocked returns the current DMX output for a session.
// Must be called with lock held.
func (s *Service) getCurrentDMXOutputLocked(sessionID string) []DMXOutput {
	session, exists := s.sessions[sessionID]
	if !exists {
		return nil
	}

	// Collect universes used
	universes := make(map[int]bool)
	for channelKey := range session.ChannelOverrides {
		var universe int
		_, _ = fmt.Sscanf(channelKey, "%d:", &universe)
		universes[universe] = true
	}

	var output []DMXOutput
	for universe := range universes {
		channels := s.getUniverseChannelsLocked(universe, sessionID)
		output = append(output, DMXOutput{
			Universe: universe,
			Channels: channels,
		})
	}

	return output
}

// getUniverseChannelsLocked returns channel values for a universe with preview overrides.
// Must be called with lock held.
func (s *Service) getUniverseChannelsLocked(universe int, sessionID string) []int {
	channels := make([]int, 512)

	// Get current DMX state
	if s.dmxService != nil {
		currentChannels := s.dmxService.GetUniverse(universe)
		copy(channels, currentChannels)
	}

	// Apply preview overrides
	if sessionID != "" {
		session, exists := s.sessions[sessionID]
		if exists {
			for channelKey, value := range session.ChannelOverrides {
				var channelUniverse, channel int
				_, _ = fmt.Sscanf(channelKey, "%d:%d", &channelUniverse, &channel)
				if channelUniverse == universe && channel >= 1 && channel <= 512 {
					channels[channel-1] = value
				}
			}
		}
	}

	return channels
}

// Helper functions

func randomString(length int) string {
	const chars = "abcdefghijklmnopqrstuvwxyz0123456789"
	result := make([]byte, length)
	for i := range result {
		result[i] = chars[time.Now().UnixNano()%int64(len(chars))]
		time.Sleep(time.Nanosecond) // Ensure different timestamps
	}
	return string(result)
}

func parseChannelValues(jsonStr string) []int {
	var values []int
	// Simple JSON array parsing
	if len(jsonStr) < 2 {
		return values
	}
	// Remove brackets
	jsonStr = jsonStr[1 : len(jsonStr)-1]
	if jsonStr == "" {
		return values
	}

	// Split by comma and parse
	var current string
	for _, c := range jsonStr {
		if c == ',' {
			if current != "" {
				var v int
				_, _ = fmt.Sscanf(current, "%d", &v)
				values = append(values, v)
				current = ""
			}
		} else if c >= '0' && c <= '9' || c == '-' {
			current += string(c)
		}
	}
	if current != "" {
		var v int
		_, _ = fmt.Sscanf(current, "%d", &v)
		values = append(values, v)
	}

	return values
}
