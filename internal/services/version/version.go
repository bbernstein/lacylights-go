// Package version provides version management functionality for LacyLights.
// It interfaces with the update-repos.sh script to check and update component versions.
package version

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"os/exec"
	"regexp"
	"strings"
	"sync"
	"time"
)

const (
	// UpdateScriptPath is the path to the update-repos.sh script on the Pi
	UpdateScriptPath = "/opt/lacylights/scripts/update-repos.sh"
	// SelfUpdateScriptPath is the path to the self-update wrapper for updating lacylights-go itself
	SelfUpdateScriptPath = "/opt/lacylights/scripts/self-update.sh"
	// RunUpdateScriptPath is the path to the secure update runner script
	// This script validates inputs before running updates, preventing command injection
	RunUpdateScriptPath = "/opt/lacylights/scripts/run-update.sh"
	// UpdateLogPath is the path to the update log file
	UpdateLogPath = "/opt/lacylights/logs/update.log"

	// Version file paths for detecting installed repositories
	frontendVersionFile = "/opt/lacylights/frontend-src/.lacylights-version"
	backendVersionFile  = "/opt/lacylights/backend/.lacylights-version"
	mcpVersionFile      = "/opt/lacylights/mcp/.lacylights-version"
)

// Build information - set at build time via ldflags or by calling SetBuildInfo
// Protected by buildInfoMu for thread-safe access
var (
	buildVersion   = "0.1.0"
	buildGitCommit = "unknown"
	buildTime      = "unknown"
	buildInfoOnce  sync.Once
	buildInfoMu    sync.RWMutex
)

// BuildInfo contains server build information for version verification
type BuildInfo struct {
	Version   string
	GitCommit string
	BuildTime string
}

// SetBuildInfo sets the build information (called from main package at startup).
// Uses sync.Once to ensure initialization happens exactly once, and RWMutex for
// thread-safe access. Empty string values are ignored to preserve default values
// during development builds.
func SetBuildInfo(version, gitCommit, buildTimeVal string) {
	buildInfoOnce.Do(func() {
		buildInfoMu.Lock()
		defer buildInfoMu.Unlock()
		if version != "" {
			buildVersion = version
		}
		if gitCommit != "" {
			buildGitCommit = gitCommit
		}
		if buildTimeVal != "" {
			buildTime = buildTimeVal
		}
	})
}

// GetBuildInfo returns the current build information.
// Thread-safe for concurrent access.
func GetBuildInfo() BuildInfo {
	buildInfoMu.RLock()
	defer buildInfoMu.RUnlock()
	return BuildInfo{
		Version:   buildVersion,
		GitCommit: buildGitCommit,
		BuildTime: buildTime,
	}
}

// ResetBuildInfoForTesting resets the build info to defaults for testing purposes.
// This function is thread-safe and can be called from concurrent tests.
// WARNING: This should only be called from test code.
func ResetBuildInfoForTesting() {
	buildInfoMu.Lock()
	defer buildInfoMu.Unlock()
	buildVersion = "0.1.0"
	buildGitCommit = "unknown"
	buildTime = "unknown"
	buildInfoOnce = sync.Once{}
}

var (
	// semverPattern validates version strings to prevent command injection
	semverPattern = regexp.MustCompile(`^v?(\d+)\.(\d+)\.(\d+)(-[a-zA-Z0-9.-]+)?(\+[a-zA-Z0-9.-]+)?$`)

	// testInstalledRepos allows tests to override repository detection.
	// When nil (default), detectInstalledRepositories() checks actual files.
	// When set, it returns this slice directly.
	testInstalledRepos   []string
	testInstalledReposMu sync.RWMutex
)

// SetTestInstalledRepos allows tests to override which repositories are "installed".
// Pass nil to reset to default behavior (check actual files).
// This function is thread-safe and can be called from concurrent tests.
// WARNING: This should only be called from test code.
func SetTestInstalledRepos(repos []string) {
	testInstalledReposMu.Lock()
	defer testInstalledReposMu.Unlock()
	testInstalledRepos = repos
}

// detectInstalledRepositories returns the list of repositories that are actually installed.
// This allows the system to dynamically adjust to different deployment configurations
// (e.g., RPi without MCP vs Mac with MCP).
//
// The repositories are returned in the correct update order:
// frontend first, then MCP (if installed), then backend last.
// Backend must be last because self-updates use systemd-run with a delay
// to avoid connection issues when the backend restarts itself.
func detectInstalledRepositories() []string {
	// Allow tests to override detection (thread-safe read)
	testInstalledReposMu.RLock()
	testOverride := testInstalledRepos
	testInstalledReposMu.RUnlock()

	if testOverride != nil {
		return testOverride
	}

	repos := []string{}

	// Check for frontend (update first)
	if _, err := os.Stat(frontendVersionFile); err == nil {
		repos = append(repos, "lacylights-fe")
	}
	// Only include MCP if it's installed (update second)
	if _, err := os.Stat(mcpVersionFile); err == nil {
		repos = append(repos, "lacylights-mcp")
	}
	// Check for backend (update last - self-updates use systemd-run with delay)
	if _, err := os.Stat(backendVersionFile); err == nil {
		repos = append(repos, "lacylights-go")
	}

	return repos
}

// validateRepository checks if the repository name is valid (must be an installed repository)
// This is more efficient than calling isValidRepository + detectInstalledRepositories separately.
func validateRepository(repository string) error {
	installedRepos := detectInstalledRepositories()

	// Handle edge case where no repositories are installed
	if len(installedRepos) == 0 {
		return fmt.Errorf("no repositories are installed or version management is not properly configured")
	}

	for _, repo := range installedRepos {
		if repo == repository {
			return nil
		}
	}

	return fmt.Errorf("invalid repository name: %s (must be one of: %s)", repository, strings.Join(installedRepos, ", "))
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
	_, err := os.Stat(UpdateScriptPath)
	return err == nil
}

// repoVersionInfo represents version info for a single repository in the JSON output
type repoVersionInfo struct {
	Installed string `json:"installed"`
	Latest    string `json:"latest"`
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
	output, err := cmd.CombinedOutput()
	if err != nil {
		// Log full output server-side for debugging, return sanitized error to client
		log.Printf("Failed to get versions: %v\nFull output: %s", err, string(output))
		return nil, fmt.Errorf("failed to get versions: %w", err)
	}

	// Parse JSON output dynamically to avoid duplicating repository names
	var versions map[string]repoVersionInfo
	if err := json.Unmarshal(output, &versions); err != nil {
		return nil, fmt.Errorf("failed to parse versions JSON: %w", err)
	}

	// Build repository list from detected installed repositories
	installedRepos := detectInstalledRepositories()
	repos := make([]*RepositoryVersion, 0, len(installedRepos))
	for _, repoName := range installedRepos {
		v, exists := versions[repoName]
		if !exists {
			log.Printf("Warning: repository %s not found in version data", repoName)
		}
		repos = append(repos, &RepositoryVersion{
			Repository:      repoName,
			Installed:       v.Installed,
			Latest:          v.Latest,
			UpdateAvailable: isUpdateAvailable(v.Installed, v.Latest),
		})
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
	output, err := cmd.CombinedOutput()
	if err != nil {
		// Log full output server-side for debugging, return sanitized error to client
		log.Printf("Failed to get available versions for %s: %v\nFull output: %s", repository, err, string(output))
		return nil, fmt.Errorf("failed to get available versions: %w", err)
	}

	// Parse output (one version per line) with validation
	lines := strings.Split(strings.TrimSpace(string(output)), "\n")
	var versions []string
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line != "" && line != "[]" {
			// Validate that returned versions match expected semver pattern
			// to prevent malicious or buggy script output from reaching clients
			if semverPattern.MatchString(line) {
				versions = append(versions, line)
			} else {
				log.Printf("Warning: skipping invalid version string from script: %q", line)
			}
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

	// Use different script for self-updates vs updating other components
	var cmd *exec.Cmd
	var output []byte

	// lacylights-go self-updates require systemd-run because they stop the backend
	if repository == "lacylights-go" {
		// Use systemd-run directly to avoid deadlock where:
		// 1. Backend waits to send GraphQL response
		// 2. Update script stops the backend (for go self-update)
		// 3. Stopping backend kills the update script (child process)
		// Solution: systemd-run schedules update with delay, returns immediately
		//
		// Security: We call the run-update.sh script which validates all inputs
		// before executing. This prevents command injection attacks.
		// The sudoers entry only allows specific systemd-run patterns with this script.

		// Verify the secure update runner script exists
		if _, err := os.Stat(RunUpdateScriptPath); err != nil {
			return &UpdateResult{
				Success:         false,
				Repository:      repository,
				PreviousVersion: previousVersion,
				Error:           "Update script not available on this system",
			}, nil
		}

		targetVersion := "latest"
		if version != nil && *version != "" {
			targetVersion = *version
		}

		// Build systemd-run command with static unit name
		// Note: Static unit names mean only one update can run at a time.
		// This is intentional - concurrent updates would cause conflicts anyway.
		unitName := "lacylights-self-update"
		description := "LacyLights Self-Update to " + targetVersion

		// Build arguments for systemd-run
		// The run-update.sh script handles logging internally
		args := []string{
			"systemd-run",
			"--unit=" + unitName,
			"--description=" + description,
			"--on-active=3s",
			"--timer-property=AccuracySec=100ms",
			RunUpdateScriptPath,
			repository,
		}
		// Add version argument if not "latest"
		if targetVersion != "latest" {
			args = append(args, targetVersion)
		}

		cmd = exec.Command("sudo", args...)

		// Capture stdout and stderr for debugging
		var stdout, stderr bytes.Buffer
		cmd.Stdout = &stdout
		cmd.Stderr = &stderr

		// Start the command without waiting for it to complete
		log.Printf("Starting %s update: sudo systemd-run --unit=%s (target: %s)", repository, unitName, targetVersion)
		err = cmd.Start()
		if err != nil {
			log.Printf("Failed to start update for %s: %v", repository, err)
			return &UpdateResult{
				Success:         false,
				Repository:      repository,
				PreviousVersion: previousVersion,
				Error:           fmt.Sprintf("Failed to start update: %v", err),
			}, nil
		}
		// Reap the process in background to prevent zombie processes
		// and log any output for debugging
		go func() {
			waitErr := cmd.Wait()
			stdoutStr := stdout.String()
			stderrStr := stderr.String()
			if waitErr != nil || len(stdoutStr) > 0 || len(stderrStr) > 0 {
				log.Printf("%s update systemd-run completed: err=%v, stdout=%s, stderr=%s", repository, waitErr, stdoutStr, stderrStr)
			}
		}()
		// Don't wait for the command to complete - let it run in the background
		// For updates that stop the backend, return success immediately
		// The actual update happens in the background via systemd-run
		log.Printf("%s update scheduled: version=%s, unit=%s", repository, targetVersion, unitName)
		return &UpdateResult{
			Success:         true,
			Repository:      repository,
			PreviousVersion: previousVersion,
			NewVersion:      targetVersion,
			Message:         "Update scheduled - service will restart automatically",
		}, nil
	}

	// Regular update for other components (fe only - doesn't stop backend)
	args := []string{"update", repository}
	if version != nil && *version != "" {
		args = append(args, *version)
	}
	cmd = exec.Command(UpdateScriptPath, args...)
	output, err = cmd.CombinedOutput()
	if err != nil {
		// Log full output server-side for debugging, return sanitized error to client
		log.Printf("Update failed for repository %s: %v\nFull output: %s", repository, err, string(output))
		return &UpdateResult{
			Success:         false,
			Repository:      repository,
			PreviousVersion: previousVersion,
			Error:           fmt.Sprintf("Update failed: %v", err),
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

	// Update installed repositories in order: frontend first, then backend
	// This ensures that if backend (self) update is triggered, other updates complete first
	// The backend update uses systemd-run with delay to avoid connection issues
	var results []*UpdateResult
	installedRepos := detectInstalledRepositories()

	for _, repo := range installedRepos {
		log.Printf("UpdateAll: updating %s to latest", repo)
		result, err := s.UpdateRepository(repo, nil) // nil = latest version
		if err != nil {
			log.Printf("UpdateAll: failed to update %s: %v", repo, err)
			results = append(results, &UpdateResult{
				Success:    false,
				Repository: repo,
				Error:      fmt.Sprintf("Failed to start update: %v", err),
			})
		} else {
			results = append(results, result)
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
