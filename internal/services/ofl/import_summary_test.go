package ofl

import (
	"bytes"
	"errors"
	"fmt"
	"log"
	"strings"
	"sync"
	"testing"
)

func TestClassifyImportError(t *testing.T) {
	tests := []struct {
		err  error
		want string
	}{
		{nil, ""},
		{fmt.Errorf("x: %w", ErrNoChannels), CategoryNoChannels},
		{fmt.Errorf("%w to a/b", ErrFixtureRedirect), CategoryRedirect},
		{fmt.Errorf("mode: %w", ErrUnknownModeChannel), CategoryUnknownModeChannel},
		{fmt.Errorf("mode: %w", ErrUnsupportedModeChannel), CategoryUnsupportedEntry},
		{fmt.Errorf("%w: open: boom", ErrReadFixture), CategoryFileIO},
		{errors.New("invalid JSON: boom"), CategoryInvalidJSON},
		{errors.New(`OFL fixture must have a "name" field`), CategoryValidation},
		{errors.New("failed to create channels: locked"), CategoryDatabase},
		{errors.New("error checking existing fixture: locked"), CategoryDatabase},
		{errors.New("something else"), CategoryOther},
	}
	for _, tt := range tests {
		if got := ClassifyImportError(tt.err); got != tt.want {
			t.Errorf("ClassifyImportError(%v) = %q, want %q", tt.err, got, tt.want)
		}
	}
}

func TestIsSkippableImportError(t *testing.T) {
	if IsSkippableImportError(nil) {
		t.Error("nil should not be skippable")
	}
	for _, err := range []error{
		errors.New("FIXTURE_EXISTS:Generic Par:0"),
		fmt.Errorf("x: %w", ErrNoChannels),
		fmt.Errorf("%w to a/b", ErrFixtureRedirect),
	} {
		if !IsSkippableImportError(err) {
			t.Errorf("%v should be skippable", err)
		}
	}
	if IsSkippableImportError(fmt.Errorf("x: %w", ErrUnknownModeChannel)) {
		t.Error("unknown channel should not be skippable")
	}
}

func TestImportSummary(t *testing.T) {
	s := NewImportSummary()
	var wg sync.WaitGroup
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			s.Record(fmt.Sprintf("ok/%d", i), nil)
		}(i)
	}
	wg.Wait()
	s.Record("dup/a", errors.New("FIXTURE_EXISTS:A B:0"))
	s.Record("redir/a", fmt.Errorf("%w to x/y", ErrFixtureRedirect))
	s.Record("bad/b", fmt.Errorf("mode %q: %w", "m", ErrUnknownModeChannel))
	s.Record("bad/a", errors.New("invalid JSON: nope"))
	s.RecordSkipped()

	imported, skipped, failed := s.Counts()
	if imported != 5 || skipped != 3 || failed != 2 {
		t.Errorf("counts = %d/%d/%d, want 5/3/2", imported, skipped, failed)
	}
	if got := fmt.Sprint(s.Categories()); got != "[invalid JSON mode references unknown channel redirect (skipped)]" {
		t.Errorf("categories = %s", got)
	}
	if got := s.Examples(CategoryUnknownModeChannel); len(got) != 1 || !strings.HasPrefix(got[0], "bad/b: ") {
		t.Errorf("examples = %v", got)
	}
	want := "imported=5 skipped=3 failed=2 [invalid JSON=1, mode references unknown channel=1, redirect (skipped)=1]"
	if got := s.String(); got != want {
		t.Errorf("String() = %q, want %q", got, want)
	}

	var buf bytes.Buffer
	prevOut, prevFlags := log.Writer(), log.Flags()
	log.SetOutput(&buf)
	log.SetFlags(0)
	defer func() { log.SetOutput(prevOut); log.SetFlags(prevFlags) }()
	s.Log("OFL import")
	out := buf.String()
	for _, want := range []string{"OFL import summary: imported=5", "OFL import invalid JSON (1):", "  - bad/a: invalid JSON: nope", "  - redir/a: OFL fixture redirect to x/y"} {
		if !strings.Contains(out, want) {
			t.Errorf("log output missing %q:\n%s", want, out)
		}
	}
}

func TestImportSummary_StringWithoutFailures(t *testing.T) {
	s := NewImportSummary()
	s.Record("a/b", nil)
	if got := s.String(); got != "imported=1 skipped=0 failed=0" {
		t.Errorf("String() = %q", got)
	}
}
