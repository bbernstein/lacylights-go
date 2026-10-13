package ofl

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/bbernstein/lacylights-go/internal/database/models"
)

// These tests import real fixtures from the Open Fixture Library (copied
// into testdata/ofl, MIT licensed, see testdata/ofl/LICENSE). Each one covers
// a category of fixture that used to fail validation (go#85) and checks the
// DMX layout of the imported modes: the channel name at every offset.

func loadOFLSample(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("testdata", "ofl", path))
	if err != nil {
		t.Fatalf("read sample %s: %v", path, err)
	}
	return string(data)
}

func importOFLSample(t *testing.T, service *Service, path string) *models.FixtureDefinition {
	t.Helper()
	def, err := service.ImportFixture(context.Background(), "Sample", loadOFLSample(t, path), false)
	if err != nil {
		t.Fatalf("ImportFixture(%s) failed: %v", path, err)
	}
	return def
}

// modeLayout returns the channel name and channel definition at each DMX
// offset of the named mode.
func modeLayout(t *testing.T, service *Service, def *models.FixtureDefinition, modeName string) ([]string, []models.ChannelDefinition) {
	t.Helper()
	var mode models.FixtureMode
	if err := service.db.Where("definition_id = ? AND name = ?", def.ID, modeName).First(&mode).Error; err != nil {
		t.Fatalf("mode %q not found: %v", modeName, err)
	}
	var mcs []models.ModeChannel
	if err := service.db.Where("mode_id = ?", mode.ID).Order("offset ASC").Find(&mcs).Error; err != nil {
		t.Fatalf("load mode channels: %v", err)
	}
	if len(mcs) != mode.ChannelCount {
		t.Errorf("mode %q: ChannelCount = %d but %d mode channels stored", modeName, mode.ChannelCount, len(mcs))
	}
	names := make([]string, len(mcs))
	defs := make([]models.ChannelDefinition, len(mcs))
	for i, mc := range mcs {
		if mc.Offset != i {
			t.Errorf("mode %q: mode channel %d has offset %d", modeName, i, mc.Offset)
		}
		var ch models.ChannelDefinition
		if err := service.db.First(&ch, "id = ?", mc.ChannelID).Error; err != nil {
			t.Fatalf("load channel %s: %v", mc.ChannelID, err)
		}
		if ch.DefinitionID != def.ID {
			t.Errorf("mode %q offset %d: channel belongs to definition %s", modeName, i, ch.DefinitionID)
		}
		names[i] = ch.Name
		defs[i] = ch
	}
	return names, defs
}

func assertLayout(t *testing.T, mode string, got, want []string) {
	t.Helper()
	if fmt.Sprint(got) != fmt.Sprint(want) {
		t.Errorf("mode %q layout:\n got  %q\n want %q", mode, got, want)
	}
}

// Channel keys that contain " / " are ordinary available channels; the
// importer used to split them and look up only the first half. The same
// fixture also uses a switching channel alias.
func TestImportOFLSample_SlashKeysAndSwitchingChannels(t *testing.T) {
	service, cleanup := setupTestDB(t)
	defer cleanup()
	def := importOFLSample(t, service, "futurelight/pro-slim-par-7-hcl.json")

	got, _ := modeLayout(t, service, def, "HSV")
	assertLayout(t, "HSV", got, []string{"Hue", "Saturation", "Value / Brightness"})

	got, defs := modeLayout(t, service, def, "12-channel")
	assertLayout(t, "12-channel", got, []string{
		"Intensity", "Red", "Green", "Blue", "Amber", "White", "UV", "Strobe",
		"Color Presets", "Internal Programs", "Program Speed / Mic Sensitivity", "Dimmer Speed",
	})
	// The switching alias behaves like its default target ("Program Speed",
	// selected by Internal Programs' default value 0).
	var target models.ChannelDefinition
	if err := service.db.Where("definition_id = ? AND name = ?", def.ID, "Program Speed").First(&target).Error; err != nil {
		t.Fatalf("Program Speed channel: %v", err)
	}
	if defs[10].Type != target.Type || defs[10].FadeBehavior != target.FadeBehavior {
		t.Errorf("switching channel = %s/%s, want target's %s/%s",
			defs[10].Type, defs[10].FadeBehavior, target.Type, target.FadeBehavior)
	}
}

// null mode entries are unused DMX slots and must keep their offset.
func TestImportOFLSample_NullChannels(t *testing.T) {
	service, cleanup := setupTestDB(t)
	defer cleanup()
	def := importOFLSample(t, service, "etc/source-four-led-series-2-daylight-hd.json")

	got, defs := modeLayout(t, service, def, "Studio")
	assertLayout(t, "Studio", got, []string{"Intensity", "Color Point", "Tint", "No function", "Strobe", "Fan Control"})
	if defs[3].Type != "OTHER" {
		t.Errorf("null slot type = %s, want OTHER", defs[3].Type)
	}

	got, _ = modeLayout(t, service, def, "HSI Plus 7")
	assertLayout(t, "HSI Plus 7", got, []string{
		"Hue", "Hue fine", "Saturation", "Intensity", "Strobe", "Fan Control", "No function",
		"Plus 7 Control", "Red", "No function", "Mint", "Blue", "Indigo", "No function", "No function",
	})
}

// Matrix insert blocks repeating template channels for each pixel
// (repeatFor eachPixelXYZ on a pixelCount matrix).
func TestImportOFLSample_MatrixInsertEachPixelXYZ(t *testing.T) {
	service, cleanup := setupTestDB(t)
	defer cleanup()
	def := importOFLSample(t, service, "eurolite/led-dmx-pixel-tube-16-rgb-ip20.json")

	got, defs := modeLayout(t, service, def, "48-channel")
	if len(got) != 48 {
		t.Fatalf("48-channel mode has %d channels", len(got))
	}
	assertLayout(t, "48-channel[0:6]", got[:6], []string{"Red 1", "Green 1", "Blue 1", "Red 2", "Green 2", "Blue 2"})
	assertLayout(t, "48-channel[45:]", got[45:], []string{"Red 16", "Green 16", "Blue 16"})
	if defs[0].Type != "RED" || defs[1].Type != "GREEN" || defs[47].Type != "BLUE" {
		t.Errorf("template channel types = %s, %s, %s", defs[0].Type, defs[1].Type, defs[47].Type)
	}
}

// Matrix insert blocks repeating for each pixel group (eachPixelGroup), on
// a matrix defined by named pixelKeys.
func TestImportOFLSample_MatrixInsertEachPixelGroup(t *testing.T) {
	service, cleanup := setupTestDB(t)
	defer cleanup()
	def := importOFLSample(t, service, "chauvet-dj/megastrobe-fx12.json")

	got, _ := modeLayout(t, service, def, "8-channel")
	assertLayout(t, "8-channel", got, []string{
		"Dimmer 1", "Dimmer 2", "Dimmer 3", "Dimmer 4", "Dimmer 5", "Dimmer 6",
		"Shutter Effects", "Shutter Effect Speed",
	})
}

// Template channels referenced directly, template fine aliases, explicit
// repeatFor lists and templated switching channels.
func TestImportOFLSample_TemplateChannelsAndTemplateSwitching(t *testing.T) {
	service, cleanup := setupTestDB(t)
	defer cleanup()
	def := importOFLSample(t, service, "cameo/hydrabeam-400-rgbw.json")

	got, defs := modeLayout(t, service, def, "6-channel")
	assertLayout(t, "6-channel", got, []string{
		"Dimmer All", "Strobe All", "Show mode All", "Show mode speed All / Sound sensitivity All",
		"Light control", "Auto color change speed",
	})
	if defs[0].Type != "INTENSITY" {
		t.Errorf("Dimmer All type = %s, want INTENSITY", defs[0].Type)
	}

	got, _ = modeLayout(t, service, def, "10-channel")
	assertLayout(t, "10-channel[8:]", got[8:], []string{"Pan All fine", "Tilt All fine"})

	got, defs = modeLayout(t, service, def, "32-channel")
	if len(got) != 32 {
		t.Fatalf("32-channel mode has %d channels", len(got))
	}
	assertLayout(t, "32-channel[8:14]", got[8:14], []string{"Pan 2", "Pan 2 fine", "Tilt 2", "Tilt 2 fine", "Head Speed 2", "Color 2"})
	if defs[8].Type != "PAN" || defs[9].Type != "PAN" {
		t.Errorf("Pan 2 / Pan 2 fine types = %s / %s, want PAN", defs[8].Type, defs[9].Type)
	}
}

// Fixtures that only have template channels (no availableChannels).
func TestImportOFLSample_TemplateOnlyFixture(t *testing.T) {
	service, cleanup := setupTestDB(t)
	defer cleanup()
	def := importOFLSample(t, service, "generic/4-channel-dimmer-pack.json")

	if def.Type != "DIMMER" {
		t.Errorf("type = %s, want DIMMER", def.Type)
	}
	got, _ := modeLayout(t, service, def, "1-channel")
	assertLayout(t, "1-channel", got, []string{"Dimmer Master"})
	got, _ = modeLayout(t, service, def, "2-channel")
	assertLayout(t, "2-channel", got, []string{"Dimmer 1/2", "Dimmer 3/4"})
	got, defs := modeLayout(t, service, def, "4-channel")
	assertLayout(t, "4-channel", got, []string{"Dimmer 1", "Dimmer 2", "Dimmer 3", "Dimmer 4"})
	for i, d := range defs {
		if d.Type != "INTENSITY" {
			t.Errorf("Dimmer %d type = %s, want INTENSITY", i+1, d.Type)
		}
	}
}

// OFL redirect files carry no channel data and are not imported.
func TestImportOFLSample_RedirectIsNotImported(t *testing.T) {
	service, cleanup := setupTestDB(t)
	defer cleanup()
	_, err := service.ImportFixture(context.Background(), "Cameo", loadOFLSample(t, "cameo/ts60.json"), false)
	if err == nil {
		t.Fatal("expected redirect fixture to be rejected")
	}
	var count int64
	service.db.Model(&models.FixtureDefinition{}).Count(&count)
	if count != 0 {
		t.Errorf("redirect created %d definitions", count)
	}
}

// The hash-tracking import path (used by the OFL manager) resolves the
// same layouts.
func TestImportFixtureWithHash_OFLSampleMatrix(t *testing.T) {
	service, cleanup := setupTestDB(t)
	defer cleanup()
	def, err := service.ImportFixtureWithHash(context.Background(), "Sample",
		loadOFLSample(t, "chauvet-dj/megastrobe-fx12.json"), "hash", "master", false)
	if err != nil {
		t.Fatalf("ImportFixtureWithHash failed: %v", err)
	}
	got, _ := modeLayout(t, service, def, "8-channel")
	assertLayout(t, "8-channel", got, []string{
		"Dimmer 1", "Dimmer 2", "Dimmer 3", "Dimmer 4", "Dimmer 5", "Dimmer 6",
		"Shutter Effects", "Shutter Effect Speed",
	})
}
