// Package playback provides cue list playback functionality.
package playback

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/bbernstein/lacylights-go/internal/database/models"
	"github.com/bbernstein/lacylights-go/internal/services/dmx"
	"github.com/bbernstein/lacylights-go/internal/services/fade"
	"gorm.io/gorm"
)

// CueForPlayback represents the essential cue info for playback.
type CueForPlayback struct {
	ID          string
	Name        string
	CueNumber   float64
	FadeInTime  float64
	FadeOutTime float64
	FollowTime  *float64
}

// PlaybackState represents the current state of cue list playback.
type PlaybackState struct {
	CueListID       string
	CurrentCueIndex *int
	IsPlaying       bool // True when scene values are active on DMX (stays true after fade until stopped)
	IsFading        bool // True when a fade transition is in progress
	CurrentCue      *CueForPlayback
	FadeProgress    float64
	StartTime       *time.Time
	LastUpdated     time.Time
}

// CueListPlaybackStatus is the GraphQL-compatible status response.
type CueListPlaybackStatus struct {
	CueListID       string
	CurrentCueIndex *int
	IsPlaying       bool // True when scene values are active on DMX (stays true after fade until stopped)
	IsFading        bool // True when a fade transition is in progress
	CurrentCue      *CueForPlayback
	FadeProgress    float64
	LastUpdated     string
}

// GlobalPlaybackStatus represents the global playback state across all cue lists.
type GlobalPlaybackStatus struct {
	IsPlaying       bool
	IsFading        bool
	CueListID       *string
	CueListName     *string
	CurrentCueIndex *int
	CueCount        *int
	CurrentCueName  *string
	FadeProgress    float64
	LastUpdated     string
}

// Service manages cue list playback.
type Service struct {
	mu sync.RWMutex

	db         *gorm.DB
	dmxService *dmx.Service
	fadeEngine *fade.Engine

	// Playback states by cue list ID
	states map[string]*PlaybackState

	// Timers for fade progress tracking, follow times, and fade completion
	fadeProgressTickers map[string]*time.Ticker
	followTimers        map[string]*time.Timer
	fadeCompleteTimers  map[string]*time.Timer

	// Callback for subscription updates (optional)
	onUpdate func(status *CueListPlaybackStatus)

	// Callback for global playback status updates (optional)
	onGlobalUpdate func(status *GlobalPlaybackStatus)
}

// NewService creates a new playback service.
func NewService(db *gorm.DB, dmxService *dmx.Service, fadeEngine *fade.Engine) *Service {
	return &Service{
		db:                  db,
		dmxService:          dmxService,
		fadeEngine:          fadeEngine,
		states:              make(map[string]*PlaybackState),
		fadeProgressTickers: make(map[string]*time.Ticker),
		followTimers:        make(map[string]*time.Timer),
		fadeCompleteTimers:  make(map[string]*time.Timer),
	}
}

// SetUpdateCallback sets the callback for playback status updates.
func (s *Service) SetUpdateCallback(callback func(status *CueListPlaybackStatus)) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.onUpdate = callback
}

// SetGlobalUpdateCallback sets the callback for global playback status updates.
func (s *Service) SetGlobalUpdateCallback(callback func(status *GlobalPlaybackStatus)) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.onGlobalUpdate = callback
}

// GetPlaybackState returns a copy of the current playback state for a cue list.
// Returns nil if no state exists for the given cue list ID.
func (s *Service) GetPlaybackState(cueListID string) *PlaybackState {
	s.mu.RLock()
	defer s.mu.RUnlock()
	state := s.states[cueListID]
	if state == nil {
		return nil
	}
	// Return a copy to avoid data races
	stateCopy := *state
	if state.CurrentCueIndex != nil {
		cueIndexCopy := *state.CurrentCueIndex
		stateCopy.CurrentCueIndex = &cueIndexCopy
	}
	if state.CurrentCue != nil {
		cueCopy := *state.CurrentCue
		stateCopy.CurrentCue = &cueCopy
	}
	if state.StartTime != nil {
		startTimeCopy := *state.StartTime
		stateCopy.StartTime = &startTimeCopy
	}
	return &stateCopy
}

// GetFormattedStatus returns the GraphQL-compatible status for a cue list.
func (s *Service) GetFormattedStatus(cueListID string) *CueListPlaybackStatus {
	s.mu.RLock()
	defer s.mu.RUnlock()

	state := s.states[cueListID]

	if state == nil {
		return &CueListPlaybackStatus{
			CueListID:       cueListID,
			CurrentCueIndex: nil,
			IsPlaying:       false,
			IsFading:        false,
			CurrentCue:      nil,
			FadeProgress:    0,
			LastUpdated:     time.Now().Format(time.RFC3339),
		}
	}

	return &CueListPlaybackStatus{
		CueListID:       state.CueListID,
		CurrentCueIndex: state.CurrentCueIndex,
		IsPlaying:       state.IsPlaying,
		IsFading:        state.IsFading,
		CurrentCue:      state.CurrentCue,
		FadeProgress:    state.FadeProgress,
		LastUpdated:     state.LastUpdated.Format(time.RFC3339),
	}
}

// GetGlobalPlaybackStatus returns the global playback status across all cue lists.
// It finds the currently playing cue list (if any) and returns its status with cue list details.
func (s *Service) GetGlobalPlaybackStatus(ctx context.Context) *GlobalPlaybackStatus {
	s.mu.RLock()

	// Find the currently playing cue list
	var playingState *PlaybackState
	for _, state := range s.states {
		if state.IsPlaying {
			playingState = state
			break
		}
	}

	// If nothing is playing, return empty status
	if playingState == nil {
		s.mu.RUnlock()
		return &GlobalPlaybackStatus{
			IsPlaying:   false,
			IsFading:    false,
			LastUpdated: time.Now().Format(time.RFC3339),
		}
	}

	// Copy the state data while holding the lock
	cueListID := playingState.CueListID
	isPlaying := playingState.IsPlaying
	isFading := playingState.IsFading
	fadeProgress := playingState.FadeProgress
	lastUpdated := playingState.LastUpdated.Format(time.RFC3339)

	var currentCueIndex *int
	if playingState.CurrentCueIndex != nil {
		idx := *playingState.CurrentCueIndex
		currentCueIndex = &idx
	}

	var currentCueName *string
	if playingState.CurrentCue != nil {
		name := playingState.CurrentCue.Name
		currentCueName = &name
	}

	s.mu.RUnlock()

	// Get cue list details from database
	var cueList models.CueList
	result := s.db.WithContext(ctx).
		Preload("Cues").
		First(&cueList, "id = ?", cueListID)

	var cueListName *string
	var cueCount *int
	if result.Error == nil {
		cueListName = &cueList.Name
		count := len(cueList.Cues)
		cueCount = &count
	}

	return &GlobalPlaybackStatus{
		IsPlaying:       isPlaying,
		IsFading:        isFading,
		CueListID:       &cueListID,
		CueListName:     cueListName,
		CurrentCueIndex: currentCueIndex,
		CueCount:        cueCount,
		CurrentCueName:  currentCueName,
		FadeProgress:    fadeProgress,
		LastUpdated:     lastUpdated,
	}
}

// StartCue starts playing a cue.
func (s *Service) StartCue(cueListID string, cueIndex int, cue *CueForPlayback) {
	// Stop any existing playback for this cue list
	s.StopCueList(cueListID)

	s.mu.Lock()
	now := time.Now()
	state := &PlaybackState{
		CueListID:       cueListID,
		CurrentCueIndex: &cueIndex,
		IsPlaying:       true,  // Scene is now active on DMX
		IsFading:        true,  // Fade transition is starting
		CurrentCue: &CueForPlayback{
			ID:          cue.ID,
			Name:        cue.Name,
			CueNumber:   cue.CueNumber,
			FadeInTime:  cue.FadeInTime,
			FadeOutTime: cue.FadeOutTime,
			FollowTime:  cue.FollowTime,
		},
		FadeProgress: 0,
		StartTime:    &now,
		LastUpdated:  now,
	}

	s.states[cueListID] = state
	s.mu.Unlock()

	// Start fade progress tracking
	s.startFadeProgress(cueListID, cue.FadeInTime)

	// Emit update
	s.emitUpdate(cueListID)

	// Schedule follow time if applicable
	if cue.FollowTime != nil && *cue.FollowTime > 0 {
		totalWaitTime := time.Duration((cue.FadeInTime + *cue.FollowTime) * float64(time.Second))

		s.mu.Lock()
		timer := time.AfterFunc(totalWaitTime, func() {
			s.handleFollowTime(cueListID, cueIndex)
		})
		s.followTimers[cueListID] = timer
		s.mu.Unlock()
	}

	// Mark fade as complete after fadeInTime (but keep isPlaying true - scene is still active)
	fadeTime := time.Duration(cue.FadeInTime * float64(time.Second))
	s.mu.Lock()
	// Stop any existing fade complete timer for this cue list
	if existingTimer := s.fadeCompleteTimers[cueListID]; existingTimer != nil {
		existingTimer.Stop()
	}
	fadeCompleteTimer := time.AfterFunc(fadeTime, func() {
		s.mu.Lock()
		currentState := s.states[cueListID]
		if currentState != nil && currentState.CurrentCueIndex != nil && *currentState.CurrentCueIndex == cueIndex {
			currentState.IsFading = false // Fade complete, but scene still playing
			currentState.LastUpdated = time.Now()
		}
		// Clean up the timer from the map
		delete(s.fadeCompleteTimers, cueListID)
		s.mu.Unlock()
		s.emitUpdate(cueListID)
	})
	s.fadeCompleteTimers[cueListID] = fadeCompleteTimer
	s.mu.Unlock()
}

// ExecuteCueDmx executes a cue's DMX output.
func (s *Service) ExecuteCueDmx(ctx context.Context, cueID string, fadeInTimeOverride *float64) error {
	// Load the cue with its scene and fixture values
	var cue models.Cue
	result := s.db.WithContext(ctx).
		Preload("Scene.FixtureValues").
		First(&cue, "id = ?", cueID)
	if result.Error != nil {
		return fmt.Errorf("cue not found: %w", result.Error)
	}

	if cue.Scene == nil {
		return fmt.Errorf("cue has no scene")
	}

	// Load fixtures for the scene's fixture values
	var fixtureIDs []string
	for _, fv := range cue.Scene.FixtureValues {
		fixtureIDs = append(fixtureIDs, fv.FixtureID)
	}

	var fixtures []models.FixtureInstance
	if len(fixtureIDs) > 0 {
		// Load fixtures with their channels to get fadeBehavior
		s.db.WithContext(ctx).Preload("Channels").Where("id IN ?", fixtureIDs).Find(&fixtures)
	}

	// Create fixture lookup map
	fixtureMap := make(map[string]*models.FixtureInstance)
	for i := range fixtures {
		fixtureMap[fixtures[i].ID] = &fixtures[i]
	}

	// Determine fade time
	actualFadeTime := cue.FadeInTime
	if fadeInTimeOverride != nil {
		actualFadeTime = *fadeInTimeOverride
	}

	// Build scene channels for fade engine
	var sceneChannels []fade.SceneChannel

	for _, fixtureValue := range cue.Scene.FixtureValues {
		fixture := fixtureMap[fixtureValue.FixtureID]
		if fixture == nil {
			continue
		}

		// Parse sparse channel values from JSON (Channels field)
		var channels []models.ChannelValue
		if err := json.Unmarshal([]byte(fixtureValue.Channels), &channels); err != nil {
			log.Printf("Warning: failed to unmarshal channels for fixtureID %s in cueID %s: %v (raw: %v)", fixtureValue.FixtureID, cue.ID, err, fixtureValue.Channels)
			continue
		}

		// Build channel targets with fade behavior from channel definitions
		// Only process channels that exist in the sparse array
		for _, ch := range channels {
			dmxChannel := fixture.StartChannel + ch.Offset

			// Validate DMX channel is within bounds (1-512 per universe)
			if dmxChannel < 1 || dmxChannel > 512 {
				log.Printf("Warning: DMX channel %d out of bounds for fixture %s (universe %d, StartChannel: %d, Offset: %d). Skipping.",
					dmxChannel, fixture.ID, fixture.Universe, fixture.StartChannel, ch.Offset)
				continue
			}

			// Get fade behavior from channel definition (if available)
			fadeBehavior := fade.FadeBehaviorFade // Default to FADE
			// Find the channel definition with matching offset
			for _, chanDef := range fixture.Channels {
				if chanDef.Offset == ch.Offset {
					if chanDef.FadeBehavior != "" {
						fadeBehavior = chanDef.FadeBehavior
					}
					break
				}
			}

			sceneChannels = append(sceneChannels, fade.SceneChannel{
				Universe:     fixture.Universe,
				Channel:      dmxChannel,
				Value:        ch.Value,
				FadeBehavior: fadeBehavior,
			})
		}
	}

	// Get easing type
	easingType := fade.EasingInOutSine
	if cue.EasingType != nil && *cue.EasingType != "" {
		easingType = fade.EasingType(*cue.EasingType)
	}

	// Execute fade
	fadeID := fmt.Sprintf("cue-%s", cueID)
	s.fadeEngine.FadeToScene(sceneChannels, time.Duration(actualFadeTime*float64(time.Second)), fadeID, easingType)

	// Track the active scene
	s.dmxService.SetActiveScene(cue.SceneID)

	return nil
}

// handleFollowTime handles automatic follow to the next cue.
func (s *Service) handleFollowTime(cueListID string, currentCueIndex int) {
	ctx := context.Background()

	// Load cue list with cues
	var cueList models.CueList
	result := s.db.WithContext(ctx).
		Preload("Cues", func(db *gorm.DB) *gorm.DB {
			return db.Order("cue_number ASC")
		}).
		First(&cueList, "id = ?", cueListID)

	if result.Error != nil {
		s.mu.Lock()
		state := s.states[cueListID]
		if state != nil {
			state.IsPlaying = false
			state.LastUpdated = time.Now()
		}
		s.mu.Unlock()
		s.emitUpdate(cueListID)
		return
	}

	// Determine next cue index
	nextCueIndex := currentCueIndex + 1

	// Check if we've reached the end
	if nextCueIndex >= len(cueList.Cues) {
		if cueList.Loop && len(cueList.Cues) > 0 {
			// Loop back to the first cue
			nextCueIndex = 0
		} else {
			// No loop, mark as stopped
			s.mu.Lock()
			state := s.states[cueListID]
			if state != nil {
				state.IsPlaying = false
				state.LastUpdated = time.Now()
			}
			s.mu.Unlock()
			s.emitUpdate(cueListID)
			return
		}
	}

	// Get the next cue
	nextCue := cueList.Cues[nextCueIndex]

	// Execute the cue's DMX output
	if err := s.ExecuteCueDmx(ctx, nextCue.ID, nil); err != nil {
		s.StopCueList(cueListID)
		return
	}

	// Update playback state for the new cue
	cueForPlayback := &CueForPlayback{
		ID:          nextCue.ID,
		Name:        nextCue.Name,
		CueNumber:   nextCue.CueNumber,
		FadeInTime:  nextCue.FadeInTime,
		FadeOutTime: nextCue.FadeOutTime,
		FollowTime:  nextCue.FollowTime,
	}
	s.StartCue(cueListID, nextCueIndex, cueForPlayback)
}

// StopCueList stops playback for a cue list.
func (s *Service) StopCueList(cueListID string) {
	s.mu.Lock()

	// Stop fade progress ticker
	if ticker := s.fadeProgressTickers[cueListID]; ticker != nil {
		ticker.Stop()
		delete(s.fadeProgressTickers, cueListID)
	}

	// Stop follow timer
	if timer := s.followTimers[cueListID]; timer != nil {
		timer.Stop()
		delete(s.followTimers, cueListID)
	}

	// Stop fade completion timer
	if timer := s.fadeCompleteTimers[cueListID]; timer != nil {
		timer.Stop()
		delete(s.fadeCompleteTimers, cueListID)
	}

	// Update state - scene is no longer active on DMX
	state := s.states[cueListID]
	if state != nil {
		state.IsPlaying = false  // Scene no longer active on DMX
		state.IsFading = false   // No fade in progress
		state.FadeProgress = 0
		state.LastUpdated = time.Now()
	}

	s.mu.Unlock()

	s.emitUpdate(cueListID)
}

// StopAllCueLists stops all cue list playback.
func (s *Service) StopAllCueLists() {
	s.mu.RLock()
	cueListIDs := make([]string, 0, len(s.states))
	for id := range s.states {
		cueListIDs = append(cueListIDs, id)
	}
	s.mu.RUnlock()

	for _, id := range cueListIDs {
		s.StopCueList(id)
	}
}

// JumpToCue jumps to a specific cue in a cue list.
func (s *Service) JumpToCue(ctx context.Context, cueListID string, cueIndex int, fadeInTimeOverride *float64) error {
	// Load cue list with cues
	var cueList models.CueList
	result := s.db.WithContext(ctx).
		Preload("Cues", func(db *gorm.DB) *gorm.DB {
			return db.Order("cue_number ASC")
		}).
		First(&cueList, "id = ?", cueListID)

	if result.Error != nil {
		return fmt.Errorf("cue list not found: %w", result.Error)
	}

	if cueIndex < 0 || cueIndex >= len(cueList.Cues) {
		return fmt.Errorf("invalid cue index: %d", cueIndex)
	}

	cue := cueList.Cues[cueIndex]

	// Execute DMX output for the cue
	if err := s.ExecuteCueDmx(ctx, cue.ID, fadeInTimeOverride); err != nil {
		return err
	}

	// Determine fade time for state tracking
	fadeInTime := cue.FadeInTime
	if fadeInTimeOverride != nil {
		fadeInTime = *fadeInTimeOverride
	}

	cueForPlayback := &CueForPlayback{
		ID:          cue.ID,
		Name:        cue.Name,
		CueNumber:   cue.CueNumber,
		FadeInTime:  fadeInTime,
		FadeOutTime: cue.FadeOutTime,
		FollowTime:  cue.FollowTime,
	}

	s.StartCue(cueListID, cueIndex, cueForPlayback)
	return nil
}

// NextCue advances to the next cue.
func (s *Service) NextCue(ctx context.Context, cueListID string, fadeInTimeOverride *float64) error {
	s.mu.RLock()
	state := s.states[cueListID]
	s.mu.RUnlock()

	currentIndex := -1
	if state != nil && state.CurrentCueIndex != nil {
		currentIndex = *state.CurrentCueIndex
	}

	// Load cue list with cues
	var cueList models.CueList
	result := s.db.WithContext(ctx).
		Preload("Cues", func(db *gorm.DB) *gorm.DB {
			return db.Order("cue_number ASC")
		}).
		First(&cueList, "id = ?", cueListID)

	if result.Error != nil {
		return fmt.Errorf("cue list not found: %w", result.Error)
	}

	nextIndex := currentIndex + 1
	if nextIndex >= len(cueList.Cues) {
		if cueList.Loop && len(cueList.Cues) > 0 {
			nextIndex = 0
		} else {
			return fmt.Errorf("no more cues in the list")
		}
	}

	cue := cueList.Cues[nextIndex]

	// Execute DMX
	if err := s.ExecuteCueDmx(ctx, cue.ID, fadeInTimeOverride); err != nil {
		return err
	}

	// Start playback state
	cueForPlayback := &CueForPlayback{
		ID:          cue.ID,
		Name:        cue.Name,
		CueNumber:   cue.CueNumber,
		FadeInTime:  cue.FadeInTime,
		FadeOutTime: cue.FadeOutTime,
		FollowTime:  cue.FollowTime,
	}
	s.StartCue(cueListID, nextIndex, cueForPlayback)

	return nil
}

// PreviousCue goes back to the previous cue.
func (s *Service) PreviousCue(ctx context.Context, cueListID string, fadeInTimeOverride *float64) error {
	s.mu.RLock()
	state := s.states[cueListID]
	s.mu.RUnlock()

	currentIndex := 0
	if state != nil && state.CurrentCueIndex != nil {
		currentIndex = *state.CurrentCueIndex
	}

	// Load cue list with cues
	var cueList models.CueList
	result := s.db.WithContext(ctx).
		Preload("Cues", func(db *gorm.DB) *gorm.DB {
			return db.Order("cue_number ASC")
		}).
		First(&cueList, "id = ?", cueListID)

	if result.Error != nil {
		return fmt.Errorf("cue list not found: %w", result.Error)
	}

	prevIndex := currentIndex - 1
	if prevIndex < 0 {
		if cueList.Loop && len(cueList.Cues) > 0 {
			prevIndex = len(cueList.Cues) - 1
		} else {
			return fmt.Errorf("already at first cue")
		}
	}

	cue := cueList.Cues[prevIndex]

	// Execute DMX
	if err := s.ExecuteCueDmx(ctx, cue.ID, fadeInTimeOverride); err != nil {
		return err
	}

	// Start playback state
	cueForPlayback := &CueForPlayback{
		ID:          cue.ID,
		Name:        cue.Name,
		CueNumber:   cue.CueNumber,
		FadeInTime:  cue.FadeInTime,
		FadeOutTime: cue.FadeOutTime,
		FollowTime:  cue.FollowTime,
	}
	s.StartCue(cueListID, prevIndex, cueForPlayback)

	return nil
}

// GoToCueNumber jumps to a cue by its cue number.
func (s *Service) GoToCueNumber(ctx context.Context, cueListID string, cueNumber float64, fadeInTimeOverride *float64) error {
	// Load cue list with cues
	var cueList models.CueList
	result := s.db.WithContext(ctx).
		Preload("Cues", func(db *gorm.DB) *gorm.DB {
			return db.Order("cue_number ASC")
		}).
		First(&cueList, "id = ?", cueListID)

	if result.Error != nil {
		return fmt.Errorf("cue list not found: %w", result.Error)
	}

	// Find the cue by number
	cueIndex := -1
	for i, cue := range cueList.Cues {
		if cue.CueNumber == cueNumber {
			cueIndex = i
			break
		}
	}

	if cueIndex < 0 {
		return fmt.Errorf("cue number %f not found", cueNumber)
	}

	cue := cueList.Cues[cueIndex]

	// Execute DMX
	if err := s.ExecuteCueDmx(ctx, cue.ID, fadeInTimeOverride); err != nil {
		return err
	}

	// Start playback state
	cueForPlayback := &CueForPlayback{
		ID:          cue.ID,
		Name:        cue.Name,
		CueNumber:   cue.CueNumber,
		FadeInTime:  cue.FadeInTime,
		FadeOutTime: cue.FadeOutTime,
		FollowTime:  cue.FollowTime,
	}
	s.StartCue(cueListID, cueIndex, cueForPlayback)

	return nil
}

// GoToCueName jumps to a cue by its name.
func (s *Service) GoToCueName(ctx context.Context, cueListID string, cueName string, fadeInTimeOverride *float64) error {
	// Load cue list with cues
	var cueList models.CueList
	result := s.db.WithContext(ctx).
		Preload("Cues", func(db *gorm.DB) *gorm.DB {
			return db.Order("cue_number ASC")
		}).
		First(&cueList, "id = ?", cueListID)

	if result.Error != nil {
		return fmt.Errorf("cue list not found: %w", result.Error)
	}

	// Find the cue by name
	cueIndex := -1
	for i, cue := range cueList.Cues {
		if cue.Name == cueName {
			cueIndex = i
			break
		}
	}

	if cueIndex < 0 {
		return fmt.Errorf("cue name %s not found", cueName)
	}

	cue := cueList.Cues[cueIndex]

	// Execute DMX
	if err := s.ExecuteCueDmx(ctx, cue.ID, fadeInTimeOverride); err != nil {
		return err
	}

	// Start playback state
	cueForPlayback := &CueForPlayback{
		ID:          cue.ID,
		Name:        cue.Name,
		CueNumber:   cue.CueNumber,
		FadeInTime:  cue.FadeInTime,
		FadeOutTime: cue.FadeOutTime,
		FollowTime:  cue.FollowTime,
	}
	s.StartCue(cueListID, cueIndex, cueForPlayback)

	return nil
}

// StartCueList starts playing a cue list from the beginning or a specific cue.
func (s *Service) StartCueList(ctx context.Context, cueListID string, startFromCueNumber *float64, fadeInTimeOverride *float64) error {
	// Load cue list with cues
	var cueList models.CueList
	result := s.db.WithContext(ctx).
		Preload("Cues", func(db *gorm.DB) *gorm.DB {
			return db.Order("cue_number ASC")
		}).
		First(&cueList, "id = ?", cueListID)

	if result.Error != nil {
		return fmt.Errorf("cue list not found: %w", result.Error)
	}

	if len(cueList.Cues) == 0 {
		return fmt.Errorf("cue list is empty")
	}

	// Find starting cue index
	startIndex := 0
	if startFromCueNumber != nil {
		for i, cue := range cueList.Cues {
			if cue.CueNumber == *startFromCueNumber {
				startIndex = i
				break
			}
		}
	}

	cue := cueList.Cues[startIndex]

	// Execute DMX with optional fade time override
	if err := s.ExecuteCueDmx(ctx, cue.ID, fadeInTimeOverride); err != nil {
		return err
	}

	// Determine the actual fade time for progress tracking
	actualFadeTime := cue.FadeInTime
	if fadeInTimeOverride != nil {
		actualFadeTime = *fadeInTimeOverride
	}

	// Start playback state with actual fade time
	cueForPlayback := &CueForPlayback{
		ID:          cue.ID,
		Name:        cue.Name,
		CueNumber:   cue.CueNumber,
		FadeInTime:  actualFadeTime, // Use actual fade time for tracking
		FadeOutTime: cue.FadeOutTime,
		FollowTime:  cue.FollowTime,
	}
	s.StartCue(cueListID, startIndex, cueForPlayback)

	return nil
}

// startFadeProgress starts tracking fade progress.
func (s *Service) startFadeProgress(cueListID string, fadeTime float64) {
	s.mu.Lock()
	state := s.states[cueListID]
	if state == nil {
		s.mu.Unlock()
		return
	}

	startTime := time.Now()

	// Create ticker for fade progress updates (100ms interval)
	ticker := time.NewTicker(100 * time.Millisecond)
	s.fadeProgressTickers[cueListID] = ticker
	s.mu.Unlock()

	go func() {
		for range ticker.C {
			s.mu.Lock()
			currentState := s.states[cueListID]
			if currentState == nil {
				s.mu.Unlock()
				return
			}

			elapsed := time.Since(startTime)
			progress := float64(elapsed) / (fadeTime * float64(time.Second)) * 100
			if progress > 100 {
				progress = 100
			}

			currentState.FadeProgress = progress
			currentState.LastUpdated = time.Now()
			s.mu.Unlock()

			// Emit update
			s.emitUpdate(cueListID)

			if progress >= 100 {
				s.mu.Lock()
				if t := s.fadeProgressTickers[cueListID]; t != nil {
					t.Stop()
					delete(s.fadeProgressTickers, cueListID)
				}
				s.mu.Unlock()
				return
			}
		}
	}()
}

// emitUpdate emits a playback status update.
func (s *Service) emitUpdate(cueListID string) {
	s.mu.RLock()
	callback := s.onUpdate
	globalCallback := s.onGlobalUpdate
	s.mu.RUnlock()

	if callback != nil {
		status := s.GetFormattedStatus(cueListID)
		callback(status)
	}

	// Also emit global playback status update
	if globalCallback != nil {
		globalStatus := s.GetGlobalPlaybackStatus(context.Background())
		globalCallback(globalStatus)
	}
}

// Cleanup cleans up all resources.
func (s *Service) Cleanup() {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Stop all fade progress tickers
	for _, ticker := range s.fadeProgressTickers {
		ticker.Stop()
	}

	// Stop all follow timers
	for _, timer := range s.followTimers {
		timer.Stop()
	}

	// Stop all fade completion timers
	for _, timer := range s.fadeCompleteTimers {
		timer.Stop()
	}

	s.fadeProgressTickers = make(map[string]*time.Ticker)
	s.followTimers = make(map[string]*time.Timer)
	s.fadeCompleteTimers = make(map[string]*time.Timer)
	s.states = make(map[string]*PlaybackState)
}
