package dispatch

import (
	"context"
	"errors"
	"fmt"
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
	string(flow.HandedOff), string(flow.Merging), string(flow.RolledBack),
}

// Ticker calls the conductor for every active task of every tenant on a fixed interval.
type Ticker struct {
	Pool    *pgxpool.Pool
	Tenants map[string]config.Tenant
	// For returns the conductor of a tenant; it holds that tenant's router and settings.
	For   func(config.Tenant) *Conductor
	Every time.Duration
	Log   *slog.Logger
	// Projector, when set, keeps each tenant's tracker in step with the book.
	Projector *Projector
	// Morning, when set, sends the morning digest of each tenant once a day.
	Morning *Morning
	// Health, when set, opens a task per open code-scanning alert and per idle pull request.
	Health *HealthTasks
	// Step advances one task; nil means the tenant's conductor. Tests replace it.
	Step func(ctx context.Context, t config.Tenant, task bookdb.BookTask) error
	// StepTimeout cuts a step that hangs (a call to the code host that never answers, a huge
	// clone), so that one task cannot hold its tenant's loop for ever; zero means 30 minutes.
	// Cutting a step is safe: a pull request opened but not recorded is found again
	// (`OpenPR`), every other side effect repeats (`sideeffects_test.go`), and a canceled git
	// is stopped with SIGTERM so that it removes its lock files (`gitops`).
	StepTimeout time.Duration

	mu     sync.Mutex
	pruned map[string]string    // tenant -> UTC date of the last memory prune
	swept  map[string]time.Time // tenant -> last preview teardown sweep
}

const defaultStepTimeout = 30 * time.Minute

// step runs one step under its timeout. A step that ran out of time is logged and left for
// the next tick, which looks at the task again from the book; it is not an error of the task.
func (t *Ticker) step(ctx context.Context, tenant config.Tenant, task bookdb.BookTask) error {
	limit := t.stepLimit()
	stepCtx, cancel := context.WithTimeout(ctx, limit)
	defer cancel()
	err := t.advance(stepCtx, tenant, task)
	if err != nil && ctx.Err() == nil && errors.Is(stepCtx.Err(), context.DeadlineExceeded) {
		t.Log.WarnContext(ctx, "conductor: step timed out; the next tick tries again", "tenant", tenant.Name, "task", task.ID, "state", task.State, "limit", limit)
		return nil
	}
	return err
}

func (t *Ticker) advance(ctx context.Context, tenant config.Tenant, task bookdb.BookTask) error {
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
	// The repository read has the step timeout: a hung remote must not hold the tenant's loop.
	pruneCtx, cancel := context.WithTimeout(ctx, t.stepLimit())
	defer cancel()
	dropped, err := memory.Store{Pool: t.Pool}.Prune(pruneCtx, tenant, time.Now(), t.repoFile(tenant))
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
	if t.Projector != nil {
		t.Projector.Run(ctx, tenant)
	}
	if t.Morning != nil {
		if err := t.Morning.Run(ctx, tenant); err != nil {
			t.Log.ErrorContext(ctx, "conductor: morning digest failed", "tenant", name, "error", err)
		}
	}
	if t.Health != nil {
		if err := t.Health.Run(ctx, tenant); err != nil {
			t.Log.ErrorContext(ctx, "conductor: health tasks failed", "tenant", name, "error", err)
		}
	}
	for _, task := range runningFirst(tasks) {
		if err := t.step(ctx, tenant, task); err != nil {
			t.Log.ErrorContext(ctx, "conductor: advance failed", "tenant", name, "task", task.ID, "state", task.State, "error", err)
		}
	}
	t.teardownPreviews(ctx, tenant)
}

// teardownEvery is how often a tenant's ended tasks are swept for previews. Each sweep reads the
// project file from a fresh mirror, so it does not run on every tick.
const teardownEvery = 10 * time.Minute

// teardownPreviews removes the previews of ended tasks, each under the step timeout, so a hung
// deployer cannot hold the tenant's loop for longer than one step per task.
func (t *Ticker) teardownPreviews(ctx context.Context, tenant config.Tenant) {
	if t.For == nil {
		return
	}
	now := time.Now()
	t.mu.Lock()
	due := now.Sub(t.swept[tenant.Name]) >= teardownEvery
	if due {
		if t.swept == nil {
			t.swept = map[string]time.Time{}
		}
		t.swept[tenant.Name] = now
	}
	t.mu.Unlock()
	if !due {
		return
	}
	c := t.For(tenant)
	if c == nil {
		return
	}
	if err := c.TeardownPreviews(ctx, tenant, t.stepLimit()); err != nil {
		t.Log.WarnContext(ctx, "conductor: preview teardown failed; the next sweep tries again", "tenant", tenant.Name, "error", err)
	}
}

// Run gives every tenant its own loop until the context ends, so a slow step of one tenant
// (a large clone, a hung call to the code host) does not hold the others back (design
// section 17). A provider's concurrency still holds across the loops: the claim enforces it.
// Loops share the repository mirrors, which is safe because a repository belongs to one
// tenant (config refuses two). With no tenants, Run returns at once.
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

// repoFile reads files of the tenant's first repository at the base branch, for the prune rule
// "the fact now lives in the repository". It returns nil (the rule is off) when the tenant has no
// repository or its conductor cannot reach one; a failed read later only keeps the entry.
func (t *Ticker) repoFile(name string) memory.RepoFile {
	tenant := t.Tenants[name]
	if t.For == nil || len(tenant.Repos) == 0 {
		return nil
	}
	c := t.For(tenant)
	if c == nil || c.Git == nil || c.Remote == nil {
		return nil
	}
	repo := tenant.Repos[0]
	var mirror, base string // fetched on the first read, then reused for the rest of this prune
	var failed error        // a failed fetch is not retried for each of the next reads
	return func(ctx context.Context, path string) (string, bool, error) {
		if failed != nil {
			return "", false, failed
		}
		if base == "" {
			m, err := c.mirror(ctx, repo)
			if err == nil {
				var b string
				if b, err = c.Git.Rev(ctx, m, c.base()); err == nil {
					mirror, base = m, b
				}
			}
			if err != nil {
				failed = fmt.Errorf("conductor: %w", err)
				return "", false, failed
			}
		}
		return c.Git.Show(ctx, mirror, base, path)
	}
}

func (t *Ticker) stepLimit() time.Duration {
	if t.StepTimeout > 0 {
		return t.StepTimeout
	}
	return defaultStepTimeout
}
