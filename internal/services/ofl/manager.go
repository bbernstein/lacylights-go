package ofl

import (
	"archive/zip"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/bbernstein/lacylights-go/internal/database/models"
	"github.com/bbernstein/lacylights-go/internal/database/repositories"
	"github.com/bbernstein/lacylights-go/internal/services/pubsub"
	"gorm.io/gorm"
)

// ImportOptions contains options for an OFL import
type ImportOptions struct {
	ForceReimport       bool     // Force reimport even if unchanged
	UpdateInUseFixtures bool     // Update fixtures currently in use
	Manufacturers       []string // Only import specific manufacturers (empty = all)
	PreferBundled       bool     // Prefer bundled data over fetching
}

// ImportStats contains statistics about an import
type ImportStats struct {
	TotalProcessed    int
	SuccessfulImports int
	FailedImports     int
	SkippedDuplicates int
	UpdatedFixtures   int
	DurationSeconds   float64
}

// ImportResult contains the final result of an import
type ImportResult struct {
	Success      bool
	Stats        ImportStats
	ErrorMessage string
	OFLVersion   string
}

// Manager orchestrates OFL import operations
type Manager struct {
	db             *gorm.DB
	fixtureRepo    *repositories.FixtureRepository
	service        *Service
	bundleService  *BundleService
	updatesService *UpdatesService
	statusTracker  *StatusTracker
	pubsub         *pubsub.PubSub
	cachePath      string
	httpClient     *http.Client

	// Import state
	importMu     sync.Mutex
	cancelImport context.CancelFunc
}

// NewManager creates a new OFL manager
func NewManager(
	db *gorm.DB,
	fixtureRepo *repositories.FixtureRepository,
	ps *pubsub.PubSub,
	cachePath string,
) *Manager {
	statusTracker := NewStatusTracker()

	m := &Manager{
		db:             db,
		fixtureRepo:    fixtureRepo,
		service:        NewService(db, fixtureRepo),
		bundleService:  NewBundleService(),
		updatesService: NewUpdatesService(db, fixtureRepo),
		statusTracker:  statusTracker,
		pubsub:         ps,
		cachePath:      cachePath,
		httpClient: &http.Client{
			Timeout: 5 * time.Minute,
		},
	}

	// Subscribe to status changes and publish to pubsub
	statusTracker.Subscribe(func(status *ProgressStatus) {
		if ps != nil {
			ps.PublishAll(pubsub.TopicOFLImportProgress, status)
		}
	})

	return m
}

// GetStatus returns the current import status
func (m *Manager) GetStatus() ProgressStatus {
	return m.statusTracker.GetStatus()
}

// SubscribeStatus subscribes to status updates
func (m *Manager) SubscribeStatus(callback StatusCallback) func() {
	return m.statusTracker.Subscribe(callback)
}

// TriggerImport starts an OFL import operation
// Returns immediately; import runs in background
func (m *Manager) TriggerImport(ctx context.Context, opts *ImportOptions) (*ImportResult, error) {
	// Check if already importing
	if !m.importMu.TryLock() {
		return nil, fmt.Errorf("an import is already in progress")
	}

	// Create cancellable context
	importCtx, cancel := context.WithCancel(ctx)
	m.cancelImport = cancel

	// Default options
	if opts == nil {
		opts = &ImportOptions{}
	}

	// Run import in goroutine
	result, err := m.runImport(importCtx, opts)

	m.importMu.Unlock()
	m.cancelImport = nil

	return result, err
}

// CancelImport cancels any ongoing import
func (m *Manager) CancelImport() bool {
	if m.cancelImport != nil {
		m.cancelImport()
		return true
	}
	return false
}

// CheckForUpdates checks for available OFL updates without importing
func (m *Manager) CheckForUpdates(ctx context.Context) (*UpdateCheckResult, error) {
	log.Println("Checking for OFL updates...")

	// Get current fixture hashes
	currentHashes, err := m.updatesService.GetCurrentFixtureHashes(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get current hashes: %w", err)
	}

	// Get instance counts
	instanceCounts, err := m.updatesService.GetFixtureInstanceCounts(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get instance counts: %w", err)
	}

	// Get current count
	currentCount, err := m.fixtureRepo.CountDefinitions(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to count definitions: %w", err)
	}

	// Get OFL data (prefer bundled for speed)
	var zipReader *zip.Reader
	var oflVersion string

	if m.bundleService.HasBundle() {
		zipReader, err = m.bundleService.GetBundleReader()
		if err != nil {
			return nil, fmt.Errorf("failed to read bundle: %w", err)
		}
		oflVersion = "bundled"
	} else {
		// Download from GitHub
		zipPath := filepath.Join(m.cachePath, "ofl-check.zip")
		if err := m.downloadOFLZip(ctx, zipPath); err != nil {
			return nil, fmt.Errorf("failed to download OFL: %w", err)
		}
		defer func() { _ = os.Remove(zipPath) }()

		zipFile, err := zip.OpenReader(zipPath)
		if err != nil {
			return nil, fmt.Errorf("failed to open zip: %w", err)
		}
		defer func() { _ = zipFile.Close() }()

		zipReader = &zipFile.Reader
		oflVersion = "master"
	}

	// Parse manufacturers
	manufacturers, err := m.parseManufacturers(zipReader)
	if err != nil {
		return nil, fmt.Errorf("failed to parse manufacturers: %w", err)
	}

	// Scan fixtures and check for changes
	result := &UpdateCheckResult{
		CurrentFixtureCount: int(currentCount),
		OFLVersion:          oflVersion,
		CheckedAt:           time.Now(),
		FixtureUpdates:      make([]FixtureUpdate, 0),
	}

	oflCount := 0
	for _, f := range zipReader.File {
		parts := strings.Split(f.Name, "/")
		if len(parts) != 4 || parts[1] != "fixtures" || !strings.HasSuffix(f.Name, ".json") {
			continue
		}
		if parts[3] == ManufacturersFile {
			continue
		}

		oflCount++
		manufacturerKey := parts[2]
		mfg, ok := manufacturers[manufacturerKey]
		manufacturerName := manufacturerKey
		if ok && mfg.Name != "" {
			manufacturerName = mfg.Name
		}

		// Read fixture JSON
		rc, err := f.Open()
		if err != nil {
			continue
		}
		data, err := io.ReadAll(rc)
		_ = rc.Close()
		if err != nil {
			continue
		}

		// Parse to get model name
		var fixture struct {
			Name string `json:"name"`
		}
		if err := json.Unmarshal(data, &fixture); err != nil {
			continue
		}

		// Compute hash
		newHash := ComputeFixtureHash(string(data))

		// Compare
		update, err := m.updatesService.CompareFixture(
			ctx, manufacturerName, fixture.Name, newHash, currentHashes, instanceCounts,
		)
		if err != nil {
			continue
		}

		switch update.ChangeType {
		case ChangeTypeNew:
			result.NewFixtureCount++
			result.FixtureUpdates = append(result.FixtureUpdates, *update)
		case ChangeTypeUpdated:
			result.ChangedFixtureCount++
			if update.IsInUse {
				result.ChangedInUseCount++
			}
			result.FixtureUpdates = append(result.FixtureUpdates, *update)
		}

		// Limit the number of updates returned
		if len(result.FixtureUpdates) >= 100 {
			break
		}
	}

	result.OFLFixtureCount = oflCount
	return result, nil
}

// runImport performs the actual import
func (m *Manager) runImport(ctx context.Context, opts *ImportOptions) (*ImportResult, error) {
	startTime := time.Now()
	var stats ImportStats

	// Determine data source
	useBundled := opts.PreferBundled && m.bundleService.HasBundle()
	if !useBundled && !m.bundleService.HasBundle() {
		// No bundle, must fetch
		useBundled = false
	} else if opts.PreferBundled {
		useBundled = true
	} else if m.bundleService.HasBundle() {
		// Default: try to fetch, fall back to bundle
		useBundled = false
	}

	m.statusTracker.Start("master", useBundled)

	// Get ZIP data
	var zipReader *zip.Reader

	if useBundled {
		m.statusTracker.SetPhase(PhaseExtracting)
		var err error
		zipReader, err = m.bundleService.GetBundleReader()
		if err != nil {
			m.statusTracker.Fail(err)
			return nil, fmt.Errorf("failed to read bundle: %w", err)
		}
	} else {
		m.statusTracker.SetPhase(PhaseDownloading)
		zipPath := filepath.Join(m.cachePath, "ofl-import.zip")
		if err := m.downloadOFLZip(ctx, zipPath); err != nil {
			// Check if cancelled
			if ctx.Err() != nil {
				m.statusTracker.Cancel()
				return nil, ctx.Err()
			}
			m.statusTracker.Fail(err)
			return nil, fmt.Errorf("failed to download OFL: %w", err)
		}
		defer func() { _ = os.Remove(zipPath) }()

		m.statusTracker.SetPhase(PhaseExtracting)
		zipFile, err := zip.OpenReader(zipPath)
		if err != nil {
			m.statusTracker.Fail(err)
			return nil, fmt.Errorf("failed to open zip: %w", err)
		}
		defer func() { _ = zipFile.Close() }()
		zipReader = &zipFile.Reader
	}

	m.statusTracker.SetPhase(PhaseParsing)

	// Parse manufacturers
	manufacturers, err := m.parseManufacturers(zipReader)
	if err != nil {
		m.statusTracker.Fail(err)
		return nil, fmt.Errorf("failed to parse manufacturers: %w", err)
	}

	// Get current hashes for change detection
	currentHashes, err := m.updatesService.GetCurrentFixtureHashes(ctx)
	if err != nil {
		m.statusTracker.Fail(err)
		return nil, fmt.Errorf("failed to get current hashes: %w", err)
	}

	// Get instance counts
	instanceCounts, err := m.updatesService.GetFixtureInstanceCounts(ctx)
	if err != nil {
		m.statusTracker.Fail(err)
		return nil, fmt.Errorf("failed to get instance counts: %w", err)
	}

	// Find all fixture files
	var fixtureFiles []*zip.File
	for _, f := range zipReader.File {
		parts := strings.Split(f.Name, "/")
		if len(parts) != 4 || parts[1] != "fixtures" || !strings.HasSuffix(f.Name, ".json") {
			continue
		}
		if parts[3] == ManufacturersFile {
			continue
		}

		// Filter by manufacturer if specified
		if len(opts.Manufacturers) > 0 {
			manufacturerKey := parts[2]
			found := false
			for _, m := range opts.Manufacturers {
				if strings.EqualFold(manufacturerKey, m) {
					found = true
					break
				}
			}
			if !found {
				continue
			}
		}

		fixtureFiles = append(fixtureFiles, f)
	}

	m.statusTracker.SetTotalFixtures(len(fixtureFiles))
	m.statusTracker.SetPhase(PhaseImporting)

	// Import fixtures with concurrency
	var (
		successCount int64
		failCount    int64
		skipCount    int64
		updateCount  int64
		wg           sync.WaitGroup
	)

	sem := make(chan struct{}, 10) // Limit concurrency

	for _, f := range fixtureFiles {
		// Check for cancellation
		select {
		case <-ctx.Done():
			m.statusTracker.Cancel()
			return nil, ctx.Err()
		default:
		}

		wg.Add(1)
		go func(file *zip.File) {
			defer wg.Done()

			sem <- struct{}{}
			defer func() { <-sem }()

			parts := strings.Split(file.Name, "/")
			manufacturerKey := parts[2]
			fixtureFileName := parts[3]

			mfg, ok := manufacturers[manufacturerKey]
			manufacturerName := manufacturerKey
			if ok && mfg.Name != "" {
				manufacturerName = mfg.Name
			}

			m.statusTracker.SetCurrentFixture(manufacturerName, strings.TrimSuffix(fixtureFileName, ".json"))

			// Read fixture JSON
			rc, err := file.Open()
			if err != nil {
				atomic.AddInt64(&failCount, 1)
				m.statusTracker.IncrementFailed()
				return
			}
			defer func() { _ = rc.Close() }()

			data, err := io.ReadAll(rc)
			if err != nil {
				atomic.AddInt64(&failCount, 1)
				m.statusTracker.IncrementFailed()
				return
			}

			jsonData := string(data)
			newHash := ComputeFixtureHash(jsonData)

			// Check if we should skip
			key := fmt.Sprintf("%s/%s", manufacturerName, strings.TrimSuffix(fixtureFileName, ".json"))
			currentHash, exists := currentHashes[key]

			if exists && !opts.ForceReimport {
				if currentHash == newHash {
					// Unchanged, skip
					atomic.AddInt64(&skipCount, 1)
					m.statusTracker.IncrementSkipped()
					return
				}

				// Changed - check if in use
				defID, _ := m.updatesService.GetDefinitionIDByManufacturerModel(ctx, manufacturerName, strings.TrimSuffix(fixtureFileName, ".json"))
				if defID != "" {
					count, ok := instanceCounts[defID]
					if ok && count > 0 && !opts.UpdateInUseFixtures {
						// In use and not allowed to update
						atomic.AddInt64(&skipCount, 1)
						m.statusTracker.IncrementSkipped()
						return
					}
				}
			}

			// Import the fixture
			_, err = m.service.ImportFixtureWithHash(ctx, manufacturerName, jsonData, newHash, "master", exists)
			if err != nil {
				if strings.HasPrefix(err.Error(), "FIXTURE_EXISTS:") {
					atomic.AddInt64(&skipCount, 1)
					m.statusTracker.IncrementSkipped()
					return
				}
				atomic.AddInt64(&failCount, 1)
				m.statusTracker.IncrementFailed()
				return
			}

			if exists {
				atomic.AddInt64(&updateCount, 1)
			}
			atomic.AddInt64(&successCount, 1)
			m.statusTracker.IncrementImported()
		}(f)
	}

	wg.Wait()

	// Calculate stats
	stats = ImportStats{
		TotalProcessed:    len(fixtureFiles),
		SuccessfulImports: int(successCount),
		FailedImports:     int(failCount),
		SkippedDuplicates: int(skipCount),
		UpdatedFixtures:   int(updateCount),
		DurationSeconds:   time.Since(startTime).Seconds(),
	}

	// Save import metadata
	m.saveImportMeta(ctx, stats, useBundled)

	m.statusTracker.Complete()

	return &ImportResult{
		Success:    true,
		Stats:      stats,
		OFLVersion: "master",
	}, nil
}

// downloadOFLZip downloads the OFL repository as a zipball
func (m *Manager) downloadOFLZip(ctx context.Context, destPath string) error {
	log.Println("Downloading Open Fixture Library from GitHub...")

	req, err := http.NewRequestWithContext(ctx, "GET", OFLZipballURL, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/vnd.github.v3+json")
	req.Header.Set("User-Agent", "LacyLights-Go")

	resp, err := m.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	// Ensure cache directory exists
	if err := os.MkdirAll(filepath.Dir(destPath), 0755); err != nil {
		return err
	}

	out, err := os.Create(destPath)
	if err != nil {
		return err
	}
	defer func() { _ = out.Close() }()

	written, err := io.Copy(out, resp.Body)
	if err != nil {
		return err
	}

	log.Printf("Downloaded %.2f MB", float64(written)/(1024*1024))
	return nil
}

// parseManufacturers finds and parses the manufacturers.json from the zip
func (m *Manager) parseManufacturers(zipReader *zip.Reader) (map[string]Manufacturer, error) {
	for _, f := range zipReader.File {
		if strings.HasSuffix(f.Name, "fixtures/"+ManufacturersFile) {
			rc, err := f.Open()
			if err != nil {
				return nil, err
			}
			defer func() { _ = rc.Close() }()

			data, err := io.ReadAll(rc)
			if err != nil {
				return nil, err
			}

			var rawManufacturers map[string]json.RawMessage
			if err := json.Unmarshal(data, &rawManufacturers); err != nil {
				return nil, err
			}

			manufacturers := make(map[string]Manufacturer)
			for key, raw := range rawManufacturers {
				if strings.HasPrefix(key, "$") {
					continue
				}

				var mfg Manufacturer
				if err := json.Unmarshal(raw, &mfg); err != nil {
					continue
				}
				manufacturers[key] = mfg
			}

			return manufacturers, nil
		}
	}

	return nil, fmt.Errorf("manufacturers.json not found in archive")
}

// saveImportMeta saves import metadata to the database
func (m *Manager) saveImportMeta(ctx context.Context, stats ImportStats, usedBundled bool) {
	meta := &models.OFLImportMeta{
		ID:                fmt.Sprintf("ofl-import-%d", time.Now().UnixNano()),
		OFLVersion:        "master",
		StartedAt:         time.Now().Add(-time.Duration(stats.DurationSeconds) * time.Second),
		CompletedAt:       time.Now(),
		TotalFixtures:     stats.TotalProcessed,
		SuccessfulImports: stats.SuccessfulImports,
		FailedImports:     stats.FailedImports,
		SkippedDuplicates: stats.SkippedDuplicates,
		UpdatedFixtures:   stats.UpdatedFixtures,
		UsedBundledData:   usedBundled,
	}

	if err := m.db.WithContext(ctx).Create(meta).Error; err != nil {
		log.Printf("Warning: failed to save import metadata: %v", err)
	}
}
