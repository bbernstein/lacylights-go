package auth

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/bbernstein/lacylights-go/internal/database/models"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func setupTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		t.Fatalf("failed to connect to test database: %v", err)
	}

	// Migrate the schema - include Session for full auth tests, and group tables for personal group creation
	if err := db.AutoMigrate(&models.User{}, &models.UserCredential{}, &models.Session{}, &models.UserGroup{}, &models.UserGroupMember{}); err != nil {
		t.Fatalf("failed to migrate test database: %v", err)
	}

	return db
}

func TestEnsureDefaultAdmin_CreatesAdminWhenNoUsers(t *testing.T) {
	db := setupTestDB(t)
	ctx := context.Background()

	service, err := NewService(Config{
		DB:                db,
		JWTSecret:         "test-secret-key-at-least-32-chars",
		JWTIssuer:         "test",
		JWTAccessTokenTTL: 15 * time.Minute,
		JWTRefreshTokenTTL: 24 * time.Hour,
		Enabled:           true,
		PasswordMinLength: 8,
	})
	if err != nil {
		t.Fatalf("failed to create auth service: %v", err)
	}

	// Ensure default admin
	err = service.EnsureDefaultAdmin(ctx, "admin@test.local", "password123")
	if err != nil {
		t.Fatalf("EnsureDefaultAdmin failed: %v", err)
	}

	// Verify user was created
	var user models.User
	if err := db.First(&user, "email = ?", "admin@test.local").Error; err != nil {
		t.Fatalf("admin user not found: %v", err)
	}

	if user.Role != "ADMIN" {
		t.Errorf("expected role ADMIN, got %s", user.Role)
	}

	if !user.IsActive {
		t.Error("expected user to be active")
	}

	// Verify credentials were created
	var creds models.UserCredential
	if err := db.First(&creds, "user_id = ?", user.ID).Error; err != nil {
		t.Fatalf("credentials not found: %v", err)
	}

	// Verify password can be verified
	if err := VerifyPassword("password123", creds.PasswordHash); err != nil {
		t.Error("password verification failed")
	}

	// Verify personal group was created
	var group models.UserGroup
	if err := db.First(&group, "owner_id = ?", user.ID).Error; err != nil {
		t.Fatalf("personal group not found: %v", err)
	}
	if !group.IsPersonal {
		t.Error("expected group to be personal")
	}
	if group.Name != "admin@test.local's Group" {
		t.Errorf("expected group name %q, got %q", "admin@test.local's Group", group.Name)
	}

	// Verify user is GROUP_ADMIN of the personal group
	var member models.UserGroupMember
	if err := db.First(&member, "user_id = ? AND group_id = ?", user.ID, group.ID).Error; err != nil {
		t.Fatalf("group membership not found: %v", err)
	}
	if member.Role != models.GroupRoleGroupAdmin {
		t.Errorf("expected role %s, got %s", models.GroupRoleGroupAdmin, member.Role)
	}
}

func TestEnsureDefaultAdmin_NoOpWhenUsersExist(t *testing.T) {
	db := setupTestDB(t)
	ctx := context.Background()

	// Create an existing user
	existingUser := models.User{
		ID:       "existing-user",
		Email:    "existing@test.local",
		Role:     "USER",
		IsActive: true,
	}
	if err := db.Create(&existingUser).Error; err != nil {
		t.Fatalf("failed to create existing user: %v", err)
	}

	service, err := NewService(Config{
		DB:                db,
		JWTSecret:         "test-secret-key-at-least-32-chars",
		JWTIssuer:         "test",
		JWTAccessTokenTTL: 15 * time.Minute,
		JWTRefreshTokenTTL: 24 * time.Hour,
		Enabled:           true,
		PasswordMinLength: 8,
	})
	if err != nil {
		t.Fatalf("failed to create auth service: %v", err)
	}

	// Ensure default admin should not create a new user
	err = service.EnsureDefaultAdmin(ctx, "admin@test.local", "password123")
	if err != nil {
		t.Fatalf("EnsureDefaultAdmin failed: %v", err)
	}

	// Verify only the existing user exists (no admin was created)
	var count int64
	db.Model(&models.User{}).Count(&count)
	if count != 1 {
		t.Errorf("expected 1 user, got %d", count)
	}

	// Verify the admin user was NOT created
	var admin models.User
	err = db.First(&admin, "email = ?", "admin@test.local").Error
	if err == nil {
		t.Error("expected admin user to not be created when users already exist")
	}
}

func TestEnsureDefaultAdmin_NoOpWhenAuthDisabled(t *testing.T) {
	db := setupTestDB(t)
	ctx := context.Background()

	service, err := NewService(Config{
		DB:                db,
		JWTSecret:         "test-secret-key-at-least-32-chars",
		JWTIssuer:         "test",
		JWTAccessTokenTTL: 15 * time.Minute,
		JWTRefreshTokenTTL: 24 * time.Hour,
		Enabled:           false, // Auth disabled
		PasswordMinLength: 8,
	})
	if err != nil {
		t.Fatalf("failed to create auth service: %v", err)
	}

	// Ensure default admin should be a no-op
	err = service.EnsureDefaultAdmin(ctx, "admin@test.local", "password123")
	if err != nil {
		t.Fatalf("EnsureDefaultAdmin failed: %v", err)
	}

	// Verify no users were created
	var count int64
	db.Model(&models.User{}).Count(&count)
	if count != 0 {
		t.Errorf("expected 0 users, got %d", count)
	}
}

func TestEnsureDefaultAdmin_FailsWithoutPassword(t *testing.T) {
	db := setupTestDB(t)
	ctx := context.Background()

	service, err := NewService(Config{
		DB:                db,
		JWTSecret:         "test-secret-key-at-least-32-chars",
		JWTIssuer:         "test",
		JWTAccessTokenTTL: 15 * time.Minute,
		JWTRefreshTokenTTL: 24 * time.Hour,
		Enabled:           true,
		PasswordMinLength: 8,
	})
	if err != nil {
		t.Fatalf("failed to create auth service: %v", err)
	}

	// Ensure default admin should fail without password
	err = service.EnsureDefaultAdmin(ctx, "admin@test.local", "")
	if err == nil {
		t.Error("expected error when password is empty")
	}
}

func TestEnsureDefaultAdmin_FailsWithShortPassword(t *testing.T) {
	db := setupTestDB(t)
	ctx := context.Background()

	service, err := NewService(Config{
		DB:                db,
		JWTSecret:         "test-secret-key-at-least-32-chars",
		JWTIssuer:         "test",
		JWTAccessTokenTTL: 15 * time.Minute,
		JWTRefreshTokenTTL: 24 * time.Hour,
		Enabled:           true,
		PasswordMinLength: 8,
	})
	if err != nil {
		t.Fatalf("failed to create auth service: %v", err)
	}

	// Ensure default admin should fail with short password
	err = service.EnsureDefaultAdmin(ctx, "admin@test.local", "short")
	if err == nil {
		t.Error("expected error when password is too short")
	}
}

// TestHashPassword tests password hashing and verification.
func TestHashPassword(t *testing.T) {
	password := "testPassword123!"

	hash, err := HashPassword(password)
	if err != nil {
		t.Fatalf("HashPassword failed: %v", err)
	}

	// Verify hash format (Argon2id PHC format)
	if !strings.HasPrefix(hash, "$argon2id$") {
		t.Errorf("hash should start with $argon2id$, got: %s", hash)
	}

	// Verify password matches
	if err := VerifyPassword(password, hash); err != nil {
		t.Errorf("VerifyPassword should succeed for correct password: %v", err)
	}

	// Verify wrong password fails
	if err := VerifyPassword("wrongPassword", hash); err == nil {
		t.Error("VerifyPassword should fail for wrong password")
	}
}

// TestHashPassword_DifferentSalts tests that same password produces different hashes.
func TestHashPassword_DifferentSalts(t *testing.T) {
	password := "testPassword123!"

	hash1, err := HashPassword(password)
	if err != nil {
		t.Fatalf("HashPassword failed: %v", err)
	}

	hash2, err := HashPassword(password)
	if err != nil {
		t.Fatalf("HashPassword failed: %v", err)
	}

	if hash1 == hash2 {
		t.Error("same password should produce different hashes due to random salt")
	}

	// Both hashes should still verify
	if err := VerifyPassword(password, hash1); err != nil {
		t.Errorf("hash1 verification failed: %v", err)
	}
	if err := VerifyPassword(password, hash2); err != nil {
		t.Errorf("hash2 verification failed: %v", err)
	}
}

// TestVerifyPassword_InvalidHash tests verification with invalid hashes.
func TestVerifyPassword_InvalidHash(t *testing.T) {
	testCases := []struct {
		name string
		hash string
	}{
		{"empty hash", ""},
		{"invalid format", "notahash"},
		{"wrong algorithm", "$argon2i$v=19$m=65536,t=1,p=4$salt$hash"},
		{"missing parts", "$argon2id$v=19$m=65536"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			err := VerifyPassword("password", tc.hash)
			if err == nil {
				t.Error("expected error for invalid hash")
			}
		})
	}
}

// TestGenerateSecureToken tests secure token generation.
func TestGenerateSecureToken(t *testing.T) {
	token1, err := GenerateSecureToken(32)
	if err != nil {
		t.Fatalf("GenerateSecureToken failed: %v", err)
	}

	token2, err := GenerateSecureToken(32)
	if err != nil {
		t.Fatalf("GenerateSecureToken failed: %v", err)
	}

	if token1 == token2 {
		t.Error("tokens should be unique")
	}

	// Tokens should be base64 encoded
	if len(token1) == 0 {
		t.Error("token should not be empty")
	}
}

// TestHashToken tests token hashing.
func TestHashToken(t *testing.T) {
	token := "test-token-12345"

	hash1 := HashToken(token)
	hash2 := HashToken(token)

	// Same token should produce same hash (deterministic)
	if hash1 != hash2 {
		t.Error("HashToken should be deterministic")
	}

	// Different tokens should produce different hashes
	hash3 := HashToken("different-token")
	if hash1 == hash3 {
		t.Error("different tokens should produce different hashes")
	}
}

// TestRegister tests user registration.
func TestRegister(t *testing.T) {
	db := setupTestDB(t)
	ctx := context.Background()

	service, err := NewService(Config{
		DB:                db,
		JWTSecret:         "test-secret-key-at-least-32-chars",
		JWTIssuer:         "test",
		JWTAccessTokenTTL: 15 * time.Minute,
		JWTRefreshTokenTTL: 24 * time.Hour,
		Enabled:           true,
		PasswordMinLength: 8,
	})
	if err != nil {
		t.Fatalf("failed to create auth service: %v", err)
	}

	name := "Test User"
	result, err := service.Register(ctx, RegisterInput{
		Email:    "test@example.com",
		Password: "password123",
		Name:     &name,
	}, nil, nil)

	if err != nil {
		t.Fatalf("Register failed: %v", err)
	}

	if result.User == nil {
		t.Fatal("result.User should not be nil")
	}

	if result.User.Email != "test@example.com" {
		t.Errorf("expected email test@example.com, got %s", result.User.Email)
	}

	if result.User.Name == nil || *result.User.Name != "Test User" {
		t.Error("user name should be 'Test User'")
	}

	if result.AccessToken == "" {
		t.Error("access token should not be empty")
	}

	if result.RefreshToken == "" {
		t.Error("refresh token should not be empty")
	}

	if result.SessionID == "" {
		t.Error("session ID should not be empty")
	}

	// Verify personal group was created
	var group models.UserGroup
	if err := db.First(&group, "owner_id = ?", result.User.ID).Error; err != nil {
		t.Fatalf("personal group not found: %v", err)
	}
	if !group.IsPersonal {
		t.Error("expected group to be personal")
	}

	// Verify user is GROUP_ADMIN of the personal group
	var member models.UserGroupMember
	if err := db.First(&member, "user_id = ? AND group_id = ?", result.User.ID, group.ID).Error; err != nil {
		t.Fatalf("group membership not found: %v", err)
	}
	if member.Role != models.GroupRoleGroupAdmin {
		t.Errorf("expected role %s, got %s", models.GroupRoleGroupAdmin, member.Role)
	}
}

// TestRegister_DuplicateEmail tests that duplicate email registration fails.
func TestRegister_DuplicateEmail(t *testing.T) {
	db := setupTestDB(t)
	ctx := context.Background()

	service, err := NewService(Config{
		DB:                db,
		JWTSecret:         "test-secret-key-at-least-32-chars",
		JWTIssuer:         "test",
		JWTAccessTokenTTL: 15 * time.Minute,
		JWTRefreshTokenTTL: 24 * time.Hour,
		Enabled:           true,
		PasswordMinLength: 8,
	})
	if err != nil {
		t.Fatalf("failed to create auth service: %v", err)
	}

	// Register first user
	_, err = service.Register(ctx, RegisterInput{
		Email:    "test@example.com",
		Password: "password123",
	}, nil, nil)
	if err != nil {
		t.Fatalf("first registration failed: %v", err)
	}

	// Try to register with same email
	_, err = service.Register(ctx, RegisterInput{
		Email:    "test@example.com",
		Password: "password456",
	}, nil, nil)

	if err != ErrUserExists {
		t.Errorf("expected ErrUserExists, got: %v", err)
	}
}

// TestRegister_InvalidEmail tests that invalid email fails.
func TestRegister_InvalidEmail(t *testing.T) {
	db := setupTestDB(t)
	ctx := context.Background()

	service, err := NewService(Config{
		DB:                db,
		JWTSecret:         "test-secret-key-at-least-32-chars",
		JWTIssuer:         "test",
		JWTAccessTokenTTL: 15 * time.Minute,
		JWTRefreshTokenTTL: 24 * time.Hour,
		Enabled:           true,
		PasswordMinLength: 8,
	})
	if err != nil {
		t.Fatalf("failed to create auth service: %v", err)
	}

	invalidEmails := []string{
		"notanemail",
		"missing@domain",
		"@nodomain.com",
		"spaces in@email.com",
	}

	for _, email := range invalidEmails {
		t.Run(email, func(t *testing.T) {
			_, err = service.Register(ctx, RegisterInput{
				Email:    email,
				Password: "password123",
			}, nil, nil)

			if err != ErrInvalidEmail {
				t.Errorf("expected ErrInvalidEmail for %s, got: %v", email, err)
			}
		})
	}
}

// TestRegister_ShortPassword tests that short password fails.
func TestRegister_ShortPassword(t *testing.T) {
	db := setupTestDB(t)
	ctx := context.Background()

	service, err := NewService(Config{
		DB:                db,
		JWTSecret:         "test-secret-key-at-least-32-chars",
		JWTIssuer:         "test",
		JWTAccessTokenTTL: 15 * time.Minute,
		JWTRefreshTokenTTL: 24 * time.Hour,
		Enabled:           true,
		PasswordMinLength: 8,
	})
	if err != nil {
		t.Fatalf("failed to create auth service: %v", err)
	}

	_, err = service.Register(ctx, RegisterInput{
		Email:    "test@example.com",
		Password: "short",
	}, nil, nil)

	if err != ErrPasswordTooShort {
		t.Errorf("expected ErrPasswordTooShort, got: %v", err)
	}
}

// TestLogin tests successful login.
func TestLogin(t *testing.T) {
	db := setupTestDB(t)
	ctx := context.Background()

	service, err := NewService(Config{
		DB:                db,
		JWTSecret:         "test-secret-key-at-least-32-chars",
		JWTIssuer:         "test",
		JWTAccessTokenTTL: 15 * time.Minute,
		JWTRefreshTokenTTL: 24 * time.Hour,
		Enabled:           true,
		PasswordMinLength: 8,
	})
	if err != nil {
		t.Fatalf("failed to create auth service: %v", err)
	}

	// Register a user
	_, err = service.Register(ctx, RegisterInput{
		Email:    "test@example.com",
		Password: "password123",
	}, nil, nil)
	if err != nil {
		t.Fatalf("registration failed: %v", err)
	}

	// Login
	result, err := service.Login(ctx, "test@example.com", "password123", nil, nil)
	if err != nil {
		t.Fatalf("Login failed: %v", err)
	}

	if result.User == nil {
		t.Fatal("result.User should not be nil")
	}

	if result.AccessToken == "" {
		t.Error("access token should not be empty")
	}

	if result.RefreshToken == "" {
		t.Error("refresh token should not be empty")
	}
}

// TestLogin_InvalidCredentials tests login with wrong password.
func TestLogin_InvalidCredentials(t *testing.T) {
	db := setupTestDB(t)
	ctx := context.Background()

	service, err := NewService(Config{
		DB:                db,
		JWTSecret:         "test-secret-key-at-least-32-chars",
		JWTIssuer:         "test",
		JWTAccessTokenTTL: 15 * time.Minute,
		JWTRefreshTokenTTL: 24 * time.Hour,
		Enabled:           true,
		PasswordMinLength: 8,
	})
	if err != nil {
		t.Fatalf("failed to create auth service: %v", err)
	}

	// Register a user
	_, err = service.Register(ctx, RegisterInput{
		Email:    "test@example.com",
		Password: "password123",
	}, nil, nil)
	if err != nil {
		t.Fatalf("registration failed: %v", err)
	}

	// Login with wrong password
	_, err = service.Login(ctx, "test@example.com", "wrongpassword", nil, nil)
	if err != ErrInvalidCredentials {
		t.Errorf("expected ErrInvalidCredentials, got: %v", err)
	}
}

// TestLogin_NonexistentUser tests login with nonexistent user.
func TestLogin_NonexistentUser(t *testing.T) {
	db := setupTestDB(t)
	ctx := context.Background()

	service, err := NewService(Config{
		DB:                db,
		JWTSecret:         "test-secret-key-at-least-32-chars",
		JWTIssuer:         "test",
		JWTAccessTokenTTL: 15 * time.Minute,
		JWTRefreshTokenTTL: 24 * time.Hour,
		Enabled:           true,
		PasswordMinLength: 8,
	})
	if err != nil {
		t.Fatalf("failed to create auth service: %v", err)
	}

	_, err = service.Login(ctx, "nonexistent@example.com", "password123", nil, nil)
	if err != ErrInvalidCredentials {
		t.Errorf("expected ErrInvalidCredentials, got: %v", err)
	}
}

// TestLogout tests session logout.
func TestLogout(t *testing.T) {
	db := setupTestDB(t)
	ctx := context.Background()

	service, err := NewService(Config{
		DB:                db,
		JWTSecret:         "test-secret-key-at-least-32-chars",
		JWTIssuer:         "test",
		JWTAccessTokenTTL: 15 * time.Minute,
		JWTRefreshTokenTTL: 24 * time.Hour,
		Enabled:           true,
		PasswordMinLength: 8,
	})
	if err != nil {
		t.Fatalf("failed to create auth service: %v", err)
	}

	// Register and login
	result, err := service.Register(ctx, RegisterInput{
		Email:    "test@example.com",
		Password: "password123",
	}, nil, nil)
	if err != nil {
		t.Fatalf("registration failed: %v", err)
	}

	// Logout
	err = service.Logout(ctx, result.SessionID)
	if err != nil {
		t.Fatalf("Logout failed: %v", err)
	}

	// Verify session is deleted
	sessions, err := service.GetUserSessions(ctx, result.User.ID)
	if err != nil {
		t.Fatalf("GetUserSessions failed: %v", err)
	}

	for _, s := range sessions {
		if s.ID == result.SessionID {
			t.Error("session should be deleted after logout")
		}
	}
}

// TestChangePassword tests password change.
func TestChangePassword(t *testing.T) {
	db := setupTestDB(t)
	ctx := context.Background()

	service, err := NewService(Config{
		DB:                db,
		JWTSecret:         "test-secret-key-at-least-32-chars",
		JWTIssuer:         "test",
		JWTAccessTokenTTL: 15 * time.Minute,
		JWTRefreshTokenTTL: 24 * time.Hour,
		Enabled:           true,
		PasswordMinLength: 8,
	})
	if err != nil {
		t.Fatalf("failed to create auth service: %v", err)
	}

	// Register a user
	result, err := service.Register(ctx, RegisterInput{
		Email:    "test@example.com",
		Password: "password123",
	}, nil, nil)
	if err != nil {
		t.Fatalf("registration failed: %v", err)
	}

	// Change password
	err = service.ChangePassword(ctx, result.User.ID, "password123", "newpassword456")
	if err != nil {
		t.Fatalf("ChangePassword failed: %v", err)
	}

	// Old password should not work
	_, err = service.Login(ctx, "test@example.com", "password123", nil, nil)
	if err != ErrInvalidCredentials {
		t.Error("old password should not work after change")
	}

	// New password should work
	_, err = service.Login(ctx, "test@example.com", "newpassword456", nil, nil)
	if err != nil {
		t.Errorf("new password should work: %v", err)
	}
}

// TestChangePassword_WrongOldPassword tests that change fails with wrong old password.
func TestChangePassword_WrongOldPassword(t *testing.T) {
	db := setupTestDB(t)
	ctx := context.Background()

	service, err := NewService(Config{
		DB:                db,
		JWTSecret:         "test-secret-key-at-least-32-chars",
		JWTIssuer:         "test",
		JWTAccessTokenTTL: 15 * time.Minute,
		JWTRefreshTokenTTL: 24 * time.Hour,
		Enabled:           true,
		PasswordMinLength: 8,
	})
	if err != nil {
		t.Fatalf("failed to create auth service: %v", err)
	}

	// Register a user
	result, err := service.Register(ctx, RegisterInput{
		Email:    "test@example.com",
		Password: "password123",
	}, nil, nil)
	if err != nil {
		t.Fatalf("registration failed: %v", err)
	}

	// Try to change password with wrong old password
	err = service.ChangePassword(ctx, result.User.ID, "wrongpassword", "newpassword456")
	if err != ErrInvalidCredentials {
		t.Errorf("expected ErrInvalidCredentials, got: %v", err)
	}
}

// TestAuthDisabled tests that auth operations fail when disabled.
func TestAuthDisabled(t *testing.T) {
	db := setupTestDB(t)
	ctx := context.Background()

	service, err := NewService(Config{
		DB:                db,
		JWTSecret:         "test-secret-key-at-least-32-chars",
		JWTIssuer:         "test",
		JWTAccessTokenTTL: 15 * time.Minute,
		JWTRefreshTokenTTL: 24 * time.Hour,
		Enabled:           false, // Auth disabled
		PasswordMinLength: 8,
	})
	if err != nil {
		t.Fatalf("failed to create auth service: %v", err)
	}

	// Register should fail
	_, err = service.Register(ctx, RegisterInput{
		Email:    "test@example.com",
		Password: "password123",
	}, nil, nil)
	if err != ErrAuthDisabled {
		t.Errorf("expected ErrAuthDisabled for Register, got: %v", err)
	}

	// Login should fail
	_, err = service.Login(ctx, "test@example.com", "password123", nil, nil)
	if err != ErrAuthDisabled {
		t.Errorf("expected ErrAuthDisabled for Login, got: %v", err)
	}

	// ChangePassword should fail
	err = service.ChangePassword(ctx, "user-id", "old", "new")
	if err != ErrAuthDisabled {
		t.Errorf("expected ErrAuthDisabled for ChangePassword, got: %v", err)
	}
}
