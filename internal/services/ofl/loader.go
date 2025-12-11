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

	"github.com/bbernstein/lacylights-go/internal/database/repositories"
	"gorm.io/gorm"
)

const (
	// OFLRepoURL is the GitHub repository URL for the Open Fixture Library
	OFLRepoURL = "https://github.com/OpenLightingProject/open-fixture-library"
	// OFLZipballURL is the URL to download the latest OFL repository as a zipball
	OFLZipballURL = "https://api.github.com/repos/OpenLightingProject/open-fixture-library/zipball/master"
	// ManufacturersFile is the name of the manufacturers JSON file
	ManufacturersFile = "manufacturers.json"
	// ImportStatusFile is the name of the file that tracks import status
	ImportStatusFile = "import_status.json"
)

// Manufacturer represents a manufacturer entry from the OFL manufacturers.json
type Manufacturer struct {
	Name    string `json:"name"`
	Website string `json:"website,omitempty"`
	RDMId   int    `json:"rdmId,omitempty"`
	Comment string `json:"comment,omitempty"`
}

// ImportStatus tracks the status of the OFL import
type ImportStatus struct {
	LastImportTime    time.Time `json:"lastImportTime"`
	TotalFixtures     int       `json:"totalFixtures"`
	SuccessfulImports int       `json:"successfulImports"`
	FailedImports     int       `json:"failedImports"`
	Version           string    `json:"version"`
}

// Loader handles downloading and importing fixtures from the Open Fixture Library
type Loader struct {
	db          *gorm.DB
	fixtureRepo *repositories.FixtureRepository
	service     *Service
	cachePath   string
	httpClient  *http.Client
}

// NewLoader creates a new OFL Loader
func NewLoader(db *gorm.DB, fixtureRepo *repositories.FixtureRepository, cachePath string) *Loader {
	return &Loader{
		db:          db,
		fixtureRepo: fixtureRepo,
		service:     NewService(db, fixtureRepo),
		cachePath:   cachePath,
		httpClient: &http.Client{
			Timeout: 5 * time.Minute, // Allow long timeout for large download
		},
	}
}

// NeedsImport checks if we need to import OFL fixtures
// Returns true if the database has no fixture definitions
func (l *Loader) NeedsImport(ctx context.Context) (bool, error) {
	count, err := l.fixtureRepo.CountDefinitions(ctx)
	if err != nil {
		return false, fmt.Errorf("failed to count fixture definitions: %w", err)
	}
	return count == 0, nil
}

// LoadAll downloads and imports all fixtures from the Open Fixture Library
// This is intended to be called on first startup when the database is empty
func (l *Loader) LoadAll(ctx context.Context) (*ImportStatus, error) {
	log.Println("📦 Starting Open Fixture Library import...")

	// Ensure cache directory exists
	if err := os.MkdirAll(l.cachePath, 0755); err != nil {
		return nil, fmt.Errorf("failed to create cache directory: %w", err)
	}

	// Download the OFL zipball
	zipPath := filepath.Join(l.cachePath, "ofl.zip")
	if err := l.downloadOFLZip(ctx, zipPath); err != nil {
		return nil, fmt.Errorf("failed to download OFL: %w", err)
	}
	defer func() { _ = os.Remove(zipPath) }()

	// Extract and import fixtures
	status, err := l.importFromZip(ctx, zipPath)
	if err != nil {
		return nil, fmt.Errorf("failed to import fixtures: %w", err)
	}

	// Save import status
	if err := l.saveImportStatus(status); err != nil {
		log.Printf("Warning: failed to save import status: %v", err)
	}

	return status, nil
}

// downloadOFLZip downloads the OFL repository as a zipball
func (l *Loader) downloadOFLZip(ctx context.Context, destPath string) error {
	log.Println("📥 Downloading Open Fixture Library from GitHub...")

	req, err := http.NewRequestWithContext(ctx, "GET", OFLZipballURL, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/vnd.github.v3+json")
	req.Header.Set("User-Agent", "LacyLights-Go")

	resp, err := l.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	// Create destination file
	out, err := os.Create(destPath)
	if err != nil {
		return err
	}
	defer func() { _ = out.Close() }()

	// Copy with progress reporting
	written, err := io.Copy(out, resp.Body)
	if err != nil {
		return err
	}

	log.Printf("📥 Downloaded %.2f MB", float64(written)/(1024*1024))
	return nil
}

// importFromZip extracts fixtures from the downloaded zip and imports them
func (l *Loader) importFromZip(ctx context.Context, zipPath string) (*ImportStatus, error) {
	log.Println("📦 Extracting and importing fixtures...")

	zipReader, err := zip.OpenReader(zipPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open zip: %w", err)
	}
	defer func() { _ = zipReader.Close() }()

	// First, find and parse manufacturers.json
	manufacturers, err := l.findAndParseManufacturers(zipReader)
	if err != nil {
		return nil, fmt.Errorf("failed to parse manufacturers: %w", err)
	}
	log.Printf("📋 Found %d manufacturers", len(manufacturers))

	// Find all fixture JSON files
	var fixtureFiles []*zip.File
	var rootDir string

	for _, f := range zipReader.File {
		// The zipball has a root directory like "OpenLightingProject-open-fixture-library-xxxxxxx/"
		parts := strings.Split(f.Name, "/")
		if len(parts) < 3 {
			continue
		}

		if rootDir == "" {
			rootDir = parts[0]
		}

		// Look for files in fixtures/<manufacturer>/<fixture>.json
		if parts[1] == "fixtures" && len(parts) == 4 && strings.HasSuffix(f.Name, ".json") {
			// Skip manufacturers.json
			if parts[3] == ManufacturersFile {
				continue
			}
			fixtureFiles = append(fixtureFiles, f)
		}
	}

	log.Printf("📋 Found %d fixture files to import", len(fixtureFiles))

	// Import fixtures with concurrency
	status := &ImportStatus{
		LastImportTime: time.Now(),
		TotalFixtures:  len(fixtureFiles),
		Version:        "master",
	}

	var (
		successCount int64
		failCount    int64
		wg           sync.WaitGroup
	)

	// Use a semaphore to limit concurrency (database operations)
	sem := make(chan struct{}, 10)

	for _, f := range fixtureFiles {
		wg.Add(1)
		go func(file *zip.File) {
			defer wg.Done()

			sem <- struct{}{}        // Acquire
			defer func() { <-sem }() // Release

			// Extract manufacturer key from path
			parts := strings.Split(file.Name, "/")
			manufacturerKey := parts[2]
			fixtureFileName := parts[3]

			// Get manufacturer name
			mfg, ok := manufacturers[manufacturerKey]
			manufacturerName := manufacturerKey
			if ok && mfg.Name != "" {
				manufacturerName = mfg.Name
			}

			// Read fixture JSON
			rc, err := file.Open()
			if err != nil {
				log.Printf("⚠️  Failed to open %s: %v", fixtureFileName, err)
				atomic.AddInt64(&failCount, 1)
				return
			}
			defer func() { _ = rc.Close() }()

			data, err := io.ReadAll(rc)
			if err != nil {
				log.Printf("⚠️  Failed to read %s: %v", fixtureFileName, err)
				atomic.AddInt64(&failCount, 1)
				return
			}

			// Import the fixture
			_, err = l.service.ImportFixture(ctx, manufacturerName, string(data), false)
			if err != nil {
				// Check if it's a duplicate (not an error, just skip)
				if strings.HasPrefix(err.Error(), "FIXTURE_EXISTS:") {
					return
				}
				// Log other errors but don't spam
				if atomic.LoadInt64(&failCount) < 10 {
					log.Printf("⚠️  Failed to import %s/%s: %v", manufacturerName, fixtureFileName, err)
				}
				atomic.AddInt64(&failCount, 1)
				return
			}

			atomic.AddInt64(&successCount, 1)
			current := atomic.LoadInt64(&successCount)
			if current%100 == 0 {
				log.Printf("✅ Imported %d fixtures...", current)
			}
		}(f)
	}

	wg.Wait()

	status.SuccessfulImports = int(successCount)
	status.FailedImports = int(failCount)

	log.Printf("✅ OFL import complete: %d successful, %d failed, %d total",
		status.SuccessfulImports, status.FailedImports, status.TotalFixtures)

	return status, nil
}

// findAndParseManufacturers finds and parses the manufacturers.json file from the zip
func (l *Loader) findAndParseManufacturers(zipReader *zip.ReadCloser) (map[string]Manufacturer, error) {
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

			// First, unmarshal into a generic map to handle mixed value types
			// (the OFL manufacturers.json has a "$schema" key with a string value)
			var rawManufacturers map[string]json.RawMessage
			if err := json.Unmarshal(data, &rawManufacturers); err != nil {
				return nil, err
			}

			manufacturers := make(map[string]Manufacturer)
			for key, raw := range rawManufacturers {
				// Skip special keys like $schema
				if strings.HasPrefix(key, "$") {
					continue
				}

				var mfg Manufacturer
				if err := json.Unmarshal(raw, &mfg); err != nil {
					// If it's not a valid manufacturer object, skip it
					log.Printf("⚠️  Skipping invalid manufacturer entry: %s", key)
					continue
				}
				manufacturers[key] = mfg
			}

			return manufacturers, nil
		}
	}

	return nil, fmt.Errorf("manufacturers.json not found in archive")
}

// saveImportStatus saves the import status to the cache directory
func (l *Loader) saveImportStatus(status *ImportStatus) error {
	data, err := json.MarshalIndent(status, "", "  ")
	if err != nil {
		return err
	}

	statusPath := filepath.Join(l.cachePath, ImportStatusFile)
	return os.WriteFile(statusPath, data, 0644)
}

// LoadImportStatus loads the last import status from the cache
func (l *Loader) LoadImportStatus() (*ImportStatus, error) {
	statusPath := filepath.Join(l.cachePath, ImportStatusFile)
	data, err := os.ReadFile(statusPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	var status ImportStatus
	if err := json.Unmarshal(data, &status); err != nil {
		return nil, err
	}

	return &status, nil
}
