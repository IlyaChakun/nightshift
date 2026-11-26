package composelocal

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
)

// OSExec runs programs on the host, with the caller's environment plus env.
type OSExec struct{}

var _ Exec = OSExec{}

// Run implements Exec. Standard error is added to the output on failure so the reason reaches the
// caller; a successful run returns standard output only, which is what the callers parse.
func (OSExec) Run(ctx context.Context, dir string, env []string, name string, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, name, args...) //nolint:gosec // G204: the adapter builds the arguments; names are validated by project()
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), env...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	if err := cmd.Run(); err != nil {
		return append(stdout.Bytes(), stderr.Bytes()...), fmt.Errorf("%s: %w", name, err)
	}
	return stdout.Bytes(), nil
}
