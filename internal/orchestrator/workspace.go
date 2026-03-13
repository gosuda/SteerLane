package orchestrator

import (
	"encoding/json"
	"log/slog"

	"github.com/gosuda/steerlane/internal/domain/project"
)

func buildWorkspaceEnv(proj *project.Project) map[string]string {
	if proj == nil {
		return nil
	}
	env := make(map[string]string)
	workspacePath, err := proj.WorkspacePath()
	if err == nil && workspacePath != "" {
		env["STEERLANE_WORKSPACE_PATH"] = workspacePath
	}
	workspacePaths, err := proj.WorkspacePaths()
	if err == nil && len(workspacePaths) != 0 {
		if encoded, marshalErr := json.Marshal(workspacePaths); marshalErr == nil {
			env["STEERLANE_WORKSPACE_PATHS"] = string(encoded)
		} else {
			slog.Default().Warn("failed to marshal workspace paths", "error", marshalErr)
		}
	}
	if len(env) == 0 {
		return nil
	}
	return env
}
