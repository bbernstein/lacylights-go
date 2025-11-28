package repositories

import (
	"context"

	"github.com/bbernstein/lacylights-go/internal/database/models"
	"github.com/lucsky/cuid"
	"gorm.io/gorm"
)

// CueListRepository handles cue list data access.
type CueListRepository struct {
	db *gorm.DB
}

// NewCueListRepository creates a new CueListRepository.
func NewCueListRepository(db *gorm.DB) *CueListRepository {
	return &CueListRepository{db: db}
}

// FindByProjectID returns all cue lists in a project.
func (r *CueListRepository) FindByProjectID(ctx context.Context, projectID string) ([]models.CueList, error) {
	var cueLists []models.CueList
	result := r.db.WithContext(ctx).
		Where("project_id = ?", projectID).
		Order("created_at DESC").
		Find(&cueLists)
	return cueLists, result.Error
}

// FindByID returns a cue list by ID.
func (r *CueListRepository) FindByID(ctx context.Context, id string) (*models.CueList, error) {
	var cueList models.CueList
	result := r.db.WithContext(ctx).First(&cueList, "id = ?", id)
	if result.Error != nil {
		if result.Error == gorm.ErrRecordNotFound {
			return nil, nil
		}
		return nil, result.Error
	}
	return &cueList, nil
}

// Create creates a new cue list.
func (r *CueListRepository) Create(ctx context.Context, cueList *models.CueList) error {
	if cueList.ID == "" {
		cueList.ID = cuid.New()
	}
	return r.db.WithContext(ctx).Create(cueList).Error
}

// Update updates an existing cue list.
func (r *CueListRepository) Update(ctx context.Context, cueList *models.CueList) error {
	return r.db.WithContext(ctx).Save(cueList).Error
}

// Delete deletes a cue list by ID.
func (r *CueListRepository) Delete(ctx context.Context, id string) error {
	return r.db.WithContext(ctx).Delete(&models.CueList{}, "id = ?", id).Error
}

// GetCues returns all cues in a cue list.
func (r *CueListRepository) GetCues(ctx context.Context, cueListID string) ([]models.Cue, error) {
	var cues []models.Cue
	result := r.db.WithContext(ctx).
		Where("cue_list_id = ?", cueListID).
		Order("cue_number ASC").
		Find(&cues)
	return cues, result.Error
}

// CountCues returns the number of cues in a cue list.
func (r *CueListRepository) CountCues(ctx context.Context, cueListID string) (int64, error) {
	var count int64
	result := r.db.WithContext(ctx).
		Model(&models.Cue{}).
		Where("cue_list_id = ?", cueListID).
		Count(&count)
	return count, result.Error
}

// CueRepository handles cue data access.
type CueRepository struct {
	db *gorm.DB
}

// NewCueRepository creates a new CueRepository.
func NewCueRepository(db *gorm.DB) *CueRepository {
	return &CueRepository{db: db}
}

// FindByID returns a cue by ID.
func (r *CueRepository) FindByID(ctx context.Context, id string) (*models.Cue, error) {
	var cue models.Cue
	result := r.db.WithContext(ctx).First(&cue, "id = ?", id)
	if result.Error != nil {
		if result.Error == gorm.ErrRecordNotFound {
			return nil, nil
		}
		return nil, result.Error
	}
	return &cue, nil
}

// Create creates a new cue.
func (r *CueRepository) Create(ctx context.Context, cue *models.Cue) error {
	if cue.ID == "" {
		cue.ID = cuid.New()
	}
	return r.db.WithContext(ctx).Create(cue).Error
}

// Update updates an existing cue.
func (r *CueRepository) Update(ctx context.Context, cue *models.Cue) error {
	return r.db.WithContext(ctx).Save(cue).Error
}

// Delete deletes a cue by ID.
func (r *CueRepository) Delete(ctx context.Context, id string) error {
	return r.db.WithContext(ctx).Delete(&models.Cue{}, "id = ?", id).Error
}

// DeleteByCueListID deletes all cues in a cue list.
func (r *CueRepository) DeleteByCueListID(ctx context.Context, cueListID string) error {
	return r.db.WithContext(ctx).Delete(&models.Cue{}, "cue_list_id = ?", cueListID).Error
}

// FindByCueListID returns all cues in a cue list ordered by cue number.
func (r *CueRepository) FindByCueListID(ctx context.Context, cueListID string) ([]models.Cue, error) {
	var cues []models.Cue
	result := r.db.WithContext(ctx).
		Where("cue_list_id = ?", cueListID).
		Order("cue_number ASC").
		Find(&cues)
	return cues, result.Error
}
