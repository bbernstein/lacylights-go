package resolvers

import (
	"context"
	"testing"
	"time"

	"github.com/99designs/gqlgen/client"
	"github.com/lucsky/cuid"
	"github.com/stretchr/testify/require"

	"github.com/bbernstein/lacylights-go/internal/database/models"
)

// TestProjectExportQuery checks Query.projectExport, including option
// handling.
func TestProjectExportQuery(t *testing.T) {
	c, r, cleanup := testSetup(t)
	defer cleanup()
	s := seedDataExport(t, r, nil, nil)

	type exportResult struct {
		ProjectID   string
		ProjectName string
		JSONContent string
		Stats       struct {
			FixtureInstancesCount int
			LooksCount            int
			CueListsCount         int
		}
	}
	const fields = `projectId projectName jsonContent stats { fixtureInstancesCount looksCount cueListsCount }`

	var q struct{ ProjectExport exportResult }
	require.NoError(t, c.Post(`query($id: ID!) { projectExport(projectId: $id) { `+fields+` } }`,
		&q, client.Var("id", s.projectID)))

	require.Equal(t, s.projectID, q.ProjectExport.ProjectID)
	require.Equal(t, "Export Show", q.ProjectExport.ProjectName)
	require.Equal(t, 1, q.ProjectExport.Stats.FixtureInstancesCount)
	require.Equal(t, 1, q.ProjectExport.Stats.LooksCount)
	require.Equal(t, 1, q.ProjectExport.Stats.CueListsCount)
	require.NotEmpty(t, q.ProjectExport.JSONContent)

	// Options are honoured.
	var opt struct{ ProjectExport exportResult }
	require.NoError(t, c.Post(`query($id: ID!, $o: ExportOptionsInput) { projectExport(projectId: $id, options: $o) { `+fields+` } }`,
		&opt, client.Var("id", s.projectID),
		client.Var("o", map[string]any{"includeFixtures": false, "includeLooks": false, "includeCueLists": false})))
	require.Equal(t, 0, opt.ProjectExport.Stats.FixtureInstancesCount)
	require.Equal(t, 0, opt.ProjectExport.Stats.LooksCount)
	require.Equal(t, 0, opt.ProjectExport.Stats.CueListsCount)

	// Unknown project.
	var missing struct{ ProjectExport exportResult }
	err := c.Post(`query { projectExport(projectId: "nope") { projectId } }`, &missing)
	require.ErrorContains(t, err, "project not found")
}

func TestProjectExportEosQuery(t *testing.T) {
	c, _, cleanup := testSetup(t)
	defer cleanup()

	var imp struct{ ImportProjectFromEos struct{ ProjectID string } }
	require.NoError(t, c.Post(`mutation($content: String!, $opts: EosImportOptionsInput) {
		importProjectFromEos(asciiContent: $content, options: $opts) { projectId }
	}`, &imp, client.Var("content", minimalEosShowfile), client.Var("opts", map[string]any{"newProjectName": "Query Eos"})))

	type eosResult struct {
		ProjectID      string
		ProjectName    string
		ASCIIContent   string
		FilenameSuffix string
	}
	var q struct{ ProjectExportEos eosResult }
	require.NoError(t, c.Post(`query($id: ID!) {
		projectExportEos(projectId: $id) { projectId projectName asciiContent filenameSuffix }
	}`, &q, client.Var("id", imp.ImportProjectFromEos.ProjectID)))
	var again struct{ ProjectExportEos eosResult }
	require.NoError(t, c.Post(`query($id: ID!) {
		projectExportEos(projectId: $id) { projectId projectName asciiContent filenameSuffix }
	}`, &again, client.Var("id", imp.ImportProjectFromEos.ProjectID)))

	require.Equal(t, ".asc", q.ProjectExportEos.FilenameSuffix)
	require.Equal(t, "Query Eos", q.ProjectExportEos.ProjectName)
	require.Contains(t, q.ProjectExportEos.ASCIIContent, "$$Title Query Eos")
	// The Eos writer is deterministic and group numbers are saved on the
	// first export, so a second export is identical.
	require.Equal(t, q.ProjectExportEos, again.ProjectExportEos)
}

func TestProjectExportQLCQuery(t *testing.T) {
	c, r, cleanup := testSetup(t)
	defer cleanup()
	s := seedDataExport(t, r, nil, nil)

	// QLC+ export is not available on this platform.
	var q struct{ ProjectExportQLC struct{ XMLContent string } }
	err := c.Post(`query($id: ID!, $m: [FixtureMappingInput!]) {
		projectExportQLC(projectId: $id, fixtureMappings: $m) { xmlContent }
	}`, &q, client.Var("id", s.projectID), client.Var("m", []map[string]any{}))
	require.ErrorContains(t, err, "QLC+ export not available")
}

func TestProjectExportQueries_EosUnavailable(t *testing.T) {
	_, r, cleanup := testSetup(t)
	defer cleanup()
	r.EosExportService = nil
	_, err := r.Query().(*queryResolver).ProjectExportEos(context.Background(), "p")
	require.ErrorContains(t, err, "not available")
}

// TestProjectExportQueries_AccessControl confirms every export Query
// enforces ensureProjectAccess.
func TestProjectExportQueries_AccessControl(t *testing.T) {
	r, cleanup := setupGroupAdminTestResolver(t, true)
	defer cleanup()

	groupID := cuid.New()
	otherGroupID := cuid.New()
	project := &models.Project{ID: cuid.New(), Name: "Owned by G1", GroupID: &groupID}
	require.NoError(t, r.ProjectRepo.Create(context.Background(), project))

	q := r.Query().(*queryResolver)
	outsider := createMemberContext("outsider", "o@test.com", otherGroupID)

	_, err := q.ProjectExport(outsider, project.ID, nil)
	require.ErrorIs(t, err, ErrNotAuthorized)
	_, err = q.ProjectExportQlc(outsider, project.ID, nil)
	require.ErrorIs(t, err, ErrNotAuthorized)
	_, err = q.ProjectExportEos(outsider, project.ID)
	require.ErrorIs(t, err, ErrNotAuthorized)

	anon := context.Background()
	_, err = q.ProjectExport(anon, project.ID, nil)
	require.ErrorIs(t, err, ErrNotAuthenticated)
	_, err = q.ProjectExportQlc(anon, project.ID, nil)
	require.ErrorIs(t, err, ErrNotAuthenticated)
	_, err = q.ProjectExportEos(anon, project.ID)
	require.ErrorIs(t, err, ErrNotAuthenticated)

	member := createMemberContext("member", "m@test.com", groupID)
	res, err := q.ProjectExport(member, project.ID, nil)
	require.NoError(t, err)
	require.Equal(t, "Owned by G1", res.ProjectName)
	eos, err := q.ProjectExportEos(member, project.ID)
	require.NoError(t, err)
	require.Equal(t, project.ID, eos.ProjectID)
}

// TestExportMutations_Removed checks the old export Mutations are gone
// from the schema (lacylights-go#87) while the import Mutations and the
// export Queries remain.
func TestExportMutations_Removed(t *testing.T) {
	c, _, cleanup := testSetup(t)
	defer cleanup()

	type fieldList struct {
		Fields []struct{ Name string }
	}
	var resp struct {
		Mutation fieldList `json:"mutation"`
		Query    fieldList `json:"query"`
	}
	require.NoError(t, c.Post(`{
		mutation: __type(name: "Mutation") { fields(includeDeprecated: true) { name } }
		query: __type(name: "Query") { fields(includeDeprecated: true) { name } }
	}`, &resp))
	names := func(l fieldList) map[string]bool {
		m := map[string]bool{}
		for _, f := range l.Fields {
			m[f.Name] = true
		}
		return m
	}
	mutations, queries := names(resp.Mutation), names(resp.Query)

	for _, removed := range []string{"exportProject", "exportProjectToQLC", "exportProjectToEos"} {
		require.False(t, mutations[removed], "Mutation.%s should be removed", removed)
	}
	for _, kept := range []string{"importProject", "importProjectFromQLC", "importProjectFromEos"} {
		require.True(t, mutations[kept], "Mutation.%s should remain", kept)
	}
	for _, kept := range []string{"projectExport", "projectExportQLC", "projectExportEos"} {
		require.True(t, queries[kept], "Query.%s should remain", kept)
	}

	// Calling a removed Mutation is a validation error.
	var out map[string]any
	err := c.Post(`mutation { exportProject(projectId: "p") { projectId } }`, &out)
	require.ErrorContains(t, err, "Cannot query field")
	require.ErrorContains(t, err, "exportProject")
	require.ErrorContains(t, err, "GRAPHQL_VALIDATION_FAILED")
}

// TestAcquireProjectExportSlot checks the shared project export bound:
// at most MaxConcurrentProjectExports run at once, JSON and Eos exports
// wait for a slot while their context lives, and releasing frees a slot.
func TestAcquireProjectExportSlot(t *testing.T) {
	r, cleanup := setupGroupAdminTestResolver(t, false)
	defer cleanup()
	project := &models.Project{ID: cuid.New(), Name: "Busy"}
	require.NoError(t, r.ProjectRepo.Create(context.Background(), project))

	cancelled, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := acquireProjectExportSlot(cancelled)
	require.ErrorIs(t, err, context.Canceled)

	releases := make([]func(), 0, MaxConcurrentProjectExports)
	for range MaxConcurrentProjectExports {
		release, err := acquireProjectExportSlot(context.Background())
		require.NoError(t, err)
		releases = append(releases, release)
	}

	short := func() context.Context {
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
		t.Cleanup(cancel)
		return ctx
	}
	_, err = acquireProjectExportSlot(short())
	require.ErrorIs(t, err, context.DeadlineExceeded)
	_, err = r.exportProjectJSON(short(), project.ID, nil)
	require.ErrorIs(t, err, context.DeadlineExceeded)
	_, err = r.exportProjectEos(short(), project.ID)
	require.ErrorIs(t, err, context.DeadlineExceeded)

	releases[0]()
	res, err := r.exportProjectJSON(context.Background(), project.ID, nil)
	require.NoError(t, err)
	require.Equal(t, "Busy", res.ProjectName)
	for _, rel := range releases[1:] {
		rel()
	}
	// Every slot is free again.
	for range MaxConcurrentProjectExports {
		release, err := acquireProjectExportSlot(context.Background())
		require.NoError(t, err)
		defer release()
	}
}
