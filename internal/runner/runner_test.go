package runner_test

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"testing/fstest"
	"time"

	"github.com/ilyachakun/nightshift/internal/gitops"
	"github.com/ilyachakun/nightshift/internal/harness"
	"github.com/ilyachakun/nightshift/internal/jobsapi"
	"github.com/ilyachakun/nightshift/internal/runner"
	"github.com/ilyachakun/nightshift/internal/stack"
)

// jobs is an in-memory jobs API with one queued job.
type jobs struct {
	mu        sync.Mutex
	next      *jobsapi.ClaimResponse
	events    []string
	completed []jobsapi.CompleteRequest
}

func (j *jobs) Claim(context.Context, string, time.Duration) (jobsapi.ClaimResponse, error) {
	j.mu.Lock()
	defer j.mu.Unlock()
	if j.next == nil {
		return jobsapi.ClaimResponse{}, nil
	}
	c := *j.next
	j.next = nil
	return c, nil
}

func (j *jobs) Heartbeat(context.Context, string, string, string) error { return nil }

func (j *jobs) Events(_ context.Context, _, _ string, ev []jobsapi.Event) error {
	j.mu.Lock()
	defer j.mu.Unlock()
	for _, e := range ev {
		j.events = append(j.events, e.Kind)
	}
	return nil
}

func (j *jobs) Complete(_ context.Context, _, _ string, req jobsapi.CompleteRequest) error {
	j.mu.Lock()
	defer j.mu.Unlock()
	j.completed = append(j.completed, req)
	return nil
}

// agent writes a file into the workspace like a Builder would.
type agent struct {
	err  error
	out  string
	env  *map[string]string       // receives the request environment, when set
	peek func(harness.RunRequest) // sees the request while the job directory still exists
}

func (a agent) Run(_ context.Context, req harness.RunRequest) (harness.RunResult, error) {
	if a.env != nil {
		*a.env = req.Env
	}
	if a.peek != nil {
		a.peek(req)
	}
	if a.err != nil {
		return harness.RunResult{}, a.err
	}
	if err := os.WriteFile(filepath.Join(req.Workspace, "feature.txt"), []byte("done\n"), 0o600); err != nil {
		return harness.RunResult{}, err
	}
	return harness.RunResult{Output: []byte(a.out), SessionID: "s1", ExitReason: harness.ExitDone, Usage: harness.Usage{InputTokens: 7}}, nil
}

func git(t *testing.T, dir string, args ...string) string {
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

// fixture builds a mirror of a one-commit repository and a runner on top of it.
func fixture(t *testing.T, h harness.Harness) (*runner.Runner, *jobs) {
	t.Helper()
	return fixtureFiles(t, h, nil)
}

// fixtureFiles is fixture with more files in the repository's first commit.
func fixtureFiles(t *testing.T, h harness.Harness, files map[string]string) (*runner.Runner, *jobs) {
	t.Helper()
	up := t.TempDir()
	git(t, up, "init", "-q", "-b", "main")
	if err := os.WriteFile(filepath.Join(up, "a.txt"), []byte("one\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	for name, body := range files {
		if err := os.MkdirAll(filepath.Join(up, filepath.Dir(name)), 0o750); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(up, name), []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	git(t, up, "add", ".")
	git(t, up, "commit", "-q", "-m", "init")
	base := git(t, up, "rev-parse", "HEAD")

	g := &gitops.Git{Dir: t.TempDir()}
	if _, err := g.EnsureMirror(t.Context(), "example/app", up, gitops.Auth{}); err != nil {
		t.Fatal(err)
	}
	q := &jobs{next: &jobsapi.ClaimResponse{
		JobID: "7", JobToken: "tok",
		Job: jobsapi.Job{
			Tenant: "example-solo", TaskID: "3", Role: "builder", Provider: "fake",
			Repo: "example/app", BaseSHA: base, Prompt: "add a feature",
		},
	}}
	r := &runner.Runner{
		Jobs: q, WorkerID: "w1", Git: g, WorkDir: t.TempDir(), ArtifactDir: t.TempDir(),
		Harness: func(jobsapi.Job) (harness.Harness, error) { return h, nil },
	}
	return r, q
}

// TestRunOneProducesPatchAndArtifacts is the T4-lite path: job → checkout → harness → patch → complete.
func TestRunOneProducesPatchAndArtifacts(t *testing.T) {
	t.Parallel()
	r, q := fixture(t, agent{out: `{"summary":"ok"}`})
	ran, err := r.RunOne(t.Context())
	if err != nil || !ran {
		t.Fatalf("run: %v %v", ran, err)
	}
	if len(q.completed) != 1 {
		t.Fatalf("completed: %+v", q.completed)
	}
	c := q.completed[0]
	if c.ExitReason != "done" || c.SessionID != "s1" || c.Usage.InputTokens != 7 || c.PatchPath != runner.PatchName || len(c.Artifacts) != 1 {
		t.Fatalf("complete: %+v", c)
	}
	data, err := os.ReadFile(filepath.Join(r.ArtifactDir, "7", runner.PatchName))
	if err != nil || !strings.Contains(string(data), "feature.txt") {
		t.Fatalf("patch: %q %v", data, err)
	}
	// nightshift, another user, reads the patch through the group of the artifacts volume.
	if info, err := os.Stat(filepath.Join(r.ArtifactDir, "7", runner.PatchName)); err != nil || info.Mode().Perm()&0o040 == 0 {
		t.Errorf("the patch must be readable by the group: %v %v", info, err)
	}
	sum := sha256.Sum256(data)
	if c.Artifacts[0].SHA256 != hex.EncodeToString(sum[:]) || c.Artifacts[0].Bytes != int64(len(data)) {
		t.Fatalf("artifact: %+v", c.Artifacts[0])
	}
	if got := strings.Join(q.events, ","); got != "turn.started,turn.finished,check.nothing_to_run,check.started,check.finished" { // no project.yaml: no check ran, and the job says so
		t.Fatalf("events: %s", got)
	}
	if ran, err := r.RunOne(t.Context()); err != nil || ran {
		t.Fatalf("empty queue: %v %v", ran, err)
	}
}

// TestNoCheckoutOutlivesItsJob: the next job, of another tenant, finds no checkout of the
// last one while its agent runs (design section 17, "Worker state"; T6).
func TestNoCheckoutOutlivesItsJob(t *testing.T) {
	t.Parallel()
	var seen []string
	var root string
	r, q := fixture(t, agent{out: `{"summary":"ok"}`, peek: func(harness.RunRequest) {
		entries, _ := filepath.Glob(filepath.Join(root, "*", "*"))
		seen = append(seen, strings.Join(entries, ","))
	}})
	root = r.WorkDir
	first := *q.next
	if ran, err := r.RunOne(t.Context()); err != nil || !ran {
		t.Fatalf("first job: %v %v", ran, err)
	}
	second := first
	second.JobID, second.Job.Tenant, second.Job.TaskID = "8", "example-team", "9"
	q.next = &second
	if ran, err := r.RunOne(t.Context()); err != nil || !ran {
		t.Fatalf("second job: %v %v", ran, err)
	}
	want := []string{filepath.Join(root, "example-solo", "3"), filepath.Join(root, "example-team", "9")}
	if len(seen) != 2 || seen[0] != want[0] || seen[1] != want[1] {
		t.Fatalf("checkouts the agents saw: %q, want only their own %q", seen, want)
	}
	if entries, _ := filepath.Glob(filepath.Join(root, "*", "*")); len(entries) != 0 {
		t.Fatalf("checkouts left after the jobs: %v", entries)
	}
}

// TestAPlantedLinkOrReadOnlyDirDoesNotEscapeOrBlock: a tenant directory an earlier agent
// replaced by a link is not followed, and a read-only directory left in a checkout does not
// fail the next job (T6).
func TestAPlantedLinkOrReadOnlyDirDoesNotEscapeOrBlock(t *testing.T) {
	t.Parallel()
	r, _ := fixture(t, agent{out: `{"summary":"ok"}`})
	loot := t.TempDir()
	if err := os.Symlink(loot, filepath.Join(r.WorkDir, "example-solo")); err != nil {
		t.Fatal(err)
	}
	if ran, err := r.RunOne(t.Context()); err != nil || !ran {
		t.Fatalf("run: %v %v", ran, err)
	}
	if entries, _ := os.ReadDir(loot); len(entries) != 0 {
		t.Fatalf("the checkout went through the planted link: %v", entries)
	}

	r2, _ := fixture(t, agent{out: `{"summary":"ok"}`})
	locked := filepath.Join(r2.WorkDir, "example-solo", "3", "cache")
	if err := os.MkdirAll(locked, 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(locked, "mod"), []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(locked, 0o500); err != nil { //nolint:gosec // the read-only directory is the point of the test
		t.Fatal(err)
	}
	if ran, err := r2.RunOne(t.Context()); err != nil || !ran {
		t.Fatalf("a read-only directory blocked the job: %v %v", ran, err)
	}
}

// TestHarnessErrorCompletesAsError checks that a failing agent still ends the job.
func TestHarnessErrorCompletesAsError(t *testing.T) {
	t.Parallel()
	r, q := fixture(t, agent{err: errors.New("model unavailable")})
	if ran, err := r.RunOne(t.Context()); err != nil || !ran {
		t.Fatalf("run: %v %v", ran, err)
	}
	if len(q.completed) != 1 || q.completed[0].ExitReason != "error" || q.completed[0].PatchPath != "" {
		t.Fatalf("complete: %+v", q.completed)
	}
	if got := strings.Join(q.events, ","); got != "turn.started,job.error" {
		t.Fatalf("events: %s", got)
	}
}

// TestUnsafeIDsAreRejected checks that a task id cannot walk out of the work directory.
func TestUnsafeIDsAreRejected(t *testing.T) {
	t.Parallel()
	r, q := fixture(t, agent{})
	q.next.Job.TaskID = "../../etc"
	if _, err := r.RunOne(t.Context()); err != nil {
		t.Fatal(err)
	}
	if len(q.completed) != 1 || q.completed[0].ExitReason != "error" {
		t.Fatalf("complete: %+v", q.completed)
	}
}

// TestTraceparentReachesTheAgent checks that the task's trace context and the credentials
// both arrive in the harness environment.
func TestTraceparentReachesTheAgent(t *testing.T) {
	t.Parallel()
	var env map[string]string
	r, q := fixture(t, agent{out: `{}`, env: &env})
	r.Env = map[string]string{"CLAUDE_CODE_OAUTH_TOKEN": "tok"}
	const tp = "00-0af7651916cd43dd8448eb211c80319c-b7ad6b7169203331-01"
	q.next.Job.Traceparent = tp
	if _, err := r.RunOne(t.Context()); err != nil {
		t.Fatal(err)
	}
	if env["TRACEPARENT"] != tp || env["CLAUDE_CODE_OAUTH_TOKEN"] != "tok" {
		t.Fatalf("env: %v", env)
	}
	if _, set := r.Env["TRACEPARENT"]; set {
		t.Fatal("the shared credentials map was modified")
	}
}

// project is a .nightshift/project.yaml with the given verify command.
func project(verify string) map[string]string {
	return map[string]string{".nightshift/project.yaml": "verify: " + verify + "\n"}
}

// TestVerifyRunsAfterTheAgentAndItsFindingsTravel checks L1: a passing verify sends nothing,
// a failing one sends a blocking finding, and neither reads a command the agent wrote.
func TestVerifyRunsAfterTheAgentAndItsFindingsTravel(t *testing.T) {
	t.Parallel()
	for name, tc := range map[string]struct {
		verify  string
		want    int
		partial bool
	}{
		"passes":         {`"test -f feature.txt"`, 0, false},
		"fails":          {`"test ! -f feature.txt"`, 1, true},
		"rewrites-patch": {`"printf rewritten > feature.txt"`, 0, true},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			r, q := fixtureFiles(t, agent{out: `{"summary":"ok"}`}, project(tc.verify))
			r.AgentHome = t.TempDir()
			if _, err := r.RunOne(t.Context()); err != nil {
				t.Fatal(err)
			}
			c := q.completed[0]
			if c.ExitReason != "done" || len(c.Findings) != tc.want || c.CheckSuite == "" || strings.HasPrefix(c.CheckSuite, "partial:") != tc.partial {
				t.Fatalf("complete: %+v", c)
			}
			if tc.want == 1 && !strings.Contains(string(c.Findings[0]), `"verify.failed"`) {
				t.Fatalf("finding: %s", c.Findings[0])
			}
		})
	}
}

// TestNoVerifyWithoutAnAgentHome checks that a worker that cannot drop to the agent user
// never runs repository commands.
func TestNoVerifyWithoutAnAgentHome(t *testing.T) {
	t.Parallel()
	r, q := fixtureFiles(t, agent{out: `{"summary":"ok"}`}, project(`"false"`))
	if _, err := r.RunOne(t.Context()); err != nil {
		t.Fatal(err)
	}
	if len(q.completed[0].Findings) != 0 {
		t.Fatalf("verify ran: %+v", q.completed[0])
	}
}

// TestStackToolsRunAfterVerify checks that the tools of the project's stack packs run as
// the agent user, that a failing one sends a blocking finding, and that a tool the worker
// cannot run is skipped instead of failing the patch.
func TestStackToolsRunAfterVerify(t *testing.T) {
	t.Parallel()
	packs, err := stack.Load(fstest.MapFS{"demo/pack.yaml": {Data: []byte(`name: demo
detect: [demo.cfg]
toolchain: [sh]
tools:
  - { name: demo-lint, requires: sh, category: code, command: "echo bad >&2; exit 1" }
  - { name: demo-absent, requires: no-such-tool-anywhere, category: code, command: "exit 1" }
`)}})
	if err != nil {
		t.Fatal(err)
	}
	files := map[string]string{".nightshift/project.yaml": "stack: [demo]\nverify: \"true\"\n"}
	r, q := fixtureFiles(t, agent{out: `{"summary":"ok"}`}, files)
	r.AgentHome, r.Stacks = t.TempDir(), packs
	if _, err := r.RunOne(t.Context()); err != nil {
		t.Fatal(err)
	}
	c := q.completed[0]
	if len(c.Findings) != 1 || !strings.Contains(string(c.Findings[0]), `"demo-lint.failed"`) {
		t.Fatalf("findings: %s", c.Findings)
	}
}

// TestAProjectPackAddsItsOwnChecks covers a language the factory has no pack for: the project
// ships `.nightshift/stacks/<name>/pack.yaml`, and its tools run like a factory pack's.
func TestAProjectPackAddsItsOwnChecks(t *testing.T) {
	t.Parallel()
	files := map[string]string{
		".nightshift/project.yaml":          "stack: [lang]\nverify: \"true\"\n",
		".nightshift/stacks/lang/pack.yaml": "name: lang\ndetect: [lang.cfg]\ntoolchain: [sh]\ntools:\n  - { name: lang-lint, requires: sh, category: code, command: \"echo bad >&2; exit 1\" }\n",
	}
	r, q := fixtureFiles(t, agent{out: `{"summary":"ok"}`}, files)
	r.AgentHome, r.Stacks = t.TempDir(), stack.Packs{}
	if _, err := r.RunOne(t.Context()); err != nil {
		t.Fatal(err)
	}
	if c := q.completed[0]; len(c.Findings) != 1 || !strings.Contains(string(c.Findings[0]), `"lang-lint.failed"`) {
		t.Fatalf("findings: %s", c.Findings)
	}
}

// TestAHarnessWithoutCostIsPricedFromTheProvider covers codex, which reports tokens only.
func TestAHarnessWithoutCostIsPricedFromTheProvider(t *testing.T) {
	t.Parallel()
	r, q := fixture(t, agent{out: `{"summary":"ok"}`}) // the agent reports 7 input tokens
	q.next.Job.Price = &jobsapi.Price{InputPerMTok: 1_000_000, OutputPerMTok: 2}
	if ran, err := r.RunOne(t.Context()); err != nil || !ran {
		t.Fatalf("run: %v %v", ran, err)
	}
	if got := q.completed[0].Usage.CostUSD; got != 7 {
		t.Fatalf("cost %v, want 7", got)
	}
}

// TestSetupRunsBeforeTheAgentAndAFailureStopsTheJob checks that `project.yaml: setup` runs in the
// checkout before the agent starts, and that a failing setup ends the job with its output.
func TestSetupRunsBeforeTheAgentAndAFailureStopsTheJob(t *testing.T) {
	t.Parallel()
	ok := map[string]string{".nightshift/project.yaml": "setup: \"touch deps.installed\"\nverify: \"test -f deps.installed\"\n"}
	r, q := fixtureFiles(t, agent{out: `{"summary":"ok"}`}, ok)
	r.AgentHome = t.TempDir()
	if _, err := r.RunOne(t.Context()); err != nil {
		t.Fatal(err)
	}
	if c := q.completed[0]; c.ExitReason != "done" || len(c.Findings) != 0 {
		t.Fatalf("a passing setup: %+v", c)
	}
	if got := strings.Join(q.events, ","); !strings.Contains(got, "setup.started,setup.finished") {
		t.Fatalf("events: %s", got)
	}

	bad := map[string]string{".nightshift/project.yaml": "setup: \"echo no network >&2; exit 3\"\nverify: \"true\"\n"}
	r, q = fixtureFiles(t, agent{out: `{"summary":"ok"}`}, bad)
	r.AgentHome = t.TempDir()
	if _, err := r.RunOne(t.Context()); err != nil {
		t.Fatal(err)
	}
	if c := q.completed[0]; c.ExitReason != "error" {
		t.Fatalf("a failing setup left the job in %q", c.ExitReason)
	}
	if got := strings.Join(q.events, ","); !strings.Contains(got, "job.error") || strings.Contains(got, "turn.started") {
		t.Fatalf("events: %s", got)
	}
}

// TestPinnedToolchainIsInstalledWithMise checks that a project with a mise.toml gets
// `mise install` before its setup, and that setup then runs through `mise exec`. A fake mise on
// PATH records what it was asked; the test cannot run in parallel because it changes PATH.
func TestPinnedToolchainIsInstalledWithMise(t *testing.T) {
	bin, log := t.TempDir(), filepath.Join(t.TempDir(), "mise.log")
	script := "#!/bin/sh\necho \"$@\" >> " + log + "\nif [ \"$1\" = exec ]; then shift 2; exec \"$@\"; fi\n"
	if err := os.WriteFile(filepath.Join(bin, "mise"), []byte(script), 0o700); err != nil { //nolint:gosec // a test script
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH")) //nolint:forbidigo // the test puts a fake mise first on the real PATH
	files := map[string]string{"mise.toml": "[tools]\nnode = \"22\"\n", ".nightshift/project.yaml": "setup: \"touch deps.installed\"\nverify: \"test -f deps.installed\"\n"}
	r, q := fixtureFiles(t, agent{out: `{"summary":"ok"}`}, files)
	r.AgentHome = t.TempDir()
	if _, err := r.RunOne(t.Context()); err != nil {
		t.Fatal(err)
	}
	if c := q.completed[0]; c.ExitReason != "done" || len(c.Findings) != 0 {
		t.Fatalf("complete: %+v", c)
	}
	got, err := os.ReadFile(log) //nolint:gosec // a path in the test's temporary directory
	if err != nil || !strings.HasPrefix(string(got), "install --yes\n") || !strings.Contains(string(got), "exec -- sh -c touch deps.installed") {
		t.Fatalf("mise was called with %q (%v)", got, err)
	}
}

type fakeInstaller struct {
	jobs []string
	dirs []string
	err  error
}

func (f *fakeInstaller) Install(_ context.Context, job, dir string) (string, error) {
	f.jobs, f.dirs = append(f.jobs, job), append(f.dirs, dir)
	return "installing node", f.err
}

func TestToolchainServiceInstallsInsteadOfTheWorkersOwnMise(t *testing.T) {
	bin, log := t.TempDir(), filepath.Join(t.TempDir(), "mise.log")
	script := "#!/bin/sh\necho \"$@\" >> " + log + "\n"
	if err := os.WriteFile(filepath.Join(bin, "mise"), []byte(script), 0o700); err != nil { //nolint:gosec // a test script
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH")) //nolint:forbidigo // the test puts a fake mise first on the real PATH
	r, _ := fixtureFiles(t, agent{out: `{"summary":"ok"}`}, map[string]string{"mise.toml": "[tools]\nnode = \"22\"\n"})
	inst := &fakeInstaller{}
	r.AgentHome, r.Installer = t.TempDir(), inst
	if _, err := r.RunOne(t.Context()); err != nil {
		t.Fatal(err)
	}
	if len(inst.jobs) != 1 || inst.jobs[0] == "" {
		t.Fatalf("the service was called %v", inst.jobs)
	}
	if got, _ := os.ReadFile(log); len(got) != 0 { //nolint:gosec // a path in the test's temporary directory
		t.Fatalf("the worker ran mise itself: %q", got)
	}
}

func TestToolchainServiceFailureStopsTheJob(t *testing.T) {
	r, q := fixtureFiles(t, agent{out: `{"summary":"ok"}`}, map[string]string{"mise.toml": "[tools]\nnode = \"22\"\n"})
	r.AgentHome, r.Installer = t.TempDir(), &fakeInstaller{err: errors.New("no route")}
	bin := t.TempDir()
	if err := os.WriteFile(filepath.Join(bin, "mise"), []byte("#!/bin/sh\n"), 0o700); err != nil { //nolint:gosec // a test script
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH")) //nolint:forbidigo // the test puts a fake mise first on the real PATH
	if _, err := r.RunOne(t.Context()); err != nil {
		t.Fatal(err)
	}
	if c := q.completed[0]; c.ExitReason != "error" || strings.Contains(strings.Join(q.events, ","), "turn.started") {
		t.Fatalf("a failed install left the job in %q", c.ExitReason)
	}
}

// lockWriter is an Installer that rewrites the tool lockfile, as `mise install` does.
type lockWriter struct{}

func (lockWriter) Install(_ context.Context, _, dir string) (string, error) {
	if err := os.Remove(filepath.Join(dir, "mise.local.lock")); err != nil { // a tracked lockfile the install deletes
		return "", err
	}
	for name, body := range map[string]string{"mise.lock": "rewritten\n", ".mise.lock": "created\n"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o600); err != nil {
			return "", err
		}
	}
	return "", nil
}

// The install must not leave its lockfile changes in the checkout, or the first patch of Builder
// is rejected as touching a protected path.
func TestToolchainInstallLeavesTheLockfilesAsCommitted(t *testing.T) {
	bin := t.TempDir()
	if err := os.WriteFile(filepath.Join(bin, "mise"), []byte("#!/bin/sh\n"), 0o700); err != nil { //nolint:gosec // a test script
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH")) //nolint:forbidigo // the test puts a fake mise first on the real PATH
	files := map[string]string{
		"mise.toml":                "[tools]\n",
		"mise.lock":                "committed\n",
		"mise.local.lock":          "committed\n",
		".nightshift/project.yaml": `verify: "git diff --quiet -- mise.lock mise.local.lock && test -f mise.local.lock && test ! -e .mise.lock"` + "\n",
	}
	r, q := fixtureFiles(t, agent{out: `{"summary":"ok"}`}, files)
	r.AgentHome, r.Installer = t.TempDir(), lockWriter{}
	if _, err := r.RunOne(t.Context()); err != nil {
		t.Fatal(err)
	}
	if c := q.completed[0]; c.ExitReason != "done" || len(c.Findings) != 0 {
		t.Fatalf("the install left lockfile changes: %+v %s", c, c.Findings)
	}
}

// TestJobTokenBudgetStopsARun: the job's token budget reaches the supervisor, and a run it
// stopped completes with the budget exit and the reason.
func TestJobTokenBudgetStopsARun(t *testing.T) {
	t.Parallel()
	spender := agent{out: `{}`, peek: func(req harness.RunRequest) {
		req.Emit(harness.Event{Kind: harness.EventUsage, Usage: harness.Usage{InputTokens: 600}})
	}}
	r, q := fixture(t, spender)
	q.next.Job.Budget.Tokens = 500
	if ran, err := r.RunOne(t.Context()); err != nil || !ran {
		t.Fatalf("run: %v %v", ran, err)
	}
	c := q.completed[0]
	if c.ExitReason != "budget" || !strings.Contains(string(c.Result), "tokens: 600 used, limit 500") {
		t.Fatalf("complete: %+v", c)
	}
}

// A job's own limits go over the worker's default: a lower repeat limit stops a run the default
// lets through, and 0 switches the rule off.
func TestJobLimitsOverrideTheWorkerPolicy(t *testing.T) {
	t.Parallel()
	repeat := agent{out: `{}`, peek: func(req harness.RunRequest) {
		for range 3 {
			req.Emit(harness.Event{Kind: harness.EventToolCall, Tool: "Bash", Input: "go test"})
		}
	}}
	two, zero := 2, 0
	r, q := fixture(t, repeat)
	q.next.Job.Budget.RepeatLimit = &two
	if ran, err := r.RunOne(t.Context()); err != nil || !ran {
		t.Fatalf("run: %v %v", ran, err)
	}
	if c := q.completed[0]; c.ExitReason != "budget" || !strings.Contains(string(c.Result), "loop: Bash") {
		t.Fatalf("limit 2: %+v", c)
	}
	r, q = fixture(t, repeat)
	r.Supervise.RepeatLimit = 2
	q.next.Job.Budget.RepeatLimit = &zero
	if ran, err := r.RunOne(t.Context()); err != nil || !ran {
		t.Fatalf("run: %v %v", ran, err)
	}
	if c := q.completed[0]; c.ExitReason != "done" {
		t.Fatalf("limit switched off: %+v", c)
	}
}

// A stopped run has no result line from the CLI, so its cost comes from the tokens the
// supervisor saw and the provider's price (Mac live check, 2026-11-26).
func TestStoppedRunIsPriced(t *testing.T) {
	t.Parallel()
	spender := agent{out: `{}`, peek: func(req harness.RunRequest) {
		req.Emit(harness.Event{Kind: harness.EventUsage, Usage: harness.Usage{InputTokens: 1_000_000}})
	}}
	r, q := fixture(t, spender)
	q.next.Job.Budget.Tokens = 500
	q.next.Job.Price = &jobsapi.Price{InputPerMTok: 3}
	if ran, err := r.RunOne(t.Context()); err != nil || !ran {
		t.Fatalf("run: %v %v", ran, err)
	}
	if c := q.completed[0]; c.ExitReason != "budget" || c.Usage.CostUSD != 3 {
		t.Fatalf("complete: %+v", c)
	}
}

// TestANoRepoJobRunsInAnEmptyDirectory: an independent reader gets no checkout, so no instruction
// file of the repository reaches it.
func TestANoRepoJobRunsInAnEmptyDirectory(t *testing.T) {
	t.Parallel()
	var listed []string
	r, q := fixture(t, agent{out: `{"summary":"ok"}`, peek: func(req harness.RunRequest) {
		entries, _ := os.ReadDir(req.Workspace)
		for _, e := range entries {
			listed = append(listed, e.Name())
		}
	}})
	q.next.Job.NoRepo = true
	q.next.Job.Repo = ""
	if ran, err := r.RunOne(t.Context()); err != nil || !ran {
		t.Fatalf("job: %v %v", ran, err)
	}
	if len(q.completed) != 1 || q.completed[0].PatchPath != "" {
		t.Fatalf("the job did not complete cleanly: %+v", q.completed)
	}
	if len(listed) != 0 {
		t.Fatalf("the reader's directory held %v", listed)
	}
}

// TestEachTenantHasItsOwnHome: the setup command of a job runs with the HOME of
// that job's tenant, so what one tenant's job leaves in its home is not in another tenant's (M3).
func TestEachTenantHasItsOwnHome(t *testing.T) {
	t.Parallel()
	files := map[string]string{".nightshift/project.yaml": "setup: \"echo $HOME > home.txt; touch $HOME/mine\"\nverify: \"true\"\n"}
	r, q := fixtureFiles(t, agent{out: `{"summary":"ok"}`}, files)
	base := t.TempDir()
	r.AgentHome = base
	var homes []string
	r.Harness = func(jobsapi.Job) (harness.Harness, error) {
		return agent{out: `{"summary":"ok"}`, peek: func(req harness.RunRequest) {
			raw, _ := os.ReadFile(filepath.Join(req.Workspace, "home.txt"))
			homes = append(homes, strings.TrimSpace(string(raw)))
		}}, nil
	}
	first := *q.next
	for i, tenant := range []string{"example-solo", "example-team"} {
		j := first
		j.JobID, j.Job.Tenant, j.Job.TaskID = fmt.Sprint(20+i), tenant, fmt.Sprint(30+i)
		q.next = &j
		if ran, err := r.RunOne(t.Context()); err != nil || !ran {
			t.Fatalf("%s: %v %v", tenant, ran, err)
		}
	}
	want := []string{filepath.Join(base, "tenants", "example-solo"), filepath.Join(base, "tenants", "example-team")}
	if len(homes) != 2 || homes[0] != want[0] || homes[1] != want[1] {
		t.Fatalf("homes the setup saw: %q, want %q", homes, want)
	}
	for _, tenant := range []string{"example-solo", "example-team"} {
		if _, err := os.Stat(filepath.Join(base, "tenants", tenant, "mine")); err != nil {
			t.Fatalf("the file the setup made is not in %s's home: %v", tenant, err)
		}
	}
	if _, err := os.Stat(filepath.Join(base, "mine")); err == nil {
		t.Fatal("a tenant's job wrote to the shared agent home")
	}
	if runner.HomeFor(base, "../x") != "" || runner.HomeFor("", "example-solo") != "" {
		t.Fatal("an unsafe tenant or an empty base must give no home")
	}
}
