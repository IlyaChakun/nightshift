package main

import (
	"context"
	"database/sql"
	"errors"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"sort"
	"syscall"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	_ "github.com/jackc/pgx/v5/stdlib" // database/sql driver "pgx" for migrations
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"

	"github.com/ilyachakun/nightshift/internal/adapters"
	"github.com/ilyachakun/nightshift/internal/book"
	"github.com/ilyachakun/nightshift/internal/config"
	"github.com/ilyachakun/nightshift/internal/dispatch"
)

// Secrets that hold Postgres connection URLs: the schema owner runs migrations,
// the service runs as the nightshift role.
const (
	secretOwnerURL   = "pg_owner"
	secretServiceURL = "pg_nightshift"
)

const defaultListen = "127.0.0.1:8080"

func serve(args []string, stderr io.Writer) int {
	fs := flag.NewFlagSet("serve", flag.ContinueOnError)
	fs.SetOutput(stderr)
	dir := fs.String("config", "/etc/nightshift", "directory with instance.yaml")
	secrets := fs.String("secrets", config.DefaultSecretsDir, "directory with compose secrets")
	listen := fs.String("listen", defaultListen, "address of the health endpoint")
	if err := fs.Parse(args); err != nil {
		return exitUsage
	}
	log := slog.New(slog.NewJSONHandler(stderr, nil))
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	if err := runServe(ctx, log, *dir, config.Secrets{Dir: *secrets}, *listen); err != nil {
		log.Error("serve failed", "error", err)
		return exitFailure
	}
	return exitOK
}

func runServe(ctx context.Context, log *slog.Logger, dir string, sec config.Secrets, listen string) error {
	inst, err := config.LoadInstance(config.NewValidator(), dir)
	if err != nil {
		return err
	}
	ownerURL, err := sec.Read(secretOwnerURL)
	if err != nil {
		return err
	}
	serviceURL, err := sec.Read(secretServiceURL)
	if err != nil {
		return err
	}
	if err := migrate(ctx, ownerURL); err != nil {
		return err
	}
	pool, err := pgxpool.New(ctx, serviceURL)
	if err != nil {
		return fmt.Errorf("connect: %w", err)
	}
	defer pool.Close()

	routes, chats, known, err := chatRoutes(inst, sec, log)
	if err != nil {
		return err
	}
	shutdownTracing, err := tracing(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = shutdownTracing(context.WithoutCancel(ctx)) }()

	mask := dispatch.NewMasker(append(known, ownerURL, serviceURL)...)
	outbox := &dispatch.Outbox{Pool: pool, Senders: map[string]dispatch.Sender{dispatch.ChatTarget: chats}, Mask: mask, Log: log}
	svc := &dispatch.Service{
		Pool: pool, Routes: routes, Inbox: &dispatch.Inbox{Pool: pool, Outbox: outbox, Log: log},
		Outbox: outbox, FlushEvery: 5 * time.Second, Log: log,
	}
	health := &http.Server{Addr: listen, Handler: healthHandler(pool), ReadHeaderTimeout: 5 * time.Second}
	go func() {
		if err := health.ListenAndServe(); !errors.Is(err, http.ErrServerClosed) {
			log.Error("health endpoint stopped", "error", err)
		}
	}()
	defer func() { _ = health.Shutdown(context.WithoutCancel(ctx)) }()
	return svc.Run(ctx)
}

func migrate(ctx context.Context, url string) error {
	db, err := sql.Open("pgx", url)
	if err != nil {
		return fmt.Errorf("migrate: open: %w", err)
	}
	defer func() { _ = db.Close() }()
	if err := book.Migrate(ctx, db); err != nil {
		return err
	}
	return dispatch.Migrate(ctx, db)
}

// chatRoutes builds one adapter per adapter name and routes its tenants to it.
func chatRoutes(inst *config.Instance, sec config.Secrets, log *slog.Logger) ([]dispatch.Route, dispatch.ChatSender, []string, error) {
	byAdapter := map[string]*dispatch.Route{}
	chats := dispatch.ChatSender{}
	var known []string
	names := make([]string, 0, len(inst.Tenants))
	for name := range inst.Tenants {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		t := inst.Tenants[name]
		r, ok := byAdapter[t.Chat.Adapter]
		if !ok {
			chat, secrets, err := adapters.NewChat(t.Chat.Adapter, sec, log)
			if err != nil {
				return nil, nil, nil, fmt.Errorf("tenant %s: %w", name, err)
			}
			known = append(known, secrets...)
			r = &dispatch.Route{Chat: chat}
			byAdapter[t.Chat.Adapter] = r
		}
		r.Tenants = append(r.Tenants, t)
		chats[name] = r.Chat
	}
	routes := make([]dispatch.Route, 0, len(byAdapter))
	for _, r := range byAdapter {
		routes = append(routes, *r)
	}
	return routes, chats, known, nil
}

// tracing exports spans over OTLP when OTEL_EXPORTER_OTLP_ENDPOINT is set; otherwise
// spans stay in memory and are dropped.
func tracing(ctx context.Context) (func(context.Context) error, error) {
	if os.Getenv("OTEL_EXPORTER_OTLP_ENDPOINT") == "" {
		return func(context.Context) error { return nil }, nil
	}
	exp, err := otlptracehttp.New(ctx)
	if err != nil {
		return nil, fmt.Errorf("tracing: %w", err)
	}
	tp := sdktrace.NewTracerProvider(sdktrace.WithBatcher(exp))
	otel.SetTracerProvider(tp)
	return tp.Shutdown, nil
}

type pinger interface{ Ping(context.Context) error }

func healthHandler(db pinger) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
		defer cancel()
		if err := db.Ping(ctx); err != nil {
			http.Error(w, "book unreachable", http.StatusServiceUnavailable)
			return
		}
		_, _ = io.WriteString(w, "ok\n")
	})
	return mux
}

// status --health asks the local health endpoint; compose uses it as the healthcheck.
func status(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("status", flag.ContinueOnError)
	fs.SetOutput(stderr)
	health := fs.Bool("health", false, "check the health endpoint and exit")
	addr := fs.String("addr", defaultListen, "address of the health endpoint")
	if err := fs.Parse(args); err != nil {
		return exitUsage
	}
	if !*health {
		_, _ = fmt.Fprintln(stderr, "nightshift status: only --health is implemented yet (design phase 1)")
		return exitNotImplemented
	}
	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Get("http://" + *addr + "/healthz")
	if err != nil {
		_, _ = fmt.Fprintln(stderr, "unhealthy:", err)
		return exitUnhealthy
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		_, _ = fmt.Fprintln(stderr, "unhealthy:", resp.Status)
		return exitUnhealthy
	}
	_, _ = fmt.Fprintln(stdout, "healthy")
	return exitOK
}
