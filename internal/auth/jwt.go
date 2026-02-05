package auth

import (
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"log"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// TokenType represents the type of JWT token.
type TokenType string

const (
	// TokenTypeAccess is a short-lived token for API access.
	TokenTypeAccess TokenType = "access"
	// TokenTypeRefresh is a long-lived token for obtaining new access tokens.
	TokenTypeRefresh TokenType = "refresh"
)

// Claims represents the JWT claims for LacyLights tokens.
type Claims struct {
	jwt.RegisteredClaims
	UserID    string    `json:"uid"`
	Email     string    `json:"email"`
	Role      string    `json:"role"`
	SessionID string    `json:"sid"`
	TokenType TokenType `json:"type"`
}

// JWTConfig holds configuration for the JWT service.
type JWTConfig struct {
	Secret           string
	Issuer           string
	AccessTokenTTL   time.Duration
	RefreshTokenTTL  time.Duration
}

// JWTService handles JWT token generation and validation.
type JWTService struct {
	secret          []byte
	issuer          string
	accessTokenTTL  time.Duration
	refreshTokenTTL time.Duration
}

// JWTError represents errors from JWT operations.
var (
	ErrInvalidToken     = errors.New("invalid token")
	ErrExpiredToken     = errors.New("token has expired")
	ErrInvalidTokenType = errors.New("invalid token type")
)

// NewJWTService creates a new JWT service with the given configuration.
// If the secret is empty, a new random secret is generated.
func NewJWTService(cfg JWTConfig) (*JWTService, error) {
	secret := []byte(cfg.Secret)
	if len(secret) == 0 {
		// Generate a random secret if none provided
		secret = make([]byte, 32)
		if _, err := rand.Read(secret); err != nil {
			return nil, fmt.Errorf("failed to generate JWT secret: %w", err)
		}
		log.Printf("Warning: JWT_SECRET not set, using auto-generated secret. Sessions will be invalidated on server restart. Set JWT_SECRET for production use.")
	}

	accessTTL := cfg.AccessTokenTTL
	if accessTTL == 0 {
		accessTTL = 15 * time.Minute
	}

	refreshTTL := cfg.RefreshTokenTTL
	if refreshTTL == 0 {
		refreshTTL = 7 * 24 * time.Hour // 7 days
	}

	issuer := cfg.Issuer
	if issuer == "" {
		issuer = "lacylights"
	}

	return &JWTService{
		secret:          secret,
		issuer:          issuer,
		accessTokenTTL:  accessTTL,
		refreshTokenTTL: refreshTTL,
	}, nil
}

// TokenPair represents an access/refresh token pair.
type TokenPair struct {
	AccessToken  string    `json:"accessToken"`
	RefreshToken string    `json:"refreshToken"`
	ExpiresAt    time.Time `json:"expiresAt"`
}

// GenerateTokenPair generates a new access/refresh token pair for a user.
func (s *JWTService) GenerateTokenPair(userID, email, role, sessionID string) (*TokenPair, error) {
	now := time.Now()

	// Generate access token
	accessToken, err := s.generateToken(userID, email, role, sessionID, TokenTypeAccess, now, s.accessTokenTTL)
	if err != nil {
		return nil, fmt.Errorf("failed to generate access token: %w", err)
	}

	// Generate refresh token
	refreshToken, err := s.generateToken(userID, email, role, sessionID, TokenTypeRefresh, now, s.refreshTokenTTL)
	if err != nil {
		return nil, fmt.Errorf("failed to generate refresh token: %w", err)
	}

	return &TokenPair{
		AccessToken:  accessToken,
		RefreshToken: refreshToken,
		ExpiresAt:    now.Add(s.accessTokenTTL),
	}, nil
}

// GenerateAccessToken generates a new access token for a user.
func (s *JWTService) GenerateAccessToken(userID, email, role, sessionID string) (string, time.Time, error) {
	now := time.Now()
	expiresAt := now.Add(s.accessTokenTTL)

	token, err := s.generateToken(userID, email, role, sessionID, TokenTypeAccess, now, s.accessTokenTTL)
	if err != nil {
		return "", time.Time{}, err
	}

	return token, expiresAt, nil
}

// generateToken creates a JWT token with the given parameters.
func (s *JWTService) generateToken(userID, email, role, sessionID string, tokenType TokenType, now time.Time, ttl time.Duration) (string, error) {
	claims := Claims{
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    s.issuer,
			Subject:   userID,
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(ttl)),
			NotBefore: jwt.NewNumericDate(now),
		},
		UserID:    userID,
		Email:     email,
		Role:      role,
		SessionID: sessionID,
		TokenType: tokenType,
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString(s.secret)
}

// ValidateToken validates a JWT token and returns the claims.
func (s *JWTService) ValidateToken(tokenString string) (*Claims, error) {
	token, err := jwt.ParseWithClaims(tokenString, &Claims{}, func(token *jwt.Token) (interface{}, error) {
		// Validate the signing method
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
		}
		return s.secret, nil
	})

	if err != nil {
		if errors.Is(err, jwt.ErrTokenExpired) {
			return nil, ErrExpiredToken
		}
		return nil, ErrInvalidToken
	}

	claims, ok := token.Claims.(*Claims)
	if !ok || !token.Valid {
		return nil, ErrInvalidToken
	}

	return claims, nil
}

// ValidateAccessToken validates an access token specifically.
func (s *JWTService) ValidateAccessToken(tokenString string) (*Claims, error) {
	claims, err := s.ValidateToken(tokenString)
	if err != nil {
		return nil, err
	}

	if claims.TokenType != TokenTypeAccess {
		return nil, ErrInvalidTokenType
	}

	return claims, nil
}

// ValidateRefreshToken validates a refresh token specifically.
func (s *JWTService) ValidateRefreshToken(tokenString string) (*Claims, error) {
	claims, err := s.ValidateToken(tokenString)
	if err != nil {
		return nil, err
	}

	if claims.TokenType != TokenTypeRefresh {
		return nil, ErrInvalidTokenType
	}

	return claims, nil
}

// GetSecret returns the JWT secret (for testing or backup purposes).
func (s *JWTService) GetSecret() string {
	return base64.StdEncoding.EncodeToString(s.secret)
}

// AccessTokenTTL returns the access token TTL.
func (s *JWTService) AccessTokenTTL() time.Duration {
	return s.accessTokenTTL
}

// RefreshTokenTTL returns the refresh token TTL.
func (s *JWTService) RefreshTokenTTL() time.Duration {
	return s.refreshTokenTTL
}
