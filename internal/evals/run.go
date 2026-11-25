package evals

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"go.yaml.in/yaml/v3"

	"github.com/ilyachakun/nightshift/internal/harness"
)

// PatchFile is the file a replaying harness leaves in the workspace; Run applies it.
const PatchFile = "fake.patch"

// Task is one polygon task: a directory with task.yaml, the starting files in repo/ and the
// hidden tests in hidden/ (design section 10, "The polygon").
type Task struct {
	ID     string `yaml:"id"`
	Set    string `yaml:"set"`
	Level  string `yaml:"level"`
	Role   string `yaml:"role"`
	Prompt string `yaml:"prompt"`
	// Test is the command that grades the result. It runs through `sh -c` in the changed
	// checkout after the hidden files are copied in.
	Test    string        `yaml:"test"`
	Timeout time.Duration `yaml:"timeout"`
	Dir     string        `yaml:"-"`
}

// LoadTask reads <dir>/task.yaml.
func LoadTask(dir string) (Task, error) {
	raw, err := os.ReadFile(filepath.Join(dir, "task.yaml")) //nolint:gosec // G304: the task directory is the caller's choice
	if err != nil {
		return Task{}, fmt.Errorf("evals: %w", err)
	}
	var t Task
	if err := yaml.Unmarshal(raw, &t); err != nil {
		return Task{}, fmt.Errorf("evals: %s: %w", dir, err)
	}
	if t.ID == "" || t.Prompt == "" || t.Test == "" {
		return Task{}, fmt.Errorf("evals: %s: id, prompt and test are required", dir)
	}
	if t.Set == "" {
		t.Set = "fast"
	}
	if t.Level == "" {
		t.Level = "E2"
	}
	if t.Role == "" {
		t.Role = "builder"
	}
	if t.Timeout <= 0 {
		t.Timeout = 5 * time.Minute
	}
	t.Dir = dir
	return t, nil
}

// Result is one run, in the shape of schemas/eval-result.json.
type Result struct {
	TaskID    string   `json:"task_id"`
	Set       string   `json:"set"`
	Level     string   `json:"level"`
	Role      string   `json:"role,omitempty"`
	Candidate string   `json:"candidate"`
	Attempt   int      `json:"attempt,omitempty"`
	Passed    bool     `json:"passed"`
	Score     float64  `json:"score"`
	Graders   []Grader `json:"graders,omitempty"`
	Usage     Usage    `json:"usage"`
	CacheKey  string   `json:"cache_key"`
}

// Grader is one check of a run.
type Grader struct {
	Kind   string `json:"kind"`
	Name   string `json:"name"`
	Passed *bool  `json:"passed"`
}

// Usage is what the run cost.
type Usage struct {
	Tokens    int     `json:"tokens"`
	CostUSD   float64 `json:"cost_usd"`
	DurationS float64 `json:"duration_s"`
}

// Runner solves tasks locally: each task gets a throwaway git repository, so no remote, no
// GitHub and no book is involved. The repository is removed afterwards. Moving the polygon to
// a hosted repository later only changes where the starting files come from (ADR 0011).
type Runner struct {
	Harness   harness.Harness
	Candidate string // names what is under test, for example a model or a skill set
	Schema    json.RawMessage
	// Work is where checkouts are made; empty means the system temp directory.
	Work string
}

// Run solves one task and grades it. A harness error is a failed run, not an error of Run:
// the polygon measures the factory, so a crash counts against the candidate.
func (r Runner) Run(ctx context.Context, t Task) (Result, error) {
	key, err := cacheKey(t, r.Candidate)
	if err != nil {
		return Result{}, err
	}
	res := Result{TaskID: t.ID, Set: t.Set, Level: t.Level, Role: t.Role, Candidate: r.Candidate, CacheKey: key, Attempt: 1}
	work, err := os.MkdirTemp(r.Work, "eval-*")
	if err != nil {
		return res, fmt.Errorf("evals: %w", err)
	}
	defer func() { _ = os.RemoveAll(work) }()
	if err := copyTree(filepath.Join(t.Dir, "repo"), work); err != nil {
		return res, err
	}
	for _, args := range [][]string{{"init", "-q", "-b", "main"}, {"add", "-A"}, {"-c", "user.email=eval@example.com", "-c", "user.name=eval", "commit", "-q", "--allow-empty", "-m", "task"}} {
		if _, err := git(ctx, work, args...); err != nil {
			return res, err
		}
	}
	start := time.Now()
	out, runErr := r.Harness.Run(ctx, harness.RunRequest{
		Role:         harness.RoleConfig{Name: t.Role},
		Workspace:    work,
		Prompt:       t.Prompt,
		OutputSchema: r.Schema,
	})
	res.Usage = Usage{Tokens: out.Usage.InputTokens + out.Usage.OutputTokens, CostUSD: out.Usage.CostUSD}
	solved := runErr == nil && r.applyPatch(ctx, work) == nil
	pass := false
	if solved {
		// Hidden tests come in after the agent is done, so it cannot read or edit them.
		if err := copyTree(filepath.Join(t.Dir, "hidden"), work); err != nil {
			return res, err
		}
		pass = runTest(ctx, work, t)
	}
	res.Usage.DurationS = time.Since(start).Seconds()
	res.Passed = pass
	if pass {
		res.Score = 1
	}
	res.Graders = []Grader{{Kind: "code", Name: "tests", Passed: &pass}}
	return res, nil
}

// applyPatch applies the patch a replaying harness leaves; real harnesses edit files in place.
func (r Runner) applyPatch(ctx context.Context, work string) error {
	p := filepath.Join(work, PatchFile)
	if _, err := os.Stat(p); err != nil {
		return nil //nolint:nilerr // no patch file means the agent edited the files itself
	}
	if _, err := git(ctx, work, "apply", PatchFile); err != nil {
		return err
	}
	return os.Remove(p)
}

func runTest(ctx context.Context, dir string, t Task) bool {
	ctx, cancel := context.WithTimeout(ctx, t.Timeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, "sh", "-c", t.Test) //nolint:gosec // G204: the command comes from the task file in the repository
	cmd.Dir = dir
	var b bytes.Buffer
	cmd.Stdout, cmd.Stderr = &b, &b
	return cmd.Run() == nil
}

func git(ctx context.Context, dir string, args ...string) (string, error) {
	full := append([]string{"-C", dir, "-c", "core.hooksPath=/dev/null", "-c", "core.fsmonitor=false"}, args...)
	cmd := exec.CommandContext(ctx, "git", full...) //nolint:gosec // G204: a fixed program; the arguments are built here
	var out, errb bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &errb
	cmd.Env = []string{"PATH=" + os.Getenv("PATH"), "HOME=/nonexistent", "GIT_CONFIG_GLOBAL=/dev/null", "GIT_CONFIG_NOSYSTEM=1"} //nolint:forbidigo // git needs the PATH, nothing else
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("evals: git %s: %w: %s", args[0], err, strings.TrimSpace(errb.String()))
	}
	return out.String(), nil
}

// copyTree copies the regular files of src into dst. A missing src copies nothing; a link or
// special file is an error, so a task cannot reach outside its directory.
func copyTree(src, dst string) error {
	if _, err := os.Stat(src); errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	root := os.DirFS(src)
	return fs.WalkDir(root, ".", func(name string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		to := filepath.Join(dst, filepath.FromSlash(name))
		if d.IsDir() {
			return os.MkdirAll(to, 0o750)
		}
		if !d.Type().IsRegular() {
			return fmt.Errorf("evals: %s/%s is not a regular file", src, name)
		}
		data, err := fs.ReadFile(root, name)
		if err != nil {
			return err
		}
		return os.WriteFile(to, data, 0o600)
	})
}

// cacheKey hashes the task files and the candidate: the same pair gives the same key.
func cacheKey(t Task, candidate string) (string, error) {
	h := sha256.New()
	_, _ = h.Write([]byte(candidate + "\x00"))
	root := os.DirFS(t.Dir)
	err := fs.WalkDir(root, ".", func(name string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		data, err := fs.ReadFile(root, name)
		if err != nil {
			return err
		}
		_, _ = fmt.Fprintf(h, "%s\x00%d\x00", name, len(data))
		_, _ = h.Write(data)
		return nil
	})
	if err != nil {
		return "", fmt.Errorf("evals: %w", err)
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}
