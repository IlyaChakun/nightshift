package main

import (
	"context"
	"errors"
	"flag"
	"io"
	"log/slog"
	"net/http"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/ilyachakun/nightshift/internal/adapters/deploy/composelocal"
	"github.com/ilyachakun/nightshift/internal/customization/config"
	"github.com/ilyachakun/nightshift/internal/execution/deployer"
	"github.com/ilyachakun/nightshift/internal/execution/gitops"
)

// secretDeployerToken is the file under the secrets directory that authenticates the core's
// calls to the deployer; the worker never holds it.
const secretDeployerToken = "deployer_token"

// deployerCmd serves the deploy service: the core asks it to bring a preview or a release up,
// and it runs `docker compose` against the engine it is given. It is the only process with
// access to the engine (design section 17, "Where it runs").
func deployerCmd(args []string, stderr io.Writer) int {
	fs := flag.NewFlagSet("deployer", flag.ContinueOnError)
	fs.SetOutput(stderr)
	listen := fs.String("listen", ":8083", "address of the deploy API")
	secrets := fs.String("secrets", config.DefaultSecretsDir, "directory with the deployer token")
	repos := fs.String("repos", "/repos", "repository mirrors, read-only")
	work := fs.String("work", "", "checkouts of previews and releases; the same absolute path inside the container and on the Docker host")
	if err := fs.Parse(args); err != nil {
		return exitUsage
	}
	log := slog.New(slog.NewJSONHandler(stderr, nil))
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	if *work == "" {
		log.ErrorContext(ctx, "deployer failed", "error", "--work is required: it is bind mounted at the same path on the host")
		return exitUsage
	}
	token, err := (config.Secrets{Dir: *secrets}).Read(secretDeployerToken)
	if err != nil {
		log.ErrorContext(ctx, "deployer failed", "error", err)
		return exitFailure
	}
	s := &deployer.Server{Token: strings.TrimSpace(token), Git: &gitops.Git{Dir: *repos}, Work: *work, Exec: composelocal.OSExec{}, Log: log}
	srv := &http.Server{Addr: *listen, Handler: s.Handler(), ReadHeaderTimeout: 5 * time.Second}
	go func() {
		<-ctx.Done()
		_ = srv.Shutdown(context.WithoutCancel(ctx))
	}()
	log.InfoContext(ctx, "deployer listening", "addr", *listen)
	if err := srv.ListenAndServe(); !errors.Is(err, http.ErrServerClosed) {
		log.ErrorContext(ctx, "deployer failed", "error", err)
		return exitFailure
	}
	return exitOK
}
