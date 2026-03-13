package auth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"fmt"

	"github.com/google/uuid"

	"github.com/gosuda/steerlane/internal/domain"
)

// APIKeyLen is the raw byte length of a generated API key.
const APIKeyLen = 32

// apiKeyPrefix is prepended to the base64-encoded key for identification.
const apiKeyPrefix = "sl_"

// APIKeyRecord is the stored representation of an API key.
// The plaintext is never persisted; only prefix + hash.
type APIKeyRecord struct {
	Prefix   string
	Hash     string
	Label    string
	ID       uuid.UUID
	TenantID domain.TenantID
	UserID   domain.UserID
}

// APIKeyRepository defines persistence operations for API keys.
// Implemented by the store layer.
type APIKeyRepository interface {
	Create(ctx context.Context, rec *APIKeyRecord) error
	GetByPrefix(ctx context.Context, prefix string) (*APIKeyRecord, error)
	ListByUser(ctx context.Context, tenantID domain.TenantID, userID domain.UserID) ([]*APIKeyRecord, error)
	Delete(ctx context.Context, tenantID domain.TenantID, id uuid.UUID) error
}

// GenerateAPIKey creates a new API key with a random 32-byte value.
// Returns:
//   - plain: the full plaintext key (shown once to the user), prefixed with "sl_"
//   - prefix: first 8 hex chars of the raw key (for lookup/display)
//   - hash: SHA-256 hex digest of the plaintext (for storage)
func GenerateAPIKey() (plain, prefix, hash string, err error) {
	raw := make([]byte, APIKeyLen)
	if _, err := rand.Read(raw); err != nil { //nolint:govet // short-lived err shadow is idiomatic Go
		return "", "", "", fmt.Errorf("generating api key: %w", err)
	}

	hexKey := hex.EncodeToString(raw)
	plain = apiKeyPrefix + hexKey
	prefix = hexKey[:8]
	hash = HashAPIKey(plain)

	return plain, prefix, hash, nil
}

// HashAPIKey returns the SHA-256 hex digest of a plaintext API key.
func HashAPIKey(plain string) string {
	h := sha256.Sum256([]byte(plain))
	return hex.EncodeToString(h[:])
}

// ValidateAPIKey checks if a plaintext key matches a stored hash
// using constant-time comparison.
func ValidateAPIKey(plain, storedHash string) bool {
	candidate := HashAPIKey(plain)
	return subtle.ConstantTimeCompare([]byte(candidate), []byte(storedHash)) == 1
}
