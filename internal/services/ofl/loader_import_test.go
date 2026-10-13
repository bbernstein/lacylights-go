package ofl

import (
	"archive/zip"
	"bytes"
	"context"
	"log"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/bbernstein/lacylights-go/internal/database/repositories"
)

// writeOFLZip builds a zip shaped like the OFL GitHub zipball.
func writeOFLZip(t *testing.T, files map[string]string) string {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for name, content := range files {
		w, err := zw.Create("OpenLightingProject-open-fixture-library-abc/" + name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := w.Write([]byte(content)); err != nil {
			t.Fatal(err)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "ofl.zip")
	if err := os.WriteFile(path, buf.Bytes(), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

// The startup loader counts redirects as skipped, imports matrix fixtures
// and logs a summary listing every failure by category.
func TestLoaderImportFromZip_SummaryByCategory(t *testing.T) {
	db := setupLoaderTestDB(t)
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	sqlDB.SetMaxOpenConns(1) // one shared in-memory database for all goroutines
	loader := NewLoader(db, repositories.NewFixtureRepository(db), t.TempDir())

	zipPath := writeOFLZip(t, map[string]string{
		"fixtures/manufacturers.json":                 `{"$schema":"x","generic":{"name":"Generic"},"cameo":{"name":"Cameo"}}`,
		"fixtures/generic/4-channel-dimmer-pack.json": loadOFLSample(t, "generic/4-channel-dimmer-pack.json"),
		"fixtures/cameo/ts60.json":                    loadOFLSample(t, "cameo/ts60.json"),
		"fixtures/generic/broken.json":                `{"name":"Broken","categories":["Dimmer"],"availableChannels":{"D":{}},"modes":[{"name":"m","channels":["Missing"]}]}`,
	})

	var logs bytes.Buffer
	prevOut, prevFlags := log.Writer(), log.Flags()
	log.SetOutput(&logs)
	log.SetFlags(0)
	defer func() { log.SetOutput(prevOut); log.SetFlags(prevFlags) }()

	status, err := loader.importFromZip(context.Background(), zipPath)
	if err != nil {
		t.Fatalf("importFromZip: %v", err)
	}
	if status.TotalFixtures != 3 || status.SuccessfulImports != 1 || status.SkippedImports != 1 || status.FailedImports != 1 {
		t.Errorf("status = %+v, want 3 total / 1 ok / 1 skipped / 1 failed", status)
	}
	out := logs.String()
	for _, want := range []string{
		"OFL import summary: imported=1 skipped=1 failed=1",
		"generic/broken: mode \"m\": mode references unknown channel",
		"cameo/ts60: OFL fixture redirect to cameo/ts-60-rgbw",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("log missing %q:\n%s", want, out)
		}
	}
}
