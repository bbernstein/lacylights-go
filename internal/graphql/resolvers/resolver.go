// Package resolvers contains GraphQL resolver implementations.
package resolvers

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"time"

	"github.com/bbernstein/lacylights-go/internal/auth"
	"github.com/bbernstein/lacylights-go/internal/database/models"
	"github.com/bbernstein/lacylights-go/internal/database/repositories"
	"github.com/bbernstein/lacylights-go/internal/graphql/generated"
	"github.com/bbernstein/lacylights-go/internal/services/dmx"
	"github.com/bbernstein/lacylights-go/internal/services/export"
	importservice "github.com/bbernstein/lacylights-go/internal/services/import"
	"github.com/bbernstein/lacylights-go/internal/services/modulator"
	"github.com/bbernstein/lacylights-go/internal/services/ofl"
	"github.com/bbernstein/lacylights-go/internal/services/playback"
	"github.com/bbernstein/lacylights-go/internal/services/preview"
	"github.com/bbernstein/lacylights-go/internal/services/pubsub"
	"github.com/bbernstein/lacylights-go/internal/services/undo"
	"github.com/bbernstein/lacylights-go/internal/services/version"
	"github.com/bbernstein/lacylights-go/internal/services/wifi"
	"gorm.io/gorm"
)

// Resolver is the root resolver for the GraphQL schema.
// It holds dependencies that are shared across all resolvers.
type Resolver struct {
	db *gorm.DB

	// Repositories
	ProjectRepo    *repositories.ProjectRepository
	SettingRepo    *repositories.SettingRepository
	FixtureRepo    *repositories.FixtureRepository
	LookRepo       *repositories.LookRepository
	CueListRepo    *repositories.CueListRepository
	CueRepo        *repositories.CueRepository
	LookBoardRepo  *repositories.LookBoardRepository
	EffectRepo     *repositories.EffectRepository
	OperationRepo  *repositories.OperationRepository

	// Services
	DMXService      *dmx.Service
	FadeEngine      *modulator.Engine
	PlaybackService *playback.Service
	ExportService   *export.Service
	ImportService   *importservice.Service
	OFLService      *ofl.Service
	OFLManager      *ofl.Manager
	PreviewService  *preview.Service
	VersionService  *version.Service
	WiFiService     *wifi.Service
	UndoService     *undo.Service
	PubSub          *pubsub.PubSub

	// Auth service (optional - nil if auth is disabled)
	AuthService *auth.Service
}

// NewResolver creates a new Resolver instance with all dependencies.
func NewResolver(db *gorm.DB, dmxService *dmx.Service, fadeEngine *modulator.Engine, playbackService *playback.Service, oflCachePath string) *Resolver {
	projectRepo := repositories.NewProjectRepository(db)
	fixtureRepo := repositories.NewFixtureRepository(db)
	lookRepo := repositories.NewLookRepository(db)
	cueListRepo := repositories.NewCueListRepository(db)
	cueRepo := repositories.NewCueRepository(db)
	lookBoardRepo := repositories.NewLookBoardRepository(db)
	effectRepo := repositories.NewEffectRepository(db)
	operationRepo := repositories.NewOperationRepository(db)

	ps := pubsub.New()

	// Create PubSub first so it can be passed to OFLManager
	oflManager := ofl.NewManager(db, fixtureRepo, ps, oflCachePath)

	// Create undo service
	undoService := undo.NewService(
		operationRepo,
		lookRepo,
		fixtureRepo,
		cueRepo,
		cueListRepo,
		lookBoardRepo,
		effectRepo,
		projectRepo,
		ps,
	)

	r := &Resolver{
		db:              db,
		ProjectRepo:     projectRepo,
		SettingRepo:     repositories.NewSettingRepository(db),
		FixtureRepo:     fixtureRepo,
		LookRepo:        lookRepo,
		CueListRepo:     cueListRepo,
		CueRepo:         cueRepo,
		LookBoardRepo:   lookBoardRepo,
		EffectRepo:      effectRepo,
		OperationRepo:   operationRepo,
		DMXService:      dmxService,
		FadeEngine:      fadeEngine,
		PlaybackService: playbackService,
		ExportService:   export.NewServiceWithLookBoards(projectRepo, fixtureRepo, lookRepo, cueListRepo, cueRepo, lookBoardRepo),
		ImportService:   importservice.NewServiceWithLookBoards(projectRepo, fixtureRepo, lookRepo, cueListRepo, cueRepo, lookBoardRepo),
		OFLService:      ofl.NewService(db, fixtureRepo),
		OFLManager:      oflManager,
		PreviewService:  preview.NewService(fixtureRepo, lookRepo, dmxService),
		VersionService:  version.NewService(),
		WiFiService:     wifi.NewService(),
		UndoService:     undoService,
		PubSub:          ps,
	}

	// Wire up playback controller for undo/redo of cue playback operations
	r.UndoService.SetPlaybackController(playbackService)

	// Wire up PubSub publishing from services
	r.wirePubSub()

	return r
}

// SetAuthService sets the auth service on the resolver.
// This is called during server initialization if auth is enabled.
func (r *Resolver) SetAuthService(authService *auth.Service) {
	r.AuthService = authService
}

// wirePubSub connects services to the PubSub system for real-time updates.
func (r *Resolver) wirePubSub() {
	// Wire up PlaybackService to publish cue list playback updates
	r.PlaybackService.SetUpdateCallback(func(status *playback.CueListPlaybackStatus) {
		// Convert playback status to generated type
		fadeProgress := status.FadeProgress
		gqlStatus := &generated.CueListPlaybackStatus{
			CueListID:       status.CueListID,
			CurrentCueIndex: status.CurrentCueIndex,
			IsPlaying:       status.IsPlaying,
			IsPaused:        status.IsPaused,
			IsFading:        status.IsFading,
			FadeProgress:    &fadeProgress,
			LastUpdated:     status.LastUpdated,
		}

		// Convert current cue if present (to models.Cue as expected by generated type)
		if status.CurrentCue != nil {
			gqlStatus.CurrentCue = &models.Cue{
				ID:          status.CurrentCue.ID,
				Name:        status.CurrentCue.Name,
				CueNumber:   status.CurrentCue.CueNumber,
				FadeInTime:  status.CurrentCue.FadeInTime,
				FadeOutTime: status.CurrentCue.FadeOutTime,
				FollowTime:  status.CurrentCue.FollowTime,
			}
		}

		r.PubSub.Publish(pubsub.TopicCueListPlayback, status.CueListID, gqlStatus)
	})

	// Wire up PlaybackService to publish global playback status updates
	r.PlaybackService.SetGlobalUpdateCallback(func(status *playback.GlobalPlaybackStatus) {
		// Convert playback status to generated type
		fadeProgress := status.FadeProgress
		gqlStatus := &generated.GlobalPlaybackStatus{
			IsPlaying:       status.IsPlaying,
			IsPaused:        status.IsPaused,
			IsFading:        status.IsFading,
			CueListID:       status.CueListID,
			CueListName:     status.CueListName,
			CurrentCueIndex: status.CurrentCueIndex,
			CueCount:        status.CueCount,
			CurrentCueName:  status.CurrentCueName,
			FadeProgress:    &fadeProgress,
			LastUpdated:     status.LastUpdated,
		}

		// Global playback status uses empty filter since it's not filtered by cue list
		r.PubSub.Publish(pubsub.TopicGlobalPlaybackStatus, "", gqlStatus)
	})

	// Wire up PlaybackService with PreviewService to cancel preview sessions during cue execution
	r.PlaybackService.SetPreviewService(r.PreviewService)

	// Wire up PreviewService to publish preview session updates
	r.PreviewService.SetSessionUpdateCallback(func(session *preview.Session, dmxOutput []preview.DMXOutput) {
		// Convert to models.PreviewSession for the subscription
		var userID string
		if session.UserID != nil {
			userID = *session.UserID
		}

		modelSession := &models.PreviewSession{
			ID:        session.ID,
			ProjectID: session.ProjectID,
			UserID:    userID,
			IsActive:  session.IsActive,
			CreatedAt: session.CreatedAt,
		}

		r.PubSub.Publish(pubsub.TopicPreviewSession, session.ProjectID, modelSession)

		// Also publish DMX output changes for each universe affected
		for _, output := range dmxOutput {
			universeOutput := &generated.UniverseOutput{
				Universe: output.Universe,
				Channels: output.Channels,
			}
			r.PubSub.Publish(pubsub.TopicDMXOutput, fmt.Sprintf("%d", output.Universe), universeOutput)
		}
	})

	// Wire up WiFi service callbacks
	r.WiFiService.SetModeCallback(func(mode wifi.Mode) {
		r.PubSub.Publish(pubsub.TopicWiFiModeChanged, "", generated.WiFiMode(mode))
	})

	r.WiFiService.SetStatusCallback(func(status *wifi.Status) {
		// Convert to generated type and publish
		gqlStatus := convertWiFiStatus(status)
		r.PubSub.Publish(pubsub.TopicWiFiStatus, "", gqlStatus)
	})
}

// convertWiFiStatus converts a wifi.Status to generated.WiFiStatus.
func convertWiFiStatus(status *wifi.Status) *generated.WiFiStatus {
	if status == nil {
		return nil
	}

	gqlStatus := &generated.WiFiStatus{
		Available: status.Available,
		Enabled:   status.Enabled,
		Connected: status.Connected,
		Mode:      generated.WiFiMode(status.Mode),
	}

	if status.SSID != nil {
		gqlStatus.Ssid = status.SSID
	}
	if status.SignalStrength != nil {
		gqlStatus.SignalStrength = status.SignalStrength
	}
	if status.IPAddress != nil {
		gqlStatus.IPAddress = status.IPAddress
	}
	if status.MACAddress != nil {
		gqlStatus.MacAddress = status.MACAddress
	}
	if status.Frequency != nil {
		gqlStatus.Frequency = status.Frequency
	}

	// Convert AP config if present
	if status.APConfig != nil {
		gqlStatus.ApConfig = &generated.APConfig{
			Ssid:           status.APConfig.SSID,
			IPAddress:      status.APConfig.IPAddress,
			Channel:        status.APConfig.Channel,
			ClientCount:    status.APConfig.ClientCount,
			TimeoutMinutes: status.APConfig.TimeoutMinutes,
		}
		if status.APConfig.MinutesRemaining != nil {
			gqlStatus.ApConfig.MinutesRemaining = status.APConfig.MinutesRemaining
		}
	}

	// Convert connected clients if present
	if status.ConnectedClients != nil {
		for _, client := range status.ConnectedClients {
			gqlClient := &generated.APClient{
				MacAddress:  client.MACAddress,
				ConnectedAt: client.ConnectedAt.Format("2006-01-02T15:04:05Z07:00"),
			}
			if client.IPAddress != nil {
				gqlClient.IPAddress = client.IPAddress
			}
			if client.Hostname != nil {
				gqlClient.Hostname = client.Hostname
			}
			gqlStatus.ConnectedClients = append(gqlStatus.ConnectedClients, gqlClient)
		}
	}

	return gqlStatus
}

// publishCueListDataChanged publishes a cue list data change event to subscribers.
func (r *Resolver) publishCueListDataChanged(cueListID string, changeType generated.CueListDataChangeType, affectedCueIds []string, affectedLookID *string, newLookName *string) {
	payload := &generated.CueListDataChangedPayload{
		CueListID:      cueListID,
		ChangeType:     changeType,
		AffectedCueIds: affectedCueIds,
		AffectedLookID: affectedLookID,
		NewLookName:    newLookName,
		Timestamp:      time.Now().UTC().Format(time.RFC3339),
	}
	r.PubSub.Publish(pubsub.TopicCueListDataChanged, cueListID, payload)
}

// notifyLookNameChange finds all cue lists using a look and publishes LOOK_NAME_CHANGED events.
// This is called when a look is renamed to keep cue list displays in sync.
func (r *Resolver) notifyLookNameChange(ctx context.Context, lookID, newName string) {
	cueListIDs, err := r.CueRepo.FindCueListIDsByLookID(ctx, lookID)
	if err != nil {
		log.Printf("Warning: failed to find cue lists for look notification: %v", err)
		return
	}
	for _, cueListID := range cueListIDs {
		lookIDCopy := lookID
		newNameCopy := newName
		r.publishCueListDataChanged(cueListID, generated.CueListDataChangeTypeLookNameChanged, nil, &lookIDCopy, &newNameCopy)
	}
}

// publishOperationHistoryChanged publishes an undo/redo status update to subscribers.
// This is called after any undo, redo, or new operation is recorded.
func (r *Resolver) publishOperationHistoryChanged(ctx context.Context, projectID string) {
	status, err := r.OperationRepo.GetUndoRedoStatus(ctx, projectID)
	if err != nil {
		log.Printf("Warning: failed to get undo/redo status for publish: %v", err)
		return
	}

	gqlStatus := &generated.UndoRedoStatus{
		ProjectID:       status.ProjectID,
		CanUndo:         status.CanUndo,
		CanRedo:         status.CanRedo,
		CurrentSequence: status.CurrentSequence,
		TotalOperations: int(status.TotalOperations),
	}

	if status.UndoDescription != "" {
		gqlStatus.UndoDescription = &status.UndoDescription
	}
	if status.RedoDescription != "" {
		gqlStatus.RedoDescription = &status.RedoDescription
	}

	r.PubSub.Publish(pubsub.TopicOperationHistoryChanged, projectID, gqlStatus)
}

// publishLookDataChanged publishes a look data change event to subscribers.
// This is called after undo/redo operations that affect looks.
func (r *Resolver) publishLookDataChanged(lookID, projectID string, changeType generated.EntityDataChangeType) {
	payload := &generated.LookDataChangedPayload{
		LookID:     lookID,
		ProjectID:  projectID,
		ChangeType: changeType,
		Timestamp:  time.Now().UTC().Format(time.RFC3339),
	}
	r.PubSub.Publish(pubsub.TopicLookDataChanged, projectID, payload)
}

// publishLookBoardDataChanged publishes a look board data change event to subscribers.
// This is called after undo/redo operations that affect look boards or buttons.
func (r *Resolver) publishLookBoardDataChanged(lookBoardID, projectID string, changeType generated.EntityDataChangeType, affectedButtonIds []string) {
	payload := &generated.LookBoardDataChangedPayload{
		LookBoardID:       lookBoardID,
		ProjectID:         projectID,
		ChangeType:        changeType,
		AffectedButtonIds: affectedButtonIds,
		Timestamp:         time.Now().UTC().Format(time.RFC3339),
	}
	r.PubSub.Publish(pubsub.TopicLookBoardDataChanged, projectID, payload)
}

// publishFixtureDataChanged publishes a fixture data change event to subscribers.
// This is called after undo/redo operations that affect fixture positions.
func (r *Resolver) publishFixtureDataChanged(fixtureIDs []string, projectID string, changeType generated.EntityDataChangeType) {
	payload := &generated.FixtureDataChangedPayload{
		FixtureIds: fixtureIDs,
		ProjectID:  projectID,
		ChangeType: changeType,
		Timestamp:  time.Now().UTC().Format(time.RFC3339),
	}
	r.PubSub.Publish(pubsub.TopicFixtureDataChanged, projectID, payload)
}

// publishEntityChangeFromUndo publishes entity-specific change events after undo/redo operations.
// This routes based on the operation's entity type to publish appropriate real-time updates.
// isUndo should be true for undo operations, false for redo operations.
func (r *Resolver) publishEntityChangeFromUndo(ctx context.Context, projectID string, op *models.Operation, entityID string, isUndo bool) {
	if op == nil {
		return
	}

	// Determine the change type based on operation type and whether this is undo or redo
	// Undo CREATE → entity was deleted; Redo CREATE → entity was created
	// Undo DELETE → entity was restored/created; Redo DELETE → entity was deleted
	var changeType generated.EntityDataChangeType
	switch op.OperationType {
	case "CREATE":
		if isUndo {
			changeType = generated.EntityDataChangeTypeDeleted
		} else {
			changeType = generated.EntityDataChangeTypeCreated
		}
	case "DELETE":
		if isUndo {
			changeType = generated.EntityDataChangeTypeCreated
		} else {
			changeType = generated.EntityDataChangeTypeDeleted
		}
	case "UPDATE":
		changeType = generated.EntityDataChangeTypeUpdated
	case "BULK":
		// For bulk operations, we publish as UPDATE since it could involve multiple changes
		changeType = generated.EntityDataChangeTypeUpdated
	default:
		changeType = generated.EntityDataChangeTypeUpdated
	}

	// Route based on entity type
	switch op.EntityType {
	case "Look":
		lookID := entityID
		if lookID == "" {
			lookID = op.EntityID
		}
		r.publishLookDataChanged(lookID, projectID, changeType)

		// If the look is currently active, refresh DMX output
		r.refreshActiveLookDMX(ctx, lookID)

	case "LookBoard":
		lookBoardID := entityID
		if lookBoardID == "" {
			lookBoardID = op.EntityID
		}
		r.publishLookBoardDataChanged(lookBoardID, projectID, changeType, nil)

	case "LookBoardButton":
		// For button changes, we need to find the parent look board
		buttonID := entityID
		if buttonID == "" {
			buttonID = op.EntityID
		}
		// Try to get the look board ID from the button
		lookBoardID := r.getLookBoardIDForButton(ctx, buttonID)
		if lookBoardID != "" {
			r.publishLookBoardDataChanged(lookBoardID, projectID, changeType, []string{buttonID})
		}

	case "Cue":
		// For cue changes, publish to the cue list data changed topic
		cueID := entityID
		if cueID == "" {
			cueID = op.EntityID
		}
		cueListID := r.getCueListIDForCue(ctx, cueID)
		if cueListID != "" {
			var cueChangeType generated.CueListDataChangeType
			switch changeType {
			case generated.EntityDataChangeTypeCreated:
				cueChangeType = generated.CueListDataChangeTypeCueAdded
			case generated.EntityDataChangeTypeDeleted:
				cueChangeType = generated.CueListDataChangeTypeCueRemoved
			default:
				cueChangeType = generated.CueListDataChangeTypeCueUpdated
			}
			r.publishCueListDataChanged(cueListID, cueChangeType, []string{cueID}, nil, nil)
		}

	case "CueList":
		cueListID := entityID
		if cueListID == "" {
			cueListID = op.EntityID
		}
		r.publishCueListDataChanged(cueListID, generated.CueListDataChangeTypeCueListMetadataChanged, nil, nil, nil)

	case "FixtureInstance":
		// Fixture changes may affect looks that use them
		// Publish a look data change for any affected looks
		fixtureID := entityID
		if fixtureID == "" {
			fixtureID = op.EntityID
		}
		r.notifyFixtureAffectedLooks(ctx, fixtureID, projectID)

	case "BulkFixturePosition":
		// Bulk fixture position changes (from 2D Layout view)
		// Extract fixture IDs from the RelatedIDs field (JSON array)
		fixtureIDs := extractFixtureIDsFromRelatedIDs(op.RelatedIDs)
		if len(fixtureIDs) > 0 {
			r.publishFixtureDataChanged(fixtureIDs, projectID, changeType)
		}
	}
}

// refreshActiveLookDMX checks if a look is currently active and logs a message.
// Note: We don't automatically refresh DMX output to avoid unexpected behavior.
// Users can reactivate the look manually if they need the updated values on DMX.
func (r *Resolver) refreshActiveLookDMX(_ context.Context, lookID string) {
	activeLookID := r.DMXService.GetActiveLookID()
	if activeLookID != nil && *activeLookID == lookID {
		log.Printf("Info: Look %s was modified via undo/redo while active. Reactivate the look to see updated DMX values.", lookID)
	}
}

// getLookBoardIDForButton retrieves the look board ID for a button.
func (r *Resolver) getLookBoardIDForButton(ctx context.Context, buttonID string) string {
	button, err := r.LookBoardRepo.FindButtonByID(ctx, buttonID)
	if err != nil || button == nil {
		return ""
	}
	return button.LookBoardID
}

// getCueListIDForCue retrieves the cue list ID for a cue.
func (r *Resolver) getCueListIDForCue(ctx context.Context, cueID string) string {
	cue, err := r.CueRepo.FindByID(ctx, cueID)
	if err != nil || cue == nil {
		return ""
	}
	return cue.CueListID
}

// notifyFixtureAffectedLooks notifies about fixtures that were changed.
// Note: We don't currently track which looks use which fixtures efficiently,
// so we just log the change. Looks that use this fixture will need to be
// refreshed manually if needed.
func (r *Resolver) notifyFixtureAffectedLooks(_ context.Context, fixtureID, projectID string) {
	log.Printf("Info: Fixture %s in project %s was modified via undo/redo. Looks using this fixture may need refresh.", fixtureID, projectID)
}

// captureCuePlaybackState captures the current playback state for undo/redo.
func (r *Resolver) captureCuePlaybackState(cueListID, projectID, cueListName string) *undo.CuePlaybackSnapshot {
	status := r.PlaybackService.GetFormattedStatus(cueListID)

	snapshot := &undo.CuePlaybackSnapshot{
		CueListID:   cueListID,
		ProjectID:   projectID,
		IsPlaying:   status.IsPlaying,
		CueListName: cueListName,
	}

	if status.CurrentCue != nil {
		snapshot.CueID = &status.CurrentCue.ID
		snapshot.CueNumber = &status.CurrentCue.CueNumber
		snapshot.CueName = &status.CurrentCue.Name
		snapshot.FadeInTime = &status.CurrentCue.FadeInTime
	}

	return snapshot
}

// extractFixtureIDsFromRelatedIDs extracts fixture IDs from the RelatedIDs JSON field.
// RelatedIDs is a JSON array of fixture IDs stored in bulk operations.
func extractFixtureIDsFromRelatedIDs(relatedIDs *string) []string {
	if relatedIDs == nil || *relatedIDs == "" {
		return nil
	}
	var ids []string
	if err := json.Unmarshal([]byte(*relatedIDs), &ids); err != nil {
		log.Printf("Warning: failed to parse RelatedIDs: %v", err)
		return nil
	}
	return ids
}
