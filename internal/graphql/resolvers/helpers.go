package resolvers

import (
	"context"
	"encoding/json"
	"fmt"
	"log"

	"github.com/bbernstein/lacylights-go/internal/database/models"
	"github.com/bbernstein/lacylights-go/internal/graphql/generated"
)

// Canvas dimension validation constants
const (
	MinCanvasSize = 100    // Minimum canvas dimension in pixels
	MaxCanvasSize = 100000 // Maximum canvas dimension in pixels
)

// validateCanvasDimension validates that a canvas dimension is within valid bounds.
// Returns an error if the value is invalid, nil if valid.
func validateCanvasDimension(value int, fieldName string) error {
	if value < MinCanvasSize {
		return fmt.Errorf("%s must be at least %d pixels, got %d", fieldName, MinCanvasSize, value)
	}
	if value > MaxCanvasSize {
		return fmt.Errorf("%s must be at most %d pixels, got %d", fieldName, MaxCanvasSize, value)
	}
	return nil
}

// Default canvas size for coordinate conversion
const DefaultCanvasSize = 2000

// isNormalizedCoordinate checks if a value appears to be a normalized 0-1 coordinate.
// Returns true if the value is strictly between 0 and 1 (exclusive of exactly 0 and 1
// to avoid false positives for pixel values of 0 or 1).
//
// EDGE CASE: Fixtures legitimately positioned at exactly 0.0 or 1.0 in normalized
// coordinates will NOT be detected as normalized. This is an accepted trade-off to
// prevent false positives with fixtures already at pixel positions 0 or 1. In practice,
// fixtures at exact edge positions are rare.
func isNormalizedCoordinate(val *float64) bool {
	if val == nil {
		return false
	}
	// Use > 0 and < 1 to avoid false positives for pixel coords at exactly 0 or 1
	return *val > 0 && *val < 1
}

// convertNormalizedToPixels converts normalized 0-1 coordinates to pixel coordinates.
// This function modifies the fixture in place if coordinates appear to be normalized.
// Returns true if conversion was performed, false otherwise.
//
// WARNING: This function modifies the fixture pointer in-place. The modified fixture
// should NOT be persisted back to the database without explicit user intent, as this
// conversion is meant only for GraphQL query responses. The database retains the
// original values until the user explicitly saves the fixture with new coordinates.
//
// DEPRECATED: This is a backward-compatibility shim for fixtures created before
// the pixel coordinate migration. This code can be removed once all existing
// fixtures have been migrated to pixel coordinates.
func convertNormalizedToPixels(fixture *models.FixtureInstance, canvasWidth, canvasHeight int) bool {
	if fixture == nil {
		return false
	}

	// Both coordinates must be in normalized range for conversion
	if !isNormalizedCoordinate(fixture.LayoutX) || !isNormalizedCoordinate(fixture.LayoutY) {
		return false
	}

	// Use defaults if canvas dimensions are invalid
	if canvasWidth <= 0 {
		canvasWidth = DefaultCanvasSize
	}
	if canvasHeight <= 0 {
		canvasHeight = DefaultCanvasSize
	}

	// Convert normalized to pixel coordinates
	newX := *fixture.LayoutX * float64(canvasWidth)
	newY := *fixture.LayoutY * float64(canvasHeight)
	fixture.LayoutX = &newX
	fixture.LayoutY = &newY

	return true
}

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

// sparseChannelsToDenseArray converts sparse channel JSON to a dense int array.
// Used for backward-compatible output like CompareScenes.
// The resulting array is sized to (maxOffset + 1), which is bounded by DMX constraints
// (max 512 channels per fixture). For sparse data with high offsets, this may create
// arrays with many zero-value elements.
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
// Returns false if either JSON string is invalid or contains duplicate offsets.
func sparseChannelsEqual(channelsJSON1, channelsJSON2 string) bool {
	var sparse1, sparse2 []models.ChannelValue

	if err := json.Unmarshal([]byte(channelsJSON1), &sparse1); err != nil {
		log.Printf("Warning: sparseChannelsEqual received invalid JSON for channels1: %v", err)
		return false
	}
	if err := json.Unmarshal([]byte(channelsJSON2), &sparse2); err != nil {
		log.Printf("Warning: sparseChannelsEqual received invalid JSON for channels2: %v", err)
		return false
	}

	// Build maps for O(1) lookup, detecting duplicate offsets
	map1 := make(map[int]int, len(sparse1))
	for _, ch := range sparse1 {
		if _, exists := map1[ch.Offset]; exists {
			log.Printf("Warning: sparseChannelsEqual detected duplicate offset %d in channels1", ch.Offset)
			return false
		}
		map1[ch.Offset] = ch.Value
	}

	map2 := make(map[int]int, len(sparse2))
	for _, ch := range sparse2 {
		if _, exists := map2[ch.Offset]; exists {
			log.Printf("Warning: sparseChannelsEqual detected duplicate offset %d in channels2", ch.Offset)
			return false
		}
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

// reapplyActiveSceneIfNeeded checks if the given scene is currently active and
// if so, re-applies its DMX values immediately. This ensures that when an active
// scene is edited and saved, the changes are immediately reflected in the DMX output.
// Returns nil if the scene is not active or if the scene was successfully re-applied.
//
// Note: This function uses immediate channel updates (no fade) because when editing
// a scene, users expect to see their changes reflected instantly. This differs from
// ActivateSceneFromBoard which uses the fade engine for smooth transitions.
//
// Note: There is a potential race condition between checking the active scene ID and
// reloading the scene data. If another operation changes the active scene between these
// operations, we may apply the wrong scene. This is acceptable because:
// 1. The scene update has already been persisted successfully
// 2. This is a best-effort UX improvement - the scene data will still be correct
// 3. The likelihood of this race is very low in practice
func (r *Resolver) reapplyActiveSceneIfNeeded(ctx context.Context, sceneID string) error {
	// Check if this scene is currently active
	activeSceneID := r.DMXService.GetActiveSceneID()
	if activeSceneID == nil || *activeSceneID != sceneID {
		// Scene is not active, nothing to do
		return nil
	}

	// Scene is active - reload and re-apply its DMX values
	var scene models.Scene
	result := r.db.WithContext(ctx).Preload("FixtureValues").First(&scene, "id = ?", sceneID)
	if result.Error != nil {
		return fmt.Errorf("failed to reload scene: %w", result.Error)
	}

	// Load fixtures for the scene's fixture values
	var fixtureIDs []string
	for _, fv := range scene.FixtureValues {
		fixtureIDs = append(fixtureIDs, fv.FixtureID)
	}

	var fixtures []models.FixtureInstance
	if len(fixtureIDs) > 0 {
		r.db.WithContext(ctx).Where("id IN ?", fixtureIDs).Find(&fixtures)
	}

	// Create fixture lookup map
	fixtureMap := make(map[string]*models.FixtureInstance)
	for i := range fixtures {
		fixtureMap[fixtures[i].ID] = &fixtures[i]
	}

	// Set channel values directly (no fade, immediate update)
	for _, fixtureValue := range scene.FixtureValues {
		fixture := fixtureMap[fixtureValue.FixtureID]
		if fixture == nil {
			continue
		}

		// Parse sparse channel values from JSON
		var channels []models.ChannelValue
		if err := json.Unmarshal([]byte(fixtureValue.Channels), &channels); err != nil {
			log.Printf("Warning: failed to unmarshal channels for fixtureID %s in sceneID %s: %v", fixtureValue.FixtureID, sceneID, err)
			continue
		}

		// Set channel values - only channels that exist in the sparse array
		for _, ch := range channels {
			dmxChannel := fixture.StartChannel + ch.Offset

			// Validate DMX channel is within bounds
			if !validateDMXChannel(dmxChannel, fixture.Universe, fixture.ID, ch.Offset) {
				continue
			}

			r.DMXService.SetChannelValue(fixture.Universe, dmxChannel, byte(ch.Value))
		}
	}

	// Force immediate transmission
	r.DMXService.TriggerChangeDetection()

	log.Printf("Re-applied active scene %s after update", sceneID)
	return nil
}
