package adr

import (
	"encoding/json"
	"fmt"
	"slices"
	"time"

	"github.com/gosuda/steerlane/internal/domain"
)

// ADRStatus represents the lifecycle state of an ADR.
type ADRStatus string

const (
	StatusDraft      ADRStatus = "draft"
	StatusProposed   ADRStatus = "proposed"
	StatusAccepted   ADRStatus = "accepted"
	StatusRejected   ADRStatus = "rejected"
	StatusDeprecated ADRStatus = "deprecated"
)

// validTransitions defines the allowed state transitions for an ADR.
//
//nolint:gochecknoglobals // effectively a constant lookup table
var validTransitions = map[ADRStatus][]ADRStatus{
	StatusDraft:    {StatusProposed},
	StatusProposed: {StatusAccepted, StatusRejected},
	StatusAccepted: {StatusDeprecated},
}

// Consequences captures the structured outcomes of an ADR decision.
type Consequences struct {
	Good    []string `json:"good"`
	Bad     []string `json:"bad"`
	Neutral []string `json:"neutral"`
}

// ADR represents an Architectural Decision Record.
type ADR struct {
	UpdatedAt      time.Time
	CreatedAt      time.Time
	AgentSessionID *domain.AgentSessionID
	CreatedBy      *domain.UserID
	Title          string
	Status         ADRStatus
	Context        string
	Decision       string
	Consequences   Consequences
	Options        json.RawMessage
	Drivers        []string
	Sequence       int
	ID             domain.ADRID
	ProjectID      domain.ProjectID
	TenantID       domain.TenantID
}

// Validate checks that the ADR's fields are well-formed.
func (a *ADR) Validate() error {
	if a.Title == "" {
		return fmt.Errorf("adr title: %w", domain.ErrInvalidInput)
	}
	if a.Sequence < 1 {
		return fmt.Errorf("adr sequence must be >= 1: %w", domain.ErrInvalidInput)
	}
	switch a.Status {
	case StatusDraft, StatusProposed, StatusAccepted, StatusRejected, StatusDeprecated:
	default:
		return fmt.Errorf("adr status %q: %w", a.Status, domain.ErrInvalidInput)
	}
	return nil
}

// CanTransitionTo checks whether the ADR can move to the given status.
func (a *ADR) CanTransitionTo(next ADRStatus) error {
	allowed, ok := validTransitions[a.Status]
	if !ok {
		return fmt.Errorf("from %q: %w", a.Status, domain.ErrInvalidTransition)
	}
	if slices.Contains(allowed, next) {
		return nil
	}
	return fmt.Errorf("from %q to %q: %w", a.Status, next, domain.ErrInvalidTransition)
}

// Transition validates and applies a status change.
func (a *ADR) Transition(next ADRStatus, now time.Time) error {
	if err := a.CanTransitionTo(next); err != nil {
		return err
	}
	a.Status = next
	a.UpdatedAt = now
	return nil
}
