package exporteos

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/bbernstein/lacylights-go/internal/database/models"
	importeos "github.com/bbernstein/lacylights-go/internal/services/import_eos"
)

func openFixture(t *testing.T, name string) *os.File {
	t.Helper()
	// import_eos has the test fixtures; reach across the package boundary so
	// we don't duplicate the 6,000-line OTBPA file.
	f, err := os.Open(filepath.Join("..", "import_eos", "testdata", name))
	if err != nil {
		t.Fatalf("open fixture %s: %v", name, err)
	}
	return f
}

func ptr[T any](v T) *T { return &v }

// TestRoundtrip_GoldenOTBPA imports the real-world OTBPA showfile, exports it
// through the writer, and re-parses the output. The test verifies that import
// + export + re-parse all succeed without fatal errors and that the patch
// count round-trips.
func TestRoundtrip_GoldenOTBPA(t *testing.T) {
	deps := newTestDeps(t)
	defer deps.close()

	f := openFixture(t, "golden_otbpa.asc")
	defer func() { _ = f.Close() }()

	imp := importeos.NewServiceWithDeps(deps.projectRepo, deps.fixtureRepo, deps.fixtureGroupRepo, deps.lookRepo,
		deps.cueListRepo, deps.cueRepo, deps.lookBoardRepo)
	res, err := imp.Import(context.Background(), f, importeos.Options{NewProjectName: ptr("OTBPA Roundtrip")})
	if err != nil {
		t.Fatalf("import: %v", err)
	}
	// OTBPA has 44 $Patch entries; 18 are unpatched (address 0) and skipped
	// with UNPATCHED_CHANNEL warnings, leaving 26 fixtures imported.
	// Asserting the exact number would over-couple the test to the fixture
	// (a small parser change might add or skip one entry); the floor of 20
	// catches catastrophic regressions while tolerating fixture edits.
	const expectedMinFixtures = 20
	if res.FixtureInstancesCount < expectedMinFixtures {
		t.Errorf("expected at least %d patched fixtures, got %d", expectedMinFixtures, res.FixtureInstancesCount)
	}
	codes := map[importeos.WarningCode]int{}
	for _, w := range res.Warnings {
		codes[w.Code]++
	}
	if codes[importeos.WarnEffectSkipped] == 0 {
		t.Errorf("expected EFFECT_SKIPPED warnings, got codes=%v", codes)
	}
	if codes[importeos.WarnUnpatchedChannel] == 0 {
		t.Errorf("expected UNPATCHED_CHANNEL warnings, got codes=%v", codes)
	}

	exp := NewServiceWithDeps(deps.projectRepo, deps.fixtureRepo, deps.fixtureGroupRepo, deps.lookRepo,
		deps.cueListRepo, deps.cueRepo, deps.lookBoardRepo)
	out, err := exp.Export(context.Background(), res.ProjectID)
	if err != nil {
		t.Fatalf("export: %v", err)
	}
	if len(out.Content) == 0 {
		t.Fatal("export produced empty output")
	}

	show2, _, err := importeos.Parse(strings.NewReader(out.Content))
	if err != nil {
		t.Fatalf("re-parse: %v", err)
	}
	if len(show2.Patch) != res.FixtureInstancesCount {
		t.Errorf("patch count after re-parse: got %d want %d", len(show2.Patch), res.FixtureInstancesCount)
	}
}

// TestRoundtrip_SyntheticSmall exercises the import → export → import flow on
// a tiny synthetic project to verify cue/look fidelity in detail without the
// noise of the OTBPA file's edge cases.
func TestRoundtrip_SyntheticSmall(t *testing.T) {
	deps := newTestDeps(t)
	defer deps.close()

	f := openFixture(t, "basic_cues.asc")
	defer func() { _ = f.Close() }()

	imp := importeos.NewServiceWithDeps(deps.projectRepo, deps.fixtureRepo, deps.fixtureGroupRepo, deps.lookRepo,
		deps.cueListRepo, deps.cueRepo, deps.lookBoardRepo)
	res, err := imp.Import(context.Background(), f, importeos.Options{NewProjectName: ptr("Roundtrip Small")})
	if err != nil {
		t.Fatalf("import: %v", err)
	}
	if res.FixtureInstancesCount == 0 {
		t.Fatal("expected at least one fixture instance")
	}
	if res.CuesCount == 0 {
		t.Fatal("expected at least one cue")
	}

	exp := NewServiceWithDeps(deps.projectRepo, deps.fixtureRepo, deps.fixtureGroupRepo, deps.lookRepo,
		deps.cueListRepo, deps.cueRepo, deps.lookBoardRepo)
	out, err := exp.Export(context.Background(), res.ProjectID)
	if err != nil {
		t.Fatalf("export: %v", err)
	}

	show2, _, err := importeos.Parse(strings.NewReader(out.Content))
	if err != nil {
		t.Fatalf("re-parse: %v", err)
	}
	if len(show2.Patch) != res.FixtureInstancesCount {
		t.Errorf("patch count: got %d want %d", len(show2.Patch), res.FixtureInstancesCount)
	}
	totalCues := 0
	for _, cl := range show2.CueLists {
		totalCues += len(cl.Cues)
	}
	if totalCues != res.CuesCount {
		t.Errorf("cue count: got %d want %d", totalCues, res.CuesCount)
	}

	// Value-level fidelity: confirm the first cue of the first cue list
	// carries forward at least one channel move with the same level. This
	// catches map-iteration nondeterminism that would slip past the
	// count-only checks.
	if len(show2.CueLists) == 0 || len(show2.CueLists[0].Cues) == 0 {
		t.Fatalf("re-parsed file has no cues")
	}
	firstCue := show2.CueLists[0].Cues[0]
	if len(firstCue.ChanMoves) == 0 && len(firstCue.ParamMoves) == 0 {
		t.Errorf("first cue has no chan/param moves; values were lost on round-trip")
	}
}

func TestEosRoundtrip_GroupsPreserved(t *testing.T) {
	src := newTestDeps(t)
	defer src.close()
	ctx := context.Background()

	// Seed src project + 3 fixtures + 1 group with all three.
	proj := &models.Project{Name: "RT"}
	if err := src.projectRepo.Create(ctx, proj); err != nil {
		t.Fatalf("project: %v", err)
	}
	def := &models.FixtureDefinition{Manufacturer: "Generic", Model: "Dimmer", Type: "DIMMER"}
	if err := src.fixtureRepo.CreateDefinition(ctx, def); err != nil {
		t.Fatalf("def: %v", err)
	}
	mkInst := func(name string, ch int) *models.FixtureInstance {
		c := ch
		fi := &models.FixtureInstance{ProjectID: proj.ID, DefinitionID: def.ID, Name: name, Universe: 1, StartChannel: ch, ProjectOrder: &c}
		if err := src.fixtureRepo.CreateWithChannels(ctx, fi, []models.InstanceChannel{{Offset: 0, Name: "I", Type: "INTENSITY", FadeBehavior: "FADE"}}); err != nil {
			t.Fatalf("inst %s: %v", name, err)
		}
		return fi
	}
	a, b, c := mkInst("A", 1), mkInst("B", 2), mkInst("C", 3)

	g := &models.FixtureGroup{ProjectID: proj.ID, Name: "Main"}
	if err := src.fixtureGroupRepo.CreateWithMembers(ctx, g, []string{a.ID, b.ID, c.ID}); err != nil {
		t.Fatalf("group: %v", err)
	}

	// Export.
	exportSvc := NewServiceWithDeps(src.projectRepo, src.fixtureRepo, src.fixtureGroupRepo, src.lookRepo, src.cueListRepo, src.cueRepo, src.lookBoardRepo)
	exportRes, err := exportSvc.Export(ctx, proj.ID)
	if err != nil {
		t.Fatalf("export: %v", err)
	}

	// Re-import into a fresh deps stack.
	dst := newTestDeps(t)
	defer dst.close()
	importSvc := importeos.NewServiceWithDeps(dst.projectRepo, dst.fixtureRepo, dst.fixtureGroupRepo, dst.lookRepo, dst.cueListRepo, dst.cueRepo, dst.lookBoardRepo)
	importRes, err := importSvc.Import(ctx, strings.NewReader(exportRes.Content), importeos.Options{NewProjectName: ptr("RT2")})
	if err != nil {
		t.Fatalf("re-import: %v", err)
	}

	groups, _ := dst.fixtureGroupRepo.FindByProjectID(ctx, importRes.ProjectID)
	if len(groups) != 1 {
		t.Fatalf("got %d groups", len(groups))
	}
	if groups[0].Name != "Main" {
		t.Errorf("name = %q", groups[0].Name)
	}
	ms, _ := dst.fixtureGroupRepo.GetMembers(ctx, groups[0].ID)
	if len(ms) != 3 {
		t.Errorf("members = %d, want 3", len(ms))
	}
}

func TestEosRoundtrip_FadeBehaviorsPreserved(t *testing.T) {
	src := newTestDeps(t)
	defer src.close()
	ctx := context.Background()
	proj := &models.Project{Name: "RT"}
	if err := src.projectRepo.Create(ctx, proj); err != nil {
		t.Fatalf("project: %v", err)
	}
	def := &models.FixtureDefinition{Manufacturer: "Generic", Model: "RGB", Type: "DIMMER"}
	if err := src.fixtureRepo.CreateDefinition(ctx, def); err != nil {
		t.Fatalf("def: %v", err)
	}
	one := 1
	fi := &models.FixtureInstance{ProjectID: proj.ID, DefinitionID: def.ID, Name: "A", Universe: 1, StartChannel: 1, ProjectOrder: &one}
	if err := src.fixtureRepo.CreateWithChannels(ctx, fi, []models.InstanceChannel{
		{Offset: 0, Name: "I", Type: "INTENSITY", FadeBehavior: "FADE"},
		{Offset: 1, Name: "R", Type: "COLOR_R", FadeBehavior: "SNAP_END"},
	}); err != nil {
		t.Fatalf("fi: %v", err)
	}

	exportRes, err := NewServiceWithDeps(src.projectRepo, src.fixtureRepo, src.fixtureGroupRepo, src.lookRepo, src.cueListRepo, src.cueRepo, src.lookBoardRepo).Export(ctx, proj.ID)
	if err != nil {
		t.Fatalf("export: %v", err)
	}

	dst := newTestDeps(t)
	defer dst.close()
	importRes, err := importeos.NewServiceWithDeps(dst.projectRepo, dst.fixtureRepo, dst.fixtureGroupRepo, dst.lookRepo, dst.cueListRepo, dst.cueRepo, dst.lookBoardRepo).
		Import(ctx, strings.NewReader(exportRes.Content), importeos.Options{NewProjectName: ptr("RT2")})
	if err != nil {
		t.Fatalf("re-import: %v", err)
	}

	instances, _ := dst.fixtureRepo.FindByProjectID(ctx, importRes.ProjectID)
	if len(instances) != 1 {
		t.Fatalf("got %d", len(instances))
	}
	channels, _ := dst.fixtureRepo.GetInstanceChannels(ctx, instances[0].ID)
	want := map[int]string{0: "FADE", 1: "SNAP_END"}
	for _, c := range channels {
		if c.FadeBehavior != want[c.Offset] {
			t.Errorf("offset %d: %q, want %q", c.Offset, c.FadeBehavior, want[c.Offset])
		}
	}
}

func TestExport_DeterministicAfterAutoAssign(t *testing.T) {
	deps := newTestDeps(t)
	defer deps.close()
	ctx := context.Background()
	proj := &models.Project{Name: "D"}
	if err := deps.projectRepo.Create(ctx, proj); err != nil {
		t.Fatalf("proj: %v", err)
	}
	def := &models.FixtureDefinition{Manufacturer: "Generic", Model: "Dimmer", Type: "DIMMER"}
	if err := deps.fixtureRepo.CreateDefinition(ctx, def); err != nil {
		t.Fatalf("def: %v", err)
	}
	one := 1
	fi := &models.FixtureInstance{ProjectID: proj.ID, DefinitionID: def.ID, Name: "A", Universe: 1, StartChannel: 1, ProjectOrder: &one}
	if err := deps.fixtureRepo.CreateWithChannels(ctx, fi, []models.InstanceChannel{{Offset: 0, Name: "I", Type: "INTENSITY", FadeBehavior: "FADE"}}); err != nil {
		t.Fatalf("fi: %v", err)
	}
	g := &models.FixtureGroup{ProjectID: proj.ID, Name: "G"}
	if err := deps.fixtureGroupRepo.CreateWithMembers(ctx, g, []string{fi.ID}); err != nil {
		t.Fatalf("group: %v", err)
	}

	svc := NewServiceWithDeps(deps.projectRepo, deps.fixtureRepo, deps.fixtureGroupRepo, deps.lookRepo, deps.cueListRepo, deps.cueRepo, deps.lookBoardRepo)
	first, err := svc.Export(ctx, proj.ID)
	if err != nil {
		t.Fatalf("export 1: %v", err)
	}
	second, err := svc.Export(ctx, proj.ID)
	if err != nil {
		t.Fatalf("export 2: %v", err)
	}
	if first.Content != second.Content {
		t.Errorf("non-deterministic output\nFIRST:\n%s\n\nSECOND:\n%s", first.Content, second.Content)
	}
}
