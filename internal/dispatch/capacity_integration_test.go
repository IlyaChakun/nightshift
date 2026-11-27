package dispatch_test

import (
	"log/slog"
	"testing"
	"time"

	"github.com/ilyachakun/nightshift/internal/customization/config"
	"github.com/ilyachakun/nightshift/internal/data/book/bookdb"
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

// A slot freed inside a tick goes to the queued task with the highest priority, also when
// that task is listed before the one that frees the slot.
func TestAFreedSlotGoesToTheFirstQueuedTask(t *testing.T) {
	r := newRig(t)
	r.withGate(config.Review{Mode: "all"}, "solo")
	r.tenant.Capacity.Tasks = 1
	r.toReview("feature.txt") // the rig's task holds the slot; its next step is human review
	low := r.queuedTask("Low priority")
	high := r.queuedTask("High priority")
	ctx := t.Context()
	// listed first: priority 9, then the running task, then the low one
	if _, err := r.c.Pool.Exec(ctx, `UPDATE book.tasks SET priority = 9 WHERE id = $1`, high.ID); err != nil {
		t.Fatal(err)
	}
	tk := &dispatch.Ticker{
		Pool: r.c.Pool, Tenants: map[string]config.Tenant{r.tenant.Name: r.tenant},
		For: func(config.Tenant) *dispatch.Conductor { return r.c }, Every: time.Hour, Log: slog.New(slog.DiscardHandler),
	}
	tk.Tick(ctx) // review -> human_review frees the slot inside this tick
	if got := r.state(r.task.ID).State; got != "human_review" {
		t.Fatalf("the running task is in %s, want human_review", got)
	}
	if h, l := r.state(high.ID).State, r.state(low.ID).State; h != "building" || l != "queued" {
		t.Fatalf("after the slot was freed: high %s (want building), low %s (want queued)", h, l)
	}
}
