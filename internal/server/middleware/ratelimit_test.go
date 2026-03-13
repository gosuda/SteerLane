package middleware

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	goredis "github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type fakeClock struct {
	now time.Time
}

func newFakeClock(now time.Time) *fakeClock {
	return &fakeClock{now: now}
}

func (c *fakeClock) Now() time.Time {
	return c.now
}

func (c *fakeClock) Advance(d time.Duration) {
	c.now = c.now.Add(d)
}

type fakeRateLimitStore struct {
	now       func() time.Time
	counts    map[string]int64
	expiresAt map[string]time.Time
	mu        sync.Mutex
}

func newFakeRateLimitStore(now func() time.Time) *fakeRateLimitStore {
	return &fakeRateLimitStore{
		now:       now,
		counts:    make(map[string]int64),
		expiresAt: make(map[string]time.Time),
	}
}

func (s *fakeRateLimitStore) Incr(_ context.Context, key string) *goredis.IntCmd {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.resetExpiredLocked(key)
	s.counts[key]++

	return goredis.NewIntResult(s.counts[key], nil)
}

func (s *fakeRateLimitStore) ExpireAt(_ context.Context, key string, expiration time.Time) *goredis.BoolCmd {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.expiresAt[key] = expiration

	return goredis.NewBoolResult(true, nil)
}

func (s *fakeRateLimitStore) resetExpiredLocked(key string) {
	expiresAt, ok := s.expiresAt[key]
	if !ok {
		return
	}
	if s.now().Before(expiresAt) {
		return
	}

	delete(s.counts, key)
	delete(s.expiresAt, key)
}

func mustParseCIDR(t *testing.T, cidr string) *net.IPNet {
	t.Helper()

	_, network, err := net.ParseCIDR(cidr)
	require.NoError(t, err)

	return network
}

func TestClientIP_NoTrustedProxiesReturnsRemoteAddr(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/projects", http.NoBody)
	req.RemoteAddr = "198.51.100.10:4567"
	req.Header.Set(headerForwardedFor, "203.0.113.9")
	req.Header.Set(headerRealIP, "203.0.113.10")

	assert.Equal(t, "198.51.100.10", clientIP(req, nil))
}

func TestClientIP_TrustedProxyUsesRightmostUntrustedXFFIP(t *testing.T) {
	trustedProxy := mustParseCIDR(t, "10.0.0.0/8")
	req := httptest.NewRequest(http.MethodGet, "/projects", http.NoBody)
	req.RemoteAddr = "10.0.0.5:4567"
	req.Header.Set(headerForwardedFor, "203.0.113.9, 192.0.2.20, 10.1.1.1")

	assert.Equal(t, "192.0.2.20", clientIP(req, []*net.IPNet{trustedProxy}))
}

func TestClientIP_UntrustedDirectPeerIgnoresForwardedHeaders(t *testing.T) {
	trustedProxy := mustParseCIDR(t, "10.0.0.0/8")
	req := httptest.NewRequest(http.MethodGet, "/projects", http.NoBody)
	req.RemoteAddr = "198.51.100.20:4567"
	req.Header.Set(headerForwardedFor, "203.0.113.9")
	req.Header.Set(headerRealIP, "203.0.113.10")

	assert.Equal(t, "198.51.100.20", clientIP(req, []*net.IPNet{trustedProxy}))
}

func TestRateLimit_AllowsWithinLimit(t *testing.T) {
	clock := newFakeClock(time.Date(2026, time.March, 12, 14, 0, 0, 0, time.UTC))
	store := newFakeRateLimitStore(clock.Now)
	nextCalls := 0

	handler := RateLimit(RateLimitConfig{
		Enabled:           true,
		RequestsPerMinute: 2,
		Store:             store,
		Logger:            newTestLogger(),
		Now:               clock.Now,
	})(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		nextCalls++
		w.WriteHeader(http.StatusOK)
	}))

	first := httptest.NewRequest(http.MethodGet, "/projects", http.NoBody).WithContext(t.Context())
	first.RemoteAddr = "192.0.2.10:1234"
	firstRecorder := httptest.NewRecorder()

	handler.ServeHTTP(firstRecorder, first)

	require.Equal(t, http.StatusOK, firstRecorder.Code)
	assert.Equal(t, "2", firstRecorder.Header().Get(headerRateLimitLimit))
	assert.Equal(t, "1", firstRecorder.Header().Get(headerRateLimitRemaining))

	second := httptest.NewRequest(http.MethodGet, "/projects", http.NoBody).WithContext(t.Context())
	second.RemoteAddr = "192.0.2.10:1234"
	secondRecorder := httptest.NewRecorder()

	handler.ServeHTTP(secondRecorder, second)

	require.Equal(t, http.StatusOK, secondRecorder.Code)
	assert.Equal(t, "0", secondRecorder.Header().Get(headerRateLimitRemaining))
	assert.Equal(t, 2, nextCalls)
}

func TestRateLimit_BlocksAfterLimit(t *testing.T) {
	clock := newFakeClock(time.Date(2026, time.March, 12, 14, 0, 0, 0, time.UTC))
	store := newFakeRateLimitStore(clock.Now)
	nextCalls := 0

	handler := RateLimit(RateLimitConfig{
		Enabled:           true,
		RequestsPerMinute: 1,
		Store:             store,
		Logger:            newTestLogger(),
		Now:               clock.Now,
	})(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		nextCalls++
		w.WriteHeader(http.StatusOK)
	}))

	first := httptest.NewRequest(http.MethodGet, "/tasks", http.NoBody).WithContext(t.Context())
	first.RemoteAddr = "192.0.2.11:1234"
	handler.ServeHTTP(httptest.NewRecorder(), first)

	blocked := httptest.NewRequest(http.MethodGet, "/tasks", http.NoBody).WithContext(t.Context())
	blocked.RemoteAddr = "192.0.2.11:1234"
	blockedRecorder := httptest.NewRecorder()

	handler.ServeHTTP(blockedRecorder, blocked)

	require.Equal(t, http.StatusTooManyRequests, blockedRecorder.Code)
	assert.Equal(t, "1", blockedRecorder.Header().Get(headerRateLimitLimit))
	assert.Equal(t, "0", blockedRecorder.Header().Get(headerRateLimitRemaining))
	assert.Equal(t, "60", blockedRecorder.Header().Get(headerRetryAfter))
	assert.Equal(t, "application/problem+json", blockedRecorder.Header().Get("Content-Type"))
	assert.Contains(t, blockedRecorder.Body.String(), "Too Many Requests")
	assert.Equal(t, 1, nextCalls)
}

func TestRateLimit_IsolatesAuthenticatedKeys(t *testing.T) {
	tenantID := uuid.New()
	userA := uuid.New()
	userB := uuid.New()
	clock := newFakeClock(time.Date(2026, time.March, 12, 14, 0, 0, 0, time.UTC))
	store := newFakeRateLimitStore(clock.Now)
	nextCalls := 0

	authenticator := &mockAuth{
		jwtFn: func(token string) (*Identity, error) {
			switch token {
			case "token-a":
				return &Identity{TenantID: tenantID, UserID: userA, Role: "member"}, nil
			case "token-b":
				return &Identity{TenantID: tenantID, UserID: userB, Role: "member"}, nil
			default:
				return nil, assert.AnError
			}
		},
	}

	handler := RateLimit(RateLimitConfig{
		Enabled:           true,
		RequestsPerMinute: 1,
		Store:             store,
		Authenticator:     authenticator,
		Logger:            newTestLogger(),
		Now:               clock.Now,
	})(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		nextCalls++
		w.WriteHeader(http.StatusOK)
	}))

	userAFirst := httptest.NewRequest(http.MethodGet, "/users", http.NoBody).WithContext(t.Context())
	userAFirst.RemoteAddr = "192.0.2.12:1234"
	userAFirst.Header.Set(headerAuthorization, bearerPrefix+"token-a")
	userAFirstRecorder := httptest.NewRecorder()
	handler.ServeHTTP(userAFirstRecorder, userAFirst)
	require.Equal(t, http.StatusOK, userAFirstRecorder.Code)

	userASecond := httptest.NewRequest(http.MethodGet, "/users", http.NoBody).WithContext(t.Context())
	userASecond.RemoteAddr = "192.0.2.12:1234"
	userASecond.Header.Set(headerAuthorization, bearerPrefix+"token-a")
	userASecondRecorder := httptest.NewRecorder()
	handler.ServeHTTP(userASecondRecorder, userASecond)
	require.Equal(t, http.StatusTooManyRequests, userASecondRecorder.Code)

	userBFirst := httptest.NewRequest(http.MethodGet, "/users", http.NoBody).WithContext(t.Context())
	userBFirst.RemoteAddr = "192.0.2.12:1234"
	userBFirst.Header.Set(headerAuthorization, bearerPrefix+"token-b")
	userBFirstRecorder := httptest.NewRecorder()
	handler.ServeHTTP(userBFirstRecorder, userBFirst)

	require.Equal(t, http.StatusOK, userBFirstRecorder.Code)
	assert.Equal(t, 2, nextCalls)
}

func TestRateLimit_ResetsAfterWindowExpiry(t *testing.T) {
	clock := newFakeClock(time.Date(2026, time.March, 12, 14, 0, 0, 0, time.UTC))
	store := newFakeRateLimitStore(clock.Now)
	nextCalls := 0

	handler := RateLimit(RateLimitConfig{
		Enabled:           true,
		RequestsPerMinute: 1,
		Store:             store,
		Logger:            newTestLogger(),
		Now:               clock.Now,
	})(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		nextCalls++
		w.WriteHeader(http.StatusOK)
	}))

	first := httptest.NewRequest(http.MethodGet, "/sessions", http.NoBody).WithContext(t.Context())
	first.RemoteAddr = "192.0.2.13:1234"
	handler.ServeHTTP(httptest.NewRecorder(), first)

	blocked := httptest.NewRequest(http.MethodGet, "/sessions", http.NoBody).WithContext(t.Context())
	blocked.RemoteAddr = "192.0.2.13:1234"
	blockedRecorder := httptest.NewRecorder()
	handler.ServeHTTP(blockedRecorder, blocked)
	require.Equal(t, http.StatusTooManyRequests, blockedRecorder.Code)

	clock.Advance(time.Minute + time.Second)

	reset := httptest.NewRequest(http.MethodGet, "/sessions", http.NoBody).WithContext(t.Context())
	reset.RemoteAddr = "192.0.2.13:1234"
	resetRecorder := httptest.NewRecorder()
	handler.ServeHTTP(resetRecorder, reset)

	require.Equal(t, http.StatusOK, resetRecorder.Code)
	assert.Equal(t, "0", resetRecorder.Header().Get(headerRateLimitRemaining))
	assert.Equal(t, 2, nextCalls)
}
