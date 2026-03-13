package agent

import (
	"fmt"
	"slices"
	"time"

	"github.com/google/uuid"

	"github.com/gosuda/steerlane/internal/domain"
)

// AgentType identifies the kind of AI agent.
type AgentType string

const (
	TypeClaude   AgentType = "claude"
	TypeCodex    AgentType = "codex"
	TypeGemini   AgentType = "gemini"
	TypeOpenCode AgentType = "opencode"
	TypeACP      AgentType = "acp"
)

// SessionStatus represents the lifecycle state of an agent session.
type SessionStatus string

const (
	StatusPending     SessionStatus = "pending"
	StatusRunning     SessionStatus = "running"
	StatusWaitingHITL SessionStatus = "waiting_hitl"
	StatusCompleted   SessionStatus = "completed"
	StatusFailed      SessionStatus = "failed"
	StatusCancelled   SessionStatus = "cancelled"
)

// validTransitions defines the allowed session state transitions.
//
//   - pending      -> running
//   - running      -> waiting_hitl | completed | failed | cancelled
//   - waiting_hitl -> running | cancelled (resume after human answer or timeout cancel)
//
// No transitions from terminal states (completed, failed, cancelled).
//
//nolint:gochecknoglobals // effectively a constant lookup table
var validTransitions = map[SessionStatus][]SessionStatus{
	StatusPending:     {StatusRunning},
	StatusRunning:     {StatusWaitingHITL, StatusCompleted, StatusFailed, StatusCancelled},
	StatusWaitingHITL: {StatusRunning, StatusCancelled},
}

// Session represents a single agent execution against a task.
type Session struct {
	CreatedAt   time.Time
	CompletedAt *time.Time
	Metadata    map[string]any
	RetryAt     *time.Time
	Error       *string
	StartedAt   *time.Time
	ContainerID *string
	BranchName  *string
	Status      SessionStatus
	AgentType   AgentType
	RetryCount  int
	ID          domain.AgentSessionID
	ProjectID   domain.ProjectID
	TaskID      domain.TaskID
	TenantID    domain.TenantID
}

// NewBranchName returns the git branch name for a session.
func NewBranchName(sessionID uuid.UUID) string {
	return "steerlane/" + sessionID.String()
}

// Validate checks that the session's fields are well-formed.
func (s *Session) Validate() error {
	switch s.AgentType {
	case TypeClaude, TypeCodex, TypeGemini, TypeOpenCode, TypeACP:
	default:
		return fmt.Errorf("agent type %q: %w", s.AgentType, domain.ErrInvalidInput)
	}
	switch s.Status {
	case StatusPending, StatusRunning, StatusWaitingHITL, StatusCompleted, StatusFailed, StatusCancelled:
	default:
		return fmt.Errorf("session status %q: %w", s.Status, domain.ErrInvalidInput)
	}
	if s.RetryCount < 0 {
		return fmt.Errorf("retry count must be >= 0: %w", domain.ErrInvalidInput)
	}
	return nil
}

// CanTransitionTo checks whether the session can move to the given status.
func (s *Session) CanTransitionTo(next SessionStatus) error {
	allowed, ok := validTransitions[s.Status]
	if !ok {
		return fmt.Errorf("from %q (terminal): %w", s.Status, domain.ErrInvalidTransition)
	}
	if slices.Contains(allowed, next) {
		return nil
	}
	return fmt.Errorf("from %q to %q: %w", s.Status, next, domain.ErrInvalidTransition)
}

// Transition validates and applies a status change, updating timestamps.
func (s *Session) Transition(next SessionStatus, now time.Time) error {
	if err := s.CanTransitionTo(next); err != nil {
		return err
	}
	s.Status = next
	switch next {
	case StatusRunning:
		if s.StartedAt == nil {
			s.StartedAt = &now
		}
	case StatusCompleted, StatusFailed, StatusCancelled:
		s.CompletedAt = &now
	}
	return nil
}
