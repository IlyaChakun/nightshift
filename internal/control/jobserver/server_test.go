package jobserver_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/ilyachakun/nightshift/internal/control/jobserver"
	"github.com/ilyachakun/nightshift/internal/data/book/bookdb"
	"github.com/ilyachakun/nightshift/internal/execution/jobsapi"
	"github.com/ilyachakun/nightshift/internal/testutil/pgtest"
)

const workerToken = "worker-secret-for-tests"

func call(t *testing.T, h http.Handler, path, token string, body any) *httptest.ResponseRecorder {
	t.Helper()
	b, err := json.Marshal(body)
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequestWithContext(t.Context(), http.MethodPost, path, bytes.NewReader(b))
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
	h := (&jobserver.Server{WorkerTokenSHA256: jobserver.HashToken(workerToken)}).Handler()
	for name, tc := range map[string]struct{ path, token string }{
		"claim without a token":     {"/jobs/claim", ""},
		"claim with a wrong token":  {"/jobs/claim", "nope"},
		"heartbeat without a token": {"/jobs/1/heartbeat", ""},
		"complete without a token":  {"/jobs/1/complete", ""},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			if rec := call(t, h, tc.path, tc.token, map[string]any{"worker_id": "w1", "exit_reason": "done"}); rec.Code != http.StatusUnauthorized {
				t.Fatalf("status %d: %s", rec.Code, rec.Body)
			}
		})
	}
}

// lifecycle is a jobs API on a real book with two claimed jobs.
type lifecycle struct {
	h     http.Handler
	q     *bookdb.Queries
	task  bookdb.BookTask
	clock time.Time
	a, b  jobsapi.ClaimResponse
}

func (f *lifecycle) claim(t *testing.T) jobsapi.ClaimResponse {
	t.Helper()
	rec := call(t, f.h, "/jobs/claim", workerToken, jobsapi.ClaimRequest{WorkerID: "w1"})
	var resp jobsapi.ClaimResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil || rec.Code != http.StatusOK {
		t.Fatalf("claim: %d %s %v", rec.Code, rec.Body, err)
	}
	return resp
}

// TestJobLifecycle runs claim, heartbeat, events and complete against a real book (T3).
func TestJobLifecycle(t *testing.T) {
	db := pgtest.Start(t)
	pool := db.ServicePool(t)
	f := &lifecycle{q: bookdb.New(pool), clock: time.Date(2026, 11, 24, 12, 0, 0, 0, time.UTC)}
	var err error
	f.task, err = f.q.CreateTask(t.Context(), bookdb.CreateTaskParams{Tenant: "example-solo", Repo: "example/app", Title: "Jobs API"})
	if err != nil {
		t.Fatal(err)
	}
	for range 2 {
		if _, err := f.q.EnqueueJob(t.Context(), bookdb.EnqueueJobParams{
			Tenant: "example-solo", TaskID: f.task.ID, Role: "builder", Provider: "claude-api",
			Payload: []byte(`{"repo":"example/app","base_sha":"abc","prompt":"do it"}`),
		}); err != nil {
			t.Fatal(err)
		}
	}
	s := &jobserver.Server{Pool: pool, WorkerTokenSHA256: jobserver.HashToken(workerToken), Lease: time.Minute, Now: func() time.Time { return f.clock }}
	f.h = s.Handler()

	f.a, f.b = f.claim(t), f.claim(t)
	if f.a.JobToken == "" || f.a.Job.Role != "builder" || f.a.Job.Prompt != "do it" || f.a.Job.TaskID == "" {
		t.Fatalf("claim: %+v", f.a)
	}
	if f.b.JobID == f.a.JobID || f.b.JobToken == f.a.JobToken {
		t.Fatalf("two claims got the same job: %+v %+v", f.a, f.b)
	}
	if got := f.claim(t); got.JobID != "" {
		t.Fatalf("an empty queue returned %+v", got)
	}
	t.Run("a token works only for its own job", f.tokenScope)
	t.Run("events are prefixed and attributed to the worker", f.events)
	t.Run("complete once, then the token is dead", f.completeOnce)
	t.Run("an expired lease rejects the token", f.expired)
}

func (f *lifecycle) tokenScope(t *testing.T) {
	if rec := call(t, f.h, "/jobs/"+f.b.JobID+"/heartbeat", f.a.JobToken, jobsapi.HeartbeatRequest{}); rec.Code != http.StatusUnauthorized {
		t.Fatalf("status %d", rec.Code)
	}
	if rec := call(t, f.h, "/jobs/"+f.a.JobID+"/heartbeat", workerToken, jobsapi.HeartbeatRequest{}); rec.Code != http.StatusUnauthorized {
		t.Fatalf("the worker token worked as a job token: %d", rec.Code)
	}
}

func (f *lifecycle) events(t *testing.T) {
	ev := jobsapi.EventsRequest{Events: []jobsapi.Event{{At: f.clock, Kind: "decision.approved"}}}
	if rec := call(t, f.h, "/jobs/"+f.a.JobID+"/events", f.a.JobToken, ev); rec.Code != http.StatusOK {
		t.Fatalf("status %d: %s", rec.Code, rec.Body)
	}
	events, err := f.q.ListTaskEvents(t.Context(), pgtype.Int8{Int64: f.task.ID, Valid: true})
	if err != nil {
		t.Fatal(err)
	}
	last := events[len(events)-1]
	if last.Kind != "worker.decision.approved" || last.Actor != "worker:builder" {
		t.Fatalf("event: %+v", last)
	}
}

func (f *lifecycle) completeOnce(t *testing.T) {
	done := jobsapi.CompleteRequest{
		ExitReason: "done", Result: json.RawMessage(`{"ok":true}`), SessionID: "sess-1",
		Artifacts: []jobsapi.Artifact{{Path: "patch.diff", SHA256: strings.Repeat("a", 64), Bytes: 12}},
	}
	path := "/jobs/" + f.a.JobID + "/complete"
	if rec := call(t, f.h, path, f.a.JobToken, done); rec.Code != http.StatusOK {
		t.Fatalf("status %d: %s", rec.Code, rec.Body)
	}
	if rec := call(t, f.h, path, f.a.JobToken, done); rec.Code != http.StatusUnauthorized {
		t.Fatalf("second complete: %d", rec.Code)
	}
	id, err := strconv.ParseInt(f.a.JobID, 10, 64)
	if err != nil {
		t.Fatal(err)
	}
	if arts, err := f.q.ListJobArtifacts(t.Context(), id); err != nil || len(arts) != 1 {
		t.Fatalf("artifacts: %+v %v", arts, err)
	}
	if _, err := f.q.GetHarnessSession(t.Context(), bookdb.GetHarnessSessionParams{TaskID: f.task.ID, Role: "builder"}); err != nil {
		t.Fatalf("session: %v", err)
	}
}

func (f *lifecycle) expired(t *testing.T) {
	f.clock = f.clock.Add(2 * time.Minute)
	if rec := call(t, f.h, "/jobs/"+f.b.JobID+"/heartbeat", f.b.JobToken, jobsapi.HeartbeatRequest{}); rec.Code != http.StatusUnauthorized {
		t.Fatalf("status %d", rec.Code)
	}
}

// TestClaimHidesScenariosFromBuilder is the T6/T3 check that a Builder token never sees
// hidden scenarios while the Verifier's job carries them.
func TestClaimHidesScenariosFromBuilder(t *testing.T) {
	db := pgtest.Start(t)
	pool := db.ServicePool(t)
	q := bookdb.New(pool)
	task, err := q.CreateTask(t.Context(), bookdb.CreateTaskParams{Tenant: "example-solo", Repo: "example/app", Title: "Hidden"})
	if err != nil {
		t.Fatal(err)
	}
	for _, role := range []string{"builder", "verifier"} {
		payload := `{"inputs":{"spec":"s","hidden_scenarios":["secret"]}}`
		if _, err := q.EnqueueJob(t.Context(), bookdb.EnqueueJobParams{
			Tenant: "example-solo", TaskID: task.ID, Role: role, Provider: "claude-api", Payload: []byte(payload),
		}); err != nil {
			t.Fatal(err)
		}
	}
	h := (&jobserver.Server{Pool: pool, WorkerTokenSHA256: jobserver.HashToken(workerToken)}).Handler()
	for _, tc := range []struct {
		role       string
		wantHidden bool
	}{{"builder", false}, {"verifier", true}} {
		rec := call(t, h, "/jobs/claim", workerToken, jobsapi.ClaimRequest{WorkerID: "w1"})
		var resp jobsapi.ClaimResponse
		if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil || resp.Job.Role != tc.role {
			t.Fatalf("claim %s: %s %v", tc.role, rec.Body, err)
		}
		if got := strings.Contains(string(resp.Job.Inputs), "hidden_scenarios"); got != tc.wantHidden {
			t.Errorf("%s sees hidden scenarios: %v (inputs %s)", tc.role, got, resp.Job.Inputs)
		}
		if !strings.Contains(string(resp.Job.Inputs), `"spec"`) {
			t.Errorf("%s lost its other inputs: %s", tc.role, resp.Job.Inputs)
		}
	}
}
