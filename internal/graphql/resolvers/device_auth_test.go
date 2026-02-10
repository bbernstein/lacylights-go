package resolvers

import (
	"context"
	"testing"
	"time"

	"github.com/99designs/gqlgen/graphql"
	"github.com/lucsky/cuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/bbernstein/lacylights-go/internal/auth"
	"github.com/bbernstein/lacylights-go/internal/auth/session"
	"github.com/bbernstein/lacylights-go/internal/database/models"
	"github.com/bbernstein/lacylights-go/internal/graphql/generated"
	"github.com/bbernstein/lacylights-go/internal/middleware"
)

func setupDeviceAuthTestResolver(t *testing.T, authEnabled, deviceAuthEnabled bool) (*Resolver, func()) {
	t.Helper()

	client, resolver, cleanup := testSetup(t)
	_ = client // We don't need the GraphQL client for these tests

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
			DeviceAuthEnabled:    deviceAuthEnabled,
		})
		require.NoError(t, err)
		resolver.SetAuthService(authService)
	}

	return resolver, cleanup
}

// createAdminContext creates a context with admin authentication for testing.
func createAdminContext() context.Context {
	sess := &session.CachedSession{
		UserID:    "admin-user-id",
		Email:     "admin@test.com",
		Role:      "ADMIN",
		SessionID: "admin-session-id",
	}
	ctx := context.Background()
	ctx = context.WithValue(ctx, middleware.ContextKeySession, sess)
	ctx = context.WithValue(ctx, middleware.ContextKeyUserID, "admin-user-id")
	ctx = context.WithValue(ctx, middleware.ContextKeyUserRole, "ADMIN")
	return ctx
}

func TestCheckDevice_AuthDisabled(t *testing.T) {
	resolver, cleanup := setupDeviceAuthTestResolver(t, false, false)
	defer cleanup()
	ctx := context.Background()

	result, err := resolver.checkDevice(ctx, "test-fingerprint")
	require.NoError(t, err)
	assert.Equal(t, generated.DeviceStatusApproved, result.Status)
	assert.Contains(t, *result.Message, "Authentication is disabled")
}

func TestCheckDevice_DeviceAuthDisabled(t *testing.T) {
	resolver, cleanup := setupDeviceAuthTestResolver(t, true, false)
	defer cleanup()
	ctx := context.Background()

	result, err := resolver.checkDevice(ctx, "test-fingerprint")
	require.NoError(t, err)
	assert.Equal(t, generated.DeviceStatusApproved, result.Status)
	assert.Contains(t, *result.Message, "Device authentication is disabled")
}

func TestCheckDevice_UnregisteredDevice(t *testing.T) {
	resolver, cleanup := setupDeviceAuthTestResolver(t, true, true)
	defer cleanup()
	ctx := context.Background()

	result, err := resolver.checkDevice(ctx, "unknown-fingerprint")
	require.NoError(t, err)
	assert.Equal(t, generated.DeviceStatusPending, result.Status)
	assert.Nil(t, result.Device)
	assert.Contains(t, *result.Message, "not registered")
}

func TestCheckDevice_PendingDevice(t *testing.T) {
	resolver, cleanup := setupDeviceAuthTestResolver(t, true, true)
	defer cleanup()

	// Create a pending device
	device := models.Device{
		ID:          cuid.New(),
		Name:        "Test Device",
		Fingerprint: "pending-fingerprint",
		Status:      models.DeviceStatusPending,
		Permissions: models.DevicePermissionsReadOnly,
		DefaultRole: "PLAYER",
	}
	require.NoError(t, resolver.db.Create(&device).Error)

	ctx := context.Background()
	result, err := resolver.checkDevice(ctx, "pending-fingerprint")
	require.NoError(t, err)
	assert.Equal(t, generated.DeviceStatusPending, result.Status)
	assert.NotNil(t, result.Device)
	assert.Equal(t, device.ID, result.Device.ID)
	assert.Contains(t, *result.Message, "pending approval")
}

func TestCheckDevice_ApprovedDevice(t *testing.T) {
	resolver, cleanup := setupDeviceAuthTestResolver(t, true, true)
	defer cleanup()

	// Create an approved device
	device := models.Device{
		ID:           cuid.New(),
		Name:         "Approved Device",
		Fingerprint:  "approved-fingerprint",
		Status:       models.DeviceStatusApproved,
		IsAuthorized: true,
		Permissions:  models.DevicePermissionsOperator,
		DefaultRole:  "OPERATOR",
	}
	require.NoError(t, resolver.db.Create(&device).Error)

	ctx := context.Background()
	result, err := resolver.checkDevice(ctx, "approved-fingerprint")
	require.NoError(t, err)
	assert.Equal(t, generated.DeviceStatusApproved, result.Status)
	assert.NotNil(t, result.Device)
	assert.Equal(t, device.ID, result.Device.ID)
	assert.Contains(t, *result.Message, "approved")
}

func TestCheckDevice_RevokedDevice(t *testing.T) {
	resolver, cleanup := setupDeviceAuthTestResolver(t, true, true)
	defer cleanup()

	// Create a revoked device
	device := models.Device{
		ID:          cuid.New(),
		Name:        "Revoked Device",
		Fingerprint: "revoked-fingerprint",
		Status:      models.DeviceStatusRevoked,
		Permissions: models.DevicePermissionsReadOnly,
		DefaultRole: "PLAYER",
	}
	require.NoError(t, resolver.db.Create(&device).Error)

	ctx := context.Background()
	result, err := resolver.checkDevice(ctx, "revoked-fingerprint")
	require.NoError(t, err)
	assert.Equal(t, generated.DeviceStatusRevoked, result.Status)
	assert.NotNil(t, result.Device)
	assert.Contains(t, *result.Message, "revoked")
}

func TestRegisterDevice_AuthDisabled(t *testing.T) {
	resolver, cleanup := setupDeviceAuthTestResolver(t, false, false)
	defer cleanup()
	ctx := context.Background()

	result, err := resolver.registerDevice(ctx, "test-fingerprint", "Test Device")
	require.NoError(t, err)
	assert.True(t, result.Success)
	assert.Contains(t, result.Message, "not required")
}

func TestRegisterDevice_NewDevice(t *testing.T) {
	resolver, cleanup := setupDeviceAuthTestResolver(t, true, true)
	defer cleanup()
	ctx := context.Background()

	result, err := resolver.registerDevice(ctx, "new-fingerprint", "New Device")
	require.NoError(t, err)
	assert.True(t, result.Success)
	assert.NotNil(t, result.Device)
	assert.Equal(t, "New Device", result.Device.Name)
	assert.Equal(t, models.DeviceStatusPending, result.Device.Status)
	assert.Contains(t, result.Message, "registered successfully")

	// Verify device was created in database
	var dbDevice models.Device
	err = resolver.db.Where("fingerprint = ?", "new-fingerprint").First(&dbDevice).Error
	require.NoError(t, err)
	assert.Equal(t, "New Device", dbDevice.Name)
	assert.Equal(t, models.DeviceStatusPending, dbDevice.Status)
}

func TestRegisterDevice_ExistingDevice(t *testing.T) {
	resolver, cleanup := setupDeviceAuthTestResolver(t, true, true)
	defer cleanup()

	// Create an existing device
	device := models.Device{
		ID:          cuid.New(),
		Name:        "Existing Device",
		Fingerprint: "existing-fingerprint",
		Status:      models.DeviceStatusPending,
		Permissions: models.DevicePermissionsReadOnly,
		DefaultRole: "PLAYER",
	}
	require.NoError(t, resolver.db.Create(&device).Error)

	ctx := context.Background()
	result, err := resolver.registerDevice(ctx, "existing-fingerprint", "Updated Name")
	require.NoError(t, err)
	assert.True(t, result.Success)
	assert.NotNil(t, result.Device)
	// Should return existing device, not update name
	assert.Equal(t, "Existing Device", result.Device.Name)
	assert.Contains(t, result.Message, "already registered")
}

func TestRegisterDevice_EmptyFingerprint(t *testing.T) {
	resolver, cleanup := setupDeviceAuthTestResolver(t, true, true)
	defer cleanup()
	ctx := context.Background()

	result, err := resolver.registerDevice(ctx, "", "Test Device")
	require.NoError(t, err)
	assert.False(t, result.Success)
	assert.Contains(t, result.Message, "Fingerprint is required")
}

func TestRegisterDevice_EmptyName(t *testing.T) {
	resolver, cleanup := setupDeviceAuthTestResolver(t, true, true)
	defer cleanup()
	ctx := context.Background()

	result, err := resolver.registerDevice(ctx, "test-fingerprint", "")
	require.NoError(t, err)
	assert.False(t, result.Success)
	assert.Contains(t, result.Message, "name is required")
}

// Tests for approveDevice

func TestApproveDevice_Success(t *testing.T) {
	resolver, cleanup := setupDeviceAuthTestResolver(t, true, true)
	defer cleanup()

	// Create a pending device
	device := models.Device{
		ID:          cuid.New(),
		Name:        "Pending Device",
		Fingerprint: "pending-fp",
		Status:      models.DeviceStatusPending,
		Permissions: models.DevicePermissionsReadOnly,
		DefaultRole: "PLAYER",
	}
	require.NoError(t, resolver.db.Create(&device).Error)

	// Use admin context for admin-only operations
	ctx := createAdminContext()
	result, err := resolver.approveDevice(ctx, device.ID, generated.DevicePermissionsOperator, nil)
	require.NoError(t, err)
	assert.Equal(t, models.DeviceStatusApproved, result.Status)
	assert.True(t, result.IsAuthorized)
	assert.Equal(t, string(generated.DevicePermissionsOperator), result.Permissions)
	assert.NotNil(t, result.ApprovedAt)

	// Verify in database
	var dbDevice models.Device
	require.NoError(t, resolver.db.First(&dbDevice, "id = ?", device.ID).Error)
	assert.Equal(t, models.DeviceStatusApproved, dbDevice.Status)
	assert.True(t, dbDevice.IsAuthorized)
}

func TestApproveDevice_NotFound(t *testing.T) {
	resolver, cleanup := setupDeviceAuthTestResolver(t, true, true)
	defer cleanup()
	ctx := createAdminContext()

	_, err := resolver.approveDevice(ctx, "non-existent-id", generated.DevicePermissionsReadOnly, nil)
	require.Error(t, err)
	assert.Equal(t, ErrDeviceNotFound, err)
}

func TestApproveDevice_NotAuthenticated(t *testing.T) {
	resolver, cleanup := setupDeviceAuthTestResolver(t, true, true)
	defer cleanup()
	ctx := context.Background() // No authentication

	device := models.Device{
		ID:          cuid.New(),
		Name:        "Test Device",
		Fingerprint: "test-fp",
		Status:      models.DeviceStatusPending,
		Permissions: models.DevicePermissionsReadOnly,
		DefaultRole: "PLAYER",
	}
	require.NoError(t, resolver.db.Create(&device).Error)

	_, err := resolver.approveDevice(ctx, device.ID, generated.DevicePermissionsOperator, nil)
	require.Error(t, err)
	assert.Equal(t, ErrNotAuthenticated, err)
}

func TestApproveDevice_AuthDisabled(t *testing.T) {
	resolver, cleanup := setupDeviceAuthTestResolver(t, false, false)
	defer cleanup()

	// Create a pending device
	device := models.Device{
		ID:          cuid.New(),
		Name:        "Test Device",
		Fingerprint: "test-fp",
		Status:      models.DeviceStatusPending,
		Permissions: models.DevicePermissionsReadOnly,
		DefaultRole: "PLAYER",
	}
	require.NoError(t, resolver.db.Create(&device).Error)

	ctx := context.Background()
	// When auth is disabled, admin operations are allowed
	result, err := resolver.approveDevice(ctx, device.ID, generated.DevicePermissionsAdmin, nil)
	require.NoError(t, err)
	assert.Equal(t, models.DeviceStatusApproved, result.Status)
}

// Tests for revokeDeviceAuth

func TestRevokeDeviceAuth_Success(t *testing.T) {
	resolver, cleanup := setupDeviceAuthTestResolver(t, true, true)
	defer cleanup()

	// Create an approved device
	device := models.Device{
		ID:           cuid.New(),
		Name:         "Approved Device",
		Fingerprint:  "approved-fp",
		Status:       models.DeviceStatusApproved,
		IsAuthorized: true,
		Permissions:  models.DevicePermissionsOperator,
		DefaultRole:  "OPERATOR",
	}
	require.NoError(t, resolver.db.Create(&device).Error)

	ctx := createAdminContext()
	result, err := resolver.revokeDeviceAuth(ctx, device.ID)
	require.NoError(t, err)
	assert.Equal(t, models.DeviceStatusRevoked, result.Status)
	assert.False(t, result.IsAuthorized)

	// Verify in database
	var dbDevice models.Device
	require.NoError(t, resolver.db.First(&dbDevice, "id = ?", device.ID).Error)
	assert.Equal(t, models.DeviceStatusRevoked, dbDevice.Status)
	assert.False(t, dbDevice.IsAuthorized)
}

func TestRevokeDeviceAuth_PreservesDefaultUser(t *testing.T) {
	resolver, cleanup := setupDeviceAuthTestResolver(t, true, true)
	defer cleanup()

	// Create a user to be the default user
	userName := "Default User"
	user := models.User{
		ID:    cuid.New(),
		Email: "revoke-preserve@test.com",
		Name:  &userName,
		Role:  "USER",
	}
	require.NoError(t, resolver.db.Create(&user).Error)

	// Create an approved device with a default user set
	device := models.Device{
		ID:            cuid.New(),
		Name:          "Device With Default User",
		Fingerprint:   "revoke-preserve-fp",
		Status:        models.DeviceStatusApproved,
		IsAuthorized:  true,
		Permissions:   models.DevicePermissionsOperator,
		DefaultRole:   "OPERATOR",
		DefaultUserID: &user.ID,
	}
	require.NoError(t, resolver.db.Create(&device).Error)

	ctx := createAdminContext()
	result, err := resolver.revokeDeviceAuth(ctx, device.ID)
	require.NoError(t, err)
	assert.Equal(t, models.DeviceStatusRevoked, result.Status)
	assert.False(t, result.IsAuthorized)

	// Verify default_user_id is preserved in database
	var dbDevice models.Device
	require.NoError(t, resolver.db.First(&dbDevice, "id = ?", device.ID).Error)
	assert.Equal(t, models.DeviceStatusRevoked, dbDevice.Status)
	assert.False(t, dbDevice.IsAuthorized)
	require.NotNil(t, dbDevice.DefaultUserID, "default_user_id should be preserved after revoke")
	assert.Equal(t, user.ID, *dbDevice.DefaultUserID)
}

func TestRevokeDeviceAuth_NotFound(t *testing.T) {
	resolver, cleanup := setupDeviceAuthTestResolver(t, true, true)
	defer cleanup()
	ctx := createAdminContext()

	_, err := resolver.revokeDeviceAuth(ctx, "non-existent-id")
	require.Error(t, err)
	assert.Equal(t, ErrDeviceNotFound, err)
}

func TestRevokeDeviceAuth_NotAuthenticated(t *testing.T) {
	resolver, cleanup := setupDeviceAuthTestResolver(t, true, true)
	defer cleanup()
	ctx := context.Background() // No authentication

	device := models.Device{
		ID:           cuid.New(),
		Name:         "Test Device",
		Fingerprint:  "test-fp",
		Status:       models.DeviceStatusApproved,
		IsAuthorized: true,
		Permissions:  models.DevicePermissionsOperator,
		DefaultRole:  "OPERATOR",
	}
	require.NoError(t, resolver.db.Create(&device).Error)

	_, err := resolver.revokeDeviceAuth(ctx, device.ID)
	require.Error(t, err)
	assert.Equal(t, ErrNotAuthenticated, err)
}

func TestRevokeDeviceAuth_AuthDisabled(t *testing.T) {
	resolver, cleanup := setupDeviceAuthTestResolver(t, false, false)
	defer cleanup()

	device := models.Device{
		ID:           cuid.New(),
		Name:         "Test Device",
		Fingerprint:  "test-fp",
		Status:       models.DeviceStatusApproved,
		IsAuthorized: true,
		Permissions:  models.DevicePermissionsReadOnly,
		DefaultRole:  "PLAYER",
	}
	require.NoError(t, resolver.db.Create(&device).Error)

	ctx := context.Background()
	result, err := resolver.revokeDeviceAuth(ctx, device.ID)
	require.NoError(t, err)
	assert.Equal(t, models.DeviceStatusRevoked, result.Status)
}

// Tests for updateDevicePermissions

func TestUpdateDevicePermissions_Success(t *testing.T) {
	resolver, cleanup := setupDeviceAuthTestResolver(t, true, true)
	defer cleanup()

	device := models.Device{
		ID:          cuid.New(),
		Name:        "Test Device",
		Fingerprint: "test-fp",
		Status:      models.DeviceStatusApproved,
		Permissions: models.DevicePermissionsReadOnly,
		DefaultRole: "PLAYER",
	}
	require.NoError(t, resolver.db.Create(&device).Error)

	ctx := createAdminContext()
	result, err := resolver.updateDevicePermissions(ctx, device.ID, generated.DevicePermissionsAdmin)
	require.NoError(t, err)
	assert.Equal(t, string(generated.DevicePermissionsAdmin), result.Permissions)

	// Verify in database
	var dbDevice models.Device
	require.NoError(t, resolver.db.First(&dbDevice, "id = ?", device.ID).Error)
	assert.Equal(t, string(generated.DevicePermissionsAdmin), dbDevice.Permissions)
}

func TestUpdateDevicePermissions_PreservesDefaultUser(t *testing.T) {
	resolver, cleanup := setupDeviceAuthTestResolver(t, true, true)
	defer cleanup()

	// Create a user to be the default user
	userName := "Default User"
	user := models.User{
		ID:    cuid.New(),
		Email: "perms-preserve@test.com",
		Name:  &userName,
		Role:  "USER",
	}
	require.NoError(t, resolver.db.Create(&user).Error)

	// Create an approved device with a default user set
	device := models.Device{
		ID:            cuid.New(),
		Name:          "Device With Default User",
		Fingerprint:   "perms-preserve-fp",
		Status:        models.DeviceStatusApproved,
		IsAuthorized:  true,
		Permissions:   models.DevicePermissionsReadOnly,
		DefaultRole:   "OPERATOR",
		DefaultUserID: &user.ID,
	}
	require.NoError(t, resolver.db.Create(&device).Error)

	ctx := createAdminContext()
	result, err := resolver.updateDevicePermissions(ctx, device.ID, generated.DevicePermissionsAdmin)
	require.NoError(t, err)
	assert.Equal(t, string(generated.DevicePermissionsAdmin), result.Permissions)

	// Verify default_user_id is preserved in database
	var dbDevice models.Device
	require.NoError(t, resolver.db.First(&dbDevice, "id = ?", device.ID).Error)
	assert.Equal(t, string(generated.DevicePermissionsAdmin), dbDevice.Permissions)
	require.NotNil(t, dbDevice.DefaultUserID, "default_user_id should be preserved after permissions update")
	assert.Equal(t, user.ID, *dbDevice.DefaultUserID)
}

func TestUpdateDevicePermissions_NotFound(t *testing.T) {
	resolver, cleanup := setupDeviceAuthTestResolver(t, true, true)
	defer cleanup()
	ctx := createAdminContext()

	_, err := resolver.updateDevicePermissions(ctx, "non-existent-id", generated.DevicePermissionsOperator)
	require.Error(t, err)
	assert.Equal(t, ErrDeviceNotFound, err)
}

func TestUpdateDevicePermissions_NotAuthenticated(t *testing.T) {
	resolver, cleanup := setupDeviceAuthTestResolver(t, true, true)
	defer cleanup()
	ctx := context.Background() // No authentication

	device := models.Device{
		ID:          cuid.New(),
		Name:        "Test Device",
		Fingerprint: "test-fp",
		Status:      models.DeviceStatusApproved,
		Permissions: models.DevicePermissionsReadOnly,
		DefaultRole: "PLAYER",
	}
	require.NoError(t, resolver.db.Create(&device).Error)

	_, err := resolver.updateDevicePermissions(ctx, device.ID, generated.DevicePermissionsOperator)
	require.Error(t, err)
	assert.Equal(t, ErrNotAuthenticated, err)
}

func TestUpdateDevicePermissions_AuthDisabled(t *testing.T) {
	resolver, cleanup := setupDeviceAuthTestResolver(t, false, false)
	defer cleanup()

	device := models.Device{
		ID:          cuid.New(),
		Name:        "Test Device",
		Fingerprint: "test-fp",
		Status:      models.DeviceStatusApproved,
		Permissions: models.DevicePermissionsReadOnly,
		DefaultRole: "PLAYER",
	}
	require.NoError(t, resolver.db.Create(&device).Error)

	ctx := context.Background()
	result, err := resolver.updateDevicePermissions(ctx, device.ID, generated.DevicePermissionsOperator)
	require.NoError(t, err)
	assert.Equal(t, string(generated.DevicePermissionsOperator), result.Permissions)
}

// Tests for approveDevice with defaultUserId

func TestApproveDevice_WithDefaultUser(t *testing.T) {
	resolver, cleanup := setupDeviceAuthTestResolver(t, true, true)
	defer cleanup()

	// Create a user to be the default user
	defaultUserName := "Default User"
	user := models.User{
		ID:    cuid.New(),
		Email: "default@test.com",
		Name:  &defaultUserName,
		Role:  "USER",
	}
	require.NoError(t, resolver.db.Create(&user).Error)

	// Create a pending device
	device := models.Device{
		ID:          cuid.New(),
		Name:        "Test Device",
		Fingerprint: "test-fp-default-user",
		Status:      models.DeviceStatusPending,
		Permissions: models.DevicePermissionsReadOnly,
		DefaultRole: "PLAYER",
	}
	require.NoError(t, resolver.db.Create(&device).Error)

	ctx := createAdminContext()
	result, err := resolver.approveDevice(ctx, device.ID, generated.DevicePermissionsOperator, &user.ID)
	require.NoError(t, err)
	assert.Equal(t, models.DeviceStatusApproved, result.Status)
	assert.NotNil(t, result.DefaultUserID)
	assert.Equal(t, user.ID, *result.DefaultUserID)

	// Verify in database
	var dbDevice models.Device
	require.NoError(t, resolver.db.First(&dbDevice, "id = ?", device.ID).Error)
	assert.NotNil(t, dbDevice.DefaultUserID)
	assert.Equal(t, user.ID, *dbDevice.DefaultUserID)
}

func TestApproveDevice_WithInvalidDefaultUser(t *testing.T) {
	resolver, cleanup := setupDeviceAuthTestResolver(t, true, true)
	defer cleanup()

	device := models.Device{
		ID:          cuid.New(),
		Name:        "Test Device",
		Fingerprint: "test-fp-bad-user",
		Status:      models.DeviceStatusPending,
		Permissions: models.DevicePermissionsReadOnly,
		DefaultRole: "PLAYER",
	}
	require.NoError(t, resolver.db.Create(&device).Error)

	ctx := createAdminContext()
	badUserID := "non-existent-user-id"
	_, err := resolver.approveDevice(ctx, device.ID, generated.DevicePermissionsOperator, &badUserID)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "default user not found")
}

// Tests for updateDevice

func TestUpdateDevice_Name(t *testing.T) {
	resolver, cleanup := setupDeviceAuthTestResolver(t, true, true)
	defer cleanup()

	device := models.Device{
		ID:          cuid.New(),
		Name:        "Original Name",
		Fingerprint: "test-fp-update-name",
		Status:      models.DeviceStatusApproved,
		Permissions: models.DevicePermissionsReadOnly,
		DefaultRole: "PLAYER",
	}
	require.NoError(t, resolver.db.Create(&device).Error)

	ctx := createAdminContext()
	newName := "Updated Name"
	input := generated.UpdateDeviceInput{
		Name: graphql.OmittableOf(&newName),
	}
	result, err := resolver.updateDevice(ctx, device.ID, input)
	require.NoError(t, err)
	assert.Equal(t, "Updated Name", result.Name)

	// Verify in database
	var dbDevice models.Device
	require.NoError(t, resolver.db.First(&dbDevice, "id = ?", device.ID).Error)
	assert.Equal(t, "Updated Name", dbDevice.Name)
}

func TestUpdateDevice_DefaultUserID(t *testing.T) {
	resolver, cleanup := setupDeviceAuthTestResolver(t, true, true)
	defer cleanup()

	// Create a user
	updateTestName := "Update Test User"
	user := models.User{
		ID:    cuid.New(),
		Email: "update-test@test.com",
		Name:  &updateTestName,
		Role:  "USER",
	}
	require.NoError(t, resolver.db.Create(&user).Error)

	device := models.Device{
		ID:          cuid.New(),
		Name:        "Test Device",
		Fingerprint: "test-fp-update-user",
		Status:      models.DeviceStatusApproved,
		Permissions: models.DevicePermissionsOperator,
		DefaultRole: "OPERATOR",
	}
	require.NoError(t, resolver.db.Create(&device).Error)

	ctx := createAdminContext()
	input := generated.UpdateDeviceInput{
		DefaultUserID: graphql.OmittableOf(&user.ID),
	}
	result, err := resolver.updateDevice(ctx, device.ID, input)
	require.NoError(t, err)
	assert.NotNil(t, result.DefaultUserID)
	assert.Equal(t, user.ID, *result.DefaultUserID)

	// Verify in database
	var dbDevice models.Device
	require.NoError(t, resolver.db.First(&dbDevice, "id = ?", device.ID).Error)
	assert.NotNil(t, dbDevice.DefaultUserID)
	assert.Equal(t, user.ID, *dbDevice.DefaultUserID)
}

func TestUpdateDevice_ClearDefaultUserID(t *testing.T) {
	resolver, cleanup := setupDeviceAuthTestResolver(t, true, true)
	defer cleanup()

	// Create a user and device with default user
	clearTestName := "Clear Test User"
	user := models.User{
		ID:    cuid.New(),
		Email: "clear-test@test.com",
		Name:  &clearTestName,
		Role:  "USER",
	}
	require.NoError(t, resolver.db.Create(&user).Error)

	device := models.Device{
		ID:            cuid.New(),
		Name:          "Test Device",
		Fingerprint:   "test-fp-clear-user",
		Status:        models.DeviceStatusApproved,
		Permissions:   models.DevicePermissionsOperator,
		DefaultRole:   "OPERATOR",
		DefaultUserID: &user.ID,
	}
	require.NoError(t, resolver.db.Create(&device).Error)

	ctx := createAdminContext()
	// Set to empty string to clear
	emptyStr := ""
	input := generated.UpdateDeviceInput{
		DefaultUserID: graphql.OmittableOf(&emptyStr),
	}
	result, err := resolver.updateDevice(ctx, device.ID, input)
	require.NoError(t, err)
	assert.Nil(t, result.DefaultUserID)
}

func TestUpdateDevice_InvalidDefaultUserID(t *testing.T) {
	resolver, cleanup := setupDeviceAuthTestResolver(t, true, true)
	defer cleanup()

	device := models.Device{
		ID:          cuid.New(),
		Name:        "Test Device",
		Fingerprint: "test-fp-invalid-user",
		Status:      models.DeviceStatusApproved,
		Permissions: models.DevicePermissionsOperator,
		DefaultRole: "OPERATOR",
	}
	require.NoError(t, resolver.db.Create(&device).Error)

	ctx := createAdminContext()
	badUserID := "non-existent-user"
	input := generated.UpdateDeviceInput{
		DefaultUserID: graphql.OmittableOf(&badUserID),
	}
	_, err := resolver.updateDevice(ctx, device.ID, input)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "default user not found")
}

func TestUpdateDevice_NotFound(t *testing.T) {
	resolver, cleanup := setupDeviceAuthTestResolver(t, true, true)
	defer cleanup()

	ctx := createAdminContext()
	input := generated.UpdateDeviceInput{}
	_, err := resolver.updateDevice(ctx, "non-existent-id", input)
	require.Error(t, err)
	assert.Equal(t, ErrDeviceNotFound, err)
}

func TestUpdateDevice_NotAuthenticated(t *testing.T) {
	resolver, cleanup := setupDeviceAuthTestResolver(t, true, true)
	defer cleanup()

	device := models.Device{
		ID:          cuid.New(),
		Name:        "Test Device",
		Fingerprint: "test-fp-no-auth",
		Status:      models.DeviceStatusApproved,
		Permissions: models.DevicePermissionsReadOnly,
		DefaultRole: "PLAYER",
	}
	require.NoError(t, resolver.db.Create(&device).Error)

	ctx := context.Background()
	input := generated.UpdateDeviceInput{}
	_, err := resolver.updateDevice(ctx, device.ID, input)
	require.Error(t, err)
	assert.Equal(t, ErrNotAuthenticated, err)
}

func TestUpdateDevice_Permissions(t *testing.T) {
	resolver, cleanup := setupDeviceAuthTestResolver(t, true, true)
	defer cleanup()

	device := models.Device{
		ID:          cuid.New(),
		Name:        "Test Device",
		Fingerprint: "test-fp-update-perms",
		Status:      models.DeviceStatusApproved,
		Permissions: models.DevicePermissionsReadOnly,
		DefaultRole: "PLAYER",
	}
	require.NoError(t, resolver.db.Create(&device).Error)

	ctx := createAdminContext()
	newPerms := generated.DevicePermissionsAdmin
	input := generated.UpdateDeviceInput{
		Permissions: graphql.OmittableOf(&newPerms),
	}
	result, err := resolver.updateDevice(ctx, device.ID, input)
	require.NoError(t, err)
	assert.Equal(t, string(generated.DevicePermissionsAdmin), result.Permissions)
}

func TestUpdateDevice_IsAuthorized_SyncsStatus(t *testing.T) {
	resolver, cleanup := setupDeviceAuthTestResolver(t, true, true)
	defer cleanup()

	// Create an approved device
	device := models.Device{
		ID:           cuid.New(),
		Name:         "Test Device",
		Fingerprint:  "test-fp-is-authorized",
		Status:       models.DeviceStatusApproved,
		IsAuthorized: true,
		Permissions:  models.DevicePermissionsOperator,
		DefaultRole:  "OPERATOR",
	}
	require.NoError(t, resolver.db.Create(&device).Error)

	ctx := createAdminContext()

	// Test: Setting isAuthorized=false should also set status=REVOKED
	falseVal := false
	input := generated.UpdateDeviceInput{
		IsAuthorized: graphql.OmittableOf(&falseVal),
	}
	result, err := resolver.updateDevice(ctx, device.ID, input)
	require.NoError(t, err)
	assert.False(t, result.IsAuthorized)
	assert.Equal(t, models.DeviceStatusRevoked, result.Status)

	// Verify in database
	var dbDevice models.Device
	require.NoError(t, resolver.db.First(&dbDevice, "id = ?", device.ID).Error)
	assert.False(t, dbDevice.IsAuthorized)
	assert.Equal(t, models.DeviceStatusRevoked, dbDevice.Status)

	// Test: Setting isAuthorized=true should also set status=APPROVED and populate approval metadata
	trueVal := true
	input2 := generated.UpdateDeviceInput{
		IsAuthorized: graphql.OmittableOf(&trueVal),
	}
	result2, err := resolver.updateDevice(ctx, device.ID, input2)
	require.NoError(t, err)
	assert.True(t, result2.IsAuthorized)
	assert.Equal(t, models.DeviceStatusApproved, result2.Status)

	// Verify in database, including approval metadata
	var dbDevice2 models.Device
	require.NoError(t, resolver.db.First(&dbDevice2, "id = ?", device.ID).Error)
	assert.True(t, dbDevice2.IsAuthorized)
	assert.Equal(t, models.DeviceStatusApproved, dbDevice2.Status)
	assert.NotNil(t, dbDevice2.ApprovedAt, "approved_at should be set when transitioning to authorized")
	assert.NotNil(t, dbDevice2.ApprovedByID, "approved_by should be set when admin transitions to authorized")
	assert.Equal(t, "admin-user-id", *dbDevice2.ApprovedByID)
}

func TestUpdateDevice_EmptyInput_NoOp(t *testing.T) {
	resolver, cleanup := setupDeviceAuthTestResolver(t, true, true)
	defer cleanup()

	device := models.Device{
		ID:          cuid.New(),
		Name:        "Original Name",
		Fingerprint: "test-fp-empty-input",
		Status:      models.DeviceStatusApproved,
		Permissions: models.DevicePermissionsOperator,
		DefaultRole: "OPERATOR",
	}
	require.NoError(t, resolver.db.Create(&device).Error)

	ctx := createAdminContext()
	// Empty input should be a no-op: return the device unchanged without error
	input := generated.UpdateDeviceInput{}
	result, err := resolver.updateDevice(ctx, device.ID, input)
	require.NoError(t, err)
	assert.Equal(t, "Original Name", result.Name)
	assert.Equal(t, models.DeviceStatusApproved, result.Status)
	assert.Equal(t, string(generated.DevicePermissionsOperator), result.Permissions)
}
