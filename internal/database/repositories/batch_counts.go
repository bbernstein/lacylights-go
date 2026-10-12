package repositories

import (
	"context"

	"github.com/bbernstein/lacylights-go/internal/database/models"
	"gorm.io/gorm"
)

// countByColumn counts rows of model grouped by column for the given IDs in
// a single query, keyed by the column value. IDs with no rows are absent
// from the map. column must be a trusted column name, never user input.
func countByColumn(ctx context.Context, db *gorm.DB, model any, column string, ids []string) (map[string]int, error) {
	out := make(map[string]int, len(ids))
	if len(ids) == 0 {
		return out, nil
	}
	type row struct {
		GroupKey string
		Count    int
	}
	var rows []row
	if err := db.WithContext(ctx).
		Model(model).
		Select(column+" AS group_key, COUNT(*) AS count").
		Where(column+" IN ?", ids).
		Group(column).
		Scan(&rows).Error; err != nil {
		return nil, err
	}
	for _, rw := range rows {
		out[rw.GroupKey] = rw.Count
	}
	return out, nil
}

// CountFixturesByLookIDs returns the number of fixture values in each look,
// keyed by look ID, using one query. Looks with no fixtures are absent.
func (r *LookRepository) CountFixturesByLookIDs(ctx context.Context, lookIDs []string) (map[string]int, error) {
	return countByColumn(ctx, r.db, &models.FixtureValue{}, "look_id", lookIDs)
}

// CountCuesByCueListIDs returns the number of cues in each cue list, keyed
// by cue list ID, using one query. Cue lists with no cues are absent.
func (r *CueListRepository) CountCuesByCueListIDs(ctx context.Context, cueListIDs []string) (map[string]int, error) {
	return countByColumn(ctx, r.db, &models.Cue{}, "cue_list_id", cueListIDs)
}

// CountFixturesByEffectIDs returns the number of fixtures in each effect,
// keyed by effect ID, using one query. Effects with no fixtures are absent.
func (r *EffectRepository) CountFixturesByEffectIDs(ctx context.Context, effectIDs []string) (map[string]int, error) {
	return countByColumn(ctx, r.db, &models.EffectFixture{}, "effect_id", effectIDs)
}

// FindDefinitionsByIDs returns the fixture definitions with the given IDs,
// keyed by ID, using one query. Unknown IDs are absent from the map.
func (r *FixtureRepository) FindDefinitionsByIDs(ctx context.Context, ids []string) (map[string]*models.FixtureDefinition, error) {
	out := make(map[string]*models.FixtureDefinition, len(ids))
	if len(ids) == 0 {
		return out, nil
	}
	var defs []models.FixtureDefinition
	if err := r.db.WithContext(ctx).Where("id IN ?", ids).Find(&defs).Error; err != nil {
		return nil, err
	}
	for i := range defs {
		out[defs[i].ID] = &defs[i]
	}
	return out, nil
}

// GetButtonsByBoardIDs returns the buttons of each look board, keyed by
// board ID, ordered top-to-bottom then left-to-right, using one query.
func (r *LookBoardRepository) GetButtonsByBoardIDs(ctx context.Context, boardIDs []string) (map[string][]models.LookBoardButton, error) {
	out := make(map[string][]models.LookBoardButton, len(boardIDs))
	if len(boardIDs) == 0 {
		return out, nil
	}
	var buttons []models.LookBoardButton
	if err := r.db.WithContext(ctx).
		Where("look_board_id IN ?", boardIDs).
		Order("layout_y ASC, layout_x ASC").
		Find(&buttons).Error; err != nil {
		return nil, err
	}
	for _, b := range buttons {
		out[b.LookBoardID] = append(out[b.LookBoardID], b)
	}
	return out, nil
}
