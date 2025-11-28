package network

import (
	"net"
	"testing"
)

func TestCalculateBroadcast(t *testing.T) {
	tests := []struct {
		name     string
		ip       net.IP
		mask     net.IPMask
		expected string
	}{
		{
			name:     "Class C network",
			ip:       net.ParseIP("192.168.1.100"),
			mask:     net.IPv4Mask(255, 255, 255, 0),
			expected: "192.168.1.255",
		},
		{
			name:     "Class B network",
			ip:       net.ParseIP("172.16.5.10"),
			mask:     net.IPv4Mask(255, 255, 0, 0),
			expected: "172.16.255.255",
		},
		{
			name:     "Class A network",
			ip:       net.ParseIP("10.0.0.5"),
			mask:     net.IPv4Mask(255, 0, 0, 0),
			expected: "10.255.255.255",
		},
		{
			name:     "/28 subnet",
			ip:       net.ParseIP("192.168.1.20"),
			mask:     net.IPv4Mask(255, 255, 255, 240), // /28
			expected: "192.168.1.31",
		},
		{
			name:     "/30 subnet",
			ip:       net.ParseIP("192.168.1.5"),
			mask:     net.IPv4Mask(255, 255, 255, 252), // /30
			expected: "192.168.1.7",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := calculateBroadcast(tt.ip, tt.mask)
			if result == nil {
				t.Fatalf("calculateBroadcast returned nil")
			}
			if result.String() != tt.expected {
				t.Errorf("calculateBroadcast(%s, %v) = %s, want %s",
					tt.ip, tt.mask, result.String(), tt.expected)
			}
		})
	}
}

func TestCalculateBroadcast_NilInputs(t *testing.T) {
	// Test nil IP
	result := calculateBroadcast(nil, net.IPv4Mask(255, 255, 255, 0))
	if result != nil {
		t.Error("calculateBroadcast(nil, mask) should return nil")
	}

	// Test nil mask
	result = calculateBroadcast(net.ParseIP("192.168.1.1"), nil)
	if result != nil {
		t.Error("calculateBroadcast(ip, nil) should return nil")
	}

	// Test IPv6 (unsupported)
	result = calculateBroadcast(net.ParseIP("::1"), net.IPv4Mask(255, 255, 255, 0))
	if result != nil {
		t.Error("calculateBroadcast(ipv6, mask) should return nil")
	}
}

func TestGetFallbackInterfaceType(t *testing.T) {
	tests := []struct {
		name     string
		iface    string
		expected string
	}{
		{"en0 is wifi", "en0", "wifi"},
		{"en1 is ethernet", "en1", "ethernet"},
		{"eth0 is ethernet", "eth0", "ethernet"},
		{"eth1 is ethernet", "eth1", "ethernet"},
		{"wlan0 is wifi", "wlan0", "wifi"},
		{"wlp2s0 is wifi", "wlp2s0", "wifi"},
		{"enp0s3 is ethernet", "enp0s3", "ethernet"},
		{"eno1 is ethernet", "eno1", "ethernet"},
		{"utun0 is other", "utun0", "other"},
		{"bridge0 is other", "bridge0", "other"},
		{"lo0 is other", "lo0", "other"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := getFallbackInterfaceType(tt.iface)
			if result != tt.expected {
				t.Errorf("getFallbackInterfaceType(%q) = %q, want %q",
					tt.iface, result, tt.expected)
			}
		})
	}
}

func TestGetTypeIcon(t *testing.T) {
	tests := []struct {
		interfaceType string
		expectedIcon  string
	}{
		{"wifi", "📶"},
		{"ethernet", "🌐"},
		{"other", "📡"},
		{"localhost", "🏠"},
		{"global", "🌍"},
		{"unknown", "📡"},
	}

	for _, tt := range tests {
		t.Run(tt.interfaceType, func(t *testing.T) {
			result := getTypeIcon(tt.interfaceType)
			if result != tt.expectedIcon {
				t.Errorf("getTypeIcon(%q) = %q, want %q",
					tt.interfaceType, result, tt.expectedIcon)
			}
		})
	}
}

func TestGetNetworkInterfaces_AlwaysIncludesLocalhostAndGlobal(t *testing.T) {
	interfaces, err := GetNetworkInterfaces()
	if err != nil {
		t.Fatalf("GetNetworkInterfaces() returned error: %v", err)
	}

	// Should have at least localhost and global broadcast
	if len(interfaces) < 2 {
		t.Fatalf("GetNetworkInterfaces() returned %d interfaces, want at least 2",
			len(interfaces))
	}

	// Check that localhost is present
	var hasLocalhost, hasGlobal bool
	for _, iface := range interfaces {
		if iface.Name == "localhost" {
			hasLocalhost = true
			if iface.Address != "127.0.0.1" {
				t.Errorf("localhost address = %s, want 127.0.0.1", iface.Address)
			}
			if iface.Broadcast != "127.0.0.1" {
				t.Errorf("localhost broadcast = %s, want 127.0.0.1", iface.Broadcast)
			}
			if iface.InterfaceType != "localhost" {
				t.Errorf("localhost type = %s, want localhost", iface.InterfaceType)
			}
		}
		if iface.Name == "global-broadcast" {
			hasGlobal = true
			if iface.Address != "0.0.0.0" {
				t.Errorf("global address = %s, want 0.0.0.0", iface.Address)
			}
			if iface.Broadcast != "255.255.255.255" {
				t.Errorf("global broadcast = %s, want 255.255.255.255", iface.Broadcast)
			}
			if iface.InterfaceType != "global" {
				t.Errorf("global type = %s, want global", iface.InterfaceType)
			}
		}
	}

	if !hasLocalhost {
		t.Error("GetNetworkInterfaces() missing localhost option")
	}
	if !hasGlobal {
		t.Error("GetNetworkInterfaces() missing global-broadcast option")
	}
}

func TestGetNetworkInterfaces_LocalhostAndGlobalAreLast(t *testing.T) {
	interfaces, err := GetNetworkInterfaces()
	if err != nil {
		t.Fatalf("GetNetworkInterfaces() returned error: %v", err)
	}

	n := len(interfaces)
	if n < 2 {
		t.Fatalf("Need at least 2 interfaces, got %d", n)
	}

	// Localhost should be second to last
	if interfaces[n-2].Name != "localhost" {
		t.Errorf("Second to last interface = %s, want localhost", interfaces[n-2].Name)
	}

	// Global broadcast should be last
	if interfaces[n-1].Name != "global-broadcast" {
		t.Errorf("Last interface = %s, want global-broadcast", interfaces[n-1].Name)
	}
}

func TestGetNetworkInterfaces_InterfacesHaveValidFields(t *testing.T) {
	interfaces, err := GetNetworkInterfaces()
	if err != nil {
		t.Fatalf("GetNetworkInterfaces() returned error: %v", err)
	}

	for _, iface := range interfaces {
		// All fields should be non-empty
		if iface.Name == "" {
			t.Error("Interface has empty name")
		}
		if iface.Address == "" {
			t.Error("Interface has empty address")
		}
		if iface.Broadcast == "" {
			t.Error("Interface has empty broadcast")
		}
		if iface.Description == "" {
			t.Error("Interface has empty description")
		}
		if iface.InterfaceType == "" {
			t.Error("Interface has empty type")
		}

		// InterfaceType should be one of the valid values
		validTypes := map[string]bool{
			"ethernet":  true,
			"wifi":      true,
			"other":     true,
			"localhost": true,
			"global":    true,
		}
		if !validTypes[iface.InterfaceType] {
			t.Errorf("Interface type %q is not valid", iface.InterfaceType)
		}
	}
}

func TestGetInterfaceType(t *testing.T) {
	// This test verifies that GetInterfaceType doesn't panic
	// Actual interface detection may vary by system
	testNames := []string{
		"en0", "en1", "eth0", "wlan0", "lo0",
		"utun0", "bridge0", "enp0s3", "eno1",
	}

	for _, name := range testNames {
		t.Run(name, func(t *testing.T) {
			result := GetInterfaceType(name)
			// Just verify it returns one of the valid types
			validTypes := map[string]bool{
				"ethernet": true,
				"wifi":     true,
				"other":    true,
			}
			if !validTypes[result] {
				t.Errorf("GetInterfaceType(%q) = %q, not a valid type", name, result)
			}
		})
	}
}

func TestGetInterfaceType_SanitizesInput(t *testing.T) {
	// Names with special characters should use fallback logic
	// and not panic or execute arbitrary commands
	specialNames := []string{
		"en0; rm -rf /",
		"eth0 && echo hacked",
		"wlan$(whoami)",
		"`id`",
	}

	for _, name := range specialNames {
		t.Run(name, func(t *testing.T) {
			// Should not panic
			result := GetInterfaceType(name)
			// Should return something (using fallback)
			if result == "" {
				t.Errorf("GetInterfaceType(%q) returned empty string", name)
			}
		})
	}
}
