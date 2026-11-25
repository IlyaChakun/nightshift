// Package pgtest starts a throwaway Postgres for T3 tests and applies the book migrations.
package pgtest

import (
	"context"
	"database/sql"
	"fmt"
	"sync"
	"sync/atomic"
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
	database string
}

// templateDB holds the migrated book; every test copies it, which takes milliseconds,
// where migrating a fresh database takes seconds under -race.
const templateDB = "book_template"

// shared is the one container of the test binary, started by the first test that needs it.
// testcontainers' reaper removes it when the process exits.
var shared struct {
	once sync.Once
	err  error
	host string
	port string
	next atomic.Int64
}

func startShared() {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	ctr, err := postgres.Run(ctx, Image,
		postgres.WithDatabase(templateDB),
		postgres.WithUsername("owner"),
		postgres.WithPassword("test-only"),
		testcontainers.WithWaitStrategy(wait.ForLog("database system is ready to accept connections").
			WithOccurrence(2).WithStartupTimeout(time.Minute)),
	)
	if err != nil {
		shared.err = fmt.Errorf("start postgres: %w", err)
		return
	}
	if shared.host, err = ctr.Host(ctx); err != nil {
		shared.err = err
		return
	}
	port, err := ctr.MappedPort(ctx, "5432/tcp")
	if err != nil {
		shared.err = err
		return
	}
	shared.port = port.Port()
	db, err := sql.Open("pgx", dsn("owner", "test-only", templateDB))
	if err != nil {
		shared.err = err
		return
	}
	defer func() { _ = db.Close() }()
	if err := book.Migrate(ctx, db); err != nil {
		shared.err = err
	}
}

func dsn(user, password, database string) string {
	return fmt.Sprintf("postgres://%s:%s@%s:%s/%s?sslmode=disable", user, password, shared.host, shared.port, database)
}

// Start gives the test its own database on a Postgres container that the whole test binary
// shares. It skips the test in -short mode and where Docker is not available; CI has Docker.
// Start marks the test parallel: call it first in a top-level test, before any other setup.
func Start(t *testing.T) *DB {
	t.Helper()
	if testing.Short() {
		t.Skip("integration test")
	}
	testcontainers.SkipIfProviderIsNotHealthy(t)
	t.Parallel()
	ctx := t.Context()
	shared.once.Do(startShared)
	if shared.err != nil {
		t.Fatal(shared.err)
	}
	name := fmt.Sprintf("book_test_%d", shared.next.Add(1))
	admin, err := sql.Open("pgx", dsn("owner", "test-only", "postgres"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = admin.Close() }()
	if _, err := admin.ExecContext(ctx, `CREATE DATABASE `+name+` TEMPLATE `+templateDB); err != nil {
		t.Fatalf("create test database: %v", err)
	}
	t.Cleanup(func() {
		// The test context is already cancelled here; the database is dropped on a fresh one.
		c, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		a, err := sql.Open("pgx", dsn("owner", "test-only", "postgres"))
		if err != nil {
			return
		}
		defer func() { _ = a.Close() }()
		_, _ = a.ExecContext(c, `DROP DATABASE IF EXISTS `+name+` WITH (FORCE)`)
	})
	d := &DB{database: name}
	d.Owner, err = sql.Open("pgx", dsn("owner", "test-only", name))
	if err != nil {
		t.Fatal(err)
	}
	d.Owner.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = d.Owner.Close() })
	return d
}

// ServicePool connects as the nightshift role, the way serve does.
func (d *DB) ServicePool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	if _, err := d.Owner.ExecContext(t.Context(), `ALTER ROLE nightshift LOGIN PASSWORD 'test-only'`); err != nil {
		t.Fatal(err)
	}
	pool, err := pgxpool.New(t.Context(), dsn("nightshift", "test-only", d.database))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	return pool
}
