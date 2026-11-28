package dispatch

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/ilyachakun/nightshift/internal/control/chatapi"
	"github.com/ilyachakun/nightshift/internal/control/flow"
	"github.com/ilyachakun/nightshift/internal/customization/config"
	"github.com/ilyachakun/nightshift/internal/data/book/bookdb"
	"github.com/ilyachakun/nightshift/internal/knowledge/search"
)

// escalationLabels are the fixed words of the "Decision needed" card in the chat language.
type escalationLabels struct {
	Stopped, Why, Cause, NoWhy, Withheld, Ask string
	Merged, AskMerged                         string
	Continue, Revisit, Cancel                 string
	Causes                                    map[flow.Event]string
}

func escalationLabelsFor(ru bool) escalationLabels {
	if ru {
		return escalationLabels{
			Stopped: "остановилась, нужно ваше решение", Why: "Причина", Cause: "Что случилось",
			NoWhy:     "фабрика не записала причину; подробности в `nightshift status --task %d`",
			Withheld:  "причина цитировала скрытый сценарий и скрыта; подробности в `nightshift status --task %d`",
			Ask:       "Продолжить заново, вернуть спеку на доработку или отменить?",
			Merged:    "изменение уже в main, релиз не прошёл smoke, а откатить не к чему",
			AskMerged: "Продакшен нужно починить вручную или откатить revert-пул-реквестом; кнопки здесь не помогут.",
			Continue:  "Продолжить", Revisit: "Доработать спеку", Cancel: "Отменить",
			Causes: map[flow.Event]string{
				flow.Escalate: "роль попросила решения человека или её ответ не читается", flow.JobFailed: "задание роли упало",
				flow.BudgetExhausted: "бюджет задачи исчерпан", flow.InfraFailed: "платформа превью или релиза не поднялась",
			},
		}
	}
	return escalationLabels{
		Stopped: "stopped and needs your decision", Why: "Why", Cause: "What happened",
		NoWhy:     "the factory recorded no reason; see `nightshift status --task %d`",
		Withheld:  "the reason quoted a hidden scenario and is withheld; see `nightshift status --task %d`",
		Ask:       "Continue with a fresh budget, send the spec back to Lead, or cancel?",
		Merged:    "the change is merged, its release failed the smoke and there is no earlier revision to roll back to",
		AskMerged: "Production needs a fix by hand or a revert pull request; no button here undoes a merge.",
		Continue:  "Continue", Revisit: "Revisit the spec", Cancel: "Cancel",
		Causes: map[flow.Event]string{
			flow.Escalate: "a role asked for a person or its result could not be read", flow.JobFailed: "a role's job failed",
			flow.BudgetExhausted: "the task budget is used up", flow.InfraFailed: "the preview or release platform failed",
		},
	}
}

// escalation is what the "Decision needed" card says: the event that moved the task, the state
// it left, the reason when there is one, and the acceptance job whose evidence it shows.
type escalation struct {
	Event flow.Event
	From  flow.State
	Why   string
	Job   *bookdb.BookJob
}

// escalateTask moves a working task to `escalated` with e and asks the owner to decide.
func (c *Conductor) escalateTask(ctx context.Context, t config.Tenant, task bookdb.BookTask, e flow.Event, why string) error {
	c.log().WarnContext(ctx, "task escalated", "task", task.ID, "event", e, "why", why)
	moved, err := c.apply(ctx, t, task, e, "")
	if err != nil {
		return err
	}
	return c.askDecision(ctx, bookdb.New(c.Pool), t, moved, escalation{Event: e, From: flow.State(task.State), Why: why})
}

// unaskedWindow bounds the sweep of escalated tasks without a card, so an upgrade does not
// send cards for tasks escalated long before the card existed.
const unaskedWindow = 24 * time.Hour

// AskEscalated queues the card of every task of the tenant that reached `escalated` without one:
// a move whose card was lost (the process stopped between the move and the card) or a move that
// escalated inside another package, such as the merge train sending a task back with its budget
// spent. The cause comes from the task's last move.
func (c *Conductor) AskEscalated(ctx context.Context, t config.Tenant) error {
	q := bookdb.New(c.Pool)
	tasks, err := q.ListUnaskedEscalations(ctx, bookdb.ListUnaskedEscalationsParams{
		Tenant: t.Name, Since: pgtype.Timestamptz{Time: time.Now().Add(-unaskedWindow), Valid: true},
	})
	if err != nil {
		return fmt.Errorf("conductor: unasked escalations: %w", err)
	}
	var errs []error
	for _, task := range tasks {
		events, err := q.ListTaskEvents(ctx, pgtype.Int8{Int64: task.ID, Valid: true})
		if err != nil {
			errs = append(errs, err)
			continue
		}
		var esc escalation
		for _, e := range events {
			var p struct {
				Event, From string
			}
			if e.Kind == "task.moved" && json.Unmarshal(e.Payload, &p) == nil {
				esc = escalation{Event: flow.Event(p.Event), From: flow.State(p.From)}
			}
		}
		errs = append(errs, c.askDecision(ctx, q, t, task, esc))
	}
	return errors.Join(errs...)
}

// askDecision queues the "Decision needed" card of a task that reached `escalated` (design
// section 7): what happened, why, the pull request, and for an acceptance job its preview and
// screenshots, with the three ways on (continue, revisit, cancel). A change already merged gets
// no buttons: going on or canceling would not undo it. It does nothing for a task in another
// state. The card is queued once per task version, with q's transaction when there is one. A
// reason that quotes a hidden scenario is withheld, as in a finding. A task with no chat thread
// and an owner without a chat get no card; the digest lists the task.
func (c *Conductor) askDecision(ctx context.Context, q *bookdb.Queries, t config.Tenant, task bookdb.BookTask, esc escalation) error {
	if flow.State(task.State) != flow.Escalated || c.Outbox == nil {
		return nil
	}
	ref, ok := ownerThread(t, task)
	if !ok {
		return nil
	}
	hidden, err := HiddenScenarios(ctx, &search.Index{Pool: c.Pool}, task.Tenant, task.ID)
	if err != nil {
		return err
	}
	ru := c.russian(ctx, task)
	lbl, ev := escalationLabelsFor(ru), evidenceLabelsFor(ru)
	cause, known := lbl.Causes[esc.Event]
	if !known && esc.Event != "" {
		cause = lbl.Causes[flow.BudgetExhausted] // a return event escalates when the budget is spent
	}
	merged := esc.From == flow.Merging && esc.Event == flow.InfraFailed
	if merged {
		cause = lbl.Merged
	}
	var b strings.Builder
	fmt.Fprintf(&b, "#%d %s: %s.", task.ID, chatapi.Title(task.Title), lbl.Stopped)
	if cause != "" {
		fmt.Fprintf(&b, "\n%s: %s.", lbl.Cause, cause)
	}
	switch {
	case esc.Why != "" && quotesAny(esc.Why, scenarioStrings(hidden)):
		fmt.Fprintf(&b, "\n%s: "+lbl.Withheld, lbl.Why, task.ID)
	case esc.Why != "":
		fmt.Fprintf(&b, "\n%s: %s", lbl.Why, esc.Why)
	case cause == "":
		fmt.Fprintf(&b, "\n%s: "+lbl.NoWhy, lbl.Why, task.ID)
	}
	if n := prNumber(task); n > 0 {
		fmt.Fprintf(&b, "\n%s: #%d", ev.PR, n)
	}
	if esc.Job != nil {
		files, err := evidenceFiles(ctx, q, *esc.Job)
		if err != nil {
			return err
		}
		b.WriteString(evidenceLines(*esc.Job, files, ev))
	}
	var buttons []chatapi.Button
	if merged {
		b.WriteString("\n" + lbl.AskMerged)
	} else {
		b.WriteString("\n" + lbl.Ask)
		buttons = []chatapi.Button{
			{Action: "continue", Label: lbl.Continue}, {Action: "revisit", Label: lbl.Revisit}, {Action: "cancel_task", Label: lbl.Cancel},
		}
	}
	return c.postCard(ctx, q, task, chatapi.CardDecisionNeeded, b.String(), buttons, ref, fmt.Sprintf("escalated:%d:%d", task.ID, task.Version))
}
