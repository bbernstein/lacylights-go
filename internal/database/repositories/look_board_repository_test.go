package repositories

import (
	"context"
	"testing"

	"github.com/bbernstein/lacylights-go/internal/database/models"
)

func TestLookBoardRepository_Create_DefaultsRefIDToID(t *testing.T) {
	testDB, cleanup := setupTestDB(t)
	defer cleanup()

	ctx := context.Background()
	repo := NewLookBoardRepository(testDB.DB)

	projRepo := NewProjectRepository(testDB.DB)
	proj := &models.Project{Name: "T"}
	if err := projRepo.Create(ctx, proj); err != nil {
		t.Fatalf("project: %v", err)
	}

	board := &models.LookBoard{ProjectID: proj.ID, Name: "Top"}
	if err := repo.Create(ctx, board); err != nil {
		t.Fatalf("create: %v", err)
	}
	if board.ID == "" {
		t.Fatalf("expected ID assigned")
	}
	if board.RefID != board.ID {
		t.Errorf("RefID = %q, want %q", board.RefID, board.ID)
	}
}

func TestLookBoardRepository_Create_PreservesExplicitRefID(t *testing.T) {
	testDB, cleanup := setupTestDB(t)
	defer cleanup()

	ctx := context.Background()
	repo := NewLookBoardRepository(testDB.DB)

	projRepo := NewProjectRepository(testDB.DB)
	proj := &models.Project{Name: "T"}
	if err := projRepo.Create(ctx, proj); err != nil {
		t.Fatalf("project: %v", err)
	}

	board := &models.LookBoard{ProjectID: proj.ID, Name: "Top", RefID: "explicit-ref"}
	if err := repo.Create(ctx, board); err != nil {
		t.Fatalf("create: %v", err)
	}
	if board.RefID != "explicit-ref" {
		t.Errorf("RefID = %q, want %q", board.RefID, "explicit-ref")
	}
}

func TestLookBoardRepository_FindByRefID(t *testing.T) {
	testDB, cleanup := setupTestDB(t)
	defer cleanup()

	ctx := context.Background()
	repo := NewLookBoardRepository(testDB.DB)
	projRepo := NewProjectRepository(testDB.DB)

	proj := &models.Project{Name: "P"}
	if err := projRepo.Create(ctx, proj); err != nil {
		t.Fatalf("proj: %v", err)
	}

	board := &models.LookBoard{ProjectID: proj.ID, Name: "Top", RefID: "lb-1"}
	if err := repo.Create(ctx, board); err != nil {
		t.Fatalf("create: %v", err)
	}

	got, err := repo.FindByRefID(ctx, proj.ID, "lb-1")
	if err != nil {
		t.Fatalf("find: %v", err)
	}
	if got == nil || got.ID != board.ID {
		t.Errorf("hit: got %+v, want ID %q", got, board.ID)
	}

	miss, err := repo.FindByRefID(ctx, proj.ID, "missing")
	if err != nil {
		t.Fatalf("miss: %v", err)
	}
	if miss != nil {
		t.Errorf("expected nil miss, got %+v", miss)
	}
}
