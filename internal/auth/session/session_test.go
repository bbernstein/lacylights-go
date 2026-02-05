package session

import (
	"context"
	"testing"
	"time"

	"github.com/bbernstein/lacylights-go/internal/database/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// setupTestDB creates an in-memory SQLite database for testing
func setupTestDB(t *testing.T) *gorm.DB {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	require.NoError(t, err)

	// Auto-migrate the models
	err = db.AutoMigrate(&models.User{}, &models.Session{})
	require.NoError(t, err)

	return db
}

// createTestUser creates a user in the database for testing
func createTestUser(t *testing.T, db *gorm.DB, id, email, role string) *models.User {
	user := &models.User{
		ID:       id,
		Email:    email,
		Role:     role,
		IsActive: true,
	}
	err := db.Create(user).Error
	require.NoError(t, err)
	return user
}

// =====================
// Cache Tests
// =====================

func TestNewCache_DefaultValues(t *testing.T) {
	// Test with zero config to verify defaults are applied
	cache := NewCache(CacheConfig{})
	defer cache.Stop()

	assert.NotNil(t, cache)
	assert.Equal(t, 1000, cache.maxSize)
	assert.Equal(t, 5*time.Minute, cache.ttl)
}

func TestNewCache_CustomValues(t *testing.T) {
	cache := NewCache(CacheConfig{
		MaxSize: 500,
		TTL:     10 * time.Minute,
	})
	defer cache.Stop()

	assert.NotNil(t, cache)
	assert.Equal(t, 500, cache.maxSize)
	assert.Equal(t, 10*time.Minute, cache.ttl)
}

func TestCache_SetAndGet(t *testing.T) {
	cache := NewCache(CacheConfig{
		MaxSize: 10,
		TTL:     1 * time.Hour,
	})
	defer cache.Stop()

	session := &CachedSession{
		UserID:    "user-1",
		Email:     "test@example.com",
		Role:      "ADMIN",
		SessionID: "session-1",
		ExpiresAt: time.Now().Add(1 * time.Hour),
	}

	cache.Set("token-hash-1", session)

	// Retrieve the session
	retrieved, ok := cache.Get("token-hash-1")
	assert.True(t, ok)
	assert.NotNil(t, retrieved)
	assert.Equal(t, "user-1", retrieved.UserID)
	assert.Equal(t, "test@example.com", retrieved.Email)
	assert.Equal(t, "ADMIN", retrieved.Role)
	assert.Equal(t, "session-1", retrieved.SessionID)
}

func TestCache_Get_NotFound(t *testing.T) {
	cache := NewCache(CacheConfig{
		MaxSize: 10,
		TTL:     1 * time.Hour,
	})
	defer cache.Stop()

	retrieved, ok := cache.Get("nonexistent")
	assert.False(t, ok)
	assert.Nil(t, retrieved)
}

func TestCache_Get_ExpiredCacheTTL(t *testing.T) {
	cache := NewCache(CacheConfig{
		MaxSize: 10,
		TTL:     1 * time.Millisecond, // Very short TTL
	})
	defer cache.Stop()

	session := &CachedSession{
		UserID:    "user-1",
		SessionID: "session-1",
		ExpiresAt: time.Now().Add(1 * time.Hour),
	}

	cache.Set("token-hash-1", session)

	// Wait for TTL to expire
	time.Sleep(10 * time.Millisecond)

	retrieved, ok := cache.Get("token-hash-1")
	assert.False(t, ok)
	assert.Nil(t, retrieved)
}

func TestCache_Get_ExpiredSession(t *testing.T) {
	cache := NewCache(CacheConfig{
		MaxSize: 10,
		TTL:     1 * time.Hour,
	})
	defer cache.Stop()

	session := &CachedSession{
		UserID:    "user-1",
		SessionID: "session-1",
		ExpiresAt: time.Now().Add(-1 * time.Hour), // Already expired
	}

	cache.Set("token-hash-1", session)

	retrieved, ok := cache.Get("token-hash-1")
	assert.False(t, ok)
	assert.Nil(t, retrieved)
}

func TestCache_Delete(t *testing.T) {
	cache := NewCache(CacheConfig{
		MaxSize: 10,
		TTL:     1 * time.Hour,
	})
	defer cache.Stop()

	session := &CachedSession{
		UserID:    "user-1",
		SessionID: "session-1",
		ExpiresAt: time.Now().Add(1 * time.Hour),
	}

	cache.Set("token-hash-1", session)

	// Verify it's there
	_, ok := cache.Get("token-hash-1")
	assert.True(t, ok)

	// Delete it
	cache.Delete("token-hash-1")

	// Verify it's gone
	retrieved, ok := cache.Get("token-hash-1")
	assert.False(t, ok)
	assert.Nil(t, retrieved)
}

func TestCache_DeleteByUserID(t *testing.T) {
	cache := NewCache(CacheConfig{
		MaxSize: 10,
		TTL:     1 * time.Hour,
	})
	defer cache.Stop()

	// Create multiple sessions for same user
	for i := 0; i < 3; i++ {
		cache.Set("token-hash-user1-"+string(rune('a'+i)), &CachedSession{
			UserID:    "user-1",
			SessionID: "session-" + string(rune('a'+i)),
			ExpiresAt: time.Now().Add(1 * time.Hour),
		})
	}

	// Create session for different user
	cache.Set("token-hash-user2", &CachedSession{
		UserID:    "user-2",
		SessionID: "session-other",
		ExpiresAt: time.Now().Add(1 * time.Hour),
	})

	assert.Equal(t, 4, cache.Size())

	// Delete all sessions for user-1
	cache.DeleteByUserID("user-1")

	assert.Equal(t, 1, cache.Size())

	// User 2's session should still exist
	retrieved, ok := cache.Get("token-hash-user2")
	assert.True(t, ok)
	assert.Equal(t, "user-2", retrieved.UserID)
}

func TestCache_DeleteBySessionID(t *testing.T) {
	cache := NewCache(CacheConfig{
		MaxSize: 10,
		TTL:     1 * time.Hour,
	})
	defer cache.Stop()

	cache.Set("token-hash-1", &CachedSession{
		UserID:    "user-1",
		SessionID: "session-1",
		ExpiresAt: time.Now().Add(1 * time.Hour),
	})

	cache.Set("token-hash-2", &CachedSession{
		UserID:    "user-1",
		SessionID: "session-2",
		ExpiresAt: time.Now().Add(1 * time.Hour),
	})

	assert.Equal(t, 2, cache.Size())

	cache.DeleteBySessionID("session-1")

	assert.Equal(t, 1, cache.Size())

	// session-2 should still exist
	retrieved, ok := cache.Get("token-hash-2")
	assert.True(t, ok)
	assert.Equal(t, "session-2", retrieved.SessionID)
}

func TestCache_Clear(t *testing.T) {
	cache := NewCache(CacheConfig{
		MaxSize: 10,
		TTL:     1 * time.Hour,
	})
	defer cache.Stop()

	for i := 0; i < 5; i++ {
		cache.Set("token-hash-"+string(rune('0'+i)), &CachedSession{
			UserID:    "user-" + string(rune('0'+i)),
			SessionID: "session-" + string(rune('0'+i)),
			ExpiresAt: time.Now().Add(1 * time.Hour),
		})
	}

	assert.Equal(t, 5, cache.Size())

	cache.Clear()

	assert.Equal(t, 0, cache.Size())
}

func TestCache_Size(t *testing.T) {
	cache := NewCache(CacheConfig{
		MaxSize: 10,
		TTL:     1 * time.Hour,
	})
	defer cache.Stop()

	assert.Equal(t, 0, cache.Size())

	cache.Set("token-hash-1", &CachedSession{
		UserID:    "user-1",
		SessionID: "session-1",
		ExpiresAt: time.Now().Add(1 * time.Hour),
	})

	assert.Equal(t, 1, cache.Size())
}

func TestCache_Eviction(t *testing.T) {
	cache := NewCache(CacheConfig{
		MaxSize: 3,
		TTL:     1 * time.Hour,
	})
	defer cache.Stop()

	// Add 3 sessions
	for i := 0; i < 3; i++ {
		cache.Set("token-hash-"+string(rune('0'+i)), &CachedSession{
			UserID:    "user-" + string(rune('0'+i)),
			SessionID: "session-" + string(rune('0'+i)),
			ExpiresAt: time.Now().Add(1 * time.Hour),
		})
		// Small delay to ensure different CachedAt times
		time.Sleep(1 * time.Millisecond)
	}

	assert.Equal(t, 3, cache.Size())

	// Add a 4th session - should trigger eviction
	cache.Set("token-hash-new", &CachedSession{
		UserID:    "user-new",
		SessionID: "session-new",
		ExpiresAt: time.Now().Add(1 * time.Hour),
	})

	assert.Equal(t, 3, cache.Size())

	// The newest session should be there
	retrieved, ok := cache.Get("token-hash-new")
	assert.True(t, ok)
	assert.Equal(t, "user-new", retrieved.UserID)
}

func TestCache_Cleanup(t *testing.T) {
	cache := NewCache(CacheConfig{
		MaxSize: 10,
		TTL:     1 * time.Hour,
	})
	defer cache.Stop()

	// Add an expired session (session expires, not cache TTL)
	cache.mu.Lock()
	cache.cache["expired-token"] = &CachedSession{
		UserID:    "user-1",
		SessionID: "session-1",
		ExpiresAt: time.Now().Add(-1 * time.Hour), // Expired session
		CachedAt:  time.Now(),
	}
	cache.mu.Unlock()

	assert.Equal(t, 1, cache.Size())

	// Trigger cleanup manually
	cache.cleanup()

	assert.Equal(t, 0, cache.Size())
}

func TestCache_CleanupLoop(t *testing.T) {
	// Use a very short TTL to trigger cleanup quickly
	cache := NewCache(CacheConfig{
		MaxSize: 10,
		TTL:     50 * time.Millisecond,
	})
	defer cache.Stop()

	cache.Set("token-hash-1", &CachedSession{
		UserID:    "user-1",
		SessionID: "session-1",
		ExpiresAt: time.Now().Add(1 * time.Hour),
	})

	assert.Equal(t, 1, cache.Size())

	// Wait for TTL to expire and cleanup to run (TTL/2 = 25ms)
	time.Sleep(100 * time.Millisecond)

	// Should be cleaned up
	assert.Equal(t, 0, cache.Size())
}

func TestCache_Stop(t *testing.T) {
	cache := NewCache(CacheConfig{
		MaxSize: 10,
		TTL:     1 * time.Minute,
	})

	// Stop should not panic
	cache.Stop()
}

// =====================
// Manager Tests
// =====================

func TestNewManager(t *testing.T) {
	db := setupTestDB(t)

	manager := NewManager(db, ManagerConfig{
		CacheMaxSize: 100,
		CacheTTL:     5 * time.Minute,
	})
	defer manager.cache.Stop()

	assert.NotNil(t, manager)
	assert.NotNil(t, manager.db)
	assert.NotNil(t, manager.cache)
}

func TestManager_Create(t *testing.T) {
	db := setupTestDB(t)
	createTestUser(t, db, "user-1", "test@example.com", "ADMIN")

	manager := NewManager(db, ManagerConfig{
		CacheMaxSize: 100,
		CacheTTL:     5 * time.Minute,
	})
	defer manager.cache.Stop()

	ctx := context.Background()
	deviceID := "device-1"
	ipAddress := "192.168.1.1"
	userAgent := "TestAgent/1.0"

	input := CreateSessionInput{
		UserID:    "user-1",
		Email:     "test@example.com",
		Role:      "ADMIN",
		DeviceID:  &deviceID,
		IPAddress: &ipAddress,
		UserAgent: &userAgent,
		ExpiresAt: time.Now().Add(1 * time.Hour),
	}

	sessionInfo, err := manager.Create(ctx, input)
	require.NoError(t, err)
	assert.NotEmpty(t, sessionInfo.SessionID)
	assert.NotEmpty(t, sessionInfo.TokenHash)
	assert.False(t, sessionInfo.ExpiresAt.IsZero())

	// Verify session is in database
	var dbSession models.Session
	err = db.First(&dbSession, "id = ?", sessionInfo.SessionID).Error
	require.NoError(t, err)
	assert.Equal(t, "user-1", dbSession.UserID)
	assert.Equal(t, sessionInfo.TokenHash, dbSession.TokenHash)

	// Verify session is in cache
	cached, ok := manager.cache.Get(sessionInfo.TokenHash)
	assert.True(t, ok)
	assert.Equal(t, "user-1", cached.UserID)
	assert.Equal(t, "test@example.com", cached.Email)
	assert.Equal(t, "ADMIN", cached.Role)
}

func TestManager_GetByTokenHash_FromCache(t *testing.T) {
	db := setupTestDB(t)
	createTestUser(t, db, "user-1", "test@example.com", "ADMIN")

	manager := NewManager(db, ManagerConfig{
		CacheMaxSize: 100,
		CacheTTL:     5 * time.Minute,
	})
	defer manager.cache.Stop()

	ctx := context.Background()

	// Create session
	input := CreateSessionInput{
		UserID:    "user-1",
		Email:     "test@example.com",
		Role:      "ADMIN",
		ExpiresAt: time.Now().Add(1 * time.Hour),
	}

	sessionInfo, err := manager.Create(ctx, input)
	require.NoError(t, err)

	// Get by token hash - should come from cache
	cached, err := manager.GetByTokenHash(ctx, sessionInfo.TokenHash)
	require.NoError(t, err)
	assert.NotNil(t, cached)
	assert.Equal(t, "user-1", cached.UserID)
	assert.Equal(t, "test@example.com", cached.Email)
}

func TestManager_GetByTokenHash_FromDatabase(t *testing.T) {
	db := setupTestDB(t)
	user := createTestUser(t, db, "user-1", "test@example.com", "ADMIN")

	manager := NewManager(db, ManagerConfig{
		CacheMaxSize: 100,
		CacheTTL:     5 * time.Minute,
	})
	defer manager.cache.Stop()

	ctx := context.Background()

	// Create session directly in database (bypassing cache)
	session := &models.Session{
		ID:             "session-direct",
		UserID:         user.ID,
		TokenHash:      "direct-token-hash",
		ExpiresAt:      time.Now().Add(1 * time.Hour),
		LastActivityAt: time.Now(),
	}
	err := db.Create(session).Error
	require.NoError(t, err)

	// Get by token hash - should come from database
	cached, err := manager.GetByTokenHash(ctx, "direct-token-hash")
	require.NoError(t, err)
	assert.NotNil(t, cached)
	assert.Equal(t, "user-1", cached.UserID)
	assert.Equal(t, "test@example.com", cached.Email)
	assert.Equal(t, "ADMIN", cached.Role)

	// Now it should be in cache
	cachedSession, ok := manager.cache.Get("direct-token-hash")
	assert.True(t, ok)
	assert.Equal(t, "user-1", cachedSession.UserID)
}

func TestManager_GetByTokenHash_NotFound(t *testing.T) {
	db := setupTestDB(t)

	manager := NewManager(db, ManagerConfig{
		CacheMaxSize: 100,
		CacheTTL:     5 * time.Minute,
	})
	defer manager.cache.Stop()

	ctx := context.Background()

	cached, err := manager.GetByTokenHash(ctx, "nonexistent-token-hash")
	require.NoError(t, err)
	assert.Nil(t, cached)
}

func TestManager_GetByTokenHash_Expired(t *testing.T) {
	db := setupTestDB(t)
	user := createTestUser(t, db, "user-1", "test@example.com", "ADMIN")

	manager := NewManager(db, ManagerConfig{
		CacheMaxSize: 100,
		CacheTTL:     5 * time.Minute,
	})
	defer manager.cache.Stop()

	ctx := context.Background()

	// Create an expired session directly in database
	session := &models.Session{
		ID:             "session-expired",
		UserID:         user.ID,
		TokenHash:      "expired-token-hash",
		ExpiresAt:      time.Now().Add(-1 * time.Hour), // Expired
		LastActivityAt: time.Now(),
	}
	err := db.Create(session).Error
	require.NoError(t, err)

	// Get by token hash - should not find it (expired)
	cached, err := manager.GetByTokenHash(ctx, "expired-token-hash")
	require.NoError(t, err)
	assert.Nil(t, cached)
}

func TestManager_GetByID(t *testing.T) {
	db := setupTestDB(t)
	user := createTestUser(t, db, "user-1", "test@example.com", "ADMIN")

	manager := NewManager(db, ManagerConfig{
		CacheMaxSize: 100,
		CacheTTL:     5 * time.Minute,
	})
	defer manager.cache.Stop()

	ctx := context.Background()

	// Create session
	input := CreateSessionInput{
		UserID:    user.ID,
		Email:     "test@example.com",
		Role:      "ADMIN",
		ExpiresAt: time.Now().Add(1 * time.Hour),
	}

	sessionInfo, err := manager.Create(ctx, input)
	require.NoError(t, err)

	// Get by ID
	session, err := manager.GetByID(ctx, sessionInfo.SessionID)
	require.NoError(t, err)
	assert.NotNil(t, session)
	assert.Equal(t, sessionInfo.SessionID, session.ID)
	assert.Equal(t, "user-1", session.UserID)
}

func TestManager_GetByID_NotFound(t *testing.T) {
	db := setupTestDB(t)

	manager := NewManager(db, ManagerConfig{
		CacheMaxSize: 100,
		CacheTTL:     5 * time.Minute,
	})
	defer manager.cache.Stop()

	ctx := context.Background()

	session, err := manager.GetByID(ctx, "nonexistent-id")
	require.NoError(t, err)
	assert.Nil(t, session)
}

func TestManager_GetByUserID(t *testing.T) {
	db := setupTestDB(t)
	createTestUser(t, db, "user-1", "test@example.com", "ADMIN")

	manager := NewManager(db, ManagerConfig{
		CacheMaxSize: 100,
		CacheTTL:     5 * time.Minute,
	})
	defer manager.cache.Stop()

	ctx := context.Background()

	// Create multiple sessions for the same user
	for i := 0; i < 3; i++ {
		input := CreateSessionInput{
			UserID:    "user-1",
			Email:     "test@example.com",
			Role:      "ADMIN",
			ExpiresAt: time.Now().Add(1 * time.Hour),
		}
		_, err := manager.Create(ctx, input)
		require.NoError(t, err)
	}

	// Get sessions by user ID
	sessions, err := manager.GetByUserID(ctx, "user-1")
	require.NoError(t, err)
	assert.Len(t, sessions, 3)
}

func TestManager_GetByUserID_Empty(t *testing.T) {
	db := setupTestDB(t)

	manager := NewManager(db, ManagerConfig{
		CacheMaxSize: 100,
		CacheTTL:     5 * time.Minute,
	})
	defer manager.cache.Stop()

	ctx := context.Background()

	sessions, err := manager.GetByUserID(ctx, "nonexistent-user")
	require.NoError(t, err)
	assert.Len(t, sessions, 0)
}

func TestManager_UpdateLastActivity(t *testing.T) {
	db := setupTestDB(t)
	createTestUser(t, db, "user-1", "test@example.com", "ADMIN")

	manager := NewManager(db, ManagerConfig{
		CacheMaxSize: 100,
		CacheTTL:     5 * time.Minute,
	})
	defer manager.cache.Stop()

	ctx := context.Background()

	// Create session
	input := CreateSessionInput{
		UserID:    "user-1",
		Email:     "test@example.com",
		Role:      "ADMIN",
		ExpiresAt: time.Now().Add(1 * time.Hour),
	}

	sessionInfo, err := manager.Create(ctx, input)
	require.NoError(t, err)

	// Get initial last activity time
	var session models.Session
	err = db.First(&session, "id = ?", sessionInfo.SessionID).Error
	require.NoError(t, err)
	initialActivity := session.LastActivityAt

	// Wait a bit
	time.Sleep(10 * time.Millisecond)

	// Update last activity
	err = manager.UpdateLastActivity(ctx, sessionInfo.SessionID)
	require.NoError(t, err)

	// Verify it was updated
	err = db.First(&session, "id = ?", sessionInfo.SessionID).Error
	require.NoError(t, err)
	assert.True(t, session.LastActivityAt.After(initialActivity))
}

func TestManager_Delete(t *testing.T) {
	db := setupTestDB(t)
	createTestUser(t, db, "user-1", "test@example.com", "ADMIN")

	manager := NewManager(db, ManagerConfig{
		CacheMaxSize: 100,
		CacheTTL:     5 * time.Minute,
	})
	defer manager.cache.Stop()

	ctx := context.Background()

	// Create session
	input := CreateSessionInput{
		UserID:    "user-1",
		Email:     "test@example.com",
		Role:      "ADMIN",
		ExpiresAt: time.Now().Add(1 * time.Hour),
	}

	sessionInfo, err := manager.Create(ctx, input)
	require.NoError(t, err)

	// Verify it exists in cache and database
	_, ok := manager.cache.Get(sessionInfo.TokenHash)
	assert.True(t, ok)

	var dbSession models.Session
	err = db.First(&dbSession, "id = ?", sessionInfo.SessionID).Error
	require.NoError(t, err)

	// Delete the session
	err = manager.Delete(ctx, sessionInfo.SessionID)
	require.NoError(t, err)

	// Verify it's gone from cache
	_, ok = manager.cache.Get(sessionInfo.TokenHash)
	assert.False(t, ok)

	// Verify it's gone from database
	err = db.First(&dbSession, "id = ?", sessionInfo.SessionID).Error
	assert.Error(t, err)
}

func TestManager_Delete_NotFound(t *testing.T) {
	db := setupTestDB(t)

	manager := NewManager(db, ManagerConfig{
		CacheMaxSize: 100,
		CacheTTL:     5 * time.Minute,
	})
	defer manager.cache.Stop()

	ctx := context.Background()

	// Delete nonexistent session should not error
	err := manager.Delete(ctx, "nonexistent-id")
	require.NoError(t, err)
}

func TestManager_DeleteByUserID(t *testing.T) {
	db := setupTestDB(t)
	createTestUser(t, db, "user-1", "test@example.com", "ADMIN")
	createTestUser(t, db, "user-2", "test2@example.com", "PLAYER")

	manager := NewManager(db, ManagerConfig{
		CacheMaxSize: 100,
		CacheTTL:     5 * time.Minute,
	})
	defer manager.cache.Stop()

	ctx := context.Background()

	// Create sessions for user-1
	for i := 0; i < 3; i++ {
		input := CreateSessionInput{
			UserID:    "user-1",
			Email:     "test@example.com",
			Role:      "ADMIN",
			ExpiresAt: time.Now().Add(1 * time.Hour),
		}
		_, err := manager.Create(ctx, input)
		require.NoError(t, err)
	}

	// Create session for user-2
	input := CreateSessionInput{
		UserID:    "user-2",
		Email:     "test2@example.com",
		Role:      "PLAYER",
		ExpiresAt: time.Now().Add(1 * time.Hour),
	}
	_, err := manager.Create(ctx, input)
	require.NoError(t, err)

	// Delete all sessions for user-1
	err = manager.DeleteByUserID(ctx, "user-1")
	require.NoError(t, err)

	// Verify user-1 sessions are gone
	sessions, err := manager.GetByUserID(ctx, "user-1")
	require.NoError(t, err)
	assert.Len(t, sessions, 0)

	// Verify user-2 session still exists
	sessions, err = manager.GetByUserID(ctx, "user-2")
	require.NoError(t, err)
	assert.Len(t, sessions, 1)
}

func TestManager_DeleteExpired(t *testing.T) {
	db := setupTestDB(t)
	createTestUser(t, db, "user-1", "test@example.com", "ADMIN")

	manager := NewManager(db, ManagerConfig{
		CacheMaxSize: 100,
		CacheTTL:     5 * time.Minute,
	})
	defer manager.cache.Stop()

	ctx := context.Background()

	// Create expired session directly in database
	expiredSession := &models.Session{
		ID:             "expired-session",
		UserID:         "user-1",
		TokenHash:      "expired-token",
		ExpiresAt:      time.Now().Add(-1 * time.Hour),
		LastActivityAt: time.Now(),
	}
	err := db.Create(expiredSession).Error
	require.NoError(t, err)

	// Create valid session
	input := CreateSessionInput{
		UserID:    "user-1",
		Email:     "test@example.com",
		Role:      "ADMIN",
		ExpiresAt: time.Now().Add(1 * time.Hour),
	}
	_, err = manager.Create(ctx, input)
	require.NoError(t, err)

	// Delete expired sessions
	deleted, err := manager.DeleteExpired(ctx)
	require.NoError(t, err)
	assert.Equal(t, int64(1), deleted)

	// Verify expired session is gone
	var session models.Session
	err = db.First(&session, "id = ?", "expired-session").Error
	assert.Error(t, err)
}

func TestManager_ClearCache(t *testing.T) {
	db := setupTestDB(t)
	createTestUser(t, db, "user-1", "test@example.com", "ADMIN")

	manager := NewManager(db, ManagerConfig{
		CacheMaxSize: 100,
		CacheTTL:     5 * time.Minute,
	})
	defer manager.cache.Stop()

	ctx := context.Background()

	// Create session
	input := CreateSessionInput{
		UserID:    "user-1",
		Email:     "test@example.com",
		Role:      "ADMIN",
		ExpiresAt: time.Now().Add(1 * time.Hour),
	}
	_, err := manager.Create(ctx, input)
	require.NoError(t, err)

	assert.Equal(t, 1, manager.CacheSize())

	manager.ClearCache()

	assert.Equal(t, 0, manager.CacheSize())
}

func TestManager_CacheSize(t *testing.T) {
	db := setupTestDB(t)
	createTestUser(t, db, "user-1", "test@example.com", "ADMIN")

	manager := NewManager(db, ManagerConfig{
		CacheMaxSize: 100,
		CacheTTL:     5 * time.Minute,
	})
	defer manager.cache.Stop()

	ctx := context.Background()

	assert.Equal(t, 0, manager.CacheSize())

	// Create sessions
	for i := 0; i < 5; i++ {
		input := CreateSessionInput{
			UserID:    "user-1",
			Email:     "test@example.com",
			Role:      "ADMIN",
			ExpiresAt: time.Now().Add(1 * time.Hour),
		}
		_, err := manager.Create(ctx, input)
		require.NoError(t, err)
	}

	assert.Equal(t, 5, manager.CacheSize())
}

// =====================
// Helper Function Tests
// =====================

func TestHashString(t *testing.T) {
	hash1 := hashString("test")
	hash2 := hashString("test")
	hash3 := hashString("different")

	// Same input should produce same hash
	assert.Equal(t, hash1, hash2)

	// Different input should produce different hash
	assert.NotEqual(t, hash1, hash3)

	// Hash should be 64 characters (SHA-256 hex encoding)
	assert.Len(t, hash1, 64)
}
