package ws

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/gosuda/steerlane/internal/domain"
	"github.com/gosuda/steerlane/internal/domain/agent"
	"github.com/gosuda/steerlane/internal/server/reqctx"
)

type mockAgentRepo struct {
	getByIDFn func(ctx context.Context, tenantID domain.TenantID, id domain.AgentSessionID) (*agent.Session, error)
}

func (m *mockAgentRepo) Create(_ context.Context, _ *agent.Session) error { return nil }

func (m *mockAgentRepo) GetByID(ctx context.Context, tenantID domain.TenantID, id domain.AgentSessionID) (*agent.Session, error) {
	if m.getByIDFn != nil {
		return m.getByIDFn(ctx, tenantID, id)
	}
	return nil, domain.ErrNotFound
}

func (m *mockAgentRepo) UpdateStatus(_ context.Context, _ domain.TenantID, _ domain.AgentSessionID, _ agent.SessionStatus) error {
	return nil
}

func (m *mockAgentRepo) ScheduleRetry(_ context.Context, _ domain.TenantID, _ domain.AgentSessionID, _ int, _ *time.Time) error {
	return nil
}

func (m *mockAgentRepo) ListByProject(_ context.Context, _ domain.TenantID, _ domain.ProjectID) ([]*agent.Session, error) {
	return nil, nil
}

func (m *mockAgentRepo) ListByTask(_ context.Context, _ domain.TenantID, _ domain.TaskID) ([]*agent.Session, error) {
	return nil, nil
}

func (m *mockAgentRepo) ListRetryReady(_ context.Context, _ time.Time, _ int) ([]*agent.Session, error) {
	return nil, nil
}

func serveAgentRequest(t *testing.T, ctx context.Context, sessionID string, repo agent.Repository) *httptest.ResponseRecorder {
	t.Helper()

	logger := newTestWSLogger()
	hub := NewHub(logger, nil)
	handler := HandleAgentStream(logger, hub, repo)
	mux := http.NewServeMux()
	mux.HandleFunc("/ws/agent/{session_id}", handler)

	req := httptest.NewRequest(http.MethodGet, "/ws/agent/"+sessionID, http.NoBody).WithContext(ctx)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	return rec
}

func TestAgentStream_NoAuth(t *testing.T) {
	t.Parallel()

	repoCalled := false
	rec := serveAgentRequest(t, t.Context(), uuid.NewString(), &mockAgentRepo{
		getByIDFn: func(context.Context, domain.TenantID, domain.AgentSessionID) (*agent.Session, error) {
			repoCalled = true
			return nil, nil
		},
	})

	require.Equal(t, http.StatusUnauthorized, rec.Code)
	assert.Contains(t, rec.Body.String(), "unauthorized")
	assert.False(t, repoCalled)
}

func TestAgentStream_InvalidSessionID(t *testing.T) {
	t.Parallel()

	ctx := reqctx.WithUser(t.Context(), uuid.New(), testWSUserRole)
	repoCalled := false
	rec := serveAgentRequest(t, ctx, "not-a-uuid", &mockAgentRepo{
		getByIDFn: func(context.Context, domain.TenantID, domain.AgentSessionID) (*agent.Session, error) {
			repoCalled = true
			return nil, nil
		},
	})

	require.Equal(t, http.StatusBadRequest, rec.Code)
	assert.Contains(t, rec.Body.String(), "invalid session ID format")
	assert.False(t, repoCalled)
}

func TestAgentStream_NoTenant(t *testing.T) {
	t.Parallel()

	ctx := reqctx.WithUser(t.Context(), uuid.New(), testWSUserRole)
	repoCalled := false
	rec := serveAgentRequest(t, ctx, uuid.NewString(), &mockAgentRepo{
		getByIDFn: func(context.Context, domain.TenantID, domain.AgentSessionID) (*agent.Session, error) {
			repoCalled = true
			return nil, nil
		},
	})

	require.Equal(t, http.StatusUnauthorized, rec.Code)
	assert.Contains(t, rec.Body.String(), "unauthorized tenant")
	assert.False(t, repoCalled)
}

func TestAgentStream_SessionNotFound(t *testing.T) {
	t.Parallel()

	userID := uuid.New()
	tenantID := uuid.New()
	sessionID := uuid.New()
	ctx := reqctx.WithTenant(reqctx.WithUser(t.Context(), userID, testWSUserRole), tenantID)

	repoCalled := false
	rec := serveAgentRequest(t, ctx, sessionID.String(), &mockAgentRepo{
		getByIDFn: func(_ context.Context, gotTenantID domain.TenantID, gotSessionID domain.AgentSessionID) (*agent.Session, error) {
			repoCalled = true
			assert.Equal(t, tenantID, gotTenantID)
			assert.Equal(t, sessionID, gotSessionID)
			return nil, domain.ErrNotFound
		},
	})

	require.Equal(t, http.StatusNotFound, rec.Code)
	assert.Contains(t, rec.Body.String(), "session not found or unauthorized")
	assert.True(t, repoCalled)
}

func TestAgentStream_WrongTenant(t *testing.T) {
	t.Parallel()

	userID := uuid.New()
	tenantID := uuid.New()
	sessionID := uuid.New()
	ctx := reqctx.WithTenant(reqctx.WithUser(t.Context(), userID, testWSUserRole), tenantID)
	expectedErr := errors.New("tenant mismatch")

	repoCalled := false
	rec := serveAgentRequest(t, ctx, sessionID.String(), &mockAgentRepo{
		getByIDFn: func(_ context.Context, gotTenantID domain.TenantID, gotSessionID domain.AgentSessionID) (*agent.Session, error) {
			repoCalled = true
			assert.Equal(t, tenantID, gotTenantID)
			assert.Equal(t, sessionID, gotSessionID)
			return nil, expectedErr
		},
	})

	require.Equal(t, http.StatusNotFound, rec.Code)
	assert.Contains(t, rec.Body.String(), "session not found or unauthorized")
	assert.True(t, repoCalled)
}
