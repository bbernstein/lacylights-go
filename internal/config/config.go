// Package config provides configuration management for the LacyLights server.
package config

import (
	"os"
	"strconv"
	"time"
)

// Config holds all configuration values for the server.
type Config struct {
	// Server configuration
	Port string
	Env  string

	// Database configuration
	DatabaseURL string

	// DMX configuration
	DMXUniverseCount    int
	DMXRefreshRate      int           // Hz (active)
	DMXIdleRate         int           // Hz (idle)
	DMXHighRateDuration time.Duration // Duration to stay in high rate after changes

	// Fade engine configuration
	FadeUpdateRateHz int // Hz (default 60, for smooth 60fps fades)

	// Art-Net configuration
	ArtNetEnabled   bool
	ArtNetPort      int
	ArtNetBroadcast string

	// Timing monitoring
	DMXDriftThreshold int // Only warn for drifts > threshold (ms)
	DMXDriftThrottle  int // Throttle warnings (ms)

	// Non-interactive mode (for Docker/CI)
	NonInteractive bool

	// CORS configuration
	CORSOrigin   string
	CORSAllowAll bool // Allow all origins (for E2E testing only, not production)

	// OFL (Open Fixture Library) import configuration
	OFLImportEnabled bool   // Enable automatic OFL import on startup
	OFLCachePath     string // Path to cache downloaded OFL data

	// Authentication configuration
	AuthEnabled          bool          // Enable authentication (default: false for easy onboarding)
	JWTSecret            string        // Secret key for signing JWTs (auto-generated if empty)
	JWTIssuer            string        // JWT issuer claim
	JWTAccessTokenTTL    time.Duration // Access token lifetime (default: 15m)
	JWTRefreshTokenTTL   time.Duration // Refresh token lifetime (default: 7d)
	SessionDurationHours int           // Session duration in hours (default: 24)
	PasswordMinLength    int           // Minimum password length (default: 8)
	DeviceAuthEnabled    bool          // Enable device-based authentication
	DefaultAdminEmail    string        // Default admin email (for initial setup)
	DefaultAdminPassword string        // Default admin password (for initial setup)
	AppleClientID        string        // Apple Sign-In client ID (bundle ID)
	AppleTeamID          string        // Apple Sign-In team ID
	AppleKeyID           string        // Apple Sign-In key ID
	ApplePrivateKeyPath  string        // Path to Apple Sign-In private key (.p8)
}

// Load loads configuration from environment variables with sensible defaults.
func Load() *Config {
	env := getEnv("ENV", "development")
	return &Config{
		// Server
		Port: getEnv("PORT", "4000"),
		Env:  env,

		// Database - default filename depends on environment
		DatabaseURL: getEnv("DATABASE_URL", defaultDatabaseURL(env)),

		// DMX
		DMXUniverseCount:    getEnvInt("DMX_UNIVERSE_COUNT", 4),
		DMXRefreshRate:      getEnvInt("DMX_REFRESH_RATE", 60), // Match fade engine default
		DMXIdleRate:         getEnvInt("DMX_IDLE_RATE", 1),
		DMXHighRateDuration: time.Duration(getEnvInt("DMX_HIGH_RATE_DURATION", 2000)) * time.Millisecond,

		// Fade engine
		FadeUpdateRateHz: getEnvInt("FADE_UPDATE_RATE", 60),

		// Art-Net
		ArtNetEnabled:   getEnvBool("ARTNET_ENABLED", true),
		ArtNetPort:      getEnvInt("ARTNET_PORT", 6454),
		ArtNetBroadcast: getEnv("ARTNET_BROADCAST", ""),

		// Timing monitoring
		DMXDriftThreshold: getEnvInt("DMX_DRIFT_THRESHOLD", 50),
		DMXDriftThrottle:  getEnvInt("DMX_DRIFT_THROTTLE", 5000),

		// Non-interactive
		NonInteractive: getEnvBool("NON_INTERACTIVE", false),

		// CORS
		CORSOrigin:   getEnv("CORS_ORIGIN", "http://localhost:3000"),
		CORSAllowAll: getEnvBool("CORS_ALLOW_ALL", false),

		// OFL Import
		OFLImportEnabled: getEnvBool("OFL_IMPORT_ENABLED", true),
		OFLCachePath:     getEnv("OFL_CACHE_PATH", "./.ofl-cache"),

		// Authentication
		AuthEnabled:          getEnvBool("AUTH_ENABLED", false),
		JWTSecret:            getEnv("JWT_SECRET", ""), // Auto-generated on first run if empty
		JWTIssuer:            getEnv("JWT_ISSUER", "lacylights"),
		JWTAccessTokenTTL:    getEnvDuration("JWT_ACCESS_TTL", 15*time.Minute),
		JWTRefreshTokenTTL:   getEnvDuration("JWT_REFRESH_TTL", 168*time.Hour), // 7 days
		SessionDurationHours: getEnvInt("SESSION_DURATION_HOURS", 24),
		PasswordMinLength:    getEnvInt("PASSWORD_MIN_LENGTH", 8),
		DeviceAuthEnabled:    getEnvBool("DEVICE_AUTH_ENABLED", true),
		DefaultAdminEmail:    getEnv("DEFAULT_ADMIN_EMAIL", "admin@lacylights.local"),
		DefaultAdminPassword: getEnv("DEFAULT_ADMIN_PASSWORD", ""),
		AppleClientID:        getEnv("APPLE_CLIENT_ID", ""),
		AppleTeamID:          getEnv("APPLE_TEAM_ID", ""),
		AppleKeyID:           getEnv("APPLE_KEY_ID", ""),
		ApplePrivateKeyPath:  getEnv("APPLE_PRIVATE_KEY_PATH", ""),
	}
}

// IsDevelopment returns true if running in development mode.
func (c *Config) IsDevelopment() bool {
	return c.Env == "development"
}

// IsProduction returns true if running in production mode.
func (c *Config) IsProduction() bool {
	return c.Env == "production"
}

// getEnv returns the value of an environment variable or a default value.
func getEnv(key, defaultValue string) string {
	if value, exists := os.LookupEnv(key); exists {
		return value
	}
	return defaultValue
}

// getEnvInt returns the integer value of an environment variable or a default value.
func getEnvInt(key string, defaultValue int) int {
	if value, exists := os.LookupEnv(key); exists {
		if intVal, err := strconv.Atoi(value); err == nil {
			return intVal
		}
	}
	return defaultValue
}

// getEnvBool returns the boolean value of an environment variable or a default value.
func getEnvBool(key string, defaultValue bool) bool {
	if value, exists := os.LookupEnv(key); exists {
		if boolVal, err := strconv.ParseBool(value); err == nil {
			return boolVal
		}
	}
	return defaultValue
}

// getEnvDuration returns the duration value of an environment variable or a default value.
// Accepts Go duration format (e.g., "15m", "24h", "7d" where d is converted to hours).
func getEnvDuration(key string, defaultValue time.Duration) time.Duration {
	if value, exists := os.LookupEnv(key); exists {
		// Handle "d" suffix for days (not natively supported by time.ParseDuration)
		if len(value) > 1 && value[len(value)-1] == 'd' {
			if days, err := strconv.Atoi(value[:len(value)-1]); err == nil {
				return time.Duration(days) * 24 * time.Hour
			}
		}
		if duration, err := time.ParseDuration(value); err == nil {
			return duration
		}
	}
	return defaultValue
}

// defaultDatabaseURL returns the default database URL based on the environment.
// This allows different database files for different purposes:
//   - development: dev.db (default for local development)
//   - production: lacylights.db (production data)
//   - integration: integration.db (integration tests, auto-cleaned)
//   - e2e: e2e.db (end-to-end tests, auto-cleaned)
//   - test: dev.db (unit tests typically use in-memory via DATABASE_URL override)
func defaultDatabaseURL(env string) string {
	switch env {
	case "production":
		return "file:./lacylights.db"
	case "integration":
		return "file:./integration.db"
	case "e2e":
		return "file:./e2e.db"
	default:
		// development and test use dev.db by default
		return "file:./dev.db"
	}
}
