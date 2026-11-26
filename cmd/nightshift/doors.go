package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/ilyachakun/nightshift/internal/config"
	"github.com/ilyachakun/nightshift/internal/dispatch"
	"github.com/ilyachakun/nightshift/internal/flow"
	"github.com/ilyachakun/nightshift/internal/mcpdoor"
	"github.com/ilyachakun/nightshift/internal/summary"
)

// doorSource is how the terminal's decisions are recorded in the book (ADR 0006).
const doorSource = "cli"

const doorUsage = `usage:
  nightshift status [--tenant name]                 open tasks
  nightshift status [--tenant name] --task <task>   one task's timeline: moves, jobs, why a run stopped
  nightshift inbox [--tenant name]                  cards waiting for your answer
  nightshift reply [--tenant name] <task> "<text>"  answer Lead's question or add to the thread
  nightshift approve [--tenant name] <task>         approve the spec card
  nightshift cancel [--tenant name] <task>          cancel a task that must not run any more`

// doorFlags are shared by the commands that talk to the book.
type doorFlags struct {
	fs      *flag.FlagSet
	dir     *string
	secrets *string
	tenant  *string
}

func newDoorFlags(name string, stderr io.Writer) doorFlags {
	fs := flag.NewFlagSet(name, flag.ContinueOnError)
	fs.SetOutput(stderr)
	return doorFlags{
		fs:      fs,
		dir:     fs.String("config", "/etc/nightshift", "directory with instance.yaml"),
		secrets: fs.String("secrets", config.DefaultSecretsDir, "directory with compose secrets"),
		tenant:  fs.String("tenant", "", "tenant name; default the only tenant of the instance"),
	}
}

// pickTenant returns the named tenant, or the only one.
func pickTenant(inst *config.Instance, name string) (config.Tenant, error) {
	if name != "" {
		t, ok := inst.Tenants[name]
		if !ok {
			return config.Tenant{}, fmt.Errorf("instance %s has no tenant %q", inst.Name, name)
		}
		return t, nil
	}
	names := make([]string, 0, len(inst.Tenants))
	for n := range inst.Tenants {
		names = append(names, n)
	}
	sort.Strings(names)
	if len(names) != 1 {
		return config.Tenant{}, fmt.Errorf("the instance has %d tenants (%s): pass --tenant", len(names), strings.Join(names, ", "))
	}
	return inst.Tenants[names[0]], nil
}

// withDoors opens the book as the service role and runs fn with the tenant's doors.
func withDoors(ctx context.Context, f doorFlags, fn func(context.Context, *dispatch.Doors, config.Tenant) error) error {
	inst, err := config.LoadInstance(config.NewValidator(), *f.dir)
	if err != nil {
		return err
	}
	t, err := pickTenant(inst, *f.tenant)
	if err != nil {
		return err
	}
	sec := config.Secrets{Dir: *f.secrets}
	url, err := sec.Read(secretServiceURL)
	if err != nil {
		return err
	}
	pool, err := pgxpool.New(ctx, url)
	if err != nil {
		return fmt.Errorf("connect: %w", err)
	}
	defer pool.Close()
	d := &dispatch.Doors{Pool: pool, Outbox: &dispatch.Outbox{Pool: pool, Mask: dispatch.NewMasker(url)}, Mover: &flow.Mover{Pool: pool}}
	return fn(ctx, d, t)
}

// doorCmd runs status, inbox, reply and approve for a tenant that works without a chat app.
func doorCmd(name string, args []string, stdout, stderr io.Writer) int {
	f := newDoorFlags(name, stderr)
	var task *int64
	if name == "status" {
		task = f.fs.Int64("task", 0, "show this task's timeline instead of the open tasks")
	}
	if err := f.fs.Parse(args); err != nil {
		return exitUsage
	}
	if task != nil && *task <= 0 && flagSet(f.fs, "task") {
		_, _ = fmt.Fprintln(stderr, "nightshift status: --task needs a task id above 0")
		return exitUsage
	}
	rest := f.fs.Args()
	ctx := context.Background()
	var err error
	switch name {
	case "status":
		err = withDoors(ctx, f, func(ctx context.Context, d *dispatch.Doors, t config.Tenant) error {
			if *task > 0 {
				entries, err := summary.Timeline(ctx, d.Pool, t.Name, *task)
				if err != nil {
					return err
				}
				_, _ = io.WriteString(stdout, summary.FormatTimeline(*task, entries, tenantZone(t)))
				return nil
			}
			lines, err := d.Status(ctx, t)
			_, _ = io.WriteString(stdout, renderStatus(lines))
			return err
		})
	case "inbox":
		err = withDoors(ctx, f, func(ctx context.Context, d *dispatch.Doors, t config.Tenant) error {
			p, err := d.Inbox(ctx, t)
			_, _ = io.WriteString(stdout, renderInbox(p))
			return err
		})
	case "reply":
		id, text, ok := replyArgs(rest)
		if !ok {
			_, _ = fmt.Fprintln(stderr, doorUsage)
			return exitUsage
		}
		err = withDoors(ctx, f, func(ctx context.Context, d *dispatch.Doors, t config.Tenant) error {
			return d.Reply(ctx, t, id, text, doorSource)
		})
	case "cancel":
		id, err2 := strconv.ParseInt(strings.TrimPrefix(strings.Join(rest, ""), "#"), 10, 64)
		if len(rest) != 1 || err2 != nil {
			_, _ = fmt.Fprintln(stderr, doorUsage)
			return exitUsage
		}
		err = withDoors(ctx, f, func(ctx context.Context, d *dispatch.Doors, t config.Tenant) error {
			if err := d.Cancel(ctx, t, id, doorSource); err != nil {
				return err
			}
			_, _ = fmt.Fprintf(stdout, "canceled task #%d\n", id)
			return nil
		})
	case "approve":
		id, err2 := strconv.ParseInt(strings.TrimPrefix(strings.Join(rest, ""), "#"), 10, 64)
		if len(rest) != 1 || err2 != nil {
			_, _ = fmt.Fprintln(stderr, doorUsage)
			return exitUsage
		}
		err = withDoors(ctx, f, func(ctx context.Context, d *dispatch.Doors, t config.Tenant) error {
			return d.Approve(ctx, t, id, doorSource)
		})
	}
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "nightshift %s: %v\n", name, err)
		return exitFailure
	}
	return exitOK
}

// replyArgs reads `<task> <text...>`; the task may be written #12.
func replyArgs(rest []string) (int64, string, bool) {
	if len(rest) < 2 {
		return 0, "", false
	}
	id, err := strconv.ParseInt(strings.TrimPrefix(rest[0], "#"), 10, 64)
	text := strings.TrimSpace(strings.Join(rest[1:], " "))
	return id, text, err == nil && text != ""
}

// flagSet reports whether the flag was given on the command line.
func flagSet(fs *flag.FlagSet, name string) bool {
	set := false
	fs.Visit(func(f *flag.Flag) { set = set || f.Name == name })
	return set
}

// tenantZone is the tenant's schedule.timezone, UTC when it is unset or unknown.
func tenantZone(t config.Tenant) *time.Location {
	if t.Schedule.Timezone == "" {
		return time.UTC
	}
	if loc, err := time.LoadLocation(t.Schedule.Timezone); err == nil {
		return loc
	}
	return time.UTC
}

func renderStatus(lines []dispatch.TaskLine) string {
	if len(lines) == 0 {
		return "no open tasks\n"
	}
	var b strings.Builder
	for _, l := range lines {
		fmt.Fprintf(&b, "#%d  %-13s %s\n", l.ID, l.State, l.Title)
	}
	return b.String()
}

func renderInbox(pending []dispatch.Pending) string {
	if len(pending) == 0 {
		return "nothing waits for you\n"
	}
	var b strings.Builder
	for _, p := range pending {
		fmt.Fprintf(&b, "#%d  %s\n%s\n", p.TaskID, p.Title, p.Text)
		if len(p.Buttons) > 0 {
			labels := make([]string, 0, len(p.Buttons))
			for _, btn := range p.Buttons {
				labels = append(labels, btn.Action)
			}
			fmt.Fprintf(&b, "  buttons: %s (nightshift approve %d)\n", strings.Join(labels, ", "), p.TaskID)
		} else {
			fmt.Fprintf(&b, "  answer with: nightshift reply %d \"...\"\n", p.TaskID)
		}
		b.WriteString("\n")
	}
	return b.String()
}

// mcpCmd serves the doors over stdio for a Claude client (ADR 0006): the client starts it, for
// example `claude mcp add nightshift -- docker compose exec -T nightshift nightshift mcp`.
// Nothing but protocol may reach stdout.
func mcpCmd(args []string, stderr io.Writer) int {
	f := newDoorFlags("mcp", stderr)
	if err := f.fs.Parse(args); err != nil {
		return exitUsage
	}
	err := withDoors(context.Background(), f, func(ctx context.Context, d *dispatch.Doors, t config.Tenant) error {
		return mcpdoor.Serve(ctx, mcpdoor.New(mcpdoor.Doors{Doors: d, Tenant: t, Source: "mcp"}, mcpdoor.Options{Version: version}))
	})
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "nightshift mcp: %v\n", err)
		return exitFailure
	}
	return exitOK
}

// secretMCPToken is the file in the secrets directory that holds the remote door's token.
const secretMCPToken = "mcp_token"

// startRemoteDoors serves the remote MCP door of every tenant that names a listen address in
// its tenant.yaml. It is off by default; the address must be loopback and the token comes from
// the secrets directory.
func startRemoteDoors(ctx context.Context, wg *sync.WaitGroup, log *slog.Logger, pool *pgxpool.Pool, inst *config.Instance, sec config.Secrets, outbox *dispatch.Outbox) error {
	doors := &dispatch.Doors{Pool: pool, Outbox: outbox, Mover: &flow.Mover{Pool: pool}}
	for _, t := range inst.Tenants {
		addr := t.MCP.Remote.Listen
		if addr == "" {
			continue
		}
		if err := mcpdoor.ValidateListen(addr); err != nil {
			return fmt.Errorf("tenant %s: %w", t.Name, err)
		}
		token, err := sec.Read(secretMCPToken)
		if err != nil {
			return fmt.Errorf("tenant %s: the remote MCP door needs %s: %w", t.Name, secretMCPToken, err)
		}
		h, err := mcpdoor.RemoteHandler(mcpdoor.Doors{Doors: doors, Tenant: t, Source: mcpdoor.SourceRemote}, strings.TrimSpace(token), mcpdoor.Options{Version: version})
		if err != nil {
			return fmt.Errorf("tenant %s: %w", t.Name, err)
		}
		srv := &http.Server{Addr: addr, Handler: h, ReadHeaderTimeout: 5 * time.Second}
		wg.Go(func() {
			if err := srv.ListenAndServe(); !errors.Is(err, http.ErrServerClosed) {
				log.ErrorContext(ctx, "remote mcp door stopped", "tenant", t.Name, "error", err)
			}
		})
		wg.Go(func() {
			<-ctx.Done()
			_ = srv.Shutdown(context.WithoutCancel(ctx))
		})
		log.InfoContext(ctx, "remote mcp door listening", "tenant", t.Name, "addr", addr)
	}
	return nil
}
