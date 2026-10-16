package resolvers

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/99designs/gqlgen/client"

	"github.com/bbernstein/lacylights-go/internal/database/models"
	"github.com/bbernstein/lacylights-go/internal/graphql/generated"
)

// withVars turns alternating name/value pairs into GraphQL variables.
func withVars(kv ...string) []client.Option {
	opts := make([]client.Option, 0, len(kv)/2)
	for i := 0; i+1 < len(kv); i += 2 {
		opts = append(opts, client.Var(kv[i], kv[i+1]))
	}
	return opts
}

const createDimmerMutation = `mutation($mfr: String!, $model: String!) {
	createFixtureDefinition(input: {
		manufacturer: $mfr, model: $model, type: DIMMER,
		channels: [{ name: "Dimmer", type: INTENSITY, offset: 0, minValue: 0, maxValue: 255, defaultValue: 0 }]
	}) { id source sourceId sourceVersion }
}`

type createDefResp struct {
	CreateFixtureDefinition struct {
		ID            string  `json:"id"`
		Source        string  `json:"source"`
		SourceID      *string `json:"sourceId"`
		SourceVersion *string `json:"sourceVersion"`
	} `json:"createFixtureDefinition"`
}

func seedResolverDef(t *testing.T, r *Resolver, source, mfr, model string) string {
	t.Helper()
	def := &models.FixtureDefinition{Manufacturer: mfr, Model: model, Type: "DIMMER", Source: source}
	if err := r.FixtureRepo.CreateDefinition(context.Background(), def); err != nil {
		t.Fatal(err)
	}
	return def.ID
}

func TestCreateFixtureDefinition_SourceIsUser(t *testing.T) {
	c, _, cleanup := testSetup(t)
	defer cleanup()
	var resp createDefResp
	if err := c.Post(createDimmerMutation, &resp, withVars("mfr", "Acme", "model", "Dim")...); err != nil {
		t.Fatal(err)
	}
	got := resp.CreateFixtureDefinition
	if got.Source != "USER" || got.SourceID != nil || got.SourceVersion != nil {
		t.Errorf("source = %q %v %v", got.Source, got.SourceID, got.SourceVersion)
	}
}

func TestCreateFixtureDefinition_DuplicateCheckIsPerSource(t *testing.T) {
	c, r, cleanup := testSetup(t)
	defer cleanup()
	seedResolverDef(t, r, models.FixtureSourceOFL, "Acme", "Dim")

	var resp createDefResp
	if err := c.Post(createDimmerMutation, &resp, withVars("mfr", "Acme", "model", "Dim")...); err != nil {
		t.Fatalf("user definition alongside OFL one should be allowed: %v", err)
	}
	err := c.Post(createDimmerMutation, &resp, withVars("mfr", "Acme", "model", "Dim")...)
	if err == nil || !strings.Contains(err.Error(), "already exists") {
		t.Errorf("second user definition: err = %v, want already exists", err)
	}
}

func TestUpdateFixtureDefinition_RenameConflictIsPerSource(t *testing.T) {
	c, r, cleanup := testSetup(t)
	defer cleanup()
	seedResolverDef(t, r, models.FixtureSourceOFL, "Acme", "Taken")
	var created createDefResp
	if err := c.Post(createDimmerMutation, &created, withVars("mfr", "Acme", "model", "Mine")...); err != nil {
		t.Fatal(err)
	}
	var resp struct {
		UpdateFixtureDefinition struct {
			Model string `json:"model"`
		} `json:"updateFixtureDefinition"`
	}
	err := c.Post(`mutation($id: ID!) {
		updateFixtureDefinition(id: $id, input: {
			manufacturer: "Acme", model: "Taken", type: DIMMER,
			channels: [{ name: "Dimmer", type: INTENSITY, offset: 0, minValue: 0, maxValue: 255, defaultValue: 0 }]
		}) { model }
	}`, &resp, withVars("id", created.CreateFixtureDefinition.ID)...)
	if err != nil {
		t.Fatalf("renaming onto another source's model should be allowed: %v", err)
	}
	if resp.UpdateFixtureDefinition.Model != "Taken" {
		t.Errorf("model = %q", resp.UpdateFixtureDefinition.Model)
	}
}

func TestFixtureDefinitions_FilterBySource(t *testing.T) {
	c, r, cleanup := testSetup(t)
	defer cleanup()
	oflID := seedResolverDef(t, r, models.FixtureSourceOFL, "Acme", "A")
	seedResolverDef(t, r, models.FixtureSourceUser, "Acme", "B")

	var resp struct {
		FixtureDefinitions []struct {
			ID     string `json:"id"`
			Source string `json:"source"`
		} `json:"fixtureDefinitions"`
	}
	if err := c.Post(`query { fixtureDefinitions(filter: { source: OFL }) { id source } }`, &resp); err != nil {
		t.Fatal(err)
	}
	if len(resp.FixtureDefinitions) != 1 || resp.FixtureDefinitions[0].ID != oflID || resp.FixtureDefinitions[0].Source != "OFL" {
		t.Errorf("filtered = %+v", resp.FixtureDefinitions)
	}
}

func TestFixtureSourcePrecedence_QueryAndSet(t *testing.T) {
	c, _, cleanup := testSetup(t)
	defer cleanup()

	var q struct {
		FixtureSourcePrecedence []string `json:"fixtureSourcePrecedence"`
	}
	if err := c.Post(`query { fixtureSourcePrecedence }`, &q); err != nil {
		t.Fatal(err)
	}
	if strings.Join(q.FixtureSourcePrecedence, ",") != "USER,GDTF,OFL,BUILTIN,EOS,SYNTHESIZED" {
		t.Errorf("default = %v", q.FixtureSourcePrecedence)
	}

	var m struct {
		SetFixtureSourcePrecedence []string `json:"setFixtureSourcePrecedence"`
	}
	if err := c.Post(`mutation { setFixtureSourcePrecedence(order: [OFL, GDTF]) }`, &m); err != nil {
		t.Fatal(err)
	}
	if strings.Join(m.SetFixtureSourcePrecedence, ",") != "OFL,GDTF,USER,BUILTIN,EOS,SYNTHESIZED" {
		t.Errorf("set = %v", m.SetFixtureSourcePrecedence)
	}
	if err := c.Post(`query { fixtureSourcePrecedence }`, &q); err != nil {
		t.Fatal(err)
	}
	if q.FixtureSourcePrecedence[0] != "OFL" {
		t.Errorf("reloaded = %v", q.FixtureSourcePrecedence)
	}

	err := c.Post(`mutation { setFixtureSourcePrecedence(order: [OFL, OFL]) }`, &m)
	if err == nil {
		t.Error("expected error for duplicate source")
	}
}

func TestSetFixtureSourcePrecedence_RequiresAdmin(t *testing.T) {
	r, cleanup := setupGroupAdminTestResolver(t, true)
	defer cleanup()
	m := r.Mutation().(*mutationResolver)
	order := []generated.FixtureSource{generated.FixtureSourceOfl}

	if _, err := m.SetFixtureSourcePrecedence(context.Background(), order); !errors.Is(err, ErrNotAuthenticated) {
		t.Errorf("unauthenticated: err = %v, want ErrNotAuthenticated", err)
	}
	if _, err := m.SetFixtureSourcePrecedence(createMemberContext("u", "u@test.com", "g"), order); !errors.Is(err, ErrNotAuthorized) {
		t.Errorf("non-admin: err = %v, want ErrNotAuthorized", err)
	}
	got, err := m.SetFixtureSourcePrecedence(createAdminContext(), order)
	if err != nil {
		t.Fatalf("admin: %v", err)
	}
	if len(got) == 0 || got[0] != generated.FixtureSourceOfl {
		t.Errorf("admin result = %v, want OFL first", got)
	}
}
