package importeos

import (
	"os"
	"path/filepath"
	"testing"
)

func parseFixture(t *testing.T, name string) *Show {
	t.Helper()
	f, err := os.Open(filepath.Join("testdata", name))
	if err != nil {
		t.Fatalf("open fixture: %v", err)
	}
	defer func() { _ = f.Close() }()
	show, _, err := Parse(f)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	return show
}

func TestParse_MinimalPatch(t *testing.T) {
	show := parseFixture(t, "minimal_patch.asc")

	if show.Ident != "3:0" {
		t.Errorf("ident: got %q, want %q", show.Ident, "3:0")
	}
	if show.Manufacturer != "ETC" {
		t.Errorf("manufacturer: got %q, want %q", show.Manufacturer, "ETC")
	}
	if show.Console != "Eos" {
		t.Errorf("console: got %q", show.Console)
	}
	if show.Format != "3.20" {
		t.Errorf("format: got %q", show.Format)
	}
	if show.Title != "Minimal Patch Test" {
		t.Errorf("title: got %q", show.Title)
	}
	if len(show.ParamTypes) != 1 || show.ParamTypes[0].ID != 1 || show.ParamTypes[0].LongName != "Intens" {
		t.Errorf("param types: got %+v", show.ParamTypes)
	}
	if len(show.Personalities) != 1 || show.Personalities[0].ID != 9001 ||
		show.Personalities[0].Manuf != "Generic" || show.Personalities[0].Model != "Dimmer" {
		t.Errorf("personalities: got %+v", show.Personalities)
	}
	if len(show.Patch) != 2 {
		t.Fatalf("patch entries: got %d, want 2", len(show.Patch))
	}
	if show.Patch[0].Channel != 1 || show.Patch[0].AddressRaw != "1" || show.Patch[0].PersonalityID != 9001 ||
		show.Patch[0].Label != "Cyc R" {
		t.Errorf("patch[0]: got %+v", show.Patch[0])
	}
	if show.Patch[1].Label != "Cyc L" {
		t.Errorf("patch[1] label: got %q", show.Patch[1].Label)
	}
}
