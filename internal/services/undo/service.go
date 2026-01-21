// Package undo provides undo/redo functionality for LacyLights operations.
package undo

import (
	"context"
	"encoding/json"
	"fmt"
	"log"

	"github.com/bbernstein/lacylights-go/internal/database/models"
	"github.com/bbernstein/lacylights-go/internal/database/repositories"
	"github.com/bbernstein/lacylights-go/internal/services/pubsub"
)

// EntityType represents the type of entity being operated on.
type EntityType string

const (
	EntityTypeLook                EntityType = "Look"
	EntityTypeFixtureInstance     EntityType = "FixtureInstance"
	EntityTypeCue                 EntityType = "Cue"
	EntityTypeCueList             EntityType = "CueList"
	EntityTypeLookBoard           EntityType = "LookBoard"
	EntityTypeLookBoardButton     EntityType = "LookBoardButton"
	EntityTypeEffect              EntityType = "Effect"
	EntityTypeCueEffect           EntityType = "CueEffect"
	EntityTypeProject             EntityType = "Project"
	EntityTypeCuePlayback         EntityType = "CuePlayback"
	EntityTypeBulkFixturePosition EntityType = "BulkFixturePosition"
	EntityTypeBulkLookUpdate      EntityType = "BulkLookUpdate"
)

// OperationType represents the type of operation performed.
type OperationType string

const (
	OperationTypeCreate OperationType = "CREATE"
	OperationTypeUpdate OperationType = "UPDATE"
	OperationTypeDelete OperationType = "DELETE"
	OperationTypeBulk   OperationType = "BULK"
)

// UndoRedoResult contains the result of an undo or redo operation.
type UndoRedoResult struct {
	Success          bool
	Operation        *models.Operation
	Message          string
	RestoredEntityID string
}

// Service provides undo/redo functionality.
type Service struct {
	opRepo             *repositories.OperationRepository
	lookRepo           *repositories.LookRepository
	fixtureRepo        *repositories.FixtureRepository
	cueRepo            *repositories.CueRepository
	cueListRepo        *repositories.CueListRepository
	lookBoardRepo      *repositories.LookBoardRepository
	effectRepo         *repositories.EffectRepository
	projectRepo        *repositories.ProjectRepository
	pubSub             *pubsub.PubSub
	playbackController PlaybackController
}

// NewService creates a new undo service.
func NewService(
	opRepo *repositories.OperationRepository,
	lookRepo *repositories.LookRepository,
	fixtureRepo *repositories.FixtureRepository,
	cueRepo *repositories.CueRepository,
	cueListRepo *repositories.CueListRepository,
	lookBoardRepo *repositories.LookBoardRepository,
	effectRepo *repositories.EffectRepository,
	projectRepo *repositories.ProjectRepository,
	pubSub *pubsub.PubSub,
) *Service {
	return &Service{
		opRepo:        opRepo,
		lookRepo:      lookRepo,
		fixtureRepo:   fixtureRepo,
		cueRepo:       cueRepo,
		cueListRepo:   cueListRepo,
		lookBoardRepo: lookBoardRepo,
		effectRepo:    effectRepo,
		projectRepo:   projectRepo,
		pubSub:        pubSub,
	}
}

// SetPlaybackController sets the playback controller for cue playback undo/redo.
// This is called after initialization to avoid circular dependencies.
func (s *Service) SetPlaybackController(pc PlaybackController) {
	s.playbackController = pc
}

// LookSnapshot captures the complete state of a look for undo/redo.
type LookSnapshot struct {
	Look          *models.Look          `json:"look"`
	FixtureValues []models.FixtureValue `json:"fixtureValues"`
}

// FixtureInstanceSnapshot captures the complete state of a fixture instance.
type FixtureInstanceSnapshot struct {
	Fixture  *models.FixtureInstance `json:"fixture"`
	Channels []models.InstanceChannel `json:"channels"`
}

// CueSnapshot captures the complete state of a cue.
type CueSnapshot struct {
	Cue *models.Cue `json:"cue"`
}

// CueListSnapshot captures the complete state of a cue list.
type CueListSnapshot struct {
	CueList *models.CueList `json:"cueList"`
	Cues    []models.Cue    `json:"cues"`
}

// LookBoardSnapshot captures the complete state of a look board.
type LookBoardSnapshot struct {
	LookBoard *models.LookBoard        `json:"lookBoard"`
	Buttons   []models.LookBoardButton `json:"buttons"`
}

// EffectSnapshot captures the complete state of an effect.
type EffectSnapshot struct {
	Effect         *models.Effect         `json:"effect"`
	EffectFixtures []models.EffectFixture `json:"effectFixtures"`
	EffectChannels []models.EffectChannel `json:"effectChannels"`
}

// CueEffectSnapshot captures the state of a cue-effect relationship.
type CueEffectSnapshot struct {
	CueEffect *models.CueEffect `json:"cueEffect"`
	CueID     string            `json:"cueId"`
	EffectID  string            `json:"effectId"`
	ProjectID string            `json:"projectId"`
}

// CuePlaybackSnapshot captures the playback state for undo/redo.
// This tracks which cue was playing (or if playback was stopped).
type CuePlaybackSnapshot struct {
	CueListID   string   `json:"cueListId"`
	ProjectID   string   `json:"projectId"`
	CueID       *string  `json:"cueId,omitempty"`       // nil if not playing
	CueNumber   *float64 `json:"cueNumber,omitempty"`   // nil if not playing
	CueName     *string  `json:"cueName,omitempty"`     // for description purposes
	FadeInTime  *float64 `json:"fadeInTime,omitempty"`  // the fade time to use when restoring
	IsPlaying   bool     `json:"isPlaying"`
	CueListName string   `json:"cueListName,omitempty"` // for description purposes
}

// FixturePositionEntry captures a single fixture's position for bulk operations.
type FixturePositionEntry struct {
	FixtureID      string   `json:"fixtureId"`
	FixtureName    string   `json:"fixtureName"`
	LayoutX        *float64 `json:"layoutX,omitempty"`
	LayoutY        *float64 `json:"layoutY,omitempty"`
	LayoutRotation *float64 `json:"layoutRotation,omitempty"`
}

// BulkFixturePositionSnapshot captures multiple fixture positions for undo/redo.
type BulkFixturePositionSnapshot struct {
	Positions []FixturePositionEntry `json:"positions"`
}

// BulkLookUpdateSnapshot captures multiple looks' state for atomic undo.
// Used by copyFixturesToLooks to restore all target looks in a single undo operation.
type BulkLookUpdateSnapshot struct {
	Looks []LookSnapshot `json:"looks"`
}

// PlaybackController defines the interface for playback operations needed by undo.
// This avoids circular dependencies with the playback package.
type PlaybackController interface {
	// GoToCueNumber jumps to a specific cue by number with optional fade time override.
	GoToCueNumber(ctx context.Context, cueListID string, cueNumber float64, fadeInTimeOverride *float64) error
	// StopCueList stops playback for a cue list.
	StopCueList(cueListID string)
}

// RecordOperation records an operation for undo/redo.
// prevState and newState should be snapshot structs (e.g., LookSnapshot).
// relatedIDs is used for bulk operations to track all affected entity IDs.
func (s *Service) RecordOperation(
	ctx context.Context,
	projectID string,
	opType OperationType,
	entityType EntityType,
	entityID string,
	description string,
	prevState interface{},
	newState interface{},
	relatedIDs []string,
) error {
	// Marshal states to JSON
	prevJSON, err := repositories.MarshalState(prevState)
	if err != nil {
		log.Printf("Warning: failed to marshal previous state for undo: %v", err)
		return err
	}

	newJSON, err := repositories.MarshalState(newState)
	if err != nil {
		log.Printf("Warning: failed to marshal new state for undo: %v", err)
		return err
	}

	// Marshal related IDs
	var relatedIDsJSON *string
	if len(relatedIDs) > 0 {
		data, err := json.Marshal(relatedIDs)
		if err != nil {
			log.Printf("Warning: failed to marshal related IDs for undo: %v", err)
			return err
		}
		str := string(data)
		relatedIDsJSON = &str
	}

	op := &models.Operation{
		ProjectID:     projectID,
		OperationType: string(opType),
		EntityType:    string(entityType),
		EntityID:      entityID,
		Description:   description,
		PreviousState: prevJSON,
		NewState:      newJSON,
		RelatedIDs:    relatedIDsJSON,
	}

	if err := s.opRepo.RecordOperation(ctx, op); err != nil {
		log.Printf("Warning: failed to record operation for undo: %v", err)
		return err
	}

	// Publish status update
	s.publishStatusUpdate(ctx, projectID)

	return nil
}

// Undo reverts the last operation for a project.
// This uses an atomic pattern: get operation, apply snapshot, then confirm pointer update.
// If the snapshot fails to apply, the pointer is not modified.
func (s *Service) Undo(ctx context.Context, projectID string) (*UndoRedoResult, error) {
	// Step 1: Get the operation without modifying the pointer
	op, err := s.opRepo.GetOperationForUndo(ctx, projectID)
	if err != nil {
		return nil, fmt.Errorf("failed to get operation for undo: %w", err)
	}

	if op == nil {
		return &UndoRedoResult{
			Success: false,
			Message: "Nothing to undo",
		}, nil
	}

	// Step 2: Apply the previous state
	entityID, err := s.applySnapshot(ctx, EntityType(op.EntityType), op.PreviousState, OperationType(op.OperationType), op.EntityID)
	if err != nil {
		// Pointer was NOT modified, so no rollback needed
		log.Printf("Warning: failed to apply undo: %v", err)
		return &UndoRedoResult{
			Success: false,
			Message: fmt.Sprintf("Failed to apply undo: %v", err),
		}, err
	}

	// Step 3: Only update the pointer after successful apply
	if err := s.opRepo.ConfirmUndo(ctx, projectID); err != nil {
		// This is a rare edge case - snapshot applied but pointer update failed
		log.Printf("Warning: undo applied but pointer update failed: %v", err)
		return &UndoRedoResult{
			Success:          true,
			Operation:        op,
			Message:          fmt.Sprintf("Undid: %s (warning: pointer sync issue)", op.Description),
			RestoredEntityID: entityID,
		}, nil
	}

	s.publishStatusUpdate(ctx, projectID)

	return &UndoRedoResult{
		Success:          true,
		Operation:        op,
		Message:          fmt.Sprintf("Undid: %s", op.Description),
		RestoredEntityID: entityID,
	}, nil
}

// Redo re-applies the last undone operation for a project.
// This uses an atomic pattern: get operation, apply snapshot, then confirm pointer update.
// If the snapshot fails to apply, the pointer is not modified.
func (s *Service) Redo(ctx context.Context, projectID string) (*UndoRedoResult, error) {
	// Step 1: Get the operation without modifying the pointer
	op, err := s.opRepo.GetOperationForRedo(ctx, projectID)
	if err != nil {
		return nil, fmt.Errorf("failed to get operation for redo: %w", err)
	}

	if op == nil {
		return &UndoRedoResult{
			Success: false,
			Message: "Nothing to redo",
		}, nil
	}

	// Step 2: Apply the new state
	entityID, err := s.applySnapshot(ctx, EntityType(op.EntityType), op.NewState, OperationType(op.OperationType), op.EntityID)
	if err != nil {
		// Pointer was NOT modified, so no rollback needed
		log.Printf("Warning: failed to apply redo: %v", err)
		return &UndoRedoResult{
			Success: false,
			Message: fmt.Sprintf("Failed to apply redo: %v", err),
		}, err
	}

	// Step 3: Only update the pointer after successful apply
	if err := s.opRepo.ConfirmRedo(ctx, projectID); err != nil {
		// This is a rare edge case - snapshot applied but pointer update failed
		log.Printf("Warning: redo applied but pointer update failed: %v", err)
		return &UndoRedoResult{
			Success:          true,
			Operation:        op,
			Message:          fmt.Sprintf("Redid: %s (warning: pointer sync issue)", op.Description),
			RestoredEntityID: entityID,
		}, nil
	}

	s.publishStatusUpdate(ctx, projectID)

	return &UndoRedoResult{
		Success:          true,
		Operation:        op,
		Message:          fmt.Sprintf("Redid: %s", op.Description),
		RestoredEntityID: entityID,
	}, nil
}

// JumpToOperation moves to a specific operation in the history.
// This applies or reverts operations to reach the target sequence.
func (s *Service) JumpToOperation(ctx context.Context, projectID, operationID string) (*UndoRedoResult, error) {
	// Get the target operation
	targetOp, err := s.opRepo.GetOperationByID(ctx, operationID)
	if err != nil {
		return nil, fmt.Errorf("failed to get target operation: %w", err)
	}
	if targetOp == nil {
		return &UndoRedoResult{
			Success: false,
			Message: "Operation not found",
		}, nil
	}

	if targetOp.ProjectID != projectID {
		return &UndoRedoResult{
			Success: false,
			Message: "Operation does not belong to this project",
		}, nil
	}

	// Get current pointer
	status, err := s.opRepo.GetUndoRedoStatus(ctx, projectID)
	if err != nil {
		return nil, fmt.Errorf("failed to get current status: %w", err)
	}

	targetSequence := targetOp.Sequence

	// If we need to go backward (undo multiple operations)
	for status.CurrentSequence > targetSequence {
		_, err := s.Undo(ctx, projectID)
		if err != nil {
			return &UndoRedoResult{
				Success: false,
				Message: fmt.Sprintf("Failed during jump: %v", err),
			}, err
		}
		status.CurrentSequence--
	}

	// If we need to go forward (redo multiple operations)
	for status.CurrentSequence < targetSequence {
		_, err := s.Redo(ctx, projectID)
		if err != nil {
			return &UndoRedoResult{
				Success: false,
				Message: fmt.Sprintf("Failed during jump: %v", err),
			}, err
		}
		status.CurrentSequence++
	}

	return &UndoRedoResult{
		Success:   true,
		Operation: targetOp,
		Message:   fmt.Sprintf("Jumped to: %s", targetOp.Description),
	}, nil
}

// GetStatus returns the current undo/redo status for a project.
func (s *Service) GetStatus(ctx context.Context, projectID string) (*repositories.UndoRedoStatus, error) {
	return s.opRepo.GetUndoRedoStatus(ctx, projectID)
}

// GetOperationHistory returns paginated operation history for a project.
func (s *Service) GetOperationHistory(ctx context.Context, projectID string, page, perPage int) ([]models.Operation, int64, error) {
	return s.opRepo.GetOperationsByProject(ctx, projectID, page, perPage)
}

// ClearHistory removes all operation history for a project.
func (s *Service) ClearHistory(ctx context.Context, projectID string) error {
	if err := s.opRepo.ClearHistory(ctx, projectID); err != nil {
		return err
	}
	s.publishStatusUpdate(ctx, projectID)
	return nil
}

// applySnapshot restores an entity from a snapshot.
// Returns the entity ID that was restored.
// entityID is needed when the snapshot is nil (e.g., undoing a CREATE means deleting the entity).
func (s *Service) applySnapshot(ctx context.Context, entityType EntityType, snapshotJSON string, opType OperationType, entityID string) (string, error) {
	switch entityType {
	case EntityTypeLook:
		return s.applyLookSnapshot(ctx, snapshotJSON, opType, entityID)
	case EntityTypeFixtureInstance:
		return s.applyFixtureSnapshot(ctx, snapshotJSON, opType, entityID)
	case EntityTypeCue:
		return s.applyCueSnapshot(ctx, snapshotJSON, opType, entityID)
	case EntityTypeCueList:
		return s.applyCueListSnapshot(ctx, snapshotJSON, opType, entityID)
	case EntityTypeLookBoard:
		return s.applyLookBoardSnapshot(ctx, snapshotJSON, opType, entityID)
	case EntityTypeEffect:
		return s.applyEffectSnapshot(ctx, snapshotJSON, opType, entityID)
	case EntityTypeCueEffect:
		return s.applyCueEffectSnapshot(ctx, snapshotJSON, opType, entityID)
	case EntityTypeCuePlayback:
		return s.applyCuePlaybackSnapshot(ctx, snapshotJSON)
	case EntityTypeBulkFixturePosition:
		return s.applyBulkFixturePositionSnapshot(ctx, snapshotJSON)
	case EntityTypeBulkLookUpdate:
		return s.applyBulkLookUpdateSnapshot(ctx, snapshotJSON)
	default:
		return "", fmt.Errorf("unsupported entity type for undo: %s", entityType)
	}
}

// applyLookSnapshot restores a look from a snapshot.
// If snapshotJSON is empty, we delete the entity (undoing a CREATE).
func (s *Service) applyLookSnapshot(ctx context.Context, snapshotJSON string, opType OperationType, entityID string) (string, error) {
	if snapshotJSON == "" {
		// Empty snapshot means we need to delete the entity (undoing a CREATE)
		if entityID != "" {
			// First delete associated fixture_values
			if err := s.lookRepo.DeleteFixtureValues(ctx, entityID); err != nil {
				return "", fmt.Errorf("failed to delete fixture values for undo: %w", err)
			}
			if err := s.lookRepo.Delete(ctx, entityID); err != nil {
				return "", fmt.Errorf("failed to delete look for undo: %w", err)
			}
		}
		return entityID, nil
	}

	var snapshot LookSnapshot
	if err := json.Unmarshal([]byte(snapshotJSON), &snapshot); err != nil {
		return "", fmt.Errorf("failed to unmarshal look snapshot: %w", err)
	}

	if snapshot.Look == nil {
		// Snapshot unmarshaled but has nil Look - delete the entity
		if entityID != "" {
			// First delete associated fixture_values
			if err := s.lookRepo.DeleteFixtureValues(ctx, entityID); err != nil {
				return "", fmt.Errorf("failed to delete fixture values for undo: %w", err)
			}
			if err := s.lookRepo.Delete(ctx, entityID); err != nil {
				return "", fmt.Errorf("failed to delete look for undo: %w", err)
			}
		}
		return entityID, nil
	}

	// Check if the look exists
	existingLook, err := s.lookRepo.FindByID(ctx, snapshot.Look.ID)
	if err != nil {
		return "", fmt.Errorf("failed to check existing look: %w", err)
	}

	if existingLook == nil {
		// Look was deleted - recreate it
		if err := s.lookRepo.CreateWithFixtureValues(ctx, snapshot.Look, snapshot.FixtureValues); err != nil {
			return "", fmt.Errorf("failed to recreate look: %w", err)
		}
	} else {
		// Look exists - update it
		// First delete existing fixture values
		if err := s.lookRepo.DeleteFixtureValues(ctx, snapshot.Look.ID); err != nil {
			return "", fmt.Errorf("failed to delete existing fixture values: %w", err)
		}

		// Update the look
		if err := s.lookRepo.Update(ctx, snapshot.Look); err != nil {
			return "", fmt.Errorf("failed to update look: %w", err)
		}

		// Recreate fixture values
		if len(snapshot.FixtureValues) > 0 {
			if err := s.lookRepo.CreateFixtureValues(ctx, snapshot.FixtureValues); err != nil {
				return "", fmt.Errorf("failed to recreate fixture values: %w", err)
			}
		}
	}

	return snapshot.Look.ID, nil
}

// applyFixtureSnapshot restores a fixture instance from a snapshot.
// If snapshotJSON is empty, we delete the entity (undoing a CREATE).
func (s *Service) applyFixtureSnapshot(ctx context.Context, snapshotJSON string, opType OperationType, entityID string) (string, error) {
	if snapshotJSON == "" {
		// Empty snapshot means we need to delete the entity (undoing a CREATE)
		if entityID != "" {
			if err := s.fixtureRepo.Delete(ctx, entityID); err != nil {
				return "", fmt.Errorf("failed to delete fixture for undo: %w", err)
			}
		}
		return entityID, nil
	}

	var snapshot FixtureInstanceSnapshot
	if err := json.Unmarshal([]byte(snapshotJSON), &snapshot); err != nil {
		return "", fmt.Errorf("failed to unmarshal fixture snapshot: %w", err)
	}

	if snapshot.Fixture == nil {
		// Snapshot unmarshaled but has nil Fixture - delete the entity
		if entityID != "" {
			if err := s.fixtureRepo.Delete(ctx, entityID); err != nil {
				return "", fmt.Errorf("failed to delete fixture for undo: %w", err)
			}
		}
		return entityID, nil
	}

	// Check if fixture exists
	existingFixture, err := s.fixtureRepo.FindByID(ctx, snapshot.Fixture.ID)
	if err != nil {
		return "", fmt.Errorf("failed to check existing fixture: %w", err)
	}

	if existingFixture == nil {
		// Fixture was deleted - recreate it with channels
		if err := s.fixtureRepo.CreateWithChannels(ctx, snapshot.Fixture, snapshot.Channels); err != nil {
			return "", fmt.Errorf("failed to recreate fixture: %w", err)
		}
	} else {
		// Fixture exists - update it and its channels
		if err := s.fixtureRepo.Update(ctx, snapshot.Fixture); err != nil {
			return "", fmt.Errorf("failed to update fixture: %w", err)
		}

		// Also update channels - delete existing and recreate from snapshot
		if err := s.fixtureRepo.DeleteInstanceChannels(ctx, snapshot.Fixture.ID); err != nil {
			return "", fmt.Errorf("failed to delete existing channels for undo: %w", err)
		}
		if len(snapshot.Channels) > 0 {
			if err := s.fixtureRepo.CreateInstanceChannels(ctx, snapshot.Channels); err != nil {
				return "", fmt.Errorf("failed to recreate channels for undo: %w", err)
			}
		}
	}

	return snapshot.Fixture.ID, nil
}

// applyCueSnapshot restores a cue from a snapshot.
// If snapshotJSON is empty, we delete the entity (undoing a CREATE).
func (s *Service) applyCueSnapshot(ctx context.Context, snapshotJSON string, opType OperationType, entityID string) (string, error) {
	if snapshotJSON == "" {
		// Empty snapshot means we need to delete the entity (undoing a CREATE)
		if entityID != "" {
			if err := s.cueRepo.Delete(ctx, entityID); err != nil {
				return "", fmt.Errorf("failed to delete cue for undo: %w", err)
			}
		}
		return entityID, nil
	}

	var snapshot CueSnapshot
	if err := json.Unmarshal([]byte(snapshotJSON), &snapshot); err != nil {
		return "", fmt.Errorf("failed to unmarshal cue snapshot: %w", err)
	}

	if snapshot.Cue == nil {
		// Snapshot unmarshaled but has nil Cue - delete the entity
		if entityID != "" {
			if err := s.cueRepo.Delete(ctx, entityID); err != nil {
				return "", fmt.Errorf("failed to delete cue for undo: %w", err)
			}
		}
		return entityID, nil
	}

	existingCue, err := s.cueRepo.FindByID(ctx, snapshot.Cue.ID)
	if err != nil {
		return "", fmt.Errorf("failed to check existing cue: %w", err)
	}

	if existingCue == nil {
		// Cue was deleted - recreate it
		if err := s.cueRepo.Create(ctx, snapshot.Cue); err != nil {
			return "", fmt.Errorf("failed to recreate cue: %w", err)
		}
	} else {
		// Cue exists - update it
		if err := s.cueRepo.Update(ctx, snapshot.Cue); err != nil {
			return "", fmt.Errorf("failed to update cue: %w", err)
		}
	}

	return snapshot.Cue.ID, nil
}

// applyCueListSnapshot restores a cue list from a snapshot.
// If snapshotJSON is empty, we delete the entity (undoing a CREATE).
//
// When the cue list exists, this function:
//   - Updates the cue list with values from the snapshot
//   - Creates or updates all cues from the snapshot
//   - Deletes any cues that exist in the database but are NOT in the snapshot
//
// This ensures proper undo behavior for operations that add/remove cues,
// such as reorder operations or cue list restores.
func (s *Service) applyCueListSnapshot(ctx context.Context, snapshotJSON string, opType OperationType, entityID string) (string, error) {
	if snapshotJSON == "" {
		// Empty snapshot means we need to delete the entity (undoing a CREATE)
		if entityID != "" {
			if err := s.cueListRepo.Delete(ctx, entityID); err != nil {
				return "", fmt.Errorf("failed to delete cue list for undo: %w", err)
			}
		}
		return entityID, nil
	}

	var snapshot CueListSnapshot
	if err := json.Unmarshal([]byte(snapshotJSON), &snapshot); err != nil {
		return "", fmt.Errorf("failed to unmarshal cue list snapshot: %w", err)
	}

	if snapshot.CueList == nil {
		// Snapshot unmarshaled but has nil CueList - delete the entity
		if entityID != "" {
			if err := s.cueListRepo.Delete(ctx, entityID); err != nil {
				return "", fmt.Errorf("failed to delete cue list for undo: %w", err)
			}
		}
		return entityID, nil
	}

	existingCueList, err := s.cueListRepo.FindByID(ctx, snapshot.CueList.ID)
	if err != nil {
		return "", fmt.Errorf("failed to check existing cue list: %w", err)
	}

	if existingCueList == nil {
		// Cue list was deleted - recreate it
		if err := s.cueListRepo.Create(ctx, snapshot.CueList); err != nil {
			return "", fmt.Errorf("failed to recreate cue list: %w", err)
		}
		// Recreate cues
		for i := range snapshot.Cues {
			if err := s.cueRepo.Create(ctx, &snapshot.Cues[i]); err != nil {
				return "", fmt.Errorf("failed to recreate cue: %w", err)
			}
		}
	} else {
		// Cue list exists - update it
		if err := s.cueListRepo.Update(ctx, snapshot.CueList); err != nil {
			return "", fmt.Errorf("failed to update cue list: %w", err)
		}

		// Build a set of cue IDs in the snapshot
		snapshotCueIDs := make(map[string]bool)
		for _, cue := range snapshot.Cues {
			snapshotCueIDs[cue.ID] = true
		}

		// Update or create cues from snapshot
		for i := range snapshot.Cues {
			cue := &snapshot.Cues[i]
			existingCue, err := s.cueRepo.FindByID(ctx, cue.ID)
			if err != nil {
				return "", fmt.Errorf("failed to check existing cue %s: %w", cue.ID, err)
			}
			if existingCue != nil {
				// Cue exists - update it
				if err := s.cueRepo.Update(ctx, cue); err != nil {
					return "", fmt.Errorf("failed to update cue: %w", err)
				}
			} else {
				// Cue doesn't exist - recreate it
				if err := s.cueRepo.Create(ctx, cue); err != nil {
					return "", fmt.Errorf("failed to recreate cue: %w", err)
				}
			}
		}

		// Delete any cues that exist in the DB but are not in the snapshot
		// This handles the case where undo needs to remove cues added after the snapshot
		currentCues, err := s.cueRepo.FindByCueListID(ctx, snapshot.CueList.ID)
		if err != nil {
			return "", fmt.Errorf("failed to fetch current cues for cue list %s: %w", snapshot.CueList.ID, err)
		}
		for _, currentCue := range currentCues {
			if !snapshotCueIDs[currentCue.ID] {
				// This cue exists but wasn't in the snapshot - delete it
				if err := s.cueRepo.Delete(ctx, currentCue.ID); err != nil {
					return "", fmt.Errorf("failed to delete orphaned cue: %w", err)
				}
			}
		}
	}

	return snapshot.CueList.ID, nil
}

// applyLookBoardSnapshot restores a look board from a snapshot.
// If snapshotJSON is empty, we delete the entity (undoing a CREATE).
func (s *Service) applyLookBoardSnapshot(ctx context.Context, snapshotJSON string, opType OperationType, entityID string) (string, error) {
	if snapshotJSON == "" {
		// Empty snapshot means we need to delete the entity (undoing a CREATE)
		if entityID != "" {
			if err := s.lookBoardRepo.Delete(ctx, entityID); err != nil {
				return "", fmt.Errorf("failed to delete look board for undo: %w", err)
			}
		}
		return entityID, nil
	}

	var snapshot LookBoardSnapshot
	if err := json.Unmarshal([]byte(snapshotJSON), &snapshot); err != nil {
		return "", fmt.Errorf("failed to unmarshal look board snapshot: %w", err)
	}

	if snapshot.LookBoard == nil {
		// Snapshot unmarshaled but has nil LookBoard - delete the entity
		if entityID != "" {
			if err := s.lookBoardRepo.Delete(ctx, entityID); err != nil {
				return "", fmt.Errorf("failed to delete look board for undo: %w", err)
			}
		}
		return entityID, nil
	}

	existingBoard, err := s.lookBoardRepo.FindByID(ctx, snapshot.LookBoard.ID)
	if err != nil {
		return "", fmt.Errorf("failed to check existing look board: %w", err)
	}

	if existingBoard == nil {
		// Look board was deleted - recreate it
		if err := s.lookBoardRepo.Create(ctx, snapshot.LookBoard); err != nil {
			return "", fmt.Errorf("failed to recreate look board: %w", err)
		}
	} else {
		// Look board exists - update it
		if err := s.lookBoardRepo.Update(ctx, snapshot.LookBoard); err != nil {
			return "", fmt.Errorf("failed to update look board: %w", err)
		}
	}

	// Delete existing buttons and recreate from snapshot
	if err := s.lookBoardRepo.DeleteButtons(ctx, snapshot.LookBoard.ID); err != nil {
		return "", fmt.Errorf("failed to delete existing buttons: %w", err)
	}
	for i := range snapshot.Buttons {
		if err := s.lookBoardRepo.CreateButton(ctx, &snapshot.Buttons[i]); err != nil {
			return "", fmt.Errorf("failed to recreate look board button: %w", err)
		}
	}

	return snapshot.LookBoard.ID, nil
}

// applyEffectSnapshot restores an effect from a snapshot.
// If snapshotJSON is empty, we delete the entity (undoing a CREATE).
func (s *Service) applyEffectSnapshot(ctx context.Context, snapshotJSON string, opType OperationType, entityID string) (string, error) {
	if snapshotJSON == "" {
		// Empty snapshot means we need to delete the entity (undoing a CREATE)
		if entityID != "" {
			if err := s.effectRepo.Delete(ctx, entityID); err != nil {
				return "", fmt.Errorf("failed to delete effect for undo: %w", err)
			}
		}
		return entityID, nil
	}

	var snapshot EffectSnapshot
	if err := json.Unmarshal([]byte(snapshotJSON), &snapshot); err != nil {
		return "", fmt.Errorf("failed to unmarshal effect snapshot: %w", err)
	}

	if snapshot.Effect == nil {
		// Snapshot unmarshaled but has nil Effect - delete the entity
		if entityID != "" {
			if err := s.effectRepo.Delete(ctx, entityID); err != nil {
				return "", fmt.Errorf("failed to delete effect for undo: %w", err)
			}
		}
		return entityID, nil
	}

	existingEffect, err := s.effectRepo.FindByID(ctx, snapshot.Effect.ID)
	if err != nil {
		return "", fmt.Errorf("failed to check existing effect: %w", err)
	}

	if existingEffect == nil {
		// Effect was deleted - recreate it
		if err := s.effectRepo.Create(ctx, snapshot.Effect); err != nil {
			return "", fmt.Errorf("failed to recreate effect: %w", err)
		}
	} else {
		// Effect exists - update it
		if err := s.effectRepo.Update(ctx, snapshot.Effect); err != nil {
			return "", fmt.Errorf("failed to update effect: %w", err)
		}
	}

	// Delete existing fixtures/channels and recreate from snapshot
	if err := s.effectRepo.DeleteEffectFixtures(ctx, snapshot.Effect.ID); err != nil {
		return "", fmt.Errorf("failed to delete existing effect fixtures: %w", err)
	}

	// Create a map to track effect fixture IDs for channel recreation
	oldToNewFixtureID := make(map[string]string)

	// Recreate effect fixtures
	for i := range snapshot.EffectFixtures {
		ef := &snapshot.EffectFixtures[i]
		oldID := ef.ID
		ef.ID = "" // Let the repo generate a new ID
		ef.EffectID = snapshot.Effect.ID
		if err := s.effectRepo.AddFixtureToEffect(ctx, ef.EffectID, ef.FixtureID, ef.PhaseOffset, ef.AmplitudeScale); err != nil {
			return "", fmt.Errorf("failed to recreate effect fixture: %w", err)
		}
		// Get the newly created effect fixture to get its ID
		newEF, err := s.effectRepo.GetEffectFixture(ctx, ef.EffectID, ef.FixtureID)
		if err != nil || newEF == nil {
			return "", fmt.Errorf("failed to get recreated effect fixture: %w", err)
		}
		// Update with additional properties if they exist
		if ef.EffectOrder != nil {
			newEF.EffectOrder = ef.EffectOrder
			if err := s.effectRepo.UpdateEffectFixture(ctx, newEF); err != nil {
				return "", fmt.Errorf("failed to update effect fixture order: %w", err)
			}
		}
		oldToNewFixtureID[oldID] = newEF.ID
	}

	// Recreate effect channels using the new fixture IDs
	for i := range snapshot.EffectChannels {
		ec := &snapshot.EffectChannels[i]
		newFixtureID, ok := oldToNewFixtureID[ec.EffectFixtureID]
		if !ok {
			// Channel references a fixture not in the snapshot - skip it
			continue
		}
		if err := s.effectRepo.AddChannelToEffectFixtureWithScales(ctx, newFixtureID, ec.ChannelOffset, ec.ChannelType, ec.AmplitudeScale, ec.FrequencyScale); err != nil {
			return "", fmt.Errorf("failed to recreate effect channel: %w", err)
		}
	}

	return snapshot.Effect.ID, nil
}

// splitEntityID splits an entity ID in "id1:id2" format into its components.
func splitEntityID(entityID string) []string {
	for i := 0; i < len(entityID); i++ {
		if entityID[i] == ':' {
			return []string{entityID[:i], entityID[i+1:]}
		}
	}
	return []string{entityID}
}

// applyCueEffectSnapshot restores a cue-effect relationship from a snapshot.
// For CREATE: empty prevState means we need to remove the cue effect (undo add)
// For DELETE: we need to recreate the cue effect (undo remove)
// entityID format: "cueID:effectID"
func (s *Service) applyCueEffectSnapshot(ctx context.Context, snapshotJSON string, opType OperationType, entityID string) (string, error) {
	// Parse entityID to get cueID and effectID (format: "cueID:effectID")
	var cueIDFromEntity, effectIDFromEntity string
	if entityID != "" {
		parts := splitEntityID(entityID)
		if len(parts) == 2 {
			cueIDFromEntity = parts[0]
			effectIDFromEntity = parts[1]
		}
	}

	if snapshotJSON == "" {
		// Empty snapshot - delete the cue effect (undoing a CREATE)
		if cueIDFromEntity != "" && effectIDFromEntity != "" {
			if err := s.effectRepo.RemoveEffectFromCue(ctx, cueIDFromEntity, effectIDFromEntity); err != nil {
				// Ignore error if it doesn't exist
				log.Printf("Warning: failed to remove cue effect for undo: %v", err)
			}
		}
		return entityID, nil
	}

	var snapshot CueEffectSnapshot
	if err := json.Unmarshal([]byte(snapshotJSON), &snapshot); err != nil {
		return "", fmt.Errorf("failed to unmarshal cue effect snapshot: %w", err)
	}

	if snapshot.CueEffect == nil {
		// Snapshot has no cue effect - remove the relationship if it exists
		cueID := snapshot.CueID
		effectID := snapshot.EffectID
		if cueID == "" {
			cueID = cueIDFromEntity
		}
		if effectID == "" {
			effectID = effectIDFromEntity
		}
		if cueID != "" && effectID != "" {
			if err := s.effectRepo.RemoveEffectFromCue(ctx, cueID, effectID); err != nil {
				// Ignore error if it doesn't exist
				log.Printf("Warning: failed to remove cue effect for undo: %v", err)
			}
		}
		return fmt.Sprintf("%s:%s", cueID, effectID), nil
	}

	// Check if cue effect already exists
	existing, err := s.effectRepo.GetCueEffect(ctx, snapshot.CueID, snapshot.EffectID)
	if err != nil {
		return "", fmt.Errorf("failed to check existing cue effect: %w", err)
	}

	if existing != nil {
		// CueEffect exists - update it
		existing.Intensity = snapshot.CueEffect.Intensity
		existing.Speed = snapshot.CueEffect.Speed
		existing.OnCueChange = snapshot.CueEffect.OnCueChange
		if err := s.effectRepo.UpdateCueEffect(ctx, existing); err != nil {
			return "", fmt.Errorf("failed to update cue effect: %w", err)
		}
	} else {
		// CueEffect doesn't exist - recreate it
		intensity := snapshot.CueEffect.Intensity
		speed := snapshot.CueEffect.Speed
		if err := s.effectRepo.AddEffectToCue(ctx, snapshot.CueID, snapshot.EffectID, intensity, speed); err != nil {
			return "", fmt.Errorf("failed to recreate cue effect: %w", err)
		}
		// Update with any additional properties
		if snapshot.CueEffect.OnCueChange != nil {
			recreated, err := s.effectRepo.GetCueEffect(ctx, snapshot.CueID, snapshot.EffectID)
			if err == nil && recreated != nil {
				recreated.OnCueChange = snapshot.CueEffect.OnCueChange
				_ = s.effectRepo.UpdateCueEffect(ctx, recreated)
			}
		}
	}

	return fmt.Sprintf("%s:%s", snapshot.CueID, snapshot.EffectID), nil
}

// publishStatusUpdate publishes an update to subscribers.
func (s *Service) publishStatusUpdate(ctx context.Context, projectID string) {
	if s.pubSub == nil {
		return
	}

	status, err := s.GetStatus(ctx, projectID)
	if err != nil {
		log.Printf("Warning: failed to get status for pubsub: %v", err)
		return
	}

	s.pubSub.Publish(pubsub.TopicOperationHistoryChanged, projectID, status)
}

// CaptureLookState captures the complete state of a look for undo.
func (s *Service) CaptureLookState(ctx context.Context, lookID string) (*LookSnapshot, error) {
	look, err := s.lookRepo.FindByID(ctx, lookID)
	if err != nil {
		return nil, err
	}
	if look == nil {
		return nil, nil
	}

	fixtureValues, err := s.lookRepo.GetFixtureValues(ctx, lookID)
	if err != nil {
		return nil, err
	}

	return &LookSnapshot{
		Look:          look,
		FixtureValues: fixtureValues,
	}, nil
}

// CaptureFixtureState captures the complete state of a fixture instance for undo.
func (s *Service) CaptureFixtureState(ctx context.Context, fixtureID string) (*FixtureInstanceSnapshot, error) {
	fixture, err := s.fixtureRepo.FindByID(ctx, fixtureID)
	if err != nil {
		return nil, err
	}
	if fixture == nil {
		return nil, nil
	}

	channels, err := s.fixtureRepo.GetInstanceChannels(ctx, fixtureID)
	if err != nil {
		return nil, err
	}

	return &FixtureInstanceSnapshot{
		Fixture:  fixture,
		Channels: channels,
	}, nil
}

// CaptureFixturePositions captures positions of multiple fixtures for bulk undo.
func (s *Service) CaptureFixturePositions(ctx context.Context, fixtureIDs []string) (*BulkFixturePositionSnapshot, error) {
	var positions []FixturePositionEntry

	for _, fixtureID := range fixtureIDs {
		fixture, err := s.fixtureRepo.FindByID(ctx, fixtureID)
		if err != nil {
			return nil, err
		}
		if fixture == nil {
			continue
		}

		positions = append(positions, FixturePositionEntry{
			FixtureID:      fixture.ID,
			FixtureName:    fixture.Name,
			LayoutX:        fixture.LayoutX,
			LayoutY:        fixture.LayoutY,
			LayoutRotation: fixture.LayoutRotation,
		})
	}

	return &BulkFixturePositionSnapshot{
		Positions: positions,
	}, nil
}

// CaptureCueState captures the complete state of a cue for undo.
func (s *Service) CaptureCueState(ctx context.Context, cueID string) (*CueSnapshot, error) {
	cue, err := s.cueRepo.FindByID(ctx, cueID)
	if err != nil {
		return nil, err
	}
	if cue == nil {
		return nil, nil
	}

	return &CueSnapshot{
		Cue: cue,
	}, nil
}

// CaptureCueListState captures the complete state of a cue list for undo.
func (s *Service) CaptureCueListState(ctx context.Context, cueListID string) (*CueListSnapshot, error) {
	cueList, err := s.cueListRepo.FindByID(ctx, cueListID)
	if err != nil {
		return nil, err
	}
	if cueList == nil {
		return nil, nil
	}

	cues, err := s.cueRepo.FindByCueListID(ctx, cueListID)
	if err != nil {
		return nil, err
	}

	return &CueListSnapshot{
		CueList: cueList,
		Cues:    cues,
	}, nil
}

// CaptureLookBoardState captures the complete state of a look board for undo.
func (s *Service) CaptureLookBoardState(ctx context.Context, lookBoardID string) (*LookBoardSnapshot, error) {
	lookBoard, err := s.lookBoardRepo.FindByID(ctx, lookBoardID)
	if err != nil {
		return nil, err
	}
	if lookBoard == nil {
		return nil, nil
	}

	buttons, err := s.lookBoardRepo.GetButtons(ctx, lookBoardID)
	if err != nil {
		return nil, err
	}

	return &LookBoardSnapshot{
		LookBoard: lookBoard,
		Buttons:   buttons,
	}, nil
}

// CaptureEffectState captures the complete state of an effect for undo.
// This includes all EffectFixtures and their EffectChannels.
func (s *Service) CaptureEffectState(ctx context.Context, effectID string) (*EffectSnapshot, error) {
	effect, err := s.effectRepo.FindByID(ctx, effectID)
	if err != nil {
		return nil, err
	}
	if effect == nil {
		return nil, nil
	}

	// Get all effect fixtures with their channels
	effectFixtures, err := s.effectRepo.GetEffectFixtures(ctx, effectID)
	if err != nil {
		return nil, err
	}

	// Collect all effect channels from all fixtures
	var effectChannels []models.EffectChannel
	for _, ef := range effectFixtures {
		channels, err := s.effectRepo.GetEffectChannels(ctx, ef.ID)
		if err != nil {
			return nil, err
		}
		effectChannels = append(effectChannels, channels...)
	}

	return &EffectSnapshot{
		Effect:         effect,
		EffectFixtures: effectFixtures,
		EffectChannels: effectChannels,
	}, nil
}

// CaptureCueEffectState captures the state of a cue-effect relationship for undo.
func (s *Service) CaptureCueEffectState(ctx context.Context, cueID, effectID string) (*CueEffectSnapshot, error) {
	cueEffect, err := s.effectRepo.GetCueEffect(ctx, cueID, effectID)
	if err != nil {
		return nil, err
	}
	if cueEffect == nil {
		return nil, nil
	}

	// Get the cue to find the project ID
	cue, err := s.cueRepo.FindByID(ctx, cueID)
	if err != nil {
		return nil, err
	}
	projectID := ""
	if cue != nil {
		// Get the cue list to find the project ID
		cueList, err := s.cueListRepo.FindByID(ctx, cue.CueListID)
		if err != nil {
			return nil, err
		}
		if cueList != nil {
			projectID = cueList.ProjectID
		}
	}

	return &CueEffectSnapshot{
		CueEffect: cueEffect,
		CueID:     cueID,
		EffectID:  effectID,
		ProjectID: projectID,
	}, nil
}

// DeleteEntityForUndo handles deletion during undo of a CREATE operation.
func (s *Service) DeleteEntityForUndo(ctx context.Context, entityType EntityType, entityID string) error {
	switch entityType {
	case EntityTypeLook:
		return s.lookRepo.Delete(ctx, entityID)
	case EntityTypeFixtureInstance:
		return s.fixtureRepo.Delete(ctx, entityID)
	case EntityTypeCue:
		return s.cueRepo.Delete(ctx, entityID)
	case EntityTypeCueList:
		return s.cueListRepo.Delete(ctx, entityID)
	case EntityTypeLookBoard:
		return s.lookBoardRepo.Delete(ctx, entityID)
	case EntityTypeEffect:
		return s.effectRepo.Delete(ctx, entityID)
	case EntityTypeCueEffect:
		// CueEffect uses composite key (cueID-effectID), entityID should be "cueID:effectID"
		// For delete, we don't need to handle this case specially - the snapshot contains
		// the cue and effect IDs for recreation
		return fmt.Errorf("CueEffect deletion should use RemoveEffectFromCue with snapshot data")
	default:
		return fmt.Errorf("unsupported entity type for delete: %s", entityType)
	}
}

// applyCuePlaybackSnapshot restores cue playback to a previous state.
// Returns (cueListID, nil) on success when playback state was restored.
// Returns ("", nil) when snapshotJSON is empty, indicating no cue list was affected
// (e.g., playback was already stopped before the operation was recorded).
func (s *Service) applyCuePlaybackSnapshot(ctx context.Context, snapshotJSON string) (string, error) {
	if s.playbackController == nil {
		return "", fmt.Errorf("playback controller not set, cannot restore cue playback")
	}

	if snapshotJSON == "" {
		// Empty snapshot means playback was already stopped - no action needed
		return "", nil
	}

	var snapshot CuePlaybackSnapshot
	if err := json.Unmarshal([]byte(snapshotJSON), &snapshot); err != nil {
		return "", fmt.Errorf("failed to unmarshal cue playback snapshot: %w", err)
	}

	if !snapshot.IsPlaying || snapshot.CueNumber == nil {
		// Snapshot indicates playback was stopped, so stop playback
		s.playbackController.StopCueList(snapshot.CueListID)
		return snapshot.CueListID, nil
	}

	// Restore playback to the specified cue using its fade-in time
	if err := s.playbackController.GoToCueNumber(ctx, snapshot.CueListID, *snapshot.CueNumber, snapshot.FadeInTime); err != nil {
		return "", fmt.Errorf("failed to restore cue playback: %w", err)
	}

	return snapshot.CueListID, nil
}

// applyBulkFixturePositionSnapshot restores multiple fixture positions from a snapshot.
// Returns empty string as there's no single entity ID (it's a bulk operation).
func (s *Service) applyBulkFixturePositionSnapshot(ctx context.Context, snapshotJSON string) (string, error) {
	if snapshotJSON == "" {
		return "", nil
	}

	var snapshot BulkFixturePositionSnapshot
	if err := json.Unmarshal([]byte(snapshotJSON), &snapshot); err != nil {
		return "", fmt.Errorf("failed to unmarshal bulk fixture position snapshot: %w", err)
	}

	// Restore each fixture's position
	// We collect errors and report the first one to ensure the caller knows the restore was incomplete
	var firstError error
	for _, pos := range snapshot.Positions {
		fixture, err := s.fixtureRepo.FindByID(ctx, pos.FixtureID)
		if err != nil {
			log.Printf("Warning: failed to find fixture %s during position restore: %v", pos.FixtureID, err)
			if firstError == nil {
				firstError = fmt.Errorf("failed to find fixture %s: %w", pos.FixtureID, err)
			}
			continue
		}
		if fixture == nil {
			log.Printf("Warning: fixture %s not found during position restore", pos.FixtureID)
			if firstError == nil {
				firstError = fmt.Errorf("fixture %s not found", pos.FixtureID)
			}
			continue
		}

		fixture.LayoutX = pos.LayoutX
		fixture.LayoutY = pos.LayoutY
		fixture.LayoutRotation = pos.LayoutRotation

		if err := s.fixtureRepo.Update(ctx, fixture); err != nil {
			log.Printf("Warning: failed to update fixture %s position during restore: %v", pos.FixtureID, err)
			if firstError == nil {
				firstError = fmt.Errorf("failed to update fixture %s: %w", pos.FixtureID, err)
			}
		}
	}

	if firstError != nil {
		return "", firstError
	}
	return "", nil
}

// applyBulkLookUpdateSnapshot restores multiple looks from a snapshot.
// Returns comma-separated list of restored look IDs.
func (s *Service) applyBulkLookUpdateSnapshot(ctx context.Context, snapshotJSON string) (string, error) {
	if snapshotJSON == "" {
		return "", nil
	}

	var snapshot BulkLookUpdateSnapshot
	if err := json.Unmarshal([]byte(snapshotJSON), &snapshot); err != nil {
		return "", fmt.Errorf("failed to unmarshal bulk look update snapshot: %w", err)
	}

	// Restore each look's state
	var restoredIDs []string
	for _, lookSnapshot := range snapshot.Looks {
		if lookSnapshot.Look == nil {
			continue
		}

		// Re-marshal individual look snapshot to use existing applyLookSnapshot
		lookJSON, err := json.Marshal(lookSnapshot)
		if err != nil {
			return "", fmt.Errorf("failed to marshal look snapshot for %s: %w", lookSnapshot.Look.ID, err)
		}

		entityID, err := s.applyLookSnapshot(ctx, string(lookJSON), OperationTypeUpdate, lookSnapshot.Look.ID)
		if err != nil {
			return "", fmt.Errorf("failed to restore look %s: %w", lookSnapshot.Look.ID, err)
		}
		restoredIDs = append(restoredIDs, entityID)
	}

	// Return comma-separated list of restored IDs
	result := ""
	for i, id := range restoredIDs {
		if i > 0 {
			result += ","
		}
		result += id
	}
	return result, nil
}

// CaptureBulkLookState captures the complete state of multiple looks for atomic undo.
// Used by copyFixturesToLooks to enable single-undo for multi-look updates.
func (s *Service) CaptureBulkLookState(ctx context.Context, lookIDs []string) (*BulkLookUpdateSnapshot, error) {
	var looks []LookSnapshot

	for _, lookID := range lookIDs {
		snapshot, err := s.CaptureLookState(ctx, lookID)
		if err != nil {
			return nil, fmt.Errorf("failed to capture look %s: %w", lookID, err)
		}
		if snapshot != nil {
			looks = append(looks, *snapshot)
		}
	}

	return &BulkLookUpdateSnapshot{Looks: looks}, nil
}
