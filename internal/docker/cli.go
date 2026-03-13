package docker

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"os/exec"
	"strconv"
	"strings"
)

// CLIRuntime implements RuntimeManager using the docker CLI.
// This avoids the heavy Docker Go SDK dependency and provides a simpler,
// more portable implementation suitable for Phase 1B.
type CLIRuntime struct {
	logger *slog.Logger
	// dockerPath is the path to the docker binary.
	dockerPath string
}

// NewCLIRuntime creates a RuntimeManager backed by the docker CLI.
func NewCLIRuntime(logger *slog.Logger) (*CLIRuntime, error) {
	if logger == nil {
		logger = slog.Default()
	}
	// Verify docker is available.
	path, err := exec.LookPath("docker")
	if err != nil {
		return nil, fmt.Errorf("docker: docker CLI not found in PATH: %w", err)
	}
	return &CLIRuntime{
		logger:     logger.With("component", "docker-runtime"),
		dockerPath: path,
	}, nil
}

func (r *CLIRuntime) CreateContainer(ctx context.Context, cfg ContainerConfig) (string, error) {
	if err := cfg.Validate(); err != nil {
		return "", err
	}

	args := []string{"create", "--label", "managed-by=steerlane"}

	if cfg.Name != "" {
		args = append(args, "--name", cfg.Name)
	}
	if cfg.WorkDir != "" {
		args = append(args, "--workdir", cfg.WorkDir)
	}
	for _, env := range cfg.Env {
		args = append(args, "--env", env)
	}
	for _, m := range cfg.Mounts {
		mountType := "volume"
		opt := fmt.Sprintf("type=%s,source=%s,target=%s", mountType, m.Source, m.Target)
		if m.ReadOnly {
			opt += ",readonly"
		}
		args = append(args, "--mount", opt)
	}
	if cfg.CPULimit > 0 {
		args = append(args, "--cpus", fmt.Sprintf("%.2f", float64(cfg.CPULimit)/1e9))
	}
	if cfg.MemoryLimit > 0 {
		args = append(args, "--memory", strconv.FormatInt(cfg.MemoryLimit, 10))
	}
	for k, v := range cfg.Labels {
		args = append(args, "--label", k+"="+v)
	}

	args = append(args, cfg.Image)
	args = append(args, cfg.Cmd...)

	out, err := r.run(ctx, args...)
	if err != nil {
		return "", fmt.Errorf("docker create: %w", err)
	}

	containerID := strings.TrimSpace(out)
	r.logger.InfoContext(ctx, "container created",
		"container_id", truncateID(containerID),
		"image", cfg.Image,
	)
	return containerID, nil
}

func (r *CLIRuntime) StartContainer(ctx context.Context, containerID string) error {
	_, err := r.run(ctx, "start", containerID)
	if err != nil {
		return fmt.Errorf("docker start: %w", err)
	}
	r.logger.InfoContext(ctx, "container started",
		"container_id", truncateID(containerID),
	)
	return nil
}

func (r *CLIRuntime) WaitContainer(ctx context.Context, containerID string) (int64, error) {
	out, err := r.run(ctx, "wait", containerID)
	if err != nil {
		return -1, fmt.Errorf("docker wait: %w", err)
	}
	code, parseErr := strconv.ParseInt(strings.TrimSpace(out), 10, 64)
	if parseErr != nil {
		return -1, fmt.Errorf("docker wait: parse exit code %q: %w", out, parseErr)
	}
	r.logger.InfoContext(ctx, "container exited",
		"container_id", truncateID(containerID),
		"exit_code", code,
	)
	return code, nil
}

func (r *CLIRuntime) StreamLogs(ctx context.Context, containerID string, output io.Writer) error {
	cmd := exec.CommandContext(ctx, r.dockerPath, "logs", "--follow", "--timestamps", containerID) //nolint:gosec // containerID is validated by Docker daemon
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return fmt.Errorf("docker logs: stdout pipe: %w", err)
	}
	cmd.Stderr = cmd.Stdout // merge stderr into stdout

	if startErr := cmd.Start(); startErr != nil {
		return fmt.Errorf("docker logs: start: %w", startErr)
	}

	scanner := bufio.NewScanner(stdout)
	for scanner.Scan() {
		if _, writeErr := fmt.Fprintln(output, scanner.Text()); writeErr != nil {
			break
		}
	}

	return cmd.Wait()
}

func (r *CLIRuntime) StopContainer(ctx context.Context, containerID string) error {
	_, err := r.run(ctx, "stop", "--time", "10", containerID)
	if err != nil {
		return fmt.Errorf("docker stop: %w", err)
	}
	r.logger.InfoContext(ctx, "container stopped",
		"container_id", truncateID(containerID),
	)
	return nil
}

func (r *CLIRuntime) RemoveContainer(ctx context.Context, containerID string) error {
	_, err := r.run(ctx, "rm", "--volumes", containerID)
	if err != nil {
		return fmt.Errorf("docker rm: %w", err)
	}
	r.logger.InfoContext(ctx, "container removed",
		"container_id", truncateID(containerID),
	)
	return nil
}

// inspectJSON is the subset of docker inspect output we care about.
type inspectJSON struct {
	ID    string `json:"Id"`
	State struct {
		Error    string `json:"Error"`
		ExitCode int    `json:"ExitCode"`
		Running  bool   `json:"Running"`
	} `json:"State"`
}

func (r *CLIRuntime) InspectContainer(ctx context.Context, containerID string) (*ContainerStatus, error) {
	out, err := r.run(ctx, "inspect", "--format", "{{json .}}", containerID)
	if err != nil {
		return nil, fmt.Errorf("docker inspect: %w", err)
	}

	var info inspectJSON
	if jsonErr := json.Unmarshal([]byte(out), &info); jsonErr != nil {
		return nil, fmt.Errorf("docker inspect: parse: %w", jsonErr)
	}

	return &ContainerStatus{
		ContainerID: info.ID,
		Running:     info.State.Running,
		ExitCode:    info.State.ExitCode,
		Error:       info.State.Error,
	}, nil
}

// listJSON is the subset of docker ps --format json output we parse.
type listJSON struct {
	ID     string `json:"ID"`
	State  string `json:"State"`
	Labels string `json:"Labels"`
}

func (r *CLIRuntime) ListContainersByLabel(ctx context.Context, labelKey, labelValue string) ([]ContainerStatus, error) {
	filter := fmt.Sprintf("label=%s=%s", labelKey, labelValue)
	out, err := r.run(ctx, "ps", "--all", "--filter", filter, "--format", "{{json .}}")
	if err != nil {
		return nil, fmt.Errorf("docker ps: %w", err)
	}

	out = strings.TrimSpace(out)
	if out == "" {
		return nil, nil
	}

	var containers []ContainerStatus
	scanner := bufio.NewScanner(strings.NewReader(out))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		var info listJSON
		if jsonErr := json.Unmarshal([]byte(line), &info); jsonErr != nil {
			r.logger.WarnContext(ctx, "skipping unparseable container entry", "error", jsonErr)
			continue
		}
		containers = append(containers, ContainerStatus{
			ContainerID: info.ID,
			Running:     strings.EqualFold(info.State, "running"),
		})
	}
	if scanErr := scanner.Err(); scanErr != nil {
		return nil, fmt.Errorf("docker ps: scan: %w", scanErr)
	}

	return containers, nil
}

// run executes a docker CLI command and returns its combined output.
func (r *CLIRuntime) run(ctx context.Context, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, r.dockerPath, args...) //nolint:gosec // args are constructed internally, not from user input
	out, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("%w: %s", err, strings.TrimSpace(string(out)))
	}
	return string(out), nil
}

// truncateID returns a short prefix of a container ID for logging.
func truncateID(id string) string {
	if len(id) > 12 {
		return id[:12]
	}
	return id
}

// Compile-time interface check.
var _ RuntimeManager = (*CLIRuntime)(nil)
