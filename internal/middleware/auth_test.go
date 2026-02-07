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
		ContextKeyDevice,
		ContextKeyDevicePermissions,
		ContextKeyUserGroups,
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

// createTestDBWithDevice creates a test database with Device model migrated.
func createTestDBWithDevice(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("failed to create test database: %v", err)
	}

	// Run migrations for all required tables including group membership tables
	err = db.AutoMigrate(
		&models.Session{}, &models.User{}, &models.Device{},
		&models.UserGroup{}, &models.UserGroupMember{}, &models.DeviceGroupMember{},
	)
	if err != nil {
		t.Fatalf("failed to migrate database: %v", err)
	}

	return db
}

// createTestAuthServiceWithDeviceAuth creates an auth service with device auth enabled.
func createTestAuthServiceWithDeviceAuth(t *testing.T, enabled, deviceAuthEnabled bool) (*auth.Service, *gorm.DB) {
	t.Helper()
	db := createTestDBWithDevice(t)

	svc, err := auth.NewService(auth.Config{
		DB:                 db,
		JWTSecret:          "test-secret-key-at-least-32-chars",
		JWTIssuer:          "test-issuer",
		JWTAccessTokenTTL:  15 * time.Minute,
		JWTRefreshTokenTTL: 24 * time.Hour,
		PasswordMinLength:  8,
		Enabled:            enabled,
		DeviceAuthEnabled:  deviceAuthEnabled,
	})
	if err != nil {
		t.Fatalf("failed to create auth service: %v", err)
	}
	return svc, db
}

// createTestDevice creates a device in the database for testing.
func createTestDevice(t *testing.T, db *gorm.DB, id, fingerprint, status, permissions string) *models.Device {
	t.Helper()
	device := &models.Device{
		ID:          id,
		Fingerprint: fingerprint,
		Name:        "Test Device",
		Status:      status,
		Permissions: permissions,
	}
	if err := db.Create(device).Error; err != nil {
		t.Fatalf("failed to create test device: %v", err)
	}
	return device
}

func TestNewAuthMiddlewareWithDB(t *testing.T) {
	authSvc, db := createTestAuthServiceWithDeviceAuth(t, true, true)
	middleware := NewAuthMiddlewareWithDB(authSvc, db)

	if middleware == nil {
		t.Fatal("expected middleware to be non-nil")
	}

	if middleware.authService != authSvc {
		t.Error("expected authService to be set correctly")
	}

	if middleware.db != db {
		t.Error("expected db to be set correctly")
	}
}

func TestAuthenticate_DeviceFingerprint_Approved(t *testing.T) {
	authSvc, db := createTestAuthServiceWithDeviceAuth(t, true, true)
	middleware := NewAuthMiddlewareWithDB(authSvc, db)

	// Create an approved device
	device := createTestDevice(t, db, "device-123", "test-fingerprint-abc", models.DeviceStatusApproved, models.DevicePermissionsOperator)

	handlerCalled := false
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		handlerCalled = true

		// Check device is in context
		ctxDevice := GetDeviceFromContext(r.Context())
		if ctxDevice == nil {
			t.Error("expected device in context")
			return
		}

		if ctxDevice.ID != device.ID {
			t.Errorf("expected device ID %s, got %s", device.ID, ctxDevice.ID)
		}

		// Check permissions in context
		permissions := GetDevicePermissionsFromContext(r.Context())
		if permissions != models.DevicePermissionsOperator {
			t.Errorf("expected permissions %s, got %s", models.DevicePermissionsOperator, permissions)
		}

		// Check role mapping
		role := GetUserRoleFromContext(r.Context())
		if role != "OPERATOR" {
			t.Errorf("expected role OPERATOR, got %s", role)
		}

		// Should NOT have session when authenticated via device
		sess := GetSessionFromContext(r.Context())
		if sess != nil {
			t.Error("expected no session when authenticated via device")
		}

		w.WriteHeader(http.StatusOK)
	})

	req := httptest.NewRequest("GET", "/test", nil)
	req.Header.Set("X-Device-Fingerprint", "test-fingerprint-abc")
	rr := httptest.NewRecorder()

	middleware.Authenticate(handler).ServeHTTP(rr, req)

	if !handlerCalled {
		t.Error("expected handler to be called")
	}

	if rr.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", rr.Code)
	}
}

func TestAuthenticate_DeviceFingerprint_Pending(t *testing.T) {
	authSvc, db := createTestAuthServiceWithDeviceAuth(t, true, true)
	middleware := NewAuthMiddlewareWithDB(authSvc, db)

	// Create a pending device
	createTestDevice(t, db, "device-123", "pending-fingerprint", models.DeviceStatusPending, models.DevicePermissionsReadOnly)

	handlerCalled := false
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		handlerCalled = true

		// Pending device should NOT be in context
		ctxDevice := GetDeviceFromContext(r.Context())
		if ctxDevice != nil {
			t.Error("expected no device in context for pending device")
		}

		w.WriteHeader(http.StatusOK)
	})

	req := httptest.NewRequest("GET", "/test", nil)
	req.Header.Set("X-Device-Fingerprint", "pending-fingerprint")
	rr := httptest.NewRecorder()

	middleware.Authenticate(handler).ServeHTTP(rr, req)

	if !handlerCalled {
		t.Error("expected handler to be called")
	}
}

func TestAuthenticate_DeviceFingerprint_Revoked(t *testing.T) {
	authSvc, db := createTestAuthServiceWithDeviceAuth(t, true, true)
	middleware := NewAuthMiddlewareWithDB(authSvc, db)

	// Create a revoked device
	createTestDevice(t, db, "device-123", "revoked-fingerprint", models.DeviceStatusRevoked, models.DevicePermissionsOperator)

	handlerCalled := false
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		handlerCalled = true

		// Revoked device should NOT be in context
		ctxDevice := GetDeviceFromContext(r.Context())
		if ctxDevice != nil {
			t.Error("expected no device in context for revoked device")
		}

		w.WriteHeader(http.StatusOK)
	})

	req := httptest.NewRequest("GET", "/test", nil)
	req.Header.Set("X-Device-Fingerprint", "revoked-fingerprint")
	rr := httptest.NewRecorder()

	middleware.Authenticate(handler).ServeHTTP(rr, req)

	if !handlerCalled {
		t.Error("expected handler to be called")
	}
}

func TestAuthenticate_DeviceFingerprint_NotFound(t *testing.T) {
	authSvc, db := createTestAuthServiceWithDeviceAuth(t, true, true)
	middleware := NewAuthMiddlewareWithDB(authSvc, db)

	handlerCalled := false
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		handlerCalled = true

		// Unknown device should NOT be in context
		ctxDevice := GetDeviceFromContext(r.Context())
		if ctxDevice != nil {
			t.Error("expected no device in context for unknown fingerprint")
		}

		w.WriteHeader(http.StatusOK)
	})

	req := httptest.NewRequest("GET", "/test", nil)
	req.Header.Set("X-Device-Fingerprint", "unknown-fingerprint")
	rr := httptest.NewRecorder()

	middleware.Authenticate(handler).ServeHTTP(rr, req)

	if !handlerCalled {
		t.Error("expected handler to be called")
	}
}

func TestAuthenticate_DeviceAuth_Disabled(t *testing.T) {
	// Auth enabled but device auth disabled
	authSvc, db := createTestAuthServiceWithDeviceAuth(t, true, false)
	middleware := NewAuthMiddlewareWithDB(authSvc, db)

	// Create an approved device (won't be used since device auth is disabled)
	createTestDevice(t, db, "device-123", "test-fingerprint", models.DeviceStatusApproved, models.DevicePermissionsAdmin)

	handlerCalled := false
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		handlerCalled = true

		// Device should NOT be in context when device auth is disabled
		ctxDevice := GetDeviceFromContext(r.Context())
		if ctxDevice != nil {
			t.Error("expected no device when device auth is disabled")
		}

		w.WriteHeader(http.StatusOK)
	})

	req := httptest.NewRequest("GET", "/test", nil)
	req.Header.Set("X-Device-Fingerprint", "test-fingerprint")
	rr := httptest.NewRecorder()

	middleware.Authenticate(handler).ServeHTTP(rr, req)

	if !handlerCalled {
		t.Error("expected handler to be called")
	}
}

func TestAuthenticate_DeviceWithDefaultUser(t *testing.T) {
	authSvc, db := createTestAuthServiceWithDeviceAuth(t, true, true)
	middleware := NewAuthMiddlewareWithDB(authSvc, db)

	// Create a user first
	user := &models.User{
		ID:    "user-123",
		Email: "device-user@example.com",
	}
	if err := db.Create(user).Error; err != nil {
		t.Fatalf("failed to create user: %v", err)
	}

	// Create an approved device with default user
	userID := "user-123"
	device := &models.Device{
		ID:            "device-456",
		Fingerprint:   "fingerprint-with-user",
		Name:          "Device with User",
		Status:        models.DeviceStatusApproved,
		Permissions:   models.DevicePermissionsAdmin,
		DefaultUserID: &userID,
	}
	if err := db.Create(device).Error; err != nil {
		t.Fatalf("failed to create device: %v", err)
	}

	handlerCalled := false
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		handlerCalled = true

		// Check user ID is set from default user
		userIDFromCtx := GetUserIDFromContext(r.Context())
		if userIDFromCtx != "user-123" {
			t.Errorf("expected user ID user-123, got %s", userIDFromCtx)
		}

		w.WriteHeader(http.StatusOK)
	})

	req := httptest.NewRequest("GET", "/test", nil)
	req.Header.Set("X-Device-Fingerprint", "fingerprint-with-user")
	rr := httptest.NewRecorder()

	middleware.Authenticate(handler).ServeHTTP(rr, req)

	if !handlerCalled {
		t.Error("expected handler to be called")
	}
}

func TestMapDevicePermissionsToRole(t *testing.T) {
	testCases := []struct {
		permissions string
		expected    string
	}{
		{models.DevicePermissionsAdmin, "ADMIN"},
		{models.DevicePermissionsOperator, "OPERATOR"},
		{models.DevicePermissionsReadOnly, "USER"},
		{"UNKNOWN", "USER"}, // Unknown defaults to USER
		{"", "USER"},        // Empty defaults to USER
	}

	for _, tc := range testCases {
		t.Run(tc.permissions, func(t *testing.T) {
			result := mapDevicePermissionsToRole(tc.permissions)
			if result != tc.expected {
				t.Errorf("mapDevicePermissionsToRole(%s) = %s, expected %s", tc.permissions, result, tc.expected)
			}
		})
	}
}

func TestGetDeviceFromContext(t *testing.T) {
	t.Run("device exists", func(t *testing.T) {
		device := &models.Device{
			ID:          "device-123",
			Fingerprint: "test-fp",
			Name:        "Test Device",
			Status:      models.DeviceStatusApproved,
			Permissions: models.DevicePermissionsOperator,
		}
		ctx := context.WithValue(context.Background(), ContextKeyDevice, device)

		result := GetDeviceFromContext(ctx)
		if result == nil {
			t.Fatal("expected device to be returned")
		}

		if result.ID != "device-123" {
			t.Errorf("expected ID device-123, got %s", result.ID)
		}
	})

	t.Run("no device", func(t *testing.T) {
		ctx := context.Background()
		result := GetDeviceFromContext(ctx)
		if result != nil {
			t.Error("expected nil device")
		}
	})

	t.Run("wrong type in context", func(t *testing.T) {
		ctx := context.WithValue(context.Background(), ContextKeyDevice, "not a device")
		result := GetDeviceFromContext(ctx)
		if result != nil {
			t.Error("expected nil for wrong type")
		}
	})
}

func TestGetDevicePermissionsFromContext(t *testing.T) {
	t.Run("permissions exist", func(t *testing.T) {
		ctx := context.WithValue(context.Background(), ContextKeyDevicePermissions, models.DevicePermissionsAdmin)
		result := GetDevicePermissionsFromContext(ctx)
		if result != models.DevicePermissionsAdmin {
			t.Errorf("expected %s, got %s", models.DevicePermissionsAdmin, result)
		}
	})

	t.Run("no permissions", func(t *testing.T) {
		ctx := context.Background()
		result := GetDevicePermissionsFromContext(ctx)
		if result != "" {
			t.Errorf("expected empty string, got %s", result)
		}
	})

	t.Run("wrong type", func(t *testing.T) {
		ctx := context.WithValue(context.Background(), ContextKeyDevicePermissions, 123)
		result := GetDevicePermissionsFromContext(ctx)
		if result != "" {
			t.Errorf("expected empty string for wrong type, got %s", result)
		}
	})
}

func TestIsDeviceAuthenticated(t *testing.T) {
	t.Run("device authenticated", func(t *testing.T) {
		device := &models.Device{ID: "device-123"}
		ctx := context.WithValue(context.Background(), ContextKeyDevice, device)

		if !IsDeviceAuthenticated(ctx) {
			t.Error("expected IsDeviceAuthenticated to return true")
		}
	})

	t.Run("not device authenticated", func(t *testing.T) {
		ctx := context.Background()
		if IsDeviceAuthenticated(ctx) {
			t.Error("expected IsDeviceAuthenticated to return false")
		}
	})
}

func TestIsAuthenticated_WithDevice(t *testing.T) {
	t.Run("authenticated via device", func(t *testing.T) {
		device := &models.Device{ID: "device-123"}
		ctx := context.WithValue(context.Background(), ContextKeyDevice, device)

		if !IsAuthenticated(ctx) {
			t.Error("expected IsAuthenticated to return true for device auth")
		}
	})

	t.Run("authenticated via session and device", func(t *testing.T) {
		sess := &session.CachedSession{UserID: "user123"}
		device := &models.Device{ID: "device-123"}
		ctx := context.WithValue(context.Background(), ContextKeySession, sess)
		ctx = context.WithValue(ctx, ContextKeyDevice, device)

		if !IsAuthenticated(ctx) {
			t.Error("expected IsAuthenticated to return true for both")
		}
	})
}

func TestUpdateDeviceLastSeen(t *testing.T) {
	db := createTestDBWithDevice(t)
	authSvc, _ := createTestAuthServiceWithDeviceAuth(t, true, true)
	middleware := NewAuthMiddlewareWithDB(authSvc, db)

	// Create a device
	device := &models.Device{
		ID:          "device-123",
		Fingerprint: "test-fp",
		Name:        "Test Device",
		Status:      models.DeviceStatusApproved,
		Permissions: models.DevicePermissionsOperator,
	}
	if err := db.Create(device).Error; err != nil {
		t.Fatalf("failed to create device: %v", err)
	}

	// Call updateDeviceLastSeen
	middleware.updateDeviceLastSeen("device-123", "192.168.1.100")

	// Wait a bit for async update
	time.Sleep(100 * time.Millisecond)

	// Verify device was updated
	var updatedDevice models.Device
	if err := db.First(&updatedDevice, "id = ?", "device-123").Error; err != nil {
		t.Fatalf("failed to fetch device: %v", err)
	}

	if updatedDevice.LastSeenAt == nil {
		t.Error("expected LastSeenAt to be set")
	}

	if updatedDevice.LastIPAddress == nil || *updatedDevice.LastIPAddress != "192.168.1.100" {
		t.Errorf("expected LastIPAddress to be 192.168.1.100, got %v", updatedDevice.LastIPAddress)
	}
}

func TestUpdateDeviceLastSeen_NilDB(t *testing.T) {
	authSvc := createTestAuthService(t, true)
	middleware := NewAuthMiddleware(authSvc) // No DB

	// Should not panic
	middleware.updateDeviceLastSeen("device-123", "192.168.1.100")
}

func TestGetDeviceByFingerprint(t *testing.T) {
	db := createTestDBWithDevice(t)
	authSvc, _ := createTestAuthServiceWithDeviceAuth(t, true, true)
	middleware := NewAuthMiddlewareWithDB(authSvc, db)

	// Create a device
	device := &models.Device{
		ID:          "device-123",
		Fingerprint: "test-fingerprint",
		Name:        "Test Device",
		Status:      models.DeviceStatusApproved,
	}
	if err := db.Create(device).Error; err != nil {
		t.Fatalf("failed to create device: %v", err)
	}

	t.Run("device found", func(t *testing.T) {
		result, err := middleware.getDeviceByFingerprint(context.Background(), "test-fingerprint")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if result == nil {
			t.Fatal("expected device to be returned")
		}
		if result.ID != "device-123" {
			t.Errorf("expected ID device-123, got %s", result.ID)
		}
	})

	t.Run("device not found", func(t *testing.T) {
		result, err := middleware.getDeviceByFingerprint(context.Background(), "unknown-fingerprint")
		if err == nil {
			t.Error("expected error for unknown fingerprint")
		}
		if result != nil {
			t.Error("expected nil device for unknown fingerprint")
		}
	})
}

func TestAuthenticate_DeviceAuth_NoDBConfigured(t *testing.T) {
	authSvc, _ := createTestAuthServiceWithDeviceAuth(t, true, true)
	// Create middleware WITHOUT database
	middleware := NewAuthMiddleware(authSvc)

	handlerCalled := false
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		handlerCalled = true
		// Should fall through to no auth when db is nil
		ctxDevice := GetDeviceFromContext(r.Context())
		if ctxDevice != nil {
			t.Error("expected no device when db is nil")
		}
		w.WriteHeader(http.StatusOK)
	})

	req := httptest.NewRequest("GET", "/test", nil)
	req.Header.Set("X-Device-Fingerprint", "test-fingerprint")
	rr := httptest.NewRecorder()

	middleware.Authenticate(handler).ServeHTTP(rr, req)

	if !handlerCalled {
		t.Error("expected handler to be called")
	}
}

func TestAuthenticate_EmptyDeviceFingerprint(t *testing.T) {
	authSvc, db := createTestAuthServiceWithDeviceAuth(t, true, true)
	middleware := NewAuthMiddlewareWithDB(authSvc, db)

	handlerCalled := false
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		handlerCalled = true
		// Empty fingerprint should not set device
		ctxDevice := GetDeviceFromContext(r.Context())
		if ctxDevice != nil {
			t.Error("expected no device for empty fingerprint")
		}
		w.WriteHeader(http.StatusOK)
	})

	req := httptest.NewRequest("GET", "/test", nil)
	req.Header.Set("X-Device-Fingerprint", "")
	rr := httptest.NewRecorder()

	middleware.Authenticate(handler).ServeHTTP(rr, req)

	if !handlerCalled {
		t.Error("expected handler to be called")
	}
}

func TestUpdateDeviceLastSeen_DatabaseError(t *testing.T) {
	db := createTestDBWithDevice(t)
	authSvc, _ := createTestAuthServiceWithDeviceAuth(t, true, true)
	middleware := NewAuthMiddlewareWithDB(authSvc, db)

	// Close the database to cause an error
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatalf("failed to get underlying DB: %v", err)
	}
	_ = sqlDB.Close() // Intentionally ignore error as we want the DB closed

	// Call updateDeviceLastSeen - should log error but not panic
	middleware.updateDeviceLastSeen("nonexistent-device", "192.168.1.100")

	// Test passes if no panic occurred
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

// --- Phase 2: Group context helper tests ---

func TestGetUserGroupsFromContext(t *testing.T) {
	t.Run("groups exist", func(t *testing.T) {
		groups := []UserGroupMembership{
			{GroupID: "group-1", Role: "MEMBER"},
			{GroupID: "group-2", Role: "GROUP_ADMIN"},
		}
		ctx := context.WithValue(context.Background(), ContextKeyUserGroups, groups)

		result := GetUserGroupsFromContext(ctx)
		if len(result) != 2 {
			t.Fatalf("expected 2 groups, got %d", len(result))
		}
		if result[0].GroupID != "group-1" {
			t.Errorf("expected group-1, got %s", result[0].GroupID)
		}
		if result[1].Role != "GROUP_ADMIN" {
			t.Errorf("expected GROUP_ADMIN, got %s", result[1].Role)
		}
	})

	t.Run("no groups", func(t *testing.T) {
		ctx := context.Background()
		result := GetUserGroupsFromContext(ctx)
		if result != nil {
			t.Errorf("expected nil, got %v", result)
		}
	})

	t.Run("wrong type in context", func(t *testing.T) {
		ctx := context.WithValue(context.Background(), ContextKeyUserGroups, "not groups")
		result := GetUserGroupsFromContext(ctx)
		if result != nil {
			t.Errorf("expected nil for wrong type, got %v", result)
		}
	})

	t.Run("empty slice", func(t *testing.T) {
		groups := []UserGroupMembership{}
		ctx := context.WithValue(context.Background(), ContextKeyUserGroups, groups)
		result := GetUserGroupsFromContext(ctx)
		if len(result) != 0 {
			t.Errorf("expected empty slice, got %d items", len(result))
		}
	})
}

func TestGetUserGroupIDs(t *testing.T) {
	t.Run("has groups", func(t *testing.T) {
		groups := []UserGroupMembership{
			{GroupID: "group-1", Role: "MEMBER"},
			{GroupID: "group-2", Role: "GROUP_ADMIN"},
			{GroupID: "group-3", Role: "MEMBER"},
		}
		ctx := context.WithValue(context.Background(), ContextKeyUserGroups, groups)

		ids := GetUserGroupIDs(ctx)
		if len(ids) != 3 {
			t.Fatalf("expected 3 IDs, got %d", len(ids))
		}
		if ids[0] != "group-1" || ids[1] != "group-2" || ids[2] != "group-3" {
			t.Errorf("unexpected IDs: %v", ids)
		}
	})

	t.Run("no groups", func(t *testing.T) {
		ctx := context.Background()
		ids := GetUserGroupIDs(ctx)
		if len(ids) != 0 {
			t.Errorf("expected empty slice, got %v", ids)
		}
	})
}

func TestIsGroupMember(t *testing.T) {
	groups := []UserGroupMembership{
		{GroupID: "group-1", Role: "MEMBER"},
		{GroupID: "group-2", Role: "GROUP_ADMIN"},
	}
	ctx := context.WithValue(context.Background(), ContextKeyUserGroups, groups)

	t.Run("is member", func(t *testing.T) {
		if !IsGroupMember(ctx, "group-1") {
			t.Error("expected true for group-1 membership")
		}
	})

	t.Run("is member as admin", func(t *testing.T) {
		if !IsGroupMember(ctx, "group-2") {
			t.Error("expected true for group-2 membership (GROUP_ADMIN is also a member)")
		}
	})

	t.Run("not member", func(t *testing.T) {
		if IsGroupMember(ctx, "group-999") {
			t.Error("expected false for non-member group")
		}
	})

	t.Run("no groups in context", func(t *testing.T) {
		emptyCtx := context.Background()
		if IsGroupMember(emptyCtx, "group-1") {
			t.Error("expected false when no groups in context")
		}
	})
}

func TestIsGroupAdmin(t *testing.T) {
	groups := []UserGroupMembership{
		{GroupID: "group-1", Role: "MEMBER"},
		{GroupID: "group-2", Role: "GROUP_ADMIN"},
	}
	ctx := context.WithValue(context.Background(), ContextKeyUserGroups, groups)

	t.Run("is group admin", func(t *testing.T) {
		if !IsGroupAdmin(ctx, "group-2") {
			t.Error("expected true for group-2 admin")
		}
	})

	t.Run("not group admin (is member)", func(t *testing.T) {
		if IsGroupAdmin(ctx, "group-1") {
			t.Error("expected false for group-1 (MEMBER, not GROUP_ADMIN)")
		}
	})

	t.Run("not member at all", func(t *testing.T) {
		if IsGroupAdmin(ctx, "group-999") {
			t.Error("expected false for non-member group")
		}
	})

	t.Run("no groups in context", func(t *testing.T) {
		emptyCtx := context.Background()
		if IsGroupAdmin(emptyCtx, "group-2") {
			t.Error("expected false when no groups in context")
		}
	})
}

func TestLoadUserGroupMemberships(t *testing.T) {
	t.Run("loads memberships", func(t *testing.T) {
		db := createTestDBWithDevice(t)
		authSvc, _ := createTestAuthServiceWithDeviceAuth(t, true, true)
		mw := NewAuthMiddlewareWithDB(authSvc, db)

		// Create group memberships
		members := []models.UserGroupMember{
			{ID: "m1", UserID: "user-1", GroupID: "group-1", Role: "MEMBER"},
			{ID: "m2", UserID: "user-1", GroupID: "group-2", Role: "GROUP_ADMIN"},
		}
		for _, m := range members {
			if err := db.Create(&m).Error; err != nil {
				t.Fatalf("failed to create member: %v", err)
			}
		}

		result := mw.loadUserGroupMemberships(context.Background(), "user-1")
		if len(result) != 2 {
			t.Fatalf("expected 2 memberships, got %d", len(result))
		}

		// Verify contents (order may vary)
		found := map[string]string{}
		for _, r := range result {
			found[r.GroupID] = r.Role
		}
		if found["group-1"] != "MEMBER" {
			t.Errorf("expected group-1 role MEMBER, got %s", found["group-1"])
		}
		if found["group-2"] != "GROUP_ADMIN" {
			t.Errorf("expected group-2 role GROUP_ADMIN, got %s", found["group-2"])
		}
	})

	t.Run("no memberships", func(t *testing.T) {
		db := createTestDBWithDevice(t)
		authSvc, _ := createTestAuthServiceWithDeviceAuth(t, true, true)
		mw := NewAuthMiddlewareWithDB(authSvc, db)

		result := mw.loadUserGroupMemberships(context.Background(), "user-nonexistent")
		if len(result) != 0 {
			t.Errorf("expected 0 memberships, got %d", len(result))
		}
	})

	t.Run("nil db", func(t *testing.T) {
		authSvc := createTestAuthService(t, true)
		mw := NewAuthMiddleware(authSvc)

		result := mw.loadUserGroupMemberships(context.Background(), "user-1")
		if result != nil {
			t.Errorf("expected nil with nil db, got %v", result)
		}
	})

	t.Run("empty userID", func(t *testing.T) {
		db := createTestDBWithDevice(t)
		authSvc, _ := createTestAuthServiceWithDeviceAuth(t, true, true)
		mw := NewAuthMiddlewareWithDB(authSvc, db)

		result := mw.loadUserGroupMemberships(context.Background(), "")
		if result != nil {
			t.Errorf("expected nil for empty userID, got %v", result)
		}
	})
}

func TestLoadDeviceGroupMemberships(t *testing.T) {
	t.Run("loads memberships", func(t *testing.T) {
		db := createTestDBWithDevice(t)
		authSvc, _ := createTestAuthServiceWithDeviceAuth(t, true, true)
		mw := NewAuthMiddlewareWithDB(authSvc, db)

		// Create device group memberships
		members := []models.DeviceGroupMember{
			{ID: "dgm1", DeviceID: "device-1", GroupID: "group-1"},
			{ID: "dgm2", DeviceID: "device-1", GroupID: "group-2"},
		}
		for _, m := range members {
			if err := db.Create(&m).Error; err != nil {
				t.Fatalf("failed to create device group member: %v", err)
			}
		}

		result := mw.loadDeviceGroupMemberships(context.Background(), "device-1")
		if len(result) != 2 {
			t.Fatalf("expected 2 memberships, got %d", len(result))
		}

		// All device memberships should have MEMBER role
		for _, r := range result {
			if r.Role != "MEMBER" {
				t.Errorf("expected MEMBER role for device group membership, got %s", r.Role)
			}
		}
	})

	t.Run("no memberships", func(t *testing.T) {
		db := createTestDBWithDevice(t)
		authSvc, _ := createTestAuthServiceWithDeviceAuth(t, true, true)
		mw := NewAuthMiddlewareWithDB(authSvc, db)

		result := mw.loadDeviceGroupMemberships(context.Background(), "device-nonexistent")
		if len(result) != 0 {
			t.Errorf("expected 0 memberships, got %d", len(result))
		}
	})

	t.Run("nil db", func(t *testing.T) {
		authSvc := createTestAuthService(t, true)
		mw := NewAuthMiddleware(authSvc)

		result := mw.loadDeviceGroupMemberships(context.Background(), "device-1")
		if result != nil {
			t.Errorf("expected nil with nil db, got %v", result)
		}
	})

	t.Run("empty deviceID", func(t *testing.T) {
		db := createTestDBWithDevice(t)
		authSvc, _ := createTestAuthServiceWithDeviceAuth(t, true, true)
		mw := NewAuthMiddlewareWithDB(authSvc, db)

		result := mw.loadDeviceGroupMemberships(context.Background(), "")
		if result != nil {
			t.Errorf("expected nil for empty deviceID, got %v", result)
		}
	})
}

func TestAuthenticate_DeviceWithGroupMemberships(t *testing.T) {
	authSvc, db := createTestAuthServiceWithDeviceAuth(t, true, true)
	mw := NewAuthMiddlewareWithDB(authSvc, db)

	// Create an approved device
	createTestDevice(t, db, "device-grp", "fp-groups", models.DeviceStatusApproved, models.DevicePermissionsOperator)

	// Create device group memberships
	for _, dgm := range []models.DeviceGroupMember{
		{ID: "dgm-1", DeviceID: "device-grp", GroupID: "group-a"},
		{ID: "dgm-2", DeviceID: "device-grp", GroupID: "group-b"},
	} {
		if err := db.Create(&dgm).Error; err != nil {
			t.Fatalf("failed to create device group member: %v", err)
		}
	}

	handlerCalled := false
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		handlerCalled = true

		// Verify group memberships are in context
		groups := GetUserGroupsFromContext(r.Context())
		if len(groups) != 2 {
			t.Errorf("expected 2 group memberships, got %d", len(groups))
		}

		// Verify IsGroupMember works
		if !IsGroupMember(r.Context(), "group-a") {
			t.Error("expected device to be member of group-a")
		}
		if !IsGroupMember(r.Context(), "group-b") {
			t.Error("expected device to be member of group-b")
		}
		if IsGroupMember(r.Context(), "group-c") {
			t.Error("expected device NOT to be member of group-c")
		}

		w.WriteHeader(http.StatusOK)
	})

	req := httptest.NewRequest("GET", "/test", nil)
	req.Header.Set("X-Device-Fingerprint", "fp-groups")
	rr := httptest.NewRecorder()

	mw.Authenticate(handler).ServeHTTP(rr, req)

	if !handlerCalled {
		t.Error("expected handler to be called")
	}
}

func TestAuthenticate_UserWithGroupMemberships(t *testing.T) {
	authSvc, db := createTestAuthServiceWithDB(t, true)

	// Also migrate group tables
	if err := db.AutoMigrate(&models.UserGroup{}, &models.UserGroupMember{}); err != nil {
		t.Fatalf("failed to migrate group tables: %v", err)
	}

	mw := NewAuthMiddlewareWithDB(authSvc, db)

	// Create a session in the database
	createTestSession(t, db, "sess-grp", "user-grp")

	// Create user group memberships
	for _, ugm := range []models.UserGroupMember{
		{ID: "ugm-1", UserID: "user-grp", GroupID: "group-x", Role: "MEMBER"},
		{ID: "ugm-2", UserID: "user-grp", GroupID: "group-y", Role: "GROUP_ADMIN"},
	} {
		if err := db.Create(&ugm).Error; err != nil {
			t.Fatalf("failed to create user group member: %v", err)
		}
	}

	// Generate a valid token
	jwtSvc := authSvc.JWTService()
	pair, err := jwtSvc.GenerateTokenPair("user-grp", "grp@example.com", "USER", "sess-grp")
	if err != nil {
		t.Fatalf("failed to generate token pair: %v", err)
	}

	handlerCalled := false
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		handlerCalled = true

		// Verify group memberships are in context
		groups := GetUserGroupsFromContext(r.Context())
		if len(groups) != 2 {
			t.Errorf("expected 2 group memberships, got %d", len(groups))
		}

		// Verify IsGroupMember works
		if !IsGroupMember(r.Context(), "group-x") {
			t.Error("expected user to be member of group-x")
		}
		if !IsGroupMember(r.Context(), "group-y") {
			t.Error("expected user to be member of group-y")
		}

		// Verify IsGroupAdmin works
		if IsGroupAdmin(r.Context(), "group-x") {
			t.Error("expected user NOT to be admin of group-x")
		}
		if !IsGroupAdmin(r.Context(), "group-y") {
			t.Error("expected user to be admin of group-y")
		}

		// Verify GetUserGroupIDs
		ids := GetUserGroupIDs(r.Context())
		if len(ids) != 2 {
			t.Errorf("expected 2 group IDs, got %d", len(ids))
		}

		w.WriteHeader(http.StatusOK)
	})

	req := httptest.NewRequest("GET", "/test", nil)
	req.Header.Set("Authorization", "Bearer "+pair.AccessToken)
	rr := httptest.NewRecorder()

	mw.Authenticate(handler).ServeHTTP(rr, req)

	if !handlerCalled {
		t.Error("expected handler to be called")
	}
}
