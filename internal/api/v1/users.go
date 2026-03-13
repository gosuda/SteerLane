package v1

import (
	"context"
	"net/http"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"github.com/google/uuid"

	"github.com/gosuda/steerlane/internal/audit"
	"github.com/gosuda/steerlane/internal/domain"
	"github.com/gosuda/steerlane/internal/domain/user"
	"github.com/gosuda/steerlane/internal/server/reqctx"
)

// UserResponse represents a single user.
type UserResponse struct {
	Body struct {
		CreatedAt time.Time       `json:"created_at"`
		UpdatedAt time.Time       `json:"updated_at"`
		Email     *string         `json:"email,omitempty"`
		AvatarURL *string         `json:"avatar_url,omitempty"`
		Name      string          `json:"name"`
		Role      user.Role       `json:"role"`
		ID        domain.UserID   `json:"id"`
		TenantID  domain.TenantID `json:"tenant_id"`
	}
}

// UserListResponse represents a paginated list of users.
type UserListResponse struct {
	Body struct {
		NextCursor *uuid.UUID      `json:"next_cursor,omitempty"`
		Items      []*UserResponse `json:"items"`
	}
}

// UpdateUserRequest is the payload for updating a user.
type UpdateUserRequest struct {
	Body struct {
		Email     *string    `json:"email,omitempty"`
		AvatarURL *string    `json:"avatar_url,omitempty"`
		Name      *string    `json:"name,omitempty"`
		Role      *user.Role `json:"role,omitempty"`
	}
	ID domain.UserID `path:"id"`
}

// UserPathRequest is a common struct for requests targeting a user by ID.
type UserPathRequest struct {
	ID domain.UserID `path:"id"`
}

func mapUser(u *user.User) *UserResponse {
	resp := &UserResponse{}
	resp.Body.ID = u.ID
	resp.Body.TenantID = u.TenantID
	resp.Body.Email = u.Email
	resp.Body.AvatarURL = u.AvatarURL
	resp.Body.Name = u.Name
	resp.Body.Role = u.Role
	resp.Body.CreatedAt = u.CreatedAt
	resp.Body.UpdatedAt = u.UpdatedAt
	return resp
}

func (a *API) registerUsers(api huma.API) {
	huma.Register(api, huma.Operation{
		OperationID: "user-list",
		Method:      http.MethodGet,
		Path:        "/api/v1/users",
		Summary:     "List users",
		Tags:        []string{"Users"},
		Security:    []map[string][]string{{"bearer": {}}},
	}, func(ctx context.Context, req *PaginationRequest) (*UserListResponse, error) {
		if a.deps.Users == nil {
			return nil, huma.Error501NotImplemented("user repository not configured")
		}

		cursor := ""
		if req.Cursor != nil {
			cursor = req.Cursor.String()
		}

		items, err := a.deps.Users.ListByTenant(ctx, reqctx.TenantIDFrom(ctx), req.Limit, cursor)
		if err != nil {
			status, model := MapDomainError(err)
			return nil, huma.NewError(status, model.Detail, err)
		}

		resp := &UserListResponse{}
		for _, item := range items {
			resp.Body.Items = append(resp.Body.Items, mapUser(item))
		}

		if len(items) == req.Limit && len(items) > 0 {
			last := items[len(items)-1].ID
			resp.Body.NextCursor = &last
		}

		return resp, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "user-get",
		Method:      http.MethodGet,
		Path:        "/api/v1/users/{id}",
		Summary:     "Get user",
		Tags:        []string{"Users"},
		Security:    []map[string][]string{{"bearer": {}}},
	}, func(ctx context.Context, req *UserPathRequest) (*UserResponse, error) {
		if a.deps.Users == nil {
			return nil, huma.Error501NotImplemented("user repository not configured")
		}

		item, err := a.deps.Users.GetByID(ctx, reqctx.TenantIDFrom(ctx), req.ID)
		if err != nil {
			status, model := MapDomainError(err)
			return nil, huma.NewError(status, model.Detail, err)
		}

		return mapUser(item), nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "user-update",
		Method:      http.MethodPatch,
		Path:        "/api/v1/users/{id}",
		Summary:     "Update user",
		Tags:        []string{"Users"},
		Security:    []map[string][]string{{"bearer": {}}},
	}, func(ctx context.Context, req *UpdateUserRequest) (*UserResponse, error) {
		if a.deps.Users == nil {
			return nil, huma.Error501NotImplemented("user repository not configured")
		}

		isAdmin := reqctx.UserRoleFrom(ctx) == string(user.RoleAdmin)
		if !isAdmin && reqctx.UserIDFrom(ctx) != req.ID {
			status, model := MapDomainError(domain.ErrForbidden)
			return nil, huma.NewError(status, model.Detail, domain.ErrForbidden)
		}

		item, err := a.deps.Users.GetByID(ctx, reqctx.TenantIDFrom(ctx), req.ID)
		if err != nil {
			status, model := MapDomainError(err)
			return nil, huma.NewError(status, model.Detail, err)
		}

		if req.Body.Email != nil {
			item.Email = req.Body.Email
		}
		if req.Body.AvatarURL != nil {
			item.AvatarURL = req.Body.AvatarURL
		}
		if req.Body.Name != nil {
			item.Name = *req.Body.Name
		}
		if req.Body.Role != nil {
			if !isAdmin {
				status, model := MapDomainError(domain.ErrForbidden)
				return nil, huma.NewError(status, model.Detail, domain.ErrForbidden)
			}
			item.Role = *req.Body.Role
		}

		item.UpdatedAt = time.Now()
		if err := item.Validate(); err != nil { //nolint:govet // short-lived err shadow is idiomatic Go
			status, model := MapDomainError(err)
			return nil, huma.NewError(status, model.Detail, err)
		}

		if err := a.deps.Users.Update(ctx, item); err != nil { //nolint:govet // short-lived err shadow is idiomatic Go
			status, model := MapDomainError(err)
			return nil, huma.NewError(status, model.Detail, err)
		}

		a.logCRUD(ctx, item.TenantID, audit.ActionCRUDUpdate, "user", item.ID, nil)

		return mapUser(item), nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "user-delete",
		Method:      http.MethodDelete,
		Path:        "/api/v1/users/{id}",
		Summary:     "Delete user",
		Tags:        []string{"Users"},
		Security:    []map[string][]string{{"bearer": {}}},
	}, func(ctx context.Context, req *UserPathRequest) (*struct{}, error) {
		if a.deps.Users == nil {
			return nil, huma.Error501NotImplemented("user repository not configured")
		}
		if reqctx.UserRoleFrom(ctx) != string(user.RoleAdmin) {
			status, model := MapDomainError(domain.ErrForbidden)
			return nil, huma.NewError(status, model.Detail, domain.ErrForbidden)
		}

		err := a.deps.Users.Delete(ctx, reqctx.TenantIDFrom(ctx), req.ID)
		if err != nil {
			status, model := MapDomainError(err)
			return nil, huma.NewError(status, model.Detail, err)
		}

		a.logCRUD(ctx, reqctx.TenantIDFrom(ctx), audit.ActionCRUDDelete, "user", req.ID, nil)

		return nil, nil
	})
}
