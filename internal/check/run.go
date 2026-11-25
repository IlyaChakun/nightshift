package check

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"syscall"
	"time"
)

// Tool is one check. Its command runs in the worktree through sh and writes a SARIF log
// to the path in $NIGHTSHIFT_SARIF. A tool that finds nothing may write nothing.
type Tool struct {
	Name     string
	Command  string
	Category string // finding category, see schemas/finding.json
}

// Options of one run.
type Options struct {
	Dir      string    // the worktree
	Out      string    // where logs and SARIF go (the job's artifacts directory)
	Verify   string    // the project's own verify command; empty skips it
	Tools    []Tool    // factory tools
	Baseline []Finding // findings of main; only others block
	Timeout  time.Duration
	// Docs are the document kinds to lint (doc-lint); nil lints none.
	Docs []DocKind
	// Agent runs the commands as this user and group, with Home as HOME, so repository code
	// never runs with the runner's rights. Nil runs them as the caller.
	Agent *Agent
}

// Agent is the unprivileged user that runs a project's commands.
type Agent struct {
	UID, GID uint32
	Home     string
	Env      []string // extra variables, for example the egress proxy
}

// Report is the result of a run. New holds what blocks or warns the author: findings the
// baseline lacks, plus every failure of a command that produced no report.
type Report struct {
	All []Finding
	New []Finding
}

var safeName = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]*$`)

// maxTail is how much of a failing command's output goes into a finding.
const maxTail = 1500

// Run runs the verify command and every tool. A command that cannot start is an error;
// a command that fails is a finding, so the author sees it and can fix it.
func Run(ctx context.Context, o Options) (Report, error) {
	if err := os.MkdirAll(o.Out, 0o750); err != nil {
		return Report{}, fmt.Errorf("check: %w", err)
	}
	var all, failures []Finding
	steps := o.Tools
	if o.Verify != "" {
		steps = append([]Tool{{Name: "verify", Command: o.Verify, Category: "code"}}, steps...)
	}
	for _, t := range steps {
		if !safeName.MatchString(t.Name) {
			return Report{}, fmt.Errorf("check: tool name %q must match %s", t.Name, safeName)
		}
		found, failed, err := runTool(ctx, o, t)
		if err != nil {
			return Report{}, err
		}
		all = append(all, found...)
		failures = append(failures, failed...)
	}
	docs, err := DocFindings(o.Dir, o.Docs)
	if err != nil {
		return Report{}, err
	}
	all = append(all, docs...)
	return Report{All: append(all, failures...), New: append(NewViolations(o.Baseline, all), failures...)}, nil
}

func runTool(ctx context.Context, o Options, t Tool) (found, failed []Finding, err error) {
	if o.Timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, o.Timeout)
		defer cancel()
	}
	sarif := filepath.Join(o.Out, t.Name+".sarif")
	if err := os.Remove(sarif); err != nil && !errors.Is(err, os.ErrNotExist) {
		return nil, nil, fmt.Errorf("check: %w", err)
	}
	var out bytes.Buffer
	cmd := exec.CommandContext(ctx, "sh", "-c", t.Command) //nolint:gosec // G204: the command comes from the runner, not from the agent
	cmd.Dir, cmd.Stdout, cmd.Stderr = o.Dir, &out, &out
	cmd.WaitDelay = time.Second                                                                               // a child that keeps the pipe open must not outlive the timeout
	cmd.Env = []string{"PATH=" + os.Getenv("PATH"), "HOME=" + os.Getenv("HOME"), "NIGHTSHIFT_SARIF=" + sarif} //nolint:forbidigo // the tool needs the runner's PATH and HOME, nothing else
	if o.Agent != nil {
		cmd.SysProcAttr = &syscall.SysProcAttr{Credential: &syscall.Credential{Uid: o.Agent.UID, Gid: o.Agent.GID}}
		cmd.Env = []string{"PATH=" + os.Getenv("PATH"), "HOME=" + o.Agent.Home, "NIGHTSHIFT_SARIF=" + sarif} //nolint:forbidigo // the tool needs the runner's PATH, nothing else
		cmd.Env = append(cmd.Env, o.Agent.Env...)
	}
	runErr := cmd.Run()
	if werr := os.WriteFile(filepath.Join(o.Out, t.Name+".log"), out.Bytes(), 0o600); werr != nil {
		return nil, nil, fmt.Errorf("check: %w", werr)
	}
	if f, perr := os.Open(sarif); perr == nil { //nolint:gosec // G304: the path is built from a validated tool name
		defer func() { _ = f.Close() }()
		found, err = ParseSARIF(f, t.Category, "L1")
		if err != nil {
			return nil, []Finding{failure(t, "wrote an invalid report: "+err.Error(), out.Bytes())}, nil
		}
		return found, nil, nil
	}
	if runErr != nil {
		return nil, []Finding{failure(t, "failed: "+runErr.Error(), out.Bytes())}, nil
	}
	return nil, nil, nil
}

func failure(t Tool, what string, output []byte) Finding {
	tail := strings.TrimSpace(string(output))
	if len(tail) > maxTail {
		tail = "..." + tail[len(tail)-maxTail:]
	}
	f := Finding{
		RuleID: t.Name + ".failed", Category: t.Category, Severity: Block, LayerCaught: "L1",
		Message: t.Name + " " + what,
	}
	if tail != "" {
		f.Message += "\n" + tail
	}
	f.Message = truncate(strings.ToValidUTF8(f.Message, ""), maxMessage)
	f.Fingerprint = fingerprint(f, nil)
	return f
}
