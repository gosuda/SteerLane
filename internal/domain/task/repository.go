package task

import (
	"context"

	"github.com/google/uuid"

	"github.com/gosuda/steerlane/internal/domain"
)

// Filter constrains task queries.
type Filter struct {
	Status   *TaskStatus
	Priority *int
}

// Repository defines persistence operations for tasks.
type Repository interface {
	Create(ctx context.Context, task *Task) error
	GetByID(ctx context.Context, tenantID domain.TenantID, id domain.TaskID) (*Task, error)
	Update(ctx context.Context, task *Task) error
	Delete(ctx context.Context, tenantID domain.TenantID, id domain.TaskID) error
	ListByProject(ctx context.Context, tenantID domain.TenantID, projectID domain.ProjectID, filter Filter, limit int, cursor *uuid.UUID) ([]*Task, error)
	// Transition atomically validates and applies a status change.
	Transition(ctx context.Context, tenantID domain.TenantID, id domain.TaskID, next TaskStatus) error
}
