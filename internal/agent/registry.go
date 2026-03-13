package agent

import (
	"fmt"
	"log/slog"
	"sync"

	domainagent "github.com/gosuda/steerlane/internal/domain/agent"
)

// Factory creates a new Backend instance for a single session.
// Each call returns a fresh Backend that must be Disposed after use.
type Factory func(logger *slog.Logger) (Backend, error)

// Registry maps agent types to their backend factories.
type Registry struct {
	factories map[domainagent.AgentType]Factory
	mu        sync.RWMutex
}

// NewRegistry creates an empty registry.
func NewRegistry() *Registry {
	return &Registry{
		factories: make(map[domainagent.AgentType]Factory),
	}
}

// Register adds a backend factory for the given agent type.
// Panics if a factory is already registered for that type.
func (r *Registry) Register(agentType domainagent.AgentType, factory Factory) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, exists := r.factories[agentType]; exists {
		panic(fmt.Sprintf("agent: factory already registered for type %q", agentType))
	}
	r.factories[agentType] = factory
}

// Create instantiates a new Backend for the given agent type.
// Returns an error if no factory is registered for that type.
func (r *Registry) Create(agentType domainagent.AgentType, logger *slog.Logger) (Backend, error) {
	r.mu.RLock()
	factory, ok := r.factories[agentType]
	r.mu.RUnlock()
	if !ok {
		return nil, fmt.Errorf("agent: no backend registered for type %q", agentType)
	}
	return factory(logger)
}

// Types returns a list of registered agent types.
func (r *Registry) Types() []domainagent.AgentType {
	r.mu.RLock()
	defer r.mu.RUnlock()
	types := make([]domainagent.AgentType, 0, len(r.factories))
	for t := range r.factories {
		types = append(types, t)
	}
	return types
}

// Has checks whether a factory is registered for the given type.
func (r *Registry) Has(agentType domainagent.AgentType) bool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	_, ok := r.factories[agentType]
	return ok
}
