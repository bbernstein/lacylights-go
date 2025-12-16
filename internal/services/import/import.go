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

// importModesForExistingDefinition imports modes from export data into an existing definition.
// It skips modes that already exist (matched by name).
func (s *Service) importModesForExistingDefinition(ctx context.Context, existingDefID string, exportedModes []export.ExportedFixtureMode, exportedChannels []export.ExportedChannelDefinition) ([]string, error) {
	var warnings []string

	// Get existing modes for this definition
	existingModes, err := s.fixtureRepo.GetDefinitionModes(ctx, existingDefID)
	if err != nil {
		return warnings, err
	}

	// Build a set of existing mode names
	existingModeNames := make(map[string]bool)
	for _, m := range existingModes {
		existingModeNames[m.Name] = true
	}

	// Get existing channels for this definition to build name -> ID mapping
	existingChannels, err := s.fixtureRepo.GetDefinitionChannels(ctx, existingDefID)
	if err != nil {
		return warnings, err
	}

	// Build channel name -> existing channel ID mapping
	channelNameToID := make(map[string]string)
	for _, ch := range existingChannels {
		channelNameToID[ch.Name] = ch.ID
	}

	// Build export RefID -> channel name mapping from exported channels
	exportRefIDToName := make(map[string]string)
	for _, ch := range exportedChannels {
		if ch.RefID != "" {
			exportRefIDToName[ch.RefID] = ch.Name
		}
	}

	// Import each mode that doesn't already exist
	for _, mode := range exportedModes {
		if existingModeNames[mode.Name] {
			// Mode already exists, skip
			continue
		}

		newMode := &models.FixtureMode{
			Name:         mode.Name,
			ShortName:    mode.ShortName,
			ChannelCount: mode.ChannelCount,
			DefinitionID: existingDefID,
		}

		if err := s.fixtureRepo.CreateMode(ctx, newMode); err != nil {
			return warnings, err
		}

		// Create mode channels
		var modeChannels []models.ModeChannel
		for _, mc := range mode.ModeChannels {
			// First try to map RefID -> channel name -> existing channel ID
			var existingChannelID string
			if channelName, ok := exportRefIDToName[mc.ChannelRefID]; ok {
				existingChannelID = channelNameToID[channelName]
			}
			// Fallback: try using RefID directly as channel name
			if existingChannelID == "" {
				existingChannelID = channelNameToID[mc.ChannelRefID]
			}

			if existingChannelID == "" {
				// Use channel name when available, otherwise use RefID
				unknownChannel := mc.ChannelRefID
				if channelName, ok := exportRefIDToName[mc.ChannelRefID]; ok {
					unknownChannel = channelName
				}
				warnings = append(warnings, "Mode '"+mode.Name+"' references unknown channel '"+unknownChannel+"'")
				continue
			}

			modeChannels = append(modeChannels, models.ModeChannel{
				ModeID:    newMode.ID,
				ChannelID: existingChannelID,
				Offset:    mc.Offset,
			})
		}

		if len(modeChannels) > 0 {
			if err := s.fixtureRepo.CreateModeChannels(ctx, modeChannels); err != nil {
				return warnings, err
			}
		}
	}

	return warnings, nil
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
				// Import modes that don't already exist on the existing definition
				if len(def.Modes) > 0 {
					modeWarnings, err := s.importModesForExistingDefinition(ctx, existing.ID, def.Modes, def.Channels)
					if err != nil {
						return "", nil, nil, err
					}
					warnings = append(warnings, modeWarnings...)
				}
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
				// Import modes that don't already exist on the existing definition
				if len(def.Modes) > 0 {
					modeWarnings, err := s.importModesForExistingDefinition(ctx, existing.ID, def.Modes, def.Channels)
					if err != nil {
						return "", nil, nil, err
					}
					warnings = append(warnings, modeWarnings...)
				}
				warnings = append(warnings, "Skipped existing fixture definition: "+def.Manufacturer+" "+def.Model)
				continue
			case FixtureConflictReplace:
				// For fixture definitions, "Replace" behaves like "Skip" - we reuse the
				// existing definition and merge new modes. We don't delete the existing
				// definition because it may be used by other projects. The "Replace"
				// strategy is more meaningful at the project level (replacing project
				// data) rather than globally shared fixture definitions.
				definitionIDMap[def.RefID] = existing.ID
				// Import modes that don't already exist on the existing definition
				if len(def.Modes) > 0 {
					modeWarnings, err := s.importModesForExistingDefinition(ctx, existing.ID, def.Modes, def.Channels)
					if err != nil {
						return "", nil, nil, err
					}
					warnings = append(warnings, modeWarnings...)
				}
				warnings = append(warnings, "Reused existing fixture definition (Replace merges modes): "+def.Manufacturer+" "+def.Model)
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

		// Build channels and track RefID -> new ID mapping
		var channels []models.ChannelDefinition
		channelRefIDMap := make(map[string]string) // old RefID -> new ID
		for _, ch := range def.Channels {
			newChannelID := cuid.New()
			channels = append(channels, models.ChannelDefinition{
				ID:           newChannelID,
				Name:         ch.Name,
				Type:         ch.Type,
				Offset:       ch.Offset,
				MinValue:     ch.MinValue,
				MaxValue:     ch.MaxValue,
				DefaultValue: ch.DefaultValue,
			})
			// Map the old RefID to the new ID
			if ch.RefID != "" {
				channelRefIDMap[ch.RefID] = newChannelID
			} else {
				// Fallback: use channel name as key if RefID not available
				channelRefIDMap[ch.Name] = newChannelID
			}
		}

		if err := s.fixtureRepo.CreateDefinitionWithChannels(ctx, newDef, channels); err != nil {
			return "", nil, nil, err
		}
		definitionIDMap[def.RefID] = newDef.ID
		stats.FixtureDefinitionsCreated++

		// Import modes for this definition
		for _, mode := range def.Modes {
			newMode := &models.FixtureMode{
				Name:         mode.Name,
				ShortName:    mode.ShortName,
				ChannelCount: mode.ChannelCount,
				DefinitionID: newDef.ID,
			}

			if err := s.fixtureRepo.CreateMode(ctx, newMode); err != nil {
				return "", nil, nil, err
			}

			// Create mode channels
			var modeChannels []models.ModeChannel
			for _, mc := range mode.ModeChannels {
				newChannelID, ok := channelRefIDMap[mc.ChannelRefID]
				if !ok {
					warnings = append(warnings, "Mode channel references unknown channel: "+mc.ChannelRefID)
					continue
				}
				modeChannels = append(modeChannels, models.ModeChannel{
					ModeID:    newMode.ID,
					ChannelID: newChannelID,
					Offset:    mc.Offset,
				})
			}

			if err := s.fixtureRepo.CreateModeChannels(ctx, modeChannels); err != nil {
				return "", nil, nil, err
			}
		}
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
			ModeName:     f.ModeName,
			ChannelCount: f.ChannelCount,
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
		// Only set channel count from instance channels if not already set from import
		if newFixture.ChannelCount == nil {
			channelCount := len(instanceChannels)
			newFixture.ChannelCount = &channelCount
		}

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
				warnings = append(warnings, "Skipping fixture value with unknown fixture '"+fv.FixtureRefID+"' in scene '"+scene.Name+"'")
				continue
			}

			// Convert exported channels to models.ChannelValue
			// Support both sparse format (channels) and legacy array format (channelValues)
			var channels []models.ChannelValue
			if len(fv.Channels) > 0 {
				// New sparse format: [{offset: 0, value: 255}, ...]
				channels = make([]models.ChannelValue, len(fv.Channels))
				for i, ch := range fv.Channels {
					channels[i] = models.ChannelValue{
						Offset: ch.Offset,
						Value:  ch.Value,
					}
				}
			} else if len(fv.ChannelValues) > 0 {
				// Legacy array format: [255, 128, 0, 0] - index is the offset
				channels = make([]models.ChannelValue, len(fv.ChannelValues))
				for i, val := range fv.ChannelValues {
					channels[i] = models.ChannelValue{
						Offset: i,
						Value:  val,
					}
				}
			}
			channelsJSON, err := json.Marshal(channels)
			if err != nil {
				warnings = append(warnings, "Skipping fixture value for fixture '"+fv.FixtureRefID+"' in scene '"+scene.Name+"' due to JSON marshaling error: "+err.Error())
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
