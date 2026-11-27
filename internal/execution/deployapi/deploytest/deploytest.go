// Package deploytest is the contract suite every deploy target passes (docs/plan/architecture/
// item A7, design section 17 "Deploy targets"): a release reports the revision it replaced and
// repeats that answer, production runs the exact revision released, a rollback and a failed
// rollout leave the way back, and a preview goes up and down idempotently. It belongs to the
// testing plane: only tests import it.
package deploytest

import (
	"errors"
	"testing"

	"github.com/ilyachakun/nightshift/internal/execution/deployapi"
)

// Target is one fresh deploy target with nothing running.
type Target struct {
	Deployer deployapi.Deployer
	Tenant   string
	// Revisions are three distinct revisions the target can run, oldest first.
	Revisions [3]deployapi.Revision
	// FailRollout makes the next revision that is brought up start and never become healthy.
	FailRollout func()
}

// Factory returns a fresh target; each case gets its own.
type Factory func(t *testing.T) Target

// Run checks the contract on one target.
func Run(t *testing.T, f Factory) {
	t.Helper()
	t.Run("release reports the previous revision", func(t *testing.T) { releasePrevious(t, f(t)) })
	t.Run("a repeated release keeps the same previous", func(t *testing.T) { releaseRepeated(t, f(t)) })
	t.Run("rollback runs the exact revision", func(t *testing.T) { rollback(t, f(t)) })
	t.Run("a failed rollout keeps the way back", func(t *testing.T) { failedRollout(t, f(t)) })
	t.Run("preview up and down", func(t *testing.T) { preview(t, f(t)) })
	t.Run("down of an absent preview is no error", func(t *testing.T) { downAbsent(t, f(t)) })
	t.Run("production is not taken down", func(t *testing.T) { downProduction(t, f(t)) })
}

func release(t *testing.T, tg Target, rev deployapi.Revision) deployapi.Released {
	t.Helper()
	r, err := tg.Deployer.Release(t.Context(), rev)
	if err != nil {
		t.Fatalf("Release(%s): %v", rev, err)
	}
	return r
}

// wantStatus checks that e runs exactly rev and is healthy.
func wantStatus(t *testing.T, tg Target, e deployapi.EnvRef, state deployapi.EnvState, rev deployapi.Revision) {
	t.Helper()
	st, err := tg.Deployer.Status(t.Context(), e)
	if err != nil {
		t.Fatalf("Status(%+v): %v", e, err)
	}
	if st.State != state || (rev != "" && st.Revision != rev) {
		t.Errorf("Status(%+v) = %s at %q, want %s at %q", e, st.State, st.Revision, state, rev)
	}
}

func releasePrevious(t *testing.T, tg Target) {
	r1, r2 := tg.Revisions[0], tg.Revisions[1]
	wantStatus(t, tg, deployapi.EnvRef{Tenant: tg.Tenant}, deployapi.EnvAbsent, "")
	if got := release(t, tg, r1); got != (deployapi.Released{Revision: r1}) {
		t.Errorf("first release = %+v, want revision %s and no previous", got, r1)
	}
	if got := release(t, tg, r2); got != (deployapi.Released{Revision: r2, Previous: r1}) {
		t.Errorf("second release = %+v, want %s replacing %s", got, r2, r1)
	}
	wantStatus(t, tg, deployapi.EnvRef{Tenant: tg.Tenant}, deployapi.EnvHealthy, r2)
}

// releaseRepeated: the conductor repeats a Release whose answer it did not record; the second
// answer must still name the revision before it, not the one it just started.
func releaseRepeated(t *testing.T, tg Target) {
	r1, r2 := tg.Revisions[0], tg.Revisions[1]
	release(t, tg, r1)
	first := release(t, tg, r2)
	if again := release(t, tg, r2); again != first {
		t.Errorf("repeated release = %+v, first answer was %+v", again, first)
	}
	wantStatus(t, tg, deployapi.EnvRef{Tenant: tg.Tenant}, deployapi.EnvHealthy, r2)
}

func rollback(t *testing.T, tg Target) {
	r1, r2, r3 := tg.Revisions[0], tg.Revisions[1], tg.Revisions[2]
	release(t, tg, r1)
	release(t, tg, r2)
	if err := tg.Deployer.Rollback(t.Context(), r1); err != nil {
		t.Fatalf("Rollback(%s): %v", r1, err)
	}
	wantStatus(t, tg, deployapi.EnvRef{Tenant: tg.Tenant}, deployapi.EnvHealthy, r1)
	if got := release(t, tg, r3); got.Previous != r1 {
		t.Errorf("release after a rollback = %+v, want previous %s", got, r1)
	}
}

func failedRollout(t *testing.T, tg Target) {
	r1, r2 := tg.Revisions[0], tg.Revisions[1]
	release(t, tg, r1)
	tg.FailRollout()
	for _, try := range []string{"first", "repeated"} {
		got, err := tg.Deployer.Release(t.Context(), r2)
		if !errors.Is(err, deployapi.ErrRolloutFailed) {
			t.Fatalf("%s release of an unhealthy revision: err = %v, want ErrRolloutFailed", try, err)
		}
		if got.Previous != r1 {
			t.Errorf("%s failed release = %+v, want previous %s", try, got, r1)
		}
	}
	if err := tg.Deployer.Rollback(t.Context(), r1); err != nil {
		t.Fatalf("Rollback(%s): %v", r1, err)
	}
	wantStatus(t, tg, deployapi.EnvRef{Tenant: tg.Tenant}, deployapi.EnvHealthy, r1)
}

func preview(t *testing.T, tg Target) {
	e := deployapi.EnvRef{Tenant: tg.Tenant, Task: "7"}
	rev := tg.Revisions[0]
	for _, try := range []string{"first", "repeated"} {
		ep, err := tg.Deployer.Up(t.Context(), e, rev)
		if err != nil || ep.URL == "" {
			t.Fatalf("%s Up = %+v, %v; want an endpoint", try, ep, err)
		}
	}
	wantStatus(t, tg, e, deployapi.EnvHealthy, rev)
	wantStatus(t, tg, deployapi.EnvRef{Tenant: tg.Tenant}, deployapi.EnvAbsent, "")
	if res, err := tg.Deployer.Smoke(t.Context(), e); err != nil || !res.OK {
		t.Errorf("Smoke of a healthy preview = %+v, %v", res, err)
	}
	for _, try := range []string{"first", "repeated"} {
		if err := tg.Deployer.Down(t.Context(), e); err != nil {
			t.Fatalf("%s Down: %v", try, err)
		}
	}
	wantStatus(t, tg, e, deployapi.EnvAbsent, "")
	if res, err := tg.Deployer.Smoke(t.Context(), e); err != nil || res.OK {
		t.Errorf("Smoke of an absent preview = %+v, %v; want a failed check", res, err)
	}
}

func downAbsent(t *testing.T, tg Target) {
	e := deployapi.EnvRef{Tenant: tg.Tenant, Task: "8"}
	if err := tg.Deployer.Down(t.Context(), e); err != nil {
		t.Errorf("Down of a preview that never ran: %v", err)
	}
	wantStatus(t, tg, e, deployapi.EnvAbsent, "")
}

func downProduction(t *testing.T, tg Target) {
	release(t, tg, tg.Revisions[0])
	if err := tg.Deployer.Down(t.Context(), deployapi.EnvRef{Tenant: tg.Tenant}); !errors.Is(err, deployapi.ErrUnsupported) {
		t.Errorf("Down of production: err = %v, want ErrUnsupported", err)
	}
	wantStatus(t, tg, deployapi.EnvRef{Tenant: tg.Tenant}, deployapi.EnvHealthy, tg.Revisions[0])
}
