// Package adrengine processes ADR tool calls from agents.
package adrengine

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"github.com/gosuda/steerlane/internal/domain"
	"github.com/gosuda/steerlane/internal/domain/adr"
	redispkg "github.com/gosuda/steerlane/internal/store/redis"
)

// Engine coordinates ADR creation from agent tool calls.
type Engine struct {
	logger *slog.Logger
	adrs   adr.Repository
	pubsub *redispkg.PubSub
}

// NewEngine creates a new ADR engine.
func NewEngine(logger *slog.Logger, adrs adr.Repository, pubsub *redispkg.PubSub) *Engine {
	return &Engine{
		logger: logger,
		adrs:   adrs,
		pubsub: pubsub,
	}
}

// CreateADRInput is the JSON payload for a create_adr tool call.
type CreateADRInput struct {
	Title        string           `json:"title"`
	Context      string           `json:"context"`
	Decision     string           `json:"decision"`
	Consequences adr.Consequences `json:"consequences"`
	Options      json.RawMessage  `json:"options,omitempty"`
	Drivers      []string         `json:"drivers,omitempty"`
}

// HandleCreateADR processes a create_adr tool call from an agent.
func (e *Engine) HandleCreateADR(ctx context.Context, tenantID domain.TenantID, projectID domain.ProjectID, sessionID domain.AgentSessionID, input json.RawMessage) (*adr.ADR, error) {
	// Parse the input.
	var in CreateADRInput
	if err := json.Unmarshal(input, &in); err != nil {
		return nil, fmt.Errorf("adrengine.HandleCreateADR: parse input: %w", err)
	}

	// Validate required fields.
	if in.Title == "" {
		return nil, fmt.Errorf("adrengine.HandleCreateADR: title is required: %w", domain.ErrInvalidInput)
	}
	if in.Context == "" {
		return nil, fmt.Errorf("adrengine.HandleCreateADR: context is required: %w", domain.ErrInvalidInput)
	}
	if in.Decision == "" {
		return nil, fmt.Errorf("adrengine.HandleCreateADR: decision is required: %w", domain.ErrInvalidInput)
	}

	// Create the ADR domain entity.
	now := time.Now()
	record := &adr.ADR{
		ID:             domain.NewID(),
		TenantID:       tenantID,
		ProjectID:      projectID,
		AgentSessionID: &sessionID,
		Title:          in.Title,
		Status:         adr.StatusProposed,
		Context:        in.Context,
		Decision:       in.Decision,
		Consequences:   in.Consequences,
		Options:        in.Options,
		Drivers:        in.Drivers,
		CreatedAt:      now,
		UpdatedAt:      now,
	}

	// Persist the ADR with auto-assigned sequence number.
	if err := e.adrs.CreateWithNextSequence(ctx, record); err != nil {
		return nil, fmt.Errorf("adrengine.HandleCreateADR: %w", err)
	}

	// Log the creation.
	e.logger.InfoContext(ctx, "ADR created",
		slog.String("adr_id", record.ID.String()),
		slog.String("tenant_id", tenantID.String()),
		slog.String("project_id", projectID.String()),
		slog.String("session_id", sessionID.String()),
		slog.Int("sequence", record.Sequence),
		slog.String("title", in.Title),
	)

	return record, nil
}
