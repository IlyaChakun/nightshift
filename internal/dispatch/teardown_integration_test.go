package dispatch_test

import (
	"context"
	"errors"
	"strconv"
	"strings"
	"testing"
	"time"

	deployfake "github.com/ilyachakun/nightshift/internal/adapters/deploy/fake"
	"github.com/ilyachakun/nightshift/internal/book/bookdb"
	"github.com/ilyachakun/nightshift/internal/config"
	"github.com/ilyachakun/nightshift/internal/deploy"
	"github.com/ilyachakun/nightshift/internal/dispatch"
	"github.com/ilyachakun/nightshift/internal/integration"
	"github.com/ilyachakun/nightshift/internal/testutil/pgtest"
)

// previewSteps counts the task's preview rows in the journal.
func (r *rig) previewSteps() int {
	r.t.Helper()
	rows, err := r.q.ListEndedTasksWithPreview(r.t.Context(), bookdb.ListEndedTasksWithPreviewParams{
		Tenant: r.task.Tenant, States: []string{"released", "canceled", "human_review"},
	})
	if err != nil {
		r.t.Fatal(err)
	}
	return len(rows)
}

// T4: a released task's preview is torn down by the conductor's sweep, once.
func TestAReleasedTaskLosesItsPreview(t *testing.T) {
	r := newRig(t)
	r.withGate(config.Review{Mode: "all"}, "solo")
	d := r.withPreview()
	r.withRelease(d)
	r.toReady()
	r.advance() // ready -> merging -> released
	if r.task.State != "released" {
		t.Fatalf("the task is in %s, want released", r.task.State)
	}
	env := integration.EnvRef{Tenant: r.task.Tenant, Task: strconv.FormatInt(r.task.ID, 10)}
	if st, _ := d.Status(t.Context(), env); st.State != integration.EnvHealthy {
		t.Fatalf("before the sweep the preview is %+v, want healthy", st)
	}
	for range 2 { // the second sweep finds nothing to do
		if err := r.c.TeardownPreviews(t.Context(), r.tenant, time.Minute); err != nil {
			t.Fatal(err)
		}
	}
	if st, _ := d.Status(t.Context(), env); st.State != integration.EnvAbsent {
		t.Errorf("after the sweep the preview is %+v, want absent", st)
	}
	if n := r.previewSteps(); n != 0 {
		t.Errorf("%d preview rows left in the journal", n)
	}
	downs := 0
	for _, call := range d.Calls() {
		if strings.HasPrefix(call, "down") {
			downs++
		}
		if strings.HasPrefix(call, "rollback") {
			t.Errorf("the sweep touched production: %s", call)
		}
	}
	if downs != 1 {
		t.Errorf("down ran %d times, want once: %v", downs, d.Calls())
	}
}

// A preview of a task that waits for a person stays; a canceled task's preview goes, and a
// teardown that failed runs again on the next sweep.
func TestACanceledTaskLosesItsPreviewAfterARetry(t *testing.T) {
	r := newRig(t)
	r.withGate(config.Review{Mode: "all"}, "solo")
	d := r.withPreview()
	r.toReview("feature.txt")
	r.advance() // review -> preview -> human_review
	env := integration.EnvRef{Tenant: r.task.Tenant, Task: strconv.FormatInt(r.task.ID, 10)}
	if err := r.c.TeardownPreviews(t.Context(), r.tenant, time.Minute); err != nil {
		t.Fatal(err)
	}
	if st, _ := d.Status(t.Context(), env); st.State != integration.EnvHealthy {
		t.Fatalf("a waiting task's preview was touched: %+v", st)
	}
	doors := &dispatch.Doors{Pool: r.c.Pool, Mover: r.c.Mover}
	if err := doors.Cancel(t.Context(), r.tenant, r.task.ID, "cli"); err != nil {
		t.Fatal(err)
	}
	d.FailNext("down", errors.New("engine down"))
	if err := r.c.TeardownPreviews(t.Context(), r.tenant, time.Minute); err == nil {
		t.Fatal("the failed teardown was not reported")
	}
	if err := r.c.TeardownPreviews(t.Context(), r.tenant, time.Minute); err != nil {
		t.Fatal(err)
	}
	if st, _ := d.Status(t.Context(), env); st.State != integration.EnvAbsent {
		t.Errorf("the canceled task's preview is %+v, want absent", st)
	}
}

// A tenant that dropped its preview capability cannot reach the environment: the row stays, so
// the preview is not forgotten while it may still run, and the capability is read once a sweep.
func TestTheSweepKeepsThePreviewWhenTheCapabilityIsGone(t *testing.T) {
	r := newRig(t)
	r.withGate(config.Review{Mode: "all"}, "solo")
	r.withPreview()
	r.toReview("feature.txt")
	r.advance()
	doors := &dispatch.Doors{Pool: r.c.Pool, Mover: r.c.Mover}
	if err := doors.Cancel(t.Context(), r.tenant, r.task.ID, "cli"); err != nil {
		t.Fatal(err)
	}
	calls := 0
	r.c.Deploys = func(context.Context, config.Tenant, bookdb.BookTask) (*deploy.Runner, bool, error) {
		calls++
		return nil, false, nil
	}
	if err := r.c.TeardownPreviews(t.Context(), r.tenant, time.Minute); err != nil {
		t.Fatal(err)
	}
	if calls != 1 || r.previewSteps() != 1 {
		t.Errorf("capability read %d times, %d preview rows; want 1 and 1", calls, r.previewSteps())
	}
}

// hangingDeployer never finishes Down of one environment, as a hung engine call would.
type hangingDeployer struct {
	integration.Deployer
	hang string
}

func (h hangingDeployer) Down(ctx context.Context, e integration.EnvRef) error {
	if e.Task == h.hang {
		<-ctx.Done()
		return ctx.Err()
	}
	return h.Deployer.Down(ctx, e)
}

// One hung teardown uses up its own limit only: the next task's preview still goes.
func TestAHungTeardownDoesNotHoldTheNextTask(t *testing.T) {
	r := newRig(t)
	ctx := t.Context()
	d := deployfake.New()
	second, err := r.q.CreateTask(ctx, bookdb.CreateTaskParams{Tenant: "example-solo", Repo: "example/app", Title: "Another"})
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range []int64{r.task.ID, second.ID} {
		if _, err := r.c.Pool.Exec(ctx, `UPDATE book.tasks SET state = 'canceled' WHERE id = $1`, id); err != nil {
			t.Fatal(err)
		}
		if _, err := r.q.StartDeployStep(ctx, bookdb.StartDeployStepParams{Tenant: "example-solo", TaskID: id, Step: deploy.StepUp}); err != nil {
			t.Fatal(err)
		}
		if _, err := d.Up(ctx, integration.EnvRef{Tenant: "example-solo", Task: strconv.FormatInt(id, 10)}, ""); err != nil {
			t.Fatal(err)
		}
	}
	run := &deploy.Runner{Journal: deploy.BookJournal{Q: r.q}, Deployer: hangingDeployer{Deployer: d, hang: strconv.FormatInt(r.task.ID, 10)}}
	r.c.Deploys = func(context.Context, config.Tenant, bookdb.BookTask) (*deploy.Runner, bool, error) {
		return run, true, nil
	}
	if err := r.c.TeardownPreviews(ctx, r.tenant, 50*time.Millisecond); err == nil {
		t.Fatal("the hung teardown was not reported")
	}
	if st, _ := d.Status(ctx, integration.EnvRef{Tenant: "example-solo", Task: strconv.FormatInt(second.ID, 10)}); st.State != integration.EnvAbsent {
		t.Errorf("the second task's preview is %+v, want absent", st)
	}
}

// Reading the project file fetches the repository; a fetch that stalls ends at the limit.
func TestAStalledProjectReadEndsAtTheLimit(t *testing.T) {
	r := newRig(t)
	ctx := t.Context()
	if _, err := r.c.Pool.Exec(ctx, `UPDATE book.tasks SET state = 'canceled' WHERE id = $1`, r.task.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := r.q.StartDeployStep(ctx, bookdb.StartDeployStepParams{Tenant: "example-solo", TaskID: r.task.ID, Step: deploy.StepUp}); err != nil {
		t.Fatal(err)
	}
	r.c.Deploys = func(ctx context.Context, _ config.Tenant, _ bookdb.BookTask) (*deploy.Runner, bool, error) {
		<-ctx.Done()
		return nil, false, ctx.Err()
	}
	if err := r.c.TeardownPreviews(ctx, r.tenant, 50*time.Millisecond); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("err = %v, want the deadline", err)
	}
}

// capacity.previews holds a second preview until the first task ends (design section 7, "Capacity").
func TestThePreviewLimitHoldsTheNextPreview(t *testing.T) {
	db := pgtest.Start(t)
	a, b := newRigOnBook(t, db), newRigOnBook(t, db)
	var twinB *deployfake.Deployer
	for i, r := range []*rig{a, b} {
		r.withGate(config.Review{Mode: "all"}, "solo")
		d := r.withPreview()
		if i == 1 {
			twinB = d
		}
		r.tenant.Capacity.Previews = 1
		r.toReview("feature.txt")
	}
	a.advance()
	if a.task.State != "human_review" {
		t.Fatalf("the first task is in %s, want human_review", a.task.State)
	}
	b.advance()
	envB := integration.EnvRef{Tenant: b.task.Tenant, Task: strconv.FormatInt(b.task.ID, 10)}
	if st, _ := twinB.Status(t.Context(), envB); b.task.State != "preview" || st.State != integration.EnvAbsent {
		t.Fatalf("the second task is in %s with preview %+v; want it to wait in preview", b.task.State, st)
	}
	a.advance() // the first task keeps its own preview while it waits for a person
	if a.task.State != "human_review" {
		t.Fatalf("the first task moved to %s", a.task.State)
	}
	doors := &dispatch.Doors{Pool: a.c.Pool, Mover: a.c.Mover}
	if err := doors.Cancel(t.Context(), a.tenant, a.task.ID, "cli"); err != nil {
		t.Fatal(err)
	}
	b.advance()
	if b.task.State != "human_review" {
		t.Fatalf("after the first task ended the second is in %s, want human_review", b.task.State)
	}
}
