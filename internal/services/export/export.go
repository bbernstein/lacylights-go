// Package export provides project export functionality.
package export

import (
	"context"
	"encoding/json"
	"log"

	"github.com/bbernstein/lacylights-go/internal/database/models"
	"github.com/bbernstein/lacylights-go/internal/database/repositories"
)

// ExportedProject represents a full project export.
// Matches the LacyLights Node.js export format.
type ExportedProject struct {
	Version            string                      `json:"version"`
	Metadata           *ExportMetadata             `json:"metadata,omitempty"`
	Project            *ExportProjectInfo          `json:"project,omitempty"`
	FixtureDefinitions []ExportedFixtureDefinition `json:"fixtureDefinitions"`
	FixtureInstances   []ExportedFixtureInstance   `json:"fixtureInstances"`
	Looks              []ExportedLook              `json:"looks"`
	Scenes             []ExportedLook              `json:"scenes,omitempty"` // Deprecated: backward compatibility, use Looks
	CueLists           []ExportedCueList           `json:"cueLists"`
	LookBoards         []ExportedLookBoard         `json:"lookBoards,omitempty"`
	SceneBoards        []ExportedLookBoard         `json:"sceneBoards,omitempty"` // Deprecated: backward compatibility, use LookBoards
}

// ExportMetadata contains export metadata.
type ExportMetadata struct {
	ExportedAt        string  `json:"exportedAt"`
	LacyLightsVersion string  `json:"lacyLightsVersion"`
	Description       *string `json:"description,omitempty"`
}

// ExportProjectInfo contains project information.
type ExportProjectInfo struct {
	OriginalID  string  `json:"originalId"`
	Name        string  `json:"name"`
	Description *string `json:"description,omitempty"`
	CreatedAt   string  `json:"createdAt,omitempty"`
	UpdatedAt   string  `json:"updatedAt,omitempty"`
}

// ExportedFixtureDefinition represents an exported fixture definition.
type ExportedFixtureDefinition struct {
	RefID        string                      `json:"refId"`
	Manufacturer string                      `json:"manufacturer"`
	Model        string                      `json:"model"`
	Type         string                      `json:"type"`
	IsBuiltIn    bool                        `json:"isBuiltIn"`
	Modes        []ExportedFixtureMode       `json:"modes,omitempty"`
	Channels     []ExportedChannelDefinition `json:"channels"`
}

// ExportedFixtureMode represents a fixture mode in export.
type ExportedFixtureMode struct {
	RefID        string               `json:"refId"`
	Name         string               `json:"name"`
	ShortName    *string              `json:"shortName,omitempty"`
	ChannelCount int                  `json:"channelCount"`
	ModeChannels []ExportedModeChannel `json:"modeChannels"`
}

// ExportedModeChannel represents a mode-specific channel mapping.
type ExportedModeChannel struct {
	ChannelRefID string `json:"channelRefId"`
	Offset       int    `json:"offset"`
}

// ExportedChannelDefinition represents an exported channel definition.
type ExportedChannelDefinition struct {
	RefID        string `json:"refId,omitempty"`
	Name         string `json:"name"`
	Type         string `json:"type"`
	Offset       int    `json:"offset"`
	MinValue     int    `json:"minValue"`
	MaxValue     int    `json:"maxValue"`
	DefaultValue int    `json:"defaultValue"`
	FadeBehavior string `json:"fadeBehavior,omitempty"` // FADE, SNAP, SNAP_END
	IsDiscrete   bool   `json:"isDiscrete,omitempty"`
}

// ExportedFixtureInstance represents an exported fixture instance.
type ExportedFixtureInstance struct {
	RefID            string                    `json:"refId"`
	OriginalID       string                    `json:"originalId,omitempty"`
	Name             string                    `json:"name"`
	Description      *string                   `json:"description,omitempty"`
	DefinitionRefID  string                    `json:"definitionRefId"`
	ModeName         *string                   `json:"modeName,omitempty"`
	ModeRefID        *string                   `json:"modeRefId,omitempty"`
	ChannelCount     *int                      `json:"channelCount,omitempty"`
	Universe         int                       `json:"universe"`
	StartChannel     int                       `json:"startChannel"`
	Tags             []string                  `json:"tags,omitempty"`
	ProjectOrder     *int                      `json:"projectOrder,omitempty"`
	LayoutX          *float64                  `json:"layoutX,omitempty"`
	LayoutY          *float64                  `json:"layoutY,omitempty"`
	LayoutRotation   *float64                  `json:"layoutRotation,omitempty"`
	InstanceChannels []ExportedInstanceChannel `json:"instanceChannels,omitempty"`
	CreatedAt        string                    `json:"createdAt,omitempty"`
	UpdatedAt        string                    `json:"updatedAt,omitempty"`
}

// ExportedInstanceChannel represents an instance-specific channel.
type ExportedInstanceChannel struct {
	Name         string `json:"name"`
	Type         string `json:"type"`
	Offset       int    `json:"offset"`
	MinValue     int    `json:"minValue"`
	MaxValue     int    `json:"maxValue"`
	DefaultValue int    `json:"defaultValue"`
	FadeBehavior string `json:"fadeBehavior,omitempty"` // FADE, SNAP, SNAP_END
	IsDiscrete   bool   `json:"isDiscrete,omitempty"`
}

// ExportedLook represents an exported look.
type ExportedLook struct {
	RefID         string                 `json:"refId"`
	OriginalID    string                 `json:"originalId,omitempty"`
	Name          string                 `json:"name"`
	Description   *string                `json:"description,omitempty"`
	FixtureValues []ExportedFixtureValue `json:"fixtureValues"`
	CreatedAt     string                 `json:"createdAt,omitempty"`
	UpdatedAt     string                 `json:"updatedAt,omitempty"`
}

// ExportedChannelValue represents a single channel value in sparse format.
type ExportedChannelValue struct {
	Offset int `json:"offset"`
	Value  int `json:"value"`
}

// ExportedFixtureValue represents exported fixture values in a look.
type ExportedFixtureValue struct {
	FixtureRefID  string                 `json:"fixtureRefId"`
	Channels      []ExportedChannelValue `json:"channels"`
	ChannelValues []int                  `json:"channelValues,omitempty"` // Read-only: used to import legacy dense array format, not populated on export
	LookOrder     *int                   `json:"lookOrder,omitempty"`
	SceneOrder    *int                   `json:"sceneOrder,omitempty"` // Deprecated: backward compatibility, use LookOrder
}

// ExportedCueList represents an exported cue list.
type ExportedCueList struct {
	RefID       string        `json:"refId"`
	OriginalID  string        `json:"originalId,omitempty"`
	Name        string        `json:"name"`
	Description *string       `json:"description,omitempty"`
	Loop        bool          `json:"loop"`
	Cues        []ExportedCue `json:"cues"`
	CreatedAt   string        `json:"createdAt,omitempty"`
	UpdatedAt   string        `json:"updatedAt,omitempty"`
}

// ExportedCue represents an exported cue.
type ExportedCue struct {
	OriginalID  string   `json:"originalId,omitempty"`
	Name        string   `json:"name"`
	CueNumber   float64  `json:"cueNumber"`
	LookRefID   string   `json:"lookRefId"`
	SceneRefID  string   `json:"sceneRefId,omitempty"` // Deprecated: backward compatibility, use LookRefID
	FadeInTime  float64  `json:"fadeInTime"`
	FadeOutTime float64  `json:"fadeOutTime"`
	FollowTime  *float64 `json:"followTime,omitempty"`
	EasingType  *string  `json:"easingType,omitempty"`
	Notes       *string  `json:"notes,omitempty"`
	CreatedAt   string   `json:"createdAt,omitempty"`
	UpdatedAt   string   `json:"updatedAt,omitempty"`
}

// ExportedLookBoard represents an exported look board.
type ExportedLookBoard struct {
	RefID           string                    `json:"refId"`
	OriginalID      string                    `json:"originalId,omitempty"`
	Name            string                    `json:"name"`
	Description     *string                   `json:"description,omitempty"`
	DefaultFadeTime float64                   `json:"defaultFadeTime"`
	GridSize        *int                      `json:"gridSize,omitempty"`
	CanvasWidth     int                       `json:"canvasWidth"`
	CanvasHeight    int                       `json:"canvasHeight"`
	Buttons         []ExportedLookBoardButton `json:"buttons,omitempty"`
	CreatedAt       string                    `json:"createdAt,omitempty"`
	UpdatedAt       string                    `json:"updatedAt,omitempty"`
}

// ExportedLookBoardButton represents an exported look board button.
type ExportedLookBoardButton struct {
	OriginalID  string  `json:"originalId,omitempty"`
	LookRefID   string  `json:"lookRefId"`
	SceneRefID  string  `json:"sceneRefId,omitempty"` // Deprecated: backward compatibility, use LookRefID
	LayoutX     int     `json:"layoutX"`
	LayoutY     int     `json:"layoutY"`
	Width       *int    `json:"width,omitempty"`
	Height      *int    `json:"height,omitempty"`
	Color       *string `json:"color,omitempty"`
	Label       *string `json:"label,omitempty"`
	CreatedAt   string  `json:"createdAt,omitempty"`
	UpdatedAt   string  `json:"updatedAt,omitempty"`
}

// ExportStats contains statistics about an export.
type ExportStats struct {
	FixtureDefinitionsCount int
	FixtureInstancesCount   int
	LooksCount              int
	ScenesCount             int // Deprecated: use LooksCount
	CueListsCount           int
	CuesCount               int
	LookBoardsCount         int
	SceneBoardsCount        int // Deprecated: use LookBoardsCount
}

// ExportOptions contains options for project export.
type ExportOptions struct {
	IncludeFixtures   bool // Include fixture definitions and instances
	IncludeLooks      bool // Include looks with fixture values
	IncludeScenes     bool // Deprecated: use IncludeLooks
	IncludeCueLists   bool // Include cue lists with cues
	IncludeLookBoards bool // Include look boards with buttons (defaults to true)
	IncludeSceneBoards bool // Deprecated: use IncludeLookBoards
}

// DefaultExportOptions returns the default export options (all true).
func DefaultExportOptions() ExportOptions {
	return ExportOptions{
		IncludeFixtures:   true,
		IncludeLooks:      true,
		IncludeCueLists:   true,
		IncludeLookBoards: true,
	}
}

// Service handles project export operations.
type Service struct {
	projectRepo   *repositories.ProjectRepository
	fixtureRepo   *repositories.FixtureRepository
	lookRepo      *repositories.LookRepository
	cueListRepo   *repositories.CueListRepository
	cueRepo       *repositories.CueRepository
	lookBoardRepo *repositories.LookBoardRepository
}

// NewService creates a new export service.
func NewService(
	projectRepo *repositories.ProjectRepository,
	fixtureRepo *repositories.FixtureRepository,
	lookRepo *repositories.LookRepository,
	cueListRepo *repositories.CueListRepository,
	cueRepo *repositories.CueRepository,
) *Service {
	return &Service{
		projectRepo: projectRepo,
		fixtureRepo: fixtureRepo,
		lookRepo:    lookRepo,
		cueListRepo: cueListRepo,
		cueRepo:     cueRepo,
	}
}

// NewServiceWithLookBoards creates a new export service with look board support.
func NewServiceWithLookBoards(
	projectRepo *repositories.ProjectRepository,
	fixtureRepo *repositories.FixtureRepository,
	lookRepo *repositories.LookRepository,
	cueListRepo *repositories.CueListRepository,
	cueRepo *repositories.CueRepository,
	lookBoardRepo *repositories.LookBoardRepository,
) *Service {
	return &Service{
		projectRepo:   projectRepo,
		fixtureRepo:   fixtureRepo,
		lookRepo:      lookRepo,
		cueListRepo:   cueListRepo,
		cueRepo:       cueRepo,
		lookBoardRepo: lookBoardRepo,
	}
}

// ExportProject exports a project to JSON.
// Deprecated: Use ExportProjectWithOptions for cleaner API.
func (s *Service) ExportProject(ctx context.Context, projectID string, includeFixtures, includeLooks, includeCueLists bool, includeLookBoards ...bool) (*ExportedProject, *ExportStats, error) {
	// Check if look boards should be included (defaults to true if not specified)
	exportLookBoards := true
	if len(includeLookBoards) > 0 {
		exportLookBoards = includeLookBoards[0]
	}

	opts := ExportOptions{
		IncludeFixtures:   includeFixtures,
		IncludeLooks:      includeLooks,
		IncludeCueLists:   includeCueLists,
		IncludeLookBoards: exportLookBoards,
	}
	return s.ExportProjectWithOptions(ctx, projectID, opts)
}

// ExportProjectWithOptions exports a project to JSON using an options struct.
func (s *Service) ExportProjectWithOptions(ctx context.Context, projectID string, opts ExportOptions) (*ExportedProject, *ExportStats, error) {
	// Get project
	project, err := s.projectRepo.FindByID(ctx, projectID)
	if err != nil {
		return nil, nil, err
	}
	if project == nil {
		return nil, nil, nil
	}

	exported := &ExportedProject{
		Version: "2.0",
		Project: &ExportProjectInfo{
			OriginalID:  project.ID,
			Name:        project.Name,
			Description: project.Description,
		},
	}

	stats := &ExportStats{}

	// Export fixture definitions and instances
	if opts.IncludeFixtures {
		// Get fixture instances for this project
		fixtures, err := s.fixtureRepo.FindByProjectID(ctx, projectID)
		if err != nil {
			return nil, nil, err
		}

		// Track which definitions we need
		definitionIDs := make(map[string]bool)
		for _, f := range fixtures {
			definitionIDs[f.DefinitionID] = true
		}

		// Track mode name -> mode ID mapping for each definition
		modeNameToIDMap := make(map[string]map[string]string) // definitionID -> (modeName -> modeID)

		// Export definitions
		for defID := range definitionIDs {
			def, err := s.fixtureRepo.FindDefinitionByID(ctx, defID)
			if err != nil {
				return nil, nil, err
			}
			if def == nil {
				continue
			}

			channels, err := s.fixtureRepo.GetDefinitionChannels(ctx, defID)
			if err != nil {
				return nil, nil, err
			}

			exportedDef := ExportedFixtureDefinition{
				RefID:        def.ID,
				Manufacturer: def.Manufacturer,
				Model:        def.Model,
				Type:         def.Type,
				IsBuiltIn:    def.IsBuiltIn,
			}

			for _, ch := range channels {
				exportedDef.Channels = append(exportedDef.Channels, ExportedChannelDefinition{
					RefID:        ch.ID,
					Name:         ch.Name,
					Type:         ch.Type,
					Offset:       ch.Offset,
					MinValue:     ch.MinValue,
					MaxValue:     ch.MaxValue,
					DefaultValue: ch.DefaultValue,
					FadeBehavior: ch.FadeBehavior,
					IsDiscrete:   ch.IsDiscrete,
				})
			}

			// Export modes for this definition
			modes, err := s.fixtureRepo.GetDefinitionModes(ctx, defID)
			if err != nil {
				return nil, nil, err
			}

			// Initialize mode name map for this definition
			modeNameToIDMap[defID] = make(map[string]string)

			for _, mode := range modes {
				exportedMode := ExportedFixtureMode{
					RefID:        mode.ID,
					Name:         mode.Name,
					ShortName:    mode.ShortName,
					ChannelCount: mode.ChannelCount,
				}

				// Track mode name -> ID mapping
				modeNameToIDMap[defID][mode.Name] = mode.ID

				// Get mode channels
				modeChannels, err := s.fixtureRepo.GetModeChannels(ctx, mode.ID)
				if err != nil {
					return nil, nil, err
				}

				for _, mc := range modeChannels {
					exportedMode.ModeChannels = append(exportedMode.ModeChannels, ExportedModeChannel{
						ChannelRefID: mc.ChannelID,
						Offset:       mc.Offset,
					})
				}

				exportedDef.Modes = append(exportedDef.Modes, exportedMode)
			}

			exported.FixtureDefinitions = append(exported.FixtureDefinitions, exportedDef)
			stats.FixtureDefinitionsCount++
		}

		// Export fixture instances
		for _, f := range fixtures {
			var tags []string
			if f.Tags != nil {
				if err := json.Unmarshal([]byte(*f.Tags), &tags); err != nil {
					log.Printf("Warning: failed to unmarshal tags for fixture %s: %v", f.ID, err)
					tags = []string{} // Continue with empty tags
				}
			}

			// Look up mode ref ID if mode name is set
			var modeRefID *string
			if f.ModeName != nil && *f.ModeName != "" {
				if modeMap, ok := modeNameToIDMap[f.DefinitionID]; ok {
					if modeID, ok := modeMap[*f.ModeName]; ok {
						modeRefID = &modeID
					}
					// Note: If mode not found in map, modeRefID remains nil but we continue
					// This is acceptable - the export will have modeName but not modeRefId
				}
			}

			exported.FixtureInstances = append(exported.FixtureInstances, ExportedFixtureInstance{
				RefID:           f.ID,
				OriginalID:      f.ID,
				Name:            f.Name,
				Description:     f.Description,
				DefinitionRefID: f.DefinitionID,
				ModeName:        f.ModeName,
				ModeRefID:       modeRefID,
				ChannelCount:    f.ChannelCount,
				Universe:        f.Universe,
				StartChannel:    f.StartChannel,
				Tags:            tags,
				ProjectOrder:    f.ProjectOrder,
				LayoutX:         f.LayoutX,
				LayoutY:         f.LayoutY,
				LayoutRotation:  f.LayoutRotation,
			})
			stats.FixtureInstancesCount++
		}
	}

	// Export looks
	if opts.IncludeLooks || opts.IncludeScenes {
		looks, err := s.lookRepo.FindByProjectID(ctx, projectID)
		if err != nil {
			return nil, nil, err
		}

		for _, look := range looks {
			fixtureValues, err := s.lookRepo.GetFixtureValues(ctx, look.ID)
			if err != nil {
				return nil, nil, err
			}

			exportedLook := ExportedLook{
				RefID:       look.ID,
				OriginalID:  look.ID,
				Name:        look.Name,
				Description: look.Description,
			}

			for _, fv := range fixtureValues {
				var channels []models.ChannelValue
				if err := json.Unmarshal([]byte(fv.Channels), &channels); err != nil {
					log.Printf("Warning: failed to unmarshal channels for fixture %s in look %s: %v", fv.FixtureID, look.ID, err)
					continue // Skip this fixture value
				}

				// Convert to exported format
				exportedChannels := make([]ExportedChannelValue, len(channels))
				for i, ch := range channels {
					exportedChannels[i] = ExportedChannelValue{
						Offset: ch.Offset,
						Value:  ch.Value,
					}
				}

				exportedLook.FixtureValues = append(exportedLook.FixtureValues, ExportedFixtureValue{
					FixtureRefID: fv.FixtureID,
					Channels:     exportedChannels,
					LookOrder:    fv.LookOrder,
				})
			}

			exported.Looks = append(exported.Looks, exportedLook)
			stats.LooksCount++
		}
	}

	// Export cue lists
	if opts.IncludeCueLists {
		cueLists, err := s.cueListRepo.FindByProjectID(ctx, projectID)
		if err != nil {
			return nil, nil, err
		}

		for _, cueList := range cueLists {
			cues, err := s.cueListRepo.GetCues(ctx, cueList.ID)
			if err != nil {
				return nil, nil, err
			}

			exportedCueList := ExportedCueList{
				RefID:       cueList.ID,
				OriginalID:  cueList.ID,
				Name:        cueList.Name,
				Description: cueList.Description,
				Loop:        cueList.Loop,
			}

			for _, cue := range cues {
				exportedCueList.Cues = append(exportedCueList.Cues, ExportedCue{
					OriginalID:  cue.ID,
					Name:        cue.Name,
					CueNumber:   cue.CueNumber,
					LookRefID:   cue.LookID,
					FadeInTime:  cue.FadeInTime,
					FadeOutTime: cue.FadeOutTime,
					FollowTime:  cue.FollowTime,
					EasingType:  cue.EasingType,
					Notes:       cue.Notes,
				})
				stats.CuesCount++
			}

			exported.CueLists = append(exported.CueLists, exportedCueList)
			stats.CueListsCount++
		}
	}

	// Export look boards
	if (opts.IncludeLookBoards || opts.IncludeSceneBoards) && s.lookBoardRepo != nil {
		boards, err := s.lookBoardRepo.FindByProjectID(ctx, projectID)
		if err != nil {
			return nil, nil, err
		}

		for _, board := range boards {
			buttons, err := s.lookBoardRepo.GetButtons(ctx, board.ID)
			if err != nil {
				return nil, nil, err
			}

			exportedBoard := ExportedLookBoard{
				RefID:           board.ID,
				OriginalID:      board.ID,
				Name:            board.Name,
				Description:     board.Description,
				DefaultFadeTime: board.DefaultFadeTime,
				GridSize:        board.GridSize,
				CanvasWidth:     board.CanvasWidth,
				CanvasHeight:    board.CanvasHeight,
			}

			for _, btn := range buttons {
				exportedBoard.Buttons = append(exportedBoard.Buttons, ExportedLookBoardButton{
					OriginalID: btn.ID,
					LookRefID:  btn.LookID,
					LayoutX:    btn.LayoutX,
					LayoutY:    btn.LayoutY,
					Width:      btn.Width,
					Height:     btn.Height,
					Color:      btn.Color,
					Label:      btn.Label,
				})
			}

			exported.LookBoards = append(exported.LookBoards, exportedBoard)
			stats.LookBoardsCount++
		}
	}

	// Populate deprecated fields for backward compatibility
	stats.ScenesCount = stats.LooksCount
	stats.SceneBoardsCount = stats.LookBoardsCount

	return exported, stats, nil
}

// ToJSON converts an exported project to JSON string.
func (e *ExportedProject) ToJSON() (string, error) {
	data, err := json.MarshalIndent(e, "", "  ")
	if err != nil {
		return "", err
	}
	return string(data), nil
}

// ParseExportedProject parses JSON into an ExportedProject.
// Handles backward compatibility for legacy "scenes" and "sceneBoards" JSON keys.
func ParseExportedProject(jsonContent string) (*ExportedProject, error) {
	var exported ExportedProject
	if err := json.Unmarshal([]byte(jsonContent), &exported); err != nil {
		return nil, err
	}

	// Backward compatibility: copy legacy "scenes" to "looks" if needed
	if len(exported.Looks) == 0 && len(exported.Scenes) > 0 {
		exported.Looks = exported.Scenes
	}

	// Backward compatibility: copy legacy "sceneBoards" to "lookBoards" if needed
	if len(exported.LookBoards) == 0 && len(exported.SceneBoards) > 0 {
		exported.LookBoards = exported.SceneBoards
	}

	return &exported, nil
}

// GetProjectName returns the project name from the exported data.
func (e *ExportedProject) GetProjectName() string {
	if e.Project != nil {
		return e.Project.Name
	}
	return ""
}

// GetProjectDescription returns the project description from the exported data.
func (e *ExportedProject) GetProjectDescription() *string {
	if e.Project != nil {
		return e.Project.Description
	}
	return nil
}
