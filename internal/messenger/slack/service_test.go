package slack

import (
	"context"
	"encoding/json"
	"errors"
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
	resolveContextFn        func(ctx context.Context, slackTeamID, slackChannelID, slackUserID string) (ResolvedContext, error)
	resolveChannelContextFn func(ctx context.Context, slackTeamID, slackChannelID string) (ResolvedContext, error)
}

func (m *mockContextResolver) ResolveContext(ctx context.Context, slackTeamID, slackChannelID, slackUserID string) (ResolvedContext, error) {
	if m.resolveContextFn != nil {
		return m.resolveContextFn(ctx, slackTeamID, slackChannelID, slackUserID)
	}
	return ResolvedContext{}, nil
}

func (m *mockContextResolver) ResolveChannelContext(ctx context.Context, slackTeamID, slackChannelID string) (ResolvedContext, error) {
	if m.resolveChannelContextFn != nil {
		return m.resolveChannelContextFn(ctx, slackTeamID, slackChannelID)
	}
	return ResolvedContext{}, nil
}

type mockTaskCreator struct {
	createFn func(ctx context.Context, t *task.Task) error
}

func (m *mockTaskCreator) Create(ctx context.Context, t *task.Task) error {
	if m.createFn != nil {
		return m.createFn(ctx, t)
	}
	return nil
}

type mockDispatcher struct {
	dispatchFn            func(ctx context.Context, tenantID domain.TenantID, taskID domain.TaskID, agentType agentdom.AgentType, prompt string) (domain.AgentSessionID, error)
	dispatchWithContextFn func(ctx context.Context, tenantID domain.TenantID, taskID domain.TaskID, agentType agentdom.AgentType, prompt string, sessionCtx messenger.SessionContext) (domain.AgentSessionID, error)
}

func (m *mockDispatcher) DispatchTask(ctx context.Context, tenantID domain.TenantID, taskID domain.TaskID, agentType agentdom.AgentType, prompt string) (domain.AgentSessionID, error) {
	if m.dispatchFn != nil {
		return m.dispatchFn(ctx, tenantID, taskID, agentType, prompt)
	}
	return domain.AgentSessionID{}, nil
}

func (m *mockDispatcher) DispatchTaskWithContext(ctx context.Context, tenantID domain.TenantID, taskID domain.TaskID, agentType agentdom.AgentType, prompt string, sessionCtx messenger.SessionContext) (domain.AgentSessionID, error) {
	if m.dispatchWithContextFn != nil {
		return m.dispatchWithContextFn(ctx, tenantID, taskID, agentType, prompt, sessionCtx)
	}
	return m.DispatchTask(ctx, tenantID, taskID, agentType, prompt)
}

type mockResumer struct {
	resumeFn func(ctx context.Context, tenantID domain.TenantID, sessionID domain.AgentSessionID, answer string) error
	answers  []string
}

func (m *mockResumer) SendHITLAnswer(ctx context.Context, tenantID domain.TenantID, sessionID domain.AgentSessionID, answer string) error {
	m.answers = append(m.answers, answer)
	if m.resumeFn != nil {
		return m.resumeFn(ctx, tenantID, sessionID, answer)
	}
	return nil
}

type mockThreadAnswerer struct {
	answerFn func(ctx context.Context, tenantID domain.TenantID, platform, threadID, answer string, answeredBy domain.UserID) (*hitl.Question, error)
	resetFn  func(ctx context.Context, tenantID domain.TenantID, questionID domain.HITLQuestionID) error
}

func (m *mockThreadAnswerer) AnswerByThread(ctx context.Context, tenantID domain.TenantID, platform, threadID, answer string, answeredBy domain.UserID) (*hitl.Question, error) {
	if m.answerFn != nil {
		return m.answerFn(ctx, tenantID, platform, threadID, answer, answeredBy)
	}
	return nil, nil
}

func (m *mockThreadAnswerer) ResetQuestion(ctx context.Context, tenantID domain.TenantID, questionID domain.HITLQuestionID) error {
	if m.resetFn != nil {
		return m.resetFn(ctx, tenantID, questionID)
	}
	return nil
}

type mockSender struct {
	sendFn func(ctx context.Context, params messenger.SendMessageParams) (messenger.MessageResult, error)
}

func (m *mockSender) SendMessage(ctx context.Context, params messenger.SendMessageParams) (messenger.MessageResult, error) {
	if m.sendFn != nil {
		return m.sendFn(ctx, params)
	}
	return messenger.MessageResult{}, nil
}

type mockNotifier struct {
	notifyFn      func(ctx context.Context, params messenger.NotificationParams) error
	notifications []messenger.NotificationParams
}

func (m *mockNotifier) SendNotification(ctx context.Context, params messenger.NotificationParams) error {
	m.notifications = append(m.notifications, params)
	if m.notifyFn != nil {
		return m.notifyFn(ctx, params)
	}
	return nil
}

type mockLinker struct {
	generateFn func(tenantID domain.TenantID, platform, externalUserID string) (string, error)
}

func (m *mockLinker) GenerateLink(tenantID domain.TenantID, platform, externalUserID string) (string, error) {
	if m.generateFn != nil {
		return m.generateFn(tenantID, platform, externalUserID)
	}
	return "", nil
}

func decodeSlackBlocks(t *testing.T, data []byte) []map[string]any {
	t.Helper()

	var blocks []map[string]any
	require.NoError(t, json.Unmarshal(data, &blocks))
	return blocks
}

func TestServiceHandleAppMention(t *testing.T) {
	t.Parallel()

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	resolved := ResolvedContext{
		TenantID:  testutil.TestTenantID(),
		ProjectID: testutil.TestProjectID(),
		UserID:    testutil.TestUserID(),
	}

	t.Run("creates task dispatches and acknowledges", func(t *testing.T) {
		t.Parallel()

		var created *task.Task
		var prompt string
		var sent messenger.SendMessageParams
		threadRegistry := messenger.NewSessionContextRegistry()

		svc := NewService(ServiceDeps{
			Logger: logger,
			Resolver: &mockContextResolver{resolveContextFn: func(ctx context.Context, slackTeamID, slackChannelID, slackUserID string) (ResolvedContext, error) {
				require.Equal(t, "T123", slackTeamID)
				require.Equal(t, "C123", slackChannelID)
				require.Equal(t, "U123", slackUserID)
				return resolved, nil
			}},
			Tasks: &mockTaskCreator{createFn: func(ctx context.Context, got *task.Task) error {
				created = got
				return nil
			}},
			Dispatcher: &mockDispatcher{dispatchWithContextFn: func(ctx context.Context, tenantID domain.TenantID, taskID domain.TaskID, agentType agentdom.AgentType, gotPrompt string, sessionCtx messenger.SessionContext) (domain.AgentSessionID, error) {
				prompt = gotPrompt
				require.Equal(t, resolved.TenantID, tenantID)
				require.Equal(t, agentdom.TypeClaude, agentType)
				require.Equal(t, messenger.PlatformSlack, sessionCtx.Platform)
				require.Equal(t, "C123", sessionCtx.ChannelID)
				require.Equal(t, "1710000000.000100", sessionCtx.ParentMessageID)
				return testutil.TestSessionID(), nil
			}},
			Sender: &mockSender{sendFn: func(ctx context.Context, params messenger.SendMessageParams) (messenger.MessageResult, error) {
				sent = params
				return messenger.MessageResult{}, nil
			}},
			Threads: threadRegistry,
		})

		err := svc.HandleAppMention(InnerEvent{TeamID: "T123", Channel: "C123", User: "U123", TS: "1710000000.000100", Text: "<@BOT> fix auth bug\nrefresh tokens fail"})
		require.NoError(t, err)
		require.NotNil(t, created)
		require.Equal(t, "fix auth bug", created.Title)
		require.Equal(t, "refresh tokens fail", created.Description)
		require.NotNil(t, created.AssignedTo)
		require.Equal(t, resolved.UserID, *created.AssignedTo)
		require.Equal(t, task.StatusBacklog, created.Status)
		require.Equal(t, "fix auth bug\n\nrefresh tokens fail", prompt)
		require.Equal(t, "C123", sent.ChannelID)
		require.Contains(t, sent.Text, "Created task")
		blocks := decodeSlackBlocks(t, sent.Blocks)
		require.Len(t, blocks, 3)
		require.Equal(t, "header", blocks[0]["type"])

		threadCtx, ok := threadRegistry.Get(resolved.TenantID, testutil.TestSessionID())
		require.True(t, ok)
		require.Equal(t, messenger.PlatformSlack, threadCtx.Platform)
		require.Equal(t, "C123", threadCtx.ChannelID)
		require.Equal(t, "1710000000.000100", threadCtx.ParentMessageID)
	})

	t.Run("returns unavailable when resolver is missing", func(t *testing.T) {
		t.Parallel()

		svc := NewService(ServiceDeps{Logger: logger})
		err := svc.HandleAppMention(InnerEvent{TeamID: "T123", Channel: "C123", Text: "<@BOT> fix auth bug"})
		require.ErrorIs(t, err, domain.ErrMessengerUnavailable)
	})

	t.Run("prompts account linking when messenger identity is missing", func(t *testing.T) {
		t.Parallel()

		notifier := &mockNotifier{}
		var ack messenger.SendMessageParams
		svc := NewService(ServiceDeps{
			Logger: logger,
			Resolver: &mockContextResolver{
				resolveContextFn: func(context.Context, string, string, string) (ResolvedContext, error) {
					return ResolvedContext{}, domain.ErrNotFound
				},
				resolveChannelContextFn: func(context.Context, string, string) (ResolvedContext, error) {
					return ResolvedContext{TenantID: resolved.TenantID, ProjectID: resolved.ProjectID}, nil
				},
			},
			Linker: &mockLinker{generateFn: func(tenantID domain.TenantID, platform, externalUserID string) (string, error) {
				require.Equal(t, resolved.TenantID, tenantID)
				require.Equal(t, string(messenger.PlatformSlack), platform)
				require.Equal(t, "U123", externalUserID)
				return "https://steerlane.example.com/auth/link?token=abc", nil
			}},
			Notifier: notifier,
			Sender: &mockSender{sendFn: func(_ context.Context, params messenger.SendMessageParams) (messenger.MessageResult, error) {
				ack = params
				return messenger.MessageResult{}, nil
			}},
		})

		err := svc.HandleAppMention(InnerEvent{TeamID: "T123", Channel: "C123", User: "U123", Text: "<@BOT> fix auth bug"})
		require.NoError(t, err)
		require.Len(t, notifier.notifications, 1)
		require.Equal(t, "U123", notifier.notifications[0].UserExternalID)
		require.Contains(t, notifier.notifications[0].Text, "/auth/link?token=abc")
		require.Equal(t, "C123", ack.ChannelID)
		require.Contains(t, ack.Text, "I sent you a DM")
	})

	t.Run("surfaces task create failure", func(t *testing.T) {
		t.Parallel()

		svc := NewService(ServiceDeps{
			Logger: logger,
			Resolver: &mockContextResolver{resolveContextFn: func(ctx context.Context, slackTeamID, slackChannelID, slackUserID string) (ResolvedContext, error) {
				return resolved, nil
			}},
			Tasks: &mockTaskCreator{createFn: func(ctx context.Context, t *task.Task) error {
				return errors.New("boom")
			}},
		})

		err := svc.HandleAppMention(InnerEvent{TeamID: "T123", Channel: "C123", User: "U123", Text: "<@BOT> fix auth bug"})
		require.Error(t, err)
		require.Contains(t, err.Error(), "create task")
	})

	t.Run("routes threaded reply to hitl question and resumes session", func(t *testing.T) {
		t.Parallel()

		resumer := &mockResumer{}
		var answeredBy domain.UserID
		svc := NewService(ServiceDeps{
			Logger: logger,
			Resolver: &mockContextResolver{resolveContextFn: func(context.Context, string, string, string) (ResolvedContext, error) {
				return resolved, nil
			}},
			Answerer: &mockThreadAnswerer{answerFn: func(_ context.Context, tenantID domain.TenantID, platform, threadID, answer string, userID domain.UserID) (*hitl.Question, error) {
				require.Equal(t, resolved.TenantID, tenantID)
				require.Equal(t, string(messenger.PlatformSlack), platform)
				require.Equal(t, "1710000000.000100", threadID)
				require.Equal(t, "Ship it", answer)
				answeredBy = userID
				return &hitl.Question{ID: domain.NewID(), AgentSessionID: testutil.TestSessionID()}, nil
			}},
			Resumer: resumer,
		})

		err := svc.HandleMessage(InnerEvent{TeamID: "T123", Channel: "C123", User: "U123", ThreadTS: "1710000000.000100", Text: "Ship it"})
		require.NoError(t, err)
		require.Equal(t, resolved.UserID, answeredBy)
		require.Equal(t, []string{"Ship it"}, resumer.answers)
	})

	t.Run("rolls back threaded reply when resume fails", func(t *testing.T) {
		t.Parallel()

		resetCalls := 0
		svc := NewService(ServiceDeps{
			Logger: logger,
			Resolver: &mockContextResolver{resolveContextFn: func(context.Context, string, string, string) (ResolvedContext, error) {
				return resolved, nil
			}},
			Answerer: &mockThreadAnswerer{
				answerFn: func(_ context.Context, _ domain.TenantID, _ string, _ string, _ string, _ domain.UserID) (*hitl.Question, error) {
					return &hitl.Question{ID: domain.NewID(), AgentSessionID: testutil.TestSessionID()}, nil
				},
				resetFn: func(_ context.Context, tenantID domain.TenantID, questionID domain.HITLQuestionID) error {
					require.Equal(t, resolved.TenantID, tenantID)
					require.NotEqual(t, domain.HITLQuestionID{}, questionID)
					resetCalls++
					return nil
				},
			},
			Resumer: &mockResumer{resumeFn: func(context.Context, domain.TenantID, domain.AgentSessionID, string) error {
				return errors.New("resume failed")
			}},
		})

		err := svc.HandleMessage(InnerEvent{TeamID: "T123", Channel: "C123", User: "U123", ThreadTS: "1710000000.000100", Text: "Ship it"})
		require.Error(t, err)
		require.Contains(t, err.Error(), "resume session")
		require.Equal(t, 1, resetCalls)
	})

	t.Run("ignores ambiguous threaded replies", func(t *testing.T) {
		t.Parallel()

		resumer := &mockResumer{}
		svc := NewService(ServiceDeps{
			Logger: logger,
			Resolver: &mockContextResolver{resolveContextFn: func(context.Context, string, string, string) (ResolvedContext, error) {
				return resolved, nil
			}},
			Answerer: &mockThreadAnswerer{answerFn: func(context.Context, domain.TenantID, string, string, string, domain.UserID) (*hitl.Question, error) {
				return nil, domain.ErrInvalidInput
			}},
			Resumer: resumer,
		})

		err := svc.HandleMessage(InnerEvent{TeamID: "T123", Channel: "C123", User: "U123", ThreadTS: "1710000000.000100", Text: "Ship it"})
		require.NoError(t, err)
		require.Empty(t, resumer.answers)
	})
}
