package deployer_test

import (
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/ilyachakun/nightshift/internal/adapters/deploy/composelocal/composetest"
	"github.com/ilyachakun/nightshift/internal/adapters/deploy/deployer"
	"github.com/ilyachakun/nightshift/internal/adapters/deploy/remote"
	"github.com/ilyachakun/nightshift/internal/execution/deployapi"
	"github.com/ilyachakun/nightshift/internal/execution/deployapi/deploytest"
	"github.com/ilyachakun/nightshift/internal/execution/gitops"
)

// TestTheServicePassesTheDeployContract runs the contract through the remote client, the
// deployer service and compose-local, on three commits of main.
func TestTheServicePassesTheDeployContract(t *testing.T) {
	t.Parallel()
	repos, revs := mainOfThree(t)
	deploytest.Run(t, func(t *testing.T) deploytest.Target {
		engine := composetest.New()
		srv := httptest.NewServer((&deployer.Server{Token: "tok", Git: &gitops.Git{Dir: repos}, Work: t.TempDir(), Exec: engine}).Handler())
		t.Cleanup(srv.Close)
		c := &remote.Client{
			URL: srv.URL, Token: "tok", Tenant: "example-solo", Repo: "example/app",
			Spec: remote.Spec{File: "deploy/compose.yaml", Entry: "web", Base: "main"},
		}
		return deploytest.Target{Deployer: c, Tenant: "example-solo", Revisions: revs, FailRollout: engine.FailRollout}
	})
}

// mainOfThree makes a bare mirror "example/app.git" whose main has three commits.
func mainOfThree(t *testing.T) (string, [3]deployapi.Revision) {
	t.Helper()
	work := t.TempDir()
	git(t, work, "init", "-q", "-b", "main")
	must(t, os.MkdirAll(filepath.Join(work, "deploy"), 0o750))
	must(t, os.WriteFile(filepath.Join(work, "deploy/compose.yaml"), []byte("services: {web: {image: example}}\n"), 0o600))
	var revs [3]deployapi.Revision
	for i := range revs {
		must(t, os.WriteFile(filepath.Join(work, "version"), []byte{byte('1' + i)}, 0o600))
		git(t, work, "add", ".")
		git(t, work, "commit", "-q", "-m", "version")
		revs[i] = deployapi.Revision(git(t, work, "rev-parse", "HEAD"))
	}
	repos := t.TempDir()
	must(t, os.MkdirAll(filepath.Join(repos, "example"), 0o750))
	git(t, repos, "clone", "-q", "--mirror", work, filepath.Join(repos, "example", "app.git"))
	return repos, revs
}
