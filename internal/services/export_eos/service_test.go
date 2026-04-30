package exporteos

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/bbernstein/lacylights-go/internal/database/models"
	"github.com/bbernstein/lacylights-go/internal/graphql/generated"
	importeos "github.com/bbernstein/lacylights-go/internal/services/import_eos"
)

// buildSampleProject creates a minimal project with a single 4-channel RGBW
// fixture, one cue list, and two cues. Returns the project ID.
func buildSampleProject(t *testing.T, deps *testDeps) string {
	t.Helper()
	ctx := context.Background()

	proj := &models.Project{Name: "Sample Show"}
	if err := deps.projectRepo.Create(ctx, proj); err != nil {
		t.Fatalf("create project: %v", err)
	}

	def := &models.FixtureDefinition{Manufacturer: "Generic", Model: "RGBW", Type: "LED", IsBuiltIn: false}
	channels := []models.ChannelDefinition{
		{Offset: 0, Name: "Intensity", Type: string(generated.ChannelTypeIntensity), DefaultValue: 0, FadeBehavior: "FADE"},
		{Offset: 1, Name: "Red", Type: string(generated.ChannelTypeRed), DefaultValue: 0, FadeBehavior: "FADE"},
		{Offset: 2, Name: "Green", Type: string(generated.ChannelTypeGreen), DefaultValue: 0, FadeBehavior: "FADE"},
		{Offset: 3, Name: "Blue", Type: string(generated.ChannelTypeBlue), DefaultValue: 0, FadeBehavior: "FADE"},
	}
	if err := deps.fixtureRepo.CreateDefinitionWithChannels(ctx, def, channels); err != nil {
		t.Fatalf("create def: %v", err)
	}

	// Two fixtures.
	for i, name := range []string{"Front Wash", "Back Wash"} {
		order := i + 1
		fi := &models.FixtureInstance{
			ProjectID:    proj.ID,
			DefinitionID: def.ID,
			Name:         name,
			Universe:     1,
			StartChannel: 1 + i*4,
			ProjectOrder: &order,
		}
		instChans := make([]models.InstanceChannel, len(channels))
		for j, ch := range channels {
			instChans[j] = models.InstanceChannel{
				Offset: ch.Offset, Name: ch.Name, Type: ch.Type,
				DefaultValue: ch.DefaultValue, FadeBehavior: ch.FadeBehavior,
				MinValue: 0, MaxValue: 255,
			}
		}
		if err := deps.fixtureRepo.CreateWithChannels(ctx, fi, instChans); err != nil {
			t.Fatalf("create fixture: %v", err)
		}
	}

	// Reload instances by project to get assigned IDs.
	instances, err := deps.fixtureRepo.FindByProjectID(ctx, proj.ID)
	if err != nil {
		t.Fatalf("find instances: %v", err)
	}

	cueList := &models.CueList{ProjectID: proj.ID, Name: "Main"}
	if err := deps.cueListRepo.Create(ctx, cueList); err != nil {
		t.Fatalf("create cue list: %v", err)
	}

	// Cue 1: full intensity on instance 1 only.
	look1 := &models.Look{ProjectID: proj.ID, Name: "Cue 1"}
	fv1ch := mustMarshal(t, []models.ChannelValue{{Offset: 0, Value: 255}, {Offset: 1, Value: 200}})
	fv1 := []models.FixtureValue{{FixtureID: instances[0].ID, Channels: fv1ch}}
	if err := deps.lookRepo.CreateWithFixtureValues(ctx, look1, fv1); err != nil {
		t.Fatalf("create look 1: %v", err)
	}
	cue1 := &models.Cue{CueListID: cueList.ID, LookID: look1.ID, Name: "Cue 1 - Open", CueNumber: 1, FadeInTime: 5, FadeOutTime: 5}
	if err := deps.cueRepo.Create(ctx, cue1); err != nil {
		t.Fatalf("create cue 1: %v", err)
	}

	// Cue 2: full intensity on both, blue on second.
	look2 := &models.Look{ProjectID: proj.ID, Name: "Cue 2"}
	fv2 := []models.FixtureValue{
		{FixtureID: instances[0].ID, Channels: fv1ch},
		{FixtureID: instances[1].ID, Channels: mustMarshal(t, []models.ChannelValue{{Offset: 0, Value: 255}, {Offset: 3, Value: 255}})},
	}
	if err := deps.lookRepo.CreateWithFixtureValues(ctx, look2, fv2); err != nil {
		t.Fatalf("create look 2: %v", err)
	}
	cue2 := &models.Cue{CueListID: cueList.ID, LookID: look2.ID, Name: "Cue 2", CueNumber: 2, FadeInTime: 3}
	if err := deps.cueRepo.Create(ctx, cue2); err != nil {
		t.Fatalf("create cue 2: %v", err)
	}

	return proj.ID
}

func mustMarshal(t *testing.T, v any) string {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	return string(b)
}

func TestExport_Deterministic(t *testing.T) {
	deps := newTestDeps(t)
	defer deps.close()
	pid := buildSampleProject(t, deps)
	svc := NewServiceWithDeps(deps.projectRepo, deps.fixtureRepo, deps.fixtureGroupRepo, deps.lookRepo,
		deps.cueListRepo, deps.cueRepo, deps.lookBoardRepo)

	res1, err := svc.Export(context.Background(), pid)
	if err != nil {
		t.Fatalf("export 1: %v", err)
	}
	res2, err := svc.Export(context.Background(), pid)
	if err != nil {
		t.Fatalf("export 2: %v", err)
	}
	if res1.Content != res2.Content {
		t.Errorf("non-deterministic output:\n--- 1 ---\n%s\n--- 2 ---\n%s", res1.Content, res2.Content)
	}
}

func TestExport_StructureBasics(t *testing.T) {
	deps := newTestDeps(t)
	defer deps.close()
	pid := buildSampleProject(t, deps)
	svc := NewServiceWithDeps(deps.projectRepo, deps.fixtureRepo, deps.fixtureGroupRepo, deps.lookRepo,
		deps.cueListRepo, deps.cueRepo, deps.lookBoardRepo)

	res, err := svc.Export(context.Background(), pid)
	if err != nil {
		t.Fatalf("export: %v", err)
	}
	got := res.Content
	if res.FilenameSuffix != ".asc" {
		t.Errorf("FilenameSuffix = %q, want .asc", res.FilenameSuffix)
	}
	if res.ProjectName != "Sample Show" {
		t.Errorf("ProjectName = %q, want Sample Show", res.ProjectName)
	}
	for _, want := range []string{
		"Ident 3:0\n",
		"Manufacturer ETC\n",
		"Console Eos\n",
		"$$Title Sample Show\n",
		"$ParamType 1 1 Intens\n",
		"$ParamType 12 3 Red\n",
		"$Personality 90001\n",
		"   $$Manuf Generic\n",
		"   $$Model RGBW\n",
		"$Patch 1 1 90001\n",
		"   Text Front Wash\n",
		"$Patch 2 5 90001\n", // EOS channel 2, address 5 (universe 1, start 5)
		"$CueList 1\n",
		"   Text Main\n",
		"Cue 1 1\n",
		"   Text Open\n", // synthetic prefix is stripped
		"   Up 5\n",
		"   $$ChanMove 1@HFF\n",
		"$$ LACYLIGHTS:version 1\n",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in:\n%s", want, got)
		}
	}
}

// TestExport_EmitsWarningForUnpatchedFixture confirms the writer surfaces an
// UNPATCHED_INSTANCE warning for any fixture with StartChannel <= 0 instead
// of silently dropping it.
func TestExport_EmitsWarningForUnpatchedFixture(t *testing.T) {
	deps := newTestDeps(t)
	defer deps.close()
	ctx := context.Background()

	proj := &models.Project{Name: "unpatched-test"}
	if err := deps.projectRepo.Create(ctx, proj); err != nil {
		t.Fatalf("create project: %v", err)
	}
	def := &models.FixtureDefinition{Manufacturer: "Generic", Model: "Dimmer", Type: "DIMMER"}
	chs := []models.ChannelDefinition{{Offset: 0, Name: "Intensity", Type: string(generated.ChannelTypeIntensity)}}
	if err := deps.fixtureRepo.CreateDefinitionWithChannels(ctx, def, chs); err != nil {
		t.Fatalf("create def: %v", err)
	}
	// One patched, one unpatched (StartChannel = 0).
	patched := &models.FixtureInstance{ProjectID: proj.ID, DefinitionID: def.ID, Name: "Patched", Universe: 1, StartChannel: 1}
	unpatched := &models.FixtureInstance{ProjectID: proj.ID, DefinitionID: def.ID, Name: "Unpatched", Universe: 1, StartChannel: 0}
	for _, fi := range []*models.FixtureInstance{patched, unpatched} {
		if err := deps.fixtureRepo.CreateWithChannels(ctx, fi, []models.InstanceChannel{{Offset: 0, Name: "Intensity", Type: string(generated.ChannelTypeIntensity)}}); err != nil {
			t.Fatalf("create instance %s: %v", fi.Name, err)
		}
	}

	svc := NewServiceWithDeps(deps.projectRepo, deps.fixtureRepo, deps.fixtureGroupRepo, deps.lookRepo,
		deps.cueListRepo, deps.cueRepo, deps.lookBoardRepo)
	res, err := svc.Export(ctx, proj.ID)
	if err != nil {
		t.Fatalf("export: %v", err)
	}

	var sawWarning bool
	for _, w := range res.Warnings {
		if w.Code == importeos.WarnUnpatchedInstance {
			sawWarning = true
			break
		}
	}
	if !sawWarning {
		t.Errorf("expected UNPATCHED_INSTANCE warning, got %+v", res.Warnings)
	}
	if strings.Contains(res.Content, "$Patch 2 0") {
		t.Errorf("export emitted Patch line for unpatched fixture; should have skipped")
	}
}

// TestExport_SanitizesNewlinesInLabels confirms that user-supplied strings
// (project name, fixture label) cannot inject phantom EOS directives via
// embedded CR/LF/tab characters.
func TestExport_SanitizesNewlinesInLabels(t *testing.T) {
	deps := newTestDeps(t)
	defer deps.close()
	ctx := context.Background()

	proj := &models.Project{Name: "Title\nClear All"}
	if err := deps.projectRepo.Create(ctx, proj); err != nil {
		t.Fatalf("create project: %v", err)
	}
	def := &models.FixtureDefinition{Manufacturer: "Generic", Model: "Dimmer", Type: "DIMMER"}
	chs := []models.ChannelDefinition{{Offset: 0, Name: "Intensity", Type: string(generated.ChannelTypeIntensity)}}
	if err := deps.fixtureRepo.CreateDefinitionWithChannels(ctx, def, chs); err != nil {
		t.Fatalf("create def: %v", err)
	}
	order := 1
	fi := &models.FixtureInstance{
		ProjectID: proj.ID, DefinitionID: def.ID,
		Name:         "Front Wash\n$Patch 999 1 90001",
		Universe:     1, StartChannel: 1, ProjectOrder: &order,
	}
	if err := deps.fixtureRepo.CreateWithChannels(ctx, fi, []models.InstanceChannel{{Offset: 0, Name: "Intensity", Type: string(generated.ChannelTypeIntensity)}}); err != nil {
		t.Fatalf("create instance: %v", err)
	}

	svc := NewServiceWithDeps(deps.projectRepo, deps.fixtureRepo, deps.fixtureGroupRepo, deps.lookRepo,
		deps.cueListRepo, deps.cueRepo, deps.lookBoardRepo)
	res, err := svc.Export(ctx, proj.ID)
	if err != nil {
		t.Fatalf("export: %v", err)
	}
	// $$Title is the only line containing "Clear All" prefix when the
	// project name carries a newline. After sanitization the title line
	// reads "$$Title Title Clear All" on a single line.
	if !strings.Contains(res.Content, "$$Title Title Clear All\n") {
		t.Errorf("project name newline not sanitized; got:\n%s", res.Content)
	}
	// The fixture label embedded a fake "$Patch 999" directive after a
	// newline. After sanitization that should appear within a Text label
	// (indented), never as its own top-level directive line.
	if strings.Contains(res.Content, "\n$Patch 999") {
		t.Errorf("fixture label injected phantom $Patch directive:\n%s", res.Content)
	}
	// And the label should appear on the same line as "Text", not on a
	// new directive line of its own.
	if !strings.Contains(res.Content, "   Text Front Wash $Patch 999 1 90001\n") {
		t.Errorf("fixture label newline not sanitized; got:\n%s", res.Content)
	}
}

// TestExport_WarnsOnInvalidLookValuesJSON forces a corrupt FixtureValue
// row and asserts the export emits LOOK_VALUES_INVALID rather than
// silently dropping the channel data without feedback.
func TestExport_WarnsOnInvalidLookValuesJSON(t *testing.T) {
	deps := newTestDeps(t)
	defer deps.close()
	ctx := context.Background()

	proj := &models.Project{Name: "bad-json-test"}
	if err := deps.projectRepo.Create(ctx, proj); err != nil {
		t.Fatalf("create project: %v", err)
	}
	def := &models.FixtureDefinition{Manufacturer: "Generic", Model: "Dimmer", Type: "DIMMER"}
	chs := []models.ChannelDefinition{{Offset: 0, Name: "Intensity", Type: string(generated.ChannelTypeIntensity)}}
	if err := deps.fixtureRepo.CreateDefinitionWithChannels(ctx, def, chs); err != nil {
		t.Fatalf("create def: %v", err)
	}
	order := 1
	fi := &models.FixtureInstance{ProjectID: proj.ID, DefinitionID: def.ID, Name: "F1", Universe: 1, StartChannel: 1, ProjectOrder: &order}
	if err := deps.fixtureRepo.CreateWithChannels(ctx, fi, []models.InstanceChannel{{Offset: 0, Name: "Intensity", Type: string(generated.ChannelTypeIntensity)}}); err != nil {
		t.Fatalf("create instance: %v", err)
	}
	cl := &models.CueList{ProjectID: proj.ID, Name: "Main"}
	if err := deps.cueListRepo.Create(ctx, cl); err != nil {
		t.Fatalf("create cue list: %v", err)
	}
	look := &models.Look{ProjectID: proj.ID, Name: "L1"}
	if err := deps.lookRepo.CreateWithFixtureValues(ctx, look, []models.FixtureValue{
		{FixtureID: fi.ID, Channels: "not-valid-json"},
	}); err != nil {
		t.Fatalf("create look: %v", err)
	}
	cue := &models.Cue{CueListID: cl.ID, LookID: look.ID, Name: "Cue 1", CueNumber: 1}
	if err := deps.cueRepo.Create(ctx, cue); err != nil {
		t.Fatalf("create cue: %v", err)
	}

	svc := NewServiceWithDeps(deps.projectRepo, deps.fixtureRepo, deps.fixtureGroupRepo, deps.lookRepo,
		deps.cueListRepo, deps.cueRepo, deps.lookBoardRepo)
	res, err := svc.Export(ctx, proj.ID)
	if err != nil {
		t.Fatalf("export: %v (expected success with warning)", err)
	}
	var saw bool
	for _, w := range res.Warnings {
		if w.Code == importeos.WarnLookValuesInvalid {
			saw = true
			break
		}
	}
	if !saw {
		t.Errorf("expected LOOK_VALUES_INVALID warning, got %+v", res.Warnings)
	}
}

// TestExport_DottedAddressForMultiUniverse confirms formatEOSAddress emits
// the dotted form for non-unit universes.
func TestExport_DottedAddressForMultiUniverse(t *testing.T) {
	deps := newTestDeps(t)
	defer deps.close()
	ctx := context.Background()

	proj := &models.Project{Name: "Multi-Universe Show"}
	if err := deps.projectRepo.Create(ctx, proj); err != nil {
		t.Fatalf("create project: %v", err)
	}
	def := &models.FixtureDefinition{Manufacturer: "Generic", Model: "Dimmer", Type: "DIMMER"}
	chs := []models.ChannelDefinition{{Offset: 0, Name: "Intensity", Type: string(generated.ChannelTypeIntensity)}}
	if err := deps.fixtureRepo.CreateDefinitionWithChannels(ctx, def, chs); err != nil {
		t.Fatalf("create def: %v", err)
	}
	order := 1
	fi := &models.FixtureInstance{
		ProjectID: proj.ID, DefinitionID: def.ID, Name: "U2", Universe: 2, StartChannel: 100,
		ProjectOrder: &order,
	}
	if err := deps.fixtureRepo.CreateWithChannels(ctx, fi, []models.InstanceChannel{{Offset: 0, Name: "Intensity", Type: string(generated.ChannelTypeIntensity)}}); err != nil {
		t.Fatalf("create instance: %v", err)
	}

	svc := NewServiceWithDeps(deps.projectRepo, deps.fixtureRepo, deps.fixtureGroupRepo, deps.lookRepo,
		deps.cueListRepo, deps.cueRepo, deps.lookBoardRepo)
	res, err := svc.Export(ctx, proj.ID)
	if err != nil {
		t.Fatalf("export: %v", err)
	}
	if !strings.Contains(res.Content, "$Patch 1 2.100 90001") {
		t.Errorf("expected dotted address '2.100' in patch line, got:\n%s", res.Content)
	}
}

// TestExport_FailsWhenProjectMissing locks in the contract that a request
// for a project ID that doesn't exist returns a structured "not found"
// error rather than panicking on the nil project pointer.
func TestExport_FailsWhenProjectMissing(t *testing.T) {
	deps := newTestDeps(t)
	defer deps.close()
	svc := NewServiceWithDeps(deps.projectRepo, deps.fixtureRepo, deps.fixtureGroupRepo, deps.lookRepo,
		deps.cueListRepo, deps.cueRepo, deps.lookBoardRepo)
	_, err := svc.Export(context.Background(), "definitely-not-a-real-project")
	if err == nil {
		t.Fatal("expected error for non-existent project")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("error %q missing 'not found'", err.Error())
	}
}

func TestExport_FailsWhenRepositoriesMissing(t *testing.T) {
	cases := []struct {
		name    string
		mutate  func(d *testDeps, s *Service)
		wantSub string
	}{
		{name: "no_repos", mutate: func(_ *testDeps, _ *Service) {}, wantSub: "project repository"},
		{name: "only_project", mutate: func(d *testDeps, s *Service) {
			s.projectRepo = d.projectRepo
		}, wantSub: "fixture repository"},
		{name: "missing_fixture_group", mutate: func(d *testDeps, s *Service) {
			s.projectRepo = d.projectRepo
			s.fixtureRepo = d.fixtureRepo
		}, wantSub: "fixture group repository"},
		{name: "missing_look", mutate: func(d *testDeps, s *Service) {
			s.projectRepo = d.projectRepo
			s.fixtureRepo = d.fixtureRepo
			s.fixtureGroupRepo = d.fixtureGroupRepo
		}, wantSub: "look repository"},
		{name: "missing_cue_list", mutate: func(d *testDeps, s *Service) {
			s.projectRepo = d.projectRepo
			s.fixtureRepo = d.fixtureRepo
			s.fixtureGroupRepo = d.fixtureGroupRepo
			s.lookRepo = d.lookRepo
		}, wantSub: "cue list repository"},
		{name: "missing_cue", mutate: func(d *testDeps, s *Service) {
			s.projectRepo = d.projectRepo
			s.fixtureRepo = d.fixtureRepo
			s.fixtureGroupRepo = d.fixtureGroupRepo
			s.lookRepo = d.lookRepo
			s.cueListRepo = d.cueListRepo
		}, wantSub: "cue repository"},
		{name: "missing_look_board", mutate: func(d *testDeps, s *Service) {
			s.projectRepo = d.projectRepo
			s.fixtureRepo = d.fixtureRepo
			s.fixtureGroupRepo = d.fixtureGroupRepo
			s.lookRepo = d.lookRepo
			s.cueListRepo = d.cueListRepo
			s.cueRepo = d.cueRepo
		}, wantSub: "look board repository"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			deps := newTestDeps(t)
			defer deps.close()
			svc := &Service{}
			c.mutate(deps, svc)
			_, err := svc.Export(context.Background(), "x")
			if err == nil || !strings.Contains(err.Error(), c.wantSub) {
				t.Errorf("err = %v, want substring %q", err, c.wantSub)
			}
		})
	}
}

func TestExport_GroupsEmitted(t *testing.T) {
	deps := newTestDeps(t)
	defer deps.close()
	ctx := context.Background()

	proj := &models.Project{Name: "T"}
	if err := deps.projectRepo.Create(ctx, proj); err != nil {
		t.Fatalf("project: %v", err)
	}
	def := &models.FixtureDefinition{Manufacturer: "Generic", Model: "Dimmer", Type: "DIMMER"}
	if err := deps.fixtureRepo.CreateDefinition(ctx, def); err != nil {
		t.Fatalf("def: %v", err)
	}

	one, two := 1, 2
	a := &models.FixtureInstance{ProjectID: proj.ID, DefinitionID: def.ID, Name: "A", Universe: 1, StartChannel: 1, ProjectOrder: &one}
	b := &models.FixtureInstance{ProjectID: proj.ID, DefinitionID: def.ID, Name: "B", Universe: 1, StartChannel: 2, ProjectOrder: &two}
	if err := deps.fixtureRepo.CreateWithChannels(ctx, a, []models.InstanceChannel{{Offset: 0, Name: "I", Type: "INTENSITY", FadeBehavior: "FADE"}}); err != nil {
		t.Fatalf("a: %v", err)
	}
	if err := deps.fixtureRepo.CreateWithChannels(ctx, b, []models.InstanceChannel{{Offset: 0, Name: "I", Type: "INTENSITY", FadeBehavior: "FADE"}}); err != nil {
		t.Fatalf("b: %v", err)
	}

	g := &models.FixtureGroup{ProjectID: proj.ID, Name: "All"}
	if err := deps.fixtureGroupRepo.CreateWithMembers(ctx, g, []string{a.ID, b.ID}); err != nil {
		t.Fatalf("group: %v", err)
	}

	svc := NewServiceWithDeps(deps.projectRepo, deps.fixtureRepo, deps.fixtureGroupRepo, deps.lookRepo, deps.cueListRepo, deps.cueRepo, deps.lookBoardRepo)
	res, err := svc.Export(ctx, proj.ID)
	if err != nil {
		t.Fatalf("export: %v", err)
	}
	if !strings.Contains(res.Content, "$Group 1") {
		t.Errorf("missing $Group 1; output:\n%s", res.Content)
	}
	if !strings.Contains(res.Content, "Text All") {
		t.Errorf("missing Text All")
	}

	// EosNumber should now be persisted.
	persisted, _ := deps.fixtureGroupRepo.FindByID(ctx, g.ID)
	if persisted.EosNumber == nil || *persisted.EosNumber != 1 {
		t.Errorf("expected EosNumber=1 persisted; got %v", persisted.EosNumber)
	}
}

func TestExport_EmptyGroupSkipped(t *testing.T) {
	deps := newTestDeps(t)
	defer deps.close()
	ctx := context.Background()
	proj := &models.Project{Name: "T"}
	if err := deps.projectRepo.Create(ctx, proj); err != nil {
		t.Fatalf("project: %v", err)
	}

	g := &models.FixtureGroup{ProjectID: proj.ID, Name: "Empty"}
	if err := deps.fixtureGroupRepo.CreateWithMembers(ctx, g, nil); err != nil {
		t.Fatalf("group: %v", err)
	}

	svc := NewServiceWithDeps(deps.projectRepo, deps.fixtureRepo, deps.fixtureGroupRepo, deps.lookRepo, deps.cueListRepo, deps.cueRepo, deps.lookBoardRepo)
	res, err := svc.Export(ctx, proj.ID)
	if err != nil {
		t.Fatalf("export: %v", err)
	}
	if strings.Contains(res.Content, "$Group") {
		t.Errorf("empty group should not be emitted; got:\n%s", res.Content)
	}
	saw := false
	for _, w := range res.Warnings {
		if w.Code == importeos.WarnExportEmptyGroupSkipped {
			saw = true
		}
	}
	if !saw {
		t.Errorf("expected WarnExportEmptyGroupSkipped")
	}
}

func TestExport_SidecarFadeBehavior(t *testing.T) {
	deps := newTestDeps(t)
	defer deps.close()
	ctx := context.Background()

	proj := &models.Project{Name: "T"}
	if err := deps.projectRepo.Create(ctx, proj); err != nil {
		t.Fatalf("project: %v", err)
	}
	def := &models.FixtureDefinition{Manufacturer: "Generic", Model: "RGB", Type: "DIMMER"}
	if err := deps.fixtureRepo.CreateDefinition(ctx, def); err != nil {
		t.Fatalf("def: %v", err)
	}

	one := 1
	fi := &models.FixtureInstance{ProjectID: proj.ID, DefinitionID: def.ID, Name: "A", Universe: 1, StartChannel: 1, ProjectOrder: &one}
	channels := []models.InstanceChannel{
		{Offset: 0, Name: "I", Type: "INTENSITY", FadeBehavior: "FADE"},
		{Offset: 1, Name: "R", Type: "COLOR_R", FadeBehavior: "SNAP_END"},
	}
	if err := deps.fixtureRepo.CreateWithChannels(ctx, fi, channels); err != nil {
		t.Fatalf("fi: %v", err)
	}

	svc := NewServiceWithDeps(deps.projectRepo, deps.fixtureRepo, deps.fixtureGroupRepo, deps.lookRepo, deps.cueListRepo, deps.cueRepo, deps.lookBoardRepo)
	res, err := svc.Export(ctx, proj.ID)
	if err != nil {
		t.Fatalf("export: %v", err)
	}

	if !strings.Contains(res.Content, `"behavior":"SNAP_END"`) {
		t.Errorf("missing fade_behavior in output:\n%s", res.Content)
	}
	if !strings.Contains(res.Content, `"instanceRefId":"`+fi.RefID+`"`) {
		t.Errorf("instanceRefId not %s in output", fi.RefID)
	}
}
