package user

import (
	"context"

	"github.com/gosuda/steerlane/internal/domain"
)

// Repository defines persistence operations for users.
type Repository interface {
	Create(ctx context.Context, user *User) error
	GetByID(ctx context.Context, tenantID domain.TenantID, id domain.UserID) (*User, error)
	GetByEmail(ctx context.Context, tenantID domain.TenantID, email string) (*User, error)
	ListByTenant(ctx context.Context, tenantID domain.TenantID, limit int, cursor string) ([]*User, error)
	Update(ctx context.Context, user *User) error
	Delete(ctx context.Context, tenantID domain.TenantID, id domain.UserID) error
}
