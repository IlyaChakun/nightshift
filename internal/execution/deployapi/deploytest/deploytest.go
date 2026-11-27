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
	// Revisions are two distinct revisions that come up healthy, oldest first.
	Revisions [2]deployapi.Revision
	// Bad is a revision that starts and never becomes healthy, each time it is brought up.
	Bad deployapi.Revision
}

// Factory returns a fresh target; each case gets its own.
type Factory func(t *testing.T) Target

// Run checks the contract on one target. The cases run in parallel, each on its own target, so
// a target on a real engine must give each case its own tenant.
func Run(t *testing.T, f Factory) {
	t.Helper()
	cases := []struct {
		name  string
		check func(*testing.T, Target)
	}{
		{"release reports the previous revision", releasePrevious},
		{"a repeated release keeps the same previous", releaseRepeated},
		{"rollback runs the exact revision", rollback},
		{"a failed rollout keeps the way back", failedRollout},
		{"preview up and down", preview},
		{"down of an absent preview is no error", downAbsent},
		{"production is not taken down", downProduction},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			c.check(t, f(t))
		})
	}
}

func release(t *testing.T, tg Target, rev deployapi.Revision) deployapi.Released {
	t.Helper()
	r, err := tg.Deployer.Release(t.Context(), rev)
	if err != nil {
		t.Fatalf("Release(%s): %v", rev, err)
	}
	return r
}

// wantStatus checks that e is in state and, when rev is not empty, runs exactly rev.
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
	if res, err := tg.Deployer.Smoke(t.Context(), deployapi.EnvRef{Tenant: tg.Tenant}); err != nil || !res.OK {
		t.Errorf("Smoke of healthy production = %+v, %v", res, err)
	}
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

// rollback: the conductor repeats a Rollback it did not record, and a later release of the
// revision it left must answer afresh, not replay the old record.
func rollback(t *testing.T, tg Target) {
	r1, r2 := tg.Revisions[0], tg.Revisions[1]
	release(t, tg, r1)
	release(t, tg, r2)
	for _, try := range []string{"first", "repeated"} {
		if err := tg.Deployer.Rollback(t.Context(), r1); err != nil {
			t.Fatalf("%s Rollback(%s): %v", try, r1, err)
		}
	}
	wantStatus(t, tg, deployapi.EnvRef{Tenant: tg.Tenant}, deployapi.EnvHealthy, r1)
	if got := release(t, tg, r2); got != (deployapi.Released{Revision: r2, Previous: r1}) {
		t.Errorf("release of %s after rolling back to %s = %+v", r2, r1, got)
	}
}

func failedRollout(t *testing.T, tg Target) {
	r1, prod := tg.Revisions[0], deployapi.EnvRef{Tenant: tg.Tenant}
	release(t, tg, r1)
	for _, try := range []string{"first", "repeated"} {
		got, err := tg.Deployer.Release(t.Context(), tg.Bad)
		if !errors.Is(err, deployapi.ErrRolloutFailed) {
			t.Fatalf("%s release of an unhealthy revision: err = %v, want ErrRolloutFailed", try, err)
		}
		if got != (deployapi.Released{Revision: tg.Bad, Previous: r1}) {
			t.Errorf("%s failed release = %+v, want %s replacing %s", try, got, tg.Bad, r1)
		}
	}
	// After a restart the conductor reads where it stands from Status alone.
	st, err := tg.Deployer.Status(t.Context(), prod)
	if err != nil {
		t.Fatal(err)
	}
	if st != (deployapi.EnvStatus{State: deployapi.EnvUnhealthy, Revision: tg.Bad, Previous: r1}) {
		t.Errorf("Status after a failed rollout = %+v, want unhealthy at %s with previous %s", st, tg.Bad, r1)
	}
	if res, err := tg.Deployer.Smoke(t.Context(), prod); err != nil || res.OK {
		t.Errorf("Smoke of unhealthy production = %+v, %v; want a failed check", res, err)
	}
	if err := tg.Deployer.Rollback(t.Context(), r1); err != nil {
		t.Fatalf("Rollback(%s): %v", r1, err)
	}
	wantStatus(t, tg, prod, deployapi.EnvHealthy, r1)
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
