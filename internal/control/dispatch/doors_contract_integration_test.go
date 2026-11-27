package dispatch_test

import (
	"errors"
	"slices"
	"strconv"
	"sync"
	"testing"

	"github.com/ilyachakun/nightshift/internal/control/chatapi"
	"github.com/ilyachakun/nightshift/internal/control/doors"
	"github.com/ilyachakun/nightshift/internal/control/flow"
	"github.com/ilyachakun/nightshift/internal/experience/mcpdoor"
)

// door approves the task's spec card through one entry point. version is the card the owner
// answers; a door without cards answers the current one.
type door struct {
	name    string
	actor   string
	approve func(r *rig, version int) error
	// refusesAgain: a door with a caller to tell says no to an answer nothing waits for; a chat
	// click on an old card changes nothing and says nothing.
	refusesAgain bool
}

func theDoors() []door {
	return []door{
		{name: "chat", actor: "chat:U1", approve: func(r *rig, version int) error {
			in := &doors.Inbox{Pool: r.c.Pool, Outbox: r.c.Outbox, Mover: r.c.Mover}
			return in.Handle(r.t.Context(), r.tenant, chatapi.ChatEvent{
				Kind: chatapi.ChatButton, User: "U1", Action: "approve", TaskID: strconv.FormatInt(r.task.ID, 10), CardVersion: version,
			})
		}},
		{name: "cli", actor: "cli:U1", refusesAgain: true, approve: func(r *rig, _ int) error {
			return (&doors.Doors{Pool: r.c.Pool, Outbox: r.c.Outbox, Mover: r.c.Mover}).Approve(r.t.Context(), r.tenant, r.task.ID, "cli")
		}},
		{name: "mcp", actor: "mcp:U1", refusesAgain: true, approve: func(r *rig, _ int) error {
			d := &doors.Doors{Pool: r.c.Pool, Outbox: r.c.Outbox, Mover: r.c.Mover}
			return mcpdoor.Doors{Doors: d, Tenant: r.tenant, Source: "mcp"}.Approve(r.t.Context(), r.task.ID)
		}},
	}
}

// specReady drives the rig's task to a spec card that waits for the owner.
func specReady(t *testing.T) *rig {
	t.Helper()
	r, _ := leadRig(t)
	r.advance()
	r.leadSays(`{"type":"blocker_or_summary","text":"Spec is ready.","spec_ready":true,"understood":"The owner wants the change described in the task.","basis":{"checked":["docs/design.md section 7"],"confidence":"high"},"spec":` + leadSpec + `}`)
	r.advance()
	if r.task.State != string(flow.Spec) {
		t.Fatalf("the task is in %s, want spec", r.task.State)
	}
	return r
}

// events lists the task's events as "kind" and its state changes as "actor".
func events(r *rig) (kinds, movers []string) {
	r.t.Helper()
	list, err := r.q.ListTaskEvents(r.t.Context(), taskID(r.task.ID))
	if err != nil {
		r.t.Fatal(err)
	}
	for _, e := range list {
		kinds = append(kinds, e.Kind)
		if e.Kind == "task.moved" {
			movers = append(movers, e.Actor)
		}
	}
	return kinds, movers
}

// approveOnce approves the spec through one door and returns the state and the event kinds
// the approval left; it fails the test unless one move, recorded by the door, queued the task.
// It then answers again, on the same card and on the card before it, and checks nothing moves.
func approveOnce(t *testing.T, d door) []string {
	r := specReady(t)
	card := int(r.task.Version)
	kindsBefore, moversBefore := events(r)
	if err := d.approve(r, card); err != nil {
		t.Fatalf("approve: %v", err)
	}
	r.reload()
	kinds, movers := events(r)
	if got := movers[len(moversBefore):]; r.task.State != string(flow.Queued) || !slices.Equal(got, []string{d.actor}) {
		t.Fatalf("after approval: state %s, moves %v; want queued, one move by %s", r.task.State, got, d.actor)
	}
	effect := append([]string{r.task.State}, kinds[len(kindsBefore):]...)

	for _, again := range []int{card, card - 1} {
		err := d.approve(r, again)
		if d.refusesAgain != (err != nil) || (err != nil && !errors.Is(err, doors.ErrDoor)) {
			t.Errorf("approval again (card %d): err = %v, want refused: %v", again, err, d.refusesAgain)
		}
	}
	r.reload()
	if _, after := events(r); r.task.State != string(flow.Queued) || len(after) != len(movers) {
		t.Errorf("a repeated approval moved the task: state %s, moves %v", r.task.State, after[len(moversBefore):])
	}
	return effect
}

// TestEveryDoorApprovesTheSameWay is the doors contract (docs/plan/architecture/ item A7): chat,
// terminal and MCP give one approval the same effect, record their door, and refuse or ignore an
// approval that comes again. Terminal and MCP approve the current spec: they carry no card.
func TestEveryDoorApprovesTheSameWay(t *testing.T) {
	// pgtest.Start runs each door's case in parallel, so the effects are compared once
	// the group is done.
	var mu sync.Mutex
	effect := map[string][]string{}
	t.Run("doors", func(t *testing.T) {
		for _, d := range theDoors() {
			t.Run(d.name, func(t *testing.T) {
				got := approveOnce(t, d)
				mu.Lock()
				defer mu.Unlock()
				effect[d.name] = got
			})
		}
	})
	want, ok := effect["chat"]
	if !ok {
		t.Fatal("the chat door left no effect to compare with")
	}
	for name, kinds := range effect {
		if !slices.Equal(kinds, want) {
			t.Errorf("a %s approval left %v; a chat approval left %v", name, kinds, want)
		}
	}
}

// TestAChatClickOnAnOlderCardChangesNothing: a click on a card older than the one waiting, such as
// a spec card the Lead has since replaced, must not approve the current spec unseen.
func TestAChatClickOnAnOlderCardChangesNothing(t *testing.T) {
	r := specReady(t)
	_, before := events(r)
	if err := theDoors()[0].approve(r, int(r.task.Version)-1); err != nil {
		t.Fatal(err)
	}
	r.reload()
	if _, after := events(r); r.task.State != string(flow.Spec) || len(after) != len(before) {
		t.Fatalf("a click on an older card moved the task to %s", r.task.State)
	}
}
