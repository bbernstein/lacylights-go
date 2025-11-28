// Package export provides project export functionality.
package export

import (
	"context"
	"encoding/json"

	"github.com/bbernstein/lacylights-go/internal/database/repositories"
)

// ExportedProject represents a full project export.
type ExportedProject struct {
	Version            string                      `json:"version"`
	ProjectID          string                      `json:"projectId"`
	ProjectName        string                      `json:"projectName"`
	ProjectDescription *string                     `json:"projectDescription,omitempty"`
	FixtureDefinitions []ExportedFixtureDefinition `json:"fixtureDefinitions"`
	FixtureInstances   []ExportedFixtureInstance   `json:"fixtureInstances"`
	Scenes             []ExportedScene             `json:"scenes"`
	CueLists           []ExportedCueList           `json:"cueLists"`
}

// ExportedFixtureDefinition represents an exported fixture definition.
type ExportedFixtureDefinition struct {
	ID           string                     `json:"id"`
	Manufacturer string                     `json:"manufacturer"`
	Model        string                     `json:"model"`
	Type         string                     `json:"type"`
	IsBuiltIn    bool                       `json:"isBuiltIn"`
	Channels     []ExportedChannelDefinition `json:"channels"`
}

// ExportedChannelDefinition represents an exported channel definition.
type ExportedChannelDefinition struct {
	Name         string `json:"name"`
	Type         string `json:"type"`
	Offset       int    `json:"offset"`
	MinValue     int    `json:"minValue"`
	MaxValue     int    `json:"maxValue"`
	DefaultValue int    `json:"defaultValue"`
}

// ExportedFixtureInstance represents an exported fixture instance.
type ExportedFixtureInstance struct {
	ID           string   `json:"id"`
	Name         string   `json:"name"`
	Description  *string  `json:"description,omitempty"`
	DefinitionID string   `json:"definitionId"`
	Universe     int      `json:"universe"`
	StartChannel int      `json:"startChannel"`
	Tags         []string `json:"tags,omitempty"`
}

// ExportedScene represents an exported scene.
type ExportedScene struct {
	ID            string                 `json:"id"`
	Name          string                 `json:"name"`
	Description   *string                `json:"description,omitempty"`
	FixtureValues []ExportedFixtureValue `json:"fixtureValues"`
}

// ExportedFixtureValue represents exported fixture values in a scene.
type ExportedFixtureValue struct {
	FixtureID     string `json:"fixtureId"`
	ChannelValues []int  `json:"channelValues"`
	SceneOrder    *int   `json:"sceneOrder,omitempty"`
}

// ExportedCueList represents an exported cue list.
type ExportedCueList struct {
	ID          string        `json:"id"`
	Name        string        `json:"name"`
	Description *string       `json:"description,omitempty"`
	Loop        bool          `json:"loop"`
	Cues        []ExportedCue `json:"cues"`
}

// ExportedCue represents an exported cue.
type ExportedCue struct {
	ID          string   `json:"id"`
	Name        string   `json:"name"`
	CueNumber   float64  `json:"cueNumber"`
	SceneID     string   `json:"sceneId"`
	FadeInTime  float64  `json:"fadeInTime"`
	FadeOutTime float64  `json:"fadeOutTime"`
	FollowTime  *float64 `json:"followTime,omitempty"`
	EasingType  *string  `json:"easingType,omitempty"`
	Notes       *string  `json:"notes,omitempty"`
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
		Version:            "1.0",
		ProjectID:          project.ID,
		ProjectName:        project.Name,
		ProjectDescription: project.Description,
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
				ID:           def.ID,
				Manufacturer: def.Manufacturer,
				Model:        def.Model,
				Type:         def.Type,
				IsBuiltIn:    def.IsBuiltIn,
			}

			for _, ch := range channels {
				exportedDef.Channels = append(exportedDef.Channels, ExportedChannelDefinition{
					Name:         ch.Name,
					Type:         ch.Type,
					Offset:       ch.Offset,
					MinValue:     ch.MinValue,
					MaxValue:     ch.MaxValue,
					DefaultValue: ch.DefaultValue,
				})
			}

			exported.FixtureDefinitions = append(exported.FixtureDefinitions, exportedDef)
			stats.FixtureDefinitionsCount++
		}

		// Export fixture instances
		for _, f := range fixtures {
			var tags []string
			if f.Tags != nil {
				_ = json.Unmarshal([]byte(*f.Tags), &tags)
			}

			exported.FixtureInstances = append(exported.FixtureInstances, ExportedFixtureInstance{
				ID:           f.ID,
				Name:         f.Name,
				Description:  f.Description,
				DefinitionID: f.DefinitionID,
				Universe:     f.Universe,
				StartChannel: f.StartChannel,
				Tags:         tags,
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
				ID:          scene.ID,
				Name:        scene.Name,
				Description: scene.Description,
			}

			for _, fv := range fixtureValues {
				var channelValues []int
				_ = json.Unmarshal([]byte(fv.ChannelValues), &channelValues)

				exportedScene.FixtureValues = append(exportedScene.FixtureValues, ExportedFixtureValue{
					FixtureID:     fv.FixtureID,
					ChannelValues: channelValues,
					SceneOrder:    fv.SceneOrder,
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
				ID:          cueList.ID,
				Name:        cueList.Name,
				Description: cueList.Description,
				Loop:        cueList.Loop,
			}

			for _, cue := range cues {
				exportedCueList.Cues = append(exportedCueList.Cues, ExportedCue{
					ID:          cue.ID,
					Name:        cue.Name,
					CueNumber:   cue.CueNumber,
					SceneID:     cue.SceneID,
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
