package jobserver_test

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/ilyachakun/nightshift/internal/control/jobserver"
	"github.com/ilyachakun/nightshift/internal/data/book/bookdb"
	"github.com/ilyachakun/nightshift/internal/execution/jobsapi"
	"github.com/ilyachakun/nightshift/internal/testing/testutil/pgtest"
)

// The persistence contract of the jobs API (docs/plan/architecture/ item A7), on real Postgres:
// a lease fences its holder, a result lands with its outputs in one transaction, and a result
// is taken once however many times it is sent. The compare-and-set of a task move is
// book.TestQueries; a Verifier verdict with its state and outbox in one transaction is
// dispatch.TestVerifierVerdictRollsBackAndReplaysOnce.

// oneJob is a jobs API on a real book with one queued builder job.
type oneJob struct {
	db    *pgtest.DB
	q     *bookdb.Queries
	h     http.Handler
	task  bookdb.BookTask
	clock time.Time
}

func newOneJob(t *testing.T) *oneJob {
	t.Helper()
	f := &oneJob{db: pgtest.Start(t), clock: time.Date(2026, 11, 27, 12, 0, 0, 0, time.UTC)}
	pool := f.db.ServicePool(t)
	f.q = bookdb.New(pool)
	var err error
	if f.task, err = f.q.CreateTask(t.Context(), bookdb.CreateTaskParams{Tenant: "example-solo", Repo: "example/app", Title: "Persistence"}); err != nil {
		t.Fatal(err)
	}
	if _, err := f.q.EnqueueJob(t.Context(), bookdb.EnqueueJobParams{
		Tenant: "example-solo", TaskID: f.task.ID, Role: "builder", Provider: "claude-api",
		Payload: []byte(`{"repo":"example/app","base_sha":"abc","prompt":"do it"}`),
	}); err != nil {
		t.Fatal(err)
	}
	f.h = (&jobserver.Server{Pool: pool, WorkerTokenSHA256: jobserver.HashToken(workerToken), Lease: time.Minute, Now: func() time.Time { return f.clock }}).Handler()
	return f
}

func (f *oneJob) claim(t *testing.T, worker string) jobsapi.ClaimResponse {
	t.Helper()
	rec := call(t, f.h, "/jobs/claim", workerToken, jobsapi.ClaimRequest{WorkerID: worker})
	var resp jobsapi.ClaimResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil || rec.Code != http.StatusOK || resp.JobID == "" {
		t.Fatalf("claim by %s: %d %s %v", worker, rec.Code, rec.Body, err)
	}
	return resp
}

func (f *oneJob) job(t *testing.T, id string) bookdb.BookJob {
	t.Helper()
	n, err := strconv.ParseInt(id, 10, 64)
	if err != nil {
		t.Fatal(err)
	}
	jobs, err := f.q.ListTaskJobs(t.Context(), bookdb.ListTaskJobsParams{TaskID: f.task.ID, Role: "builder"})
	if err != nil || len(jobs) != 1 || jobs[0].ID != n {
		t.Fatalf("jobs of the task: %+v, %v; want only job %d", jobs, err, n)
	}
	return jobs[0]
}

func done(result string, artifacts ...string) jobsapi.CompleteRequest {
	req := jobsapi.CompleteRequest{ExitReason: "done", Result: json.RawMessage(result), SessionID: "sess-" + result}
	for _, p := range artifacts {
		req.Artifacts = append(req.Artifacts, jobsapi.Artifact{Path: p, SHA256: strings.Repeat("a", 64), Bytes: 1})
	}
	return req
}

// TestALeaseFencesItsHolder: a worker whose lease ran out and whose job went to another worker
// can no longer heartbeat, report or complete it; the new holder's result is the one kept.
func TestALeaseFencesItsHolder(t *testing.T) {
	f := newOneJob(t)
	first := f.claim(t, "w1")
	f.clock = f.clock.Add(2 * time.Minute) // w1's lease is over; the next claim requeues the job
	second := f.claim(t, "w2")
	if second.JobID != first.JobID || second.JobToken == first.JobToken {
		t.Fatalf("the expired job was not leased again with a new token: %+v then %+v", first, second)
	}

	base := "/jobs/" + first.JobID
	for path, body := range map[string]any{
		base + "/heartbeat": jobsapi.HeartbeatRequest{Stage: "build"},
		base + "/events":    jobsapi.EventsRequest{Events: []jobsapi.Event{{At: f.clock, Kind: "note"}}},
		base + "/complete":  done(`{"by":"w1"}`),
	} {
		if rec := call(t, f.h, path, first.JobToken, body); rec.Code != http.StatusUnauthorized {
			t.Errorf("the old holder's %s: %d %s, want 401", path, rec.Code, rec.Body)
		}
	}
	if rec := call(t, f.h, base+"/complete", second.JobToken, done(`{"by":"w2"}`)); rec.Code != http.StatusOK {
		t.Fatalf("the holder's complete: %d %s", rec.Code, rec.Body)
	}
	if j := f.job(t, first.JobID); j.State != "done" || string(j.Result) != `{"by": "w2"}` || j.Attempt != 2 {
		t.Fatalf("job after both workers: state %s, result %s, attempt %d", j.State, j.Result, j.Attempt)
	}
	if rec := call(t, f.h, base+"/complete", first.JobToken, done(`{"by":"w1"}`)); rec.Code != http.StatusUnauthorized {
		t.Errorf("the old holder completed a finished job: %d", rec.Code)
	}
}

// TestACompleteLandsWhole: when a write after the result fails, the job keeps running with no
// result and no outputs, and the same complete sent again lands once.
func TestACompleteLandsWhole(t *testing.T) {
	f := newOneJob(t)
	lease := f.claim(t, "w1")
	path := "/jobs/" + lease.JobID + "/complete"
	if _, err := f.db.Owner.ExecContext(t.Context(), `ALTER TABLE book.artifacts ADD CONSTRAINT fixture_reject CHECK (path <> 'b.diff')`); err != nil {
		t.Fatal(err)
	}
	req := done(`{"ok":true}`, "a.diff", "b.diff")
	if rec := call(t, f.h, path, lease.JobToken, req); rec.Code != http.StatusInternalServerError {
		t.Fatalf("complete with a failing write: %d %s, want 500", rec.Code, rec.Body)
	}
	n, _ := strconv.ParseInt(lease.JobID, 10, 64)
	if j := f.job(t, lease.JobID); j.State != "running" || len(j.Result) != 0 {
		t.Fatalf("a failed complete left the job %s with result %s", j.State, j.Result)
	}
	if arts, err := f.q.ListJobArtifacts(t.Context(), n); err != nil || len(arts) != 0 {
		t.Fatalf("a failed complete left artifacts %+v, %v", arts, err)
	}
	if _, err := f.q.GetHarnessSession(t.Context(), bookdb.GetHarnessSessionParams{TaskID: f.task.ID, Role: "builder"}); err == nil {
		t.Fatal("a failed complete left a harness session")
	}

	if _, err := f.db.Owner.ExecContext(t.Context(), `ALTER TABLE book.artifacts DROP CONSTRAINT fixture_reject`); err != nil {
		t.Fatal(err)
	}
	// The worker retries; two copies race and exactly one lands.
	codes := make(chan int, 2)
	var wg sync.WaitGroup
	for range 2 {
		wg.Go(func() { codes <- call(t, f.h, path, lease.JobToken, req).Code })
	}
	wg.Wait()
	close(codes)
	got := map[int]int{}
	for c := range codes {
		got[c]++
	}
	if got[http.StatusOK] != 1 || got[http.StatusUnauthorized] != 1 {
		t.Fatalf("two racing completes answered %v, want one 200 and one 401", got)
	}
	if arts, err := f.q.ListJobArtifacts(t.Context(), n); err != nil || len(arts) != 2 {
		t.Fatalf("artifacts after the retry: %+v, %v", arts, err)
	}
	if j := f.job(t, lease.JobID); j.State != "done" {
		t.Fatalf("job after the retry: %s", j.State)
	}
}
