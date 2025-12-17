package repositories

import (
	"context"
	"errors"

	"github.com/bbernstein/lacylights-go/internal/database/models"
	"github.com/lucsky/cuid"
	"gorm.io/gorm"
)

// SceneBoardRepository handles scene board data access.
type SceneBoardRepository struct {
	db *gorm.DB
}

// NewSceneBoardRepository creates a new SceneBoardRepository.
func NewSceneBoardRepository(db *gorm.DB) *SceneBoardRepository {
	return &SceneBoardRepository{db: db}
}

// FindByProjectID returns all scene boards in a project.
func (r *SceneBoardRepository) FindByProjectID(ctx context.Context, projectID string) ([]models.SceneBoard, error) {
	var boards []models.SceneBoard
	result := r.db.WithContext(ctx).
		Where("project_id = ?", projectID).
		Order("created_at DESC").
		Find(&boards)
	return boards, result.Error
}

// FindByID returns a scene board by ID.
func (r *SceneBoardRepository) FindByID(ctx context.Context, id string) (*models.SceneBoard, error) {
	var board models.SceneBoard
	result := r.db.WithContext(ctx).First(&board, "id = ?", id)
	if result.Error != nil {
		if errors.Is(result.Error, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, result.Error
	}
	return &board, nil
}

// Create creates a new scene board.
func (r *SceneBoardRepository) Create(ctx context.Context, board *models.SceneBoard) error {
	if board.ID == "" {
		board.ID = cuid.New()
	}
	return r.db.WithContext(ctx).Create(board).Error
}

// Update updates an existing scene board.
func (r *SceneBoardRepository) Update(ctx context.Context, board *models.SceneBoard) error {
	return r.db.WithContext(ctx).Save(board).Error
}

// Delete deletes a scene board by ID, including all associated buttons.
// Uses a transaction to ensure atomicity.
func (r *SceneBoardRepository) Delete(ctx context.Context, id string) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// Delete associated buttons first
		if err := tx.Delete(&models.SceneBoardButton{}, "scene_board_id = ?", id).Error; err != nil {
			return err
		}
		// Then delete the scene board
		return tx.Delete(&models.SceneBoard{}, "id = ?", id).Error
	})
}

// GetButtons returns all buttons for a scene board.
func (r *SceneBoardRepository) GetButtons(ctx context.Context, boardID string) ([]models.SceneBoardButton, error) {
	var buttons []models.SceneBoardButton
	result := r.db.WithContext(ctx).
		Where("scene_board_id = ?", boardID).
		Order("layout_y ASC, layout_x ASC").
		Find(&buttons)
	return buttons, result.Error
}

// CreateButton creates a scene board button.
func (r *SceneBoardRepository) CreateButton(ctx context.Context, button *models.SceneBoardButton) error {
	if button.ID == "" {
		button.ID = cuid.New()
	}
	return r.db.WithContext(ctx).Create(button).Error
}

// CreateButtons creates multiple scene board buttons.
func (r *SceneBoardRepository) CreateButtons(ctx context.Context, buttons []models.SceneBoardButton) error {
	if len(buttons) == 0 {
		return nil
	}
	for i := range buttons {
		if buttons[i].ID == "" {
			buttons[i].ID = cuid.New()
		}
	}
	return r.db.WithContext(ctx).Create(&buttons).Error
}

// DeleteButtons deletes all buttons for a scene board.
func (r *SceneBoardRepository) DeleteButtons(ctx context.Context, boardID string) error {
	return r.db.WithContext(ctx).Delete(&models.SceneBoardButton{}, "scene_board_id = ?", boardID).Error
}

// CreateWithButtons creates a scene board with its buttons in a transaction.
func (r *SceneBoardRepository) CreateWithButtons(ctx context.Context, board *models.SceneBoard, buttons []models.SceneBoardButton) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if board.ID == "" {
			board.ID = cuid.New()
		}
		if err := tx.Create(board).Error; err != nil {
			return err
		}

		if len(buttons) > 0 {
			for i := range buttons {
				if buttons[i].ID == "" {
					buttons[i].ID = cuid.New()
				}
				buttons[i].SceneBoardID = board.ID
			}
			if err := tx.Create(&buttons).Error; err != nil {
				return err
			}
		}
		return nil
	})
}
