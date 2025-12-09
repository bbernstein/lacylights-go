package importservice

import (
	"testing"
)

func TestImportModeConstants(t *testing.T) {
	// Test that constants have expected values
	tests := []struct {
		mode     ImportMode
		expected string
	}{
		{ImportModeCreate, "CREATE"},
		{ImportModeMerge, "MERGE"},
		{ImportModeReplace, "REPLACE"},
	}

	for _, tt := range tests {
		if string(tt.mode) != tt.expected {
			t.Errorf("Expected %s, got %s", tt.expected, string(tt.mode))
		}
	}
}

func TestFixtureConflictStrategyConstants(t *testing.T) {
	// Test that constants have expected values
	tests := []struct {
		strategy FixtureConflictStrategy
		expected string
	}{
		{FixtureConflictSkip, "SKIP"},
		{FixtureConflictReplace, "REPLACE"},
		{FixtureConflictRename, "RENAME"},
	}

	for _, tt := range tests {
		if string(tt.strategy) != tt.expected {
			t.Errorf("Expected %s, got %s", tt.expected, string(tt.strategy))
		}
	}
}

func TestImportStats(t *testing.T) {
	stats := &ImportStats{
		FixtureDefinitionsCreated: 5,
		FixtureInstancesCreated:   10,
		ScenesCreated:             8,
		CueListsCreated:           2,
		CuesCreated:               15,
	}

	if stats.FixtureDefinitionsCreated != 5 {
		t.Errorf("Expected FixtureDefinitionsCreated 5, got %d", stats.FixtureDefinitionsCreated)
	}
	if stats.FixtureInstancesCreated != 10 {
		t.Errorf("Expected FixtureInstancesCreated 10, got %d", stats.FixtureInstancesCreated)
	}
	if stats.ScenesCreated != 8 {
		t.Errorf("Expected ScenesCreated 8, got %d", stats.ScenesCreated)
	}
	if stats.CueListsCreated != 2 {
		t.Errorf("Expected CueListsCreated 2, got %d", stats.CueListsCreated)
	}
	if stats.CuesCreated != 15 {
		t.Errorf("Expected CuesCreated 15, got %d", stats.CuesCreated)
	}
}

func TestImportOptions(t *testing.T) {
	projectName := "Test Project"
	targetProjectID := "proj-123"

	options := ImportOptions{
		Mode:                    ImportModeCreate,
		TargetProjectID:         &targetProjectID,
		ProjectName:             &projectName,
		FixtureConflictStrategy: FixtureConflictSkip,
		ImportBuiltInFixtures:   true,
	}

	if options.Mode != ImportModeCreate {
		t.Errorf("Expected Mode ImportModeCreate, got %s", options.Mode)
	}
	if options.TargetProjectID == nil || *options.TargetProjectID != targetProjectID {
		t.Error("Expected TargetProjectID to be set")
	}
	if options.ProjectName == nil || *options.ProjectName != projectName {
		t.Error("Expected ProjectName to be set")
	}
	if options.FixtureConflictStrategy != FixtureConflictSkip {
		t.Errorf("Expected FixtureConflictStrategy FixtureConflictSkip, got %s", options.FixtureConflictStrategy)
	}
	if !options.ImportBuiltInFixtures {
		t.Error("Expected ImportBuiltInFixtures to be true")
	}
}

func TestImportOptions_Defaults(t *testing.T) {
	// Test zero value options
	options := ImportOptions{}

	if options.Mode != "" {
		t.Errorf("Expected empty Mode, got %s", options.Mode)
	}
	if options.TargetProjectID != nil {
		t.Error("Expected nil TargetProjectID")
	}
	if options.ProjectName != nil {
		t.Error("Expected nil ProjectName")
	}
	if options.FixtureConflictStrategy != "" {
		t.Errorf("Expected empty FixtureConflictStrategy, got %s", options.FixtureConflictStrategy)
	}
	if options.ImportBuiltInFixtures {
		t.Error("Expected ImportBuiltInFixtures to be false by default")
	}
}

func TestNewService(t *testing.T) {
	// Test that NewService creates a service with nil repos
	// (since we can't easily create real repos without a database)
	service := NewService(nil, nil, nil, nil, nil)

	if service == nil {
		t.Fatal("Expected NewService to return non-nil service")
	}
	if service.projectRepo != nil {
		t.Error("Expected projectRepo to be nil")
	}
	if service.fixtureRepo != nil {
		t.Error("Expected fixtureRepo to be nil")
	}
	if service.sceneRepo != nil {
		t.Error("Expected sceneRepo to be nil")
	}
	if service.cueListRepo != nil {
		t.Error("Expected cueListRepo to be nil")
	}
	if service.cueRepo != nil {
		t.Error("Expected cueRepo to be nil")
	}
}

func TestService_Structure(t *testing.T) {
	// Test that Service struct has expected fields
	service := &Service{}

	// These should compile - verifying struct has expected field types
	_ = service.projectRepo
	_ = service.fixtureRepo
	_ = service.sceneRepo
	_ = service.cueListRepo
	_ = service.cueRepo
}

func TestImportStats_ZeroValues(t *testing.T) {
	stats := &ImportStats{}

	if stats.FixtureDefinitionsCreated != 0 {
		t.Errorf("Expected 0, got %d", stats.FixtureDefinitionsCreated)
	}
	if stats.FixtureInstancesCreated != 0 {
		t.Errorf("Expected 0, got %d", stats.FixtureInstancesCreated)
	}
	if stats.ScenesCreated != 0 {
		t.Errorf("Expected 0, got %d", stats.ScenesCreated)
	}
	if stats.CueListsCreated != 0 {
		t.Errorf("Expected 0, got %d", stats.CueListsCreated)
	}
	if stats.CuesCreated != 0 {
		t.Errorf("Expected 0, got %d", stats.CuesCreated)
	}
}

func TestImportMode_TypeConversion(t *testing.T) {
	// Test type conversion
	mode := ImportMode("CUSTOM")
	if string(mode) != "CUSTOM" {
		t.Errorf("Expected CUSTOM, got %s", string(mode))
	}
}

func TestFixtureConflictStrategy_TypeConversion(t *testing.T) {
	// Test type conversion
	strategy := FixtureConflictStrategy("CUSTOM")
	if string(strategy) != "CUSTOM" {
		t.Errorf("Expected CUSTOM, got %s", string(strategy))
	}
}
