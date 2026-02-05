// Package session provides session management for authentication.
package session

import (
	"sync"
	"time"
)

// CachedSession represents a session stored in the in-memory cache.
// This provides O(1) lookups without database hits for authenticated requests.
type CachedSession struct {
	UserID       string
	Email        string
	Role         string
	SessionID    string
	DeviceID     *string
	ExpiresAt    time.Time
	CachedAt     time.Time
}

// Cache provides an in-memory cache for sessions.
// It's designed for O(1) lookups to avoid database hits during authentication.
type Cache struct {
	mu       sync.RWMutex
	cache    map[string]*CachedSession // tokenHash -> session
	maxSize  int
	ttl      time.Duration
	stopChan chan struct{}
}

// CacheConfig holds configuration for the session cache.
type CacheConfig struct {
	MaxSize int           // Maximum number of cached sessions (default: 1000)
	TTL     time.Duration // Time-to-live for cached entries (default: 5 minutes)
}

// NewCache creates a new session cache with the given configuration.
func NewCache(cfg CacheConfig) *Cache {
	maxSize := cfg.MaxSize
	if maxSize <= 0 {
		maxSize = 1000
	}

	ttl := cfg.TTL
	if ttl <= 0 {
		ttl = 5 * time.Minute
	}

	c := &Cache{
		cache:    make(map[string]*CachedSession),
		maxSize:  maxSize,
		ttl:      ttl,
		stopChan: make(chan struct{}),
	}

	// Start background cleanup goroutine
	go c.cleanupLoop()

	return c
}

// Stop stops the cache cleanup goroutine.
// Call this when shutting down the application to prevent goroutine leaks.
func (c *Cache) Stop() {
	close(c.stopChan)
}

// Get retrieves a session from the cache by token hash.
// Returns nil, false if the session is not found or has expired.
func (c *Cache) Get(tokenHash string) (*CachedSession, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	session, ok := c.cache[tokenHash]
	if !ok {
		return nil, false
	}

	// Check if cache entry has expired (local TTL)
	if time.Since(session.CachedAt) > c.ttl {
		// Entry expired, return not found (will be cleaned up in background)
		return nil, false
	}

	// Check if session itself has expired
	if time.Now().After(session.ExpiresAt) {
		return nil, false
	}

	return session, true
}

// Set stores a session in the cache.
func (c *Cache) Set(tokenHash string, session *CachedSession) {
	c.mu.Lock()
	defer c.mu.Unlock()

	// Check if we need to evict entries
	if len(c.cache) >= c.maxSize {
		c.evictOldest()
	}

	session.CachedAt = time.Now()
	c.cache[tokenHash] = session
}

// Delete removes a session from the cache.
func (c *Cache) Delete(tokenHash string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.cache, tokenHash)
}

// DeleteByUserID removes all sessions for a user from the cache.
func (c *Cache) DeleteByUserID(userID string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	for tokenHash, session := range c.cache {
		if session.UserID == userID {
			delete(c.cache, tokenHash)
		}
	}
}

// DeleteBySessionID removes a specific session from the cache.
func (c *Cache) DeleteBySessionID(sessionID string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	for tokenHash, session := range c.cache {
		if session.SessionID == sessionID {
			delete(c.cache, tokenHash)
		}
	}
}

// Clear removes all sessions from the cache.
func (c *Cache) Clear() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.cache = make(map[string]*CachedSession)
}

// Size returns the current number of cached sessions.
func (c *Cache) Size() int {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return len(c.cache)
}

// evictOldest removes the oldest cache entry (by CachedAt time).
// Must be called with lock held.
func (c *Cache) evictOldest() {
	var oldestKey string
	var oldestTime time.Time

	for key, session := range c.cache {
		if oldestKey == "" || session.CachedAt.Before(oldestTime) {
			oldestKey = key
			oldestTime = session.CachedAt
		}
	}

	if oldestKey != "" {
		delete(c.cache, oldestKey)
	}
}

// cleanupLoop periodically removes expired entries from the cache.
func (c *Cache) cleanupLoop() {
	ticker := time.NewTicker(c.ttl / 2)
	defer ticker.Stop()

	for {
		select {
		case <-c.stopChan:
			return
		case <-ticker.C:
			c.cleanup()
		}
	}
}

// cleanup removes expired entries from the cache.
func (c *Cache) cleanup() {
	c.mu.Lock()
	defer c.mu.Unlock()

	now := time.Now()
	for tokenHash, session := range c.cache {
		// Remove if cache TTL expired or session expired
		if time.Since(session.CachedAt) > c.ttl || now.After(session.ExpiresAt) {
			delete(c.cache, tokenHash)
		}
	}
}
