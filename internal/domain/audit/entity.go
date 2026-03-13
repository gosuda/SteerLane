package audit

import (
	"fmt"
	"time"

	"github.com/google/uuid"

	"github.com/gosuda/steerlane/internal/domain"
)

// ActorType identifies who performed an auditable action.
type ActorType string

const (
	ActorUser   ActorType = "user"
	ActorAgent  ActorType = "agent"
	ActorSystem ActorType = "system"
)

// Entry represents an immutable audit log entry.
type Entry struct {
	CreatedAt  time.Time
	Details    map[string]any
	ActorType  ActorType
	Action     string
	Resource   string
	ID         uuid.UUID
	TenantID   domain.TenantID
	ActorID    uuid.UUID
	ResourceID uuid.UUID
}

// Validate checks that the entry's fields are well-formed.
func (e *Entry) Validate() error {
	switch e.ActorType {
	case ActorUser, ActorAgent, ActorSystem:
	default:
		return fmt.Errorf("actor type %q: %w", e.ActorType, domain.ErrInvalidInput)
	}
	if e.Action == "" {
		return fmt.Errorf("audit action: %w", domain.ErrInvalidInput)
	}
	if e.Resource == "" {
		return fmt.Errorf("audit resource: %w", domain.ErrInvalidInput)
	}
	return nil
}
