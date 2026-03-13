package orchestrator

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/gosuda/steerlane/internal/domain"
	domainagent "github.com/gosuda/steerlane/internal/domain/agent"
	"github.com/gosuda/steerlane/internal/domain/task"
	"github.com/gosuda/steerlane/internal/testutil"
)

func TestStallDetector_DetectsStall(t *testing.T) {
	t.Parallel()

	sessionID := domain.NewID()
	taskID := testutil.TestTaskID()
	tenantID := testutil.TestTenantID()
	backend := &fakeBackend{}
	notifier := &fakeNotifier{}
	publisher := &fakePublisher{}

	var taskTransitions []task.TaskStatus
	var sessionStatuses []domainagent.SessionStatus

	orch := New(Deps{
		Logger: testOrchestratorLogger(),
		Tasks: &testutil.MockTaskRepo{
			TransitionFn: func(_ context.Context, gotTenantID domain.TenantID, gotTaskID domain.TaskID, next task.TaskStatus) error {
				assert.Equal(t, tenantID, gotTenantID)
				assert.Equal(t, taskID, gotTaskID)
				taskTransitions = append(taskTransitions, next)
				return nil
			},
		},
		Sessions: &testutil.MockAgentRepo{
			UpdateStatusFn: func(_ context.Context, gotTenantID domain.TenantID, gotSessionID domain.AgentSessionID, status domainagent.SessionStatus) error {
				assert.Equal(t, tenantID, gotTenantID)
				assert.Equal(t, sessionID, gotSessionID)
				sessionStatuses = append(sessionStatuses, status)
				return nil
			},
		},
		Notifier: notifier,
		PubSub:   publisher,
	})

	rs := &runningSession{
		backend:   backend,
		sessionID: sessionID,
		taskID:    taskID,
		tenantID:  tenantID,
		cancel:    func() {},
	}
	rs.lastEventAt.Store(time.Now().Add(-10 * time.Minute).UnixNano())
	orch.sessions[sessionID] = rs

	detector := NewStallDetector(orch, 5*time.Minute, time.Second)
	detector.checkStalledSessions(time.Now())

	assert.Equal(t, []domainagent.SessionStatus{domainagent.StatusFailed}, sessionStatuses)
	assert.Equal(t, []task.TaskStatus{task.StatusBacklog}, taskTransitions)
	assert.Equal(t, 1, backend.cancelCalls)
	assert.Equal(t, 1, backend.disposeCalls)
	require.Len(t, notifier.failed, 1)
	assert.Equal(t, taskID, notifier.failed[0].taskID)
	assert.Equal(t, sessionID, notifier.failed[0].sessionID)
	assert.Contains(t, notifier.failed[0].reason, "stalled")
	assert.Empty(t, orch.ActiveSessions())

	events := publisher.snapshot()
	require.Len(t, events, 1)
	payload, ok := events[0].payload.(map[string]any)
	require.True(t, ok)
	assert.Equal(t, string(domainagent.StatusFailed), payload["status"])
	assert.Equal(t, taskID.String(), payload["task_id"])
	assert.Contains(t, payload["error"], "stalled")
}

func TestStallDetector_ActiveSessionNotStalled(t *testing.T) {
	t.Parallel()

	sessionID := domain.NewID()
	backend := &fakeBackend{}
	notifier := &fakeNotifier{}

	var taskTransitions []task.TaskStatus
	var sessionStatuses []domainagent.SessionStatus

	orch := New(Deps{
		Logger: testOrchestratorLogger(),
		Tasks: &testutil.MockTaskRepo{
			TransitionFn: func(_ context.Context, _ domain.TenantID, _ domain.TaskID, next task.TaskStatus) error {
				taskTransitions = append(taskTransitions, next)
				return nil
			},
		},
		Sessions: &testutil.MockAgentRepo{
			UpdateStatusFn: func(_ context.Context, _ domain.TenantID, _ domain.AgentSessionID, status domainagent.SessionStatus) error {
				sessionStatuses = append(sessionStatuses, status)
				return nil
			},
		},
		Notifier: notifier,
	})

	rs := &runningSession{
		backend:   backend,
		sessionID: sessionID,
		taskID:    testutil.TestTaskID(),
		tenantID:  testutil.TestTenantID(),
		cancel:    func() {},
	}
	rs.lastEventAt.Store(time.Now().Add(-1 * time.Minute).UnixNano())
	orch.sessions[sessionID] = rs

	detector := NewStallDetector(orch, 5*time.Minute, time.Second)
	detector.checkStalledSessions(time.Now())

	assert.Empty(t, sessionStatuses)
	assert.Empty(t, taskTransitions)
	assert.Equal(t, 0, backend.cancelCalls)
	assert.Equal(t, 0, backend.disposeCalls)
	assert.Empty(t, notifier.failed)
	assert.Equal(t, []domain.AgentSessionID{sessionID}, orch.ActiveSessions())
}
