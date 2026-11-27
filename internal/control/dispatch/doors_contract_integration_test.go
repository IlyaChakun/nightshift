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

// moves lists the task's state changes as "actor kind".
func moves(r *rig) []string {
	r.t.Helper()
	events, err := r.q.ListTaskEvents(r.t.Context(), taskID(r.task.ID))
	if err != nil {
		r.t.Fatal(err)
	}
	var out []string
	for _, e := range events {
		if e.Kind == "task.moved" {
			out = append(out, e.Actor)
		}
	}
	return out
}

// TestEveryDoorApprovesTheSameWay is the doors contract (docs/plan/architecture/ item A7): chat,
// terminal and MCP give one approval the same effect, record their door, and refuse or ignore an
// approval that comes again or answers an older card.
func TestEveryDoorApprovesTheSameWay(t *testing.T) {
	// effect is the state and the kinds of events an approval leaves, by door. The book starts
	// each door's case in parallel, so they are compared once the group is done.
	var mu sync.Mutex
	effect := map[string][]string{}
	t.Run("doors", func(t *testing.T) {
		for _, d := range theDoors() {
			t.Run(d.name, func(t *testing.T) {
				r := specReady(t)
				before := len(moves(r))
				if err := d.approve(r, int(r.task.Version)); err != nil {
					t.Fatalf("approve: %v", err)
				}
				r.reload()
				got := moves(r)[before:]
				if r.task.State != string(flow.Queued) || !slices.Equal(got, []string{d.actor}) {
					t.Fatalf("after approval: state %s, moves %v; want queued, one move by %s", r.task.State, got, d.actor)
				}
				kinds := []string{r.task.State}
				events, _ := r.q.ListTaskEvents(t.Context(), taskID(r.task.ID))
				for _, e := range events[before:] {
					kinds = append(kinds, e.Kind)
				}
				mu.Lock()
				effect[d.name] = kinds
				mu.Unlock()

				// The same approval again, and one on the card before it: nothing more moves.
				for _, version := range []int{int(r.task.Version), int(r.task.Version) - 1} {
					err := d.approve(r, version)
					if d.refusesAgain != (err != nil) || (err != nil && !errors.Is(err, doors.ErrDoor)) {
						t.Errorf("approval again (card %d): err = %v, want refused: %v", version, err, d.refusesAgain)
					}
				}
				r.reload()
				if after := moves(r)[before:]; r.task.State != string(flow.Queued) || len(after) != 1 {
					t.Errorf("a repeated approval moved the task: state %s, moves %v", r.task.State, after)
				}
			})
		}
	})
	for name, kinds := range effect {
		if !slices.Equal(kinds, effect["chat"]) {
			t.Errorf("a %s approval left %v; a chat approval left %v", name, kinds, effect["chat"])
		}
	}
}

// TestAChatClickOnAnOlderCardChangesNothing: the spec card was replaced (the owner asked for an
// edit), so a click on the card before it must not approve the new spec unseen.
func TestAChatClickOnAnOlderCardChangesNothing(t *testing.T) {
	r := specReady(t)
	before := len(moves(r))
	if err := theDoors()[0].approve(r, int(r.task.Version)-1); err != nil {
		t.Fatal(err)
	}
	r.reload()
	if r.task.State != string(flow.Spec) || len(moves(r)) != before {
		t.Fatalf("a click on an older card moved the task to %s", r.task.State)
	}
}
