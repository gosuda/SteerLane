package user

import (
	"fmt"
	"net/mail"
	"time"

	"github.com/gosuda/steerlane/internal/domain"
)

// Role represents a user's role within a tenant.
type Role string

const (
	RoleAdmin  Role = "admin"
	RoleMember Role = "member"
)

// User represents a human participant in the system.
type User struct {
	CreatedAt    time.Time
	UpdatedAt    time.Time
	Email        *string
	PasswordHash *string
	AvatarURL    *string
	Name         string
	Role         Role
	ID           domain.UserID
	TenantID     domain.TenantID
}

// Validate checks that the user's fields are well-formed.
func (u *User) Validate() error {
	if u.Name == "" {
		return fmt.Errorf("user name: %w", domain.ErrInvalidInput)
	}
	if u.Email != nil {
		if _, err := mail.ParseAddress(*u.Email); err != nil {
			return fmt.Errorf("user email %q: %w", *u.Email, domain.ErrInvalidInput)
		}
	}
	switch u.Role {
	case RoleAdmin, RoleMember:
	default:
		return fmt.Errorf("user role %q: %w", u.Role, domain.ErrInvalidInput)
	}
	return nil
}
