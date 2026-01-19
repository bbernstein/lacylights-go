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
	EntityTypeLook            EntityType = "Look"
	EntityTypeFixtureInstance EntityType = "FixtureInstance"
	EntityTypeCue             EntityType = "Cue"
	EntityTypeCueList         EntityType = "CueList"
	EntityTypeLookBoard       EntityType = "LookBoard"
	EntityTypeLookBoardButton EntityType = "LookBoardButton"
	EntityTypeEffect          EntityType = "Effect"
	EntityTypeProject         EntityType = "Project"
	EntityTypeCuePlayback     EntityType = "CuePlayback"
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
func (s *Service) Undo(ctx context.Context, projectID string) (*UndoRedoResult, error) {
	op, err := s.opRepo.Undo(ctx, projectID)
	if err != nil {
		return nil, fmt.Errorf("failed to get operation for undo: %w", err)
	}

	if op == nil {
		return &UndoRedoResult{
			Success: false,
			Message: "Nothing to undo",
		}, nil
	}

	// Apply the previous state
	entityID, err := s.applySnapshot(ctx, EntityType(op.EntityType), op.PreviousState, OperationType(op.OperationType), op.EntityID)
	if err != nil {
		// Rollback the pointer change
		// Note: In a more robust implementation, we'd want to handle this atomically
		log.Printf("Warning: failed to apply undo, attempting rollback: %v", err)
		return &UndoRedoResult{
			Success: false,
			Message: fmt.Sprintf("Failed to apply undo: %v", err),
		}, err
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
func (s *Service) Redo(ctx context.Context, projectID string) (*UndoRedoResult, error) {
	op, err := s.opRepo.Redo(ctx, projectID)
	if err != nil {
		return nil, fmt.Errorf("failed to get operation for redo: %w", err)
	}

	if op == nil {
		return &UndoRedoResult{
			Success: false,
			Message: "Nothing to redo",
		}, nil
	}

	// Apply the new state
	entityID, err := s.applySnapshot(ctx, EntityType(op.EntityType), op.NewState, OperationType(op.OperationType), op.EntityID)
	if err != nil {
		log.Printf("Warning: failed to apply redo: %v", err)
		return &UndoRedoResult{
			Success: false,
			Message: fmt.Sprintf("Failed to apply redo: %v", err),
		}, err
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
	case EntityTypeCuePlayback:
		return s.applyCuePlaybackSnapshot(ctx, snapshotJSON)
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
		// Fixture exists - update it
		if err := s.fixtureRepo.Update(ctx, snapshot.Fixture); err != nil {
			return "", fmt.Errorf("failed to update fixture: %w", err)
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
		// Also update all cues (for reorder operations)
		for i := range snapshot.Cues {
			cue := &snapshot.Cues[i]
			existingCue, _ := s.cueRepo.FindByID(ctx, cue.ID)
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
		// Recreate buttons
		for i := range snapshot.Buttons {
			if err := s.lookBoardRepo.CreateButton(ctx, &snapshot.Buttons[i]); err != nil {
				return "", fmt.Errorf("failed to recreate look board button: %w", err)
			}
		}
	} else {
		// Look board exists - update it
		if err := s.lookBoardRepo.Update(ctx, snapshot.LookBoard); err != nil {
			return "", fmt.Errorf("failed to update look board: %w", err)
		}
	}

	return snapshot.LookBoard.ID, nil
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
