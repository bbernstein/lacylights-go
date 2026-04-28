package importeos

import (
	"reflect"
	"testing"
)

func TestTracker_PropagatesAndOverrides(t *testing.T) {
	show := parseFixture(t, "tracking.asc")
	tr := NewTracker()
	snapshots := tr.ResolveCueList(show.CueLists[0].Cues, show.ColorPalettes,
		show.BeamPalettes, show.FocusPalettes, show.IntensPalettes, show.Presets)

	if len(snapshots) != 3 {
		t.Fatalf("snapshots: got %d", len(snapshots))
	}
	want0 := map[int]int{1: 0xFF, 2: 0xFF}
	if !reflect.DeepEqual(snapshots[0].ChannelLevels, want0) {
		t.Errorf("cue 1 levels: got %v, want %v", snapshots[0].ChannelLevels, want0)
	}
	want1 := map[int]int{1: 0xFF, 2: 0xFF, 3: 0xFF}
	if !reflect.DeepEqual(snapshots[1].ChannelLevels, want1) {
		t.Errorf("cue 2 (track): got %v, want %v", snapshots[1].ChannelLevels, want1)
	}
	want2 := map[int]int{1: 0x00, 2: 0xFF, 3: 0xFF}
	if !reflect.DeepEqual(snapshots[2].ChannelLevels, want2) {
		t.Errorf("cue 3 (override): got %v, want %v", snapshots[2].ChannelLevels, want2)
	}
}
