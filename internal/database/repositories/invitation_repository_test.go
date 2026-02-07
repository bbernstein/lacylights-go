package repositories

import (
	"context"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"github.com/lucsky/cuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"github.com/bbernstein/lacylights-go/internal/database/models"
)

func setupInvitationTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	require.NoError(t, err)

	err = db.AutoMigrate(
		&models.User{},
		&models.UserGroup{},
		&models.GroupInvitation{},
		&models.UserGroupMember{},
	)
	require.NoError(t, err)
	return db
}

func TestInvitationRepository_FindByID(t *testing.T) {
	db := setupInvitationTestDB(t)
	repo := NewInvitationRepository(db)

	group := &models.UserGroup{ID: cuid.New(), Name: "Test Group"}
	require.NoError(t, db.Create(group).Error)
	user := &models.User{ID: cuid.New(), Email: "inviter@test.com", Role: "USER", IsActive: true}
	require.NoError(t, db.Create(user).Error)

	invitation, err := repo.Create(context.Background(), group.ID, "invitee@test.com", user.ID, models.GroupRoleMember, time.Now().Add(7*24*time.Hour))
	require.NoError(t, err)

	t.Run("found", func(t *testing.T) {
		result, err := repo.FindByID(context.Background(), invitation.ID)
		require.NoError(t, err)
		assert.NotNil(t, result)
		assert.Equal(t, invitation.ID, result.ID)
		assert.Equal(t, group.ID, result.GroupID)
		assert.Equal(t, "invitee@test.com", result.Email)
	})

	t.Run("not found", func(t *testing.T) {
		result, err := repo.FindByID(context.Background(), "non-existent")
		require.NoError(t, err)
		assert.Nil(t, result)
	})
}

func TestInvitationRepository_FindPendingByGroupAndEmail(t *testing.T) {
	db := setupInvitationTestDB(t)
	repo := NewInvitationRepository(db)

	group := &models.UserGroup{ID: cuid.New(), Name: "Test Group"}
	require.NoError(t, db.Create(group).Error)
	user := &models.User{ID: cuid.New(), Email: "inviter@test.com", Role: "USER", IsActive: true}
	require.NoError(t, db.Create(user).Error)

	_, err := repo.Create(context.Background(), group.ID, "invitee@test.com", user.ID, models.GroupRoleMember, time.Now().Add(7*24*time.Hour))
	require.NoError(t, err)

	t.Run("found", func(t *testing.T) {
		result, err := repo.FindPendingByGroupAndEmail(context.Background(), group.ID, "invitee@test.com")
		require.NoError(t, err)
		assert.NotNil(t, result)
	})

	t.Run("not found - wrong email", func(t *testing.T) {
		result, err := repo.FindPendingByGroupAndEmail(context.Background(), group.ID, "other@test.com")
		require.NoError(t, err)
		assert.Nil(t, result)
	})

	t.Run("not found - wrong group", func(t *testing.T) {
		result, err := repo.FindPendingByGroupAndEmail(context.Background(), "other-group", "invitee@test.com")
		require.NoError(t, err)
		assert.Nil(t, result)
	})
}

func TestInvitationRepository_UpdateStatus(t *testing.T) {
	db := setupInvitationTestDB(t)
	repo := NewInvitationRepository(db)

	group := &models.UserGroup{ID: cuid.New(), Name: "Test Group"}
	require.NoError(t, db.Create(group).Error)
	user := &models.User{ID: cuid.New(), Email: "inviter@test.com", Role: "USER", IsActive: true}
	require.NoError(t, db.Create(user).Error)

	invitation, err := repo.Create(context.Background(), group.ID, "invitee@test.com", user.ID, models.GroupRoleMember, time.Now().Add(7*24*time.Hour))
	require.NoError(t, err)

	t.Run("accept", func(t *testing.T) {
		now := time.Now()
		err := repo.UpdateStatus(context.Background(), invitation.ID, models.InvitationStatusAccepted, &now)
		require.NoError(t, err)

		result, err := repo.FindByID(context.Background(), invitation.ID)
		require.NoError(t, err)
		assert.Equal(t, models.InvitationStatusAccepted, result.Status)
		assert.NotNil(t, result.AcceptedAt)
	})

	t.Run("not found", func(t *testing.T) {
		err := repo.UpdateStatus(context.Background(), "non-existent", models.InvitationStatusDeclined, nil)
		require.Error(t, err)
	})
}

func TestInvitationRepository_FindByGroupID(t *testing.T) {
	db := setupInvitationTestDB(t)
	repo := NewInvitationRepository(db)

	group := &models.UserGroup{ID: cuid.New(), Name: "Test Group"}
	require.NoError(t, db.Create(group).Error)
	user := &models.User{ID: cuid.New(), Email: "inviter@test.com", Role: "USER", IsActive: true}
	require.NoError(t, db.Create(user).Error)

	_, err := repo.Create(context.Background(), group.ID, "a@test.com", user.ID, models.GroupRoleMember, time.Now().Add(7*24*time.Hour))
	require.NoError(t, err)
	_, err = repo.Create(context.Background(), group.ID, "b@test.com", user.ID, models.GroupRoleMember, time.Now().Add(7*24*time.Hour))
	require.NoError(t, err)

	results, err := repo.FindByGroupID(context.Background(), group.ID)
	require.NoError(t, err)
	assert.Len(t, results, 2)
}

func TestInvitationRepository_FindPendingByEmail(t *testing.T) {
	db := setupInvitationTestDB(t)
	repo := NewInvitationRepository(db)

	group1 := &models.UserGroup{ID: cuid.New(), Name: "Group 1"}
	require.NoError(t, db.Create(group1).Error)
	group2 := &models.UserGroup{ID: cuid.New(), Name: "Group 2"}
	require.NoError(t, db.Create(group2).Error)
	user := &models.User{ID: cuid.New(), Email: "inviter@test.com", Role: "USER", IsActive: true}
	require.NoError(t, db.Create(user).Error)

	_, err := repo.Create(context.Background(), group1.ID, "invitee@test.com", user.ID, models.GroupRoleMember, time.Now().Add(7*24*time.Hour))
	require.NoError(t, err)
	_, err = repo.Create(context.Background(), group2.ID, "invitee@test.com", user.ID, models.GroupRoleMember, time.Now().Add(7*24*time.Hour))
	require.NoError(t, err)

	results, err := repo.FindPendingByEmail(context.Background(), "invitee@test.com")
	require.NoError(t, err)
	assert.Len(t, results, 2)
}

func TestInvitationRepository_ExpireOverdueInvitations(t *testing.T) {
	db := setupInvitationTestDB(t)
	repo := NewInvitationRepository(db)

	group := &models.UserGroup{ID: cuid.New(), Name: "Test Group"}
	require.NoError(t, db.Create(group).Error)
	user := &models.User{ID: cuid.New(), Email: "inviter@test.com", Role: "USER", IsActive: true}
	require.NoError(t, db.Create(user).Error)

	// Create one expired invitation
	_, err := repo.Create(context.Background(), group.ID, "expired@test.com", user.ID, models.GroupRoleMember, time.Now().Add(-1*time.Hour))
	require.NoError(t, err)

	// Create one valid invitation
	_, err = repo.Create(context.Background(), group.ID, "valid@test.com", user.ID, models.GroupRoleMember, time.Now().Add(7*24*time.Hour))
	require.NoError(t, err)

	count, err := repo.ExpireOverdueInvitations(context.Background())
	require.NoError(t, err)
	assert.Equal(t, int64(1), count)

	// Verify the expired one was updated
	pending, err := repo.FindPendingByEmail(context.Background(), "expired@test.com")
	require.NoError(t, err)
	assert.Len(t, pending, 0)

	// Verify the valid one is still pending
	pending, err = repo.FindPendingByEmail(context.Background(), "valid@test.com")
	require.NoError(t, err)
	assert.Len(t, pending, 1)
}

func TestInvitationRepository_FindByIDForUpdate(t *testing.T) {
	db := setupInvitationTestDB(t)
	repo := NewInvitationRepository(db)

	group := &models.UserGroup{ID: cuid.New(), Name: "Test Group"}
	require.NoError(t, db.Create(group).Error)
	user := &models.User{ID: cuid.New(), Email: "inviter@test.com", Role: "USER", IsActive: true}
	require.NoError(t, db.Create(user).Error)

	invitation, err := repo.Create(context.Background(), group.ID, "invitee@test.com", user.ID, models.GroupRoleMember, time.Now().Add(7*24*time.Hour))
	require.NoError(t, err)

	t.Run("found", func(t *testing.T) {
		result, err := repo.FindByIDForUpdate(db, invitation.ID)
		require.NoError(t, err)
		assert.NotNil(t, result)
		assert.Equal(t, invitation.ID, result.ID)
	})

	t.Run("not found", func(t *testing.T) {
		result, err := repo.FindByIDForUpdate(db, "non-existent")
		require.NoError(t, err)
		assert.Nil(t, result)
	})
}

func TestInvitationRepository_DB(t *testing.T) {
	db := setupInvitationTestDB(t)
	repo := NewInvitationRepository(db)

	result := repo.DB()
	assert.NotNil(t, result)
	assert.Equal(t, db, result)
}

func TestNewInvitationID(t *testing.T) {
	id1 := NewInvitationID()
	id2 := NewInvitationID()

	assert.NotEmpty(t, id1)
	assert.NotEmpty(t, id2)
	assert.NotEqual(t, id1, id2, "each call should generate a unique ID")
}

func TestInvitationRepository_CreateWithDefaults(t *testing.T) {
	db := setupInvitationTestDB(t)
	repo := NewInvitationRepository(db)

	group := &models.UserGroup{ID: cuid.New(), Name: "Test Group"}
	require.NoError(t, db.Create(group).Error)
	user := &models.User{ID: cuid.New(), Email: "inviter@test.com", Role: "USER", IsActive: true}
	require.NoError(t, db.Create(user).Error)

	t.Run("default role and TTL", func(t *testing.T) {
		invitation, err := repo.CreateWithDefaults(context.Background(), GroupInvitationInput{
			GroupID:     group.ID,
			Email:       "test@test.com",
			InvitedByID: user.ID,
		})
		require.NoError(t, err)
		assert.Equal(t, models.GroupRoleMember, invitation.Role)
		assert.True(t, invitation.ExpiresAt.After(time.Now().Add(6*24*time.Hour))) // At least 6 days out
	})

	t.Run("custom role", func(t *testing.T) {
		invitation, err := repo.CreateWithDefaults(context.Background(), GroupInvitationInput{
			GroupID:     group.ID,
			Email:       "admin-invite@test.com",
			InvitedByID: user.ID,
			Role:        models.GroupRoleGroupAdmin,
		})
		require.NoError(t, err)
		assert.Equal(t, models.GroupRoleGroupAdmin, invitation.Role)
	})

	t.Run("custom TTL", func(t *testing.T) {
		invitation, err := repo.CreateWithDefaults(context.Background(), GroupInvitationInput{
			GroupID:     group.ID,
			Email:       "ttl-test@test.com",
			InvitedByID: user.ID,
			TTL:         1 * time.Hour,
		})
		require.NoError(t, err)
		assert.True(t, invitation.ExpiresAt.Before(time.Now().Add(2*time.Hour)))
	})
}
