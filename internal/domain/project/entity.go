package project

import (
	"fmt"
	"path"
	"path/filepath"
	"strings"
	"time"

	"github.com/gosuda/steerlane/internal/domain"
)

const (
	settingWorkspacePath  = "workspace_path"
	settingWorkspacePaths = "workspace_paths"
)

// Project represents a code repository being managed on the kanban board.
type Project struct {
	CreatedAt time.Time
	Settings  map[string]any
	Name      string
	RepoURL   string
	Branch    string
	ID        domain.ProjectID
	TenantID  domain.TenantID
}

// Validate checks that the project's fields are well-formed.
func (p *Project) Validate() error {
	if p.Name == "" {
		return fmt.Errorf("project name: %w", domain.ErrInvalidInput)
	}
	if _, err := p.WorkspacePath(); err != nil {
		return err
	}
	if _, err := p.WorkspacePaths(); err != nil {
		return err
	}
	return nil
}

func (p *Project) WorkspacePath() (string, error) {
	if p == nil || p.Settings == nil {
		return "", nil
	}
	raw, ok := p.Settings[settingWorkspacePath]
	if !ok || raw == nil {
		return "", nil
	}
	value, ok := raw.(string)
	if !ok {
		return "", fmt.Errorf("project workspace_path: %w", domain.ErrInvalidInput)
	}
	return normalizeWorkspacePath(value)
}

func (p *Project) WorkspacePaths() ([]string, error) {
	if p == nil || p.Settings == nil {
		return nil, nil
	}
	raw, ok := p.Settings[settingWorkspacePaths]
	if !ok || raw == nil {
		return nil, nil
	}
	items, ok := raw.([]any)
	if !ok {
		return nil, fmt.Errorf("project workspace_paths: %w", domain.ErrInvalidInput)
	}
	paths := make([]string, 0, len(items))
	for _, item := range items {
		value, isString := item.(string)
		if !isString {
			return nil, fmt.Errorf("project workspace_paths: %w", domain.ErrInvalidInput)
		}
		normalized, err := normalizeWorkspacePath(value)
		if err != nil {
			return nil, err
		}
		if normalized != "" {
			paths = append(paths, normalized)
		}
	}
	return paths, nil
}

func (p *Project) ResolveWorkspace(baseRepoDir string) (string, error) {
	workspacePath, err := p.WorkspacePath()
	if err != nil {
		return "", err
	}
	if workspacePath == "" {
		return baseRepoDir, nil
	}
	return filepath.Join(baseRepoDir, filepath.FromSlash(workspacePath)), nil
}

func normalizeWorkspacePath(raw string) (string, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return "", nil
	}
	if strings.HasPrefix(trimmed, "/") {
		return "", fmt.Errorf("project workspace path: %w", domain.ErrInvalidInput)
	}
	cleaned := path.Clean(trimmed)
	if cleaned == "." {
		return "", nil
	}
	if cleaned == ".." || strings.HasPrefix(cleaned, "../") {
		return "", fmt.Errorf("project workspace path: %w", domain.ErrInvalidInput)
	}
	return cleaned, nil
}
