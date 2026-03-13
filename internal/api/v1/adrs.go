package v1

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"github.com/google/uuid"

	"github.com/gosuda/steerlane/internal/domain"
	"github.com/gosuda/steerlane/internal/domain/adr"

	"github.com/gosuda/steerlane/internal/server/reqctx"
)

// ADRResponse represents an Architectural Decision Record.
type ADRResponse struct {
	Body struct {
		UpdatedAt      time.Time              `json:"updated_at"`
		CreatedAt      time.Time              `json:"created_at"`
		CreatedBy      *domain.UserID         `json:"created_by,omitempty"`
		AgentSessionID *domain.AgentSessionID `json:"agent_session_id,omitempty"`
		Decision       string                 `json:"decision"`
		Context        string                 `json:"context"`
		Status         adr.ADRStatus          `json:"status"`
		Title          string                 `json:"title"`
		Consequences   adr.Consequences       `json:"consequences"`
		Drivers        []string               `json:"drivers"`
		Options        json.RawMessage        `json:"options"`
		Sequence       int                    `json:"sequence"`
		ID             domain.ADRID           `json:"id"`
		ProjectID      domain.ProjectID       `json:"project_id"`
	}
}

// ADRListResponse represents a paginated list of ADRs.
type ADRListResponse struct {
	Body struct {
		NextCursor *uuid.UUID     `json:"next_cursor,omitempty"`
		Items      []*ADRResponse `json:"items"`
	}
}

// ListADRsRequest query for ADR listing by project.
type ListADRsRequest struct {
	PaginationRequest
	ProjectID domain.ProjectID `path:"projectId" required:"true" doc:"Project ID"`
}

// ADRPathRequest is a common struct for requests targeting an ADR by ID.
type ADRPathRequest struct {
	ID domain.ADRID `path:"id"`
}

// ReviewADRRequest is the payload for reviewing/updating ADR status.
type ReviewADRRequest struct {
	Body struct {
		Status adr.ADRStatus `json:"status" required:"true" doc:"Next status for the ADR"`
	}
	ID domain.ADRID `path:"id"`
}

func mapADR(a *adr.ADR) *ADRResponse {
	resp := &ADRResponse{}
	resp.Body.ID = a.ID
	resp.Body.ProjectID = a.ProjectID
	resp.Body.Sequence = a.Sequence
	resp.Body.Title = a.Title
	resp.Body.Status = a.Status
	resp.Body.Context = a.Context
	resp.Body.Decision = a.Decision
	resp.Body.Drivers = a.Drivers
	resp.Body.Options = a.Options
	resp.Body.Consequences = a.Consequences
	resp.Body.CreatedBy = a.CreatedBy
	resp.Body.AgentSessionID = a.AgentSessionID
	resp.Body.CreatedAt = a.CreatedAt
	resp.Body.UpdatedAt = a.UpdatedAt
	return resp
}

func (a *API) registerADRs(api huma.API) {
	huma.Register(api, huma.Operation{
		OperationID: "adr-list",
		Method:      http.MethodGet,
		Path:        "/api/v1/projects/{projectId}/adrs",
		Summary:     "List ADRs for a project",
		Tags:        []string{"ADRs"},
		Security:    []map[string][]string{{"bearer": {}}},
	}, func(ctx context.Context, req *ListADRsRequest) (*ADRListResponse, error) {
		if a.deps.ADRs == nil {
			return nil, huma.Error501NotImplemented("adr repository not configured")
		}

		tenantID := reqctx.TenantIDFrom(ctx)
		items, err := a.deps.ADRs.ListByProject(ctx, tenantID, req.ProjectID, req.Limit, req.Cursor)
		if err != nil {
			status, model := MapDomainError(err)
			return nil, huma.NewError(status, model.Detail, err)
		}

		resp := &ADRListResponse{}
		for _, item := range items {
			resp.Body.Items = append(resp.Body.Items, mapADR(item))
		}

		if len(items) == req.Limit && len(items) > 0 {
			last := items[len(items)-1].ID
			resp.Body.NextCursor = &last
		}

		return resp, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "adr-get",
		Method:      http.MethodGet,
		Path:        "/api/v1/adrs/{id}",
		Summary:     "Get ADR by ID",
		Tags:        []string{"ADRs"},
		Security:    []map[string][]string{{"bearer": {}}},
	}, func(ctx context.Context, req *ADRPathRequest) (*ADRResponse, error) {
		if a.deps.ADRs == nil {
			return nil, huma.Error501NotImplemented("adr repository not configured")
		}

		item, err := a.deps.ADRs.GetByID(ctx, reqctx.TenantIDFrom(ctx), req.ID)
		if err != nil {
			status, model := MapDomainError(err)
			return nil, huma.NewError(status, model.Detail, err)
		}

		return mapADR(item), nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "adr-review",
		Method:      http.MethodPost,
		Path:        "/api/v1/adrs/{id}/review",
		Summary:     "Review and transition ADR status",
		Tags:        []string{"ADRs"},
		Security:    []map[string][]string{{"bearer": {}}},
	}, func(ctx context.Context, req *ReviewADRRequest) (*ADRResponse, error) {
		if a.deps.ADRs == nil {
			return nil, huma.Error501NotImplemented("adr repository not configured")
		}

		tenantID := reqctx.TenantIDFrom(ctx)
		before, err := a.deps.ADRs.GetByID(ctx, tenantID, req.ID)
		if err != nil {
			status, model := MapDomainError(err)
			return nil, huma.NewError(status, model.Detail, err)
		}

		err = a.deps.ADRs.UpdateStatus(ctx, tenantID, req.ID, req.Body.Status)
		if err != nil {
			status, model := MapDomainError(err)
			return nil, huma.NewError(status, model.Detail, err)
		}

		item, err := a.deps.ADRs.GetByID(ctx, tenantID, req.ID)
		if err != nil {
			status, model := MapDomainError(err)
			return nil, huma.NewError(status, model.Detail, err)
		}

		a.logStateTransition(ctx, tenantID, "adr", item.ID, string(before.Status), string(item.Status), map[string]any{
			"project_id": item.ProjectID,
		})

		return mapADR(item), nil
	})
}
