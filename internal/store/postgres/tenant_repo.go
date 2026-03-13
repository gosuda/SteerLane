package postgres

import (
	"context"
	"fmt"

	"github.com/gosuda/steerlane/internal/domain"
	"github.com/gosuda/steerlane/internal/domain/tenant"
	"github.com/gosuda/steerlane/internal/store/postgres/sqlc"
)

var _ tenant.Repository = (*tenantRepository)(nil)

type tenantRepository struct {
	store *Store
}

func mapTenantModel(row sqlc.Tenant) (*tenant.Tenant, error) {
	settings, err := unmarshalJSONObject(row.Settings)
	if err != nil {
		return nil, fmt.Errorf("decode tenant settings: %w", err)
	}

	entity := &tenant.Tenant{
		ID:        row.ID,
		Name:      row.Name,
		Slug:      row.Slug,
		Settings:  settings,
		CreatedAt: row.CreatedAt,
		UpdatedAt: row.UpdatedAt,
	}
	if err := entity.Validate(); err != nil { //nolint:govet // short-lived err shadow is idiomatic Go
		return nil, fmt.Errorf("map tenant: %w", err)
	}
	return entity, nil
}

func (r *tenantRepository) Create(ctx context.Context, record *tenant.Tenant) error {
	if record == nil {
		return fmt.Errorf("tenant: %w", domain.ErrInvalidInput)
	}
	if err := record.Validate(); err != nil {
		return err
	}

	settings, err := marshalJSONObject(record.Settings)
	if err != nil {
		return fmt.Errorf("marshal tenant settings: %w", err)
	}

	row, err := r.store.queries.CreateTenant(ctx, sqlc.CreateTenantParams{
		Name:     record.Name,
		Slug:     record.Slug,
		Settings: settings,
	})
	if err != nil {
		return fmt.Errorf("create tenant: %w", classifyError(err))
	}

	mapped, err := mapTenantModel(row)
	if err != nil {
		return err
	}
	*record = *mapped

	return nil
}

func (r *tenantRepository) GetByID(ctx context.Context, id domain.TenantID) (*tenant.Tenant, error) {
	if err := requireUUID("tenant id", id); err != nil {
		return nil, err
	}

	row, err := r.store.queries.GetTenantByID(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("get tenant by id: %w", classifyError(err))
	}

	entity, err := mapTenantModel(row)
	if err != nil {
		return nil, err
	}
	return entity, nil
}

func (r *tenantRepository) GetBySlug(ctx context.Context, slug string) (*tenant.Tenant, error) {
	if err := requireString("tenant slug", slug); err != nil {
		return nil, err
	}

	row, err := r.store.queries.GetTenantBySlug(ctx, slug)
	if err != nil {
		return nil, fmt.Errorf("get tenant by slug: %w", classifyError(err))
	}

	entity, err := mapTenantModel(row)
	if err != nil {
		return nil, err
	}
	return entity, nil
}

func (r *tenantRepository) Update(ctx context.Context, record *tenant.Tenant) error {
	if record == nil {
		return fmt.Errorf("tenant: %w", domain.ErrInvalidInput)
	}
	if err := requireUUID("tenant id", record.ID); err != nil {
		return err
	}
	if err := record.Validate(); err != nil {
		return err
	}

	settings, err := marshalJSONObject(record.Settings)
	if err != nil {
		return fmt.Errorf("marshal tenant settings: %w", err)
	}

	row, err := r.store.queries.UpdateTenant(ctx, sqlc.UpdateTenantParams{
		ID:       record.ID,
		Name:     record.Name,
		Slug:     record.Slug,
		Settings: settings,
	})
	if err != nil {
		return fmt.Errorf("update tenant: %w", classifyError(err))
	}

	mapped, err := mapTenantModel(row)
	if err != nil {
		return err
	}
	*record = *mapped

	return nil
}
