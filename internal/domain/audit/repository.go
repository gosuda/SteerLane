package audit

import (
	"context"

	"github.com/google/uuid"

	"github.com/gosuda/steerlane/internal/domain"
)

// Repository defines persistence operations for audit entries.
// Audit entries are append-only; no update or delete.
type Repository interface {
	Append(ctx context.Context, entry *Entry) error
	ListByTenant(ctx context.Context, tenantID domain.TenantID, limit int, cursor *uuid.UUID) ([]*Entry, error)
	ListByResource(ctx context.Context, tenantID domain.TenantID, resource string, resourceID uuid.UUID, limit int, cursor *uuid.UUID) ([]*Entry, error)
}
