package dispatch

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/ilyachakun/nightshift/internal/control/flow"
	"github.com/ilyachakun/nightshift/internal/customization/config"
	"github.com/ilyachakun/nightshift/internal/data/book/bookdb"
)

// needsResearchPrefix starts the blocked_reason of a Builder that stopped for a choice the spec
// does not cover (roles/builder.md).
const needsResearchPrefix = "needs_research:"

// maxResearchReturns is how often one task may go back to Lead; the next stop escalates to the
// owner, so Builder and Lead cannot loop.
const maxResearchReturns = 2

// maxAskRunes bounds the reason that reaches Lead's prompt.
const maxAskRunes = 500

// backToLead sends a task whose Builder stopped with needs_research back to Lead, with the
// reason in the thread. It reports false when that is not possible (the Lead stage is off, the
// reason is another one, or the task already came back twice) and the caller escalates.
func (c *Conductor) backToLead(ctx context.Context, t config.Tenant, task bookdb.BookTask, job bookdb.BookJob, reason string) (bool, error) {
	if _, ok := c.Roles["lead"]; !ok || len(c.Router.Chains["lead"]) == 0 || c.Outbox == nil {
		return false, nil
	}
	ask, ok := strings.CutPrefix(strings.TrimSpace(reason), needsResearchPrefix)
	if !ok {
		return false, nil
	}
	q := bookdb.New(c.Pool)
	events, err := q.ListTaskEvents(ctx, pgtype.Int8{Int64: task.ID, Valid: true})
	if err != nil {
		return false, fmt.Errorf("conductor: %w", err)
	}
	n, hasOwner := 0, false
	for _, e := range events {
		switch e.Kind {
		case eventResearchRequested:
			if !ofJob(e, job.ID) { // a repeated consumption of this job is not another return
				n++
			}
		case eventSpecExpanded:
			return false, nil // a task of a multi-task spec: a new spec here would clash with its siblings
		case "task_created", "owner_message":
			hasOwner = true
		}
	}
	// A sibling made from a spec has no owner words of its own for Lead to answer.
	if n >= maxResearchReturns || !hasOwner {
		return false, nil
	}
	// One line, so Builder's words cannot pose as another turn of the thread.
	ask = strings.Join(strings.Fields(ask), " ")
	if r := []rune(ask); len(r) > maxAskRunes {
		ask = string(r[:maxAskRunes])
	}
	payload, _ := json.Marshal(map[string]string{"text": "Builder stopped: " + ask}) //nolint:errchkjson // a string map always encodes
	if err := recordJobEvent(ctx, q, task, job.ID, eventResearchRequested, "builder", payload); err != nil {
		return false, err
	}
	if _, err := q.ConsumeJob(ctx, job.ID); err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return false, fmt.Errorf("conductor: %w", err)
	}
	_, err = c.move(ctx, t, task, flow.ResearchNeeded, "")
	return err == nil, err
}
