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
