package postgres

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/gosuda/steerlane/internal/domain"
	"github.com/gosuda/steerlane/internal/store/postgres/sqlc"
)

func (s *Store) withinTx(ctx context.Context, opts pgx.TxOptions, fn func(*sqlc.Queries) error) (err error) {
	if s == nil || s.pool == nil {
		return fmt.Errorf("postgres store: %w", domain.ErrDatabaseUnavailable)
	}

	tx, err := s.pool.BeginTx(ctx, opts)
	if err != nil {
		return fmt.Errorf("begin transaction: %w", classifyError(err))
	}

	committed := false
	defer func() {
		if committed {
			return
		}
		rollbackErr := tx.Rollback(ctx)
		if rollbackErr == nil || errors.Is(rollbackErr, pgx.ErrTxClosed) {
			return
		}
		if err == nil {
			err = fmt.Errorf("rollback transaction: %w", rollbackErr)
			return
		}
		err = errors.Join(err, fmt.Errorf("rollback transaction: %w", rollbackErr))
	}()

	err = fn(sqlc.New(tx))
	if err != nil {
		return err
	}

	if err = tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit transaction: %w", classifyError(err))
	}
	committed = true

	return nil
}
