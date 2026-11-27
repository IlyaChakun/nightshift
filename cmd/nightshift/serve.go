package main

import (
	"context"
	"database/sql"
	"errors"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"log/slog"
	"maps"
	"net/http"
	"os"
	"os/signal"
	"slices"
	"sort"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	_ "github.com/jackc/pgx/v5/stdlib" // database/sql driver "pgx" for migrations
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"

	"github.com/ilyachakun/nightshift/internal/adapters"
	"github.com/ilyachakun/nightshift/internal/adapters/chat/inbox"
	"github.com/ilyachakun/nightshift/internal/control/chatapi"
	"github.com/ilyachakun/nightshift/internal/control/doors"
	"github.com/ilyachakun/nightshift/internal/control/flow"
	"github.com/ilyachakun/nightshift/internal/control/jobserver"
	"github.com/ilyachakun/nightshift/internal/control/mask"
	"github.com/ilyachakun/nightshift/internal/control/outbox"
	"github.com/ilyachakun/nightshift/internal/control/trackerapi"
	"github.com/ilyachakun/nightshift/internal/control/tracking"
	"github.com/ilyachakun/nightshift/internal/control/trustedgit"
	"github.com/ilyachakun/nightshift/internal/customization/config"
	"github.com/ilyachakun/nightshift/internal/data/book"
)

// Secrets that hold Postgres connection URLs. Only the one-shot --migrate-only step reads
// the schema owner's URL; the long-running service holds just its own (design section 3).
const (
	secretOwnerURL    = "pg_owner"
	secretServiceURL  = "pg_nightshift"
	secretWorkerHash  = "worker_token_sha256"
	secretWorkerToken = "worker_token"
	// The password Grafana logs in with as the dashboard role. Optional: without it the role
	// keeps no login and the obs profile shows no book dashboards.
	secretGrafanaPassword = "pg_grafana_password" //nolint:gosec // G101: the name of a secret file, not a credential
)

const defaultListen = "127.0.0.1:8080"

func serve(args []string, stderr io.Writer) int {
	fs := flag.NewFlagSet("serve", flag.ContinueOnError)
	fs.SetOutput(stderr)
	dir := fs.String("config", "/etc/nightshift", "directory with instance.yaml")
	secrets := fs.String("secrets", config.DefaultSecretsDir, "directory with compose secrets")
	listen := fs.String("listen", defaultListen, "address of the health endpoint")
	jobsListen := fs.String("jobs-listen", "", "address of the jobs API for the worker; empty turns it off")
	repos := fs.String("repos", "", "clone directory of the conductor; empty keeps the conductor off")
	artifacts := fs.String("artifacts", "/artifacts", "job artifacts written by the worker (read-only)")
	rolesDir := fs.String("roles", "/roles", "role definitions (roles/<name>.yaml)")
	standardsDir := fs.String("standards", "/standards", "standards packs for the L0 checklist")
	deployerURL := fs.String("deployer-url", "", "address of the deploy service; empty disables previews and holds configured releases")
	migrateOnly := fs.Bool("migrate-only", false, "apply migrations as the schema owner and exit")
	if err := fs.Parse(args); err != nil {
		return exitUsage
	}
	log := slog.New(slog.NewJSONHandler(stderr, nil))
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	sec := config.Secrets{Dir: *secrets}
	var err error
	if *migrateOnly {
		err = migrate(ctx, sec)
	} else {
		err = runServe(ctx, log, *dir, sec, *listen, *jobsListen, conductorDirs{Repos: *repos, Artifacts: *artifacts, Roles: *rolesDir, Standards: *standardsDir, DeployerURL: *deployerURL})
	}
	if err != nil {
		log.Error("serve failed", "error", err)
		return exitFailure
	}
	return exitOK
}

func runServe(ctx context.Context, log *slog.Logger, dir string, sec config.Secrets, listen, jobsListen string, dirs conductorDirs) error {
	inst, err := config.LoadInstance(config.NewValidator(), dir)
	if err != nil {
		return err
	}
	serviceURL, err := sec.Read(secretServiceURL)
	if err != nil {
		return err
	}
	pool, err := pgxpool.New(ctx, serviceURL)
	if err != nil {
		return fmt.Errorf("connect: %w", err)
	}
	defer pool.Close()

	routes, chats, known, err := chatRoutes(inst, sec, log, pool)
	if err != nil {
		return err
	}
	shutdownTracing, err := tracing(ctx, "nightshift")
	if err != nil {
		return err
	}
	defer func() { _ = shutdownTracing(context.WithoutCancel(ctx)) }()

	masker := mask.NewMasker(append(known, serviceURL)...)
	// The outbox logger is also River's; River reports routine job counts at info level.
	quiet := slog.New(warnOnly{log.Handler()})
	trackers := trackerFor(inst, log)
	ob := &outbox.Outbox{Pool: pool, Senders: map[string]outbox.Sender{
		outbox.ChatTarget:      chats,
		tracking.TrackerTarget: tracking.TrackerSender{Pool: pool, Trackers: trackers},
	}, Mask: masker, Log: quiet, Hold: outbox.QuietHold(inst.Tenants, log)}
	svc := &doors.Service{
		Pool: pool, Routes: routes, Inbox: &doors.Inbox{Pool: pool, Outbox: ob, Mover: &flow.Mover{Pool: pool}, Log: log},
		Outbox: ob, FlushEvery: 5 * time.Second, Log: log,
	}
	health := &http.Server{Addr: listen, Handler: healthHandler(pool), ReadHeaderTimeout: 5 * time.Second}
	// Shutdown runs first and makes ListenAndServe return; Wait then joins the goroutine.
	var wg sync.WaitGroup
	defer wg.Wait()
	// Cancel runs before Wait, so every goroutine stops when svc.Run returns on its own
	// (for example a closed chat stream) or a later start fails.
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	wg.Go(func() {
		if err := health.ListenAndServe(); !errors.Is(err, http.ErrServerClosed) {
			log.ErrorContext(ctx, "health endpoint stopped", "error", err)
		}
	})
	defer func() { _ = health.Shutdown(context.WithoutCancel(ctx)) }()
	// Before the jobs API: a worker must not read a mirror that holds more than branches and tags.
	if dirs.Repos != "" {
		if err := (&trustedgit.Git{Dir: dirs.Repos}).ConvertMirrors(ctx); err != nil {
			log.ErrorContext(ctx, "mirrors not converted; they are moved aside or retried at the next use", "error", err)
		}
	}
	if jobsListen != "" {
		stopJobs, err := serveJobs(ctx, log, pool, inst, sec, jobsListen, &wg)
		if err != nil {
			return err
		}
		defer stopJobs()
	}
	dirs.Outbox = ob
	dirs.Trackers = trackers
	if err := startRemoteDoors(ctx, &wg, log, pool, inst, sec, ob); err != nil {
		return err
	}
	if err := startConductor(ctx, &wg, log, pool, inst, sec, dirs); err != nil {
		return err
	}
	return svc.Run(ctx)
}

// serveJobs starts the jobs API for the worker. The worker token is kept only as its hash.
func serveJobs(ctx context.Context, log *slog.Logger, pool *pgxpool.Pool, inst *config.Instance, sec config.Secrets, addr string, wg *sync.WaitGroup) (func(), error) {
	hash, err := sec.Read(secretWorkerHash)
	if err != nil {
		return nil, err
	}
	api := &jobserver.Server{Pool: pool, WorkerTokenSHA256: strings.TrimSpace(hash), Log: log, ProviderCaps: map[string]int{}, TenantWeights: map[string]float64{}}
	api.ClaimableProviders = make(map[string]jobserver.BillingIdentity, len(inst.Providers))
	for name, p := range inst.Providers {
		// The current harnesses report usage after spending. Even with token prices, they
		// cannot guarantee a positive pre-call maximum that the book could reserve.
		if inst.Pools.NeedsCreditReservation(p) {
			log.WarnContext(ctx, "jobs held: monthly credit pool needs a guaranteed pre-run cost reservation", "provider", name, "pool", p.Pool)
		} else {
			api.ClaimableProviders[name] = jobserver.BillingIdentity{Harness: p.Harness, Auth: p.Auth, KeyEnv: p.KeyEnv, BaseURL: p.BaseURL}
		}
		api.ProviderCaps[name] = p.Concurrency
	}
	for name, t := range inst.Tenants {
		api.TenantWeights[name] = t.Budget.Weight
	}
	srv := &http.Server{Addr: addr, Handler: api.Handler(), ReadHeaderTimeout: 5 * time.Second}
	wg.Go(func() {
		if err := srv.ListenAndServe(); !errors.Is(err, http.ErrServerClosed) {
			log.ErrorContext(ctx, "jobs api stopped", "error", err)
		}
	})
	return func() { _ = srv.Shutdown(context.WithoutCancel(ctx)) }, nil
}

// migrate applies the book and River migrations; they are expand-only, so a running
// older service keeps working.
func migrate(ctx context.Context, sec config.Secrets) error {
	url, err := sec.Read(secretOwnerURL)
	if err != nil {
		return err
	}
	db, err := sql.Open("pgx", url)
	if err != nil {
		return fmt.Errorf("migrate: open: %w", err)
	}
	defer func() { _ = db.Close() }()
	if err := book.Migrate(ctx, db); err != nil {
		return err
	}
	if err := outbox.Migrate(ctx, db); err != nil {
		return err
	}
	password, err := sec.Read(secretGrafanaPassword)
	if errors.Is(err, fs.ErrNotExist) || errors.Is(err, config.ErrEmptySecret) {
		return nil
	}
	if err != nil {
		return err
	}
	return book.DashboardLogin(ctx, db, password)
}

// chatRoutes builds one adapter per adapter name and routes its tenants to it.
func chatRoutes(inst *config.Instance, sec config.Secrets, log *slog.Logger, pool *pgxpool.Pool) ([]doors.Route, outbox.ChatSender, []string, error) {
	byAdapter := map[string]*doors.Route{}
	chats := outbox.ChatSender{}
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
			var chat chatapi.Chat
			var secrets []string
			var err error
			if t.Chat.Adapter == "inbox" { // lives in the book, needs the pool
				chat = inbox.New(pool)
			} else {
				chat, secrets, err = adapters.NewChat(t.Chat.Adapter, sec, log)
			}
			if err != nil {
				return nil, nil, nil, fmt.Errorf("tenant %s: %w", name, err)
			}
			known = append(known, secrets...)
			r = &doors.Route{Chat: chat}
			byAdapter[t.Chat.Adapter] = r
		}
		r.Tenants = append(r.Tenants, t)
		chats[name] = r.Chat
	}
	routes := make([]doors.Route, 0, len(byAdapter))
	for _, r := range byAdapter {
		routes = append(routes, *r)
	}
	return routes, chats, known, nil
}

// tracing exports spans over OTLP when OTEL_EXPORTER_OTLP_ENDPOINT is set; otherwise
// spans stay in memory and are dropped.
func tracing(ctx context.Context, service string) (func(context.Context) error, error) {
	if os.Getenv("OTEL_EXPORTER_OTLP_ENDPOINT") == "" { //nolint:forbidigo // standard OTel setting, not a secret; the SDK reads it too
		return func(context.Context) error { return nil }, nil
	}
	exp, err := otlptracehttp.New(ctx)
	if err != nil {
		return nil, fmt.Errorf("tracing: %w", err)
	}
	tp := sdktrace.NewTracerProvider(sdktrace.WithBatcher(exp),
		sdktrace.WithResource(resource.NewSchemaless(attribute.String("service.name", service))))
	otel.SetTracerProvider(tp)
	return tp.Shutdown, nil
}

// warnOnly drops records below warn level.
type warnOnly struct{ slog.Handler }

func (h warnOnly) Enabled(ctx context.Context, l slog.Level) bool {
	return l >= slog.LevelWarn && h.Handler.Enabled(ctx, l)
}

func (h warnOnly) WithAttrs(a []slog.Attr) slog.Handler { return warnOnly{h.Handler.WithAttrs(a)} }

func (h warnOnly) WithGroup(n string) slog.Handler { return warnOnly{h.Handler.WithGroup(n)} }

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
	if !slices.ContainsFunc(args, func(a string) bool {
		return a == "--health" || a == "-health" || strings.HasPrefix(a, "--addr") || strings.HasPrefix(a, "-addr")
	}) {
		return doorCmd("status", args, stdout, stderr)
	}
	fs := flag.NewFlagSet("status", flag.ContinueOnError)
	fs.SetOutput(stderr)
	health := fs.Bool("health", false, "check the health endpoint and exit")
	addr := fs.String("addr", defaultListen, "address of the health endpoint")
	if err := fs.Parse(args); err != nil {
		return exitUsage
	}
	if !*health {
		_, _ = fmt.Fprintln(stderr, "nightshift status: pass --health, or run `nightshift status` without flags of its own to list tasks")
		return exitUsage
	}
	client := &http.Client{Timeout: 5 * time.Second}
	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, "http://"+*addr+"/healthz", nil)
	if err != nil {
		_, _ = fmt.Fprintln(stderr, "unhealthy:", err)
		return exitUnhealthy
	}
	resp, err := client.Do(req)
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

// trackerFor builds the tracker of every tenant whose adapter exists. A tenant whose adapter is
// not built runs without a projection: the book is its only view, and the log says so once.
func trackerFor(inst *config.Instance, log *slog.Logger) map[string]trackerapi.Tracker {
	out := map[string]trackerapi.Tracker{}
	for _, name := range slices.Sorted(maps.Keys(inst.Tenants)) {
		tr, err := adapters.NewTracker(inst.Tenants[name].Tracker.Adapter)
		if err != nil {
			log.Info("no tracker projection for a tenant", "tenant", name, "adapter", inst.Tenants[name].Tracker.Adapter)
			continue
		}
		out[name] = tr
	}
	return out
}
