package resolvers

import (
	"context"
	"strings"
	"testing"

	"github.com/99designs/gqlgen/client"
	"github.com/bbernstein/lacylights-go/internal/database/models"
	"github.com/lucsky/cuid"
	"github.com/stretchr/testify/require"
)

const minimalEosShowfile = `Ident 3:0
Manufacturer ETC
Console Eos
$$Format 3.20
$$Title E2E Show
Clear All
Set Channels 5000

$ParamType 1 1 Intens

$Personality 9001
   $$Manuf Generic
   $$Model Dimmer
   $$Footprint 1
   $$PersChan 1 1 1 0 0

$Patch 1 1 9001
   Text Front Wash
$Patch 2 2 9001
   Text Back Wash

$CueList 1
   Text Main

Cue 1 1
   Text Open
   Up 5
   Down 5
   $$ChanMove 1@HFF 2@HFF
`

// TestImportProjectFromEos_E2E exercises the importProjectFromEos mutation
// end-to-end: through the GraphQL server, parser, mapper, and repositories.
func TestImportProjectFromEos_E2E(t *testing.T) {
	c, _, cleanup := testSetup(t)
	defer cleanup()

	var resp struct {
		ImportProjectFromEos struct {
			ProjectID               string
			FixtureInstancesCount   int
			CueListsCount           int
			CuesCount               int
			Warnings                []struct {
				Code     string
				Severity string
				Message  string
			}
		}
	}
	err := c.Post(`mutation Import($content: String!, $opts: EosImportOptionsInput) {
		importProjectFromEos(asciiContent: $content, options: $opts) {
			projectId
			fixtureInstancesCount
			cueListsCount
			cuesCount
			warnings { code severity message }
		}
	}`, &resp,
		client.Var("content", minimalEosShowfile),
		client.Var("opts", map[string]any{"newProjectName": "E2E Test"}),
	)
	if err != nil {
		t.Fatalf("graphql error: %v", err)
	}
	if resp.ImportProjectFromEos.ProjectID == "" {
		t.Fatal("expected project ID to be returned")
	}
	if resp.ImportProjectFromEos.FixtureInstancesCount != 2 {
		t.Errorf("FixtureInstancesCount = %d, want 2", resp.ImportProjectFromEos.FixtureInstancesCount)
	}
	if resp.ImportProjectFromEos.CueListsCount != 1 {
		t.Errorf("CueListsCount = %d, want 1", resp.ImportProjectFromEos.CueListsCount)
	}
	if resp.ImportProjectFromEos.CuesCount != 1 {
		t.Errorf("CuesCount = %d, want 1", resp.ImportProjectFromEos.CuesCount)
	}
}

// TestImportProjectFromEos_SynthesizedDefinitionIdsIsAlwaysList confirms the
// resolver coerces a nil SynthesizedDefinitionIDs from the import service into
// an empty list, satisfying the schema's non-null list contract.
func TestImportProjectFromEos_SynthesizedDefinitionIdsIsAlwaysList(t *testing.T) {
	c, _, cleanup := testSetup(t)
	defer cleanup()

	var resp struct {
		ImportProjectFromEos struct {
			ProjectID                string
			SynthesizedDefinitionIds []string
		}
	}
	err := c.Post(`mutation Import($content: String!, $opts: EosImportOptionsInput) {
		importProjectFromEos(asciiContent: $content, options: $opts) {
			projectId
			synthesizedDefinitionIds
		}
	}`, &resp,
		client.Var("content", minimalEosShowfile),
		client.Var("opts", map[string]any{"newProjectName": "Synth List Test"}),
	)
	if err != nil {
		t.Fatalf("graphql error: %v", err)
	}
	if resp.ImportProjectFromEos.SynthesizedDefinitionIds == nil {
		t.Errorf("synthesizedDefinitionIds must be a non-nil list, got nil")
	}
}

// TestImportProjectFromEos_RejectsOversizedContent verifies the resolver caps
// the size of asciiContent to prevent unbounded memory growth from a malicious
// or misconfigured client.
func TestImportProjectFromEos_RejectsOversizedContent(t *testing.T) {
	c, _, cleanup := testSetup(t)
	defer cleanup()

	// Build a payload just over the cap. Use a simple repeated character so
	// the payload itself is cheap to construct.
	oversized := strings.Repeat("X", maxEosContentBytes+1)

	var resp struct {
		ImportProjectFromEos struct {
			ProjectID string
		}
	}
	err := c.Post(`mutation Import($content: String!) {
		importProjectFromEos(asciiContent: $content) {
			projectId
		}
	}`, &resp, client.Var("content", oversized))
	if err == nil {
		t.Fatal("expected oversized payload to be rejected")
	}
	if !strings.Contains(err.Error(), "exceeds maximum size") {
		t.Errorf("error %q missing 'exceeds maximum size'", err.Error())
	}
}

// TestImportProjectFromEos_RejectsConflictingOptions verifies the resolver
// rejects requests that pass both newProjectName and targetProjectId.
func TestImportProjectFromEos_RejectsConflictingOptions(t *testing.T) {
	c, _, cleanup := testSetup(t)
	defer cleanup()

	var resp struct {
		ImportProjectFromEos struct {
			ProjectID string
		}
	}
	err := c.Post(`mutation Import($content: String!, $opts: EosImportOptionsInput) {
		importProjectFromEos(asciiContent: $content, options: $opts) {
			projectId
		}
	}`, &resp,
		client.Var("content", minimalEosShowfile),
		client.Var("opts", map[string]any{
			"newProjectName":  "Conflict",
			"targetProjectId": "some-project",
		}),
	)
	if err == nil {
		t.Fatal("expected error when both options are set")
	}
	if !strings.Contains(err.Error(), "mutually exclusive") {
		t.Errorf("error %q missing 'mutually exclusive'", err.Error())
	}
}

// TestExportProjectToEos_RejectsUnauthorizedUser confirms ensureProjectAccess
// is enforced before the export runs: a user who is not a member of the
// project's group must not be able to export it.
func TestExportProjectToEos_RejectsUnauthorizedUser(t *testing.T) {
	resolver, cleanup := setupGroupAdminTestResolver(t, true)
	defer cleanup()

	// Project belongs to group G1; user is a member of group G2.
	groupID := cuid.New()
	otherGroupID := cuid.New()
	project := &models.Project{ID: cuid.New(), Name: "Owned by G1", GroupID: &groupID}
	require.NoError(t, resolver.ProjectRepo.Create(context.Background(), project))

	mut := resolver.Mutation().(*mutationResolver)
	ctx := createMemberContext("user-1", "user@test.com", otherGroupID)
	res, err := mut.ExportProjectToEos(ctx, project.ID)
	if err == nil {
		t.Fatalf("expected unauthorized error, got result %+v", res)
	}
	if !strings.Contains(err.Error(), "not authorized") {
		t.Errorf("expected 'not authorized' in error, got %q", err.Error())
	}
}

// TestExportProjectToEos_E2E imports a project, exports it via GraphQL, and
// verifies key sections of the output.
func TestExportProjectToEos_E2E(t *testing.T) {
	c, _, cleanup := testSetup(t)
	defer cleanup()

	var imp struct {
		ImportProjectFromEos struct {
			ProjectID string
		}
	}
	if err := c.Post(`mutation Import($content: String!, $opts: EosImportOptionsInput) {
		importProjectFromEos(asciiContent: $content, options: $opts) {
			projectId
		}
	}`, &imp,
		client.Var("content", minimalEosShowfile),
		client.Var("opts", map[string]any{"newProjectName": "E2E Roundtrip"}),
	); err != nil {
		t.Fatalf("import: %v", err)
	}
	pid := imp.ImportProjectFromEos.ProjectID
	if pid == "" {
		t.Fatal("expected project ID from import")
	}

	var exp struct {
		ExportProjectToEos struct {
			ProjectID      string
			ProjectName    string
			ASCIIContent   string
			FilenameSuffix string
		}
	}
	if err := c.Post(`mutation Export($id: ID!) {
		exportProjectToEos(projectId: $id) {
			projectId
			projectName
			asciiContent
			filenameSuffix
		}
	}`, &exp, client.Var("id", pid)); err != nil {
		t.Fatalf("export: %v", err)
	}

	if exp.ExportProjectToEos.FilenameSuffix != ".asc" {
		t.Errorf("FilenameSuffix = %q, want .asc", exp.ExportProjectToEos.FilenameSuffix)
	}
	for _, want := range []string{
		"Ident 3:0",
		"$$Title E2E Roundtrip",
		"$Personality 9001",
		"$Patch 1",
		"$CueList 1",
	} {
		if !strings.Contains(exp.ExportProjectToEos.ASCIIContent, want) {
			t.Errorf("export missing %q", want)
		}
	}
}
