package runner_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ilyachakun/nightshift/internal/harness"
)

func write(t *testing.T, path, text string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(text), 0o600); err != nil {
		t.Fatal(err)
	}
}

// TestRoleContextIsAssembled checks the system file and the skills directory the agent gets,
// and that nothing is left in the temporary directory afterwards.
func TestRoleContextIsAssembled(t *testing.T) {
	t.Parallel()
	var req harness.RunRequest
	var system, skill string
	var mode os.FileMode
	r, q := fixture(t, agent{out: `{}`, peek: func(rr harness.RunRequest) {
		req = rr
		if info, err := os.Stat(filepath.Dir(rr.SystemFile)); err == nil {
			mode = info.Mode().Perm()
		}
		b, _ := os.ReadFile(rr.SystemFile)
		system = string(b)
		b, _ = os.ReadFile(filepath.Join(rr.AddDirs[0], ".claude", "skills", "own-audit", "SKILL.md"))
		skill = string(b)
	}})
	r.RolesDir, r.SkillsDir = t.TempDir(), t.TempDir()
	write(t, filepath.Join(r.RolesDir, "builder.md"), "# Builder\nWrite tests first.\n")
	write(t, filepath.Join(r.SkillsDir, "own", "audit", "SKILL.md"), "audit skill")
	q.next.Job.Instructions = []string{"a.txt"} // the fixture repository has this file
	q.next.Job.Checklist = []string{"wrap errors with context", "no\nnewline"}
	q.next.Job.Skills = []string{"own/audit"}

	if _, err := r.RunOne(t.Context()); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"# Builder\nWrite tests first.", "# Project instructions\n\n## a.txt\n\none", "- wrap errors with context", "- no newline"} {
		if !strings.Contains(system, want) {
			t.Errorf("system file lacks %q:\n%s", want, system)
		}
	}
	if mode != 0o755 {
		t.Errorf("the agent user cannot read the job directory: %v", mode)
	}
	if skill != "audit skill" {
		t.Errorf("skill: %q", skill)
	}
	if !strings.HasSuffix(req.Prompt, "Skills you can use: own/audit.") || !strings.HasPrefix(req.Prompt, "add a feature") {
		t.Errorf("prompt: %q", req.Prompt)
	}
	if _, err := os.Stat(req.SystemFile); !os.IsNotExist(err) {
		t.Errorf("the job directory was not removed: %v", err)
	}
}

func TestRoleContextFailsClosed(t *testing.T) {
	t.Parallel()
	for name, job := range map[string]func(*testing.T, *jobs){
		"instruction file missing": func(_ *testing.T, q *jobs) { q.next.Job.Instructions = []string{"nope.md"} },
		"instruction escapes":      func(_ *testing.T, q *jobs) { q.next.Job.Instructions = []string{"../../etc/passwd"} },
		"bad skill id":             func(_ *testing.T, q *jobs) { q.next.Job.Skills = []string{"../x/y"} },
		"unknown skill":            func(_ *testing.T, q *jobs) { q.next.Job.Skills = []string{"own/missing"} },
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			r, q := fixture(t, agent{out: `{}`})
			r.SkillsDir = t.TempDir()
			job(t, q)
			if _, err := r.RunOne(t.Context()); err != nil {
				t.Fatal(err)
			}
			if len(q.completed) != 1 || q.completed[0].ExitReason != "error" {
				t.Fatalf("complete: %+v", q.completed)
			}
		})
	}
}
