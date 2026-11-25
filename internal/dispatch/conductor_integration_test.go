package dispatch_test

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/ilyachakun/nightshift/internal/adapters/github"
	"github.com/ilyachakun/nightshift/internal/adapters/github/fake"
	"github.com/ilyachakun/nightshift/internal/book/bookdb"
	"github.com/ilyachakun/nightshift/internal/config"
	"github.com/ilyachakun/nightshift/internal/dispatch"
	"github.com/ilyachakun/nightshift/internal/flow"
	"github.com/ilyachakun/nightshift/internal/gitops"
	"github.com/ilyachakun/nightshift/internal/jobsapi"
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
		Git:    &gitops.Git{Dir: t.TempDir(), Guard: func(_ context.Context, _, _ string, paths []string) error { return cons.Check(paths) }}, Forge: &github.Client{BaseURL: twin.URL, Token: "tok"},
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
		if err := os.WriteFile(filepath.Join(dir, "patch.diff"), patch, 0o600); err != nil { //nolint:gosec // test helper: the path is a temp dir and a job id
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

func TestBuilderJobCarriesProjectInstructionsAndTheChecklist(t *testing.T) {
	r := newRig(t)
	if err := os.MkdirAll(filepath.Join(r.up, ".nightshift"), 0o750); err != nil {
		t.Fatal(err)
	}
	doc := "name: app\nstack: [go]\ninstructions: [AGENTS.md]\nverify: make verify\n"
	if err := os.WriteFile(filepath.Join(r.up, ".nightshift", "project.yaml"), []byte(doc), 0o600); err != nil {
		t.Fatal(err)
	}
	gitIn(t, r.up, "add", ".")
	gitIn(t, r.up, "commit", "-q", "-m", "project")
	r.c.Standards = fstest.MapFS{"industry/go/style.yaml": {Data: []byte("id: go.style\ntitle: Go style\nstatus: active\n")}}
	r.advance()
	var job jobsapi.Job
	if err := json.Unmarshal(r.lastJob().Payload, &job); err != nil {
		t.Fatal(err)
	}
	if len(job.Instructions) != 1 || job.Instructions[0] != "AGENTS.md" || len(job.Checklist) != 1 || job.Checklist[0] != "Go style (go.style)" {
		t.Fatalf("instructions %q, checklist %q", job.Instructions, job.Checklist)
	}
}

func TestFreshBuilderSessionGetsTheHandoffOrAMinimalOne(t *testing.T) {
	r := newRig(t)
	ctx := r.t.Context()
	r.advance()
	// The first session dies before it writes a handoff: the next prompt lists the events.
	r.finish(r.patchFor("feature.txt", "x\n"), red("verify failed"))
	r.advance()
	if p := string(r.lastJob().Payload); !strings.Contains(p, "Previous session, unverified") || !strings.Contains(p, "No handoff was written") {
		t.Fatalf("no minimal handoff: %s", p)
	}
	// The second session writes one: the third prompt carries it, not the minimal list.
	withHandoff := `{"status":"done","summary":"s","changed_files":["feature.txt"],"handoff":{"done":["the parser"],"pending":["the docs"]}}`
	r.finish(r.patchFor("feature.txt", "y\n"), red("verify failed again"))
	if _, err := r.c.Pool.Exec(ctx, `UPDATE book.jobs SET result = $2 WHERE id = $1`, r.lastJob().ID, []byte(withHandoff)); err != nil {
		t.Fatal(err)
	}
	r.advance()
	p := string(r.lastJob().Payload)
	if !strings.Contains(p, "the parser") || !strings.Contains(p, "the docs") || strings.Contains(p, "No handoff was written") {
		t.Fatalf("handoff not carried: %s", p)
	}
}

// limitRig gives the router two providers and plays a Builder job that hit a limit.
func limitRig(t *testing.T, chain ...string) *rig {
	t.Helper()
	r := newRig(t)
	r.c.Router.Providers = map[string]config.Provider{"a": {Harness: "fake"}, "b": {Harness: "fake"}}
	r.c.Router.Chains = map[string][]string{"builder": chain}
	return r
}

func (r *rig) hitLimit(ago time.Duration) {
	r.t.Helper()
	job := r.lastJob()
	if _, err := r.c.Pool.Exec(r.t.Context(), `UPDATE book.jobs SET state = 'failed', exit_reason = 'rate_limit', usage = '{"limit_hit":true}', finished_at = now() - $2::interval WHERE id = $1`, job.ID, ago.String()); err != nil {
		r.t.Fatal(err)
	}
}

// TestLimitMovesJobToNextProvider: a limit is not a failure. The task stays in building and
// the next job runs on the next provider of the chain (design section 12, ADR 0012).
func TestLimitMovesJobToNextProvider(t *testing.T) {
	r := limitRig(t, "a", "b")
	r.advance()
	if got := r.lastJob().Provider; got != "a" {
		t.Fatalf("first job on %s", got)
	}
	r.hitLimit(0)
	r.advance() // closes the failed job
	r.advance() // queues the next one
	if r.task.State != "building" {
		t.Fatalf("a limit moved the task to %s", r.task.State)
	}
	if j := r.lastJob(); j.Provider != "b" || j.State != "queued" {
		t.Fatalf("next job: provider %s, state %s", j.Provider, j.State)
	}
}

// TestLimitParksUntilBackoffEnds: with one provider the task waits, and the wait is read from
// the book, so a new router (a restart) agrees.
func TestLimitParksUntilBackoffEnds(t *testing.T) {
	r := limitRig(t, "a")
	r.advance()
	r.hitLimit(time.Minute)
	r.advance()
	r.advance()
	if r.task.State != "building" || !r.lastJob().ConsumedAt.Valid || r.lastJob().State != "failed" {
		t.Fatalf("parked task: state %s, last job %+v", r.task.State, r.lastJob())
	}
	r.c.Router = &dispatch.Router{Pool: r.c.Pool, Providers: r.c.Router.Providers, Chains: r.c.Router.Chains}
	r.advance()
	if r.lastJob().State != "failed" {
		t.Fatal("a job was queued while the provider is limited")
	}
	// The backoff (15 minutes) has passed: the same provider works again.
	if _, err := r.c.Pool.Exec(t.Context(), `UPDATE book.jobs SET finished_at = now() - interval '20 minutes' WHERE id = $1`, r.lastJob().ID); err != nil {
		t.Fatal(err)
	}
	r.advance()
	if j := r.lastJob(); j.State != "queued" || j.Provider != "a" {
		t.Fatalf("after the backoff: %+v", j)
	}
}

// TestPlannedTaskWaitsForItsTime: a queued task with a not_before in the future stays queued
// with no job; once the time has passed it starts (design section 12, plan L3).
func TestPlannedTaskWaitsForItsTime(t *testing.T) {
	r := newRig(t)
	if r.task.State != "queued" {
		t.Fatalf("the rig task is %s", r.task.State)
	}
	if _, err := r.c.Pool.Exec(t.Context(), `UPDATE book.tasks SET not_before = now() + interval '1 hour' WHERE id = $1`, r.task.ID); err != nil {
		t.Fatal(err)
	}
	r.reload()
	r.advance()
	if r.task.State != "queued" {
		t.Fatalf("a planned task started early: %s", r.task.State)
	}
	if _, err := r.c.Pool.Exec(t.Context(), `UPDATE book.tasks SET not_before = now() - interval '1 minute' WHERE id = $1`, r.task.ID); err != nil {
		t.Fatal(err)
	}
	r.reload()
	r.advance()
	if r.task.State != "building" {
		t.Fatalf("after its time the task is %s", r.task.State)
	}
}

// TestHigherPriorityIsListedFirst: the conductor advances tasks by priority, then by age.
func TestHigherPriorityIsListedFirst(t *testing.T) {
	r := newRig(t)
	other, err := r.q.CreateQueuedTask(t.Context(), bookdb.CreateQueuedTaskParams{Tenant: "example-solo", Repo: "example/app", Title: "Urgent", Priority: 3})
	if err != nil {
		t.Fatal(err)
	}
	tasks, err := r.q.ListActiveTasks(t.Context(), bookdb.ListActiveTasksParams{Tenant: "example-solo", States: []string{"queued"}})
	if err != nil || len(tasks) != 2 || tasks[0].ID != other.ID {
		t.Fatalf("order %v, %v", tasks, err)
	}
}

// TestFallbackProviderNeedsTheTaskToOptIn: a primary provider at a limit that resets in five
// hours sends a task that allows it to the fallback provider; a task that does not waits.
func TestFallbackProviderNeedsTheTaskToOptIn(t *testing.T) {
	for _, allow := range []bool{true, false} {
		t.Run(strconv.FormatBool(allow), func(t *testing.T) { fallbackCase(t, allow) })
	}
}

func fallbackCase(t *testing.T, allow bool) {
	{
		r := limitRig(t, "a", "b")
		r.c.Router.Providers["b"] = config.Provider{Harness: "fake", Tier: "fallback"}
		if _, err := r.c.Pool.Exec(t.Context(), `UPDATE book.tasks SET allow_fallback = $2 WHERE id = $1`, r.task.ID, allow); err != nil {
			t.Fatal(err)
		}
		r.reload()
		r.advance() // first job on the primary provider
		reset := time.Now().Add(5 * time.Hour).UTC().Format(time.RFC3339)
		if _, err := r.c.Pool.Exec(t.Context(), `UPDATE book.jobs SET state = 'failed', exit_reason = 'rate_limit', finished_at = now(), usage = jsonb_build_object('limit_until', $2::text) WHERE id = $1`, r.lastJob().ID, reset); err != nil {
			t.Fatal(err)
		}
		r.advance() // closes the job
		r.advance() // routes the next one
		j := r.lastJob()
		switch {
		case allow && (j.Provider != "b" || j.State != "queued"):
			t.Fatalf("opted in: job on %s in state %s", j.Provider, j.State)
		case !allow && j.State != "failed":
			t.Fatalf("not opted in: a job was queued on %s", j.Provider)
		}
	}
}

// A task typed with `nightshift task add` keeps its title short, but Builder gets the whole text.
func TestBuilderJobCarriesTheWholeTaskText(t *testing.T) {
	r := newRig(t)
	long := "Add the export\n\n" + strings.Repeat("Acceptance: the file keeps its header row. ", 10) + "LAST-LINE-MARKER"
	r.say("task_created", long)
	r.advance()
	if p := string(r.lastJob().Payload); !strings.Contains(p, "LAST-LINE-MARKER") {
		t.Fatalf("Builder's prompt lost the end of the task: %s", p)
	}
}

func TestBuilderPromptKeepsAHugeTaskTextBounded(t *testing.T) {
	r := newRig(t)
	r.say("task_created", "Add the export\n\n"+strings.Repeat("x", 20000))
	r.advance()
	if n := len(r.lastJob().Payload); n > 12000 {
		t.Fatalf("the job payload is %d bytes", n)
	}
}
