package v1

import (
	"context"
	"net/http"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/adapters/humago"
	"github.com/google/uuid"

	"github.com/gosuda/steerlane/internal/audit"
	"github.com/gosuda/steerlane/internal/auth"
	"github.com/gosuda/steerlane/internal/domain"
	"github.com/gosuda/steerlane/internal/domain/adr"
	"github.com/gosuda/steerlane/internal/domain/agent"
	domainaudit "github.com/gosuda/steerlane/internal/domain/audit"
	"github.com/gosuda/steerlane/internal/domain/project"
	"github.com/gosuda/steerlane/internal/domain/task"
	"github.com/gosuda/steerlane/internal/domain/tenant"
	"github.com/gosuda/steerlane/internal/domain/user"
	"github.com/gosuda/steerlane/internal/hitlrouter"
	"github.com/gosuda/steerlane/internal/orchestrator"
	"github.com/gosuda/steerlane/internal/server/reqctx"
)

// Dependencies holds the domain services and repositories required by the API handlers.
type Dependencies struct {
	Audit         *audit.Service
	Auth          *auth.Service
	Orchestrator  *orchestrator.Orchestrator
	HITLRouter    *hitlrouter.Router
	Tenants       tenant.Repository
	Users         user.Repository
	Projects      project.Repository
	Tasks         task.Repository
	ADRs          adr.Repository
	AgentSessions agent.Repository
	AgentEvents   agent.EventRepository
}

// API provides Huma operation handlers for all v1 endpoints.
type API struct {
	deps Dependencies
}

// New creates a new API instance with the given dependencies.
func New(deps Dependencies) *API {
	return &API{deps: deps}
}

func (a *API) auditActorFromContext(ctx context.Context) (audit.Actor, bool) {
	userID := reqctx.UserIDFrom(ctx)
	if userID == uuid.Nil {
		return audit.Actor{}, false
	}

	return audit.Actor{Type: domainaudit.ActorUser, ID: userID}, true
}

func auditDetails(ctx context.Context, details map[string]any) map[string]any {
	requestID := reqctx.RequestIDFrom(ctx)
	if requestID == "" {
		return details
	}
	if details == nil {
		details = map[string]any{}
	}
	if _, exists := details["request_id"]; !exists {
		details["request_id"] = requestID
	}

	return details
}

func (a *API) logCRUD(ctx context.Context, tenantID domain.TenantID, action audit.Action, resourceType string, resourceID uuid.UUID, details map[string]any) {
	if a.deps.Audit == nil {
		return
	}

	actor, ok := a.auditActorFromContext(ctx)
	if !ok {
		return
	}

	_, _ = a.deps.Audit.LogCRUD(ctx, tenantID, actor, action, audit.Resource{
		Type: resourceType,
		ID:   resourceID,
	}, auditDetails(ctx, details))
}

func (a *API) logStateTransition(ctx context.Context, tenantID domain.TenantID, resourceType string, resourceID uuid.UUID, from, to string, details map[string]any) {
	if a.deps.Audit == nil {
		return
	}

	actor, ok := a.auditActorFromContext(ctx)
	if !ok {
		return
	}

	_, _ = a.deps.Audit.LogStateTransition(ctx, audit.StateTransitionInput{
		TenantID: tenantID,
		Actor:    actor,
		Resource: audit.Resource{Type: resourceType, ID: resourceID},
		From:     from,
		To:       to,
		Details:  auditDetails(ctx, details),
	})
}

func (a *API) logAuthEvent(ctx context.Context, tenantID domain.TenantID, actor audit.Actor, action audit.Action, resourceType string, resourceID uuid.UUID, details map[string]any) {
	if a.deps.Audit == nil {
		return
	}

	_, _ = a.deps.Audit.LogAuthEvent(ctx, tenantID, actor, action, audit.Resource{
		Type: resourceType,
		ID:   resourceID,
	}, auditDetails(ctx, details))
}

// wrapMiddleware converts a standard HTTP middleware into a huma Middleware.
func wrapMiddleware(mw func(http.Handler) http.Handler) func(huma.Context, func(huma.Context)) {
	return func(ctx huma.Context, next func(huma.Context)) {
		req, w := humago.Unwrap(ctx)
		handler := mw(http.HandlerFunc(func(w2 http.ResponseWriter, r2 *http.Request) {
			ctx = huma.WithContext(ctx, r2.Context())
			next(ctx)
		}))
		handler.ServeHTTP(w, req)
	}
}

// ensureSecurity defines the bearer security scheme in the OpenAPI spec.
func ensureSecurity(api huma.API) {
	spec := api.OpenAPI()
	if spec.Components == nil {
		spec.Components = &huma.Components{}
	}
	if spec.Components.SecuritySchemes == nil {
		spec.Components.SecuritySchemes = make(map[string]*huma.SecurityScheme)
	}
	spec.Components.SecuritySchemes["bearer"] = &huma.SecurityScheme{
		Type:         "http",
		Scheme:       "bearer",
		BearerFormat: "JWT",
		Description:  "JWT Access Token",
	}
	spec.Components.SecuritySchemes["apiKey"] = &huma.SecurityScheme{
		Type:        "apiKey",
		In:          "header",
		Name:        "X-API-Key",
		Description: "Static API Key for integrations",
	}
}

// Register registers all v1 endpoints on the provided Huma API instance.
func Register(api huma.API, deps Dependencies, authMw, tenantMw func(http.Handler) http.Handler) {
	ensureSecurity(api)
	a := New(deps)

	// Public routes
	if deps.Auth != nil {
		a.registerAuth(api)
	}

	// Authenticated routes group
	authGrp := huma.NewGroup(api)
	if authMw != nil {
		authGrp.UseMiddleware(wrapMiddleware(authMw))
	}
	if tenantMw != nil {
		authGrp.UseMiddleware(wrapMiddleware(tenantMw))
	}

	if deps.Tenants != nil {
		a.registerTenants(authGrp)
	}
	if deps.Users != nil {
		a.registerUsers(authGrp)
	}
	if deps.Projects != nil {
		a.registerProjects(authGrp)
	}
	if deps.Tasks != nil {
		a.registerTasks(authGrp)
	}
	if deps.ADRs != nil {
		a.registerADRs(authGrp)
	}
	if deps.AgentSessions != nil || deps.Orchestrator != nil {
		a.registerAgents(authGrp)
	}
}
