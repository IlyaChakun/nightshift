package dispatch_test

import (
	"context"
	"log/slog"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ilyachakun/nightshift/internal/control/dispatch"
	"github.com/ilyachakun/nightshift/internal/customization/config"
	"github.com/ilyachakun/nightshift/internal/data/book/bookdb"
	"github.com/ilyachakun/nightshift/internal/testutil/pgtest"
)

// loopRig is a ticker over the two synthetic tenants, one active task each, whose steps are
// stand-ins: the solo tenant's step hangs until its context ends.
type loopRig struct {
	tk              *dispatch.Ticker
	teamSteps, solo atomic.Int32
}

func newLoopRig(t *testing.T) *loopRig {
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
		Pool: pool, Tenants: tenants, Every: 10 * time.Millisecond, Log: slog.Default(),
		Step: func(ctx context.Context, tenant config.Tenant, _ bookdb.BookTask) error {
			if tenant.Name == "example-team" {
				r.teamSteps.Add(1)
				return nil
			}
			r.solo.Add(1)
			<-ctx.Done()
			return ctx.Err()
		},
	}
	return r
}

// TestASlowTenantDoesNotHoldBackAnother: each tenant has its own loop, so while one tenant's
// step hangs the other keeps advancing (design section 17, "Conductor"; T3).
func TestASlowTenantDoesNotHoldBackAnother(t *testing.T) {
	r := newLoopRig(t)
	ctx, cancel := context.WithCancel(t.Context())
	done := make(chan struct{})
	go func() { r.tk.Run(ctx); close(done) }()
	// Count only the team's steps after the solo step has started to hang.
	deadline := time.Now().Add(5 * time.Second)
	for r.solo.Load() == 0 && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
	}
	before := r.teamSteps.Load()
	for r.teamSteps.Load()-before < 5 && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	cancel()
	<-done
	if n := r.teamSteps.Load() - before; n < 5 {
		t.Fatalf("the team tenant advanced %d times while the solo tenant hung, want at least 5", n)
	}
	if n := r.solo.Load(); n != 1 {
		t.Fatalf("the hung solo step started %d times, want 1", n)
	}
}
