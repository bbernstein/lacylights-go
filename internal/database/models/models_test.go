package models

import (
	"testing"
	"time"
)

func TestGroupRoleConstants(t *testing.T) {
	if GroupRoleMember != "MEMBER" {
		t.Errorf("GroupRoleMember = %q, want %q", GroupRoleMember, "MEMBER")
	}
	if GroupRoleGroupAdmin != "GROUP_ADMIN" {
		t.Errorf("GroupRoleGroupAdmin = %q, want %q", GroupRoleGroupAdmin, "GROUP_ADMIN")
	}
}

func TestInvitationStatusConstants(t *testing.T) {
	if InvitationStatusPending != "PENDING" {
		t.Errorf("InvitationStatusPending = %q, want %q", InvitationStatusPending, "PENDING")
	}
	if InvitationStatusAccepted != "ACCEPTED" {
		t.Errorf("InvitationStatusAccepted = %q, want %q", InvitationStatusAccepted, "ACCEPTED")
	}
	if InvitationStatusDeclined != "DECLINED" {
		t.Errorf("InvitationStatusDeclined = %q, want %q", InvitationStatusDeclined, "DECLINED")
	}
	if InvitationStatusExpired != "EXPIRED" {
		t.Errorf("InvitationStatusExpired = %q, want %q", InvitationStatusExpired, "EXPIRED")
	}
}

func TestUserGroupFields(t *testing.T) {
	ownerID := "user-123"
	group := UserGroup{
		ID:         "group-1",
		Name:       "Test Group",
		IsPersonal: true,
		OwnerID:    &ownerID,
	}
	if !group.IsPersonal {
		t.Error("Expected IsPersonal to be true")
	}
	if group.OwnerID == nil || *group.OwnerID != ownerID {
		t.Errorf("OwnerID = %v, want %q", group.OwnerID, ownerID)
	}
}

func TestUserGroupMemberRole(t *testing.T) {
	member := UserGroupMember{
		ID:      "member-1",
		UserID:  "user-1",
		GroupID: "group-1",
		Role:    GroupRoleGroupAdmin,
	}
	if member.Role != "GROUP_ADMIN" {
		t.Errorf("Role = %q, want %q", member.Role, "GROUP_ADMIN")
	}
}

func TestProjectGroupID(t *testing.T) {
	// Test nullable group_id
	project := Project{
		ID:   "proj-1",
		Name: "Test",
	}
	if project.GroupID != nil {
		t.Error("Expected GroupID to be nil for auth-off project")
	}

	groupID := "group-1"
	project.GroupID = &groupID
	if project.GroupID == nil || *project.GroupID != groupID {
		t.Errorf("GroupID = %v, want %q", project.GroupID, groupID)
	}
}

func TestDeviceGroupMember(t *testing.T) {
	dgm := DeviceGroupMember{
		ID:       "dgm-1",
		DeviceID: "device-1",
		GroupID:  "group-1",
	}
	if dgm.DeviceID != "device-1" {
		t.Errorf("DeviceID = %q, want %q", dgm.DeviceID, "device-1")
	}
	if dgm.GroupID != "group-1" {
		t.Errorf("GroupID = %q, want %q", dgm.GroupID, "group-1")
	}
}

func TestGroupInvitation(t *testing.T) {
	now := time.Now()
	inv := GroupInvitation{
		ID:          "inv-1",
		GroupID:     "group-1",
		Email:       "test@example.com",
		InvitedByID: "user-1",
		Role:        GroupRoleMember,
		Status:      InvitationStatusPending,
		ExpiresAt:   now.Add(7 * 24 * time.Hour),
	}
	if inv.Status != "PENDING" {
		t.Errorf("Status = %q, want %q", inv.Status, "PENDING")
	}
	if inv.Role != "MEMBER" {
		t.Errorf("Role = %q, want %q", inv.Role, "MEMBER")
	}
	if inv.AcceptedAt != nil {
		t.Error("Expected AcceptedAt to be nil for pending invitation")
	}
}

func TestTableNames(t *testing.T) {
	tests := []struct {
		name      string
		model     interface{ TableName() string }
		tableName string
	}{
		// Auth models
		{"User", User{}, "users"},
		{"UserCredential", UserCredential{}, "user_credentials"},
		{"UserOAuth", UserOAuth{}, "user_oauth"},
		{"Session", Session{}, "sessions"},
		{"Device", Device{}, "devices"},
		{"VerificationToken", VerificationToken{}, "verification_tokens"},
		{"UserGroup", UserGroup{}, "user_groups"},
		{"UserGroupMember", UserGroupMember{}, "user_group_members"},
		{"DeviceGroupMember", DeviceGroupMember{}, "device_group_members"},
		{"GroupInvitation", GroupInvitation{}, "group_invitations"},
		{"AuthSetting", AuthSetting{}, "auth_settings"},
		// Lighting models
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
		{"FixtureGroup", FixtureGroup{}, "fixture_groups"},
		{"FixtureGroupMember", FixtureGroupMember{}, "fixture_group_members"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.model.TableName(); got != tt.tableName {
				t.Errorf("%s.TableName() = %q, want %q", tt.name, got, tt.tableName)
			}
		})
	}
}
