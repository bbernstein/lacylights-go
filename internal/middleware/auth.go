// Package middleware provides HTTP middleware for the LacyLights server.
package middleware

import (
	"context"
	"net/http"
	"strings"

	"github.com/bbernstein/lacylights-go/internal/auth"
	"github.com/bbernstein/lacylights-go/internal/auth/session"
)

// ContextKey is a type for context keys used by auth middleware.
type ContextKey string

const (
	// ContextKeySession is the context key for the authenticated session.
	ContextKeySession ContextKey = "session"
	// ContextKeyUserID is the context key for the authenticated user's ID.
	ContextKeyUserID ContextKey = "userID"
	// ContextKeyUserEmail is the context key for the authenticated user's email.
	ContextKeyUserEmail ContextKey = "userEmail"
	// ContextKeyUserRole is the context key for the authenticated user's role.
	ContextKeyUserRole ContextKey = "userRole"
)

// AuthMiddleware provides authentication middleware.
type AuthMiddleware struct {
	authService *auth.Service
}

// NewAuthMiddleware creates a new auth middleware instance.
func NewAuthMiddleware(authService *auth.Service) *AuthMiddleware {
	return &AuthMiddleware{
		authService: authService,
	}
}

// Authenticate is middleware that extracts and validates the JWT token.
// If auth is disabled, it passes through without checking.
// If auth is enabled and token is valid, it adds user info to context.
// If auth is enabled and token is invalid/missing, it still passes through
// but without user context (resolvers can check for auth requirement).
func (m *AuthMiddleware) Authenticate(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// If auth is disabled, pass through without auth context
		if !m.authService.IsEnabled() {
			next.ServeHTTP(w, r)
			return
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

// IsAuthenticated checks if the request is authenticated.
func IsAuthenticated(ctx context.Context) bool {
	return GetSessionFromContext(ctx) != nil
}

// IsAdmin checks if the authenticated user is an admin.
func IsAdmin(ctx context.Context) bool {
	return GetUserRoleFromContext(ctx) == "ADMIN"
}
