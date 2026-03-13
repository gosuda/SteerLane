// Package orchestrator coordinates agent session lifecycle.
//
// It sits in the Orchestration layer, tying together the agent backend registry,
// Docker runtime, volume manager, gitops, domain repositories, and Redis pub/sub.
// The orchestrator contains no domain logic itself — it delegates to domain entities
// for state transitions and validation.
package orchestrator

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"maps"
	"sync"
	"sync/atomic"
	"time"

	"github.com/google/uuid"
	"golang.org/x/sync/errgroup"

	agentpkg "github.com/gosuda/steerlane/internal/agent"
	"github.com/gosuda/steerlane/internal/docker"
	"github.com/gosuda/steerlane/internal/domain"
	"github.com/gosuda/steerlane/internal/domain/adr"
	agentdom "github.com/gosuda/steerlane/internal/domain/agent"
	"github.com/gosuda/steerlane/internal/domain/hitl"
	"github.com/gosuda/steerlane/internal/domain/project"
	"github.com/gosuda/steerlane/internal/domain/task"
	"github.com/gosuda/steerlane/internal/messenger"
	"github.com/gosuda/steerlane/internal/observability"
	redispkg "github.com/gosuda/steerlane/internal/store/redis"
	"github.com/gosuda/steerlane/internal/volume"
)

type volumeManager interface {
	EnsureVolume(ctx context.Context, projectID domain.ProjectID) (string, error)
}

type gitOperator interface {
	Fetch(ctx context.Context, repoDir string) error
	CreateBranch(ctx context.Context, repoDir string, sessionID domain.AgentSessionID, baseBranch string) (string, error)
}

type agentEventPublisher interface {
	PublishAgentEvent(ctx context.Context, sessionID string, eventType redispkg.EventType, payload any) error
}

type askHumanHandler interface {
	HandleAskHuman(
		ctx context.Context,
		tenantID domain.TenantID,
		sessionID domain.AgentSessionID,
		input json.RawMessage,
	) (*hitl.Question, error)
}

type sessionNotifier interface {
	NotifySessionFailed(ctx context.Context, tenantID domain.TenantID, taskID domain.TaskID, sessionID domain.AgentSessionID, reason string) error
	NotifyTaskCompleted(ctx context.Context, tenantID domain.TenantID, taskID domain.TaskID, sessionID domain.AgentSessionID) error
	NotifyADRCreated(ctx context.Context, tenantID domain.TenantID, sessionID domain.AgentSessionID, record *adr.ADR) error
}

type sessionContextCleaner interface {
	Put(tenantID domain.TenantID, sessionID domain.AgentSessionID, ctx messenger.SessionContext)
	Delete(tenantID domain.TenantID, sessionID domain.AgentSessionID)
}

// adrEngineHandler defines the interface for ADR creation tool handling.
type adrEngineHandler interface {
	HandleCreateADR(ctx context.Context, tenantID domain.TenantID, projectID domain.ProjectID, sessionID domain.AgentSessionID, input json.RawMessage) (*adr.ADR, error)
	ExtractFromSession(ctx context.Context, tenantID domain.TenantID, projectID domain.ProjectID, sessionID domain.AgentSessionID, metadata map[string]any) ([]*adr.ADR, error)
}

// Deps holds the injected dependencies for the Orchestrator.
type Deps struct {
	Logger    *slog.Logger
	Registry  *agentpkg.Registry
	Runtime   docker.RuntimeManager
	Volumes   volumeManager
	GitOps    gitOperator
	Projects  project.Repository
	Tasks     task.Repository
	Sessions  agentdom.Repository
	Questions hitl.Repository
	ADRs      adr.Repository
	Events    agentdom.EventRepository
	PubSub    agentEventPublisher
	HITL      askHumanHandler
	Notifier  sessionNotifier
	Threads   sessionContextCleaner
	ADREngine adrEngineHandler
}

// Orchestrator coordinates agent session lifecycle across infrastructure
// and domain boundaries.
type Orchestrator struct {
	deps           Deps
	logger         *slog.Logger
	sessions       map[domain.AgentSessionID]*runningSession
	lastFetchTimes map[domain.ProjectID]time.Time
	stallDetector  *StallDetector
	retryScheduler *RetryScheduler
	reconciler     *Reconciler
	mu             sync.Mutex
}

// runningSession tracks a currently active agent session.
//
//nolint:govet // field grouping prioritises semantic clarity over alignment savings.
type runningSession struct {
	backend     agentpkg.Backend
	cancel      context.CancelFunc
	lastEventAt atomic.Int64
	containerID string
	sessionID   domain.AgentSessionID
	taskID      domain.TaskID
	projectID   domain.ProjectID
	tenantID    domain.TenantID
}

const (
	operationResultSuccess = "success"
	operationResultError   = "error"
)

// New creates an Orchestrator with the given dependencies.
// If deps.Logger is nil, slog.Default() is used.
func New(deps Deps) *Orchestrator {
	logger := deps.Logger
	if logger == nil {
		logger = slog.Default()
	}
	orch := &Orchestrator{
		deps:           deps,
		logger:         logger.With("component", "orchestrator"),
		sessions:       make(map[domain.AgentSessionID]*runningSession),
		lastFetchTimes: make(map[domain.ProjectID]time.Time),
	}
	orch.stallDetector = NewStallDetector(orch, 0, 0)
	orch.retryScheduler = NewRetryScheduler(orch, 0, 0, 0, 0)
	orch.reconciler = NewReconciler(orch, 0)
	return orch
}

// DispatchTask creates an agent session for a task and starts the agent backend.
//
// Steps:
//  1. Load and validate the task (must be in backlog).
//  2. Load the project.
//  3. Create a pending agent session.
//  4. Transition the task to in_progress.
//  5. Link the session to the task.
//  6. Ensure the Docker volume exists.
//  7. Set up the git branch via gitops.
//  8. Create and configure the agent backend.
//  9. Transition the session to running and start it.
//  10. Publish SessionStarted and register in the sessions map.
func (o *Orchestrator) DispatchTask(
	ctx context.Context,
	tenantID domain.TenantID,
	taskID domain.TaskID,
	agentType agentdom.AgentType,
	prompt string,
) (domain.AgentSessionID, error) {
	return o.dispatchTaskAttempt(ctx, tenantID, taskID, agentType, prompt, nil, 0)
}

// DispatchTaskWithContext pre-registers messenger session context before the
// backend starts, avoiding races with immediate ask_human tool calls.
func (o *Orchestrator) DispatchTaskWithContext(
	ctx context.Context,
	tenantID domain.TenantID,
	taskID domain.TaskID,
	agentType agentdom.AgentType,
	prompt string,
	sessionCtx messenger.SessionContext,
) (domain.AgentSessionID, error) {
	return o.dispatchTaskAttempt(ctx, tenantID, taskID, agentType, prompt, &sessionCtx, 0)
}

func (o *Orchestrator) dispatchTaskAttempt(
	ctx context.Context,
	tenantID domain.TenantID,
	taskID domain.TaskID,
	agentType agentdom.AgentType,
	prompt string,
	sessionCtx *messenger.SessionContext,
	retryCount int,
) (domain.AgentSessionID, error) {
	const op = "orchestrator.DispatchTask"
	result := operationResultSuccess
	var spanErr error
	ctx, finishSpan := observability.StartSpan(ctx, o.logger, "orchestrator.dispatch_task", map[string]any{
		"task_id":     taskID.String(),
		"tenant_id":   tenantID.String(),
		"agent_type":  string(agentType),
		"retry_count": retryCount,
	})
	defer func() {
		observability.RecordOperation("dispatch_task", result)
		if result == operationResultError && spanErr == nil {
			spanErr = errors.New("dispatch task failed")
		}
		finishSpan(spanErr, map[string]any{"result": result})
	}()
	now := time.Now().UTC()

	// Step 1: load and validate the task.
	t, err := o.deps.Tasks.GetByID(ctx, tenantID, taskID)
	if err != nil {
		result = operationResultError
		spanErr = err
		return domain.AgentSessionID{}, fmt.Errorf("%s: load task: %w", op, err)
	}
	if t.Status != task.StatusBacklog {
		result = operationResultError
		spanErr = domain.ErrInvalidTransition
		return domain.AgentSessionID{}, fmt.Errorf("%s: task %s is %q, want %q: %w",
			op, taskID, t.Status, task.StatusBacklog, domain.ErrInvalidTransition)
	}

	// Step 2: load the project.
	proj, err := o.deps.Projects.GetByID(ctx, tenantID, t.ProjectID)
	if err != nil {
		result = operationResultError
		spanErr = err
		return domain.AgentSessionID{}, fmt.Errorf("%s: load project: %w", op, err)
	}

	// Step 3: create a pending agent session.
	sessionID := domain.NewID()
	session := &agentdom.Session{
		ID:         sessionID,
		TenantID:   tenantID,
		ProjectID:  proj.ID,
		TaskID:     taskID,
		AgentType:  agentType,
		Status:     agentdom.StatusPending,
		RetryCount: retryCount,
		CreatedAt:  now,
		Metadata: map[string]any{
			"prompt": prompt,
		},
	}
	if err = o.deps.Sessions.Create(ctx, session); err != nil {
		result = operationResultError
		spanErr = err
		return domain.AgentSessionID{}, fmt.Errorf("%s: create session: %w", op, err)
	}

	// From here, any error must fail the session.
	taskTransitioned := false
	cleanup := func(cleanupErr error) {
		o.failSession(ctx, tenantID, sessionID, taskID, cleanupErr)
		if o.deps.Threads != nil {
			o.deps.Threads.Delete(tenantID, sessionID)
		}
		if taskTransitioned {
			o.revertTaskToBacklog(ctx, tenantID, taskID)
		}
	}
	if sessionCtx != nil && o.deps.Threads != nil {
		o.deps.Threads.Put(tenantID, sessionID, *sessionCtx)
	}

	// Step 4: transition the task backlog -> in_progress.
	if err = o.deps.Tasks.Transition(ctx, tenantID, taskID, task.StatusInProgress); err != nil {
		result = operationResultError
		spanErr = err
		cleanup(err)
		return domain.AgentSessionID{}, fmt.Errorf("%s: transition task: %w", op, err)
	}
	taskTransitioned = true

	// Step 5: link the session to the task.
	t.AgentSessionID = &sessionID
	t.UpdatedAt = now
	if err = o.deps.Tasks.Update(ctx, t); err != nil {
		result = operationResultError
		spanErr = err
		cleanup(err)
		return domain.AgentSessionID{}, fmt.Errorf("%s: link session to task: %w", op, err)
	}

	// Step 6: ensure the Docker volume exists for the project.
	volumeName, err := o.deps.Volumes.EnsureVolume(ctx, proj.ID)
	if err != nil {
		result = operationResultError
		spanErr = err
		cleanup(err)
		return domain.AgentSessionID{}, fmt.Errorf("%s: ensure volume: %w", op, err)
	}

	// Step 7: git operations — fetch latest and create session branch.
	// TODO(1B): Git operations need to run inside a sidecar container because
	// volumes are Docker volumes (not bind mounts) and are not directly accessible
	// from the host. For now, use a placeholder host path convention.
	baseRepoDir := "/mnt/volumes/" + volumeName
	repoDir, err := proj.ResolveWorkspace(baseRepoDir)
	if err != nil {
		result = operationResultError
		spanErr = err
		cleanup(err)
		return domain.AgentSessionID{}, fmt.Errorf("%s: resolve workspace: %w", op, err)
	}

	// Check staleness before fetching.
	var lastFetch *time.Time
	o.mu.Lock()
	if fetchedAt, exists := o.lastFetchTimes[proj.ID]; exists {
		lastFetch = &fetchedAt
	}
	o.mu.Unlock()

	switch {
	case volume.IsStale(lastFetch, volume.StalenessThreshold):
		if err = o.deps.GitOps.Fetch(ctx, baseRepoDir); err != nil {
			// Non-fatal on first run (volume may be freshly created with no clone yet).
			o.logger.WarnContext(ctx, "git fetch failed (may be first run)",
				"error", err,
				"session_id", sessionID,
				"repo_dir", repoDir,
			)
		} else {
			// Update fetch timestamp on success.
			o.mu.Lock()
			o.lastFetchTimes[proj.ID] = time.Now()
			o.mu.Unlock()
			o.logger.InfoContext(ctx, "volume fetched successfully",
				"project_id", proj.ID,
				"repo_dir", repoDir,
			)
		}
	case lastFetch != nil:
		o.logger.InfoContext(ctx, "volume not stale, skipping fetch",
			"project_id", proj.ID,
			"repo_dir", repoDir,
			"last_fetched", lastFetch.Format(time.RFC3339),
		)
	default:
		o.logger.InfoContext(ctx, "volume not stale, skipping fetch",
			"project_id", proj.ID,
			"repo_dir", repoDir,
		)
	}

	branchName, err := o.deps.GitOps.CreateBranch(ctx, baseRepoDir, sessionID, proj.Branch)
	if err != nil {
		result = operationResultError
		spanErr = err
		cleanup(err)
		return domain.AgentSessionID{}, fmt.Errorf("%s: create branch: %w", op, err)
	}

	// Step 8: create the agent backend via the registry.
	backend, err := o.deps.Registry.Create(agentType, o.logger.With(
		"session_id", sessionID,
		"task_id", taskID,
	))
	if err != nil {
		result = operationResultError
		spanErr = err
		cleanup(err)
		return domain.AgentSessionID{}, fmt.Errorf("%s: create backend: %w", op, err)
	}
	runtimeCtx, sessionCancel := context.WithCancel(context.Background()) //nolint:gosec // cancellation is stored in runningSession and invoked on stop paths
	started := false
	defer func() {
		if !started {
			sessionCancel()
		}
	}()

	running := &runningSession{
		backend:   backend,
		sessionID: sessionID,
		taskID:    taskID,
		projectID: proj.ID,
		tenantID:  tenantID,
		cancel:    sessionCancel,
	}
	running.lastEventAt.Store(time.Now().UTC().UnixNano())

	// Step 9: register message handler.
	backend.OnMessage(o.buildMessageHandler(runtimeCtx, tenantID, sessionID, proj.ID, running))

	// Step 10: transition session to running and persist.
	if err = o.deps.Sessions.UpdateStatus(ctx, tenantID, sessionID, agentdom.StatusRunning); err != nil {
		result = operationResultError
		spanErr = err
		cleanup(err)
		_ = backend.Dispose()
		return domain.AgentSessionID{}, fmt.Errorf("%s: transition session to running: %w", op, err)
	}

	// Step 11: start the backend.
	opts := agentpkg.SessionOpts{
		SessionID:  sessionID,
		ProjectID:  proj.ID,
		TaskID:     taskID,
		TenantID:   tenantID,
		Prompt:     prompt,
		Env:        buildWorkspaceEnv(proj),
		RepoPath:   repoDir,
		BranchName: branchName,
	}
	if err = backend.StartSession(runtimeCtx, opts); err != nil {
		result = operationResultError
		spanErr = err
		cleanup(err)
		_ = backend.Dispose()
		return domain.AgentSessionID{}, fmt.Errorf("%s: start session: %w", op, err)
	}
	started = true

	// Step 12: publish SessionStarted.
	o.publishEvent(ctx, tenantID, sessionID.String(), redispkg.EventSessionStarted, map[string]any{
		"session_id": sessionID.String(),
		"task_id":    taskID.String(),
		"project_id": proj.ID.String(),
		"agent_type": string(agentType),
	})

	// Step 13: register the running session.
	// We don't spawn a goroutine here; the monitoring/wait loop is wired by the API layer.
	o.mu.Lock()
	o.sessions[sessionID] = running
	o.mu.Unlock()

	o.logger.InfoContext(ctx, "task dispatched",
		"session_id", sessionID,
		"task_id", taskID,
		"project_id", proj.ID,
		"agent_type", string(agentType),
		"branch", branchName,
		"volume", volumeName,
	)

	return sessionID, nil
}

// CancelSession gracefully cancels a running agent session and returns its
// task to the backlog.
func (o *Orchestrator) CancelSession(
	ctx context.Context,
	tenantID domain.TenantID,
	sessionID domain.AgentSessionID,
) error {
	const op = "orchestrator.CancelSession"
	result := operationResultSuccess
	var spanErr error
	ctx, finishSpan := observability.StartSpan(ctx, o.logger, "orchestrator.cancel_session", map[string]any{
		"session_id": sessionID.String(),
		"tenant_id":  tenantID.String(),
	})
	defer func() {
		observability.RecordOperation("cancel_session", result)
		if result == operationResultError && spanErr == nil {
			spanErr = errors.New("cancel session failed")
		}
		finishSpan(spanErr, map[string]any{"result": result})
	}()

	rs, err := o.lookupSession(sessionID)
	if err != nil {
		if !errors.Is(err, domain.ErrNotFound) {
			result = operationResultError
			spanErr = err
			return fmt.Errorf("%s: %w", op, err)
		}
		persisted, loadErr := o.deps.Sessions.GetByID(ctx, tenantID, sessionID)
		if loadErr != nil {
			result = operationResultError
			spanErr = loadErr
			return fmt.Errorf("%s: load persisted session: %w", op, loadErr)
		}
		if persisted.Status == agentdom.StatusCancelled {
			o.revertTaskToBacklog(ctx, tenantID, persisted.TaskID)
			o.cancelPendingHITL(ctx, tenantID, sessionID)
			if o.deps.Threads != nil {
				o.deps.Threads.Delete(tenantID, sessionID)
			}
			return nil
		}
		if persisted.Status == agentdom.StatusCompleted || persisted.Status == agentdom.StatusFailed {
			o.cancelPendingHITL(ctx, tenantID, sessionID)
			if o.deps.Threads != nil {
				o.deps.Threads.Delete(tenantID, sessionID)
			}
			return nil
		}
		if statusErr := o.deps.Sessions.UpdateStatus(ctx, tenantID, sessionID, agentdom.StatusCancelled); statusErr != nil {
			result = operationResultError
			spanErr = statusErr
			return fmt.Errorf("%s: cancel persisted session: %w", op, statusErr)
		}
		o.revertTaskToBacklog(ctx, tenantID, persisted.TaskID)
		o.cancelPendingHITL(ctx, tenantID, sessionID)
		if o.deps.Threads != nil {
			o.deps.Threads.Delete(tenantID, sessionID)
		}
		o.publishEvent(ctx, tenantID, sessionID.String(), redispkg.EventSessionEnded, map[string]any{
			"session_id": sessionID.String(),
			"task_id":    persisted.TaskID.String(),
			"status":     string(agentdom.StatusCancelled),
		})
		return nil
	}

	// Cancel the backend.
	if cancelErr := rs.backend.Cancel(ctx); cancelErr != nil {
		o.logger.ErrorContext(ctx, "backend cancel failed",
			"error", cancelErr,
			"session_id", sessionID,
		)
	}

	// Transition session to cancelled.
	if err = o.deps.Sessions.UpdateStatus(ctx, tenantID, sessionID, agentdom.StatusCancelled); err != nil {
		result = operationResultError
		spanErr = err
		return fmt.Errorf("%s: update session status: %w", op, err)
	}

	// Return the task to backlog.
	o.revertTaskToBacklog(ctx, tenantID, rs.taskID)
	o.cancelPendingHITL(ctx, tenantID, sessionID)

	// Dispose backend and remove from map.
	o.disposeSession(sessionID, rs)

	// Publish session ended.
	o.publishEvent(ctx, tenantID, sessionID.String(), redispkg.EventSessionEnded, map[string]any{
		"session_id": sessionID.String(),
		"task_id":    rs.taskID.String(),
		"status":     string(agentdom.StatusCancelled),
	})

	o.logger.InfoContext(ctx, "session cancelled",
		"session_id", sessionID,
		"task_id", rs.taskID,
	)

	return nil
}

// HandleSessionComplete is called when a container exits. It transitions the
// session and task based on the exit code.
//
//   - exitCode == 0: session completed, task moves to review.
//   - exitCode != 0: session failed, task returns to backlog.
func (o *Orchestrator) HandleSessionComplete(
	ctx context.Context,
	sessionID domain.AgentSessionID,
	exitCode int64,
) error {
	const op = "orchestrator.HandleSessionComplete"
	result := operationResultSuccess
	var spanErr error
	ctx, finishSpan := observability.StartSpan(ctx, o.logger, "orchestrator.handle_session_complete", map[string]any{
		"session_id": sessionID.String(),
		"exit_code":  exitCode,
	})
	defer func() {
		observability.RecordOperation("handle_session_complete", result)
		if result == operationResultError && spanErr == nil {
			spanErr = errors.New("handle session complete failed")
		}
		finishSpan(spanErr, map[string]any{"result": result})
	}()

	rs, err := o.lookupSession(sessionID)
	if err != nil {
		result = operationResultError
		spanErr = err
		return fmt.Errorf("%s: %w", op, err)
	}

	persisted, err := o.deps.Sessions.GetByID(ctx, rs.tenantID, sessionID)
	if err != nil {
		result = operationResultError
		spanErr = err
		return fmt.Errorf("%s: load session: %w", op, err)
	}

	var sessionStatus agentdom.SessionStatus
	var taskStatus task.TaskStatus
	var retryAt *time.Time
	shouldNotifyFailure := true

	if exitCode == 0 {
		sessionStatus = agentdom.StatusCompleted
		taskStatus = task.StatusReview

		// Attempt heuristic ADR extraction as best-effort post-processing.
		if o.deps.ADREngine != nil {
			persistedSession, getErr := o.deps.Sessions.GetByID(ctx, rs.tenantID, sessionID)
			if getErr != nil {
				o.logger.WarnContext(ctx, "failed to get session for adr extraction", "error", getErr, "session_id", sessionID)
			} else {
				if _, extractErr := o.deps.ADREngine.ExtractFromSession(ctx, rs.tenantID, rs.projectID, sessionID, persistedSession.Metadata); extractErr != nil {
					o.logger.WarnContext(ctx, "failed to extract ADRs from session", "error", extractErr, "session_id", sessionID)
				}
			}
		}
	} else {
		sessionStatus = agentdom.StatusFailed
		taskStatus = task.StatusBacklog
		if o.retryScheduler != nil && o.retryScheduler.shouldRetry(persisted) {
			shouldNotifyFailure = false
			nextRetryAt := o.retryScheduler.nextRetryAt(time.Now().UTC(), persisted.RetryCount+1)
			retryAt = &nextRetryAt
		}
	}

	if err = o.deps.Sessions.UpdateStatus(ctx, rs.tenantID, sessionID, sessionStatus); err != nil {
		result = operationResultError
		spanErr = err
		return fmt.Errorf("%s: update session status: %w", op, err)
	}
	if retryAt != nil {
		if err = o.deps.Sessions.ScheduleRetry(ctx, rs.tenantID, sessionID, persisted.RetryCount+1, retryAt); err != nil {
			result = operationResultError
			spanErr = err
			return fmt.Errorf("%s: schedule retry: %w", op, err)
		}
	}

	// Transition task.
	if err = o.deps.Tasks.Transition(ctx, rs.tenantID, rs.taskID, taskStatus); err != nil {
		o.logger.ErrorContext(ctx, "task transition failed after session complete",
			"error", err,
			"session_id", sessionID,
			"task_id", rs.taskID,
			"target_status", string(taskStatus),
		)
	}
	o.cancelPendingHITL(ctx, rs.tenantID, sessionID)

	// Dispose and remove.
	o.disposeSession(sessionID, rs)

	// Publish session ended.
	o.publishEvent(ctx, rs.tenantID, sessionID.String(), redispkg.EventSessionEnded, map[string]any{
		"session_id":      sessionID.String(),
		"task_id":         rs.taskID.String(),
		"exit_code":       exitCode,
		"status":          string(sessionStatus),
		"retry_scheduled": retryAt != nil,
		"retry_count":     persisted.RetryCount + btoi(retryAt != nil),
		"retry_at":        retryAt,
	})

	o.logger.InfoContext(ctx, "session completed",
		"session_id", sessionID,
		"task_id", rs.taskID,
		"exit_code", exitCode,
		"session_status", string(sessionStatus),
		"task_status", string(taskStatus),
	)

	if o.deps.Notifier != nil {
		if exitCode == 0 {
			if notifyErr := o.deps.Notifier.NotifyTaskCompleted(ctx, rs.tenantID, rs.taskID, sessionID); notifyErr != nil {
				o.logger.ErrorContext(ctx, "failed to send task completed notification",
					"error", notifyErr,
					"session_id", sessionID,
					"task_id", rs.taskID,
				)
			}
		} else if shouldNotifyFailure {
			reason := fmt.Sprintf("agent exited with code %d", exitCode)
			if notifyErr := o.deps.Notifier.NotifySessionFailed(ctx, rs.tenantID, rs.taskID, sessionID, reason); notifyErr != nil {
				o.logger.ErrorContext(ctx, "failed to send session failed notification",
					"error", notifyErr,
					"session_id", sessionID,
					"task_id", rs.taskID,
				)
			}
		}
	}

	return nil
}

// SendHITLAnswer delivers a human answer to a waiting agent session and
// resumes execution.
func (o *Orchestrator) SendHITLAnswer(
	ctx context.Context,
	tenantID domain.TenantID,
	sessionID domain.AgentSessionID,
	answer string,
) error {
	const op = "orchestrator.SendHITLAnswer"

	rs, err := o.lookupSession(sessionID)
	if err != nil {
		if !errors.Is(err, domain.ErrNotFound) {
			return fmt.Errorf("%s: %w", op, err)
		}
		persisted, loadErr := o.deps.Sessions.GetByID(ctx, tenantID, sessionID)
		if loadErr != nil {
			return fmt.Errorf("%s: load persisted session: %w", op, loadErr)
		}
		if persisted.Status == agentdom.StatusWaitingHITL {
			if statusErr := o.deps.Sessions.UpdateStatus(ctx, tenantID, sessionID, agentdom.StatusCancelled); statusErr != nil {
				return fmt.Errorf("%s: cancel unavailable session: %w", op, statusErr)
			}
			o.revertTaskToBacklog(ctx, tenantID, persisted.TaskID)
			o.cancelPendingHITL(ctx, tenantID, sessionID)
			if o.deps.Threads != nil {
				o.deps.Threads.Delete(tenantID, sessionID)
			}
			o.publishEvent(ctx, tenantID, sessionID.String(), redispkg.EventSessionEnded, map[string]any{
				"session_id": sessionID.String(),
				"task_id":    persisted.TaskID.String(),
				"status":     string(agentdom.StatusCancelled),
				"event":      "hitl_resume_unavailable",
			})
		}
		return fmt.Errorf("%s: %w", op, domain.ErrSessionUnavailable)
	}

	// Transition session from waiting_hitl to running.
	if err = o.deps.Sessions.UpdateStatus(ctx, tenantID, sessionID, agentdom.StatusRunning); err != nil {
		return fmt.Errorf("%s: transition to running: %w", op, err)
	}

	// Send the answer to the backend.
	if err = rs.backend.SendPrompt(ctx, answer); err != nil {
		if rollbackErr := o.deps.Sessions.UpdateStatus(ctx, tenantID, sessionID, agentdom.StatusWaitingHITL); rollbackErr != nil {
			return fmt.Errorf("%s: send prompt: %w (rollback failed: %w)", op, err, rollbackErr)
		}
		return fmt.Errorf("%s: send prompt: %w", op, err)
	}

	// Publish status change.
	o.publishEvent(ctx, tenantID, sessionID.String(), redispkg.EventAgentStatus, map[string]any{
		"session_id": sessionID.String(),
		"status":     string(agentdom.StatusRunning),
		"event":      "hitl_answered",
	})

	o.logger.InfoContext(ctx, "HITL answer sent",
		"session_id", sessionID,
		"answer_len", len(answer),
	)

	return nil
}

// ActiveSessions returns the IDs of all currently tracked sessions.
func (o *Orchestrator) ActiveSessions() []domain.AgentSessionID {
	o.mu.Lock()
	defer o.mu.Unlock()

	ids := make([]domain.AgentSessionID, 0, len(o.sessions))
	for id := range o.sessions {
		ids = append(ids, id)
	}
	return ids
}

// StartStallDetector starts the background stalled-session sweep loop.
func (o *Orchestrator) StartStallDetector(ctx context.Context) {
	if o == nil || o.stallDetector == nil {
		return
	}
	o.stallDetector.Start(ctx)
}

// StartRetryScheduler starts the background retry sweep loop.
func (o *Orchestrator) StartRetryScheduler(ctx context.Context) {
	if o == nil || o.retryScheduler == nil {
		return
	}
	o.retryScheduler.Start(ctx)
}

// Shutdown gracefully cancels all running sessions. It respects the context
// deadline for orderly teardown.
func (o *Orchestrator) Shutdown(ctx context.Context) error {
	const op = "orchestrator.Shutdown"

	o.mu.Lock()
	snapshot := make(map[domain.AgentSessionID]*runningSession, len(o.sessions))
	maps.Copy(snapshot, o.sessions)
	o.mu.Unlock()

	if len(snapshot) == 0 {
		o.logger.InfoContext(ctx, "shutdown: no active sessions")
		return nil
	}

	o.logger.InfoContext(ctx, "shutdown: cancelling sessions",
		"count", len(snapshot),
	)

	g, gCtx := errgroup.WithContext(ctx)
	for id, rs := range snapshot {
		g.Go(func() error {
			// Cancel the context for this session.
			rs.cancel()

			// Cancel the backend.
			if cancelErr := rs.backend.Cancel(gCtx); cancelErr != nil {
				o.logger.ErrorContext(gCtx, "shutdown: backend cancel failed",
					"error", cancelErr,
					"session_id", id,
				)
			}

			// Best-effort: transition session to cancelled.
			if statusErr := o.deps.Sessions.UpdateStatus(gCtx, rs.tenantID, id, agentdom.StatusCancelled); statusErr != nil {
				o.logger.ErrorContext(gCtx, "shutdown: session status update failed",
					"error", statusErr,
					"session_id", id,
				)
			}

			// Best-effort: revert task to backlog.
			o.revertTaskToBacklog(gCtx, rs.tenantID, rs.taskID)
			o.cancelPendingHITL(gCtx, rs.tenantID, id)

			// Dispose the backend.
			if disposeErr := rs.backend.Dispose(); disposeErr != nil {
				o.logger.ErrorContext(gCtx, "shutdown: backend dispose failed",
					"error", disposeErr,
					"session_id", id,
				)
			}

			if o.deps.Threads != nil {
				o.deps.Threads.Delete(rs.tenantID, id)
			}

			return nil
		})
	}

	err := g.Wait()

	// Clear the sessions map.
	o.mu.Lock()
	clear(o.sessions)
	o.mu.Unlock()

	o.logger.InfoContext(ctx, "shutdown complete")

	if err != nil {
		return fmt.Errorf("%s: %w", op, err)
	}
	return nil
}

// ---------------------------------------------------------------------------
// Internal helpers
// ---------------------------------------------------------------------------

// lookupSession retrieves a running session from the map.
func (o *Orchestrator) lookupSession(sessionID domain.AgentSessionID) (*runningSession, error) {
	o.mu.Lock()
	defer o.mu.Unlock()

	rs, ok := o.sessions[sessionID]
	if !ok {
		return nil, fmt.Errorf("session %s: %w", sessionID, domain.ErrNotFound)
	}
	return rs, nil
}

// disposeSession cleans up a session's backend and removes it from the map.
func (o *Orchestrator) disposeSession(sessionID domain.AgentSessionID, rs *runningSession) {
	rs.cancel()

	if disposeErr := rs.backend.Dispose(); disposeErr != nil {
		o.logger.Error("backend dispose failed",
			"error", disposeErr,
			"session_id", sessionID,
		)
	}

	o.mu.Lock()
	delete(o.sessions, sessionID)
	o.mu.Unlock()

	if o.deps.Threads != nil {
		o.deps.Threads.Delete(rs.tenantID, sessionID)
	}
}

// failSession transitions a session to failed and publishes the event.
func (o *Orchestrator) failSession(
	ctx context.Context,
	tenantID domain.TenantID,
	sessionID domain.AgentSessionID,
	taskID domain.TaskID,
	cause error,
) {
	if statusErr := o.deps.Sessions.UpdateStatus(ctx, tenantID, sessionID, agentdom.StatusFailed); statusErr != nil {
		o.logger.ErrorContext(ctx, "failed to mark session as failed",
			"error", statusErr,
			"session_id", sessionID,
		)
	}

	errMsg := ""
	if cause != nil {
		errMsg = cause.Error()
	}

	o.publishEvent(ctx, tenantID, sessionID.String(), redispkg.EventSessionEnded, map[string]any{
		"session_id": sessionID.String(),
		"task_id":    taskID.String(),
		"status":     string(agentdom.StatusFailed),
		"error":      errMsg,
	})

	if o.deps.Notifier != nil {
		if notifyErr := o.deps.Notifier.NotifySessionFailed(ctx, tenantID, taskID, sessionID, errMsg); notifyErr != nil {
			o.logger.ErrorContext(ctx, "failed to send session failed notification",
				"error", notifyErr,
				"session_id", sessionID,
				"task_id", taskID,
			)
		}
	}
}

// revertTaskToBacklog transitions a task back to backlog. Errors are logged
// but not returned because this is a best-effort compensating action.
func (o *Orchestrator) revertTaskToBacklog(
	ctx context.Context,
	tenantID domain.TenantID,
	taskID domain.TaskID,
) {
	if err := o.deps.Tasks.Transition(ctx, tenantID, taskID, task.StatusBacklog); err != nil {
		o.logger.ErrorContext(ctx, "failed to revert task to backlog",
			"error", err,
			"task_id", taskID,
		)
	}
}

func (o *Orchestrator) cancelPendingHITL(ctx context.Context, tenantID domain.TenantID, sessionID domain.AgentSessionID) {
	if o.deps.Questions == nil {
		return
	}
	if err := o.deps.Questions.CancelPendingBySession(ctx, tenantID, sessionID); err != nil {
		o.logger.ErrorContext(ctx, "failed to cancel pending HITL questions",
			"error", err,
			"session_id", sessionID,
		)
	}
}

// publishEvent publishes a typed event to the agent Redis channel.
// Errors are logged but not propagated — pub/sub is best-effort.
func (o *Orchestrator) publishEvent(
	ctx context.Context,
	tenantID domain.TenantID,
	sessionID string,
	eventType redispkg.EventType,
	payload any,
) {
	if o.deps.Events == nil {
		observability.RecordAgentEvent(string(eventType), "skipped")
	} else {
		encoded, err := json.Marshal(payload)
		if err != nil {
			o.logger.ErrorContext(ctx, "failed to encode agent event for replay",
				"error", err,
				"event_type", string(eventType),
				"session_id", sessionID,
			)
			observability.RecordAgentEvent(string(eventType), "error")
			return
		}

		sessionUUID, parseErr := uuid.Parse(sessionID)
		if parseErr != nil {
			o.logger.ErrorContext(ctx, "failed to parse session id for replay persistence",
				"error", parseErr,
				"session_id", sessionID,
			)
			observability.RecordAgentEvent(string(eventType), "error")
			return
		}

		record := &agentdom.Event{
			ID:        domain.NewID(),
			TenantID:  tenantID,
			SessionID: sessionUUID,
			Type:      string(eventType),
			Payload:   encoded,
			CreatedAt: time.Now().UTC(),
		}
		if appendErr := o.deps.Events.Append(ctx, record); appendErr != nil {
			o.logger.ErrorContext(ctx, "failed to persist agent event for replay",
				"error", appendErr,
				"event_type", string(eventType),
				"session_id", sessionID,
			)
			observability.RecordAgentEvent(string(eventType), "error")
			return
		}
	}

	if o.deps.PubSub == nil {
		observability.RecordAgentEvent(string(eventType), "success")
		return
	}
	if err := o.deps.PubSub.PublishAgentEvent(ctx, sessionID, eventType, payload); err != nil {
		observability.RecordAgentEvent(string(eventType), "error")
		o.logger.ErrorContext(ctx, "failed to publish event",
			"error", err,
			"event_type", string(eventType),
			"session_id", sessionID,
		)
		return
	}
	observability.RecordAgentEvent(string(eventType), "success")
}

// buildMessageHandler creates the MessageHandler callback for an agent backend.
// It routes agent output to Redis pub/sub and intercepts known tool calls.
func (o *Orchestrator) buildMessageHandler(
	ctx context.Context,
	tenantID domain.TenantID,
	sessionID domain.AgentSessionID,
	projectID domain.ProjectID,
	session *runningSession,
) agentpkg.MessageHandler {
	sid := sessionID.String()

	return func(msg agentpkg.Message) {
		if session != nil {
			session.lastEventAt.Store(time.Now().UTC().UnixNano())
		}

		switch msg.Type {
		case agentpkg.MessageText, agentpkg.MessageToolResult:
			// Forward text output to subscribers.
			o.publishEvent(ctx, tenantID, sid, redispkg.EventAgentOutput, map[string]any{
				"session_id": sid,
				"type":       string(msg.Type),
				"content":    msg.Content,
			})

		case agentpkg.MessageToolCall:
			if msg.ToolCall == nil {
				return
			}
			o.handleToolCall(ctx, tenantID, sessionID, projectID, msg)

		case agentpkg.MessageStatus:
			o.publishEvent(ctx, tenantID, sid, redispkg.EventAgentStatus, map[string]any{
				"session_id": sid,
				"status":     msg.Content,
			})

		case agentpkg.MessageError:
			o.logger.ErrorContext(ctx, "agent error",
				"session_id", sessionID,
				"error", msg.Content,
			)
			o.publishEvent(ctx, tenantID, sid, redispkg.EventAgentOutput, map[string]any{
				"session_id": sid,
				"type":       string(msg.Type),
				"content":    msg.Content,
			})

		case agentpkg.MessageTokenUsage:
			if msg.TokenUsage != nil {
				o.logger.InfoContext(ctx, "token usage",
					"session_id", sessionID,
					"input_tokens", msg.TokenUsage.InputTokens,
					"output_tokens", msg.TokenUsage.OutputTokens,
				)
				o.publishEvent(ctx, tenantID, sid, redispkg.EventTokenUsage, map[string]any{
					"session_id":    sid,
					"input_tokens":  msg.TokenUsage.InputTokens,
					"output_tokens": msg.TokenUsage.OutputTokens,
					"total_tokens":  msg.TokenUsage.InputTokens + msg.TokenUsage.OutputTokens,
				})
			}
		}
	}
}

// handleToolCall routes intercepted tool calls.
func (o *Orchestrator) handleToolCall(
	ctx context.Context,
	tenantID domain.TenantID,
	sessionID domain.AgentSessionID,
	projectID domain.ProjectID,
	msg agentpkg.Message,
) {
	tc := msg.ToolCall
	sid := sessionID.String()

	switch tc.Name {
	case "ask_human":
		if o.deps.HITL == nil {
			o.logger.WarnContext(ctx, "ask_human tool call intercepted without HITL handler",
				"session_id", sessionID,
				"tool_call_id", tc.ID,
			)
			o.publishEvent(ctx, tenantID, sid, redispkg.EventAgentOutput, map[string]any{
				"session_id":   sid,
				"type":         string(agentpkg.MessageToolCall),
				"tool_call_id": tc.ID,
				"tool_name":    tc.Name,
				"tool_input":   string(tc.Input),
				"intercepted":  true,
				"pending_hitl": false,
			})
			return
		}

		question, err := o.deps.HITL.HandleAskHuman(ctx, tenantID, sessionID, tc.Input)
		if err != nil {
			o.logger.ErrorContext(ctx, "ask_human routing failed",
				"error", err,
				"session_id", sessionID,
				"tool_call_id", tc.ID,
			)
			o.publishEvent(ctx, tenantID, sid, redispkg.EventAgentOutput, map[string]any{
				"session_id":   sid,
				"type":         string(agentpkg.MessageToolCall),
				"tool_call_id": tc.ID,
				"tool_name":    tc.Name,
				"tool_input":   string(tc.Input),
				"intercepted":  true,
				"pending_hitl": false,
				"error":        err.Error(),
			})
			return
		}

		o.publishEvent(ctx, tenantID, sid, redispkg.EventAgentOutput, map[string]any{
			"session_id":   sid,
			"type":         string(agentpkg.MessageToolCall),
			"tool_call_id": tc.ID,
			"tool_name":    tc.Name,
			"tool_input":   string(tc.Input),
			"intercepted":  true,
			"pending_hitl": true,
			"question_id":  question.ID.String(),
		})

	case "create_adr":
		if o.deps.ADREngine == nil {
			o.logger.WarnContext(ctx, "create_adr intercepted without ADR engine",
				"session_id", sessionID,
				"tool_call_id", tc.ID,
			)
			o.publishEvent(ctx, tenantID, sid, redispkg.EventAgentOutput, map[string]any{
				"session_id":   sid,
				"type":         string(agentpkg.MessageToolCall),
				"tool_call_id": tc.ID,
				"tool_name":    tc.Name,
				"tool_input":   string(tc.Input),
				"intercepted":  true,
			})
			return
		}
		record, adrErr := o.deps.ADREngine.HandleCreateADR(ctx, tenantID, projectID, sessionID, tc.Input)
		if adrErr != nil {
			o.logger.ErrorContext(ctx, "create_adr failed",
				"error", adrErr,
				"session_id", sessionID,
				"tool_call_id", tc.ID,
			)
			o.publishEvent(ctx, tenantID, sid, redispkg.EventAgentOutput, map[string]any{
				"session_id":   sid,
				"type":         string(agentpkg.MessageToolCall),
				"tool_call_id": tc.ID,
				"tool_name":    tc.Name,
				"tool_input":   string(tc.Input),
				"intercepted":  true,
				"error":        adrErr.Error(),
			})
			return
		}
		if o.deps.Notifier != nil {
			if notifyErr := o.deps.Notifier.NotifyADRCreated(ctx, tenantID, sessionID, record); notifyErr != nil {
				o.logger.WarnContext(ctx, "create_adr notification failed",
					"error", notifyErr,
					"session_id", sessionID,
					"tool_call_id", tc.ID,
				)
			}
		}
		o.publishEvent(ctx, tenantID, sid, redispkg.EventAgentOutput, map[string]any{
			"session_id":   sid,
			"type":         string(agentpkg.MessageToolCall),
			"tool_call_id": tc.ID,
			"tool_name":    tc.Name,
			"intercepted":  true,
			"adr_id":       record.ID.String(),
			"adr_sequence": record.Sequence,
		})

	default:
		// Forward unhandled tool calls as output events.
		o.publishEvent(ctx, tenantID, sid, redispkg.EventAgentOutput, map[string]any{
			"session_id":   sid,
			"type":         string(agentpkg.MessageToolCall),
			"tool_call_id": tc.ID,
			"tool_name":    tc.Name,
			"tool_input":   string(tc.Input),
		})
	}
}

// activeContainerIDs returns a set of container IDs currently tracked by the orchestrator.
func (o *Orchestrator) activeContainerIDs() map[string]bool {
	o.mu.Lock()
	defer o.mu.Unlock()

	ids := make(map[string]bool, len(o.sessions))
	for _, rs := range o.sessions {
		if rs.containerID != "" {
			ids[rs.containerID] = true
		}
	}
	return ids
}

// StartReconciler starts the background orphan container reconciliation loop.
func (o *Orchestrator) StartReconciler(ctx context.Context) {
	if o == nil || o.reconciler == nil {
		return
	}
	o.reconciler.Start(ctx)
}

// btoi converts a bool to an int (0 or 1).
func btoi(b bool) int {
	if b {
		return 1
	}
	return 0
}
