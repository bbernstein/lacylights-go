package auth

import (
	"context"
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

	// Migrate the schema
	if err := db.AutoMigrate(&models.User{}, &models.UserCredential{}); err != nil {
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
