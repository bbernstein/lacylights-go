package models

import "testing"

func TestTableNames(t *testing.T) {
	tests := []struct {
		name      string
		model     interface{ TableName() string }
		tableName string
	}{
		{"User", User{}, "users"},
		{"Project", Project{}, "projects"},
		{"ProjectUser", ProjectUser{}, "project_users"},
		{"FixtureDefinition", FixtureDefinition{}, "fixture_definitions"},
		{"ChannelDefinition", ChannelDefinition{}, "channel_definitions"},
		{"FixtureMode", FixtureMode{}, "fixture_modes"},
		{"ModeChannel", ModeChannel{}, "mode_channels"},
		{"FixtureInstance", FixtureInstance{}, "fixture_instances"},
		{"InstanceChannel", InstanceChannel{}, "instance_channels"},
		{"Scene", Scene{}, "scenes"},
		{"FixtureValue", FixtureValue{}, "fixture_values"},
		{"CueList", CueList{}, "cue_lists"},
		{"Cue", Cue{}, "cues"},
		{"PreviewSession", PreviewSession{}, "preview_sessions"},
		{"Setting", Setting{}, "settings"},
		{"SceneBoard", SceneBoard{}, "scene_boards"},
		{"SceneBoardButton", SceneBoardButton{}, "scene_board_buttons"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.model.TableName(); got != tt.tableName {
				t.Errorf("%s.TableName() = %q, want %q", tt.name, got, tt.tableName)
			}
		})
	}
}
