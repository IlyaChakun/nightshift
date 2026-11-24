package jobsapi_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/ilyachakun/nightshift/internal/book/bookdb"
	"github.com/ilyachakun/nightshift/internal/jobsapi"
	"github.com/ilyachakun/nightshift/internal/testutil/pgtest"
)

const workerToken = "worker-secret-for-tests"

func call(t *testing.T, h http.Handler, method, path, token string, body any) *httptest.ResponseRecorder {
	t.Helper()
	b, err := json.Marshal(body)
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequestWithContext(t.Context(), method, path, bytes.NewReader(b))
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

// TestAuthWithoutDatabase covers the checks that run before any query (T1).
func TestAuthWithoutDatabase(t *testing.T) {
	t.Parallel()
	h := (&jobsapi.Server{WorkerTokenSHA256: jobsapi.HashToken(workerToken)}).Handler()
	for name, tc := range map[string]struct{ path, token string }{
		"claim without a token":     {"/jobs/claim", ""},
		"claim with a wrong token":  {"/jobs/claim", "nope"},
		"heartbeat without a token": {"/jobs/1/heartbeat", ""},
		"complete without a token":  {"/jobs/1/complete", ""},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			if rec := call(t, h, http.MethodPost, tc.path, tc.token, map[string]any{"worker_id": "w1", "exit_reason": "done"}); rec.Code != http.StatusUnauthorized {
				t.Fatalf("status %d: %s", rec.Code, rec.Body)
			}
		})
	}
}

// TestJobLifecycle runs claim, heartbeat, events and complete against a real book (T3).
func TestJobLifecycle(t *testing.T) {
	db := pgtest.Start(t)
	pool := db.ServicePool(t)
	q := bookdb.New(pool)
	ctx := t.Context()
	task, err := q.CreateTask(ctx, bookdb.CreateTaskParams{Tenant: "example-solo", Repo: "example/app", Title: "Jobs API"})
	if err != nil {
		t.Fatal(err)
	}
	enqueue := func() bookdb.BookJob {
		j, err := q.EnqueueJob(ctx, bookdb.EnqueueJobParams{
			Tenant: "example-solo", TaskID: task.ID, Role: "builder", Provider: "claude-api",
			Payload: []byte(`{"repo":"example/app","base_sha":"abc","prompt":"do it"}`),
		})
		if err != nil {
			t.Fatal(err)
		}
		return j
	}
	first := enqueue()
	enqueue()

	clock := time.Date(2026, 11, 24, 12, 0, 0, 0, time.UTC)
	s := &jobsapi.Server{Pool: pool, WorkerTokenSHA256: jobsapi.HashToken(workerToken), Lease: time.Minute, Now: func() time.Time { return clock }}
	h := s.Handler()

	claim := func() jobsapi.ClaimResponse {
		t.Helper()
		rec := call(t, h, http.MethodPost, "/jobs/claim", workerToken, jobsapi.ClaimRequest{WorkerID: "w1"})
		var resp jobsapi.ClaimResponse
		if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil || rec.Code != http.StatusOK {
			t.Fatalf("claim: %d %s %v", rec.Code, rec.Body, err)
		}
		return resp
	}
	a := claim()
	if a.JobToken == "" || a.Job.Role != "builder" || a.Job.Prompt != "do it" || a.Job.TaskID == "" {
		t.Fatalf("claim: %+v", a)
	}
	b := claim()
	if b.JobID == a.JobID || b.JobToken == a.JobToken {
		t.Fatalf("two claims got the same job: %+v %+v", a, b)
	}
	if got := claim(); got.JobID != "" {
		t.Fatalf("an empty queue returned %+v", got)
	}

	t.Run("a token works only for its own job", func(t *testing.T) {
		if rec := call(t, h, http.MethodPost, "/jobs/"+b.JobID+"/heartbeat", a.JobToken, jobsapi.HeartbeatRequest{}); rec.Code != http.StatusUnauthorized {
			t.Fatalf("status %d", rec.Code)
		}
		if rec := call(t, h, http.MethodPost, "/jobs/"+a.JobID+"/heartbeat", workerToken, jobsapi.HeartbeatRequest{}); rec.Code != http.StatusUnauthorized {
			t.Fatalf("the worker token worked as a job token: %d", rec.Code)
		}
	})
	t.Run("events are prefixed and attributed to the worker", func(t *testing.T) {
		ev := jobsapi.EventsRequest{Events: []jobsapi.Event{{At: clock, Kind: "decision.approved"}}}
		if rec := call(t, h, http.MethodPost, "/jobs/"+a.JobID+"/events", a.JobToken, ev); rec.Code != http.StatusOK {
			t.Fatalf("status %d: %s", rec.Code, rec.Body)
		}
		events, err := q.ListTaskEvents(ctx, pgtype.Int8{Int64: task.ID, Valid: true})
		if err != nil {
			t.Fatal(err)
		}
		last := events[len(events)-1]
		if last.Kind != "worker.decision.approved" || last.Actor != "worker:builder" {
			t.Fatalf("event: %+v", last)
		}
	})
	t.Run("complete once, then the token is dead", func(t *testing.T) {
		done := jobsapi.CompleteRequest{
			ExitReason: "done", Result: json.RawMessage(`{"ok":true}`), SessionID: "sess-1",
			Artifacts: []jobsapi.Artifact{{Path: "patch.diff", SHA256: strings.Repeat("a", 64), Bytes: 12}},
		}
		if rec := call(t, h, http.MethodPost, "/jobs/"+a.JobID+"/complete", a.JobToken, done); rec.Code != http.StatusOK {
			t.Fatalf("status %d: %s", rec.Code, rec.Body)
		}
		if rec := call(t, h, http.MethodPost, "/jobs/"+a.JobID+"/complete", a.JobToken, done); rec.Code != http.StatusUnauthorized {
			t.Fatalf("second complete: %d", rec.Code)
		}
		arts, err := q.ListJobArtifacts(ctx, first.ID)
		if err != nil || len(arts) != 1 {
			t.Fatalf("artifacts: %+v %v", arts, err)
		}
		if _, err := q.GetHarnessSession(ctx, bookdb.GetHarnessSessionParams{TaskID: task.ID, Role: "builder"}); err != nil {
			t.Fatalf("session: %v", err)
		}
	})
	t.Run("an expired lease rejects the token", func(t *testing.T) {
		clock = clock.Add(2 * time.Minute)
		if rec := call(t, h, http.MethodPost, "/jobs/"+b.JobID+"/heartbeat", b.JobToken, jobsapi.HeartbeatRequest{}); rec.Code != http.StatusUnauthorized {
			t.Fatalf("status %d", rec.Code)
		}
	})
}
