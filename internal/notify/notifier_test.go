package notify

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/gosuda/steerlane/internal/messenger"
)

type mockEmailSender struct { //nolint:govet // test readability over field packing
	payloads []EmailPayload
	sendFn   func(context.Context, EmailPayload) error
}

func (m *mockEmailSender) Send(ctx context.Context, payload EmailPayload) error {
	m.payloads = append(m.payloads, payload)
	if m.sendFn != nil {
		return m.sendFn(ctx, payload)
	}
	return nil
}

type mockMessenger struct { //nolint:govet // fieldalignment: test mock
	notifications  []messenger.NotificationParams
	threads        []messenger.CreateThreadParams
	platform       messenger.Platform
	notificationFn func(ctx context.Context, params messenger.NotificationParams) error
	threadFn       func(ctx context.Context, params messenger.CreateThreadParams) (messenger.MessageResult, error)
}

func (m *mockMessenger) SendMessage(context.Context, messenger.SendMessageParams) (messenger.MessageResult, error) {
	return messenger.MessageResult{}, nil
}

func (m *mockMessenger) CreateThread(ctx context.Context, params messenger.CreateThreadParams) (messenger.MessageResult, error) {
	m.threads = append(m.threads, params)
	if m.threadFn != nil {
		return m.threadFn(ctx, params)
	}
	return messenger.MessageResult{}, nil
}

func (m *mockMessenger) UpdateMessage(context.Context, messenger.UpdateMessageParams) error {
	return nil
}

func (m *mockMessenger) SendNotification(ctx context.Context, params messenger.NotificationParams) error {
	m.notifications = append(m.notifications, params)
	if m.notificationFn != nil {
		return m.notificationFn(ctx, params)
	}
	return nil
}

func (m *mockMessenger) Platform() messenger.Platform {
	if m.platform != "" {
		return m.platform
	}
	return messenger.PlatformSlack
}

func decodeNotificationBlocks(t *testing.T, data []byte) []map[string]any {
	t.Helper()

	var blocks []map[string]any
	require.NoError(t, json.Unmarshal(data, &blocks))
	return blocks
}

func testLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func TestNotifierSendTaskCompleted(t *testing.T) {
	t.Parallel()

	messengerMock := &mockMessenger{}
	notifier := New(testLogger(), messengerMock)

	err := notifier.SendTaskCompleted(context.Background(), TaskCompletedPayload{
		UserExternalID: "U123",
		TaskID:         "task-42",
		TaskTitle:      "Fix flaky CI",
		SessionID:      "session-7",
	})
	require.NoError(t, err)
	require.Len(t, messengerMock.notifications, 1)
	require.Equal(t, "U123", messengerMock.notifications[0].UserExternalID)
	require.Equal(t, "Task completed: Fix flaky CI (task-42). Session session-7 finished successfully.", messengerMock.notifications[0].Text)
	blocks := decodeNotificationBlocks(t, messengerMock.notifications[0].Blocks)
	require.Len(t, blocks, 3)
	require.Equal(t, "header", blocks[0]["type"])
}

func TestNotifierSendSessionFailed(t *testing.T) {
	t.Parallel()

	messengerMock := &mockMessenger{}
	notifier := New(testLogger(), messengerMock)

	err := notifier.SendSessionFailed(context.Background(), SessionFailedPayload{
		UserExternalID: "U456",
		TaskID:         "task-99",
		TaskTitle:      "Ship notifier",
		SessionID:      "session-9",
		Reason:         "agent runtime exited unexpectedly",
	})
	require.NoError(t, err)
	require.Len(t, messengerMock.notifications, 1)
	require.Equal(t, "U456", messengerMock.notifications[0].UserExternalID)
	require.Equal(t, "Session failed for Ship notifier (task-99). Session session-9 ended with an error. Reason: agent runtime exited unexpectedly", messengerMock.notifications[0].Text)
	blocks := decodeNotificationBlocks(t, messengerMock.notifications[0].Blocks)
	require.Len(t, blocks, 2)
	require.Equal(t, "section", blocks[0]["type"])
}

func TestNotifierSendHITLTimedOut(t *testing.T) {
	t.Parallel()

	messengerMock := &mockMessenger{}
	notifier := New(testLogger(), messengerMock)

	err := notifier.SendHITLTimedOut(context.Background(), HITLTimedOutPayload{
		UserExternalID: "U789",
		TaskID:         "task-11",
		TaskTitle:      "Review ADR",
		SessionID:      "session-3",
		Question:       "Should the rollout proceed?",
	})
	require.NoError(t, err)
	require.Len(t, messengerMock.notifications, 1)
	require.Equal(t, "U789", messengerMock.notifications[0].UserExternalID)
	require.Equal(t, "Human input timed out for Review ADR (task-11). Session session-3 is waiting for follow-up. Unanswered question: Should the rollout proceed?", messengerMock.notifications[0].Text)
	blocks := decodeNotificationBlocks(t, messengerMock.notifications[0].Blocks)
	require.Len(t, blocks, 2)
	require.Equal(t, "section", blocks[0]["type"])
}

func TestNotifierSkipsSlackBlocksForNonSlackMessenger(t *testing.T) {
	t.Parallel()

	messengerMock := &mockMessenger{platform: messenger.PlatformDiscord}
	notifier := New(testLogger(), messengerMock)

	err := notifier.SendTaskCompleted(context.Background(), TaskCompletedPayload{
		UserExternalID: "U123",
		TaskID:         "task-42",
		TaskTitle:      "Fix flaky CI",
		SessionID:      "session-7",
	})
	require.NoError(t, err)
	require.Len(t, messengerMock.notifications, 1)
	require.Nil(t, messengerMock.notifications[0].Blocks)
}

func TestNotifierSendNotificationError(t *testing.T) {
	t.Parallel()

	wantErr := errors.New("boom")
	messengerMock := &mockMessenger{
		notificationFn: func(context.Context, messenger.NotificationParams) error {
			return wantErr
		},
	}
	notifier := New(testLogger(), messengerMock)

	err := notifier.SendSessionFailed(context.Background(), SessionFailedPayload{UserExternalID: "U1"})
	require.Error(t, err)
	require.ErrorIs(t, err, wantErr)
	require.ErrorContains(t, err, "notify.Notifier.SendSessionFailed")
}

func TestNotifierRequiresMessenger(t *testing.T) {
	t.Parallel()

	notifier := New(testLogger(), nil)

	err := notifier.SendTaskCompleted(context.Background(), TaskCompletedPayload{UserExternalID: "U1"})
	require.Error(t, err)
	require.ErrorContains(t, err, "messenger not configured")
}

func TestNotifierFallsBackToEmailWhenMessengerFails(t *testing.T) {
	t.Parallel()

	messengerMock := &mockMessenger{notificationFn: func(context.Context, messenger.NotificationParams) error {
		return errors.New("messenger down")
	}}
	emailMock := &mockEmailSender{}
	notifier := NewWithEmail(testLogger(), messengerMock, emailMock)

	err := notifier.SendTaskCompleted(context.Background(), TaskCompletedPayload{
		UserExternalID: "U123",
		FallbackEmail:  "user@example.com",
		TaskID:         "task-42",
		TaskTitle:      "Fix flaky CI",
		SessionID:      "session-7",
	})
	require.NoError(t, err)
	require.Len(t, emailMock.payloads, 1)
	require.Equal(t, "user@example.com", emailMock.payloads[0].To)
	require.Contains(t, emailMock.payloads[0].Body, "Task completed")
}
