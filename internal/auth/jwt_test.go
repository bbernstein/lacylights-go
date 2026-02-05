package auth

import (
	"testing"
	"time"
)

func TestNewJWTService_WithSecret(t *testing.T) {
	svc, err := NewJWTService(JWTConfig{
		Secret:          "test-secret-key-at-least-32-chars",
		Issuer:          "test-issuer",
		AccessTokenTTL:  15 * time.Minute,
		RefreshTokenTTL: 24 * time.Hour,
	})
	if err != nil {
		t.Fatalf("NewJWTService failed: %v", err)
	}

	if svc.AccessTokenTTL() != 15*time.Minute {
		t.Errorf("expected access TTL 15m, got %v", svc.AccessTokenTTL())
	}

	if svc.RefreshTokenTTL() != 24*time.Hour {
		t.Errorf("expected refresh TTL 24h, got %v", svc.RefreshTokenTTL())
	}
}

func TestNewJWTService_GeneratesRandomSecret(t *testing.T) {
	// When secret is empty, it should generate a random one
	svc, err := NewJWTService(JWTConfig{
		Secret: "", // Empty secret
	})
	if err != nil {
		t.Fatalf("NewJWTService failed: %v", err)
	}

	// Secret should be generated (non-empty)
	secret := svc.GetSecret()
	if secret == "" {
		t.Error("expected generated secret to be non-empty")
	}
}

func TestNewJWTService_DefaultTTLs(t *testing.T) {
	svc, err := NewJWTService(JWTConfig{
		Secret: "test-secret-key-at-least-32-chars",
		// No TTLs specified - should use defaults
	})
	if err != nil {
		t.Fatalf("NewJWTService failed: %v", err)
	}

	if svc.AccessTokenTTL() != 15*time.Minute {
		t.Errorf("expected default access TTL 15m, got %v", svc.AccessTokenTTL())
	}

	if svc.RefreshTokenTTL() != 7*24*time.Hour {
		t.Errorf("expected default refresh TTL 7 days, got %v", svc.RefreshTokenTTL())
	}
}

func TestGenerateTokenPair(t *testing.T) {
	svc, err := NewJWTService(JWTConfig{
		Secret:          "test-secret-key-at-least-32-chars",
		Issuer:          "test-issuer",
		AccessTokenTTL:  15 * time.Minute,
		RefreshTokenTTL: 24 * time.Hour,
	})
	if err != nil {
		t.Fatalf("NewJWTService failed: %v", err)
	}

	pair, err := svc.GenerateTokenPair("user123", "test@example.com", "USER", "session456")
	if err != nil {
		t.Fatalf("GenerateTokenPair failed: %v", err)
	}

	if pair.AccessToken == "" {
		t.Error("access token should not be empty")
	}

	if pair.RefreshToken == "" {
		t.Error("refresh token should not be empty")
	}

	if pair.AccessToken == pair.RefreshToken {
		t.Error("access and refresh tokens should be different")
	}

	// ExpiresAt should be approximately 15 minutes from now
	expectedExpiry := time.Now().Add(15 * time.Minute)
	if pair.ExpiresAt.Before(expectedExpiry.Add(-1*time.Second)) || pair.ExpiresAt.After(expectedExpiry.Add(1*time.Second)) {
		t.Errorf("ExpiresAt should be ~15 minutes from now, got %v", pair.ExpiresAt)
	}
}

func TestValidateAccessToken(t *testing.T) {
	svc, err := NewJWTService(JWTConfig{
		Secret:          "test-secret-key-at-least-32-chars",
		Issuer:          "test-issuer",
		AccessTokenTTL:  15 * time.Minute,
		RefreshTokenTTL: 24 * time.Hour,
	})
	if err != nil {
		t.Fatalf("NewJWTService failed: %v", err)
	}

	pair, err := svc.GenerateTokenPair("user123", "test@example.com", "USER", "session456")
	if err != nil {
		t.Fatalf("GenerateTokenPair failed: %v", err)
	}

	// Validate access token
	claims, err := svc.ValidateAccessToken(pair.AccessToken)
	if err != nil {
		t.Fatalf("ValidateAccessToken failed: %v", err)
	}

	if claims.UserID != "user123" {
		t.Errorf("expected UserID user123, got %s", claims.UserID)
	}

	if claims.Email != "test@example.com" {
		t.Errorf("expected Email test@example.com, got %s", claims.Email)
	}

	if claims.Role != "USER" {
		t.Errorf("expected Role USER, got %s", claims.Role)
	}

	if claims.SessionID != "session456" {
		t.Errorf("expected SessionID session456, got %s", claims.SessionID)
	}

	if claims.TokenType != TokenTypeAccess {
		t.Errorf("expected TokenType access, got %s", claims.TokenType)
	}
}

func TestValidateRefreshToken(t *testing.T) {
	svc, err := NewJWTService(JWTConfig{
		Secret:          "test-secret-key-at-least-32-chars",
		Issuer:          "test-issuer",
		AccessTokenTTL:  15 * time.Minute,
		RefreshTokenTTL: 24 * time.Hour,
	})
	if err != nil {
		t.Fatalf("NewJWTService failed: %v", err)
	}

	pair, err := svc.GenerateTokenPair("user123", "test@example.com", "USER", "session456")
	if err != nil {
		t.Fatalf("GenerateTokenPair failed: %v", err)
	}

	// Validate refresh token
	claims, err := svc.ValidateRefreshToken(pair.RefreshToken)
	if err != nil {
		t.Fatalf("ValidateRefreshToken failed: %v", err)
	}

	if claims.TokenType != TokenTypeRefresh {
		t.Errorf("expected TokenType refresh, got %s", claims.TokenType)
	}
}

func TestValidateAccessToken_WrongTokenType(t *testing.T) {
	svc, err := NewJWTService(JWTConfig{
		Secret:          "test-secret-key-at-least-32-chars",
		Issuer:          "test-issuer",
		AccessTokenTTL:  15 * time.Minute,
		RefreshTokenTTL: 24 * time.Hour,
	})
	if err != nil {
		t.Fatalf("NewJWTService failed: %v", err)
	}

	pair, err := svc.GenerateTokenPair("user123", "test@example.com", "USER", "session456")
	if err != nil {
		t.Fatalf("GenerateTokenPair failed: %v", err)
	}

	// Try to validate refresh token as access token
	_, err = svc.ValidateAccessToken(pair.RefreshToken)
	if err != ErrInvalidTokenType {
		t.Errorf("expected ErrInvalidTokenType, got: %v", err)
	}
}

func TestValidateRefreshToken_WrongTokenType(t *testing.T) {
	svc, err := NewJWTService(JWTConfig{
		Secret:          "test-secret-key-at-least-32-chars",
		Issuer:          "test-issuer",
		AccessTokenTTL:  15 * time.Minute,
		RefreshTokenTTL: 24 * time.Hour,
	})
	if err != nil {
		t.Fatalf("NewJWTService failed: %v", err)
	}

	pair, err := svc.GenerateTokenPair("user123", "test@example.com", "USER", "session456")
	if err != nil {
		t.Fatalf("GenerateTokenPair failed: %v", err)
	}

	// Try to validate access token as refresh token
	_, err = svc.ValidateRefreshToken(pair.AccessToken)
	if err != ErrInvalidTokenType {
		t.Errorf("expected ErrInvalidTokenType, got: %v", err)
	}
}

func TestValidateToken_InvalidToken(t *testing.T) {
	svc, err := NewJWTService(JWTConfig{
		Secret: "test-secret-key-at-least-32-chars",
	})
	if err != nil {
		t.Fatalf("NewJWTService failed: %v", err)
	}

	testCases := []struct {
		name  string
		token string
	}{
		{"empty token", ""},
		{"invalid format", "not.a.jwt"},
		{"invalid signature", "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJzdWIiOiIxMjM0NTY3ODkwIn0.wrong_signature"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := svc.ValidateToken(tc.token)
			if err == nil {
				t.Error("expected error for invalid token")
			}
		})
	}
}

func TestValidateToken_WrongSecret(t *testing.T) {
	svc1, err := NewJWTService(JWTConfig{
		Secret: "first-secret-key-at-least-32-chars",
	})
	if err != nil {
		t.Fatalf("NewJWTService failed: %v", err)
	}

	svc2, err := NewJWTService(JWTConfig{
		Secret: "second-secret-key-at-least-32-chars",
	})
	if err != nil {
		t.Fatalf("NewJWTService failed: %v", err)
	}

	// Generate token with first service
	pair, err := svc1.GenerateTokenPair("user123", "test@example.com", "USER", "session456")
	if err != nil {
		t.Fatalf("GenerateTokenPair failed: %v", err)
	}

	// Try to validate with second service (different secret)
	_, err = svc2.ValidateToken(pair.AccessToken)
	if err != ErrInvalidToken {
		t.Errorf("expected ErrInvalidToken, got: %v", err)
	}
}

func TestGenerateAccessToken(t *testing.T) {
	svc, err := NewJWTService(JWTConfig{
		Secret:         "test-secret-key-at-least-32-chars",
		AccessTokenTTL: 15 * time.Minute,
	})
	if err != nil {
		t.Fatalf("NewJWTService failed: %v", err)
	}

	token, expiresAt, err := svc.GenerateAccessToken("user123", "test@example.com", "USER", "session456")
	if err != nil {
		t.Fatalf("GenerateAccessToken failed: %v", err)
	}

	if token == "" {
		t.Error("token should not be empty")
	}

	expectedExpiry := time.Now().Add(15 * time.Minute)
	if expiresAt.Before(expectedExpiry.Add(-1*time.Second)) || expiresAt.After(expectedExpiry.Add(1*time.Second)) {
		t.Errorf("expiresAt should be ~15 minutes from now, got %v", expiresAt)
	}

	// Validate the token
	claims, err := svc.ValidateAccessToken(token)
	if err != nil {
		t.Fatalf("ValidateAccessToken failed: %v", err)
	}

	if claims.TokenType != TokenTypeAccess {
		t.Errorf("expected TokenType access, got %s", claims.TokenType)
	}
}
