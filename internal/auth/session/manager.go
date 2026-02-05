package session

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"time"

	"github.com/lucsky/cuid"
	"gorm.io/gorm"

	"github.com/bbernstein/lacylights-go/internal/database/models"
)

// Manager handles session CRUD operations.
type Manager struct {
	db    *gorm.DB
	cache *Cache
}

// ManagerConfig holds configuration for the session manager.
type ManagerConfig struct {
	CacheMaxSize int
	CacheTTL     time.Duration
}

// NewManager creates a new session manager.
func NewManager(db *gorm.DB, cfg ManagerConfig) *Manager {
	return &Manager{
		db: db,
		cache: NewCache(CacheConfig{
			MaxSize: cfg.CacheMaxSize,
			TTL:     cfg.CacheTTL,
		}),
	}
}

// CreateSessionInput holds input for creating a new session.
type CreateSessionInput struct {
	UserID    string
	Email     string
	Role      string
	DeviceID  *string
	IPAddress *string
	UserAgent *string
	ExpiresAt time.Time
}

// SessionInfo holds information about a created session.
type SessionInfo struct {
	SessionID string
	TokenHash string
	ExpiresAt time.Time
}

// Create creates a new session for a user.
func (m *Manager) Create(ctx context.Context, input CreateSessionInput) (*SessionInfo, error) {
	sessionID := cuid.New()

	// Generate a token hash (this would normally be the hash of the JWT)
	// In our case, we use the session ID to create a unique hash
	tokenHash := hashString(sessionID)

	session := &models.Session{
		ID:             sessionID,
		UserID:         input.UserID,
		TokenHash:      tokenHash,
		DeviceID:       input.DeviceID,
		IPAddress:      input.IPAddress,
		UserAgent:      input.UserAgent,
		ExpiresAt:      input.ExpiresAt,
		LastActivityAt: time.Now(),
	}

	if err := m.db.WithContext(ctx).Create(session).Error; err != nil {
		return nil, err
	}

	// Cache the session for fast lookups
	m.cache.Set(tokenHash, &CachedSession{
		UserID:    input.UserID,
		Email:     input.Email,
		Role:      input.Role,
		SessionID: sessionID,
		DeviceID:  input.DeviceID,
		ExpiresAt: input.ExpiresAt,
	})

	return &SessionInfo{
		SessionID: sessionID,
		TokenHash: tokenHash,
		ExpiresAt: input.ExpiresAt,
	}, nil
}

// GetByTokenHash retrieves a session by its token hash.
// First checks the cache, then falls back to the database.
func (m *Manager) GetByTokenHash(ctx context.Context, tokenHash string) (*CachedSession, error) {
	// Try cache first (O(1) lookup)
	if cached, ok := m.cache.Get(tokenHash); ok {
		return cached, nil
	}

	// Fall back to database
	var session models.Session
	if err := m.db.WithContext(ctx).
		Preload("User").
		Where("token_hash = ? AND expires_at > ?", tokenHash, time.Now()).
		First(&session).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}

	// Populate cache
	cached := &CachedSession{
		UserID:    session.UserID,
		SessionID: session.ID,
		DeviceID:  session.DeviceID,
		ExpiresAt: session.ExpiresAt,
	}
	if session.User != nil {
		cached.Email = session.User.Email
		cached.Role = session.User.Role
	}
	m.cache.Set(tokenHash, cached)

	return cached, nil
}

// GetByID retrieves a session by its ID.
func (m *Manager) GetByID(ctx context.Context, sessionID string) (*models.Session, error) {
	var session models.Session
	if err := m.db.WithContext(ctx).
		Preload("User").
		First(&session, "id = ?", sessionID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &session, nil
}

// GetByUserID retrieves all sessions for a user.
func (m *Manager) GetByUserID(ctx context.Context, userID string) ([]models.Session, error) {
	var sessions []models.Session
	if err := m.db.WithContext(ctx).
		Where("user_id = ? AND expires_at > ?", userID, time.Now()).
		Order("created_at DESC").
		Find(&sessions).Error; err != nil {
		return nil, err
	}
	return sessions, nil
}

// UpdateLastActivity updates the last activity timestamp for a session.
func (m *Manager) UpdateLastActivity(ctx context.Context, sessionID string) error {
	return m.db.WithContext(ctx).
		Model(&models.Session{}).
		Where("id = ?", sessionID).
		Update("last_activity_at", time.Now()).Error
}

// Delete removes a session by its ID.
func (m *Manager) Delete(ctx context.Context, sessionID string) error {
	// First get the session to find the token hash
	var session models.Session
	if err := m.db.WithContext(ctx).First(&session, "id = ?", sessionID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil // Already deleted
		}
		return err
	}

	// Remove from cache
	m.cache.DeleteBySessionID(sessionID)

	// Delete from database
	return m.db.WithContext(ctx).Delete(&models.Session{}, "id = ?", sessionID).Error
}

// DeleteByUserID removes all sessions for a user.
func (m *Manager) DeleteByUserID(ctx context.Context, userID string) error {
	// Remove from cache
	m.cache.DeleteByUserID(userID)

	// Delete from database
	return m.db.WithContext(ctx).Delete(&models.Session{}, "user_id = ?", userID).Error
}

// DeleteExpired removes all expired sessions from the database.
func (m *Manager) DeleteExpired(ctx context.Context) (int64, error) {
	result := m.db.WithContext(ctx).Delete(&models.Session{}, "expires_at < ?", time.Now())
	return result.RowsAffected, result.Error
}

// ClearCache clears the session cache.
func (m *Manager) ClearCache() {
	m.cache.Clear()
}

// CacheSize returns the current size of the session cache.
func (m *Manager) CacheSize() int {
	return m.cache.Size()
}

// hashString creates a SHA-256 hash of a string.
func hashString(s string) string {
	h := sha256.New()
	h.Write([]byte(s))
	return hex.EncodeToString(h.Sum(nil))
}
