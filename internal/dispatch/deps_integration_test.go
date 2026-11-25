package dispatch_test

import (
	"testing"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/ilyachakun/nightshift/internal/book/bookdb"
	"github.com/ilyachakun/nightshift/internal/flow"
)

const threeTasks = `{"title":"feat: whole","goal":"g","prd_path":"docs/prd/x.md","acceptance_criteria":[{"id":"AC-1","given":"a","when":"b","then":"c"}],
 "tasks":[{"id":"T-1","title":"feat: first"},{"id":"T-2","title":"feat: second","depends_on":["T-1"]},{"id":"T-3","title":"feat: third","depends_on":["T-1","T-2"]}]}`

// specRig puts an approved three-task spec on the rig's task.
func specRig(t *testing.T) *rig {
	t.Helper()
	r := newRig(t)
	if _, err := r.q.AppendEvent(t.Context(), bookdb.AppendEventParams{
		Tenant: r.task.Tenant, TaskID: pgtype.Int8{Int64: r.task.ID, Valid: true}, Kind: "lead.spec", Actor: "lead", Payload: []byte(threeTasks),
	}); err != nil {
		t.Fatal(err)
	}
	return r
}

func (r *rig) siblings() []bookdb.BookTask {
	r.t.Helper()
	all, err := r.q.ListTasksByState(r.t.Context(), bookdb.ListTasksByStateParams{Tenant: r.task.Tenant, State: "queued"})
	if err != nil {
		r.t.Fatal(err)
	}
	return all
}

func TestAnApprovedSpecBecomesTasksThatWaitForTheirDependencies(t *testing.T) {
	r := specRig(t)
	ctx := t.Context()
	r.advance() // expands the spec, then T-1 starts at once
	if r.task.Title != "feat: first" || flow.State(r.task.State) != flow.Building {
		t.Fatalf("first task: %q in %s", r.task.Title, r.task.State)
	}
	waiting := r.siblings()
	if len(waiting) != 2 || waiting[0].Title != "feat: second" || waiting[1].Title != "feat: third" {
		t.Fatalf("siblings %+v", waiting)
	}
	r.advance() // a second tick must not expand again
	if again := r.siblings(); len(again) != 2 {
		t.Fatalf("the spec was expanded twice: %d queued", len(again))
	}
	second, third := waiting[0], waiting[1]
	for _, w := range waiting {
		if err := r.c.Advance(ctx, r.tenant, w); err != nil {
			t.Fatal(err)
		}
	}
	for _, w := range waiting {
		if got, _ := r.q.GetTask(ctx, w.ID); flow.State(got.State) != flow.Queued {
			t.Fatalf("task %d left the queue before its dependency was released: %s", w.ID, got.State)
		}
	}
	// T-1 is released: T-2 may start; T-3 still waits for T-2.
	if _, err := r.c.Pool.Exec(ctx, `UPDATE book.tasks SET state = 'released' WHERE id = $1`, r.task.ID); err != nil {
		t.Fatal(err)
	}
	if err := r.c.Advance(ctx, r.tenant, second); err != nil {
		t.Fatal(err)
	}
	if err := r.c.Advance(ctx, r.tenant, third); err != nil {
		t.Fatal(err)
	}
	if got, _ := r.q.GetTask(ctx, second.ID); flow.State(got.State) != flow.Building {
		t.Fatalf("T-2 should be building, is %s", got.State)
	}
	if got, _ := r.q.GetTask(ctx, third.ID); flow.State(got.State) != flow.Queued {
		t.Fatalf("T-3 should still wait, is %s", got.State)
	}
}

func TestACancelledDependencyBlocksTheTask(t *testing.T) {
	r := specRig(t)
	ctx := t.Context()
	r.advance()
	second := r.siblings()[0]
	if _, err := r.c.Pool.Exec(ctx, `UPDATE book.tasks SET state = 'canceled' WHERE id = $1`, r.task.ID); err != nil {
		t.Fatal(err)
	}
	if err := r.c.Advance(ctx, r.tenant, second); err != nil {
		t.Fatal(err)
	}
	if got, _ := r.q.GetTask(ctx, second.ID); flow.State(got.State) != flow.Blocked {
		t.Fatalf("T-2 should be blocked, is %s", got.State)
	}
}

func TestExpandingTwiceAtOnceMakesTheTasksOnce(t *testing.T) {
	r := specRig(t)
	errs := make(chan error, 2)
	for range 2 {
		go func() { errs <- r.c.Advance(t.Context(), r.tenant, r.task) }()
	}
	for range 2 {
		if err := <-errs; err != nil {
			t.Log(err) // a lost race on the transition is fine; duplicates are not
		}
	}
	if n := len(r.siblings()); n != 2 {
		t.Fatalf("want 2 queued siblings, got %d", n)
	}
}

// A dependency that someone else merged (the team, or the owner on GitHub) is done: the next
// task must start, not wait for a `released` that never comes.
func TestADependencyMergedBySomeoneElseUnblocksTheTask(t *testing.T) {
	r := specRig(t)
	ctx := t.Context()
	r.advance()
	second := r.siblings()[0]
	if _, err := r.c.Pool.Exec(ctx, `UPDATE book.tasks SET state = 'merged_by_team' WHERE id = $1`, r.task.ID); err != nil {
		t.Fatal(err)
	}
	if err := r.c.Advance(ctx, r.tenant, second); err != nil {
		t.Fatal(err)
	}
	if got, _ := r.q.GetTask(ctx, second.ID); flow.State(got.State) != flow.Building {
		t.Fatalf("T-2 should be building after T-1 was merged by someone else, is %s", got.State)
	}
}
