package agent

import (
	"context"
	"time"

	"github.com/gosuda/steerlane/internal/domain"
)

// Repository defines persistence operations for agent sessions.
type Repository interface {
	Create(ctx context.Context, session *Session) error
	GetByID(ctx context.Context, tenantID domain.TenantID, id domain.AgentSessionID) (*Session, error)
	UpdateStatus(ctx context.Context, tenantID domain.TenantID, id domain.AgentSessionID, status SessionStatus) error
	ScheduleRetry(ctx context.Context, tenantID domain.TenantID, id domain.AgentSessionID, retryCount int, retryAt *time.Time) error
	ListByProject(ctx context.Context, tenantID domain.TenantID, projectID domain.ProjectID) ([]*Session, error)
	ListByTask(ctx context.Context, tenantID domain.TenantID, taskID domain.TaskID) ([]*Session, error)
	ListRetryReady(ctx context.Context, before time.Time, limit int) ([]*Session, error)
}
