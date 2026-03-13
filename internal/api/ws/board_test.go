package ws

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/gosuda/steerlane/internal/domain"
	"github.com/gosuda/steerlane/internal/domain/project"
	"github.com/gosuda/steerlane/internal/server/reqctx"
)

const testWSUserRole = "member"

type mockProjectRepo struct {
	getByIDFn func(ctx context.Context, tenantID domain.TenantID, id domain.ProjectID) (*project.Project, error)
}

func (m *mockProjectRepo) Create(_ context.Context, _ *project.Project) error { return nil }

func (m *mockProjectRepo) GetByID(ctx context.Context, tenantID domain.TenantID, id domain.ProjectID) (*project.Project, error) {
	if m.getByIDFn != nil {
		return m.getByIDFn(ctx, tenantID, id)
	}
	return nil, domain.ErrNotFound
}

func (m *mockProjectRepo) Update(_ context.Context, _ *project.Project) error { return nil }

func (m *mockProjectRepo) Delete(_ context.Context, _ domain.TenantID, _ domain.ProjectID) error {
	return nil
}

func (m *mockProjectRepo) ListByTenant(_ context.Context, _ domain.TenantID, _ int, _ *uuid.UUID) ([]*project.Project, error) {
	return nil, nil
}

func newTestWSLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func serveBoardRequest(t *testing.T, ctx context.Context, projectID string, repo project.Repository) *httptest.ResponseRecorder {
	t.Helper()

	logger := newTestWSLogger()
	hub := NewHub(logger, nil)
	handler := HandleBoardStream(logger, hub, repo)
	mux := http.NewServeMux()
	mux.HandleFunc("/ws/board/{project_id}", handler)

	req := httptest.NewRequest(http.MethodGet, "/ws/board/"+projectID, http.NoBody).WithContext(ctx)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	return rec
}

func TestBoardStream_NoAuth(t *testing.T) {
	t.Parallel()

	repoCalled := false
	rec := serveBoardRequest(t, t.Context(), uuid.NewString(), &mockProjectRepo{
		getByIDFn: func(context.Context, domain.TenantID, domain.ProjectID) (*project.Project, error) {
			repoCalled = true
			return nil, nil
		},
	})

	require.Equal(t, http.StatusUnauthorized, rec.Code)
	assert.Contains(t, rec.Body.String(), "unauthorized")
	assert.False(t, repoCalled)
}

func TestBoardStream_InvalidProjectID(t *testing.T) {
	t.Parallel()

	ctx := reqctx.WithUser(t.Context(), uuid.New(), testWSUserRole)
	repoCalled := false
	rec := serveBoardRequest(t, ctx, "not-a-uuid", &mockProjectRepo{
		getByIDFn: func(context.Context, domain.TenantID, domain.ProjectID) (*project.Project, error) {
			repoCalled = true
			return nil, nil
		},
	})

	require.Equal(t, http.StatusBadRequest, rec.Code)
	assert.Contains(t, rec.Body.String(), "invalid project ID format")
	assert.False(t, repoCalled)
}

func TestBoardStream_NoTenant(t *testing.T) {
	t.Parallel()

	ctx := reqctx.WithUser(t.Context(), uuid.New(), testWSUserRole)
	repoCalled := false
	rec := serveBoardRequest(t, ctx, uuid.NewString(), &mockProjectRepo{
		getByIDFn: func(context.Context, domain.TenantID, domain.ProjectID) (*project.Project, error) {
			repoCalled = true
			return nil, nil
		},
	})

	require.Equal(t, http.StatusUnauthorized, rec.Code)
	assert.Contains(t, rec.Body.String(), "unauthorized tenant")
	assert.False(t, repoCalled)
}

func TestBoardStream_ProjectNotFound(t *testing.T) {
	t.Parallel()

	userID := uuid.New()
	tenantID := uuid.New()
	projectID := uuid.New()
	ctx := reqctx.WithTenant(reqctx.WithUser(t.Context(), userID, testWSUserRole), tenantID)

	repoCalled := false
	rec := serveBoardRequest(t, ctx, projectID.String(), &mockProjectRepo{
		getByIDFn: func(_ context.Context, gotTenantID domain.TenantID, gotProjectID domain.ProjectID) (*project.Project, error) {
			repoCalled = true
			assert.Equal(t, tenantID, gotTenantID)
			assert.Equal(t, projectID, gotProjectID)
			return nil, domain.ErrNotFound
		},
	})

	require.Equal(t, http.StatusNotFound, rec.Code)
	assert.Contains(t, rec.Body.String(), "project not found or unauthorized")
	assert.True(t, repoCalled)
}

func TestBoardStream_WrongTenant(t *testing.T) {
	t.Parallel()

	userID := uuid.New()
	tenantID := uuid.New()
	projectID := uuid.New()
	ctx := reqctx.WithTenant(reqctx.WithUser(t.Context(), userID, testWSUserRole), tenantID)
	expectedErr := errors.New("tenant mismatch")

	repoCalled := false
	rec := serveBoardRequest(t, ctx, projectID.String(), &mockProjectRepo{
		getByIDFn: func(_ context.Context, gotTenantID domain.TenantID, gotProjectID domain.ProjectID) (*project.Project, error) {
			repoCalled = true
			assert.Equal(t, tenantID, gotTenantID)
			assert.Equal(t, projectID, gotProjectID)
			return nil, expectedErr
		},
	})

	require.Equal(t, http.StatusNotFound, rec.Code)
	assert.Contains(t, rec.Body.String(), "project not found or unauthorized")
	assert.True(t, repoCalled)
}
