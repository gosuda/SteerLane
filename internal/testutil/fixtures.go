package testutil

import (
	"time"

	"github.com/google/uuid"

	"github.com/gosuda/steerlane/internal/auth"
	"github.com/gosuda/steerlane/internal/config"
	"github.com/gosuda/steerlane/internal/domain/agent"
	"github.com/gosuda/steerlane/internal/domain/project"
	"github.com/gosuda/steerlane/internal/domain/tenant"
	"github.com/gosuda/steerlane/internal/domain/user"
)

// TestPassword is the plaintext password used for test user fixtures.
const TestPassword = "password123"

// testPasswordHash is the argon2id hash of TestPassword, computed once at init.
//
//nolint:gochecknoglobals // test utility
var testPasswordHash string

//nolint:gochecknoinits // test utility
func init() {
	var err error
	testPasswordHash, err = auth.HashPassword(TestPassword)
	if err != nil {
		panic("testutil: failed to hash test password: " + err.Error())
	}
}

// ---------------------------------------------------------------------------
// Fixed test IDs
// ---------------------------------------------------------------------------

// TestTenantID returns a deterministic UUID for the default test tenant.
func TestTenantID() uuid.UUID {
	return uuid.MustParse("10000000-0000-0000-0000-000000000001")
}

// TestUserID returns a deterministic UUID for the default test admin user.
func TestUserID() uuid.UUID {
	return uuid.MustParse("20000000-0000-0000-0000-000000000001")
}

// TestProjectID returns a deterministic UUID for the default test project.
func TestProjectID() uuid.UUID {
	return uuid.MustParse("30000000-0000-0000-0000-000000000001")
}

// TestSessionID returns a deterministic UUID for the default test agent session.
func TestSessionID() uuid.UUID {
	return uuid.MustParse("40000000-0000-0000-0000-000000000001")
}

// TestTaskID returns a deterministic UUID for the default test task.
func TestTaskID() uuid.UUID {
	return uuid.MustParse("50000000-0000-0000-0000-000000000001")
}

// TestMemberUserID returns a deterministic UUID for the default test member user.
func TestMemberUserID() uuid.UUID {
	return uuid.MustParse("20000000-0000-0000-0000-000000000002")
}

// ---------------------------------------------------------------------------
// Domain entity fixtures
// ---------------------------------------------------------------------------

// TestTenant returns a valid tenant fixture with default test values.
// Each call returns a new value so tests can mutate independently.
func TestTenant() *tenant.Tenant {
	now := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	return &tenant.Tenant{
		ID:        TestTenantID(),
		Name:      "Test Org",
		Slug:      "test-org",
		Settings:  map[string]any{},
		CreatedAt: now,
		UpdatedAt: now,
	}
}

// TestUser returns a valid admin user fixture with default test values.
// PasswordHash is a valid argon2id hash of TestPassword ("password123").
// Each call returns a new value so tests can mutate independently.
func TestUser() *user.User {
	now := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	email := "admin@test.com"
	hash := testPasswordHash
	return &user.User{
		ID:           TestUserID(),
		TenantID:     TestTenantID(),
		Email:        &email,
		Name:         "Test Admin",
		Role:         user.RoleAdmin,
		PasswordHash: &hash,
		CreatedAt:    now,
		UpdatedAt:    now,
	}
}

// TestMemberUser returns a valid member user fixture with default test values.
// Each call returns a new value so tests can mutate independently.
func TestMemberUser() *user.User {
	now := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	email := "member@test.com"
	hash := testPasswordHash
	return &user.User{
		ID:           TestMemberUserID(),
		TenantID:     TestTenantID(),
		Email:        &email,
		Name:         "Test Member",
		Role:         user.RoleMember,
		PasswordHash: &hash,
		CreatedAt:    now,
		UpdatedAt:    now,
	}
}

// TestProject returns a valid project fixture with default test values.
// Each call returns a new value so tests can mutate independently.
func TestProject() *project.Project {
	now := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	return &project.Project{
		ID:        TestProjectID(),
		TenantID:  TestTenantID(),
		Name:      "Test Project",
		RepoURL:   "https://github.com/test-org/test-repo",
		Branch:    "main",
		Settings:  map[string]any{},
		CreatedAt: now,
	}
}

// TestAgentSession returns a valid agent session fixture with default test values.
// Each call returns a new value so tests can mutate independently.
func TestAgentSession() *agent.Session {
	now := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	return &agent.Session{
		ID:         TestSessionID(),
		TenantID:   TestTenantID(),
		ProjectID:  TestProjectID(),
		TaskID:     TestTaskID(),
		AgentType:  agent.TypeClaude,
		Status:     agent.StatusPending,
		RetryCount: 0,
		Metadata:   map[string]any{},
		CreatedAt:  now,
	}
}

// ---------------------------------------------------------------------------
// Config fixtures
// ---------------------------------------------------------------------------

// SelfHostedConfig returns a valid self-hosted config with test values.
// Each call returns a new value so tests can mutate independently.
func SelfHostedConfig() config.Config {
	return config.Config{
		Mode:     config.ModeSelfHosted,
		LogLevel: "debug",
		HTTP: config.HTTPConfig{
			Addr:              ":8080",
			ReadHeaderTimeout: 5 * time.Second,
			ReadTimeout:       30 * time.Second,
			WriteTimeout:      60 * time.Second,
			IdleTimeout:       120 * time.Second,
		},
		Postgres: config.PostgresConfig{ //nolint:gosec // G101: test DSN, not a real credential
			DSN: "postgres://steerlane:steerlane@localhost:5432/steerlane_test?sslmode=disable",
		},
		Redis: config.RedisConfig{
			Addr: "localhost:6379",
		},
		Auth: config.AuthConfig{
			JWTSecret:        "test-secret-key-min-32-chars-long!",
			JWTIssuer:        "test",
			JWTExpiry:        24 * time.Hour,
			JWTRefreshExpiry: 168 * time.Hour,
		},
		CORS: config.CORSConfig{
			Origins: []string{"http://localhost:5173"},
			MaxAge:  3600,
		},
	}
}

// SelfHostedConfigWithBootstrap returns a self-hosted config with bootstrap
// admin credentials pre-filled for first-run tests.
// Each call returns a new value so tests can mutate independently.
func SelfHostedConfigWithBootstrap() config.Config {
	cfg := SelfHostedConfig()
	cfg.Bootstrap = config.BootstrapConfig{
		AdminEmail:    "admin@test.com",
		AdminPassword: "secure-password-123",
		AdminName:     "Test Admin",
	}
	return cfg
}

// SaaSConfig returns a valid SaaS-mode config with test values.
// Each call returns a new value so tests can mutate independently.
func SaaSConfig() config.Config {
	cfg := SelfHostedConfig()
	cfg.Mode = config.ModeSaaS
	return cfg
}
