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
	{"deps", "check a package before it becomes a dependency", 4},
	{"toolchain", "install a project's pinned tools for the worker", 4},
	{"check", "run quality checks in a worktree and emit SARIF", 3},
	{"standards", "list, index and sync standards", 4},
	{"skills", "vendor and scan external skills", 4},
	{"tenant", "init, validate and diagnose tenants", 1},
	{"config", "validate the instance, tenants and a project's .nightshift/ files", 5},
	{"task", "add a task to the queue", 3},
	{"digest", "print the morning digest: merges and decisions with reasons and revert links", 5},
	{"memory", "show the project memory page or remember a fact", 5},
	{"setup", "create what an instance needs: the GitHub App", 3},
	{"eval", "run polygon evals", 4},
	{"doctor", "check tokens, providers and harness versions", 2},
	{"status", "show factory health and open tasks", 1},
	{"inbox", "show the cards waiting for your answer", 3},
	{"reply", "answer Lead's question in a task thread", 3},
	{"approve", "approve the spec card of a task", 3},
	{"cancel", "cancel a task that must not run any more", 5},
	{"mcp", "serve the doors to a Claude client over MCP (stdio)", 3},
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
	case "eval":
		return evalCmd(args[1:], stdout, stderr)
	case "deps":
		return depsCmd(args[1:], stdout, stderr)
	case "toolchain":
		return toolchainCmd(args[1:], stderr)
	case "check":
		return checkCmd(args[1:], stdout, stderr)
	case "status":
		return status(args[1:], stdout, stderr)
	case "inbox", "reply", "approve", "cancel":
		return doorCmd(args[0], args[1:], stdout, stderr)
	case "mcp":
		return mcpCmd(args[1:], stderr)
	case "doctor":
		return doctorCmd(args[1:], stdout, stderr)
	case "setup":
		return setupCmd(args[1:], stdout, stderr)
	case "skills":
		return skillsCmd(args[1:], stdout, stderr)
	case "standards":
		return standardsCmd(args[1:], stdout, stderr)
	case "task":
		return taskCmd(args[1:], stdout, stderr)
	case "config":
		return configCmd(args[1:], stdout, stderr)
	case "tenant":
		return tenantCmd(args[1:], stdout, stderr)
	case "digest":
		return digestCmd(args[1:], stdout, stderr)
	case "memory":
		return memoryCmd(args[1:], stdout, stderr)
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
