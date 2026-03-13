package discord

import (
	"bytes"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestHandlerPing(t *testing.T) {
	t.Parallel()
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)
	h, err := NewHandler(nil, hex.EncodeToString(pub), nil)
	require.NoError(t, err)
	body := []byte(`{"id":"1","token":"abc","type":1}`)
	ts := "1710000000"
	sig := ed25519.Sign(priv, append([]byte(ts), body...))
	req := httptest.NewRequest(http.MethodPost, "/discord/webhook", bytes.NewReader(body))
	req.Header.Set("X-Signature-Ed25519", hex.EncodeToString(sig))
	req.Header.Set("X-Signature-Timestamp", ts)
	res := httptest.NewRecorder()
	h.ServeHTTP(res, req)
	require.Equal(t, http.StatusOK, res.Code)
	require.Contains(t, res.Body.String(), `"type":1`)
}
