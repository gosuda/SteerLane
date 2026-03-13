package main

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/gosuda/steerlane/internal/domain"
	agentdom "github.com/gosuda/steerlane/internal/domain/agent"
	"github.com/gosuda/steerlane/internal/domain/hitl"
	"github.com/gosuda/steerlane/internal/domain/task"
	"github.com/gosuda/steerlane/internal/messenger"
	"github.com/gosuda/steerlane/internal/notify"
	"github.com/gosuda/steerlane/internal/orchestrator"
	"github.com/gosuda/steerlane/internal/store/postgres/sqlc"
	redispkg "github.com/gosuda/steerlane/internal/store/redis"
	"github.com/gosuda/steerlane/internal/testutil"
)

type mainTestLinkQuery struct {
	links []sqlc.UserMessengerLink
}

type mainTestPublisher struct{}

func (mainTestPublisher) PublishAgentEvent(context.Context, string, redispkg.EventType, any) error {
	return nil
}

type mainTestMessenger struct {
	notificationFn func(context.Context, messenger.NotificationParams) error
	notifications  []messenger.NotificationParams
}

func (m *mainTestMessenger) SendMessage(context.Context, messenger.SendMessageParams) (messenger.MessageResult, error) {
	return messenger.MessageResult{}, nil
}

func (m *mainTestMessenger) CreateThread(context.Context, messenger.CreateThreadParams) (messenger.MessageResult, error) {
	return messenger.MessageResult{}, nil
}

func (m *mainTestMessenger) UpdateMessage(context.Context, messenger.UpdateMessageParams) error {
	return nil
}

func (m *mainTestMessenger) SendNotification(ctx context.Context, params messenger.NotificationParams) error {
	m.notifications = append(m.notifications, params)
	if m.notificationFn != nil {
		return m.notificationFn(ctx, params)
	}
	return nil
}

func (m *mainTestMessenger) Platform() messenger.Platform {
	return messenger.PlatformSlack
}

func (q *mainTestLinkQuery) ListMessengerLinksByUser(context.Context, sqlc.ListMessengerLinksByUserParams) ([]sqlc.UserMessengerLink, error) {
	return q.links, nil
}

func mainTestLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func TestSweepExpiredHITLQuestionsEscalatesTier1(t *testing.T) {
	t.Parallel()

	tenantID := testutil.TestTenantID()
	sessionID := testutil.TestSessionID()
	timedOutAt := time.Now().UTC().Add(-time.Minute)
	questionOne := &hitl.Question{ID: domain.NewID(), TenantID: tenantID, AgentSessionID: sessionID, Question: "First?", Status: hitl.StatusPending, TimeoutAt: &timedOutAt}
	questionTwo := &hitl.Question{ID: domain.NewID(), TenantID: tenantID, AgentSessionID: sessionID, Question: "Second?", Status: hitl.StatusPending, TimeoutAt: &timedOutAt}

	type escalation struct { //nolint:govet // test readability over field packing
		id         domain.HITLQuestionID
		newTimeout time.Time
	}
	var escalated []escalation
	dispatcher := notify.NewDispatcher(
		mainTestLogger(),
		messenger.PlatformSlack,
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		&testutil.MockHITLRepo{
			ListExpiredPendingFn: func(context.Context, time.Time, int) ([]*hitl.Question, error) {
				return []*hitl.Question{questionOne, questionTwo}, nil
			},
			EscalateFn: func(_ context.Context, _ domain.TenantID, id domain.HITLQuestionID, newTimeoutAt time.Time) error {
				escalated = append(escalated, escalation{id: id, newTimeout: newTimeoutAt})
				return nil
			},
		},
		nil,
	)

	now := time.Now().UTC()
	extendedTimeout := 30 * time.Minute
	sweepExpiredHITLQuestions(context.Background(), mainTestLogger(), dispatcher, now, extendedTimeout)

	require.Len(t, escalated, 2)
	assert.ElementsMatch(t, []domain.HITLQuestionID{questionOne.ID, questionTwo.ID}, []domain.HITLQuestionID{escalated[0].id, escalated[1].id})
	// Extended timeout should be approximately now + extendedTimeout.
	for _, esc := range escalated {
		assert.WithinDuration(t, now.Add(extendedTimeout), esc.newTimeout, time.Second)
	}
}

func TestSweepEscalatedHITLQuestionsCancelsSessionTier2(t *testing.T) {
	t.Parallel()

	tenantID := testutil.TestTenantID()
	sessionID := testutil.TestSessionID()
	timedOutAt := time.Now().UTC().Add(-time.Minute)
	questionOne := &hitl.Question{ID: domain.NewID(), TenantID: tenantID, AgentSessionID: sessionID, Question: "First?", Status: hitl.StatusEscalated, TimeoutAt: &timedOutAt}
	questionTwo := &hitl.Question{ID: domain.NewID(), TenantID: tenantID, AgentSessionID: sessionID, Question: "Second?", Status: hitl.StatusEscalated, TimeoutAt: &timedOutAt}

	var markedEscalated []domain.HITLQuestionID
	var cancelledSessions []domain.AgentSessionID
	dispatcher := notify.NewDispatcher(
		mainTestLogger(),
		messenger.PlatformSlack,
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		&testutil.MockHITLRepo{
			ListEscalatedExpiredFn: func(context.Context, time.Time, int) ([]*hitl.Question, error) {
				return []*hitl.Question{questionOne, questionTwo}, nil
			},
			MarkTimedOutEscalatedFn: func(_ context.Context, _ domain.TenantID, id domain.HITLQuestionID) error {
				markedEscalated = append(markedEscalated, id)
				return nil
			},
			ClaimTimeoutFn: func(_ context.Context, _ domain.TenantID, id domain.HITLQuestionID) (*hitl.Question, error) {
				if id == questionOne.ID {
					return questionOne, nil
				}
				return questionTwo, nil
			},
		},
		nil,
	)
	orch := orchestrator.New(orchestrator.Deps{
		Logger: mainTestLogger(),
		PubSub: mainTestPublisher{},
		Sessions: &testutil.MockAgentRepo{
			GetByIDFn: func(context.Context, domain.TenantID, domain.AgentSessionID) (*agentdom.Session, error) {
				return &agentdom.Session{ID: sessionID, TaskID: testutil.TestTaskID(), Status: agentdom.StatusWaitingHITL}, nil
			},
			UpdateStatusFn: func(context.Context, domain.TenantID, domain.AgentSessionID, agentdom.SessionStatus) error {
				cancelledSessions = append(cancelledSessions, sessionID)
				return nil
			},
		},
		Tasks: &testutil.MockTaskRepo{TransitionFn: func(context.Context, domain.TenantID, domain.TaskID, task.TaskStatus) error {
			return nil
		}},
		Questions: &testutil.MockHITLRepo{CancelPendingBySessionFn: func(context.Context, domain.TenantID, domain.AgentSessionID) error {
			return nil
		}},
	})

	sweepEscalatedHITLQuestions(context.Background(), mainTestLogger(), dispatcher, orch, time.Now().UTC())

	assert.ElementsMatch(t, []domain.HITLQuestionID{questionOne.ID, questionTwo.ID}, markedEscalated)
	assert.Equal(t, []domain.AgentSessionID{sessionID}, cancelledSessions)
}

func TestRetryTimedOutHITLNotificationsReleasesClaimOnFailure(t *testing.T) {
	t.Parallel()

	tenantID := testutil.TestTenantID()
	question := &hitl.Question{ID: domain.NewID(), TenantID: tenantID, AgentSessionID: testutil.TestSessionID(), Question: "Need approval?"}
	assignee := testutil.TestUserID()
	var cleared []domain.HITLQuestionID

	messengerMock := &mainTestMessenger{notificationFn: func(context.Context, messenger.NotificationParams) error {
		return errors.New("slack down")
	}}
	dispatcher := notify.NewDispatcher(
		mainTestLogger(),
		messenger.PlatformSlack,
		notify.New(mainTestLogger(), messengerMock),
		messengerMock,
		nil,
		&mainTestLinkQuery{links: []sqlc.UserMessengerLink{{ExternalID: "U123", Platform: string(messenger.PlatformSlack)}}},
		&testutil.MockTaskRepo{GetByIDFn: func(context.Context, domain.TenantID, domain.TaskID) (*task.Task, error) {
			return &task.Task{ID: testutil.TestTaskID(), Title: "Fix auth", AssignedTo: &assignee}, nil
		}},
		&testutil.MockAgentRepo{GetByIDFn: func(context.Context, domain.TenantID, domain.AgentSessionID) (*agentdom.Session, error) {
			return &agentdom.Session{ID: question.AgentSessionID, TaskID: testutil.TestTaskID()}, nil
		}},
		&testutil.MockHITLRepo{
			ListUnnotifiedTimedOutFn: func(context.Context, int) ([]*hitl.Question, error) {
				return []*hitl.Question{question}, nil
			},
			ClaimTimeoutFn: func(context.Context, domain.TenantID, domain.HITLQuestionID) (*hitl.Question, error) {
				return question, nil
			},
			ClearTimeoutClaimFn: func(_ context.Context, _ domain.TenantID, id domain.HITLQuestionID) error {
				cleared = append(cleared, id)
				return nil
			},
		},
		nil,
	)

	retryTimedOutHITLNotifications(context.Background(), mainTestLogger(), dispatcher)

	require.Equal(t, []domain.HITLQuestionID{question.ID}, cleared)
	require.Len(t, messengerMock.notifications, 1)
}
