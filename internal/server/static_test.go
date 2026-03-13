package server

import (
	"io/fs"
	"net/http"
	"net/http/httptest"
	"testing"
	"testing/fstest"

	"github.com/stretchr/testify/require"
)

func TestNewStaticHandler(t *testing.T) {
	t.Parallel()

	assets := fstest.MapFS{
		"index.html":        {Data: []byte("<html>spa</html>")},
		"favicon.svg":       {Data: []byte("<svg></svg>")},
		"_app/env.js":       {Data: []byte("console.log('env')")},
		"nested/route.html": {Data: []byte("<html>nested</html>")},
	}

	root, err := fs.Sub(assets, ".")
	require.NoError(t, err)

	h := newStaticHandler(root)

	t.Run("serves existing asset", func(t *testing.T) {
		t.Parallel()

		req := httptest.NewRequest(http.MethodGet, "/favicon.svg", http.NoBody)
		res := httptest.NewRecorder()

		h.ServeHTTP(res, req)

		require.Equal(t, http.StatusOK, res.Code)
		require.Contains(t, res.Body.String(), "<svg>")
	})

	t.Run("falls back to index for spa route", func(t *testing.T) {
		t.Parallel()

		req := httptest.NewRequest(http.MethodGet, "/projects/alpha", http.NoBody)
		res := httptest.NewRecorder()

		h.ServeHTTP(res, req)

		require.Equal(t, http.StatusOK, res.Code)
		require.Contains(t, res.Body.String(), "spa")
	})

	t.Run("does not intercept api routes", func(t *testing.T) {
		t.Parallel()

		req := httptest.NewRequest(http.MethodGet, "/api/v1/tasks", http.NoBody)
		res := httptest.NewRecorder()

		h.ServeHTTP(res, req)

		require.Equal(t, http.StatusNotFound, res.Code)
	})

	t.Run("does not intercept slack routes", func(t *testing.T) {
		t.Parallel()

		req := httptest.NewRequest(http.MethodGet, "/slack/events", http.NoBody)
		res := httptest.NewRecorder()

		h.ServeHTTP(res, req)

		require.Equal(t, http.StatusNotFound, res.Code)
	})

	t.Run("does not intercept discord routes", func(t *testing.T) {
		t.Parallel()

		req := httptest.NewRequest(http.MethodGet, "/discord/webhook", http.NoBody)
		res := httptest.NewRecorder()

		h.ServeHTTP(res, req)

		require.Equal(t, http.StatusNotFound, res.Code)
	})

	t.Run("does not intercept telegram routes", func(t *testing.T) {
		t.Parallel()

		req := httptest.NewRequest(http.MethodGet, "/telegram/webhook", http.NoBody)
		res := httptest.NewRecorder()

		h.ServeHTTP(res, req)

		require.Equal(t, http.StatusNotFound, res.Code)
	})
}
