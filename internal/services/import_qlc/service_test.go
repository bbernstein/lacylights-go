package importqlc

import (
	"context"
	"encoding/json"
	"sort"
	"strings"
	"testing"

	"github.com/bbernstein/lacylights-go/internal/database/migrations"
	"github.com/bbernstein/lacylights-go/internal/database/models"
	"github.com/bbernstein/lacylights-go/internal/database/repositories"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

type testDeps struct {
	db          *gorm.DB
	projectRepo *repositories.ProjectRepository
	fixtureRepo *repositories.FixtureRepository
	lookRepo    *repositories.LookRepository
	cueListRepo *repositories.CueListRepository
	cueRepo     *repositories.CueRepository
	svc         *Service
}

func newTestDeps(t *testing.T) *testDeps {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(
		&models.Project{},
		&models.FixtureDefinition{},
		&models.ChannelDefinition{},
		&models.FixtureMode{},
		&models.ModeChannel{},
		&models.FixtureInstance{},
		&models.InstanceChannel{},
		&models.Look{},
		&models.FixtureValue{},
		&models.CueList{},
		&models.Cue{},
		&models.LookBoard{},
		&models.LookBoardButton{},
		&models.FixtureGroup{},
		&models.FixtureGroupMember{},
	))
	// Production creates ref_id unique indexes at boot; tests must too.
	require.NoError(t, migrations.CreateRefIDIndexes(db))
	t.Cleanup(func() {
		if sqlDB, err := db.DB(); err == nil {
			_ = sqlDB.Close()
		}
	})
	d := &testDeps{
		db:          db,
		projectRepo: repositories.NewProjectRepository(db),
		fixtureRepo: repositories.NewFixtureRepository(db),
		lookRepo:    repositories.NewLookRepository(db),
		cueListRepo: repositories.NewCueListRepository(db),
		cueRepo:     repositories.NewCueRepository(db),
	}
	d.svc = NewService(d.projectRepo, d.fixtureRepo, d.lookRepo, d.cueListRepo, d.cueRepo)
	return d
}

// seedLibraryDefinition creates a built-in definition with one mode per
// entry in modes (mode name -> channel types, in DMX order).
func (d *testDeps) seedLibraryDefinition(t *testing.T, manufacturer, model string, modes map[string][]string) *models.FixtureDefinition {
	t.Helper()
	ctx := context.Background()
	def := &models.FixtureDefinition{Manufacturer: manufacturer, Model: model, Type: "LED_PAR", IsBuiltIn: true}

	// One channel definition per distinct type name.
	names := map[string]bool{}
	var channels []models.ChannelDefinition
	modeNames := make([]string, 0, len(modes))
	for name := range modes {
		modeNames = append(modeNames, name)
	}
	sort.Strings(modeNames)
	for _, name := range modeNames {
		for _, typ := range modes[name] {
			if !names[typ] {
				names[typ] = true
				channels = append(channels, models.ChannelDefinition{
					Name: strings.ToLower(typ), Type: typ, Offset: len(channels), MaxValue: 255,
					FadeBehavior: "FADE",
				})
			}
		}
	}
	require.NoError(t, d.fixtureRepo.CreateDefinitionWithChannels(ctx, def, channels))
	byType := map[string]string{}
	for _, ch := range channels {
		byType[ch.Type] = ch.ID
	}
	for _, name := range modeNames {
		mode := &models.FixtureMode{Name: name, ChannelCount: len(modes[name]), DefinitionID: def.ID}
		require.NoError(t, d.fixtureRepo.CreateMode(ctx, mode))
		var mcs []models.ModeChannel
		for i, typ := range modes[name] {
			mcs = append(mcs, models.ModeChannel{ModeID: mode.ID, ChannelID: byType[typ], Offset: i})
		}
		require.NoError(t, d.fixtureRepo.CreateModeChannels(ctx, mcs))
	}
	return def
}

func (d *testDeps) countDefinitions(t *testing.T) int64 {
	t.Helper()
	n, err := d.fixtureRepo.CountDefinitions(context.Background())
	require.NoError(t, err)
	return n
}

type fixtureView struct {
	inst     models.FixtureInstance
	def      *models.FixtureDefinition
	channels []models.InstanceChannel
}

func (d *testDeps) fixturesByName(t *testing.T, projectID string) map[string]fixtureView {
	t.Helper()
	ctx := context.Background()
	fixtures, err := d.fixtureRepo.FindByProjectID(ctx, projectID)
	require.NoError(t, err)
	out := map[string]fixtureView{}
	for _, f := range fixtures {
		def, err := d.fixtureRepo.FindDefinitionByID(ctx, f.DefinitionID)
		require.NoError(t, err)
		chs, err := d.fixtureRepo.GetInstanceChannels(ctx, f.ID)
		require.NoError(t, err)
		sort.Slice(chs, func(i, j int) bool { return chs[i].Offset < chs[j].Offset })
		out[f.Name] = fixtureView{inst: f, def: def, channels: chs}
	}
	return out
}

func channelTypes(chs []models.InstanceChannel) []string {
	out := make([]string, len(chs))
	for i, c := range chs {
		out[i] = c.Type
	}
	return out
}

// lookValues returns look name -> fixture name -> channel values.
func (d *testDeps) lookValues(t *testing.T, projectID string) map[string]map[string][]models.ChannelValue {
	t.Helper()
	ctx := context.Background()
	fixtures, err := d.fixtureRepo.FindByProjectID(ctx, projectID)
	require.NoError(t, err)
	names := map[string]string{}
	for _, f := range fixtures {
		names[f.ID] = f.Name
	}
	looks, err := d.lookRepo.FindByProjectID(ctx, projectID)
	require.NoError(t, err)
	out := map[string]map[string][]models.ChannelValue{}
	for _, l := range looks {
		fvs, err := d.lookRepo.GetFixtureValues(ctx, l.ID)
		require.NoError(t, err)
		m := map[string][]models.ChannelValue{}
		for _, fv := range fvs {
			var cvs []models.ChannelValue
			require.NoError(t, json.Unmarshal([]byte(fv.Channels), &cvs))
			m[names[fv.FixtureID]] = cvs
		}
		out[l.Name] = m
	}
	return out
}

type cueView struct {
	Name     string
	Number   float64
	Look     string
	FadeIn   float64
	FadeOut  float64
	Follow   *float64
	HasNotes bool
}

func (d *testDeps) cueLists(t *testing.T, projectID string) (map[string]models.CueList, map[string][]cueView) {
	t.Helper()
	ctx := context.Background()
	looks, err := d.lookRepo.FindByProjectID(ctx, projectID)
	require.NoError(t, err)
	lookNames := map[string]string{}
	for _, l := range looks {
		lookNames[l.ID] = l.Name
	}
	lists, err := d.cueListRepo.FindByProjectID(ctx, projectID)
	require.NoError(t, err)
	byName := map[string]models.CueList{}
	cues := map[string][]cueView{}
	for _, cl := range lists {
		byName[cl.Name] = cl
		cs, err := d.cueRepo.FindByCueListID(ctx, cl.ID)
		require.NoError(t, err)
		sort.Slice(cs, func(i, j int) bool { return cs[i].CueNumber < cs[j].CueNumber })
		for _, c := range cs {
			cues[cl.Name] = append(cues[cl.Name], cueView{
				Name: c.Name, Number: c.CueNumber, Look: lookNames[c.LookID],
				FadeIn: c.FadeInTime, FadeOut: c.FadeOutTime, Follow: c.FollowTime,
			})
		}
	}
	return byName, cues
}

func fptr(f float64) *float64 { return &f }

func containsWarning(warnings []string, substrs ...string) bool {
	for _, w := range warnings {
		ok := true
		for _, s := range substrs {
			if !strings.Contains(w, s) {
				ok = false
				break
			}
		}
		if ok {
			return true
		}
	}
	return false
}

// TestImport_Issue91Workspace is the regression test for
// bbernstein/lacylights-go#91: importing a QLC+ workspace used to fail with
// "QLC+ import not available on this platform".
func TestImport_Issue91Workspace(t *testing.T) {
	d := newTestDeps(t)
	ctx := context.Background()
	library := d.seedLibraryDefinition(t, "Chauvet DJ", "SlimPAR Pro Q USB", map[string][]string{
		"4 Channel": {"RED", "GREEN", "BLUE", "AMBER"},
	})

	res, err := d.svc.Import(ctx, readTestdata(t, "issue91_workspace.qxw"), Options{ProjectName: "issue91_workspace"})
	require.NoError(t, err)
	require.NotEmpty(t, res.ProjectID)

	project, err := d.projectRepo.FindByID(ctx, res.ProjectID)
	require.NoError(t, err)
	require.NotNil(t, project)
	assert.Equal(t, "issue91_workspace", project.Name)

	assert.Equal(t, 4, res.FixtureCount)
	assert.Equal(t, 2, res.LookCount)
	assert.Equal(t, 1, res.CueListCount)
	assert.Equal(t, 4, res.CueCount)
	assert.Equal(t, 2, res.FixtureDefinitionsCreated, "ETC ColorSource PAR and the 5-channel SlimPAR mode are synthesized")

	fx := d.fixturesByName(t, res.ProjectID)
	require.Len(t, fx, 4)

	// QLC+ universes/addresses are 0-based; LacyLights' are 1-based.
	par4 := fx["par-4chan"]
	assert.Equal(t, 1, par4.inst.Universe)
	assert.Equal(t, 1, par4.inst.StartChannel)
	assert.Equal(t, library.ID, par4.inst.DefinitionID, "matched to the library definition despite the manufacturer alias")
	require.NotNil(t, par4.inst.ModeName)
	assert.Equal(t, "4 Channel", *par4.inst.ModeName)
	assert.Equal(t, []string{"RED", "GREEN", "BLUE", "AMBER"}, channelTypes(par4.channels))

	u2 := fx["par-u2-4chan"]
	assert.Equal(t, 2, u2.inst.Universe)
	assert.Equal(t, 1, u2.inst.StartChannel)
	assert.Equal(t, library.ID, u2.inst.DefinitionID)

	par5 := fx["par-5chan"]
	assert.Equal(t, 5, par5.inst.StartChannel)
	assert.NotEqual(t, library.ID, par5.inst.DefinitionID, "library has no 5-channel mode")
	assert.False(t, par5.def.IsBuiltIn)
	assert.Equal(t, "Chauvet", par5.def.Manufacturer)
	assert.Equal(t, "SlimPAR Pro Q USB", par5.def.Model)
	require.Len(t, par5.channels, 5)
	require.NotNil(t, par5.inst.ChannelCount)
	assert.Equal(t, 5, *par5.inst.ChannelCount)

	etc := fx["etc-par-3chan"]
	assert.Equal(t, 10, etc.inst.StartChannel)
	assert.Equal(t, "ETC", etc.def.Manufacturer)
	assert.Equal(t, "ColorSource PAR", etc.def.Model)
	assert.Equal(t, []string{"RED", "GREEN", "BLUE"}, channelTypes(etc.channels), "colour channels inferred from the mode name")

	looks := d.lookValues(t, res.ProjectID)
	require.Len(t, looks, 2)
	assert.Equal(t, []models.ChannelValue{{Offset: 0, Value: 232}, {Offset: 1, Value: 237}, {Offset: 2, Value: 229}},
		looks["first scene"]["etc-par-3chan"])
	assert.Equal(t, []models.ChannelValue{{Offset: 0, Value: 200}, {Offset: 1, Value: 200}, {Offset: 2, Value: 200}, {Offset: 3, Value: 200}, {Offset: 4, Value: 200}},
		looks["first scene"]["par-5chan"])
	assert.Len(t, looks["blackout"], 4)

	lists, cues := d.cueLists(t, res.ProjectID)
	require.Contains(t, lists, "Full Show Cuelist")
	assert.True(t, lists["Full Show Cuelist"].Loop)
	assert.Equal(t, []cueView{
		{Name: "first scene", Number: 1, Look: "first scene", FadeIn: 1, FadeOut: 0.5, Follow: fptr(1)},
		{Name: "blackout", Number: 2, Look: "blackout", FadeIn: 1, FadeOut: 0.5, Follow: fptr(1)},
		{Name: "first scene", Number: 3, Look: "first scene", FadeIn: 2.5, FadeOut: 0.5},
		{Name: "Final blackout", Number: 4, Look: "blackout", FadeIn: 1, FadeOut: 0.5},
	}, cues["Full Show Cuelist"])

	assert.True(t, containsWarning(res.Warnings, "ColorSource PAR"), "synthesized definitions are reported: %v", res.Warnings)
	assert.True(t, containsWarning(res.Warnings, "Chauvet DJ"), "alias matches are reported: %v", res.Warnings)
}

func TestImport_ReimportReusesSynthesizedDefinitions(t *testing.T) {
	d := newTestDeps(t)
	ctx := context.Background()
	content := readTestdata(t, "issue91_workspace.qxw")

	first, err := d.svc.Import(ctx, content, Options{ProjectName: "one"})
	require.NoError(t, err)
	assert.Equal(t, 3, first.FixtureDefinitionsCreated, "4ch SlimPAR, 5ch SlimPAR, ColorSource PAR")
	before := d.countDefinitions(t)

	second, err := d.svc.Import(ctx, content, Options{ProjectName: "two"})
	require.NoError(t, err)
	assert.Equal(t, 0, second.FixtureDefinitionsCreated)
	assert.Equal(t, before, d.countDefinitions(t))
	assert.NotEqual(t, first.ProjectID, second.ProjectID)

	a := d.fixturesByName(t, first.ProjectID)
	b := d.fixturesByName(t, second.ProjectID)
	for name, fa := range a {
		assert.Equal(t, fa.inst.DefinitionID, b[name].inst.DefinitionID, name)
	}
	// Both SlimPAR modes live under distinct definitions.
	assert.NotEqual(t, a["par-4chan"].inst.DefinitionID, a["par-5chan"].inst.DefinitionID)
}

func TestImport_FeatureCoverage(t *testing.T) {
	d := newTestDeps(t)
	ctx := context.Background()

	res, err := d.svc.Import(ctx, readTestdata(t, "feature_coverage.qxw"), Options{ProjectName: "features"})
	require.NoError(t, err)

	assert.Equal(t, 3, res.FixtureCount)
	assert.Equal(t, 5, res.LookCount, "3 scenes + 2 sequence steps")
	assert.Equal(t, 3, res.CueListCount)

	fx := d.fixturesByName(t, res.ProjectID)

	dimmers := fx["Dimmers 1-4"]
	assert.Equal(t, 101, dimmers.inst.StartChannel)
	assert.Equal(t, "DIMMER", dimmers.def.Type)
	assert.Equal(t, "Generic", dimmers.def.Manufacturer)
	assert.Equal(t, "Dimmer (4 ch)", dimmers.def.Model)
	assert.Equal(t, []string{"INTENSITY", "INTENSITY", "INTENSITY", "INTENSITY"}, channelTypes(dimmers.channels))
	assert.Equal(t, "FADE", dimmers.channels[0].FadeBehavior)
	assert.Equal(t, "SNAP", dimmers.channels[3].FadeBehavior, "QLC+ ExcludeFade channels snap")

	panel := fx["Pixel Strip"]
	assert.Equal(t, []string{"GREEN", "RED", "BLUE", "GREEN", "RED", "BLUE"}, channelTypes(panel.channels))
	assert.Equal(t, "Green 1", panel.channels[0].Name)
	assert.Equal(t, "RGB Panel (GRB, 6 ch)", panel.def.Model)
	assert.Equal(t, "Blue 2", panel.channels[5].Name)

	mystery := fx["Mystery"]
	assert.Equal(t, []string{"OTHER", "OTHER"}, channelTypes(mystery.channels))

	looks := d.lookValues(t, res.ProjectID)
	assert.Equal(t, []models.ChannelValue{{Offset: 0, Value: 255}, {Offset: 1, Value: 128}}, looks["Warm"]["Dimmers 1-4"],
		"out-of-range channel 7 is dropped")
	assert.Equal(t, []models.ChannelValue{{Offset: 0, Value: 10}, {Offset: 1, Value: 20}, {Offset: 2, Value: 255}}, looks["Warm"]["Pixel Strip"],
		"values are clamped to 0-255")
	assert.NotContains(t, looks["Warm"], "Mystery", "fixtures with no channel values are not added to the look")
	assert.Equal(t, []models.ChannelValue{{Offset: 2, Value: 64}}, looks["Legacy Values"]["Dimmers 1-4"])
	assert.Equal(t, []models.ChannelValue{{Offset: 1, Value: 200}}, looks["Legacy Values"]["Mystery"])

	// Sequence steps start from the bound scene's values.
	assert.Equal(t, []models.ChannelValue{{Offset: 0, Value: 255}, {Offset: 1, Value: 0}}, looks["Dimmer Chase - Step 1"]["Dimmers 1-4"])
	assert.Equal(t, []models.ChannelValue{{Offset: 0, Value: 0}, {Offset: 1, Value: 255}}, looks["Dimmer Chase - Step 2"]["Dimmers 1-4"])

	lists, cues := d.cueLists(t, res.ProjectID)

	assert.False(t, lists["Dimmer Chase"].Loop)
	assert.Equal(t, []cueView{
		{Name: "Dimmer Chase - Step 1", Number: 1, Look: "Dimmer Chase - Step 1", Follow: fptr(0.5)},
		{Name: "Dimmer Chase - Step 2", Number: 2, Look: "Dimmer Chase - Step 2", Follow: fptr(0.5)},
	}, cues["Dimmer Chase"])

	// Common speeds come from the chaser; Backward reverses the steps; the
	// step pointing at an EFX is skipped.
	assert.True(t, lists["Common Speeds"].Loop)
	assert.Equal(t, []cueView{
		{Name: "Legacy Values", Number: 1, Look: "Legacy Values", FadeIn: 3, FadeOut: 1.5, Follow: fptr(2)},
		{Name: "Warm", Number: 2, Look: "Warm", FadeIn: 3, FadeOut: 1.5, Follow: fptr(2)},
	}, cues["Common Speeds"])

	// Default fade speeds come from the referenced scene; legacy Duration
	// becomes the hold.
	assert.Equal(t, []cueView{
		{Name: "Legacy Values", Number: 1, Look: "Legacy Values", FadeIn: 2, Follow: fptr(4)},
		{Name: "Warm", Number: 2, Look: "Warm"},
	}, cues["Legacy Durations"])

	w := res.Warnings
	assert.True(t, containsWarning(w, "EFX", "Circle"), "%v", w)
	assert.True(t, containsWarning(w, "Collection", "Combo"), "%v", w)
	assert.True(t, containsWarning(w, "Warm", "77"), "unknown fixture reference: %v", w)
	assert.True(t, containsWarning(w, "Warm", "channel 7"), "out-of-range channel: %v", w)
	assert.True(t, containsWarning(w, "Common Speeds", "PingPong"), "%v", w)
	assert.True(t, containsWarning(w, "Common Speeds", "Circle"), "skipped step: %v", w)
	assert.True(t, containsWarning(w, "Mystery Spot"), "synthesized generic definition: %v", w)
	assert.False(t, containsWarning(w, "Generic"), "QLC+ generic dimmers/panels are exact, not guesses: %v", w)
}

func TestImport_LibraryMatchPrefersExactNameAndMode(t *testing.T) {
	d := newTestDeps(t)
	ctx := context.Background()
	lib := d.seedLibraryDefinition(t, "ETC", "ColorSource PAR", map[string][]string{
		"3-channel (RGB)":   {"RED", "GREEN", "BLUE"},
		"3-channel (Other)": {"INTENSITY", "STROBE", "OTHER"},
		"5-channel":         {"INTENSITY", "RED", "GREEN", "BLUE", "STROBE"},
	})
	xml := `<Workspace><Engine>
  <Fixture><Manufacturer>etc</Manufacturer><Model>colorsource  par</Model><Mode>3-channel (RGB)</Mode>
   <ID>0</ID><Name>a</Name><Universe>0</Universe><Address>0</Address><Channels>3</Channels></Fixture>
  <Fixture><Manufacturer>ETC</Manufacturer><Model>ColorSource PAR</Model><Mode>Unknown name</Mode>
   <ID>1</ID><Name>b</Name><Universe>0</Universe><Address>3</Address><Channels>5</Channels></Fixture>
 </Engine></Workspace>`
	res, err := d.svc.Import(ctx, xml, Options{ProjectName: "match"})
	require.NoError(t, err)
	assert.Equal(t, 0, res.FixtureDefinitionsCreated)

	fx := d.fixturesByName(t, res.ProjectID)
	assert.Equal(t, lib.ID, fx["a"].inst.DefinitionID, "case/whitespace-insensitive match")
	assert.Equal(t, "3-channel (RGB)", *fx["a"].inst.ModeName, "mode chosen by name among same-size modes")
	assert.Equal(t, []string{"RED", "GREEN", "BLUE"}, channelTypes(fx["a"].channels))
	assert.Equal(t, lib.ID, fx["b"].inst.DefinitionID)
	assert.Equal(t, "5-channel", *fx["b"].inst.ModeName, "falls back to the mode with the right channel count")
}

func TestImport_AmbiguousManufacturerAliasIsNotGuessed(t *testing.T) {
	d := newTestDeps(t)
	ctx := context.Background()
	d.seedLibraryDefinition(t, "Chauvet DJ", "SlimPAR", map[string][]string{"3ch": {"RED", "GREEN", "BLUE"}})
	d.seedLibraryDefinition(t, "Chauvet Professional", "SlimPAR", map[string][]string{"3ch": {"RED", "GREEN", "BLUE"}})
	xml := `<Workspace><Engine>
  <Fixture><Manufacturer>Chauvet</Manufacturer><Model>SlimPAR</Model><Mode>3ch</Mode>
   <ID>0</ID><Name>a</Name><Universe>0</Universe><Address>0</Address><Channels>3</Channels></Fixture>
 </Engine></Workspace>`
	res, err := d.svc.Import(ctx, xml, Options{ProjectName: "ambiguous"})
	require.NoError(t, err)
	assert.Equal(t, 1, res.FixtureDefinitionsCreated)
	fx := d.fixturesByName(t, res.ProjectID)
	assert.Equal(t, "Chauvet", fx["a"].def.Manufacturer)
	assert.False(t, fx["a"].def.IsBuiltIn)
}

func TestImport_SetsGroupAndDescription(t *testing.T) {
	d := newTestDeps(t)
	ctx := context.Background()
	gid := "group-1"
	res, err := d.svc.Import(ctx, readTestdata(t, "issue91_workspace.qxw"), Options{
		ProjectName: "grouped", GroupID: &gid, Description: "Imported from QLC+ workspace show.qxw",
	})
	require.NoError(t, err)
	p, err := d.projectRepo.FindByID(ctx, res.ProjectID)
	require.NoError(t, err)
	require.NotNil(t, p.GroupID)
	assert.Equal(t, gid, *p.GroupID)
	require.NotNil(t, p.Description)
	assert.Equal(t, "Imported from QLC+ workspace show.qxw", *p.Description)
}

func TestImport_InvalidContentCreatesNothing(t *testing.T) {
	d := newTestDeps(t)
	ctx := context.Background()
	_, err := d.svc.Import(ctx, "<NotAWorkspace/>", Options{ProjectName: "bad"})
	require.Error(t, err)
	projects, err := d.projectRepo.FindAll(ctx)
	require.NoError(t, err)
	assert.Empty(t, projects)
}

func TestImport_EmptyWorkspace(t *testing.T) {
	d := newTestDeps(t)
	res, err := d.svc.Import(context.Background(), "<Workspace><Engine/></Workspace>", Options{ProjectName: ""})
	require.NoError(t, err)
	p, err := d.projectRepo.FindByID(context.Background(), res.ProjectID)
	require.NoError(t, err)
	assert.Equal(t, DefaultProjectName, p.Name)
	assert.True(t, containsWarning(res.Warnings, "no fixtures"), "%v", res.Warnings)
}

func TestImport_DuplicateAndInvalidFixtures(t *testing.T) {
	d := newTestDeps(t)
	xml := `<Workspace><Engine>
  <Fixture><Manufacturer>Generic</Manufacturer><Model>Generic</Model><Mode>Generic</Mode>
   <ID>0</ID><Name>first</Name><Universe>0</Universe><Address>0</Address><Channels>1</Channels></Fixture>
  <Fixture><Manufacturer>Generic</Manufacturer><Model>Generic</Model><Mode>Generic</Mode>
   <ID>0</ID><Name>dup</Name><Universe>0</Universe><Address>1</Address><Channels>1</Channels></Fixture>
  <Fixture><Manufacturer>Generic</Manufacturer><Model>Generic</Model><Mode>Generic</Mode>
   <ID>2</ID><Name>zero</Name><Universe>0</Universe><Address>2</Address><Channels>0</Channels></Fixture>
 </Engine></Workspace>`
	res, err := d.svc.Import(context.Background(), xml, Options{ProjectName: "dups"})
	require.NoError(t, err)
	assert.Equal(t, 2, res.FixtureCount, "duplicate ID skipped")
	assert.True(t, containsWarning(res.Warnings, "dup", "duplicate"), "%v", res.Warnings)
	fx := d.fixturesByName(t, res.ProjectID)
	require.Contains(t, fx, "zero")
	assert.Len(t, fx["zero"].channels, 1, "QLC+ treats channels <= 0 as 1")
}

func TestImport_RequiresDependencies(t *testing.T) {
	_, err := NewService(nil, nil, nil, nil, nil).Import(context.Background(), "<Workspace/>", Options{})
	require.Error(t, err)
}

func TestProjectNameFromFileName(t *testing.T) {
	tests := map[string]string{
		"show.qxw":               "show",
		"My Show.QXW":            "My Show",
		"/Users/x/Desktop/a.qxw": "a",
		`C:\shows\b.qxw`:         "b",
		"  spaced name .qxw  ":   "spaced name",
		"no-extension":           "no-extension",
		"":                       DefaultProjectName,
		".qxw":                   DefaultProjectName,
		"archive.tar.qxw":        "archive.tar",
		"workspace.xml":          "workspace.xml",
	}
	for in, want := range tests {
		assert.Equal(t, want, ProjectNameFromFileName(in), "input %q", in)
	}
}

func TestImport_GenericDimmersOfDifferentSizes(t *testing.T) {
	d := newTestDeps(t)
	xml := `<Workspace><Engine>
  <Fixture><Manufacturer>Generic</Manufacturer><Model>Generic</Model><Mode>Generic</Mode>
   <ID>0</ID><Name>six</Name><Universe>0</Universe><Address>0</Address><Channels>6</Channels></Fixture>
  <Fixture><Manufacturer>Generic</Manufacturer><Model>Generic</Model><Mode>Generic</Mode>
   <ID>1</ID><Name>four</Name><Universe>0</Universe><Address>6</Address><Channels>4</Channels></Fixture>
  <Fixture><Manufacturer>Generic</Manufacturer><Model>Generic</Model><Mode>Generic</Mode>
   <ID>2</ID><Name>six again</Name><Universe>0</Universe><Address>10</Address><Channels>6</Channels></Fixture>
 </Engine></Workspace>`
	res, err := d.svc.Import(context.Background(), xml, Options{ProjectName: "dimmers"})
	require.NoError(t, err)
	assert.Equal(t, 2, res.FixtureDefinitionsCreated)
	fx := d.fixturesByName(t, res.ProjectID)
	assert.Equal(t, "Dimmer (6 ch)", fx["six"].def.Model)
	assert.Equal(t, "Dimmer (4 ch)", fx["four"].def.Model)
	assert.Equal(t, fx["six"].inst.DefinitionID, fx["six again"].inst.DefinitionID)
}
