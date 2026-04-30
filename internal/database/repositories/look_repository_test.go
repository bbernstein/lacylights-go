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

func TestLookRepository_MapByRefID_ReturnsPresentOnly(t *testing.T) {
	testDB, cleanup := setupTestDB(t)
	defer cleanup()

	ctx := context.Background()
	repo := NewLookRepository(testDB.DB)
	projRepo := NewProjectRepository(testDB.DB)

	p1 := &models.Project{Name: "P1"}
	if err := projRepo.Create(ctx, p1); err != nil {
		t.Fatalf("p1: %v", err)
	}
	p2 := &models.Project{Name: "P2"}
	if err := projRepo.Create(ctx, p2); err != nil {
		t.Fatalf("p2: %v", err)
	}

	l1 := &models.Look{ProjectID: p1.ID, Name: "Look 1", RefID: "L1"}
	if err := repo.Create(ctx, l1); err != nil {
		t.Fatalf("l1: %v", err)
	}
	l2 := &models.Look{ProjectID: p1.ID, Name: "Look 2", RefID: "L2"}
	if err := repo.Create(ctx, l2); err != nil {
		t.Fatalf("l2: %v", err)
	}
	leak := &models.Look{ProjectID: p2.ID, Name: "Other", RefID: "L1"}
	if err := repo.Create(ctx, leak); err != nil {
		t.Fatalf("leak: %v", err)
	}

	got, err := repo.MapByRefID(ctx, p1.ID, []string{"L1", "L2", "missing"})
	if err != nil {
		t.Fatalf("map: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("expected 2 entries, got %d: %+v", len(got), got)
	}
	if got["L1"] != l1.ID {
		t.Errorf("L1 = %q, want %q", got["L1"], l1.ID)
	}
	if got["L2"] != l2.ID {
		t.Errorf("L2 = %q, want %q", got["L2"], l2.ID)
	}
	if _, present := got["missing"]; present {
		t.Errorf("expected missing absent, got %q", got["missing"])
	}
}

func TestLookRepository_FindByRefID(t *testing.T) {
	testDB, cleanup := setupTestDB(t)
	defer cleanup()

	ctx := context.Background()
	repo := NewLookRepository(testDB.DB)
	projRepo := NewProjectRepository(testDB.DB)

	proj := &models.Project{Name: "P"}
	if err := projRepo.Create(ctx, proj); err != nil {
		t.Fatalf("proj: %v", err)
	}

	look := &models.Look{ProjectID: proj.ID, Name: "L", RefID: "look-ref"}
	if err := repo.Create(ctx, look); err != nil {
		t.Fatalf("create: %v", err)
	}

	got, err := repo.FindByRefID(ctx, proj.ID, "look-ref")
	if err != nil {
		t.Fatalf("find: %v", err)
	}
	if got == nil || got.ID != look.ID {
		t.Errorf("hit: got %+v, want ID %q", got, look.ID)
	}

	miss, err := repo.FindByRefID(ctx, proj.ID, "missing")
	if err != nil {
		t.Fatalf("miss: %v", err)
	}
	if miss != nil {
		t.Errorf("expected nil miss, got %+v", miss)
	}
}

func TestLookRepository_FindByIDs(t *testing.T) {
	testDB, cleanup := setupTestDB(t)
	defer cleanup()
	ctx := context.Background()
	repo := NewLookRepository(testDB.DB)

	mk := func(name string) *models.Look {
		l := &models.Look{ProjectID: "p1", Name: name}
		if err := repo.Create(ctx, l); err != nil {
			t.Fatalf("create %s: %v", name, err)
		}
		return l
	}
	a, b, c := mk("A"), mk("B"), mk("C")

	got, err := repo.FindByIDs(ctx, nil)
	if err != nil {
		t.Errorf("empty: %v", err)
	}
	if got == nil || len(got) != 0 {
		t.Errorf("empty: want non-nil empty slice, got %v", got)
	}

	got, err = repo.FindByIDs(ctx, []string{a.ID, "missing", b.ID, c.ID})
	if err != nil {
		t.Fatalf("hits: %v", err)
	}
	if len(got) != 3 {
		t.Errorf("got %d, want 3", len(got))
	}
}
