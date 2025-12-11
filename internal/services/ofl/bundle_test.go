package ofl

import (
	"os"
	"path/filepath"
	"testing"
)

func TestBundleService_NewBundleService(t *testing.T) {
	service := NewBundleService()
	if service == nil {
		t.Fatal("NewBundleService returned nil")
	}
}

func TestBundleService_HasBundle(t *testing.T) {
	service := NewBundleService()

	// The bundle may or may not exist depending on whether the download script was run.
	// This test just verifies the method doesn't panic and returns a boolean.
	hasBundle := service.HasBundle()
	t.Logf("HasBundle: %v", hasBundle)
}

func TestBundleService_GetBundleSize_NoBundleError(t *testing.T) {
	service := NewBundleService()

	// If no bundle exists, this should return an error
	if !service.HasBundle() {
		_, err := service.GetBundleSize()
		if err == nil {
			t.Error("GetBundleSize should return error when bundle doesn't exist")
		}
	}
}

func TestBundleService_ListBundleContents_NoBundleError(t *testing.T) {
	service := NewBundleService()

	// If no bundle exists, this should return an error
	if !service.HasBundle() {
		_, err := service.ListBundleContents()
		if err == nil {
			t.Error("ListBundleContents should return error when bundle doesn't exist")
		}
	}
}

func TestBundleService_WithBundle(t *testing.T) {
	service := NewBundleService()

	// Skip if no bundle
	if !service.HasBundle() {
		t.Skip("No bundle available for testing")
	}

	// Test GetBundleSize
	size, err := service.GetBundleSize()
	if err != nil {
		t.Fatalf("GetBundleSize failed: %v", err)
	}
	if size <= 0 {
		t.Error("Bundle size should be > 0")
	}
	t.Logf("Bundle size: %d bytes", size)

	// Test GetBundleReader
	reader, err := service.GetBundleReader()
	if err != nil {
		t.Fatalf("GetBundleReader failed: %v", err)
	}
	if reader == nil {
		t.Fatal("GetBundleReader returned nil reader")
	}
	if len(reader.File) == 0 {
		t.Error("Bundle should contain files")
	}
	t.Logf("Bundle contains %d files", len(reader.File))

	// Test ListBundleContents
	contents, err := service.ListBundleContents()
	if err != nil {
		t.Fatalf("ListBundleContents failed: %v", err)
	}
	if len(contents) == 0 {
		t.Error("Bundle should have contents")
	}
	t.Logf("Bundle has %d entries", len(contents))
}

func TestBundleService_ExtractBundleTo(t *testing.T) {
	service := NewBundleService()

	// Skip if no bundle
	if !service.HasBundle() {
		t.Skip("No bundle available for testing")
	}

	// Create temp directory
	tempDir, err := os.MkdirTemp("", "ofl-bundle-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer func() { _ = os.RemoveAll(tempDir) }()

	// Extract bundle
	err = service.ExtractBundleTo(tempDir)
	if err != nil {
		t.Fatalf("ExtractBundleTo failed: %v", err)
	}

	// Verify something was extracted
	entries, err := os.ReadDir(tempDir)
	if err != nil {
		t.Fatalf("Failed to read temp dir: %v", err)
	}
	if len(entries) == 0 {
		t.Error("Expected files to be extracted")
	}
	t.Logf("Extracted %d entries", len(entries))
}

func TestBundleService_CopyBundleTo(t *testing.T) {
	service := NewBundleService()

	// Skip if no bundle
	if !service.HasBundle() {
		t.Skip("No bundle available for testing")
	}

	// Create temp directory
	tempDir, err := os.MkdirTemp("", "ofl-bundle-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer func() { _ = os.RemoveAll(tempDir) }()

	destPath := filepath.Join(tempDir, "copy.zip")

	// Copy bundle
	err = service.CopyBundleTo(destPath)
	if err != nil {
		t.Fatalf("CopyBundleTo failed: %v", err)
	}

	// Verify file exists and has content
	info, err := os.Stat(destPath)
	if err != nil {
		t.Fatalf("Copied file not found: %v", err)
	}
	if info.Size() == 0 {
		t.Error("Copied file should not be empty")
	}
	t.Logf("Copied bundle size: %d bytes", info.Size())
}

func TestIsSubPath(t *testing.T) {
	tests := []struct {
		name     string
		parent   string
		child    string
		expected bool
	}{
		{
			name:     "valid subpath",
			parent:   "/tmp/extract",
			child:    "/tmp/extract/file.txt",
			expected: true,
		},
		{
			name:     "valid nested subpath",
			parent:   "/tmp/extract",
			child:    "/tmp/extract/subdir/file.txt",
			expected: true,
		},
		{
			name:     "parent traversal attack",
			parent:   "/tmp/extract",
			child:    "/tmp/extract/../../../etc/passwd",
			expected: false,
		},
		{
			name:     "absolute path escape",
			parent:   "/tmp/extract",
			child:    "/etc/passwd",
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isSubPath(tt.parent, tt.child)
			if result != tt.expected {
				t.Errorf("isSubPath(%q, %q) = %v, want %v", tt.parent, tt.child, result, tt.expected)
			}
		})
	}
}
