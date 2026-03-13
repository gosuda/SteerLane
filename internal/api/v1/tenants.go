package v1

import (
	"context"
	"net/http"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"github.com/google/uuid"

	"github.com/gosuda/steerlane/internal/audit"
	"github.com/gosuda/steerlane/internal/domain"
	"github.com/gosuda/steerlane/internal/domain/tenant"

	"github.com/gosuda/steerlane/internal/server/reqctx"
)

// TenantResponse represents a tenant.
type TenantResponse struct {
	Body struct {
		CreatedAt time.Time       `json:"created_at"`
		UpdatedAt time.Time       `json:"updated_at"`
		Settings  map[string]any  `json:"settings,omitempty"`
		Name      string          `json:"name"`
		Slug      string          `json:"slug"`
		ID        domain.TenantID `json:"id"`
	}
}

// UpdateTenantRequest is the payload for updating a tenant.
type UpdateTenantRequest struct {
	Body struct {
		Name     *string         `json:"name,omitempty" doc:"New tenant name"`
		Settings *map[string]any `json:"settings,omitempty" doc:"Updated settings"`
	}
}

func mapTenant(t *tenant.Tenant) *TenantResponse {
	resp := &TenantResponse{}
	resp.Body.ID = t.ID
	resp.Body.Name = t.Name
	resp.Body.Slug = t.Slug
	resp.Body.Settings = t.Settings
	resp.Body.CreatedAt = t.CreatedAt
	resp.Body.UpdatedAt = t.UpdatedAt
	return resp
}

func (a *API) registerTenants(api huma.API) {
	huma.Register(api, huma.Operation{
		OperationID: "tenant-get-me",
		Method:      http.MethodGet,
		Path:        "/api/v1/tenants/me",
		Summary:     "Get current tenant",
		Tags:        []string{"Tenants"},
		Security:    []map[string][]string{{"bearer": {}}},
	}, func(ctx context.Context, _ *struct{}) (*TenantResponse, error) {
		if a.deps.Tenants == nil {
			return nil, huma.Error501NotImplemented("tenant repository not configured")
		}

		tenantID := reqctx.TenantIDFrom(ctx)
		if tenantID == uuid.Nil {
			return nil, huma.Error401Unauthorized("missing tenant context")
		}

		t, err := a.deps.Tenants.GetByID(ctx, tenantID)
		if err != nil {
			status, model := MapDomainError(err)
			return nil, huma.NewError(status, model.Detail, err)
		}

		return mapTenant(t), nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "tenant-update-me",
		Method:      http.MethodPatch,
		Path:        "/api/v1/tenants/me",
		Summary:     "Update current tenant",
		Tags:        []string{"Tenants"},
		Security:    []map[string][]string{{"bearer": {}}},
	}, func(ctx context.Context, req *UpdateTenantRequest) (*TenantResponse, error) {
		if a.deps.Tenants == nil {
			return nil, huma.Error501NotImplemented("tenant repository not configured")
		}

		tenantID := reqctx.TenantIDFrom(ctx)
		if tenantID == uuid.Nil {
			return nil, huma.Error401Unauthorized("missing tenant context")
		}

		t, err := a.deps.Tenants.GetByID(ctx, tenantID)
		if err != nil {
			status, model := MapDomainError(err)
			return nil, huma.NewError(status, model.Detail, err)
		}

		if req.Body.Name != nil {
			t.Name = *req.Body.Name
		}
		if req.Body.Settings != nil {
			t.Settings = *req.Body.Settings
		}

		t.UpdatedAt = time.Now()
		if err := t.Validate(); err != nil { //nolint:govet // short-lived err shadow is idiomatic Go
			status, model := MapDomainError(err)
			return nil, huma.NewError(status, model.Detail, err)
		}

		if err := a.deps.Tenants.Update(ctx, t); err != nil { //nolint:govet // short-lived err shadow is idiomatic Go
			status, model := MapDomainError(err)
			return nil, huma.NewError(status, model.Detail, err)
		}

		a.logCRUD(ctx, tenantID, audit.ActionCRUDUpdate, "tenant", t.ID, nil)

		return mapTenant(t), nil
	})
}
