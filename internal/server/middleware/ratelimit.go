package middleware

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	goredis "github.com/redis/go-redis/v9"

	"github.com/gosuda/steerlane/internal/server/reqctx"
)

const (
	rateLimitKeyPrefix       = "steerlane:ratelimit:v1"
	headerRateLimitLimit     = "X-RateLimit-Limit"
	headerRateLimitRemaining = "X-RateLimit-Remaining"
	headerRetryAfter         = "Retry-After"
	headerForwardedFor       = "X-Forwarded-For"
	headerRealIP             = "X-Real-IP"
	rateLimitProblemResponse = `{"status":429,"title":"Too Many Requests","detail":"rate limit exceeded"}`
)

// RateLimitStore is the small Redis surface the middleware needs.
// *redis.Client satisfies this interface directly.
type RateLimitStore interface {
	Incr(ctx context.Context, key string) *goredis.IntCmd
	ExpireAt(ctx context.Context, key string, expiration time.Time) *goredis.BoolCmd
}

// RateLimitConfig controls the global rate limiting middleware.
type RateLimitConfig struct {
	Store             RateLimitStore
	Authenticator     Authenticator
	Logger            *slog.Logger
	Now               func() time.Time
	TrustedProxyCIDRs []string
	RequestsPerMinute int
	Enabled           bool
}

type rateLimitDecision struct {
	allowed    bool
	limit      int
	remaining  int
	retryAfter int
}

// RateLimit enforces a fixed-window Redis-backed request limit.
// Authenticated requests are keyed by tenant and user; all others fall back to IP.
func RateLimit(cfg RateLimitConfig) func(http.Handler) http.Handler {
	if !cfg.Enabled || cfg.RequestsPerMinute <= 0 || cfg.Store == nil {
		return func(next http.Handler) http.Handler {
			return next
		}
	}

	logger := cfg.Logger
	if logger == nil {
		logger = slog.Default()
	}

	nowFn := cfg.Now
	if nowFn == nil {
		nowFn = time.Now
	}

	var trustedNets []*net.IPNet
	for _, cidr := range cfg.TrustedProxyCIDRs {
		_, network, err := net.ParseCIDR(strings.TrimSpace(cidr))
		if err != nil {
			logger.Warn("invalid trusted proxy CIDR, skipping", "cidr", cidr, "error", err)
			continue
		}
		trustedNets = append(trustedNets, network)
	}

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			key := rateLimitKey(r, cfg.Authenticator, trustedNets)

			decision, err := cfg.evaluate(r.Context(), key, nowFn().UTC())
			if err != nil {
				logger.WarnContext(r.Context(), "rate limit check failed",
					"error", err,
					"key", key,
					"path", r.URL.Path,
					"request_id", reqctx.RequestIDFrom(r.Context()),
				)
				next.ServeHTTP(w, r)
				return
			}

			w.Header().Set(headerRateLimitLimit, strconv.Itoa(decision.limit))
			w.Header().Set(headerRateLimitRemaining, strconv.Itoa(decision.remaining))

			if !decision.allowed {
				w.Header().Set(headerRetryAfter, strconv.Itoa(decision.retryAfter))
				writeRateLimitExceeded(w)
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

func (cfg RateLimitConfig) evaluate(ctx context.Context, key string, now time.Time) (rateLimitDecision, error) {
	windowStart := now.Truncate(time.Minute)
	windowEnd := windowStart.Add(time.Minute)
	redisKey := fmt.Sprintf("%s:%s:%d", rateLimitKeyPrefix, key, windowStart.Unix())

	countCmd := cfg.Store.Incr(ctx, redisKey)
	if err := countCmd.Err(); err != nil {
		return rateLimitDecision{}, fmt.Errorf("increment counter: %w", err)
	}

	expireCmd := cfg.Store.ExpireAt(ctx, redisKey, windowEnd)
	if err := expireCmd.Err(); err != nil {
		return rateLimitDecision{}, fmt.Errorf("set counter expiry: %w", err)
	}

	count := int(countCmd.Val())
	remaining := max(cfg.RequestsPerMinute-count, 0)

	return rateLimitDecision{
		allowed:    count <= cfg.RequestsPerMinute,
		limit:      cfg.RequestsPerMinute,
		remaining:  remaining,
		retryAfter: secondsUntil(windowEnd, now),
	}, nil
}

func rateLimitKey(r *http.Request, authenticator Authenticator, trustedProxies []*net.IPNet) string {
	if tenantID := reqctx.TenantIDFrom(r.Context()); tenantID != uuid.Nil {
		if userID := reqctx.UserIDFrom(r.Context()); userID != uuid.Nil {
			return authenticatedRateLimitKey(tenantID, userID)
		}
	}

	if identity := authenticateRateLimitIdentity(r, authenticator); identity != nil {
		if identity.TenantID != uuid.Nil && identity.UserID != uuid.Nil {
			return authenticatedRateLimitKey(identity.TenantID, identity.UserID)
		}
	}

	return "ip:" + clientIP(r, trustedProxies)
}

func authenticateRateLimitIdentity(r *http.Request, authenticator Authenticator) *Identity {
	if authenticator == nil {
		return nil
	}

	if authHeader := r.Header.Get(headerAuthorization); strings.HasPrefix(authHeader, bearerPrefix) {
		identity, err := authenticator.AuthenticateJWT(strings.TrimPrefix(authHeader, bearerPrefix))
		if err == nil {
			return identity
		}
		return nil
	}

	if apiKey := strings.TrimSpace(r.Header.Get(headerAPIKey)); apiKey != "" {
		identity, err := authenticator.AuthenticateAPIKey(r.Context(), apiKey)
		if err == nil {
			return identity
		}
		return nil
	}

	accessCookie, err := r.Cookie(cookieAccessToken)
	if err != nil || strings.TrimSpace(accessCookie.Value) == "" {
		return nil
	}

	identity, authErr := authenticator.AuthenticateJWT(accessCookie.Value)
	if authErr != nil {
		return nil
	}

	return identity
}

func authenticatedRateLimitKey(tenantID, userID uuid.UUID) string {
	return fmt.Sprintf("tenant:%s:user:%s", tenantID.String(), userID.String())
}

func clientIP(r *http.Request, trustedProxies []*net.IPNet) string {
	directAddr, directIP := directClientIP(r.RemoteAddr)
	if len(trustedProxies) == 0 || directIP == nil || !ipInNets(directIP, trustedProxies) {
		if directAddr != "" {
			return directAddr
		}
		return "unknown"
	}

	if forwardedFor := strings.TrimSpace(r.Header.Get(headerForwardedFor)); forwardedFor != "" {
		forwardedIPs := strings.Split(forwardedFor, ",")
		leftmostValid := ""

		for _, forwardedIP := range forwardedIPs {
			parsedIP := net.ParseIP(strings.TrimSpace(forwardedIP))
			if parsedIP == nil {
				continue
			}
			leftmostValid = parsedIP.String()
			break
		}

		for i := len(forwardedIPs) - 1; i >= 0; i-- {
			parsedIP := net.ParseIP(strings.TrimSpace(forwardedIPs[i]))
			if parsedIP == nil {
				continue
			}
			if !ipInNets(parsedIP, trustedProxies) {
				return parsedIP.String()
			}
		}

		if leftmostValid != "" {
			return leftmostValid
		}
	}

	if realIP := net.ParseIP(strings.TrimSpace(r.Header.Get(headerRealIP))); realIP != nil {
		return realIP.String()
	}

	if directAddr != "" {
		return directAddr
	}

	return "unknown"
}

func directClientIP(remoteAddr string) (string, net.IP) {
	remoteAddr = strings.TrimSpace(remoteAddr)
	if remoteAddr == "" {
		return "", nil
	}

	host, _, err := net.SplitHostPort(remoteAddr)
	if err == nil {
		if ip := net.ParseIP(host); ip != nil {
			return ip.String(), ip
		}
		return host, nil
	}

	if ip := net.ParseIP(remoteAddr); ip != nil {
		return ip.String(), ip
	}

	return remoteAddr, nil
}

func ipInNets(ip net.IP, nets []*net.IPNet) bool {
	for _, n := range nets {
		if n.Contains(ip) {
			return true
		}
	}

	return false
}

func secondsUntil(windowEnd, now time.Time) int {
	remaining := windowEnd.Sub(now)
	if remaining <= 0 {
		return 1
	}

	seconds := int(remaining / time.Second)
	if remaining%time.Second != 0 {
		seconds++
	}
	if seconds < 1 {
		return 1
	}

	return seconds
}

func writeRateLimitExceeded(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "application/problem+json")
	w.WriteHeader(http.StatusTooManyRequests)
	_, _ = w.Write([]byte(rateLimitProblemResponse))
}
