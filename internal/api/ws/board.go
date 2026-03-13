package ws

import (
	"context"
	"log/slog"
	"net/http"

	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"
	"github.com/google/uuid"

	"github.com/gosuda/steerlane/internal/domain/project"
	"github.com/gosuda/steerlane/internal/server/reqctx"
	"github.com/gosuda/steerlane/internal/store/redis"
)

// HandleBoardStream upgrades the connection and streams board events.
// Call site: mux.Handle("/ws/board/{project_id}", ws.HandleBoardStream(logger, hub)).
func HandleBoardStream(logger *slog.Logger, hub *Hub, projects project.Repository) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		reqCtx := r.Context()

		// PRE-1: request must already be authenticated before upgrade
		if reqctx.UserIDFrom(reqCtx) == uuid.Nil {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}

		projectIDStr := r.PathValue("project_id")

		// PRE-2: stream identifiers must be valid UUIDs
		projID, err := uuid.Parse(projectIDStr)
		if err != nil {
			http.Error(w, "invalid project ID format", http.StatusBadRequest)
			return
		}

		tenantID := reqctx.TenantIDFrom(reqCtx)
		if tenantID == uuid.Nil {
			http.Error(w, "unauthorized tenant", http.StatusUnauthorized)
			return
		}

		// PRE-2: verify caller belongs to the same tenant as requested project
		if _, err := projects.GetByID(reqCtx, tenantID, projID); err != nil { //nolint:govet // short-lived err shadow is idiomatic Go
			http.Error(w, "project not found or unauthorized", http.StatusNotFound)
			return
		}

		c, err := websocket.Accept(w, r, &websocket.AcceptOptions{
			// Allow cross-origin for local development if needed,
			// though typical production would use a proper Origin check.
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
		hub.SubscribeBoard(projectIDStr, client)
		// POST-2: disconnect removes connection
		defer hub.UnsubscribeBoard(projectIDStr, client)

		ctx, cancel := context.WithCancel(reqCtx)
		defer cancel()

		// websocket requires a concurrent reader to process control
		// frames (ping/pong) and properly detect client disconnection.
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
					logger.ErrorContext(ctx, "failed to write websocket event", "error", err)
					_ = c.Close(websocket.StatusInternalError, "write failed")
					return
				}
			}
		}
	}
}
