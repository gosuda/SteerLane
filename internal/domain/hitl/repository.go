package hitl

import (
	"context"
	"time"

	"github.com/gosuda/steerlane/internal/domain"
)

// Repository defines persistence operations for HITL questions.
type Repository interface {
	Create(ctx context.Context, question *Question) error
	Delete(ctx context.Context, tenantID domain.TenantID, id domain.HITLQuestionID) error
	CancelPendingBySession(ctx context.Context, tenantID domain.TenantID, sessionID domain.AgentSessionID) error
	ClearTimeoutNotificationClaim(ctx context.Context, tenantID domain.TenantID, id domain.HITLQuestionID) error
	ClaimTimeoutNotification(ctx context.Context, tenantID domain.TenantID, id domain.HITLQuestionID) (*Question, error)
	MarkTimeoutNotificationSent(ctx context.Context, tenantID domain.TenantID, id domain.HITLQuestionID) error
	GetByID(ctx context.Context, tenantID domain.TenantID, id domain.HITLQuestionID) (*Question, error)
	GetPendingByThread(ctx context.Context, tenantID domain.TenantID, platform, threadID string) (*Question, error)
	UpdateMessengerThread(ctx context.Context, tenantID domain.TenantID, id domain.HITLQuestionID, platform, threadID string) error
	// Answer records a human response to a pending question.
	Answer(ctx context.Context, tenantID domain.TenantID, id domain.HITLQuestionID, answer string, answeredBy domain.UserID) error
	ResetAnswer(ctx context.Context, tenantID domain.TenantID, id domain.HITLQuestionID) error
	ReopenTimedOut(ctx context.Context, tenantID domain.TenantID, id domain.HITLQuestionID) error
	ListBySession(ctx context.Context, tenantID domain.TenantID, sessionID domain.AgentSessionID) ([]*Question, error)
	Escalate(ctx context.Context, tenantID domain.TenantID, id domain.HITLQuestionID, newTimeoutAt time.Time) error
	ListExpiredPendingBefore(ctx context.Context, before time.Time, limit int) ([]*Question, error)
	ListEscalatedExpiredBefore(ctx context.Context, before time.Time, limit int) ([]*Question, error)
	MarkTimedOutEscalated(ctx context.Context, tenantID domain.TenantID, id domain.HITLQuestionID) error
	ListTimedOut(ctx context.Context, limit int) ([]*Question, error)
	ListUnnotifiedTimedOut(ctx context.Context, limit int) ([]*Question, error)
	MarkTimedOut(ctx context.Context, tenantID domain.TenantID, id domain.HITLQuestionID) error
	MarkTimedOutBefore(ctx context.Context, before time.Time, limit int) ([]*Question, error)
}
