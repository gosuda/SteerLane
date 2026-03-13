package slack

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestBuildLinkingDM(t *testing.T) {
	t.Parallel()

	text := BuildLinkingDM("https://steerlane.example.com/auth/link?token=abc")
	require.Contains(t, text, "https://steerlane.example.com/auth/link?token=abc")
	require.Contains(t, text, "Connect your Slack identity")

	empty := BuildLinkingDM("")
	require.Contains(t, empty, "Connect your Slack identity")
	require.NotContains(t, empty, "token=")
}
