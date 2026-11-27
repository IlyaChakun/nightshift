package deploy_test

import (
	"testing"

	"github.com/ilyachakun/nightshift/internal/control/deploy"
	"github.com/ilyachakun/nightshift/internal/data/book/bookdb"
	"github.com/ilyachakun/nightshift/internal/testing/testutil/pgtest"
)

const (
	revOne = "1111111111111111111111111111111111111111"
	revTwo = "2222222222222222222222222222222222222222"
)

func TestBookJournalKeepsThePreviousRevisionAcrossARestart(t *testing.T) {
	pool := pgtest.Start(t).ServicePool(t)
	ctx := t.Context()
	q := bookdb.New(pool)
	task, err := q.CreateTask(ctx, bookdb.CreateTaskParams{Tenant: "example-solo", Repo: "example/app", Title: "Release"})
	if err != nil {
		t.Fatal(err)
	}
	j := deploy.BookJournal{Q: q}
	if s, err := j.Start(ctx, "example-solo", task.ID, deploy.StepRelease); err != nil || s.Done {
		t.Fatalf("start: %+v %v", s, err)
	}
	if err := j.Finish(ctx, task.ID, deploy.Step{Name: deploy.StepRelease, Revision: revTwo, Previous: revOne, OK: true}); err != nil {
		t.Fatal(err)
	}
	again, err := (deploy.BookJournal{Q: bookdb.New(pool)}).Start(ctx, "example-solo", task.ID, deploy.StepRelease)
	if err != nil || !again.Done || again.Previous != revOne || again.Revision != revTwo || !again.OK {
		t.Fatalf("restart: %+v %v", again, err)
	}
	// A preview step is forgotten by ClearPreview; the release rows stay.
	if _, err := j.Start(ctx, "example-solo", task.ID, deploy.StepUp); err != nil {
		t.Fatal(err)
	}
	if err := j.Finish(ctx, task.ID, deploy.Step{Name: deploy.StepUp, Revision: revOne, OK: true}); err != nil {
		t.Fatal(err)
	}
	if err := j.ClearPreview(ctx, task.ID); err != nil {
		t.Fatal(err)
	}
	if up, _ := j.Start(ctx, "example-solo", task.ID, deploy.StepUp); up.Done {
		t.Error("the up step survived ClearPreview")
	}
	if rel, _ := j.Start(ctx, "example-solo", task.ID, deploy.StepRelease); !rel.Done {
		t.Error("ClearPreview removed a release step")
	}
}
