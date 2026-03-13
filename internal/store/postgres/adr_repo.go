package postgres

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/gosuda/steerlane/internal/domain"
	"github.com/gosuda/steerlane/internal/domain/adr"
	"github.com/gosuda/steerlane/internal/store/postgres/sqlc"
)

var _ adr.Repository = (*adrRepository)(nil)

type adrRepository struct {
	store *Store
}

func normalizeConsequences(value adr.Consequences) adr.Consequences {
	if value.Good == nil {
		value.Good = []string{}
	}
	if value.Bad == nil {
		value.Bad = []string{}
	}
	if value.Neutral == nil {
		value.Neutral = []string{}
	}
	return value
}

func validateADRStatus(status adr.ADRStatus) error {
	switch status {
	case adr.StatusDraft, adr.StatusProposed, adr.StatusAccepted, adr.StatusRejected, adr.StatusDeprecated:
		return nil
	default:
		return fmt.Errorf("adr status %q: %w", status, domain.ErrInvalidInput)
	}
}

func marshalADRConsequences(value adr.Consequences) (json.RawMessage, error) {
	normalized := normalizeConsequences(value)
	encoded, err := json.Marshal(normalized)
	if err != nil {
		return nil, err
	}
	return encoded, nil
}

func mapADRModel(row sqlc.Adr) (*adr.ADR, error) {
	var consequences adr.Consequences
	if len(row.Consequences) != 0 {
		if err := json.Unmarshal(row.Consequences, &consequences); err != nil {
			return nil, fmt.Errorf("decode adr consequences: %w", err)
		}
	}

	entity := &adr.ADR{
		ID:             row.ID,
		TenantID:       row.TenantID,
		ProjectID:      row.ProjectID,
		Sequence:       int(row.Sequence),
		Title:          row.Title,
		Status:         adr.ADRStatus(row.Status),
		Context:        row.Context,
		Decision:       row.Decision,
		Drivers:        stringsOrEmpty(row.Drivers),
		Options:        rawJSONOrDefault(row.Options, "[]"),
		Consequences:   normalizeConsequences(consequences),
		CreatedBy:      uuidPtrFromPG(row.CreatedBy),
		AgentSessionID: uuidPtrFromPG(row.AgentSessionID),
		CreatedAt:      row.CreatedAt,
		UpdatedAt:      row.UpdatedAt,
	}
	if err := entity.Validate(); err != nil {
		return nil, fmt.Errorf("map adr: %w", err)
	}
	return entity, nil
}

func mapADRs(rows []sqlc.Adr) ([]*adr.ADR, error) {
	items := make([]*adr.ADR, 0, len(rows))
	for _, row := range rows {
		mapped, err := mapADRModel(row)
		if err != nil {
			return nil, err
		}
		items = append(items, mapped)
	}
	return items, nil
}

func (r *adrRepository) CreateWithNextSequence(ctx context.Context, record *adr.ADR) error {
	if record == nil {
		return fmt.Errorf("adr: %w", domain.ErrInvalidInput)
	}
	if err := requireUUID("adr tenant id", record.TenantID); err != nil {
		return err
	}
	if err := requireUUID("adr project id", record.ProjectID); err != nil {
		return err
	}
	if record.Sequence != 0 {
		return fmt.Errorf("adr sequence must be unset: %w", domain.ErrInvalidInput)
	}

	validationCopy := *record
	validationCopy.Sequence = 1
	validationCopy.Consequences = normalizeConsequences(validationCopy.Consequences)
	if err := validationCopy.Validate(); err != nil {
		return err
	}

	consequences, err := marshalADRConsequences(record.Consequences)
	if err != nil {
		return fmt.Errorf("marshal adr consequences: %w", err)
	}

	var created *adr.ADR
	err = r.store.withinTx(ctx, pgx.TxOptions{IsoLevel: pgx.Serializable}, func(q *sqlc.Queries) error {
		if _, err := q.LockProjectForUpdate(ctx, sqlc.LockProjectForUpdateParams{ID: record.ProjectID, TenantID: record.TenantID}); err != nil { //nolint:govet // short-lived err shadow is idiomatic Go
			return fmt.Errorf("lock project for adr sequence: %w", classifyError(err))
		}

		row, err := q.CreateADRWithNextSequence(ctx, sqlc.CreateADRWithNextSequenceParams{ //nolint:govet // short-lived err shadow is idiomatic Go
			TenantID:       record.TenantID,
			ProjectID:      record.ProjectID,
			Title:          record.Title,
			Status:         string(record.Status),
			Context:        record.Context,
			Decision:       record.Decision,
			Drivers:        stringsOrEmpty(record.Drivers),
			Options:        rawJSONOrDefault(record.Options, "[]"),
			Consequences:   consequences,
			CreatedBy:      pgUUIDFromPtr(record.CreatedBy),
			AgentSessionID: pgUUIDFromPtr(record.AgentSessionID),
		})
		if err != nil {
			return fmt.Errorf("create adr with next sequence: %w", classifyError(err))
		}

		created, err = mapADRModel(row)
		if err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		return err
	}

	*record = *created
	return nil
}

func (r *adrRepository) GetByID(ctx context.Context, tenantID domain.TenantID, id domain.ADRID) (*adr.ADR, error) {
	if err := requireUUID("tenant id", tenantID); err != nil {
		return nil, err
	}
	if err := requireUUID("adr id", id); err != nil {
		return nil, err
	}

	row, err := r.store.queries.GetADRByID(ctx, sqlc.GetADRByIDParams{ID: id, TenantID: tenantID})
	if err != nil {
		return nil, fmt.Errorf("get adr by id: %w", classifyError(err))
	}

	entity, err := mapADRModel(row)
	if err != nil {
		return nil, err
	}
	return entity, nil
}

func (r *adrRepository) ListByProject(ctx context.Context, tenantID domain.TenantID, projectID domain.ProjectID, limit int, cursor *uuid.UUID) ([]*adr.ADR, error) {
	if err := requireUUID("tenant id", tenantID); err != nil {
		return nil, err
	}
	if err := requireUUID("project id", projectID); err != nil {
		return nil, err
	}

	listLimit := normalizeListLimit(limit)
	if cursor == nil {
		rows, err := r.store.queries.ListADRsByProject(ctx, sqlc.ListADRsByProjectParams{ProjectID: projectID, TenantID: tenantID, Limit: listLimit})
		if err != nil {
			return nil, fmt.Errorf("list adrs by project: %w", classifyError(err))
		}
		return mapADRs(rows)
	}

	if err := requireUUID("adr cursor", *cursor); err != nil {
		return nil, err
	}

	cursorEntity, err := r.GetByID(ctx, tenantID, *cursor)
	if err != nil {
		return nil, fmt.Errorf("load adr cursor: %w", err)
	}
	if cursorEntity.ProjectID != projectID {
		return nil, fmt.Errorf("adr cursor project mismatch: %w", domain.ErrInvalidInput)
	}

	rows, err := r.store.queries.ListADRsByProjectAfter(ctx, sqlc.ListADRsByProjectAfterParams{
		ProjectID: projectID,
		TenantID:  tenantID,
		Sequence:  int32(cursorEntity.Sequence), //nolint:gosec // G115: sequence validated >= 1 by domain model
		ID:        cursorEntity.ID,
		Limit:     listLimit,
	})
	if err != nil {
		return nil, fmt.Errorf("list adrs by project after cursor: %w", classifyError(err))
	}
	return mapADRs(rows)
}

func (r *adrRepository) UpdateStatus(ctx context.Context, tenantID domain.TenantID, id domain.ADRID, status adr.ADRStatus) error {
	if err := requireUUID("tenant id", tenantID); err != nil {
		return err
	}
	if err := requireUUID("adr id", id); err != nil {
		return err
	}
	if err := validateADRStatus(status); err != nil {
		return err
	}

	current, err := r.GetByID(ctx, tenantID, id)
	if err != nil {
		return err
	}

	updated := *current
	if err := updated.Transition(status, time.Now().UTC()); err != nil { //nolint:govet // short-lived err shadow is idiomatic Go
		return err
	}

	if _, err := r.store.queries.UpdateADRStatus(ctx, sqlc.UpdateADRStatusParams{ID: id, Status: string(status), TenantID: tenantID}); err != nil { //nolint:govet // short-lived err shadow is idiomatic Go
		return fmt.Errorf("update adr status: %w", classifyError(err))
	}
	return nil
}
