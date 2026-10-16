package ofl

import (
	"archive/zip"
	"bytes"
	"context"
	"testing"

	"github.com/bbernstein/lacylights-go/internal/database/models"
	"github.com/bbernstein/lacylights-go/internal/services/testutil"
)

func testOFLArchive(t *testing.T, files map[string]string) *zip.Reader {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for name, body := range files {
		w, err := zw.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := w.Write([]byte(body)); err != nil {
			t.Fatal(err)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	zr, err := zip.NewReader(bytes.NewReader(buf.Bytes()), int64(buf.Len()))
	if err != nil {
		t.Fatal(err)
	}
	return zr
}

func legacyArchive(t *testing.T) *zip.Reader {
	return testOFLArchive(t, map[string]string{
		"root/fixtures/manufacturers.json":     `{"$schema": "x", "test": {"name": "Test"}}`,
		"root/fixtures/test/test-fixture.json": minimalOFLFixture,
		"root/fixtures/test/complex.json":      complexOFLFixture,
		"root/fixtures/test/broken.json":       `{not json`,
		"root/README.md":                       "ignored",
	})
}

func TestReclassifyLegacyImports(t *testing.T) {
	testDB, cleanup := testutil.SetupTestDB(t)
	defer cleanup()
	ctx := context.Background()

	create := func(mfr, model string, builtIn bool, hash *string, channels ...string) string {
		t.Helper()
		def := &models.FixtureDefinition{Manufacturer: mfr, Model: model, Type: "OTHER", IsBuiltIn: builtIn, OFLSourceHash: hash}
		var chans []models.ChannelDefinition
		for i, n := range channels {
			chans = append(chans, models.ChannelDefinition{Name: n, Type: "INTENSITY", Offset: i, MaxValue: 255})
		}
		if err := testDB.FixtureRepo.CreateDefinitionWithChannels(ctx, def, chans); err != nil {
			t.Fatal(err)
		}
		return def.ID
	}
	legacy := create("Test", "Test Fixture", false, nil, "Dimmer")
	// Same name as an OFL fixture but different channels: a genuine user definition.
	userSameName := create("Test", "Complex LED PAR", false, nil, "Dimmer")
	userOtherMfr := create("Elsewhere", "Test Fixture", false, nil, "Dimmer")

	n, err := ReclassifyLegacyImports(ctx, testDB.DB, legacyArchive(t))
	if err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Errorf("reclassified = %d, want 1", n)
	}
	want := map[string]string{
		legacy:       models.FixtureSourceOFL,
		userSameName: models.FixtureSourceUser,
		userOtherMfr: models.FixtureSourceUser,
	}
	for id, src := range want {
		def, err := testDB.FixtureRepo.FindDefinitionByID(ctx, id)
		if err != nil || def == nil {
			t.Fatalf("find %s: %v", id, err)
		}
		if def.Source != src {
			t.Errorf("%s %s source = %s, want %s", def.Manufacturer, def.Model, def.Source, src)
		}
	}

	// Runs once: a later matching USER definition is left alone.
	later := create("Test", "Test Fixture", false, nil, "Dimmer")
	n, err = ReclassifyLegacyImports(ctx, testDB.DB, legacyArchive(t))
	if err != nil || n != 0 {
		t.Errorf("second run = %d, %v; want 0, nil", n, err)
	}
	if def, _ := testDB.FixtureRepo.FindDefinitionByID(ctx, later); def == nil || def.Source != models.FixtureSourceUser {
		t.Errorf("definition created after the backfill was reclassified: %+v", def)
	}
}

func TestReclassifyLegacyImports_NoCandidatesMarksDone(t *testing.T) {
	testDB, cleanup := testutil.SetupTestDB(t)
	defer cleanup()
	ctx := context.Background()

	// An empty archive would fail to parse; with no candidates it is never read.
	empty := testOFLArchive(t, map[string]string{})
	if n, err := ReclassifyLegacyImports(ctx, testDB.DB, empty); err != nil || n != 0 {
		t.Fatalf("no candidates = %d, %v", n, err)
	}
	def := &models.FixtureDefinition{Manufacturer: "Test", Model: "Test Fixture", Type: "OTHER"}
	if err := testDB.FixtureRepo.CreateDefinition(ctx, def); err != nil {
		t.Fatal(err)
	}
	if n, err := ReclassifyLegacyImports(ctx, testDB.DB, empty); err != nil || n != 0 {
		t.Errorf("after done = %d, %v; want 0, nil", n, err)
	}
}

func TestReclassifyLegacyImports_MissingManufacturers(t *testing.T) {
	testDB, cleanup := testutil.SetupTestDB(t)
	defer cleanup()
	ctx := context.Background()
	def := &models.FixtureDefinition{Manufacturer: "Test", Model: "Test Fixture", Type: "OTHER"}
	if err := testDB.FixtureRepo.CreateDefinition(ctx, def); err != nil {
		t.Fatal(err)
	}
	zr := testOFLArchive(t, map[string]string{"root/fixtures/test/test-fixture.json": minimalOFLFixture})
	if _, err := ReclassifyLegacyImports(ctx, testDB.DB, zr); err == nil {
		t.Error("expected error for an archive without manufacturers.json")
	}
	// Not marked done after a failure, so the next start retries.
	if _, err := ReclassifyLegacyImports(ctx, testDB.DB, legacyArchive(t)); err != nil {
		t.Fatal(err)
	}
	if got, _ := testDB.FixtureRepo.FindDefinitionByID(ctx, def.ID); got == nil || got.Source != models.FixtureSourceUser {
		// minimal fixture has a "Dimmer" channel; this definition has none.
		t.Errorf("definition without matching channels should stay USER: %+v", got)
	}
}

func TestReclassifyLegacyImportsFromBundle(t *testing.T) {
	testDB, cleanup := testutil.SetupTestDB(t)
	defer cleanup()
	if _, err := ReclassifyLegacyImportsFromBundle(context.Background(), testDB.DB); err != nil {
		t.Errorf("bundle backfill: %v", err)
	}
}
