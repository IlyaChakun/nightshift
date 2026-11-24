package dispatch_test

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/ilyachakun/nightshift/internal/adapters/github"
	"github.com/ilyachakun/nightshift/internal/adapters/github/fake"
	"github.com/ilyachakun/nightshift/internal/book/bookdb"
	"github.com/ilyachakun/nightshift/internal/config"
	"github.com/ilyachakun/nightshift/internal/dispatch"
	"github.com/ilyachakun/nightshift/internal/flow"
	"github.com/ilyachakun/nightshift/internal/gitops"
	"github.com/ilyachakun/nightshift/internal/policy"
	"github.com/ilyachakun/nightshift/internal/testutil/pgtest"
)

// rig is a conductor on a real book, a local upstream repository and the GitHub twin. The
// test plays the worker by completing jobs straight in the book.
type rig struct {
	t         *testing.T
	c         *dispatch.Conductor
	q         *bookdb.Queries
	tenant    config.Tenant
	up        string
	artifacts string
	twin      *fake.Server
	task      bookdb.BookTask
}

func gitIn(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.CommandContext(t.Context(), "git", args...) //nolint:gosec // test helper with fixed arguments
	cmd.Dir = dir
	cmd.Env = []string{
		"PATH=" + os.Getenv("PATH"), //nolint:forbidigo // git must be found
		"HOME=/nonexistent", "GIT_CONFIG_GLOBAL=/dev/null",
		"GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@example.test", "GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@example.test",
	}
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v: %s", args, err, out)
	}
	return strings.TrimSpace(string(out))
}

func newRig(t *testing.T) *rig {
	t.Helper()
	pool := pgtest.Start(t).ServicePool(t)
	up := t.TempDir()
	gitIn(t, up, "init", "-q", "-b", "main")
	if err := os.WriteFile(filepath.Join(up, "a.txt"), []byte("one\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	gitIn(t, up, "add", ".")
	gitIn(t, up, "commit", "-q", "-m", "init")

	cons, err := policy.NewConstitution(nil)
	if err != nil {
		t.Fatal(err)
	}
	twin := fake.New(t, "tok")
	q := bookdb.New(pool)
	r := &rig{t: t, q: q, up: up, artifacts: t.TempDir(), twin: twin}
	r.c = &dispatch.Conductor{
		Pool: pool, Mover: &flow.Mover{Pool: pool},
		Router: &dispatch.Router{Pool: pool, Providers: map[string]config.Provider{"fake": {Harness: "fake"}}, Chains: map[string][]string{"builder": {"fake"}}},
		Roles:  map[string]config.Role{"builder": {Name: "builder", OutputSchema: "schemas/builder-result.json"}},
		Git:    &gitops.Git{Dir: t.TempDir(), Guard: cons.Check}, Forge: &github.Client{BaseURL: twin.URL, Token: "tok"},
		Remote: func(string) string { return up }, ArtifactDir: r.artifacts,
		Identity: gitops.Identity{Name: "nightshift", Email: "ns@example.test"},
	}
	r.tenant = config.Tenant{Name: "example-solo", Governance: "solo"}
	r.task, err = q.CreateTask(t.Context(), bookdb.CreateTaskParams{Tenant: "example-solo", Repo: "example/app", Title: "Add a feature"})
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range []flow.Event{flow.LeadStarted, flow.DocsDrafted, flow.DocLoopPassed, flow.Approve} {
		if _, err := r.c.Mover.Apply(t.Context(), flow.Move{TaskID: r.task.ID, Event: e, Actor: "owner", Profile: flow.Solo}); err != nil {
			t.Fatal(err)
		}
	}
	r.reload()
	return r
}

func (r *rig) reload() {
	r.t.Helper()
	var err error
	if r.task, err = r.q.GetTask(r.t.Context(), r.task.ID); err != nil {
		r.t.Fatal(err)
	}
}

func (r *rig) advance() {
	r.t.Helper()
	if err := r.c.Advance(r.t.Context(), r.tenant, r.task); err != nil {
		r.t.Fatal(err)
	}
	r.reload()
}

// lastJob is the newest Builder job.
func (r *rig) lastJob() bookdb.BookJob {
	r.t.Helper()
	jobs, err := r.q.ListTaskJobs(r.t.Context(), bookdb.ListTaskJobsParams{TaskID: r.task.ID, Role: "builder"})
	if err != nil || len(jobs) == 0 {
		r.t.Fatalf("jobs: %v %v", jobs, err)
	}
	return jobs[len(jobs)-1]
}

// patchFor makes a patch that writes a file into the upstream's main.
func (r *rig) patchFor(name, body string) []byte {
	r.t.Helper()
	dest := filepath.Join(r.t.TempDir(), "w")
	gitIn(r.t, r.t.TempDir(), "clone", "-q", r.up, dest)
	if err := os.MkdirAll(filepath.Dir(filepath.Join(dest, name)), 0o750); err != nil {
		r.t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dest, name), []byte(body), 0o600); err != nil {
		r.t.Fatal(err)
	}
	gitIn(r.t, dest, "add", "-A")
	return []byte(gitIn(r.t, dest, "diff", "--cached") + "\n")
}

// finish plays the worker: the newest job ends with a result, a patch and findings.
func (r *rig) finish(patch []byte, findings ...bookdb.AddFindingParams) {
	r.t.Helper()
	ctx := r.t.Context()
	job := r.lastJob()
	if _, err := r.c.Pool.Exec(ctx, `UPDATE book.jobs SET state = 'done', exit_reason = 'done', result = $2, finished_at = now() WHERE id = $1`, job.ID, []byte(done)); err != nil {
		r.t.Fatal(err)
	}
	if patch != nil {
		dir := filepath.Join(r.artifacts, strconv.FormatInt(job.ID, 10))
		if err := os.MkdirAll(dir, 0o750); err != nil {
			r.t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "patch.diff"), patch, 0o600); err != nil {
			r.t.Fatal(err)
		}
		sum := sha256.Sum256(patch)
		if err := r.q.AddArtifact(ctx, bookdb.AddArtifactParams{JobID: job.ID, Path: "patch.diff", Sha256: hex.EncodeToString(sum[:]), Bytes: int64(len(patch))}); err != nil {
			r.t.Fatal(err)
		}
	}
	for _, f := range findings {
		f.Tenant, f.TaskID, f.JobID = r.task.Tenant, r.task.ID, pgtype.Int8{Int64: job.ID, Valid: true}
		if _, err := r.q.AddFinding(ctx, f); err != nil {
			r.t.Fatal(err)
		}
	}
}

const done = `{"status":"done","summary":"Adds the feature.","changed_files":["feature.txt"]}`

func red(msg string) bookdb.AddFindingParams {
	return bookdb.AddFindingParams{RuleID: "verify.failed", Category: "code", Severity: "block", LayerCaught: "L1", Message: msg, Fingerprint: "f:" + msg}
}

func TestBuilderPatchBecomesAPullRequestAndCIDecides(t *testing.T) {
	r := newRig(t)
	r.advance() // queued -> building, job queued
	if r.task.State != "building" || r.lastJob().State != "queued" {
		t.Fatalf("after the first tick: %s, job %s", r.task.State, r.lastJob().State)
	}
	r.advance() // the job has not finished: nothing happens
	if jobs, _ := r.q.ListTaskJobs(t.Context(), bookdb.ListTaskJobsParams{TaskID: r.task.ID, Role: "builder"}); len(jobs) != 1 {
		t.Fatalf("a waiting task enqueued again: %d jobs", len(jobs))
	}

	r.finish(r.patchFor("feature.txt", "done\n"))
	r.advance()
	if r.task.State != "checks" || r.task.PrNumber.Int32 != 1 || !r.task.HeadSha.Valid {
		t.Fatalf("after the patch: %s pr %v head %v", r.task.State, r.task.PrNumber, r.task.HeadSha)
	}
	pr, _ := r.twin.PR("example/app", 1)
	if !pr.Draft || pr.Head != "ns/"+strconv.FormatInt(r.task.ID, 10) {
		t.Fatalf("pull request: %+v", pr)
	}
	if got := gitIn(t, r.up, "rev-parse", "refs/heads/"+pr.Head); got != r.task.HeadSha.String {
		t.Fatalf("pushed %s, book head %s", got, r.task.HeadSha.String)
	}
	if !strings.Contains(gitIn(t, r.up, "show", "--stat", pr.Head), "feature.txt") {
		t.Fatal("the pushed commit lacks the change")
	}
	r.advance() // CI has not reported
	if r.task.State != "checks" {
		t.Fatalf("pending CI moved the task to %s", r.task.State)
	}

	// Red CI goes back to Builder as a finding, with the failing check in the prompt.
	r.twin.SetChecks("example/app", r.task.HeadSha.String, fake.Check{Name: "test", Status: "completed", Conclusion: "failure"})
	r.advance()
	if r.task.State != "building" || !strings.Contains(string(r.lastJob().Payload), "CI failed: test") {
		t.Fatalf("after red CI: %s, payload %s", r.task.State, r.lastJob().Payload)
	}
	// The next patch updates the same pull request; green CI moves the task to review.
	r.finish(r.patchFor("feature.txt", "fixed\n"))
	r.advance()
	r.twin.SetChecks("example/app", r.task.HeadSha.String, fake.Check{Name: "test", Status: "completed", Conclusion: "success"})
	r.advance()
	if r.task.State != "review" || r.task.PrNumber.Int32 != 1 {
		t.Fatalf("after green CI: %s pr %v", r.task.State, r.task.PrNumber)
	}
}

func TestChecksReturnTwiceThenEscalate(t *testing.T) {
	r := newRig(t)
	r.advance()
	for round := 1; round <= 3; round++ {
		r.finish(r.patchFor("feature.txt", "x\n"), red("verify failed in round "+strconv.Itoa(round)))
		r.advance()
		if round < 3 {
			if r.task.State != "building" || r.lastJob().State != "queued" || !strings.Contains(string(r.lastJob().Payload), "verify failed in round "+strconv.Itoa(round)) {
				t.Fatalf("round %d: %s, payload %s", round, r.task.State, r.lastJob().Payload)
			}
		}
	}
	if r.task.State != "escalated" {
		t.Fatalf("the third red result left the task in %s", r.task.State)
	}
	if pr, ok := r.twin.PR("example/app", 1); ok {
		t.Fatalf("a pull request was opened for red checks: %+v", pr)
	}
}

func TestAProtectedPathPatchIsSentBackNotPushed(t *testing.T) {
	r := newRig(t)
	r.advance()
	r.finish(r.patchFor(".github/workflows/x.yml", "on: push\n"))
	r.advance()
	if r.task.State != "building" || !strings.Contains(string(r.lastJob().Payload), "constitution.protected") {
		t.Fatalf("state %s, payload %s", r.task.State, r.lastJob().Payload)
	}
	if out, err := exec.CommandContext(t.Context(), "git", "-C", r.up, "rev-parse", "--verify", "-q", "refs/heads/ns/"+strconv.FormatInt(r.task.ID, 10)).CombinedOutput(); err == nil { //nolint:gosec // test with a temp directory
		t.Fatalf("the branch was pushed: %s", out)
	}
}

func TestATamperedPatchAndABlockedBuilderEscalate(t *testing.T) {
	r := newRig(t)
	r.advance()
	patch := r.patchFor("feature.txt", "done\n")
	r.finish(patch)
	// The worker reported one hash; the file on the volume changed afterwards.
	if err := os.WriteFile(filepath.Join(r.artifacts, strconv.FormatInt(r.lastJob().ID, 10), "patch.diff"), []byte("evil"), 0o600); err != nil {
		t.Fatal(err)
	}
	r.advance()
	if r.task.State != "escalated" {
		t.Fatalf("a tampered patch left the task in %s", r.task.State)
	}
}
