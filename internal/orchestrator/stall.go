package orchestrator

import (
	"context"
	"errors"
	"time"

	"github.com/gosuda/steerlane/internal/domain"
)

const (
	defaultStallCheckInterval = 30 * time.Second
	defaultStallTimeout       = 5 * time.Minute
	stallActionTimeout        = 30 * time.Second
)

var errSessionStalled = errors.New("session stalled: agent output timeout exceeded")

// StallDetector scans active sessions for stalled agent output.
type StallDetector struct {
	orchestrator  *Orchestrator
	checkInterval time.Duration
	stallTimeout  time.Duration
}

// NewStallDetector constructs a detector with optional timeout overrides.
func NewStallDetector(orchestrator *Orchestrator, stallTimeout, checkInterval time.Duration) *StallDetector {
	if stallTimeout <= 0 {
		stallTimeout = defaultStallTimeout
	}
	if checkInterval <= 0 {
		checkInterval = defaultStallCheckInterval
	}

	return &StallDetector{
		orchestrator:  orchestrator,
		checkInterval: checkInterval,
		stallTimeout:  stallTimeout,
	}
}

// Start begins the background stalled-session sweep loop.
func (d *StallDetector) Start(ctx context.Context) {
	if d == nil || d.orchestrator == nil {
		return
	}

	ticker := time.NewTicker(d.checkInterval)
	go func() {
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			case now := <-ticker.C:
				d.checkStalledSessions(now.UTC())
			}
		}
	}()
}

func (d *StallDetector) checkStalledSessions(now time.Time) {
	if d == nil || d.orchestrator == nil {
		return
	}

	type stalledSession struct {
		sessionID   domain.AgentSessionID
		taskID      domain.TaskID
		tenantID    domain.TenantID
		lastEventAt int64
	}

	stalled := make([]stalledSession, 0)
	d.orchestrator.mu.Lock()
	for sessionID, session := range d.orchestrator.sessions {
		if session == nil {
			continue
		}

		lastEventAt := session.lastEventAt.Load()
		if now.Sub(time.Unix(0, lastEventAt)) <= d.stallTimeout {
			continue
		}

		stalled = append(stalled, stalledSession{
			sessionID:   sessionID,
			taskID:      session.taskID,
			tenantID:    session.tenantID,
			lastEventAt: lastEventAt,
		})
	}
	d.orchestrator.mu.Unlock()

	for _, session := range stalled {
		idleFor := now.Sub(time.Unix(0, session.lastEventAt))
		d.orchestrator.logger.Warn("session stalled",
			"session_id", session.sessionID,
			"task_id", session.taskID,
			"tenant_id", session.tenantID,
			"idle_for", idleFor,
			"stall_timeout", d.stallTimeout,
			"last_event_at", time.Unix(0, session.lastEventAt).UTC(),
		)
		d.orchestrator.handleStall(session.sessionID)
	}
}

func (o *Orchestrator) handleStall(sessionID domain.AgentSessionID) {
	if o == nil {
		return
	}

	rs, err := o.lookupSession(sessionID)
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			return
		}
		o.logger.Error("failed to load stalled session",
			"error", err,
			"session_id", sessionID,
		)
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), stallActionTimeout)
	defer cancel()

	if cancelErr := rs.backend.Cancel(ctx); cancelErr != nil {
		o.logger.ErrorContext(ctx, "failed to cancel stalled backend",
			"error", cancelErr,
			"session_id", sessionID,
		)
	}

	o.failSession(ctx, rs.tenantID, sessionID, rs.taskID, errSessionStalled)
	o.revertTaskToBacklog(ctx, rs.tenantID, rs.taskID)
	o.cancelPendingHITL(ctx, rs.tenantID, sessionID)
	o.disposeSession(sessionID, rs)
}
