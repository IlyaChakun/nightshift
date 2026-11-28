package toolchain

import (
	"context"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

// att is an attempt name in the form of runner.attemptOf; a checkout ends with it.
const att = "0123456789abcdef"

func newServer(t *testing.T) (Client, string, *int) {
	t.Helper()
	root := t.TempDir()
	dir := filepath.Join(root, "task-1-"+att)
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
	out, err := c.Install(t.Context(), "job-1", att, dir)
	if err != nil || out != "installed go" || *calls != 1 {
		t.Fatalf("out=%q err=%v calls=%d", out, err, *calls)
	}
	if _, err := c.Install(t.Context(), "job-1", att, dir); err == nil || !strings.Contains(err.Error(), "409") {
		t.Fatalf("a second call for the same job must be refused: %v", err)
	}
	if *calls != 1 {
		t.Fatalf("calls = %d", *calls)
	}
}

// A worker that dies mid-job leaves the job to a new attempt; it must install again
// (live check V3, 2026-11-28: attempt 2 got 409 and the task ran out of budget).
func TestInstallAcceptsANewAttemptButNotASeenOne(t *testing.T) {
	c, dir, calls := newServer(t)
	root := filepath.Dir(dir)
	a2 := "fedcba9876543210"
	dir2 := filepath.Join(root, "task-1-"+a2)
	if err := os.Mkdir(dir2, 0o750); err != nil {
		t.Fatal(err)
	}
	install := func(attempt, d string) error {
		_, err := c.Install(t.Context(), "job-1", attempt, d)
		return err
	}
	if err := install(att, dir); err != nil {
		t.Fatalf("first attempt: %v", err)
	}
	if err := install(a2, dir2); err != nil {
		t.Fatalf("a retried job must install again: %v", err)
	}
	for name, tc := range map[string]struct{ attempt, dir, code string }{
		"same attempt again":   {a2, dir2, "409"},
		"older attempt":        {att, dir, "409"},
		"no attempt":           {"", dir, "400"},
		"another checkout":     {"00000000000000ff", dir2, "400"},
		"suffix of a checkout": {"1-" + att, dir, "400"},
		"not the attempt form": {"task-1-" + att, dir, "400"},
	} {
		if err := install(tc.attempt, tc.dir); err == nil || !strings.Contains(err.Error(), tc.code) {
			t.Errorf("%s: want %s, got %v", name, tc.code, err)
		}
	}
	if *calls != 2 {
		t.Fatalf("calls = %d, want 2", *calls)
	}
}

// A new attempt that comes while the old one still installs waits for it instead of failing,
// and the two installs never overlap.
func TestANewAttemptWaitsForTheRunningInstall(t *testing.T) {
	root := t.TempDir()
	a1, a2 := att, "fedcba9876543210"
	for _, a := range []string{a1, a2} {
		if err := os.Mkdir(filepath.Join(root, "task-1-"+a), 0o750); err != nil {
			t.Fatal(err)
		}
	}
	started, release := make(chan struct{}), make(chan struct{})
	var once sync.Once
	var mu sync.Mutex
	inside, overlap := 0, false
	s := &Server{Token: "secret", Root: root, Run: func(_ context.Context, dir string) ([]byte, error) {
		mu.Lock()
		inside++
		overlap = overlap || inside > 1
		mu.Unlock()
		if strings.HasSuffix(dir, a1) {
			close(started)
			<-release
		}
		mu.Lock()
		inside--
		mu.Unlock()
		return []byte("ok"), nil
	}}
	ts := httptest.NewServer(s.Handler())
	t.Cleanup(ts.Close)
	t.Cleanup(func() { once.Do(func() { close(release) }) }) // runs before ts.Close, so a failure does not hang
	c := Client{URL: ts.URL, Token: "secret"}
	first := make(chan error, 1)
	go func() { _, err := c.Install(t.Context(), "job-1", a1, filepath.Join(root, "task-1-"+a1)); first <- err }()
	<-started
	second := make(chan error, 1)
	go func() {
		_, err := c.Install(t.Context(), "job-1", a2, filepath.Join(root, "task-1-"+a2))
		second <- err
	}()
	select {
	case err := <-second:
		t.Fatalf("the new attempt did not wait: %v", err)
	case <-time.After(100 * time.Millisecond):
	}
	once.Do(func() { close(release) })
	if err := <-first; err != nil {
		t.Fatal(err)
	}
	if err := <-second; err != nil {
		t.Fatalf("the new attempt must install after the old one: %v", err)
	}
	if overlap {
		t.Fatal("two installs of one job ran at once")
	}
}

func TestInstallRefusesABadTokenAndAPathOutsideTheRoot(t *testing.T) {
	c, dir, calls := newServer(t)
	bad := c
	bad.Token = "other"
	if _, err := bad.Install(t.Context(), "j", att, dir); err == nil || !strings.Contains(err.Error(), "401") {
		t.Fatalf("token: %v", err)
	}
	for _, p := range []string{"/etc", filepath.Dir(dir), filepath.Join(dir, "..", "..")} {
		if _, err := c.Install(t.Context(), "j2", att, p); err == nil {
			t.Fatalf("%s must be refused", p)
		}
	}
	if *calls != 0 {
		t.Fatalf("nothing may run: %d", *calls)
	}
}

func TestInstallReportsAFailureWithItsOutput(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "t-"+att)
	if err := os.Mkdir(dir, 0o750); err != nil {
		t.Fatal(err)
	}
	s := &Server{Token: "x", Root: root, Run: func(context.Context, string) ([]byte, error) {
		return []byte("no such tool"), context.DeadlineExceeded
	}}
	ts := httptest.NewServer(s.Handler())
	defer ts.Close()
	out, err := Client{URL: ts.URL, Token: "x"}.Install(t.Context(), "j", att, dir)
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
	root, err := filepath.EvalSymlinks(t.TempDir()) // within passes a resolved directory to mise
	if err != nil {
		t.Fatal(err)
	}
	home := t.TempDir()
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

func TestInstallRefusesALinkThatLeadsOutOfTheRoot(t *testing.T) {
	c, dir, calls := newServer(t)
	link := filepath.Join(filepath.Dir(dir), "escape")
	if err := os.Symlink(t.TempDir(), link); err != nil {
		t.Fatal(err)
	}
	for _, p := range []string{link, "relative/dir", filepath.Dir(dir)} {
		if _, err := c.Install(t.Context(), "j-"+p, att, p); err == nil {
			t.Fatalf("%s must be refused", p)
		}
	}
	if *calls != 0 {
		t.Fatalf("nothing may run: %d", *calls)
	}
}

func TestInstallUsesResolvedCheckoutBelowConfiguredRoot(t *testing.T) {
	parent := t.TempDir()
	checkout := filepath.Join(parent, "example-team", "9-"+att)
	if err := os.MkdirAll(checkout, 0o750); err != nil {
		t.Fatal(err)
	}
	alias := filepath.Join(t.TempDir(), "work")
	if err := os.Symlink(parent, alias); err != nil {
		t.Fatal(err)
	}
	s := &Server{Root: alias, AgentHome: t.TempDir()}
	got, err := s.within(filepath.Join(alias, "example-team", "9-"+att))
	if err != nil {
		t.Fatal(err)
	}
	want, err := filepath.EvalSymlinks(checkout)
	if err != nil || got != want {
		t.Fatalf("within = %q, want %q: %v", got, want, err)
	}
	if home := s.homeOf(got); home != filepath.Join(s.AgentHome, "tenants", "example-team") {
		t.Fatalf("unexpected tenant home %q", home)
	}
}
