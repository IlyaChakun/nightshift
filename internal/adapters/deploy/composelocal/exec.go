package composelocal

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
)

// OSExec runs programs on the host with a small environment (the search path, the home
// directory and Docker's own settings) plus env. The deployer's other variables are secrets of
// the factory and never reach a recipe or a check.
type OSExec struct{}

var _ Exec = OSExec{}

// Run implements Exec. Standard error is added to the output on failure so the reason reaches the
// caller; a successful run returns standard output only, which is what the callers parse.
func (OSExec) Run(ctx context.Context, dir string, env []string, name string, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, name, args...) //nolint:gosec // G204: the adapter builds the arguments; names are validated by project()
	cmd.Dir = dir
	cmd.Env = append(passEnv(), env...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	if err := cmd.Run(); err != nil {
		return append(stdout.Bytes(), stderr.Bytes()...), fmt.Errorf("%s: %w", name, err)
	}
	return stdout.Bytes(), nil
}

func passEnv() []string {
	var out []string
	for _, kv := range os.Environ() { //nolint:forbidigo // the child gets only the names matched below
		k, _, _ := strings.Cut(kv, "=")
		switch {
		case k == "PATH", k == "HOME", k == "TMPDIR", k == "LANG", strings.HasPrefix(k, "DOCKER_"), strings.HasPrefix(k, "XDG_"):
			out = append(out, kv)
		}
	}
	return out
}
