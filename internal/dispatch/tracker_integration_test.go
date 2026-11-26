package dispatch_test

import (
	"testing"

	"github.com/ilyachakun/nightshift/internal/adapters/tracker/fake"
	"github.com/ilyachakun/nightshift/internal/book/bookdb"
	"github.com/ilyachakun/nightshift/internal/config"
	"github.com/ilyachakun/nightshift/internal/dispatch"
	"github.com/ilyachakun/nightshift/internal/flow"
	"github.com/ilyachakun/nightshift/internal/integration"
)

// The tracker is a projection: tasks reach it through the outbox, and only a cancel or a priority
// from the owner or an approver comes back. A stranger's change is ignored.
func TestTrackerProjectionAndTheTwoThingsThatComeBack(t *testing.T) {
	r, tr, run := trackerRig(t)

	run()
	v, ok := tr.Issue("FAKE-1")
	if !ok || v.TaskID != itoa(r.task.ID) || v.State != r.task.State || v.Priority != "P2" {
		t.Fatalf("projected view: %+v %v", v, ok)
	}
	run() // nothing changed: the same key, no second message
	if n := countOutbox(t, r, dispatch.TrackerTarget); n != 1 {
		t.Fatalf("%d tracker messages for one state", n)
	}

	r.advance() // the task moves; the next run projects the new state
	run()
	if v, _ = tr.Issue("FAKE-1"); v.State != r.task.State {
		t.Fatalf("the issue shows %s, the task is %s", v.State, r.task.State)
	}

	// A stranger's cancel and an unknown priority change nothing.
	if err := tr.Cancel("FAKE-1", "U-STRANGER"); err != nil {
		t.Fatal(err)
	}
	run()
	r.reload()
	if flow.Terminal(flow.State(r.task.State)) {
		t.Fatal("a stranger canceled the task")
	}
	// The owner changes the priority, then cancels.
	if err := tr.SetPriority("FAKE-1", "U1", "P0"); err != nil {
		t.Fatal(err)
	}
	run()
	r.reload()
	if r.task.Priority != 10 {
		t.Fatalf("priority %d after P0", r.task.Priority)
	}
	run() // the new priority is a change of its own and reaches the issue
	if v, _ = tr.Issue("FAKE-1"); v.Priority != "P0" {
		t.Fatalf("the issue shows priority %s", v.Priority)
	}
	if err := tr.Cancel("FAKE-1", "U1"); err != nil {
		t.Fatal(err)
	}
	run()
	r.reload()
	if r.task.State != string(flow.Canceled) {
		t.Fatalf("the owner's cancel left the task in %s", r.task.State)
	}
	run() // the same cancel again, on a finished task, is no error
	run()
	if v, _ = tr.Issue("FAKE-1"); v.State != "canceled" {
		t.Fatalf("the issue shows %s after the cancel", v.State)
	}

}

func TestTrackerWaitsForApproval(t *testing.T) {
	r, tr, run := trackerRig(t)
	ctx := t.Context()
	draft, err := r.q.CreateTask(ctx, bookdb.CreateTaskParams{Tenant: r.tenant.Name, Repo: "example/app", Title: "Unapproved idea"})
	if err != nil {
		t.Fatal(err)
	}
	moveDraft := func(event flow.Event) {
		t.Helper()
		if _, err := r.c.Mover.Apply(ctx, flow.Move{TaskID: draft.ID, Event: event, Actor: "owner", Profile: flow.Solo}); err != nil {
			t.Fatal(err)
		}
	}
	for _, event := range []flow.Event{flow.LeadStarted, flow.DocsDrafted, flow.DocLoopPassed} {
		moveDraft(event)
	}
	// An older release may have queued a tracker delivery for this spec.
	sender := r.c.Outbox.Senders[dispatch.TrackerTarget]
	if err := sender.Send(ctx, dispatch.Message{Tenant: r.tenant.Name, TaskID: draft.ID}); err != nil {
		t.Fatal(err)
	}
	if _, ok := tr.Issue("FAKE-1"); ok {
		t.Fatal("stale delivery projected an unapproved spec")
	}

	run()
	if n := countOutbox(t, r, dispatch.TrackerTarget); n != 1 {
		t.Fatalf("projector enqueued %d messages for one approved task", n)
	}
	moveDraft(flow.Approve)
	run()
	if v, ok := tr.Issue("FAKE-2"); !ok || v.TaskID != itoa(draft.ID) || v.State != string(flow.Queued) {
		t.Fatalf("approval did not project the queued task: %+v, %v", v, ok)
	}
}

func trackerRig(t *testing.T) (*rig, *fake.Tracker, func()) {
	t.Helper()
	r := newRig(t)
	ctx := t.Context()
	r.tenant.People.Owner.Chat = "U1"
	tr := fake.New(nil)
	r.c.Outbox = &dispatch.Outbox{
		Pool: r.c.Pool, Mask: dispatch.NewMasker(),
		Senders: map[string]dispatch.Sender{dispatch.TrackerTarget: dispatch.TrackerSender{Pool: r.c.Pool, Trackers: map[string]integration.Tracker{"example-solo": tr}}},
	}
	p := &dispatch.Projector{
		Pool: r.c.Pool, Outbox: r.c.Outbox, Mover: r.c.Mover,
		Tracker: func(config.Tenant) integration.Tracker { return tr },
	}
	run := func() {
		t.Helper()
		p.Run(ctx, r.tenant)
		if _, err := r.c.Outbox.Flush(ctx); err != nil {
			t.Fatal(err)
		}
	}

	return r, tr, run
}

func countOutbox(t *testing.T, r *rig, target string) int {
	t.Helper()
	var n int
	if err := r.c.Pool.QueryRow(t.Context(), `SELECT count(*) FROM book.outbox WHERE target = $1`, target).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

// A retried older message must not put an old state over a newer one: the sender projects the
// task as it is now.
func TestTrackerSenderProjectsTheCurrentTaskNotTheMessage(t *testing.T) {
	r := newRig(t)
	tr := fake.New(nil)
	s := dispatch.TrackerSender{Pool: r.c.Pool, Trackers: map[string]integration.Tracker{"example-solo": tr}}
	stale := []byte(`{"tenant":"example-solo","task_id":"1","title":"old","state":"idea","priority":"P3"}`)
	if err := s.Send(t.Context(), dispatch.Message{Tenant: "example-solo", TaskID: r.task.ID, Payload: stale}); err != nil {
		t.Fatal(err)
	}
	if v, _ := tr.Issue("FAKE-1"); v.State != r.task.State || v.Title != r.task.Title || v.Priority != "P2" {
		t.Fatalf("the issue shows the stale message: %+v", v)
	}
}
