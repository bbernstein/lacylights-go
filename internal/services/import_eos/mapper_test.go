package importeos

import (
	"context"
	"strings"
	"testing"
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
