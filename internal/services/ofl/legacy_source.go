package ofl

import (
	"archive/zip"
	"context"
	"fmt"
	"io"
	"log"
	"slices"
	"strings"

	"github.com/bbernstein/lacylights-go/internal/database/models"
	"github.com/bbernstein/lacylights-go/internal/database/repositories"
	"gorm.io/gorm"
)

// legacySourceBackfillSettingKey records that legacy OFL imports have been
// reclassified, so the bundle is only scanned once.
const legacySourceBackfillSettingKey = "ofl_legacy_source_backfill"

// ReclassifyLegacyImportsFromBundle runs ReclassifyLegacyImports against the
// embedded OFL bundle. Without a bundle it does nothing and leaves the
// backfill pending, so a later build that embeds one can still run it.
func ReclassifyLegacyImportsFromBundle(ctx context.Context, db *gorm.DB) (int, error) {
	bundle := NewBundleService()
	if !bundle.HasBundle() {
		return 0, nil
	}
	zr, err := bundle.GetBundleReader()
	if err != nil {
		return 0, err
	}
	return ReclassifyLegacyImports(ctx, db, zr)
}

// ReclassifyLegacyImports marks definitions created by the original OFL
// startup loader as OFL. That loader stored neither an OFL hash nor
// is_built_in, so BackfillFixtureSources leaves those rows as USER.
//
// Names alone are not evidence: a USER definition is reclassified only when
// the OFL archive has a fixture with the same manufacturer and model and the
// OFL import of that fixture yields exactly the same set of channel names.
// The result is recorded in settings, so later calls return immediately.
func ReclassifyLegacyImports(ctx context.Context, db *gorm.DB, zr *zip.Reader) (int, error) {
	settings := repositories.NewSettingRepository(db)
	if done, err := settings.FindByKey(ctx, legacySourceBackfillSettingKey); err == nil && done != nil {
		return 0, nil
	}

	var candidates []models.FixtureDefinition
	if err := db.WithContext(ctx).
		Where("source = ? AND ofl_source_hash IS NULL AND NOT is_built_in", models.FixtureSourceUser).
		Preload("Channels").
		Find(&candidates).Error; err != nil {
		return 0, fmt.Errorf("find legacy OFL candidates: %w", err)
	}

	reclassified := 0
	if len(candidates) > 0 {
		var err error
		reclassified, err = reclassifyCandidates(ctx, db, zr, candidates)
		if err != nil {
			return 0, err
		}
	}

	if _, err := settings.Upsert(ctx, legacySourceBackfillSettingKey, "done"); err != nil {
		return reclassified, err
	}
	if reclassified > 0 {
		log.Printf("📋 Reclassified %d legacy OFL fixture definitions as OFL", reclassified)
	}
	return reclassified, nil
}

func reclassifyCandidates(ctx context.Context, db *gorm.DB, zr *zip.Reader, candidates []models.FixtureDefinition) (int, error) {
	manufacturers, err := parseManufacturersFromZip(zr)
	if err != nil {
		return 0, err
	}

	// candidate channel-name signatures by "manufacturer/model"
	byKey := make(map[string][]*models.FixtureDefinition)
	candidateMfrs := make(map[string]bool)
	for i := range candidates {
		c := &candidates[i]
		key := c.Manufacturer + "/" + c.Model
		byKey[key] = append(byKey[key], c)
		candidateMfrs[c.Manufacturer] = true
	}

	var ids []string
	for _, f := range zr.File {
		parts := strings.Split(f.Name, "/")
		if len(parts) != 4 || parts[1] != "fixtures" || !strings.HasSuffix(f.Name, ".json") || parts[3] == ManufacturersFile {
			continue
		}
		mfg, ok := manufacturers[parts[2]]
		if !ok || !candidateMfrs[mfg.Name] {
			continue
		}
		data, err := readZipFile(f)
		if err != nil {
			continue
		}
		fixture, err := parseOFLFixture(data)
		if err != nil {
			continue
		}
		defs := byKey[mfg.Name+"/"+fixture.Name]
		if len(defs) == 0 {
			continue
		}
		plan, err := buildImportPlan(fixture)
		if err != nil {
			continue
		}
		want := planChannelNames(plan)
		for _, d := range defs {
			if slices.Equal(definitionChannelNames(d), want) {
				ids = append(ids, d.ID)
			}
		}
	}

	if len(ids) == 0 {
		return 0, nil
	}
	if err := db.WithContext(ctx).Model(&models.FixtureDefinition{}).
		Where("id IN ? AND source = ?", ids, models.FixtureSourceUser).
		Update("source", models.FixtureSourceOFL).Error; err != nil {
		return 0, fmt.Errorf("reclassify legacy OFL definitions: %w", err)
	}
	return len(ids), nil
}

func readZipFile(f *zip.File) (string, error) {
	rc, err := f.Open()
	if err != nil {
		return "", err
	}
	defer func() { _ = rc.Close() }()
	data, err := io.ReadAll(rc)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

func planChannelNames(plan *importPlan) []string {
	names := make([]string, len(plan.Channels))
	for i, c := range plan.Channels {
		names[i] = c.Name
	}
	slices.Sort(names)
	return names
}

func definitionChannelNames(def *models.FixtureDefinition) []string {
	names := make([]string, len(def.Channels))
	for i, c := range def.Channels {
		names[i] = c.Name
	}
	slices.Sort(names)
	return names
}
