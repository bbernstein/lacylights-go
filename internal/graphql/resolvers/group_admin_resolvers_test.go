package resolvers

import (
	"context"
	"testing"
	"time"

	"github.com/lucsky/cuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/bbernstein/lacylights-go/internal/auth"
	"github.com/bbernstein/lacylights-go/internal/auth/session"
	"github.com/bbernstein/lacylights-go/internal/database/models"
	"github.com/bbernstein/lacylights-go/internal/database/repositories"
	"github.com/bbernstein/lacylights-go/internal/graphql/generated"
	"github.com/bbernstein/lacylights-go/internal/middleware"
)

// setupGroupAdminTestResolver creates a resolver for group admin tests.
func setupGroupAdminTestResolver(t *testing.T, authEnabled bool) (*Resolver, func()) {
	t.Helper()
	client, resolver, cleanup := testSetup(t)
	_ = client

	if authEnabled {
		authService, err := auth.NewService(auth.Config{
			DB:                   resolver.db,
			JWTSecret:            "test-secret-key-at-least-32-bytes",
			JWTIssuer:            "test",
			JWTAccessTokenTTL:    15 * time.Minute,
			JWTRefreshTokenTTL:   7 * 24 * time.Hour,
			SessionDurationHours: 168,
			CacheMaxSize:         1000,
			CacheTTL:             5 * time.Minute,
			PasswordMinLength:    8,
			Enabled:              true,
			DeviceAuthEnabled:    true,
		})
		require.NoError(t, err)
		resolver.SetAuthService(authService)
	}

	return resolver, cleanup
}

// createGroupAdminContext creates a context with a GROUP_ADMIN role for a specific group.
func createGroupAdminContext(userID, email, groupID string) context.Context {
	sess := &session.CachedSession{
		UserID:    userID,
		Email:     email,
		Role:      "USER",
		SessionID: "test-session-id",
	}
	ctx := context.Background()
	ctx = context.WithValue(ctx, middleware.ContextKeySession, sess)
	ctx = context.WithValue(ctx, middleware.ContextKeyUserID, userID)
	ctx = context.WithValue(ctx, middleware.ContextKeyUserEmail, email)
	ctx = context.WithValue(ctx, middleware.ContextKeyUserRole, "USER")
	ctx = context.WithValue(ctx, middleware.ContextKeyUserGroups, []middleware.UserGroupMembership{
		{GroupID: groupID, Role: "GROUP_ADMIN"},
	})
	return ctx
}

// createMemberContext creates a context with a MEMBER role for a specific group.
func createMemberContext(userID, email, groupID string) context.Context {
	sess := &session.CachedSession{
		UserID:    userID,
		Email:     email,
		Role:      "USER",
		SessionID: "test-session-id",
	}
	ctx := context.Background()
	ctx = context.WithValue(ctx, middleware.ContextKeySession, sess)
	ctx = context.WithValue(ctx, middleware.ContextKeyUserID, userID)
	ctx = context.WithValue(ctx, middleware.ContextKeyUserEmail, email)
	ctx = context.WithValue(ctx, middleware.ContextKeyUserRole, "USER")
	ctx = context.WithValue(ctx, middleware.ContextKeyUserGroups, []middleware.UserGroupMembership{
		{GroupID: groupID, Role: "MEMBER"},
	})
	return ctx
}

// helper to create a test group and return it.
func createTestGroup(t *testing.T, resolver *Resolver, name string) *models.UserGroup {
	t.Helper()
	group := &models.UserGroup{
		ID:   cuid.New(),
		Name: name,
	}
	require.NoError(t, resolver.db.Create(group).Error)
	return group
}

// helper to create a test user and return it.
func createTestUser(t *testing.T, resolver *Resolver, email string) *models.User {
	t.Helper()
	user := &models.User{
		ID:       cuid.New(),
		Email:    email,
		Role:     "USER",
		IsActive: true,
	}
	require.NoError(t, resolver.db.Create(user).Error)
	return user
}

// helper to add a member to a group.
func addTestGroupMember(t *testing.T, resolver *Resolver, userID, groupID, role string) *models.UserGroupMember {
	t.Helper()
	member := &models.UserGroupMember{
		ID:      cuid.New(),
		UserID:  userID,
		GroupID: groupID,
		Role:    role,
	}
	require.NoError(t, resolver.db.Create(member).Error)
	return member
}

// ============================================================
// UpdateGroupMemberRole tests
// ============================================================

func TestUpdateGroupMemberRole_Success(t *testing.T) {
	resolver, cleanup := setupGroupAdminTestResolver(t, true)
	defer cleanup()

	group := createTestGroup(t, resolver, "Test Group")
	user := createTestUser(t, resolver, "member@test.com")
	addTestGroupMember(t, resolver, user.ID, group.ID, models.GroupRoleMember)

	ctx := createGroupAdminContext("admin-user", "admin@test.com", group.ID)
	result, err := resolver.updateGroupMemberRole(ctx, group.ID, user.ID, generated.GroupMemberRoleGroupAdmin)
	require.NoError(t, err)
	assert.True(t, result)

	// Verify in database
	member, err := resolver.GroupRepo.FindMember(context.Background(), user.ID, group.ID)
	require.NoError(t, err)
	assert.Equal(t, models.GroupRoleGroupAdmin, member.Role)
}

func TestUpdateGroupMemberRole_GroupNotFound(t *testing.T) {
	resolver, cleanup := setupGroupAdminTestResolver(t, true)
	defer cleanup()

	ctx := createAdminContext()
	_, err := resolver.updateGroupMemberRole(ctx, "non-existent-group", "user-id", generated.GroupMemberRoleMember)
	require.Error(t, err)
	assert.Equal(t, ErrGroupNotFound, err)
}

func TestUpdateGroupMemberRole_MemberNotFound(t *testing.T) {
	resolver, cleanup := setupGroupAdminTestResolver(t, true)
	defer cleanup()

	group := createTestGroup(t, resolver, "Test Group")
	ctx := createAdminContext()
	_, err := resolver.updateGroupMemberRole(ctx, group.ID, "non-existent-user", generated.GroupMemberRoleMember)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not a member")
}

func TestUpdateGroupMemberRole_NotAuthorized(t *testing.T) {
	resolver, cleanup := setupGroupAdminTestResolver(t, true)
	defer cleanup()

	group := createTestGroup(t, resolver, "Test Group")
	user := createTestUser(t, resolver, "member@test.com")
	addTestGroupMember(t, resolver, user.ID, group.ID, models.GroupRoleMember)

	// Regular member should not be able to update roles
	ctx := createMemberContext("other-user", "other@test.com", group.ID)
	_, err := resolver.updateGroupMemberRole(ctx, group.ID, user.ID, generated.GroupMemberRoleGroupAdmin)
	require.Error(t, err)
	assert.Equal(t, ErrNotAuthorized, err)
}

func TestUpdateGroupMemberRole_CannotDemoteOwner(t *testing.T) {
	resolver, cleanup := setupGroupAdminTestResolver(t, true)
	defer cleanup()

	user := createTestUser(t, resolver, "owner@test.com")
	group := &models.UserGroup{
		ID:      cuid.New(),
		Name:    "Owner's Group",
		OwnerID: &user.ID,
	}
	require.NoError(t, resolver.db.Create(group).Error)
	addTestGroupMember(t, resolver, user.ID, group.ID, models.GroupRoleGroupAdmin)

	ctx := createAdminContext()
	_, err := resolver.updateGroupMemberRole(ctx, group.ID, user.ID, generated.GroupMemberRoleMember)
	require.Error(t, err)
	assert.Equal(t, ErrCannotDemoteOwner, err)
}

func TestUpdateGroupMemberRole_AuthDisabled(t *testing.T) {
	resolver, cleanup := setupGroupAdminTestResolver(t, false)
	defer cleanup()

	group := createTestGroup(t, resolver, "Test Group")
	user := createTestUser(t, resolver, "member@test.com")
	addTestGroupMember(t, resolver, user.ID, group.ID, models.GroupRoleMember)

	ctx := context.Background()
	result, err := resolver.updateGroupMemberRole(ctx, group.ID, user.ID, generated.GroupMemberRoleGroupAdmin)
	require.NoError(t, err)
	assert.True(t, result)
}

// ============================================================
// InviteToGroup tests
// ============================================================

func TestInviteToGroup_Success(t *testing.T) {
	resolver, cleanup := setupGroupAdminTestResolver(t, true)
	defer cleanup()

	group := createTestGroup(t, resolver, "Test Group")
	ctx := createGroupAdminContext("admin-user", "admin@test.com", group.ID)

	invitation, err := resolver.inviteToGroup(ctx, group.ID, "invitee@test.com", nil)
	require.NoError(t, err)
	assert.NotEmpty(t, invitation.ID)
	assert.Equal(t, group.ID, invitation.GroupID)
	assert.Equal(t, "invitee@test.com", invitation.Email)
	assert.Equal(t, models.GroupRoleMember, invitation.Role)
	assert.Equal(t, models.InvitationStatusPending, invitation.Status)
	assert.True(t, invitation.ExpiresAt.After(time.Now()))
}

func TestInviteToGroup_WithAdminRole(t *testing.T) {
	resolver, cleanup := setupGroupAdminTestResolver(t, true)
	defer cleanup()

	group := createTestGroup(t, resolver, "Test Group")
	ctx := createGroupAdminContext("admin-user", "admin@test.com", group.ID)

	adminRole := generated.GroupMemberRoleGroupAdmin
	invitation, err := resolver.inviteToGroup(ctx, group.ID, "invitee@test.com", &adminRole)
	require.NoError(t, err)
	assert.Equal(t, models.GroupRoleGroupAdmin, invitation.Role)
}

func TestInviteToGroup_AlreadyMember(t *testing.T) {
	resolver, cleanup := setupGroupAdminTestResolver(t, true)
	defer cleanup()

	group := createTestGroup(t, resolver, "Test Group")
	user := createTestUser(t, resolver, "existing@test.com")
	addTestGroupMember(t, resolver, user.ID, group.ID, models.GroupRoleMember)

	ctx := createGroupAdminContext("admin-user", "admin@test.com", group.ID)
	_, err := resolver.inviteToGroup(ctx, group.ID, "existing@test.com", nil)
	require.Error(t, err)
	assert.Equal(t, ErrAlreadyMember, err)
}

func TestInviteToGroup_DuplicateInvitation(t *testing.T) {
	resolver, cleanup := setupGroupAdminTestResolver(t, true)
	defer cleanup()

	group := createTestGroup(t, resolver, "Test Group")
	ctx := createGroupAdminContext("admin-user", "admin@test.com", group.ID)

	// First invitation
	inv1, err := resolver.inviteToGroup(ctx, group.ID, "invitee@test.com", nil)
	require.NoError(t, err)

	// Second invitation should return the existing one
	inv2, err := resolver.inviteToGroup(ctx, group.ID, "invitee@test.com", nil)
	require.NoError(t, err)
	assert.Equal(t, inv1.ID, inv2.ID)
}

func TestInviteToGroup_GroupNotFound(t *testing.T) {
	resolver, cleanup := setupGroupAdminTestResolver(t, true)
	defer cleanup()

	ctx := createAdminContext()
	_, err := resolver.inviteToGroup(ctx, "non-existent-group", "invitee@test.com", nil)
	require.Error(t, err)
	assert.Equal(t, ErrGroupNotFound, err)
}

func TestInviteToGroup_NotAuthorized(t *testing.T) {
	resolver, cleanup := setupGroupAdminTestResolver(t, true)
	defer cleanup()

	group := createTestGroup(t, resolver, "Test Group")
	ctx := createMemberContext("member-user", "member@test.com", group.ID)

	_, err := resolver.inviteToGroup(ctx, group.ID, "invitee@test.com", nil)
	require.Error(t, err)
	assert.Equal(t, ErrNotAuthorized, err)
}

func TestInviteToGroup_AuthDisabled(t *testing.T) {
	resolver, cleanup := setupGroupAdminTestResolver(t, false)
	defer cleanup()

	group := createTestGroup(t, resolver, "Test Group")
	ctx := context.Background()

	invitation, err := resolver.inviteToGroup(ctx, group.ID, "invitee@test.com", nil)
	require.NoError(t, err)
	assert.NotEmpty(t, invitation.ID)
	assert.Equal(t, "system", invitation.InvitedByID)
}

// ============================================================
// AcceptInvitation tests
// ============================================================

func TestAcceptInvitation_Success(t *testing.T) {
	resolver, cleanup := setupGroupAdminTestResolver(t, true)
	defer cleanup()

	group := createTestGroup(t, resolver, "Test Group")
	user := createTestUser(t, resolver, "invitee@test.com")

	// Create an invitation
	invitation, err := resolver.InvitationRepo.CreateWithDefaults(context.Background(), invitationInput(group.ID, user.Email, "inviter-id", models.GroupRoleMember))
	require.NoError(t, err)

	// Accept it with the invitee's context
	ctx := createUserContext(user.ID, user.Email)
	result, err := resolver.acceptInvitation(ctx, invitation.ID)
	require.NoError(t, err)
	assert.True(t, result)

	// Verify user is now a group member
	member, err := resolver.GroupRepo.FindMember(context.Background(), user.ID, group.ID)
	require.NoError(t, err)
	assert.NotNil(t, member)
	assert.Equal(t, models.GroupRoleMember, member.Role)

	// Verify invitation status is accepted
	updatedInv, err := resolver.InvitationRepo.FindByID(context.Background(), invitation.ID)
	require.NoError(t, err)
	assert.Equal(t, models.InvitationStatusAccepted, updatedInv.Status)
	assert.NotNil(t, updatedInv.AcceptedAt)
}

func TestAcceptInvitation_WithAdminRole(t *testing.T) {
	resolver, cleanup := setupGroupAdminTestResolver(t, true)
	defer cleanup()

	group := createTestGroup(t, resolver, "Test Group")
	user := createTestUser(t, resolver, "invitee@test.com")

	// Create an invitation with GROUP_ADMIN role
	invitation, err := resolver.InvitationRepo.CreateWithDefaults(context.Background(), invitationInput(group.ID, user.Email, "inviter-id", models.GroupRoleGroupAdmin))
	require.NoError(t, err)

	ctx := createUserContext(user.ID, user.Email)
	result, err := resolver.acceptInvitation(ctx, invitation.ID)
	require.NoError(t, err)
	assert.True(t, result)

	// Verify user is a group admin
	member, err := resolver.GroupRepo.FindMember(context.Background(), user.ID, group.ID)
	require.NoError(t, err)
	assert.Equal(t, models.GroupRoleGroupAdmin, member.Role)
}

func TestAcceptInvitation_NotFound(t *testing.T) {
	resolver, cleanup := setupGroupAdminTestResolver(t, true)
	defer cleanup()

	ctx := createUserContext("user-id", "user@test.com")
	_, err := resolver.acceptInvitation(ctx, "non-existent-invitation")
	require.Error(t, err)
	assert.Equal(t, ErrInvitationNotFound, err)
}

func TestAcceptInvitation_AlreadyAccepted(t *testing.T) {
	resolver, cleanup := setupGroupAdminTestResolver(t, true)
	defer cleanup()

	group := createTestGroup(t, resolver, "Test Group")
	user := createTestUser(t, resolver, "invitee@test.com")

	invitation, err := resolver.InvitationRepo.CreateWithDefaults(context.Background(), invitationInput(group.ID, user.Email, "inviter-id", models.GroupRoleMember))
	require.NoError(t, err)

	// Accept it once
	ctx := createUserContext(user.ID, user.Email)
	_, err = resolver.acceptInvitation(ctx, invitation.ID)
	require.NoError(t, err)

	// Trying to accept again should fail (not pending anymore)
	_, err = resolver.acceptInvitation(ctx, invitation.ID)
	require.Error(t, err)
	assert.Equal(t, ErrInvitationNotPending, err)
}

func TestAcceptInvitation_Expired(t *testing.T) {
	resolver, cleanup := setupGroupAdminTestResolver(t, true)
	defer cleanup()

	group := createTestGroup(t, resolver, "Test Group")
	user := createTestUser(t, resolver, "invitee@test.com")

	// Create an already-expired invitation
	invitation := &models.GroupInvitation{
		ID:          cuid.New(),
		GroupID:     group.ID,
		Email:       user.Email,
		InvitedByID: "inviter-id",
		Role:        models.GroupRoleMember,
		Status:      models.InvitationStatusPending,
		ExpiresAt:   time.Now().Add(-1 * time.Hour), // Expired
	}
	require.NoError(t, resolver.db.Create(invitation).Error)

	ctx := createUserContext(user.ID, user.Email)
	_, err := resolver.acceptInvitation(ctx, invitation.ID)
	require.Error(t, err)
	assert.Equal(t, ErrInvitationExpired, err)
}

func TestAcceptInvitation_EmailMismatch(t *testing.T) {
	resolver, cleanup := setupGroupAdminTestResolver(t, true)
	defer cleanup()

	group := createTestGroup(t, resolver, "Test Group")

	invitation, err := resolver.InvitationRepo.CreateWithDefaults(context.Background(), invitationInput(group.ID, "correct@test.com", "inviter-id", models.GroupRoleMember))
	require.NoError(t, err)

	// Try to accept with a different email
	ctx := createUserContext("wrong-user", "wrong@test.com")
	_, err = resolver.acceptInvitation(ctx, invitation.ID)
	require.Error(t, err)
	assert.Equal(t, ErrEmailMismatch, err)
}

func TestAcceptInvitation_AuthDisabled(t *testing.T) {
	resolver, cleanup := setupGroupAdminTestResolver(t, false)
	defer cleanup()

	group := createTestGroup(t, resolver, "Test Group")
	inviter := createTestUser(t, resolver, "inviter@test.com")
	user := createTestUser(t, resolver, "invitee@test.com")

	invitation, err := resolver.InvitationRepo.CreateWithDefaults(context.Background(), invitationInput(group.ID, user.Email, inviter.ID, models.GroupRoleMember))
	require.NoError(t, err)

	// Auth disabled: no email check
	ctx := context.Background()
	result, err := resolver.acceptInvitation(ctx, invitation.ID)
	require.NoError(t, err)
	assert.True(t, result)

	// Verify user is a member (lookup by email)
	member, err := resolver.GroupRepo.FindMember(context.Background(), user.ID, group.ID)
	require.NoError(t, err)
	assert.NotNil(t, member)
}

// ============================================================
// DeclineInvitation tests
// ============================================================

func TestDeclineInvitation_Success(t *testing.T) {
	resolver, cleanup := setupGroupAdminTestResolver(t, true)
	defer cleanup()

	group := createTestGroup(t, resolver, "Test Group")
	invitation, err := resolver.InvitationRepo.CreateWithDefaults(context.Background(), invitationInput(group.ID, "invitee@test.com", "inviter-id", models.GroupRoleMember))
	require.NoError(t, err)

	ctx := createUserContext("user-id", "invitee@test.com")
	result, err := resolver.declineInvitation(ctx, invitation.ID)
	require.NoError(t, err)
	assert.True(t, result)

	// Verify status
	updatedInv, err := resolver.InvitationRepo.FindByID(context.Background(), invitation.ID)
	require.NoError(t, err)
	assert.Equal(t, models.InvitationStatusDeclined, updatedInv.Status)
}

func TestDeclineInvitation_NotFound(t *testing.T) {
	resolver, cleanup := setupGroupAdminTestResolver(t, true)
	defer cleanup()

	ctx := createUserContext("user-id", "user@test.com")
	_, err := resolver.declineInvitation(ctx, "non-existent")
	require.Error(t, err)
	assert.Equal(t, ErrInvitationNotFound, err)
}

func TestDeclineInvitation_NotPending(t *testing.T) {
	resolver, cleanup := setupGroupAdminTestResolver(t, true)
	defer cleanup()

	group := createTestGroup(t, resolver, "Test Group")
	invitation, err := resolver.InvitationRepo.CreateWithDefaults(context.Background(), invitationInput(group.ID, "invitee@test.com", "inviter-id", models.GroupRoleMember))
	require.NoError(t, err)

	// First decline
	ctx := createUserContext("user-id", "invitee@test.com")
	_, err = resolver.declineInvitation(ctx, invitation.ID)
	require.NoError(t, err)

	// Second decline fails (not pending)
	_, err = resolver.declineInvitation(ctx, invitation.ID)
	require.Error(t, err)
	assert.Equal(t, ErrInvitationNotPending, err)
}

func TestDeclineInvitation_EmailMismatch(t *testing.T) {
	resolver, cleanup := setupGroupAdminTestResolver(t, true)
	defer cleanup()

	group := createTestGroup(t, resolver, "Test Group")
	invitation, err := resolver.InvitationRepo.CreateWithDefaults(context.Background(), invitationInput(group.ID, "correct@test.com", "inviter-id", models.GroupRoleMember))
	require.NoError(t, err)

	ctx := createUserContext("wrong-user", "wrong@test.com")
	_, err = resolver.declineInvitation(ctx, invitation.ID)
	require.Error(t, err)
	assert.Equal(t, ErrEmailMismatch, err)
}

// ============================================================
// CancelInvitation tests
// ============================================================

func TestCancelInvitation_Success(t *testing.T) {
	resolver, cleanup := setupGroupAdminTestResolver(t, true)
	defer cleanup()

	group := createTestGroup(t, resolver, "Test Group")
	invitation, err := resolver.InvitationRepo.CreateWithDefaults(context.Background(), invitationInput(group.ID, "invitee@test.com", "inviter-id", models.GroupRoleMember))
	require.NoError(t, err)

	ctx := createGroupAdminContext("admin-user", "admin@test.com", group.ID)
	result, err := resolver.cancelInvitation(ctx, invitation.ID)
	require.NoError(t, err)
	assert.True(t, result)

	// Verify status
	updatedInv, err := resolver.InvitationRepo.FindByID(context.Background(), invitation.ID)
	require.NoError(t, err)
	assert.Equal(t, models.InvitationStatusDeclined, updatedInv.Status)
}

func TestCancelInvitation_NotFound(t *testing.T) {
	resolver, cleanup := setupGroupAdminTestResolver(t, true)
	defer cleanup()

	ctx := createAdminContext()
	_, err := resolver.cancelInvitation(ctx, "non-existent")
	require.Error(t, err)
	assert.Equal(t, ErrInvitationNotFound, err)
}

func TestCancelInvitation_NotAuthorized(t *testing.T) {
	resolver, cleanup := setupGroupAdminTestResolver(t, true)
	defer cleanup()

	group := createTestGroup(t, resolver, "Test Group")
	invitation, err := resolver.InvitationRepo.CreateWithDefaults(context.Background(), invitationInput(group.ID, "invitee@test.com", "inviter-id", models.GroupRoleMember))
	require.NoError(t, err)

	// Regular member should not be able to cancel
	ctx := createMemberContext("member-user", "member@test.com", group.ID)
	_, err = resolver.cancelInvitation(ctx, invitation.ID)
	require.Error(t, err)
	assert.Equal(t, ErrNotAuthorized, err)
}

func TestCancelInvitation_SystemAdminCanCancel(t *testing.T) {
	resolver, cleanup := setupGroupAdminTestResolver(t, true)
	defer cleanup()

	group := createTestGroup(t, resolver, "Test Group")
	invitation, err := resolver.InvitationRepo.CreateWithDefaults(context.Background(), invitationInput(group.ID, "invitee@test.com", "inviter-id", models.GroupRoleMember))
	require.NoError(t, err)

	// System admin can cancel any invitation
	ctx := createAdminContext()
	result, err := resolver.cancelInvitation(ctx, invitation.ID)
	require.NoError(t, err)
	assert.True(t, result)
}

// ============================================================
// AddDeviceToGroup tests
// ============================================================

func TestAddDeviceToGroup_Success(t *testing.T) {
	resolver, cleanup := setupGroupAdminTestResolver(t, true)
	defer cleanup()

	group := createTestGroup(t, resolver, "Test Group")
	device := createTestDevice(t, resolver, "Test Device")

	ctx := createGroupAdminContext("admin-user", "admin@test.com", group.ID)
	result, err := resolver.addDeviceToGroup(ctx, device.ID, group.ID)
	require.NoError(t, err)
	assert.True(t, result)

	// Verify in database
	member, err := resolver.GroupRepo.FindDeviceGroupMember(context.Background(), device.ID, group.ID)
	require.NoError(t, err)
	assert.NotNil(t, member)
}

func TestAddDeviceToGroup_GroupNotFound(t *testing.T) {
	resolver, cleanup := setupGroupAdminTestResolver(t, true)
	defer cleanup()

	device := createTestDevice(t, resolver, "Test Device")
	ctx := createAdminContext()
	_, err := resolver.addDeviceToGroup(ctx, device.ID, "non-existent-group")
	require.Error(t, err)
	assert.Equal(t, ErrGroupNotFound, err)
}

func TestAddDeviceToGroup_DeviceNotFound(t *testing.T) {
	resolver, cleanup := setupGroupAdminTestResolver(t, true)
	defer cleanup()

	group := createTestGroup(t, resolver, "Test Group")
	ctx := createGroupAdminContext("admin-user", "admin@test.com", group.ID)
	_, err := resolver.addDeviceToGroup(ctx, "non-existent-device", group.ID)
	require.Error(t, err)
	assert.Equal(t, ErrDeviceNotFound, err)
}

func TestAddDeviceToGroup_AlreadyInGroup(t *testing.T) {
	resolver, cleanup := setupGroupAdminTestResolver(t, true)
	defer cleanup()

	group := createTestGroup(t, resolver, "Test Group")
	device := createTestDevice(t, resolver, "Test Device")

	ctx := createGroupAdminContext("admin-user", "admin@test.com", group.ID)
	_, err := resolver.addDeviceToGroup(ctx, device.ID, group.ID)
	require.NoError(t, err)

	// Try again
	_, err = resolver.addDeviceToGroup(ctx, device.ID, group.ID)
	require.Error(t, err)
	assert.Equal(t, ErrDeviceAlreadyInGroup, err)
}

func TestAddDeviceToGroup_NotAuthorized(t *testing.T) {
	resolver, cleanup := setupGroupAdminTestResolver(t, true)
	defer cleanup()

	group := createTestGroup(t, resolver, "Test Group")
	device := createTestDevice(t, resolver, "Test Device")

	ctx := createMemberContext("member-user", "member@test.com", group.ID)
	_, err := resolver.addDeviceToGroup(ctx, device.ID, group.ID)
	require.Error(t, err)
	assert.Equal(t, ErrNotAuthorized, err)
}

func TestAddDeviceToGroup_AuthDisabled(t *testing.T) {
	resolver, cleanup := setupGroupAdminTestResolver(t, false)
	defer cleanup()

	group := createTestGroup(t, resolver, "Test Group")
	device := createTestDevice(t, resolver, "Test Device")

	ctx := context.Background()
	result, err := resolver.addDeviceToGroup(ctx, device.ID, group.ID)
	require.NoError(t, err)
	assert.True(t, result)
}

// ============================================================
// RemoveDeviceFromGroup tests
// ============================================================

func TestRemoveDeviceFromGroup_Success(t *testing.T) {
	resolver, cleanup := setupGroupAdminTestResolver(t, true)
	defer cleanup()

	group := createTestGroup(t, resolver, "Test Group")
	device := createTestDevice(t, resolver, "Test Device")
	_, err := resolver.GroupRepo.AddDeviceToGroup(context.Background(), device.ID, group.ID)
	require.NoError(t, err)

	ctx := createGroupAdminContext("admin-user", "admin@test.com", group.ID)
	result, err := resolver.removeDeviceFromGroup(ctx, device.ID, group.ID)
	require.NoError(t, err)
	assert.True(t, result)

	// Verify removed
	member, err := resolver.GroupRepo.FindDeviceGroupMember(context.Background(), device.ID, group.ID)
	require.NoError(t, err)
	assert.Nil(t, member)
}

func TestRemoveDeviceFromGroup_NotInGroup(t *testing.T) {
	resolver, cleanup := setupGroupAdminTestResolver(t, true)
	defer cleanup()

	group := createTestGroup(t, resolver, "Test Group")
	ctx := createGroupAdminContext("admin-user", "admin@test.com", group.ID)
	result, err := resolver.removeDeviceFromGroup(ctx, "not-in-group", group.ID)
	require.NoError(t, err)
	assert.False(t, result)
}

func TestRemoveDeviceFromGroup_NotAuthorized(t *testing.T) {
	resolver, cleanup := setupGroupAdminTestResolver(t, true)
	defer cleanup()

	group := createTestGroup(t, resolver, "Test Group")
	ctx := createMemberContext("member-user", "member@test.com", group.ID)
	_, err := resolver.removeDeviceFromGroup(ctx, "device-id", group.ID)
	require.Error(t, err)
	assert.Equal(t, ErrNotAuthorized, err)
}

// ============================================================
// AddUserToGroupWithRole tests
// ============================================================

func TestAddUserToGroupWithRole_DefaultRole(t *testing.T) {
	resolver, cleanup := setupGroupAdminTestResolver(t, true)
	defer cleanup()

	group := createTestGroup(t, resolver, "Test Group")
	user := createTestUser(t, resolver, "new@test.com")

	ctx := createGroupAdminContext("admin-user", "admin@test.com", group.ID)
	result, err := resolver.addUserToGroupWithRole(ctx, user.ID, group.ID, nil)
	require.NoError(t, err)
	assert.True(t, result)

	member, err := resolver.GroupRepo.FindMember(context.Background(), user.ID, group.ID)
	require.NoError(t, err)
	assert.Equal(t, models.GroupRoleMember, member.Role)
}

func TestAddUserToGroupWithRole_AdminRole(t *testing.T) {
	resolver, cleanup := setupGroupAdminTestResolver(t, true)
	defer cleanup()

	group := createTestGroup(t, resolver, "Test Group")
	user := createTestUser(t, resolver, "new@test.com")

	adminRole := generated.GroupMemberRoleGroupAdmin
	ctx := createGroupAdminContext("admin-user", "admin@test.com", group.ID)
	result, err := resolver.addUserToGroupWithRole(ctx, user.ID, group.ID, &adminRole)
	require.NoError(t, err)
	assert.True(t, result)

	member, err := resolver.GroupRepo.FindMember(context.Background(), user.ID, group.ID)
	require.NoError(t, err)
	assert.Equal(t, models.GroupRoleGroupAdmin, member.Role)
}

func TestAddUserToGroupWithRole_AlreadyMember(t *testing.T) {
	resolver, cleanup := setupGroupAdminTestResolver(t, true)
	defer cleanup()

	group := createTestGroup(t, resolver, "Test Group")
	user := createTestUser(t, resolver, "existing@test.com")
	addTestGroupMember(t, resolver, user.ID, group.ID, models.GroupRoleMember)

	ctx := createGroupAdminContext("admin-user", "admin@test.com", group.ID)
	_, err := resolver.addUserToGroupWithRole(ctx, user.ID, group.ID, nil)
	require.Error(t, err)
	assert.Equal(t, ErrAlreadyMember, err)
}

// ============================================================
// ApproveDeviceWithGroup tests
// ============================================================

func TestApproveDeviceWithGroup_WithGroupID(t *testing.T) {
	resolver, cleanup := setupGroupAdminTestResolver(t, true)
	defer cleanup()

	group := createTestGroup(t, resolver, "Test Group")
	device := &models.Device{
		ID:          cuid.New(),
		Name:        "Test Device",
		Fingerprint: "test-fp-" + cuid.New(),
		Status:      models.DeviceStatusPending,
		Permissions: models.DevicePermissionsReadOnly,
		DefaultRole: "PLAYER",
	}
	require.NoError(t, resolver.db.Create(device).Error)

	ctx := createAdminContext()
	groupID := group.ID
	result, err := resolver.approveDeviceWithGroup(ctx, device.ID, generated.DevicePermissionsOperator, &groupID)
	require.NoError(t, err)
	assert.Equal(t, models.DeviceStatusApproved, result.Status)

	// Verify device is in the group
	member, err := resolver.GroupRepo.FindDeviceGroupMember(context.Background(), device.ID, group.ID)
	require.NoError(t, err)
	assert.NotNil(t, member)
}

func TestApproveDeviceWithGroup_NoGroupID(t *testing.T) {
	resolver, cleanup := setupGroupAdminTestResolver(t, true)
	defer cleanup()

	device := &models.Device{
		ID:          cuid.New(),
		Name:        "Test Device",
		Fingerprint: "test-fp-" + cuid.New(),
		Status:      models.DeviceStatusPending,
		Permissions: models.DevicePermissionsReadOnly,
		DefaultRole: "PLAYER",
	}
	require.NoError(t, resolver.db.Create(device).Error)

	ctx := createAdminContext()
	result, err := resolver.approveDeviceWithGroup(ctx, device.ID, generated.DevicePermissionsOperator, nil)
	require.NoError(t, err)
	assert.Equal(t, models.DeviceStatusApproved, result.Status)
}

func TestApproveDeviceWithGroup_GroupNotFound(t *testing.T) {
	resolver, cleanup := setupGroupAdminTestResolver(t, true)
	defer cleanup()

	device := &models.Device{
		ID:          cuid.New(),
		Name:        "Test Device",
		Fingerprint: "test-fp-" + cuid.New(),
		Status:      models.DeviceStatusPending,
		Permissions: models.DevicePermissionsReadOnly,
		DefaultRole: "PLAYER",
	}
	require.NoError(t, resolver.db.Create(device).Error)

	ctx := createAdminContext()
	badGroupID := "non-existent-group"
	result, err := resolver.approveDeviceWithGroup(ctx, device.ID, generated.DevicePermissionsOperator, &badGroupID)
	// Group validation happens before approval - device should NOT be approved
	require.Error(t, err)
	assert.Contains(t, err.Error(), "group not found")
	assert.Nil(t, result, "device should not be returned when group validation fails")

	// Verify device is still pending in the database
	var dbDevice models.Device
	require.NoError(t, resolver.db.First(&dbDevice, "id = ?", device.ID).Error)
	assert.Equal(t, models.DeviceStatusPending, dbDevice.Status)
}

// ============================================================
// Helper functions
// ============================================================

func createTestDevice(t *testing.T, resolver *Resolver, name string) *models.Device {
	t.Helper()
	device := &models.Device{
		ID:          cuid.New(),
		Name:        name,
		Fingerprint: "fp-" + cuid.New(),
		Status:      models.DeviceStatusApproved,
		Permissions: models.DevicePermissionsOperator,
		DefaultRole: "OPERATOR",
	}
	require.NoError(t, resolver.db.Create(device).Error)
	return device
}

func createUserContext(userID, email string) context.Context {
	sess := &session.CachedSession{
		UserID:    userID,
		Email:     email,
		Role:      "USER",
		SessionID: "test-session-id",
	}
	ctx := context.Background()
	ctx = context.WithValue(ctx, middleware.ContextKeySession, sess)
	ctx = context.WithValue(ctx, middleware.ContextKeyUserID, userID)
	ctx = context.WithValue(ctx, middleware.ContextKeyUserEmail, email)
	ctx = context.WithValue(ctx, middleware.ContextKeyUserRole, "USER")
	return ctx
}

func invitationInput(groupID, email, invitedByID, role string) repositories.GroupInvitationInput {
	return repositories.GroupInvitationInput{
		GroupID:     groupID,
		Email:       email,
		InvitedByID: invitedByID,
		Role:        role,
	}
}

// --- ensureProjectAccess tests ---

func TestEnsureProjectAccess_AuthDisabled(t *testing.T) {
	resolver, cleanup := setupGroupAdminTestResolver(t, false)
	defer cleanup()

	// Create a project
	project := &models.Project{ID: cuid.New(), Name: "Test Project"}
	require.NoError(t, resolver.ProjectRepo.Create(context.Background(), project))

	// With auth disabled, any context should work
	err := resolver.ensureProjectAccess(context.Background(), project.ID)
	assert.NoError(t, err)
}

func TestEnsureProjectAccess_AdminHasAccess(t *testing.T) {
	resolver, cleanup := setupGroupAdminTestResolver(t, true)
	defer cleanup()

	groupID := cuid.New()
	project := &models.Project{ID: cuid.New(), Name: "Test Project", GroupID: &groupID}
	require.NoError(t, resolver.ProjectRepo.Create(context.Background(), project))

	ctx := createAdminContext()
	err := resolver.ensureProjectAccess(ctx, project.ID)
	assert.NoError(t, err)
}

func TestEnsureProjectAccess_GroupMemberHasAccess(t *testing.T) {
	resolver, cleanup := setupGroupAdminTestResolver(t, true)
	defer cleanup()

	groupID := cuid.New()
	project := &models.Project{ID: cuid.New(), Name: "Test Project", GroupID: &groupID}
	require.NoError(t, resolver.ProjectRepo.Create(context.Background(), project))

	ctx := createMemberContext("user-1", "user@test.com", groupID)
	err := resolver.ensureProjectAccess(ctx, project.ID)
	assert.NoError(t, err)
}

func TestEnsureProjectAccess_NonMemberDenied(t *testing.T) {
	resolver, cleanup := setupGroupAdminTestResolver(t, true)
	defer cleanup()

	groupID := cuid.New()
	otherGroupID := cuid.New()
	project := &models.Project{ID: cuid.New(), Name: "Test Project", GroupID: &groupID}
	require.NoError(t, resolver.ProjectRepo.Create(context.Background(), project))

	// User is member of a different group
	ctx := createMemberContext("user-1", "user@test.com", otherGroupID)
	err := resolver.ensureProjectAccess(ctx, project.ID)
	assert.ErrorIs(t, err, ErrNotAuthorized)
}

func TestEnsureProjectAccess_UnauthenticatedDenied(t *testing.T) {
	resolver, cleanup := setupGroupAdminTestResolver(t, true)
	defer cleanup()

	groupID := cuid.New()
	project := &models.Project{ID: cuid.New(), Name: "Test Project", GroupID: &groupID}
	require.NoError(t, resolver.ProjectRepo.Create(context.Background(), project))

	// No auth context
	err := resolver.ensureProjectAccess(context.Background(), project.ID)
	assert.ErrorIs(t, err, ErrNotAuthenticated)
}

func TestEnsureProjectAccess_OrphanedProjectDeniedForRegularUser(t *testing.T) {
	resolver, cleanup := setupGroupAdminTestResolver(t, true)
	defer cleanup()

	// Project without a group (orphaned/legacy)
	project := &models.Project{ID: cuid.New(), Name: "Orphaned Project"}
	require.NoError(t, resolver.ProjectRepo.Create(context.Background(), project))

	groupID := cuid.New()
	ctx := createMemberContext("user-1", "user@test.com", groupID)
	err := resolver.ensureProjectAccess(ctx, project.ID)
	assert.ErrorIs(t, err, ErrNotAuthorized)
}

func TestEnsureProjectAccess_OrphanedProjectAllowedForAdmin(t *testing.T) {
	resolver, cleanup := setupGroupAdminTestResolver(t, true)
	defer cleanup()

	// Project without a group (orphaned/legacy)
	project := &models.Project{ID: cuid.New(), Name: "Orphaned Project"}
	require.NoError(t, resolver.ProjectRepo.Create(context.Background(), project))

	ctx := createAdminContext()
	err := resolver.ensureProjectAccess(ctx, project.ID)
	assert.NoError(t, err)
}

func TestEnsureProjectAccess_NonexistentProject(t *testing.T) {
	resolver, cleanup := setupGroupAdminTestResolver(t, true)
	defer cleanup()

	ctx := createMemberContext("user-1", "user@test.com", cuid.New())
	err := resolver.ensureProjectAccess(ctx, "nonexistent-project-id")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "project not found")
}

// --- getAccessibleGroupIDs tests ---

func TestGetAccessibleGroupIDs_AuthDisabled(t *testing.T) {
	resolver, cleanup := setupGroupAdminTestResolver(t, false)
	defer cleanup()

	ids, err := resolver.getAccessibleGroupIDs(context.Background())
	assert.NoError(t, err)
	assert.Nil(t, ids, "nil means no filtering when auth disabled")
}

func TestGetAccessibleGroupIDs_AdminGetsNil(t *testing.T) {
	resolver, cleanup := setupGroupAdminTestResolver(t, true)
	defer cleanup()

	ctx := createAdminContext()
	ids, err := resolver.getAccessibleGroupIDs(ctx)
	assert.NoError(t, err)
	assert.Nil(t, ids, "nil means no filtering for admin")
}

func TestGetAccessibleGroupIDs_RegularUserGetsGroupIDs(t *testing.T) {
	resolver, cleanup := setupGroupAdminTestResolver(t, true)
	defer cleanup()

	groupID := cuid.New()
	ctx := createMemberContext("user-1", "user@test.com", groupID)
	ids, err := resolver.getAccessibleGroupIDs(ctx)
	assert.NoError(t, err)
	assert.Equal(t, []string{groupID}, ids)
}

func TestGetAccessibleGroupIDs_UnauthenticatedReturnsError(t *testing.T) {
	resolver, cleanup := setupGroupAdminTestResolver(t, true)
	defer cleanup()

	_, err := resolver.getAccessibleGroupIDs(context.Background())
	assert.ErrorIs(t, err, ErrNotAuthenticated)
}

// ============================================================
// CreateUserGroup membership + transaction tests
// ============================================================

func TestCreateUserGroup_AddsCreatorAsMember(t *testing.T) {
	resolver, cleanup := setupGroupAdminTestResolver(t, true)
	defer cleanup()

	ctx := createAdminContext()
	input := generated.CreateUserGroupInput{
		Name: "New Test Group",
	}

	group, err := resolver.createUserGroup(ctx, input)
	require.NoError(t, err)
	require.NotNil(t, group)

	// Verify membership was created
	member, err := resolver.GroupRepo.FindMember(context.Background(), "admin-user-id", group.ID)
	require.NoError(t, err)
	require.NotNil(t, member)
	assert.Equal(t, models.GroupRoleGroupAdmin, member.Role)
}

func TestCreateUserGroup_FailsWithoutUserID(t *testing.T) {
	resolver, cleanup := setupGroupAdminTestResolver(t, true)
	defer cleanup()

	// Create admin context without userID
	sess := &session.CachedSession{
		UserID:    "",
		Email:     "admin@test.com",
		Role:      "ADMIN",
		SessionID: "admin-session-id",
	}
	ctx := context.Background()
	ctx = context.WithValue(ctx, middleware.ContextKeySession, sess)
	ctx = context.WithValue(ctx, middleware.ContextKeyUserID, "")
	ctx = context.WithValue(ctx, middleware.ContextKeyUserRole, "ADMIN")

	input := generated.CreateUserGroupInput{
		Name: "Should Fail Group",
	}

	_, err := resolver.createUserGroup(ctx, input)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no authenticated user")

	// Verify no group was created (transaction rollback)
	var count int64
	resolver.db.Model(&models.UserGroup{}).Where("name = ?", "Should Fail Group").Count(&count)
	assert.Equal(t, int64(0), count)
}

// ============================================================
// MyGroups tests
// ============================================================

func TestMyGroups_AdminSeesOnlyMembershipGroups(t *testing.T) {
	resolver, cleanup := setupGroupAdminTestResolver(t, true)
	defer cleanup()

	// Create two groups
	group1 := createTestGroup(t, resolver, "Admin's Group")
	group2 := createTestGroup(t, resolver, "Other Group")
	_ = group2 // exists but admin is not a member

	// Create admin context with membership in only group1
	sess := &session.CachedSession{
		UserID:    "admin-user-id",
		Email:     "admin@test.com",
		Role:      "ADMIN",
		SessionID: "admin-session-id",
	}
	ctx := context.Background()
	ctx = context.WithValue(ctx, middleware.ContextKeySession, sess)
	ctx = context.WithValue(ctx, middleware.ContextKeyUserID, "admin-user-id")
	ctx = context.WithValue(ctx, middleware.ContextKeyUserEmail, "admin@test.com")
	ctx = context.WithValue(ctx, middleware.ContextKeyUserRole, "ADMIN")
	ctx = context.WithValue(ctx, middleware.ContextKeyUserGroups, []middleware.UserGroupMembership{
		{GroupID: group1.ID, Role: "GROUP_ADMIN"},
	})

	queryRes := resolver.Query()
	groups, err := queryRes.MyGroups(ctx)
	require.NoError(t, err)
	assert.Len(t, groups, 1)
	assert.Equal(t, group1.ID, groups[0].ID)
}

func TestMyGroups_RegularUserSeesOnlyMembershipGroups(t *testing.T) {
	resolver, cleanup := setupGroupAdminTestResolver(t, true)
	defer cleanup()

	group1 := createTestGroup(t, resolver, "User's Group")
	group2 := createTestGroup(t, resolver, "Other Group")
	_ = group2

	ctx := createMemberContext("user-1", "user@test.com", group1.ID)

	queryRes := resolver.Query()
	groups, err := queryRes.MyGroups(ctx)
	require.NoError(t, err)
	assert.Len(t, groups, 1)
	assert.Equal(t, group1.ID, groups[0].ID)
}
