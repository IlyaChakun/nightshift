package dispatch_test

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/ilyachakun/nightshift/internal/adapters/github/fake"
	"github.com/ilyachakun/nightshift/internal/book/bookdb"
	"github.com/ilyachakun/nightshift/internal/config"
	"github.com/ilyachakun/nightshift/internal/jobsapi"
	"github.com/ilyachakun/nightshift/internal/testutil/pgtest"
)

func TestFindingHandoffNeedsAComparableReview(t *testing.T) {
	t.Run("confirmed", func(t *testing.T) {
		checkFindingRecheck(t, `{"verdict":"pass","summary":"Rechecked every listed finding.\nThe export includes all rows."}`, "fixed")
	})
	t.Run("missing_confirmation", func(t *testing.T) {
		checkFindingRecheck(t, `{"verdict":"pass","summary":"The change meets the spec. I could not recheck the missing row."}`, "handed_off")
	})
}

func checkFindingRecheck(t *testing.T, pass, status string) {
	t.Helper()
	r := newRig(t)
	r.withGate(config.Review{Mode: "all"}, "solo")
	r.withVerifier()
	r.toReview("feature.txt")
	r.advance()
	const returned = `{"verdict":"return","summary":"missing row","findings":[
		{"rule_id":"csv.rows","category":"code","severity":"block","layer_caught":"L5",
		 "message":"the export drops a row","fingerprint":"csv:missing-row"}]}`
	r.verdict(returned)
	r.advance()
	checkStatuses := func(want ...string) {
		t.Helper()
		found, err := r.q.ListTaskFindings(t.Context(), r.task.ID)
		if err != nil || len(found) != len(want) {
			t.Fatalf("findings = %+v, err = %v, want statuses %v", found, err, want)
		}
		for i, f := range found {
			if f.Status != want[i] {
				t.Fatalf("finding %d status = %q, want %q", i, f.Status, want[i])
			}
		}
	}
	advanceToReview := func(text string) {
		t.Helper()
		r.finish(r.patchFor(text+".txt", "fix\n"))
		r.beforeGreenReview()
	}
	checkStatuses("open")
	r.advance()
	checkStatuses("handed_off")
	advanceToReview("second")
	checkStatuses("handed_off") // Builder and CI cannot resolve a Verifier finding.
	r.verdict(returned)
	r.advance()
	checkStatuses("handed_off", "open")
	r.advance()
	checkStatuses("handed_off", "handed_off")
	if got := strings.Count(string(r.lastJob().Payload), "the export drops a row"); got != 1 {
		t.Fatalf("repeated finding appears %d times in the next Builder prompt", got)
	}
	advanceToReview("third")
	var review jobsapi.Job
	if err := json.Unmarshal(r.verifierJob().Payload, &review); err != nil {
		t.Fatal(err)
	}
	if len(review.ReviewFindingIDs) != 2 || !strings.Contains(review.Prompt, "the export drops a row") ||
		!strings.Contains(review.Prompt, "on the current head before returning pass") {
		t.Fatalf("Verifier did not receive the prior findings: %+v", review)
	}
	r.verdict(pass)
	r.advance()
	checkStatuses(status, status)
}

func TestBuilderReceivesDistinctFindings(t *testing.T) {
	r := newRig(t)
	r.advance()
	first, second := red("the export drops a row"), red("the header has the wrong label")
	first.Fingerprint, second.Fingerprint = "", ""
	third, fourth := red("a row is too short"), red("a row is too long")
	third.Fingerprint, fourth.Fingerprint = "shared", "shared"
	fourth.RuleID = "verify.extra"
	r.finish(r.patchFor("broken.txt", "broken\n"), first, second, third, fourth)
	r.advance()
	payload := string(r.lastJob().Payload)
	for _, f := range []bookdb.AddFindingParams{first, second, third, fourth} {
		if !strings.Contains(payload, f.Message) {
			t.Fatalf("Builder missed %q: %s", f.Message, payload)
		}
	}
}

func TestFailedFindingHandoffLeavesNoReplacementJob(t *testing.T) {
	db := pgtest.Start(t)
	r := newRigOnBook(t, db)
	r.advance()
	r.finish(r.patchFor("broken.txt", "broken\n"), red("verify failed"))
	execVerdictFixture(t, db, `ALTER TABLE book.findings ADD CONSTRAINT fixture_reject_handoff CHECK (handoff_job_id IS NULL)`)
	if err := r.c.Advance(t.Context(), r.tenant, r.task); err == nil {
		t.Fatal("expected handoff transaction to fail")
	}
	check := func(status string, jobCount int) {
		t.Helper()
		found, err := r.q.ListTaskFindings(t.Context(), r.task.ID)
		if err != nil || len(found) != 1 || found[0].Status != status {
			t.Fatalf("findings = %+v, err = %v, want %s", found, err, status)
		}
		jobs, err := r.q.ListTaskJobs(t.Context(), bookdb.ListTaskJobsParams{TaskID: r.task.ID, Role: "builder"})
		if err != nil || len(jobs) != jobCount {
			t.Fatalf("Builder jobs = %d, err = %v, want %d", len(jobs), err, jobCount)
		}
	}
	check("open", 1)
	execVerdictFixture(t, db, "ALTER TABLE book.findings DROP CONSTRAINT fixture_reject_handoff")
	r.advance()
	check("handed_off", 2)
}

func (r *rig) beforeGreenReview() {
	r.t.Helper()
	r.advance() // push
	r.twin.SetChecks("example/app", r.task.HeadSha.String, fake.Check{Name: "test", Status: "completed", Conclusion: "success"})
	r.advance() // checks -> review
	r.advance() // enqueue Verifier
}
