package adrengine

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/gosuda/steerlane/internal/domain"
	"github.com/gosuda/steerlane/internal/domain/adr"
)

// ExtractFromSession attempts to extract architectural decisions from available
// session metadata. It acts as a conservative heuristic extractor, only identifying
// decisions if explicit markers (like "[ADR]") are present.
// It returns a list of created ADRs, or nil if none were found.
func (e *Engine) ExtractFromSession(ctx context.Context, tenantID domain.TenantID, projectID domain.ProjectID, sessionID domain.AgentSessionID, metadata map[string]any) ([]*adr.ADR, error) {
	if metadata == nil {
		return nil, nil
	}

	// For now, we only look at a known "summary" or "transcript" field if they exist.
	// Since full transcript persistence may not exist yet, we check available strings.
	var textsToScan []string

	for key, val := range metadata {
		if key == "summary" || key == "transcript" || key == "notes" {
			if s, ok := val.(string); ok {
				textsToScan = append(textsToScan, s)
			}
		}
	}

	if len(textsToScan) == 0 {
		return nil, nil
	}

	var extracted []*adr.ADR

	for _, text := range textsToScan {
		adrs, err := e.extractDecisionsFromText(ctx, tenantID, projectID, sessionID, text)
		if err != nil {
			return nil, fmt.Errorf("adrengine.ExtractFromSession: %w", err)
		}
		extracted = append(extracted, adrs...)
	}

	return extracted, nil
}

// Extract specific [ADR] ... patterns heuristically.
var adrPattern = regexp.MustCompile(`(?i)\[ADR\]\s+(.*?)(?:\n|$)`)

func (e *Engine) extractDecisionsFromText(ctx context.Context, tenantID domain.TenantID, projectID domain.ProjectID, sessionID domain.AgentSessionID, text string) ([]*adr.ADR, error) {
	matches := adrPattern.FindAllStringSubmatch(text, -1)
	if len(matches) == 0 {
		return nil, nil
	}

	var results []*adr.ADR
	now := time.Now().UTC()

	for _, match := range matches {
		if len(match) < 2 {
			continue
		}
		decisionText := strings.TrimSpace(match[1])
		if decisionText == "" {
			continue
		}

		record := &adr.ADR{
			ID:             domain.NewID(),
			TenantID:       tenantID,
			ProjectID:      projectID,
			AgentSessionID: &sessionID,
			Title:          "Extracted: " + truncate(decisionText, 50),
			Status:         adr.StatusProposed,
			Context:        "Extracted from session metadata",
			Decision:       decisionText,
			CreatedAt:      now,
			UpdatedAt:      now,
		}

		if err := e.adrs.CreateWithNextSequence(ctx, record); err != nil {
			// If one fails, we can return the error, but we might want to continue.
			// However, since we return an error, let's just abort.
			return results, fmt.Errorf("save extracted ADR: %w", err)
		}

		e.logger.InfoContext(ctx, "Heuristic ADR extracted",
			"adr_id", record.ID.String(),
			"session_id", sessionID.String(),
			"sequence", record.Sequence,
			"title", record.Title,
		)

		results = append(results, record)
	}

	return results, nil
}

func truncate(s string, maxLength int) string {
	if len(s) <= maxLength {
		return s
	}
	return s[:maxLength] + "..."
}
