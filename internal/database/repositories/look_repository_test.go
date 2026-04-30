package repositories

import (
	"context"
	"testing"

	"github.com/bbernstein/lacylights-go/internal/database/models"
)

func TestLookRepository_Create_DefaultsRefIDToID(t *testing.T) {
	testDB, cleanup := setupTestDB(t)
	defer cleanup()

	ctx := context.Background()
	repo := NewLookRepository(testDB.DB)

	projRepo := NewProjectRepository(testDB.DB)
	proj := &models.Project{Name: "T"}
	if err := projRepo.Create(ctx, proj); err != nil {
		t.Fatalf("project: %v", err)
	}

	look := &models.Look{ProjectID: proj.ID, Name: "Look 1"}
	if err := repo.Create(ctx, look); err != nil {
		t.Fatalf("create: %v", err)
	}
	if look.ID == "" {
		t.Fatalf("expected ID assigned")
	}
	if look.RefID != look.ID {
		t.Errorf("RefID = %q, want %q", look.RefID, look.ID)
	}
}

func TestLookRepository_Create_PreservesExplicitRefID(t *testing.T) {
	testDB, cleanup := setupTestDB(t)
	defer cleanup()

	ctx := context.Background()
	repo := NewLookRepository(testDB.DB)

	projRepo := NewProjectRepository(testDB.DB)
	proj := &models.Project{Name: "T"}
	if err := projRepo.Create(ctx, proj); err != nil {
		t.Fatalf("project: %v", err)
	}

	look := &models.Look{ProjectID: proj.ID, Name: "Look 1", RefID: "explicit-ref"}
	if err := repo.Create(ctx, look); err != nil {
		t.Fatalf("create: %v", err)
	}
	if look.RefID != "explicit-ref" {
		t.Errorf("RefID = %q, want %q", look.RefID, "explicit-ref")
	}
}
