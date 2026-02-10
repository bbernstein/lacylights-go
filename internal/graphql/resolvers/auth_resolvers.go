package resolvers

// This file contains implementations for auth-related GraphQL resolvers.
// The actual resolver stubs are generated in schema.resolvers.go by gqlgen.
// These helper functions are called from those stubs.

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/lucsky/cuid"
	"gorm.io/gorm"

	"github.com/bbernstein/lacylights-go/internal/auth"
	"github.com/bbernstein/lacylights-go/internal/database/models"
	"github.com/bbernstein/lacylights-go/internal/graphql/generated"
	"github.com/bbernstein/lacylights-go/internal/middleware"
)

// Auth errors for GraphQL responses.
var (
	ErrAuthNotEnabled    = errors.New("authentication is not enabled")
	ErrNotAuthenticated  = errors.New("not authenticated")
	ErrNotAuthorized     = errors.New("not authorized")
	ErrUserNotFound      = errors.New("user not found")
	ErrGroupNotFound     = errors.New("group not found")
	ErrDeviceNotFound    = errors.New("device not found")
	ErrSessionNotFound   = errors.New("session not found")
	ErrFeatureNotEnabled = errors.New("this feature is not enabled")
)

// registerUser handles user registration.
func (r *Resolver) registerUser(ctx context.Context, input generated.RegisterInput) (*generated.AuthPayload, error) {
	if r.AuthService == nil || !r.AuthService.IsEnabled() {
		return nil, ErrAuthNotEnabled
	}

	// Get client info from headers (would be extracted by middleware in production)
	var ipAddress, userAgent *string
	// These would typically come from the request context

	// Handle optional name field
	var name *string
	if input.Name.IsSet() {
		name = input.Name.Value()
	}

	result, err := r.AuthService.Register(ctx, auth.RegisterInput{
		Email:    input.Email,
		Password: input.Password,
		Name:     name,
	}, ipAddress, userAgent)
	if err != nil {
		return nil, err
	}

	return &generated.AuthPayload{
		User:         *result.User,
		AccessToken:  result.AccessToken,
		RefreshToken: result.RefreshToken,
		ExpiresAt:    result.ExpiresAt.UTC().Format("2006-01-02T15:04:05.000Z"),
	}, nil
}

// loginUser handles user login.
func (r *Resolver) loginUser(ctx context.Context, email, password string) (*generated.AuthPayload, error) {
	if r.AuthService == nil || !r.AuthService.IsEnabled() {
		return nil, ErrAuthNotEnabled
	}

	var ipAddress, userAgent *string

	result, err := r.AuthService.Login(ctx, email, password, ipAddress, userAgent)
	if err != nil {
		return nil, err
	}

	return &generated.AuthPayload{
		User:         *result.User,
		AccessToken:  result.AccessToken,
		RefreshToken: result.RefreshToken,
		ExpiresAt:    result.ExpiresAt.UTC().Format("2006-01-02T15:04:05.000Z"),
	}, nil
}

// logoutUser handles user logout.
func (r *Resolver) logoutUser(ctx context.Context) (bool, error) {
	if r.AuthService == nil || !r.AuthService.IsEnabled() {
		return true, nil // No-op if auth is disabled
	}

	sess := middleware.GetSessionFromContext(ctx)
	if sess == nil {
		return false, ErrNotAuthenticated
	}

	if err := r.AuthService.Logout(ctx, sess.SessionID); err != nil {
		return false, err
	}

	return true, nil
}

// logoutAllSessions handles logging out all sessions for the current user.
func (r *Resolver) logoutAllSessions(ctx context.Context) (bool, error) {
	if r.AuthService == nil || !r.AuthService.IsEnabled() {
		return true, nil
	}

	sess := middleware.GetSessionFromContext(ctx)
	if sess == nil {
		return false, ErrNotAuthenticated
	}

	if err := r.AuthService.LogoutAll(ctx, sess.UserID); err != nil {
		return false, err
	}

	return true, nil
}

// refreshUserToken handles token refresh.
func (r *Resolver) refreshUserToken(ctx context.Context, refreshToken string) (*generated.AuthPayload, error) {
	if r.AuthService == nil || !r.AuthService.IsEnabled() {
		return nil, ErrAuthNotEnabled
	}

	result, err := r.AuthService.RefreshToken(ctx, refreshToken)
	if err != nil {
		return nil, err
	}

	return &generated.AuthPayload{
		User:         *result.User,
		AccessToken:  result.AccessToken,
		RefreshToken: result.RefreshToken,
		ExpiresAt:    result.ExpiresAt.UTC().Format("2006-01-02T15:04:05.000Z"),
	}, nil
}

// changeUserPassword handles password changes.
func (r *Resolver) changeUserPassword(ctx context.Context, oldPassword, newPassword string) (bool, error) {
	if r.AuthService == nil || !r.AuthService.IsEnabled() {
		return false, ErrAuthNotEnabled
	}

	sess := middleware.GetSessionFromContext(ctx)
	if sess == nil {
		return false, ErrNotAuthenticated
	}

	if err := r.AuthService.ChangePassword(ctx, sess.UserID, oldPassword, newPassword); err != nil {
		return false, err
	}

	return true, nil
}

// getCurrentUser returns the authenticated user's info.
func (r *Resolver) getCurrentUser(ctx context.Context) (*generated.AuthUser, error) {
	if r.AuthService == nil || !r.AuthService.IsEnabled() {
		return nil, ErrAuthNotEnabled
	}

	sess := middleware.GetSessionFromContext(ctx)
	if sess == nil {
		return nil, nil // Return nil (not an error) if not authenticated
	}

	user, err := r.AuthService.GetUserByID(ctx, sess.UserID)
	if err != nil {
		return nil, err
	}

	return r.toAuthUser(user), nil
}

// isAuthEnabled returns whether auth is enabled.
func (r *Resolver) isAuthEnabled(_ context.Context) bool {
	return r.AuthService != nil && r.AuthService.IsEnabled()
}

// getAuthSettings returns current auth settings.
func (r *Resolver) getAuthSettings(ctx context.Context) (*generated.AuthSettings, error) {
	// Get settings from database
	var settings []models.AuthSetting
	if err := r.db.WithContext(ctx).Find(&settings).Error; err != nil {
		return nil, err
	}

	// Build settings map
	settingsMap := make(map[string]string)
	for _, s := range settings {
		settingsMap[s.Key] = s.Value
	}

	// Return auth settings with defaults
	authEnabled := r.AuthService != nil && r.AuthService.IsEnabled()
	deviceAuthEnabled := r.AuthService != nil && r.AuthService.IsDeviceAuthEnabled()

	return &generated.AuthSettings{
		AuthEnabled:              authEnabled,
		AllowedMethods:           []string{"password"},
		DeviceAuthEnabled:        deviceAuthEnabled,
		SessionDurationHours:     24,
		PasswordMinLength:        8,
		RequireEmailVerification: false,
	}, nil
}

// getUserSessions returns the current user's sessions.
func (r *Resolver) getUserSessions(ctx context.Context) ([]*models.Session, error) {
	if r.AuthService == nil || !r.AuthService.IsEnabled() {
		return nil, ErrAuthNotEnabled
	}

	sess := middleware.GetSessionFromContext(ctx)
	if sess == nil {
		return nil, ErrNotAuthenticated
	}

	sessions, err := r.AuthService.GetUserSessions(ctx, sess.UserID)
	if err != nil {
		return nil, err
	}

	// Convert to pointers
	result := make([]*models.Session, len(sessions))
	for i := range sessions {
		result[i] = &sessions[i]
	}
	return result, nil
}

// Admin operations

// getUsers returns all users (admin only).
func (r *Resolver) getUsers(ctx context.Context, page, perPage *int) ([]*models.User, error) {
	if err := r.requireAdmin(ctx); err != nil {
		return nil, err
	}

	// Default pagination with validation
	pageNum := 1
	pageSize := 50
	if page != nil && *page > 0 {
		pageNum = *page
	}
	if perPage != nil {
		if *perPage < 1 {
			pageSize = 1
		} else if *perPage > 100 {
			pageSize = 100
		} else {
			pageSize = *perPage
		}
	}

	var users []models.User
	offset := (pageNum - 1) * pageSize
	if err := r.db.WithContext(ctx).Offset(offset).Limit(pageSize).Find(&users).Error; err != nil {
		return nil, err
	}

	result := make([]*models.User, len(users))
	for i := range users {
		result[i] = &users[i]
	}
	return result, nil
}

// getUser returns a user by ID (admin only).
func (r *Resolver) getUser(ctx context.Context, id string) (*models.User, error) {
	if err := r.requireAdmin(ctx); err != nil {
		return nil, err
	}

	var user models.User
	if err := r.db.WithContext(ctx).First(&user, "id = ?", id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrUserNotFound
		}
		return nil, err
	}
	return &user, nil
}

// createUser creates a new user (admin only).
func (r *Resolver) createUser(ctx context.Context, input generated.CreateUserInput) (*models.User, error) {
	if err := r.requireAdmin(ctx); err != nil {
		return nil, err
	}

	var user *models.User

	// Use transaction to ensure atomicity of user and credentials creation
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		user = &models.User{
			ID:       cuid.New(),
			Email:    input.Email,
			Role:     "USER",
			IsActive: true,
		}

		if input.Name.IsSet() && input.Name.Value() != nil {
			user.Name = input.Name.Value()
		}
		if input.Role.IsSet() && input.Role.Value() != nil {
			user.Role = string(*input.Role.Value())
		}

		if err := tx.Create(user).Error; err != nil {
			return err
		}

		// Create credentials with the password
		passwordHash, err := auth.HashPassword(input.Password)
		if err != nil {
			return err
		}
		creds := &models.UserCredential{
			ID:           cuid.New(),
			UserID:       user.ID,
			PasswordHash: passwordHash,
		}
		if err := tx.Create(creds).Error; err != nil {
			return err
		}

		return nil
	})
	if err != nil {
		return nil, err
	}

	return user, nil
}

// updateUser updates a user (admin only).
func (r *Resolver) updateUser(ctx context.Context, id string, input generated.UpdateUserInput) (*models.User, error) {
	if err := r.requireAdmin(ctx); err != nil {
		return nil, err
	}

	var user models.User
	if err := r.db.WithContext(ctx).First(&user, "id = ?", id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrUserNotFound
		}
		return nil, err
	}

	if input.Email.IsSet() && input.Email.Value() != nil {
		user.Email = *input.Email.Value()
	}
	if input.Name.IsSet() {
		user.Name = input.Name.Value()
	}
	if input.Role.IsSet() && input.Role.Value() != nil {
		user.Role = string(*input.Role.Value())
	}
	if input.IsActive.IsSet() && input.IsActive.Value() != nil {
		user.IsActive = *input.IsActive.Value()
	}

	if err := r.db.WithContext(ctx).Save(&user).Error; err != nil {
		return nil, err
	}

	return &user, nil
}

// deleteUser deletes a user (admin only).
func (r *Resolver) deleteUser(ctx context.Context, id string) (bool, error) {
	if err := r.requireAdmin(ctx); err != nil {
		return false, err
	}

	result := r.db.WithContext(ctx).Delete(&models.User{}, "id = ?", id)
	if result.Error != nil {
		return false, result.Error
	}
	return result.RowsAffected > 0, nil
}

// Group operations

// getUserGroups returns all groups (admin only).
func (r *Resolver) getUserGroups(ctx context.Context) ([]*models.UserGroup, error) {
	if err := r.requireAdmin(ctx); err != nil {
		return nil, err
	}

	var groups []models.UserGroup
	if err := r.db.WithContext(ctx).Find(&groups).Error; err != nil {
		return nil, err
	}

	result := make([]*models.UserGroup, len(groups))
	for i := range groups {
		result[i] = &groups[i]
	}
	return result, nil
}

// getUserGroup returns a group by ID (admin or group member).
func (r *Resolver) getUserGroup(ctx context.Context, id string) (*models.UserGroup, error) {
	if err := r.requireGroupAccess(ctx, id); err != nil {
		return nil, err
	}

	var group models.UserGroup
	if err := r.db.WithContext(ctx).First(&group, "id = ?", id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrGroupNotFound
		}
		return nil, err
	}
	return &group, nil
}

// createUserGroup creates a new group (admin only).
func (r *Resolver) createUserGroup(ctx context.Context, input generated.CreateUserGroupInput) (*models.UserGroup, error) {
	if err := r.requireAdmin(ctx); err != nil {
		return nil, err
	}

	group := &models.UserGroup{
		ID:   cuid.New(),
		Name: input.Name,
	}

	if input.Description.IsSet() && input.Description.Value() != nil {
		group.Description = input.Description.Value()
	}
	if input.Permissions.IsSet() {
		perms := input.Permissions.Value()
		// Store permissions as JSON array for consistent parsing
		if len(perms) > 0 {
			permBytes, err := json.Marshal(perms)
			if err != nil {
				return nil, fmt.Errorf("failed to encode permissions: %w", err)
			}
			permStr := string(permBytes)
			group.Permissions = &permStr
		}
	}

	// Create group and optionally add creator as GROUP_ADMIN member atomically
	userID := middleware.GetUserIDFromContext(ctx)

	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(group).Error; err != nil {
			return err
		}
		// When auth is enabled, add the creator as a member.
		// When auth is disabled (no user in context), skip membership creation.
		if userID != "" {
			member := models.UserGroupMember{
				ID:      cuid.New(),
				UserID:  userID,
				GroupID: group.ID,
				Role:    models.GroupRoleGroupAdmin,
			}
			if err := tx.Create(&member).Error; err != nil {
				return fmt.Errorf("failed to add creator as group member: %w", err)
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}

	return group, nil
}

// updateUserGroup updates a group (group admin or system admin).
func (r *Resolver) updateUserGroup(ctx context.Context, id string, input generated.UpdateUserGroupInput) (*models.UserGroup, error) {
	if err := r.requireGroupAdmin(ctx, id); err != nil {
		return nil, err
	}

	var group models.UserGroup
	if err := r.db.WithContext(ctx).First(&group, "id = ?", id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrGroupNotFound
		}
		return nil, err
	}

	if input.Name.IsSet() && input.Name.Value() != nil {
		group.Name = *input.Name.Value()
	}
	if input.Description.IsSet() {
		group.Description = input.Description.Value()
	}
	if input.Permissions.IsSet() {
		perms := input.Permissions.Value()
		// Store permissions as JSON array for consistent parsing
		if len(perms) > 0 {
			permBytes, err := json.Marshal(perms)
			if err != nil {
				return nil, fmt.Errorf("failed to encode permissions: %w", err)
			}
			permStr := string(permBytes)
			group.Permissions = &permStr
		} else {
			group.Permissions = nil
		}
	}

	if err := r.db.WithContext(ctx).Save(&group).Error; err != nil {
		return nil, err
	}

	return &group, nil
}

// deleteUserGroup deletes a group (admin only).
func (r *Resolver) deleteUserGroup(ctx context.Context, id string) (bool, error) {
	if err := r.requireAdmin(ctx); err != nil {
		return false, err
	}

	result := r.db.WithContext(ctx).Delete(&models.UserGroup{}, "id = ?", id)
	if result.Error != nil {
		return false, result.Error
	}
	return result.RowsAffected > 0, nil
}

// removeUserFromGroup removes a user from a group (group admin or system admin).
func (r *Resolver) removeUserFromGroup(ctx context.Context, userID, groupID string) (bool, error) {
	if err := r.requireGroupAdmin(ctx, groupID); err != nil {
		return false, err
	}

	result := r.db.WithContext(ctx).Delete(&models.UserGroupMember{}, "user_id = ? AND group_id = ?", userID, groupID)
	if result.Error != nil {
		return false, result.Error
	}
	return result.RowsAffected > 0, nil
}

// Device operations

// getDevicesByStatus returns devices filtered by status (admin only).
func (r *Resolver) getDevicesByStatus(ctx context.Context, status *generated.DeviceStatus) ([]*models.Device, error) {
	if err := r.requireAdmin(ctx); err != nil {
		return nil, err
	}

	var devices []models.Device
	query := r.db.WithContext(ctx)
	if status != nil {
		query = query.Where("status = ?", string(*status))
	}
	if err := query.Find(&devices).Error; err != nil {
		return nil, err
	}

	result := make([]*models.Device, len(devices))
	for i := range devices {
		result[i] = &devices[i]
	}
	return result, nil
}

// getPendingDevices returns all devices with PENDING status (admin only).
func (r *Resolver) getPendingDevices(ctx context.Context) ([]*models.Device, error) {
	if err := r.requireAdmin(ctx); err != nil {
		return nil, err
	}

	var devices []models.Device
	if err := r.db.WithContext(ctx).Where("status = ?", models.DeviceStatusPending).Find(&devices).Error; err != nil {
		return nil, err
	}

	result := make([]*models.Device, len(devices))
	for i := range devices {
		result[i] = &devices[i]
	}
	return result, nil
}

// getDevice returns a device by ID (admin only).
func (r *Resolver) getDevice(ctx context.Context, id string) (*models.Device, error) {
	if err := r.requireAdmin(ctx); err != nil {
		return nil, err
	}

	var device models.Device
	if err := r.db.WithContext(ctx).First(&device, "id = ?", id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrDeviceNotFound
		}
		return nil, err
	}
	return &device, nil
}

// checkDeviceAuth checks if a device fingerprint is authorized (legacy).
func (r *Resolver) checkDeviceAuth(ctx context.Context, fingerprint string) (*generated.DeviceAuthStatus, error) {
	if r.AuthService == nil || !r.AuthService.IsDeviceAuthEnabled() {
		return &generated.DeviceAuthStatus{
			IsAuthorized: false,
			IsPending:    false,
			Device:       nil,
			DefaultUser:  nil,
		}, nil
	}

	var device models.Device
	err := r.db.WithContext(ctx).Where("fingerprint = ?", fingerprint).First(&device).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return &generated.DeviceAuthStatus{
				IsAuthorized: false,
				IsPending:    false,
				Device:       nil,
				DefaultUser:  nil,
			}, nil
		}
		return nil, err
	}

	isAuthorized := device.Status == models.DeviceStatusApproved
	isPending := device.Status == models.DeviceStatusPending

	if !isAuthorized {
		return &generated.DeviceAuthStatus{
			IsAuthorized: false,
			IsPending:    isPending,
			Device:       &device,
			DefaultUser:  nil,
		}, nil
	}

	// Get default user if set
	var defaultUser *models.User
	if device.DefaultUserID != nil {
		var user models.User
		if err := r.db.WithContext(ctx).First(&user, "id = ?", *device.DefaultUserID).Error; err == nil {
			defaultUser = &user
		}
	}

	return &generated.DeviceAuthStatus{
		IsAuthorized: true,
		IsPending:    false,
		Device:       &device,
		DefaultUser:  defaultUser,
	}, nil
}

// checkDevice checks a device's status by fingerprint (unauthenticated, for registration flow).
// This is used by clients (MCP server, frontend) to determine if device registration is needed.
func (r *Resolver) checkDevice(ctx context.Context, fingerprint string) (*generated.DeviceCheckResult, error) {
	// If auth is not enabled at all, device checks are bypassed.
	// NOTE: We return APPROVED status here to indicate that no device registration
	// or approval is required because authentication is globally disabled. This does NOT
	// mean that a specific device record has been approved; in this case, Device is nil
	// and callers should interpret this as "device checks not applicable".
	if r.AuthService == nil || !r.AuthService.IsEnabled() {
		return &generated.DeviceCheckResult{
			Status:  generated.DeviceStatusApproved,
			Device:  nil,
			Message: stringPtr("Authentication is disabled - device checks are bypassed and all devices have access"),
		}, nil
	}

	// If device authentication is disabled, we also bypass per-device checks.
	// NOTE: We return APPROVED status to indicate that no device registration
	// flow is required and access should be controlled via standard authentication
	// only. Device will be nil to indicate that no device-specific record was checked.
	if !r.AuthService.IsDeviceAuthEnabled() {
		return &generated.DeviceCheckResult{
			Status:  generated.DeviceStatusApproved,
			Device:  nil,
			Message: stringPtr("Device authentication is disabled - device checks are bypassed; use standard authentication"),
		}, nil
	}

	var device models.Device
	err := r.db.WithContext(ctx).Where("fingerprint = ?", fingerprint).First(&device).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			// Device not found - client should register
			return &generated.DeviceCheckResult{
				Status:  generated.DeviceStatusPending, // Return PENDING to indicate registration needed
				Device:  nil,
				Message: stringPtr("Device not registered - please register this device"),
			}, nil
		}
		return nil, err
	}

	// Map database status to GraphQL enum
	var status generated.DeviceStatus
	var message string
	switch device.Status {
	case models.DeviceStatusApproved:
		status = generated.DeviceStatusApproved
		message = "Device is approved"
	case models.DeviceStatusPending:
		status = generated.DeviceStatusPending
		message = "Device is pending approval"
	case models.DeviceStatusRevoked:
		status = generated.DeviceStatusRevoked
		message = "Device access has been revoked"
	default:
		status = generated.DeviceStatusPending
		message = "Unknown device status"
	}

	return &generated.DeviceCheckResult{
		Status:  status,
		Device:  &device,
		Message: stringPtr(message),
	}, nil
}

// registerDevice registers a new device for device-based auth (unauthenticated).
func (r *Resolver) registerDevice(ctx context.Context, fingerprint string, name string) (*generated.DeviceRegistrationResult, error) {
	// If auth or device auth is not enabled, return success (no registration needed)
	if r.AuthService == nil || !r.AuthService.IsEnabled() {
		return &generated.DeviceRegistrationResult{
			Success: true,
			Device:  nil,
			Message: "Authentication is disabled - device registration not required",
		}, nil
	}

	if !r.AuthService.IsDeviceAuthEnabled() {
		return &generated.DeviceRegistrationResult{
			Success: false,
			Device:  nil,
			Message: "Device authentication is not enabled",
		}, nil
	}

	// Validate input
	if fingerprint == "" {
		return &generated.DeviceRegistrationResult{
			Success: false,
			Device:  nil,
			Message: "Fingerprint is required",
		}, nil
	}
	if name == "" {
		return &generated.DeviceRegistrationResult{
			Success: false,
			Device:  nil,
			Message: "Device name is required",
		}, nil
	}

	// Check if device already exists
	var existingDevice models.Device
	err := r.db.WithContext(ctx).Where("fingerprint = ?", fingerprint).First(&existingDevice).Error
	if err == nil {
		// Device already exists
		return &generated.DeviceRegistrationResult{
			Success: true,
			Device:  &existingDevice,
			Message: fmt.Sprintf("Device already registered with status: %s", existingDevice.Status),
		}, nil
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, err
	}

	// Create new device with PENDING status
	device := models.Device{
		ID:          cuid.New(),
		Name:        name,
		Fingerprint: fingerprint,
		Status:      models.DeviceStatusPending,
		Permissions: models.DevicePermissionsReadOnly,
		DefaultRole: "PLAYER",
	}

	if err := r.db.WithContext(ctx).Create(&device).Error; err != nil {
		return nil, fmt.Errorf("failed to register device: %w", err)
	}

	return &generated.DeviceRegistrationResult{
		Success: true,
		Device:  &device,
		Message: "Device registered successfully. Please wait for admin approval.",
	}, nil
}

// approveDevice approves a pending device (admin only).
// If defaultUserID is provided, it is set as the device's default user for device-only auth.
// Uses column-level updates to avoid GORM nulling foreign keys when relation objects aren't loaded.
func (r *Resolver) approveDevice(ctx context.Context, deviceID string, permissions generated.DevicePermissions, defaultUserID *string) (*models.Device, error) {
	if err := r.requireAdmin(ctx); err != nil {
		return nil, err
	}

	var device models.Device
	if err := r.db.WithContext(ctx).First(&device, "id = ?", deviceID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrDeviceNotFound
		}
		return nil, err
	}

	// Build column-level updates
	now := time.Now()
	updates := map[string]interface{}{
		"status":        models.DeviceStatusApproved,
		"is_authorized": true,
		"permissions":   string(permissions),
		"approved_at":   now,
	}

	// Get the approving user's ID from context if available
	userID := middleware.GetUserIDFromContext(ctx)
	if userID != "" {
		updates["approved_by"] = userID
	}

	// If a default user ID is provided, validate it exists
	if defaultUserID != nil && *defaultUserID != "" {
		var user models.User
		if err := r.db.WithContext(ctx).First(&user, "id = ?", *defaultUserID).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return nil, fmt.Errorf("default user not found: %s", *defaultUserID)
			}
			return nil, fmt.Errorf("failed to verify default user: %w", err)
		}
		updates["default_user_id"] = *defaultUserID
	}

	if err := r.db.WithContext(ctx).Model(&device).Updates(updates).Error; err != nil {
		return nil, err
	}

	// Reload with relations to return complete data
	if err := r.db.WithContext(ctx).
		Preload("DefaultUser").
		Preload("ApprovedBy").
		First(&device, "id = ?", deviceID).Error; err != nil {
		return nil, err
	}

	return &device, nil
}

// updateDevice updates a device's fields (admin only).
// Uses column-level updates to avoid GORM nulling foreign keys when relation objects aren't loaded.
func (r *Resolver) updateDevice(ctx context.Context, id string, input generated.UpdateDeviceInput) (*models.Device, error) {
	if err := r.requireAdmin(ctx); err != nil {
		return nil, err
	}

	var device models.Device
	if err := r.db.WithContext(ctx).First(&device, "id = ?", id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrDeviceNotFound
		}
		return nil, err
	}

	updates := map[string]interface{}{}

	if input.Name.IsSet() && input.Name.Value() != nil {
		updates["name"] = *input.Name.Value()
	}

	if input.DefaultUserID.IsSet() {
		if input.DefaultUserID.Value() != nil && *input.DefaultUserID.Value() != "" {
			// Validate that the user exists
			var user models.User
			if err := r.db.WithContext(ctx).First(&user, "id = ?", *input.DefaultUserID.Value()).Error; err != nil {
				if errors.Is(err, gorm.ErrRecordNotFound) {
					return nil, fmt.Errorf("default user not found: %s", *input.DefaultUserID.Value())
				}
				return nil, fmt.Errorf("failed to verify default user: %w", err)
			}
			updates["default_user_id"] = *input.DefaultUserID.Value()
		} else {
			// Explicitly set to null to clear the default user
			updates["default_user_id"] = nil
		}
	}

	if input.DefaultRole.IsSet() && input.DefaultRole.Value() != nil {
		updates["default_role"] = string(*input.DefaultRole.Value())
	}

	if input.IsAuthorized.IsSet() && input.IsAuthorized.Value() != nil {
		isAuthorized := *input.IsAuthorized.Value()
		updates["is_authorized"] = isAuthorized
		// Keep status in sync with is_authorized to preserve the invariant
		// that is_authorized == (status == APPROVED).
		if isAuthorized {
			updates["status"] = models.DeviceStatusApproved
			// Populate approval metadata when transitioning to authorized,
			// consistent with approveDevice and maintaining an audit trail.
			if !device.IsAuthorized {
				updates["approved_at"] = time.Now()
				userID := middleware.GetUserIDFromContext(ctx)
				if userID != "" {
					updates["approved_by"] = userID
				}
			}
		} else {
			updates["status"] = models.DeviceStatusRevoked
		}
	}

	if input.Permissions.IsSet() && input.Permissions.Value() != nil {
		updates["permissions"] = string(*input.Permissions.Value())
	}

	if len(updates) == 0 {
		// No updates to apply, reload with relations and return current device
		if err := r.db.WithContext(ctx).Preload("DefaultUser").First(&device, "id = ?", id).Error; err != nil {
			return nil, err
		}
		return &device, nil
	}

	if err := r.db.WithContext(ctx).Model(&device).Updates(updates).Error; err != nil {
		return nil, err
	}

	// Reload with relations to return complete data
	if err := r.db.WithContext(ctx).Preload("DefaultUser").First(&device, "id = ?", id).Error; err != nil {
		return nil, err
	}

	return &device, nil
}

// revokeDeviceAuth revokes a device's authorization (admin only).
// Uses column-level updates to avoid GORM nulling foreign keys when relation objects aren't loaded.
func (r *Resolver) revokeDeviceAuth(ctx context.Context, deviceID string) (*models.Device, error) {
	if err := r.requireAdmin(ctx); err != nil {
		return nil, err
	}

	var device models.Device
	if err := r.db.WithContext(ctx).First(&device, "id = ?", deviceID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrDeviceNotFound
		}
		return nil, err
	}

	// Use column-level updates to preserve foreign keys (e.g. default_user_id)
	updates := map[string]interface{}{
		"status":        models.DeviceStatusRevoked,
		"is_authorized": false,
	}

	if err := r.db.WithContext(ctx).Model(&device).Updates(updates).Error; err != nil {
		return nil, err
	}

	// Reload with relations to return complete data
	if err := r.db.WithContext(ctx).Preload("DefaultUser").First(&device, "id = ?", deviceID).Error; err != nil {
		return nil, err
	}

	return &device, nil
}

// updateDevicePermissions updates a device's permissions (admin only).
// Uses column-level updates to avoid GORM nulling foreign keys when relation objects aren't loaded.
func (r *Resolver) updateDevicePermissions(ctx context.Context, deviceID string, permissions generated.DevicePermissions) (*models.Device, error) {
	if err := r.requireAdmin(ctx); err != nil {
		return nil, err
	}

	var device models.Device
	if err := r.db.WithContext(ctx).First(&device, "id = ?", deviceID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrDeviceNotFound
		}
		return nil, err
	}

	updates := map[string]interface{}{
		"permissions": string(permissions),
	}

	if err := r.db.WithContext(ctx).Model(&device).Updates(updates).Error; err != nil {
		return nil, err
	}

	// Reload with relations to return complete data
	if err := r.db.WithContext(ctx).Preload("DefaultUser").First(&device, "id = ?", deviceID).Error; err != nil {
		return nil, err
	}

	return &device, nil
}

// Session management

// revokeSession revokes a specific session (admin only).
func (r *Resolver) revokeSession(ctx context.Context, sessionID string) (bool, error) {
	if err := r.requireAdmin(ctx); err != nil {
		return false, err
	}

	if r.AuthService == nil {
		return false, ErrAuthNotEnabled
	}

	if err := r.AuthService.Logout(ctx, sessionID); err != nil {
		return false, err
	}

	return true, nil
}

// revokeAllUserSessions revokes all sessions for a user (admin only).
func (r *Resolver) revokeAllUserSessions(ctx context.Context, userID string) (bool, error) {
	if err := r.requireAdmin(ctx); err != nil {
		return false, err
	}

	if r.AuthService == nil {
		return false, ErrAuthNotEnabled
	}

	if err := r.AuthService.LogoutAll(ctx, userID); err != nil {
		return false, err
	}

	return true, nil
}

// Helper functions

// requireAdmin checks if the current user is an admin.
func (r *Resolver) requireAdmin(ctx context.Context) error {
	if r.AuthService == nil || !r.AuthService.IsEnabled() {
		// If auth is disabled, allow all operations
		return nil
	}

	if !middleware.IsAuthenticated(ctx) {
		return ErrNotAuthenticated
	}

	if !middleware.IsAdmin(ctx) {
		return ErrNotAuthorized
	}

	return nil
}

// requireGroupAccess checks if the user has access to a group.
// Returns nil if auth is disabled (all access allowed).
func (r *Resolver) requireGroupAccess(ctx context.Context, groupID string) error {
	if r.AuthService == nil || !r.AuthService.IsEnabled() {
		return nil
	}

	if !middleware.IsAuthenticated(ctx) {
		return ErrNotAuthenticated
	}

	// System admins have access to all groups
	if middleware.IsAdmin(ctx) {
		return nil
	}

	if !middleware.IsGroupMember(ctx, groupID) {
		return ErrNotAuthorized
	}

	return nil
}

// requireGroupAdmin checks if the user is a group admin or system admin.
// Returns nil if auth is disabled.
func (r *Resolver) requireGroupAdmin(ctx context.Context, groupID string) error {
	if r.AuthService == nil || !r.AuthService.IsEnabled() {
		return nil
	}

	if !middleware.IsAuthenticated(ctx) {
		return ErrNotAuthenticated
	}

	// System admins have admin access to all groups
	if middleware.IsAdmin(ctx) {
		return nil
	}

	if !middleware.IsGroupAdmin(ctx, groupID) {
		return ErrNotAuthorized
	}

	return nil
}

// ensureProjectAccess checks if the user has access to a project by verifying
// the project's group is in the user's accessible groups.
// Returns nil if auth is disabled.
func (r *Resolver) ensureProjectAccess(ctx context.Context, projectID string) error {
	if r.AuthService == nil || !r.AuthService.IsEnabled() {
		return nil
	}

	if !middleware.IsAuthenticated(ctx) {
		return ErrNotAuthenticated
	}

	// System admins have access to all projects
	if middleware.IsAdmin(ctx) {
		return nil
	}

	// Load the project to check its group
	project, err := r.ProjectRepo.FindByID(ctx, projectID)
	if err != nil {
		return err
	}
	if project == nil {
		return fmt.Errorf("project not found")
	}

	// Projects without a group are only accessible to system admins when auth is enabled.
	// This prevents orphaned projects from leaking across tenants.
	if project.GroupID == nil {
		return ErrNotAuthorized
	}

	if !middleware.IsGroupMember(ctx, *project.GroupID) {
		return ErrNotAuthorized
	}

	return nil
}

// getAccessibleGroupIDs returns the group IDs the current user can access.
// System admins get all groups. Regular users get their membership groups.
// Returns nil (no filtering) if auth is disabled.
func (r *Resolver) getAccessibleGroupIDs(ctx context.Context) ([]string, error) {
	if r.AuthService == nil || !r.AuthService.IsEnabled() {
		return nil, nil // nil means no filtering
	}

	if !middleware.IsAuthenticated(ctx) {
		return nil, ErrNotAuthenticated
	}

	// System admins see all groups
	if middleware.IsAdmin(ctx) {
		return nil, nil // nil means no filtering
	}

	return middleware.GetUserGroupIDs(ctx), nil
}

// toAuthUser converts a User model to an AuthUser response.
func (r *Resolver) toAuthUser(user *models.User) *generated.AuthUser {
	if user == nil {
		return nil
	}

	var lastLoginAt *string
	if user.LastLoginAt != nil {
		formatted := user.LastLoginAt.UTC().Format("2006-01-02T15:04:05.000Z")
		lastLoginAt = &formatted
	}

	return &generated.AuthUser{
		ID:            user.ID,
		Email:         user.Email,
		Name:          user.Name,
		Phone:         user.Phone,
		Role:          generated.UserRole(user.Role),
		EmailVerified: user.EmailVerified,
		PhoneVerified: user.PhoneVerified,
		IsActive:      user.IsActive,
		CreatedAt:     user.CreatedAt.UTC().Format("2006-01-02T15:04:05.000Z"),
		UpdatedAt:     user.UpdatedAt.UTC().Format("2006-01-02T15:04:05.000Z"),
		LastLoginAt:   lastLoginAt,
		Groups:        nil,        // Groups will be populated by field resolver if queried
		Permissions:   []string{}, // TODO: Compute permissions from groups
	}
}
