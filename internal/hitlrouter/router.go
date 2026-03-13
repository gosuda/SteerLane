package hitlrouter

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/gosuda/steerlane/internal/domain"
	agentdom "github.com/gosuda/steerlane/internal/domain/agent"
	"github.com/gosuda/steerlane/internal/domain/hitl"
	"github.com/gosuda/steerlane/internal/messenger"
	redispkg "github.com/gosuda/steerlane/internal/store/redis"
)

type agentEventPublisher interface {
	PublishAgentEvent(ctx context.Context, sessionID string, eventType redispkg.EventType, payload any) error
}

type threadCreator interface {
	CreateThread(ctx context.Context, params messenger.CreateThreadParams) (messenger.MessageResult, error)
	Platform() messenger.Platform
}

type sessionContextLookup interface {
	Get(tenantID domain.TenantID, sessionID domain.AgentSessionID) (messenger.SessionContext, bool)
	Delete(tenantID domain.TenantID, sessionID domain.AgentSessionID)
}

const (
	// DefaultTimeout is the time before a HITL question is considered timed out.
	DefaultTimeout = 30 * time.Minute
)

// Router manages routing of HITL questions between agents and humans.
type Router struct {
	logger          *slog.Logger
	questions       hitl.Repository
	sessions        agentdom.Repository
	pubsub          agentEventPublisher
	threadCreators  map[messenger.Platform]threadCreator
	sessionContexts sessionContextLookup
}

// NewRouter creates a new Router with the given dependencies.
func NewRouter(
	logger *slog.Logger,
	questions hitl.Repository,
	sessions agentdom.Repository,
	pubsub agentEventPublisher,
) *Router {
	return &Router{
		logger:         logger,
		questions:      questions,
		sessions:       sessions,
		pubsub:         pubsub,
		threadCreators: make(map[messenger.Platform]threadCreator),
	}
}

// ConfigureThreading enables messenger thread creation for HITL questions.
func (r *Router) ConfigureThreading(threader threadCreator, contexts sessionContextLookup) {
	r.ConfigureThreadingForPlatform(threader, contexts)
}

// ConfigureThreadingForPlatform registers a messenger-specific thread creator.
func (r *Router) ConfigureThreadingForPlatform(threader threadCreator, contexts sessionContextLookup) {
	if r == nil {
		return
	}

	if threader != nil {
		r.threadCreators[threader.Platform()] = threader
	}
	r.sessionContexts = contexts
}

// AskHumanInput represents the parameters for asking a human a question.
type AskHumanInput struct {
	Question string          `json:"question"`
	Options  json.RawMessage `json:"options,omitempty"`
}

// HandleAskHuman processes an ask_human tool call from an agent.
func (r *Router) HandleAskHuman(
	ctx context.Context,
	tenantID domain.TenantID,
	sessionID domain.AgentSessionID,
	input json.RawMessage,
) (*hitl.Question, error) {
	// Parse input
	var ask AskHumanInput
	if err := json.Unmarshal(input, &ask); err != nil {
		return nil, fmt.Errorf("hitlrouter.HandleAskHuman: unmarshal input: %w", err)
	}

	// Validate question text
	if ask.Question == "" {
		return nil, fmt.Errorf("hitlrouter.HandleAskHuman: question text required: %w", domain.ErrInvalidInput)
	}

	// Compute timeout
	now := time.Now().UTC()
	timeout := now.Add(DefaultTimeout)

	// Create HITL Question entity
	question := &hitl.Question{
		ID:             domain.NewID(),
		TenantID:       tenantID,
		AgentSessionID: sessionID,
		Question:       ask.Question,
		Options:        ask.Options,
		Status:         hitl.StatusPending,
		CreatedAt:      now,
		TimeoutAt:      &timeout,
	}

	// Persist question
	if err := r.questions.Create(ctx, question); err != nil {
		return nil, fmt.Errorf("hitlrouter.HandleAskHuman: create question: %w", err)
	}

	// Transition agent session to waiting_hitl
	if err := r.sessions.UpdateStatus(ctx, tenantID, sessionID, agentdom.StatusWaitingHITL); err != nil {
		if deleteErr := r.questions.Delete(ctx, tenantID, question.ID); deleteErr != nil {
			return nil, fmt.Errorf("hitlrouter.HandleAskHuman: update session status: %w (cleanup failed: %w)", err, deleteErr)
		}
		return nil, fmt.Errorf("hitlrouter.HandleAskHuman: update session status: %w", err)
	}

	threadPlatform, threadID, threadErr := r.attachMessengerThread(ctx, tenantID, sessionID, question)
	if threadErr != nil {
		r.logger.ErrorContext(ctx, "failed to attach messenger thread for HITL question",
			"error", threadErr,
			"session_id", sessionID,
			"question_id", question.ID,
		)
	} else if threadID != "" {
		if err := r.questions.UpdateMessengerThread(ctx, tenantID, question.ID, threadPlatform, threadID); err != nil {
			r.logger.ErrorContext(ctx, "failed to persist messenger thread metadata for HITL question",
				"error", err,
				"session_id", sessionID,
				"question_id", question.ID,
				"thread_id", threadID,
			)
		} else {
			question.MessengerThreadID = &threadID
			question.MessengerPlatform = &threadPlatform
		}
	}

	// Publish agent status event
	payload := map[string]any{
		"session_id":  sessionID,
		"status":      "waiting_hitl",
		"question_id": question.ID,
	}
	if err := r.pubsub.PublishAgentEvent(ctx, sessionID.String(), redispkg.EventAgentStatus, payload); err != nil {
		// Log but don't fail: event delivery is best-effort
		r.logger.ErrorContext(ctx, "publish agent status event failed",
			"error", err,
			"session_id", sessionID,
			"question_id", question.ID,
		)
	}

	r.logger.InfoContext(ctx, "HITL question created",
		"session_id", sessionID,
		"question_id", question.ID,
		"question", ask.Question,
	)

	return question, nil
}

// AnswerByThread processes a threaded messenger reply for a pending question.
func (r *Router) AnswerByThread(
	ctx context.Context,
	tenantID domain.TenantID,
	platform, threadID, answer string,
	answeredBy domain.UserID,
) (*hitl.Question, error) {
	question, err := r.questions.GetPendingByThread(ctx, tenantID, platform, threadID)
	if err != nil {
		return nil, fmt.Errorf("hitlrouter.AnswerByThread: get pending question by thread: %w", err)
	}

	if answerErr := r.AnswerQuestion(ctx, tenantID, question.ID, answer, answeredBy); answerErr != nil {
		return nil, answerErr
	}

	answeredAt := time.Now().UTC()
	question.Status = hitl.StatusAnswered
	question.Answer = &answer
	question.AnsweredBy = &answeredBy
	question.AnsweredAt = &answeredAt

	return question, nil
}

// AnswerQuestion processes a human answer to a HITL question.
func (r *Router) AnswerQuestion(
	ctx context.Context,
	tenantID domain.TenantID,
	questionID domain.HITLQuestionID,
	answer string,
	answeredBy domain.UserID,
) error {
	// Load question
	question, err := r.questions.GetByID(ctx, tenantID, questionID)
	if err != nil {
		return fmt.Errorf("hitlrouter.AnswerQuestion: get question: %w", err)
	}

	// Verify pending status
	if question.Status != hitl.StatusPending {
		return fmt.Errorf("hitlrouter.AnswerQuestion: question not in pending status (current: %s): %w",
			question.Status, domain.ErrInvalidInput)
	}

	// Answer the question
	if answerErr := r.questions.Answer(ctx, tenantID, questionID, answer, answeredBy); answerErr != nil {
		return fmt.Errorf("hitlrouter.AnswerQuestion: answer question: %w", answerErr)
	}

	r.logger.InfoContext(ctx, "HITL question answered",
		"session_id", question.AgentSessionID,
		"question_id", questionID,
		"answered_by", answeredBy,
	)

	return nil
}

// ResetQuestion clears a previously recorded answer and reopens the question.
func (r *Router) ResetQuestion(
	ctx context.Context,
	tenantID domain.TenantID,
	questionID domain.HITLQuestionID,
) error {
	if err := r.questions.ResetAnswer(ctx, tenantID, questionID); err != nil {
		return fmt.Errorf("hitlrouter.ResetQuestion: reset answer: %w", err)
	}
	return nil
}

// ListBySession returns all HITL questions for a given agent session.
func (r *Router) ListBySession(
	ctx context.Context,
	tenantID domain.TenantID,
	sessionID domain.AgentSessionID,
) ([]*hitl.Question, error) {
	questions, err := r.questions.ListBySession(ctx, tenantID, sessionID)
	if err != nil {
		return nil, fmt.Errorf("hitlrouter.ListBySession: list questions: %w", err)
	}
	return questions, nil
}

// GetQuestion retrieves a single HITL question by ID.
func (r *Router) GetQuestion(
	ctx context.Context,
	tenantID domain.TenantID,
	questionID domain.HITLQuestionID,
) (*hitl.Question, error) {
	question, err := r.questions.GetByID(ctx, tenantID, questionID)
	if err != nil {
		return nil, fmt.Errorf("hitlrouter.GetQuestion: get question: %w", err)
	}
	return question, nil
}

func (r *Router) attachMessengerThread(
	ctx context.Context,
	tenantID domain.TenantID,
	sessionID domain.AgentSessionID,
	question *hitl.Question,
) (platform, threadID string, err error) {
	if r.sessionContexts == nil {
		return "", "", nil
	}

	target, ok := r.sessionContexts.Get(tenantID, sessionID)
	if !ok {
		return "", "", nil
	}
	creator, ok := r.threadCreators[target.Platform]
	if !ok || creator == nil {
		return "", "", nil
	}
	if strings.TrimSpace(target.ChannelID) == "" || strings.TrimSpace(target.ParentMessageID) == "" {
		return "", "", nil
	}

	options, err := parseThreadOptions(question.Options)
	if err != nil {
		return "", "", err
	}

	result, err := creator.CreateThread(ctx, messenger.CreateThreadParams{
		ChannelID:       target.ChannelID,
		ParentMessageID: target.ParentMessageID,
		Text:            question.Question,
		ActionID:        formatActionID(question.ID, sessionID),
		Options:         options,
	})
	if err != nil {
		return "", "", err
	}
	return string(target.Platform), result.ThreadID, nil
}

func parseThreadOptions(raw json.RawMessage) ([]messenger.ThreadOption, error) {
	if len(raw) == 0 || string(raw) == "null" {
		return nil, nil
	}

	var stringOptions []string
	if err := json.Unmarshal(raw, &stringOptions); err == nil {
		options := make([]messenger.ThreadOption, 0, len(stringOptions))
		for _, option := range stringOptions {
			trimmed := strings.TrimSpace(option)
			if trimmed == "" {
				continue
			}
			options = append(options, messenger.ThreadOption{Label: trimmed, Value: trimmed})
		}
		return options, nil
	}

	var structured []messenger.ThreadOption
	if err := json.Unmarshal(raw, &structured); err != nil {
		return nil, fmt.Errorf("parse thread options: %w", err)
	}
	return structured, nil
}

func formatActionID(questionID domain.HITLQuestionID, sessionID domain.AgentSessionID) string {
	return "steerlane:hitl:" + questionID.String() + ":" + sessionID.String()
}
