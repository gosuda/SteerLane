package orchestrator

import (
	"context"
	"io"
	"log/slog"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/gosuda/steerlane/internal/docker"
	"github.com/gosuda/steerlane/internal/domain"
	"github.com/gosuda/steerlane/internal/testutil"
)

type fakeRuntime struct {
	listFn   func(ctx context.Context, labelKey, labelValue string) ([]docker.ContainerStatus, error)
	stopFn   func(ctx context.Context, containerID string) error
	removeFn func(ctx context.Context, containerID string) error
}

func (f *fakeRuntime) CreateContainer(context.Context, docker.ContainerConfig) (string, error) {
	return "", nil
}
func (f *fakeRuntime) StartContainer(context.Context, string) error { return nil }
func (f *fakeRuntime) WaitContainer(context.Context, string) (int64, error) {
	return 0, nil
}
func (f *fakeRuntime) StreamLogs(context.Context, string, io.Writer) error {
	return nil
}
func (f *fakeRuntime) StopContainer(ctx context.Context, containerID string) error {
	if f.stopFn != nil {
		return f.stopFn(ctx, containerID)
	}
	return nil
}
func (f *fakeRuntime) RemoveContainer(ctx context.Context, containerID string) error {
	if f.removeFn != nil {
		return f.removeFn(ctx, containerID)
	}
	return nil
}
func (f *fakeRuntime) InspectContainer(context.Context, string) (*docker.ContainerStatus, error) {
	return nil, nil
}
func (f *fakeRuntime) ListContainersByLabel(ctx context.Context, labelKey, labelValue string) ([]docker.ContainerStatus, error) {
	if f.listFn != nil {
		return f.listFn(ctx, labelKey, labelValue)
	}
	return nil, nil
}

func TestReconciler_RemovesOrphanContainers(t *testing.T) {
	t.Parallel()

	var stopped []string
	var removed []string

	runtime := &fakeRuntime{
		listFn: func(_ context.Context, _, _ string) ([]docker.ContainerStatus, error) {
			return []docker.ContainerStatus{
				{ContainerID: "orphan-running-1", Running: true},
				{ContainerID: "orphan-stopped-2", Running: false},
				{ContainerID: "tracked-3", Running: true},
			}, nil
		},
		stopFn: func(_ context.Context, id string) error {
			stopped = append(stopped, id)
			return nil
		},
		removeFn: func(_ context.Context, id string) error {
			removed = append(removed, id)
			return nil
		},
	}

	orch := New(Deps{
		Logger:  slog.New(slog.NewTextHandler(io.Discard, nil)),
		Runtime: runtime,
	})

	// "tracked-3" is an active session.
	orch.sessions[domain.NewID()] = &runningSession{
		containerID: "tracked-3",
		sessionID:   testutil.TestSessionID(),
		taskID:      testutil.TestTaskID(),
		tenantID:    testutil.TestTenantID(),
		cancel:      func() {},
	}

	reconciler := NewReconciler(orch, 0)
	reconciler.reconcile(context.Background())

	assert.Equal(t, []string{"orphan-running-1"}, stopped, "only running orphan should be stopped")
	assert.ElementsMatch(t, []string{"orphan-running-1", "orphan-stopped-2"}, removed, "both orphans should be removed")
}

func TestReconciler_SkipsActiveContainers(t *testing.T) {
	t.Parallel()

	var removed []string
	runtime := &fakeRuntime{
		listFn: func(_ context.Context, _, _ string) ([]docker.ContainerStatus, error) {
			return []docker.ContainerStatus{
				{ContainerID: "active-1", Running: true},
			}, nil
		},
		removeFn: func(_ context.Context, id string) error {
			removed = append(removed, id)
			return nil
		},
	}

	orch := New(Deps{
		Logger:  slog.New(slog.NewTextHandler(io.Discard, nil)),
		Runtime: runtime,
	})
	orch.sessions[domain.NewID()] = &runningSession{
		containerID: "active-1",
		sessionID:   testutil.TestSessionID(),
		taskID:      testutil.TestTaskID(),
		tenantID:    testutil.TestTenantID(),
		cancel:      func() {},
	}

	reconciler := NewReconciler(orch, 0)
	reconciler.reconcile(context.Background())

	assert.Empty(t, removed, "active containers should not be removed")
}
