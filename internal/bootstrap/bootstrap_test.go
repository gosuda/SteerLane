package bootstrap

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/gosuda/steerlane/internal/auth"
	"github.com/gosuda/steerlane/internal/config"
	"github.com/gosuda/steerlane/internal/domain"
	"github.com/gosuda/steerlane/internal/domain/tenant"
	"github.com/gosuda/steerlane/internal/domain/user"
)

// ---------------------------------------------------------------------------
// Mock repositories
// ---------------------------------------------------------------------------

type mockTenantRepo struct {
	createFn    func(ctx context.Context, t *tenant.Tenant) error
	getByIDFn   func(ctx context.Context, id domain.TenantID) (*tenant.Tenant, error)
	getBySlugFn func(ctx context.Context, slug string) (*tenant.Tenant, error)
	updateFn    func(ctx context.Context, t *tenant.Tenant) error
}

func (m *mockTenantRepo) Create(ctx context.Context, t *tenant.Tenant) error {
	if m.createFn != nil {
		return m.createFn(ctx, t)
	}
	return nil
}

func (m *mockTenantRepo) GetByID(ctx context.Context, id domain.TenantID) (*tenant.Tenant, error) {
	if m.getByIDFn != nil {
		return m.getByIDFn(ctx, id)
	}
	return nil, domain.ErrNotFound
}

func (m *mockTenantRepo) GetBySlug(ctx context.Context, slug string) (*tenant.Tenant, error) {
	if m.getBySlugFn != nil {
		return m.getBySlugFn(ctx, slug)
	}
	return nil, domain.ErrNotFound
}

func (m *mockTenantRepo) Update(ctx context.Context, t *tenant.Tenant) error {
	if m.updateFn != nil {
		return m.updateFn(ctx, t)
	}
	return nil
}

type mockUserRepo struct {
	createFn       func(ctx context.Context, u *user.User) error
	getByIDFn      func(ctx context.Context, tenantID domain.TenantID, id domain.UserID) (*user.User, error)
	getByEmailFn   func(ctx context.Context, tenantID domain.TenantID, email string) (*user.User, error)
	listByTenantFn func(ctx context.Context, tenantID domain.TenantID, limit int, cursor string) ([]*user.User, error)
	updateFn       func(ctx context.Context, u *user.User) error
	deleteFn       func(ctx context.Context, tenantID domain.TenantID, id domain.UserID) error
}

func (m *mockUserRepo) Create(ctx context.Context, u *user.User) error {
	if m.createFn != nil {
		return m.createFn(ctx, u)
	}
	return nil
}

func (m *mockUserRepo) GetByID(ctx context.Context, tenantID domain.TenantID, id domain.UserID) (*user.User, error) {
	if m.getByIDFn != nil {
		return m.getByIDFn(ctx, tenantID, id)
	}
	return nil, domain.ErrNotFound
}

func (m *mockUserRepo) GetByEmail(ctx context.Context, tenantID domain.TenantID, email string) (*user.User, error) {
	if m.getByEmailFn != nil {
		return m.getByEmailFn(ctx, tenantID, email)
	}
	return nil, domain.ErrNotFound
}

func (m *mockUserRepo) ListByTenant(ctx context.Context, tenantID domain.TenantID, limit int, cursor string) ([]*user.User, error) {
	if m.listByTenantFn != nil {
		return m.listByTenantFn(ctx, tenantID, limit, cursor)
	}
	return nil, nil
}

func (m *mockUserRepo) Update(ctx context.Context, u *user.User) error {
	if m.updateFn != nil {
		return m.updateFn(ctx, u)
	}
	return nil
}

func (m *mockUserRepo) Delete(ctx context.Context, tenantID domain.TenantID, id domain.UserID) error {
	if m.deleteFn != nil {
		return m.deleteFn(ctx, tenantID, id)
	}
	return nil
}

// ---------------------------------------------------------------------------
// Test helpers
// ---------------------------------------------------------------------------

var fixedTime = time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

const fakeHash = "$argon2id$v=19$m=65536,t=3,p=4$dGVzdHNhbHQ$testhash"

func discardLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func newTestRunner(tenants tenant.Repository, users user.Repository, cfg config.Config) runner {
	return runner{
		cfg:     cfg,
		tenants: tenants,
		users:   users,
		logger:  discardLogger(),
		now:     func() time.Time { return fixedTime },
		hashPassword: func(_ string) (string, error) {
			return fakeHash, nil
		},
	}
}

func selfHostedConfig() config.Config {
	return config.Config{
		Mode: config.ModeSelfHosted,
		Bootstrap: config.BootstrapConfig{
			AdminEmail:    "admin@example.com",
			AdminPassword: "secure-password-123",
			AdminName:     "Bootstrap Admin",
		},
	}
}

func selfHostedConfigNoAdmin() config.Config {
	return config.Config{
		Mode: config.ModeSelfHosted,
	}
}

func saasConfig() config.Config {
	return config.Config{
		Mode: config.ModeSaaS,
	}
}

func fixedTenant() *tenant.Tenant {
	return &tenant.Tenant{
		ID:        uuid.MustParse("00000000-0000-0000-0000-000000000001"),
		Name:      DefaultTenantName,
		Slug:      DefaultTenantSlug,
		Settings:  map[string]any{},
		CreatedAt: fixedTime,
		UpdatedAt: fixedTime,
	}
}

func fixedAdminUser(tenantID domain.TenantID) *user.User {
	return &user.User{
		ID:           uuid.MustParse("00000000-0000-0000-0000-000000000002"),
		TenantID:     tenantID,
		Email:        new("admin@example.com"), //nolint:modernize // string literal pointer helper in tests
		PasswordHash: new(fakeHash),            //nolint:modernize // variable pointer helper in tests
		Name:         "Bootstrap Admin",
		Role:         user.RoleAdmin,
		CreatedAt:    fixedTime,
		UpdatedAt:    fixedTime,
	}
}

func fixedMemberUser(tenantID domain.TenantID) *user.User {
	u := fixedAdminUser(tenantID)
	u.Role = user.RoleMember
	return u
}

// ---------------------------------------------------------------------------
// Tests: Run() public entry point via runner.run() with injected deps
// ---------------------------------------------------------------------------

func TestRun_SaaSMode_ReturnsError(t *testing.T) {
	ctx := t.Context()
	r := newTestRunner(&mockTenantRepo{}, &mockUserRepo{}, saasConfig())

	result, err := r.run(ctx)

	require.Error(t, err)
	require.ErrorIs(t, err, domain.ErrConfigInvalid)
	assert.Nil(t, result)
	assert.Contains(t, err.Error(), "selfhosted mode")
}

func TestRun_SelfHosted_NoAdminConfig(t *testing.T) {
	ctx := t.Context()
	tenants := &mockTenantRepo{
		getBySlugFn: func(_ context.Context, _ string) (*tenant.Tenant, error) {
			return nil, domain.ErrNotFound
		},
	}

	r := newTestRunner(tenants, &mockUserRepo{}, selfHostedConfigNoAdmin())

	result, err := r.run(ctx)

	require.NoError(t, err)
	require.NotNil(t, result)
	assert.True(t, result.CreatedTenant, "tenant should be created")
	assert.False(t, result.CreatedAdmin, "no admin config means no admin created")
	assert.Nil(t, result.Admin, "admin should be nil when no bootstrap admin configured")
	assert.NotNil(t, result.Tenant, "tenant should always be populated")
}

func TestRun_SelfHosted_CreateAll(t *testing.T) {
	ctx := t.Context()
	existingTenant := fixedTenant()

	tenants := &mockTenantRepo{
		getBySlugFn: func(_ context.Context, _ string) (*tenant.Tenant, error) {
			return nil, domain.ErrNotFound
		},
		createFn: func(_ context.Context, t *tenant.Tenant) error {
			// Simulate the repo setting the tenant in-place (caller keeps the pointer).
			return nil
		},
	}

	users := &mockUserRepo{
		getByEmailFn: func(_ context.Context, _ domain.TenantID, _ string) (*user.User, error) {
			return nil, domain.ErrNotFound
		},
		createFn: func(_ context.Context, _ *user.User) error {
			return nil
		},
	}

	_ = existingTenant // referenced below for clarity

	r := newTestRunner(tenants, users, selfHostedConfig())

	result, err := r.run(ctx)

	require.NoError(t, err)
	require.NotNil(t, result)
	assert.True(t, result.CreatedTenant)
	assert.True(t, result.CreatedAdmin)
	assert.NotNil(t, result.Tenant)
	assert.NotNil(t, result.Admin)

	// Verify admin properties.
	assert.Equal(t, user.RoleAdmin, result.Admin.Role)
	assert.Equal(t, "Bootstrap Admin", result.Admin.Name)
	require.NotNil(t, result.Admin.Email)
	assert.Equal(t, "admin@example.com", *result.Admin.Email)
	require.NotNil(t, result.Admin.PasswordHash)
	assert.Equal(t, fakeHash, *result.Admin.PasswordHash)
	assert.Equal(t, fixedTime, result.Admin.CreatedAt)
	assert.Equal(t, fixedTime, result.Admin.UpdatedAt)
}

func TestRun_SelfHosted_ExistingTenant_ExistingAdmin(t *testing.T) {
	ctx := t.Context()
	existing := fixedTenant()
	admin := fixedAdminUser(existing.ID)

	tenants := &mockTenantRepo{
		getBySlugFn: func(_ context.Context, _ string) (*tenant.Tenant, error) {
			return existing, nil
		},
	}

	users := &mockUserRepo{
		getByEmailFn: func(_ context.Context, _ domain.TenantID, _ string) (*user.User, error) {
			return admin, nil
		},
	}

	r := newTestRunner(tenants, users, selfHostedConfig())

	result, err := r.run(ctx)

	require.NoError(t, err)
	require.NotNil(t, result)
	assert.False(t, result.CreatedTenant)
	assert.False(t, result.CreatedAdmin)
	assert.Equal(t, existing, result.Tenant)
	assert.Equal(t, admin, result.Admin)
}

func TestRun_SelfHosted_ExistingTenant_NewAdmin(t *testing.T) {
	ctx := t.Context()
	existing := fixedTenant()

	tenants := &mockTenantRepo{
		getBySlugFn: func(_ context.Context, _ string) (*tenant.Tenant, error) {
			return existing, nil
		},
	}

	users := &mockUserRepo{
		getByEmailFn: func(_ context.Context, _ domain.TenantID, _ string) (*user.User, error) {
			return nil, domain.ErrNotFound
		},
		createFn: func(_ context.Context, _ *user.User) error {
			return nil
		},
	}

	r := newTestRunner(tenants, users, selfHostedConfig())

	result, err := r.run(ctx)

	require.NoError(t, err)
	require.NotNil(t, result)
	assert.False(t, result.CreatedTenant, "tenant already existed")
	assert.True(t, result.CreatedAdmin, "admin was newly created")
	assert.Equal(t, existing, result.Tenant)
	assert.NotNil(t, result.Admin)
	assert.Equal(t, user.RoleAdmin, result.Admin.Role)
}

func TestRun_ExistingUser_NotAdmin(t *testing.T) {
	ctx := t.Context()
	existing := fixedTenant()
	memberUser := fixedMemberUser(existing.ID)

	tenants := &mockTenantRepo{
		getBySlugFn: func(_ context.Context, _ string) (*tenant.Tenant, error) {
			return existing, nil
		},
	}

	users := &mockUserRepo{
		getByEmailFn: func(_ context.Context, _ domain.TenantID, _ string) (*user.User, error) {
			return memberUser, nil
		},
	}

	r := newTestRunner(tenants, users, selfHostedConfig())

	result, err := r.run(ctx)

	require.Error(t, err)
	require.ErrorIs(t, err, domain.ErrConfigInvalid)
	assert.Contains(t, err.Error(), "non-admin")
	// Result should still have the tenant populated.
	require.NotNil(t, result)
	assert.Equal(t, existing, result.Tenant)
}

func TestRun_TenantConflict_Retry(t *testing.T) {
	ctx := t.Context()
	existingAfterConflict := fixedTenant()

	getBySlugCalls := 0
	tenants := &mockTenantRepo{
		getBySlugFn: func(_ context.Context, _ string) (*tenant.Tenant, error) {
			getBySlugCalls++
			if getBySlugCalls == 1 {
				return nil, domain.ErrNotFound
			}
			// Second call (after conflict) returns the tenant.
			return existingAfterConflict, nil
		},
		createFn: func(_ context.Context, _ *tenant.Tenant) error {
			return domain.ErrConflict
		},
	}

	r := newTestRunner(tenants, &mockUserRepo{}, selfHostedConfigNoAdmin())

	result, err := r.run(ctx)

	require.NoError(t, err)
	require.NotNil(t, result)
	assert.False(t, result.CreatedTenant, "tenant was found via retry after conflict")
	assert.Equal(t, existingAfterConflict, result.Tenant)
	assert.Equal(t, 2, getBySlugCalls, "GetBySlug called once initially, once after conflict")
}

func TestRun_UserConflict_Retry_AdminRole(t *testing.T) {
	ctx := t.Context()
	existing := fixedTenant()
	admin := fixedAdminUser(existing.ID)

	tenants := &mockTenantRepo{
		getBySlugFn: func(_ context.Context, _ string) (*tenant.Tenant, error) {
			return existing, nil
		},
	}

	getByEmailCalls := 0
	users := &mockUserRepo{
		getByEmailFn: func(_ context.Context, _ domain.TenantID, _ string) (*user.User, error) {
			getByEmailCalls++
			if getByEmailCalls == 1 {
				return nil, domain.ErrNotFound
			}
			return admin, nil
		},
		createFn: func(_ context.Context, _ *user.User) error {
			return domain.ErrConflict
		},
	}

	r := newTestRunner(tenants, users, selfHostedConfig())

	result, err := r.run(ctx)

	require.NoError(t, err)
	require.NotNil(t, result)
	assert.False(t, result.CreatedAdmin, "admin existed via conflict retry")
	assert.Equal(t, admin, result.Admin)
	assert.Equal(t, 2, getByEmailCalls, "GetByEmail called once initially, once after conflict")
}

func TestRun_UserConflict_Retry_NonAdminRole(t *testing.T) {
	ctx := t.Context()
	existing := fixedTenant()
	memberUser := fixedMemberUser(existing.ID)

	tenants := &mockTenantRepo{
		getBySlugFn: func(_ context.Context, _ string) (*tenant.Tenant, error) {
			return existing, nil
		},
	}

	getByEmailCalls := 0
	users := &mockUserRepo{
		getByEmailFn: func(_ context.Context, _ domain.TenantID, _ string) (*user.User, error) {
			getByEmailCalls++
			if getByEmailCalls == 1 {
				return nil, domain.ErrNotFound
			}
			return memberUser, nil
		},
		createFn: func(_ context.Context, _ *user.User) error {
			return domain.ErrConflict
		},
	}

	r := newTestRunner(tenants, users, selfHostedConfig())

	result, err := r.run(ctx)

	require.Error(t, err)
	require.ErrorIs(t, err, domain.ErrConfigInvalid)
	assert.Contains(t, err.Error(), "non-admin")
	// Result should still have tenant populated from the successful first phase.
	require.NotNil(t, result)
	assert.Equal(t, existing, result.Tenant)
}

// ---------------------------------------------------------------------------
// Tests: Run() public entry point
// ---------------------------------------------------------------------------

func TestRun_PublicAPI_SelfHosted(t *testing.T) {
	ctx := t.Context()

	tenants := &mockTenantRepo{
		getBySlugFn: func(_ context.Context, _ string) (*tenant.Tenant, error) {
			return nil, domain.ErrNotFound
		},
	}
	users := &mockUserRepo{
		getByEmailFn: func(_ context.Context, _ domain.TenantID, _ string) (*user.User, error) {
			return nil, domain.ErrNotFound
		},
	}

	result, err := Run(ctx, selfHostedConfig(), tenants, users, discardLogger())

	require.NoError(t, err)
	require.NotNil(t, result)
	assert.True(t, result.CreatedTenant)
	assert.True(t, result.CreatedAdmin)
	assert.NotNil(t, result.Tenant)
	assert.NotNil(t, result.Admin)
}

func TestRun_PublicAPI_SaaS(t *testing.T) {
	ctx := t.Context()

	_, err := Run(ctx, saasConfig(), &mockTenantRepo{}, &mockUserRepo{}, discardLogger())

	require.Error(t, err)
	require.ErrorIs(t, err, domain.ErrConfigInvalid)
}

func TestRun_PublicAPI_NilTenantRepo_Panics(t *testing.T) {
	assert.Panics(t, func() {
		_, _ = Run(t.Context(), selfHostedConfig(), nil, &mockUserRepo{}, discardLogger())
	})
}

func TestRun_PublicAPI_NilUserRepo_Panics(t *testing.T) {
	assert.Panics(t, func() {
		_, _ = Run(t.Context(), selfHostedConfig(), &mockTenantRepo{}, nil, discardLogger())
	})
}

func TestRun_PublicAPI_NilLogger_UsesDefault(t *testing.T) {
	ctx := t.Context()

	tenants := &mockTenantRepo{
		getBySlugFn: func(_ context.Context, _ string) (*tenant.Tenant, error) {
			return nil, domain.ErrNotFound
		},
	}

	// nil logger should not panic -- Run falls back to slog.Default().
	result, err := Run(ctx, selfHostedConfigNoAdmin(), tenants, &mockUserRepo{}, nil)

	require.NoError(t, err)
	require.NotNil(t, result)
	assert.True(t, result.CreatedTenant)
}

// ---------------------------------------------------------------------------
// Tests: parseAdminConfig
// ---------------------------------------------------------------------------

func TestParseAdminConfig_Valid(t *testing.T) {
	cfg := config.BootstrapConfig{
		AdminEmail:    "admin@example.com",
		AdminPassword: "secure-password-123",
		AdminName:     "Bootstrap Admin",
	}

	ac, err := parseAdminConfig(cfg)

	require.NoError(t, err)
	require.NotNil(t, ac)
	assert.Equal(t, "admin@example.com", ac.email)
	assert.Equal(t, "secure-password-123", ac.password)
	assert.Equal(t, "Bootstrap Admin", ac.name)
}

func TestParseAdminConfig_NormalizesEmail(t *testing.T) {
	cfg := config.BootstrapConfig{
		AdminEmail:    "  ADMIN@Example.COM  ",
		AdminPassword: "secure-password-123",
		AdminName:     "Test Admin",
	}

	ac, err := parseAdminConfig(cfg)

	require.NoError(t, err)
	require.NotNil(t, ac)
	assert.Equal(t, "admin@example.com", ac.email, "email should be lowercased and trimmed")
}

func TestParseAdminConfig_TrimsWhitespace(t *testing.T) {
	cfg := config.BootstrapConfig{
		AdminEmail:    "  admin@example.com  ",
		AdminPassword: "  secure-password-123  ",
		AdminName:     "  Bootstrap Admin  ",
	}

	ac, err := parseAdminConfig(cfg)

	require.NoError(t, err)
	require.NotNil(t, ac)
	assert.Equal(t, "admin@example.com", ac.email)
	assert.Equal(t, "secure-password-123", ac.password)
	assert.Equal(t, "Bootstrap Admin", ac.name)
}

func TestParseAdminConfig_AllEmpty(t *testing.T) {
	cfg := config.BootstrapConfig{}

	ac, err := parseAdminConfig(cfg)

	require.NoError(t, err)
	assert.Nil(t, ac, "all-empty config means no admin configured")
}

func TestParseAdminConfig_PartialFields(t *testing.T) {
	tests := []struct {
		name string
		cfg  config.BootstrapConfig
	}{
		{
			name: "email only",
			cfg:  config.BootstrapConfig{AdminEmail: "admin@example.com"},
		},
		{
			name: "password only",
			cfg:  config.BootstrapConfig{AdminPassword: "secure-password-123"},
		},
		{
			name: "name only",
			cfg:  config.BootstrapConfig{AdminName: "Admin"},
		},
		{
			name: "email and password, no name",
			cfg: config.BootstrapConfig{
				AdminEmail:    "admin@example.com",
				AdminPassword: "secure-password-123",
			},
		},
		{
			name: "email and name, no password",
			cfg: config.BootstrapConfig{
				AdminEmail: "admin@example.com",
				AdminName:  "Admin",
			},
		},
		{
			name: "password and name, no email",
			cfg: config.BootstrapConfig{
				AdminPassword: "secure-password-123",
				AdminName:     "Admin",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ac, err := parseAdminConfig(tt.cfg)

			require.Error(t, err)
			require.ErrorIs(t, err, domain.ErrConfigInvalid)
			assert.Nil(t, ac)
			assert.Contains(t, err.Error(), "requires email, password, and name")
		})
	}
}

func TestParseAdminConfig_InvalidEmail(t *testing.T) {
	cfg := config.BootstrapConfig{
		AdminEmail:    "not-an-email",
		AdminPassword: "secure-password-123",
		AdminName:     "Admin",
	}

	ac, err := parseAdminConfig(cfg)

	require.Error(t, err)
	require.ErrorIs(t, err, domain.ErrConfigInvalid)
	assert.Nil(t, ac)
	assert.Contains(t, err.Error(), "not-an-email")
}

func TestParseAdminConfig_WeakPassword(t *testing.T) {
	cfg := config.BootstrapConfig{
		AdminEmail:    "admin@example.com",
		AdminPassword: "short",
		AdminName:     "Admin",
	}

	ac, err := parseAdminConfig(cfg)

	require.Error(t, err)
	require.ErrorIs(t, err, auth.ErrWeakPassword)
	assert.Nil(t, ac)
}

func TestParseAdminConfig_ExactMinPasswordLength(t *testing.T) {
	cfg := config.BootstrapConfig{
		AdminEmail:    "admin@example.com",
		AdminPassword: "12345678", // exactly minBootstrapPassLen
		AdminName:     "Admin",
	}

	ac, err := parseAdminConfig(cfg)

	require.NoError(t, err)
	require.NotNil(t, ac)
	assert.Equal(t, "12345678", ac.password)
}

// ---------------------------------------------------------------------------
// Tests: validateBootstrapAdminRole
// ---------------------------------------------------------------------------

func TestValidateBootstrapAdminRole_Admin(t *testing.T) {
	u := &user.User{
		ID:   domain.NewID(),
		Role: user.RoleAdmin,
	}

	err := validateBootstrapAdminRole(u, "admin@example.com")

	require.NoError(t, err)
}

func TestValidateBootstrapAdminRole_Member(t *testing.T) {
	u := &user.User{
		ID:   domain.NewID(),
		Role: user.RoleMember,
	}

	err := validateBootstrapAdminRole(u, "admin@example.com")

	require.Error(t, err)
	require.ErrorIs(t, err, domain.ErrConfigInvalid)
	assert.Contains(t, err.Error(), "non-admin")
	assert.Contains(t, err.Error(), "admin@example.com")
}

// ---------------------------------------------------------------------------
// Tests: runner error propagation
// ---------------------------------------------------------------------------

func TestRun_TenantGetBySlug_UnexpectedError(t *testing.T) {
	ctx := t.Context()
	dbErr := errors.New("connection refused")

	tenants := &mockTenantRepo{
		getBySlugFn: func(_ context.Context, _ string) (*tenant.Tenant, error) {
			return nil, dbErr
		},
	}

	r := newTestRunner(tenants, &mockUserRepo{}, selfHostedConfigNoAdmin())

	_, err := r.run(ctx)

	require.Error(t, err)
	require.ErrorIs(t, err, dbErr)
	assert.Contains(t, err.Error(), "get default tenant")
}

func TestRun_TenantCreate_UnexpectedError(t *testing.T) {
	ctx := t.Context()
	dbErr := errors.New("disk full")

	tenants := &mockTenantRepo{
		getBySlugFn: func(_ context.Context, _ string) (*tenant.Tenant, error) {
			return nil, domain.ErrNotFound
		},
		createFn: func(_ context.Context, _ *tenant.Tenant) error {
			return dbErr
		},
	}

	r := newTestRunner(tenants, &mockUserRepo{}, selfHostedConfigNoAdmin())

	_, err := r.run(ctx)

	require.Error(t, err)
	require.ErrorIs(t, err, dbErr)
	assert.Contains(t, err.Error(), "create default tenant")
}

func TestRun_UserGetByEmail_UnexpectedError(t *testing.T) {
	ctx := t.Context()
	existing := fixedTenant()
	dbErr := errors.New("timeout")

	tenants := &mockTenantRepo{
		getBySlugFn: func(_ context.Context, _ string) (*tenant.Tenant, error) {
			return existing, nil
		},
	}

	users := &mockUserRepo{
		getByEmailFn: func(_ context.Context, _ domain.TenantID, _ string) (*user.User, error) {
			return nil, dbErr
		},
	}

	r := newTestRunner(tenants, users, selfHostedConfig())

	_, err := r.run(ctx)

	require.Error(t, err)
	require.ErrorIs(t, err, dbErr)
	assert.Contains(t, err.Error(), "get bootstrap admin by email")
}

func TestRun_UserCreate_UnexpectedError(t *testing.T) {
	ctx := t.Context()
	existing := fixedTenant()
	dbErr := errors.New("unique violation on id")

	tenants := &mockTenantRepo{
		getBySlugFn: func(_ context.Context, _ string) (*tenant.Tenant, error) {
			return existing, nil
		},
	}

	users := &mockUserRepo{
		getByEmailFn: func(_ context.Context, _ domain.TenantID, _ string) (*user.User, error) {
			return nil, domain.ErrNotFound
		},
		createFn: func(_ context.Context, _ *user.User) error {
			return dbErr
		},
	}

	r := newTestRunner(tenants, users, selfHostedConfig())

	_, err := r.run(ctx)

	require.Error(t, err)
	require.ErrorIs(t, err, dbErr)
	assert.Contains(t, err.Error(), "create bootstrap admin")
}

func TestRun_HashPassword_Error(t *testing.T) {
	ctx := t.Context()
	existing := fixedTenant()
	hashErr := errors.New("argon2 memory allocation failed")

	tenants := &mockTenantRepo{
		getBySlugFn: func(_ context.Context, _ string) (*tenant.Tenant, error) {
			return existing, nil
		},
	}

	users := &mockUserRepo{
		getByEmailFn: func(_ context.Context, _ domain.TenantID, _ string) (*user.User, error) {
			return nil, domain.ErrNotFound
		},
	}

	r := newTestRunner(tenants, users, selfHostedConfig())
	r.hashPassword = func(_ string) (string, error) {
		return "", hashErr
	}

	_, err := r.run(ctx)

	require.Error(t, err)
	require.ErrorIs(t, err, hashErr)
	assert.Contains(t, err.Error(), "hash bootstrap admin password")
}

func TestRun_TenantConflict_ReloadFails(t *testing.T) {
	ctx := t.Context()
	reloadErr := errors.New("connection reset")

	getBySlugCalls := 0
	tenants := &mockTenantRepo{
		getBySlugFn: func(_ context.Context, _ string) (*tenant.Tenant, error) {
			getBySlugCalls++
			if getBySlugCalls == 1 {
				return nil, domain.ErrNotFound
			}
			return nil, reloadErr
		},
		createFn: func(_ context.Context, _ *tenant.Tenant) error {
			return domain.ErrConflict
		},
	}

	r := newTestRunner(tenants, &mockUserRepo{}, selfHostedConfigNoAdmin())

	_, err := r.run(ctx)

	require.Error(t, err)
	require.ErrorIs(t, err, reloadErr)
	assert.Contains(t, err.Error(), "reload default tenant after conflict")
}

func TestRun_UserConflict_ReloadFails(t *testing.T) {
	ctx := t.Context()
	existing := fixedTenant()
	reloadErr := errors.New("connection reset")

	tenants := &mockTenantRepo{
		getBySlugFn: func(_ context.Context, _ string) (*tenant.Tenant, error) {
			return existing, nil
		},
	}

	getByEmailCalls := 0
	users := &mockUserRepo{
		getByEmailFn: func(_ context.Context, _ domain.TenantID, _ string) (*user.User, error) {
			getByEmailCalls++
			if getByEmailCalls == 1 {
				return nil, domain.ErrNotFound
			}
			return nil, reloadErr
		},
		createFn: func(_ context.Context, _ *user.User) error {
			return domain.ErrConflict
		},
	}

	r := newTestRunner(tenants, users, selfHostedConfig())

	_, err := r.run(ctx)

	require.Error(t, err)
	require.ErrorIs(t, err, reloadErr)
	assert.Contains(t, err.Error(), "reload bootstrap admin after conflict")
}
