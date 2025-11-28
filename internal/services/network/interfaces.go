// Package network provides utilities for network interface enumeration
package network

import (
	"fmt"
	"net"
	"os/exec"
	"runtime"
	"strings"
)

// InterfaceOption represents a network interface option for Art-Net broadcast
type InterfaceOption struct {
	Name          string
	Address       string
	Broadcast     string
	Description   string
	InterfaceType string // "ethernet", "wifi", "other", "localhost", "global"
}

// GetInterfaceType determines the type of network interface
func GetInterfaceType(ifaceName string) string {
	// Try macOS-specific detection first
	if runtime.GOOS == "darwin" {
		interfaceType := getMacOSInterfaceType(ifaceName)
		if interfaceType != "other" {
			return interfaceType
		}
	}

	// Fallback logic based on naming conventions
	return getFallbackInterfaceType(ifaceName)
}

// getMacOSInterfaceType uses networksetup to determine interface type on macOS
func getMacOSInterfaceType(ifaceName string) string {
	// Sanitize interface name to prevent command injection
	for _, char := range ifaceName {
		isLowerLetter := char >= 'a' && char <= 'z'
		isUpperLetter := char >= 'A' && char <= 'Z'
		isDigit := char >= '0' && char <= '9'
		isAllowed := isLowerLetter || isUpperLetter || isDigit || char == '-' || char == '_'
		if !isAllowed {
			return getFallbackInterfaceType(ifaceName)
		}
	}

	cmd := exec.Command("networksetup", "-listallhardwareports")
	output, err := cmd.Output()
	if err != nil {
		return getFallbackInterfaceType(ifaceName)
	}

	outputLower := strings.ToLower(string(output))
	deviceSearch := fmt.Sprintf("device: %s", strings.ToLower(ifaceName))

	// Split output into blocks
	blocks := strings.Split(outputLower, "hardware port:")
	for _, block := range blocks[1:] { // Skip first empty split
		if strings.Contains(block, deviceSearch) {
			if strings.Contains(block, "wi-fi") ||
				strings.Contains(block, "wifi") ||
				strings.Contains(block, "wireless") {
				return "wifi"
			}
			if (strings.Contains(block, "usb") &&
				(strings.Contains(block, "lan") ||
					strings.Contains(block, "ethernet") ||
					strings.Contains(block, "100"))) ||
				strings.Contains(block, "thunderbolt") ||
				strings.Contains(block, "ethernet") ||
				strings.Contains(block, "wired") {
				return "ethernet"
			}
			return "other"
		}
	}

	return getFallbackInterfaceType(ifaceName)
}

// getFallbackInterfaceType uses naming patterns to guess interface type
func getFallbackInterfaceType(ifaceName string) string {
	name := strings.ToLower(ifaceName)

	// en0 is typically WiFi on macOS
	if name == "en0" {
		return "wifi"
	}

	// Common ethernet naming patterns
	if strings.HasPrefix(name, "eth") ||
		strings.HasPrefix(name, "en") ||
		strings.HasPrefix(name, "enp") ||
		strings.HasPrefix(name, "eno") {
		return "ethernet"
	}

	// Common WiFi naming patterns
	if strings.HasPrefix(name, "wlan") ||
		strings.HasPrefix(name, "wl") ||
		strings.Contains(name, "wifi") ||
		strings.Contains(name, "wireless") {
		return "wifi"
	}

	return "other"
}

// getTypeIcon returns an emoji for the interface type
func getTypeIcon(interfaceType string) string {
	switch interfaceType {
	case "wifi":
		return "📶"
	case "ethernet":
		return "🌐"
	case "other":
		return "📡"
	case "localhost":
		return "🏠"
	case "global":
		return "🌍"
	default:
		return "📡"
	}
}

// capitalize returns the string with the first letter capitalized.
func capitalize(s string) string {
	if len(s) == 0 {
		return s
	}
	return strings.ToUpper(s[:1]) + s[1:]
}

// calculateBroadcast computes the broadcast address from IP and netmask
func calculateBroadcast(ip net.IP, mask net.IPMask) net.IP {
	if ip == nil || mask == nil {
		return nil
	}

	// Convert to 4-byte IPv4 representation
	ip4 := ip.To4()
	if ip4 == nil {
		return nil
	}

	// Ensure mask is also 4 bytes
	if len(mask) == 16 {
		mask = mask[12:16]
	}
	if len(mask) != 4 {
		return nil
	}

	broadcast := make(net.IP, 4)
	for i := 0; i < 4; i++ {
		broadcast[i] = ip4[i] | ^mask[i]
	}

	return broadcast
}

// GetNetworkInterfaces returns all available network interfaces for Art-Net broadcast
func GetNetworkInterfaces() ([]InterfaceOption, error) {
	interfaces, err := net.Interfaces()
	if err != nil {
		return nil, fmt.Errorf("failed to get network interfaces: %w", err)
	}

	var ethernetOptions []InterfaceOption
	var wifiOptions []InterfaceOption
	var otherOptions []InterfaceOption

	for _, iface := range interfaces {
		// Skip down interfaces
		if iface.Flags&net.FlagUp == 0 {
			continue
		}

		// Skip loopback
		if iface.Flags&net.FlagLoopback != 0 {
			continue
		}

		addrs, err := iface.Addrs()
		if err != nil {
			continue
		}

		for _, addr := range addrs {
			ipNet, ok := addr.(*net.IPNet)
			if !ok {
				continue
			}

			// Only IPv4
			ip4 := ipNet.IP.To4()
			if ip4 == nil {
				continue
			}

			// Calculate broadcast address
			broadcast := calculateBroadcast(ip4, ipNet.Mask)
			if broadcast == nil {
				continue
			}

			broadcastStr := broadcast.String()

			// Skip if broadcast equals IP (point-to-point)
			if broadcastStr == ip4.String() {
				continue
			}

			interfaceType := GetInterfaceType(iface.Name)
			typeIcon := getTypeIcon(interfaceType)

			option := InterfaceOption{
				Name:          fmt.Sprintf("%s-broadcast", iface.Name),
				Address:       ip4.String(),
				Broadcast:     broadcastStr,
				Description:   fmt.Sprintf("%s %s - %s Broadcast (%s)", typeIcon, iface.Name, capitalize(interfaceType), broadcastStr),
				InterfaceType: interfaceType,
			}

			switch interfaceType {
			case "ethernet":
				ethernetOptions = append(ethernetOptions, option)
			case "wifi":
				wifiOptions = append(wifiOptions, option)
			default:
				otherOptions = append(otherOptions, option)
			}
		}
	}

	// Build final sorted array: ethernet, wifi, other, localhost, global broadcast
	options := make([]InterfaceOption, 0, len(ethernetOptions)+len(wifiOptions)+len(otherOptions)+2)
	options = append(options, ethernetOptions...)
	options = append(options, wifiOptions...)
	options = append(options, otherOptions...)

	// Add localhost option
	options = append(options, InterfaceOption{
		Name:          "localhost",
		Address:       "127.0.0.1",
		Broadcast:     "127.0.0.1",
		Description:   fmt.Sprintf("%s Localhost (for testing only)", getTypeIcon("localhost")),
		InterfaceType: "localhost",
	})

	// Add global broadcast option
	options = append(options, InterfaceOption{
		Name:          "global-broadcast",
		Address:       "0.0.0.0",
		Broadcast:     "255.255.255.255",
		Description:   fmt.Sprintf("%s Global Broadcast (255.255.255.255)", getTypeIcon("global")),
		InterfaceType: "global",
	})

	return options, nil
}
