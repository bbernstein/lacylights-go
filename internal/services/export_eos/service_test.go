package exporteos

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/bbernstein/lacylights-go/internal/database/models"
	"github.com/bbernstein/lacylights-go/internal/graphql/generated"
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
	fv1ch, _ := json.Marshal([]models.ChannelValue{{Offset: 0, Value: 255}, {Offset: 1, Value: 200}})
	fv1 := []models.FixtureValue{{FixtureID: instances[0].ID, Channels: string(fv1ch)}}
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
		{FixtureID: instances[0].ID, Channels: string(fv1ch)},
		{FixtureID: instances[1].ID, Channels: mustMarshal([]models.ChannelValue{{Offset: 0, Value: 255}, {Offset: 3, Value: 255}})},
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

func mustMarshal(v any) string {
	b, _ := json.Marshal(v)
	return string(b)
}

func TestExport_Deterministic(t *testing.T) {
	deps := newTestDeps(t)
	defer deps.close()
	pid := buildSampleProject(t, deps)
	svc := NewServiceWithDeps(deps.projectRepo, deps.fixtureRepo, deps.lookRepo,
		deps.cueListRepo, deps.cueRepo, deps.lookBoardRepo)

	var buf1, buf2 bytes.Buffer
	if _, err := svc.Export(context.Background(), pid, &buf1); err != nil {
		t.Fatalf("export 1: %v", err)
	}
	if _, err := svc.Export(context.Background(), pid, &buf2); err != nil {
		t.Fatalf("export 2: %v", err)
	}
	if buf1.String() != buf2.String() {
		t.Errorf("non-deterministic output:\n--- 1 ---\n%s\n--- 2 ---\n%s", buf1.String(), buf2.String())
	}
}

func TestExport_StructureBasics(t *testing.T) {
	deps := newTestDeps(t)
	defer deps.close()
	pid := buildSampleProject(t, deps)
	svc := NewServiceWithDeps(deps.projectRepo, deps.fixtureRepo, deps.lookRepo,
		deps.cueListRepo, deps.cueRepo, deps.lookBoardRepo)

	var buf bytes.Buffer
	res, err := svc.Export(context.Background(), pid, &buf)
	if err != nil {
		t.Fatalf("export: %v", err)
	}
	got := buf.String()
	if res.FilenameSuffix != ".asc" {
		t.Errorf("FilenameSuffix = %q, want .asc", res.FilenameSuffix)
	}
	for _, want := range []string{
		"Ident 3:0\n",
		"Manufacturer ETC\n",
		"Console Eos\n",
		"$$Title Sample Show\n",
		"$ParamType 1 1 Intens\n",
		"$ParamType 12 3 Red\n",
		"$Personality 9001\n",
		"   $$Manuf Generic\n",
		"   $$Model RGBW\n",
		"$Patch 1 1 9001\n",
		"   Text Front Wash\n",
		"$Patch 2 5 9001\n", // EOS channel 2, address 5 (universe 1, start 5)
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
		{name: "missing_look", mutate: func(d *testDeps, s *Service) {
			s.projectRepo = d.projectRepo
			s.fixtureRepo = d.fixtureRepo
		}, wantSub: "look repository"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			deps := newTestDeps(t)
			defer deps.close()
			svc := NewService()
			c.mutate(deps, svc)
			_, err := svc.Export(context.Background(), "x", &bytes.Buffer{})
			if err == nil || !strings.Contains(err.Error(), c.wantSub) {
				t.Errorf("err = %v, want substring %q", err, c.wantSub)
			}
		})
	}
}
