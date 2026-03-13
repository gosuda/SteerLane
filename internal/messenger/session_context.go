package messenger

import (
	"sync"

	"github.com/gosuda/steerlane/internal/domain"
)

// SessionContext captures the messenger location tied to a dispatched session.
type SessionContext struct {
	Platform        Platform
	ChannelID       string
	ParentMessageID string
}

// SessionContextRegistry keeps transient messenger session routing state in memory.
type SessionContextRegistry struct {
	items map[sessionContextKey]SessionContext
	mu    sync.RWMutex
}

type sessionContextKey struct {
	tenantID  domain.TenantID
	sessionID domain.AgentSessionID
}

// NewSessionContextRegistry constructs an empty registry.
func NewSessionContextRegistry() *SessionContextRegistry {
	return &SessionContextRegistry{
		items: make(map[sessionContextKey]SessionContext),
	}
}

// Put stores routing context for a session.
func (r *SessionContextRegistry) Put(tenantID domain.TenantID, sessionID domain.AgentSessionID, ctx SessionContext) {
	if r == nil {
		return
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	r.items[sessionContextKey{tenantID: tenantID, sessionID: sessionID}] = ctx
}

// Get returns routing context for a session, if present.
func (r *SessionContextRegistry) Get(tenantID domain.TenantID, sessionID domain.AgentSessionID) (SessionContext, bool) {
	if r == nil {
		return SessionContext{}, false
	}

	r.mu.RLock()
	defer r.mu.RUnlock()

	ctx, ok := r.items[sessionContextKey{tenantID: tenantID, sessionID: sessionID}]
	return ctx, ok
}

// Delete removes routing context for a session.
func (r *SessionContextRegistry) Delete(tenantID domain.TenantID, sessionID domain.AgentSessionID) {
	if r == nil {
		return
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	delete(r.items, sessionContextKey{tenantID: tenantID, sessionID: sessionID})
}
