// Package artnet provides Art-Net protocol packet building and transmission.
package artnet

import (
	"encoding/binary"
)

const (
	// OpCodeDMX is the Art-Net operation code for DMX data.
	OpCodeDMX uint16 = 0x5000
	// ProtocolVersion is the Art-Net protocol version.
	ProtocolVersion uint16 = 14
	// DMXDataLength is the number of DMX channels per universe.
	DMXDataLength uint16 = 512
	// PacketSize is the total size of an Art-Net DMX packet.
	PacketSize = 18 + DMXDataLength // Header (18) + Data (512)
	// DefaultPort is the standard Art-Net UDP port.
	DefaultPort = 6454
)

// ArtNetID is the Art-Net packet identifier.
var ArtNetID = []byte{'A', 'r', 't', '-', 'N', 'e', 't', 0x00}

// BuildDMXPacket creates an Art-Net DMX packet for the specified universe.
// Universe should be 1-based (1-4), as used in the application.
// Channels should be exactly 512 bytes.
// Sequence should increment for each packet (0-255, wraps around) to enable receivers
// to detect and handle out-of-order UDP packets.
func BuildDMXPacket(universe int, channels []byte, sequence byte) []byte {
	packet := make([]byte, PacketSize)

	// Art-Net header
	copy(packet[0:8], ArtNetID)                                     // ID (8 bytes): "Art-Net\0"
	binary.LittleEndian.PutUint16(packet[8:10], OpCodeDMX)          // OpCode (2 bytes): 0x5000 for DMX
	binary.BigEndian.PutUint16(packet[10:12], ProtocolVersion)      // Protocol version (2 bytes): 14
	packet[12] = sequence                                           // Sequence (1 byte): increments for each packet
	packet[13] = 0                                                  // Physical input port (1 byte): 0
	binary.LittleEndian.PutUint16(packet[14:16], uint16(universe-1)) // Universe (2 bytes): 0-based
	binary.BigEndian.PutUint16(packet[16:18], DMXDataLength)        // Data length (2 bytes): 512

	// DMX data (512 channels)
	if len(channels) >= 512 {
		copy(packet[18:530], channels[:512])
	} else {
		// Pad with zeros if less than 512 channels provided
		copy(packet[18:18+len(channels)], channels)
	}

	return packet
}
