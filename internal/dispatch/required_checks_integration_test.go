package dispatch_test

import (
	"context"
	"errors"
	"testing"
	"time"

	deployfake "github.com/ilyachakun/nightshift/internal/adapters/deploy/fake"
	"github.com/ilyachakun/nightshift/internal/config"
)

func (r *rig) assertHeld(state string, d *deployfake.Deployer) {
	r.t.Helper()
	pr, ok := r.twin.PR("example/app", 1)
	if !ok || r.task.State != state || pr.Merged || len(d.Calls()) != 0 {
		r.t.Fatalf("state %s, PR %+v, deploy calls %v; want %s and no publication", r.task.State, pr, d.Calls(), state)
	}
}

func requiredReview(t *testing.T) (*rig, *deployfake.Deployer) {
	t.Helper()
	r := newRig(t)
	r.withGate(config.Review{Mode: "blast_radius", Actions: map[string]string{"low": "auto"}, SLAAutomergeHours: 1}, "solo")
	d := deployfake.New()
	r.withRelease(d)
	r.toReview("docs/guides/x.md")
	return r, d
}

func TestRequiredReviewAwaitsEvidence(t *testing.T) {
	t.Run("unconfigured", func(t *testing.T) {
		r, d := requiredReview(t)
		r.advance()
		r.assertHeld("human_review", d)
		var recorded bool
		if err := r.c.Pool.QueryRow(t.Context(), `SELECT EXISTS (
			SELECT 1 FROM book.events WHERE task_id=$1 AND kind='task.moved'
			AND payload->>'event'='verifier_unavailable')`, r.task.ID).Scan(&recorded); err != nil || !recorded {
			t.Fatalf("unavailable review recorded: %v, %v", recorded, err)
		}
		// Restoring the configuration does not change this head's review history.
		r.withVerifier()
		r.after(2 * time.Hour)
		r.advance()
		r.assertHeld("human_review", d)
	})
	for _, name := range []string{"failed", "unreadable"} {
		t.Run(name, func(t *testing.T) {
			r, d := requiredReview(t)
			r.withVerifier()
			r.advance()
			if name == "failed" {
				if _, err := r.c.Pool.Exec(t.Context(), "UPDATE book.jobs SET state='failed', exit_reason='error', finished_at=now() WHERE id=$1", r.verifierJob().ID); err != nil {
					t.Fatal(err)
				}
			} else {
				r.verdict(`{"verdict":"unknown"}`)
			}
			r.advance()
			r.assertHeld("escalated", d)
		})
	}
}

func TestReleaseAwaitsKnownPolicyAndRetries(t *testing.T) {
	for _, condition := range []string{"capability", "window"} {
		t.Run(condition, func(t *testing.T) {
			r := newRig(t)
			r.withGate(config.Review{Mode: "all"}, "solo")
			d := deployfake.New()
			r.withRelease(d)
			r.toReady()
			if condition == "capability" {
				r.c.Train.Releases = func(context.Context, string) (bool, error) { return false, errors.New("capability unavailable") }
			} else {
				r.tenant.Schedule.QuietHours = "invalid"
			}
			r.advance()
			r.assertHeld("merging", d)
			r.c.Train.Releases = nil
			r.tenant.Schedule.QuietHours = ""
			r.advance()
			if r.task.State != "released" || d.Production() == "" {
				t.Fatalf("state %s, deployed %q", r.task.State, d.Production())
			}
		})
	}
}
