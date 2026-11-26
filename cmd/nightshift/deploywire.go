package main

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/ilyachakun/nightshift/internal/adapters/deploy/remote"
	"github.com/ilyachakun/nightshift/internal/book/bookdb"
	"github.com/ilyachakun/nightshift/internal/config"
	"github.com/ilyachakun/nightshift/internal/deploy"
	"github.com/ilyachakun/nightshift/internal/dispatch"
	"github.com/ilyachakun/nightshift/internal/integration"
	"github.com/ilyachakun/nightshift/internal/train"
)

const kindComposeLocal = "compose-local"

// deployWire connects one tenant's conductor to the deployer service: previews at the gate and
// the release in the merge train. It reads the tenant's capabilities from the base branch on every
// use, so a change to project.yaml takes effect after it is merged, never before.
type deployWire struct {
	url, token string
	q          *bookdb.Queries
	c          *dispatch.Conductor
	tenant     config.Tenant
}

// specFor turns a project's capabilities into what the deployer needs for op ("preview" or
// "release"). It reports false when the tenant has no compose-local capability for op: a script or
// external adapter belongs to a later item, and then the task is released at the merge as before.
func specFor(caps config.Capabilities, op, base string) (remote.Spec, bool) {
	capab := caps.Release
	if op == "preview" {
		capab = caps.Preview
	}
	if capab == nil || capab.Kind != kindComposeLocal {
		return remote.Spec{}, false
	}
	spec := remote.Spec{File: capab.File, Entry: capab.Entry, CPUs: capab.CPUs, Memory: capab.Memory, Base: base}
	if p := caps.Preview; p != nil && p.Kind == kindComposeLocal {
		// The release serves the same entry service, and its limits apply there too.
		spec.Entry, spec.CPUs, spec.Memory = p.Entry, p.CPUs, p.Memory
		if spec.File == "" {
			spec.File = p.File
		}
	}
	// A project's own smoke command is not sent: the deployer must not run project code while it
	// holds the engine. Health checks still gate the preview and the release.
	return spec, spec.File != ""
}

// runner builds the deploy steps of repo for op, or false when the tenant has none.
func (w *deployWire) runner(ctx context.Context, repo, op string) (*deploy.Runner, bool, error) {
	c := w.c
	base := c.BaseBranch
	if base == "" {
		base = "main"
	}
	// The merge commit may not be in the mirror yet; the project file is read from a fresh one.
	mirror, err := c.RefreshMirror(ctx, repo)
	if err != nil {
		return nil, false, err
	}
	// Show takes a commit, not a branch name.
	sha, err := c.Git.Rev(ctx, mirror, base)
	if err != nil {
		return nil, false, fmt.Errorf("deploy: %w", err)
	}
	doc, ok, err := c.Git.Show(ctx, mirror, strings.TrimSpace(sha), ".nightshift/project.yaml")
	if err != nil || !ok {
		return nil, false, err
	}
	caps, err := config.ParseCapabilities([]byte(doc))
	if errors.Is(err, config.ErrNoCapabilities) {
		if c.Log != nil {
			c.Log.WarnContext(ctx, "project.yaml has no capabilities block: previews and releases are off", "repo", repo, "op", op)
		}
		return nil, false, nil // a project without deploy targets: previews and releases are off, the task goes on
	}
	if err != nil {
		return nil, false, err
	}
	spec, ok := specFor(caps, op, base)
	if !ok {
		if op == "release" && caps.Release != nil && caps.Release.Kind != "script" && caps.Release.Kind != "external" {
			return nil, false, fmt.Errorf("deploy: release capability %q has no usable recipe", caps.Release.Kind)
		}
		return nil, false, nil
	}
	cl := &remote.Client{URL: w.url, Token: w.token, Tenant: w.tenant.Name, Repo: repo, Spec: spec}
	return &deploy.Runner{Journal: deploy.BookJournal{Q: w.q}, Deployer: cl}, true, nil
}

// install sets the hooks on the conductor and its train.
func (w *deployWire) install() {
	w.c.Deploys = func(ctx context.Context, _ config.Tenant, task bookdb.BookTask) (*deploy.Runner, bool, error) {
		return w.runner(ctx, task.Repo, "preview")
	}
	w.c.Train.Releases = func(ctx context.Context, repo string) (bool, error) {
		_, ok, err := w.runner(ctx, repo, "release")
		return ok, err
	}
	w.c.Train.Release = func(ctx context.Context, job train.Job, mergeSHA string) (deploy.ReleaseResult, bool, error) {
		run, ok, err := w.runner(ctx, job.PR.Repo, "release")
		if err != nil || !ok {
			return deploy.ReleaseResult{}, false, err
		}
		res, err := run.Release(ctx, deploy.Job{Tenant: job.Tenant, TaskID: job.TaskID, Rev: integration.Revision(mergeSHA)})
		return res, err == nil, err
	}
}
