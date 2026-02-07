package repositories

import (
	"context"
	"testing"

	"github.com/glebarez/sqlite"
	"github.com/lucsky/cuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"github.com/bbernstein/lacylights-go/internal/database/models"
)

func setupGroupTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	require.NoError(t, err)

	err = db.AutoMigrate(
		&models.User{},
		&models.UserGroup{},
		&models.UserGroupMember{},
		&models.Device{},
		&models.DeviceGroupMember{},
	)
	require.NoError(t, err)
	return db
}

func TestGroupRepository_FindGroupByID(t *testing.T) {
	db := setupGroupTestDB(t)
	repo := NewGroupRepository(db)

	group := &models.UserGroup{ID: cuid.New(), Name: "Test Group"}
	require.NoError(t, db.Create(group).Error)

	t.Run("found", func(t *testing.T) {
		result, err := repo.FindGroupByID(context.Background(), group.ID)
		require.NoError(t, err)
		assert.Equal(t, group.ID, result.ID)
		assert.Equal(t, "Test Group", result.Name)
	})

	t.Run("not found", func(t *testing.T) {
		result, err := repo.FindGroupByID(context.Background(), "non-existent")
		require.NoError(t, err)
		assert.Nil(t, result)
	})
}

func TestGroupRepository_FindGroupByID_DBError(t *testing.T) {
	db := setupGroupTestDB(t)
	repo := NewGroupRepository(db)

	// Close the database to trigger a DB error
	sqlDB, err := db.DB()
	require.NoError(t, err)
	_ = sqlDB.Close()

	result, err := repo.FindGroupByID(context.Background(), "any-id")
	assert.Error(t, err)
	assert.Nil(t, result)
}

func TestGroupRepository_FindMember_DBError(t *testing.T) {
	db := setupGroupTestDB(t)
	repo := NewGroupRepository(db)

	sqlDB, err := db.DB()
	require.NoError(t, err)
	_ = sqlDB.Close()

	result, err := repo.FindMember(context.Background(), "user-id", "group-id")
	assert.Error(t, err)
	assert.Nil(t, result)
}

func TestGroupRepository_FindMember(t *testing.T) {
	db := setupGroupTestDB(t)
	repo := NewGroupRepository(db)

	group := &models.UserGroup{ID: cuid.New(), Name: "Test Group"}
	require.NoError(t, db.Create(group).Error)
	user := &models.User{ID: cuid.New(), Email: "test@test.com", Role: "USER", IsActive: true}
	require.NoError(t, db.Create(user).Error)
	member := &models.UserGroupMember{ID: cuid.New(), UserID: user.ID, GroupID: group.ID, Role: models.GroupRoleMember}
	require.NoError(t, db.Create(member).Error)

	t.Run("found", func(t *testing.T) {
		result, err := repo.FindMember(context.Background(), user.ID, group.ID)
		require.NoError(t, err)
		assert.NotNil(t, result)
		assert.Equal(t, user.ID, result.UserID)
		assert.Equal(t, group.ID, result.GroupID)
		assert.Equal(t, models.GroupRoleMember, result.Role)
	})

	t.Run("not found", func(t *testing.T) {
		result, err := repo.FindMember(context.Background(), "other-user", group.ID)
		require.NoError(t, err)
		assert.Nil(t, result)
	})
}

func TestGroupRepository_UpdateMemberRole(t *testing.T) {
	db := setupGroupTestDB(t)
	repo := NewGroupRepository(db)

	group := &models.UserGroup{ID: cuid.New(), Name: "Test Group"}
	require.NoError(t, db.Create(group).Error)
	user := &models.User{ID: cuid.New(), Email: "test@test.com", Role: "USER", IsActive: true}
	require.NoError(t, db.Create(user).Error)
	member := &models.UserGroupMember{ID: cuid.New(), UserID: user.ID, GroupID: group.ID, Role: models.GroupRoleMember}
	require.NoError(t, db.Create(member).Error)

	t.Run("success", func(t *testing.T) {
		err := repo.UpdateMemberRole(context.Background(), user.ID, group.ID, models.GroupRoleGroupAdmin)
		require.NoError(t, err)

		result, err := repo.FindMember(context.Background(), user.ID, group.ID)
		require.NoError(t, err)
		assert.Equal(t, models.GroupRoleGroupAdmin, result.Role)
	})

	t.Run("not found", func(t *testing.T) {
		err := repo.UpdateMemberRole(context.Background(), "non-existent", group.ID, models.GroupRoleMember)
		require.Error(t, err)
	})
}

func TestGroupRepository_AddMember(t *testing.T) {
	db := setupGroupTestDB(t)
	repo := NewGroupRepository(db)

	group := &models.UserGroup{ID: cuid.New(), Name: "Test Group"}
	require.NoError(t, db.Create(group).Error)
	user := &models.User{ID: cuid.New(), Email: "test@test.com", Role: "USER", IsActive: true}
	require.NoError(t, db.Create(user).Error)

	result, err := repo.AddMember(context.Background(), user.ID, group.ID, models.GroupRoleGroupAdmin)
	require.NoError(t, err)
	assert.NotEmpty(t, result.ID)
	assert.Equal(t, user.ID, result.UserID)
	assert.Equal(t, group.ID, result.GroupID)
	assert.Equal(t, models.GroupRoleGroupAdmin, result.Role)

	// Verify in database
	found, err := repo.FindMember(context.Background(), user.ID, group.ID)
	require.NoError(t, err)
	assert.NotNil(t, found)
}

func TestGroupRepository_RemoveMember(t *testing.T) {
	db := setupGroupTestDB(t)
	repo := NewGroupRepository(db)

	group := &models.UserGroup{ID: cuid.New(), Name: "Test Group"}
	require.NoError(t, db.Create(group).Error)
	user := &models.User{ID: cuid.New(), Email: "test@test.com", Role: "USER", IsActive: true}
	require.NoError(t, db.Create(user).Error)
	member := &models.UserGroupMember{ID: cuid.New(), UserID: user.ID, GroupID: group.ID, Role: models.GroupRoleMember}
	require.NoError(t, db.Create(member).Error)

	t.Run("success", func(t *testing.T) {
		removed, err := repo.RemoveMember(context.Background(), user.ID, group.ID)
		require.NoError(t, err)
		assert.True(t, removed)

		found, err := repo.FindMember(context.Background(), user.ID, group.ID)
		require.NoError(t, err)
		assert.Nil(t, found)
	})

	t.Run("not found", func(t *testing.T) {
		removed, err := repo.RemoveMember(context.Background(), "non-existent", group.ID)
		require.NoError(t, err)
		assert.False(t, removed)
	})
}

func TestGroupRepository_AddDeviceToGroup(t *testing.T) {
	db := setupGroupTestDB(t)
	repo := NewGroupRepository(db)

	group := &models.UserGroup{ID: cuid.New(), Name: "Test Group"}
	require.NoError(t, db.Create(group).Error)
	device := &models.Device{ID: cuid.New(), Name: "Test Device", Fingerprint: "test-fp", Status: models.DeviceStatusApproved, Permissions: models.DevicePermissionsOperator, DefaultRole: "OPERATOR"}
	require.NoError(t, db.Create(device).Error)

	result, err := repo.AddDeviceToGroup(context.Background(), device.ID, group.ID)
	require.NoError(t, err)
	assert.NotEmpty(t, result.ID)
	assert.Equal(t, device.ID, result.DeviceID)
	assert.Equal(t, group.ID, result.GroupID)
}

func TestGroupRepository_FindDeviceGroupMember(t *testing.T) {
	db := setupGroupTestDB(t)
	repo := NewGroupRepository(db)

	group := &models.UserGroup{ID: cuid.New(), Name: "Test Group"}
	require.NoError(t, db.Create(group).Error)
	device := &models.Device{ID: cuid.New(), Name: "Test Device", Fingerprint: "test-fp", Status: models.DeviceStatusApproved, Permissions: models.DevicePermissionsOperator, DefaultRole: "OPERATOR"}
	require.NoError(t, db.Create(device).Error)

	_, err := repo.AddDeviceToGroup(context.Background(), device.ID, group.ID)
	require.NoError(t, err)

	t.Run("found", func(t *testing.T) {
		result, err := repo.FindDeviceGroupMember(context.Background(), device.ID, group.ID)
		require.NoError(t, err)
		assert.NotNil(t, result)
	})

	t.Run("not found", func(t *testing.T) {
		result, err := repo.FindDeviceGroupMember(context.Background(), "other-device", group.ID)
		require.NoError(t, err)
		assert.Nil(t, result)
	})
}

func TestGroupRepository_RemoveDeviceFromGroup(t *testing.T) {
	db := setupGroupTestDB(t)
	repo := NewGroupRepository(db)

	group := &models.UserGroup{ID: cuid.New(), Name: "Test Group"}
	require.NoError(t, db.Create(group).Error)
	device := &models.Device{ID: cuid.New(), Name: "Test Device", Fingerprint: "test-fp", Status: models.DeviceStatusApproved, Permissions: models.DevicePermissionsOperator, DefaultRole: "OPERATOR"}
	require.NoError(t, db.Create(device).Error)

	_, err := repo.AddDeviceToGroup(context.Background(), device.ID, group.ID)
	require.NoError(t, err)

	removed, err := repo.RemoveDeviceFromGroup(context.Background(), device.ID, group.ID)
	require.NoError(t, err)
	assert.True(t, removed)

	// Verify removed
	result, err := repo.FindDeviceGroupMember(context.Background(), device.ID, group.ID)
	require.NoError(t, err)
	assert.Nil(t, result)
}

func TestGroupRepository_FindDeviceByID(t *testing.T) {
	db := setupGroupTestDB(t)
	repo := NewGroupRepository(db)

	device := &models.Device{ID: cuid.New(), Name: "Test Device", Fingerprint: "test-fp", Status: models.DeviceStatusApproved, Permissions: models.DevicePermissionsOperator, DefaultRole: "OPERATOR"}
	require.NoError(t, db.Create(device).Error)

	t.Run("found", func(t *testing.T) {
		result, err := repo.FindDeviceByID(context.Background(), device.ID)
		require.NoError(t, err)
		assert.NotNil(t, result)
		assert.Equal(t, device.ID, result.ID)
	})

	t.Run("not found", func(t *testing.T) {
		result, err := repo.FindDeviceByID(context.Background(), "non-existent")
		require.NoError(t, err)
		assert.Nil(t, result)
	})
}

func TestGroupRepository_FindUserByEmail(t *testing.T) {
	db := setupGroupTestDB(t)
	repo := NewGroupRepository(db)

	user := &models.User{ID: cuid.New(), Email: "test@test.com", Role: "USER", IsActive: true}
	require.NoError(t, db.Create(user).Error)

	t.Run("found", func(t *testing.T) {
		result, err := repo.FindUserByEmail(context.Background(), "test@test.com")
		require.NoError(t, err)
		assert.NotNil(t, result)
		assert.Equal(t, user.ID, result.ID)
	})

	t.Run("not found", func(t *testing.T) {
		result, err := repo.FindUserByEmail(context.Background(), "unknown@test.com")
		require.NoError(t, err)
		assert.Nil(t, result)
	})
}
