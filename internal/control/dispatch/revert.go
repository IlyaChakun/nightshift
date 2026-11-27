package dispatch

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strconv"
	"strings"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/ilyachakun/nightshift/internal/control/codehost"
	"github.com/ilyachakun/nightshift/internal/control/flow"
	"github.com/ilyachakun/nightshift/internal/control/train"
	"github.com/ilyachakun/nightshift/internal/control/trustedgit"
	"github.com/ilyachakun/nightshift/internal/customization/config"
	"github.com/ilyachakun/nightshift/internal/data/book/bookdb"
	"github.com/ilyachakun/nightshift/internal/integration"
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
	bad, err := c.badCommit(ctx, q, task)
	if err != nil {
		return err
	}
	number, err := c.revertPR(ctx, q, task, bad)
	if err != nil {
		return err
	}
	if number == 0 {
		return c.openRevert(ctx, t, task, bad, true)
	}
	st, err := c.Forge.PRState(ctx, codehost.PRRef{Repo: task.Repo, Number: number})
	if err != nil {
		return fmt.Errorf("conductor: revert of task %d: %w", task.ID, err)
	}
	switch {
	case st.Merged:
		return c.revertMerged(ctx, t, q, task, bad)
	case st.State == "closed":
		// A person closed it, perhaps after reverting by hand: an empty revert sends the task on.
		return c.openRevert(ctx, t, task, bad, false)
	}
	ci, err := c.Forge.CI(ctx, task.Repo, st.Ref.HeadSHA)
	if err != nil {
		return fmt.Errorf("conductor: revert of task %d: %w", task.ID, err)
	}
	switch ci.State {
	case codehost.CISuccess:
		if _, err := c.Forge.Merge(ctx, st.Ref); err != nil {
			return fmt.Errorf("conductor: merge the revert of task %d: %w", task.ID, err)
		}
		return nil // the next tick sees the merge and sends the task back
	case codehost.CIFailure:
		c.tellRevertStuck(ctx, task, bad, "its pull request fails "+strings.Join(ci.Failing, ", "), "failing:"+st.Ref.HeadSHA)
	case codehost.CIPending:
		// the next tick looks again
	}
	return nil
}

// badCommit is the merge commit the rollback is about, from the train journal. It is empty only
// after a crash between clearing the journal and moving the task, when the revert has merged.
func (c *Conductor) badCommit(ctx context.Context, q *bookdb.Queries, task bookdb.BookTask) (string, error) {
	merged, ok, err := train.BookJournal{Q: q}.Peek(ctx, task.ID, train.StepMerge)
	if err != nil {
		return "", fmt.Errorf("conductor: revert of task %d: %w", task.ID, err)
	}
	if !ok || !merged.Done {
		return "", nil
	}
	return merged.SHA, nil
}

// revertPR returns the number of the revert pull request for the bad commit, or 0 before it is
// opened. A task rolled back twice has one event per commit; an older one is history. Without a
// bad commit on record the newest event stands.
func (c *Conductor) revertPR(ctx context.Context, q *bookdb.Queries, task bookdb.BookTask, bad string) (int, error) {
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
		num, sha, _ := strings.Cut(p.Note, " ")
		if bad != "" && sha != bad {
			continue
		}
		n, err := strconv.Atoi(num)
		if err != nil {
			return 0, fmt.Errorf("conductor: revert event of task %d: %w", task.ID, err)
		}
		return n, nil
	}
	return 0, nil
}

// openRevert commits the revert of the bad commit on the base branch and, when open is set,
// pushes it and opens the pull request. A base branch that no longer holds the change sends the
// task on; a conflict or a commit outside the base branch waits for a person. A crash before the
// event is written repeats the revert; the code host returns the pull request already open.
func (c *Conductor) openRevert(ctx context.Context, t config.Tenant, task bookdb.BookTask, bad string, open bool) error {
	if bad == "" {
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
	_, err = c.Git.Revert(ctx, mirror, revertBranch(task), base, bad, c.Identity)
	switch {
	case errors.Is(err, trustedgit.ErrNothingToRevert):
		return c.revertMerged(ctx, t, bookdb.New(c.Pool), task, bad)
	case errors.Is(err, trustedgit.ErrRevertConflict):
		c.tellRevertStuck(ctx, task, bad, "conflicts with later commits on "+c.base(), "conflict")
		return nil
	case errors.Is(err, trustedgit.ErrNotOnBase):
		c.tellRevertStuck(ctx, task, bad, "is not possible: the commit is not on "+c.base(), "not-on-base")
		return nil
	case err != nil:
		return fmt.Errorf("conductor: revert of task %d: %w", task.ID, err)
	case !open:
		c.tellRevertStuck(ctx, task, bad, "pull request was closed without a merge", "closed")
		return nil
	}
	if err := c.pushBranch(ctx, mirror, task.Repo, revertBranch(task)); err != nil {
		return err
	}
	ref, err := c.Forge.OpenPR(ctx, codehost.OpenPR{
		Repo: task.Repo, Head: revertBranch(task), Base: c.base(),
		Title: "Revert " + c.changeTitle(ctx, task),
		Body:  fmt.Sprintf("The release of %s failed its smoke and production was rolled back. This takes the change out of %s so the next release does not ship it again.", bad, c.base()),
	})
	if err != nil {
		return fmt.Errorf("conductor: open the revert of task %d: %w", task.ID, err)
	}
	return recordEvent(ctx, bookdb.New(c.Pool), task, eventRevertOpened, strconv.Itoa(ref.Number)+" "+bad)
}

// tellRevertStuck tells the owner once per cause that the revert needs a person and that the
// train of the repository holds until the change is out of the base branch. Reverting it by hand
// is enough: the next tick finds nothing left to revert.
func (c *Conductor) tellRevertStuck(ctx context.Context, task bookdb.BookTask, bad, why, cause string) {
	c.log().WarnContext(ctx, "the revert needs a person", "task", task.ID, "commit", bad, "why", why)
	ref, err := threadOf(task)
	if err != nil || c.Outbox == nil {
		return
	}
	text := fmt.Sprintf("Task #%d: production was rolled back, but the revert of %s %s. The merge train of %s holds until the change is out of %s; revert it by hand to go on.", task.ID, bad, why, task.Repo, c.base())
	if err := c.postCard(ctx, bookdb.New(c.Pool), task, integration.CardBlocker, text, nil, ref, fmt.Sprintf("revert-stuck:%d:%s:%s", task.ID, bad, cause)); err != nil {
		c.log().WarnContext(ctx, "card about a stuck revert not queued", "task", task.ID, "error", err)
	}
}

// revertMerged records the incident and sends the task back to Builder on a fresh pull request:
// the merged one is history, and a task that kept it would look merged again at once.
func (c *Conductor) revertMerged(ctx context.Context, t config.Tenant, q *bookdb.Queries, task bookdb.BookTask, bad string) error {
	fp := "incident:" + strconv.FormatInt(task.ID, 10) + ":" + bad
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
