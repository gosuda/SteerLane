package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/gosuda/steerlane/internal/server/reqctx"
)

func TestTenant_Present(t *testing.T) {
	tenantID := uuid.New()
	nextCalled := false

	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		nextCalled = true
		w.WriteHeader(http.StatusOK)
	})

	handler := Tenant(newTestLogger())(next)
	req := httptest.NewRequest(http.MethodGet, "/tenant", http.NoBody).WithContext(reqctx.WithTenant(t.Context(), tenantID))
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	require.True(t, nextCalled)
	assert.Equal(t, http.StatusOK, rr.Code)
}

func TestTenant_Missing(t *testing.T) {
	nextCalled := false

	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		nextCalled = true
		w.WriteHeader(http.StatusOK)
	})

	handler := Tenant(newTestLogger())(next)
	req := httptest.NewRequest(http.MethodGet, "/tenant", http.NoBody).WithContext(t.Context())
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	require.False(t, nextCalled)
	require.Equal(t, http.StatusForbidden, rr.Code)
	assert.Equal(t, "application/problem+json", rr.Header().Get("Content-Type"))
	assert.Contains(t, rr.Body.String(), "tenant context is required")
}

func TestTenant_NilUUID(t *testing.T) {
	nextCalled := false

	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		nextCalled = true
		w.WriteHeader(http.StatusOK)
	})

	handler := Tenant(newTestLogger())(next)
	req := httptest.NewRequest(http.MethodGet, "/tenant", http.NoBody).WithContext(reqctx.WithTenant(t.Context(), uuid.Nil))
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	require.False(t, nextCalled)
	require.Equal(t, http.StatusForbidden, rr.Code)
	assert.Equal(t, "application/problem+json", rr.Header().Get("Content-Type"))
	assert.Contains(t, rr.Body.String(), "tenant context is required")
}
