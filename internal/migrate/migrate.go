// Package migrate provides database migration execution using golang-migrate
// with embedded SQL files.
package migrate

import (
	"errors"
	"fmt"
	"io/fs"
	"log/slog"

	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/pgx/v5" // pgx v5 driver
	"github.com/golang-migrate/migrate/v4/source/iofs"
)

// Run executes all pending up-migrations against the database identified by dsn.
//
// The migrations are read from the given embed.FS, which must contain a
// "migrations" subdirectory with numbered .sql files following golang-migrate
// naming conventions (e.g. 000001_extensions.up.sql).
//
// If the database is already at the latest version, Run returns nil.
// The dsn must be a valid PostgreSQL connection string; it is re-prefixed with
// "pgx5://" for the golang-migrate pgx v5 driver.
func Run(fsys fs.FS, dsn string, logger *slog.Logger) error {
	if dsn == "" {
		return errors.New("migrate: empty dsn")
	}

	source, err := iofs.New(fsys, "migrations")
	if err != nil {
		return fmt.Errorf("migrate: create source: %w", err)
	}

	// golang-migrate uses its own URL scheme for the pgx v5 driver.
	m, err := migrate.NewWithSourceInstance("iofs", source, rewriteDSN(dsn))
	if err != nil {
		return fmt.Errorf("migrate: init: %w", err)
	}
	defer m.Close()

	version, dirty, _ := m.Version()
	logger.Info("migration state before run", "version", version, "dirty", dirty)

	if dirty {
		return fmt.Errorf("migrate: database is in dirty state at version %d; manual intervention required", version)
	}

	if err := m.Up(); err != nil { //nolint:govet // short-lived err shadow is idiomatic Go
		if errors.Is(err, migrate.ErrNoChange) {
			logger.Info("migrations: already at latest version", "version", version)
			return nil
		}
		return fmt.Errorf("migrate: up: %w", err)
	}

	newVersion, _, _ := m.Version()
	logger.Info("migrations applied successfully", "from_version", version, "to_version", newVersion)
	return nil
}

// rewriteDSN converts a standard PostgreSQL DSN to the golang-migrate pgx5 scheme.
//
// golang-migrate's pgx v5 driver expects "pgx5://" or "pgx5h://" prefixes.
// We accept both postgres:// and postgresql:// and rewrite to pgx5://.
// If the DSN already has the correct prefix, it is returned as-is.
func rewriteDSN(dsn string) string {
	switch {
	case len(dsn) >= 13 && dsn[:13] == "postgresql://":
		return "pgx5://" + dsn[13:]
	case len(dsn) >= 11 && dsn[:11] == "postgres://":
		return "pgx5://" + dsn[11:]
	default:
		return dsn
	}
}
