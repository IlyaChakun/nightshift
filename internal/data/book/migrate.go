// Package book owns the Postgres schema of the book: migrations here, typed queries in bookdb.
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
// nightshift service role; `serve --migrate-only` (the compose migrate service) is the only
// caller in production.
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

// DashboardRole is the Postgres role Grafana logs in as; it reads only the book.dash_* views.
const DashboardRole = "grafana_ro"

// DashboardLogin gives the dashboard role a login with this password, so Grafana can read the
// views. It runs as the schema owner after Migrate, which created the role and so administers it.
// An empty password changes nothing: the role keeps no login.
func DashboardLogin(ctx context.Context, db *sql.DB, password string) error {
	if password == "" {
		return nil
	}
	// ALTER ROLE and GRANT take no bind parameters, so Postgres quotes the values itself.
	if err := execFormatted(ctx, db, `SELECT format('ALTER ROLE %I LOGIN PASSWORD %L', $1::text, $2::text)`, DashboardRole, password); err != nil {
		return err
	}
	return execFormatted(ctx, db, `SELECT format('GRANT CONNECT ON DATABASE %I TO %I', current_database(), $1::text)`, DashboardRole)
}

// execFormatted runs the statement that the query builds.
func execFormatted(ctx context.Context, db *sql.DB, query string, args ...any) error {
	var stmt string
	if err := db.QueryRowContext(ctx, query, args...).Scan(&stmt); err != nil {
		return fmt.Errorf("book: dashboard login: %w", err)
	}
	if _, err := db.ExecContext(ctx, stmt); err != nil {
		return fmt.Errorf("book: dashboard login: %w", err)
	}
	return nil
}
