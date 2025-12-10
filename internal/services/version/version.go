// Package version provides version management functionality for LacyLights.
// It interfaces with the update-repos.sh script to check and update component versions.
package version

import (
	"encoding/json"
	"fmt"
	"log"
	"os/exec"
	"regexp"
	"strings"
	"time"
)

const (
	// UpdateScriptPath is the path to the update-repos.sh script on the Pi
	UpdateScriptPath = "/opt/lacylights/scripts/update-repos.sh"
)

var (
	// validRepositories defines the allowed repository names to prevent command injection
	validRepositories = map[string]bool{
		"lacylights-fe":  true,
		"lacylights-go":  true,
		"lacylights-mcp": true,
	}
	// semverPattern validates version strings to prevent command injection
	semverPattern = regexp.MustCompile(`^v?(\d+)\.(\d+)\.(\d+)(-[a-zA-Z0-9.-]+)?(\+[a-zA-Z0-9.-]+)?$`)
)

// validateRepository checks if the repository name is valid
func validateRepository(repository string) error {
	if !validRepositories[repository] {
		return fmt.Errorf("invalid repository name: %s (must be one of: lacylights-fe, lacylights-go, lacylights-mcp)", repository)
	}
	return nil
}

// validateVersion checks if the version string is valid semver format
func validateVersion(version string) error {
	if version == "" {
		return nil // empty version means "latest"
	}
	if !semverPattern.MatchString(version) {
		return fmt.Errorf("invalid version format: %s (must be semver format, e.g., v1.0.0 or 1.2.3)", version)
	}
	return nil
}

// RepositoryVersion contains version information for a single repository
type RepositoryVersion struct {
	Repository      string
	Installed       string
	Latest          string
	UpdateAvailable bool
}

// SystemVersionInfo contains version information for all repositories
type SystemVersionInfo struct {
	Repositories               []*RepositoryVersion
	LastChecked                string
	VersionManagementSupported bool
}

// UpdateResult contains the result of an update operation
type UpdateResult struct {
	Success         bool
	Repository      string
	PreviousVersion string
	NewVersion      string
	Message         string
	Error           string
}

// Service provides version management functionality
type Service struct{}

// NewService creates a new version management service
func NewService() *Service {
	return &Service{}
}

// IsSupported checks if version management is available on this system
func (s *Service) IsSupported() bool {
	_, err := exec.LookPath(UpdateScriptPath)
	return err == nil
}

// versionsJSON represents the JSON output from update-repos.sh versions json
type versionsJSON struct {
	LacylightsFE struct {
		Installed string `json:"installed"`
		Latest    string `json:"latest"`
	} `json:"lacylights-fe"`
	LacylightsGo struct {
		Installed string `json:"installed"`
		Latest    string `json:"latest"`
	} `json:"lacylights-go"`
	LacylightsMCP struct {
		Installed string `json:"installed"`
		Latest    string `json:"latest"`
	} `json:"lacylights-mcp"`
}

// GetSystemVersions returns version information for all repositories
func (s *Service) GetSystemVersions() (*SystemVersionInfo, error) {
	if !s.IsSupported() {
		return &SystemVersionInfo{
			Repositories:               []*RepositoryVersion{},
			LastChecked:                "",
			VersionManagementSupported: false,
		}, nil
	}

	// Execute update-repos.sh versions json
	cmd := exec.Command(UpdateScriptPath, "versions", "json")
	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("failed to get versions: %w", err)
	}

	// Parse JSON output
	var versions versionsJSON
	if err := json.Unmarshal(output, &versions); err != nil {
		return nil, fmt.Errorf("failed to parse versions JSON: %w", err)
	}

	// Build repository list
	repos := []*RepositoryVersion{
		{
			Repository:      "lacylights-fe",
			Installed:       versions.LacylightsFE.Installed,
			Latest:          versions.LacylightsFE.Latest,
			UpdateAvailable: isUpdateAvailable(versions.LacylightsFE.Installed, versions.LacylightsFE.Latest),
		},
		{
			Repository:      "lacylights-go",
			Installed:       versions.LacylightsGo.Installed,
			Latest:          versions.LacylightsGo.Latest,
			UpdateAvailable: isUpdateAvailable(versions.LacylightsGo.Installed, versions.LacylightsGo.Latest),
		},
		{
			Repository:      "lacylights-mcp",
			Installed:       versions.LacylightsMCP.Installed,
			Latest:          versions.LacylightsMCP.Latest,
			UpdateAvailable: isUpdateAvailable(versions.LacylightsMCP.Installed, versions.LacylightsMCP.Latest),
		},
	}

	return &SystemVersionInfo{
		Repositories:               repos,
		LastChecked:                time.Now().UTC().Format(time.RFC3339),
		VersionManagementSupported: true,
	}, nil
}

// GetAvailableVersions returns available versions for a specific repository
func (s *Service) GetAvailableVersions(repository string) ([]string, error) {
	if !s.IsSupported() {
		return []string{}, nil
	}

	// Validate repository name to prevent command injection
	if err := validateRepository(repository); err != nil {
		return nil, err
	}

	// Execute update-repos.sh available <repo>
	cmd := exec.Command(UpdateScriptPath, "available", repository)
	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("failed to get available versions: %w", err)
	}

	// Parse output (one version per line)
	lines := strings.Split(strings.TrimSpace(string(output)), "\n")
	var versions []string
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line != "" && line != "[]" {
			versions = append(versions, line)
		}
	}

	return versions, nil
}

// UpdateRepository updates a specific repository to a specific version
func (s *Service) UpdateRepository(repository string, version *string) (*UpdateResult, error) {
	if !s.IsSupported() {
		return &UpdateResult{
			Success:    false,
			Repository: repository,
			Error:      "Version management not available on this platform",
		}, nil
	}

	// Validate repository name to prevent command injection
	if err := validateRepository(repository); err != nil {
		return &UpdateResult{
			Success:    false,
			Repository: repository,
			Error:      err.Error(),
		}, nil
	}

	// Validate version string if provided
	if version != nil {
		if err := validateVersion(*version); err != nil {
			return &UpdateResult{
				Success:    false,
				Repository: repository,
				Error:      err.Error(),
			}, nil
		}
	}

	// Get current version before update
	info, err := s.GetSystemVersions()
	if err != nil {
		return &UpdateResult{
			Success:    false,
			Repository: repository,
			Error:      fmt.Sprintf("Failed to get current version: %v", err),
		}, nil
	}

	var previousVersion string
	for _, repo := range info.Repositories {
		if repo.Repository == repository {
			previousVersion = repo.Installed
			break
		}
	}

	// Build command arguments
	args := []string{"update", repository}
	if version != nil && *version != "" {
		args = append(args, *version)
	}

	// Execute update-repos.sh update <repo> [version]
	cmd := exec.Command(UpdateScriptPath, args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return &UpdateResult{
			Success:         false,
			Repository:      repository,
			PreviousVersion: previousVersion,
			Error:           fmt.Sprintf("Update failed: %v\nOutput: %s", err, string(output)),
		}, nil
	}

	// Get new version after update
	infoAfter, err := s.GetSystemVersions()
	if err != nil {
		return &UpdateResult{
			Success:         true,
			Repository:      repository,
			PreviousVersion: previousVersion,
			NewVersion:      "unknown",
			Message:         "Update completed but failed to verify new version",
		}, nil
	}

	var newVersion string
	for _, repo := range infoAfter.Repositories {
		if repo.Repository == repository {
			newVersion = repo.Installed
			break
		}
	}

	return &UpdateResult{
		Success:         true,
		Repository:      repository,
		PreviousVersion: previousVersion,
		NewVersion:      newVersion,
		Message:         "Update completed successfully",
	}, nil
}

// UpdateAllRepositories updates all repositories to their latest versions
func (s *Service) UpdateAllRepositories() ([]*UpdateResult, error) {
	if !s.IsSupported() {
		return []*UpdateResult{
			{
				Success:    false,
				Repository: "all",
				Error:      "Version management not available on this platform",
			},
		}, nil
	}

	// Get current versions before update
	infoBefore, err := s.GetSystemVersions()
	previousVersions := make(map[string]string)
	if err != nil {
		log.Printf("Warning: failed to get versions before update: %v", err)
	} else if infoBefore != nil {
		for _, repo := range infoBefore.Repositories {
			previousVersions[repo.Repository] = repo.Installed
		}
	}

	// Execute update-repos.sh update-all
	cmd := exec.Command(UpdateScriptPath, "update-all")
	output, cmdErr := cmd.CombinedOutput()

	// Get versions after update
	infoAfter, err := s.GetSystemVersions()
	if err != nil {
		log.Printf("Warning: failed to get versions after update: %v", err)
	}
	newVersions := make(map[string]string)
	if infoAfter != nil {
		for _, repo := range infoAfter.Repositories {
			newVersions[repo.Repository] = repo.Installed
		}
	}

	// Build results for each repository
	repos := []string{"lacylights-fe", "lacylights-go", "lacylights-mcp"}
	var results []*UpdateResult

	if cmdErr != nil {
		// Update failed
		for _, repo := range repos {
			results = append(results, &UpdateResult{
				Success:         false,
				Repository:      repo,
				PreviousVersion: previousVersions[repo],
				NewVersion:      newVersions[repo],
				Error:           fmt.Sprintf("Update failed: %v\nOutput: %s", cmdErr, string(output)),
			})
		}
	} else {
		// Update succeeded
		for _, repo := range repos {
			results = append(results, &UpdateResult{
				Success:         true,
				Repository:      repo,
				PreviousVersion: previousVersions[repo],
				NewVersion:      newVersions[repo],
				Message:         "Update completed successfully",
			})
		}
	}

	return results, nil
}

// isUpdateAvailable checks if an update is available by comparing versions.
//
// Note: This function uses simple string comparison after normalizing the 'v' prefix.
// This is a heuristic approach that works for most cases but does not perform
// proper semantic versioning comparison (e.g., "1.9.0" vs "1.10.0" would compare
// lexicographically rather than numerically). For production use cases requiring
// precise version ordering, consider using a dedicated semver library like
// github.com/Masterminds/semver or golang.org/x/mod/semver.
//
// Current behavior:
// - Returns false if either version is "unknown" or empty
// - Returns true if versions differ after removing 'v' prefix
// - Does not account for pre-release versions (e.g., v1.0.0-beta < v1.0.0)
func isUpdateAvailable(installed, latest string) bool {
	// If either version is unknown, we can't determine if an update is available
	if installed == "unknown" || latest == "unknown" || installed == "" || latest == "" {
		return false
	}

	// Normalize versions (remove 'v' prefix for comparison)
	installed = strings.TrimPrefix(installed, "v")
	latest = strings.TrimPrefix(latest, "v")

	// Simple comparison - if they're different, an update might be available
	return installed != latest
}
