// Package wifi provides WiFi management and AP mode capabilities for LacyLights.
package wifi

import (
	"time"
)

// NOTE: time package is used for APClient.ConnectedAt and CommandExecutor.ExecuteWithTimeout

// Mode represents the current WiFi operation mode.
type Mode string

const (
	// ModeClient indicates the device is connected to a WiFi network as a client.
	ModeClient Mode = "CLIENT"
	// ModeAP indicates the device is operating as an access point (hotspot).
	ModeAP Mode = "AP"
	// ModeDisabled indicates WiFi is disabled.
	ModeDisabled Mode = "DISABLED"
	// ModeConnecting indicates the device is attempting to connect to a network.
	ModeConnecting Mode = "CONNECTING"
	// ModeStartingAP indicates the device is starting AP mode.
	ModeStartingAP Mode = "STARTING_AP"
)

// APConfig represents the configuration of the access point.
type APConfig struct {
	SSID             string
	IPAddress        string
	Channel          int
	ClientCount      int
	TimeoutMinutes   int
	MinutesRemaining *int
}

// APClient represents a client connected to the access point.
type APClient struct {
	MACAddress  string
	IPAddress   *string
	Hostname    *string
	ConnectedAt time.Time
}

// Status represents the current WiFi status.
type Status struct {
	Available        bool
	Enabled          bool
	Connected        bool
	SSID             *string
	SignalStrength   *int
	IPAddress        *string
	MACAddress       *string
	Frequency        *string
	Mode             Mode
	APConfig         *APConfig
	ConnectedClients []APClient
}

// Network represents a WiFi network.
type Network struct {
	SSID           string
	SignalStrength int
	Frequency      string
	Security       SecurityType
	InUse          bool
	Saved          bool
}

// SecurityType represents the security type of a WiFi network.
type SecurityType string

const (
	SecurityOpen    SecurityType = "OPEN"
	SecurityWEP     SecurityType = "WEP"
	SecurityWPAPSK  SecurityType = "WPA_PSK"
	SecurityWPAEAP  SecurityType = "WPA_EAP"
	SecurityWPA3PSK SecurityType = "WPA3_PSK"
	SecurityWPA3EAP SecurityType = "WPA3_EAP"
	SecurityOWE     SecurityType = "OWE"
)

// ConnectionResult represents the result of a WiFi connection operation.
type ConnectionResult struct {
	Success   bool
	Message   *string
	Connected bool
}

// ModeResult represents the result of a mode change operation.
type ModeResult struct {
	Success bool
	Message *string
	Mode    Mode
}

// CommandExecutor interface for executing shell commands (for testing).
type CommandExecutor interface {
	Execute(name string, args ...string) ([]byte, error)
	ExecuteWithTimeout(timeout time.Duration, name string, args ...string) ([]byte, error)
}
