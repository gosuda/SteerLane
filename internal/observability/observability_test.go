package observability

import (
	"context"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestWithTraceIDAndStartSpan(t *testing.T) {
	t.Parallel()

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	ctx := WithTraceID(context.Background(), "trace-123")
	ctx, finish := StartSpan(ctx, logger, "test.span", map[string]any{"component": "observability"})
	finish(nil, map[string]any{"status": "ok"})

	require.Equal(t, "trace-123", TraceIDFrom(ctx))
}

func TestRecordersDoNotPanic(t *testing.T) {
	t.Parallel()

	RecordHTTPRequest("GET", "/healthz", 200, 25*time.Millisecond)
	RecordOperation("dispatch_task", "success")
	RecordAgentEvent("session.started", "success")
	RecordNotification("task_completed", "success")
}

func TestNormalizePath(t *testing.T) {
	t.Parallel()

	require.Equal(t, "/api/v1/agent-sessions/:id", normalizePath("/api/v1/agent-sessions/550e8400-e29b-41d4-a716-446655440000"))
	require.Equal(t, "/healthz", normalizePath("/healthz"))
}
