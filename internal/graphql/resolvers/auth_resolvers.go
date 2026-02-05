package resolvers

// This file contains implementations for auth-related GraphQL resolvers.
// The actual resolver stubs are generated in schema.resolvers.go by gqlgen.
// These helper functions are called from those stubs.

import (
	"context"
	"errors"

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

// getUserGroup returns a group by ID (admin only).
func (r *Resolver) getUserGroup(ctx context.Context, id string) (*models.UserGroup, error) {
	if err := r.requireAdmin(ctx); err != nil {
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
		// Store permissions as comma-separated string
		if len(perms) > 0 {
			permStr := ""
			for i, p := range perms {
				if i > 0 {
					permStr += ","
				}
				permStr += p
			}
			group.Permissions = &permStr
		}
	}

	if err := r.db.WithContext(ctx).Create(group).Error; err != nil {
		return nil, err
	}

	return group, nil
}

// updateUserGroup updates a group (admin only).
func (r *Resolver) updateUserGroup(ctx context.Context, id string, input generated.UpdateUserGroupInput) (*models.UserGroup, error) {
	if err := r.requireAdmin(ctx); err != nil {
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
		if len(perms) > 0 {
			permStr := ""
			for i, p := range perms {
				if i > 0 {
					permStr += ","
				}
				permStr += p
			}
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

// addUserToGroup adds a user to a group (admin only).
func (r *Resolver) addUserToGroup(ctx context.Context, userID, groupID string) (bool, error) {
	if err := r.requireAdmin(ctx); err != nil {
		return false, err
	}

	member := &models.UserGroupMember{
		ID:      cuid.New(),
		UserID:  userID,
		GroupID: groupID,
	}

	if err := r.db.WithContext(ctx).Create(member).Error; err != nil {
		return false, err
	}

	return true, nil
}

// removeUserFromGroup removes a user from a group (admin only).
func (r *Resolver) removeUserFromGroup(ctx context.Context, userID, groupID string) (bool, error) {
	if err := r.requireAdmin(ctx); err != nil {
		return false, err
	}

	result := r.db.WithContext(ctx).Delete(&models.UserGroupMember{}, "user_id = ? AND group_id = ?", userID, groupID)
	if result.Error != nil {
		return false, result.Error
	}
	return result.RowsAffected > 0, nil
}

// Device operations

// getDevices returns all devices (admin only).
func (r *Resolver) getDevices(ctx context.Context) ([]*models.Device, error) {
	if err := r.requireAdmin(ctx); err != nil {
		return nil, err
	}

	var devices []models.Device
	if err := r.db.WithContext(ctx).Find(&devices).Error; err != nil {
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

// checkDeviceAuth checks if a device fingerprint is authorized.
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

	if !device.IsAuthorized {
		return &generated.DeviceAuthStatus{
			IsAuthorized: false,
			IsPending:    true,
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
