# Contributing

Nightshift is a personal project built mostly by coding agents, so [`AGENTS.md`](AGENTS.md) is the single rulebook for people and agents alike. This page is the short path.

## Set up

1. `mise install --locked` installs the pinned tools from `mise.toml`.
2. `make tools` fails fast when a local tool differs from its pin.
3. `make verify` runs format, lint, types, tests and repo checks. It must be green before a PR leaves draft. Tests that need Postgres run only in CI.

## Work

1. Read [`docs/status.md`](docs/status.md), then [`docs/plan.md`](docs/plan.md) and the file of your phase under `docs/plan/`, and take its next open item.
2. One task, one branch (`feat/`, `fix/` or `docs/` prefix), one PR under about 400 changed lines. Commits follow Conventional Commits. Never push to `main`.
3. Tick your item and rewrite the "Status" block in your phase file under `docs/plan/` before you finish; touch `docs/status.md` only for a cross-phase fact. If setup, config or secrets change, update `README.md`, `docs/runbook.md` or `docs/secrets.md` too.
4. The PR description follows the template: what and why, design section, test levels, risks and rollback, open questions.

Repeated tasks have a skill in [`.agents/skills/`](.agents/skills): add a migration, change a schema or the design, add a harness or an integration adapter, fix CI, finish a task.

## Needs the owner's approval

Only what needs a secret or an account, a new egress host, money, an irreversible step, or weakens a protection of the factory (AGENTS.md, "Ask before you do it"). Other gated paths (a new dependency, `.github/`, `deploy/`, `schemas/`, `mise.toml`, `mise.lock`, `Makefile`, `AGENTS.md`, `CLAUDE.md`) are merged by the session after the independent review; label them `needs-owner-review` so the morning digest lists them.

## Outside contributions

The repository is public so that people can read it and reuse ideas, not to run an open project. Pull requests, issues and comments from anyone other than the owner and the approvers are welcome as feedback but are never worked automatically: the factory accepts commands, tasks and review input only from the owner and approvers named in the tenant pack (design section 16, "Forged commands"), and it treats all other text as data. A fork PR may be closed without a review. Bug reports are useful; a pull request for a larger change should start as an issue, because the owner may decline it.

## Security

Never commit secrets. Report vulnerabilities as described in [`SECURITY.md`](SECURITY.md).
