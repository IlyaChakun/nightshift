package dispatch_test

import (
	"context"
	"testing"
	"time"

	"github.com/ilyachakun/nightshift/internal/control/dispatch"
	"github.com/ilyachakun/nightshift/internal/control/flow"
	"github.com/ilyachakun/nightshift/internal/data/book/bookdb"
	"github.com/ilyachakun/nightshift/internal/observability/summary"
	"github.com/ilyachakun/nightshift/internal/testing/testutil/pgtest"
)

type healthSource struct {
	alerts []summary.Alert
	prs    []summary.PRActivity
}

func (h *healthSource) OpenAlerts(context.Context, string) ([]summary.Alert, error) {
	return h.alerts, nil
}

func (h *healthSource) OpenPullRequests(context.Context, string) ([]summary.PRActivity, error) {
	return h.prs, nil
}

func (h *healthSource) DefaultBranchFailures(context.Context, string) ([]string, error) {
	return nil, nil
}

func TestHealthTasksOpenOnceAndCancelWhenTheAlertIsGone(t *testing.T) {
	db := pgtest.Start(t)
	pool := db.ServicePool(t)
	ctx := t.Context()
	tenant := soloTenant()
	now := time.Date(2026, 11, 26, 9, 0, 0, 0, time.UTC)
	src := &healthSource{
		alerts: []summary.Alert{{Number: 4, Rule: "go/x", Severity: "high", Path: "a.go", Created: now.Add(-40 * time.Hour)}},
		prs:    []summary.PRActivity{{Number: 7, Title: "t", Updated: now.Add(-30 * time.Hour)}},
	}
	h := &dispatch.HealthTasks{Pool: pool, Mover: &flow.Mover{Pool: pool}, Source: src, Now: func() time.Time { return now }}
	queued := func() []bookdb.BookTask {
		ts, err := bookdb.New(pool).ListActiveTasks(ctx, bookdb.ListActiveTasksParams{Tenant: tenant.Name, States: []string{"queued"}})
		if err != nil {
			t.Fatal(err)
		}
		return ts
	}

	if err := h.Run(ctx, tenant); err != nil {
		t.Fatal(err)
	}
	if got := queued(); len(got) != 2 || got[0].Priority != -5 {
		t.Fatalf("tasks = %+v, want an alert task and a pull request task at priority -5", got)
	}
	now = now.Add(2 * time.Hour)
	if err := h.Run(ctx, tenant); err != nil {
		t.Fatal(err)
	}
	if got := queued(); len(got) != 2 {
		t.Fatalf("a second run opened tasks again: %+v", got)
	}

	src.alerts = nil // the alert is fixed
	now = now.Add(2 * time.Hour)
	if err := h.Run(ctx, tenant); err != nil {
		t.Fatal(err)
	}
	if got := queued(); len(got) != 1 {
		t.Fatalf("the task of the fixed alert must be canceled, the pull request task stays: %+v", got)
	}
}
