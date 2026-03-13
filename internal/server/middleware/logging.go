package middleware

import (
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/google/uuid"

	"github.com/gosuda/steerlane/internal/observability"
	"github.com/gosuda/steerlane/internal/server/reqctx"
)

// statusRecorder wraps http.ResponseWriter to capture the status code.
type statusRecorder struct {
	http.ResponseWriter
	status      int
	wroteHeader bool
}

func (sr *statusRecorder) WriteHeader(code int) {
	if sr.wroteHeader {
		return
	}
	sr.status = code
	sr.wroteHeader = true
	sr.ResponseWriter.WriteHeader(code)
}

func (sr *statusRecorder) Write(b []byte) (int, error) {
	if !sr.wroteHeader {
		sr.WriteHeader(http.StatusOK)
	}
	return sr.ResponseWriter.Write(b)
}

// Unwrap returns the underlying ResponseWriter for middleware that needs
// access to the original writer (e.g., http.Flusher).
func (sr *statusRecorder) Unwrap() http.ResponseWriter {
	return sr.ResponseWriter
}

// Logging logs each request with method, path, status, duration,
// and the request ID from context using structured slog.
func Logging(logger *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			traceID := reqctx.RequestIDFrom(r.Context())
			if traceID == "" {
				traceID = uuid.NewString()
			}
			ctx := observability.WithTraceID(r.Context(), traceID)
			ctx, finishSpan := observability.StartSpan(ctx, logger, "http.request", map[string]any{
				"method": r.Method,
				"path":   r.URL.Path,
			})
			r = r.WithContext(ctx)

			rec := &statusRecorder{
				ResponseWriter: w,
				status:         http.StatusOK,
			}

			next.ServeHTTP(rec, r)
			duration := time.Since(start)
			observability.RecordHTTPRequest(r.Method, r.URL.Path, rec.status, duration)
			var spanErr error
			if rec.status >= http.StatusInternalServerError {
				spanErr = fmt.Errorf("http status %d", rec.status)
			}
			finishSpan(spanErr, map[string]any{
				"status":      rec.status,
				"duration_ms": duration.Milliseconds(),
			})

			logger.LogAttrs(r.Context(), levelForStatus(rec.status), "http request",
				slog.String("method", r.Method),
				slog.String("path", r.URL.Path),
				slog.Int("status", rec.status),
				slog.Duration("duration", duration),
				slog.String("request_id", reqctx.RequestIDFrom(r.Context())),
				slog.String("trace_id", observability.TraceIDFrom(r.Context())),
				slog.String("remote_addr", r.RemoteAddr),
			)
		})
	}
}

// levelForStatus returns the appropriate slog level based on HTTP status code.
func levelForStatus(status int) slog.Level {
	switch {
	case status >= 500:
		return slog.LevelError
	case status >= 400:
		return slog.LevelWarn
	default:
		return slog.LevelInfo
	}
}
