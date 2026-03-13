package ws

import (
	"context"
	"log/slog"
	"sync"

	"github.com/gosuda/steerlane/internal/store/redis"
)

// Client represents a single websocket connection's send channel.
type Client struct {
	Send chan redis.Event
}

// Hub manages WebSocket clients and fans out Redis pub/sub events to them.
type Hub struct {
	logger       *slog.Logger
	pubsub       *redis.PubSub
	boardClients map[string]map[*Client]struct{}
	boardSubs    map[string]*redis.Subscription
	agentClients map[string]map[*Client]struct{}
	agentSubs    map[string]*redis.Subscription
	mu           sync.RWMutex
}

// NewHub creates a new websocket Hub.
func NewHub(logger *slog.Logger, pubsub *redis.PubSub) *Hub {
	return &Hub{
		logger:       logger,
		pubsub:       pubsub,
		boardClients: make(map[string]map[*Client]struct{}),
		boardSubs:    make(map[string]*redis.Subscription),
		agentClients: make(map[string]map[*Client]struct{}),
		agentSubs:    make(map[string]*redis.Subscription),
	}
}

// SubscribeBoard registers a client for board events of a specific project.
func (h *Hub) SubscribeBoard(projectID string, client *Client) {
	h.mu.Lock()
	defer h.mu.Unlock()

	if h.boardClients[projectID] == nil {
		h.boardClients[projectID] = make(map[*Client]struct{})
		sub := h.pubsub.SubscribeBoard(context.Background(), projectID)
		h.boardSubs[projectID] = sub

		h.logger.Debug("created redis board subscription", "project_id", projectID)
		go h.runBoardFanout(projectID, sub.Channel())
	}
	h.boardClients[projectID][client] = struct{}{}
}

// UnsubscribeBoard unregisters a client from board events.
func (h *Hub) UnsubscribeBoard(projectID string, client *Client) {
	h.mu.Lock()
	defer h.mu.Unlock()

	if clients, ok := h.boardClients[projectID]; ok {
		delete(clients, client)
		if len(clients) == 0 {
			delete(h.boardClients, projectID)
			if sub, ok := h.boardSubs[projectID]; ok { //nolint:govet // short-lived err shadow is idiomatic Go
				_ = sub.Close()
				delete(h.boardSubs, projectID)
				h.logger.Debug("closed redis board subscription", "project_id", projectID)
			}
		}
	}
}

func (h *Hub) runBoardFanout(projectID string, ch <-chan redis.Event) {
	for evt := range ch {
		h.mu.RLock()
		clients := h.boardClients[projectID]
		for c := range clients {
			select {
			case c.Send <- evt:
			default:
				h.logger.Warn("slow board client, dropping event", "project_id", projectID, "event_type", evt.Type)
			}
		}
		h.mu.RUnlock()
	}
}

// SubscribeAgent registers a client for agent events of a specific session.
func (h *Hub) SubscribeAgent(sessionID string, client *Client) {
	h.mu.Lock()
	defer h.mu.Unlock()

	if h.agentClients[sessionID] == nil {
		h.agentClients[sessionID] = make(map[*Client]struct{})
		sub := h.pubsub.SubscribeAgent(context.Background(), sessionID)
		h.agentSubs[sessionID] = sub

		h.logger.Debug("created redis agent subscription", "session_id", sessionID)
		go h.runAgentFanout(sessionID, sub.Channel())
	}
	h.agentClients[sessionID][client] = struct{}{}
}

// UnsubscribeAgent unregisters a client from agent events.
func (h *Hub) UnsubscribeAgent(sessionID string, client *Client) {
	h.mu.Lock()
	defer h.mu.Unlock()

	if clients, ok := h.agentClients[sessionID]; ok {
		delete(clients, client)
		if len(clients) == 0 {
			delete(h.agentClients, sessionID)
			if sub, ok := h.agentSubs[sessionID]; ok { //nolint:govet // short-lived err shadow is idiomatic Go
				_ = sub.Close()
				delete(h.agentSubs, sessionID)
				h.logger.Debug("closed redis agent subscription", "session_id", sessionID)
			}
		}
	}
}

func (h *Hub) runAgentFanout(sessionID string, ch <-chan redis.Event) {
	for evt := range ch {
		h.mu.RLock()
		clients := h.agentClients[sessionID]
		for c := range clients {
			select {
			case c.Send <- evt:
			default:
				h.logger.Warn("slow agent client, dropping event", "session_id", sessionID, "event_type", evt.Type)
			}
		}
		h.mu.RUnlock()
	}
}
