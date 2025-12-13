// Package importservice provides project import functionality.
package importservice

import (
	"context"
	"encoding/json"

	"github.com/bbernstein/lacylights-go/internal/database/models"
	"github.com/bbernstein/lacylights-go/internal/database/repositories"
	"github.com/bbernstein/lacylights-go/internal/services/export"
	"github.com/lucsky/cuid"
)

// ImportMode determines how to handle the import.
type ImportMode string

const (
	ImportModeCreate  ImportMode = "CREATE"
	ImportModeMerge   ImportMode = "MERGE"
	ImportModeReplace ImportMode = "REPLACE"
)

// FixtureConflictStrategy determines how to handle fixture conflicts.
type FixtureConflictStrategy string

const (
	FixtureConflictSkip    FixtureConflictStrategy = "SKIP"
	FixtureConflictReplace FixtureConflictStrategy = "REPLACE"
	FixtureConflictRename  FixtureConflictStrategy = "RENAME"
)

// ImportStats contains statistics about an import.
type ImportStats struct {
	FixtureDefinitionsCreated int
	FixtureInstancesCreated   int
	ScenesCreated             int
	CueListsCreated           int
	CuesCreated               int
}

// ImportOptions configures the import behavior.
type ImportOptions struct {
	Mode                    ImportMode
	TargetProjectID         *string
	ProjectName             *string
	FixtureConflictStrategy FixtureConflictStrategy
	ImportBuiltInFixtures   bool
}

// Service handles project import operations.
type Service struct {
	projectRepo  *repositories.ProjectRepository
	fixtureRepo  *repositories.FixtureRepository
	sceneRepo    *repositories.SceneRepository
	cueListRepo  *repositories.CueListRepository
	cueRepo      *repositories.CueRepository
}

// NewService creates a new import service.
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

// ImportProject imports a project from JSON.
func (s *Service) ImportProject(ctx context.Context, jsonContent string, options ImportOptions) (string, *ImportStats, []string, error) {
	// Parse the JSON
	exported, err := export.ParseExportedProject(jsonContent)
	if err != nil {
		return "", nil, nil, err
	}

	stats := &ImportStats{}
	var warnings []string

	// Determine the target project
	var projectID string

	switch options.Mode {
	case ImportModeCreate:
		// Create a new project
		projectName := exported.GetProjectName()
		if options.ProjectName != nil {
			projectName = *options.ProjectName
		}

		project := &models.Project{
			Name:        projectName,
			Description: exported.GetProjectDescription(),
		}
		if err := s.projectRepo.Create(ctx, project); err != nil {
			return "", nil, nil, err
		}
		projectID = project.ID

	case ImportModeMerge, ImportModeReplace:
		if options.TargetProjectID == nil {
			return "", nil, nil, nil // No target project specified
		}
		projectID = *options.TargetProjectID

		// Verify project exists
		existing, err := s.projectRepo.FindByID(ctx, projectID)
		if err != nil {
			return "", nil, nil, err
		}
		if existing == nil {
			return "", nil, nil, nil // Project not found
		}

		// For REPLACE mode, we would delete existing data first
		// For now, we treat both as merge
	}

	// Track ID mappings for references
	definitionIDMap := make(map[string]string) // old ID -> new ID
	fixtureIDMap := make(map[string]string)    // old ID -> new ID
	sceneIDMap := make(map[string]string)      // old ID -> new ID

	// Import fixture definitions
	for _, def := range exported.FixtureDefinitions {
		if def.IsBuiltIn && !options.ImportBuiltInFixtures {
			// Use existing built-in definition
			existing, err := s.fixtureRepo.FindDefinitionByManufacturerModel(ctx, def.Manufacturer, def.Model)
			if err != nil {
				return "", nil, nil, err
			}
			if existing != nil {
				definitionIDMap[def.RefID] = existing.ID
				continue
			}
		}

		// Check for existing definition
		existing, err := s.fixtureRepo.FindDefinitionByManufacturerModel(ctx, def.Manufacturer, def.Model)
		if err != nil {
			return "", nil, nil, err
		}

		if existing != nil {
			switch options.FixtureConflictStrategy {
			case FixtureConflictSkip:
				definitionIDMap[def.RefID] = existing.ID
				warnings = append(warnings, "Skipped existing fixture definition: "+def.Manufacturer+" "+def.Model)
				continue
			case FixtureConflictReplace:
				// Delete and recreate
				definitionIDMap[def.RefID] = existing.ID
				// For now, just use existing
				continue
			case FixtureConflictRename:
				// Will create with new ID
			}
		}

		// Create new definition
		newDef := &models.FixtureDefinition{
			Manufacturer: def.Manufacturer,
			Model:        def.Model,
			Type:         def.Type,
			IsBuiltIn:    false, // Imported definitions are not built-in
		}

		var channels []models.ChannelDefinition
		for _, ch := range def.Channels {
			channels = append(channels, models.ChannelDefinition{
				Name:         ch.Name,
				Type:         ch.Type,
				Offset:       ch.Offset,
				MinValue:     ch.MinValue,
				MaxValue:     ch.MaxValue,
				DefaultValue: ch.DefaultValue,
			})
		}

		if err := s.fixtureRepo.CreateDefinitionWithChannels(ctx, newDef, channels); err != nil {
			return "", nil, nil, err
		}
		definitionIDMap[def.RefID] = newDef.ID
		stats.FixtureDefinitionsCreated++
	}

	// Import fixture instances
	for _, f := range exported.FixtureInstances {
		newDefID, ok := definitionIDMap[f.DefinitionRefID]
		if !ok {
			warnings = append(warnings, "Skipping fixture instance with unknown definition: "+f.Name)
			continue
		}

		// Get the definition for denormalized fields
		def, err := s.fixtureRepo.FindDefinitionByID(ctx, newDefID)
		if err != nil {
			return "", nil, nil, err
		}
		if def == nil {
			warnings = append(warnings, "Definition not found for fixture: "+f.Name)
			continue
		}

		var tagsJSON *string
		if len(f.Tags) > 0 {
			data, _ := json.Marshal(f.Tags)
			str := string(data)
			tagsJSON = &str
		}

		newFixture := &models.FixtureInstance{
			Name:         f.Name,
			Description:  f.Description,
			DefinitionID: newDefID,
			ProjectID:    projectID,
			Universe:     f.Universe,
			StartChannel: f.StartChannel,
			Tags:         tagsJSON,
			Manufacturer: &def.Manufacturer,
			Model:        &def.Model,
			Type:         &def.Type,
		}

		// Use instance channels from export if available, otherwise get from definition
		var instanceChannels []models.InstanceChannel
		if len(f.InstanceChannels) > 0 {
			for _, ch := range f.InstanceChannels {
				instanceChannels = append(instanceChannels, models.InstanceChannel{
					Offset:       ch.Offset,
					Name:         ch.Name,
					Type:         ch.Type,
					MinValue:     ch.MinValue,
					MaxValue:     ch.MaxValue,
					DefaultValue: ch.DefaultValue,
				})
			}
		} else {
			// Get channels from definition
			channels, err := s.fixtureRepo.GetDefinitionChannels(ctx, newDefID)
			if err != nil {
				return "", nil, nil, err
			}

			for _, ch := range channels {
				instanceChannels = append(instanceChannels, models.InstanceChannel{
					Offset:       ch.Offset,
					Name:         ch.Name,
					Type:         ch.Type,
					MinValue:     ch.MinValue,
					MaxValue:     ch.MaxValue,
					DefaultValue: ch.DefaultValue,
				})
			}
		}
		channelCount := len(instanceChannels)
		newFixture.ChannelCount = &channelCount

		if err := s.fixtureRepo.CreateWithChannels(ctx, newFixture, instanceChannels); err != nil {
			return "", nil, nil, err
		}
		fixtureIDMap[f.RefID] = newFixture.ID
		stats.FixtureInstancesCreated++
	}

	// Import scenes
	for _, scene := range exported.Scenes {
		newScene := &models.Scene{
			Name:        scene.Name,
			Description: scene.Description,
			ProjectID:   projectID,
		}

		var fixtureValues []models.FixtureValue
		for _, fv := range scene.FixtureValues {
			newFixtureID, ok := fixtureIDMap[fv.FixtureRefID]
			if !ok {
				warnings = append(warnings, "Skipping fixture value with unknown fixture in scene: "+scene.Name)
				continue
			}

			// Convert exported channels to models.ChannelValue
			channels := make([]models.ChannelValue, len(fv.Channels))
			for i, ch := range fv.Channels {
				channels[i] = models.ChannelValue{
					Offset: ch.Offset,
					Value:  ch.Value,
				}
			}
			channelsJSON, err := json.Marshal(channels)
			if err != nil {
				warnings = append(warnings, "Skipping fixture value due to JSON marshaling error in scene '"+scene.Name+"': "+err.Error())
				continue
			}
			fixtureValues = append(fixtureValues, models.FixtureValue{
				ID:        cuid.New(),
				FixtureID: newFixtureID,
				Channels:  string(channelsJSON),
				SceneOrder: fv.SceneOrder,
			})
		}

		if err := s.sceneRepo.CreateWithFixtureValues(ctx, newScene, fixtureValues); err != nil {
			return "", nil, nil, err
		}
		sceneIDMap[scene.RefID] = newScene.ID
		stats.ScenesCreated++
	}

	// Import cue lists
	for _, cueList := range exported.CueLists {
		newCueList := &models.CueList{
			Name:        cueList.Name,
			Description: cueList.Description,
			Loop:        cueList.Loop,
			ProjectID:   projectID,
		}

		if err := s.cueListRepo.Create(ctx, newCueList); err != nil {
			return "", nil, nil, err
		}
		stats.CueListsCreated++

		// Import cues
		for _, cue := range cueList.Cues {
			newSceneID, ok := sceneIDMap[cue.SceneRefID]
			if !ok {
				warnings = append(warnings, "Skipping cue with unknown scene in cue list: "+cueList.Name)
				continue
			}

			newCue := &models.Cue{
				Name:        cue.Name,
				CueNumber:   cue.CueNumber,
				CueListID:   newCueList.ID,
				SceneID:     newSceneID,
				FadeInTime:  cue.FadeInTime,
				FadeOutTime: cue.FadeOutTime,
				FollowTime:  cue.FollowTime,
				EasingType:  cue.EasingType,
				Notes:       cue.Notes,
			}

			if err := s.cueRepo.Create(ctx, newCue); err != nil {
				return "", nil, nil, err
			}
			stats.CuesCreated++
		}
	}

	return projectID, stats, warnings, nil
}
