package gemini

import (
	"context"
	"errors"
	"log/slog"
	"sync"

	"github.com/gosuda/steerlane/internal/agent"
	domainagent "github.com/gosuda/steerlane/internal/domain/agent"
)

var _ agent.Backend = (*Backend)(nil)

const DefaultImage = "steerlane/gemini-agent:latest"

type Backend struct {
	logger  *slog.Logger
	handler agent.MessageHandler
	done    chan struct{}
	mu      sync.Mutex
	active  bool
	started bool
}

func NewBackend(logger *slog.Logger) (*Backend, error) {
	if logger == nil {
		logger = slog.Default()
	}
	return &Backend{
		logger: logger.With("agent_type", "gemini"),
		done:   make(chan struct{}),
	}, nil
}

func Factory() agent.Factory {
	return func(logger *slog.Logger) (agent.Backend, error) {
		return NewBackend(logger)
	}
}

func (b *Backend) OnMessage(handler agent.MessageHandler) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.handler = handler
}

func (b *Backend) StartSession(ctx context.Context, opts agent.SessionOpts) error {
	b.mu.Lock()
	if b.started {
		b.mu.Unlock()
		return errors.New("gemini: session already started")
	}
	b.started = true
	b.active = true
	b.mu.Unlock()

	b.logger.InfoContext(ctx, "gemini session starting",
		"session_id", opts.SessionID,
		"task_id", opts.TaskID,
		"branch", opts.BranchName,
	)

	return nil
}

func (b *Backend) SendPrompt(ctx context.Context, prompt string) error {
	b.mu.Lock()
	active := b.active
	started := b.started
	b.mu.Unlock()

	if !started {
		return errors.New("gemini: session not started")
	}
	if !active {
		return errors.New("gemini: session not active")
	}

	b.logger.InfoContext(ctx, "sending prompt to gemini agent", "prompt_len", len(prompt))
	return nil
}

func (b *Backend) Cancel(ctx context.Context) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	if !b.started || !b.active {
		return nil
	}
	b.active = false
	b.logger.InfoContext(ctx, "cancelling gemini session")
	return nil
}

func (b *Backend) Dispose() error {
	b.mu.Lock()
	defer b.mu.Unlock()

	select {
	case <-b.done:
	default:
		close(b.done)
	}
	b.active = false
	return nil
}

func RegisterDefault(registry *agent.Registry) {
	registry.Register(domainagent.TypeGemini, Factory())
}
