// Package middleware provides HTTP middleware for the LacyLights server.
package middleware

import (
	"context"
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
)

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
//   - First checks for device fingerprint (X-Device-Fingerprint header)
//   - If device is approved, adds device info to context
//   - Falls back to Bearer token authentication
//   - If no valid auth, passes through without context (resolvers check)
func (m *AuthMiddleware) Authenticate(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// If auth is disabled, pass through without auth context
		if !m.authService.IsEnabled() {
			next.ServeHTTP(w, r)
			return
		}

		// Try device fingerprint authentication first if device auth is enabled
		if m.authService.IsDeviceAuthEnabled() && m.db != nil {
			fingerprint := r.Header.Get("X-Device-Fingerprint")
			if fingerprint != "" {
				device, err := m.getDeviceByFingerprint(r.Context(), fingerprint)
				if err == nil && device != nil && device.Status == models.DeviceStatusApproved {
					// Device is approved - add device info to context
					ctx := r.Context()
					ctx = context.WithValue(ctx, ContextKeyDevice, device)
					ctx = context.WithValue(ctx, ContextKeyDevicePermissions, device.Permissions)

					// Map device permissions to a role for authorization checks
					role := mapDevicePermissionsToRole(device.Permissions)
					ctx = context.WithValue(ctx, ContextKeyUserRole, role)

					// If device has a default user, also set user context
					if device.DefaultUserID != nil {
						ctx = context.WithValue(ctx, ContextKeyUserID, *device.DefaultUserID)
					}

					// Update last seen timestamp asynchronously
					go m.updateDeviceLastSeen(device.ID, r.RemoteAddr)

					// Continue with enriched context
					next.ServeHTTP(w, r.WithContext(ctx))
					return
				}
			}
		}

		// Try to extract token from Authorization header first
		token := extractBearerToken(r)

		// Fall back to cookie if no header token
		if token == "" {
			token = extractCookieToken(r)
		}

		// If no token found, continue without auth context
		if token == "" {
			next.ServeHTTP(w, r)
			return
		}

		// Validate the token
		claims, err := m.authService.JWTService().ValidateAccessToken(token)
		if err != nil {
			// Invalid token - continue without auth context
			// Resolvers will handle unauthorized access
			next.ServeHTTP(w, r)
			return
		}

		// Verify the session still exists (not revoked)
		// This prevents use of valid JWTs after session logout/revocation
		dbSession, err := m.authService.SessionManager().GetByID(r.Context(), claims.SessionID)
		if err != nil || dbSession == nil {
			// Session not found or revoked - continue without auth context
			next.ServeHTTP(w, r)
			return
		}

		// Create cached session from claims
		sess := &session.CachedSession{
			UserID:    claims.UserID,
			Email:     claims.Email,
			Role:      claims.Role,
			SessionID: claims.SessionID,
		}

		// Add session and user info to context
		ctx := r.Context()
		ctx = context.WithValue(ctx, ContextKeySession, sess)
		ctx = context.WithValue(ctx, ContextKeyUserID, claims.UserID)
		ctx = context.WithValue(ctx, ContextKeyUserEmail, claims.Email)
		ctx = context.WithValue(ctx, ContextKeyUserRole, claims.Role)

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
func (m *AuthMiddleware) updateDeviceLastSeen(deviceID string, ipAddress string) {
	if m.db == nil {
		return
	}
	now := time.Now()
	m.db.Model(&models.Device{}).Where("id = ?", deviceID).Updates(map[string]interface{}{
		"last_seen_at":    now,
		"last_ip_address": ipAddress,
	})
}

// mapDevicePermissionsToRole maps device permissions to a user role for authorization.
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
