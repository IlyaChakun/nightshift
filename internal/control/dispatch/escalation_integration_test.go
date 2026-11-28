package dispatch_test

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/ilyachakun/nightshift/internal/control/chatapi"
	"github.com/ilyachakun/nightshift/internal/control/doors"
	"github.com/ilyachakun/nightshift/internal/control/flow"
	"github.com/ilyachakun/nightshift/internal/control/mask"
	"github.com/ilyachakun/nightshift/internal/control/outbox"
)

// decisionCards are the "Decision needed" cards queued for the rig's task.
func (r *rig) decisionCards() []chatapi.Card { return r.cardsKeyed("escalated:%") }

// lastMove is the flow event of the task's last recorded move.
func (r *rig) lastMove() string {
	r.t.Helper()
	events, err := r.q.ListTaskEvents(r.t.Context(), pgtype.Int8{Int64: r.task.ID, Valid: true})
	if err != nil {
		r.t.Fatal(err)
	}
	last := ""
	for _, e := range events {
		var p struct {
			Event string `json:"event"`
		}
		if e.Kind == "task.moved" && json.Unmarshal(e.Payload, &p) == nil {
			last = p.Event
		}
	}
	return last
}

func escalationRig(t *testing.T, lang string) *rig {
	t.Helper()
	r := newRig(t)
	r.c.Outbox = &outbox.Outbox{Pool: r.c.Pool, Mask: mask.NewMasker()}
	r.c.ChatLanguage = lang
	r.tenant.People.Owner.Chat = "U1"
	return r
}

// T4, regression of V4 on the Mac (2026-11-28): an acceptance the Verifier escalated reached
// `escalated` as budget_exhausted and the owner heard nothing. It now moves as `escalate` and
// queues one "Decision needed" card with the reason, the preview, the screenshots and the
// three ways on.
func TestAnEscalatedAcceptanceAsksTheOwner(t *testing.T) {
	for lang, want := range map[string]string{"en": "needs your decision", "ru": "нужно ваше решение"} {
		t.Run(lang, func(t *testing.T) {
			r := escalationRig(t, lang)
			r.withVerifier()
			r.toAcceptance()
			r.completeAcceptance(`{"verdict":"escalate","summary":"two scenarios need a shell","escalation_reason":"two scenarios cannot be observed in the preview"}`)
			r.advance()
			r.advance() // a later tick queues no second card
			if r.task.State != string(flow.Escalated) || r.lastMove() != string(flow.Escalate) {
				t.Fatalf("task in %s after %s, want escalated after escalate", r.task.State, r.lastMove())
			}
			checkDecisionCard(t, r, want)
		})
	}
}

// checkDecisionCard checks the one decision card of an escalated acceptance.
func checkDecisionCard(t *testing.T, r *rig, want string) {
	t.Helper()
	cards := r.decisionCards()
	if len(cards) != 1 {
		t.Fatalf("%d decision cards, want 1", len(cards))
	}
	c := cards[0]
	var actions []string
	for _, b := range c.Buttons {
		actions = append(actions, b.Action)
	}
	if c.Kind != chatapi.CardDecisionNeeded || c.Thread.Channel != "U1" || c.Version != int(r.task.Version) ||
		strings.Join(actions, ",") != "continue,revisit,cancel_task" {
		t.Fatalf("card %+v", c)
	}
	for _, s := range []string{want, "cannot be observed in the preview", ".preview.invalid", "/evidence/01.png"} {
		if !strings.Contains(c.Text, s) {
			t.Errorf("the card misses %q:\n%s", s, c.Text)
		}
	}
	if strings.Contains(c.Text, hiddenStep) || strings.Contains(c.Text, "scenarios.json") {
		t.Errorf("the card shows a hidden scenario:\n%s", c.Text)
	}
}

// An escalation reason that quotes a hidden scenario is withheld from the chat.
func TestAnEscalationReasonThatQuotesAScenarioIsWithheld(t *testing.T) {
	r := escalationRig(t, "en")
	r.withVerifier()
	r.toAcceptance()
	r.completeAcceptance(`{"verdict":"escalate","summary":"x","escalation_reason":"cannot check that ` + hiddenStep + `"}`)
	r.advance()
	cards := r.decisionCards()
	if len(cards) != 1 || strings.Contains(cards[0].Text, hiddenStep) || !strings.Contains(cards[0].Text, "withheld") {
		t.Fatalf("cards %+v", cards)
	}
}

// A failed Builder job escalates with a card too, and the owner's click on it moves the task.
func TestTheOwnerDecidesAnEscalatedTask(t *testing.T) {
	for action, want := range map[string]flow.State{"continue": flow.Building, "revisit": flow.Grilling, "cancel_task": flow.Canceled} {
		t.Run(action, func(t *testing.T) {
			r := escalationRig(t, "en")
			r.task.ThreadRef = pgtype.Text{String: "C1/1.0", Valid: true}
			if _, err := r.c.Pool.Exec(t.Context(), `UPDATE book.tasks SET thread_ref = $2 WHERE id = $1`, r.task.ID, r.task.ThreadRef.String); err != nil {
				t.Fatal(err)
			}
			r.advance() // queues the Builder job
			if _, err := r.c.Pool.Exec(t.Context(), `UPDATE book.jobs SET state = 'failed', exit_reason = 'error', finished_at = now() WHERE id = $1`, r.lastJob().ID); err != nil {
				t.Fatal(err)
			}
			r.advance()
			cards := r.decisionCards()
			if r.task.State != string(flow.Escalated) || len(cards) != 1 || cards[0].Thread.Channel != "C1" || !strings.Contains(cards[0].Text, "job failed") {
				t.Fatalf("task in %s, cards %+v", r.task.State, cards)
			}
			inbox := &doors.Inbox{Pool: r.c.Pool, Outbox: r.c.Outbox, Mover: r.c.Mover}
			if err := inbox.Handle(t.Context(), r.tenant, chatapi.ChatEvent{Kind: chatapi.ChatButton, User: "U1", Action: action, TaskID: itoa(r.task.ID), CardVersion: cards[0].Version}); err != nil {
				t.Fatal(err)
			}
			r.reload()
			if r.task.State != string(want) {
				t.Fatalf("%s left the task in %s, want %s", action, r.task.State, want)
			}
		})
	}
}

// The escalation buttons do nothing on a task that is not escalated.
func TestEscalationButtonsNeedAnEscalatedTask(t *testing.T) {
	r := escalationRig(t, "en")
	r.advance() // building
	inbox := &doors.Inbox{Pool: r.c.Pool, Outbox: r.c.Outbox, Mover: r.c.Mover}
	for _, action := range []string{"continue", "revisit"} {
		if err := inbox.Handle(t.Context(), r.tenant, chatapi.ChatEvent{Kind: chatapi.ChatButton, User: "U1", Action: action, TaskID: itoa(r.task.ID), CardVersion: int(r.task.Version)}); err != nil {
			t.Fatal(err)
		}
	}
	r.reload()
	if r.task.State != string(flow.Building) {
		t.Fatalf("a click moved a building task to %s", r.task.State)
	}
}
