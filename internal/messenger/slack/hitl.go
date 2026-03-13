package slack

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/gosuda/steerlane/internal/domain"
	"github.com/gosuda/steerlane/internal/domain/hitl"
	"github.com/gosuda/steerlane/internal/messenger"
)

// ---------------------------------------------------------------------------
// Action value format
// ---------------------------------------------------------------------------
//
// Interactive buttons/menus for HITL questions encode routing metadata in
// the action value string using a colon-separated format:
//
//   steerlane:hitl:<question_id>:<session_id>
//
// Example:
//   steerlane:hitl:550e8400-e29b-41d4-a716-446655440000:660e8400-e29b-41d4-a716-446655440001
//
// The selected answer text is carried in either:
//   - InteractionAction.Value (for buttons with a label-as-value)
//   - InteractionAction.SelectedOption.Value (for menus/radio groups)
//
// For buttons, the action_id carries the routing prefix and the value
// carries the human-readable answer.

const (
	// ActionPrefix identifies SteerLane HITL actions in Slack interactive payloads.
	ActionPrefix = "steerlane:hitl"

	// actionParts is the expected number of colon-separated segments.
	actionParts = 4
)

// HITLAction holds parsed HITL routing metadata from a Slack interactive action.
type HITLAction struct {
	Answer     string
	QuestionID domain.HITLQuestionID
	SessionID  domain.AgentSessionID
}

// ParseHITLAction parses an action_id with the steerlane:hitl:... format
// and extracts the question/session IDs. The answer text is taken from
// answerValue (the button value or selected option value).
//
// Returns an error if the format is invalid or the UUIDs cannot be parsed.
func ParseHITLAction(actionID, answerValue string) (HITLAction, error) {
	const op = "slack.ParseHITLAction"

	parts := strings.SplitN(actionID, ":", actionParts)
	if len(parts) != actionParts {
		return HITLAction{}, fmt.Errorf("%s: expected %d parts, got %d: %w",
			op, actionParts, len(parts), domain.ErrInvalidInput)
	}

	if parts[0]+":"+parts[1] != ActionPrefix {
		return HITLAction{}, fmt.Errorf("%s: unknown prefix %q: %w",
			op, parts[0]+":"+parts[1], domain.ErrInvalidInput)
	}

	questionID, err := uuid.Parse(parts[2])
	if err != nil {
		return HITLAction{}, fmt.Errorf("%s: invalid question_id %q: %w", op, parts[2], domain.ErrInvalidInput)
	}

	sessionPart, _, _ := strings.Cut(parts[3], "#")
	sessionID, err := uuid.Parse(sessionPart)
	if err != nil {
		return HITLAction{}, fmt.Errorf("%s: invalid session_id %q: %w", op, sessionPart, domain.ErrInvalidInput)
	}

	if answerValue == "" {
		return HITLAction{}, fmt.Errorf("%s: answer value is empty: %w", op, domain.ErrInvalidInput)
	}

	return HITLAction{
		QuestionID: questionID,
		SessionID:  sessionID,
		Answer:     answerValue,
	}, nil
}

// FormatActionID builds an action_id string for use in Slack Block Kit
// interactive elements (buttons, menus).
func FormatActionID(questionID domain.HITLQuestionID, sessionID domain.AgentSessionID) string {
	return fmt.Sprintf("%s:%s:%s", ActionPrefix, questionID.String(), sessionID.String())
}

// ---------------------------------------------------------------------------
// HITL interaction handler
// ---------------------------------------------------------------------------

// HITLAnswerer routes answered HITL questions to the domain layer.
// Satisfied by hitlrouter.Router.AnswerQuestion.
type HITLAnswerer interface {
	AnswerQuestion(
		ctx context.Context,
		tenantID domain.TenantID,
		questionID domain.HITLQuestionID,
		answer string,
		answeredBy domain.UserID,
	) error
	GetQuestion(ctx context.Context, tenantID domain.TenantID, questionID domain.HITLQuestionID) (*hitl.Question, error)
	ResetQuestion(ctx context.Context, tenantID domain.TenantID, questionID domain.HITLQuestionID) error
}

type HITLResumer interface {
	SendHITLAnswer(ctx context.Context, tenantID domain.TenantID, sessionID domain.AgentSessionID, answer string) error
}

// HITLInteractionHandler processes HITL-related Slack interactive payloads.
// It implements the InteractionHandler interface.
type HITLInteractionHandler struct {
	logger   *slog.Logger
	answerer HITLAnswerer
	resumer  HITLResumer
	resolver ContextResolver
	sender   MessageSender
}

// Compile-time check.
var _ InteractionHandler = (*HITLInteractionHandler)(nil)

// NewHITLInteractionHandler creates a handler for HITL interactive actions.
func NewHITLInteractionHandler(
	logger *slog.Logger,
	answerer HITLAnswerer,
	resumer HITLResumer,
	resolver ContextResolver,
	sender MessageSender,
) *HITLInteractionHandler {
	return &HITLInteractionHandler{
		logger:   logger.With("component", "slack.hitl"),
		answerer: answerer,
		resumer:  resumer,
		resolver: resolver,
		sender:   sender,
	}
}

// HandleInteraction processes a Slack interactive payload that contains
// HITL answer actions. It parses the action value, resolves the tenant
// context, and routes the answer through the HITLAnswerer.
func (h *HITLInteractionHandler) HandleInteraction(ctx context.Context, payload InteractionPayload) error {
	const op = "slack.HITLInteractionHandler.HandleInteraction"

	for _, action := range payload.Actions {
		if !strings.HasPrefix(action.ActionID, ActionPrefix) {
			continue
		}

		// Extract answer text from value or selected option.
		answerText := action.Value
		if answerText == "" && action.SelectedOption != nil {
			answerText = action.SelectedOption.Value
		}

		parsed, err := ParseHITLAction(action.ActionID, answerText)
		if err != nil {
			h.logger.WarnContext(ctx, "invalid HITL action",
				slog.String("error", err.Error()),
				slog.String("action_id", action.ActionID),
				slog.String("user_id", payload.User.ID),
			)
			continue
		}

		// Resolve Slack user to SteerLane tenant + user context.
		channelID := ""
		if payload.Channel != nil {
			channelID = payload.Channel.ID
		}

		resolved, err := h.resolver.ResolveContext(ctx, payload.Team.ID, channelID, payload.User.ID)
		if err != nil {
			h.logger.ErrorContext(ctx, "context resolution failed for HITL answer",
				slog.String("error", err.Error()),
				slog.String("team_id", payload.Team.ID),
				slog.String("channel_id", channelID),
			)
			return fmt.Errorf("%s: resolve context: %w", op, err)
		}

		// Route the answer through the domain layer.
		if err = h.answerer.AnswerQuestion(ctx, resolved.TenantID, parsed.QuestionID, parsed.Answer, resolved.UserID); err != nil {
			if errors.Is(err, domain.ErrInvalidInput) || errors.Is(err, domain.ErrInvalidTransition) || errors.Is(err, domain.ErrNotFound) {
				h.logger.InfoContext(ctx, "stale HITL interaction ignored",
					slog.String("question_id", parsed.QuestionID.String()),
					slog.String("session_id", parsed.SessionID.String()),
					"error", err,
				)
				continue
			}
			h.logger.ErrorContext(ctx, "failed to route HITL answer",
				slog.String("error", err.Error()),
				slog.String("question_id", parsed.QuestionID.String()),
				slog.String("session_id", parsed.SessionID.String()),
			)
			return fmt.Errorf("%s: answer question: %w", op, err)
		}

		if h.resumer != nil {
			resumeCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
			question, getErr := h.answerer.GetQuestion(resumeCtx, resolved.TenantID, parsed.QuestionID)
			if getErr != nil {
				rollbackCtx, rollbackCancel := context.WithTimeout(context.Background(), 15*time.Second)
				if resetErr := h.answerer.ResetQuestion(rollbackCtx, resolved.TenantID, parsed.QuestionID); resetErr != nil {
					rollbackCancel()
					cancel()
					return fmt.Errorf("%s: get question: %w (rollback failed: %w)", op, getErr, resetErr)
				}
				rollbackCancel()
				cancel()
				return fmt.Errorf("%s: get question: %w", op, getErr)
			}

			if err = h.resumer.SendHITLAnswer(resumeCtx, resolved.TenantID, question.AgentSessionID, parsed.Answer); err != nil {
				if errors.Is(err, domain.ErrSessionUnavailable) {
					cancel()
					h.logger.InfoContext(ctx, "HITL answer recorded but session was unavailable; requeued task instead",
						slog.String("question_id", parsed.QuestionID.String()),
						slog.String("session_id", question.AgentSessionID.String()),
					)
					break
				}
				if !errors.Is(err, domain.ErrSessionUnavailable) {
					rollbackCtx, rollbackCancel := context.WithTimeout(context.Background(), 15*time.Second)
					if resetErr := h.answerer.ResetQuestion(rollbackCtx, resolved.TenantID, parsed.QuestionID); resetErr != nil {
						rollbackCancel()
						cancel()
						return fmt.Errorf("%s: resume session: %w (rollback failed: %w)", op, err, resetErr)
					}
					rollbackCancel()
				}
				cancel()
				return fmt.Errorf("%s: resume session: %w", op, err)
			}
			cancel()
		}

		h.logger.InfoContext(ctx, "HITL answer routed",
			slog.String("question_id", parsed.QuestionID.String()),
			slog.String("session_id", parsed.SessionID.String()),
			slog.String("answered_by", resolved.UserID.String()),
		)

		// Best-effort acknowledgement in channel.
		// TODO(1C.3): Update the original message to show the answer
		// inline and disable the buttons, using payload.ResponseURL.
		if h.sender != nil && channelID != "" {
			ackText := "Answer recorded for question " + parsed.QuestionID.String()[:8]
			if _, sendErr := h.sender.SendMessage(ctx, messenger.SendMessageParams{
				ChannelID: channelID,
				Text:      ackText,
			}); sendErr != nil {
				h.logger.ErrorContext(ctx, "failed to send HITL ack",
					slog.String("error", sendErr.Error()),
				)
			}
		}
	}

	return nil
}
