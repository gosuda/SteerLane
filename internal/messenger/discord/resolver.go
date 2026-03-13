package discord

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
	GuildID   string           `json:"guild_id,omitempty"`
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

func (r *PostgresContextResolver) ResolveContext(ctx context.Context, guildID, channelID, userID string) (ResolvedContext, error) {
	if strings.TrimSpace(userID) == "" {
		return ResolvedContext{}, fmt.Errorf("discord resolver: user id: %w", domain.ErrInvalidInput)
	}
	resolved, err := r.ResolveChannelContext(ctx, guildID, channelID)
	if err != nil {
		return ResolvedContext{}, err
	}
	link, err := r.queries.GetMessengerLink(ctx, sqlc.GetMessengerLinkParams{TenantID: resolved.TenantID, Platform: string(messenger.PlatformDiscord), ExternalID: userID})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ResolvedContext{}, domain.ErrNotFound
		}
		return ResolvedContext{}, fmt.Errorf("discord resolver: messenger link: %w", err)
	}
	resolved.UserID = link.UserID
	return resolved, nil
}

func (r *PostgresContextResolver) ResolveChannelContext(ctx context.Context, guildID, channelID string) (ResolvedContext, error) {
	if strings.TrimSpace(channelID) == "" {
		return ResolvedContext{}, fmt.Errorf("discord resolver: channel id: %w", domain.ErrInvalidInput)
	}
	channel := channelID
	conn, err := r.queries.GetMessengerConnectionByChannel(ctx, sqlc.GetMessengerConnectionByChannelParams{Platform: string(messenger.PlatformDiscord), ChannelID: &channel})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ResolvedContext{}, domain.ErrNotFound
		}
		return ResolvedContext{}, fmt.Errorf("discord resolver: messenger connection: %w", err)
	}
	var cfg connectionConfig
	if len(conn.Config) != 0 {
		if err = json.Unmarshal(conn.Config, &cfg); err != nil {
			return ResolvedContext{}, fmt.Errorf("discord resolver: decode config: %w", err)
		}
	}
	if cfg.ProjectID == uuid.Nil {
		return ResolvedContext{}, fmt.Errorf("discord resolver: project id missing in messenger connection config: %w", domain.ErrConfigInvalid)
	}
	if cfg.GuildID != "" && guildID != "" && cfg.GuildID != guildID {
		return ResolvedContext{}, domain.ErrNotFound
	}
	return ResolvedContext{TenantID: conn.TenantID, ProjectID: cfg.ProjectID}, nil
}
