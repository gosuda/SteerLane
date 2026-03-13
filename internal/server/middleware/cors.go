package middleware

import (
	"net/http"
	"strconv"
)

// CORSConfig holds configuration for the CORS middleware.
type CORSConfig struct {
	// AllowedOrigins is the list of origins permitted to make cross-origin requests.
	// Use ["*"] to allow all origins (not recommended for production with credentials).
	AllowedOrigins []string

	// MaxAge is the value for Access-Control-Max-Age in seconds.
	MaxAge int
}

// allowedOriginSet builds a set for O(1) lookup. Returns nil if wildcard is present.
func (c CORSConfig) allowedOriginSet() map[string]struct{} {
	set := make(map[string]struct{}, len(c.AllowedOrigins))
	for _, o := range c.AllowedOrigins {
		if o == "*" {
			return nil // wildcard mode
		}
		set[o] = struct{}{}
	}
	return set
}

const (
	corsAllowedMethods = "GET, POST, PUT, PATCH, DELETE, OPTIONS"
	corsAllowedHeaders = "Accept, Authorization, Content-Type, X-API-Key, X-Request-ID"
)

// CORS returns middleware that handles Cross-Origin Resource Sharing.
// It responds to preflight OPTIONS requests and sets the appropriate headers.
func CORS(cfg CORSConfig) func(http.Handler) http.Handler {
	originSet := cfg.allowedOriginSet()
	maxAge := strconv.Itoa(cfg.MaxAge)

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			origin := r.Header.Get("Origin")
			if origin == "" {
				next.ServeHTTP(w, r)
				return
			}

			allowed := false
			if originSet == nil {
				// Wildcard mode.
				allowed = true
				w.Header().Set("Access-Control-Allow-Origin", "*")
			} else if _, ok := originSet[origin]; ok {
				allowed = true
				w.Header().Set("Access-Control-Allow-Origin", origin)
				w.Header().Set("Vary", "Origin")
			}

			if !allowed {
				next.ServeHTTP(w, r)
				return
			}

			w.Header().Set("Access-Control-Allow-Methods", corsAllowedMethods)
			w.Header().Set("Access-Control-Allow-Headers", corsAllowedHeaders)
			w.Header().Set("Access-Control-Allow-Credentials", "true")

			if cfg.MaxAge > 0 {
				w.Header().Set("Access-Control-Max-Age", maxAge)
			}

			// Handle preflight.
			if r.Method == http.MethodOptions && r.Header.Get("Access-Control-Request-Method") != "" {
				w.WriteHeader(http.StatusNoContent)
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}
