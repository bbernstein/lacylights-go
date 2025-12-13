package resolvers

import (
	"encoding/json"
	"fmt"
	"log"

	"github.com/bbernstein/lacylights-go/internal/database/models"
	"github.com/bbernstein/lacylights-go/internal/graphql/generated"
)

// Helper function to convert int to *int
func intPtr(i int) *int {
	return &i
}

// Helper function to convert string to *string
func stringPtr(s string) *string {
	return &s
}

// Helper function to convert string to *string, returning nil for empty strings
func stringToPointer(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

// serializeSparseChannels converts sparse channel values to JSON for storage
func serializeSparseChannels(channels []*generated.ChannelValueInput) (string, error) {
	// Validate input and check for duplicates
	offsetMap := make(map[int]bool)
	sparseChannels := make([]models.ChannelValue, 0, len(channels))

	for _, ch := range channels {
		// Validate offset is non-negative and within DMX channel range
		if ch.Offset < 0 {
			return "", fmt.Errorf("invalid channel offset %d: must be non-negative", ch.Offset)
		}
		if ch.Offset >= 512 {
			return "", fmt.Errorf("invalid channel offset %d: must be less than 512", ch.Offset)
		}

		// Validate DMX value is in valid range (0-255)
		if ch.Value < 0 || ch.Value > 255 {
			return "", fmt.Errorf("invalid DMX value %d at offset %d: must be 0-255", ch.Value, ch.Offset)
		}

		// Check for duplicate offsets - reject to ensure data integrity
		if offsetMap[ch.Offset] {
			return "", fmt.Errorf("duplicate channel offset %d found in input", ch.Offset)
		}
		offsetMap[ch.Offset] = true

		sparseChannels = append(sparseChannels, models.ChannelValue{
			Offset: ch.Offset,
			Value:  ch.Value,
		})
	}

	jsonData, err := json.Marshal(sparseChannels)
	if err != nil {
		return "", fmt.Errorf("failed to serialize channels: %w", err)
	}
	return string(jsonData), nil
}

// sparseChannelsToDenseArray converts sparse channel JSON to a dense int array
// Used for backward-compatible output like CompareScenes
func sparseChannelsToDenseArray(channelsJSON string) []int {
	var sparse []models.ChannelValue
	if err := json.Unmarshal([]byte(channelsJSON), &sparse); err != nil {
		return []int{}
	}

	if len(sparse) == 0 {
		return []int{}
	}

	// Find max offset to determine array size
	maxOffset := 0
	for _, ch := range sparse {
		if ch.Offset > maxOffset {
			maxOffset = ch.Offset
		}
	}

	// Create dense array
	dense := make([]int, maxOffset+1)
	for _, ch := range sparse {
		dense[ch.Offset] = ch.Value
	}
	return dense
}

// validateDMXChannel validates that a calculated DMX channel is within valid bounds
// DMX channels are 1-512 per universe
func validateDMXChannel(dmxChannel, universe int, fixtureID string, offset int) bool {
	if dmxChannel < 1 || dmxChannel > 512 {
		log.Printf("Warning: DMX channel %d out of bounds for fixture %s (universe %d, offset %d). Skipping.",
			dmxChannel, fixtureID, universe, offset)
		return false
	}
	return true
}

// sparseChannelsEqual compares two sparse channel JSON strings semantically.
// Returns true if they represent the same channel values, regardless of JSON encoding order.
// Invalid JSON is treated as an empty array with a warning logged for debugging.
func sparseChannelsEqual(channelsJSON1, channelsJSON2 string) bool {
	var sparse1, sparse2 []models.ChannelValue

	if err := json.Unmarshal([]byte(channelsJSON1), &sparse1); err != nil {
		log.Printf("Warning: sparseChannelsEqual received invalid JSON for channels1: %v", err)
		sparse1 = []models.ChannelValue{}
	}
	if err := json.Unmarshal([]byte(channelsJSON2), &sparse2); err != nil {
		log.Printf("Warning: sparseChannelsEqual received invalid JSON for channels2: %v", err)
		sparse2 = []models.ChannelValue{}
	}

	// Build maps for O(1) lookup
	map1 := make(map[int]int, len(sparse1))
	for _, ch := range sparse1 {
		map1[ch.Offset] = ch.Value
	}

	map2 := make(map[int]int, len(sparse2))
	for _, ch := range sparse2 {
		map2[ch.Offset] = ch.Value
	}

	// Different number of entries means different
	if len(map1) != len(map2) {
		return false
	}

	// Check all entries in map1 exist with same value in map2
	for offset, value := range map1 {
		if map2[offset] != value {
			return false
		}
	}

	return true
}
