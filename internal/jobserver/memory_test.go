package jobserver_test

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/ilyachakun/nightshift/internal/book/bookdb"
	"github.com/ilyachakun/nightshift/internal/jobsapi"
	"github.com/ilyachakun/nightshift/internal/jobserver"
	"github.com/ilyachakun/nightshift/internal/testutil/pgtest"
)

// TestCompleteStoresFindingsAndNotes checks that a role result's findings and notes reach
// the book with the job's provenance, notes as proposed, and that an invalid element is
// dropped without failing the job (T3).
func TestCompleteStoresFindingsAndNotes(t *testing.T) {
	pool := pgtest.Start(t).ServicePool(t)
	q := bookdb.New(pool)
	task, err := q.CreateTask(t.Context(), bookdb.CreateTaskParams{Tenant: "example-solo", Repo: "example/app", Title: "Memory"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := q.EnqueueJob(t.Context(), bookdb.EnqueueJobParams{
		Tenant: "example-solo", TaskID: task.ID, Role: "verifier", Provider: "claude-api", Payload: []byte(`{}`),
	}); err != nil {
		t.Fatal(err)
	}
	h := (&jobserver.Server{Pool: pool, WorkerTokenSHA256: jobserver.HashToken(workerToken)}).Handler()
	rec := call(t, h, "/jobs/claim", workerToken, jobsapi.ClaimRequest{WorkerID: "w1"})
	var claim jobsapi.ClaimResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &claim); err != nil || claim.JobID == "" {
		t.Fatalf("claim: %s %v", rec.Body, err)
	}
	result := `{"verdict":"return","summary":"s","findings":[
	  {"rule_id":"gosec.g101","category":"security","severity":"block","layer_caught":"L5","message":"hardcoded credential",
	   "location":{"path":"a/b.go","start_line":7.0,"end_line":2147483648},"fingerprint":"abc"},
	  {"rule_id":"x","category":"code","severity":"catastrophic","layer_caught":"L5","message":"bad severity"}],
	 "notes":[{"kind":"gotcha","text":"tests need -race","source":"Makefile"},{"kind":"secret_plan","text":"x","source":"y"}]}`
	done := jobsapi.CompleteRequest{ExitReason: "done", Result: json.RawMessage(result), Artifacts: []jobsapi.Artifact{}}
	if rec := call(t, h, "/jobs/"+claim.JobID+"/complete", claim.JobToken, done); rec.Code != http.StatusOK {
		t.Fatalf("complete: %d %s", rec.Code, rec.Body)
	}

	findings, err := q.ListTaskFindings(t.Context(), task.ID)
	if err != nil || len(findings) != 1 || findings[0].RuleID != "gosec.g101" || findings[0].Path.String != "a/b.go" ||
		!findings[0].StartLine.Valid || findings[0].StartLine.Int32 != 7 || findings[0].EndLine.Valid ||
		findings[0].Status != "open" || !findings[0].JobID.Valid {
		t.Fatalf("findings: %+v, %v", findings, err)
	}
	notes, err := q.ListNotes(t.Context(), bookdb.ListNotesParams{Tenant: "example-solo", Status: "proposed"})
	if err != nil || len(notes) != 1 || notes[0].Role != "verifier" || notes[0].Kind != "gotcha" || notes[0].TaskID != task.ID {
		t.Fatalf("notes: %+v, %v", notes, err)
	}
}
