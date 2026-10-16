package resolvers

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/99designs/gqlgen/client"
)

const importGdtfMutation = `mutation($files: [GdtfFileInput!]!, $options: GdtfImportOptions) {
	importGdtfFixtures(files: $files, options: $options) {
		createdCount updatedCount unchangedCount skippedCount failedCount
		results {
			fileName status manufacturer model modeCount warnings error
			definition { id manufacturer model type source sourceId sourceVersion modes { name channelCount } }
		}
	}
}`

type gdtfImportResp struct {
	ImportGdtfFixtures struct {
		CreatedCount   int `json:"createdCount"`
		UpdatedCount   int `json:"updatedCount"`
		UnchangedCount int `json:"unchangedCount"`
		SkippedCount   int `json:"skippedCount"`
		FailedCount    int `json:"failedCount"`
		Results        []struct {
			FileName     string   `json:"fileName"`
			Status       string   `json:"status"`
			Manufacturer *string  `json:"manufacturer"`
			Model        *string  `json:"model"`
			ModeCount    *int     `json:"modeCount"`
			Warnings     []string `json:"warnings"`
			Error        *string  `json:"error"`
			Definition   *struct {
				ID            string  `json:"id"`
				Manufacturer  string  `json:"manufacturer"`
				Model         string  `json:"model"`
				Type          string  `json:"type"`
				Source        string  `json:"source"`
				SourceID      *string `json:"sourceId"`
				SourceVersion *string `json:"sourceVersion"`
				Modes         []struct {
					Name         string `json:"name"`
					ChannelCount int    `json:"channelCount"`
				} `json:"modes"`
			} `json:"definition"`
		} `json:"results"`
	} `json:"importGdtfFixtures"`
}

func gdtfTestdata(t *testing.T, name string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("..", "..", "services", "gdtf", "testdata", name))
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func gdtfFiles(kv ...string) []map[string]any {
	out := make([]map[string]any, 0, len(kv)/2)
	for i := 0; i+1 < len(kv); i += 2 {
		out = append(out, map[string]any{"fileName": kv[i], "descriptionXml": kv[i+1]})
	}
	return out
}

func TestImportGdtfFixtures(t *testing.T) {
	c, _, cleanup := testSetup(t)
	defer cleanup()
	par := gdtfTestdata(t, "rgbw_par.xml")

	var resp gdtfImportResp
	err := c.Post(importGdtfMutation, &resp,
		client.Var("files", gdtfFiles("par.gdtf", par, "bad.gdtf", "<nope")))
	if err != nil {
		t.Fatal(err)
	}
	got := resp.ImportGdtfFixtures
	if got.CreatedCount != 1 || got.FailedCount != 1 || len(got.Results) != 2 {
		t.Fatalf("result = %+v", got)
	}
	ok := got.Results[0]
	if ok.Status != "CREATED" || ok.Definition == nil || ok.ModeCount == nil || *ok.ModeCount != 2 || ok.Error != nil {
		t.Fatalf("par result = %+v", ok)
	}
	def := ok.Definition
	if def.Source != "GDTF" || def.SourceID == nil || *def.SourceID != "5A1C0B7E-0000-4000-8000-000000000001" ||
		def.Type != "LED_PAR" || def.Model != "Test Par RGBW" || len(def.Modes) != 2 {
		t.Errorf("definition = %+v", def)
	}
	if def.SourceVersion == nil || !strings.HasPrefix(*def.SourceVersion, "1.2") {
		t.Errorf("sourceVersion = %v", def.SourceVersion)
	}
	bad := got.Results[1]
	if bad.Status != "FAILED" || bad.Error == nil || bad.Definition != nil || bad.ModeCount != nil || bad.Warnings == nil {
		t.Errorf("bad result = %+v", bad)
	}

	// Changed file with onExisting: SKIP.
	changed := strings.Replace(par, `<DMXMode Name="4ch"`, `<DMXMode Name="Four"`, 1)
	var skip gdtfImportResp
	err = c.Post(importGdtfMutation, &skip,
		client.Var("files", gdtfFiles("par.gdtf", changed)),
		client.Var("options", map[string]any{"onExisting": "SKIP"}))
	if err != nil {
		t.Fatal(err)
	}
	if r := skip.ImportGdtfFixtures.Results[0]; r.Status != "SKIPPED" || r.Definition == nil || r.Definition.ID != def.ID || skip.ImportGdtfFixtures.SkippedCount != 1 {
		t.Errorf("skip result = %+v", skip.ImportGdtfFixtures)
	}

	// Default policy UPDATE keeps the definition ID.
	var upd gdtfImportResp
	if err := c.Post(importGdtfMutation, &upd, client.Var("files", gdtfFiles("par.gdtf", changed))); err != nil {
		t.Fatal(err)
	}
	r := upd.ImportGdtfFixtures.Results[0]
	if r.Status != "UPDATED" || r.Definition == nil || r.Definition.ID != def.ID || upd.ImportGdtfFixtures.UpdatedCount != 1 {
		t.Fatalf("update result = %+v", upd.ImportGdtfFixtures)
	}
	names := []string{r.Definition.Modes[0].Name, r.Definition.Modes[1].Name}
	if !strings.Contains(strings.Join(names, ","), "Four") {
		t.Errorf("modes after update = %v", names)
	}

	// Same content again: UNCHANGED.
	var same gdtfImportResp
	if err := c.Post(importGdtfMutation, &same, client.Var("files", gdtfFiles("par.gdtf", changed)),
		client.Var("options", map[string]any{"onExisting": "UPDATE"})); err != nil {
		t.Fatal(err)
	}
	if same.ImportGdtfFixtures.UnchangedCount != 1 {
		t.Errorf("re-import = %+v", same.ImportGdtfFixtures)
	}
}

func TestImportGdtfFixtures_GDTFDefinitionIsFilterable(t *testing.T) {
	c, _, cleanup := testSetup(t)
	defer cleanup()
	var resp gdtfImportResp
	if err := c.Post(importGdtfMutation, &resp, client.Var("files", gdtfFiles("spot.gdtf", gdtfTestdata(t, "mover16.xml")))); err != nil {
		t.Fatal(err)
	}
	var list struct {
		FixtureDefinitions []struct {
			Model  string `json:"model"`
			Source string `json:"source"`
		} `json:"fixtureDefinitions"`
	}
	if err := c.Post(`{ fixtureDefinitions(filter: { source: GDTF }) { model source } }`, &list); err != nil {
		t.Fatal(err)
	}
	if len(list.FixtureDefinitions) != 1 || list.FixtureDefinitions[0].Model != "TestSpot" {
		t.Errorf("GDTF definitions = %+v", list.FixtureDefinitions)
	}
}

func TestImportGdtfFixtures_RequiresAuthentication(t *testing.T) {
	r, cleanup := setupGroupAdminTestResolver(t, true)
	defer cleanup()
	m := r.Mutation().(*mutationResolver)

	if _, err := m.ImportGdtfFixtures(context.Background(), nil, nil); !errors.Is(err, ErrNotAuthenticated) {
		t.Errorf("anonymous: err = %v, want ErrNotAuthenticated", err)
	}
	if r.GDTFService == nil {
		t.Skip("GDTF service not configured in this resolver")
	}
	res, err := m.ImportGdtfFixtures(createMemberContext("u", "u@test.com", "g"), nil, nil)
	if err != nil {
		t.Fatalf("authenticated: %v", err)
	}
	if len(res.Results) != 0 {
		t.Errorf("results = %v, want none", res.Results)
	}
}
