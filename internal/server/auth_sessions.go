package server

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"

	"github.com/google/uuid"

	"github.com/gosuda/steerlane/internal/domain"
)

const (
	authCookieAccess  = "steerlane_access_token"
	authCookieRefresh = "steerlane_refresh_token"
)

type authSessionRequest struct {
	Name       string          `json:"name,omitempty"`
	Email      string          `json:"email,omitempty"`
	Password   string          `json:"password,omitempty"`
	TenantSlug string          `json:"tenant_slug,omitempty"`
	TenantID   domain.TenantID `json:"tenant_id,omitempty"`
}

type authSessionResponse struct {
	TenantSlug    string          `json:"tenant_slug,omitempty"`
	Role          string          `json:"role,omitempty"`
	TenantID      domain.TenantID `json:"tenant_id,omitempty"`
	Authenticated bool            `json:"authenticated"`
}

type authLinkMetadataResponse struct {
	TenantSlug string `json:"tenant_slug"`
	Platform   string `json:"platform"`
}

func (s *Server) registerAuthSessionRoutes() {
	s.mux.HandleFunc("GET /api/v1/auth/session", s.handleAuthSessionGet)
	s.mux.HandleFunc("GET /api/v1/auth/link/metadata", s.handleAuthLinkMetadata)
	s.mux.HandleFunc("POST /api/v1/auth/session/login", s.handleAuthSessionLogin)
	s.mux.HandleFunc("POST /api/v1/auth/session/register", s.handleAuthSessionRegister)
	s.mux.HandleFunc("POST /api/v1/auth/session/refresh", s.handleAuthSessionRefresh)
	s.mux.HandleFunc("POST /api/v1/auth/session/logout", s.handleAuthSessionLogout)
}

func (s *Server) handleAuthLinkMetadata(w http.ResponseWriter, r *http.Request) {
	token := strings.TrimSpace(r.URL.Query().Get("token"))
	if token == "" {
		writeAuthProblem(w, http.StatusBadRequest, "link token is required")
		return
	}
	if s.deps.Linking == nil {
		writeAuthProblem(w, http.StatusNotImplemented, "linking is not configured")
		return
	}

	claims, err := s.deps.Linking.ParseToken(token)
	if err != nil {
		writeAuthProblem(w, http.StatusBadRequest, "invalid or expired link token")
		return
	}

	tenantRecord, err := s.deps.V1.Tenants.GetByID(r.Context(), claims.TenantID)
	if err != nil {
		writeAuthProblem(w, http.StatusNotFound, "tenant not found")
		return
	}

	writeJSON(w, http.StatusOK, authLinkMetadataResponse{TenantSlug: tenantRecord.Slug, Platform: claims.Platform})
}

func (s *Server) handleAuthSessionGet(w http.ResponseWriter, r *http.Request) {
	accessCookie, err := r.Cookie(authCookieAccess)
	if err != nil || strings.TrimSpace(accessCookie.Value) == "" {
		writeAuthProblem(w, http.StatusUnauthorized, "missing session")
		return
	}

	claims, err := s.deps.V1.Auth.AuthenticateJWT(accessCookie.Value)
	if err != nil {
		clearAuthCookies(w, r)
		writeAuthProblem(w, http.StatusUnauthorized, "invalid session")
		return
	}

	tenantRecord, err := s.deps.V1.Tenants.GetByID(r.Context(), claims.TenantID)
	if err != nil {
		writeAuthProblem(w, http.StatusUnauthorized, "tenant not found")
		return
	}

	writeJSON(w, http.StatusOK, authSessionResponse{
		Authenticated: true,
		TenantID:      tenantRecord.ID,
		TenantSlug:    tenantRecord.Slug,
		Role:          claims.Role,
	})
}

func (s *Server) handleAuthSessionLogin(w http.ResponseWriter, r *http.Request) {
	req, ok := decodeAuthSessionRequest(w, r)
	if !ok {
		return
	}

	tenantRecord, ok := s.resolveAuthTenantRecord(w, r, req.TenantID, req.TenantSlug)
	if !ok {
		return
	}

	accessToken, refreshToken, err := s.deps.V1.Auth.Login(r.Context(), tenantRecord.ID, req.Email, req.Password)
	if err != nil {
		writeAuthProblem(w, http.StatusUnauthorized, "unable to sign in")
		return
	}

	setAuthCookies(w, r, accessToken, refreshToken)
	writeJSON(w, http.StatusOK, authSessionResponse{Authenticated: true, TenantID: tenantRecord.ID, TenantSlug: tenantRecord.Slug})
}

func (s *Server) handleAuthSessionRegister(w http.ResponseWriter, r *http.Request) {
	req, ok := decodeAuthSessionRequest(w, r)
	if !ok {
		return
	}

	tenantRecord, ok := s.resolveAuthTenantRecord(w, r, req.TenantID, req.TenantSlug)
	if !ok {
		return
	}

	if _, err := s.deps.V1.Auth.Register(r.Context(), tenantRecord.ID, req.Email, req.Password, req.Name); err != nil {
		writeAuthProblem(w, http.StatusBadRequest, "unable to create account")
		return
	}

	accessToken, refreshToken, err := s.deps.V1.Auth.Login(r.Context(), tenantRecord.ID, req.Email, req.Password)
	if err != nil {
		writeAuthProblem(w, http.StatusUnauthorized, "unable to sign in")
		return
	}

	setAuthCookies(w, r, accessToken, refreshToken)
	writeJSON(w, http.StatusOK, authSessionResponse{Authenticated: true, TenantID: tenantRecord.ID, TenantSlug: tenantRecord.Slug})
}

func (s *Server) handleAuthSessionRefresh(w http.ResponseWriter, r *http.Request) {
	refreshCookie, err := r.Cookie(authCookieRefresh)
	if err != nil || strings.TrimSpace(refreshCookie.Value) == "" {
		clearAuthCookies(w, r)
		writeAuthProblem(w, http.StatusUnauthorized, "missing refresh session")
		return
	}

	accessToken, refreshToken, err := s.deps.V1.Auth.RefreshToken(r.Context(), refreshCookie.Value)
	if err != nil {
		clearAuthCookies(w, r)
		writeAuthProblem(w, http.StatusUnauthorized, "unable to refresh session")
		return
	}

	setAuthCookies(w, r, accessToken, refreshToken)
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleAuthSessionLogout(w http.ResponseWriter, r *http.Request) {
	clearAuthCookies(w, r)
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) resolveAuthTenantRecord(w http.ResponseWriter, r *http.Request, tenantID domain.TenantID, tenantSlug string) (*domainTenantRecord, bool) {
	if tenantID != uuid.Nil {
		record, err := s.deps.V1.Tenants.GetByID(r.Context(), tenantID)
		if err != nil {
			writeAuthProblem(w, http.StatusNotFound, "tenant not found")
			return nil, false
		}
		return &domainTenantRecord{ID: record.ID, Slug: record.Slug}, true
	}

	trimmedSlug := strings.TrimSpace(tenantSlug)
	if trimmedSlug == "" {
		writeAuthProblem(w, http.StatusBadRequest, "tenant_id or tenant_slug is required")
		return nil, false
	}

	record, err := s.deps.V1.Tenants.GetBySlug(r.Context(), trimmedSlug)
	if err != nil {
		writeAuthProblem(w, http.StatusNotFound, "tenant not found")
		return nil, false
	}
	return &domainTenantRecord{ID: record.ID, Slug: record.Slug}, true
}

type domainTenantRecord struct {
	Slug string
	ID   domain.TenantID
}

func decodeAuthSessionRequest(w http.ResponseWriter, r *http.Request) (*authSessionRequest, bool) {
	defer r.Body.Close()

	decoder := json.NewDecoder(io.LimitReader(r.Body, 1<<20))
	decoder.DisallowUnknownFields()

	var req authSessionRequest
	if err := decoder.Decode(&req); err != nil {
		writeAuthProblem(w, http.StatusBadRequest, "invalid request body")
		return nil, false
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		writeAuthProblem(w, http.StatusBadRequest, "invalid request body")
		return nil, false
	}

	return &req, true
}

func setAuthCookies(w http.ResponseWriter, r *http.Request, accessToken, refreshToken string) {
	http.SetCookie(w, buildAuthCookie(r, authCookieAccess, accessToken, false))
	http.SetCookie(w, buildAuthCookie(r, authCookieRefresh, refreshToken, false))
}

func clearAuthCookies(w http.ResponseWriter, r *http.Request) {
	http.SetCookie(w, buildAuthCookie(r, authCookieAccess, "", true))
	http.SetCookie(w, buildAuthCookie(r, authCookieRefresh, "", true))
}

func buildAuthCookie(r *http.Request, name, value string, expired bool) *http.Cookie {
	cookie := &http.Cookie{
		Name:     name,
		Value:    value,
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Secure:   requestUsesTLS(r),
	}
	if expired {
		cookie.MaxAge = -1
	}
	return cookie
}

func requestUsesTLS(r *http.Request) bool {
	if r.TLS != nil {
		return true
	}
	return strings.EqualFold(r.Header.Get("X-Forwarded-Proto"), "https")
}

func writeAuthProblem(w http.ResponseWriter, status int, detail string) {
	w.Header().Set("Content-Type", "application/problem+json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"status": status,
		"title":  http.StatusText(status),
		"detail": detail,
	})
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}
