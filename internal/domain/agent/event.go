package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"

	"github.com/gosuda/steerlane/internal/domain"
)

// Event is a canonical persisted agent session event for replay.
type Event struct { //nolint:govet // field layout follows domain readability first.
	CreatedAt time.Time
	Payload   json.RawMessage
	Type      string
	ID        uuid.UUID
	SessionID domain.AgentSessionID
	TenantID  domain.TenantID
}

// EventRepository defines persistence operations for replayable agent session events.
type EventRepository interface {
	Append(ctx context.Context, event *Event) error
	ListBySession(ctx context.Context, tenantID domain.TenantID, sessionID domain.AgentSessionID, limit int, cursor *uuid.UUID) ([]*Event, error)
}

// Validate checks that the event is well formed.
func (e *Event) Validate() error {
	if e == nil {
		return fmt.Errorf("agent session event: %w", domain.ErrInvalidInput)
	}
	if e.ID == uuid.Nil {
		return fmt.Errorf("agent session event id: %w", domain.ErrInvalidInput)
	}
	if e.TenantID == uuid.Nil {
		return fmt.Errorf("agent session event tenant id: %w", domain.ErrInvalidInput)
	}
	if e.SessionID == uuid.Nil {
		return fmt.Errorf("agent session event session id: %w", domain.ErrInvalidInput)
	}
	if e.Type == "" {
		return fmt.Errorf("agent session event type: %w", domain.ErrInvalidInput)
	}
	if len(e.Payload) == 0 {
		return fmt.Errorf("agent session event payload: %w", domain.ErrInvalidInput)
	}
	if e.CreatedAt.IsZero() {
		return fmt.Errorf("agent session event created at: %w", domain.ErrInvalidInput)
	}
	return nil
}
