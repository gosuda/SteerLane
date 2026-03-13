package messenger

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestParseCommand(t *testing.T) {
	t.Parallel()

	t.Run("parses title and description from multiline mention", func(t *testing.T) {
		t.Parallel()

		got, err := ParseCommand("<@U123> fix auth bug\nUsers cannot refresh tokens after login")
		require.NoError(t, err)
		require.Equal(t, "fix auth bug", got.Title)
		require.Equal(t, "Users cannot refresh tokens after login", got.Description)
	})

	t.Run("returns empty command error when mention has no content", func(t *testing.T) {
		t.Parallel()

		_, err := ParseCommand("<@U123>   ")
		require.ErrorIs(t, err, ErrEmptyCommand)
	})

	t.Run("splits long single line at word boundary", func(t *testing.T) {
		t.Parallel()

		text := "<@U123> " + strings.Repeat("word ", 60)
		got, err := ParseCommand(text)
		require.NoError(t, err)
		require.NotEmpty(t, got.Title)
		require.NotEmpty(t, got.Description)
		require.LessOrEqual(t, len(got.Title), maxTitleLen)
	})
}
