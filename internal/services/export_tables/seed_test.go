package exporttables

import (
	"context"
	"testing"
	"time"

	"github.com/bbernstein/lacylights-go/internal/database/migrations"
	"github.com/bbernstein/lacylights-go/internal/database/models"
	"github.com/bbernstein/lacylights-go/internal/database/repositories"
	"github.com/bbernstein/lacylights-go/internal/services/testutil"
)

// seed holds IDs of the data created by seedProject.
type seed struct {
	svc *Service
	db  *testutil.TestDB

	projectID      string
	otherProjectID string

	defOFL, defCustom, defUnused string

	washA, dimmer, washB string

	lookWarm, lookEmpty string
	otherLook           string

	cueList      string
	otherCueList string

	boardMain, boardAlt string
	otherBoard          string

	effect string
}

func ptr[T any](v T) *T { return &v }

// newSeededService builds an in-memory database with a representative
// project ("My Show") plus a second project ("Other Show") used for
// scope-mismatch tests.
func newSeededService(t *testing.T) *seed {
	t.Helper()
	db, cleanup := testutil.SetupTestDB(t)
	t.Cleanup(cleanup)
	if err := db.DB.AutoMigrate(&models.FixtureGroup{}, &models.FixtureGroupMember{}); err != nil {
		t.Fatalf("migrate groups: %v", err)
	}

	// Create the production ref_id unique indexes so tests catch create
	// paths that skip RefID (see migrations.CreateRefIDIndexes).
	if err := migrations.CreateRefIDIndexes(db.DB); err != nil {
		t.Fatalf("Failed to create ref_id indexes: %v", err)
	}
	ctx := context.Background()
	must := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatalf("seed: %v", err)
		}
	}

	groupRepo := repositories.NewFixtureGroupRepository(db.DB)
	boardRepo := repositories.NewLookBoardRepository(db.DB)
	effectRepo := repositories.NewEffectRepository(db.DB)

	s := &seed{db: db}
	s.svc = NewService(Repos{
		Fixtures:      db.FixtureRepo,
		FixtureGroups: groupRepo,
		Looks:         db.LookRepo,
		CueLists:      db.CueListRepo,
		LookBoards:    boardRepo,
		Effects:       effectRepo,
		Counts:        repositories.NewRowCounter(db.DB),
	})
	s.svc.Now = func() time.Time { return time.Date(2026, 10, 12, 9, 0, 0, 0, time.UTC) }

	proj := &models.Project{Name: "My Show"}
	must(db.ProjectRepo.Create(ctx, proj))
	s.projectID = proj.ID
	other := &models.Project{Name: "Other Show"}
	must(db.ProjectRepo.Create(ctx, other))
	s.otherProjectID = other.ID

	// Fixture definitions: an OFL one with a mode, a custom one, and one
	// that no fixture uses (library only).
	defOFL := &models.FixtureDefinition{Manufacturer: "Chauvet", Model: "SlimPAR 56", Type: "LED_PAR", OFLSourceHash: ptr("abc")}
	must(db.FixtureRepo.CreateDefinitionWithChannels(ctx, defOFL, []models.ChannelDefinition{
		{Name: "Red", Type: "RED", Offset: 0},
		{Name: "Green", Type: "GREEN", Offset: 1},
		{Name: "Blue", Type: "BLUE", Offset: 2},
	}))
	must(db.FixtureRepo.CreateMode(ctx, &models.FixtureMode{Name: "3ch", ChannelCount: 3, DefinitionID: defOFL.ID}))
	must(db.FixtureRepo.CreateMode(ctx, &models.FixtureMode{Name: "7ch", ChannelCount: 7, DefinitionID: defOFL.ID}))
	defCustom := &models.FixtureDefinition{Manufacturer: "Generic", Model: "Dimmer", Type: "DIMMER"}
	must(db.FixtureRepo.CreateDefinitionWithChannels(ctx, defCustom, []models.ChannelDefinition{
		{Name: "Intensity", Type: "INTENSITY", Offset: 0},
	}))
	defUnused := &models.FixtureDefinition{Manufacturer: "Acme", Model: "Builtin Spot", Type: "MOVING_HEAD", IsBuiltIn: true}
	must(db.FixtureRepo.CreateDefinitionWithChannels(ctx, defUnused, []models.ChannelDefinition{
		{Name: "Pan", Type: "PAN", Offset: 0},
		{Name: "Tilt", Type: "TILT", Offset: 1},
	}))
	s.defOFL, s.defCustom, s.defUnused = defOFL.ID, defCustom.ID, defUnused.ID

	rgb := func() []models.InstanceChannel {
		return []models.InstanceChannel{
			{Offset: 0, Name: "Red", Type: "RED"},
			{Offset: 1, Name: "Green", Type: "GREEN"},
			{Offset: 2, Name: "Blue", Type: "BLUE"},
		}
	}
	washA := &models.FixtureInstance{
		Name: "Wash 1", ProjectID: s.projectID, DefinitionID: defOFL.ID,
		Manufacturer: ptr("Chauvet"), Model: ptr("SlimPAR 56"), Type: ptr("LED_PAR"),
		ModeName: ptr("3ch"), ChannelCount: ptr(3), Universe: 1, StartChannel: 1,
		Tags: ptr(`["front","wash"]`), Description: ptr("Stage left"),
	}
	must(db.FixtureRepo.CreateWithChannels(ctx, washA, rgb()))
	// No denormalised manufacturer/model/type or channel count: the export
	// must fall back to the definition and the instance channels.
	dimmer := &models.FixtureInstance{
		Name: "Dimmer 1", ProjectID: s.projectID, DefinitionID: defCustom.ID,
		ModeName: ptr("1ch"), Universe: 1, StartChannel: 10,
	}
	must(db.FixtureRepo.CreateWithChannels(ctx, dimmer, []models.InstanceChannel{
		{Offset: 0, Name: "Intensity", Type: "INTENSITY"},
	}))
	washB := &models.FixtureInstance{
		Name: "Wash 2", ProjectID: s.projectID, DefinitionID: defOFL.ID,
		Manufacturer: ptr("Chauvet"), Model: ptr("SlimPAR 56"), Type: ptr("LED_PAR"),
		ModeName: ptr("3ch"), ChannelCount: ptr(3), Universe: 2, StartChannel: 1,
	}
	must(db.FixtureRepo.CreateWithChannels(ctx, washB, rgb()))
	s.washA, s.dimmer, s.washB = washA.ID, dimmer.ID, washB.ID

	must(groupRepo.CreateWithMembers(ctx, &models.FixtureGroup{ProjectID: s.projectID, Name: "Front"}, []string{washA.ID, washB.ID}))
	must(groupRepo.CreateWithMembers(ctx, &models.FixtureGroup{ProjectID: s.projectID, Name: "All"}, []string{washA.ID}))

	// Looks: "Warm" has sparse values (Wash 1 channel 3 unset, an explicit
	// 0 on channel 2); "Empty" has none.
	warm := &models.Look{Name: "Warm", Description: ptr("Warm wash"), ProjectID: s.projectID}
	must(db.LookRepo.CreateWithFixtureValues(ctx, warm, []models.FixtureValue{
		{FixtureID: washA.ID, Channels: `[{"offset":0,"value":255},{"offset":1,"value":0}]`, LookOrder: ptr(0)},
		{FixtureID: dimmer.ID, Channels: `[{"offset":0,"value":128}]`, LookOrder: ptr(1)},
	}))
	empty := &models.Look{Name: "Empty", ProjectID: s.projectID}
	must(db.LookRepo.Create(ctx, empty))
	s.lookWarm, s.lookEmpty = warm.ID, empty.ID
	otherLook := &models.Look{Name: "Elsewhere", ProjectID: s.otherProjectID}
	must(db.LookRepo.Create(ctx, otherLook))
	s.otherLook = otherLook.ID

	// Cue list with a formula-looking note and a fractional cue number.
	cl := &models.CueList{Name: "Act 1", Description: ptr("First act"), ProjectID: s.projectID}
	must(db.CueListRepo.Create(ctx, cl))
	s.cueList = cl.ID
	must(db.CueRepo.Create(ctx, &models.Cue{
		Name: "Open", CueNumber: 1, CueListID: cl.ID, LookID: warm.ID,
		FadeInTime: 3, FadeOutTime: 2, EasingType: ptr("EASE_IN_OUT"), Notes: ptr("=SUM(A1)"),
	}))
	must(db.CueRepo.Create(ctx, &models.Cue{
		Name: "Blackout", CueNumber: 2.5, CueListID: cl.ID, LookID: empty.ID,
		FadeInTime: 0, FadeOutTime: 5, FollowTime: ptr(1.5), Skip: true,
	}))
	loopList := &models.CueList{Name: "Preshow", Loop: true, ProjectID: s.projectID}
	must(db.CueListRepo.Create(ctx, loopList))
	otherList := &models.CueList{Name: "Other list", ProjectID: s.otherProjectID}
	must(db.CueListRepo.Create(ctx, otherList))
	s.otherCueList = otherList.ID

	// Look boards.
	main := &models.LookBoard{Name: "Main", ProjectID: s.projectID, DefaultFadeTime: 2.5}
	must(boardRepo.CreateWithButtons(ctx, main, []models.LookBoardButton{
		{LookID: warm.ID, LayoutX: 0, LayoutY: 0, Width: ptr(200), Height: ptr(120), Label: ptr("Warm wash"), Color: ptr("#ff0000")},
		{LookID: empty.ID, LayoutX: 220, LayoutY: 0, Width: ptr(200), Height: ptr(120)},
	}))
	alt := &models.LookBoard{Name: "Alt", ProjectID: s.projectID, DefaultFadeTime: 1}
	must(boardRepo.CreateWithButtons(ctx, alt, []models.LookBoardButton{
		{LookID: warm.ID, LayoutX: 10, LayoutY: 20},
	}))
	s.boardMain, s.boardAlt = main.ID, alt.ID
	otherBoard := &models.LookBoard{Name: "Other board", ProjectID: s.otherProjectID}
	must(boardRepo.Create(ctx, otherBoard))
	s.otherBoard = otherBoard.ID

	// Effect with two fixtures.
	eff := &models.Effect{
		Name: "Pulse", ProjectID: s.projectID, EffectType: "WAVEFORM", Waveform: ptr("SINE"),
		Frequency: 2, Amplitude: 50, Offset: 50, PhaseOffset: 90,
		PriorityBand: "USER", PrioritySub: 50, CompositionMode: "OVERRIDE", OnCueChange: "FADE_OUT",
	}
	must(effectRepo.CreateWithFixtures(ctx, eff, []models.EffectFixture{
		{FixtureID: washA.ID}, {FixtureID: washB.ID},
	}))
	s.effect = eff.ID

	return s
}

// colIndex returns the index of header h in data's columns, failing the
// test when absent.
func colIndex(t *testing.T, data *TableData, h string) int {
	t.Helper()
	for i, c := range data.Columns {
		if c.Header == h {
			return i
		}
	}
	t.Fatalf("column %q not found in %v", h, headers(data.Columns))
	return -1
}

// rowBy returns the first row whose column h equals v.
func rowBy(t *testing.T, data *TableData, h string, v any) []any {
	t.Helper()
	i := colIndex(t, data, h)
	for _, r := range data.Rows {
		if r[i] == v {
			return r
		}
	}
	t.Fatalf("no row with %s=%v", h, v)
	return nil
}
