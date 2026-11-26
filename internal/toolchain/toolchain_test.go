package toolchain

import (
	"context"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func newServer(t *testing.T) (Client, string, *int) {
	t.Helper()
	root := t.TempDir()
	dir := filepath.Join(root, "task-1")
	if err := os.Mkdir(dir, 0o750); err != nil {
		t.Fatal(err)
	}
	calls := 0
	s := &Server{Token: "secret", Root: root, Run: func(context.Context, string) ([]byte, error) {
		calls++
		return []byte("installed go"), nil
	}}
	ts := httptest.NewServer(s.Handler())
	t.Cleanup(ts.Close)
	return Client{URL: ts.URL, Token: "secret"}, dir, &calls
}

func TestInstallRunsOnceAndReturnsTheOutput(t *testing.T) {
	c, dir, calls := newServer(t)
	out, err := c.Install(t.Context(), "job-1", dir)
	if err != nil || out != "installed go" || *calls != 1 {
		t.Fatalf("out=%q err=%v calls=%d", out, err, *calls)
	}
	if _, err := c.Install(t.Context(), "job-1", dir); err == nil || !strings.Contains(err.Error(), "409") {
		t.Fatalf("a second call for the same job must be refused: %v", err)
	}
	if *calls != 1 {
		t.Fatalf("calls = %d", *calls)
	}
}

func TestInstallRefusesABadTokenAndAPathOutsideTheRoot(t *testing.T) {
	c, dir, calls := newServer(t)
	bad := c
	bad.Token = "other"
	if _, err := bad.Install(t.Context(), "j", dir); err == nil || !strings.Contains(err.Error(), "401") {
		t.Fatalf("token: %v", err)
	}
	for _, p := range []string{"/etc", filepath.Dir(dir), filepath.Join(dir, "..", "..")} {
		if _, err := c.Install(t.Context(), "j2", p); err == nil {
			t.Fatalf("%s must be refused", p)
		}
	}
	if *calls != 0 {
		t.Fatalf("nothing may run: %d", *calls)
	}
}

func TestInstallReportsAFailureWithItsOutput(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "t")
	if err := os.Mkdir(dir, 0o750); err != nil {
		t.Fatal(err)
	}
	s := &Server{Token: "x", Root: root, Run: func(context.Context, string) ([]byte, error) {
		return []byte("no such tool"), context.DeadlineExceeded
	}}
	ts := httptest.NewServer(s.Handler())
	defer ts.Close()
	out, err := Client{URL: ts.URL, Token: "x"}.Install(t.Context(), "j", dir)
	if err == nil || out != "no such tool" {
		t.Fatalf("out=%q err=%v", out, err)
	}
}

// The install runs with the service's own proxies and module cache, and nothing else of its
// environment.
func TestMiseGetsTheProxiesAndNoOtherEnvironment(t *testing.T) {
	bin := t.TempDir()
	if err := os.WriteFile(filepath.Join(bin, "mise"), []byte("#!/bin/sh\nenv\n"), 0o700); err != nil { //nolint:gosec // G306: the test runs this script
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+":/usr/bin:/bin")
	t.Setenv("HTTPS_PROXY", "http://egress-install:4750")
	t.Setenv("NO_PROXY", "localhost,modcache")
	t.Setenv("GOPROXY", "http://modcache:3000")
	t.Setenv("WORKER_TOKEN", "not-for-mise")
	out, err := (&Server{AgentHome: bin}).mise(t.Context(), bin)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"HTTPS_PROXY=http://egress-install:4750", "NO_PROXY=localhost,modcache", "GOPROXY=http://modcache:3000"} {
		if !strings.Contains(string(out), want+"\n") {
			t.Errorf("mise lacks %s", want)
		}
	}
	if strings.Contains(string(out), "WORKER_TOKEN") {
		t.Error("the service's environment reached mise")
	}
}

// TestMiseInstallsIntoTheTenantsHome: the tools of a checkout go into the HOME of its tenant, the
// first directory below the work root, the one the worker runs that tenant's jobs with (M3).
func TestMiseInstallsIntoTheTenantsHome(t *testing.T) {
	root, home := t.TempDir(), t.TempDir()
	s := &Server{Root: root, AgentHome: home}
	for dir, want := range map[string]string{
		filepath.Join(root, "example-team", "9"): filepath.Join(home, "tenants", "example-team"),
		root:                                     home,
		"/elsewhere/x":                           home,
	} {
		if got := s.homeOf(dir); got != want {
			t.Errorf("homeOf(%s) = %s, want %s", dir, got, want)
		}
	}
	if got := (&Server{AgentHome: home}).homeOf("/x/y"); got != home {
		t.Errorf("without a root the plain home stays, got %s", got)
	}
}

// TestNoAgentHomeMeansNoTenantHome: without an agent home (a worker that does not drop to the agent
// user) the service must not invent a relative HOME; the runner gives no home in that case either.
func TestNoAgentHomeMeansNoTenantHome(t *testing.T) {
	root := t.TempDir()
	if got := (&Server{Root: root}).homeOf(filepath.Join(root, "example-team", "9")); got != "" {
		t.Errorf("homeOf without an agent home = %q, want empty", got)
	}
}
