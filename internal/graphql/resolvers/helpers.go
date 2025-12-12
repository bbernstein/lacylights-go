package resolvers

import (
	"encoding/json"
	"fmt"

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
	// Convert to models.ChannelValue for proper JSON serialization
	sparseChannels := make([]models.ChannelValue, len(channels))
	for i, ch := range channels {
		sparseChannels[i] = models.ChannelValue{
			Offset: ch.Offset,
			Value:  ch.Value,
		}
	}
	jsonData, err := json.Marshal(sparseChannels)
	if err != nil {
		return "", fmt.Errorf("failed to serialize channel values: %w", err)
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
