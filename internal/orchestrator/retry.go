package orchestrator

import (
	"context"
	"fmt"
	"math"
	"time"

	"github.com/gosuda/steerlane/internal/domain"
	agentdom "github.com/gosuda/steerlane/internal/domain/agent"
)

const (
	defaultRetryPollInterval = 5 * time.Second
	defaultRetryBaseDelay    = 10 * time.Second
	defaultRetryMaxDelay     = 5 * time.Minute
	defaultRetryMaxAttempts  = 3
	retrySweepBatchSize      = 25
	retryActionTimeout       = 30 * time.Second
)

// RetryScheduler re-dispatches failed sessions with bounded exponential backoff.
type RetryScheduler struct {
	orchestrator *Orchestrator
	pollInterval time.Duration
	baseDelay    time.Duration
	maxDelay     time.Duration
	maxAttempts  int
}

// NewRetryScheduler constructs a retry scheduler with optional overrides.
func NewRetryScheduler(orchestrator *Orchestrator, pollInterval, baseDelay, maxDelay time.Duration, maxAttempts int) *RetryScheduler {
	if pollInterval <= 0 {
		pollInterval = defaultRetryPollInterval
	}
	if baseDelay <= 0 {
		baseDelay = defaultRetryBaseDelay
	}
	if maxDelay <= 0 {
		maxDelay = defaultRetryMaxDelay
	}
	if maxAttempts <= 0 {
		maxAttempts = defaultRetryMaxAttempts
	}

	return &RetryScheduler{
		orchestrator: orchestrator,
		pollInterval: pollInterval,
		baseDelay:    baseDelay,
		maxDelay:     maxDelay,
		maxAttempts:  maxAttempts,
	}
}

// Start begins the background retry sweep loop.
func (s *RetryScheduler) Start(ctx context.Context) {
	if s == nil || s.orchestrator == nil || s.orchestrator.deps.Sessions == nil {
		return
	}

	ticker := time.NewTicker(s.pollInterval)
	go func() {
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			case now := <-ticker.C:
				s.retryReadySessions(now.UTC())
			}
		}
	}()
}

func (s *RetryScheduler) shouldRetry(session *agentdom.Session) bool {
	return s != nil && session != nil && session.RetryCount < s.maxAttempts
}

func (s *RetryScheduler) nextRetryAt(now time.Time, attempt int) time.Time {
	return now.Add(s.retryDelay(attempt)).UTC()
}

func (s *RetryScheduler) retryDelay(attempt int) time.Duration {
	if attempt <= 1 {
		return s.baseDelay
	}

	multiplier := math.Pow(2, float64(attempt-1))
	delay := time.Duration(float64(s.baseDelay) * multiplier)
	if delay > s.maxDelay {
		return s.maxDelay
	}
	return delay
}

func (s *RetryScheduler) retryReadySessions(now time.Time) {
	items, err := s.orchestrator.deps.Sessions.ListRetryReady(context.Background(), now, retrySweepBatchSize)
	if err != nil {
		s.orchestrator.logger.Error("retry sweep failed", "error", err)
		return
	}

	for _, session := range items {
		s.retrySession(session, now)
	}
}

func (s *RetryScheduler) retrySession(session *agentdom.Session, now time.Time) {
	if session == nil {
		return
	}

	prompt, err := retryPrompt(session)
	if err != nil {
		s.orchestrator.logger.Error("retry skipped: prompt unavailable",
			"error", err,
			"session_id", session.ID,
			"task_id", session.TaskID,
		)
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), retryActionTimeout)
	defer cancel()

	if claimErr := s.orchestrator.deps.Sessions.ScheduleRetry(ctx, session.TenantID, session.ID, session.RetryCount, nil); claimErr != nil {
		s.orchestrator.logger.ErrorContext(ctx, "failed to claim retryable session",
			"error", claimErr,
			"session_id", session.ID,
		)
		return
	}

	if _, dispatchErr := s.orchestrator.dispatchTaskAttempt(ctx, session.TenantID, session.TaskID, session.AgentType, prompt, nil, session.RetryCount); dispatchErr != nil {
		nextRetryAt := s.nextRetryAt(now, session.RetryCount+1)
		if scheduleErr := s.orchestrator.deps.Sessions.ScheduleRetry(ctx, session.TenantID, session.ID, session.RetryCount, &nextRetryAt); scheduleErr != nil {
			s.orchestrator.logger.ErrorContext(ctx, "failed to reschedule retry after dispatch failure",
				"error", scheduleErr,
				"session_id", session.ID,
				"task_id", session.TaskID,
			)
		}
		s.orchestrator.logger.ErrorContext(ctx, "retry dispatch failed",
			"error", dispatchErr,
			"session_id", session.ID,
			"task_id", session.TaskID,
			"retry_count", session.RetryCount,
			"next_retry_at", nextRetryAt,
		)
		return
	}

	s.orchestrator.logger.InfoContext(ctx, "session retry dispatched",
		"session_id", session.ID,
		"task_id", session.TaskID,
		"retry_count", session.RetryCount,
	)
}

func retryPrompt(session *agentdom.Session) (string, error) {
	if session == nil {
		return "", fmt.Errorf("nil session: %w", domain.ErrInvalidInput)
	}
	prompt, ok := session.Metadata["prompt"].(string)
	if !ok || prompt == "" {
		return "", fmt.Errorf("session %s missing prompt metadata: %w", session.ID, domain.ErrInvalidInput)
	}
	return prompt, nil
}
