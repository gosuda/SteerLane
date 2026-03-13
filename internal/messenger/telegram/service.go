package telegram

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/gosuda/steerlane/internal/domain"
	agentdom "github.com/gosuda/steerlane/internal/domain/agent"
	"github.com/gosuda/steerlane/internal/domain/hitl"
	"github.com/gosuda/steerlane/internal/domain/task"
	"github.com/gosuda/steerlane/internal/messenger"
)

const actionPrefix = "steerlane:hitl"

type ContextResolver interface {
	ResolveContext(ctx context.Context, chatID, userID string) (ResolvedContext, error)
	ResolveChatContext(ctx context.Context, chatID string) (ResolvedContext, error)
}

type TaskCreator interface {
	Create(ctx context.Context, t *task.Task) error
}
type TaskDispatcher interface {
	DispatchTask(ctx context.Context, tenantID domain.TenantID, taskID domain.TaskID, agentType agentdom.AgentType, prompt string) (domain.AgentSessionID, error)
}
type MessageSender interface {
	SendMessage(ctx context.Context, params messenger.SendMessageParams) (messenger.MessageResult, error)
}
type NotificationSender interface {
	SendNotification(ctx context.Context, params messenger.NotificationParams) error
}
type LinkGenerator interface {
	GenerateLink(tenantID domain.TenantID, platform, externalUserID string) (string, error)
}
type SessionContextWriter interface {
	Put(tenantID domain.TenantID, sessionID domain.AgentSessionID, ctx messenger.SessionContext)
}
type ThreadAnswerer interface {
	AnswerByThread(ctx context.Context, tenantID domain.TenantID, platform, threadID, answer string, answeredBy domain.UserID) (*hitl.Question, error)
	ResetQuestion(ctx context.Context, tenantID domain.TenantID, questionID domain.HITLQuestionID) error
}
type HITLAnswerer interface {
	AnswerQuestion(ctx context.Context, tenantID domain.TenantID, questionID domain.HITLQuestionID, answer string, answeredBy domain.UserID) error
	GetQuestion(ctx context.Context, tenantID domain.TenantID, questionID domain.HITLQuestionID) (*hitl.Question, error)
	ResetQuestion(ctx context.Context, tenantID domain.TenantID, questionID domain.HITLQuestionID) error
}
type HITLResumer interface {
	SendHITLAnswer(ctx context.Context, tenantID domain.TenantID, sessionID domain.AgentSessionID, answer string) error
}

type ServiceDeps struct {
	Logger     *slog.Logger
	Resolver   ContextResolver
	Tasks      TaskCreator
	Dispatcher TaskDispatcher
	Sender     MessageSender
	Notifier   NotificationSender
	Linker     LinkGenerator
	Resumer    HITLResumer
	Threads    SessionContextWriter
	Answerer   ThreadAnswerer
	Questions  HITLAnswerer
}

type Service struct {
	logger     *slog.Logger
	resolver   ContextResolver
	tasks      TaskCreator
	dispatcher TaskDispatcher
	sender     MessageSender
	notifier   NotificationSender
	linker     LinkGenerator
	resumer    HITLResumer
	threads    SessionContextWriter
	answerer   ThreadAnswerer
	questions  HITLAnswerer
}

var _ UpdateProcessor = (*Service)(nil)

func NewService(deps ServiceDeps) *Service {
	logger := deps.Logger
	if logger == nil {
		logger = slog.Default()
	}
	return &Service{logger: logger.With("component", "telegram.service"), resolver: deps.Resolver, tasks: deps.Tasks, dispatcher: deps.Dispatcher, sender: deps.Sender, notifier: deps.Notifier, linker: deps.Linker, resumer: deps.Resumer, threads: deps.Threads, answerer: deps.Answerer, questions: deps.Questions}
}

func (s *Service) HandleUpdate(ctx context.Context, update Update) error {
	if update.CallbackQuery != nil {
		return s.handleCallback(ctx, *update.CallbackQuery)
	}
	if update.Message == nil || strings.TrimSpace(update.Message.Text) == "" {
		return nil
	}
	if update.Message.ReplyToMessage != nil {
		return s.handleReply(ctx, *update.Message)
	}
	return s.handleCommand(ctx, *update.Message)
}

func (s *Service) handleCommand(ctx context.Context, msg Message) error {
	const op = "telegram.Service.handleCommand"
	if s.resolver == nil {
		return fmt.Errorf("%s: context resolver not configured: %w", op, domain.ErrMessengerUnavailable)
	}
	cmd, parseErr := messenger.ParseCommand(msg.Text)
	if parseErr != nil {
		return fmt.Errorf("%s: parse command: %w", op, parseErr)
	}
	userExternalID := telegramUserID(msg.From)
	resolved, err := s.resolver.ResolveContext(ctx, strconv.FormatInt(msg.Chat.ID, 10), userExternalID)
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			return s.promptAccountLink(ctx, msg.Chat.ID, userExternalID)
		}
		return fmt.Errorf("%s: resolve context: %w", op, err)
	}
	now := time.Now().UTC()
	t := &task.Task{ID: domain.NewID(), TenantID: resolved.TenantID, ProjectID: resolved.ProjectID, AssignedTo: &resolved.UserID, Title: cmd.Title, Description: cmd.Description, Status: task.StatusBacklog, Priority: 2, CreatedAt: now, UpdatedAt: now}
	if err = s.tasks.Create(ctx, t); err != nil {
		return fmt.Errorf("%s: create task: %w", op, err)
	}
	var sessionID domain.AgentSessionID
	if s.dispatcher != nil {
		promptText := t.Title
		if t.Description != "" {
			promptText += "\n\n" + t.Description
		}
		sessionID, err = s.dispatcher.DispatchTask(ctx, resolved.TenantID, t.ID, agentdom.TypeClaude, promptText)
		if err != nil {
			s.logger.ErrorContext(ctx, "dispatch failed after task creation", "error", err, "task_id", t.ID)
		}
	}
	if s.sender != nil {
		ackText := buildTaskAcknowledgement(t, sessionID)
		result, sendErr := s.sender.SendMessage(ctx, messenger.SendMessageParams{ChannelID: strconv.FormatInt(msg.Chat.ID, 10), Text: ackText})
		if sendErr == nil && s.threads != nil && sessionID != (domain.AgentSessionID{}) {
			s.threads.Put(resolved.TenantID, sessionID, messenger.SessionContext{Platform: messenger.PlatformTelegram, ChannelID: strconv.FormatInt(msg.Chat.ID, 10), ParentMessageID: result.MessageID})
		}
	}
	return nil
}

func (s *Service) handleReply(ctx context.Context, msg Message) error {
	const op = "telegram.Service.handleReply"
	if s.resolver == nil || s.answerer == nil {
		return nil
	}
	resolved, err := s.resolver.ResolveContext(ctx, strconv.FormatInt(msg.Chat.ID, 10), telegramUserID(msg.From))
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			return nil
		}
		return fmt.Errorf("%s: resolve context: %w", op, err)
	}
	question, err := s.answerer.AnswerByThread(ctx, resolved.TenantID, string(messenger.PlatformTelegram), strconv.FormatInt(msg.ReplyToMessage.MessageID, 10), strings.TrimSpace(msg.Text), resolved.UserID)
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) || errors.Is(err, domain.ErrInvalidInput) {
			return nil
		}
		return fmt.Errorf("%s: answer by thread: %w", op, err)
	}
	if s.resumer != nil {
		resumeCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		if err = s.resumer.SendHITLAnswer(resumeCtx, resolved.TenantID, question.AgentSessionID, strings.TrimSpace(msg.Text)); err != nil {
			rollbackCtx, rollbackCancel := context.WithTimeout(context.Background(), 15*time.Second)
			defer rollbackCancel()
			if resetErr := s.answerer.ResetQuestion(rollbackCtx, resolved.TenantID, question.ID); resetErr != nil {
				return fmt.Errorf("%s: resume session: %w (rollback failed: %w)", op, err, resetErr)
			}
			return fmt.Errorf("%s: resume session: %w", op, err)
		}
	}
	return nil
}

func (s *Service) handleCallback(ctx context.Context, callback CallbackQuery) error {
	const op = "telegram.Service.handleCallback"
	if s.resolver == nil || s.questions == nil {
		return nil
	}
	parsed, err := parseComponentAction(callback.Data)
	if err != nil {
		return nil
	}
	chatID := ""
	if callback.Message != nil {
		chatID = strconv.FormatInt(callback.Message.Chat.ID, 10)
	}
	resolved, err := s.resolver.ResolveContext(ctx, chatID, strconv.FormatInt(callback.From.ID, 10))
	if err != nil {
		return fmt.Errorf("%s: resolve context: %w", op, err)
	}
	if err = s.questions.AnswerQuestion(ctx, resolved.TenantID, parsed.QuestionID, parsed.Answer, resolved.UserID); err != nil {
		if errors.Is(err, domain.ErrInvalidInput) || errors.Is(err, domain.ErrInvalidTransition) || errors.Is(err, domain.ErrNotFound) {
			return nil
		}
		return fmt.Errorf("%s: answer question: %w", op, err)
	}
	if s.resumer != nil {
		resumeCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		question, getErr := s.questions.GetQuestion(resumeCtx, resolved.TenantID, parsed.QuestionID)
		if getErr != nil {
			return fmt.Errorf("%s: get question: %w", op, getErr)
		}
		if resumeErr := s.resumer.SendHITLAnswer(resumeCtx, resolved.TenantID, question.AgentSessionID, parsed.Answer); resumeErr != nil {
			rollbackCtx, rollbackCancel := context.WithTimeout(context.Background(), 15*time.Second)
			defer rollbackCancel()
			if resetErr := s.questions.ResetQuestion(rollbackCtx, resolved.TenantID, parsed.QuestionID); resetErr != nil {
				return fmt.Errorf("%s: resume session: %w (rollback failed: %w)", op, resumeErr, resetErr)
			}
			return fmt.Errorf("%s: resume session: %w", op, resumeErr)
		}
	}
	return nil
}

func (s *Service) promptAccountLink(ctx context.Context, chatID int64, externalUserID string) error {
	if s.linker == nil || s.notifier == nil || s.resolver == nil {
		return domain.ErrNotFound
	}
	resolved, err := s.resolver.ResolveChatContext(ctx, strconv.FormatInt(chatID, 10))
	if err != nil {
		return err
	}
	linkURL, err := s.linker.GenerateLink(resolved.TenantID, string(messenger.PlatformTelegram), externalUserID)
	if err != nil {
		return err
	}
	notifyErr := s.notifier.SendNotification(ctx, messenger.NotificationParams{UserExternalID: externalUserID, Text: BuildLinkingDM(linkURL)})
	if notifyErr != nil {
		return notifyErr
	}
	if s.sender != nil {
		_, _ = s.sender.SendMessage(ctx, messenger.SendMessageParams{ChannelID: strconv.FormatInt(chatID, 10), Text: "I sent you a DM to connect your Telegram account before I can create tasks or answer HITL prompts."})
	}
	return nil
}

type componentAction struct {
	Answer     string
	QuestionID domain.HITLQuestionID
	SessionID  domain.AgentSessionID
}

func parseComponentAction(data string) (componentAction, error) {
	rawRoute, answer, ok := strings.Cut(data, "|")
	if !ok || strings.TrimSpace(answer) == "" {
		return componentAction{}, domain.ErrInvalidInput
	}
	parts := strings.SplitN(rawRoute, ":", 4)
	if len(parts) != 4 || parts[0]+":"+parts[1] != actionPrefix {
		return componentAction{}, domain.ErrInvalidInput
	}
	questionID, err := uuid.Parse(parts[2])
	if err != nil {
		return componentAction{}, domain.ErrInvalidInput
	}
	sessionID, err := uuid.Parse(parts[3])
	if err != nil {
		return componentAction{}, domain.ErrInvalidInput
	}
	decoded, err := url.QueryUnescape(answer)
	if err != nil || decoded == "" {
		return componentAction{}, domain.ErrInvalidInput
	}
	return componentAction{QuestionID: questionID, SessionID: sessionID, Answer: decoded}, nil
}

func buildTaskAcknowledgement(record *task.Task, sessionID domain.AgentSessionID) string {
	ack := fmt.Sprintf("Created task: *%s*", record.Title)
	if sessionID != (domain.AgentSessionID{}) {
		ack += fmt.Sprintf(" (dispatched, session %s)", sessionID.String()[:8])
	}
	return ack
}

func telegramUserID(user *User) string {
	if user == nil {
		return ""
	}
	return strconv.FormatInt(user.ID, 10)
}
