package v1

import (
	"context"
	"net/http"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"github.com/google/uuid"

	"github.com/gosuda/steerlane/internal/audit"
	"github.com/gosuda/steerlane/internal/domain"
	"github.com/gosuda/steerlane/internal/domain/project"

	"github.com/gosuda/steerlane/internal/server/reqctx"
)

// ProjectResponse represents a project.
type ProjectResponse struct {
	Body struct {
		CreatedAt time.Time        `json:"created_at"`
		Settings  map[string]any   `json:"settings,omitempty"`
		Name      string           `json:"name"`
		RepoURL   string           `json:"repo_url"`
		Branch    string           `json:"branch"`
		ID        domain.ProjectID `json:"id"`
	}
}

// ProjectListResponse represents a paginated list of projects.
type ProjectListResponse struct {
	Body struct {
		NextCursor *uuid.UUID         `json:"next_cursor,omitempty"`
		Items      []*ProjectResponse `json:"items"`
	}
}

// CreateProjectRequest is the payload for creating a project.
type CreateProjectRequest struct {
	Body struct {
		Settings map[string]any `json:"settings,omitempty"`
		Name     string         `json:"name" required:"true"`
		RepoURL  string         `json:"repo_url" required:"true"`
		Branch   string         `json:"branch" required:"true"`
	}
}

// UpdateProjectRequest is the payload for updating a project.
type UpdateProjectRequest struct {
	Body struct {
		Name     *string         `json:"name,omitempty"`
		RepoURL  *string         `json:"repo_url,omitempty"`
		Branch   *string         `json:"branch,omitempty"`
		Settings *map[string]any `json:"settings,omitempty"`
	}
	ID domain.ProjectID `path:"id"`
}

// ProjectPathRequest is a common struct for requests targeting a specific project by ID.
type ProjectPathRequest struct {
	ID domain.ProjectID `path:"id"`
}

func mapProject(p *project.Project) *ProjectResponse {
	resp := &ProjectResponse{}
	resp.Body.ID = p.ID
	resp.Body.Name = p.Name
	resp.Body.RepoURL = p.RepoURL
	resp.Body.Branch = p.Branch
	resp.Body.Settings = p.Settings
	resp.Body.CreatedAt = p.CreatedAt
	return resp
}

func (a *API) registerProjects(api huma.API) {
	huma.Register(api, huma.Operation{
		OperationID: "project-list",
		Method:      http.MethodGet,
		Path:        "/api/v1/projects",
		Summary:     "List projects",
		Tags:        []string{"Projects"},
		Security:    []map[string][]string{{"bearer": {}}},
	}, func(ctx context.Context, req *PaginationRequest) (*ProjectListResponse, error) {
		if a.deps.Projects == nil {
			return nil, huma.Error501NotImplemented("project repository not configured")
		}

		tenantID := reqctx.TenantIDFrom(ctx)
		items, err := a.deps.Projects.ListByTenant(ctx, tenantID, req.Limit, req.Cursor)
		if err != nil {
			status, model := MapDomainError(err)
			return nil, huma.NewError(status, model.Detail, err)
		}

		resp := &ProjectListResponse{}
		for _, p := range items {
			resp.Body.Items = append(resp.Body.Items, mapProject(p))
		}

		if len(items) == req.Limit && len(items) > 0 {
			last := items[len(items)-1].ID
			resp.Body.NextCursor = &last
		}

		return resp, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "project-create",
		Method:      http.MethodPost,
		Path:        "/api/v1/projects",
		Summary:     "Create project",
		Tags:        []string{"Projects"},
		Security:    []map[string][]string{{"bearer": {}}},
	}, func(ctx context.Context, req *CreateProjectRequest) (*ProjectResponse, error) {
		if a.deps.Projects == nil {
			return nil, huma.Error501NotImplemented("project repository not configured")
		}

		p := &project.Project{
			ID:        domain.NewID(),
			TenantID:  reqctx.TenantIDFrom(ctx),
			Name:      req.Body.Name,
			RepoURL:   req.Body.RepoURL,
			Branch:    req.Body.Branch,
			Settings:  req.Body.Settings,
			CreatedAt: time.Now(),
		}

		if err := p.Validate(); err != nil {
			status, model := MapDomainError(err)
			return nil, huma.NewError(status, model.Detail, err)
		}

		if err := a.deps.Projects.Create(ctx, p); err != nil {
			status, model := MapDomainError(err)
			return nil, huma.NewError(status, model.Detail, err)
		}

		a.logCRUD(ctx, p.TenantID, audit.ActionCRUDCreate, "project", p.ID, map[string]any{
			"branch": p.Branch,
		})

		return mapProject(p), nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "project-get",
		Method:      http.MethodGet,
		Path:        "/api/v1/projects/{id}",
		Summary:     "Get project",
		Tags:        []string{"Projects"},
		Security:    []map[string][]string{{"bearer": {}}},
	}, func(ctx context.Context, req *ProjectPathRequest) (*ProjectResponse, error) {
		if a.deps.Projects == nil {
			return nil, huma.Error501NotImplemented("project repository not configured")
		}

		p, err := a.deps.Projects.GetByID(ctx, reqctx.TenantIDFrom(ctx), req.ID)
		if err != nil {
			status, model := MapDomainError(err)
			return nil, huma.NewError(status, model.Detail, err)
		}

		return mapProject(p), nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "project-update",
		Method:      http.MethodPatch,
		Path:        "/api/v1/projects/{id}",
		Summary:     "Update project",
		Tags:        []string{"Projects"},
		Security:    []map[string][]string{{"bearer": {}}},
	}, func(ctx context.Context, req *UpdateProjectRequest) (*ProjectResponse, error) {
		if a.deps.Projects == nil {
			return nil, huma.Error501NotImplemented("project repository not configured")
		}

		tenantID := reqctx.TenantIDFrom(ctx)
		p, err := a.deps.Projects.GetByID(ctx, tenantID, req.ID)
		if err != nil {
			status, model := MapDomainError(err)
			return nil, huma.NewError(status, model.Detail, err)
		}

		if req.Body.Name != nil {
			p.Name = *req.Body.Name
		}
		if req.Body.RepoURL != nil {
			p.RepoURL = *req.Body.RepoURL
		}
		if req.Body.Branch != nil {
			p.Branch = *req.Body.Branch
		}
		if req.Body.Settings != nil {
			p.Settings = *req.Body.Settings
		}

		if err := p.Validate(); err != nil { //nolint:govet // short-lived err shadow is idiomatic Go
			status, model := MapDomainError(err)
			return nil, huma.NewError(status, model.Detail, err)
		}

		if err := a.deps.Projects.Update(ctx, p); err != nil { //nolint:govet // short-lived err shadow is idiomatic Go
			status, model := MapDomainError(err)
			return nil, huma.NewError(status, model.Detail, err)
		}

		a.logCRUD(ctx, tenantID, audit.ActionCRUDUpdate, "project", p.ID, nil)

		return mapProject(p), nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "project-delete",
		Method:      http.MethodDelete,
		Path:        "/api/v1/projects/{id}",
		Summary:     "Delete project",
		Tags:        []string{"Projects"},
		Security:    []map[string][]string{{"bearer": {}}},
	}, func(ctx context.Context, req *ProjectPathRequest) (*struct{}, error) {
		if a.deps.Projects == nil {
			return nil, huma.Error501NotImplemented("project repository not configured")
		}

		err := a.deps.Projects.Delete(ctx, reqctx.TenantIDFrom(ctx), req.ID)
		if err != nil {
			status, model := MapDomainError(err)
			return nil, huma.NewError(status, model.Detail, err)
		}

		a.logCRUD(ctx, reqctx.TenantIDFrom(ctx), audit.ActionCRUDDelete, "project", req.ID, nil)

		return nil, nil // 204 No Content
	})
}
