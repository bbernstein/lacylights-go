package repositories

import (
	"context"
	"errors"
	"fmt"

	"github.com/bbernstein/lacylights-go/internal/database/models"
	"github.com/lucsky/cuid"
	"gorm.io/gorm"
)

// FixtureGroupRepository handles fixture-group data access.
type FixtureGroupRepository struct {
	db *gorm.DB
}

// NewFixtureGroupRepository creates a new FixtureGroupRepository.
func NewFixtureGroupRepository(db *gorm.DB) *FixtureGroupRepository {
	return &FixtureGroupRepository{db: db}
}

// Create creates a new fixture group (no members).
func (r *FixtureGroupRepository) Create(ctx context.Context, g *models.FixtureGroup) error {
	if g.ID == "" {
		g.ID = cuid.New()
	}
	if g.RefID == "" {
		g.RefID = g.ID
	}
	return r.db.WithContext(ctx).Create(g).Error
}

// FindByID returns a group by primary key. (nil, nil) on miss.
func (r *FixtureGroupRepository) FindByID(ctx context.Context, id string) (*models.FixtureGroup, error) {
	var g models.FixtureGroup
	result := r.db.WithContext(ctx).First(&g, "id = ?", id)
	if result.Error != nil {
		if errors.Is(result.Error, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, result.Error
	}
	return &g, nil
}

// FindByProjectID returns all groups in a project, sorted by ProjectOrder
// (NULL last) then Name.
func (r *FixtureGroupRepository) FindByProjectID(ctx context.Context, projectID string) ([]models.FixtureGroup, error) {
	var groups []models.FixtureGroup
	result := r.db.WithContext(ctx).
		Where("project_id = ?", projectID).
		Order("project_order IS NULL, project_order ASC, name ASC, id ASC").
		Find(&groups)
	return groups, result.Error
}

// FindByRefID resolves a group by (project, refID).
func (r *FixtureGroupRepository) FindByRefID(ctx context.Context, projectID, refID string) (*models.FixtureGroup, error) {
	var g models.FixtureGroup
	result := r.db.WithContext(ctx).
		Where("project_id = ? AND ref_id = ?", projectID, refID).
		First(&g)
	if result.Error != nil {
		if errors.Is(result.Error, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, result.Error
	}
	return &g, nil
}

// FindByEosNumber resolves a group by (project, eosNumber).
func (r *FixtureGroupRepository) FindByEosNumber(ctx context.Context, projectID string, eosNumber int) (*models.FixtureGroup, error) {
	var g models.FixtureGroup
	result := r.db.WithContext(ctx).
		Where("project_id = ? AND eos_number = ?", projectID, eosNumber).
		First(&g)
	if result.Error != nil {
		if errors.Is(result.Error, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, result.Error
	}
	return &g, nil
}

// Update saves a group's mutable fields (name, description, eos_number,
// project_order). RefID is intentionally not part of the update set —
// it's frozen post-creation.
func (r *FixtureGroupRepository) Update(ctx context.Context, g *models.FixtureGroup) error {
	return r.db.WithContext(ctx).
		Model(&models.FixtureGroup{}).
		Where("id = ?", g.ID).
		Updates(map[string]interface{}{
			"name":          g.Name,
			"description":   g.Description,
			"eos_number":    g.EosNumber,
			"project_order": g.ProjectOrder,
		}).Error
}

// Delete deletes a group; cascade FK on members removes their rows.
func (r *FixtureGroupRepository) Delete(ctx context.Context, id string) error {
	return r.db.WithContext(ctx).Delete(&models.FixtureGroup{}, "id = ?", id).Error
}

// CreateWithMembers atomically creates a group and its members.
func (r *FixtureGroupRepository) CreateWithMembers(ctx context.Context, g *models.FixtureGroup, fixtureIDs []string) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if g.ID == "" {
			g.ID = cuid.New()
		}
		if g.RefID == "" {
			g.RefID = g.ID
		}
		if err := tx.Create(g).Error; err != nil {
			return err
		}
		return r.replaceMembersTx(tx, g.ID, fixtureIDs)
	})
}

// GetMembers returns the members of a group sorted by OrderIndex.
func (r *FixtureGroupRepository) GetMembers(ctx context.Context, groupID string) ([]models.FixtureGroupMember, error) {
	var ms []models.FixtureGroupMember
	result := r.db.WithContext(ctx).
		Where("group_id = ?", groupID).
		Order("order_index ASC, fixture_id ASC").
		Find(&ms)
	return ms, result.Error
}

// SetMembers transactionally replaces the membership list. OrderIndex is
// assigned in input-list order (0, 1, 2, ...).
func (r *FixtureGroupRepository) SetMembers(ctx context.Context, groupID string, fixtureIDs []string) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		return r.replaceMembersTx(tx, groupID, fixtureIDs)
	})
}

func (r *FixtureGroupRepository) replaceMembersTx(tx *gorm.DB, groupID string, fixtureIDs []string) error {
	if err := tx.Delete(&models.FixtureGroupMember{}, "group_id = ?", groupID).Error; err != nil {
		return err
	}
	if len(fixtureIDs) == 0 {
		return nil
	}
	rows := make([]models.FixtureGroupMember, 0, len(fixtureIDs))
	for i, fid := range fixtureIDs {
		rows = append(rows, models.FixtureGroupMember{
			GroupID:    groupID,
			FixtureID:  fid,
			OrderIndex: i,
		})
	}
	return tx.Create(&rows).Error
}

// AddMembers appends the given fixtures to the group, skipping duplicates.
// New rows get OrderIndex = current max + 1, +2, ...
func (r *FixtureGroupRepository) AddMembers(ctx context.Context, groupID string, fixtureIDs []string) error {
	if len(fixtureIDs) == 0 {
		return nil
	}
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// Find the current max OrderIndex.
		var maxOrder *int
		if err := tx.Model(&models.FixtureGroupMember{}).
			Select("MAX(order_index)").
			Where("group_id = ?", groupID).
			Scan(&maxOrder).Error; err != nil {
			return err
		}
		next := 0
		if maxOrder != nil {
			next = *maxOrder + 1
		}

		// Find existing members to skip duplicates.
		var existingIDs []string
		if err := tx.Model(&models.FixtureGroupMember{}).
			Where("group_id = ?", groupID).
			Pluck("fixture_id", &existingIDs).Error; err != nil {
			return err
		}
		existing := map[string]bool{}
		for _, id := range existingIDs {
			existing[id] = true
		}

		var rows []models.FixtureGroupMember
		for _, fid := range fixtureIDs {
			if existing[fid] {
				continue
			}
			rows = append(rows, models.FixtureGroupMember{
				GroupID:    groupID,
				FixtureID:  fid,
				OrderIndex: next,
			})
			next++
			existing[fid] = true
		}
		if len(rows) == 0 {
			return nil
		}
		return tx.Create(&rows).Error
	})
}

// RemoveMembers deletes the listed fixtures from the group; missing IDs
// are silently ignored. Surviving members keep their OrderIndex.
func (r *FixtureGroupRepository) RemoveMembers(ctx context.Context, groupID string, fixtureIDs []string) error {
	if len(fixtureIDs) == 0 {
		return nil
	}
	return r.db.WithContext(ctx).
		Where("group_id = ? AND fixture_id IN ?", groupID, fixtureIDs).
		Delete(&models.FixtureGroupMember{}).Error
}

// ReorderMembers updates OrderIndex according to the supplied list. Any
// fixtureIDs not currently members are silently ignored. Any current
// members not listed retain their existing OrderIndex.
func (r *FixtureGroupRepository) ReorderMembers(ctx context.Context, groupID string, fixtureIDs []string) error {
	if len(fixtureIDs) == 0 {
		return nil
	}
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		for i, fid := range fixtureIDs {
			res := tx.Model(&models.FixtureGroupMember{}).
				Where("group_id = ? AND fixture_id = ?", groupID, fid).
				Update("order_index", i)
			if res.Error != nil {
				return res.Error
			}
		}
		return nil
	})
}

// AssignNextEosNumber returns one more than the largest EosNumber in the
// project, or 1 if no group has an assigned number. It does NOT mutate
// state — callers persist via Update.
func (r *FixtureGroupRepository) AssignNextEosNumber(ctx context.Context, projectID string) (int, error) {
	var maxNum *int
	err := r.db.WithContext(ctx).
		Model(&models.FixtureGroup{}).
		Where("project_id = ?", projectID).
		Select("MAX(eos_number)").
		Scan(&maxNum).Error
	if err != nil {
		return 0, fmt.Errorf("max eos_number: %w", err)
	}
	if maxNum == nil {
		return 1, nil
	}
	return *maxNum + 1, nil
}

// AssignAndPersistMissingEosNumbers fills in EosNumber for any group in the
// supplied slice that has nil EosNumber, persisting each new value in a
// single transaction. Mutates the in-memory groups so callers see the
// assigned numbers without re-reading.
//
// Concurrency: SQLite's deferred transactions don't serialize the initial
// MAX(eos_number) read against parallel writers, so two concurrent
// exporters can theoretically both pick the same next number and one
// will fail the (project_id, eos_number) partial unique index. In
// practice this is vanishingly unlikely (export is a user-initiated
// action; nobody fires two exports of the same project concurrently).
// If it does happen, the constraint violation surfaces as an export
// error and the user re-runs — the second run succeeds because the
// first run's numbers are now persisted.
func (r *FixtureGroupRepository) AssignAndPersistMissingEosNumbers(ctx context.Context, projectID string, groups []models.FixtureGroup) error {
	hasMissing := false
	for i := range groups {
		if groups[i].EosNumber == nil {
			hasMissing = true
			break
		}
	}
	if !hasMissing {
		return nil
	}
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var maxNum *int
		if err := tx.Model(&models.FixtureGroup{}).
			Where("project_id = ?", projectID).
			Select("MAX(eos_number)").
			Scan(&maxNum).Error; err != nil {
			return fmt.Errorf("max eos_number: %w", err)
		}
		next := 1
		if maxNum != nil {
			next = *maxNum + 1
		}
		for i := range groups {
			if groups[i].EosNumber != nil {
				continue
			}
			n := next
			next++
			if err := tx.Model(&models.FixtureGroup{}).
				Where("id = ?", groups[i].ID).
				Update("eos_number", n).Error; err != nil {
				return fmt.Errorf("persist eos_number for group %s: %w", groups[i].ID, err)
			}
			groups[i].EosNumber = &n
		}
		return nil
	})
}
