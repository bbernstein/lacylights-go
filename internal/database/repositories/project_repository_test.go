package repositories

import (
	"context"
	"testing"

	"github.com/bbernstein/lacylights-go/internal/database/models"
	"github.com/lucsky/cuid"
)

func TestFindAllByGroupIDs(t *testing.T) {
	tdb, cleanup := setupTestDB(t)
	defer cleanup()

	repo := NewProjectRepository(tdb.DB)
	ctx := context.Background()

	groupA := cuid.New()
	groupB := cuid.New()

	// Create projects: one in group A, one in group B, one with no group (legacy)
	projectA := &models.Project{ID: cuid.New(), Name: "Project A", GroupID: &groupA}
	projectB := &models.Project{ID: cuid.New(), Name: "Project B", GroupID: &groupB}
	projectNoGroup := &models.Project{ID: cuid.New(), Name: "Legacy Project"}

	for _, p := range []*models.Project{projectA, projectB, projectNoGroup} {
		if err := repo.Create(ctx, p); err != nil {
			t.Fatalf("failed to create project: %v", err)
		}
	}

	t.Run("nil groupIDs returns all projects", func(t *testing.T) {
		projects, err := repo.FindAllByGroupIDs(ctx, nil)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(projects) != 3 {
			t.Errorf("expected 3 projects, got %d", len(projects))
		}
	})

	t.Run("empty groupIDs returns no projects", func(t *testing.T) {
		projects, err := repo.FindAllByGroupIDs(ctx, []string{})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(projects) != 0 {
			t.Errorf("expected 0 projects, got %d", len(projects))
		}
	})

	t.Run("single group returns only that group's projects", func(t *testing.T) {
		projects, err := repo.FindAllByGroupIDs(ctx, []string{groupA})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(projects) != 1 {
			t.Errorf("expected 1 project, got %d", len(projects))
		}
		if len(projects) > 0 && projects[0].Name != "Project A" {
			t.Errorf("expected 'Project A', got %q", projects[0].Name)
		}
	})

	t.Run("multiple groups returns all matching but not legacy", func(t *testing.T) {
		projects, err := repo.FindAllByGroupIDs(ctx, []string{groupA, groupB})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(projects) != 2 {
			t.Errorf("expected 2 projects, got %d", len(projects))
		}
	})

	t.Run("non-matching group returns nothing", func(t *testing.T) {
		projects, err := repo.FindAllByGroupIDs(ctx, []string{"nonexistent"})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(projects) != 0 {
			t.Errorf("expected 0 projects, got %d", len(projects))
		}
	})

	t.Run("excludes soft-deleted projects", func(t *testing.T) {
		if err := repo.Delete(ctx, projectA.ID); err != nil {
			t.Fatalf("failed to delete project: %v", err)
		}
		projects, err := repo.FindAllByGroupIDs(ctx, []string{groupA})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(projects) != 0 {
			t.Errorf("expected 0 projects (deleted), got %d", len(projects))
		}
		// Restore for other tests
		if err := repo.Restore(ctx, projectA.ID); err != nil {
			t.Fatalf("failed to restore project: %v", err)
		}
	})
}

func TestFindByIDWithGroupCheck(t *testing.T) {
	tdb, cleanup := setupTestDB(t)
	defer cleanup()

	repo := NewProjectRepository(tdb.DB)
	ctx := context.Background()

	groupA := cuid.New()
	groupB := cuid.New()

	projectA := &models.Project{ID: cuid.New(), Name: "Project A", GroupID: &groupA}
	projectNoGroup := &models.Project{ID: cuid.New(), Name: "Legacy Project"}

	for _, p := range []*models.Project{projectA, projectNoGroup} {
		if err := repo.Create(ctx, p); err != nil {
			t.Fatalf("failed to create project: %v", err)
		}
	}

	t.Run("nil groupIDs returns project without filtering", func(t *testing.T) {
		project, err := repo.FindByIDWithGroupCheck(ctx, projectA.ID, nil)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if project == nil {
			t.Fatal("expected project, got nil")
		}
		if project.Name != "Project A" {
			t.Errorf("expected 'Project A', got %q", project.Name)
		}
	})

	t.Run("matching group returns project", func(t *testing.T) {
		project, err := repo.FindByIDWithGroupCheck(ctx, projectA.ID, []string{groupA})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if project == nil {
			t.Fatal("expected project, got nil")
		}
	})

	t.Run("non-matching group returns nil", func(t *testing.T) {
		project, err := repo.FindByIDWithGroupCheck(ctx, projectA.ID, []string{groupB})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if project != nil {
			t.Error("expected nil for non-matching group, got project")
		}
	})

	t.Run("legacy project not accessible to regular users", func(t *testing.T) {
		project, err := repo.FindByIDWithGroupCheck(ctx, projectNoGroup.ID, []string{groupB})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if project != nil {
			t.Error("expected nil for legacy project accessed by regular user")
		}
	})

	t.Run("legacy project accessible with nil groupIDs (admin/auth-off)", func(t *testing.T) {
		project, err := repo.FindByIDWithGroupCheck(ctx, projectNoGroup.ID, nil)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if project == nil {
			t.Fatal("expected legacy project to be accessible to admin")
		}
	})

	t.Run("nonexistent project returns nil", func(t *testing.T) {
		project, err := repo.FindByIDWithGroupCheck(ctx, "nonexistent", []string{groupA})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if project != nil {
			t.Error("expected nil for nonexistent project")
		}
	})

	t.Run("soft-deleted project returns nil", func(t *testing.T) {
		if err := repo.Delete(ctx, projectA.ID); err != nil {
			t.Fatalf("failed to delete project: %v", err)
		}
		project, err := repo.FindByIDWithGroupCheck(ctx, projectA.ID, []string{groupA})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if project != nil {
			t.Error("expected nil for soft-deleted project")
		}
	})
}
