package importqlc

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func readTestdata(t *testing.T, name string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("testdata", name))
	require.NoError(t, err)
	return string(b)
}

func TestParse_Issue91Workspace(t *testing.T) {
	ws, err := Parse(readTestdata(t, "issue91_workspace.qxw"))
	require.NoError(t, err)

	assert.Equal(t, "4.13.1", ws.CreatorVersion)

	require.Len(t, ws.Fixtures, 4)
	assert.Equal(t, Fixture{
		ID: "0", Manufacturer: "Chauvet", Model: "SlimPAR Pro Q USB", Mode: "4 Channel",
		Name: "par-4chan", Universe: 0, Address: 0, Channels: 4,
	}, ws.Fixtures[0])
	assert.Equal(t, "4", ws.Fixtures[3].ID)
	assert.Equal(t, 1, ws.Fixtures[3].Universe)

	require.Len(t, ws.Functions, 3)
	scene := ws.Functions[0]
	assert.Equal(t, "0", scene.ID)
	assert.Equal(t, TypeScene, scene.Type)
	assert.Equal(t, "first scene", scene.Name)
	require.Len(t, scene.Values, 4)
	assert.Equal(t, FixtureValues{FixtureID: "2", Channels: []ChannelValue{
		{Channel: 0, Value: 232}, {Channel: 1, Value: 237}, {Channel: 2, Value: 229},
	}}, scene.Values[0])

	chaser := ws.Functions[2]
	assert.Equal(t, TypeChaser, chaser.Type)
	assert.Equal(t, "Loop", chaser.RunOrder)
	assert.Equal(t, "Forward", chaser.Direction)
	assert.Equal(t, SpeedModes{FadeIn: SpeedModePerStep, FadeOut: SpeedModePerStep, Duration: SpeedModePerStep}, chaser.SpeedModes)
	require.Len(t, chaser.Steps, 4)
	assert.Equal(t, Step{Number: 0, FunctionID: "0", FadeIn: 1000, Hold: 1000, FadeOut: 500}, chaser.Steps[0])
	assert.Equal(t, InfiniteSpeed, chaser.Steps[2].Hold)
	assert.Equal(t, "Final blackout", chaser.Steps[3].Note)
}

func TestParse_FeatureCoverage(t *testing.T) {
	ws, err := Parse(readTestdata(t, "feature_coverage.qxw"))
	require.NoError(t, err)

	// Only direct <Engine><Fixture> children are fixtures; the <Fixture>
	// inside the EFX function must not be picked up.
	require.Len(t, ws.Fixtures, 3)
	assert.Equal(t, []int{3}, ws.Fixtures[0].ExcludeFade)
	assert.Equal(t, "RGBPanel", ws.Fixtures[1].Model)

	byName := map[string]Function{}
	for _, f := range ws.Functions {
		byName[f.Name] = f
	}

	warm := byName["Warm"]
	require.Len(t, warm.Values, 4)
	assert.Equal(t, "12", warm.Values[2].FixtureID)
	assert.Empty(t, warm.Values[2].Channels)

	legacy := byName["Legacy Values"]
	assert.Equal(t, Speed{FadeIn: 2000}, legacy.Speed)
	assert.Equal(t, []FixtureValues{
		{FixtureID: "10", Channels: []ChannelValue{{Channel: 2, Value: 64}}},
		{FixtureID: "12", Channels: []ChannelValue{{Channel: 1, Value: 200}}},
	}, legacy.Values)

	seq := byName["Dimmer Chase"]
	assert.Equal(t, TypeSequence, seq.Type)
	assert.Equal(t, "2", seq.BoundSceneID)
	require.Len(t, seq.Steps, 2)
	assert.True(t, seq.Steps[0].HasValues)
	assert.Equal(t, []FixtureValues{{FixtureID: "10", Channels: []ChannelValue{{Channel: 0, Value: 255}}}}, seq.Steps[0].Values)
	assert.Equal(t, uint32(500), seq.Steps[0].Hold)

	common := byName["Common Speeds"]
	assert.Equal(t, Speed{FadeIn: 3000, FadeOut: 1500, Duration: 5000}, common.Speed)
	assert.Equal(t, SpeedModes{FadeIn: SpeedModeCommon, FadeOut: SpeedModeCommon, Duration: SpeedModeCommon}, common.SpeedModes)
	assert.Equal(t, "Backward", common.Direction)

	// Legacy (QLC+ 4.0) steps carry Duration instead of Hold:
	// hold = duration - fadeIn, mirroring ChaserStep::loadXML.
	legacyDur := byName["Legacy Durations"]
	require.Len(t, legacyDur.Steps, 2)
	assert.Equal(t, uint32(4000), legacyDur.Steps[0].Hold)
	assert.Equal(t, uint32(0), legacyDur.Steps[1].Hold)
	assert.Equal(t, SpeedModeDefault, legacyDur.SpeedModes.FadeIn)
}

func TestParse_LegacyDurationSubtractsFadeIn(t *testing.T) {
	xml := `<Workspace><Engine>
  <Function ID="1" Type="Chaser" Name="c">
   <Step Number="0" FadeIn="1000" Duration="3000">0</Step>
   <Step Number="1" FadeIn="5000" Duration="3000">0</Step>
  </Function></Engine></Workspace>`
	ws, err := Parse(xml)
	require.NoError(t, err)
	require.Len(t, ws.Functions[0].Steps, 2)
	assert.Equal(t, uint32(2000), ws.Functions[0].Steps[0].Hold)
	assert.Equal(t, uint32(0), ws.Functions[0].Steps[1].Hold, "hold clamps at zero")
}

func TestParse_StepsSortedByNumber(t *testing.T) {
	xml := `<Workspace><Engine>
  <Function ID="1" Type="Chaser" Name="c">
   <Step Number="2">7</Step>
   <Step Number="0">5</Step>
   <Step Number="1">6</Step>
  </Function></Engine></Workspace>`
	ws, err := Parse(xml)
	require.NoError(t, err)
	var ids []string
	for _, s := range ws.Functions[0].Steps {
		ids = append(ids, s.FunctionID)
	}
	assert.Equal(t, []string{"5", "6", "7"}, ids)
}

func TestParse_AcceptsBOMAndNonUTF8Declaration(t *testing.T) {
	// The frontend reads the file as text, so the payload is always a
	// UTF-8 string even when the XML declaration names another charset.
	xml := "\ufeff<?xml version=\"1.0\" encoding=\"ISO-8859-1\"?>\n<Workspace><Engine>" +
		"<Fixture><Manufacturer>Générique</Manufacturer><Model>Generic</Model><Mode>Generic</Mode>" +
		"<ID>0</ID><Name>Dimmer</Name><Universe>0</Universe><Address>0</Address><Channels>1</Channels></Fixture>" +
		"</Engine></Workspace>"
	ws, err := Parse(xml)
	require.NoError(t, err)
	require.Len(t, ws.Fixtures, 1)
	assert.Equal(t, "Générique", ws.Fixtures[0].Manufacturer)
}

func TestParse_Errors(t *testing.T) {
	tests := []struct {
		name    string
		content string
		wantErr string
	}{
		{"empty", "", "empty"},
		{"not xml", "this is not xml", "invalid QLC+ workspace"},
		{"wrong root", `<?xml version="1.0"?><FixtureDefinition/>`, "expected <Workspace>"},
		{"truncated", `<Workspace><Engine><Fixture>`, "invalid QLC+ workspace"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := Parse(tt.content)
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.wantErr)
		})
	}
}

func TestParse_IgnoresMalformedNumbersLikeQLC(t *testing.T) {
	// QString::toUInt returns 0 on malformed input; follow suit rather
	// than failing the whole import.
	xml := `<Workspace><Engine>
  <Fixture><Manufacturer>Generic</Manufacturer><Model>Generic</Model><Mode>Generic</Mode>
   <ID> 3 </ID><Name>d</Name><Universe>x</Universe><Address>-5</Address><Channels>2</Channels></Fixture>
  <Function ID="0" Type="Scene" Name="s"><FixtureVal ID="3">0,abc,1</FixtureVal></Function>
 </Engine></Workspace>`
	ws, err := Parse(xml)
	require.NoError(t, err)
	require.Len(t, ws.Fixtures, 1)
	assert.Equal(t, "3", ws.Fixtures[0].ID)
	assert.Equal(t, 0, ws.Fixtures[0].Universe)
	assert.Equal(t, 0, ws.Fixtures[0].Address)
	// "0,abc" -> channel 0 value 0; the dangling "1" has no value and is dropped.
	assert.Equal(t, []ChannelValue{{Channel: 0, Value: 0}}, ws.Functions[0].Values[0].Channels)
}

func TestParse_LargeWorkspaceIsLinear(t *testing.T) {
	var b strings.Builder
	b.WriteString("<Workspace><Engine>")
	for i := 0; i < 2000; i++ {
		b.WriteString(`<Function ID="1" Type="Scene" Name="s"><FixtureVal ID="0">0,1,1,2,2,3</FixtureVal></Function>`)
	}
	b.WriteString("</Engine></Workspace>")
	ws, err := Parse(b.String())
	require.NoError(t, err)
	assert.Len(t, ws.Functions, 2000)
}
