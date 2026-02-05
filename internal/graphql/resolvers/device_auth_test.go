package resolvers

import (
	"context"
	"testing"
	"time"

	"github.com/lucsky/cuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/bbernstein/lacylights-go/internal/auth"
	"github.com/bbernstein/lacylights-go/internal/database/models"
	"github.com/bbernstein/lacylights-go/internal/graphql/generated"
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
