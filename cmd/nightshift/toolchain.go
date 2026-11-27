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

	"github.com/ilyachakun/nightshift/internal/customization/config"
	"github.com/ilyachakun/nightshift/internal/toolchain"
)

// toolchainCmd serves the install service: the worker asks it to run `mise install` for a
// job's workspace, so the worker itself needs no way to the tool hosts (ADR 0010).
func toolchainCmd(args []string, stderr io.Writer) int {
	fs := flag.NewFlagSet("toolchain", flag.ContinueOnError)
	fs.SetOutput(stderr)
	listen := fs.String("listen", ":8082", "address of the install API")
	secrets := fs.String("secrets", config.DefaultSecretsDir, "directory with the worker token")
	root := fs.String("work", "/work", "task checkouts; installs are refused outside it")
	agentUID := fs.Int("agent-uid", 0, "install as this uid; 0 keeps the current user")
	agentGID := fs.Int("agent-gid", 0, "group of the agent user")
	if err := fs.Parse(args); err != nil {
		return exitUsage
	}
	log := slog.New(slog.NewJSONHandler(stderr, nil))
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	token, err := (config.Secrets{Dir: *secrets}).Read(secretWorkerToken)
	if err != nil {
		log.ErrorContext(ctx, "toolchain failed", "error", err)
		return exitFailure
	}
	s := &toolchain.Server{
		Token: strings.TrimSpace(token), Root: *root,
		AgentHome: agentHome(*agentUID), AgentUID: *agentUID, AgentGID: *agentGID,
	}
	srv := &http.Server{Addr: *listen, Handler: s.Handler(), ReadHeaderTimeout: 5 * time.Second}
	go func() {
		<-ctx.Done()
		_ = srv.Shutdown(context.WithoutCancel(ctx))
	}()
	log.InfoContext(ctx, "toolchain service listening", "addr", *listen)
	if err := srv.ListenAndServe(); !errors.Is(err, http.ErrServerClosed) {
		log.ErrorContext(ctx, "toolchain failed", "error", err)
		return exitFailure
	}
	return exitOK
}
