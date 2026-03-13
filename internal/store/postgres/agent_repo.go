package postgres

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/gosuda/steerlane/internal/domain"
	"github.com/gosuda/steerlane/internal/domain/agent"
	"github.com/gosuda/steerlane/internal/store/postgres/sqlc"
)

var _ agent.Repository = (*agentRepository)(nil)

type agentRepository struct {
	store *Store
}

func mapAgentSessionModel(row sqlc.AgentSession) (*agent.Session, error) {
	metadata, err := unmarshalJSONObject(row.Metadata)
	if err != nil {
		return nil, fmt.Errorf("decode agent metadata: %w", err)
	}

	entity := &agent.Session{
		ID:          row.ID,
		TenantID:    row.TenantID,
		ProjectID:   row.ProjectID,
		TaskID:      row.TaskID,
		AgentType:   agent.AgentType(row.AgentType),
		Status:      agent.SessionStatus(row.Status),
		ContainerID: row.ContainerID,
		BranchName:  row.BranchName,
		StartedAt:   timePtrFromPG(row.StartedAt),
		CompletedAt: timePtrFromPG(row.CompletedAt),
		Error:       row.Error,
		Metadata:    metadata,
		RetryCount:  int(row.RetryCount),
		RetryAt:     timePtrFromPG(row.RetryAt),
		CreatedAt:   row.CreatedAt,
	}
	if err := entity.Validate(); err != nil { //nolint:govet // short-lived err shadow is idiomatic Go
		return nil, fmt.Errorf("map agent session: %w", err)
	}
	return entity, nil
}

func mapAgentSessions(rows []sqlc.AgentSession) ([]*agent.Session, error) {
	items := make([]*agent.Session, 0, len(rows))
	for _, row := range rows {
		mapped, err := mapAgentSessionModel(row)
		if err != nil {
			return nil, err
		}
		items = append(items, mapped)
	}
	return items, nil
}

func validateAgentStatus(status agent.SessionStatus) error {
	switch status {
	case agent.StatusPending, agent.StatusRunning, agent.StatusWaitingHITL, agent.StatusCompleted, agent.StatusFailed, agent.StatusCancelled:
		return nil
	default:
		return fmt.Errorf("agent session status %q: %w", status, domain.ErrInvalidInput)
	}
}

func (r *agentRepository) Create(ctx context.Context, record *agent.Session) error {
	if record == nil {
		return fmt.Errorf("agent session: %w", domain.ErrInvalidInput)
	}
	if err := requireUUID("agent tenant id", record.TenantID); err != nil {
		return err
	}
	if err := requireUUID("agent project id", record.ProjectID); err != nil {
		return err
	}
	if err := requireUUID("agent task id", record.TaskID); err != nil {
		return err
	}
	if err := record.Validate(); err != nil {
		return err
	}

	metadata, err := marshalJSONObject(record.Metadata)
	if err != nil {
		return fmt.Errorf("marshal agent metadata: %w", err)
	}

	row, err := r.store.queries.CreateAgentSession(ctx, sqlc.CreateAgentSessionParams{
		TenantID:   record.TenantID,
		ProjectID:  record.ProjectID,
		TaskID:     record.TaskID,
		AgentType:  string(record.AgentType),
		Status:     string(record.Status),
		BranchName: record.BranchName,
		Metadata:   metadata,
		RetryCount: int32(min(record.RetryCount, 100)), //nolint:gosec // bounded by maxRetryAttempts
		RetryAt:    pgTimestamptzFromPtr(record.RetryAt),
	})
	if err != nil {
		return fmt.Errorf("create agent session: %w", classifyError(err))
	}

	mapped, err := mapAgentSessionModel(row)
	if err != nil {
		return err
	}
	*record = *mapped

	return nil
}

func (r *agentRepository) ScheduleRetry(ctx context.Context, tenantID domain.TenantID, id domain.AgentSessionID, retryCount int, retryAt *time.Time) error {
	if err := requireUUID("tenant id", tenantID); err != nil {
		return err
	}
	if err := requireUUID("agent session id", id); err != nil {
		return err
	}
	if retryCount < 0 {
		return fmt.Errorf("retry count must be >= 0: %w", domain.ErrInvalidInput)
	}

	if err := r.store.queries.UpdateAgentSessionRetry(ctx, sqlc.UpdateAgentSessionRetryParams{
		ID:         id,
		RetryCount: int32(min(retryCount, 100)), //nolint:gosec // bounded by maxRetryAttempts
		RetryAt:    pgTimestamptzFromPtr(retryAt),
		TenantID:   tenantID,
	}); err != nil {
		return fmt.Errorf("schedule agent session retry: %w", classifyError(err))
	}
	return nil
}

func (r *agentRepository) GetByID(ctx context.Context, tenantID domain.TenantID, id domain.AgentSessionID) (*agent.Session, error) {
	if err := requireUUID("tenant id", tenantID); err != nil {
		return nil, err
	}
	if err := requireUUID("agent session id", id); err != nil {
		return nil, err
	}

	row, err := r.store.queries.GetAgentSessionByID(ctx, sqlc.GetAgentSessionByIDParams{ID: id, TenantID: tenantID})
	if err != nil {
		return nil, fmt.Errorf("get agent session by id: %w", classifyError(err))
	}

	entity, err := mapAgentSessionModel(row)
	if err != nil {
		return nil, err
	}
	return entity, nil
}

func (r *agentRepository) UpdateStatus(ctx context.Context, tenantID domain.TenantID, id domain.AgentSessionID, status agent.SessionStatus) error {
	if err := requireUUID("tenant id", tenantID); err != nil {
		return err
	}
	if err := requireUUID("agent session id", id); err != nil {
		return err
	}
	if err := validateAgentStatus(status); err != nil {
		return err
	}

	current, err := r.GetByID(ctx, tenantID, id)
	if err != nil {
		return err
	}

	updated := *current
	if err := updated.Transition(status, time.Now().UTC()); err != nil { //nolint:govet // short-lived err shadow is idiomatic Go
		return err
	}

	if _, err := r.store.queries.UpdateAgentSessionStatus(ctx, sqlc.UpdateAgentSessionStatusParams{ //nolint:govet // short-lived err shadow is idiomatic Go
		ID:          id,
		Status:      string(updated.Status),
		ContainerID: current.ContainerID,
		StartedAt:   pgTimestamptzFromPtr(updated.StartedAt),
		CompletedAt: pgTimestamptzFromPtr(updated.CompletedAt),
		Error:       current.Error,
		TenantID:    tenantID,
	}); err != nil {
		return fmt.Errorf("update agent session status: %w", classifyError(err))
	}
	return nil
}

func (r *agentRepository) ListByProject(ctx context.Context, tenantID domain.TenantID, projectID domain.ProjectID) ([]*agent.Session, error) {
	if err := requireUUID("tenant id", tenantID); err != nil {
		return nil, err
	}
	if err := requireUUID("project id", projectID); err != nil {
		return nil, err
	}

	rows, err := r.store.queries.ListAgentSessionsByProject(ctx, sqlc.ListAgentSessionsByProjectParams{
		ProjectID: projectID,
		TenantID:  tenantID,
	})
	if err != nil {
		return nil, fmt.Errorf("list agent sessions by project: %w", classifyError(err))
	}
	return mapAgentSessions(rows)
}

func (r *agentRepository) ListByTask(ctx context.Context, tenantID domain.TenantID, taskID domain.TaskID) ([]*agent.Session, error) {
	if err := requireUUID("tenant id", tenantID); err != nil {
		return nil, err
	}
	if err := requireUUID("task id", taskID); err != nil {
		return nil, err
	}

	rows, err := r.store.queries.ListAgentSessionsByTask(ctx, sqlc.ListAgentSessionsByTaskParams{TaskID: taskID, TenantID: tenantID})
	if err != nil {
		return nil, fmt.Errorf("list agent sessions by task: %w", classifyError(err))
	}
	return mapAgentSessions(rows)
}

func (r *agentRepository) ListRetryReady(ctx context.Context, before time.Time, limit int) ([]*agent.Session, error) {
	if limit <= 0 {
		return nil, fmt.Errorf("retry ready limit must be > 0: %w", domain.ErrInvalidInput)
	}

	rows, err := r.store.queries.ListAgentSessionsRetryReady(ctx, sqlc.ListAgentSessionsRetryReadyParams{
		RetryAt: pgtype.Timestamptz{Time: before, Valid: true},
		Limit:   int32(min(limit, maxListLimit)), //nolint:gosec // clamped to maxListLimit
	})
	if err != nil {
		return nil, fmt.Errorf("list retry-ready agent sessions: %w", classifyError(err))
	}
	return mapAgentSessions(rows)
}
