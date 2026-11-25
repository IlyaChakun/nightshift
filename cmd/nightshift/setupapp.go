package main

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"errors"
	"flag"
	"fmt"
	"html/template"
	"io"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/ilyachakun/nightshift/internal/adapters/github"
)

const setupUsage = "nightshift setup: usage: setup github-app --name <app name> --out <secrets dir> | setup chat-app --manifest <file>"

// nightshiftUID reads the App's secrets in compose (docs/secrets.md).
const nightshiftUID = 10001

func setupCmd(args []string, stdout, stderr io.Writer) int {
	if len(args) > 0 && args[0] == "chat-app" {
		return setupChatApp(args[1:], stdout, stderr)
	}
	if len(args) == 0 || args[0] != "github-app" {
		_, _ = fmt.Fprintln(stderr, setupUsage)
		return exitUsage
	}
	fs := flag.NewFlagSet("setup github-app", flag.ContinueOnError)
	fs.SetOutput(stderr)
	name := fs.String("name", "", "name of the new app, unique on GitHub")
	out := fs.String("out", "", "secrets directory to write github_app_id and github_app_key into")
	homepage := fs.String("homepage", "https://github.com/ilyachakun/nightshift", "homepage shown on the app")
	listen := fs.String("listen", "127.0.0.1:8765", "address to serve the one page on")
	browse := fs.String("url", "http://127.0.0.1:8765", "address of that page as your browser sees it")
	replace := fs.Bool("replace", false, "overwrite an app that is already in the secrets")
	if err := fs.Parse(args[1:]); err != nil {
		return exitUsage
	}
	if *name == "" || *out == "" {
		_, _ = fmt.Fprintln(stderr, setupUsage)
		return exitUsage
	}
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	err := func() error {
		// Look before GitHub creates an app whose key could then not be stored.
		if err := writable(*out, *replace); err != nil {
			return err
		}
		manifest, err := github.Manifest(*name, *homepage, *browse+"/created")
		if err != nil {
			return err
		}
		ln, err := (&net.ListenConfig{}).Listen(ctx, "tcp", *listen)
		if err != nil {
			return err
		}
		_, _ = fmt.Fprintf(stdout, "open %s in your browser and confirm on GitHub (one hour)\n", *browse)
		app, err := register(ctx, ln, &appSetup{manifest: manifest, state: rand.Text(), api: &github.Client{}, out: *out})
		if err != nil {
			return err
		}
		_, _ = fmt.Fprintf(stdout, "app %s (id %s) is in %s\nnow install it on the tenant's repositories only: %s/installations/new\n", app.Slug, app.ID, *out, app.Page)
		return nil
	}()
	if err != nil {
		_, _ = fmt.Fprintln(stderr, "nightshift setup github-app:", err)
		return exitFailure
	}
	return exitOK
}

// appSetup is one registration: the page that posts the manifest and the address GitHub
// sends the browser back to.
type appSetup struct {
	manifest, state, out string
	api                  *github.Client
	done                 chan registration
}

type registration struct {
	app github.Registered
	err error
}

var startPage = template.Must(template.New("start").Parse(`<!doctype html><meta charset="utf-8"><title>Nightshift GitHub App</title>
<p>GitHub shows the app and its permissions; confirm there.</p>
<form method="post" action="{{.Action}}"><input type="hidden" name="manifest" value="{{.Manifest}}"><button>Create the GitHub App</button></form>
`))

func (s *appSetup) handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /{$}", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_ = startPage.Execute(w, map[string]string{"Action": github.NewAppURL + "?state=" + s.state, "Manifest": s.manifest})
	})
	mux.HandleFunc("GET /created", func(w http.ResponseWriter, r *http.Request) {
		// The state shows that this visit answers our page and not a link someone else made.
		if subtle.ConstantTimeCompare([]byte(r.URL.Query().Get("state")), []byte(s.state)) != 1 {
			http.Error(w, "this answer is not for this setup", http.StatusForbidden)
			return
		}
		app, err := s.api.ConvertManifest(r.Context(), r.URL.Query().Get("code"))
		if err == nil {
			err = storeApp(s.out, app)
		}
		if err != nil {
			http.Error(w, "failed; see the terminal", http.StatusInternalServerError)
		} else {
			_, _ = io.WriteString(w, "The app is stored. Go back to the terminal.\n")
		}
		select {
		case s.done <- registration{app, err}:
		default: // a second visit changes nothing
		}
	})
	return mux
}

// register serves the page until GitHub has answered once.
func register(ctx context.Context, ln net.Listener, s *appSetup) (github.Registered, error) {
	s.done = make(chan registration, 1)
	srv := &http.Server{Handler: s.handler(), ReadHeaderTimeout: 5 * time.Second}
	served := make(chan error, 1)
	go func() { served <- srv.Serve(ln) }()
	ctx, cancel := context.WithTimeout(ctx, time.Hour)
	defer cancel()
	var res registration
	select {
	case res = <-s.done:
	case <-ctx.Done():
		res.err = errors.New("nobody confirmed the app")
	}
	shutdown, stop := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer stop()
	_ = srv.Shutdown(shutdown)
	if err := <-served; !errors.Is(err, http.ErrServerClosed) && res.err == nil {
		res.err = err
	}
	return res.app, res.err
}

// writable refuses to replace an app that is already stored, unless asked to.
func writable(dir string, replace bool) error {
	if info, err := os.Stat(dir); err != nil || !info.IsDir() {
		return fmt.Errorf("%s is not a directory; run init-secrets.sh first", dir)
	}
	for _, name := range []string{secretGitHubAppID, secretGitHubAppKey} {
		info, err := os.Stat(filepath.Join(dir, name))
		if err == nil && info.Size() > 0 && !replace {
			return fmt.Errorf("%s already holds an app; pass --replace to overwrite it", name)
		}
	}
	return nil
}

// storeApp writes the two secrets, each readable by its owner only.
func storeApp(dir string, app github.Registered) error {
	for name, value := range map[string]string{secretGitHubAppID: app.ID, secretGitHubAppKey: app.PrivateKey} {
		path := filepath.Join(dir, name)
		if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("store %s: %w", name, err)
		}
		if err := os.WriteFile(path, []byte(value), 0o400); err != nil {
			return fmt.Errorf("store %s: %w", name, err)
		}
		// Only root may give a file away; Docker Desktop and tests need not.
		_ = os.Chown(path, nightshiftUID, -1)
	}
	return nil
}
