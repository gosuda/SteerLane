// Package testutil provides shared mock implementations and test fixtures
// for SteerLane unit and integration tests.
package testutil

import (
	"context"
	"time"

	"github.com/google/uuid"

	"github.com/gosuda/steerlane/internal/auth"
	"github.com/gosuda/steerlane/internal/domain"
	"github.com/gosuda/steerlane/internal/domain/adr"
	"github.com/gosuda/steerlane/internal/domain/agent"
	"github.com/gosuda/steerlane/internal/domain/hitl"
	"github.com/gosuda/steerlane/internal/domain/project"
	"github.com/gosuda/steerlane/internal/domain/task"
	"github.com/gosuda/steerlane/internal/domain/tenant"
	"github.com/gosuda/steerlane/internal/domain/user"
	"github.com/gosuda/steerlane/internal/server/middleware"
)

// Compile-time interface satisfaction checks.
var (
	_ user.Repository          = (*MockUserRepo)(nil)
	_ tenant.Repository        = (*MockTenantRepo)(nil)
	_ auth.APIKeyRepository    = (*MockAPIKeyRepo)(nil)
	_ project.Repository       = (*MockProjectRepo)(nil)
	_ agent.Repository         = (*MockAgentRepo)(nil)
	_ agent.EventRepository    = (*MockAgentEventRepo)(nil)
	_ task.Repository          = (*MockTaskRepo)(nil)
	_ hitl.Repository          = (*MockHITLRepo)(nil)
	_ adr.Repository           = (*MockADRRepo)(nil)
	_ middleware.Authenticator = (*MockAuthenticator)(nil)
)

// ---------------------------------------------------------------------------
// MockUserRepo
// ---------------------------------------------------------------------------

// MockUserRepo is a configurable mock of user.Repository.
// Each exported function field, when non-nil, is called by the corresponding
// method. When nil, the method returns zero values.
type MockUserRepo struct {
	CreateFn       func(ctx context.Context, u *user.User) error
	GetByIDFn      func(ctx context.Context, tenantID domain.TenantID, id domain.UserID) (*user.User, error)
	GetByEmailFn   func(ctx context.Context, tenantID domain.TenantID, email string) (*user.User, error)
	ListByTenantFn func(ctx context.Context, tenantID domain.TenantID, limit int, cursor string) ([]*user.User, error)
	UpdateFn       func(ctx context.Context, u *user.User) error
	DeleteFn       func(ctx context.Context, tenantID domain.TenantID, id domain.UserID) error
}

func (m *MockUserRepo) Create(ctx context.Context, u *user.User) error {
	if m.CreateFn != nil {
		return m.CreateFn(ctx, u)
	}
	return nil
}

func (m *MockUserRepo) GetByID(ctx context.Context, tenantID domain.TenantID, id domain.UserID) (*user.User, error) {
	if m.GetByIDFn != nil {
		return m.GetByIDFn(ctx, tenantID, id)
	}
	return nil, nil
}

func (m *MockUserRepo) GetByEmail(ctx context.Context, tenantID domain.TenantID, email string) (*user.User, error) {
	if m.GetByEmailFn != nil {
		return m.GetByEmailFn(ctx, tenantID, email)
	}
	return nil, nil
}

func (m *MockUserRepo) ListByTenant(ctx context.Context, tenantID domain.TenantID, limit int, cursor string) ([]*user.User, error) {
	if m.ListByTenantFn != nil {
		return m.ListByTenantFn(ctx, tenantID, limit, cursor)
	}
	return nil, nil
}

func (m *MockUserRepo) Update(ctx context.Context, u *user.User) error {
	if m.UpdateFn != nil {
		return m.UpdateFn(ctx, u)
	}
	return nil
}

func (m *MockUserRepo) Delete(ctx context.Context, tenantID domain.TenantID, id domain.UserID) error {
	if m.DeleteFn != nil {
		return m.DeleteFn(ctx, tenantID, id)
	}
	return nil
}

// ---------------------------------------------------------------------------
// MockTenantRepo
// ---------------------------------------------------------------------------

// MockTenantRepo is a configurable mock of tenant.Repository.
type MockTenantRepo struct {
	CreateFn    func(ctx context.Context, t *tenant.Tenant) error
	GetByIDFn   func(ctx context.Context, id domain.TenantID) (*tenant.Tenant, error)
	GetBySlugFn func(ctx context.Context, slug string) (*tenant.Tenant, error)
	UpdateFn    func(ctx context.Context, t *tenant.Tenant) error
}

func (m *MockTenantRepo) Create(ctx context.Context, t *tenant.Tenant) error {
	if m.CreateFn != nil {
		return m.CreateFn(ctx, t)
	}
	return nil
}

func (m *MockTenantRepo) GetByID(ctx context.Context, id domain.TenantID) (*tenant.Tenant, error) {
	if m.GetByIDFn != nil {
		return m.GetByIDFn(ctx, id)
	}
	return nil, nil
}

func (m *MockTenantRepo) GetBySlug(ctx context.Context, slug string) (*tenant.Tenant, error) {
	if m.GetBySlugFn != nil {
		return m.GetBySlugFn(ctx, slug)
	}
	return nil, nil
}

func (m *MockTenantRepo) Update(ctx context.Context, t *tenant.Tenant) error {
	if m.UpdateFn != nil {
		return m.UpdateFn(ctx, t)
	}
	return nil
}

// ---------------------------------------------------------------------------
// MockAPIKeyRepo
// ---------------------------------------------------------------------------

// MockAPIKeyRepo is a configurable mock of auth.APIKeyRepository.
type MockAPIKeyRepo struct {
	CreateFn      func(ctx context.Context, rec *auth.APIKeyRecord) error
	GetByPrefixFn func(ctx context.Context, prefix string) (*auth.APIKeyRecord, error)
	ListByUserFn  func(ctx context.Context, tenantID domain.TenantID, userID domain.UserID) ([]*auth.APIKeyRecord, error)
	DeleteFn      func(ctx context.Context, tenantID domain.TenantID, id uuid.UUID) error
}

func (m *MockAPIKeyRepo) Create(ctx context.Context, rec *auth.APIKeyRecord) error {
	if m.CreateFn != nil {
		return m.CreateFn(ctx, rec)
	}
	return nil
}

func (m *MockAPIKeyRepo) GetByPrefix(ctx context.Context, prefix string) (*auth.APIKeyRecord, error) {
	if m.GetByPrefixFn != nil {
		return m.GetByPrefixFn(ctx, prefix)
	}
	return nil, nil
}

func (m *MockAPIKeyRepo) ListByUser(ctx context.Context, tenantID domain.TenantID, userID domain.UserID) ([]*auth.APIKeyRecord, error) {
	if m.ListByUserFn != nil {
		return m.ListByUserFn(ctx, tenantID, userID)
	}
	return nil, nil
}

func (m *MockAPIKeyRepo) Delete(ctx context.Context, tenantID domain.TenantID, id uuid.UUID) error {
	if m.DeleteFn != nil {
		return m.DeleteFn(ctx, tenantID, id)
	}
	return nil
}

// ---------------------------------------------------------------------------
// MockProjectRepo
// ---------------------------------------------------------------------------

// MockProjectRepo is a configurable mock of project.Repository.
type MockProjectRepo struct {
	CreateFn       func(ctx context.Context, p *project.Project) error
	GetByIDFn      func(ctx context.Context, tenantID domain.TenantID, id domain.ProjectID) (*project.Project, error)
	UpdateFn       func(ctx context.Context, p *project.Project) error
	DeleteFn       func(ctx context.Context, tenantID domain.TenantID, id domain.ProjectID) error
	ListByTenantFn func(ctx context.Context, tenantID domain.TenantID, limit int, cursor *uuid.UUID) ([]*project.Project, error)
}

func (m *MockProjectRepo) Create(ctx context.Context, p *project.Project) error {
	if m.CreateFn != nil {
		return m.CreateFn(ctx, p)
	}
	return nil
}

func (m *MockProjectRepo) GetByID(ctx context.Context, tenantID domain.TenantID, id domain.ProjectID) (*project.Project, error) {
	if m.GetByIDFn != nil {
		return m.GetByIDFn(ctx, tenantID, id)
	}
	return nil, nil
}

func (m *MockProjectRepo) Update(ctx context.Context, p *project.Project) error {
	if m.UpdateFn != nil {
		return m.UpdateFn(ctx, p)
	}
	return nil
}

func (m *MockProjectRepo) Delete(ctx context.Context, tenantID domain.TenantID, id domain.ProjectID) error {
	if m.DeleteFn != nil {
		return m.DeleteFn(ctx, tenantID, id)
	}
	return nil
}

func (m *MockProjectRepo) ListByTenant(ctx context.Context, tenantID domain.TenantID, limit int, cursor *uuid.UUID) ([]*project.Project, error) {
	if m.ListByTenantFn != nil {
		return m.ListByTenantFn(ctx, tenantID, limit, cursor)
	}
	return nil, nil
}

// ---------------------------------------------------------------------------
// MockAgentRepo
// ---------------------------------------------------------------------------

// MockAgentRepo is a configurable mock of agent.Repository.
type MockAgentRepo struct {
	CreateFn         func(ctx context.Context, session *agent.Session) error
	GetByIDFn        func(ctx context.Context, tenantID domain.TenantID, id domain.AgentSessionID) (*agent.Session, error)
	UpdateStatusFn   func(ctx context.Context, tenantID domain.TenantID, id domain.AgentSessionID, status agent.SessionStatus) error
	ScheduleRetryFn  func(ctx context.Context, tenantID domain.TenantID, id domain.AgentSessionID, retryCount int, retryAt *time.Time) error
	ListByProjectFn  func(ctx context.Context, tenantID domain.TenantID, projectID domain.ProjectID) ([]*agent.Session, error)
	ListByTaskFn     func(ctx context.Context, tenantID domain.TenantID, taskID domain.TaskID) ([]*agent.Session, error)
	ListRetryReadyFn func(ctx context.Context, before time.Time, limit int) ([]*agent.Session, error)
}

// MockAgentEventRepo is a configurable mock of agent.EventRepository.
type MockAgentEventRepo struct {
	AppendFn        func(ctx context.Context, event *agent.Event) error
	ListBySessionFn func(ctx context.Context, tenantID domain.TenantID, sessionID domain.AgentSessionID, limit int, cursor *uuid.UUID) ([]*agent.Event, error)
}

func (m *MockAgentEventRepo) Append(ctx context.Context, event *agent.Event) error {
	if m.AppendFn != nil {
		return m.AppendFn(ctx, event)
	}
	return nil
}

func (m *MockAgentEventRepo) ListBySession(ctx context.Context, tenantID domain.TenantID, sessionID domain.AgentSessionID, limit int, cursor *uuid.UUID) ([]*agent.Event, error) {
	if m.ListBySessionFn != nil {
		return m.ListBySessionFn(ctx, tenantID, sessionID, limit, cursor)
	}
	return nil, nil
}

func (m *MockAgentRepo) Create(ctx context.Context, session *agent.Session) error {
	if m.CreateFn != nil {
		return m.CreateFn(ctx, session)
	}
	return nil
}

func (m *MockAgentRepo) GetByID(ctx context.Context, tenantID domain.TenantID, id domain.AgentSessionID) (*agent.Session, error) {
	if m.GetByIDFn != nil {
		return m.GetByIDFn(ctx, tenantID, id)
	}
	return nil, nil
}

func (m *MockAgentRepo) UpdateStatus(ctx context.Context, tenantID domain.TenantID, id domain.AgentSessionID, status agent.SessionStatus) error {
	if m.UpdateStatusFn != nil {
		return m.UpdateStatusFn(ctx, tenantID, id, status)
	}
	return nil
}

func (m *MockAgentRepo) ScheduleRetry(ctx context.Context, tenantID domain.TenantID, id domain.AgentSessionID, retryCount int, retryAt *time.Time) error {
	if m.ScheduleRetryFn != nil {
		return m.ScheduleRetryFn(ctx, tenantID, id, retryCount, retryAt)
	}
	return nil
}

func (m *MockAgentRepo) ListByProject(ctx context.Context, tenantID domain.TenantID, projectID domain.ProjectID) ([]*agent.Session, error) {
	if m.ListByProjectFn != nil {
		return m.ListByProjectFn(ctx, tenantID, projectID)
	}
	return nil, nil
}

func (m *MockAgentRepo) ListByTask(ctx context.Context, tenantID domain.TenantID, taskID domain.TaskID) ([]*agent.Session, error) {
	if m.ListByTaskFn != nil {
		return m.ListByTaskFn(ctx, tenantID, taskID)
	}
	return nil, nil
}

func (m *MockAgentRepo) ListRetryReady(ctx context.Context, before time.Time, limit int) ([]*agent.Session, error) {
	if m.ListRetryReadyFn != nil {
		return m.ListRetryReadyFn(ctx, before, limit)
	}
	return nil, nil
}

// ---------------------------------------------------------------------------
// MockTaskRepo
// ---------------------------------------------------------------------------

// MockTaskRepo is a configurable mock of task.Repository.
type MockTaskRepo struct {
	CreateFn        func(ctx context.Context, tk *task.Task) error
	GetByIDFn       func(ctx context.Context, tenantID domain.TenantID, id domain.TaskID) (*task.Task, error)
	UpdateFn        func(ctx context.Context, tk *task.Task) error
	DeleteFn        func(ctx context.Context, tenantID domain.TenantID, id domain.TaskID) error
	ListByProjectFn func(ctx context.Context, tenantID domain.TenantID, projectID domain.ProjectID, filter task.Filter, limit int, cursor *uuid.UUID) ([]*task.Task, error)
	TransitionFn    func(ctx context.Context, tenantID domain.TenantID, id domain.TaskID, next task.TaskStatus) error
}

func (m *MockTaskRepo) Create(ctx context.Context, tk *task.Task) error {
	if m.CreateFn != nil {
		return m.CreateFn(ctx, tk)
	}
	return nil
}

func (m *MockTaskRepo) GetByID(ctx context.Context, tenantID domain.TenantID, id domain.TaskID) (*task.Task, error) {
	if m.GetByIDFn != nil {
		return m.GetByIDFn(ctx, tenantID, id)
	}
	return nil, nil
}

func (m *MockTaskRepo) Update(ctx context.Context, tk *task.Task) error {
	if m.UpdateFn != nil {
		return m.UpdateFn(ctx, tk)
	}
	return nil
}

func (m *MockTaskRepo) Delete(ctx context.Context, tenantID domain.TenantID, id domain.TaskID) error {
	if m.DeleteFn != nil {
		return m.DeleteFn(ctx, tenantID, id)
	}
	return nil
}

func (m *MockTaskRepo) ListByProject(ctx context.Context, tenantID domain.TenantID, projectID domain.ProjectID, filter task.Filter, limit int, cursor *uuid.UUID) ([]*task.Task, error) {
	if m.ListByProjectFn != nil {
		return m.ListByProjectFn(ctx, tenantID, projectID, filter, limit, cursor)
	}
	return nil, nil
}

func (m *MockTaskRepo) Transition(ctx context.Context, tenantID domain.TenantID, id domain.TaskID, next task.TaskStatus) error {
	if m.TransitionFn != nil {
		return m.TransitionFn(ctx, tenantID, id, next)
	}
	return nil
}

// ---------------------------------------------------------------------------
// MockHITLRepo
// ---------------------------------------------------------------------------

// MockHITLRepo is a configurable mock of hitl.Repository.
type MockHITLRepo struct {
	CreateFn                 func(ctx context.Context, question *hitl.Question) error
	DeleteFn                 func(ctx context.Context, tenantID domain.TenantID, id domain.HITLQuestionID) error
	CancelPendingBySessionFn func(ctx context.Context, tenantID domain.TenantID, sessionID domain.AgentSessionID) error
	ClearTimeoutClaimFn      func(ctx context.Context, tenantID domain.TenantID, id domain.HITLQuestionID) error
	ClaimTimeoutFn           func(ctx context.Context, tenantID domain.TenantID, id domain.HITLQuestionID) (*hitl.Question, error)
	MarkTimeoutSentFn        func(ctx context.Context, tenantID domain.TenantID, id domain.HITLQuestionID) error
	GetByIDFn                func(ctx context.Context, tenantID domain.TenantID, id domain.HITLQuestionID) (*hitl.Question, error)
	GetByThreadFn            func(ctx context.Context, tenantID domain.TenantID, platform, threadID string) (*hitl.Question, error)
	UpdateMessengerThreadFn  func(ctx context.Context, tenantID domain.TenantID, id domain.HITLQuestionID, platform, threadID string) error
	AnswerFn                 func(ctx context.Context, tenantID domain.TenantID, id domain.HITLQuestionID, answer string, answeredBy domain.UserID) error
	ResetAnswerFn            func(ctx context.Context, tenantID domain.TenantID, id domain.HITLQuestionID) error
	ReopenTimedOutFn         func(ctx context.Context, tenantID domain.TenantID, id domain.HITLQuestionID) error
	ListBySessionFn          func(ctx context.Context, tenantID domain.TenantID, sessionID domain.AgentSessionID) ([]*hitl.Question, error)
	EscalateFn               func(ctx context.Context, tenantID domain.TenantID, id domain.HITLQuestionID, newTimeoutAt time.Time) error
	ListExpiredPendingFn     func(ctx context.Context, before time.Time, limit int) ([]*hitl.Question, error)
	ListEscalatedExpiredFn   func(ctx context.Context, before time.Time, limit int) ([]*hitl.Question, error)
	MarkTimedOutEscalatedFn  func(ctx context.Context, tenantID domain.TenantID, id domain.HITLQuestionID) error
	ListTimedOutFn           func(ctx context.Context, limit int) ([]*hitl.Question, error)
	ListUnnotifiedTimedOutFn func(ctx context.Context, limit int) ([]*hitl.Question, error)
	MarkTimedOutQuestionFn   func(ctx context.Context, tenantID domain.TenantID, id domain.HITLQuestionID) error
	MarkTimedOutFn           func(ctx context.Context, before time.Time, limit int) ([]*hitl.Question, error)
}

func (m *MockHITLRepo) Create(ctx context.Context, question *hitl.Question) error {
	if m.CreateFn != nil {
		return m.CreateFn(ctx, question)
	}
	return nil
}

func (m *MockHITLRepo) Delete(ctx context.Context, tenantID domain.TenantID, id domain.HITLQuestionID) error {
	if m.DeleteFn != nil {
		return m.DeleteFn(ctx, tenantID, id)
	}
	return nil
}

func (m *MockHITLRepo) CancelPendingBySession(ctx context.Context, tenantID domain.TenantID, sessionID domain.AgentSessionID) error {
	if m.CancelPendingBySessionFn != nil {
		return m.CancelPendingBySessionFn(ctx, tenantID, sessionID)
	}
	return nil
}

func (m *MockHITLRepo) ClearTimeoutNotificationClaim(ctx context.Context, tenantID domain.TenantID, id domain.HITLQuestionID) error {
	if m.ClearTimeoutClaimFn != nil {
		return m.ClearTimeoutClaimFn(ctx, tenantID, id)
	}
	return nil
}

func (m *MockHITLRepo) ClaimTimeoutNotification(ctx context.Context, tenantID domain.TenantID, id domain.HITLQuestionID) (*hitl.Question, error) {
	if m.ClaimTimeoutFn != nil {
		return m.ClaimTimeoutFn(ctx, tenantID, id)
	}
	return nil, nil
}

func (m *MockHITLRepo) MarkTimeoutNotificationSent(ctx context.Context, tenantID domain.TenantID, id domain.HITLQuestionID) error {
	if m.MarkTimeoutSentFn != nil {
		return m.MarkTimeoutSentFn(ctx, tenantID, id)
	}
	return nil
}

func (m *MockHITLRepo) GetByID(ctx context.Context, tenantID domain.TenantID, id domain.HITLQuestionID) (*hitl.Question, error) {
	if m.GetByIDFn != nil {
		return m.GetByIDFn(ctx, tenantID, id)
	}
	return nil, nil
}

func (m *MockHITLRepo) GetPendingByThread(ctx context.Context, tenantID domain.TenantID, platform, threadID string) (*hitl.Question, error) {
	if m.GetByThreadFn != nil {
		return m.GetByThreadFn(ctx, tenantID, platform, threadID)
	}
	return nil, nil
}

func (m *MockHITLRepo) UpdateMessengerThread(ctx context.Context, tenantID domain.TenantID, id domain.HITLQuestionID, platform, threadID string) error {
	if m.UpdateMessengerThreadFn != nil {
		return m.UpdateMessengerThreadFn(ctx, tenantID, id, platform, threadID)
	}
	return nil
}

func (m *MockHITLRepo) Answer(ctx context.Context, tenantID domain.TenantID, id domain.HITLQuestionID, answer string, answeredBy domain.UserID) error {
	if m.AnswerFn != nil {
		return m.AnswerFn(ctx, tenantID, id, answer, answeredBy)
	}
	return nil
}

func (m *MockHITLRepo) ResetAnswer(ctx context.Context, tenantID domain.TenantID, id domain.HITLQuestionID) error {
	if m.ResetAnswerFn != nil {
		return m.ResetAnswerFn(ctx, tenantID, id)
	}
	return nil
}

func (m *MockHITLRepo) ReopenTimedOut(ctx context.Context, tenantID domain.TenantID, id domain.HITLQuestionID) error {
	if m.ReopenTimedOutFn != nil {
		return m.ReopenTimedOutFn(ctx, tenantID, id)
	}
	return nil
}

func (m *MockHITLRepo) ListBySession(ctx context.Context, tenantID domain.TenantID, sessionID domain.AgentSessionID) ([]*hitl.Question, error) {
	if m.ListBySessionFn != nil {
		return m.ListBySessionFn(ctx, tenantID, sessionID)
	}
	return nil, nil
}

func (m *MockHITLRepo) ListExpiredPendingBefore(ctx context.Context, before time.Time, limit int) ([]*hitl.Question, error) {
	if m.ListExpiredPendingFn != nil {
		return m.ListExpiredPendingFn(ctx, before, limit)
	}
	return nil, nil
}

func (m *MockHITLRepo) ListTimedOut(ctx context.Context, limit int) ([]*hitl.Question, error) {
	if m.ListTimedOutFn != nil {
		return m.ListTimedOutFn(ctx, limit)
	}
	return nil, nil
}

func (m *MockHITLRepo) ListUnnotifiedTimedOut(ctx context.Context, limit int) ([]*hitl.Question, error) {
	if m.ListUnnotifiedTimedOutFn != nil {
		return m.ListUnnotifiedTimedOutFn(ctx, limit)
	}
	return nil, nil
}

func (m *MockHITLRepo) MarkTimedOut(ctx context.Context, tenantID domain.TenantID, id domain.HITLQuestionID) error {
	if m.MarkTimedOutQuestionFn != nil {
		return m.MarkTimedOutQuestionFn(ctx, tenantID, id)
	}
	return nil
}

func (m *MockHITLRepo) MarkTimedOutBefore(ctx context.Context, before time.Time, limit int) ([]*hitl.Question, error) {
	if m.MarkTimedOutFn != nil {
		return m.MarkTimedOutFn(ctx, before, limit)
	}
	return nil, nil
}

func (m *MockHITLRepo) Escalate(ctx context.Context, tenantID domain.TenantID, id domain.HITLQuestionID, newTimeoutAt time.Time) error {
	if m.EscalateFn != nil {
		return m.EscalateFn(ctx, tenantID, id, newTimeoutAt)
	}
	return nil
}

func (m *MockHITLRepo) ListEscalatedExpiredBefore(ctx context.Context, before time.Time, limit int) ([]*hitl.Question, error) {
	if m.ListEscalatedExpiredFn != nil {
		return m.ListEscalatedExpiredFn(ctx, before, limit)
	}
	return nil, nil
}

func (m *MockHITLRepo) MarkTimedOutEscalated(ctx context.Context, tenantID domain.TenantID, id domain.HITLQuestionID) error {
	if m.MarkTimedOutEscalatedFn != nil {
		return m.MarkTimedOutEscalatedFn(ctx, tenantID, id)
	}
	return nil
}

// ---------------------------------------------------------------------------
// MockADRRepo
// ---------------------------------------------------------------------------

// MockADRRepo is a configurable mock of adr.Repository.
type MockADRRepo struct {
	CreateWithNextSequenceFn func(ctx context.Context, record *adr.ADR) error
	GetByIDFn                func(ctx context.Context, tenantID domain.TenantID, id domain.ADRID) (*adr.ADR, error)
	ListByProjectFn          func(ctx context.Context, tenantID domain.TenantID, projectID domain.ProjectID, limit int, cursor *uuid.UUID) ([]*adr.ADR, error)
	UpdateStatusFn           func(ctx context.Context, tenantID domain.TenantID, id domain.ADRID, status adr.ADRStatus) error
}

func (m *MockADRRepo) CreateWithNextSequence(ctx context.Context, record *adr.ADR) error {
	if m.CreateWithNextSequenceFn != nil {
		return m.CreateWithNextSequenceFn(ctx, record)
	}
	return nil
}

func (m *MockADRRepo) GetByID(ctx context.Context, tenantID domain.TenantID, id domain.ADRID) (*adr.ADR, error) {
	if m.GetByIDFn != nil {
		return m.GetByIDFn(ctx, tenantID, id)
	}
	return nil, nil
}

func (m *MockADRRepo) ListByProject(ctx context.Context, tenantID domain.TenantID, projectID domain.ProjectID, limit int, cursor *uuid.UUID) ([]*adr.ADR, error) {
	if m.ListByProjectFn != nil {
		return m.ListByProjectFn(ctx, tenantID, projectID, limit, cursor)
	}
	return nil, nil
}

func (m *MockADRRepo) UpdateStatus(ctx context.Context, tenantID domain.TenantID, id domain.ADRID, status adr.ADRStatus) error {
	if m.UpdateStatusFn != nil {
		return m.UpdateStatusFn(ctx, tenantID, id, status)
	}
	return nil
}

// ---------------------------------------------------------------------------
// MockAuthenticator
// ---------------------------------------------------------------------------

// MockAuthenticator is a configurable mock of middleware.Authenticator.
type MockAuthenticator struct {
	AuthenticateJWTFn    func(token string) (*middleware.Identity, error)
	AuthenticateAPIKeyFn func(ctx context.Context, rawKey string) (*middleware.Identity, error)
}

func (m *MockAuthenticator) AuthenticateJWT(token string) (*middleware.Identity, error) {
	if m.AuthenticateJWTFn != nil {
		return m.AuthenticateJWTFn(token)
	}
	return nil, nil
}

func (m *MockAuthenticator) AuthenticateAPIKey(ctx context.Context, rawKey string) (*middleware.Identity, error) {
	if m.AuthenticateAPIKeyFn != nil {
		return m.AuthenticateAPIKeyFn(ctx, rawKey)
	}
	return nil, nil
}
