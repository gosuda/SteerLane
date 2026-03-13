package postgres

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/gosuda/steerlane/internal/auth"
	"github.com/gosuda/steerlane/internal/domain"
	"github.com/gosuda/steerlane/internal/store/postgres/sqlc"
)

type apiKeyRepository struct {
	store *Store
}

func (r *apiKeyRepository) Create(ctx context.Context, rec *auth.APIKeyRecord) error {
	if rec == nil {
		return fmt.Errorf("apikey create: nil record: %w", domain.ErrInvalidInput)
	}

	row, err := r.store.queries.CreateAPIKey(ctx, sqlc.CreateAPIKeyParams{
		TenantID:  rec.TenantID,
		UserID:    rec.UserID,
		Name:      rec.Label,
		KeyHash:   rec.Hash,
		Prefix:    rec.Prefix,
		Scopes:    []string{},
		ExpiresAt: pgtype.Timestamptz{Valid: false},
	})
	if err != nil {
		return fmt.Errorf("apikey create: %w", classifyError(err))
	}

	rec.ID = row.ID
	return nil
}

func (r *apiKeyRepository) GetByPrefix(ctx context.Context, prefix string) (*auth.APIKeyRecord, error) {
	if prefix == "" {
		return nil, fmt.Errorf("apikey get by prefix: empty prefix: %w", domain.ErrInvalidInput)
	}

	row, err := r.store.queries.GetAPIKeyByPrefix(ctx, prefix)
	if err != nil {
		return nil, fmt.Errorf("apikey get by prefix: %w", classifyError(err))
	}

	return mapAPIKeyRow(row), nil
}

func (r *apiKeyRepository) ListByUser(ctx context.Context, tenantID domain.TenantID, userID domain.UserID) ([]*auth.APIKeyRecord, error) {
	rows, err := r.store.queries.ListAPIKeysByUser(ctx, sqlc.ListAPIKeysByUserParams{
		UserID:   userID,
		TenantID: tenantID,
	})
	if err != nil {
		return nil, fmt.Errorf("apikey list by user: %w", classifyError(err))
	}

	result := make([]*auth.APIKeyRecord, 0, len(rows))
	for _, row := range rows {
		result = append(result, mapAPIKeyRow(row))
	}
	return result, nil
}

func (r *apiKeyRepository) Delete(ctx context.Context, tenantID domain.TenantID, id uuid.UUID) error {
	if err := requireUUID("apikey id", id); err != nil {
		return err
	}

	err := r.store.queries.DeleteAPIKey(ctx, sqlc.DeleteAPIKeyParams{
		ID:       id,
		TenantID: tenantID,
	})
	if err != nil {
		return fmt.Errorf("apikey delete: %w", classifyError(err))
	}
	return nil
}

func mapAPIKeyRow(row sqlc.ApiKey) *auth.APIKeyRecord {
	return &auth.APIKeyRecord{
		ID:       row.ID,
		TenantID: row.TenantID,
		UserID:   row.UserID,
		Prefix:   row.Prefix,
		Hash:     row.KeyHash,
		Label:    row.Name,
	}
}
