package auth

import (
	"context"
	"errors"
	"log"
	"time"

	"github.com/lucsky/cuid"
	"gorm.io/gorm"

	"github.com/bbernstein/lacylights-go/internal/auth/session"
	"github.com/bbernstein/lacylights-go/internal/database/models"
)

// Service provides authentication functionality.
type Service struct {
	db              *gorm.DB
	jwt             *JWTService
	sessionManager  *session.Manager
	passwordMinLen  int
	enabled         bool
	deviceAuthEnabled bool
}

// Config holds configuration for the auth service.
type Config struct {
	// Database connection
	DB *gorm.DB

	// JWT configuration
	JWTSecret          string
	JWTIssuer          string
	JWTAccessTokenTTL  time.Duration
	JWTRefreshTokenTTL time.Duration

	// Session configuration
	SessionDurationHours int
	CacheMaxSize         int
	CacheTTL             time.Duration

	// Password configuration
	PasswordMinLength int

	// Feature flags
	Enabled           bool
	DeviceAuthEnabled bool
}

// Errors
var (
	ErrAuthDisabled       = errors.New("authentication is not enabled")
	ErrUserNotFound       = errors.New("user not found")
	ErrUserExists         = errors.New("user already exists")
	ErrInvalidCredentials = errors.New("invalid email or password")
	ErrAccountLocked      = errors.New("account is locked")
	ErrAccountInactive    = errors.New("account is inactive")
	ErrPasswordTooShort   = errors.New("password is too short")
	ErrSessionNotFound    = errors.New("session not found")
	ErrSessionExpired     = errors.New("session has expired")
)

// NewService creates a new authentication service.
func NewService(cfg Config) (*Service, error) {
	jwtService, err := NewJWTService(JWTConfig{
		Secret:          cfg.JWTSecret,
		Issuer:          cfg.JWTIssuer,
		AccessTokenTTL:  cfg.JWTAccessTokenTTL,
		RefreshTokenTTL: cfg.JWTRefreshTokenTTL,
	})
	if err != nil {
		return nil, err
	}

	sessionManager := session.NewManager(cfg.DB, session.ManagerConfig{
		CacheMaxSize: cfg.CacheMaxSize,
		CacheTTL:     cfg.CacheTTL,
	})

	passwordMinLen := cfg.PasswordMinLength
	if passwordMinLen <= 0 {
		passwordMinLen = 8
	}

	return &Service{
		db:                cfg.DB,
		jwt:               jwtService,
		sessionManager:    sessionManager,
		passwordMinLen:    passwordMinLen,
		enabled:           cfg.Enabled,
		deviceAuthEnabled: cfg.DeviceAuthEnabled,
	}, nil
}

// IsEnabled returns whether authentication is enabled.
func (s *Service) IsEnabled() bool {
	return s.enabled
}

// IsDeviceAuthEnabled returns whether device authentication is enabled.
func (s *Service) IsDeviceAuthEnabled() bool {
	return s.deviceAuthEnabled
}

// RegisterInput holds input for registering a new user.
type RegisterInput struct {
	Email    string
	Password string
	Name     *string
}

// AuthResult holds the result of a successful authentication.
type AuthResult struct {
	User         *models.User
	AccessToken  string
	RefreshToken string
	ExpiresAt    time.Time
	SessionID    string
}

// Register creates a new user account with email/password.
func (s *Service) Register(ctx context.Context, input RegisterInput, ipAddress, userAgent *string) (*AuthResult, error) {
	if !s.enabled {
		return nil, ErrAuthDisabled
	}

	// Validate password length
	if len(input.Password) < s.passwordMinLen {
		return nil, ErrPasswordTooShort
	}

	// Check if user already exists
	var existingUser models.User
	if err := s.db.WithContext(ctx).Where("email = ?", input.Email).First(&existingUser).Error; err == nil {
		return nil, ErrUserExists
	} else if !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, err
	}

	// Hash the password
	passwordHash, err := HashPassword(input.Password)
	if err != nil {
		return nil, err
	}

	// Create user and credentials in a transaction
	var user models.User
	err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// Create user
		user = models.User{
			ID:        cuid.New(),
			Email:     input.Email,
			Name:      input.Name,
			Role:      "USER",
			IsActive:  true,
		}
		if err := tx.Create(&user).Error; err != nil {
			return err
		}

		// Create credentials
		creds := models.UserCredential{
			ID:                cuid.New(),
			UserID:            user.ID,
			PasswordHash:      passwordHash,
			PasswordUpdatedAt: time.Now(),
		}
		if err := tx.Create(&creds).Error; err != nil {
			return err
		}

		return nil
	})
	if err != nil {
		return nil, err
	}

	// Create session and generate tokens
	return s.createSessionAndTokens(ctx, &user, ipAddress, userAgent)
}

// Login authenticates a user with email and password.
func (s *Service) Login(ctx context.Context, email, password string, ipAddress, userAgent *string) (*AuthResult, error) {
	if !s.enabled {
		return nil, ErrAuthDisabled
	}

	// Find user by email
	var user models.User
	if err := s.db.WithContext(ctx).Where("email = ?", email).First(&user).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrInvalidCredentials
		}
		return nil, err
	}

	// Check if user is active
	if !user.IsActive {
		return nil, ErrAccountInactive
	}

	// Get credentials
	var creds models.UserCredential
	if err := s.db.WithContext(ctx).Where("user_id = ?", user.ID).First(&creds).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrInvalidCredentials
		}
		return nil, err
	}

	// Check if account is locked
	if creds.LockedUntil != nil && time.Now().Before(*creds.LockedUntil) {
		return nil, ErrAccountLocked
	}

	// Verify password
	if err := VerifyPassword(password, creds.PasswordHash); err != nil {
		// Increment failed attempts
		creds.FailedAttempts++
		if creds.FailedAttempts >= 5 {
			// Lock account for 15 minutes
			lockUntil := time.Now().Add(15 * time.Minute)
			creds.LockedUntil = &lockUntil
		}
		if saveErr := s.db.WithContext(ctx).Save(&creds).Error; saveErr != nil {
			log.Printf("Warning: failed to save failed login attempt count for user %s: %v", user.ID, saveErr)
		}
		return nil, ErrInvalidCredentials
	}

	// Reset failed attempts on successful login
	if creds.FailedAttempts > 0 {
		creds.FailedAttempts = 0
		creds.LockedUntil = nil
		if saveErr := s.db.WithContext(ctx).Save(&creds).Error; saveErr != nil {
			log.Printf("Warning: failed to reset failed login attempts for user %s: %v", user.ID, saveErr)
		}
	}

	// Update last login time
	now := time.Now()
	user.LastLoginAt = &now
	if saveErr := s.db.WithContext(ctx).Save(&user).Error; saveErr != nil {
		log.Printf("Warning: failed to update last login time for user %s: %v", user.ID, saveErr)
	}

	// Create session and generate tokens
	return s.createSessionAndTokens(ctx, &user, ipAddress, userAgent)
}

// Logout invalidates a session.
func (s *Service) Logout(ctx context.Context, sessionID string) error {
	return s.sessionManager.Delete(ctx, sessionID)
}

// LogoutAll invalidates all sessions for a user.
func (s *Service) LogoutAll(ctx context.Context, userID string) error {
	return s.sessionManager.DeleteByUserID(ctx, userID)
}

// RefreshToken generates new tokens using a refresh token.
func (s *Service) RefreshToken(ctx context.Context, refreshToken string) (*AuthResult, error) {
	if !s.enabled {
		return nil, ErrAuthDisabled
	}

	// Validate the refresh token
	claims, err := s.jwt.ValidateRefreshToken(refreshToken)
	if err != nil {
		return nil, err
	}

	// Get the session
	sess, err := s.sessionManager.GetByID(ctx, claims.SessionID)
	if err != nil {
		return nil, err
	}
	if sess == nil {
		return nil, ErrSessionNotFound
	}

	// Check if session has expired
	if time.Now().After(sess.ExpiresAt) {
		return nil, ErrSessionExpired
	}

	// Get the user
	var user models.User
	if err := s.db.WithContext(ctx).First(&user, "id = ?", claims.UserID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrUserNotFound
		}
		return nil, err
	}

	// Generate new tokens
	tokenPair, err := s.jwt.GenerateTokenPair(user.ID, user.Email, user.Role, sess.ID)
	if err != nil {
		return nil, err
	}

	// Update session last activity
	_ = s.sessionManager.UpdateLastActivity(ctx, sess.ID)

	return &AuthResult{
		User:         &user,
		AccessToken:  tokenPair.AccessToken,
		RefreshToken: tokenPair.RefreshToken,
		ExpiresAt:    tokenPair.ExpiresAt,
		SessionID:    sess.ID,
	}, nil
}

// ValidateSession validates an access token and returns the session info.
func (s *Service) ValidateSession(ctx context.Context, accessToken string) (*session.CachedSession, error) {
	if !s.enabled {
		return nil, ErrAuthDisabled
	}

	// Validate the token
	claims, err := s.jwt.ValidateAccessToken(accessToken)
	if err != nil {
		return nil, err
	}

	// Get session from cache/database
	sess, err := s.sessionManager.GetByTokenHash(ctx, HashToken(claims.SessionID))
	if err != nil {
		return nil, err
	}
	if sess == nil {
		return nil, ErrSessionNotFound
	}

	return sess, nil
}

// GetUserByID retrieves a user by their ID.
func (s *Service) GetUserByID(ctx context.Context, userID string) (*models.User, error) {
	var user models.User
	if err := s.db.WithContext(ctx).First(&user, "id = ?", userID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrUserNotFound
		}
		return nil, err
	}
	return &user, nil
}

// GetUserByEmail retrieves a user by their email.
func (s *Service) GetUserByEmail(ctx context.Context, email string) (*models.User, error) {
	var user models.User
	if err := s.db.WithContext(ctx).Where("email = ?", email).First(&user).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrUserNotFound
		}
		return nil, err
	}
	return &user, nil
}

// GetUserSessions retrieves all active sessions for a user.
func (s *Service) GetUserSessions(ctx context.Context, userID string) ([]models.Session, error) {
	return s.sessionManager.GetByUserID(ctx, userID)
}

// createSessionAndTokens creates a new session and generates JWT tokens.
func (s *Service) createSessionAndTokens(ctx context.Context, user *models.User, ipAddress, userAgent *string) (*AuthResult, error) {
	// Create session
	sessionInfo, err := s.sessionManager.Create(ctx, session.CreateSessionInput{
		UserID:    user.ID,
		Email:     user.Email,
		Role:      user.Role,
		IPAddress: ipAddress,
		UserAgent: userAgent,
		ExpiresAt: time.Now().Add(s.jwt.RefreshTokenTTL()),
	})
	if err != nil {
		return nil, err
	}

	// Generate tokens
	tokenPair, err := s.jwt.GenerateTokenPair(user.ID, user.Email, user.Role, sessionInfo.SessionID)
	if err != nil {
		return nil, err
	}

	return &AuthResult{
		User:         user,
		AccessToken:  tokenPair.AccessToken,
		RefreshToken: tokenPair.RefreshToken,
		ExpiresAt:    tokenPair.ExpiresAt,
		SessionID:    sessionInfo.SessionID,
	}, nil
}

// ChangePassword changes a user's password.
func (s *Service) ChangePassword(ctx context.Context, userID, oldPassword, newPassword string) error {
	if !s.enabled {
		return ErrAuthDisabled
	}

	// Validate new password length
	if len(newPassword) < s.passwordMinLen {
		return ErrPasswordTooShort
	}

	// Get credentials
	var creds models.UserCredential
	if err := s.db.WithContext(ctx).Where("user_id = ?", userID).First(&creds).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ErrUserNotFound
		}
		return err
	}

	// Verify old password
	if err := VerifyPassword(oldPassword, creds.PasswordHash); err != nil {
		return ErrInvalidCredentials
	}

	// Hash new password
	passwordHash, err := HashPassword(newPassword)
	if err != nil {
		return err
	}

	// Update credentials
	creds.PasswordHash = passwordHash
	creds.PasswordUpdatedAt = time.Now()
	return s.db.WithContext(ctx).Save(&creds).Error
}

// JWTService returns the JWT service for middleware use.
func (s *Service) JWTService() *JWTService {
	return s.jwt
}

// SessionManager returns the session manager.
func (s *Service) SessionManager() *session.Manager {
	return s.sessionManager
}

// EnsureDefaultAdmin creates the default admin user if no users exist.
// This is called on startup when auth is enabled to ensure there's at least
// one admin user to manage the system.
func (s *Service) EnsureDefaultAdmin(ctx context.Context, email, password string) error {
	if !s.enabled {
		return nil // No-op if auth is disabled
	}

	if password == "" {
		return errors.New("default admin password is required when authentication is enabled")
	}

	// Check if any users exist
	var count int64
	if err := s.db.WithContext(ctx).Model(&models.User{}).Count(&count).Error; err != nil {
		return err
	}

	if count > 0 {
		return nil // Users already exist, no need to create default admin
	}

	// Validate password length
	if len(password) < s.passwordMinLen {
		return ErrPasswordTooShort
	}

	// Hash the password
	passwordHash, err := HashPassword(password)
	if err != nil {
		return err
	}

	// Create admin user and credentials in a transaction
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// Create user with ADMIN role
		user := models.User{
			ID:       cuid.New(),
			Email:    email,
			Role:     "ADMIN",
			IsActive: true,
		}
		if err := tx.Create(&user).Error; err != nil {
			return err
		}

		// Create credentials
		creds := models.UserCredential{
			ID:                cuid.New(),
			UserID:            user.ID,
			PasswordHash:      passwordHash,
			PasswordUpdatedAt: time.Now(),
		}
		if err := tx.Create(&creds).Error; err != nil {
			return err
		}

		return nil
	})
}
