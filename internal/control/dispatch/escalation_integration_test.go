package dispatch_test

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/ilyachakun/nightshift/internal/adapters/github/fake"
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

// An escalation reason that quotes a hidden scenario reaches the chat as the ids it names, and
// as a fixed sentence when it names none.
func TestAnEscalationReasonThatQuotesAScenarioIsWithheld(t *testing.T) {
	for reason, want := range map[string]string{"HS-1 cannot be checked: " + hiddenStep: "could not check hidden scenarios HS-1", "cannot check that " + hiddenStep: "its text is not shown"} {
		t.Run(want, func(t *testing.T) { checkWithheldReason(t, reason, want) })
	}
}

func checkWithheldReason(t *testing.T, reason, want string) {
	t.Helper()
	r := escalationRig(t, "en")
	r.withVerifier()
	r.toAcceptance()
	r.completeAcceptance(`{"verdict":"escalate","summary":"x","escalation_reason":"` + reason + `"}`)
	r.advance()
	cards := r.decisionCards()
	if len(cards) != 1 || strings.Contains(cards[0].Text, hiddenStep) || !strings.Contains(cards[0].Text, want) {
		t.Fatalf("reason %q: cards %+v", reason, cards)
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

// moveOutside moves the rig's task from state with e through the Mover alone, like a package that
// escalates without the conductor's card (the merge train sending a task back with its budget
// spent), and returns the task's cards after the next sweep.
func (r *rig) moveOutside(state flow.State, e flow.Event) []chatapi.Card {
	r.t.Helper()
	if _, err := r.c.Pool.Exec(r.t.Context(), `UPDATE book.tasks SET state = $2 WHERE id = $1`, r.task.ID, string(state)); err != nil {
		r.t.Fatal(err)
	}
	if _, err := r.c.Mover.Apply(r.t.Context(), flow.Move{TaskID: r.task.ID, Event: e, Actor: "system", Profile: flow.Solo}); err != nil {
		r.t.Fatal(err)
	}
	r.reload()
	if len(r.decisionCards()) != 0 {
		r.t.Fatal("a card before the sweep")
	}
	if err := r.c.AskEscalated(r.t.Context(), r.tenant); err != nil {
		r.t.Fatal(err)
	}
	if err := r.c.AskEscalated(r.t.Context(), r.tenant); err != nil { // a second sweep adds nothing
		r.t.Fatal(err)
	}
	return r.decisionCards()
}

// Review of #491: an escalation outside the conductor's own moves still reaches the owner, with
// the budget as its cause.
func TestTheSweepAsksForAnEscalationWithoutACard(t *testing.T) {
	r := escalationRig(t, "en")
	cards := r.moveOutside(flow.Building, flow.BudgetExhausted)
	if r.task.State != string(flow.Escalated) || len(cards) != 1 || len(cards[0].Buttons) != 3 ||
		!strings.Contains(cards[0].Text, "budget is used up") || strings.Contains(cards[0].Text, "recorded no reason") {
		t.Fatalf("task in %s, cards %+v", r.task.State, cards)
	}
}

// Review of #491: a release that failed its smoke with nothing to roll back to is merged
// already, so its card offers no button that would rework or cancel it.
func TestAMergedEscalationOffersNoButtons(t *testing.T) {
	r := escalationRig(t, "en")
	cards := r.moveOutside(flow.Merging, flow.InfraFailed)
	if len(cards) != 1 || len(cards[0].Buttons) != 0 || !strings.Contains(cards[0].Text, "revert pull request") {
		t.Fatalf("cards %+v", cards)
	}
}

// The sweep leaves tasks escalated before its window alone: an upgrade sends no old cards.
func TestTheSweepSkipsOldEscalations(t *testing.T) {
	r := escalationRig(t, "en")
	if _, err := r.c.Pool.Exec(t.Context(), `UPDATE book.tasks SET state = 'escalated', updated_at = now() - interval '2 days' WHERE id = $1`, r.task.ID); err != nil {
		t.Fatal(err)
	}
	if err := r.c.AskEscalated(t.Context(), r.tenant); err != nil {
		t.Fatal(err)
	}
	if n := len(r.decisionCards()); n != 0 {
		t.Fatalf("%d cards for an old escalation", n)
	}
}

// Review of #491: red CI on a task whose budget is spent escalates it; no Builder job starts
// after the move, and the card names the budget.
func TestASpentBudgetStopsBuilderAndAsks(t *testing.T) {
	r := escalationRig(t, "en")
	r.advance() // queues the Builder job
	r.finish(r.patchFor("feature.txt", "x\n"))
	r.advance() // checks
	if _, err := r.c.Pool.Exec(t.Context(), `INSERT INTO book.budgets (task_id, max_iterations, max_tokens, max_duration, used_iterations)
		VALUES ($1, 1, 1000, interval '1 hour', 1) ON CONFLICT (task_id) DO UPDATE SET max_iterations = 1, used_iterations = 1`, r.task.ID); err != nil {
		t.Fatal(err)
	}
	before := r.lastJob().ID
	r.twin.SetChecks("example/app", r.task.HeadSha.String, fake.Check{Name: "test", Status: "completed", Conclusion: "failure", Summary: "1 test failed"})
	r.advance()
	cards := r.decisionCards()
	if r.task.State != string(flow.Escalated) || r.lastJob().ID != before || len(cards) != 1 || !strings.Contains(cards[0].Text, "budget is used up") {
		t.Fatalf("task in %s, last job %d (was %d), cards %+v", r.task.State, r.lastJob().ID, before, cards)
	}
}
