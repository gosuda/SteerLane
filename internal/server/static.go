package server

import (
	"io/fs"
	"net/http"
	"strings"
)

// newStaticHandler returns an http.Handler that serves embedded SPA assets.
//
// Behaviour:
//  1. Requests whose path starts with an API prefix are passed through to the
//     next handler (the mux's default 404, effectively a no-op since those
//     routes are already registered).
//  2. If the request matches an existing file in the embedded filesystem,
//     it is served directly with the standard library's FileServer.
//  3. Otherwise the handler serves index.html — the SPA entry point — so that
//     client-side routing can resolve the path.
//
// The assets parameter must be a sub-filesystem rooted at the build output
// directory (e.g. via fs.Sub(web.Build, "build")).
func newStaticHandler(assets fs.FS) http.Handler {
	fileServer := http.FileServer(http.FS(assets))

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Do not serve static files for API/WS/system paths.
		if isReservedPath(r.URL.Path) {
			http.NotFound(w, r)
			return
		}

		// Try to open the requested file. If it exists, let FileServer
		// handle content-type detection and range requests.
		path := strings.TrimPrefix(r.URL.Path, "/")
		if path == "" {
			path = "index.html"
		}

		if _, err := fs.Stat(assets, path); err == nil {
			fileServer.ServeHTTP(w, r)
			return
		}

		// SPA fallback: serve index.html for any unmatched route so the
		// client-side router can handle it.
		r.URL.Path = "/"
		fileServer.ServeHTTP(w, r)
	})
}

func isReservedPath(path string) bool {
	switch {
	case strings.HasPrefix(path, "/api/"), path == "/api":
		return true
	case strings.HasPrefix(path, "/ws/"), path == "/ws":
		return true
	case strings.HasPrefix(path, "/slack/"), path == "/slack":
		return true
	case strings.HasPrefix(path, "/discord/"), path == "/discord":
		return true
	case strings.HasPrefix(path, "/telegram/"), path == "/telegram":
		return true
	case path == "/healthz", path == "/openapi.json", path == "/docs":
		return true
	default:
		return false
	}
}
