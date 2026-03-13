package claude

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

func testClaudeLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func testSessionOpts() agentpkg.SessionOpts {
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

func TestBackend_SendPromptRequiresActiveSession(t *testing.T) {
	t.Parallel()

	//nolint:govet // test case shape prioritizes readability over field packing.
	tests := []struct {
		name         string
		setup        func(t *testing.T, backend *Backend)
		wantContains string
	}{
		{
			name:         "session must be started first",
			setup:        func(*testing.T, *Backend) {},
			wantContains: "session not started",
		},
		{
			name: "cancelled session is not active",
			setup: func(t *testing.T, backend *Backend) {
				t.Helper()
				require.NoError(t, backend.StartSession(t.Context(), testSessionOpts()))
				require.NoError(t, backend.Cancel(t.Context()))
			},
			wantContains: "session not active",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			backend, err := NewBackend(testClaudeLogger())
			require.NoError(t, err)

			tt.setup(t, backend)

			err = backend.SendPrompt(t.Context(), "answer")

			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.wantContains)
		})
	}
}

func TestBackend_StartSession_AllowsSingleStart(t *testing.T) {
	t.Parallel()

	backend, err := NewBackend(testClaudeLogger())
	require.NoError(t, err)

	require.NoError(t, backend.StartSession(t.Context(), testSessionOpts()))
	require.NoError(t, backend.SendPrompt(t.Context(), "continue"))

	err = backend.StartSession(t.Context(), testSessionOpts())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "session already started")
}

func TestBackend_CancelAndDispose(t *testing.T) {
	t.Parallel()

	//nolint:govet // test case shape prioritizes readability over field packing.
	tests := []struct {
		name string
		run  func(t *testing.T, backend *Backend)
	}{
		{
			name: "cancel before start is a no-op",
			run: func(t *testing.T, backend *Backend) {
				t.Helper()
				require.NoError(t, backend.Cancel(t.Context()))
			},
		},
		{
			name: "cancel after start succeeds",
			run: func(t *testing.T, backend *Backend) {
				t.Helper()
				require.NoError(t, backend.StartSession(t.Context(), testSessionOpts()))
				require.NoError(t, backend.Cancel(t.Context()))
			},
		},
		{
			name: "dispose is idempotent",
			run: func(t *testing.T, backend *Backend) {
				t.Helper()
				require.NoError(t, backend.Dispose())
				require.NoError(t, backend.Dispose())
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			backend, err := NewBackend(testClaudeLogger())
			require.NoError(t, err)

			tt.run(t, backend)
		})
	}
}

func TestRegisterDefault(t *testing.T) {
	t.Parallel()

	registry := agentpkg.NewRegistry()
	RegisterDefault(registry)

	assert.True(t, registry.Has(domainagent.TypeClaude))

	backend, err := registry.Create(domainagent.TypeClaude, testClaudeLogger())
	require.NoError(t, err)
	assert.IsType(t, &Backend{}, backend)
}
