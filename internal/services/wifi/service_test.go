package wifi

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mockExecutor implements CommandExecutor for testing.
type mockExecutor struct {
	responses map[string][]byte
	errors    map[string]error
	calls     []string
}

func newMockExecutor() *mockExecutor {
	return &mockExecutor{
		responses: make(map[string][]byte),
		errors:    make(map[string]error),
		calls:     []string{},
	}
}

func (m *mockExecutor) Execute(name string, args ...string) ([]byte, error) {
	return m.ExecuteWithTimeout(0, name, args...)
}

func (m *mockExecutor) ExecuteWithTimeout(_ time.Duration, name string, args ...string) ([]byte, error) {
	key := name + " " + strings.Join(args, " ")
	m.calls = append(m.calls, key)

	if err, ok := m.errors[key]; ok {
		return nil, err
	}

	if resp, ok := m.responses[key]; ok {
		return resp, nil
	}

	// Return empty response by default
	return []byte{}, nil
}

func (m *mockExecutor) setResponse(cmd string, response string) {
	m.responses[cmd] = []byte(response)
}

func TestNewService(t *testing.T) {
	s := NewService()
	require.NotNil(t, s)
	// Wait for the detectCurrentMode goroutine to complete
	time.Sleep(150 * time.Millisecond)
	// Use GetMode() instead of accessing s.mode directly for thread safety
	assert.Equal(t, ModeClient, s.GetMode())
	assert.Equal(t, DefaultAPTimeout, s.apTimeoutMinutes)
	assert.Equal(t, "wlan0", s.wifiInterface)
}

func TestGetMode(t *testing.T) {
	s := NewService()

	// Wait for the detectCurrentMode goroutine to complete
	time.Sleep(150 * time.Millisecond)

	// Default mode is CLIENT
	mode := s.GetMode()
	assert.Equal(t, ModeClient, mode)

	// Change mode using lock for thread safety
	s.mu.Lock()
	s.mode = ModeAP
	s.mu.Unlock()
	mode = s.GetMode()
	assert.Equal(t, ModeAP, mode)
}

func TestGetStatus(t *testing.T) {
	s := NewService()
	mock := newMockExecutor()
	s.SetExecutor(mock)

	ctx := context.Background()
	status, err := s.GetStatus(ctx)

	require.NoError(t, err)
	require.NotNil(t, status)
	assert.Equal(t, ModeClient, status.Mode)
}

func TestGetAPConfig_NoAPMode(t *testing.T) {
	s := NewService()

	config := s.GetAPConfig()
	assert.Nil(t, config)
}

func TestGetAPClients_NoAPMode(t *testing.T) {
	s := NewService()

	clients := s.GetAPClients()
	assert.Nil(t, clients)
}

func TestGenerateAPSSID(t *testing.T) {
	s := NewService()
	mock := newMockExecutor()
	s.SetExecutor(mock)

	// Test with valid MAC address
	mock.setResponse("cat /sys/class/net/wlan0/address", "dc:a6:32:12:ab:cd")
	ssid := s.generateAPSSID()
	assert.Equal(t, "lacylights-ABCD", ssid)

	// Test with different MAC
	mock.setResponse("cat /sys/class/net/wlan0/address", "00:11:22:33:44:55")
	ssid = s.generateAPSSID()
	assert.Equal(t, "lacylights-4455", ssid)
}

func TestStartAPMode_NotLinux(t *testing.T) {
	// This test will only run on non-Linux systems
	s := NewService()
	mock := newMockExecutor()
	s.SetExecutor(mock)

	ctx := context.Background()
	result, err := s.StartAPMode(ctx)

	require.NoError(t, err)
	require.NotNil(t, result)

	// On non-Linux, should fail gracefully
	if result.Success {
		// If on Linux, this would succeed
		assert.Equal(t, ModeAP, result.Mode)
	} else {
		assert.Contains(t, *result.Message, "Linux")
	}
}

func TestStopAPMode_NotInAPMode(t *testing.T) {
	s := NewService()

	ctx := context.Background()
	result, err := s.StopAPMode(ctx, nil)

	require.NoError(t, err)
	require.NotNil(t, result)
	assert.True(t, result.Success)
	assert.Equal(t, ModeClient, result.Mode)
}

func TestResetAPTimeout_NotInAPMode(t *testing.T) {
	s := NewService()

	ctx := context.Background()
	success, err := s.ResetAPTimeout(ctx)

	require.NoError(t, err)
	assert.False(t, success)
}

func TestResetAPTimeout_InAPMode(t *testing.T) {
	s := NewService()
	s.mode = ModeAP
	s.apConfig = &APConfig{
		SSID:           "lacylights-TEST",
		TimeoutMinutes: 30,
	}
	now := time.Now().Add(-10 * time.Minute)
	s.apStartTime = &now

	ctx := context.Background()
	success, err := s.ResetAPTimeout(ctx)

	require.NoError(t, err)
	assert.True(t, success)

	// Start time should be reset to now
	assert.True(t, s.apStartTime.After(now))
}

func TestCallbacks(t *testing.T) {
	s := NewService()
	mock := newMockExecutor()
	s.SetExecutor(mock)

	// Use channels to synchronize callback verification
	modeChan := make(chan Mode, 1)
	statusChan := make(chan *Status, 1)

	s.SetModeCallback(func(m Mode) {
		modeChan <- m
	})

	s.SetStatusCallback(func(st *Status) {
		statusChan <- st
	})

	// Trigger mode callback and wait for result
	s.notifyModeChange()
	select {
	case calledMode := <-modeChan:
		assert.Equal(t, ModeClient, calledMode)
	case <-time.After(100 * time.Millisecond):
		t.Error("Mode callback timeout")
	}

	// Trigger status callback and wait for result
	s.mu.Lock()
	s.notifyStatusChangeLocked()
	s.mu.Unlock()
	select {
	case calledStatus := <-statusChan:
		assert.NotNil(t, calledStatus)
	case <-time.After(100 * time.Millisecond):
		t.Error("Status callback timeout")
	}
}

func TestRefreshAPClients(t *testing.T) {
	s := NewService()
	mock := newMockExecutor()
	s.SetExecutor(mock)

	// Not in AP mode - should do nothing
	s.RefreshAPClients()
	assert.Nil(t, s.connectedClients)

	// Set to AP mode
	s.mode = ModeAP
	s.apConfig = &APConfig{
		SSID: "lacylights-TEST",
	}

	// Mock DHCP leases
	mock.setResponse("cat /var/lib/misc/dnsmasq.leases",
		"1704067200 dc:a6:32:12:ab:cd 192.168.4.10 device1\n"+
			"1704067300 00:11:22:33:44:55 192.168.4.11 device2\n")

	s.RefreshAPClients()

	assert.Len(t, s.connectedClients, 2)
	assert.Equal(t, 2, s.apConfig.ClientCount)
	assert.Equal(t, "dc:a6:32:12:ab:cd", s.connectedClients[0].MACAddress)
	assert.Equal(t, "192.168.4.10", *s.connectedClients[0].IPAddress)
}

func TestModeConstants(t *testing.T) {
	assert.Equal(t, Mode("CLIENT"), ModeClient)
	assert.Equal(t, Mode("AP"), ModeAP)
	assert.Equal(t, Mode("DISABLED"), ModeDisabled)
	assert.Equal(t, Mode("CONNECTING"), ModeConnecting)
	assert.Equal(t, Mode("STARTING_AP"), ModeStartingAP)
}

func TestSecurityTypeConstants(t *testing.T) {
	assert.Equal(t, SecurityType("OPEN"), SecurityOpen)
	assert.Equal(t, SecurityType("WEP"), SecurityWEP)
	assert.Equal(t, SecurityType("WPA_PSK"), SecurityWPAPSK)
	assert.Equal(t, SecurityType("WPA_EAP"), SecurityWPAEAP)
	assert.Equal(t, SecurityType("WPA3_PSK"), SecurityWPA3PSK)
	assert.Equal(t, SecurityType("WPA3_EAP"), SecurityWPA3EAP)
	assert.Equal(t, SecurityType("OWE"), SecurityOWE)
}

func TestAPConfigDefaults(t *testing.T) {
	assert.Equal(t, 30, DefaultAPTimeout)
	assert.Equal(t, "LacyLights-AP", APConnectionName)
	assert.Equal(t, "192.168.4.1", APIPAddress)
	assert.Equal(t, 6, APChannel)
}

func TestSetWiFiEnabled_NotLinux(t *testing.T) {
	// This test will only run on non-Linux systems
	s := NewService()
	mock := newMockExecutor()
	s.SetExecutor(mock)

	ctx := context.Background()

	// On non-Linux, should return current status without error
	status, err := s.SetWiFiEnabled(ctx, true)
	require.NoError(t, err)
	require.NotNil(t, status)

	// Should not have called nmcli on non-Linux
	for _, call := range mock.calls {
		assert.NotContains(t, call, "nmcli radio wifi")
	}
}

func TestGetAPConfig_MinutesRemaining(t *testing.T) {
	s := NewService()

	// Set to AP mode with apStartTime
	s.mode = ModeAP
	startTime := time.Now().Add(-5 * time.Minute) // Started 5 minutes ago
	s.apStartTime = &startTime
	s.apConfig = &APConfig{
		SSID:           "lacylights-TEST",
		TimeoutMinutes: 30,
	}

	config := s.GetAPConfig()

	require.NotNil(t, config)
	require.NotNil(t, config.MinutesRemaining)
	// Should be approximately 25 minutes remaining (30 - 5)
	assert.InDelta(t, 25, *config.MinutesRemaining, 1)
}

func TestGetAPConfig_WithoutStartTime(t *testing.T) {
	s := NewService()

	// Set to AP mode without apStartTime
	s.mode = ModeAP
	s.apConfig = &APConfig{
		SSID:           "lacylights-TEST",
		TimeoutMinutes: 30,
	}

	config := s.GetAPConfig()

	require.NotNil(t, config)
	// MinutesRemaining should be nil when apStartTime is nil
	assert.Nil(t, config.MinutesRemaining)
}

func TestGetStatus_IncludesMinutesRemaining(t *testing.T) {
	s := NewService()
	mock := newMockExecutor()
	s.SetExecutor(mock)

	// Set to AP mode with apStartTime
	s.mode = ModeAP
	startTime := time.Now().Add(-5 * time.Minute) // Started 5 minutes ago
	s.apStartTime = &startTime
	s.apConfig = &APConfig{
		SSID:           "lacylights-TEST",
		TimeoutMinutes: 30,
	}

	ctx := context.Background()
	status, err := s.GetStatus(ctx)

	require.NoError(t, err)
	require.NotNil(t, status)
	require.NotNil(t, status.APConfig)
	require.NotNil(t, status.APConfig.MinutesRemaining)
	// Should be approximately 25 minutes remaining (30 - 5)
	assert.InDelta(t, 25, *status.APConfig.MinutesRemaining, 1)
}

func TestConnectToNetwork_NotLinux(t *testing.T) {
	s := NewService()
	mock := newMockExecutor()
	s.SetExecutor(mock)

	ctx := context.Background()
	result, err := s.ConnectToNetwork(ctx, "TestNetwork", nil)

	require.NoError(t, err)
	require.NotNil(t, result)
	// On non-Linux, should fail gracefully
	assert.False(t, result.Success)
	assert.Contains(t, *result.Message, "Linux")
}

func TestDisconnect_NotLinux(t *testing.T) {
	s := NewService()
	mock := newMockExecutor()
	s.SetExecutor(mock)

	ctx := context.Background()
	result, err := s.Disconnect(ctx)

	require.NoError(t, err)
	require.NotNil(t, result)
	// On non-Linux, should fail gracefully
	assert.False(t, result.Success)
	assert.Contains(t, *result.Message, "Linux")
}

func TestForgetNetwork_NotLinux(t *testing.T) {
	s := NewService()
	mock := newMockExecutor()
	s.SetExecutor(mock)

	ctx := context.Background()
	success, err := s.ForgetNetwork(ctx, "TestNetwork")

	require.NoError(t, err)
	// On non-Linux, should return false
	assert.False(t, success)
}
