package postgres

import (
	"context"
	"fmt"

	"github.com/google/uuid"

	"github.com/gosuda/steerlane/internal/domain"
	"github.com/gosuda/steerlane/internal/domain/agent"
	"github.com/gosuda/steerlane/internal/store/postgres/sqlc"
)

var _ agent.EventRepository = (*agentEventRepository)(nil)

type agentEventRepository struct {
	store *Store
}

func mapAgentEventModel(row sqlc.AgentSessionEvent) (*agent.Event, error) {
	record := &agent.Event{
		ID:        row.ID,
		TenantID:  row.TenantID,
		SessionID: row.AgentSessionID,
		Type:      row.EventType,
		Payload:   rawJSONOrDefault(row.Payload, "{}"),
		CreatedAt: row.CreatedAt,
	}
	if err := record.Validate(); err != nil {
		return nil, fmt.Errorf("map agent session event: %w", err)
	}
	return record, nil
}

func mapAgentEvents(rows []sqlc.AgentSessionEvent) ([]*agent.Event, error) {
	items := make([]*agent.Event, 0, len(rows))
	for _, row := range rows {
		record, err := mapAgentEventModel(row)
		if err != nil {
			return nil, err
		}
		items = append(items, record)
	}
	return items, nil
}

func (r *agentEventRepository) Append(ctx context.Context, event *agent.Event) error {
	if event == nil {
		return fmt.Errorf("agent session event: %w", domain.ErrInvalidInput)
	}
	if err := event.Validate(); err != nil {
		return err
	}

	row, err := r.store.queries.CreateAgentSessionEvent(ctx, sqlc.CreateAgentSessionEventParams{
		TenantID:       event.TenantID,
		AgentSessionID: event.SessionID,
		EventType:      event.Type,
		Payload:        event.Payload,
		CreatedAt:      event.CreatedAt,
	})
	if err != nil {
		return fmt.Errorf("create agent session event: %w", classifyError(err))
	}

	mapped, err := mapAgentEventModel(row)
	if err != nil {
		return err
	}
	*event = *mapped
	return nil
}

func (r *agentEventRepository) ListBySession(ctx context.Context, tenantID domain.TenantID, sessionID domain.AgentSessionID, limit int, cursor *uuid.UUID) ([]*agent.Event, error) {
	if err := requireUUID("tenant id", tenantID); err != nil {
		return nil, err
	}
	if err := requireUUID("agent session id", sessionID); err != nil {
		return nil, err
	}

	normalizedLimit := normalizeListLimit(limit)
	if cursor != nil {
		if _, err := r.store.queries.GetAgentSessionEventByID(ctx, sqlc.GetAgentSessionEventByIDParams{
			ID:             *cursor,
			TenantID:       tenantID,
			AgentSessionID: sessionID,
		}); err != nil {
			return nil, fmt.Errorf("load agent session event cursor: %w", classifyError(err))
		}

		rows, err := r.store.queries.ListAgentSessionEventsBySessionAfterCursor(ctx, sqlc.ListAgentSessionEventsBySessionAfterCursorParams{
			TenantID:       tenantID,
			AgentSessionID: sessionID,
			ID:             *cursor,
			Limit:          normalizedLimit,
		})
		if err != nil {
			return nil, fmt.Errorf("list agent session events by session after cursor: %w", classifyError(err))
		}
		return mapAgentEvents(rows)
	}

	rows, err := r.store.queries.ListAgentSessionEventsBySession(ctx, sqlc.ListAgentSessionEventsBySessionParams{
		TenantID:       tenantID,
		AgentSessionID: sessionID,
		Limit:          normalizedLimit,
	})
	if err != nil {
		return nil, fmt.Errorf("list agent session events by session: %w", classifyError(err))
	}
	return mapAgentEvents(rows)
}
