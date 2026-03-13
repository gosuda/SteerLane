// Package middleware provides HTTP middleware for the SteerLane server.
package middleware

import (
	"net/http"

	"github.com/google/uuid"

	"github.com/gosuda/steerlane/internal/server/reqctx"
)

const headerXRequestID = "X-Request-ID"

// RequestID generates a UUID v4 request ID for each request,
// stores it in the request context, and sets the X-Request-ID response header.
// If the incoming request already has an X-Request-ID header, it is reused.
func RequestID(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := r.Header.Get(headerXRequestID)
		if id == "" {
			id = uuid.New().String()
		}

		ctx := reqctx.WithRequestID(r.Context(), id)
		w.Header().Set(headerXRequestID, id)

		next.ServeHTTP(w, r.WithContext(ctx))
	})
}
