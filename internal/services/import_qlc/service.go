package importqlc

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"path"
	"sort"
	"strings"

	"github.com/bbernstein/lacylights-go/internal/database/models"
	"github.com/bbernstein/lacylights-go/internal/database/repositories"
)

// DefaultProjectName names an imported project when the file name is empty.
const DefaultProjectName = "QLC+ Import"

// maxFixtureChannels caps a fixture's channel count at one DMX universe, so
// a corrupt or hostile <Channels> value can't make the importer build
// millions of channel rows.
const maxFixtureChannels = 512

// Options configures an import run.
type Options struct {
	// ProjectName is the new project's name; DefaultProjectName when blank.
	ProjectName string
	// Description is the new project's description (optional).
	Description string
	// GroupID assigns the new project to a user group (auth enabled).
	GroupID *string
}

// Result summarizes a successful import.
type Result struct {
	ProjectID                 string
	FixtureDefinitionsCreated int
	FixtureCount              int
	LookCount                 int
	CueListCount              int
	CueCount                  int
	Warnings                  []string
}

// Service imports QLC+ workspaces as new LacyLights projects.
type Service struct {
	projectRepo *repositories.ProjectRepository
	fixtureRepo *repositories.FixtureRepository
	lookRepo    *repositories.LookRepository
	cueListRepo *repositories.CueListRepository
	cueRepo     *repositories.CueRepository
}

// NewService constructs a Service.
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

// ProjectNameFromFileName derives a project name from an uploaded file name:
// any directory part and a trailing ".qxw" are removed.
func ProjectNameFromFileName(fileName string) string {
	name := strings.TrimSpace(strings.ReplaceAll(fileName, `\`, "/"))
	name = strings.TrimSpace(path.Base(name))
	if name == "." || name == "/" {
		name = ""
	}
	if strings.HasSuffix(strings.ToLower(name), ".qxw") {
		name = strings.TrimSpace(name[:len(name)-len(".qxw")])
	}
	if name == "" {
		return DefaultProjectName
	}
	return name
}

// Import parses a QLC+ workspace and creates a new project from it. Parse
// errors are returned before anything is written.
func (s *Service) Import(ctx context.Context, content string, opts Options) (*Result, error) {
	if s.projectRepo == nil || s.fixtureRepo == nil || s.lookRepo == nil || s.cueListRepo == nil || s.cueRepo == nil {
		return nil, errors.New("import_qlc: service not wired with repositories")
	}
	ws, err := Parse(content)
	if err != nil {
		return nil, err
	}

	name := strings.TrimSpace(opts.ProjectName)
	if name == "" {
		name = DefaultProjectName
	}
	project := &models.Project{Name: name, GroupID: opts.GroupID}
	if d := strings.TrimSpace(opts.Description); d != "" {
		project.Description = &d
	}
	if err := s.projectRepo.Create(ctx, project); err != nil {
		return nil, fmt.Errorf("create project: %w", err)
	}

	imp := &importer{
		svc:        s,
		projectID:  project.ID,
		defs:       newDefinitionResolver(s.fixtureRepo),
		fixtures:   map[string]importedFixture{},
		functions:  map[string]*Function{},
		sceneLooks: map[string]string{},
		result:     &Result{ProjectID: project.ID, Warnings: []string{}},
	}
	if err := imp.run(ctx, ws); err != nil {
		// Hide the partial project (soft delete, so it stays recoverable).
		_ = s.projectRepo.Delete(ctx, project.ID)
		return nil, err
	}
	return imp.result, nil
}

type importedFixture struct {
	id           string
	name         string
	channelCount int
}

type importer struct {
	svc        *Service
	projectID  string
	defs       *definitionResolver
	fixtures   map[string]importedFixture // QLC+ fixture ID -> created fixture
	functions  map[string]*Function       // QLC+ function ID -> function
	sceneLooks map[string]string          // QLC+ scene function ID -> look ID
	result     *Result
}

func (im *importer) warn(format string, args ...any) {
	im.result.Warnings = append(im.result.Warnings, fmt.Sprintf(format, args...))
}

func (im *importer) run(ctx context.Context, ws *Workspace) error {
	if err := im.importFixtures(ctx, ws.Fixtures); err != nil {
		return err
	}

	for i := range ws.Functions {
		fn := &ws.Functions[i]
		if _, dup := im.functions[fn.ID]; !dup {
			im.functions[fn.ID] = fn
		}
	}

	// Scenes first, so chasers can reference their looks.
	for i := range ws.Functions {
		fn := &ws.Functions[i]
		if fn.Type != TypeScene {
			continue
		}
		lookID, err := im.createLook(ctx, functionName(fn), fn.Values)
		if err != nil {
			return err
		}
		if _, exists := im.sceneLooks[fn.ID]; !exists {
			im.sceneLooks[fn.ID] = lookID
		}
	}

	skipped := map[string][]string{}
	for i := range ws.Functions {
		fn := &ws.Functions[i]
		var err error
		switch fn.Type {
		case TypeScene:
		case TypeChaser:
			err = im.importChaser(ctx, fn)
		case TypeSequence:
			err = im.importSequence(ctx, fn)
		default:
			skipped[fn.Type] = append(skipped[fn.Type], functionName(fn))
		}
		if err != nil {
			return err
		}
	}
	types := make([]string, 0, len(skipped))
	for t := range skipped {
		types = append(types, t)
	}
	sort.Strings(types)
	for _, t := range types {
		im.warn("Skipped %d QLC+ %s function(s), which LacyLights does not support: %s",
			len(skipped[t]), t, strings.Join(quoteAll(skipped[t]), ", "))
	}
	return nil
}

func (im *importer) importFixtures(ctx context.Context, fixtures []Fixture) error {
	if len(fixtures) == 0 {
		im.warn("The QLC+ workspace has no fixtures")
	}
	for i, fx := range fixtures {
		name := fx.Name
		if name == "" {
			name = strings.TrimSpace(fx.Manufacturer + " " + fx.Model)
		}
		if _, dup := im.fixtures[fx.ID]; dup {
			im.warn("QLC+ fixture %q has duplicate ID %s; skipped", name, fx.ID)
			continue
		}
		channels := fx.Channels
		if channels <= 0 {
			channels = 1 // as QLC+ does
		}
		if channels > maxFixtureChannels {
			im.warn("QLC+ fixture %q declares %d channels; only the first %d (one DMX universe) were imported",
				name, channels, maxFixtureChannels)
			channels = maxFixtureChannels
		}

		res, err := im.defs.resolve(ctx, fx, channels)
		if err != nil {
			return fmt.Errorf("resolve fixture definition for %q: %w", name, err)
		}

		instanceChannels := make([]models.InstanceChannel, len(res.channels))
		copy(instanceChannels, res.channels)
		for _, idx := range fx.ExcludeFade {
			if idx < len(instanceChannels) {
				instanceChannels[idx].FadeBehavior = "SNAP"
			}
		}

		description := fmt.Sprintf("Imported from QLC+: %s %s (%s)", fx.Manufacturer, fx.Model, fx.Mode)
		count := len(instanceChannels)
		order := i
		inst := &models.FixtureInstance{
			Name:         name,
			Description:  &description,
			DefinitionID: res.def.ID,
			ProjectID:    im.projectID,
			Universe:     fx.Universe + 1,
			StartChannel: fx.Address + 1,
			Manufacturer: &res.def.Manufacturer,
			Model:        &res.def.Model,
			Type:         &res.def.Type,
			ModeName:     res.modeName,
			ChannelCount: &count,
			ProjectOrder: &order,
		}
		if err := im.svc.fixtureRepo.CreateWithChannels(ctx, inst, instanceChannels); err != nil {
			return fmt.Errorf("create fixture %q: %w", name, err)
		}
		im.fixtures[fx.ID] = importedFixture{id: inst.ID, name: name, channelCount: count}
		im.result.FixtureCount++
	}
	im.result.FixtureDefinitionsCreated = im.defs.created
	im.result.Warnings = append(im.result.Warnings, im.defs.warnings...)
	return nil
}

// createLook creates a look from QLC+ fixture values. Values for unknown
// fixtures or channels are skipped with a warning; values are clamped to
// 0-255 and the last value for a channel wins, as in QLC+.
func (im *importer) createLook(ctx context.Context, name string, values []FixtureValues) (string, error) {
	var fixtureValues []models.FixtureValue
	for _, fv := range values {
		fix, ok := im.fixtures[fv.FixtureID]
		if !ok {
			im.warn("Look %q: QLC+ fixture ID %s is not in the imported patch; its values were skipped", name, fv.FixtureID)
			continue
		}
		var channels []models.ChannelValue
		index := map[int]int{}
		var dropped []string
		for _, cv := range fv.Channels {
			if cv.Channel >= fix.channelCount {
				dropped = append(dropped, fmt.Sprintf("channel %d", cv.Channel))
				continue
			}
			v := min(max(cv.Value, 0), 255)
			if i, seen := index[cv.Channel]; seen {
				channels[i].Value = v
				continue
			}
			index[cv.Channel] = len(channels)
			channels = append(channels, models.ChannelValue{Offset: cv.Channel, Value: v})
		}
		if len(dropped) > 0 {
			im.warn("Look %q: fixture %q has no %s; value skipped", name, fix.name, strings.Join(dropped, ", "))
		}
		if len(channels) == 0 {
			continue
		}
		b, err := json.Marshal(channels)
		if err != nil {
			return "", fmt.Errorf("encode values for look %q: %w", name, err)
		}
		order := len(fixtureValues)
		fixtureValues = append(fixtureValues, models.FixtureValue{
			FixtureID: fix.id,
			Channels:  string(b),
			LookOrder: &order,
		})
	}

	look := &models.Look{Name: name, ProjectID: im.projectID}
	if err := im.svc.lookRepo.CreateWithFixtureValues(ctx, look, fixtureValues); err != nil {
		return "", fmt.Errorf("create look %q: %w", name, err)
	}
	im.result.LookCount++
	return look.ID, nil
}

func (im *importer) createCueList(ctx context.Context, fn *Function) (*models.CueList, error) {
	name := functionName(fn)
	loop := true // QLC+'s default run order is Loop
	switch fn.RunOrder {
	case "SingleShot":
		loop = false
	case "PingPong", "Random":
		im.warn("Cue list %q: QLC+ run order %s is imported as a looping cue list", name, fn.RunOrder)
	}
	cl := &models.CueList{Name: name, Loop: loop, ProjectID: im.projectID}
	if err := im.svc.cueListRepo.Create(ctx, cl); err != nil {
		return nil, fmt.Errorf("create cue list %q: %w", name, err)
	}
	im.result.CueListCount++
	return cl, nil
}

type plannedCue struct {
	name   string
	lookID string
	speed  Speed // the referenced function's own speed, for "Default" fade modes
	step   Step
}

func (im *importer) createCues(ctx context.Context, fn *Function, cl *models.CueList, cues []plannedCue) error {
	if fn.Direction == "Backward" {
		for i, j := 0, len(cues)-1; i < j; i, j = i+1, j-1 {
			cues[i], cues[j] = cues[j], cues[i]
		}
	}
	for i, pc := range cues {
		fadeIn, fadeOut, follow := stepTimes(fn, pc.step, pc.speed)
		name := pc.name
		if pc.step.Note != "" {
			name = pc.step.Note
		}
		cue := &models.Cue{
			Name:        name,
			CueNumber:   float64(i + 1),
			CueListID:   cl.ID,
			LookID:      pc.lookID,
			FadeInTime:  fadeIn,
			FadeOutTime: fadeOut,
			FollowTime:  follow,
		}
		if err := im.svc.cueRepo.Create(ctx, cue); err != nil {
			return fmt.Errorf("create cue %d in %q: %w", i+1, cl.Name, err)
		}
		im.result.CueCount++
	}
	return nil
}

func (im *importer) importChaser(ctx context.Context, fn *Function) error {
	cl, err := im.createCueList(ctx, fn)
	if err != nil {
		return err
	}
	var cues []plannedCue
	for _, step := range fn.Steps {
		target := im.functions[step.FunctionID]
		lookID, ok := im.sceneLooks[step.FunctionID]
		if !ok || target == nil {
			if target == nil {
				im.warn("Cue list %q: step %d references unknown QLC+ function ID %s; step skipped", cl.Name, step.Number+1, step.FunctionID)
			} else {
				im.warn("Cue list %q: step %d plays QLC+ %s %q, which is not a scene; step skipped", cl.Name, step.Number+1, target.Type, functionName(target))
			}
			continue
		}
		cues = append(cues, plannedCue{name: functionName(target), lookID: lookID, speed: target.Speed, step: step})
	}
	return im.createCues(ctx, fn, cl, cues)
}

// importSequence imports a QLC+ sequence: each step becomes a look built from
// the bound scene's values overridden by the step's values.
func (im *importer) importSequence(ctx context.Context, fn *Function) error {
	var base []FixtureValues
	var baseSpeed Speed
	if bound := im.functions[fn.BoundSceneID]; bound != nil && bound.Type == TypeScene {
		base = bound.Values
		baseSpeed = bound.Speed
	} else {
		im.warn("Sequence %q: bound scene %s not found; steps use only their own values", functionName(fn), fn.BoundSceneID)
	}

	cl, err := im.createCueList(ctx, fn)
	if err != nil {
		return err
	}
	var cues []plannedCue
	for i, step := range fn.Steps {
		name := fmt.Sprintf("%s - Step %d", cl.Name, i+1)
		lookID, err := im.createLook(ctx, name, overlayValues(base, step.Values))
		if err != nil {
			return err
		}
		cues = append(cues, plannedCue{name: name, lookID: lookID, speed: baseSpeed, step: step})
	}
	return im.createCues(ctx, fn, cl, cues)
}

// overlayValues returns base with overrides applied per fixture/channel.
func overlayValues(base, overrides []FixtureValues) []FixtureValues {
	out := make([]FixtureValues, 0, len(base)+len(overrides))
	pos := map[string]int{}
	for _, fv := range base {
		pos[fv.FixtureID] = len(out)
		out = append(out, FixtureValues{FixtureID: fv.FixtureID, Channels: append([]ChannelValue(nil), fv.Channels...)})
	}
	for _, fv := range overrides {
		i, ok := pos[fv.FixtureID]
		if !ok {
			pos[fv.FixtureID] = len(out)
			out = append(out, FixtureValues{FixtureID: fv.FixtureID, Channels: append([]ChannelValue(nil), fv.Channels...)})
			continue
		}
		for _, cv := range fv.Channels {
			replaced := false
			for j := range out[i].Channels {
				if out[i].Channels[j].Channel == cv.Channel {
					out[i].Channels[j].Value = cv.Value
					replaced = true
					break
				}
			}
			if !replaced {
				out[i].Channels = append(out[i].Channels, cv)
			}
		}
	}
	return out
}

// stepTimes resolves a step's fade in/out and follow time in seconds
// according to the chaser's speed modes, as QLC+'s ChaserRunner does: for
// fades, PerStep uses the step's own times, Common the chaser's <Speed> and
// Default the played function's <Speed>; for the duration, PerStep uses the
// step's hold and both Common and Default the chaser's <Speed Duration>
// (ChaserRunner::stepDuration never defers to the played function).
// A hold of zero or "infinite" becomes a manual (no follow) cue.
func stepTimes(fn *Function, step Step, own Speed) (fadeIn, fadeOut float64, follow *float64) {
	pick := func(mode string, perStep, common, def uint32) uint32 {
		switch mode {
		case SpeedModePerStep:
			return perStep
		case SpeedModeCommon:
			return common
		default:
			return def
		}
	}
	fi := pick(fn.SpeedModes.FadeIn, step.FadeIn, fn.Speed.FadeIn, own.FadeIn)
	fo := pick(fn.SpeedModes.FadeOut, step.FadeOut, fn.Speed.FadeOut, own.FadeOut)
	chaserHold := holdFromDuration(fn.Speed.Duration, fi)
	hold := pick(fn.SpeedModes.Duration, step.Hold, chaserHold, chaserHold)

	fadeIn, fadeOut = seconds(fi), seconds(fo)
	if hold > 0 && hold < InfiniteSpeed {
		f := float64(hold) / 1000
		follow = &f
	}
	return fadeIn, fadeOut, follow
}

func holdFromDuration(duration, fadeIn uint32) uint32 {
	switch {
	case duration >= InfiniteSpeed:
		return duration
	case fadeIn >= InfiniteSpeed || duration <= fadeIn:
		return 0
	default:
		return duration - fadeIn
	}
}

func seconds(ms uint32) float64 {
	if ms >= InfiniteSpeed {
		return 0
	}
	return float64(ms) / 1000
}

func functionName(fn *Function) string {
	if fn.Name != "" {
		return fn.Name
	}
	return fmt.Sprintf("%s %s", fn.Type, fn.ID)
}

func quoteAll(ss []string) []string {
	out := make([]string, len(ss))
	for i, s := range ss {
		out[i] = fmt.Sprintf("%q", s)
	}
	return out
}
