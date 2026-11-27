package dispatch

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/ilyachakun/nightshift/internal/control/flow"
	"github.com/ilyachakun/nightshift/internal/customization/config"
	"github.com/ilyachakun/nightshift/internal/data/book/bookdb"
)

func (c *Conductor) moveWithFindingUpdates(ctx context.Context, t config.Tenant, task bookdb.BookTask, event flow.Event, head string, update func(*bookdb.Queries) error) error {
	return pgx.BeginFunc(ctx, c.Pool, func(tx pgx.Tx) error {
		q := bookdb.New(tx)
		if _, err := lockTaskSnapshot(ctx, q, task); err != nil {
			return err
		}
		if err := update(q); err != nil {
			return fmt.Errorf("conductor: resolve findings for task %d: %w", task.ID, err)
		}
		_, err := c.Mover.ApplyInTx(ctx, tx, flow.Move{TaskID: task.ID, Event: event, Actor: "system", Profile: c.profile(t), HeadSHA: head})
		return err
	})
}
