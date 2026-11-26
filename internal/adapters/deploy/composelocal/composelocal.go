package composelocal

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"

	"github.com/ilyachakun/nightshift/internal/integration"
)

// Labels the adapter puts on every container, so Status needs no state of its own: a restart
// reads them back from the engine.
const (
	labelRevision = "ns.revision"
	labelPrevious = "ns.previous"
)

// Config is the `compose-local` entry of project.yaml, plus the project's own smoke command.
type Config struct {
	Tenant string  // the tenant this deployer serves; Release and Rollback carry no environment
	Entry  string  // the compose service that serves the preview
	CPUs   float64 // per service; 0 means the default
	Memory string  // per service, for example "2g"; empty means the default
	// Smoke is the project's own check, run after the healthchecks; empty means none.
	Smoke string
}

// Tree is a checkout the adapter works in: the code at the revision under test, and the recipe
// as it is on main (the code under test cannot change the rules it runs under).
type Tree struct {
	Dir    string // the code at the revision; the only place a bind mount may point into
	Recipe string // absolute path of the compose file, read from main
	// Main is a clean checkout of main. The project's smoke check runs there, not in Dir: a check
	// from the code under test would run on the host with the deployer's rights.
	Main string
}

// Checkouts gives the adapter a tree at a revision for an environment. A running stack may bind
// mount files of its tree, so the tree must outlive the call: the provider keeps it until the
// environment is taken down or replaced, and the returned func only says the adapter is done.
type Checkouts interface {
	Checkout(ctx context.Context, e integration.EnvRef, rev integration.Revision) (Tree, func(), error)
}

// Exec runs a program and returns its standard output. The real one runs the docker CLI; tests
// replace it.
type Exec interface {
	Run(ctx context.Context, dir string, env []string, name string, args ...string) ([]byte, error)
}

// Deployer is the compose-local adapter.
type Deployer struct {
	Cfg       Config
	Checkouts Checkouts
	Exec      Exec
	// Work is a directory for the rendered recipes, outside every checkout.
	Work string

	mu sync.Mutex // one Up, Release or Rollback at a time: they share the rendered files
}

var _ integration.Deployer = (*Deployer)(nil)

var serviceName = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9_.-]*$`)

var safeName = regexp.MustCompile(`^[a-z0-9][a-z0-9-]*$`)

// project is the compose project of an environment: `ns-<tenant>-<task>`, or `ns-<tenant>-prod`.
func project(e integration.EnvRef) (string, error) {
	task := e.Task
	if task == "" {
		task = "prod"
	}
	if !safeName.MatchString(e.Tenant) || !safeName.MatchString(task) {
		return "", fmt.Errorf("composelocal: %q/%q is not a safe environment name", e.Tenant, e.Task)
	}
	return "ns-" + e.Tenant + "-" + task, nil
}

func (d *Deployer) compose(ctx context.Context, dir, proj string, args ...string) ([]byte, error) {
	out, err := d.Exec.Run(ctx, dir, nil, "docker", append([]string{"compose", "-p", proj}, args...)...)
	if err != nil {
		return out, fmt.Errorf("docker compose %s: %w", strings.Join(args[:min(len(args), 2)], " "), err)
	}
	return out, nil
}

// Up renders the recipe, refuses it when Lint finds a forbidden setting, rewrites it with
// loopback ports, limits and labels, and brings the stack up and waits for its healthchecks.
func (d *Deployer) Up(ctx context.Context, e integration.EnvRef, rev integration.Revision) (integration.Endpoint, error) {
	return d.up(ctx, e, rev, "")
}

func (d *Deployer) up(ctx context.Context, e integration.EnvRef, rev, previous integration.Revision) (integration.Endpoint, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	proj, err := project(e)
	if err != nil {
		return integration.Endpoint{}, err
	}
	tree, done, err := d.Checkouts.Checkout(ctx, e, rev)
	if err != nil {
		return integration.Endpoint{}, fmt.Errorf("composelocal: checkout %s: %w", rev, err)
	}
	defer done()
	rendered, err := d.compose(ctx, tree.Dir, proj, "--project-directory", tree.Dir, "-f", tree.Recipe, "config", "--format", "json")
	if err != nil {
		return integration.Endpoint{}, fmt.Errorf("composelocal: render: %w", err)
	}
	bad, err := Lint(rendered, tree.Dir)
	if err != nil {
		return integration.Endpoint{}, err
	}
	if len(bad) > 0 {
		return integration.Endpoint{}, &RecipeError{Violations: bad}
	}
	safe, err := d.rewrite(rendered, rev, previous)
	if err != nil {
		return integration.Endpoint{}, err
	}
	file := filepath.Join(d.Work, proj+".json")
	if err := os.MkdirAll(d.Work, 0o700); err != nil {
		return integration.Endpoint{}, fmt.Errorf("composelocal: %w", err)
	}
	if err := os.WriteFile(file, safe, 0o600); err != nil {
		return integration.Endpoint{}, fmt.Errorf("composelocal: %w", err)
	}
	if _, err := d.compose(ctx, tree.Dir, proj, "--project-directory", tree.Dir, "-f", file, "up", "-d", "--build", "--wait", "--remove-orphans"); err != nil {
		return integration.Endpoint{}, err
	}
	return d.endpoint(ctx, proj)
}

// RecipeError says the recipe uses a setting the adapter refuses. It is a defect of the change
// that carries the recipe, not of the platform.
type RecipeError struct{ Violations []Violation }

// Is makes errors.Is(err, integration.ErrRefused) true.
func (r *RecipeError) Is(target error) bool { return target == integration.ErrRefused }

func (r *RecipeError) Error() string {
	var s []string
	for _, v := range r.Violations {
		s = append(s, v.String())
	}
	return "composelocal: the recipe is refused: " + strings.Join(s, "; ")
}

// rewrite adds what the adapter owns to a linted recipe: ports on loopback only, the limits and
// the labels. Everything else is the project's.
func (d *Deployer) rewrite(rendered []byte, rev, previous integration.Revision) ([]byte, error) {
	var cfg map[string]any
	if err := json.Unmarshal(rendered, &cfg); err != nil {
		return nil, fmt.Errorf("composelocal: %w", err)
	}
	services, _ := cfg["services"].(map[string]any)
	for name, raw := range services {
		svc, ok := raw.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("composelocal: service %q is not an object", name)
		}
		ports, _ := svc["ports"].([]any)
		for _, p := range ports {
			pm, ok := p.(map[string]any)
			if !ok {
				return nil, fmt.Errorf("composelocal: service %q has a port the adapter cannot read; it would not be bound to loopback", name)
			}
			pm["host_ip"] = "127.0.0.1"
		}
		// The project's own limits stay when it set them.
		if _, set := svc["cpus"]; !set && d.Cfg.CPUs > 0 {
			svc["cpus"] = d.Cfg.CPUs
		}
		if _, set := svc["mem_limit"]; !set && d.Cfg.Memory != "" {
			svc["mem_limit"] = d.Cfg.Memory
		}
		labels, _ := svc["labels"].(map[string]any)
		if labels == nil {
			labels = map[string]any{}
		}
		labels[labelRevision] = string(rev)
		if previous != "" {
			labels[labelPrevious] = string(previous)
		}
		svc["labels"] = labels
	}
	out, err := json.Marshal(cfg)
	if err != nil {
		return nil, fmt.Errorf("composelocal: %w", err)
	}
	return out, nil
}

// endpoint asks the engine where the entry service answers.
func (d *Deployer) endpoint(ctx context.Context, proj string) (integration.Endpoint, error) {
	if !serviceName.MatchString(d.Cfg.Entry) {
		return integration.Endpoint{}, fmt.Errorf("composelocal: %q is not a service name", d.Cfg.Entry)
	}
	out, err := d.compose(ctx, d.Work, proj, "ps", "--format", "json", d.Cfg.Entry)
	if err != nil {
		return integration.Endpoint{}, err
	}
	for _, c := range parsePS(out) {
		for _, p := range c.Publishers {
			if p.PublishedPort > 0 {
				return integration.Endpoint{URL: fmt.Sprintf("http://127.0.0.1:%d", p.PublishedPort)}, nil
			}
		}
	}
	return integration.Endpoint{}, fmt.Errorf("composelocal: service %q publishes no port; the preview has nothing to open", d.Cfg.Entry)
}

// Down removes the stack and its volumes; a stack that is not there is not an error.
func (d *Deployer) Down(ctx context.Context, e integration.EnvRef) error {
	if e.Task == "" {
		return fmt.Errorf("composelocal: production is not taken down; it changes by release or rollback: %w", integration.ErrUnsupported)
	}
	proj, err := project(e)
	if err != nil {
		return err
	}
	_, err = d.compose(ctx, d.Work, proj, "down", "--volumes", "--remove-orphans")
	return err
}

// Release brings production up at rev and returns the revision that was live before.
func (d *Deployer) Release(ctx context.Context, rev integration.Revision) (integration.Released, error) {
	prod := integration.EnvRef{Tenant: d.Cfg.Tenant}
	cur, err := d.Status(ctx, prod)
	if err != nil {
		return integration.Released{}, err
	}
	// Only a healthy revision is a way back: a half-upgraded stack must not become the rollback target.
	previous := cur.Previous
	if cur.State == integration.EnvHealthy && cur.Revision != rev {
		previous = cur.Revision
	}
	if _, err := d.up(ctx, prod, rev, previous); err != nil {
		return integration.Released{}, err
	}
	return integration.Released{Revision: rev, Previous: previous}, nil
}

// Rollback brings production up at the earlier revision.
func (d *Deployer) Rollback(ctx context.Context, to integration.Revision) error {
	_, err := d.up(ctx, integration.EnvRef{Tenant: d.Cfg.Tenant}, to, "")
	return err
}

// Smoke checks that every container is healthy, then runs the project's own check with the
// entry URL in $NS_URL.
func (d *Deployer) Smoke(ctx context.Context, e integration.EnvRef) (integration.SmokeResult, error) {
	st, err := d.Status(ctx, e)
	if err != nil {
		return integration.SmokeResult{}, err
	}
	if st.State != integration.EnvHealthy {
		return integration.SmokeResult{Detail: "the environment is " + string(st.State)}, nil
	}
	if d.Cfg.Smoke == "" {
		return integration.SmokeResult{OK: true}, nil
	}
	proj, err := project(e)
	if err != nil {
		return integration.SmokeResult{}, err
	}
	tree, done, err := d.Checkouts.Checkout(ctx, e, st.Revision)
	if err != nil {
		return integration.SmokeResult{}, fmt.Errorf("composelocal: checkout %s: %w", st.Revision, err)
	}
	defer done()
	ep, err := d.endpoint(ctx, proj)
	if err != nil {
		return integration.SmokeResult{}, err
	}
	out, err := d.Exec.Run(ctx, tree.Main, []string{"NS_URL=" + ep.URL}, "sh", "-c", d.Cfg.Smoke)
	if err != nil {
		return integration.SmokeResult{Detail: tail(string(out), err)}, nil
	}
	return integration.SmokeResult{OK: true}, nil
}

// Status reads the containers of the environment and their labels.
func (d *Deployer) Status(ctx context.Context, e integration.EnvRef) (integration.EnvStatus, error) {
	proj, err := project(e)
	if err != nil {
		return integration.EnvStatus{}, err
	}
	out, err := d.Exec.Run(ctx, d.Work, nil, "docker", "ps", "-a", "--filter", "label=com.docker.compose.project="+proj, "--format", "json")
	if err != nil {
		return integration.EnvStatus{}, fmt.Errorf("docker ps: %w", err)
	}
	return statusOf(parsePS(out)), nil
}

func tail(out string, err error) string {
	out = strings.TrimSpace(out)
	if r := []rune(out); len(r) > 400 {
		out = "…" + string(r[len(r)-400:])
	}
	if out == "" {
		return err.Error()
	}
	return out
}
