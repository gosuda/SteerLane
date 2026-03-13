package middleware

import (
	"log/slog"
	"net/http"

	"github.com/google/uuid"

	"github.com/gosuda/steerlane/internal/server/reqctx"
)

// Tenant validates that the request has a tenant ID in context (set by the
// auth middleware from JWT claims). Returns 403 Forbidden if the tenant ID
// is missing.
//
// This middleware MUST be applied after the Auth middleware in the chain.
func Tenant(logger *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			tenantID := reqctx.TenantIDFrom(r.Context())
			if tenantID == uuid.Nil {
				logger.WarnContext(r.Context(), "missing tenant ID in request context",
					"method", r.Method,
					"path", r.URL.Path,
					"request_id", reqctx.RequestIDFrom(r.Context()),
				)

				w.Header().Set("Content-Type", "application/problem+json")
				w.WriteHeader(http.StatusForbidden)
				_, _ = w.Write([]byte(`{"status":403,"title":"Forbidden","detail":"tenant context is required"}`))
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}
