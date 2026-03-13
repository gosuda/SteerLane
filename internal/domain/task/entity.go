package task

import (
	"fmt"
	"slices"
	"time"

	"github.com/gosuda/steerlane/internal/domain"
)

// TaskStatus represents the kanban column a task belongs to.
type TaskStatus string

const (
	StatusBacklog    TaskStatus = "backlog"
	StatusInProgress TaskStatus = "in_progress"
	StatusReview     TaskStatus = "review"
	StatusDone       TaskStatus = "done"
)

// validTransitions defines the allowed kanban state transitions.
//
// From SPEC.md:
//   - backlog     -> in_progress
//   - in_progress -> review
//   - in_progress -> backlog       (re-queue)
//   - review      -> done
//   - review      -> in_progress   (rework)
//   - review      -> backlog       (re-queue)
//
// Invalid:
//   - anything -> backlog from done
//   - in_progress -> done (must go through review)
//
//nolint:gochecknoglobals // effectively a constant lookup table
var validTransitions = map[TaskStatus][]TaskStatus{
	StatusBacklog:    {StatusInProgress},
	StatusInProgress: {StatusReview, StatusBacklog},
	StatusReview:     {StatusDone, StatusInProgress, StatusBacklog},
}

// Task represents a unit of work on the kanban board.
type Task struct {
	CreatedAt      time.Time
	UpdatedAt      time.Time
	ADRID          *domain.ADRID
	AssignedTo     *domain.UserID
	AgentSessionID *domain.AgentSessionID
	Title          string
	Description    string
	Status         TaskStatus
	Priority       int
	ID             domain.TaskID
	TenantID       domain.TenantID
	ProjectID      domain.ProjectID
}

// Validate checks that the task's fields are well-formed.
func (t *Task) Validate() error {
	if t.Title == "" {
		return fmt.Errorf("task title: %w", domain.ErrInvalidInput)
	}
	switch t.Status {
	case StatusBacklog, StatusInProgress, StatusReview, StatusDone:
	default:
		return fmt.Errorf("task status %q: %w", t.Status, domain.ErrInvalidInput)
	}
	if t.Priority < 0 {
		return fmt.Errorf("task priority must be >= 0: %w", domain.ErrInvalidInput)
	}
	return nil
}

// CanTransitionTo checks whether the task can move to the given status.
// Returns domain.ErrInvalidTransition if the transition is not allowed.
func (t *Task) CanTransitionTo(next TaskStatus) error {
	allowed, ok := validTransitions[t.Status]
	if !ok {
		return fmt.Errorf("from %q (terminal): %w", t.Status, domain.ErrInvalidTransition)
	}
	if slices.Contains(allowed, next) {
		return nil
	}
	return fmt.Errorf("from %q to %q: %w", t.Status, next, domain.ErrInvalidTransition)
}

// Transition validates and applies a status change, updating the timestamp.
func (t *Task) Transition(next TaskStatus, now time.Time) error {
	if err := t.CanTransitionTo(next); err != nil {
		return err
	}
	t.Status = next
	t.UpdatedAt = now
	return nil
}
