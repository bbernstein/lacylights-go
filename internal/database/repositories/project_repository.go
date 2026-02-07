// Package repositories provides data access layer implementations.
package repositories

import (
	"context"
	"errors"
	"slices"
	"time"

	"github.com/bbernstein/lacylights-go/internal/database/models"
	"github.com/lucsky/cuid"
	"gorm.io/gorm"
)

// ProjectRepository handles project data access.
type ProjectRepository struct {
	db *gorm.DB
}

// NewProjectRepository creates a new ProjectRepository.
func NewProjectRepository(db *gorm.DB) *ProjectRepository {
	return &ProjectRepository{db: db}
}

// FindAll returns all non-deleted projects.
func (r *ProjectRepository) FindAll(ctx context.Context) ([]models.Project, error) {
	var projects []models.Project
	result := r.db.WithContext(ctx).
		Where("deleted_at IS NULL").
		Order("created_at DESC").
		Find(&projects)
	return projects, result.Error
}

// FindAllDeleted returns all soft-deleted projects.
func (r *ProjectRepository) FindAllDeleted(ctx context.Context) ([]models.Project, error) {
	var projects []models.Project
	result := r.db.WithContext(ctx).
		Where("deleted_at IS NOT NULL").
		Order("deleted_at DESC").
		Find(&projects)
	return projects, result.Error
}

// FindByID returns a non-deleted project by ID.
func (r *ProjectRepository) FindByID(ctx context.Context, id string) (*models.Project, error) {
	var project models.Project
	result := r.db.WithContext(ctx).
		Where("deleted_at IS NULL").
		First(&project, "id = ?", id)
	if result.Error != nil {
		if errors.Is(result.Error, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, result.Error
	}
	return &project, nil
}

// FindByIDIncludingDeleted returns a project by ID regardless of deleted status.
func (r *ProjectRepository) FindByIDIncludingDeleted(ctx context.Context, id string) (*models.Project, error) {
	var project models.Project
	result := r.db.WithContext(ctx).First(&project, "id = ?", id)
	if result.Error != nil {
		if errors.Is(result.Error, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, result.Error
	}
	return &project, nil
}

// Create creates a new project.
func (r *ProjectRepository) Create(ctx context.Context, project *models.Project) error {
	if project.ID == "" {
		project.ID = cuid.New()
	}
	return r.db.WithContext(ctx).Create(project).Error
}

// Update updates an existing project.
func (r *ProjectRepository) Update(ctx context.Context, project *models.Project) error {
	return r.db.WithContext(ctx).Save(project).Error
}

// Delete soft-deletes a project by setting deleted_at timestamp.
func (r *ProjectRepository) Delete(ctx context.Context, id string) error {
	now := time.Now()
	return r.db.WithContext(ctx).
		Model(&models.Project{}).
		Where("id = ?", id).
		Update("deleted_at", now).Error
}

// Restore restores a soft-deleted project by clearing deleted_at.
func (r *ProjectRepository) Restore(ctx context.Context, id string) error {
	return r.db.WithContext(ctx).
		Model(&models.Project{}).
		Where("id = ?", id).
		Update("deleted_at", nil).Error
}

// PermanentDelete permanently removes a project and all its data.
// This should only be called for projects that are already soft-deleted.
func (r *ProjectRepository) PermanentDelete(ctx context.Context, id string) error {
	return r.db.WithContext(ctx).Unscoped().Delete(&models.Project{}, "id = ?", id).Error
}

// CountFixtures returns the number of fixtures in a project.
func (r *ProjectRepository) CountFixtures(ctx context.Context, projectID string) (int64, error) {
	var count int64
	result := r.db.WithContext(ctx).
		Model(&models.FixtureInstance{}).
		Where("project_id = ?", projectID).
		Count(&count)
	return count, result.Error
}

// CountLooks returns the number of looks in a project.
func (r *ProjectRepository) CountLooks(ctx context.Context, projectID string) (int64, error) {
	var count int64
	result := r.db.WithContext(ctx).
		Model(&models.Look{}).
		Where("project_id = ?", projectID).
		Count(&count)
	return count, result.Error
}

// CountCueLists returns the number of cue lists in a project.
func (r *ProjectRepository) CountCueLists(ctx context.Context, projectID string) (int64, error) {
	var count int64
	result := r.db.WithContext(ctx).
		Model(&models.CueList{}).
		Where("project_id = ?", projectID).
		Count(&count)
	return count, result.Error
}

// FindAllByGroupIDs returns non-deleted projects that belong to any of the given groups.
// If groupIDs is nil, returns all non-deleted projects (no filtering).
// If groupIDs is empty, returns projects with no group (legacy auth-off projects).
func (r *ProjectRepository) FindAllByGroupIDs(ctx context.Context, groupIDs []string) ([]models.Project, error) {
	var projects []models.Project
	query := r.db.WithContext(ctx).Where("deleted_at IS NULL")

	if groupIDs == nil {
		// nil means no filtering (admin or auth-off)
		query = query.Order("created_at DESC")
	} else if len(groupIDs) == 0 {
		// Empty slice means user has no groups - show only unowned projects
		query = query.Where("group_id IS NULL").Order("created_at DESC")
	} else {
		// Filter by group IDs only - legacy (NULL group_id) projects are only accessible via nil groupIDs (admin/auth-off)
		query = query.Where("group_id IN ?", groupIDs).Order("created_at DESC")
	}

	result := query.Find(&projects)
	return projects, result.Error
}

// FindByIDWithGroupCheck returns a non-deleted project by ID, verifying group access.
// If groupIDs is nil, no group check is performed (admin or auth-off).
// Returns nil if the project exists but the user doesn't have group access.
func (r *ProjectRepository) FindByIDWithGroupCheck(ctx context.Context, id string, groupIDs []string) (*models.Project, error) {
	var project models.Project
	query := r.db.WithContext(ctx).Where("deleted_at IS NULL")

	result := query.First(&project, "id = ?", id)
	if result.Error != nil {
		if errors.Is(result.Error, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, result.Error
	}

	// No filtering needed
	if groupIDs == nil {
		return &project, nil
	}

	// Projects without a group (legacy) require admin access (nil groupIDs) - deny to regular users
	if project.GroupID == nil {
		return nil, nil
	}

	// Check if the project's group is in the user's groups
	if slices.Contains(groupIDs, *project.GroupID) {
		return &project, nil
	}

	return nil, nil // Not accessible
}
