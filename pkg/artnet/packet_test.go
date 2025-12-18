package artnet

import (
	"encoding/binary"
	"testing"
)

func TestBuildDMXPacket(t *testing.T) {
	tests := []struct {
		name       string
		universe   int
		channels   []byte
		wantID     string
		wantOpCode uint16
		wantUniverse uint16
		wantLength uint16
	}{
		{
			name:        "Universe 1",
			universe:    1,
			channels:    make([]byte, 512),
			wantID:      "Art-Net\x00",
			wantOpCode:  0x5000,
			wantUniverse: 0, // 0-based in packet
			wantLength:  512,
		},
		{
			name:        "Universe 4",
			universe:    4,
			channels:    make([]byte, 512),
			wantID:      "Art-Net\x00",
			wantOpCode:  0x5000,
			wantUniverse: 3, // 0-based in packet
			wantLength:  512,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			packet := BuildDMXPacket(tt.universe, tt.channels, 123) // Test with sequence 123

			// Check packet size
			if len(packet) != int(PacketSize) {
				t.Errorf("BuildDMXPacket() packet size = %d, want %d", len(packet), PacketSize)
			}

			// Check Art-Net ID
			gotID := string(packet[0:8])
			if gotID != tt.wantID {
				t.Errorf("BuildDMXPacket() ID = %q, want %q", gotID, tt.wantID)
			}

			// Check OpCode (little-endian)
			gotOpCode := binary.LittleEndian.Uint16(packet[8:10])
			if gotOpCode != tt.wantOpCode {
				t.Errorf("BuildDMXPacket() OpCode = 0x%04x, want 0x%04x", gotOpCode, tt.wantOpCode)
			}

			// Check Protocol Version (big-endian)
			gotVersion := binary.BigEndian.Uint16(packet[10:12])
			if gotVersion != ProtocolVersion {
				t.Errorf("BuildDMXPacket() Protocol Version = %d, want %d", gotVersion, ProtocolVersion)
			}

			// Check Sequence
			if packet[12] != 123 {
				t.Errorf("BuildDMXPacket() Sequence = %d, want 123", packet[12])
			}

			// Check Physical
			if packet[13] != 0 {
				t.Errorf("BuildDMXPacket() Physical = %d, want 0", packet[13])
			}

			// Check Universe (little-endian)
			gotUniverse := binary.LittleEndian.Uint16(packet[14:16])
			if gotUniverse != tt.wantUniverse {
				t.Errorf("BuildDMXPacket() Universe = %d, want %d", gotUniverse, tt.wantUniverse)
			}

			// Check Data Length (big-endian)
			gotLength := binary.BigEndian.Uint16(packet[16:18])
			if gotLength != tt.wantLength {
				t.Errorf("BuildDMXPacket() Length = %d, want %d", gotLength, tt.wantLength)
			}
		})
	}
}

func TestBuildDMXPacket_ChannelData(t *testing.T) {
	// Create channels with specific values
	channels := make([]byte, 512)
	channels[0] = 255   // First channel
	channels[100] = 128 // Middle channel
	channels[511] = 64  // Last channel

	packet := BuildDMXPacket(1, channels, 0)

	// Check channel values in packet
	if packet[18] != 255 {
		t.Errorf("BuildDMXPacket() channel 1 = %d, want 255", packet[18])
	}
	if packet[18+100] != 128 {
		t.Errorf("BuildDMXPacket() channel 101 = %d, want 128", packet[18+100])
	}
	if packet[18+511] != 64 {
		t.Errorf("BuildDMXPacket() channel 512 = %d, want 64", packet[18+511])
	}
}

func TestBuildDMXPacket_ShortChannelArray(t *testing.T) {
	// Test with fewer than 512 channels
	channels := []byte{100, 200}
	packet := BuildDMXPacket(1, channels, 0)

	// First two channels should have values
	if packet[18] != 100 {
		t.Errorf("BuildDMXPacket() channel 1 = %d, want 100", packet[18])
	}
	if packet[19] != 200 {
		t.Errorf("BuildDMXPacket() channel 2 = %d, want 200", packet[19])
	}

	// Remaining channels should be zero-padded
	if packet[20] != 0 {
		t.Errorf("BuildDMXPacket() channel 3 = %d, want 0", packet[20])
	}
}

func TestBuildDMXPacket_EmptyChannels(t *testing.T) {
	packet := BuildDMXPacket(1, nil, 0)

	// Packet should still be valid with all zeros
	if len(packet) != int(PacketSize) {
		t.Errorf("BuildDMXPacket() with nil channels size = %d, want %d", len(packet), PacketSize)
	}

	// All data should be zero
	for i := 18; i < int(PacketSize); i++ {
		if packet[i] != 0 {
			t.Errorf("BuildDMXPacket() channel at offset %d = %d, want 0", i-18, packet[i])
			break
		}
	}
}
