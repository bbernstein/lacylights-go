package ofl

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"time"

	"github.com/bbernstein/lacylights-go/internal/database/repositories"
	"gorm.io/gorm"
)

// FixtureChangeType represents the type of change for a fixture
type FixtureChangeType string

const (
	ChangeTypeNew       FixtureChangeType = "NEW"
	ChangeTypeUpdated   FixtureChangeType = "UPDATED"
	ChangeTypeUnchanged FixtureChangeType = "UNCHANGED"
)

// FixtureUpdate represents information about a fixture that may need updating
type FixtureUpdate struct {
	FixtureKey    string            // manufacturer/model key
	Manufacturer  string            // Manufacturer name
	Model         string            // Model name
	ChangeType    FixtureChangeType // Type of change
	IsInUse       bool              // Whether fixture is used by any project
	InstanceCount int               // Number of instances using this definition
	CurrentHash   *string           // Current hash (nil if new)
	NewHash       string            // New hash from OFL
}

// UpdateCheckResult represents the result of checking for OFL updates
type UpdateCheckResult struct {
	CurrentFixtureCount int             // Total fixtures in database
	OFLFixtureCount     int             // Total fixtures in OFL source
	NewFixtureCount     int             // New fixtures available
	ChangedFixtureCount int             // Changed fixtures
	ChangedInUseCount   int             // Changed fixtures that are in use
	FixtureUpdates      []FixtureUpdate // Detailed list
	OFLVersion          string          // Version being checked
	CheckedAt           time.Time       // When check was performed
}

// UpdatesService handles checking for OFL updates
type UpdatesService struct {
	db          *gorm.DB
	fixtureRepo *repositories.FixtureRepository
}

// NewUpdatesService creates a new updates service
func NewUpdatesService(db *gorm.DB, fixtureRepo *repositories.FixtureRepository) *UpdatesService {
	return &UpdatesService{
		db:          db,
		fixtureRepo: fixtureRepo,
	}
}

// ComputeFixtureHash computes a SHA256 hash of the fixture JSON
func ComputeFixtureHash(jsonData string) string {
	hash := sha256.Sum256([]byte(jsonData))
	return hex.EncodeToString(hash[:])
}

// GetCurrentFixtureHashes returns a map of fixture keys to their current hashes
func (u *UpdatesService) GetCurrentFixtureHashes(ctx context.Context) (map[string]string, error) {
	type hashResult struct {
		Manufacturer  string
		Model         string
		OFLSourceHash *string
	}

	var results []hashResult
	if err := u.db.WithContext(ctx).
		Table("fixture_definitions").
		Select("manufacturer, model, ofl_source_hash").
		Where("ofl_source_hash IS NOT NULL").
		Find(&results).Error; err != nil {
		return nil, err
	}

	hashes := make(map[string]string)
	for _, r := range results {
		if r.OFLSourceHash != nil {
			key := fmt.Sprintf("%s/%s", r.Manufacturer, r.Model)
			hashes[key] = *r.OFLSourceHash
		}
	}
	return hashes, nil
}

// GetFixtureInstanceCounts returns a map of definition IDs to their instance counts
func (u *UpdatesService) GetFixtureInstanceCounts(ctx context.Context) (map[string]int, error) {
	type countResult struct {
		DefinitionID string
		Count        int
	}

	var results []countResult
	if err := u.db.WithContext(ctx).
		Table("fixture_instances").
		Select("definition_id, COUNT(*) as count").
		Group("definition_id").
		Find(&results).Error; err != nil {
		return nil, err
	}

	counts := make(map[string]int)
	for _, r := range results {
		counts[r.DefinitionID] = r.Count
	}
	return counts, nil
}

// GetDefinitionIDByManufacturerModel looks up a definition ID by manufacturer/model
func (u *UpdatesService) GetDefinitionIDByManufacturerModel(ctx context.Context, manufacturer, model string) (string, error) {
	type idResult struct {
		ID string
	}

	var result idResult
	if err := u.db.WithContext(ctx).
		Table("fixture_definitions").
		Select("id").
		Where("manufacturer = ? AND model = ?", manufacturer, model).
		First(&result).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return "", nil
		}
		return "", err
	}
	return result.ID, nil
}

// IsFixtureInUse checks if a fixture definition is used by any fixture instances
func (u *UpdatesService) IsFixtureInUse(ctx context.Context, definitionID string) (bool, int, error) {
	var count int64
	if err := u.db.WithContext(ctx).
		Table("fixture_instances").
		Where("definition_id = ?", definitionID).
		Count(&count).Error; err != nil {
		return false, 0, err
	}
	return count > 0, int(count), nil
}

// CompareFixture compares a new fixture against the database
// Returns the change type and whether it's in use
func (u *UpdatesService) CompareFixture(
	ctx context.Context,
	manufacturer string,
	model string,
	newHash string,
	currentHashes map[string]string,
	instanceCounts map[string]int,
) (*FixtureUpdate, error) {
	key := fmt.Sprintf("%s/%s", manufacturer, model)

	update := &FixtureUpdate{
		FixtureKey:   key,
		Manufacturer: manufacturer,
		Model:        model,
		NewHash:      newHash,
	}

	// Check if fixture exists
	currentHash, exists := currentHashes[key]
	if !exists {
		// New fixture
		update.ChangeType = ChangeTypeNew
		update.IsInUse = false
		update.InstanceCount = 0
		return update, nil
	}

	// Check if changed
	update.CurrentHash = &currentHash
	if currentHash == newHash {
		update.ChangeType = ChangeTypeUnchanged
	} else {
		update.ChangeType = ChangeTypeUpdated
	}

	// Check if in use
	defID, err := u.GetDefinitionIDByManufacturerModel(ctx, manufacturer, model)
	if err != nil {
		return nil, err
	}

	if defID != "" {
		count, ok := instanceCounts[defID]
		if ok && count > 0 {
			update.IsInUse = true
			update.InstanceCount = count
		}
	}

	return update, nil
}
