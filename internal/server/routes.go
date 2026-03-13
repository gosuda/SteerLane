package server

import (
	"context"
	"expvar"
	"net/http"

	"github.com/danielgtaylor/huma/v2"

	v1 "github.com/gosuda/steerlane/internal/api/v1"
	"github.com/gosuda/steerlane/internal/api/ws"
	"github.com/gosuda/steerlane/internal/server/middleware"
)

// HealthOutput is the response body for the health check endpoint.
type HealthOutput struct {
	Body struct {
		Status string `json:"status" example:"ok" doc:"Server health status"`
	}
}

// RegisterRoutes registers all HTTP routes on the server's mux and Huma API.
// Routes are organized into groups:
//   - Public: health check, OpenAPI spec (served automatically by Huma)
//   - Auth: login, register, token refresh (Phase 1A.10)
//   - Authenticated: tenant-scoped CRUD operations (Phase 1A.10)
//   - Admin: tenant management, user administration (Phase 1A.10)
func (s *Server) RegisterRoutes() {
	// -----------------------------------------------------------------------
	// Public routes (no authentication required)
	// -----------------------------------------------------------------------

	// Health check — used by load balancers and container orchestrators.
	huma.Register(s.api, huma.Operation{
		OperationID: "health-check",
		Method:      http.MethodGet,
		Path:        "/healthz",
		Summary:     "Health check",
		Description: "Returns the server health status. Used by load balancers and container orchestration platforms.",
		Tags:        []string{"system"},
	}, func(_ context.Context, _ *struct{}) (*HealthOutput, error) {
		out := &HealthOutput{}
		out.Body.Status = "ok"
		return out, nil
	})

	// OpenAPI spec is served automatically by Huma at:
	//   GET /openapi.json  — OpenAPI 3.1 specification
	//   GET /docs           — Interactive documentation UI
	if s.deps.V1.Auth != nil && s.deps.V1.Tenants != nil {
		s.registerAuthSessionRoutes()
	}

	// -----------------------------------------------------------------------
	// v1 API Registration
	// -----------------------------------------------------------------------

	// Create standard middlewares
	var authMw, tenantMw func(http.Handler) http.Handler
	if s.auth != nil {
		authMw = middleware.Auth(s.auth, s.logger)
		tenantMw = middleware.Tenant(s.logger)
		s.mux.Handle("GET /debug/vars", authMw(expvar.Handler()))
	}

	// Register v1 endpoints. Endpoints are conditionally registered based
	// on the presence of their required repositories in s.deps.V1.
	v1.Register(s.api, s.deps.V1, authMw, tenantMw)
	s.registerMessengerLinkRoutes(authMw, tenantMw)

	// -----------------------------------------------------------------------
	// Slack Webhook Registration (no auth — Slack uses its own signatures)
	// -----------------------------------------------------------------------
	if s.deps.SlackHandler != nil {
		s.mux.Handle("POST /slack/events", s.deps.SlackHandler.HandleEvents())
		s.mux.Handle("POST /slack/interactions", s.deps.SlackHandler.HandleInteractions())
	}
	if s.deps.DiscordWebhook != nil {
		s.mux.Handle("POST /discord/webhook", s.deps.DiscordWebhook)
	}
	if s.deps.TelegramWebhook != nil {
		s.mux.Handle("POST /telegram/webhook", s.deps.TelegramWebhook)
	}

	// -----------------------------------------------------------------------
	// WebSocket Registration
	// -----------------------------------------------------------------------
	if s.deps.Hub != nil {
		if authMw != nil && tenantMw != nil {
			if s.deps.V1.Projects != nil {
				// Wrap handler with auth & tenant middleware
				boardHandler := authMw(tenantMw(ws.HandleBoardStream(s.logger, s.deps.Hub, s.deps.V1.Projects)))
				s.mux.Handle("GET /ws/board/{project_id}", boardHandler)
			}

			if s.deps.V1.AgentSessions != nil {
				// Wrap handler with auth & tenant middleware
				agentHandler := authMw(tenantMw(ws.HandleAgentStream(s.logger, s.deps.Hub, s.deps.V1.AgentSessions)))
				s.mux.Handle("GET /ws/agent/{session_id}", agentHandler)
			}
		}
	}

	// -----------------------------------------------------------------------
	// Dashboard (embedded SPA) — must be registered last as a catch-all.
	// -----------------------------------------------------------------------
	if s.deps.DashboardAssets != nil {
		s.mux.Handle("GET /", newStaticHandler(s.deps.DashboardAssets))
		s.logger.Info("dashboard enabled, serving embedded SPA at /")
	}
}
