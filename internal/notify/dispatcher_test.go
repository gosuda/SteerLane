package notify

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/gosuda/steerlane/internal/domain"
	"github.com/gosuda/steerlane/internal/domain/adr"
	agentdom "github.com/gosuda/steerlane/internal/domain/agent"
	"github.com/gosuda/steerlane/internal/domain/hitl"
	"github.com/gosuda/steerlane/internal/domain/task"
	userdom "github.com/gosuda/steerlane/internal/domain/user"
	"github.com/gosuda/steerlane/internal/messenger"
	"github.com/gosuda/steerlane/internal/store/postgres/sqlc"
	"github.com/gosuda/steerlane/internal/testutil"
)

type fakeLinkQuery struct {
	links []sqlc.UserMessengerLink
}

type fakeUserLookup struct {
	user *userdom.User
}

func (f *fakeLinkQuery) ListMessengerLinksByUser(_ context.Context, _ sqlc.ListMessengerLinksByUserParams) ([]sqlc.UserMessengerLink, error) {
	return f.links, nil
}

func (f *fakeUserLookup) GetByID(context.Context, domain.TenantID, domain.UserID) (*userdom.User, error) {
	return f.user, nil
}

func testDispatcherLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func TestDispatcherNotifyTaskCompleted(t *testing.T) {
	t.Parallel()

	messengerMock := &mockMessenger{}
	dispatcher := NewDispatcher(
		testDispatcherLogger(),
		messenger.PlatformSlack,
		New(testDispatcherLogger(), messengerMock),
		messengerMock,
		nil,
		&fakeLinkQuery{links: []sqlc.UserMessengerLink{{ExternalID: "U123", Platform: string(messenger.PlatformSlack)}}},
		&testutil.MockTaskRepo{GetByIDFn: func(context.Context, domain.TenantID, domain.TaskID) (*task.Task, error) {
			assignee := testutil.TestUserID()
			return &task.Task{ID: testutil.TestTaskID(), Title: "Fix auth", AssignedTo: &assignee}, nil
		}},
		nil,
		nil,
		nil,
	)

	err := dispatcher.NotifyTaskCompleted(context.Background(), testutil.TestTenantID(), testutil.TestTaskID(), testutil.TestSessionID())
	require.NoError(t, err)
	require.Len(t, messengerMock.notifications, 1)
	require.Equal(t, "U123", messengerMock.notifications[0].UserExternalID)
	require.Contains(t, messengerMock.notifications[0].Text, "Task completed")
}

func TestDispatcherNotifySessionFailedSkipsWithoutAssignee(t *testing.T) {
	t.Parallel()

	messengerMock := &mockMessenger{}
	dispatcher := NewDispatcher(
		testDispatcherLogger(),
		messenger.PlatformSlack,
		New(testDispatcherLogger(), messengerMock),
		messengerMock,
		nil,
		&fakeLinkQuery{},
		&testutil.MockTaskRepo{GetByIDFn: func(context.Context, domain.TenantID, domain.TaskID) (*task.Task, error) {
			return &task.Task{ID: testutil.TestTaskID(), Title: "Fix auth"}, nil
		}},
		nil,
		nil,
		nil,
	)

	err := dispatcher.NotifySessionFailed(context.Background(), testutil.TestTenantID(), testutil.TestTaskID(), testutil.TestSessionID(), "boom")
	require.NoError(t, err)
	require.Empty(t, messengerMock.notifications)
}

func TestDispatcherTimedOutQuestionHelpers(t *testing.T) {
	t.Parallel()

	messengerMock := &mockMessenger{}
	assignee := testutil.TestUserID()
	email := "user@example.com"
	question := &hitl.Question{
		ID:             domain.NewID(),
		TenantID:       testutil.TestTenantID(),
		AgentSessionID: testutil.TestSessionID(),
		Question:       "Need approval?",
	}
	dispatcher := NewDispatcher(
		testDispatcherLogger(),
		messenger.PlatformSlack,
		New(testDispatcherLogger(), messengerMock),
		messengerMock,
		nil,
		&fakeLinkQuery{links: []sqlc.UserMessengerLink{{ExternalID: "U123", Platform: string(messenger.PlatformSlack)}}},
		&testutil.MockTaskRepo{GetByIDFn: func(context.Context, domain.TenantID, domain.TaskID) (*task.Task, error) {
			return &task.Task{ID: testutil.TestTaskID(), Title: "Fix auth", AssignedTo: &assignee}, nil
		}},
		&testutil.MockAgentRepo{GetByIDFn: func(context.Context, domain.TenantID, domain.AgentSessionID) (*agentdom.Session, error) {
			return &agentdom.Session{ID: testutil.TestSessionID(), TaskID: testutil.TestTaskID()}, nil
		}},
		&testutil.MockHITLRepo{
			ListExpiredPendingFn: func(context.Context, time.Time, int) ([]*hitl.Question, error) {
				return []*hitl.Question{question}, nil
			},
			MarkTimedOutQuestionFn: func(context.Context, domain.TenantID, domain.HITLQuestionID) error {
				return nil
			},
		},
		&fakeUserLookup{user: &userdom.User{Email: &email}},
	)

	questions, err := dispatcher.ListExpiredPendingQuestions(context.Background(), time.Now().UTC(), 10)
	require.NoError(t, err)
	require.Len(t, questions, 1)
	require.NoError(t, dispatcher.MarkQuestionTimedOut(context.Background(), question.TenantID, question.ID))
	require.NoError(t, dispatcher.NotifyQuestionTimedOut(context.Background(), question))
	require.Len(t, messengerMock.notifications, 1)
	require.Contains(t, messengerMock.notifications[0].Text, "Need approval?")
}

func TestDispatcherTimeoutNotificationClaimHelpers(t *testing.T) {
	t.Parallel()

	question := &hitl.Question{ID: domain.NewID(), TenantID: testutil.TestTenantID(), AgentSessionID: testutil.TestSessionID()}
	var cleared []domain.HITLQuestionID
	dispatcher := NewDispatcher(
		testDispatcherLogger(),
		messenger.PlatformSlack,
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
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

	questions, err := dispatcher.ListUnnotifiedTimedOutQuestions(context.Background(), 10)
	require.NoError(t, err)
	require.Len(t, questions, 1)

	claimed, err := dispatcher.ClaimTimeoutNotification(context.Background(), question.TenantID, question.ID)
	require.NoError(t, err)
	require.Equal(t, question, claimed)

	require.NoError(t, dispatcher.ClearTimeoutNotificationClaim(context.Background(), question.TenantID, question.ID))
	require.Equal(t, []domain.HITLQuestionID{question.ID}, cleared)
}

func TestDispatcherNotifyADRCreated(t *testing.T) {
	t.Parallel()

	messengerMock := &mockMessenger{}
	contexts := messenger.NewSessionContextRegistry()
	contexts.Put(testutil.TestTenantID(), testutil.TestSessionID(), messenger.SessionContext{
		Platform:        messenger.PlatformSlack,
		ChannelID:       "C123",
		ParentMessageID: "1710000000.000100",
	})

	dispatcher := NewDispatcher(
		testDispatcherLogger(),
		messenger.PlatformSlack,
		New(testDispatcherLogger(), messengerMock),
		messengerMock,
		contexts,
		nil,
		nil,
		nil,
		nil,
		nil,
	)

	record := &adr.ADR{
		ID:        domain.NewID(),
		TenantID:  testutil.TestTenantID(),
		ProjectID: testutil.TestProjectID(),
		Title:     "Choose pgx for Postgres access",
		Status:    adr.StatusProposed,
		Decision:  "Use pgx directly for lower-level control.",
		Sequence:  7,
		CreatedAt: time.Date(2026, time.March, 12, 16, 0, 0, 0, time.UTC),
	}

	err := dispatcher.NotifyADRCreated(context.Background(), testutil.TestTenantID(), testutil.TestSessionID(), record)
	require.NoError(t, err)
	require.Len(t, messengerMock.threads, 1)
	require.Equal(t, "C123", messengerMock.threads[0].ChannelID)
	require.Equal(t, "1710000000.000100", messengerMock.threads[0].ParentMessageID)
	require.Contains(t, messengerMock.threads[0].Text, "ADR-7")
	require.NotEmpty(t, messengerMock.threads[0].Blocks)
}

func TestDispatcherNotifyADRCreatedSkipsWithoutSessionContext(t *testing.T) {
	t.Parallel()

	messengerMock := &mockMessenger{}
	dispatcher := NewDispatcher(
		testDispatcherLogger(),
		messenger.PlatformSlack,
		New(testDispatcherLogger(), messengerMock),
		messengerMock,
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
	)

	record := &adr.ADR{ID: domain.NewID(), Title: "Use pgx", Status: adr.StatusProposed, Sequence: 1}
	err := dispatcher.NotifyADRCreated(context.Background(), testutil.TestTenantID(), testutil.TestSessionID(), record)
	require.NoError(t, err)
	require.Empty(t, messengerMock.threads)
}

func TestDispatcherNotifyQuestionTimedOutUsesFallbackEmail(t *testing.T) {
	t.Parallel()

	messengerMock := &mockMessenger{notificationFn: func(context.Context, messenger.NotificationParams) error {
		return errors.New("messenger down")
	}}
	emailMock := &mockEmailSender{}
	assignee := testutil.TestUserID()
	question := &hitl.Question{
		ID:             domain.NewID(),
		TenantID:       testutil.TestTenantID(),
		AgentSessionID: testutil.TestSessionID(),
		Question:       "Need approval?",
	}
	email := "user@example.com"
	dispatcher := NewDispatcher(
		testDispatcherLogger(),
		messenger.PlatformSlack,
		NewWithEmail(testDispatcherLogger(), messengerMock, emailMock),
		messengerMock,
		nil,
		&fakeLinkQuery{links: []sqlc.UserMessengerLink{{ExternalID: "U123", Platform: string(messenger.PlatformSlack)}}},
		&testutil.MockTaskRepo{GetByIDFn: func(context.Context, domain.TenantID, domain.TaskID) (*task.Task, error) {
			return &task.Task{ID: testutil.TestTaskID(), Title: "Fix auth", AssignedTo: &assignee}, nil
		}},
		&testutil.MockAgentRepo{GetByIDFn: func(context.Context, domain.TenantID, domain.AgentSessionID) (*agentdom.Session, error) {
			return &agentdom.Session{ID: testutil.TestSessionID(), TaskID: testutil.TestTaskID()}, nil
		}},
		&testutil.MockHITLRepo{},
		&fakeUserLookup{user: &userdom.User{Email: &email}},
	)

	require.NoError(t, dispatcher.NotifyQuestionTimedOut(context.Background(), question))
	require.Len(t, emailMock.payloads, 1)
	require.Equal(t, "user@example.com", emailMock.payloads[0].To)
}
