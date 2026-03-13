package bootstrap

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/mail"
	"strings"
	"time"

	"github.com/gosuda/steerlane/internal/auth"
	"github.com/gosuda/steerlane/internal/config"
	"github.com/gosuda/steerlane/internal/domain"
	"github.com/gosuda/steerlane/internal/domain/tenant"
	"github.com/gosuda/steerlane/internal/domain/user"
)

const (
	DefaultTenantName   = "Default"
	DefaultTenantSlug   = "default"
	minBootstrapPassLen = 8
)

type passwordHasher func(string) (string, error)

type runner struct {
	tenants      tenant.Repository
	users        user.Repository
	logger       *slog.Logger
	now          func() time.Time
	hashPassword passwordHasher
	cfg          config.Config
}

// Result reports what the bootstrap process observed or created.
type Result struct {
	Tenant        *tenant.Tenant
	Admin         *user.User
	CreatedTenant bool
	CreatedAdmin  bool
}

// Run ensures self-hosted bootstrap state exists.
func Run(
	ctx context.Context,
	cfg config.Config,
	tenants tenant.Repository,
	users user.Repository,
	logger *slog.Logger,
) (*Result, error) {
	if tenants == nil {
		panic("bootstrap: tenant repository must not be nil")
	}
	if users == nil {
		panic("bootstrap: user repository must not be nil")
	}
	if logger == nil {
		logger = slog.Default()
	}

	r := runner{
		cfg:          cfg,
		tenants:      tenants,
		users:        users,
		logger:       logger.With("component", "bootstrap"),
		now:          time.Now,
		hashPassword: auth.HashPassword,
	}

	return r.run(ctx)
}

func (r runner) run(ctx context.Context) (*Result, error) {
	if !r.cfg.IsSelfHosted() {
		return nil, fmt.Errorf("bootstrap requires selfhosted mode: %w", domain.ErrConfigInvalid)
	}

	result := &Result{}

	defaultTenant, createdTenant, err := r.ensureDefaultTenant(ctx)
	if err != nil {
		return nil, err
	}
	result.Tenant = defaultTenant
	result.CreatedTenant = createdTenant

	adminCfg, err := parseAdminConfig(r.cfg.Bootstrap)
	if err != nil {
		return result, err
	}
	if adminCfg == nil {
		return result, nil
	}

	adminUser, createdAdmin, err := r.ensureAdminUser(ctx, defaultTenant.ID, *adminCfg)
	if err != nil {
		return result, err
	}
	result.Admin = adminUser
	result.CreatedAdmin = createdAdmin

	return result, nil
}

func (r runner) ensureDefaultTenant(ctx context.Context) (*tenant.Tenant, bool, error) {
	existing, err := r.tenants.GetBySlug(ctx, DefaultTenantSlug)
	if err == nil {
		r.logger.InfoContext(ctx, "default tenant already exists", "tenant_id", existing.ID, "tenant_slug", existing.Slug)
		return existing, false, nil
	}
	if !errors.Is(err, domain.ErrNotFound) {
		return nil, false, fmt.Errorf("get default tenant: %w", err)
	}

	record := &tenant.Tenant{
		ID:       domain.NewID(),
		Name:     DefaultTenantName,
		Slug:     DefaultTenantSlug,
		Settings: map[string]any{},
	}

	if err := r.tenants.Create(ctx, record); err != nil { //nolint:govet // short-lived err shadow is idiomatic Go
		if errors.Is(err, domain.ErrConflict) {
			existing, getErr := r.tenants.GetBySlug(ctx, DefaultTenantSlug) //nolint:govet // short-lived err shadow is idiomatic Go
			if getErr != nil {
				return nil, false, fmt.Errorf("reload default tenant after conflict: %w", getErr)
			}
			return existing, false, nil
		}
		return nil, false, fmt.Errorf("create default tenant: %w", err)
	}

	r.logger.InfoContext(ctx, "default tenant created", "tenant_id", record.ID, "tenant_slug", record.Slug)

	return record, true, nil
}

func (r runner) ensureAdminUser(ctx context.Context, tenantID domain.TenantID, cfg adminConfig) (*user.User, bool, error) {
	existing, err := r.users.GetByEmail(ctx, tenantID, cfg.email)
	if err == nil {
		if err := validateBootstrapAdminRole(existing, cfg.email); err != nil { //nolint:govet // short-lived err shadow is idiomatic Go
			return nil, false, err
		}
		r.logger.InfoContext(ctx, "bootstrap admin already exists", "tenant_id", tenantID, "user_id", existing.ID, "email", cfg.email)
		return existing, false, nil
	}
	if !errors.Is(err, domain.ErrNotFound) {
		return nil, false, fmt.Errorf("get bootstrap admin by email: %w", err)
	}

	hashedPassword, err := r.hashPassword(cfg.password)
	if err != nil {
		return nil, false, fmt.Errorf("hash bootstrap admin password: %w", err)
	}

	now := r.now()
	adminUser := &user.User{
		ID:           domain.NewID(),
		TenantID:     tenantID,
		Email:        &cfg.email,
		PasswordHash: &hashedPassword,
		Name:         cfg.name,
		Role:         user.RoleAdmin,
		CreatedAt:    now,
		UpdatedAt:    now,
	}

	if err := adminUser.Validate(); err != nil { //nolint:govet // short-lived err shadow is idiomatic Go
		return nil, false, fmt.Errorf("validate bootstrap admin: %w", err)
	}

	if err := r.users.Create(ctx, adminUser); err != nil { //nolint:govet // short-lived err shadow is idiomatic Go
		if errors.Is(err, domain.ErrConflict) {
			existing, getErr := r.users.GetByEmail(ctx, tenantID, cfg.email) //nolint:govet // short-lived err shadow is idiomatic Go
			if getErr != nil {
				return nil, false, fmt.Errorf("reload bootstrap admin after conflict: %w", getErr)
			}
			if err := validateBootstrapAdminRole(existing, cfg.email); err != nil { //nolint:govet // short-lived err shadow is idiomatic Go
				return nil, false, err
			}
			return existing, false, nil
		}
		return nil, false, fmt.Errorf("create bootstrap admin: %w", err)
	}

	r.logger.InfoContext(ctx, "bootstrap admin created", "tenant_id", tenantID, "user_id", adminUser.ID, "email", cfg.email)

	return adminUser, true, nil
}

func validateBootstrapAdminRole(existing *user.User, email string) error {
	if existing.Role == user.RoleAdmin {
		return nil
	}

	return fmt.Errorf(
		"bootstrap admin email %q belongs to non-admin user %s with role %q: %w",
		email,
		existing.ID,
		existing.Role,
		domain.ErrConfigInvalid,
	)
}

type adminConfig struct {
	email    string
	password string
	name     string
}

func parseAdminConfig(cfg config.BootstrapConfig) (*adminConfig, error) {
	email := strings.TrimSpace(strings.ToLower(cfg.AdminEmail))
	password := strings.TrimSpace(cfg.AdminPassword)
	name := strings.TrimSpace(cfg.AdminName)

	if email == "" && password == "" && name == "" {
		return nil, nil
	}

	if email == "" || password == "" || name == "" {
		return nil, fmt.Errorf("bootstrap admin config requires email, password, and name: %w", domain.ErrConfigInvalid)
	}

	parsedEmail, err := mail.ParseAddress(email)
	if err != nil {
		return nil, fmt.Errorf("bootstrap admin email %q: %w", email, domain.ErrConfigInvalid)
	}
	email = strings.ToLower(parsedEmail.Address)

	if len(password) < minBootstrapPassLen {
		return nil, fmt.Errorf("bootstrap admin password: %w", auth.ErrWeakPassword)
	}

	return &adminConfig{
		email:    email,
		password: password,
		name:     name,
	}, nil
}
