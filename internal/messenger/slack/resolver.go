package slack

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
	TeamID    string           `json:"team_id,omitempty"`
	ProjectID domain.ProjectID `json:"project_id"`
}

// PostgresContextResolver resolves Slack team/channel/user identifiers using messenger connection and link tables.
type PostgresContextResolver struct {
	queries contextResolverQueries
}

// NewContextResolver creates a Slack context resolver backed by sqlc queries.
func NewContextResolver(queries contextResolverQueries) *PostgresContextResolver {
	return &PostgresContextResolver{queries: queries}
}

// ResolveContext maps Slack team/channel/user identifiers to tenant/project/user IDs.
func (r *PostgresContextResolver) ResolveContext(ctx context.Context, slackTeamID, slackChannelID, slackUserID string) (ResolvedContext, error) {
	if strings.TrimSpace(slackUserID) == "" {
		return ResolvedContext{}, fmt.Errorf("slack resolver: user id: %w", domain.ErrInvalidInput)
	}

	resolved, err := r.ResolveChannelContext(ctx, slackTeamID, slackChannelID)
	if err != nil {
		return ResolvedContext{}, err
	}

	link, err := r.queries.GetMessengerLink(ctx, sqlc.GetMessengerLinkParams{
		TenantID:   resolved.TenantID,
		Platform:   string(messenger.PlatformSlack),
		ExternalID: slackUserID,
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ResolvedContext{}, domain.ErrNotFound
		}
		return ResolvedContext{}, fmt.Errorf("slack resolver: messenger link: %w", err)
	}

	resolved.UserID = link.UserID
	return resolved, nil
}

// ResolveChannelContext maps Slack workspace/channel identifiers to tenant and project IDs.
func (r *PostgresContextResolver) ResolveChannelContext(ctx context.Context, slackTeamID, slackChannelID string) (ResolvedContext, error) {
	if strings.TrimSpace(slackChannelID) == "" {
		return ResolvedContext{}, fmt.Errorf("slack resolver: channel id: %w", domain.ErrInvalidInput)
	}

	channelID := slackChannelID
	conn, err := r.queries.GetMessengerConnectionByChannel(ctx, sqlc.GetMessengerConnectionByChannelParams{
		Platform:  string(messenger.PlatformSlack),
		ChannelID: &channelID,
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ResolvedContext{}, domain.ErrNotFound
		}
		return ResolvedContext{}, fmt.Errorf("slack resolver: messenger connection: %w", err)
	}

	var cfg connectionConfig
	if len(conn.Config) != 0 {
		if err = json.Unmarshal(conn.Config, &cfg); err != nil {
			return ResolvedContext{}, fmt.Errorf("slack resolver: decode config: %w", err)
		}
	}
	if cfg.ProjectID == uuid.Nil {
		return ResolvedContext{}, fmt.Errorf("slack resolver: project id missing in messenger connection config: %w", domain.ErrConfigInvalid)
	}
	if cfg.TeamID != "" && slackTeamID != "" && cfg.TeamID != slackTeamID {
		return ResolvedContext{}, domain.ErrNotFound
	}

	return ResolvedContext{TenantID: conn.TenantID, ProjectID: cfg.ProjectID}, nil
}
