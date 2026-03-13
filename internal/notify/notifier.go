package notify

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"

	"github.com/gosuda/steerlane/internal/messenger"
	slackmsg "github.com/gosuda/steerlane/internal/messenger/slack"
)

// Notifier formats human-readable notifications and delivers them through a
// messenger.Messenger.
type Notifier struct {
	logger        *slog.Logger
	messenger     messenger.Messenger
	emailFallback EmailSender
}

// TaskCompletedPayload describes a completed task notification.
type TaskCompletedPayload struct {
	UserExternalID string
	FallbackEmail  string
	TaskID         string
	TaskTitle      string
	SessionID      string
}

// SessionFailedPayload describes a failed session notification.
type SessionFailedPayload struct {
	UserExternalID string
	FallbackEmail  string
	TaskID         string
	TaskTitle      string
	SessionID      string
	Reason         string
}

// HITLEscalatedPayload describes a HITL escalation notification (tier 1 timeout).
type HITLEscalatedPayload struct {
	UserExternalID string
	FallbackEmail  string
	TaskID         string
	TaskTitle      string
	SessionID      string
	Question       string
}

// HITLTimedOutPayload describes a HITL timeout notification.
type HITLTimedOutPayload struct {
	UserExternalID string
	FallbackEmail  string
	TaskID         string
	TaskTitle      string
	SessionID      string
	Question       string
}

// New constructs a Notifier.
func New(logger *slog.Logger, msg messenger.Messenger) *Notifier {
	return NewWithEmail(logger, msg, nil)
}

// NewWithEmail constructs a Notifier with an optional email fallback sender.
func NewWithEmail(logger *slog.Logger, msg messenger.Messenger, emailFallback EmailSender) *Notifier {
	if logger == nil {
		logger = slog.Default()
	}

	return &Notifier{
		logger:        logger.With("component", "notify.notifier"),
		messenger:     msg,
		emailFallback: emailFallback,
	}
}

// SendTaskCompleted sends a task completion notification.
func (n *Notifier) SendTaskCompleted(ctx context.Context, payload TaskCompletedPayload) error {
	const op = "notify.Notifier.SendTaskCompleted"

	text := formatTaskCompleted(payload)
	blocks, err := n.buildTaskCompletedBlocks(payload)
	if err != nil {
		return fmt.Errorf("%s: build slack blocks: %w", op, err)
	}

	if err = n.send(ctx, messenger.NotificationParams{
		UserExternalID: payload.UserExternalID,
		Text:           text,
		Blocks:         blocks,
	}, payload.FallbackEmail, "Task completed", text); err != nil {
		return fmt.Errorf("%s: %w", op, err)
	}

	n.logger.InfoContext(ctx, "sent task completed notification",
		slog.String("task_id", payload.TaskID),
		slog.String("session_id", payload.SessionID),
		slog.String("user_external_id", payload.UserExternalID),
	)

	return nil
}

// SendSessionFailed sends a session failure notification.
func (n *Notifier) SendSessionFailed(ctx context.Context, payload SessionFailedPayload) error {
	const op = "notify.Notifier.SendSessionFailed"

	text := formatSessionFailed(payload)
	blocks, err := n.buildSessionFailedBlocks(payload)
	if err != nil {
		return fmt.Errorf("%s: build slack blocks: %w", op, err)
	}

	if err = n.send(ctx, messenger.NotificationParams{
		UserExternalID: payload.UserExternalID,
		Text:           text,
		Blocks:         blocks,
	}, payload.FallbackEmail, "Session failed", text); err != nil {
		return fmt.Errorf("%s: %w", op, err)
	}

	n.logger.InfoContext(ctx, "sent session failed notification",
		slog.String("task_id", payload.TaskID),
		slog.String("session_id", payload.SessionID),
		slog.String("user_external_id", payload.UserExternalID),
	)

	return nil
}

// SendHITLEscalated sends an escalation notification (tier 1 timeout — question is still active).
func (n *Notifier) SendHITLEscalated(ctx context.Context, payload HITLEscalatedPayload) error {
	const op = "notify.Notifier.SendHITLEscalated"

	text := formatHITLEscalated(payload)
	blocks, err := n.buildHITLEscalatedBlocks(payload)
	if err != nil {
		return fmt.Errorf("%s: build slack blocks: %w", op, err)
	}

	if err = n.send(ctx, messenger.NotificationParams{
		UserExternalID: payload.UserExternalID,
		Text:           text,
		Blocks:         blocks,
	}, payload.FallbackEmail, "Human input escalated", text); err != nil {
		return fmt.Errorf("%s: %w", op, err)
	}

	n.logger.InfoContext(ctx, "sent hitl escalated notification",
		slog.String("task_id", payload.TaskID),
		slog.String("session_id", payload.SessionID),
		slog.String("user_external_id", payload.UserExternalID),
	)

	return nil
}

// SendHITLTimedOut sends a HITL timeout notification.
func (n *Notifier) SendHITLTimedOut(ctx context.Context, payload HITLTimedOutPayload) error {
	const op = "notify.Notifier.SendHITLTimedOut"

	text := formatHITLTimedOut(payload)
	blocks, err := n.buildHITLTimedOutBlocks(payload)
	if err != nil {
		return fmt.Errorf("%s: build slack blocks: %w", op, err)
	}

	if err = n.send(ctx, messenger.NotificationParams{
		UserExternalID: payload.UserExternalID,
		Text:           text,
		Blocks:         blocks,
	}, payload.FallbackEmail, "Human input timed out", text); err != nil {
		return fmt.Errorf("%s: %w", op, err)
	}

	n.logger.InfoContext(ctx, "sent hitl timed out notification",
		slog.String("task_id", payload.TaskID),
		slog.String("session_id", payload.SessionID),
		slog.String("user_external_id", payload.UserExternalID),
	)

	return nil
}

func (n *Notifier) send(ctx context.Context, params messenger.NotificationParams, fallbackEmail, subject, body string) error {
	const op = "notify.Notifier.send"

	var messengerErr error
	if n.messenger != nil {
		if err := n.messenger.SendNotification(ctx, params); err == nil {
			return nil
		} else {
			messengerErr = fmt.Errorf("%s: send notification: %w", op, err)
		}
	} else {
		messengerErr = fmt.Errorf("%s: messenger not configured", op)
	}

	if n.emailFallback != nil && fallbackEmail != "" {
		if err := n.emailFallback.Send(ctx, EmailPayload{To: fallbackEmail, Subject: subject, Body: body}); err == nil {
			n.logger.WarnContext(ctx, "delivered notification through email fallback",
				slog.String("fallback_email", fallbackEmail),
				slog.String("subject", subject),
				slog.String("messenger_user_external_id", params.UserExternalID),
			)
			return nil
		} else {
			return errors.Join(messengerErr, fmt.Errorf("%s: send fallback email: %w", op, err))
		}
	}

	return messengerErr
}

func (n *Notifier) buildTaskCompletedBlocks(payload TaskCompletedPayload) ([]byte, error) {
	if !n.shouldSendSlackBlocks() {
		return nil, nil
	}

	description := "Task completed successfully."
	if payload.SessionID != "" {
		description = "Session " + payload.SessionID + " finished successfully."
	}

	return slackmsg.EncodeBlocks(slackmsg.BuildTaskCard(slackmsg.TaskCardInput{
		Title:       describeTask(payload.TaskTitle, payload.TaskID),
		Description: description,
		Status:      "done",
		Priority:    -1,
	}))
}

func (n *Notifier) buildSessionFailedBlocks(payload SessionFailedPayload) ([]byte, error) {
	if !n.shouldSendSlackBlocks() {
		return nil, nil
	}

	return slackmsg.EncodeBlocks(slackmsg.BuildSessionStatusCard(slackmsg.SessionStatusInput{
		Status:    "failed",
		TaskTitle: describeTask(payload.TaskTitle, payload.TaskID),
		Detail:    notificationDetail(payload.SessionID, payload.Reason, "ended with an error.", "Reason: "),
	}))
}

func (n *Notifier) buildHITLEscalatedBlocks(payload HITLEscalatedPayload) ([]byte, error) {
	if !n.shouldSendSlackBlocks() {
		return nil, nil
	}

	return slackmsg.EncodeBlocks(slackmsg.BuildSessionStatusCard(slackmsg.SessionStatusInput{
		Status:    "waiting_hitl",
		TaskTitle: describeTask(payload.TaskTitle, payload.TaskID),
		Detail:    notificationDetail(payload.SessionID, payload.Question, "is waiting for human input (escalated).", "Unanswered question: "),
	}))
}

func (n *Notifier) buildHITLTimedOutBlocks(payload HITLTimedOutPayload) ([]byte, error) {
	if !n.shouldSendSlackBlocks() {
		return nil, nil
	}

	return slackmsg.EncodeBlocks(slackmsg.BuildSessionStatusCard(slackmsg.SessionStatusInput{
		Status:    "waiting_hitl",
		TaskTitle: describeTask(payload.TaskTitle, payload.TaskID),
		Detail:    notificationDetail(payload.SessionID, payload.Question, "is waiting for follow-up.", "Unanswered question: "),
	}))
}

func (n *Notifier) shouldSendSlackBlocks() bool {
	return n.messenger != nil && n.messenger.Platform() == messenger.PlatformSlack
}

func notificationDetail(sessionID, message, sessionSuffix, messagePrefix string) string {
	parts := make([]string, 0, 2)
	if sessionID != "" {
		parts = append(parts, "Session "+sessionID+" "+sessionSuffix)
	}
	if message != "" {
		parts = append(parts, messagePrefix+message)
	}
	return strings.Join(parts, "\n")
}

func formatTaskCompleted(payload TaskCompletedPayload) string {
	parts := []string{"Task completed:"}
	parts = append(parts, describeTask(payload.TaskTitle, payload.TaskID)+".")

	if payload.SessionID != "" {
		parts = append(parts, "Session "+payload.SessionID+" finished successfully.")
	}

	return strings.Join(parts, " ")
}

func formatSessionFailed(payload SessionFailedPayload) string {
	parts := []string{"Session failed for", describeTask(payload.TaskTitle, payload.TaskID) + "."}

	if payload.SessionID != "" {
		parts = append(parts, "Session "+payload.SessionID+" ended with an error.")
	}

	if payload.Reason != "" {
		parts = append(parts, "Reason: "+payload.Reason)
	}

	return strings.Join(parts, " ")
}

func formatHITLEscalated(payload HITLEscalatedPayload) string {
	parts := []string{"Human input escalated for", describeTask(payload.TaskTitle, payload.TaskID) + "."}

	if payload.SessionID != "" {
		parts = append(parts, "Session "+payload.SessionID+" is still waiting for human input.")
	}

	if payload.Question != "" {
		parts = append(parts, "Unanswered question: "+payload.Question)
	}

	return strings.Join(parts, " ")
}

func formatHITLTimedOut(payload HITLTimedOutPayload) string {
	parts := []string{"Human input timed out for", describeTask(payload.TaskTitle, payload.TaskID) + "."}

	if payload.SessionID != "" {
		parts = append(parts, "Session "+payload.SessionID+" is waiting for follow-up.")
	}

	if payload.Question != "" {
		parts = append(parts, "Unanswered question: "+payload.Question)
	}

	return strings.Join(parts, " ")
}

func describeTask(title, id string) string {
	switch {
	case title != "" && id != "":
		return fmt.Sprintf("%s (%s)", title, id)
	case title != "":
		return title
	case id != "":
		return "task " + id
	default:
		return "the task"
	}
}
