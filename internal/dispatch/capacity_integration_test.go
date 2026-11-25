package dispatch_test

import (
	"context"
	"log/slog"
	"testing"
	"time"

	"github.com/ilyachakun/nightshift/internal/book/bookdb"
	"github.com/ilyachakun/nightshift/internal/config"
	"github.com/ilyachakun/nightshift/internal/dispatch"
	"github.com/ilyachakun/nightshift/internal/flow"
)

// queuedTask adds one more approved task of the rig's tenant.
func (r *rig) queuedTask(title string) bookdb.BookTask {
	r.t.Helper()
	task, err := r.q.CreateTask(r.t.Context(), bookdb.CreateTaskParams{Tenant: r.tenant.Name, Repo: "example/app", Title: title})
	if err != nil {
		r.t.Fatal(err)
	}
	for _, e := range []flow.Event{flow.LeadStarted, flow.DocsDrafted, flow.DocLoopPassed, flow.Approve} {
		if _, err := r.c.Mover.Apply(r.t.Context(), flow.Move{TaskID: task.ID, Event: e, Actor: "owner", Profile: flow.Solo}); err != nil {
			r.t.Fatal(err)
		}
	}
	return r.state(task.ID)
}

func (r *rig) state(id int64) bookdb.BookTask {
	r.t.Helper()
	task, err := r.q.GetTask(r.t.Context(), id)
	if err != nil {
		r.t.Fatal(err)
	}
	return task
}

func TestATenantRunsNoMoreTasksThanItsCapacity(t *testing.T) {
	r := newRig(t)
	r.tenant.Capacity.Tasks = 1
	second := r.queuedTask("A second feature")

	r.advance() // the first task takes the only slot
	if r.task.State != "building" {
		t.Fatalf("the first task is in %s", r.task.State)
	}
	if err := r.c.Advance(t.Context(), r.tenant, second); err != nil {
		t.Fatal(err)
	}
	if got := r.state(second.ID).State; got != "queued" {
		t.Fatalf("over capacity the second task is in %s, want queued", got)
	}

	// a task that waits for a person gives its slot back
	if _, err := r.c.Pool.Exec(t.Context(), `UPDATE book.tasks SET state = 'human_review' WHERE id = $1`, r.task.ID); err != nil {
		t.Fatal(err)
	}
	if err := r.c.Advance(t.Context(), r.tenant, r.state(second.ID)); err != nil {
		t.Fatal(err)
	}
	if got := r.state(second.ID).State; got != "building" {
		t.Fatalf("with a free slot the second task is in %s, want building", got)
	}
}

func TestWithoutACapacityEveryTaskStarts(t *testing.T) {
	r := newRig(t)
	second := r.queuedTask("A second feature")
	r.advance()
	if err := r.c.Advance(t.Context(), r.tenant, second); err != nil {
		t.Fatal(err)
	}
	if got := r.state(second.ID).State; got != "building" {
		t.Fatalf("without a capacity the second task is in %s", got)
	}
}

// A tenant whose tick hangs must not delay another tenant: each has its own loop.
func TestASlowTenantDoesNotDelayAnother(t *testing.T) {
	r := newRig(t)
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	slow := make(chan struct{})
	ticked := make(chan struct{}, 2)
	tk := &dispatch.Ticker{
		Pool:    r.c.Pool,
		Tenants: map[string]config.Tenant{"example-slow": {Name: "example-slow"}, "example-solo": r.tenant},
		For: func(tn config.Tenant) *dispatch.Conductor {
			if tn.Name == "example-slow" {
				<-slow // hangs until the test ends
				return r.c
			}
			select {
			case ticked <- struct{}{}:
			default:
			}
			return r.c
		},
		Every: 10 * time.Millisecond, Timeout: time.Hour, Log: slog.New(slog.DiscardHandler),
	}
	done := make(chan struct{})
	go func() { tk.Run(ctx); close(done) }()
	// two ticks: one loop for all tenants would reach the hanging tenant after the first
	for range 2 {
		select {
		case <-ticked:
		case <-time.After(30 * time.Second):
			t.Fatal("the other tenant stopped being ticked while one tenant hung")
		}
	}
	close(slow)
	cancel()
	<-done
}
