// Package deployer is the `deployer` service (design section 17, "Where it runs"): the only
// process with access to the Docker engine. The core calls it over HTTP with the token of the
// deployer; it checks out the revision from the read-only repository mirrors, reads the recipe
// from the base branch and runs the compose-local adapter. The worker has no route to it.
package deployer

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
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
	"github.com/ilyachakun/nightshift/internal/execution/deployapi"
	"github.com/ilyachakun/nightshift/internal/execution/gitops"
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
	// PreviewNetwork is the instance's preview network (composelocal.Config.PreviewNetwork).
	PreviewNetwork string

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
	_ = json.NewEncoder(w).Encode(r) //nolint:errchkjson // a plain struct always encodes; a write error means the client is gone
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
		resp.Released = res.Released // a failed rollout still says where production can go back to
		switch {
		case errors.Is(err, deployapi.ErrRolloutFailed):
			resp.Kind = "rollout_failed"
		case errors.Is(err, deployapi.ErrRefused):
			resp.Kind = "refused"
		case errors.Is(err, deployapi.ErrUnsupported):
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
	if req.Spec.Smoke != "" {
		return errors.New("project smoke commands are not run by the deployer")
	}
	if req.Spec.Base == "" {
		return errors.New("no base branch")
	}
	return nil
}

func (s *Server) adapter(req remote.Request) (*composelocal.Deployer, *checkouts, error) {
	co := &checkouts{git: s.Git, repo: req.Repo, spec: req.Spec, work: s.Work, root: filepath.Join(s.Work, req.Tenant)}
	render, err := co.within(filepath.Join(co.root, "render"))
	if err != nil {
		return nil, nil, err
	}
	// Every docker call runs in it, so it must exist before a tenant's first operation, which is
	// often Status or a first Release, not an Up.
	if err := os.MkdirAll(render, 0o700); err != nil {
		return nil, nil, fmt.Errorf("deployer: %w", err)
	}
	return &composelocal.Deployer{
		Cfg:       composelocal.Config{Tenant: req.Tenant, Entry: req.Spec.Entry, CPUs: req.Spec.CPUs, Memory: req.Spec.Memory, Smoke: req.Spec.Smoke, PreviewNetwork: s.PreviewNetwork},
		Checkouts: co, Exec: s.Exec, Work: render,
	}, co, nil
}

func (s *Server) run(ctx context.Context, op string, req remote.Request) (remote.Response, error) {
	d, co, err := s.adapter(req)
	if err != nil {
		return remote.Response{}, err
	}
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
	work string // the deployer's own work directory, from its flag
	root string // the tenant's part of it
}

// within returns p when it is inside the work directory of the tenant. Every name in the path was
// validated on the way in; this is the second check, at the place where files are removed and written.
func (c *checkouts) within(p string) (string, error) {
	clean := filepath.Clean(p)
	sep := string(filepath.Separator)
	if !strings.HasPrefix(clean, filepath.Clean(c.work)+sep) || !strings.HasPrefix(clean, filepath.Clean(c.root)+sep) {
		return "", fmt.Errorf("deployer: %q is outside the work directory", p)
	}
	return clean, nil
}

func (c *checkouts) envDir(e deployapi.EnvRef) string {
	if e.Task == "" {
		return filepath.Join(c.root, "prod")
	}
	return filepath.Join(c.root, "preview-"+e.Task)
}

// Checkout implements composelocal.Checkouts.
func (c *checkouts) Checkout(ctx context.Context, e deployapi.EnvRef, rev deployapi.Revision) (composelocal.Tree, func(), error) {
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
	// The checkout is the change's: a link among the recipe's directories would send the write
	// below, and the engine's read after it, outside the checkout. Any link is refused, also one
	// that stays inside: the recipe's place is the one main gives it.
	for p := filepath.Dir(path); len(p) > len(dir); p = filepath.Dir(p) {
		if fi, err := os.Lstat(p); err == nil && fi.Mode()&os.ModeSymlink != 0 {
			return composelocal.Tree{}, noop, fmt.Errorf("deployer: the recipe's directory %s is a link: %w", filepath.Base(p), deployapi.ErrRefused)
		}
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
	// The checkout may carry the marker's name as a link; it is replaced, never written through.
	marker := filepath.Join(dir, readyMarker)
	if err := os.RemoveAll(marker); err != nil {
		return fmt.Errorf("deployer: %w", err)
	}
	f, err := os.OpenFile(marker, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600) //nolint:gosec // G304: a fixed name in the deployer's own checkout
	if err != nil {
		return fmt.Errorf("deployer: %w", err)
	}
	return f.Close()
}

const readyMarker = ".ns-checkout-ready"

// ensure checks rev out into dir unless a finished checkout is already there.
func (c *checkouts) ensure(ctx context.Context, mirror, dir, rev string) error {
	dir, err := c.within(dir)
	if err != nil {
		return err
	}
	if fi, err := os.Lstat(filepath.Join(dir, readyMarker)); err == nil && fi.Mode().IsRegular() {
		return nil
	}
	return c.fresh(ctx, mirror, dir, rev)
}

// Render implements composelocal.Checkouts: the rewritten recipe goes to a file named by a hash of
// the project, directly under the tenant's render directory.
func (c *checkouts) Render(proj string, data []byte) (string, error) {
	sum := sha256.Sum256([]byte(proj))
	file, err := c.within(filepath.Join(c.root, "render", hex.EncodeToString(sum[:8])+".json"))
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(filepath.Dir(file), 0o700); err != nil {
		return "", fmt.Errorf("deployer: %w", err)
	}
	if err := os.WriteFile(file, data, 0o600); err != nil {
		return "", fmt.Errorf("deployer: %w", err)
	}
	return file, nil
}

// forget removes the checkouts of an environment that was taken down. The directory is found by
// listing the tenant's folder, so the name that is removed comes from the disk, not from the request.
func (c *checkouts) forget(e deployapi.EnvRef) error {
	if e.Task == "" {
		return nil
	}
	if !nameRe.MatchString(e.Task) {
		return errors.New("deployer: bad task name")
	}
	root := filepath.Clean(c.root)
	if !strings.HasPrefix(root, filepath.Clean(c.work)+string(filepath.Separator)) {
		return fmt.Errorf("deployer: %q is outside the work directory", root)
	}
	entries, err := os.ReadDir(root)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("deployer: %w", err)
	}
	for _, ent := range entries {
		if ent.Name() != "preview-"+e.Task {
			continue
		}
		if err := os.RemoveAll(filepath.Join(root, ent.Name())); err != nil {
			return fmt.Errorf("deployer: %w", err)
		}
	}
	return nil
}

// prune keeps the newest n production checkouts and the newest n of main.
func (c *checkouts) prune(n int) {
	for _, dir := range []string{filepath.Join(c.root, "prod"), filepath.Join(c.root, "main")} {
		dir, err := c.within(dir)
		if err != nil {
			continue
		}
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
			if old, err := c.within(filepath.Join(dir, it.name)); err == nil {
				_ = os.RemoveAll(old)
			}
		}
	}
}
