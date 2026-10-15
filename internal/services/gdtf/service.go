package gdtf

import (
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"slices"

	"github.com/lucsky/cuid"
	"gorm.io/gorm"

	"github.com/bbernstein/lacylights-go/internal/database/models"
)

// ExistingPolicy says what to do when a file's FixtureTypeID was already
// imported from GDTF and its content changed.
type ExistingPolicy string

const (
	// OnExistingUpdate rebuilds the existing definition in place (default).
	OnExistingUpdate ExistingPolicy = "UPDATE"
	// OnExistingSkip leaves the existing definition untouched.
	OnExistingSkip ExistingPolicy = "SKIP"
)

// Options configures an import.
type Options struct {
	OnExisting ExistingPolicy
}

// Status is the outcome of importing one file.
type Status string

// File outcomes.
const (
	StatusCreated   Status = "CREATED"
	StatusUpdated   Status = "UPDATED"
	StatusUnchanged Status = "UNCHANGED"
	StatusSkipped   Status = "SKIPPED"
	StatusFailed    Status = "FAILED"
)

// FileResult is the outcome of one file.
type FileResult struct {
	FileName     string
	Status       Status
	Definition   *models.FixtureDefinition // nil when FAILED
	Manufacturer string
	Model        string
	ModeCount    int
	Warnings     []string
	Error        string
}

// Result is the outcome of an import batch.
type Result struct {
	Results        []FileResult
	CreatedCount   int
	UpdatedCount   int
	UnchangedCount int
	SkippedCount   int
	FailedCount    int
}

// Service imports GDTF fixture types as fixture definitions.
type Service struct {
	db *gorm.DB
}

// NewService creates a GDTF import service.
func NewService(db *gorm.DB) *Service {
	return &Service{db: db}
}

// Import imports each file in its own transaction, so a bad file fails
// alone. A FixtureTypeID imported before is matched by (source GDTF,
// FixtureTypeID): an identical description is UNCHANGED, a changed one is
// updated in place (UPDATED, keeping the definition ID and RefID) or left
// alone (SKIPPED) per opts.OnExisting.
func (s *Service) Import(ctx context.Context, files []File, opts Options) *Result {
	if opts.OnExisting == "" {
		opts.OnExisting = OnExistingUpdate
	}
	res := &Result{Results: make([]FileResult, 0, len(files))}
	for _, f := range files {
		var fr FileResult
		if err := ctx.Err(); err != nil {
			fr = FileResult{FileName: f.FileName, Status: StatusFailed, Error: err.Error()}
		} else {
			fr = s.importFile(ctx, f, opts)
		}
		switch fr.Status {
		case StatusCreated:
			res.CreatedCount++
		case StatusUpdated:
			res.UpdatedCount++
		case StatusUnchanged:
			res.UnchangedCount++
		case StatusSkipped:
			res.SkippedCount++
		case StatusFailed:
			res.FailedCount++
		}
		res.Results = append(res.Results, fr)
	}
	return res
}

func (s *Service) importFile(ctx context.Context, f File, opts Options) FileResult {
	fr := FileResult{FileName: f.FileName, Warnings: []string{}}
	fail := func(err error) FileResult {
		fr.Status = StatusFailed
		fr.Error = err.Error()
		return fr
	}
	if len(f.DescriptionXML) > MaxDescriptionBytes {
		return fail(ErrDescriptionTooLarge)
	}
	doc, versionWarnings, err := parseDescription(f.DescriptionXML)
	if err != nil {
		return fail(err)
	}
	plan, err := buildPlan(doc, versionWarnings)
	if err != nil {
		return fail(err)
	}
	fr.Manufacturer, fr.Model, fr.ModeCount = plan.Manufacturer, plan.Model, len(plan.Modes)
	fr.Warnings = append(fr.Warnings, plan.Warnings...)

	sum := sha256.Sum256([]byte(f.DescriptionXML))
	hash := hex.EncodeToString(sum[:])
	payload, err := gzipBytes([]byte(f.DescriptionXML))
	if err != nil {
		return fail(fmt.Errorf("compress description: %w", err))
	}

	var def *models.FixtureDefinition
	err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		existing, err := findBySourceID(tx, plan.SourceID)
		if err != nil {
			return err
		}
		switch {
		case existing == nil:
			fr.Status = StatusCreated
			def, err = createDefinition(tx, plan, hash, payload)
			return err
		case existing.SourceHash != nil && *existing.SourceHash == hash:
			fr.Status = StatusUnchanged
			def = existing
			return nil
		case opts.OnExisting == OnExistingSkip:
			fr.Status = StatusSkipped
			def = existing
			fr.Warnings = append(fr.Warnings, "an older import of this fixture type exists and was kept (onExisting: SKIP)")
			return nil
		}
		fr.Status = StatusUpdated
		warnings, err := updateDefinition(tx, existing, plan, hash, payload)
		fr.Warnings = append(fr.Warnings, warnings...)
		def = existing
		return err
	})
	if err != nil {
		fr.Warnings = []string{}
		return fail(err)
	}
	loaded, err := loadDefinition(s.db.WithContext(ctx), def.ID)
	if err != nil {
		return fail(err)
	}
	fr.Definition = loaded
	if fr.Status == StatusUnchanged || fr.Status == StatusSkipped {
		fr.ModeCount = len(loaded.Modes)
	}
	return fr
}

func findBySourceID(tx *gorm.DB, sourceID string) (*models.FixtureDefinition, error) {
	var def models.FixtureDefinition
	err := tx.Where("source = ? AND source_id = ?", models.FixtureSourceGDTF, sourceID).
		Order("created_at ASC").First(&def).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("look up existing definition: %w", err)
	}
	return &def, nil
}

func loadDefinition(db *gorm.DB, id string) (*models.FixtureDefinition, error) {
	var def models.FixtureDefinition
	if err := db.Preload("Channels").Preload("Modes").First(&def, "id = ?", id).Error; err != nil {
		return nil, fmt.Errorf("load definition: %w", err)
	}
	return &def, nil
}

func createDefinition(tx *gorm.DB, plan *importPlan, hash string, payload []byte) (*models.FixtureDefinition, error) {
	id := cuid.New()
	def := &models.FixtureDefinition{
		ID:            id,
		RefID:         id,
		Manufacturer:  plan.Manufacturer,
		Model:         plan.Model,
		Type:          plan.Type,
		Source:        models.FixtureSourceGDTF,
		SourceID:      &plan.SourceID,
		SourceVersion: optional(plan.SourceVersion),
		SourceHash:    &hash,
		SourcePayload: payload,
	}
	if err := tx.Create(def).Error; err != nil {
		return nil, fmt.Errorf("create definition: %w", err)
	}
	if err := writePlan(tx, id, plan); err != nil {
		return nil, err
	}
	return def, nil
}

// updateDefinition rebuilds an existing definition's channels and modes in
// place. Its ID and RefID stay, so fixture instances keep referencing it;
// instances keep their own copied channels, as with OFL updates. Warnings
// report the patched fixtures affected.
func updateDefinition(tx *gorm.DB, def *models.FixtureDefinition, plan *importPlan, hash string, payload []byte) ([]string, error) {
	var modeIDs []string
	if err := tx.Model(&models.FixtureMode{}).Where("definition_id = ?", def.ID).Pluck("id", &modeIDs).Error; err != nil {
		return nil, fmt.Errorf("load existing modes: %w", err)
	}
	if len(modeIDs) > 0 {
		if err := tx.Where("mode_id IN ?", modeIDs).Delete(&models.ModeChannel{}).Error; err != nil {
			return nil, fmt.Errorf("delete existing mode channels: %w", err)
		}
	}
	if err := tx.Where("definition_id = ?", def.ID).Delete(&models.FixtureMode{}).Error; err != nil {
		return nil, fmt.Errorf("delete existing modes: %w", err)
	}
	if err := tx.Where("definition_id = ?", def.ID).Delete(&models.ChannelDefinition{}).Error; err != nil {
		return nil, fmt.Errorf("delete existing channels: %w", err)
	}

	def.Manufacturer = plan.Manufacturer
	def.Model = plan.Model
	def.Type = plan.Type
	def.SourceVersion = optional(plan.SourceVersion)
	def.SourceHash = &hash
	def.SourcePayload = payload
	if err := tx.Model(def).Select("manufacturer", "model", "type", "source_version", "source_hash", "source_payload", "updated_at").
		Updates(def).Error; err != nil {
		return nil, fmt.Errorf("update definition: %w", err)
	}
	if err := writePlan(tx, def.ID, plan); err != nil {
		return nil, err
	}
	return instanceWarnings(tx, def.ID, plan)
}

// instanceWarnings describes the patched fixtures of an updated definition.
func instanceWarnings(tx *gorm.DB, defID string, plan *importPlan) ([]string, error) {
	var instances []models.FixtureInstance
	if err := tx.Select("id", "mode_name").Where("definition_id = ?", defID).Find(&instances).Error; err != nil {
		return nil, fmt.Errorf("count fixture instances: %w", err)
	}
	if len(instances) == 0 {
		return nil, nil
	}
	warnings := []string{fmt.Sprintf("%d patched fixture(s) use this definition; their channels were not changed", len(instances))}
	modeNames := make([]string, len(plan.Modes))
	for i, m := range plan.Modes {
		modeNames[i] = m.Name
	}
	var missing []string
	for _, inst := range instances {
		if inst.ModeName != nil && *inst.ModeName != "" && !slices.Contains(modeNames, *inst.ModeName) && !slices.Contains(missing, *inst.ModeName) {
			missing = append(missing, *inst.ModeName)
		}
	}
	for _, m := range missing {
		warnings = append(warnings, fmt.Sprintf("mode %q used by patched fixtures is no longer in the file", m))
	}
	return warnings, nil
}

// writePlan creates the plan's channel definitions, modes and mode channels
// for definition defID.
func writePlan(tx *gorm.DB, defID string, plan *importPlan) error {
	channelIDs := make([]string, len(plan.Channels))
	channels := make([]models.ChannelDefinition, len(plan.Channels))
	for i, ch := range plan.Channels {
		channelIDs[i] = cuid.New()
		channels[i] = models.ChannelDefinition{
			ID:           channelIDs[i],
			Name:         ch.Name,
			Type:         ch.Type,
			Offset:       i,
			MinValue:     0,
			MaxValue:     255,
			DefaultValue: ch.DefaultValue,
			FadeBehavior: ch.FadeBehavior,
			IsDiscrete:   ch.IsDiscrete,
			DefinitionID: defID,
		}
	}
	if len(channels) > 0 {
		if err := tx.CreateInBatches(&channels, 100).Error; err != nil {
			return fmt.Errorf("create channels: %w", err)
		}
	}
	for _, pm := range plan.Modes {
		modeID := cuid.New()
		mode := &models.FixtureMode{
			ID:           modeID,
			Name:         pm.Name,
			ChannelCount: len(pm.channelIdx),
			DefinitionID: defID,
		}
		if err := tx.Create(mode).Error; err != nil {
			return fmt.Errorf("create mode %q: %w", pm.Name, err)
		}
		modeChannels := make([]models.ModeChannel, len(pm.channelIdx))
		for offset, idx := range pm.channelIdx {
			modeChannels[offset] = models.ModeChannel{
				ID:        cuid.New(),
				ModeID:    modeID,
				ChannelID: channelIDs[idx],
				Offset:    offset,
			}
		}
		if len(modeChannels) > 0 {
			if err := tx.CreateInBatches(&modeChannels, 100).Error; err != nil {
				return fmt.Errorf("create mode channels for %q: %w", pm.Name, err)
			}
		}
	}
	return nil
}

func gzipBytes(data []byte) ([]byte, error) {
	var buf bytes.Buffer
	zw := gzip.NewWriter(&buf)
	if _, err := zw.Write(data); err != nil {
		return nil, err
	}
	if err := zw.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func optional(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}
