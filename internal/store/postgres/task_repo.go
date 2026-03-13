package postgres

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"

	"github.com/gosuda/steerlane/internal/domain"
	"github.com/gosuda/steerlane/internal/domain/task"
	"github.com/gosuda/steerlane/internal/store/postgres/sqlc"
)

var _ task.Repository = (*taskRepository)(nil)

type taskRepository struct {
	store *Store
}

func mapTaskModel(row sqlc.Task) (*task.Task, error) {
	entity := &task.Task{
		ID:             row.ID,
		TenantID:       row.TenantID,
		ProjectID:      row.ProjectID,
		ADRID:          uuidPtrFromPG(row.AdrID),
		Title:          row.Title,
		Description:    row.Description,
		Status:         task.TaskStatus(row.Status),
		Priority:       int(row.Priority),
		AssignedTo:     uuidPtrFromPG(row.AssignedTo),
		AgentSessionID: uuidPtrFromPG(row.AgentSessionID),
		CreatedAt:      row.CreatedAt,
		UpdatedAt:      row.UpdatedAt,
	}
	if err := entity.Validate(); err != nil {
		return nil, fmt.Errorf("map task: %w", err)
	}
	return entity, nil
}

func mapTasks(rows []sqlc.Task) ([]*task.Task, error) {
	items := make([]*task.Task, 0, len(rows))
	for _, row := range rows {
		mapped, err := mapTaskModel(row)
		if err != nil {
			return nil, err
		}
		items = append(items, mapped)
	}
	return items, nil
}

func validateTaskStatus(status task.TaskStatus) error {
	switch status {
	case task.StatusBacklog, task.StatusInProgress, task.StatusReview, task.StatusDone:
		return nil
	default:
		return fmt.Errorf("task status %q: %w", status, domain.ErrInvalidInput)
	}
}

func buildTaskFilter(filter task.Filter) (statusStr *string, priorityVal *int32, err error) {
	if filter.Status != nil {
		if err = validateTaskStatus(*filter.Status); err != nil { //nolint:gocritic // err is named return
			return nil, nil, err
		}
		value := string(*filter.Status)
		statusStr = &value
	}

	if filter.Priority != nil {
		if *filter.Priority < 0 {
			return nil, nil, fmt.Errorf("task priority: %w", domain.ErrInvalidInput)
		}
		value := int32(*filter.Priority) //nolint:gosec // G115: priority validated non-negative above, fits int32
		priorityVal = &value
	}

	return statusStr, priorityVal, nil
}

func taskMatchesFilter(entity *task.Task, filter task.Filter) bool {
	if filter.Status != nil && entity.Status != *filter.Status {
		return false
	}
	if filter.Priority != nil && entity.Priority != *filter.Priority {
		return false
	}
	return true
}

func (r *taskRepository) Create(ctx context.Context, record *task.Task) error {
	if record == nil {
		return fmt.Errorf("task: %w", domain.ErrInvalidInput)
	}
	if err := requireUUID("task tenant id", record.TenantID); err != nil {
		return err
	}
	if err := requireUUID("task project id", record.ProjectID); err != nil {
		return err
	}
	if err := record.Validate(); err != nil {
		return err
	}

	row, err := r.store.queries.CreateTask(ctx, sqlc.CreateTaskParams{
		TenantID:    record.TenantID,
		ProjectID:   record.ProjectID,
		AdrID:       pgUUIDFromPtr(record.ADRID),
		Title:       record.Title,
		Description: record.Description,
		Status:      string(record.Status),
		Priority:    int32(record.Priority), //nolint:gosec // G115: priority validated >= 0 by record.Validate()
		AssignedTo:  pgUUIDFromPtr(record.AssignedTo),
	})
	if err != nil {
		return fmt.Errorf("create task: %w", classifyError(err))
	}

	mapped, err := mapTaskModel(row)
	if err != nil {
		return err
	}
	*record = *mapped

	return nil
}

func (r *taskRepository) GetByID(ctx context.Context, tenantID domain.TenantID, id domain.TaskID) (*task.Task, error) {
	if err := requireUUID("tenant id", tenantID); err != nil {
		return nil, err
	}
	if err := requireUUID("task id", id); err != nil {
		return nil, err
	}

	row, err := r.store.queries.GetTaskByID(ctx, sqlc.GetTaskByIDParams{ID: id, TenantID: tenantID})
	if err != nil {
		return nil, fmt.Errorf("get task by id: %w", classifyError(err))
	}

	entity, err := mapTaskModel(row)
	if err != nil {
		return nil, err
	}
	return entity, nil
}

func (r *taskRepository) Update(ctx context.Context, record *task.Task) error {
	if record == nil {
		return fmt.Errorf("task: %w", domain.ErrInvalidInput)
	}
	if err := requireUUID("task tenant id", record.TenantID); err != nil {
		return err
	}
	if err := requireUUID("task id", record.ID); err != nil {
		return err
	}
	if err := requireUUID("task project id", record.ProjectID); err != nil {
		return err
	}
	if err := record.Validate(); err != nil {
		return err
	}

	row, err := r.store.queries.UpdateTask(ctx, sqlc.UpdateTaskParams{
		ID:          record.ID,
		Title:       record.Title,
		Description: record.Description,
		Priority:    int32(record.Priority), //nolint:gosec // G115: priority validated >= 0 by record.Validate()
		AssignedTo:  pgUUIDFromPtr(record.AssignedTo),
		AdrID:       pgUUIDFromPtr(record.ADRID),
		TenantID:    record.TenantID,
	})
	if err != nil {
		return fmt.Errorf("update task: %w", classifyError(err))
	}

	mapped, err := mapTaskModel(row)
	if err != nil {
		return err
	}
	*record = *mapped

	return nil
}

func (r *taskRepository) Delete(ctx context.Context, tenantID domain.TenantID, id domain.TaskID) error {
	if err := requireUUID("tenant id", tenantID); err != nil {
		return err
	}
	if err := requireUUID("task id", id); err != nil {
		return err
	}

	deleted, err := r.store.queries.DeleteTask(ctx, sqlc.DeleteTaskParams{ID: id, TenantID: tenantID})
	if err != nil {
		return fmt.Errorf("delete task: %w", classifyError(err))
	}
	if deleted == 0 {
		return domain.ErrNotFound
	}
	return nil
}

func (r *taskRepository) ListByProject(ctx context.Context, tenantID domain.TenantID, projectID domain.ProjectID, filter task.Filter, limit int, cursor *uuid.UUID) ([]*task.Task, error) {
	if err := requireUUID("tenant id", tenantID); err != nil {
		return nil, err
	}
	if err := requireUUID("project id", projectID); err != nil {
		return nil, err
	}

	status, priority, err := buildTaskFilter(filter)
	if err != nil {
		return nil, err
	}
	listLimit := normalizeListLimit(limit)

	if cursor == nil {
		rows, err := r.store.queries.ListTasksByProject(ctx, sqlc.ListTasksByProjectParams{ //nolint:govet // short-lived err shadow is idiomatic Go
			ProjectID:      projectID,
			TenantID:       tenantID,
			FilterStatus:   status,
			FilterPriority: priority,
			LimitCount:     listLimit,
		})
		if err != nil {
			return nil, fmt.Errorf("list tasks by project: %w", classifyError(err))
		}
		return mapTasks(rows)
	}

	if err := requireUUID("task cursor", *cursor); err != nil { //nolint:govet // short-lived err shadow is idiomatic Go
		return nil, err
	}

	cursorEntity, err := r.GetByID(ctx, tenantID, *cursor)
	if err != nil {
		return nil, fmt.Errorf("load task cursor: %w", err)
	}
	if cursorEntity.ProjectID != projectID {
		return nil, fmt.Errorf("task cursor project mismatch: %w", domain.ErrInvalidInput)
	}
	if !taskMatchesFilter(cursorEntity, filter) {
		return nil, fmt.Errorf("task cursor filter mismatch: %w", domain.ErrInvalidInput)
	}

	rows, err := r.store.queries.ListTasksByProjectAfter(ctx, sqlc.ListTasksByProjectAfterParams{
		ProjectID:       projectID,
		TenantID:        tenantID,
		FilterStatus:    status,
		FilterPriority:  priority,
		CursorPriority:  int32(cursorEntity.Priority), //nolint:gosec // G115: priority validated >= 0 by domain model
		CursorCreatedAt: cursorEntity.CreatedAt,
		CursorID:        cursorEntity.ID,
		LimitCount:      listLimit,
	})
	if err != nil {
		return nil, fmt.Errorf("list tasks by project after cursor: %w", classifyError(err))
	}
	return mapTasks(rows)
}

func (r *taskRepository) Transition(ctx context.Context, tenantID domain.TenantID, id domain.TaskID, next task.TaskStatus) error {
	if err := requireUUID("tenant id", tenantID); err != nil {
		return err
	}
	if err := requireUUID("task id", id); err != nil {
		return err
	}
	if err := validateTaskStatus(next); err != nil {
		return err
	}

	current, err := r.GetByID(ctx, tenantID, id)
	if err != nil {
		return err
	}

	updated := *current
	if err := updated.Transition(next, time.Now().UTC()); err != nil { //nolint:govet // short-lived err shadow is idiomatic Go
		return err
	}

	if _, err := r.store.queries.TransitionTask(ctx, sqlc.TransitionTaskParams{ //nolint:govet // short-lived err shadow is idiomatic Go
		ID:             id,
		Status:         string(updated.Status),
		AgentSessionID: pgUUIDFromPtr(current.AgentSessionID),
		TenantID:       tenantID,
	}); err != nil {
		return fmt.Errorf("transition task: %w", classifyError(err))
	}

	return nil
}
