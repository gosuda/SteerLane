package audit

import (
	"context"
	"fmt"
	"maps"
	"strings"

	"github.com/google/uuid"

	"github.com/gosuda/steerlane/internal/domain"
	domainaudit "github.com/gosuda/steerlane/internal/domain/audit"
)

// Action identifies a canonical audit action.
type Action string

const (
	ActionAuthLogin       Action = "auth.login"
	ActionAuthLogout      Action = "auth.logout"
	ActionAuthRefresh     Action = "auth.refresh"
	ActionCRUDCreate      Action = "crud.create"
	ActionCRUDUpdate      Action = "crud.update"
	ActionCRUDDelete      Action = "crud.delete"
	ActionStateTransition Action = "state.transition"
)

// Actor identifies the caller that triggered an audit event.
type Actor struct {
	Type domainaudit.ActorType
	ID   uuid.UUID
}

// Resource identifies the domain object affected by an audit event.
type Resource struct {
	Type string
	ID   uuid.UUID
}

// LogInput captures the minimal information needed to append an audit record.
type LogInput struct {
	Details  map[string]any
	Action   Action
	Actor    Actor
	Resource Resource
	TenantID domain.TenantID
}

// StateTransitionInput represents a resource state change event.
type StateTransitionInput struct {
	Details  map[string]any
	From     string
	To       string
	Actor    Actor
	Resource Resource
	TenantID domain.TenantID
}

// Service appends immutable audit entries.
type Service struct {
	repo domainaudit.Repository
}

// NewService constructs an append-only audit service.
func NewService(repo domainaudit.Repository) *Service {
	if repo == nil {
		panic("audit: repository must not be nil")
	}

	return &Service{repo: repo}
}

// Log validates and persists an audit entry.
func (s *Service) Log(ctx context.Context, input LogInput) (*domainaudit.Entry, error) {
	entry, err := NewEntry(input)
	if err != nil {
		return nil, err
	}

	if err := s.repo.Append(ctx, entry); err != nil { //nolint:govet // short-lived err shadow is idiomatic Go
		return nil, fmt.Errorf("append audit entry: %w", err)
	}

	return cloneEntry(entry), nil
}

// LogAuthEvent appends a canonical authentication audit event.
func (s *Service) LogAuthEvent(ctx context.Context, tenantID domain.TenantID, actor Actor, action Action, resource Resource, details map[string]any) (*domainaudit.Entry, error) {
	switch action {
	case ActionAuthLogin, ActionAuthLogout, ActionAuthRefresh:
	default:
		return nil, fmt.Errorf("audit auth action %q: %w", action, domain.ErrInvalidInput)
	}

	return s.Log(ctx, LogInput{
		TenantID: tenantID,
		Actor:    actor,
		Action:   action,
		Resource: resource,
		Details:  details,
	})
}

// LogCRUD appends a canonical CRUD audit event.
func (s *Service) LogCRUD(ctx context.Context, tenantID domain.TenantID, actor Actor, action Action, resource Resource, details map[string]any) (*domainaudit.Entry, error) {
	switch action {
	case ActionCRUDCreate, ActionCRUDUpdate, ActionCRUDDelete:
	default:
		return nil, fmt.Errorf("audit CRUD action %q: %w", action, domain.ErrInvalidInput)
	}

	return s.Log(ctx, LogInput{
		TenantID: tenantID,
		Actor:    actor,
		Action:   action,
		Resource: resource,
		Details:  details,
	})
}

// LogStateTransition appends a canonical resource transition event.
func (s *Service) LogStateTransition(ctx context.Context, input StateTransitionInput) (*domainaudit.Entry, error) {
	details := cloneDetails(input.Details)
	details["from"] = strings.TrimSpace(input.From)
	details["to"] = strings.TrimSpace(input.To)

	if details["from"] == "" || details["to"] == "" {
		return nil, fmt.Errorf("audit state transition fields: %w", domain.ErrInvalidInput)
	}

	return s.Log(ctx, LogInput{
		TenantID: input.TenantID,
		Actor:    input.Actor,
		Action:   ActionStateTransition,
		Resource: input.Resource,
		Details:  details,
	})
}

// NewEntry constructs a validated audit entry without persisting it.
func NewEntry(input LogInput) (*domainaudit.Entry, error) {
	if err := validateLogInput(input); err != nil {
		return nil, err
	}

	entry := &domainaudit.Entry{
		TenantID:   input.TenantID,
		ActorType:  input.Actor.Type,
		ActorID:    input.Actor.ID,
		Action:     string(input.Action),
		Resource:   input.Resource.Type,
		ResourceID: input.Resource.ID,
		Details:    cloneDetails(input.Details),
	}
	if err := entry.Validate(); err != nil {
		return nil, fmt.Errorf("validate audit entry: %w", err)
	}

	return entry, nil
}

func validateLogInput(input LogInput) error {
	if input.TenantID == uuid.Nil {
		return fmt.Errorf("audit tenant id: %w", domain.ErrInvalidInput)
	}
	if input.Actor.ID == uuid.Nil {
		return fmt.Errorf("audit actor id: %w", domain.ErrInvalidInput)
	}
	if strings.TrimSpace(string(input.Action)) == "" {
		return fmt.Errorf("audit action: %w", domain.ErrInvalidInput)
	}
	if strings.TrimSpace(input.Resource.Type) == "" {
		return fmt.Errorf("audit resource: %w", domain.ErrInvalidInput)
	}
	if input.Resource.ID == uuid.Nil {
		return fmt.Errorf("audit resource id: %w", domain.ErrInvalidInput)
	}

	return nil
}

func cloneEntry(entry *domainaudit.Entry) *domainaudit.Entry {
	if entry == nil {
		return nil
	}

	clone := *entry
	clone.Details = cloneDetails(entry.Details)
	return &clone
}

func cloneDetails(details map[string]any) map[string]any {
	if len(details) == 0 {
		return map[string]any{}
	}

	clone := make(map[string]any, len(details))
	maps.Copy(clone, details)
	return clone
}
