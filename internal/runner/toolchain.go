package runner

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"go.yaml.in/yaml/v3"

	"github.com/ilyachakun/nightshift/internal/jobsapi"
)

// setupTail is how much of a failing command's output goes into the error.
const setupTail = 1500

// setupTimeout bounds the toolchain install and the project's setup command together.
const setupTimeout = 20 * time.Minute

// toolchainFiles are the files that pin a project's toolchain for mise (design section 5).
var toolchainFiles = []string{"mise.toml", ".mise.toml", ".tool-versions"}

// shimsDir is where mise puts the executables of the installed tools. The agent home volume
// keeps them, so a second job with the same versions installs nothing.
func (r *Runner) shimsDir(home string) string {
	if home == "" {
		return ""
	}
	dir := filepath.Join(home, ".local", "share", "mise", "shims")
	if st, err := os.Stat(dir); err != nil || !st.IsDir() {
		return ""
	}
	return dir
}

// withShims puts the mise shims first in PATH, so the agent, verify and the stack tools run
// the project's own tool versions.
func (r *Runner) withShims(env map[string]string, home string) {
	if dir := r.shimsDir(home); dir != "" {
		env["PATH"] = dir + ":" + os.Getenv("PATH") //nolint:forbidigo // the tools need the worker's PATH, with the shims first
	}
}

// prepareProject creates the tenant's HOME, installs the toolchain the project pins and runs its `setup` command, as the
// agent user and before the agent starts, so the agent and the checks see the same environment.
// A worker that cannot drop to the agent user never runs repository commands.
func (r *Runner) prepareProject(ctx context.Context, claim jobsapi.ClaimResponse, workspace string) error {
	home := r.homeFor(claim.Job.Tenant)
	if home == "" {
		return nil
	}
	// The agent's own CLI state lives here too, so the directory exists whether or not the project has a setup.
	if err := r.prepareHome(ctx, home); err != nil {
		return err
	}
	setup := projectSetup(workspace)
	pinned := hasToolchainFile(workspace)
	if !pinned && setup == "" {
		return nil
	}
	ctx, cancel := context.WithTimeout(ctx, setupTimeout)
	defer cancel()
	r.event(ctx, claim, "setup.started")
	useMise := false
	if pinned {
		if _, err := exec.LookPath("mise"); err != nil {
			r.log().WarnContext(ctx, "the project pins a toolchain but the worker has no mise", "job", claim.JobID)
			r.event(ctx, claim, "setup.mise_missing")
		} else {
			useMise = true
			if err := r.installTools(ctx, claim, home, workspace); err != nil {
				return err
			}
		}
	}
	if setup != "" {
		args := []string{"sh", "-c", setup}
		if useMise {
			args = append([]string{"mise", "exec", "--"}, args...)
		}
		if out, err := r.projectCmd(ctx, home, workspace, args[0], args[1:]...); err != nil {
			return fmt.Errorf("runner: setup failed: %w\n%s", err, tail(out))
		}
	}
	r.restoreToolLocks(ctx, home, workspace)
	r.event(ctx, claim, "setup.finished")
	return nil
}

// toolLocks are the files `mise install` may write. The install and the setup command are the
// factory's own steps, so what they leave there is not the agent's change; a protected-path
// check on the patch would otherwise reject Builder's first patch for it.
var toolLocks = []string{"mise.lock", ".mise.lock", "mise.local.lock"}

// restoreToolLocks puts the tool lockfiles back as the commit has them (a tracked file is
// checked out again, a file the install created is removed), before the agent starts.
func (r *Runner) restoreToolLocks(ctx context.Context, home, workspace string) {
	for _, f := range toolLocks {
		var args []string
		if _, err := r.projectCmd(ctx, home, workspace, "git", "ls-files", "--error-unmatch", "--", f); err == nil {
			args = []string{"git", "checkout", "--", f} // also brings back a file the install deleted
		} else if _, err := os.Lstat(filepath.Join(workspace, f)); err == nil {
			args = []string{"rm", "-f", "--", f}
		} else {
			continue
		}
		if out, err := r.projectCmd(ctx, home, workspace, args[0], args[1:]...); err != nil {
			r.log().WarnContext(ctx, "tool lockfile not restored", "file", f, "error", err, "output", tail(out))
		}
	}
}

// installTools asks the toolchain service when there is one, and otherwise runs mise here.
func (r *Runner) installTools(ctx context.Context, claim jobsapi.ClaimResponse, home, workspace string) error {
	if r.Installer != nil {
		if out, err := r.Installer.Install(ctx, claim.JobID, workspace); err != nil {
			return fmt.Errorf("runner: toolchain install failed: %w\n%s", err, tail([]byte(out)))
		}
		return nil
	}
	if out, err := r.projectCmd(ctx, home, workspace, "mise", "install", "--yes"); err != nil {
		return fmt.Errorf("runner: mise install failed: %w\n%s", err, tail(out))
	}
	return nil
}

// projectCmd runs one command in the workspace as the agent user.
func (r *Runner) projectCmd(ctx context.Context, home, dir, name string, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, name, args...) //nolint:gosec // G204: the command is the project's own setup, run as the unprivileged agent user
	var out bytes.Buffer
	cmd.Dir, cmd.Stdout, cmd.Stderr = dir, &out, &out
	cmd.WaitDelay = time.Second
	env := map[string]string{
		"PATH": os.Getenv("PATH"), //nolint:forbidigo // the tool needs the worker's PATH, nothing else
		"HOME": home, "MISE_YES": "1", "MISE_TRUSTED_CONFIG_PATHS": dir,
	}
	r.withShims(env, home)
	for _, kv := range r.toolEnv(home) {
		if k, v, ok := strings.Cut(kv, "="); ok {
			env[k] = v
		}
	}
	for k, v := range env {
		cmd.Env = append(cmd.Env, k+"="+v)
	}
	if r.AgentUID != 0 {
		cmd.SysProcAttr = &syscall.SysProcAttr{Credential: &syscall.Credential{Uid: uint32(r.AgentUID), Gid: uint32(r.AgentGID)}} //nolint:gosec // ids come from the worker's own configuration
	}
	return out.Bytes(), cmd.Run()
}

func hasToolchainFile(workspace string) bool {
	for _, f := range toolchainFiles {
		if _, err := os.Stat(filepath.Join(workspace, f)); err == nil {
			return true
		}
	}
	return false
}

// projectSetup reads `setup` from .nightshift/project.yaml of the fresh checkout.
func projectSetup(workspace string) string {
	root, err := os.OpenRoot(workspace)
	if err != nil {
		return ""
	}
	defer func() { _ = root.Close() }()
	doc, err := readInRoot(root, ".nightshift/project.yaml")
	if err != nil {
		return ""
	}
	var p struct {
		Setup string `yaml:"setup"`
	}
	if yaml.Unmarshal([]byte(doc), &p) != nil {
		return ""
	}
	return strings.TrimSpace(p.Setup)
}

func tail(out []byte) string {
	s := strings.TrimSpace(string(out))
	if len(s) > setupTail {
		s = "..." + s[len(s)-setupTail:]
	}
	return s
}

// hasExecutable reports whether the worker can run name: in the mise shims or on PATH.
func (r *Runner) hasExecutable(home, name string) bool {
	dirs := filepath.SplitList(os.Getenv("PATH")) //nolint:forbidigo // the worker's own PATH decides what it can run
	if d := r.shimsDir(home); d != "" {
		dirs = append([]string{d}, dirs...)
	}
	for _, d := range dirs {
		st, err := os.Stat(filepath.Join(d, name)) //nolint:gosec // the name comes from pack files and is only probed, never run
		if err == nil && !st.IsDir() && st.Mode()&0o111 != 0 {
			return true
		}
	}
	return false
}

// HomeFor is the HOME of the agent user for one tenant: a directory of its own under the agent
// home volume, so a cache, a session file or a tool one tenant's job leaves is not readable by
// another tenant's job (M3, design section 17). Empty when there is no agent home or the tenant is
// not a plain name.
func HomeFor(base, tenant string) string {
	if base == "" || !safeSegment(tenant) {
		return ""
	}
	return filepath.Join(base, "tenants", tenant)
}

func (r *Runner) homeFor(tenant string) string { return HomeFor(r.AgentHome, tenant) }

// prepareHome creates the tenant's HOME as the agent user; an empty home needs nothing.
func (r *Runner) prepareHome(ctx context.Context, home string) error {
	if home == "" {
		return nil
	}
	return r.asAgent(ctx, "mkdir", "-p", "--", home)
}
