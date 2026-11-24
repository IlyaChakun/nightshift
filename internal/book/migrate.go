// Package book owns the Postgres schema of the book: migrations now, sqlc queries later.
package book

import (
	"context"
	"database/sql"
	"embed"
	"fmt"
	"io/fs"

	"github.com/pressly/goose/v3"
)

//go:embed migrations/*.sql
var migrations embed.FS

// Migrations returns the embedded migration files, for checks that read them.
func Migrations() fs.FS {
	sub, err := fs.Sub(migrations, "migrations")
	if err != nil {
		panic(err) // the directory is embedded at build time
	}
	return sub
}

// Migrate applies all pending migrations. It must run as the schema owner, never as the
// nightshift service role; deploy/upgrade.sh is the only caller in production.
func Migrate(ctx context.Context, db *sql.DB) error {
	p, err := goose.NewProvider(goose.DialectPostgres, db, Migrations())
	if err != nil {
		return fmt.Errorf("book: create migration provider: %w", err)
	}
	if _, err := p.Up(ctx); err != nil {
		return fmt.Errorf("book: apply migrations: %w", err)
	}
	return nil
}
