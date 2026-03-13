package adrengine_test

import (
	"context"
	"log/slog"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/gosuda/steerlane/internal/adrengine"
	"github.com/gosuda/steerlane/internal/domain"
	"github.com/gosuda/steerlane/internal/domain/adr"
)

// mockADRRepo implements adr.Repository for testing.
type mockADRRepo struct {
	adrs []*adr.ADR
}

func (m *mockADRRepo) CreateWithNextSequence(ctx context.Context, record *adr.ADR) error {
	record.Sequence = len(m.adrs) + 1
	m.adrs = append(m.adrs, record)
	return nil
}

func (m *mockADRRepo) GetByID(ctx context.Context, tenantID domain.TenantID, id domain.ADRID) (*adr.ADR, error) {
	return nil, nil // unused
}
func (m *mockADRRepo) ListByProject(ctx context.Context, tenantID domain.TenantID, projectID domain.ProjectID, limit int, cursor *domain.ADRID) ([]*adr.ADR, error) {
	return nil, nil // unused
}
func (m *mockADRRepo) Update(ctx context.Context, record *adr.ADR) error {
	return nil // unused
}
func (m *mockADRRepo) UpdateStatus(ctx context.Context, tenantID domain.TenantID, id domain.ADRID, status adr.ADRStatus) error {
	return nil // unused
}

func TestEngine_ExtractFromSession(t *testing.T) {
	logger := slog.Default() // You could use a test logger here.
	repo := &mockADRRepo{}
	engine := adrengine.NewEngine(logger, repo, nil)

	tenantID := domain.NewID()
	projectID := domain.NewID()
	sessionID := domain.NewID()

	t.Run("nil metadata", func(t *testing.T) {
		res, err := engine.ExtractFromSession(context.Background(), tenantID, projectID, sessionID, nil)
		require.NoError(t, err)
		assert.Empty(t, res)
	})

	t.Run("no adr markers", func(t *testing.T) {
		metadata := map[string]any{
			"summary": "This is a summary without any markers.",
			"other":   "some other value",
		}
		res, err := engine.ExtractFromSession(context.Background(), tenantID, projectID, sessionID, metadata)
		require.NoError(t, err)
		assert.Empty(t, res)
	})

	t.Run("with adr markers", func(t *testing.T) {
		metadata := map[string]any{
			"summary": "We did some work.\n[ADR] Using PostgreSQL for user data storage due to ACID requirements\nAnd more work.",
			"notes":   "[ADR] Adopted gRPC for inter-service communication",
		}

		res, err := engine.ExtractFromSession(context.Background(), tenantID, projectID, sessionID, metadata)
		require.NoError(t, err)
		assert.Len(t, res, 2)

		titles := []string{res[0].Title, res[1].Title}
		assert.Contains(t, titles, "Extracted: Using PostgreSQL for user data storage due to ACID...")
		assert.Contains(t, titles, "Extracted: Adopted gRPC for inter-service communication")
	})
}
