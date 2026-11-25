package dispatch_test

import (
	"strings"
	"testing"

	"github.com/ilyachakun/nightshift/internal/adapters/chat/inbox"
	"github.com/ilyachakun/nightshift/internal/dispatch"
	"github.com/ilyachakun/nightshift/internal/flow"
	"github.com/ilyachakun/nightshift/internal/integration"
)

// TestInboxDoorsRunTheLeadDialogWithoutSlack is the T4 scenario of ADR 0006: an idea, Lead's
// question, the owner's reply, the spec card and Approve, all through the doors and the book.
func TestInboxDoorsRunTheLeadDialogWithoutSlack(t *testing.T) {
	r, _ := leadRig(t)
	ctx := t.Context()
	r.tenant.Repos = []string{"example/app"}
	r.c.Outbox = &dispatch.Outbox{
		Pool: r.c.Pool, Mask: dispatch.NewMasker(),
		Senders: map[string]dispatch.Sender{dispatch.ChatTarget: dispatch.ChatSender{"example-solo": inbox.New(r.c.Pool)}},
	}
	d := &dispatch.Doors{Pool: r.c.Pool, Outbox: r.c.Outbox, Mover: r.c.Mover}

	task, err := d.AddIdea(ctx, r.tenant, "", "Add CSV export", "mcp")
	if err != nil {
		t.Fatal(err)
	}
	r.task = task
	r.advance() // idea -> grilling, a Lead job
	r.leadSays(`{"type":"question","text":"Which columns?","options":[{"label":"All"},{"label":"Visible only"}],"recommended":0,"basis":{"checked":["docs/design.md section 7"],"confidence":"high"}}`)
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
	r.leadSays(`{"type":"blocker_or_summary","text":"Spec is ready.","spec_ready":true,"basis":{"checked":["docs/design.md section 7"],"confidence":"high"},"spec":` + leadSpec + `}`)
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
	card := integration.Card{Tenant: "example-solo", TaskID: itoa(r.task.ID), Version: 1, Kind: integration.CardQuestion, Text: "Which columns?"}
	for range 2 {
		if _, err := chat.Post(t.Context(), card); err != nil {
			t.Fatal(err)
		}
	}
	events, _ := r.q.ListTaskEvents(t.Context(), taskID(r.task.ID))
	n := 0
	for _, e := range events {
		if e.Kind == inbox.EventCard {
			n++
		}
	}
	if n != 1 {
		t.Fatalf("%d stored cards, want 1", n)
	}
}
