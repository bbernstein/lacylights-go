package version

import (
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
