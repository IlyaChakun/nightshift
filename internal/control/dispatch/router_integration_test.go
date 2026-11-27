package dispatch_test

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/ilyachakun/nightshift/internal/control/dispatch"
	"github.com/ilyachakun/nightshift/internal/customization/config"
	"github.com/ilyachakun/nightshift/internal/data/book/bookdb"
	"github.com/ilyachakun/nightshift/internal/execution/jobsapi"
	"github.com/ilyachakun/nightshift/internal/testutil/pgtest"
)

// TestRouterEnqueuesOnTheBook checks the concurrency cap against real jobs, the session
// rule on a provider switch and the budget counters (T3).
func TestRouterEnqueuesOnTheBook(t *testing.T) {
	db := pgtest.Start(t)
	pool := db.ServicePool(t)
	q := bookdb.New(pool)
	ctx := t.Context()
	task, err := q.CreateTask(ctx, bookdb.CreateTaskParams{Tenant: "example-solo", Repo: "example/app", Title: "Route"})
	if err != nil {
		t.Fatal(err)
	}
	r := &dispatch.Router{
		Pool: pool,
		Providers: map[string]config.Provider{
			"claude-sub": {Harness: "claude", Auth: "subscription", Concurrency: 1},
			"claude-api": {Harness: "claude", Auth: "api_key"},
		},
		Chains: map[string][]string{"builder": {"claude-sub", "claude-api"}},
	}
	if err := q.SaveHarnessSession(ctx, bookdb.SaveHarnessSessionParams{TaskID: task.ID, Role: "builder", Provider: "claude-sub", SessionID: "s-sub"}); err != nil {
		t.Fatal(err)
	}
	enqueue := func() dispatch.Choice {
		t.Helper()
		c, err := r.EnqueueJob(ctx, q, "example-solo", task.ID, "builder", jobsapi.Job{TaskID: "1", Prompt: "go"})
		if err != nil {
			t.Fatal(err)
		}
		return c
	}
	if c := enqueue(); c.Provider != "claude-sub" {
		t.Fatalf("first: %+v", c)
	}
	if c := enqueue(); c.Provider != "claude-api" { // the subscription holds one job at a time
		t.Fatalf("second: %+v", c)
	}
	jobs := map[string]jobsapi.Job{}
	rows, err := pool.Query(ctx, `SELECT provider, payload FROM book.jobs ORDER BY id`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	for rows.Next() {
		var p string
		var raw []byte
		if err := rows.Scan(&p, &raw); err != nil {
			t.Fatal(err)
		}
		var j jobsapi.Job
		if err := json.Unmarshal(raw, &j); err != nil {
			t.Fatal(err)
		}
		jobs[p] = j
	}
	if jobs["claude-sub"].Harness != "claude" || jobs["claude-sub"].Auth != "subscription" || jobs["claude-api"].Auth != "api_key" {
		t.Fatalf("harness and auth: %+v", jobs)
	}
	if jobs["claude-sub"].SessionID != "s-sub" || jobs["claude-api"].SessionID != "" {
		t.Fatalf("sessions: %+v", jobs)
	}
}

// TestSpendCountsAgainstTheBudget checks the budget counters and the exhaustion flag (T3).
func TestSpendCountsAgainstTheBudget(t *testing.T) {
	db := pgtest.Start(t)
	q := bookdb.New(db.ServicePool(t))
	ctx := t.Context()
	task, err := q.CreateTask(ctx, bookdb.CreateTaskParams{Tenant: "example-solo", Repo: "example/app", Title: "Budget"})
	if err != nil {
		t.Fatal(err)
	}
	if err := q.CreateBudget(ctx, bookdb.CreateBudgetParams{TaskID: task.ID, MaxIterations: 2, MaxTokens: 1000,
		MaxDuration: pgtype.Interval{Microseconds: time.Hour.Microseconds(), Valid: true}}); err != nil {
		t.Fatal(err)
	}
	if done, err := dispatch.Spend(ctx, q, task.ID, 1, 400); err != nil || done {
		t.Fatalf("first spend: %v %v", done, err)
	}
	if done, err := dispatch.Spend(ctx, q, task.ID, 1, 700); err != nil || !done {
		t.Fatalf("second spend: %v %v", done, err)
	}
}
