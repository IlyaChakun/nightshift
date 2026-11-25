package dispatch

import (
	"context"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/ilyachakun/nightshift/internal/book/bookdb"
	"github.com/ilyachakun/nightshift/internal/config"
	"github.com/ilyachakun/nightshift/internal/flow"
)

// activeStates are the states the conductor drives. Everything else waits for Lead, for a
// person or for a later phase.
var activeStates = []string{
	string(flow.Idea), string(flow.Grilling), string(flow.Queued), string(flow.Building), string(flow.Checks), string(flow.Review),
	string(flow.Preview), string(flow.Acceptance), string(flow.HumanReview), string(flow.Ready),
	string(flow.HandedOff), string(flow.Merging),
}

// Ticker calls the conductor for every active task of every tenant on a fixed interval.
type Ticker struct {
	Pool    *pgxpool.Pool
	Tenants map[string]config.Tenant
	// For returns the conductor of a tenant; it holds that tenant's router and settings.
	For   func(config.Tenant) *Conductor
	Every time.Duration
	Log   *slog.Logger
	// Morning, when set, sends the morning digest of each tenant once a day.
	Morning *Morning
}

// Tick advances each active task once. One task's error is logged and does not stop the
// others; the next tick looks at it again.
func (t *Ticker) Tick(ctx context.Context) {
	q := bookdb.New(t.Pool)
	for name, tenant := range t.Tenants {
		tasks, err := q.ListActiveTasks(ctx, bookdb.ListActiveTasksParams{Tenant: name, States: activeStates})
		if err != nil {
			t.Log.ErrorContext(ctx, "conductor: listing tasks failed", "tenant", name, "error", err)
			continue
		}
		if t.Morning != nil {
			if err := t.Morning.Run(ctx, tenant); err != nil {
				t.Log.ErrorContext(ctx, "conductor: morning digest failed", "tenant", name, "error", err)
			}
		}
		c := t.For(tenant)
		for _, task := range tasks {
			if err := c.Advance(ctx, tenant, task); err != nil {
				t.Log.ErrorContext(ctx, "conductor: advance failed", "tenant", name, "task", task.ID, "state", task.State, "error", err)
			}
		}
	}
}

// Run ticks until the context ends.
func (t *Ticker) Run(ctx context.Context) {
	tk := time.NewTicker(t.Every)
	defer tk.Stop()
	for {
		t.Tick(ctx)
		select {
		case <-ctx.Done():
			return
		case <-tk.C:
		}
	}
}
