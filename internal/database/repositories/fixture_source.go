package repositories

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"slices"
	"sort"
	"strings"

	"github.com/bbernstein/lacylights-go/internal/database/models"
	"gorm.io/gorm"
)

// FixtureSourcePrecedenceSettingKey is the settings key holding the
// fixture source precedence as a JSON array of source names.
const FixtureSourcePrecedenceSettingKey = "fixture_source_precedence"

// ParseFixtureSourcePrecedence parses a stored precedence value. An empty
// value yields the default order. Sources missing from the value are
// appended in default order, so the result always names every source once.
func ParseFixtureSourcePrecedence(value string) ([]string, error) {
	if strings.TrimSpace(value) == "" {
		return slices.Clone(models.DefaultFixtureSourcePrecedence), nil
	}
	var order []string
	if err := json.Unmarshal([]byte(value), &order); err != nil {
		return nil, fmt.Errorf("invalid fixture source precedence: %w", err)
	}
	return normalizeFixtureSourcePrecedence(order)
}

// normalizeFixtureSourcePrecedence validates order and completes it with
// any missing sources in default order.
func normalizeFixtureSourcePrecedence(order []string) ([]string, error) {
	out := make([]string, 0, len(models.DefaultFixtureSourcePrecedence))
	for _, src := range order {
		if !slices.Contains(models.DefaultFixtureSourcePrecedence, src) {
			return nil, fmt.Errorf("unknown fixture source %q", src)
		}
		if slices.Contains(out, src) {
			return nil, fmt.Errorf("fixture source %q listed more than once", src)
		}
		out = append(out, src)
	}
	for _, src := range models.DefaultFixtureSourcePrecedence {
		if !slices.Contains(out, src) {
			out = append(out, src)
		}
	}
	return out, nil
}

// FixtureSourcePrecedence returns the configured source precedence. A
// missing setting yields the default order. A read error or an invalid
// stored value is logged and also yields the default order: precedence is a
// preference, so failing to read it must not fail the lookup that needs it.
func (r *FixtureRepository) FixtureSourcePrecedence(ctx context.Context) ([]string, error) {
	setting, err := NewSettingRepository(r.db).FindByKey(ctx, FixtureSourcePrecedenceSettingKey)
	if err != nil {
		log.Printf("Warning: could not read %s, using the default order: %v", FixtureSourcePrecedenceSettingKey, err)
		return slices.Clone(models.DefaultFixtureSourcePrecedence), nil
	}
	if setting == nil {
		return slices.Clone(models.DefaultFixtureSourcePrecedence), nil
	}
	order, err := ParseFixtureSourcePrecedence(setting.Value)
	if err != nil {
		log.Printf("Warning: ignoring stored %s: %v", FixtureSourcePrecedenceSettingKey, err)
		return slices.Clone(models.DefaultFixtureSourcePrecedence), nil
	}
	return order, nil
}

// SetFixtureSourcePrecedence validates, completes and stores a precedence
// order, returning the stored order.
func (r *FixtureRepository) SetFixtureSourcePrecedence(ctx context.Context, order []string) ([]string, error) {
	full, err := normalizeFixtureSourcePrecedence(order)
	if err != nil {
		return nil, err
	}
	value, err := json.Marshal(full)
	if err != nil {
		return nil, err
	}
	if _, err := NewSettingRepository(r.db).Upsert(ctx, FixtureSourcePrecedenceSettingKey, string(value)); err != nil {
		return nil, err
	}
	return full, nil
}

// SortDefinitionsByPrecedence orders defs by source precedence (stable).
// Sources not in order sort last.
func SortDefinitionsByPrecedence(defs []models.FixtureDefinition, order []string) {
	rank := func(src string) int {
		if i := slices.Index(order, src); i >= 0 {
			return i
		}
		return len(order)
	}
	sort.SliceStable(defs, func(i, j int) bool { return rank(defs[i].Source) < rank(defs[j].Source) })
}

// FindDefinitionsByManufacturerModel returns every definition with the given
// manufacturer and model, ordered by the configured source precedence.
func (r *FixtureRepository) FindDefinitionsByManufacturerModel(ctx context.Context, manufacturer, model string) ([]models.FixtureDefinition, error) {
	var defs []models.FixtureDefinition
	if err := r.db.WithContext(ctx).
		Where("manufacturer = ? AND model = ?", manufacturer, model).
		Order("created_at ASC").
		Find(&defs).Error; err != nil {
		return nil, err
	}
	order, err := r.FixtureSourcePrecedence(ctx)
	if err != nil {
		return nil, err
	}
	SortDefinitionsByPrecedence(defs, order)
	return defs, nil
}

// FindDefinitionBySourceManufacturerModel returns the definition with the
// given manufacturer and model from one source, or nil.
func (r *FixtureRepository) FindDefinitionBySourceManufacturerModel(ctx context.Context, source, manufacturer, model string) (*models.FixtureDefinition, error) {
	return r.firstDefinition(ctx, "source = ? AND manufacturer = ? AND model = ?", source, manufacturer, model)
}

// FindDefinitionBySourceID returns the definition with the given source and
// source ID (e.g. GDTF FixtureTypeID), or nil.
func (r *FixtureRepository) FindDefinitionBySourceID(ctx context.Context, source, sourceID string) (*models.FixtureDefinition, error) {
	return r.firstDefinition(ctx, "source = ? AND source_id = ?", source, sourceID)
}

func (r *FixtureRepository) firstDefinition(ctx context.Context, query string, args ...any) (*models.FixtureDefinition, error) {
	var def models.FixtureDefinition
	err := r.db.WithContext(ctx).Where(query, args...).Order("created_at ASC").First(&def).Error
	if err == gorm.ErrRecordNotFound {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &def, nil
}
