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
	"time"

	"github.com/ilyachakun/nightshift/internal/gitops"
	"github.com/ilyachakun/nightshift/internal/harness"
	"github.com/ilyachakun/nightshift/internal/jobsapi"
	"github.com/ilyachakun/nightshift/internal/runner"
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
	err error
	out string
}

func (a agent) Run(_ context.Context, req harness.RunRequest) (harness.RunResult, error) {
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
	up := t.TempDir()
	git(t, up, "init", "-q", "-b", "main")
	if err := os.WriteFile(filepath.Join(up, "a.txt"), []byte("one\n"), 0o600); err != nil {
		t.Fatal(err)
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
		Harness: func(string) (harness.Harness, error) { return h, nil },
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
