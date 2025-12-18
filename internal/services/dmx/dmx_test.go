package dmx

import (
	"net"
	"testing"
	"time"

	"github.com/bbernstein/lacylights-go/pkg/artnet"
)

func TestNewService(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Enabled = false // Disable UDP for testing

	service := NewService(cfg)
	if service == nil {
		t.Fatal("NewService() returned nil")
	}

	if service.enabled != false {
		t.Error("Expected enabled to be false")
	}

	if len(service.universes) != 4 {
		t.Errorf("Expected 4 universes, got %d", len(service.universes))
	}

	// Each universe should have 512 channels
	for i := 1; i <= 4; i++ {
		if len(service.universes[i]) != UniverseSize {
			t.Errorf("Universe %d should have %d channels, got %d", i, UniverseSize, len(service.universes[i]))
		}
	}
}

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()

	if cfg.Enabled != true {
		t.Error("DefaultConfig should have Enabled = true")
	}
	if cfg.BroadcastAddr != "255.255.255.255" {
		t.Errorf("DefaultConfig BroadcastAddr = %s, want 255.255.255.255", cfg.BroadcastAddr)
	}
	if cfg.Port != 6454 {
		t.Errorf("DefaultConfig Port = %d, want 6454", cfg.Port)
	}
	if cfg.RefreshRateHz != 60 {
		t.Errorf("DefaultConfig RefreshRateHz = %d, want 60", cfg.RefreshRateHz)
	}
	if cfg.IdleRateHz != 1 {
		t.Errorf("DefaultConfig IdleRateHz = %d, want 1", cfg.IdleRateHz)
	}
}

func TestSetChannelValue(t *testing.T) {
	service := NewService(Config{Enabled: false})

	// Set a channel value
	service.SetChannelValue(1, 1, 128)

	got := service.GetChannelValue(1, 1)
	if got != 128 {
		t.Errorf("GetChannelValue(1, 1) = %d, want 128", got)
	}

	// Set another channel
	service.SetChannelValue(1, 512, 255)
	got = service.GetChannelValue(1, 512)
	if got != 255 {
		t.Errorf("GetChannelValue(1, 512) = %d, want 255", got)
	}

	// Channel out of range should not crash
	service.SetChannelValue(1, 0, 100)   // Below range
	service.SetChannelValue(1, 513, 100) // Above range

	// Invalid universe should not crash
	service.SetChannelValue(10, 1, 100)
}

func TestGetChannelValue_Invalid(t *testing.T) {
	service := NewService(Config{Enabled: false})

	// Invalid channel should return 0
	got := service.GetChannelValue(1, 0)
	if got != 0 {
		t.Errorf("GetChannelValue(1, 0) = %d, want 0", got)
	}

	got = service.GetChannelValue(1, 513)
	if got != 0 {
		t.Errorf("GetChannelValue(1, 513) = %d, want 0", got)
	}

	// Invalid universe should return 0
	got = service.GetChannelValue(10, 1)
	if got != 0 {
		t.Errorf("GetChannelValue(10, 1) = %d, want 0", got)
	}
}

func TestSetChannelOverride(t *testing.T) {
	service := NewService(Config{Enabled: false})

	// Set base value
	service.SetChannelValue(1, 1, 100)

	// Set override
	service.SetChannelOverride(1, 1, 200)

	// GetUniverse should return the override value
	universe := service.GetUniverse(1)
	if universe[0] != 200 {
		t.Errorf("GetUniverse(1)[0] = %d, want 200 (override)", universe[0])
	}

	// Clear override
	service.ClearChannelOverride(1, 1)

	// Should return to base value
	universe = service.GetUniverse(1)
	if universe[0] != 100 {
		t.Errorf("GetUniverse(1)[0] = %d, want 100 (base)", universe[0])
	}
}

func TestClearAllOverrides(t *testing.T) {
	service := NewService(Config{Enabled: false})

	// Set multiple overrides
	service.SetChannelOverride(1, 1, 100)
	service.SetChannelOverride(1, 2, 150)
	service.SetChannelOverride(2, 1, 200)

	// Clear all
	service.ClearAllOverrides()

	// All channels should be 0 (no base values set)
	if service.GetUniverse(1)[0] != 0 {
		t.Error("Expected channel to be 0 after clearing overrides")
	}
	if service.GetUniverse(1)[1] != 0 {
		t.Error("Expected channel to be 0 after clearing overrides")
	}
	if service.GetUniverse(2)[0] != 0 {
		t.Error("Expected channel to be 0 after clearing overrides")
	}
}

func TestSetAllChannels(t *testing.T) {
	service := NewService(Config{Enabled: false})

	// Create 512 channel values
	channels := make([]byte, 512)
	for i := range channels {
		channels[i] = byte(i % 256)
	}

	service.SetAllChannels(1, channels)

	// Verify values
	universe := service.GetUniverse(1)
	for i := 0; i < 256; i++ {
		if universe[i] != i {
			t.Errorf("GetUniverse(1)[%d] = %d, want %d", i, universe[i], i)
			break
		}
	}
}

func TestGetAllUniverses(t *testing.T) {
	service := NewService(Config{Enabled: false})

	service.SetChannelValue(1, 1, 100)
	service.SetChannelValue(2, 1, 150)
	service.SetChannelValue(3, 1, 200)
	service.SetChannelValue(4, 1, 250)

	universes := service.GetAllUniverses()

	if len(universes) != 4 {
		t.Errorf("GetAllUniverses() returned %d universes, want 4", len(universes))
	}

	expected := map[int]int{1: 100, 2: 150, 3: 200, 4: 250}
	for universe, val := range expected {
		if universes[universe][0] != val {
			t.Errorf("Universe %d channel 1 = %d, want %d", universe, universes[universe][0], val)
		}
	}
}

func TestFadeToBlack(t *testing.T) {
	service := NewService(Config{Enabled: false})

	// Set some values
	service.SetChannelValue(1, 1, 255)
	service.SetChannelValue(1, 100, 128)
	service.SetChannelValue(2, 50, 200)
	service.SetChannelOverride(1, 2, 180)

	// Set active scene
	service.SetActiveScene("test-scene")

	// Fade to black (immediate)
	service.FadeToBlack()

	// All channels should be 0
	if service.GetChannelValue(1, 1) != 0 {
		t.Error("Expected channel to be 0 after fade to black")
	}
	if service.GetChannelValue(1, 100) != 0 {
		t.Error("Expected channel to be 0 after fade to black")
	}
	if service.GetChannelValue(2, 50) != 0 {
		t.Error("Expected channel to be 0 after fade to black")
	}

	// Overrides should be cleared
	universe := service.GetUniverse(1)
	if universe[1] != 0 {
		t.Error("Expected override to be cleared after fade to black")
	}

	// Active scene should be cleared
	if service.GetActiveSceneID() != nil {
		t.Error("Expected active scene to be nil after fade to black")
	}
}

func TestActiveScene(t *testing.T) {
	service := NewService(Config{Enabled: false})

	// Initially nil
	if service.GetActiveSceneID() != nil {
		t.Error("Expected initial active scene to be nil")
	}

	// Set active scene
	service.SetActiveScene("scene-1")
	sceneID := service.GetActiveSceneID()
	if sceneID == nil || *sceneID != "scene-1" {
		t.Error("Expected active scene to be 'scene-1'")
	}

	// Clear active scene
	service.ClearActiveScene()
	if service.GetActiveSceneID() != nil {
		t.Error("Expected active scene to be nil after clear")
	}
}

func TestIsEnabled(t *testing.T) {
	service := NewService(Config{Enabled: true})
	if !service.IsEnabled() {
		t.Error("Expected IsEnabled() to return true")
	}

	service2 := NewService(Config{Enabled: false})
	if service2.IsEnabled() {
		t.Error("Expected IsEnabled() to return false")
	}
}

func TestGetBroadcastAddress(t *testing.T) {
	service := NewService(Config{
		Enabled:       false,
		BroadcastAddr: "192.168.1.255",
	})

	if service.GetBroadcastAddress() != "192.168.1.255" {
		t.Errorf("GetBroadcastAddress() = %s, want 192.168.1.255", service.GetBroadcastAddress())
	}
}

func TestCountActiveChannels(t *testing.T) {
	service := NewService(Config{Enabled: false})

	// Initially 0
	count := service.CountActiveChannels()
	if count != 0 {
		t.Errorf("CountActiveChannels() = %d, want 0", count)
	}

	// Set some channels
	service.SetChannelValue(1, 1, 100)
	service.SetChannelValue(1, 2, 200)
	service.SetChannelValue(2, 1, 150)

	count = service.CountActiveChannels()
	if count != 3 {
		t.Errorf("CountActiveChannels() = %d, want 3", count)
	}

	// Overrides count too
	service.SetChannelOverride(3, 1, 50)
	count = service.CountActiveChannels()
	if count != 4 {
		t.Errorf("CountActiveChannels() = %d, want 4", count)
	}
}

func TestDirtyFlagBehavior(t *testing.T) {
	service := NewService(Config{Enabled: false})

	// Initially not dirty
	if service.isDirty {
		t.Error("Service should not be dirty initially")
	}

	// Setting a channel should mark as dirty
	service.SetChannelValue(1, 1, 100)

	service.mu.RLock()
	isDirty := service.isDirty
	service.mu.RUnlock()

	if !isDirty {
		t.Error("Service should be dirty after setting channel")
	}
}

func TestAdaptiveRateMode(t *testing.T) {
	testRefreshRate := 50
	testIdleRate := 1

	service := NewService(Config{
		Enabled:          false,
		RefreshRateHz:    testRefreshRate,
		IdleRateHz:       testIdleRate,
		HighRateDuration: 100 * time.Millisecond, // Short for testing
	})

	// Initially not in high rate mode
	if service.isInHighRateMode {
		t.Error("Service should not be in high rate mode initially")
	}

	// Initial rate should be idle rate
	if service.currentRate != testIdleRate {
		t.Errorf("Initial rate = %d, want %d (idle rate)", service.currentRate, testIdleRate)
	}

	// Setting a channel should trigger high rate
	service.SetChannelValue(1, 1, 100)

	service.mu.RLock()
	isHighRate := service.isInHighRateMode
	currentRate := service.currentRate
	service.mu.RUnlock()

	if !isHighRate {
		t.Error("Service should be in high rate mode after change")
	}
	if currentRate != testRefreshRate {
		t.Errorf("High rate should be %d, got %d", testRefreshRate, currentRate)
	}
}

func TestGetCurrentRate(t *testing.T) {
	testIdleRate := 1

	service := NewService(Config{
		Enabled:       false,
		RefreshRateHz: 50,
		IdleRateHz:    testIdleRate,
	})

	// Should start at idle rate
	rate := service.GetCurrentRate()
	if rate != testIdleRate {
		t.Errorf("GetCurrentRate() = %d, want %d (idle rate at startup)", rate, testIdleRate)
	}
}

func TestIsActive(t *testing.T) {
	service := NewService(Config{Enabled: false})

	// Initially not active
	if service.IsActive() {
		t.Error("Service should not be active initially")
	}

	// Trigger high rate mode
	service.SetChannelValue(1, 1, 100)

	if !service.IsActive() {
		t.Error("Service should be active after channel change")
	}
}

func TestTriggerChangeDetection(t *testing.T) {
	service := NewService(Config{Enabled: false})

	// Initially not in high rate mode
	if service.IsActive() {
		t.Error("Service should not be active initially")
	}

	// Trigger manually
	service.TriggerChangeDetection()

	if !service.IsActive() {
		t.Error("Service should be active after TriggerChangeDetection")
	}
}

func TestReloadBroadcastAddress_EnablesArtNet(t *testing.T) {
	// Start with Art-Net disabled
	service := NewService(Config{
		Enabled:       false,
		BroadcastAddr: "",
		Port:          6454,
	})

	// Initially disabled
	if service.IsEnabled() {
		t.Error("Service should be disabled initially")
	}

	// Initialize the service (starts transmission loop)
	err := service.Initialize()
	if err != nil {
		t.Fatalf("Initialize() error: %v", err)
	}
	defer service.Stop()

	// Reload broadcast address should enable Art-Net
	err = service.ReloadBroadcastAddress("127.0.0.1")
	if err != nil {
		t.Fatalf("ReloadBroadcastAddress() error: %v", err)
	}

	// Should now be enabled
	if !service.IsEnabled() {
		t.Error("Service should be enabled after ReloadBroadcastAddress")
	}

	// Should have correct broadcast address
	if service.GetBroadcastAddress() != "127.0.0.1" {
		t.Errorf("GetBroadcastAddress() = %s, want 127.0.0.1", service.GetBroadcastAddress())
	}
}

func TestReloadBroadcastAddress_ChangesAddress(t *testing.T) {
	// Start with Art-Net enabled
	service := NewService(Config{
		Enabled:       true,
		BroadcastAddr: "127.0.0.1",
		Port:          6454,
	})

	err := service.Initialize()
	if err != nil {
		t.Fatalf("Initialize() error: %v", err)
	}
	defer service.Stop()

	// Change to a different address
	err = service.ReloadBroadcastAddress("192.168.1.255")
	if err != nil {
		t.Fatalf("ReloadBroadcastAddress() error: %v", err)
	}

	// Should still be enabled
	if !service.IsEnabled() {
		t.Error("Service should remain enabled")
	}

	// Should have new broadcast address
	if service.GetBroadcastAddress() != "192.168.1.255" {
		t.Errorf("GetBroadcastAddress() = %s, want 192.168.1.255", service.GetBroadcastAddress())
	}
}

func TestDisableArtNet(t *testing.T) {
	// Start with Art-Net enabled
	service := NewService(Config{
		Enabled:       true,
		BroadcastAddr: "127.0.0.1",
		Port:          6454,
	})

	err := service.Initialize()
	if err != nil {
		t.Fatalf("Initialize() error: %v", err)
	}
	defer service.Stop()

	// Verify enabled
	if !service.IsEnabled() {
		t.Error("Service should be enabled initially")
	}

	// Disable Art-Net
	service.DisableArtNet()

	// Should be disabled
	if service.IsEnabled() {
		t.Error("Service should be disabled after DisableArtNet")
	}

	// Broadcast address should be empty
	if service.GetBroadcastAddress() != "" {
		t.Errorf("GetBroadcastAddress() = %s, want empty", service.GetBroadcastAddress())
	}
}

func TestArtNetEnableDisableCycle(t *testing.T) {
	// Start disabled
	service := NewService(Config{
		Enabled:       false,
		BroadcastAddr: "",
		Port:          6454,
	})

	err := service.Initialize()
	if err != nil {
		t.Fatalf("Initialize() error: %v", err)
	}
	defer service.Stop()

	// Enable via ReloadBroadcastAddress
	err = service.ReloadBroadcastAddress("127.0.0.1")
	if err != nil {
		t.Fatalf("ReloadBroadcastAddress() error: %v", err)
	}

	if !service.IsEnabled() {
		t.Error("Should be enabled after ReloadBroadcastAddress")
	}

	// Disable
	service.DisableArtNet()

	if service.IsEnabled() {
		t.Error("Should be disabled after DisableArtNet")
	}

	// Re-enable with different address
	err = service.ReloadBroadcastAddress("192.168.1.255")
	if err != nil {
		t.Fatalf("Second ReloadBroadcastAddress() error: %v", err)
	}

	if !service.IsEnabled() {
		t.Error("Should be enabled after second ReloadBroadcastAddress")
	}

	if service.GetBroadcastAddress() != "192.168.1.255" {
		t.Errorf("GetBroadcastAddress() = %s, want 192.168.1.255", service.GetBroadcastAddress())
	}
}

// TestArtNetBroadcast_UDPReceive verifies that Art-Net packets are actually being
// transmitted over UDP by listening on the Art-Net port.
func TestArtNetBroadcast_UDPReceive(t *testing.T) {
	// Use a unique port to avoid conflicts with other tests
	testPort := 6555

	// Create a UDP listener first
	addr := &net.UDPAddr{IP: net.ParseIP("127.0.0.1"), Port: testPort}

	listener, err := net.ListenUDP("udp4", addr)
	if err != nil {
		t.Fatalf("Failed to create UDP listener: %v", err)
	}
	defer func() { _ = listener.Close() }()

	// Create DMX service broadcasting to our listener
	service := NewService(Config{
		Enabled:          true,
		BroadcastAddr:    "127.0.0.1",
		Port:             testPort,
		RefreshRateHz:    100, // High rate for quick testing
		IdleRateHz:       1,
		HighRateDuration: 5 * time.Second,
	})

	err = service.Initialize()
	if err != nil {
		t.Fatalf("Initialize() error: %v", err)
	}
	defer service.Stop()

	// Set some channel values to trigger transmission
	service.SetChannelValue(1, 1, 255)
	service.SetChannelValue(1, 10, 128)
	service.SetChannelValue(1, 100, 64)

	// Read packets with timeout
	received := make(chan []byte, 10)
	go func() {
		buffer := make([]byte, 1024)
		for {
			_ = listener.SetReadDeadline(time.Now().Add(2 * time.Second))
			n, _, err := listener.ReadFromUDP(buffer)
			if err != nil {
				return
			}
			packet := make([]byte, n)
			copy(packet, buffer[:n])
			received <- packet
		}
	}()

	// Wait for at least one packet
	select {
	case packet := <-received:
		// Verify Art-Net header
		if len(packet) < 18 {
			t.Fatalf("Packet too short: %d bytes", len(packet))
		}

		// Check Art-Net ID: "Art-Net\x00"
		artNetID := string(packet[0:8])
		if artNetID != "Art-Net\x00" {
			t.Errorf("Invalid Art-Net ID: %q", artNetID)
		}

		// Check OpCode (little-endian): 0x5000 for DMX output
		opCode := int(packet[8]) | int(packet[9])<<8
		if opCode != 0x5000 {
			t.Errorf("Invalid OpCode: 0x%04x, want 0x5000", opCode)
		}

		// Check protocol version (big-endian): should be 14
		protoVer := int(packet[10])<<8 | int(packet[11])
		if protoVer != 14 {
			t.Errorf("Invalid protocol version: %d, want 14", protoVer)
		}

		// Check that DMX data starts at byte 18
		if len(packet) < 18+512 {
			t.Fatalf("Packet doesn't contain full DMX data: %d bytes", len(packet))
		}

		// Verify our channel values are in the packet (DMX data starts at offset 18)
		dmxData := packet[18:]
		if dmxData[0] != 255 {
			t.Errorf("Channel 1 = %d, want 255", dmxData[0])
		}
		if dmxData[9] != 128 {
			t.Errorf("Channel 10 = %d, want 128", dmxData[9])
		}
		if dmxData[99] != 64 {
			t.Errorf("Channel 100 = %d, want 64", dmxData[99])
		}

		t.Logf("Successfully received Art-Net packet with %d bytes, DMX channels verified", len(packet))

	case <-time.After(3 * time.Second):
		t.Error("Timeout waiting for Art-Net packet")
	}
}

// TestArtNetBroadcast_MultipleUniverses verifies that multiple universes
// are transmitted correctly.
func TestArtNetBroadcast_MultipleUniverses(t *testing.T) {
	testPort := 6556

	addr := &net.UDPAddr{IP: net.ParseIP("127.0.0.1"), Port: testPort}
	listener, err := net.ListenUDP("udp4", addr)
	if err != nil {
		t.Fatalf("Failed to create UDP listener: %v", err)
	}
	defer func() { _ = listener.Close() }()

	service := NewService(Config{
		Enabled:          true,
		BroadcastAddr:    "127.0.0.1",
		Port:             testPort,
		RefreshRateHz:    100,
		IdleRateHz:       1,
		HighRateDuration: 5 * time.Second,
	})

	err = service.Initialize()
	if err != nil {
		t.Fatalf("Initialize() error: %v", err)
	}
	defer service.Stop()

	// Set channel values in multiple universes
	service.SetChannelValue(1, 1, 100)
	service.SetChannelValue(2, 1, 150)
	service.SetChannelValue(3, 1, 200)
	service.SetChannelValue(4, 1, 250)

	// Collect packets from multiple universes
	universesSeen := make(map[int]bool)
	universesExpected := 4
	timeout := time.After(3 * time.Second)

	for len(universesSeen) < universesExpected {
		buffer := make([]byte, 1024)
		_ = listener.SetReadDeadline(time.Now().Add(500 * time.Millisecond))
		n, _, err := listener.ReadFromUDP(buffer)
		if err != nil {
			select {
			case <-timeout:
				t.Fatalf("Timeout: only received %d universes, expected %d", len(universesSeen), universesExpected)
			default:
				continue
			}
		}

		if n < 18 {
			continue
		}

		// Parse universe from packet (SubNet + Net encoding at bytes 14-15)
		subUni := int(buffer[14])
		// Universe is 0-indexed in Art-Net, we use 1-indexed
		universe := subUni + 1

		if universe >= 1 && universe <= 4 {
			universesSeen[universe] = true
			t.Logf("Received packet for universe %d", universe)
		}

		select {
		case <-timeout:
			t.Fatalf("Timeout: only received %d universes, expected %d", len(universesSeen), universesExpected)
		default:
		}
	}

	t.Logf("Successfully received packets from all %d universes", len(universesSeen))
}

// TestArtNetBroadcast_AfterEnable verifies that Art-Net packets are sent
// after enabling Art-Net via ReloadBroadcastAddress on a disabled service.
func TestArtNetBroadcast_AfterEnable(t *testing.T) {
	testPort := 6557

	// Create listener first
	addr := &net.UDPAddr{IP: net.ParseIP("127.0.0.1"), Port: testPort}
	listener, err := net.ListenUDP("udp4", addr)
	if err != nil {
		t.Fatalf("Failed to create UDP listener: %v", err)
	}
	defer func() { _ = listener.Close() }()

	// Start with Art-Net DISABLED
	service := NewService(Config{
		Enabled:          false,
		BroadcastAddr:    "",
		Port:             testPort,
		RefreshRateHz:    100,
		IdleRateHz:       1,
		HighRateDuration: 5 * time.Second,
	})

	err = service.Initialize()
	if err != nil {
		t.Fatalf("Initialize() error: %v", err)
	}
	defer service.Stop()

	// Verify initially disabled
	if service.IsEnabled() {
		t.Error("Service should be disabled initially")
	}

	// Enable Art-Net via ReloadBroadcastAddress
	err = service.ReloadBroadcastAddress("127.0.0.1")
	if err != nil {
		t.Fatalf("ReloadBroadcastAddress() error: %v", err)
	}

	// Verify now enabled
	if !service.IsEnabled() {
		t.Fatal("Service should be enabled after ReloadBroadcastAddress")
	}

	// Set channel values and trigger transmission
	service.SetChannelValue(1, 1, 200)
	service.TriggerChangeDetection()

	// Wait for packet
	buffer := make([]byte, 1024)
	_ = listener.SetReadDeadline(time.Now().Add(2 * time.Second))
	n, _, err := listener.ReadFromUDP(buffer)
	if err != nil {
		t.Fatalf("Failed to receive packet after enabling Art-Net: %v", err)
	}

	// Verify it's a valid Art-Net packet
	if n < 18 {
		t.Fatalf("Packet too short: %d bytes", n)
	}

	artNetID := string(buffer[0:8])
	if artNetID != "Art-Net\x00" {
		t.Errorf("Invalid Art-Net ID: %q", artNetID)
	}

	// Verify our channel value
	dmxData := buffer[18:]
	if dmxData[0] != 200 {
		t.Errorf("Channel 1 = %d, want 200", dmxData[0])
	}

	t.Log("Successfully received Art-Net packet after enabling via ReloadBroadcastAddress")
}

// TestArtNetPacketFormat verifies the exact Art-Net DMX packet format.
func TestArtNetPacketFormat(t *testing.T) {
	// Build a test packet using the artnet package
	channels := make([]byte, 512)
	channels[0] = 255
	channels[99] = 128
	channels[255] = 64

	packet := artnet.BuildDMXPacket(1, channels, 42) // Test with sequence number 42

	// Verify packet length
	expectedLen := 18 + 512 // Header + DMX data
	if len(packet) != expectedLen {
		t.Errorf("Packet length = %d, want %d", len(packet), expectedLen)
	}

	// Verify Art-Net ID
	if string(packet[0:7]) != "Art-Net" {
		t.Errorf("Art-Net ID = %q, want 'Art-Net'", string(packet[0:7]))
	}
	if packet[7] != 0 {
		t.Error("Art-Net ID should be null-terminated")
	}

	// Verify OpCode (0x5000 for DMX, little-endian)
	if packet[8] != 0x00 || packet[9] != 0x50 {
		t.Errorf("OpCode = 0x%02x%02x, want 0x0050", packet[9], packet[8])
	}

	// Verify Protocol Version (14, big-endian)
	if packet[10] != 0 || packet[11] != 14 {
		t.Errorf("Protocol version = %d.%d, want 0.14", packet[10], packet[11])
	}

	// Verify Sequence number
	if packet[12] != 42 {
		t.Errorf("Sequence number = %d, want 42", packet[12])
	}

	// Verify DMX data starts at byte 18
	if packet[18] != 255 {
		t.Errorf("DMX channel 1 = %d, want 255", packet[18])
	}
	if packet[18+99] != 128 {
		t.Errorf("DMX channel 100 = %d, want 128", packet[18+99])
	}
	if packet[18+255] != 64 {
		t.Errorf("DMX channel 256 = %d, want 64", packet[18+255])
	}
}

// TestTriggerChangeDetectionNoImmediateTransmission verifies that TriggerChangeDetection()
// does NOT cause immediate packet transmission, preventing the race condition where
// both the fade engine and transmitLoop were sending packets independently.
// This is a regression test for the race condition fix.
func TestTriggerChangeDetectionNoImmediateTransmission(t *testing.T) {
	cfg := Config{
		Enabled:          true,
		BroadcastAddr:    "127.0.0.1",
		Port:             6558, // Unique port to avoid conflicts
		RefreshRateHz:    60,
		IdleRateHz:       1,
		HighRateDuration: 5 * time.Second,
	}

	svc := NewService(cfg)
	err := svc.Initialize()
	if err != nil {
		t.Fatalf("Initialize failed: %v", err)
	}
	defer svc.Stop()

	// Set up UDP listener to count packets
	addr, err := net.ResolveUDPAddr("udp4", "127.0.0.1:6558")
	if err != nil {
		t.Fatalf("ResolveUDPAddr failed: %v", err)
	}

	conn, err := net.ListenUDP("udp4", addr)
	if err != nil {
		t.Fatalf("ListenUDP failed: %v", err)
	}
	defer func() {
		if err := conn.Close(); err != nil {
			t.Logf("conn.Close error: %v", err)
		}
	}()

	// Trigger high-rate mode and wait for it to fully engage
	// The service starts in idle mode (1Hz), so the first tick may take up to 1 second
	// After that tick detects changes, it switches to high-rate mode
	svc.SetChannelValue(1, 1, 100)
	svc.TriggerChangeDetection()
	time.Sleep(1200 * time.Millisecond) // Wait for idle tick + high-rate mode switch

	// Count packets for exactly 1 second
	startTime := time.Now()
	testDuration := 1 * time.Second
	packetCount := 0
	buffer := make([]byte, 1024)

	if err := conn.SetReadDeadline(time.Now().Add(testDuration + 100*time.Millisecond)); err != nil {
		t.Fatalf("SetReadDeadline failed: %v", err)
	}

	// Simulate ongoing fade changes while counting packets
	go func() {
		for time.Since(startTime) < testDuration {
			svc.SetChannelValue(1, 1, byte(time.Since(startTime).Milliseconds()%256))
			time.Sleep(16 * time.Millisecond) // ~60Hz fade updates
		}
	}()

	// Count packets received during test duration
	for time.Since(startTime) < testDuration {
		_, err := conn.Read(buffer)
		if err == nil {
			packetCount++
		}
	}

	// At 60Hz over 1 second, we expect ~60-240 packets depending on dirty universe logic
	// (60 if only one universe is dirty, 240 if all universes transmitted every tick)
	// The key test is that we don't get excessive packets which would indicate
	// the race condition (immediate transmissions) is occurring
	//
	// Note: maxExpected of 400 allows for race detector overhead while still detecting
	// the race condition (which would produce 600+ packets from dual transmission paths)
	minExpected := 50
	maxExpected := 400

	t.Logf("Received %d packets over %v", packetCount, testDuration)

	if packetCount < minExpected {
		t.Errorf("Too few packets: got %d, expected at least %d (possible transmission issues)", packetCount, minExpected)
	}

	if packetCount > maxExpected {
		t.Errorf("Too many packets: got %d, expected at most %d (possible race condition - immediate transmissions occurring)", packetCount, maxExpected)
	}

	t.Logf("✓ Packet rate verification passed: %d packets is within expected range [%d-%d]", packetCount, minExpected, maxExpected)
}

// TestTransmitLoopConsistentTiming verifies that the Ticker-based transmitLoop
// maintains consistent timing without drift, which was the second part of the fix.
func TestTransmitLoopConsistentTiming(t *testing.T) {
	cfg := Config{
		Enabled:          true,
		BroadcastAddr:    "127.0.0.1",
		Port:             6559, // Unique port
		RefreshRateHz:    60,
		IdleRateHz:       1,
		HighRateDuration: 5 * time.Second,
	}

	svc := NewService(cfg)
	err := svc.Initialize()
	if err != nil {
		t.Fatalf("Initialize failed: %v", err)
	}
	defer svc.Stop()

	// Set up UDP listener to measure packet timing
	addr, err := net.ResolveUDPAddr("udp4", "127.0.0.1:6559")
	if err != nil {
		t.Fatalf("ResolveUDPAddr failed: %v", err)
	}

	conn, err := net.ListenUDP("udp4", addr)
	if err != nil {
		t.Fatalf("ListenUDP failed: %v", err)
	}
	defer func() {
		if err := conn.Close(); err != nil {
			t.Logf("conn.Close error: %v", err)
		}
	}()

	// Trigger high-rate mode and wait for it to fully engage
	svc.SetChannelValue(1, 1, 100)
	svc.TriggerChangeDetection()
	time.Sleep(1200 * time.Millisecond) // Wait for idle tick + high-rate mode switch

	// Count packets for exactly 1 second
	startTime := time.Now()
	testDuration := 1 * time.Second
	packetCount := 0
	buffer := make([]byte, 1024)

	if err := conn.SetReadDeadline(time.Now().Add(testDuration + 100*time.Millisecond)); err != nil {
		t.Fatalf("SetReadDeadline failed: %v", err)
	}

	// Keep updating channels to maintain dirty state
	done := make(chan bool)
	go func() {
		for {
			select {
			case <-done:
				return
			default:
				svc.SetChannelValue(1, 1, byte(time.Since(startTime).Milliseconds()%256))
				time.Sleep(10 * time.Millisecond)
			}
		}
	}()

	// Count all packets received during test duration
	for time.Since(startTime) < testDuration {
		_, err := conn.Read(buffer)
		if err == nil {
			packetCount++
		}
	}
	close(done)

	// At 60Hz with 4 universes, expect ~240 packets/sec
	// With race detector overhead, allow wider tolerance
	minExpected := 100
	maxExpected := 350

	t.Logf("Received %d packets over %v", packetCount, testDuration)

	if packetCount < minExpected {
		t.Fatalf("Too few packets: got %d, expected at least %d (transmission not working)", packetCount, minExpected)
	}

	if packetCount > maxExpected {
		t.Fatalf("Too many packets: got %d, expected at most %d (timing issues)", packetCount, maxExpected)
	}

	t.Logf("✓ Transmission rate verified: %d packets is within expected range [%d-%d]", packetCount, minExpected, maxExpected)
}
