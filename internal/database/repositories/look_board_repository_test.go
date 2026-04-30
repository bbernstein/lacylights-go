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

func TestLookBoardRepository_ReplaceButtons(t *testing.T) {
	testDB, cleanup := setupTestDB(t)
	defer cleanup()

	ctx := context.Background()
	repo := NewLookBoardRepository(testDB.DB)
	projRepo := NewProjectRepository(testDB.DB)

	proj := &models.Project{Name: "T"}
	if err := projRepo.Create(ctx, proj); err != nil {
		t.Fatalf("proj: %v", err)
	}
	board := &models.LookBoard{ProjectID: proj.ID, Name: "Original"}
	if err := repo.CreateWithButtons(ctx, board, []models.LookBoardButton{
		{LookID: "look-a", LayoutX: 0, LayoutY: 0},
		{LookID: "look-b", LayoutX: 1, LayoutY: 0},
	}); err != nil {
		t.Fatalf("create: %v", err)
	}

	// Replace with a single new button and rename atomically.
	board.Name = "Renamed"
	color := "#ff0000"
	if err := repo.ReplaceButtons(ctx, board, []models.LookBoardButton{
		{LookID: "look-c", LayoutX: 5, LayoutY: 5, Color: &color},
	}); err != nil {
		t.Fatalf("replace: %v", err)
	}

	btns, err := repo.GetButtons(ctx, board.ID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if len(btns) != 1 {
		t.Fatalf("got %d buttons, want 1", len(btns))
	}
	if btns[0].LookID != "look-c" {
		t.Errorf("look = %q, want look-c", btns[0].LookID)
	}
	// Verify the rename persisted.
	boards, _ := repo.FindByProjectID(ctx, proj.ID)
	if len(boards) != 1 || boards[0].Name != "Renamed" {
		t.Errorf("rename: got %+v", boards)
	}

	// Replace with empty list — should leave board with zero buttons.
	if err := repo.ReplaceButtons(ctx, board, nil); err != nil {
		t.Fatalf("replace empty: %v", err)
	}
	btns2, _ := repo.GetButtons(ctx, board.ID)
	if len(btns2) != 0 {
		t.Errorf("expected 0 buttons after empty replace, got %d", len(btns2))
	}
}
