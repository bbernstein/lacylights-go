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
	"gorm.io/gorm"
)

// Resolver is the root resolver for the GraphQL schema.
// It holds dependencies that are shared across all resolvers.
type Resolver struct {
	db *gorm.DB

	// Repositories
	ProjectRepo *repositories.ProjectRepository
	SettingRepo *repositories.SettingRepository
	FixtureRepo *repositories.FixtureRepository
	SceneRepo   *repositories.SceneRepository
	CueListRepo *repositories.CueListRepository
	CueRepo     *repositories.CueRepository

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
	PubSub          *pubsub.PubSub
}

// NewResolver creates a new Resolver instance with all dependencies.
func NewResolver(db *gorm.DB, dmxService *dmx.Service, fadeEngine *fade.Engine, playbackService *playback.Service) *Resolver {
	projectRepo := repositories.NewProjectRepository(db)
	fixtureRepo := repositories.NewFixtureRepository(db)
	sceneRepo := repositories.NewSceneRepository(db)
	cueListRepo := repositories.NewCueListRepository(db)
	cueRepo := repositories.NewCueRepository(db)

	ps := pubsub.New()

	// Create PubSub first so it can be passed to OFLManager
	oflManager := ofl.NewManager(db, fixtureRepo, ps, "./.ofl-cache")

	r := &Resolver{
		db:              db,
		ProjectRepo:     projectRepo,
		SettingRepo:     repositories.NewSettingRepository(db),
		FixtureRepo:     fixtureRepo,
		SceneRepo:       sceneRepo,
		CueListRepo:     cueListRepo,
		CueRepo:         cueRepo,
		DMXService:      dmxService,
		FadeEngine:      fadeEngine,
		PlaybackService: playbackService,
		ExportService:   export.NewService(projectRepo, fixtureRepo, sceneRepo, cueListRepo, cueRepo),
		ImportService:   importservice.NewService(projectRepo, fixtureRepo, sceneRepo, cueListRepo, cueRepo),
		OFLService:      ofl.NewService(db, fixtureRepo),
		OFLManager:      oflManager,
		PreviewService:  preview.NewService(fixtureRepo, sceneRepo, dmxService),
		VersionService:  version.NewService(),
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
}
