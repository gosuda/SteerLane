package docker

import (
	"context"
	"fmt"
	"io"

	"github.com/gosuda/steerlane/internal/domain"
)

// Mount describes a filesystem mount for a container.
type Mount struct {
	// Source is the host path or Docker volume name.
	Source string
	// Target is the container-internal mount path.
	Target string
	// ReadOnly makes the mount read-only inside the container.
	ReadOnly bool
}

// ContainerConfig describes how to create a container.
type ContainerConfig struct {
	// Labels are metadata labels attached to the container.
	Labels map[string]string
	// Name is an optional container name (for identification/logging).
	Name string
	// Image is the Docker image to run.
	Image string
	// WorkDir is the working directory inside the container.
	WorkDir string
	// Cmd is the command and arguments to execute.
	Cmd []string
	// Env is environment variables in KEY=VALUE format.
	Env []string
	// Mounts are filesystem mounts (volumes, bind mounts).
	Mounts []Mount
	// CPULimit is the CPU quota in units of 1e-9 CPUs (nanoCPUs).
	// 0 means unlimited.
	CPULimit int64
	// MemoryLimit is the memory limit in bytes. 0 means unlimited.
	MemoryLimit int64
}

// ContainerStatus represents the current state of a container.
type ContainerStatus struct {
	// ContainerID is the Docker-assigned container identifier.
	ContainerID string
	// Error is set if the container exited due to an error.
	Error string
	// ExitCode is the process exit code (valid only when Running is false).
	ExitCode int
	// Running indicates whether the container is currently executing.
	Running bool
}

// RuntimeManager abstracts container lifecycle management.
// The interface decouples the orchestrator from the specific container runtime.
type RuntimeManager interface {
	// CreateContainer creates a new container from the given config.
	// Returns the container ID.
	CreateContainer(ctx context.Context, cfg ContainerConfig) (containerID string, err error)

	// StartContainer starts a previously created container.
	StartContainer(ctx context.Context, containerID string) error

	// WaitContainer blocks until the container exits, returning the exit code.
	WaitContainer(ctx context.Context, containerID string) (exitCode int64, err error)

	// StreamLogs streams container stdout+stderr to the provided writer.
	// Blocks until the container stops or the context is cancelled.
	StreamLogs(ctx context.Context, containerID string, output io.Writer) error

	// StopContainer sends a stop signal and waits for graceful shutdown.
	StopContainer(ctx context.Context, containerID string) error

	// RemoveContainer removes a stopped container and its anonymous volumes.
	RemoveContainer(ctx context.Context, containerID string) error

	// InspectContainer returns the current status of a container.
	InspectContainer(ctx context.Context, containerID string) (*ContainerStatus, error)

	// ListContainersByLabel returns all containers that have the given label key=value.
	ListContainersByLabel(ctx context.Context, labelKey, labelValue string) ([]ContainerStatus, error)
}

// Validate checks that the container config has required fields.
func (c *ContainerConfig) Validate() error {
	if c.Image == "" {
		return fmt.Errorf("docker: image is required: %w", domain.ErrInvalidInput)
	}
	return nil
}
