package auth

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"

	"github.com/gosuda/steerlane/internal/domain"
	"github.com/gosuda/steerlane/internal/domain/user"
)

const minPasswordLen = 8

// Service orchestrates registration, login, and token management.
type Service struct {
	users   user.Repository
	apiKeys APIKeyRepository
	jwt     *JWTService
	logger  *slog.Logger
}

// NewService creates an auth service. All dependencies are required.
func NewService(
	users user.Repository,
	apiKeys APIKeyRepository,
	jwtSvc *JWTService,
	logger *slog.Logger,
) *Service {
	if users == nil {
		panic("auth: user repository must not be nil")
	}
	if apiKeys == nil {
		panic("auth: api key repository must not be nil")
	}
	if jwtSvc == nil {
		panic("auth: jwt service must not be nil")
	}
	if logger == nil {
		logger = slog.Default()
	}
	return &Service{
		users:   users,
		apiKeys: apiKeys,
		jwt:     jwtSvc,
		logger:  logger.With("component", "auth"),
	}
}

// Register creates a new user with a hashed password.
// Returns domain.ErrConflict if the email is already registered.
func (s *Service) Register(ctx context.Context, tenantID domain.TenantID, email, password, name string) (*user.User, error) {
	email = strings.TrimSpace(strings.ToLower(email))

	if len(password) < minPasswordLen {
		return nil, ErrWeakPassword
	}

	// Check for existing user with this email.
	existing, err := s.users.GetByEmail(ctx, tenantID, email)
	if err != nil && !errors.Is(err, domain.ErrNotFound) {
		return nil, fmt.Errorf("checking existing user: %w", err)
	}
	if existing != nil {
		return nil, fmt.Errorf("email %q: %w", email, domain.ErrConflict)
	}

	hashed, err := HashPassword(password)
	if err != nil {
		return nil, fmt.Errorf("hashing password: %w", err)
	}

	now := time.Now()
	u := &user.User{
		ID:           domain.NewID(),
		TenantID:     tenantID,
		Email:        &email,
		PasswordHash: &hashed,
		Name:         name,
		Role:         user.RoleMember,
		CreatedAt:    now,
		UpdatedAt:    now,
	}

	if err := u.Validate(); err != nil { //nolint:govet // short-lived err shadow is idiomatic Go
		return nil, err
	}

	if err := s.users.Create(ctx, u); err != nil { //nolint:govet // short-lived err shadow is idiomatic Go
		return nil, fmt.Errorf("creating user: %w", err)
	}

	s.logger.InfoContext(ctx, "user registered",
		"user_id", u.ID,
		"tenant_id", tenantID,
		"email", email,
	)

	return u, nil
}

// Login validates credentials and returns JWT tokens.
// Returns ErrInvalidCredentials if the email is not found or the password is wrong.
func (s *Service) Login(ctx context.Context, tenantID domain.TenantID, email, password string) (accessToken, refreshToken string, err error) {
	email = strings.TrimSpace(strings.ToLower(email))

	u, err := s.users.GetByEmail(ctx, tenantID, email)
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			return "", "", ErrInvalidCredentials
		}
		return "", "", fmt.Errorf("looking up user: %w", err)
	}

	if u.PasswordHash == nil {
		// User registered via OAuth -- no password set.
		return "", "", ErrInvalidCredentials
	}

	if err := VerifyPassword(*u.PasswordHash, password); err != nil { //nolint:govet // short-lived err shadow is idiomatic Go
		return "", "", ErrInvalidCredentials
	}

	access, refresh, err := s.jwt.IssueTokens(u.ID, tenantID, string(u.Role))
	if err != nil {
		return "", "", fmt.Errorf("issuing tokens: %w", err)
	}

	s.logger.InfoContext(ctx, "user logged in",
		"user_id", u.ID,
		"tenant_id", tenantID,
	)

	return access, refresh, nil
}

// RefreshToken validates a refresh token and issues a new access/refresh pair.
// The old refresh token is implicitly invalidated by issuing a new one with a new JTI.
// Note: for Phase 1A this is stateless. Token blacklisting can be added in Phase 3.
func (s *Service) RefreshToken(ctx context.Context, refreshTokenStr string) (accessToken, refreshToken string, err error) {
	claims, err := s.jwt.ParseToken(refreshTokenStr)
	if err != nil {
		return "", "", ErrInvalidToken
	}

	userID, err := claims.SubjectUUID()
	if err != nil {
		return "", "", ErrInvalidToken
	}

	access, refresh, err := s.jwt.IssueTokens(userID, claims.TenantID, claims.Role)
	if err != nil {
		return "", "", fmt.Errorf("issuing refreshed tokens: %w", err)
	}

	s.logger.InfoContext(ctx, "tokens refreshed",
		"user_id", userID,
		"tenant_id", claims.TenantID,
	)

	return access, refresh, nil
}

// AuthenticateJWT validates a JWT and returns the claims.
// This is a stateless operation that does not hit the database.
func (s *Service) AuthenticateJWT(token string) (*Claims, error) {
	claims, err := s.jwt.ParseToken(token)
	if err != nil {
		return nil, err
	}
	return claims, nil
}

// AuthenticateAPIKey validates an API key and returns synthetic claims
// for the associated user, suitable for middleware integration.
func (s *Service) AuthenticateAPIKey(ctx context.Context, rawKey string) (*Claims, error) {
	// Extract the prefix (first 8 hex chars after the "sl_" prefix).
	trimmed := strings.TrimPrefix(rawKey, apiKeyPrefix)
	if len(trimmed) < 8 {
		return nil, ErrInvalidCredentials
	}
	prefix := trimmed[:8]

	rec, err := s.apiKeys.GetByPrefix(ctx, prefix)
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			return nil, ErrInvalidCredentials
		}
		return nil, fmt.Errorf("looking up api key: %w", err)
	}

	if !ValidateAPIKey(rawKey, rec.Hash) {
		return nil, ErrInvalidCredentials
	}

	// Build synthetic claims for the API key holder.
	claims := &Claims{
		TenantID: rec.TenantID,
		Role:     "apikey",
		RegisteredClaims: jwt.RegisteredClaims{
			Subject: rec.UserID.String(),
			Issuer:  "apikey",
		},
	}

	s.logger.InfoContext(ctx, "api key authenticated",
		"user_id", rec.UserID,
		"tenant_id", rec.TenantID,
		"key_prefix", prefix,
	)

	return claims, nil
}

// CreateAPIKey generates a new API key for a user.
// Returns the plaintext key (shown once) and the stored record.
func (s *Service) CreateAPIKey(ctx context.Context, tenantID domain.TenantID, userID domain.UserID, label string) (plaintext string, rec *APIKeyRecord, err error) {
	plain, prefix, hash, err := GenerateAPIKey()
	if err != nil {
		return "", nil, fmt.Errorf("generating api key: %w", err)
	}

	rec = &APIKeyRecord{
		ID:       domain.NewID(),
		TenantID: tenantID,
		UserID:   userID,
		Prefix:   prefix,
		Hash:     hash,
		Label:    label,
	}

	if err := s.apiKeys.Create(ctx, rec); err != nil { //nolint:govet // short-lived err shadow is idiomatic Go
		return "", nil, fmt.Errorf("storing api key: %w", err)
	}

	s.logger.InfoContext(ctx, "api key created",
		"user_id", userID,
		"tenant_id", tenantID,
		"key_prefix", prefix,
		"label", label,
	)

	return plain, rec, nil
}
