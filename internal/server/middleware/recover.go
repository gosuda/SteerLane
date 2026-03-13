package middleware

import (
	"log/slog"
	"net/http"
	"runtime/debug"

	"github.com/gosuda/steerlane/internal/server/reqctx"
)

// Recover catches panics from downstream handlers, logs the stack trace,
// and returns a 500 Internal Server Error response. This middleware MUST be
// the outermost in the chain so it catches panics from all subsequent handlers.
func Recover(logger *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			defer func() {
				if rec := recover(); rec != nil {
					stack := debug.Stack()

					logger.ErrorContext(r.Context(), "panic recovered",
						"error", rec,
						"stack", string(stack),
						"method", r.Method,
						"path", r.URL.Path,
						"request_id", reqctx.RequestIDFrom(r.Context()),
					)

					// Only write the response if headers haven't been sent yet.
					// If they have, we can't change the status code.
					w.Header().Set("Content-Type", "application/problem+json")
					w.WriteHeader(http.StatusInternalServerError)
					_, _ = w.Write([]byte(`{"status":500,"title":"Internal Server Error","detail":"an unexpected error occurred"}`))
				}
			}()

			next.ServeHTTP(w, r)
		})
	}
}
