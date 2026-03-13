package v1

import (
	"context"
	"net/http"
	"strings"

	"github.com/danielgtaylor/huma/v2"
	"github.com/google/uuid"

	"github.com/gosuda/steerlane/internal/audit"
	"github.com/gosuda/steerlane/internal/domain"
	domainaudit "github.com/gosuda/steerlane/internal/domain/audit"
)

// RegisterRequest is the payload for user registration.
type RegisterRequest struct {
	Body struct {
		Email      string          `json:"email" format:"email" required:"true" doc:"User email address"`
		Password   string          `json:"password" required:"true" minLength:"8" doc:"User password (min 8 chars)"`
		Name       string          `json:"name" required:"true" doc:"User full name"`
		TenantSlug string          `json:"tenant_slug,omitempty" doc:"Tenant slug to join (recommended for browser flows)"`
		TenantID   domain.TenantID `json:"tenant_id,omitempty" doc:"Tenant ID to join (optional when tenant_slug is provided)"`
	}
}

// LoginRequest is the payload for user login.
type LoginRequest struct {
	Body struct {
		Email      string          `json:"email" format:"email" required:"true" doc:"User email address"`
		Password   string          `json:"password" required:"true" doc:"User password"`
		TenantSlug string          `json:"tenant_slug,omitempty" doc:"Tenant slug to authenticate against"`
		TenantID   domain.TenantID `json:"tenant_id,omitempty" doc:"Tenant ID (optional when tenant_slug is provided)"`
	}
}

// RefreshRequest is the payload for token refresh.
type RefreshRequest struct {
	Body struct {
		RefreshToken string `json:"refresh_token" required:"true" doc:"The refresh token issued during login"`
	}
}

// TokenResponse contains the JWT tokens.
type TokenResponse struct {
	Body struct {
		AccessToken  string `json:"access_token" doc:"JWT access token (short-lived)"`
		RefreshToken string `json:"refresh_token" doc:"Refresh token (long-lived)"`
	}
}

func (a *API) resolveAuthTenantID(ctx context.Context, tenantID domain.TenantID, tenantSlug string) (domain.TenantID, error) {
	if tenantID != uuid.Nil {
		return tenantID, nil
	}

	trimmedSlug := strings.TrimSpace(tenantSlug)
	if trimmedSlug == "" {
		return uuid.Nil, huma.Error400BadRequest("tenant_id or tenant_slug is required")
	}
	if a.deps.Tenants == nil {
		return uuid.Nil, huma.Error501NotImplemented("tenant repository not configured")
	}

	tenantRecord, err := a.deps.Tenants.GetBySlug(ctx, trimmedSlug)
	if err != nil {
		status, model := MapDomainError(err)
		return uuid.Nil, huma.NewError(status, model.Detail, err)
	}

	return tenantRecord.ID, nil
}

func (a *API) registerAuth(api huma.API) {
	huma.Register(api, huma.Operation{
		OperationID: "auth-register",
		Method:      http.MethodPost,
		Path:        "/api/v1/auth/register",
		Summary:     "Register a new user",
		Tags:        []string{"Auth"},
	}, func(ctx context.Context, req *RegisterRequest) (*struct{}, error) {
		if a.deps.Auth == nil {
			return nil, huma.Error501NotImplemented("auth service not configured")
		}

		tenantID, err := a.resolveAuthTenantID(ctx, req.Body.TenantID, req.Body.TenantSlug)
		if err != nil {
			return nil, err
		}

		registeredUser, err := a.deps.Auth.Register(ctx, tenantID, req.Body.Email, req.Body.Password, req.Body.Name)
		if err != nil {
			status, model := MapDomainError(err)
			return nil, huma.NewError(status, model.Detail, err)
		}

		if a.deps.Audit != nil {
			_, _ = a.deps.Audit.LogCRUD(ctx, tenantID, audit.Actor{
				Type: domainaudit.ActorUser,
				ID:   registeredUser.ID,
			}, audit.ActionCRUDCreate, audit.Resource{
				Type: "user",
				ID:   registeredUser.ID,
			}, auditDetails(ctx, map[string]any{
				"email":  req.Body.Email,
				"source": "auth.register",
			}))
		}

		return nil, nil // 204 No Content
	})

	huma.Register(api, huma.Operation{
		OperationID: "auth-login",
		Method:      http.MethodPost,
		Path:        "/api/v1/auth/login",
		Summary:     "Log in",
		Tags:        []string{"Auth"},
	}, func(ctx context.Context, req *LoginRequest) (*TokenResponse, error) {
		if a.deps.Auth == nil {
			return nil, huma.Error501NotImplemented("auth service not configured")
		}

		tenantID, err := a.resolveAuthTenantID(ctx, req.Body.TenantID, req.Body.TenantSlug)
		if err != nil {
			return nil, err
		}

		access, refresh, err := a.deps.Auth.Login(ctx, tenantID, req.Body.Email, req.Body.Password)
		if err != nil {
			status, model := MapDomainError(err)
			return nil, huma.NewError(status, model.Detail, err)
		}

		resp := &TokenResponse{}
		resp.Body.AccessToken = access
		resp.Body.RefreshToken = refresh

		claims, claimsErr := a.deps.Auth.AuthenticateJWT(access)
		if claimsErr == nil {
			userID, userErr := claims.SubjectUUID()
			if userErr == nil {
				a.logAuthEvent(ctx, tenantID, audit.Actor{Type: domainaudit.ActorUser, ID: userID}, audit.ActionAuthLogin, "user", userID, map[string]any{
					"email":  req.Body.Email,
					"method": "password",
				})
			}
		}

		return resp, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "auth-refresh",
		Method:      http.MethodPost,
		Path:        "/api/v1/auth/refresh",
		Summary:     "Refresh access token",
		Tags:        []string{"Auth"},
	}, func(ctx context.Context, req *RefreshRequest) (*TokenResponse, error) {
		if a.deps.Auth == nil {
			return nil, huma.Error501NotImplemented("auth service not configured")
		}

		access, refresh, err := a.deps.Auth.RefreshToken(ctx, req.Body.RefreshToken)
		if err != nil {
			status, model := MapDomainError(err)
			return nil, huma.NewError(status, model.Detail, err)
		}

		resp := &TokenResponse{}
		resp.Body.AccessToken = access
		resp.Body.RefreshToken = refresh

		claims, claimsErr := a.deps.Auth.AuthenticateJWT(access)
		if claimsErr == nil {
			userID, userErr := claims.SubjectUUID()
			if userErr == nil {
				a.logAuthEvent(ctx, claims.TenantID, audit.Actor{Type: domainaudit.ActorUser, ID: userID}, audit.ActionAuthRefresh, "user", userID, map[string]any{
					"method": "refresh_token",
				})
			}
		}

		return resp, nil
	})
}
