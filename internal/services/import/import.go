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
	SceneBoardsCreated        int
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
	projectRepo    *repositories.ProjectRepository
	fixtureRepo    *repositories.FixtureRepository
	sceneRepo      *repositories.SceneRepository
	cueListRepo    *repositories.CueListRepository
	cueRepo        *repositories.CueRepository
	sceneBoardRepo *repositories.SceneBoardRepository
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

// NewServiceWithSceneBoards creates a new import service with scene board support.
func NewServiceWithSceneBoards(
	projectRepo *repositories.ProjectRepository,
	fixtureRepo *repositories.FixtureRepository,
	sceneRepo *repositories.SceneRepository,
	cueListRepo *repositories.CueListRepository,
	cueRepo *repositories.CueRepository,
	sceneBoardRepo *repositories.SceneBoardRepository,
) *Service {
	return &Service{
		projectRepo:    projectRepo,
		fixtureRepo:    fixtureRepo,
		sceneRepo:      sceneRepo,
		cueListRepo:    cueListRepo,
		cueRepo:        cueRepo,
		sceneBoardRepo: sceneBoardRepo,
	}
}

// importModesForExistingDefinition imports modes from export data into an existing definition.
// It skips modes that already exist (matched by name).
// Returns warnings and a map of old mode refID -> mode name for imported modes.
func (s *Service) importModesForExistingDefinition(ctx context.Context, existingDefID string, exportedModes []export.ExportedFixtureMode, exportedChannels []export.ExportedChannelDefinition) ([]string, map[string]string, error) {
	var warnings []string
	modeRefIDToNameMap := make(map[string]string)

	// Get existing modes for this definition
	existingModes, err := s.fixtureRepo.GetDefinitionModes(ctx, existingDefID)
	if err != nil {
		return warnings, modeRefIDToNameMap, err
	}

	// Build a set of existing mode names and track refID -> name mapping for existing modes
	existingModeNames := make(map[string]bool)
	for _, m := range existingModes {
		existingModeNames[m.Name] = true
	}

	// Get existing channels for this definition to build name -> ID mapping
	existingChannels, err := s.fixtureRepo.GetDefinitionChannels(ctx, existingDefID)
	if err != nil {
		return warnings, modeRefIDToNameMap, err
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

	// Import each mode that doesn't already exist and track the mapping
	for _, mode := range exportedModes {
		if existingModeNames[mode.Name] {
			// Mode already exists, track the mapping
			modeRefIDToNameMap[mode.RefID] = mode.Name
			continue
		}

		newMode := &models.FixtureMode{
			Name:         mode.Name,
			ShortName:    mode.ShortName,
			ChannelCount: mode.ChannelCount,
			DefinitionID: existingDefID,
		}

		if err := s.fixtureRepo.CreateMode(ctx, newMode); err != nil {
			return warnings, modeRefIDToNameMap, err
		}

		// Track the mapping of old mode refID -> new mode name
		modeRefIDToNameMap[mode.RefID] = newMode.Name

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
				return warnings, modeRefIDToNameMap, err
			}
		}
	}

	return warnings, modeRefIDToNameMap, nil
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
	modeRefIDToNameMap := make(map[string]string) // old mode refID -> new mode name

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
					modeWarnings, modeMappings, err := s.importModesForExistingDefinition(ctx, existing.ID, def.Modes, def.Channels)
					if err != nil {
						return "", nil, nil, err
					}
					warnings = append(warnings, modeWarnings...)
					// Merge mode mappings into global map
					for oldRefID, modeName := range modeMappings {
						modeRefIDToNameMap[oldRefID] = modeName
					}
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
					modeWarnings, modeMappings, err := s.importModesForExistingDefinition(ctx, existing.ID, def.Modes, def.Channels)
					if err != nil {
						return "", nil, nil, err
					}
					warnings = append(warnings, modeWarnings...)
					// Merge mode mappings into global map
					for oldRefID, modeName := range modeMappings {
						modeRefIDToNameMap[oldRefID] = modeName
					}
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
					modeWarnings, modeMappings, err := s.importModesForExistingDefinition(ctx, existing.ID, def.Modes, def.Channels)
					if err != nil {
						return "", nil, nil, err
					}
					warnings = append(warnings, modeWarnings...)
					// Merge mode mappings into global map
					for oldRefID, modeName := range modeMappings {
						modeRefIDToNameMap[oldRefID] = modeName
					}
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
			// Default fade behavior to FADE if not specified
			fadeBehavior := ch.FadeBehavior
			if fadeBehavior == "" {
				fadeBehavior = "FADE"
			}
			channels = append(channels, models.ChannelDefinition{
				ID:           newChannelID,
				Name:         ch.Name,
				Type:         ch.Type,
				Offset:       ch.Offset,
				MinValue:     ch.MinValue,
				MaxValue:     ch.MaxValue,
				DefaultValue: ch.DefaultValue,
				FadeBehavior: fadeBehavior,
				IsDiscrete:   ch.IsDiscrete,
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

			// Track the mapping of old mode refID -> new mode name
			modeRefIDToNameMap[mode.RefID] = newMode.Name

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

		// Determine the correct mode name to use
		// Prefer modeRefId mapping if available, otherwise fall back to modeName
		var modeName *string
		if f.ModeRefID != nil && *f.ModeRefID != "" {
			// Use the mode refID to look up the correct mode name
			if mappedModeName, ok := modeRefIDToNameMap[*f.ModeRefID]; ok {
				modeName = &mappedModeName
			} else {
				// ModeRefID not found in map, fall back to ModeName
				modeName = f.ModeName
				if modeName != nil && *modeName != "" {
					warnings = append(warnings, "Mode refID '"+*f.ModeRefID+"' not found for fixture '"+f.Name+"', using mode name '"+*modeName+"' instead")
				}
			}
		} else {
			// No modeRefID, use the modeName directly (backwards compatibility)
			modeName = f.ModeName
		}

		newFixture := &models.FixtureInstance{
			Name:           f.Name,
			Description:    f.Description,
			DefinitionID:   newDefID,
			ProjectID:      projectID,
			Universe:       f.Universe,
			StartChannel:   f.StartChannel,
			Tags:           tagsJSON,
			Manufacturer:   &def.Manufacturer,
			Model:          &def.Model,
			Type:           &def.Type,
			ModeName:       modeName,
			ChannelCount:   f.ChannelCount,
			ProjectOrder:   f.ProjectOrder,
			LayoutX:        f.LayoutX,
			LayoutY:        f.LayoutY,
			LayoutRotation: f.LayoutRotation,
		}

		// Use instance channels from export if available, otherwise get from definition
		var instanceChannels []models.InstanceChannel
		if len(f.InstanceChannels) > 0 {
			for _, ch := range f.InstanceChannels {
				// Default fade behavior to FADE if not specified
				fadeBehavior := ch.FadeBehavior
				if fadeBehavior == "" {
					fadeBehavior = "FADE"
				}
				instanceChannels = append(instanceChannels, models.InstanceChannel{
					Offset:       ch.Offset,
					Name:         ch.Name,
					Type:         ch.Type,
					MinValue:     ch.MinValue,
					MaxValue:     ch.MaxValue,
					DefaultValue: ch.DefaultValue,
					FadeBehavior: fadeBehavior,
					IsDiscrete:   ch.IsDiscrete,
				})
			}
		} else {
			// Get channels based on the selected mode, or all channels if no mode specified
			if modeName != nil && *modeName != "" {
				// Get the mode for this fixture
				modes, err := s.fixtureRepo.GetDefinitionModes(ctx, newDefID)
				if err != nil {
					return "", nil, nil, err
				}

				// Find the mode by name
				var selectedMode *models.FixtureMode
				for i := range modes {
					if modes[i].Name == *modeName {
						selectedMode = &modes[i]
						break
					}
				}

				if selectedMode != nil {
					// Get mode channels (these define which channels are used and in what order)
					modeChannels, err := s.fixtureRepo.GetModeChannels(ctx, selectedMode.ID)
					if err != nil {
						return "", nil, nil, err
					}

					// Get all channel definitions
					allChannels, err := s.fixtureRepo.GetDefinitionChannels(ctx, newDefID)
					if err != nil {
						return "", nil, nil, err
					}

					// Build a map of channel ID to channel definition
					channelMap := make(map[string]models.ChannelDefinition)
					for _, ch := range allChannels {
						channelMap[ch.ID] = ch
					}

					// Create instance channels from mode channels (in mode order)
					for _, mc := range modeChannels {
						if ch, ok := channelMap[mc.ChannelID]; ok {
							instanceChannels = append(instanceChannels, models.InstanceChannel{
								Offset:       mc.Offset, // Use mode's offset, not definition's offset
								Name:         ch.Name,
								Type:         ch.Type,
								MinValue:     ch.MinValue,
								MaxValue:     ch.MaxValue,
								DefaultValue: ch.DefaultValue,
								FadeBehavior: ch.FadeBehavior,
								IsDiscrete:   ch.IsDiscrete,
							})
						}
					}
				} else {
					// Mode not found, fall back to all definition channels
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
							FadeBehavior: ch.FadeBehavior,
							IsDiscrete:   ch.IsDiscrete,
						})
					}
				}
			} else {
				// No mode specified, use all definition channels
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
						FadeBehavior: ch.FadeBehavior,
						IsDiscrete:   ch.IsDiscrete,
					})
				}
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

	// Import scene boards
	// Note: Scene boards are imported regardless of includeScenes flag. If scenes were not
	// included in the import (or failed to import), scene board buttons referencing those
	// scenes will be skipped with a warning. This allows partial imports while maintaining
	// data integrity.
	if s.sceneBoardRepo != nil && len(exported.SceneBoards) > 0 {
		for _, board := range exported.SceneBoards {
			newBoard := &models.SceneBoard{
				Name:            board.Name,
				Description:     board.Description,
				DefaultFadeTime: board.DefaultFadeTime,
				GridSize:        board.GridSize,
				CanvasWidth:     board.CanvasWidth,
				CanvasHeight:    board.CanvasHeight,
				ProjectID:       projectID,
			}

			var buttons []models.SceneBoardButton
			for _, btn := range board.Buttons {
				newSceneID, ok := sceneIDMap[btn.SceneRefID]
				if !ok {
					warnings = append(warnings, "Skipping scene board button with unknown scene in board: "+board.Name)
					continue
				}

				buttons = append(buttons, models.SceneBoardButton{
					SceneID: newSceneID,
					LayoutX: btn.LayoutX,
					LayoutY: btn.LayoutY,
					Width:   btn.Width,
					Height:  btn.Height,
					Color:   btn.Color,
					Label:   btn.Label,
				})
			}

			if err := s.sceneBoardRepo.CreateWithButtons(ctx, newBoard, buttons); err != nil {
				return "", nil, nil, err
			}
			stats.SceneBoardsCreated++
		}
	}

	return projectID, stats, warnings, nil
}
