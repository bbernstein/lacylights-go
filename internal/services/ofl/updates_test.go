package ofl

import (
	"context"
	"testing"

	"github.com/bbernstein/lacylights-go/internal/database/models"
	"github.com/bbernstein/lacylights-go/internal/services/testutil"
)

func TestComputeFixtureHash(t *testing.T) {
	tests := []struct {
		name string
		json string
	}{
		{
			name: "simple JSON",
			json: `{"name": "Test"}`,
		},
		{
			name: "complex JSON",
			json: `{"name": "Complex", "channels": [1, 2, 3], "nested": {"key": "value"}}`,
		},
		{
			name: "empty JSON",
			json: `{}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			hash := ComputeFixtureHash(tt.json)
			if hash == "" {
				t.Error("Hash should not be empty")
			}
			if len(hash) != 64 { // SHA256 hex is 64 chars
				t.Errorf("Hash should be 64 chars (SHA256), got %d", len(hash))
			}

			// Verify deterministic
			hash2 := ComputeFixtureHash(tt.json)
			if hash != hash2 {
				t.Error("Hash should be deterministic")
			}
		})
	}
}

func TestComputeFixtureHash_DifferentInputs(t *testing.T) {
	hash1 := ComputeFixtureHash(`{"name": "A"}`)
	hash2 := ComputeFixtureHash(`{"name": "B"}`)

	if hash1 == hash2 {
		t.Error("Different JSON should produce different hashes")
	}
}

func TestUpdatesService_NewUpdatesService(t *testing.T) {
	testDB, cleanup := testutil.SetupTestDB(t)
	defer cleanup()

	service := NewUpdatesService(testDB.DB, testDB.FixtureRepo)
	if service == nil {
		t.Fatal("NewUpdatesService returned nil")
	}
}

func TestUpdatesService_GetCurrentFixtureHashes(t *testing.T) {
	testDB, cleanup := testutil.SetupTestDB(t)
	defer cleanup()

	ctx := context.Background()

	// Create a fixture definition with hash
	hash := "abc123def456abc123def456abc123def456abc123def456abc123def456abcd"
	def := &models.FixtureDefinition{
		ID:            "test-def-1",
		Manufacturer:  "TestMfg",
		Model:         "TestModel",
		Type:          "LED_PAR",
		OFLSourceHash: &hash,
	}
	if err := testDB.DB.Create(def).Error; err != nil {
		t.Fatalf("Failed to create fixture definition: %v", err)
	}

	service := NewUpdatesService(testDB.DB, testDB.FixtureRepo)
	hashes, err := service.GetCurrentFixtureHashes(ctx)
	if err != nil {
		t.Fatalf("GetCurrentFixtureHashes failed: %v", err)
	}

	key := "TestMfg/TestModel"
	if h, ok := hashes[key]; !ok {
		t.Errorf("Expected hash for %s", key)
	} else if h != hash {
		t.Errorf("Expected hash %s, got %s", hash, h)
	}
}

func TestUpdatesService_GetCurrentFixtureHashes_Empty(t *testing.T) {
	testDB, cleanup := testutil.SetupTestDB(t)
	defer cleanup()

	ctx := context.Background()

	service := NewUpdatesService(testDB.DB, testDB.FixtureRepo)
	hashes, err := service.GetCurrentFixtureHashes(ctx)
	if err != nil {
		t.Fatalf("GetCurrentFixtureHashes failed: %v", err)
	}

	if len(hashes) != 0 {
		t.Errorf("Expected empty hashes map, got %d entries", len(hashes))
	}
}

func TestUpdatesService_GetFixtureInstanceCounts(t *testing.T) {
	testDB, cleanup := testutil.SetupTestDB(t)
	defer cleanup()

	ctx := context.Background()

	// Create a project
	project := &models.Project{
		ID:   "test-project",
		Name: "Test Project",
	}
	if err := testDB.DB.Create(project).Error; err != nil {
		t.Fatalf("Failed to create project: %v", err)
	}

	// Create a fixture definition
	def := &models.FixtureDefinition{
		ID:           "test-def-1",
		Manufacturer: "TestMfg",
		Model:        "TestModel",
		Type:         "LED_PAR",
	}
	if err := testDB.DB.Create(def).Error; err != nil {
		t.Fatalf("Failed to create fixture definition: %v", err)
	}

	// Create fixture instances
	for i := 0; i < 3; i++ {
		instance := &models.FixtureInstance{
			ID:           "test-instance-" + string(rune('a'+i)),
			Name:         "Instance",
			ProjectID:    project.ID,
			DefinitionID: def.ID,
			Universe:     1,
			StartChannel: i*10 + 1,
		}
		if err := testDB.DB.Create(instance).Error; err != nil {
			t.Fatalf("Failed to create fixture instance: %v", err)
		}
	}

	service := NewUpdatesService(testDB.DB, testDB.FixtureRepo)
	counts, err := service.GetFixtureInstanceCounts(ctx)
	if err != nil {
		t.Fatalf("GetFixtureInstanceCounts failed: %v", err)
	}

	if count, ok := counts["test-def-1"]; !ok {
		t.Errorf("Expected count for test-def-1")
	} else if count != 3 {
		t.Errorf("Expected count 3, got %d", count)
	}
}

func TestUpdatesService_CompareFixture_New(t *testing.T) {
	testDB, cleanup := testutil.SetupTestDB(t)
	defer cleanup()

	ctx := context.Background()

	service := NewUpdatesService(testDB.DB, testDB.FixtureRepo)

	// Empty current hashes = new fixture
	currentHashes := make(map[string]string)
	instanceCounts := make(map[string]int)

	result, err := service.CompareFixture(ctx, "NewMfg", "NewModel", "hash123", currentHashes, instanceCounts)
	if err != nil {
		t.Fatalf("CompareFixture failed: %v", err)
	}

	if result.ChangeType != ChangeTypeNew {
		t.Errorf("Expected ChangeTypeNew, got %s", result.ChangeType)
	}
	if result.IsInUse {
		t.Error("New fixture should not be in use")
	}
}

func TestUpdatesService_CompareFixture_Unchanged(t *testing.T) {
	testDB, cleanup := testutil.SetupTestDB(t)
	defer cleanup()

	ctx := context.Background()

	service := NewUpdatesService(testDB.DB, testDB.FixtureRepo)

	// Same hash = unchanged
	currentHashes := map[string]string{
		"TestMfg/TestModel": "samehash",
	}
	instanceCounts := make(map[string]int)

	result, err := service.CompareFixture(ctx, "TestMfg", "TestModel", "samehash", currentHashes, instanceCounts)
	if err != nil {
		t.Fatalf("CompareFixture failed: %v", err)
	}

	if result.ChangeType != ChangeTypeUnchanged {
		t.Errorf("Expected ChangeTypeUnchanged, got %s", result.ChangeType)
	}
}

func TestUpdatesService_CompareFixture_Updated(t *testing.T) {
	testDB, cleanup := testutil.SetupTestDB(t)
	defer cleanup()

	ctx := context.Background()

	service := NewUpdatesService(testDB.DB, testDB.FixtureRepo)

	// Different hash = updated
	currentHashes := map[string]string{
		"TestMfg/TestModel": "oldhash",
	}
	instanceCounts := make(map[string]int)

	result, err := service.CompareFixture(ctx, "TestMfg", "TestModel", "newhash", currentHashes, instanceCounts)
	if err != nil {
		t.Fatalf("CompareFixture failed: %v", err)
	}

	if result.ChangeType != ChangeTypeUpdated {
		t.Errorf("Expected ChangeTypeUpdated, got %s", result.ChangeType)
	}
}

func TestUpdatesService_IsFixtureInUse(t *testing.T) {
	testDB, cleanup := testutil.SetupTestDB(t)
	defer cleanup()

	ctx := context.Background()

	// Create project and definition
	project := &models.Project{ID: "test-project", Name: "Test"}
	if err := testDB.DB.Create(project).Error; err != nil {
		t.Fatalf("Failed to create project: %v", err)
	}

	def := &models.FixtureDefinition{
		ID:           "test-def",
		Manufacturer: "Mfg",
		Model:        "Model",
		Type:         "LED_PAR",
	}
	if err := testDB.DB.Create(def).Error; err != nil {
		t.Fatalf("Failed to create definition: %v", err)
	}

	service := NewUpdatesService(testDB.DB, testDB.FixtureRepo)

	// Initially not in use
	inUse, count, err := service.IsFixtureInUse(ctx, def.ID)
	if err != nil {
		t.Fatalf("IsFixtureInUse failed: %v", err)
	}
	if inUse {
		t.Error("Fixture should not be in use initially")
	}
	if count != 0 {
		t.Errorf("Count should be 0, got %d", count)
	}

	// Create instance
	instance := &models.FixtureInstance{
		ID:           "test-instance",
		Name:         "Instance",
		ProjectID:    project.ID,
		DefinitionID: def.ID,
		Universe:     1,
		StartChannel: 1,
	}
	if err := testDB.DB.Create(instance).Error; err != nil {
		t.Fatalf("Failed to create instance: %v", err)
	}

	// Now in use
	inUse, count, err = service.IsFixtureInUse(ctx, def.ID)
	if err != nil {
		t.Fatalf("IsFixtureInUse failed: %v", err)
	}
	if !inUse {
		t.Error("Fixture should be in use")
	}
	if count != 1 {
		t.Errorf("Count should be 1, got %d", count)
	}
}
