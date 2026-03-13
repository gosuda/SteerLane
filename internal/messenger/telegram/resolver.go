package telegram

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/gosuda/steerlane/internal/domain"
	"github.com/gosuda/steerlane/internal/messenger"
	"github.com/gosuda/steerlane/internal/store/postgres/sqlc"
)

type contextResolverQueries interface {
	GetMessengerConnectionByChannel(ctx context.Context, arg sqlc.GetMessengerConnectionByChannelParams) (sqlc.MessengerConnection, error)
	GetMessengerLink(ctx context.Context, arg sqlc.GetMessengerLinkParams) (sqlc.UserMessengerLink, error)
}

type connectionConfig struct {
	ProjectID domain.ProjectID `json:"project_id"`
}

type ResolvedContext struct {
	TenantID  domain.TenantID
	ProjectID domain.ProjectID
	UserID    domain.UserID
}

type PostgresContextResolver struct{ queries contextResolverQueries }

func NewContextResolver(queries contextResolverQueries) *PostgresContextResolver {
	return &PostgresContextResolver{queries: queries}
}

func (r *PostgresContextResolver) ResolveContext(ctx context.Context, chatID, userID string) (ResolvedContext, error) {
	if strings.TrimSpace(userID) == "" {
		return ResolvedContext{}, fmt.Errorf("telegram resolver: user id: %w", domain.ErrInvalidInput)
	}
	resolved, err := r.ResolveChatContext(ctx, chatID)
	if err != nil {
		return ResolvedContext{}, err
	}
	link, err := r.queries.GetMessengerLink(ctx, sqlc.GetMessengerLinkParams{TenantID: resolved.TenantID, Platform: string(messenger.PlatformTelegram), ExternalID: userID})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ResolvedContext{}, domain.ErrNotFound
		}
		return ResolvedContext{}, fmt.Errorf("telegram resolver: messenger link: %w", err)
	}
	resolved.UserID = link.UserID
	return resolved, nil
}

func (r *PostgresContextResolver) ResolveChatContext(ctx context.Context, chatID string) (ResolvedContext, error) {
	if strings.TrimSpace(chatID) == "" {
		return ResolvedContext{}, fmt.Errorf("telegram resolver: chat id: %w", domain.ErrInvalidInput)
	}
	conn, err := r.queries.GetMessengerConnectionByChannel(ctx, sqlc.GetMessengerConnectionByChannelParams{Platform: string(messenger.PlatformTelegram), ChannelID: &chatID})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ResolvedContext{}, domain.ErrNotFound
		}
		return ResolvedContext{}, fmt.Errorf("telegram resolver: messenger connection: %w", err)
	}
	var cfg connectionConfig
	if len(conn.Config) != 0 {
		if err = json.Unmarshal(conn.Config, &cfg); err != nil {
			return ResolvedContext{}, fmt.Errorf("telegram resolver: decode config: %w", err)
		}
	}
	if cfg.ProjectID == uuid.Nil {
		return ResolvedContext{}, fmt.Errorf("telegram resolver: project id missing in messenger connection config: %w", domain.ErrConfigInvalid)
	}
	return ResolvedContext{TenantID: conn.TenantID, ProjectID: cfg.ProjectID}, nil
}
