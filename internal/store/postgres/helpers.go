package postgres

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/gosuda/steerlane/internal/domain"
)

const (
	defaultListLimit = 50
	maxListLimit     = 200

	pgErrUniqueViolation           = "23505"
	pgErrForeignKeyViolation       = "23503"
	pgErrCheckViolation            = "23514"
	pgErrNotNullViolation          = "23502"
	pgErrInvalidTextRepresentation = "22P02"
)

func normalizeListLimit(limit int) int32 {
	switch {
	case limit <= 0:
		return defaultListLimit
	case limit > maxListLimit:
		return maxListLimit
	default:
		return int32(limit)
	}
}

func requireUUID(name string, id uuid.UUID) error {
	if id == uuid.Nil {
		return fmt.Errorf("%s: %w", name, domain.ErrInvalidInput)
	}
	return nil
}

func requireString(name, value string) error {
	if strings.TrimSpace(value) == "" {
		return fmt.Errorf("%s: %w", name, domain.ErrInvalidInput)
	}
	return nil
}

func classifyError(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.ErrNotFound
	}

	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) {
		return err
	}

	detail := pgErr.ConstraintName
	if detail == "" {
		detail = pgErr.Message
	}

	switch pgErr.Code {
	case pgErrUniqueViolation:
		if detail == "" {
			return domain.ErrConflict
		}
		return fmt.Errorf("%s: %w", detail, domain.ErrConflict)
	case pgErrForeignKeyViolation, pgErrCheckViolation, pgErrNotNullViolation, pgErrInvalidTextRepresentation:
		if detail == "" {
			return domain.ErrInvalidInput
		}
		return fmt.Errorf("%s: %w", detail, domain.ErrInvalidInput)
	default:
		return err
	}
}

func marshalJSONObject(value map[string]any) (json.RawMessage, error) {
	if value == nil {
		return json.RawMessage("{}"), nil
	}
	encoded, err := json.Marshal(value)
	if err != nil {
		return nil, err
	}
	return encoded, nil
}

func unmarshalJSONObject(value json.RawMessage) (map[string]any, error) {
	if len(value) == 0 {
		return map[string]any{}, nil
	}

	var decoded map[string]any
	if err := json.Unmarshal(value, &decoded); err != nil {
		return nil, err
	}
	if decoded == nil {
		decoded = map[string]any{}
	}
	return decoded, nil
}

func rawJSONOrDefault(value json.RawMessage, fallback string) json.RawMessage {
	if len(value) == 0 {
		return json.RawMessage(fallback)
	}
	copyValue := make(json.RawMessage, len(value))
	copy(copyValue, value)
	return copyValue
}

func stringsOrEmpty(value []string) []string {
	if value == nil {
		return []string{}
	}
	return append([]string(nil), value...)
}

func pgUUIDFromPtr(value *uuid.UUID) pgtype.UUID {
	if value == nil {
		return pgtype.UUID{}
	}
	return pgtype.UUID{Bytes: *value, Valid: true}
}

func uuidPtrFromPG(value pgtype.UUID) *uuid.UUID {
	if !value.Valid {
		return nil
	}
	id := uuid.UUID(value.Bytes)
	return &id
}

func pgTimestamptzFromPtr(value *time.Time) pgtype.Timestamptz {
	if value == nil {
		return pgtype.Timestamptz{}
	}
	return pgtype.Timestamptz{Time: *value, Valid: true}
}

func timePtrFromPG(value pgtype.Timestamptz) *time.Time {
	if !value.Valid {
		return nil
	}
	t := value.Time
	return &t
}
