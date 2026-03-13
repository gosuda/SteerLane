package server

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/gosuda/steerlane/internal/server/reqctx"
	"github.com/gosuda/steerlane/internal/store/postgres/sqlc"
)

type completeMessengerLinkRequest struct {
	Token string `json:"token"`
}

type messengerLinkQueries interface {
	CreateMessengerLink(ctx context.Context, arg sqlc.CreateMessengerLinkParams) (sqlc.UserMessengerLink, error)
	DeleteMessengerLinkByID(ctx context.Context, arg sqlc.DeleteMessengerLinkByIDParams) (sqlc.UserMessengerLink, error)
	GetMessengerLink(ctx context.Context, arg sqlc.GetMessengerLinkParams) (sqlc.UserMessengerLink, error)
	ListMessengerLinksByUser(ctx context.Context, arg sqlc.ListMessengerLinksByUserParams) ([]sqlc.UserMessengerLink, error)
}

type messengerLinkResponse struct {
	Platform   string `json:"platform"`
	ExternalID string `json:"external_id"`
	CreatedAt  string `json:"created_at"`
	Status     string `json:"status,omitempty"`
	Message    string `json:"message,omitempty"`
	ID         string `json:"id"`
}

func (s *Server) registerMessengerLinkRoutes(authMw, tenantMw func(http.Handler) http.Handler) {
	if authMw == nil || tenantMw == nil || s.deps.Links == nil {
		return
	}

	protected := func(next http.HandlerFunc) http.Handler {
		return authMw(tenantMw(next))
	}

	s.mux.Handle("GET /api/v1/messenger-links", protected(s.handleMessengerLinksList))
	s.mux.Handle("DELETE /api/v1/messenger-links/{id}", protected(s.handleMessengerLinksDelete))
	if s.deps.Linking != nil {
		s.mux.Handle("POST /api/v1/auth/link/complete", protected(s.handleMessengerLinkComplete))
	}
}

func (s *Server) handleMessengerLinksList(w http.ResponseWriter, r *http.Request) {
	links, err := s.deps.Links.ListMessengerLinksByUser(r.Context(), sqlc.ListMessengerLinksByUserParams{
		UserID:   reqctx.UserIDFrom(r.Context()),
		TenantID: reqctx.TenantIDFrom(r.Context()),
	})
	if err != nil {
		writeAuthProblem(w, http.StatusInternalServerError, "failed to load messenger links")
		return
	}

	items := make([]messengerLinkResponse, 0, len(links))
	for _, link := range links {
		items = append(items, mapMessengerLink(link, "", ""))
	}

	writeJSON(w, http.StatusOK, map[string]any{"items": items})
}

func (s *Server) handleMessengerLinksDelete(w http.ResponseWriter, r *http.Request) {
	linkID, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		writeAuthProblem(w, http.StatusBadRequest, "invalid messenger link id")
		return
	}

	deleted, err := s.deps.Links.DeleteMessengerLinkByID(r.Context(), sqlc.DeleteMessengerLinkByIDParams{
		ID:       linkID,
		UserID:   reqctx.UserIDFrom(r.Context()),
		TenantID: reqctx.TenantIDFrom(r.Context()),
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			writeAuthProblem(w, http.StatusNotFound, "messenger link not found")
			return
		}
		writeAuthProblem(w, http.StatusInternalServerError, "failed to delete messenger link")
		return
	}

	writeJSON(w, http.StatusOK, mapMessengerLink(deleted, "removed", "Messenger account disconnected."))
}

func (s *Server) handleMessengerLinkComplete(w http.ResponseWriter, r *http.Request) {
	req, ok := decodeCompleteMessengerLinkRequest(w, r)
	if !ok {
		return
	}

	claims, err := s.deps.Linking.ParseToken(req.Token)
	if err != nil {
		writeAuthProblem(w, http.StatusBadRequest, "invalid or expired link token")
		return
	}
	if claims.TenantID != reqctx.TenantIDFrom(r.Context()) {
		writeAuthProblem(w, http.StatusForbidden, "link token does not belong to this tenant")
		return
	}

	existing, err := s.deps.Links.GetMessengerLink(r.Context(), sqlc.GetMessengerLinkParams{
		TenantID:   claims.TenantID,
		Platform:   claims.Platform,
		ExternalID: claims.ExternalUserID,
	})
	if err == nil {
		if existing.UserID != reqctx.UserIDFrom(r.Context()) {
			writeAuthProblem(w, http.StatusConflict, "messenger account is already linked to another user")
			return
		}
		writeJSON(w, http.StatusOK, mapMessengerLink(existing, "already_linked", "Messenger account already linked."))
		return
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		writeAuthProblem(w, http.StatusInternalServerError, "failed to inspect messenger link")
		return
	}

	created, err := s.deps.Links.CreateMessengerLink(r.Context(), sqlc.CreateMessengerLinkParams{
		UserID:     reqctx.UserIDFrom(r.Context()),
		TenantID:   claims.TenantID,
		Platform:   claims.Platform,
		ExternalID: claims.ExternalUserID,
	})
	if err != nil {
		retry, retryErr := s.deps.Links.GetMessengerLink(r.Context(), sqlc.GetMessengerLinkParams{
			TenantID:   claims.TenantID,
			Platform:   claims.Platform,
			ExternalID: claims.ExternalUserID,
		})
		if retryErr == nil {
			if retry.UserID != reqctx.UserIDFrom(r.Context()) {
				writeAuthProblem(w, http.StatusConflict, "messenger account is already linked to another user")
				return
			}
			writeJSON(w, http.StatusOK, mapMessengerLink(retry, "already_linked", "Messenger account already linked."))
			return
		}
		writeAuthProblem(w, http.StatusInternalServerError, "failed to link messenger account")
		return
	}

	writeJSON(w, http.StatusCreated, mapMessengerLink(created, "linked", "Messenger account linked."))
}

func decodeCompleteMessengerLinkRequest(w http.ResponseWriter, r *http.Request) (*completeMessengerLinkRequest, bool) {
	defer r.Body.Close()

	var req completeMessengerLinkRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeAuthProblem(w, http.StatusBadRequest, "invalid request body")
		return nil, false
	}
	if strings.TrimSpace(req.Token) == "" {
		writeAuthProblem(w, http.StatusBadRequest, "link token is required")
		return nil, false
	}

	return &req, true
}

func mapMessengerLink(link sqlc.UserMessengerLink, status, message string) messengerLinkResponse {
	return messengerLinkResponse{
		Platform:   link.Platform,
		ExternalID: link.ExternalID,
		CreatedAt:  link.CreatedAt.UTC().Format(time.RFC3339),
		Status:     status,
		Message:    message,
		ID:         link.ID.String(),
	}
}
