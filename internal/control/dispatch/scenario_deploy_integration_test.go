package dispatch_test

import (
	"strconv"
	"testing"
	"time"

	"github.com/ilyachakun/nightshift/internal/integration"
)

// scenarioDeploy gives a scenario's tenant a preview and a release on the deploy twin (design
// section 17, "Tests": preview, acceptance, release, a failed smoke and the automatic rollback).
type scenarioDeploy struct {
	// Previous is a revision live before the task, so a failed smoke has somewhere to go back to.
	Previous bool
	// ReleaseSmoke is the production smoke after the release: "pass" (the default) or "fail".
	ReleaseSmoke string `yaml:"release_smoke"`
}

func (r *rig) withDeploy(s scenarioDeploy) {
	r.t.Helper()
	d := r.withPreview()
	if s.Previous {
		if _, err := d.Release(r.t.Context(), earlierRev); err != nil {
			r.t.Fatal(err)
		}
	}
	if s.ReleaseSmoke == "fail" {
		d.SetSmoke(integration.EnvRef{Tenant: r.task.Tenant}, integration.SmokeResult{Detail: "the start page is blank"})
	}
	r.withRelease(d)
	r.deploy = d
}

// expectDeploy runs the teardown sweep as a tick would and checks the twin's environments.
func (r *rig) expectDeploy(t *testing.T, name, preview, production string) {
	t.Helper()
	if preview == "" && production == "" {
		return
	}
	d := r.deploy
	if d == nil {
		t.Fatalf("%s: the scenario has no deploy in given", name)
	}
	if err := r.c.TeardownPreviews(t.Context(), r.tenant, time.Minute); err != nil {
		t.Fatalf("%s: teardown: %v", name, err)
	}
	if preview != "" {
		st, err := d.Status(t.Context(), integration.EnvRef{Tenant: r.task.Tenant, Task: strconv.FormatInt(r.task.ID, 10)})
		if err != nil || string(st.State) != preview {
			t.Fatalf("%s: the preview is %+v (%v), want %s", name, st, err, preview)
		}
	}
	switch production {
	case "":
	case "previous":
		if d.Production() != earlierRev {
			t.Fatalf("%s: production is %q, want the earlier revision", name, d.Production())
		}
	case "merge":
		if got := d.Production(); got == "" || got == earlierRev {
			t.Fatalf("%s: production is %q, want the merge commit", name, got)
		}
	default:
		t.Fatalf("%s: unknown production %q", name, production)
	}
}
