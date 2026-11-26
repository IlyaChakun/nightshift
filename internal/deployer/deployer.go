// Package deployer is the `deployer` service (design section 17, "Where it runs"): the only
// process with access to the Docker engine. The core calls it over HTTP with the token of the
// deployer; it checks out the revision from the read-only repository mirrors, reads the recipe
// from the base branch and runs the compose-local adapter. The worker has no route to it.
package deployer

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"

	"github.com/ilyachakun/nightshift/internal/adapters/deploy/composelocal"
	"github.com/ilyachakun/nightshift/internal/adapters/deploy/remote"
	"github.com/ilyachakun/nightshift/internal/gitops"
	"github.com/ilyachakun/nightshift/internal/integration"
)

const maxBody = 64 << 10

// keepProduction is how many production checkouts stay on disk: the live one, the one before
// it for a rollback, and one spare.
const keepProduction = 3

var (
	nameRe = regexp.MustCompile(`^[a-z0-9][a-z0-9-]*$`)
	revRe  = regexp.MustCompile(`^[0-9a-f]{40}$`)
	ops    = map[string]bool{"up": true, "down": true, "release": true, "smoke": true, "rollback": true, "status": true}
)

// Server serves POST /v1/{op}.
type Server struct {
	Token string
	Git   *gitops.Git
	// Work holds the checkouts. It must be the same absolute path inside this container and on the
	// Docker host: the engine resolves bind mount sources on the host.
	Work string
	Exec composelocal.Exec
	Log  *slog.Logger

	mu    sync.Mutex
	locks map[string]*sync.Mutex
}

// Handler returns the HTTP handler.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /v1/{op}", s.serve)
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusNoContent) })
	return mux
}

func reply(w http.ResponseWriter, code int, r remote.Response) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(r)
}

func (s *Server) lock(tenant string) func() {
	s.mu.Lock()
	if s.locks == nil {
		s.locks = map[string]*sync.Mutex{}
	}
	l, ok := s.locks[tenant]
	if !ok {
		l = &sync.Mutex{}
		s.locks[tenant] = l
	}
	s.mu.Unlock()
	l.Lock()
	return l.Unlock
}

func (s *Server) serve(w http.ResponseWriter, r *http.Request) {
	want := "Bearer " + s.Token
	if s.Token == "" || subtle.ConstantTimeCompare([]byte(r.Header.Get("Authorization")), []byte(want)) != 1 {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	op := r.PathValue("op")
	var req remote.Request
	if !ops[op] || json.NewDecoder(io.LimitReader(r.Body, maxBody)).Decode(&req) != nil {
		reply(w, http.StatusBadRequest, remote.Response{Error: "bad request"})
		return
	}
	if err := validate(req, op); err != nil {
		reply(w, http.StatusBadRequest, remote.Response{Error: err.Error()})
		return
	}
	defer s.lock(req.Tenant)() // one operation per tenant at a time; the adapter shares files per project
	res, err := s.run(r.Context(), op, req)
	if err != nil {
		resp := remote.Response{Error: err.Error()}
		switch {
		case errors.Is(err, integration.ErrRefused):
			resp.Kind = "refused"
		case errors.Is(err, integration.ErrUnsupported):
			resp.Kind = "unsupported"
		}
		if s.Log != nil {
			s.Log.WarnContext(r.Context(), "deploy operation failed", "op", op, "tenant", req.Tenant, "kind", resp.Kind, "error", err)
		}
		reply(w, http.StatusOK, resp)
		return
	}
	reply(w, http.StatusOK, res)
}

func validate(req remote.Request, op string) error {
	if !nameRe.MatchString(req.Tenant) {
		return errors.New("bad tenant")
	}
	if req.Env.Task != "" && !nameRe.MatchString(req.Env.Task) {
		return errors.New("bad task")
	}
	if (op == "up" || op == "release" || op == "rollback") && !revRe.MatchString(string(req.Rev)) {
		return errors.New("a revision is a full commit hash")
	}
	if req.Spec.File == "" || filepath.IsAbs(req.Spec.File) || !filepath.IsLocal(req.Spec.File) {
		return errors.New("the recipe must be a path inside the repository")
	}
	if req.Spec.Base == "" {
		return errors.New("no base branch")
	}
	return nil
}

func (s *Server) adapter(req remote.Request) (*composelocal.Deployer, *checkouts) {
	co := &checkouts{git: s.Git, repo: req.Repo, spec: req.Spec, root: filepath.Join(s.Work, req.Tenant)}
	return &composelocal.Deployer{
		Cfg:       composelocal.Config{Tenant: req.Tenant, Entry: req.Spec.Entry, CPUs: req.Spec.CPUs, Memory: req.Spec.Memory, Smoke: req.Spec.Smoke},
		Checkouts: co, Exec: s.Exec, Work: filepath.Join(co.root, "render"),
	}, co
}

func (s *Server) run(ctx context.Context, op string, req remote.Request) (remote.Response, error) {
	d, co := s.adapter(req)
	switch op {
	case "up":
		ep, err := d.Up(ctx, req.Env, req.Rev)
		return remote.Response{Endpoint: &ep}, err
	case "down":
		if err := d.Down(ctx, req.Env); err != nil {
			return remote.Response{}, err
		}
		return remote.Response{}, co.forget(req.Env)
	case "release":
		rel, err := d.Release(ctx, req.Rev)
		if err == nil {
			co.prune(keepProduction)
		}
		return remote.Response{Released: &rel}, err
	case "rollback":
		return remote.Response{}, d.Rollback(ctx, req.Rev)
	case "smoke":
		sm, err := d.Smoke(ctx, req.Env)
		return remote.Response{Smoke: &sm}, err
	default:
		st, err := d.Status(ctx, req.Env)
		return remote.Response{Status: &st}, err
	}
}

// checkouts keeps one checkout per environment and revision under root, and one clean checkout
// of the base branch for the project's own smoke check.
type checkouts struct {
	git  *gitops.Git
	repo string
	spec remote.Spec
	root string
}

// within returns p when it is inside the work directory of the tenant. Every name in the path was
// validated on the way in; this is the second check, at the place where files are removed and written.
func (c *checkouts) within(p string) (string, error) {
	rel, err := filepath.Rel(c.root, p)
	if err != nil || !filepath.IsLocal(rel) {
		return "", fmt.Errorf("deployer: %q is outside the work directory", p)
	}
	return filepath.Join(c.root, rel), nil
}

func (c *checkouts) envDir(e integration.EnvRef) string {
	if e.Task == "" {
		return filepath.Join(c.root, "prod")
	}
	return filepath.Join(c.root, "preview-"+e.Task)
}

// Checkout implements composelocal.Checkouts.
func (c *checkouts) Checkout(ctx context.Context, e integration.EnvRef, rev integration.Revision) (composelocal.Tree, func(), error) {
	noop := func() {}
	mirror, err := c.git.MirrorPath(c.repo)
	if err != nil {
		return composelocal.Tree{}, noop, fmt.Errorf("deployer: %w", err)
	}
	baseRaw, err := c.git.Rev(ctx, mirror, c.spec.Base)
	if err != nil {
		return composelocal.Tree{}, noop, fmt.Errorf("deployer: %w", err)
	}
	base := strings.TrimSpace(baseRaw)
	dir := filepath.Join(c.envDir(e), string(rev))
	// A revision's content never changes, so a finished checkout is kept: a replayed release must
	// not delete the files that running containers have mounted.
	if err := c.ensure(ctx, mirror, dir, string(rev)); err != nil {
		return composelocal.Tree{}, noop, err
	}
	// The recipe is the one on the base branch: the change under test cannot rewrite the rules it runs under.
	recipe, ok, err := c.git.Show(ctx, mirror, base, c.spec.File)
	if err != nil || !ok {
		return composelocal.Tree{}, noop, fmt.Errorf("deployer: recipe %s is not on %s: %w", c.spec.File, c.spec.Base, err)
	}
	path, err := c.within(filepath.Join(dir, filepath.FromSlash(c.spec.File)))
	if err != nil {
		return composelocal.Tree{}, noop, err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		return composelocal.Tree{}, noop, fmt.Errorf("deployer: %w", err)
	}
	// A symlink at the recipe's path would send this write outside the checkout.
	if err := os.RemoveAll(path); err != nil {
		return composelocal.Tree{}, noop, fmt.Errorf("deployer: %w", err)
	}
	if err := os.WriteFile(path, []byte(recipe), 0o600); err != nil {
		return composelocal.Tree{}, noop, fmt.Errorf("deployer: %w", err)
	}
	mainDir := filepath.Join(c.root, "main", base)
	if err := c.ensure(ctx, mirror, mainDir, base); err != nil {
		return composelocal.Tree{}, noop, err
	}
	return composelocal.Tree{Dir: dir, Recipe: path, Main: mainDir}, noop, nil
}

// fresh makes dir a new checkout of rev.
func (c *checkouts) fresh(ctx context.Context, mirror, dir, rev string) error {
	dir, err := c.within(dir)
	if err != nil {
		return err
	}
	if err := os.RemoveAll(dir); err != nil {
		return fmt.Errorf("deployer: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(dir), 0o750); err != nil {
		return fmt.Errorf("deployer: %w", err)
	}
	if err := c.git.Checkout(ctx, mirror, dir, rev); err != nil {
		return fmt.Errorf("deployer: checkout %s: %w", rev, err)
	}
	// The marker says the checkout finished; a crash before it leaves a directory that is redone.
	if err := os.WriteFile(filepath.Join(dir, readyMarker), nil, 0o600); err != nil {
		return fmt.Errorf("deployer: %w", err)
	}
	return nil
}

const readyMarker = ".ns-checkout-ready"

// ensure checks rev out into dir unless a finished checkout is already there.
func (c *checkouts) ensure(ctx context.Context, mirror, dir, rev string) error {
	if _, err := os.Stat(filepath.Join(dir, readyMarker)); err == nil {
		return nil
	}
	return c.fresh(ctx, mirror, dir, rev)
}

// forget removes the checkouts of an environment that was taken down.
func (c *checkouts) forget(e integration.EnvRef) error {
	if e.Task == "" {
		return nil
	}
	dir, err := c.within(c.envDir(e))
	if err != nil {
		return err
	}
	if err := os.RemoveAll(dir); err != nil {
		return fmt.Errorf("deployer: %w", err)
	}
	return nil
}

// prune keeps the newest n production checkouts and the newest n of main.
func (c *checkouts) prune(n int) {
	for _, dir := range []string{filepath.Join(c.root, "prod"), filepath.Join(c.root, "main")} {
		entries, err := os.ReadDir(dir)
		if err != nil || len(entries) <= n {
			continue
		}
		type item struct {
			name string
			mod  int64
		}
		var items []item
		for _, e := range entries {
			if info, err := e.Info(); err == nil {
				items = append(items, item{e.Name(), info.ModTime().UnixNano()})
			}
		}
		sort.Slice(items, func(i, j int) bool { return items[i].mod > items[j].mod })
		for _, it := range items[min(n, len(items)):] {
			_ = os.RemoveAll(filepath.Join(dir, it.name))
		}
	}
}
