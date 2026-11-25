package main

import (
	"fmt"
	"io"
)

// version is set at build time with -ldflags "-X main.version=...".
var version = "dev"

// Exit codes.
const (
	exitOK             = 0
	exitNotImplemented = 1
	exitUnhealthy      = 1
	exitUsage          = 2
	exitFailure        = 3
)

type command struct {
	name    string
	summary string
	phase   int // design phase that implements the command
}

// commands lists every subcommand from the design (section 4). Those not handled in
// run are stubs until the phase that implements them.
var commands = []command{
	{"serve", "run the core service: chat, dispatcher, gates, merge train", 1},
	{"work", "run the worker runner that executes role jobs", 2},
	{"check", "run quality checks in a worktree and emit SARIF", 3},
	{"standards", "list, index and sync standards", 4},
	{"skills", "vendor and scan external skills", 4},
	{"tenant", "init, validate and diagnose tenants", 1},
	{"eval", "run polygon evals", 4},
	{"doctor", "check tokens, providers and harness versions", 2},
	{"status", "show factory health and active tasks", 1},
}

func run(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		usage(stderr)
		return exitUsage
	}
	switch args[0] {
	case "-h", "--help", "help":
		usage(stdout)
		return exitOK
	case "version", "--version":
		_, _ = fmt.Fprintln(stdout, "nightshift", version)
		return exitOK
	case "serve":
		return serve(args[1:], stderr)
	case "work":
		return work(args[1:], stderr)
	case "check":
		return checkCmd(args[1:], stdout, stderr)
	case "status":
		return status(args[1:], stdout, stderr)
	case "doctor":
		return doctorCmd(args[1:], stdout, stderr)
	case "tenant":
		return tenantCmd(args[1:], stdout, stderr)
	}
	for _, c := range commands {
		if c.name == args[0] {
			_, _ = fmt.Fprintf(stderr, "nightshift %s: not implemented yet (design phase %d)\n", c.name, c.phase)
			return exitNotImplemented
		}
	}
	_, _ = fmt.Fprintf(stderr, "nightshift: unknown command %q\n\n", args[0])
	usage(stderr)
	return exitUsage
}

func usage(w io.Writer) {
	_, _ = fmt.Fprintln(w, "Usage: nightshift <command> [flags]")
	_, _ = fmt.Fprintln(w)
	_, _ = fmt.Fprintln(w, "Commands:")
	for _, c := range commands {
		_, _ = fmt.Fprintf(w, "  %-10s %s\n", c.name, c.summary)
	}
	_, _ = fmt.Fprintf(w, "  %-10s %s\n", "version", "print the version")
}
