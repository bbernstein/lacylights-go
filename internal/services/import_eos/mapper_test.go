package importeos

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
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

	svc := NewServiceWithDeps(deps.projectRepo, deps.fixtureRepo, deps.lookRepo,
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

func TestImport_AddressConflictEmitsWarning(t *testing.T) {
	// EOS supports multi-patching (two logical channels at the same DMX
	// address); the importer surfaces that as a WARN-severity warning and
	// continues, rather than aborting the whole file with a hard error.
	deps := newTestDeps(t)
	defer deps.close()
	f := openFixtureFile(t, "conflict_addresses.asc")
	defer func() { _ = f.Close() }()
	svc := NewServiceWithDeps(deps.projectRepo, deps.fixtureRepo, deps.lookRepo,
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

	svc := NewServiceWithDeps(deps.projectRepo, deps.fixtureRepo, deps.lookRepo,
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

	svc := NewServiceWithDeps(deps.projectRepo, deps.fixtureRepo, deps.lookRepo,
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
