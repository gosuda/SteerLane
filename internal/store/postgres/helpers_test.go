package postgres

import (
	"encoding/json"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/gosuda/steerlane/internal/domain"
)

func TestRequireUUID_Valid(t *testing.T) {
	t.Parallel()

	require.NoError(t, requireUUID("id", uuid.New()))
}

func TestRequireUUID_Nil(t *testing.T) {
	t.Parallel()

	err := requireUUID("id", uuid.Nil)
	require.Error(t, err)
	require.ErrorIs(t, err, domain.ErrInvalidInput)
	assert.Contains(t, err.Error(), "id")
}

func TestRequireString_Valid(t *testing.T) {
	t.Parallel()

	require.NoError(t, requireString("name", "steerlane"))
}

func TestRequireString_Empty(t *testing.T) {
	t.Parallel()

	err := requireString("name", "")
	require.Error(t, err)
	require.ErrorIs(t, err, domain.ErrInvalidInput)
	assert.Contains(t, err.Error(), "name")
}

func TestRequireString_Whitespace(t *testing.T) {
	t.Parallel()

	err := requireString("name", "   ")
	require.Error(t, err)
	require.ErrorIs(t, err, domain.ErrInvalidInput)
	assert.Contains(t, err.Error(), "name")
}

func TestClassifyError_Nil(t *testing.T) {
	t.Parallel()

	assert.NoError(t, classifyError(nil))
}

func TestClassifyError_NoRows(t *testing.T) {
	t.Parallel()

	assert.ErrorIs(t, classifyError(pgx.ErrNoRows), domain.ErrNotFound)
}

func TestClassifyError_UniqueViolation(t *testing.T) {
	t.Parallel()

	err := classifyError(&pgconn.PgError{Code: "23505", ConstraintName: "users_email_key"})
	require.Error(t, err)
	require.ErrorIs(t, err, domain.ErrConflict)
	assert.Contains(t, err.Error(), "users_email_key")
}

func TestClassifyError_ForeignKeyViolation(t *testing.T) {
	t.Parallel()

	err := classifyError(&pgconn.PgError{Code: "23503", ConstraintName: "tasks_project_id_fkey"})
	require.Error(t, err)
	require.ErrorIs(t, err, domain.ErrInvalidInput)
	assert.Contains(t, err.Error(), "tasks_project_id_fkey")
}

func TestClassifyError_UnknownError(t *testing.T) {
	t.Parallel()

	expectedErr := errors.New("boom")
	assert.Same(t, expectedErr, classifyError(expectedErr))
}

func TestNormalizeListLimit_Default(t *testing.T) {
	t.Parallel()

	assert.EqualValues(t, defaultListLimit, normalizeListLimit(0))
}

func TestNormalizeListLimit_Negative(t *testing.T) {
	t.Parallel()

	assert.EqualValues(t, defaultListLimit, normalizeListLimit(-1))
}

func TestNormalizeListLimit_Exceeds(t *testing.T) {
	t.Parallel()

	assert.EqualValues(t, maxListLimit, normalizeListLimit(500))
}

func TestNormalizeListLimit_Valid(t *testing.T) {
	t.Parallel()

	assert.EqualValues(t, 25, normalizeListLimit(25))
}

func TestMarshalJSONObject_Nil(t *testing.T) {
	t.Parallel()

	got, err := marshalJSONObject(nil)
	require.NoError(t, err)
	assert.JSONEq(t, `{}`, string(got))
}

func TestMarshalJSONObject_Data(t *testing.T) {
	t.Parallel()

	got, err := marshalJSONObject(map[string]any{"count": 2, "name": "steerlane"})
	require.NoError(t, err)
	assert.JSONEq(t, `{"count":2,"name":"steerlane"}`, string(got))
}

func TestUnmarshalJSONObject_Empty(t *testing.T) {
	t.Parallel()

	got, err := unmarshalJSONObject(json.RawMessage{})
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Empty(t, got)
}

func TestUnmarshalJSONObject_Data(t *testing.T) {
	t.Parallel()

	got, err := unmarshalJSONObject(json.RawMessage(`{"count":2,"name":"steerlane"}`))
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, "steerlane", got["name"])
	assert.EqualValues(t, 2, got["count"])
}
