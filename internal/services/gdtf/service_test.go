package gdtf

import (
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"strings"
	"sync"
	"testing"

	"github.com/bbernstein/lacylights-go/internal/database/models"
	"github.com/bbernstein/lacylights-go/internal/services/testutil"
)

func setupService(t *testing.T) (*Service, *testutil.TestDB) {
	t.Helper()
	db, cleanup := testutil.SetupTestDB(t)
	t.Cleanup(cleanup)
	return NewService(db.DB), db
}

func importOne(t *testing.T, s *Service, name, xmlText string, opts Options) FileResult {
	t.Helper()
	res := s.Import(context.Background(), []File{{FileName: name, DescriptionXML: xmlText}}, opts)
	if len(res.Results) != 1 {
		t.Fatalf("got %d results", len(res.Results))
	}
	return res.Results[0]
}

func sha(s string) string {
	sum := sha256.Sum256([]byte(s))
	return hex.EncodeToString(sum[:])
}

// modeLayout returns mode name -> channel names in offset order.
func modeLayout(t *testing.T, db *testutil.TestDB, defID string) map[string][]string {
	t.Helper()
	ctx := context.Background()
	modes, err := db.FixtureRepo.GetDefinitionModes(ctx, defID)
	if err != nil {
		t.Fatal(err)
	}
	var channels []models.ChannelDefinition
	if err := db.DB.Where("definition_id = ?", defID).Find(&channels).Error; err != nil {
		t.Fatal(err)
	}
	byID := map[string]string{}
	for _, c := range channels {
		byID[c.ID] = c.Name
	}
	out := map[string][]string{}
	for _, m := range modes {
		mcs, err := db.FixtureRepo.GetModeChannels(ctx, m.ID)
		if err != nil {
			t.Fatal(err)
		}
		names := make([]string, len(mcs))
		for _, mc := range mcs {
			name, ok := byID[mc.ChannelID]
			if !ok {
				t.Fatalf("mode %q references channel %s of another definition", m.Name, mc.ChannelID)
			}
			names[mc.Offset] = name
		}
		if m.ChannelCount != len(mcs) {
			t.Errorf("mode %q ChannelCount = %d, has %d mode channels", m.Name, m.ChannelCount, len(mcs))
		}
		out[m.Name] = names
	}
	return out
}

func TestImport_Create(t *testing.T) {
	s, db := setupService(t)
	text := readTestdata(t, "rgbw_par.xml")
	fr := importOne(t, s, "TestPar.gdtf", text, Options{})
	if fr.Status != StatusCreated || fr.Error != "" {
		t.Fatalf("status = %s, error = %q", fr.Status, fr.Error)
	}
	if fr.Manufacturer != "Synthetic Lighting Co" || fr.Model != "Test Par RGBW" || fr.ModeCount != 2 {
		t.Errorf("result = %+v", fr)
	}
	def := fr.Definition
	if def == nil {
		t.Fatal("no definition")
	}
	if def.Source != models.FixtureSourceGDTF || def.RefID != def.ID || def.Type != "LED_PAR" {
		t.Errorf("definition = %+v", def)
	}
	if def.SourceID == nil || *def.SourceID != "5A1C0B7E-0000-4000-8000-000000000001" {
		t.Errorf("SourceID = %v", def.SourceID)
	}
	if def.SourceHash == nil || *def.SourceHash != sha(text) {
		t.Errorf("SourceHash = %v", def.SourceHash)
	}
	if def.SourceVersion == nil || *def.SourceVersion != "1.2 (2026-02-03T11:30:00 Added 6ch mode)" {
		t.Errorf("SourceVersion = %v", def.SourceVersion)
	}
	if def.OFLSourceHash != nil || def.IsBuiltIn {
		t.Errorf("OFL/builtin fields set: %+v", def)
	}
	zr, err := gzip.NewReader(bytes.NewReader(def.SourcePayload))
	if err != nil {
		t.Fatal(err)
	}
	payload, _ := io.ReadAll(zr)
	if string(payload) != text {
		t.Error("SourcePayload does not gunzip to description.xml")
	}
	if len(def.Channels) != 6 || len(def.Modes) != 2 {
		t.Errorf("preloaded %d channels, %d modes", len(def.Channels), len(def.Modes))
	}
	layout := modeLayout(t, db, def.ID)
	if got := strings.Join(layout["6ch"], ","); got != "Dimmer,Red,Green,Blue,White,Shutter 1" {
		t.Errorf("6ch = %s", got)
	}
	if got := strings.Join(layout["4ch"], ","); got != "Red,Green,Blue,White" {
		t.Errorf("4ch = %s", got)
	}
}

func TestImport_ReimportUnchangedUpdatedSkip(t *testing.T) {
	s, db := setupService(t)
	ctx := context.Background()
	original := readTestdata(t, "rgbw_par.xml")
	created := importOne(t, s, "TestPar.gdtf", original, Options{})
	if created.Status != StatusCreated {
		t.Fatalf("create: %s %s", created.Status, created.Error)
	}
	defID, refID := created.Definition.ID, created.Definition.RefID

	// Patch a fixture instance in a project, using the 6ch mode.
	project := &models.Project{ID: "p1", Name: "Show"}
	if err := db.DB.Create(project).Error; err != nil {
		t.Fatal(err)
	}
	mode6, mode4 := "6ch", "4ch"
	count := 6
	for i, mode := range []*string{&mode6, &mode4} {
		inst := &models.FixtureInstance{
			ID: "inst" + string(rune('1'+i)), RefID: "inst-ref" + string(rune('1'+i)), Name: "Par",
			DefinitionID: defID, ModeName: mode, ChannelCount: &count, ProjectID: project.ID, Universe: 1, StartChannel: 1 + 10*i,
			Channels: []models.InstanceChannel{{ID: "ic" + string(rune('1'+i)), Offset: 0, Name: "Dimmer", Type: "INTENSITY", MaxValue: 255}},
		}
		if err := db.DB.Create(inst).Error; err != nil {
			t.Fatal(err)
		}
	}

	// Same content: UNCHANGED.
	again := importOne(t, s, "TestPar.gdtf", original, Options{})
	if again.Status != StatusUnchanged || again.Definition.ID != defID || again.ModeCount != 2 {
		t.Errorf("re-import: %s id=%s modes=%d", again.Status, again.Definition.ID, again.ModeCount)
	}

	// Changed content: rename 4ch away and add a revision.
	changed := strings.Replace(original, `<DMXMode Name="4ch"`, `<DMXMode Name="4ch RGBW"`, 1)
	changed = strings.Replace(changed, `</Revisions>`, `<Revision Date="2026-03-04T09:00:00" Text="Renamed mode"/></Revisions>`, 1)

	// SKIP leaves it alone.
	skipped := importOne(t, s, "TestPar.gdtf", changed, Options{OnExisting: OnExistingSkip})
	if skipped.Status != StatusSkipped || skipped.Definition.ID != defID {
		t.Errorf("skip: %s", skipped.Status)
	}
	def, err := db.FixtureRepo.FindDefinitionByID(ctx, defID)
	if err != nil || def == nil || *def.SourceHash != sha(original) {
		t.Fatalf("SKIP changed the definition: %+v %v", def, err)
	}

	var oldChannelIDs []string
	db.DB.Model(&models.ChannelDefinition{}).Where("definition_id = ?", defID).Pluck("id", &oldChannelIDs)

	// UPDATE rebuilds it in place.
	updated := importOne(t, s, "TestPar.gdtf", changed, Options{OnExisting: OnExistingUpdate})
	if updated.Status != StatusUpdated {
		t.Fatalf("update: %s %s", updated.Status, updated.Error)
	}
	if updated.Definition.ID != defID || updated.Definition.RefID != refID {
		t.Errorf("update changed ID/RefID: %s/%s, want %s/%s", updated.Definition.ID, updated.Definition.RefID, defID, refID)
	}
	if *updated.Definition.SourceHash != sha(changed) || *updated.Definition.SourceVersion != "1.2 (2026-03-04T09:00:00 Renamed mode)" {
		t.Errorf("source fields not updated: %v %v", *updated.Definition.SourceHash, *updated.Definition.SourceVersion)
	}
	if !hasWarning(updated.Warnings, "2 patched fixture(s)") || !hasWarning(updated.Warnings, `mode "4ch" used by patched fixtures`) {
		t.Errorf("warnings = %v", updated.Warnings)
	}
	layout := modeLayout(t, db, defID)
	if _, ok := layout["4ch RGBW"]; !ok || len(layout) != 2 {
		t.Errorf("modes after update = %v", layout)
	}

	// Old rows are gone; nothing is orphaned.
	var n int64
	db.DB.Model(&models.ChannelDefinition{}).Where("id IN ?", oldChannelIDs).Count(&n)
	if n != 0 {
		t.Errorf("%d old channel definitions remain", n)
	}
	db.DB.Model(&models.ModeChannel{}).Where("mode_id NOT IN (?)", db.DB.Model(&models.FixtureMode{}).Select("id")).Count(&n)
	if n != 0 {
		t.Errorf("%d orphaned mode channels", n)
	}
	var defCount int64
	db.DB.Model(&models.FixtureDefinition{}).Where("source = ?", models.FixtureSourceGDTF).Count(&defCount)
	if defCount != 1 {
		t.Errorf("%d GDTF definitions, want 1", defCount)
	}

	// The patched fixtures still resolve their definition and keep their channels.
	var insts []models.FixtureInstance
	if err := db.DB.Preload("Definition").Preload("Channels").Where("project_id = ?", project.ID).Find(&insts).Error; err != nil {
		t.Fatal(err)
	}
	for _, inst := range insts {
		if inst.Definition == nil || inst.Definition.ID != defID || len(inst.Channels) != 1 {
			t.Errorf("instance %s after update: def=%v channels=%d", inst.ID, inst.Definition, len(inst.Channels))
		}
	}
}

func TestImport_CoexistsWithOFL(t *testing.T) {
	s, db := setupService(t)
	ctx := context.Background()
	oflHash := "abc"
	oflDef := &models.FixtureDefinition{
		Manufacturer: "Synthetic Lighting Co", Model: "Test Par RGBW", Type: "LED_PAR",
		Source: models.FixtureSourceOFL, SourceHash: &oflHash, OFLSourceHash: &oflHash,
	}
	if err := db.FixtureRepo.CreateDefinition(ctx, oflDef); err != nil {
		t.Fatal(err)
	}
	fr := importOne(t, s, "TestPar.gdtf", readTestdata(t, "rgbw_par.xml"), Options{})
	if fr.Status != StatusCreated || fr.Definition.ID == oflDef.ID {
		t.Fatalf("status %s, id %s", fr.Status, fr.Definition.ID)
	}
	defs, err := db.FixtureRepo.FindDefinitionsByManufacturerModel(ctx, "Synthetic Lighting Co", "Test Par RGBW")
	if err != nil {
		t.Fatal(err)
	}
	// Default precedence: USER > GDTF > OFL.
	if len(defs) != 2 || defs[0].Source != models.FixtureSourceGDTF || defs[1].Source != models.FixtureSourceOFL {
		t.Errorf("definitions = %+v", defs)
	}
	got, _ := db.FixtureRepo.FindDefinitionByID(ctx, oflDef.ID)
	if got == nil || got.Source != models.FixtureSourceOFL || *got.SourceHash != "abc" {
		t.Errorf("OFL definition changed: %+v", got)
	}
}

func TestImport_BatchWithFailures(t *testing.T) {
	s, _ := setupService(t)
	valid := readTestdata(t, "mover16.xml")
	files := []File{
		{FileName: "bad.gdtf", DescriptionXML: "<GDTF DataVersion=\"1.2\"><FixtureType"},
		{FileName: "noid.gdtf", DescriptionXML: strings.Replace(valid, `FixtureTypeID="5A1C0B7E-0000-4000-8000-000000000002"`, "", 1)},
		{FileName: "old.gdtf", DescriptionXML: strings.Replace(valid, `DataVersion="1.2"`, `DataVersion="1.0"`, 1)},
		{FileName: "huge.gdtf", DescriptionXML: strings.Repeat(" ", MaxDescriptionBytes+1)},
		{FileName: "spot.gdtf", DescriptionXML: valid},
		{FileName: "copy/spot.gdtf", DescriptionXML: valid},
		{FileName: "v11.gdtf", DescriptionXML: strings.Replace(readTestdata(t, "pixelbar.xml"), `DataVersion="1.2"`, `DataVersion="1.1"`, 1)},
		{FileName: "v13.gdtf", DescriptionXML: strings.Replace(readTestdata(t, "multibreak.xml"), `DataVersion="1.2"`, `DataVersion="1.3"`, 1)},
	}
	res := s.Import(context.Background(), files, Options{})
	want := []Status{StatusFailed, StatusFailed, StatusFailed, StatusFailed, StatusCreated, StatusUnchanged, StatusCreated, StatusCreated}
	for i, w := range want {
		if res.Results[i].Status != w {
			t.Errorf("%s: status %s (%s), want %s", files[i].FileName, res.Results[i].Status, res.Results[i].Error, w)
		}
	}
	if res.CreatedCount != 3 || res.UnchangedCount != 1 || res.FailedCount != 4 || res.UpdatedCount != 0 || res.SkippedCount != 0 {
		t.Errorf("counts = %+v", res)
	}
	errs := []string{"XML", "no FixtureTypeID", "GDTF 1.0 is not supported", "larger than 32 MB"}
	for i, e := range errs {
		if r := res.Results[i]; !strings.Contains(r.Error, e) || r.Definition != nil || r.Warnings == nil {
			t.Errorf("%s: error %q (want %q), definition %v", r.FileName, r.Error, e, r.Definition)
		}
	}
	if !hasWarning(res.Results[6].Warnings, "1.1") || !hasWarning(res.Results[7].Warnings, "newer than the supported") {
		t.Errorf("version warnings: %v / %v", res.Results[6].Warnings, res.Results[7].Warnings)
	}
}

func TestImport_CancelledContext(t *testing.T) {
	s, _ := setupService(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	res := s.Import(ctx, []File{{FileName: "a.gdtf", DescriptionXML: readTestdata(t, "rgbw_par.xml")}}, Options{})
	if res.FailedCount != 1 || !strings.Contains(res.Results[0].Error, "canceled") {
		t.Errorf("result = %+v", res.Results[0])
	}
}

func TestImport_ExtractedArchive(t *testing.T) {
	s, _ := setupService(t)
	outer := buildZip(t,
		zipEntry{"a/par.gdtf", gdtfArchive(t, readTestdata(t, "rgbw_par.xml"))},
		zipEntry{"b/spot.gdtf", gdtfArchive(t, readTestdata(t, "mover16.xml"))},
	)
	files, err := extractBytes(outer, "rig.zip")
	if err != nil {
		t.Fatal(err)
	}
	res := s.Import(context.Background(), files, Options{})
	if res.CreatedCount != 2 || res.Results[0].FileName != "rig.zip/a/par.gdtf" {
		t.Errorf("result = %+v", res)
	}
}

func TestImport_ConcurrentSameFixtureTypeCreatesOne(t *testing.T) {
	s, db := setupService(t)
	text := readTestdata(t, "rgbw_par.xml")
	const n = 4
	statuses := make([]Status, n)
	var wg sync.WaitGroup
	for i := range n {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			res := s.Import(context.Background(), []File{{FileName: "TestPar.gdtf", DescriptionXML: text}}, Options{})
			statuses[i] = res.Results[0].Status
		}(i)
	}
	wg.Wait()

	var count int64
	if err := db.DB.Model(&models.FixtureDefinition{}).Where("source = ?", models.FixtureSourceGDTF).Count(&count).Error; err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Errorf("GDTF definitions = %d, want 1", count)
	}
	created := 0
	for _, st := range statuses {
		switch st {
		case StatusCreated:
			created++
		case StatusUnchanged:
		default:
			t.Errorf("unexpected status %s", st)
		}
	}
	if created != 1 {
		t.Errorf("created = %d, want 1 (statuses %v)", created, statuses)
	}
}
