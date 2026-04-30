package repositories

import (
	"context"
	"errors"
	"testing"

	"github.com/bbernstein/lacylights-go/internal/database/models"
)

func TestFixtureRepository_Create_DefaultsRefIDToID(t *testing.T) {
	testDB, cleanup := setupTestDB(t)
	defer cleanup()

	ctx := context.Background()
	repo := NewFixtureRepository(testDB.DB)

	projRepo := NewProjectRepository(testDB.DB)
	proj := &models.Project{Name: "T"}
	if err := projRepo.Create(ctx, proj); err != nil {
		t.Fatalf("project: %v", err)
	}

	def := &models.FixtureDefinition{
		Manufacturer: "Mfg",
		Model:        "Model",
		Type:         "test",
	}
	if err := repo.CreateDefinition(ctx, def); err != nil {
		t.Fatalf("definition: %v", err)
	}

	fixture := &models.FixtureInstance{
		Name:         "F1",
		ProjectID:    proj.ID,
		DefinitionID: def.ID,
		Universe:     1,
		StartChannel: 1,
	}
	if err := repo.Create(ctx, fixture); err != nil {
		t.Fatalf("create: %v", err)
	}
	if fixture.ID == "" {
		t.Fatalf("expected ID assigned")
	}
	if fixture.RefID != fixture.ID {
		t.Errorf("RefID = %q, want %q", fixture.RefID, fixture.ID)
	}
}

func TestFixtureRepository_Create_PreservesExplicitRefID(t *testing.T) {
	testDB, cleanup := setupTestDB(t)
	defer cleanup()

	ctx := context.Background()
	repo := NewFixtureRepository(testDB.DB)

	projRepo := NewProjectRepository(testDB.DB)
	proj := &models.Project{Name: "T"}
	if err := projRepo.Create(ctx, proj); err != nil {
		t.Fatalf("project: %v", err)
	}

	def := &models.FixtureDefinition{
		Manufacturer: "Mfg",
		Model:        "Model",
		Type:         "test",
	}
	if err := repo.CreateDefinition(ctx, def); err != nil {
		t.Fatalf("definition: %v", err)
	}

	fixture := &models.FixtureInstance{
		Name:         "F1",
		ProjectID:    proj.ID,
		DefinitionID: def.ID,
		Universe:     1,
		StartChannel: 1,
		RefID:        "explicit-ref",
	}
	if err := repo.Create(ctx, fixture); err != nil {
		t.Fatalf("create: %v", err)
	}
	if fixture.RefID != "explicit-ref" {
		t.Errorf("RefID = %q, want %q", fixture.RefID, "explicit-ref")
	}
}

func TestFixtureRepository_CreateDefinition_DefaultsRefIDToID(t *testing.T) {
	testDB, cleanup := setupTestDB(t)
	defer cleanup()

	ctx := context.Background()
	repo := NewFixtureRepository(testDB.DB)

	def := &models.FixtureDefinition{
		Manufacturer: "Mfg",
		Model:        "Model",
		Type:         "test",
	}
	if err := repo.CreateDefinition(ctx, def); err != nil {
		t.Fatalf("create: %v", err)
	}
	if def.ID == "" {
		t.Fatalf("expected ID assigned")
	}
	if def.RefID != def.ID {
		t.Errorf("RefID = %q, want %q", def.RefID, def.ID)
	}
}

func TestFixtureRepository_CreateDefinition_PreservesExplicitRefID(t *testing.T) {
	testDB, cleanup := setupTestDB(t)
	defer cleanup()

	ctx := context.Background()
	repo := NewFixtureRepository(testDB.DB)

	def := &models.FixtureDefinition{
		Manufacturer: "Mfg",
		Model:        "Model",
		Type:         "test",
		RefID:        "explicit-def-ref",
	}
	if err := repo.CreateDefinition(ctx, def); err != nil {
		t.Fatalf("create: %v", err)
	}
	if def.RefID != "explicit-def-ref" {
		t.Errorf("RefID = %q, want %q", def.RefID, "explicit-def-ref")
	}
}

func TestFixtureRepository_FindInstanceByRefID_ResolvesAndDistinguishesProjects(t *testing.T) {
	testDB, cleanup := setupTestDB(t)
	defer cleanup()

	ctx := context.Background()
	repo := NewFixtureRepository(testDB.DB)
	projRepo := NewProjectRepository(testDB.DB)

	p1 := &models.Project{Name: "P1"}
	if err := projRepo.Create(ctx, p1); err != nil {
		t.Fatalf("p1: %v", err)
	}
	p2 := &models.Project{Name: "P2"}
	if err := projRepo.Create(ctx, p2); err != nil {
		t.Fatalf("p2: %v", err)
	}

	def := &models.FixtureDefinition{Manufacturer: "Mfg", Model: "Model", Type: "test"}
	if err := repo.CreateDefinition(ctx, def); err != nil {
		t.Fatalf("def: %v", err)
	}

	fi1 := &models.FixtureInstance{
		Name: "F1", ProjectID: p1.ID, DefinitionID: def.ID,
		Universe: 1, StartChannel: 1, RefID: "ch-1",
	}
	if err := repo.Create(ctx, fi1); err != nil {
		t.Fatalf("fi1: %v", err)
	}
	fi2 := &models.FixtureInstance{
		Name: "F2", ProjectID: p2.ID, DefinitionID: def.ID,
		Universe: 1, StartChannel: 1, RefID: "ch-1",
	}
	if err := repo.Create(ctx, fi2); err != nil {
		t.Fatalf("fi2: %v", err)
	}

	got, err := repo.FindInstanceByRefID(ctx, p1.ID, "ch-1")
	if err != nil {
		t.Fatalf("find: %v", err)
	}
	if got == nil {
		t.Fatalf("expected hit")
	}
	if got.ID != fi1.ID {
		t.Errorf("got ID %q, want %q", got.ID, fi1.ID)
	}

	miss, err := repo.FindInstanceByRefID(ctx, p1.ID, "missing")
	if err != nil {
		t.Fatalf("miss: %v", err)
	}
	if miss != nil {
		t.Errorf("expected nil miss, got %+v", miss)
	}
}

func TestFixtureRepository_UpdateInstanceChannelFadeBehavior_OverridesOnlyTargetOffset(t *testing.T) {
	testDB, cleanup := setupTestDB(t)
	defer cleanup()

	ctx := context.Background()
	repo := NewFixtureRepository(testDB.DB)
	projRepo := NewProjectRepository(testDB.DB)

	proj := &models.Project{Name: "P"}
	if err := projRepo.Create(ctx, proj); err != nil {
		t.Fatalf("proj: %v", err)
	}
	def := &models.FixtureDefinition{Manufacturer: "Mfg", Model: "Model", Type: "test"}
	if err := repo.CreateDefinition(ctx, def); err != nil {
		t.Fatalf("def: %v", err)
	}
	fi := &models.FixtureInstance{
		Name: "F", ProjectID: proj.ID, DefinitionID: def.ID,
		Universe: 1, StartChannel: 1,
	}
	chans := []models.InstanceChannel{
		{Offset: 0, Name: "Dim", Type: "INTENSITY", FadeBehavior: "FADE"},
		{Offset: 1, Name: "R", Type: "RED", FadeBehavior: "FADE"},
		{Offset: 2, Name: "G", Type: "GREEN", FadeBehavior: "FADE"},
	}
	if err := repo.CreateWithChannels(ctx, fi, chans); err != nil {
		t.Fatalf("create with channels: %v", err)
	}

	if err := repo.UpdateInstanceChannelFadeBehavior(ctx, fi.ID, 1, "SNAP_END"); err != nil {
		t.Fatalf("update: %v", err)
	}

	got, err := repo.GetInstanceChannels(ctx, fi.ID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if len(got) != 3 {
		t.Fatalf("expected 3 channels, got %d", len(got))
	}
	want := map[int]string{0: "FADE", 1: "SNAP_END", 2: "FADE"}
	for _, c := range got {
		if c.FadeBehavior != want[c.Offset] {
			t.Errorf("offset %d: got %q, want %q", c.Offset, c.FadeBehavior, want[c.Offset])
		}
	}
}

func TestFixtureRepository_UpdateInstanceChannelFadeBehavior_UnknownOffsetReturnsSentinel(t *testing.T) {
	testDB, cleanup := setupTestDB(t)
	defer cleanup()

	ctx := context.Background()
	repo := NewFixtureRepository(testDB.DB)
	projRepo := NewProjectRepository(testDB.DB)

	proj := &models.Project{Name: "P"}
	if err := projRepo.Create(ctx, proj); err != nil {
		t.Fatalf("proj: %v", err)
	}
	def := &models.FixtureDefinition{Manufacturer: "Mfg", Model: "Model", Type: "test"}
	if err := repo.CreateDefinition(ctx, def); err != nil {
		t.Fatalf("def: %v", err)
	}
	fi := &models.FixtureInstance{
		Name: "F", ProjectID: proj.ID, DefinitionID: def.ID,
		Universe: 1, StartChannel: 1,
	}
	chans := []models.InstanceChannel{
		{Offset: 0, Name: "Dim", Type: "INTENSITY", FadeBehavior: "FADE"},
	}
	if err := repo.CreateWithChannels(ctx, fi, chans); err != nil {
		t.Fatalf("create: %v", err)
	}

	err := repo.UpdateInstanceChannelFadeBehavior(ctx, fi.ID, 5, "SNAP")
	if !errors.Is(err, ErrChannelOffsetNotFound) {
		t.Fatalf("expected ErrChannelOffsetNotFound, got %v", err)
	}
}

func TestFixtureRepository_FindDefinitionByRefID_Global(t *testing.T) {
	testDB, cleanup := setupTestDB(t)
	defer cleanup()

	ctx := context.Background()
	repo := NewFixtureRepository(testDB.DB)

	def := &models.FixtureDefinition{
		Manufacturer: "Generic",
		Model:        "Dimmer",
		Type:         "test",
		RefID:        "def-generic-dimmer",
	}
	if err := repo.CreateDefinition(ctx, def); err != nil {
		t.Fatalf("def: %v", err)
	}

	got, err := repo.FindDefinitionByRefID(ctx, "def-generic-dimmer")
	if err != nil {
		t.Fatalf("find: %v", err)
	}
	if got == nil {
		t.Fatalf("expected hit")
	}
	if got.ID != def.ID {
		t.Errorf("got ID %q, want %q", got.ID, def.ID)
	}

	miss, err := repo.FindDefinitionByRefID(ctx, "missing")
	if err != nil {
		t.Fatalf("miss: %v", err)
	}
	if miss != nil {
		t.Errorf("expected nil, got %+v", miss)
	}
}
