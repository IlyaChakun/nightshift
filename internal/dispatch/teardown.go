package dispatch

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"time"

	"github.com/ilyachakun/nightshift/internal/book/bookdb"
	"github.com/ilyachakun/nightshift/internal/config"
	"github.com/ilyachakun/nightshift/internal/deploy"
	"github.com/ilyachakun/nightshift/internal/flow"
	"github.com/ilyachakun/nightshift/internal/integration"
)

// endedStates are the states in which a task needs no preview any more (design section 7,
// "Cancel", and section 17, "Deploy targets": a preview is torn down afterwards). A task in
// rolled_back may go back to building; its next preview starts from nothing, because a teardown
// clears the preview steps. An escalated task keeps its preview: the person it waits for may
// want to look at it.
var endedStates = []string{
	string(flow.Released), string(flow.RolledBack), string(flow.MergedByTeam),
	string(flow.Canceled), string(flow.Rejected),
}

// TeardownPreviews removes the preview of every task of the tenant that has ended. Each task has
// its own time limit, so one hung teardown does not hold the others. Down of an absent preview
// is fine, so a teardown that failed runs again on the next sweep. When the tenant no longer
// declares a preview, the factory cannot reach the environment: the journal keeps its row and
// the log names the environment to remove by hand, on every sweep, until someone does.
func (c *Conductor) TeardownPreviews(ctx context.Context, t config.Tenant, perTask time.Duration) error {
	if c.Deploys == nil {
		return nil
	}
	q := bookdb.New(c.Pool)
	tasks, err := q.ListEndedTasksWithPreview(ctx, bookdb.ListEndedTasksWithPreviewParams{Tenant: t.Name, States: endedStates})
	if err != nil {
		return fmt.Errorf("conductor: previews to tear down: %w", err)
	}
	// The runner reads the project file from a fresh mirror: once per repository per sweep.
	type resolved struct {
		run *deploy.Runner
		ok  bool
		err error
	}
	runners := map[string]resolved{}
	var errs []error
	for _, task := range tasks {
		r, seen := runners[task.Repo]
		if !seen {
			r.run, r.ok, r.err = c.Deploys(ctx, t, task)
			runners[task.Repo] = r
		}
		if err := c.teardown(ctx, task, r.run, r.ok, r.err, perTask); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
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
	if err := run.Teardown(taskCtx, deploy.Job{Tenant: task.Tenant, TaskID: task.ID, TaskKey: key, Rev: integration.Revision(task.HeadSha.String)}); err != nil {
		return fmt.Errorf("conductor: teardown of task %d: %w", task.ID, err)
	}
	if c.Log != nil {
		c.Log.InfoContext(ctx, "preview torn down", "tenant", task.Tenant, "task", task.ID, "state", task.State)
	}
	return nil
}
