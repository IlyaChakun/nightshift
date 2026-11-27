package dispatch_test

import (
	"context"
	"errors"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	deployfake "github.com/ilyachakun/nightshift/internal/adapters/deploy/fake"
	"github.com/ilyachakun/nightshift/internal/control/deploy"
	"github.com/ilyachakun/nightshift/internal/control/dispatch"
	"github.com/ilyachakun/nightshift/internal/control/doors"
	"github.com/ilyachakun/nightshift/internal/customization/config"
	"github.com/ilyachakun/nightshift/internal/data/book/bookdb"
	"github.com/ilyachakun/nightshift/internal/execution/deployapi"
	"github.com/ilyachakun/nightshift/internal/testing/testutil/pgtest"
)

// previewSteps counts the task's preview rows in the journal.
func (r *rig) previewSteps() int {
	r.t.Helper()
	rows, err := r.q.ListTasksWithStalePreview(r.t.Context(), bookdb.ListTasksWithStalePreviewParams{
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
	env := deployapi.EnvRef{Tenant: r.task.Tenant, Task: strconv.FormatInt(r.task.ID, 10)}
	if st, _ := d.Status(t.Context(), env); st.State != deployapi.EnvHealthy {
		t.Fatalf("before the sweep the preview is %+v, want healthy", st)
	}
	for range 2 { // the second sweep finds nothing to do
		if err := r.c.TeardownPreviews(t.Context(), r.tenant, time.Minute); err != nil {
			t.Fatal(err)
		}
	}
	if st, _ := d.Status(t.Context(), env); st.State != deployapi.EnvAbsent {
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
	env := deployapi.EnvRef{Tenant: r.task.Tenant, Task: strconv.FormatInt(r.task.ID, 10)}
	if err := r.c.TeardownPreviews(t.Context(), r.tenant, time.Minute); err != nil {
		t.Fatal(err)
	}
	if st, _ := d.Status(t.Context(), env); st.State != deployapi.EnvHealthy {
		t.Fatalf("a waiting task's preview was touched: %+v", st)
	}
	dr := &doors.Doors{Pool: r.c.Pool, Mover: r.c.Mover}
	if err := dr.Cancel(t.Context(), r.tenant, r.task.ID, "cli"); err != nil {
		t.Fatal(err)
	}
	d.FailNext("down", errors.New("engine down"))
	if err := r.c.TeardownPreviews(t.Context(), r.tenant, time.Minute); err == nil {
		t.Fatal("the failed teardown was not reported")
	}
	if err := r.c.TeardownPreviews(t.Context(), r.tenant, time.Minute); err != nil {
		t.Fatal(err)
	}
	if st, _ := d.Status(t.Context(), env); st.State != deployapi.EnvAbsent {
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
	dr := &doors.Doors{Pool: r.c.Pool, Mover: r.c.Mover}
	if err := dr.Cancel(t.Context(), r.tenant, r.task.ID, "cli"); err != nil {
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
	deployapi.Deployer
	hang string
}

func (h hangingDeployer) Down(ctx context.Context, e deployapi.EnvRef) error {
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
		if _, err := d.Up(ctx, deployapi.EnvRef{Tenant: "example-solo", Task: strconv.FormatInt(id, 10)}, ""); err != nil {
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
	if st, _ := d.Status(ctx, deployapi.EnvRef{Tenant: "example-solo", Task: strconv.FormatInt(second.ID, 10)}); st.State != deployapi.EnvAbsent {
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

// capacity.previews holds a second preview until the first is gone (design section 7, "Capacity").
// The waiting task decides nothing else meanwhile: no review policy run, no event per tick.
func TestThePreviewLimitHoldsTheNextPreview(t *testing.T) {
	db := pgtest.Start(t)
	a, b := newRigOnBook(t, db), newRigOnBook(t, db)
	twins := make([]*deployfake.Deployer, 2)
	for i, r := range []*rig{a, b} {
		r.withGate(config.Review{Mode: "all"}, "solo")
		twins[i] = r.withPreview()
		r.tenant.Capacity.Previews = new(1)
		r.toReview("feature.txt")
	}
	a.advance()
	if a.task.State != "human_review" {
		t.Fatalf("the first task is in %s, want human_review", a.task.State)
	}
	envA := deployapi.EnvRef{Tenant: a.task.Tenant, Task: strconv.FormatInt(a.task.ID, 10)}
	envB := deployapi.EnvRef{Tenant: b.task.Tenant, Task: strconv.FormatInt(b.task.ID, 10)}
	b.advance()
	events := func() int {
		evs, err := b.q.ListTaskEvents(t.Context(), taskID(b.task.ID))
		if err != nil {
			t.Fatal(err)
		}
		return len(evs)
	}
	before := events()
	b.advance()
	st, err := twins[1].Status(t.Context(), envB)
	if err != nil || b.task.State != "preview" || st.State != deployapi.EnvAbsent {
		t.Fatalf("the second task is in %s with preview %+v (%v); want it to wait in preview", b.task.State, st, err)
	}
	if after := events(); after != before {
		t.Errorf("a waiting tick wrote %d events", after-before)
	}
	dr := &doors.Doors{Pool: a.c.Pool, Mover: a.c.Mover}
	if err := dr.Cancel(t.Context(), a.tenant, a.task.ID, "cli"); err != nil {
		t.Fatal(err)
	}
	b.advance() // the first preview still runs until the sweep removes it
	if b.task.State != "preview" {
		t.Fatalf("the second task started while the first preview still ran: %s", b.task.State)
	}
	if err := a.c.TeardownPreviews(t.Context(), a.tenant, time.Minute); err != nil {
		t.Fatal(err)
	}
	if st, err := twins[0].Status(t.Context(), envA); err != nil || st.State != deployapi.EnvAbsent {
		t.Fatalf("the first preview is %+v (%v), want absent", st, err)
	}
	b.advance()
	if b.task.State != "human_review" {
		t.Fatalf("after the first preview went the second task is in %s, want human_review", b.task.State)
	}
}

// capacity.previews of 0: the tenant gets no preview, and the task goes on.
func TestAZeroPreviewLimitSkipsThePreview(t *testing.T) {
	r := newRig(t)
	r.withGate(config.Review{Mode: "all"}, "solo")
	d := r.withPreview()
	r.tenant.Capacity.Previews = new(0)
	r.toReview("feature.txt")
	r.advance()
	if r.task.State != "human_review" {
		t.Fatalf("the task is in %s, want human_review", r.task.State)
	}
	for _, call := range d.Calls() {
		if strings.HasPrefix(call, "up") {
			t.Errorf("a preview was started: %v", d.Calls())
		}
	}
}

// A task sent back to work no longer holds its old preview: the sweep removes it.
func TestATaskBackAtWorkLosesItsOldPreview(t *testing.T) {
	r := newRig(t)
	r.withGate(config.Review{Mode: "all"}, "solo")
	d := r.withPreview()
	r.toReview("feature.txt")
	r.advance() // human_review, with a preview
	if _, err := r.c.Pool.Exec(t.Context(), `UPDATE book.tasks SET state = 'building' WHERE id = $1`, r.task.ID); err != nil {
		t.Fatal(err)
	}
	if err := r.c.TeardownPreviews(t.Context(), r.tenant, time.Minute); err != nil {
		t.Fatal(err)
	}
	env := deployapi.EnvRef{Tenant: r.task.Tenant, Task: strconv.FormatInt(r.task.ID, 10)}
	if st, err := d.Status(t.Context(), env); err != nil || st.State != deployapi.EnvAbsent {
		t.Errorf("the old preview is %+v (%v), want absent", st, err)
	}
}

// T4: a task that waits for a person keeps its preview up to preview_hours after acceptance;
// then the sweep tears it down, and the default applies when the tenant sets none.
func TestAWaitingTaskLosesItsPreviewHoursAfterAcceptance(t *testing.T) {
	r := newRig(t)
	r.withGate(config.Review{Mode: "all"}, "solo")
	d := r.withPreview()
	r.toReview("feature.txt")
	r.advance() // review -> preview -> acceptance -> human_review
	if r.task.State != "human_review" {
		t.Fatalf("the task is in %s, want human_review", r.task.State)
	}
	env := deployapi.EnvRef{Tenant: r.task.Tenant, Task: strconv.FormatInt(r.task.ID, 10)}
	sweepAt := func(after time.Duration) deployapi.EnvState {
		t.Helper()
		at := time.Now().Add(after)
		r.c.Now = func() time.Time { return at }
		if err := r.c.TeardownPreviews(t.Context(), r.tenant, time.Minute); err != nil {
			t.Fatal(err)
		}
		st, _ := d.Status(t.Context(), env)
		return st.State
	}
	hours := 48
	r.tenant.Capacity.PreviewHours = &hours
	if st := sweepAt(25 * time.Hour); st != deployapi.EnvHealthy {
		t.Fatalf("25 hours after acceptance with preview_hours 48 the preview is %s, want healthy", st)
	}
	r.tenant.Capacity.PreviewHours = nil
	if st := sweepAt(time.Duration(dispatch.DefaultPreviewHours-1) * time.Hour); st != deployapi.EnvHealthy {
		t.Fatalf("before the default limit the preview is %s, want healthy", st)
	}
	if st := sweepAt(time.Duration(dispatch.DefaultPreviewHours+1) * time.Hour); st != deployapi.EnvAbsent {
		t.Fatalf("after the default limit the preview is %s, want absent", st)
	}
	got, err := r.q.ListTasksWithIdlePreview(t.Context(), bookdb.ListTasksWithIdlePreviewParams{
		Tenant: r.task.Tenant, States: []string{"human_review"}, AcceptedBefore: pgtype.Timestamptz{Time: time.Now().Add(100 * time.Hour), Valid: true},
	})
	if err != nil || len(got) != 0 {
		t.Errorf("the journal still holds the preview: %d rows, %v", len(got), err)
	}
}

// Regression (review of P6d): a task the train sends back to human_review long after acceptance
// gets the whole preview_hours from that move, so the person it asks finds a live preview.
func TestALateReviewRequestRestartsThePreviewClock(t *testing.T) {
	r := newRig(t)
	r.withGate(config.Review{Mode: "all"}, "solo")
	d := r.withPreview()
	r.toReview("feature.txt")
	r.advance() // accepted into human_review now
	// The move back to human_review 20 hours later, as the train's ReviewRequired writes it.
	if _, err := r.c.Pool.Exec(t.Context(), `INSERT INTO book.events (tenant, task_id, kind, actor, payload, created_at)
		VALUES ($1, $2, 'task.moved', 'system', '{"from":"ready","to":"human_review","event":"review_required"}', now() + interval '20 hours')`,
		r.task.Tenant, r.task.ID); err != nil {
		t.Fatal(err)
	}
	env := deployapi.EnvRef{Tenant: r.task.Tenant, Task: strconv.FormatInt(r.task.ID, 10)}
	for _, c := range []struct {
		after time.Duration
		want  deployapi.EnvState
	}{{25 * time.Hour, deployapi.EnvHealthy}, {45 * time.Hour, deployapi.EnvAbsent}} {
		at := time.Now().Add(c.after)
		r.c.Now = func() time.Time { return at }
		if err := r.c.TeardownPreviews(t.Context(), r.tenant, time.Minute); err != nil {
			t.Fatal(err)
		}
		if st, _ := d.Status(t.Context(), env); st.State != c.want {
			t.Fatalf("%s after acceptance the preview is %s, want %s", c.after, st.State, c.want)
		}
	}
	task, err := r.q.GetTask(t.Context(), r.task.ID)
	if err != nil || task.State != "human_review" {
		t.Errorf("after the teardown the task is in %q (%v), want human_review", task.State, err)
	}
}
