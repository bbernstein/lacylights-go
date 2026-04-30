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
