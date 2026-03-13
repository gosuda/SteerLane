package middleware

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

	"github.com/gosuda/steerlane/internal/server/reqctx"
)

type mockAuth struct {
	jwtFn    func(token string) (*Identity, error)
	apiKeyFn func(ctx context.Context, rawKey string) (*Identity, error)
}

func (m *mockAuth) AuthenticateJWT(token string) (*Identity, error) {
	if m.jwtFn != nil {
		return m.jwtFn(token)
	}

	return nil, errors.New("not implemented")
}

func (m *mockAuth) AuthenticateAPIKey(ctx context.Context, rawKey string) (*Identity, error) {
	if m.apiKeyFn != nil {
		return m.apiKeyFn(ctx, rawKey)
	}

	return nil, errors.New("not implemented")
}

func newTestLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func TestAuth_BearerToken_Success(t *testing.T) {
	userID := uuid.New()
	tenantID := uuid.New()

	authenticator := &mockAuth{
		jwtFn: func(token string) (*Identity, error) {
			require.Equal(t, "valid-token", token)

			return &Identity{
				Role:     "admin",
				UserID:   userID,
				TenantID: tenantID,
			}, nil
		},
	}

	nextCalled := false
	var nextCtx context.Context

	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		nextCalled = true
		nextCtx = r.Context() //nolint:fatcontext // capturing context for test assertion
		w.WriteHeader(http.StatusOK)
	})

	handler := Auth(authenticator, newTestLogger())(next)
	req := httptest.NewRequest(http.MethodGet, "/protected", http.NoBody).WithContext(t.Context())
	req.Header.Set(headerAuthorization, bearerPrefix+"valid-token")
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	require.True(t, nextCalled)
	require.NotNil(t, nextCtx)
	require.Equal(t, http.StatusOK, rr.Code)
	assert.Equal(t, userID, reqctx.UserIDFrom(nextCtx))
	assert.Equal(t, "admin", reqctx.UserRoleFrom(nextCtx))
	assert.Equal(t, tenantID, reqctx.TenantIDFrom(nextCtx))
}

func TestAuth_APIKey_Success(t *testing.T) {
	userID := uuid.New()
	tenantID := uuid.New()

	authenticator := &mockAuth{
		apiKeyFn: func(ctx context.Context, rawKey string) (*Identity, error) {
			require.NotNil(t, ctx)
			require.Equal(t, "sl_testkey", rawKey)

			return &Identity{
				Role:     "agent",
				UserID:   userID,
				TenantID: tenantID,
			}, nil
		},
	}

	nextCalled := false
	var nextCtx context.Context

	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		nextCalled = true
		nextCtx = r.Context() //nolint:fatcontext // capturing context for test assertion
		w.WriteHeader(http.StatusOK)
	})

	handler := Auth(authenticator, newTestLogger())(next)
	req := httptest.NewRequest(http.MethodGet, "/protected", http.NoBody).WithContext(t.Context())
	req.Header.Set(headerAPIKey, "sl_testkey")
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	require.True(t, nextCalled)
	require.NotNil(t, nextCtx)
	require.Equal(t, http.StatusOK, rr.Code)
	assert.Equal(t, userID, reqctx.UserIDFrom(nextCtx))
	assert.Equal(t, "agent", reqctx.UserRoleFrom(nextCtx))
	assert.Equal(t, tenantID, reqctx.TenantIDFrom(nextCtx))
}

func TestAuth_CookieAccessToken_Success(t *testing.T) {
	userID := uuid.New()
	tenantID := uuid.New()

	authenticator := &mockAuth{
		jwtFn: func(token string) (*Identity, error) {
			require.Equal(t, "cookie-token", token)
			return &Identity{
				Role:     "member",
				UserID:   userID,
				TenantID: tenantID,
			}, nil
		},
	}

	nextCalled := false
	var nextCtx context.Context

	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		nextCalled = true
		nextCtx = r.Context() //nolint:fatcontext // capturing context for test assertion
		w.WriteHeader(http.StatusOK)
	})

	handler := Auth(authenticator, newTestLogger())(next)
	req := httptest.NewRequest(http.MethodGet, "/ws/board/project", http.NoBody).WithContext(t.Context())
	req.AddCookie(&http.Cookie{Name: cookieAccessToken, Value: "cookie-token"})
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	require.True(t, nextCalled)
	require.NotNil(t, nextCtx)
	require.Equal(t, http.StatusOK, rr.Code)
	assert.Equal(t, userID, reqctx.UserIDFrom(nextCtx))
	assert.Equal(t, tenantID, reqctx.TenantIDFrom(nextCtx))
}

func TestAuth_NoHeader(t *testing.T) {
	authenticator := &mockAuth{}
	nextCalled := false

	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		nextCalled = true
		w.WriteHeader(http.StatusOK)
	})

	handler := Auth(authenticator, newTestLogger())(next)
	req := httptest.NewRequest(http.MethodGet, "/protected", http.NoBody).WithContext(t.Context())
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	require.False(t, nextCalled)
	require.Equal(t, http.StatusUnauthorized, rr.Code)
	assert.Equal(t, "application/problem+json", rr.Header().Get("Content-Type"))
	assert.Contains(t, rr.Body.String(), "missing authentication credentials")
}

func TestAuth_InvalidBearerToken(t *testing.T) {
	authenticator := &mockAuth{
		jwtFn: func(token string) (*Identity, error) {
			require.Equal(t, "bad-token", token)
			return nil, errors.New("invalid token")
		},
	}

	nextCalled := false
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		nextCalled = true
		w.WriteHeader(http.StatusOK)
	})

	handler := Auth(authenticator, newTestLogger())(next)
	req := httptest.NewRequest(http.MethodGet, "/protected", http.NoBody).WithContext(t.Context())
	req.Header.Set(headerAuthorization, bearerPrefix+"bad-token")
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	require.False(t, nextCalled)
	require.Equal(t, http.StatusUnauthorized, rr.Code)
	assert.Equal(t, "application/problem+json", rr.Header().Get("Content-Type"))
	assert.Contains(t, rr.Body.String(), "invalid or expired credentials")
}

func TestAuth_BearerPreferred(t *testing.T) {
	jwtUserID := uuid.New()
	jwtTenantID := uuid.New()
	apiKeyCalled := false

	authenticator := &mockAuth{
		jwtFn: func(token string) (*Identity, error) {
			require.Equal(t, "jwt-token", token)

			return &Identity{
				Role:     "member",
				UserID:   jwtUserID,
				TenantID: jwtTenantID,
			}, nil
		},
		apiKeyFn: func(ctx context.Context, rawKey string) (*Identity, error) {
			apiKeyCalled = true
			return &Identity{
				Role:     "api",
				UserID:   uuid.New(),
				TenantID: uuid.New(),
			}, nil
		},
	}

	nextCalled := false
	var nextCtx context.Context

	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		nextCalled = true
		nextCtx = r.Context() //nolint:fatcontext // capturing context for test assertion
		w.WriteHeader(http.StatusOK)
	})

	handler := Auth(authenticator, newTestLogger())(next)
	req := httptest.NewRequest(http.MethodGet, "/protected", http.NoBody).WithContext(t.Context())
	req.Header.Set(headerAuthorization, bearerPrefix+"jwt-token")
	req.Header.Set(headerAPIKey, "sl_testkey")
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	require.True(t, nextCalled)
	require.NotNil(t, nextCtx)
	require.Equal(t, http.StatusOK, rr.Code)
	assert.False(t, apiKeyCalled)
	assert.Equal(t, jwtUserID, reqctx.UserIDFrom(nextCtx))
	assert.Equal(t, "member", reqctx.UserRoleFrom(nextCtx))
	assert.Equal(t, jwtTenantID, reqctx.TenantIDFrom(nextCtx))
}

func TestAuth_NilTenantID(t *testing.T) {
	userID := uuid.New()

	authenticator := &mockAuth{
		jwtFn: func(token string) (*Identity, error) {
			require.Equal(t, "valid-token", token)

			return &Identity{
				Role:     "viewer",
				UserID:   userID,
				TenantID: uuid.Nil,
			}, nil
		},
	}

	nextCalled := false
	var nextCtx context.Context

	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		nextCalled = true
		nextCtx = r.Context() //nolint:fatcontext // capturing context for test assertion
		w.WriteHeader(http.StatusOK)
	})

	handler := Auth(authenticator, newTestLogger())(next)
	req := httptest.NewRequest(http.MethodGet, "/protected", http.NoBody).WithContext(t.Context())
	req.Header.Set(headerAuthorization, bearerPrefix+"valid-token")
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	require.True(t, nextCalled)
	require.NotNil(t, nextCtx)
	require.Equal(t, http.StatusOK, rr.Code)
	assert.Equal(t, userID, reqctx.UserIDFrom(nextCtx))
	assert.Equal(t, "viewer", reqctx.UserRoleFrom(nextCtx))
	assert.Equal(t, uuid.Nil, reqctx.TenantIDFrom(nextCtx))
}
