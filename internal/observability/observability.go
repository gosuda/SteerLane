package observability

import (
	"context"
	"expvar"
	"fmt"
	"log/slog"
	"maps"
	"strings"
	"time"

	"github.com/google/uuid"
)

type contextKey string

const (
	contextKeyTraceID contextKey = "observability.trace_id"
	contextKeySpanID  contextKey = "observability.span_id"
)

var (
	//nolint:gochecknoglobals // expvar metrics are process-wide by design.
	httpRequestsTotal = expvar.NewMap("http_requests_total")
	//nolint:gochecknoglobals // expvar metrics are process-wide by design.
	httpRequestMillisTotal = expvar.NewMap("http_request_duration_ms_total")
	//nolint:gochecknoglobals // expvar metrics are process-wide by design.
	operationTotal = expvar.NewMap("operation_total")
	//nolint:gochecknoglobals // expvar metrics are process-wide by design.
	agentEventsTotal = expvar.NewMap("agent_events_total")
	//nolint:gochecknoglobals // expvar metrics are process-wide by design.
	notificationTotal = expvar.NewMap("notification_total")
)

// WithTraceID stores a trace identifier in context for downstream spans.
func WithTraceID(ctx context.Context, traceID string) context.Context {
	if traceID == "" {
		return ctx
	}
	return context.WithValue(ctx, contextKeyTraceID, traceID)
}

// TraceIDFrom returns the current trace identifier, if any.
func TraceIDFrom(ctx context.Context) string {
	if ctx == nil {
		return ""
	}
	traceID, _ := ctx.Value(contextKeyTraceID).(string)
	return traceID
}

// StartSpan creates a lightweight structured-log span and returns a child context.
func StartSpan(ctx context.Context, logger *slog.Logger, name string, attrs map[string]any) (spanCtx context.Context, finish func(error, map[string]any)) {
	if logger == nil {
		logger = slog.Default()
	}

	traceID := TraceIDFrom(ctx)
	if traceID == "" {
		traceID = uuid.NewString()
		ctx = WithTraceID(ctx, traceID)
	}
	spanID := uuid.NewString()
	ctx = context.WithValue(ctx, contextKeySpanID, spanID)
	start := time.Now()

	logger.InfoContext(ctx, name+".start", attrsToArgs(traceID, spanID, attrs)...)

	return ctx, func(err error, attrs map[string]any) {
		merged := cloneAttrs(attrs)
		merged["duration_ms"] = time.Since(start).Milliseconds()
		if err != nil {
			merged["error"] = err.Error()
			logger.ErrorContext(ctx, name+".finish", attrsToArgs(traceID, spanID, merged)...)
			return
		}
		logger.InfoContext(ctx, name+".finish", attrsToArgs(traceID, spanID, merged)...)
	}
}

// RecordHTTPRequest records request count and cumulative latency.
func RecordHTTPRequest(method, path string, status int, duration time.Duration) {
	normalizedPath := normalizePath(path)
	httpRequestsTotal.Add(fmt.Sprintf("%s %s %d", method, normalizedPath, status), 1)
	httpRequestMillisTotal.Add(fmt.Sprintf("%s %s", method, normalizedPath), duration.Milliseconds())
}

// RecordOperation records a named operation outcome.
func RecordOperation(name, outcome string) {
	operationTotal.Add(name+":"+outcome, 1)
}

// RecordAgentEvent records an agent event publication attempt.
func RecordAgentEvent(eventType, outcome string) {
	agentEventsTotal.Add(eventType+":"+outcome, 1)
}

// RecordNotification records notification delivery outcome by kind.
func RecordNotification(kind, outcome string) {
	notificationTotal.Add(kind+":"+outcome, 1)
}

func attrsToArgs(traceID, spanID string, attrs map[string]any) []any {
	merged := cloneAttrs(attrs)
	merged["trace_id"] = traceID
	merged["span_id"] = spanID
	args := make([]any, 0, len(merged)*2)
	for key, value := range merged {
		args = append(args, key, value)
	}
	return args
}

func cloneAttrs(attrs map[string]any) map[string]any {
	if len(attrs) == 0 {
		return map[string]any{}
	}
	clone := make(map[string]any, len(attrs))
	maps.Copy(clone, attrs)
	return clone
}

func normalizePath(path string) string {
	trimmed := strings.TrimSpace(path)
	if trimmed == "" || trimmed == "/" {
		return "/"
	}

	parts := strings.Split(trimmed, "/")
	for idx, part := range parts {
		if part == "" {
			continue
		}
		if _, err := uuid.Parse(part); err == nil {
			parts[idx] = ":id"
		}
	}

	return strings.Join(parts, "/")
}
