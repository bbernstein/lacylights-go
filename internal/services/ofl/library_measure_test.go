package ofl

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"testing"
)

var channelCountLabel = regexp.MustCompile(`(?i)\b(\d+)[ -]?(ch|channel)`)

func channelCountFromLabel(label string) (int, bool) {
	m := channelCountLabel.FindStringSubmatch(label)
	if m == nil {
		return 0, false
	}
	n, err := strconv.Atoi(m[1])
	return n, err == nil
}

// TestMeasureOFLLibrary imports every fixture in a local checkout of the
// Open Fixture Library and reports the failures grouped by category. It is
// skipped unless OFL_FIXTURES_DIR points at the library's "fixtures"
// directory, e.g.:
//
//	OFL_FIXTURES_DIR=/path/to/open-fixture-library/fixtures \
//	  go test ./internal/services/ofl -run TestMeasureOFLLibrary -v
//
// Use it to measure before/after counts when changing the importer (go#85).
func TestMeasureOFLLibrary(t *testing.T) {
	dir := os.Getenv("OFL_FIXTURES_DIR")
	if dir == "" {
		t.Skip("OFL_FIXTURES_DIR not set")
	}

	manufacturers := map[string]Manufacturer{}
	if data, err := os.ReadFile(filepath.Join(dir, ManufacturersFile)); err == nil {
		var raw map[string]json.RawMessage
		if err := json.Unmarshal(data, &raw); err == nil {
			for k, v := range raw {
				var m Manufacturer
				if json.Unmarshal(v, &m) == nil {
					manufacturers[k] = m
				}
			}
		}
	}

	files, err := filepath.Glob(filepath.Join(dir, "*", "*.json"))
	if err != nil {
		t.Fatal(err)
	}

	service, cleanup := setupTestDB(t)
	defer cleanup()
	ctx := context.Background()

	summary := NewImportSummary()
	var mismatches []string
	checked := 0
	for _, f := range files {
		data, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		key := filepath.Base(filepath.Dir(f))
		name := key
		if m, ok := manufacturers[key]; ok && m.Name != "" {
			name = m.Name
		}
		fixture := key + "/" + strings.TrimSuffix(filepath.Base(f), ".json")
		def, err := service.ImportFixture(ctx, name, string(data), false)
		summary.Record(fixture, err)
		if err != nil {
			continue
		}
		// Cross-check: many OFL modes encode their channel count in the
		// name or shortName ("14ch", "8-channel"). Report disagreements.
		for _, m := range def.Modes {
			label := m.Name
			if m.ShortName != nil {
				label += " " + *m.ShortName
			}
			want, ok := channelCountFromLabel(label)
			if !ok {
				continue
			}
			checked++
			if want != m.ChannelCount {
				mismatches = append(mismatches, fmt.Sprintf("%s mode %q: %d channels", fixture, label, m.ChannelCount))
			}
		}
	}
	t.Logf("modes checked against the channel count in their name: %d, mismatches: %d", checked, len(mismatches))
	for _, m := range mismatches {
		t.Logf("  %s", m)
	}

	t.Logf("files=%d %s", len(files), summary.String())
	cats := summary.Categories()
	sort.Strings(cats)
	for _, c := range cats {
		for _, e := range summary.Examples(c) {
			t.Logf("  [%s] %s", c, e)
		}
	}
}
