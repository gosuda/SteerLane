package slack

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/gosuda/steerlane/internal/domain"
	agentdom "github.com/gosuda/steerlane/internal/domain/agent"
	"github.com/gosuda/steerlane/internal/domain/hitl"
	"github.com/gosuda/steerlane/internal/domain/task"
	"github.com/gosuda/steerlane/internal/messenger"
)

// ---------------------------------------------------------------------------
// Narrow interfaces — keep the slack package decoupled from store/sqlc.
// ---------------------------------------------------------------------------

// ContextResolver maps Slack workspace/channel/user identifiers to SteerLane
// tenant, project, and user context. Implementations live in the integration
// layer (e.g. a postgres-backed adapter).
type ContextResolver interface {
	ResolveContext(ctx context.Context, slackTeamID, slackChannelID, slackUserID string) (ResolvedContext, error)
}

type channelContextResolver interface {
	ResolveChannelContext(ctx context.Context, slackTeamID, slackChannelID string) (ResolvedContext, error)
}

// ResolvedContext holds the SteerLane identifiers that correspond to a
// Slack workspace + channel pair.
type ResolvedContext struct {
	TenantID  domain.TenantID
	ProjectID domain.ProjectID
	UserID    domain.UserID
}

// TaskCreator creates tasks in the domain layer. Satisfied by task.Repository.
type TaskCreator interface {
	Create(ctx context.Context, t *task.Task) error
}

// TaskDispatcher dispatches a backlog task to an agent. Satisfied by
// orchestrator.Orchestrator.DispatchTask.
type TaskDispatcher interface {
	DispatchTask(
		ctx context.Context,
		tenantID domain.TenantID,
		taskID domain.TaskID,
		agentType agentdom.AgentType,
		prompt string,
	) (domain.AgentSessionID, error)
}

type contextualTaskDispatcher interface {
	DispatchTaskWithContext(
		ctx context.Context,
		tenantID domain.TenantID,
		taskID domain.TaskID,
		agentType agentdom.AgentType,
		prompt string,
		sessionCtx messenger.SessionContext,
	) (domain.AgentSessionID, error)
}

// MessageSender sends acknowledgement/status messages back to a messenger
// channel. Satisfied by any messenger.Messenger implementation.
type MessageSender interface {
	SendMessage(ctx context.Context, params messenger.SendMessageParams) (messenger.MessageResult, error)
}

type NotificationSender interface {
	SendNotification(ctx context.Context, params messenger.NotificationParams) error
}

type LinkGenerator interface {
	GenerateLink(tenantID domain.TenantID, platform, externalUserID string) (string, error)
}

// SessionContextWriter records messenger session routing context after dispatch.
type SessionContextWriter interface {
	Put(tenantID domain.TenantID, sessionID domain.AgentSessionID, ctx messenger.SessionContext)
}

// ThreadAnswerer resolves and answers pending HITL questions by messenger thread.
type ThreadAnswerer interface {
	AnswerByThread(
		ctx context.Context,
		tenantID domain.TenantID,
		platform, threadID, answer string,
		answeredBy domain.UserID,
	) (*hitl.Question, error)
	ResetQuestion(ctx context.Context, tenantID domain.TenantID, questionID domain.HITLQuestionID) error
}

// ---------------------------------------------------------------------------
// Service — Slack-facing router implementing EventHandler
// ---------------------------------------------------------------------------

// Compile-time check: Service satisfies EventHandler.
var _ EventHandler = (*Service)(nil)

// ServiceDeps holds the injected dependencies for Service.
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
}

// Service implements the EventHandler interface wired into the Slack Handler.
// It parses app_mention events into tasks, creates them in the backlog, and
// dispatches them to the orchestrator.
type Service struct {
	logger     *slog.Logger
	resolver   ContextResolver
	tasks      TaskCreator
	dispatcher TaskDispatcher
	sender     MessageSender
	resumer    HITLResumer
	threads    SessionContextWriter
	answerer   ThreadAnswerer
	notifier   NotificationSender
	linker     LinkGenerator
}

// NewService creates a Service with the given dependencies.
// All dependencies are required; the caller must validate before calling.
func NewService(deps ServiceDeps) *Service {
	logger := deps.Logger
	if logger == nil {
		logger = slog.Default()
	}
	return &Service{
		logger:     logger.With("component", "slack.service"),
		resolver:   deps.Resolver,
		tasks:      deps.Tasks,
		dispatcher: deps.Dispatcher,
		sender:     deps.Sender,
		resumer:    deps.Resumer,
		threads:    deps.Threads,
		answerer:   deps.Answerer,
		notifier:   deps.Notifier,
		linker:     deps.Linker,
	}
}

// HandleAppMention processes an app_mention event: parses the mention text
// into a task, creates it in the backlog, dispatches it, and acknowledges
// back in the channel.
func (s *Service) HandleAppMention(event InnerEvent) error {
	const op = "slack.Service.HandleAppMention"
	ctx := context.Background()

	// Step 1: Parse the mention text into a command.
	cmd, err := messenger.ParseCommand(event.Text)
	if err != nil {
		s.logger.WarnContext(ctx, "unparseable mention ignored",
			slog.String("error", err.Error()),
			slog.String("user", event.User),
			slog.String("channel", event.Channel),
		)
		// Not a dispatch error — Slack already got a 200.
		return nil
	}

	// Step 2: Resolve tenant/project/user from Slack team+channel context.
	if s.resolver == nil {
		return fmt.Errorf("%s: context resolver not configured: %w", op, domain.ErrMessengerUnavailable)
	}

	// We don't have the team ID on InnerEvent. The team ID was on the outer
	// EventRequest envelope. For now, pass empty and let the resolver match
	// by channel. TODO(1C.3): Thread team_id through from envelope.
	resolved, err := s.resolver.ResolveContext(ctx, event.TeamID, event.Channel, event.User)
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			if linkErr := s.promptAccountLink(ctx, event); linkErr == nil {
				return nil
			}
		}
		return fmt.Errorf("%s: resolve context: %w", op, err)
	}

	// Step 3: Create a backlog task.
	now := time.Now().UTC()
	t := &task.Task{
		ID:          domain.NewID(),
		TenantID:    resolved.TenantID,
		ProjectID:   resolved.ProjectID,
		AssignedTo:  &resolved.UserID,
		Title:       cmd.Title,
		Description: cmd.Description,
		Status:      task.StatusBacklog,
		Priority:    2, // default medium priority for messenger-created tasks
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	if err = s.tasks.Create(ctx, t); err != nil {
		return fmt.Errorf("%s: create task: %w", op, err)
	}

	s.logger.InfoContext(ctx, "task created from mention",
		slog.String("task_id", t.ID.String()),
		slog.String("title", t.Title),
		slog.String("channel", event.Channel),
		slog.String("user", event.User),
	)

	// Step 4: Dispatch the task to an agent.
	// Use the default Claude agent type for messenger-triggered tasks.
	prompt := t.Title
	if t.Description != "" {
		prompt = t.Title + "\n\n" + t.Description
	}
	parentMessageID := event.TS
	if strings.TrimSpace(event.ThreadTS) != "" {
		parentMessageID = event.ThreadTS
	}

	var sessionID domain.AgentSessionID
	if s.dispatcher != nil {
		if contextual, ok := s.dispatcher.(contextualTaskDispatcher); ok && strings.TrimSpace(parentMessageID) != "" {
			sessionID, err = contextual.DispatchTaskWithContext(ctx, resolved.TenantID, t.ID, agentdom.TypeClaude, prompt, messenger.SessionContext{
				Platform:        messenger.PlatformSlack,
				ChannelID:       event.Channel,
				ParentMessageID: parentMessageID,
			})
		} else {
			sessionID, err = s.dispatcher.DispatchTask(ctx, resolved.TenantID, t.ID, agentdom.TypeClaude, prompt)
		}
		if err != nil {
			// Task was created but dispatch failed. Log and acknowledge the
			// task creation; the user can dispatch manually from the board.
			s.logger.ErrorContext(ctx, "dispatch failed after task creation",
				slog.String("error", err.Error()),
				slog.String("task_id", t.ID.String()),
			)
		}
	}
	if s.threads != nil && sessionID != (domain.AgentSessionID{}) && strings.TrimSpace(parentMessageID) != "" {
		s.threads.Put(resolved.TenantID, sessionID, messenger.SessionContext{
			Platform:        messenger.PlatformSlack,
			ChannelID:       event.Channel,
			ParentMessageID: parentMessageID,
		})
	}

	// Step 5: Acknowledge in the channel.
	if s.sender != nil {
		ackText, ackBlocks, buildErr := buildTaskAcknowledgement(t, sessionID)
		if buildErr != nil {
			s.logger.ErrorContext(ctx, "failed to build task acknowledgement blocks",
				slog.String("error", buildErr.Error()),
				slog.String("task_id", t.ID.String()),
			)
		}

		if _, sendErr := s.sender.SendMessage(ctx, messenger.SendMessageParams{
			ChannelID: event.Channel,
			Text:      ackText,
			Blocks:    ackBlocks,
		}); sendErr != nil {
			// Best-effort: log but don't fail the overall operation.
			s.logger.ErrorContext(ctx, "failed to send acknowledgement",
				slog.String("error", sendErr.Error()),
				slog.String("channel", event.Channel),
			)
		}
	}

	return nil
}

func buildTaskAcknowledgement(record *task.Task, sessionID domain.AgentSessionID) (text string, blocks []byte, err error) {
	ackText := fmt.Sprintf("Created task: *%s*", record.Title)
	description := strings.TrimSpace(record.Description)
	if sessionID != (domain.AgentSessionID{}) {
		sessionLabel := sessionID.String()[:8]
		ackText += fmt.Sprintf(" (dispatched, session %s)", sessionLabel)
		if description != "" {
			description += "\n\n"
		}
		description += "Dispatch started as session " + sessionLabel + "."
	}

	blocks, err = EncodeBlocks(BuildTaskCard(TaskCardInput{
		Title:       record.Title,
		Description: description,
		Status:      string(record.Status),
		Priority:    record.Priority,
	}))
	if err != nil {
		return ackText, nil, fmt.Errorf("encode task acknowledgement blocks: %w", err)
	}

	return ackText, blocks, nil
}

func (s *Service) promptAccountLink(ctx context.Context, event InnerEvent) error {
	channelResolver, ok := s.resolver.(channelContextResolver)
	if !ok || s.linker == nil || s.notifier == nil {
		return domain.ErrNotFound
	}

	resolved, err := channelResolver.ResolveChannelContext(ctx, event.TeamID, event.Channel)
	if err != nil {
		return err
	}

	linkURL, err := s.linker.GenerateLink(resolved.TenantID, string(messenger.PlatformSlack), event.User)
	if err != nil {
		return err
	}
	if notifyErr := s.notifier.SendNotification(ctx, messenger.NotificationParams{
		UserExternalID: event.User,
		Text:           BuildLinkingDM(linkURL),
	}); notifyErr != nil {
		return notifyErr
	}

	if s.sender != nil {
		_, _ = s.sender.SendMessage(ctx, messenger.SendMessageParams{
			ChannelID: event.Channel,
			Text:      "I sent you a DM to connect your Slack account before I can create tasks or answer HITL prompts.",
		})
	}

	return nil
}

// HandleMessage routes threaded replies to pending HITL questions when possible.
func (s *Service) HandleMessage(event InnerEvent) error {
	const op = "slack.Service.HandleMessage"
	ctx := context.Background()

	if event.ThreadTS == "" || strings.TrimSpace(event.Text) == "" {
		return nil
	}
	if s.resolver == nil || s.answerer == nil {
		return nil
	}

	resolved, err := s.resolver.ResolveContext(ctx, event.TeamID, event.Channel, event.User)
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			return nil
		}
		return fmt.Errorf("%s: resolve context: %w", op, err)
	}

	question, err := s.answerer.AnswerByThread(
		ctx,
		resolved.TenantID,
		string(messenger.PlatformSlack),
		event.ThreadTS,
		strings.TrimSpace(event.Text),
		resolved.UserID,
	)
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			return nil
		}
		if errors.Is(err, domain.ErrInvalidInput) {
			s.logger.InfoContext(ctx, "ignored ambiguous HITL thread reply",
				slog.String("thread_ts", event.ThreadTS),
				slog.String("channel", event.Channel),
			)
			return nil
		}
		return fmt.Errorf("%s: answer by thread: %w", op, err)
	}

	if s.resumer != nil {
		resumeCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()

		if err = s.resumer.SendHITLAnswer(resumeCtx, resolved.TenantID, question.AgentSessionID, strings.TrimSpace(event.Text)); err != nil {
			if errors.Is(err, domain.ErrSessionUnavailable) {
				cancel()
				s.logger.InfoContext(ctx, "thread reply recorded but session was unavailable; task requeued instead",
					slog.String("question_id", question.ID.String()),
					slog.String("session_id", question.AgentSessionID.String()),
				)
			} else {
				if !errors.Is(err, domain.ErrSessionUnavailable) {
					rollbackCtx, rollbackCancel := context.WithTimeout(context.Background(), 15*time.Second)
					defer rollbackCancel()
					if resetErr := s.answerer.ResetQuestion(rollbackCtx, resolved.TenantID, question.ID); resetErr != nil {
						return fmt.Errorf("%s: resume session: %w (rollback failed: %w)", op, err, resetErr)
					}
				}
				return fmt.Errorf("%s: resume session: %w", op, err)
			}
		}
	}

	s.logger.InfoContext(ctx, "thread reply routed to HITL question",
		slog.String("thread_ts", event.ThreadTS),
		slog.String("question_id", question.ID.String()),
		slog.String("session_id", question.AgentSessionID.String()),
	)

	return nil
}
