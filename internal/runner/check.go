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

// projectVerify returns `verify` from .nightshift/project.yaml of the fresh checkout, or an
// empty string when the project has none or the runner cannot drop to the agent user.
func (r *Runner) projectVerify(workspace string) string {
	if r.AgentHome == "" {
		return ""
	}
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
		Verify string `yaml:"verify"`
	}
	if yaml.Unmarshal([]byte(doc), &p) != nil {
		return ""
	}
	return p.Verify
}

// runChecks runs the verify command in the changed checkout as the agent user and returns
// the findings that block, as the jobs API expects them. Reports stay in the job's artifacts.
func (r *Runner) runChecks(ctx context.Context, claim jobsapi.ClaimResponse, workspace, verify string) ([]json.RawMessage, error) {
	if verify == "" {
		return nil, nil
	}
	if !safeSegment(claim.JobID) {
		return nil, errors.New("runner: unsafe job id")
	}
	out := filepath.Join(r.ArtifactDir, claim.JobID, "check")
	if err := os.MkdirAll(out, 0o750); err != nil {
		return nil, fmt.Errorf("runner: %w", err)
	}
	if err := r.giveToAgent(out); err != nil {
		return nil, err
	}
	r.event(ctx, claim, "check.started")
	opts := check.Options{Dir: workspace, Out: out, Verify: verify, Timeout: checkTimeout}
	if r.AgentUID != 0 {
		opts.Agent = &check.Agent{UID: uint32(r.AgentUID), GID: uint32(r.AgentGID), Home: r.AgentHome} //nolint:gosec // ids come from the worker's own configuration
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
