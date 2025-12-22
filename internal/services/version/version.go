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
	"time"
)

const (
	// UpdateScriptPath is the path to the update-repos.sh script on the Pi
	UpdateScriptPath = "/opt/lacylights/scripts/update-repos.sh"
	// SelfUpdateScriptPath is the path to the self-update wrapper for updating lacylights-go itself
	SelfUpdateScriptPath = "/opt/lacylights/scripts/self-update.sh"
	// UpdateLogPath is the path to the update log file
	UpdateLogPath = "/opt/lacylights/logs/update.log"
)

// Build information - set at build time via ldflags or by calling SetBuildInfo
var (
	buildVersion   = "0.1.0"
	buildGitCommit = "unknown"
	buildTime      = "unknown"
)

// BuildInfo contains server build information for version verification
type BuildInfo struct {
	Version   string
	GitCommit string
	BuildTime string
}

// SetBuildInfo sets the build information (called from main package)
func SetBuildInfo(version, gitCommit, buildTimeVal string) {
	buildVersion = version
	buildGitCommit = gitCommit
	buildTime = buildTimeVal
}

// GetBuildInfo returns the current build information
func GetBuildInfo() BuildInfo {
	return BuildInfo{
		Version:   buildVersion,
		GitCommit: buildGitCommit,
		BuildTime: buildTime,
	}
}

var (
	// repositoryNames is the canonical list of managed repositories
	repositoryNames = []string{"lacylights-fe", "lacylights-go", "lacylights-mcp"}

	// validRepositories defines the allowed repository names to prevent command injection
	// Derived from repositoryNames to avoid duplication
	validRepositories = func() map[string]bool {
		m := make(map[string]bool)
		for _, name := range repositoryNames {
			m[name] = true
		}
		return m
	}()

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

	// Build repository list from the shared repositoryNames constant
	repos := make([]*RepositoryVersion, 0, len(repositoryNames))
	for _, repoName := range repositoryNames {
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

	// Both lacylights-go and lacylights-mcp updates require systemd-run because they stop the backend
	if repository == "lacylights-go" || repository == "lacylights-mcp" {
		// Use systemd-run directly to avoid deadlock where:
		// 1. Backend waits to send GraphQL response
		// 2. Update script stops the backend (for go self-update or mcp update)
		// 3. Stopping backend kills the update script (child process)
		// Solution: systemd-run schedules update with delay, returns immediately
		//
		// We call sudo systemd-run directly instead of using a wrapper script
		// because the backend has NoNewPrivileges=true which prevents child processes
		// from using sudo. The sudoers entry allows this specific sudo command.
		targetVersion := "latest"
		if version != nil && *version != "" {
			targetVersion = *version
		}

		// Build the update command that systemd-run will execute
		// Quote all arguments for shell safety
		updateCmd := fmt.Sprintf("'%s' update '%s'", UpdateScriptPath, repository)
		if targetVersion != "latest" {
			updateCmd += fmt.Sprintf(" '%s'", targetVersion)
		}
		updateCmd += fmt.Sprintf(" >> %s 2>&1", UpdateLogPath)

		// Build systemd-run command with unique unit name to avoid conflicts
		// Use nanosecond precision to prevent race conditions from concurrent requests
		// NOTE: Requires sudoers entries allowing wildcards:
		//   - lacylights-self-update-* (for Go updates)
		//   - lacylights-mcp-update-* (for MCP updates)
		timestamp := time.Now().Format("20060102-150405.000000")
		var unitName, description string
		if repository == "lacylights-go" {
			unitName = fmt.Sprintf("lacylights-self-update-%s", timestamp)
			description = "LacyLights Self-Update to " + targetVersion
		} else {
			unitName = fmt.Sprintf("lacylights-mcp-update-%s", timestamp)
			description = "LacyLights MCP Update to " + targetVersion
		}

		args := []string{
			"systemd-run",
			"--unit=" + unitName,
			"--description=" + description,
			"--on-active=3s",
			"--timer-property=AccuracySec=100ms",
			"/bin/bash",
			"-c",
			updateCmd,
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

	// Update repositories in order: frontend, MCP, then backend
	// This ensures that if backend (self) update is triggered, other updates complete first
	// The backend update uses systemd-run with delay to avoid connection issues
	var results []*UpdateResult

	for _, repo := range repositoryNames {
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
