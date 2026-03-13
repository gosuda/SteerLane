package auth

import (
	"context"
	"errors"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/gosuda/steerlane/internal/domain"
	"github.com/gosuda/steerlane/internal/domain/user"
)

// ---------------------------------------------------------------------------
// Test helpers
// ---------------------------------------------------------------------------

func newTestJWT() *JWTService {
	return NewJWTService(
		"test-secret-key-minimum-32-chars!",
		"test-issuer",
		15*time.Minute,
		7*24*time.Hour,
	)
}

func mustHashPassword(t *testing.T, password string) string {
	t.Helper()
	h, err := HashPassword(password)
	require.NoError(t, err)
	return h
}

func discardLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(nopWriter{}, &slog.HandlerOptions{Level: slog.LevelError + 4}))
}

type nopWriter struct{}

func (nopWriter) Write(p []byte) (int, error) { return len(p), nil }

// ---------------------------------------------------------------------------
// Hand-rolled mocks (same-package, minimal)
// ---------------------------------------------------------------------------

// mockUserRepo implements user.Repository with injectable functions.
type mockUserRepo struct {
	CreateFn       func(ctx context.Context, u *user.User) error
	GetByIDFn      func(ctx context.Context, tenantID domain.TenantID, id domain.UserID) (*user.User, error)
	GetByEmailFn   func(ctx context.Context, tenantID domain.TenantID, email string) (*user.User, error)
	ListByTenantFn func(ctx context.Context, tenantID domain.TenantID, limit int, cursor string) ([]*user.User, error)
	UpdateFn       func(ctx context.Context, u *user.User) error
	DeleteFn       func(ctx context.Context, tenantID domain.TenantID, id domain.UserID) error
}

func (m *mockUserRepo) Create(ctx context.Context, u *user.User) error {
	if m.CreateFn != nil {
		return m.CreateFn(ctx, u)
	}
	return nil
}

func (m *mockUserRepo) GetByID(ctx context.Context, tenantID domain.TenantID, id domain.UserID) (*user.User, error) {
	if m.GetByIDFn != nil {
		return m.GetByIDFn(ctx, tenantID, id)
	}
	return nil, domain.ErrNotFound
}

func (m *mockUserRepo) GetByEmail(ctx context.Context, tenantID domain.TenantID, email string) (*user.User, error) {
	if m.GetByEmailFn != nil {
		return m.GetByEmailFn(ctx, tenantID, email)
	}
	return nil, domain.ErrNotFound
}

func (m *mockUserRepo) ListByTenant(ctx context.Context, tenantID domain.TenantID, limit int, cursor string) ([]*user.User, error) {
	if m.ListByTenantFn != nil {
		return m.ListByTenantFn(ctx, tenantID, limit, cursor)
	}
	return nil, nil
}

func (m *mockUserRepo) Update(ctx context.Context, u *user.User) error {
	if m.UpdateFn != nil {
		return m.UpdateFn(ctx, u)
	}
	return nil
}

func (m *mockUserRepo) Delete(ctx context.Context, tenantID domain.TenantID, id domain.UserID) error {
	if m.DeleteFn != nil {
		return m.DeleteFn(ctx, tenantID, id)
	}
	return nil
}

// mockAPIKeyRepo implements APIKeyRepository with injectable functions.
type mockAPIKeyRepo struct {
	CreateFn      func(ctx context.Context, rec *APIKeyRecord) error
	GetByPrefixFn func(ctx context.Context, prefix string) (*APIKeyRecord, error)
	ListByUserFn  func(ctx context.Context, tenantID domain.TenantID, userID domain.UserID) ([]*APIKeyRecord, error)
	DeleteFn      func(ctx context.Context, tenantID domain.TenantID, id uuid.UUID) error
}

func (m *mockAPIKeyRepo) Create(ctx context.Context, rec *APIKeyRecord) error {
	if m.CreateFn != nil {
		return m.CreateFn(ctx, rec)
	}
	return nil
}

func (m *mockAPIKeyRepo) GetByPrefix(ctx context.Context, prefix string) (*APIKeyRecord, error) {
	if m.GetByPrefixFn != nil {
		return m.GetByPrefixFn(ctx, prefix)
	}
	return nil, domain.ErrNotFound
}

func (m *mockAPIKeyRepo) ListByUser(ctx context.Context, tenantID domain.TenantID, userID domain.UserID) ([]*APIKeyRecord, error) {
	if m.ListByUserFn != nil {
		return m.ListByUserFn(ctx, tenantID, userID)
	}
	return nil, nil
}

func (m *mockAPIKeyRepo) Delete(ctx context.Context, tenantID domain.TenantID, id uuid.UUID) error {
	if m.DeleteFn != nil {
		return m.DeleteFn(ctx, tenantID, id)
	}
	return nil
}

// newTestService builds a Service with the given mocks and a discard logger.
func newTestService(users *mockUserRepo, apiKeys *mockAPIKeyRepo) *Service {
	return NewService(users, apiKeys, newTestJWT(), discardLogger())
}

// ---------------------------------------------------------------------------
// Register tests
// ---------------------------------------------------------------------------

func TestRegister_Success(t *testing.T) {
	t.Parallel()
	tenantID := uuid.New()
	var captured *user.User

	users := &mockUserRepo{
		GetByEmailFn: func(_ context.Context, _ domain.TenantID, _ string) (*user.User, error) {
			return nil, domain.ErrNotFound
		},
		CreateFn: func(_ context.Context, u *user.User) error {
			captured = u
			return nil
		},
	}
	svc := newTestService(users, &mockAPIKeyRepo{})

	got, err := svc.Register(t.Context(), tenantID, "Alice@Example.COM", "strongpassword1", "Alice")
	require.NoError(t, err)
	require.NotNil(t, got)

	// Email should be lowercased and trimmed.
	require.NotNil(t, got.Email)
	assert.Equal(t, "alice@example.com", *got.Email)

	// Password hash should be set.
	require.NotNil(t, got.PasswordHash)
	assert.True(t, strings.HasPrefix(*got.PasswordHash, "$argon2id$"), "hash should be argon2id encoded")

	// Verify identity fields.
	assert.Equal(t, tenantID, got.TenantID)
	assert.Equal(t, "Alice", got.Name)
	assert.Equal(t, user.RoleMember, got.Role)
	assert.NotEqual(t, uuid.Nil, got.ID, "user ID should be set")

	// Verify the mock received the same user.
	assert.Equal(t, got, captured)
}

func TestRegister_WeakPassword(t *testing.T) {
	t.Parallel()
	svc := newTestService(&mockUserRepo{}, &mockAPIKeyRepo{})

	tests := []struct {
		name     string
		password string
	}{
		{"empty", ""},
		{"too_short", "short"},
		{"exactly_7_chars", "1234567"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			_, err := svc.Register(t.Context(), uuid.New(), "a@b.com", tt.password, "Test")
			require.ErrorIs(t, err, ErrWeakPassword)
		})
	}
}

func TestRegister_DuplicateEmail(t *testing.T) {
	t.Parallel()
	tenantID := uuid.New()
	existingUser := &user.User{
		ID:       uuid.New(),
		TenantID: tenantID,
		Name:     "Existing",
		Role:     user.RoleMember,
	}

	users := &mockUserRepo{
		GetByEmailFn: func(_ context.Context, _ domain.TenantID, _ string) (*user.User, error) {
			return existingUser, nil
		},
	}
	svc := newTestService(users, &mockAPIKeyRepo{})

	_, err := svc.Register(t.Context(), tenantID, "taken@example.com", "strongpassword1", "New User")
	require.ErrorIs(t, err, domain.ErrConflict)
}

func TestRegister_RepoError(t *testing.T) {
	t.Parallel()
	repoErr := errors.New("database connection lost")

	users := &mockUserRepo{
		GetByEmailFn: func(_ context.Context, _ domain.TenantID, _ string) (*user.User, error) {
			return nil, repoErr
		},
	}
	svc := newTestService(users, &mockAPIKeyRepo{})

	_, err := svc.Register(t.Context(), uuid.New(), "new@example.com", "strongpassword1", "Test")
	require.Error(t, err)
	require.ErrorIs(t, err, repoErr, "should propagate the underlying repository error")
	require.NotErrorIs(t, err, ErrWeakPassword)
	assert.NotErrorIs(t, err, domain.ErrConflict)
}

func TestRegister_ValidationError_EmptyName(t *testing.T) {
	t.Parallel()
	createCalled := false

	users := &mockUserRepo{
		GetByEmailFn: func(_ context.Context, _ domain.TenantID, _ string) (*user.User, error) {
			return nil, domain.ErrNotFound
		},
		CreateFn: func(_ context.Context, _ *user.User) error {
			createCalled = true
			return nil
		},
	}
	svc := newTestService(users, &mockAPIKeyRepo{})

	_, err := svc.Register(t.Context(), uuid.New(), "valid@example.com", "strongpassword1", "")
	require.Error(t, err)
	require.ErrorIs(t, err, domain.ErrInvalidInput)
	assert.False(t, createCalled, "Create should NOT be called when validation fails")
}

// ---------------------------------------------------------------------------
// Login tests
// ---------------------------------------------------------------------------

func TestLogin_Success(t *testing.T) {
	t.Parallel()
	tenantID := uuid.New()
	userID := uuid.New()
	password := "correcthorse"
	hashed := mustHashPassword(t, password)
	email := "user@example.com"

	users := &mockUserRepo{
		GetByEmailFn: func(_ context.Context, tid domain.TenantID, e string) (*user.User, error) {
			assert.Equal(t, tenantID, tid)
			assert.Equal(t, email, e)
			return &user.User{
				ID:           userID,
				TenantID:     tenantID,
				Email:        &email,
				PasswordHash: &hashed,
				Name:         "Test User",
				Role:         user.RoleMember,
			}, nil
		},
	}
	svc := newTestService(users, &mockAPIKeyRepo{})

	access, refresh, err := svc.Login(t.Context(), tenantID, email, password)
	require.NoError(t, err)
	assert.NotEmpty(t, access, "access token should be non-empty")
	assert.NotEmpty(t, refresh, "refresh token should be non-empty")
	assert.NotEqual(t, access, refresh, "access and refresh tokens must differ")

	// Verify the access token is parseable and carries correct claims.
	claims, err := svc.AuthenticateJWT(access)
	require.NoError(t, err)
	sub, err := claims.SubjectUUID()
	require.NoError(t, err)
	assert.Equal(t, userID, sub)
	assert.Equal(t, tenantID, claims.TenantID)
	assert.Equal(t, string(user.RoleMember), claims.Role)
}

func TestLogin_UserNotFound(t *testing.T) {
	t.Parallel()
	users := &mockUserRepo{
		GetByEmailFn: func(_ context.Context, _ domain.TenantID, _ string) (*user.User, error) {
			return nil, domain.ErrNotFound
		},
	}
	svc := newTestService(users, &mockAPIKeyRepo{})

	_, _, err := svc.Login(t.Context(), uuid.New(), "nobody@example.com", "strongpassword1")
	require.ErrorIs(t, err, ErrInvalidCredentials)
}

func TestLogin_WrongPassword(t *testing.T) {
	t.Parallel()
	hashed := mustHashPassword(t, "correctpassword")
	email := "user@example.com"

	users := &mockUserRepo{
		GetByEmailFn: func(_ context.Context, _ domain.TenantID, _ string) (*user.User, error) {
			return &user.User{
				ID:           uuid.New(),
				TenantID:     uuid.New(),
				Email:        &email,
				PasswordHash: &hashed,
				Name:         "Test",
				Role:         user.RoleMember,
			}, nil
		},
	}
	svc := newTestService(users, &mockAPIKeyRepo{})

	_, _, err := svc.Login(t.Context(), uuid.New(), email, "wrongpassword!")
	require.ErrorIs(t, err, ErrInvalidCredentials)
}

func TestLogin_OAuthOnlyUser(t *testing.T) {
	t.Parallel()
	email := "oauth@example.com"

	users := &mockUserRepo{
		GetByEmailFn: func(_ context.Context, _ domain.TenantID, _ string) (*user.User, error) {
			return &user.User{
				ID:           uuid.New(),
				TenantID:     uuid.New(),
				Email:        &email,
				PasswordHash: nil, // OAuth-only, no password set
				Name:         "OAuth User",
				Role:         user.RoleMember,
			}, nil
		},
	}
	svc := newTestService(users, &mockAPIKeyRepo{})

	_, _, err := svc.Login(t.Context(), uuid.New(), email, "anypassword123")
	require.ErrorIs(t, err, ErrInvalidCredentials)
}

func TestLogin_RepoError(t *testing.T) {
	t.Parallel()
	repoErr := errors.New("connection refused")

	users := &mockUserRepo{
		GetByEmailFn: func(_ context.Context, _ domain.TenantID, _ string) (*user.User, error) {
			return nil, repoErr
		},
	}
	svc := newTestService(users, &mockAPIKeyRepo{})

	_, _, err := svc.Login(t.Context(), uuid.New(), "user@example.com", "strongpassword1")
	require.Error(t, err)
	require.ErrorIs(t, err, repoErr, "should propagate the repository error")
	assert.NotErrorIs(t, err, ErrInvalidCredentials, "should NOT mask as invalid credentials")
}

// ---------------------------------------------------------------------------
// RefreshToken tests
// ---------------------------------------------------------------------------

func TestRefreshToken_Success(t *testing.T) {
	t.Parallel()
	tenantID := uuid.New()
	userID := uuid.New()
	jwtSvc := newTestJWT()

	_, refresh, err := jwtSvc.IssueTokens(userID, tenantID, string(user.RoleMember))
	require.NoError(t, err)

	svc := NewService(&mockUserRepo{}, &mockAPIKeyRepo{}, jwtSvc, discardLogger())

	newAccess, newRefresh, err := svc.RefreshToken(t.Context(), refresh)
	require.NoError(t, err)
	assert.NotEmpty(t, newAccess)
	assert.NotEmpty(t, newRefresh)

	// Verify the new access token carries the same identity.
	claims, err := jwtSvc.ParseToken(newAccess)
	require.NoError(t, err)
	sub, err := claims.SubjectUUID()
	require.NoError(t, err)
	assert.Equal(t, userID, sub)
	assert.Equal(t, tenantID, claims.TenantID)
}

func TestRefreshToken_InvalidToken(t *testing.T) {
	t.Parallel()
	svc := newTestService(&mockUserRepo{}, &mockAPIKeyRepo{})

	_, _, err := svc.RefreshToken(t.Context(), "garbage-token")
	require.ErrorIs(t, err, ErrInvalidToken)
}

func TestRefreshToken_ExpiredToken(t *testing.T) {
	t.Parallel()

	// Create a JWTService with already-expired refresh tokens.
	jwtSvc := NewJWTService(
		"test-secret-key-minimum-32-chars!",
		"test-issuer",
		15*time.Minute,
		1*time.Nanosecond, // effectively immediate expiry
	)

	_, refresh, err := jwtSvc.IssueTokens(uuid.New(), uuid.New(), "member")
	require.NoError(t, err)

	// Even a nanosecond should be enough for the token to expire by the
	// time we call RefreshToken.
	time.Sleep(2 * time.Millisecond)

	svc := NewService(&mockUserRepo{}, &mockAPIKeyRepo{}, jwtSvc, discardLogger())

	_, _, err = svc.RefreshToken(t.Context(), refresh)
	require.ErrorIs(t, err, ErrInvalidToken)
}

// ---------------------------------------------------------------------------
// AuthenticateJWT tests
// ---------------------------------------------------------------------------

func TestAuthenticateJWT_Success(t *testing.T) {
	t.Parallel()
	tenantID := uuid.New()
	userID := uuid.New()
	role := string(user.RoleAdmin)
	jwtSvc := newTestJWT()

	access, _, err := jwtSvc.IssueTokens(userID, tenantID, role)
	require.NoError(t, err)

	svc := NewService(&mockUserRepo{}, &mockAPIKeyRepo{}, jwtSvc, discardLogger())

	claims, err := svc.AuthenticateJWT(access)
	require.NoError(t, err)
	require.NotNil(t, claims)

	sub, err := claims.SubjectUUID()
	require.NoError(t, err)
	assert.Equal(t, userID, sub)
	assert.Equal(t, tenantID, claims.TenantID)
	assert.Equal(t, role, claims.Role)
	assert.Equal(t, "test-issuer", claims.Issuer)
}

func TestAuthenticateJWT_Invalid(t *testing.T) {
	t.Parallel()
	svc := newTestService(&mockUserRepo{}, &mockAPIKeyRepo{})

	_, err := svc.AuthenticateJWT("bad-token")
	require.ErrorIs(t, err, ErrInvalidToken)
}

// ---------------------------------------------------------------------------
// AuthenticateAPIKey tests
// ---------------------------------------------------------------------------

func TestAuthenticateAPIKey_Success(t *testing.T) {
	t.Parallel()
	tenantID := uuid.New()
	userID := uuid.New()

	// Generate a real API key to get a consistent plain/prefix/hash triple.
	plain, prefix, hash, err := GenerateAPIKey()
	require.NoError(t, err)

	apiKeys := &mockAPIKeyRepo{
		GetByPrefixFn: func(_ context.Context, p string) (*APIKeyRecord, error) {
			require.Equal(t, prefix, p, "should look up by the correct prefix")
			return &APIKeyRecord{
				ID:       uuid.New(),
				TenantID: tenantID,
				UserID:   userID,
				Prefix:   prefix,
				Hash:     hash,
				Label:    "test-key",
			}, nil
		},
	}
	svc := newTestService(&mockUserRepo{}, apiKeys)

	claims, err := svc.AuthenticateAPIKey(t.Context(), plain)
	require.NoError(t, err)
	require.NotNil(t, claims)

	sub, err := claims.SubjectUUID()
	require.NoError(t, err)
	assert.Equal(t, userID, sub)
	assert.Equal(t, tenantID, claims.TenantID)
	assert.Equal(t, "apikey", claims.Role)
	assert.Equal(t, "apikey", claims.Issuer)
}

func TestAuthenticateAPIKey_BadPrefix(t *testing.T) {
	t.Parallel()
	svc := newTestService(&mockUserRepo{}, &mockAPIKeyRepo{})

	tests := []struct {
		name string
		key  string
	}{
		{"empty", ""},
		{"just_prefix", "sl_"},
		{"too_short_after_prefix", "sl_abc"},
		{"no_prefix_short", "abcdefg"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			_, err := svc.AuthenticateAPIKey(t.Context(), tt.key)
			require.ErrorIs(t, err, ErrInvalidCredentials)
		})
	}
}

func TestAuthenticateAPIKey_NotFound(t *testing.T) {
	t.Parallel()
	apiKeys := &mockAPIKeyRepo{
		GetByPrefixFn: func(_ context.Context, _ string) (*APIKeyRecord, error) {
			return nil, domain.ErrNotFound
		},
	}
	svc := newTestService(&mockUserRepo{}, apiKeys)

	// Need a key long enough to pass the prefix length check.
	fakeKey := "sl_" + strings.Repeat("ab", 16)
	_, err := svc.AuthenticateAPIKey(t.Context(), fakeKey)
	require.ErrorIs(t, err, ErrInvalidCredentials)
}

func TestAuthenticateAPIKey_HashMismatch(t *testing.T) {
	t.Parallel()
	tenantID := uuid.New()
	userID := uuid.New()

	// Generate a real key for valid prefix extraction.
	plain, prefix, _, err := GenerateAPIKey()
	require.NoError(t, err)

	apiKeys := &mockAPIKeyRepo{
		GetByPrefixFn: func(_ context.Context, _ string) (*APIKeyRecord, error) {
			return &APIKeyRecord{
				ID:       uuid.New(),
				TenantID: tenantID,
				UserID:   userID,
				Prefix:   prefix,
				Hash:     "0000000000000000000000000000000000000000000000000000000000000000", // wrong hash
				Label:    "test-key",
			}, nil
		},
	}
	svc := newTestService(&mockUserRepo{}, apiKeys)

	_, err = svc.AuthenticateAPIKey(t.Context(), plain)
	require.ErrorIs(t, err, ErrInvalidCredentials)
}

// ---------------------------------------------------------------------------
// CreateAPIKey tests
// ---------------------------------------------------------------------------

func TestCreateAPIKey_Success(t *testing.T) {
	t.Parallel()
	tenantID := uuid.New()
	userID := uuid.New()
	label := "deploy-key"
	var captured *APIKeyRecord

	apiKeys := &mockAPIKeyRepo{
		CreateFn: func(_ context.Context, rec *APIKeyRecord) error {
			captured = rec
			return nil
		},
	}
	svc := newTestService(&mockUserRepo{}, apiKeys)

	plain, rec, err := svc.CreateAPIKey(t.Context(), tenantID, userID, label)
	require.NoError(t, err)
	require.NotNil(t, rec)

	// Plaintext key starts with the expected prefix.
	assert.True(t, strings.HasPrefix(plain, apiKeyPrefix), "plaintext should start with %q", apiKeyPrefix)

	// Record fields.
	assert.Equal(t, tenantID, rec.TenantID)
	assert.Equal(t, userID, rec.UserID)
	assert.Equal(t, label, rec.Label)
	assert.NotEmpty(t, rec.Prefix)
	assert.NotEmpty(t, rec.Hash)
	assert.NotEqual(t, uuid.Nil, rec.ID, "record ID should be set")

	// The stored hash must validate against the plaintext.
	assert.True(t, ValidateAPIKey(plain, rec.Hash), "hash should validate against plaintext")

	// The mock should have received the same record.
	assert.Equal(t, rec, captured)
}

func TestCreateAPIKey_RepoError(t *testing.T) {
	t.Parallel()
	repoErr := errors.New("disk full")

	apiKeys := &mockAPIKeyRepo{
		CreateFn: func(_ context.Context, _ *APIKeyRecord) error {
			return repoErr
		},
	}
	svc := newTestService(&mockUserRepo{}, apiKeys)

	_, _, err := svc.CreateAPIKey(t.Context(), uuid.New(), uuid.New(), "test")
	require.Error(t, err)
	assert.ErrorIs(t, err, repoErr, "should propagate the repository error")
}

// ---------------------------------------------------------------------------
// Edge case: Register with boundary-length password
// ---------------------------------------------------------------------------

func TestRegister_MinPasswordLength(t *testing.T) {
	t.Parallel()
	users := &mockUserRepo{
		GetByEmailFn: func(_ context.Context, _ domain.TenantID, _ string) (*user.User, error) {
			return nil, domain.ErrNotFound
		},
	}
	svc := newTestService(users, &mockAPIKeyRepo{})

	// Exactly minPasswordLen (8) characters should succeed.
	got, err := svc.Register(t.Context(), uuid.New(), "min@example.com", "12345678", "Min User")
	require.NoError(t, err)
	require.NotNil(t, got)

	// One less than minPasswordLen should fail.
	_, err = svc.Register(t.Context(), uuid.New(), "min@example.com", "1234567", "Min User")
	require.ErrorIs(t, err, ErrWeakPassword)
}
