package messenger

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/gosuda/steerlane/internal/testutil"
)

func TestSessionContextRegistry(t *testing.T) {
	t.Parallel()

	registry := NewSessionContextRegistry()
	tenantID := testutil.TestTenantID()
	sessionID := testutil.TestSessionID()
	want := SessionContext{
		Platform:        PlatformSlack,
		ChannelID:       "C123",
		ParentMessageID: "1710000000.000100",
	}

	_, ok := registry.Get(tenantID, sessionID)
	require.False(t, ok)

	registry.Put(tenantID, sessionID, want)
	got, ok := registry.Get(tenantID, sessionID)
	require.True(t, ok)
	require.Equal(t, want, got)

	registry.Delete(tenantID, sessionID)
	_, ok = registry.Get(tenantID, sessionID)
	require.False(t, ok)
}
