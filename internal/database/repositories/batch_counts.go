package repositories

import (
	"context"

	"github.com/bbernstein/lacylights-go/internal/database/models"
	"gorm.io/gorm"
)

// maxInVars is the number of IDs bound in one "IN ?" clause. SQLite limits
// bound variables per statement (999 in older builds, 32766 in newer
// ones), so batch lookups split larger ID lists into chunks of this size.
const maxInVars = 500

// chunkIDs splits ids into slices of at most maxInVars elements.
func chunkIDs(ids []string) [][]string {
	var out [][]string
	for len(ids) > maxInVars {
		out = append(out, ids[:maxInVars])
		ids = ids[maxInVars:]
	}
	if len(ids) > 0 {
		out = append(out, ids)
	}
	return out
}

// countByColumn counts rows of model grouped by column for the given IDs,
// keyed by the column value. IDs with no rows are absent from the map.
// column must be a trusted column name, never user input.
func countByColumn(ctx context.Context, db *gorm.DB, model any, column string, ids []string) (map[string]int, error) {
	out := make(map[string]int, len(ids))
	type row struct {
		GroupKey string
		Count    int
	}
	for _, chunk := range chunkIDs(ids) {
		var rows []row
		if err := db.WithContext(ctx).
			Model(model).
			Select(column+" AS group_key, COUNT(*) AS count").
			Where(column+" IN ?", chunk).
			Group(column).
			Scan(&rows).Error; err != nil {
			return nil, err
		}
		for _, rw := range rows {
			out[rw.GroupKey] = rw.Count
		}
	}
	return out, nil
}

// CountFixturesByLookIDs returns the number of fixture values in each look,
// keyed by look ID. Looks with no fixtures are absent.
func (r *LookRepository) CountFixturesByLookIDs(ctx context.Context, lookIDs []string) (map[string]int, error) {
	return countByColumn(ctx, r.db, &models.FixtureValue{}, "look_id", lookIDs)
}

// CountCuesByCueListIDs returns the number of cues in each cue list, keyed
// by cue list ID. Cue lists with no cues are absent.
func (r *CueListRepository) CountCuesByCueListIDs(ctx context.Context, cueListIDs []string) (map[string]int, error) {
	return countByColumn(ctx, r.db, &models.Cue{}, "cue_list_id", cueListIDs)
}

// CountFixturesByEffectIDs returns the number of fixtures in each effect,
// keyed by effect ID. Effects with no fixtures are absent.
func (r *EffectRepository) CountFixturesByEffectIDs(ctx context.Context, effectIDs []string) (map[string]int, error) {
	return countByColumn(ctx, r.db, &models.EffectFixture{}, "effect_id", effectIDs)
}

// FindDefinitionsByIDs returns the fixture definitions with the given IDs,
// keyed by ID. Unknown IDs are absent from the map.
func (r *FixtureRepository) FindDefinitionsByIDs(ctx context.Context, ids []string) (map[string]*models.FixtureDefinition, error) {
	out := make(map[string]*models.FixtureDefinition, len(ids))
	for _, chunk := range chunkIDs(ids) {
		var defs []models.FixtureDefinition
		if err := r.db.WithContext(ctx).Where("id IN ?", chunk).Find(&defs).Error; err != nil {
			return nil, err
		}
		for i := range defs {
			out[defs[i].ID] = &defs[i]
		}
	}
	return out, nil
}

// GetButtonsByBoardIDs returns the buttons of each look board, keyed by
// board ID, ordered top-to-bottom then left-to-right.
func (r *LookBoardRepository) GetButtonsByBoardIDs(ctx context.Context, boardIDs []string) (map[string][]models.LookBoardButton, error) {
	out := make(map[string][]models.LookBoardButton, len(boardIDs))
	for _, chunk := range chunkIDs(boardIDs) {
		var buttons []models.LookBoardButton
		if err := r.db.WithContext(ctx).
			Where("look_board_id IN ?", chunk).
			Order("layout_y ASC, layout_x ASC").
			Find(&buttons).Error; err != nil {
			return nil, err
		}
		for _, b := range buttons {
			out[b.LookBoardID] = append(out[b.LookBoardID], b)
		}
	}
	return out, nil
}

// RowCounter answers cheap COUNT queries used to size an export before
// loading its data.
type RowCounter struct {
	db *gorm.DB
}

// NewRowCounter creates a RowCounter.
func NewRowCounter(db *gorm.DB) *RowCounter {
	return &RowCounter{db: db}
}

// CountWhere counts rows of model whose column equals value. column must be
// a trusted column name, never user input.
func (c *RowCounter) CountWhere(ctx context.Context, model any, column, value string) (int, error) {
	var n int64
	err := c.db.WithContext(ctx).Model(model).Where(column+" = ?", value).Count(&n).Error
	return int(n), err
}

// CountAll counts every row of model.
func (c *RowCounter) CountAll(ctx context.Context, model any) (int, error) {
	var n int64
	err := c.db.WithContext(ctx).Model(model).Count(&n).Error
	return int(n), err
}

// CountProjectLookBoardButtons counts the buttons on every look board in
// a project.
func (c *RowCounter) CountProjectLookBoardButtons(ctx context.Context, projectID string) (int, error) {
	var n int64
	err := c.db.WithContext(ctx).
		Model(&models.LookBoardButton{}).
		Joins("JOIN look_boards ON look_boards.id = look_board_buttons.look_board_id").
		Where("look_boards.project_id = ?", projectID).
		Count(&n).Error
	return int(n), err
}

// CountProjectFixtureTypes counts the distinct (definition, mode) pairs used
// by fixtures in a project: exactly the FIXTURE_TYPES rows.
func (c *RowCounter) CountProjectFixtureTypes(ctx context.Context, projectID string) (int, error) {
	var n int64
	sub := c.db.Model(&models.FixtureInstance{}).
		Select("DISTINCT definition_id, COALESCE(mode_name, '')").
		Where("project_id = ?", projectID)
	err := c.db.WithContext(ctx).Table("(?) AS t", sub).Count(&n).Error
	return int(n), err
}

// CountLibraryRows counts the FIXTURE_LIBRARY rows: one per mode, plus one
// for each definition that has no modes.
func (c *RowCounter) CountLibraryRows(ctx context.Context) (int, error) {
	var modes, bare int64
	db := c.db.WithContext(ctx)
	// Only modes of existing definitions are exported; deleting a
	// definition does not delete its modes.
	if err := db.Model(&models.FixtureMode{}).
		Where("EXISTS (SELECT 1 FROM fixture_definitions WHERE fixture_definitions.id = fixture_modes.definition_id)").
		Count(&modes).Error; err != nil {
		return 0, err
	}
	if err := db.Model(&models.FixtureDefinition{}).
		Where("NOT EXISTS (SELECT 1 FROM fixture_modes WHERE fixture_modes.definition_id = fixture_definitions.id)").
		Count(&bare).Error; err != nil {
		return 0, err
	}
	return int(modes + bare), nil
}

// CountLookFixturesInProject counts the fixture values in a look whose
// fixture belongs to the project: exactly the LOOK_DETAILS rows (dangling
// and cross-project references are skipped by the export).
func (c *RowCounter) CountLookFixturesInProject(ctx context.Context, lookID, projectID string) (int, error) {
	var n int64
	err := c.db.WithContext(ctx).
		Model(&models.FixtureValue{}).
		Joins("JOIN fixture_instances ON fixture_instances.id = fixture_values.fixture_id").
		Where("fixture_values.look_id = ? AND fixture_instances.project_id = ?", lookID, projectID).
		Count(&n).Error
	return int(n), err
}

// GetFixtureValuesInProject returns a look's fixture values whose fixture
// belongs to the project, ordered by look order. Dangling and
// cross-project references are never loaded.
func (r *LookRepository) GetFixtureValuesInProject(ctx context.Context, lookID, projectID string) ([]models.FixtureValue, error) {
	var values []models.FixtureValue
	err := r.db.WithContext(ctx).
		Select("fixture_values.*").
		Joins("JOIN fixture_instances ON fixture_instances.id = fixture_values.fixture_id").
		Where("fixture_values.look_id = ? AND fixture_instances.project_id = ?", lookID, projectID).
		Order("fixture_values.look_order ASC").
		Find(&values).Error
	return values, err
}
