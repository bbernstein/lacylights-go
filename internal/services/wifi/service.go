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
	return exec.Command(name, args...).Output()
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
	s.mu.RLock()
	defer s.mu.RUnlock()

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

	return status, nil
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

	// Notify status change
	s.notifyStatusChange()

	return s.GetStatus(ctx)
}

// ScanNetworks scans for available WiFi networks.
func (s *Service) ScanNetworks(ctx context.Context, rescan bool, deduplicate bool) ([]Network, error) {
	if runtime.GOOS != "linux" {
		return []Network{}, nil
	}

	// Trigger a rescan if requested
	if rescan {
		_, err := s.executor.Execute("nmcli", "device", "wifi", "rescan")
		if err != nil {
			log.Printf("WiFi rescan failed (may be rate-limited): %v", err)
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

	// Check if AP connection is active
	output, err := s.executor.Execute("nmcli", "-t", "-f", "NAME,DEVICE,STATE", "connection", "show", "--active")
	if err != nil {
		log.Printf("Failed to detect WiFi mode: %v", err)
		s.mode = ModeClient
		return
	}

	if strings.Contains(string(output), APConnectionName) {
		s.mode = ModeAP
		// TODO: Restore AP config from active connection
	} else {
		s.mode = ModeClient
	}
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
			return strings.Contains(line, "connected")
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

func (s *Service) notifyStatusChange() {
	if s.statusCallback != nil {
		status, _ := s.GetStatus(context.Background())
		go s.statusCallback(status)
	}
}

// RefreshAPClients updates the list of connected AP clients.
func (s *Service) RefreshAPClients() {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.mode != ModeAP {
		return
	}

	// Read DHCP leases
	output, err := s.executor.Execute("cat", "/var/lib/misc/dnsmasq.leases")
	if err != nil {
		log.Printf("Failed to read DHCP leases: %v", err)
		return
	}

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

func stringPtr(s string) *string {
	return &s
}
