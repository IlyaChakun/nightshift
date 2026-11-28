---
status: accepted
date: 2026-11-28
---

# The acceptance Verifier browses through a browser tool

## Context and Problem Statement

A Verifier acceptance job runs the hidden scenarios against the preview in Chromium and must keep screenshots as evidence (design section 6). The role is read-only: it reads the repository and writes only to its job directory. The skill `browser-acceptance` had the agent write and run Playwright scripts with the shell, and the runner collected screenshots from the checkout. On the owner's Mac on 2026-11-28 (V4, task #1005), the read-only turn wrote no screenshot, so the evidence card had none. How does the acceptance job drive a browser and keep screenshots, while it writes nothing but its evidence?

## Considered Options

- A browser tool: a pinned Playwright MCP server in the worker image, the only tool besides reading; the shell and the file tools are refused for the job.
- The shell for acceptance jobs, with the checkout made read-only and a private HOME and temporary directory per job.
- The agent writes one script into its evidence directory, and the runner runs it after the turn.

## Decision Outcome

Chosen option: "a browser tool" (owner, 2026-11-28, `docs/decisions.md`). The server starts in the job's evidence directory with `--ignore-roots`, so the only place it writes files is that directory, and the runner collects the evidence from there. The turn runs in the CLI's `dontAsk` mode, which runs only the listed tools: reading and the browser tools. The tools that run code in the server or the page or read local files (`browser_run_code_unsafe`, `browser_evaluate`, `browser_file_upload`, `browser_drop`) are refused, and so are `Bash`, `Write`, `Edit` and `NotebookEdit`. The runner keeps only files whose content is an image or video, so the browser server cannot pass text off as evidence, and Chromium bypasses the proxies only for loopback. Only a harness that wires the tool gets it; on another the job keeps its screenshots in the checkout, as before. Mode and tool behaviour: [permission modes](https://code.claude.com/docs/en/permission-modes) and the `@playwright/mcp` 0.0.85 README, both read 2026-11-28. Review jobs keep their tools; they run checks with the shell.

### Consequences

- Good: the acceptance job writes screenshots and nothing else, without a new uid or mount.
- Good: the agent still iterates on a step that fails, as a person would.
- Bad: a new pinned dependency in the worker image (`@playwright/mcp`, `PLAYWRIGHT_MCP_VERSION`), with its own Playwright build driving Debian's Chromium.
- Bad: only the `claude` harness wires the tool; an acceptance job on `codex` browses with its shell as before.

### Confirmation

T1 tests of the arguments (`internal/execution/harness/claude/browser_test.go`) and of the runner (`internal/execution/runner/browser_test.go`); a local run of the generated configuration saved a PNG in the evidence directory and refused paths outside it; the live check `docs/live-checks/V4-acceptance-screenshots.md`.

## Pros and Cons of the Options

### The shell with a read-only checkout

- Good: the skill keeps Playwright scripts.
- Bad: any process the agent starts writes wherever the agent user can; limiting that takes a separate user or mount per job.

### A script the runner runs

- Good: the agent writes one file.
- Bad: the agent cannot see a failing step and try again, so acceptance gets weaker.
