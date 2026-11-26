package deploy_test

import (
	"context"
	"errors"
	"slices"
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

func key(id int64, step string) string { return string(rune('a'+id)) + step }

var ctx = context.Background()

func job(rev integration.Revision) deploy.Job {
	return deploy.Job{Tenant: "example-solo", TaskID: 1, TaskKey: "t1", Rev: rev}
}

func TestPreviewReadyAndTeardown(t *testing.T) {
	t.Parallel()
	d, j := fake.New(), memJournal{}
	r := &deploy.Runner{Journal: j, Deployer: d}
	res, err := r.Preview(ctx, job("aaa"))
	if err != nil || res.Outcome != deploy.PreviewReady || res.Endpoint.URL == "" {
		t.Fatalf("preview = %+v, %v", res, err)
	}
	if err := r.Teardown(ctx, job("aaa")); err != nil {
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
	res, err := (&deploy.Runner{Journal: memJournal{}, Deployer: d}).Preview(ctx, job("aaa"))
	if err != nil || res.Outcome != deploy.PreviewBroken || res.Detail != "login page is blank" {
		t.Fatalf("preview = %+v, %v", res, err)
	}
}

func TestPreviewPlatformErrorIsNotACodeFailure(t *testing.T) {
	t.Parallel()
	d := fake.New()
	boom := errors.New("engine down")
	d.FailNext("up", boom)
	res, err := (&deploy.Runner{Journal: memJournal{}, Deployer: d}).Preview(ctx, job("aaa"))
	if !errors.Is(err, boom) || res.Outcome == deploy.PreviewBroken {
		t.Fatalf("preview = %+v, %v; want the platform error", res, err)
	}
}

func TestSmokeIsNotRepeatedAfterARestart(t *testing.T) {
	t.Parallel()
	d, j := fake.New(), memJournal{}
	if _, err := (&deploy.Runner{Journal: j, Deployer: d}).Preview(ctx, job("aaa")); err != nil {
		t.Fatal(err)
	}
	if _, err := (&deploy.Runner{Journal: j, Deployer: d}).Preview(ctx, job("aaa")); err != nil {
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
