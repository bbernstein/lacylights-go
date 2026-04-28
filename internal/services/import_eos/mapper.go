package importeos

import (
	"context"
	"fmt"

	"github.com/bbernstein/lacylights-go/internal/database/models"
	"github.com/bbernstein/lacylights-go/internal/database/repositories"
)

// Mapper applies a parsed Show + Sidecar to LacyLights repos.
type Mapper struct {
	projectRepo   *repositories.ProjectRepository
	fixtureRepo   *repositories.FixtureRepository
	lookRepo      *repositories.LookRepository
	cueListRepo   *repositories.CueListRepository
	cueRepo       *repositories.CueRepository
	lookBoardRepo *repositories.LookBoardRepository
}

// NewMapper constructs a Mapper.
func NewMapper(p *repositories.ProjectRepository, f *repositories.FixtureRepository,
	l *repositories.LookRepository, cl *repositories.CueListRepository,
	c *repositories.CueRepository, lb *repositories.LookBoardRepository) *Mapper {
	return &Mapper{p, f, l, cl, c, lb}
}

// Apply maps the parsed show into the database.
func (m *Mapper) Apply(ctx context.Context, show *Show, sidecar Sidecar, opts Options, warn *Collector) (*Result, error) {
	res := &Result{}

	projectID, err := m.resolveProject(ctx, show, opts)
	if err != nil {
		return nil, fmt.Errorf("resolve project: %w", err)
	}
	res.ProjectID = projectID

	table := NewParamTable(show.ParamTypes)

	matcher := NewMatcher(newRepoAdapter(m.fixtureRepo), table)
	persToDefID := make(map[int]string, len(show.Personalities))
	persToChannels := make(map[int][]models.ChannelDefinition, len(show.Personalities))
	for _, pers := range show.Personalities {
		mr, mWarns, err := matcher.Match(ctx, pers)
		if err != nil {
			return nil, fmt.Errorf("match personality %d: %w", pers.ID, err)
		}
		for _, w := range mWarns {
			warn.Add(w.Code, w.Severity, w.Message, w.Context)
		}
		var defID string
		var channels []models.ChannelDefinition
		if mr.ExistingDefinitionID != "" {
			defID = mr.ExistingDefinitionID
			channels, err = m.fixtureRepo.GetDefinitionChannels(ctx, defID)
			if err != nil {
				return nil, fmt.Errorf("load existing def channels: %w", err)
			}
		} else {
			if err := m.fixtureRepo.CreateDefinitionWithChannels(ctx, mr.SynthesizedDef, mr.SynthesizedChannels); err != nil {
				return nil, fmt.Errorf("create synth def: %w", err)
			}
			defID = mr.SynthesizedDef.ID
			channels = mr.SynthesizedChannels
			res.SynthesizedDefinitionIDs = append(res.SynthesizedDefinitionIDs, defID)
		}
		persToDefID[pers.ID] = defID
		persToChannels[pers.ID] = channels
		res.FixtureDefinitionsCount++
	}

	eosChannelToInstanceID := make(map[int]string, len(show.Patch))
	for _, pe := range show.Patch {
		defID, ok := persToDefID[pe.PersonalityID]
		if !ok {
			return nil, fmt.Errorf("$Patch %d references undeclared personality %d", pe.Channel, pe.PersonalityID)
		}
		universe, address, err := NormalizeAddress(pe.AddressRaw)
		if err != nil {
			return nil, err
		}
		label := pe.Label
		if pe.UnicodeText != nil {
			label = *pe.UnicodeText
		}
		if label == "" {
			label = fmt.Sprintf("Channel %d", pe.Channel)
		}
		order := pe.Channel
		instance := &models.FixtureInstance{
			ProjectID:    projectID,
			DefinitionID: defID,
			Name:         label,
			Universe:     universe,
			StartChannel: address,
			ProjectOrder: &order,
		}
		instanceChannels := createInstanceChannelsFor(persToChannels[pe.PersonalityID])
		if err := m.fixtureRepo.CreateWithChannels(ctx, instance, instanceChannels); err != nil {
			return nil, fmt.Errorf("create instance: %w", err)
		}
		eosChannelToInstanceID[pe.Channel] = instance.ID
		res.FixtureInstancesCount++
	}

	if err := m.applyPalettes(ctx, projectID, show, eosChannelToInstanceID, persToDefID, persToChannels, table, &res.LooksCount, &res.CueListsCount); err != nil {
		return nil, err
	}
	if err := m.applyCues(ctx, projectID, show, eosChannelToInstanceID, persToDefID, persToChannels, table, &res.LooksCount, &res.CueListsCount, &res.CuesCount); err != nil {
		return nil, err
	}
	if err := m.applyGroups(ctx, projectID, show, &res.GroupsCount); err != nil {
		return nil, err
	}
	if err := m.applySidecar(ctx, projectID, sidecar, warn); err != nil {
		return nil, err
	}

	res.Warnings = warn.All()
	return res, nil
}

func (m *Mapper) resolveProject(ctx context.Context, show *Show, opts Options) (string, error) {
	if opts.TargetProjectID != nil {
		return *opts.TargetProjectID, nil
	}
	name := show.Title
	if opts.NewProjectName != nil {
		name = *opts.NewProjectName
	}
	if name == "" {
		name = "Imported Eos Show"
	}
	p := &models.Project{Name: name}
	if err := m.projectRepo.Create(ctx, p); err != nil {
		return "", err
	}
	return p.ID, nil
}

// createInstanceChannelsFor copies definition channels to instance channels.
func createInstanceChannelsFor(defChannels []models.ChannelDefinition) []models.InstanceChannel {
	out := make([]models.InstanceChannel, 0, len(defChannels))
	for _, ch := range defChannels {
		out = append(out, models.InstanceChannel{
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
	return out
}

// Stubs for sub-tasks 14a-14c.
func (m *Mapper) applyPalettes(_ context.Context, _ string, _ *Show, _ map[int]string,
	_ map[int]string, _ map[int][]models.ChannelDefinition, _ *ParamTable,
	_ *int, _ *int) error {
	return nil
}

func (m *Mapper) applyCues(_ context.Context, _ string, _ *Show, _ map[int]string,
	_ map[int]string, _ map[int][]models.ChannelDefinition, _ *ParamTable,
	_ *int, _ *int, _ *int) error {
	return nil
}

func (m *Mapper) applyGroups(_ context.Context, _ string, _ *Show, _ *int) error {
	return nil
}

func (m *Mapper) applySidecar(_ context.Context, _ string, _ Sidecar, _ *Collector) error {
	return nil
}
