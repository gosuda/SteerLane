package notify

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/gosuda/steerlane/internal/domain"
	"github.com/gosuda/steerlane/internal/domain/adr"
	agentdom "github.com/gosuda/steerlane/internal/domain/agent"
	"github.com/gosuda/steerlane/internal/domain/hitl"
	"github.com/gosuda/steerlane/internal/domain/task"
	userdom "github.com/gosuda/steerlane/internal/domain/user"
	"github.com/gosuda/steerlane/internal/messenger"
	slackmsg "github.com/gosuda/steerlane/internal/messenger/slack"
	"github.com/gosuda/steerlane/internal/observability"
	"github.com/gosuda/steerlane/internal/store/postgres/sqlc"
)

type linkQuery interface {
	ListMessengerLinksByUser(ctx context.Context, arg sqlc.ListMessengerLinksByUserParams) ([]sqlc.UserMessengerLink, error)
}

type threadMessenger interface {
	CreateThread(ctx context.Context, params messenger.CreateThreadParams) (messenger.MessageResult, error)
	Platform() messenger.Platform
}

type sessionContextReader interface {
	Get(tenantID domain.TenantID, sessionID domain.AgentSessionID) (messenger.SessionContext, bool)
}

type userEmailLookup interface {
	GetByID(ctx context.Context, tenantID domain.TenantID, id domain.UserID) (*userdom.User, error)
}

type taskRecipients struct { //nolint:govet // readability over field packing
	task          *task.Task
	messengerIDs  []string
	fallbackEmail string
}

// Dispatcher resolves internal users to messenger identities and emits runtime notifications.
type Dispatcher struct {
	threader  threadMessenger
	notifier  *Notifier
	logger    *slog.Logger
	links     linkQuery
	tasks     task.Repository
	sessions  agentdom.Repository
	questions hitl.Repository
	users     userEmailLookup
	contexts  sessionContextReader
	platform  messenger.Platform
}

// NewDispatcher constructs a Dispatcher for a single messenger platform.
func NewDispatcher(
	logger *slog.Logger,
	platform messenger.Platform,
	notifier *Notifier,
	threader threadMessenger,
	contexts sessionContextReader,
	links linkQuery,
	tasks task.Repository,
	sessions agentdom.Repository,
	questions hitl.Repository,
	users userEmailLookup,
) *Dispatcher {
	if logger == nil {
		logger = slog.Default()
	}

	return &Dispatcher{
		logger:    logger.With("component", "notify.dispatcher"),
		platform:  platform,
		notifier:  notifier,
		threader:  threader,
		contexts:  contexts,
		links:     links,
		tasks:     tasks,
		sessions:  sessions,
		questions: questions,
		users:     users,
	}
}

// NotifyADRCreated posts a runtime ADR summary into the originating messenger thread when available.
func (d *Dispatcher) NotifyADRCreated(ctx context.Context, tenantID domain.TenantID, sessionID domain.AgentSessionID, record *adr.ADR) error {
	const op = "notify.Dispatcher.NotifyADRCreated"

	if record == nil || d.threader == nil || d.contexts == nil {
		return nil
	}
	if d.threader.Platform() != d.platform {
		return nil
	}

	target, ok := d.contexts.Get(tenantID, sessionID)
	if !ok || target.Platform != d.platform || target.ChannelID == "" || target.ParentMessageID == "" {
		return nil
	}

	text := formatADRCreated(record)
	blocks, err := d.buildADRCreatedBlocks(record)
	if err != nil {
		return fmt.Errorf("%s: build slack blocks: %w", op, err)
	}

	_, err = d.threader.CreateThread(ctx, messenger.CreateThreadParams{
		ChannelID:       target.ChannelID,
		ParentMessageID: target.ParentMessageID,
		Text:            text,
		Blocks:          blocks,
	})
	if err != nil {
		observability.RecordNotification("adr_created", "error")
		return fmt.Errorf("%s: create thread: %w", op, err)
	}
	observability.RecordNotification("adr_created", "success")

	return nil
}

// NotifyTaskCompleted sends a completion notification to the task assignee.
func (d *Dispatcher) NotifyTaskCompleted(
	ctx context.Context,
	tenantID domain.TenantID,
	taskID domain.TaskID,
	sessionID domain.AgentSessionID,
) error {
	const op = "notify.Dispatcher.NotifyTaskCompleted"

	resolved, err := d.resolveTaskRecipients(ctx, tenantID, taskID)
	if err != nil {
		return fmt.Errorf("%s: %w", op, err)
	}

	var joined error
	for _, recipient := range resolved.messengerIDs {
		if sendErr := d.notifier.SendTaskCompleted(ctx, TaskCompletedPayload{
			UserExternalID: recipient,
			FallbackEmail:  resolved.fallbackEmail,
			TaskID:         taskID.String(),
			TaskTitle:      resolved.task.Title,
			SessionID:      sessionID.String(),
		}); sendErr != nil {
			observability.RecordNotification("task_completed", "error")
			joined = errors.Join(joined, sendErr)
			continue
		}
		observability.RecordNotification("task_completed", "success")
	}
	return joined
}

// NotifySessionFailed sends a failure notification to the task assignee.
func (d *Dispatcher) NotifySessionFailed(
	ctx context.Context,
	tenantID domain.TenantID,
	taskID domain.TaskID,
	sessionID domain.AgentSessionID,
	reason string,
) error {
	const op = "notify.Dispatcher.NotifySessionFailed"

	resolved, err := d.resolveTaskRecipients(ctx, tenantID, taskID)
	if err != nil {
		return fmt.Errorf("%s: %w", op, err)
	}

	var joined error
	for _, recipient := range resolved.messengerIDs {
		if sendErr := d.notifier.SendSessionFailed(ctx, SessionFailedPayload{
			UserExternalID: recipient,
			FallbackEmail:  resolved.fallbackEmail,
			TaskID:         taskID.String(),
			TaskTitle:      resolved.task.Title,
			SessionID:      sessionID.String(),
			Reason:         reason,
		}); sendErr != nil {
			observability.RecordNotification("session_failed", "error")
			joined = errors.Join(joined, sendErr)
			continue
		}
		observability.RecordNotification("session_failed", "success")
	}
	return joined
}

// EscalateQuestion transitions a pending question to escalated status and extends its timeout.
func (d *Dispatcher) EscalateQuestion(ctx context.Context, tenantID domain.TenantID, questionID domain.HITLQuestionID, newTimeoutAt time.Time) error {
	if d.questions == nil {
		return nil
	}
	return d.questions.Escalate(ctx, tenantID, questionID, newTimeoutAt)
}

// ListEscalatedExpiredQuestions returns escalated questions whose extended timeout has expired.
func (d *Dispatcher) ListEscalatedExpiredQuestions(ctx context.Context, before time.Time, limit int) ([]*hitl.Question, error) {
	if d.questions == nil {
		return nil, nil
	}
	return d.questions.ListEscalatedExpiredBefore(ctx, before, limit)
}

// MarkEscalatedTimedOut finalizes timeout state for an escalated question (tier 2 timeout).
func (d *Dispatcher) MarkEscalatedTimedOut(ctx context.Context, tenantID domain.TenantID, questionID domain.HITLQuestionID) error {
	if d.questions == nil {
		return nil
	}
	return d.questions.MarkTimedOutEscalated(ctx, tenantID, questionID)
}

// NotifyQuestionEscalated sends best-effort escalation notifications for a question (tier 1 timeout).
func (d *Dispatcher) NotifyQuestionEscalated(ctx context.Context, question *hitl.Question) error {
	if d.notifier == nil || question == nil {
		return nil
	}

	session, err := d.sessions.GetByID(ctx, question.TenantID, question.AgentSessionID)
	if err != nil {
		return fmt.Errorf("load session %s: %w", question.AgentSessionID, err)
	}

	resolved, err := d.resolveTaskRecipients(ctx, question.TenantID, session.TaskID)
	if err != nil {
		return err
	}

	var joined error
	for _, recipient := range resolved.messengerIDs {
		if sendErr := d.notifier.SendHITLEscalated(ctx, HITLEscalatedPayload{
			UserExternalID: recipient,
			FallbackEmail:  resolved.fallbackEmail,
			TaskID:         resolved.task.ID.String(),
			TaskTitle:      resolved.task.Title,
			SessionID:      question.AgentSessionID.String(),
			Question:       question.Question,
		}); sendErr != nil {
			observability.RecordNotification("hitl_escalated", "error")
			joined = errors.Join(joined, sendErr)
			continue
		}
		observability.RecordNotification("hitl_escalated", "success")
	}
	return joined
}

// ListExpiredPendingQuestions returns expired pending questions without mutating state.
func (d *Dispatcher) ListExpiredPendingQuestions(ctx context.Context, before time.Time, limit int) ([]*hitl.Question, error) {
	if d.questions == nil {
		return nil, nil
	}
	return d.questions.ListExpiredPendingBefore(ctx, before, limit)
}

// ListSessionQuestions returns all HITL questions for a session.
func (d *Dispatcher) ListSessionQuestions(ctx context.Context, tenantID domain.TenantID, sessionID domain.AgentSessionID) ([]*hitl.Question, error) {
	if d.questions == nil {
		return nil, nil
	}
	return d.questions.ListBySession(ctx, tenantID, sessionID)
}

// ListTimedOutQuestions returns already-timed-out questions that may still need session finalization.
func (d *Dispatcher) ListTimedOutQuestions(ctx context.Context, limit int) ([]*hitl.Question, error) {
	if d.questions == nil {
		return nil, nil
	}
	return d.questions.ListTimedOut(ctx, limit)
}

// ListUnnotifiedTimedOutQuestions returns timed-out questions that still need a notification attempt.
func (d *Dispatcher) ListUnnotifiedTimedOutQuestions(ctx context.Context, limit int) ([]*hitl.Question, error) {
	if d.questions == nil {
		return nil, nil
	}
	return d.questions.ListUnnotifiedTimedOut(ctx, limit)
}

// MarkQuestionTimedOut finalizes timeout state for a question after cancellation succeeds.
func (d *Dispatcher) MarkQuestionTimedOut(ctx context.Context, tenantID domain.TenantID, questionID domain.HITLQuestionID) error {
	if d.questions == nil {
		return nil
	}
	return d.questions.MarkTimedOut(ctx, tenantID, questionID)
}

// ReopenTimedOutQuestion reverts a timed-out question back to pending when cancellation fails.
func (d *Dispatcher) ReopenTimedOutQuestion(ctx context.Context, tenantID domain.TenantID, questionID domain.HITLQuestionID) error {
	if d.questions == nil {
		return nil
	}
	return d.questions.ReopenTimedOut(ctx, tenantID, questionID)
}

// ClaimTimeoutNotification atomically reserves a timed-out question for notification delivery.
func (d *Dispatcher) ClaimTimeoutNotification(ctx context.Context, tenantID domain.TenantID, questionID domain.HITLQuestionID) (*hitl.Question, error) {
	if d.questions == nil {
		return nil, nil
	}
	return d.questions.ClaimTimeoutNotification(ctx, tenantID, questionID)
}

// ClearTimeoutNotificationClaim releases a failed notification reservation so a later sweep can retry.
func (d *Dispatcher) ClearTimeoutNotificationClaim(ctx context.Context, tenantID domain.TenantID, questionID domain.HITLQuestionID) error {
	if d.questions == nil {
		return nil
	}
	return d.questions.ClearTimeoutNotificationClaim(ctx, tenantID, questionID)
}

// MarkTimeoutNotificationSent records successful timeout notification delivery.
func (d *Dispatcher) MarkTimeoutNotificationSent(ctx context.Context, tenantID domain.TenantID, questionID domain.HITLQuestionID) error {
	if d.questions == nil {
		return nil
	}
	return d.questions.MarkTimeoutNotificationSent(ctx, tenantID, questionID)
}

// NotifyQuestionTimedOut sends best-effort timeout notifications for a question.
func (d *Dispatcher) NotifyQuestionTimedOut(ctx context.Context, question *hitl.Question) error {
	if d.notifier == nil || question == nil {
		return nil
	}

	session, err := d.sessions.GetByID(ctx, question.TenantID, question.AgentSessionID)
	if err != nil {
		return fmt.Errorf("load session %s: %w", question.AgentSessionID, err)
	}

	resolved, err := d.resolveTaskRecipients(ctx, question.TenantID, session.TaskID)
	if err != nil {
		return err
	}

	var joined error
	for _, recipient := range resolved.messengerIDs {
		if sendErr := d.notifier.SendHITLTimedOut(ctx, HITLTimedOutPayload{
			UserExternalID: recipient,
			FallbackEmail:  resolved.fallbackEmail,
			TaskID:         resolved.task.ID.String(),
			TaskTitle:      resolved.task.Title,
			SessionID:      question.AgentSessionID.String(),
			Question:       question.Question,
		}); sendErr != nil {
			observability.RecordNotification("hitl_timed_out", "error")
			joined = errors.Join(joined, sendErr)
			continue
		}
		observability.RecordNotification("hitl_timed_out", "success")
	}
	return joined
}

func (d *Dispatcher) resolveTaskRecipients(
	ctx context.Context,
	tenantID domain.TenantID,
	taskID domain.TaskID,
) (*taskRecipients, error) {
	if d.tasks == nil || d.links == nil || d.notifier == nil {
		return nil, nil
	}

	tk, err := d.tasks.GetByID(ctx, tenantID, taskID)
	if err != nil {
		return nil, fmt.Errorf("load task %s: %w", taskID, err)
	}
	if tk.AssignedTo == nil {
		return &taskRecipients{task: tk}, nil
	}

	links, err := d.links.ListMessengerLinksByUser(ctx, sqlc.ListMessengerLinksByUserParams{
		UserID:   *tk.AssignedTo,
		TenantID: tenantID,
	})
	if err != nil {
		return nil, fmt.Errorf("load messenger links for task %s: %w", taskID, err)
	}

	recipients := make([]string, 0, len(links))
	seen := make(map[string]struct{}, len(links))
	for _, link := range links {
		if link.Platform != string(d.platform) {
			continue
		}
		if _, ok := seen[link.ExternalID]; ok {
			continue
		}
		seen[link.ExternalID] = struct{}{}
		recipients = append(recipients, link.ExternalID)
	}

	resolved := &taskRecipients{task: tk, messengerIDs: recipients}
	if d.users != nil {
		u, userErr := d.users.GetByID(ctx, tenantID, *tk.AssignedTo)
		if userErr != nil {
			return nil, fmt.Errorf("load fallback email for task %s: %w", taskID, userErr)
		}
		if u != nil && u.Email != nil {
			resolved.fallbackEmail = *u.Email
		}
	}

	return resolved, nil
}

func (d *Dispatcher) buildADRCreatedBlocks(record *adr.ADR) ([]byte, error) {
	if d.platform != messenger.PlatformSlack || record == nil {
		return nil, nil
	}

	return slackmsg.EncodeBlocks(slackmsg.BuildADRSummaryCard(slackmsg.ADRCardInput{
		Title:       record.Title,
		Status:      string(record.Status),
		CreatedDate: record.CreatedAt.UTC().Format(time.DateOnly),
		Decision:    record.Decision,
		Sequence:    record.Sequence,
		Consequences: &struct {
			Good    []string
			Bad     []string
			Neutral []string
		}{
			Good:    record.Consequences.Good,
			Bad:     record.Consequences.Bad,
			Neutral: record.Consequences.Neutral,
		},
	}))
}

func formatADRCreated(record *adr.ADR) string {
	if record == nil {
		return "ADR created."
	}

	label := fmt.Sprintf("ADR-%d", record.Sequence)
	if record.Title != "" {
		return fmt.Sprintf("%s created: %s", label, record.Title)
	}
	return label + " created."
}
