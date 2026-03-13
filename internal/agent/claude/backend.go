package claude

import (
	"context"
	"errors"
	"log/slog"
	"sync"

	"github.com/gosuda/steerlane/internal/agent"
	domainagent "github.com/gosuda/steerlane/internal/domain/agent"
)

// Compile-time interface satisfaction check.
var _ agent.Backend = (*Backend)(nil)

const (
	// DefaultImage is the Docker image for Claude agent execution.
	DefaultImage = "steerlane/claude-agent:latest"
)

// Backend implements agent.Backend for the Claude coding agent.
// It manages a Claude CLI process inside a Docker container.
type Backend struct {
	logger  *slog.Logger
	handler agent.MessageHandler
	done    chan struct{}
	mu      sync.Mutex
	active  bool
	started bool
}

// NewBackend creates a new Claude backend instance.
func NewBackend(logger *slog.Logger) (*Backend, error) {
	if logger == nil {
		logger = slog.Default()
	}
	return &Backend{
		logger: logger.With("agent_type", "claude"),
		done:   make(chan struct{}),
	}, nil
}

// Factory returns an agent.Factory for Claude backends.
func Factory() agent.Factory {
	return func(logger *slog.Logger) (agent.Backend, error) {
		return NewBackend(logger)
	}
}

// OnMessage registers a handler for agent output messages.
func (b *Backend) OnMessage(handler agent.MessageHandler) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.handler = handler
}

// StartSession begins Claude agent execution.
// In Phase 1B, the actual execution is delegated to the Docker runtime;
// this backend is responsible for protocol adaptation and message routing.
func (b *Backend) StartSession(ctx context.Context, opts agent.SessionOpts) error {
	b.mu.Lock()
	if b.started {
		b.mu.Unlock()
		return errors.New("claude: session already started")
	}
	b.started = true
	b.active = true
	b.mu.Unlock()

	b.logger.InfoContext(ctx, "claude session starting",
		"session_id", opts.SessionID,
		"task_id", opts.TaskID,
		"branch", opts.BranchName,
	)

	// Phase 1B: The orchestrator manages the Docker container lifecycle.
	// This backend adapts the Claude-specific protocol (tool calls, output format).
	// The actual container process is started by the orchestrator using the Docker runtime.
	return nil
}

// SendPrompt sends a follow-up prompt to the running Claude agent.
func (b *Backend) SendPrompt(ctx context.Context, prompt string) error {
	b.mu.Lock()
	active := b.active
	started := b.started
	b.mu.Unlock()

	if !started {
		return errors.New("claude: session not started")
	}
	if !active {
		return errors.New("claude: session not active")
	}

	b.logger.InfoContext(ctx, "sending prompt to claude agent", "prompt_len", len(prompt))
	// Phase 1B: Write prompt to the agent's stdin or API endpoint.
	return nil
}

// Cancel gracefully stops the Claude agent.
func (b *Backend) Cancel(ctx context.Context) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	if !b.started || !b.active {
		return nil
	}
	b.active = false
	b.logger.InfoContext(ctx, "cancelling claude session")
	// Phase 1B: Signal the agent process to stop.
	return nil
}

// Dispose releases all resources.
func (b *Backend) Dispose() error {
	b.mu.Lock()
	defer b.mu.Unlock()

	select {
	case <-b.done:
		// already closed
	default:
		close(b.done)
	}
	b.active = false
	return nil
}

// RegisterDefault registers the Claude backend factory in the given registry.
func RegisterDefault(registry *agent.Registry) {
	registry.Register(domainagent.TypeClaude, Factory())
}
