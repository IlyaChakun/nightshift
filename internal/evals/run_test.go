package evals

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/ilyachakun/nightshift/internal/harness"
)

// editor is a harness that edits one file, as a real agent would.
type editor struct {
	file, body string
	seenHidden bool
}

func (e *editor) Run(_ context.Context, req harness.RunRequest) (harness.RunResult, error) {
	if _, err := os.Stat(filepath.Join(req.Workspace, "hidden_test.sh")); err == nil {
		e.seenHidden = true
	}
	return harness.RunResult{}, os.WriteFile(filepath.Join(req.Workspace, e.file), []byte(e.body), 0o600)
}

func task(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	for name, body := range map[string]string{
		"task.yaml":             "id: greet\nprompt: make greeting.txt say hello\ntest: sh hidden_test.sh\n",
		"repo/README.md":        "start\n",
		"hidden/hidden_test.sh": "grep -qx hello greeting.txt\n",
	} {
		p := filepath.Join(dir, name)
		if err := os.MkdirAll(filepath.Dir(p), 0o750); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

func TestARunPassesOnlyWhenTheHiddenTestsPass(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("no git")
	}
	for _, tc := range []struct {
		name, body string
		want       bool
	}{{"solved", "hello\n", true}, {"wrong", "bye\n", false}} {
		t.Run(tc.name, func(t *testing.T) {
			tk, err := LoadTask(task(t))
			if err != nil {
				t.Fatal(err)
			}
			h := &editor{file: "greeting.txt", body: tc.body}
			work := t.TempDir()
			res, err := Runner{Harness: h, Candidate: "c", Work: work}.Run(t.Context(), tk)
			if err != nil {
				t.Fatal(err)
			}
			if res.Passed != tc.want || (res.Score == 1) != tc.want || len(res.CacheKey) != 64 {
				t.Fatalf("result %+v", res)
			}
			if h.seenHidden {
				t.Fatal("the agent saw the hidden tests")
			}
			if left, _ := os.ReadDir(work); len(left) != 0 {
				t.Fatalf("the checkout was not discarded: %v", left)
			}
		})
	}
}

func TestALinkInATaskIsRefused(t *testing.T) {
	dir := task(t)
	if err := os.Symlink("/etc/passwd", filepath.Join(dir, "repo", "link")); err != nil {
		t.Skip(err)
	}
	tk, _ := LoadTask(dir)
	if _, err := (Runner{Harness: &editor{file: "x", body: "x"}}).Run(t.Context(), tk); err == nil {
		t.Fatal("a symlink must be refused")
	}
}

func TestTheCacheKeyFollowsTheTaskAndTheCandidate(t *testing.T) {
	tk, _ := LoadTask(task(t))
	a, _ := cacheKey(tk, "one")
	b, _ := cacheKey(tk, "two")
	if a == b {
		t.Fatal("the candidate must change the key")
	}
	if err := os.WriteFile(filepath.Join(tk.Dir, "repo", "README.md"), []byte("changed\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if c, _ := cacheKey(tk, "one"); c == a {
		t.Fatal("the task files must change the key")
	}
}

func TestAnIdThatIsNotAPlainNameIsRefused(t *testing.T) {
	dir := task(t)
	if err := os.WriteFile(filepath.Join(dir, "task.yaml"), []byte("id: ../x\nprompt: p\ntest: t\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadTask(dir); err == nil {
		t.Fatal("a task id with a path in it must be refused")
	}
}

// linker leaves a link where the hidden test will land, as a hostile agent would.
type linker struct{ target string }

func (l linker) Run(_ context.Context, req harness.RunRequest) (harness.RunResult, error) {
	return harness.RunResult{}, os.Symlink(l.target, filepath.Join(req.Workspace, "hidden_test.sh"))
}

func TestALinkLeftByTheAgentDoesNotRedirectTheHiddenCopy(t *testing.T) {
	victim := filepath.Join(t.TempDir(), "victim")
	if err := os.WriteFile(victim, []byte("keep\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	tk, _ := LoadTask(task(t))
	if _, err := (Runner{Harness: linker{victim}}).Run(t.Context(), tk); err != nil {
		t.Fatal(err)
	}
	got, _ := os.ReadFile(victim) //nolint:gosec // G304: a path made by this test
	if string(got) != "keep\n" {
		t.Fatalf("the file behind the link was overwritten: %q", got)
	}
}
