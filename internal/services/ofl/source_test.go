package ofl

import (
	"context"
	"testing"

	"github.com/bbernstein/lacylights-go/internal/database/models"
)

func TestImportFixture_SetsOFLSource(t *testing.T) {
	service, cleanup := setupTestDB(t)
	defer cleanup()
	ctx := context.Background()

	def, err := service.ImportFixture(ctx, "Test", minimalOFLFixture, false)
	if err != nil {
		t.Fatal(err)
	}
	if def.Source != models.FixtureSourceOFL {
		t.Errorf("ImportFixture Source = %q, want OFL", def.Source)
	}

	def2, err := service.ImportFixtureWithHash(ctx, "Other", minimalOFLFixture, "hash1", "v9", false)
	if err != nil {
		t.Fatal(err)
	}
	if def2.Source != models.FixtureSourceOFL {
		t.Errorf("ImportFixtureWithHash Source = %q, want OFL", def2.Source)
	}
	if def2.SourceHash == nil || *def2.SourceHash != "hash1" || def2.SourceVersion == nil || *def2.SourceVersion != "v9" {
		t.Errorf("source hash/version = %v %v", def2.SourceHash, def2.SourceVersion)
	}
}

// A definition from another source with the same manufacturer and model
// must not block, or be replaced by, an OFL import.
func TestImportFixture_IgnoresOtherSources(t *testing.T) {
	service, cleanup := setupTestDB(t)
	defer cleanup()
	ctx := context.Background()

	user := &models.FixtureDefinition{Manufacturer: "Test", Model: "Test Fixture", Type: "OTHER", Source: models.FixtureSourceUser}
	if err := service.fixtureRepo.CreateDefinition(ctx, user); err != nil {
		t.Fatal(err)
	}

	for _, replace := range []bool{false, true} {
		if _, err := service.ImportFixture(ctx, "Test", minimalOFLFixture, replace); err != nil {
			t.Fatalf("ImportFixture(replace=%v): %v", replace, err)
		}
		if _, err := service.ImportFixtureWithHash(ctx, "Test", minimalOFLFixture, "h", "v", replace); err != nil && replace {
			t.Fatalf("ImportFixtureWithHash(replace=%v): %v", replace, err)
		}
	}

	still, err := service.fixtureRepo.FindDefinitionByID(ctx, user.ID)
	if err != nil || still == nil {
		t.Fatalf("user definition was removed: %v", err)
	}
	all, err := service.fixtureRepo.FindDefinitionsByManufacturerModel(ctx, "Test", "Test Fixture")
	if err != nil {
		t.Fatal(err)
	}
	if len(all) != 2 || all[0].Source != models.FixtureSourceUser || all[1].Source != models.FixtureSourceOFL {
		var got []string
		for _, d := range all {
			got = append(got, d.Source)
		}
		t.Errorf("definitions = %v, want [USER OFL]", got)
	}
}
