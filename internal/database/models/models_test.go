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
		{"Look", Look{}, "looks"},
		{"FixtureValue", FixtureValue{}, "fixture_values"},
		{"CueList", CueList{}, "cue_lists"},
		{"Cue", Cue{}, "cues"},
		{"PreviewSession", PreviewSession{}, "preview_sessions"},
		{"Setting", Setting{}, "settings"},
		{"LookBoard", LookBoard{}, "look_boards"},
		{"LookBoardButton", LookBoardButton{}, "look_board_buttons"},
		{"Effect", Effect{}, "effects"},
		{"EffectFixture", EffectFixture{}, "effect_fixtures"},
		{"EffectChannel", EffectChannel{}, "effect_channels"},
		{"CueEffect", CueEffect{}, "cue_effects"},
		{"OFLImportMeta", OFLImportMeta{}, "ofl_import_meta"},
		{"Operation", Operation{}, "operations"},
		{"OperationPointer", OperationPointer{}, "operation_pointers"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.model.TableName(); got != tt.tableName {
				t.Errorf("%s.TableName() = %q, want %q", tt.name, got, tt.tableName)
			}
		})
	}
}
