// Package dmx provides DMX output management and Art-Net communication.
package dmx

import (
	"log"
	"net"
	"os"
	"strconv"
	"sync"
	"time"

	"github.com/bbernstein/lacylights-go/pkg/artnet"
)

const (
	// UniverseSize is the number of channels per DMX universe.
	UniverseSize = 512
	// MaxUniverses is the maximum number of supported universes.
	MaxUniverses = 4
)

// Service manages DMX channel values and Art-Net output.
type Service struct {
	mu sync.RWMutex

	// Channel values for each universe (1-indexed in the map, 0-indexed channels)
	universes map[int][]byte

	// Channel overrides (key: "universe:channel", 1-indexed)
	channelOverrides map[string]byte

	// Active scene tracking
	activeSceneID *string

	// Configuration
	enabled         bool
	broadcastAddr   string
	port            int
	refreshRateHz   int
	idleRateHz      int
	highRateDuration time.Duration

	// Adaptive transmission rate state
	currentRate      int
	isInHighRateMode bool
	lastChangeTime   time.Time

	// Dirty flag system for efficient transmission
	isDirty        bool
	dirtyUniverses map[int]bool

	// Timing tracking
	lastTransmissionTime time.Time

	// Art-Net sequence number (increments for each packet, wraps at 255)
	sequence byte

	// UDP socket
	conn *net.UDPConn
	addr *net.UDPAddr

	// Control
	stopChan       chan struct{}
	resetTickerChan chan struct{} // Signal to reset ticker immediately when rate changes
	running        bool
}

// Config holds DMX service configuration.
type Config struct {
	Enabled          bool
	BroadcastAddr    string
	Port             int
	RefreshRateHz    int
	IdleRateHz       int
	HighRateDuration time.Duration
}

// DefaultConfig returns a configuration with default values.
func DefaultConfig() Config {
	return Config{
		Enabled:          true,
		BroadcastAddr:    "255.255.255.255",
		Port:             artnet.DefaultPort,
		RefreshRateHz:    60, // Match fade engine default (60Hz)
		IdleRateHz:       1,
		HighRateDuration: 2 * time.Second,
	}
}

// ConfigFromEnv loads configuration from environment variables.
func ConfigFromEnv() Config {
	cfg := DefaultConfig()

	if enabled := os.Getenv("ARTNET_ENABLED"); enabled == "false" {
		cfg.Enabled = false
	}

	if addr := os.Getenv("ARTNET_BROADCAST"); addr != "" {
		cfg.BroadcastAddr = addr
	}

	if port := os.Getenv("ARTNET_PORT"); port != "" {
		if p, err := strconv.Atoi(port); err == nil && p > 0 {
			cfg.Port = p
		}
	}

	if rate := os.Getenv("DMX_REFRESH_RATE"); rate != "" {
		if r, err := strconv.Atoi(rate); err == nil && r > 0 {
			cfg.RefreshRateHz = r
		}
	}

	if rate := os.Getenv("DMX_IDLE_RATE"); rate != "" {
		if r, err := strconv.Atoi(rate); err == nil && r > 0 {
			cfg.IdleRateHz = r
		}
	}

	if dur := os.Getenv("DMX_HIGH_RATE_DURATION"); dur != "" {
		if d, err := strconv.Atoi(dur); err == nil && d > 0 {
			cfg.HighRateDuration = time.Duration(d) * time.Millisecond
		}
	}

	return cfg
}

// NewService creates a new DMX service.
func NewService(cfg Config) *Service {
	// Apply defaults for zero values
	refreshRate := cfg.RefreshRateHz
	if refreshRate <= 0 {
		refreshRate = 60 // Default refresh rate (matches fade engine)
	}
	idleRate := cfg.IdleRateHz
	if idleRate <= 0 {
		idleRate = 1 // Default idle rate
	}
	highRateDuration := cfg.HighRateDuration
	if highRateDuration <= 0 {
		highRateDuration = 2 * time.Second
	}
	port := cfg.Port
	if port <= 0 {
		port = 6454 // Default Art-Net port
	}

	s := &Service{
		universes:        make(map[int][]byte),
		channelOverrides: make(map[string]byte),
		dirtyUniverses:   make(map[int]bool),
		enabled:          cfg.Enabled,
		broadcastAddr:    cfg.BroadcastAddr,
		port:             port,
		refreshRateHz:    refreshRate,
		idleRateHz:       idleRate,
		highRateDuration: highRateDuration,
		currentRate:      idleRate, // Start at idle rate until first change
		isInHighRateMode: false,
		stopChan:         make(chan struct{}),
		resetTickerChan:  make(chan struct{}, 1), // Buffered to avoid blocking
	}

	// Initialize universes with 512 channels each, all set to 0
	universeCount := 4
	if envCount := os.Getenv("DMX_UNIVERSE_COUNT"); envCount != "" {
		if c, err := strconv.Atoi(envCount); err == nil && c > 0 && c <= MaxUniverses {
			universeCount = c
		}
	}

	for i := 1; i <= universeCount; i++ {
		s.universes[i] = make([]byte, UniverseSize)
	}

	return s
}

// Initialize starts the DMX service and Art-Net transmission.
func (s *Service) Initialize() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.running {
		return nil
	}

	if s.enabled {
		// Create UDP socket for Art-Net broadcast
		addr, err := net.ResolveUDPAddr("udp4", s.broadcastAddr+":"+strconv.Itoa(s.port))
		if err != nil {
			return err
		}
		s.addr = addr

		conn, err := net.DialUDP("udp4", nil, addr)
		if err != nil {
			return err
		}
		s.conn = conn

		log.Printf("🎭 DMX Service initialized with %d universes", len(s.universes))
		log.Printf("📡 Adaptive transmission: %dHz (active) / %dHz (idle), %v high-rate duration",
			s.refreshRateHz, s.idleRateHz, s.highRateDuration)
		log.Printf("📡 Art-Net output enabled, broadcasting to %s:%d", s.broadcastAddr, s.port)
	} else {
		log.Printf("🎭 DMX Service initialized with %d universes (simulation mode)", len(s.universes))
	}

	// Start the transmission loop
	s.running = true
	go s.transmitLoop()

	return nil
}

// transmitLoop runs the adaptive rate transmission loop.
func (s *Service) transmitLoop() {
	// Use Ticker instead of Timer to maintain consistent timing without drift
	s.mu.RLock()
	interval := time.Second / time.Duration(s.currentRate)
	s.mu.RUnlock()
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	lastRate := 0

	for {
		select {
		case <-s.stopChan:
			return
		case <-s.resetTickerChan:
			// Immediately reset ticker when rate changes (e.g., from ForceImmediateTransmission)
			s.mu.RLock()
			currentRate := s.currentRate
			s.mu.RUnlock()

			if currentRate != lastRate {
				oldTicker := ticker
				newInterval := time.Second / time.Duration(currentRate)
				ticker = time.NewTicker(newInterval)
				oldTicker.Stop()
				lastRate = currentRate
				log.Printf("📡 DMX transmitLoop: ticker reset to %dHz immediately", currentRate)
			}
		case <-ticker.C:
			s.processTransmission()

			// Check if rate changed and recreate ticker if needed
			s.mu.RLock()
			currentRate := s.currentRate
			s.mu.RUnlock()

			if currentRate != lastRate {
				// Rate changed, recreate ticker with new interval
				// Stop old ticker before creating new one to avoid leaks
				oldTicker := ticker
				newInterval := time.Second / time.Duration(currentRate)
				ticker = time.NewTicker(newInterval)
				oldTicker.Stop()
				lastRate = currentRate
			}
		}
	}
}

// processTransmission handles a single transmission cycle.
func (s *Service) processTransmission() {
	s.mu.Lock()
	defer s.mu.Unlock()

	currentTime := time.Now()
	hasChanges := s.isDirty

	// Update transmission rate based on changes
	if hasChanges {
		s.lastChangeTime = currentTime
		if !s.isInHighRateMode {
			s.isInHighRateMode = true
			s.currentRate = s.refreshRateHz
			log.Printf("📡 DMX transmission: switching to high rate (%dHz) - changes detected", s.refreshRateHz)
		}
	} else {
		// Check if we should switch to idle rate
		timeSinceLastChange := currentTime.Sub(s.lastChangeTime)
		if s.isInHighRateMode && !s.lastChangeTime.IsZero() && timeSinceLastChange > s.highRateDuration {
			s.isInHighRateMode = false
			s.currentRate = s.idleRateHz
			log.Printf("📡 DMX transmission: switching to idle rate (%dHz) - no changes for %v", s.idleRateHz, timeSinceLastChange)
		}
	}

	// Always transmit in both high-rate and idle modes
	// High-rate mode: transmit at 60Hz for smooth fades/transitions
	// Idle mode: transmit at 1Hz for keep-alive
	// This ensures DMX output stays fresh and responsive
	if s.enabled && s.conn != nil {
		s.outputDMX()
	}
}

// outputDMX sends Art-Net packets for dirty or all universes.
func (s *Service) outputDMX() {
	var universesToTransmit []int

	if s.isDirty && len(s.dirtyUniverses) > 0 {
		// Only transmit changed universes
		for u := range s.dirtyUniverses {
			universesToTransmit = append(universesToTransmit, u)
		}
	} else {
		// In idle mode, transmit all universes for keep-alive
		for u := range s.universes {
			universesToTransmit = append(universesToTransmit, u)
		}
	}

	// Send Art-Net packets
	for _, universe := range universesToTransmit {
		channels := s.getUniverseOutputChannels(universe)

		// Increment sequence number for each packet (wraps at 255)
		s.sequence++
		packet := artnet.BuildDMXPacket(universe, channels, s.sequence)

		_, err := s.conn.Write(packet)
		if err != nil {
			log.Printf("Art-Net send error for universe %d: %v", universe, err)
		}
	}

	// Clear dirty flags after transmission
	s.isDirty = false
	s.dirtyUniverses = make(map[int]bool)
	s.lastTransmissionTime = time.Now()
}

// getUniverseOutputChannels returns the channel values with overrides applied.
func (s *Service) getUniverseOutputChannels(universe int) []byte {
	baseChannels := s.universes[universe]
	if baseChannels == nil {
		return make([]byte, UniverseSize)
	}

	outputChannels := make([]byte, UniverseSize)
	copy(outputChannels, baseChannels)

	// Apply overrides
	for i := 0; i < UniverseSize; i++ {
		key := strconv.Itoa(universe) + ":" + strconv.Itoa(i+1)
		if val, ok := s.channelOverrides[key]; ok {
			outputChannels[i] = val
		}
	}

	return outputChannels
}

// markDirty marks a universe as having changes.
func (s *Service) markDirty(universe int) {
	s.isDirty = true
	s.dirtyUniverses[universe] = true
}

// SetChannelValue sets a channel value.
func (s *Service) SetChannelValue(universe, channel int, value byte) {
	s.mu.Lock()
	defer s.mu.Unlock()

	universeData := s.universes[universe]
	if universeData == nil || channel < 1 || channel > UniverseSize {
		return
	}

	currentValue := universeData[channel-1]
	if currentValue != value {
		universeData[channel-1] = value
		s.markDirty(universe)
		s.triggerHighRate()
	}
}

// SetChannelOverride sets a channel override value.
func (s *Service) SetChannelOverride(universe, channel int, value byte) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if channel < 1 || channel > UniverseSize {
		return
	}

	key := strconv.Itoa(universe) + ":" + strconv.Itoa(channel)
	currentValue, exists := s.channelOverrides[key]

	if !exists || currentValue != value {
		s.channelOverrides[key] = value
		s.markDirty(universe)
		s.triggerHighRate()
	}
}

// ClearChannelOverride removes a channel override.
func (s *Service) ClearChannelOverride(universe, channel int) {
	s.mu.Lock()
	defer s.mu.Unlock()

	key := strconv.Itoa(universe) + ":" + strconv.Itoa(channel)
	if _, exists := s.channelOverrides[key]; exists {
		delete(s.channelOverrides, key)
		s.markDirty(universe)
		s.triggerHighRate()
	}
}

// ClearAllOverrides removes all channel overrides.
func (s *Service) ClearAllOverrides() {
	s.mu.Lock()
	defer s.mu.Unlock()

	if len(s.channelOverrides) > 0 {
		// Mark affected universes as dirty
		affectedUniverses := make(map[int]bool)
		for key := range s.channelOverrides {
			// Parse universe from key "universe:channel"
			for i, c := range key {
				if c == ':' {
					if u, err := strconv.Atoi(key[:i]); err == nil {
						affectedUniverses[u] = true
					}
					break
				}
			}
		}

		s.channelOverrides = make(map[string]byte)

		for u := range affectedUniverses {
			s.markDirty(u)
		}
		s.triggerHighRate()
	}
}

// triggerHighRate immediately switches to high rate mode.
func (s *Service) triggerHighRate() {
	s.lastChangeTime = time.Now()
	if !s.isInHighRateMode {
		s.isInHighRateMode = true
		s.currentRate = s.refreshRateHz
		log.Printf("📡 DMX transmission: switching to high rate (%dHz) - active fade/transition", s.refreshRateHz)
	}
}

// TriggerChangeDetection manually triggers high-rate mode (useful for fades).
// This switches to high-rate transmission mode but lets the transmitLoop
// handle actual packet sending to avoid race conditions and duplicate transmissions.
func (s *Service) TriggerChangeDetection() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.triggerHighRate()

	// Note: We do NOT immediately transmit here to avoid race conditions
	// with the transmitLoop. The transmitLoop will pick up changes on its
	// next scheduled transmission at the high refresh rate.
}

// ForceImmediateTransmission forces an immediate Art-Net transmission.
// This is used when we need to ensure the first frame of a fade is sent
// immediately without waiting for the next transmitLoop tick.
func (s *Service) ForceImmediateTransmission() {
	s.mu.Lock()

	wasInIdleMode := !s.isInHighRateMode
	s.triggerHighRate()

	// Mark everything as dirty to ensure transmission
	s.isDirty = true
	for universe := range s.universes {
		s.dirtyUniverses[universe] = true
	}

	// Immediately send Art-Net packets
	if s.enabled && s.conn != nil {
		s.outputDMX()
	}

	s.mu.Unlock()

	// If we were in idle mode, signal the transmitLoop to reset ticker immediately
	// This ensures the next frame is sent at 60Hz, not 1Hz
	if wasInIdleMode {
		select {
		case s.resetTickerChan <- struct{}{}:
			// Signal sent successfully
		default:
			// Channel already has a pending signal, no need to send another
		}
	}
}

// GetChannelValue returns the current value of a channel.
func (s *Service) GetChannelValue(universe, channel int) byte {
	s.mu.RLock()
	defer s.mu.RUnlock()

	universeData := s.universes[universe]
	if universeData == nil || channel < 1 || channel > UniverseSize {
		return 0
	}

	return universeData[channel-1]
}

// GetUniverse returns all channel values for a universe (as ints for API compatibility).
func (s *Service) GetUniverse(universe int) []int {
	s.mu.RLock()
	defer s.mu.RUnlock()

	channels := s.getUniverseOutputChannels(universe)
	result := make([]int, UniverseSize)
	for i, v := range channels {
		result[i] = int(v)
	}
	return result
}

// GetAllUniverses returns all universes with channel values (1-indexed).
func (s *Service) GetAllUniverses() map[int][]int {
	s.mu.RLock()
	defer s.mu.RUnlock()

	result := make(map[int][]int)
	for universe := range s.universes {
		channels := s.getUniverseOutputChannels(universe)
		intChannels := make([]int, UniverseSize)
		for i, v := range channels {
			intChannels[i] = int(v)
		}
		result[universe] = intChannels
	}
	return result
}

// SetAllChannels sets all channels in a universe.
func (s *Service) SetAllChannels(universe int, values []byte) {
	s.mu.Lock()
	defer s.mu.Unlock()

	universeData := s.universes[universe]
	if universeData == nil {
		return
	}

	changed := false
	for i := 0; i < UniverseSize && i < len(values); i++ {
		if universeData[i] != values[i] {
			universeData[i] = values[i]
			changed = true
		}
	}

	if changed {
		s.markDirty(universe)
		s.triggerHighRate()
	}
}

// FadeToBlack sets all channels to 0 (immediate, no fade).
func (s *Service) FadeToBlack() {
	s.mu.Lock()
	defer s.mu.Unlock()

	for universe, channels := range s.universes {
		changed := false
		for i := range channels {
			if channels[i] != 0 {
				channels[i] = 0
				changed = true
			}
		}
		if changed {
			s.markDirty(universe)
		}
	}

	// Clear active scene
	s.activeSceneID = nil

	// Clear all overrides
	s.channelOverrides = make(map[string]byte)

	s.triggerHighRate()
}

// SetActiveScene sets the currently active scene ID.
func (s *Service) SetActiveScene(sceneID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.activeSceneID = &sceneID
}

// GetActiveSceneID returns the currently active scene ID.
func (s *Service) GetActiveSceneID() *string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.activeSceneID
}

// ClearActiveScene clears the active scene.
func (s *Service) ClearActiveScene() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.activeSceneID = nil
}

// IsEnabled returns whether DMX output is enabled.
func (s *Service) IsEnabled() bool {
	return s.enabled
}

// GetBroadcastAddress returns the Art-Net broadcast address.
func (s *Service) GetBroadcastAddress() string {
	return s.broadcastAddr
}

// IsActive returns whether DMX output is currently active.
func (s *Service) IsActive() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.isInHighRateMode
}

// GetCurrentRate returns the current transmission rate in Hz.
func (s *Service) GetCurrentRate() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.currentRate
}

// CountActiveChannels returns the number of non-zero channels.
func (s *Service) CountActiveChannels() int {
	s.mu.RLock()
	defer s.mu.RUnlock()

	count := 0
	for universe := range s.universes {
		channels := s.getUniverseOutputChannels(universe)
		for _, v := range channels {
			if v > 0 {
				count++
			}
		}
	}
	return count
}

// Stop stops the DMX service and closes the socket.
func (s *Service) Stop() {
	s.mu.Lock()
	defer s.mu.Unlock()

	if !s.running {
		return
	}

	// Signal the transmission loop to stop
	close(s.stopChan)
	s.running = false

	// Send final blackout packet
	if s.enabled && s.conn != nil {
		for universe := range s.universes {
			s.universes[universe] = make([]byte, UniverseSize) // All zeros
			s.sequence++
			packet := artnet.BuildDMXPacket(universe, s.universes[universe], s.sequence)
			_, _ = s.conn.Write(packet)
		}

		_ = s.conn.Close()
		s.conn = nil
	}

	log.Printf("🎭 DMX Service stopped")
}

// ReloadBroadcastAddress updates the broadcast address and reconnects.
// If Art-Net was disabled, this will enable it.
func (s *Service) ReloadBroadcastAddress(newAddress string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	wasEnabled := s.enabled
	log.Printf("🔄 Reloading Art-Net broadcast address from %s to %s (was enabled: %v)", s.broadcastAddr, newAddress, wasEnabled)

	// Close existing connection
	if s.conn != nil {
		_ = s.conn.Close()
		s.conn = nil
	}

	// Update address
	s.broadcastAddr = newAddress

	// Create new connection
	addr, err := net.ResolveUDPAddr("udp4", s.broadcastAddr+":"+strconv.Itoa(s.port))
	if err != nil {
		return err
	}
	s.addr = addr

	conn, err := net.DialUDP("udp4", nil, addr)
	if err != nil {
		return err
	}
	s.conn = conn

	// Enable Art-Net output now that we have a valid broadcast address
	if !wasEnabled {
		s.enabled = true
		log.Printf("✅ Art-Net enabled with broadcast address %s:%d", s.broadcastAddr, s.port)
	} else {
		log.Printf("✅ Art-Net broadcast address updated to %s:%d", s.broadcastAddr, s.port)
	}
	return nil
}

// DisableArtNet disables Art-Net output and closes the connection.
func (s *Service) DisableArtNet() {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.conn != nil {
		_ = s.conn.Close()
		s.conn = nil
	}
	s.enabled = false
	s.broadcastAddr = ""
	log.Printf("🔌 Art-Net output disabled")
}
