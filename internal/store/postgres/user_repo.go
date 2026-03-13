package postgres

import (
	"context"
	"fmt"
	"strings"

	"github.com/google/uuid"

	"github.com/gosuda/steerlane/internal/domain"
	"github.com/gosuda/steerlane/internal/domain/user"
	"github.com/gosuda/steerlane/internal/store/postgres/sqlc"
)

var _ user.Repository = (*userRepository)(nil)

type userRepository struct {
	store *Store
}

func mapUserModel(row sqlc.User) (*user.User, error) {
	entity := &user.User{
		ID:           row.ID,
		TenantID:     row.TenantID,
		Email:        row.Email,
		PasswordHash: row.PasswordHash,
		Name:         row.Name,
		Role:         user.Role(row.Role),
		AvatarURL:    row.AvatarUrl,
		CreatedAt:    row.CreatedAt,
		UpdatedAt:    row.UpdatedAt,
	}
	if err := entity.Validate(); err != nil {
		return nil, fmt.Errorf("map user: %w", err)
	}
	return entity, nil
}

func mapUsers(rows []sqlc.User) ([]*user.User, error) {
	items := make([]*user.User, 0, len(rows))
	for _, row := range rows {
		mapped, err := mapUserModel(row)
		if err != nil {
			return nil, err
		}
		items = append(items, mapped)
	}
	return items, nil
}

func (r *userRepository) Create(ctx context.Context, record *user.User) error {
	if record == nil {
		return fmt.Errorf("user: %w", domain.ErrInvalidInput)
	}
	if err := requireUUID("user tenant id", record.TenantID); err != nil {
		return err
	}
	if err := record.Validate(); err != nil {
		return err
	}

	row, err := r.store.queries.CreateUser(ctx, sqlc.CreateUserParams{
		TenantID:     record.TenantID,
		Email:        record.Email,
		PasswordHash: record.PasswordHash,
		Name:         record.Name,
		Role:         string(record.Role),
		AvatarUrl:    record.AvatarURL,
	})
	if err != nil {
		return fmt.Errorf("create user: %w", classifyError(err))
	}

	mapped, err := mapUserModel(row)
	if err != nil {
		return err
	}
	*record = *mapped

	return nil
}

func (r *userRepository) GetByID(ctx context.Context, tenantID domain.TenantID, id domain.UserID) (*user.User, error) {
	if err := requireUUID("tenant id", tenantID); err != nil {
		return nil, err
	}
	if err := requireUUID("user id", id); err != nil {
		return nil, err
	}

	row, err := r.store.queries.GetUserByID(ctx, sqlc.GetUserByIDParams{ID: id, TenantID: tenantID})
	if err != nil {
		return nil, fmt.Errorf("get user by id: %w", classifyError(err))
	}

	entity, err := mapUserModel(row)
	if err != nil {
		return nil, err
	}
	return entity, nil
}

func (r *userRepository) GetByEmail(ctx context.Context, tenantID domain.TenantID, email string) (*user.User, error) {
	if err := requireUUID("tenant id", tenantID); err != nil {
		return nil, err
	}
	if err := requireString("user email", email); err != nil {
		return nil, err
	}

	row, err := r.store.queries.GetUserByEmail(ctx, sqlc.GetUserByEmailParams{
		Email:    &email,
		TenantID: tenantID,
	})
	if err != nil {
		return nil, fmt.Errorf("get user by email: %w", classifyError(err))
	}

	entity, err := mapUserModel(row)
	if err != nil {
		return nil, err
	}
	return entity, nil
}

func (r *userRepository) ListByTenant(ctx context.Context, tenantID domain.TenantID, limit int, cursor string) ([]*user.User, error) {
	if err := requireUUID("tenant id", tenantID); err != nil {
		return nil, err
	}

	normalizedLimit := normalizeListLimit(limit)
	trimmedCursor := strings.TrimSpace(cursor)
	if trimmedCursor != "" {
		cursorID, err := uuid.Parse(trimmedCursor)
		if err != nil {
			return nil, fmt.Errorf("user cursor: %w", domain.ErrInvalidInput)
		}

		rows, err := r.store.queries.ListUsersByTenantAfterCursor(ctx, sqlc.ListUsersByTenantAfterCursorParams{
			TenantID: tenantID,
			ID:       cursorID,
			Limit:    normalizedLimit,
		})
		if err != nil {
			return nil, fmt.Errorf("list users by tenant (cursor): %w", classifyError(err))
		}

		return mapUsers(rows)
	}

	rows, err := r.store.queries.ListUsersByTenant(ctx, sqlc.ListUsersByTenantParams{
		TenantID: tenantID,
		Limit:    normalizedLimit,
	})
	if err != nil {
		return nil, fmt.Errorf("list users by tenant: %w", classifyError(err))
	}

	return mapUsers(rows)
}

func (r *userRepository) Update(ctx context.Context, record *user.User) error {
	if record == nil {
		return fmt.Errorf("user: %w", domain.ErrInvalidInput)
	}
	if err := requireUUID("user tenant id", record.TenantID); err != nil {
		return err
	}
	if err := requireUUID("user id", record.ID); err != nil {
		return err
	}
	if err := record.Validate(); err != nil {
		return err
	}

	row, err := r.store.queries.UpdateUser(ctx, sqlc.UpdateUserParams{
		ID:           record.ID,
		Email:        record.Email,
		PasswordHash: record.PasswordHash,
		Name:         record.Name,
		Role:         string(record.Role),
		AvatarUrl:    record.AvatarURL,
		TenantID:     record.TenantID,
	})
	if err != nil {
		return fmt.Errorf("update user: %w", classifyError(err))
	}

	mapped, err := mapUserModel(row)
	if err != nil {
		return err
	}
	*record = *mapped

	return nil
}

func (r *userRepository) Delete(ctx context.Context, tenantID domain.TenantID, id domain.UserID) error {
	if err := requireUUID("tenant id", tenantID); err != nil {
		return err
	}
	if err := requireUUID("user id", id); err != nil {
		return err
	}

	deleted, err := r.store.queries.DeleteUser(ctx, sqlc.DeleteUserParams{ID: id, TenantID: tenantID})
	if err != nil {
		return fmt.Errorf("delete user: %w", classifyError(err))
	}
	if deleted == 0 {
		return domain.ErrNotFound
	}
	return nil
}
