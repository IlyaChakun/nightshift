package dispatch_test

import (
	"context"
	"errors"
	"strconv"
	"testing"

	"github.com/ilyachakun/nightshift/internal/book/bookdb"
	"github.com/ilyachakun/nightshift/internal/config"
	"github.com/ilyachakun/nightshift/internal/deploy"
	"github.com/ilyachakun/nightshift/internal/dispatch"
	"github.com/ilyachakun/nightshift/internal/integration"
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
		if err := r.c.TeardownPreviews(t.Context(), r.tenant); err != nil {
			t.Fatal(err)
		}
	}
	if st, _ := d.Status(t.Context(), env); st.State != integration.EnvAbsent {
		t.Errorf("after the sweep the preview is %+v, want absent", st)
	}
	if n := r.previewSteps(); n != 0 {
		t.Errorf("%d preview rows left in the journal", n)
	}
	if d.Production() == "" {
		t.Error("the sweep touched production")
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
	if err := r.c.TeardownPreviews(t.Context(), r.tenant); err != nil {
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
	if err := r.c.TeardownPreviews(t.Context(), r.tenant); err == nil {
		t.Fatal("the failed teardown was not reported")
	}
	if err := r.c.TeardownPreviews(t.Context(), r.tenant); err != nil {
		t.Fatal(err)
	}
	if st, _ := d.Status(t.Context(), env); st.State != integration.EnvAbsent {
		t.Errorf("the canceled task's preview is %+v, want absent", st)
	}
}

// A tenant that dropped its preview capability cannot reach the environment: the journal is
// cleared, so the sweep does not ask again on every tick.
func TestTheSweepStopsWhenThePreviewCapabilityIsGone(t *testing.T) {
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
	for range 2 {
		if err := r.c.TeardownPreviews(t.Context(), r.tenant); err != nil {
			t.Fatal(err)
		}
	}
	if calls != 1 {
		t.Errorf("the capability was read %d times, want once", calls)
	}
}
