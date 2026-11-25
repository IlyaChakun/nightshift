package dispatch_test

import (
	"context"
	"errors"
	"log/slog"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ilyachakun/nightshift/internal/book/bookdb"
	"github.com/ilyachakun/nightshift/internal/config"
	"github.com/ilyachakun/nightshift/internal/dispatch"
	"github.com/ilyachakun/nightshift/internal/testutil/pgtest"
)

// loopRig is a ticker over the two synthetic tenants, one active task each, whose steps are
// stand-ins: the solo tenant's step hangs until its context ends.
type loopRig struct {
	tk               *dispatch.Ticker
	teamSteps, solo  atomic.Int32
	soloDeadlineSeen atomic.Bool
}

func newLoopRig(t *testing.T, timeout time.Duration) *loopRig {
	t.Helper()
	pool := pgtest.Start(t).ServicePool(t)
	q := bookdb.New(pool)
	tenants := map[string]config.Tenant{}
	for _, name := range []string{"example-solo", "example-team"} {
		if _, err := q.CreateTask(t.Context(), bookdb.CreateTaskParams{Tenant: name, Repo: "example/app", Title: "Loops"}); err != nil {
			t.Fatal(err)
		}
		tenants[name] = config.Tenant{Name: name}
	}
	r := &loopRig{}
	r.tk = &dispatch.Ticker{
		Pool: pool, Tenants: tenants, Every: 10 * time.Millisecond, StepTimeout: timeout, Log: slog.Default(),
		Step: func(ctx context.Context, tenant config.Tenant, _ bookdb.BookTask) error {
			if tenant.Name == "example-team" {
				r.teamSteps.Add(1)
				return nil
			}
			r.solo.Add(1)
			<-ctx.Done()
			if errors.Is(ctx.Err(), context.DeadlineExceeded) {
				r.soloDeadlineSeen.Store(true)
			}
			return ctx.Err()
		},
	}
	return r
}

// TestASlowTenantDoesNotHoldBackAnother: each tenant has its own loop, so while one tenant's
// step hangs the other keeps advancing (design section 17, "Conductor"; T3).
func TestASlowTenantDoesNotHoldBackAnother(t *testing.T) {
	r := newLoopRig(t, time.Minute)
	ctx, cancel := context.WithCancel(t.Context())
	done := make(chan struct{})
	go func() { r.tk.Run(ctx); close(done) }()
	deadline := time.Now().Add(5 * time.Second)
	for r.teamSteps.Load() < 5 && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	cancel()
	<-done
	if n := r.teamSteps.Load(); n < 5 {
		t.Fatalf("the team tenant advanced %d times while the solo tenant hung, want at least 5", n)
	}
	if n := r.solo.Load(); n != 1 {
		t.Fatalf("the hung solo step started %d times, want 1", n)
	}
}

// TestAStepThatRunsOutIsCut: a hung step ends at StepTimeout, so a tick finishes and the next
// one tries the task again (T3).
func TestAStepThatRunsOutIsCut(t *testing.T) {
	r := newLoopRig(t, 50*time.Millisecond)
	start := time.Now()
	r.tk.Tick(t.Context())
	if d := time.Since(start); d > 5*time.Second {
		t.Fatalf("a tick with a hung step took %s", d)
	}
	if !r.soloDeadlineSeen.Load() {
		t.Fatal("the hung step did not see its deadline")
	}
	if r.teamSteps.Load() != 1 {
		t.Fatalf("team steps %d, want 1", r.teamSteps.Load())
	}
}
