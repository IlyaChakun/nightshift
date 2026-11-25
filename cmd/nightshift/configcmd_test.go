package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func copyFile(t *testing.T, from, to string) {
	t.Helper()
	b, err := os.ReadFile(from) //nolint:gosec // test fixtures under testdata
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(to), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(to, b, 0o600); err != nil {
		t.Fatal(err)
	}
}

// instanceDir builds an instance directory from the synthetic personal instance and tenant.
func instanceDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	copyFile(t, "../../testdata/instances/personal/instance.yaml", filepath.Join(dir, "instance.yaml"))
	copyFile(t, "../../testdata/instances/personal/providers.yaml", filepath.Join(dir, "providers.yaml"))
	copyFile(t, "../../testdata/tenants/example-solo/tenant.yaml", filepath.Join(dir, "tenants/example-solo/tenant.yaml"))
	return dir
}

func TestConfigValidate(t *testing.T) {
	t.Parallel()
	good := instanceDir(t)
	broken := instanceDir(t)
	if err := os.WriteFile(filepath.Join(broken, "tenants/example-solo/tenant.yaml"), []byte("name: example-solo\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	project := t.TempDir()
	for _, f := range []string{"project.yaml", "blast-radius.yaml"} {
		copyFile(t, "../../testdata/tenants/example-solo/.nightshift/"+f, filepath.Join(project, ".nightshift", f))
	}
	copyFile(t, "../../testdata/tenants/example-solo/tenant.yaml", filepath.Join(project, "AGENTS.md"))
	if err := os.MkdirAll(filepath.Join(project, ".nightshift/skills"), 0o750); err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name     string
		args     []string
		wantCode int
		wantOut  string
	}{
		{name: "no subcommand", args: nil, wantCode: exitUsage},
		{name: "good instance", args: []string{"--config", good}, wantCode: exitOK, wantOut: "ok: instance personal, tenants [example-solo]"},
		{name: "unknown tenant", args: []string{"--config", good, "--tenant", "nope"}, wantCode: exitFailure, wantOut: `tenant "nope" is not in instance`},
		{name: "missing directory", args: []string{"--config", good + "/missing"}, wantCode: exitFailure, wantOut: "error: " + good + "/missing"},
		{name: "schema error in a tenant", args: []string{"--config", broken}, wantCode: exitFailure, wantOut: "example-solo"},
		{name: "good project", args: []string{"--repo-dir", project, "--stacks", "../../stacks", "--tenant", "example-solo"}, wantCode: exitOK, wantOut: "ok: " + project},
		{name: "project of another tenant", args: []string{"--repo-dir", project, "--stacks", "../../stacks", "--tenant", "other"}, wantCode: exitFailure, wantOut: `the tenant is "other"`},
	}
	// a link out of the checkout is never followed, so its target is neither read nor probed
	secret := filepath.Join(t.TempDir(), "outside.yaml")
	if err := os.WriteFile(secret, []byte("not: [a, blast radius\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	linked := t.TempDir()
	copyFile(t, filepath.Join(project, ".nightshift/project.yaml"), filepath.Join(linked, ".nightshift/project.yaml"))
	copyFile(t, filepath.Join(project, "AGENTS.md"), filepath.Join(linked, "AGENTS.md"))
	if err := os.MkdirAll(filepath.Join(linked, ".nightshift/skills"), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(secret, filepath.Join(linked, ".nightshift/blast-radius.yaml")); err != nil {
		t.Fatal(err)
	}
	// only a warning: the stack has no pack here, everything else is in order
	warnOnly := t.TempDir()
	for _, f := range []string{".nightshift/project.yaml", ".nightshift/blast-radius.yaml", "AGENTS.md"} {
		copyFile(t, filepath.Join(project, f), filepath.Join(warnOnly, f))
	}
	if err := os.MkdirAll(filepath.Join(warnOnly, ".nightshift/skills"), 0o750); err != nil {
		t.Fatal(err)
	}
	tests = append(tests, []struct {
		name     string
		args     []string
		wantCode int
		wantOut  string
	}{
		{name: "symlink out of the checkout", args: []string{"--repo-dir", linked, "--stacks", "../../stacks"}, wantCode: exitFailure, wantOut: "cannot be read"},
		{name: "only a warning passes", args: []string{"--repo-dir", warnOnly, "--stacks", "/nonexistent"}, wantCode: exitOK, wantOut: "warning:"},
		{name: "instance is skipped with only a checkout", args: []string{"--repo-dir", project, "--stacks", "../../stacks"}, wantCode: exitOK, wantOut: "ok: " + project},
	}...)
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			var out, errb bytes.Buffer
			args := []string{"config"}
			if tt.args != nil {
				args = append(args, "validate")
				args = append(args, tt.args...)
			}
			if got := run(args, &out, &errb); got != tt.wantCode {
				t.Fatalf("exit = %d, want %d\nstdout: %s\nstderr: %s", got, tt.wantCode, out.String(), errb.String())
			}
			if !strings.Contains(out.String(), tt.wantOut) {
				t.Errorf("stdout %q lacks %q", out.String(), tt.wantOut)
			}
		})
	}
}
