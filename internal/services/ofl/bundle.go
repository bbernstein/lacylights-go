package ofl

import (
	"archive/zip"
	"bytes"
	"embed"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
)

// BundleFileName is the name of the embedded OFL bundle file
const BundleFileName = "ofl-bundle.zip"

//go:embed data/*
var embeddedData embed.FS

// BundleService handles access to the embedded OFL data
type BundleService struct{}

// NewBundleService creates a new bundle service
func NewBundleService() *BundleService {
	return &BundleService{}
}

// HasBundle checks if the embedded OFL bundle exists
func (b *BundleService) HasBundle() bool {
	bundlePath := filepath.Join("data", BundleFileName)
	_, err := embeddedData.Open(bundlePath)
	return err == nil
}

// GetBundleReader returns a zip.Reader for the embedded bundle
func (b *BundleService) GetBundleReader() (*zip.Reader, error) {
	bundlePath := filepath.Join("data", BundleFileName)

	data, err := embeddedData.ReadFile(bundlePath)
	if err != nil {
		return nil, fmt.Errorf("failed to read embedded bundle: %w", err)
	}

	reader := bytes.NewReader(data)
	return zip.NewReader(reader, int64(len(data)))
}

// GetBundleSize returns the size of the embedded bundle in bytes
func (b *BundleService) GetBundleSize() (int64, error) {
	bundlePath := filepath.Join("data", BundleFileName)

	file, err := embeddedData.Open(bundlePath)
	if err != nil {
		return 0, fmt.Errorf("bundle not found: %w", err)
	}
	defer func() { _ = file.Close() }()

	stat, err := file.Stat()
	if err != nil {
		return 0, err
	}

	return stat.Size(), nil
}

// ExtractBundleTo extracts the embedded bundle to a temporary directory
// Returns the path to the extracted directory
func (b *BundleService) ExtractBundleTo(destDir string) error {
	reader, err := b.GetBundleReader()
	if err != nil {
		return err
	}

	for _, file := range reader.File {
		destPath := filepath.Join(destDir, file.Name)

		// Check for zip slip vulnerability
		if !isSubPath(destDir, destPath) {
			return fmt.Errorf("invalid zip entry: %s", file.Name)
		}

		if file.FileInfo().IsDir() {
			if err := os.MkdirAll(destPath, 0755); err != nil {
				return err
			}
			continue
		}

		// Create parent directory
		if err := os.MkdirAll(filepath.Dir(destPath), 0755); err != nil {
			return err
		}

		// Extract file
		if err := extractFile(file, destPath); err != nil {
			return err
		}
	}

	return nil
}

// CopyBundleTo copies the embedded bundle to a file path
func (b *BundleService) CopyBundleTo(destPath string) error {
	bundlePath := filepath.Join("data", BundleFileName)

	data, err := embeddedData.ReadFile(bundlePath)
	if err != nil {
		return fmt.Errorf("failed to read embedded bundle: %w", err)
	}

	// Ensure parent directory exists
	if err := os.MkdirAll(filepath.Dir(destPath), 0755); err != nil {
		return err
	}

	return os.WriteFile(destPath, data, 0644)
}

// ListBundleContents returns a list of file names in the bundle
func (b *BundleService) ListBundleContents() ([]string, error) {
	reader, err := b.GetBundleReader()
	if err != nil {
		return nil, err
	}

	var files []string
	for _, file := range reader.File {
		files = append(files, file.Name)
	}
	return files, nil
}

// isSubPath checks if child is a subdirectory of parent (prevents zip slip)
func isSubPath(parent, child string) bool {
	rel, err := filepath.Rel(parent, child)
	if err != nil {
		return false
	}
	// Handle empty string case
	if len(rel) == 0 {
		return false
	}
	// "." means same directory (valid)
	if rel == "." {
		return true
	}
	// Check for path traversal attempts
	if filepath.IsAbs(rel) {
		return false
	}
	// Check if path starts with ".." which would escape parent
	if rel == ".." || (len(rel) >= 3 && rel[:3] == "../") || (len(rel) >= 3 && rel[:3] == "..\\") {
		return false
	}
	return true
}

// extractFile extracts a single file from the zip
func extractFile(file *zip.File, destPath string) error {
	src, err := file.Open()
	if err != nil {
		return err
	}
	defer func() { _ = src.Close() }()

	dest, err := os.OpenFile(destPath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, file.Mode())
	if err != nil {
		return err
	}
	defer func() { _ = dest.Close() }()

	_, err = io.Copy(dest, src)
	return err
}

// GetEmbeddedFS returns the embedded filesystem for direct access
func (b *BundleService) GetEmbeddedFS() fs.FS {
	return embeddedData
}
