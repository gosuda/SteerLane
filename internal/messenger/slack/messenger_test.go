package slack

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/gosuda/steerlane/internal/messenger"
)

func TestMessengerSendMessage(t *testing.T) {
	t.Parallel()

	server := newSlackTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/chat.postMessage" {
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer xoxb-test" {
			t.Fatalf("unexpected authorization %q", got)
		}
		if got := r.Header.Get("Content-Type"); got != "application/json" {
			t.Fatalf("unexpected content type %q", got)
		}

		var req map[string]any
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		if got := req["channel"]; got != "C123" {
			t.Fatalf("unexpected channel %#v", got)
		}
		if got := req["text"]; got != "hello" {
			t.Fatalf("unexpected text %#v", got)
		}
		expectedBlocks := []any{map[string]any{"type": "section", "text": map[string]any{"type": "mrkdwn", "text": "hello"}}}
		if !reflect.DeepEqual(req["blocks"], expectedBlocks) {
			t.Fatalf("unexpected blocks %#v", req["blocks"])
		}

		writeJSON(t, w, map[string]any{"ok": true, "channel": "C123", "ts": "171234.000100"})
	})

	defer server.Close()

	m := NewMessenger("xoxb-test", WithBaseURL(server.URL), WithHTTPClient(server.Client()))
	result, err := m.SendMessage(t.Context(), messenger.SendMessageParams{
		ChannelID: "C123",
		Text:      "hello",
		Blocks:    []byte(`[{"type":"section","text":{"type":"mrkdwn","text":"hello"}}]`),
	})
	require.NoError(t, err)
	require.Equal(t, messenger.MessageResult{MessageID: "171234.000100", ThreadID: "171234.000100"}, result)
}

func TestMessengerCreateThread(t *testing.T) {
	t.Parallel()

	server := newSlackTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/chat.postMessage" {
			t.Fatalf("unexpected path %s", r.URL.Path)
		}

		var req map[string]any
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		if got := req["channel"]; got != "C123" {
			t.Fatalf("unexpected channel %#v", got)
		}
		if got := req["text"]; got != "follow-up" {
			t.Fatalf("unexpected text %#v", got)
		}
		if got := req["thread_ts"]; got != "171200.000001" {
			t.Fatalf("unexpected thread_ts %#v", got)
		}

		writeJSON(t, w, map[string]any{"ok": true, "channel": "C123", "ts": "171234.000101"})
	})

	defer server.Close()

	m := NewMessenger("xoxb-test", WithBaseURL(server.URL), WithHTTPClient(server.Client()))
	result, err := m.CreateThread(t.Context(), messenger.CreateThreadParams{
		ChannelID:       "C123",
		ParentMessageID: "171200.000001",
		Text:            "follow-up",
	})
	require.NoError(t, err)
	require.Equal(t, messenger.MessageResult{MessageID: "171234.000101", ThreadID: "171200.000001"}, result)
}

func TestMessengerUpdateMessage(t *testing.T) {
	t.Parallel()

	server := newSlackTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/chat.update" {
			t.Fatalf("unexpected path %s", r.URL.Path)
		}

		var req map[string]any
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		if got := req["channel"]; got != "C123" {
			t.Fatalf("unexpected channel %#v", got)
		}
		if got := req["ts"]; got != "171234.000102" {
			t.Fatalf("unexpected ts %#v", got)
		}
		if got := req["text"]; got != "updated" {
			t.Fatalf("unexpected text %#v", got)
		}

		writeJSON(t, w, map[string]any{"ok": true, "channel": "C123", "ts": "171234.000102"})
	})

	defer server.Close()

	m := NewMessenger("xoxb-test", WithBaseURL(server.URL), WithHTTPClient(server.Client()))
	err := m.UpdateMessage(t.Context(), messenger.UpdateMessageParams{
		ChannelID: "C123",
		MessageID: "171234.000102",
		Text:      "updated",
	})
	require.NoError(t, err)
}

func TestMessengerSendNotification(t *testing.T) {
	t.Parallel()

	var calls []string
	server := newSlackTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		calls = append(calls, r.URL.Path)

		var req map[string]any
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("decode request: %v", err)
		}

		switch r.URL.Path {
		case "/conversations.open":
			if got := req["users"]; got != "U123" {
				t.Fatalf("unexpected users %#v", got)
			}
			writeJSON(t, w, map[string]any{"ok": true, "channel": map[string]any{"id": "D123"}})
		case "/chat.postMessage":
			if got := req["channel"]; got != "D123" {
				t.Fatalf("unexpected channel %#v", got)
			}
			if got := req["text"]; got != "notify" {
				t.Fatalf("unexpected text %#v", got)
			}
			expectedBlocks := []any{map[string]any{"type": "section", "text": map[string]any{"type": "mrkdwn", "text": "notify"}}}
			if !reflect.DeepEqual(req["blocks"], expectedBlocks) {
				t.Fatalf("unexpected blocks %#v", req["blocks"])
			}
			writeJSON(t, w, map[string]any{"ok": true, "channel": "D123", "ts": "171234.000103"})
		default:
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
	})

	defer server.Close()

	m := NewMessenger("xoxb-test", WithBaseURL(server.URL), WithHTTPClient(server.Client()))
	err := m.SendNotification(t.Context(), messenger.NotificationParams{
		UserExternalID: "U123",
		Text:           "notify",
		Blocks:         []byte(`[{"type":"section","text":{"type":"mrkdwn","text":"notify"}}]`),
	})
	require.NoError(t, err)
	require.Equal(t, []string{"/conversations.open", "/chat.postMessage"}, calls)
}

func TestMessengerSendMessageSlackError(t *testing.T) {
	t.Parallel()

	server := newSlackTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/chat.postMessage" {
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
		writeJSON(t, w, map[string]any{"ok": false, "error": "channel_not_found"})
	})

	defer server.Close()

	m := NewMessenger("xoxb-test", WithBaseURL(server.URL), WithHTTPClient(server.Client()))
	_, err := m.SendMessage(t.Context(), messenger.SendMessageParams{ChannelID: "C404", Text: "hello"})
	require.Error(t, err)
	require.ErrorContains(t, err, "channel_not_found")
}

func TestMessengerPlatform(t *testing.T) {
	t.Parallel()

	require.Equal(t, messenger.PlatformSlack, NewMessenger("token").Platform())
}

func newSlackTestServer(t *testing.T, handler func(w http.ResponseWriter, r *http.Request)) *httptest.Server {
	t.Helper()

	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Fatalf("unexpected method %s", r.Method)
		}
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("read body: %v", err)
		}
		if err = r.Body.Close(); err != nil {
			t.Fatalf("close body: %v", err)
		}
		r.Body = io.NopCloser(bytes.NewReader(body))
		handler(w, r)
	}))
}

func writeJSON(t *testing.T, w http.ResponseWriter, body map[string]any) {
	t.Helper()

	w.Header().Set("Content-Type", "application/json")
	require.NoError(t, json.NewEncoder(w).Encode(body))
}
