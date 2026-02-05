// Package auth provides authentication services for LacyLights.
package auth

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"

	"golang.org/x/crypto/argon2"
)

// Argon2 parameters (OWASP recommended for 2023+)
const (
	argon2Time    = 1         // Number of iterations
	argon2Memory  = 64 * 1024 // 64 MB
	argon2Threads = 4         // Number of threads
	argon2KeyLen  = 32        // Length of the generated key
	argon2SaltLen = 16        // Length of the salt
)

// ErrInvalidHash is returned when the hash format is invalid.
var ErrInvalidHash = errors.New("invalid password hash format")

// ErrHashMismatch is returned when the password doesn't match the hash.
var ErrHashMismatch = errors.New("password does not match hash")

// HashPassword hashes a password using Argon2id.
// Returns the hash in the format: $argon2id$v=19$m=65536,t=1,p=4$<salt>$<hash>
func HashPassword(password string) (string, error) {
	// Generate a random salt
	salt := make([]byte, argon2SaltLen)
	if _, err := rand.Read(salt); err != nil {
		return "", fmt.Errorf("failed to generate salt: %w", err)
	}

	// Hash the password
	hash := argon2.IDKey([]byte(password), salt, argon2Time, argon2Memory, argon2Threads, argon2KeyLen)

	// Encode salt and hash to base64
	b64Salt := base64.RawStdEncoding.EncodeToString(salt)
	b64Hash := base64.RawStdEncoding.EncodeToString(hash)

	// Return the encoded hash in PHC format
	return fmt.Sprintf("$argon2id$v=%d$m=%d,t=%d,p=%d$%s$%s",
		argon2.Version, argon2Memory, argon2Time, argon2Threads, b64Salt, b64Hash), nil
}

// VerifyPassword verifies a password against an Argon2id hash.
// Returns nil if the password matches, ErrHashMismatch if it doesn't.
func VerifyPassword(password, encodedHash string) error {
	// Parse the hash
	params, salt, hash, err := parseArgon2Hash(encodedHash)
	if err != nil {
		return err
	}

	// Hash the password with the same parameters
	otherHash := argon2.IDKey([]byte(password), salt, params.time, params.memory, params.threads, params.keyLen)

	// Constant-time comparison to prevent timing attacks
	if subtle.ConstantTimeCompare(hash, otherHash) != 1 {
		return ErrHashMismatch
	}

	return nil
}

// argon2Params holds the Argon2 parameters extracted from a hash.
type argon2Params struct {
	memory  uint32
	time    uint32
	threads uint8
	keyLen  uint32
}

// parseArgon2Hash parses an Argon2id hash string and returns the parameters, salt, and hash.
func parseArgon2Hash(encodedHash string) (*argon2Params, []byte, []byte, error) {
	// Expected format: $argon2id$v=19$m=65536,t=1,p=4$<salt>$<hash>
	parts := strings.Split(encodedHash, "$")
	if len(parts) != 6 {
		return nil, nil, nil, ErrInvalidHash
	}

	if parts[1] != "argon2id" {
		return nil, nil, nil, ErrInvalidHash
	}

	// Parse version (we accept v=19)
	var version int
	_, err := fmt.Sscanf(parts[2], "v=%d", &version)
	if err != nil || version != argon2.Version {
		return nil, nil, nil, ErrInvalidHash
	}

	// Parse parameters
	var memory, time uint32
	var threads uint8
	_, err = fmt.Sscanf(parts[3], "m=%d,t=%d,p=%d", &memory, &time, &threads)
	if err != nil {
		return nil, nil, nil, ErrInvalidHash
	}

	// Decode salt
	salt, err := base64.RawStdEncoding.DecodeString(parts[4])
	if err != nil {
		return nil, nil, nil, ErrInvalidHash
	}

	// Decode hash
	hash, err := base64.RawStdEncoding.DecodeString(parts[5])
	if err != nil {
		return nil, nil, nil, ErrInvalidHash
	}

	params := &argon2Params{
		memory:  memory,
		time:    time,
		threads: threads,
		keyLen:  uint32(len(hash)),
	}

	return params, salt, hash, nil
}

// GenerateSecureToken generates a cryptographically secure random token.
// Used for verification tokens, password reset tokens, etc.
func GenerateSecureToken(length int) (string, error) {
	bytes := make([]byte, length)
	if _, err := rand.Read(bytes); err != nil {
		return "", fmt.Errorf("failed to generate token: %w", err)
	}
	return base64.URLEncoding.EncodeToString(bytes), nil
}

// HashToken creates a hash of a token for storage.
// Uses SHA-256 to create a deterministic hash that can be looked up.
// This is appropriate for high-entropy tokens (like session IDs) where
// we need consistent hashing for lookups.
func HashToken(token string) string {
	// For tokens, we use SHA-256 since we need deterministic lookups
	// and the tokens are already high-entropy random values.
	// Unlike passwords, tokens don't need salting because:
	// 1. They're cryptographically random (not user-chosen)
	// 2. We need to look them up by their hash
	// 3. Rainbow tables are infeasible for 256-bit random values
	h := sha256.Sum256([]byte(token))
	return base64.RawStdEncoding.EncodeToString(h[:])
}
