package dispatch

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/ilyachakun/nightshift/internal/control/deploy"
	"github.com/ilyachakun/nightshift/internal/control/flow"
	"github.com/ilyachakun/nightshift/internal/customization/config"
	"github.com/ilyachakun/nightshift/internal/data/book/bookdb"
	"github.com/ilyachakun/nightshift/internal/execution/deployapi"
)

// stalePreviewStates are the states in which a task needs no preview (design section 7, "Cancel"
// and "Capacity", and section 17, "Deploy targets"): it has ended, or it went back to work and
// its next preview is of a new head, or it waits for a decision with no review to look at. A task
// in rolled_back may go back to building; its next preview starts from nothing, because a
// teardown clears the preview steps. The states from preview to acceptance keep their preview;
// the waiting states after acceptance keep it for preview_hours (previewKeptStates).
var stalePreviewStates = []string{
	string(flow.Released), string(flow.RolledBack), string(flow.MergedByTeam),
	string(flow.Canceled), string(flow.Rejected), string(flow.Escalated), string(flow.Blocked),
	string(flow.Queued), string(flow.Building), string(flow.Checks), string(flow.Review),
}

// previewKeptStates are the states after acceptance in which a task takes no slot (design section 7,
// "Capacity"): its preview stays for a person to look at, up to the tenant's preview_hours.
var previewKeptStates = []string{
	string(flow.HumanReview), string(flow.HandedOff), string(flow.Ready), string(flow.Merging),
}

// DefaultPreviewHours is how long a waiting task keeps its preview when the tenant sets no
// capacity.preview_hours: a working day for the owner to look at it.
const DefaultPreviewHours = 24

// maxPreviewHours is the largest preview_hours, a year, as in schemas/tenant.json.
const maxPreviewHours = 8760

// TeardownPreviews removes the preview of every task of the tenant that needs none any more, and of
// every task that has waited longer than preview_hours since acceptance. Each task has
// its own time limit, so one hung teardown does not hold the others. Down of an absent preview
// is fine, so a teardown that failed runs again on the next sweep. When the tenant no longer
// declares a preview, the factory cannot reach the environment: the journal keeps its row and
// the log names the environment to remove by hand, on every sweep, until someone does.
func (c *Conductor) TeardownPreviews(ctx context.Context, t config.Tenant, perTask time.Duration) error {
	if c.Deploys == nil {
		return nil
	}
	q := bookdb.New(c.Pool)
	tasks, err := q.ListTasksWithStalePreview(ctx, bookdb.ListTasksWithStalePreviewParams{Tenant: t.Name, States: stalePreviewStates})
	if err != nil {
		return fmt.Errorf("conductor: previews to tear down: %w", err)
	}
	hours := DefaultPreviewHours
	if h := t.Capacity.PreviewHours; h != nil {
		hours = min(*h, maxPreviewHours) // the schema caps it too; a bigger value would overflow the duration
	}
	cutoff := c.now().Add(-time.Duration(hours) * time.Hour)
	idle, err := q.ListTasksWithIdlePreview(ctx, bookdb.ListTasksWithIdlePreviewParams{
		Tenant: t.Name, States: previewKeptStates, AcceptedBefore: pgtype.Timestamptz{Time: cutoff, Valid: true},
	})
	if err != nil {
		return fmt.Errorf("conductor: idle previews: %w", err)
	}
	tasks = append(tasks, idle...)
	// The runner reads the project file from a fresh mirror: once per repository per sweep.
	runners := map[string]resolved{}
	var errs []error
	for _, task := range tasks {
		r, seen := runners[task.Repo]
		if !seen {
			r = c.resolve(ctx, t, task, perTask)
			runners[task.Repo] = r
		}
		if err := c.teardown(ctx, task, r.run, r.ok, r.err, perTask); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

type resolved struct {
	run *deploy.Runner
	ok  bool
	err error
}

// resolve reads the tenant's deploy runner under the per-task limit: it fetches the repository,
// and a remote that stalls must not hold the tenant's loop.
func (c *Conductor) resolve(ctx context.Context, t config.Tenant, task bookdb.BookTask, limit time.Duration) resolved {
	ctx, cancel := context.WithTimeout(ctx, limit)
	defer cancel()
	var r resolved
	r.run, r.ok, r.err = c.Deploys(ctx, t, task)
	return r
}

func (c *Conductor) teardown(ctx context.Context, task bookdb.BookTask, run *deploy.Runner, ok bool, err error, limit time.Duration) error {
	if err != nil {
		return fmt.Errorf("conductor: teardown of task %d: %w", task.ID, err)
	}
	key := strconv.FormatInt(task.ID, 10)
	if !ok {
		if c.Log != nil {
			c.Log.WarnContext(ctx, "the preview capability is gone: remove the preview of this task by hand", "tenant", task.Tenant, "task", task.ID, "state", task.State)
		}
		return nil
	}
	taskCtx, cancel := context.WithTimeout(ctx, limit)
	defer cancel()
	if err := run.Teardown(taskCtx, deploy.Job{Tenant: task.Tenant, TaskID: task.ID, TaskKey: key, Rev: deployapi.Revision(task.HeadSha.String)}); err != nil {
		return fmt.Errorf("conductor: teardown of task %d: %w", task.ID, err)
	}
	if c.Log != nil {
		c.Log.InfoContext(ctx, "preview torn down", "tenant", task.Tenant, "task", task.ID, "state", task.State)
	}
	return nil
}

// waitsForPreview reports whether a task in preview must wait for a free preview slot.
func (c *Conductor) waitsForPreview(ctx context.Context, t config.Tenant, task bookdb.BookTask) (bool, error) {
	if flow.State(task.State) != flow.Preview {
		return false, nil
	}
	free, err := c.previewSlot(ctx, t, task)
	return !free, err
}

// previewSlot reports whether the tenant may start the task's preview under capacity.previews
// (design section 7, "Capacity"). A task that already has a preview keeps it. Unset means no
// limit. A limit of 0 means no previews at all; previewStep skips them, so no slot is needed. A preview
// counts until the teardown sweep removes it, so a slot frees within one sweep after a task ends.
func (c *Conductor) previewSlot(ctx context.Context, t config.Tenant, task bookdb.BookTask) (bool, error) {
	limit := t.Capacity.Previews
	if limit == nil || *limit <= 0 || c.Deploys == nil {
		return true, nil
	}
	row, err := bookdb.New(c.Pool).CountLivePreviews(ctx, bookdb.CountLivePreviewsParams{Tenant: t.Name, TaskID: task.ID})
	if err != nil {
		return false, fmt.Errorf("conductor: count previews of %s: %w", t.Name, err)
	}
	if row.Own || row.Others < int64(*limit) {
		return true, nil
	}
	if c.Log != nil {
		c.Log.DebugContext(ctx, "preview waits for a free slot", "tenant", t.Name, "task", task.ID, "previews", row.Others, "limit", *limit)
	}
	return false, nil
}
