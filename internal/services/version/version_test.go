package version

import (
	"sync"
	"testing"
)

func TestNewService(t *testing.T) {
	service := NewService()
	if service == nil {
		t.Error("Expected service to be non-nil")
	}
}

func TestIsSupported_NotOnPi(t *testing.T) {
	service := NewService()
	// On a development machine, the update script won't exist
	// so IsSupported should return false
	supported := service.IsSupported()
	// We can't assert true or false here because it depends on the environment
	// Just verify it doesn't panic
	_ = supported
}

func TestGetSystemVersions_NotSupported(t *testing.T) {
	service := NewService()

	// If not supported, should return empty info with VersionManagementSupported=false
	if !service.IsSupported() {
		info, err := service.GetSystemVersions()
		if err != nil {
			t.Errorf("Expected no error when not supported, got: %v", err)
		}
		if info == nil {
			t.Error("Expected info to be non-nil")
			return
		}
		if info.VersionManagementSupported {
			t.Error("Expected VersionManagementSupported to be false when script not available")
		}
		if len(info.Repositories) != 0 {
			t.Errorf("Expected 0 repositories, got %d", len(info.Repositories))
		}
	}
}

func TestGetAvailableVersions_NotSupported(t *testing.T) {
	service := NewService()

	// If not supported, should return empty list
	if !service.IsSupported() {
		versions, err := service.GetAvailableVersions("lacylights-go")
		if err != nil {
			t.Errorf("Expected no error when not supported, got: %v", err)
		}
		if len(versions) != 0 {
			t.Errorf("Expected 0 versions, got %d", len(versions))
		}
	}
}

func TestUpdateRepository_NotSupported(t *testing.T) {
	service := NewService()

	// If not supported, should return failure result
	if !service.IsSupported() {
		result, err := service.UpdateRepository("lacylights-go", nil)
		if err != nil {
			t.Errorf("Expected no error, got: %v", err)
		}
		if result == nil {
			t.Error("Expected result to be non-nil")
			return
		}
		if result.Success {
			t.Error("Expected Success to be false when script not available")
		}
		if result.Error == "" {
			t.Error("Expected Error to be set when not supported")
		}
	}
}

func TestUpdateAllRepositories_NotSupported(t *testing.T) {
	service := NewService()

	// If not supported, should return failure results
	if !service.IsSupported() {
		results, err := service.UpdateAllRepositories()
		if err != nil {
			t.Errorf("Expected no error, got: %v", err)
		}
		if len(results) != 1 {
			t.Errorf("Expected 1 result, got %d", len(results))
			return
		}
		if results[0].Success {
			t.Error("Expected Success to be false when script not available")
		}
	}
}

func TestValidateRepository(t *testing.T) {
	// Set up test repos - simulates all repos being installed
	SetTestInstalledRepos([]string{"lacylights-fe", "lacylights-go", "lacylights-mcp"})
	defer SetTestInstalledRepos(nil) // Reset after test

	tests := []struct {
		name       string
		repository string
		wantErr    bool
	}{
		{"valid lacylights-fe", "lacylights-fe", false},
		{"valid lacylights-go", "lacylights-go", false},
		{"valid lacylights-mcp", "lacylights-mcp", false},
		{"invalid repo", "invalid-repo", true},
		{"empty repo", "", true},
		{"command injection attempt", "lacylights-fe; rm -rf /", true},
		{"path traversal attempt", "../../../etc/passwd", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateRepository(tt.repository)
			if (err != nil) != tt.wantErr {
				t.Errorf("validateRepository(%q) error = %v, wantErr %v", tt.repository, err, tt.wantErr)
			}
		})
	}
}

func TestValidateRepository_WithoutMCP(t *testing.T) {
	// Set up test repos - simulates RPi without MCP
	SetTestInstalledRepos([]string{"lacylights-fe", "lacylights-go"})
	defer SetTestInstalledRepos(nil) // Reset after test

	tests := []struct {
		name       string
		repository string
		wantErr    bool
	}{
		{"valid lacylights-fe", "lacylights-fe", false},
		{"valid lacylights-go", "lacylights-go", false},
		{"mcp not installed", "lacylights-mcp", true}, // MCP should be invalid when not installed
		{"invalid repo", "invalid-repo", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateRepository(tt.repository)
			if (err != nil) != tt.wantErr {
				t.Errorf("validateRepository(%q) error = %v, wantErr %v", tt.repository, err, tt.wantErr)
			}
		})
	}
}

func TestDetectInstalledRepositories(t *testing.T) {
	defer SetTestInstalledRepos(nil) // Always reset after test

	// Test with all repos installed (Mac mode)
	t.Run("all repos installed", func(t *testing.T) {
		SetTestInstalledRepos([]string{"lacylights-fe", "lacylights-go", "lacylights-mcp"})
		repos := detectInstalledRepositories()

		if len(repos) != 3 {
			t.Errorf("Expected 3 repos when all installed, got %d", len(repos))
		}

		// Verify actual repository names
		expected := map[string]bool{"lacylights-fe": true, "lacylights-go": true, "lacylights-mcp": true}
		for _, repo := range repos {
			if !expected[repo] {
				t.Errorf("Unexpected repository in results: %s", repo)
			}
		}
	})

	// Test with only fe and go (RPi mode)
	t.Run("without MCP (RPi mode)", func(t *testing.T) {
		SetTestInstalledRepos([]string{"lacylights-fe", "lacylights-go"})
		repos := detectInstalledRepositories()

		if len(repos) != 2 {
			t.Errorf("Expected 2 repos when MCP not installed, got %d", len(repos))
		}

		// Verify MCP is not in the list
		for _, repo := range repos {
			if repo == "lacylights-mcp" {
				t.Error("MCP should not be in repository list when not installed")
			}
		}

		// Verify expected repos are present
		expected := map[string]bool{"lacylights-fe": true, "lacylights-go": true}
		for _, repo := range repos {
			if !expected[repo] {
				t.Errorf("Unexpected repository in results: %s", repo)
			}
		}
	})
}

func TestValidateRepository_NoReposInstalled(t *testing.T) {
	// Test edge case where no repositories are installed
	SetTestInstalledRepos([]string{})
	defer SetTestInstalledRepos(nil)

	err := validateRepository("lacylights-go")
	if err == nil {
		t.Error("Expected error when no repositories are installed")
	}

	expectedMsg := "no repositories are installed or version management is not properly configured"
	if err.Error() != expectedMsg {
		t.Errorf("Expected error message %q, got %q", expectedMsg, err.Error())
	}
}

func TestValidateVersion(t *testing.T) {
	tests := []struct {
		name    string
		version string
		wantErr bool
	}{
		{"valid v1.0.0", "v1.0.0", false},
		{"valid 1.0.0", "1.0.0", false},
		{"valid with prerelease", "v1.0.0-beta.1", false},
		{"valid with build metadata", "v1.0.0+build.123", false},
		{"empty (latest)", "", false},
		{"invalid format", "not-a-version", true},
		{"command injection attempt", "1.0.0; rm -rf /", true},
		{"missing patch", "v1.0", true},
		{"missing minor", "v1", true},
		{"letters in version", "v1.a.0", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateVersion(tt.version)
			if (err != nil) != tt.wantErr {
				t.Errorf("validateVersion(%q) error = %v, wantErr %v", tt.version, err, tt.wantErr)
			}
		})
	}
}

func TestIsUpdateAvailable(t *testing.T) {
	tests := []struct {
		name      string
		installed string
		latest    string
		expected  bool
	}{
		{"same version", "v1.0.0", "v1.0.0", false},
		{"different version", "v1.0.0", "v1.1.0", true},
		{"unknown installed", "unknown", "v1.0.0", false},
		{"unknown latest", "v1.0.0", "unknown", false},
		{"both unknown", "unknown", "unknown", false},
		{"empty installed", "", "v1.0.0", false},
		{"empty latest", "v1.0.0", "", false},
		{"no v prefix", "1.0.0", "1.1.0", true},
		{"mixed v prefix", "v1.0.0", "1.1.0", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isUpdateAvailable(tt.installed, tt.latest)
			if result != tt.expected {
				t.Errorf("isUpdateAvailable(%q, %q) = %v, expected %v",
					tt.installed, tt.latest, result, tt.expected)
			}
		})
	}
}

func TestGetBuildInfo_Defaults(t *testing.T) {
	// Reset to defaults before testing
	ResetBuildInfoForTesting()

	info := GetBuildInfo()

	if info.Version != "0.1.0" {
		t.Errorf("Expected default version '0.1.0', got %q", info.Version)
	}
	if info.GitCommit != "unknown" {
		t.Errorf("Expected default gitCommit 'unknown', got %q", info.GitCommit)
	}
	if info.BuildTime != "unknown" {
		t.Errorf("Expected default buildTime 'unknown', got %q", info.BuildTime)
	}
}

func TestSetBuildInfo(t *testing.T) {
	// Reset to defaults before testing
	ResetBuildInfoForTesting()

	SetBuildInfo("v1.2.3", "abc123", "2025-01-15T10:30:00Z")

	info := GetBuildInfo()

	if info.Version != "v1.2.3" {
		t.Errorf("Expected version 'v1.2.3', got %q", info.Version)
	}
	if info.GitCommit != "abc123" {
		t.Errorf("Expected gitCommit 'abc123', got %q", info.GitCommit)
	}
	if info.BuildTime != "2025-01-15T10:30:00Z" {
		t.Errorf("Expected buildTime '2025-01-15T10:30:00Z', got %q", info.BuildTime)
	}
}

func TestSetBuildInfo_EmptyStringsPreserveDefaults(t *testing.T) {
	// Reset to defaults before testing
	ResetBuildInfoForTesting()

	// Empty strings should preserve default values
	SetBuildInfo("", "", "")

	info := GetBuildInfo()

	if info.Version != "0.1.0" {
		t.Errorf("Expected default version '0.1.0' when empty string passed, got %q", info.Version)
	}
	if info.GitCommit != "unknown" {
		t.Errorf("Expected default gitCommit 'unknown' when empty string passed, got %q", info.GitCommit)
	}
	if info.BuildTime != "unknown" {
		t.Errorf("Expected default buildTime 'unknown' when empty string passed, got %q", info.BuildTime)
	}
}

func TestSetBuildInfo_PartialEmptyStrings(t *testing.T) {
	// Reset to defaults before testing
	ResetBuildInfoForTesting()

	// Only set version, leave others as defaults
	SetBuildInfo("v2.0.0", "", "")

	info := GetBuildInfo()

	if info.Version != "v2.0.0" {
		t.Errorf("Expected version 'v2.0.0', got %q", info.Version)
	}
	if info.GitCommit != "unknown" {
		t.Errorf("Expected default gitCommit 'unknown' when empty string passed, got %q", info.GitCommit)
	}
	if info.BuildTime != "unknown" {
		t.Errorf("Expected default buildTime 'unknown' when empty string passed, got %q", info.BuildTime)
	}
}

func TestSetBuildInfo_OnlyCalledOnce(t *testing.T) {
	// Reset to defaults before testing
	ResetBuildInfoForTesting()

	// First call should set the values
	SetBuildInfo("v1.0.0", "first", "2025-01-01T00:00:00Z")

	// Second call should be ignored due to sync.Once
	SetBuildInfo("v2.0.0", "second", "2025-12-31T23:59:59Z")

	info := GetBuildInfo()

	if info.Version != "v1.0.0" {
		t.Errorf("Expected version 'v1.0.0' (first call), got %q", info.Version)
	}
	if info.GitCommit != "first" {
		t.Errorf("Expected gitCommit 'first' (first call), got %q", info.GitCommit)
	}
	if info.BuildTime != "2025-01-01T00:00:00Z" {
		t.Errorf("Expected buildTime '2025-01-01T00:00:00Z' (first call), got %q", info.BuildTime)
	}
}

func TestSetBuildInfo_ConcurrentAccess(t *testing.T) {
	// Reset to defaults before testing
	ResetBuildInfoForTesting()

	// Test concurrent access to ensure thread safety
	var wg sync.WaitGroup
	const numGoroutines = 100

	// Start multiple goroutines trying to set build info simultaneously
	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			SetBuildInfo("v1.0.0", "concurrent", "2025-01-01T00:00:00Z")
		}(i)
	}

	wg.Wait()

	// All should have completed without panic
	info := GetBuildInfo()
	if info.Version != "v1.0.0" {
		t.Errorf("Expected version 'v1.0.0' after concurrent access, got %q", info.Version)
	}
}

func TestGetBuildInfo_ConcurrentReads(t *testing.T) {
	// Reset to defaults before testing
	ResetBuildInfoForTesting()
	SetBuildInfo("v1.0.0", "abc123", "2025-01-15T10:30:00Z")

	// Test concurrent reads
	var wg sync.WaitGroup
	const numGoroutines = 100

	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			info := GetBuildInfo()
			if info.Version != "v1.0.0" {
				t.Errorf("Expected version 'v1.0.0', got %q", info.Version)
			}
		}()
	}

	wg.Wait()
}

// TestDetectInstalledRepositories_Integration tests that detectInstalledRepositories
// is properly used by functions that depend on it. Since GetSystemVersions and
// UpdateAllRepositories require IsSupported() to return true (which needs the
// update script to exist), we verify the integration through the test override mechanism.
func TestDetectInstalledRepositories_Integration(t *testing.T) {
	defer SetTestInstalledRepos(nil) // Always reset after test

	// Test that when MCP is not in the installed repos list, it's excluded from operations
	t.Run("repository list respects installed repos", func(t *testing.T) {
		// Simulate RPi mode (no MCP)
		SetTestInstalledRepos([]string{"lacylights-fe", "lacylights-go"})

		repos := detectInstalledRepositories()

		// Verify MCP is not included
		for _, repo := range repos {
			if repo == "lacylights-mcp" {
				t.Error("MCP should not be in installed repos list on RPi mode")
			}
		}

		// Verify validateRepository correctly rejects MCP
		err := validateRepository("lacylights-mcp")
		if err == nil {
			t.Error("Expected validateRepository to reject MCP when not installed")
		}

		// Verify fe and go are accepted
		if err := validateRepository("lacylights-fe"); err != nil {
			t.Errorf("Expected lacylights-fe to be valid, got error: %v", err)
		}
		if err := validateRepository("lacylights-go"); err != nil {
			t.Errorf("Expected lacylights-go to be valid, got error: %v", err)
		}
	})

	// Test Mac mode (all repos)
	t.Run("all repos available in Mac mode", func(t *testing.T) {
		SetTestInstalledRepos([]string{"lacylights-fe", "lacylights-go", "lacylights-mcp"})

		repos := detectInstalledRepositories()

		if len(repos) != 3 {
			t.Errorf("Expected 3 repos in Mac mode, got %d", len(repos))
		}

		// All should be valid
		for _, repo := range repos {
			if err := validateRepository(repo); err != nil {
				t.Errorf("Expected %s to be valid, got error: %v", repo, err)
			}
		}
	})
}

// TestSetTestInstalledRepos_ConcurrentAccess verifies thread-safe access to the
// test override mechanism for repository detection.
func TestSetTestInstalledRepos_ConcurrentAccess(t *testing.T) {
	defer SetTestInstalledRepos(nil) // Always reset after test

	// Set an initial value before starting concurrent operations
	// This ensures reads won't see nil (which would fall back to checking real files)
	SetTestInstalledRepos([]string{"lacylights-fe", "lacylights-go"})

	var wg sync.WaitGroup
	const numGoroutines = 50

	// Concurrent writes and reads mixed together
	for i := 0; i < numGoroutines; i++ {
		wg.Add(2) // One for write, one for read

		// Write goroutine
		go func(n int) {
			defer wg.Done()
			if n%2 == 0 {
				SetTestInstalledRepos([]string{"lacylights-fe", "lacylights-go"})
			} else {
				SetTestInstalledRepos([]string{"lacylights-fe", "lacylights-go", "lacylights-mcp"})
			}
		}(i)

		// Read goroutine
		go func() {
			defer wg.Done()
			repos := detectInstalledRepositories()
			// Should have either 2 or 3 repos, never panic or race
			// Since we always set to 2 or 3 repos, we should never see 0
			if len(repos) < 2 || len(repos) > 3 {
				t.Errorf("Unexpected repo count: %d", len(repos))
			}
		}()
	}

	wg.Wait()
}

// TestUpdateAllRepositories_UsesInstalledRepos verifies that UpdateAllRepositories
// respects the installed repositories list. Since this test runs in an environment
// without the update script (IsSupported returns false), we verify the behavior
// through the validation layer which does use detectInstalledRepositories.
func TestUpdateAllRepositories_UsesInstalledRepos(t *testing.T) {
	defer SetTestInstalledRepos(nil) // Always reset after test

	service := NewService()

	// If version management is not supported, we can still verify that
	// the installed repos override works correctly
	SetTestInstalledRepos([]string{"lacylights-fe", "lacylights-go"})

	// Verify MCP is not in the installed repos
	repos := detectInstalledRepositories()
	for _, repo := range repos {
		if repo == "lacylights-mcp" {
			t.Error("MCP should not be in installed repos list")
		}
	}

	// When not supported, UpdateAllRepositories returns early with a failure result
	// This verifies the test environment is set up correctly
	if !service.IsSupported() {
		results, err := service.UpdateAllRepositories()
		if err != nil {
			t.Errorf("Expected no error, got: %v", err)
		}
		if len(results) != 1 {
			t.Errorf("Expected 1 result (not supported), got %d", len(results))
		}
	}
}

// TestGetSystemVersions_UsesInstalledRepos verifies that GetSystemVersions
// properly respects the installed repositories configuration. Since this test
// runs without the update script, we verify the validation layer behavior.
func TestGetSystemVersions_UsesInstalledRepos(t *testing.T) {
	defer SetTestInstalledRepos(nil) // Always reset after test

	service := NewService()

	// Configure for RPi mode (no MCP)
	SetTestInstalledRepos([]string{"lacylights-fe", "lacylights-go"})

	// Verify detectInstalledRepositories returns correct repos
	repos := detectInstalledRepositories()
	if len(repos) != 2 {
		t.Errorf("Expected 2 repos, got %d", len(repos))
	}

	// Verify MCP is excluded
	for _, repo := range repos {
		if repo == "lacylights-mcp" {
			t.Error("MCP should not be in installed repos for RPi mode")
		}
	}

	// When not supported, GetSystemVersions returns empty info
	// This verifies the test environment is set up correctly
	if !service.IsSupported() {
		info, err := service.GetSystemVersions()
		if err != nil {
			t.Errorf("Expected no error, got: %v", err)
		}
		if info == nil {
			t.Error("Expected info to be non-nil")
			return
		}
		if info.VersionManagementSupported {
			t.Error("Expected VersionManagementSupported to be false")
		}
	}
}
