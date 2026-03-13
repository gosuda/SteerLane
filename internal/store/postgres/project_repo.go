package postgres

import (
	"context"
	"fmt"

	"github.com/google/uuid"

	"github.com/gosuda/steerlane/internal/domain"
	"github.com/gosuda/steerlane/internal/domain/project"
	"github.com/gosuda/steerlane/internal/store/postgres/sqlc"
)

var _ project.Repository = (*projectRepository)(nil)

type projectRepository struct {
	store *Store
}

func mapProjectModel(row sqlc.Project) (*project.Project, error) {
	settings, err := unmarshalJSONObject(row.Settings)
	if err != nil {
		return nil, fmt.Errorf("decode project settings: %w", err)
	}

	entity := &project.Project{
		ID:        row.ID,
		TenantID:  row.TenantID,
		Name:      row.Name,
		RepoURL:   row.RepoUrl,
		Branch:    row.Branch,
		Settings:  settings,
		CreatedAt: row.CreatedAt,
	}
	if err := entity.Validate(); err != nil { //nolint:govet // short-lived err shadow is idiomatic Go
		return nil, fmt.Errorf("map project: %w", err)
	}
	return entity, nil
}

func mapProjects(rows []sqlc.Project) ([]*project.Project, error) {
	projects := make([]*project.Project, 0, len(rows))
	for _, row := range rows {
		mapped, err := mapProjectModel(row)
		if err != nil {
			return nil, err
		}
		projects = append(projects, mapped)
	}
	return projects, nil
}

func (r *projectRepository) Create(ctx context.Context, record *project.Project) error {
	if record == nil {
		return fmt.Errorf("project: %w", domain.ErrInvalidInput)
	}
	if err := requireUUID("project tenant id", record.TenantID); err != nil {
		return err
	}
	if err := record.Validate(); err != nil {
		return err
	}

	settings, err := marshalJSONObject(record.Settings)
	if err != nil {
		return fmt.Errorf("marshal project settings: %w", err)
	}

	row, err := r.store.queries.CreateProject(ctx, sqlc.CreateProjectParams{
		TenantID: record.TenantID,
		Name:     record.Name,
		RepoUrl:  record.RepoURL,
		Branch:   record.Branch,
		Settings: settings,
	})
	if err != nil {
		return fmt.Errorf("create project: %w", classifyError(err))
	}

	mapped, err := mapProjectModel(row)
	if err != nil {
		return err
	}
	*record = *mapped

	return nil
}

func (r *projectRepository) GetByID(ctx context.Context, tenantID domain.TenantID, id domain.ProjectID) (*project.Project, error) {
	if err := requireUUID("tenant id", tenantID); err != nil {
		return nil, err
	}
	if err := requireUUID("project id", id); err != nil {
		return nil, err
	}

	row, err := r.store.queries.GetProjectByID(ctx, sqlc.GetProjectByIDParams{ID: id, TenantID: tenantID})
	if err != nil {
		return nil, fmt.Errorf("get project by id: %w", classifyError(err))
	}

	entity, err := mapProjectModel(row)
	if err != nil {
		return nil, err
	}
	return entity, nil
}

func (r *projectRepository) Update(ctx context.Context, record *project.Project) error {
	if record == nil {
		return fmt.Errorf("project: %w", domain.ErrInvalidInput)
	}
	if err := requireUUID("project tenant id", record.TenantID); err != nil {
		return err
	}
	if err := requireUUID("project id", record.ID); err != nil {
		return err
	}
	if err := record.Validate(); err != nil {
		return err
	}

	settings, err := marshalJSONObject(record.Settings)
	if err != nil {
		return fmt.Errorf("marshal project settings: %w", err)
	}

	row, err := r.store.queries.UpdateProject(ctx, sqlc.UpdateProjectParams{
		ID:       record.ID,
		Name:     record.Name,
		RepoUrl:  record.RepoURL,
		Branch:   record.Branch,
		Settings: settings,
		TenantID: record.TenantID,
	})
	if err != nil {
		return fmt.Errorf("update project: %w", classifyError(err))
	}

	mapped, err := mapProjectModel(row)
	if err != nil {
		return err
	}
	*record = *mapped

	return nil
}

func (r *projectRepository) Delete(ctx context.Context, tenantID domain.TenantID, id domain.ProjectID) error {
	if err := requireUUID("tenant id", tenantID); err != nil {
		return err
	}
	if err := requireUUID("project id", id); err != nil {
		return err
	}

	deleted, err := r.store.queries.DeleteProject(ctx, sqlc.DeleteProjectParams{ID: id, TenantID: tenantID})
	if err != nil {
		return fmt.Errorf("delete project: %w", classifyError(err))
	}
	if deleted == 0 {
		return domain.ErrNotFound
	}
	return nil
}

func (r *projectRepository) ListByTenant(ctx context.Context, tenantID domain.TenantID, limit int, cursor *uuid.UUID) ([]*project.Project, error) {
	if err := requireUUID("tenant id", tenantID); err != nil {
		return nil, err
	}

	listLimit := normalizeListLimit(limit)
	if cursor == nil {
		rows, err := r.store.queries.ListProjectsByTenant(ctx, sqlc.ListProjectsByTenantParams{TenantID: tenantID, Limit: listLimit})
		if err != nil {
			return nil, fmt.Errorf("list projects by tenant: %w", classifyError(err))
		}
		return mapProjects(rows)
	}

	if err := requireUUID("project cursor", *cursor); err != nil {
		return nil, err
	}

	cursorRow, err := r.store.queries.GetProjectByID(ctx, sqlc.GetProjectByIDParams{ID: *cursor, TenantID: tenantID})
	if err != nil {
		return nil, fmt.Errorf("load project cursor: %w", classifyError(err))
	}

	rows, err := r.store.queries.ListProjectsByTenantAfter(ctx, sqlc.ListProjectsByTenantAfterParams{
		TenantID:  tenantID,
		CreatedAt: cursorRow.CreatedAt,
		ID:        cursorRow.ID,
		Limit:     listLimit,
	})
	if err != nil {
		return nil, fmt.Errorf("list projects by tenant after cursor: %w", classifyError(err))
	}

	return mapProjects(rows)
}
