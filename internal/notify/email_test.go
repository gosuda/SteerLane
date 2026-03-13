package notify

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/gosuda/steerlane/internal/config"
)

func TestNewEmailSenderDisabledWithoutConfig(t *testing.T) {
	t.Parallel()

	require.Nil(t, NewEmailSender(config.EmailConfig{}))
	require.Nil(t, NewEmailSender(config.EmailConfig{Enabled: true}))
}

func TestBuildEmailMessage(t *testing.T) {
	t.Parallel()

	msg := string(buildEmailMessage("noreply@example.com", EmailPayload{
		To:      "user@example.com",
		Subject: "SteerLane notification",
		Body:    "Task completed.",
	}))

	require.Contains(t, msg, "From: noreply@example.com")
	require.Contains(t, msg, "To: user@example.com")
	require.Contains(t, msg, "Subject: SteerLane notification")
	require.Contains(t, msg, "Content-Type: text/plain; charset=UTF-8")
	require.Contains(t, msg, "\r\n\r\nTask completed.")
}
