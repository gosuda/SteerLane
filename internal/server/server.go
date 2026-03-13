package server

import (
	"io/fs"
	"log/slog"
	"net/http"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/adapters/humago"

	v1 "github.com/gosuda/steerlane/internal/api/v1"
	"github.com/gosuda/steerlane/internal/api/ws"
	"github.com/gosuda/steerlane/internal/auth"
	"github.com/gosuda/steerlane/internal/config"
	"github.com/gosuda/steerlane/internal/messenger/slack"
	"github.com/gosuda/steerlane/internal/server/middleware"
)

// Dependencies holds all services required to wire the server routes.
type Dependencies struct {
	V1              v1.Dependencies
	Hub             *ws.Hub
	SlackHandler    *slack.Handler
	DiscordWebhook  http.Handler
	TelegramWebhook http.Handler
	Linking         *auth.LinkingService
	Links           messengerLinkQueries
	RateLimit       middleware.RateLimitStore

	// DashboardAssets is the embedded SPA filesystem (rooted at build output).
	// When non-nil the server serves the dashboard at / with SPA fallback.
	// When nil the dashboard is not served (e.g. during tests).
	DashboardAssets fs.FS
}

// Server holds the HTTP multiplexer, Huma API, and all dependencies
// required for request handling. It wires middleware and routes together.
type Server struct {
	deps   Dependencies
	api    huma.API
	auth   middleware.Authenticator
	mux    *http.ServeMux
	logger *slog.Logger
	cfg    config.Config
}

// New creates a Server with its multiplexer and Huma API configured.
// The returned Server is not yet listening — call Handler() to get the
// http.Handler with the full middleware chain applied.
func New(cfg config.Config, logger *slog.Logger, authSvc middleware.Authenticator, deps Dependencies) *Server {
	mux := http.NewServeMux()

	humaConfig := huma.DefaultConfig("SteerLane API", "1.0.0")
	api := humago.New(mux, humaConfig)

	s := &Server{
		mux:    mux,
		api:    api,
		cfg:    cfg,
		logger: logger.With("component", "server"),
		auth:   authSvc,
		deps:   deps,
	}

	s.RegisterRoutes()

	return s
}

// Handler returns the root http.Handler with the global middleware chain applied.
// The chain order (outermost to innermost):
//
//	request_id -> logging -> recover -> cors -> rate_limit -> mux (routes)
//
// Auth and tenant middleware are applied per-route group in RegisterRoutes,
// not globally, so unauthenticated routes (healthz, OpenAPI, auth endpoints)
// remain accessible.
func (s *Server) Handler() http.Handler {
	var handler http.Handler = s.mux

	// Apply middleware inside-out: last applied = outermost.
	if s.cfg.RateLimit.Enabled && s.deps.RateLimit == nil {
		s.logger.Warn("rate limiting enabled but no Redis rate-limit store configured; middleware disabled")
	}
	handler = middleware.RateLimit(middleware.RateLimitConfig{
		Enabled:           s.cfg.RateLimit.Enabled,
		RequestsPerMinute: s.cfg.RateLimit.RequestsPerMinute,
		TrustedProxyCIDRs: s.cfg.RateLimit.TrustedProxies,
		Store:             s.deps.RateLimit,
		Authenticator:     s.auth,
		Logger:            s.logger,
	})(handler)
	handler = middleware.CORS(middleware.CORSConfig{
		AllowedOrigins: s.cfg.CORS.Origins,
		MaxAge:         s.cfg.CORS.MaxAge,
	})(handler)
	handler = middleware.Recover(s.logger)(handler)
	handler = middleware.Logging(s.logger)(handler)
	handler = middleware.RequestID(handler)

	return handler
}

// API returns the Huma API instance for external registration of operations.
func (s *Server) API() huma.API {
	return s.api
}

// Mux returns the underlying http.ServeMux for raw handler registration.
func (s *Server) Mux() *http.ServeMux {
	return s.mux
}
