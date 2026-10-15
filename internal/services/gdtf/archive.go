// Package gdtf imports GDTF (General Device Type Format, DIN SPEC 15800)
// fixture types as LacyLights fixture definitions.
//
// Only files a user supplies are imported. The package never fetches,
// bundles or caches GDTF Share content.
package gdtf

import (
	"archive/zip"
	"bytes"
	"errors"
	"fmt"
	"io"
	"path"
	"strings"
)

// Archive limits. They bound the work done on a hostile or broken upload.
const (
	// MaxArchiveEntries is the most entries one archive (outer or nested)
	// may contain.
	MaxArchiveEntries = 2000
	// MaxDescriptionBytes is the largest description.xml accepted, measured
	// after decompression.
	MaxDescriptionBytes = 32 << 20
	// MaxNestingDepth is how deep archives may nest: a .gdtf inside a .zip
	// inside the uploaded .zip is depth 2, the deepest allowed.
	MaxNestingDepth = 2
	// MaxNestedArchiveBytes caps the decompressed size of one nested
	// archive, which has to be held in memory to be opened.
	MaxNestedArchiveBytes = 512 << 20
)

// descriptionFileName is the GDTF description at the root of a .gdtf.
const descriptionFileName = "description.xml"

// Archive errors. Returned errors wrap these, so test with errors.Is.
var (
	ErrNotArchive          = errors.New("not a zip archive")
	ErrNoDescription       = errors.New("no description.xml found")
	ErrTooManyEntries      = fmt.Errorf("archive has more than %d entries", MaxArchiveEntries)
	ErrDescriptionTooLarge = fmt.Errorf("description.xml is larger than %d MB", MaxDescriptionBytes>>20)
	ErrNestingTooDeep      = fmt.Errorf("archives nested more than %d levels deep", MaxNestingDepth)
	ErrNestedTooLarge      = fmt.Errorf("nested archive is larger than %d MB", MaxNestedArchiveBytes>>20)
)

// File is one GDTF description to import.
type File struct {
	// FileName names the source: "Foo.gdtf", or "outer.zip/dir/Foo.gdtf"
	// for a .gdtf found inside a .zip.
	FileName string
	// DescriptionXML is the content of the .gdtf's description.xml.
	DescriptionXML string
}

// ExtractDescriptions reads the description.xml of a .gdtf, or of every
// .gdtf inside a .zip (at any depth, with nested .zip files up to
// MaxNestingDepth). Nothing is written to disk.
//
// An archive with description.xml at its root is treated as a .gdtf, so a
// renamed file still works. Otherwise it is searched for .gdtf and .zip
// entries; macOS "__MACOSX/" and "._*" metadata entries are ignored.
//
// Problems with individual entries of a container do not stop the others:
// the files that could be read are returned together with a non-nil error
// that joins one error per failed entry. A nil slice with an error means
// nothing could be read.
func ExtractDescriptions(r io.ReaderAt, size int64, name string) ([]File, error) {
	return extract(r, size, name, 0)
}

func extract(r io.ReaderAt, size int64, name string, depth int) ([]File, error) {
	if depth > MaxNestingDepth {
		return nil, fmt.Errorf("%s: %w", name, ErrNestingTooDeep)
	}
	zr, err := zip.NewReader(r, size)
	if err != nil {
		return nil, fmt.Errorf("%s: %w: %v", name, ErrNotArchive, err)
	}
	if len(zr.File) > MaxArchiveEntries {
		return nil, fmt.Errorf("%s: %w", name, ErrTooManyEntries)
	}

	if desc := rootDescription(zr); desc != nil {
		xmlText, err := readDescription(desc)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", name, err)
		}
		return []File{{FileName: name, DescriptionXML: xmlText}}, nil
	}
	if strings.EqualFold(path.Ext(name), ".gdtf") {
		return nil, fmt.Errorf("%s: %w", name, ErrNoDescription)
	}

	var files []File
	var errs []error
	for _, f := range zr.File {
		if skipEntry(f) {
			continue
		}
		ext := strings.ToLower(path.Ext(f.Name))
		if ext != ".gdtf" && ext != ".zip" {
			continue
		}
		entryName := name + "/" + f.Name
		got, err := extractEntry(f, entryName, depth+1)
		files = append(files, got...)
		if err != nil {
			errs = append(errs, err)
		}
	}
	if len(files) == 0 && len(errs) == 0 {
		return nil, fmt.Errorf("%s: %w (no .gdtf files in archive)", name, ErrNoDescription)
	}
	return files, errors.Join(errs...)
}

// extractEntry loads a nested archive into memory and extracts from it.
func extractEntry(f *zip.File, name string, depth int) ([]File, error) {
	if depth > MaxNestingDepth {
		return nil, fmt.Errorf("%s: %w", name, ErrNestingTooDeep)
	}
	if f.UncompressedSize64 > MaxNestedArchiveBytes {
		return nil, fmt.Errorf("%s: %w", name, ErrNestedTooLarge)
	}
	data, err := readLimited(f, MaxNestedArchiveBytes)
	if err != nil {
		if errors.Is(err, errLimit) {
			return nil, fmt.Errorf("%s: %w", name, ErrNestedTooLarge)
		}
		return nil, fmt.Errorf("%s: %w", name, err)
	}
	return extract(bytes.NewReader(data), int64(len(data)), name, depth)
}

// rootDescription returns the description.xml entry at the archive root.
func rootDescription(zr *zip.Reader) *zip.File {
	for _, f := range zr.File {
		if strings.EqualFold(f.Name, descriptionFileName) {
			return f
		}
	}
	return nil
}

// skipEntry reports directories and macOS resource-fork metadata.
func skipEntry(f *zip.File) bool {
	if f.FileInfo().IsDir() || strings.HasSuffix(f.Name, "/") {
		return true
	}
	if strings.HasPrefix(f.Name, "__MACOSX/") || strings.Contains(f.Name, "/__MACOSX/") {
		return true
	}
	return strings.HasPrefix(path.Base(f.Name), "._")
}

func readDescription(f *zip.File) (string, error) {
	if f.UncompressedSize64 > MaxDescriptionBytes {
		return "", ErrDescriptionTooLarge
	}
	data, err := readLimited(f, MaxDescriptionBytes)
	if err != nil {
		if errors.Is(err, errLimit) {
			return "", ErrDescriptionTooLarge
		}
		return "", err
	}
	return string(data), nil
}

var errLimit = errors.New("entry exceeds size limit")

// readLimited reads a zip entry, failing with errLimit beyond limit bytes.
// The header's size is not trusted: the read itself is bounded.
func readLimited(f *zip.File, limit int64) ([]byte, error) {
	rc, err := f.Open()
	if err != nil {
		return nil, fmt.Errorf("open %s: %w", f.Name, err)
	}
	defer func() { _ = rc.Close() }()
	data, err := io.ReadAll(io.LimitReader(rc, limit+1))
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", f.Name, err)
	}
	if int64(len(data)) > limit {
		return nil, errLimit
	}
	return data, nil
}
