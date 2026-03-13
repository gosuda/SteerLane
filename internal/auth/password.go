package auth

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"fmt"
	"strings"

	"golang.org/x/crypto/argon2"
)

// Argon2id parameters.
// These follow OWASP recommendations for interactive logins.
const (
	argonMemory      = 64 * 1024 // 64 MB
	argonIterations  = 3
	argonParallelism = 4
	argonSaltLen     = 16
	argonKeyLen      = 32
)

// HashPassword hashes a password using argon2id with secure defaults.
// The returned string encodes all parameters needed for verification:
//
//	$argon2id$v=19$m=65536,t=3,p=4$<base64-salt>$<base64-hash>
func HashPassword(password string) (string, error) {
	salt := make([]byte, argonSaltLen)
	if _, err := rand.Read(salt); err != nil {
		return "", fmt.Errorf("generating salt: %w", err)
	}

	hash := argon2.IDKey(
		[]byte(password),
		salt,
		argonIterations,
		argonMemory,
		argonParallelism,
		argonKeyLen,
	)

	b64Salt := base64.RawStdEncoding.EncodeToString(salt)
	b64Hash := base64.RawStdEncoding.EncodeToString(hash)

	return fmt.Sprintf(
		"$argon2id$v=%d$m=%d,t=%d,p=%d$%s$%s",
		argon2.Version, argonMemory, argonIterations, argonParallelism,
		b64Salt, b64Hash,
	), nil
}

// VerifyPassword checks if a password matches an argon2id hash.
// Returns nil on success, or an error describing the mismatch or parse failure.
func VerifyPassword(encoded, password string) error {
	salt, storedHash, params, err := decodeArgon2Hash(encoded)
	if err != nil {
		return fmt.Errorf("decoding hash: %w", err)
	}

	candidateHash := argon2.IDKey(
		[]byte(password),
		salt,
		params.iterations,
		params.memory,
		params.parallelism,
		uint32(len(storedHash)), //nolint:gosec // G115: storedHash length is always argonKeyLen (32), fits uint32
	)

	if subtle.ConstantTimeCompare(storedHash, candidateHash) != 1 {
		return ErrInvalidCredentials
	}

	return nil
}

type argon2Params struct {
	memory      uint32
	iterations  uint32
	parallelism uint8
}

// decodeArgon2Hash parses the PHC string format for argon2id.
func decodeArgon2Hash(encoded string) (salt, hash []byte, params argon2Params, err error) {
	parts := strings.Split(encoded, "$")
	if len(parts) != 6 {
		return nil, nil, params, fmt.Errorf("invalid hash format: expected 6 parts, got %d", len(parts))
	}

	if parts[1] != "argon2id" {
		return nil, nil, params, fmt.Errorf("unsupported algorithm: %s", parts[1])
	}

	var version int
	if _, err := fmt.Sscanf(parts[2], "v=%d", &version); err != nil { //nolint:govet // short-lived err shadow is idiomatic Go
		return nil, nil, params, fmt.Errorf("parsing version: %w", err)
	}
	if version != argon2.Version {
		return nil, nil, params, fmt.Errorf("unsupported argon2 version: %d", version)
	}

	if _, err := fmt.Sscanf(parts[3], "m=%d,t=%d,p=%d", //nolint:govet // short-lived err shadow is idiomatic Go
		&params.memory, &params.iterations, &params.parallelism,
	); err != nil {
		return nil, nil, params, fmt.Errorf("parsing parameters: %w", err)
	}

	salt, err = base64.RawStdEncoding.DecodeString(parts[4])
	if err != nil {
		return nil, nil, params, fmt.Errorf("decoding salt: %w", err)
	}

	hash, err = base64.RawStdEncoding.DecodeString(parts[5])
	if err != nil {
		return nil, nil, params, fmt.Errorf("decoding hash: %w", err)
	}

	return salt, hash, params, nil
}
