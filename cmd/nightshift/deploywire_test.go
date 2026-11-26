package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/ilyachakun/nightshift/internal/config"
	"github.com/ilyachakun/nightshift/internal/dispatch"
	"github.com/ilyachakun/nightshift/internal/gitops"
	"github.com/ilyachakun/nightshift/internal/train"
)

func TestSpecFor(t *testing.T) {
	t.Parallel()
	doc := `capabilities:
  merge_train: true
  preview: {kind: compose-local, file: deploy/compose.yaml, entry: web, limits: {cpus: 1, memory: 512m}}
  release: {kind: compose-local}
  smoke: ./smoke.sh
`
	caps, err := config.ParseCapabilities([]byte(doc))
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		op   string
		want bool
	}{{"preview", true}, {"release", true}} {
		spec, ok := specFor(caps, tc.op, "main")
		if ok != tc.want || spec.File != "deploy/compose.yaml" || spec.Entry != "web" || spec.Memory != "512m" || spec.Smoke != "" || spec.Base != "main" {
			t.Errorf("%s: spec = %+v, %v", tc.op, spec, ok)
		}
	}
	// A script adapter cannot be served by the compose-local deployer.
	script, _ := config.ParseCapabilities([]byte("capabilities:\n  merge_train: true\n  release: make deploy\n  preview: null\n"))
	if _, ok := specFor(script, "release", "main"); ok {
		t.Error("a script release must not go to the compose-local deployer")
	}
	if _, ok := specFor(script, "preview", "main"); ok {
		t.Error("no preview capability, no preview")
	}
}

// TestRunnerReadsTheProjectFromTheBaseBranch covers the path that a branch name used to break:
// the project file is read at the resolved base commit of a fresh mirror.
func TestRunnerReadsTheProjectFromTheBaseBranch(t *testing.T) {
	t.Parallel()
	origin := t.TempDir()
	git := func(dir string, args ...string) {
		t.Helper()
		cmd := exec.CommandContext(t.Context(), "git", append([]string{"-c", "user.name=t", "-c", "user.email=t@example.com", "-c", "init.defaultBranch=main"}, args...)...) //nolint:gosec // a test helper
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	git(origin, "init", "-q")
	if err := os.MkdirAll(filepath.Join(origin, ".nightshift"), 0o750); err != nil {
		t.Fatal(err)
	}
	doc := "capabilities:\n  merge_train: true\n  preview: {kind: compose-local, file: deploy/compose.yaml, entry: web}\n  release: {kind: compose-local}\n"
	if err := os.WriteFile(filepath.Join(origin, ".nightshift", "project.yaml"), []byte(doc), 0o600); err != nil {
		t.Fatal(err)
	}
	git(origin, "add", "-A")
	git(origin, "commit", "-q", "-m", "init")

	c := &dispatch.Conductor{Git: &gitops.Git{Dir: t.TempDir()}, Remote: func(string) string { return origin }, Train: &train.Train{}}
	w := &deployWire{url: "http://deployer:8083", c: c, tenant: config.Tenant{Name: "example-solo"}}
	for _, op := range []string{"preview", "release"} {
		run, ok, err := w.runner(t.Context(), "example/app", op)
		if err != nil || !ok || run == nil {
			t.Errorf("%s: runner = %v, %v, %v", op, run, ok, err)
		}
	}
	for _, tc := range []struct{ name, release, url string }{
		{"recipe", "{kind: compose-local}", "http://deployer:8083"},
		{"adapter", "make deploy", "http://deployer:8083"},
		{"service", "{kind: compose-local, file: deploy/compose.yaml}", ""},
	} {
		if err := os.WriteFile(filepath.Join(origin, ".nightshift", "project.yaml"), []byte("capabilities:\n  release: "+tc.release+"\n"), 0o600); err != nil {
			t.Fatal(err)
		}
		git(origin, "commit", "-qam", tc.name)
		w.url = tc.url
		w.install()
		if _, err := c.Train.Releases(t.Context(), "example/app"); err == nil {
			t.Errorf("%s: a configured release needs a usable adapter, recipe and service", tc.name)
		}
	}
}

// A project file without `capabilities` has no deploy targets; it must not stall the task.
func TestARunnerForAProjectWithoutCapabilitiesIsOff(t *testing.T) {
	t.Parallel()
	origin := t.TempDir()
	git := func(args ...string) {
		t.Helper()
		cmd := exec.CommandContext(t.Context(), "git", append([]string{"-c", "user.name=t", "-c", "user.email=t@example.com", "-c", "init.defaultBranch=main"}, args...)...) //nolint:gosec // a test helper
		cmd.Dir = origin
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	git("init", "-q")
	if err := os.MkdirAll(filepath.Join(origin, ".nightshift"), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(origin, ".nightshift", "project.yaml"), []byte("stack: [go]\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	git("add", "-A")
	git("commit", "-q", "-m", "init")
	c := &dispatch.Conductor{Git: &gitops.Git{Dir: t.TempDir()}, Remote: func(string) string { return origin }, Train: &train.Train{}}
	w := &deployWire{url: "http://deployer:8083", c: c, tenant: config.Tenant{Name: "example-solo"}}
	for _, op := range []string{"preview", "release"} {
		if run, ok, err := w.runner(t.Context(), "example/app", op); err != nil || ok || run != nil {
			t.Errorf("%s: runner = %v, %v, %v; want off without an error", op, run, ok, err)
		}
	}
}
