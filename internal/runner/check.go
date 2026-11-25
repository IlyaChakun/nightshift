package runner

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"go.yaml.in/yaml/v3"

	"github.com/ilyachakun/nightshift/internal/check"
	"github.com/ilyachakun/nightshift/internal/jobsapi"
)

// checkTimeout bounds the project's verify command.
const checkTimeout = 15 * time.Minute

// projectChecks returns `verify` from .nightshift/project.yaml of the fresh checkout and the
// stack packs' tools for the project's stacks. Both are empty when the project has no file or
// the runner cannot drop to the agent user. Tools whose executable the worker lacks are named
// in skipped.
func (r *Runner) projectChecks(workspace string) (verify string, tools []check.Tool, skipped []string) {
	if r.AgentHome == "" {
		return "", nil, nil
	}
	root, err := os.OpenRoot(workspace)
	if err != nil {
		return "", nil, nil
	}
	defer func() { _ = root.Close() }()
	doc, err := readInRoot(root, ".nightshift/project.yaml")
	if err != nil {
		return "", nil, nil
	}
	var p struct {
		Stack  []string `yaml:"stack"`
		Verify string   `yaml:"verify"`
	}
	if yaml.Unmarshal([]byte(doc), &p) != nil {
		return "", nil, nil
	}
	tools, skipped = r.Stacks.ToolsWith(p.Stack, r.hasExecutable)
	return p.Verify, tools, skipped
}

// runChecks runs the verify command in the changed checkout as the agent user and returns
// the findings that block, as the jobs API expects them. Reports stay in the job's artifacts.
func (r *Runner) runChecks(ctx context.Context, claim jobsapi.ClaimResponse, workspace, verify string, tools []check.Tool) ([]json.RawMessage, error) {
	if verify == "" && len(tools) == 0 {
		// A patch that no check looked at must not pass for a checked one: say so in the job's
		// events and the logs, where the Verifier and the owner can see it.
		r.log().WarnContext(ctx, "nothing to check: the project has no verify command and no stack tool ran", "job", claim.JobID)
		r.event(ctx, claim, "check.nothing_to_run")
	}
	if !safeSegment(claim.JobID) {
		return nil, errors.New("runner: unsafe job id")
	}
	out := filepath.Join(r.ArtifactDir, claim.JobID, "check")
	if err := r.reportDir(out); err != nil {
		return nil, err
	}
	r.event(ctx, claim, "check.started")
	opts := check.Options{Dir: workspace, Out: out, Verify: verify, Tools: tools, Docs: check.DefaultDocKinds, Timeout: checkTimeout}
	if r.AgentUID != 0 {
		opts.Agent = &check.Agent{UID: uint32(r.AgentUID), GID: uint32(r.AgentGID), Home: r.AgentHome, Env: r.toolEnv()} //nolint:gosec // ids come from the worker's own configuration
	}
	rep, err := check.Run(ctx, opts)
	if err != nil {
		return nil, fmt.Errorf("runner: check: %w", err)
	}
	r.event(ctx, claim, "check.finished")
	var found []json.RawMessage
	for _, f := range rep.New {
		b, err := json.Marshal(f)
		if err != nil {
			return nil, fmt.Errorf("runner: %w", err)
		}
		found = append(found, b)
	}
	return found, nil
}

// reportDir creates the directory for check reports. The tools write there as the agent user
// and the runner writes the logs, so it is shared and sticky: neither can replace the other's
// files, and the kernel does not follow another user's symlink in it.
func (r *Runner) reportDir(dir string) error {
	if err := os.MkdirAll(filepath.Dir(dir), 0o751); err != nil { //nolint:gosec // see writeArtifact
		return fmt.Errorf("runner: %w", err)
	}
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return fmt.Errorf("runner: %w", err)
	}
	if r.AgentUID == 0 {
		return nil
	}
	if err := os.Chmod(dir, 0o777|os.ModeSticky); err != nil {
		return fmt.Errorf("runner: %w", err)
	}
	return nil
}

// toolEnv is the part of the worker environment project commands need: the egress proxy,
// without which a package download cannot work. Credentials never go to them.
func (r *Runner) toolEnv() []string {
	var env []string
	if dir := r.shimsDir(); dir != "" {
		env = append(env, "PATH="+dir+":"+os.Getenv("PATH")) //nolint:forbidigo // the checks need the worker's PATH, with the shims first
	}
	for _, k := range []string{"HTTPS_PROXY", "NO_PROXY", "GOPROXY"} {
		if v := r.Env[k]; v != "" {
			env = append(env, k+"="+v)
		}
	}
	return env
}
