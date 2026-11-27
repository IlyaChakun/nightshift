package doors

import (
	"errors"
	"strconv"
	"testing"

	"github.com/ilyachakun/nightshift/internal/control/chatapi"
	"github.com/ilyachakun/nightshift/internal/control/flow"
	"github.com/ilyachakun/nightshift/internal/customization/config"
	"github.com/ilyachakun/nightshift/internal/data/book/bookdb"
	"github.com/ilyachakun/nightshift/internal/testing/testutil/pgtest"
)

// TestAReplacedCardIsReportedByADoor: when the card an approval names was replaced before the
// click landed, chat drops it in silence and a door says so; neither moves the task.
func TestAReplacedCardIsReportedByADoor(t *testing.T) {
	pool := pgtest.Start(t).ServicePool(t)
	ctx := t.Context()
	q := bookdb.New(pool)
	tenant := config.Tenant{Name: "example-solo"}
	tenant.People.Owner.Chat = "U1"
	task, err := q.CreateTask(ctx, bookdb.CreateTaskParams{Tenant: tenant.Name, Repo: "example/app", Title: "Stale"})
	if err != nil {
		t.Fatal(err)
	}
	m := &flow.Mover{Pool: pool}
	for _, e := range []flow.Event{flow.LeadStarted, flow.DocsDrafted, flow.DocLoopPassed} {
		if task, err = m.Apply(ctx, flow.Move{TaskID: task.ID, Event: e, Actor: "system", Profile: flow.Solo}); err != nil {
			t.Fatal(err)
		}
	}
	if task.State != string(flow.Spec) {
		t.Fatalf("task is in %s, want spec", task.State)
	}
	in := &Inbox{Pool: pool, Mover: m}
	stale := chatapi.ChatEvent{Kind: chatapi.ChatButton, User: "U1", Action: "approve", TaskID: strconv.FormatInt(task.ID, 10), CardVersion: int(task.Version) - 1}
	if err := in.button(ctx, tenant, stale); !errors.Is(err, errStaleCard) {
		t.Fatalf("button on a replaced card: %v, want errStaleCard", err)
	}
	if err := in.Handle(ctx, tenant, stale); err != nil {
		t.Fatalf("chat must drop a replaced card in silence: %v", err)
	}
	d := &Doors{Pool: pool, Mover: m}
	if err := d.Approve(ctx, tenant, task.ID, int(task.Version)-1, "cli"); !errors.Is(err, ErrDoor) {
		t.Fatalf("a door approving a replaced card: %v, want ErrDoor", err)
	}
	if got, err := q.GetTask(ctx, task.ID); err != nil || got.Version != task.Version || got.State != string(flow.Spec) {
		t.Fatalf("task after the refused approvals: %+v, %v", got, err)
	}
}
