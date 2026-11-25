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
	// StepTimeout bounds one Advance of one task; zero means ten minutes. A step that runs
	// out is logged and tried again on the next tick: every step is safe to repeat.
	StepTimeout time.Duration
	// Step advances one task; nil means the tenant's conductor. Tests replace it.
	Step func(ctx context.Context, t config.Tenant, task bookdb.BookTask) error

	mu     sync.Mutex
	pruned map[string]string // tenant -> UTC date of the last memory prune
}

// defaultStepTimeout leaves room for a first clone of a large repository.
const defaultStepTimeout = 10 * time.Minute

func (t *Ticker) step(ctx context.Context, tenant config.Tenant, task bookdb.BookTask) error {
	d := t.StepTimeout
	if d <= 0 {
		d = defaultStepTimeout
	}
	ctx, cancel := context.WithTimeout(ctx, d)
	defer cancel()
	if t.Step != nil {
		return t.Step(ctx, tenant, task)
	}
	return t.For(tenant).Advance(ctx, tenant, task)
}

// pruneMemory drops expired and repeated entries of the tenant's memory page once a UTC day.
func (t *Ticker) pruneMemory(ctx context.Context, tenant string) {
	day := time.Now().UTC().Format(time.DateOnly)
	t.mu.Lock()
	done := t.pruned[tenant] == day
	t.mu.Unlock()
	if done {
		return
	}
	dropped, err := memory.Store{Pool: t.Pool}.Prune(ctx, tenant, time.Now())
	if errors.Is(err, memory.ErrStale) {
		return // someone wrote the page meanwhile; the next tick prunes the new revision
	}
	// Marked on an error too: a page that cannot be pruned (it fails its own cap) is reported
	// once a day, not on every tick.
	t.mu.Lock()
	if t.pruned == nil {
		t.pruned = map[string]string{}
	}
	t.pruned[tenant] = day
	t.mu.Unlock()
	if err != nil {
		t.Log.ErrorContext(ctx, "conductor: memory prune failed", "tenant", tenant, "error", err)
		return
	}
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
	for _, task := range runningFirst(tasks) {
		if err := t.step(ctx, tenant, task); err != nil {
			t.Log.ErrorContext(ctx, "conductor: advance failed", "tenant", name, "task", task.ID, "state", task.State, "error", err)
		}
	}
}

// Run gives every tenant its own loop until the context ends, so a slow step of one tenant
// (a large clone, a hung call to the code host) does not hold the others back (design
// section 17). A provider's concurrency still holds across the loops: the claim enforces it.
func (t *Ticker) Run(ctx context.Context) {
	var wg sync.WaitGroup
	for name, tenant := range t.Tenants {
		wg.Go(func() {
			tk := time.NewTicker(t.Every)
			defer tk.Stop()
			for {
				t.tickTenant(ctx, name, tenant)
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

// runningFirst puts the tasks that are already past the queue before the queued ones, each
// group in its given order (priority, then age). A task that leaves its capacity slot in this
// tick then frees it before the queue is looked at, so the slot goes to the first queued
// task, not to whichever happened to come after the finishing one.
func runningFirst(tasks []bookdb.BookTask) []bookdb.BookTask {
	out := make([]bookdb.BookTask, 0, len(tasks))
	for _, queued := range []bool{false, true} {
		for _, task := range tasks {
			if (flow.State(task.State) == flow.Queued) == queued {
				out = append(out, task)
			}
		}
	}
	return out
}
