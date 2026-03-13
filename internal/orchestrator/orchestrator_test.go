package orchestrator

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	agentpkg "github.com/gosuda/steerlane/internal/agent"
	"github.com/gosuda/steerlane/internal/domain"
	"github.com/gosuda/steerlane/internal/domain/adr"
	domainagent "github.com/gosuda/steerlane/internal/domain/agent"
	"github.com/gosuda/steerlane/internal/domain/hitl"
	"github.com/gosuda/steerlane/internal/domain/project"
	"github.com/gosuda/steerlane/internal/domain/task"
	redispkg "github.com/gosuda/steerlane/internal/store/redis"
	"github.com/gosuda/steerlane/internal/testutil"
)

type fakeVolumeManager struct {
	ensureVolumeFn func(ctx context.Context, projectID domain.ProjectID) (string, error)
	projectIDs     []domain.ProjectID
}

func (f *fakeVolumeManager) EnsureVolume(ctx context.Context, projectID domain.ProjectID) (string, error) {
	f.projectIDs = append(f.projectIDs, projectID)
	if f.ensureVolumeFn != nil {
		return f.ensureVolumeFn(ctx, projectID)
	}
	return "project-volume", nil
}

type createBranchCall struct {
	baseBranch string
	repoDir    string
	sessionID  domain.AgentSessionID
}

type fakeGitOps struct {
	createBranchFn func(ctx context.Context, repoDir string, sessionID domain.AgentSessionID, baseBranch string) (string, error)
	fetchFn        func(ctx context.Context, repoDir string) error
	fetchRepoDirs  []string
	createCalls    []createBranchCall
}

func (f *fakeGitOps) Fetch(ctx context.Context, repoDir string) error {
	f.fetchRepoDirs = append(f.fetchRepoDirs, repoDir)
	if f.fetchFn != nil {
		return f.fetchFn(ctx, repoDir)
	}
	return nil
}

func (f *fakeGitOps) CreateBranch(ctx context.Context, repoDir string, sessionID domain.AgentSessionID, baseBranch string) (string, error) {
	f.createCalls = append(f.createCalls, createBranchCall{
		repoDir:    repoDir,
		sessionID:  sessionID,
		baseBranch: baseBranch,
	})
	if f.createBranchFn != nil {
		return f.createBranchFn(ctx, repoDir, sessionID, baseBranch)
	}
	return "steerlane/test-branch", nil
}

//nolint:govet // test double fields group callbacks and recordings for clarity.
type fakeBackend struct {
	cancelFn     func(ctx context.Context) error
	disposeFn    func() error
	sendPromptFn func(ctx context.Context, prompt string) error
	startFn      func(ctx context.Context, opts agentpkg.SessionOpts) error

	cancelCalls    int
	disposeCalls   int
	messageHandler agentpkg.MessageHandler
	prompts        []string
	startCalls     []agentpkg.SessionOpts
}

func (f *fakeBackend) StartSession(ctx context.Context, opts agentpkg.SessionOpts) error {
	f.startCalls = append(f.startCalls, opts)
	if f.startFn != nil {
		return f.startFn(ctx, opts)
	}
	return nil
}

func (f *fakeBackend) SendPrompt(ctx context.Context, prompt string) error {
	f.prompts = append(f.prompts, prompt)
	if f.sendPromptFn != nil {
		return f.sendPromptFn(ctx, prompt)
	}
	return nil
}

func (f *fakeBackend) Cancel(ctx context.Context) error {
	f.cancelCalls++
	if f.cancelFn != nil {
		return f.cancelFn(ctx)
	}
	return nil
}

func (f *fakeBackend) OnMessage(handler agentpkg.MessageHandler) {
	f.messageHandler = handler
}

func (f *fakeBackend) Dispose() error {
	f.disposeCalls++
	if f.disposeFn != nil {
		return f.disposeFn()
	}
	return nil
}

type recordedPublisherEvent struct {
	eventType redispkg.EventType
	payload   any
	sessionID string
}

type fakeHITLHandler struct {
	question *hitl.Question
	err      error
	inputs   [][]byte
}

type fakeADREngine struct {
	createRecord *adr.ADR
	createErr    error
	createInputs [][]byte
}

func (f *fakeADREngine) HandleCreateADR(_ context.Context, _ domain.TenantID, _ domain.ProjectID, _ domain.AgentSessionID, input json.RawMessage) (*adr.ADR, error) {
	f.createInputs = append(f.createInputs, input)
	if f.createErr != nil {
		return nil, f.createErr
	}
	return f.createRecord, nil
}

func (f *fakeADREngine) ExtractFromSession(context.Context, domain.TenantID, domain.ProjectID, domain.AgentSessionID, map[string]any) ([]*adr.ADR, error) {
	return nil, nil
}

type completedNotification struct {
	taskID    domain.TaskID
	sessionID domain.AgentSessionID
}

type failedNotification struct {
	reason    string
	taskID    domain.TaskID
	sessionID domain.AgentSessionID
}

type adrCreatedNotification struct {
	record    *adr.ADR
	sessionID domain.AgentSessionID
	tenantID  domain.TenantID
}

func (f *fakeHITLHandler) HandleAskHuman(_ context.Context, _ domain.TenantID, _ domain.AgentSessionID, input json.RawMessage) (*hitl.Question, error) {
	f.inputs = append(f.inputs, input)
	if f.err != nil {
		return nil, f.err
	}
	return f.question, nil
}

type fakeNotifier struct { //nolint:govet // test double groups captured notification state for clarity
	completed []completedNotification
	failed    []failedNotification
	created   []adrCreatedNotification
	err       error
}

func (f *fakeNotifier) NotifySessionFailed(_ context.Context, _ domain.TenantID, taskID domain.TaskID, sessionID domain.AgentSessionID, reason string) error {
	f.failed = append(f.failed, failedNotification{reason: reason, taskID: taskID, sessionID: sessionID})
	return nil
}

func (f *fakeNotifier) NotifyTaskCompleted(_ context.Context, _ domain.TenantID, taskID domain.TaskID, sessionID domain.AgentSessionID) error {
	f.completed = append(f.completed, completedNotification{taskID: taskID, sessionID: sessionID})
	return nil
}

func (f *fakeNotifier) NotifyADRCreated(_ context.Context, tenantID domain.TenantID, sessionID domain.AgentSessionID, record *adr.ADR) error {
	f.created = append(f.created, adrCreatedNotification{tenantID: tenantID, sessionID: sessionID, record: record})
	return f.err
}

//nolint:govet // test double fields group synchronization and captured events for clarity.
type fakePublisher struct {
	mu     sync.Mutex
	events []recordedPublisherEvent
	err    error
}

type fakeEventRepo struct { //nolint:govet // test double groups captured replay appends for clarity.
	mu      sync.Mutex
	appends []*domainagent.Event
	err     error
}

func (f *fakePublisher) PublishAgentEvent(_ context.Context, sessionID string, eventType redispkg.EventType, payload any) error {
	f.mu.Lock()
	defer f.mu.Unlock()

	f.events = append(f.events, recordedPublisherEvent{
		sessionID: sessionID,
		eventType: eventType,
		payload:   payload,
	})

	return f.err
}

func (f *fakePublisher) snapshot() []recordedPublisherEvent {
	f.mu.Lock()
	defer f.mu.Unlock()

	out := make([]recordedPublisherEvent, len(f.events))
	copy(out, f.events)
	return out
}

func (f *fakeEventRepo) Append(_ context.Context, event *domainagent.Event) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if event != nil {
		clone := *event
		clone.Payload = append([]byte(nil), event.Payload...)
		f.appends = append(f.appends, &clone)
	}
	return f.err
}

func (f *fakeEventRepo) ListBySession(context.Context, domain.TenantID, domain.AgentSessionID, int, *uuid.UUID) ([]*domainagent.Event, error) {
	return nil, nil
}

func (f *fakeEventRepo) snapshot() []*domainagent.Event {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]*domainagent.Event, len(f.appends))
	copy(out, f.appends)
	return out
}

func testOrchestratorLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func newTaskFixture(status task.TaskStatus) *task.Task {
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	return &task.Task{
		ID:          testutil.TestTaskID(),
		TenantID:    testutil.TestTenantID(),
		ProjectID:   testutil.TestProjectID(),
		Title:       "Implement retries",
		Description: "Handle transient failures",
		Status:      status,
		Priority:    1,
		CreatedAt:   now,
		UpdatedAt:   now,
	}
}

func newRegistryWithBackend(t *testing.T, backend agentpkg.Backend) *agentpkg.Registry {
	t.Helper()

	registry := agentpkg.NewRegistry()
	registry.Register(domainagent.TypeClaude, func(_ *slog.Logger) (agentpkg.Backend, error) {
		return backend, nil
	})

	return registry
}

func TestOrchestrator_DispatchTask_HappyPath(t *testing.T) {
	t.Parallel()

	projectRecord := testutil.TestProject()
	taskRecord := newTaskFixture(task.StatusBacklog)
	backend := &fakeBackend{}
	volumes := &fakeVolumeManager{}
	gitOps := &fakeGitOps{}
	publisher := &fakePublisher{}

	var createdSession *domainagent.Session
	var updatedTask task.Task
	var taskTransitions []task.TaskStatus
	var sessionStatuses []domainagent.SessionStatus

	orch := New(Deps{
		Logger:   testOrchestratorLogger(),
		Registry: newRegistryWithBackend(t, backend),
		Volumes:  volumes,
		GitOps:   gitOps,
		Projects: &testutil.MockProjectRepo{
			GetByIDFn: func(_ context.Context, tenantID domain.TenantID, id domain.ProjectID) (*project.Project, error) {
				assert.Equal(t, testutil.TestTenantID(), tenantID)
				assert.Equal(t, projectRecord.ID, id)
				return projectRecord, nil
			},
		},
		Tasks: &testutil.MockTaskRepo{
			GetByIDFn: func(_ context.Context, tenantID domain.TenantID, id domain.TaskID) (*task.Task, error) {
				assert.Equal(t, testutil.TestTenantID(), tenantID)
				assert.Equal(t, taskRecord.ID, id)
				return taskRecord, nil
			},
			TransitionFn: func(_ context.Context, tenantID domain.TenantID, id domain.TaskID, next task.TaskStatus) error {
				assert.Equal(t, testutil.TestTenantID(), tenantID)
				assert.Equal(t, taskRecord.ID, id)
				taskTransitions = append(taskTransitions, next)
				taskRecord.Status = next
				return nil
			},
			UpdateFn: func(_ context.Context, got *task.Task) error {
				updatedTask = *got
				return nil
			},
		},
		Sessions: &testutil.MockAgentRepo{
			CreateFn: func(_ context.Context, session *domainagent.Session) error {
				createdSession = session
				return nil
			},
			UpdateStatusFn: func(_ context.Context, tenantID domain.TenantID, id domain.AgentSessionID, status domainagent.SessionStatus) error {
				assert.Equal(t, testutil.TestTenantID(), tenantID)
				assert.NotEqual(t, domain.AgentSessionID{}, id)
				sessionStatuses = append(sessionStatuses, status)
				return nil
			},
		},
		PubSub: publisher,
	})

	sessionID, err := orch.DispatchTask(t.Context(), testutil.TestTenantID(), taskRecord.ID, domainagent.TypeClaude, "Implement retries")

	require.NoError(t, err)
	require.NotNil(t, createdSession)
	require.Len(t, backend.startCalls, 1)
	require.NotNil(t, backend.messageHandler)

	assert.Equal(t, createdSession.ID, sessionID)
	assert.Equal(t, domainagent.StatusPending, createdSession.Status)
	assert.Equal(t, projectRecord.ID, createdSession.ProjectID)
	assert.Equal(t, taskRecord.ID, createdSession.TaskID)
	assert.Equal(t, []task.TaskStatus{task.StatusInProgress}, taskTransitions)
	assert.Equal(t, []domainagent.SessionStatus{domainagent.StatusRunning}, sessionStatuses)
	require.NotNil(t, updatedTask.AgentSessionID)
	assert.Equal(t, sessionID, *updatedTask.AgentSessionID)
	assert.Equal(t, []domain.ProjectID{projectRecord.ID}, volumes.projectIDs)
	assert.Equal(t, []string{"/mnt/volumes/project-volume"}, gitOps.fetchRepoDirs)
	require.Len(t, gitOps.createCalls, 1)
	assert.Equal(t, "/mnt/volumes/project-volume", gitOps.createCalls[0].repoDir)
	assert.Equal(t, sessionID, gitOps.createCalls[0].sessionID)
	assert.Equal(t, projectRecord.Branch, gitOps.createCalls[0].baseBranch)
	assert.Equal(t, sessionID, backend.startCalls[0].SessionID)
	assert.Equal(t, taskRecord.ID, backend.startCalls[0].TaskID)
	assert.Equal(t, projectRecord.ID, backend.startCalls[0].ProjectID)
	assert.Equal(t, testutil.TestTenantID(), backend.startCalls[0].TenantID)
	assert.Equal(t, "Implement retries", backend.startCalls[0].Prompt)
	assert.Equal(t, "/mnt/volumes/project-volume", backend.startCalls[0].RepoPath)
	assert.Equal(t, "steerlane/test-branch", backend.startCalls[0].BranchName)
	assert.Equal(t, []domain.AgentSessionID{sessionID}, orch.ActiveSessions())

	events := publisher.snapshot()
	require.Len(t, events, 1)
	assert.Equal(t, redispkg.EventSessionStarted, events[0].eventType)
	assert.Equal(t, sessionID.String(), events[0].sessionID)
	payload, ok := events[0].payload.(map[string]any)
	require.True(t, ok, "payload should be a map")
	assert.Equal(t, sessionID.String(), payload["session_id"])
	assert.Equal(t, taskRecord.ID.String(), payload["task_id"])
	assert.Equal(t, projectRecord.ID.String(), payload["project_id"])
	assert.Equal(t, string(domainagent.TypeClaude), payload["agent_type"])
}

func TestDispatchTaskUsesWorkspacePathForBackendOnly(t *testing.T) {
	t.Parallel()

	registry := agentpkg.NewRegistry()
	backend := &fakeBackend{}
	registry.Register(domainagent.TypeClaude, func(_ *slog.Logger) (agentpkg.Backend, error) { return backend, nil })
	gitOps := &fakeGitOps{}
	volumes := &fakeVolumeManager{}
	projectRecord := &project.Project{ID: testutil.TestProjectID(), TenantID: testutil.TestTenantID(), Name: "app", RepoURL: "https://example.com/repo.git", Branch: "main", Settings: map[string]any{"workspace_path": "apps/web", "workspace_paths": []any{"apps/web", "libs/shared"}}}
	taskRecord := &task.Task{ID: testutil.TestTaskID(), TenantID: testutil.TestTenantID(), ProjectID: projectRecord.ID, Title: "Implement retries", Status: task.StatusBacklog}
	orch := New(Deps{
		Logger:   slog.New(slog.NewTextHandler(io.Discard, nil)),
		Registry: registry,
		Runtime:  &fakeRuntime{},
		Volumes:  volumes,
		GitOps:   gitOps,
		Projects: &testutil.MockProjectRepo{GetByIDFn: func(context.Context, domain.TenantID, domain.ProjectID) (*project.Project, error) {
			return projectRecord, nil
		}},
		Tasks: &testutil.MockTaskRepo{
			GetByIDFn:    func(context.Context, domain.TenantID, domain.TaskID) (*task.Task, error) { return taskRecord, nil },
			TransitionFn: func(context.Context, domain.TenantID, domain.TaskID, task.TaskStatus) error { return nil },
			UpdateFn:     func(context.Context, *task.Task) error { return nil },
		},
		Sessions: &testutil.MockAgentRepo{CreateFn: func(context.Context, *domainagent.Session) error { return nil }, UpdateStatusFn: func(context.Context, domain.TenantID, domain.AgentSessionID, domainagent.SessionStatus) error {
			return nil
		}},
		Questions: &testutil.MockHITLRepo{},
		ADRs:      &testutil.MockADRRepo{},
		Events:    &testutil.MockAgentEventRepo{},
		PubSub:    &fakePublisher{},
		HITL:      &fakeHITLHandler{},
		ADREngine: &fakeADREngine{},
	})

	_, err := orch.DispatchTask(t.Context(), testutil.TestTenantID(), taskRecord.ID, domainagent.TypeClaude, "Implement retries")
	require.NoError(t, err)
	require.Len(t, gitOps.createCalls, 1)
	assert.Equal(t, "/mnt/volumes/project-volume", gitOps.createCalls[0].repoDir)
	require.Len(t, backend.startCalls, 1)
	assert.Equal(t, "/mnt/volumes/project-volume/apps/web", backend.startCalls[0].RepoPath)
	assert.Equal(t, "apps/web", backend.startCalls[0].Env["STEERLANE_WORKSPACE_PATH"])
	assert.JSONEq(t, `["apps/web","libs/shared"]`, backend.startCalls[0].Env["STEERLANE_WORKSPACE_PATHS"])
}

func TestOrchestrator_DispatchTask_StartFailureCompensates(t *testing.T) {
	t.Parallel()

	projectRecord := testutil.TestProject()
	taskRecord := newTaskFixture(task.StatusBacklog)
	startErr := errors.New("container failed")
	backend := &fakeBackend{
		startFn: func(_ context.Context, _ agentpkg.SessionOpts) error {
			return startErr
		},
	}
	publisher := &fakePublisher{}

	var linkedSessionID domain.AgentSessionID
	var taskTransitions []task.TaskStatus
	var sessionStatuses []domainagent.SessionStatus

	orch := New(Deps{
		Logger:   testOrchestratorLogger(),
		Registry: newRegistryWithBackend(t, backend),
		Volumes:  &fakeVolumeManager{},
		GitOps:   &fakeGitOps{},
		Projects: &testutil.MockProjectRepo{
			GetByIDFn: func(_ context.Context, _ domain.TenantID, _ domain.ProjectID) (*project.Project, error) {
				return projectRecord, nil
			},
		},
		Tasks: &testutil.MockTaskRepo{
			GetByIDFn: func(_ context.Context, _ domain.TenantID, _ domain.TaskID) (*task.Task, error) {
				return taskRecord, nil
			},
			TransitionFn: func(_ context.Context, _ domain.TenantID, _ domain.TaskID, next task.TaskStatus) error {
				taskTransitions = append(taskTransitions, next)
				taskRecord.Status = next
				return nil
			},
			UpdateFn: func(_ context.Context, got *task.Task) error {
				linkedSessionID = *got.AgentSessionID
				return nil
			},
		},
		Sessions: &testutil.MockAgentRepo{
			CreateFn: func(_ context.Context, session *domainagent.Session) error {
				linkedSessionID = session.ID
				return nil
			},
			UpdateStatusFn: func(_ context.Context, _ domain.TenantID, _ domain.AgentSessionID, status domainagent.SessionStatus) error {
				sessionStatuses = append(sessionStatuses, status)
				return nil
			},
		},
		PubSub: publisher,
	})

	gotSessionID, err := orch.DispatchTask(t.Context(), testutil.TestTenantID(), taskRecord.ID, domainagent.TypeClaude, "Implement retries")

	require.Error(t, err)
	require.ErrorIs(t, err, startErr)
	assert.Equal(t, domain.AgentSessionID{}, gotSessionID)
	assert.Equal(t, []task.TaskStatus{task.StatusInProgress, task.StatusBacklog}, taskTransitions)
	assert.Equal(t, []domainagent.SessionStatus{domainagent.StatusRunning, domainagent.StatusFailed}, sessionStatuses)
	assert.Equal(t, 1, backend.disposeCalls)
	assert.Empty(t, orch.ActiveSessions())

	events := publisher.snapshot()
	require.Len(t, events, 1)
	assert.Equal(t, redispkg.EventSessionEnded, events[0].eventType)
	assert.Equal(t, linkedSessionID.String(), events[0].sessionID)
	payload, ok := events[0].payload.(map[string]any)
	require.True(t, ok, "payload should be a map")
	assert.Equal(t, linkedSessionID.String(), payload["session_id"])
	assert.Equal(t, string(domainagent.StatusFailed), payload["status"])
	assert.Contains(t, payload["error"], startErr.Error())
}

func TestOrchestrator_CancelSession(t *testing.T) {
	t.Parallel()

	sessionID := testutil.TestSessionID()
	backend := &fakeBackend{
		cancelFn: func(_ context.Context) error {
			return errors.New("cancel failed")
		},
	}
	publisher := &fakePublisher{}
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
		PubSub: publisher,
	})
	orch.sessions[sessionID] = &runningSession{
		backend:   backend,
		sessionID: sessionID,
		taskID:    testutil.TestTaskID(),
		tenantID:  testutil.TestTenantID(),
		cancel:    func() {},
	}

	err := orch.CancelSession(t.Context(), testutil.TestTenantID(), sessionID)

	require.NoError(t, err)
	assert.Equal(t, 1, backend.cancelCalls)
	assert.Equal(t, 1, backend.disposeCalls)
	assert.Equal(t, []domainagent.SessionStatus{domainagent.StatusCancelled}, sessionStatuses)
	assert.Equal(t, []task.TaskStatus{task.StatusBacklog}, taskTransitions)
	assert.Empty(t, orch.ActiveSessions())

	events := publisher.snapshot()
	require.Len(t, events, 1)
	assert.Equal(t, redispkg.EventSessionEnded, events[0].eventType)
	payload, ok := events[0].payload.(map[string]any)
	require.True(t, ok, "payload should be a map")
	assert.Equal(t, string(domainagent.StatusCancelled), payload["status"])
}

func TestOrchestrator_HandleSessionComplete(t *testing.T) {
	t.Parallel()

	//nolint:govet // test case shape prioritizes readability over field packing.
	tests := []struct {
		name              string
		exitCode          int64
		transitionErr     error
		wantSessionStatus domainagent.SessionStatus
		wantTaskStatus    task.TaskStatus
	}{
		{
			name:              "success moves task to review",
			exitCode:          0,
			wantSessionStatus: domainagent.StatusCompleted,
			wantTaskStatus:    task.StatusReview,
		},
		{
			name:              "failure requeues task",
			exitCode:          17,
			wantSessionStatus: domainagent.StatusFailed,
			wantTaskStatus:    task.StatusBacklog,
		},
		{
			name:              "task transition failure is best effort",
			exitCode:          0,
			transitionErr:     errors.New("transition failed"),
			wantSessionStatus: domainagent.StatusCompleted,
			wantTaskStatus:    task.StatusReview,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			sessionID := domain.NewID()
			backend := &fakeBackend{}
			publisher := &fakePublisher{}
			var taskTransitions []task.TaskStatus
			var sessionStatuses []domainagent.SessionStatus

			orch := New(Deps{
				Logger: testOrchestratorLogger(),
				Tasks: &testutil.MockTaskRepo{
					TransitionFn: func(_ context.Context, _ domain.TenantID, _ domain.TaskID, next task.TaskStatus) error {
						taskTransitions = append(taskTransitions, next)
						return tt.transitionErr
					},
				},
				Sessions: &testutil.MockAgentRepo{
					GetByIDFn: func(_ context.Context, _ domain.TenantID, id domain.AgentSessionID) (*domainagent.Session, error) {
						return &domainagent.Session{ID: id, TenantID: testutil.TestTenantID(), TaskID: testutil.TestTaskID()}, nil
					},
					UpdateStatusFn: func(_ context.Context, _ domain.TenantID, _ domain.AgentSessionID, status domainagent.SessionStatus) error {
						sessionStatuses = append(sessionStatuses, status)
						return nil
					},
				},
				PubSub: publisher,
			})
			orch.sessions[sessionID] = &runningSession{
				backend:   backend,
				sessionID: sessionID,
				taskID:    testutil.TestTaskID(),
				tenantID:  testutil.TestTenantID(),
				cancel:    func() {},
			}

			err := orch.HandleSessionComplete(t.Context(), sessionID, tt.exitCode)

			require.NoError(t, err)
			assert.Equal(t, []domainagent.SessionStatus{tt.wantSessionStatus}, sessionStatuses)
			assert.Equal(t, []task.TaskStatus{tt.wantTaskStatus}, taskTransitions)
			assert.Equal(t, 1, backend.disposeCalls)
			assert.Empty(t, orch.ActiveSessions())

			events := publisher.snapshot()
			require.Len(t, events, 1)
			assert.Equal(t, redispkg.EventSessionEnded, events[0].eventType)
			payload, ok := events[0].payload.(map[string]any)
			require.True(t, ok, "payload should be a map")
			assert.Equal(t, tt.exitCode, payload["exit_code"])
			assert.Equal(t, string(tt.wantSessionStatus), payload["status"])
		})
	}
}

func TestOrchestrator_SendHITLAnswer(t *testing.T) {
	t.Parallel()

	sessionID := testutil.TestSessionID()
	backend := &fakeBackend{}
	publisher := &fakePublisher{}
	var sessionStatuses []domainagent.SessionStatus
	orch := New(Deps{
		Logger: testOrchestratorLogger(),
		Sessions: &testutil.MockAgentRepo{
			UpdateStatusFn: func(_ context.Context, tenantID domain.TenantID, id domain.AgentSessionID, status domainagent.SessionStatus) error {
				assert.Equal(t, testutil.TestTenantID(), tenantID)
				assert.Equal(t, sessionID, id)
				sessionStatuses = append(sessionStatuses, status)
				return nil
			},
		},
		PubSub: publisher,
	})
	orch.sessions[sessionID] = &runningSession{
		backend:   backend,
		sessionID: sessionID,
		taskID:    testutil.TestTaskID(),
		tenantID:  testutil.TestTenantID(),
		cancel:    func() {},
	}

	err := orch.SendHITLAnswer(t.Context(), testutil.TestTenantID(), sessionID, "Use exponential backoff")

	require.NoError(t, err)
	assert.Equal(t, []domainagent.SessionStatus{domainagent.StatusRunning}, sessionStatuses)
	assert.Equal(t, []string{"Use exponential backoff"}, backend.prompts)

	events := publisher.snapshot()
	require.Len(t, events, 1)
	assert.Equal(t, redispkg.EventAgentStatus, events[0].eventType)
	payload, ok := events[0].payload.(map[string]any)
	require.True(t, ok, "payload should be a map")
	assert.Equal(t, sessionID.String(), payload["session_id"])
	assert.Equal(t, string(domainagent.StatusRunning), payload["status"])
	assert.Equal(t, "hitl_answered", payload["event"])
}

func TestOrchestrator_SendHITLAnswer_RollsBackStatusWhenPromptFails(t *testing.T) {
	t.Parallel()

	backend := &fakeBackend{sendPromptFn: func(context.Context, string) error { return errors.New("prompt failed") }}
	var statuses []domainagent.SessionStatus
	orch := New(Deps{
		Logger: testOrchestratorLogger(),
		Sessions: &testutil.MockAgentRepo{UpdateStatusFn: func(_ context.Context, _ domain.TenantID, _ domain.AgentSessionID, status domainagent.SessionStatus) error {
			statuses = append(statuses, status)
			return nil
		}},
		PubSub: &fakePublisher{},
	})
	orch.sessions[testutil.TestSessionID()] = &runningSession{
		backend:   backend,
		sessionID: testutil.TestSessionID(),
		taskID:    testutil.TestTaskID(),
		tenantID:  testutil.TestTenantID(),
		cancel:    func() {},
	}

	err := orch.SendHITLAnswer(t.Context(), testutil.TestTenantID(), testutil.TestSessionID(), "Ship it")
	require.Error(t, err)
	assert.Equal(t, []domainagent.SessionStatus{domainagent.StatusRunning, domainagent.StatusWaitingHITL}, statuses)
}

func TestOrchestrator_SendHITLAnswer_UnavailableSessionCancelsPendingQuestions(t *testing.T) {
	t.Parallel()

	publisher := &fakePublisher{}
	var statuses []domainagent.SessionStatus
	var transitions []task.TaskStatus
	var cancelledSessions []domain.AgentSessionID
	questionRepo := &testutil.MockHITLRepo{
		CancelPendingBySessionFn: func(_ context.Context, _ domain.TenantID, sessionID domain.AgentSessionID) error {
			cancelledSessions = append(cancelledSessions, sessionID)
			return nil
		},
	}
	orch := New(Deps{
		Logger: testOrchestratorLogger(),
		Sessions: &testutil.MockAgentRepo{
			GetByIDFn: func(context.Context, domain.TenantID, domain.AgentSessionID) (*domainagent.Session, error) {
				return &domainagent.Session{ID: testutil.TestSessionID(), TaskID: testutil.TestTaskID(), Status: domainagent.StatusWaitingHITL}, nil
			},
			UpdateStatusFn: func(context.Context, domain.TenantID, domain.AgentSessionID, domainagent.SessionStatus) error {
				statuses = append(statuses, domainagent.StatusCancelled)
				return nil
			},
		},
		Tasks: &testutil.MockTaskRepo{TransitionFn: func(context.Context, domain.TenantID, domain.TaskID, task.TaskStatus) error {
			transitions = append(transitions, task.StatusBacklog)
			return nil
		}},
		Questions: questionRepo,
		PubSub:    publisher,
	})

	err := orch.SendHITLAnswer(t.Context(), testutil.TestTenantID(), testutil.TestSessionID(), "Ship it")

	require.ErrorIs(t, err, domain.ErrSessionUnavailable)
	assert.Equal(t, []domainagent.SessionStatus{domainagent.StatusCancelled}, statuses)
	assert.Equal(t, []task.TaskStatus{task.StatusBacklog}, transitions)
	assert.Equal(t, []domain.AgentSessionID{testutil.TestSessionID()}, cancelledSessions)
	require.Len(t, publisher.snapshot(), 1)
}

func TestOrchestrator_HandleToolCallRoutesAskHuman(t *testing.T) {
	t.Parallel()

	publisher := &fakePublisher{}
	hitlHandler := &fakeHITLHandler{question: &hitl.Question{ID: domain.NewID()}}
	orch := New(Deps{
		Logger: testOrchestratorLogger(),
		HITL:   hitlHandler,
		PubSub: publisher,
	})

	orch.handleToolCall(t.Context(), testutil.TestTenantID(), testutil.TestSessionID(), testutil.TestProjectID(), agentpkg.Message{
		Type: agentpkg.MessageToolCall,
		ToolCall: &agentpkg.ToolCall{
			ID:    "tool-1",
			Name:  "ask_human",
			Input: json.RawMessage(`{"question":"Ship it?"}`),
		},
	})

	require.Len(t, hitlHandler.inputs, 1)
	events := publisher.snapshot()
	require.Len(t, events, 1)
	payload, ok := events[0].payload.(map[string]any)
	require.True(t, ok)
	assert.Equal(t, true, payload["pending_hitl"])
	assert.Equal(t, hitlHandler.question.ID.String(), payload["question_id"])
}

func TestOrchestrator_HandleToolCallRoutesCreateADRAndNotifies(t *testing.T) {
	t.Parallel()

	publisher := &fakePublisher{}
	notifier := &fakeNotifier{}
	record := &adr.ADR{ID: domain.NewID(), Sequence: 3, Title: "Use pgx", Status: adr.StatusProposed}
	engine := &fakeADREngine{createRecord: record}
	orch := New(Deps{
		Logger:    testOrchestratorLogger(),
		ADREngine: engine,
		Notifier:  notifier,
		PubSub:    publisher,
	})

	orch.handleToolCall(t.Context(), testutil.TestTenantID(), testutil.TestSessionID(), testutil.TestProjectID(), agentpkg.Message{
		Type: agentpkg.MessageToolCall,
		ToolCall: &agentpkg.ToolCall{
			ID:    "tool-2",
			Name:  "create_adr",
			Input: json.RawMessage(`{"title":"Use pgx","context":"Need better Postgres control","decision":"Adopt pgx directly"}`),
		},
	})

	require.Len(t, notifier.created, 1)
	assert.Equal(t, testutil.TestTenantID(), notifier.created[0].tenantID)
	assert.Equal(t, testutil.TestSessionID(), notifier.created[0].sessionID)
	assert.Equal(t, record, notifier.created[0].record)

	events := publisher.snapshot()
	require.Len(t, events, 1)
	payload, ok := events[0].payload.(map[string]any)
	require.True(t, ok)
	assert.Equal(t, record.ID.String(), payload["adr_id"])
	assert.Equal(t, record.Sequence, payload["adr_sequence"])
}

func TestOrchestrator_HandleToolCallCreateADRPublishesEventWhenNotificationFails(t *testing.T) {
	t.Parallel()

	publisher := &fakePublisher{}
	notifier := &fakeNotifier{err: errors.New("slack unavailable")}
	record := &adr.ADR{ID: domain.NewID(), Sequence: 4, Title: "Use sqlc", Status: adr.StatusProposed}
	engine := &fakeADREngine{createRecord: record}
	orch := New(Deps{
		Logger:    testOrchestratorLogger(),
		ADREngine: engine,
		Notifier:  notifier,
		PubSub:    publisher,
	})

	orch.handleToolCall(t.Context(), testutil.TestTenantID(), testutil.TestSessionID(), testutil.TestProjectID(), agentpkg.Message{
		Type: agentpkg.MessageToolCall,
		ToolCall: &agentpkg.ToolCall{
			ID:    "tool-3",
			Name:  "create_adr",
			Input: json.RawMessage(`{"title":"Use sqlc","context":"Need typed queries","decision":"Adopt sqlc"}`),
		},
	})

	require.Len(t, notifier.created, 1)
	events := publisher.snapshot()
	require.Len(t, events, 1)
	payload, ok := events[0].payload.(map[string]any)
	require.True(t, ok)
	assert.Equal(t, record.ID.String(), payload["adr_id"])
	assert.Equal(t, record.Sequence, payload["adr_sequence"])
}

func TestOrchestrator_BuildMessageHandlerPersistsReplayEvent(t *testing.T) {
	t.Parallel()

	publisher := &fakePublisher{}
	events := &fakeEventRepo{}
	orch := New(Deps{
		Logger: testOrchestratorLogger(),
		PubSub: publisher,
		Events: events,
	})

	running := &runningSession{}
	handler := orch.buildMessageHandler(t.Context(), testutil.TestTenantID(), testutil.TestSessionID(), testutil.TestProjectID(), running)
	handler(agentpkg.Message{Type: agentpkg.MessageText, Content: "hello replay"})

	replay := events.snapshot()
	require.Len(t, replay, 1)
	assert.Equal(t, testutil.TestTenantID(), replay[0].TenantID)
	assert.Equal(t, testutil.TestSessionID(), replay[0].SessionID)
	assert.Equal(t, string(redispkg.EventAgentOutput), replay[0].Type)
	assert.JSONEq(t, `{"content":"hello replay","session_id":"`+testutil.TestSessionID().String()+`","type":"text"}`, string(replay[0].Payload))
	assert.WithinDuration(t, time.Now(), time.Unix(0, running.lastEventAt.Load()), time.Second)

	pub := publisher.snapshot()
	require.Len(t, pub, 1)
	assert.Equal(t, redispkg.EventAgentOutput, pub[0].eventType)
}

func TestOrchestrator_HandleSessionCompleteNotifies(t *testing.T) {
	t.Parallel()

	sessionID := domain.NewID()
	backend := &fakeBackend{}
	publisher := &fakePublisher{}
	notifier := &fakeNotifier{}
	orch := New(Deps{
		Logger: testOrchestratorLogger(),
		Tasks: &testutil.MockTaskRepo{TransitionFn: func(context.Context, domain.TenantID, domain.TaskID, task.TaskStatus) error {
			return nil
		}},
		Sessions: &testutil.MockAgentRepo{
			GetByIDFn: func(_ context.Context, _ domain.TenantID, id domain.AgentSessionID) (*domainagent.Session, error) {
				return &domainagent.Session{ID: id, TenantID: testutil.TestTenantID(), TaskID: testutil.TestTaskID()}, nil
			},
			UpdateStatusFn: func(context.Context, domain.TenantID, domain.AgentSessionID, domainagent.SessionStatus) error {
				return nil
			},
		},
		Notifier: notifier,
		PubSub:   publisher,
	})
	orch.sessions[sessionID] = &runningSession{
		backend:   backend,
		sessionID: sessionID,
		taskID:    testutil.TestTaskID(),
		tenantID:  testutil.TestTenantID(),
		cancel:    func() {},
	}

	err := orch.HandleSessionComplete(t.Context(), sessionID, 0)
	require.NoError(t, err)
	require.Len(t, notifier.completed, 1)
	assert.Equal(t, testutil.TestTaskID(), notifier.completed[0].taskID)
	require.Empty(t, notifier.failed)
}

func TestOrchestrator_CancelSession_PersistedWaitingSession(t *testing.T) {
	t.Parallel()

	publisher := &fakePublisher{}
	var statuses []domainagent.SessionStatus
	var transitions []task.TaskStatus
	orch := New(Deps{
		Logger: testOrchestratorLogger(),
		Sessions: &testutil.MockAgentRepo{
			GetByIDFn: func(context.Context, domain.TenantID, domain.AgentSessionID) (*domainagent.Session, error) {
				return &domainagent.Session{ID: testutil.TestSessionID(), TaskID: testutil.TestTaskID(), Status: domainagent.StatusWaitingHITL}, nil
			},
			UpdateStatusFn: func(context.Context, domain.TenantID, domain.AgentSessionID, domainagent.SessionStatus) error {
				statuses = append(statuses, domainagent.StatusCancelled)
				return nil
			},
		},
		Tasks: &testutil.MockTaskRepo{TransitionFn: func(context.Context, domain.TenantID, domain.TaskID, task.TaskStatus) error {
			transitions = append(transitions, task.StatusBacklog)
			return nil
		}},
		PubSub: publisher,
	})

	err := orch.CancelSession(t.Context(), testutil.TestTenantID(), testutil.TestSessionID())
	require.NoError(t, err)
	assert.Equal(t, []domainagent.SessionStatus{domainagent.StatusCancelled}, statuses)
	assert.Equal(t, []task.TaskStatus{task.StatusBacklog}, transitions)
	require.Len(t, publisher.snapshot(), 1)
}

func TestOrchestrator_CancelSession_PersistedTerminalSessionIsNoop(t *testing.T) {
	t.Parallel()

	questionRepo := &testutil.MockHITLRepo{}
	var cancelledPending []domain.AgentSessionID
	questionRepo.CancelPendingBySessionFn = func(_ context.Context, _ domain.TenantID, sessionID domain.AgentSessionID) error {
		cancelledPending = append(cancelledPending, sessionID)
		return nil
	}
	var statusUpdates []domainagent.SessionStatus
	var transitions []task.TaskStatus
	orch := New(Deps{
		Logger: testOrchestratorLogger(),
		Sessions: &testutil.MockAgentRepo{
			GetByIDFn: func(context.Context, domain.TenantID, domain.AgentSessionID) (*domainagent.Session, error) {
				return &domainagent.Session{ID: testutil.TestSessionID(), TaskID: testutil.TestTaskID(), Status: domainagent.StatusCompleted}, nil
			},
			UpdateStatusFn: func(context.Context, domain.TenantID, domain.AgentSessionID, domainagent.SessionStatus) error {
				statusUpdates = append(statusUpdates, domainagent.StatusCancelled)
				return nil
			},
		},
		Tasks: &testutil.MockTaskRepo{TransitionFn: func(context.Context, domain.TenantID, domain.TaskID, task.TaskStatus) error {
			transitions = append(transitions, task.StatusBacklog)
			return nil
		}},
		Questions: questionRepo,
		PubSub:    &fakePublisher{},
	})

	err := orch.CancelSession(t.Context(), testutil.TestTenantID(), testutil.TestSessionID())

	require.NoError(t, err)
	assert.Empty(t, statusUpdates)
	assert.Empty(t, transitions)
	assert.Equal(t, []domain.AgentSessionID{testutil.TestSessionID()}, cancelledPending)
}

func TestOrchestrator_Shutdown_CancelsTrackedSessions(t *testing.T) {
	t.Parallel()

	var mu sync.Mutex
	var taskTransitions []task.TaskStatus
	var sessionStatuses []domainagent.SessionStatus
	backendOne := &fakeBackend{}
	backendTwo := &fakeBackend{}
	orch := New(Deps{
		Logger: testOrchestratorLogger(),
		Tasks: &testutil.MockTaskRepo{
			TransitionFn: func(_ context.Context, _ domain.TenantID, _ domain.TaskID, next task.TaskStatus) error {
				mu.Lock()
				defer mu.Unlock()
				taskTransitions = append(taskTransitions, next)
				return nil
			},
		},
		Sessions: &testutil.MockAgentRepo{
			UpdateStatusFn: func(_ context.Context, _ domain.TenantID, _ domain.AgentSessionID, status domainagent.SessionStatus) error {
				mu.Lock()
				defer mu.Unlock()
				sessionStatuses = append(sessionStatuses, status)
				return nil
			},
		},
	})
	orch.sessions[domain.NewID()] = &runningSession{
		backend:  backendOne,
		taskID:   testutil.TestTaskID(),
		tenantID: testutil.TestTenantID(),
		cancel:   func() {},
	}
	orch.sessions[domain.NewID()] = &runningSession{
		backend:  backendTwo,
		taskID:   domain.NewID(),
		tenantID: testutil.TestTenantID(),
		cancel:   func() {},
	}

	err := orch.Shutdown(t.Context())

	require.NoError(t, err)
	assert.Len(t, taskTransitions, 2)
	assert.ElementsMatch(t, []task.TaskStatus{task.StatusBacklog, task.StatusBacklog}, taskTransitions)
	assert.Len(t, sessionStatuses, 2)
	assert.ElementsMatch(t, []domainagent.SessionStatus{domainagent.StatusCancelled, domainagent.StatusCancelled}, sessionStatuses)
	assert.Equal(t, 1, backendOne.cancelCalls)
	assert.Equal(t, 1, backendTwo.cancelCalls)
	assert.Equal(t, 1, backendOne.disposeCalls)
	assert.Equal(t, 1, backendTwo.disposeCalls)
	assert.Empty(t, orch.ActiveSessions())
}

func TestOrchestrator_HandleSessionCompleteSchedulesRetryOnFailure(t *testing.T) {
	t.Parallel()

	sessionID := domain.NewID()
	backend := &fakeBackend{}
	publisher := &fakePublisher{}
	notifier := &fakeNotifier{}

	persistedSession := &domainagent.Session{
		ID:         sessionID,
		TenantID:   testutil.TestTenantID(),
		TaskID:     testutil.TestTaskID(),
		ProjectID:  testutil.TestProjectID(),
		RetryCount: 0,
		Status:     domainagent.StatusRunning,
		Metadata:   map[string]any{"prompt": "Implement feature"},
	}

	var scheduledRetryCount int
	var scheduledRetryAt *time.Time
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
			GetByIDFn: func(_ context.Context, _ domain.TenantID, _ domain.AgentSessionID) (*domainagent.Session, error) {
				return persistedSession, nil
			},
			UpdateStatusFn: func(_ context.Context, _ domain.TenantID, _ domain.AgentSessionID, status domainagent.SessionStatus) error {
				sessionStatuses = append(sessionStatuses, status)
				return nil
			},
			ScheduleRetryFn: func(_ context.Context, _ domain.TenantID, _ domain.AgentSessionID, retryCount int, retryAt *time.Time) error {
				scheduledRetryCount = retryCount
				scheduledRetryAt = retryAt
				return nil
			},
		},
		Notifier: notifier,
		PubSub:   publisher,
	})
	orch.sessions[sessionID] = &runningSession{
		backend:   backend,
		sessionID: sessionID,
		taskID:    testutil.TestTaskID(),
		tenantID:  testutil.TestTenantID(),
		cancel:    func() {},
	}

	err := orch.HandleSessionComplete(t.Context(), sessionID, 1)

	require.NoError(t, err)
	assert.Equal(t, []domainagent.SessionStatus{domainagent.StatusFailed}, sessionStatuses)
	assert.Equal(t, []task.TaskStatus{task.StatusBacklog}, taskTransitions)
	assert.Equal(t, 1, scheduledRetryCount, "retry count should be persisted.RetryCount + 1")
	assert.NotNil(t, scheduledRetryAt, "retry should be scheduled")
	assert.Empty(t, notifier.failed, "should not notify failure when retry is scheduled")

	events := publisher.snapshot()
	require.Len(t, events, 1)
	payload, ok := events[0].payload.(map[string]any)
	require.True(t, ok)
	assert.Equal(t, true, payload["retry_scheduled"])
}

func TestOrchestrator_HandleSessionCompleteNoRetryWhenMaxAttemptsReached(t *testing.T) {
	t.Parallel()

	sessionID := domain.NewID()
	backend := &fakeBackend{}
	publisher := &fakePublisher{}
	notifier := &fakeNotifier{}

	persistedSession := &domainagent.Session{
		ID:         sessionID,
		TenantID:   testutil.TestTenantID(),
		TaskID:     testutil.TestTaskID(),
		ProjectID:  testutil.TestProjectID(),
		RetryCount: 3, // max attempts reached
		Status:     domainagent.StatusRunning,
		Metadata:   map[string]any{"prompt": "Implement feature"},
	}

	var scheduledRetry bool

	orch := New(Deps{
		Logger: testOrchestratorLogger(),
		Tasks: &testutil.MockTaskRepo{
			TransitionFn: func(_ context.Context, _ domain.TenantID, _ domain.TaskID, _ task.TaskStatus) error {
				return nil
			},
		},
		Sessions: &testutil.MockAgentRepo{
			GetByIDFn: func(_ context.Context, _ domain.TenantID, _ domain.AgentSessionID) (*domainagent.Session, error) {
				return persistedSession, nil
			},
			UpdateStatusFn: func(_ context.Context, _ domain.TenantID, _ domain.AgentSessionID, _ domainagent.SessionStatus) error {
				return nil
			},
			ScheduleRetryFn: func(_ context.Context, _ domain.TenantID, _ domain.AgentSessionID, _ int, _ *time.Time) error {
				scheduledRetry = true
				return nil
			},
		},
		Notifier: notifier,
		PubSub:   publisher,
	})
	orch.sessions[sessionID] = &runningSession{
		backend:   backend,
		sessionID: sessionID,
		taskID:    testutil.TestTaskID(),
		tenantID:  testutil.TestTenantID(),
		cancel:    func() {},
	}

	err := orch.HandleSessionComplete(t.Context(), sessionID, 1)

	require.NoError(t, err)
	assert.False(t, scheduledRetry, "should not schedule retry when max attempts reached")
	require.Len(t, notifier.failed, 1, "should notify failure when no retry")

	events := publisher.snapshot()
	require.Len(t, events, 1)
	payload, ok := events[0].payload.(map[string]any)
	require.True(t, ok)
	assert.Equal(t, false, payload["retry_scheduled"])
}

func TestRetryScheduler_RetryDelay(t *testing.T) {
	t.Parallel()

	rs := &RetryScheduler{
		baseDelay: 10 * time.Second,
		maxDelay:  5 * time.Minute,
	}

	tests := []struct {
		attempt  int
		expected time.Duration
	}{
		{1, 10 * time.Second},
		{2, 20 * time.Second},
		{3, 40 * time.Second},
		{4, 80 * time.Second},
		{10, 5 * time.Minute}, // capped at maxDelay
	}

	for _, tt := range tests {
		got := rs.retryDelay(tt.attempt)
		assert.Equal(t, tt.expected, got, "attempt %d", tt.attempt)
	}
}

func TestRetryScheduler_ShouldRetry(t *testing.T) {
	t.Parallel()

	rs := &RetryScheduler{maxAttempts: 3}

	assert.True(t, rs.shouldRetry(&domainagent.Session{RetryCount: 0}))
	assert.True(t, rs.shouldRetry(&domainagent.Session{RetryCount: 2}))
	assert.False(t, rs.shouldRetry(&domainagent.Session{RetryCount: 3}))
	assert.False(t, rs.shouldRetry(&domainagent.Session{RetryCount: 5}))
	assert.False(t, rs.shouldRetry(nil))
}
