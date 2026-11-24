// Package pgtest starts a throwaway Postgres for T3 tests and applies the book migrations.
package pgtest

import (
	"context"
	"database/sql"
	"fmt"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	_ "github.com/jackc/pgx/v5/stdlib" // database/sql driver "pgx"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"

	"github.com/ilyachakun/nightshift/internal/book"
)

// Image matches the postgres image in deploy/docker-compose.yml.
const Image = "postgres:18.1-alpine"

// DB is a migrated test database.
type DB struct {
	// Owner is a single connection as the schema owner, so SET ROLE sticks.
	Owner *sql.DB
	dsn   string // with %s for the user and the password
}

// Start runs Postgres in a container. It skips the test in -short mode and where
// Docker is not available; CI has Docker.
func Start(t *testing.T) *DB {
	t.Helper()
	if testing.Short() {
		t.Skip("integration test")
	}
	testcontainers.SkipIfProviderIsNotHealthy(t)
	ctx := context.Background()
	ctr, err := postgres.Run(ctx, Image,
		postgres.WithDatabase("book"),
		postgres.WithUsername("owner"),
		postgres.WithPassword("test-only"),
		testcontainers.WithWaitStrategy(wait.ForLog("database system is ready to accept connections").
			WithOccurrence(2).WithStartupTimeout(time.Minute)),
	)
	testcontainers.CleanupContainer(t, ctr)
	if err != nil {
		t.Fatalf("start postgres: %v", err)
	}
	host, err := ctr.Host(ctx)
	if err != nil {
		t.Fatal(err)
	}
	port, err := ctr.MappedPort(ctx, "5432/tcp")
	if err != nil {
		t.Fatal(err)
	}
	d := &DB{dsn: "postgres://%s:%s@" + host + ":" + port.Port() + "/book?sslmode=disable"}
	d.Owner, err = sql.Open("pgx", fmt.Sprintf(d.dsn, "owner", "test-only"))
	if err != nil {
		t.Fatal(err)
	}
	d.Owner.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = d.Owner.Close() })
	if err := book.Migrate(ctx, d.Owner); err != nil {
		t.Fatal(err)
	}
	return d
}

// ServicePool connects as the nightshift role, the way serve does.
func (d *DB) ServicePool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	if _, err := d.Owner.ExecContext(context.Background(), `ALTER ROLE nightshift LOGIN PASSWORD 'test-only'`); err != nil {
		t.Fatal(err)
	}
	pool, err := pgxpool.New(context.Background(), fmt.Sprintf(d.dsn, "nightshift", "test-only"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	return pool
}
