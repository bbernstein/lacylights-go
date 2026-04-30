package repositories

import (
	"context"
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
