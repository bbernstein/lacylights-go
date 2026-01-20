package repositories

import (
	"context"
	"encoding/json"

	"github.com/bbernstein/lacylights-go/internal/database/models"
	"github.com/lucsky/cuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// DefaultMaxHistoryPerProject is the default maximum number of operations to keep per project.
const DefaultMaxHistoryPerProject = 100

// OperationRepository handles operation data access for undo/redo functionality.
type OperationRepository struct {
	db                   *gorm.DB
	maxHistoryPerProject int
}

// NewOperationRepository creates a new OperationRepository.
func NewOperationRepository(db *gorm.DB) *OperationRepository {
	return &OperationRepository{
		db:                   db,
		maxHistoryPerProject: DefaultMaxHistoryPerProject,
	}
}

// NewOperationRepositoryWithLimit creates a new OperationRepository with a custom history limit.
func NewOperationRepositoryWithLimit(db *gorm.DB, maxHistory int) *OperationRepository {
	return &OperationRepository{
		db:                   db,
		maxHistoryPerProject: maxHistory,
	}
}

// RecordOperation adds a new operation to the history.
// This is done within a transaction to ensure atomicity with pointer updates.
// If there are operations after the current sequence (redo candidates), they are truncated.
func (r *OperationRepository) RecordOperation(ctx context.Context, op *models.Operation) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// Get or create pointer for this project
		pointer, err := r.getOrCreatePointerTx(tx, op.ProjectID)
		if err != nil {
			return err
		}

		// Truncate any operations after current sequence (fork timeline)
		if err := r.truncateAfterSequenceTx(tx, op.ProjectID, pointer.CurrentSequence); err != nil {
			return err
		}

		// Assign sequence number and ID
		newSequence := pointer.CurrentSequence + 1
		op.Sequence = newSequence
		if op.ID == "" {
			op.ID = cuid.New()
		}

		// Create the operation
		if err := tx.Create(op).Error; err != nil {
			return err
		}

		// Update pointer
		pointer.CurrentSequence = newSequence
		pointer.MaxSequence = newSequence
		if err := tx.Save(pointer).Error; err != nil {
			return err
		}

		// Prune old operations if we exceed the limit
		return r.pruneOldOperationsTx(tx, op.ProjectID)
	})
}

// GetOperationAtSequence returns the operation at a specific sequence number.
func (r *OperationRepository) GetOperationAtSequence(ctx context.Context, projectID string, seq int) (*models.Operation, error) {
	var op models.Operation
	result := r.db.WithContext(ctx).
		Where("project_id = ? AND sequence = ?", projectID, seq).
		First(&op)
	if result.Error != nil {
		if result.Error == gorm.ErrRecordNotFound {
			return nil, nil
		}
		return nil, result.Error
	}
	return &op, nil
}

// GetOperationByID returns an operation by its ID.
func (r *OperationRepository) GetOperationByID(ctx context.Context, operationID string) (*models.Operation, error) {
	var op models.Operation
	result := r.db.WithContext(ctx).First(&op, "id = ?", operationID)
	if result.Error != nil {
		if result.Error == gorm.ErrRecordNotFound {
			return nil, nil
		}
		return nil, result.Error
	}
	return &op, nil
}

// GetOperationsByProject returns paginated operations for a project.
func (r *OperationRepository) GetOperationsByProject(ctx context.Context, projectID string, page, perPage int) ([]models.Operation, int64, error) {
	var ops []models.Operation
	var total int64

	// Count total
	if err := r.db.WithContext(ctx).Model(&models.Operation{}).
		Where("project_id = ?", projectID).
		Count(&total).Error; err != nil {
		return nil, 0, err
	}

	// Calculate offset
	offset := (page - 1) * perPage

	// Fetch paginated results, ordered by sequence descending (most recent first)
	result := r.db.WithContext(ctx).
		Where("project_id = ?", projectID).
		Order("sequence DESC").
		Offset(offset).
		Limit(perPage).
		Find(&ops)

	return ops, total, result.Error
}

// GetPointer returns the operation pointer for a project.
func (r *OperationRepository) GetPointer(ctx context.Context, projectID string) (*models.OperationPointer, error) {
	var pointer models.OperationPointer
	result := r.db.WithContext(ctx).Where("project_id = ?", projectID).First(&pointer)
	if result.Error != nil {
		if result.Error == gorm.ErrRecordNotFound {
			return nil, nil
		}
		return nil, result.Error
	}
	return &pointer, nil
}

// UpdatePointer updates the current sequence for a project.
// This is typically called during undo/redo operations.
func (r *OperationRepository) UpdatePointer(ctx context.Context, projectID string, newSequence int) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		pointer, err := r.getOrCreatePointerTx(tx, projectID)
		if err != nil {
			return err
		}
		pointer.CurrentSequence = newSequence
		return tx.Save(pointer).Error
	})
}

// TruncateAfterSequence removes all operations after a given sequence.
// This is used when a new operation follows an undo (forking the timeline).
func (r *OperationRepository) TruncateAfterSequence(ctx context.Context, projectID string, sequence int) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		return r.truncateAfterSequenceTx(tx, projectID, sequence)
	})
}

// ClearHistory removes all operations for a project.
func (r *OperationRepository) ClearHistory(ctx context.Context, projectID string) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// Delete all operations
		if err := tx.Where("project_id = ?", projectID).Delete(&models.Operation{}).Error; err != nil {
			return err
		}

		// Reset pointer
		return tx.Where("project_id = ?", projectID).Delete(&models.OperationPointer{}).Error
	})
}

// DeleteByProjectID deletes all operations for a project (called on project deletion).
func (r *OperationRepository) DeleteByProjectID(ctx context.Context, projectID string) error {
	return r.ClearHistory(ctx, projectID)
}

// getOrCreatePointerTx gets or creates a pointer within a transaction.
func (r *OperationRepository) getOrCreatePointerTx(tx *gorm.DB, projectID string) (*models.OperationPointer, error) {
	var pointer models.OperationPointer
	result := tx.Where("project_id = ?", projectID).First(&pointer)
	if result.Error == gorm.ErrRecordNotFound {
		pointer = models.OperationPointer{
			ID:              cuid.New(),
			ProjectID:       projectID,
			CurrentSequence: 0,
			MaxSequence:     0,
		}
		if err := tx.Create(&pointer).Error; err != nil {
			return nil, err
		}
		return &pointer, nil
	}
	if result.Error != nil {
		return nil, result.Error
	}
	return &pointer, nil
}

// truncateAfterSequenceTx removes operations after a sequence within a transaction.
func (r *OperationRepository) truncateAfterSequenceTx(tx *gorm.DB, projectID string, sequence int) error {
	return tx.Where("project_id = ? AND sequence > ?", projectID, sequence).
		Delete(&models.Operation{}).Error
}

// pruneOldOperationsTx removes old operations if we exceed the limit.
func (r *OperationRepository) pruneOldOperationsTx(tx *gorm.DB, projectID string) error {
	var count int64
	if err := tx.Model(&models.Operation{}).
		Where("project_id = ?", projectID).
		Count(&count).Error; err != nil {
		return err
	}

	if count <= int64(r.maxHistoryPerProject) {
		return nil
	}

	// Find the sequence number to keep from (delete everything below this)
	var minSeqToKeep int
	if err := tx.Model(&models.Operation{}).
		Select("sequence").
		Where("project_id = ?", projectID).
		Order("sequence DESC").
		Offset(r.maxHistoryPerProject - 1).
		Limit(1).
		Scan(&minSeqToKeep).Error; err != nil {
		return err
	}

	// Delete operations below the minimum sequence to keep
	return tx.Where("project_id = ? AND sequence < ?", projectID, minSeqToKeep).
		Delete(&models.Operation{}).Error
}

// GetOperationForUndo returns the operation that would be undone, without modifying the pointer.
// Returns nil if there's nothing to undo.
// Use this with ConfirmUndo for atomic undo operations.
func (r *OperationRepository) GetOperationForUndo(ctx context.Context, projectID string) (*models.Operation, error) {
	pointer, err := r.GetPointer(ctx, projectID)
	if err != nil {
		return nil, err
	}
	if pointer == nil || pointer.CurrentSequence == 0 {
		return nil, nil // Nothing to undo
	}

	return r.GetOperationAtSequence(ctx, projectID, pointer.CurrentSequence)
}

// GetOperationForRedo returns the operation that would be redone, without modifying the pointer.
// Returns nil if there's nothing to redo.
// Use this with ConfirmRedo for atomic redo operations.
func (r *OperationRepository) GetOperationForRedo(ctx context.Context, projectID string) (*models.Operation, error) {
	pointer, err := r.GetPointer(ctx, projectID)
	if err != nil {
		return nil, err
	}
	if pointer == nil || pointer.CurrentSequence >= pointer.MaxSequence {
		return nil, nil // Nothing to redo
	}

	return r.GetOperationAtSequence(ctx, projectID, pointer.CurrentSequence+1)
}

// ConfirmUndo decrements the pointer after a successful undo apply.
// This should only be called after the snapshot has been successfully applied.
func (r *OperationRepository) ConfirmUndo(ctx context.Context, projectID string) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var pointer models.OperationPointer
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("project_id = ?", projectID).
			First(&pointer).Error; err != nil {
			return err
		}

		if pointer.CurrentSequence == 0 {
			return nil // Already at beginning
		}

		pointer.CurrentSequence--
		return tx.Save(&pointer).Error
	})
}

// ConfirmRedo increments the pointer after a successful redo apply.
// This should only be called after the snapshot has been successfully applied.
func (r *OperationRepository) ConfirmRedo(ctx context.Context, projectID string) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var pointer models.OperationPointer
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("project_id = ?", projectID).
			First(&pointer).Error; err != nil {
			return err
		}

		if pointer.CurrentSequence >= pointer.MaxSequence {
			return nil // Already at end
		}

		pointer.CurrentSequence++
		return tx.Save(&pointer).Error
	})
}

// Undo decrements the current sequence and returns the operation to undo.
// Returns nil if there's nothing to undo.
// DEPRECATED: Use GetOperationForUndo + ConfirmUndo for atomic operations.
func (r *OperationRepository) Undo(ctx context.Context, projectID string) (*models.Operation, error) {
	var result *models.Operation

	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// Get pointer with lock
		var pointer models.OperationPointer
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("project_id = ?", projectID).
			First(&pointer).Error; err != nil {
			if err == gorm.ErrRecordNotFound {
				return nil // No history
			}
			return err
		}

		if pointer.CurrentSequence == 0 {
			return nil // Nothing to undo
		}

		// Get the operation at current sequence
		var op models.Operation
		if err := tx.Where("project_id = ? AND sequence = ?", projectID, pointer.CurrentSequence).
			First(&op).Error; err != nil {
			if err == gorm.ErrRecordNotFound {
				return nil
			}
			return err
		}

		// Decrement pointer
		pointer.CurrentSequence--
		if err := tx.Save(&pointer).Error; err != nil {
			return err
		}

		result = &op
		return nil
	})

	return result, err
}

// Redo increments the current sequence and returns the operation to redo.
// Returns nil if there's nothing to redo.
// DEPRECATED: Use GetOperationForRedo + ConfirmRedo for atomic operations.
func (r *OperationRepository) Redo(ctx context.Context, projectID string) (*models.Operation, error) {
	var result *models.Operation

	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// Get pointer with lock
		var pointer models.OperationPointer
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("project_id = ?", projectID).
			First(&pointer).Error; err != nil {
			if err == gorm.ErrRecordNotFound {
				return nil // No history
			}
			return err
		}

		if pointer.CurrentSequence >= pointer.MaxSequence {
			return nil // Nothing to redo
		}

		nextSequence := pointer.CurrentSequence + 1

		// Get the operation at next sequence
		var op models.Operation
		if err := tx.Where("project_id = ? AND sequence = ?", projectID, nextSequence).
			First(&op).Error; err != nil {
			if err == gorm.ErrRecordNotFound {
				return nil
			}
			return err
		}

		// Increment pointer
		pointer.CurrentSequence = nextSequence
		if err := tx.Save(&pointer).Error; err != nil {
			return err
		}

		result = &op
		return nil
	})

	return result, err
}

// GetUndoRedoStatus returns the current undo/redo status for a project.
type UndoRedoStatus struct {
	ProjectID       string
	CanUndo         bool
	CanRedo         bool
	CurrentSequence int
	MaxSequence     int
	TotalOperations int64
	UndoDescription string
	RedoDescription string
}

// GetUndoRedoStatus returns the current status for a project.
func (r *OperationRepository) GetUndoRedoStatus(ctx context.Context, projectID string) (*UndoRedoStatus, error) {
	status := &UndoRedoStatus{
		ProjectID: projectID,
	}

	pointer, err := r.GetPointer(ctx, projectID)
	if err != nil {
		return nil, err
	}

	if pointer == nil {
		// No history yet
		return status, nil
	}

	status.CurrentSequence = pointer.CurrentSequence
	status.MaxSequence = pointer.MaxSequence
	status.CanUndo = pointer.CurrentSequence > 0
	status.CanRedo = pointer.CurrentSequence < pointer.MaxSequence

	// Count total operations
	if err := r.db.WithContext(ctx).Model(&models.Operation{}).
		Where("project_id = ?", projectID).
		Count(&status.TotalOperations).Error; err != nil {
		return nil, err
	}

	// Get undo description
	if status.CanUndo {
		undoOp, err := r.GetOperationAtSequence(ctx, projectID, pointer.CurrentSequence)
		if err != nil {
			return nil, err
		}
		if undoOp != nil {
			status.UndoDescription = undoOp.Description
		}
	}

	// Get redo description
	if status.CanRedo {
		redoOp, err := r.GetOperationAtSequence(ctx, projectID, pointer.CurrentSequence+1)
		if err != nil {
			return nil, err
		}
		if redoOp != nil {
			status.RedoDescription = redoOp.Description
		}
	}

	return status, nil
}

// Helper to marshal state to JSON string
func MarshalState(state interface{}) (string, error) {
	if state == nil {
		return "", nil
	}
	data, err := json.Marshal(state)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

// Helper to unmarshal state from JSON string
func UnmarshalState(jsonStr string, target interface{}) error {
	if jsonStr == "" {
		return nil
	}
	return json.Unmarshal([]byte(jsonStr), target)
}
