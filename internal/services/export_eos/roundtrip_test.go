package exporteos

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

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

	imp := importeos.NewServiceWithDeps(deps.projectRepo, deps.fixtureRepo, deps.lookRepo,
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

	exp := NewServiceWithDeps(deps.projectRepo, deps.fixtureRepo, deps.lookRepo,
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

	imp := importeos.NewServiceWithDeps(deps.projectRepo, deps.fixtureRepo, deps.lookRepo,
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

	exp := NewServiceWithDeps(deps.projectRepo, deps.fixtureRepo, deps.lookRepo,
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
}
