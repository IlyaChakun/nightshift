package deployer_test

import (
	"context"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"sync/atomic"
	"testing"

	"github.com/ilyachakun/nightshift/internal/adapters/deploy/composelocal"
	"github.com/ilyachakun/nightshift/internal/adapters/deploy/composelocal/composetest"
	"github.com/ilyachakun/nightshift/internal/adapters/deploy/deployer"
	"github.com/ilyachakun/nightshift/internal/adapters/deploy/remote"
	"github.com/ilyachakun/nightshift/internal/execution/deployapi"
	"github.com/ilyachakun/nightshift/internal/execution/deployapi/deploytest"
	"github.com/ilyachakun/nightshift/internal/execution/gitops"
)

// TestTheServicePassesTheDeployContract runs the contract through the remote client, the
// deployer service and compose-local, on the docker double.
func TestTheServicePassesTheDeployContract(t *testing.T) {
	t.Parallel()
	repos, revs, bad := mainOf(t, "services: {web: {image: example}}\n")
	deploytest.Run(t, func(t *testing.T) deploytest.Target {
		engine := composetest.New()
		engine.Bad(string(bad))
		c := serviceClient(t, repos, engine, "example-solo")
		return deploytest.Target{Deployer: c, Tenant: "example-solo", Revisions: revs, Bad: bad}
	})
}

// TestEnginePassesTheDeployContract runs the same contract on a real Docker engine: the
// healthcheck needs app/healthy from the revision's checkout, which the bad revision lacks.
func TestEnginePassesTheDeployContract(t *testing.T) {
	needEngine(t)
	recipe := "services:\n  web:\n    image: " + plainImage + "\n" +
		"    command: [httpd, -f, -p, \"80\", -h, /app]\n" +
		"    ports: [\"80\"]\n" +
		"    volumes: [\"./app:/app:ro\"]\n" +
		"    healthcheck: {test: [CMD, test, -f, /app/healthy], interval: 1s, retries: 3}\n"
	repos, revs, bad := mainOf(t, recipe)
	run := runID()
	var n atomic.Int32
	deploytest.Run(t, func(t *testing.T) deploytest.Target {
		tenant := run + strconv.Itoa(int(n.Add(1)))
		t.Cleanup(func() {
			for _, env := range []string{"prod", "7", "8"} {
				_ = exec.CommandContext(context.Background(), "docker", "compose", "-p", "ns-"+tenant+"-"+env, "down", "--volumes", "--remove-orphans").Run() //nolint:gosec,usetesting // fixed program; the test context is gone at cleanup
			}
		})
		c := serviceClient(t, repos, composelocal.OSExec{}, tenant)
		return deploytest.Target{Deployer: c, Tenant: tenant, Revisions: revs, Bad: bad}
	})
}

func serviceClient(t *testing.T, repos string, exec composelocal.Exec, tenant string) *remote.Client {
	t.Helper()
	srv := httptest.NewServer((&deployer.Server{Token: "tok", Git: &gitops.Git{Dir: repos}, Work: t.TempDir(), Exec: exec}).Handler())
	t.Cleanup(srv.Close)
	return &remote.Client{
		URL: srv.URL, Token: "tok", Tenant: tenant, Repo: "example/app",
		Spec: remote.Spec{File: "deploy/compose.yaml", Entry: "web", Base: "main"},
	}
}

// mainOf makes a bare mirror "example/app.git" whose main holds recipe and three commits: two
// with app/healthy and, last, one without it.
func mainOf(t *testing.T, recipe string) (repos string, good [2]deployapi.Revision, bad deployapi.Revision) {
	t.Helper()
	work := t.TempDir()
	git(t, work, "init", "-q", "-b", "main")
	must(t, os.MkdirAll(filepath.Join(work, "deploy"), 0o750))
	must(t, os.MkdirAll(filepath.Join(work, "app"), 0o750))
	must(t, os.WriteFile(filepath.Join(work, "deploy/compose.yaml"), []byte(recipe), 0o600))
	commit := func(version string, healthy bool) deployapi.Revision {
		must(t, os.WriteFile(filepath.Join(work, "app/index.html"), []byte(version), 0o600))
		if healthy {
			must(t, os.WriteFile(filepath.Join(work, "app/healthy"), nil, 0o600))
		} else {
			must(t, os.Remove(filepath.Join(work, "app/healthy")))
		}
		git(t, work, "add", "-A")
		git(t, work, "commit", "-q", "-m", "version "+version)
		return deployapi.Revision(git(t, work, "rev-parse", "HEAD"))
	}
	good = [2]deployapi.Revision{commit("1", true), commit("2", true)}
	bad = commit("bad", false)
	repos = t.TempDir()
	must(t, os.MkdirAll(filepath.Join(repos, "example"), 0o750))
	git(t, repos, "clone", "-q", "--mirror", work, filepath.Join(repos, "example", "app.git"))
	return repos, good, bad
}
