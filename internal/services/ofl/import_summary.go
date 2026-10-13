package ofl

import (
	"errors"
	"fmt"
	"log"
	"sort"
	"strings"
	"sync"
)

// Sentinel errors returned by the OFL importer. They let callers (the
// startup loader and the manager) group failures by cause instead of
// pattern-matching error strings.
var (
	// ErrNoChannels means the fixture defines no usable channels at all.
	// Such fixtures are skipped (not failed): there is nothing to patch.
	ErrNoChannels = errors.New("OFL fixture defines no channels")
	// ErrFixtureRedirect means the file is an OFL redirect (a renamed
	// fixture, or the same fixture sold under another brand). The target
	// fixture is imported from its own file, so redirects are skipped.
	ErrFixtureRedirect = errors.New("OFL fixture redirect")
	// ErrUnknownModeChannel means a mode references a channel key that is
	// neither an available channel, a fine-channel alias, a switching
	// channel alias nor a resolvable matrix template channel.
	ErrUnknownModeChannel = errors.New("mode references unknown channel")
	// ErrUnsupportedModeChannel means a mode channel entry has a shape the
	// importer does not understand (e.g. an unknown insert block).
	ErrUnsupportedModeChannel = errors.New("unsupported mode channel entry")
	// ErrReadFixture means the fixture file could not be read from the
	// OFL archive.
	ErrReadFixture = errors.New("failed to read fixture file")
)

// Import failure categories reported by ImportSummary.
const (
	CategoryNoChannels         = "no channels (skipped)"
	CategoryRedirect           = "redirect (skipped)"
	CategoryUnknownModeChannel = "mode references unknown channel"
	CategoryUnsupportedEntry   = "unsupported mode channel entry"
	CategoryInvalidJSON        = "invalid JSON"
	CategoryFileIO             = "file read error"
	CategoryValidation         = "validation"
	CategoryDatabase           = "database"
	CategoryOther              = "other"
)

// ClassifyImportError maps an importer error to a summary category.
func ClassifyImportError(err error) string {
	switch {
	case err == nil:
		return ""
	case errors.Is(err, ErrNoChannels):
		return CategoryNoChannels
	case errors.Is(err, ErrFixtureRedirect):
		return CategoryRedirect
	case errors.Is(err, ErrUnknownModeChannel):
		return CategoryUnknownModeChannel
	case errors.Is(err, ErrUnsupportedModeChannel):
		return CategoryUnsupportedEntry
	case errors.Is(err, ErrReadFixture):
		return CategoryFileIO
	}
	msg := err.Error()
	switch {
	case strings.HasPrefix(msg, "invalid JSON"):
		return CategoryInvalidJSON
	case strings.HasPrefix(msg, "OFL fixture must have"):
		return CategoryValidation
	case strings.HasPrefix(msg, "failed to"):
		return CategoryDatabase
	}
	return CategoryOther
}

// IsSkippableImportError reports whether err means the fixture was
// intentionally skipped rather than failed (duplicates, redirects, no
// channels).
func IsSkippableImportError(err error) bool {
	if err == nil {
		return false
	}
	return errors.Is(err, ErrNoChannels) || errors.Is(err, ErrFixtureRedirect) ||
		strings.HasPrefix(err.Error(), "FIXTURE_EXISTS:")
}

// ImportSummary collects per-fixture import outcomes so a bulk import can
// log one summary listing every failure grouped by category. It is safe for
// concurrent use.
type ImportSummary struct {
	mu         sync.Mutex
	imported   int
	skipped    int
	failed     int
	byCategory map[string][]string
}

// NewImportSummary returns an empty summary.
func NewImportSummary() *ImportSummary {
	return &ImportSummary{byCategory: map[string][]string{}}
}

// Record adds the outcome of importing one fixture. Duplicate
// (FIXTURE_EXISTS) results count as skipped without being listed; redirects
// and fixtures with no channels count as skipped and are listed under their
// category.
func (s *ImportSummary) Record(fixture string, err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	switch {
	case err == nil:
		s.imported++
	case strings.HasPrefix(err.Error(), "FIXTURE_EXISTS:"):
		s.skipped++
	case errors.Is(err, ErrNoChannels), errors.Is(err, ErrFixtureRedirect):
		s.skipped++
		c := ClassifyImportError(err)
		s.byCategory[c] = append(s.byCategory[c], fmt.Sprintf("%s: %v", fixture, err))
	default:
		s.failed++
		c := ClassifyImportError(err)
		s.byCategory[c] = append(s.byCategory[c], fmt.Sprintf("%s: %v", fixture, err))
	}
}

// RecordSkipped counts a fixture that was skipped before import was
// attempted (e.g. unchanged since the last import). It is not listed.
func (s *ImportSummary) RecordSkipped() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.skipped++
}

// Counts returns the imported, skipped and failed totals.
func (s *ImportSummary) Counts() (imported, skipped, failed int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.imported, s.skipped, s.failed
}

// Categories returns the categories that have at least one entry.
func (s *ImportSummary) Categories() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	cats := make([]string, 0, len(s.byCategory))
	for c := range s.byCategory {
		cats = append(cats, c)
	}
	sort.Strings(cats)
	return cats
}

// Examples returns every entry recorded for a category, sorted.
func (s *ImportSummary) Examples(category string) []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := append([]string(nil), s.byCategory[category]...)
	sort.Strings(out)
	return out
}

// String returns a one-line summary with per-category counts.
func (s *ImportSummary) String() string {
	imported, skipped, failed := s.Counts()
	var parts []string
	for _, c := range s.Categories() {
		parts = append(parts, fmt.Sprintf("%s=%d", c, len(s.Examples(c))))
	}
	line := fmt.Sprintf("imported=%d skipped=%d failed=%d", imported, skipped, failed)
	if len(parts) > 0 {
		line += " [" + strings.Join(parts, ", ") + "]"
	}
	return line
}

// Log writes the summary and every failure, grouped by category.
func (s *ImportSummary) Log(prefix string) {
	log.Printf("%s summary: %s", prefix, s.String())
	for _, c := range s.Categories() {
		entries := s.Examples(c)
		log.Printf("%s %s (%d):", prefix, c, len(entries))
		for _, e := range entries {
			log.Printf("%s   - %s", prefix, e)
		}
	}
}
