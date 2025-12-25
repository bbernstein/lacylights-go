// Package wifi provides WiFi management and AP mode capabilities for LacyLights.
package wifi

import (
	"context"
	"fmt"
	"log"
	"os/exec"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	// commandTimeout is the timeout for shell commands to prevent hanging.
	commandTimeout = 5 * time.Second
	// connectTimeout is a longer timeout for WiFi connect operations
	// which need time for authentication and DHCP.
	connectTimeout = 30 * time.Second
)

const (
	// DefaultAPTimeout is the default AP mode timeout in minutes.
	DefaultAPTimeout = 30
	// APConnectionName is the NetworkManager connection name for AP mode.
	APConnectionName = "LacyLights-AP"
	// APIPAddress is the IP address for the Pi in AP mode.
	APIPAddress = "192.168.4.1"
	// APChannel is the WiFi channel for AP mode.
	APChannel = 6
)

// Service manages WiFi operations including client and AP modes.
type Service struct {
	mu               sync.RWMutex
	mode             Mode
	apConfig         *APConfig
	connectedClients []APClient
	apStartTime      *time.Time
	apTimeoutMinutes int
	apTimer          *time.Timer
	wifiInterface    string

	// Callbacks for PubSub integration
	statusCallback func(*Status)
	modeCallback   func(Mode)

	// Command executor (for testing)
	executor CommandExecutor
}

// realExecutor implements CommandExecutor using actual shell commands.
type realExecutor struct{}

func (e *realExecutor) Execute(name string, args ...string) ([]byte, error) {
	return e.ExecuteWithTimeout(commandTimeout, name, args...)
}

func (e *realExecutor) ExecuteWithTimeout(timeout time.Duration, name string, args ...string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, name, args...)
	// Use CombinedOutput to capture both stdout and stderr
	output, err := cmd.CombinedOutput()
	if ctx.Err() == context.DeadlineExceeded {
		log.Printf("WiFi: command timed out after %v: %s %v", timeout, name, args)
		return nil, fmt.Errorf("command timed out: %s", name)
	}
	return output, err
}

// NewService creates a new WiFi service.
func NewService() *Service {
	s := &Service{
		mode:             ModeClient,
		apTimeoutMinutes: DefaultAPTimeout,
		wifiInterface:    "wlan0",
		executor:         &realExecutor{},
	}

	// Detect initial mode
	go func() {
		time.Sleep(100 * time.Millisecond)
		s.detectCurrentMode()
	}()

	return s
}

// SetExecutor sets the command executor (for testing).
func (s *Service) SetExecutor(executor CommandExecutor) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.executor = executor
}

// SetStatusCallback sets the callback for status updates.
func (s *Service) SetStatusCallback(callback func(*Status)) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.statusCallback = callback
}

// SetModeCallback sets the callback for mode changes.
func (s *Service) SetModeCallback(callback func(Mode)) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.modeCallback = callback
}

// GetMode returns the current WiFi mode.
func (s *Service) GetMode() Mode {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.mode
}

// GetStatus returns the current WiFi status.
func (s *Service) GetStatus(ctx context.Context) (*Status, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	return s.getStatusLocked(), nil
}

// getStatusLocked builds the status. Caller must hold s.mu lock.
func (s *Service) getStatusLocked() *Status {
	// Update minutes remaining before returning status
	s.updateAPMinutesRemaining()

	// Refresh connected clients if in AP mode
	if s.mode == ModeAP {
		s.refreshAPClientsLocked()
	}

	status := &Status{
		Available:        s.isWiFiAvailable(),
		Enabled:          s.isWiFiEnabled(),
		Connected:        s.mode == ModeClient && s.isConnected(),
		Mode:             s.mode,
		APConfig:         s.apConfig,
		ConnectedClients: s.connectedClients,
	}

	// Get client mode details if connected
	if s.mode == ModeClient && status.Connected {
		s.fillClientStatus(status)
	}

	return status
}

// GetAPConfig returns the current AP configuration.
func (s *Service) GetAPConfig() *APConfig {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Update minutes remaining before returning
	s.updateAPMinutesRemaining()

	return s.apConfig
}

// GetAPClients returns the list of clients connected to the AP.
func (s *Service) GetAPClients() []APClient {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.connectedClients
}

// SetWiFiEnabled enables or disables the WiFi radio.
func (s *Service) SetWiFiEnabled(ctx context.Context, enabled bool) (*Status, error) {
	if runtime.GOOS != "linux" {
		return s.GetStatus(ctx)
	}

	var arg string
	if enabled {
		arg = "on"
	} else {
		arg = "off"
	}

	_, err := s.executor.Execute("nmcli", "radio", "wifi", arg)
	if err != nil {
		log.Printf("Failed to set WiFi enabled=%v: %v", enabled, err)
		return nil, fmt.Errorf("failed to set WiFi enabled: %w", err)
	}

	log.Printf("WiFi radio set to %s", arg)

	// Give NetworkManager a moment to update status
	time.Sleep(500 * time.Millisecond)

	// Notify status change and return status (both require lock)
	s.mu.Lock()
	s.notifyStatusChange()
	status := s.getStatusLocked()
	s.mu.Unlock()

	return status, nil
}

// ScanNetworks scans for available WiFi networks.
// Note: Scanning may work in AP mode on some WiFi adapters. We attempt it
// regardless of mode, relying on command timeouts to prevent hanging.
func (s *Service) ScanNetworks(ctx context.Context, rescan bool, deduplicate bool) ([]Network, error) {
	if runtime.GOOS != "linux" {
		return []Network{}, nil
	}

	// Trigger a rescan if requested
	if rescan {
		_, err := s.executor.Execute("nmcli", "device", "wifi", "rescan")
		if err != nil {
			log.Printf("WiFi rescan failed (may be rate-limited or in AP mode): %v", err)
			// Continue anyway - we can still list cached results
		}
		// Give the scan a moment to complete
		time.Sleep(2 * time.Second)
	}

	// Get list of available networks
	// Format: SSID:SIGNAL:SECURITY:FREQ:IN-USE
	output, err := s.executor.Execute("nmcli", "-t", "-f", "SSID,SIGNAL,SECURITY,FREQ,IN-USE", "device", "wifi", "list")
	if err != nil {
		log.Printf("Failed to list WiFi networks: %v", err)
		return nil, fmt.Errorf("failed to list networks: %w", err)
	}

	var networks []Network
	seenSSIDs := make(map[string]bool)
	lines := strings.Split(string(output), "\n")

	for _, line := range lines {
		if line == "" {
			continue
		}

		// Parse the colon-separated fields
		// Note: SSID may contain colons, so we need to be careful
		parts := strings.Split(line, ":")
		if len(parts) < 5 {
			continue
		}

		// Last 4 fields are SIGNAL, SECURITY, FREQ, IN-USE
		// Everything before that is the SSID (which may contain colons)
		inUse := parts[len(parts)-1]
		freq := parts[len(parts)-2]
		security := parts[len(parts)-3]
		signalStr := parts[len(parts)-4]
		ssid := strings.Join(parts[:len(parts)-4], ":")

		// Skip empty SSIDs (hidden networks)
		if ssid == "" {
			continue
		}

		// Deduplicate by SSID if requested
		if deduplicate {
			if seenSSIDs[ssid] {
				continue
			}
			seenSSIDs[ssid] = true
		}

		// Parse signal strength (ignore error - default to 0 if parsing fails)
		signal := 0
		_, _ = fmt.Sscanf(signalStr, "%d", &signal)

		network := Network{
			SSID:           ssid,
			SignalStrength: signal,
			Frequency:      freq,
			Security:       parseSecurityType(security),
			InUse:          inUse == "*",
		}

		networks = append(networks, network)
	}

	return networks, nil
}

// ConnectToNetwork connects to a WiFi network.
func (s *Service) ConnectToNetwork(ctx context.Context, ssid string, password *string) (*ConnectionResult, error) {
	if runtime.GOOS != "linux" {
		return &ConnectionResult{
			Success:   false,
			Message:   stringPtr("WiFi management only available on Linux"),
			Connected: false,
		}, nil
	}

	s.mu.Lock()
	wasInAPMode := s.mode == ModeAP
	// If in AP mode, stop it first
	if wasInAPMode {
		s.mu.Unlock()
		_, err := s.StopAPMode(ctx, nil)
		if err != nil {
			return &ConnectionResult{
				Success:   false,
				Message:   stringPtr(fmt.Sprintf("Failed to stop AP mode: %v", err)),
				Connected: false,
			}, nil
		}
		// Give NetworkManager time to fully release the interface after AP mode
		log.Printf("Waiting for interface to be released after AP mode...")
		time.Sleep(3 * time.Second)
		s.mu.Lock()
	}

	s.mode = ModeConnecting
	s.notifyModeChange()
	s.mu.Unlock()

	// Delete any existing connection for this SSID to avoid corrupted profiles
	// This is safe - if no connection exists, nmcli just returns an error we ignore
	_, _ = s.executor.Execute("nmcli", "connection", "delete", ssid)

	// Try to connect using nmcli
	var args []string
	if password != nil && *password != "" {
		// Connect with password - use explicit wifi-sec settings to avoid key-mgmt errors
		args = []string{
			"device", "wifi", "connect", ssid,
			"password", *password,
			"--",
			"wifi-sec.key-mgmt", "wpa-psk",
		}
	} else {
		// Connect without password (open network or saved credentials)
		args = []string{"device", "wifi", "connect", ssid}
	}

	// Use longer timeout for connect - it needs time for auth and DHCP
	// Retry up to 3 times since first attempt after AP mode may fail
	var output []byte
	var err error
	maxRetries := 1
	if wasInAPMode {
		maxRetries = 3 // More retries needed when transitioning from AP mode
	}

	for attempt := 1; attempt <= maxRetries; attempt++ {
		log.Printf("WiFi connect attempt %d/%d for SSID: %s", attempt, maxRetries, ssid)
		output, err = s.executor.ExecuteWithTimeout(connectTimeout, "nmcli", args...)
		if err == nil {
			break
		}
		log.Printf("Connect attempt %d failed: %v, output: %s", attempt, err, string(output))

		// Check if we're actually connected despite the error
		if s.isConnected() {
			log.Printf("Actually connected despite error - treating as success")
			err = nil
			break
		}

		if attempt < maxRetries {
			log.Printf("Waiting before retry...")
			time.Sleep(2 * time.Second)
		}
	}

	if err != nil {
		// Final check - are we actually connected?
		if s.isConnected() {
			log.Printf("Connected despite nmcli error")
		} else {
			log.Printf("Failed to connect to WiFi network %s after %d attempts", ssid, maxRetries)
			s.mu.Lock()
			s.mode = ModeClient
			s.notifyModeChange()
			s.mu.Unlock()

			// Parse error message from output
			errMsg := strings.TrimSpace(string(output))
			if errMsg == "" {
				errMsg = err.Error()
			}
			return &ConnectionResult{
				Success:   false,
				Message:   stringPtr(fmt.Sprintf("Connection failed: %s", errMsg)),
				Connected: false,
			}, nil
		}
	}

	// Give NetworkManager a moment to fully establish connection
	time.Sleep(1 * time.Second)

	s.mu.Lock()
	s.mode = ModeClient
	s.notifyModeChange()
	s.notifyStatusChange()
	s.mu.Unlock()

	log.Printf("Successfully connected to WiFi network: %s", ssid)

	return &ConnectionResult{
		Success:   true,
		Message:   stringPtr(fmt.Sprintf("Connected to %s", ssid)),
		Connected: true,
	}, nil
}

// Disconnect disconnects from the current WiFi network.
func (s *Service) Disconnect(ctx context.Context) (*ConnectionResult, error) {
	if runtime.GOOS != "linux" {
		return &ConnectionResult{
			Success:   false,
			Message:   stringPtr("WiFi management only available on Linux"),
			Connected: false,
		}, nil
	}

	// Disconnect the WiFi interface
	_, err := s.executor.Execute("nmcli", "device", "disconnect", s.wifiInterface)
	if err != nil {
		log.Printf("Failed to disconnect WiFi: %v", err)
		return &ConnectionResult{
			Success:   false,
			Message:   stringPtr(fmt.Sprintf("Disconnect failed: %v", err)),
			Connected: s.isConnected(),
		}, nil
	}

	// Notify status change
	s.mu.Lock()
	s.notifyStatusChange()
	s.mu.Unlock()

	log.Printf("Disconnected from WiFi")

	return &ConnectionResult{
		Success:   true,
		Message:   stringPtr("Disconnected from WiFi"),
		Connected: false,
	}, nil
}

// ForgetNetwork removes a saved WiFi network.
func (s *Service) ForgetNetwork(ctx context.Context, ssid string) (bool, error) {
	if runtime.GOOS != "linux" {
		return false, nil
	}

	// Delete the connection by name
	_, err := s.executor.Execute("nmcli", "connection", "delete", ssid)
	if err != nil {
		log.Printf("Failed to forget WiFi network %s: %v", ssid, err)
		return false, nil
	}

	log.Printf("Forgot WiFi network: %s", ssid)
	return true, nil
}

// parseSecurityType converts nmcli security string to SecurityType.
func parseSecurityType(security string) SecurityType {
	security = strings.ToUpper(security)
	switch {
	case strings.Contains(security, "WPA3") && strings.Contains(security, "EAP"):
		return SecurityWPA3EAP
	case strings.Contains(security, "WPA3"):
		return SecurityWPA3PSK
	case strings.Contains(security, "WPA") && strings.Contains(security, "EAP"):
		return SecurityWPAEAP
	case strings.Contains(security, "WPA"):
		return SecurityWPAPSK
	case strings.Contains(security, "WEP"):
		return SecurityWEP
	case strings.Contains(security, "OWE"):
		return SecurityOWE
	case security == "" || security == "--":
		return SecurityOpen
	default:
		return SecurityWPAPSK // Default to WPA-PSK for unknown
	}
}

// StartAPMode starts the access point mode.
func (s *Service) StartAPMode(ctx context.Context) (*ModeResult, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Check if already in AP mode
	if s.mode == ModeAP {
		return &ModeResult{
			Success: true,
			Message: stringPtr("Already in AP mode"),
			Mode:    ModeAP,
		}, nil
	}

	// Check platform
	if runtime.GOOS != "linux" {
		return &ModeResult{
			Success: false,
			Message: stringPtr("AP mode is only supported on Linux"),
			Mode:    s.mode,
		}, nil
	}

	s.mode = ModeStartingAP
	s.notifyModeChange()

	// Generate SSID from MAC address
	ssid := s.generateAPSSID()

	// Create and activate AP connection
	if err := s.createAPConnection(ssid); err != nil {
		log.Printf("Failed to create AP connection: %v", err)
		s.mode = ModeClient
		s.notifyModeChange()
		return &ModeResult{
			Success: false,
			Message: stringPtr(fmt.Sprintf("Failed to create AP: %v", err)),
			Mode:    s.mode,
		}, nil
	}

	if err := s.activateAPConnection(); err != nil {
		log.Printf("Failed to activate AP connection: %v", err)
		s.mode = ModeClient
		s.notifyModeChange()
		return &ModeResult{
			Success: false,
			Message: stringPtr(fmt.Sprintf("Failed to start AP: %v", err)),
			Mode:    s.mode,
		}, nil
	}

	// Update state
	now := time.Now()
	s.apStartTime = &now
	s.mode = ModeAP
	s.apConfig = &APConfig{
		SSID:           ssid,
		IPAddress:      APIPAddress,
		Channel:        APChannel,
		ClientCount:    0,
		TimeoutMinutes: s.apTimeoutMinutes,
	}

	// Start timeout timer
	s.startAPTimer()

	// Notify callbacks
	s.notifyModeChange()
	s.notifyStatusChange()

	log.Printf("AP mode started with SSID: %s", ssid)

	return &ModeResult{
		Success: true,
		Message: stringPtr(fmt.Sprintf("AP mode started with SSID: %s", ssid)),
		Mode:    ModeAP,
	}, nil
}

// StopAPMode stops the access point mode and optionally connects to a network.
func (s *Service) StopAPMode(ctx context.Context, connectToSSID *string) (*ModeResult, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Check if in AP mode
	if s.mode != ModeAP {
		return &ModeResult{
			Success: true,
			Message: stringPtr("Not in AP mode"),
			Mode:    s.mode,
		}, nil
	}

	// Stop AP timer
	if s.apTimer != nil {
		s.apTimer.Stop()
		s.apTimer = nil
	}

	// Deactivate AP connection
	if err := s.deactivateAPConnection(); err != nil {
		log.Printf("Failed to deactivate AP connection: %v", err)
		// Continue anyway - try to connect to target network
	}

	s.apConfig = nil
	s.apStartTime = nil
	s.connectedClients = nil

	// Connect to target network if specified
	if connectToSSID != nil && *connectToSSID != "" {
		s.mode = ModeConnecting
		s.notifyModeChange()

		// This would be implemented in client.go
		log.Printf("Would connect to SSID: %s", *connectToSSID)
	}

	s.mode = ModeClient
	s.notifyModeChange()
	s.notifyStatusChange()

	log.Printf("AP mode stopped")

	return &ModeResult{
		Success: true,
		Message: stringPtr("AP mode stopped"),
		Mode:    ModeClient,
	}, nil
}

// ResetAPTimeout resets the AP mode timeout timer.
func (s *Service) ResetAPTimeout(ctx context.Context) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.mode != ModeAP {
		return false, nil
	}

	// Restart timer
	if s.apTimer != nil {
		s.apTimer.Stop()
	}
	s.startAPTimer()

	// Update start time for minutes remaining calculation
	now := time.Now()
	s.apStartTime = &now

	log.Printf("AP timeout reset to %d minutes", s.apTimeoutMinutes)

	return true, nil
}

// Internal methods

func (s *Service) detectCurrentMode() {
	s.mu.Lock()
	defer s.mu.Unlock()

	if runtime.GOOS != "linux" {
		s.mode = ModeClient
		return
	}

	// Check if wlan0 is in AP mode by looking at the connection type
	// nmcli -t -f NAME,TYPE,DEVICE connection show --active
	output, err := s.executor.Execute("nmcli", "-t", "-f", "NAME,TYPE,DEVICE", "connection", "show", "--active")
	if err != nil {
		log.Printf("Failed to detect WiFi mode: %v", err)
		s.mode = ModeClient
		return
	}

	// Check for AP mode connections on wlan0
	// Also check for connections with "lacylights" prefix (our AP naming convention)
	lines := strings.Split(string(output), "\n")
	for _, line := range lines {
		if strings.Contains(line, s.wifiInterface) {
			// Check if it's our AP connection (starts with "lacylights-")
			if strings.HasPrefix(line, "lacylights-") {
				s.mode = ModeAP
				// Extract SSID from connection name
				parts := strings.Split(line, ":")
				if len(parts) >= 1 {
					ssid := parts[0]
					// Set apStartTime to now - gives a fresh timeout from server start
					now := time.Now()
					s.apStartTime = &now
					s.apConfig = &APConfig{
						SSID:           ssid,
						IPAddress:      APIPAddress,
						Channel:        APChannel,
						TimeoutMinutes: s.apTimeoutMinutes,
					}
					// Start the AP timeout timer
					s.startAPTimer()
					log.Printf("Detected existing AP mode with SSID: %s, timeout reset to %d minutes", ssid, s.apTimeoutMinutes)
				}
				return
			}
		}
	}

	s.mode = ModeClient
}

func (s *Service) isWiFiAvailable() bool {
	if runtime.GOOS != "linux" {
		log.Printf("WiFi: Not on Linux (GOOS=%s), WiFi unavailable", runtime.GOOS)
		return false
	}

	output, err := s.executor.Execute("nmcli", "-t", "-f", "DEVICE,TYPE", "device", "status")
	if err != nil {
		log.Printf("WiFi: nmcli command failed: %v", err)
		return false
	}

	hasWifi := strings.Contains(string(output), "wifi")
	if !hasWifi {
		log.Printf("WiFi: No wifi device found in nmcli output: %q", string(output))
	}
	return hasWifi
}

func (s *Service) isWiFiEnabled() bool {
	if runtime.GOOS != "linux" {
		return false
	}

	output, err := s.executor.Execute("nmcli", "radio", "wifi")
	if err != nil {
		return false
	}

	return strings.TrimSpace(string(output)) == "enabled"
}

func (s *Service) isConnected() bool {
	if runtime.GOOS != "linux" {
		return false
	}

	output, err := s.executor.Execute("nmcli", "-t", "-f", "DEVICE,STATE", "device", "status")
	if err != nil {
		return false
	}

	lines := strings.Split(string(output), "\n")
	for _, line := range lines {
		if strings.HasPrefix(line, s.wifiInterface+":") {
			// Check for ":connected" to avoid matching "disconnected"
			return strings.Contains(line, ":connected")
		}
	}

	return false
}

func (s *Service) fillClientStatus(status *Status) {
	// Get current SSID
	output, err := s.executor.Execute("nmcli", "-t", "-f", "GENERAL.CONNECTION", "device", "show", s.wifiInterface)
	if err == nil {
		for _, line := range strings.Split(string(output), "\n") {
			if strings.HasPrefix(line, "GENERAL.CONNECTION:") {
				ssid := strings.TrimPrefix(line, "GENERAL.CONNECTION:")
				status.SSID = &ssid
				break
			}
		}
	}

	// Get IP address
	output, err = s.executor.Execute("nmcli", "-t", "-f", "IP4.ADDRESS", "device", "show", s.wifiInterface)
	if err == nil {
		for _, line := range strings.Split(string(output), "\n") {
			if strings.HasPrefix(line, "IP4.ADDRESS") {
				parts := strings.Split(line, ":")
				if len(parts) > 1 {
					ip := strings.Split(parts[1], "/")[0]
					status.IPAddress = &ip
					break
				}
			}
		}
	}

	// Get MAC address
	output, err = s.executor.Execute("nmcli", "-t", "-f", "GENERAL.HWADDR", "device", "show", s.wifiInterface)
	if err == nil {
		for _, line := range strings.Split(string(output), "\n") {
			if strings.HasPrefix(line, "GENERAL.HWADDR:") {
				mac := strings.TrimPrefix(line, "GENERAL.HWADDR:")
				status.MACAddress = &mac
				break
			}
		}
	}
}

func (s *Service) generateAPSSID() string {
	// Get MAC address
	output, err := s.executor.Execute("cat", "/sys/class/net/"+s.wifiInterface+"/address")
	if err != nil {
		// Fallback to random suffix
		return fmt.Sprintf("lacylights-%04x", time.Now().UnixNano()&0xFFFF)
	}

	mac := strings.TrimSpace(string(output))
	// Remove colons and get last 4 characters
	mac = strings.ReplaceAll(mac, ":", "")
	if len(mac) >= 4 {
		return "lacylights-" + strings.ToUpper(mac[len(mac)-4:])
	}

	return "lacylights-" + strings.ToUpper(mac)
}

func (s *Service) createAPConnection(ssid string) error {
	// Check if connection already exists
	output, _ := s.executor.Execute("nmcli", "connection", "show", ssid)
	if len(output) > 0 {
		// Connection exists, just update it
		return nil
	}

	// Create new AP connection
	args := []string{
		"connection", "add",
		"type", "wifi",
		"ifname", s.wifiInterface,
		"con-name", ssid,
		"autoconnect", "no",
		"ssid", ssid,
		"mode", "ap",
		"ipv4.method", "shared",
		"ipv4.addresses", APIPAddress + "/24",
		"wifi.band", "bg",
		"wifi.channel", strconv.Itoa(APChannel),
	}

	_, err := s.executor.Execute("nmcli", args...)
	return err
}

func (s *Service) activateAPConnection() error {
	var ssid string
	if s.apConfig != nil {
		ssid = s.apConfig.SSID
	} else {
		ssid = s.generateAPSSID()
	}

	_, err := s.executor.Execute("nmcli", "connection", "up", ssid)
	return err
}

func (s *Service) deactivateAPConnection() error {
	if s.apConfig == nil {
		return nil
	}

	_, err := s.executor.Execute("nmcli", "connection", "down", s.apConfig.SSID)
	return err
}

func (s *Service) startAPTimer() {
	if s.apTimer != nil {
		s.apTimer.Stop()
	}

	timeout := time.Duration(s.apTimeoutMinutes) * time.Minute
	s.apTimer = time.AfterFunc(timeout, func() {
		log.Printf("AP mode timeout reached after %d minutes", s.apTimeoutMinutes)
		ctx := context.Background()
		_, err := s.StopAPMode(ctx, nil)
		if err != nil {
			log.Printf("Failed to stop AP mode after timeout: %v", err)
		}
	})
}

func (s *Service) updateAPMinutesRemaining() {
	if s.apConfig == nil || s.apStartTime == nil {
		return
	}

	elapsed := time.Since(*s.apStartTime)
	remaining := s.apTimeoutMinutes - int(elapsed.Minutes())
	if remaining < 0 {
		remaining = 0
	}
	s.apConfig.MinutesRemaining = &remaining
}

func (s *Service) notifyModeChange() {
	if s.modeCallback != nil {
		go s.modeCallback(s.mode)
	}
}

// notifyStatusChange notifies status callback. Caller must hold s.mu lock.
func (s *Service) notifyStatusChange() {
	if s.statusCallback != nil {
		status := s.getStatusLocked()
		go s.statusCallback(status)
	}
}

// RefreshAPClients updates the list of connected AP clients.
func (s *Service) RefreshAPClients() {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.refreshAPClientsLocked()
}

// refreshAPClientsLocked updates the client list. Caller must hold s.mu lock.
func (s *Service) refreshAPClientsLocked() {
	if s.mode != ModeAP {
		return
	}

	// Try multiple possible DHCP lease file locations
	// NetworkManager with shared connections uses different paths
	leaseFiles := []string{
		"/var/lib/NetworkManager/dnsmasq-" + s.wifiInterface + ".leases",
		"/var/lib/misc/dnsmasq.leases",
		"/run/NetworkManager/dnsmasq-" + s.wifiInterface + ".leases",
	}

	var output []byte
	var err error
	var foundFile string
	for _, file := range leaseFiles {
		output, err = s.executor.Execute("cat", file)
		if err == nil && len(output) > 0 {
			foundFile = file
			break
		}
	}

	if foundFile == "" {
		// No leases file found - try to get clients from arp table as fallback
		output, err = s.executor.Execute("arp", "-n")
		if err != nil {
			log.Printf("Failed to read DHCP leases and arp table")
			return
		}
		s.parseArpTable(output)
		return
	}

	log.Printf("Reading DHCP leases from: %s", foundFile)

	var clients []APClient
	lines := strings.Split(string(output), "\n")
	leaseRegex := regexp.MustCompile(`^(\d+)\s+([0-9a-f:]+)\s+(\d+\.\d+\.\d+\.\d+)\s+(\S+)`)

	for _, line := range lines {
		matches := leaseRegex.FindStringSubmatch(line)
		if len(matches) >= 5 {
			timestamp, _ := strconv.ParseInt(matches[1], 10, 64)
			connectedAt := time.Unix(timestamp, 0)
			ip := matches[3]
			hostname := matches[4]

			client := APClient{
				MACAddress:  matches[2],
				IPAddress:   &ip,
				Hostname:    &hostname,
				ConnectedAt: connectedAt,
			}
			clients = append(clients, client)
		}
	}

	s.connectedClients = clients
	if s.apConfig != nil {
		s.apConfig.ClientCount = len(clients)
	}
}

// parseArpTable parses the arp -n output to find connected clients.
// This is a fallback when DHCP lease files are not available.
// Only includes entries for our AP subnet (192.168.4.x).
func (s *Service) parseArpTable(output []byte) {
	var clients []APClient
	lines := strings.Split(string(output), "\n")
	// arp -n output format: Address HWtype HWaddress Flags Mask Iface
	// Example: 192.168.4.10 ether dc:a6:32:12:ab:cd C wlan0
	arpRegex := regexp.MustCompile(`^(192\.168\.4\.\d+)\s+\w+\s+([0-9a-f:]+)\s+\w+\s+\S*\s*` + s.wifiInterface)

	for _, line := range lines {
		matches := arpRegex.FindStringSubmatch(line)
		if len(matches) >= 3 {
			ip := matches[1]
			client := APClient{
				MACAddress:  matches[2],
				IPAddress:   &ip,
				ConnectedAt: time.Now(), // ARP doesn't have connection time
			}
			clients = append(clients, client)
		}
	}

	log.Printf("Found %d clients from ARP table", len(clients))
	s.connectedClients = clients
	if s.apConfig != nil {
		s.apConfig.ClientCount = len(clients)
	}
}

func stringPtr(s string) *string {
	return &s
}
