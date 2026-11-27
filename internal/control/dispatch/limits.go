package dispatch

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/ilyachakun/nightshift/internal/data/book/bookdb"
)

// limitHit reports whether a failed job stopped on a provider limit.
func limitHit(job bookdb.BookJob) bool {
	return job.ExitReason.String == "rate_limit"
}

// requeueAfterLimit closes a job that hit a limit without touching the task. The book now
// holds the limit, so the next tick routes to another provider or waits for the reset.
func (c *Conductor) requeueAfterLimit(ctx context.Context, task bookdb.BookTask, job bookdb.BookJob) error {
	if _, err := bookdb.New(c.Pool).ConsumeJob(ctx, job.ID); err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return fmt.Errorf("conductor: %w", err)
	}
	c.log().InfoContext(ctx, "provider limit, job returns to the queue", "task", task.ID, "provider", job.Provider, "role", job.Role)
	return nil
}
