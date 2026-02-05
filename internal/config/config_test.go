package config

import (
	"testing"
	"time"
)

func TestGetEnv_ReturnsDefault_WhenEnvNotSet(t *testing.T) {
	// Test that getEnv returns default values when environment variables are not set
	// Using unique keys that are guaranteed to not exist in the environment

	// Test string default
	result := getEnv("LACYLIGHTS_TEST_NONEXISTENT_PORT_12345", "4000")
	if result != "4000" {
		t.Errorf("Expected default '4000', got '%s'", result)
	}

	// Test another string default
	result = getEnv("LACYLIGHTS_TEST_NONEXISTENT_ENV_12345", "development")
	if result != "development" {
		t.Errorf("Expected default 'development', got '%s'", result)
	}

	// Test int default
	intResult := getEnvInt("LACYLIGHTS_TEST_NONEXISTENT_INT_12345", 44)
	if intResult != 44 {
		t.Errorf("Expected default 44, got %d", intResult)
	}

	// Test bool default (true)
	boolResult := getEnvBool("LACYLIGHTS_TEST_NONEXISTENT_BOOL_TRUE_12345", true)
	if !boolResult {
		t.Error("Expected default true, got false")
	}

	// Test bool default (false)
	boolResult = getEnvBool("LACYLIGHTS_TEST_NONEXISTENT_BOOL_FALSE_12345", false)
	if boolResult {
		t.Error("Expected default false, got true")
	}
}

func TestLoad_CustomEnvironment(t *testing.T) {
	// Set custom environment variables using t.Setenv (auto cleanup)
	t.Setenv("PORT", "8080")
	t.Setenv("ENV", "production")
	t.Setenv("DATABASE_URL", "file:./prod.db")
	t.Setenv("DMX_UNIVERSE_COUNT", "8")
	t.Setenv("DMX_REFRESH_RATE", "30")
	t.Setenv("DMX_IDLE_RATE", "5")
	t.Setenv("DMX_HIGH_RATE_DURATION", "3000")
	t.Setenv("ARTNET_ENABLED", "false")
	t.Setenv("ARTNET_PORT", "6455")
	t.Setenv("ARTNET_BROADCAST", "192.168.1.255")
	t.Setenv("DMX_DRIFT_THRESHOLD", "100")
	t.Setenv("DMX_DRIFT_THROTTLE", "10000")
	t.Setenv("NON_INTERACTIVE", "true")
	t.Setenv("CORS_ORIGIN", "http://example.com")
	t.Setenv("CORS_ALLOW_ALL", "true")
	t.Setenv("FADE_UPDATE_RATE", "120")

	cfg := Load()

	if cfg.Port != "8080" {
		t.Errorf("Expected Port to be '8080', got '%s'", cfg.Port)
	}
	if cfg.Env != "production" {
		t.Errorf("Expected Env to be 'production', got '%s'", cfg.Env)
	}
	if cfg.DatabaseURL != "file:./prod.db" {
		t.Errorf("Expected DatabaseURL to be 'file:./prod.db', got '%s'", cfg.DatabaseURL)
	}
	if cfg.DMXUniverseCount != 8 {
		t.Errorf("Expected DMXUniverseCount to be 8, got %d", cfg.DMXUniverseCount)
	}
	if cfg.DMXRefreshRate != 30 {
		t.Errorf("Expected DMXRefreshRate to be 30, got %d", cfg.DMXRefreshRate)
	}
	if cfg.DMXIdleRate != 5 {
		t.Errorf("Expected DMXIdleRate to be 5, got %d", cfg.DMXIdleRate)
	}
	if cfg.DMXHighRateDuration != 3000*time.Millisecond {
		t.Errorf("Expected DMXHighRateDuration to be 3000ms, got %v", cfg.DMXHighRateDuration)
	}
	if cfg.ArtNetEnabled != false {
		t.Errorf("Expected ArtNetEnabled to be false, got %v", cfg.ArtNetEnabled)
	}
	if cfg.ArtNetPort != 6455 {
		t.Errorf("Expected ArtNetPort to be 6455, got %d", cfg.ArtNetPort)
	}
	if cfg.ArtNetBroadcast != "192.168.1.255" {
		t.Errorf("Expected ArtNetBroadcast to be '192.168.1.255', got '%s'", cfg.ArtNetBroadcast)
	}
	if cfg.DMXDriftThreshold != 100 {
		t.Errorf("Expected DMXDriftThreshold to be 100, got %d", cfg.DMXDriftThreshold)
	}
	if cfg.DMXDriftThrottle != 10000 {
		t.Errorf("Expected DMXDriftThrottle to be 10000, got %d", cfg.DMXDriftThrottle)
	}
	if cfg.NonInteractive != true {
		t.Errorf("Expected NonInteractive to be true, got %v", cfg.NonInteractive)
	}
	if cfg.CORSOrigin != "http://example.com" {
		t.Errorf("Expected CORSOrigin to be 'http://example.com', got '%s'", cfg.CORSOrigin)
	}
	if cfg.CORSAllowAll != true {
		t.Errorf("Expected CORSAllowAll to be true, got %v", cfg.CORSAllowAll)
	}
	if cfg.FadeUpdateRateHz != 120 {
		t.Errorf("Expected FadeUpdateRateHz to be 120, got %d", cfg.FadeUpdateRateHz)
	}
}

func TestIsDevelopment(t *testing.T) {
	tests := []struct {
		env      string
		expected bool
	}{
		{"development", true},
		{"production", false},
		{"staging", false},
		{"", false},
	}

	for _, tt := range tests {
		t.Run(tt.env, func(t *testing.T) {
			cfg := &Config{Env: tt.env}
			if got := cfg.IsDevelopment(); got != tt.expected {
				t.Errorf("IsDevelopment() = %v, want %v for env '%s'", got, tt.expected, tt.env)
			}
		})
	}
}

func TestIsProduction(t *testing.T) {
	tests := []struct {
		env      string
		expected bool
	}{
		{"production", true},
		{"development", false},
		{"staging", false},
		{"", false},
	}

	for _, tt := range tests {
		t.Run(tt.env, func(t *testing.T) {
			cfg := &Config{Env: tt.env}
			if got := cfg.IsProduction(); got != tt.expected {
				t.Errorf("IsProduction() = %v, want %v for env '%s'", got, tt.expected, tt.env)
			}
		})
	}
}

func TestGetEnv(t *testing.T) {
	// Test with existing env var
	t.Setenv("TEST_GET_ENV", "custom_value")

	result := getEnv("TEST_GET_ENV", "default")
	if result != "custom_value" {
		t.Errorf("Expected 'custom_value', got '%s'", result)
	}

	// Test with non-existing env var (use a unique key that won't be set)
	result = getEnv("NON_EXISTING_VAR_12345_UNIQUE", "default_value")
	if result != "default_value" {
		t.Errorf("Expected 'default_value', got '%s'", result)
	}
}

func TestGetEnvInt(t *testing.T) {
	// Test with valid int
	t.Setenv("TEST_INT_VAR", "42")

	result := getEnvInt("TEST_INT_VAR", 10)
	if result != 42 {
		t.Errorf("Expected 42, got %d", result)
	}

	// Test with invalid int (should return default)
	t.Setenv("TEST_INVALID_INT", "not_a_number")

	result = getEnvInt("TEST_INVALID_INT", 10)
	if result != 10 {
		t.Errorf("Expected default 10 for invalid int, got %d", result)
	}

	// Test with non-existing env var
	result = getEnvInt("NON_EXISTING_INT_VAR_12345_UNIQUE", 100)
	if result != 100 {
		t.Errorf("Expected default 100, got %d", result)
	}
}

func TestGetEnvBool(t *testing.T) {
	tests := []struct {
		name         string
		envValue     string
		defaultValue bool
		expected     bool
		setEnv       bool
	}{
		{"true_string", "true", false, true, true},
		{"false_string", "false", true, false, true},
		{"1_string", "1", false, true, true},
		{"0_string", "0", true, false, true},
		{"invalid_string_returns_default", "invalid", true, true, true},
		{"non_existing_returns_default_true", "", true, true, false},
		{"non_existing_returns_default_false", "", false, false, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Use a unique env key for each test
			envKey := "TEST_BOOL_VAR_" + tt.name + "_UNIQUE"
			if tt.setEnv {
				t.Setenv(envKey, tt.envValue)
			}

			result := getEnvBool(envKey, tt.defaultValue)
			if result != tt.expected {
				t.Errorf("getEnvBool(%s, %v) = %v, want %v", envKey, tt.defaultValue, result, tt.expected)
			}
		})
	}
}

func TestGetEnvInt_ZeroValue(t *testing.T) {
	t.Setenv("TEST_ZERO_INT", "0")

	result := getEnvInt("TEST_ZERO_INT", 10)
	if result != 0 {
		t.Errorf("Expected 0, got %d", result)
	}
}

func TestGetEnvBool_VariousTrue(t *testing.T) {
	trueValues := []string{"true", "TRUE", "True", "1", "t", "T"}
	for _, val := range trueValues {
		t.Run(val, func(t *testing.T) {
			envKey := "TEST_BOOL_TRUE_" + val
			t.Setenv(envKey, val)
			result := getEnvBool(envKey, false)
			if !result {
				t.Errorf("getEnvBool with value '%s' should be true", val)
			}
		})
	}
}

func TestGetEnvBool_VariousFalse(t *testing.T) {
	falseValues := []string{"false", "FALSE", "False", "0", "f", "F"}
	for _, val := range falseValues {
		t.Run(val, func(t *testing.T) {
			envKey := "TEST_BOOL_FALSE_" + val
			t.Setenv(envKey, val)
			result := getEnvBool(envKey, true)
			if result {
				t.Errorf("getEnvBool with value '%s' should be false", val)
			}
		})
	}
}

func TestConfig_StructFields(t *testing.T) {
	// Test that all struct fields are accessible
	cfg := &Config{
		Port:                "4000",
		Env:                 "test",
		DatabaseURL:         "test.db",
		DMXUniverseCount:    4,
		DMXRefreshRate:      60,
		DMXIdleRate:         1,
		DMXHighRateDuration: time.Second,
		FadeUpdateRateHz:    60,
		ArtNetEnabled:       true,
		ArtNetPort:          6454,
		ArtNetBroadcast:     "255.255.255.255",
		DMXDriftThreshold:   50,
		DMXDriftThrottle:    5000,
		NonInteractive:      false,
		CORSOrigin:          "http://localhost",
		CORSAllowAll:        true,
	}

	if cfg.Port != "4000" {
		t.Error("Port field access failed")
	}
	if cfg.DMXUniverseCount != 4 {
		t.Error("DMXUniverseCount field access failed")
	}
	if cfg.ArtNetEnabled != true {
		t.Error("ArtNetEnabled field access failed")
	}
	if cfg.FadeUpdateRateHz != 60 {
		t.Error("FadeUpdateRateHz field access failed")
	}
}

func TestLoad_FadeUpdateRateHz_DefaultValue(t *testing.T) {
	// Test that FadeUpdateRateHz defaults to 60 when not set
	// Use a fresh config with no FADE_UPDATE_RATE env var
	cfg := Load()

	if cfg.FadeUpdateRateHz != 60 {
		t.Errorf("Expected default FadeUpdateRateHz to be 60, got %d", cfg.FadeUpdateRateHz)
	}
}

func TestLoad_CORSAllowAll(t *testing.T) {
	tests := []struct {
		name     string
		envValue string
		expected bool
		setEnv   bool
	}{
		{"enabled_true", "true", true, true},
		{"enabled_1", "1", true, true},
		{"disabled_false", "false", false, true},
		{"disabled_0", "0", false, true},
		{"default_not_set", "", false, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.setEnv {
				t.Setenv("CORS_ALLOW_ALL", tt.envValue)
			}
			cfg := Load()
			if cfg.CORSAllowAll != tt.expected {
				t.Errorf("CORSAllowAll = %v, want %v for env value '%s'", cfg.CORSAllowAll, tt.expected, tt.envValue)
			}
		})
	}
}

func TestDefaultDatabaseURL(t *testing.T) {
	tests := []struct {
		env      string
		expected string
	}{
		{"development", "file:./dev.db"},
		{"production", "file:./lacylights.db"},
		{"integration", "file:./integration.db"},
		{"e2e", "file:./e2e.db"},
		{"test", "file:./dev.db"},
		{"", "file:./dev.db"},
		{"unknown", "file:./dev.db"},
	}

	for _, tt := range tests {
		t.Run(tt.env, func(t *testing.T) {
			result := defaultDatabaseURL(tt.env)
			if result != tt.expected {
				t.Errorf("defaultDatabaseURL(%q) = %q, want %q", tt.env, result, tt.expected)
			}
		})
	}
}

func TestLoad_DatabaseURL_DefaultsByEnvironment(t *testing.T) {
	tests := []struct {
		env         string
		expectedURL string
	}{
		{"development", "file:./dev.db"},
		{"production", "file:./lacylights.db"},
		{"integration", "file:./integration.db"},
		{"e2e", "file:./e2e.db"},
	}

	for _, tt := range tests {
		t.Run(tt.env, func(t *testing.T) {
			t.Setenv("ENV", tt.env)
			// Ensure DATABASE_URL is not set so we get the default
			// t.Setenv with empty string still sets the var, so we need to
			// test without setting it - but other tests may have set it.
			// The safest approach is to check that the defaultDatabaseURL
			// function itself works correctly (tested above).

			cfg := Load()
			if cfg.Env != tt.env {
				t.Errorf("Expected Env to be %q, got %q", tt.env, cfg.Env)
			}
			// If DATABASE_URL is not set by environment, it should use the default
			if cfg.DatabaseURL != tt.expectedURL {
				t.Errorf("Expected DatabaseURL to be %q for env %q, got %q", tt.expectedURL, tt.env, cfg.DatabaseURL)
			}
		})
	}
}

func TestLoad_DatabaseURL_OverridesTakePrecedence(t *testing.T) {
	// Test that explicit DATABASE_URL overrides the environment-based default
	t.Setenv("ENV", "production")
	t.Setenv("DATABASE_URL", "file:./custom.db")

	cfg := Load()

	if cfg.DatabaseURL != "file:./custom.db" {
		t.Errorf("Expected DATABASE_URL override to take precedence, got %q", cfg.DatabaseURL)
	}
}

func TestGetEnvDuration(t *testing.T) {
	tests := []struct {
		name         string
		envValue     string
		defaultValue time.Duration
		expected     time.Duration
		setEnv       bool
	}{
		{
			name:         "valid_days_suffix",
			envValue:     "7d",
			defaultValue: time.Hour,
			expected:     7 * 24 * time.Hour,
			setEnv:       true,
		},
		{
			name:         "valid_days_suffix_single_day",
			envValue:     "1d",
			defaultValue: time.Hour,
			expected:     24 * time.Hour,
			setEnv:       true,
		},
		{
			name:         "invalid_days_suffix_non_numeric",
			envValue:     "abcd",
			defaultValue: 5 * time.Minute,
			expected:     5 * time.Minute, // returns default
			setEnv:       true,
		},
		{
			name:         "valid_go_duration_minutes",
			envValue:     "15m",
			defaultValue: time.Hour,
			expected:     15 * time.Minute,
			setEnv:       true,
		},
		{
			name:         "valid_go_duration_hours",
			envValue:     "24h",
			defaultValue: time.Minute,
			expected:     24 * time.Hour,
			setEnv:       true,
		},
		{
			name:         "valid_go_duration_seconds",
			envValue:     "30s",
			defaultValue: time.Hour,
			expected:     30 * time.Second,
			setEnv:       true,
		},
		{
			name:         "invalid_duration_format",
			envValue:     "invalid",
			defaultValue: 10 * time.Minute,
			expected:     10 * time.Minute, // returns default
			setEnv:       true,
		},
		{
			name:         "env_not_set_returns_default",
			envValue:     "",
			defaultValue: 20 * time.Minute,
			expected:     20 * time.Minute,
			setEnv:       false,
		},
		{
			name:         "single_char_d_not_treated_as_days",
			envValue:     "d",
			defaultValue: 3 * time.Hour,
			expected:     3 * time.Hour, // "d" alone is not valid (len must be > 1)
			setEnv:       true,
		},
		{
			name:         "empty_string_returns_default",
			envValue:     "",
			defaultValue: 15 * time.Minute,
			expected:     15 * time.Minute,
			setEnv:       true, // set to empty string
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			envKey := "TEST_DURATION_VAR_" + tt.name + "_UNIQUE"
			if tt.setEnv {
				t.Setenv(envKey, tt.envValue)
			}

			result := getEnvDuration(envKey, tt.defaultValue)
			if result != tt.expected {
				t.Errorf("getEnvDuration(%s, %v) = %v, want %v", envKey, tt.defaultValue, result, tt.expected)
			}
		})
	}
}

func TestLoad_AuthenticationConfig(t *testing.T) {
	// Test AUTH_ENABLED
	t.Run("auth_enabled_true", func(t *testing.T) {
		t.Setenv("AUTH_ENABLED", "true")
		cfg := Load()
		if !cfg.AuthEnabled {
			t.Error("Expected AuthEnabled to be true")
		}
	})

	t.Run("auth_enabled_false", func(t *testing.T) {
		t.Setenv("AUTH_ENABLED", "false")
		cfg := Load()
		if cfg.AuthEnabled {
			t.Error("Expected AuthEnabled to be false")
		}
	})

	t.Run("auth_enabled_default", func(t *testing.T) {
		// Don't set AUTH_ENABLED
		cfg := Load()
		if cfg.AuthEnabled {
			t.Error("Expected AuthEnabled to default to false")
		}
	})
}

func TestLoad_JWTConfig(t *testing.T) {
	// Test JWT_SECRET
	t.Run("jwt_secret_custom", func(t *testing.T) {
		t.Setenv("JWT_SECRET", "my-super-secret-key")
		cfg := Load()
		if cfg.JWTSecret != "my-super-secret-key" {
			t.Errorf("Expected JWTSecret to be 'my-super-secret-key', got '%s'", cfg.JWTSecret)
		}
	})

	t.Run("jwt_secret_default_empty", func(t *testing.T) {
		cfg := Load()
		if cfg.JWTSecret != "" {
			t.Errorf("Expected JWTSecret to default to empty string, got '%s'", cfg.JWTSecret)
		}
	})

	// Test JWT_ACCESS_TTL
	t.Run("jwt_access_ttl_custom", func(t *testing.T) {
		t.Setenv("JWT_ACCESS_TTL", "30m")
		cfg := Load()
		if cfg.JWTAccessTokenTTL != 30*time.Minute {
			t.Errorf("Expected JWTAccessTokenTTL to be 30m, got %v", cfg.JWTAccessTokenTTL)
		}
	})

	t.Run("jwt_access_ttl_default", func(t *testing.T) {
		cfg := Load()
		if cfg.JWTAccessTokenTTL != 15*time.Minute {
			t.Errorf("Expected JWTAccessTokenTTL to default to 15m, got %v", cfg.JWTAccessTokenTTL)
		}
	})

	// Test JWT_REFRESH_TTL with days suffix
	t.Run("jwt_refresh_ttl_custom_days", func(t *testing.T) {
		t.Setenv("JWT_REFRESH_TTL", "14d")
		cfg := Load()
		if cfg.JWTRefreshTokenTTL != 14*24*time.Hour {
			t.Errorf("Expected JWTRefreshTokenTTL to be 14 days, got %v", cfg.JWTRefreshTokenTTL)
		}
	})

	t.Run("jwt_refresh_ttl_custom_hours", func(t *testing.T) {
		t.Setenv("JWT_REFRESH_TTL", "72h")
		cfg := Load()
		if cfg.JWTRefreshTokenTTL != 72*time.Hour {
			t.Errorf("Expected JWTRefreshTokenTTL to be 72h, got %v", cfg.JWTRefreshTokenTTL)
		}
	})

	t.Run("jwt_refresh_ttl_default", func(t *testing.T) {
		cfg := Load()
		if cfg.JWTRefreshTokenTTL != 168*time.Hour {
			t.Errorf("Expected JWTRefreshTokenTTL to default to 168h (7 days), got %v", cfg.JWTRefreshTokenTTL)
		}
	})

	// Test JWT_ISSUER
	t.Run("jwt_issuer_custom", func(t *testing.T) {
		t.Setenv("JWT_ISSUER", "my-custom-issuer")
		cfg := Load()
		if cfg.JWTIssuer != "my-custom-issuer" {
			t.Errorf("Expected JWTIssuer to be 'my-custom-issuer', got '%s'", cfg.JWTIssuer)
		}
	})

	t.Run("jwt_issuer_default", func(t *testing.T) {
		cfg := Load()
		if cfg.JWTIssuer != "lacylights" {
			t.Errorf("Expected JWTIssuer to default to 'lacylights', got '%s'", cfg.JWTIssuer)
		}
	})
}

func TestLoad_DefaultAdminConfig(t *testing.T) {
	// Test DEFAULT_ADMIN_EMAIL
	t.Run("default_admin_email_custom", func(t *testing.T) {
		t.Setenv("DEFAULT_ADMIN_EMAIL", "admin@example.com")
		cfg := Load()
		if cfg.DefaultAdminEmail != "admin@example.com" {
			t.Errorf("Expected DefaultAdminEmail to be 'admin@example.com', got '%s'", cfg.DefaultAdminEmail)
		}
	})

	t.Run("default_admin_email_default", func(t *testing.T) {
		cfg := Load()
		if cfg.DefaultAdminEmail != "admin@lacylights.local" {
			t.Errorf("Expected DefaultAdminEmail to default to 'admin@lacylights.local', got '%s'", cfg.DefaultAdminEmail)
		}
	})

	// Test DEFAULT_ADMIN_PASSWORD
	t.Run("default_admin_password_custom", func(t *testing.T) {
		t.Setenv("DEFAULT_ADMIN_PASSWORD", "mysecretpassword")
		cfg := Load()
		if cfg.DefaultAdminPassword != "mysecretpassword" {
			t.Errorf("Expected DefaultAdminPassword to be 'mysecretpassword', got '%s'", cfg.DefaultAdminPassword)
		}
	})

	t.Run("default_admin_password_default_empty", func(t *testing.T) {
		cfg := Load()
		if cfg.DefaultAdminPassword != "" {
			t.Errorf("Expected DefaultAdminPassword to default to empty string, got '%s'", cfg.DefaultAdminPassword)
		}
	})
}

func TestLoad_PasswordConfig(t *testing.T) {
	// Test PASSWORD_MIN_LENGTH
	t.Run("password_min_length_custom", func(t *testing.T) {
		t.Setenv("PASSWORD_MIN_LENGTH", "12")
		cfg := Load()
		if cfg.PasswordMinLength != 12 {
			t.Errorf("Expected PasswordMinLength to be 12, got %d", cfg.PasswordMinLength)
		}
	})

	t.Run("password_min_length_default", func(t *testing.T) {
		cfg := Load()
		if cfg.PasswordMinLength != 8 {
			t.Errorf("Expected PasswordMinLength to default to 8, got %d", cfg.PasswordMinLength)
		}
	})
}

func TestLoad_DeviceAuthConfig(t *testing.T) {
	// Test DEVICE_AUTH_ENABLED
	t.Run("device_auth_enabled_true", func(t *testing.T) {
		t.Setenv("DEVICE_AUTH_ENABLED", "true")
		cfg := Load()
		if !cfg.DeviceAuthEnabled {
			t.Error("Expected DeviceAuthEnabled to be true")
		}
	})

	t.Run("device_auth_enabled_false", func(t *testing.T) {
		t.Setenv("DEVICE_AUTH_ENABLED", "false")
		cfg := Load()
		if cfg.DeviceAuthEnabled {
			t.Error("Expected DeviceAuthEnabled to be false")
		}
	})

	t.Run("device_auth_enabled_default", func(t *testing.T) {
		cfg := Load()
		if !cfg.DeviceAuthEnabled {
			t.Error("Expected DeviceAuthEnabled to default to true")
		}
	})
}

func TestLoad_SessionConfig(t *testing.T) {
	// Test SESSION_DURATION_HOURS
	t.Run("session_duration_hours_custom", func(t *testing.T) {
		t.Setenv("SESSION_DURATION_HOURS", "48")
		cfg := Load()
		if cfg.SessionDurationHours != 48 {
			t.Errorf("Expected SessionDurationHours to be 48, got %d", cfg.SessionDurationHours)
		}
	})

	t.Run("session_duration_hours_default", func(t *testing.T) {
		cfg := Load()
		if cfg.SessionDurationHours != 24 {
			t.Errorf("Expected SessionDurationHours to default to 24, got %d", cfg.SessionDurationHours)
		}
	})
}

func TestLoad_AppleSignInConfig(t *testing.T) {
	// Test Apple Sign-In configuration
	t.Run("apple_client_id_custom", func(t *testing.T) {
		t.Setenv("APPLE_CLIENT_ID", "com.example.app")
		cfg := Load()
		if cfg.AppleClientID != "com.example.app" {
			t.Errorf("Expected AppleClientID to be 'com.example.app', got '%s'", cfg.AppleClientID)
		}
	})

	t.Run("apple_client_id_default_empty", func(t *testing.T) {
		cfg := Load()
		if cfg.AppleClientID != "" {
			t.Errorf("Expected AppleClientID to default to empty string, got '%s'", cfg.AppleClientID)
		}
	})

	t.Run("apple_team_id_custom", func(t *testing.T) {
		t.Setenv("APPLE_TEAM_ID", "ABC123DEF")
		cfg := Load()
		if cfg.AppleTeamID != "ABC123DEF" {
			t.Errorf("Expected AppleTeamID to be 'ABC123DEF', got '%s'", cfg.AppleTeamID)
		}
	})

	t.Run("apple_team_id_default_empty", func(t *testing.T) {
		cfg := Load()
		if cfg.AppleTeamID != "" {
			t.Errorf("Expected AppleTeamID to default to empty string, got '%s'", cfg.AppleTeamID)
		}
	})

	t.Run("apple_key_id_custom", func(t *testing.T) {
		t.Setenv("APPLE_KEY_ID", "KEY123")
		cfg := Load()
		if cfg.AppleKeyID != "KEY123" {
			t.Errorf("Expected AppleKeyID to be 'KEY123', got '%s'", cfg.AppleKeyID)
		}
	})

	t.Run("apple_key_id_default_empty", func(t *testing.T) {
		cfg := Load()
		if cfg.AppleKeyID != "" {
			t.Errorf("Expected AppleKeyID to default to empty string, got '%s'", cfg.AppleKeyID)
		}
	})

	t.Run("apple_private_key_path_custom", func(t *testing.T) {
		t.Setenv("APPLE_PRIVATE_KEY_PATH", "/path/to/key.p8")
		cfg := Load()
		if cfg.ApplePrivateKeyPath != "/path/to/key.p8" {
			t.Errorf("Expected ApplePrivateKeyPath to be '/path/to/key.p8', got '%s'", cfg.ApplePrivateKeyPath)
		}
	})

	t.Run("apple_private_key_path_default_empty", func(t *testing.T) {
		cfg := Load()
		if cfg.ApplePrivateKeyPath != "" {
			t.Errorf("Expected ApplePrivateKeyPath to default to empty string, got '%s'", cfg.ApplePrivateKeyPath)
		}
	})
}

func TestLoad_OFLConfig(t *testing.T) {
	// Test OFL_IMPORT_ENABLED
	t.Run("ofl_import_enabled_false", func(t *testing.T) {
		t.Setenv("OFL_IMPORT_ENABLED", "false")
		cfg := Load()
		if cfg.OFLImportEnabled {
			t.Error("Expected OFLImportEnabled to be false")
		}
	})

	t.Run("ofl_import_enabled_default_true", func(t *testing.T) {
		cfg := Load()
		if !cfg.OFLImportEnabled {
			t.Error("Expected OFLImportEnabled to default to true")
		}
	})

	// Test OFL_CACHE_PATH
	t.Run("ofl_cache_path_custom", func(t *testing.T) {
		t.Setenv("OFL_CACHE_PATH", "/custom/ofl/path")
		cfg := Load()
		if cfg.OFLCachePath != "/custom/ofl/path" {
			t.Errorf("Expected OFLCachePath to be '/custom/ofl/path', got '%s'", cfg.OFLCachePath)
		}
	})

	t.Run("ofl_cache_path_default", func(t *testing.T) {
		cfg := Load()
		if cfg.OFLCachePath != "./.ofl-cache" {
			t.Errorf("Expected OFLCachePath to default to './.ofl-cache', got '%s'", cfg.OFLCachePath)
		}
	})
}
