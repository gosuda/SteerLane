package orchestrator

import (
	"context"
	"time"
)

const (
	defaultReconcileInterval = 60 * time.Second
	reconcileActionTimeout   = 30 * time.Second
	steerlaneLabel           = "managed-by"
	steerlaneLabelValue      = "steerlane"
)

// Reconciler periodically detects and cleans up orphan containers.
type Reconciler struct {
	orchestrator *Orchestrator
	interval     time.Duration
}

// NewReconciler creates a reconciler with the given interval.
// If interval <= 0, defaultReconcileInterval is used.
func NewReconciler(orchestrator *Orchestrator, interval time.Duration) *Reconciler {
	if interval <= 0 {
		interval = defaultReconcileInterval
	}
	return &Reconciler{
		orchestrator: orchestrator,
		interval:     interval,
	}
}

// Start begins the background reconciliation loop.
func (r *Reconciler) Start(ctx context.Context) {
	if r == nil || r.orchestrator == nil || r.orchestrator.deps.Runtime == nil {
		return
	}

	ticker := time.NewTicker(r.interval)
	go func() {
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				r.reconcile(ctx)
			}
		}
	}()
}

func (r *Reconciler) reconcile(ctx context.Context) {
	containers, err := r.orchestrator.deps.Runtime.ListContainersByLabel(ctx, steerlaneLabel, steerlaneLabelValue)
	if err != nil {
		r.orchestrator.logger.ErrorContext(ctx, "reconciler: failed to list containers", "error", err)
		return
	}

	activeContainers := r.orchestrator.activeContainerIDs()

	for _, c := range containers {
		if activeContainers[c.ContainerID] {
			continue
		}

		r.orchestrator.logger.WarnContext(ctx, "reconciler: removing orphan container",
			"container_id", c.ContainerID,
			"running", c.Running,
		)

		cleanupCtx, cancel := context.WithTimeout(ctx, reconcileActionTimeout)
		if c.Running {
			if stopErr := r.orchestrator.deps.Runtime.StopContainer(cleanupCtx, c.ContainerID); stopErr != nil {
				r.orchestrator.logger.ErrorContext(ctx, "reconciler: failed to stop orphan container",
					"error", stopErr,
					"container_id", c.ContainerID,
				)
				cancel()
				continue
			}
		}
		if rmErr := r.orchestrator.deps.Runtime.RemoveContainer(cleanupCtx, c.ContainerID); rmErr != nil {
			r.orchestrator.logger.ErrorContext(ctx, "reconciler: failed to remove orphan container",
				"error", rmErr,
				"container_id", c.ContainerID,
			)
		}
		cancel()
	}
}
