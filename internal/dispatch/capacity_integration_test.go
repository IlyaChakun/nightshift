package dispatch_test

import (
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

// A free slot goes to the queued task with the highest priority, not to the oldest.
func TestAFreedSlotGoesToTheFirstQueuedTask(t *testing.T) {
	r := newRig(t)
	r.tenant.Capacity.Tasks = 1
	r.advance() // the rig's task builds and holds the slot
	low := r.queuedTask("Low priority")
	high := r.queuedTask("High priority")
	ctx := t.Context()
	if _, err := r.c.Pool.Exec(ctx, `UPDATE book.tasks SET priority = 9 WHERE id = $1`, high.ID); err != nil {
		t.Fatal(err)
	}
	tk := &dispatch.Ticker{
		Pool: r.c.Pool, Tenants: map[string]config.Tenant{r.tenant.Name: r.tenant},
		For: func(config.Tenant) *dispatch.Conductor { return r.c }, Every: time.Hour, Log: slog.New(slog.DiscardHandler),
	}
	tk.Tick(ctx)
	if h, l := r.state(high.ID).State, r.state(low.ID).State; h != "queued" || l != "queued" {
		t.Fatalf("with the slot taken: high %s, low %s", h, l)
	}
	// the running task now waits for a person: its slot is free
	if _, err := r.c.Pool.Exec(ctx, `UPDATE book.tasks SET state = 'human_review' WHERE id = $1`, r.task.ID); err != nil {
		t.Fatal(err)
	}
	tk.Tick(ctx)
	if h, l := r.state(high.ID).State, r.state(low.ID).State; h != "building" || l != "queued" {
		t.Fatalf("after the slot was freed: high %s (want building), low %s (want queued)", h, l)
	}
}
