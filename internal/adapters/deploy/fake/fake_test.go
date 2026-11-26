package fake

import (
	"context"
	"errors"
	"testing"

	"github.com/ilyachakun/nightshift/internal/integration"
)

var (
	ctx     = context.Background()
	preview = integration.EnvRef{Tenant: "example-solo", Task: "t1"}
	prod    = integration.EnvRef{}
)

func TestPreviewLifecycleIsIdempotent(t *testing.T) {
	t.Parallel()
	d := New()
	if st, _ := d.Status(ctx, preview); st.State != integration.EnvAbsent {
		t.Fatalf("new env state = %q, want absent", st.State)
	}
	ep1, err := d.Up(ctx, preview, "abc")
	if err != nil {
		t.Fatal(err)
	}
	ep2, _ := d.Up(ctx, preview, "abc")
	if ep1 != ep2 {
		t.Errorf("repeated Up changed the endpoint: %v vs %v", ep1, ep2)
	}
	if st, _ := d.Status(ctx, preview); st.State != integration.EnvHealthy || st.Revision != "abc" {
		t.Errorf("status = %+v, want healthy at abc", st)
	}
	if r, _ := d.Smoke(ctx, preview); !r.OK {
		t.Errorf("smoke of a healthy preview failed: %+v", r)
	}
	for i := 0; i < 2; i++ { // Down twice: the second finds nothing and is fine
		if err := d.Down(ctx, preview); err != nil {
			t.Fatal(err)
		}
	}
	if st, _ := d.Status(ctx, preview); st.State != integration.EnvAbsent {
		t.Errorf("after Down state = %q, want absent", st.State)
	}
	if r, _ := d.Smoke(ctx, preview); r.OK {
		t.Error("smoke of an absent preview passed")
	}
}

func TestUpNeedsATask(t *testing.T) {
	t.Parallel()
	if _, err := New().Up(ctx, prod, "abc"); err == nil {
		t.Error("Up accepted production; it must come up by Release")
	}
}

func TestReleaseKeepsThePreviousRevisionAcrossRetries(t *testing.T) {
	t.Parallel()
	d := New()
	r1, _ := d.Release(ctx, "r1")
	if r1.Previous != "" {
		t.Errorf("first release Previous = %q, want empty", r1.Previous)
	}
	r2, _ := d.Release(ctx, "r2")
	again, _ := d.Release(ctx, "r2") // a retry after a restart
	if r2.Previous != "r1" || again != r2 {
		t.Errorf("release r2 = %+v, retry = %+v; want Previous r1 both times", r2, again)
	}
	if err := d.Rollback(ctx, r2.Previous); err != nil {
		t.Fatal(err)
	}
	if got := d.Production(); got != "r1" {
		t.Errorf("production after rollback = %q, want r1", got)
	}
	if st, _ := d.Status(ctx, prod); st.Revision != "r1" || st.State != integration.EnvHealthy {
		t.Errorf("production status = %+v", st)
	}
}

func TestInjectedFailureHitsOnce(t *testing.T) {
	t.Parallel()
	d := New()
	boom := errors.New("engine down")
	d.FailNext("up", boom)
	if _, err := d.Up(ctx, preview, "abc"); !errors.Is(err, boom) {
		t.Fatalf("Up error = %v, want the injected one", err)
	}
	if _, err := d.Up(ctx, preview, "abc"); err != nil {
		t.Errorf("second Up failed: %v", err)
	}
}

func TestSmokeCanBeScriptedAndStatusSeesACrash(t *testing.T) {
	t.Parallel()
	d := New()
	_, _ = d.Up(ctx, preview, "abc")
	d.SetState(preview, integration.EnvUnhealthy)
	if r, _ := d.Smoke(ctx, preview); r.OK {
		t.Error("smoke passed on an unhealthy environment")
	}
	d.SetSmoke(preview, integration.SmokeResult{OK: false, Detail: "main path broke"})
	if r, _ := d.Smoke(ctx, preview); r.Detail != "main path broke" {
		t.Errorf("scripted smoke = %+v", r)
	}
	if got := d.Calls(); len(got) != 3 || got[0] != "up abc" {
		t.Errorf("calls = %v", got)
	}
}

func TestDownRefusesProduction(t *testing.T) {
	t.Parallel()
	d := New()
	_, _ = d.Release(ctx, "r1")
	if err := d.Down(ctx, prod); err == nil {
		t.Error("Down accepted production")
	}
	if st, _ := d.Status(ctx, prod); st.State != integration.EnvHealthy {
		t.Errorf("production status after refused Down = %+v", st)
	}
}
