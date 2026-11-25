package dispatch

import (
	"context"
	"errors"
	"log/slog"
	"sync"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/ilyachakun/nightshift/internal/book/bookdb"
	"github.com/ilyachakun/nightshift/internal/config"
	"github.com/ilyachakun/nightshift/internal/flow"
	"github.com/ilyachakun/nightshift/internal/memory"
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
	// Timeout bounds one tick of one tenant in Run; zero means 10 minutes.
	Timeout time.Duration

	mu     sync.Mutex        // the tenants' loops share pruned
	pruned map[string]string // tenant -> UTC date of the last memory prune
}

func (t *Ticker) prunedOn(tenant string) string {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.pruned[tenant]
}

func (t *Ticker) markPruned(tenant, day string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.pruned == nil {
		t.pruned = map[string]string{}
	}
	t.pruned[tenant] = day
}

// pruneMemory drops expired and repeated entries of the tenant's memory page once a UTC day.
func (t *Ticker) pruneMemory(ctx context.Context, tenant string) {
	day := time.Now().UTC().Format(time.DateOnly)
	if t.prunedOn(tenant) == day {
		return
	}
	dropped, err := memory.Store{Pool: t.Pool}.Prune(ctx, tenant, time.Now())
	switch {
	case errors.Is(err, memory.ErrStale):
		return // someone wrote the page meanwhile; the next tick prunes the new revision
	case err != nil:
		// Marked all the same: a page that cannot be pruned (it fails its own cap) is reported
		// once a day, not on every tick.
		t.markPruned(tenant, day)
		t.Log.ErrorContext(ctx, "conductor: memory prune failed", "tenant", tenant, "error", err)
		return
	}
	t.markPruned(tenant, day)
	if len(dropped) > 0 {
		t.Log.InfoContext(ctx, "memory page pruned", "tenant", tenant, "dropped", dropped)
	}
}

// Tick advances each active task of every tenant once, one tenant after another. One task's
// error is logged and does not stop the others; the next tick looks at it again.
func (t *Ticker) Tick(ctx context.Context) {
	for name, tenant := range t.Tenants {
		t.tickTenant(ctx, name, tenant)
	}
}

func (t *Ticker) tickTenant(ctx context.Context, name string, tenant config.Tenant) {
	tasks, err := bookdb.New(t.Pool).ListActiveTasks(ctx, bookdb.ListActiveTasksParams{Tenant: name, States: activeStates})
	if err != nil {
		t.Log.ErrorContext(ctx, "conductor: listing tasks failed", "tenant", name, "error", err)
		return
	}
	t.pruneMemory(ctx, name)
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

// defaultTickTimeout bounds one tick of one tenant. Every step is safe to repeat, so a tick
// cut short (a clone that hangs, a code host that does not answer) is picked up by the next.
const defaultTickTimeout = 10 * time.Minute

// Run ticks until the context ends. Each tenant has its own loop, so a tenant whose tick is
// slow or fails does not delay the others (design section 17).
func (t *Ticker) Run(ctx context.Context) {
	timeout := t.Timeout
	if timeout <= 0 {
		timeout = defaultTickTimeout
	}
	var wg sync.WaitGroup
	for name, tenant := range t.Tenants {
		wg.Go(func() {
			tk := time.NewTicker(t.Every)
			defer tk.Stop()
			for {
				tctx, cancel := context.WithTimeout(ctx, timeout)
				t.tickTenant(tctx, name, tenant)
				cancel()
				select {
				case <-ctx.Done():
					return
				case <-tk.C:
				}
			}
		})
	}
	wg.Wait()
}
