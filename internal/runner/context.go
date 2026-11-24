package runner

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/ilyachakun/nightshift/internal/jobsapi"
)

// maxInstructions caps one instruction file; AGENTS.md is meant to be an index of ~100 lines.
const maxInstructions = 64 << 10

var skillID = regexp.MustCompile(`^[a-z0-9][a-z0-9-]*/[a-z0-9][a-z0-9-]*$`)

// Context is what the runner gives a role besides the task prompt (design section 12):
// one system file and a directory of skills, the same for every harness.
type Context struct {
	SystemFile string   // role instructions, project instructions and the L0 checklist
	SkillsDir  string   // contains .claude/skills/<set>-<name>/ for each selected skill
	Skills     []string // ids, to be listed in the prompt
	Dir        string   // the job's scratch directory; remove it when the job ends
}

// buildContext assembles the context of a job in a new temporary directory. Role
// instructions come from <RolesDir>/<role>.md (optional), project instructions from the
// worktree, skills from <SkillsDir>. The caller removes Context.Dir.
func (r *Runner) buildContext(job jobsapi.Job, workspace string) (Context, error) {
	dir, err := os.MkdirTemp("", "nightshift-job-")
	if err != nil {
		return Context{}, fmt.Errorf("runner: %w", err)
	}
	c := Context{Dir: dir, Skills: job.Skills}
	if err := r.fill(&c, job, workspace); err != nil {
		_ = os.RemoveAll(dir)
		return Context{}, err
	}
	return c, nil
}

func (r *Runner) fill(c *Context, job jobsapi.Job, workspace string) error {
	var b strings.Builder
	if r.RolesDir != "" && safeSegment(job.Role) {
		switch text, err := os.ReadFile(filepath.Join(r.RolesDir, job.Role+".md")); { //nolint:gosec // job.Role passed safeSegment
		case err == nil:
			b.WriteString(strings.TrimSpace(string(text)) + "\n\n")
		case !errors.Is(err, os.ErrNotExist):
			return fmt.Errorf("runner: role instructions: %w", err)
		}
	}
	if len(job.Instructions) > 0 {
		b.WriteString("# Project instructions\n\n")
		root, err := os.OpenRoot(workspace)
		if err != nil {
			return fmt.Errorf("runner: %w", err)
		}
		defer func() { _ = root.Close() }()
		for _, p := range job.Instructions {
			text, err := readInRoot(root, p)
			if err != nil {
				return fmt.Errorf("runner: project instructions %s: %w", p, err)
			}
			fmt.Fprintf(&b, "## %s\n\n%s\n\n", p, strings.TrimSpace(text))
		}
	}
	if len(job.Checklist) > 0 {
		b.WriteString("# Checklist for this task\n\n")
		for _, item := range job.Checklist {
			b.WriteString("- " + strings.ReplaceAll(item, "\n", " ") + "\n")
		}
	}
	if b.Len() > 0 {
		c.SystemFile = filepath.Join(c.Dir, "system.md")
		if err := os.WriteFile(c.SystemFile, []byte(b.String()), 0o644); err != nil { //nolint:gosec // the agent user must read it; it holds no secrets
			return fmt.Errorf("runner: %w", err)
		}
	}
	if len(job.Skills) > 0 {
		c.SkillsDir = filepath.Join(c.Dir, "skills")
		for _, id := range job.Skills {
			if err := r.stageSkill(c.SkillsDir, id); err != nil {
				return err
			}
		}
	}
	return nil
}

// readInRoot reads a file inside root; a path or a symlink that leaves it is an error.
func readInRoot(root *os.Root, path string) (string, error) {
	f, err := root.Open(path)
	if err != nil {
		return "", err
	}
	defer func() { _ = f.Close() }()
	data, err := io.ReadAll(io.LimitReader(f, maxInstructions+1))
	if err != nil {
		return "", err
	}
	if len(data) > maxInstructions {
		return "", fmt.Errorf("longer than %d bytes", maxInstructions)
	}
	return string(data), nil
}

// stageSkill copies one skill to <dir>/.claude/skills/<set>-<name>, where bare mode looks
// for skills. Own skills live in <SkillsDir>/own, the rest in <SkillsDir>/vendor.
func (r *Runner) stageSkill(dir, id string) error {
	if !skillID.MatchString(id) {
		return fmt.Errorf("runner: bad skill id %q", id)
	}
	set, name, _ := strings.Cut(id, "/")
	src := filepath.Join(r.SkillsDir, "vendor", set, name)
	if set == "own" {
		src = filepath.Join(r.SkillsDir, "own", name)
	}
	dst := filepath.Join(dir, ".claude", "skills", set+"-"+name)
	if err := os.MkdirAll(dst, 0o755); err != nil { //nolint:gosec // the agent user must read the skills
		return fmt.Errorf("runner: %w", err)
	}
	if err := os.CopyFS(dst, os.DirFS(src)); err != nil {
		return fmt.Errorf("runner: skill %s: %w", id, err)
	}
	return nil
}

// promptWithSkills lists the skills in the prompt, because bare mode does not tell the model
// which skills exist.
func promptWithSkills(prompt string, skills []string) string {
	if len(skills) == 0 {
		return prompt
	}
	return prompt + "\n\nSkills you can use: " + strings.Join(skills, ", ") + "."
}
