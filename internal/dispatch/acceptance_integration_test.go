package dispatch_test

import (
	"encoding/json"
	"strconv"
	"strings"
	"testing"

	"github.com/ilyachakun/nightshift/internal/customization/config"
	"github.com/ilyachakun/nightshift/internal/data/book/bookdb"
	"github.com/ilyachakun/nightshift/internal/knowledge/search"
)

const hiddenStep = "the export holds exactly 10001 rows"

// toAcceptance takes a task with hidden scenarios, a Verifier and a preview through a passing
// review to its queued acceptance job.
func (r *rig) toAcceptance() {
	r.t.Helper()
	r.toAcceptanceUnder(config.Review{Mode: "all"}, "feature.txt")
}

func (r *rig) toAcceptanceUnder(review config.Review, path string) {
	r.t.Helper()
	r.withGate(review, "solo")
	r.withPreview()
	r.toReview(path)
	if _, err := (&search.Index{Pool: r.c.Pool}).Add(r.t.Context(), search.Document{
		Tenant: r.task.Tenant, Kind: "hidden_scenarios", Source: "task:" + strconv.FormatInt(r.task.ID, 10),
		Title: "hidden", Body: `[{"id":"HS-1","given":"a report","when":"the owner exports it","then":"` + hiddenStep + `"}]`, Visibility: "lead_verifier",
	}); err != nil {
		r.t.Fatal(err)
	}
	r.passReview() // review passes: preview
	r.advance()    // preview up; acceptance job queued
	if r.task.State != "acceptance" {
		r.t.Fatalf("the task is in %s, want acceptance", r.task.State)
	}
}

// T4: the Verifier's acceptance runs the hidden scenarios against the preview before the task
// reaches a person (design section 6, plan P5a).
func TestAcceptanceRunsTheHiddenScenariosOnThePreview(t *testing.T) {
	r := newRig(t)
	r.withVerifier()
	r.toAcceptance()
	job := r.verifierJob()
	var payload struct {
		Inputs struct {
			Mode string `json:"mode"`
			URL  string `json:"preview_url"`
		} `json:"inputs"`
		Prompt string `json:"prompt"`
	}
	if err := json.Unmarshal(job.Payload, &payload); err != nil {
		t.Fatal(err)
	}
	if payload.Inputs.Mode != "acceptance" || !strings.HasSuffix(payload.Inputs.URL, ".preview.invalid") ||
		!strings.Contains(payload.Prompt, payload.Inputs.URL) || !strings.Contains(payload.Prompt, hiddenStep) {
		t.Fatalf("acceptance job: %s", job.Payload)
	}
	r.advance() // still running: nothing moves
	if r.task.State != "acceptance" {
		t.Fatalf("the task is in %s while acceptance runs", r.task.State)
	}
	r.verdict(`{"verdict":"pass","summary":"HS-1 passed"}`)
	r.advance()
	if r.task.State != "human_review" {
		t.Fatalf("the task is in %s after a passing acceptance, want human_review", r.task.State)
	}
}

func TestAFailedAcceptanceSendsBehaviorBackToBuilder(t *testing.T) {
	r := newRig(t)
	r.withVerifier()
	r.toAcceptance()
	r.verdict(`{"verdict":"return","summary":"export is short","findings":[{"rule_id":"new","category":"code","severity":"block","layer_caught":"L5","message":"the exported report misses its last row","fingerprint":"acceptance:last-row"}]}`)
	r.advance() // acceptance -> building with the finding
	if r.task.State != "building" {
		t.Fatalf("the task is in %s after a failed acceptance, want building", r.task.State)
	}
	found, err := r.q.ListTaskFindings(t.Context(), r.task.ID)
	if err != nil || len(found) != 1 || !strings.Contains(found[0].Message, "last row") {
		t.Fatalf("findings = %+v, %v", found, err)
	}
	r.advance() // building: Builder is queued
	jobs, err := r.q.ListTaskJobs(t.Context(), bookdb.ListTaskJobsParams{TaskID: r.task.ID, Role: "builder"})
	if err != nil || len(jobs) < 2 {
		t.Fatalf("builder jobs: %d, %v", len(jobs), err)
	}
	if last := string(jobs[len(jobs)-1].Payload); strings.Contains(last, hiddenStep) || strings.Contains(last, "hidden_scenarios") {
		t.Fatalf("Builder's job shows the hidden scenario: %s", last)
	}
}

// A task without hidden scenarios keeps the path it had: no acceptance job.
func TestNoHiddenScenariosMeansNoAcceptanceJob(t *testing.T) {
	r := newRig(t)
	r.withGate(config.Review{Mode: "all"}, "solo")
	r.withPreview()
	r.toReview("feature.txt")
	r.passReview()
	r.advance()
	if r.task.State != "human_review" {
		t.Fatalf("the task is in %s, want human_review", r.task.State)
	}
	jobs, err := r.q.ListTaskJobs(t.Context(), bookdb.ListTaskJobsParams{TaskID: r.task.ID, Role: "verifier"})
	if err != nil || len(jobs) != 1 {
		t.Fatalf("verifier jobs: %d, %v; want the review only", len(jobs), err)
	}
}

// Review finding: a pass that raises the blast radius must not go to ready on a needsHuman that was
// read before the verdict.
func TestAnAcceptanceThatRaisesTheRadiusReachesAPerson(t *testing.T) {
	r := newRig(t)
	r.withVerifier()
	r.toAcceptanceUnder(config.Review{Mode: "blast_radius", Actions: map[string]string{"low": "auto"}}, "docs/guides/x.md")
	r.verdict(`{"verdict":"pass","summary":"HS-1 passed","raise_blast_radius":{"level":"high","reason":"changes access"}}`)
	r.advance()
	if r.task.State != "human_review" {
		t.Fatalf("the task is in %s after a pass that raised the radius, want human_review", r.task.State)
	}
}

// The control of the test above: the same pass without a raised radius goes on without a person.
func TestAPlainAcceptancePassGoesOnAlone(t *testing.T) {
	r := newRig(t)
	r.withVerifier()
	r.toAcceptanceUnder(config.Review{Mode: "blast_radius", Actions: map[string]string{"low": "auto"}}, "docs/guides/x.md")
	r.verdict(`{"verdict":"pass","summary":"HS-1 passed"}`)
	r.advance()
	if r.task.State == "human_review" || r.task.State == "acceptance" {
		t.Fatalf("the task is in %s after a plain pass under auto review", r.task.State)
	}
}

// Review finding: a failed acceptance counts as a return, and a finding that quotes a hidden
// scenario reaches Builder withheld; the next review does not recheck acceptance findings.
func TestAFailedAcceptanceCountsAndHidesQuotes(t *testing.T) {
	r := newRig(t)
	r.withVerifier()
	r.toAcceptance()
	r.verdict(`{"verdict":"return","summary":"short","findings":[{"rule_id":"new","category":"code","severity":"block","layer_caught":"L5","message":"HS-1: then ` + hiddenStep + ` failed","fingerprint":"acceptance:quote"}]}`)
	r.advance()
	if r.task.State != "building" || r.task.Returns != 1 {
		t.Fatalf("task in %s with %d returns, want building with 1", r.task.State, r.task.Returns)
	}
	found, err := r.q.ListTaskFindings(t.Context(), r.task.ID)
	if err != nil || len(found) != 1 || strings.Contains(found[0].Message, hiddenStep) {
		t.Fatalf("findings = %+v, %v; want the quote withheld", found, err)
	}
	r.advance() // Builder is queued with the finding
	r.finish(r.patchFor("fix.txt", "fix\n"))
	r.beforeGreenReview()
	if p := string(r.verifierJob().Payload); !strings.Contains(p, `"mode": "review"`) || strings.Contains(p, "Recheck") {
		t.Fatalf("the next review rechecks acceptance findings: %s", p)
	}
}
