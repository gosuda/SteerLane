package ws

import (
	"context"
	"log/slog"
	"net/http"

	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"
	"github.com/google/uuid"

	"github.com/gosuda/steerlane/internal/domain/agent"
	"github.com/gosuda/steerlane/internal/server/reqctx"
	"github.com/gosuda/steerlane/internal/store/redis"
)

// HandleAgentStream upgrades the connection and streams agent events.
// Call site: mux.Handle("/ws/agent/{session_id}", ws.HandleAgentStream(logger, hub))
//
// TODO(Phase 1B): Agent event upstream producer work will be fully integrated
// in Phase 1B. This stream scaffold correctly handles real-time subscriptions,
// but expects backend workers to start publishing to the redis agent stream.
func HandleAgentStream(logger *slog.Logger, hub *Hub, sessions agent.Repository) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		reqCtx := r.Context()

		// PRE-1: request must already be authenticated before upgrade
		if reqctx.UserIDFrom(reqCtx) == uuid.Nil {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}

		sessionIDStr := r.PathValue("session_id")

		// PRE-2: stream identifiers must be valid UUIDs
		sessID, err := uuid.Parse(sessionIDStr)
		if err != nil {
			http.Error(w, "invalid session ID format", http.StatusBadRequest)
			return
		}

		tenantID := reqctx.TenantIDFrom(reqCtx)
		if tenantID == uuid.Nil {
			http.Error(w, "unauthorized tenant", http.StatusUnauthorized)
			return
		}

		// PRE-2: verify caller belongs to the same tenant as requested session
		if _, err := sessions.GetByID(reqCtx, tenantID, sessID); err != nil { //nolint:govet // short-lived err shadow is idiomatic Go
			http.Error(w, "session not found or unauthorized", http.StatusNotFound)
			return
		}

		c, err := websocket.Accept(w, r, &websocket.AcceptOptions{
			InsecureSkipVerify: true,
		})
		if err != nil {
			logger.ErrorContext(reqCtx, "websocket accept failed", "error", err)
			return
		}
		defer c.CloseNow()

		client := &Client{
			Send: make(chan redis.Event, 64),
		}

		// POST-1: subscribe registers connection exactly once
		hub.SubscribeAgent(sessionIDStr, client)
		// POST-2: disconnect removes connection
		defer hub.UnsubscribeAgent(sessionIDStr, client)

		ctx, cancel := context.WithCancel(reqCtx)
		defer cancel()

		// Reader goroutine for control frames and disconnect detection
		go func() {
			defer cancel()
			for {
				_, _, err := c.Read(ctx) //nolint:govet // short-lived err shadow is idiomatic Go
				if err != nil {
					break
				}
			}
		}()

		// Write loop
		for {
			select {
			case <-ctx.Done():
				_ = c.Close(websocket.StatusNormalClosure, "")
				return
			case evt := <-client.Send:
				if err := wsjson.Write(ctx, c, evt); err != nil { //nolint:govet // short-lived err shadow is idiomatic Go
					logger.ErrorContext(ctx, "failed to write agent event", "error", err)
					_ = c.Close(websocket.StatusInternalError, "write failed")
					return
				}
			}
		}
	}
}
