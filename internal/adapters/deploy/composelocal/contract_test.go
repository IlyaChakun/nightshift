package composelocal_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/ilyachakun/nightshift/internal/adapters/deploy/composelocal"
	"github.com/ilyachakun/nightshift/internal/adapters/deploy/composelocal/composetest"
	"github.com/ilyachakun/nightshift/internal/execution/deployapi"
	"github.com/ilyachakun/nightshift/internal/execution/deployapi/deploytest"
)

// checkouts gives every revision the same plain recipe and writes rendered recipes to a directory.
type checkouts struct{ dir, render string }

func (c checkouts) Checkout(context.Context, deployapi.EnvRef, deployapi.Revision) (composelocal.Tree, func(), error) {
	return composelocal.Tree{Dir: c.dir, Recipe: filepath.Join(c.dir, "compose.yaml"), Main: c.dir}, func() {}, nil
}

func (c checkouts) Render(proj string, data []byte) (string, error) {
	file := filepath.Join(c.render, proj+".json")
	return file, os.WriteFile(file, data, 0o600)
}

func TestComposeLocalPassesTheDeployContract(t *testing.T) {
	t.Parallel()
	deploytest.Run(t, func(t *testing.T) deploytest.Target {
		dir := t.TempDir()
		if err := os.WriteFile(filepath.Join(dir, "compose.yaml"), []byte("services:\n  web:\n    image: example\n"), 0o600); err != nil {
			t.Fatal(err)
		}
		engine := composetest.New()
		engine.Bad("bad")
		d := &composelocal.Deployer{
			Cfg:       composelocal.Config{Tenant: "example-solo", Entry: "web", Smoke: "true"},
			Checkouts: checkouts{dir: dir, render: t.TempDir()}, Exec: engine, Work: t.TempDir(),
		}
		return deploytest.Target{Deployer: d, Tenant: "example-solo", Revisions: [2]deployapi.Revision{"r1", "r2"}, Bad: "bad"}
	})
}
