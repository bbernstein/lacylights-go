package resolvers

import (
	"context"
	"testing"

	"github.com/99designs/gqlgen/client"
	"github.com/lucsky/cuid"
	"github.com/stretchr/testify/require"

	"github.com/bbernstein/lacylights-go/internal/database/models"
)

// TestProjectExportQuery_MatchesMutation checks that Query.projectExport
// returns the same result as the deprecated exportProject mutation,
// including option handling.
func TestProjectExportQuery_MatchesMutation(t *testing.T) {
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
	var m struct{ ExportProject exportResult }
	require.NoError(t, c.Post(`mutation($id: ID!) { exportProject(projectId: $id) { `+fields+` } }`,
		&m, client.Var("id", s.projectID)))

	require.Equal(t, s.projectID, q.ProjectExport.ProjectID)
	require.Equal(t, "Export Show", q.ProjectExport.ProjectName)
	require.Equal(t, 1, q.ProjectExport.Stats.FixtureInstancesCount)
	require.Equal(t, 1, q.ProjectExport.Stats.LooksCount)
	require.Equal(t, 1, q.ProjectExport.Stats.CueListsCount)
	require.NotEmpty(t, q.ProjectExport.JSONContent)
	require.Equal(t, m.ExportProject.Stats, q.ProjectExport.Stats)
	require.Equal(t, m.ExportProject.ProjectName, q.ProjectExport.ProjectName)

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
	var m struct{ ExportProjectToEos eosResult }
	require.NoError(t, c.Post(`mutation($id: ID!) {
		exportProjectToEos(projectId: $id) { projectId projectName asciiContent filenameSuffix }
	}`, &m, client.Var("id", imp.ImportProjectFromEos.ProjectID)))

	require.Equal(t, ".asc", q.ProjectExportEos.FilenameSuffix)
	require.Equal(t, "Query Eos", q.ProjectExportEos.ProjectName)
	require.Contains(t, q.ProjectExportEos.ASCIIContent, "$$Title Query Eos")
	// The Eos writer is deterministic, so query and mutation agree exactly.
	require.Equal(t, m.ExportProjectToEos, q.ProjectExportEos)
}

func TestProjectExportQLCQuery(t *testing.T) {
	c, r, cleanup := testSetup(t)
	defer cleanup()
	s := seedDataExport(t, r, nil, nil)

	// QLC+ export is not available on this platform; the query mirrors the
	// mutation's behaviour.
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
// enforces ensureProjectAccess like the Mutations do.
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

	_, err = q.ProjectExport(context.Background(), project.ID, nil)
	require.ErrorIs(t, err, ErrNotAuthenticated)

	member := createMemberContext("member", "m@test.com", groupID)
	res, err := q.ProjectExport(member, project.ID, nil)
	require.NoError(t, err)
	require.Equal(t, "Owned by G1", res.ProjectName)
	eos, err := q.ProjectExportEos(member, project.ID)
	require.NoError(t, err)
	require.Equal(t, project.ID, eos.ProjectID)
}

// TestExportMutations_Deprecated checks the deprecation is visible through
// introspection so client tooling can warn.
func TestExportMutations_Deprecated(t *testing.T) {
	c, _, cleanup := testSetup(t)
	defer cleanup()

	var resp struct {
		Type struct {
			Fields []struct {
				Name              string
				IsDeprecated      bool
				DeprecationReason *string
			}
		} `json:"__type"`
	}
	require.NoError(t, c.Post(`{ __type(name: "Mutation") { fields(includeDeprecated: true) { name isDeprecated deprecationReason } } }`, &resp))
	deprecated := map[string]string{}
	for _, f := range resp.Type.Fields {
		if f.IsDeprecated && f.DeprecationReason != nil {
			deprecated[f.Name] = *f.DeprecationReason
		}
	}
	for _, name := range []string{"exportProject", "exportProjectToQLC", "exportProjectToEos"} {
		require.Equal(t, "Use Query.projectExport*", deprecated[name], name)
	}
	require.NotContains(t, deprecated, "importProject")
}
