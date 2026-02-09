// Package middleware provides HTTP middleware for the LacyLights server.
package middleware

import (
	"context"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/bbernstein/lacylights-go/internal/auth"
	"github.com/bbernstein/lacylights-go/internal/auth/session"
	"github.com/bbernstein/lacylights-go/internal/database/models"
	"gorm.io/gorm"
)

// ContextKey is a type for context keys used by auth middleware.
// It is defined as its own type to reduce the risk of collisions with context keys from other packages.
type ContextKey string

const (
	// ContextKeySession is the context key for the authenticated session.
	ContextKeySession ContextKey = "lacylights:auth:session"
	// ContextKeyUserID is the context key for the authenticated user's ID.
	ContextKeyUserID ContextKey = "lacylights:auth:userID"
	// ContextKeyUserEmail is the context key for the authenticated user's email.
	ContextKeyUserEmail ContextKey = "lacylights:auth:userEmail"
	// ContextKeyUserRole is the context key for the authenticated user's role.
	ContextKeyUserRole ContextKey = "lacylights:auth:userRole"
	// ContextKeyDevice is the context key for the authenticated device.
	ContextKeyDevice ContextKey = "lacylights:auth:device"
	// ContextKeyDevicePermissions is the context key for the device's permissions.
	ContextKeyDevicePermissions ContextKey = "lacylights:auth:devicePermissions"
	// ContextKeyUserGroups is the context key for the user's group memberships.
	ContextKeyUserGroups ContextKey = "lacylights:auth:userGroups"
)

// UserGroupMembership represents a user's or device's membership in a group.
type UserGroupMembership struct {
	GroupID string
	Role    string
}

// AuthMiddleware provides authentication middleware.
type AuthMiddleware struct {
	authService *auth.Service
	db          *gorm.DB
}

// NewAuthMiddleware creates a new auth middleware instance.
func NewAuthMiddleware(authService *auth.Service) *AuthMiddleware {
	return &AuthMiddleware{
		authService: authService,
	}
}

// NewAuthMiddlewareWithDB creates a new auth middleware instance with database access.
func NewAuthMiddlewareWithDB(authService *auth.Service, db *gorm.DB) *AuthMiddleware {
	return &AuthMiddleware{
		authService: authService,
		db:          db,
	}
}

// Authenticate is middleware that extracts and validates authentication.
// If auth is disabled, it passes through without checking.
// If auth is enabled:
//   - First checks for Bearer token (JWT) authentication
//   - Then checks for device fingerprint (X-Device-Fingerprint header)
//   - When both are present, JWT provides user identity and device context is also set
//   - If only device is approved, uses device identity as fallback
//   - If no valid auth, passes through without context (resolvers check)
func (m *AuthMiddleware) Authenticate(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// If auth is disabled, pass through without auth context
		if !m.authService.IsEnabled() {
			next.ServeHTTP(w, r)
			return
		}

		ctx := r.Context()
		jwtAuthenticated := false

		// Try JWT Bearer token authentication first (highest priority)
		token := extractBearerToken(r)
		if token == "" {
			token = extractCookieToken(r)
		}

		if token != "" {
			claims, err := m.authService.JWTService().ValidateAccessToken(token)
			if err == nil {
				// Verify the session still exists (not revoked)
				dbSession, sessErr := m.authService.SessionManager().GetByID(ctx, claims.SessionID)
				if sessErr == nil && dbSession != nil {
					// Valid JWT session - set user context
					sess := &session.CachedSession{
						UserID:    claims.UserID,
						Email:     claims.Email,
						Role:      claims.Role,
						SessionID: claims.SessionID,
					}

					ctx = context.WithValue(ctx, ContextKeySession, sess)
					ctx = context.WithValue(ctx, ContextKeyUserID, claims.UserID)
					ctx = context.WithValue(ctx, ContextKeyUserEmail, claims.Email)
					ctx = context.WithValue(ctx, ContextKeyUserRole, claims.Role)

					// Load user group memberships
					userGroups := m.loadUserGroupMemberships(r.Context(), claims.UserID)
					ctx = context.WithValue(ctx, ContextKeyUserGroups, userGroups)

					jwtAuthenticated = true
				}
			}
		}

		// Check device fingerprint (adds device context alongside JWT, or as fallback)
		if m.authService.IsDeviceAuthEnabled() && m.db != nil {
			fingerprint := r.Header.Get("X-Device-Fingerprint")
			if fingerprint != "" {
				device, err := m.getDeviceByFingerprint(r.Context(), fingerprint)
				if err == nil && device != nil && device.Status == models.DeviceStatusApproved {
					// Always add device info to context (useful for device-aware features)
					ctx = context.WithValue(ctx, ContextKeyDevice, device)
					ctx = context.WithValue(ctx, ContextKeyDevicePermissions, device.Permissions)

					// Update last seen timestamp asynchronously
					go m.updateDeviceLastSeen(device.ID, r.RemoteAddr)

					// If no JWT user, fall back to device identity
					if !jwtAuthenticated {
						role := mapDevicePermissionsToRole(device.Permissions)
						ctx = context.WithValue(ctx, ContextKeyUserRole, role)

						if device.DefaultUserID != nil {
							ctx = context.WithValue(ctx, ContextKeyUserID, *device.DefaultUserID)
						}

						// Load device group memberships
						deviceGroups := m.loadDeviceGroupMemberships(r.Context(), device.ID)
						ctx = context.WithValue(ctx, ContextKeyUserGroups, deviceGroups)
					}
				}
			}
		}

		// Continue with enriched context
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// getDeviceByFingerprint retrieves a device by its fingerprint.
func (m *AuthMiddleware) getDeviceByFingerprint(ctx context.Context, fingerprint string) (*models.Device, error) {
	var device models.Device
	err := m.db.WithContext(ctx).Where("fingerprint = ?", fingerprint).First(&device).Error
	if err != nil {
		return nil, err
	}
	return &device, nil
}

// updateDeviceLastSeen updates the device's last seen timestamp.
// This method is called asynchronously to avoid blocking requests.
// Uses a detached context with timeout to prevent writes after the request is cancelled
// and to avoid connection leaks from long-running database operations.
// Errors are logged but not propagated since this is a non-critical operation.
func (m *AuthMiddleware) updateDeviceLastSeen(deviceID string, ipAddress string) {
	if m.db == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	now := time.Now()
	result := m.db.WithContext(ctx).Model(&models.Device{}).Where("id = ?", deviceID).Updates(map[string]any{
		"last_seen_at":    now,
		"last_ip_address": ipAddress,
	})
	if result.Error != nil {
		log.Printf("Warning: failed to update device last seen for device %s: %v", deviceID, result.Error)
	}
}

// mapDevicePermissionsToRole maps device permissions to a user role for authorization.
// This mapping allows device-authenticated requests to use the same role-based
// authorization checks as user-authenticated requests.
//
// Mapping:
//   - DevicePermissionsAdmin ("ADMIN") -> "ADMIN" role
//   - DevicePermissionsOperator ("OPERATOR") -> "OPERATOR" role
//   - DevicePermissionsReadOnly ("READ_ONLY") -> "USER" role (default)
//
// Unknown permission values default to "USER" for security (least privilege).
func mapDevicePermissionsToRole(permissions string) string {
	switch permissions {
	case models.DevicePermissionsAdmin:
		return "ADMIN"
	case models.DevicePermissionsOperator:
		return "OPERATOR"
	default:
		return "USER"
	}
}

// RequireAuth is middleware that requires a valid authentication.
// Returns 401 if not authenticated.
func (m *AuthMiddleware) RequireAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// If auth is disabled, pass through
		if !m.authService.IsEnabled() {
			next.ServeHTTP(w, r)
			return
		}

		// Check if session exists in context
		sess := GetSessionFromContext(r.Context())
		if sess == nil {
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}

		next.ServeHTTP(w, r)
	})
}

// RequireAdmin is middleware that requires admin role.
// Returns 401 if not authenticated, 403 if not admin.
func (m *AuthMiddleware) RequireAdmin(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// If auth is disabled, pass through
		if !m.authService.IsEnabled() {
			next.ServeHTTP(w, r)
			return
		}

		// Check if session exists in context
		sess := GetSessionFromContext(r.Context())
		if sess == nil {
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}

		// Check for admin role
		if sess.Role != "ADMIN" {
			http.Error(w, "Forbidden", http.StatusForbidden)
			return
		}

		next.ServeHTTP(w, r)
	})
}

// extractBearerToken extracts the token from the Authorization header.
func extractBearerToken(r *http.Request) string {
	authHeader := r.Header.Get("Authorization")
	if authHeader == "" {
		return ""
	}

	// Check for "Bearer " prefix
	const prefix = "Bearer "
	if !strings.HasPrefix(authHeader, prefix) {
		return ""
	}

	return strings.TrimPrefix(authHeader, prefix)
}

// extractCookieToken extracts the token from the auth cookie.
func extractCookieToken(r *http.Request) string {
	cookie, err := r.Cookie("lacylights_token")
	if err != nil {
		return ""
	}
	return cookie.Value
}

// GetSessionFromContext retrieves the session from the request context.
func GetSessionFromContext(ctx context.Context) *session.CachedSession {
	sess, ok := ctx.Value(ContextKeySession).(*session.CachedSession)
	if !ok {
		return nil
	}
	return sess
}

// GetUserIDFromContext retrieves the user ID from the request context.
func GetUserIDFromContext(ctx context.Context) string {
	userID, ok := ctx.Value(ContextKeyUserID).(string)
	if !ok {
		return ""
	}
	return userID
}

// GetUserEmailFromContext retrieves the user email from the request context.
func GetUserEmailFromContext(ctx context.Context) string {
	email, ok := ctx.Value(ContextKeyUserEmail).(string)
	if !ok {
		return ""
	}
	return email
}

// GetUserRoleFromContext retrieves the user role from the request context.
func GetUserRoleFromContext(ctx context.Context) string {
	role, ok := ctx.Value(ContextKeyUserRole).(string)
	if !ok {
		return ""
	}
	return role
}

// IsAuthenticated checks if the request is authenticated (either by session or device).
func IsAuthenticated(ctx context.Context) bool {
	return GetSessionFromContext(ctx) != nil || GetDeviceFromContext(ctx) != nil
}

// IsAdmin checks if the authenticated user/device is an admin.
func IsAdmin(ctx context.Context) bool {
	return GetUserRoleFromContext(ctx) == "ADMIN"
}

// GetDeviceFromContext retrieves the device from the request context.
func GetDeviceFromContext(ctx context.Context) *models.Device {
	device, ok := ctx.Value(ContextKeyDevice).(*models.Device)
	if !ok {
		return nil
	}
	return device
}

// GetDevicePermissionsFromContext retrieves the device permissions from the request context.
func GetDevicePermissionsFromContext(ctx context.Context) string {
	permissions, ok := ctx.Value(ContextKeyDevicePermissions).(string)
	if !ok {
		return ""
	}
	return permissions
}

// IsDeviceAuthenticated checks if the request is authenticated by a device.
func IsDeviceAuthenticated(ctx context.Context) bool {
	return GetDeviceFromContext(ctx) != nil
}

// GetUserGroupsFromContext retrieves the user's group memberships from the context.
func GetUserGroupsFromContext(ctx context.Context) []UserGroupMembership {
	groups, ok := ctx.Value(ContextKeyUserGroups).([]UserGroupMembership)
	if !ok {
		return nil
	}
	return groups
}

// GetUserGroupIDs returns the group IDs the user belongs to.
func GetUserGroupIDs(ctx context.Context) []string {
	groups := GetUserGroupsFromContext(ctx)
	ids := make([]string, len(groups))
	for i, g := range groups {
		ids[i] = g.GroupID
	}
	return ids
}

// IsGroupMember checks if the user is a member of the given group.
func IsGroupMember(ctx context.Context, groupID string) bool {
	for _, g := range GetUserGroupsFromContext(ctx) {
		if g.GroupID == groupID {
			return true
		}
	}
	return false
}

// IsGroupAdmin checks if the user is a GROUP_ADMIN of the given group.
func IsGroupAdmin(ctx context.Context, groupID string) bool {
	for _, g := range GetUserGroupsFromContext(ctx) {
		if g.GroupID == groupID && g.Role == "GROUP_ADMIN" {
			return true
		}
	}
	return false
}

// loadUserGroupMemberships queries the database for a user's group memberships.
func (m *AuthMiddleware) loadUserGroupMemberships(ctx context.Context, userID string) []UserGroupMembership {
	if m.db == nil || userID == "" {
		return nil
	}

	var members []models.UserGroupMember
	if err := m.db.WithContext(ctx).Select("group_id", "role").Where("user_id = ?", userID).Find(&members).Error; err != nil {
		log.Printf("Warning: failed to load group memberships for user %s: %v", userID, err)
		return nil
	}

	result := make([]UserGroupMembership, len(members))
	for i, member := range members {
		result[i] = UserGroupMembership{
			GroupID: member.GroupID,
			Role:    member.Role,
		}
	}
	return result
}

// loadDeviceGroupMemberships queries the database for a device's group memberships.
func (m *AuthMiddleware) loadDeviceGroupMemberships(ctx context.Context, deviceID string) []UserGroupMembership {
	if m.db == nil || deviceID == "" {
		return nil
	}

	var members []models.DeviceGroupMember
	if err := m.db.WithContext(ctx).Select("group_id").Where("device_id = ?", deviceID).Find(&members).Error; err != nil {
		log.Printf("Warning: failed to load group memberships for device %s: %v", deviceID, err)
		return nil
	}

	result := make([]UserGroupMembership, len(members))
	for i, member := range members {
		result[i] = UserGroupMembership{
			GroupID: member.GroupID,
			Role:    "MEMBER", // Devices are always MEMBER role within a group
		}
	}
	return result
}
