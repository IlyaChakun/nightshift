package flow_test

import (
	"errors"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/ilyachakun/nightshift/internal/book/bookdb"
	"github.com/ilyachakun/nightshift/internal/flow"
	"github.com/ilyachakun/nightshift/internal/testutil/pgtest"
)

func TestMoverWritesStateAndEventTogether(t *testing.T) {
	pool := pgtest.Start(t).ServicePool(t)
	ctx := t.Context()
	q := bookdb.New(pool)
	task, err := q.CreateTask(ctx, bookdb.CreateTaskParams{Tenant: "example-solo", Repo: "example/app", Title: "Move"})
	if err != nil {
		t.Fatal(err)
	}
	m := &flow.Mover{Pool: pool}
	move := func(e flow.Event) (bookdb.BookTask, error) {
		return m.Apply(ctx, flow.Move{TaskID: task.ID, Event: e, Actor: "system", Profile: flow.Solo})
	}

	got, err := move(flow.LeadStarted)
	if err != nil || got.State != "grilling" || got.Version != task.Version+1 {
		t.Fatalf("first move: %+v, %v", got, err)
	}
	// An event the state does not allow changes nothing and records nothing.
	if _, err := move(flow.Merge); !errors.Is(err, flow.ErrNotAllowed) {
		t.Fatalf("merge while grilling: %v", err)
	}
	events, err := q.ListTaskEvents(ctx, pgtype.Int8{Int64: task.ID, Valid: true})
	if err != nil || len(events) != 1 || events[0].Kind != "task.moved" {
		t.Fatalf("events after a refused move: %+v, %v", events, err)
	}
}

// TestMoverCountsReturnsAndEscalates drives a task through review three times; the fourth
// return escalates instead of going back to Builder.
func TestMoverCountsReturnsAndEscalates(t *testing.T) {
	pool := pgtest.Start(t).ServicePool(t)
	ctx := t.Context()
	task, err := bookdb.New(pool).CreateTask(ctx, bookdb.CreateTaskParams{Tenant: "example-solo", Repo: "example/app", Title: "Returns"})
	if err != nil {
		t.Fatal(err)
	}
	m := &flow.Mover{Pool: pool}
	var last bookdb.BookTask
	steps := []flow.Event{flow.LeadStarted, flow.DocsDrafted, flow.DocLoopPassed, flow.Approve, flow.DepsClosed}
	for range flow.MaxReturns {
		steps = append(steps, flow.PROpened, flow.AllGreen, flow.Returned)
	}
	steps = append(steps, flow.PROpened, flow.AllGreen, flow.Returned)
	for i, e := range steps {
		last, err = m.Apply(ctx, flow.Move{TaskID: task.ID, Event: e, Actor: "system", Profile: flow.Solo, HeadSHA: sha(i)})
		if err != nil {
			t.Fatalf("step %d (%s): %v", i, e, err)
		}
	}
	if last.State != "escalated" || last.Returns != flow.MaxReturns {
		t.Fatalf("after the fourth return: %+v", last)
	}
	if !last.HeadSha.Valid || last.HeadSha.String != sha(len(steps)-1) {
		t.Errorf("head sha: %+v", last.HeadSha)
	}
}

func sha(i int) string {
	const hex = "0123456789abcdef"
	b := make([]byte, 40)
	for j := range b {
		b[j] = hex[(i+j)%16]
	}
	return string(b)
}

func TestCancelStopsTheTasksJobs(t *testing.T) {
	pool := pgtest.Start(t).ServicePool(t)
	ctx := t.Context()
	q := bookdb.New(pool)
	task, err := q.CreateTask(ctx, bookdb.CreateTaskParams{Tenant: "example-solo", Repo: "example/app", Title: "Cancel"})
	if err != nil {
		t.Fatal(err)
	}
	other, err := q.CreateTask(ctx, bookdb.CreateTaskParams{Tenant: "example-solo", Repo: "example/app", Title: "Other"})
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range []int64{task.ID, other.ID} {
		if _, err := q.EnqueueJob(ctx, bookdb.EnqueueJobParams{Tenant: "example-solo", TaskID: id, Role: "builder", Provider: "fake", Payload: []byte(`{}`)}); err != nil {
			t.Fatal(err)
		}
	}
	m := &flow.Mover{Pool: pool}
	if _, err := m.Apply(ctx, flow.Move{TaskID: task.ID, Event: flow.LeadStarted, Actor: "system", Profile: flow.Solo}); err != nil {
		t.Fatal(err)
	}
	if got, err := m.Apply(ctx, flow.Move{TaskID: task.ID, Event: flow.Cancel, Actor: "U1", Profile: flow.Solo}); err != nil || got.State != "canceled" {
		t.Fatalf("cancel: %+v %v", got, err)
	}
	var canceled, queued int
	if err := pool.QueryRow(ctx, `SELECT count(*) FILTER (WHERE state = 'canceled'), count(*) FILTER (WHERE state = 'queued') FROM book.jobs WHERE task_id = $1 OR task_id = $2`, task.ID, other.ID).Scan(&canceled, &queued); err != nil {
		t.Fatal(err)
	}
	if canceled != 1 || queued != 1 {
		t.Fatalf("canceled %d, still queued %d: another task's job must stay", canceled, queued)
	}
}
