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
	CORSOrigin string

	// OFL (Open Fixture Library) import configuration
	OFLImportEnabled bool   // Enable automatic OFL import on startup
	OFLCachePath     string // Path to cache downloaded OFL data
}

// Load loads configuration from environment variables with sensible defaults.
func Load() *Config {
	return &Config{
		// Server
		Port: getEnv("PORT", "4000"),
		Env:  getEnv("ENV", "development"),

		// Database
		DatabaseURL: getEnv("DATABASE_URL", "file:./dev.db"),

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
		CORSOrigin: getEnv("CORS_ORIGIN", "http://localhost:3000"),

		// OFL Import
		OFLImportEnabled: getEnvBool("OFL_IMPORT_ENABLED", true),
		OFLCachePath:     getEnv("OFL_CACHE_PATH", "./.ofl-cache"),
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
