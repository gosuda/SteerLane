package tenant

import (
	"context"

	"github.com/gosuda/steerlane/internal/domain"
)

// Repository defines persistence operations for tenants.
type Repository interface {
	Create(ctx context.Context, tenant *Tenant) error
	GetByID(ctx context.Context, id domain.TenantID) (*Tenant, error)
	GetBySlug(ctx context.Context, slug string) (*Tenant, error)
	Update(ctx context.Context, tenant *Tenant) error
}
