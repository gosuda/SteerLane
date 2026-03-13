// Package steerlane embeds SQL migration files for use by the migration runner.
package steerlane

import "embed"

// Migrations contains the embedded SQL migration files from the migrations/ directory.
//
//go:embed migrations/*.sql
var Migrations embed.FS
