package postgres

import (
	"context"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/gosuda/steerlane/internal/auth"
	"github.com/gosuda/steerlane/internal/config"
	"github.com/gosuda/steerlane/internal/domain"
	"github.com/gosuda/steerlane/internal/domain/adr"
	"github.com/gosuda/steerlane/internal/domain/agent"
	"github.com/gosuda/steerlane/internal/domain/audit"
	"github.com/gosuda/steerlane/internal/domain/hitl"
	"github.com/gosuda/steerlane/internal/domain/project"
	"github.com/gosuda/steerlane/internal/domain/task"
	"github.com/gosuda/steerlane/internal/domain/tenant"
	"github.com/gosuda/steerlane/internal/domain/user"
	"github.com/gosuda/steerlane/internal/store/postgres/sqlc"
)

type Store struct {
	pool    *pgxpool.Pool
	queries *sqlc.Queries
}

func New(ctx context.Context, cfg config.PostgresConfig) (*Store, error) {
	return NewWithDSN(ctx, cfg.DSN)
}

func NewWithDSN(ctx context.Context, dsn string) (*Store, error) {
	if strings.TrimSpace(dsn) == "" {
		return nil, fmt.Errorf("postgres dsn: %w", domain.ErrInvalidInput)
	}

	poolConfig, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		return nil, fmt.Errorf("parse postgres dsn: %w", err)
	}

	pool, err := pgxpool.NewWithConfig(ctx, poolConfig)
	if err != nil {
		return nil, fmt.Errorf("create postgres pool: %w", err)
	}

	if err := pool.Ping(ctx); err != nil { //nolint:govet // short-lived err shadow is idiomatic Go
		pool.Close()
		return nil, fmt.Errorf("ping postgres: %w", classifyError(err))
	}

	return NewWithPool(pool), nil
}

func NewWithPool(pool *pgxpool.Pool) *Store {
	if pool == nil {
		panic("postgres: pool must not be nil")
	}

	return &Store{
		pool:    pool,
		queries: sqlc.New(pool),
	}
}

func (s *Store) Close() {
	if s == nil || s.pool == nil {
		return
	}
	s.pool.Close()
}

func (s *Store) Ping(ctx context.Context) error {
	if s == nil || s.pool == nil {
		return fmt.Errorf("postgres store: %w", domain.ErrDatabaseUnavailable)
	}
	return s.pool.Ping(ctx)
}

func (s *Store) Pool() *pgxpool.Pool {
	if s == nil {
		return nil
	}
	return s.pool
}

func (s *Store) Queries() *sqlc.Queries {
	if s == nil {
		return nil
	}
	return s.queries
}

func (s *Store) Tenants() tenant.Repository { return &tenantRepository{store: s} }

func (s *Store) Users() user.Repository { return &userRepository{store: s} }

func (s *Store) Projects() project.Repository { return &projectRepository{store: s} }

func (s *Store) Tasks() task.Repository { return &taskRepository{store: s} }

func (s *Store) ADRs() adr.Repository { return &adrRepository{store: s} }

func (s *Store) Agents() agent.Repository { return &agentRepository{store: s} }

func (s *Store) AgentEvents() agent.EventRepository { return &agentEventRepository{store: s} }

func (s *Store) HITL() hitl.Repository { return &hitlRepository{store: s} }

func (s *Store) Audit() audit.Repository { return &auditRepository{store: s} }

func (s *Store) APIKeys() auth.APIKeyRepository { return &apiKeyRepository{store: s} }
