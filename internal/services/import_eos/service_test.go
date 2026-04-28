package importeos

import (
	"context"
	"strings"
	"testing"

	"github.com/bbernstein/lacylights-go/internal/database/models"
)

// TestService_ImportFailsWhenRepositoriesMissing exercises the dependency
// validation path so callers see a clear error instead of a nil-pointer panic
// inside the mapper if a repo is not wired.
func TestService_ImportFailsWhenRepositoriesMissing(t *testing.T) {
	cases := []struct {
		name    string
		mutate  func(d *testDeps, s *Service)
		wantSub string
	}{
		{
			name:    "no_repos",
			mutate:  func(_ *testDeps, _ *Service) {},
			wantSub: "project repository",
		},
		{
			name: "only_project",
			mutate: func(d *testDeps, s *Service) {
				s.projectRepo = d.projectRepo
			},
			wantSub: "fixture repository",
		},
		{
			name: "missing_look",
			mutate: func(d *testDeps, s *Service) {
				s.projectRepo = d.projectRepo
				s.fixtureRepo = d.fixtureRepo
			},
			wantSub: "look repository",
		},
		{
			name: "missing_cue_list",
			mutate: func(d *testDeps, s *Service) {
				s.projectRepo = d.projectRepo
				s.fixtureRepo = d.fixtureRepo
				s.lookRepo = d.lookRepo
			},
			wantSub: "cue list repository",
		},
		{
			name: "missing_cue",
			mutate: func(d *testDeps, s *Service) {
				s.projectRepo = d.projectRepo
				s.fixtureRepo = d.fixtureRepo
				s.lookRepo = d.lookRepo
				s.cueListRepo = d.cueListRepo
			},
			wantSub: "cue repository",
		},
		{
			name: "missing_look_board",
			mutate: func(d *testDeps, s *Service) {
				s.projectRepo = d.projectRepo
				s.fixtureRepo = d.fixtureRepo
				s.lookRepo = d.lookRepo
				s.cueListRepo = d.cueListRepo
				s.cueRepo = d.cueRepo
			},
			wantSub: "look board repository",
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			deps := newTestDeps(t)
			defer deps.close()
			svc := NewService()
			c.mutate(deps, svc)
			_, err := svc.Import(context.Background(), strings.NewReader(""), Options{})
			if err == nil {
				t.Fatal("expected error")
			}
			if !strings.Contains(err.Error(), c.wantSub) {
				t.Errorf("error %q missing substring %q", err.Error(), c.wantSub)
			}
		})
	}
}

// TestMapper_NewProjectPersistsGroupID verifies Options.GroupID is written to
// models.Project.GroupID when a new project is created, matching the existing
// JSON import service's behavior.
func TestMapper_NewProjectPersistsGroupID(t *testing.T) {
	deps := newTestDeps(t)
	defer deps.close()

	// Pre-create the group so the FK is satisfied.
	group := &models.UserGroup{Name: "g1"}
	if err := deps.db.Create(group).Error; err != nil {
		t.Fatalf("create group: %v", err)
	}

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
`)

	svc := NewServiceWithDeps(deps.projectRepo, deps.fixtureRepo, deps.lookRepo,
		deps.cueListRepo, deps.cueRepo, deps.lookBoardRepo)
	res, err := svc.Import(context.Background(), src, Options{
		NewProjectName: ptr("With Group"),
		GroupID:        ptr(group.ID),
	})
	if err != nil {
		t.Fatalf("Import: %v", err)
	}
	if res.ProjectID == "" {
		t.Fatal("expected project ID")
	}

	var got models.Project
	if err := deps.db.First(&got, "id = ?", res.ProjectID).Error; err != nil {
		t.Fatalf("load project: %v", err)
	}
	if got.GroupID == nil || *got.GroupID != group.ID {
		t.Errorf("project.GroupID = %v, want %q", got.GroupID, group.ID)
	}
}
