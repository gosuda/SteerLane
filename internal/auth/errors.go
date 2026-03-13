package auth

import "errors"

// Sentinel errors for the auth package.
var (
	// ErrInvalidCredentials is returned when email/password or API key validation fails.
	ErrInvalidCredentials = errors.New("invalid credentials")

	// ErrInvalidToken is returned when a JWT is malformed, expired, or has an invalid signature.
	ErrInvalidToken = errors.New("invalid token")

	// ErrWeakPassword is returned when a password does not meet minimum requirements.
	ErrWeakPassword = errors.New("password must be at least 8 characters")
)
