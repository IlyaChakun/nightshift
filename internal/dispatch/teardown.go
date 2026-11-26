package dispatch

import (
	"context"
	"errors"
	"fmt"
	"strconv"

	"github.com/ilyachakun/nightshift/internal/book/bookdb"
	"github.com/ilyachakun/nightshift/internal/config"
	"github.com/ilyachakun/nightshift/internal/deploy"
	"github.com/ilyachakun/nightshift/internal/flow"
	"github.com/ilyachakun/nightshift/internal/integration"
)

// endedStates are the states in which a task needs no preview any more (design section 7,
// "Cancel", and section 17, "Deploy targets": a preview is torn down afterwards). An escalated
// task keeps its preview: the person it waits for may want to look at it.
var endedStates = []string{
	string(flow.Released), string(flow.RolledBack), string(flow.MergedByTeam),
	string(flow.Canceled), string(flow.Rejected),
}

// TeardownPreviews removes the preview of every task of the tenant that has ended. Down of an
// absent preview is fine, so a teardown that failed half way runs again on the next tick. When
// the tenant no longer declares a preview the factory cannot reach the environment: the journal
// is cleared so the sweep stops, and the log names the environment to remove by hand.
func (c *Conductor) TeardownPreviews(ctx context.Context, t config.Tenant) error {
	if c.Deploys == nil {
		return nil
	}
	q := bookdb.New(c.Pool)
	tasks, err := q.ListEndedTasksWithPreview(ctx, bookdb.ListEndedTasksWithPreviewParams{Tenant: t.Name, States: endedStates})
	if err != nil {
		return fmt.Errorf("conductor: previews to tear down: %w", err)
	}
	var errs []error
	for _, task := range tasks {
		if err := c.teardown(ctx, t, q, task); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

func (c *Conductor) teardown(ctx context.Context, t config.Tenant, q *bookdb.Queries, task bookdb.BookTask) error {
	job := deploy.Job{Tenant: task.Tenant, TaskID: task.ID, TaskKey: strconv.FormatInt(task.ID, 10), Rev: integration.Revision(task.HeadSha.String)}
	run, ok, err := c.Deploys(ctx, t, task)
	if err != nil {
		return fmt.Errorf("conductor: teardown of task %d: %w", task.ID, err)
	}
	if !ok {
		if c.Log != nil {
			c.Log.WarnContext(ctx, "preview capability is gone: remove the preview by hand", "tenant", task.Tenant, "task", task.ID, "state", task.State)
		}
		if err := q.ClearDeploySteps(ctx, task.ID); err != nil {
			return fmt.Errorf("conductor: teardown of task %d: %w", task.ID, err)
		}
		return nil
	}
	if err := run.Teardown(ctx, job); err != nil {
		return fmt.Errorf("conductor: teardown of task %d: %w", task.ID, err)
	}
	if c.Log != nil {
		c.Log.InfoContext(ctx, "preview torn down", "tenant", task.Tenant, "task", task.ID, "state", task.State)
	}
	return nil
}
