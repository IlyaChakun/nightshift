package dispatch_test

import (
	"context"
	"strconv"
	"strings"
	"testing"

	deployfake "github.com/ilyachakun/nightshift/internal/adapters/deploy/fake"
	"github.com/ilyachakun/nightshift/internal/book/bookdb"
	"github.com/ilyachakun/nightshift/internal/config"
	"github.com/ilyachakun/nightshift/internal/deploy"
	"github.com/ilyachakun/nightshift/internal/integration"
)

// withPreview gives the tenant a preview capability on the deploy twin.
func (r *rig) withPreview() *deployfake.Deployer {
	r.t.Helper()
	d := deployfake.New()
	run := &deploy.Runner{Journal: deploy.BookJournal{Q: r.q}, Deployer: d}
	r.c.Deploys = func(context.Context, config.Tenant, bookdb.BookTask) (*deploy.Runner, bool, error) {
		return run, true, nil
	}
	return d
}

func TestAHealthyPreviewLetsTheTaskThrough(t *testing.T) {
	r := newRig(t)
	r.withGate(config.Review{Mode: "all"}, "solo")
	d := r.withPreview()
	r.toReview("feature.txt")

	r.advance() // review -> preview (up, smoke) -> acceptance -> human_review
	if r.task.State != "human_review" {
		t.Fatalf("the task is in %s, want human_review", r.task.State)
	}
	env := integration.EnvRef{Tenant: r.task.Tenant, Task: strconv.FormatInt(r.task.ID, 10)}
	if st, _ := d.Status(t.Context(), env); st.State != integration.EnvHealthy || string(st.Revision) != r.task.HeadSha.String {
		t.Errorf("preview status = %+v, want healthy at the head", st)
	}
}

func TestABrokenPreviewSendsTheTaskBackToBuilderWithAFinding(t *testing.T) {
	r := newRig(t)
	r.withGate(config.Review{Mode: "all"}, "solo")
	d := r.withPreview()
	r.toReview("feature.txt")
	env := integration.EnvRef{Tenant: r.task.Tenant, Task: strconv.FormatInt(r.task.ID, 10)}
	d.SetSmoke(env, integration.SmokeResult{Detail: "the start page is blank"})

	r.advance()
	if r.task.State != "building" {
		t.Fatalf("the task is in %s, want building", r.task.State)
	}
	found, err := r.q.ListTaskFindings(t.Context(), r.task.ID)
	if err != nil || len(found) != 1 || found[0].RuleID != "preview.smoke_failed" || !strings.Contains(found[0].Message, "blank") {
		t.Fatalf("findings = %+v, %v", found, err)
	}
	if st, _ := d.Status(t.Context(), env); st.State != integration.EnvAbsent {
		t.Errorf("the broken preview was not torn down: %+v", st)
	}
}
