package dispatch

import (
	"context"
	"fmt"
	"strings"

	"github.com/ilyachakun/nightshift/internal/control/chatapi"
	"github.com/ilyachakun/nightshift/internal/control/flow"
	"github.com/ilyachakun/nightshift/internal/customization/config"
	"github.com/ilyachakun/nightshift/internal/data/book/bookdb"
	"github.com/ilyachakun/nightshift/internal/knowledge/search"
)

// escalationLabels are the fixed words of the "Decision needed" card in the chat language.
type escalationLabels struct {
	Stopped, Why, Cause, NoWhy, Withheld, Ask string
	Continue, Revisit, Cancel                 string
	Causes                                    map[flow.Event]string
}

func escalationLabelsFor(ru bool) escalationLabels {
	if ru {
		return escalationLabels{
			Stopped: "остановилась, нужно ваше решение", Why: "Причина", Cause: "Что случилось",
			NoWhy:    "фабрика не записала причину; подробности в `nightshift status --task`",
			Withheld: "причина цитировала скрытый сценарий и скрыта; подробности в `nightshift status --task`",
			Ask:      "Продолжить заново, вернуть спеку на доработку или отменить?",
			Continue: "Продолжить", Revisit: "Доработать спеку", Cancel: "Отменить",
			Causes: map[flow.Event]string{
				flow.Escalate: "роль попросила решения человека", flow.JobFailed: "задание роли упало",
				flow.BudgetExhausted: "бюджет задачи исчерпан", flow.InfraFailed: "платформа превью или релиза не поднялась",
			},
		}
	}
	return escalationLabels{
		Stopped: "stopped and needs your decision", Why: "Why", Cause: "What happened",
		NoWhy:    "the factory recorded no reason; see `nightshift status --task`",
		Withheld: "the reason quoted a hidden scenario and is withheld; see `nightshift status --task`",
		Ask:      "Continue with a fresh budget, send the spec back to Lead, or cancel?",
		Continue: "Continue", Revisit: "Revisit the spec", Cancel: "Cancel",
		Causes: map[flow.Event]string{
			flow.Escalate: "a role asked for a person", flow.JobFailed: "a role's job failed",
			flow.BudgetExhausted: "the task budget is used up", flow.InfraFailed: "the preview or release platform failed",
		},
	}
}

// escalateTask moves a working task to `escalated` with e and asks the owner to decide.
func (c *Conductor) escalateTask(ctx context.Context, t config.Tenant, task bookdb.BookTask, e flow.Event, why string) error {
	c.log().WarnContext(ctx, "task escalated", "task", task.ID, "event", e, "why", why)
	moved, err := c.apply(ctx, t, task, e, "")
	if err != nil {
		return err
	}
	return c.askDecision(ctx, bookdb.New(c.Pool), t, moved, e, why, nil)
}

// askDecision queues the "Decision needed" card of a task that reached `escalated` (design
// section 7): what happened, why, the pull request, and for an acceptance job its preview and
// screenshots, with the three ways on (continue, revisit, cancel). It does nothing for a task in
// another state. The card is queued once per task version, with q's transaction when there is
// one. A reason that quotes a hidden scenario is withheld, as in a finding. A task with no chat
// thread and an owner without a chat get no card; the digest lists the task.
func (c *Conductor) askDecision(ctx context.Context, q *bookdb.Queries, t config.Tenant, task bookdb.BookTask, e flow.Event, why string, job *bookdb.BookJob) error {
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
	switch {
	case why == "":
		why = lbl.NoWhy
	case quotesAny(why, scenarioStrings(hidden)):
		why = lbl.Withheld
	}
	var b strings.Builder
	fmt.Fprintf(&b, "#%d %s: %s.", task.ID, chatapi.Title(task.Title), lbl.Stopped)
	if cause, ok := lbl.Causes[e]; ok {
		fmt.Fprintf(&b, "\n%s: %s.", lbl.Cause, cause)
	}
	fmt.Fprintf(&b, "\n%s: %s", lbl.Why, why)
	if n := prNumber(task); n > 0 {
		fmt.Fprintf(&b, "\n%s: #%d", ev.PR, n)
	}
	if job != nil {
		files, err := evidenceFiles(ctx, q, *job)
		if err != nil {
			return err
		}
		b.WriteString(evidenceLines(*job, files, ev))
	}
	b.WriteString("\n" + lbl.Ask)
	buttons := []chatapi.Button{
		{Action: "continue", Label: lbl.Continue}, {Action: "revisit", Label: lbl.Revisit}, {Action: "cancel_task", Label: lbl.Cancel},
	}
	return c.postCard(ctx, q, task, chatapi.CardDecisionNeeded, b.String(), buttons, ref, fmt.Sprintf("escalated:%d:%d", task.ID, task.Version))
}
