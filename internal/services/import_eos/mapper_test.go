package importeos

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/bbernstein/lacylights-go/internal/database/models"
)

func TestMapper_MinimalPatchCreatesProjectAndFixtures(t *testing.T) {
	deps := newTestDeps(t)
	defer deps.close()

	src := strings.NewReader(`Ident 3:0
Manufacturer ETC
Console Eos
$$Format 3.20
$$Title Test Show

$ParamType 1 1 Intens

$Personality 9001
   $$Manuf Generic
   $$Model Dimmer
   $$Footprint 1
   $$PersChan 1 1 1 0 0

$Patch 1 1 9001
   Text Front
$Patch 2 2 9001
   Text Back
`)

	svc := NewServiceWithDeps(deps.projectRepo, deps.fixtureRepo, deps.fixtureGroupRepo, deps.lookRepo,
		deps.cueListRepo, deps.cueRepo, deps.lookBoardRepo)
	res, err := svc.Import(context.Background(), src, Options{NewProjectName: ptr("Test Show")})
	if err != nil {
		t.Fatalf("Import: %v", err)
	}
	if res.FixtureInstancesCount != 2 {
		t.Errorf("instances: got %d, want 2", res.FixtureInstancesCount)
	}
	if res.FixtureDefinitionsCount != 1 {
		t.Errorf("definitions: got %d, want 1", res.FixtureDefinitionsCount)
	}
	if res.ProjectID == "" {
		t.Errorf("expected project ID")
	}
}

func openFixtureFile(t *testing.T, name string) *os.File {
	t.Helper()
	f, err := os.Open(filepath.Join("testdata", name))
	if err != nil {
		t.Fatalf("open fixture %s: %v", name, err)
	}
	return f
}

func TestImport_TargetProjectNotFound(t *testing.T) {
	deps := newTestDeps(t)
	defer deps.close()
	svc := NewServiceWithDeps(deps.projectRepo, deps.fixtureRepo, deps.fixtureGroupRepo, deps.lookRepo,
		deps.cueListRepo, deps.cueRepo, deps.lookBoardRepo)
	missing := "definitely-not-a-real-project"
	_, err := svc.Import(context.Background(), strings.NewReader(`Ident 3:0
$$Format 3.20
$$Title T
$ParamType 1 1 Intens
$Personality 9001
   $$Manuf G
   $$Model D
   $$Footprint 1
   $$PersChan 1 1 1 0 0
$Patch 1 1 9001
`), Options{TargetProjectID: &missing})
	if err == nil {
		t.Fatal("expected error when target project does not exist")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("error %q missing 'not found'", err.Error())
	}
}

func TestImport_AddressConflictEmitsWarning(t *testing.T) {
	// EOS supports multi-patching (two logical channels at the same DMX
	// address); the importer surfaces that as a WARN-severity warning and
	// continues, rather than aborting the whole file with a hard error.
	deps := newTestDeps(t)
	defer deps.close()
	f := openFixtureFile(t, "conflict_addresses.asc")
	defer func() { _ = f.Close() }()
	svc := NewServiceWithDeps(deps.projectRepo, deps.fixtureRepo, deps.fixtureGroupRepo, deps.lookRepo,
		deps.cueListRepo, deps.cueRepo, deps.lookBoardRepo)
	res, err := svc.Import(context.Background(), f, Options{NewProjectName: ptr("X")})
	if err != nil {
		t.Fatalf("import: %v (expected success with warning)", err)
	}
	var sawConflict bool
	for _, w := range res.Warnings {
		if w.Code == WarnAddressConflict {
			sawConflict = true
			break
		}
	}
	if !sawConflict {
		t.Errorf("expected ADDRESS_CONFLICT warning, got %+v", res.Warnings)
	}
}

func TestMapper_PalettesBecomeLooksInDedicatedCueLists(t *testing.T) {
	deps := newTestDeps(t)
	defer deps.close()

	f := openFixtureFile(t, "palettes_color_focus.asc")
	defer func() { _ = f.Close() }()

	svc := NewServiceWithDeps(deps.projectRepo, deps.fixtureRepo, deps.fixtureGroupRepo, deps.lookRepo,
		deps.cueListRepo, deps.cueRepo, deps.lookBoardRepo)
	res, err := svc.Import(context.Background(), f, Options{NewProjectName: ptr("Pal")})
	if err != nil {
		t.Fatal(err)
	}
	lists, err := deps.cueListRepo.FindByProjectID(context.Background(), res.ProjectID)
	if err != nil {
		t.Fatal(err)
	}
	names := map[string]int{}
	for _, l := range lists {
		names[l.Name]++
	}
	if names["Color Palettes"] != 1 || names["Focus Palettes"] != 1 {
		t.Errorf("missing palette cue lists, got %v", names)
	}
}

func TestMapper_CuesProduceTrackedSnapshots(t *testing.T) {
	deps := newTestDeps(t)
	defer deps.close()

	f := openFixtureFile(t, "tracking.asc")
	defer func() { _ = f.Close() }()

	svc := NewServiceWithDeps(deps.projectRepo, deps.fixtureRepo, deps.fixtureGroupRepo, deps.lookRepo,
		deps.cueListRepo, deps.cueRepo, deps.lookBoardRepo)
	res, err := svc.Import(context.Background(), f, Options{NewProjectName: ptr("Track")})
	if err != nil {
		t.Fatal(err)
	}
	lists, err := deps.cueListRepo.FindByProjectID(context.Background(), res.ProjectID)
	if err != nil {
		t.Fatal(err)
	}
	var trackList *models.CueList
	for i := range lists {
		if lists[i].Name == "Track" {
			trackList = &lists[i]
		}
	}
	if trackList == nil {
		t.Fatalf("expected cue list 'Track', got %+v", lists)
	}
	cues, err := deps.cueListRepo.GetCues(context.Background(), trackList.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(cues) != 3 {
		t.Fatalf("cues: got %d, want 3", len(cues))
	}
	// Cue 2 should have channels 1,2,3 at full (1,2 tracked, 3 newly moved).
	cue2 := cues[1]
	values, err := deps.lookRepo.GetFixtureValues(context.Background(), cue2.LookID)
	if err != nil {
		t.Fatal(err)
	}
	fullCount := 0
	for _, fv := range values {
		var chans []models.ChannelValue
		if err := json.Unmarshal([]byte(fv.Channels), &chans); err != nil {
			t.Fatalf("unmarshal channels: %v", err)
		}
		for _, cv := range chans {
			if cv.Value == 0xFF {
				fullCount++
			}
		}
	}
	if fullCount != 3 {
		t.Errorf("expected 3 full intensities in cue 2, got %d", fullCount)
	}
}

func TestMapper_RefIDsDeterministicAcrossProjects(t *testing.T) {
	// Import the same minimal patch into two distinct fresh projects.
	// Same input file should yield the same RefIDs regardless of project,
	// which is what makes the $$ LACYLIGHTS: sidecar resolve cleanly on
	// round-trip.
	importInto := func(name string) string {
		deps := newTestDeps(t)
		t.Cleanup(deps.close)
		f := openFixtureFile(t, "minimal_patch.asc")
		t.Cleanup(func() { _ = f.Close() })
		svc := NewServiceWithDeps(deps.projectRepo, deps.fixtureRepo, deps.fixtureGroupRepo, deps.lookRepo,
			deps.cueListRepo, deps.cueRepo, deps.lookBoardRepo)
		res, err := svc.Import(context.Background(), f, Options{NewProjectName: ptr(name)})
		if err != nil {
			t.Fatalf("import %s: %v", name, err)
		}
		instances, err := deps.fixtureRepo.FindByProjectID(context.Background(), res.ProjectID)
		if err != nil {
			t.Fatalf("read: %v", err)
		}
		sort.Slice(instances, func(i, j int) bool {
			oi, oj := 0, 0
			if instances[i].ProjectOrder != nil {
				oi = *instances[i].ProjectOrder
			}
			if instances[j].ProjectOrder != nil {
				oj = *instances[j].ProjectOrder
			}
			return oi < oj
		})
		var b strings.Builder
		for _, fi := range instances {
			b.WriteString(fi.RefID)
			b.WriteByte('|')
		}
		return b.String()
	}

	a := importInto("Project A")
	b := importInto("Project B")
	if a != b {
		t.Errorf("non-deterministic RefIDs:\nA: %s\nB: %s", a, b)
	}
	if !strings.Contains(a, "ch-") {
		t.Errorf("expected EOS-derived RefIDs in %q", a)
	}
}

func TestMapper_GroupsCreated(t *testing.T) {
	deps := newTestDeps(t)
	defer deps.close()

	f := openFixtureFile(t, "groups_only.asc")
	defer func() { _ = f.Close() }()

	svc := NewServiceWithDeps(deps.projectRepo, deps.fixtureRepo, deps.fixtureGroupRepo, deps.lookRepo,
		deps.cueListRepo, deps.cueRepo, deps.lookBoardRepo)
	res, err := svc.Import(context.Background(), f, Options{NewProjectName: ptr("Test Show")})
	if err != nil {
		t.Fatalf("Import: %v", err)
	}
	if res.GroupsCount != 2 {
		t.Errorf("GroupsCount = %d, want 2", res.GroupsCount)
	}

	groups := deps.MustListGroupsByProject(t, res.ProjectID)
	if len(groups) != 2 {
		t.Fatalf("got %d groups, want 2", len(groups))
	}

	byEos := map[int]models.FixtureGroup{}
	for _, g := range groups {
		if g.EosNumber == nil {
			t.Errorf("group %s has nil EosNumber", g.Name)
			continue
		}
		byEos[*g.EosNumber] = g
	}

	if got := byEos[1].Name; got != "SR Specials" {
		t.Errorf("group 1 name = %q", got)
	}
	if got := byEos[2].Name; got != "All Lights" {
		t.Errorf("group 2 name = %q", got)
	}

	// Membership check: group 2 should have 3 fixtures.
	ms2, err := deps.fixtureGroupRepo.GetMembers(context.Background(), byEos[2].ID)
	if err != nil {
		t.Fatalf("members: %v", err)
	}
	if len(ms2) != 3 {
		t.Errorf("group 2 members = %d, want 3", len(ms2))
	}
}

func TestMapper_GroupChannelUnresolvedWarn(t *testing.T) {
	asc := `Ident 3:0
$ParamType 1 1 Intens
$Personality 1
$$Manuf Generic
$$Model Dimmer
$$Footprint 1
$$PersChan 1 1 1 0 0
$Patch 1 1 1
   Text A
$Group 1
   Text Mostly
   1 99
`
	deps := newTestDeps(t)
	defer deps.close()
	svc := NewServiceWithDeps(deps.projectRepo, deps.fixtureRepo, deps.fixtureGroupRepo, deps.lookRepo,
		deps.cueListRepo, deps.cueRepo, deps.lookBoardRepo)
	res, err := svc.Import(context.Background(), strings.NewReader(asc), Options{NewProjectName: ptr("T")})
	if err != nil {
		t.Fatalf("import: %v", err)
	}
	if res.GroupsCount != 1 {
		t.Errorf("GroupsCount = %d", res.GroupsCount)
	}
	saw := false
	for _, w := range res.Warnings {
		if w.Code == WarnGroupChannelUnresolved {
			saw = true
		}
	}
	if !saw {
		t.Errorf("expected WarnGroupChannelUnresolved")
	}
}

func TestMapper_SidecarLookBoardRebuilt(t *testing.T) {
	deps := newTestDeps(t)
	defer deps.close()

	f := openFixtureFile(t, "sidecar_roundtrip.asc")
	defer f.Close()
	svc := NewServiceWithDeps(deps.projectRepo, deps.fixtureRepo, deps.fixtureGroupRepo, deps.lookRepo,
		deps.cueListRepo, deps.cueRepo, deps.lookBoardRepo)
	res, err := svc.Import(context.Background(), f, Options{NewProjectName: ptr("T")})
	if err != nil {
		t.Fatalf("import: %v", err)
	}

	boards, _ := deps.lookBoardRepo.FindByProjectID(context.Background(), res.ProjectID)
	if len(boards) != 1 {
		t.Fatalf("got %d boards, want 1", len(boards))
	}
	if boards[0].Name != "Top" {
		t.Errorf("name = %q", boards[0].Name)
	}
	btns, _ := deps.lookBoardRepo.GetButtons(context.Background(), boards[0].ID)
	if len(btns) != 1 {
		t.Fatalf("got %d buttons, want 1", len(btns))
	}
	if btns[0].Color == nil || *btns[0].Color != "#ff0000" {
		t.Errorf("color = %v", btns[0].Color)
	}
}

func TestMapper_SidecarLookBoardUnresolvedButton(t *testing.T) {
	asc := `Ident 3:0
$ParamType 1 1 Intens
$Personality 1
$$Manuf Generic
$$Model Dimmer
$$Footprint 1
$$PersChan 1 1 1 0 0
$Patch 1 1 1
   Text A
$CueList 1
Cue 1 1
   Text Open
   Up 5
   $$ChanMove  1@HFF
$$ LACYLIGHTS:version 1
$$ LACYLIGHTS:look_board {"refId":"b1","name":"X","buttons":[{"lookRefId":"cue-1-1","x":0,"y":0,"color":"#ff"},{"lookRefId":"missing","x":1,"y":0,"color":"#00"}]}
`
	deps := newTestDeps(t)
	defer deps.close()
	svc := NewServiceWithDeps(deps.projectRepo, deps.fixtureRepo, deps.fixtureGroupRepo, deps.lookRepo,
		deps.cueListRepo, deps.cueRepo, deps.lookBoardRepo)
	res, err := svc.Import(context.Background(), strings.NewReader(asc), Options{NewProjectName: ptr("T")})
	if err != nil {
		t.Fatalf("import: %v", err)
	}
	boards, _ := deps.lookBoardRepo.FindByProjectID(context.Background(), res.ProjectID)
	if len(boards) != 1 {
		t.Fatalf("got %d boards", len(boards))
	}
	btns, _ := deps.lookBoardRepo.GetButtons(context.Background(), boards[0].ID)
	if len(btns) != 1 {
		t.Errorf("got %d buttons (should drop the unresolved one)", len(btns))
	}
	saw := false
	for _, w := range res.Warnings {
		if w.Code == WarnSidecarUnresolved && w.Context["kind"] == "look_board_button" {
			saw = true
		}
	}
	if !saw {
		t.Errorf("expected WarnSidecarUnresolved kind=look_board_button")
	}
}

func TestMapper_SidecarFadeBehaviorApplied(t *testing.T) {
	asc := `Ident 3:0
$ParamType 1 1 Intens
$ParamType 12 3 Red
$ParamType 13 3 Green
$Personality 1
   $$Manuf Generic
   $$Model RGB
   $$Footprint 3
   $$PersChan 1 1 1 0 0
   $$PersChan 12 1 2 0 0
   $$PersChan 13 1 3 0 0
$Patch 1 1 1
   Text A
$$ LACYLIGHTS:version 1
$$ LACYLIGHTS:fade_behavior {"instanceRefId":"ch-1","channels":[{"offset":1,"behavior":"SNAP_END"}]}
`
	deps := newTestDeps(t)
	defer deps.close()
	svc := NewServiceWithDeps(deps.projectRepo, deps.fixtureRepo, deps.fixtureGroupRepo, deps.lookRepo,
		deps.cueListRepo, deps.cueRepo, deps.lookBoardRepo)
	res, err := svc.Import(context.Background(), strings.NewReader(asc), Options{NewProjectName: ptr("T")})
	if err != nil {
		t.Fatalf("import: %v", err)
	}
	instances, _ := deps.fixtureRepo.FindByProjectID(context.Background(), res.ProjectID)
	if len(instances) != 1 {
		t.Fatalf("got %d instances, want 1", len(instances))
	}
	channels, _ := deps.fixtureRepo.GetInstanceChannels(context.Background(), instances[0].ID)
	want := map[int]string{0: "FADE", 1: "SNAP_END", 2: "FADE"}
	for _, c := range channels {
		if c.FadeBehavior != want[c.Offset] {
			t.Errorf("offset %d: %q, want %q", c.Offset, c.FadeBehavior, want[c.Offset])
		}
	}
}

func TestMapper_SidecarFadeBehaviorOffsetMissing(t *testing.T) {
	asc := `Ident 3:0
$ParamType 1 1 Intens
$Personality 1
   $$Manuf Generic
   $$Model Dimmer
   $$Footprint 1
   $$PersChan 1 1 1 0 0
$Patch 1 1 1
   Text A
$$ LACYLIGHTS:version 1
$$ LACYLIGHTS:fade_behavior {"instanceRefId":"ch-1","channels":[{"offset":99,"behavior":"SNAP"}]}
`
	deps := newTestDeps(t)
	defer deps.close()
	svc := NewServiceWithDeps(deps.projectRepo, deps.fixtureRepo, deps.fixtureGroupRepo, deps.lookRepo,
		deps.cueListRepo, deps.cueRepo, deps.lookBoardRepo)
	res, err := svc.Import(context.Background(), strings.NewReader(asc), Options{NewProjectName: ptr("T")})
	if err != nil {
		t.Fatalf("import: %v", err)
	}
	saw := false
	for _, w := range res.Warnings {
		if w.Code == WarnSidecarUnresolved && w.Context["kind"] == "fade_behavior_offset" {
			saw = true
		}
	}
	if !saw {
		t.Errorf("expected WarnSidecarUnresolved kind=fade_behavior_offset")
	}
}
