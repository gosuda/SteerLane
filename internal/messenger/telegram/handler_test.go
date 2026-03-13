package telegram

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestHandlerSecretVerification(t *testing.T) {
	t.Parallel()
	h := NewHandler(nil, "secret", nil)
	req := httptest.NewRequest(http.MethodPost, "/telegram/webhook", bytes.NewReader([]byte(`{"update_id":1}`)))
	res := httptest.NewRecorder()
	h.ServeHTTP(res, req)
	require.Equal(t, http.StatusUnauthorized, res.Code)

	req = httptest.NewRequest(http.MethodPost, "/telegram/webhook", bytes.NewReader([]byte(`{"update_id":1}`)))
	req.Header.Set("X-Telegram-Bot-Api-Secret-Token", "secret")
	res = httptest.NewRecorder()
	h.ServeHTTP(res, req)
	require.Equal(t, http.StatusOK, res.Code)
}
