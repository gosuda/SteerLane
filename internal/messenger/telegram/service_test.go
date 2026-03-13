package telegram

import (
	"context"
	"io"
	"log/slog"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/gosuda/steerlane/internal/domain"
	agentdom "github.com/gosuda/steerlane/internal/domain/agent"
	"github.com/gosuda/steerlane/internal/domain/hitl"
	"github.com/gosuda/steerlane/internal/domain/task"
	"github.com/gosuda/steerlane/internal/messenger"
	"github.com/gosuda/steerlane/internal/testutil"
)

type mockContextResolver struct {
	resolveContextFn func(context.Context, string, string) (ResolvedContext, error)
	resolveChatFn    func(context.Context, string) (ResolvedContext, error)
}

func (m *mockContextResolver) ResolveContext(ctx context.Context, chatID, userID string) (ResolvedContext, error) {
	return m.resolveContextFn(ctx, chatID, userID)
}
func (m *mockContextResolver) ResolveChatContext(ctx context.Context, chatID string) (ResolvedContext, error) {
	return m.resolveChatFn(ctx, chatID)
}

type mockTaskCreator struct {
	createFn func(context.Context, *task.Task) error
}

func (m *mockTaskCreator) Create(ctx context.Context, t *task.Task) error { return m.createFn(ctx, t) }

type mockDispatcher struct {
	dispatchFn func(context.Context, domain.TenantID, domain.TaskID, agentdom.AgentType, string) (domain.AgentSessionID, error)
}

func (m *mockDispatcher) DispatchTask(ctx context.Context, tenantID domain.TenantID, taskID domain.TaskID, agentType agentdom.AgentType, prompt string) (domain.AgentSessionID, error) {
	return m.dispatchFn(ctx, tenantID, taskID, agentType, prompt)
}

type mockSender struct {
	sendFn func(context.Context, messenger.SendMessageParams) (messenger.MessageResult, error)
}

func (m *mockSender) SendMessage(ctx context.Context, p messenger.SendMessageParams) (messenger.MessageResult, error) {
	return m.sendFn(ctx, p)
}

type mockNotifier struct {
	notifications []messenger.NotificationParams
}

func (m *mockNotifier) SendNotification(_ context.Context, p messenger.NotificationParams) error {
	m.notifications = append(m.notifications, p)
	return nil
}

type mockLinker struct {
	generateFn func(domain.TenantID, string, string) (string, error)
}

func (m *mockLinker) GenerateLink(tid domain.TenantID, platform, externalUserID string) (string, error) {
	return m.generateFn(tid, platform, externalUserID)
}

type mockThreads struct{ puts []messenger.SessionContext }

func (m *mockThreads) Put(_ domain.TenantID, _ domain.AgentSessionID, ctx messenger.SessionContext) {
	m.puts = append(m.puts, ctx)
}

type mockThreadAnswerer struct {
	answerFn func(context.Context, domain.TenantID, string, string, string, domain.UserID) (*hitl.Question, error)
	resetFn  func(context.Context, domain.TenantID, domain.HITLQuestionID) error
}

func (m *mockThreadAnswerer) AnswerByThread(ctx context.Context, tenantID domain.TenantID, platform, threadID, answer string, by domain.UserID) (*hitl.Question, error) {
	return m.answerFn(ctx, tenantID, platform, threadID, answer, by)
}
func (m *mockThreadAnswerer) ResetQuestion(ctx context.Context, tenantID domain.TenantID, qid domain.HITLQuestionID) error {
	return m.resetFn(ctx, tenantID, qid)
}

type mockQuestionAnswerer struct {
	answerFn func(context.Context, domain.TenantID, domain.HITLQuestionID, string, domain.UserID) error
	getFn    func(context.Context, domain.TenantID, domain.HITLQuestionID) (*hitl.Question, error)
	resetFn  func(context.Context, domain.TenantID, domain.HITLQuestionID) error
}

func (m *mockQuestionAnswerer) AnswerQuestion(ctx context.Context, tenantID domain.TenantID, qid domain.HITLQuestionID, answer string, by domain.UserID) error {
	return m.answerFn(ctx, tenantID, qid, answer, by)
}
func (m *mockQuestionAnswerer) GetQuestion(ctx context.Context, tenantID domain.TenantID, qid domain.HITLQuestionID) (*hitl.Question, error) {
	return m.getFn(ctx, tenantID, qid)
}
func (m *mockQuestionAnswerer) ResetQuestion(ctx context.Context, tenantID domain.TenantID, qid domain.HITLQuestionID) error {
	return m.resetFn(ctx, tenantID, qid)
}

type mockResumer struct{}

func (m *mockResumer) SendHITLAnswer(context.Context, domain.TenantID, domain.AgentSessionID, string) error {
	return nil
}
func testLogger() *slog.Logger { return slog.New(slog.NewTextHandler(io.Discard, nil)) }

func TestServiceHandleCommandCreatesTaskAndStoresThread(t *testing.T) {
	t.Parallel()
	resolved := ResolvedContext{TenantID: testutil.TestTenantID(), ProjectID: testutil.TestProjectID(), UserID: testutil.TestUserID()}
	threads := &mockThreads{}
	var created *task.Task
	svc := NewService(ServiceDeps{Logger: testLogger(), Resolver: &mockContextResolver{resolveContextFn: func(context.Context, string, string) (ResolvedContext, error) { return resolved, nil }}, Tasks: &mockTaskCreator{createFn: func(_ context.Context, got *task.Task) error { created = got; return nil }}, Dispatcher: &mockDispatcher{dispatchFn: func(context.Context, domain.TenantID, domain.TaskID, agentdom.AgentType, string) (domain.AgentSessionID, error) {
		return testutil.TestSessionID(), nil
	}}, Sender: &mockSender{sendFn: func(context.Context, messenger.SendMessageParams) (messenger.MessageResult, error) {
		return messenger.MessageResult{MessageID: "42"}, nil
	}}, Threads: threads})
	require.NoError(t, svc.HandleUpdate(context.Background(), Update{UpdateID: 1, Message: &Message{MessageID: 1, Chat: Chat{ID: 1001}, From: &User{ID: 2002}, Text: "fix auth bug"}}))
	require.NotNil(t, created)
	require.Equal(t, "fix auth bug", created.Title)
	require.Len(t, threads.puts, 1)
	require.Equal(t, "42", threads.puts[0].ParentMessageID)
}

func TestServiceHandleReplyRoutesHITL(t *testing.T) {
	t.Parallel()
	resolved := ResolvedContext{TenantID: testutil.TestTenantID(), ProjectID: testutil.TestProjectID(), UserID: testutil.TestUserID()}
	var gotAnswer string
	svc := NewService(ServiceDeps{Logger: testLogger(), Resolver: &mockContextResolver{resolveContextFn: func(context.Context, string, string) (ResolvedContext, error) { return resolved, nil }}, Answerer: &mockThreadAnswerer{answerFn: func(_ context.Context, _ domain.TenantID, platform, threadID, answer string, _ domain.UserID) (*hitl.Question, error) {
		gotAnswer = answer
		require.Equal(t, string(messenger.PlatformTelegram), platform)
		require.Equal(t, "10", threadID)
		return &hitl.Question{ID: domain.NewID(), AgentSessionID: testutil.TestSessionID()}, nil
	}, resetFn: func(context.Context, domain.TenantID, domain.HITLQuestionID) error { return nil }}, Resumer: &mockResumer{}})
	require.NoError(t, svc.HandleUpdate(context.Background(), Update{UpdateID: 1, Message: &Message{MessageID: 11, Chat: Chat{ID: 1001}, From: &User{ID: 2002}, ReplyToMessage: &Message{MessageID: 10}, Text: "Ship it"}}))
	require.Equal(t, "Ship it", gotAnswer)
}

func TestServiceHandleCommandPromptsLinking(t *testing.T) {
	t.Parallel()
	resolved := ResolvedContext{TenantID: testutil.TestTenantID(), ProjectID: testutil.TestProjectID()}
	notifier := &mockNotifier{}
	svc := NewService(ServiceDeps{Logger: testLogger(), Resolver: &mockContextResolver{resolveContextFn: func(context.Context, string, string) (ResolvedContext, error) {
		return ResolvedContext{}, domain.ErrNotFound
	}, resolveChatFn: func(context.Context, string) (ResolvedContext, error) { return resolved, nil }}, Linker: &mockLinker{generateFn: func(domain.TenantID, string, string) (string, error) { return "https://example.com/link", nil }}, Notifier: notifier, Sender: &mockSender{sendFn: func(context.Context, messenger.SendMessageParams) (messenger.MessageResult, error) {
		return messenger.MessageResult{}, nil
	}}})
	require.NoError(t, svc.HandleUpdate(context.Background(), Update{UpdateID: 1, Message: &Message{MessageID: 1, Chat: Chat{ID: 1001}, From: &User{ID: 2002}, Text: "fix auth bug"}}))
	require.Len(t, notifier.notifications, 1)
	require.Contains(t, notifier.notifications[0].Text, "https://example.com/link")
}

func TestServiceHandleCallbackAnswersQuestion(t *testing.T) {
	t.Parallel()
	resolved := ResolvedContext{TenantID: testutil.TestTenantID(), ProjectID: testutil.TestProjectID(), UserID: testutil.TestUserID()}
	questionID := domain.NewID()
	sessionID := testutil.TestSessionID()
	var answered string
	svc := NewService(ServiceDeps{Logger: testLogger(), Resolver: &mockContextResolver{resolveContextFn: func(context.Context, string, string) (ResolvedContext, error) { return resolved, nil }}, Questions: &mockQuestionAnswerer{answerFn: func(_ context.Context, _ domain.TenantID, _ domain.HITLQuestionID, answer string, _ domain.UserID) error {
		answered = answer
		return nil
	}, getFn: func(context.Context, domain.TenantID, domain.HITLQuestionID) (*hitl.Question, error) {
		return &hitl.Question{ID: questionID, AgentSessionID: sessionID}, nil
	}, resetFn: func(context.Context, domain.TenantID, domain.HITLQuestionID) error { return nil }}, Resumer: &mockResumer{}})
	data := actionPrefix + ":" + questionID.String() + ":" + sessionID.String() + "|Approve"
	require.NoError(t, svc.HandleUpdate(context.Background(), Update{UpdateID: 1, CallbackQuery: &CallbackQuery{ID: "cb1", Data: data, From: User{ID: 2002}, Message: &Message{MessageID: 11, Chat: Chat{ID: 1001}}}}))
	require.Equal(t, "Approve", answered)
}
