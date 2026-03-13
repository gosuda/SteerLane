package project

import (
	"context"

	"github.com/google/uuid"

	"github.com/gosuda/steerlane/internal/domain"
)

// Repository defines persistence operations for projects.
type Repository interface {
	Create(ctx context.Context, project *Project) error
	GetByID(ctx context.Context, tenantID domain.TenantID, id domain.ProjectID) (*Project, error)
	Update(ctx context.Context, project *Project) error
	Delete(ctx context.Context, tenantID domain.TenantID, id domain.ProjectID) error
	ListByTenant(ctx context.Context, tenantID domain.TenantID, limit int, cursor *uuid.UUID) ([]*Project, error)
}
