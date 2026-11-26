package dispatch

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"strconv"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/ilyachakun/nightshift/internal/book/bookdb"
	"github.com/ilyachakun/nightshift/internal/config"
	"github.com/ilyachakun/nightshift/internal/flow"
	"github.com/ilyachakun/nightshift/internal/integration"
	"github.com/ilyachakun/nightshift/internal/train"
)

// eventRevertOpened records the revert pull request of a rolled-back task; its note is the number.
const eventRevertOpened = "revert.opened"

func revertBranch(task bookdb.BookTask) string { return branchOf(task) + "-revert" }

// rolledBack takes the bad commit out of the base branch (design section 7, "Rollback"): it opens
// a revert pull request, merges it out of turn once its checks are green, then sends the task
// back to Builder with an incident finding. While the task is here the train of its repository is
// frozen, so the next release cannot ship the bad commit again. Each step repeats safely: the
// revert pull request is found again by its branch, and the finding by its fingerprint.
func (c *Conductor) rolledBack(ctx context.Context, t config.Tenant, task bookdb.BookTask) error {
	q := bookdb.New(c.Pool)
	number, err := c.revertPR(ctx, q, task)
	if err != nil {
		return err
	}
	if number == 0 {
		return c.openRevert(ctx, task)
	}
	st, err := c.Forge.PRState(ctx, integration.PRRef{Repo: task.Repo, Number: number})
	if err != nil {
		return fmt.Errorf("conductor: revert of task %d: %w", task.ID, err)
	}
	switch {
	case st.Merged:
		return c.revertMerged(ctx, t, q, task, number)
	case st.State == "closed":
		c.log().WarnContext(ctx, "the revert pull request was closed without a merge: a person decides", "task", task.ID, "pr", number)
		return nil
	}
	ci, err := c.Forge.CI(ctx, task.Repo, st.Ref.HeadSHA)
	if err != nil {
		return fmt.Errorf("conductor: revert of task %d: %w", task.ID, err)
	}
	switch ci.State {
	case integration.CISuccess:
		if _, err := c.Forge.Merge(ctx, st.Ref); err != nil {
			return fmt.Errorf("conductor: merge the revert of task %d: %w", task.ID, err)
		}
		return nil // the next tick sees the merge and sends the task back
	case integration.CIFailure:
		c.log().WarnContext(ctx, "the revert pull request fails its checks: a person decides", "task", task.ID, "pr", number, "failing", ci.Failing)
	case integration.CIPending:
		// the next tick looks again
	}
	return nil
}

// revertPR returns the number of the task's revert pull request, or 0 before it is opened.
func (c *Conductor) revertPR(ctx context.Context, q *bookdb.Queries, task bookdb.BookTask) (int, error) {
	events, err := q.ListTaskEvents(ctx, pgtype.Int8{Int64: task.ID, Valid: true})
	if err != nil {
		return 0, fmt.Errorf("conductor: %w", err)
	}
	for _, ev := range slices.Backward(events) {
		if ev.Kind != eventRevertOpened {
			continue
		}
		var p struct{ Note string }
		if err := json.Unmarshal(ev.Payload, &p); err != nil {
			return 0, fmt.Errorf("conductor: revert event of task %d: %w", task.ID, err)
		}
		n, err := strconv.Atoi(p.Note)
		if err != nil {
			return 0, fmt.Errorf("conductor: revert event of task %d: %w", task.ID, err)
		}
		return n, nil
	}
	return 0, nil
}

// openRevert commits the revert of the task's merge commit on the base branch, pushes it and
// opens the pull request. A crash before the event is written repeats the revert; the code host
// returns the pull request already open for the branch.
func (c *Conductor) openRevert(ctx context.Context, task bookdb.BookTask) error {
	merged, ok, err := train.BookJournal{Q: bookdb.New(c.Pool)}.Peek(ctx, task.ID, train.StepMerge)
	if err != nil {
		return fmt.Errorf("conductor: revert of task %d: %w", task.ID, err)
	}
	if !ok || !merged.Done || merged.SHA == "" {
		return fmt.Errorf("conductor: revert of task %d: no merge commit on record", task.ID)
	}
	mirror, err := c.mirror(ctx, task.Repo)
	if err != nil {
		return err
	}
	base, err := c.Git.Rev(ctx, mirror, c.base())
	if err != nil {
		return fmt.Errorf("conductor: %w", err)
	}
	if _, err := c.Git.Revert(ctx, mirror, revertBranch(task), base, merged.SHA, c.Identity); err != nil {
		return fmt.Errorf("conductor: revert of task %d: %w", task.ID, err)
	}
	if err := c.pushBranch(ctx, mirror, task.Repo, revertBranch(task)); err != nil {
		return err
	}
	ref, err := c.Forge.OpenPR(ctx, integration.OpenPR{
		Repo: task.Repo, Head: revertBranch(task), Base: c.base(),
		Title: "Revert " + c.changeTitle(ctx, task),
		Body:  fmt.Sprintf("The release of %s failed its smoke and production was rolled back. This takes the change out of %s so the next release does not ship it again.", merged.SHA, c.base()),
	})
	if err != nil {
		return fmt.Errorf("conductor: open the revert of task %d: %w", task.ID, err)
	}
	return recordEvent(ctx, bookdb.New(c.Pool), task, eventRevertOpened, strconv.Itoa(ref.Number))
}

// revertMerged records the incident and sends the task back to Builder on a fresh pull request:
// the merged one is history, and a task that kept it would look merged again at once.
func (c *Conductor) revertMerged(ctx context.Context, t config.Tenant, q *bookdb.Queries, task bookdb.BookTask, number int) error {
	fp := "incident:" + strconv.FormatInt(task.ID, 10) + ":" + strconv.Itoa(number)
	have, err := q.ListTaskFindings(ctx, task.ID)
	if err != nil {
		return fmt.Errorf("conductor: %w", err)
	}
	if !slices.ContainsFunc(have, func(f bookdb.BookFindingState) bool { return f.Fingerprint == fp }) {
		if _, err := q.AddFinding(ctx, bookdb.AddFindingParams{
			Tenant: task.Tenant, TaskID: task.ID, RuleID: "release.rolled_back", Category: "incident", Severity: "block",
			LayerCaught: "L6", Message: "The released change failed its production smoke and was reverted; fix it before it ships again.",
			Fingerprint: fp,
		}); err != nil {
			return fmt.Errorf("conductor: %w", err)
		}
	}
	if err := q.SetTaskPR(ctx, bookdb.SetTaskPRParams{ID: task.ID}); err != nil {
		return fmt.Errorf("conductor: %w", err)
	}
	if err := (train.BookJournal{Q: q}).Clear(ctx, task.ID); err != nil {
		return fmt.Errorf("conductor: %w", err)
	}
	_, err = c.move(ctx, t, task, flow.RevertMerged, "")
	return err
}

// trainFrozen reports whether a task of the repository waits for its revert: the train holds.
func (c *Conductor) trainFrozen(ctx context.Context, task bookdb.BookTask) (bool, error) {
	held, err := bookdb.New(c.Pool).ListTasksByState(ctx, bookdb.ListTasksByStateParams{Tenant: task.Tenant, State: string(flow.RolledBack)})
	if err != nil {
		return false, fmt.Errorf("conductor: %w", err)
	}
	return slices.ContainsFunc(held, func(o bookdb.BookTask) bool { return o.Repo == task.Repo && o.ID != task.ID }), nil
}
