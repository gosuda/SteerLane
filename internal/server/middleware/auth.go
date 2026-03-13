package middleware

import (
	"context"
	"log/slog"
	"net/http"
	"strings"

	"github.com/google/uuid"

	"github.com/gosuda/steerlane/internal/server/reqctx"
)

// Identity represents the authenticated user's identity extracted from
// a JWT or API key. This is the data contract between the Authenticator
// and the middleware — it avoids coupling to the concrete auth.Claims type.
type Identity struct {
	Role     string
	UserID   uuid.UUID
	TenantID uuid.UUID
}

// Authenticator validates tokens and API keys. Implemented by an adapter
// over auth.Service. The interface decouples middleware from the concrete
// auth package for testability.
type Authenticator interface {
	// AuthenticateJWT validates a Bearer token and returns the identity.
	AuthenticateJWT(token string) (*Identity, error)
	// AuthenticateAPIKey validates an API key and returns the identity.
	AuthenticateAPIKey(ctx context.Context, rawKey string) (*Identity, error)
}

const (
	headerAuthorization = "Authorization"
	headerAPIKey        = "X-API-Key" //nolint:gosec // G101: this is a header name constant, not a credential
	bearerPrefix        = "Bearer "
	cookieAccessToken   = "steerlane_access_token"
)

// Auth extracts Bearer token or X-API-Key from the request, validates it
// using the provided Authenticator, and stores the user identity in context.
// Returns 401 Unauthorized if no valid credentials are present.
func Auth(authenticator Authenticator, logger *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			var (
				identity *Identity
				err      error
			)

			// Try Bearer token first, then API key.
			if authHeader := r.Header.Get(headerAuthorization); strings.HasPrefix(authHeader, bearerPrefix) {
				token := strings.TrimPrefix(authHeader, bearerPrefix)
				identity, err = authenticator.AuthenticateJWT(token)
			} else if apiKey := r.Header.Get(headerAPIKey); apiKey != "" {
				identity, err = authenticator.AuthenticateAPIKey(r.Context(), apiKey)
			} else if accessCookie, cookieErr := r.Cookie(cookieAccessToken); cookieErr == nil && strings.TrimSpace(accessCookie.Value) != "" {
				identity, err = authenticator.AuthenticateJWT(accessCookie.Value)
			} else {
				writeUnauthorized(w, "missing authentication credentials")
				return
			}

			if err != nil {
				logger.WarnContext(r.Context(), "authentication failed",
					"error", err,
					"method", r.Method,
					"path", r.URL.Path,
					"request_id", reqctx.RequestIDFrom(r.Context()),
				)
				writeUnauthorized(w, "invalid or expired credentials")
				return
			}

			ctx := reqctx.WithUser(r.Context(), identity.UserID, identity.Role)

			if identity.TenantID != uuid.Nil {
				ctx = reqctx.WithTenant(ctx, identity.TenantID)
			}

			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// writeUnauthorized writes a 401 response with a problem+json body.
func writeUnauthorized(w http.ResponseWriter, detail string) {
	w.Header().Set("Content-Type", "application/problem+json")
	w.WriteHeader(http.StatusUnauthorized)
	_, _ = w.Write([]byte(`{"status":401,"title":"Unauthorized","detail":"` + detail + `"}`))
}
