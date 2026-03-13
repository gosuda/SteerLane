package postgres

import (
	"context"
	"fmt"

	"github.com/google/uuid"

	"github.com/gosuda/steerlane/internal/domain"
	"github.com/gosuda/steerlane/internal/domain/audit"
	"github.com/gosuda/steerlane/internal/store/postgres/sqlc"
)

var _ audit.Repository = (*auditRepository)(nil)

type auditRepository struct {
	store *Store
}

func mapAuditModel(row sqlc.AuditLog) (*audit.Entry, error) {
	actorID, err := uuid.Parse(row.ActorID)
	if err != nil {
		return nil, fmt.Errorf("parse audit actor id: %w", err)
	}
	resourceID, err := uuid.Parse(row.ResourceID)
	if err != nil {
		return nil, fmt.Errorf("parse audit resource id: %w", err)
	}
	details, err := unmarshalJSONObject(row.Details)
	if err != nil {
		return nil, fmt.Errorf("decode audit details: %w", err)
	}

	entity := &audit.Entry{
		ID:         row.ID,
		TenantID:   row.TenantID,
		ActorType:  audit.ActorType(row.ActorType),
		ActorID:    actorID,
		Action:     row.Action,
		Resource:   row.Resource,
		ResourceID: resourceID,
		Details:    details,
		CreatedAt:  row.CreatedAt,
	}
	if err := entity.Validate(); err != nil { //nolint:govet // short-lived err shadow is idiomatic Go
		return nil, fmt.Errorf("map audit entry: %w", err)
	}
	return entity, nil
}

func mapAuditEntries(rows []sqlc.AuditLog) ([]*audit.Entry, error) {
	items := make([]*audit.Entry, 0, len(rows))
	for _, row := range rows {
		mapped, err := mapAuditModel(row)
		if err != nil {
			return nil, err
		}
		items = append(items, mapped)
	}
	return items, nil
}

func (r *auditRepository) Append(ctx context.Context, record *audit.Entry) error {
	if record == nil {
		return fmt.Errorf("audit entry: %w", domain.ErrInvalidInput)
	}
	if err := requireUUID("audit tenant id", record.TenantID); err != nil {
		return err
	}
	if err := requireUUID("audit actor id", record.ActorID); err != nil {
		return err
	}
	if err := requireUUID("audit resource id", record.ResourceID); err != nil {
		return err
	}
	if err := record.Validate(); err != nil {
		return err
	}

	details, err := marshalJSONObject(record.Details)
	if err != nil {
		return fmt.Errorf("marshal audit details: %w", err)
	}

	row, err := r.store.queries.AppendAuditEntry(ctx, sqlc.AppendAuditEntryParams{
		TenantID:   record.TenantID,
		ActorType:  string(record.ActorType),
		ActorID:    record.ActorID.String(),
		Action:     record.Action,
		Resource:   record.Resource,
		ResourceID: record.ResourceID.String(),
		Details:    details,
	})
	if err != nil {
		return fmt.Errorf("append audit entry: %w", classifyError(err))
	}

	mapped, err := mapAuditModel(row)
	if err != nil {
		return err
	}
	*record = *mapped

	return nil
}

func (r *auditRepository) ListByTenant(ctx context.Context, tenantID domain.TenantID, limit int, cursor *uuid.UUID) ([]*audit.Entry, error) {
	if err := requireUUID("tenant id", tenantID); err != nil {
		return nil, err
	}

	listLimit := normalizeListLimit(limit)
	if cursor == nil {
		rows, err := r.store.queries.ListAuditByTenant(ctx, sqlc.ListAuditByTenantParams{TenantID: tenantID, Limit: listLimit})
		if err != nil {
			return nil, fmt.Errorf("list audit by tenant: %w", classifyError(err))
		}
		return mapAuditEntries(rows)
	}

	if err := requireUUID("audit cursor", *cursor); err != nil {
		return nil, err
	}

	cursorRow, err := r.store.queries.GetAuditEntryByID(ctx, sqlc.GetAuditEntryByIDParams{ID: *cursor, TenantID: tenantID})
	if err != nil {
		return nil, fmt.Errorf("load audit cursor: %w", classifyError(err))
	}

	rows, err := r.store.queries.ListAuditByTenantAfter(ctx, sqlc.ListAuditByTenantAfterParams{
		TenantID:  tenantID,
		CreatedAt: cursorRow.CreatedAt,
		ID:        cursorRow.ID,
		Limit:     listLimit,
	})
	if err != nil {
		return nil, fmt.Errorf("list audit by tenant after cursor: %w", classifyError(err))
	}
	return mapAuditEntries(rows)
}

func (r *auditRepository) ListByResource(ctx context.Context, tenantID domain.TenantID, resource string, resourceID uuid.UUID, limit int, cursor *uuid.UUID) ([]*audit.Entry, error) {
	if err := requireUUID("tenant id", tenantID); err != nil {
		return nil, err
	}
	if err := requireString("audit resource", resource); err != nil {
		return nil, err
	}
	if err := requireUUID("audit resource id", resourceID); err != nil {
		return nil, err
	}

	listLimit := normalizeListLimit(limit)
	if cursor == nil {
		rows, err := r.store.queries.ListAuditByResource(ctx, sqlc.ListAuditByResourceParams{
			TenantID:   tenantID,
			Resource:   resource,
			ResourceID: resourceID.String(),
			Limit:      listLimit,
		})
		if err != nil {
			return nil, fmt.Errorf("list audit by resource: %w", classifyError(err))
		}
		return mapAuditEntries(rows)
	}

	if err := requireUUID("audit cursor", *cursor); err != nil {
		return nil, err
	}

	cursorRow, err := r.store.queries.GetAuditEntryByID(ctx, sqlc.GetAuditEntryByIDParams{ID: *cursor, TenantID: tenantID})
	if err != nil {
		return nil, fmt.Errorf("load audit cursor: %w", classifyError(err))
	}
	if cursorRow.Resource != resource || cursorRow.ResourceID != resourceID.String() {
		return nil, fmt.Errorf("audit cursor resource mismatch: %w", domain.ErrInvalidInput)
	}

	rows, err := r.store.queries.ListAuditByResourceAfter(ctx, sqlc.ListAuditByResourceAfterParams{
		TenantID:   tenantID,
		Resource:   resource,
		ResourceID: resourceID.String(),
		CreatedAt:  cursorRow.CreatedAt,
		ID:         cursorRow.ID,
		Limit:      listLimit,
	})
	if err != nil {
		return nil, fmt.Errorf("list audit by resource after cursor: %w", classifyError(err))
	}
	return mapAuditEntries(rows)
}
