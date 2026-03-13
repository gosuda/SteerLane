package adr

import (
	"context"

	"github.com/google/uuid"

	"github.com/gosuda/steerlane/internal/domain"
)

// Repository defines persistence operations for ADRs.
type Repository interface {
	// CreateWithNextSequence persists a new ADR, auto-assigning the next
	// sequence number within the project.
	CreateWithNextSequence(ctx context.Context, adr *ADR) error
	GetByID(ctx context.Context, tenantID domain.TenantID, id domain.ADRID) (*ADR, error)
	ListByProject(ctx context.Context, tenantID domain.TenantID, projectID domain.ProjectID, limit int, cursor *uuid.UUID) ([]*ADR, error)
	UpdateStatus(ctx context.Context, tenantID domain.TenantID, id domain.ADRID, status ADRStatus) error
}
