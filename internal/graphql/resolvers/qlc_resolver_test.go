package resolvers

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/99designs/gqlgen/client"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/bbernstein/lacylights-go/internal/middleware"
)

func readQLCTestdata(t *testing.T, name string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("..", "..", "services", "import_qlc", "testdata", name))
	require.NoError(t, err)
	return string(b)
}

// importProjectFromQLCMutation is the operation lacylights-fe sends
// (IMPORT_PROJECT_FROM_QLC in src/graphql/projects.ts).
const importProjectFromQLCMutation = `mutation ImportProjectFromQLC($xmlContent: String!, $originalFileName: String!) {
  importProjectFromQLC(xmlContent: $xmlContent, originalFileName: $originalFileName) {
    project { id name description createdAt updatedAt }
    originalFileName
    fixtureCount
    lookCount
    cueListCount
    warnings
  }
}`

type qlcImportResponse struct {
	ImportProjectFromQLC struct {
		Project struct {
			ID          string
			Name        string
			Description *string
			CreatedAt   string
			UpdatedAt   string
		}
		OriginalFileName string
		FixtureCount     int
		LookCount        int
		CueListCount     int
		Warnings         []string
	}
}

// TestImportProjectFromQLC_E2E is the regression test for
// bbernstein/lacylights-go#91: the mutation used to always fail with
// "QLC+ import not available on this platform".
func TestImportProjectFromQLC_E2E(t *testing.T) {
	c, _, cleanup := testSetup(t)
	defer cleanup()

	var resp qlcImportResponse
	err := c.Post(importProjectFromQLCMutation, &resp,
		client.Var("xmlContent", readQLCTestdata(t, "issue91_workspace.qxw")),
		client.Var("originalFileName", "My Show.qxw"),
	)
	require.NoError(t, err)

	r := resp.ImportProjectFromQLC
	assert.NotEmpty(t, r.Project.ID)
	assert.Equal(t, "My Show", r.Project.Name)
	require.NotNil(t, r.Project.Description)
	assert.Contains(t, *r.Project.Description, "My Show.qxw")
	assert.Equal(t, "My Show.qxw", r.OriginalFileName)
	assert.Equal(t, 4, r.FixtureCount)
	assert.Equal(t, 2, r.LookCount)
	assert.Equal(t, 1, r.CueListCount)
	assert.NotNil(t, r.Warnings)

	// The project is readable through the API. Fields are fetched one query
	// at a time: testSetup uses an in-memory SQLite database, and gqlgen
	// resolves sibling fields concurrently on separate (empty) connections.
	var fixtures struct {
		Project struct{ Fixtures []struct{ Name string } }
	}
	require.NoError(t, c.Post(`query P($id: ID!) { project(id: $id) { fixtures { name } } }`,
		&fixtures, client.Var("id", r.Project.ID)))
	assert.Len(t, fixtures.Project.Fixtures, 4)

	var cueLists struct {
		Project struct {
			CueLists []struct {
				Name string
				Loop bool
				Cues []struct {
					CueNumber  float64
					FadeInTime float64
					FollowTime *float64
				}
			}
		}
	}
	require.NoError(t, c.Post(`query P($id: ID!) { project(id: $id) {
		cueLists { name loop cues { cueNumber fadeInTime followTime } } } }`,
		&cueLists, client.Var("id", r.Project.ID)))
	require.Len(t, cueLists.Project.CueLists, 1)
	cl := cueLists.Project.CueLists[0]
	assert.Equal(t, "Full Show Cuelist", cl.Name)
	assert.True(t, cl.Loop)
	require.Len(t, cl.Cues, 4)
	assert.InDelta(t, 2.5, cl.Cues[2].FadeInTime, 1e-9)
	require.NotNil(t, cl.Cues[0].FollowTime)
	assert.InDelta(t, 1.0, *cl.Cues[0].FollowTime, 1e-9)
	assert.Nil(t, cl.Cues[3].FollowTime, "infinite hold waits for GO")
}

func TestImportProjectFromQLC_InvalidContent(t *testing.T) {
	c, _, cleanup := testSetup(t)
	defer cleanup()

	var resp qlcImportResponse
	err := c.Post(importProjectFromQLCMutation, &resp,
		client.Var("xmlContent", `{"not": "xml"}`),
		client.Var("originalFileName", "bad.qxw"),
	)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid QLC+ workspace")
}

func TestImportProjectFromQLC_RejectsOversizedContent(t *testing.T) {
	c, _, cleanup := testSetup(t)
	defer cleanup()

	var resp qlcImportResponse
	err := c.Post(importProjectFromQLCMutation, &resp,
		client.Var("xmlContent", strings.Repeat("X", maxQLCContentBytes+1)),
		client.Var("originalFileName", "huge.qxw"),
	)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "exceeds the 50 MiB maximum")
}

func TestImportProjectFromQLC_ServiceNotWired(t *testing.T) {
	_, resolver, cleanup := testSetup(t)
	defer cleanup()
	resolver.QLCImportService = nil

	_, err := (&mutationResolver{resolver}).ImportProjectFromQlc(context.Background(), "<Workspace/>", "a.qxw")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not available")
}

func TestImportProjectFromQLC_AuthAssignsCallerGroup(t *testing.T) {
	resolver, cleanup := setupGroupAdminTestResolver(t, true)
	defer cleanup()
	group := createTestGroup(t, resolver, "Lighting Crew")
	ctx := createMemberContext("user-1", "member@example.com", group.ID)

	res, err := (&mutationResolver{resolver}).ImportProjectFromQlc(ctx, readQLCTestdata(t, "issue91_workspace.qxw"), "show.qxw")
	require.NoError(t, err)

	p, err := resolver.ProjectRepo.FindByID(context.Background(), res.Project.ID)
	require.NoError(t, err)
	require.NotNil(t, p.GroupID)
	assert.Equal(t, group.ID, *p.GroupID)
}

func TestImportProjectFromQLC_AuthRequiresGroup(t *testing.T) {
	resolver, cleanup := setupGroupAdminTestResolver(t, true)
	defer cleanup()
	ctx := context.WithValue(context.Background(), middleware.ContextKeyUserRole, "USER")

	_, err := (&mutationResolver{resolver}).ImportProjectFromQlc(ctx, readQLCTestdata(t, "issue91_workspace.qxw"), "show.qxw")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "does not belong to any groups")
}

func TestImportProjectFromQLC_AuthMultipleGroupsWarns(t *testing.T) {
	resolver, cleanup := setupGroupAdminTestResolver(t, true)
	defer cleanup()
	first := createTestGroup(t, resolver, "First")
	second := createTestGroup(t, resolver, "Second")
	ctx := createMemberContext("user-1", "member@example.com", first.ID)
	ctx = context.WithValue(ctx, middleware.ContextKeyUserGroups, []middleware.UserGroupMembership{
		{GroupID: first.ID, Role: "MEMBER"},
		{GroupID: second.ID, Role: "MEMBER"},
	})

	res, err := (&mutationResolver{resolver}).ImportProjectFromQlc(ctx, readQLCTestdata(t, "issue91_workspace.qxw"), "show.qxw")
	require.NoError(t, err)
	assert.Contains(t, strings.Join(res.Warnings, "\n"), "first group")
}
