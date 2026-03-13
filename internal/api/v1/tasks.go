package v1

import (
	"context"
	"net/http"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"github.com/google/uuid"

	"github.com/gosuda/steerlane/internal/audit"
	"github.com/gosuda/steerlane/internal/domain"
	"github.com/gosuda/steerlane/internal/domain/task"

	"github.com/gosuda/steerlane/internal/server/reqctx"
)

// TaskResponse represents a single task.
type TaskResponse struct {
	Body struct {
		CreatedAt      time.Time              `json:"created_at"`
		UpdatedAt      time.Time              `json:"updated_at"`
		ADRID          *domain.ADRID          `json:"adr_id,omitempty"`
		AssignedTo     *domain.UserID         `json:"assigned_to,omitempty"`
		AgentSessionID *domain.AgentSessionID `json:"agent_session_id,omitempty"`
		Title          string                 `json:"title"`
		Description    string                 `json:"description"`
		Status         task.TaskStatus        `json:"status"`
		Priority       int                    `json:"priority"`
		ID             domain.TaskID          `json:"id"`
		ProjectID      domain.ProjectID       `json:"project_id"`
	}
}

// TaskListResponse represents a paginated list of tasks.
type TaskListResponse struct {
	Body struct {
		NextCursor *uuid.UUID      `json:"next_cursor,omitempty"`
		Items      []*TaskResponse `json:"items"`
	}
}

// ListTasksRequest query for task listing.
type ListTasksRequest struct {
	Status   *task.TaskStatus `query:"status" required:"false" doc:"Filter by task status"`
	Priority *int             `query:"priority" required:"false" doc:"Filter by task priority"`
	PaginationRequest
	ProjectID domain.ProjectID `query:"project_id" required:"true" doc:"Project ID to list tasks for"`
}

// CreateTaskRequest is the payload for creating a task.
type CreateTaskRequest struct {
	Body struct {
		Title       string           `json:"title" required:"true"`
		Description string           `json:"description,omitempty"`
		Status      task.TaskStatus  `json:"status" default:"backlog"`
		Priority    int              `json:"priority" default:"2"`
		ProjectID   domain.ProjectID `json:"project_id" required:"true"`
	}
}

// UpdateTaskRequest is the payload for updating a task.
type UpdateTaskRequest struct {
	Body struct {
		Title       *string        `json:"title,omitempty"`
		Description *string        `json:"description,omitempty"`
		Priority    *int           `json:"priority,omitempty"`
		AssignedTo  *domain.UserID `json:"assigned_to,omitempty"`
	}
	ID domain.TaskID `path:"id"`
}

// TransitionTaskRequest is the payload for changing a task status.
type TransitionTaskRequest struct {
	Body struct {
		Status task.TaskStatus `json:"status" required:"true"`
	}
	ID domain.TaskID `path:"id"`
}

// TaskPathRequest is a common struct for requests targeting a task by ID.
type TaskPathRequest struct {
	ID domain.TaskID `path:"id"`
}

func mapTask(t *task.Task) *TaskResponse {
	resp := &TaskResponse{}
	resp.Body.ID = t.ID
	resp.Body.ProjectID = t.ProjectID
	resp.Body.ADRID = t.ADRID
	resp.Body.Title = t.Title
	resp.Body.Description = t.Description
	resp.Body.Status = t.Status
	resp.Body.Priority = t.Priority
	resp.Body.AssignedTo = t.AssignedTo
	resp.Body.AgentSessionID = t.AgentSessionID
	resp.Body.CreatedAt = t.CreatedAt
	resp.Body.UpdatedAt = t.UpdatedAt
	return resp
}

func (a *API) registerTasks(api huma.API) {
	huma.Register(api, huma.Operation{
		OperationID: "task-list",
		Method:      http.MethodGet,
		Path:        "/api/v1/tasks",
		Summary:     "List tasks",
		Tags:        []string{"Tasks"},
		Security:    []map[string][]string{{"bearer": {}}},
	}, func(ctx context.Context, req *ListTasksRequest) (*TaskListResponse, error) {
		if a.deps.Tasks == nil {
			return nil, huma.Error501NotImplemented("task repository not configured")
		}

		filter := task.Filter{
			Status:   req.Status,
			Priority: req.Priority,
		}

		tenantID := reqctx.TenantIDFrom(ctx)
		items, err := a.deps.Tasks.ListByProject(ctx, tenantID, req.ProjectID, filter, req.Limit, req.Cursor)
		if err != nil {
			status, model := MapDomainError(err)
			return nil, huma.NewError(status, model.Detail, err)
		}

		resp := &TaskListResponse{}
		for _, t := range items {
			resp.Body.Items = append(resp.Body.Items, mapTask(t))
		}

		if len(items) == req.Limit && len(items) > 0 {
			last := items[len(items)-1].ID
			resp.Body.NextCursor = &last
		}

		return resp, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "task-create",
		Method:      http.MethodPost,
		Path:        "/api/v1/tasks",
		Summary:     "Create task",
		Tags:        []string{"Tasks"},
		Security:    []map[string][]string{{"bearer": {}}},
	}, func(ctx context.Context, req *CreateTaskRequest) (*TaskResponse, error) {
		if a.deps.Tasks == nil {
			return nil, huma.Error501NotImplemented("task repository not configured")
		}

		if req.Body.Status == "" {
			req.Body.Status = task.StatusBacklog
		}

		now := time.Now()
		t := &task.Task{
			ID:          domain.NewID(),
			TenantID:    reqctx.TenantIDFrom(ctx),
			ProjectID:   req.Body.ProjectID,
			Title:       req.Body.Title,
			Description: req.Body.Description,
			Status:      req.Body.Status,
			Priority:    req.Body.Priority,
			CreatedAt:   now,
			UpdatedAt:   now,
		}

		if err := t.Validate(); err != nil {
			status, model := MapDomainError(err)
			return nil, huma.NewError(status, model.Detail, err)
		}

		if err := a.deps.Tasks.Create(ctx, t); err != nil {
			status, model := MapDomainError(err)
			return nil, huma.NewError(status, model.Detail, err)
		}

		a.logCRUD(ctx, t.TenantID, audit.ActionCRUDCreate, "task", t.ID, map[string]any{
			"project_id": t.ProjectID,
			"status":     t.Status,
		})

		return mapTask(t), nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "task-get",
		Method:      http.MethodGet,
		Path:        "/api/v1/tasks/{id}",
		Summary:     "Get task",
		Tags:        []string{"Tasks"},
		Security:    []map[string][]string{{"bearer": {}}},
	}, func(ctx context.Context, req *TaskPathRequest) (*TaskResponse, error) {
		if a.deps.Tasks == nil {
			return nil, huma.Error501NotImplemented("task repository not configured")
		}

		t, err := a.deps.Tasks.GetByID(ctx, reqctx.TenantIDFrom(ctx), req.ID)
		if err != nil {
			status, model := MapDomainError(err)
			return nil, huma.NewError(status, model.Detail, err)
		}

		return mapTask(t), nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "task-update",
		Method:      http.MethodPatch,
		Path:        "/api/v1/tasks/{id}",
		Summary:     "Update task",
		Tags:        []string{"Tasks"},
		Security:    []map[string][]string{{"bearer": {}}},
	}, func(ctx context.Context, req *UpdateTaskRequest) (*TaskResponse, error) {
		if a.deps.Tasks == nil {
			return nil, huma.Error501NotImplemented("task repository not configured")
		}

		tenantID := reqctx.TenantIDFrom(ctx)
		t, err := a.deps.Tasks.GetByID(ctx, tenantID, req.ID)
		if err != nil {
			status, model := MapDomainError(err)
			return nil, huma.NewError(status, model.Detail, err)
		}

		if req.Body.Title != nil {
			t.Title = *req.Body.Title
		}
		if req.Body.Description != nil {
			t.Description = *req.Body.Description
		}
		if req.Body.Priority != nil {
			t.Priority = *req.Body.Priority
		}
		if req.Body.AssignedTo != nil {
			t.AssignedTo = req.Body.AssignedTo
		}

		t.UpdatedAt = time.Now()
		if err := t.Validate(); err != nil { //nolint:govet // short-lived err shadow is idiomatic Go
			status, model := MapDomainError(err)
			return nil, huma.NewError(status, model.Detail, err)
		}

		if err := a.deps.Tasks.Update(ctx, t); err != nil { //nolint:govet // short-lived err shadow is idiomatic Go
			status, model := MapDomainError(err)
			return nil, huma.NewError(status, model.Detail, err)
		}

		a.logCRUD(ctx, tenantID, audit.ActionCRUDUpdate, "task", t.ID, map[string]any{
			"project_id": t.ProjectID,
		})

		return mapTask(t), nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "task-transition",
		Method:      http.MethodPost,
		Path:        "/api/v1/tasks/{id}/transition",
		Summary:     "Transition task status",
		Tags:        []string{"Tasks"},
		Security:    []map[string][]string{{"bearer": {}}},
	}, func(ctx context.Context, req *TransitionTaskRequest) (*TaskResponse, error) {
		if a.deps.Tasks == nil {
			return nil, huma.Error501NotImplemented("task repository not configured")
		}

		tenantID := reqctx.TenantIDFrom(ctx)
		before, err := a.deps.Tasks.GetByID(ctx, tenantID, req.ID)
		if err != nil {
			status, model := MapDomainError(err)
			return nil, huma.NewError(status, model.Detail, err)
		}

		// Use the specific Transition method which handles atomic validation
		if transErr := a.deps.Tasks.Transition(ctx, tenantID, req.ID, req.Body.Status); transErr != nil {
			status, model := MapDomainError(transErr)
			return nil, huma.NewError(status, model.Detail, transErr)
		}

		// Fetch the updated task to return
		t, err := a.deps.Tasks.GetByID(ctx, tenantID, req.ID)
		if err != nil {
			status, model := MapDomainError(err)
			return nil, huma.NewError(status, model.Detail, err)
		}

		a.logStateTransition(ctx, tenantID, "task", t.ID, string(before.Status), string(t.Status), map[string]any{
			"project_id": t.ProjectID,
		})

		return mapTask(t), nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "task-delete",
		Method:      http.MethodDelete,
		Path:        "/api/v1/tasks/{id}",
		Summary:     "Delete task",
		Tags:        []string{"Tasks"},
		Security:    []map[string][]string{{"bearer": {}}},
	}, func(ctx context.Context, req *TaskPathRequest) (*struct{}, error) {
		if a.deps.Tasks == nil {
			return nil, huma.Error501NotImplemented("task repository not configured")
		}

		err := a.deps.Tasks.Delete(ctx, reqctx.TenantIDFrom(ctx), req.ID)
		if err != nil {
			status, model := MapDomainError(err)
			return nil, huma.NewError(status, model.Detail, err)
		}

		a.logCRUD(ctx, reqctx.TenantIDFrom(ctx), audit.ActionCRUDDelete, "task", req.ID, nil)

		return nil, nil // 204 No Content
	})
}
