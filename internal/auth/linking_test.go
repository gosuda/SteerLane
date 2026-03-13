package auth

import (
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/gosuda/steerlane/internal/domain"
)

func TestLinkingService(t *testing.T) {
	t.Parallel()

	service := NewLinkingService("link-secret", "https://steerlane.example.com", time.Hour)
	tenantID := uuid.MustParse("10000000-0000-0000-0000-000000000001")

	t.Run("issues and parses token", func(t *testing.T) {
		t.Parallel()

		token, err := service.IssueToken(tenantID, "slack", "U123")
		require.NoError(t, err)

		claims, err := service.ParseToken(token)
		require.NoError(t, err)
		require.Equal(t, tenantID, claims.TenantID)
		require.Equal(t, "slack", claims.Platform)
		require.Equal(t, "U123", claims.ExternalUserID)
	})

	t.Run("generates browser URL", func(t *testing.T) {
		t.Parallel()

		link, err := service.GenerateLink(tenantID, "slack", "U123")
		require.NoError(t, err)
		require.True(t, strings.HasPrefix(link, "https://steerlane.example.com/auth/link?token="))

		parsed, err := url.Parse(link)
		require.NoError(t, err)
		require.NotEmpty(t, parsed.Query().Get("token"))
	})

	t.Run("rejects tampered token", func(t *testing.T) {
		t.Parallel()

		token, err := service.IssueToken(tenantID, "slack", "U123")
		require.NoError(t, err)

		_, err = service.ParseToken(token + "tampered")
		require.ErrorIs(t, err, ErrInvalidToken)
	})

	t.Run("rejects expired token", func(t *testing.T) {
		t.Parallel()

		expired := NewLinkingService("link-secret", "https://steerlane.example.com", time.Nanosecond)
		token, err := expired.IssueToken(tenantID, "slack", "U123")
		require.NoError(t, err)

		time.Sleep(time.Millisecond)
		_, err = expired.ParseToken(token)
		require.ErrorIs(t, err, ErrInvalidToken)
	})

	t.Run("rejects missing platform", func(t *testing.T) {
		t.Parallel()

		_, err := service.IssueToken(domain.NewID(), "", "U123")
		require.ErrorIs(t, err, domain.ErrInvalidInput)
	})
}
