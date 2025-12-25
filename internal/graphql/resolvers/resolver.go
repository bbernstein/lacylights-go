// Package resolvers contains GraphQL resolver implementations.
package resolvers

import (
	"fmt"

	"github.com/bbernstein/lacylights-go/internal/database/models"
	"github.com/bbernstein/lacylights-go/internal/database/repositories"
	"github.com/bbernstein/lacylights-go/internal/graphql/generated"
	"github.com/bbernstein/lacylights-go/internal/services/dmx"
	"github.com/bbernstein/lacylights-go/internal/services/export"
	"github.com/bbernstein/lacylights-go/internal/services/fade"
	importservice "github.com/bbernstein/lacylights-go/internal/services/import"
	"github.com/bbernstein/lacylights-go/internal/services/ofl"
	"github.com/bbernstein/lacylights-go/internal/services/playback"
	"github.com/bbernstein/lacylights-go/internal/services/preview"
	"github.com/bbernstein/lacylights-go/internal/services/pubsub"
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
	SceneRepo      *repositories.SceneRepository
	CueListRepo    *repositories.CueListRepository
	CueRepo        *repositories.CueRepository
	SceneBoardRepo *repositories.SceneBoardRepository

	// Services
	DMXService      *dmx.Service
	FadeEngine      *fade.Engine
	PlaybackService *playback.Service
	ExportService   *export.Service
	ImportService   *importservice.Service
	OFLService      *ofl.Service
	OFLManager      *ofl.Manager
	PreviewService  *preview.Service
	VersionService  *version.Service
	WiFiService     *wifi.Service
	PubSub          *pubsub.PubSub
}

// NewResolver creates a new Resolver instance with all dependencies.
func NewResolver(db *gorm.DB, dmxService *dmx.Service, fadeEngine *fade.Engine, playbackService *playback.Service, oflCachePath string) *Resolver {
	projectRepo := repositories.NewProjectRepository(db)
	fixtureRepo := repositories.NewFixtureRepository(db)
	sceneRepo := repositories.NewSceneRepository(db)
	cueListRepo := repositories.NewCueListRepository(db)
	cueRepo := repositories.NewCueRepository(db)
	sceneBoardRepo := repositories.NewSceneBoardRepository(db)

	ps := pubsub.New()

	// Create PubSub first so it can be passed to OFLManager
	oflManager := ofl.NewManager(db, fixtureRepo, ps, oflCachePath)

	r := &Resolver{
		db:              db,
		ProjectRepo:     projectRepo,
		SettingRepo:     repositories.NewSettingRepository(db),
		FixtureRepo:     fixtureRepo,
		SceneRepo:       sceneRepo,
		CueListRepo:     cueListRepo,
		CueRepo:         cueRepo,
		SceneBoardRepo:  sceneBoardRepo,
		DMXService:      dmxService,
		FadeEngine:      fadeEngine,
		PlaybackService: playbackService,
		ExportService:   export.NewServiceWithSceneBoards(projectRepo, fixtureRepo, sceneRepo, cueListRepo, cueRepo, sceneBoardRepo),
		ImportService:   importservice.NewServiceWithSceneBoards(projectRepo, fixtureRepo, sceneRepo, cueListRepo, cueRepo, sceneBoardRepo),
		OFLService:      ofl.NewService(db, fixtureRepo),
		OFLManager:      oflManager,
		PreviewService:  preview.NewService(fixtureRepo, sceneRepo, dmxService),
		VersionService:  version.NewService(),
		WiFiService:     wifi.NewService(),
		PubSub:          ps,
	}

	// Wire up PubSub publishing from services
	r.wirePubSub()

	return r
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
