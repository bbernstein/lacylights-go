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
	result, err := resolver.approveDevice(ctx, device.ID, generated.DevicePermissionsOperator)
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

	_, err := resolver.approveDevice(ctx, "non-existent-id", generated.DevicePermissionsReadOnly)
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

	_, err := resolver.approveDevice(ctx, device.ID, generated.DevicePermissionsOperator)
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
	result, err := resolver.approveDevice(ctx, device.ID, generated.DevicePermissionsAdmin)
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
