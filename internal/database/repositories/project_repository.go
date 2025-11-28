// Package repositories provides data access layer implementations.
package repositories

import (
	"context"

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

// FindAll returns all projects.
func (r *ProjectRepository) FindAll(ctx context.Context) ([]models.Project, error) {
	var projects []models.Project
	result := r.db.WithContext(ctx).
		Order("created_at DESC").
		Find(&projects)
	return projects, result.Error
}

// FindByID returns a project by ID.
func (r *ProjectRepository) FindByID(ctx context.Context, id string) (*models.Project, error) {
	var project models.Project
	result := r.db.WithContext(ctx).First(&project, "id = ?", id)
	if result.Error != nil {
		if result.Error == gorm.ErrRecordNotFound {
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

// Delete deletes a project by ID.
func (r *ProjectRepository) Delete(ctx context.Context, id string) error {
	return r.db.WithContext(ctx).Delete(&models.Project{}, "id = ?", id).Error
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

// CountScenes returns the number of scenes in a project.
func (r *ProjectRepository) CountScenes(ctx context.Context, projectID string) (int64, error) {
	var count int64
	result := r.db.WithContext(ctx).
		Model(&models.Scene{}).
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
