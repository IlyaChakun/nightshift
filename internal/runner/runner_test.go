package runner_test

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
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
	if got := strings.Join(q.events, ","); got != "turn.started,turn.finished" {
		t.Fatalf("events: %s", got)
	}
	if ran, err := r.RunOne(t.Context()); err != nil || ran {
		t.Fatalf("empty queue: %v %v", ran, err)
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
		verify string
		want   int
	}{
		"passes": {`"test -f feature.txt"`, 0},
		"fails":  {`"test ! -f feature.txt"`, 1},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			r, q := fixtureFiles(t, agent{out: `{"summary":"ok"}`}, project(tc.verify))
			r.AgentHome = t.TempDir()
			if _, err := r.RunOne(t.Context()); err != nil {
				t.Fatal(err)
			}
			c := q.completed[0]
			if c.ExitReason != "done" || len(c.Findings) != tc.want {
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
