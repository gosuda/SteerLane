package slack

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
)

// maxRequestBodySize limits the size of incoming Slack payloads.
// Slack's documented maximum is ~64KB for events, but interactive payloads
// can be larger when modals with many fields are submitted.
const maxRequestBodySize = 256 * 1024 // 256 KiB

// EventHandler dispatches processed Slack events to application logic.
// Implemented by Service in service.go.
type EventHandler interface {
	// HandleAppMention is called when the bot is mentioned in a channel.
	HandleAppMention(event InnerEvent) error

	// HandleMessage is called for message events (new messages, edits).
	HandleMessage(event InnerEvent) error
}

// InteractionHandler dispatches parsed Slack interactive payloads to
// application logic. Multiple handlers can be registered; the Handler
// dispatches to each in sequence until one handles the payload.
type InteractionHandler interface {
	// HandleInteraction processes a parsed Slack interactive payload.
	// Returns nil if the payload was handled or not relevant to this handler.
	HandleInteraction(ctx context.Context, payload InteractionPayload) error
}

// Handler serves Slack webhook endpoints (Events API and interactive components).
type Handler struct {
	logger   *slog.Logger
	verifier SignatureVerifier

	// eventHandler dispatches parsed events. Nil means events are acknowledged
	// but not processed (skeleton mode).
	eventHandler EventHandler

	// interactionHandlers dispatch parsed interactive payloads. They are
	// called in order; each decides whether the payload is relevant.
	interactionHandlers []InteractionHandler
}

// NewHandler creates a Slack webhook handler.
//
// Parameters:
//   - logger: structured logger for request lifecycle events
//   - verifier: validates Slack request signatures (use NewNoopVerifier for dev)
//   - eventHandler: optional event dispatcher (nil = acknowledge-only skeleton)
//   - interactionHandlers: optional handlers for interactive payloads (buttons, menus)
func NewHandler(
	logger *slog.Logger,
	verifier SignatureVerifier,
	eventHandler EventHandler,
	interactionHandlers ...InteractionHandler,
) *Handler {
	return &Handler{
		logger:              logger.With("component", "slack.handler"),
		verifier:            verifier,
		eventHandler:        eventHandler,
		interactionHandlers: interactionHandlers,
	}
}

// HandleEvents returns an http.HandlerFunc for POST /slack/events.
//
// It handles three Slack envelope types:
//   - url_verification: responds with the challenge token (required for Slack app setup)
//   - event_callback: dispatches to the appropriate event handler
//   - app_rate_limited: logs and acknowledges
func (h *Handler) HandleEvents() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}

		body, verifyErr := h.readAndVerify(r)
		if verifyErr != nil {
			h.logger.WarnContext(r.Context(), "slack events: verification failed",
				slog.String("error", verifyErr.Error()),
				slog.String("remote_addr", r.RemoteAddr),
			)
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}

		var req EventRequest
		if unmarshalErr := json.Unmarshal(body, &req); unmarshalErr != nil {
			h.logger.WarnContext(r.Context(), "slack events: invalid JSON",
				slog.String("error", unmarshalErr.Error()),
			)
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}

		switch req.Type {
		case "url_verification":
			h.handleURLVerification(w, r, req)

		case "event_callback":
			h.handleEventCallback(w, r, req)

		case "app_rate_limited":
			h.logger.WarnContext(r.Context(), "slack events: rate limited by Slack",
				slog.String("team_id", req.TeamID),
			)
			w.WriteHeader(http.StatusOK)

		default:
			h.logger.WarnContext(r.Context(), "slack events: unknown envelope type",
				slog.String("type", req.Type),
			)
			w.WriteHeader(http.StatusOK)
		}
	}
}

// HandleInteractions returns an http.HandlerFunc for POST /slack/interactions.
//
// Slack sends interactive component payloads (button clicks, modal submissions)
// as application/x-www-form-urlencoded with a single "payload" field containing JSON.
//
// Phase 1C.1 provides the parsing skeleton. Actual interaction routing
// (HITL answers, ADR approvals) will be added in Phase 1C.2 (SteerLane-7em).
func (h *Handler) HandleInteractions() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}

		body, verifyErr := h.readAndVerify(r)
		if verifyErr != nil {
			h.logger.WarnContext(r.Context(), "slack interactions: verification failed",
				slog.String("error", verifyErr.Error()),
				slog.String("remote_addr", r.RemoteAddr),
			)
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}

		// Slack sends the payload as form-encoded: payload=<url-encoded JSON>
		values, parseErr := url.ParseQuery(string(body))
		if parseErr != nil {
			h.logger.WarnContext(r.Context(), "slack interactions: invalid form data",
				slog.String("error", parseErr.Error()),
			)
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}

		payloadStr := values.Get("payload")
		if payloadStr == "" {
			h.logger.WarnContext(r.Context(), "slack interactions: missing payload field")
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}

		var payload InteractionPayload
		if unmarshalErr := json.Unmarshal([]byte(payloadStr), &payload); unmarshalErr != nil {
			h.logger.WarnContext(r.Context(), "slack interactions: invalid payload JSON",
				slog.String("error", unmarshalErr.Error()),
			)
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}

		h.logger.InfoContext(r.Context(), "slack interactions: received",
			slog.String("type", payload.Type),
			slog.String("user_id", payload.User.ID),
			slog.String("team_id", payload.Team.ID),
		)

		// Acknowledge receipt immediately. Slack requires a 200 within 3 seconds.
		// Interaction handlers run synchronously after the ack; if they become
		// slow, move to async dispatch.
		w.WriteHeader(http.StatusOK)

		// Dispatch to registered interaction handlers.
		h.dispatchInteraction(r.Context(), payload)
	}
}

// handleURLVerification responds to Slack's initial endpoint verification challenge.
func (h *Handler) handleURLVerification(w http.ResponseWriter, r *http.Request, req EventRequest) {
	h.logger.InfoContext(r.Context(), "slack events: URL verification challenge received")

	w.Header().Set("Content-Type", "application/json")
	resp := ChallengeResponse{Challenge: req.Challenge}
	if err := json.NewEncoder(w).Encode(resp); err != nil {
		h.logger.ErrorContext(r.Context(), "slack events: failed to write challenge response",
			slog.String("error", err.Error()),
		)
	}
}

// handleEventCallback dispatches an event_callback envelope to the event handler.
func (h *Handler) handleEventCallback(w http.ResponseWriter, r *http.Request, req EventRequest) {
	var inner InnerEvent
	if err := json.Unmarshal(req.Event, &inner); err != nil {
		h.logger.WarnContext(r.Context(), "slack events: failed to parse inner event",
			slog.String("error", err.Error()),
			slog.String("event_id", req.EventID),
		)
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	inner.TeamID = req.TeamID

	h.logger.InfoContext(r.Context(), "slack events: dispatching",
		slog.String("event_type", inner.Type),
		slog.String("event_id", req.EventID),
		slog.String("team_id", req.TeamID),
		slog.String("channel", inner.Channel),
		slog.String("user", inner.User),
	)

	// Acknowledge immediately — Slack requires a 200 within 3 seconds.
	// Event processing happens synchronously for now; if handlers become
	// slow, move to async dispatch with a work queue.
	w.WriteHeader(http.StatusOK)

	if h.eventHandler == nil {
		// Skeleton mode: acknowledge but do not process.
		return
	}

	// Dispatch based on event type.
	var dispatchErr error
	switch inner.Type {
	case "app_mention":
		dispatchErr = h.eventHandler.HandleAppMention(inner)
	case "message":
		// Ignore bot messages to prevent loops.
		if inner.SubType == "bot_message" || inner.SubType == "message_changed" {
			return
		}
		dispatchErr = h.eventHandler.HandleMessage(inner)
	default:
		h.logger.DebugContext(r.Context(), "slack events: unhandled event type",
			slog.String("event_type", inner.Type),
		)
	}

	if dispatchErr != nil {
		h.logger.ErrorContext(r.Context(), "slack events: handler error",
			slog.String("event_type", inner.Type),
			slog.String("event_id", req.EventID),
			slog.String("error", dispatchErr.Error()),
		)
	}
}

// dispatchInteraction routes a parsed interactive payload to all registered
// interaction handlers. Errors are logged but not propagated because the
// HTTP response has already been sent (Slack's 3s ack requirement).
func (h *Handler) dispatchInteraction(ctx context.Context, payload InteractionPayload) {
	for _, handler := range h.interactionHandlers {
		if err := handler.HandleInteraction(ctx, payload); err != nil {
			h.logger.ErrorContext(ctx, "slack interactions: handler error",
				slog.String("error", err.Error()),
				slog.String("type", payload.Type),
				slog.String("user_id", payload.User.ID),
			)
		}
	}
}

// readAndVerify reads the request body (with size limit) and verifies the
// Slack signature. Returns the raw body bytes on success.
func (h *Handler) readAndVerify(r *http.Request) ([]byte, error) {
	body, readErr := io.ReadAll(io.LimitReader(r.Body, maxRequestBodySize))
	if readErr != nil {
		return nil, fmt.Errorf("slack: read body: %w", readErr)
	}

	if verifyErr := h.verifier.Verify(r.Header, body); verifyErr != nil {
		return nil, fmt.Errorf("slack: %w", verifyErr)
	}

	return body, nil
}
