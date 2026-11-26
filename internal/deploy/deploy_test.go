package deploy_test

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/ilyachakun/nightshift/internal/adapters/deploy/fake"
	"github.com/ilyachakun/nightshift/internal/deploy"
	"github.com/ilyachakun/nightshift/internal/integration"
)

// memJournal is the journal in memory; a new Runner over the same memJournal stands for a restart.
type memJournal map[string]*deploy.Step

func (m memJournal) Start(_ context.Context, _ string, taskID int64, step string) (deploy.Step, error) {
	k := key(taskID, step)
	if s, ok := m[k]; ok {
		return *s, nil
	}
	m[k] = &deploy.Step{Name: step}
	return *m[k], nil
}

func (m memJournal) Finish(_ context.Context, taskID int64, s deploy.Step) error {
	s.Done = true
	m[key(taskID, s.Name)] = &s
	return nil
}

func (m memJournal) ClearPreview(_ context.Context, taskID int64) error {
	delete(m, key(taskID, deploy.StepUp))
	delete(m, key(taskID, deploy.StepSmoke))
	return nil
}

func (m memJournal) Reset(_ context.Context, taskID int64, step string) error {
	delete(m, key(taskID, step))
	return nil
}

func key(id int64, step string) string { return strconv.FormatInt(id, 10) + "/" + step }

var ctx = context.Background()

func job() deploy.Job {
	return deploy.Job{Tenant: "example-solo", TaskID: 1, TaskKey: "t1", Rev: "aaa"}
}

func TestPreviewReadyAndTeardown(t *testing.T) {
	t.Parallel()
	d, j := fake.New(), memJournal{}
	r := &deploy.Runner{Journal: j, Deployer: d}
	res, err := r.Preview(ctx, job())
	if err != nil || res.Outcome != deploy.PreviewReady || res.Endpoint.URL == "" {
		t.Fatalf("preview = %+v, %v", res, err)
	}
	if err := r.Teardown(ctx, job()); err != nil {
		t.Fatal(err)
	}
	if st, _ := d.Status(ctx, integration.EnvRef{Tenant: "example-solo", Task: "t1"}); st.State != integration.EnvAbsent {
		t.Errorf("after teardown: %+v", st)
	}
	if len(j) != 0 {
		t.Errorf("journal after teardown: %v", j)
	}
}

func TestPreviewWithAFailedSmokeIsTheCodesFault(t *testing.T) {
	t.Parallel()
	d := fake.New()
	d.SetSmoke(integration.EnvRef{Tenant: "example-solo", Task: "t1"}, integration.SmokeResult{Detail: "login page is blank"})
	res, err := (&deploy.Runner{Journal: memJournal{}, Deployer: d}).Preview(ctx, job())
	if err != nil || res.Outcome != deploy.PreviewBroken || res.Detail != "login page is blank" {
		t.Fatalf("preview = %+v, %v", res, err)
	}
}

func TestPreviewRefusedRecipeIsTheCodesFault(t *testing.T) {
	t.Parallel()
	d := fake.New()
	d.FailNext("up", fmt.Errorf("service db mounts the host: %w", integration.ErrRefused))
	res, err := (&deploy.Runner{Journal: memJournal{}, Deployer: d}).Preview(ctx, job())
	if err != nil || res.Outcome != deploy.PreviewBroken || !strings.Contains(res.Detail, "mounts the host") {
		t.Fatalf("preview = %+v, %v; want a broken preview with the reason", res, err)
	}
}

func TestPreviewPlatformErrorIsNotACodeFailure(t *testing.T) {
	t.Parallel()
	d := fake.New()
	boom := errors.New("engine down")
	d.FailNext("up", boom)
	res, err := (&deploy.Runner{Journal: memJournal{}, Deployer: d}).Preview(ctx, job())
	if !errors.Is(err, boom) || res.Outcome == deploy.PreviewBroken {
		t.Fatalf("preview = %+v, %v; want the platform error", res, err)
	}
}

func TestSmokeIsNotRepeatedAfterARestart(t *testing.T) {
	t.Parallel()
	d, j := fake.New(), memJournal{}
	if _, err := (&deploy.Runner{Journal: j, Deployer: d}).Preview(ctx, job()); err != nil {
		t.Fatal(err)
	}
	if _, err := (&deploy.Runner{Journal: j, Deployer: d}).Preview(ctx, job()); err != nil {
		t.Fatal(err)
	}
	if n := count(d.Calls(), "smoke"); n != 1 {
		t.Errorf("smoke ran %d times, want once; calls %v", n, d.Calls())
	}
}

func TestReleaseSmokeFailureRollsBackOnItsOwn(t *testing.T) {
	t.Parallel()
	d, j := fake.New(), memJournal{}
	r := &deploy.Runner{Journal: j, Deployer: d}
	if res, err := r.Release(ctx, deploy.Job{Tenant: "example-solo", TaskID: 1, Rev: "r1"}); err != nil || res.Outcome != deploy.Released {
		t.Fatalf("first release = %+v, %v", res, err)
	}
	d.SetSmoke(integration.EnvRef{}, integration.SmokeResult{Detail: "checkout returns 500"})
	res, err := r.Release(ctx, deploy.Job{Tenant: "example-solo", TaskID: 2, Rev: "r2"})
	if err != nil || res.Outcome != deploy.RolledBack || res.Previous != "r1" || res.Detail != "checkout returns 500" {
		t.Fatalf("second release = %+v, %v", res, err)
	}
	if got := d.Production(); got != "r1" {
		t.Errorf("production = %q, want r1 after the rollback", got)
	}
}

// A revision whose containers never become healthy fails the release call itself. The answer is
// the same as for a failed smoke: roll back, and do not retry the release.
func TestReleaseThatNeverComesUpRollsBackOnItsOwn(t *testing.T) {
	t.Parallel()
	d, j := fake.New(), memJournal{}
	r := &deploy.Runner{Journal: j, Deployer: d}
	if _, err := r.Release(ctx, deploy.Job{Tenant: "example-solo", TaskID: 1, Rev: "r1"}); err != nil {
		t.Fatal(err)
	}
	d.FailRollout()
	boom := errors.New("engine down")
	two := deploy.Job{Tenant: "example-solo", TaskID: 2, Rev: "r2"}
	d.FailNext("rollback", boom)
	if _, err := r.Release(ctx, two); !errors.Is(err, boom) {
		t.Fatalf("first attempt err = %v, want the rollback failure", err)
	}
	res, err := (&deploy.Runner{Journal: j, Deployer: d}).Release(ctx, two) // the restart
	if err != nil || res.Outcome != deploy.RolledBack || res.Previous != "r1" || res.Detail == "" {
		t.Fatalf("resumed release = %+v, %v", res, err)
	}
	if got := d.Production(); got != "r1" {
		t.Errorf("production = %q, want r1", got)
	}
	if n := count(d.Calls(), "release r2"); n != 1 || count(d.Calls(), "smoke") != 1 { // the one smoke is r1's
		t.Errorf("release r2 ran %d times, smoke %d times; calls %v", n, count(d.Calls(), "smoke"), d.Calls())
	}
}

func TestFirstReleaseThatNeverComesUpHasNothingToRollBackTo(t *testing.T) {
	t.Parallel()
	d := fake.New()
	d.FailRollout()
	res, err := (&deploy.Runner{Journal: memJournal{}, Deployer: d}).Release(ctx, deploy.Job{Tenant: "example-solo", TaskID: 1, Rev: "r1"})
	if err != nil || res.Outcome != deploy.RollbackImpossible {
		t.Fatalf("release = %+v, %v", res, err)
	}
}

func TestFailedFirstReleaseHasNothingToRollBackTo(t *testing.T) {
	t.Parallel()
	d := fake.New()
	d.SetSmoke(integration.EnvRef{}, integration.SmokeResult{Detail: "down"})
	res, err := (&deploy.Runner{Journal: memJournal{}, Deployer: d}).Release(ctx, deploy.Job{Tenant: "example-solo", TaskID: 1, Rev: "r1"})
	if err != nil || res.Outcome != deploy.RollbackImpossible {
		t.Fatalf("release = %+v, %v", res, err)
	}
	if slices.Contains(d.Calls(), "rollback") {
		t.Error("rollback was called without a previous revision")
	}
}

// A crash after the release but before the smoke, and one after a failed rollback, must neither
// repeat the release nor lose the previous revision.
func TestReleaseResumesFromTheJournal(t *testing.T) {
	t.Parallel()
	d, j := fake.New(), memJournal{}
	if _, err := (&deploy.Runner{Journal: j, Deployer: d}).Release(ctx, deploy.Job{Tenant: "example-solo", TaskID: 1, Rev: "r1"}); err != nil {
		t.Fatal(err)
	}
	d.SetSmoke(integration.EnvRef{}, integration.SmokeResult{Detail: "500"})
	boom := errors.New("engine down")
	two := deploy.Job{Tenant: "example-solo", TaskID: 2, Rev: "r2"}
	d.FailNext("rollback", boom)
	if _, err := (&deploy.Runner{Journal: j, Deployer: d}).Release(ctx, two); !errors.Is(err, boom) {
		t.Fatalf("first attempt err = %v, want the rollback failure", err)
	}
	res, err := (&deploy.Runner{Journal: j, Deployer: d}).Release(ctx, two) // the restart
	if err != nil || res.Outcome != deploy.RolledBack || res.Previous != "r1" {
		t.Fatalf("resumed release = %+v, %v", res, err)
	}
	if n := count(d.Calls(), "release r2"); n != 1 {
		t.Errorf("release r2 ran %d times, want once; calls %v", n, d.Calls())
	}
}

func TestAnAdapterWithoutSmokePasses(t *testing.T) {
	t.Parallel()
	res, err := (&deploy.Runner{Journal: memJournal{}, Deployer: noSmoke{fake.New()}}).Release(ctx, deploy.Job{Tenant: "example-solo", TaskID: 1, Rev: "r1"})
	if err != nil || res.Outcome != deploy.Released {
		t.Fatalf("release = %+v, %v", res, err)
	}
}

type noSmoke struct{ *fake.Deployer }

func (noSmoke) Smoke(context.Context, integration.EnvRef) (integration.SmokeResult, error) {
	return integration.SmokeResult{}, integration.ErrUnsupported
}

func count(calls []string, c string) int {
	n := 0
	for _, x := range calls {
		if x == c {
			n++
		}
	}
	return n
}

// A task that came back after a rollback releases a new revision: the old answers must not be reused.
func TestANewRevisionOfTheSameTaskReleasesAgain(t *testing.T) {
	t.Parallel()
	d, j := fake.New(), memJournal{}
	r := &deploy.Runner{Journal: j, Deployer: d}
	d.SetSmoke(integration.EnvRef{}, integration.SmokeResult{Detail: "500"})
	if _, err := r.Release(ctx, deploy.Job{Tenant: "example-solo", TaskID: 1, Rev: "r1"}); err != nil {
		t.Fatal(err)
	}
	d.SetSmoke(integration.EnvRef{}, integration.SmokeResult{OK: true})
	res, err := r.Release(ctx, deploy.Job{Tenant: "example-solo", TaskID: 1, Rev: "r2"})
	if err != nil || res.Outcome != deploy.Released {
		t.Fatalf("release of the fixed revision = %+v, %v", res, err)
	}
	if got := d.Production(); got != "r2" {
		t.Errorf("production = %q, want r2", got)
	}
}
