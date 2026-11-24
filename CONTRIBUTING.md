# Contributing

Nightshift is a personal project built mostly by coding agents, so [`AGENTS.md`](AGENTS.md) is the single rulebook for people and agents alike. This page is the short path.

## Set up

1. `mise install --locked` installs the pinned tools from `mise.toml`.
2. `make tools` fails fast when a local tool differs from its pin.
3. `make verify` runs format, lint, types, tests and repo checks. It must be green before a PR leaves draft. Tests that need Postgres run only in CI.

## Work

1. Read [`docs/status.md`](docs/status.md), then [`docs/plan.md`](docs/plan.md), and take the next open item of the current phase.
2. One task, one branch (`feat/`, `fix/` or `docs/` prefix), one PR under about 400 changed lines. Commits follow Conventional Commits. Never push to `main`.
3. Update `docs/plan.md` and `docs/status.md` before you finish. If setup, config or secrets change, update `README.md`, `docs/runbook.md` or `docs/secrets.md` too.
4. The PR description follows the template: what and why, design section, test levels, risks and rollback, open questions.

Repeated tasks have a skill in [`.agents/skills/`](.agents/skills): add a migration, change a schema or the design, add a harness or an integration adapter, fix CI, finish a task.

## Needs the owner's approval

A new dependency, anything under `.github/`, `deploy/`, `schemas/`, `mise.toml`, `mise.lock`, `Makefile`, `AGENTS.md` or `CLAUDE.md`, and anything about secrets, egress, permissions, gates or the review policy. Label such a PR `needs-owner-review`.

## Security

Never commit secrets. Report vulnerabilities as described in [`SECURITY.md`](SECURITY.md).
