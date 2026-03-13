package opencode

import (
	"io"
	"log/slog"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	agentpkg "github.com/gosuda/steerlane/internal/agent"
	domainagent "github.com/gosuda/steerlane/internal/domain/agent"
	"github.com/gosuda/steerlane/internal/testutil"
)

func testOpenCodeLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func testOpenCodeSessionOpts() agentpkg.SessionOpts {
	return agentpkg.SessionOpts{
		SessionID:  testutil.TestSessionID(),
		ProjectID:  testutil.TestProjectID(),
		TaskID:     testutil.TestTaskID(),
		TenantID:   testutil.TestTenantID(),
		Prompt:     "Implement retries",
		RepoPath:   "/tmp/repo",
		BranchName: "steerlane/test-session",
	}
}

func TestBackendLifecycle(t *testing.T) {
	t.Parallel()

	backend, err := NewBackend(testOpenCodeLogger())
	require.NoError(t, err)

	require.ErrorContains(t, backend.SendPrompt(t.Context(), "continue"), "session not started")
	require.NoError(t, backend.StartSession(t.Context(), testOpenCodeSessionOpts()))
	require.NoError(t, backend.SendPrompt(t.Context(), "continue"))
	require.NoError(t, backend.Cancel(t.Context()))
	require.ErrorContains(t, backend.SendPrompt(t.Context(), "continue"), "session not active")
	require.NoError(t, backend.Dispose())
	require.NoError(t, backend.Dispose())
}

func TestRegisterDefault(t *testing.T) {
	t.Parallel()

	registry := agentpkg.NewRegistry()
	RegisterDefault(registry)

	assert.True(t, registry.Has(domainagent.TypeOpenCode))

	backend, err := registry.Create(domainagent.TypeOpenCode, testOpenCodeLogger())
	require.NoError(t, err)
	assert.IsType(t, &Backend{}, backend)
}
