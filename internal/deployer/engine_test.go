package deployer_test

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/ilyachakun/nightshift/internal/adapters/deploy/composelocal"
	"github.com/ilyachakun/nightshift/internal/adapters/deploy/remote"
	"github.com/ilyachakun/nightshift/internal/deployer"
	"github.com/ilyachakun/nightshift/internal/gitops"
	"github.com/ilyachakun/nightshift/internal/integration"
)

// engineEnv turns on the tests that run against a real Docker engine (design section 17, "Tests":
// compose-local against a real engine runs in CI). They start containers, so they are off by default.
const engineEnv = "NIGHTSHIFT_TEST_DOCKER"

func needEngine(t *testing.T) {
	t.Helper()
	if os.Getenv(engineEnv) == "" { //nolint:forbidigo // a test-only switch, not configuration
		t.Skip("set " + engineEnv + "=1 to run against a real Docker engine")
	}
}

// mirrorWith makes a bare mirror "example/app.git" under repos whose main holds files, and
// returns main's commit.
func mirrorWith(t *testing.T, files map[string]string) (repos, sha string) {
	t.Helper()
	work := t.TempDir()
	git(t, work, "init", "-q", "-b", "main")
	for name, body := range files {
		p := filepath.Join(work, name)
		must(t, os.MkdirAll(filepath.Dir(p), 0o750))
		must(t, os.WriteFile(p, []byte(body), 0o600))
	}
	git(t, work, "add", ".")
	git(t, work, "commit", "-q", "-m", "main")
	sha = git(t, work, "rev-parse", "HEAD")
	repos = t.TempDir()
	must(t, os.MkdirAll(filepath.Join(repos, "example"), 0o750))
	git(t, repos, "clone", "-q", "--mirror", work, filepath.Join(repos, "example", "app.git"))
	return repos, sha
}

func engineClient(t *testing.T, repos string) *remote.Client {
	t.Helper()
	srv := httptest.NewServer((&deployer.Server{Token: "tok", Git: &gitops.Git{Dir: repos}, Work: t.TempDir(), Exec: composelocal.OSExec{}}).Handler())
	t.Cleanup(srv.Close)
	return &remote.Client{
		URL: srv.URL, Token: "tok", Tenant: "example-solo", Repo: "example/app",
		Spec: remote.Spec{File: "deploy/compose.yaml", Entry: "web", Base: "main"},
	}
}

// containers lists the containers of a compose project, running or not.
func containers(t *testing.T, proj string) string {
	t.Helper()
	out, err := exec.CommandContext(t.Context(), "docker", "ps", "-aq", "--filter", "label=com.docker.compose.project="+proj).CombinedOutput() //nolint:gosec // test helper with a fixed program
	if err != nil {
		t.Fatalf("docker ps: %v: %s", err, out)
	}
	return strings.TrimSpace(string(out))
}

const plainImage = "busybox:1.37"

// runID makes the compose project names of one run its own, so two runs on one engine do not
// meet: a task key is lower-case letters, digits and dashes.
func runID() string {
	return "t" + strconv.FormatInt(time.Now().UnixNano()%1_000_000_000, 36) + "-"
}

// T6 on a real engine: every escape recipe is refused before anything starts.
func TestEngineRefusesEachEscapeRecipe(t *testing.T) {
	needEngine(t)
	svc := func(extra string) string {
		return "services:\n  web:\n    image: " + plainImage + "\n    command: [sleep, \"60\"]\n" + extra
	}
	harmless := "services:\n  web:\n    image: " + plainImage + "\n"
	tests := []struct {
		name  string
		files map[string]string
		rule  string // the rule the refusal must name, when another rule could refuse it too
	}{
		{"privileged", map[string]string{"deploy/compose.yaml": svc("    privileged: true\n")}, ""},
		{"host network", map[string]string{"deploy/compose.yaml": svc("    network_mode: host\n")}, ""},
		{"host pid", map[string]string{"deploy/compose.yaml": svc("    pid: host\n")}, ""},
		{"added capability", map[string]string{"deploy/compose.yaml": svc("    cap_add: [SYS_ADMIN]\n")}, ""},
		{"docker socket", map[string]string{"deploy/compose.yaml": svc("    volumes: [\"/var/run/docker.sock:/s\"]\n")}, ""},
		{"host directory", map[string]string{"deploy/compose.yaml": svc("    volumes: [\"/etc:/h:ro\"]\n")}, ""},
		{"device", map[string]string{"deploy/compose.yaml": svc("    devices: [\"/dev/null:/dev/x\"]\n")}, ""},
		// The other file is harmless, so only the one-file rule can refuse these.
		{"include of another file", map[string]string{
			"deploy/compose.yaml": "include: [other.yaml]\n" + svc(""),
			"deploy/other.yaml":   "services:\n  side:\n    image: " + plainImage + "\n",
		}, "another compose file"},
		{"extends from another file", map[string]string{
			"deploy/compose.yaml": "services:\n  web:\n    extends: {file: base.yaml, service: web}\n",
			"deploy/base.yaml":    harmless,
		}, "another compose file"},
	}
	run := runID()
	for i, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			repos, sha := mirrorWith(t, tt.files)
			c := engineClient(t, repos)
			env := integration.EnvRef{Tenant: "example-solo", Task: run + strconv.Itoa(i)}
			// A refusal that regresses must not leave a privileged container on the engine.
			t.Cleanup(func() { _ = c.Down(context.Background(), env) }) //nolint:usetesting // the test context is gone at cleanup
			_, err := c.Up(t.Context(), env, integration.Revision(sha))
			if !errors.Is(err, integration.ErrRefused) {
				t.Fatalf("Up = %v, want ErrRefused", err)
			}
			if tt.rule != "" && !strings.Contains(err.Error(), tt.rule) {
				t.Errorf("refused for another reason: %v; want %q", err, tt.rule)
			}
			if got := containers(t, "ns-example-solo-"+env.Task); got != "" {
				t.Errorf("containers were created for a refused recipe: %s", got)
			}
		})
	}
}

// A plain recipe comes up healthy on loopback, passes its smoke and goes away on Down.
func TestEnginePlainRecipeComesUpAndGoes(t *testing.T) {
	needEngine(t)
	recipe := "services:\n  web:\n    image: " + plainImage + "\n" +
		"    command: [sh, -c, \"mkdir -p /www && echo ok > /www/index.html && exec httpd -f -p 80 -h /www\"]\n" +
		"    ports: [\"80\"]\n" +
		"    healthcheck: {test: [CMD, wget, -q, -O, /dev/null, \"http://127.0.0.1/\"], interval: 1s, retries: 30}\n"
	repos, sha := mirrorWith(t, map[string]string{"deploy/compose.yaml": recipe})
	c := engineClient(t, repos)
	env := integration.EnvRef{Tenant: "example-solo", Task: runID() + "p"}
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Minute)
	defer cancel()
	t.Cleanup(func() { _ = c.Down(context.Background(), env) }) //nolint:usetesting // the test context is gone at cleanup

	ep, err := c.Up(ctx, env, integration.Revision(sha))
	if err != nil {
		t.Fatalf("Up: %v", err)
	}
	if !strings.HasPrefix(ep.URL, "http://127.0.0.1:") {
		t.Errorf("the preview answers at %q, want loopback", ep.URL)
	}
	sm, err := c.Smoke(ctx, env)
	if err != nil || !sm.OK {
		t.Fatalf("Smoke = %+v, %v", sm, err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, ep.URL, nil)
	must(t, err)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("GET %s: %v", ep.URL, err)
	}
	body, _ := io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusOK || strings.TrimSpace(string(body)) != "ok" {
		t.Errorf("GET %s = %d %q", ep.URL, resp.StatusCode, body)
	}
	if err := c.Down(ctx, env); err != nil {
		t.Fatalf("Down: %v", err)
	}
	if st, err := c.Status(ctx, env); err != nil || st.State != integration.EnvAbsent {
		t.Errorf("after Down: %+v, %v", st, err)
	}
}
