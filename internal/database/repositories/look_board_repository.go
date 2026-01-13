package repositories

import (
	"context"
	"errors"

	"github.com/bbernstein/lacylights-go/internal/database/models"
	"github.com/lucsky/cuid"
	"gorm.io/gorm"
)

// LookBoardRepository handles scene board data access.
type LookBoardRepository struct {
	db *gorm.DB
}

// NewLookBoardRepository creates a new LookBoardRepository.
func NewLookBoardRepository(db *gorm.DB) *LookBoardRepository {
	return &LookBoardRepository{db: db}
}

// FindByProjectID returns all look boards in a project.
func (r *LookBoardRepository) FindByProjectID(ctx context.Context, projectID string) ([]models.LookBoard, error) {
	var boards []models.LookBoard
	result := r.db.WithContext(ctx).
		Where("project_id = ?", projectID).
		Order("created_at DESC").
		Find(&boards)
	return boards, result.Error
}

// FindByID returns a look board by ID.
func (r *LookBoardRepository) FindByID(ctx context.Context, id string) (*models.LookBoard, error) {
	var board models.LookBoard
	result := r.db.WithContext(ctx).First(&board, "id = ?", id)
	if result.Error != nil {
		if errors.Is(result.Error, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, result.Error
	}
	return &board, nil
}

// Create creates a new look board.
func (r *LookBoardRepository) Create(ctx context.Context, board *models.LookBoard) error {
	if board.ID == "" {
		board.ID = cuid.New()
	}
	return r.db.WithContext(ctx).Create(board).Error
}

// Update updates an existing look board.
func (r *LookBoardRepository) Update(ctx context.Context, board *models.LookBoard) error {
	return r.db.WithContext(ctx).Save(board).Error
}

// Delete deletes a look board by ID, including all associated buttons.
// Uses a transaction to ensure atomicity.
func (r *LookBoardRepository) Delete(ctx context.Context, id string) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// Delete associated buttons first
		if err := tx.Delete(&models.LookBoardButton{}, "look_board_id = ?", id).Error; err != nil {
			return err
		}
		// Then delete the look board
		return tx.Delete(&models.LookBoard{}, "id = ?", id).Error
	})
}

// GetButtons returns all buttons for a look board.
func (r *LookBoardRepository) GetButtons(ctx context.Context, boardID string) ([]models.LookBoardButton, error) {
	var buttons []models.LookBoardButton
	result := r.db.WithContext(ctx).
		Where("look_board_id = ?", boardID).
		Order("layout_y ASC, layout_x ASC").
		Find(&buttons)
	return buttons, result.Error
}

// CreateButton creates a look board button.
func (r *LookBoardRepository) CreateButton(ctx context.Context, button *models.LookBoardButton) error {
	if button.ID == "" {
		button.ID = cuid.New()
	}
	return r.db.WithContext(ctx).Create(button).Error
}

// CreateButtons creates multiple look board buttons.
func (r *LookBoardRepository) CreateButtons(ctx context.Context, buttons []models.LookBoardButton) error {
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

// DeleteButtons deletes all buttons for a look board.
func (r *LookBoardRepository) DeleteButtons(ctx context.Context, boardID string) error {
	return r.db.WithContext(ctx).Delete(&models.LookBoardButton{}, "look_board_id = ?", boardID).Error
}

// CreateWithButtons creates a look board with its buttons in a transaction.
func (r *LookBoardRepository) CreateWithButtons(ctx context.Context, board *models.LookBoard, buttons []models.LookBoardButton) error {
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
				buttons[i].LookBoardID = board.ID
			}
			if err := tx.Create(&buttons).Error; err != nil {
				return err
			}
		}
		return nil
	})
}

// FindButtonByID returns a look board button by ID.
func (r *LookBoardRepository) FindButtonByID(ctx context.Context, id string) (*models.LookBoardButton, error) {
	var button models.LookBoardButton
	result := r.db.WithContext(ctx).First(&button, "id = ?", id)
	if result.Error != nil {
		if errors.Is(result.Error, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, result.Error
	}
	return &button, nil
}

// UpdateButton updates an existing look board button.
func (r *LookBoardRepository) UpdateButton(ctx context.Context, button *models.LookBoardButton) error {
	return r.db.WithContext(ctx).Save(button).Error
}

// DeleteButton deletes a single look board button by ID.
func (r *LookBoardRepository) DeleteButton(ctx context.Context, id string) error {
	return r.db.WithContext(ctx).Delete(&models.LookBoardButton{}, "id = ?", id).Error
}
