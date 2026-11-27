package outbox

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/riverqueue/river"
	"github.com/riverqueue/river/riverdriver/riverdatabasesql"
	"github.com/riverqueue/river/riverdriver/riverpgxv5"
	"github.com/riverqueue/river/rivermigrate"
)

// RiverSchema keeps River's tables apart from the book's.
const RiverSchema = "river"

// Migrate creates River's tables and gives the nightshift role what River needs.
// Like book.Migrate it runs as the schema owner.
func Migrate(ctx context.Context, db *sql.DB) error {
	if _, err := db.ExecContext(ctx, `CREATE SCHEMA IF NOT EXISTS `+RiverSchema); err != nil {
		return fmt.Errorf("river: create schema: %w", err)
	}
	m, err := rivermigrate.New(riverdatabasesql.New(db), &rivermigrate.Config{Schema: RiverSchema})
	if err != nil {
		return fmt.Errorf("river: migrator: %w", err)
	}
	if _, err := m.Migrate(ctx, rivermigrate.DirectionUp, nil); err != nil {
		return fmt.Errorf("river: migrate: %w", err)
	}
	for _, q := range []string{
		`GRANT USAGE ON SCHEMA ` + RiverSchema + ` TO nightshift`,
		`GRANT SELECT, INSERT, UPDATE, DELETE ON ALL TABLES IN SCHEMA ` + RiverSchema + ` TO nightshift`,
		`GRANT USAGE, SELECT ON ALL SEQUENCES IN SCHEMA ` + RiverSchema + ` TO nightshift`,
	} {
		if _, err := db.ExecContext(ctx, q); err != nil {
			return fmt.Errorf("river: grant: %w", err)
		}
	}
	return nil
}

// FlushArgs is the periodic job that delivers the outbox.
type FlushArgs struct{}

// Kind implements river.JobArgs.
func (FlushArgs) Kind() string { return "outbox_flush" }

type flushWorker struct {
	river.WorkerDefaults[FlushArgs]
	outbox *Outbox
}

func (w *flushWorker) Work(ctx context.Context, _ *river.Job[FlushArgs]) error {
	_, err := w.outbox.Flush(ctx)
	return err
}

// NewClient returns a River client that runs the outbox every interval. River elects a
// leader among clients, so only nightshift runs one; the worker container never does.
func NewClient(pool *pgxpool.Pool, ob *Outbox, interval time.Duration) (*river.Client[pgx.Tx], error) {
	workers := river.NewWorkers()
	if err := river.AddWorkerSafely(workers, &flushWorker{outbox: ob}); err != nil {
		return nil, fmt.Errorf("river: add worker: %w", err)
	}
	client, err := river.NewClient(riverpgxv5.New(pool), &river.Config{
		Schema:  RiverSchema,
		Queues:  map[string]river.QueueConfig{river.QueueDefault: {MaxWorkers: 4}},
		Workers: workers,
		Logger:  ob.Logger(),
		PeriodicJobs: []*river.PeriodicJob{
			river.NewPeriodicJob(river.PeriodicInterval(interval),
				func() (river.JobArgs, *river.InsertOpts) { return FlushArgs{}, nil },
				&river.PeriodicJobOpts{RunOnStart: true}),
		},
	})
	if err != nil {
		return nil, fmt.Errorf("river: client: %w", err)
	}
	return client, nil
}
