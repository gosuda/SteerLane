package postgres

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/gosuda/steerlane/internal/domain"
	"github.com/gosuda/steerlane/internal/domain/hitl"
	"github.com/gosuda/steerlane/internal/store/postgres/sqlc"
)

var _ hitl.Repository = (*hitlRepository)(nil)

const timeoutNotificationClaimTTL = 5 * time.Minute

type hitlRepository struct {
	store *Store
}

func mapHITLQuestionModel(row sqlc.HitlQuestion) (*hitl.Question, error) {
	entity := &hitl.Question{
		ID:                row.ID,
		TenantID:          row.TenantID,
		AgentSessionID:    row.AgentSessionID,
		Question:          row.Question,
		Options:           rawJSONOrDefault(row.Options, "null"),
		MessengerThreadID: row.MessengerThreadID,
		MessengerPlatform: row.MessengerPlatform,
		Answer:            row.Answer,
		AnsweredBy:        uuidPtrFromPG(row.AnsweredBy),
		Status:            hitl.QuestionStatus(row.Status),
		TimeoutAt:         timePtrFromPG(row.TimeoutAt),
		CreatedAt:         row.CreatedAt,
		AnsweredAt:        timePtrFromPG(row.AnsweredAt),
	}
	if err := entity.Validate(); err != nil {
		return nil, fmt.Errorf("map hitl question: %w", err)
	}
	return entity, nil
}

func mapHITLQuestions(rows []sqlc.HitlQuestion) ([]*hitl.Question, error) {
	items := make([]*hitl.Question, 0, len(rows))
	for _, row := range rows {
		mapped, err := mapHITLQuestionModel(row)
		if err != nil {
			return nil, err
		}
		items = append(items, mapped)
	}
	return items, nil
}

func (r *hitlRepository) Create(ctx context.Context, record *hitl.Question) error {
	if record == nil {
		return fmt.Errorf("hitl question: %w", domain.ErrInvalidInput)
	}
	if err := requireUUID("hitl tenant id", record.TenantID); err != nil {
		return err
	}
	if err := requireUUID("hitl agent session id", record.AgentSessionID); err != nil {
		return err
	}
	if err := record.Validate(); err != nil {
		return err
	}

	row, err := r.store.queries.CreateHITLQuestion(ctx, sqlc.CreateHITLQuestionParams{
		ID:                record.ID,
		TenantID:          record.TenantID,
		AgentSessionID:    record.AgentSessionID,
		Question:          record.Question,
		Options:           rawJSONOrDefault(record.Options, "null"),
		MessengerThreadID: record.MessengerThreadID,
		MessengerPlatform: record.MessengerPlatform,
		Status:            string(record.Status),
		TimeoutAt:         pgTimestamptzFromPtr(record.TimeoutAt),
	})
	if err != nil {
		return fmt.Errorf("create hitl question: %w", classifyError(err))
	}

	mapped, err := mapHITLQuestionModel(row)
	if err != nil {
		return err
	}
	*record = *mapped

	return nil
}

func (r *hitlRepository) Delete(ctx context.Context, tenantID domain.TenantID, id domain.HITLQuestionID) error {
	if err := requireUUID("tenant id", tenantID); err != nil {
		return err
	}
	if err := requireUUID("hitl question id", id); err != nil {
		return err
	}

	if err := r.store.queries.DeleteHITLQuestion(ctx, sqlc.DeleteHITLQuestionParams{ID: id, TenantID: tenantID}); err != nil {
		return fmt.Errorf("delete hitl question: %w", classifyError(err))
	}
	return nil
}

func (r *hitlRepository) CancelPendingBySession(ctx context.Context, tenantID domain.TenantID, sessionID domain.AgentSessionID) error {
	if err := requireUUID("tenant id", tenantID); err != nil {
		return err
	}
	if err := requireUUID("agent session id", sessionID); err != nil {
		return err
	}

	if err := r.store.queries.CancelPendingHITLQuestionsBySession(ctx, sqlc.CancelPendingHITLQuestionsBySessionParams{TenantID: tenantID, AgentSessionID: sessionID}); err != nil {
		return fmt.Errorf("cancel pending hitl questions by session: %w", classifyError(err))
	}
	return nil
}

func (r *hitlRepository) ClearTimeoutNotificationClaim(ctx context.Context, tenantID domain.TenantID, id domain.HITLQuestionID) error {
	if err := requireUUID("tenant id", tenantID); err != nil {
		return err
	}
	if err := requireUUID("hitl question id", id); err != nil {
		return err
	}

	if err := r.store.queries.ClearHITLTimeoutNotificationClaim(ctx, sqlc.ClearHITLTimeoutNotificationClaimParams{
		ID:       id,
		TenantID: tenantID,
	}); err != nil {
		return fmt.Errorf("clear hitl timeout notification claim: %w", classifyError(err))
	}
	return nil
}

func (r *hitlRepository) ClaimTimeoutNotification(ctx context.Context, tenantID domain.TenantID, id domain.HITLQuestionID) (*hitl.Question, error) {
	if err := requireUUID("tenant id", tenantID); err != nil {
		return nil, err
	}
	if err := requireUUID("hitl question id", id); err != nil {
		return nil, err
	}

	staleBefore := time.Now().UTC().Add(-timeoutNotificationClaimTTL)
	row, err := r.store.queries.ClaimHITLTimeoutNotification(ctx, sqlc.ClaimHITLTimeoutNotificationParams{
		ID:          id,
		TenantID:    tenantID,
		StaleBefore: pgTimestamptzFromPtr(&staleBefore),
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, fmt.Errorf("claim hitl timeout notification: %w", domain.ErrInvalidTransition)
		}
		return nil, fmt.Errorf("claim hitl timeout notification: %w", classifyError(err))
	}

	entity, err := mapHITLQuestionModel(row)
	if err != nil {
		return nil, err
	}
	return entity, nil
}

func (r *hitlRepository) MarkTimeoutNotificationSent(ctx context.Context, tenantID domain.TenantID, id domain.HITLQuestionID) error {
	if err := requireUUID("tenant id", tenantID); err != nil {
		return err
	}
	if err := requireUUID("hitl question id", id); err != nil {
		return err
	}

	if _, err := r.store.queries.MarkHITLTimeoutNotificationSent(ctx, sqlc.MarkHITLTimeoutNotificationSentParams{ //nolint:govet // short-lived err shadow is idiomatic Go
		ID:       id,
		TenantID: tenantID,
	}); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return fmt.Errorf("mark hitl timeout notification sent: %w", domain.ErrInvalidTransition)
		}
		return fmt.Errorf("mark hitl timeout notification sent: %w", classifyError(err))
	}
	return nil
}

func (r *hitlRepository) GetByID(ctx context.Context, tenantID domain.TenantID, id domain.HITLQuestionID) (*hitl.Question, error) {
	if err := requireUUID("tenant id", tenantID); err != nil {
		return nil, err
	}
	if err := requireUUID("hitl question id", id); err != nil {
		return nil, err
	}

	row, err := r.store.queries.GetHITLQuestionByID(ctx, sqlc.GetHITLQuestionByIDParams{ID: id, TenantID: tenantID})
	if err != nil {
		return nil, fmt.Errorf("get hitl question by id: %w", classifyError(err))
	}

	entity, err := mapHITLQuestionModel(row)
	if err != nil {
		return nil, err
	}
	return entity, nil
}

func (r *hitlRepository) GetPendingByThread(ctx context.Context, tenantID domain.TenantID, platform, threadID string) (*hitl.Question, error) {
	if err := requireUUID("tenant id", tenantID); err != nil {
		return nil, err
	}
	if err := requireString("messenger platform", platform); err != nil {
		return nil, err
	}
	if err := requireString("messenger thread id", threadID); err != nil {
		return nil, err
	}

	trimmedThreadID := strings.TrimSpace(threadID)
	trimmedPlatform := strings.TrimSpace(platform)
	rows, err := r.store.queries.GetPendingHITLQuestionByThread(ctx, sqlc.GetPendingHITLQuestionByThreadParams{
		TenantID:          tenantID,
		MessengerPlatform: &trimmedPlatform,
		MessengerThreadID: &trimmedThreadID,
	})
	if err != nil {
		return nil, fmt.Errorf("get pending hitl question by thread: %w", classifyError(err))
	}
	if len(rows) == 0 {
		return nil, fmt.Errorf("get pending hitl question by thread: %w", domain.ErrNotFound)
	}
	if len(rows) > 1 {
		return nil, fmt.Errorf("get pending hitl question by thread: %w", domain.ErrInvalidInput)
	}

	entity, err := mapHITLQuestionModel(rows[0])
	if err != nil {
		return nil, err
	}
	return entity, nil
}

func (r *hitlRepository) UpdateMessengerThread(ctx context.Context, tenantID domain.TenantID, id domain.HITLQuestionID, platform, threadID string) error {
	if err := requireUUID("tenant id", tenantID); err != nil {
		return err
	}
	if err := requireUUID("hitl question id", id); err != nil {
		return err
	}
	if err := requireString("messenger platform", platform); err != nil {
		return err
	}
	if err := requireString("messenger thread id", threadID); err != nil {
		return err
	}

	trimmedPlatform := strings.TrimSpace(platform)
	trimmedThreadID := strings.TrimSpace(threadID)
	if _, err := r.store.queries.UpdateHITLQuestionMessengerThread(ctx, sqlc.UpdateHITLQuestionMessengerThreadParams{ //nolint:govet // short-lived err shadow is idiomatic Go
		ID:                id,
		TenantID:          tenantID,
		MessengerPlatform: &trimmedPlatform,
		MessengerThreadID: &trimmedThreadID,
	}); err != nil {
		return fmt.Errorf("update hitl messenger thread: %w", classifyError(err))
	}
	return nil
}

func (r *hitlRepository) Escalate(ctx context.Context, tenantID domain.TenantID, id domain.HITLQuestionID, newTimeoutAt time.Time) error {
	if err := requireUUID("tenant id", tenantID); err != nil {
		return err
	}
	if err := requireUUID("hitl question id", id); err != nil {
		return err
	}

	if _, err := r.store.queries.EscalateHITLQuestion(ctx, sqlc.EscalateHITLQuestionParams{
		ID:           id,
		TenantID:     tenantID,
		NewTimeoutAt: pgtype.Timestamptz{Time: newTimeoutAt, Valid: true},
	}); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return fmt.Errorf("escalate hitl question: %w", domain.ErrInvalidTransition)
		}
		return fmt.Errorf("escalate hitl question: %w", classifyError(err))
	}
	return nil
}

func (r *hitlRepository) ListEscalatedExpiredBefore(ctx context.Context, before time.Time, limit int) ([]*hitl.Question, error) {
	if limit <= 0 {
		limit = 50
	}

	rows, err := r.store.queries.ListEscalatedExpiredHITLQuestionsBefore(ctx, sqlc.ListEscalatedExpiredHITLQuestionsBeforeParams{
		TimeoutAt: pgTimestamptzFromPtr(&before),
		Limit:     int32(limit),
	})
	if err != nil {
		return nil, fmt.Errorf("list escalated expired hitl questions before: %w", classifyError(err))
	}

	return mapHITLQuestions(rows)
}

func (r *hitlRepository) MarkTimedOutEscalated(ctx context.Context, tenantID domain.TenantID, id domain.HITLQuestionID) error {
	if err := requireUUID("tenant id", tenantID); err != nil {
		return err
	}
	if err := requireUUID("hitl question id", id); err != nil {
		return err
	}

	if _, err := r.store.queries.MarkTimedOutEscalatedHITLQuestion(ctx, sqlc.MarkTimedOutEscalatedHITLQuestionParams{
		ID:       id,
		TenantID: tenantID,
	}); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return fmt.Errorf("mark timed out escalated hitl question: %w", domain.ErrInvalidTransition)
		}
		return fmt.Errorf("mark timed out escalated hitl question: %w", classifyError(err))
	}
	return nil
}

func (r *hitlRepository) Answer(ctx context.Context, tenantID domain.TenantID, id domain.HITLQuestionID, answer string, answeredBy domain.UserID) error {
	if err := requireUUID("tenant id", tenantID); err != nil {
		return err
	}
	if err := requireUUID("hitl question id", id); err != nil {
		return err
	}
	if err := requireUUID("answered by", answeredBy); err != nil {
		return err
	}
	if err := requireString("hitl answer", answer); err != nil {
		return err
	}

	current, err := r.GetByID(ctx, tenantID, id)
	if err != nil {
		return err
	}
	if current.Status != hitl.StatusPending && current.Status != hitl.StatusEscalated {
		return fmt.Errorf("cannot answer hitl question from %q: %w", current.Status, domain.ErrInvalidTransition)
	}

	if _, err := r.store.queries.AnswerHITLQuestion(ctx, sqlc.AnswerHITLQuestionParams{ //nolint:govet // short-lived err shadow is idiomatic Go
		ID:         id,
		Answer:     &answer,
		AnsweredBy: pgUUIDFromPtr(&answeredBy),
		TenantID:   tenantID,
	}); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return fmt.Errorf("answer hitl question: %w", domain.ErrInvalidTransition)
		}
		return fmt.Errorf("answer hitl question: %w", classifyError(err))
	}
	return nil
}

func (r *hitlRepository) ResetAnswer(ctx context.Context, tenantID domain.TenantID, id domain.HITLQuestionID) error {
	if err := requireUUID("tenant id", tenantID); err != nil {
		return err
	}
	if err := requireUUID("hitl question id", id); err != nil {
		return err
	}

	if _, err := r.store.queries.ResetHITLQuestionAnswer(ctx, sqlc.ResetHITLQuestionAnswerParams{ //nolint:govet // short-lived err shadow is idiomatic Go
		ID:       id,
		TenantID: tenantID,
	}); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return fmt.Errorf("reset hitl question answer: %w", domain.ErrInvalidTransition)
		}
		return fmt.Errorf("reset hitl question answer: %w", classifyError(err))
	}
	return nil
}

func (r *hitlRepository) ReopenTimedOut(ctx context.Context, tenantID domain.TenantID, id domain.HITLQuestionID) error {
	if err := requireUUID("tenant id", tenantID); err != nil {
		return err
	}
	if err := requireUUID("hitl question id", id); err != nil {
		return err
	}

	if _, err := r.store.queries.ReopenTimedOutHITLQuestion(ctx, sqlc.ReopenTimedOutHITLQuestionParams{ //nolint:govet // short-lived err shadow is idiomatic Go
		ID:       id,
		TenantID: tenantID,
	}); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return fmt.Errorf("reopen timed out hitl question: %w", domain.ErrInvalidTransition)
		}
		return fmt.Errorf("reopen timed out hitl question: %w", classifyError(err))
	}
	return nil
}

func (r *hitlRepository) ListBySession(ctx context.Context, tenantID domain.TenantID, sessionID domain.AgentSessionID) ([]*hitl.Question, error) {
	if err := requireUUID("tenant id", tenantID); err != nil {
		return nil, err
	}
	if err := requireUUID("agent session id", sessionID); err != nil {
		return nil, err
	}

	rows, err := r.store.queries.ListHITLQuestionsBySession(ctx, sqlc.ListHITLQuestionsBySessionParams{AgentSessionID: sessionID, TenantID: tenantID})
	if err != nil {
		return nil, fmt.Errorf("list hitl questions by session: %w", classifyError(err))
	}
	return mapHITLQuestions(rows)
}

func (r *hitlRepository) ListExpiredPendingBefore(ctx context.Context, before time.Time, limit int) ([]*hitl.Question, error) {
	if limit <= 0 {
		limit = 50
	}

	rows, err := r.store.queries.ListExpiredPendingHITLQuestionsBefore(ctx, sqlc.ListExpiredPendingHITLQuestionsBeforeParams{
		TimeoutAt: pgTimestamptzFromPtr(&before),
		Limit:     int32(limit),
	})
	if err != nil {
		return nil, fmt.Errorf("list expired pending hitl questions before: %w", classifyError(err))
	}

	return mapHITLQuestions(rows)
}

func (r *hitlRepository) ListTimedOut(ctx context.Context, limit int) ([]*hitl.Question, error) {
	if limit <= 0 {
		limit = 50
	}

	rows, err := r.store.queries.ListTimedOutHITLQuestions(ctx, int32(limit))
	if err != nil {
		return nil, fmt.Errorf("list timed out hitl questions: %w", classifyError(err))
	}

	return mapHITLQuestions(rows)
}

func (r *hitlRepository) ListUnnotifiedTimedOut(ctx context.Context, limit int) ([]*hitl.Question, error) {
	if limit <= 0 {
		limit = 50
	}

	staleBefore := time.Now().UTC().Add(-timeoutNotificationClaimTTL)
	rows, err := r.store.queries.ListUnnotifiedTimedOutHITLQuestions(ctx, sqlc.ListUnnotifiedTimedOutHITLQuestionsParams{
		TimeoutNotificationClaimedAt: pgTimestamptzFromPtr(&staleBefore),
		Limit:                        int32(limit),
	})
	if err != nil {
		return nil, fmt.Errorf("list unnotified timed out hitl questions: %w", classifyError(err))
	}

	return mapHITLQuestions(rows)
}

func (r *hitlRepository) MarkTimedOut(ctx context.Context, tenantID domain.TenantID, id domain.HITLQuestionID) error {
	if err := requireUUID("tenant id", tenantID); err != nil {
		return err
	}
	if err := requireUUID("hitl question id", id); err != nil {
		return err
	}

	if _, err := r.store.queries.MarkTimedOutHITLQuestion(ctx, sqlc.MarkTimedOutHITLQuestionParams{ //nolint:govet // short-lived err shadow is idiomatic Go
		ID:       id,
		TenantID: tenantID,
	}); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return fmt.Errorf("mark timed out hitl question: %w", domain.ErrInvalidTransition)
		}
		return fmt.Errorf("mark timed out hitl question: %w", classifyError(err))
	}
	return nil
}

func (r *hitlRepository) MarkTimedOutBefore(ctx context.Context, before time.Time, limit int) ([]*hitl.Question, error) {
	if limit <= 0 {
		limit = 50
	}

	rows, err := r.store.queries.MarkTimedOutHITLQuestionsBefore(ctx, sqlc.MarkTimedOutHITLQuestionsBeforeParams{
		TimeoutAt: pgTimestamptzFromPtr(&before),
		Limit:     int32(limit),
	})
	if err != nil {
		return nil, fmt.Errorf("mark timed out hitl questions before: %w", classifyError(err))
	}

	return mapHITLQuestions(rows)
}
