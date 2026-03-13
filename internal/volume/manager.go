package volume

import (
	"context"
	"fmt"
	"log/slog"
	"os/exec"
	"strings"
	"time"

	"github.com/gosuda/steerlane/internal/domain"
)

// StalenessThreshold is the default duration after which a volume's repo
// is considered stale and needs a fresh git fetch before use.
const StalenessThreshold = 1 * time.Hour

// Manager handles Docker volume lifecycle for project repositories.
type Manager struct {
	logger     *slog.Logger
	dockerPath string
}

// NewManager creates a volume manager. Requires docker CLI in PATH.
func NewManager(logger *slog.Logger) (*Manager, error) {
	if logger == nil {
		logger = slog.Default()
	}
	path, err := exec.LookPath("docker")
	if err != nil {
		return nil, fmt.Errorf("volume: docker CLI not found in PATH: %w", err)
	}
	return &Manager{
		logger:     logger.With("component", "volume-manager"),
		dockerPath: path,
	}, nil
}

// VolumeName returns the Docker volume name for a project.
func VolumeName(projectID domain.ProjectID) string {
	return "steerlane-repo-" + projectID.String()
}

// EnsureVolume creates a Docker volume if it doesn't exist.
// Returns the volume name.
func (m *Manager) EnsureVolume(ctx context.Context, projectID domain.ProjectID) (string, error) {
	name := VolumeName(projectID)

	// Check if volume already exists
	out, err := m.docker(ctx, "volume", "ls", "--filter", "name="+name, "--format", "{{.Name}}")
	if err != nil {
		return "", fmt.Errorf("volume: list: %w", err)
	}
	if strings.TrimSpace(out) == name {
		m.logger.InfoContext(ctx, "volume already exists", "volume", name)
		return name, nil
	}

	// Create volume
	if _, createErr := m.docker(ctx, "volume", "create", "--label", "managed-by=steerlane", "--label", "project-id="+projectID.String(), name); createErr != nil {
		return "", fmt.Errorf("volume: create %s: %w", name, createErr)
	}

	m.logger.InfoContext(ctx, "volume created", "volume", name, "project_id", projectID)
	return name, nil
}

// RemoveVolume removes a Docker volume.
func (m *Manager) RemoveVolume(ctx context.Context, projectID domain.ProjectID) error {
	name := VolumeName(projectID)
	if _, err := m.docker(ctx, "volume", "rm", name); err != nil {
		return fmt.Errorf("volume: remove %s: %w", name, err)
	}
	m.logger.InfoContext(ctx, "volume removed", "volume", name, "project_id", projectID)
	return nil
}

// IsStale checks whether the volume's last fetch is older than the threshold.
func IsStale(lastFetchedAt *time.Time, threshold time.Duration) bool {
	if lastFetchedAt == nil {
		return true
	}
	return time.Since(*lastFetchedAt) > threshold
}

func (m *Manager) docker(ctx context.Context, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, m.dockerPath, args...) //nolint:gosec // args are constructed internally, not from user input
	out, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("%w: %s", err, strings.TrimSpace(string(out)))
	}
	return string(out), nil
}
