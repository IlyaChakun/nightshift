package deployer_test

import (
	"context"
	"errors"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/ilyachakun/nightshift/internal/adapters/deploy/remote"
	"github.com/ilyachakun/nightshift/internal/deployer"
	"github.com/ilyachakun/nightshift/internal/gitops"
	"github.com/ilyachakun/nightshift/internal/integration"
)

// dockerStub answers the few docker calls the adapter makes.
type dockerStub struct {
	rendered string
	calls    []string
	dirs     []string
	failUp   bool // `compose up` fails, as a stack that never becomes healthy does
}

func (d *dockerStub) Run(_ context.Context, dir string, _ []string, name string, args ...string) ([]byte, error) {
	d.calls = append(d.calls, name+" "+strings.Join(args, " "))
	d.dirs = append(d.dirs, dir)
	joined := " " + strings.Join(args, " ") + " "
	switch {
	case strings.Contains(joined, " config "):
		return []byte(d.rendered), nil
	case d.failUp && strings.Contains(joined, " up "):
		return []byte("service web is unhealthy"), errors.New("exit status 1")
	case name == "docker" && args[0] == "ps":
		return []byte(`{"State":"running","Status":"Up (healthy)","Labels":"ns.revision=` + lastRev(d.calls) + `"}`), nil
	case strings.Contains(joined, " ps "):
		return []byte(`{"Publishers":[{"PublishedPort":49200}]}`), nil
	}
	return nil, nil
}

func lastRev(calls []string) string {
	for _, call := range slices.Backward(calls) {
		if strings.Contains(call, " up ") {
			return "x"
		}
	}
	return ""
}

func git(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.CommandContext(t.Context(), "git", append([]string{"-C", dir, "-c", "user.name=t", "-c", "user.email=t@example.test", "-c", "commit.gpgsign=false"}, args...)...) //nolint:gosec // test helper with fixed arguments
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
	return strings.TrimSpace(string(out))
}

// mirrorOf makes a bare mirror "example/app.git" under repos with a recipe on main and a task
// branch that tries to change the recipe.
func mirrorOf(t *testing.T) (repos, branchSHA string) {
	t.Helper()
	work := t.TempDir()
	git(t, work, "init", "-q", "-b", "main")
	must(t, os.MkdirAll(filepath.Join(work, "deploy"), 0o750))
	must(t, os.WriteFile(filepath.Join(work, "deploy/compose.yaml"), []byte("services: {web: {image: ok}}\n"), 0o600))
	git(t, work, "add", ".")
	git(t, work, "commit", "-q", "-m", "main")
	git(t, work, "checkout", "-q", "-b", "ns/1")
	must(t, os.WriteFile(filepath.Join(work, "deploy/compose.yaml"), []byte("services: {web: {image: evil, privileged: true}}\n"), 0o600))
	git(t, work, "commit", "-q", "-am", "change the recipe")
	branchSHA = git(t, work, "rev-parse", "HEAD")
	repos = t.TempDir()
	must(t, os.MkdirAll(filepath.Join(repos, "example"), 0o750))
	git(t, repos, "clone", "-q", "--mirror", work, filepath.Join(repos, "example", "app.git"))
	return repos, branchSHA
}

func must(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}

func newClient(t *testing.T, stub *dockerStub, repos string) (*remote.Client, string) {
	t.Helper()
	work := t.TempDir()
	srv := httptest.NewServer((&deployer.Server{Token: "tok", Git: &gitops.Git{Dir: repos}, Work: work, Exec: stub}).Handler())
	t.Cleanup(srv.Close)
	return &remote.Client{
		URL: srv.URL, Token: "tok", Tenant: "example-solo", Repo: "example/app",
		Spec: remote.Spec{File: "deploy/compose.yaml", Entry: "web", Base: "main"},
	}, work
}

func TestPreviewOfATaskBranchUsesTheRecipeFromMain(t *testing.T) {
	t.Parallel()
	repos, branchSHA := mirrorOf(t)
	stub := &dockerStub{rendered: `{"services":{"web":{"image":"ok","ports":[{"target":80}]}}}`}
	c, work := newClient(t, stub, repos)

	ep, err := c.Up(t.Context(), integration.EnvRef{Tenant: "example-solo", Task: "7"}, integration.Revision(branchSHA))
	if err != nil || ep.URL != "http://127.0.0.1:49200" {
		t.Fatalf("Up = %+v, %v", ep, err)
	}
	recipe := filepath.Join(work, "example-solo", "preview-7", branchSHA, "deploy", "compose.yaml")
	got, err := os.ReadFile(recipe) //nolint:gosec // a path in the test's temp directory
	if err != nil || strings.Contains(string(got), "privileged") || !strings.Contains(string(got), "image: ok") {
		t.Fatalf("the recipe at %s is %q (%v); it must be main's, not the branch's", recipe, got, err)
	}
	if stub.dirs[0] == "" || !strings.HasPrefix(stub.dirs[0], work) {
		t.Errorf("docker ran in %q, outside the deployer's work directory %q", stub.dirs[0], work)
	}

	if err := c.Down(t.Context(), integration.EnvRef{Tenant: "example-solo", Task: "7"}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(work, "example-solo", "preview-7")); !os.IsNotExist(err) {
		t.Errorf("the preview checkout survived Down: %v", err)
	}
}

func TestAReplayedUpKeepsTheFinishedCheckout(t *testing.T) {
	t.Parallel()
	repos, branchSHA := mirrorOf(t)
	stub := &dockerStub{rendered: `{"services":{"web":{"image":"ok","ports":[{"target":80}]}}}`}
	c, work := newClient(t, stub, repos)
	env := integration.EnvRef{Tenant: "example-solo", Task: "7"}
	if _, err := c.Up(t.Context(), env, integration.Revision(branchSHA)); err != nil {
		t.Fatal(err)
	}
	keep := filepath.Join(work, "example-solo", "preview-7", branchSHA, "kept-by-a-container")
	if err := os.WriteFile(keep, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := c.Up(t.Context(), env, integration.Revision(branchSHA)); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(keep); err != nil {
		t.Errorf("a replay wiped the checkout that running containers mount: %v", err)
	}
}

func TestARecipeThatBreaksARuleIsRefused(t *testing.T) {
	t.Parallel()
	repos, branchSHA := mirrorOf(t)
	stub := &dockerStub{rendered: `{"services":{"web":{"image":"x","privileged":true}}}`}
	c, _ := newClient(t, stub, repos)
	_, err := c.Up(t.Context(), integration.EnvRef{Tenant: "example-solo", Task: "7"}, integration.Revision(branchSHA))
	if !errors.Is(err, integration.ErrRefused) {
		t.Fatalf("err = %v, want ErrRefused", err)
	}
	for _, call := range stub.calls {
		if strings.Contains(call, " up ") {
			t.Errorf("the stack was started: %s", call)
		}
	}
}

func TestTheServiceChecksTheTokenAndTheInput(t *testing.T) {
	t.Parallel()
	repos, branchSHA := mirrorOf(t)
	c, _ := newClient(t, &dockerStub{}, repos)
	env := integration.EnvRef{Tenant: "example-solo", Task: "7"}

	bad := *c
	bad.Token = "wrong"
	if _, err := bad.Status(t.Context(), env); err == nil {
		t.Error("a wrong token was accepted")
	}
	for name, mut := range map[string]func(*remote.Client){
		"recipe outside":      func(c *remote.Client) { c.Spec.File = "../etc/passwd" },
		"absolute recipe":     func(c *remote.Client) { c.Spec.File = "/etc/passwd" },
		"tenant with a slash": func(c *remote.Client) { c.Tenant = "a/b" },
		"project smoke":       func(c *remote.Client) { c.Spec.Smoke = "./smoke.sh" },
	} {
		cc := *c
		mut(&cc)
		if _, err := cc.Up(t.Context(), env, integration.Revision(branchSHA)); err == nil {
			t.Errorf("%s: accepted", name)
		}
	}
	if _, err := c.Up(t.Context(), env, "main"); err == nil {
		t.Error("a branch name instead of a commit was accepted")
	}
}

// A failed rollout of a release crosses the wire as itself, with the revision to go back to.
func TestAFailedRolloutKeepsItsKindAcrossTheService(t *testing.T) {
	t.Parallel()
	repos, branchSHA := mirrorOf(t)
	stub := &dockerStub{rendered: `{"services":{"web":{"image":"ok","ports":[{"target":80}]}}}`, failUp: true}
	c, _ := newClient(t, stub, repos)
	rel, err := c.Release(t.Context(), integration.Revision(branchSHA))
	if !errors.Is(err, integration.ErrRolloutFailed) || rel.Revision != integration.Revision(branchSHA) {
		t.Fatalf("Release = %+v, %v; want ErrRolloutFailed for the revision", rel, err)
	}
}

// The task branch owns the checkout. A link in place of the recipe's directory must not send
// the deployer's write of the recipe outside the checkout.
func TestARecipeDirectoryThatIsALinkIsRefused(t *testing.T) {
	t.Parallel()
	outside := t.TempDir()
	work := t.TempDir()
	git(t, work, "init", "-q", "-b", "main")
	must(t, os.MkdirAll(filepath.Join(work, "deploy"), 0o750))
	must(t, os.WriteFile(filepath.Join(work, "deploy/compose.yaml"), []byte("services: {web: {image: ok}}\n"), 0o600))
	git(t, work, "add", ".")
	git(t, work, "commit", "-q", "-m", "main")
	git(t, work, "checkout", "-q", "-b", "ns/1")
	must(t, os.RemoveAll(filepath.Join(work, "deploy")))
	must(t, os.Symlink(outside, filepath.Join(work, "deploy")))
	git(t, work, "add", "-A")
	git(t, work, "commit", "-q", "-m", "a link")
	sha := git(t, work, "rev-parse", "HEAD")
	repos := t.TempDir()
	must(t, os.MkdirAll(filepath.Join(repos, "example"), 0o750))
	git(t, repos, "clone", "-q", "--mirror", work, filepath.Join(repos, "example", "app.git"))

	stub := &dockerStub{rendered: `{"services":{"web":{"image":"ok","ports":[{"target":80}]}}}`}
	c, _ := newClient(t, stub, repos)
	if _, err := c.Up(t.Context(), integration.EnvRef{Tenant: "example-solo", Task: "7"}, integration.Revision(sha)); !errors.Is(err, integration.ErrRefused) {
		t.Fatalf("err = %v, want ErrRefused", err)
	}
	if left, err := os.ReadDir(outside); err != nil || len(left) != 0 {
		t.Errorf("the deployer wrote outside the checkout: %v, %v", left, err)
	}
	if len(stub.calls) != 0 {
		t.Errorf("the engine was called: %v", stub.calls)
	}
}

// The marker of a finished checkout has a fixed name, which the task branch can commit as a link.
func TestTheCheckoutMarkerIsNotWrittenThroughALink(t *testing.T) {
	t.Parallel()
	victim := filepath.Join(t.TempDir(), "victim")
	must(t, os.WriteFile(victim, []byte("precious\n"), 0o600))
	work := t.TempDir()
	git(t, work, "init", "-q", "-b", "main")
	must(t, os.MkdirAll(filepath.Join(work, "deploy"), 0o750))
	must(t, os.WriteFile(filepath.Join(work, "deploy/compose.yaml"), []byte("services: {web: {image: ok}}\n"), 0o600))
	git(t, work, "add", ".")
	git(t, work, "commit", "-q", "-m", "main")
	git(t, work, "checkout", "-q", "-b", "ns/1")
	must(t, os.Symlink(victim, filepath.Join(work, ".ns-checkout-ready")))
	git(t, work, "add", "-A")
	git(t, work, "commit", "-q", "-m", "a link")
	sha := git(t, work, "rev-parse", "HEAD")
	repos := t.TempDir()
	must(t, os.MkdirAll(filepath.Join(repos, "example"), 0o750))
	git(t, repos, "clone", "-q", "--mirror", work, filepath.Join(repos, "example", "app.git"))

	stub := &dockerStub{rendered: `{"services":{"web":{"image":"ok","ports":[{"target":80}]}}}`}
	c, _ := newClient(t, stub, repos)
	if _, err := c.Up(t.Context(), integration.EnvRef{Tenant: "example-solo", Task: "7"}, integration.Revision(sha)); err != nil {
		t.Fatal(err)
	}
	if got, err := os.ReadFile(victim); err != nil || string(got) != "precious\n" { //nolint:gosec // a path in the test's temp directory
		t.Errorf("the file outside the checkout is %q (%v); the marker was written through the link", got, err)
	}
}
