package dispatch_test

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/ilyachakun/nightshift/internal/adapters/chat/inbox"
	"github.com/ilyachakun/nightshift/internal/control/chatapi"
	"github.com/ilyachakun/nightshift/internal/control/dispatch"
	"github.com/ilyachakun/nightshift/internal/control/flow"
	"github.com/ilyachakun/nightshift/internal/control/mask"
	"github.com/ilyachakun/nightshift/internal/control/outbox"
)

// TestInboxDoorsRunTheLeadDialogWithoutSlack is the T4 scenario of ADR 0006: an idea, Lead's
// question, the owner's reply, the spec card and Approve, all through the doors and the book.
func TestInboxDoorsRunTheLeadDialogWithoutSlack(t *testing.T) {
	r, _ := leadRig(t)
	ctx := t.Context()
	r.tenant.Repos = []string{"example/app"}
	r.c.Outbox = &outbox.Outbox{
		Pool: r.c.Pool, Mask: mask.NewMasker(),
		Senders: map[string]outbox.Sender{outbox.ChatTarget: outbox.ChatSender{"example-solo": inbox.New(r.c.Pool)}},
	}
	d := &dispatch.Doors{Pool: r.c.Pool, Outbox: r.c.Outbox, Mover: r.c.Mover}

	task, err := d.AddIdea(ctx, r.tenant, "", "Add CSV export", "mcp")
	if err != nil {
		t.Fatal(err)
	}
	r.task = task
	r.advance() // idea -> grilling, a Lead job
	r.leadSays(`{"type":"question","understood":"The owner wants the change described in the task.","text":"Which columns?","options":[{"label":"All"},{"label":"Visible only"}],"recommended":0,"basis":{"checked":["docs/design.md section 7"],"confidence":"high"}}`)
	r.advance()
	if _, err := r.c.Outbox.Flush(ctx); err != nil {
		t.Fatal(err)
	}

	pending, err := d.Inbox(ctx, r.tenant)
	if err != nil || len(pending) != 1 || !strings.Contains(pending[0].Text, "Which columns?") {
		t.Fatalf("inbox: %+v %v", pending, err)
	}
	lines, _ := d.Status(ctx, r.tenant) // the rig holds two other open tasks
	var grilling bool
	for _, l := range lines {
		grilling = grilling || (l.ID == task.ID && l.State == "grilling")
	}
	if !grilling {
		t.Fatalf("status: %+v", lines)
	}

	if err := d.Reply(ctx, r.tenant, task.ID, "All columns", "mcp"); err != nil {
		t.Fatal(err)
	}
	if pending, _ := d.Inbox(ctx, r.tenant); len(pending) != 0 {
		t.Fatalf("an answered question is still pending: %+v", pending)
	}
	r.advance() // a second Lead job carries the answer
	r.leadSays(`{"type":"blocker_or_summary","text":"Spec is ready.","spec_ready":true,"understood":"The owner wants the change described in the task.","basis":{"checked":["docs/design.md section 7"],"confidence":"high"},"spec":` + leadSpec + `}`)
	r.advance()
	if _, err := r.c.Outbox.Flush(ctx); err != nil {
		t.Fatal(err)
	}
	pending, _ = d.Inbox(ctx, r.tenant)
	if len(pending) != 1 || len(pending[0].Buttons) != 3 || pending[0].Version != int(r.task.Version) {
		t.Fatalf("spec card: %+v", pending)
	}

	if err := d.Approve(ctx, r.tenant, task.ID, "cli"); err != nil {
		t.Fatal(err)
	}
	r.reload()
	if r.task.State != string(flow.Queued) {
		t.Fatalf("approval left the task in %s", r.task.State)
	}
	events, _ := r.q.ListTaskEvents(ctx, taskID(task.ID))
	var moved bool
	for _, e := range events {
		moved = moved || (e.Kind == "task.moved" && e.Actor == "cli:U1")
	}
	if !moved {
		t.Fatal("the decision does not record its door")
	}
	if err := d.Approve(ctx, r.tenant, task.ID, "cli"); err == nil {
		t.Fatal("approving a task that waits for nothing must fail")
	}
}

func TestDoorsRefuseAnotherTenantsTaskAndAMissingOwner(t *testing.T) {
	r, _ := leadRig(t)
	d := &dispatch.Doors{Pool: r.c.Pool, Outbox: r.c.Outbox, Mover: r.c.Mover}
	other := r.tenant
	other.Name = "example-team"
	if err := d.Reply(t.Context(), other, r.task.ID, "hi", "cli"); err == nil {
		t.Fatal("a tenant answered another tenant's task")
	}
	noOwner := r.tenant
	noOwner.People.Owner.Chat = ""
	if _, err := d.AddIdea(t.Context(), noOwner, "", "x", "cli"); err == nil {
		t.Fatal("a tenant without an owner chat id got a task")
	}
	if _, err := d.AddIdea(t.Context(), r.tenant, "other-org/private", "x", "mcp"); err == nil {
		t.Fatal("an idea started a task in a repository the tenant does not have")
	}
	if _, err := d.AddIdea(t.Context(), r.tenant, "", "  ", "mcp"); err == nil {
		t.Fatal("an empty idea started a task")
	}
}

func TestPostingTheSameCardTwiceStoresItOnce(t *testing.T) {
	r, _ := leadRig(t)
	chat := inbox.New(r.c.Pool)
	card := chatapi.Card{Tenant: "example-solo", TaskID: itoa(r.task.ID), Version: 1, Kind: chatapi.CardQuestion, Text: "Which columns?"}
	for range 2 {
		if _, err := chat.Post(t.Context(), card); err != nil {
			t.Fatal(err)
		}
	}
	events, _ := r.q.ListTaskEvents(t.Context(), taskID(r.task.ID))
	n := 0
	for _, e := range events {
		if e.Kind == chatapi.InboxEventCard {
			n++
		}
	}
	if n != 1 {
		t.Fatalf("%d stored cards, want 1", n)
	}
}

func TestARepeatedCardIsKeptAndARedeliveryIsNot(t *testing.T) {
	r, _ := leadRig(t)
	chat := inbox.New(r.c.Pool)
	card := chatapi.Card{Tenant: "example-solo", TaskID: itoa(r.task.ID), Version: 1, Kind: chatapi.CardBlocker, Text: "I could not answer this time."}
	for _, delivery := range []int64{10, 10, 11} {
		card.Delivery = delivery
		if _, err := chat.Post(t.Context(), card); err != nil {
			t.Fatal(err)
		}
	}
	events, _ := r.q.ListTaskEvents(t.Context(), taskID(r.task.ID))
	n := 0
	for _, e := range events {
		if e.Kind == chatapi.InboxEventCard {
			n++
		}
	}
	if n != 2 {
		t.Fatalf("%d stored cards, want 2 (delivery 10 once, delivery 11 once)", n)
	}
}

func TestApprovalQueuesLeadTasksWithTheirSchedule(t *testing.T) {
	t.Run("single", func(t *testing.T) { r := approveScheduledTasks(t, false); checkSingleSchedule(t, r) })
	t.Run("multi", func(t *testing.T) { r := approveScheduledTasks(t, true); checkEligibleSibling(t, r) })
}
func approveScheduledTasks(t *testing.T, siblings bool) *rig {
	t.Helper()
	r, rec := leadRig(t)
	ctx := t.Context()
	due := time.Now().UTC().Add(24 * time.Hour).Truncate(time.Second).Add(123456 * time.Microsecond)
	tasks := `{"id":"T-1","title":"Add the export","not_before":"` + strings.ToLower(due.Format(time.RFC3339Nano)) + `","priority":-4.0}`
	if siblings {
		tasks += `,{"id":"T-2","title":"Prepare the data","priority":5}`
	}
	spec := strings.Replace(leadSpec, `{"id":"T-1","title":"Add the export"}`, tasks, 1)
	r.advance()
	r.leadSays(`{"type":"blocker_or_summary","text":"Spec is ready.","spec_ready":true,"understood":"The owner wants a scheduled export.","basis":{"checked":["docs/design.md section 7"],"confidence":"high"},"spec":` + spec + `}`)
	r.advance()
	if _, err := r.c.Outbox.Flush(ctx); err != nil {
		t.Fatal(err)
	}
	if card := rec.last().Text; !strings.Contains(card, due.Format(time.RFC3339Nano)) || !strings.Contains(card, "priority -4") {
		t.Fatalf("schedule missing from spec card: %s", card)
	}
	d := dispatch.Doors{Pool: r.c.Pool, Outbox: r.c.Outbox, Mover: r.c.Mover}
	if err := d.Approve(ctx, r.tenant, r.task.ID, "cli"); err != nil {
		t.Fatal(err)
	}
	r.reload()
	r.advance()
	if r.task.State != "queued" || !r.task.NotBefore.Valid || !r.task.NotBefore.Time.Equal(due) || r.task.Priority != -4 {
		t.Fatalf("scheduled task %d: %s, %v, priority %d", r.task.ID, r.task.State, r.task.NotBefore, r.task.Priority)
	}
	return r
}

func checkSingleSchedule(t *testing.T, r *rig) {
	t.Helper()
	ctx := t.Context()
	events, err := r.q.ListTaskEvents(ctx, taskID(r.task.ID))
	if err != nil {
		t.Fatal(err)
	}
	scheduledCount := 0
	for _, e := range events {
		if e.Kind == "spec.expanded" {
			t.Fatal("single task marked as split: Verifier would lose the approved spec")
		}
		if e.Kind == "spec.scheduled" {
			scheduledCount++
		}
	}
	if scheduledCount != 1 {
		t.Fatalf("single task schedule recorded %d times", scheduledCount)
	}
}

func checkEligibleSibling(t *testing.T, r *rig) {
	t.Helper()
	ctx := t.Context()
	found := false
	for _, task := range r.siblings() {
		if task.Title != "Prepare the data" {
			continue
		}
		found = true
		checkExpansionMap(t, r, map[string]int64{"T-1": r.task.ID, "T-2": task.ID})
		if task.Priority != 5 || task.NotBefore.Valid {
			t.Fatalf("sibling priority %d, time %v", task.Priority, task.NotBefore)
		}
		if err := r.c.Advance(ctx, r.tenant, task); err != nil {
			t.Fatal(err)
		}
		moved, err := r.q.GetTask(ctx, task.ID)
		if err != nil || moved.State != "building" {
			t.Fatalf("eligible sibling: %s, %v", moved.State, err)
		}
	}
	if !found {
		t.Fatal("missing eligible sibling")
	}
}

// checkExpansionMap reads which book task each spec task became from the one spec.expanded event.
func checkExpansionMap(t *testing.T, r *rig, want map[string]int64) {
	t.Helper()
	events, err := r.q.ListTaskEvents(t.Context(), taskID(r.task.ID))
	if err != nil {
		t.Fatal(err)
	}
	var maps []map[string]int64
	for _, e := range events {
		if e.Kind != "spec.expanded" {
			continue
		}
		var p struct {
			Tasks map[string]int64 `json:"tasks"`
		}
		if err := json.Unmarshal(e.Payload, &p); err != nil {
			t.Fatal(err)
		}
		maps = append(maps, p.Tasks)
	}
	if len(maps) != 1 || !reflect.DeepEqual(maps[0], want) {
		t.Fatalf("expansion maps %v, want one %v", maps, want)
	}
}
