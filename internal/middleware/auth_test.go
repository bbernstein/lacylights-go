package middleware

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/bbernstein/lacylights-go/internal/auth"
	"github.com/bbernstein/lacylights-go/internal/auth/session"
	"github.com/bbernstein/lacylights-go/internal/database/models"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

// createTestDB creates an in-memory SQLite database for testing.
func createTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("failed to create test database: %v", err)
	}

	// Run migrations for session and user tables
	err = db.AutoMigrate(&models.Session{}, &models.User{})
	if err != nil {
		t.Fatalf("failed to migrate database: %v", err)
	}

	return db
}

// createTestAuthService creates an auth service for testing.
func createTestAuthService(t *testing.T, enabled bool) *auth.Service {
	t.Helper()
	db := createTestDB(t)

	svc, err := auth.NewService(auth.Config{
		DB:                 db,
		JWTSecret:          "test-secret-key-at-least-32-chars",
		JWTIssuer:          "test-issuer",
		JWTAccessTokenTTL:  15 * time.Minute,
		JWTRefreshTokenTTL: 24 * time.Hour,
		PasswordMinLength:  8,
		Enabled:            enabled,
		DeviceAuthEnabled:  false,
	})
	if err != nil {
		t.Fatalf("failed to create auth service: %v", err)
	}
	return svc
}

// createTestSession creates a session in the database for testing.
func createTestSession(t *testing.T, db *gorm.DB, sessionID, userID string) {
	t.Helper()
	sess := &models.Session{
		ID:             sessionID,
		UserID:         userID,
		TokenHash:      "token-hash-for-" + sessionID, // Unique token hash per session
		ExpiresAt:      time.Now().Add(1 * time.Hour),
		LastActivityAt: time.Now(),
	}
	if err := db.Create(sess).Error; err != nil {
		t.Fatalf("failed to create test session: %v", err)
	}
}

// createTestAuthServiceWithDB creates an auth service for testing and returns both the service and DB.
func createTestAuthServiceWithDB(t *testing.T, enabled bool) (*auth.Service, *gorm.DB) {
	t.Helper()
	db := createTestDB(t)

	svc, err := auth.NewService(auth.Config{
		DB:                 db,
		JWTSecret:          "test-secret-key-at-least-32-chars",
		JWTIssuer:          "test-issuer",
		JWTAccessTokenTTL:  15 * time.Minute,
		JWTRefreshTokenTTL: 24 * time.Hour,
		PasswordMinLength:  8,
		Enabled:            enabled,
		DeviceAuthEnabled:  false,
	})
	if err != nil {
		t.Fatalf("failed to create auth service: %v", err)
	}
	return svc, db
}

func TestNewAuthMiddleware(t *testing.T) {
	authSvc := createTestAuthService(t, true)
	middleware := NewAuthMiddleware(authSvc)

	if middleware == nil {
		t.Fatal("expected middleware to be non-nil")
	}

	if middleware.authService != authSvc {
		t.Error("expected authService to be set correctly")
	}
}

func TestAuthenticate_AuthDisabled(t *testing.T) {
	authSvc := createTestAuthService(t, false)
	middleware := NewAuthMiddleware(authSvc)

	// Create a test handler that checks for session in context
	handlerCalled := false
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		handlerCalled = true
		// Should have no session when auth is disabled
		sess := GetSessionFromContext(r.Context())
		if sess != nil {
			t.Error("expected no session when auth is disabled")
		}
		w.WriteHeader(http.StatusOK)
	})

	// Create request without token
	req := httptest.NewRequest("GET", "/test", nil)
	rr := httptest.NewRecorder()

	middleware.Authenticate(handler).ServeHTTP(rr, req)

	if !handlerCalled {
		t.Error("expected handler to be called")
	}

	if rr.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", rr.Code)
	}
}

func TestAuthenticate_NoToken(t *testing.T) {
	authSvc := createTestAuthService(t, true)
	middleware := NewAuthMiddleware(authSvc)

	handlerCalled := false
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		handlerCalled = true
		sess := GetSessionFromContext(r.Context())
		if sess != nil {
			t.Error("expected no session when no token provided")
		}
		w.WriteHeader(http.StatusOK)
	})

	req := httptest.NewRequest("GET", "/test", nil)
	rr := httptest.NewRecorder()

	middleware.Authenticate(handler).ServeHTTP(rr, req)

	if !handlerCalled {
		t.Error("expected handler to be called")
	}
}

func TestAuthenticate_ValidBearerToken(t *testing.T) {
	authSvc, db := createTestAuthServiceWithDB(t, true)
	middleware := NewAuthMiddleware(authSvc)

	// Create a session in the database first
	createTestSession(t, db, "session456", "user123")

	// Generate a valid token
	jwtSvc := authSvc.JWTService()
	pair, err := jwtSvc.GenerateTokenPair("user123", "test@example.com", "USER", "session456")
	if err != nil {
		t.Fatalf("failed to generate token pair: %v", err)
	}

	handlerCalled := false
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		handlerCalled = true

		sess := GetSessionFromContext(r.Context())
		if sess == nil {
			t.Error("expected session in context")
			return
		}

		if sess.UserID != "user123" {
			t.Errorf("expected UserID user123, got %s", sess.UserID)
		}

		if sess.Email != "test@example.com" {
			t.Errorf("expected Email test@example.com, got %s", sess.Email)
		}

		if sess.Role != "USER" {
			t.Errorf("expected Role USER, got %s", sess.Role)
		}

		if sess.SessionID != "session456" {
			t.Errorf("expected SessionID session456, got %s", sess.SessionID)
		}

		w.WriteHeader(http.StatusOK)
	})

	req := httptest.NewRequest("GET", "/test", nil)
	req.Header.Set("Authorization", "Bearer "+pair.AccessToken)
	rr := httptest.NewRecorder()

	middleware.Authenticate(handler).ServeHTTP(rr, req)

	if !handlerCalled {
		t.Error("expected handler to be called")
	}
}

func TestAuthenticate_ValidCookieToken(t *testing.T) {
	authSvc, db := createTestAuthServiceWithDB(t, true)
	middleware := NewAuthMiddleware(authSvc)

	// Create a session in the database first
	createTestSession(t, db, "session456", "user123")

	// Generate a valid token
	jwtSvc := authSvc.JWTService()
	pair, err := jwtSvc.GenerateTokenPair("user123", "test@example.com", "USER", "session456")
	if err != nil {
		t.Fatalf("failed to generate token pair: %v", err)
	}

	handlerCalled := false
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		handlerCalled = true

		sess := GetSessionFromContext(r.Context())
		if sess == nil {
			t.Error("expected session in context")
			return
		}

		if sess.UserID != "user123" {
			t.Errorf("expected UserID user123, got %s", sess.UserID)
		}

		w.WriteHeader(http.StatusOK)
	})

	req := httptest.NewRequest("GET", "/test", nil)
	req.AddCookie(&http.Cookie{
		Name:  "lacylights_token",
		Value: pair.AccessToken,
	})
	rr := httptest.NewRecorder()

	middleware.Authenticate(handler).ServeHTTP(rr, req)

	if !handlerCalled {
		t.Error("expected handler to be called")
	}
}

func TestAuthenticate_InvalidToken(t *testing.T) {
	authSvc := createTestAuthService(t, true)
	middleware := NewAuthMiddleware(authSvc)

	handlerCalled := false
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		handlerCalled = true
		sess := GetSessionFromContext(r.Context())
		if sess != nil {
			t.Error("expected no session with invalid token")
		}
		w.WriteHeader(http.StatusOK)
	})

	req := httptest.NewRequest("GET", "/test", nil)
	req.Header.Set("Authorization", "Bearer invalid-token")
	rr := httptest.NewRecorder()

	middleware.Authenticate(handler).ServeHTTP(rr, req)

	if !handlerCalled {
		t.Error("expected handler to be called even with invalid token")
	}
}

func TestAuthenticate_SessionRevoked(t *testing.T) {
	authSvc := createTestAuthService(t, true)
	middleware := NewAuthMiddleware(authSvc)

	// Generate a valid token WITHOUT creating a session in the database
	// This simulates a session that was revoked/deleted
	jwtSvc := authSvc.JWTService()
	pair, err := jwtSvc.GenerateTokenPair("user123", "test@example.com", "USER", "revoked-session")
	if err != nil {
		t.Fatalf("failed to generate token pair: %v", err)
	}

	handlerCalled := false
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		handlerCalled = true
		sess := GetSessionFromContext(r.Context())
		if sess != nil {
			t.Error("expected no session when session is revoked")
		}
		w.WriteHeader(http.StatusOK)
	})

	req := httptest.NewRequest("GET", "/test", nil)
	req.Header.Set("Authorization", "Bearer "+pair.AccessToken)
	rr := httptest.NewRecorder()

	middleware.Authenticate(handler).ServeHTTP(rr, req)

	if !handlerCalled {
		t.Error("expected handler to be called even with revoked session")
	}

	if rr.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", rr.Code)
	}
}

func TestAuthenticate_BearerTokenPrioritizedOverCookie(t *testing.T) {
	authSvc, db := createTestAuthServiceWithDB(t, true)
	middleware := NewAuthMiddleware(authSvc)

	// Create sessions in the database first
	createTestSession(t, db, "session1", "headerUser")
	createTestSession(t, db, "session2", "cookieUser")

	// Generate two different tokens
	jwtSvc := authSvc.JWTService()
	headerPair, err := jwtSvc.GenerateTokenPair("headerUser", "header@example.com", "USER", "session1")
	if err != nil {
		t.Fatalf("failed to generate header token: %v", err)
	}

	cookiePair, err := jwtSvc.GenerateTokenPair("cookieUser", "cookie@example.com", "USER", "session2")
	if err != nil {
		t.Fatalf("failed to generate cookie token: %v", err)
	}

	handlerCalled := false
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		handlerCalled = true

		sess := GetSessionFromContext(r.Context())
		if sess == nil {
			t.Error("expected session in context")
			return
		}

		// Should use header token, not cookie token
		if sess.UserID != "headerUser" {
			t.Errorf("expected UserID headerUser, got %s (cookie token was used instead of header)", sess.UserID)
		}

		w.WriteHeader(http.StatusOK)
	})

	req := httptest.NewRequest("GET", "/test", nil)
	req.Header.Set("Authorization", "Bearer "+headerPair.AccessToken)
	req.AddCookie(&http.Cookie{
		Name:  "lacylights_token",
		Value: cookiePair.AccessToken,
	})
	rr := httptest.NewRecorder()

	middleware.Authenticate(handler).ServeHTTP(rr, req)

	if !handlerCalled {
		t.Error("expected handler to be called")
	}
}

func TestRequireAuth_AuthDisabled(t *testing.T) {
	authSvc := createTestAuthService(t, false)
	middleware := NewAuthMiddleware(authSvc)

	handlerCalled := false
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		handlerCalled = true
		w.WriteHeader(http.StatusOK)
	})

	req := httptest.NewRequest("GET", "/test", nil)
	rr := httptest.NewRecorder()

	middleware.RequireAuth(handler).ServeHTTP(rr, req)

	if !handlerCalled {
		t.Error("expected handler to be called when auth is disabled")
	}

	if rr.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", rr.Code)
	}
}

func TestRequireAuth_NoSession(t *testing.T) {
	authSvc := createTestAuthService(t, true)
	middleware := NewAuthMiddleware(authSvc)

	handlerCalled := false
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		handlerCalled = true
		w.WriteHeader(http.StatusOK)
	})

	req := httptest.NewRequest("GET", "/test", nil)
	rr := httptest.NewRecorder()

	middleware.RequireAuth(handler).ServeHTTP(rr, req)

	if handlerCalled {
		t.Error("expected handler NOT to be called without session")
	}

	if rr.Code != http.StatusUnauthorized {
		t.Errorf("expected status 401, got %d", rr.Code)
	}
}

func TestRequireAuth_WithSession(t *testing.T) {
	authSvc := createTestAuthService(t, true)
	middleware := NewAuthMiddleware(authSvc)

	handlerCalled := false
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		handlerCalled = true
		w.WriteHeader(http.StatusOK)
	})

	// Create a request with session in context
	req := httptest.NewRequest("GET", "/test", nil)
	sess := &session.CachedSession{
		UserID:    "user123",
		Email:     "test@example.com",
		Role:      "USER",
		SessionID: "session456",
	}
	ctx := context.WithValue(req.Context(), ContextKeySession, sess)
	req = req.WithContext(ctx)
	rr := httptest.NewRecorder()

	middleware.RequireAuth(handler).ServeHTTP(rr, req)

	if !handlerCalled {
		t.Error("expected handler to be called with session")
	}

	if rr.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", rr.Code)
	}
}

func TestRequireAdmin_AuthDisabled(t *testing.T) {
	authSvc := createTestAuthService(t, false)
	middleware := NewAuthMiddleware(authSvc)

	handlerCalled := false
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		handlerCalled = true
		w.WriteHeader(http.StatusOK)
	})

	req := httptest.NewRequest("GET", "/test", nil)
	rr := httptest.NewRecorder()

	middleware.RequireAdmin(handler).ServeHTTP(rr, req)

	if !handlerCalled {
		t.Error("expected handler to be called when auth is disabled")
	}

	if rr.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", rr.Code)
	}
}

func TestRequireAdmin_NoSession(t *testing.T) {
	authSvc := createTestAuthService(t, true)
	middleware := NewAuthMiddleware(authSvc)

	handlerCalled := false
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		handlerCalled = true
		w.WriteHeader(http.StatusOK)
	})

	req := httptest.NewRequest("GET", "/test", nil)
	rr := httptest.NewRecorder()

	middleware.RequireAdmin(handler).ServeHTTP(rr, req)

	if handlerCalled {
		t.Error("expected handler NOT to be called without session")
	}

	if rr.Code != http.StatusUnauthorized {
		t.Errorf("expected status 401, got %d", rr.Code)
	}
}

func TestRequireAdmin_NonAdminUser(t *testing.T) {
	authSvc := createTestAuthService(t, true)
	middleware := NewAuthMiddleware(authSvc)

	handlerCalled := false
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		handlerCalled = true
		w.WriteHeader(http.StatusOK)
	})

	// Create a request with non-admin session in context
	req := httptest.NewRequest("GET", "/test", nil)
	sess := &session.CachedSession{
		UserID:    "user123",
		Email:     "test@example.com",
		Role:      "USER", // Not ADMIN
		SessionID: "session456",
	}
	ctx := context.WithValue(req.Context(), ContextKeySession, sess)
	req = req.WithContext(ctx)
	rr := httptest.NewRecorder()

	middleware.RequireAdmin(handler).ServeHTTP(rr, req)

	if handlerCalled {
		t.Error("expected handler NOT to be called for non-admin user")
	}

	if rr.Code != http.StatusForbidden {
		t.Errorf("expected status 403, got %d", rr.Code)
	}
}

func TestRequireAdmin_AdminUser(t *testing.T) {
	authSvc := createTestAuthService(t, true)
	middleware := NewAuthMiddleware(authSvc)

	handlerCalled := false
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		handlerCalled = true
		w.WriteHeader(http.StatusOK)
	})

	// Create a request with admin session in context
	req := httptest.NewRequest("GET", "/test", nil)
	sess := &session.CachedSession{
		UserID:    "admin123",
		Email:     "admin@example.com",
		Role:      "ADMIN",
		SessionID: "session789",
	}
	ctx := context.WithValue(req.Context(), ContextKeySession, sess)
	req = req.WithContext(ctx)
	rr := httptest.NewRecorder()

	middleware.RequireAdmin(handler).ServeHTTP(rr, req)

	if !handlerCalled {
		t.Error("expected handler to be called for admin user")
	}

	if rr.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", rr.Code)
	}
}

func TestExtractBearerToken(t *testing.T) {
	testCases := []struct {
		name     string
		header   string
		expected string
	}{
		{"valid bearer token", "Bearer abc123", "abc123"},
		{"empty header", "", ""},
		{"no bearer prefix", "abc123", ""},
		{"lowercase bearer", "bearer abc123", ""},
		{"only bearer prefix", "Bearer ", ""},
		{"bearer with token", "Bearer my-jwt-token", "my-jwt-token"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest("GET", "/test", nil)
			if tc.header != "" {
				req.Header.Set("Authorization", tc.header)
			}

			token := extractBearerToken(req)
			if token != tc.expected {
				t.Errorf("expected %q, got %q", tc.expected, token)
			}
		})
	}
}

func TestExtractCookieToken(t *testing.T) {
	testCases := []struct {
		name       string
		cookieName string
		cookieVal  string
		expected   string
	}{
		{"valid cookie", "lacylights_token", "abc123", "abc123"},
		{"wrong cookie name", "other_token", "abc123", ""},
		{"no cookie", "", "", ""},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest("GET", "/test", nil)
			if tc.cookieName != "" {
				req.AddCookie(&http.Cookie{
					Name:  tc.cookieName,
					Value: tc.cookieVal,
				})
			}

			token := extractCookieToken(req)
			if token != tc.expected {
				t.Errorf("expected %q, got %q", tc.expected, token)
			}
		})
	}
}

func TestGetSessionFromContext(t *testing.T) {
	t.Run("session exists", func(t *testing.T) {
		sess := &session.CachedSession{
			UserID:    "user123",
			Email:     "test@example.com",
			Role:      "USER",
			SessionID: "session456",
		}
		ctx := context.WithValue(context.Background(), ContextKeySession, sess)

		result := GetSessionFromContext(ctx)
		if result == nil {
			t.Fatal("expected session to be returned")
		}

		if result.UserID != "user123" {
			t.Errorf("expected UserID user123, got %s", result.UserID)
		}
	})

	t.Run("no session", func(t *testing.T) {
		ctx := context.Background()
		result := GetSessionFromContext(ctx)
		if result != nil {
			t.Error("expected nil session")
		}
	})

	t.Run("wrong type in context", func(t *testing.T) {
		ctx := context.WithValue(context.Background(), ContextKeySession, "not a session")
		result := GetSessionFromContext(ctx)
		if result != nil {
			t.Error("expected nil for wrong type")
		}
	})
}

func TestGetUserIDFromContext(t *testing.T) {
	t.Run("user ID exists", func(t *testing.T) {
		ctx := context.WithValue(context.Background(), ContextKeyUserID, "user123")
		result := GetUserIDFromContext(ctx)
		if result != "user123" {
			t.Errorf("expected user123, got %s", result)
		}
	})

	t.Run("no user ID", func(t *testing.T) {
		ctx := context.Background()
		result := GetUserIDFromContext(ctx)
		if result != "" {
			t.Errorf("expected empty string, got %s", result)
		}
	})

	t.Run("wrong type", func(t *testing.T) {
		ctx := context.WithValue(context.Background(), ContextKeyUserID, 123)
		result := GetUserIDFromContext(ctx)
		if result != "" {
			t.Errorf("expected empty string for wrong type, got %s", result)
		}
	})
}

func TestGetUserEmailFromContext(t *testing.T) {
	t.Run("email exists", func(t *testing.T) {
		ctx := context.WithValue(context.Background(), ContextKeyUserEmail, "test@example.com")
		result := GetUserEmailFromContext(ctx)
		if result != "test@example.com" {
			t.Errorf("expected test@example.com, got %s", result)
		}
	})

	t.Run("no email", func(t *testing.T) {
		ctx := context.Background()
		result := GetUserEmailFromContext(ctx)
		if result != "" {
			t.Errorf("expected empty string, got %s", result)
		}
	})

	t.Run("wrong type", func(t *testing.T) {
		ctx := context.WithValue(context.Background(), ContextKeyUserEmail, 123)
		result := GetUserEmailFromContext(ctx)
		if result != "" {
			t.Errorf("expected empty string for wrong type, got %s", result)
		}
	})
}

func TestGetUserRoleFromContext(t *testing.T) {
	t.Run("role exists", func(t *testing.T) {
		ctx := context.WithValue(context.Background(), ContextKeyUserRole, "ADMIN")
		result := GetUserRoleFromContext(ctx)
		if result != "ADMIN" {
			t.Errorf("expected ADMIN, got %s", result)
		}
	})

	t.Run("no role", func(t *testing.T) {
		ctx := context.Background()
		result := GetUserRoleFromContext(ctx)
		if result != "" {
			t.Errorf("expected empty string, got %s", result)
		}
	})

	t.Run("wrong type", func(t *testing.T) {
		ctx := context.WithValue(context.Background(), ContextKeyUserRole, 123)
		result := GetUserRoleFromContext(ctx)
		if result != "" {
			t.Errorf("expected empty string for wrong type, got %s", result)
		}
	})
}

func TestIsAuthenticated(t *testing.T) {
	t.Run("authenticated", func(t *testing.T) {
		sess := &session.CachedSession{
			UserID:    "user123",
			Email:     "test@example.com",
			Role:      "USER",
			SessionID: "session456",
		}
		ctx := context.WithValue(context.Background(), ContextKeySession, sess)

		if !IsAuthenticated(ctx) {
			t.Error("expected IsAuthenticated to return true")
		}
	})

	t.Run("not authenticated", func(t *testing.T) {
		ctx := context.Background()
		if IsAuthenticated(ctx) {
			t.Error("expected IsAuthenticated to return false")
		}
	})
}

func TestIsAdmin(t *testing.T) {
	t.Run("is admin", func(t *testing.T) {
		ctx := context.WithValue(context.Background(), ContextKeyUserRole, "ADMIN")
		if !IsAdmin(ctx) {
			t.Error("expected IsAdmin to return true")
		}
	})

	t.Run("is user", func(t *testing.T) {
		ctx := context.WithValue(context.Background(), ContextKeyUserRole, "USER")
		if IsAdmin(ctx) {
			t.Error("expected IsAdmin to return false for USER role")
		}
	})

	t.Run("no role", func(t *testing.T) {
		ctx := context.Background()
		if IsAdmin(ctx) {
			t.Error("expected IsAdmin to return false when no role")
		}
	})
}

func TestContextKeys(t *testing.T) {
	// Verify context keys are unique and properly defined
	keys := []ContextKey{
		ContextKeySession,
		ContextKeyUserID,
		ContextKeyUserEmail,
		ContextKeyUserRole,
	}

	seen := make(map[ContextKey]bool)
	for _, key := range keys {
		if seen[key] {
			t.Errorf("duplicate context key: %s", key)
		}
		seen[key] = true

		if key == "" {
			t.Error("context key should not be empty")
		}
	}
}

func TestAuthenticate_InvalidAuthHeaderFormat(t *testing.T) {
	authSvc := createTestAuthService(t, true)
	middleware := NewAuthMiddleware(authSvc)

	testCases := []struct {
		name   string
		header string
	}{
		{"basic auth", "Basic dXNlcm5hbWU6cGFzc3dvcmQ="},
		{"missing space", "Bearerabc123"},
		{"extra spaces", "Bearer  abc123"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			handlerCalled := false
			handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				handlerCalled = true
				sess := GetSessionFromContext(r.Context())
				if sess != nil {
					t.Error("expected no session with invalid auth header format")
				}
				w.WriteHeader(http.StatusOK)
			})

			req := httptest.NewRequest("GET", "/test", nil)
			req.Header.Set("Authorization", tc.header)
			rr := httptest.NewRecorder()

			middleware.Authenticate(handler).ServeHTTP(rr, req)

			if !handlerCalled {
				t.Error("expected handler to be called")
			}
		})
	}
}
