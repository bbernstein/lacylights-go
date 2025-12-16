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
	Scenes             []ExportedScene             `json:"scenes"`
	CueLists           []ExportedCueList           `json:"cueLists"`
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
}

// ExportedFixtureInstance represents an exported fixture instance.
type ExportedFixtureInstance struct {
	RefID            string                    `json:"refId"`
	OriginalID       string                    `json:"originalId,omitempty"`
	Name             string                    `json:"name"`
	Description      *string                   `json:"description,omitempty"`
	DefinitionRefID  string                    `json:"definitionRefId"`
	ModeName         *string                   `json:"modeName,omitempty"`
	ChannelCount     *int                      `json:"channelCount,omitempty"`
	Universe         int                       `json:"universe"`
	StartChannel     int                       `json:"startChannel"`
	Tags             []string                  `json:"tags,omitempty"`
	ProjectOrder     *int                      `json:"projectOrder,omitempty"`
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
}

// ExportedScene represents an exported scene.
type ExportedScene struct {
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

// ExportedFixtureValue represents exported fixture values in a scene.
type ExportedFixtureValue struct {
	FixtureRefID  string                 `json:"fixtureRefId"`
	Channels      []ExportedChannelValue `json:"channels"`
	ChannelValues []int                  `json:"channelValues,omitempty"` // Read-only: used to import legacy dense array format, not populated on export
	SceneOrder    *int                   `json:"sceneOrder,omitempty"`
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
	SceneRefID  string   `json:"sceneRefId"`
	FadeInTime  float64  `json:"fadeInTime"`
	FadeOutTime float64  `json:"fadeOutTime"`
	FollowTime  *float64 `json:"followTime,omitempty"`
	EasingType  *string  `json:"easingType,omitempty"`
	Notes       *string  `json:"notes,omitempty"`
	CreatedAt   string   `json:"createdAt,omitempty"`
	UpdatedAt   string   `json:"updatedAt,omitempty"`
}

// ExportStats contains statistics about an export.
type ExportStats struct {
	FixtureDefinitionsCount int
	FixtureInstancesCount   int
	ScenesCount             int
	CueListsCount           int
	CuesCount               int
}

// Service handles project export operations.
type Service struct {
	projectRepo  *repositories.ProjectRepository
	fixtureRepo  *repositories.FixtureRepository
	sceneRepo    *repositories.SceneRepository
	cueListRepo  *repositories.CueListRepository
	cueRepo      *repositories.CueRepository
}

// NewService creates a new export service.
func NewService(
	projectRepo *repositories.ProjectRepository,
	fixtureRepo *repositories.FixtureRepository,
	sceneRepo *repositories.SceneRepository,
	cueListRepo *repositories.CueListRepository,
	cueRepo *repositories.CueRepository,
) *Service {
	return &Service{
		projectRepo:  projectRepo,
		fixtureRepo:  fixtureRepo,
		sceneRepo:    sceneRepo,
		cueListRepo:  cueListRepo,
		cueRepo:      cueRepo,
	}
}

// ExportProject exports a project to JSON.
func (s *Service) ExportProject(ctx context.Context, projectID string, includeFixtures, includeScenes, includeCueLists bool) (*ExportedProject, *ExportStats, error) {
	// Get project
	project, err := s.projectRepo.FindByID(ctx, projectID)
	if err != nil {
		return nil, nil, err
	}
	if project == nil {
		return nil, nil, nil
	}

	exported := &ExportedProject{
		Version: "1.0",
		Project: &ExportProjectInfo{
			OriginalID:  project.ID,
			Name:        project.Name,
			Description: project.Description,
		},
	}

	stats := &ExportStats{}

	// Export fixture definitions and instances
	if includeFixtures {
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
				})
			}

			// Export modes for this definition
			modes, err := s.fixtureRepo.GetDefinitionModes(ctx, defID)
			if err != nil {
				return nil, nil, err
			}

			for _, mode := range modes {
				exportedMode := ExportedFixtureMode{
					RefID:        mode.ID,
					Name:         mode.Name,
					ShortName:    mode.ShortName,
					ChannelCount: mode.ChannelCount,
				}

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

			exported.FixtureInstances = append(exported.FixtureInstances, ExportedFixtureInstance{
				RefID:           f.ID,
				OriginalID:      f.ID,
				Name:            f.Name,
				Description:     f.Description,
				DefinitionRefID: f.DefinitionID,
				ModeName:        f.ModeName,
				ChannelCount:    f.ChannelCount,
				Universe:        f.Universe,
				StartChannel:    f.StartChannel,
				Tags:            tags,
			})
			stats.FixtureInstancesCount++
		}
	}

	// Export scenes
	if includeScenes {
		scenes, err := s.sceneRepo.FindByProjectID(ctx, projectID)
		if err != nil {
			return nil, nil, err
		}

		for _, scene := range scenes {
			fixtureValues, err := s.sceneRepo.GetFixtureValues(ctx, scene.ID)
			if err != nil {
				return nil, nil, err
			}

			exportedScene := ExportedScene{
				RefID:       scene.ID,
				OriginalID:  scene.ID,
				Name:        scene.Name,
				Description: scene.Description,
			}

			for _, fv := range fixtureValues {
				var channels []models.ChannelValue
				if err := json.Unmarshal([]byte(fv.Channels), &channels); err != nil {
					log.Printf("Warning: failed to unmarshal channels for fixture %s in scene %s: %v", fv.FixtureID, scene.ID, err)
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

				exportedScene.FixtureValues = append(exportedScene.FixtureValues, ExportedFixtureValue{
					FixtureRefID: fv.FixtureID,
					Channels:     exportedChannels,
					SceneOrder:   fv.SceneOrder,
				})
			}

			exported.Scenes = append(exported.Scenes, exportedScene)
			stats.ScenesCount++
		}
	}

	// Export cue lists
	if includeCueLists {
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
					SceneRefID:  cue.SceneID,
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
func ParseExportedProject(jsonContent string) (*ExportedProject, error) {
	var exported ExportedProject
	if err := json.Unmarshal([]byte(jsonContent), &exported); err != nil {
		return nil, err
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
