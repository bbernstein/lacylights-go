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
	channelValues := make([]models.ChannelValue, len(channels))
	for i, ch := range channels {
		channelValues[i] = models.ChannelValue{
			Offset: ch.Offset,
			Value:  ch.Value,
		}
	}
	jsonData, err := json.Marshal(channelValues)
	if err != nil {
		return "", fmt.Errorf("failed to serialize channel values: %w", err)
	}
	return string(jsonData), nil
}
