package project

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/gosuda/steerlane/internal/domain"
)

func TestProjectWorkspaceSettings(t *testing.T) {
	t.Parallel()

	p := &Project{ID: domain.NewID(), TenantID: domain.NewID(), Name: "App", RepoURL: "https://example.com/repo.git", Branch: "main", Settings: map[string]any{"workspace_path": "apps/web", "workspace_paths": []any{"apps/web", "libs/shared"}}}
	require.NoError(t, p.Validate())
	workspace, err := p.WorkspacePath()
	require.NoError(t, err)
	require.Equal(t, "apps/web", workspace)
	paths, err := p.WorkspacePaths()
	require.NoError(t, err)
	require.Equal(t, []string{"apps/web", "libs/shared"}, paths)
	resolved, err := p.ResolveWorkspace("/mnt/volumes/project-volume")
	require.NoError(t, err)
	require.Equal(t, "/mnt/volumes/project-volume/apps/web", resolved)
}

func TestProjectWorkspacePathRejectsTraversal(t *testing.T) {
	t.Parallel()
	p := &Project{Name: "App", Settings: map[string]any{"workspace_path": "../secret"}}
	require.ErrorIs(t, p.Validate(), domain.ErrInvalidInput)
}
