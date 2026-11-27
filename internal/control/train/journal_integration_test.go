package train_test

import (
	"testing"

	"github.com/ilyachakun/nightshift/internal/control/train"
	"github.com/ilyachakun/nightshift/internal/data/book/bookdb"
	"github.com/ilyachakun/nightshift/internal/testutil/pgtest"
)

func TestBookJournalSurvivesARestart(t *testing.T) {
	pool := pgtest.Start(t).ServicePool(t)
	ctx := t.Context()
	q := bookdb.New(pool)
	task, err := q.CreateTask(ctx, bookdb.CreateTaskParams{Tenant: "example-solo", Repo: "example/app", Title: "Train"})
	if err != nil {
		t.Fatal(err)
	}
	j := train.BookJournal{Q: q}
	first, err := j.Start(ctx, "example-solo", task.ID, train.StepRebase)
	if err != nil || !first.Fresh || first.Done {
		t.Fatalf("first start: %+v %v", first, err)
	}
	if err := j.Finish(ctx, task.ID, train.StepRebase, newHead); err != nil {
		t.Fatal(err)
	}
	// A new process starts the step again and finds it finished, with its sha.
	again, err := (train.BookJournal{Q: bookdb.New(pool)}).Start(ctx, "example-solo", task.ID, train.StepRebase)
	if err != nil || again.Fresh || !again.Done || again.SHA != newHead {
		t.Fatalf("restart: %+v %v", again, err)
	}
	if err := j.Clear(ctx, task.ID); err != nil {
		t.Fatal(err)
	}
	if fresh, err := j.Start(ctx, "example-solo", task.ID, train.StepRebase); err != nil || !fresh.Fresh {
		t.Fatalf("after clear: %+v %v", fresh, err)
	}
}
